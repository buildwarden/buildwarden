//go:build windows

package hyperv

import (
	"bytes"
	"encoding/json"
	"os/exec"
	"strings"
)

// runPS runs a PowerShell command and returns its stdout. On failure the error
// carries stderr, or with none the tail of stdout (psError), so callers
// surface a real message instead of a bare exit code.
func runPS(script string) (string, error) {
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script)
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		return out.String(), psError(out.String(), errb.String(), err)
	}
	return out.String(), nil
}

// runPSDone is runPS for a script that prints psDone last: a run that exits 0
// without it stopped early (psDoneCheck), and that is an error too.
func runPSDone(script string) (string, error) {
	out, err := runPS(script)
	if err != nil {
		return out, err
	}
	return out, psDoneCheck(out)
}

// runPSJSON runs a PowerShell command, appends `| ConvertTo-Json -Compress`,
// and unmarshals the result into out.
func runPSJSON(script string, out any) error {
	res, err := runPS(script + " | ConvertTo-Json -Compress")
	if err != nil {
		return err
	}
	res = strings.TrimSpace(res)
	if res == "" {
		return nil
	}
	return json.Unmarshal([]byte(res), out)
}
