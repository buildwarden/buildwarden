package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/lesiw/ctrctl"
	"github.com/spf13/cobra"

	"github.com/buildwarden/buildwarden/driver"
	"github.com/buildwarden/buildwarden/driver/firecracker"
	"github.com/buildwarden/buildwarden/driver/hyperv"
)

var cleanCmd = &cobra.Command{
	Use:   "clean",
	Short: "Remove orphaned warden containers, networks, and images",
	Long: `Remove containers, networks, and per-build images left behind by
interrupted or crashed warden builds. Only removes resources that are not
associated with a currently running warden process.

With --driver firecracker it needs no container runtime: it kills VMMs left
on a network slot by a warden that did not exit cleanly (or by --keep-vms)
and removes the work dirs and per-build rootfs copies of builds that are no
longer running. With --driver hyperv it likewise needs no container runtime:
it removes the build and relay VMs, ephemeral build switch and work dir of
each build that is no longer running (including VMs kept with --keep-vms).

Nothing that a later build would reuse is removed: prepared and downloaded
VM images, the relay boot disk and the support images stay. --cache also
removes warden's whole cache directory, so the next VM build downloads and
prepares its images again.`,
	RunE: runClean,
}

var flagCleanCache bool

func init() {
	cleanCmd.Flags().BoolVar(&flagCleanCache, "cache", false,
		"also remove the whole cache (prepared and downloaded VM images, relay boot disk)")
	rootCmd.AddCommand(cleanCmd)
}

func runClean(cmd *cobra.Command, args []string) error {
	cfg, err := announcedConfig()
	if err != nil {
		return err
	}
	sd, err := selectDriver(cfg)
	if err != nil {
		return err
	}
	if sd.bin != nil {
		return cleanBinary(cfg, sd)
	}
	// The driver's settings are resolved, and refused, before anything is
	// removed, where a driver binary's clean is probed.
	set, err := linkedSettings(cfg, sd, "clean")
	if err != nil {
		return err
	}
	removed := cleanAfterLinkedBuilds(driverName(cfg))
	if cfg.Runtime.Driver == "firecracker" {
		tempDir, err := resolveTempDir(cfg)
		if err != nil {
			return err
		}
		removed += firecracker.Clean(os.Stderr, tempDir)
		finishClean(removed)
		return nil
	}
	if cfg.Runtime.Driver == "hyperv" {
		tempDir, err := resolveTempDir(cfg)
		if err != nil {
			return err
		}
		removed += hyperv.Clean(os.Stderr, tempDir)
		finishClean(removed)
		return nil
	}
	if cfg.Runtime.Driver != "" && cfg.Runtime.Driver != "container" {
		// qemu and vz have no clean of their own beyond dead builds; the
		// container runtime is not theirs to clean (Decision 16).
		finishClean(removed)
		return nil
	}
	n, err := cleanContainers(set)
	if err != nil {
		return err
	}
	// Build containers are removed with their anonymous volumes; other
	// dangling volumes on the runtime are not warden's to prune.
	finishClean(removed + n)
	return nil
}

// cleanContainers removes the containers, networks and per-build relay
// images of builds that are no longer running, on the container runtime
// cfg resolves, and reports how many it removed.
func cleanContainers(set driver.Settings) (int, error) {
	removed := 0
	runtime, err := resolveRuntime(set)
	if err != nil {
		return 0, err
	}
	ctrctl.Cli = []string{runtime}

	liveBuildIDs := findLiveWardenProcesses()

	// Clean containers (Volumes: true removes anonymous DinD volumes)
	containers := listResources("container", "ls", "-a",
		"--filter", "name=warden-", "--format", "{{.Names}}")
	for _, name := range containers {
		if isOrphan(name, liveBuildIDs) {
			fmt.Fprintf(os.Stderr, "Removing container: %s\n", name)
			_, _ = ctrctl.ContainerRm(
				&ctrctl.ContainerRmOpts{Force: true, Volumes: true},
				name)
			removed++
		}
	}

	// Clean networks
	networks := listResources("network", "ls",
		"--format", "{{.Name}}")
	for _, name := range networks {
		if !strings.HasPrefix(name, "warden-") {
			continue
		}
		if isOrphan(name, liveBuildIDs) {
			fmt.Fprintf(os.Stderr, "Removing network: %s\n", name)
			_, _ = ctrctl.NetworkRm(nil, name)
			removed++
		}
	}

	// Clean images
	images := listResources("image", "ls",
		"--format", "{{.Repository}}:{{.Tag}}")
	for _, img := range images {
		if !strings.HasPrefix(img, "warden-relay:") {
			continue
		}
		buildID := strings.TrimPrefix(img, "warden-relay:")
		if !liveBuildIDs[buildID] {
			fmt.Fprintf(os.Stderr, "Removing image: %s\n", img)
			_, _ = ctrctl.ImageRm(nil, img)
			removed++
		}
	}

	return removed, nil
}

// cleanAfterLinkedBuilds removes the stage directory, record and lock of each
// dead build of the driver drv. The drivers are linked into warden and have
// no reap: what a dead build's driver left is found by the driver's own
// cleanup, which follows. A file that cannot be removed is reported and does
// not stop the rest.
func cleanAfterLinkedBuilds(drv string) int {
	n, err := cleanDeadBuilds(drv, nil, os.Stderr)
	if err != nil {
		log.Error(fmt.Sprintf("cleaning up after dead builds: %v", err))
	}
	return n
}

// finishClean handles the cache (removed with --cache, else its size noted)
// and reports the count.
func finishClean(removed int) {
	if flagCleanCache {
		if cleanCache() {
			removed++
		}
	} else if size := dirSize(driver.CacheDir()); size > 0 {
		fmt.Fprintf(os.Stderr, "Kept the cache (%d MB, %s); --cache removes it\n",
			size/(1024*1024), driver.CacheDir())
	}

	if removed == 0 {
		fmt.Println("Nothing to clean.")
	} else {
		fmt.Printf("Removed %d resource(s).\n", removed)
	}
}

// cleanCache removes warden's cache directory (clean --cache).
func cleanCache() bool {
	dir := driver.CacheDir()
	size := dirSize(dir)
	if size == 0 {
		return false
	}
	if err := os.RemoveAll(dir); err != nil {
		fmt.Fprintf(os.Stderr, "Removing the cache %s: %v\n", dir, err)
		return false
	}
	fmt.Fprintf(os.Stderr, "Removed the cache (%d MB, %s)\n", size/(1024*1024), dir)
	return true
}

// dirSize is the total size of the files under dir (0 if it does not exist).
func dirSize(dir string) int64 {
	var size int64
	_ = filepath.Walk(dir, func(_ string, fi os.FileInfo, _ error) error {
		if fi != nil && !fi.IsDir() {
			size += fi.Size()
		}
		return nil
	})
	return size
}

func listResources(args ...string) []string {
	cmdArgs := append(ctrctl.Cli, args...)
	out, err := exec.Command(cmdArgs[0], cmdArgs[1:]...).Output()
	if err != nil {
		return nil
	}
	var results []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line != "" {
			results = append(results, line)
		}
	}
	return results
}

func findLiveWardenProcesses() map[string]bool {
	out, err := exec.Command("ps", "aux").Output()
	if err != nil {
		return nil
	}
	ids := make(map[string]bool)
	for _, line := range strings.Split(string(out), "\n") {
		if !strings.Contains(line, "warden") {
			continue
		}
		// Extract build IDs from running warden-relay-XXXX or warden-build-XXXX
		for _, field := range strings.Fields(line) {
			if strings.HasPrefix(field, "warden-relay-") ||
				strings.HasPrefix(field, "warden-build-") {
				parts := strings.SplitN(field, "-", 3)
				if len(parts) == 3 {
					ids[parts[2]] = true
				}
			}
		}
	}
	return ids
}

func isOrphan(name string, liveIDs map[string]bool) bool {
	if !strings.HasPrefix(name, "warden-") {
		return false
	}
	// Extract build ID: warden-relay-XXXX or warden-build-XXXX or warden-XXXX
	parts := strings.Split(name, "-")
	if len(parts) < 2 {
		return false
	}
	buildID := parts[len(parts)-1]
	return !liveIDs[buildID]
}

// cleanBinary is warden clean for a driver binary (the design's warden
// clean): it probes the driver's clean with this invocation's flags and
// config file, so a value the driver refuses stops it before anything is
// removed; then it reaps every dead build from its record; then it runs the
// driver's clean with the held builds as keep_ids. A failed reap keeps that
// build's files, and clean exits 1 at the end.
func cleanBinary(cfg *Config, sd *selectedDriver) error {
	bin := sd.bin
	tempDir, err := resolveTempDir(cfg)
	if err != nil {
		return err
	}
	cleanReq := func(keep []string, probe bool) map[string]any {
		req := withConfig(map[string]any{"contract": bin.contract,
			"keep_ids": nonNil(keep), "cache": flagCleanCache, "tmpdir": tempDir,
			"args": driverFlags.argsFor("clean")}, absConfigFile(cfg))
		if probe {
			req["probe"] = true
		}
		return req
	}
	if bin.desc.lists("clean") {
		if _, err := bin.call("clean", cleanReq(nil, true),
			callOpts{limit: probeLimit}); err != nil {
			return err
		}
	}
	removed := 0
	reap := func(rec buildRecord, stateFile string) error {
		rb, err := recordDriver(rec, sd)
		if err != nil {
			return err
		}
		res, err := rb.call("reap", reapRequest(rec, stateFile, rb.contract),
			callOpts{argv: []string{rec.ID}})
		removed += resultInt(res, "removed")
		return err
	}
	n, reapErr := cleanDeadBuilds(sd.name(), reap, os.Stderr)
	removed += n
	if bin.desc.lists("clean") {
		keep, err := heldBuildIDs(sd.name())
		if err != nil {
			return err
		}
		res, err := bin.call("clean", cleanReq(keep, false), callOpts{})
		if err != nil {
			return err
		}
		removed += resultInt(res, "removed")
	}
	finishClean(removed)
	if reapErr != nil {
		return fmt.Errorf("cleaning up after dead builds: %w", reapErr)
	}
	return nil
}

// recordDriver is the driver binary that reaps the dead build rec: the file
// its record names, or the one named with --driver for this clean. A file
// that is gone or cannot be run is an error with the fix; one that changed
// since the build is run, with a line that says so.
func recordDriver(rec buildRecord, sd *selectedDriver) (*execDriver, error) {
	if sd.found.Found == foundPath {
		return sd.bin, nil
	}
	if reason := unrunnable(rec.Path, isWindows); reason != "" {
		fix := fmt.Sprintf("reap it with the driver's file named: warden clean "+
			"--driver /path/to/%s", driverFileName(rec.Driver, isWindows))
		return nil, driver.Fixable(fix, "the driver %s that ran build %s %s",
			driver.EscapeOutside(rec.Path), rec.ID, reason)
	}
	if sum, err := fileSHA256(rec.Path); err == nil && rec.SHA256 != "" && sum != rec.SHA256 {
		log.Info(fmt.Sprintf("the driver %s has changed since build %s; reaping with it",
			driver.EscapeOutside(rec.Path), rec.ID))
	}
	return &execDriver{found: &foundDriver{Name: rec.Driver, Path: rec.Path},
		desc: sd.bin.desc, contract: rec.Contract, after: sd.bin.after,
		teardown: sd.bin.teardown}, nil
}
