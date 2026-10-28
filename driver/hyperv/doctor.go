package hyperv

import (
	"fmt"
	"io"
	"time"
)

// dotted returns "label " padded with dots to a fixed width, for aligned rows.
func dotted(label string) string {
	const width = 38
	s := label + " "
	for len(s) < width {
		s += "."
	}
	return s + " "
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

// Doctor writes the capability preflight report to w and reports whether the
// driver is blocked (a required capability for running builds is unmet). A
// blocked result is meant to drive a non-zero exit so `warden doctor --driver hyperv`
// works as a CI/agent gate.
func Doctor(w io.Writer, c Capabilities) (blocked bool) {
	if !c.Supported {
		fmt.Fprintf(w,
			"Hyper-V driver ....................... unavailable on %s "+
				"(requires a Windows host)\n", c.Platform)
		return true
	}

	fmt.Fprintf(w, "%s%s\n", dotted("Hyper-V hypervisor present"), yesNo(c.HypervisorPresent))
	fmt.Fprintf(w, "%s%s\n", dotted("Hyper-V feature enabled"), yesNo(c.HyperVFeature))
	fmt.Fprintf(w, "%s%s\n", dotted("Token read"), c.TokenScope)
	fmt.Fprintf(w,
		"%s%-4s(needed for: New-VMSwitch, New-NetNat, New-NetIPAddress)\n",
		dotted("Full Administrator"), yesNo(c.Elevated))
	hvaNote := "(covers: New-VM, Start-VM, Remove-VM, adapters, VHDX)"
	if !c.HyperVAdmin && c.HyperVAdminByAccount {
		hvaNote = "(account IS a member, but this process's token " +
			"predates it - stale token)"
	}
	fmt.Fprintf(w, "%s%-4s%s\n",
		dotted("Hyper-V Administrators"), yesNo(c.HyperVAdmin), hvaNote)

	switchLabel := "Dev switch"
	if c.DevSwitch != "" {
		switchLabel = fmt.Sprintf("Dev switch '%s'", c.DevSwitch)
	}
	switchState := "not found"
	if c.DevSwitchPresent {
		switchState = "present"
	}
	fmt.Fprintf(w, "%s%s\n", dotted(switchLabel), switchState)

	svc := "not installed"
	if c.ServiceReachable {
		svc = "reachable"
	}
	fmt.Fprintf(w, "%s%s\n", dotted("Privileged service"), svc)
	cacheBlocked := writeCacheVisibility(w)
	imageBlocked := writeImageStatus(w, Status(), time.Now())

	// Resolved per-operation outcomes.
	fmt.Fprintln(w, "\nResolved:")
	writeVMLifecycle(w, c)
	writeNetworkStrategy(w, c)

	if len(c.Errors) > 0 {
		fmt.Fprintln(w, "\nProbe notes:")
		for _, e := range c.Errors {
			fmt.Fprintf(w, "  - %s\n", e)
		}
	}

	ready, reason := c.Ready()
	if imageBlocked && ready {
		ready, reason = false, "the prepared base image cannot start builds (see above)"
	}
	if cacheBlocked && ready {
		ready, reason = false, "Hyper-V cannot see the image cache (see above)"
	}
	if !ready {
		fmt.Fprintf(w, "\nNot ready: %s\n", reason)
	}
	return !ready
}

// writeCacheVisibility reports whether files warden creates in the image cache
// are where Hyper-V can see them (see visible.go), and whether that blocks.
func writeCacheVisibility(w io.Writer) (blocked bool) {
	dir := hypervCacheDir()
	if err := checkDirVisible(dir, "the Hyper-V cache", cacheVisibleFix); err != nil {
		fmt.Fprintf(w, "%sREDIRECTED (%s)\n    -> %s\n", dotted("Image cache"), dir,
			cacheVisibleFix)
		return true
	}
	fmt.Fprintf(w, "%s%s\n", dotted("Image cache"), dir)
	return false
}

// writeVMLifecycle reports the resolved VM-lifecycle outcome and, when blocked,
// the remediation steps.
func writeVMLifecycle(w io.Writer, c Capabilities) {
	switch c.Resolve(OpVMLifecycle) {
	case OutcomeDirect:
		via := "Hyper-V Administrators"
		if c.Elevated {
			via = "elevated"
		}
		fmt.Fprintf(w, "  VM lifecycle ......... OK (direct, %s)\n", via)
	case OutcomeDelegate:
		fmt.Fprintln(w, "  VM lifecycle ......... OK (via privileged service)")
	case OutcomeBlocked:
		fmt.Fprintln(w, "  VM lifecycle ......... BLOCKED")
		if c.HyperVAdminByAccount {
			fmt.Fprintln(w, "    your account is in Hyper-V Administrators, but "+
				"this process's token predates that change.")
			fmt.Fprintln(w, "    -> sign out and back in, then restart the "+
				"gateway (a running process keeps its old token), or")
			fmt.Fprintln(w,
				"    -> run this build from a fresh Administrator terminal")
		} else {
			fmt.Fprintln(w, "    -> join the local Hyper-V Administrators "+
				"group (then re-login), or")
			fmt.Fprintln(w, "    -> launch warden from an Administrator terminal, or")
			fmt.Fprintln(w, "    -> install the network service (one-time, elevated)")
		}
	}
}

// writeNetworkStrategy reports how the build network will be provisioned and,
// when blocked, the remediation steps.
func writeNetworkStrategy(w io.Writer, c Capabilities) {
	switch c.NetworkStrategy() {
	case NetEphemeralPerBuild:
		fmt.Fprintln(w, "  Network .............. OK (fresh per-build "+
			"network, elevated - cleanest isolation)")
		if c.DevSwitchPresent {
			fmt.Fprintf(w, "                         (durable switch '%s' "+
				"present; used only for lower-privilege runs)\n", c.DevSwitch)
		}
	case NetDelegateService:
		fmt.Fprintln(w,
			"  Network .............. OK (fresh per-build via privileged service)")
	case NetReuseDurable:
		fmt.Fprintf(w, "  Network .............. OK (reuse durable switch "+
			"'%s', lower-privilege path)\n", c.DevSwitch)
	default:
		fmt.Fprintln(w, "  Network .............. BLOCKED")
		fmt.Fprintln(w, "    -> run `warden setup --driver hyperv` once from "+
			"an elevated shell (reusable switch), or")
		fmt.Fprintln(w, "    -> run this build from an Administrator terminal "+
			"(fresh per-build network), or")
		fmt.Fprintln(w, "    -> install the network service (one-time, elevated)")
	}
}
