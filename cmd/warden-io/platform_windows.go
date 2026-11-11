//go:build windows

package main

import (
	"crypto/x509"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/buildwarden/buildwarden/bootstrap"
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
//   - selfIP set (e.g. "192.0.2.2/30"): assign a static IP + default route via
//     the relay gateway and set DNS. This is for an orchestrator that passes
//     --ip; warden's own launcher (warden-run.ps1) does not.
//   - selfIP empty: rely on relay-provided DHCP for IP, best-effort set DNS.
//     The NIC keeps the lease the vm-mode relay serves on the build link.
//
// The build VM's sole NIC faces the relay, so operating on all interfaces is
// safe. Steps are best-effort and non-fatal individually so a partially
// pre-configured image still proceeds.
func configureNetwork(gateway, selfIP string) error {
	if gateway == "" {
		return nil // nothing to point at
	}

	// The relay serves its control endpoints on the magic hostnames
	// "artifacts" (ca.pem, heartbeat, exit, artifact POST) and "cwd"
	// (build script + context). On Linux the container gets these via
	// `--add-host artifacts:<relay>`; Windows has no such mechanism, so map
	// them in the hosts file to the relay/gateway IP. The relay VM's iptables
	// then transparently redirects the resulting port-80 traffic to the relay
	// process. Without this, warden-io fails with "lookup artifacts: no such
	// host" before it can fetch the CA or the build script.
	if err := writeRelayHostsEntries(gateway); err != nil {
		fmt.Fprintf(os.Stderr,
			"warden-io: hosts entries: %s (continuing)\n", err)
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

// splitCIDR splits "192.0.2.2/30" into ("192.0.2.2", "30"). A bare address
// without a prefix defaults to /24.
func splitCIDR(cidr string) (ip, prefix string) {
	if i := strings.IndexByte(cidr, '/'); i >= 0 {
		return cidr[:i], cidr[i+1:]
	}
	return cidr, "24"
}

// relayHostnames are the magic hosts the relay serves the build agent on.
// warden-io reaches them over HTTP (port 80), which the relay VM's iptables
// transparently redirects to the relay process.
var relayHostnames = []string{"artifacts", "cwd"}

// hostsFilePath returns the Windows hosts file path.
func hostsFilePath() string {
	root := os.Getenv("SystemRoot")
	if root == "" {
		root = `C:\Windows`
	}
	return filepath.Join(root, "System32", "drivers", "etc", "hosts")
}

// writeRelayHostsEntries maps the relay magic hostnames to the relay/gateway
// IP in the Windows hosts file. It rewrites its own managed lines each call
// (idempotent) and preserves all other content. Requires admin (the boot task
// runs as SYSTEM).
func writeRelayHostsEntries(gateway string) error {
	const marker = "# warden-io"
	path := hostsFilePath()

	existing, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	var b strings.Builder
	for _, line := range strings.Split(string(existing), "\n") {
		if strings.Contains(line, marker) {
			continue // drop a prior managed entry
		}
		b.WriteString(line)
		b.WriteString("\n")
	}
	for _, name := range relayHostnames {
		fmt.Fprintf(&b, "%s\t%s %s\n", gateway, name, marker)
	}
	return os.WriteFile(path, []byte(b.String()), 0644)
}

// wardenDataDir holds the files warden-io derives for the build (the Java
// truststore, the shell environment).
func wardenDataDir() string { return wardenCADir() }

// bazelSystemRC is the system-wide bazelrc Bazel reads on Windows.
func bazelSystemRC() string {
	return filepath.Join(filepath.Dir(wardenCADir()), "bazel.bazelrc")
}

// installCA writes the relay's per-build CA to a known path (for tools that
// read an explicit bundle) and imports it into the machine Root store via
// certutil so Schannel-based tooling trusts the MITM CA. The certutil step
// requires administrative rights; it is non-fatal because the explicit-bundle
// environment variables cover openssl-based tools regardless.
func installCA(pem []byte, _ *x509.Certificate, mode string) error {
	if mode == bootstrap.TrustExclusive {
		logStep("exclusive trust is not supported on Windows; adding the CA instead")
	}
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

// nicAddr is one IPv4 address on a guest interface and how Windows got it
// (Get-NetIPAddress's PrefixOrigin: Dhcp, Manual, WellKnown, ...).
type nicAddr struct {
	alias, ip, origin string
}

// relayWaitDiagnostic names the guest's IPv4 addresses and their origin, so
// a failed relay wait on a NIC that got no lease from the relay says so.
func relayWaitDiagnostic() string {
	out, err := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive",
		"-Command", "Get-NetIPAddress -AddressFamily IPv4 | ForEach-Object { "+
			"'{0}{3}{1}{3}{2}' -f $_.InterfaceAlias, $_.IPAddress, $_.PrefixOrigin, [char]9 }").Output()
	if err != nil {
		return fmt.Sprintf("IPv4 addresses unavailable: %s", err)
	}
	return renderNICAddrs(parseNICAddrs(string(out)))
}

// parseNICAddrs reads relayWaitDiagnostic's tab-separated query output,
// skipping loopback addresses and malformed lines.
func parseNICAddrs(out string) []nicAddr {
	var addrs []nicAddr
	for _, line := range strings.Split(out, "\n") {
		f := strings.Split(strings.TrimRight(line, "\r"), "\t")
		if len(f) != 3 || strings.HasPrefix(f[1], "127.") {
			continue
		}
		addrs = append(addrs, nicAddr{alias: f[0], ip: f[1], origin: f[2]})
	}
	return addrs
}

// renderNICAddrs renders the addresses for a failed relay wait. A WellKnown
// 169.254 address is Windows' self-assigned one: the NIC got no lease.
func renderNICAddrs(addrs []nicAddr) string {
	if len(addrs) == 0 {
		return "no IPv4 address on any interface"
	}
	parts := make([]string, 0, len(addrs))
	for _, a := range addrs {
		origin := a.origin
		if origin == "WellKnown" && strings.HasPrefix(a.ip, "169.254.") {
			origin += ", no DHCP lease from the relay"
		}
		parts = append(parts, fmt.Sprintf("%s has %s (%s)", a.alias, a.ip, origin))
	}
	return strings.Join(parts, "; ")
}
