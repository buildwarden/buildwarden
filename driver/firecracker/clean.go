package firecracker

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/buildwarden/buildwarden/driver"
)

// Clean is `warden clean`'s firecracker part: it kills VMMs recorded on a
// slot no build holds (including VMs --keep-vms left), and removes the
// per-build rootfs copies and work dirs of builds that are no longer running,
// and the partial raw images of conversions that were killed.
// tempDir is where work dirs are made (empty: the OS temp dir). It needs no
// container runtime and returns how many resources it removed.
func Clean(out io.Writer, tempDir string) int {
	return reapVMMs(out) + reapWorkDirs(out, tempDir) + reapConversions(out)
}

// reapConversions removes the *.raw.tmp a killed conversion left in ImageDir,
// skipping those whose conversion is still running (it holds the lock).
func reapConversions(out io.Writer) int {
	removed := 0
	tmps, _ := filepath.Glob(filepath.Join(ImageDir(), "*.raw.tmp"))
	for _, tmp := range tmps {
		lock, ok := convertLockOf(filepath.Base(tmp))
		if !ok {
			continue
		}
		unlock, ok, err := driver.TryHostLock(lock)
		if err != nil || !ok {
			continue
		}
		if err := os.Remove(tmp); err == nil {
			fmt.Fprintf(out, "Removed partial firecracker image: %s\n", tmp)
			removed++
		}
		unlock()
	}
	return removed
}

// convertLockName is held while a source whose sha256 starts with prefix (12
// hex digits, as in its raw image's name) is converted.
func convertLockName(prefix string) string { return "firecracker-convert-" + prefix }

// convertLockOf is the conversion lock of a <stem>-<12 hex>.raw.tmp name.
func convertLockOf(name string) (string, bool) {
	stem, ok := strings.CutSuffix(name, ".raw.tmp")
	if !ok || len(stem) < 13 || stem[len(stem)-13] != '-' {
		return "", false
	}
	prefix := stem[len(stem)-12:]
	if strings.Trim(prefix, "0123456789abcdef") != "" {
		return "", false
	}
	return convertLockName(prefix), true
}

// reapVMMs kills the VMMs recorded on slots no build holds.
func reapVMMs(out io.Writer) int {
	removed := 0
	pidFiles, _ := filepath.Glob(filepath.Join(driver.LockDir(), "firecracker-slot-*.pids"))
	for _, p := range pidFiles {
		n, err := strconv.Atoi(strings.TrimSuffix(
			strings.TrimPrefix(filepath.Base(p), "firecracker-slot-"), ".pids"))
		if err != nil {
			continue
		}
		s := Slot(n)
		unlock, ok, err := driver.TryHostLock(s.LockName())
		if err != nil || !ok {
			continue
		}
		b, _ := os.ReadFile(p)
		live := liveVMMs(parsePidFile(string(b)), os.ReadFile, os.Readlink)
		if len(live) > 0 {
			if err := killVMMs(live); err != nil {
				fmt.Fprintf(out, "Killing VMMs on slot %d: %v\n", s, err)
				unlock()
				continue
			}
			fmt.Fprintf(out, "Killed %d VMM(s) on firecracker slot %d\n", len(live), s)
			removed += len(live)
		}
		_ = os.Remove(p)
		unlock()
	}
	return removed
}

// reapWorkDirs removes the rootfs copies and work dirs of builds no longer
// running.
func reapWorkDirs(out io.Writer, tempDir string) int {
	removed := 0
	if tempDir == "" {
		tempDir = os.TempDir()
	}
	var dirs []string
	if ents, err := os.ReadDir(WorkRoot()); err == nil {
		for _, e := range ents {
			if e.IsDir() {
				dirs = append(dirs, filepath.Join(WorkRoot(), e.Name()))
			}
		}
	}
	temps, _ := filepath.Glob(filepath.Join(tempDir, "warden-firecracker-*"))
	dirs = append(dirs, temps...)
	for _, dir := range dirs {
		id := buildIDOf(filepath.Base(dir))
		if id == "" {
			continue
		}
		unlock, ok, err := driver.TryHostLock(buildLockName(id))
		if err != nil || !ok {
			continue
		}
		if err := os.RemoveAll(dir); err == nil {
			fmt.Fprintf(out, "Removed firecracker work dir: %s\n", dir)
			removed++
		}
		unlock()
	}
	return removed
}

// buildIDOf is the build id in a work dir's name: the per-build rootfs
// directory is the id itself, the temp work dir warden-firecracker-<id>-XXXX.
func buildIDOf(name string) string {
	if rest, ok := strings.CutPrefix(name, "warden-firecracker-"); ok {
		id, _, _ := strings.Cut(rest, "-")
		return id
	}
	if len(name) == 8 && !strings.ContainsAny(name, "-. ") {
		return name
	}
	return ""
}

// buildLockName is held for a build's life, so Clean can tell a running
// build's work dirs from stale ones.
func buildLockName(buildID string) string { return "firecracker-build-" + buildID }
