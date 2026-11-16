package driver

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync/atomic"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/buildwarden/buildwarden/bootstrap"
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
//	id, _ := s.ClaimRelay(ctx, addr, tok, req, nil, link, fix) // tunnel up, relay configured
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
	// GuestOS picks the bootstrap config the collector serves
	// (bootstrap.ForGuest; empty means a Linux guest).
	GuestOS string
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
		logf = func(f string, a ...any) {
			fmt.Fprintf(os.Stderr, "[warden] "+f+"\n", a...)
		}
	}
	col, err := collector.New(collector.Config{
		OutputDir:       cfg.OutputDir,
		Token:           token,
		ContextDir:      cfg.ContextDir,
		BuildScriptPath: cfg.BuildScript,
		Bootstrap:       bootstrap.ForGuest(cfg.GuestOS).Marshal(),
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
// (the container driver's base image manifest). The upstream CA certificates
// it sends are the request's, read before the build started; it opens no
// file for them.
//
// link is the build link the relay configures its build segment from (nil
// sends none: the container driver, and a claimant whose relay is already
// addressed). req.DNS, when set, is sent as upstream_dns, in order. A relay
// too old to know a claim field refuses the claim naming it; that refusal is
// returned as a FixableError with oldRelayFix, before the caller boots any
// build guest.
func (s *Session) ClaimRelay(ctx context.Context, addr, claimToken string,
	req *BuildRequest, env *relaycfg.Environment,
	link *relaycfg.BuildLink, oldRelayFix string) (relaycfg.Identity, error) {
	cfg := relaycfg.Config{
		CollectorURL:   relaycfg.TunnelURL,
		CollectorToken: s.Token,
		CaptureMode:    req.CaptureMode,
		Environment:    env,
		BuildLink:      link,
	}
	for _, a := range req.DNS {
		cfg.UpstreamDNS = append(cfg.UpstreamDNS, a.String())
	}
	if len(req.UpstreamCACerts) > 0 {
		cfg.UpstreamCACerts = []string{string(req.UpstreamCACerts)}
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
		if field := unknownClaimField(err); field != "" {
			return relaycfg.Identity{}, Fixable(oldRelayFix,
				"the relay predates the claim field %s (%v)", field, err)
		}
		return relaycfg.Identity{}, fmt.Errorf("claiming relay: %w", err)
	}
	select {
	case <-s.Collector.Ready():
	case <-end:
		return relaycfg.Identity{},
			fmt.Errorf("relay tunnel closed before the relay was ready")
	case <-ctx.Done():
		return relaycfg.Identity{},
			fmt.Errorf("relay was claimed but never reported ready")
	}
	caPath := filepath.Join(s.outputDir, "ca.cert.pem")
	if err := os.WriteFile(caPath, []byte(id.CACert), 0o644); err != nil {
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
		var ue *collector.UnresponsiveError
		switch {
		case errors.As(err, &ue):
			// Usually the build itself hung or killed the agent, not warden:
			// point at what the guest last printed rather than a bug report.
			return Fixable(
				"see "+filepath.Join(s.outputDir, "build-output.log")+
					" for the last thing the build printed",
				"build failed: %v", err)
		case tunnelLost.Load():
			return fmt.Errorf("build failed: lost the connection to the relay")
		}
		return err
	}
	return exitErr(sig)
}

// ExitError reports a build (or shell) that ran to completion and exited
// non-zero. Code is the build script's own exit code, which warden exits
// with; it is an expected outcome, not a warden failure.
type ExitError struct {
	Code   int
	Detail string
}

func (e *ExitError) Error() string {
	if e.Detail != "" {
		return fmt.Sprintf("build exited with code %d: %s", e.Code, e.Detail)
	}
	return fmt.Sprintf("build exited with code %d", e.Code)
}

func exitErr(sig collector.CompletionSignal) error {
	if sig.ExitCode == 0 {
		return nil
	}
	return &ExitError{Code: sig.ExitCode, Detail: firstNonEmpty(sig.Error, sig.Message)}
}

// Close tears down the relay tunnel.
func (s *Session) Close() {
	if s.tunnel != nil {
		_ = s.tunnel.Close()
		// Let the tunnel's server wind down before the collector's
		// directory handles go away under any request it is finishing.
		select {
		case <-s.tunnelEnd:
		case <-time.After(2 * time.Second):
		}
	}
	_ = s.Collector.Close()
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

// Where upstream CA paths were configured, as LoadUpstreamCAs' lines and
// errors name it.
const (
	CAFromConfig = "[relay] upstream_ca_certs"
	CAFromEnv    = "WARDEN_UPSTREAM_CA_CERTS"
	CAFromFlag   = "--upstream-ca"
)

// CAReport says what LoadUpstreamCAs found under the configured paths, as
// the lines its caller prints: every block and file that was skipped, with
// the reason, and one line per configured path with its counts.
type CAReport struct {
	lines []caLine
}

type caLine struct {
	text string
	// detail marks a line of the per-file list: a file that gives a
	// certificate, or an entry that was not read because of what it is.
	detail bool
}

// Lines returns the report's lines in the order they were found, without a
// prefix. A skip and a path's own line are always among them; verbose adds
// the per-file list. The line for a private key starts with "warning:".
func (r *CAReport) Lines(verbose bool) []string {
	if r == nil {
		return nil
	}
	var out []string
	for _, l := range r.lines {
		if verbose || !l.detail {
			out = append(out, l.text)
		}
	}
	return out
}

func (r *CAReport) add(detail bool, format string, args ...any) {
	r.lines = append(r.lines, caLine{text: fmt.Sprintf(format, args...), detail: detail})
}

// LoadUpstreamCAs reads the configured upstream CA paths, each a file or a
// directory, and returns the valid PEM certificates found under them,
// re-encoded from their DER bytes, so that no other byte of a file is in
// what is returned: not a private key, not another block, not the text
// between blocks. source says where the paths were configured (CAFromConfig,
// CAFromEnv or CAFromFlag).
//
// What is not a valid PEM certificate is skipped, not refused, and the
// report has a line for each skip: the run goes on with the certificates the
// path does give. That is a continuation on failure, allowed because it is
// said on every run (GLOBAL-10, GLOBAL-11). A configured path that does not
// exist, cannot be read or gives no certificate at all is an error; the
// paths are taken in order and the first error stops. The report is returned
// with the error and holds the lines up to it.
//
// A path named directly is read whatever kind of file it is (a pipe is how a
// bundle is handed over without a file). A directory is read one level deep:
// its .pem, .crt and .cer files, in name order, links followed. An entry
// there that is not a regular file is never opened, because nothing named
// it.
//
// Every string in a line or an error that is not warden's own is escaped
// (EscapeOutside): each can come from the repository being built.
func LoadUpstreamCAs(paths []string, source string) ([]byte, *CAReport, error) {
	report := &CAReport{}
	var certs []byte
	for _, p := range paths {
		got, err := report.loadPath(p, source)
		if err != nil {
			return nil, report, err
		}
		certs = append(certs, got...)
	}
	return certs, report, nil
}

// loadPath reads one configured path.
func (r *CAReport) loadPath(p, source string) ([]byte, error) {
	if p == "" {
		fix := "remove the doubled or trailing separator"
		if source == CAFromConfig {
			fix = `remove the "" entry from the list`
		}
		// Never read as the working directory, which is what an empty entry
		// of PATH means.
		return nil, Fixable(fix, "%s has an empty entry", source)
	}
	name := p
	if abs, err := filepath.Abs(p); err == nil {
		name = abs
	}
	shown := EscapeOutside(name)
	const noCertFix = "name a file or directory that holds at least one PEM " +
		"certificate, or remove the path"
	info, err := os.Stat(p)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return nil, Fixable("name an existing file or directory of PEM "+
			"certificates, or remove the path",
			"upstream CA path %s (from %s) does not exist", shown, source)
	case err != nil:
		return nil, unreadableCAPath(shown, source, err)
	case info.IsDir():
		return r.loadDir(p, name, source)
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return nil, unreadableCAPath(shown, source, err)
	}
	file := scanCAFile(data)
	r.blockLines(shown, file)
	if file.certs == 0 {
		fix := noCertFix
		if file.der {
			fix = "convert it to PEM (" + derToPEM + ") and name the result, " +
				"or remove the path"
		}
		return nil, Fixable(fix, "upstream CA path %s (from %s) gives no "+
			"certificate to send: %s", shown, source, file.nothing("it is ",
			"there is ", "it holds "))
	}
	r.add(false, "upstream CA %s (%s): %s", shown, source, file.toSend())
	return file.pem, nil
}

// loadDir reads a configured path that is a directory; name is its absolute
// path.
func (r *CAReport) loadDir(p, name, source string) ([]byte, error) {
	shown := EscapeOutside(name)
	entries, err := os.ReadDir(p)
	if err != nil {
		return nil, unreadableCAPath(shown, source, err)
	}
	var certs []byte
	var sent, gave, skipped, other int
	for _, e := range entries {
		entry := EscapeOutside(filepath.Join(name, e.Name()))
		file, why := readCAEntry(p, e)
		switch {
		case why.notRead != "":
			other++
			r.add(true, "upstream CA %s: not read: %s", entry, why.notRead)
		case why.skipped != "":
			skipped++
			r.add(false, "upstream CA %s: skipped: %s", entry, why.skipped)
		case file.certs == 0:
			skipped++
			r.blockLines(entry, file)
			r.add(false, "upstream CA %s: skipped: %s", entry,
				file.nothing("", "", "it holds "))
		default:
			gave++
			sent += file.certs
			certs = append(certs, file.pem...)
			r.blockLines(entry, file)
			r.add(true, "upstream CA %s: %s", entry, file.toSend())
		}
	}
	notSent := joinNonEmpty(", ",
		counted(skipped, "file skipped", "files skipped"),
		counted(other, "other entry not read", "other entries not read"))
	if sent == 0 {
		if gave+skipped == 0 {
			notSent = "it holds no .pem, .crt or .cer file"
		}
		return nil, Fixable("put at least one PEM certificate in a .pem, .crt "+
			"or .cer file there, or remove the path",
			"upstream CA path %s (from %s) gives no certificate to send: %s",
			shown, source, notSent)
	}
	r.add(false, "upstream CA %s (%s): %s", shown, source, joinNonEmpty("; ",
		counted(sent, "certificate to send", "certificates to send")+" from "+
			counted(gave, "file", "files"),
		counted(skipped, "file skipped", "files skipped"),
		counted(other, "other entry not read", "other entries not read")))
	return certs, nil
}

// caEntryNote says why an entry of a CA directory gave no file to scan:
// notRead for an entry that is not one of the directory's CA files (a12),
// skipped for a CA file that could not be read (a13). Both empty: it was
// read.
type caEntryNote struct {
	notRead, skipped string
}

// readCAEntry reads one entry of the CA directory dir when it is one of the
// directory's CA files: its name ends in .pem, .crt or .cer, in any letter
// case, and it is a regular file or a link to one.
func readCAEntry(dir string, e fs.DirEntry) (caFile, caEntryNote) {
	const aDirectory = "a directory"
	if e.IsDir() {
		return caFile{}, caEntryNote{notRead: aDirectory}
	}
	switch strings.ToLower(filepath.Ext(e.Name())) {
	case ".pem", ".crt", ".cer":
	default:
		return caFile{}, caEntryNote{
			notRead: "its name does not end in .pem, .crt or .cer"}
	}
	path := filepath.Join(dir, e.Name())
	info, err := os.Stat(path)
	switch {
	case errors.Is(err, fs.ErrNotExist) && e.Type()&fs.ModeSymlink != 0:
		return caFile{}, caEntryNote{skipped: "a link to a file that does not exist"}
	case err != nil:
		return caFile{}, caEntryNote{skipped: "cannot be read (" + osReason(err) + ")"}
	case info.IsDir():
		return caFile{}, caEntryNote{notRead: aDirectory}
	case !info.Mode().IsRegular():
		// A FIFO here would block the open for ever, and nothing named it.
		return caFile{}, caEntryNote{skipped: "not a regular file"}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return caFile{}, caEntryNote{skipped: "cannot be read (" + osReason(err) + ")"}
	}
	return scanCAFile(data), caEntryNote{}
}

func unreadableCAPath(shown, source string, err error) error {
	return Fixable("make it readable by the user warden runs as, or remove the path",
		"cannot read upstream CA path %s (from %s): %s", shown, source, osReason(err))
}

// derToPEM is the conversion a DER file's line and error suggest. <file>
// stands as it is: a file's name can come from the repository being built,
// and a command to paste that carried it would carry whatever the name holds
// ($(...), a ;) into a shell.
const derToPEM = "openssl x509 -inform der -in <file> -out <file>.pem"

// caFile is what one file under a CA path holds.
type caFile struct {
	pem    []byte // its valid certificates, re-encoded
	certs  int
	blocks int       // its PEM blocks, sent or not
	skips  []caBlock // the blocks that are not sent
	der    bool      // no PEM block, and the whole file is one DER certificate
}

// caBlock is a block that is not sent: its number in the file, from 1, and
// the line's text after the file's name.
type caBlock struct {
	text    string
	warning bool
}

// toSend words what a file gives: its certificates, and its blocks that are
// not sent.
func (f caFile) toSend() string {
	return joinNonEmpty("; ",
		counted(f.certs, "certificate to send", "certificates to send"),
		counted(len(f.skips), "block skipped", "blocks skipped"))
}

// nothing says why a file gives no certificate, after the words the caller's
// sentence needs before each of the three reasons.
func (f caFile) nothing(beforeDER, beforeNoBlock, beforeBlocks string) string {
	switch {
	case f.der:
		if beforeDER == "" {
			return "DER-encoded, and only PEM is read (to convert: " + derToPEM + ")"
		}
		return beforeDER + "DER-encoded, and only PEM is read"
	case f.blocks == 0:
		return beforeNoBlock + "no PEM block in it"
	}
	return beforeBlocks + counted(f.blocks, "PEM block", "PEM blocks") +
		" and none is a certificate that can be sent"
}

// blockLines adds the line of each block of file that is not sent.
func (r *CAReport) blockLines(shown string, file caFile) {
	for _, b := range file.skips {
		if b.warning {
			r.add(false, "warning: upstream CA %s: %s", shown, b.text)
		} else {
			r.add(false, "upstream CA %s: %s", shown, b.text)
		}
	}
}

var (
	pemBegin  = []byte("-----BEGIN ")
	pemEnd    = []byte("-----END ")
	pemDashes = []byte("-----")
)

// scanCAFile finds the certificates to send in one file's bytes and accounts
// for every other PEM block in it.
//
// A block starts at each line that begins with "-----BEGIN " and runs to the
// next such line or the end of the file. The definition does not lean on
// encoding/pem's decoder, which passes over a damaged block without a word
// and returns the next whole one: every BEGIN line is accounted for. A block
// is whole when that decoder decodes it, and is sent when it is a
// CERTIFICATE without PEM headers that parses, which are the conditions of
// the relay's trust pool (x509.CertPool.AppendCertsFromPEM); so the
// certificates sent are the ones the relay would take from the file.
func scanCAFile(data []byte) caFile {
	var f caFile
	for _, segment := range pemSegments(data) {
		f.blocks++
		skip := func(warning bool, format string, args ...any) {
			text := fmt.Sprintf("block %d ", f.blocks) + fmt.Sprintf(format, args...)
			f.skips = append(f.skips, caBlock{text: text, warning: warning})
		}
		const keyNotSent = " and is not sent to the relay; a private key does " +
			"not belong in a file of trust roots"
		block, _ := pem.Decode(segment)
		if block == nil {
			typ, why := pemDamage(segment)
			switch {
			case typ == nil:
				skip(false, "skipped: a damaged PEM block (%s)", why)
			case isPrivateKey(*typ):
				skip(true, "is a damaged %s (%s)%s", blockType(*typ), why,
					keyNotSent)
			default:
				skip(false, "skipped: a damaged %s block (%s)",
					blockType(*typ), why)
			}
			continue
		}
		switch {
		case isPrivateKey(block.Type):
			skip(true, "is %s %s%s", article(block.Type), blockType(block.Type),
				keyNotSent)
		case block.Type != "CERTIFICATE":
			skip(false, "skipped: %s %s block, not a CERTIFICATE",
				article(block.Type), blockType(block.Type))
		case len(block.Headers) != 0:
			skip(false, "skipped: a CERTIFICATE block with PEM headers, which "+
				"the relay's trust pool does not take")
		default:
			if _, err := x509.ParseCertificate(block.Bytes); err != nil {
				skip(false, "skipped: a CERTIFICATE that does not parse (%s)",
					EscapeOutside(err.Error()))
				continue
			}
			f.certs++
			f.pem = append(f.pem, pem.EncodeToMemory(
				&pem.Block{Type: "CERTIFICATE", Bytes: block.Bytes})...)
		}
	}
	if f.blocks == 0 {
		_, err := x509.ParseCertificate(data)
		f.der = err == nil
	}
	return f
}

// pemSegments cuts data at each line that begins with "-----BEGIN ".
func pemSegments(data []byte) [][]byte {
	var starts []int
	for at := 0; at < len(data); {
		if bytes.HasPrefix(data[at:], pemBegin) {
			starts = append(starts, at)
		}
		nl := bytes.IndexByte(data[at:], '\n')
		if nl < 0 {
			break
		}
		at += nl + 1
	}
	segments := make([][]byte, len(starts))
	for i, start := range starts {
		end := len(data)
		if i+1 < len(starts) {
			end = starts[i+1]
		}
		segments[i] = data[start:end]
	}
	return segments
}

// pemDamage says why encoding/pem does not decode a block: the first of its
// reasons that holds, in the decoder's own order. typ is the type on the
// block's BEGIN line, nil when that line is not complete.
func pemDamage(segment []byte) (typ *string, why string) {
	typeLine, rest := pemLine(segment[len(pemBegin):])
	if !bytes.HasSuffix(typeLine, pemDashes) {
		return nil, "its BEGIN line is not complete"
	}
	t := string(typeLine[:len(typeLine)-len(pemDashes)])
	const noEnd = "it has no END line"
	headers := false
	for {
		if len(rest) == 0 {
			return &t, noEnd
		}
		line, next := pemLine(rest)
		if !bytes.Contains(line, []byte(":")) {
			break
		}
		headers, rest = true, next
	}
	var endLine []byte
	if !headers && bytes.HasPrefix(rest, pemEnd) {
		endLine, _ = pemLine(rest[len(pemEnd):])
	} else {
		_, after, found := bytes.Cut(rest, append([]byte("\n"), pemEnd...))
		if !found {
			return &t, noEnd
		}
		endLine, _ = pemLine(after)
	}
	if string(endLine) != t+string(pemDashes) {
		return &t, "its END line is for another type"
	}
	return &t, "its body is not base64"
}

// pemLine returns the first line of data without its line ending and
// trailing blanks, and what follows it, as encoding/pem reads a line.
func pemLine(data []byte) (line, rest []byte) {
	i, j := len(data), len(data)
	if nl := bytes.IndexByte(data, '\n'); nl >= 0 {
		i, j = nl, nl+1
		if i > 0 && data[i-1] == '\r' {
			i--
		}
	}
	return bytes.TrimRight(data[:i], " \t"), data[j:]
}

func isPrivateKey(blockType string) bool {
	return strings.HasSuffix(blockType, "PRIVATE KEY")
}

// blockType is a block's type as a line prints it.
func blockType(typ string) string {
	if typ == "" {
		return "PEM"
	}
	return EscapeOutside(typ)
}

// article is "an" before a block type that is read aloud with a vowel first:
// one that starts with a vowel, or with an abbreviation spelled out letter
// by letter (an RSA PRIVATE KEY, an X509 CRL).
func article(typ string) string {
	first, _, _ := strings.Cut(typ, " ")
	switch {
	case first == "RSA", first == "X509", first == "SSH2":
		return "an"
	case typ != "" && strings.ContainsRune("AEIOU", rune(typ[0])):
		return "an"
	}
	return "a"
}

// counted is n with the noun that agrees with it, or "" for none.
func counted(n int, one, many string) string {
	switch n {
	case 0:
		return ""
	case 1:
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}

func joinNonEmpty(sep string, parts ...string) string {
	var kept []string
	for _, p := range parts {
		if p != "" {
			kept = append(kept, p)
		}
	}
	return strings.Join(kept, sep)
}

// osReason is the operating system's reason for a failed file operation,
// without the operation and the path that *fs.PathError puts before it.
func osReason(err error) string {
	var pe *fs.PathError
	if errors.As(err, &pe) {
		err = pe.Err
	}
	return EscapeOutside(err.Error())
}

// EscapeOutside makes a string that did not come from warden safe to print
// in a line or an error: a configured path, a directory entry's name, a PEM
// block's type, the X.509 parser's reason, the operating system's. Control
// characters, other characters that are not printable and bytes that are not
// valid UTF-8 are written as escapes (\n, \x1b, \u202e), so none of them
// can move the cursor, clear a line or start a line of its own in a terminal
// or a log.
func EscapeOutside(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		switch {
		case r == utf8.RuneError && size == 1:
			fmt.Fprintf(&b, `\x%02x`, s[i])
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\r':
			b.WriteString(`\r`)
		case r == '\t':
			b.WriteString(`\t`)
		case unicode.IsPrint(r):
			b.WriteRune(r)
		case r < 0x100:
			fmt.Fprintf(&b, `\x%02x`, r)
		case r < 0x10000:
			fmt.Fprintf(&b, `\u%04x`, r)
		default:
			fmt.Fprintf(&b, `\U%08x`, r)
		}
		i += size
	}
	return b.String()
}

// RandomToken returns a 256-bit hex token.
func RandomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// unknownFieldRE matches the decoder's refusal of a claim field the relay
// does not know.
var unknownFieldRE = regexp.MustCompile(`unknown field "([^"]+)"`)

// unknownClaimField returns the claim field a relay refused as unknown, or ""
// when err is not such a refusal.
func unknownClaimField(err error) string {
	if !errors.Is(err, relaycfg.ErrRejected) {
		return ""
	}
	if m := unknownFieldRE.FindStringSubmatch(err.Error()); m != nil {
		return m[1]
	}
	return ""
}

// OldRelayFix is the old-relay fix for drivers that append warden's own relay
// binary to the relay per build (qemu, Firecracker, vz): a relay that predates
// a claim field (build_link, upstream_dns) means a mismatched install.
const OldRelayFix = "reinstall warden so its relay binary matches it " +
	"(in a source tree: make build)"

// FreeLoopbackPort reserves and releases a loopback TCP port, for forwarding
// a relay's claim port to the host. The tiny reuse race is acceptable: a
// collision fails the claim loudly rather than misrouting it.
func FreeLoopbackPort() (int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer ln.Close()
	addr, ok := ln.Addr().(*net.TCPAddr)
	if !ok {
		return 0, fmt.Errorf("unexpected listener address type %T", ln.Addr())
	}
	return addr.Port, nil
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
	if translates(req) {
		res, err := script.Translate(req.Containerfile, req.ContextDir)
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

// translates reports whether StageBuildScript builds the script by
// translating the request's Containerfile, the one way it alters an input.
func translates(req *BuildRequest) bool {
	return req.Script == "" && !strings.EqualFold(req.GuestOS, "windows") &&
		req.Containerfile != ""
}

// RecordInputs keeps an altered input next to what it became (ORCH-02): when
// the script was translated from the Containerfile, the submitted file goes to
// <output>/inputs/Containerfile, whatever it was called, and the staged script
// to <output>/inputs/build.sh. req is the request given to StageBuildScript,
// and script the path it returned. Drivers call it straight after staging,
// before the build starts, so a failed build keeps its inputs and a failure to
// record fails the build before anything runs. An input run as given is not
// recorded.
func RecordInputs(outputDir string, req *BuildRequest, script string) error {
	if !translates(req) {
		return nil
	}
	dir := filepath.Join(outputDir, "inputs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := CopyFile(req.Containerfile, filepath.Join(dir, "Containerfile")); err != nil {
		return err
	}
	return CopyFile(script, filepath.Join(dir, "build.sh"))
}

func fileExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}
