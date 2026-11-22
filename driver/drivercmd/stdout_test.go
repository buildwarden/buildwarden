package drivercmd

import (
	"os"
	"os/exec"
	"runtime"
	"testing"
)

// writeAfterCollect is the "collected" child: it moves its message stream
// off standard output, lets the old os.Stdout be collected, and then writes
// on the stream.
func writeAfterCollect() int {
	msgs, err := redirectStdout()
	if err != nil {
		return 2
	}
	for range 3 {
		runtime.GC()
	}
	if _, err := msgs.WriteString("still open\n"); err != nil {
		_, _ = os.Stderr.WriteString(err.Error() + "\n")
		return 1
	}
	return 0
}

// The message stream outlives the os.Stdout it was taken from: once that
// file is collected, a write on the stream still reaches the orchestrator.
// On Windows the two once shared one handle, which the file's finalizer
// closed partway through a build.
func TestMessageStreamSurvivesCollection(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=^$")
	cmd.Env = append(os.Environ(), testDriverEnv+"=collected")
	out, err := cmd.Output()
	if err != nil {
		var stderr []byte
		if ee, ok := err.(*exec.ExitError); ok {
			stderr = ee.Stderr
		}
		t.Fatalf("child: %v: %s", err, stderr)
	}
	if string(out) != "still open\n" {
		t.Errorf("the stream carried %q, want %q", out, "still open\n")
	}
}
