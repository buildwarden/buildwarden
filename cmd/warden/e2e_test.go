package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// warden's end-to-end tests run a copy of this test binary as warden (its
// main), with warden-driver-container, warden-driver-fake and stubrelay
// beside it: the fake is a driver warden was not built with, through every
// command of the contract (specs/driver-contract/v1.md). The cases that
// signal a process are in e2e_unix_test.go.

var e2eBin struct {
	once sync.Once
	dir  string
	err  error
}

// e2eBinaries is the directory holding warden, warden-driver-fake and
// stubrelay, built once for the package's tests with the toolchain that runs
// them.
func e2eBinaries(t *testing.T) string {
	t.Helper()
	e2eBin.once.Do(func() {
		dir := filepath.Join(testBinDir, "e2e")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			e2eBin.err = err
			return
		}
		if err := copyFile(os.Args[0], filepath.Join(dir, exeName("warden")),
			0o755); err != nil {
			e2eBin.err = err
			return
		}
		for _, b := range []struct{ out, pkg string }{
			{driverFileName("container", isWindows), "../warden-driver-container"},
			{driverFileName("fake", isWindows), "../../conformance/driver/fake"},
			{exeName("stubrelay"), "../../conformance/driver/stubrelay"},
		} {
			cmd := exec.Command(goCommand(), "build", "-o", filepath.Join(dir, b.out),
				b.pkg)
			if out, err := cmd.CombinedOutput(); err != nil {
				e2eBin.err = fmt.Errorf("building %s: %w\n%s", b.pkg, err, out)
				return
			}
		}
		e2eBin.dir = dir
	})
	if e2eBin.err != nil {
		t.Fatal(e2eBin.err)
	}
	return e2eBin.dir
}

func exeName(name string) string {
	if isWindows {
		return name + ".exe"
	}
	return name
}

// goCommand is the go command on PATH, which make test runs under.
func goCommand() string {
	if p, err := exec.LookPath("go"); err == nil {
		return p
	}
	return "go"
}

// e2e is one end-to-end test's sandbox: its own temporary, cache and home
// directories, so that its locks, records, stage directories and the fake's
// work are its own, and a build context.
type e2e struct {
	t    *testing.T
	bin  string
	root string
	tmp  string
	ctx  string
	env  []string
}

func newE2E(t *testing.T) *e2e {
	t.Helper()
	bin := e2eBinaries(t)
	root := t.TempDir()
	e := &e2e{t: t, bin: bin, root: root, tmp: filepath.Join(root, "tmp"),
		ctx: filepath.Join(root, "ctx")}
	for _, d := range []string{e.tmp, e.ctx, filepath.Join(root, "cache"),
		filepath.Join(root, "home")} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	for _, kv := range os.Environ() {
		k, _, _ := strings.Cut(kv, "=")
		switch strings.ToUpper(k) {
		case "PATH", "TMPDIR", "TMP", "TEMP", "HOME", "XDG_CACHE_HOME", "LOCALAPPDATA",
			"WARDEN_CONFIG", "WARDEN_DRIVER":
			continue
		}
		if strings.HasPrefix(k, "WARDEN_") {
			continue
		}
		e.env = append(e.env, kv)
	}
	e.env = append(e.env, envTestAsWarden+"=1",
		"PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"TMPDIR="+e.tmp, "TMP="+e.tmp, "TEMP="+e.tmp,
		"HOME="+filepath.Join(root, "home"),
		"XDG_CACHE_HOME="+filepath.Join(root, "cache"),
		"LOCALAPPDATA="+filepath.Join(root, "cache"))
	e.script("output started")
	return e
}

// script writes the context's Dockerfile, whose RUN lines are stubrelay's
// directives.
func (e *e2e) script(directives ...string) {
	e.t.Helper()
	var b strings.Builder
	b.WriteString("FROM scratch\n")
	for _, d := range directives {
		b.WriteString("RUN # stub: " + d + "\n")
	}
	if err := os.WriteFile(filepath.Join(e.ctx, "Dockerfile"), []byte(b.String()),
		0o600); err != nil {
		e.t.Fatal(err)
	}
}

// out is where a build of the sandbox writes its output.
func (e *e2e) out() string { return filepath.Join(e.root, "out") }

// buildArgs is a build of the sandbox's context with the fake.
func (e *e2e) buildArgs(more ...string) []string {
	return append([]string{"build", "--driver", "fake", "--tmpdir", e.tmp,
		"--output", e.out()}, append(more, e.ctx)...)
}

func (e *e2e) command(extraEnv []string, args ...string) *exec.Cmd {
	cmd := exec.Command(filepath.Join(e.bin, exeName("warden")), args...)
	cmd.Dir = e.root
	cmd.Env = append(append([]string{}, e.env...), extraEnv...)
	return cmd
}

// run runs warden with args to its end and returns its stdout, its stderr
// and its exit code.
func (e *e2e) run(stdin string, extraEnv []string, args ...string) (string, string, int) {
	e.t.Helper()
	cmd := e.command(extraEnv, args...)
	cmd.Stdin = strings.NewReader(stdin)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	done := make(chan error, 1)
	if err := cmd.Start(); err != nil {
		e.t.Fatal(err)
	}
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		var ee *exec.ExitError
		if err != nil && !errors.As(err, &ee) {
			e.t.Fatal(err)
		}
	case <-time.After(2 * time.Minute):
		_ = cmd.Process.Kill()
		e.t.Fatalf("warden %v did not end\nstderr:\n%s", args, stderr.String())
	}
	return stdout.String(), stderr.String(), cmd.ProcessState.ExitCode()
}

// leftovers are the files of a build left in the sandbox: the fake's work
// directories, stage directories, warden's call directories, and build locks,
// records and state files.
func (e *e2e) leftovers() []string {
	var left []string
	_ = filepath.WalkDir(e.root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		for _, p := range []string{"warden-fake-", "warden-stage-", "warden-call-",
			"build-fake-"} {
			if strings.HasPrefix(d.Name(), p) {
				left = append(left, path)
				if d.IsDir() {
					return fs.SkipDir
				}
			}
		}
		return nil
	})
	return left
}

// noLeftovers fails the test when a build left files, after waiting a while
// for a driver that tears down by itself.
func (e *e2e) noLeftovers(wait time.Duration) {
	e.t.Helper()
	deadline := time.Now().Add(wait)
	for {
		left := e.leftovers()
		if len(left) == 0 {
			return
		}
		if time.Now().After(deadline) {
			e.t.Fatalf("left behind: %v", left)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func wantExit(t *testing.T, got, want int, stderr string) {
	t.Helper()
	if got != want {
		t.Fatalf("exit code %d, want %d\nstderr:\n%s", got, want, stderr)
	}
}

func wantIn(t *testing.T, what, text string, subs ...string) {
	t.Helper()
	for _, s := range subs {
		if !strings.Contains(text, s) {
			t.Errorf("%s lacks %q:\n%s", what, s, text)
		}
	}
}

// A build through the fake: the script's output, its exit code and the
// timeout's, the driver's flag passed on in args, driver.json and the staged
// inputs, and nothing of the build left once warden exits (DRV-45, DRV-57,
// DRV-63).
func TestE2EBuild(t *testing.T) {
	e := newE2E(t)
	e.script("output marker-1", "exit 0")
	stdout, stderr, code := e.run("", nil, e.buildArgs("--greeting", "hi", "--shout")...)
	wantExit(t, code, 0, stderr)
	wantIn(t, "output", stdout+stderr, "marker-1", "fake: HI, build ", "build SUCCEEDED")
	var dj map[string]any
	data, err := os.ReadFile(filepath.Join(e.out(), "driver.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &dj); err != nil {
		t.Fatal(err)
	}
	if dj["name"] != "fake" || dj["found"] != "beside-warden" ||
		dj["path"] != filepath.Join(e.bin, driverFileName("fake", isWindows)) ||
		dj["sha256"] == "" {
		t.Errorf("driver.json: %s", data)
	}
	for _, f := range []string{"inputs/Containerfile", "inputs/build.sh", "relay.log"} {
		if _, err := os.Stat(filepath.Join(e.out(), f)); err != nil {
			t.Errorf("output: %v", err)
		}
	}
	e.noLeftovers(0)

	t.Run("failure", func(t *testing.T) {
		e.script("output failing", "exit 3")
		_, stderr, code := e.run("", nil, e.buildArgs()...)
		wantExit(t, code, 3, stderr)
		wantIn(t, "stderr", stderr, "build FAILED (exit code 3)")
		e.noLeftovers(0)
	})
	t.Run("timeout", func(t *testing.T) {
		e.script("output started", "sleep 1m")
		_, stderr, code := e.run("", nil, e.buildArgs("--timeout", "2s")...)
		wantExit(t, code, 124, stderr)
		wantIn(t, "stderr", stderr, "timed out after 2s")
		e.noLeftovers(0)
	})
}

// A driver named by its path runs from there, and driver.json says so and
// names the copy it shadowed (DRV-4, DRV-7).
func TestE2EDriverByPath(t *testing.T) {
	e := newE2E(t)
	path := filepath.Join(e.bin, driverFileName("fake", isWindows))
	args := e.buildArgs()
	args[2] = path
	_, stderr, code := e.run("", nil, args...)
	wantExit(t, code, 0, stderr)
	var dj struct {
		Path     string `json:"path"`
		Found    string `json:"found"`
		Shadowed []struct {
			Path  string `json:"path"`
			Found string `json:"found"`
		} `json:"shadowed"`
	}
	data, _ := os.ReadFile(filepath.Join(e.out(), "driver.json"))
	if err := json.Unmarshal(data, &dj); err != nil {
		t.Fatal(err)
	}
	if dj.Path != path || dj.Found != "path" || len(dj.Shadowed) != 1 ||
		dj.Shadowed[0].Found != "beside-warden" {
		t.Errorf("driver.json: %s", data)
	}
}

// driver list shows the container driver and the fake beside warden, the
// linked drivers, and with --driver a path, that path selected (DRV-4).
func TestE2EDriverList(t *testing.T) {
	e := newE2E(t)
	stdout, stderr, code := e.run("", nil, "driver", "list")
	wantExit(t, code, 0, stderr)
	rows := map[string]string{}
	for _, line := range strings.Split(stdout, "\n") {
		if f := strings.Fields(line); len(f) > 0 {
			rows[f[0]] = strings.Join(f[1:], " ")
		}
	}
	for name, want := range map[string]string{
		"container": "* " + filepath.Join(e.bin, driverFileName("container", isWindows)) +
			" beside warden",
		"qemu": "(linked into warden) linked",
		"fake": filepath.Join(e.bin, driverFileName("fake", isWindows)) +
			" beside warden",
	} {
		if rows[name] != want {
			t.Errorf("driver list's %s row is %q, want %q:\n%s", name, rows[name],
				want, stdout)
		}
	}

	path := filepath.Join(e.bin, driverFileName("fake", isWindows))
	stdout, stderr, code = e.run("", nil, "driver", "list", "--json", "--driver", path)
	wantExit(t, code, 0, stderr)
	var list struct {
		Drivers []struct {
			Name     string `json:"name"`
			Found    string `json:"found"`
			Source   string `json:"source"`
			Selected bool   `json:"selected"`
		} `json:"drivers"`
	}
	if err := json.Unmarshal([]byte(stdout), &list); err != nil {
		t.Fatalf("%v: %s", err, stdout)
	}
	var fake []string
	for _, d := range list.Drivers {
		if d.Name == "fake" {
			fake = append(fake, fmt.Sprintf("%s %s %v", d.Found, d.Source, d.Selected))
		}
	}
	if strings.Join(fake, ";") != "path --driver true" {
		t.Errorf("the fake's entries: %q\n%s", fake, stdout)
	}
}

// The one-shot commands reach the fake: doctor, setup, image prepare, image
// verify and the fake's own ext subcommand (DRV-25, DRV-26, DRV-29).
func TestE2EOneShots(t *testing.T) {
	e := newE2E(t)
	stdout, stderr, code := e.run("", nil, "doctor", "--driver", "fake", "--greeting", "hey")
	wantExit(t, code, 0, stderr)
	if !strings.HasSuffix(stdout, "\nfake: hey\nok\n") {
		t.Errorf("doctor printed %q", stdout)
	}

	_, stderr, code = e.run("", nil, "setup", "--driver", "fake")
	wantExit(t, code, 0, stderr)
	if _, err := os.Stat(filepath.Join(e.tmp, "warden-fake", "setup.done")); err != nil {
		t.Errorf("setup: %v", err)
	}

	stdout, stderr, code = e.run("", nil, "image", "prepare", "--driver", "fake")
	wantExit(t, code, 0, stderr)
	img := filepath.Join(e.tmp, "warden-fake", "images", "linux.img")
	wantIn(t, "image prepare", stdout+stderr, "Base image ready: "+img)

	stdout, stderr, code = e.run("", nil, "image", "verify", "--driver", "fake")
	wantExit(t, code, 0, stderr)
	if stdout != "ok linux.img\n" {
		t.Errorf("image verify printed %q", stdout)
	}

	stdout, stderr, code = e.run("", nil, "fake", "echo", "a", "--driver", "b")
	wantExit(t, code, 0, stderr)
	if stdout != "a --driver b\n" {
		t.Errorf("warden fake echo printed %q", stdout)
	}
}

// image list selects no driver and calls the bundled drivers' binaries; the
// fake stands in for one here, run by rootCmd in this process, since none
// implements image-list until K2 (DRV-26).
func TestE2EImageList(t *testing.T) {
	e := newE2E(t)
	img := filepath.Join(e.tmp, "warden-fake", "images", "linux.img")
	if err := os.MkdirAll(filepath.Dir(img), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(img, []byte("linux"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", e.bin)
	t.Setenv("TMPDIR", e.tmp)
	t.Setenv("TMP", e.tmp)
	t.Setenv("TEMP", e.tmp)
	// warden runs in this process, so the linked drivers read these: keep
	// them off the host's own image caches (hyperv's under LOCALAPPDATA).
	t.Setenv("HOME", filepath.Join(e.root, "home"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(e.root, "cache"))
	t.Setenv("LOCALAPPDATA", filepath.Join(e.root, "cache"))
	saved := bundledDrivers
	bundledDrivers = []string{"fake"}
	t.Cleanup(func() { bundledDrivers = saved })

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout := os.Stdout
	os.Stdout = w
	var printed bytes.Buffer
	copied := make(chan struct{})
	go func() {
		_, _ = io.Copy(&printed, r)
		close(copied)
	}()
	_, runErr := runWarden(t, "image", "list")
	os.Stdout = stdout
	_ = w.Close()
	<-copied
	if runErr != nil {
		t.Fatal(runErr)
	}
	if printed.String() != img+"\n" {
		t.Errorf("image list printed %q, want the fake's image", printed.String())
	}
}

// A shell whose attach fails: warden reports the driver's error, exits 1
// and the held build is torn down (DRV-28).
func TestE2EFailedAttach(t *testing.T) {
	e := newE2E(t)
	// With only warden's directory on PATH, the fake finds no shell to run.
	env := []string{"PATH=" + e.bin}
	_, stderr, code := e.run("", env, "shell", "--driver", "fake", "--tmpdir", e.tmp, e.ctx)
	wantExit(t, code, 1, stderr)
	wantIn(t, "stderr", stderr, "[warden] ERROR: driver \"fake\"")
	e.noLeftovers(0)
}

// The fake's own key in another letter case is refused by the fake in the
// probe: warden prints the error with its fix and exits 1 before it takes
// the build's lock or creates the output directory (DRV-C12, DRV-33).
func TestE2EKeyInAnotherCase(t *testing.T) {
	e := newE2E(t)
	cfg := filepath.Join(e.root, "warden.toml")
	if err := os.WriteFile(cfg, []byte("[driver.fake]\nGreeting = \"hi\"\n"),
		0o600); err != nil {
		t.Fatal(err)
	}
	_, stderr, code := e.run("", nil, e.buildArgs()...)
	wantExit(t, code, 1, stderr)
	wantIn(t, "stderr", stderr,
		"[warden] ERROR: "+cfg+": driver.fake.Greeting: keys are case-sensitive",
		"To fix: write it as driver.fake.greeting")
	if _, err := os.Stat(e.out()); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("the output directory was created: %v", err)
	}
	e.noLeftovers(0)
}

// A driver that writes a stray line on its standard output is a driver
// defect, shown with the tail of its standard error; nothing of the build is
// created (DRV-16, DRV-67).
func TestE2ENoisyDriver(t *testing.T) {
	e := newE2E(t)
	_, stderr, code := e.run("", []string{"WARDEN_FAKE_FAULT=stray-stdout"}, e.buildArgs()...)
	wantExit(t, code, 1, stderr)
	wantIn(t, "stderr", stderr, "[warden] ERROR: driver defect: the driver \"fake\"",
		"The driver's last output:", "This is a bug in the driver \"fake\"")
	if _, err := os.Stat(e.out()); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("the output directory was created: %v", err)
	}
	e.noLeftovers(0)
}
