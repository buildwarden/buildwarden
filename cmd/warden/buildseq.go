package main

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/buildwarden/buildwarden/driver"
)

// The build sequence (specs/driver-contract/v1.md, The build): one piece of
// code over either transport. warden sends start, claims the relay at the
// endpoint the driver reports, sends claimed, waits for the build through
// the collector, sends stop, and waits for the driver to tear down. The
// driver's side is a runPeer: a linked driver's Run called in warden's
// process, or a driver binary's run over its pipes.

// buildExitGrace is how long a build whose environment exited is given to
// deliver a completion already on its way, before it is failed (DRV-48).
const buildExitGrace = 10 * time.Second

// eventKind is what a runEvent reports.
type eventKind int

// The events of a driver's run, its messages in the specification's terms.
const (
	evRelayStarted eventKind = iota // relay-started
	evStarted                       // environment-started
	evExited                        // exited
	evEnd                           // stopped or failed: the driver tore down
)

// runEvent is one message from the driver's run.
type runEvent struct {
	kind  eventKind
	relay driver.RelayStarted
	// reply answers relay-started: nil once warden has claimed the relay
	// (claimed), or why it was not.
	reply  chan<- error
	which  driver.ExitedPart
	detail string
	// kept and err end the run: err nil is stopped, else failed.
	kept []string
	err  error
}

// runPeer is the driver's side of a build sequence.
type runPeer interface {
	// begin starts the driver's run with start; its messages arrive on
	// events, the last of them evEnd.
	begin(start *driver.Start)
	events() <-chan runEvent
	// stop asks the driver to tear down, once; later calls do nothing.
	stop(reason, detail string)
}

// buildSession is the collector's side of a build: driver.Session, which
// tests replace.
type buildSession interface {
	// claim starts the collector for the guest r reports and claims the
	// relay at r's endpoint, until the relay is ready.
	claim(ctx context.Context, r driver.RelayStarted) error
	// wait returns when the build completes (nil, or a *driver.ExitError),
	// fails, or ctx ends.
	wait(ctx context.Context) error
	// result is the build's result when the collector holds its completion:
	// nil or a *driver.ExitError, and true.
	result() (error, bool)
	// close ends the session; the collector's files are complete after it.
	close()
}

// buildOutcome is how a build ended.
type buildOutcome struct {
	// err is nil for a build that completed with code 0, a *driver.ExitError
	// for one that completed with another code, and otherwise why it failed.
	err error
	// interrupted, terminated and timedOut name why warden stopped it:
	// SIGINT, SIGTERM or --timeout.
	interrupted, terminated, timedOut bool
	// claimed reports that a claim was made, so the session holds output.
	claimed bool
	// kept is what the driver left under --keep-vms.
	kept []string
	// teardownErr is an error the driver reported after it was asked to
	// stop: the build's result stands, and this is said beside it.
	teardownErr error
}

// buildSequence runs one build of a driver.
type buildSequence struct {
	start   *driver.Start
	peer    runPeer
	session buildSession
	// timeout is --timeout, counted from start; zero is none.
	timeout time.Duration
	// signals delivers SIGINT and SIGTERM. stopSignals is called on the
	// first, so that a second one ends warden at once.
	signals     <-chan os.Signal
	stopSignals func()
	// after is the clock of the grace and the timeout.
	after func(time.Duration) <-chan time.Time
}

// errStopped is what a claim the sequence gave up on returns to the driver.
var errStopped = errors.New("the build was stopped")

// seqState is a running sequence's state.
type seqState struct {
	*buildSequence
	out     buildOutcome
	decided bool // out.err is set
	stopped bool // stop was sent
	relayUp bool // relay-started was received
	claimed bool // claimed was sent

	pending     chan<- error // the reply to a relay-started being claimed
	cancelClaim context.CancelFunc
	cancelWait  context.CancelFunc
	claimDone   chan error
	waitDone    chan error
	grace       <-chan time.Time
	graceDetail string
	relayGone   string // why a claim in progress was cancelled
}

// decide sets the outcome, the first cause winning, and sends stop.
func (st *seqState) decide(err error, reason string) {
	if !st.decided {
		st.out.err, st.decided = err, true
	}
	if st.stopped {
		return
	}
	st.stopped = true
	detail := "the build completed"
	if err != nil {
		detail = err.Error()
	}
	st.cancelClaim()
	st.cancelWait()
	st.peer.stop(reason, detail)
}

// run runs the sequence to its end: until the driver has torn down.
func (s *buildSequence) run() buildOutcome {
	st := &seqState{buildSequence: s, cancelClaim: func() {}, cancelWait: func() {},
		claimDone: make(chan error, 1), waitDone: make(chan error, 1)}
	defer func() { st.cancelClaim(); st.cancelWait() }()
	var deadline <-chan time.Time
	if s.timeout > 0 {
		deadline = s.after(s.timeout)
	}
	s.peer.begin(s.start)
	for {
		select {
		case ev := <-s.peer.events():
			if st.onEvent(ev) {
				return st.out
			}
		case err := <-st.claimDone:
			st.onClaimed(err)
		case err := <-st.waitDone:
			reason := driver.StopCompleted
			var ee *driver.ExitError
			if err != nil && !errors.As(err, &ee) {
				reason = driver.StopFailed
			}
			st.decide(err, reason)
		case <-st.grace:
			st.decide(fmt.Errorf("the build environment exited before the build "+
				"completed: %s", st.graceDetail), driver.StopFailed)
		case sig := <-s.signals:
			st.onSignal(sig)
		case <-deadline:
			if !st.stopped {
				st.out.timedOut = true
				st.decide(context.DeadlineExceeded, driver.StopTimeout)
			}
		}
	}
}

// onEvent handles one message of the driver's, and reports whether it was
// the last.
func (st *seqState) onEvent(ev runEvent) bool {
	switch ev.kind {
	case evRelayStarted:
		st.onRelayStarted(ev)
	case evStarted:
		if !st.claimed && !st.stopped {
			st.decide(driverDefect("reported its environment started before "+
				"warden had claimed the relay"), driver.StopFailed)
		}
	case evExited:
		st.onExited(ev)
	case evEnd:
		st.onEnd(ev)
		return true
	}
	return false
}

// onRelayStarted claims the relay the driver reports, in the background.
func (st *seqState) onRelayStarted(ev runEvent) {
	switch {
	case st.stopped:
		ev.reply <- errStopped
	case st.relayUp:
		ev.reply <- errStopped
		st.decide(driverDefect("reported its relay started twice"), driver.StopFailed)
	default:
		st.relayUp, st.pending, st.out.claimed = true, ev.reply, true
		var ctx context.Context
		ctx, st.cancelClaim = context.WithCancel(context.Background())
		r := ev.relay
		go func() { st.claimDone <- st.session.claim(ctx, r) }()
	}
}

// onClaimed answers the driver's relay-started with the claim's result, and
// once the relay is claimed waits for the build.
func (st *seqState) onClaimed(err error) {
	reply := st.pending
	st.pending = nil
	switch {
	case err == nil && !st.stopped:
		st.claimed = true
		var ctx context.Context
		ctx, st.cancelWait = context.WithCancel(context.Background())
		go func() { st.waitDone <- st.session.wait(ctx) }()
		reply <- nil
		return
	case st.relayGone != "":
		err = fmt.Errorf("the relay exited before it was claimed: %s", st.relayGone)
	case err == nil || st.stopped:
		err = errStopped
	}
	reply <- err
	st.decide(err, driver.StopFailed)
}

// onExited applies the rule of DRV-48: a relay that exited cancels the
// claim or fails the build at once; an environment that exited gives a
// completion on its way the grace.
func (st *seqState) onExited(ev runEvent) {
	switch {
	case st.stopped:
	case ev.which == driver.ExitedRelay && st.pending != nil:
		st.relayGone = ev.detail
		st.cancelClaim()
	case ev.which == driver.ExitedRelay:
		st.decide(fmt.Errorf("the relay exited: %s", ev.detail), driver.StopFailed)
	case st.grace == nil:
		if _, ok := st.session.result(); ok {
			return // the completion is in; wait returns it
		}
		st.graceDetail = ev.detail
		st.grace = st.after(buildExitGrace)
	}
}

// onEnd ends the sequence with the driver's last message.
func (st *seqState) onEnd(ev runEvent) {
	st.out.kept = ev.kept
	if st.pending != nil {
		st.pending <- errStopped
		st.pending = nil
	}
	switch {
	case st.stopped:
		// A driver returns the claim's refusal it was given, or says it was
		// stopped; anything else is its teardown's own failure.
		if ev.err != nil && !errors.Is(ev.err, st.out.err) &&
			!errors.Is(ev.err, errStopped) {
			st.out.teardownErr = ev.err
		}
	case ev.err != nil:
		st.decide(ev.err, driver.StopFailed)
	default:
		// It stopped by itself: only a completion already in makes that a
		// result.
		if res, ok := st.session.result(); ok {
			st.decided, st.out.err = true, res
			st.stopped = true
			return
		}
		st.decide(driverDefect("ended the build before warden asked it to stop"),
			driver.StopFailed)
	}
}

// onSignal stops the build on the first SIGINT or SIGTERM.
func (st *seqState) onSignal(sig os.Signal) {
	if st.stopped {
		return
	}
	st.stopSignals()
	if sig == syscall.SIGTERM {
		st.out.terminated = true
		st.decide(errors.New("terminated (SIGTERM)"), driver.StopInterrupted)
		return
	}
	st.out.interrupted = true
	fmt.Fprintf(os.Stderr, "\nInterrupted, cleaning up... (press ctrl+c again to force)\n")
	st.decide(context.Canceled, driver.StopInterrupted)
}

// driverDefect is a driver that broke the build dialogue: a defect of the
// driver, not of the build (DRV-22).
func driverDefect(what string) error {
	return fmt.Errorf("driver defect: the driver %s", what)
}

// runLinkedBuild runs a build of a linked driver through the build sequence:
// warden makes the claim token, starts the collector and claims the relay,
// owns SIGINT and SIGTERM and --timeout, and reports the build.
func runLinkedBuild(p *prepared) error {
	token, err := driver.RandomToken()
	if err != nil {
		return err
	}
	start := newStart(p.cfg, p.req, p.linked.Capabilities, p.id, token,
		p.live.statePath())
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigs)
	sess := &collectorSession{req: p.req, token: token, verbose: p.cfg.Output.Verbose}
	seq := &buildSequence{
		start: start, peer: newLinkedPeer(p.linked, p.cfg, p.req), session: sess,
		timeout: p.req.Timeout, signals: sigs, stopSignals: func() { signal.Stop(sigs) },
		after: time.After,
	}
	out := seq.run()
	sess.close()
	if out.claimed {
		driver.FinalizeOutput(p.req.OutputDir, p.req.Compress)
	}
	return reportBuild(out, p.req)
}

// reportBuild says how the build ended and gives warden's exit: the build
// script's code, 124 for --timeout, 130 for Ctrl-C, and 1 for SIGTERM and
// every other failure (UX-09).
func reportBuild(out buildOutcome, req *driver.BuildRequest) error {
	if out.teardownErr != nil {
		log.Info(fmt.Sprintf("warning: the driver's teardown failed: %v", out.teardownErr))
	}
	for _, k := range out.kept {
		log.Info("--keep-vms: left " + k)
	}
	var ee *driver.ExitError
	switch {
	case out.err == nil:
		fmt.Fprintf(os.Stderr, "warden: build SUCCEEDED (output: %s)\n", req.OutputDir)
		return nil
	case errors.As(out.err, &ee):
		msg := fmt.Sprintf("warden: build FAILED (exit code %d)", ee.Code)
		if ee.Detail != "" {
			msg += ": " + ee.Detail
		}
		fmt.Fprintln(os.Stderr, msg)
		return exitStatusError(ee.Code)
	case out.timedOut:
		fmt.Fprintf(os.Stderr,
			"warden: build FAILED: timed out after %s (--timeout)\n", req.Timeout)
		return exitStatusError(exitTimedOut)
	case out.interrupted:
		fmt.Fprintln(os.Stderr, "warden: build interrupted")
		return exitStatusError(exitInterrupted)
	case out.terminated:
		fmt.Fprintln(os.Stderr, "warden: build terminated (SIGTERM)")
		return exitStatusError(1)
	}
	fmt.Fprintln(os.Stderr, "warden: build FAILED")
	return out.err
}

// newStart is the start message of a build of a driver with caps: the build
// ID, claim token and state file are empty for a probe (DRV-32).
func newStart(cfg *Config, req *driver.BuildRequest, caps capabilities,
	id, token, stateFile string) *driver.Start {
	st := &driver.Start{
		Contract: contractVersion, BuildID: id, Mode: driver.ModeBuild,
		ClaimToken: token, GuestOS: req.GuestOS, Image: req.Image,
		BaseImage: req.BaseImage, BuildScript: req.BuildScript,
		ContextDir: req.ContextDir, TempDir: req.TempDir, StateFile: stateFile,
		Keep: req.KeepVMs && caps.Keep != keepNone, Verbose: cfg.Output.Verbose,
		ConfigFile: absConfigFile(cfg), Args: []string{},
	}
	if st.GuestOS == "" && len(caps.GuestOS) > 0 {
		st.GuestOS = caps.GuestOS[0]
	}
	if id != "" {
		st.DiagnosticsDir, _ = filepath.Abs(req.OutputDir)
	}
	if caps.VM {
		st.VM = &driver.StartVM{CPUs: req.CPUs, MemoryMB: req.MemoryMB,
			BuildLink: req.BuildLink.String(), DNS: []string{}}
		if _, source, err := resolveBuildLink(cfg); err == nil {
			st.VM.BuildLinkSource = source
		}
		if req.BuildLink.IsZero() {
			st.VM.BuildLink = ""
		}
		for _, a := range req.DNS {
			st.VM.DNS = append(st.VM.DNS, a.String())
		}
	}
	return st
}

// linkedPeer runs a linked driver's Run in warden's process, its messages as
// the calls of the driver.Host it is given.
type linkedPeer struct {
	d   *linkedDriver
	cfg *Config
	req *driver.BuildRequest
	ev  chan runEvent

	once   sync.Once
	stopCh chan struct{}
	reason string
}

func newLinkedPeer(d *linkedDriver, cfg *Config, req *driver.BuildRequest) *linkedPeer {
	return &linkedPeer{d: d, cfg: cfg, req: req, ev: make(chan runEvent),
		stopCh: make(chan struct{})}
}

func (p *linkedPeer) begin(start *driver.Start) {
	go func() {
		// The driver's context is not cancelled: stop is how it is told to
		// tear down, and its teardown needs a live context.
		kept, err := p.d.Run(context.Background(), p.cfg, p.req, start, linkedHost{p})
		p.ev <- runEvent{kind: evEnd, kept: kept, err: err}
	}()
}

func (p *linkedPeer) events() <-chan runEvent { return p.ev }

func (p *linkedPeer) stop(reason, _ string) {
	p.once.Do(func() {
		p.reason = reason
		close(p.stopCh)
	})
}

// linkedHost is warden's driver.Host for a linked driver.
type linkedHost struct{ p *linkedPeer }

func (h linkedHost) ClaimRelay(ctx context.Context, r driver.RelayStarted) error {
	reply := make(chan error, 1)
	select {
	case h.p.ev <- runEvent{kind: evRelayStarted, relay: r, reply: reply}:
	case <-ctx.Done():
		return ctx.Err()
	}
	select {
	case err := <-reply:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (h linkedHost) Started() error {
	h.p.ev <- runEvent{kind: evStarted}
	return nil
}

func (h linkedHost) Exited(which driver.ExitedPart, detail string) {
	h.p.ev <- runEvent{kind: evExited, which: which, detail: detail}
}

func (h linkedHost) Wait(ctx context.Context) string {
	select {
	case <-h.p.stopCh:
		return h.p.reason
	case <-ctx.Done():
		return driver.StopInterrupted
	}
}

// collectorSession is the build's driver.Session: warden's collector, served
// to the relay over the tunnel warden opens on the claim connection.
type collectorSession struct {
	req     *driver.BuildRequest
	token   string
	verbose bool
	s       *driver.Session
}

func (c *collectorSession) claim(ctx context.Context, r driver.RelayStarted) error {
	if r.Claim.Network != driver.ClaimTCP {
		return driverDefect(fmt.Sprintf("reported a claim endpoint on network %q",
			r.Claim.Network))
	}
	dns, err := parseUpstreamDNS(r.ClaimValues.UpstreamDNS)
	if err != nil {
		return driverDefect(err.Error())
	}
	s, err := driver.StartSession(driver.SessionConfig{
		OutputDir:   c.req.OutputDir,
		ContextDir:  c.req.ContextDir,
		BuildScript: c.req.BuildScript,
		GuestOS:     r.GuestOS,
		OutputTee:   driver.ConsoleTee(c.req),
		Verbose:     c.verbose,
	})
	if err != nil {
		return err
	}
	c.s = s
	claimReq := *c.req
	claimReq.DNS = dns
	_, err = s.ClaimRelay(ctx, r.Claim.Address, c.token, &claimReq,
		r.ClaimValues.Environment, r.ClaimValues.BuildLink, r.OldRelayFix)
	return err
}

func (c *collectorSession) wait(ctx context.Context) error { return c.s.Wait(ctx) }

func (c *collectorSession) result() (error, bool) {
	if c.s == nil {
		return nil, false
	}
	sig, ok := c.s.Collector.Status()
	if !ok {
		return nil, false
	}
	if sig.ExitCode == 0 {
		return nil, true
	}
	detail := sig.Error
	if detail == "" {
		detail = sig.Message
	}
	return &driver.ExitError{Code: sig.ExitCode, Detail: detail}, true
}

func (c *collectorSession) close() {
	if c.s != nil {
		c.s.Close()
	}
}

// parseUpstreamDNS reads the relay's upstream resolvers a driver reported.
func parseUpstreamDNS(ss []string) ([]netip.Addr, error) {
	var out []netip.Addr
	for _, s := range ss {
		a, err := netip.ParseAddr(s)
		if err != nil {
			return nil, fmt.Errorf("reported an upstream resolver %q that is not an "+
				"address", s)
		}
		out = append(out, a)
	}
	return out, nil
}
