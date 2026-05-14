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
}
