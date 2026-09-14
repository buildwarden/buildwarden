//go:build windows && integration

package hyperv

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

// TestVMLifecycleSmoke validates CreateVM -> (exists) -> RemoveVM against the
// durable warden-dev switch, with the gateway/process running non-elevated
// under Hyper-V Administrators. It is the Phase 0 exit-criterion check:
// "warden --driver hyperv can create and destroy VMs against the reused dev
// switch with the gateway running non-elevated."
//
// Requires an actual Hyper-V host with the warden-dev switch present (run
// `warden hyperv setup` once, elevated) and VM-lifecycle privilege. It skips
// cleanly otherwise. Run with: go test -tags integration -run VMLifecycle ./driver/hyperv/
func TestVMLifecycleSmoke(t *testing.T) {
	caps := Detect(DefaultDevSwitch)
	if !caps.Supported {
		t.Skip("not a Windows host")
	}
	if !caps.DevSwitchPresent {
		t.Skipf("switch %q not present; run `warden hyperv setup` first", DefaultDevSwitch)
	}
	if caps.Resolve(OpVMLifecycle) != OutcomeDirect {
		t.Skip("VM lifecycle not available directly (need Hyper-V Administrators or elevation)")
	}

	ctx := context.Background()
	p := newLocalProvisioner(true)
	name := fmt.Sprintf("warden-selftest-%d", time.Now().UnixNano())

	h, err := p.CreateVM(ctx, VMSpec{
		Name:       name,
		Generation: 2,
		MemoryMB:   512,
		CPUs:       1,
		SwitchName: DefaultDevSwitch,
	})
	if err != nil {
		t.Fatalf("CreateVM: %v", err)
	}
	// Always attempt cleanup even if later assertions fail.
	t.Cleanup(func() {
		if err := p.RemoveVM(ctx, name); err != nil {
			t.Errorf("RemoveVM (cleanup): %v", err)
		}
	})

	if h == nil || h.Name != name {
		t.Fatalf("CreateVM returned handle %+v, want name %q", h, name)
	}

	// Confirm it exists.
	out, err := runPS(fmt.Sprintf("if (Get-VM -Name '%s' -ErrorAction SilentlyContinue) { 'yes' } else { 'no' }",
		psEscapeSingle(name)))
	if err != nil {
		t.Fatalf("Get-VM probe: %v", err)
	}
	if !strings.EqualFold(strings.TrimSpace(out), "yes") {
		t.Fatalf("VM %q not found after CreateVM", name)
	}

	// Explicit remove (also covered by cleanup; assert it clears).
	if err := p.RemoveVM(ctx, name); err != nil {
		t.Fatalf("RemoveVM: %v", err)
	}
	out, err = runPS(fmt.Sprintf("if (Get-VM -Name '%s' -ErrorAction SilentlyContinue) { 'yes' } else { 'no' }",
		psEscapeSingle(name)))
	if err != nil {
		t.Fatalf("Get-VM probe after remove: %v", err)
	}
	if !strings.EqualFold(strings.TrimSpace(out), "no") {
		t.Fatalf("VM %q still present after RemoveVM", name)
	}
}
