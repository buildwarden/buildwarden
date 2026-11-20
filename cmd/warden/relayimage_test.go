package main

import (
	"strings"
	"testing"
)

// The relay image is the container driver's (driver/container's
// TestRelayImageShape); a VM driver's build does not check it.
func TestRelayImageNotCheckedUnderAVMDriver(t *testing.T) {
	inConfigFiles(t, "[runtime]\nrelay_image = \"--volume=/:/host\"\n")
	flagDriver = "qemu"
	captureLog(t)
	cfg, err := resolveConfig()
	if err != nil {
		t.Fatal(err)
	}
	_, err = newRequest(cfg, linkedSel(cfg), []string{buildProject(t)})
	if err != nil && strings.Contains(err.Error(), "relay_image") {
		t.Errorf("a VM driver's build checked the relay image: %v", err)
	}
}
