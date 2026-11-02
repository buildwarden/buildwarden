//go:build linux

package firecracker

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"syscall"
	"time"

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
