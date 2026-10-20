//go:build !windows

package hyperv

// startBackgroundRefresh is a no-op off Windows: Hyper-V only runs there.
func startBackgroundRefresh() (logPath string, started bool) { return "", false }
