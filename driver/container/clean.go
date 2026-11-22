package container

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
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

// removeWorkDirs removes the work directories under tmpdir (the system's
// when empty) of every build that dead reports, printing a Removing line on
// w for each. It goes on past a directory it cannot remove and returns those
// errors joined.
func removeWorkDirs(w io.Writer, tmpdir string, dead func(id string) bool) (int, error) {
	if tmpdir == "" {
		tmpdir = os.TempDir()
	}
	dirs, err := filepath.Glob(filepath.Join(tmpdir, "warden-container-*-*"))
	if err != nil {
		return 0, err
	}
	removed := 0
	var errs []error
	for _, dir := range dirs {
		// warden-container-<id>-<random>: a build ID holds no dash.
		rest := strings.TrimPrefix(filepath.Base(dir), "warden-container-")
		id, _, _ := strings.Cut(rest, "-")
		if !dead(id) {
			continue
		}
		fmt.Fprintf(w, "Removing %s\n", dir)
		if err := os.RemoveAll(dir); err != nil {
			errs = append(errs, err)
			continue
		}
		removed++
	}
	return removed, errors.Join(errs...)
}

// liveProcessBuilds are the build IDs that a running build's waiter shows
// alive, read from one listing of every user's processes, which keep_ids
// cannot see (a runtime daemon is shared between users; a lock directory is
// not). With no ps, or one that lacks -A, -ww or -o (on Windows, BusyBox), it
// finds nothing, and keep_ids is the only sign.
func liveProcessBuilds() map[string]bool {
	out, err := exec.Command("ps", "-A", "-ww", "-o", "pid=,ppid=,args=").Output()
	if err != nil {
		return nil
	}
	return liveBuilds(string(out))
}

// driverProgram is the base name of the container driver's binary.
const driverProgram = "warden-driver-container"

// liveBuilds parses a `ps -A -ww -o pid=,ppid=,args=` listing and returns
// the IDs of the builds whose waiter runs under a live driver: a process
// whose arguments are exactly `<runtime> container wait warden-build-<id>`,
// the runtime one of runtimes by name or path, whose parent in the same
// listing is a warden-driver-container. A waiter whose driver was killed is
// orphaned (its parent is then init, a subreaper or gone) and does not
// count, so a build whose driver is dead is cleaned while its container
// still runs (Decision 25). No other process that names a build counts:
// podman's conmon names the build container for its whole life, and a
// runtime's helpers (finch's limactl, its ssh to nerdctl) run under the
// runtime, not the driver.
//
// Arguments are split at blanks, as ps prints them: a driver or runtime
// whose path holds a space is not recognised, which only ever leaves a build
// to keep_ids.
func liveBuilds(listing string) map[string]bool {
	type proc struct {
		ppid string
		args []string
	}
	procs := make(map[string]proc)
	var order []string
	for _, line := range strings.Split(listing, "\n") {
		f := strings.Fields(line)
		if len(f) < 3 {
			continue
		}
		procs[f[0]] = proc{ppid: f[1], args: f[2:]}
		order = append(order, f[0])
	}
	ids := make(map[string]bool)
	for _, pid := range order {
		p := procs[pid]
		id, ok := waiterBuild(p.args)
		if !ok {
			continue
		}
		parent, ok := procs[p.ppid]
		if ok && p.ppid != pid && path.Base(parent.args[0]) == driverProgram {
			ids[id] = true
		}
	}
	return ids
}

// waiterBuild is the build ID that args wait on, when args are a waiter's:
// `<runtime> container wait warden-build-<id>`.
func waiterBuild(args []string) (string, bool) {
	if len(args) != 4 || args[1] != "container" || args[2] != "wait" {
		return "", false
	}
	if _, ok := supportedRuntime(path.Base(args[0])); !ok {
		return "", false
	}
	id, ok := strings.CutPrefix(args[3], "warden-build-")
	if !ok || id == "" || strings.Contains(id, "-") {
		return "", false
	}
	return id, true
}
