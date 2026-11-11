//go:build !windows

package hyperv

import "fmt"

// makeRelaySeedISO is a stub off Windows: the relay seed ISO is built with
// IMAPI2, which only a Windows host has.
func makeRelaySeedISO(string, string) error {
	return fmt.Errorf("the hyperv relay seed ISO is only built on Windows")
}
