package hyperv

import (
	"fmt"
	"io"
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
// blocked result is meant to drive a non-zero exit so `warden hyperv doctor`
// works as a CI/agent gate.
func Doctor(w io.Writer, c Capabilities) (blocked bool) {
	if !c.Supported {
		fmt.Fprintf(w, "Hyper-V driver ....................... unavailable on %s (requires a Windows host)\n", c.Platform)
		return true
	}

	fmt.Fprintf(w, "%s%s\n", dotted("Hyper-V hypervisor present"), yesNo(c.HypervisorPresent))
	fmt.Fprintf(w, "%s%s\n", dotted("Hyper-V feature enabled"), yesNo(c.HyperVFeature))
	fmt.Fprintf(w, "%s%s\n", dotted("Token read"), c.TokenScope)
	fmt.Fprintf(w, "%s%-4s(needed for: New-VMSwitch, New-NetNat, New-NetIPAddress)\n",
		dotted("Full Administrator"), yesNo(c.Elevated))
	fmt.Fprintf(w, "%s%-4s(covers: New-VM, Start-VM, Remove-VM, adapters, VHDX)\n",
		dotted("Hyper-V Administrators"), yesNo(c.HyperVAdmin))

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

	// Resolved per-operation outcomes.
	fmt.Fprintln(w, "\nResolved:")
	vm := c.Resolve(OpVMLifecycle)
	switch vm {
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
		fmt.Fprintln(w, "    -> join the local Hyper-V Administrators group, or")
		fmt.Fprintln(w, "    -> launch warden from an Administrator terminal, or")
		fmt.Fprintln(w, "    -> install the network service (one-time, elevated)")
	}

	net := c.Resolve(OpNetworkStandup)
	switch {
	case c.DevSwitchPresent:
		fmt.Fprintf(w, "  Network standup ...... not needed (reusing switch '%s')\n", c.DevSwitch)
	case net == OutcomeDirect:
		fmt.Fprintln(w, "  Network standup ...... OK (direct, elevated)")
	case net == OutcomeDelegate:
		fmt.Fprintln(w, "  Network standup ...... OK (via privileged service)")
	default:
		fmt.Fprintln(w, "  Network standup ...... BLOCKED")
		fmt.Fprintln(w, "    -> run `warden hyperv setup` once from an elevated shell, or")
		fmt.Fprintln(w, "    -> launch warden from an Administrator terminal, or")
		fmt.Fprintln(w, "    -> install the network service (one-time, elevated)")
	}

	if len(c.Errors) > 0 {
		fmt.Fprintln(w, "\nProbe notes:")
		for _, e := range c.Errors {
			fmt.Fprintf(w, "  - %s\n", e)
		}
	}

	ready, reason := c.Ready()
	if !ready {
		fmt.Fprintf(w, "\nNot ready: %s\n", reason)
	}
	return !ready
}
