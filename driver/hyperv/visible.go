package hyperv

import (
	"path/filepath"
	"strings"

	"github.com/buildwarden/buildwarden/driver"
)

// Hyper-V (its VM management service) opens the disks and seed images warden
// prepares, so they must exist where the service looks. A process started from
// an MSIX-packaged app (for example a desktop app's terminal) has its new files
// under %LOCALAPPDATA% silently redirected into the package's private folder,
// %LOCALAPPDATA%\Packages\<package>\LocalCache\Local\...: warden sees them, the
// Hyper-V service does not, and attaching one fails with "path not found". The
// OS temp directory is not redirected.

// packagesRedirect is the path segment a redirected file's real location has.
const packagesRedirect = `\appdata\local\packages\`

// redirected reports whether final, the real location of something warden
// created or will hand to Hyper-V at want, is an app-package redirection of it.
func redirected(want, final string) bool {
	f := strings.ToLower(filepath.Clean(strings.TrimPrefix(final, `\\?\`)))
	w := strings.ToLower(filepath.Clean(want))
	return f != w && strings.Contains(f, packagesRedirect) &&
		!strings.Contains(w, packagesRedirect)
}

// cacheVisibleFix is the remedy for a redirected Hyper-V cache.
const cacheVisibleFix = "set WARDEN_HYPERV_CACHE_DIR to a folder outside AppData " +
	`(for example C:\warden-cache\hyperv) and prepare again, or run warden from a ` +
	"terminal that is not started by a packaged app"

// checkDirVisible fails when new files created in dir would be redirected away
// from Hyper-V. what names dir for the message; fix is the remedy. A probe that
// cannot tell (another platform, an unreadable path) does not block.
func checkDirVisible(dir, what, fix string) error {
	final, ok := probeNewFile(dir)
	if !ok || !redirected(filepath.Join(dir, filepath.Base(final)), final) {
		return nil
	}
	actual := filepath.Dir(strings.TrimPrefix(final, `\\?\`))
	return driver.Fixable(fix,
		"%s %s is redirected by Windows app virtualization to %s, where Hyper-V "+
			"cannot see it", what, dir, actual)
}

// checkFilesVisible fails when an existing file Hyper-V must open lives only in
// an app package's redirected copy.
func checkFilesVisible(fix string, paths ...string) error {
	for _, p := range paths {
		final, ok := finalPath(p)
		if ok && redirected(p, final) {
			actual := strings.TrimPrefix(final, `\\?\`)
			return driver.Fixable(fix,
				"%s exists only in a Windows app-package copy (%s), "+
					"where Hyper-V cannot see it", p, actual)
		}
	}
	return nil
}
