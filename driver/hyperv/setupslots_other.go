//go:build !windows

package hyperv

// runSetupSlots is unavailable off Windows; RunSetup refuses a non-Windows
// host before it gets here.
func runSetupSlots(string, int) (setupSlots, error) {
	return setupSlots{}, errUnsupported()
}
