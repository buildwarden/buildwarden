package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/lesiw/ctrctl"
	"github.com/spf13/cobra"
)

var cleanCmd = &cobra.Command{
	Use:   "clean",
	Short: "Remove orphaned warden containers, networks, and images",
	Long: `Remove containers, networks, and images left behind by interrupted
or crashed warden builds. Only removes resources that are not associated
with a currently running warden process.`,
	RunE: runClean,
}

func init() {
	rootCmd.AddCommand(cleanCmd)
}

func runClean(cmd *cobra.Command, args []string) error {
	cfg, err := resolveConfig()
	if err != nil {
		return err
	}
	if err := setupRuntime(cfg); err != nil {
		return err
	}

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

	// Prune any dangling anonymous volumes left by build containers.
	pruneArgs := append(ctrctl.Cli, "volume", "prune", "-f")
	pruneCmd := exec.Command(pruneArgs[0], pruneArgs[1:]...)
	if out, err := pruneCmd.Output(); err == nil && len(out) > 0 {
		fmt.Fprintf(os.Stderr, "Pruned dangling volumes\n")
		removed++
	}

	if removed == 0 {
		fmt.Println("Nothing to clean.")
	} else {
		fmt.Printf("Removed %d orphaned resource(s).\n", removed)
	}
	return nil
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
