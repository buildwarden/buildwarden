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
		// The thread takes the user's credentials and is never unlocked, so
		// the runtime discards it when the goroutine ends rather than reuse
		// it with them.
		runtime.LockOSThread()
		f, held, err := openLockAs(path, c)
		done <- result{f, held, err}
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

// openLockAs creates and locks path as driver.TryHostLock would, on a thread
// switched to c. The fs ids and groups are per-thread syscalls here (x/sys,
// unlike package syscall, does not apply them to every thread), and an fsuid
// other than 0 drops the thread's file capabilities, so every check on the
// path is the user's. A warden already running as the user switches nothing.
func openLockAs(path string, c cred) (*os.File, bool, error) {
	if os.Geteuid() != c.uid {
		if err := unix.Setgroups(c.groups); err != nil {
			return nil, false, fmt.Errorf("taking uid %d's groups: %w", c.uid, err)
		}
		// setfs[ug]id report failure only by not changing the id, so each is
		// read back by a second call.
		_, _ = unix.SetfsgidRetGid(c.gid)
		if prev, _ := unix.SetfsgidRetGid(c.gid); prev != c.gid {
			return nil, false, fmt.Errorf("taking gid %d failed", c.gid)
		}
		_, _ = unix.SetfsuidRetUid(c.uid)
		if prev, _ := unix.SetfsuidRetUid(c.uid); prev != c.uid {
			return nil, false, fmt.Errorf("taking uid %d failed", c.uid)
		}
	}
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
