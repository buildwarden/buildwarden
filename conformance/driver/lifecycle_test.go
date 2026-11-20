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
		Teardown: 20 * time.Second}) {
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
		"no-teardown":          "stop",
		"ignore-end-of-input":  "end of input",
		"die-on-closed-stderr": "orphaned",
		"reap-without-looking": "kill",
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
				Teardown: 3 * time.Second}) {
				if r.Check == check && r.Failure == "" {
					t.Errorf("%s passed under fault %s", check, fault)
				}
			}
		})
	}
}

// stopStubRelays kills the stub relays the fake's builds under tmp left.
func stopStubRelays(tmp string) {
	pids, _ := filepath.Glob(filepath.Join(tmp, "warden-fake-*", "relay.pid"))
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
