package driver

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/buildwarden/buildwarden/collector"
	"github.com/buildwarden/buildwarden/driver/script"
	"github.com/buildwarden/buildwarden/relaycfg"
)

// Session is the host side of one build, identical across drivers: a
// collector that receives every relay output and serves every build input,
// the claim that configures the relay, and the wait for completion. The
// collector is served to the relay over a tunnel the host opens on the
// relay's claim port, so the host never listens on any network and the relay
// never dials the host. Drivers differ only in how they start the relay and
// the build guest and in the claim address they hand to the session.
//
//	s, _ := driver.StartSession(cfg)              // collector ready (no listener)
//	(driver boots the relay, unconfigured)
//	id, _ := s.ClaimRelay(ctx, addr, tok, req, nil) // tunnel up, relay configured
//	(driver boots the build guest)
//	_ = s.Wait(ctx)                               // completion or failure
//	s.Close(); driver.FinalizeOutput(...)
type Session struct {
	Collector *collector.Collector
	// Token authenticates the relay's requests to the collector.
	Token string

	outputDir string
	tunnel    net.Conn
	tunnelEnd <-chan struct{}
}

// SessionConfig configures StartSession.
type SessionConfig struct {
	// OutputDir receives the build's outputs. Required.
	OutputDir string
	// ContextDir and BuildScript are the build inputs the collector serves.
	ContextDir  string
	BuildScript string
	// OutputTee receives the live build console (nil under --quiet).
	OutputTee io.Writer
	// Verbose logs collector traffic to stderr.
	Verbose bool
}

// HeartbeatTimeout is how long a build guest may go silent, once it has sent
// a first heartbeat, before the build is failed as hung.
const HeartbeatTimeout = 30 * time.Second

// ClaimTimeout bounds how long a relay may take to boot and accept its claim.
const ClaimTimeout = 90 * time.Second

// StartSession prepares the build's collector.
func StartSession(cfg SessionConfig) (*Session, error) {
	if cfg.OutputDir == "" {
		return nil, fmt.Errorf("session: OutputDir is required")
	}
	token, err := RandomToken()
	if err != nil {
		return nil, err
	}
	var logf func(string, ...any)
	if cfg.Verbose {
		logf = func(f string, a ...any) { fmt.Fprintf(os.Stderr, "[warden] "+f+"\n", a...) }
	}
	col, err := collector.New(collector.Config{
		OutputDir:       cfg.OutputDir,
		Token:           token,
		ContextDir:      cfg.ContextDir,
		BuildScriptPath: cfg.BuildScript,
		OutputTee:       cfg.OutputTee,
		Logf:            logf,
	})
	if err != nil {
		return nil, err
	}
	return &Session{Collector: col, Token: token, outputDir: cfg.OutputDir}, nil
}

// ClaimRelay connects to the relay's claim endpoint at addr (retrying until
// the relay is up), opens the collector tunnel, pushes the build's
// configuration, waits for the relay to report ready, and writes its CA
// certificate to <output>/ca.cert.pem. env is the optional environment record
// (the container driver's base image manifest).
func (s *Session) ClaimRelay(ctx context.Context, addr, claimToken string,
	req *BuildRequest, env *relaycfg.Environment) (relaycfg.Identity, error) {
	cas, err := LoadUpstreamCAs(req.UpstreamCACerts)
	if err != nil {
		return relaycfg.Identity{}, err
	}
	cfg := relaycfg.Config{
		CollectorURL:   relaycfg.TunnelURL,
		CollectorToken: s.Token,
		CaptureMode:    req.CaptureMode,
		Environment:    env,
	}
	if len(cas) > 0 {
		cfg.UpstreamCACerts = []string{string(cas)}
		if !req.UpstreamSystemCA {
			f := false
			cfg.UpstreamSystemCA = &f
		}
	}

	ctx, cancel := context.WithTimeout(ctx, ClaimTimeout)
	defer cancel()
	conn, end, err := relaycfg.OpenTunnel(ctx, addr, claimToken, s.Collector.Handler())
	if err != nil {
		return relaycfg.Identity{}, fmt.Errorf("connecting to relay: %w", err)
	}
	s.tunnel, s.tunnelEnd = conn, end
	id, err := relaycfg.Claim(ctx, addr, claimToken, cfg)
	if err != nil {
		return relaycfg.Identity{}, fmt.Errorf("claiming relay: %w", err)
	}
	select {
	case <-s.Collector.Ready():
	case <-end:
		return relaycfg.Identity{}, fmt.Errorf("relay tunnel closed before the relay was ready")
	case <-ctx.Done():
		return relaycfg.Identity{}, fmt.Errorf("relay was claimed but never reported ready")
	}
	if err := os.WriteFile(filepath.Join(s.outputDir, "ca.cert.pem"), []byte(id.CACert), 0o644); err != nil {
		return relaycfg.Identity{}, err
	}
	return id, nil
}

// Wait blocks until the build completes, its guest stops heartbeating, the
// relay's tunnel drops, or ctx ends. A non-zero exit code is an error.
func (s *Session) Wait(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var tunnelLost atomic.Bool
	if s.tunnelEnd != nil {
		go func() {
			select {
			case <-s.tunnelEnd:
				tunnelLost.Store(true)
				cancel()
			case <-ctx.Done():
			}
		}()
	}
	sig, err := s.Collector.Wait(ctx, HeartbeatTimeout)
	if err != nil {
		if sig, ok := s.Collector.Status(); ok {
			return exitErr(sig) // completion raced the tunnel teardown
		}
		var ue *collector.ErrUnresponsive
		switch {
		case errors.As(err, &ue):
			return fmt.Errorf("build failed: %w", err)
		case tunnelLost.Load():
			return fmt.Errorf("build failed: lost the connection to the relay")
		}
		return err
	}
	return exitErr(sig)
}

func exitErr(sig collector.CompletionSignal) error {
	if sig.ExitCode == 0 {
		return nil
	}
	if msg := firstNonEmpty(sig.Error, sig.Message); msg != "" {
		return fmt.Errorf("build exited with code %d: %s", sig.ExitCode, msg)
	}
	return fmt.Errorf("build exited with code %d", sig.ExitCode)
}

// Close tears down the relay tunnel.
func (s *Session) Close() {
	if s.tunnel != nil {
		_ = s.tunnel.Close()
	}
}

// ConsoleTee is where a guest's live build console goes: req.Stdout (or
// os.Stdout), or nowhere under --quiet. It is captured to build-output.log
// either way.
func ConsoleTee(req *BuildRequest) io.Writer {
	if req.Quiet {
		return nil
	}
	if req.Stdout != nil {
		return req.Stdout
	}
	return os.Stdout
}

// FinalizeOutput applies the output conventions every driver shares:
// optional zstd compression of the ledger and CA certificate.
func FinalizeOutput(outputDir string, compress bool) {
	if !compress {
		return
	}
	for _, name := range []string{"ledger", "ca.cert.pem"} {
		src := filepath.Join(outputDir, name)
		if _, err := os.Stat(src); err != nil {
			continue
		}
		compressFile(src, src+".zst")
		if _, err := os.Stat(src + ".zst"); err == nil {
			_ = os.Remove(src)
		}
	}
}

// LoadUpstreamCAs reads extra upstream CA certificates from files or
// directories (*.pem, *.crt, *.cer) into one PEM bundle.
func LoadUpstreamCAs(paths []string) ([]byte, error) {
	var bundle []byte
	add := func(path string) error {
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("reading CA cert %q: %w", path, err)
		}
		bundle = append(bundle, data...)
		if len(data) > 0 && data[len(data)-1] != '\n' {
			bundle = append(bundle, '\n')
		}
		return nil
	}
	for _, p := range paths {
		info, err := os.Stat(p)
		if err != nil {
			return nil, fmt.Errorf("upstream CA path %q: %w", p, err)
		}
		if !info.IsDir() {
			if err := add(p); err != nil {
				return nil, err
			}
			continue
		}
		entries, err := os.ReadDir(p)
		if err != nil {
			return nil, fmt.Errorf("reading CA directory %q: %w", p, err)
		}
		for _, e := range entries {
			switch strings.ToLower(filepath.Ext(e.Name())) {
			case ".pem", ".crt", ".cer":
				if !e.IsDir() {
					if err := add(filepath.Join(p, e.Name())); err != nil {
						return nil, err
					}
				}
			}
		}
	}
	return bundle, nil
}

// RandomToken returns a 256-bit hex token.
func RandomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// FreeLoopbackPort reserves and releases a loopback TCP port, for forwarding
// a relay's claim port to the host. The tiny reuse race is acceptable: a
// collision fails the claim loudly rather than misrouting it.
func FreeLoopbackPort() (int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port, nil
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// StageBuildScript resolves the script a VM guest runs and writes it to
// workDir, returning its path (served by the collector at /build-script).
// Precedence, the same for every VM driver: --script; then, for Linux guests,
// the Containerfile translated to a script; then build.sh / build.ps1 in the
// context; then a no-op so the pipeline still runs end to end.
func StageBuildScript(req *BuildRequest, workDir string) (string, error) {
	windows := strings.EqualFold(req.GuestOS, "windows")
	name, noop := "build.sh", "#!/bin/sh\ntrue\n"
	if windows {
		name, noop = "build.ps1", "exit 0\r\n"
	}
	dst := filepath.Join(workDir, name)

	if req.Script != "" {
		return dst, CopyFile(req.Script, dst)
	}
	if !windows && req.Containerfile != "" {
		res, err := script.Translate(req.Containerfile)
		if err != nil {
			return "", fmt.Errorf("translating %s: %w", req.Containerfile, err)
		}
		return dst, os.WriteFile(dst, []byte(res.Script), 0o755)
	}
	if req.ContextDir != "" {
		if cand := filepath.Join(req.ContextDir, name); fileExists(cand) {
			return dst, CopyFile(cand, dst)
		}
	}
	return dst, os.WriteFile(dst, []byte(noop), 0o755)
}

func fileExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}
