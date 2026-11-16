//go:build !darwin || !arm64 || !cgo

// The stand-ins for the Virtualization.framework bindings, for every build
// that does not compile them: any target other than darwin/arm64, and
// darwin/arm64 without cgo (a file that imports "C" is left out of every
// build with cgo off). Its constraint is the complement of the bindings'.

package vz

import "unsafe"

// hasBindings reports whether this build compiles the Virtualization.framework
// bindings. checkPlatform refuses the driver when it is false.
const hasBindings = false

func createLinuxVM(_ linuxVMConfig) (unsafe.Pointer, error) {
	return nil, checkPlatform()
}

func createMacOSVM(_ macOSVMConfig) (unsafe.Pointer, error) {
	return nil, checkPlatform()
}

func startVM(_ unsafe.Pointer) error {
	return checkPlatform()
}

func stopVM(_ unsafe.Pointer) error {
	return checkPlatform()
}

func vmState(_ unsafe.Pointer) int {
	return 0
}

func latestSupportedIPSW() (string, error) {
	return "", checkPlatform()
}

func restoreIPSW(_, _ string, _ int, _, _, _ string) error {
	return checkPlatform()
}

type linuxVMConfig struct {
	CPUs               int
	MemoryMB           int
	KernelPath         string
	InitrdPath         string
	Cmdline            string
	SharedDirPath      string
	SharedDirTag       string
	FileHandleSocketFD int
	AttachNAT          bool
}

type macOSVMConfig struct {
	CPUs               int
	MemoryMB           int
	DiskImagePath      string
	AuxStoragePath     string
	HardwareModelPath  string
	MachineIDPath      string
	SharedDirPath      string
	SharedDirTag       string
	FileHandleSocketFD int
	AttachNAT          bool
}
