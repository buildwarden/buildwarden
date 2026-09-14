//go:build windows

package hyperv

import (
	"os/exec"
	"strings"

	"golang.org/x/sys/windows"
)

// detectPlatform fills c with the current process/host capabilities. Layer 1 is
// side-effect-free token/SID inspection; the PowerShell probes are read-only.
// (Layer 2, mapping a live ERROR_ACCESS_DENIED to the same remediation, belongs
// in the operation path and is added with the privileged ops.)
func detectPlatform(c *Capabilities) {
	c.Supported = true

	// Full Administrator (elevation) from the current process token.
	tok := windows.GetCurrentProcessToken()
	c.Elevated = tok.IsElevated()
	if c.Elevated {
		c.TokenScope = "current process (elevated)"
	} else {
		c.TokenScope = "current process (non-elevated)"
	}

	// Hyper-V Administrators membership (well-known SID S-1-5-32-578). Pass a
	// NULL token so CheckTokenMembership uses the calling thread's token,
	// duplicating the primary token to an impersonation token as the API
	// requires. On a UAC-filtered (non-elevated) token the group reads as
	// not-a-member, which correctly reflects what the process can actually do.
	if sid, err := windows.CreateWellKnownSid(windows.WinBuiltinHyperVAdminsSid); err == nil {
		if member, err := windows.Token(0).IsMember(sid); err == nil {
			c.HyperVAdmin = member
		} else {
			c.Errors = append(c.Errors, "hyper-v administrators membership check: "+err.Error())
		}
	} else {
		c.Errors = append(c.Errors, "hyper-v administrators SID: "+err.Error())
	}

	// Hypervisor present (read-only CIM).
	if out, err := psQuery(`(Get-CimInstance Win32_ComputerSystem).HypervisorPresent`); err == nil {
		c.HypervisorPresent = strings.EqualFold(strings.TrimSpace(out), "True")
	} else {
		c.Errors = append(c.Errors, "hypervisor probe: "+err.Error())
	}

	// Hyper-V management stack present: the vmms service exists. Read-only and
	// needs no privilege, and cheaper/less privilege-sensitive than
	// Get-WindowsOptionalFeature.
	if out, err := psQuery(`if (Get-Service vmms -ErrorAction SilentlyContinue) { 'yes' } else { 'no' }`); err == nil {
		c.HyperVFeature = strings.EqualFold(strings.TrimSpace(out), "yes")
	} else {
		c.Errors = append(c.Errors, "hyper-v feature probe: "+err.Error())
	}

	// Durable/dev switch present (read-only).
	if c.DevSwitch != "" {
		script := "if (Get-VMSwitch -Name '" + psEscapeSingle(c.DevSwitch) +
			"' -ErrorAction SilentlyContinue) { 'yes' } else { 'no' }"
		if out, err := psQuery(script); err == nil {
			c.DevSwitchPresent = strings.EqualFold(strings.TrimSpace(out), "yes")
		} else {
			c.Errors = append(c.Errors, "dev switch probe: "+err.Error())
		}
	}

	// Privileged service: not implemented yet (Phase 4), so unreachable.
	c.ServiceReachable = false
}

// psQuery runs a read-only PowerShell command and returns its stdout.
func psQuery(script string) (string, error) {
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script)
	out, err := cmd.Output()
	return string(out), err
}

// psEscapeSingle escapes a value for a single-quoted PowerShell string literal.
func psEscapeSingle(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}
