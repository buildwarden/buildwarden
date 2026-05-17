# Platform Compatibility Matrix

BuildWarden is infrastructure-level software that requires administrative
access to the host (VM creation, network namespace manipulation, etc.).
It is intended to be run *by* CI/CD platform operators as part of their
build infrastructure, not *from within* CI jobs as a user-space tool.

This document defines what host→target combinations are possible, which
are implemented and tested, and which are planned for future support.

---

## Reading This Document

**Status key:**
- **Supported** — Implemented, integration-tested, security-validated
- **In Progress** — Implementation exists but not yet fully validated
- **Planned** — Architecture supports it, implementation not started
- **Possible** — Technically feasible but no current plans
- **N/A** — Not possible due to technical or legal constraints

---

## Host → Target × Driver Matrix

### macOS/arm64 Host

| Target | Driver | Acceleration | Status | Notes |
|--------|--------|-------------|--------|-------|
| Linux/arm64 | vz | Native (VZ.framework) | In Progress | VZLinuxBootLoader, relay VM same arch |
| Linux/arm64 | qemu | HVF | Planned | Same perf as vz, more config |
| Linux/amd64 | vz | Rosetta | Planned | Binary translation, good perf |
| Linux/amd64 | qemu | Emulation | Planned | Slow (~10x), fallback only |
| macOS/arm64 | vz | Native (VZ.framework) | In Progress | IPSW restore, headless prep |
| macOS/arm64 | qemu | HVF | Possible | Legal on Apple hw, redundant with vz |
| Windows/amd64 | qemu | Emulation | Planned | Slow, requires user-supplied image |
| Linux/arm64 | container | Docker/Finch | Supported | Existing implementation (via Linux VM) |
| Linux/amd64 | container | Docker/Finch | Supported | Via Rosetta in Finch/Docker Desktop |

### Linux/arm64 Host

| Target | Driver | Acceleration | Status | Notes |
|--------|--------|-------------|--------|-------|
| Linux/arm64 | container | Native iptables | Supported | Existing implementation, fastest |
| Linux/arm64 | qemu | KVM | Planned | VM overhead, stronger isolation than container |
| Linux/amd64 | qemu | Emulation | Planned | Slow cross-arch |
| macOS/arm64 | — | — | N/A | macOS VMs legally restricted to Apple hardware |
| Windows/amd64 | qemu | Emulation | Possible | Very slow, unlikely use case |

### Linux/amd64 Host

| Target | Driver | Acceleration | Status | Notes |
|--------|--------|-------------|--------|-------|
| Linux/amd64 | container | Native iptables | Supported | Existing implementation, fastest |
| Linux/amd64 | qemu | KVM | Planned | VM overhead, stronger isolation |
| Linux/arm64 | qemu | Emulation | Planned | Slow cross-arch |
| macOS/arm64 | — | — | N/A | macOS VMs legally restricted to Apple hardware |
| Windows/amd64 | qemu | KVM | Planned | Good perf with KVM, needs UEFI image |

### Windows/amd64 Host

| Target | Driver | Acceleration | Status | Notes |
|--------|--------|-------------|--------|-------|
| Linux/amd64 | hyperv | Native | Planned | Good perf, Hyper-V built into Pro/Enterprise |
| Linux/arm64 | hyperv | Emulation | Possible | Hyper-V supports arm64 guests on some versions |
| Windows/amd64 | hyperv | Native | Planned | Best perf for Windows targets |
| Linux/amd64 | qemu | WHPX | Possible | Alternative to Hyper-V, more complex setup |
| macOS/arm64 | — | — | N/A | macOS VMs legally restricted to Apple hardware |

---

## Driver Availability by Host

| Host | container | vz | qemu | hyperv |
|------|-----------|-------|------|--------|
| macOS/arm64 | ✓ (via Finch/Docker) | ✓ | ✓ | — |
| Linux/arm64 | ✓ (native) | — | ✓ | — |
| Linux/amd64 | ✓ (native) | — | ✓ | — |
| Windows/amd64 | ✓ (via Docker/WSL2) | — | ✓ | ✓ |

---

## Legal Constraints

- **macOS guests** may only run on Apple hardware. This is an Apple EULA
  restriction, not a technical one. BuildWarden enforces this by only
  supporting macOS targets through the vz driver (which requires Apple
  Silicon) or QEMU with HVF (which requires macOS host).

- **Windows guests** require a valid Windows license for the VM image.
  BuildWarden does not ship Windows images. Users/operators must provide
  their own (Microsoft offers evaluation images for development).

---

## Network Isolation by Driver

All drivers must provide the same security guarantee: the build
environment cannot communicate with any external system without that
traffic passing through the relay.

| Driver | Isolation Mechanism | Bypass Surface |
|--------|-------------------|----------------|
| container | iptables DNAT + OUTPUT DROP via sidecar | Kernel vuln, container escape, cap escalation |
| vz | Topological (sole interface → relay VM) | VM escape, virtio device vuln |
| qemu | QEMU user-net restrict + relay gateway | QEMU escape, device emulation vuln |
| hyperv | Virtual switch ACLs + relay gateway | Hyper-V escape, vSwitch vuln |

The VM-based drivers have a stronger security posture than the container
driver because the isolation boundary is a full hardware virtualization
layer rather than Linux namespaces + netfilter rules.

---

## Future Platform Considerations

Platforms that may become relevant but have no current plans:

| Platform | Role | Feasibility | Dependency |
|----------|------|-------------|-----------|
| FreeBSD/arm64 | Target | Possible | QEMU guest, community interest |
| RISC-V | Target | Possible | QEMU supports it, ecosystem immature |
| macOS/arm64 | Host (CI) | In Progress | Apple Silicon CI runners exist (Cirrus, AWS) |
| ChromeOS | Host | Unlikely | Limited admin access, Crostini sandboxing |
| iOS/arm64 | Target | N/A | No VM support, Apple restrictions |

When a new platform request arrives:
1. Determine if it's a host or target request
2. Check which existing drivers could support it
3. If new driver needed, evaluate available hypervisors
4. Add to this matrix with "Planned" or "Possible" status
5. Security testing required before "Supported" status

---

## Graduation Criteria: Planned → Supported

A host×target×driver combination graduates to "Supported" when:

1. **Implementation complete** — Driver code handles full lifecycle
2. **Integration tested** — End-to-end build produces valid ledger
3. **Security validated** — Independent adversarial agent finds no bypass
4. **Performance baselined** — Boot time, build latency, throughput measured
5. **Image sourcing documented** — Clear instructions for acquiring base images
6. **Error handling complete** — Graceful failures with actionable messages
