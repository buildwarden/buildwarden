//go:build linux

package firecracker

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"

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

// StartBuild implements driver.Driver: it takes a network slot, boots and
// claims the relay VM, then boots the build VM whose only NIC is on the
// slot's bridge, and waits for the build to finish.
func (d *Driver) StartBuild(ctx context.Context, req *driver.BuildRequest) (*driver.BuildResult,
	error) {
	if req.GuestOS != "" && req.GuestOS != "linux" {
		return nil, driver.Fixable("drop --guest-os, or use --driver qemu or hyperv for "+
			"Windows",
			"--driver firecracker runs Linux guests only")
	}
	pre, err := buildPreflight(req)
	if err != nil {
		return nil, err
	}
	if req.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, req.Timeout)
		defer cancel()
	}
	ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	buildID := driver.RandAlphaNum(8)
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

	outputDir, err := makeOutputDir(req)
	if err != nil {
		return nil, err
	}
	workDir, err := driver.NewWorkDir(req, "firecracker-"+buildID)
	if err != nil {
		return nil, err
	}
	defer driver.RemoveWorkDir(req, workDir)

	st, err := d.stage(req, pre, workDir, outputDir, buildID)
	if err != nil {
		return nil, err
	}
	if !req.KeepVMs {
		defer st.dlv.remove()
	}

	sess, err := driver.StartSession(driver.SessionConfig{
		OutputDir:   outputDir,
		ContextDir:  req.ContextDir,
		BuildScript: st.script,
		GuestOS:     req.GuestOS,
		OutputTee:   driver.ConsoleTee(req),
		Verbose:     d.Verbose,
	})
	if err != nil {
		return nil, err
	}
	defer sess.Close()

	r := &run{d: d, req: req, slot: slot, work: workDir}
	defer r.teardown()

	if err := r.startRelay(ctx, sess, pre, st.relayInitrd, st.env); err != nil {
		return nil, err
	}
	if err := r.startBuild(st.dlv.boot); err != nil {
		return nil, err
	}
	d.status("running...")
	if err := r.wait(ctx, sess); err != nil {
		return nil, err
	}
	driver.FinalizeOutput(outputDir, req.Compress)
	return &driver.BuildResult{OutputDir: outputDir}, nil
}

// buildPreflight is preflight on this host for req's build link, plus the
// refusal of a link a host interface uses.
func buildPreflight(req *driver.BuildRequest) (*preflighted, error) {
	pre, err := preflight(realHost{}, hostLocal, req.Link())
	if err != nil {
		return nil, err
	}
	return pre, driver.CheckHostLink(req.Link())
}

// makeOutputDir creates the build's output directory (default warden-output).
func makeOutputDir(req *driver.BuildRequest) (string, error) {
	dir := req.OutputDir
	if dir == "" {
		dir = "warden-output"
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("creating output dir: %w", err)
	}
	return dir, nil
}

// staged is a build's inputs and boot files, ready before any VM starts.
type staged struct {
	script      string
	dlv         *delivery
	env         *relaycfg.Environment
	relayInitrd string
}

// stage records the build's inputs (ORCH-02) and readies both VMs' boot
// files and the environment record, so a missing image or asset fails before
// a VM boots.
func (d *Driver) stage(req *driver.BuildRequest, pre *preflighted,
	workDir, outputDir, buildID string) (*staged, error) {
	d.status("preparing build environment...")
	script, err := driver.StageBuildScript(req, workDir)
	if err != nil {
		return nil, fmt.Errorf("staging build script: %w", err)
	}
	if err := driver.RecordInputs(outputDir, req, script); err != nil {
		return nil, fmt.Errorf("recording inputs: %w", err)
	}
	image := linuxguest.ContainerfileImage(req)
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
}

// startRelay boots the relay VM and claims it at the slot's relay address
// (Decision 5). A relay that exits during the claim fails it with the
// relay's console.
func (r *run) startRelay(ctx context.Context, sess *driver.Session, pre *preflighted,
	initrd string, env *relaycfg.Environment) error {
	token, err := driver.RandomToken()
	if err != nil {
		return err
	}
	r.d.status(fmt.Sprintf("starting relay on slot %d...", r.slot))
	cfg := relayConfig(r.slot, pre.assets.Kernel, initrd, relayArgs(r.slot, pre.resolvers,
		token))
	if r.relay, err = startVMM(r.work, "relay", cfg, r.d.Verbose, r.req.KeepVMs); err != nil {
		return err
	}
	if err := writePidFile(r.slot, r.relay); err != nil {
		return fmt.Errorf("recording VMM pids: %w", err)
	}
	cctx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		select {
		case <-r.relay.done:
			cancel()
		case <-cctx.Done():
		}
	}()
	if _, err := sess.ClaimRelay(cctx, r.slot.ClaimAddr(), token, r.req, env,
		r.req.Link().Claim(), driver.OldRelayFix); err != nil {
		if r.relay.exited() {
			return r.vmFailure(errRelayVMExited, r.relay)
		}
		return err
	}
	return nil
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

// wait waits for the build with the exit ordering of Decision 8, turning a
// VM that stopped first into a failure with its console.
func (r *run) wait(ctx context.Context, sess *driver.Session) error {
	err := waitBuild(ctx, sess.Wait, r.build.done, r.relay.done, afterGrace)
	switch {
	case errors.Is(err, errBuildVMExited):
		return r.vmFailure(err, r.build)
	case errors.Is(err, errRelayVMExited):
		return r.vmFailure(err, r.relay)
	}
	return err
}

// vmFailure is a build failed by a VM stopping, with that VM's last console
// lines.
func (r *run) vmFailure(err error, v *vmm) error {
	tail := consoleTail(v.console, 20)
	if tail == "" {
		return fmt.Errorf("build failed: %w", err)
	}
	return fmt.Errorf("build failed: %w; last %s console lines:\n%s", err, v.name,
		indent(tail))
}

func indent(s string) string {
	return "  " + strings.ReplaceAll(s, "\n", "\n  ")
}

// teardown stops both VMs and clears the slot's pid file (ORCH-04), or with
// --keep-vms leaves them running, the slot held by their pid file, and says
// how to stop them.
func (r *run) teardown() {
	if r.req.KeepVMs {
		var ids []string
		for _, v := range []*vmm{r.relay, r.build} {
			if v != nil {
				ids = append(ids, fmt.Sprintf("%s %d", v.name, v.rec.pid))
			}
		}
		fmt.Fprintf(os.Stderr, "[warden] --keep-vms: left VMs running on slot %d (%s); "+
			"consoles in %s; stop them with kill, or warden clean --driver "+
			"firecracker\n",
			r.slot, strings.Join(ids, ", "), r.work)
		return
	}
	for _, v := range []*vmm{r.build, r.relay} {
		if v != nil {
			v.stop()
		}
	}
	_ = os.Remove(pidFilePath(r.slot))
}
