//go:build windows

package hyperv

import (
	"os/exec"
	"strings"
	"testing"
)

// TestRunPSTrailingSuppressedLookup (native F6): powershell.exe exits 1 with
// an empty stderr when a script's last statement is a SilentlyContinue
// lookup that finds nothing; runPS's error names stdout's tail, and a final
// DONE line makes the same script succeed.
func TestRunPSTrailingSuppressedLookup(t *testing.T) {
	if _, err := exec.LookPath("powershell.exe"); err != nil {
		t.Skip("no powershell.exe")
	}
	script := "'SLOT=a'\n$x = Get-Item -Path 'Env:WARDEN_NO_SUCH_VARIABLE' " +
		"-ErrorAction SilentlyContinue\nif ($x) { 'found' }\n"
	_, err := runPS(script)
	if err == nil || !strings.Contains(err.Error(), "stdout ended: SLOT=a") {
		t.Errorf("trailing suppressed lookup: %v", err)
	}
	out, err := runPS(script + "'" + psDone + "'\n")
	if err != nil || psDoneCheck(out) != nil {
		t.Errorf("with DONE: %q, %v", out, err)
	}
}
