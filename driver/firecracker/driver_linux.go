//go:build linux

package firecracker

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/buildwarden/buildwarden/driver"
	"github.com/buildwarden/buildwarden/driver/linuxguest"
	"github.com/buildwarden/buildwarden/driver/relayvm"
	"github.com/buildwarden/buildwarden/relaycfg"
)

func hostSupported() error {
	if runtime.GOARCH != "amd64" {
		return unsupportedHost(runtime.GOOS, runtime.GOARCH)
	}
	return nil
}

// Preflight is the firecracker driver's probe: the checks it makes before it
// creates anything, which Run makes again.
func (d *Driver) Preflight(req *driver.BuildRequest) error {
	_, err := buildPreflight(req)
	return err
}

// Run is the firecracker driver's run of one build
// (specs/driver-contract/v1.md, The build): it takes a network slot, boots
// the relay VM and reports it through host, and once warden has claimed it
// boots the build VM whose only NIC is on the slot's bridge. It reports
// either VM's exit as exited, and stops both when host.Wait returns, unless
// they are kept; it returns what it kept.
func (d *Driver) Run(ctx context.Context, req *driver.BuildRequest, start *driver.Start,
	host driver.Host) (kept []string, err error) {
	pre, err := buildPreflight(req)
	if err != nil {
		return nil, err
	}
	buildID := start.BuildID
	unlockBuild, err := driver.HostLock(buildLockName(buildID))
	if err != nil {
		return nil, err
	}
	defer unlockBuild()

	// The slot comes first so a busy host fails before the image conversion
	// and rootfs copy, which can take minutes (UX-04).
	slot, release, err := acquireSlot(pre.slots)
	if err != nil {
		return nil, err
	}
	defer release()

	workDir, err := driver.NewWorkDir(req, "firecracker-"+buildID)
	if err != nil {
		return nil, err
	}
	defer driver.RemoveWorkDir(req, workDir)

	st, err := d.stage(req, pre, workDir, buildID)
	if err != nil {
		return nil, err
	}
	if !req.KeepVMs {
		defer st.dlv.remove()
	}

	r := &run{d: d, req: req, slot: slot, work: workDir}
	defer func() { kept = r.teardown() }()

	if err := bootInOrder(
		func() error {
			return r.startRelay(ctx, host, pre, st.relayInitrd, st.env,
				start.ClaimToken)
		},
		func() error { return r.startBuild(st.dlv.boot) },
	); err != nil {
		return nil, err
	}
	r.watch(host, driver.ExitedBuild, r.build)
	if err := host.Started(); err != nil {
		return nil, err
	}
	d.status("running...")
	host.Wait(ctx)
	return nil, nil
}

// buildPreflight is requestPreflight on this host, after the refusal of a
// guest other than Linux, plus the refusal of a link a host interface uses.
func buildPreflight(req *driver.BuildRequest) (*preflighted, error) {
	if req.GuestOS != "" && req.GuestOS != "linux" {
		return nil, driver.Fixable("drop --guest-os, or use --driver qemu or hyperv for "+
			"Windows",
			"--driver firecracker runs Linux guests only")
	}
	pre, err := requestPreflight(realHost{}, hostLocal, req)
	if err != nil {
		return nil, err
	}
	return pre, driver.CheckHostLink(req.Link())
}

// staged is a build's inputs and boot files, ready before any VM starts.
type staged struct {
	script      string
	dlv         *delivery
	env         *relaycfg.Environment
	relayInitrd string
}

// stage readies both VMs' boot files and the environment record, so a missing
// image or asset fails before a VM boots.
func (d *Driver) stage(req *driver.BuildRequest, pre *preflighted,
	workDir, buildID string) (*staged, error) {
	d.status("preparing build environment...")
	script := req.BuildScript
	image := linuxguest.RequestImage(req)
	if image != "" {
		if err := checkDiskTools(realHost{}); err != nil {
			return nil, err
		}
	} else if err := launcherStamp().Check(); err != nil {
		return nil, err
	}
	dlv, err := deliver(pre.assets, image, workDir, buildID, script, req.Link())
	if err != nil {
		return nil, err
	}
	env, err := describeBuild(LayerCache(), dlv.record)
	if err != nil {
		dlv.remove()
		return nil, fmt.Errorf("describing build environment: %w", err)
	}
	initrd, err := relayvm.RelayInitrd(pre.assets.RelayInitramfs, workDir, "amd64")
	if err != nil {
		dlv.remove()
		return nil, fmt.Errorf("composing relay initrd: %w", err)
	}
	return &staged{script: script, dlv: dlv, env: env, relayInitrd: initrd}, nil
}

// run is one build's VMs on its slot, for teardown.
type run struct {
	d            *Driver
	req          *driver.BuildRequest
	slot         Slot
	work         string
	relay, build *vmm
	// stops stop the watches of the VMs' exits.
	stops []func()
}

// startRelay boots the relay VM with the claim token and reports it at the
// slot's relay address (Decision 5), watching for its exit, which warden
// takes as the claim's end when the claim is in progress.
func (r *run) startRelay(ctx context.Context, host driver.Host, pre *preflighted,
	initrd string, env *relaycfg.Environment, token string) error {
	r.d.status(fmt.Sprintf("starting relay on slot %d...", r.slot))
	cfg := relayConfig(r.slot, pre.assets.Kernel, initrd, relayArgs(r.slot, token))
	var err error
	if r.relay, err = startVMM(r.work, "relay", cfg, r.d.Verbose, r.req.KeepVMs); err != nil {
		return err
	}
	if err := writePidFile(r.slot, r.relay); err != nil {
		return fmt.Errorf("recording VMM pids: %w", err)
	}
	r.watch(host, driver.ExitedRelay, r.relay)
	return host.ClaimRelay(ctx, driver.RelayStarted{
		Claim: driver.ClaimEndpoint{Network: driver.ClaimTCP,
			Address: r.slot.ClaimAddr()},
		ClaimValues: driver.ClaimValues{Environment: env,
			BuildLink:   r.req.Link().Claim(),
			UpstreamDNS: driver.UpstreamDNS(r.req.DNS)},
		GuestOS:     "linux",
		OldRelayFix: driver.OldRelayFix,
	})
}

// watch reports v's exit through host as which's, with v's last console
// lines; teardown stops the watch first.
func (r *run) watch(host driver.Host, which driver.ExitedPart, v *vmm) {
	r.stops = append(r.stops, driver.WatchExit(host, which, v.done, func() string {
		return vmExitDetail(v)
	}))
}

// startBuild boots the build VM on the slot's bridge and records both VMMs.
func (r *run) startBuild(boot buildBoot) error {
	r.d.status("starting build...")
	cpus := driver.OrDefault(r.req.CPUs, buildVCPUs)
	mem := driver.OrDefault(r.req.MemoryMB, buildMemoryMB)
	var err error
	if r.build, err = startVMM(r.work, "build", buildConfig(r.slot, boot, cpus, mem),
		r.d.Verbose, r.req.KeepVMs); err != nil {
		return err
	}
	if err := writePidFile(r.slot, r.relay, r.build); err != nil {
		return fmt.Errorf("recording VMM pids: %w", err)
	}
	return nil
}

// vmExitDetail says that v's VM stopped, with its last console lines.
func vmExitDetail(v *vmm) string {
	what := fmt.Sprintf("the %s VM stopped", v.name)
	tail := consoleTail(v.console, 20)
	if tail == "" {
		return what
	}
	return fmt.Sprintf("%s; last %s console lines:\n%s", what, v.name, indent(tail))
}

func indent(s string) string {
	return "  " + strings.ReplaceAll(s, "\n", "\n  ")
}

// teardown stops the exit watches, then stops both VMs and clears the slot's
// pid file (ORCH-04), or with --keep-vms leaves them running, the slot held
// by their pid file, says how to stop them, and returns what it left.
func (r *run) teardown() (kept []string) {
	for _, stop := range r.stops {
		stop()
	}
	if r.req.KeepVMs {
		var ids []string
		for _, v := range []*vmm{r.relay, r.build} {
			if v == nil {
				continue
			}
			ids = append(ids, fmt.Sprintf("%s %d", v.name, v.rec.pid))
			kept = append(kept, fmt.Sprintf("the %s VM running on slot %d (pid %d)",
				v.name, r.slot, v.rec.pid))
		}
		if len(kept) == 0 {
			return nil
		}
		fmt.Fprintf(os.Stderr, "[warden] --keep-vms: left VMs running on slot %d (%s); "+
			"consoles in %s; stop them with kill, or warden clean --driver "+
			"firecracker\n",
			r.slot, strings.Join(ids, ", "), r.work)
		return kept
	}
	for _, v := range []*vmm{r.build, r.relay} {
		if v != nil {
			v.stop()
		}
	}
	_ = os.Remove(pidFilePath(r.slot))
	return nil
}
