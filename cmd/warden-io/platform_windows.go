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

// configureNetwork points DNS at the relay gateway. The build VM's sole NIC
// faces the relay, so DHCP served by the relay is the primary path and
// supplies both IP and DNS; this is a best-effort, non-fatal supplement for
// images that do not pick up DNS from DHCP.
func configureNetwork(gateway, selfIP string) error {
	if gateway == "" {
		return nil // relay-provided DHCP supplies IP + DNS
	}

	ps := fmt.Sprintf(
		"Get-DnsClientServerAddress -AddressFamily IPv4 | "+
			"ForEach-Object { Set-DnsClientServerAddress "+
			"-InterfaceIndex $_.InterfaceIndex "+
			"-ServerAddresses '%s' -ErrorAction SilentlyContinue }",
		gateway)
	cmd := exec.Command("powershell.exe",
		"-NoProfile", "-NonInteractive", "-Command", ps)
	cmd.Stderr = os.Stderr
	_ = cmd.Run() // non-fatal: DHCP from the relay is the primary path
	return nil
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
