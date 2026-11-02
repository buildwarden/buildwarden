//go:build linux

package firecracker

import (
	"os"
	"strings"
	"syscall"
	"testing"
)

// TestVMMCommand pins the VMM's process attributes (Decision 8).
func TestVMMCommand(t *testing.T) {
	c := vmmCommand("/usr/bin/firecracker", "/w/relay.json", false)
	if got := strings.Join(c.Args,
		" "); got != "/usr/bin/firecracker --no-api --config-file /w/relay.json" {
		t.Fatalf("args = %q", got)
	}
	if c.Stdin != nil {
		t.Fatal("stdin is not /dev/null")
	}
	a := c.SysProcAttr
	if a == nil || !a.Setpgid || a.Pdeathsig != syscall.SIGKILL {
		t.Fatalf("attrs = %+v", a)
	}
	if k := vmmCommand("fc", "x", true).SysProcAttr; k.Pdeathsig != 0 || !k.Setpgid {
		t.Fatalf("--keep-vms attrs = %+v (kept VMs must outlive warden)", k)
	}
}

// TestProcessRecord checks that a running process's record matches it in
// liveVMMs, and that a pid /proc cannot describe fails instead of writing an
// entry nothing would ever match.
func TestProcessRecord(t *testing.T) {
	rec, err := processRecord(os.Getpid(), true)
	if err != nil {
		t.Fatal(err)
	}
	if live := liveVMMs([]vmmRecord{rec}, os.ReadFile, os.Readlink); len(live) != 1 ||
		!live[0].keep {
		t.Fatalf("this process's record %+v is not live", rec)
	}
	if _, err := processRecord(1<<30, false); err == nil {
		t.Fatal("a pid with no /proc entry was recorded")
	}
}
