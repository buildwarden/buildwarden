package hyperv

import (
	"strings"
	"testing"
)

func validRelaySpec() RelayVMSpec {
	return RelayVMSpec{
		Name:        "warden-relay-abc",
		BootVHDX:    `C:\warden\relay-boot.vhdx`,
		OverlayVHDX: `C:\warden\builds\abc\relay.vhdx`,
		BuildSwitch: "warden-dev",
		NATSwitch:   "warden-dev-nat",
		COMPipePath: `\\.\pipe\warden-relay-abc`,
		SeedISO:     `C:\warden\builds\abc\relay-seed.iso`,
		Notes:       "warden-relay-ip=192.168.240.2 warden-relay-switch=warden-dev-0",
	}
}

func TestBuildRelayVMScript_SafetyCriticalProperties(t *testing.T) {
	script, err := buildRelayVMScript(validRelaySpec())
	if err != nil {
		t.Fatalf("buildRelayVMScript: %v", err)
	}

	// Secure Boot MUST be off for the unsigned UKI, and NO template may be set
	// (a template implies Secure Boot on and would refuse the unsigned image).
	if !strings.Contains(script, "-EnableSecureBoot Off") {
		t.Error("script must disable Secure Boot for the unsigned UKI")
	}
	if strings.Contains(script, "SecureBootTemplate") {
		t.Error("script must NOT set a SecureBootTemplate on the relay VM")
	}

	// Boots a per-build differencing overlay off the static base, never the base.
	if !strings.Contains(script,
		"New-VHD -Path 'C:\\warden\\builds\\abc\\relay.vhdx' "+
			"-ParentPath 'C:\\warden\\relay-boot.vhdx' -Differencing") {
		t.Errorf("script must create a differencing overlay off the boot VHDX; got:\n%s",
			script)
	}
	if !strings.Contains(script, "-VHDPath 'C:\\warden\\builds\\abc\\relay.vhdx'") {
		t.Error("VM must boot the overlay, not the base VHDX")
	}

	// COM1 wired to the host pipe.
	if !strings.Contains(script,
		`Set-VMComPort -VMName 'warden-relay-abc' -Number 1 `+
			`-Path '\\.\pipe\warden-relay-abc'`) {
		t.Error("script must wire COM1 to the host named pipe")
	}
	// Boot device set explicitly.
	if !strings.Contains(script, "-FirstBootDevice $drive") {
		t.Error("script must set the overlay as the first boot device")
	}
}

func TestBuildRelayVMScript_TwoNICsInOrder(t *testing.T) {
	script, err := buildRelayVMScript(validRelaySpec())
	if err != nil {
		t.Fatalf("buildRelayVMScript: %v", err)
	}
	// NIC1 (build link) attaches via New-VM -SwitchName; NIC2 (NAT) via
	// Add-VMNetworkAdapter. Order matters: the guest maps eth0->build, eth1->NAT.
	buildIdx := strings.Index(script,
		"New-VM -Name 'warden-relay-abc' -Generation 2 "+
			"-MemoryStartupBytes 512MB "+
			"-VHDPath 'C:\\warden\\builds\\abc\\relay.vhdx' "+
			"-SwitchName 'warden-dev'")
	natIdx := strings.Index(script,
		"Add-VMNetworkAdapter -VMName 'warden-relay-abc' "+
			"-SwitchName 'warden-dev-nat'")
	if buildIdx < 0 {
		t.Fatal("build-link NIC (New-VM -SwitchName) not found")
	}
	if natIdx < 0 {
		t.Fatal("NAT NIC (Add-VMNetworkAdapter) not found")
	}
	if buildIdx > natIdx {
		t.Error("build-link NIC must be attached before the NAT NIC (eth0 before eth1)")
	}
}

func TestBuildRelayVMScript_Defaults(t *testing.T) {
	script, err := buildRelayVMScript(validRelaySpec())
	if err != nil {
		t.Fatalf("buildRelayVMScript: %v", err)
	}
	if !strings.Contains(script, "-MemoryStartupBytes 512MB") {
		t.Error("default memory should be 512MB")
	}
	if !strings.Contains(script, "-Count 2") {
		t.Error("default CPU count should be 2")
	}
}

func TestBuildRelayVMScript_Validation(t *testing.T) {
	cases := map[string]func(*RelayVMSpec){
		"name":    func(s *RelayVMSpec) { s.Name = "" },
		"boot":    func(s *RelayVMSpec) { s.BootVHDX = "" },
		"overlay": func(s *RelayVMSpec) { s.OverlayVHDX = "" },
		"build":   func(s *RelayVMSpec) { s.BuildSwitch = "" },
		"nat":     func(s *RelayVMSpec) { s.NATSwitch = "" },
		"com":     func(s *RelayVMSpec) { s.COMPipePath = "" },
		"seed":    func(s *RelayVMSpec) { s.SeedISO = "" },
		"notes":   func(s *RelayVMSpec) { s.Notes = "" },
	}
	for name, mutate := range cases {
		spec := validRelaySpec()
		mutate(&spec)
		if _, err := buildRelayVMScript(spec); err == nil {
			t.Errorf("expected error when %s is empty", name)
		}
	}
}

func TestBuildRelayVMScript_EscapesQuotes(t *testing.T) {
	spec := validRelaySpec()
	spec.Name = "warden'relay"
	script, err := buildRelayVMScript(spec)
	if err != nil {
		t.Fatalf("buildRelayVMScript: %v", err)
	}
	if !strings.Contains(script, "warden''relay") {
		t.Error("single quotes in the name must be doubled for PowerShell")
	}
}

// TestBuildRelayVMScript_SeedAndNotes: the seed DVD and the Notes are set
// while the VM is off (before any Start-VM, which the script never runs),
// escaped, and the build NIC is on the given switch.
func TestBuildRelayVMScript_SeedAndNotes(t *testing.T) {
	spec := validRelaySpec()
	spec.SeedISO = `C:\it's\relay-seed.iso`
	spec.Notes = "warden-relay-ip=10.0.0.2 warden-relay-switch=it's-0"
	spec.BuildSwitch = "warden-dev-3"
	script, err := buildRelayVMScript(spec)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`Add-VMDvdDrive -VMName 'warden-relay-abc' -Path 'C:\it''s\relay-seed.iso'`,
		`Set-VM -Name 'warden-relay-abc' -Notes 'warden-relay-ip=10.0.0.2 ` +
			`warden-relay-switch=it''s-0'`,
		`-SwitchName 'warden-dev-3' | Out-Null`,
	} {
		if !strings.Contains(script, want) {
			t.Errorf("script lacks %q:\n%s", want, script)
		}
	}
	if strings.Contains(script, "Start-VM") {
		t.Error("the relay VM script starts the VM")
	}
}
