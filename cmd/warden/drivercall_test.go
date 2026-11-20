package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/buildwarden/buildwarden/driver"
)

// testFound is the test driver, as found by path.
func testFound() *foundDriver {
	return &foundDriver{Name: "tst", Path: testDriverPath, Found: foundPath,
		Source: sourceFlag}
}

// describedTest is the test driver, described, with the clock after.
func describedTest(t *testing.T, after clock) *execDriver {
	t.Helper()
	d, err := describeDriver(testFound(), time.After)
	if err != nil {
		t.Fatal(err)
	}
	d.after = after
	d.stdout, d.stderr = &strings.Builder{}, &strings.Builder{}
	return d
}

// describe is refused for each way it breaks DRV-38, naming what is wrong,
// and when it does not answer within 10 s (DRV-36), on a fake clock.
func TestDescribeRefusals(t *testing.T) {
	for variant, want := range map[string]string{
		"wrong-name": "name: the driver reports \"other\"",
		"no-reap":    "value must be 'reap'",
		"clash":      "flags: --timeout is warden's own flag on run",
		"bad-key":    "'vm.greeting' does not match",
		"contract-9": "speaks contract versions [9]",
	} {
		t.Run(variant, func(t *testing.T) {
			useTestDriver(t, "WARDEN_TEST_DESCRIBE", variant)
			_, err := describeDriver(testFound(), time.After)
			if err == nil || !strings.Contains(err.Error(), want) {
				t.Errorf("describe: %v, want %q", err, want)
			}
		})
	}
	t.Run("hang", func(t *testing.T) {
		useTestDriver(t, "WARDEN_TEST_DESCRIBE", "hang")
		clock := newFakeClock()
		done := make(chan error, 1)
		go func() { _, err := describeDriver(testFound(), clock.after); done <- err }()
		clock.fire(t, describeLimit)
		if err := <-done; err == nil || !strings.Contains(err.Error(), "within 10s") {
			t.Errorf("describe that hangs: %v", err)
		}
	})
}

// Each exit code of a command maps to its error, and anything the contract
// does not allow is a defect of the driver, with its bug link and not
// warden's (DRV-21, DRV-22, DRV-66, DRV-67).
func TestCallExitCodes(t *testing.T) {
	for variant, check := range map[string]func(error) bool{
		"ok": func(err error) bool { return err == nil },
		"fixable": func(err error) bool {
			var fe *driver.FixableError
			return errors.As(err, &fe) && fe.Fix == "do what the test says"
		},
		"busy":   isDriverError(false),
		"exit-2": isDriverError(true),
		"exit-3": isDriverError(true),
		"exit-4": func(err error) bool {
			var fe *driver.FixableError
			return errors.As(err, &fe) && strings.Contains(err.Error(), "contract")
		},
		"exit-7":    isDriverError(true),
		"no-result": isDriverError(true),
		"not-json":  isDriverError(true),
	} {
		t.Run(variant, func(t *testing.T) {
			useTestDriver(t, "WARDEN_TEST_CALL", variant)
			d := describedTest(t, time.After)
			_, err := d.call("doctor", map[string]any{"contract": 1}, callOpts{})
			if !check(err) {
				t.Errorf("doctor exiting as %s: %v", variant, err)
			}
		})
	}
}

// doctor of a driver binary prints ok only when neither warden's part nor
// the driver's result is blocked, and fails when either is: a blocked doctor
// has no ok line (UX-09). A driver that lists no doctor has no result, so
// warden's part alone decides.
func TestDoctorBinaryOk(t *testing.T) {
	for _, tc := range []struct {
		name          string
		lists         bool
		call          string
		wardenBlocked bool
		failed        bool
	}{
		{"nothing blocked", true, "ok", false, false},
		{"warden's part blocked", true, "ok", true, true},
		{"the driver blocked", true, "blocked", false, true},
		{"both blocked", true, "blocked", true, true},
		{"no doctor listed", false, "ok", false, false},
		{"no doctor listed, warden's part blocked", false, "ok", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			useTestDriver(t, "WARDEN_TEST_CALL", tc.call)
			d := describedTest(t, time.After)
			if !tc.lists {
				d.desc.Commands = slices.DeleteFunc(slices.Clone(d.desc.Commands),
					func(c string) bool { return c == "doctor" })
			}
			var out strings.Builder
			failed, err := doctorBinary(&out, &Config{},
				&selectedDriver{found: testFound(), bin: d}, tc.wardenBlocked)
			want := "ok\n"
			if tc.failed {
				want = ""
			}
			if err != nil || failed != tc.failed || out.String() != want {
				t.Errorf("doctor = %v, %v, printed %q; want %v, printed %q",
					failed, err, out.String(), tc.failed, want)
			}
		})
	}
}

func isDriverError(bug bool) func(error) bool {
	return func(err error) bool {
		var de *driverError
		return errors.As(err, &de) && de.bug == bug &&
			(!bug || de.bugURL == "https://example.com/tst/bugs")
	}
}

// A probe of run that does not answer within 30 s is a defect (DRV-31).
func TestProbeLimit(t *testing.T) {
	useTestDriver(t, "WARDEN_TEST_RUN", "probe-hang")
	clock := newFakeClock()
	d := describedTest(t, clock.after)
	done := make(chan error, 1)
	go func() { done <- d.probeRun(&driver.Start{Contract: 1, Probe: true}) }()
	clock.fire(t, probeLimit)
	if err := <-done; err == nil || !strings.Contains(err.Error(), "within 30s") {
		t.Errorf("probe that hangs: %v", err)
	}
	useTestDriver(t, "WARDEN_TEST_RUN", "ok")
	if err := d.probeRun(&driver.Start{Contract: 1, Probe: true}); err != nil {
		t.Errorf("probe: %v", err)
	}
}

// execTest is a build sequence over the exec transport with the test driver.
type execTest struct {
	*sequenceTest
	peer *execPeer
	log  string
}

func newExecTest(t *testing.T, vars ...string) *execTest {
	t.Helper()
	log := filepath.Join(t.TempDir(), "requests")
	useTestDriver(t, append([]string{"WARDEN_TEST_LOG", log}, vars...)...)
	st := newSequenceTest(nil, 0)
	d := describedTest(t, st.clock.after)
	rec := buildRecord{Driver: "tst", ID: "k3v9x2ab", Path: testDriverPath,
		TempDir: "/the/build/tmp", ConfigFile: "/the/build/warden.toml",
		Args: []string{"--greeting=hi"}}
	peer := newExecPeer(d, rec, "/locks/build-tst-k3v9x2ab.state")
	st.seq.peer = peer
	return &execTest{sequenceTest: st, peer: peer, log: log}
}

// asked waits until a timer of d is asked for, and does not fire it.
func (e *execTest) asked(t *testing.T, d time.Duration) {
	t.Helper()
	select {
	case got := <-e.clock.asked:
		if got != d {
			t.Fatalf("timer of %s asked for, want %s", got, d)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("no timer of %s was asked for", d)
	}
}

// requests is what the test driver logged of the requests it was sent.
func (e *execTest) requests(t *testing.T) string {
	t.Helper()
	data, _ := os.ReadFile(e.log)
	return string(data)
}

// A build over the exec transport: the claim at the reported endpoint,
// claimed and environment-started, stop on completion and stopped.
func TestExecBuild(t *testing.T) {
	e := newExecTest(t)
	e.begin()
	e.claimStarted(t)
	e.sess.done <- nil
	out := e.end(t)
	if out.err != nil || out.leftovers || !out.claimed {
		t.Errorf("outcome %+v", out)
	}
	if !strings.Contains(e.requests(t), `"claim_token":"token"`) {
		t.Errorf("run was not sent start: %s", e.requests(t))
	}
}

// A driver that writes a line that is not a message, or ends with neither
// stopped nor failed, is killed and its build reaped with the record's
// values (DRV-54, DRV-60); a reap that fails keeps the record and lock.
func TestExecDefectsAreReaped(t *testing.T) {
	for _, tc := range []struct{ run, reap string }{
		{"noise", "ok"}, {"early", "ok"}, {"noise", "fail"},
	} {
		t.Run(tc.run+" reap "+tc.reap, func(t *testing.T) {
			e := newExecTest(t, "WARDEN_TEST_RUN", tc.run, "WARDEN_TEST_REAP", tc.reap)
			e.begin()
			out := e.end(t)
			var de *driverError
			if !errors.As(out.err, &de) || !de.bug {
				t.Fatalf("outcome %v, want a driver defect", out.err)
			}
			reqs := e.requests(t)
			if !strings.Contains(reqs, `reap {"args":["--greeting=hi"],`+
				`"build_id":"k3v9x2ab","config_file":"/the/build/warden.toml",`+
				`"contract":1,"state_file":"/locks/build-tst-k3v9x2ab.state",`+
				`"tmpdir":"/the/build/tmp"}`) {
				t.Errorf("reap's request: %s", reqs)
			}
			if out.leftovers != (tc.reap == "fail") {
				t.Errorf("leftovers %v with a reap that did %s", out.leftovers,
					tc.reap)
			}
		})
	}
}

// A driver that does not stop within the teardown wait is killed and
// reaped; a reap that does not finish within the same wait is killed, and
// the record and lock stay (DRV-52).
func TestExecTeardownWait(t *testing.T) {
	e := newExecTest(t, "WARDEN_TEST_RUN", "no-stop", "WARDEN_TEST_REAP", "hang")
	e.begin()
	e.claimStarted(t)
	e.sess.done <- nil
	e.clock.fire(t, defaultTeardown)
	e.clock.fire(t, defaultTeardown)
	out := e.end(t)
	if out.err != nil || out.teardownErr == nil || !out.leftovers {
		t.Errorf("outcome %+v: want the build's result, the defect beside it and "+
			"its leftovers kept", out)
	}
}

// A second signal kills the driver and reaps; a third kills that reap
// (DRV-52).
func TestExecSecondSignal(t *testing.T) {
	e := newExecTest(t, "WARDEN_TEST_RUN", "no-stop", "WARDEN_TEST_REAP", "hang")
	e.seq.stopSignals = func() {}
	e.begin()
	e.claimStarted(t)
	e.signals <- os.Interrupt
	e.asked(t, defaultTeardown) // the teardown wait starts on stop
	e.signals <- syscall.SIGTERM
	e.asked(t, defaultTeardown) // the reap's
	e.signals <- os.Interrupt
	out := e.end(t)
	if !out.interrupted || !out.leftovers {
		t.Errorf("outcome %+v, want interrupted with leftovers", out)
	}
}

// A driver's failed ends the build with its error; no reap runs (DRV-54).
func TestExecFailed(t *testing.T) {
	e := newExecTest(t, "WARDEN_TEST_RUN", "failed")
	e.begin()
	out := e.end(t)
	var fe *driver.FixableError
	if !errors.As(out.err, &fe) || fe.Fix != "install one" {
		t.Errorf("outcome %v", out.err)
	}
	if strings.Contains(e.requests(t), "reap ") {
		t.Errorf("a driver that failed was reaped: %s", e.requests(t))
	}
}

// attach is sent the record's values and its exit code is the shell's
// (DRV-28, DRV-60).
func TestExecAttach(t *testing.T) {
	log := filepath.Join(t.TempDir(), "requests")
	useTestDriver(t, "WARDEN_TEST_LOG", log)
	d := describedTest(t, time.After)
	p := &prepared{sd: &selectedDriver{found: d.found, bin: d}, id: "k3v9x2ab",
		live: &liveBuild{record: buildRecord{Driver: "tst", ID: "k3v9x2ab",
			ConfigFile: "/the/build/warden.toml", Args: []string{"--greeting=hi"}}}}
	code, err := attach(p)
	if err != nil || code != 5 {
		t.Errorf("attach: %d, %v; want the shell's 5", code, err)
	}
	data, _ := os.ReadFile(log)
	var req map[string]any
	if err := json.Unmarshal([]byte(strings.TrimPrefix(strings.TrimSpace(string(data)),
		"attach ")), &req); err != nil {
		t.Fatal(err)
	}
	if req["config_file"] != "/the/build/warden.toml" || req["build_id"] != "k3v9x2ab" {
		t.Errorf("attach's request %v", req)
	}
}

// A context, output or temporary path that is not UTF-8 is refused before a
// driver binary starts (DRV-19).
func TestUTF8Paths(t *testing.T) {
	for _, what := range []string{"context", "output", "temporary"} {
		err := checkUTF8Paths(map[string]string{what: "/tmp/bad-\xff"})
		var fe *driver.FixableError
		if !errors.As(err, &fe) || !strings.Contains(err.Error(), what) {
			t.Errorf("%s: %v", what, err)
		}
	}
	if err := checkUTF8Paths(map[string]string{"context": "/tmp/ok-é"}); err != nil {
		t.Error(err)
	}
}

// [build] teardown_timeout: its default, a value, and each refusal with the
// key's own message as the file loads.
func TestTeardownTimeout(t *testing.T) {
	if (BuildCfg{}).teardown() != 60*time.Second {
		t.Error("the default is not 60s")
	}
	for value, want := range map[string]string{
		`"90s"`:    "",
		`"soon"`:   `"soon" is not a duration`,
		`"0s"`:     `"0s" is below the minimum of 1s`,
		`"2h"`:     `"2h" is above the maximum of 1h`,
		`90`:       "90 is not a duration",
		`"999ms"`:  "below the minimum",
		`"1h"`:     "",
		`"1s"`:     "",
		`["90s"]`:  "is not a duration",
		`"-5s"`:    "below the minimum",
		`"1h0m1s"`: "above the maximum",
	} {
		inConfigFiles(t, "[build]\nteardown_timeout = "+value+"\n")
		cfg, err := LoadConfig()
		switch {
		case want == "" && err != nil:
			t.Errorf("%s: %v", value, err)
		case want == "":
			if cfg.Build.teardown() <= 0 {
				t.Errorf("%s: %v", value, cfg.Build.teardown())
			}
		case err == nil || !strings.Contains(err.Error(), want) ||
			!strings.Contains(err.Error(), "[build] teardown_timeout in "):
			t.Errorf("%s: %v, want %q", value, err, want)
		}
	}
}

// The copy of the contract's schema warden embeds is the specification's.
func TestDriverSchemaCopy(t *testing.T) {
	spec, err := os.ReadFile(filepath.Join("..", "..", "specs", "driver-contract",
		"v1.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(spec) != string(driverSchema) {
		t.Error("cmd/warden/driver-contract-v1.schema.json differs from the " +
			"specification's v1.schema.json")
	}
}
