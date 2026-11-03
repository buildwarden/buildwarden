//go:build linux

package firecracker

import (
	"errors"
	"io/fs"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"

	"github.com/buildwarden/buildwarden/driver"
)

// TestRealTryLock checks the lock doctor takes on a user's slot lock files: a
// held lock is reported, a free one taken, and a missing file is free without
// being created.
func TestRealTryLock(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "missing.lock")
	unlock, held, err := realHost{}.tryLock(missing)
	if err != nil || held {
		t.Fatalf("missing lock: held %v, err %v", held, err)
	}
	unlock()
	if _, err := os.Stat(missing); !os.IsNotExist(err) {
		t.Fatalf("tryLock created %s", missing)
	}

	path := filepath.Join(dir, "slot.lock")
	if err := os.WriteFile(path, nil, 0o400); err != nil {
		t.Fatal(err)
	}
	unlock, held, err = realHost{}.tryLock(path)
	if err != nil || held {
		t.Fatalf("free lock: held %v, err %v", held, err)
	}
	// flock locks belong to the open file, so a second open conflicts even in
	// this process.
	if _, held, err := (realHost{}).tryLock(path); err != nil || !held {
		t.Fatalf("held lock: held %v, err %v", held, err)
	}
	unlock()
	unlock, held, err = realHost{}.tryLock(path)
	if err != nil || held {
		t.Fatalf("released lock: held %v, err %v", held, err)
	}
	unlock()
}

// TestLockAsExcludesBuild checks that a slot setup holds with lockAs, on a
// slot no build has used (no lock file, no lock dir), is the lock a build's
// acquireSlot takes: the build is refused until setup lets go.
func TestLockAsExcludesBuild(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	me, err := user.Current()
	if err != nil {
		t.Fatal(err)
	}
	s := Slot(3)
	path := filepath.Join(driver.LockDir(), s.LockName()+".lock")
	unlock, held, err := realHost{}.lockAs(path, me.Username)
	if err != nil || held {
		t.Fatalf("lockAs: held %v, err %v", held, err)
	}
	for p, want := range map[string]os.FileMode{
		path: 0o600, filepath.Dir(path): 0o700 | os.ModeDir,
	} {
		if fi, err := os.Stat(p); err != nil || fi.Mode() != want {
			t.Fatalf("%s: %v, %v; want mode %v", p, fi, err, want)
		}
	}
	var fe *driver.FixableError
	if _, _, err := acquireSlot([]Slot{s}); !errors.As(err, &fe) {
		t.Fatalf("build during setup: %v, want all slots in use", err)
	}
	unlock()
	got, release, err := acquireSlot([]Slot{s})
	if err != nil || got != s {
		t.Fatalf("build after setup: slot %d, %v", got, err)
	}
	release()
}

// TestLockAsUsesUserCreds checks, as root, that lockAs acts with the user's
// credentials only: it cannot create in a directory only root may write, it
// creates files the user owns where the user may, and every thread of warden,
// including one the runtime parks rather than discards, stays root.
func TestLockAsUsesUserCreds(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("needs root")
	}
	uid, gid := nobodyIDs(t)
	groups := rootGroups(t)
	// Not t.TempDir, whose 0700 parent nobody could not search.
	root, err := os.MkdirTemp("", "lockas")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(root) })
	if err := os.Chmod(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, _, err := (realHost{}).lockAs(filepath.Join(root, "locks", "x.lock"),
		"nobody"); !errors.Is(err, os.ErrPermission) {
		t.Fatalf("lockAs in root's dir: %v, want permission denied", err)
	}
	allTasksRoot(t, groups)
	own := filepath.Join(root, "own")
	if err := os.Mkdir(own, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chown(own, uid, gid); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(own, "locks", "x.lock")
	unlock, held, err := realHost{}.lockAs(path, "nobody")
	if err != nil || held {
		t.Fatalf("lockAs in nobody's dir: held %v, err %v", held, err)
	}
	unlock()
	// A thread a call leaves locked is replaced, so over repeated calls one
	// lands on the main thread, which the runtime parks rather than discard.
	relock(t, path, 20)
	allTasksRoot(t, groups)
	for _, p := range []string{path, filepath.Dir(path)} {
		if u, g := ownerOf(t, p); u != uid || g != gid {
			t.Fatalf("%s is owned by %d:%d, want %d:%d", p, u, g, uid, gid)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "still-root"), nil, 0o600); err != nil ||
		os.Geteuid() != 0 {
		t.Fatalf("warden lost root after lockAs: %v", err)
	}
}

// relock takes and releases the lock at path as nobody n times.
func relock(t *testing.T, path string, n int) {
	t.Helper()
	for range n {
		unlock, _, err := realHost{}.lockAs(path, "nobody")
		if err != nil {
			t.Fatal(err)
		}
		unlock()
	}
}

// rootGroups returns warden's groups as /proc/<task>/status lists them. A
// thread lockAs leaves with the user's groups never runs Go code again, so
// whichever thread asks reads root's.
func rootGroups(t *testing.T) string {
	t.Helper()
	ids, err := os.Getgroups()
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, id := range ids {
		out = append(out, strconv.Itoa(id))
	}
	return strings.Join(out, " ")
}

// allTasksRoot fails t unless every thread of this process has uid and gid 0
// (real, effective, saved and fs) and root's groups. A thread that exits
// while being read holds no ids and is skipped.
func allTasksRoot(t *testing.T, groups string) {
	t.Helper()
	tasks, err := os.ReadDir("/proc/self/task")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range tasks {
		st, ok := taskStatus(t, "self/task/"+e.Name())
		if !ok {
			continue
		}
		for _, k := range []string{"Uid", "Gid"} {
			if st[k] != "0 0 0 0" {
				t.Errorf("thread %s: %s %q, want all 0", e.Name(), k, st[k])
			}
		}
		if st["Groups"] != groups {
			t.Errorf("thread %s: Groups %q, want %q", e.Name(), st["Groups"], groups)
		}
	}
}

// taskStatus reads /proc/<task>/status into its keys, each value's fields
// joined by one space; ok is false if the task is gone.
func taskStatus(t *testing.T, task string) (st map[string]string, ok bool) {
	t.Helper()
	b, err := os.ReadFile("/proc/" + task + "/status")
	if errors.Is(err, fs.ErrNotExist) || errors.Is(err, syscall.ESRCH) {
		return nil, false
	}
	if err != nil {
		t.Fatal(err)
	}
	st = map[string]string{}
	for _, l := range strings.Split(string(b), "\n") {
		if k, v, found := strings.Cut(l, ":"); found {
			st[k] = strings.Join(strings.Fields(v), " ")
		}
	}
	return st, true
}

func nobodyIDs(t *testing.T) (uid, gid int) {
	t.Helper()
	u, err := user.Lookup("nobody")
	if err != nil {
		t.Skip("no nobody user")
	}
	uid, err = strconv.Atoi(u.Uid)
	if err == nil {
		gid, err = strconv.Atoi(u.Gid)
	}
	if err != nil {
		t.Fatal(err)
	}
	return uid, gid
}

func ownerOf(t *testing.T, path string) (uid, gid int) {
	t.Helper()
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		t.Fatalf("%s: no stat_t", path)
	}
	return int(st.Uid), int(st.Gid)
}
