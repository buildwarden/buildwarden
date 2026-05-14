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

// createVMConfig calls the ObjC layer to create a VZVirtualMachineConfiguration.
func createVMConfig(cfg VMConfig) (unsafe.Pointer, error) {
	result := C.vz_create_vm_config(
		C.int(cfg.CPUs),
		C.uint64_t(cfg.MemoryMB*1024*1024),
	)
	if result.error != nil {
		err := fmt.Errorf("create VM config: %s", C.GoString(result.error))
		C.free(unsafe.Pointer(result.error))
		return nil, err
	}
	return result.handle, nil
}

// startVM calls VZVirtualMachine.start().
func startVM(handle unsafe.Pointer) error {
	errStr := C.vz_start_vm(handle)
	if errStr != nil {
		err := fmt.Errorf("start VM: %s", C.GoString(errStr))
		C.free(unsafe.Pointer(errStr))
		return err
	}
	return nil
}

// stopVM calls VZVirtualMachine.stop().
func stopVM(handle unsafe.Pointer) error {
	errStr := C.vz_stop_vm(handle)
	if errStr != nil {
		err := fmt.Errorf("stop VM: %s", C.GoString(errStr))
		C.free(unsafe.Pointer(errStr))
		return err
	}
	return nil
}
