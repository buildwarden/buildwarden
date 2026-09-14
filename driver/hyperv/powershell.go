//go:build windows

package hyperv

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// runPS runs a PowerShell command and returns its stdout. On failure the error
// carries stderr (or the raw exec error) so callers surface a real message
// instead of a bare exit code.
func runPS(script string) (string, error) {
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script)
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(errb.String())
		if msg == "" {
			msg = err.Error()
		}
		return out.String(), fmt.Errorf("powershell: %s", msg)
	}
	return out.String(), nil
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

// psEscapeSingle escapes a value for a single-quoted PowerShell string literal.
func psEscapeSingle(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}
