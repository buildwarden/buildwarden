// +build linux

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

func configureNetwork(gateway, selfIP string) error {
	if gateway == "" {
		return nil
	}

	// Write resolv.conf pointing DNS at the relay gateway
	resolv := fmt.Sprintf("nameserver %s\n", gateway)
	if err := os.WriteFile("/etc/resolv.conf", []byte(resolv), 0644); err != nil {
		return fmt.Errorf("writing resolv.conf: %w", err)
	}

	return nil
}

func installCA(pem []byte) error {
	// Write to system CA directory
	dir := "/usr/local/share/ca-certificates"
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating CA dir: %w", err)
	}

	certPath := filepath.Join(dir, "warden-ca.crt")
	if err := os.WriteFile(certPath, pem, 0644); err != nil {
		return fmt.Errorf("writing CA cert: %w", err)
	}

	// Try update-ca-certificates (Debian/Ubuntu/Alpine)
	if _, err := exec.LookPath("update-ca-certificates"); err == nil {
		cmd := exec.Command("update-ca-certificates")
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			// Non-fatal: fall through to manual append
			fmt.Fprintf(os.Stderr,
				"warden-io: update-ca-certificates: %s (continuing)\n", err)
		} else {
			return nil
		}
	}

	// Fallback: append to bundle file directly
	for _, bundle := range []string{
		"/etc/ssl/certs/ca-certificates.crt",
		"/etc/pki/tls/certs/ca-bundle.crt",
		"/etc/ssl/cert.pem",
	} {
		f, err := os.OpenFile(bundle, os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			continue
		}
		_, _ = f.Write([]byte("\n"))
		_, _ = f.Write(pem)
		f.Close()
		return nil
	}

	return fmt.Errorf("no writable CA bundle found")
}
