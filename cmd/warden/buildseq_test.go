package main

import (
	"context"
	"errors"
	"os"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/buildwarden/buildwarden/driver"
)

// The build sequence against a fake driver: a linked driver whose Run plays
// the driver's side of the dialogue through the driver.Host it is given, and
// a fake collector session. Each test checks the order of the messages, the
// stop warden sends and how the build ends (DRV-45 to DRV-54).

// trace records the messages of one build in the order they happened.
type trace struct {
	mu    sync.Mutex
	lines []string
}

func (tr *trace) add(s string) {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	tr.lines = append(tr.lines, s)
}

func (tr *trace) get() []string {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	return slices.Clone(tr.lines)
}

// fakeSession is a collector session whose claim and completion the test
// controls.
type fakeSession struct {
	tr       *trace
	claimErr error
	// claimHold makes the claim wait until its context ends.
	claimHold bool
	claiming  chan struct{}
	done      chan error // the build's result, sent by the test

	mu     sync.Mutex
	res    error
	hasRes bool
	claims []driver.RelayStarted
	// waited reports that warden waited for a completion.
	waited atomic.Bool
}

func newFakeSession(tr *trace) *fakeSession {
	return &fakeSession{tr: tr, claiming: make(chan struct{}, 1), done: make(chan error, 1)}
}

func (f *fakeSession) claim(ctx context.Context, r driver.RelayStarted) error {
	f.mu.Lock()
	f.claims = append(f.claims, r)
	f.mu.Unlock()
	f.tr.add("claim " + r.Claim.Address)
	f.claiming <- struct{}{}
	if f.claimHold {
		<-ctx.Done()
		return ctx.Err()
	}
	return f.claimErr
}

func (f *fakeSession) wait(ctx context.Context) error {
	f.waited.Store(true)
	select {
	case err := <-f.done:
		f.complete(err)
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// complete puts a completion in the collector, as a guest's would.
func (f *fakeSession) complete(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.res, f.hasRes = err, true
}

func (f *fakeSession) result() (error, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.res, f.hasRes
}

func (f *fakeSession) close() {}

// fakeClock hands out timers the test fires.
type fakeClock struct {
	mu     sync.Mutex
	timers map[time.Duration]chan time.Time
	asked  chan time.Duration
}

func newFakeClock() *fakeClock {
	return &fakeClock{timers: map[time.Duration]chan time.Time{},
		asked: make(chan time.Duration, 4)}
}

func (c *fakeClock) after(d time.Duration) <-chan time.Time {
	c.mu.Lock()
	ch := make(chan time.Time, 1)
	c.timers[d] = ch
	c.mu.Unlock()
	c.asked <- d
	return ch
}

func (c *fakeClock) fire(t *testing.T, d time.Duration) {
	t.Helper()
	select {
	case got := <-c.asked:
		if got != d {
			t.Fatalf("timer of %s asked for, want %s", got, d)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("no timer of %s was asked for", d)
	}
	c.mu.Lock()
	ch := c.timers[d]
	c.mu.Unlock()
	ch <- time.Now()
}

// fakeRun is the driver side of a test: it is given the host and the start
// message and plays its part, logging what it does to the trace.
type fakeRun func(tr *trace, st *driver.Start, h driver.Host) ([]string, error)

// fakeRelay is the relay a fake driver reports.
var fakeRelay = driver.RelayStarted{
	Claim:   driver.ClaimEndpoint{Network: driver.ClaimTCP, Address: "127.0.0.1:41233"},
	GuestOS: "linux", OldRelayFix: "reinstall warden",
}

// wellBehaved is a driver that does everything in order.
func wellBehaved(tr *trace, _ *driver.Start, h driver.Host) ([]string, error) {
	tr.add("relay-started")
	if err := h.ClaimRelay(context.Background(), fakeRelay); err != nil {
		tr.add("claim refused: " + err.Error())
		tr.add("stopped")
		return nil, err
	}
	tr.add("claimed")
	_ = h.Started()
	tr.add("environment-started")
	tr.add("stop " + h.Wait(context.Background()))
	tr.add("stopped")
	return nil, nil
}

// sequenceTest is one build: its driver, session, clock and signals.
type sequenceTest struct {
	tr      *trace
	sess    *fakeSession
	clock   *fakeClock
	signals chan os.Signal
	stopped bool // stopSignals was called
	seq     *buildSequence
	out     chan buildOutcome
}

func newSequenceTest(run fakeRun, timeout time.Duration) *sequenceTest {
	tr := &trace{}
	st := &sequenceTest{tr: tr, sess: newFakeSession(tr), clock: newFakeClock(),
		signals: make(chan os.Signal, 1), out: make(chan buildOutcome, 1)}
	d := &linkedDriver{Commands: []string{"run"}, Run: func(_ context.Context, _ *Config,
		_ *driver.BuildRequest,
		start *driver.Start, h driver.Host) ([]string, error) {
		return run(tr, start, h)
	}}
	st.seq = &buildSequence{
		start: &driver.Start{Contract: 1, BuildID: "k3v9x2ab", Mode: driver.ModeBuild,
			ClaimToken: "token"},
		peer:    newLinkedPeer(d, "fake", &Config{}, &driver.BuildRequest{}),
		session: st.sess,
		timeout: timeout, signals: st.signals, after: st.clock.after,
		stopSignals: func() { st.stopped = true },
	}
	return st
}

func (s *sequenceTest) begin() { go func() { s.out <- s.seq.run() }() }

func (s *sequenceTest) end(t *testing.T) buildOutcome {
	t.Helper()
	select {
	case out := <-s.out:
		return out
	case <-time.After(5 * time.Second):
		t.Fatalf("the sequence did not end; trace %v", s.tr.get())
	}
	return buildOutcome{}
}

func (s *sequenceTest) claimStarted(t *testing.T) {
	t.Helper()
	select {
	case <-s.sess.claiming:
	case <-time.After(5 * time.Second):
		t.Fatalf("no claim was made; trace %v", s.tr.get())
	}
}

// waitFor waits until the trace holds line.
func (s *sequenceTest) waitFor(t *testing.T, line string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if slices.Contains(s.tr.get(), line) {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("trace %v never held %q", s.tr.get(), line)
}

func wantTrace(t *testing.T, got []string, want ...string) {
	t.Helper()
	if !slices.Equal(got, want) {
		t.Errorf("trace\n  %s\nwant\n  %s", strings.Join(got, "\n  "),
			strings.Join(want, "\n  "))
	}
}

// A build in order: relay-started, the claim at the endpoint the driver
// reported, claimed, environment-started, then stop with completed once the
// collector has the completion, and stopped.
func TestBuildSequenceOrder(t *testing.T) {
	for _, tc := range []struct {
		name   string
		result error
	}{
		{"exit 0", nil},
		{"exit 3", &driver.ExitError{Code: 3}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newSequenceTest(wellBehaved, 0)
			s.begin()
			s.waitFor(t, "environment-started")
			s.sess.done <- tc.result
			out := s.end(t)
			wantTrace(t, s.tr.get(), traceOf("stop completed", "stopped")...)
			if !errors.Is(out.err, tc.result) || !out.claimed {
				t.Errorf("outcome %+v, want the build's result %v", out, tc.result)
			}
		})
	}
}

// Every way warden stops a build sends stop with its reason, and the driver
// tears down: a failed build, --timeout, SIGINT and SIGTERM.
func TestBuildSequenceStop(t *testing.T) {
	for _, tc := range []struct {
		name   string
		act    func(t *testing.T, s *sequenceTest)
		reason string
		check  func(out buildOutcome) bool
	}{
		{"the build failed", func(t *testing.T, s *sequenceTest) {
			s.sess.done <- errors.New("the guest stopped heartbeating")
		}, "failed", func(o buildOutcome) bool { return o.err != nil }},
		{"timeout", func(t *testing.T, s *sequenceTest) {
			s.clock.fire(t, 5*time.Minute)
		}, "timeout", func(o buildOutcome) bool { return o.timedOut }},
		{"SIGINT", func(t *testing.T, s *sequenceTest) {
			s.signals <- os.Interrupt
		}, "interrupted", func(o buildOutcome) bool { return o.interrupted }},
		{"SIGTERM", func(t *testing.T, s *sequenceTest) {
			s.signals <- syscall.SIGTERM
		}, "interrupted", func(o buildOutcome) bool { return o.terminated }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newSequenceTest(wellBehaved, 5*time.Minute)
			s.begin()
			s.waitFor(t, "environment-started")
			if tc.name != "timeout" {
				<-s.clock.asked // the timeout's timer, never fired
			}
			tc.act(t, s)
			out := s.end(t)
			wantTrace(t, s.tr.get(), traceOf("stop "+tc.reason, "stopped")...)
			if !tc.check(out) {
				t.Errorf("outcome %+v", out)
			}
			if signal := strings.HasPrefix(tc.name, "SIG"); signal != s.stopped {
				t.Errorf("signals stopped: %v, want %v", s.stopped, signal)
			}
		})
	}
}

// A signal before the relay is up stops the build there: the driver's claim
// is refused and nothing more is started.
func TestBuildSequenceSignalBeforeTheRelay(t *testing.T) {
	gate := make(chan struct{})
	s := newSequenceTest(func(tr *trace, st *driver.Start, h driver.Host) ([]string, error) {
		<-gate
		return wellBehaved(tr, st, h)
	}, 0)
	s.begin()
	s.signals <- os.Interrupt
	time.Sleep(10 * time.Millisecond)
	close(gate)
	out := s.end(t)
	wantTrace(t, s.tr.get(), "relay-started", "claim refused: the build was stopped",
		"stopped")
	if !out.interrupted || out.claimed {
		t.Errorf("outcome %+v, want interrupted with no claim", out)
	}
}

// run is a fake driver's Run in the tests' short form.
type run = fakeRun

// upTo is the trace of a well-behaved build up to its environment, at the
// fake relay's address.
var upTo = []string{"relay-started", "claim 127.0.0.1:41233", "claimed",
	"environment-started"}

func traceOf(lines ...string) []string { return append(slices.Clone(upTo), lines...) }

// Failure table (DRV-54): the driver refuses the build before its relay is
// up. warden makes no claim, and reports the driver's error.
func TestBuildSequenceDriverRefuses(t *testing.T) {
	refusal := driver.Fixable("install qemu", "qemu-system not found")
	s := newSequenceTest(func(tr *trace, _ *driver.Start, _ driver.Host) ([]string,
		error) {
		tr.add("failed")
		return nil, refusal
	}, 0)
	s.begin()
	out := s.end(t)
	wantTrace(t, s.tr.get(), "failed")
	if !errors.Is(out.err, refusal) || out.claimed || out.teardownErr != nil {
		t.Errorf("outcome %+v, want the refusal and no claim", out)
	}
}

// Failure table: the claim fails. The driver's ClaimRelay returns the claim's
// error, with its fix, and the build fails with it.
func TestBuildSequenceClaimFails(t *testing.T) {
	s := newSequenceTest(wellBehaved, 0)
	s.sess.claimErr = driver.Fixable("reinstall warden", "the relay predates a field")
	s.begin()
	out := s.end(t)
	wantTrace(t, s.tr.get(), "relay-started", "claim 127.0.0.1:41233",
		"claim refused: the relay predates a field", "stopped")
	var fe *driver.FixableError
	if !errors.As(out.err, &fe) || fe.Fix != "reinstall warden" || out.teardownErr != nil {
		t.Errorf("outcome %+v, want the claim's error with its fix", out)
	}
}

// Failure table: the relay exits during the claim, which cancels it.
func TestBuildSequenceRelayExitsDuringTheClaim(t *testing.T) {
	var s *sequenceTest
	var exits run = func(tr *trace, st *driver.Start, h driver.Host) ([]string, error) {
		go func() {
			<-s.sess.claiming
			h.Exited(driver.ExitedRelay, "exit status 1")
		}()
		return wellBehaved(tr, st, h)
	}
	s = newSequenceTest(exits, 0)
	s.sess.claimHold = true
	s.begin()
	out := s.end(t)
	const why = "the relay exited before it was claimed: exit status 1"
	wantTrace(t, s.tr.get(), "relay-started", "claim 127.0.0.1:41233",
		"claim refused: "+why, "stopped")
	if out.err == nil || out.err.Error() != why || out.teardownErr != nil {
		t.Errorf("outcome %+v, want the relay's exit", out)
	}
}

// Failure table: the relay exits after the claim, which fails the build at
// once, with no grace.
func TestBuildSequenceRelayExitsAfterTheClaim(t *testing.T) {
	exit := make(chan struct{})
	var exits run = func(tr *trace, st *driver.Start, h driver.Host) ([]string, error) {
		go func() { <-exit; h.Exited(driver.ExitedRelay, "killed") }()
		return wellBehaved(tr, st, h)
	}
	s := newSequenceTest(exits, 0)
	s.begin()
	s.waitFor(t, "environment-started")
	close(exit)
	out := s.end(t)
	wantTrace(t, s.tr.get(), traceOf("stop failed", "stopped")...)
	if out.err == nil || out.err.Error() != "the relay exited: killed" {
		t.Errorf("outcome %+v, want the relay's exit at once", out)
	}
}

// Failure table: the environment cannot be started; the driver's error is
// the build's.
func TestBuildSequenceEnvironmentCannotStart(t *testing.T) {
	noEnv := errors.New("the build VM did not boot")
	var fails run = func(tr *trace, _ *driver.Start, h driver.Host) ([]string, error) {
		if err := h.ClaimRelay(context.Background(), fakeRelay); err != nil {
			return nil, err
		}
		tr.add("failed")
		return nil, noEnv
	}
	s := newSequenceTest(fails, 0)
	s.begin()
	if out := s.end(t); !errors.Is(out.err, noEnv) {
		t.Errorf("outcome %+v, want the driver's error", out)
	}
}

// A driver that reports its environment before the claim broke the dialogue
// (DRV-46): a driver defect, and warden stops it.
func TestBuildSequenceStartedBeforeTheClaim(t *testing.T) {
	var early run = func(tr *trace, _ *driver.Start, h driver.Host) ([]string, error) {
		_ = h.Started()
		tr.add("stop " + h.Wait(context.Background()))
		return nil, nil
	}
	s := newSequenceTest(early, 0)
	s.begin()
	out := s.end(t)
	wantTrace(t, s.tr.get(), "stop failed")
	if out.err == nil || !strings.Contains(out.err.Error(), "driver defect") {
		t.Errorf("outcome %+v, want a driver defect", out)
	}
}

// A driver that ends the build before warden asked it to: a driver defect,
// unless the collector already holds the completion, which then stands.
func TestBuildSequenceDriverEndsUnasked(t *testing.T) {
	var ends run = func(_ *trace, _ *driver.Start, h driver.Host) ([]string, error) {
		if err := h.ClaimRelay(context.Background(), fakeRelay); err != nil {
			return nil, err
		}
		_ = h.Started()
		return nil, nil
	}
	s := newSequenceTest(ends, 0)
	s.begin()
	if out := s.end(t); out.err == nil || !strings.Contains(out.err.Error(), "driver defect") {
		t.Errorf("without a completion: outcome %+v, want a driver defect", out)
	}
	s = newSequenceTest(ends, 0)
	s.sess.complete(&driver.ExitError{Code: 7})
	s.begin()
	var ee *driver.ExitError
	if out := s.end(t); !errors.As(out.err, &ee) || ee.Code != 7 {
		t.Errorf("with the completion in: outcome %+v, want its code 7", out)
	}
}

// An environment that exits before the build completes is given 10 s for a
// completion on its way, then the build fails; a completion within the grace
// is the build's result.
func TestBuildSequenceExitGrace(t *testing.T) {
	for _, inTime := range []bool{false, true} {
		exit := make(chan struct{})
		var exits run = func(tr *trace, st *driver.Start, h driver.Host) ([]string,
			error) {
			go func() {
				<-exit
				h.Exited(driver.ExitedBuild, "the build VM powered off")
			}()
			return wellBehaved(tr, st, h)
		}
		s := newSequenceTest(exits, 0)
		s.begin()
		s.waitFor(t, "environment-started")
		close(exit)
		if inTime {
			select {
			case d := <-s.clock.asked:
				if d != buildExitGrace {
					t.Fatalf("timer of %s, want the grace", d)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("no grace was given")
			}
			s.sess.done <- nil
		} else {
			s.clock.fire(t, 10*time.Second)
		}
		out := s.end(t)
		switch {
		case inTime && out.err != nil:
			t.Errorf("a completion within the grace: outcome %+v, want success", out)
		case !inTime && (out.err == nil || !strings.Contains(out.err.Error(),
			"exited before the build completed: the build VM powered off")):
			t.Errorf("after the grace: outcome %+v, want the environment's exit", out)
		}
		want := "stop completed"
		if !inTime {
			want = "stop failed"
		}
		wantTrace(t, s.tr.get(), traceOf(want, "stopped")...)
	}
}

// The exit ordering firecracker's waitBuild had (its Decision 8), now the
// sequence's for every driver: a non-zero completion within the grace is the
// build's result, and a relay that exits during the grace fails the build at
// once, without waiting for the grace to end.
func TestBuildSequenceExitOrdering(t *testing.T) {
	for _, tc := range []struct {
		name string
		// during acts once the grace has started.
		during  func(s *sequenceTest, relayExit chan<- struct{})
		want    string // the outcome's error, or "exit 3"
		wantLog string // the stop warden sent
	}{
		{"non-zero exit just after the environment exits",
			func(s *sequenceTest, _ chan<- struct{}) {
				s.sess.done <- &driver.ExitError{Code: 3}
			}, "exit 3", "stop completed"},
		{"relay exits during the grace",
			func(_ *sequenceTest, relayExit chan<- struct{}) { close(relayExit) },
			"the relay exited: killed", "stop failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			buildExit, relayExit := make(chan struct{}), make(chan struct{})
			var exits run = func(tr *trace, st *driver.Start, h driver.Host) ([]string,
				error) {
				go func() {
					<-buildExit
					h.Exited(driver.ExitedBuild, "the build VM powered off")
					<-relayExit
					h.Exited(driver.ExitedRelay, "killed")
				}()
				return wellBehaved(tr, st, h)
			}
			s := newSequenceTest(exits, 0)
			s.begin()
			s.waitFor(t, "environment-started")
			close(buildExit)
			select {
			case d := <-s.clock.asked:
				if d != buildExitGrace {
					t.Fatalf("timer of %s, want the grace", d)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("no grace was given")
			}
			tc.during(s, relayExit)
			out := s.end(t)
			var ee *driver.ExitError
			switch {
			case tc.want == "exit 3":
				if !errors.As(out.err, &ee) || ee.Code != 3 {
					t.Errorf("outcome %+v, want exit 3", out)
				}
			case out.err == nil || out.err.Error() != tc.want:
				t.Errorf("outcome %+v, want %q", out, tc.want)
			}
			wantTrace(t, s.tr.get(), traceOf(tc.wantLog, "stopped")...)
		})
	}
}

// The start message of a VM driver carries the shared VM settings; another
// driver's does not, and a probe has no build ID, token, script, output or
// state file (DRV-32, DRV-40).
func TestNewStart(t *testing.T) {
	cfg := &Config{}
	req := &driver.BuildRequest{ContextDir: "/c", OutputDir: "out", CPUs: 8,
		BuildScript: "/t/warden-stage-k3v9x2ab/build.sh", KeepVMs: true}
	vm := newStart(cfg, req, linkedDrivers["qemu"].Capabilities, "k3v9x2ab", "tok", "/s")
	if vm.VM == nil || vm.VM.CPUs != 8 || vm.GuestOS != "linux" || !vm.Keep ||
		vm.DiagnosticsDir == "" || vm.ClaimToken != "tok" || vm.StateFile != "/s" {
		t.Errorf("qemu's start %+v", vm)
	}
	ctr := newStart(cfg, req, capabilities{Shell: true, Keep: keepNone, Exited: true,
		Containerfile: containerfileTranslate, GuestOS: []string{"linux"}}, "k3v9x2ab",
		"tok", "/s")
	if ctr.VM != nil || ctr.Keep {
		t.Errorf("the container driver's start %+v carries the VM settings or keep", ctr)
	}
	probe := newStart(cfg, &driver.BuildRequest{ContextDir: "/c"},
		linkedDrivers["qemu"].Capabilities, "", "", "")
	if probe.BuildID != "" || probe.ClaimToken != "" || probe.BuildScript != "" ||
		probe.DiagnosticsDir != "" || probe.StateFile != "" {
		t.Errorf("a probe's start %+v", probe)
	}
}

// warden's exit for each way a build through the sequence ends (UX-09): the
// build's code, 124 for --timeout, 130 for Ctrl-C, 1 for SIGTERM and for any
// other failure.
func TestReportBuildExitCodes(t *testing.T) {
	captureLog(t)
	failed := errors.New("the guest stopped heartbeating")
	for _, tc := range []struct {
		name string
		out  buildOutcome
		want int // -1 is no exit status error
	}{
		{"success", buildOutcome{}, -1},
		{"exit 3", buildOutcome{err: &driver.ExitError{Code: 3}}, 3},
		{"timeout", buildOutcome{err: context.DeadlineExceeded, timedOut: true}, 124},
		{"SIGINT", buildOutcome{err: context.Canceled, interrupted: true}, 130},
		{"SIGTERM", buildOutcome{err: errors.New("terminated"), terminated: true}, 1},
	} {
		err := reportBuild(tc.out, &driver.BuildRequest{Timeout: time.Minute})
		var es exitStatusError
		switch {
		case tc.want < 0 && err != nil:
			t.Errorf("%s: %v, want success", tc.name, err)
		case tc.want >= 0 && (!errors.As(err, &es) || int(es) != tc.want):
			t.Errorf("%s: %v, want exit %d", tc.name, err, tc.want)
		}
	}
	if err := reportBuild(buildOutcome{err: failed}, &driver.BuildRequest{}); !errors.Is(err,
		failed) {
		t.Errorf("a failure: %v, want the failure itself, which exits 1", err)
	}
}

// warden shell (The build, warden shell): in mode shell warden attaches once
// the held environment is reported started, waits for no completion, and
// sends stop with shell-exited when the shell exits; the outcome is the
// shell's exit code, or the attach's own failure (BOOT-03, UX-09).
func TestBuildSequenceShell(t *testing.T) {
	refused := driver.Fixable("start a shell with warden shell",
		"no held shell environment of build k3v9x2ab")
	for _, tc := range []struct {
		name string
		code int
		err  error
	}{
		{"the shell exits 7", 7, nil},
		{"the shell exits 0", 0, nil},
		{"the attach fails", 0, refused},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newSequenceTest(wellBehaved, 0)
			s.seq.start.Mode = driver.ModeShell
			attached := make(chan struct{}, 1)
			s.seq.attach = func() (int, error) {
				attached <- struct{}{}
				return tc.code, tc.err
			}
			s.begin()
			out := s.end(t)
			select {
			case <-attached:
			default:
				t.Fatal("warden did not attach")
			}
			wantTrace(t, s.tr.get(), traceOf("stop shell-exited", "stopped")...)
			if out.shellCode != tc.code || !errors.Is(out.err, tc.err) {
				t.Errorf("outcome %+v, want code %d and %v", out, tc.code, tc.err)
			}
			if s.sess.waited.Load() {
				t.Error("warden waited for a completion in a shell")
			}
		})
	}
}

// A shell's exit: the shell's code, 130 for Ctrl-C, and 1 for SIGTERM and
// for a failed attach, whose error warden prints (UX-09).
func TestReportShellExitCodes(t *testing.T) {
	captureLog(t)
	refused := errors.New("no held shell environment")
	for _, tc := range []struct {
		name string
		out  buildOutcome
		want int // -1 is no exit status error
	}{
		{"exit 0", buildOutcome{}, -1},
		{"exit 7", buildOutcome{shellCode: 7}, 7},
		{"SIGINT", buildOutcome{err: context.Canceled, interrupted: true}, 130},
		{"SIGTERM", buildOutcome{err: errors.New("terminated"), terminated: true}, 1},
	} {
		err := reportShell(tc.out)
		var es exitStatusError
		switch {
		case tc.want < 0 && err != nil:
			t.Errorf("%s: %v, want success", tc.name, err)
		case tc.want >= 0 && (!errors.As(err, &es) || int(es) != tc.want):
			t.Errorf("%s: %v, want exit %d", tc.name, err, tc.want)
		}
	}
	if err := reportShell(buildOutcome{err: refused}); !errors.Is(err, refused) {
		t.Errorf("a failed attach: %v, want its error, which exits 1", err)
	}
}
