//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// wardenCADir is where the relay CA is written on Windows guests. Tools that
// bypass the system trust store (conda, pip, openssl-based) read the PEM here
// via the SSL_CERT_FILE / REQUESTS_CA_BUNDLE / ... environment variables.
func wardenCADir() string {
	if pd := os.Getenv("ProgramData"); pd != "" {
		return filepath.Join(pd, "warden")
	}
	return `C:\ProgramData\warden`
}

func wardenCABundlePath() string {
	return filepath.Join(wardenCADir(), "warden-ca.pem")
}

// configureNetwork points the guest at the relay. Two modes:
//   - selfIP set (e.g. "10.0.0.2/30"): assign a static IP + default route via
//     the relay gateway and set DNS. Required by drivers that do not run a
//     DHCP server on the isolated link (e.g. the qemu two-VM topology).
//   - selfIP empty: rely on relay-provided DHCP for IP, best-effort set DNS.
// The build VM's sole NIC faces the relay, so operating on all interfaces is
// safe. Steps are best-effort and non-fatal individually so a partially
// pre-configured image still proceeds.
func configureNetwork(gateway, selfIP string) error {
	if gateway == "" {
		return nil // nothing to point at
	}

	var ps strings.Builder
	if selfIP != "" {
		ip, prefix := splitCIDR(selfIP)
		// Assign the static address and a default route via the gateway on
		// the first up, non-loopback IPv4 interface.
		fmt.Fprintf(&ps,
			"$if = Get-NetAdapter -Physical | "+
				"Where-Object Status -eq 'Up' | Select-Object -First 1; "+
				"if (-not $if) { $if = Get-NetAdapter | "+
				"Select-Object -First 1 }; "+
				"New-NetIPAddress -InterfaceIndex $if.ifIndex "+
				"-IPAddress '%s' -PrefixLength %s "+
				"-DefaultGateway '%s' -ErrorAction SilentlyContinue | Out-Null; "+
				"Set-DnsClientServerAddress -InterfaceIndex $if.ifIndex "+
				"-ServerAddresses '%s' -ErrorAction SilentlyContinue; ",
			ip, prefix, gateway, gateway)
	} else {
		fmt.Fprintf(&ps,
			"Get-DnsClientServerAddress -AddressFamily IPv4 | "+
				"ForEach-Object { Set-DnsClientServerAddress "+
				"-InterfaceIndex $_.InterfaceIndex "+
				"-ServerAddresses '%s' -ErrorAction SilentlyContinue }",
			gateway)
	}

	cmd := exec.Command("powershell.exe",
		"-NoProfile", "-NonInteractive", "-Command", ps.String())
	cmd.Stderr = os.Stderr
	_ = cmd.Run() // non-fatal: provisioning may also configure the NIC
	return nil
}

// splitCIDR splits "10.0.0.2/30" into ("10.0.0.2", "30"). A bare address
// without a prefix defaults to /24.
func splitCIDR(cidr string) (ip, prefix string) {
	if i := strings.IndexByte(cidr, '/'); i >= 0 {
		return cidr[:i], cidr[i+1:]
	}
	return cidr, "24"
}

// installCA writes the relay's per-build CA to a known path (for tools that
// read an explicit bundle) and imports it into the machine Root store via
// certutil so Schannel-based tooling trusts the MITM CA. The certutil step
// requires administrative rights; it is non-fatal because the explicit-bundle
// environment variables cover openssl-based tools regardless.
func installCA(pem []byte) error {
	dir := wardenCADir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating CA dir: %w", err)
	}

	bundle := wardenCABundlePath()
	if err := os.WriteFile(bundle, pem, 0644); err != nil {
		return fmt.Errorf("writing CA bundle: %w", err)
	}

	// Import into the LocalMachine\Root store.
	cmd := exec.Command("certutil.exe", "-addstore", "-f", "Root", bundle)
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr,
			"warden-io: certutil -addstore Root: %s (continuing)\n", err)
	}

	return nil
}

// findCABundle returns the path to the written CA bundle if it exists, for use
// as SSL_CERT_FILE and friends.
func findCABundle() string {
	if bundle := wardenCABundlePath(); fileExists(bundle) {
		return bundle
	}
	return ""
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// scriptCommand builds the command to execute the fetched build script,
// selected by extension: .ps1 via PowerShell, .cmd/.bat via cmd, and anything
// else defaulting to PowerShell.
func scriptCommand(path string) *exec.Cmd {
	switch {
	case strings.HasSuffix(strings.ToLower(path), ".cmd"),
		strings.HasSuffix(strings.ToLower(path), ".bat"):
		return exec.Command("cmd.exe", "/c", path)
	default:
		return exec.Command("powershell.exe",
			"-NoProfile", "-ExecutionPolicy", "Bypass", "-File", path)
	}
}

// defaultScriptName is the build script fetched from the relay when the caller
// does not specify one. Windows guests default to a PowerShell script.
func defaultScriptName() string { return "build.ps1" }
