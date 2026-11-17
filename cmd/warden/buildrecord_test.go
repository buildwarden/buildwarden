package main

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"sort"
	"testing"

	"github.com/buildwarden/buildwarden/driver"
)

// inLocks gives the test a user cache, and so a lock directory, of its own.
func inLocks(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	t.Setenv("LocalAppData", t.TempDir())
}

// deadBuild leaves the files of a build of drv whose warden died: a record,
// a state file, a stage directory under tmp and a lock nobody holds.
func deadBuild(t *testing.T, drv, id, tmp string, linked bool) buildRecord {
	t.Helper()
	rec := buildRecord{Driver: drv, ID: id, Path: "/opt/warden-driver-" + drv,
		Linked: linked, Contract: contractVersion, TempDir: tmp,
		StageDir:   stageDir(tmp, id),
		ConfigFile: filepath.Join(tmp, "warden.toml"), Args: []string{"--runtime=podman"}}
	if !linked {
		rec.SHA256 = "9f2c"
	}
	live, err := startBuildRecord(rec)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(rec.StageDir, 0o700); err != nil {
		t.Fatal(err)
	}
	write(t, rec.StageDir, "build.sh", "true\n")
	state := []byte(`{"runtime":"podman"}`)
	if err := os.WriteFile(live.statePath(), state, 0o600); err != nil {
		t.Fatal(err)
	}
	live.keep()
	return rec
}

// files lists the lock directory's names.
func lockDirFiles(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(driver.LockDir())
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)
	return names
}

// The record holds what a later warden needs and nothing else: no secret and
// no value of the config file, which it names (DRV-59).
func TestBuildRecordHoldsNoSecret(t *testing.T) {
	inLocks(t)
	live, err := startBuildRecord(buildRecord{Driver: "fake", ID: "k3v9x2ab",
		Path: "/opt/warden-driver-fake", SHA256: "9f2c", Contract: 1, TempDir: "/t",
		StageDir: "/t/warden-stage-k3v9x2ab", ConfigFile: "/p/warden.toml",
		Args: []string{"--greeting=hi"}})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = live.end() }()
	data, err := os.ReadFile(buildFiles("fake", "k3v9x2ab") + ".json")
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatal(err)
	}
	var names []string
	for k := range fields {
		names = append(names, k)
	}
	sort.Strings(names)
	want := []string{"args", "build_id", "config_file", "contract", "driver", "linked",
		"path", "sha256", "stage_dir", "tmpdir"}
	if !slices.Equal(names, want) {
		t.Errorf("record fields %v, want %v", names, want)
	}
	if got := reflect.TypeOf(buildRecord{}).NumField(); got != len(want) {
		t.Errorf("buildRecord has %d fields, the record %d", got, len(want))
	}
}

// A build takes its lock before anything of it is created: when the lock
// cannot be taken, no output directory and no stage directory exist
// (DRV-56).
func TestBuildLockTakenBeforeAnythingIsCreated(t *testing.T) {
	withoutProbe(t, "qemu")
	dir := inConfigFiles(t, "")
	write(t, dir, "build.sh", "true\n")
	cache := filepath.Join(t.TempDir(), "not-a-directory")
	write(t, filepath.Dir(cache), filepath.Base(cache), "")
	t.Setenv("XDG_CACHE_HOME", cache) // Linux
	t.Setenv("LocalAppData", cache)   // Windows
	if runtime.GOOS == "darwin" {
		t.Setenv("HOME", cache)
	}
	tmp, out := t.TempDir(), filepath.Join(t.TempDir(), "out")
	args := parseBuildLine(t, "build", "--driver", "qemu", "--tmpdir", tmp,
		"--output", out, dir)
	if _, err := prepare(args); err == nil {
		t.Fatal("prepare went on without the build's lock")
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Errorf("output directory created without the lock: %v", err)
	}
	if entries, _ := os.ReadDir(tmp); len(entries) > 0 {
		t.Errorf("stage directory created without the lock: %v", entries)
	}

	// With a lock directory, the lock is held and the record names the stage
	// directory the script was staged in, until the build ends.
	inLocks(t)
	args = parseBuildLine(t, "build", "--driver", "qemu", "--tmpdir", tmp,
		"--output", out, dir)
	p, err := prepare(args)
	if err != nil {
		t.Fatal(err)
	}
	req, done := p.req, p.done
	ids, err := heldBuildIDs("qemu")
	if err != nil || len(ids) != 1 {
		t.Fatalf("held builds %v, %v; want the one being prepared", ids, err)
	}
	rec, err := readRecord(buildFiles("qemu", ids[0])+".json", "qemu", ids[0])
	if err != nil {
		t.Fatal(err)
	}
	if rec.StageDir != filepath.Dir(req.BuildScript) || !rec.Linked ||
		rec.TempDir != tmp || rec.Contract != contractVersion {
		t.Errorf("record %+v for a build staged at %s", rec, req.BuildScript)
	}
	done()
	if names := lockDirFiles(t); len(names) != 0 {
		t.Errorf("a build that ended left %v", names)
	}
	if _, err := os.Stat(rec.StageDir); !os.IsNotExist(err) {
		t.Errorf("stage directory left: %v", err)
	}
}

// withoutProbe makes the linked driver d probe nothing for the test, so that
// prepare gets past the probe on a host without d's tools.
func withoutProbe(t *testing.T, d string) {
	t.Helper()
	orig := linkedDrivers[d]
	ld := *orig
	ld.Preflight = nil
	linkedDrivers[d] = &ld
	t.Cleanup(func() { linkedDrivers[d] = orig })
}

// parseBuildLine parses a command line's flags into warden's flag variables,
// as rootCmd does, and returns its positional arguments.
func parseBuildLine(t *testing.T, args ...string) []string {
	t.Helper()
	resetCommandFlags(t)
	c, rest, err := rootCmd.Find(args)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.ParseFlags(rest); err != nil {
		t.Fatal(err)
	}
	return c.Flags().Args()
}

// reap is given the record's values and the state file's path, not the
// invocation's; a failed reap keeps the state file, the record and the lock
// for the next clean, and a successful one lets clean remove them. The stage
// directory goes either way (DRV-57, DRV-60).
func TestCleanDeadBuildReap(t *testing.T) {
	inLocks(t)
	rec := deadBuild(t, "fake", "aaaaaaaa", t.TempDir(), false)
	var got buildRecord
	var gotState string
	failing := func(r buildRecord, state string) error {
		got, gotState = r, state
		return errors.New("the runtime does not answer")
	}
	n, err := cleanDeadBuilds("fake", failing, io.Discard)
	if err == nil || n != 0 {
		t.Fatalf("a failed reap: cleaned %d, err %v", n, err)
	}
	if !reflect.DeepEqual(got, rec) ||
		gotState != buildFiles("fake", "aaaaaaaa")+".state" {
		t.Errorf("reap given %+v, %s; want the record %+v", got, gotState, rec)
	}
	want := []string{"build-fake-aaaaaaaa.json", "build-fake-aaaaaaaa.lock",
		"build-fake-aaaaaaaa.state"}
	if names := lockDirFiles(t); !slices.Equal(names, want) {
		t.Errorf("after a failed reap the lock directory holds %v, want %v", names, want)
	}
	if _, err := os.Stat(rec.StageDir); !os.IsNotExist(err) {
		t.Errorf("stage directory kept: %v", err)
	}

	n, err = cleanDeadBuilds("fake", func(buildRecord, string) error { return nil },
		io.Discard)
	if err != nil || n != 1 {
		t.Fatalf("a successful reap: cleaned %d, err %v", n, err)
	}
	if names := lockDirFiles(t); len(names) != 0 {
		t.Errorf("after a successful reap the lock directory holds %v", names)
	}
}

// A dead build of a linked driver has no reap; clean removes its stage
// directory, record and lock all the same. A live build, another driver's
// build and a lock with no record are handled as each needs.
func TestCleanDeadBuildsLinked(t *testing.T) {
	inLocks(t)
	tmp := t.TempDir()
	dead := deadBuild(t, "qemu", "bbbbbbbb", tmp, true)
	deadBuild(t, "vz", "cccccccc", tmp, true)
	live, err := startBuildRecord(buildRecord{Driver: "qemu", ID: "dddddddd",
		Linked: true, StageDir: stageDir(tmp, "dddddddd")})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = live.end() }()
	orphan, err := driver.TakeBuildLock("qemu", "eeeeeeee")
	if err != nil {
		t.Fatal(err)
	}
	orphan.Release()

	reaped := false
	n, err := cleanDeadBuilds("qemu", func(buildRecord, string) error {
		reaped = true
		return nil
	}, io.Discard)
	if err != nil || n != 1 || reaped {
		t.Fatalf("cleaned %d, err %v, reap run %v; want 1, nil, false", n, err, reaped)
	}
	if _, err := os.Stat(dead.StageDir); !os.IsNotExist(err) {
		t.Errorf("a linked build's stage directory kept: %v", err)
	}
	want := []string{"build-qemu-dddddddd.json", "build-qemu-dddddddd.lock",
		"build-vz-cccccccc.json", "build-vz-cccccccc.lock", "build-vz-cccccccc.state"}
	if names := lockDirFiles(t); !slices.Equal(names, want) {
		t.Errorf("lock directory holds %v, want %v", names, want)
	}
	ids, err := heldBuildIDs("qemu")
	if err != nil || !slices.Equal(ids, []string{"dddddddd"}) {
		t.Errorf("held builds %v, %v; want the live one", ids, err)
	}
}

// A record that names a stage directory not of its build's name is not
// followed: nothing there is removed, and clean reports it.
func TestCleanDeadBuildStageDirMustBeTheBuilds(t *testing.T) {
	inLocks(t)
	precious := t.TempDir()
	write(t, precious, "keep", "x")
	rec := deadBuild(t, "qemu", "ffffffff", t.TempDir(), true)
	rec.StageDir = precious
	if err := writeRecord(buildFiles("qemu", "ffffffff")+".json", rec); err != nil {
		t.Fatal(err)
	}
	if _, err := cleanDeadBuilds("qemu", nil, io.Discard); err == nil {
		t.Error("a record naming another directory was cleaned without an error")
	}
	if _, err := os.Stat(filepath.Join(precious, "keep")); err != nil {
		t.Errorf("a directory the record named was removed: %v", err)
	}
}
