//go:build !windows

package hyperv

import (
	"fmt"

	"github.com/buildwarden/buildwarden/driver"
)

// buildSeed is a no-op stub off Windows: Hyper-V provisioning only runs on a
// Windows host. The Windows implementation (winseed_windows.go) stages the seed
// and packages it into an ISO via IMAPI2.
func buildSeed(workDir, buildID string, isWindows bool, req *driver.BuildRequest) (seedISO, seedVHDX string, err error) {
	return "", "", fmt.Errorf("hyperv build seed generation is only supported on Windows")
}
