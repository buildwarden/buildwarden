package hyperv

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"encoding/xml"
	"fmt"
	"unicode/utf16"
)

// Windows build-guest seed generation for the Hyper-V driver.
//
// The build guest boots Microsoft's stock Windows Server evaluation VHD, which
// starts at OOBE with no warden agent installed. Unlike the qemu path (which
// bakes a startup Scheduled Task into the image during an unattended install),
// the Hyper-V path drives that stock image entirely from attached seed media:
//
//   - unattend.xml automates OOBE (no interactive screens, ephemeral admin,
//     one-shot autologon) and, via FirstLogonCommands, launches warden-run.ps1
//     off the WARDEN seed volume at first logon.
//   - warden-run.ps1 hands off to warden-io.exe, which configures the static
//     build-link network, installs the per-build CA fetched from the relay,
//     pulls build.ps1, runs it, and reports the exit code — the same runtime
//     contract the qemu Windows guest uses.
//
// These generators are pure (string + XML), so they are unit-tested on any
// platform. Packaging the staged files into attachable media is the
// platform-specific, boot-validated step in buildSeed (winseed_windows.go).

const (
	// Static build-link addressing; must match the relay VM's build NIC.
	buildGatewayIP = "10.0.0.1"
	buildGuestCIDR = "10.0.0.2/30"
	// wardenSeedLabel is the seed volume label warden-run.ps1 is found on.
	wardenSeedLabel = "WARDEN"
)

// unattendConfig parameterizes the generated OOBE unattend.xml.
type unattendConfig struct {
	// AdminPassword is the ephemeral built-in Administrator password, random
	// per build. It only ever exists on the per-build seed media and in the
	// throwaway guest; the build VM is torn down after the build.
	AdminPassword string
	// ProcArch is the unattend processorArchitecture ("amd64").
	ProcArch string
}

// generateWindowsUnattend renders the OOBE unattend.xml for the stock eval VHD.
// It runs only the oobeSystem pass (the image is already installed): auto-answer
// locale, set + autologon the built-in Administrator once, hide every OOBE
// screen, and run the warden bootstrap at first logon.
func generateWindowsUnattend(cfg unattendConfig) string {
	arch := cfg.ProcArch
	if arch == "" {
		arch = "amd64"
	}
	pw := xmlText(cfg.AdminPassword)
	cmd := xmlText(firstLogonCommand())

	return fmt.Sprintf(`<?xml version="1.0" encoding="utf-8"?>
<unattend xmlns="urn:schemas-microsoft-com:unattend" xmlns:wcm="http://schemas.microsoft.com/WMIConfig/2002/State">
  <settings pass="oobeSystem">
    <component name="Microsoft-Windows-International-Core" processorArchitecture="%[1]s" publicKeyToken="31bf3856ad364e35" language="neutral" versionScope="nonSxS">
      <InputLocale>en-US</InputLocale>
      <SystemLocale>en-US</SystemLocale>
      <UILanguage>en-US</UILanguage>
      <UserLocale>en-US</UserLocale>
    </component>
    <component name="Microsoft-Windows-Shell-Setup" processorArchitecture="%[1]s" publicKeyToken="31bf3856ad364e35" language="neutral" versionScope="nonSxS">
      <UserAccounts>
        <AdministratorPassword>
          <Value>%[2]s</Value>
          <PlainText>true</PlainText>
        </AdministratorPassword>
      </UserAccounts>
      <AutoLogon>
        <Enabled>true</Enabled>
        <LogonCount>1</LogonCount>
        <Username>Administrator</Username>
        <Password>
          <Value>%[2]s</Value>
          <PlainText>true</PlainText>
        </Password>
      </AutoLogon>
      <OOBE>
        <HideEULAPage>true</HideEULAPage>
        <HideOEMRegistrationScreen>true</HideOEMRegistrationScreen>
        <HideOnlineAccountScreens>true</HideOnlineAccountScreens>
        <HideLocalAccountScreen>true</HideLocalAccountScreen>
        <HideWirelessSetupInOOBE>true</HideWirelessSetupInOOBE>
        <NetworkLocation>Work</NetworkLocation>
        <ProtectYourPC>3</ProtectYourPC>
      </OOBE>
      <FirstLogonCommands>
        <SynchronousCommand wcm:action="add">
          <Order>1</Order>
          <CommandLine>%[3]s</CommandLine>
          <Description>warden build bootstrap</Description>
        </SynchronousCommand>
      </FirstLogonCommands>
      <TimeZone>UTC</TimeZone>
    </component>
  </settings>
</unattend>
`, arch, pw, cmd)
}

// firstLogonCommand is the single OOBE FirstLogonCommands entry. It must stay
// well under the 1024-char FirstLogonCommands limit (field reports show trouble
// past ~200), so the actual logic is a tiny drive-scan delivered as a base64
// -EncodedCommand: base64 is pure [A-Za-z0-9+/=], which sidesteps both XML
// escaping and cmd/PowerShell quoting entirely. The scan finds warden-run.ps1
// at the root of any filesystem drive (the WARDEN seed) and runs it.
func firstLogonCommand() string {
	scan := `gdr -PSProvider FileSystem|%{$p=$_.Root+'warden-run.ps1';if(Test-Path $p){& $p;break}}`
	return "powershell.exe -NoProfile -ExecutionPolicy Bypass -EncodedCommand " + psEncodedCommand(scan)
}

// generateWardenRunPS1 is the first-boot bootstrap launched by the OOBE
// FirstLogonCommand off the WARDEN seed. It mirrors the qemu Windows contract:
// copy warden-io.exe local, run `initialize` (static network, per-build CA from
// the relay, fetch + run build.ps1, report exit code), then power off so the
// host detects completion via the relay's /v1/complete.
func generateWardenRunPS1() string {
	return fmt.Sprintf(`# warden-run.ps1 - Hyper-V build-guest first-boot bootstrap.
# Launched by the OOBE FirstLogonCommand from the WARDEN seed volume.
$ErrorActionPreference = 'Continue'
$seed = $PSScriptRoot
New-Item -ItemType Directory -Force -Path 'C:\warden' | Out-Null
Copy-Item -Force (Join-Path $seed 'warden-io.exe') 'C:\warden\warden-io.exe'
$log = 'C:\warden\warden-run.log'
"warden-run start $(Get-Date -Format o)" | Out-File -FilePath $log -Encoding utf8
& 'C:\warden\warden-io.exe' initialize --gateway=%s --ip=%s *>> $log 2>&1
"warden-io exit=$LASTEXITCODE $(Get-Date -Format o)" | Out-File -FilePath $log -Append -Encoding utf8
Stop-Computer -Force
`, buildGatewayIP, buildGuestCIDR)
}

// xmlText escapes a string for safe inclusion as XML element text (the unattend
// is rendered by fmt, not an auto-escaping template; the admin password can
// contain '&', '<', or quotes, any of which would make Windows Setup reject the
// whole answer file).
func xmlText(s string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

// psEncodedCommand encodes a PowerShell script for powershell.exe
// -EncodedCommand (UTF-16LE, base64).
func psEncodedCommand(script string) string {
	u := utf16.Encode([]rune(script))
	b := make([]byte, len(u)*2)
	for i, r := range u {
		binary.LittleEndian.PutUint16(b[i*2:], r)
	}
	return base64.StdEncoding.EncodeToString(b)
}
