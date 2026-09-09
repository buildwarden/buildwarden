//go:build windows

package main

import (
	"strings"
	"testing"
)

func TestScriptCommandRouting(t *testing.T) {
	tests := []struct {
		path     string
		wantExe  string // substring expected in Args[0]
		wantFlag string // substring expected somewhere in Args
	}{
		{`C:\warden\build.ps1`, "powershell", "-File"},
		{`C:\warden\build.cmd`, "cmd", "/c"},
		{`C:\warden\build.bat`, "cmd", "/c"},
		{`C:\warden\BUILD.PS1`, "powershell", "-File"}, // case-insensitive
		{`C:\warden\build`, "powershell", "-File"},     // default
	}

	for _, tt := range tests {
		cmd := scriptCommand(tt.path)
		if !strings.Contains(strings.ToLower(cmd.Args[0]), tt.wantExe) {
			t.Errorf("scriptCommand(%q): Args[0] = %q, want substring %q",
				tt.path, cmd.Args[0], tt.wantExe)
		}
		joined := strings.Join(cmd.Args, " ")
		if !strings.Contains(joined, tt.wantFlag) {
			t.Errorf("scriptCommand(%q): Args = %v, want flag %q",
				tt.path, cmd.Args, tt.wantFlag)
		}
		found := false
		for _, a := range cmd.Args {
			if a == tt.path {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("scriptCommand(%q): script path not in Args %v",
				tt.path, cmd.Args)
		}
	}
}

func TestWardenCABundlePath(t *testing.T) {
	t.Setenv("ProgramData", `C:\ProgramData`)
	got := wardenCABundlePath()
	if !strings.HasSuffix(got, `warden-ca.pem`) {
		t.Errorf("wardenCABundlePath() = %q, want suffix warden-ca.pem", got)
	}
	if !strings.Contains(got, "warden") {
		t.Errorf("wardenCABundlePath() = %q, want it under a warden dir", got)
	}
}
