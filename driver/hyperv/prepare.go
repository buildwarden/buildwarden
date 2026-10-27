package hyperv

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/buildwarden/buildwarden/driver"
	"github.com/buildwarden/buildwarden/driver/winguest"
	"github.com/buildwarden/buildwarden/ledger"
)

// Prepare turns the active generalized base (Microsoft's eval image, or any
// installed and sysprepped disk) into a prepared layer: a differencing child
// that has been through OOBE and the winguest prepare script, carries the
// warden-run startup task, and is what every build boots from. Prepare runs
// with no network adapter, so the only inputs are the base and the seed.
// See docs/design/hyperv-windows-prepare.md.

const prepareLogHint = "rerun with --keep-vms and look at C:\\warden\\warden-prepare.log " +
	"in the prepare VM"

// ErrPrepareRunning reports that another prepare holds the prepare lock.
var ErrPrepareRunning = errors.New("a Hyper-V image prepare is already running")

const prepareLockName = "hyperv-prepare"

// PrepareOptions configures a prepare.
type PrepareOptions struct {
	MemoryMB int           // default 4096
	CPUs     int           // default 4
	Timeout  time.Duration // default 45 minutes
	Poll     time.Duration // default 3 s
	KeepVM   bool          // leave the prepare VM for inspection
	Progress io.Writer
}

// PreparedBase is a prepared layer recorded in the image manifest.
type PreparedBase struct {
	Path           string `json:"path"`
	SHA256         string `json:"sha256"`
	Size           int64  `json:"size"`
	Base           string `json:"base"` // manifest name of the generalized base
	BasePath       string `json:"base_path,omitempty"`
	BaseSHA256     string `json:"base_sha256"`
	Generation     int    `json:"generation"`
	PreparedAt     string `json:"prepared_at"`
	GraceEnd       string `json:"grace_end,omitempty"` // eval editions only
	LicenseStatus  string `json:"license_status,omitempty"`
	LicenseChannel string `json:"license_channel,omitempty"`
	PreparedBy     string `json:"prepared_by"`
}

// PrepareBase prepares the active generalized base with the local
// provisioner (VM lifecycle only; prepare needs no host network).
func PrepareBase(ctx context.Context, opts PrepareOptions) (*PreparedBase, error) {
	return prepareBase(ctx, newLocalProvisioner(false), buildPrepareSeed, opts)
}

type prepareSeedFunc func(workDir string) (seedISO, seedDir string, err error)

func prepareBase(
	ctx context.Context, prov Provisioner, seed prepareSeedFunc, opts PrepareOptions,
) (*PreparedBase, error) {
	applyPrepareDefaults(&opts)
	unlock, ok, err := driver.TryHostLock(prepareLockName)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrPrepareRunning
	}
	defer unlock()
	reapPrepareLeftovers(ctx, prov, opts.Progress)

	base, err := activeGeneralizedBase()
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	stamp := now.Format("20060102T150405Z")
	layer, workDir, err := preparePaths(base, stamp)
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(workDir) }()

	seedISO, seedDir, err := seed(workDir)
	if err != nil {
		return nil, fmt.Errorf("prepare seed: %w", err)
	}
	if err := prov.CreateDiffDisk(ctx, layer, base.Path); err != nil {
		return nil, err
	}
	keepLayer := false
	defer func() {
		if !keepLayer && !opts.KeepVM {
			_ = os.Remove(layer)
		}
	}()

	vm := prepareVMPrefix + strings.ToLower(stamp)
	logf(opts.Progress, "preparing %s from %s in VM %s (no network)\n",
		filepath.Base(layer), base.Name, vm)
	if _, err := prov.CreateVM(ctx, VMSpec{
		Name: vm, Generation: base.Generation, MemoryMB: opts.MemoryMB, CPUs: opts.CPUs,
		VHDXPath: layer, SeedISO: seedISO, IsWindows: true,
	}); err != nil {
		return nil, err
	}
	defer func() {
		if opts.KeepVM {
			logf(opts.Progress,
				"--keep-vms: leaving prepare VM %s (disk %s)\n", vm, layer)
			return
		}
		_ = prov.RemoveVM(context.Background(), vm)
	}()
	if err := prov.StartVM(ctx, vm); err != nil {
		return nil, err
	}

	report, err := watchPrepare(ctx, prov, vm, opts)
	if err != nil {
		return nil, err
	}

	pb, err := recordPrepared(layer, base, seedDir, report, now)
	if err != nil {
		return nil, err
	}
	keepLayer = true
	retireOldPrepared(ctx, prov, opts.Progress)
	logf(opts.Progress, "prepared %s (sha256:%s)%s\n",
		filepath.Base(layer), short(pb.SHA256), graceNote(pb.GraceEnd))
	return pb, nil
}

// activeGeneralizedBase is the manifest's active base image.
func activeGeneralizedBase() (evalManifestEntry, error) {
	m, err := loadEvalManifest()
	if err != nil {
		return evalManifestEntry{}, err
	}
	base, ok := m.Images[m.Active]
	if !ok || m.Active == "" {
		return evalManifestEntry{}, driver.Fixable(
			"warden image prepare --driver hyperv --source <eval-image-url-or-path>",
			"no generalized Hyper-V base image to prepare")
	}
	return base, nil
}

// preparePaths names the new prepared layer and creates a work directory for
// its seed, both under <images>/prepared.
func preparePaths(base evalManifestEntry, stamp string) (layer, workDir string, err error) {
	dir := filepath.Join(evalImagesDir(), "prepared")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", "", err
	}
	workDir, err = os.MkdirTemp(dir, "work-")
	if err != nil {
		return "", "", err
	}
	return filepath.Join(dir, base.Name+"-"+stamp+preparedExt(base.Path)), workDir, nil
}

func applyPrepareDefaults(o *PrepareOptions) {
	if o.MemoryMB == 0 {
		o.MemoryMB = 4096
	}
	if o.CPUs == 0 {
		o.CPUs = 4
	}
	if o.Timeout == 0 {
		o.Timeout = 45 * time.Minute
	}
	if o.Poll == 0 {
		o.Poll = 3 * time.Second
	}
}

// preparedExt keeps the parent's format: a differencing disk must match it.
func preparedExt(parent string) string {
	if strings.EqualFold(filepath.Ext(parent), ".vhd") {
		return ".vhd"
	}
	return ".vhdx"
}

// watchPrepare follows the prepare VM until it powers off, printing each new
// guest state, and returns the last guest-reported items. It fails unless the
// guest reported done before powering off.
func watchPrepare(
	ctx context.Context, prov Provisioner, vm string, opts PrepareOptions,
) (map[string]string, error) {
	ctx, cancel := context.WithTimeout(ctx, opts.Timeout)
	defer cancel()
	seen := map[string]string{}
	lastState, running := "", false
	for {
		state, err := prov.VMState(ctx, vm)
		if err != nil {
			return nil, err
		}
		switch state {
		case "":
			return nil, fmt.Errorf("prepare VM %s disappeared", vm)
		case "Running":
			running = true
			kv, err := prov.GuestKVP(ctx, vm)
			if err != nil {
				return nil, err
			}
			for k, v := range kv {
				if strings.HasPrefix(k, "warden-") {
					seen[k] = v
				}
			}
			if s := seen[winguest.KVPState]; s != lastState {
				logf(opts.Progress, "  guest: %s\n", s)
				lastState = s
			}
		case "Off":
			if running {
				return finishedState(vm, seen)
			}
		}
		select {
		case <-ctx.Done():
			_ = prov.StopVM(context.Background(), vm)
			return nil, driver.Fixable(
				prepareLogHint,
				"prepare VM %s did not finish within %s (last guest state %q)",
				vm, opts.Timeout, lastState)
		case <-time.After(opts.Poll):
		}
	}
}

func finishedState(vm string, seen map[string]string) (map[string]string, error) {
	state := seen[winguest.KVPState]
	if state == winguest.StateDone {
		return seen, nil
	}
	if strings.HasPrefix(state, winguest.StateErrorPref) {
		return nil, fmt.Errorf("prepare failed in the guest: %s",
			strings.TrimPrefix(state, winguest.StateErrorPref))
	}
	return nil, driver.Fixable(
		prepareLogHint,
		"prepare VM %s powered off before finishing (last guest state %q)", vm, state)
}

// recordPrepared hashes the layer, records it in the layer cache as a
// prepared child of the base, marks it read-only, and makes it the active
// prepared base in the manifest.
func recordPrepared(
	layer string, base evalManifestEntry, seedDir string,
	report map[string]string, at time.Time,
) (*PreparedBase, error) {
	inputs, err := winguest.SeedInputs(seedDir)
	if err != nil {
		return nil, err
	}
	sum, err := driver.HashFileSHA256(layer)
	if err != nil {
		return nil, err
	}
	src := ledger.LayerSource{
		Kind: ledger.SourcePrepared, PreparedBy: driver.PreparedBy(), Inputs: inputs,
	}
	if err := LayerCache().Record(layer, sum, base.SHA256, src); err != nil {
		return nil, err
	}
	if err := os.Chmod(layer, 0o444); err != nil {
		return nil, err
	}
	fi, err := os.Stat(layer)
	if err != nil {
		return nil, err
	}
	pb := &PreparedBase{
		Path: layer, SHA256: sum, Size: fi.Size(), Base: base.Name,
		BasePath: base.Path, BaseSHA256: base.SHA256, Generation: base.Generation,
		PreparedAt:     at.Format(time.RFC3339),
		LicenseStatus:  report[winguest.KVPLicenseStatus],
		LicenseChannel: report[winguest.KVPLicenseChannel],
		GraceEnd:       winguest.GraceEnd(report),
		PreparedBy:     driver.PreparedBy(),
	}
	m, err := loadEvalManifest()
	if err != nil {
		return nil, err
	}
	m.Prepared = append(m.Prepared, *pb)
	m.ActivePrepared = layer
	return pb, saveEvalManifest(m)
}

func graceNote(end string) string {
	if end == "" {
		return ""
	}
	return "; evaluation grace ends " + end
}

// parseKVPLines parses "name=value" lines (GuestKVP's PowerShell output).
func parseKVPLines(out string) map[string]string {
	kv := map[string]string{}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if k, v, ok := strings.Cut(line, "="); ok && k != "" {
			kv[k] = v
		}
	}
	return kv
}

// activePrepared returns the active prepared base and its generalized parent.
func activePrepared() (PreparedBase, evalManifestEntry, bool) {
	m, err := loadEvalManifest()
	if err != nil || m.ActivePrepared == "" {
		return PreparedBase{}, evalManifestEntry{}, false
	}
	for _, p := range m.Prepared {
		if p.Path != m.ActivePrepared {
			continue
		}
		base, ok := m.Images[p.Base]
		if !ok {
			return PreparedBase{}, evalManifestEntry{}, false
		}
		return p, base, true
	}
	return PreparedBase{}, evalManifestEntry{}, false
}

// retireOldPrepared deletes prepared layers that two refreshes have
// superseded: everything but the active layer and the one it replaced, which
// a build that resolved its chain just before the switch may still be
// starting on. It then deletes replaced base files no remaining layer is
// built on. Anything a VM still depends on is kept for a later prepare.
// Failures are logged, never fatal.
func retireOldPrepared(ctx context.Context, prov Provisioner, progress io.Writer) {
	m, err := loadEvalManifest()
	if err != nil || (len(m.Prepared) <= 2 && len(m.OldBases) == 0) {
		return
	}
	inUse, err := prov.DisksInUse(ctx)
	if err != nil {
		logf(progress, "warning: not retiring old images: %v\n", err)
		return
	}
	used := map[string]bool{}
	for _, d := range inUse {
		used[diskKey(d)] = true
	}
	m.Prepared = retireLayers(m, used, progress)
	m.OldBases = retireBases(m, used, progress)
	if err := saveEvalManifest(m); err != nil {
		logf(progress, "warning: %v\n", err)
	}
}

func diskKey(p string) string { return strings.ToLower(filepath.Clean(p)) }

// retireLayers returns m.Prepared without the layers it deleted.
func retireLayers(m *evalManifest, used map[string]bool, progress io.Writer) []PreparedBase {
	if len(m.Prepared) <= 2 {
		return m.Prepared
	}
	keep := m.Prepared[len(m.Prepared)-2:]
	var kept []PreparedBase
	for _, pb := range m.Prepared[:len(m.Prepared)-2] {
		if pb.Path == m.ActivePrepared || used[diskKey(pb.Path)] {
			kept = append(kept, pb)
			continue
		}
		_ = os.Chmod(pb.Path, 0o644)
		if err := os.Remove(pb.Path); err != nil && !os.IsNotExist(err) {
			logf(progress, "warning: retiring %s: %v\n", pb.Path, err)
			kept = append(kept, pb)
			continue
		}
		_ = LayerCache().Forget(pb.Path)
		logf(progress, "retired prepared layer %s\n", filepath.Base(pb.Path))
	}
	return append(kept, keep...)
}

// retireBases returns m.OldBases without the base files it deleted: those no
// remaining prepared layer is built on and no VM uses. A layer recorded
// before BasePath existed may be on any base, so then nothing is deleted. A
// file outside the image cache was registered by the user and is only
// dropped from the list.
func retireBases(m *evalManifest, used map[string]bool, progress io.Writer) []string {
	needed := map[string]bool{}
	for _, pb := range m.Prepared {
		if pb.BasePath == "" {
			return m.OldBases
		}
		needed[diskKey(pb.BasePath)] = true
	}
	var kept []string
	for _, b := range m.OldBases {
		if !ownedBase(b) {
			continue
		}
		if needed[diskKey(b)] || used[diskKey(b)] {
			kept = append(kept, b)
			continue
		}
		if err := os.Remove(b); err != nil && !os.IsNotExist(err) {
			logf(progress, "warning: retiring base %s: %v\n", b, err)
			kept = append(kept, b)
			continue
		}
		_ = os.Remove(b + ".sha256")
		_ = LayerCache().Forget(b)
		logf(progress, "retired base image %s\n", filepath.Base(b))
	}
	return kept
}
