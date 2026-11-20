package driverconformance

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/buildwarden/buildwarden/collector"
	"github.com/buildwarden/buildwarden/relaycfg"
)

// bin holds warden-driver-fake, stubrelay and warden-driver-container, built
// once for the package's tests with the toolchain that runs them (TEST-01).
var bin string

func exe(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "driver-conformance-bin-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	bin = dir
	code := build(dir)
	if code == 0 {
		code = m.Run()
	}
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

// goTool is the go command on PATH, which make test runs under.
func goTool() string {
	if p, err := exec.LookPath("go"); err == nil {
		return p
	}
	return "go"
}

func build(dir string) int {
	for _, b := range []struct{ out, pkg string }{
		{exe("warden-driver-fake"), "./fake"},
		{exe("stubrelay"), "./stubrelay"},
		{exe("warden-driver-container"), "../../cmd/warden-driver-container"},
	} {
		cmd := exec.Command(goTool(), "build", "-o", filepath.Join(dir, b.out), b.pkg)
		if out, err := cmd.CombinedOutput(); err != nil {
			fmt.Fprintf(os.Stderr, "building %s: %v\n%s", b.pkg, err, out)
			return 1
		}
	}
	return 0
}

func fakeSuite(t *testing.T) *Suite {
	t.Helper()
	s, err := New(filepath.Join(bin, exe("warden-driver-fake")))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

// The embedded schema is the specification's.
func TestSchemaIsTheSpecifications(t *testing.T) {
	spec, err := os.ReadFile("../../specs/driver-contract/v1.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(spec, schemaJSON) {
		t.Error("conformance/driver/v1.schema.json differs from " +
			"specs/driver-contract/v1.schema.json; copy it again")
	}
}

// The static tier passes against the reference fake (TEST-05).
func TestStaticFake(t *testing.T) {
	t.Parallel()
	for _, r := range fakeSuite(t).Static() {
		if r.Failure != "" {
			t.Errorf("%s (%s): %s", r.Check, strings.Join(r.Requirements, ", "),
				r.Failure)
		}
		if len(r.Requirements) == 0 {
			t.Errorf("%s names no requirement", r.Check)
		}
	}
}

// The static tier passes against the container driver, on a host with or
// without a container runtime (TEST-05).
func TestStaticContainer(t *testing.T) {
	t.Parallel()
	s, err := New(filepath.Join(bin, exe("warden-driver-container")))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	for _, r := range s.Static() {
		if r.Failure != "" {
			t.Errorf("%s (%s): %s", r.Check, strings.Join(r.Requirements, ", "),
				r.Failure)
		}
	}
}

// The suite's self-test: each check fails under the fake's fault that breaks
// what it checks.
func TestSelfTest(t *testing.T) {
	t.Parallel()
	for fault, check := range map[string]string{
		"stray-stdout":               "run with no start",
		"wrong-exit-code":            "invalid request",
		"undeclared-command":         "unlisted commands",
		"accept-undeclared-argument": "undeclared argument",
		"read-cwd-warden-toml":       "only the named file",
		"read-warden-config":         "only the named file",
		"refuse-unowned-table":       "tables it does not own",
		"read-unlisted-common-key":   "tables it does not own",
		"accept-wrong-type":          "config file and key refused",
		"reap-fails-config-gone":     "reap with its config file gone",
		"accept-another-case":        "a listed key in another letter case",
	} {
		t.Run(fault, func(t *testing.T) {
			t.Parallel()
			s := fakeSuite(t)
			s.env = []string{"WARDEN_FAKE_FAULT=" + fault}
			for _, r := range s.Static() {
				if r.Check == check {
					if r.Failure == "" {
						t.Errorf("%s passed under fault %s", check, fault)
					}
					return
				}
			}
			t.Fatalf("no check %q", check)
		})
	}
}

// DRV-C4: the declarations check refuses two places equal when ASCII letters
// are folded, whether two keys, a key and a common key's override, or a key
// and an old name; and passes the bundled drivers' shapes.
func TestPlacesFolded(t *testing.T) {
	for _, tc := range []struct {
		name string
		keys []describeKey
		ok   bool
	}{
		{"two keys", []describeKey{{Key: "driver.nomad.region"},
			{Key: "driver.nomad.Region"}}, false},
		{"a key and an override", []describeKey{{Key: "driver.image_min_grace"},
			{Key: "driver.nomad.IMAGE_MIN_GRACE"}}, false},
		{"one place each", []describeKey{{Key: "driver.image_min_grace"},
			{Key: "driver.nomad.region", Aliases: []string{"vm.region"}}}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := &Suite{desc: &description{Name: "nomad", Keys: tc.keys}}
			if msg := s.checkPlacesFolded(); (msg == "") != tc.ok {
				t.Errorf("checkPlacesFolded = %q, want ok %v", msg, tc.ok)
			}
		})
	}
}

// DRV-C8, the suite's self-test for the fake: its probe echoes each value
// with the place it came from, by the steps of the precedence it can reach,
// and the fault that reads [driver] first is caught.
func TestFakePrecedence(t *testing.T) {
	s := fakeSuite(t)
	if r := s.Static(); r[0].Failure != "" {
		t.Fatal(r[0].Failure)
	}
	worked := "[driver]\nimage_refresh_after = \"3d\"\n\n[driver.fake]\n" +
		"image_refresh_after = \"12h\"\ngreeting = \"from the file\"\n\n" +
		"[driver.qemu]\nimage_min_grace = \"6h\"\n"
	for _, tc := range []struct {
		name, file, fault string
		args              []string
		want              []string
	}{
		{name: "flag over own table (step 1 over 3)", file: worked,
			args: []string{"--greeting=from the flag"},
			want: []string{`greeting="from the flag" from --greeting`}},
		{name: "own table (step 3)", file: worked,
			want: []string{`image_refresh_after="12h" from`,
				"[driver.fake] image_refresh_after",
				`greeting="from the file"`}},
		{name: "[driver] (step 4)", file: "[driver]\nimage_refresh_after = \"3d\"\n",
			want: []string{`image_refresh_after="3d" from`,
				"[driver] image_refresh_after"}},
		{name: "nowhere (step 5)", file: "[driver.qemu]\nimage_refresh_after = \"1d\"\n",
			want: []string{`image_refresh_after="" from the default`,
				`greeting="hello" from the default`}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			file := filepath.Join(t.TempDir(), "warden.toml")
			if err := os.WriteFile(file, []byte(tc.file), 0o600); err != nil {
				t.Fatal(err)
			}
			o := s.probe("run", map[string]any{"config_file": file, "args": tc.args})
			for _, w := range tc.want {
				if !strings.Contains(string(o.stderr), w) {
					t.Errorf("probe stderr %q lacks %q", o.stderr, w)
				}
			}
		})
	}
	t.Run("fault: [driver] before its own table", func(t *testing.T) {
		file := filepath.Join(t.TempDir(), "warden.toml")
		if err := os.WriteFile(file, []byte(worked), 0o600); err != nil {
			t.Fatal(err)
		}
		o := s.probe("run", map[string]any{"config_file": file},
			"WARDEN_FAKE_FAULT=driver-before-own-table")
		if strings.Contains(string(o.stderr), `image_refresh_after="12h"`) {
			t.Errorf("the fault still read the fake's own table first: %s", o.stderr)
		}
	})
}

// The suite and the fake are written from the specification: the suite
// imports nothing under driver/ or cmd/warden, and the fake nothing of this
// module at all (GLOBAL-08, LOOP-03).
func TestImports(t *testing.T) {
	const module = "github.com/buildwarden/buildwarden/"
	for _, tc := range []struct {
		pkg     string
		allowed func(string) bool
	}{
		{".", func(p string) bool {
			return !strings.HasPrefix(p, module+"driver") &&
				!strings.HasPrefix(p, module+"cmd/warden")
		}},
		{"./cmd/driver-conformance", func(p string) bool {
			return !strings.HasPrefix(p, module+"driver") &&
				!strings.HasPrefix(p, module+"cmd/warden")
		}},
		{"./fake", func(p string) bool {
			return !strings.HasPrefix(p,
				module) || p == module+"conformance/driver/fake"
		}},
		{"./stubrelay", func(p string) bool {
			return !strings.HasPrefix(p, module) || p == module+"relaycfg" ||
				p == module+"conformance/driver/stubrelay"
		}},
	} {
		out, err := exec.Command(goTool(), "list", "-deps", tc.pkg).Output()
		if err != nil {
			t.Fatalf("go list %s: %v", tc.pkg, err)
		}
		for _, p := range strings.Fields(string(out)) {
			if !tc.allowed(p) {
				t.Errorf("%s imports %s", tc.pkg, p)
			}
		}
	}
}

// stubrelay accepts the claim, reaches the collector over the tunnel, and
// plays the guest by the script's directives.
func TestStubRelay(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	script := filepath.Join(dir, "build.sh")
	body := "# stub: output marker-1\n# stub: sleep 10ms\n# stub: exit 3\n" +
		"# stub: output never\n"
	if err := os.WriteFile(script, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	col, err := collector.New(collector.Config{OutputDir: filepath.Join(dir, "out"),
		Token: "ctoken", BuildScriptPath: script})
	if err != nil {
		t.Fatal(err)
	}
	defer col.Close()

	cmd := exec.Command(filepath.Join(bin, exe("stubrelay")), "--listen", "127.0.0.1:0")
	cmd.Env = append(os.Environ(), relaycfg.TokenEnv+"=claim-token")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()
	var line [128]byte
	n, _ := stdout.Read(line[:])
	addr, ok := strings.CutPrefix(strings.TrimSpace(string(line[:n])), "listening ")
	if _, _, err := net.SplitHostPort(addr); !ok || err != nil {
		t.Fatalf("stubrelay printed %q", line[:n])
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	conn, _, err := relaycfg.OpenTunnel(ctx, addr, "claim-token", col.Handler())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err := relaycfg.Claim(ctx, addr, "claim-token", relaycfg.Config{
		CollectorURL: relaycfg.TunnelURL, CollectorToken: "ctoken"}); err != nil {
		t.Fatal(err)
	}
	select {
	case sig := <-col.Done():
		if sig.ExitCode != 3 {
			t.Errorf("completion %+v, want exit 3", sig)
		}
	case <-ctx.Done():
		t.Fatal("no completion")
	}
	out, _ := os.ReadFile(filepath.Join(dir, "out", "build-output.log"))
	if string(out) != "marker-1\n" {
		t.Errorf("build output %q", out)
	}
}
