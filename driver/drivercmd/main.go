package drivercmd

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"slices"
	"strings"
)

// The environment variables that name the request and result files
// (DRV-15, DRV-16).
const (
	EnvRequest = "WARDEN_DRIVER_REQUEST"
	EnvResult  = "WARDEN_DRIVER_RESULT"
)

// MaxMessage is the longest message line of run (DRV-39).
const MaxMessage = 1 << 20

// Main runs the command named by os.Args and exits with the contract's code.
// For run it first points the process's standard output at its standard
// error, so that a stray print of the driver's or of a child's cannot
// corrupt the message stream, and ignores SIGPIPE, so that a driver whose
// orchestrator died still finishes its teardown (DRV-51).
func Main(d *Driver) {
	stdout := io.Writer(os.Stdout)
	if len(os.Args) > 1 && os.Args[1] == CmdRun {
		ignoreSIGPIPE()
		if msgs, err := redirectStdout(); err == nil {
			stdout = msgs
		} else {
			fmt.Fprintf(os.Stderr, "[warden] driver: keeping standard output: %v\n",
				err)
		}
	}
	os.Exit(runMain(d, os.Args[1:], os.Environ(), os.Stdin, stdout, os.Stderr))
}

// runMain is Main without the process: it runs the command args[0] of d with
// the environment environ, reading the request from stdin or the request
// file, writing describe's output or run's messages to stdout, and returns
// the exit code. A command that inherits the terminal is given stdin,
// stdout and stderr as its streams.
func runMain(d *Driver, args, environ []string, stdin io.Reader, stdout,
	stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "[warden] driver: no command; this is a driver, "+
			"run by warden")
		return 2
	}
	command, rest := args[0], args[1:]
	if command == CmdDescribe {
		return describe(d, stdout, stderr)
	}
	m := &mainRun{d: d, command: command, environ: environ, stderr: stderr,
		streams: Streams{Stdin: stdin, Stdout: stdout, Stderr: stderr}}
	if command == CmdRun {
		return m.run(stdin, stdout)
	}
	return m.command1(rest)
}

func describe(d *Driver, stdout, stderr io.Writer) int {
	desc, err := d.Description()
	if err == nil && !slices.Contains(desc.Commands, CmdReap) {
		err = fmt.Errorf("driver %q has no reap", d.Describe.Name)
	}
	if err != nil {
		fmt.Fprintf(stderr, "[warden] driver: describe: %v\n", err)
		return 1
	}
	data, err := json.Marshal(desc)
	if err != nil {
		fmt.Fprintf(stderr, "[warden] driver: describe: %v\n", err)
		return 1
	}
	_, _ = stdout.Write(append(data, '\n'))
	return 0
}

// mainRun is one command of a driver process.
type mainRun struct {
	d       *Driver
	command string
	environ []string
	stderr  io.Writer
	streams Streams
}

func (m *mainRun) env(name string) string { return lookupEnv(m.environ, name) }

// command1 runs a command other than describe and run: its request from
// standard input, or from the request file for attach and ext, and its
// result to the result file.
func (m *mainRun) command1(rest []string) int {
	req, err := m.request(rest)
	if err == nil {
		var res *Result
		if res, err = Call(context.Background(), m.d, m.command, req, nil); err == nil {
			return m.finish(res, nil)
		}
	}
	return m.finish(nil, err)
}

// finish writes the result, or the error, and returns the exit code.
func (m *mainRun) finish(res *Result, err error) int {
	fields := map[string]any{}
	code := 0
	switch {
	case err != nil:
		e := ErrorObject(err)
		fields["error"] = e
		code = exitCode(e.Code)
		fmt.Fprintf(m.stderr, "[warden] driver %s: %s\n", m.d.Describe.Name, e.Msg)
	case m.command == CmdAttach:
		code = res.ExitCode
	case m.command != CmdExt:
		fields = resultFields(m.command, res)
	}
	if m.command == CmdExt {
		return code
	}
	if path := m.env(EnvResult); path != "" {
		data, _ := json.Marshal(fields)
		if werr := os.WriteFile(path, append(data, '\n'), 0o600); werr != nil {
			fmt.Fprintf(m.stderr, "[warden] driver: writing the result: %v\n", werr)
			return 1
		}
	}
	return code
}

// The fields a request of each command must carry (v1.schema.json).
var requiredFields = map[string][]string{
	CmdReap:         {"build_id", "state_file"},
	CmdAttach:       {"build_id", "state_file"},
	CmdClean:        {"keep_ids"},
	CmdImagePrepare: {"os"},
}

var buildIDRE = regexp.MustCompile(`^[a-z0-9]{8}$`)

// request reads and checks the command's request (DRV-14, DRV-15, DRV-70).
func (m *mainRun) request(rest []string) (*Request, error) {
	req := &Request{Environ: m.environ, Streams: m.streams}
	if m.command == CmdExt {
		req.ExtArgs = rest
	}
	if !slices.Contains(Commands, m.command) {
		return nil, &Error{Code: CodeUnsupportedCommand,
			Msg: fmt.Sprintf("%q is not a command of the driver contract", m.command)}
	}
	// A command the driver does not list exits 3 before its request is
	// checked (DRV-21).
	if !slices.Contains(m.d.commands(), m.command) {
		return nil, &Error{Code: CodeUnsupportedCommand,
			Msg: fmt.Sprintf("driver %q does not implement %s", m.d.Describe.Name,
				m.command)}
	}
	data, err := m.requestData()
	if err != nil || data == nil {
		return req, err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil || fields == nil {
		return nil, invalid("the request is not a JSON object")
	}
	if err := json.Unmarshal(data, req); err != nil {
		return nil, invalid("the request: %v", err)
	}
	for _, f := range requiredFields[m.command] {
		if _, ok := fields[f]; !ok {
			return nil, invalid("the %s request has no %s", m.command, f)
		}
	}
	return req, checkBuildID(m.command, req, rest)
}

// checkBuildID refuses a reap or attach whose build_id is not a build ID or
// not its argument.
func checkBuildID(command string, req *Request, rest []string) error {
	if command != CmdReap && command != CmdAttach {
		return nil
	}
	if !buildIDRE.MatchString(req.BuildID) {
		return invalid("build_id %q is not a build ID", req.BuildID)
	}
	if len(rest) > 0 && rest[0] != req.BuildID {
		return invalid("the argument %q is not the request's build_id %q",
			rest[0], req.BuildID)
	}
	return nil
}

// requestData is the request's bytes: standard input, or the request file
// for attach and ext. It is nil for an ext started by the system and not
// warden, which runs with no config file and the defaults (DRV-34).
func (m *mainRun) requestData() ([]byte, error) {
	if m.command != CmdAttach && m.command != CmdExt {
		data, err := io.ReadAll(m.streams.stdin())
		if err != nil {
			return nil, invalid("reading the request: %v", err)
		}
		return data, nil
	}
	path := m.env(EnvRequest)
	switch {
	case path == "" && m.command == CmdExt:
		return nil, nil
	case path == "":
		return nil, invalid("%s is not set", EnvRequest)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, invalid("reading the request: %v", err)
	}
	return data, nil
}

// startFields are the fields of start a build that is not a probe must
// carry, and a probe must not (DRV-32).
var startFields = []string{"build_id", "claim_token", "build_script", "diagnostics_dir",
	"state_file"}

// run is the run command: the start message on stdin, messages after it,
// and messages out on stdout (The build).
func (m *mainRun) run(stdin io.Reader, stdout io.Writer) int {
	in := bufio.NewReaderSize(stdin, 64<<10)
	out := &msgWriter{w: stdout}
	line, err := readLine(in)
	if err != nil {
		// The input ended before start: nothing is written (DRV-50).
		return 0
	}
	var start startMessage
	var fields map[string]json.RawMessage
	switch {
	case json.Unmarshal(line, &fields) != nil || fields == nil:
		return m.failed(out, invalid("start is not a JSON object"))
	case json.Unmarshal(line, &start) != nil || start.Type != "start":
		return m.failed(out, invalid("the first message of run is not a valid start"))
	}
	for _, f := range startFields {
		if _, ok := fields[f]; ok == start.Probe {
			return m.failed(out, invalid("start has %s: %v, with probe %v",
				f, ok, start.Probe))
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	host := newPipeHost(out, cancel)
	if !start.Probe {
		go host.read(in)
	}
	req := &Request{Start: &start.Start, Environ: m.environ,
		Streams: Streams{Stderr: m.stderr}}
	res, err := Call(ctx, m.d, CmdRun, req, host)
	switch {
	case errors.Is(err, errStoppedBeforeClaim):
		// Stop or the end of the input came before the claim, and Run has
		// torn down: that is a stop, answered with stopped (DRV-49).
		out.send(stoppedMessage{Type: "stopped"})
		return 0
	case err != nil:
		return m.failed(out, err)
	}
	out.send(stoppedMessage{Type: "stopped", Kept: res.Kept})
	return 0
}

func (m *mainRun) failed(out *msgWriter, err error) int {
	e := ErrorObject(err)
	fmt.Fprintf(m.stderr, "[warden] driver %s: %s\n", m.d.Describe.Name, e.Msg)
	out.send(failedMessage{Type: "failed", Error: e})
	return exitCode(e.Code)
}

// readLine reads one message line, without its newline. A line longer than
// MaxMessage is an error.
func readLine(r *bufio.Reader) ([]byte, error) {
	var buf bytes.Buffer
	for {
		part, err := r.ReadSlice('\n')
		buf.Write(part)
		if buf.Len() > MaxMessage+1 {
			return nil, errors.New("a message is longer than 1 MiB")
		}
		switch {
		case err == nil:
			return bytes.TrimSuffix(buf.Bytes(), []byte("\n")), nil
		case errors.Is(err, bufio.ErrBufferFull):
			continue
		case errors.Is(err, io.EOF) && strings.TrimSpace(buf.String()) != "":
			return buf.Bytes(), nil
		default:
			return nil, err
		}
	}
}
