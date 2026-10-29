---
# Retrofitted (FLOW-12) onto a pre-standard roadmap; body unchanged.
feature: vm-driver-release-plan
status: active
round: parallel
drivers: [container, qemu, vz, hyperv, firecracker]
seams: [relay-claim, collector, ledger-format, image-manifest, windows-guest-seed]
surfaces: [cli, config, output]
security_boundary: host-env             # also env-relay; see "Security Testing Framework"
alters_relationships: true
native_validation:
  - {host: macos-vz, blocking: true, reason: "Phase 1 vz driver runs only on macOS/arm64"}
  - {host: windows-hyperv, blocking: true, reason: "Phase 5 and 5a Hyper-V Windows-guest lifecycle"}
  - {host: macos-qemu, blocking: false, reason: "shared Windows guest package on qemu, tracked as qemu-windows-guest-package"}
tier2_gaps:
  - "shared Windows guest package (driver/winguest) on qemu under macOS: not yet validated natively"
rules: [ORCH-01, ORCH-04, ORCH-05, GLOBAL-06, GLOBAL-07, GLOBAL-08, RELAY-05, RELAY-07, TEST-02, TEST-03, TEST-08, SEC-01, UX-04, UX-07]
concepts: [release-plan, driver-matrix, security-testing, escape-vectors, image-lifecycle, relay-vm-distribution, ledger-compatibility, image-prepare, claim, collector]
related: [hyperv-driver-plan.md, windows-driver-plan.md, firecracker-driver-plan.md, hyperv-windows-prepare.md, platform-compatibility.md]
---

# VM Driver Release Plan

## Target Matrix

### Build Target Platforms (what runs inside the VM)

| Target OS | Architecture | Priority |
|-----------|-------------|----------|
| Linux | arm64 | P0 |
| Linux | amd64 | P0 |
| macOS | arm64 | P0 |
| Windows | amd64 | P1 |
| ~~macOS~~ | ~~amd64~~ | Skipped (Apple dropped Intel Macs) |

### Host Platforms (where warden runs)

| Host OS | Architecture | Drivers Available |
|---------|-------------|-------------------|
| macOS | arm64 | vz (native), qemu (fallback) |
| Linux | arm64 | kvm/qemu |
| Linux | amd64 | kvm/qemu |
| Windows | amd64 | hyperv (native), qemu (fallback) |

### Driver × Target Compatibility

| Driver | Linux/arm64 | Linux/amd64 | macOS/arm64 | Windows/amd64 |
|--------|-------------|-------------|-------------|---------------|
| container | ✓ (same-arch only) | ✓ (same-arch only) | — | — |
| vz | ✓ | ✓ (Rosetta) | ✓ | — |
| qemu | ✓ | ✓ | ✓ (macOS on macOS host only) | ✓ |
| hyperv | — | ✓ | — | ✓ |
| kvm | ✓ (same-arch) | ✓ (same-arch) | — | — |

Notes:
- Container driver: Linux targets only, same host arch, no VM overhead.
  This is the existing implementation (iptables isolation).
- VZ: macOS hosts only. Linux targets via VZLinuxBootLoader, macOS via
  VZMacOSBootLoader. amd64 Linux possible via Rosetta translation.
- QEMU: Universal. Cross-architecture via emulation. Native-speed when
  HVF (macOS) or KVM (Linux) is available for same-arch.
- Hyper-V: Windows hosts only. Linux and Windows guests.
- KVM: Linux hosts only. Same-arch guests at near-native speed.
  QEMU is the userspace frontend; KVM is the accelerator.

---

## Drivers to Implement

### 1. Container Driver (driver/container/)
**Status: Implemented, needs wiring**

Refactored from ScriptEnv. Uses docker/finch/podman + iptables sidecar.
Only valid for Linux targets on same-architecture Linux hosts (or any
host with a Linux VM runtime like Docker Desktop / Finch).

Remaining work:
- [ ] Wire into CLI via `--driver container` (currently still uses old ScriptEnv directly)
- [ ] Verify the refactored package produces identical ledgers to the original
- [ ] Remove old ScriptEnv code once fully transitioned

### 2. VZ Driver (driver/vz/)
**Status: Architecture complete, blocked on integration test**

Apple Virtualization.framework. macOS/arm64 hosts only.

Remaining work:
- [ ] Integration test: boot relay VM (blocked on codesigning)
- [ ] Integration test: full build cycle
- [ ] IPSW restore end-to-end validation
- [ ] Headless image preparation validation (Setup Assistant skip, user creation)
- [ ] Validate virtio-fs mounts in both Linux and macOS guests
- [ ] Validate socket-pair networking between VMs
- [ ] Rosetta support for amd64 Linux targets on arm64 host

### 3. QEMU Driver (driver/qemu/)
**Status: Functional — transparent proxy e2e working**

Universal fallback. Uses QEMU userspace emulator with optional
hardware acceleration (HVF on macOS, KVM on Linux).

Current implementation:
- Two-VM topology: relay (Alpine 3.21 initramfs) + build (direct kernel or QCOW2)
- 9p shared volume for trusted channel (boot script only)
- `stream` netdev for Unix socket between VMs
- iptables-legacy REDIRECT for transparent HTTP/HTTPS/DNS proxying
- MASQUERADE for relay's own upstream traffic
- Heartbeat protocol for build completion signaling
- CLI: `warden build --driver qemu [--script] [--image]`
- Verified: HTTP/HTTPS fetch from build VM recorded in ledger

Remaining work:
- [ ] Build context delivery through relay (not 9p mount)
- [ ] CA cert served from relay endpoint (self-discoverable)
- [ ] Dockerfile → script translation (reuse container driver logic)
- [ ] Stock image support (FROM declaration → cloud image fetch)
- [ ] Graceful VM shutdown (poweroff after watcher exits)
- [ ] warden-io integration for context fetch + artifact push

### 4. Hyper-V Driver (driver/hyperv/)
**Status: Not started**

Windows hosts only. Uses Hyper-V via PowerShell cmdlets or WMI.
Available on Windows Pro/Enterprise/Server at no additional cost.

Architecture choices:
- Invoke PowerShell cmdlets (New-VM, Start-VM, etc.) as subprocesses
- Or use the Hyper-V WMI provider via Go's syscall/COM layer
- Network isolation via Hyper-V virtual switches (internal switch
  for relay↔build, external switch for relay→internet)
- Shared volumes via SMB share or virtio-fs (if guest supports it)

Key considerations:
- Requires Hyper-V feature enabled on Windows host
- Linux guests use standard Linux ISOs with cloud-init
- Windows guests boot from prepared VHD/VHDX images
- The relay runs inside a Linux VM (same as other drivers)

### 5. KVM Driver (driver/kvm/)
**Status: Not currently targeted**

KVM is the Linux kernel's hardware virtualization support. QEMU uses
KVM as its accelerator on Linux — so `driver/qemu/` with KVM
acceleration (`-enable-kvm`) covers this case without a separate driver.

A dedicated KVM driver (using libvirt or direct /dev/kvm ioctls) would
bypass QEMU's userspace overhead entirely. We are not currently
targeting this, but will re-evaluate if feedback or performance data
indicates that QEMU's overhead is meaningful for build workloads or
that libvirt integration provides unique operational value for platform
operators already managing VM fleets through it.

---

## Security Testing Framework

### Requirements

Every valid host×driver×target combination must be validated by an
independent security agent that attempts to:

1. **Egress from the build environment** without going through the relay
2. **Compromise the relay** to gain unmonitored egress
3. **Escape to the host** from the build environment or relay

The agent must be independent from the implementation — it should not
share code with the driver, should not know the isolation mechanism's
internals, and should operate as a black-box attacker with full
capabilities available inside the build environment.

### Adversarial Test Architecture

```
Security Agent (independent binary/script)
├── Runs INSIDE the build environment as the "build"
├── Has root/admin access within its environment
├── Has access to all standard networking tools
├── Attempts every known escape vector
└── Reports success/failure for each vector

Test Harness (host-side)
├── Boots the full environment via the driver
├── Injects the adversarial agent as the "build script"
├── Monitors for escapes via:
│   ├── External listener (did anything reach the internet?)
│   ├── Relay ledger (did all traffic get recorded?)
│   └── Host monitoring (did anything touch the host?)
└── Asserts zero unrecorded egress
```

### Escape Vectors to Test (per-platform)

**Network-level (all platforms):**
- Direct TCP to external IP (bypass DNS)
- Direct UDP to external IP
- DNS tunneling (encoded data in TXT/CNAME queries)
- ICMP tunnel (ping with payload)
- IPv6 link-local communication
- All 65535 ports (not just 80/443)
- ARP spoofing (claim relay's IP/MAC)
- DHCP starvation/spoofing
- Raw socket creation (AF_PACKET, AF_RAW)
- IP-in-IP tunneling
- GRE tunneling

**VM-specific:**
- virtio-fs escape (path traversal, symlink following)
- Shared memory side channels
- VM escape via virtio device bugs (known CVEs)
- Clock/timing side channels for data exfiltration
- Serial console injection

**macOS-specific:**
- mDNS/Bonjour discovery of host services
- AirDrop/Bluetooth (should be unavailable in VM)
- Hypervisor.framework from within guest (nested virt)
- Accessing host clipboard/pasteboard

**Windows-specific:**
- SMB discovery on local network
- NetBIOS name resolution
- Windows networking stack fallbacks
- Hyper-V integration services escape

**Container-specific (existing iptables driver):**
- iptables rule flush from within container
- Network namespace escape
- Docker socket access
- Mount namespace escape
- Sidecar container race condition

### Per-Platform Adversarial Images

| Platform | Format | Base |
|----------|--------|------|
| Linux/arm64 | Shell script | Alpine with nmap, curl, nc, hping3, python3 |
| Linux/amd64 | Shell script | Same |
| macOS/arm64 | Shell script | Base macOS with developer tools |
| Windows/amd64 | PowerShell | Base Windows with nmap/npcap |

### Security Agent Independence

The adversarial agents should be:
- Written by a separate "security agent" (Claude instance with no knowledge of the driver implementation details)
- Given only the contract: "you are running inside an isolated build environment; try to communicate with the outside world or escape to the host"
- Updated independently when new escape vectors are discovered
- Run as a CI gate on any PR touching driver/ or relay/ code

---

## What's Missing from This Plan

### 1. Image Lifecycle Management

Beyond initial acquisition, images need:
- **Updates**: macOS/Windows/Linux images go stale. How does the user
  update their cached base image?
- **Garbage collection**: Old image clones from past builds accumulate.
  Need a `warden clean` that removes orphaned clones.
- **Integrity verification**: How do we ensure a cached image hasn't been
  tampered with between builds? Content-hash the image on first restore,
  verify before each clone.
- **Multi-version support**: A user might need macOS 14 and macOS 15
  images simultaneously for testing across versions.

### 2. Configuration Schema

The `warden.toml` needs to support multi-target builds with per-target
driver selection:

```toml
[runtime]
driver = "auto"  # auto-selects based on host + target

[[target]]
name = "linux-arm64"
mode = "container"  # or "vm"
containerfile = "Dockerfile"
context = "./src"

[[target]]
name = "macos-arm64"
mode = "vm"
driver = "vz"
image = "latest"  # or a path, or an IPSW URL
script = "scripts/build-macos.sh"
context = "./src"

[target.driver-config]
cpus = 4
memory = "8GiB"
disk = "64GiB"
```

This is described in the v2-architecture doc but not yet implemented.

### 3. Relay VM Image Distribution

For non-dev users, building the relay VM from source is friction.
Options:
- Pre-built kernel+initramfs in GitHub Releases (alongside warden binary)
- Embedded in the warden binary via `//go:embed` (adds ~10MB)
- Separate download on first use (like the IPSW)

Recommendation: embed in the binary. 10MB for instant relay boot with
zero external dependencies is worth it.

### 4. Cross-Architecture Linux Builds

When host and target arch differ (e.g., macOS/arm64 host → Linux/amd64
target), the relay VM must match the host arch (it's just running the
proxy), but the build VM needs to run the target arch. Options:
- QEMU full emulation (slow but universal)
- VZ + Rosetta (arm64 host only, translates amd64 Linux binaries)
- Native cross-compilation inside an arm64 build VM

### 5. Error Recovery and Diagnostics

When things go wrong (VM won't boot, relay doesn't start, build hangs):
- `warden doctor` — validates all prerequisites (runtime, images,
  entitlements, disk space, memory)
- Relay VM serial console capture for debugging boot failures
- Build VM console/log collection on failure
- Timeout handling per phase (boot, build, collection) with distinct
  error messages

### 6. Performance Baselines

Before release, establish performance baselines:
- Relay VM boot time (target: <3s)
- macOS VM boot time (target: <30s to login)
- Build start latency (time from `warden build` to first network request)
- Relay throughput (requests/sec, bandwidth)
- Comparison: container driver vs VM driver for same build

### 7. Ledger Compatibility

Ledgers produced by the VM driver must be verifiable by the same
`warden inspect` command and `ledger.Verify()` function as container
driver ledgers. The format is identical — only the environment
metadata record differs (vm type vs container type).

Need to define the environment schema for VM-based builds:
```json
{
  "type": "vm",
  "driver": "vz",
  "platform": "macos/arm64",
  "image_hash": "sha256:...",
  "relay_hash": "sha256:..."
}
```

### 8. Graceful Degradation

When a preferred driver isn't available:
- `driver = "auto"` should fall through: vz → qemu → error (on macOS)
- If QEMU isn't installed, provide a clear error with install instructions
- If VZ entitlements are missing, detect and advise
- If Hyper-V isn't enabled on Windows, detect and advise

### 9. Concurrent Builds

Multiple `warden build` invocations on the same machine:
- Container driver handles this via unique network allocation (existing)
- VM drivers need unique socket pairs per build (already the case)
- Relay VM port conflicts: not an issue since each relay runs in its own VM
- Resource contention: document recommended resource limits per concurrent build
- Disk space: COW clones are cheap but the base images are large

### 10. Build Platform Operator Integration

BuildWarden is infrastructure-level software run *by* build platform
operators (GitHub, GitLab, internal CI teams), not *within* CI jobs.
It requires admin access for VM creation and network isolation.

Operator considerations:
- Linux hosts: container driver (fastest) or QEMU for cross-platform targets
- macOS hosts (Apple Silicon): vz driver for macOS/Linux targets
- Windows hosts: Hyper-V for Windows/Linux targets
- Operators need to ensure their host environment allows virtualization
  (bare metal or nested-virt-enabled VMs)
- Resource allocation guidance: RAM/CPU/disk per concurrent build
- Image caching strategy for fleet deployment (shared NFS, local disk)

---

## Open Questions

### FROM declaration for VM images

The Dockerfile-like format currently maps `FROM` to an OCI image ref
(container driver). For VM drivers, `FROM` could specify:

- A cloud image URL: `FROM https://dl-cdn.alpinelinux.org/.../alpine-virt-3.21-aarch64.qcow2`
- A registry ref: `FROM registry.example.com/build-images/ubuntu-24.04:latest`
- A local path: `FROM ./images/my-build-env.qcow2`
- A shorthand: `FROM alpine:3.21` (resolved per-driver)

This would keep the Dockerfile as a single-file build configuration
even for VM-based builds. The driver would interpret `FROM` as "boot
this image" rather than "pull this container layer."

Questions to resolve:
- Should `FROM` for QEMU mean "boot from this disk image directly" or
  "fetch this image, apply remaining Dockerfile directives inside it"?
- For stock cloud images (Ubuntu, Alpine), how does the warden-io agent
  get installed? Cloud-init userdata? Pre-baked into a known location?
- Should there be a `warden image prepare <cloud-image>` command that
  bakes warden-io + trust config into an image for reuse?
- How does image caching work? Content-hash after first fetch?

### Trust boundary: what goes on the 9p mount vs through the relay

> **Resolved by the integration pass:** there is no shared volume any more.
> The relay keeps no state on disk; it is claimed by the orchestrator
> (`relaycfg`) and streams every output to the host collector over a
> host-initiated tunnel, which also serves the build script and context. The
> guest-facing HTTP endpoints below are unchanged. The original analysis is
> kept for history.

The 9p shared volume is the trusted channel. Only relay-controlled
content should be on it:
- The boot/watcher script (orchestration)
- Signal files (heartbeat, exit_code) — written by build, read by host

Everything else flows through the relay as HTTP endpoints:
- **Build context**: fetched by warden-io from `http://cwd/` (same as container driver)
- **CA certificate**: fetched from `http://artifacts/ca.pem` or similar relay endpoint,
  allowing any tool inside the build to self-configure TLS trust
- **Artifacts**: pushed by warden-io to `http://artifacts/`
- **Ledger writes**: handled by relay internally

This has the property that:
1. The relay records all meaningful I/O in the ledger
2. Package managers can manually configure trust if they detect a
   buildwarden environment (CA cert at a known URL)
3. The 9p mount surface area is minimal (read-only script + signal files)
4. Context transfer accounting is identical to the container driver

The CA cert fetch could be omitted from the ledger since it's
internally controlled (relay serving its own cert to its own client).

---

## Implementation Priority Order

### Phase 1: Complete VZ Driver (current work)
- Resolve codesigning for integration test
- Validate full build cycle on macOS/arm64
- Security testing for vz topology

### Phase 2: QEMU Driver — COMPLETE
- Linux arm64/amd64 targets (most users)
- Works on all host platforms
- Reuses relay VM kernel+initramfs
- Image sourcing from cloud image registries (Alpine, Ubuntu, Debian)
- Security testing: 12 adversarial tests + 10 independent v2 tests pass
- Validated: Alpine 3.21, Ubuntu 24.04 cloud images

### Phase 3: Firecracker Driver (driver/firecracker/)
**Status: Not started — future feature for Linux CI fleets**

Firecracker microVMs for high-density build isolation on Linux hosts.
Targets platform operators running hundreds of concurrent builds.

Why Firecracker over QEMU for this use case:
- Boot time: <125ms vs QEMU's ~1-2s
- Memory overhead: ~5MB per microVM vs QEMU's ~30MB+
- Attack surface: ~50K LOC vs QEMU's millions
- Purpose-built for multi-tenant isolation (no PCI, no USB, no legacy)

Implementation notes (from QEMU driver lessons):
- Reuses the same relay VM initramfs (shared trust boundary)
- MMDS (169.254.169.254) for metadata delivery (no cloud-init ISO)
- virtio-vsock for host↔VM signaling (alternative to 9p signal dir)
- Same `driver/script` Containerfile translator
- Same heartbeat protocol (relay auto-beats during traffic)
- KVM-only, same-arch-only, Linux hosts only
- Root images: ext4 from cloud image (no QCOW2, Firecracker uses raw)

Remaining design questions:
- vsock vs 9p for signal delivery (vsock avoids symlink attacks entirely)
- Jailer integration for additional host-side sandboxing
- Snapshot/resume for sub-100ms subsequent builds (warm VM pool)

### Phase 4: Container Driver Transition
- Wire refactored container driver into CLI
- Remove old ScriptEnv code path
- Verify ledger compatibility
- Re-validate existing security tests

### Phase 5: Hyper-V Driver
**Status: Windows guest path implemented and validated end to end (numpy
source build with MSVC on the Windows Server 2025 eval VHD). Linux guests on
Hyper-V not started.**
- Windows host support
- Linux + Windows guest targets
- PowerShell-based VM management
- Virtual switch network isolation

### Phase 5a: Hyper-V Windows-guest consolidation
**Status: Done (2026-10-28). Owner: Windows x86-64 desktop.** Re-validation of
the integration pass is recorded below. The consolidation shipped as the archived
[`hyperv-windows-prepare.md`](hyperv-windows-prepare.md) design (merged in #60):
one Windows guest package (`driver/winguest`) for qemu and Hyper-V, and
prepare-once images. Checklist items 2 and 4 (Gen1 `.vhd`) are out of scope. Still
to validate natively: the shared guest package on qemu under macOS (see the
native-gap backlog in [`docs/development/README.md`](../development/README.md)).

**Why.** The integration pass that merges `feat/windows-driver` into `main`
unifies the cross-driver seams (collector as the single output path, relay
config pushed to a listening relay, one CLI shape). It deliberately leaves the
Hyper-V *Windows-guest lifecycle* as implemented, because the remaining
differences from qemu depend on VHD and Windows behaviour that can only be
tested on Hyper-V. Today the two Windows guest paths diverge:

| | qemu (Windows guest) | hyperv (Windows guest) |
|---|---|---|
| Base image | Installed once from ISO (`image prepare`) | Stock eval VHD, booted as-is |
| Per build | Differencing disk + seed; startup task runs `warden-run` | Full OOBE every build; FirstLogonCommand runs `warden-run` |
| Build runs as | SYSTEM (scheduled task) | Auto-logged-on Administrator |
| Seed media | FAT image (second NVMe disk) | ISO via IMAPI2 (Gen1 IDE DVD) |
| `warden-run.ps1` / unattend generator | `driver/qemu` | separate copy in `driver/hyperv` (adds COM1 mirroring) |

**Hypothesis to validate (prepare once, as qemu does).**
`warden image prepare --driver hyperv --os windows` would:
1. Create a differencing child of the pinned eval VHD (chain: eval VHD ->
   prepared base -> per-build child; parent never copied or modified).
2. Boot it once with no network adapter (VM lifecycle privilege only).
3. Attach a seed ISO whose OOBE answer file's FirstLogonCommand registers the
   at-startup `warden-run` scheduled task (qemu's command, kept under the
   1024-char FirstLogonCommands limit), then shuts down.
4. Wait for VM state Off (timeout + COM1 diagnostics), hash the prepared base
   and record it in the image manifest as the active base.

Per build then becomes: differencing child of the prepared base plus a `WARDEN`
seed carrying only `warden-run.ps1` and `warden-io.exe`, identical to qemu.

**Validation checklist (do these before writing the feature).**
1. *Build account context.* Run `examples/numpy-source-win` from a startup task
   as SYSTEM. If MSVC/conda/pip misbehave under the systemprofile, try a local
   `warden` account created at prepare time and used as the task principal.
2. *Seed discovery.* Confirm `Get-Volume -FileSystemLabel WARDEN` finds the
   Gen1 IDE DVD from an early-boot SYSTEM task (qemu proved this only for a FAT
   USB/NVMe disk).
3. *Eval licensing clock.* Determine whether the 180-day evaluation period
   then runs from prepare time, whether `slmgr /rearm` applies, and what
   `image list` / `doctor` should warn about.
4. *Differencing chain.* Confirm a two-level chain off a Gen1 `.vhd` boots and
   tears down cleanly, and that moving the cache dir is either supported or
   detected.
5. *Per-build time.* Compare build wall time against today's per-build OOBE.

**Alternatives to evaluate if the hypothesis fails.** Keep per-build OOBE but
share the generators with qemu; flatten the prepared base (Convert-VHD) instead
of chaining; deliver the seed as a VHDX instead of an ISO; or another
consolidated approach found on the Windows host.

**Acceptance criteria.**
- One `warden-run.ps1` and one unattend generator shared by qemu and hyperv
  (in a shared Windows-guest package), with COM1 mirroring kept where a serial
  port exists.
- `warden image prepare` / `image list` behave the same for `--driver qemu`
  and `--driver hyperv` (same flags, same pin/verify semantics), differing only
  where the source media genuinely differs (ISO vs eval VHD).
- `examples/numpy-source-win` and `examples/conda-forge-win` pass on Hyper-V,
  and the qemu Windows path still passes on macOS/arm64.
- No dead code left in either driver's seed/bootstrap path.

**Already applied to the Hyper-V driver by the integration pass (re-validate
on the Windows host).** These were compiled for windows/amd64 and covered by
the unit tests (fake provisioner + real claim/tunnel), but have NOT run against
real Hyper-V. Validate them first; if any must be rolled back, record the
change, the reason, and the evidence under "Rollback notes" below so the
cross-driver design can be revisited rather than silently diverging.

1. *Relay config is pushed, not pulled.* The relay VM boots unconfigured and
   listens on :8301; `StartRelay` claims it at its static NAT address
   (192.168.240.2) after `StartVM`. `relaycfg`'s config responder (:8299) and
   the init script's `wget /config` loop are gone.
2. *Collector over a host-initiated tunnel.* The host opens
   `POST /v1/tunnel` on the claim port and serves the collector over it as
   HTTP/2, so nothing listens on the host. The collector port (:8390) and the
   `<nat>-relay-ingress` inbound firewall rule that `EnsureNetwork` created are
   removed. Check: claim + tunnel succeed through the NAT switch with Windows
   Firewall in its default state.
3. *One relay VM init* (`tools/relay-vm/init`, shared with qemu; the
   Hyper-V-only `tools/relay-vm/hyperv/init` is deleted). It detects Hyper-V via
   `/sys/bus/vmbus` (static NAT addressing, COM1 mirroring) and drops build-VM
   access to :8301. Rebuild the boot VHDX (`build.sh` now copies `../init`,
   `build-boot-vhdx*.ps1|sh`) with a current relay binary: an old boot disk
   speaks the retired pull protocol and will never accept a claim.
4. *Features Hyper-V previously lacked, now inherited from the shared path:*
   build-context files (`http://cwd/...`), `--capture`, upstream CA
   certificates, ledger/CA compression, heartbeat liveness (build fails after
   30 s of guest silence once heartbeats start), and a ledger streamed for the
   whole build. Check each with `examples/numpy-source-win`, plus one
   `--capture headers` run.
5. *CLI.* `warden hyperv doctor|setup` became `warden doctor|setup --driver
   hyperv`; `warden image fetch` became `warden image prepare --driver hyperv`
   (`--source <url>`, `--sha256`, `--accept-unpinned`, `--force`, `--name`).
   Env knobs became flags: `WARDEN_HYPERV_SWITCH` -> `--network` / `[vm]
   network`, `WARDEN_HYPERV_KEEP_VMS` -> `--keep-vms`,
   `WARDEN_HYPERV_BUILD_CPUS|MEMORY_MB` -> `--cpus` / `--memory`.
   `warden hyperv service ...` is unchanged.
6. *Shared staging.* warden-io.exe comes from `driver.GuestBinary` (prebuilt
   next to warden, else a cache rebuilt when stale; `WARDEN_IO_WINDOWS_EXE`
   is gone), and the build script from `driver.StageBuildScript` (`--script`,
   then `build.ps1` in the context; an empty context is rejected).

Untouched, and left to this phase: the seed ISO, the OOBE answer file,
`warden-run.ps1`, the per-build OOBE lifecycle on the eval VHD, and the
Gen1/Gen2 handling.

**Re-validation results (2026-10-20, Windows 11 Pro desktop, Hyper-V).** Run with
a rebuilt boot VHDX (current relay + shared init), the privileged service
reinstalled from the same build, and the retired `<nat>-relay-ingress` rule
removed (Windows Firewall in its default state). The build guest was the Windows
Server 2025 Datacenter eval **VHDX** passed with `--image` (Gen2): no Gen1 eval
`.vhd` was available, so checklist items 2 and 4 (Gen1 IDE DVD seed, two-level
`.vhd` chain) remain open.

| Item | Result |
|---|---|
| 1. Relay config pushed | Pass. `TestLiveRelayHostEndToEnd` (`WARDEN_HYPERV_LIVE=1`): claimed at 192.168.240.2, ready in 6-13 s. |
| 2. Collector over tunnel | Pass, with no warden firewall rules present. |
| 3. Shared relay VM init | Pass (same run; boot disk rebuilt from `tools/relay-vm/init`). |
| 4. Inherited features | Pass for context files (`http://cwd/build.ps1`), ledger streaming, `--capture headers` (1628 payloads) and heartbeats on success. Not exercised: upstream CA certificates, and the 30 s silence failure path. |
| 5. CLI | Pass: `doctor --driver hyperv`, `--image`, `--memory`, `--cpus`, `-o`. |
| 6. Shared staging | Pass: `build.ps1` from the context, `warden-io.exe` from next to warden. An empty context is now rejected up front ("nothing to build") rather than running a no-op. |

Builds: `examples/conda-forge-win` passed (205 requests, 292 MB, 831 signatures
valid, all channels closed). `examples/numpy-source-win` with `--memory 8192
--cpus 6 --capture headers` passed in 488 s (815 requests, 2.2 GB, 3289
signatures valid). A trivial `build.ps1` takes 81 s end to end, which bounds what
prepare-once can save per build on this image.

Checklist item 3 (eval licensing clock), measured on the Gen2 Server 2025
Datacenter eval VHDX right after a per-build OOBE: the guest is **unactivated**,
in "Initial grace period" (`LicenseStatus` 2, `TIMEBASED_EVAL` channel) with
10 days remaining. The end is an absolute date ("ends <install + 10 days>",
against "Trusted time"), so the clock runs on wall time from install, not
uptime. Rearm count is 1. A long build (numpy) activates online through the
relay (`activation-v2.sls.microsoft.com/SLActivateProduct`, in the ledger) and
discards it with the VM; a 30 s build does not get that far. Consequence for
prepare-once: a base prepared with no network stays in the 10-day initial grace
from prepare time, not 180 days, unless it is activated during prepare.

Found in passing (not Hyper-V specific): the relay hardcodes the ledger header
environment to `{"type": "container"}` (`relay/relay.go`), and VM drivers send no
`relaycfg.Environment`, so VM-driver ledgers mislabel their environment and do
not record the base image.

**Rollback notes.** _(none: no integration-pass change needed rolling back)_

### Phase 6: Multi-Target Config + Release Polish
- warden.toml [[target]] array support
- `warden build --all` for multi-target
- Image lifecycle management
- `warden doctor` diagnostics
- Performance baselines
- Documentation

### Phase 7: Security Audit
- Independent adversarial agent per driver/platform combo
- CI gate for isolation code changes
- External security review of the relay + network topology
