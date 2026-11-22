package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/buildwarden/buildwarden/driver"
)

// lookupDirs are the directories of a lookup test: three PATH directories
// and warden's own.
type lookupDirs struct {
	a, b, c, self string
}

func newLookupDirs(t *testing.T) lookupDirs {
	t.Helper()
	root := t.TempDir()
	d := lookupDirs{a: filepath.Join(root, "a"), b: filepath.Join(root, "b"),
		c: filepath.Join(root, "c"), self: filepath.Join(root, "self")}
	for _, p := range []string{d.a, d.b, d.c, d.self} {
		if err := os.Mkdir(p, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return d
}

// put creates driver name's file in dir, executable unless mode says not.
func put(t *testing.T, dir, name string, mode os.FileMode) string {
	t.Helper()
	p := filepath.Join(dir, driverFileName(name, isWindows))
	if err := os.WriteFile(p, []byte("#!/bin/sh\n"), mode); err != nil {
		t.Fatal(err)
	}
	return p
}

func (d lookupDirs) env(path ...string) lookupEnv {
	return lookupEnv{path: strings.Join(path, string(os.PathListSeparator)),
		selfDir: d.self, self: filepath.Join(d.self, "warden"), windows: isWindows,
		linked: func(n string) bool { return n == "qemu" }}
}

// The lookup's order: a path, PATH in order, warden's directory, a linked
// driver (DRV-6); the bundled copy by identity, listed once (DRV-10).
func TestDriverLookupOrder(t *testing.T) {
	d := newLookupDirs(t)
	inA, inB, inSelf := put(t, d.a, "nomad", 0o755), put(t, d.b, "nomad", 0o755),
		put(t, d.self, "nomad", 0o755)
	qemuInA := put(t, d.a, "qemu", 0o755)
	for _, tc := range []struct {
		name     string
		sel      selection
		env      lookupEnv
		path     string
		found    string
		shadowed []driverCopy
	}{
		{"first PATH entry wins", selection{value: "nomad"}, d.env(d.a, d.b), inA,
			foundPATH, []driverCopy{{inB, foundPATH}, {inSelf, foundBeside}}},
		{"a bundled name shadowed from an earlier entry", selection{value: "nomad"},
			d.env(d.a, d.self), inA, foundPATH, []driverCopy{{inSelf, foundBeside}}},
		{"not shadowed from a later entry", selection{value: "nomad"},
			d.env(d.self, d.a), inSelf, foundBeside, []driverCopy{{inA, foundPATH}}},
		{"warden's directory on PATH twice, listed once", selection{value: "nomad"},
			d.env(d.self, d.self), inSelf, foundBeside, nil},
		{"warden's directory when PATH has none", selection{value: "nomad"},
			d.env(d.c), inSelf, foundBeside, nil},
		{"a linked name replaced by a file", selection{value: "qemu"}, d.env(d.a),
			qemuInA, foundPATH, []driverCopy{{filepath.Join(d.self, "warden"),
				foundLinked}}},
		{"a linked driver last", selection{value: "qemu"}, d.env(d.b),
			filepath.Join(d.self, "warden"), foundLinked, nil},
		{"a path is used as given", selection{value: inB, source: sourceFlag},
			d.env(d.a), inB, foundPath, []driverCopy{{inA, foundPATH},
				{inSelf, foundBeside}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fd, err := findDriver(tc.sel, tc.env)
			if err != nil {
				t.Fatal(err)
			}
			if fd.Path != tc.path || fd.Found != tc.found {
				t.Errorf("found %s (%s), want %s (%s)", fd.Path, fd.Found, tc.path,
					tc.found)
			}
			if len(fd.Shadowed) != len(tc.shadowed) {
				t.Fatalf("shadowed %v, want %v", fd.Shadowed, tc.shadowed)
			}
			for i := range tc.shadowed {
				if fd.Shadowed[i] != tc.shadowed[i] {
					t.Errorf("shadowed %v, want %v", fd.Shadowed, tc.shadowed)
				}
			}
		})
	}
}

// warden's directory reached through a symbolic link is still the bundled
// copy, and a path through a link is followed (DRV-7, DRV-10).
func TestDriverLookupSymlinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symbolic links need privileges on Windows")
	}
	d := newLookupDirs(t)
	inSelf := put(t, d.self, "nomad", 0o755)
	link := filepath.Join(filepath.Dir(d.self), "link")
	if err := os.Symlink(d.self, link); err != nil {
		t.Fatal(err)
	}
	fd, err := findDriver(selection{value: "nomad"}, d.env(link, d.self))
	if err != nil {
		t.Fatal(err)
	}
	if fd.Found != foundBeside || len(fd.Shadowed) != 0 {
		t.Errorf("found %+v, want the bundled copy once", fd)
	}
	file := filepath.Join(d.a, driverFileName("nomad", false))
	if err := os.Symlink(inSelf, file); err != nil {
		t.Fatal(err)
	}
	fd, err = findDriver(selection{value: file, source: sourceEnv}, d.env())
	if err != nil || fd.Path != file || fd.Found != foundPath {
		t.Errorf("a path through a link: %+v, %v", fd, err)
	}
}

// A directory, and a file that cannot be executed, are passed over, listed
// as skipped and named in the not-found error (DRV-8).
func TestDriverLookupSkips(t *testing.T) {
	d := newLookupDirs(t)
	dirNamed := filepath.Join(d.a, driverFileName("nomad", isWindows))
	if err := os.Mkdir(dirNamed, 0o755); err != nil {
		t.Fatal(err)
	}
	want := 1
	inC := put(t, d.c, "nomad", 0o755)
	if !isWindows {
		put(t, d.b, "nomad", 0o644)
		want = 2
	}
	fd, err := findDriver(selection{value: "nomad"}, d.env(d.a, d.b, d.c))
	if err != nil {
		t.Fatal(err)
	}
	if fd.Path != inC || len(fd.Skipped) != want {
		t.Errorf("found %s, skipped %v; want %s and %d skipped", fd.Path, fd.Skipped,
			inC, want)
	}
	if err := os.Remove(inC); err != nil {
		t.Fatal(err)
	}
	_, err = findDriver(selection{value: "nomad"}, d.env(d.a, d.b, d.c))
	if err == nil || !strings.Contains(err.Error(), "passed over") ||
		!strings.Contains(err.Error(), "is a directory") {
		t.Errorf("not found: %v, want the skipped files named", err)
	}
}

// A candidate through a relative PATH entry, or an empty one on Unix, stops
// the lookup with the entry named (DRV-9).
func TestDriverLookupRelativeEntry(t *testing.T) {
	d := newLookupDirs(t)
	put(t, d.a, "nomad", 0o755)
	put(t, d.b, "nomad", 0o755)
	t.Chdir(filepath.Dir(d.a))
	_, err := findDriver(selection{value: "nomad"}, d.env("a", d.b))
	if err == nil || !strings.Contains(err.Error(), "relative PATH entry a") {
		t.Errorf("relative entry: %v, want it refused and named", err)
	}
	if isWindows {
		return
	}
	t.Chdir(d.a)
	_, err = findDriver(selection{value: "nomad"}, d.env("", d.b))
	if err == nil || !strings.Contains(err.Error(), "empty entry") {
		t.Errorf("empty entry: %v, want it refused", err)
	}
}

// No config file names a driver by path, whichever way the file was found
// (DRV-4); a path from the flag or the variable must be a driver's file that
// can be run (DRV-7).
func TestDriverLookupPaths(t *testing.T) {
	d := newLookupDirs(t)
	inA := put(t, d.a, "nomad", 0o755)
	for _, file := range []string{"/project/warden.toml", "/elsewhere/named.toml"} {
		_, err := findDriver(selection{value: inA, source: sourceConfig, file: file},
			d.env())
		var fe *driver.FixableError
		if !errors.As(err, &fe) ||
			!strings.Contains(err.Error(), "names a driver, not a file") ||
			!strings.Contains(fe.Fix, "--driver or WARDEN_DRIVER") {
			t.Errorf("a path in %s: %v, want it refused", file, err)
		}
	}
	other := filepath.Join(d.a, "nomad")
	if err := os.WriteFile(other, nil, 0o755); err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]string{
		other: "is not a driver's file",
		filepath.Join(d.b, driverFileName("gone", isWindows)): "does not exist",
	} {
		_, err := findDriver(selection{value: path, source: sourceFlag}, d.env())
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: %v, want %q", path, err, want)
		}
	}
	if isWindows {
		bat := filepath.Join(d.a, "warden-driver-nomad.bat")
		if err := os.WriteFile(bat, nil, 0o755); err != nil {
			t.Fatal(err)
		}
		_, err := findDriver(selection{value: bat, source: sourceFlag}, d.env())
		if err == nil {
			t.Error("a batch file was taken for a driver (DRV-2)")
		}
	}
	if _, err := findDriver(selection{value: "Nomad"}, d.env()); err == nil {
		t.Error("a name with a capital letter was looked up")
	}
}

// driverListFixture is a lookup in which container is on PATH and beside
// warden, nomad on PATH, and (on Unix) an unrunnable nomad earlier on PATH.
type driverListFixture struct {
	env         lookupEnv
	inA, inSelf string
}

func newDriverListFixture(t *testing.T) driverListFixture {
	t.Helper()
	d := newLookupDirs(t)
	f := driverListFixture{env: d.env(d.a, d.b), inA: put(t, d.a, "container", 0o755),
		inSelf: put(t, d.self, "container", 0o755)}
	put(t, d.b, "nomad", 0o755)
	if !isWindows {
		put(t, d.a, "nomad", 0o644)
	}
	return f
}

// driver list: the bundled names and every warden-driver-* found, the
// selected one marked, and shadowed and skipped copies beneath.
func TestDriverList(t *testing.T) {
	f := newDriverListFixture(t)
	list, err := listDrivers(f.env, &selection{value: "container", source: sourceDflt})
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]listedDriver{}
	var names []string
	for _, l := range list {
		byName[l.Name] = l
		names = append(names, l.Name)
	}
	if strings.Join(names, " ") != "container qemu vz hyperv firecracker nomad" {
		t.Errorf("names %v", names)
	}
	c := byName["container"]
	if !c.Selected || c.Path != f.inA || c.Found != foundPATH ||
		len(c.Shadowed) != 1 || c.Shadowed[0] != (driverCopy{f.inSelf, foundBeside}) {
		t.Errorf("container %+v", c)
	}
	if q := byName["qemu"]; q.Found != foundLinked || q.Selected {
		t.Errorf("qemu %+v", q)
	}
	if n := byName["nomad"]; !isWindows && len(n.Skipped) != 1 {
		t.Errorf("nomad %+v, want its unrunnable copy skipped", n)
	}
	var buf strings.Builder
	printDriverList(&buf, list)
	if !strings.Contains(buf.String(), "shadows "+f.inSelf+" (beside warden)") {
		t.Errorf("driver list:\n%s", buf.String())
	}
}

// A path from the flag or the variable is listed under its name, with what
// the name would have found shadowed, and --json prints every field.
func TestDriverListPathSelection(t *testing.T) {
	f := newDriverListFixture(t)
	for _, src := range []string{sourceFlag, sourceEnv} {
		list, err := listDrivers(f.env, &selection{value: f.inSelf, source: src})
		if err != nil {
			t.Fatal(err)
		}
		c := list[0]
		if !c.Selected || c.Path != f.inSelf || c.Found != foundPath || c.Source != src ||
			len(c.Shadowed) == 0 || c.Shadowed[0].Path != f.inA {
			t.Errorf("a path from %s: %+v", src, c)
		}
		var buf strings.Builder
		if err := printDriverListJSON(&buf, list); err != nil {
			t.Fatal(err)
		}
		var got struct {
			Drivers []map[string]any `json:"drivers"`
		}
		if err := json.Unmarshal([]byte(buf.String()), &got); err != nil {
			t.Fatal(err)
		}
		for _, k := range []string{"name", "path", "found", "source", "selected",
			"shadowed", "skipped"} {
			if _, ok := got.Drivers[0][k]; !ok {
				t.Errorf("driver list --json has no %s: %s", k, buf.String())
			}
		}
	}
}

// driverCmd's lookup environment is this process's: PATH, and warden's own
// directory with links resolved.
func TestHostLookupEnv(t *testing.T) {
	env := hostLookupEnv()
	if env.selfDir == "" || !env.linked("qemu") || env.linked("nomad") {
		t.Errorf("host lookup environment %+v", env)
	}
}
