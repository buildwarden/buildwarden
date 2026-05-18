# Firecracker Driver Implementation Plan

## Open Questions

1. **Relay VM networking model**: Firecracker has no user-net equivalent (no NAT
   out of the box). The relay VM needs upstream internet access. Options:
   - TAP device on the host with iptables MASQUERADE (requires CAP_NET_ADMIN on
     host — acceptable for CI fleets?)
   - Slirp4netns in user-space (adds latency, avoids host capabilities)
   - Pre-configured bridge on the CI host that fleet operators provide

2. **Jailer ownership model**: The jailer expects a dedicated UID/GID per VM.
   For a pool of warm VMs, do we pre-allocate a UID range (e.g., 10000-10999)?
   Or run without the jailer in dev/CI and document jailer as production
   hardening?

3. **vsock CID allocation**: Each Firecracker VM needs a unique Context ID (CID)
   for virtio-vsock. With hundreds of concurrent builds, we need a host-level
   CID allocator. Options:
   - File-lock-based allocation from a range (simple, works for single host)
   - Systemd-based socket activation / cgroup-scoped
   - Random from a large range with collision retry

4. **Kernel and rootfs provisioning**: Should the driver:
   - Ship a pre-built relay kernel + rootfs (like QEMU ships kernel + initramfs)?
   - Build them at install time via a `make firecracker-assets` target?
   - Download from a release URL on first use?

5. **Root filesystem mutability**: Firecracker rootfs is a raw block device.
   For the build VM:
   - Use device-mapper thin provisioning (copy-on-write over base image)?
   - Create a full copy per build (simpler but slower for large images)?
   - Use overlayfs on a shared read-only base mount inside the VM (requires
     kernel support in guest)?

6. **MMDS version**: Firecracker supports MMDS v1 (no auth) and v2 (token-based,
   like IMDSv2). v2 is more secure but adds boot-time complexity. Which to use?
   Recommendation: v2 for build VM metadata, since the build script is untrusted.

7. **Snapshot base image lifecycle**: For warm pools, when does a base snapshot
   expire? On relay binary change? On kernel update? On config change?

---

## Architecture

### Two-VM Topology (Mirrors QEMU)

```
Host (orchestrator)
├── Firecracker Relay VM (Alpine, direct kernel boot, raw ext4 rootfs)
│   ├── eth0: TAP → host bridge/NAT (upstream internet)
│   ├── vsock CID=N: relay↔host signaling
│   └── virtio-net via TAP pair → Build VM
├── Firecracker Build VM (user-specified OS, raw ext4 rootfs)
│   ├── eth0: TAP pair → Relay VM (sole network path)
│   ├── vsock CID=M: build↔host signaling (heartbeat/exit)
│   └── MMDS: cloud-init-like metadata injection
└── Host process
    ├── Manages TAP devices and network plumbing
    ├── Watches vsock for heartbeat/exit signals
    └── Collects ledger output from relay rootfs
```

### Key Differences from QEMU

| Concern | QEMU | Firecracker |
|---------|------|-------------|
| Relay↔Build network | Unix socket stream netdev | TAP pair (veth-like) or macvtap |
| Relay↔Internet | QEMU user-net (built-in NAT) | Host TAP + iptables MASQUERADE |
| Shared filesystem | virtio-9p | Not available; use vsock or block device |
| Metadata injection | Cloud-init seed ISO | MMDS (169.254.169.254) or vsock push |
| Signaling | 9p shared directory (file-based) | vsock (stream-based) |
| Process model | QEMU is a single process per VM | firecracker binary + optional jailer |

### Why No 9p/virtiofs

Firecracker deliberately omits virtio-9p and virtiofs to minimize attack surface.
All data exchange between host and VM must go through:
- **vsock** (bidirectional stream, like TCP over a virtual socket)
- **Block devices** (raw images, read-only or read-write)
- **MMDS** (metadata service, JSON, max ~50KB, read-only from VM)
- **Network** (TAP device, standard IP traffic)

This means the signaling mechanism must change from file-based to vsock-based.

---

## Network Topology

### Option A: Dual-TAP with Host Bridge (Recommended)

```
                    ┌─────────────────────────────────┐
                    │           Host                   │
                    │                                  │
  ┌─────────┐      │  ┌───────────────────────────┐  │
  │ Internet│◄─────┼──┤ tap-relay-ext (MASQUERADE) │  │
  └─────────┘      │  └───────────┬───────────────┘  │
                    │              │                   │
                    │  ┌───────────▼───────────────┐  │
                    │  │     Relay VM (eth0)        │  │
                    │  │     10.0.0.1/30            │  │
                    │  │     eth1 = upstream        │  │
                    │  └───────────┬───────────────┘  │
                    │              │                   │
                    │  ┌───────────▼───────────────┐  │
                    │  │ tap-relay-build (bridge)   │  │
                    │  └───────────┬───────────────┘  │
                    │              │                   │
                    │  ┌───────────▼───────────────┐  │
                    │  │     Build VM (eth0)        │  │
                    │  │     10.0.0.2/30            │  │
                    │  └───────────────────────────┘  │
                    └─────────────────────────────────┘
```

**Implementation:**

```go
// createTAPPair creates a connected TAP pair for relay↔build communication.
// Returns the TAP device names.
func createTAPPair(buildID string) (relayTAP, buildTAP string, err error) {
    relayTAP = fmt.Sprintf("fc-r-%s", buildID[:8])
    buildTAP = fmt.Sprintf("fc-b-%s", buildID[:8])

    // Create a bridge to connect the two TAPs
    bridge := fmt.Sprintf("fc-br-%s", buildID[:8])

    cmds := [][]string{
        {"ip", "tuntap", "add", relayTAP, "mode", "tap"},
        {"ip", "tuntap", "add", buildTAP, "mode", "tap"},
        {"ip", "link", "add", bridge, "type", "bridge"},
        {"ip", "link", "set", relayTAP, "master", bridge},
        {"ip", "link", "set", buildTAP, "master", bridge},
        {"ip", "link", "set", relayTAP, "up"},
        {"ip", "link", "set", buildTAP, "up"},
        {"ip", "link", "set", bridge, "up"},
    }
    for _, args := range cmds {
        if err := exec.Command(args[0], args[1:]...).Run(); err != nil {
            return "", "", fmt.Errorf("network setup %v: %w", args, err)
        }
    }
    return relayTAP, buildTAP, nil
}
```

**Relay upstream (internet) TAP:**

```go
func createUpstreamTAP(buildID string) (tapName string, err error) {
    tapName = fmt.Sprintf("fc-u-%s", buildID[:8])
    cmds := [][]string{
        {"ip", "tuntap", "add", tapName, "mode", "tap"},
        {"ip", "addr", "add", "10.0.2.1/24", "dev", tapName},
        {"ip", "link", "set", tapName, "up"},
        // MASQUERADE for relay's outbound traffic
        {"iptables", "-t", "nat", "-A", "POSTROUTING",
            "-s", "10.0.2.0/24", "-o", defaultInterface(),
            "-j", "MASQUERADE"},
    }
    // ...
    return tapName, nil
}
```

### Option B: vsock-only (No Build VM TAP)

Instead of giving the build VM a TAP device that connects to the relay VM via
L2, use vsock for all build↔relay communication. The relay would expose an HTTP
proxy over vsock that the build VM connects to via a SOCKS/HTTP proxy in the
guest.

**Pros:** No TAP pair needed for the internal link, simpler network isolation.
**Cons:** Requires all guest traffic to go through an explicit proxy (breaks
tools that don't respect `http_proxy`). Not transparent.

**Recommendation:** Option A (dual-TAP) for transparent proxying parity with
QEMU. The relay VM's iptables REDIRECT rules work identically.

---

## Image Management

### Cloud Image to Raw ext4 Conversion

Firecracker requires raw block device images. The QEMU driver downloads QCOW2
cloud images; we need a conversion step.

```go
// convertToRaw converts a QCOW2 cloud image to raw ext4 for Firecracker.
// Caches the result alongside the QCOW2 source.
func convertToRaw(qcow2Path string) (string, error) {
    rawPath := strings.TrimSuffix(qcow2Path, ".qcow2") + ".raw"

    // Check cache
    if info, err := os.Stat(rawPath); err == nil {
        qInfo, _ := os.Stat(qcow2Path)
        if qInfo != nil && !qInfo.ModTime().After(info.ModTime()) {
            return rawPath, nil
        }
    }

    // Convert with qemu-img (available on all Linux CI hosts)
    cmd := exec.Command("qemu-img", "convert",
        "-f", "qcow2", "-O", "raw", qcow2Path, rawPath+".tmp")
    if out, err := cmd.CombinedOutput(); err != nil {
        return "", fmt.Errorf("qemu-img convert: %s: %w", out, err)
    }

    if err := os.Rename(rawPath+".tmp", rawPath); err != nil {
        return "", err
    }
    return rawPath, nil
}
```

### Copy-on-Write for Build Images

Since Firecracker does not support QCOW2 overlays, we need device-mapper or a
full copy:

**Option 1: Device-mapper thin provisioning (recommended for fleets)**

```go
// createThinSnapshot creates a device-mapper thin snapshot of the base image.
// Requires dm-thin-pool kernel module and dmsetup binary.
func createThinSnapshot(baseImage, buildID string) (string, error) {
    // Pool setup (one-time, reused across builds):
    //   dmsetup create warden-pool --table "0 $SIZE thin-pool /dev/loop0 /dev/loop1 128 0"
    // Per-build snapshot:
    //   dmsetup message warden-pool 0 "create_thin $THIN_ID"
    //   dmsetup create "warden-$buildID" --table "0 $SIZE thin /dev/mapper/warden-pool $THIN_ID"
    // ...
}
```

**Option 2: cp + truncate (simple, acceptable for reasonable image sizes)**

```go
func copyImage(src, dst string) error {
    // Use reflink if available (instant on btrfs/xfs with reflink)
    cmd := exec.Command("cp", "--reflink=auto", src, dst)
    if err := cmd.Run(); err != nil {
        // Fallback to regular copy
        return copyFile(src, dst)
    }
    return nil
}
```

**Recommendation:** Start with `cp --reflink=auto` (instant on modern
filesystems like btrfs/xfs, falls back to full copy on ext4). Graduate to
device-mapper thin provisioning when fleet operators need it.

### Relay VM rootfs

A minimal Alpine-based ext4 image (~15MB) containing:
- `/sbin/init` (the relay init script)
- `/usr/sbin/iptables` and kernel modules for netfilter
- The relay binary (injected at build time or via vsock at boot)

Built by `tools/firecracker-relay-vm/build-rootfs.sh`:
```sh
#!/bin/sh
# Creates a minimal ext4 rootfs for the Firecracker relay VM
# Output: tools/firecracker-relay-vm/output/relay-rootfs.ext4

truncate -s 64M "$OUTPUT"
mkfs.ext4 -L relay "$OUTPUT"
mount -o loop "$OUTPUT" /mnt
# ... install Alpine base, iptables, modules ...
cp tools/relay-vm/init /mnt/sbin/init
umount /mnt
e2fsck -f "$OUTPUT"
resize2fs -M "$OUTPUT"  # shrink to minimum
```

---

## Signaling

### Comparison

| Mechanism | Latency | Complexity | Security |
|-----------|---------|------------|----------|
| 9p files | ~1ms | Low (existing code) | Medium (symlink attacks) |
| vsock | ~0.1ms | Medium (new protocol) | High (no filesystem) |
| MMDS | ~1ms | Low (HTTP GET) | Medium (VM can read all metadata) |

### Recommendation: vsock for Signaling

vsock eliminates the filesystem-based attack surface (symlink races, TOCTOU on
signal files) and provides sub-millisecond latency. The protocol is simple:

**Host-side (driver):**
```go
// vsockListener listens for heartbeat and exit signals from VMs.
type vsockListener struct {
    relayConn net.Conn  // vsock connection to relay VM
    buildConn net.Conn  // vsock connection to build VM (optional)
    exitCh    chan int
    mu        sync.Mutex
    lastBeat  time.Time
}

const (
    vsockPortHeartbeat = 52
    vsockPortExit      = 53
    vsockPortRelay     = 54  // relay writes ledger completion signal
)

func (v *vsockListener) waitForBuild(ctx context.Context) (int, error) {
    for {
        select {
        case <-ctx.Done():
            return -1, ctx.Err()
        case code := <-v.exitCh:
            return code, nil
        case <-time.After(heartbeatTimeout):
            v.mu.Lock()
            last := v.lastBeat
            v.mu.Unlock()
            if time.Since(last) > heartbeatTimeout {
                return -1, fmt.Errorf("build VM unresponsive")
            }
        }
    }
}
```

**Guest-side (watcher.sh equivalent):**
```sh
#!/bin/sh
# Heartbeat via vsock (socat or custom binary)
/opt/warden/build.sh &
BUILD_PID=$!

while kill -0 "$BUILD_PID" 2>/dev/null; do
    echo "beat" | socat - VSOCK-CONNECT:2:52 2>/dev/null
    sleep 2
done

wait "$BUILD_PID"
CODE=$?
echo "$CODE" | socat - VSOCK-CONNECT:2:53
```

**However**, the relay VM still needs to communicate signals to the host (ledger
written, CA ready). Two options:

1. **vsock for relay→host signaling, keep file-based for relay→build**: The
   relay writes heartbeat/exit to vsock port on the host. The build VM
   communicates via HTTP to the relay (existing `/heartbeat` and `/exit`
   endpoints work unchanged).

2. **Pure vsock end-to-end**: Both relay and build use vsock. More complex but
   cleaner.

**Final recommendation:** Use approach (1). The relay already has HTTP endpoints
for heartbeat/exit that the build VM calls. The only new vsock path is
relay→host for "CA ready" and "ledger complete" signals. This minimizes changes
to the relay binary.

```go
// Host listens on vsock for relay readiness
func waitForRelayReady(ctx context.Context, relayCID uint32) error {
    l, err := vsock.Listen(vsockPortRelay, nil)
    if err != nil {
        return err
    }
    defer l.Close()

    connCh := make(chan net.Conn, 1)
    go func() {
        conn, _ := l.Accept()
        connCh <- conn
    }()

    select {
    case <-ctx.Done():
        return ctx.Err()
    case conn := <-connCh:
        defer conn.Close()
        buf := make([]byte, 64)
        n, _ := conn.Read(buf)
        if string(buf[:n]) == "ready" {
            return nil
        }
        return fmt.Errorf("unexpected relay signal: %s", buf[:n])
    case <-time.After(30 * time.Second):
        return fmt.Errorf("relay did not signal readiness")
    }
}
```

---

## Boot Sequence

Step-by-step from `StartBuild()` to first build command executing:

```
1. StartBuild(ctx, req) called
   │
2. ├── Allocate build ID, create output dir
   │
3. ├── Parallel preparation:
   │   ├── Cross-compile relay binary (linux/amd64, cached)
   │   ├── Cross-compile warden-io binary (linux/amd64, cached)
   │   ├── Resolve/download/convert build image to raw ext4
   │   └── Prepare build script + cloud-init metadata
   │
4. ├── Create network infrastructure:
   │   ├── Allocate 2 vsock CIDs (relay, build)
   │   ├── Create upstream TAP for relay (+ iptables MASQUERADE)
   │   ├── Create TAP pair for relay↔build (+ bridge)
   │   └── Write Firecracker network config JSONs
   │
5. ├── Prepare relay VM rootfs:
   │   ├── Copy base relay rootfs (reflink)
   │   ├── Inject relay binary into rootfs (mount + cp + umount, or vsock push)
   │   ├── Inject relay.env config
   │   └── Inject CA output path
   │
6. ├── Start Relay VM:
   │   ├── Write relay VM config JSON (kernel, rootfs, drives, net, vsock)
   │   ├── Launch firecracker --config-file relay.json
   │   ├── Wait for "ready" signal on vsock (CA generated, listeners up)
   │   └── Extract CA cert from relay rootfs or vsock
   │
7. ├── Prepare build VM rootfs:
   │   ├── Copy/snapshot base build image
   │   ├── Inject cloud-init seed via MMDS (network-config, user-data)
   │   └── Or: mount rootfs, inject warden-io + script, umount
   │
8. ├── Start Build VM:
   │   ├── Write build VM config JSON (kernel, rootfs, net, vsock, mmds)
   │   ├── Launch firecracker --config-file build.json
   │   ├── Push MMDS metadata via Firecracker API socket
   │   └── VM boots, cloud-init runs:
   │       ├── Configures network (10.0.0.2/30, gw 10.0.0.1)
   │       ├── Fetches CA from relay (http://artifacts/ca.pem)
   │       ├── Installs CA trust
   │       ├── Fetches context from relay (warden-io fetch)
   │       └── Executes build script with watcher (heartbeat loop)
   │
9. ├── Wait for completion:
   │   ├── Monitor heartbeat (relay writes to signal dir or vsock)
   │   ├── On exit signal: collect exit code
   │   └── On timeout/stall: kill both VMs
   │
10.└── Cleanup:
       ├── Stop build VM (SendCtrlAltDel or kill)
       ├── Stop relay VM
       ├── Collect ledger from relay rootfs (mount + cp)
       ├── Destroy TAP devices and bridge
       ├── Release vsock CIDs
       └── Return BuildResult
```

### Firecracker Configuration (Relay VM)

```json
{
  "boot-source": {
    "kernel_image_path": "/var/lib/warden/assets/vmlinux",
    "boot_args": "console=ttyS0 reboot=k panic=1 pci=off init=/sbin/init"
  },
  "drives": [
    {
      "drive_id": "rootfs",
      "path_on_host": "/tmp/warden-abc123/relay-rootfs.ext4",
      "is_root_device": true,
      "is_read_only": false
    }
  ],
  "network-interfaces": [
    {
      "iface_id": "build",
      "guest_mac": "AA:FC:00:00:00:01",
      "host_dev_name": "fc-r-abc12345"
    },
    {
      "iface_id": "upstream",
      "guest_mac": "AA:FC:00:00:00:02",
      "host_dev_name": "fc-u-abc12345"
    }
  ],
  "vsock": {
    "guest_cid": 100,
    "uds_path": "/tmp/warden-abc123/relay.vsock"
  },
  "machine-config": {
    "vcpu_count": 2,
    "mem_size_mib": 512
  }
}
```

### Firecracker Configuration (Build VM)

```json
{
  "boot-source": {
    "kernel_image_path": "/var/lib/warden/assets/vmlinux",
    "boot_args": "console=ttyS0 reboot=k panic=1 pci=off ds=nocloud"
  },
  "drives": [
    {
      "drive_id": "rootfs",
      "path_on_host": "/tmp/warden-abc123/build-rootfs.raw",
      "is_root_device": true,
      "is_read_only": false
    }
  ],
  "network-interfaces": [
    {
      "iface_id": "relay",
      "guest_mac": "AA:FC:00:00:01:01",
      "host_dev_name": "fc-b-abc12345"
    }
  ],
  "vsock": {
    "guest_cid": 101,
    "uds_path": "/tmp/warden-abc123/build.vsock"
  },
  "mmds-config": {
    "version": "V2",
    "network_interfaces": ["relay"]
  },
  "machine-config": {
    "vcpu_count": 4,
    "mem_size_mib": 4096
  }
}
```

---

## Jailer Integration

### What the Jailer Provides

1. **chroot** — Firecracker process sees only its own directory
2. **seccomp** — Syscall allowlist (already built into firecracker binary)
3. **cgroup isolation** — CPU/memory limits per VM
4. **UID/GID namespacing** — Each VM runs as a dedicated unprivileged user
5. **/dev protection** — Only `/dev/kvm`, `/dev/urandom`, `/dev/null` exposed

### Recommendation: Optional Jailer with Graceful Degradation

```go
type FirecrackerConfig struct {
    // UseJailer enables jailer-based sandboxing. Requires:
    // - jailer binary in PATH
    // - Pre-allocated UID range in /etc/subuid
    // - CAP_SYS_ADMIN for cgroup creation
    UseJailer bool

    // JailerConfig (only used when UseJailer=true)
    JailerUID   int
    JailerGID   int
    CgroupPath  string
    NetNS       string  // optional network namespace
}

func (d *Driver) startVM(cfg *vmConfig) (*vmProcess, error) {
    if d.config.UseJailer {
        return d.startWithJailer(cfg)
    }
    return d.startBare(cfg)
}

func (d *Driver) startWithJailer(cfg *vmConfig) (*vmProcess, error) {
    args := []string{
        "--id", cfg.VMID,
        "--exec-file", d.firecrackerBinary(),
        "--uid", fmt.Sprintf("%d", d.config.JailerUID),
        "--gid", fmt.Sprintf("%d", d.config.JailerGID),
        "--chroot-base-dir", cfg.ChrootDir,
    }
    if d.config.NetNS != "" {
        args = append(args, "--netns", d.config.NetNS)
    }
    if d.config.CgroupPath != "" {
        args = append(args, "--cgroup", d.config.CgroupPath)
    }

    cmd := exec.Command("jailer", args...)
    cmd.Stdin = nil
    cmd.Stdout = d.vmOutput()
    cmd.Stderr = d.vmOutput()

    if err := cmd.Start(); err != nil {
        return nil, fmt.Errorf("starting jailer: %w", err)
    }
    return &vmProcess{cmd: cmd, name: cfg.Name}, nil
}
```

### When to Use Jailer

| Environment | Jailer | Rationale |
|-------------|--------|-----------|
| Production CI fleet | Yes | Defense in depth; untrusted build code |
| Developer laptop (nested virt) | No | Simpler setup, adequate for testing |
| Integration tests | No | Cannot run jailer without real KVM |

---

## Warm VM Pool / Snapshot-Resume

### Design

Firecracker supports full VM snapshots (memory + device state) that can be
resumed in <5ms. Combined with ~100ms boot time, this enables:

1. **Cold start**: ~125ms (kernel boot + init + relay startup)
2. **Warm start (snapshot resume)**: ~10ms (restore memory image)

### Pool Architecture

```
┌─────────────────────────────────────────────┐
│              VM Pool Manager                  │
│                                              │
│  ┌─────────┐  ┌─────────┐  ┌─────────┐    │
│  │ Warm VM │  │ Warm VM │  │ Warm VM │    │
│  │ (relay) │  │ (relay) │  │ (relay) │    │
│  │ paused  │  │ paused  │  │ paused  │    │
│  └─────────┘  └─────────┘  └─────────┘    │
│                                              │
│  Base Snapshots:                             │
│  ├── relay-snapshot.mem (memory image)       │
│  ├── relay-snapshot.state (device state)     │
│  └── relay-rootfs.ext4 (CoW base)           │
│                                              │
│  Per-build:                                  │
│  ├── Restore from snapshot                   │
│  ├── Inject build-specific config via vsock  │
│  └── Resume                                  │
└─────────────────────────────────────────────┘
```

### Snapshot Creation Flow

```go
// CreateBaseSnapshot boots a relay VM to the "ready" state, then snapshots it.
// The snapshot can be restored in ~5ms for subsequent builds.
func (p *Pool) CreateBaseSnapshot() error {
    // 1. Boot relay VM normally
    proc, err := p.driver.startRelayVM(p.baseConfig)
    if err != nil {
        return err
    }

    // 2. Wait for relay to be ready (CA generated, listeners up)
    if err := waitForRelayReady(ctx, proc.cid); err != nil {
        return err
    }

    // 3. Pause the VM
    if err := p.apiCall(proc, "PATCH", "/vm", `{"state": "Paused"}`); err != nil {
        return err
    }

    // 4. Create snapshot
    snapReq := map[string]string{
        "snapshot_path": p.snapshotStatePath,
        "mem_file_path": p.snapshotMemPath,
        "snapshot_type": "Full",
    }
    if err := p.apiCall(proc, "PUT", "/snapshot/create", snapReq); err != nil {
        return err
    }

    proc.stop()
    return nil
}

// AcquireRelayVM restores a relay VM from snapshot for a new build.
func (p *Pool) AcquireRelayVM(buildID string) (*vmProcess, error) {
    // 1. Create CoW copy of relay rootfs
    rootfs := filepath.Join(p.workDir, buildID, "relay-rootfs.ext4")
    if err := copyImage(p.baseRootfs, rootfs); err != nil {
        return nil, err
    }

    // 2. Restore from snapshot
    cfg := &restoreConfig{
        SnapshotPath: p.snapshotStatePath,
        MemFilePath:  p.snapshotMemPath,
        Drives: []driveOverride{
            {DriveID: "rootfs", Path: rootfs},
        },
    }
    return p.restoreVM(cfg)
}
```

### Pool Manager

```go
type Pool struct {
    mu          sync.Mutex
    available   []*vmProcess  // pre-warmed, paused VMs
    maxSize     int
    driver      *Driver
    baseConfig  *vmConfig
    snapshotDir string
}

// Get returns a warm VM or boots a new one.
func (p *Pool) Get(ctx context.Context) (*vmProcess, error) {
    p.mu.Lock()
    if len(p.available) > 0 {
        vm := p.available[len(p.available)-1]
        p.available = p.available[:len(p.available)-1]
        p.mu.Unlock()
        // Resume the paused VM
        return vm, p.resumeVM(vm)
    }
    p.mu.Unlock()
    // Cold boot
    return p.driver.startRelayVM(p.baseConfig)
}

// Return pauses a VM and returns it to the pool (or destroys it).
func (p *Pool) Return(vm *vmProcess) {
    // Only pool relay VMs (build VMs are single-use)
    p.mu.Lock()
    defer p.mu.Unlock()
    if len(p.available) >= p.maxSize {
        vm.stop()
        return
    }
    // Pause and reset state
    p.pauseVM(vm)
    p.available = append(p.available, vm)
}
```

### Invalidation

Base snapshots are invalidated when:
- Relay binary hash changes (new build of `cmd/relay`)
- Kernel version changes
- Relay configuration (capture mode, etc.) changes

```go
func (p *Pool) isSnapshotValid() bool {
    hashFile := filepath.Join(p.snapshotDir, "relay.sha256")
    expected, err := os.ReadFile(hashFile)
    if err != nil {
        return false
    }
    actual, _ := hashFile(p.relayBinaryPath)
    return strings.TrimSpace(string(expected)) == actual
}
```

---

## Go Implementation Structure

### Package Layout

```
driver/firecracker/
├── driver.go          # Driver struct, StartBuild, Close (mirrors qemu/driver.go)
├── vm.go             # startRelayVM, startBuildVM, vmProcess, stop/wait
├── config.go         # Firecracker JSON config generation
├── network.go        # TAP creation, bridge setup, cleanup
├── images.go         # Image resolution, QCOW2→raw conversion, caching
├── vsock.go          # vsock signaling: heartbeat listener, exit listener
├── mmds.go           # MMDS metadata injection (cloud-init replacement)
├── pool.go           # Warm VM pool manager, snapshot/restore
├── jailer.go         # Jailer integration (optional sandboxing)
├── api.go            # Firecracker API socket client (HTTP over UDS)
├── cid.go            # vsock CID allocator
└── util.go           # Binary caching, file helpers (shared with qemu)
```

### Key Types

```go
package firecracker

import (
    "context"
    "net"
    "os/exec"
    "sync"
    "time"

    "warden/driver"
)

// Driver implements driver.Driver using Firecracker microVMs.
// Linux-only, requires /dev/kvm access.
type Driver struct {
    // FirecrackerBinary overrides path to firecracker binary.
    FirecrackerBinary string
    // JailerBinary overrides path to jailer binary (optional).
    JailerBinary string
    // UseJailer enables jailer sandboxing.
    UseJailer bool
    // Verbose enables VM console output on stderr.
    Verbose bool
    // PoolSize sets the warm VM pool size (0 = no pool).
    PoolSize int

    pool     *Pool
    cidAlloc *CIDAllocator
}

// vmProcess wraps a running Firecracker microVM.
type vmProcess struct {
    cmd      *exec.Cmd
    name     string
    apiSock  string   // path to Firecracker API socket
    vsockUDS string   // path to vsock UDS on host
    cid      uint32   // vsock context ID
    tapDevs  []string // TAP devices to clean up
    bridge   string   // bridge device to clean up
    rootfs   string   // rootfs path (for cleanup)
}

// CIDAllocator manages vsock CID assignment.
type CIDAllocator struct {
    mu       sync.Mutex
    lockFile string
    base     uint32 // e.g., 100
    max      uint32 // e.g., 65535
    inUse    map[uint32]bool
}

// firecrackerConfig is the JSON structure Firecracker expects.
type firecrackerConfig struct {
    BootSource    bootSource    `json:"boot-source"`
    Drives        []drive       `json:"drives"`
    NetworkIfaces []networkIf   `json:"network-interfaces"`
    Vsock         *vsockConfig  `json:"vsock,omitempty"`
    MMDSConfig    *mmdsConfig   `json:"mmds-config,omitempty"`
    MachineConfig machineConfig `json:"machine-config"`
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

### Key Functions

```go
func New() *Driver
func (d *Driver) Name() string                    // "firecracker"
func (d *Driver) StartBuild(ctx, req) (*BuildResult, error)
func (d *Driver) Exec(ctx, req) error             // ErrExecNotSupported initially
func (d *Driver) Close() error                    // drain pool, release CIDs

// vm.go
func (d *Driver) startRelayVM(cfg *relayVMConfig) (*vmProcess, error)
func (d *Driver) startBuildVM(cfg *buildVMConfig) (*vmProcess, error)
func (p *vmProcess) stop()
func (p *vmProcess) wait() error

// network.go
func createBuildNetwork(buildID string) (*networkState, error)
func (ns *networkState) cleanup()

// images.go
func resolveImage(imageRef string) (string, error)       // reuse qemu logic
func convertToRaw(qcow2Path string) (string, error)
func createBuildRootfs(basePath, buildID string) (string, error)

// vsock.go
func waitForRelayReady(ctx context.Context, vsockUDS string) error
func waitForBuild(ctx context.Context, vsockUDS string) (int, error)

// mmds.go
func pushMMDS(apiSock string, metadata map[string]any) error

// api.go
func apiPut(sock, path string, body any) error
func apiPatch(sock, path string, body any) error
func apiGet(sock, path string) ([]byte, error)

// cid.go
func (a *CIDAllocator) Acquire() (uint32, error)
func (a *CIDAllocator) Release(cid uint32)
```

---

## Security Considerations

### Threat Model Differences from QEMU

| Threat | QEMU Mitigation | Firecracker Mitigation |
|--------|-----------------|------------------------|
| VM escape | Large attack surface (~millions LOC) | ~50K LOC, minimal virtio |
| Host filesystem access | 9p (untrusted guest can traverse) | No shared FS; vsock only |
| Network bypass | iptables in relay VM | Same iptables + no user-net complexity |
| Resource exhaustion | QEMU has no built-in limits | Built-in rate limiters (net/disk) |
| Privilege escalation | Runs as user | Jailer: seccomp + chroot + dedicated UID |
| Side-channel (Spectre) | Host kernel mitigations | Same + KVM guest-first mode if available |

### New Attack Surface

1. **Host TAP devices**: Unlike QEMU's user-net (which is entirely in QEMU
   process space), Firecracker requires host-kernel TAP devices. A bug in
   virtio-net could theoretically reach the host kernel's network stack.
   Mitigation: jailer + network namespace isolation.

2. **vsock host listener**: The host process listens on vsock. A malicious VM
   could attempt to connect to other vsock listeners on the host.
   Mitigation: each VM gets a unique CID; host only listens on specific ports.

3. **API socket**: Firecracker exposes a Unix socket API for runtime
   configuration. Must be protected (0600, owned by warden process).
   Mitigation: socket created in temp directory with restrictive permissions.

4. **Snapshot memory images**: Contain full VM memory (may include secrets from
   previous builds if relay is pooled).
   Mitigation: relay VM never handles build secrets (only proxies TLS); base
   snapshot taken before any build traffic.

### Hardening Checklist

- [ ] Firecracker API socket: mode 0600, in per-build tmpdir
- [ ] TAP devices: created in dedicated network namespace when jailer is used
- [ ] vsock CID: unique per VM, released on cleanup
- [ ] Seccomp: enabled by default (Firecracker's built-in filter)
- [ ] Memory: balloon device or cgroup limit prevents single VM from exhausting host
- [ ] Disk I/O: rate limiter configured to prevent noisy-neighbor
- [ ] Network I/O: rate limiter configured (e.g., 100 Mbps per build)

---

## Testing Plan

### Unit Tests (No KVM Required)

```go
// config_test.go — verify JSON config generation
func TestRelayVMConfig(t *testing.T) {
    cfg := buildRelayConfig("test-id", "/tmp/rootfs.ext4", "tap0", "tap1", 100)
    data, _ := json.Marshal(cfg)
    // Verify structure matches Firecracker schema
}

// network_test.go — verify TAP/bridge command generation (mock exec)
func TestCreateBuildNetwork(t *testing.T) {
    // Use exec mock to verify ip/iptables commands are correct
}

// images_test.go — verify image path resolution and caching logic
func TestResolveImage(t *testing.T) {
    // Test local path, cloud image URL generation, cache hit/miss
}

// vsock_test.go — verify protocol handling with mock connections
func TestVsockSignaling(t *testing.T) {
    // Create unix socket pair, simulate heartbeat/exit protocol
}

// mmds_test.go — verify MMDS JSON construction
func TestMMDSMetadata(t *testing.T) {
    meta := buildMMDSPayload("build.sh content", networkConfig)
    // Verify structure
}
```

### Integration Tests (Require KVM)

```go
//go:build linux && integration

// TestRelayVMBoot boots a relay VM and verifies it reaches ready state.
func TestRelayVMBoot(t *testing.T) {
    if _, err := os.Stat("/dev/kvm"); err != nil {
        t.Skip("no /dev/kvm")
    }
    // ...
}

// TestEndToEnd runs a full build cycle.
func TestEndToEnd(t *testing.T) {
    // FROM alpine, RUN echo hello > /output/greeting.txt
}
```

### CI Strategy

1. **GitHub Actions**: Use `ubuntu-latest` runners with nested virtualization
   enabled (`-enable-kvm` flag in VM settings). GHA metal runners support this.

2. **Alternatively**: Run integration tests only in a self-hosted runner fleet
   that has KVM access. Gate on `WARDEN_INTEGRATION=1` env var.

3. **Mock-based testing for core logic**: The Firecracker API is HTTP over Unix
   socket. Mock it with `httptest.NewUnstartedServer` + Unix socket listener.

```go
func newMockFirecrackerAPI(t *testing.T) (socketPath string, cleanup func()) {
    tmpDir := t.TempDir()
    sock := filepath.Join(tmpDir, "firecracker.sock")

    l, err := net.Listen("unix", sock)
    require.NoError(t, err)

    mux := http.NewServeMux()
    mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
        // Record and respond to API calls
    })
    srv := &http.Server{Handler: mux}
    go srv.Serve(l)

    return sock, func() { srv.Close(); l.Close() }
}
```

---

## Dependencies

### Required on Host

| Binary | Package (Debian/Ubuntu) | Purpose |
|--------|-------------------------|---------|
| `firecracker` | [GitHub release](https://github.com/firecracker-microvm/firecracker/releases) | microVM hypervisor |
| `jailer` | Same release as firecracker | Optional sandboxing |
| `ip` | `iproute2` | TAP/bridge creation |
| `iptables` | `iptables` | NAT for upstream connectivity |
| `qemu-img` | `qemu-utils` | QCOW2 → raw conversion |
| `curl` | `curl` | Cloud image downloads |
| `mkfs.ext4` | `e2fsprogs` | Relay rootfs creation (build time only) |

### Required Kernel Features

- KVM (`/dev/kvm` accessible)
- TUN/TAP (`/dev/net/tun`)
- vsock (`vhost_vsock` module loaded)
- Bridge (`bridge` module, or built-in)
- NAT (`nf_nat`, `iptable_nat`)

### Go Dependencies

```go
// go.mod additions
require (
    github.com/mdlayher/vsock v1.2.1  // vsock client/listener
)
```

### Build-Time Assets

| Asset | Size | Built By |
|-------|------|----------|
| `relay-vmlinux` | ~8MB | `tools/firecracker-relay-vm/build-kernel.sh` |
| `relay-rootfs.ext4` | ~15MB | `tools/firecracker-relay-vm/build-rootfs.sh` |
| `build-vmlinux` | ~8MB | Shared with relay (same kernel) |

Note: Firecracker requires an uncompressed kernel (`vmlinux`), not the
compressed `vmlinuz` used by QEMU. The build scripts must produce both or a
separate kernel build target is needed.

### Makefile Targets

```makefile
.PHONY: firecracker-assets
firecracker-assets: firecracker-kernel firecracker-relay-rootfs

firecracker-kernel:
	tools/firecracker-relay-vm/build-kernel.sh

firecracker-relay-rootfs:
	tools/firecracker-relay-vm/build-rootfs.sh

.PHONY: test-firecracker
test-firecracker:
	go test -tags integration ./driver/firecracker/...
```

---

## Implementation Phases

### Phase 1: Minimal E2E (2-3 weeks)

- `driver.go`: Driver struct, StartBuild skeleton
- `config.go`: Firecracker config JSON generation
- `network.go`: TAP creation (without jailer/netns)
- `vm.go`: Start relay VM, start build VM, stop
- `images.go`: Reuse QEMU's `resolveImage` + add `convertToRaw`
- Relay rootfs build script
- File-based signaling (relay writes to rootfs, host mounts after stop)
- Manual TAP cleanup on failure

Goal: `warden build --driver firecracker` completes a simple build.

### Phase 2: vsock Signaling + MMDS (1-2 weeks)

- `vsock.go`: Live heartbeat/exit monitoring
- `mmds.go`: Cloud-init replacement via MMDS
- `api.go`: Runtime Firecracker API calls
- Remove need to mount rootfs for signal/ledger extraction

Goal: Real-time build monitoring, no post-hoc rootfs mounting.

### Phase 3: Jailer + Production Hardening (1-2 weeks)

- `jailer.go`: Jailer integration
- `cid.go`: CID allocator with file locking
- Network namespace support
- Rate limiters (network + disk)
- Graceful cleanup on all error paths

Goal: Safe for multi-tenant CI fleet deployment.

### Phase 4: Warm Pool + Snapshots (2-3 weeks)

- `pool.go`: Pool manager, snapshot creation, restore
- Snapshot invalidation logic
- Pool size tuning / backpressure
- Metrics (boot time, pool hit rate)

Goal: <10ms warm start for subsequent builds.

---

## Appendix: Relay VM Init (Firecracker Version)

The Firecracker relay VM init script differs from the QEMU version because:
- Network interfaces are `eth0`/`eth1` (no PCI discovery needed)
- No module loading needed (Firecracker kernel has everything built-in)
- Uses vsock to signal readiness to host

```sh
#!/bin/sh
# Firecracker relay VM init

mount -t devtmpfs devtmpfs /dev
mount -t proc proc /proc
mount -t sysfs sysfs /sys
mount -t tmpfs tmpfs /tmp

# Network: eth0 = build link, eth1 = upstream
ip link set eth0 up
ip addr add 10.0.0.1/30 dev eth0

ip link set eth1 up
# Upstream: static config (host assigns 10.0.2.15/24 via TAP)
ip addr add 10.0.2.15/24 dev eth1
ip route add default via 10.0.2.1

# Disable IPv6
echo 1 > /proc/sys/net/ipv6/conf/all/disable_ipv6
echo 1 > /proc/sys/net/ipv4/ip_forward

# iptables (same as QEMU relay)
iptables -P FORWARD DROP
iptables -A FORWARD -m state --state ESTABLISHED,RELATED -j ACCEPT
iptables -t nat -A PREROUTING -i eth0 -p tcp --dport 80 -j REDIRECT --to-port 80
iptables -t nat -A PREROUTING -i eth0 -p tcp --dport 443 -j REDIRECT --to-port 443
iptables -t nat -A PREROUTING -i eth0 -p udp --dport 53 -j REDIRECT --to-port 53
iptables -t nat -A PREROUTING -i eth0 -p tcp --dport 53 -j REDIRECT --to-port 53
iptables -A INPUT -i eth0 -p icmp -j DROP
iptables -t nat -A POSTROUTING -o eth1 -j MASQUERADE

echo "nameserver 10.0.2.1" > /etc/resolv.conf

# Signal readiness to host via vsock (CID 2 = host)
echo "ready" | socat - VSOCK-CONNECT:2:54 2>/dev/null || true

exec /relay
```
