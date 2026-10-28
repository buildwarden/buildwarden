package main

import (
	"bytes"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/buildwarden/buildwarden/bootstrap"
)

// initArgs are initialize's flags. Everything else comes from the relay.
type initArgs struct {
	gateway, selfIP, script string
	// workdir is created and entered before the script runs.
	workdir string
	// hold stops after setup, for an interactive shell (warden shell): it
	// writes the build environment to shellEnvPath() and blocks.
	hold bool
}

// runInitialize bootstraps the build environment from the relay and runs the
// build (docs/development/bootstrap.md):
//  1. Configure the network (platform-specific DNS/IP) and wait for the relay
//  2. Fetch the CA and the bootstrap config; install the CA per its trust mode
//  3. Apply the baseline (Java truststore, bazelrc, CA and default env)
//  4. Enter the workdir; with --hold, stop here
//  5. Fetch the build script, run it with heartbeats, report its exit
func runInitialize(args []string) int {
	a := parseInitArgs(args)
	if a.gateway == "" {
		a.gateway = os.Getenv("WARDEN_GATEWAY")
	}
	if a.selfIP == "" {
		a.selfIP = os.Getenv("WARDEN_IP")
	}
	if a.script == "" {
		a.script = defaultScriptName()
	}

	logStep("configuring network")
	if err := configureNetwork(a.gateway, a.selfIP); err != nil {
		return fail("network config", err)
	}
	logStep("waiting for relay")
	if err := waitForRelay(a.gateway); err != nil {
		return fail("relay not ready", err)
	}

	env, err := setUp(a.workdir)
	if err != nil {
		return fail("setup", err)
	}
	if a.hold {
		return holdForShell(env, a.workdir)
	}

	// The local filename (and therefore the interpreter scriptCommand picks)
	// comes from the platform default or --script; the source is always the
	// relay's /build-script.
	logStep("fetching build script")
	scriptPath := scriptDestPath(a.script)
	if err := fetchBuildScript(scriptPath); err != nil {
		return fail("fetch script", err)
	}
	os.Chmod(scriptPath, 0755) //nolint:errcheck

	logStep("running " + a.script)
	code := execScript(scriptPath, a.gateway)
	reportComplete(code)
	return code
}

// setUp installs the CA per the bootstrap config, applies the baseline (its
// environment is set on this process, so the build script inherits it) and
// enters the workdir. It returns the baseline environment.
func setUp(workdir string) (map[string]string, error) {
	logStep("installing CA")
	ca, err := fetchCA()
	if err != nil {
		return nil, fmt.Errorf("CA: %w", err)
	}
	cfg, err := fetchBootstrap()
	if err != nil {
		return nil, fmt.Errorf("bootstrap config: %w", err)
	}
	if err := installCA(ca.pem, ca.cert, cfg.TrustMode()); err != nil {
		return nil, fmt.Errorf("CA install: %w", err)
	}

	logStep("applying baseline (trust " + cfg.TrustMode() + ")")
	env, err := applyBaseline(ca, findCABundle())
	if err != nil {
		return nil, fmt.Errorf("baseline: %w", err)
	}
	for k, v := range env {
		os.Setenv(k, v) //nolint:errcheck
	}

	if workdir != "" {
		if err := os.MkdirAll(workdir, 0755); err != nil {
			return nil, fmt.Errorf("workdir: %w", err)
		}
		if err := os.Chdir(workdir); err != nil {
			return nil, fmt.Errorf("workdir: %w", err)
		}
	}
	return env, nil
}

func fail(what string, err error) int {
	fmt.Fprintf(os.Stderr, "warden-io: %s: %s\n", what, err)
	return 1
}

// holdForShell records the build environment for an interactive shell to
// source and blocks until the environment is torn down. The file's appearance
// tells the orchestrator setup is done.
func holdForShell(env map[string]string, workdir string) int {
	if self, err := os.Executable(); err == nil {
		env["PATH"] = filepath.Dir(self) + string(os.PathListSeparator) + os.Getenv("PATH")
	}
	script := shellEnvScript(env, workdir)
	if err := writeFileMkdir(shellEnvPath(), []byte(script), 0o644); err != nil {
		return fail("hold", err)
	}
	logStep("ready; holding for a shell")
	select {}
}

// shellEnvPath is where --hold writes the environment a shell sources.
func shellEnvPath() string { return filepath.Join(wardenDataDir(), "shell.env") }

func waitForRelay(gateway string) error {
	healthURL := "http://artifacts/health"
	if gateway != "" {
		healthURL = fmt.Sprintf("http://%s:8300/health", gateway)
	}

	client := &http.Client{Timeout: 2 * time.Second}
	deadline := time.Now().Add(60 * time.Second)

	for time.Now().Before(deadline) {
		resp, err := client.Get(healthURL)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("relay did not become healthy within 60s")
}

// fetchBuildScript downloads the build script from the relay's canonical
// build-script endpoint and writes it to dest. Unlike the generic context fetch
// (http://cwd/<file>), this is a well-known resource the relay resolves itself
// (from the shared context, or on a diskless relay by streaming from the
// collector), so the guest never needs to know where the script physically
// lives.
func fetchBuildScript(dest string) error {
	resp, err := http.Get("http://artifacts/build-script")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	if dir := filepath.Dir(dest); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, resp.Body)
	return err
}

// caCert is the relay's CA as served and as parsed.
type caCert struct {
	pem  []byte
	cert *x509.Certificate
}

func fetchCA() (*caCert, error) {
	data, status, err := relayGet("http://artifacts/ca.pem")
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", status)
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("no PEM certificate")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parsing CA: %w", err)
	}
	return &caCert{pem: data, cert: cert}, nil
}

// fetchBootstrap reads the build's bootstrap config. A relay or collector
// without one (404) gives the empty config; anything this executor does not
// understand is an error rather than a guess.
func fetchBootstrap() (bootstrap.Config, error) {
	data, status, err := relayGet("http://artifacts/bootstrap")
	if err != nil {
		return bootstrap.Config{}, err
	}
	switch status {
	case http.StatusOK:
		return bootstrap.Parse(data)
	case http.StatusNotFound:
		return bootstrap.Config{}, nil
	}
	return bootstrap.Config{}, fmt.Errorf("HTTP %d", status)
}

func relayGet(url string) ([]byte, int, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	return data, resp.StatusCode, err
}

// scriptDestPath returns where the fetched build script is written on the
// guest. It preserves the script's base name (and thus its extension, which
// scriptCommand uses to pick an interpreter) under the OS temp directory.
func scriptDestPath(script string) string {
	return filepath.Join(os.TempDir(), filepath.Base(script))
}

func execScript(path, gateway string) int {
	cmd := scriptCommand(path)
	cmd.Stdin = os.Stdin

	// Capture build output while echoing it to the local console (serial), and
	// STREAM it to the collector as it is produced rather than buffering the
	// whole log in memory. The build's stdout/stderr fan out to os.Stdout and an
	// io.Pipe; a reader goroutine (below) POSTs each chunk to /v1/output, so a
	// chatty build never costs more than one ~32KB chunk of RAM. Chunk boundaries
	// don't matter: the collector appends raw bytes, so build-output.log is
	// byte-identical regardless of how the stream is split.
	pr, pw := io.Pipe()
	w := io.MultiWriter(os.Stdout, pw)
	cmd.Stdout = w
	cmd.Stderr = w

	// Ensure warden-io is on PATH for the build script (e.g. `warden-io post`).
	cmd.Env = buildScriptEnv()

	if err := cmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "warden-io: exec: %s\n", err)
		return 1
	}

	// Stream build output to the collector as it is produced; see
	// streamBuildOutput for the back-pressure and dead-link behaviour.
	streamDone := make(chan struct{})
	go func() {
		defer close(streamDone)
		streamBuildOutput(pr, gateway)
	}()

	// Send heartbeats while the build runs.
	done := make(chan struct{})
	go heartbeat(done)

	err := cmd.Wait()
	close(done)

	// cmd.Wait has copied all stdout/stderr into the pipe; close the writer so
	// the streamer sees EOF, then wait for it to deliver the final chunk before
	// returning. The caller reports completion right after, which triggers relay
	// teardown, so the tail must already be on its way.
	_ = pw.Close()
	<-streamDone

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return exitErr.ExitCode()
		}
		fmt.Fprintf(os.Stderr, "warden-io: exec: %s\n", err)
		return 1
	}
	return 0
}

// buildScriptEnv returns the build script's environment with warden-io's own
// directory prepended to PATH, so the script can call `warden-io post` etc. The
// PATH key is matched case-INSENSITIVELY (Windows exposes it as "Path=") and
// joined with the OS path-list separator (';' on Windows, ':' elsewhere); a
// hardcoded ":" + "PATH=" would silently no-op on Windows. A PATH is appended
// if none exists, and the environment is left untouched if the executable path
// cannot be resolved.
func buildScriptEnv() []string {
	env := os.Environ()
	self, err := os.Executable()
	if err != nil {
		return env
	}
	selfDir := filepath.Dir(self)
	sep := string(os.PathListSeparator)
	for i, e := range env {
		if len(e) >= 5 && strings.EqualFold(e[:5], "PATH=") {
			env[i] = e[:5] + selfDir + sep + e[5:]
			return env
		}
	}
	return append(env, "PATH="+selfDir)
}

// streamBuildOutput reads build output from pr and POSTs it to the collector in
// chunks until EOF. Reading pr keeps the build's stdout/stderr flowing: io.Pipe
// is unbuffered, so the build blocks only for as long as one chunk POST takes
// against the local relay link. If that link fails mid-build the reader keeps
// draining (and discarding) so the build never hangs on a dead relay; the tail
// is then only in the guest console, which is logged.
func streamBuildOutput(pr *io.PipeReader, gateway string) {
	const disabledMsg = "warden-io: output streaming disabled after a " +
		"relay-link error; remaining build output stays in the " +
		"guest console only"
	client := &http.Client{Timeout: 30 * time.Second}
	buf := make([]byte, 32*1024)
	deliver := gateway != ""
	for {
		n, rerr := pr.Read(buf)
		if n > 0 && deliver {
			if !postBuildOutputChunk(client, gateway, buf[:n]) {
				deliver = false
				fmt.Fprintln(os.Stderr, disabledMsg)
			}
		}
		if rerr != nil {
			return
		}
	}
}

// heartbeat pings the relay every 2s until done is closed, so the relay knows
// the build is still alive.
func heartbeat(done <-chan struct{}) {
	client := &http.Client{Timeout: 2 * time.Second}
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			resp, err := client.Get("http://artifacts/heartbeat")
			if err == nil {
				resp.Body.Close()
			}
		}
	}
}

// postBuildOutputChunk streams one chunk of build output to the relay's
// control-plane /v1/output endpoint (gateway:8300), which forwards it to the
// collector's build-output.log. Returns false on failure so the caller can stop
// delivering (and just drain) rather than stalling the build on a dead link.
// Best-effort by design: a dropped chunk weakens the convenience log but never
// fails the build.
func postBuildOutputChunk(client *http.Client, gateway string, data []byte) bool {
	if gateway == "" || len(data) == 0 {
		return false
	}
	url := fmt.Sprintf("http://%s:8300/v1/output", gateway)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		fmt.Fprintf(os.Stderr, "warden-io: post build output: %s\n", err)
		return false
	}
	req.Header.Set("Content-Type", "text/plain")
	resp, err := client.Do(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warden-io: post build output: %s\n", err)
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "warden-io: post build output: HTTP %d\n", resp.StatusCode)
		return false
	}
	return true
}

func logStep(msg string) {
	fmt.Fprintf(os.Stderr, "warden-io: %s\n", msg)
}

// reportComplete notifies the relay that the build is finished.
func reportComplete(code int) {
	url := fmt.Sprintf("http://artifacts/exit?code=%d", code)
	resp, err := http.Post(url, "", strings.NewReader(""))
	if err == nil {
		resp.Body.Close()
	}
}

func parseInitArgs(args []string) initArgs {
	var a initArgs
	str := map[string]*string{
		"--gateway": &a.gateway, "--ip": &a.selfIP,
		"--script": &a.script, "--workdir": &a.workdir,
	}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--hold" {
			a.hold = true
			continue
		}
		name, val, hasVal := strings.Cut(arg, "=")
		dst, ok := str[name]
		if !ok {
			continue
		}
		if !hasVal {
			i++
			if i >= len(args) {
				break
			}
			val = args[i]
		}
		*dst = val
	}
	return a
}
