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
)

var cleanCmd = &cobra.Command{
	Use:   "clean",
	Short: "Remove orphaned warden containers, networks, and images",
	Long: `Remove containers, networks, and per-build images left behind by
interrupted or crashed warden builds. Only removes resources that are not
associated with a currently running warden process.

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
	cfg, err := resolveConfig()
	if err != nil {
		return err
	}
	runtime, err := resolveRuntime(cfg)
	if err != nil {
		return err
	}
	ctrctl.Cli = []string{runtime}

	liveBuildIDs := findLiveWardenProcesses()

	removed := 0

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

	// Build containers are removed with their anonymous volumes above; other
	// dangling volumes on the runtime are not warden's to prune.

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
	return nil
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
