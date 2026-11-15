package driver

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// testCert is a self-signed certificate made for one test: its DER bytes and
// its PEM encoding.
type testCert struct {
	der []byte
	pem []byte
}

func newTestCert(t *testing.T, name string) testCert {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(time.Now().UnixNano()),
		Subject:               pkix.Name{CommonName: name},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return testCert{der: der, pem: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})}
}

// pemBlock encodes body under typ, as a file of keys or requests holds it.
func pemBlock(typ string, body []byte) []byte {
	return pem.EncodeToMemory(&pem.Block{Type: typ, Bytes: body})
}

// keyBytes stands for a private key's body: what must never leave the host.
var keyBytes = []byte("SECRET-KEY-MATERIAL-0123456789")

func writeCAFile(t *testing.T, dir, name string, data []byte) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func join(parts ...[]byte) []byte { return bytes.Join(parts, nil) }

// assertSameAsPool checks that the certificates returned for data are the
// ones the relay's trust pool takes from the same bytes.
func assertSameAsPool(t *testing.T, data, got []byte) {
	t.Helper()
	want := x509.NewCertPool()
	want.AppendCertsFromPEM(data)
	have := x509.NewCertPool()
	have.AppendCertsFromPEM(got)
	if !want.Equal(have) {
		t.Errorf("the certificates returned are not the ones AppendCertsFromPEM takes from the file")
	}
}

// TestLoadUpstreamCAsFile is F2's rule for one file named directly: what is
// returned, every line, and the agreement with the relay's pool.
func TestLoadUpstreamCAsFile(t *testing.T) {
	a, b := newTestCert(t, "a"), newTestCert(t, "b")
	unparsable := pemBlock("CERTIFICATE", []byte("not a certificate"))
	_, parseErr := x509.ParseCertificate([]byte("not a certificate"))
	headers := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE",
		Headers: map[string]string{"Proc-Type": "4,ENCRYPTED"}, Bytes: b.der})

	tests := []struct {
		name  string
		data  []byte
		certs [][]byte // DER of what is returned, in order
		lines []string // after "upstream CA <path>"
	}{
		{"one certificate", a.pem, [][]byte{a.der},
			[]string{" ([relay] upstream_ca_certs): 1 certificate to send"}},
		{"text between blocks is not sent",
			join([]byte("# Corp root\n"), a.pem, []byte("# Second\n"), b.pem),
			[][]byte{a.der, b.der},
			[]string{" ([relay] upstream_ca_certs): 2 certificates to send"}},
		{"an RSA key beside a certificate",
			join(a.pem, pemBlock("RSA PRIVATE KEY", keyBytes)), [][]byte{a.der},
			[]string{
				"warning: block 2 is an RSA PRIVATE KEY and is not sent to the relay; a private key does not belong in a file of trust roots",
				" ([relay] upstream_ca_certs): 1 certificate to send; 1 block skipped"}},
		{"a request", join(a.pem, pemBlock("CERTIFICATE REQUEST", []byte("csr"))),
			[][]byte{a.der}, []string{
				": block 2 skipped: a CERTIFICATE REQUEST block, not a CERTIFICATE",
				" ([relay] upstream_ca_certs): 1 certificate to send; 1 block skipped"}},
		{"a certificate with PEM headers", join(a.pem, headers), [][]byte{a.der},
			[]string{
				": block 2 skipped: a CERTIFICATE block with PEM headers, which the relay's trust pool does not take",
				" ([relay] upstream_ca_certs): 1 certificate to send; 1 block skipped"}},
		{"a certificate that does not parse", join(unparsable, a.pem),
			[][]byte{a.der}, []string{
				": block 1 skipped: a CERTIFICATE that does not parse (" + parseErr.Error() + ")",
				" ([relay] upstream_ca_certs): 1 certificate to send; 1 block skipped"}},
		{"a body that is not base64, in the middle",
			join(a.pem, []byte("-----BEGIN CERTIFICATE-----\n!!!!\n-----END CERTIFICATE-----\n"), b.pem),
			[][]byte{a.der, b.der}, []string{
				": block 2 skipped: a damaged CERTIFICATE block (its body is not base64)",
				" ([relay] upstream_ca_certs): 2 certificates to send; 1 block skipped"}},
		{"an END line for another type",
			join(a.pem, []byte("-----BEGIN CERTIFICATE-----\nAAAA\n-----END X509 CRL-----\n"), b.pem),
			[][]byte{a.der, b.der}, []string{
				": block 2 skipped: a damaged CERTIFICATE block (its END line is for another type)",
				" ([relay] upstream_ca_certs): 2 certificates to send; 1 block skipped"}},
		{"no END line, in the middle",
			join(a.pem, []byte("-----BEGIN CERTIFICATE-----\nAAAA\n"), b.pem),
			[][]byte{a.der, b.der}, []string{
				": block 2 skipped: a damaged CERTIFICATE block (it has no END line)",
				" ([relay] upstream_ca_certs): 2 certificates to send; 1 block skipped"}},
		{"no END line, at the end",
			join(a.pem, []byte("-----BEGIN CERTIFICATE-----\nAAAA\n")),
			[][]byte{a.der}, []string{
				": block 2 skipped: a damaged CERTIFICATE block (it has no END line)",
				" ([relay] upstream_ca_certs): 1 certificate to send; 1 block skipped"}},
		{"a BEGIN line that is not complete",
			join([]byte("-----BEGIN CERTIFICATE\nAAAA\n-----END CERTIFICATE-----\n"), a.pem),
			[][]byte{a.der}, []string{
				": block 1 skipped: a damaged PEM block (its BEGIN line is not complete)",
				" ([relay] upstream_ca_certs): 1 certificate to send; 1 block skipped"}},
		{"a damaged key",
			join(a.pem, []byte("-----BEGIN RSA PRIVATE KEY-----\n"+string(keyBytes)+"!\n-----END RSA PRIVATE KEY-----\n")),
			[][]byte{a.der}, []string{
				"warning: block 2 is a damaged RSA PRIVATE KEY (its body is not base64) and is not sent to the relay; a private key does not belong in a file of trust roots",
				" ([relay] upstream_ca_certs): 1 certificate to send; 1 block skipped"}},
		{"CRLF line endings", bytes.ReplaceAll(a.pem, []byte("\n"), []byte("\r\n")),
			[][]byte{a.der},
			[]string{" ([relay] upstream_ca_certs): 1 certificate to send"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := writeCAFile(t, t.TempDir(), "site.pem", tc.data)
			got, report, err := LoadUpstreamCAs([]string{p}, CAFromConfig)
			if err != nil {
				t.Fatalf("error: %v", err)
			}
			var want []byte
			for _, der := range tc.certs {
				want = append(want, pemBlock("CERTIFICATE", der)...)
			}
			if !bytes.Equal(got, want) {
				t.Errorf("returned:\n%s\nwant:\n%s", got, want)
			}
			assertSameAsPool(t, tc.data, got)
			var wantLines []string
			for _, l := range tc.lines {
				if rest, ok := strings.CutPrefix(l, "warning: "); ok {
					wantLines = append(wantLines, "warning: upstream CA "+p+": "+rest)
				} else {
					wantLines = append(wantLines, "upstream CA "+p+l)
				}
			}
			assertLines(t, report.Lines(true), wantLines)
			assertLines(t, report.Lines(false), wantLines)
		})
	}
}

func assertLines(t *testing.T, got, want []string) {
	t.Helper()
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("lines:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// TestLoadUpstreamCAsNeverSendsAKey puts a key of each type that ends in
// PRIVATE KEY beside a certificate: each is a warning, and no byte of a key
// is in what is returned.
func TestLoadUpstreamCAsNeverSendsAKey(t *testing.T) {
	a := newTestCert(t, "a")
	for _, typ := range []string{"PRIVATE KEY", "RSA PRIVATE KEY", "EC PRIVATE KEY",
		"ENCRYPTED PRIVATE KEY", "OPENSSH PRIVATE KEY"} {
		t.Run(typ, func(t *testing.T) {
			key := pemBlock(typ, keyBytes)
			p := writeCAFile(t, t.TempDir(), "bundle.pem", join(key, a.pem))
			got, report, err := LoadUpstreamCAs([]string{p}, CAFromEnv)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, a.pem) {
				t.Errorf("returned %q, want the certificate alone", got)
			}
			if bytes.Contains(got, keyBytes) || bytes.Contains(got, key) ||
				bytes.Contains(got, []byte("PRIVATE")) {
				t.Errorf("a key's bytes are in what is returned: %q", got)
			}
			article := "an"
			if typ == "PRIVATE KEY" {
				article = "a"
			}
			assertLines(t, report.Lines(false), []string{
				"warning: upstream CA " + p + ": block 1 is " + article + " " + typ +
					" and is not sent to the relay; a private key does not belong in a file of trust roots",
				"upstream CA " + p + " (WARDEN_UPSTREAM_CA_CERTS): 1 certificate to send; 1 block skipped",
			})
		})
	}
}

// TestLoadUpstreamCAsFileErrors: a file named directly that gives no
// certificate is an error that says why.
func TestLoadUpstreamCAsFileErrors(t *testing.T) {
	a := newTestCert(t, "a")
	const noCert = "name a file or directory that holds at least one PEM certificate, or remove the path"
	tests := []struct {
		name, file string
		data       []byte
		msg, fix   string
		lines      []string
	}{
		{"DER", "corp.cer", a.der,
			"gives no certificate to send: it is DER-encoded, and only PEM is read",
			"convert it to PEM (openssl x509 -inform der -in <file> -out <file>.pem) and name the result, or remove the path",
			nil},
		{"a key alone", "site.key", pemBlock("RSA PRIVATE KEY", keyBytes),
			"gives no certificate to send: it holds 1 PEM block and none is a certificate that can be sent",
			noCert,
			[]string{"warning: upstream CA %s: block 1 is an RSA PRIVATE KEY and is not sent to the relay; a private key does not belong in a file of trust roots"}},
		{"text", "notes.pem", []byte("hello\n"),
			"gives no certificate to send: there is no PEM block in it", noCert, nil},
		{"empty", "empty.pem", nil,
			"gives no certificate to send: there is no PEM block in it", noCert, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := writeCAFile(t, t.TempDir(), tc.file, tc.data)
			got, report, err := LoadUpstreamCAs([]string{p}, CAFromConfig)
			var fe *FixableError
			if !errors.As(err, &fe) {
				t.Fatalf("error %v is not a FixableError", err)
			}
			if got != nil {
				t.Errorf("returned %q with the error", got)
			}
			if want := "upstream CA path " + p + " (from [relay] upstream_ca_certs) " + tc.msg; fe.Msg != want {
				t.Errorf("message:\n%s\nwant:\n%s", fe.Msg, want)
			}
			if fe.Fix != tc.fix {
				t.Errorf("fix:\n%s\nwant:\n%s", fe.Fix, tc.fix)
			}
			var want []string
			for _, l := range tc.lines {
				want = append(want, strings.ReplaceAll(l, "%s", p))
			}
			assertLines(t, report.Lines(true), want)
		})
	}
}

// TestLoadUpstreamCAsDirectory reads a directory one level deep: what is
// sent, every skip, the counts, and the lines --verbose adds.
func TestLoadUpstreamCAsDirectory(t *testing.T) {
	a, b, c := newTestCert(t, "a"), newTestCert(t, "b"), newTestCert(t, "c")
	dir := t.TempDir()
	writeCAFile(t, dir, "a.pem", join(a.pem, b.pem))
	writeCAFile(t, dir, "b.CRT", join(c.pem, pemBlock("PRIVATE KEY", keyBytes)))
	writeCAFile(t, dir, "corp.cer", a.der)
	writeCAFile(t, dir, "keys.pem", join(pemBlock("EC PRIVATE KEY", keyBytes),
		pemBlock("X509 CRL", []byte("crl"))))
	writeCAFile(t, dir, "notes.crt", []byte("see the wiki\n"))
	writeCAFile(t, dir, "README", a.pem)
	if err := os.Mkdir(filepath.Join(dir, "old"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "sub.pem"), 0o700); err != nil {
		t.Fatal(err)
	}
	symlink(t, filepath.Join(dir, "missing.pem"), filepath.Join(dir, "gone.crt"))
	symlink(t, filepath.Join(dir, "old"), filepath.Join(dir, "linked.pem"))
	symlink(t, filepath.Join(dir, "a.pem"), filepath.Join(dir, "z.pem"))

	got, report, err := LoadUpstreamCAs([]string{dir}, CAFromEnv)
	if err != nil {
		t.Fatal(err)
	}
	want := join(a.pem, b.pem, c.pem, a.pem, b.pem)
	if !bytes.Equal(got, want) {
		t.Errorf("returned:\n%s\nwant:\n%s", got, want)
	}
	if bytes.Contains(got, keyBytes) {
		t.Error("a key's bytes are in what is returned")
	}
	at := func(name string) string { return "upstream CA " + filepath.Join(dir, name) }
	const keyNotSent = " and is not sent to the relay; a private key does not belong in a file of trust roots"
	verbose := []string{
		at("README") + ": not read: its name does not end in .pem, .crt or .cer",
		at("a.pem") + ": 2 certificates to send",
		"warning: " + at("b.CRT") + ": block 2 is a PRIVATE KEY" + keyNotSent,
		at("b.CRT") + ": 1 certificate to send; 1 block skipped",
		at("corp.cer") + ": skipped: DER-encoded, and only PEM is read (to convert: openssl x509 -inform der -in <file> -out <file>.pem)",
		at("gone.crt") + ": skipped: a link to a file that does not exist",
		"warning: " + at("keys.pem") + ": block 1 is an EC PRIVATE KEY" + keyNotSent,
		at("keys.pem") + ": block 2 skipped: an X509 CRL block, not a CERTIFICATE",
		at("keys.pem") + ": skipped: it holds 2 PEM blocks and none is a certificate that can be sent",
		at("linked.pem") + ": not read: a directory",
		at("notes.crt") + ": skipped: no PEM block in it",
		at("old") + ": not read: a directory",
		at("sub.pem") + ": not read: a directory",
		at("z.pem") + ": 2 certificates to send",
		"upstream CA " + dir + " (WARDEN_UPSTREAM_CA_CERTS): 5 certificates to send from 3 files; 4 files skipped; 4 other entries not read",
	}
	assertLines(t, report.Lines(true), verbose)
	// Without --verbose: the skips and the path's line, not the per-file
	// list of what is sent nor the entries that were not read.
	assertLines(t, report.Lines(false), []string{
		verbose[2], verbose[4], verbose[5], verbose[6], verbose[7], verbose[8],
		verbose[10], verbose[14],
	})
}

func symlink(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("cannot make a symbolic link here: %v", err)
	}
}

// TestLoadUpstreamCAsErrors: the errors that are not about one file's
// content, the paths taken in order and the first error stopping.
func TestLoadUpstreamCAsErrors(t *testing.T) {
	a := newTestCert(t, "a")
	good := writeCAFile(t, t.TempDir(), "corp.pem", a.pem)
	missing := filepath.Join(t.TempDir(), "roots.pem")
	onlySkips := t.TempDir()
	writeCAFile(t, onlySkips, "x.pem", []byte("x"))
	writeCAFile(t, onlySkips, "y.cer", a.der)
	writeCAFile(t, onlySkips, "README", nil)
	noCAFiles := t.TempDir()
	writeCAFile(t, noCAFiles, "README", a.pem)
	const dirFix = "put at least one PEM certificate in a .pem, .crt or .cer file there, or remove the path"
	tests := []struct {
		name     string
		paths    []string
		source   string
		msg, fix string
		lines    int // the report's lines with the error
	}{
		{"does not exist", []string{missing}, CAFromConfig,
			"upstream CA path " + missing + " (from [relay] upstream_ca_certs) does not exist",
			"name an existing file or directory of PEM certificates, or remove the path", 0},
		{"the second path fails after the first gave certificates",
			[]string{good, missing}, CAFromEnv,
			"upstream CA path " + missing + " (from WARDEN_UPSTREAM_CA_CERTS) does not exist",
			"name an existing file or directory of PEM certificates, or remove the path", 1},
		{"an empty entry in the variable", []string{good, ""}, CAFromEnv,
			"WARDEN_UPSTREAM_CA_CERTS has an empty entry",
			"remove the doubled or trailing separator", 1},
		{"an empty entry in the key", []string{""}, CAFromConfig,
			"[relay] upstream_ca_certs has an empty entry",
			`remove the "" entry from the list`, 0},
		{"an empty entry of --upstream-ca", []string{"", good}, CAFromFlag,
			"--upstream-ca has an empty entry",
			"remove the doubled or trailing separator", 0},
		{"a directory whose files give nothing", []string{onlySkips}, CAFromEnv,
			"upstream CA path " + onlySkips + " (from WARDEN_UPSTREAM_CA_CERTS) gives no certificate to send: 2 files skipped, 1 other entry not read",
			dirFix, 2},
		{"a directory with no CA file", []string{noCAFiles}, CAFromConfig,
			"upstream CA path " + noCAFiles + " (from [relay] upstream_ca_certs) gives no certificate to send: it holds no .pem, .crt or .cer file",
			dirFix, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, report, err := LoadUpstreamCAs(tc.paths, tc.source)
			var fe *FixableError
			if !errors.As(err, &fe) {
				t.Fatalf("error %v is not a FixableError", err)
			}
			if got != nil {
				t.Errorf("returned %q with the error", got)
			}
			if fe.Msg != tc.msg || fe.Fix != tc.fix {
				t.Errorf("error:\n%s\n  To fix: %s\nwant:\n%s\n  To fix: %s", fe.Msg, fe.Fix, tc.msg, tc.fix)
			}
			if n := len(report.Lines(false)); n != tc.lines {
				t.Errorf("%d lines with the error, want %d: %q", n, tc.lines, report.Lines(false))
			}
		})
	}
}

// TestLoadUpstreamCAsRelativePath: a relative path is the working
// directory's, and the lines print it absolute.
func TestLoadUpstreamCAsRelativePath(t *testing.T) {
	a := newTestCert(t, "a")
	dir := t.TempDir()
	writeCAFile(t, dir, "corp.pem", a.pem)
	t.Chdir(dir)
	got, report, err := LoadUpstreamCAs([]string{"corp.pem"}, CAFromConfig)
	if err != nil || !bytes.Equal(got, a.pem) {
		t.Fatalf("got %q, %v", got, err)
	}
	abs, _ := filepath.Abs("corp.pem")
	assertLines(t, report.Lines(false), []string{
		"upstream CA " + abs + " ([relay] upstream_ca_certs): 1 certificate to send"})
}

// TestLoadUpstreamCAsEscapes: a string that is not warden's own reaches no
// line or error as it is, and the DER hint never carries a file's name.
func TestLoadUpstreamCAsEscapes(t *testing.T) {
	a := newTestCert(t, "a")
	dir := t.TempDir()
	writeCAFile(t, dir, "$(id).cer", a.der)
	writeCAFile(t, dir, "ok.pem", join(a.pem,
		[]byte("-----BEGIN \x1b[2J\u202e-----\nAAAA\n-----END \x1b[2J\u202e-----\n")))
	_, report, err := LoadUpstreamCAs([]string{dir}, CAFromConfig)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Join(report.Lines(true), "\n")
	for _, want := range []string{
		filepath.Join(dir, "$(id).cer") + ": skipped: DER-encoded, and only PEM is read (to convert: openssl x509 -inform der -in <file> -out <file>.pem)",
		filepath.Join(dir, "ok.pem") + `: block 2 skipped: a \x1b[2J\u202e block, not a CERTIFICATE`,
	} {
		if !strings.Contains(lines, want) {
			t.Errorf("no line %q in:\n%s", want, lines)
		}
	}
	if strings.ContainsAny(lines, "\x1b\u202e") {
		t.Errorf("a control character reached a line:\n%q", lines)
	}

	_, _, err = LoadUpstreamCAs([]string{"/no/such\n\x1b[2Jpath\xff"}, CAFromConfig)
	var fe *FixableError
	if !errors.As(err, &fe) {
		t.Fatalf("error %v", err)
	}
	if want := `upstream CA path /no/such\n\x1b[2Jpath\xff (from [relay] upstream_ca_certs) does not exist`; fe.Msg != want {
		t.Errorf("message %q, want %q", fe.Msg, want)
	}
}

func TestEscapeOutside(t *testing.T) {
	for in, want := range map[string]string{
		"/etc/ssl/certs":     "/etc/ssl/certs",
		"café 中.pem":         "café 中.pem",
		"a\nb\rc\td":         `a\nb\rc\td`,
		"\x1b[2J\x07\x00":    `\x1b[2J\x07\x00`,
		"bad\xff\xfe":        `bad\xff\xfe`,
		"\u0085\u202e\u2028": `\x85\u202e\u2028`,
		"\U000e0001":         `\U000e0001`,
	} {
		if got := escapeOutside(in); got != want {
			t.Errorf("escapeOutside(%q) = %q, want %q", in, got, want)
		}
	}
}
