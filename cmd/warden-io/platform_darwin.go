//go:build darwin

package main

import (
	"fmt"
	"os"
	"os/exec"
)

func configureNetwork(gateway, selfIP string) error {
	if gateway == "" {
		return nil
	}

	// The boot script (warden-boot) already configures static IP, route,
	// and DNS before launching warden-io. This function supplements any
	// missing configuration but doesn't duplicate what's already done.

	// Ensure /etc/resolver entries exist (some tools bypass mDNSResponder)
	if err := os.MkdirAll("/etc/resolver", 0755); err == nil {
		res := fmt.Sprintf("nameserver %s\n", gateway)
		resB := []byte(res)
		os.WriteFile("/etc/resolver/artifacts", resB, 0644) //nolint:errcheck
		os.WriteFile("/etc/resolver/cwd", resB, 0644)       //nolint:errcheck
	}

	// Ensure resolv.conf points to relay
	resolv := fmt.Sprintf("nameserver %s\n", gateway)
	os.WriteFile("/etc/resolv.conf", []byte(resolv), 0644) //nolint:errcheck

	// Poke scutil for any late-starting mDNSResponder
	script := fmt.Sprintf(`d.init
d.add ServerAddresses * %s
d.add SupplementalMatchDomains * ""
set State:/Network/Service/warden/DNS
`, gateway)

	cmd := exec.Command("scutil")
	pipe, err := cmd.StdinPipe()
	if err != nil {
		return nil
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return nil
	}
	pipe.Write([]byte(script)) //nolint:errcheck
	pipe.Close()
	cmd.Wait() //nolint:errcheck

	return nil
}

func installCA(pem []byte) error {
	tmpFile := "/tmp/warden-ca.pem"
	if err := os.WriteFile(tmpFile, pem, 0644); err != nil {
		return fmt.Errorf("writing temp CA: %w", err)
	}

	// Try adding to System keychain (requires root)
	cmd := exec.Command("security", "add-trusted-cert",
		"-d", "-r", "trustRoot",
		"-k", "/Library/Keychains/System.keychain",
		tmpFile)
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr,
			"warden-io: keychain add failed (expected): %s\n", err)
	}

	// Always create/update combined bundle at /etc/ssl/cert.pem
	bundlePath := "/etc/ssl/cert.pem"
	existing, err := os.ReadFile(bundlePath)
	if err != nil {
		existing = nil
	}

	combined := append(existing, '\n')
	combined = append(combined, pem...)

	if err := os.WriteFile(bundlePath, combined, 0644); err != nil {
		return fmt.Errorf("writing CA bundle: %w", err)
	}

	os.Remove(tmpFile)
	return nil
}
