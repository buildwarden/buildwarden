package main

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
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/buildwarden/buildwarden/driver"
)

// caCert makes a self-signed certificate and returns its DER bytes and its
// PEM encoding.
func caCert(t *testing.T, name string) (der, pemBytes []byte) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: name},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
	}
	der, err = x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return der, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}

const caKeyBody = "SECRET-KEY-MATERIAL"

// caDirectory is the Validation plan's directory: a certificate, a
// certificate with its key, a DER .cer and a README. It returns the
// directory and the certificates a build sends from it.
func caDirectory(t *testing.T) (dir string, sent []byte) {
	t.Helper()
	dir = t.TempDir()
	_, a := caCert(t, "a")
	_, b := caCert(t, "b")
	der, _ := caCert(t, "c")
	key := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: []byte(caKeyBody)})
	write(t, dir, "a.pem", string(a))
	write(t, dir, "b.pem", string(b)+string(key))
	write(t, dir, "c.cer", string(der))
	write(t, dir, "README", "the corporate roots\n")
	return dir, append(append([]byte{}, a...), b...)
}

// caLines are the lines of caDirectory's report, in order: the per-file
// lines are those --verbose adds.
func caLines(dir string, verbose bool) []string {
	at := func(name string) string { return "upstream CA " + filepath.Join(dir, name) }
	var lines []string
	if verbose {
		lines = append(lines,
			at("README")+": not read: its name does not end in .pem, .crt or .cer",
			at("a.pem")+": 1 certificate to send")
	}
	lines = append(lines, "warning: "+at("b.pem")+": block 2 is an RSA PRIVATE KEY and is "+
		"not sent to the relay; a private key does not belong in a file of trust roots")
	if verbose {
		lines = append(lines, at("b.pem")+": 1 certificate to send; 1 block skipped")
	}
	return append(lines,
		at("c.cer")+": skipped: DER-encoded, and only PEM is read (to convert: "+
			"openssl x509 -inform der -in <file> -out <file>.pem)",
		"upstream CA "+dir+" (WARDEN_UPSTREAM_CA_CERTS): 2 certificates to send "+
			"from 2 files; 1 file skipped; 1 other entry not read")
}

// buildProject is a build context with a Dockerfile.
func buildProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	write(t, dir, "Dockerfile", "FROM alpine\n")
	return dir
}

func caKeyFile(paths ...string) string {
	return "[relay]\nupstream_ca_certs = [" + strings.Join(quoteAll(paths), ", ") + "]\n"
}

func quoteAll(ss []string) []string {
	out := make([]string, len(ss))
	for i, s := range ss {
		out[i] = tomlString(s)
	}
	return out
}

// The variable replaces the key, split on the operating system's path-list
// separator; LoadConfig no longer adds it to the key. (This was
// TestLoadConfig_UpstreamCACertsEnv, which asserted the variable's paths
// appended to LoadConfig's result.)
func TestUpstreamCAPaths(t *testing.T) {
	sep := string(os.PathListSeparator)
	t.Run("the variable alone", func(t *testing.T) {
		inConfigFiles(t, "")
		t.Setenv(driver.CAFromEnv, "/path/to/ca.pem"+sep+"/other/ca.crt")
		cfg, err := LoadConfig()
		if err != nil {
			t.Fatal(err)
		}
		if len(cfg.Relay.UpstreamCACerts) != 0 {
			t.Errorf("LoadConfig put the variable in the key: "+
				"%q", cfg.Relay.UpstreamCACerts)
		}
		paths, source, err := upstreamCAPaths(cfg)
		if err != nil || source != driver.CAFromEnv ||
			strings.Join(paths, "|") != "/path/to/ca.pem|/other/ca.crt" {
			t.Errorf("got %q from %s, %v", paths, source, err)
		}
	})
	t.Run("the key alone", func(t *testing.T) {
		inConfigFiles(t, caKeyFile("/my/corp-ca.pem"))
		cfg, err := LoadConfig()
		if err != nil {
			t.Fatal(err)
		}
		paths, source, err := upstreamCAPaths(cfg)
		if err != nil || source != driver.CAFromConfig ||
			strings.Join(paths, "|") != "/my/corp-ca.pem" {
			t.Errorf("got %q from %s, %v", paths, source, err)
		}
	})
	t.Run("an empty variable is not set", func(t *testing.T) {
		inConfigFiles(t, caKeyFile("/my/corp-ca.pem"))
		cfg, _ := LoadConfig()
		_, source, err := upstreamCAPaths(cfg)
		if err != nil || source != driver.CAFromConfig {
			t.Errorf("source %s, %v", source, err)
		}
	})
}

// Both set is the transitional error where the paths are used, build,
// shell and doctor, and nowhere else: clean, setup and image prepare load
// their configuration as before.
func TestUpstreamCAsBothSet(t *testing.T) {
	const msg = "WARDEN_UPSTREAM_CA_CERTS and [relay] upstream_ca_certs in "
	setup := func(t *testing.T) {
		inConfigFiles(t, caKeyFile("/etc/warden/corp.pem"))
		t.Setenv(driver.CAFromEnv, "/etc/warden/other.pem")
		t.Setenv("PATH", t.TempDir())
	}
	for name, run := range map[string]func() error{
		"build": func() error { return runBuild(buildCmd, []string{buildProject(t)}) },
		"shell": func() error { return runShell(shellCmd, []string{buildProject(t)}) },
		"doctor": func() error {
			doctorCmd.SetOut(&bytes.Buffer{})
			defer doctorCmd.SetOut(nil)
			return doctorCmd.RunE(doctorCmd, nil)
		},
	} {
		t.Run(name, func(t *testing.T) {
			setup(t)
			captureLog(t)
			err := run()
			var fe *driver.FixableError
			if !errors.As(err, &fe) || !strings.HasPrefix(fe.Msg, msg+configPath(t)) {
				t.Errorf("got %v, want the error %q", err, msg)
			}
		})
	}
	for name, run := range map[string]func() error{
		"clean":         func() error { return runClean(cleanCmd, nil) },
		"setup":         func() error { return setupCmd.RunE(setupCmd, nil) },
		"image prepare": func() error { return runImagePrepare(imagePrepareCmd, nil) },
	} {
		t.Run(name, func(t *testing.T) {
			setup(t)
			captureLog(t)
			if _, err := resolveConfig(); err != nil {
				t.Fatalf("the configuration does not load: %v", err)
			}
			_ = captureStdout(t, func() {
				err := run()
				if err != nil && strings.Contains(err.Error(), driver.CAFromEnv) {
					t.Errorf("%s uses the CA paths: %v", name, err)
				}
			})
		})
	}
}

// The lines, in order after the config: line, on stderr with the prefix,
// with and without --verbose, the same with --quiet, and nothing on stdout;
// the request carries the certificates and nothing else.
func TestUpstreamCALinesOfABuild(t *testing.T) {
	for _, tc := range []struct {
		name           string
		verbose, quiet bool
	}{
		{"plain", false, false},
		{"--verbose", true, false},
		{"--quiet", false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			inConfigFiles(t, "")
			dir, sent := caDirectory(t)
			t.Setenv(driver.CAFromEnv, dir)
			flagVerbose, flagQuiet = tc.verbose, tc.quiet
			t.Cleanup(func() { flagVerbose, flagQuiet = false, false })
			stderr := captureLog(t)
			var req *driver.BuildRequest
			stdout := captureStdout(t, func() {
				cfg, err := announcedConfig()
				if err != nil {
					t.Fatal(err)
				}
				req, err = newRequest(cfg, linkedSel(cfg),
					[]string{buildProject(t)})
				if err != nil {
					t.Fatal(err)
				}
			})
			want := "[warden] config: none (no --config, no WARDEN_CONFIG, no " +
				"./warden.toml): defaults apply\n"
			for _, l := range caLines(dir, tc.verbose) {
				want += "[warden] " + l + "\n"
			}
			if stderr.String() != want {
				t.Errorf("stderr:\n%s\nwant:\n%s", stderr, want)
			}
			if stdout != "" {
				t.Errorf("stdout: %q", stdout)
			}
			if !bytes.Equal(req.UpstreamCACerts, sent) {
				t.Errorf("the request "+
					"carries:\n%s\nwant:\n%s", req.UpstreamCACerts, sent)
			}
			if bytes.Contains(req.UpstreamCACerts, []byte(caKeyBody)) {
				t.Error("the key beside a certificate is in the request")
			}
		})
	}
}

// doctor prints the same lines in its report, on stdout without the prefix,
// after its config: line; with a path a build refuses it prints BLOCKED and
// exits 1. Run in a child process, since doctor ends the process itself;
// the container driver finds a docker that answers. Since the container
// driver is a binary its doctor prints its own ok, which warden's BLOCKED
// line does not stop: warden exits 1 after it.
func TestUpstreamCALinesOfDoctor(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fake docker is a shell script")
	}
	missing := filepath.Join(t.TempDir(), "roots.pem")
	noCert := t.TempDir()
	write(t, noCert, "site.pem", "not a certificate\n")
	for _, tc := range []struct {
		name, paths string
		lines       func(dir string) []string
		exit        int
	}{
		{"a directory that gives certificates", "", func(dir string) []string {
			return append(caLines(dir, false), "container runtime: docker", "ok")
		}, 0},
		{"a path that does not exist", missing, func(string) []string {
			return []string{
				"upstream CA: BLOCKED: upstream CA path " + missing +
					" (from WARDEN_UPSTREAM_CA_CERTS) does not exist",
				"upstream CA: to fix: name an existing file or directory of PEM " +
					"certificates, or remove the path",
				"container runtime: docker", "ok",
			}
		}, 1},
		{"a path that gives no certificate", noCert, func(string) []string {
			at := "upstream CA " + filepath.Join(noCert, "site.pem")
			return []string{
				at + ": skipped: no PEM block in it",
				"upstream CA: BLOCKED: upstream CA path " + noCert +
					" (from WARDEN_UPSTREAM_CA_CERTS) gives no certificate " +
					"to send: 1 file skipped",
				"upstream CA: to fix: put at least one PEM certificate in a " +
					".pem, " +
					".crt or .cer file there, or remove the path",
				"container runtime: docker", "ok",
			}
		}, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			inConfigFiles(t, "")
			paths := tc.paths
			if paths == "" {
				paths, _ = caDirectory(t)
			}
			bin := t.TempDir()
			writeScript(t, filepath.Join(bin, "docker"), "#!/bin/sh\nexit 0\n")
			self, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			child := exec.Command(self, "-test.run=^TestDoctorChild$")
			child.Env = append(os.Environ(), doctorChildEnv+"=container",
				driver.CAFromEnv+"="+paths, "PATH="+bin)
			var stdout, stderr bytes.Buffer
			child.Stdout, child.Stderr = &stdout, &stderr
			err = child.Run()
			exit := 0
			var ee *exec.ExitError
			if errors.As(err, &ee) {
				exit = ee.ExitCode()
			}
			want := doctorChildRan + "\n" +
				"config: none (no --config, no WARDEN_CONFIG, no " +
				"./warden.toml): defaults apply\n" +
				strings.Join(tc.lines(paths), "\n") + "\n"
			got := strings.TrimSuffix(stdout.String(), "PASS\n")
			if got != want || exit != tc.exit {
				t.Errorf("exit %d, stdout:\n%s\nwant exit %d, "+
					"stdout:\n%s", exit, got, tc.exit, want)
			}
			if strings.Contains(stderr.String(), "upstream CA") {
				t.Errorf("doctor put a CA line on stderr: %s", stderr.String())
			}
		})
	}
}

// A path a build cannot use stops it before the output directory or the
// work directory is made.
func TestUpstreamCAErrorsStopABuildBeforeAnythingIsMade(t *testing.T) {
	sep := string(os.PathListSeparator)
	noCert := filepath.Join(t.TempDir(), "site.key")
	if err := os.WriteFile(noCert, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY",
		Bytes: []byte(caKeyBody)}), 0o600); err != nil {
		t.Fatal(err)
	}
	_, good := caCert(t, "good")
	goodFile := filepath.Join(t.TempDir(), "good.pem")
	if err := os.WriteFile(goodFile, good, 0o600); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(t.TempDir(), "roots.pem")
	for _, tc := range []struct {
		name, variable, msg string
	}{
		{"does not exist", missing, "upstream CA path " + missing +
			" (from WARDEN_UPSTREAM_CA_CERTS) does not exist"},
		{"gives no certificate", noCert, "upstream CA path " + noCert +
			" (from WARDEN_UPSTREAM_CA_CERTS) gives no certificate to send: it " +
			"holds " +
			"1 PEM block and none is a certificate that can be sent"},
		{"an empty entry", goodFile + sep, "WARDEN_UPSTREAM_CA_CERTS has an empty entry"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			wd := inConfigFiles(t, "")
			t.Setenv(driver.CAFromEnv, tc.variable)
			tmp := t.TempDir()
			flagTempDir, flagOutput = tmp, filepath.Join(wd, "out")
			t.Cleanup(func() { flagOutput = "" })
			captureLog(t)
			err := runBuild(buildCmd, []string{buildProject(t)})
			var fe *driver.FixableError
			if !errors.As(err, &fe) || fe.Msg != tc.msg {
				t.Fatalf("got %v, want %q", err, tc.msg)
			}
			if _, err := os.Stat(flagOutput); !os.IsNotExist(err) {
				t.Errorf("the output directory was made: %v", err)
			}
			if entries, _ := os.ReadDir(tmp); len(entries) != 0 {
				t.Errorf("the work directory was made: %v", entries)
			}
		})
	}
}

// No configured path: nothing is read or printed, and the request carries
// no certificate.
func TestNoUpstreamCAPath(t *testing.T) {
	inConfigFiles(t, "")
	stderr := captureLog(t)
	cfg, err := resolveConfig()
	if err != nil {
		t.Fatal(err)
	}
	req, err := newRequest(cfg, linkedSel(cfg), []string{buildProject(t)})
	if err != nil {
		t.Fatal(err)
	}
	if req.UpstreamCACerts != nil || stderr.Len() != 0 {
		t.Errorf("request %q, printed %q", req.UpstreamCACerts, stderr)
	}
}
