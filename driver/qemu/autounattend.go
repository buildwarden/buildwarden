package qemu

import (
	"bytes"
	_ "embed"
	"encoding/xml"
	"fmt"
	"strings"
	"text/template"
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

// wardenRunTaskCmd is the PowerShell that first-logon runs to register a
// startup Scheduled Task, which on every boot finds the seed volume and runs
// its warden-run.ps1.
func wardenRunTaskCmd(seedLabel string) string {
	arg := fmt.Sprintf(
		`-NoProfile -ExecutionPolicy Bypass -Command `+
			`\"$v = Get-Volume -FileSystemLabel %s; `+
			`& ($v.DriveLetter + \":\\warden-run.ps1\")\"`,
		seedLabel)
	return `$a = New-ScheduledTaskAction -Execute 'powershell.exe' ` +
		`-Argument '` + arg + `'; ` +
		`$t = New-ScheduledTaskTrigger -AtStartup; ` +
		`Register-ScheduledTask -TaskName warden-run -Action $a ` +
		`-Trigger $t -User SYSTEM -RunLevel Highest -Force`
}
