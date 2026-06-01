//go:build darwin && arm64 && integration

package vz

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// TestRelayVMBoot validates that the relay VM boots successfully using
// VZLinuxBootLoader with our Alpine kernel + initramfs.
//
// Prerequisites:
//   - Run tools/relay-vm/build-initramfs.sh first
//   - Cross-compile the relay: GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o <path> ./cmd/relay
//   - Binary must be codesigned with com.apple.security.virtualization entitlement
//
// Run: go test -tags integration -run TestRelayVMBoot ./driver/vz/ -v
func TestRelayVMBoot(t *testing.T) {
	// Locate relay VM assets (VZ needs uncompressed Image, not vmlinuz)
	repoRoot := findModuleRoot()
	kernelPath := filepath.Join(repoRoot, "tools", "relay-vm", "output", "Image")
	initrdPath := filepath.Join(repoRoot, "tools", "relay-vm", "output", "initramfs.cpio.gz")

	if _, err := os.Stat(kernelPath); err != nil {
		t.Skipf("relay VM kernel not found at %s; run tools/relay-vm/build-initramfs.sh", kernelPath)
	}
	if _, err := os.Stat(initrdPath); err != nil {
		t.Skipf("relay VM initrd not found at %s; run tools/relay-vm/build-initramfs.sh", initrdPath)
	}

	// Build relay binary for the VM
	relayBin := filepath.Join(t.TempDir(), "relay")
	cmd := exec.Command("go", "build", "-o", relayBin, "./cmd/relay")
	cmd.Dir = repoRoot
	cmd.Env = append(os.Environ(), "GOOS=linux", "GOARCH=arm64", "CGO_ENABLED=0")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("cross-compiling relay: %s: %v", string(out), err)
	}

	// Prepare shared directory
	sharedDir := t.TempDir()
	for _, sub := range []string{"ledger", "context"} {
		os.MkdirAll(filepath.Join(sharedDir, sub), 0755)
	}

	// Copy relay binary to shared volume
	if err := copyFile(relayBin, filepath.Join(sharedDir, "relay")); err != nil {
		t.Fatalf("copying relay: %v", err)
	}

	// Write minimal relay.env
	env := "LEDGER_DIR=/shared/ledger\nCONTEXT_DIR=/shared/context\n"
	os.WriteFile(filepath.Join(sharedDir, "relay.env"), []byte(env), 0644)

	// Create the VM
	vm, err := NewLinuxVM(linuxVMConfig{
		CPUs:               2,
		MemoryMB:           256,
		KernelPath:         kernelPath,
		InitrdPath:         initrdPath,
		Cmdline:            "console=hvc0",
		SharedDirPath:      sharedDir,
		SharedDirTag:       "shared",
		FileHandleSocketFD: -1,
		AttachNAT:          true,
	})
	if err != nil {
		t.Fatalf("creating VM: %v", err)
	}

	// Start the VM
	if err := vm.Start(); err != nil {
		t.Fatalf("starting VM: %v", err)
	}
	defer vm.Stop()

	// Give it time to boot and start the relay
	// The relay writes ca.cert.pem to the ledger dir when ready
	caPath := filepath.Join(sharedDir, "ledger", "ca.cert.pem")
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if info, err := os.Stat(caPath); err == nil && info.Size() > 0 {
			t.Logf("Relay VM booted and relay started (ca.cert.pem = %d bytes)", info.Size())
			return
		}
		time.Sleep(500 * time.Millisecond)
	}

	t.Fatalf("relay VM did not start within 30 seconds (no ca.cert.pem at %s)", caPath)
}
