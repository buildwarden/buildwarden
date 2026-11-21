package container

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/lesiw/ctrctl"

	"github.com/buildwarden/buildwarden/driver"
	"github.com/buildwarden/buildwarden/driver/drivercmd"
)

// BugURL is where the driver's defects are reported.
const BugURL = "https://github.com/buildwarden/buildwarden/issues/new"

// Driver is the container driver as the kit runs it
// (specs/driver-contract/v1.md): cmd/warden-driver-container's main is
// drivercmd.Main(container.Driver()).
func Driver() *drivercmd.Driver {
	return &drivercmd.Driver{
		Describe: drivercmd.Describe{Name: "container", Version: Version,
			Vendor: "buildwarden", BugURL: BugURL,
			Capabilities: drivercmd.Capabilities{Shell: true, Keep: "none",
				Exited: true, Containerfile: "translate",
				GuestOS:        []string{"linux"},
				ReservedRanges: []string{driver.ContainerBaseCIDR}},
			Settings: Settings},
		Preflight: preflight,
		Run:       run,
		Reap:      reap,
		Doctor:    doctor,
		Clean:     clean,
		Attach:    attach,
	}
}

// state is the driver's state file: the runtime the build runs on, which
// reap and attach use whatever their own settings say (DRV-61, DRV-C10).
type state struct {
	Runtime string `json:"runtime"`
}

// preflight is the probe of run: it refuses a relay image that is not one,
// and resolves the runtime (the setting, else detection), refusing one that
// is not supported or does not answer, before anything is created.
func preflight(_ context.Context, start *driver.Start) error {
	_, err := buildRunner(start.Settings)
	return err
}

// buildRunner checks set's relay image and resolves its runtime.
func buildRunner(set driver.Settings) (*runner, error) {
	img, _ := set.String("relay-image")
	if err := checkRelayImage(img, set.Source("relay-image")); err != nil {
		return nil, err
	}
	rt, err := resolveRuntime(set)
	if err != nil {
		return nil, err
	}
	return &runner{runtime: rt, relayOrigin: set.Source("relay-image")}, nil
}

// run is the build: the runtime resolved once more, as after a probe, and
// written to the state file before anything is created.
func run(ctx context.Context, start *driver.Start, host driver.Host) ([]string, error) {
	d, err := buildRunner(start.Settings)
	if err != nil {
		return nil, err
	}
	if start.StateFile != "" {
		err := drivercmd.SaveState(start.StateFile, state{Runtime: d.runtime})
		if err != nil {
			return nil, err
		}
	}
	img, _ := start.Settings.String("relay-image")
	req := &driver.BuildRequest{ContextDir: start.ContextDir, Image: start.Image,
		GuestOS: start.GuestOS, BuildScript: start.BuildScript,
		BaseImage: start.BaseImage, TempDir: start.TempDir, RelayImage: img}
	return d.Run(ctx, req, start, host)
}

// stateRuntime is the runtime of a build: the one its state file names,
// checked again as any other value is, else the one set resolves to. set is
// read only when there is no state file (DRV-C10).
func stateRuntime(stateFile string, set driver.Settings) (string, error) {
	var st state
	found, err := drivercmd.LoadState(stateFile, &st)
	if err != nil {
		return "", err
	}
	if !found {
		return resolveRuntime(set)
	}
	if rt, ok := supportedRuntime(st.Runtime); ok {
		return rt, nil
	}
	return "", driver.Fixable("remove the file; warden clean then reaps the build "+
		"with the runtime its settings name",
		"the state file %s names the runtime %q; the container driver supports %s",
		stateFile, st.Runtime, runtimeNames("and"))
}

// reap removes what build req.BuildID left: its containers, network, dev
// relay image and work directories. A runtime that does not answer is an
// error, never nothing to remove (DRV-24).
func reap(_ context.Context, req *drivercmd.ReapRequest) (int, error) {
	rt, err := stateRuntime(req.StateFile, req.Settings)
	if err != nil {
		return 0, err
	}
	useRuntime(rt)
	w := stderrOf(req.Streams)
	removed, err := removeResources(w, func(id string) bool {
		return id == req.BuildID
	})
	if err != nil {
		return removed, notAnswering(rt, rt)
	}
	n, err := removeWorkDirs(w, req.TempDir, func(id string) bool {
		return id == req.BuildID
	})
	return removed + n, err
}

// doctor checks what a build checks before it creates anything: the relay
// image's shape and the runtime, which it names (DRV-25). warden prints ok
// after it when nothing blocks.
func doctor(_ context.Context, req *drivercmd.DoctorRequest) (bool, error) {
	d, err := buildRunner(req.Settings)
	if err != nil {
		return false, err
	}
	out := stdoutOf(req.Streams)
	fmt.Fprintf(out, "container runtime: %s\n", d.runtime)
	return false, nil
}

// clean removes the containers, networks and dev relay images of every build
// that is neither in keep_ids nor named by a running process (DRV-27).
func clean(_ context.Context, req *drivercmd.CleanRequest) (int, error) {
	rt, err := resolveRuntime(req.Settings)
	if err != nil {
		return 0, err
	}
	useRuntime(rt)
	live := liveProcessBuilds()
	if live == nil {
		live = map[string]bool{}
	}
	for _, id := range req.KeepIDs {
		live[id] = true
	}
	dead := func(id string) bool { return !live[id] }
	removed, err := removeResources(stderrOf(req.Streams), dead)
	if err != nil {
		return removed, notAnswering(rt, rt)
	}
	// And their work directories, as reap removes a build's (DRV-27). One
	// that cannot be removed, such as another user's in a shared temporary
	// directory, is reported and passed over.
	w := stderrOf(req.Streams)
	n, err := removeWorkDirs(w, req.TempDir, dead)
	if err != nil {
		fmt.Fprintf(w, "warning: %v\n", err)
	}
	return removed + n, nil
}

// attach enters the held build of req.BuildID on the runtime its state file
// names (DRV-28).
func attach(_ context.Context, req *drivercmd.AttachRequest) (int, error) {
	rt, err := stateRuntime(req.StateFile, req.Settings)
	if err != nil {
		return 0, err
	}
	return (&runner{runtime: rt}).Attach(req.BuildID, stdinOf(req.Streams),
		stdoutOf(req.Streams), stderrOf(req.Streams))
}

// useRuntime makes rt the runtime ctrctl and the listings run.
func useRuntime(rt string) { ctrctl.Cli = []string{rt} }

// The streams of a command that inherits the terminal; nil is the
// process's own.
func stdinOf(s drivercmd.Streams) io.Reader {
	if s.Stdin == nil {
		return os.Stdin
	}
	return s.Stdin
}

func stdoutOf(s drivercmd.Streams) io.Writer {
	if s.Stdout == nil {
		return os.Stdout
	}
	return s.Stdout
}

func stderrOf(s drivercmd.Streams) io.Writer {
	if s.Stderr == nil {
		return os.Stderr
	}
	return s.Stderr
}
