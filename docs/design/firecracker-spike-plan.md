# Firecracker Driver — Design Document

**Date:** 2026-10-08
**Branch:** `feat/firecracker-driver` (from `origin/main`)
**Status:** Design Phase — ready for implementation by AI coding agents

---

## 1. What This Is

A design document for a Firecracker driver that enables BuildWarden to run builds inside Firecracker microVMs on Linux hosts. This targets CI fleets running hundreds of concurrent builds where sub-125ms warm starts and minimal attack surface matter.

This is NOT a file-by-file specification. It specifies the architecture, constraints, and design decisions. The AI agent implementing this has full discretion on implementation details (function signatures, error messages, helper functions, test coverage) — it should check in with me only on decisions that have security implications or cross-cutting architectural impact.

---

## 2. What This Creates

### New Code

| Category | Details |
|----------|---------|
| `driver/firecracker/` package | 10-12 Go source files (~1,500-2,000 lines) |
| `driver/firecracker/*_test.go` | Unit and integration tests (~400-600 lines) |
| CLI wiring | `config.go`, `main.go`, `clean.go` modifications (~50 lines) |
| Build scripts | `tools/firecracker-relay-vm/build-kernel.sh`, `build-rootfs.sh` (~80 lines) |
| Makefile targets | `firecracker-assets`, `test-firecracer` (~15 lines) |
| Go dependency | `github.com/mdlayher/vsock` |

### What's Reused (no changes)

- `driver.Driver` interface, `BuildRequest`, `BuildResult`
- `driver/script.Translate()` — Dockerfile → shell script
- `driver.DefaultExtensions()` — CA trust, epoch, bazel
- `relay.Start()` — core relay logic (all modes)
- `cmd/relay/mode_vm.go` — relay VM mode (already works)
- `waitForFile()`, `collectOutputs()`, `cachedBinaryPath()`, `findModuleRoot()` — shared utilities
- `copyFile()` — file copy (used by all drivers)

---

## 3. Architecture

### Topology (mirrors QEMU driver)

```
Host (orchestrator)
├── Firecracer Relay VM (Alpine, direct kernel boot, raw ext4 rootfs)
│   ├── eth0: TAP → bridge → Build VM (sole network path)
│   ├── eth1: TAP → host MASQUERADE (upstream internet)
│   └── vsock CID=N: relay↔host readiness signaling
├── Firecracer Build VM (user-specified OS, raw ext4 rootfs)
│   ├── eth0: TAP → bridge → Relay VM (sole network path)
│   └── vsock CID=M: build↔host heartbeat/exit
└── Host process
    ├── Manages TAP devices and bridge
    ├── Monitors vsock for readiness/heartbeat/exit
    └── Collects ledger from relay rootfs after build
```

### Key Differences from QEMU

| Concern | QEMU | Firecracer |
|---------|------|-----------|
| Relay↔Build network | Unix socket stream netdev | TAP pair (veth-like) via host bridge |
| Relay↔Internet | QEMU user-net (built-in NAT) | Host TAP + iptables MASQUERADE |
| Shared filesystem | virtio-9p | Not available; use vsock or block device |
| Metadata injection | Cloud-init seed ISO | MMDS (169.254.169.254) |
| Signaling | 9p shared directory (file-based) | vsock (stream-based) or file-based |
| Process model | QEMU single process per VM | firecracer binary + optional jailer |

### Key Differences from VZ

| Concern | VZ | Firecracer |
|---------|----|-----------|
| Relay location | Runs on host (process) | Runs inside relay VM |
| Network model | Socket pair (topological) | TAP devices (L2 bridge) |
| Platform | macOS/arm64 only | Linux only (KVM) |
| VM overhead | ~200MB/VM | ~5MB/VM |
| Boot time | ~5s cold | ~125ms cold, ~10ms warm |

---

## 4. Design Decisions

### 4.1 Relay Mode — Use Existing `vm` Mode (NO CHANGES TO RELAY)

The relay binary (`cmd/relay/`) already supports three modes via auto-detection: `container`, `vm`, and `host`. The Firecracer relay VM runs the relay as init (PID 1) and binds directly to TAP interfaces — exactly what `mode_vm.go` handles.

**No relay binary changes needed.** The existing relay VM init script (used by QEMU) works with minor modifications for Firecracer's network interface naming (`eth0`/`eth1` instead of PCI-discovered names).

### 4.2 Signaling — Hybrid Approach (File-Based Phase 1, vsock Phase 2)

The QEMU driver uses file-based signaling (signal dir on 9p shared volume). Firecracer has no 9p/virtiofs. Two options:

**Phase 1 (MVP): File-based via rootfs mount.** The relay already has HTTP endpoints (`/heartbeat`, `/exit`) that the build VM calls — these write to the signal dir on the rootfs. The host simply mounts the rootfs after stopping the VM to read signals. This reuses existing relay code with zero relay changes.

**Phase 2 (optimization): vsock for host↔VM signaling.** Once Phase 1 works, add vsock-based heartbeat/exit monitoring for real-time build monitoring without post-hoc rootfs mounting.

**Recommendation:** Implement Phase 1 first. It's simpler and reuses the existing relay. Phase 2 is a natural follow-up.

### 4.3 Jailer — Optional, Disabled by Default

The jailer adds significant operational complexity (UID allocation, cgroup setup, network namespace). For the spike/MVP:

- Default: `UseJailer = false` (KVM provides sufficient isolation for development/testing)
- Optional: `UseJailer = true` (requires jailer binary, pre-allocated UID/GID, CAP_SYS_ADMIN)
- Multi-tenant production: enable jailer

**This is a design decision that requires human review** (see Section 7).

### 4.4 Image Format — QCOW2 → Raw ext4 (via `qemu-img`)

Firecracer requires raw block device images. The QEMU driver downloads QCOW2 cloud images. The conversion step:

```
cloud image (QCOW2) → qemu-img convert → per-build raw ext4
```

This is cached per base image (one raw file per cloud image), with per-build copies using `cp --reflink=auto`.

**This is a design decision that requires human review** (see Section 7).

### 4.5 Copy-on-Write — `cp --reflink=auto` with Fallback

Start with `cp --reflink=auto`. Modern Linux filesystems (btrfs, xfs with reflink) support instant copies. Falls back to full copy on ext4 without reflink support.

Graduate to device-mapper thin provisioning as an optional fleet feature after MVP.

### 4.6 MMDS — v2 (Token-Based)

v2 requires a token GET before reading metadata, preventing the build script from reading relay metadata. The relay VM doesn't use MMDS (it IS the metadata source). The build VM uses MMDS v2 for:

- Build script injection (via cloud-init-like user-data)
- Network configuration (static IP assignment)
- CA certificate URL

**This is a design decision that requires human review** (see Section 7).

### 4.7 Network — Host TAP + Bridge (Option A)

The relay VM needs upstream internet access. Firecracer has no QEMU user-net equivalent. The host TAP approach requires `CAP_NET_ADMIN` on the CI host — acceptable for CI fleets since:

- CI fleet operators already run with elevated privileges
- It's the same requirement as the container driver (iptables)
- It provides the most transparent proxying (identical iptables REDIRECT rules as QEMU)

**This is a design decision that requires human review** (see Section 7).

### 4.8 vsock CID Allocation — Range 100-999, File-Lock Based

Each Firecracer VM needs a unique vsock Context ID (CID). Range 2 is reserved for the host. Range 100-999 gives 900 CIDs per host — sufficient for most fleets. File-lock based (`flock`) is simple and correct for a single-host scenario.

### 4.9 Kernel and Rootfs — Pre-Built via `make firecracer-assets`

Ship build scripts (`tools/firecracer-relay-vm/build-kernel.sh` and `build-rootfs.sh`) that produce cached assets. First use builds from source; subsequent uses hit the cache.

**Key difference from QEMU:** Firecracer requires an **uncompressed** kernel (`vmlinux`, not `vmlinuz`). The kernel build script must produce this format.

### 4.10 Extension System — Same Extensions, Minor Adaptation

The existing extensions (`ExtTrustStore`, `ExtBazel`, `ExtCACerts`, `ExtEpoch`) operate on the `ExtensionContext` which provides `WardenDir`, `LedgerDir`, and `RelayIP`. These are all available in the Firecracer context:

- `WardenDir` → injected into build VM rootfs (mounted at `/warden`)
- `LedgerDir` → path on relay VM rootfs (where relay writes)
- `RelayIP` → relay VM's IP on the bridge (e.g., `10.0.0.1`)

Minor adaptation: trust store injection must happen by mounting the build VM rootfs and copying the CA cert, rather than writing to a shared volume.

---

## 5. Implementation Structure

### Package: `driver/firecracer/`

```
driver/firecracer/
├── driver.go          # Driver struct, StartBuild, Close (mirrors qemu/driver.go)
├── vm.go              # startRelayVM, startBuildVM, vmProcess, stop/wait
├── config.go          # Firecracer JSON config generation
├── network.go         # TAP creation, bridge setup, cleanup
├── images.go          # Image resolution, QCOW2→raw conversion, caching
├── vsock.go           # vsock signaling: heartbeat listener, exit listener
├── mmds.go            # MMDS metadata injection (cloud-init replacement)
├── api.go             # Firecracer API socket client (HTTP over UDS)
├── cid.go             # vsock CID allocator
├── pool.go            # Warm VM pool manager (Phase 4)
├── jailer.go          # Jailer integration (Phase 3)
└── util.go            # Binary caching, file helpers (shared with qemu)
```

### Key Types

```go
package firecracer

import (
    "context"
    "net"
    "os/exec"
    "sync"
    "time"

    "warden/driver"
)

// Driver implements driver.Driver using Firecracer microVMs.
// Linux-only, requires /dev/kvm access.
type Driver struct {
    FirecracerBinary string  // override path to firecracer binary
    JailerBinary     string  // override path to jailer binary (optional)
    UseJailer        bool    // enable jailer-based sandboxing (default: false)
    Verbose          bool    // enable VM console output on stderr
    PoolSize         int     // warm VM pool size (0 = no pool)

    pool     *Pool           // warm VM pool (nil if PoolSize == 0)
    cidAlloc *CIDAllocator   // vsock CID allocator
}

// vmProcess wraps a running Firecracer microVM.
type vmProcess struct {
    cmd      *exec.Cmd
    name     string       // "relay" or "build"
    apiSock  string       // path to Firecracer API socket
    vsockUDS string       // path to vsock UDS on host
    cid      uint32       // vsock context ID
    tapDevs  []string     // TAP devices to clean up
    bridge   string       // bridge device name
    rootfs   string       // rootfs path (for cleanup)
}

// CIDAllocator manages vsock CID assignment.
type CIDAllocator struct {
    mu       sync.Mutex
    base     uint32 // e.g., 100
    max      uint32 // e.g., 65535
    inUse    map[uint32]bool
}

// firecracerConfig is the JSON structure Firecracer expects.
type firecracerConfig struct {
    BootSource    bootSource      `json:"boot-source"`
    Drives        []drive         `json:"drives"`
    NetworkIfaces []networkIf     `json:"network-interfaces"`
    Vsock         *vsockConfig    `json:"vsock,omitempty"`
    MMDSConfig    *mmdsConfig     `json:"mmds-config,omitempty"`
    MachineConfig machineConfig   `json:"machine-config"`
}

type bootSource struct {
    KernelPath string `json:"kernel_image_path"`
    BootArgs   string `json:"boot_args"`
    InitrdPath string `json:"initrd_path,omitempty"`
}

type drive struct {
    DriveID      string `json:"drive_id"`
    PathOnHost   string `json:"path_on_host"`
    IsRootDevice bool   `json:"is_root_device"`
    IsReadOnly   bool   `json:"is_read_only"`
}

type networkIf struct {
    IfaceID     string `json:"iface_id"`
    GuestMAC    string `json:"guest_mac"`
    HostDevName string `json:"host_dev_name"`
}

type vsockConfig struct {
    GuestCID uint32 `json:"guest_cid"`
    UDSPath  string `json:"uds_path"`
}

type mmdsConfig struct {
    Version    string   `json:"version"`
    Interfaces []string `json:"network_interfaces"`
}

type machineConfig struct {
    VcpuCount  int  `json:"vcpu_count"`
    MemSizeMiB int  `json:"mem_size_mib"`
    SMT        bool `json:"smt,omitempty"`
}
```

### StartBuild Flow

```
1. StartBuild(ctx, req) called
   │
2. ├── checkPrereqs() — validate KVM, TUN/TAP, vhost_vsock, binaries
   │
3. ├── create workDir (temp dir for this build)
   │
4. ├── prepareSharedDir() — relay binary, warden-io, build script
   │
5. ├── allocate vsock CIDs (relay, build) — CIDAllocator.AcquirePair()
   │
6. ├── createBuildNetwork() — TAP devices + bridge + MASQUERADE
   │
7. ├── resolveRelayAssets() — kernel + rootfs from cache or build
   │
8. ├── startRelayVM() — write config JSON, mount rootfs, start firecracer
   │
9. ├── waitForRelayReady() — vsock or file-based "ready" signal
   │
10. ├── resolveBuildImage() — cloud image → QCOW2 → raw ext4
    │
11. ├── startBuildVM() — write config JSON, push MMDS, start firecracer
    │
12. ├── waitForBuild() — heartbeat/exit monitoring (vsock or file-based)
    │
13. ├── collectOutputs() — move ledger + CA cert to output dir
    │
14. └── cleanupWorkDir() — stop VMs, remove TAP/bridge, release CIDs
```

---

## 6. Integration Points with Existing Code

### Files That Need Changes (minimal)

| File | Change | Scope |
|------|--------|-------|
| `cmd/warden/config.go` | Add `"firecracer"` to `validateDriver()` | 1 line |
| `cmd/warden/main.go` | Add `case "firecracer":` in `runBuild()` | ~25 lines |
| `cmd/warden/clean.go` | Add `cleanFirecracer()` call | ~30 lines |
| `go.mod` | Add `github.com/mdlayher/vsock` | 1 line |

### Shared Code (reuse from existing drivers)

- `driver.RandAlphaNum()` — Build ID generation
- `driver/script.Translate()` — Dockerfile → script translation
- `driver.DefaultExtensions()` — CA trust, epoch, bazel
- `relay.Start()` — Core relay logic (all modes)
- `waitForFile()` — File polling utility (used by vz and qemu)
- `collectOutputs()` — Ledger/artifact collection (used by vz and qemu)
- `cachedBinaryPath()`, `isCacheStale()`, `findModuleRoot()` — Caching (used by qemu)
- `copyFile()` — File copy (used by all)

The Firecracer driver reuses ~60% of the existing driver infrastructure. The new code is primarily hypervisor-specific VM management (TAP, vsock, Firecracer API).

---

## 7. Decisions Requiring Human Review

These are the ONLY items that require your input before implementation. The AI agent should STOP and check in with you on these topics:

### 7.1 Kernel Version Pinning

**Question:** Which kernel version to ship with Firecracer?
- Options: 5.15 (LTS, stable), 6.1 (LTS, newer features), 6.6 (latest LTS)
- Recommendation: 6.1 (matches Firecracer 1.10+ requirements)
- **Decision:** Pin version or allow configurable version?

### 7.2 Cloud Image URL Mapping

**Question:** Which cloud images to support by default?
- Currently maps: ubuntu-24.04, ubuntu-22.04, ubuntu-20.04, ubuntu-arm64, debian-12, debian-11
- **Decision:** Add more images (Alpine, CentOS, RHEL)? Add custom image URL support?

### 7.3 Signaling Approach (Phase 1 vs Phase 2)

**Question:** Phase 1 uses file-based signaling (mount rootfs after VM stop). Phase 2 adds vsock signaling.
- **Decision:** Commit to Phase 1 file-based approach or skip directly to vsock?
- **Recommendation:** Phase 1 file-based (reuses existing relay code), Phase 2 vsock (cleaner)

### 7.4 Default Resource Allocation

**Question:** Default vCPUs and memory for relay/build VMs:
- Relay: 2 vCPUs, 512MB (current)
- Build: 4 vCPUs, 4096MB (current)
- **Decision:** Adjust defaults based on typical build workloads?

### 7.5 Jailer Default

**Question:** Should jailer be enabled by default?
- **Decision:** Default to `UseJailer = false` (spike/MVP) or `true` (production-ready)?
- **Recommendation:** `false` for spike, configurable via `--jailer` flag
- **Security implication:** Enabling jailer changes the threat model — it adds sandboxing but also operational complexity.

### 7.6 Image Format: QCOW2→raw Conversion

**Question:** Should we convert cloud images to raw at download time or at build time?
- **Decision:** Convert at download time (cache raw per cloud image) or convert per-build (slower but fresh)?
- **Recommendation:** Convert at download time (cache raw per cloud image), use `cp --reflink=auto` per-build

### 7.7 MMDS v2 Token Handling

**Question:** How should the build VM guest fetch the MMDS token?
- **Decision:** Embed token fetch in build VM init script, or provide a helper binary?
- **Security implication:** If the token is hardcoded in the guest, it could be extracted from the rootfs image. If fetched at runtime, it adds a network call during boot.

### 7.8 Network Subnet Range

**Question:** What subnet range to use for Firecracer builds?
- Current container driver uses `100.64.87.0/24` (CGNAT range)
- Firecracer uses `10.0.0.0/30` (point-to-point per build)
- **Decision:** Use a different range to avoid conflicts with container driver on same host?
- **Recommendation:** Use `10.0.0.0/16` for Firecracer (non-overlapping with container driver)

---

## 8. What the AI Agent CAN Adapt (No Check-In Required)

The AI agent has full discretion on these implementation details — it can modify, extend, or change them without checking in:

### 8.1 Function Signatures

The agent can change any function signature (parameters, return types, error handling) as long as:
- The function still serves its stated purpose
- The function is called correctly from `StartBuild()`
- The function doesn't break the `driver.Driver` interface contract

### 8.2 Helper Functions

The agent can add, remove, or rename any helper function (e.g., `prepareRelayRootfs()`, `createUpstreamTAP()`, `cachedImagePath()`) as long as:
- The helper serves a clear purpose
- It's documented (even briefly)
- It's called correctly from its callers

### 8.3 Error Messages

The agent can change error messages, add context, or restructure error wrapping as long as:
- The error is still actionable (tells the user what went wrong and how to fix it)
- The error category is preserved (prerequisite, network, VM startup, etc.)

### 8.4 Test Coverage

The agent can add, remove, or restructure tests as long as:
- Unit tests exist for config generation (JSON marshaling/unmarshaling)
- Unit tests exist for CID allocation
- Unit tests exist for network command generation (mock exec)
- Integration tests exist for full build cycle (on hosts with KVM)

### 8.5 File Organization

The agent can split or merge files within `driver/firecracer/` as long as:
- The package compiles
- The driver package is importable by `cmd/warden/main.go`
- The package exports the `Driver` type and `New()` constructor

### 8.6 Implementation Approach

The agent can choose different implementation approaches for any function as long as:
- The approach respects the constraints in Section 4 (decisions)
- The approach doesn't introduce new security risks
- The approach doesn't break existing drivers

### 8.7 Code Style

The agent can follow its own code style conventions as long as:
- The code is idiomatic Go
- The code follows the existing patterns in `driver/qemu/` and `driver/vz/`
- The code is readable and maintainable

---

## 9. What the AI Agent MUST NOT Change Without Check-In

These are the hard boundaries — the agent should STOP and ask before modifying:

### 9.1 Security Boundaries

- **Do NOT change the relay mode** — the relay binary works as-is with `vm` mode. Do not add new relay modes.
- **Do NOT remove or weaken network isolation** — all traffic must flow through the relay. No direct egress from build VM.
- **Do NOT change the `driver.Driver` interface** — all drivers must implement `Name()`, `StartBuild()`, `Exec()`, `Close()`.
- **Do NOT change the `BuildRequest` or `BuildResult` structs** — these are the contract between CLI and drivers.

### 9.2 Cross-Cutting Changes

- **Do NOT modify the relay binary** (`cmd/relay/`) — the relay works as-is. Do not add Firecracer-specific relay features.
- **Do NOT modify existing drivers** (`driver/container/`, `driver/qemu/`, `driver/vz/`) — the Firecracer driver is a new package.
- **Do NOT modify the CLI flag set** beyond adding `--driver firecracer` — do not add new flags without discussion.

### 9.3 Behavioral Changes

- **Do NOT change ledger format** — the binary ledger format is immutable. The Firecracer driver must produce identical ledgers.
- **Do NOT change extension behavior** — extensions must work identically across all drivers.
- **Do NOT change error semantics** — errors must be consistent across drivers (e.g., `ErrExecNotSupported` for non-interactive drivers).

### 9.4 Existing Integration

- **Do NOT modify `cmd/relay/`** — the relay binary works as-is with `vm` mode. Do not add Firecracer-specific relay features.
- **Do NOT modify `driver/container/`, `driver/qemu/`, or `driver/vz/`** — the Firecracer driver is a new package.
- **Do NOT change existing CLI flags** beyond adding `--driver firecracer` — do not add new flags without discussion.

---

## 10. Implementation Phases

### Phase 0: Spike (1 week)

**Goal:** Prove the core concept works — boot a relay VM, start a build VM, complete a simple build.

**Deliverables:**
- `driver/firecracer/` skeleton (driver.go, config.go, vm.go)
- Relay VM boots and signals "ready" (file-based or vsock)
- Build VM boots and communicates with relay
- Simple build (e.g., `FROM alpine, RUN echo hello`) completes
- Ledger file produced and verified

### Phase 1: Minimal E2E (2-3 weeks)

**Goal:** `warden build --driver firecracer` completes a realistic build.

**Deliverables:**
- `network.go` — TAP creation and cleanup
- `images.go` — Image resolution, QCOW2→raw conversion, caching
- Relay rootfs build script (`tools/firecracer-relay-vm/build-rootfs.sh`)
- File-based signaling (mount rootfs after VM stop to read signals)
- CLI integration (`--driver firecracer` recognized)
- Graceful cleanup on all error paths

### Phase 2: Live Monitoring (1-2 weeks)

**Goal:** Real-time build monitoring without post-hoc rootfs mounting.

**Deliverables:**
- `vsock.go` — Live heartbeat/exit monitoring
- `mmds.go` — Cloud-init replacement via MMDS v2
- `api.go` — Runtime Firecracer API calls (pause, resume, config update)
- Remove need to mount rootfs for signal/ledger extraction

### Phase 3: Production Hardening (1-2 weeks)

**Goal:** Safe for multi-tenant CI fleet deployment.

**Deliverables:**
- `jailer.go` — Jailer integration (chroot, seccomp, UID/GID)
- `cid.go` — CID allocator with file locking
- Network namespace support (when jailer is used)
- Rate limiters (network + disk I/O)
- Security audit: all attack surface vectors reviewed

### Phase 4: Warm Pool + Snapshots (2-3 weeks)

**Goal:** <10ms warm start for subsequent builds.

**Deliverables:**
- `pool.go` — Pool manager, snapshot creation, resume
- Snapshot invalidation logic (relay binary hash check)
- Pool size tuning / backpressure
- Metrics (boot time, pool hit rate)
- Integration tests with warm pool enabled

---

## 11. Risk Assessment

### Technical Risks

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|------------|
| Kernel not bootable with Firecracer | Medium | High | Spike Phase 0 validates; fallback to QEMU kernel if needed |
| TAP device creation requires CAP_NET_ADMIN | Certain | Medium | Document requirement; provide slirp4netns fallback option |
| vsock not available on all Linux kernels | Low | High | Check `vhost_vsock` module availability; error early |
| Device-mapper thin provisioning not available | Medium | Low | Start with `cp --reflink=auto`; thin provisioning is Phase 4 optional |
| Firecracer binary licensing/compliance | Low | Medium | Firecracer is Apache 2.0; verify CI fleet compliance |

### Operational Risks

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|------------|
| Firecracer version drift | Medium | Low | Pin version; build from source via `firecracer-assets` target |
| Kernel updates break VM boot | Low | High | Ship kernel alongside Firecracer release; test in CI |
| No macOS support | Certain | Low | Intentional — Firecracer is Linux-only; vz and qemu cover macOS |
| Windows guest support | Low | Medium | Possible in future; would need Windows-compatible kernel |

---

## 12. Comparison: Firecracer vs QEMU vs VZ

| Concern | Container | QEMU | VZ | Firecracer (proposed) |
|---------|-----------|------|-----|----------------------|
| **Host platform** | Linux | All | macOS/arm64 | Linux only |
| **Boot time (cold)** | ~3s | ~1-2s | ~5s | ~125ms |
| **Boot time (warm)** | N/A | N/A | N/A | ~10ms (snapshot) |
| **Memory overhead** | ~50MB | ~30MB/VM | ~200MB/VM | ~5MB/VM |
| **Attack surface** | Medium (namespaces) | High (QEMU) | Low (VZ.framework) | Very low (~50K LOC) |
| **Network isolation** | iptables (shared kernel) | Hypervisor | Hypervisor | Hypervisor + jailer |
| **Max concurrent VMs** | ~100 (container) | ~20 (QEMU) | ~5 (VZ) | ~200 (Firecracer) |
| **Guest OS support** | Linux only | Linux, Windows, macOS | macOS, Linux | Linux only |
| **Shared filesystem** | Volume mount | 9p/virtiofs | None (socket pair) | None (vsock/MMDS) |
| **Signaling** | File-based | 9p file-based | File-based | vsock (Phase 2+) |
| **Snapshot support** | N/A | N/A | N/A | Full state (Phase 4) |
| **Jailer sandboxing** | N/A | No | No | Yes (optional) |
| **Best for** | Fast builds, same-arch | Universal compatibility | macOS hosts | High-density CI fleets |

---

## 13. Recommended Next Steps

1. **Human review:** Go through Section 7 (8 decisions) and provide guidance on each
2. **Create branch:** `feat/firecracker-driver` from `origin/main` (already done)
3. **Hand off to AI agent:** The agent can begin implementation immediately after Section 7 decisions are resolved
4. **Phase 0 spike:** Validate core concept (boot relay VM, vsock connectivity, simple build)
5. **Review spike results** with the team before committing to full implementation
6. **If spike succeeds:** Proceed to Phase 1 (minimal E2E)

---

## 14. Appendix: Relay Init Script (Firecracer Version)

The existing QEMU relay init script works with minor modifications for Firecracer:

```sh
#!/bin/sh
# Firecracer relay VM init (simplified from plan)

mount -t devtmpfs devtmpfs /dev
mount -t proc proc /proc
mount -t sysfs sysfs /sys
mount -t tmpfs tmpfs /tmp

# Network: eth0 = build link, eth1 = upstream
ip link set eth0 up
ip addr add 10.0.0.1/30 dev eth0

ip link set eth1 up
ip addr add 10.0.2.15/24 dev eth1
ip route add default via 10.0.2.1

# iptables (same as QEMU relay)
iptables -P FORWARD DROP
iptables -A FORWARD -m state --state ESTABLISHED,RELATED -j ACCEPT
iptables -t nat -A PREROUTING -i eth0 -p tcp --dport 80 -j REDIRECT --to-port 80
iptables -t nat -A PREROUTING -i eth0 -p tcp --dport 443 -j REDIRECT --to-port 443
iptables -t nat -A PREROUTING -i eth0 -p udp --dport 53 -j REDIRECT --to-port 53

# Signal readiness to host via vsock (CID 2 = host)
echo "ready" | socat - VSOCK-CONNECT:2:54 2>/dev/null || true

exec /relay
```

Key difference from QEMU relay: uses `eth0`/`eth1` (no PCI discovery needed) and signals readiness via vsock instead of writing to a shared volume.
