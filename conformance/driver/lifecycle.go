package driverconformance

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
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

	"github.com/buildwarden/buildwarden/collector"
	"github.com/buildwarden/buildwarden/ledger"
	"github.com/buildwarden/buildwarden/relaycfg"
)

// LifecycleOptions configure the lifecycle tier for the host it runs on.
type LifecycleOptions struct {
	// DriverArgs are the driver's flags, passed on as args; ConfigFile is a
	// config file for this host, passed on as config_file (a copy of it, so
	// that the Kill case can delete and overwrite it).
	DriverArgs []string
	ConfigFile string
	// BaseImage and Image say what the build runs in (start.base_image and
	// start.image).
	BaseImage, Image string
	// StubRelay skips the checks that need a ledger: the relay is stubrelay,
	// which writes none.
	StubRelay bool
	// Skip names cases not to run on this host, with --skip.
	Skip []string
	// ExitScript is a build script that ends the environment without
	// completing, for the Exited case on a guest where the default one does
	// not work.
	ExitScript string
	// Teardown bounds each wait for the driver to stop; 0 is 60 s, the
	// orchestrator's default teardown wait (DRV-52).
	Teardown time.Duration
	// CaseDeadline bounds each case; 0 is DefaultCaseDeadline(Teardown),
	// which no case that is still making progress reaches. A case still
	// running then fails as hung, the drivers of its builds are killed and
	// reaped, and the cases after it are not run.
	CaseDeadline time.Duration
}

// lifecycleCase is one case of the lifecycle tier.
type lifecycleCase struct {
	name string
	reqs []string
	run  func(l *lifecycle) (skipped, failure string)
}

// lifecycleCases are the lifecycle tier's cases, in order.
var lifecycleCases = []lifecycleCase{
	{"probe", []string{"DRV-30", "DRV-32", "DRV-33"}, (*lifecycle).caseProbe},
	{"sequence", []string{"DRV-16", "DRV-20", "DRV-39", "DRV-42", "DRV-45", "DRV-46",
		"DRV-47", "DRV-49"}, (*lifecycle).caseSequence},
	{"success", []string{"DRV-14", "DRV-15", "DRV-43", "DRV-45"}, (*lifecycle).caseSuccess},
	{"inputs", []string{"DRV-45"}, (*lifecycle).caseInputs},
	{"failure", []string{"DRV-45"}, (*lifecycle).caseFailure},
	{"stop", []string{"DRV-24", "DRV-49"}, (*lifecycle).caseStop},
	{"end of input", []string{"DRV-50", "DRV-51"}, (*lifecycle).caseEndOfInput},
	{"orphaned", []string{"DRV-51"}, (*lifecycle).caseOrphaned},
	{"kill", []string{"DRV-24", "DRV-61", "DRV-C10"}, (*lifecycle).caseKill},
	{"reap", []string{"DRV-24"}, (*lifecycle).caseReap},
	{"diagnostics", []string{"DRV-45"}, (*lifecycle).caseDiagnostics},
	{"two at once", []string{"DRV-45"}, (*lifecycle).caseTwoAtOnce},
	{"clean", []string{"DRV-27", "DRV-55", "DRV-58"}, (*lifecycle).caseClean},
	{"keep", []string{"DRV-24", "DRV-49"}, (*lifecycle).caseKeep},
	{"shell", []string{"DRV-28", "DRV-53", "DRV-65", "DRV-66"}, (*lifecycle).caseShell},
	{"exited", []string{"DRV-48"}, (*lifecycle).caseExited},
	{"doctor", []string{"DRV-25"}, (*lifecycle).caseDoctor},
}

// lifecycle is one run of the lifecycle tier.
type lifecycle struct {
	s   *Suite
	o   LifecycleOptions
	dir string // the build context, holding input.txt
	// held are the builds whose locks the suite holds, by ID.
	mu   sync.Mutex
	held map[string]*buildRun
}

// Lifecycle runs the lifecycle tier: real builds on this host, with the
// suite in the orchestrator's place (its own implementation of that side,
// from the specification). It needs a static tier's describe, which it runs
// first when the static tier has not run.
func (s *Suite) Lifecycle(o LifecycleOptions) []Result {
	if s.desc == nil {
		if _, failure := s.checkDescribe(); failure != "" {
			return []Result{{Check: "describe", Requirements: []string{"DRV-36"},
				Failure: failure}}
		}
	}
	if o.Teardown == 0 {
		o.Teardown = 60 * time.Second
	}
	if o.CaseDeadline == 0 {
		o.CaseDeadline = DefaultCaseDeadline(o.Teardown)
	}
	l := &lifecycle{s: s, o: o, dir: filepath.Join(s.scratch, "context"),
		held: map[string]*buildRun{}}
	var out []Result
	if err := l.prepare(); err != nil {
		return []Result{{Check: "setup", Failure: err.Error()}}
	}
	hung := ""
	for _, c := range lifecycleCases {
		r := Result{Check: c.name, Requirements: c.reqs}
		switch {
		case slices.Contains(o.Skip, c.name):
			r.Skipped = "skipped with --skip"
		case hung != "":
			r.Failure = "not run: the " + hung + " case hung"
		default:
			var finished bool
			if r.Skipped, r.Failure, finished = l.runCase(c); !finished {
				hung = c.name
				r.Failure = fmt.Sprintf("the case did not finish within %s: "+
					"it hung; %s", o.CaseDeadline, l.killHeld())
			}
		}
		out = append(out, r)
	}
	return out
}

// DefaultCaseDeadline is the case deadline under the teardown wait
// teardown: the slowest case's worst path, every one of its waits run out,
// so only a case blocked outside its waits reaches it (r4, CI repair).
//
// The slowest is Kill with a config file: three variants at each of three
// points, nine builds. One build to a point and its end is bounded by
// upTo's fixed waits, 12 min (claim 2 min, environment-started 5 min, the
// marker 5 min), and at most seven teardown waits (relay-started, stopped,
// the exit, then two reaps of 2 x teardown each, the case's and end's);
// another minute covers the small fixed waits (settle, closeWait,
// drainWait, each run's WaitDelay) and starting the processes. So nine
// builds of 13 min and seven teardown waits: 180 min under the default
// 60 s teardown. Every other case runs fewer builds under the same waits;
// Success, the slowest with one build, is 17 min and five teardown waits.
func DefaultCaseDeadline(teardown time.Duration) time.Duration {
	const builds = 9
	return builds * (13*time.Minute + 7*teardown)
}

// runCase runs c, or gives up on it at the case deadline with finished
// false: a case that hangs is reported as one rather than holding the whole
// tier, and its goroutine is left behind.
func (l *lifecycle) runCase(c lifecycleCase) (skipped, failure string, finished bool) {
	type result struct{ skipped, failure string }
	done := make(chan result, 1)
	go func() {
		s, f := c.run(l)
		done <- result{s, f}
	}()
	select {
	case r := <-done:
		return r.skipped, r.failure, true
	case <-time.After(l.o.CaseDeadline):
		return "", "", false
	}
}

// killHeld kills the drivers of the builds the suite still holds, which a
// hung case may be waiting on, reaps each as end does (DRV-52), releases
// what the suite holds of it, and describes them. Each wait is bounded: the
// exit by Teardown and drainWait, the reap by reap's own 2 x Teardown. The
// hung case's goroutine may still run its end later; release is once only.
func (l *lifecycle) killHeld() string {
	l.mu.Lock()
	builds := make([]*buildRun, 0, len(l.held))
	procs := make([]*os.Process, 0, len(l.held))
	for _, b := range l.held {
		builds = append(builds, b)
		procs = append(procs, b.proc)
	}
	l.mu.Unlock()
	if len(builds) == 0 {
		return "it held no build"
	}
	var held []string
	for i, b := range builds {
		if procs[i] != nil {
			_ = procs[i].Kill()
			select {
			case <-b.exited:
			case <-time.After(l.o.Teardown + drainWait):
			}
		}
		reaped := "reaped"
		if _, err := l.reap(b); err != nil {
			reaped = "not reaped: " + err.Error()
		}
		b.release()
		held = append(held, fmt.Sprintf("build %s, whose driver was killed and %s; "+
			"its log:\n%s", b.id, reaped, b.log()))
	}
	slices.Sort(held)
	return "it held " + strings.Join(held, "\n")
}

// The marker the scripts print, and the file they read from the context.
const (
	marker    = "conformance-marker"
	inputFile = "conformance-input.txt"
)

// script is a build script for both a real guest and stubrelay, which reads
// only its "# stub:" lines. With sleep it waits mid-build after the marker;
// it exits with code.
func script(code int, sleep bool) string {
	var b strings.Builder
	b.WriteString("#!/bin/sh\n# stub: output " + marker + "\necho " + marker + "\n")
	// A build reads its context only through the relay (http://cwd/), so
	// the input is fetched as a translated COPY fetches it, and the ledger
	// records it (the Inputs case).
	b.WriteString("warden-io fetch " + inputFile + " -o /tmp/" + inputFile +
		" >/dev/null 2>&1 || true\n")
	if sleep {
		b.WriteString("# stub: sleep 1h\nsleep 3600\n")
	}
	fmt.Fprintf(&b, "# stub: exit %d\nexit %d\n", code, code)
	return b.String()
}

func (l *lifecycle) prepare() error {
	if err := os.MkdirAll(l.dir, 0o700); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(l.dir, inputFile), []byte("an input\n"), 0o600)
}

// buildRun is one build the suite runs.
type buildRun struct {
	l          *lifecycle
	id         string
	work       string // the suite's directory for the build
	stateFile  string
	configFile string
	diag, out  string
	col        *collector.Collector
	token      string
	lock       *heldLock
	cmd        *exec.Cmd
	proc       *os.Process // cmd's, once started; under l.mu
	stdin      io.WriteCloser
	stdout     io.ReadCloser
	stderr     io.ReadCloser
	msgs       chan map[string]any
	// lines are every stdout line; defect is set when one is not a message.
	mu     sync.Mutex
	lines  []string
	defect string
	errLog bytes.Buffer
	state  *os.ProcessState // the driver's exit, once exited is closed
	// outDone and errDone are closed when the driver's stdout and stderr end;
	// exited when the driver has exited and its output ended, or did not end
	// within drainWait of the exit.
	outDone, errDone, exited chan struct{}
	tunnel                   io.Closer // under l.mu
	released                 sync.Once
}

// drainWait bounds the wait for a driver's output to end after its exit: a
// process the driver started may hold it open.
const drainWait = 5 * time.Second

// newID is a build ID: 8 characters of [a-z0-9] (DRV-40's form).
func newID() string {
	const chars = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	for i := range b {
		b[i] = chars[int(b[i])%len(chars)]
	}
	return string(b)
}

// start starts a build with body as its script, in mode, with keep.
func (l *lifecycle) start(body, mode string, keep bool) (*buildRun, error) {
	b := &buildRun{l: l, id: newID(), token: newID() + newID(), exited: make(chan struct{}),
		outDone: make(chan struct{}), errDone: make(chan struct{}),
		msgs: make(chan map[string]any, 16)}
	b.work = filepath.Join(l.s.scratch, "builds", b.id)
	b.diag, b.out = filepath.Join(b.work, "diagnostics"), filepath.Join(b.work, "out")
	for _, d := range []string{b.diag, b.out} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			return nil, err
		}
	}
	lock, err := takeLock(l.s.name, b.id)
	if err != nil {
		return nil, err
	}
	b.lock = lock
	b.stateFile = lock.path(".state")
	l.mu.Lock()
	l.held[b.id] = b
	l.mu.Unlock()
	if l.o.ConfigFile != "" {
		data, err := os.ReadFile(l.o.ConfigFile)
		if err != nil {
			return nil, err
		}
		b.configFile = filepath.Join(b.work, "warden.toml")
		if err := os.WriteFile(b.configFile, data, 0o600); err != nil {
			return nil, err
		}
	}
	scriptPath := filepath.Join(b.work, "build.sh")
	if err := os.WriteFile(scriptPath, []byte(body), 0o600); err != nil {
		return nil, err
	}
	b.col, err = collector.New(collector.Config{OutputDir: b.out, Token: b.token,
		BuildScriptPath: scriptPath, ContextDir: l.dir})
	if err != nil {
		return nil, err
	}
	msg := l.startMessage(b, mode, keep, false)
	msg["build_script"] = scriptPath
	return b, b.run(msg)
}

// startMessage is the start of b, or a probe's with probe and b nil.
func (l *lifecycle) startMessage(b *buildRun, mode string, keep, probe bool) map[string]any {
	m := map[string]any{"type": "start", "contract": 1, "mode": mode,
		"guest_os": l.s.desc.Capabilities.GuestOS[0], "image": l.o.Image,
		"base_image": l.o.BaseImage, "context_dir": l.dir, "tmpdir": l.s.tmp,
		"keep": keep, "verbose": false, "args": nonNil(l.o.DriverArgs),
		// A field the driver does not know, which it ignores (DRV-20).
		"conformance_unknown_field": true}
	if l.s.desc.Capabilities.VM {
		m["vm"] = map[string]any{}
	}
	if probe {
		m["probe"] = true
		if l.o.ConfigFile != "" {
			m["config_file"] = l.o.ConfigFile
		}
		return m
	}
	m["build_id"], m["claim_token"] = b.id, b.token
	m["diagnostics_dir"], m["state_file"] = b.diag, b.stateFile
	if b.configFile != "" {
		m["config_file"] = b.configFile
	}
	return m
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// run starts the driver's run with msg as its first message.
func (b *buildRun) run(msg map[string]any) error {
	b.cmd = exec.Command(b.l.s.path, "run")
	b.cmd.Dir = b.work
	b.cmd.Env = append(b.l.s.environ(), b.l.s.env...)
	var err error
	if b.stdin, err = b.cmd.StdinPipe(); err != nil {
		return err
	}
	if b.stdout, err = b.cmd.StdoutPipe(); err != nil {
		return err
	}
	if b.stderr, err = b.cmd.StderrPipe(); err != nil {
		return err
	}
	if err := b.cmd.Start(); err != nil {
		return err
	}
	b.l.mu.Lock()
	b.proc = b.cmd.Process
	b.l.mu.Unlock()
	go b.read()
	go func() {
		_, _ = io.Copy(&lockedWriter{mu: &b.mu, w: &b.errLog}, b.stderr)
		_ = b.stderr.Close()
		close(b.errDone)
	}()
	go b.wait()
	return b.send(msg)
}

type lockedWriter struct {
	mu *sync.Mutex
	w  io.Writer
}

func (l *lockedWriter) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.w.Write(p)
}

// read reads the driver's messages until its stdout ends.
func (b *buildRun) read() {
	sc := bufio.NewScanner(b.stdout)
	sc.Buffer(make([]byte, 64<<10), 1<<20+1)
	for sc.Scan() {
		line := sc.Text()
		var m map[string]any
		err := json.Unmarshal([]byte(line), &m)
		b.mu.Lock()
		b.lines = append(b.lines, line)
		if err != nil && b.defect == "" {
			b.defect = fmt.Sprintf("a line on stdout that is not a JSON object: %q",
				line)
		}
		b.mu.Unlock()
		if err == nil {
			b.msgs <- m
		}
	}
	_ = b.stdout.Close()
	close(b.msgs)
	close(b.outDone)
}

// wait waits for the driver's exit, then for its output to end, for
// drainWait at most.
//
// It does not use cmd.Wait, which closes the suite's ends of the pipes as
// soon as the process exits, while read and the stderr copy may still be
// reading them (os/exec's StdoutPipe says not to). On Windows, where the
// pipes are synchronous, os.File.Close cancels a read in progress and then
// waits for the reader to let go; a reader that had not yet entered its read
// when the cancel came is not woken, and Close waits on it for as long as
// some process still holds the write end. Each reader closes its own end
// when its stream ends instead, and a stream that never ends is left to its
// reader.
func (b *buildRun) wait() {
	state, _ := b.cmd.Process.Wait()
	drained := time.After(drainWait)
	defect := ""
	select {
	case <-b.outDone:
	case <-drained:
		defect = fmt.Sprintf("the driver exited, but its stdout did not end within "+
			"%s: a process it started holds it", drainWait)
	}
	select {
	case <-b.errDone:
	case <-drained:
	}
	b.mu.Lock()
	b.state = state
	if defect != "" && b.defect == "" {
		b.defect = defect
	}
	b.mu.Unlock()
	close(b.exited)
}

// exitCode is the driver's exit code, after exited; -1 when unknown.
func (b *buildRun) exitCode() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.state.ExitCode()
}

// exitText says how the driver ended, waiting for drainWait at most.
func (b *buildRun) exitText() string {
	select {
	case <-b.exited:
		return fmt.Sprintf("exit %d", b.exitCode())
	case <-time.After(drainWait):
		return "still running"
	}
}

func (b *buildRun) send(m map[string]any) error {
	data, err := json.Marshal(m)
	if err != nil {
		return err
	}
	_, err = b.stdin.Write(append(data, '\n'))
	return err
}

// next is the driver's next message, or an error when its output ends or
// d passes first.
func (b *buildRun) next(d time.Duration) (map[string]any, error) {
	select {
	case m, ok := <-b.msgs:
		if !ok {
			return nil, fmt.Errorf("the driver's output ended (%s); its log:\n%s",
				b.exitText(), b.log())
		}
		return m, nil
	case <-time.After(d):
		return nil, fmt.Errorf("no message within %s; its log:\n%s", d, b.log())
	}
}

func (b *buildRun) log() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	s := b.errLog.String()
	if len(s) > 2000 {
		s = s[len(s)-2000:]
	}
	return s
}

// expect waits for a message of type want, failing on any other but the
// ones allowed in between.
func (b *buildRun) expect(want string, d time.Duration) (map[string]any, error) {
	for {
		m, err := b.next(d)
		if err != nil {
			return nil, fmt.Errorf("waiting for %s: %w", want, err)
		}
		switch m["type"] {
		case want:
			return m, nil
		case "failed":
			return nil, fmt.Errorf("waiting for %s, the driver failed: %v", want,
				m["error"])
		}
		if want == "stopped" || m["type"] == "exited" {
			continue
		}
		return nil, fmt.Errorf("waiting for %s, the driver sent %v", want, m["type"])
	}
}

// claim claims the relay relay-started reports, with the claim values it
// gives, and sends claimed (DRV-42, DRV-43).
func (b *buildRun) claim(started map[string]any) error {
	c, _ := started["claim"].(map[string]any)
	addr, _ := c["address"].(string)
	if network, _ := c["network"].(string); network != "tcp" || addr == "" {
		return fmt.Errorf("relay-started names no TCP claim endpoint: %v", c)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	conn, _, err := relaycfg.OpenTunnel(ctx, addr, b.token, b.col.Handler())
	if err != nil {
		return fmt.Errorf("opening the tunnel at %s: %w", addr, err)
	}
	b.l.mu.Lock()
	b.tunnel = conn
	b.l.mu.Unlock()
	cfg := relaycfg.Config{}
	if values, ok := started["claim_values"]; ok {
		data, _ := json.Marshal(values)
		_ = json.Unmarshal(data, &cfg)
	}
	cfg.CollectorURL, cfg.CollectorToken = relaycfg.TunnelURL, b.token
	if _, err := relaycfg.Claim(ctx, addr, b.token, cfg); err != nil {
		return fmt.Errorf("claiming the relay at %s: %w", addr, err)
	}
	return b.send(map[string]any{"type": "claimed"})
}

// upTo runs b to the point named: "relay" (relay-started), "environment"
// (environment-started) or "mid-build" (the marker in the build's output).
func (b *buildRun) upTo(point string) error {
	started, err := b.expect("relay-started", b.l.o.Teardown)
	if err != nil || point == "relay" {
		return err
	}
	if err := b.claim(started); err != nil {
		return err
	}
	if _, err := b.expect("environment-started", 5*time.Minute); err != nil {
		return err
	}
	if point == "environment" {
		return nil
	}
	deadline := time.Now().Add(5 * time.Minute)
	for time.Now().Before(deadline) {
		data, _ := os.ReadFile(filepath.Join(b.out, "build-output.log"))
		if bytes.Contains(data, []byte(marker)) {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("the build did not print its marker; its log:\n%s", b.log())
}

// complete waits for the build's completion.
func (b *buildRun) complete() (collector.CompletionSignal, error) {
	select {
	case sig := <-b.col.Done():
		return sig, nil
	case <-b.exited:
		return collector.CompletionSignal{}, fmt.Errorf("the driver exited before the "+
			"build completed; its log:\n%s", b.log())
	case <-time.After(10 * time.Minute):
		return collector.CompletionSignal{}, errors.New("no completion within 10 min")
	}
}

// stop sends stop with reason and waits for stopped and the driver's exit 0.
func (b *buildRun) stop(reason string) (map[string]any, error) {
	if err := b.send(map[string]any{"type": "stop", "reason": reason}); err != nil {
		return nil, err
	}
	_ = b.stdin.Close()
	m, err := b.expect("stopped", b.l.o.Teardown)
	if err != nil {
		return nil, err
	}
	return m, b.waitExit(0)
}

// waitExit waits for the driver's exit within the teardown wait, with code.
func (b *buildRun) waitExit(code int) error {
	select {
	case <-b.exited:
	case <-time.After(b.l.o.Teardown):
		_ = b.cmd.Process.Kill()
		return fmt.Errorf("the driver did not exit within %s", b.l.o.Teardown)
	}
	b.mu.Lock()
	defect := b.defect
	b.mu.Unlock()
	if defect != "" {
		return errors.New(defect)
	}
	if got := b.exitCode(); got != code {
		return fmt.Errorf("the driver exited %d, not %d; its log:\n%s", got, code, b.log())
	}
	return nil
}

// end releases what the suite holds of b: its input, tunnel, collector and
// lock; a driver still running is killed and reaped.
func (b *buildRun) end() {
	select {
	case <-b.exited:
	default:
		_ = b.cmd.Process.Kill()
		<-b.exited
		_, _ = b.l.reap(b)
	}
	b.release()
}

// release releases b's input, tunnel, collector and lock, once: end and
// killHeld may both come to it.
func (b *buildRun) release() {
	b.released.Do(func() {
		b.l.mu.Lock()
		tunnel := b.tunnel
		delete(b.l.held, b.id)
		b.l.mu.Unlock()
		if b.stdin != nil {
			_ = b.stdin.Close()
		}
		if tunnel != nil {
			_ = tunnel.Close()
		}
		if b.col != nil {
			_ = b.col.Close()
		}
		b.lock.release()
	})
}

// reap runs reap of b with its values, and returns how many it removed.
func (l *lifecycle) reap(b *buildRun) (int, error) {
	req := map[string]any{"contract": 1, "build_id": b.id, "tmpdir": l.s.tmp,
		"state_file": b.stateFile, "args": nonNil(l.o.DriverArgs)}
	if b.configFile != "" {
		req["config_file"] = b.configFile
	}
	data, _ := json.Marshal(req)
	o := l.s.run(call{command: "reap", argv: []string{b.id}, request: data,
		timeout: 2 * l.o.Teardown})
	if o.code != 0 {
		return 0, fmt.Errorf("reap exited %d: %s %s", o.code, o.errText(), o.stderr)
	}
	n, _ := o.result["removed"].(float64)
	return int(n), nil
}

// reapNothing reaps b and fails unless nothing was left.
func (l *lifecycle) reapNothing(b *buildRun) string {
	n, err := l.reap(b)
	switch {
	case err != nil:
		return err.Error()
	case n != 0:
		return fmt.Sprintf("reap after a clean end removed %d", n)
	}
	return l.leftIn(b)
}

// leftIn fails when the temporary directory the driver was given still
// holds something named with b's ID.
func (l *lifecycle) leftIn(b *buildRun) string {
	entries, _ := os.ReadDir(l.s.tmp)
	for _, e := range entries {
		if strings.Contains(e.Name(), b.id) {
			return fmt.Sprintf("%s is left in the temporary directory", e.Name())
		}
	}
	return ""
}

func (l *lifecycle) caseProbe() (string, string) {
	data, _ := json.Marshal(l.startMessage(nil, "build", false, true))
	o := l.s.run(call{command: "run", request: append(data, '\n')})
	if o.code != 0 {
		return "", fmt.Sprintf("the probe of run exited %d on this host: %s %s", o.code,
			o.errText(), o.stderr)
	}
	if made := l.s.created(); made != "" {
		return "", "the probe created " + made
	}
	return "", ""
}

// success runs a build of a script that exits code to its completion and
// stop, and returns it.
func (l *lifecycle) success(code int) (*buildRun, collector.CompletionSignal, string) {
	b, err := l.start(script(code, false), "build", false)
	if err != nil {
		return nil, collector.CompletionSignal{}, err.Error()
	}
	if err := b.upTo("environment"); err != nil {
		b.end()
		return nil, collector.CompletionSignal{}, err.Error()
	}
	sig, err := b.complete()
	if err != nil {
		b.end()
		return nil, sig, err.Error()
	}
	if _, err := b.stop("completed"); err != nil {
		b.end()
		return nil, sig, err.Error()
	}
	b.end()
	return b, sig, ""
}

func (l *lifecycle) caseSequence() (string, string) {
	b, _, failure := l.success(0)
	if failure != "" {
		return "", failure
	}
	b.mu.Lock()
	lines := slices.Clone(b.lines)
	b.mu.Unlock()
	var types []string
	for _, line := range lines {
		var m map[string]any
		_ = json.Unmarshal([]byte(line), &m)
		if t, _ := m["type"].(string); t != "exited" {
			types = append(types, t)
		}
	}
	if strings.Join(types, " ") != "relay-started environment-started stopped" {
		return "", fmt.Sprintf("the messages were %v, want relay-started, "+
			"environment-started, stopped", types)
	}
	return "", ""
}

func (l *lifecycle) caseSuccess() (string, string) {
	b, sig, failure := l.success(0)
	if failure != "" {
		return "", failure
	}
	out, _ := os.ReadFile(filepath.Join(b.out, "build-output.log"))
	switch {
	case sig.ExitCode != 0:
		return "", fmt.Sprintf("the completion carries %d, not 0", sig.ExitCode)
	case !bytes.Contains(out, []byte(marker)):
		return "", "the collector did not receive the script's marker"
	}
	if l.o.StubRelay {
		return "", ""
	}
	return "", verifyLedger(b.out)
}

// verifyLedger checks that out holds a ledger whose signatures verify.
func verifyLedger(out string) string {
	data, err := os.ReadFile(filepath.Join(out, "ledger"))
	if err != nil {
		return "no ledger was delivered: " + err.Error()
	}
	res, err := ledger.Verify(data)
	if err != nil || res.SigErrors != 0 {
		return fmt.Sprintf("the ledger does not verify: %v (%d signature errors)", err,
			sigErrors(res))
	}
	return ""
}

func sigErrors(r *ledger.VerifyResult) int {
	if r == nil {
		return 0
	}
	return r.SigErrors
}

func (l *lifecycle) caseInputs() (string, string) {
	if l.o.StubRelay {
		return "needs a ledger, which stubrelay does not write", ""
	}
	b, _, failure := l.success(0)
	if failure != "" {
		return "", failure
	}
	data, _ := os.ReadFile(filepath.Join(b.out, "ledger"))
	if !bytes.Contains(data, []byte(inputFile)) {
		return "", "the ledger does not record " + inputFile + ", read from the context"
	}
	return "", ""
}

func (l *lifecycle) caseFailure() (string, string) {
	b, sig, failure := l.success(3)
	if failure != "" {
		return "", failure
	}
	if sig.ExitCode != 3 {
		return "", fmt.Sprintf("the completion carries %d, not 3", sig.ExitCode)
	}
	if l.o.StubRelay {
		return "", ""
	}
	return "", verifyLedger(b.out)
}

// points are where Stop, End of input, Orphaned and Kill end a build.
var points = []string{"relay", "environment", "mid-build"}

// atEachPoint runs end on a build brought to each point, and reaps it after.
func (l *lifecycle) atEachPoint(end func(b *buildRun) string) string {
	for _, p := range points {
		b, err := l.start(script(0, true), "build", false)
		if err != nil {
			return err.Error()
		}
		failure := ""
		if err := b.upTo(p); err != nil {
			failure = err.Error()
		} else {
			failure = end(b)
		}
		b.end()
		if failure != "" {
			return "after " + p + ": " + failure
		}
	}
	return ""
}

func (l *lifecycle) caseStop() (string, string) {
	return "", l.atEachPoint(func(b *buildRun) string {
		if _, err := b.stop("interrupted"); err != nil {
			return err.Error()
		}
		return l.reapNothing(b)
	})
}

func (l *lifecycle) caseEndOfInput() (string, string) {
	return "", l.atEachPoint(func(b *buildRun) string {
		_ = b.stdin.Close()
		if _, err := b.expect("stopped", b.l.o.Teardown); err != nil {
			return err.Error()
		}
		if err := b.waitExit(0); err != nil {
			return err.Error()
		}
		return l.reapNothing(b)
	})
}

// caseOrphaned ends a build at each point as a killed orchestrator does:
// its output closed, then its input.
//
// Its limit, on Windows only: a close of stdout or stderr that missed its
// cancel (closeOutput) ends only on the driver's next write to that stream
// or its exit. That write succeeds, the suite's pending read taking it, and
// only the driver's second write meets the closed stream. So when the race
// hits, a driver that writes to a stream once during teardown and then
// dies on the failed write passes there; one that writes twice or more, as
// the fake's die-on-closed-stderr fault does, is caught. settle makes the
// race rare; on POSIX the close is exact. The kit's handling of a closed
// stream is the same code on every platform, so the Linux and macOS runs
// check it exactly, and native Windows validation (container-any) runs the
// case against the real driver there. Waiting for both closes before the
// input closes would deadlock against a driver that waits for the end of
// its input before it writes.
func (l *lifecycle) caseOrphaned() (string, string) {
	return "", l.atEachPoint(func(b *buildRun) string {
		// Output first: a driver that tears down on the end of its input
		// meets its closed output, as when warden was killed.
		b.closeOutput()
		_ = b.stdin.Close()
		select {
		case <-b.exited:
		case <-time.After(b.l.o.Teardown):
			return fmt.Sprintf("the driver did not exit within %s of its streams "+
				"closing", b.l.o.Teardown)
		}
		return l.reapNothing(b)
	})
}

// Bounds on closeOutput: settle lets the readers block in their reads first,
// and closeWait is how long a close is waited on before the input is closed
// all the same.
const (
	settle    = 200 * time.Millisecond
	closeWait = 2 * time.Second
)

// closeOutput closes the suite's ends of the driver's stdout and stderr, as a
// killed orchestrator's are closed, while read and the stderr copy read them.
//
// On POSIX the close wakes the readers. On Windows the pipes are synchronous:
// os.File.Close cancels a ReadFile in progress (CancelIoEx) and then waits
// for the reader to let go of the file. A reader that was about to enter its
// read when the cancel came, as one is just after the message that brought
// the build to its point, misses it; its read then ends only when the driver
// writes or exits, so a Close waited on before the input closes waits for a
// driver that is waiting for the end of its input. So the readers are first
// given settle to enter their reads, where the cancel reaches them, and each
// close runs on its own and is waited on for closeWait at most. A close that
// missed still ends once the driver's teardown writes to the stream or the
// driver exits: the driver then meets its closed output on its next write
// (caseOrphaned states the limit this leaves on Windows).
func (b *buildRun) closeOutput() {
	time.Sleep(settle)
	var wg sync.WaitGroup
	for _, f := range []io.Closer{b.stdout, b.stderr} {
		wg.Go(func() { _ = f.Close() })
	}
	closed := make(chan struct{})
	go func() {
		wg.Wait()
		close(closed)
	}()
	select {
	case <-closed:
	case <-time.After(closeWait):
	}
}

func (l *lifecycle) caseKill() (string, string) {
	variants := []string{""}
	if l.o.ConfigFile != "" {
		variants = append(variants, "deleted", "not TOML")
	}
	for _, v := range variants {
		failure := l.atEachPoint(func(b *buildRun) string {
			_ = b.cmd.Process.Kill()
			<-b.exited
			switch v {
			case "deleted":
				_ = os.Remove(b.configFile)
			case "not TOML":
				_ = os.WriteFile(b.configFile, []byte("not toml ["), 0o600)
			}
			if _, err := l.reap(b); err != nil {
				return err.Error()
			}
			if n, err := l.reap(b); err != nil || n != 0 {
				return fmt.Sprintf("a second reap removed %d (%v)", n, err)
			}
			return l.leftIn(b)
		})
		if failure != "" {
			if v != "" {
				failure = "with the config file " + v + ": " + failure
			}
			return "", failure
		}
	}
	return "", ""
}

func (l *lifecycle) caseReap() (string, string) {
	b := &buildRun{id: newID(), stateFile: filepath.Join(l.s.scratch, "none.state")}
	for range 2 {
		if n, err := l.reap(b); err != nil || n != 0 {
			return "", fmt.Sprintf("reap of an ID that never existed: removed %d "+
				"(%v)", n, err)
		}
	}
	return "", ""
}

func (l *lifecycle) caseDiagnostics() (string, string) {
	b, _, failure := l.success(0)
	if failure != "" {
		return "", failure
	}
	entries, _ := os.ReadDir(b.diag)
	for _, e := range entries {
		if e.Name() != "relay.log" && e.Name() != "console.log" {
			return "", "diagnostics_dir holds " + e.Name()
		}
	}
	return "", ""
}

func (l *lifecycle) caseTwoAtOnce() (string, string) {
	failures := make(chan string, 2)
	for range 2 {
		go func() {
			_, sig, failure := l.success(0)
			if failure == "" && sig.ExitCode != 0 {
				failure = fmt.Sprintf("completion %d", sig.ExitCode)
			}
			failures <- failure
		}()
	}
	for range 2 {
		if f := <-failures; f != "" {
			return "", f
		}
	}
	return "", ""
}

// heldIDs are the IDs of the builds of the driver whose locks are held in
// this user's lock directory, the suite's own among them (DRV-58).
func (l *lifecycle) heldIDs() []string {
	ids := heldLocks(l.s.name)
	l.mu.Lock()
	for id := range l.held {
		if !slices.Contains(ids, id) {
			ids = append(ids, id)
		}
	}
	l.mu.Unlock()
	return ids
}

// caseClean runs two builds to mid-build and kills the driver of one alone;
// clean, given the other's lock in keep_ids, must remove the killed build
// and leave the running one (DRV-27, DRV-58).
func (l *lifecycle) caseClean() (string, string) {
	if !l.s.lists("clean") {
		return "clean is not listed", ""
	}
	live, err := l.start(script(0, true), "build", false)
	if err != nil {
		return "", err.Error()
	}
	defer live.end()
	dead, err := l.start(script(0, true), "build", false)
	if err != nil {
		return "", err.Error()
	}
	for _, b := range []*buildRun{live, dead} {
		if err := b.upTo("mid-build"); err != nil {
			dead.end()
			return "", err.Error()
		}
	}
	// The killed build's driver is killed alone (SIGKILL to its process,
	// never its group), as a crash kills it: a child it started, such as the
	// container driver's waiter, is orphaned and may still run when clean
	// runs, and is no sign that the build lives (DRV-27).
	_ = dead.cmd.Process.Kill()
	<-dead.exited
	dead.lock.release()
	l.mu.Lock()
	delete(l.held, dead.id)
	l.mu.Unlock()
	req, _ := json.Marshal(map[string]any{"contract": 1, "keep_ids": l.heldIDs(),
		"cache": false, "tmpdir": l.s.tmp, "args": nonNil(l.o.DriverArgs),
		"config_file": l.o.ConfigFile})
	o := l.s.run(call{command: "clean", request: req, timeout: 2 * l.o.Teardown})
	dead.end()
	if o.code != 0 {
		return "", fmt.Sprintf("clean exited %d: %s", o.code, o.errText())
	}
	if n, err := l.reap(dead); err != nil || n != 0 {
		return "", fmt.Sprintf("clean left the killed build: its reap removed %d (%v); "+
			"only its driver was killed, so a process the driver started may "+
			"still run: is a process whose driver is gone taken for a sign of life?",
			n, err)
	}
	if _, err := live.stop("interrupted"); err != nil {
		return "", "the running build, after clean: " + err.Error()
	}
	return "", l.reapNothing(live)
}

func (l *lifecycle) caseKeep() (string, string) {
	if l.s.desc.Capabilities.Keep == "none" || l.s.desc.Capabilities.Keep == "" {
		return "capabilities.keep is none", ""
	}
	b, err := l.start(script(0, false), "build", true)
	if err != nil {
		return "", err.Error()
	}
	defer b.end()
	if err := b.upTo("environment"); err != nil {
		return "", err.Error()
	}
	if _, err := b.complete(); err != nil {
		return "", err.Error()
	}
	m, err := b.stop("completed")
	if err != nil {
		return "", err.Error()
	}
	kept, _ := m["kept"].([]any)
	if len(kept) == 0 {
		return "", "stopped lists nothing under kept"
	}
	for _, k := range kept {
		if p, _ := k.(string); filepath.IsAbs(p) {
			if _, err := os.Stat(p); err != nil {
				return "", fmt.Sprintf("kept %s does not exist", p)
			}
		}
	}
	if n, err := l.reap(b); err != nil || n == 0 {
		return "", fmt.Sprintf("reap of a kept build removed %d (%v)", n, err)
	}
	return "", ""
}

func (l *lifecycle) caseShell() (string, string) {
	if !l.s.desc.Capabilities.Shell {
		return "capabilities.shell is false", ""
	}
	unknown, _ := json.Marshal(map[string]any{"contract": 1, "build_id": newID(),
		"state_file": filepath.Join(l.s.scratch, "none.state"),
		"args":       nonNil(l.o.DriverArgs)})
	o := l.s.run(call{command: "attach", request: unknown})
	if o.errCode() == "" {
		return "", fmt.Sprintf("attach of an ID that does not exist wrote no error "+
			"(exit %d)", o.code)
	}
	b, err := l.start(script(0, true), "shell", false)
	if err != nil {
		return "", err.Error()
	}
	defer b.end()
	if err := b.upTo("environment"); err != nil {
		return "", err.Error()
	}
	req, _ := json.Marshal(map[string]any{"contract": 1, "build_id": b.id,
		"state_file": b.stateFile, "args": nonNil(l.o.DriverArgs),
		"config_file": b.configFile})
	a := l.s.attach(req, "exit 7\n")
	if a.code != 7 || a.errCode() != "" {
		return "", fmt.Sprintf("attach running exit 7 exited %d with %v: %s", a.code,
			a.result, a.stderr)
	}
	if _, err := b.stop("shell-exited"); err != nil {
		return "", err.Error()
	}
	return "", ""
}

// attach runs attach with req in its request file and input on its
// standard input, which is not a terminal; a driver that needs one is run
// with --skip shell.
func (s *Suite) attach(req []byte, input string) *outcome {
	reqFile := filepath.Join(s.scratch, "attach-"+newID()+".json")
	if err := os.WriteFile(reqFile, req, 0o600); err != nil {
		return &outcome{code: -1, resultErr: err.Error()}
	}
	result := filepath.Join(s.scratch, "attach-"+newID()+".result")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, s.path, "attach")
	cmd.Env = append(s.environ(), "WARDEN_DRIVER_REQUEST="+reqFile,
		"WARDEN_DRIVER_RESULT="+result)
	cmd.Stdin = strings.NewReader(input)
	// A process attach starts may hold its stderr past its exit; Wait gives
	// up on it after this, rather than waiting for it to end.
	cmd.WaitDelay = 2 * time.Second
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	err := cmd.Run()
	o := &outcome{stderr: stderr.Bytes()}
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		o.code = exit.ExitCode()
	} else if err != nil {
		o.code = -1
	}
	if data, err := os.ReadFile(result); err == nil {
		_ = json.Unmarshal(data, &o.result)
	}
	return o
}

func (l *lifecycle) caseExited() (string, string) {
	if !l.s.desc.Capabilities.Exited {
		return "capabilities.exited is false", ""
	}
	body := l.o.ExitScript
	if body == "" {
		if l.o.StubRelay {
			return "stubrelay plays no environment that can end", ""
		}
		body = "#!/bin/sh\nkill -9 1\npoweroff -f\n"
	}
	b, err := l.start(body, "build", false)
	if err != nil {
		return "", err.Error()
	}
	defer b.end()
	if err := b.upTo("environment"); err != nil {
		return "", err.Error()
	}
	for {
		m, err := b.next(5 * time.Minute)
		if err != nil {
			return "", "no exited message: " + err.Error()
		}
		if m["type"] == "exited" && m["which"] == "build" {
			break
		}
	}
	if _, err := b.stop("failed"); err != nil {
		return "", err.Error()
	}
	return "", ""
}

func (l *lifecycle) caseDoctor() (string, string) {
	if !l.s.lists("doctor") {
		return "doctor is not listed", ""
	}
	req := map[string]any{"contract": 1, "args": nonNil(l.o.DriverArgs), "verbose": false}
	if l.o.ConfigFile != "" {
		req["config_file"] = l.o.ConfigFile
	}
	if l.s.desc.Capabilities.VM {
		req["vm"] = map[string]any{}
	}
	data, _ := json.Marshal(req)
	o := l.s.run(call{command: "doctor", request: data})
	if blocked, ok := o.result["blocked"].(bool); o.code != 0 || !ok || blocked {
		return "", fmt.Sprintf("doctor exited %d with %v on this host", o.code, o.result)
	}
	return "", ""
}
