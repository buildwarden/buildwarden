//go:build !darwin || !arm64

package vz

import "unsafe"

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

func restoreIPSW(_, _, _ string, _ int, _, _ string) error {
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
