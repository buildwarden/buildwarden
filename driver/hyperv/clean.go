package hyperv

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/buildwarden/buildwarden/driver"
)

// buildLockName is the host lock a build holds for its whole run, so
// `warden clean` can tell a live build's VMs, switch and work dir from what a
// build that did not exit cleanly (or one run with --keep-vms) left behind.
func buildLockName(buildID string) string { return "hyperv-build-" + buildID }

// buildIDLen is the length of driver.RandAlphaNum build IDs.
const buildIDLen = 8

// isBuildID reports whether s is a build ID: 8 of [a-z0-9]. Anything else
// under a warden- name (warden-dev, warden-dev-0, warden-dev-nat, a prepare
// VM) is not a build's, and clean leaves it alone.
func isBuildID(s string) bool {
	if len(s) != buildIDLen {
		return false
	}
	for _, r := range s {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') {
			return false
		}
	}
	return true
}

// cleaner is the part of Provisioner clean uses.
type cleaner interface {
	ListVMs(ctx context.Context, prefix string) ([]string, error)
	RemoveVM(ctx context.Context, name string) error
	TeardownNetwork(ctx context.Context, id string) error
}

// Clean removes what Hyper-V builds that are no longer running left behind:
// their build and relay VMs, their ephemeral build switch and their work dir.
// A build whose lock is held is running and is skipped. Failures are reported
// and skipped. It returns the number of builds cleaned.
func Clean(out io.Writer, tempDir string) int {
	return clean(context.Background(), out, newLocalProvisioner(false),
		tempDir, driver.TryHostLock)
}

func clean(ctx context.Context, out io.Writer, prov cleaner, tempDir string,
	tryLock func(string) (func(), bool, error)) int {
	if tempDir == "" {
		tempDir = os.TempDir()
	}
	vms := map[string][]string{}
	var ids []string
	add := func(id string) {
		if _, ok := vms[id]; !ok {
			vms[id] = nil
			ids = append(ids, id)
		}
	}
	for _, prefix := range []string{"warden-build-", relayVMPrefix} {
		names, err := prov.ListVMs(ctx, prefix)
		if err != nil {
			fmt.Fprintf(out, "Not checking Hyper-V VMs: %v\n", err)
			return 0
		}
		for _, name := range names {
			if id := strings.TrimPrefix(name, prefix); isBuildID(id) {
				add(id)
				vms[id] = append(vms[id], name)
			}
		}
	}
	dirs := map[string][]string{}
	matches, _ := filepath.Glob(filepath.Join(tempDir, "warden-hyperv-*"))
	for _, dir := range matches {
		rest := strings.TrimPrefix(filepath.Base(dir), "warden-hyperv-")
		id, _, _ := strings.Cut(rest, "-")
		if isBuildID(id) {
			add(id)
			dirs[id] = append(dirs[id], dir)
		}
	}

	cleaned := 0
	for _, id := range ids {
		unlock, ok, err := tryLock(buildLockName(id))
		if err != nil || !ok {
			continue // running, or its lock cannot be checked
		}
		if cleanBuild(ctx, out, prov, id, vms[id], dirs[id]) {
			cleaned++
		}
		unlock()
	}
	return cleaned
}

// cleanBuild removes one finished build's VMs, then its ephemeral switch (none
// on the durable strategy: the teardown is then a no-op), then its work dirs,
// which hold the VMs' overlays. A VM that will not go keeps the switch and the
// work dir for the next clean. It reports whether everything went.
func cleanBuild(ctx context.Context, out io.Writer, prov cleaner, id string,
	vms, dirs []string) bool {
	ok := true
	for _, vm := range vms {
		if err := prov.RemoveVM(ctx, vm); err != nil {
			fmt.Fprintf(out, "Removing Hyper-V VM %s: %v\n", vm, err)
			ok = false
			continue
		}
		fmt.Fprintf(out, "Removed Hyper-V VM: %s\n", vm)
	}
	if !ok {
		return false
	}
	if err := prov.TeardownNetwork(ctx, "warden-"+id); err != nil {
		fmt.Fprintf(out, "Removing Hyper-V switch warden-%s: %v\n", id, err)
		return false
	}
	for _, dir := range dirs {
		if err := os.RemoveAll(dir); err != nil {
			fmt.Fprintf(out, "Removing Hyper-V work dir %s: %v\n", dir, err)
			ok = false
			continue
		}
		fmt.Fprintf(out, "Removed Hyper-V work dir: %s\n", dir)
	}
	return ok
}
