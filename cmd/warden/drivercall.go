package main

import (
	"bufio"
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/buildwarden/buildwarden/driver"
)

// The exec transport (specs/driver-contract/v1.md, Commands): warden runs a
// driver binary's commands as processes. describe and run own the driver's
// standard output; every other command inherits the terminal and writes its
// result to the file WARDEN_DRIVER_RESULT names. The request is on standard
// input, or for attach and ext in the file WARDEN_DRIVER_REQUEST names.

// The fixed limits of the contract: describe answers within 10 s and a probe
// within 30 s (DRV-31, DRV-36).
const (
	describeLimit = 10 * time.Second
	probeLimit    = 30 * time.Second
)

// The environment variables that name the request and result files.
const (
	envDriverRequest = "WARDEN_DRIVER_REQUEST"
	envDriverResult  = "WARDEN_DRIVER_RESULT"
)

// maxMessage is the longest message line of run (DRV-39).
const maxMessage = 1 << 20

// stderrTail is how much of a driver's standard error warden keeps for an
// error report.
const stderrTail = 4 << 10

// driverSchema is a copy of specs/driver-contract/v1.schema.json, which a
// test keeps identical to it.
//
//go:embed driver-contract-v1.schema.json
var driverSchema []byte

// describedFlag is one of describe's flags.
type describedFlag struct {
	Name     string   `json:"name"`
	Value    bool     `json:"value"`
	Commands []string `json:"commands"`
	Usage    string   `json:"usage"`
}

// describedKey is one of describe's keys.
type describedKey struct {
	Key      string   `json:"key"`
	Type     string   `json:"type"`
	Aliases  []string `json:"aliases"`
	Commands []string `json:"commands"`
	Usage    string   `json:"usage"`
}

// describedExt is one of describe's ext commands.
type describedExt struct {
	Path  []string `json:"path"`
	Usage string   `json:"usage"`
}

// description is a driver's describe, as warden reads it.
type description struct {
	Contract     []int  `json:"contract"`
	Name         string `json:"name"`
	Version      string `json:"version"`
	Vendor       string `json:"vendor"`
	BugURL       string `json:"bug_url"`
	Commands     []string
	Capabilities struct {
		VM             bool     `json:"vm"`
		Shell          bool     `json:"shell"`
		Keep           string   `json:"keep"`
		Exited         bool     `json:"exited"`
		Containerfile  string   `json:"containerfile"`
		GuestOS        []string `json:"guest_os"`
		ImageOS        []string `json:"image_os"`
		DefaultImageOS string   `json:"default_image_os"`
		ReservedRanges []string `json:"reserved_ranges"`
	} `json:"capabilities"`
	Flags []describedFlag     `json:"flags"`
	Keys  []describedKey      `json:"keys"`
	Ext   []describedExt      `json:"ext"`
	Help  map[string]string   `json:"help"`
	Extra map[string]struct{} `json:"-"`
}

// caps is the description's capabilities as the build sequence uses them.
func (d *description) caps() capabilities {
	c := d.Capabilities
	return capabilities{VM: c.VM, Shell: c.Shell, Keep: c.Keep, Exited: c.Exited,
		Containerfile: c.Containerfile, GuestOS: c.GuestOS}
}

// lists reports whether the driver implements command.
func (d *description) lists(command string) bool { return slices.Contains(d.Commands, command) }

// clock is when a limit passes; tests replace it.
type clock func(time.Duration) <-chan time.Time

// execDriver is a driver binary warden runs, once described.
type execDriver struct {
	found    *foundDriver
	desc     *description
	contract int
	after    clock
	// teardown is the teardown wait ([build] teardown_timeout).
	teardown time.Duration
	// The terminal a command inherits; nil is the process's own.
	stdin          io.Reader
	stdout, stderr io.Writer
}

func (d *execDriver) out() io.Writer {
	if d.stdout == nil {
		return os.Stdout
	}
	return d.stdout
}

func (d *execDriver) errOut() io.Writer {
	if d.stderr == nil {
		return os.Stderr
	}
	return d.stderr
}

func (d *execDriver) in() io.Reader {
	if d.stdin == nil {
		return os.Stdin
	}
	return d.stdin
}

// describeDriver runs describe on the driver fd names and checks what it
// reports (DRV-36, DRV-38, DRV-69).
func describeDriver(fd *foundDriver, after clock) (*execDriver, error) {
	cmd := exec.Command(fd.Path, "describe")
	var stdout bytes.Buffer
	tail := &tailWriter{max: stderrTail}
	cmd.Stdout, cmd.Stderr = &stdout, tail
	code, err := runLimited(context.Background(), cmd, describeLimit, after)
	who := describedName(fd)
	switch {
	case errors.Is(err, errLimit):
		return nil, defectOf(who, nil, "did not answer describe within 10s", tail.String())
	case err != nil:
		return nil, driver.Fixable("check that the file is a driver for this system",
			"running driver %s: %v", who, err)
	case code != 0:
		return nil, defectOf(who, nil, fmt.Sprintf("exited %d from describe", code),
			tail.String())
	}
	desc, err := parseDescription(stdout.Bytes(), fd.Name)
	if err != nil {
		return nil, defectOf(who, nil, "describe: "+err.Error(), tail.String())
	}
	v := 0
	for _, c := range desc.Contract {
		if c == contractVersion {
			v = c
		}
	}
	if v == 0 {
		return nil, driver.Fixable("update the driver, or warden, so they share a "+
			"contract version",
			"driver %s speaks contract versions %v, and warden speaks %d", who,
			desc.Contract, contractVersion)
	}
	return &execDriver{found: fd, desc: desc, contract: v, after: after,
		teardown: defaultTeardown}, nil
}

// describedName is how a message names the driver fd.
func describedName(fd *foundDriver) string {
	return fmt.Sprintf("%q (%s)", fd.Name, driver.EscapeOutside(fd.Path))
}

// parseDescription checks a describe's output against the schema and the
// rules warden holds a driver to (DRV-36, DRV-38): one JSON object, the name
// looked up, run and reap listed, no flag of warden's on the same command and
// no key outside the driver's two tables.
func parseDescription(out []byte, name string) (*description, error) {
	out = bytes.TrimSpace(out)
	if bytes.Count(out, []byte("\n")) > 0 {
		var probe any
		dec := json.NewDecoder(bytes.NewReader(out))
		if dec.Decode(&probe) == nil && dec.More() {
			return nil, errors.New("printed more than one JSON object")
		}
	}
	if err := validateSchema("describe", out); err != nil {
		return nil, err
	}
	var d description
	if err := json.Unmarshal(out, &d); err != nil {
		return nil, err
	}
	var raw struct {
		Commands []string `json:"commands"`
	}
	_ = json.Unmarshal(out, &raw)
	d.Commands = raw.Commands
	if d.Name != name {
		return nil, fmt.Errorf("name: the driver reports %q, and was looked up as %q",
			d.Name, name)
	}
	for _, c := range []string{"run", "reap"} {
		if !d.lists(c) {
			return nil, fmt.Errorf("commands: %s is not listed", c)
		}
	}
	for _, f := range d.Flags {
		for _, c := range f.Commands {
			if reservedFlag(c, f.Name) {
				return nil, fmt.Errorf("flags: --%s is warden's own flag on %s",
					f.Name, c)
			}
		}
	}
	for _, k := range d.Keys {
		if err := checkKeyPlace(name, k); err != nil {
			return nil, fmt.Errorf("keys: %w", err)
		}
	}
	return &d, nil
}

// validateSchema checks data against the definition def of the contract's
// schema.
func validateSchema(def string, data []byte) error {
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(driverSchema))
	if err != nil {
		return err
	}
	m, _ := doc.(map[string]any)
	id, _ := m["$id"].(string)
	c := jsonschema.NewCompiler()
	if err := c.AddResource(id, doc); err != nil {
		return err
	}
	sch, err := c.Compile(id + "#/$defs/" + def)
	if err != nil {
		return err
	}
	v, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("is not JSON: %w", err)
	}
	if err := sch.Validate(v); err != nil {
		return fmt.Errorf("does not match the contract's schema: %w", err)
	}
	return nil
}

// reservedFlags are warden's flag names by contract command; those under ""
// are warden's on every command (v1.md, Reserved flag names).
var reservedFlags = map[string][]string{
	"": {"driver", "config", "verbose", "color", "help", "version"},
	"run": {"capture", "output", "no-compress", "timeout", "tmpdir", "quiet", "guest-os",
		"script", "image", "cpus", "memory", "keep-vms", "build-link"},
	"doctor":        {"build-link"},
	"image-prepare": {"os", "source"},
	"clean":         {"cache"},
}

func reservedFlag(command, name string) bool {
	return slices.Contains(reservedFlags[""], name) ||
		slices.Contains(reservedFlags[command], name)
}

// commonKeys are the common keys of contract version 1.
var commonKeys = []string{"image_refresh_after", "image_min_grace"}

// oldKeyNames are version 1's old names by driver name, each with its key.
var oldKeyNames = map[string]map[string]string{
	"container": {"runtime.cli": "driver.container.cli",
		"runtime.relay_image": "driver.container.relay_image"},
	"qemu": {"vm.image_refresh_after": "driver.image_refresh_after",
		"vm.image_min_grace": "driver.image_min_grace"},
	"hyperv": {"vm.image_refresh_after": "driver.image_refresh_after",
		"vm.image_min_grace":      "driver.image_min_grace",
		"vm.network":              "driver.hyperv.network",
		"vm.windows_image":        "driver.hyperv.windows_image",
		"vm.windows_image_update": "driver.hyperv.windows_image_update"},
}

// checkKeyPlace refuses a key outside the driver's place in the config file,
// a driver.<key> that is not a common key, and an old name the specification
// does not list for the driver's name (DRV-38, DRV-C4, DRV-C11).
func checkKeyPlace(name string, k describedKey) error {
	own := "driver." + name + "."
	rest, isDriver := strings.CutPrefix(k.Key, "driver.")
	switch {
	case strings.HasPrefix(k.Key, own):
		if slices.Contains(commonKeys, strings.TrimPrefix(k.Key, own)) {
			return fmt.Errorf("%s is the override of a common key, which is listed "+
				"as driver.%s", k.Key, strings.TrimPrefix(k.Key, own))
		}
	case isDriver && !strings.Contains(rest, "."):
		if !slices.Contains(commonKeys, rest) {
			return fmt.Errorf("%s is not a common key of contract version 1", k.Key)
		}
	default:
		return fmt.Errorf("%s is outside [driver] and [driver.%s]", k.Key, name)
	}
	for _, a := range k.Aliases {
		if oldKeyNames[name][a] != k.Key {
			return fmt.Errorf("%s is not an old name of %s for a driver named %q",
				a, k.Key, name)
		}
	}
	return nil
}

// errLimit is a process that did not end within its limit, and was killed.
var errLimit = errors.New("did not answer within its limit")

// runLimited runs cmd to its end and returns its exit code. With a limit, a
// process still running when it passes is killed and errLimit returned; one
// still running when ctx ends is killed and ctx's error returned.
func runLimited(ctx context.Context, cmd *exec.Cmd, limit time.Duration,
	after clock) (int, error) {
	if err := cmd.Start(); err != nil {
		return 0, err
	}
	done := make(chan struct{})
	var waitErr error
	go func() { waitErr = cmd.Wait(); close(done) }()
	var timer <-chan time.Time
	if limit > 0 {
		timer = after(limit)
	}
	select {
	case <-done:
	case <-timer:
		killGroup(cmd)
		<-done
		return 0, errLimit
	case <-ctx.Done():
		killGroup(cmd)
		<-done
		return 0, ctx.Err()
	}
	err := waitErr
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode(), nil
	}
	if err != nil {
		return 0, err
	}
	return 0, nil
}

// callOpts are the parts of one command's call that differ.
type callOpts struct {
	// argv are the arguments after the command name.
	argv []string
	// limit bounds the command, 0 none; the teardown wait bounds a reap at
	// the end of a build, and probeLimit a probe.
	limit time.Duration
	// ctx ends the command early, as a further signal ends a reap.
	ctx context.Context
}

// call runs one command other than describe and run, with req as its
// request, and returns its result object. An exit code of 1 to 4 is the
// error its result holds; anything else is a defect of the driver (DRV-21,
// DRV-22, DRV-65).
func (d *execDriver) call(command string, req any, o callOpts) (map[string]json.RawMessage,
	error) {
	if o.ctx == nil {
		o.ctx = context.Background()
	}
	dir, err := os.MkdirTemp("", "warden-call-")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(dir) }()
	cmd, tail, err := d.command(command, req, o.argv, dir)
	if err != nil {
		return nil, err
	}
	code, err := runLimited(o.ctx, cmd, o.limit, d.after)
	switch {
	case errors.Is(err, errLimit):
		return nil, d.defect(fmt.Sprintf("did not finish %s within %s", command, o.limit),
			tail.String())
	case err != nil:
		return nil, fmt.Errorf("running driver %s: %w", describedName(d.found), err)
	case command == "ext" && code != 0:
		return nil, exitStatusError(code)
	case command == "ext":
		return map[string]json.RawMessage{}, nil
	}
	return d.result(command, code, filepath.Join(dir, "result.json"), tail.String())
}

// command is the process of one command, with its request written where the
// command reads it, in dir, and the tail of its standard error.
func (d *execDriver) command(command string, req any, argv []string,
	dir string) (*exec.Cmd, *tailWriter, error) {
	data, err := json.Marshal(req)
	if err != nil {
		return nil, nil, err
	}
	cmd := exec.Command(d.found.Path, append([]string{command}, argv...)...)
	cmd.Env = os.Environ()
	tail := &tailWriter{max: stderrTail}
	if command == "attach" || command == "ext" {
		reqFile := filepath.Join(dir, "request.json")
		if err := os.WriteFile(reqFile, data, 0o600); err != nil {
			return nil, nil, err
		}
		cmd.Env = append(cmd.Env, envDriverRequest+"="+reqFile)
		cmd.Stdin, cmd.Stdout, cmd.Stderr = d.in(), d.out(), d.errOut()
	} else {
		cmd.Stdin = bytes.NewReader(data)
		cmd.Stdout, cmd.Stderr = d.out(), io.MultiWriter(d.errOut(), tail)
	}
	if command != "ext" {
		cmd.Env = append(cmd.Env, envDriverResult+"="+filepath.Join(dir, "result.json"))
	}
	return cmd, tail, nil
}

// result is what a command that exited with code left in its result file:
// its result, or the error it holds, or the driver's defect.
func (d *execDriver) result(command string, code int, path,
	tail string) (map[string]json.RawMessage, error) {
	res, rerr := readResult(path)
	hasErr := rerr == nil && res["error"] != nil
	switch {
	case command == "attach" && rerr == nil && !hasErr:
		res["exit_code"], _ = json.Marshal(code)
		return res, nil
	case hasErr && command == "attach":
		return nil, d.errorFrom(res["error"], code, tail)
	case code == 0 && rerr == nil:
		return res, nil
	case code < 0 || code > 4 && command != "attach":
		return nil, d.defect(fmt.Sprintf("exited %d from %s, a code the contract does "+
			"not have", code, command), tail)
	case hasErr && code != 0:
		return nil, d.errorFrom(res["error"], code, tail)
	case rerr != nil:
		return nil, d.defect(fmt.Sprintf("exited %d from %s, and its result %v",
			code, command, rerr), tail)
	}
	return nil, d.defect(fmt.Sprintf("exited %d from %s with no error in its result",
		code, command), tail)
}

// readResult reads a result file, which must hold one JSON object.
func readResult(path string) (map[string]json.RawMessage, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, errors.New("is missing")
	}
	if err != nil {
		return nil, err
	}
	var m map[string]json.RawMessage
	if json.Unmarshal(data, &m) != nil || m == nil {
		return nil, errors.New("is not one JSON object")
	}
	return m, nil
}

// errorObject is the contract's error object.
type errorObject struct {
	Code    string         `json:"code"`
	Msg     string         `json:"msg"`
	Fix     string         `json:"fix"`
	Details map[string]any `json:"details"`
}

// errorFrom maps a driver's error object to warden's error (DRV-66).
func (d *execDriver) errorFrom(raw json.RawMessage, code int, tail string) error {
	var e errorObject
	if json.Unmarshal(raw, &e) != nil || e.Code == "" {
		return d.defect(fmt.Sprintf("exited %d with an error that is not an error object",
			code), tail)
	}
	return d.mapError(e, tail)
}

func (d *execDriver) mapError(e errorObject, tail string) error {
	switch e.Code {
	case "fixable":
		return driver.Fixable(e.Fix, "%s", e.Msg)
	case "busy":
		return &driverError{driverName: d.found.Name, msg: e.Msg}
	case "unsupported-contract":
		return driver.Fixable("update the driver, or warden, so they share a contract "+
			"version", "driver %s: %s (warden speaks contract %d)",
			describedName(d.found), e.Msg, d.contract)
	case "invalid-request", "unsupported-command":
		return d.defect(fmt.Sprintf("refused warden's request (%s): %s", e.Code, e.Msg),
			tail)
	}
	err := d.defect(e.Msg, tail)
	err.internal = true
	return err
}

// defect is a defect of this driver, reported with its own bug link.
func (d *execDriver) defect(what, tail string) *driverError {
	return defectOf(describedName(d.found), d.desc, what, tail)
}

// driverError is a failure a driver reported, or a defect of the driver
// (DRV-67): main prints it with the tail of the driver's standard error and
// the driver's bug link, not warden's.
type driverError struct {
	driverName string
	msg        string
	// bug is true for a defect or an internal error: the driver's bug text
	// follows, naming its version and bug_url.
	bug      bool
	internal bool
	version  string
	bugURL   string
	tail     string
}

func (e *driverError) Error() string {
	if e.internal || !e.bug {
		return fmt.Sprintf("driver %s: %s", e.driverName, e.msg)
	}
	return fmt.Sprintf("driver defect: the driver %s %s", e.driverName, e.msg)
}

func defectOf(who string, desc *description, what, tail string) *driverError {
	e := &driverError{driverName: who, msg: what, bug: true, tail: tail}
	if desc != nil {
		e.version, e.bugURL = desc.Version, desc.BugURL
	}
	return e
}

// report prints the error as main does for a driver's error.
func (e *driverError) report() {
	log.Error(e.Error())
	if t := strings.TrimSpace(e.tail); t != "" {
		fmt.Fprintf(log.out, "  The driver's last output:\n")
		for _, l := range strings.Split(t, "\n") {
			fmt.Fprintf(log.out, "    %s\n", driver.EscapeOutside(l))
		}
	}
	if !e.bug {
		return
	}
	who := e.driverName
	if e.version != "" {
		who += " version " + e.version
	}
	where := "to whoever provides it"
	if e.bugURL != "" {
		where = "at " + driver.EscapeOutside(e.bugURL)
	}
	fmt.Fprintf(log.out, "\n  This is a bug in the driver %s; report it %s\n\n",
		driver.EscapeOutside(who), where)
}

// tailWriter keeps the last max bytes written to it.
type tailWriter struct {
	mu  sync.Mutex
	max int
	buf []byte
}

func (t *tailWriter) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.buf = append(t.buf, p...)
	if len(t.buf) > t.max {
		t.buf = t.buf[len(t.buf)-t.max:]
	}
	return len(p), nil
}

func (t *tailWriter) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return string(t.buf)
}

// checkUTF8Paths refuses, before a driver binary starts, a path a request
// would carry that is not valid UTF-8, which JSON cannot carry unchanged
// (DRV-19).
func checkUTF8Paths(paths map[string]string) error {
	for _, what := range []string{"context", "output", "temporary"} {
		p := paths[what]
		if p != "" && !utf8.ValidString(p) {
			return driver.Fixable("rename it, or choose another directory whose "+
				"path is valid UTF-8", "the %s directory %s is not a valid UTF-8 "+
				"path, which a driver binary cannot be sent", what,
				driver.EscapeOutside(p))
		}
	}
	return nil
}

// probeRun probes a driver binary's run with the start message start, which
// has Probe set (DRV-30, DRV-32, DRV-33): one message, stopped or failed,
// within 30 s.
func (d *execDriver) probeRun(start *driver.Start) error {
	line, err := json.Marshal(startMessage{Type: "start", Start: start})
	if err != nil {
		return err
	}
	cmd := exec.Command(d.found.Path, "run")
	cmd.Stdin = bytes.NewReader(append(line, '\n'))
	var stdout bytes.Buffer
	tail := &tailWriter{max: stderrTail}
	cmd.Stdout, cmd.Stderr = &stdout, io.MultiWriter(d.errOut(), tail)
	code, err := runLimited(context.Background(), cmd, probeLimit, d.after)
	switch {
	case errors.Is(err, errLimit):
		return d.defect("did not answer the probe of run within 30s", tail.String())
	case err != nil:
		return fmt.Errorf("running driver %s: %w", describedName(d.found), err)
	}
	lines := bytes.Split(bytes.TrimSpace(stdout.Bytes()), []byte("\n"))
	var m struct {
		Type  string          `json:"type"`
		Error json.RawMessage `json:"error"`
	}
	if len(lines) != 1 || json.Unmarshal(lines[0], &m) != nil {
		return d.defect("did not answer the probe of run with one message", tail.String())
	}
	switch {
	case m.Type == "stopped" && code == 0:
		return nil
	case m.Type == "failed" && code >= 1 && code <= 4:
		return d.errorFrom(m.Error, code, tail.String())
	}
	return d.defect(fmt.Sprintf("answered the probe of run with %q and exit code %d",
		m.Type, code), tail.String())
}

// startMessage is start as it is written.
type startMessage struct {
	Type string `json:"type"`
	*driver.Start
}

// execPeer is a driver binary's run, over its pipes: the build sequence's
// runPeer for the exec transport.
type execPeer struct {
	d   *execDriver
	rec buildRecord
	// stateFile is the build's state file, for a reap.
	stateFile string
	ev        chan runEvent

	stopOnce sync.Once
	stopCh   chan stopMessage
	kills    chan struct{}

	// leftovers is set once the driver had to be killed and its reap did not
	// remove what the build left: its record and lock stay.
	mu        sync.Mutex
	leftovers bool
}

// stopMessage is stop as it is written.
type stopMessage struct {
	Type   string `json:"type"`
	Reason string `json:"reason"`
	Detail string `json:"detail"`
}

func newExecPeer(d *execDriver, rec buildRecord, stateFile string) *execPeer {
	return &execPeer{d: d, rec: rec, stateFile: stateFile, ev: make(chan runEvent),
		stopCh: make(chan stopMessage, 1), kills: make(chan struct{}, 8)}
}

func (p *execPeer) events() <-chan runEvent { return p.ev }

func (p *execPeer) stop(reason, detail string) {
	p.stopOnce.Do(func() {
		p.stopCh <- stopMessage{Type: "stop", Reason: reason, Detail: detail}
	})
}

// kill is a further signal: it kills the driver's process group, or the reap
// that followed (DRV-52).
func (p *execPeer) kill() {
	select {
	case p.kills <- struct{}{}:
	default:
	}
}

// hasLeftovers reports whether the build's record and lock must stay.
func (p *execPeer) hasLeftovers() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.leftovers
}

// inMessage is a message from the driver, the fields of every type.
type inMessage struct {
	Type        string               `json:"type"`
	Claim       driver.ClaimEndpoint `json:"claim"`
	ClaimValues driver.ClaimValues   `json:"claim_values"`
	GuestOS     string               `json:"guest_os"`
	OldRelayFix string               `json:"old_relay_fix"`
	Which       string               `json:"which"`
	Detail      string               `json:"detail"`
	Kept        []string             `json:"kept"`
	Error       json.RawMessage      `json:"error"`
}

func (p *execPeer) begin(start *driver.Start) { go p.loop(start) }

// runPipes is a driver's run process and its pipes.
type runPipes struct {
	cmd    *exec.Cmd
	exited chan struct{}
	lines  chan []byte
	bad    chan string
	tail   *tailWriter
	wmu    sync.Mutex
	stdin  io.WriteCloser
}

// write sends one message to the driver; a failed write is the driver's end,
// which the loop sees.
func (r *runPipes) write(v any) {
	data, _ := json.Marshal(v)
	r.wmu.Lock()
	defer r.wmu.Unlock()
	_, _ = r.stdin.Write(append(data, '\n'))
}

func (r *runPipes) closeInput() {
	r.wmu.Lock()
	defer r.wmu.Unlock()
	_ = r.stdin.Close()
}

// startRun starts the driver's run in its own process group, with its
// message lines read as they come.
func (p *execPeer) startRun() (*runPipes, error) {
	d := p.d
	r := &runPipes{cmd: exec.Command(d.found.Path, "run"), exited: make(chan struct{}),
		lines: make(chan []byte), bad: make(chan string, 1),
		tail: &tailWriter{max: stderrTail}}
	ownGroup(r.cmd)
	var err error
	if r.stdin, err = r.cmd.StdinPipe(); err != nil {
		return nil, err
	}
	stdout, err := r.cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	r.cmd.Stderr = io.MultiWriter(d.errOut(), r.tail)
	if err := r.cmd.Start(); err != nil {
		return nil, fmt.Errorf("running driver %s: %w", describedName(d.found), err)
	}
	go func() {
		defer close(r.lines)
		br := bufio.NewReaderSize(stdout, 64<<10)
		for {
			line, err := readMessage(br)
			if err != nil {
				if !errors.Is(err, io.EOF) {
					r.bad <- err.Error()
				}
				return
			}
			r.lines <- line
		}
	}()
	go func() { _ = r.cmd.Wait(); close(r.exited) }()
	return r, nil
}

// loop runs the driver's run and turns its messages into events, until its
// last. A driver that breaks the dialogue is killed, and its build reaped.
func (p *execPeer) loop(start *driver.Start) {
	r, err := p.startRun()
	if err != nil {
		p.ev <- runEvent{kind: evEnd, err: err}
		return
	}
	r.write(startMessage{Type: "start", Start: start})
	end := p.dialogue(r)
	// stopped or failed: the driver tore down; it exits next.
	r.closeInput()
	select {
	case <-r.exited:
	case <-p.d.after(p.d.teardown):
		killGroup(r.cmd)
		<-r.exited
	case <-p.kills:
		killGroup(r.cmd)
		<-r.exited
	}
	p.ev <- *end
}

// dialogue reads the driver's messages, sends stop when asked, and bounds
// the teardown, until the run's end.
func (p *execPeer) dialogue(r *runPipes) *runEvent {
	var timer <-chan time.Time
	stopCh, lines := p.stopCh, r.lines
	var end *runEvent
	for end == nil {
		select {
		case line, ok := <-lines:
			if !ok {
				lines = nil
				continue
			}
			end = p.onLine(r, line)
		case what := <-r.bad:
			end = p.reapEnd(r, "wrote "+what)
		case <-r.exited:
			for lines != nil && end == nil { // what it wrote before it ended
				line, ok := <-lines
				if !ok {
					break
				}
				end = p.onLine(r, line)
			}
			if end == nil {
				end = p.reapEnd(r, "ended with neither stopped nor failed")
			}
		case m := <-stopCh:
			stopCh = nil
			r.write(m)
			r.closeInput()
			timer = p.d.after(p.d.teardown)
		case <-timer:
			what := fmt.Sprintf("did not stop within the teardown wait (%s)",
				p.d.teardown)
			end = p.reapEnd(r, what)
		case <-p.kills:
			end = p.reapEnd(r, "was killed on a second signal")
			end.err = killedError{end.err}
		}
	}
	return end
}

// reapEnd is the end of a run whose driver had to be killed.
func (p *execPeer) reapEnd(r *runPipes, what string) *runEvent {
	return &runEvent{kind: evEnd, err: p.killAndReap(r, what)}
}

// killedError is the defect of a driver killed on a second signal, which is
// the user's choice and not the driver's fault: the build reports its
// interruption, and this is said beside it.
type killedError struct{ err error }

func (e killedError) Error() string { return e.err.Error() }
func (e killedError) Unwrap() error { return e.err }

// onLine handles one message line; it returns the run's end for stopped,
// failed or a line that is not a message, after which the driver is killed
// and its build reaped.
func (p *execPeer) onLine(r *runPipes, line []byte) *runEvent {
	var m inMessage
	var obj map[string]json.RawMessage
	if json.Unmarshal(line, &obj) != nil || obj == nil || json.Unmarshal(line, &m) != nil {
		return p.reapEnd(r, "wrote a line on run's output that is not a JSON object")
	}
	switch m.Type {
	case "relay-started":
		reply := make(chan error, 1)
		p.ev <- runEvent{kind: evRelayStarted, reply: reply, relay: driver.RelayStarted{
			Claim: m.Claim, ClaimValues: m.ClaimValues, GuestOS: m.GuestOS,
			OldRelayFix: m.OldRelayFix}}
		go func() {
			if <-reply == nil {
				r.write(map[string]string{"type": "claimed"})
			}
		}()
	case "environment-started":
		p.ev <- runEvent{kind: evStarted}
	case "exited":
		p.ev <- runEvent{kind: evExited, which: driver.ExitedPart(m.Which),
			detail: m.Detail}
	case "stopped":
		return &runEvent{kind: evEnd, kept: m.Kept}
	case "failed":
		return &runEvent{kind: evEnd, err: p.d.errorFrom(m.Error, 1, r.tail.String())}
	}
	return nil
}

// killAndReap kills the driver's process group, then runs reap on the build
// with the record's values, bounded by the teardown wait and by a further
// signal (DRV-52, DRV-54). It returns the driver's defect; when the reap did
// not succeed, the build's leftovers stay for warden clean.
func (p *execPeer) killAndReap(r *runPipes, what string) error {
	killGroup(r.cmd)
	<-r.exited
	defect := p.d.defect(what, r.tail.String())
	if p.rec.ID == "" {
		return defect
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		select {
		case <-p.kills:
			cancel()
		case <-ctx.Done():
		}
	}()
	_, err := p.d.call("reap", reapRequest(p.rec, p.stateFile, p.d.contract),
		callOpts{argv: []string{p.rec.ID}, limit: p.d.teardown, ctx: ctx})
	if err != nil {
		p.mu.Lock()
		p.leftovers = true
		p.mu.Unlock()
		log.Info(fmt.Sprintf("the build's leftovers remain for `warden clean`: reap: %v",
			err))
	}
	return defect
}

// readMessage reads one message line of at most maxMessage bytes.
func readMessage(r *bufio.Reader) ([]byte, error) {
	var buf bytes.Buffer
	for {
		part, err := r.ReadSlice('\n')
		buf.Write(part)
		if buf.Len() > maxMessage+1 {
			return nil, errors.New("a message longer than 1 MiB")
		}
		switch {
		case err == nil:
			return bytes.TrimSuffix(buf.Bytes(), []byte("\n")), nil
		case errors.Is(err, bufio.ErrBufferFull):
			continue
		case errors.Is(err, io.EOF) && buf.Len() > 0:
			return buf.Bytes(), nil
		default:
			return nil, err
		}
	}
}

// reapRequest is reap's request for the build rec records (DRV-60).
func reapRequest(rec buildRecord, stateFile string, contract int) map[string]any {
	req := map[string]any{"contract": contract, "build_id": rec.ID, "tmpdir": rec.TempDir,
		"args": nonNil(rec.Args), "state_file": stateFile}
	if rec.ConfigFile != "" {
		req["config_file"] = rec.ConfigFile
	}
	return req
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// withConfig adds config_file to a request when there is a config file
// (DRV-62).
func withConfig(req map[string]any, configFile string) map[string]any {
	if configFile != "" {
		req["config_file"] = configFile
	}
	return req
}

// resultInt reads an integer field of a result.
func resultInt(res map[string]json.RawMessage, field string) int {
	var n int
	_ = json.Unmarshal(res[field], &n)
	return n
}
