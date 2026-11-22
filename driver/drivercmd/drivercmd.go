// Package drivercmd implements the driver command contract
// (specs/driver-contract/v1.md) for a driver written in Go.
//
// A driver's main is one call, drivercmd.Main(d), with d its Driver: what
// describe reports and a function per command. Main dispatches the command
// argv[1] names, decodes and checks its request, resolves the driver's
// settings from the request's args and config_file and from the environment
// (Resolve), calls the command's function, and writes its result and exit
// code as the contract gives them. For run it speaks the message stream on
// standard input and output, and gives the driver a driver.Host over it.
//
// The Go types are not the contract: they may change with the kit, and the
// specification and its conformance suite are what a driver is held to.
package drivercmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"

	"github.com/buildwarden/buildwarden/driver"
)

// The commands of the contract, as argv[1] names them.
const (
	CmdDescribe     = "describe"
	CmdRun          = "run"
	CmdReap         = "reap"
	CmdDoctor       = "doctor"
	CmdSetup        = "setup"
	CmdImagePrepare = "image-prepare"
	CmdImageList    = "image-list"
	CmdImageVerify  = "image-verify"
	CmdClean        = "clean"
	CmdAttach       = "attach"
	CmdExt          = "ext"
)

// Commands are the commands of the contract other than describe, in the
// order describe lists them.
var Commands = []string{CmdRun, CmdReap, CmdDoctor, CmdSetup, CmdImagePrepare,
	CmdImageList, CmdImageVerify, CmdClean, CmdAttach, CmdExt}

// flagCommands are the commands a driver's flag may name (v1.md, Flags).
var flagCommands = []string{CmdRun, CmdDoctor, CmdSetup, CmdImagePrepare, CmdClean}

// unprobed are the commands warden does not probe first, in which a request
// that is not a probe prints the old-name warnings (DRV-C11).
var unprobed = []string{CmdDoctor, CmdSetup, CmdImagePrepare, CmdImageList,
	CmdImageVerify, CmdExt}

// ContractVersions are the versions of the contract the kit speaks.
var ContractVersions = []int{1}

// Driver is one driver: what describe reports and a function per command.
// A nil function is a command the driver does not implement.
type Driver struct {
	// Describe is the driver's name, version, capabilities, settings, ext
	// commands and help.
	Describe Describe
	// Preflight is what a probe of run calls: the checks the driver makes
	// before it creates anything. It creates nothing. Nil makes none.
	Preflight func(ctx context.Context, start *driver.Start) error
	// Run is the build: it starts the relay, reports it through host,
	// starts the environment once host has claimed it, and tears down when
	// host.Wait returns. kept is what start.Keep left in place.
	Run func(ctx context.Context, start *driver.Start,
		host driver.Host) (kept []string, err error)
	// Reap removes what the build of req.BuildID left.
	Reap func(ctx context.Context, req *ReapRequest) (removed int, err error)
	// Doctor reports whether this host can run the driver's builds.
	Doctor func(ctx context.Context, req *DoctorRequest) (blocked bool, err error)
	// Setup makes the driver's one-time host changes.
	Setup func(ctx context.Context, req *SetupRequest) error
	// Prepare is image-prepare: it returns the prepared image's path, or ""
	// when it already reported the outcome.
	Prepare func(ctx context.Context, req *PrepareRequest) (path string, err error)
	// List is image-list: it prints one line per image and returns how many.
	List func(ctx context.Context, req *ListRequest) (count int, err error)
	// Verify is image-verify: it prints one line per cached layer.
	Verify func(ctx context.Context, req *VerifyRequest) (checked, drift int, err error)
	// Clean removes the leftovers of every build not in req.KeepIDs.
	Clean func(ctx context.Context, req *CleanRequest) (removed int, err error)
	// Attach returns the shell's exit code, or an error when the attach
	// itself failed; Main writes the error to the result file.
	Attach func(ctx context.Context, req *AttachRequest) (exitCode int, err error)
	// Ext runs the driver's own subcommands with args as typed.
	Ext func(ctx context.Context, req *ExtRequest, args []string) error
}

// Describe is what the driver's describe reports, besides the commands and
// flags and keys the kit derives.
type Describe struct {
	Name    string
	Version string
	Vendor  string
	BugURL  string
	// Capabilities are reported as they are.
	Capabilities Capabilities
	// Settings are the driver's flags and keys; describe's flags and keys
	// are built from them.
	Settings []Setting
	// Ext are the driver's own subcommands.
	Ext []ExtCommand
	// Help is a paragraph of the driver's own per command.
	Help map[string]string
	// Commands, when not nil, are the commands describe lists, for a driver
	// linked into warden whose other commands warden serves. Otherwise the
	// kit lists the commands whose function is set.
	Commands []string
}

// Capabilities are describe's capabilities (v1.md, describe).
type Capabilities struct {
	VM             bool     `json:"vm"`
	Shell          bool     `json:"shell"`
	Keep           string   `json:"keep"`
	Exited         bool     `json:"exited"`
	Containerfile  string   `json:"containerfile"`
	GuestOS        []string `json:"guest_os"`
	ImageOS        []string `json:"image_os,omitempty"`
	DefaultImageOS string   `json:"default_image_os,omitempty"`
	ReservedRanges []string `json:"reserved_ranges,omitempty"`
}

// ExtCommand is one of the driver's own subcommands.
type ExtCommand struct {
	Path  []string `json:"path"`
	Usage string   `json:"usage"`
}

// Streams are the terminal a command inherits; nil is the process's own.
type Streams struct {
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
}

func (s *Streams) stdin() io.Reader {
	if s.Stdin == nil {
		return os.Stdin
	}
	return s.Stdin
}

func (s *Streams) stdout() io.Writer {
	if s.Stdout == nil {
		return os.Stdout
	}
	return s.Stdout
}

func (s *Streams) stderr() io.Writer {
	if s.Stderr == nil {
		return os.Stderr
	}
	return s.Stderr
}

// Every request type below carries the command's Settings, the ones for
// image-list and image-verify included: their requests name the config file
// too.

// ReapRequest is reap's request.
type ReapRequest struct {
	Streams
	BuildID   string
	TempDir   string
	StateFile string
	// Settings resolve on first read, from the build record's args and
	// config file (DRV-C10).
	Settings driver.Settings
}

// DoctorRequest is doctor's request.
type DoctorRequest struct {
	Streams
	VM       *driver.StartVM
	Verbose  bool
	Settings driver.Settings
}

// SetupRequest is setup's request.
type SetupRequest struct {
	Streams
	VM       *driver.StartVM
	Verbose  bool
	Settings driver.Settings
}

// PrepareRequest is image-prepare's request.
type PrepareRequest struct {
	Streams
	OS       string
	Source   string
	Verbose  bool
	Settings driver.Settings
}

// ListRequest is image-list's request.
type ListRequest struct {
	Streams
	Settings driver.Settings
}

// VerifyRequest is image-verify's request.
type VerifyRequest struct {
	Streams
	Settings driver.Settings
}

// CleanRequest is clean's request.
type CleanRequest struct {
	Streams
	KeepIDs  []string
	Cache    bool
	TempDir  string
	Settings driver.Settings
}

// AttachRequest is attach's request.
type AttachRequest struct {
	Streams
	BuildID   string
	StateFile string
	// Settings resolve on first read, as reap's do.
	Settings driver.Settings
}

// ExtRequest is ext's request.
type ExtRequest struct {
	Streams
	Settings driver.Settings
}

// Request is one command's request, the fields of every command's in one
// value: Main decodes it from the request, and warden's linked transport
// builds it in memory. Start is run's start message.
type Request struct {
	Contract   int             `json:"contract"`
	Probe      bool            `json:"probe"`
	BuildID    string          `json:"build_id"`
	TempDir    string          `json:"tmpdir"`
	StateFile  string          `json:"state_file"`
	ConfigFile string          `json:"config_file"`
	Args       []string        `json:"args"`
	VM         *driver.StartVM `json:"vm"`
	Verbose    bool            `json:"verbose"`
	OS         string          `json:"os"`
	Source     string          `json:"source"`
	KeepIDs    []string        `json:"keep_ids"`
	Cache      bool            `json:"cache"`

	// Start is run's start message; its own ConfigFile, Args and Probe are
	// the ones read.
	Start *driver.Start `json:"-"`
	// ExtArgs are ext's arguments, as typed.
	ExtArgs []string `json:"-"`
	// Environ is the environment settings are resolved against; nil is the
	// process's own.
	Environ []string `json:"-"`
	// Streams are the terminal of a command that inherits one.
	Streams `json:"-"`
}

// Result is what a command returned; Main writes the fields of its command
// to the result file.
type Result struct {
	Removed  int
	Blocked  bool
	Path     string
	Count    int
	Checked  int
	Drift    int
	ExitCode int
	Kept     []string
}

// Call runs one command of d with req, as Main does in a driver process;
// warden's linked transport calls it for a driver linked into warden. It
// resolves and checks the driver's settings with Resolve, prints their
// old-name warnings where the contract says (DRV-C11), answers a probe, and
// calls the command's function; host is run's, and nil for every other
// command. Its error is the command's failure, for ErrorObject to map.
func Call(ctx context.Context, d *Driver, command string, req *Request,
	host driver.Host) (*Result, error) {
	probe, set, err := settingsFor(d, command, req)
	if err != nil {
		return nil, err
	}
	if r, ok := set.(*resolved); ok {
		r.notices = req.stderr()
	}
	if probe || slices.Contains(unprobed, command) {
		for _, w := range set.Warnings() {
			fmt.Fprintln(req.stderr(), w)
		}
	}
	if probe && command != CmdRun {
		return &Result{}, nil
	}
	res, err := dispatch(ctx, d, command, req, set, host)
	if err == nil && (command == CmdReap || command == CmdAttach) && set.Err() != nil {
		err = set.Err()
	}
	return res, err
}

// settingsFor checks req for command of d, and resolves the driver's
// settings from it; probe is whether it is a probe.
func settingsFor(d *Driver, command string, req *Request) (bool, driver.Settings, error) {
	if !slices.Contains(d.commands(), command) {
		return false, nil, &Error{Code: CodeUnsupportedCommand,
			Msg: fmt.Sprintf("driver %q does not implement %s", d.Describe.Name,
				command)}
	}
	probe, configFile, args := req.Probe, req.ConfigFile, req.Args
	if command == CmdRun {
		if req.Start == nil {
			return false, nil, invalid("run has no start message")
		}
		probe, configFile, args = req.Start.Probe, req.Start.ConfigFile, req.Start.Args
		req.Contract = req.Start.Contract
	}
	if err := checkContract(req.Contract, command); err != nil {
		return false, nil, err
	}
	environ := req.Environ
	if environ == nil {
		environ = os.Environ()
	}
	set, err := Resolve(d.Describe.Name, d.Describe.Settings, command, args, configFile,
		environ)
	return probe, set, err
}

// checkContract refuses a request with no version, or one the kit does not
// speak (DRV-69, DRV-70). ext started with none is DRV-34's.
func checkContract(v int, command string) error {
	switch {
	case v == 0 && command == CmdExt:
		return nil
	case v == 0:
		return invalid("the request carries no contract version")
	case !slices.Contains(ContractVersions, v):
		return &Error{Code: CodeUnsupportedContract,
			Msg: fmt.Sprintf("contract version %d is not one this driver speaks (%v)",
				v, ContractVersions),
			Fix: "update the driver, or the orchestrator, so they share a contract " +
				"version"}
	}
	return nil
}

func dispatch(ctx context.Context, d *Driver, command string, req *Request,
	set driver.Settings, host driver.Host) (*Result, error) {
	res := &Result{}
	var err error
	switch command {
	case CmdRun:
		req.Start.Settings = set
		if req.Start.Probe {
			if d.Preflight != nil {
				err = d.Preflight(ctx, req.Start)
			}
			return res, err
		}
		if host == nil {
			return nil, errors.New("run needs a host")
		}
		res.Kept, err = d.Run(ctx, req.Start, host)
	case CmdReap:
		res.Removed, err = d.Reap(ctx, &ReapRequest{Streams: req.Streams,
			BuildID: req.BuildID, TempDir: req.TempDir, StateFile: req.StateFile,
			Settings: set})
	case CmdDoctor:
		res.Blocked, err = d.Doctor(ctx, &DoctorRequest{Streams: req.Streams, VM: req.VM,
			Verbose: req.Verbose, Settings: set})
	case CmdSetup:
		err = d.Setup(ctx, &SetupRequest{Streams: req.Streams, VM: req.VM,
			Verbose: req.Verbose, Settings: set})
	case CmdImagePrepare:
		res.Path, err = d.Prepare(ctx, &PrepareRequest{Streams: req.Streams, OS: req.OS,
			Source: req.Source, Verbose: req.Verbose, Settings: set})
	case CmdImageList:
		res.Count, err = d.List(ctx, &ListRequest{Streams: req.Streams, Settings: set})
	case CmdImageVerify:
		res.Checked, res.Drift, err = d.Verify(ctx, &VerifyRequest{Streams: req.Streams,
			Settings: set})
	case CmdClean:
		res.Removed, err = d.Clean(ctx, &CleanRequest{Streams: req.Streams,
			KeepIDs: req.KeepIDs, Cache: req.Cache, TempDir: req.TempDir,
			Settings: set})
	case CmdAttach:
		res.ExitCode, err = d.Attach(ctx, &AttachRequest{Streams: req.Streams,
			BuildID: req.BuildID, StateFile: req.StateFile, Settings: set})
	case CmdExt:
		err = d.Ext(ctx, &ExtRequest{Streams: req.Streams, Settings: set}, req.ExtArgs)
	}
	return res, err
}

// commands are the commands d lists in describe.
func (d *Driver) commands() []string {
	if d.Describe.Commands != nil {
		return d.Describe.Commands
	}
	set := map[string]bool{
		CmdRun: d.Run != nil, CmdReap: d.Reap != nil, CmdDoctor: d.Doctor != nil,
		CmdSetup: d.Setup != nil, CmdImagePrepare: d.Prepare != nil,
		CmdImageList: d.List != nil, CmdImageVerify: d.Verify != nil,
		CmdClean: d.Clean != nil, CmdAttach: d.Attach != nil, CmdExt: d.Ext != nil,
	}
	var out []string
	for _, c := range Commands {
		if set[c] {
			out = append(out, c)
		}
	}
	return out
}

// resultFields is the result file's object for command.
func resultFields(command string, res *Result) map[string]any {
	switch command {
	case CmdReap, CmdClean:
		return map[string]any{"removed": res.Removed}
	case CmdDoctor:
		return map[string]any{"blocked": res.Blocked}
	case CmdImagePrepare:
		return map[string]any{"path": res.Path}
	case CmdImageList:
		return map[string]any{"count": res.Count}
	case CmdImageVerify:
		return map[string]any{"checked": res.Checked, "drift": res.Drift}
	}
	return map[string]any{}
}
