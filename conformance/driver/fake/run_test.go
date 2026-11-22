package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"
	"time"
)

// sleeper starts a process that lives until it is killed or the test ends.
func sleeper(t *testing.T) *exec.Cmd {
	t.Helper()
	c := exec.Command("sleep", "60")
	if err := c.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Process.Kill() })
	return c
}

// Teardown kills the relay through its own process and the waiter through
// waiter.pid, and never the PID relay.pid holds: by then the relay may have
// been reaped and its PID reused by an unrelated process (TEST-04).
func TestTeardownLeavesRelayPIDAlone(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the processes are sleep(1)")
	}
	w, err := os.Create(filepath.Join(t.TempDir(), "out"))
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	old := out.w
	out.w = w
	t.Cleanup(func() { out.w = old })

	dir := t.TempDir()
	relay, bystander, waiter := sleeper(t), sleeper(t), sleeper(t)
	for f, c := range map[string]*exec.Cmd{relayPIDFile: bystander, waiterPIDFile: waiter} {
		if err := os.WriteFile(filepath.Join(dir, f),
			[]byte(strconv.Itoa(c.Process.Pid)), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if code := teardown(&start{Keep: true}, relay, dir); code != 0 {
		t.Fatalf("teardown = %d", code)
	}
	for name, c := range map[string]*exec.Cmd{"relay": relay, "waiter": waiter} {
		if !exits(c, 5*time.Second) {
			t.Errorf("teardown did not kill the %s", name)
		}
	}
	if exits(bystander, 500*time.Millisecond) {
		t.Error("teardown killed the process relay.pid names")
	}
}

// exits reports whether c ends within d. The test reaps its own children, so
// a killed one cannot pass for alive as an unreaped zombie would.
func exits(c *exec.Cmd, d time.Duration) bool {
	done := make(chan struct{})
	go func() { _ = c.Wait(); close(done) }()
	select {
	case <-done:
		return true
	case <-time.After(d):
		return false
	}
}
