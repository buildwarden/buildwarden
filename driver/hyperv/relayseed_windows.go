//go:build windows

package hyperv

// makeRelaySeedISO packs the staged relay seed directory into the ISO the
// relay VM boots with, by the same IMAPI2 path as the build VM's seed.
func makeRelaySeedISO(seedDir, iso string) error {
	return makeSeedISOWindows(seedDir, iso, RelaySeedLabel)
}
