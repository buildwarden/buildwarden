package qemu

import (
	"encoding/xml"
	"io"
	"strings"
	"testing"
)

// TestGenerateAutounattendWellFormed guards against the class of bug where a
// dynamic value (notably the FirstLogonCommands PowerShell, which contains a
// raw '&') is inserted unescaped and produces invalid XML that Windows Setup
// rejects at PreFinalize. Renders for both arches and parses every token.
func TestGenerateAutounattendWellFormed(t *testing.T) {
	for _, arch := range []string{"arm64", "amd64"} {
		doc := generateAutounattend(autounattendConfig{
			Arch:              arch,
			Edition:           "Windows 11 Pro",
			AdminUser:         "warden",
			AdminPassword:     "p@ss & <w0rd>", // specials must not break XML
			VirtioDriveLetter: "E:",
			SeedLabel:         windowsSeedName,
		})
		dec := xml.NewDecoder(strings.NewReader(doc))
		for {
			_, err := dec.Token()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatalf("%s: rendered Autounattend is not well-formed XML: %v", arch, err)
			}
		}
	}
}

func TestGenerateAutounattendARM64(t *testing.T) {
	xml := generateAutounattend(autounattendConfig{
		Arch:              "arm64",
		Edition:           "Windows 11 Pro",
		AdminUser:         "warden",
		AdminPassword:     "s3cret-ephemeral",
		VirtioDriveLetter: "E:",
		SeedLabel:         windowsSeedName,
	})

	// All firmware-bypass keys present.
	for _, key := range labConfigBypassKeys {
		if !strings.Contains(xml, key) {
			t.Errorf("autounattend missing LabConfig key %q", key)
		}
	}
	// ARM64 processor arch and virtio ARM64 driver folder.
	if !strings.Contains(xml, `processorArchitecture="arm64"`) {
		t.Error("autounattend missing arm64 processorArchitecture")
	}
	if !strings.Contains(xml, `E:\NetKVM\w11\ARM64`) {
		t.Error("autounattend missing arm64 virtio NetKVM driver path")
	}
	// Edition, admin, autologon, and the startup-task registration.
	for _, want := range []string{
		"<Value>Windows 11 Pro</Value>",
		"<Name>warden</Name>",
		"<Enabled>true</Enabled>", // AutoLogon
		"s3cret-ephemeral",
		"Register-ScheduledTask",
		"warden-run",
		"AllowStartIfOnBatteries", // battery-proof settings
		"EncodedCommand",          // wait-loop delivered as base64
	} {
		if !strings.Contains(xml, want) {
			t.Errorf("autounattend missing %q", want)
		}
	}
	// Headless OOBE.
	if !strings.Contains(xml, "<HideEULAPage>true</HideEULAPage>") {
		t.Error("autounattend does not hide the OOBE EULA page")
	}
}

func TestGenerateAutounattendAMD64(t *testing.T) {
	xml := generateAutounattend(autounattendConfig{
		Arch:              "amd64",
		Edition:           "Windows 11 Enterprise Evaluation",
		AdminUser:         "warden",
		AdminPassword:     "x",
		VirtioDriveLetter: "E:",
		SeedLabel:         windowsSeedName,
	})
	if !strings.Contains(xml, `processorArchitecture="amd64"`) {
		t.Error("autounattend missing amd64 processorArchitecture")
	}
	if !strings.Contains(xml, `E:\NetKVM\w11\amd64`) {
		t.Error("autounattend missing amd64 NetKVM driver path")
	}
	// NVMe OS disk => no virtio block driver injection.
	if strings.Contains(xml, "viostor") || strings.Contains(xml, "vioscsi") {
		t.Error("autounattend should not inject viostor/vioscsi (NVMe OS disk)")
	}
	if strings.Contains(xml, "ARM64") {
		t.Error("amd64 autounattend should not reference ARM64 driver dirs")
	}
}

func TestUnattendArchMapping(t *testing.T) {
	if unattendArch("arm64") != "arm64" || unattendArch("amd64") != "amd64" {
		t.Error("unattendArch mapping wrong")
	}
	if unattendArch("386") != "amd64" {
		t.Error("unattendArch should default non-arm64 to amd64")
	}
	if virtioArchDir("arm64") != "ARM64" || virtioArchDir("amd64") != "amd64" {
		t.Error("virtioArchDir mapping wrong")
	}
}

// TestFirstLogonCommandLineUnderLimit guards the FirstLogonCommands CommandLine
// against Windows' 1024-char cap. An over-long entry makes the oobeSystem pass
// fail with "Windows could not complete the installation" (a Setup error that
// appears before the commands run), which reproducibly stalled hands-off preps.
// The xml decoder unescapes entities, so len() here is the length Windows sees.
func TestFirstLogonCommandLineUnderLimit(t *testing.T) {
	doc := generateAutounattend(autounattendConfig{
		Arch:              "arm64",
		Edition:           "Windows 11 Pro",
		AdminUser:         "warden",
		AdminPassword:     "s3cret-ephemeral",
		VirtioDriveLetter: "E:",
		SeedLabel:         windowsSeedName,
	})
	dec := xml.NewDecoder(strings.NewReader(doc))
	var inCmd bool
	var cur strings.Builder
	var cmds []string
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		switch e := tok.(type) {
		case xml.StartElement:
			if e.Name.Local == "CommandLine" {
				inCmd = true
				cur.Reset()
			}
		case xml.CharData:
			if inCmd {
				cur.Write(e)
			}
		case xml.EndElement:
			if e.Name.Local == "CommandLine" {
				cmds = append(cmds, cur.String())
				inCmd = false
			}
		}
	}
	var found bool
	for _, c := range cmds {
		if !strings.Contains(c, "Register-ScheduledTask") {
			continue
		}
		found = true
		if len(c) >= 1024 {
			t.Fatalf("FirstLogonCommands CommandLine is %d chars, exceeds Windows' 1024 cap", len(c))
		}
		if len(c) > 900 {
			t.Errorf("FirstLogonCommands CommandLine is %d chars; keep well under 1024 (target <900)", len(c))
		}
		t.Logf("FirstLogonCommands CommandLine length = %d chars (limit 1024)", len(c))
	}
	if !found {
		t.Fatal("no FirstLogonCommands CommandLine with Register-ScheduledTask found")
	}
}
