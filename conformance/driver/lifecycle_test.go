package driverconformance

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

// hermetic gives the suite and the drivers it runs a user cache directory of
// the test's own, where the build locks are.
func hermetic(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	for _, v := range []string{"XDG_CACHE_HOME", "HOME", "LocalAppData"} {
		t.Setenv(v, dir)
	}
}

// The lifecycle tier passes against the reference fake with stubrelay
// (TEST-05).
func TestLifecycleFake(t *testing.T) {
	hermetic(t)
	s := fakeSuite(t)
	for _, r := range s.Lifecycle(LifecycleOptions{StubRelay: true,
		Teardown: 20 * time.Second, CaseDeadline: 2 * time.Minute}) {
		if r.Failure != "" {
			t.Errorf("%s (%s): %s", r.Check, strings.Join(r.Requirements, ", "),
				r.Failure)
		}
	}
}

// The lifecycle tier's self-test: each case fails under the fake's fault
// that breaks what it checks.
func TestLifecycleSelfTest(t *testing.T) {
	hermetic(t)
	for fault, check := range map[string]string{
		"no-teardown":            "stop",
		"ignore-end-of-input":    "end of input",
		"die-on-closed-stderr":   "orphaned",
		"reap-without-looking":   "kill",
		"orphan-counts-as-alive": "clean",
	} {
		t.Run(fault, func(t *testing.T) {
			s := fakeSuite(t)
			s.env = []string{"WARDEN_FAKE_FAULT=" + fault}
			// A fault can leave a build's stub relay running; the fake
			// records its process ID in the build's work directory.
			t.Cleanup(func() { stopStubRelays(s.tmp) })
			var skip []string
			for _, c := range lifecycleCases {
				if c.name != check && c.name != "probe" {
					skip = append(skip, c.name)
				}
			}
			for _, r := range s.Lifecycle(LifecycleOptions{StubRelay: true, Skip: skip,
				Teardown: 3 * time.Second, CaseDeadline: time.Minute}) {
				if r.Check == check && r.Failure == "" {
					t.Errorf("%s passed under fault %s", check, fault)
				}
			}
		})
	}
}

// A case that passes its deadline is reported as hung, and the drivers of
// the builds it holds are killed, rather than the tier waiting on it.
func TestRunCaseDeadline(t *testing.T) {
	block := make(chan struct{})
	defer close(block)
	l := &lifecycle{o: LifecycleOptions{CaseDeadline: 50 * time.Millisecond},
		held: map[string]*buildRun{}}
	for _, tc := range []struct {
		name     string
		run      func(*lifecycle) (string, string)
		finished bool
		failure  string
	}{
		{"finishes", func(*lifecycle) (string, string) { return "", "a failure" }, true,
			"a failure"},
		{"hangs", func(*lifecycle) (string, string) { <-block; return "", "" }, false, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := lifecycleCase{name: tc.name, run: tc.run}
			_, failure, finished := l.runCase(c)
			if finished != tc.finished || failure != tc.failure {
				t.Errorf("runCase = %q, finished %v; want %q, %v", failure,
					finished, tc.failure, tc.finished)
			}
		})
	}
	if got := l.killHeld(); got != "it held no build" {
		t.Errorf("killHeld with no build = %q", got)
	}
}

// The default case deadline is the slowest case's worst path, nine builds
// of 13 min and seven teardown waits: 180 min under the default teardown,
// and it grows with the teardown wait a slower host is given.
func TestDefaultCaseDeadline(t *testing.T) {
	for teardown, want := range map[time.Duration]time.Duration{
		60 * time.Second: 180 * time.Minute,
		5 * time.Minute:  432 * time.Minute,
	} {
		if got := DefaultCaseDeadline(teardown); got != want {
			t.Errorf("DefaultCaseDeadline(%s) = %s, want %s", teardown, got, want)
		}
	}
}

// A build held by a case that passed its deadline is killed, reaped and
// released: nothing of it is left on the host, and its lock is free
// (DRV-52).
func TestKillHeldReaps(t *testing.T) {
	hermetic(t)
	s := fakeSuite(t)
	t.Cleanup(func() { stopStubRelays(s.tmp) })
	if _, failure := s.checkDescribe(); failure != "" {
		t.Fatal(failure)
	}
	l := &lifecycle{s: s, o: LifecycleOptions{StubRelay: true, Teardown: 3 * time.Second},
		dir: filepath.Join(s.scratch, "context"), held: map[string]*buildRun{}}
	if err := l.prepare(); err != nil {
		t.Fatal(err)
	}
	b, err := l.start(script(0, true), "build", false)
	if err != nil {
		t.Fatal(err)
	}
	if err := b.upTo("environment"); err != nil {
		t.Fatal(err)
	}
	if l.leftIn(b) == "" {
		t.Fatal("the running build left nothing in the temporary directory to reap")
	}
	if got := l.killHeld(); !strings.Contains(got, "killed and reaped") {
		t.Errorf("killHeld = %q; want the build killed and reaped", got)
	}
	if left := l.leftIn(b); left != "" {
		t.Errorf("after killHeld, %s", left)
	}
	if ids := heldLocks(s.name); len(ids) != 0 {
		t.Errorf("after killHeld, the suite still holds the locks of %v", ids)
	}
	if len(l.held) != 0 {
		t.Errorf("after killHeld, the suite still holds %d builds", len(l.held))
	}
	b.end() // the hung case's own end, should it wake: a no-op now
}

// stopStubRelays kills the stub relays, and the waiters, the fake's builds
// under tmp left.
func stopStubRelays(tmp string) {
	pids, _ := filepath.Glob(filepath.Join(tmp, "warden-fake-*", "*.pid"))
	for _, f := range pids {
		data, _ := os.ReadFile(f)
		pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
		if err == nil && pid > 0 {
			if p, err := os.FindProcess(pid); err == nil {
				_ = p.Kill()
			}
		}
	}
}

// Every requirement of v1.md is named by a case of the static or the
// lifecycle tier, or listed in the specification as not machine-checked
// (TEST-05, LOOP-03).
func TestRequirementsCovered(t *testing.T) {
	spec, err := os.ReadFile("../../specs/driver-contract/v1.md")
	if err != nil {
		t.Fatal(err)
	}
	defined := regexp.MustCompile(`(?m)^\*\*(DRV-C?[0-9]+)\.\*\*`).FindAllSubmatch(spec, -1)
	if len(defined) == 0 {
		t.Fatal("no requirement found in v1.md")
	}
	covered := map[string]bool{}
	for _, c := range staticChecks {
		for _, r := range c.reqs {
			covered[r] = true
		}
	}
	for _, c := range lifecycleCases {
		for _, r := range c.reqs {
			covered[r] = true
		}
	}
	_, table, ok := strings.Cut(string(spec), "## Requirements that are not machine-checked")
	if !ok {
		t.Fatal("v1.md has no list of requirements that are not machine-checked")
	}
	for _, line := range strings.Split(table, "\n") {
		if !strings.HasPrefix(line, "| DRV-") {
			continue
		}
		first, _, _ := strings.Cut(strings.TrimPrefix(line, "| "), " |")
		for _, id := range regexp.MustCompile(`DRV-C?[0-9]+`).FindAllString(first, -1) {
			covered[id] = true
		}
	}
	for _, m := range defined {
		if id := string(m[1]); !covered[id] {
			t.Errorf("%s is named by no case and not listed as not "+
				"machine-checked", id)
		}
	}
}
