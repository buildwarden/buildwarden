package qemu

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/buildwarden/buildwarden/driver"
	"github.com/buildwarden/buildwarden/driver/winguest"
	"github.com/buildwarden/buildwarden/ledger"
)

// Windows images for the qemu driver come in two stages (see
// docs/design/hyperv-windows-prepare.md):
//
//   - The generalized base, windows-<arch>.qcow2 (+ its UEFI vars): the ISO
//     install, ending in sysprep /generalize. Redone only when its inputs
//     change. A marker file distinguishes it from an image installed before
//     the split, which carries the warden-run task itself and boots as is.
//   - The prepared layer: a qcow2 overlay of the base booted once with no
//     network through OOBE and winguest's prepare script. Builds boot it; it
//     is redone every refresh, before the evaluation grace runs out.

// ErrPrepareRunning reports that another prepare for the arch holds the lock.
var ErrPrepareRunning = errors.New("a qemu Windows image prepare is already running")

// PreparedLayer is one prepared layer in the per-arch manifest.
type PreparedLayer struct {
	Path           string `json:"path"`
	VarsFD         string `json:"vars"`
	SHA256         string `json:"sha256"`
	BaseSHA256     string `json:"base_sha256"`
	PreparedAt     string `json:"prepared_at"`
	GraceEnd       string `json:"grace_end,omitempty"`
	LicenseStatus  string `json:"license_status,omitempty"`
	LicenseChannel string `json:"license_channel,omitempty"`
	PreparedBy     string `json:"prepared_by"`
}

type preparedManifest struct {
	Active   string          `json:"active"`
	Prepared []PreparedLayer `json:"prepared"`
}

func windowsVarsPath(goarch string) string {
	return filepath.Join(driver.CacheDir(), "images", "windows-"+goarch+"-vars.fd")
}

// generalizedMarker exists once the base at WindowsImagePath was produced by
// the generalizing install.
func generalizedMarker(goarch string) string {
	return WindowsImagePath(goarch) + ".generalized"
}

func preparedManifestPath(goarch string) string {
	return filepath.Join(driver.CacheDir(), "images", "windows-"+goarch+"-prepared.json")
}

func prepareLockName(goarch string) string { return "qemu-windows-prepare-" + goarch }

func prepareFix(goarch string) string {
	return "warden image prepare --driver qemu --os windows --arch " + goarch
}

func loadPreparedManifest(goarch string) (*preparedManifest, error) {
	m := &preparedManifest{}
	b, err := os.ReadFile(preparedManifestPath(goarch))
	if errors.Is(err, os.ErrNotExist) {
		return m, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, m); err != nil {
		return nil, fmt.Errorf("prepared image manifest %s is corrupt: %w",
			preparedManifestPath(goarch), err)
	}
	return m, nil
}

func savePreparedManifest(goarch string, m *preparedManifest) error {
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	p := preparedManifestPath(goarch)
	if err := os.WriteFile(p+".tmp", append(b, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(p+".tmp", p)
}

// activeWindowsLayer returns the active prepared layer for the arch.
func activeWindowsLayer(goarch string) (PreparedLayer, bool) {
	m, err := loadPreparedManifest(goarch)
	if err != nil || m.Active == "" {
		return PreparedLayer{}, false
	}
	for _, l := range m.Prepared {
		if l.Path == m.Active {
			return l, true
		}
	}
	return PreparedLayer{}, false
}

// PrepareWindowsLayer prepares the generalized base for goarch ("" = host):
// it boots a new qcow2 overlay of it once with no network and the WPREP seed,
// reads the guest's result from that seed, and makes the layer active.
func PrepareWindowsLayer(goarch string, keepVM bool, progress io.Writer) (string, error) {
	if goarch == "" {
		goarch = runtime.GOARCH
	}
	unlock, free, err := driver.TryHostLock(prepareLockName(goarch))
	if err != nil {
		return "", err
	}
	if !free {
		return "", ErrPrepareRunning
	}
	defer unlock()

	base, baseVars := WindowsImagePath(goarch), windowsVarsPath(goarch)
	if _, err := os.Stat(generalizedMarker(goarch)); err != nil {
		return "", driver.Fixable(
			"warden image prepare --driver qemu --os windows --arch "+goarch+
				" --force --source <install ISO>",
			"the Windows base for %s is missing or predates the generalizing "+
				"install, so it cannot be prepared", goarch)
	}
	baseInfo, err := LayerCache().Lookup(base, "")
	if err != nil {
		return "", err
	}

	now := time.Now().UTC()
	layer, vars, work, err := windowsLayerPaths(goarch, now)
	if err != nil {
		return "", err
	}
	defer func() { _ = os.RemoveAll(work) }()

	seedDir, seedImg, err := buildPrepareSeed(work, goarch)
	if err != nil {
		return "", err
	}
	if err := createPrepareDisks(base, baseVars, layer, vars); err != nil {
		return "", err
	}
	ok := false
	defer func() {
		if !ok && !keepVM {
			_ = os.Remove(layer)
			_ = os.Remove(vars)
		}
	}()

	fmt.Fprintf(progress, "[warden] preparing %s (no network)...\n", filepath.Base(layer))
	if err := runPrepareVM(goarch, layer, vars, seedImg); err != nil {
		return "", err
	}
	report, err := readPrepareResult(seedImg)
	if err != nil {
		return "", err
	}
	pl, err := recordWindowsLayer(goarch, layer, vars, baseInfo.SHA256, seedDir, report, now)
	if err != nil {
		return "", err
	}
	ok = true
	retireWindowsLayers(goarch, progress)
	note := ""
	if pl.GraceEnd != "" {
		note = "; evaluation grace ends " + pl.GraceEnd
	}
	fmt.Fprintf(progress, "[warden] prepared %s%s\n", filepath.Base(layer), note)
	return layer, nil
}

// windowsLayerPaths names a new prepared layer and its UEFI vars, and makes
// a work directory for its seed, all under images/prepared.
func windowsLayerPaths(goarch string, now time.Time) (layer, vars, work string, err error) {
	dir := filepath.Join(driver.CacheDir(), "images", "prepared")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", "", "", err
	}
	stamp := now.Format("20060102T150405Z")
	layer = filepath.Join(dir, "windows-"+goarch+"-"+stamp+".qcow2")
	vars = strings.TrimSuffix(layer, ".qcow2") + "-vars.fd"
	work, err = os.MkdirTemp(dir, "work-")
	return layer, vars, work, err
}

// buildPrepareSeed stages the prepare seed and packages it as a writable FAT
// disk labelled WPREP (the guest writes its status back to it).
func buildPrepareSeed(work, goarch string) (seedDir, seedImg string, err error) {
	seedDir = filepath.Join(work, "prepare-seed")
	if err := winguest.StagePrepareSeed(seedDir, goarch); err != nil {
		return "", "", err
	}
	seedImg = filepath.Join(work, "prepare-seed.img")
	if err := generateWindowsSeedFAT(seedDir, seedImg, winguest.PrepareSeedLabel); err != nil {
		return "", "", fmt.Errorf("prepare seed: %w", err)
	}
	return seedDir, seedImg, nil
}

// createPrepareDisks makes the layer a qcow2 overlay of the base and gives it
// its own copy of the base's UEFI vars (OOBE may change boot entries).
func createPrepareDisks(base, baseVars, layer, vars string) error {
	out, err := exec.Command("qemu-img", "create", "-f", "qcow2",
		"-b", base, "-F", "qcow2", layer).CombinedOutput()
	if err != nil {
		return fmt.Errorf("creating prepared layer: %s: %w", out, err)
	}
	if err := driver.CopyFile(baseVars, vars); err != nil {
		_ = os.Remove(layer)
		return fmt.Errorf("copying UEFI vars: %w", err)
	}
	return nil
}

// runPrepareVM boots the layer until the guest powers itself off (qemu exits)
// or the timeout passes. Guest reboots (the Defender removal) stay inside
// the one qemu process.
func runPrepareVM(goarch, layer, vars, seedImg string) error {
	qarch := qemuArchOf(goarch)
	d := &Driver{}
	binary := d.qemuBinary(qarch)
	args := windowsPrepareArgs(qarch, d.detectAccel(), layer, vars, seedImg)
	if vnc := os.Getenv("WARDEN_QEMU_VNC"); vnc != "" {
		args = append(args, "-vnc", vnc)
	}
	ctx, cancel := context.WithTimeout(context.Background(), installTimeout())
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Stdout, cmd.Stderr = io.Discard, os.Stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return driver.Fixable("rerun with WARDEN_QEMU_VNC=127.0.0.1:0 and "+
				"--keep-vms to watch the guest",
				"Windows prepare did not finish within %s", installTimeout())
		}
		return fmt.Errorf("windows prepare (qemu): %w", err)
	}
	return nil
}

// windowsPrepareArgs is the build VM's Windows device model with no network
// and the WPREP seed as a writable second NVMe disk.
func windowsPrepareArgs(qarch, accel, layer, vars, seedImg string) []string {
	return []string{
		"-machine", fmt.Sprintf("virt,accel=%s", accel),
		"-cpu", cpuForAccel(accel, qarch),
		"-m", "4096",
		"-smp", "4",
		"-nodefaults",
		"-display", "none",
		"-device", "ramfb",
		"-drive", fmt.Sprintf("if=pflash,format=raw,readonly=on,file=%s",
			efiCodePath(qarch)),
		"-drive", fmt.Sprintf("if=pflash,format=raw,file=%s", vars),
		"-device", "qemu-xhci,id=xhci",
		"-device", "usb-kbd,bus=xhci.0",
		"-device", "usb-tablet,bus=xhci.0",
		"-drive", fmt.Sprintf("if=none,id=osdisk,file=%s,format=qcow2", layer),
		"-device", "nvme,drive=osdisk,serial=wardenwin,bootindex=0",
		"-drive", fmt.Sprintf("if=none,id=seed,format=raw,file=%s", seedImg),
		"-device", "nvme,drive=seed,serial=wardenprep",
		"-nic", "none",
		"-device", "virtio-rng-pci",
	}
}

// readPrepareResult reads the guest's last status block from the seed disk
// and fails unless the prepare finished.
func readPrepareResult(seedImg string) (map[string]string, error) {
	raw, err := os.ReadFile(seedImg)
	if err != nil {
		return nil, err
	}
	report, found := winguest.ParseSeedStatus(raw)
	state := report[winguest.KVPState]
	switch {
	case found && state == winguest.StateDone:
		return report, nil
	case strings.HasPrefix(state, winguest.StateErrorPref):
		return nil, fmt.Errorf("prepare failed in the guest: %s",
			strings.TrimPrefix(state, winguest.StateErrorPref))
	case !found:
		return nil, driver.Fixable(
			"rerun with WARDEN_QEMU_VNC=127.0.0.1:0 and --keep-vms to watch the guest",
			"the prepare VM powered off without reporting a status (did OOBE "+
				"find the WPREP answer file?)")
	}
	return nil, driver.Fixable(
		"rerun with --keep-vms and look at C:\\warden\\warden-prepare.log in the layer",
		"the prepare VM powered off before finishing (last guest state %q)", state)
}

// recordWindowsLayer hashes the layer, records it in the layer cache as a
// prepared child of the base, makes it read-only and active.
func recordWindowsLayer(
	goarch, layer, vars, baseSHA, seedDir string, report map[string]string, at time.Time,
) (PreparedLayer, error) {
	inputs, err := winguest.SeedInputs(seedDir)
	if err != nil {
		return PreparedLayer{}, err
	}
	sum, err := driver.HashFileSHA256(layer)
	if err != nil {
		return PreparedLayer{}, err
	}
	src := ledger.LayerSource{
		Kind: ledger.SourcePrepared, PreparedBy: driver.PreparedBy(), Inputs: inputs,
	}
	if err := LayerCache().Record(layer, sum, baseSHA, src); err != nil {
		return PreparedLayer{}, err
	}
	_ = os.Chmod(layer, 0o444)
	pl := PreparedLayer{
		Path: layer, VarsFD: vars, SHA256: sum, BaseSHA256: baseSHA,
		PreparedAt:     at.Format(time.RFC3339),
		GraceEnd:       winguest.GraceEnd(report),
		LicenseStatus:  report[winguest.KVPLicenseStatus],
		LicenseChannel: report[winguest.KVPLicenseChannel],
		PreparedBy:     driver.PreparedBy(),
	}
	m, err := loadPreparedManifest(goarch)
	if err != nil {
		return PreparedLayer{}, err
	}
	m.Prepared = append(m.Prepared, pl)
	m.Active = layer
	return pl, savePreparedManifest(goarch, m)
}

// retireWindowsLayers deletes layers older than the active one and the one it
// replaced. qemu keeps no host-wide VM registry to ask, so this relies on no
// build outliving two refresh periods on a superseded layer.
func retireWindowsLayers(goarch string, progress io.Writer) {
	m, err := loadPreparedManifest(goarch)
	if err != nil || len(m.Prepared) <= 2 {
		return
	}
	old, keep := m.Prepared[:len(m.Prepared)-2], m.Prepared[len(m.Prepared)-2:]
	var kept []PreparedLayer
	for _, l := range old {
		if l.Path == m.Active {
			kept = append(kept, l)
			continue
		}
		_ = os.Chmod(l.Path, 0o644)
		if err := os.Remove(l.Path); err != nil && !os.IsNotExist(err) {
			fmt.Fprintf(progress, "[warden] warning: retiring %s: %v\n", l.Path, err)
			kept = append(kept, l)
			continue
		}
		_ = os.Remove(l.VarsFD)
		_ = LayerCache().Forget(l.Path)
	}
	m.Prepared = append(kept, keep...)
	if err := savePreparedManifest(goarch, m); err != nil {
		fmt.Fprintf(progress, "[warden] warning: %v\n", err)
	}
}

// discardWindowsLayers deletes every prepared layer for goarch and the
// manifest (the base they overlay is being replaced).
func discardWindowsLayers(goarch string) {
	m, err := loadPreparedManifest(goarch)
	if err != nil {
		return
	}
	for _, l := range m.Prepared {
		_ = os.Chmod(l.Path, 0o644)
		_ = os.Remove(l.Path)
		_ = os.Remove(l.VarsFD)
		_ = LayerCache().Forget(l.Path)
	}
	_ = os.Remove(preparedManifestPath(goarch))
}
