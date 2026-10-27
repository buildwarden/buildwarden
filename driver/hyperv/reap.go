package hyperv

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// prepareVMPrefix names every prepare VM; prepareBase appends the stamp.
const prepareVMPrefix = "warden-prepare-"

// reapPrepareLeftovers removes what an interrupted prepare left behind: its
// VM, its unrecorded layer and its seed work dir. It runs under the prepare
// lock, so no other prepare can own anything it finds; a VM kept with
// --keep-vms is reaped too, since its inspection window ends with the next
// prepare. A layer any VM still uses is kept. Failures are logged, never
// fatal.
func reapPrepareLeftovers(ctx context.Context, prov Provisioner, progress io.Writer) {
	vms, err := prov.ListVMs(ctx, prepareVMPrefix)
	if err != nil {
		logf(progress, "warning: not checking for leftover prepare VMs: %v\n", err)
	}
	for _, vm := range vms {
		if err := prov.RemoveVM(ctx, vm); err != nil {
			logf(progress, "warning: removing leftover prepare VM %s: %v\n", vm, err)
			continue
		}
		logf(progress, "removed leftover prepare VM %s\n", vm)
	}
	reapPreparedDir(ctx, prov, progress)
}

// reapPreparedDir deletes work dirs and layer files in <images>/prepared that
// the manifest does not record and no VM uses.
func reapPreparedDir(ctx context.Context, prov Provisioner, progress io.Writer) {
	dir := filepath.Join(evalImagesDir(), "prepared")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	m, err := loadEvalManifest()
	if err != nil {
		return
	}
	keep := map[string]bool{}
	for _, pb := range m.Prepared {
		keep[diskKey(pb.Path)] = true
	}
	var inUse []string
	if inUse, err = prov.DisksInUse(ctx); err != nil {
		logf(progress, "warning: not reaping unrecorded prepared layers: %v\n", err)
		return
	}
	for _, d := range inUse {
		keep[diskKey(d)] = true
	}
	for _, e := range entries {
		p := filepath.Join(dir, e.Name())
		if keep[diskKey(p)] || !leftoverName(e) {
			continue
		}
		if err := os.RemoveAll(p); err != nil {
			logf(progress, "warning: removing leftover %s: %v\n", e.Name(), err)
			continue
		}
		logf(progress, "removed leftover %s\n", e.Name())
	}
}

// leftoverName reports whether a prepared-dir entry is something prepareBase
// creates: a work-* seed dir or a layer disk.
func leftoverName(e os.DirEntry) bool {
	n := strings.ToLower(e.Name())
	if e.IsDir() {
		return strings.HasPrefix(n, "work-")
	}
	return strings.HasSuffix(n, ".vhdx") || strings.HasSuffix(n, ".vhd")
}
