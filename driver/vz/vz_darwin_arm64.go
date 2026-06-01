//go:build darwin && arm64

package vz

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework Virtualization -framework Foundation

#include <stdlib.h>
#include "vz_darwin.h"
*/
import "C"
import (
	"fmt"
	"unsafe"
)

// createLinuxVM creates a Linux VM (used for the relay).
func createLinuxVM(cfg linuxVMConfig) (unsafe.Pointer, error) {
	cKernel := C.CString(cfg.KernelPath)
	defer C.free(unsafe.Pointer(cKernel))

	var cInitrd *C.char
	if cfg.InitrdPath != "" {
		cInitrd = C.CString(cfg.InitrdPath)
		defer C.free(unsafe.Pointer(cInitrd))
	}

	var cCmdline *C.char
	if cfg.Cmdline != "" {
		cCmdline = C.CString(cfg.Cmdline)
		defer C.free(unsafe.Pointer(cCmdline))
	}

	var cSharedPath, cSharedTag *C.char
	if cfg.SharedDirPath != "" {
		cSharedPath = C.CString(cfg.SharedDirPath)
		defer C.free(unsafe.Pointer(cSharedPath))
		cSharedTag = C.CString(cfg.SharedDirTag)
		defer C.free(unsafe.Pointer(cSharedTag))
	}

	attachNAT := C.int(0)
	if cfg.AttachNAT {
		attachNAT = 1
	}

	result := C.vz_create_linux_vm(
		C.int(cfg.CPUs),
		C.uint64_t(cfg.MemoryMB*1024*1024),
		cKernel,
		cInitrd,
		cCmdline,
		cSharedPath,
		cSharedTag,
		C.int(cfg.FileHandleSocketFD),
		attachNAT,
	)

	if result.error != nil {
		err := fmt.Errorf("create linux VM: %s", C.GoString(result.error))
		C.free(unsafe.Pointer(result.error))
		return nil, err
	}
	return result.handle, nil
}

// createMacOSVM creates a macOS VM (used for the build).
func createMacOSVM(cfg macOSVMConfig) (unsafe.Pointer, error) {
	if cfg.FileHandleSocketFD >= 0 && cfg.AttachNAT {
		return nil, fmt.Errorf(
			"macOS VM cannot have both a socketpair and NAT: " +
				"NAT would bypass network isolation")
	}
	cDisk := C.CString(cfg.DiskImagePath)
	defer C.free(unsafe.Pointer(cDisk))
	cAux := C.CString(cfg.AuxStoragePath)
	defer C.free(unsafe.Pointer(cAux))
	cHWModel := C.CString(cfg.HardwareModelPath)
	defer C.free(unsafe.Pointer(cHWModel))
	cMachineID := C.CString(cfg.MachineIDPath)
	defer C.free(unsafe.Pointer(cMachineID))

	var cSharedPath, cSharedTag *C.char
	if cfg.SharedDirPath != "" {
		cSharedPath = C.CString(cfg.SharedDirPath)
		defer C.free(unsafe.Pointer(cSharedPath))
		cSharedTag = C.CString(cfg.SharedDirTag)
		defer C.free(unsafe.Pointer(cSharedTag))
	}

	attachNAT := C.int(0)
	if cfg.AttachNAT {
		attachNAT = 1
	}

	result := C.vz_create_macos_vm(
		C.int(cfg.CPUs),
		C.uint64_t(cfg.MemoryMB*1024*1024),
		cDisk,
		cAux,
		cHWModel,
		cMachineID,
		cSharedPath,
		cSharedTag,
		C.int(cfg.FileHandleSocketFD),
		attachNAT,
	)

	if result.error != nil {
		err := fmt.Errorf("create macOS VM: %s", C.GoString(result.error))
		C.free(unsafe.Pointer(result.error))
		return nil, err
	}
	return result.handle, nil
}

func startVM(handle unsafe.Pointer) error {
	errStr := C.vz_start_vm(handle)
	if errStr != nil {
		err := fmt.Errorf("start VM: %s", C.GoString(errStr))
		C.free(unsafe.Pointer(errStr))
		return err
	}
	return nil
}

func stopVM(handle unsafe.Pointer) error {
	errStr := C.vz_stop_vm(handle)
	if errStr != nil {
		err := fmt.Errorf("stop VM: %s", C.GoString(errStr))
		C.free(unsafe.Pointer(errStr))
		return err
	}
	return nil
}

func vmState(handle unsafe.Pointer) int {
	return int(C.vz_vm_state(handle))
}

// latestSupportedIPSW fetches the URL of the latest macOS IPSW from Apple.
func latestSupportedIPSW() (string, error) {
	result := C.vz_latest_supported_ipsw()
	if result.error != nil {
		err := fmt.Errorf("fetching latest IPSW: %s", C.GoString(result.error))
		C.free(unsafe.Pointer(result.error))
		return "", err
	}
	url := C.GoString((*C.char)(result.handle))
	C.free(result.handle)
	return url, nil
}

// restoreIPSW runs the full IPSW restore process: creates disk, platform
// state files, and installs macOS. Progress is printed to stderr from ObjC.
func restoreIPSW(
	ipswPath, diskPath string, diskSizeGB int,
	auxPath, hwModelPath, machineIDPath string,
) error {
	cIpsw := C.CString(ipswPath)
	defer C.free(unsafe.Pointer(cIpsw))
	cDisk := C.CString(diskPath)
	defer C.free(unsafe.Pointer(cDisk))
	cAux := C.CString(auxPath)
	defer C.free(unsafe.Pointer(cAux))
	cHW := C.CString(hwModelPath)
	defer C.free(unsafe.Pointer(cHW))
	cMID := C.CString(machineIDPath)
	defer C.free(unsafe.Pointer(cMID))

	diskBytes := C.uint64_t(diskSizeGB) * 1024 * 1024 * 1024

	errStr := C.vz_restore_ipsw(
		cIpsw, cDisk, diskBytes, cAux, cHW, cMID,
		nil,
	)

	if errStr != nil {
		err := fmt.Errorf("IPSW restore: %s", C.GoString(errStr))
		C.free(unsafe.Pointer(errStr))
		return err
	}
	return nil
}

// Config types for the cgo bridge

type linuxVMConfig struct {
	CPUs               int
	MemoryMB           int
	KernelPath         string
	InitrdPath         string
	Cmdline            string
	SharedDirPath      string
	SharedDirTag       string
	FileHandleSocketFD int // -1 to skip
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
