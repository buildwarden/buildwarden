//go:build linux

package firecracker

import (
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
