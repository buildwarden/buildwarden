package container

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/lesiw/ctrctl"
)

// listNames runs the runtime's listing args and returns its lines; an
// error is a runtime that did not answer.
func listNames(args ...string) ([]string, error) {
	cmdArgs := append(append([]string{}, ctrctl.Cli...), args...)
	out, err := exec.Command(cmdArgs[0], cmdArgs[1:]...).Output()
	if err != nil {
		return nil, err
	}
	var names []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line != "" {
			names = append(names, line)
		}
	}
	return names, nil
}

// The runtime's listings of what a build creates: its containers
// (warden-build-, warden-relay- and warden-holder-<id>), its network
// (warden-<id>), and a dev build's relay image (warden-relay:<id>).
var (
	listContainers = []string{"container", "ls", "-a", "--filter", "name=warden-",
		"--format", "{{.Names}}"}
	listNetworks = []string{"network", "ls", "--format", "{{.Name}}"}
	listImages   = []string{"image", "ls", "--format", "{{.Repository}}:{{.Tag}}"}
)

// buildIDOf is the build ID a container or network name of the driver's
// ends with, and whether name is one: warden-<id> or warden-<role>-<id>.
func buildIDOf(name string) (string, bool) {
	if !strings.HasPrefix(name, "warden-") {
		return "", false
	}
	parts := strings.Split(name, "-")
	return parts[len(parts)-1], true
}

// removeResources removes, on the runtime in ctrctl.Cli, the containers,
// networks and dev relay images of every build that dead reports, printing a
// Removing line on w for each, and returns how many it removed. A listing
// that fails is a runtime that did not answer, and is returned: the places
// it did not look in are not reported clean.
func removeResources(w io.Writer, dead func(id string) bool) (int, error) {
	removed := 0
	containers, err := listNames(listContainers...)
	if err != nil {
		return 0, err
	}
	for _, name := range removalOrder(containers) {
		if id, ok := buildIDOf(name); ok && dead(id) {
			fmt.Fprintf(w, "Removing container: %s\n", name)
			// Volumes: true removes the anonymous volumes of a build.
			_, _ = ctrctl.ContainerRm(&ctrctl.ContainerRmOpts{Force: true,
				Volumes: true}, name)
			removed++
		}
	}
	networks, err := listNames(listNetworks...)
	if err != nil {
		return removed, err
	}
	for _, name := range networks {
		if id, ok := buildIDOf(name); ok && dead(id) {
			fmt.Fprintf(w, "Removing network: %s\n", name)
			_, _ = ctrctl.NetworkRm(nil, name)
			removed++
		}
	}
	images, err := listNames(listImages...)
	if err != nil {
		return removed, err
	}
	for _, img := range images {
		if id, ok := strings.CutPrefix(img, "warden-relay:"); ok && dead(id) {
			fmt.Fprintf(w, "Removing image: %s\n", img)
			_, _ = ctrctl.ImageRm(nil, img)
			removed++
		}
	}
	return removed, nil
}

// removalOrder is names in the order a build's containers can be removed:
// the build container joins the holder's network namespace, and podman
// refuses to remove the holder while it does (a dependent container), which
// then keeps the network too. So every build container goes first and every
// holder last; the rest keep the listing's order.
func removalOrder(names []string) []string {
	rank := func(name string) int {
		switch {
		case strings.HasPrefix(name, "warden-build-"):
			return 0
		case strings.HasPrefix(name, "warden-holder-"):
			return 2
		}
		return 1
	}
	out := slices.Clone(names)
	slices.SortStableFunc(out, func(a, b string) int { return rank(a) - rank(b) })
	return out
}

// removeWorkDirs removes the work directories of build id under tmpdir (the
// system's when empty), printing a Removing line on w for each.
func removeWorkDirs(w io.Writer, tmpdir, id string) (int, error) {
	if tmpdir == "" {
		tmpdir = os.TempDir()
	}
	dirs, err := filepath.Glob(filepath.Join(tmpdir, "warden-container-"+id+"-*"))
	if err != nil {
		return 0, err
	}
	for _, dir := range dirs {
		fmt.Fprintf(w, "Removing %s\n", dir)
		if err := os.RemoveAll(dir); err != nil {
			return 0, err
		}
	}
	return len(dirs), nil
}

// liveProcessBuilds are the build IDs of the driver's containers that a
// running process names: the driver's own `container wait warden-build-<id>`
// child, and the runtime's processes. It sees every user's processes, which
// keep_ids cannot (a runtime daemon is shared between users; a lock
// directory is not). With no ps, on Windows, it finds nothing.
func liveProcessBuilds() map[string]bool {
	out, err := exec.Command("ps", "aux").Output()
	if err != nil {
		return nil
	}
	ids := make(map[string]bool)
	for _, line := range strings.Split(string(out), "\n") {
		if !strings.Contains(line, "warden") {
			continue
		}
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
