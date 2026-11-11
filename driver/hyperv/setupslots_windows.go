//go:build windows

package hyperv

import "fmt"

// runSetupSlots runs setupSlotsScript (full Administrator: New-VMSwitch).
func runSetupSlots(network string, n int) (setupSlots, error) {
	out, err := runPS(setupSlotsScript(network, n))
	if err != nil {
		return setupSlots{}, fmt.Errorf("build switches: %w", err)
	}
	return parseSetupSlots(out), nil
}
