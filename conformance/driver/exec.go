package driverconformance

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// call is one command run on the driver.
type call struct {
	command string
	// argv are the arguments after the command.
	argv []string
	// request is the request's JSON, or nil for none (describe, or ext with
	// no request file).
	request []byte
	// env are variables set for the driver on top of the suite's own.
	env []string
	// dir is the driver's working directory; "" is a fresh one.
	dir     string
	timeout time.Duration
}

// outcome is what a call came to.
type outcome struct {
	code     int
	stdout   []byte
	stderr   []byte
	timedOut bool
	elapsed  time.Duration
	// result is the result file's object, or for run the last message.
	result map[string]any
	// resultErr says why there is no result.
	resultErr string
}

// errCode is the code of the outcome's error object, or "".
func (o *outcome) errCode() string {
	e, _ := o.result["error"].(map[string]any)
	c, _ := e["code"].(string)
	return c
}

// errText is the message and fix of the outcome's error object.
func (o *outcome) errText() string {
	e, _ := o.result["error"].(map[string]any)
	m, _ := e["msg"].(string)
	f, _ := e["fix"].(string)
	return m + " " + f
}

// same reports whether two outcomes end alike: the exit code and the code
// of the error object.
func (o *outcome) same(p *outcome) bool {
	return o.code == p.code && o.errCode() == p.errCode()
}

// run runs c against the driver at s.path, in a scratch directory whose
// variables TMPDIR, TMP and TEMP name s.tmp.
func (s *Suite) run(c call) *outcome {
	if c.timeout == 0 {
		c.timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()
	scratch, err := os.MkdirTemp(s.scratch, "call-")
	if err != nil {
		return &outcome{code: -1, resultErr: err.Error()}
	}
	cmd := exec.CommandContext(ctx, s.path, append([]string{c.command}, c.argv...)...)
	cmd.Dir = c.dir
	if cmd.Dir == "" {
		cmd.Dir = scratch
	}
	cmd.WaitDelay = 2 * time.Second
	result := filepath.Join(scratch, "result.json")
	cmd.Env = append(s.environ(), "WARDEN_DRIVER_RESULT="+result)
	switch {
	case c.request == nil:
	case c.command == "attach" || c.command == "ext":
		reqFile := filepath.Join(scratch, "request.json")
		if err := os.WriteFile(reqFile, c.request, 0o600); err != nil {
			return &outcome{code: -1, resultErr: err.Error()}
		}
		cmd.Env = append(cmd.Env, "WARDEN_DRIVER_REQUEST="+reqFile)
	default:
		cmd.Stdin = bytes.NewReader(c.request)
	}
	cmd.Env = append(append(cmd.Env, s.env...), c.env...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	start := time.Now()
	err = cmd.Run()
	o := &outcome{stdout: stdout.Bytes(), stderr: stderr.Bytes(),
		elapsed: time.Since(start), timedOut: ctx.Err() != nil}
	var exit *exec.ExitError
	switch {
	case err == nil:
	case errors.As(err, &exit):
		o.code = exit.ExitCode()
	default:
		o.code = -1
		o.resultErr = err.Error()
	}
	if c.command == "run" {
		o.result, o.resultErr = lastMessage(o.stdout)
	} else if data, err := os.ReadFile(result); err == nil {
		if err := json.Unmarshal(data, &o.result); err != nil {
			o.resultErr = "the result file is not one JSON object"
		}
	} else {
		o.resultErr = "no result file"
	}
	return o
}

// lastMessage is the last line of run's output, as a JSON object.
func lastMessage(stdout []byte) (map[string]any, string) {
	var last []byte
	sc := bufio.NewScanner(bytes.NewReader(stdout))
	sc.Buffer(make([]byte, 64<<10), 1<<20+1)
	for sc.Scan() {
		last = append([]byte(nil), sc.Bytes()...)
	}
	if last == nil {
		return nil, "no message"
	}
	var m map[string]any
	if err := json.Unmarshal(last, &m); err != nil {
		return nil, "the last line is not a JSON object"
	}
	return m, ""
}

// environ is the driver's environment: the suite's own with the config
// variable unset and the temporary directory pointed at s.tmp.
func (s *Suite) environ() []string {
	var out []string
	for _, kv := range os.Environ() {
		name, _, _ := strings.Cut(kv, "=")
		switch strings.ToUpper(name) {
		case "WARDEN_CONFIG", "WARDEN_DRIVER_REQUEST", "WARDEN_DRIVER_RESULT", "TMPDIR",
			"TMP", "TEMP":
			continue
		}
		out = append(out, kv)
	}
	out = append(out, "TMPDIR="+s.tmp)
	if runtime.GOOS == "windows" {
		out = append(out, "TMP="+s.tmp, "TEMP="+s.tmp)
	}
	return out
}
