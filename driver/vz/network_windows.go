//go:build windows

package vz

// NewVirtualNetwork is unavailable on Windows. The vz driver wraps Apple's
// Virtualization.framework and only runs on macOS; its isolated link relies on
// a Unix datagram socket pair. This stub exists so the package (and the warden
// CLI that imports it) compiles on Windows; the driver itself reports an
// unsupported-platform error before ever reaching here. It defers to
// checkPlatform for that error, matching the other non-darwin stubs.
func NewVirtualNetwork() (*VirtualNetwork, error) {
	return nil, checkPlatform()
}

// Close is a no-op on Windows (no socket pair was created).
func (vn *VirtualNetwork) Close() error { return nil }
