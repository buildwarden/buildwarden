package firecracker

import (
	"errors"
	"os"
	"strconv"
	"strings"
	"testing"
)

// fakeHost is a host whose commands, files and links are canned, and which
// records every command it is asked to run.
type fakeHost struct {
	ran     []string
	fail    map[string]bool   // a command line that fails
	outputs map[string]string // a command line's stdout
	files   map[string]string
	links   map[string]string
	missing map[string]bool // tools not on PATH
	env     map[string]string
	uid     int
	inKVM   bool
	noRW    bool
	noRefl  bool
	// heldLocks are lock files another process holds; locked and unlocked
	// record the ones taken and released.
	heldLocks map[string]bool
	locked    []string
	unlocked  []string
	// ranAtUnlock is len(ran) at each unlock, to show what ran under a lock.
	ranAtUnlock []int
	// onRun, when set, sees each command line run before it runs.
	onRun func(line string)
}

func newFakeHost() *fakeHost {
	return &fakeHost{
		fail: map[string]bool{}, outputs: map[string]string{}, files: map[string]string{},
		links: map[string]string{}, missing: map[string]bool{}, env: map[string]string{},
		heldLocks: map[string]bool{}, uid: 1000, inKVM: true,
	}
}

func (f *fakeHost) run(args []string) error {
	line := strings.Join(args, " ")
	if f.onRun != nil {
		f.onRun(line)
	}
	f.ran = append(f.ran, line)
	if f.fail[line] {
		return errors.New("exit status 1")
	}
	return nil
}

func (f *fakeHost) output(args []string) (string, error) {
	line := strings.Join(args, " ")
	f.ran = append(f.ran, line)
	if f.fail[line] {
		return "", errors.New("exit status 1")
	}
	return f.outputs[line], nil
}

func (f *fakeHost) readFile(path string) ([]byte, error) {
	if s, ok := f.files[path]; ok {
		return []byte(s), nil
	}
	return nil, os.ErrNotExist
}

func (f *fakeHost) readlink(path string) (string, error) {
	if s, ok := f.links[path]; ok {
		return s, nil
	}
	return "", os.ErrNotExist
}

func (f *fakeHost) exists(path string) bool {
	_, ok := f.files[path]
	return ok
}

func (f *fakeHost) openRW(string) error {
	if f.noRW {
		return os.ErrPermission
	}
	return nil
}

func (f *fakeHost) reflink(string) error {
	if f.noRefl {
		return errors.New("Operation not supported")
	}
	return nil
}

func (f *fakeHost) lookPath(name string) error {
	if f.missing[name] {
		return errors.New("not found")
	}
	return nil
}

func (f *fakeHost) getenv(key string) string { return f.env[key] }

func (f *fakeHost) euid() int { return f.uid }

func (f *fakeHost) lookupOwner(name string) (int, bool, error) {
	if name != "jeff" {
		return 0, false, errors.New("unknown user " + name)
	}
	return 1000, f.inKVM, nil
}

func (f *fakeHost) userHome(name string) (string, error) {
	if name != "jeff" {
		return "", errors.New("unknown user " + name)
	}
	return "/home/jeff", nil
}

// tryLock treats a lock file as existing when it is in files, like exists.
func (f *fakeHost) tryLock(path string) (func(), bool, error) {
	if _, ok := f.files[path]; !ok {
		return func() {}, false, nil
	}
	if f.heldLocks[path] {
		return nil, true, nil
	}
	f.locked = append(f.locked, path)
	return func() {
		f.unlocked = append(f.unlocked, path)
		f.ranAtUnlock = append(f.ranAtUnlock, len(f.ran))
	}, false, nil
}

// lockedNow reports whether path's lock is taken here and not yet released,
// so another process's flock on it would fail.
func (f *fakeHost) lockedNow(path string) bool {
	n := 0
	for _, p := range f.locked {
		if p == path {
			n++
		}
	}
	for _, p := range f.unlocked {
		if p == path {
			n--
		}
	}
	return n > 0
}

// lockAs creates a missing lock file, as jeff only, then is tryLock.
func (f *fakeHost) lockAs(path, name string) (func(), bool, error) {
	if name != "jeff" {
		return nil, false, errors.New("unknown user " + name)
	}
	if _, ok := f.files[path]; !ok {
		f.files[path] = ""
	}
	return f.tryLock(path)
}

// withSlots gives the fake n slots owned by uid.
func (f *fakeHost) withSlots(n, uid int) *fakeHost {
	for i := 0; i < n; i++ {
		s := Slot(i)
		for _, tap := range []string{s.RelayTap(), s.GuestTap(), s.UpstreamTap()} {
			f.files["/sys/class/net/"+tap] = ""
			f.files["/sys/class/net/"+tap+"/owner"] = strconv.Itoa(uid) + "\n"
		}
	}
	return f
}

// supportedHost makes hostCheck pass for one test.
func supportedHost(t *testing.T) {
	t.Helper()
	prev := hostCheck
	hostCheck = func() error { return nil }
	t.Cleanup(func() { hostCheck = prev })
}

func ranContains(f *fakeHost, sub string) bool {
	for _, l := range f.ran {
		if strings.Contains(l, sub) {
			return true
		}
	}
	return false
}
