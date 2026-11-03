//go:build linux

package firecracker

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"syscall"
	"time"

	"golang.org/x/sys/unix"

	"github.com/buildwarden/buildwarden/driver"
)

// acquireSlot takes the first free slot of slots under its host lock. A slot
// whose recorded VMMs outlived their warden has them killed first (they hold
// its TAPs); one still held by VMs a --keep-vms build left is skipped. The
// returned function releases the slot.
func acquireSlot(slots []Slot) (Slot, func(), error) {
	for _, s := range slots {
		unlock, ok, err := driver.TryHostLock(s.LockName())
		if err != nil {
			return 0, nil, err
		}
		if !ok {
			continue
		}
		live := slotVMMs(s)
		if anyKept(live) {
			unlock()
			continue
		}
		if len(live) > 0 {
			fmt.Fprintf(os.Stderr, "[warden] slot %d: killing %d VMM(s) left by a "+
				"warden that did not exit cleanly\n", s, len(live))
			if err := killVMMs(live); err != nil {
				unlock()
				return 0, nil, err
			}
		}
		_ = os.Remove(pidFilePath(s))
		return s, unlock, nil
	}
	return 0, nil, driver.Fixable(
		"wait for a running build to finish, stop VMs left by --keep-vms (warden clean "+
			"--driver firecracker), or add slots: sudo warden setup --driver "+
			"firecracker --slots N",
		"all %d firecracker slots are in use", len(slots))
}

// slotVMMs returns the VMMs recorded for s that are still running.
func slotVMMs(s Slot) []vmmRecord {
	b, err := os.ReadFile(pidFilePath(s))
	if err != nil {
		return nil
	}
	return liveVMMs(parsePidFile(string(b)), os.ReadFile, os.Readlink)
}

func anyKept(recs []vmmRecord) bool {
	for _, r := range recs {
		if r.keep {
			return true
		}
	}
	return false
}

// killVMMs SIGKILLs each VMM and waits for it to be gone.
func killVMMs(recs []vmmRecord) error {
	for _, r := range recs {
		if err := syscall.Kill(r.pid, syscall.SIGKILL); err != nil && !errors.Is(err,
			syscall.ESRCH) {
			return fmt.Errorf("killing stale VMM %d: %w", r.pid, err)
		}
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if len(liveVMMs(recs, os.ReadFile, os.Readlink)) == 0 {
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return fmt.Errorf("stale VMMs %v did not exit after SIGKILL", pids(recs))
}

func pids(recs []vmmRecord) []string {
	var out []string
	for _, r := range recs {
		out = append(out, strconv.Itoa(r.pid))
	}
	return out
}

// writePidFile records the VMMs running on s.
func writePidFile(s Slot, vms ...*vmm) error {
	var recs []vmmRecord
	for _, v := range vms {
		recs = append(recs, v.rec)
	}
	if err := os.MkdirAll(driver.LockDir(), 0o700); err != nil {
		return err
	}
	return os.WriteFile(pidFilePath(s), []byte(formatPidFile(recs)), 0o600)
}

func (realHost) tryLock(path string) (func(), bool, error) {
	// flock needs no write access, so O_RDONLY locks a file root does not own
	// without changing it.
	f, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return func() {}, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, true, nil
		}
		return nil, false, fmt.Errorf("locking %s: %w", path, err)
	}
	return func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		f.Close()
	}, false, nil
}

func (realHost) lockAs(path, name string) (func(), bool, error) {
	c, err := lookupCred(name)
	if err != nil {
		return nil, false, err
	}
	type result struct {
		f    *os.File
		held bool
		err  error
	}
	done := make(chan result, 1)
	go func() {
		// The thread takes the user's fs ids and groups, and gives them back
		// before the goroutine ends: the runtime discards a thread still
		// locked then, except the main thread, which it parks for good with
		// whatever ids it holds. Only a thread whose root ids were restored
		// and read back is unlocked for reuse. One that could not be restored
		// stays locked; parked or discarded, it runs no Go code again, and
		// its uid, euid and suid stay 0, so the user gains nothing over it.
		runtime.LockOSThread()
		var r result
		restore, err := switchCreds(c)
		if err == nil {
			r.f, r.held, r.err = openLockAs(path)
		} else {
			r.err = err
		}
		if err := restore(); err != nil {
			if r.f != nil {
				r.f.Close()
			}
			r = result{err: errors.Join(r.err, err)}
		} else {
			runtime.UnlockOSThread()
		}
		done <- r
	}()
	r := <-done
	if r.err != nil || r.held {
		return nil, r.held, r.err
	}
	return func() {
		_ = syscall.Flock(int(r.f.Fd()), syscall.LOCK_UN)
		r.f.Close()
	}, false, nil
}

// cred is a user's filesystem identity.
type cred struct {
	uid, gid int
	groups   []int
}

func lookupCred(name string) (cred, error) {
	u, err := user.Lookup(name)
	if err != nil {
		return cred{}, fmt.Errorf("looking up %s: %w", name, err)
	}
	var c cred
	if c.uid, err = strconv.Atoi(u.Uid); err != nil {
		return cred{}, fmt.Errorf("uid of %s: %w", name, err)
	}
	if c.gid, err = strconv.Atoi(u.Gid); err != nil {
		return cred{}, fmt.Errorf("gid of %s: %w", name, err)
	}
	ids, err := u.GroupIds()
	if err != nil {
		return cred{}, fmt.Errorf("groups of %s: %w", name, err)
	}
	for _, id := range ids {
		g, err := strconv.Atoi(id)
		if err != nil {
			return cred{}, fmt.Errorf("group %q of %s: %w", id, name, err)
		}
		c.groups = append(c.groups, g)
	}
	return c, nil
}

// switchCreds switches the calling thread's fs ids and groups to c. The fs ids
// and groups are per-thread syscalls here (x/sys, unlike package syscall, does
// not apply them to every thread), and an fsuid other than 0 drops the thread's
// file capabilities, so every check on a path is then the user's. A warden
// already running as the user switches nothing. The returned function gives
// the thread back the ids and groups it had, reading each back, and is to be
// called even when switchCreds fails part way.
func switchCreds(c cred) (restore func() error, err error) {
	if os.Geteuid() == c.uid {
		return func() error { return nil }, nil
	}
	groups, err := unix.Getgroups()
	if err != nil {
		return func() error { return nil }, fmt.Errorf("reading groups: %w", err)
	}
	fsuid, fsgid := os.Geteuid(), os.Getegid()
	restore = func() error {
		if err := setFsuid(fsuid); err != nil {
			return fmt.Errorf("restoring uid %d: %w", fsuid, err)
		}
		if err := setFsgid(fsgid); err != nil {
			return fmt.Errorf("restoring gid %d: %w", fsgid, err)
		}
		if err := unix.Setgroups(groups); err != nil {
			return fmt.Errorf("restoring groups: %w", err)
		}
		got, err := unix.Getgroups()
		if err != nil {
			return fmt.Errorf("reading groups back: %w", err)
		}
		if !slices.Equal(got, groups) {
			return fmt.Errorf("restoring groups: got %v, want %v", got, groups)
		}
		return nil
	}
	if err := unix.Setgroups(c.groups); err != nil {
		return restore, fmt.Errorf("taking uid %d's groups: %w", c.uid, err)
	}
	if err := setFsgid(c.gid); err != nil {
		return restore, fmt.Errorf("taking gid %d: %w", c.gid, err)
	}
	if err := setFsuid(c.uid); err != nil {
		return restore, fmt.Errorf("taking uid %d: %w", c.uid, err)
	}
	return restore, nil
}

// setFsuid and setFsgid set the thread's fs id. setfs[ug]id report failure
// only by not changing the id, so each is read back by a second call.
func setFsuid(id int) error {
	_, _ = unix.SetfsuidRetUid(id)
	if prev, _ := unix.SetfsuidRetUid(id); prev != id {
		return fmt.Errorf("fsuid is %d", prev)
	}
	return nil
}

func setFsgid(id int) error {
	_, _ = unix.SetfsgidRetGid(id)
	if prev, _ := unix.SetfsgidRetGid(id); prev != id {
		return fmt.Errorf("fsgid is %d", prev)
	}
	return nil
}

// openLockAs creates and locks path as driver.TryHostLock would, on a thread
// switchCreds has switched to the lock's owner.
func openLockAs(path string) (*os.File, bool, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, false, fmt.Errorf("creating lock dir: %w", err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, false, fmt.Errorf("opening lock %s: %w", path, err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, true, nil
		}
		return nil, false, fmt.Errorf("locking %s: %w", path, err)
	}
	return f, false, nil
}
