//go:build windows

package hyperv

import (
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

	// If the token doesn't carry the group, check whether the account is a
	// member per the persistent group membership. A mismatch is the stale-token
	// signature (the group was joined after this process's logon token was
	// issued). Locale-independent: resolve the group name from the well-known
	// SID rather than hard-coding "Hyper-V Administrators".
	if !c.HyperVAdmin {
		script := `
$me = ([System.Security.Principal.WindowsIdentity]::GetCurrent()).User.Value
try {
  $g = (New-Object System.Security.Principal.SecurityIdentifier('S-1-5-32-578')).Translate([System.Security.Principal.NTAccount]).Value.Split('\')[-1]
  $m = Get-LocalGroupMember -Group $g -ErrorAction Stop | Where-Object { $_.SID.Value -eq $me }
  if ($m) { 'yes' } else { 'no' }
} catch { 'unknown' }`
		if out, err := runPS(script); err == nil {
			c.HyperVAdminByAccount = strings.EqualFold(strings.TrimSpace(out), "yes")
		}
	}

	// Hypervisor present (read-only CIM).
	if out, err := runPS(`(Get-CimInstance Win32_ComputerSystem).HypervisorPresent`); err == nil {
		c.HypervisorPresent = strings.EqualFold(strings.TrimSpace(out), "True")
	} else {
		c.Errors = append(c.Errors, "hypervisor probe: "+err.Error())
	}

	// Hyper-V management stack present: the vmms service exists. Read-only and
	// needs no privilege, and cheaper/less privilege-sensitive than
	// Get-WindowsOptionalFeature.
	if out, err := runPS(`if (Get-Service vmms -ErrorAction SilentlyContinue) { 'yes' } else { 'no' }`); err == nil {
		c.HyperVFeature = strings.EqualFold(strings.TrimSpace(out), "yes")
	} else {
		c.Errors = append(c.Errors, "hyper-v feature probe: "+err.Error())
	}

	// Durable/dev switch present (read-only).
	if c.DevSwitch != "" {
		script := "if (Get-VMSwitch -Name '" + psEscapeSingle(c.DevSwitch) +
			"' -ErrorAction SilentlyContinue) { 'yes' } else { 'no' }"
		if out, err := runPS(script); err == nil {
			c.DevSwitchPresent = strings.EqualFold(strings.TrimSpace(out), "yes")
		} else {
			c.Errors = append(c.Errors, "dev switch probe: "+err.Error())
		}
	}

	// Privileged service: not implemented yet (Phase 4), so unreachable.
	c.ServiceReachable = false
}
