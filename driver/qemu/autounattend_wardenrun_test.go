package qemu

import (
	"encoding/base64"
	"encoding/binary"
	"strings"
	"testing"
)

// decodePSEncoded reverses psEncodedCommand (base64 UTF-16LE -> string).
func decodePSEncoded(b64 string) string {
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return ""
	}
	u := make([]uint16, len(raw)/2)
	for i := range u {
		u[i] = binary.LittleEndian.Uint16(raw[i*2:])
	}
	return string(utf16Decode(u))
}

func utf16Decode(u []uint16) []rune {
	var out []rune
	for _, c := range u {
		out = append(out, rune(c))
	}
	return out
}

// TestWardenRunTaskCmdWaitLoop verifies the registered task carries a
// base64-encoded wait-for-volume loop that references the seed label and runs
// warden-run.ps1, plus the battery-proof settings and execution-policy fix.
func TestWardenRunTaskCmdWaitLoop(t *testing.T) {
	cmd := wardenRunTaskCmd(windowsSeedName)

	for _, want := range []string{
		"Register-ScheduledTask",
		"-TaskName warden-run",
		"AllowStartIfOnBatteries",
		"DontStopIfGoingOnBatteries",
		"Set-ExecutionPolicy Bypass -Scope LocalMachine",
		"-EncodedCommand ",
	} {
		if !strings.Contains(cmd, want) {
			t.Errorf("task registration missing %q", want)
		}
	}

	// The registration script must be XML/quote-safe: no double quotes and no
	// raw ampersand (the historical answer-file XML break).
	if strings.Contains(cmd, `"`) {
		t.Error("task registration should contain no double quotes")
	}
	if strings.Contains(cmd, "&") {
		t.Error("task registration should contain no raw ampersand")
	}

	// Decode the -EncodedCommand payload and confirm the wait loop.
	i := strings.Index(cmd, "-EncodedCommand ")
	if i < 0 {
		t.Fatal("no EncodedCommand in task registration")
	}
	rest := cmd[i+len("-EncodedCommand "):]
	b64 := rest
	if j := strings.IndexByte(rest, '\''); j >= 0 {
		b64 = rest[:j] // trim the closing single quote of -Argument '...'
	}
	boot := decodePSEncoded(strings.TrimSpace(b64))
	for _, want := range []string{
		"Get-Volume -FileSystemLabel '" + windowsSeedName + "'",
		"warden-run.ps1",
		"Start-Sleep",
		"DriveLetter",
	} {
		if !strings.Contains(boot, want) {
			t.Errorf("decoded wait loop missing %q; got: %s", want, boot)
		}
	}
}
