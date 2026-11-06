//go:build !linux

package main

import "errors"

// openVMDHCPConn is Linux-only, like the rest of vm mode.
func openVMDHCPConn(string) (packetConn, error) {
	return nil, errors.New("vm mode needs Linux")
}
