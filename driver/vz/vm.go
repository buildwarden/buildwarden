package vz

import (
	"fmt"
	"unsafe"
)

// VM states (mirror VZVirtualMachine.State)
const (
	StateStopped  = 0
	StateRunning  = 1
	StatePaused   = 2
	StateError    = 3
	StateStarting = 4
	StateStopping = 5
)

// VM represents a running virtual machine.
type VM struct {
	handle unsafe.Pointer
	kind   string // "linux" or "macos"
}

// LinuxVMConfig is the exported form for external test harnesses.
type LinuxVMConfig = linuxVMConfig

// MacOSVMConfig is the exported form for external test harnesses.
type MacOSVMConfig = macOSVMConfig

// NewLinuxVM creates a Linux VM (used for the relay).
// Boots directly from kernel + initrd — no disk image needed.
func NewLinuxVM(cfg linuxVMConfig) (*VM, error) {
	handle, err := createLinuxVM(cfg)
	if err != nil {
		return nil, err
	}
	return &VM{handle: handle, kind: "linux"}, nil
}

// NewMacOSVM creates a macOS VM (used for builds).
// Requires a restored disk image + platform state files from IPSW install.
func NewMacOSVM(cfg macOSVMConfig) (*VM, error) {
	handle, err := createMacOSVM(cfg)
	if err != nil {
		return nil, err
	}
	return &VM{handle: handle, kind: "macos"}, nil
}

// Start boots the virtual machine.
func (vm *VM) Start() error {
	if vm.handle == nil {
		return fmt.Errorf("VM not initialized")
	}
	return startVM(vm.handle)
}

// Stop gracefully shuts down the virtual machine.
func (vm *VM) Stop() error {
	if vm.handle == nil {
		return nil
	}
	state := vmState(vm.handle)
	if state == StateStopped || state == StateError {
		return nil
	}
	return stopVM(vm.handle)
}

// State returns the current VM state.
func (vm *VM) State() int {
	if vm.handle == nil {
		return StateStopped
	}
	return vmState(vm.handle)
}

// IsRunning returns true if the VM is in the running state.
func (vm *VM) IsRunning() bool {
	return vm.State() == StateRunning
}
