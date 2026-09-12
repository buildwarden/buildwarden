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
		windowsSeedName, // seed volume label referenced by the task
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
	if !strings.Contains(xml, `E:\viostor\w11\amd64`) {
		t.Error("autounattend missing amd64 virtio viostor driver path")
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
