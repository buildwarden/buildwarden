//go:build !darwin || !arm64

package vz

import "unsafe"

func createVMConfig(_ VMConfig) (unsafe.Pointer, error) {
	return nil, errUnsupported
}

func startVM(_ unsafe.Pointer) error {
	return errUnsupported
}

func stopVM(_ unsafe.Pointer) error {
	return errUnsupported
}

var errUnsupported = checkPlatform()
