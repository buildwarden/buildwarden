//go:build unix

package driver

import (
	"bytes"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// The cases of F2 that need a kind of file Windows does not make the same
// way: a FIFO, a socket, a link that points at itself, a name holding a
// control character, a file the process may not read.

// TestLoadUpstreamCAsFIFONamedDirectly: a path named directly is read
// whatever kind of file it is, as <(cat a.pem) is.
func TestLoadUpstreamCAsFIFONamedDirectly(t *testing.T) {
	a := newTestCert(t, "a")
	p := filepath.Join(t.TempDir(), "bundle")
	if err := syscall.Mkfifo(p, 0o600); err != nil {
		t.Skipf("no FIFO here: %v", err)
	}
	go func() {
		w, err := os.OpenFile(p, os.O_WRONLY, 0)
		if err != nil {
			return
		}
		_, _ = w.Write(join(a.pem, []byte("trailing text\n")))
		_ = w.Close()
	}()
	got, report, err := LoadUpstreamCAs([]string{p}, CAFromEnv)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, a.pem) {
		t.Errorf("returned %q", got)
	}
	assertLines(t, report.Lines(false), []string{
		"upstream CA " + p + " (WARDEN_UPSTREAM_CA_CERTS): 1 certificate to send"})
}

// TestLoadUpstreamCAsDirectoryEntriesNotOpened: in a directory, a FIFO and a
// socket under a CA ending are skipped and never opened (an open of the
// FIFO would wait for ever), and a link that points at itself is a skip
// with the operating system's reason. The directory still gives its
// certificate.
func TestLoadUpstreamCAsDirectoryEntriesNotOpened(t *testing.T) {
	a := newTestCert(t, "a")
	dir := t.TempDir()
	writeCAFile(t, dir, "a.pem", a.pem)
	fifo := filepath.Join(dir, "pipe.pem")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Skipf("no FIFO here: %v", err)
	}
	sock := filepath.Join(dir, "s.crt")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Skipf("no Unix socket here: %v", err)
	}
	defer func() { _ = ln.Close() }()
	loop := filepath.Join(dir, "loop.cer")
	symlink(t, loop, loop)

	type result struct {
		got    []byte
		report *CAReport
		err    error
	}
	done := make(chan result, 1)
	go func() {
		got, report, err := LoadUpstreamCAs([]string{dir}, CAFromConfig)
		done <- result{got, report, err}
	}()
	var r result
	select {
	case r = <-done:
	case <-time.After(10 * time.Second):
		// Release the open that should never have happened.
		if w, err := os.OpenFile(fifo, os.O_WRONLY, 0); err == nil {
			_ = w.Close()
		}
		t.Fatal("the FIFO in the directory was opened")
	}
	if r.err != nil {
		t.Fatal(r.err)
	}
	if !bytes.Equal(r.got, a.pem) {
		t.Errorf("returned %q", r.got)
	}
	at := func(name string) string { return "upstream CA " + filepath.Join(dir, name) }
	assertLines(t, r.report.Lines(false), []string{
		at("loop.cer") + ": skipped: cannot be read (" + syscall.ELOOP.Error() + ")",
		at("pipe.pem") + ": skipped: not a regular file",
		at("s.crt") + ": skipped: not a regular file",
		"upstream CA " + dir + " ([relay] upstream_ca_certs): 1 certificate to send " +
			"from 1 file; 3 files skipped",
	})
}

// TestLoadUpstreamCAsCannotRead: a path named directly that cannot be
// opened is the error with the operating system's reason. Both rows run as
// root: a link that points at itself, and a socket, whose open fails.
func TestLoadUpstreamCAsCannotRead(t *testing.T) {
	dir := t.TempDir()
	loop := filepath.Join(dir, "loop.pem")
	symlink(t, loop, loop)
	sock := filepath.Join(dir, "s.pem")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Skipf("no Unix socket here: %v", err)
	}
	defer func() { _ = ln.Close() }()
	_, sockErr := os.ReadFile(sock)
	if sockErr == nil {
		t.Skip("a socket can be read here")
	}
	for _, tc := range []struct{ path, reason string }{
		{loop, syscall.ELOOP.Error()},
		{sock, osReason(sockErr)},
	} {
		_, _, err := LoadUpstreamCAs([]string{tc.path}, CAFromEnv)
		assertCannotRead(t, err, tc.path, tc.reason)
	}
}

func assertCannotRead(t *testing.T, err error, path, reason string) {
	t.Helper()
	var fe *FixableError
	if !errors.As(err, &fe) {
		t.Fatalf("error %v is not a FixableError", err)
	}
	want := "cannot read upstream CA path " + path + " (from " +
		"WARDEN_UPSTREAM_CA_CERTS): " + reason
	if fe.Msg != want {
		t.Errorf("message:\n%s\nwant:\n%s", fe.Msg, want)
	}
	if fe.Fix != "make it readable by the user warden runs as, or remove the path" {
		t.Errorf("fix: %s", fe.Fix)
	}
}

// TestLoadUpstreamCAsPermissionDenied is the same at mode 0, which denies
// nothing to root: run as another user, as CI is.
func TestLoadUpstreamCAsPermissionDenied(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads a file at mode 0; the rows that run as root are " +
			"TestLoadUpstreamCAsCannotRead and " +
			"TestLoadUpstreamCAsDirectoryEntriesNotOpened")
	}
	a := newTestCert(t, "a")
	denied := syscall.EACCES.Error()

	file := writeCAFile(t, t.TempDir(), "site.pem", a.pem)
	if err := os.Chmod(file, 0); err != nil {
		t.Fatal(err)
	}
	_, _, err := LoadUpstreamCAs([]string{file}, CAFromEnv)
	assertCannotRead(t, err, file, denied)

	unlisted := t.TempDir()
	if err := os.Chmod(unlisted, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(unlisted, 0o700) })
	_, _, err = LoadUpstreamCAs([]string{unlisted}, CAFromEnv)
	assertCannotRead(t, err, unlisted, denied)

	dir := t.TempDir()
	writeCAFile(t, dir, "a.pem", a.pem)
	if err := os.Chmod(writeCAFile(t, dir, "site.pem", a.pem), 0); err != nil {
		t.Fatal(err)
	}
	got, report, err := LoadUpstreamCAs([]string{dir}, CAFromEnv)
	if err != nil || !bytes.Equal(got, a.pem) {
		t.Fatalf("got %q, %v", got, err)
	}
	assertLines(t, report.Lines(false), []string{
		"upstream CA " + filepath.Join(dir, "site.pem") + ": skipped: cannot be read " +
			"(" + denied + ")",
		"upstream CA " + dir + " (WARDEN_UPSTREAM_CA_CERTS): 1 certificate to send from " +
			"1 file; 1 file skipped",
	})
}

// TestLoadUpstreamCAsEscapesNames: an entry's name that holds a line break
// or an escape sequence is printed escaped.
func TestLoadUpstreamCAsEscapesNames(t *testing.T) {
	a := newTestCert(t, "a")
	dir := t.TempDir()
	writeCAFile(t, dir, "a.pem", a.pem)
	writeCAFile(t, dir, "x\n[warden] all good\x1b[2K.crt", []byte("x"))
	_, report, err := LoadUpstreamCAs([]string{dir}, CAFromConfig)
	if err != nil {
		t.Fatal(err)
	}
	lines := report.Lines(true)
	want := "upstream CA " + filepath.Join(dir, `x\n[warden] all good\x1b[2K.crt`) + ": " +
		"skipped: no PEM block in it"
	if !strings.Contains(strings.Join(lines, "\n"), want) {
		t.Errorf("no line %q in %q", want, lines)
	}
	for _, l := range lines {
		if strings.ContainsAny(l, "\n\x1b") {
			t.Errorf("a control character reached a line: %q", l)
		}
	}
}
