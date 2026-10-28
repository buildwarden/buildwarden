//go:build linux

package main

import (
	"crypto/x509"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/buildwarden/buildwarden/bootstrap"
)

// scriptCommand builds the command to execute the fetched build script,
// preferring bash when available and falling back to POSIX sh.
func scriptCommand(path string) *exec.Cmd {
	shell := "/bin/sh"
	if _, err := os.Stat("/bin/bash"); err == nil {
		shell = "/bin/bash"
	}
	return exec.Command(shell, path)
}

// findCABundle returns the path to the system CA bundle for use as
// SSL_CERT_FILE and friends.
func findCABundle() string {
	for _, path := range []string{
		"/etc/ssl/certs/ca-certificates.crt",
		"/etc/pki/tls/certs/ca-bundle.crt",
		"/etc/ssl/cert.pem",
	} {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return ""
}

// defaultScriptName is the build script fetched from the relay when the caller
// does not specify one.
func defaultScriptName() string { return "build.sh" }

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

// wardenDataDir holds the files warden-io derives for the build (the Java
// truststore, the shell environment).
func wardenDataDir() string { return "/etc/warden" }

// bazelSystemRC is the system-wide bazelrc Bazel reads on Linux.
func bazelSystemRC() string { return "/etc/bazel.bazelrc" }

// installCA installs the relay CA as a system root: exclusively (the only
// root) or appended to the existing ones. See bootstrap.Trust*.
func installCA(pem []byte, cert *x509.Certificate, mode string) error {
	if err := os.MkdirAll(filepath.Dir(wardenCAPath), 0755); err != nil {
		return fmt.Errorf("creating CA dir: %w", err)
	}
	if err := os.WriteFile(wardenCAPath, pem, 0644); err != nil {
		return fmt.Errorf("writing CA cert: %w", err)
	}
	if mode == bootstrap.TrustExclusive {
		return installExclusiveCA(pem, cert)
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
