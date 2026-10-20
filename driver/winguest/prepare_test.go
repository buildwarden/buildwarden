package winguest

import (
	"encoding/xml"
	"io"
	"strings"
	"testing"
)

func TestOOBEUnattend(t *testing.T) {
	doc := OOBEUnattend(OOBEConfig{
		AdminPassword:     `p&w"<x>`,
		FirstLogonCommand: SeedScriptCommand(PrepareScriptName),
		Description:       "warden prepare",
	})
	dec := xml.NewDecoder(strings.NewReader(doc))
	var cmd string
	inCmd := false
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("answer file is not well-formed: %v", err)
		}
		switch e := tok.(type) {
		case xml.StartElement:
			inCmd = e.Name.Local == "CommandLine"
		case xml.CharData:
			if inCmd {
				cmd += string(e)
			}
		case xml.EndElement:
			inCmd = false
		}
	}
	for _, want := range []string{
		`pass="oobeSystem"`, "<LogonCount>1</LogonCount>",
		"<Username>Administrator</Username>", "p&amp;w", `processorArchitecture="amd64"`,
	} {
		if !strings.Contains(doc, want) {
			t.Errorf("answer file missing %q", want)
		}
	}
	if len(cmd) >= FirstLogonCommandLimit {
		t.Errorf("FirstLogonCommand is %d chars; must stay under %d",
			len(cmd), FirstLogonCommandLimit)
	}
}

func TestSeedScriptCommand(t *testing.T) {
	const prefix = "powershell.exe -NoProfile -ExecutionPolicy Bypass -EncodedCommand "
	cmd := SeedScriptCommand("warden-run.ps1")
	if !strings.HasPrefix(cmd, prefix) {
		t.Fatalf("unexpected prefix: %q", cmd)
	}
	scan := decodeEncoded(t, strings.TrimPrefix(cmd, prefix))
	if !strings.Contains(scan, "$_.Root+'warden-run.ps1'") {
		t.Errorf("scan does not look for warden-run.ps1: %q", scan)
	}
}

func TestPrepareScript(t *testing.T) {
	ps := PrepareScript()
	if strings.Contains(ps, "%!") {
		t.Fatalf("template formatting error in prepare script:\n%s", ps)
	}
	// The host reads exactly these names; the script must write them.
	for _, name := range []string{
		KVPState, KVPGraceMinutes, KVPLicenseStatus, KVPLicenseChannel, KVPReportedAt,
	} {
		if !strings.Contains(ps, "'"+name+"'") {
			t.Errorf("prepare script never reports %s", name)
		}
	}
	for _, want := range []string{
		`HKLM:\SOFTWARE\Microsoft\Virtual Machine\Guest`,
		"\"running:$name\"", "'done'", "error:",
		"Register-ScheduledTask -TaskName warden-run",
		"Uninstall-WindowsFeature -Name Windows-Defender",
		"warden-prepare-continue",
		"Stop-Computer -Force",
	} {
		if !strings.Contains(ps, want) {
			t.Errorf("prepare script missing %q", want)
		}
	}
	// New-Item -Force on the existing key would wipe earlier reports.
	if strings.Contains(ps, "New-Item -Path $kvp -Force") {
		t.Error("Report recreates the KVP key, dropping earlier values")
	}
	if !strings.HasPrefix(StateRunningPref, "running:") ||
		!strings.Contains(ps, StateRunningPref) {
		t.Error("running state prefix out of sync with the script")
	}
}
