//go:build !linux

package main

import "errors"

// openBuildIface is Linux-only: vm mode runs as init in a Linux relay VM and
// addresses its build interface over rtnetlink.
func openBuildIface(string) (buildIface, error) {
	return nil, errors.New("vm mode needs Linux")
}
