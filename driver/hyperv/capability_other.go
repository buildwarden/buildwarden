//go:build !windows

package hyperv

// detectPlatform reports that the Hyper-V driver is unavailable off Windows.
// The report still renders (Doctor) so a cross-platform run explains why rather
// than silently doing nothing.
func detectPlatform(c *Capabilities) {
	c.Supported = false
	c.TokenScope = "n/a (" + c.Platform + ")"
	c.Errors = append(c.Errors, "the hyperv driver requires a Windows host")
}
