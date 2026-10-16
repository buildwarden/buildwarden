package qemu

import (
	"bytes"
	_ "embed"
	"encoding/base64"
	"encoding/binary"
	"encoding/xml"
	"fmt"
	"strings"
	"text/template"
	"unicode/utf16"
)

//go:embed autounattend.xml.tmpl
var autounattendTemplate string

// autounattendConfig parameterizes the generated Autounattend.xml used to drive
// a fully unattended Windows install from ISO during image preparation.
//
// The firmware gate (TPM 2.0 / Secure Boot) is cleared with LabConfig bypass
// keys. A swtpm + Secure Boot path is a documented opt-in future feature (see
// tools/win-image-prep/README.md); when built, it would drop the bypass keys
// and enroll Secure Boot vars instead.
type autounattendConfig struct {
	// Arch is the Go arch of the target guest ("amd64" or "arm64").
	Arch string
	// Edition is the install.wim image name (e.g. "Windows 11 Pro",
	// "Windows 11 Enterprise Evaluation").
	Edition string
	// AdminUser / AdminPassword is the ephemeral local admin baked into the
	// prepared image for headless autologon. Rotated per prep run.
	AdminUser     string
	AdminPassword string
	// VirtioDriveLetter is where the virtio-win driver ISO is mounted during
	// setup (e.g. "E:").
	VirtioDriveLetter string
	// SeedLabel is the volume label of the per-build provisioning seed the
	// startup task looks for (see windows.go / windowsSeedName).
	SeedLabel string
}

// unattendArch maps a Go arch to the unattend processorArchitecture token.
func unattendArch(goarch string) string {
	if goarch == "arm64" {
		return "arm64"
	}
	return "amd64"
}

// virtioArchDir maps a Go arch to the virtio-win ISO's per-arch driver folder.
func virtioArchDir(goarch string) string {
	if goarch == "arm64" {
		return "ARM64"
	}
	return "amd64"
}

// labConfigBypassKeys are the registry values that let Win11 Setup proceed on
// firmware without TPM 2.0 / Secure Boot (and relaxed CPU/RAM/storage checks).
var labConfigBypassKeys = []string{
	"BypassTPMCheck",
	"BypassSecureBootCheck",
	"BypassRAMCheck",
	"BypassCPUCheck",
	"BypassStorageCheck",
}

// autounattendData is the template model.
type autounattendData struct {
	ProcArch      string
	Edition       string
	AdminUser     string
	AdminPassword string
	TaskCmd       string
	BypassKeys    []bypassEntry
	DriverPaths   []driverPathEntry
}

type bypassEntry struct {
	Order int
	Key   string
}

type driverPathEntry struct {
	Key  int
	Path string
}

var autounattendTmpl = template.Must(
	template.New("autounattend").Parse(autounattendTemplate))

// generateAutounattend renders the Autounattend.xml for the given config.
func generateAutounattend(cfg autounattendConfig) string {
	vArch := virtioArchDir(cfg.Arch)
	drv := strings.TrimRight(cfg.VirtioDriveLetter, `\`)

	data := autounattendData{
		ProcArch:      unattendArch(cfg.Arch),
		Edition:       xmlText(cfg.Edition),
		AdminUser:     xmlText(cfg.AdminUser),
		AdminPassword: xmlText(cfg.AdminPassword),
		TaskCmd:       xmlText(wardenRunTaskCmd(cfg.SeedLabel)),
	}
	for i, key := range labConfigBypassKeys {
		data.BypassKeys = append(data.BypassKeys,
			bypassEntry{Order: i + 1, Key: key})
	}
	// The OS disk is NVMe (in-box stornvme.sys), so no virtio block driver
	// (viostor/vioscsi) needs injecting. Inject only NetKVM so the installed
	// image carries the virtio-net driver for the build-time NIC that warden-io
	// configures (static IP) on first boot.
	for i, d := range []string{"NetKVM"} {
		data.DriverPaths = append(data.DriverPaths, driverPathEntry{
			Key:  i + 1,
			Path: xmlText(fmt.Sprintf(`%s\%s\w11\%s`, drv, d, vArch)),
		})
	}

	var sb strings.Builder
	// The template is compiled from a trusted embedded asset; execution only
	// fails on a template bug, which the tests catch.
	_ = autounattendTmpl.Execute(&sb, data)
	return sb.String()
}

// xmlText escapes a string for safe inclusion as XML element-text content.
// The Autounattend is rendered with text/template (no auto-escaping), and the
// FirstLogonCommands PowerShell contains a raw '&' (and quotes); left unescaped
// these produce invalid XML and Windows Setup fails to parse the answer file at
// PreFinalize ("Whitespace is not allowed at this location" / 0x80131501),
// which applies the image but never makes the disk bootable. Windows decodes
// the entities before executing the command, so the runtime string is intact.
func xmlText(s string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

// psEncodedCommand encodes a PowerShell script for powershell.exe
// -EncodedCommand (UTF-16LE, base64). This sidesteps every nested-quoting and
// XML-escaping hazard for a command embedded in the answer file: the payload
// is pure base64 ([A-Za-z0-9+/=]), all of which is XML- and shell-safe.
func psEncodedCommand(script string) string {
	u := utf16.Encode([]rune(script))
	b := make([]byte, len(u)*2)
	for i, r := range u {
		binary.LittleEndian.PutUint16(b[i*2:], r)
	}
	return base64.StdEncoding.EncodeToString(b)
}

// wardenRunTaskCmd is the PowerShell that first-logon runs to register the
// warden-run startup task. On every boot the task waits for the WARDEN seed
// volume (the second NVMe disk enumerates a few seconds after the -AtStartup
// trigger fires) and then runs its warden-run.ps1.
//
// LENGTH IS LOAD-BEARING. A FirstLogonCommands <CommandLine> is capped at 1024
// characters and misbehaves well before that; an over-long entry makes the
// oobeSystem pass fail with "Windows could not complete the installation"
// (a Setup error dialog that appears BEFORE FirstLogonCommands even runs, so
// the guest never reaches the Order-2 shutdown and prep times out). An earlier
// version blew past 1024 and reproducibly stalled two hands-off preps at
// exactly that dialog. This version is kept compact and
// TestFirstLogonCommandLineUnderLimit guards the rendered length.
//
// Hardening kept from the earlier failure analysis:
//   - Wait/retry loop for the seed volume (polls up to ~180s): at boot
//     Get-Volume returns nothing until the second NVMe seed enumerates.
//   - Battery settings: Register-ScheduledTask defaults
//     (DisallowStartIfOnBatteries/StopIfGoingOnBatteries) block the task in a
//     VM whose power state QEMU does not report as AC.
//   - The wait loop is delivered as a base64 -EncodedCommand so no quoting has
//     to survive XML + the FirstLogonCommands wrapper.
//
// Dropped as redundant to stay under the limit: the machine-scope
// Set-ExecutionPolicy (the task's own powershell already runs -ExecutionPolicy
// Bypass, under which `& x.ps1` runs) and -ExecutionTimeLimit.
func wardenRunTaskCmd(seedLabel string) string {
	bootstrap := fmt.Sprintf(
		"for($i=0;$i -lt 90;$i++){"+
			"$v=Get-Volume -FileSystemLabel '%s' -EA 0;"+
			"if($v.DriveLetter){&($v.DriveLetter+':\\warden-run.ps1');break};"+
			"Start-Sleep 2}",
		seedLabel)
	boot64 := psEncodedCommand(bootstrap)

	// Single quotes only and no '&', so it is XML-safe (xmlText escapes the
	// quotes; Windows decodes them before running it). The embedded wait loop
	// is base64, so it carries no quotes.
	return fmt.Sprintf(
		"$a=New-ScheduledTaskAction -Execute 'powershell.exe' "+
			"-Argument '-NoProfile -ExecutionPolicy Bypass -EncodedCommand %s';"+
			"$s=New-ScheduledTaskSettingsSet -AllowStartIfOnBatteries "+
			"-DontStopIfGoingOnBatteries;"+
			"$t=New-ScheduledTaskTrigger -AtStartup;"+
			"Register-ScheduledTask -TaskName warden-run -Action $a -Trigger $t "+
			"-Settings $s -User SYSTEM -RunLevel Highest -Force",
		boot64)
}
