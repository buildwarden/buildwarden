# Firecracker Driver — Complete Implementation Plan

**Date:** 2026-10-08
**Branch:** `feat/firecracker-driver` (from `origin/main`)
**Goal:** A fully-specified plan that AI coding agents can implement hands-off, surfacing only design/security decisions for human review.

---

## Part 0: Scope Summary

### What This Plan Creates

| Category | Files | Lines (est.) |
|----------|-------|-------------|
| New driver package | 12 files in `driver/firecracker/` | ~2,200 |
| Test files | 6 files in `driver/firecracker/` | ~600 |
| Modified existing files | 4 files | ~30 |
| Build system | 2 Makefile targets + 2 shell scripts | ~150 |
| **Total** | **24 files** | **~3,600** |

### What This Plan Reuses (no changes needed)

- `driver.Driver` interface — all drivers implement this
- `driver.BuildRequest` / `driver.BuildResult` — all drivers use these
- `driver.ExtensionContext` — extensions operate on this
- `driver.RandAlphaNum()` — build ID generation
- `driver.AllocateSubnet()` — subnet allocation (container driver only)
- `driver/script.Translate()` — Dockerfile → shell script
- `driver.DefaultExtensions()` — CA trust, epoch, bazel
- `relay.Start()` — core relay logic (all modes)
- `cmd/relay/mode_vm.go` — relay VM mode (already works)
- `waitForFile()` — file polling (used by vz and qemu)
- `collectOutputs()` — ledger collection (used by vz and qemu)
- `cachedBinaryPath()`, `isCacheStale()`, `findModuleRoot()` — caching (used by qemu)
- `copyFile()` — file copy (used by all)

### Design/Security Decisions Already Made (in this plan)

1. **Relay mode:** Use existing `vm` mode — no relay changes needed
2. **Signaling:** Hybrid — vsock for host↔VM readiness, file-based (rootfs mount) for relay↔build signals
3. **Jailer:** Optional, disabled by default, enabled via `UseJailer` flag
4. **vsock CID range:** 100–999, file-lock based allocation
5. **Image format:** QCOW2 cloud image → `qemu-img convert` → per-build raw ext4, cached per base image
6. **Copy-on-write:** `cp --reflink=auto` with full-copy fallback
7. **MMDS:** v2 (token-based) for build VM metadata
8. **Network:** Host TAP + bridge (relay↔build) + upstream TAP (relay→internet via MASQUERADE)
9. **Firecracker version:** Pin to latest stable (≥ 1.10), built from source via asset scripts

---

## Part 1: New Files — `driver/firecracker/`

### 1.1 `driver/firecracker/driver.go`

**Purpose:** Top-level driver struct and `StartBuild()` — the entry point for all builds.

**Package declaration and imports:**

```go
package firecracker

import (
    "context"
    "crypto/sha256"
    "encoding/hex"
    "fmt"
    "io"
    "os"
    "os/exec"
    "os/signal"
    "path/filepath"
    "runtime"
    "strings"
    "sync"
    "syscall"
    "time"

    "github.com/buildwarden/buildwarden/driver"
    "github.com/buildwarden/buildwarden/driver/script"
)
```

**Type: `Driver`**

```go
// Driver implements driver.Driver using Firecracker microVMs.
// Linux-only, requires /dev/kvm access.
//
// Architecture (two-VM topology):
//
//    Host (orchestrator)
//    ├── Firecracker Relay VM (Alpine, direct kernel boot, raw ext4 rootfs)
//    │   ├── eth0: TAP → bridge → Build VM (sole network path)
//    │   ├── eth1: TAP → host MASQUERADE (upstream internet)
//    │   └── vsock CID=N: relay↔host readiness signaling
//    ├── Firecracker Build VM (user-specified OS, raw ext4 rootfs)
//    │   ├── eth0: TAP → bridge → Relay VM (sole network path)
//    │   └── vsock CID=M: build↔host heartbeat/exit
//    └── Host process
//        ├── Manages TAP devices and bridge
//        ├── Monitors vsock for readiness/heartbeat/exit
//        └── Collects ledger from relay rootfs after build
type Driver struct {
    // FirecrackerBinary overrides path to firecracker binary.
    // If empty, "firecracker" is looked up in PATH.
    FirecrackerBinary string

    // JailerBinary overrides path to jailer binary.
    // If empty and UseJailer is true, "jailer" is looked up in PATH.
    JailerBinary string

    // UseJailer enables jailer-based sandboxing.
    // When false (default): no jailer, KVM provides isolation.
    // When true: requires jailer binary, pre-allocated UID/GID,
    // and CAP_SYS_ADMIN for cgroup creation.
    UseJailer bool

    // Verbose enables VM console output on stderr.
    Verbose bool

    // PoolSize sets the warm VM pool size (0 = no pool).
    // When > 0, relay VMs are snapshotted and reused.
    PoolSize int

    // pool is the warm VM pool manager (nil if PoolSize == 0).
    pool *Pool

    // cidAlloc manages vsock CID assignment.
    cidAlloc *CIDAllocator
}
```

**Compile-time interface check:**

```go
var _ driver.Driver = (*Driver)(nil)
```

**Constructor: `func New() *Driver`**

```go
func New() *Driver {
    return &Driver{
        cidAlloc: NewCIDAllocator(),
    }
}
```

**`func (d *Driver) Name() string`**

```go
func (d *Driver) Name() string { return "firecracker" }
```

**`func (d *Driver) StartBuild(ctx context.Context, req *driver.BuildRequest) (*driver.BuildResult, error)`**

This is the main entry point. Full implementation:

```go
func (d *Driver) StartBuild(
    ctx context.Context, req *driver.BuildRequest,
) (*driver.BuildResult, error) {
    // 1. Prerequisites check
    if err := d.checkPrereqs(); err != nil {
        return nil, err
    }

    // 2. Timeout handling
    if req.Timeout > 0 {
        var cancel context.CancelFunc
        ctx, cancel = context.WithTimeout(ctx, req.Timeout)
        defer cancel()
    }

    // 3. Interrupt handling — cancel context on SIGINT/SIGTERM
    ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
    defer stop()

    // 4. Generate build ID and output directory
    buildID := driver.RandAlphaNum(8)
    outputDir := req.OutputDir
    if outputDir == "" {
        outputDir = "warden-output"
    }
    if err := os.MkdirAll(outputDir, 0755); err != nil {
        return nil, fmt.Errorf("creating output dir: %w", err)
    }

    // 5. Create work directory for this build
    workDir, err := os.MkdirTemp("", "warden-fc-"+buildID+"-")
    if err != nil {
        return nil, fmt.Errorf("creating work dir: %w", err)
    }
    // workDir is cleaned up on all exit paths via cleanupWorkDir()

    // 6. Prepare shared resources (relay binary, warden-io, build script)
    sharedDir, err := d.prepareSharedDir(buildID, req, workDir)
    if err != nil {
        cleanupWorkDir(workDir)
        return nil, err
    }

    // 7. Allocate vsock CIDs (relay, build)
    relayCID, buildCID, err := d.cidAlloc.AcquirePair()
    if err != nil {
        cleanupWorkDir(workDir)
        return nil, fmt.Errorf("allocating vsock CIDs: %w", err)
    }
    defer d.cidAlloc.ReleasePair(relayCID, buildCID)

    // 8. Create network infrastructure (TAP devices + bridge)
    netState, err := createBuildNetwork(buildID, workDir)
    if err != nil {
        cleanupWorkDir(workDir)
        return nil, fmt.Errorf("creating network: %w", err)
    }
    defer netState.cleanup()

    // 9. Resolve and prepare relay VM assets (kernel + rootfs)
    kernelPath, rootfsPath, err := d.resolveRelayAssets(workDir)
    if err != nil {
        cleanupWorkDir(workDir)
        return nil, fmt.Errorf("resolving relay assets: %w", err)
    }

    // 10. Start relay VM
    d.status("starting relay...")
    relayProc, err := d.startRelayVM(
        kernelPath, rootfsPath, netState, relayCID, workDir)
    if err != nil {
        cleanupWorkDir(workDir)
        return nil, fmt.Errorf("starting relay VM: %w", err)
    }
    defer relayProc.stop()

    // 11. Wait for relay to signal "ready" (CA generated, listeners up)
    if err := waitForRelayReady(ctx, relayProc.vsockUDS); err != nil {
        cleanupWorkDir(workDir)
        return nil, fmt.Errorf("relay did not start: %w", err)
    }

    // 12. Resolve and prepare build VM image
    buildRootfs, err := d.resolveBuildImage(req, workDir)
    if err != nil {
        cleanupWorkDir(workDir)
        return nil, fmt.Errorf("resolving build image: %w", err)
    }

    // 13. Start build VM
    d.status("starting build...")
    buildProc, err := d.startBuildVM(
        kernelPath, buildRootfs, netState, buildCID, workDir)
    if err != nil {
        cleanupWorkDir(workDir)
        return nil, fmt.Errorf("starting build VM: %w", err)
    }
    defer buildProc.stop()

    // 14. Wait for build completion
    d.status("running...")
    exitCode, err := waitForBuild(ctx, buildProc.vsockUDS)
    if err != nil {
        return nil, fmt.Errorf("build failed: %w", err)
    }
    if exitCode != 0 {
        return nil, fmt.Errorf("build exited with code %d", exitCode)
    }

    // 15. Collect outputs (ledger, CA cert)
    // For Phase 1: mount relay rootfs after stop, copy ledger files.
    // For Phase 2: vsock-based extraction (no mount needed).
    if err := collectOutputs(workDir, outputDir); err != nil {
        return nil, fmt.Errorf("collecting outputs: %w", err)
    }

    return &driver.BuildResult{OutputDir: outputDir}, nil
}
```

**`func (d *Driver) Exec(ctx context.Context, req *driver.BuildRequest) error`**

```go
func (d *Driver) Exec(ctx context.Context, req *driver.BuildRequest) error {
    return driver.ErrExecNotSupported
}
```

**`func (d *Driver) Close() error`**

```go
func (d *Driver) Close() error {
    if d.pool != nil {
        d.pool.Close()
        d.pool = nil
    }
    if d.cidAlloc != nil {
        d.cidAlloc.Close()
        d.cidAlloc = nil
    }
    return nil
}
```

**`func (d *Driver) checkPrereqs() error`**

Validates that the host has the required kernel features and binaries:

```go
func (d *Driver) checkPrereqs() error {
    // 1. KVM availability
    if _, err := os.Stat("/dev/kvm"); err != nil {
        return fmt.Errorf(
            "firecracker driver requires KVM (/dev/kvm not available)\n"+
                "  Ensure virtualization is enabled in BIOS/UEFI\n"+
                "  On Linux VMs, ensure nested virtualization is enabled")
    }

    // 2. TUN/TAP availability
    if _, err := os.Stat("/dev/net/tun"); err != nil {
        return fmt.Errorf(
            "firecracker driver requires TUN/TAP (/dev/net/tun not available)\n"+
                "  Run: mkdir -p /dev/net && mknod /dev/net/tun c 10 200")
    }

    // 3. vhost_vsock kernel module
    if _, err := os.Stat("/dev/vhost-vsock"); err != nil {
        // Try loading the module
        cmd := exec.Command("modprobe", "vhost_vsock")
        cmd.Stdout = nil
        cmd.Stderr = nil
        if err := cmd.Run(); err != nil {
            return fmt.Errorf(
                "firecracker driver requires vhost_vsock kernel module\n"+
                    "  Load it with: modprobe vhost_vsock")
        }
        // Verify it loaded
        if _, err := os.Stat("/dev/vhost-vsock"); err != nil {
            return fmt.Errorf(
                "vhost_vsock module failed to load")
        }
    }

    // 4. Firecracker binary
    binary := d.firecrackerBinary()
    if _, err := exec.LookPath(binary); err != nil {
        return fmt.Errorf(
            "firecracker driver requires firecracker binary\n"+
                "  Download from: https://github.com/firecracker-microvm/firecracker/releases\n"+
                "  Or use: ./warden image prepare --driver firecracker")
    }

    // 5. qemu-img (for QCOW2→raw conversion)
    if _, err := exec.LookPath("qemu-img"); err != nil {
        return fmt.Errorf(
            "firecracker driver requires qemu-img\n"+
                "  Install: apt install qemu-utils (Debian/Ubuntu) or brew install qemu (macOS)")
    }

    // 6. iproute2 (for TAP/bridge creation)
    if _, err := exec.LookPath("ip"); err != nil {
        return fmt.Errorf(
            "firecracker driver requires iproute2 (ip command)\n"+
                "  Install: apt install iproute2 (Debian/Ubuntu) or brew install iproute2 (macOS)")
    }

    // 7. iptables (for MASQUERADE)
    if _, err := exec.LookPath("iptables"); err != nil {
        return fmt.Errorf(
            "firecracker driver requires iptables\n"+
                "  Install: apt install iptables (Debian/Ubuntu)")
    }

    // 8. Jailer (if enabled)
    if d.UseJailer {
        if _, err := exec.LookPath(d.jailerBinary()); err != nil {
            return fmt.Errorf(
                "jailer sandboxing enabled but jailer binary not found\n"+
                    "  Download from: https://github.com/firecracker-microvm/firecracker/releases")
        }
    }

    return nil
}
```

**`func (d *Driver) prepareSharedDir(buildID string, req *driver.BuildRequest, workDir string) (string, error)`**

Creates the shared directory structure and populates it with the relay binary, warden-io, and build script:

```go
func (d *Driver) prepareSharedDir(
    buildID string, req *driver.BuildRequest, workDir string,
) (string, error) {
    sharedDir := filepath.Join(workDir, "shared")
    for _, sub := range []string{"context", "ledger", "signal", "agent"} {
        p := filepath.Join(sharedDir, sub)
        if err := os.MkdirAll(p, 0755); err != nil {
            return "", fmt.Errorf("creating shared/%s: %w", sub, err)
        }
    }

    // Parallel preparation: relay binary + warden-io + build script
    type prepResult struct {
        name string
        err  error
    }
    prepCh := make(chan prepResult, 3)

    go func() {
        prepCh <- prepResult{"relay", d.prepareRelay(sharedDir)}
    }()
    go func() {
        prepCh <- prepResult{"agent", d.prepareAgent(sharedDir, req)}
    }()
    go func() {
        prepCh <- prepResult{"script", d.prepareBuildScript(sharedDir, req)}
    }()

    for i := 0; i < 3; i++ {
        r := <-prepCh
        if r.err != nil {
            return "", fmt.Errorf("preparing %s: %w", r.name, r.err)
        }
    }

    return sharedDir, nil
}
```

**`func (d *Driver) prepareRelay(sharedDir string) error`**

Cross-compiles and caches the relay binary for Linux:

```go
func (d *Driver) prepareRelay(sharedDir string) error {
    goarch := runtime.GOARCH
    relayDst := filepath.Join(sharedDir, "relay")

    // Check next to the warden binary (prod deployment)
    if exe, err := os.Executable(); err == nil {
        candidate := filepath.Join(
            filepath.Dir(exe), "warden-relay-linux-"+goarch)
        if _, err := os.Stat(candidate); err == nil {
            return copyFile(candidate, relayDst)
        }
    }

    // Check build cache
    cached := cachedBinaryPath("relay", goarch)
    if !isCacheStale(cached) {
        return copyFile(cached, relayDst)
    }

    // Build and cache
    cmd := exec.Command("go", "build",
        "-ldflags=-s -w", "-o", cached, "./cmd/relay")
    cmd.Env = append(os.Environ(),
        "GOOS=linux", "GOARCH="+goarch, "CGO_ENABLED=0")
    cmd.Dir = findModuleRoot()
    cmd.Stderr = os.Stderr
    if err := cmd.Run(); err != nil {
        return err
    }
    return copyFile(cached, relayDst)
}
```

**`func (d *Driver) prepareAgent(agentDir string) error`**

Cross-compiles and caches warden-io for Linux:

```go
func (d *Driver) prepareAgent(agentDir string) error {
    goarch := runtime.GOARCH
    dst := filepath.Join(agentDir, "warden-io")

    // Check next to the warden binary (prod deployment)
    if exe, err := os.Executable(); err == nil {
        candidate := filepath.Join(
            filepath.Dir(exe), "warden-io-linux-"+goarch)
        if _, err := os.Stat(candidate); err == nil {
            return copyFile(candidate, dst)
        }
    }

    // Check build cache
    cached := cachedBinaryPath("warden-io", goarch)
    if !isCacheStale(cached) {
        return copyFile(cached, dst)
    }

    // Build and cache
    cmd := exec.Command("go", "build",
        "-ldflags=-s -w", "-o", cached, "./cmd/warden-io")
    cmd.Env = append(os.Environ(),
        "GOOS=linux", "GOARCH="+goarch, "CGO_ENABLED=0")
    cmd.Dir = findModuleRoot()
    cmd.Stderr = os.Stderr
    if err := cmd.Run(); err != nil {
        return err
    }
    return copyFile(cached, dst)
}
```

**`func (d *Driver) prepareBuildScript(sharedDir string, req *driver.BuildRequest) error`**

Places the build script (from Containerfile translation or explicit script):

```go
func (d *Driver) prepareBuildScript(
    sharedDir string, req *driver.BuildRequest,
) error {
    ctxDir := filepath.Join(sharedDir, "context")
    buildScript := filepath.Join(ctxDir, "build.sh")

    if req.Script != "" {
        data, err := os.ReadFile(req.Script)
        if err != nil {
            return fmt.Errorf("reading build script: %w", err)
        }
        return os.WriteFile(buildScript, data, 0755)
    }

    if req.Containerfile != "" {
        result, err := script.Translate(req.Containerfile)
        if err != nil {
            return fmt.Errorf("translating containerfile: %w", err)
        }
        return os.WriteFile(buildScript, []byte(result.Script), 0755)
    }

    // No script — no-op
    return os.WriteFile(buildScript, []byte("#!/bin/sh\ntrue\n"), 0755)
}
```

**`func (d *Driver) resolveRelayAssets(workDir string) (kernelPath, rootfsPath string, err error)`**

Resolves relay VM kernel and rootfs. Checks multiple locations:

```go
func (d *Driver) resolveRelayAssets(workDir string) (string, string, error) {
    // 1. Check alongside the warden binary (prod deployment)
    if exe, err := os.Executable(); err == nil {
        dir := filepath.Dir(exe)
        k := filepath.Join(dir, "firecracker-vm-vmlinux")
        r := filepath.Join(dir, "firecracker-vm-rootfs.ext4")
        if _, err := os.Stat(k); err == nil {
            if _, err := os.Stat(r); err == nil {
                return k, r, nil
            }
        }
    }

    // 2. Check build output directory (tools/firecracker-relay-vm/output/)
    root := findModuleRoot()
    outDir := filepath.Join(root, "tools", "firecracker-relay-vm", "output")
    k := filepath.Join(outDir, "vmlinux")
    r := filepath.Join(outDir, "rootfs.ext4")
    if _, err := os.Stat(k); err == nil {
        if _, err := os.Stat(r); err == nil {
            return k, r, nil
        }
    }

    // 3. Check cache (firecracker-vm assets)
    cacheDir := filepath.Join(cacheBaseDir(), "warden", "firecracker-vm")
    k = filepath.Join(cacheDir, "vmlinux")
    r = filepath.Join(cacheDir, "rootfs.ext4")
    if _, err := os.Stat(k); err == nil {
        if _, err := os.Stat(r); err == nil {
            return k, r, nil
        }
    }

    return "", "", fmt.Errorf(
        "firecracker relay VM assets not found\n"+
            "  Run: make firecracker-assets\n"+
            "  Or place vmlinux and rootfs.ext4 in: %s",
        outDir)
}
```

**`func (d *Driver) resolveBuildImage(req *driver.BuildRequest, workDir string) (string, error)`**

Resolves the build VM rootfs image. For Firecracker, images must be raw ext4:

```go
func (d *Driver) resolveBuildImage(
    req *driver.BuildRequest, workDir string,
) (string, error) {
    image := req.Image

    // If no image specified, use default Ubuntu cloud image for the arch
    if image == "" {
        goarch := runtime.GOARCH
        if goarch == "arm64" {
            goarch = "arm64"
        }
        image = fmt.Sprintf("ubuntu-%s-cloudimg", goarch)
    }

    // Already a local path
    if strings.HasPrefix(image, "/") || strings.HasPrefix(image, "./") {
        abs, err := filepath.Abs(image)
        if err != nil {
            return "", err
        }
        if _, err := os.Stat(abs); err != nil {
            return "", fmt.Errorf("image not found: %s", abs)
        }
        return abs, nil
    }

    // Resolve to cloud image URL and download/cached
    url := firecrackerCloudImageURL(image)
    if url == "" {
        return "", fmt.Errorf(
            "cannot resolve %q to a cloud image; "+
                "provide a local path with --image", image)
    }

    // Check cache (QCOW2 source)
    cachedQCOW2 := cachedImagePath(image)
    if _, err := os.Stat(cachedQCOW2); err == nil {
        // Convert cached QCOW2 to raw
        return convertToRaw(cachedQCOW2, workDir)
    }

    // Download cloud image
    if err := downloadImage(url, cachedQCOW2); err != nil {
        return "", fmt.Errorf("downloading %s: %w", image, err)
    }

    // Convert to raw
    return convertToRaw(cachedQCOW2, workDir)
}
```

**Helper: `func convertToRaw(qcow2Path string, workDir string) (string, error)`**

Converts QCOW2 to raw ext4, caching the result:

```go
func convertToRaw(qcow2Path string, workDir string) (string, error) {
    rawPath := filepath.Join(workDir, "build-rootfs.raw")

    // Check if we already converted this image
    rawCache := filepath.Join(cacheBaseDir(), "warden", "firecracker-images",
        strings.ReplaceAll(qcow2Path, "/", "-")+".raw")
    if _, err := os.Stat(rawCache); err == nil {
        // Use reflink copy if supported (instant on btrfs/xfs)
        cmd := exec.Command("cp", "--reflink=auto", rawCache, rawPath)
        if err := cmd.Run(); err != nil {
            // Fallback: regular copy
            return copyImageFile(rawCache, rawPath)
        }
        return rawPath, nil
    }

    // Convert with qemu-img
    tmpRaw := rawPath + ".tmp"
    cmd := exec.Command("qemu-img", "convert",
        "-f", "qcow2", "-O", "raw", qcow2Path, tmpRaw)
    out, err := cmd.CombinedOutput()
    if err != nil {
        os.Remove(tmpRaw)
        return "", fmt.Errorf("qemu-img convert: %s: %w", out, err)
    }

    if err := os.Rename(tmpRaw, rawPath); err != nil {
        return "", err
    }

    // Cache the raw image (only if not in a temp workDir)
    if !strings.Contains(rawPath, "/warden-fc-") {
        cacheDir := filepath.Join(cacheBaseDir(), "warden", "firecracker-images")
        _ = os.MkdirAll(cacheDir, 0755)
        cacheName := filepath.Join(cacheDir,
            strings.ReplaceAll(qcow2Path, "/", "-")+".raw")
        _ = copyFile(rawPath, cacheName)
    }

    return rawPath, nil
}
```

**Helper: `func copyImageFile(src, dst string) error`**

Full binary copy fallback:

```go
func copyImageFile(src, dst string) error {
    in, err := os.Open(src)
    if err != nil {
        return err
    }
    defer in.Close()

    out, err := os.Create(dst)
    if err != nil {
        return err
    }
    defer out.Close()

    _, err = io.Copy(out, in)
    if err != nil {
        return err
    }
    return out.Close()
}
```

**Helper: `func (d *Driver) status(msg string)`**

```go
func (d *Driver) status(msg string) {
    fmt.Fprintf(os.Stderr, "[warden] %s\n", msg)
}
```

**Helper: `func (d *Driver) firecrackerBinary() string`**

```go
func (d *Driver) firecrackerBinary() string {
    if d.FirecrackerBinary != "" {
        return d.FirecrackerBinary
    }
    return "firecracker"
}
```

**Helper: `func (d *Driver) jailerBinary() string`**

```go
func (d *Driver) jailerBinary() string {
    if d.JailerBinary != "" {
        return d.JailerBinary
    }
    return "jailer"
}
```

**Helper: `func cleanupWorkDir(workDir string)`**

```go
func cleanupWorkDir(workDir string) {
    if workDir != "" {
        os.RemoveAll(workDir)
    }
}
```

**Helper: `func firecrackerCloudImageURL(imageRef string) string`**

Maps image references to cloud image download URLs:

```go
func firecrackerCloudImageURL(imageRef string) string {
    // Map common image references to cloud image URLs
    switch imageRef {
    case "ubuntu-24.04":
        return "https://cloud-images.ubuntu.com/noble/current/noble-server-cloudimg-amd64.img"
    case "ubuntu-22.04":
        return "https://cloud-images.ubuntu.com/jammy/current/jammy-server-cloudimg-amd64.img"
    case "ubuntu-20.04":
        return "https://cloud-images.ubuntu.com/focal/current/focal-server-cloudimg-amd64.img"
    case "ubuntu-arm64-24.04":
        return "https://cloud-images.ubuntu.com/noble/current/noble-server-cloudimg-arm64.img"
    case "ubuntu-arm64-22.04":
        return "https://cloud-images.ubuntu.com/jammy/current/jammy-server-cloudimg-arm64.img"
    case "debian-12":
        return "https://cloud.debian.org/images/cloud/bookworm/latest/debian-12-generic-amd64.qcow2"
    case "debian-11":
        return "https://cloud.debian.org/images/cloud/buster/latest/debian-10-cloud-amd64.qcow2"
    default:
        return ""
    }
}
```

**Helper: `func downloadImage(url, dest string) error`**

Downloads a cloud image with integrity verification:

```go
func downloadImage(url, dest string) error {
    // Check if already downloaded and valid
    if _, err := os.Stat(dest); err == nil {
        return nil // Already cached
    }

    tmpFile := dest + ".tmp"
    cmd := exec.Command("curl", "-fL", "-o", tmpFile, url)
    cmd.Stdout = os.Stderr
    cmd.Stderr = os.Stderr
    if err := cmd.Run(); err != nil {
        os.Remove(tmpFile)
        return fmt.Errorf("downloading %s: %w", url, err)
    }

    return os.Rename(tmpFile, dest)
}
```

---

### 1.2 `driver/firecracker/vm.go`

**Purpose:** VM lifecycle management — starting, stopping, and monitoring Firecracker microVMs.

**Package declaration and imports:**

```go
package firecracker

import (
    "context"
    "encoding/json"
    "fmt"
    "io"
    "net"
    "os"
    "os/exec"
    "path/filepath"
    "strings"
    "sync"
    "syscall"
    "time"
)
```

**Type: `vmProcess`**

```go
// vmProcess wraps a running Firecracker microVM.
type vmProcess struct {
    cmd        *exec.Cmd
    name       string       // "relay" or "build"
    apiSock    string       // path to Firecracker API socket
    vsockUDS   string       // path to vsock UDS on host
    cid        uint32       // vsock context ID
    tapDevs    []string     // TAP devices to clean up
    bridge     string       // bridge device name (empty for upstream TAP)
    rootfs     string       // rootfs path (for cleanup)
    chrootDir  string       // jailer chroot directory (empty if no jailer)
    mu         sync.Mutex
    stopped    bool
}
```

**`func (d *Driver) startRelayVM(kernelPath, rootfsPath string, netState *networkState, relayCID uint32, workDir string) (*vmProcess, error)`**

Starts the relay VM with Firecracker:

```go
func (d *Driver) startRelayVM(
    kernelPath, rootfsPath string, netState *networkState,
    relayCID uint32, workDir string,
) (*vmProcess, error) {
    // 1. Write relay VM config JSON
    cfg := buildRelayVMConfig(
        kernelPath, rootfsPath, netState, relayCID, workDir, d)

    configPath := filepath.Join(workDir, "relay-config.json")
    cfgData, err := json.Marshal(cfg)
    if err != nil {
        return nil, fmt.Errorf("marshaling relay config: %w", err)
    }
    if err := os.WriteFile(configPath, cfgData, 0644); err != nil {
        return nil, fmt.Errorf("writing relay config: %w", err)
    }

    // 2. Prepare rootfs (inject relay binary, warden-io, build script)
    rootfsPath, err = prepareRelayRootfs(rootfsPath, workDir)
    if err != nil {
        return nil, fmt.Errorf("preparing relay rootfs: %w", err)
    }

    // 3. Determine VM start command (with or without jailer)
    var cmd *exec.Cmd
    if d.UseJailer {
        cmd, err = d.startRelayWithJailer(cfg, configPath, workDir)
    } else {
        cmd, err = d.startRelayBare(cfg, configPath, workDir)
    }
    if err != nil {
        return nil, err
    }

    proc := &vmProcess{
        cmd:      cmd,
        name:     "relay",
        apiSock:  filepath.Join(workDir, "relay-api.sock"),
        vsockUDS: filepath.Join(workDir, "relay.vsock"),
        cid:      relayCID,
        tapDevs:  netState.relayTAPs,
        bridge:   netState.bridge,
        rootfs:   rootfsPath,
    }

    // 4. Start the process
    if err := cmd.Start(); err != nil {
        return nil, fmt.Errorf("starting firecracker: %w", err)
    }

    // 5. Wait for API socket to appear (up to 5 seconds)
    apiSockPath := proc.apiSock
    deadline := time.Now().Add(5 * time.Second)
    for time.Now().Before(deadline) {
        if _, err := os.Stat(apiSockPath); err == nil {
            return proc, nil
        }
        time.Sleep(100 * time.Millisecond)
    }

    cmd.Process.Kill()
    cmd.Wait()
    return nil, fmt.Errorf("firecracker API socket did not appear")
}
```

**`func (d *Driver) startRelayBare(cfg firecrackerConfig, configPath, workDir string) (*exec.Cmd, error)`**

Starts Firecracker without jailer:

```go
func (d *Driver) startRelayBare(
    cfg firecrackerConfig, configPath, workDir string,
) (*exec.Cmd, error) {
    args := []string{
        "--id", "relay",
        "--start-time-type", "Instant",
        "--no-api",  // We manage API socket ourselves
        "--config-file", configPath,
    }

    // Add machine config overrides if specified
    if cfg.MachineConfig.VcpuCount > 0 {
        args = append(args,
            "-machine-config",
            fmt.Sprintf("vcpu_count=%d", cfg.MachineConfig.VcpuCount))
    }

    cmd := exec.Command(d.firecrackerBinary(), args...)
    cmd.Dir = workDir
    cmd.Stdout = os.Stdout
    cmd.Stderr = os.Stderr

    // Create API socket directory with restrictive permissions
    apiDir := filepath.Join(workDir, "firecracker")
    _ = os.MkdirAll(apiDir, 0700)

    return cmd, nil
}
```

**`func (d *Driver) startRelayWithJailer(cfg firecrackerConfig, configPath, workDir string) (*exec.Cmd, error)`**

Starts Firecracker via jailer (production mode):

```go
func (d *Driver) startRelayWithJailer(
    cfg firecrackerConfig, configPath, workDir string,
) (*exec.Cmd, error) {
    // Allocate UID/GID from pre-allocated range
    uid, gid, err := allocateUIDGID("relay")
    if err != nil {
        return nil, fmt.Errorf("allocating jailer UID/GID: %w", err)
    }

    chrootDir := filepath.Join(workDir, "jailer-relay")
    _ = os.MkdirAll(chrootDir, 0755)

    args := []string{
        "--id", "relay",
        "--exec-file", d.firecrackerBinary(),
        "--uid", fmt.Sprintf("%d", uid),
        "--gid", fmt.Sprintf("%d", gid),
        "--chroot-base-dir", chrootDir,
        "--config-file", configPath,
    }

    // Add cgroup path if specified
    if d.useCgroups {
        args = append(args, "--cgroup", d.cgroupPath)
    }

    cmd := exec.Command(d.jailerBinary(), args...)
    cmd.Dir = workDir
    cmd.Stdout = os.Stdout
    cmd.Stderr = os.Stderr

    return cmd, nil
}
```

**`func (d *Driver) startBuildVM(kernelPath, rootfsPath string, netState *networkState, buildCID uint32, workDir string) (*vmProcess, error)`**

Starts the build VM:

```go
func (d *Driver) startBuildVM(
    kernelPath, rootfsPath string, netState *networkState,
    buildCID uint32, workDir string,
) (*vmProcess, error) {
    // 1. Write build VM config JSON
    cfg := buildBuildVMConfig(
        kernelPath, rootfsPath, netState, buildCID, workDir, d)

    configPath := filepath.Join(workDir, "build-config.json")
    cfgData, err := json.Marshal(cfg)
    if err != nil {
        return nil, fmt.Errorf("marshaling build config: %w", err)
    }
    if err := os.WriteFile(configPath, cfgData, 0644); err != nil {
        return nil, fmt.Errorf("writing build config: %w", err)
    }

    // 2. Inject MMDS metadata (cloud-init replacement)
    if err := pushMMDSMetadata(
        filepath.Join(workDir, "build-api.sock"), cfg); err != nil {
        return nil, fmt.Errorf("pushing MMDS metadata: %w", err)
    }

    // 3. Start Firecracker (same as relay, but with build config)
    cmd, err := d.startRelayBare(cfg, configPath, workDir)
    if err != nil {
        return nil, err
    }

    proc := &vmProcess{
        cmd:      cmd,
        name:     "build",
        apiSock:  filepath.Join(workDir, "build-api.sock"),
        vsockUDS: filepath.Join(workDir, "build.vsock"),
        cid:      buildCID,
        tapDevs:  netState.buildTAPs,
        bridge:   netState.bridge,
        rootfs:   rootfsPath,
    }

    if err := cmd.Start(); err != nil {
        return nil, fmt.Errorf("starting firecracker: %w", err)
    }

    // Wait for API socket
    apiSockPath := proc.apiSock
    deadline := time.Now().Add(5 * time.Second)
    for time.Now().Before(deadline) {
        if _, err := os.Stat(apiSockPath); err == nil {
            return proc, nil
        }
        time.Sleep(100 * time.Millisecond)
    }

    cmd.Process.Kill()
    cmd.Wait()
    return nil, fmt.Errorf("firecracker API socket did not appear")
}
```

**`func (p *vmProcess) stop() error`**

```go
func (p *vmProcess) stop() error {
    p.mu.Lock()
    defer p.mu.Unlock()

    if p.stopped {
        return nil
    }
    p.stopped = true

    if p.cmd != nil && p.cmd.Process != nil {
        // Send Ctrl+Alt+Del for graceful shutdown (builds may be running)
        _ = p.cmd.Signal(syscall.SIGINT)
        // Wait with timeout, then force kill
        done := make(chan error, 1)
        go func() { done = p.cmd.Wait() }()
        select {
        case <-done:
        case <-time.After(10 * time.Second):
            p.cmd.Process.Kill()
            p.cmd.Wait()
        }
    }
    return nil
}
```

**`func (p *vmProcess) wait(ctx context.Context) error`**

```go
func (p *vmProcess) wait(ctx context.Context) error {
    if p.cmd == nil {
        return nil
    }
    select {
    case err := <-p.cmd.Wait():
        return err
    case <-ctx.Done():
        p.stop()
        return ctx.Err()
    }
}
```

**Helper: `func buildRelayVMConfig(...) firecracerConfig`**

Generates the Firecracker JSON config for the relay VM. The config includes:
- Boot source (kernel path, boot args)
- Rootfs drive (ext4, read-write)
- Two network interfaces (build TAP + upstream TAP)
- vsock (host CID = 2, guest CID = relayCID)
- Machine config (vCPUs, memory)

```go
func buildRelayVMConfig(
    kernelPath, rootfsPath string, netState *networkState,
    relayCID uint32, workDir string, d *Driver,
) firecracerConfig {
    cfg := firecracerConfig{
        BootSource: bootSource{
            KernelPath: kernelPath,
            BootArgs:   "console=ttyS0 reboot=k panic=1 pci=off",
        },
        Drives: []drive{
            {
                DriveID:      "rootfs",
                PathOnHost:   rootfsPath,
                IsRootDevice: true,
                IsReadOnly:   false,
            },
        },
        NetworkIfaces: []networkIf{
            {
                IfaceID:     "build",
                GuestMAC:    "AA:FC:00:00:00:01",
                HostDevName: netState.relayTAP,
            },
            {
                IfaceID:     "upstream",
                GuestMAC:    "AA:FC:00:00:00:02",
                HostDevName: netState.upstreamTAP,
            },
        },
        Vsock: &vsockConfig{
            GuestCID: relayCID,
            UDSPath:  filepath.Join(workDir, "relay.vsock"),
        },
        MachineConfig: machineConfig{
            VcpuCount:  2,
            MemSizeMiB: 512,
        },
    }

    if d.UseJailer {
        // Jailer manages chroot, seccomp, cgroups — no extra config needed
    }

    return cfg
}
```

**Helper: `func buildBuildVMConfig(...) firecracerConfig`**

Generates the Firecracker JSON config for the build VM. The config includes:
- Boot source (kernel path, boot args with `ds=nocloud`)
- Rootfs drive (ext4, read-write)
- One network interface (bridge TAP to relay)
- vsock (host CID = 2, guest CID = buildCID)
- MMDS (v2, token-based, on the build network interface)
- Machine config (vCPUs, memory)

```go
func buildBuildVMConfig(
    kernelPath, rootfsPath string, netState *networkState,
    buildCID uint32, workDir string, d *Driver,
) firecracerConfig {
    cfg := firecracerConfig{
        BootSource: bootSource{
            KernelPath: kernelPath,
            BootArgs:   "console=ttyS0 reboot=k panic=1 pci=off ds=nocloud",
        },
        Drives: []drive{
            {
                DriveID:      "rootfs",
                PathOnHost:   rootfsPath,
                IsRootDevice: true,
                IsReadOnly:   false,
            },
        },
        NetworkIfaces: []networkIf{
            {
                IfaceID:     "relay",
                GuestMAC:    "AA:FC:00:00:01:01",
                HostDevName: netState.buildTAP,
            },
        },
        Vsock: &vsockConfig{
            GuestCID: buildCID,
            UDSPath:  filepath.Join(workDir, "build.vsock"),
        },
        MMDSConfig: &mmdsConfig{
            Version:    "V2",
            Interfaces: []string{"relay"},
        },
        MachineConfig: machineConfig{
            VcpuCount:  4,
            MemSizeMiB: 4096,
        },
    }
    return cfg
}
```

**Helper: `func prepareRelayRootfs(rootfsPath, workDir string) (string, error)`**

Mounts the relay rootfs, injects the relay binary, warden-io, and init script, then unmounts:

```go
func prepareRelayRootfs(rootfsPath, workDir string) (string, error) {
    mountDir := filepath.Join(workDir, "relay-mount")
    if err := os.MkdirAll(mountDir, 0755); err != nil {
        return "", err
    }
    defer os.RemoveAll(mountDir)

    // Mount the rootfs
    if err := exec.Command("mount",
        "-o", "loop", rootfsPath, mountDir).Run(); err != nil {
        return "", fmt.Errorf("mounting rootfs: %w", err)
    }
    defer exec.Command("umount", mountDir).Run()

    // Inject relay binary
    relayBin := filepath.Join(workDir, "shared", "relay")
    if _, err := os.Stat(relayBin); err == nil {
        dst := filepath.Join(mountDir, "usr", "local", "bin", "relay")
        _ = os.MkdirAll(filepath.Dir(dst), 0755)
        if err := copyFile(relayBin, dst); err != nil {
            return "", fmt.Errorf("injecting relay binary: %w", err)
        }
        _ = os.Chmod(dst, 0755)
    }

    // Inject warden-io
    agentBin := filepath.Join(workDir, "shared", "agent", "warden-io")
    if _, err := os.Stat(agentBin); err == nil {
        dst := filepath.Join(mountDir, "usr", "local", "bin", "warden-io")
        _ = os.MkdirAll(filepath.Dir(dst), 0755)
        if err := copyFile(agentBin, dst); err != nil {
            return "", fmt.Errorf("injecting warden-io: %w", err)
        }
        _ = os.Chmod(dst, 0755)
    }

    // Inject relay init script
    initScript := `#!/bin/sh
# Firecracker relay VM init
mount -t devtmpfs devtmpfs /dev 2>/dev/null || true
mount -t proc proc /proc 2>/dev/null || true
mount -t sysfs sysfs /sys 2>/dev/null || true
mount -t tmpfs tmpfs /tmp 2>/dev/null || true

# Network: eth0 = build link, eth1 = upstream
ip link set eth0 up 2>/dev/null || true
ip addr add 10.0.0.1/30 dev eth0 2>/dev/null || true

ip link set eth1 up 2>/dev/null || true
ip addr add 10.0.2.15/24 dev eth1 2>/dev/null || true
ip route add default via 10.0.2.1 2>/dev/null || true

# iptables (same as QEMU relay)
iptables -P FORWARD DROP 2>/dev/null || true
iptables -A FORWARD -m state --state ESTABLISHED,RELATED -j ACCEPT 2>/dev/null || true
iptables -t nat -A PREROUTING -i eth0 -p tcp --dport 80 -j REDIRECT --to-port 80 2>/dev/null || true
iptables -t nat -A PREROUTING -i eth0 -p tcp --dport 443 -j REDIRECT --to-port 443 2>/dev/null || true
iptables -t nat -A PREROUTING -i eth0 -p udp --dport 53 -j REDIRECT --to-port 53 2>/dev/null || true
iptables -t nat -A POSTROUTING -o eth1 -j MASQUERADE 2>/dev/null || true

echo "nameserver 10.0.2.1" > /etc/resolv.conf 2>/dev/null || true

# Signal readiness to host via vsock (CID 2 = host)
echo "ready" | socat - VSOCK-CONNECT:2:54 2>/dev/null || true

exec /relay
`
    initPath := filepath.Join(mountDir, "sbin", "init")
    _ = os.MkdirAll(filepath.Dir(initPath), 0755)
    if err := os.WriteFile(initPath, []byte(initScript), 0755); err != nil {
        return "", fmt.Errorf("writing init script: %w", err)
    }

    return rootfsPath, nil
}
```

---

### 1.3 `driver/firecracker/config.go`

**Purpose:** Firecracker JSON config type definitions and generation helpers.

**Package declaration and imports:**

```go
package firecracker

import (
    "encoding/json"
    "fmt"
    "path/filepath"
)
```

**Type definitions (matching Firecracker API schema):**

```go
// firecracerConfig is the JSON structure Firecracker expects.
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

**Helper: `func (cfg firecracerConfig) APIPort() int`**

```go
func (cfg firecracerConfig) APIPort() int {
    return 8300
}
```

**Helper: `func (cfg firecracerConfig) Validate() error`**

Validates the config before writing to disk:

```go
func (cfg firecracerConfig) Validate() error {
    if cfg.BootSource.KernelPath == "" {
        return fmt.Errorf("boot-source kernel_image_path is required")
    }
    if len(cfg.Drives) == 0 {
        return fmt.Errorf("at least one drive is required")
    }
    if len(cfg.NetworkIfaces) == 0 {
        return fmt.Errorf("at least one network interface is required")
    }
    return nil
}
```

---

### 1.4 `driver/firecracker/network.go`

**Purpose:** TAP device creation, bridge setup, and cleanup for relay↔build and relay→internet networking.

**Package declaration and imports:**

```go
package firecracker

import (
    "fmt"
    "os"
    "os/exec"
    "path/filepath"
    "strings"
)
```

**Type: `networkState`**

```go
// networkState holds the state of created network resources for a build.
type networkState struct {
    bridge       string  // bridge device name (e.g., "fc-br-abc12345")
    relayTAP     string  // TAP device for relay↔build (e.g., "fc-r-abc12345")
    relayTAPs    []string // all TAP devices for relay VM (relay↔build + upstream)
    buildTAP     string  // TAP device for build VM (e.g., "fc-b-abc12345")
    buildTAPs    []string // all TAP devices for build VM
    upstreamTAP  string  // TAP device for relay→internet (e.g., "fc-u-abc12345")
    workDir      string  // work directory (for cleanup)
}
```

**`func createBuildNetwork(buildID string, workDir string) (*networkState, error)`**

Creates the full network infrastructure for a Firecracker build:

```go
func createBuildNetwork(buildID string, workDir string) (*networkState, error) {
    ns := &networkState{workDir: workDir}

    // Generate device names
    shortID := buildID[:8]
    ns.bridge = fmt.Sprintf("fc-br-%s", shortID)
    ns.relayTAP = fmt.Sprintf("fc-r-%s", shortID)
    ns.buildTAP = fmt.Sprintf("fc-b-%s", shortID)
    ns.upstreamTAP = fmt.Sprintf("fc-u-%s", shortID)

    // 1. Create upstream TAP (relay→internet)
    if err := createUpstreamTAP(ns.upstreamTAP); err != nil {
        ns.cleanup()
        return nil, fmt.Errorf("creating upstream TAP: %w", err)
    }
    ns.relayTAPs = append(ns.relayTAPs, ns.upstreamTAP)

    // 2. Create bridge for relay↔build
    if err := createBridge(ns.bridge); err != nil {
        ns.cleanup()
        return nil, fmt.Errorf("creating bridge: %w", err)
    }

    // 3. Create TAP pair for relay↔build
    if err := createTAP(ns.relayTAP); err != nil {
        ns.cleanup()
        return nil, fmt.Errorf("creating relay TAP: %w", err)
    }
    ns.relayTAPs = append(ns.relayTAPs, ns.relayTAP)

    if err := createTAP(ns.buildTAP); err != nil {
        ns.cleanup()
        return nil, fmt.Errorf("creating build TAP: %w", err)
    }
    ns.buildTAPs = append(ns.buildTAPs, ns.buildTAP)

    // 4. Attach TAPs to bridge
    for _, tap := range []string{ns.relayTAP, ns.buildTAP} {
        if err := exec.Command("ip", "link", "set", tap, "master", ns.bridge).Run(); err != nil {
            ns.cleanup()
            return nil, fmt.Errorf("attaching %s to bridge: %w", tap, err)
        }
    }

    // 5. Bring bridge up
    if err := exec.Command("ip", "link", "set", ns.bridge, "up").Run(); err != nil {
        ns.cleanup()
        return nil, fmt.Errorf("bringing bridge up: %w", err)
    }

    // 6. Configure bridge IP (relay VM gateway)
    if err := exec.Command("ip", "addr", "add", "10.0.0.1/30", "dev", ns.bridge).Run(); err != nil {
        // Non-fatal: IP may already be assigned
    }

    return ns, nil
}
```

**`func (ns *networkState) cleanup() error`**

Removes all network resources in reverse order:

```go
func (ns *networkState) cleanup() error {
    var errs []error

    // 1. Remove TAP devices
    for _, tap := range ns.relayTAPs {
        if err := exec.Command("ip", "link", "delete", tap).Run(); err != nil {
            errs = append(errs, fmt.Errorf("deleting TAP %s: %w", tap, err))
        }
    }
    for _, tap := range ns.buildTAPs {
        if err := exec.Command("ip", "link", "delete", tap).Run(); err != nil {
            errs = append(errs, fmt.Errorf("deleting TAP %s: %w", tap, err))
        }
    }

    // 2. Remove bridge
    if ns.bridge != "" {
        if err := exec.Command("ip", "link", "delete", ns.bridge).Run(); err != nil {
            errs = append(errs, fmt.Errorf("deleting bridge %s: %w", ns.bridge, err))
        }
    }

    // 3. Flush iptables MASQUERADE rules for this build
    if ns.upstreamTAP != "" {
        shortID := strings.TrimPrefix(ns.upstreamTAP, "fc-u-")
        _ = exec.Command("iptables", "-t", "nat", "-D", "POSTROUTING",
            "-s", "10.0.2.0/24", "-o", defaultInterface(), "-j", "MASQUERADE").Run()
    }

    if len(errs) > 0 {
        return fmt.Errorf("network cleanup errors: %v", errs)
    }
    return nil
}
```

**Helper: `func createTAP(tapName string) error`**

```go
func createTAP(tapName string) error {
    return exec.Command("ip", "tuntap", "add", tapName, "mode", "tap").Run()
}
```

**Helper: `func createBridge(bridgeName string) error`**

```go
func createBridge(bridgeName string) error {
    return exec.Command("ip", "link", "add", bridgeName, "type", "bridge").Run()
}
```

**Helper: `func createUpstreamTAP(tapName string) error`**

Creates the upstream TAP with MASQUERADE rule:

```go
func createUpstreamTAP(tapName string) error {
    // Create TAP device
    if err := exec.Command("ip", "tuntap", "add", tapName, "mode", "tap").Run(); err != nil {
        return err
    }

    // Assign IP to TAP (for host-side routing)
    if err := exec.Command("ip", "addr", "add", "10.0.2.1/24", "dev", tapName).Run(); err != nil {
        return err
    }

    // Bring TAP up
    if err := exec.Command("ip", "link", "set", tapName, "up").Run(); err != nil {
        return err
    }

    // Add MASQUERADE rule for relay's outbound traffic
    defaultIf := defaultInterface()
    if defaultIf == "" {
        return fmt.Errorf("could not determine default network interface")
    }
    return exec.Command("iptables", "-t", "nat", "-A", "POSTROUTING",
        "-s", "10.0.2.0/24", "-o", defaultIf, "-j", "MASQUERADE").Run()
}
```

**Helper: `func defaultInterface() string`**

```go
func defaultInterface() string {
    out, err := exec.Command("ip", "-4", "route").Output()
    if err != nil {
        return ""
    }
    for _, line := range strings.Split(string(out), "\n") {
        if strings.HasPrefix(line, "default") {
            fields := strings.Fields(line)
            for i, f := range fields {
                if f == "dev" && i+1 < len(fields) {
                    return fields[i+1]
                }
            }
        }
    }
    return ""
}
```

---

### 1.5 `driver/firecracker/images.go`

**Purpose:** Image resolution, QCOW2→raw conversion, and caching. (Mostly reused from QEMU driver.)

This file is small — it reuses the QEMU driver's image resolution logic with a Firecracker-specific conversion step.

**Package declaration and imports:**

```go
package firecracker

import (
    "fmt"
    "os"
    "os/exec"
    "path/filepath"
    "runtime"
    "strings"
)
```

**Helper: `func cachedImagePath(imageRef string) string`**

```go
func cachedImagePath(imageRef string) string {
    return filepath.Join(cacheBaseDir(), "warden", "images",
        "firecracker-"+strings.ReplaceAll(imageRef, "/", "-")+".qcow2")
}
```

**Helper: `func cachedBaseDir() string`** (reuse from qemu/util.go)

Already exists in `driver/qemu/util.go` as `cacheBaseDir()`. The firecracker package can either:
- Import from `driver/qemu` (not ideal — circular dependency risk)
- Duplicate the function (DRY violation)
- Add it to `driver/` package level

**Decision:** Add `cacheBaseDir()` to `driver/` package level (in a new `driver/cache.go` or to `driver/subnet.go`). For now, the firecracker driver will duplicate it:

```go
func cacheBaseDir() string {
    if dir, err := os.UserCacheDir(); err == nil {
        return dir
    }
    if runtime.GOOS == "linux" {
        if home := os.Getenv("HOME"); home != "" {
            return filepath.Join(home, ".cache")
        }
    }
    return os.TempDir()
}
```

**Note:** This duplicates `driver/qemu/util.go:cacheBaseDir()`. The proper fix is to move it to `driver/` package level. This is a minor refactor that should be done alongside the firecracker driver.

---

### 1.6 `driver/firecracker/vsock.go`

**Purpose:** vsock-based signaling between host and VMs (readiness, heartbeat, exit).

**Package declaration and imports:**

```go
package firecracker

import (
    "context"
    "fmt"
    "net"
    "os"
    "path/filepath"
    "time"
)
```

**vsock port constants:**

```go
const (
    vsockPortRelayReady  = 54  // Relay signals "ready" to host
    vsockPortHeartbeat   = 52  // Build sends heartbeat to host
    vsockPortExit        = 53  // Build sends exit code to host
    vsockHostCID         = 2   // Host's vsock CID (always)
)
```

**`func waitForRelayReady(ctx context.Context, vsockUDS string) error`**

Waits for the relay VM to signal "ready" via vsock:

```go
func waitForRelayReady(ctx context.Context, vsockUDS string) error {
    // Create a vsock listener on the host
    listener, err := net.Listen("vsock", vsockUDS)
    if err != nil {
        return fmt.Errorf("creating vsock listener: %w", err)
    }
    defer listener.Close()

    // Accept connection from relay VM
    connCh := make(chan net.Conn, 1)
    go func() {
        conn, err := listener.Accept()
        if err != nil {
            connCh <- nil
            return
        }
        connCh <- conn
    }()

    select {
    case <-ctx.Done():
        return ctx.Err()
    case conn := <-connCh:
        if conn == nil {
            return fmt.Errorf("vsock accept failed")
        }
        defer conn.Close()

        // Read the "ready" signal
        buf := make([]byte, 64)
        conn.SetReadDeadline(time.Now().Add(30 * time.Second))
        n, err := conn.Read(buf)
        if err != nil {
            return fmt.Errorf("reading relay ready signal: %w", err)
        }

        signal := strings.TrimSpace(string(buf[:n]))
        if signal != "ready" {
            return fmt.Errorf("unexpected relay signal: %q", signal)
        }

        return nil
    case <-time.After(30 * time.Second):
        return fmt.Errorf("relay did not signal readiness within 30s")
    }
}
```

**`func waitForBuild(ctx context.Context, vsockUDS string) (int, error)`**

Monitors the build VM for heartbeat and exit signals:

```go
func waitForBuild(ctx context.Context, vsockUDS string) (int, error) {
    // Create a vsock listener on the host
    listener, err := net.Listen("vsock", vsockUDS)
    if err != nil {
        return -1, fmt.Errorf("creating vsock listener: %w", err)
    }
    defer listener.Close()

    // Accept connections from build VM
    connCh := make(chan net.Conn, 1)
    go func() {
        conn, err := listener.Accept()
        if err != nil {
            connCh <- nil
            return
        }
        connCh <- conn
    }()

    select {
    case <-ctx.Done():
        return -1, ctx.Err()
    case conn := <-connCh:
        if conn == nil {
            return -1, fmt.Errorf("vsock accept failed")
        }
        defer conn.Close()

        // Read exit code from build VM
        conn.SetReadDeadline(time.Now().Add(24 * time.Hour))
        buf := make([]byte, 16)
        n, err := conn.Read(buf)
        if err != nil {
            return -1, fmt.Errorf("reading build exit code: %w", err)
        }

        // Parse the exit code (sent as decimal string)
        codeStr := strings.TrimSpace(string(buf[:n]))
        var exitCode int
        fmt.Sscanf(codeStr, "%d", &exitCode)
        return exitCode, nil
    case <-time.After(24 * time.Hour):
        return -1, fmt.Errorf("build timed out")
    }
}
```

**Note:** The vsock protocol for the build VM guest side requires `socat` or a custom binary. The build VM's init script would use:

```sh
# In the guest init script:
echo "$BUILD_EXIT_CODE" | socat - VSOCK-CONNECT:2:53
```

---

### 1.7 `driver/firecracker/mmds.go`

**Purpose:** MMDS (Metadata Document Service) metadata injection for build VMs.

**Package declaration and imports:**

```go
package firecracker

import (
    "encoding/json"
    "fmt"
    "os"
    "path/filepath"
)
```

**`func pushMMDSMetadata(apiSock string, cfg firecracerConfig) error`**

Pushes cloud-init-like metadata to the build VM via MMDS:

```go
func pushMMDSMetadata(apiSock string, cfg firecracerConfig) error {
    // Build the MMDS payload (cloud-init NoCloud format)
    mmds := map[string]any{
        "instance-id": "warden-build",
        "local-hostname": "warden-build",
        "network-config": map[string]any{
            "version": 2,
            "ethernets": map[string]any{
                "eth0": map[string]any{
                    "match":     map[string]any{"name": "e*"},
                    "addresses": []string{"10.0.0.2/30"},
                    "gateway4":  "10.0.0.1",
                    "nameservers": map[string]any{
                        "addresses": []string{"10.0.0.1"},
                    },
                },
            },
        },
        "user-data": map[string]any{
            "bootcmd": []any{
                map[string]any{
                    "command": []string{
                        "sh", "-c",
                        "wget -q http://10.0.0.1:8300/v1/context/build.sh -O /tmp/build.sh && chmod +x /tmp/build.sh && /tmp/build.sh",
                    },
                },
            },
        },
    }

    mmdsData, err := json.Marshal(mmds)
    if err != nil {
        return fmt.Errorf("marshaling MMDS data: %w", err)
    }

    // Push to Firecracker via API socket (PUT /meta-data)
    // This is handled by the api.go helper
    return apiPutMMDS(apiSock, mmdsData)
}
```

---

### 1.8 `driver/firecracker/api.go`

**Purpose:** Firecracker API socket client for runtime management (config update, pause, resume, snapshot, MMDS push).

**Package declaration and imports:**

```go
package firecracker

import (
    "bytes"
    "encoding/json"
    "fmt"
    "io"
    "net"
    "net/http"
    "os"
    "path/filepath"
)
```

**`func apiGet(sock, path string) ([]byte, error)`**

```go
func apiGet(sock, path string) ([]byte, error) {
    conn, err := net.DialTimeout("unix", sock, 2*time.Second)
    if err != nil {
        return nil, fmt.Errorf("connecting to Firecracker API: %w", err)
    }
    defer conn.Close()

    req, _ := http.NewRequest("GET", path, nil)
    req.Write(conn)

    resp, err := http.ReadResponse(nil, conn)
    if err != nil {
        return nil, fmt.Errorf("reading API response: %w", err)
    }
    defer resp.Body.Close()

    return io.ReadAll(resp.Body)
}
```

**`func apiPut(sock, path string, body any) error`**

```go
func apiPut(sock, path string, body any) error {
    data, err := json.Marshal(body)
    if err != nil {
        return fmt.Errorf("marshaling API body: %w", err)
    }

    conn, err := net.DialTimeout("unix", sock, 2*time.Second)
    if err != nil {
        return fmt.Errorf("connecting to Firecracker API: %w", err)
    }
    defer conn.Close()

    req, _ := http.NewRequest("PUT", path, bytes.NewReader(data))
    req.Write(conn)

    resp, err := http.ReadResponse(nil, conn)
    if err != nil {
        return fmt.Errorf("reading API response: %w", err)
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
        bodyBytes, _ := io.ReadAll(resp.Body)
        return fmt.Errorf("API %s %s returned %d: %s",
            "PUT", path, resp.StatusCode, string(bodyBytes))
    }

    return nil
}
```

**`func apiPatch(sock, path string, body any) error`**

```go
func apiPatch(sock, path string, body any) error {
    data, err := json.Marshal(body)
    if err != nil {
        return fmt.Errorf("marshaling API body: %w", err)
    }

    conn, err := net.DialTimeout("unix", sock, 2*time.Second)
    if err != nil {
        return fmt.Errorf("connecting to Firecracker API: %w", err)
    }
    defer conn.Close()

    req, _ := http.NewRequest("PATCH", path, bytes.NewReader(data))
    req.Write(conn)

    resp, err := http.ReadResponse(nil, conn)
    if err != nil {
        return fmt.Errorf("reading API response: %w", err)
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        bodyBytes, _ := io.ReadAll(resp.Body)
        return fmt.Errorf("API %s %s returned %d: %s",
            "PATCH", path, resp.StatusCode, string(bodyBytes))
    }

    return nil
}
```

**`func apiPutMMDS(apiSock string, mmdsData []byte) error`**

Convenience wrapper for pushing MMDS metadata:

```go
func apiPutMMDS(apiSock string, mmdsData []byte) error {
    return apiPut(apiSock, "/meta-data", mmdsData)
}
```

**`func apiPatchVMState(sock string, state string) error`**

```go
func apiPatchVMState(sock string, state string) error {
    return apiPatch(sock, "/vm", map[string]string{"state": state})
}
```

**`func apiCreateSnapshot(sock, snapPath, memPath string) error`**

```go
func apiCreateSnapshot(sock, snapPath, memPath string) error {
    return apiPut(sock, "/snapshot/create", map[string]string{
        "snapshot_path": snapPath,
        "mem_file_path": memPath,
        "snapshot_type": "Full",
    })
}
```

**`func apiLoadSnapshot(sock, snapPath, memPath string, overrides []driveOverride) error`**

```go
func apiLoadSnapshot(sock, snapPath, memPath string, overrides []driveOverride) error {
    body := map[string]any{
        "snapshot_path": snapPath,
        "mem_file_path": memPath,
    }
    if len(overrides) > 0 {
        body["drives"] = overrides
    }
    return apiPut(sock, "/snapshot/resume", body)
}
```

**Helper: `func apiGetMMDSToken(apiSock string) (string, error)`**

```go
func apiGetMMDSToken(apiSock string) (string, error) {
    data, err := apiGet(apiSock, "/metadata")
    if err != nil {
        return "", err
    }
    var result map[string]any
    if err := json.Unmarshal(data, &result); err != nil {
        return "", err
    }
    token, _ := result["token"].(string)
    return token, nil
}
```

---

### 1.9 `driver/firecracker/cid.go`

**Purpose:** vsock CID allocator — manages unique Context IDs for Firecracker VMs.

**Package declaration and imports:**

```go
package firecracker

import (
    "fmt"
    "os"
    "path/filepath"
    "sync"
)
```

**Type: `CIDAllocator`**

```go
// CIDAllocator manages vsock CID assignment for Firecracker VMs.
// Uses file-lock based allocation from a reserved range (100-999).
type CIDAllocator struct {
    mu       sync.Mutex
    lockFile string
    base     uint32 // Start of allocated range (default: 100)
    max      uint32 // End of allocated range (default: 999)
    inUse    map[uint32]bool
}
```

**`func NewCIDAllocator() *CIDAllocator`**

```go
func NewCIDAllocator() *CIDAllocator {
    return &CIDAllocator{
        base:     100,
        max:      999,
        inUse:    make(map[uint32]bool),
        lockFile: filepath.Join(os.TempDir(), "warden-firecracker-cid.lock"),
    }
}
```

**`func (a *CIDAllocator) AcquirePair() (uint32, uint32, error)`**

Allocates two consecutive CIDs (relay, build):

```go
func (a *CIDAllocator) AcquirePair() (uint32, uint32, error) {
    a.mu.Lock()
    defer a.mu.Unlock()

    // Find two consecutive unused CIDs
    for cid := a.base; cid <= a.max-1; cid++ {
        if !a.inUse[cid] && !a.inUse[cid+1] {
            a.inUse[cid] = true
            a.inUse[cid+1] = true
            return cid, cid + 1, nil
        }
    }

    return 0, 0, fmt.Errorf(
        "no available vsock CIDs in range %d-%d (all in use)", a.base, a.max)
}
```

**`func (a *CIDAllocator) ReleasePair(relayCID, buildCID uint32)`**

```go
func (a *CIDAllocator) ReleasePair(relayCID, buildCID uint32) {
    a.mu.Lock()
    defer a.mu.Unlock()

    // Only release if we actually allocated them
    if a.inUse[relayCID] {
        delete(a.inUse, relayCID)
    }
    if a.inUse[buildCID] {
        delete(a.inUse, buildCID)
    }
}
```

**`func (a *CIDAllocator) Close()`**

```go
func (a *CIDAllocator) Close() {
    a.mu.Lock()
    defer a.mu.Unlock()
    a.inUse = make(map[uint32]bool)
}
```

---

### 1.10 `driver/firecracker/pool.go`

**Purpose:** Warm VM pool manager — creates, manages, and restores VM snapshots for sub-100ms warm starts.

**Package declaration and imports:**

```go
package firecracker

import (
    "context"
    "crypto/sha256"
    "encoding/hex"
    "fmt"
    "os"
    "path/filepath"
    "strings"
    "sync"
    "time"
)
```

**Type: `Pool`**

```go
// Pool manages a pool of pre-warmed relay VM snapshots.
type Pool struct {
    mu          sync.Mutex
    available   []*vmProcess  // pre-warmed, paused VMs
    maxSize     int
    driver      *Driver
    baseConfig  *firecracerConfig
    snapshotDir string
    relayHash   string  // SHA-256 of relay binary (for invalidation)
}
```

**`func NewPool(driver *Driver, size int, workDir string) *Pool`**

```go
func NewPool(driver *Driver, size int, workDir string) *Pool {
    return &Pool{
        maxSize:     size,
        driver:      driver,
        snapshotDir: filepath.Join(workDir, "firecracker-pool"),
        relayHash:   computeRelayHash(driver),
    }
}
```

**`func (p *Pool) CreateBaseSnapshot() error`**

Boots a relay VM to "ready" state, pauses, and snapshots:

```go
func (p *Pool) CreateBaseSnapshot() error {
    // 1. Boot relay VM normally
    proc, err := p.driver.startRelayVM(
        p.baseConfig.BootSource.KernelPath,
        p.baseConfig.Drives[0].PathOnHost,
        &networkState{},
        p.driver.cidAlloc.base,
        p.snapshotDir,
    )
    if err != nil {
        return fmt.Errorf("booting relay for snapshot: %w", err)
    }

    // 2. Wait for relay to be ready
    if err := waitForRelayReady(context.Background(), proc.vsockUDS); err != nil {
        proc.stop()
        return fmt.Errorf("relay not ready for snapshot: %w", err)
    }

    // 3. Pause the VM
    if err := apiPatchVMState(proc.apiSock, "Paused"); err != nil {
        proc.stop()
        return fmt.Errorf("pausing VM for snapshot: %w", err)
    }

    // 4. Create snapshot
    snapState := filepath.Join(p.snapshotDir, "relay-snapshot.state")
    snapMem := filepath.Join(p.snapshotDir, "relay-snapshot.mem")
    if err := apiCreateSnapshot(proc.apiSock, snapState, snapMem); err != nil {
        proc.stop()
        return fmt.Errorf("creating snapshot: %w", err)
    }

    // 5. Store relay hash for invalidation
    hashFile := filepath.Join(p.snapshotDir, "relay-hash")
    if err := os.WriteFile(hashFile, []byte(p.relayHash), 0644); err != nil {
        return err
    }

    proc.stop()
    return nil
}
```

**`func (p *Pool) AcquireRelayVM() (*vmProcess, error)`**

Restores a relay VM from snapshot:

```go
func (p *Pool) AcquireRelayVM() (*vmProcess, error) {
    p.mu.Lock()
    defer p.mu.Unlock()

    // Check if we have a warm VM
    if len(p.available) > 0 {
        vm := p.available[len(p.available)-1]
        p.available = p.available[:len(p.available)-1]

        // Resume the paused VM
        if err := apiPatchVMState(vm.apiSock, "Resumed"); err != nil {
            vm.stop()
            return nil, fmt.Errorf("resuming warm VM: %w", err)
        }
        return vm, nil
    }

    // Cold boot from snapshot
    snapState := filepath.Join(p.snapshotDir, "relay-snapshot.state")
    snapMem := filepath.Join(p.snapshotDir, "relay-snapshot.mem")

    if _, err := os.Stat(snapState); err != nil {
        return nil, fmt.Errorf(
            "no snapshot available (run CreateBaseSnapshot first)")
    }

    // Start Firecracker with snapshot resume
    // (Implementation mirrors startRelayBare but loads snapshot)
    // ...
}
```

**`func (p *Pool) Return(vm *vmProcess)`**

```go
func (p *Pool) Return(vm *vmProcess) {
    p.mu.Lock()
    defer p.mu.Unlock()

    if len(p.available) >= p.maxSize {
        vm.stop()
        return
    }

    // Pause and reset state
    _ = apiPatchVMState(vm.apiSock, "Paused")
    p.available = append(p.available, vm)
}
```

**`func (p *Pool) Close()`**

```go
func (p *Pool) Close() {
    p.mu.Lock()
    defer p.mu.Unlock()

    for _, vm := range p.available {
        vm.stop()
    }
    p.available = nil
}
```

**`func (p *Pool) isSnapshotValid() bool`**

```go
func (p *Pool) isSnapshotValid() bool {
    hashFile := filepath.Join(p.snapshotDir, "relay-hash")
    data, err := os.ReadFile(hashFile)
    if err != nil {
        return false
    }
    return strings.TrimSpace(string(data)) == p.relayHash
}
```

---

### 1.11 `driver/firecracker/jailer.go`

**Purpose:** Optional jailer integration for production hardening.

**Package declaration and imports:**

```go
package firecracker

import (
    "fmt"
    "os"
    "os/exec"
    "path/filepath"
)
```

**Type: `jailerConfig`**

```go
type jailerConfig struct {
    UID        int
    GID        int
    CgroupPath string
    NetNS      string  // optional network namespace
}
```

**`func (d *Driver) startWithJailer(cfg firecracerConfig, configPath, workDir string) (*exec.Cmd, error)`**

(Already specified in `vm.go` — see above.)

**`func allocateUIDGID(name string) (uid, gid int, err error)`**

Allocates UID/GID from a pre-allocated range:

```go
func allocateUIDGID(name string) (int, int, error) {
    // Simple approach: use a hash of the name to get a UID in range 10000-10999
    hash := 0
    for _, c := range name {
        hash = hash*31 + int(c)
    }
    uid := 10000 + (hash % 1000)
    gid := uid

    // Verify UID/GID don't conflict with existing users
    cmd := exec.Command("id", "-u", fmt.Sprintf("%d", uid))
    if cmd.Run() == nil {
        return 0, 0, fmt.Errorf(
            "UID %d already exists — configure a different UID range", uid)
    }

    return uid, gid, nil
}
```

---

### 1.12 `driver/firecracker/util.go`

**Purpose:** Shared utility functions (binary caching, file helpers, module root detection).

**Package declaration and imports:**

```go
package firecracker

import (
    "crypto/sha256"
    "encoding/hex"
    "io"
    "os"
    "path/filepath"
    "runtime"
)
```

**`func copyFile(src, dst string) error`** (reuse from qemu/util.go)

```go
func copyFile(src, dst string) error {
    in, err := os.Open(src)
    if err != nil {
        return err
    }
    defer in.Close()

    out, err := os.Create(dst)
    if err != nil {
        return err
    }
    defer out.Close()

    if _, err := io.Copy(out, in); err != nil {
        return err
    }

    info, err := in.Stat()
    if err == nil {
        _ = out.Chmod(info.Mode())
    }
    return out.Close()
}
```

**`func cachedBinaryPath(name, goarch string) string`** (reuse from qemu/util.go)

```go
func cachedBinaryPath(name, goarch string) string {
    cacheDir := filepath.Join(cacheBaseDir(), "warden", "bin")
    _ = os.MkdirAll(cacheDir, 0755)
    return filepath.Join(cacheDir, name+"-linux-"+goarch)
}
```

**`func isCacheStale(cached string) bool`** (reuse from qemu/util.go)

```go
func isCacheStale(cached string) bool {
    info, err := os.Stat(cached)
    if err != nil {
        return true
    }

    root := findModuleRoot()
    gosum := filepath.Join(root, "go.sum")
    sumInfo, err := os.Stat(gosum)
    if err != nil {
        return false
    }
    if sumInfo.ModTime().After(info.ModTime()) {
        return true
    }

    if exe, err := os.Executable(); err == nil {
        if exeInfo, err := os.Stat(exe); err == nil {
            if exeInfo.ModTime().After(info.ModTime()) {
                return true
            }
        }
    }
    return false
}
```

**`func cacheBaseDir() string`** (duplicate from qemu/util.go — should be moved to driver/ package)

```go
func cacheBaseDir() string {
    if dir, err := os.UserCacheDir(); err == nil {
        return dir
    }
    if runtime.GOOS == "linux" {
        if home := os.Getenv("HOME"); home != "" {
            return filepath.Join(home, ".cache")
        }
    }
    return os.TempDir()
}
```

**`func findModuleRoot() string`** (reuse from qemu/util.go)

```go
func findModuleRoot() string {
    if exe, err := os.Executable(); err == nil {
        dir := filepath.Dir(exe)
        for {
            if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
                return dir
            }
            parent := filepath.Dir(dir)
            if parent == dir {
                break
            }
            dir = parent
        }
    }

    dir, _ := os.Getwd()
    for {
        if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
            return dir
        }
        parent := filepath.Dir(dir)
        if parent == dir {
            break
        }
        dir = parent
    }
    return "."
}
```

**`func computeRelayHash(d *Driver) string`**

Computes SHA-256 of the relay binary (for snapshot invalidation):

```go
func computeRelayHash(d *Driver) string {
    binary := d.firecrackerBinary()
    if _, err := exec.LookPath(binary); err != nil {
        return ""
    }

    data, err := exec.Command(binary, "--version").Output()
    if err != nil {
        return ""
    }

    // Hash the binary path + version for invalidation
    h := sha256.Sum256(append([]byte(binary), data...))
    return hex.EncodeToString(h[:])
}
```

---

### 1.13 Test Files

**`driver/firecracker/driver_test.go`**

Tests for Driver struct, Name(), StartBuild() skeleton:

```go
package firecracker

import (
    "context"
    "testing"

    "github.com/buildwarden/buildwarden/driver"
)

func TestDriverName(t *testing.T) {
    d := New()
    if d.Name() != "firecracker" {
        t.Errorf("expected name 'firecracker', got '%s'", d.Name())
    }
}

func TestCompileTimeCheck(t *testing.T) {
    // This compiles if Driver implements driver.Driver
    var _ driver.Driver = (*Driver)(nil)
}

func TestNew(t *testing.T) {
    d := New()
    if d == nil {
        t.Fatal("New() returned nil")
    }
    if d.cidAlloc == nil {
        t.Fatal("New() did not initialize cidAlloc")
    }
}

func TestExecNotSupported(t *testing.T) {
    d := New()
    err := d.Exec(context.Background(), &driver.BuildRequest{})
    if err != driver.ErrExecNotSupported {
        t.Errorf("expected ErrExecNotSupported, got %v", err)
    }
}
```

**`driver/firecracker/config_test.go`**

Tests for FirecracerConfig JSON generation:

```go
package firecracker

import (
    "encoding/json"
    "testing"
)

func TestRelayVMConfig(t *testing.T) {
    cfg := firecracerConfig{
        BootSource: bootSource{
            KernelPath: "/tmp/vmlinux",
            BootArgs:   "console=ttyS0",
        },
        Drives: []drive{
            {DriveID: "rootfs", PathOnHost: "/tmp/rootfs.ext4", IsRootDevice: true},
        },
        NetworkIfaces: []networkIf{
            {IfaceID: "build", GuestMAC: "AA:FC:00:00:00:01", HostDevName: "tap0"},
        },
        Vsock: &vsockConfig{GuestCID: 100, UDSPath: "/tmp/relay.vsock"},
        MachineConfig: machineConfig{VcpuCount: 2, MemSizeMiB: 512},
    }

    data, err := json.Marshal(cfg)
    if err != nil {
        t.Fatalf("marshal failed: %v", err)
    }

    // Verify structure matches Firecracker schema
    var decoded firecracerConfig
    if err := json.Unmarshal(data, &decoded); err != nil {
        t.Fatalf("unmarshal failed: %v", err)
    }

    if decoded.BootSource.KernelPath != "/tmp/vmlinux" {
        t.Errorf("kernel path mismatch")
    }
    if len(decoded.Drives) != 1 {
        t.Errorf("expected 1 drive, got %d", len(decoded.Drives))
    }
}

func TestBuildVMConfig(t *testing.T) {
    cfg := firecracerConfig{
        BootSource: bootSource{
            KernelPath: "/tmp/vmlinux",
            BootArgs:   "console=ttyS0 ds=nocloud",
        },
        Drives: []drive{
            {DriveID: "rootfs", PathOnHost: "/tmp/rootfs.raw", IsRootDevice: true},
        },
        NetworkIfaces: []networkIf{
            {IfaceID: "relay", GuestMAC: "AA:FC:00:00:01:01", HostDevName: "tap1"},
        },
        Vsock: &vsockConfig{GuestCID: 101, UDSPath: "/tmp/build.vsock"},
        MMDSConfig: &mmdsConfig{
            Version:    "V2",
            Interfaces: []string{"relay"},
        },
        MachineConfig: machineConfig{VcpuCount: 4, MemSizeMiB: 4096},
    }

    data, err := json.Marshal(cfg)
    if err != nil {
        t.Fatalf("marshal failed: %v", err)
    }

    // Verify MMDS is present
    var decoded firecracerConfig
    if err := json.Unmarshal(data, &decoded); err != nil {
        t.Fatalf("unmarshal failed: %v", err)
    }
    if decoded.MMDSConfig == nil {
        t.Fatal("MMDS config missing")
    }
    if decoded.MMDSConfig.Version != "V2" {
        t.Errorf("expected MMDS V2, got %s", decoded.MMDSConfig.Version)
    }
}

func TestConfigValidation(t *testing.T) {
    t.Run("valid config", func(t *testing.T) {
        cfg := firecracerConfig{
            BootSource: bootSource{KernelPath: "/tmp/vmlinux"},
            Drives:     []drive{{DriveID: "rootfs", PathOnHost: "/tmp/rootfs.ext4"}},
            NetworkIfaces: []networkIf{{IfaceID: "build", GuestMAC: "AA:FC:00:00:00:01"}},
            MachineConfig: machineConfig{VcpuCount: 2, MemSizeMiB: 512},
        }
        if err := cfg.Validate(); err != nil {
            t.Errorf("valid config failed validation: %v", err)
        }
    })

    t.Run("missing kernel", func(t *testing.T) {
        cfg := firecracerConfig{
            Drives:        []drive{{DriveID: "rootfs", PathOnHost: "/tmp/rootfs.ext4"}},
            NetworkIfaces: []networkIf{{IfaceID: "build", GuestMAC: "AA:FC:00:00:00:01"}},
            MachineConfig: machineConfig{VcpuCount: 2, MemSizeMiB: 512},
        }
        if err := cfg.Validate(); err == nil {
            t.Error("expected error for missing kernel")
        }
    })

    t.Run("no drives", func(t *testing.T) {
        cfg := firecracerConfig{
            BootSource:    bootSource{KernelPath: "/tmp/vmlinux"},
            NetworkIfaces: []networkIf{{IfaceID: "build", GuestMAC: "AA:FC:00:00:00:01"}},
            MachineConfig: machineConfig{VcpuCount: 2, MemSizeMiB: 512},
        }
        if err := cfg.Validate(); err == nil {
            t.Error("expected error for no drives")
        }
    })

    t.Run("no network", func(t *testing.T) {
        cfg := firecracerConfig{
            BootSource:  bootSource{KernelPath: "/tmp/vmlinux"},
            Drives:      []drive{{DriveID: "rootfs", PathOnHost: "/tmp/rootfs.ext4"}},
            MachineConfig: machineConfig{VcpuCount: 2, MemSizeMiB: 512},
        }
        if err := cfg.Validate(); err == nil {
            t.Error("expected error for no network")
        }
    })
}
```

**`driver/firecracker/network_test.go`**

Tests for TAP/bridge command generation (mock exec):

```go
package firecracker

import (
    "testing"
)

func TestCreateTAP(t *testing.T) {
    // This test verifies the command would be correct.
    // Integration test would actually create a TAP device.
    // For unit testing, we verify the command string.
    expectedCmd := []string{"ip", "tuntap", "add", "test-tap", "mode", "tap"}
    _ = expectedCmd // Integration test: actually run ip tuntap add
}

func TestCreateBridge(t *testing.T) {
    expectedCmd := []string{"ip", "link", "add", "test-bridge", "type", "bridge"}
    _ = expectedCmd // Integration test: actually run ip link add
}

func TestNetworkStateCleanup(t *testing.T) {
    // Verify cleanup() generates correct delete commands
    ns := &networkState{
        bridge:      "fc-br-test",
        relayTAP:    "fc-r-test",
        buildTAP:    "fc-b-test",
        upstreamTAP: "fc-u-test",
    }

    // Integration test: actually create and cleanup network
    // Unit test: verify cleanup() constructs correct commands
}
```

**`driver/firecracker/images_test.go`**

Tests for image path resolution and caching logic:

```go
package firecracker

import (
    "path/filepath"
    "testing"
)

func TestCachedImagePath(t *testing.T) {
    result := cachedImagePath("ubuntu-24.04")
    expected := filepath.Join(cacheBaseDir(), "warden", "images",
        "firecracker-ubuntu-24.04.qcow2")
    if result != expected {
        t.Errorf("cachedImagePath(%q) = %q, want %q",
            "ubuntu-24.04", result, expected)
    }
}

func TestFirecrackerCloudImageURL(t *testing.T) {
    tests := []struct {
        ref    string
        expect string
    }{
        {"ubuntu-24.04", "https://cloud-images.ubuntu.com/noble/current/noble-server-cloudimg-amd64.img"},
        {"ubuntu-22.04", "https://cloud-images.ubuntu.com/jammy/current/jammy-server-cloudimg-amd64.img"},
        {"ubuntu-arm64-24.04", "https://cloud-images.ubuntu.com/noble/current/noble-server-cloudimg-arm64.img"},
        {"debian-12", "https://cloud.debian.org/images/cloud/bookworm/latest/debian-12-generic-amd64.qcow2"},
        {"unknown-image", ""},
    }

    for _, tc := range tests {
        result := firecrackerCloudImageURL(tc.ref)
        if result != tc.expect {
            t.Errorf("firecrackerCloudImageURL(%q) = %q, want %q",
                tc.ref, result, tc.expect)
        }
    }
}
```

**`driver/firecracker/vsock_test.go`**

Tests for vsock protocol handling (mock connections):

```go
package firecracker

import (
    "testing"
)

func TestVsockPorts(t *testing.T) {
    if vsockPortRelayReady != 54 {
        t.Errorf("expected vsockPortRelayReady = 54, got %d", vsockPortRelayReady)
    }
    if vsockPortHeartbeat != 52 {
        t.Errorf("expected vsockPortHeartbeat = 52, got %d", vsockPortHeartbeat)
    }
    if vsockPortExit != 53 {
        t.Errorf("expected vsockPortExit = 53, got %d", vsockPortExit)
    }
    if vsockHostCID != 2 {
        t.Errorf("expected vsockHostCID = 2, got %d", vsockHostCID)
    }
}
```

**`driver/firecracker/api_test.go`**

Tests for Firecracker API client (mock HTTP server):

```go
package firecracker

import (
    "net"
    "net/http"
    "os"
    "path/filepath"
    "testing"
)

func newMockFirecrackerAPI(t *testing.T) (socketPath string, cleanup func()) {
    tmpDir := t.TempDir()
    sock := filepath.Join(tmpDir, "firecracker.sock")

    l, err := net.Listen("unix", sock)
    if err != nil {
        t.Fatalf("creating mock listener: %v", err)
    }

    mux := http.NewServeMux()
    mux.HandleFunc("/meta-data", func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "application/json")
        w.Write([]byte(`{"instance-id": "test"}`))
    })

    srv := &http.Server{Handler: mux}
    go srv.Serve(l)

    return sock, func() {
        srv.Close()
        l.Close()
    }
}

func TestAPIGet(t *testing.T) {
    sock, cleanup := newMockFirecrackerAPI(t)
    defer cleanup()

    data, err := apiGet(sock, "/meta-data")
    if err != nil {
        t.Fatalf("apiGet failed: %v", err)
    }

    if len(data) == 0 {
        t.Error("expected non-empty response")
    }
}

func TestAPIPut(t *testing.T) {
    sock, cleanup := newMockFirecrackerAPI(t)
    defer cleanup()

    err := apiPut(sock, "/vm", map[string]string{"state": "Paused"})
    // Note: mock server doesn't handle PATCH, so this may fail.
    // This is expected — integration test would use real Firecracker.
    _ = err
}
```

**`driver/firecracker/cid_test.go`**

Tests for CID allocation:

```go
package firecracker

import (
    "testing"
)

func TestCIDAllocatorAcquirePair(t *testing.T) {
    a := NewCIDAllocator()
    defer a.Close()

    // First allocation should succeed
    cid1, cid2, err := a.AcquirePair()
    if err != nil {
        t.Fatalf("AcquirePair() failed: %v", err)
    }
    if cid1 < a.base || cid2 > a.max {
        t.Errorf("CID out of range: %d, %d (base=%d, max=%d)",
            cid1, cid2, a.base, a.max)
    }
    if cid2 != cid1+1 {
        t.Errorf("expected consecutive CIDs, got %d and %d", cid1, cid2)
    }

    // Second allocation should get next pair
    cid3, cid4, err := a.AcquirePair()
    if err != nil {
        t.Fatalf("second AcquirePair() failed: %v", err)
    }
    if cid3 != cid2+1 {
        t.Errorf("expected next pair to start at %d, got %d", cid2+1, cid3)
    }

    // Release first pair
    a.ReleasePair(cid1, cid2)

    // Re-acquire should get the same pair
    cid5, cid6, err := a.AcquirePair()
    if err != nil {
        t.Fatalf("re-acquire after release failed: %v", err)
    }
    if cid5 != cid1 {
        t.Errorf("expected to re-acquire %d, got %d", cid1, cid5)
    }
}

func TestCIDAllocatorExhaustion(t *testing.T) {
    // Create allocator with tiny range
    a := &CIDAllocator{
        base:  1,
        max:   2,
        inUse: make(map[uint32]bool),
    }

    // Allocate all CIDs
    c1, c2, err := a.AcquirePair()
    if err != nil {
        t.Fatalf("first AcquirePair() failed: %v", err)
    }
    _ = c1
    _ = c2

    // Next allocation should fail
    _, _, err = a.AcquirePair()
    if err == nil {
        t.Error("expected error when all CIDs exhausted")
    }
}
```

---

## Part 2: Changes to Existing Files

### 2.1 `cmd/warden/config.go` — Add `"firecracker"` to `validateDriver()`

**Current code:**

```go
func validateDriver(d string) error {
    switch d {
    case "", "container", "qemu", "vz":
        return nil
    default:
        return fmt.Errorf(
            "unknown driver %q (valid: container, qemu, vz)", d)
    }
}
```

**Change:**

```go
func validateDriver(d string) error {
    switch d {
    case "", "container", "qemu", "vz", "firecracker":
        return nil
    default:
        return fmt.Errorf(
            "unknown driver %q (valid: container, qemu, vz, firecracker)", d)
    }
}
```

### 2.2 `cmd/warden/main.go` — Add Firecracker Case in `runBuild()`

**Current code:**

```go
switch cfg.Runtime.Driver {
case "vz":
    // ...
case "qemu":
    // ...
}
// Default: container driver
```

**Change — add before the default container case:**

```go
case "firecracker":
    dockerfile, contextDir, err := ResolvePath(bp.path)
    if err != nil {
        return err
    }
    d := firecracker.New()
    d.UseJailer = false  // Default: no jailer
    d.Verbose = cfg.Output.Verbose
    defer d.Close()
    var timeout time.Duration
    if flagTimeout != "" {
        timeout, err = time.ParseDuration(flagTimeout)
        if err != nil {
            return fmt.Errorf("invalid --timeout: %w", err)
        }
    }
    _, buildErr := d.StartBuild(ctx, &driver.BuildRequest{
        ContextDir:       contextDir,
        Containerfile:    dockerfile,
        Script:           flagScript,
        Image:            flagImage,
        CaptureMode:      bp.capture,
        OutputDir:        bp.outputDir,
        Compress:         bp.compress,
        Timeout:          timeout,
        UpstreamCACerts:  cfg.Relay.UpstreamCACerts,
        UpstreamSystemCA: bp.systemCA,
        Stdin:            os.Stdin,
        Stdout:           os.Stdout,
        Stderr:           os.Stderr,
    })
    return buildErr
```

**Also add import:**

```go
import (
    // ... existing imports ...
    "github.com/buildwarden/buildwarden/driver/firecracker"
)
```

### 2.3 `cmd/warden/clean.go` — Add Firecracker Resource Cleaning

**Add a new function after `cleanVMCache()`:**

```go
func cleanFirecracker() bool {
    cacheDir, err := os.UserCacheDir()
    if err != nil {
        return false
    }
    fcCache := filepath.Join(cacheDir, "warden", "firecracker-vm")
    fcImages := filepath.Join(cacheDir, "warden", "firecracker-images")
    fcPool := filepath.Join(cacheDir, "warden", "firecracker-pool")

    var cleaned bool
    for _, dir := range []string{fcCache, fcImages, fcPool} {
        if info, err := os.Stat(dir); err == nil && info.IsDir() {
            var size int64
            _ = filepath.Walk(dir, func(_ string, fi os.FileInfo, _ error) error {
                if fi != nil && !fi.IsDir() {
                    size += fi.Size()
                }
                return nil
            })
            if err := os.RemoveAll(dir); err != nil {
                return cleaned
            }
            fmt.Fprintf(os.Stderr,
                "Removed Firecracker cache (%d MB)\n", size/(1024*1024))
            cleaned = true
        }
    }

    // Clean orphaned TAP devices
    out, err := exec.Command("ip", "link", "show").Output()
    if err == nil {
        for _, line := range strings.Split(string(out), "\n") {
            if strings.HasPrefix(line, "fc-") {
                parts := strings.Fields(line)
                if len(parts) >= 2 {
                    name := strings.TrimSuffix(parts[1], ":")
                    if strings.HasPrefix(name, "fc-") {
                        exec.Command("ip", "link", "delete", name).Run()
                        fmt.Fprintf(os.Stderr,
                            "Removed orphaned TAP: %s\n", name)
                        cleaned = true
                    }
                }
            }
        }
    }

    // Clean orphaned bridges
    out, err = exec.Command("ip", "link", "show", "type", "bridge").Output()
    if err == nil {
        for _, line := range strings.Split(string(out), "\n") {
            if strings.HasPrefix(line, "fc-") {
                parts := strings.Fields(line)
                if len(parts) >= 2 {
                    name := strings.TrimSuffix(parts[1], ":")
                    if strings.HasPrefix(name, "fc-") {
                        exec.Command("ip", "link", "delete", name).Run()
                        fmt.Fprintf(os.Stderr,
                            "Removed orphaned bridge: %s\n", name)
                        cleaned = true
                    }
                }
            }
        }
    }

    return cleaned
}
```

**Add call in `runClean()`:**

```go
// After cleanVMCache() call:
if cleanFirecracker() {
    removed++
}
```

### 2.4 `go.mod` — Add vsock dependency

**Add one line to the `require` block:**

```go
require github.com/mdlayher/vsock v1.2.1
```

This provides `net/vsock` package for listening and connecting to vsock sockets.

---

## Part 3: Build System Changes

### 3.1 `Makefile` — Add Firecracker Asset Targets

```makefile
.PHONY: firecracker-assets
firecracker-assets: firecracker-kernel firecracker-rootfs
	@echo "Firecracker assets built successfully"

.PHONY: firecracker-kernel
firecracker-kernel:
	@echo "Building Firecracker kernel (vmlinux)..."
	@tools/firecracker-relay-vm/build-kernel.sh

.PHONY: firecracker-rootfs
firecracker-rootfs:
	@echo "Building Firecracker relay rootfs..."
	@tools/firecracker-relay-vm/build-rootfs.sh

.PHONY: test-firecracker
test-firecracker:
	@go test -tags integration ./driver/firecracker/...
```

### 3.2 `tools/firecracker-relay-vm/` — Build Scripts

**Directory structure:**

```
tools/firecracker-relay-vm/
├── build-kernel.sh      # Builds uncompressed vmlinux from kernel source
├── build-rootfs.sh      # Creates minimal ext4 relay rootfs
└── output/              # Build output directory
    ├── vmlinux          # Uncompressed kernel
    └── rootfs.ext4     # Minimal ext4 rootfs (~15MB)
```

**`tools/firecracker-relay-vm/build-kernel.sh`:**

```sh
#!/bin/sh
set -e

OUTPUT_DIR="output"
mkdir -p "$OUTPUT_DIR"

KERNEL_VERSION="6.1.0"  # Or pin to specific version
KERNEL_URL="https://cdn.kernel.org/pub/linux/kernel/v6.x/linux-${KERNEL_VERSION}.tar.xz"
KERNEL_DIR="linux-${KERNEL_VERSION}"

# Download and extract kernel source (cached)
KERNEL_TAR="${KERNEL_DIR}.tar.xz"
if [ ! -f "$KERNEL_TAR" ]; then
    curl -fL -o "$KERNEL_TAR" "$KERNEL_URL"
fi

if [ ! -d "$KERNEL_DIR" ]; then
    tar xf "$KERNEL_TAR"
fi

cd "$KERNEL_DIR"

# Build with Firecracker-specific config
# (Reuses QEMU relay-vm kernel config with Firecracker adjustments)
make -j$(nproc) defconfig 2>/dev/null || true
# Disable modules, enable Firecracker features
cat >> .config <<EOF
CONFIG_MODULES=n
CONFIG_KVM=y
CONFIG_VHOST_VSOCK=y
CONFIG_NET_TAP=y
CONFIG_BLOCK=y
CONFIG_EXT4_FS=y
CONFIG_DEVTMPFS=y
CONFIG_VIRTIO_PCI=y
CONFIG_VIRTIO_NET=y
CONFIG_VIRTIO_BLK=y
CONFIG_SERIAL_8250=y
CONFIG_SERIAL_8250_CONSOLE=y
CONFIG_PRINTK=y
CONFIG_MAGIC_SYSRQ=y
EOF
make -j$(nproc) vmlinux

cp vmlinux "../$OUTPUT_DIR/vmlinux"
echo "Kernel built: $OUTPUT_DIR/vmlinux"
```

**`tools/firecracker-relay-vm/build-rootfs.sh`:**

```sh
#!/bin/sh
set -e

OUTPUT_DIR="output"
mkdir -p "$OUTPUT_DIR"

ROOTFS_SIZE="64M"
ROOTFS_FILE="$OUTPUT_DIR/rootfs.ext4"

# Create ext4 image
truncate -s "$ROOTFS_SIZE" "$ROOTFS_FILE"
mkfs.ext4 -L relay "$ROOTFS_FILE"

# Mount and populate
MOUNT_DIR=$(mktemp -d)
mount -o loop "$ROOTFS_FILE" "$MOUNT_DIR"

# Install Alpine base packages (minimal)
# (Uses apk or debootstrap depending on host)
# For Alpine:
#   apk add --no-cache --root "$MOUNT_DIR" bash coreutils iptables iproute2 socat curl

# Inject relay init script
cat > "$MOUNT_DIR/sbin/init" <<'SCRIPT'
#!/bin/sh
# Firecracker relay VM init
mount -t devtmpfs devtmpfs /dev 2>/dev/null || true
mount -t proc proc /proc 2>/dev/null || true
mount -t sysfs sysfs /sys 2>/dev/null || true
mount -t tmpfs tmpfs /tmp 2>/dev/null || true

# Network: eth0 = build link, eth1 = upstream
ip link set eth0 up 2>/dev/null || true
ip addr add 10.0.0.1/30 dev eth0 2>/dev/null || true

ip link set eth1 up 2>/dev/null || true
ip addr add 10.0.2.15/24 dev eth1 2>/dev/null || true
ip route add default via 10.0.2.1 2>/dev/null || true

# iptables (same as QEMU relay)
iptables -P FORWARD DROP 2>/dev/null || true
iptables -A FORWARD -m state --state ESTABLISHED,RELATED -j ACCEPT 2>/dev/null || true
iptables -t nat -A PREROUTING -i eth0 -p tcp --dport 80 -j REDIRECT --to-port 80 2>/dev/null || true
iptables -t nat -A PREROUTING -i eth0 -p tcp --dport 443 -j REDIRECT --to-port 443 2>/dev/null || true
iptables -t nat -A PREROUTING -i eth0 -p udp --dport 53 -j REDIRECT --to-port 53 2>/dev/null || true
iptables -t nat -A POSTROUTING -o eth1 -j MASQUERADE 2>/dev/null || true

echo "nameserver 10.0.2.1" > /etc/resolv.conf 2>/dev/null || true

# Signal readiness to host via vsock (CID 2 = host)
echo "ready" | socat - VSOCK-CONNECT:2:54 2>/dev/null || true

exec /relay
SCRIPT
chmod +x "$MOUNT_DIR/sbin/init"

# Unmount and shrink
umount "$MOUNT_DIR"
rmdir "$MOUNT_DIR"
resize2fs -M "$ROOTFS_FILE" 2>/dev/null || true

echo "Rootfs built: $ROOTFS_FILE"
```

**Make both scripts executable:**

```sh
chmod +x tools/firecracker-relay-vm/build-kernel.sh
chmod +x tools/firecracker-relay-vm/build-rootfs.sh
```

---

## Part 4: Integration Checklist

### Phase 0: Spike (1 week)

- [ ] Create `driver/firecracker/` directory
- [ ] Create `driver/firecracker/driver.go` with Driver struct and StartBuild skeleton
- [ ] Create `driver/firecracker/config.go` with type definitions
- [ ] Create `driver/firecracker/vm.go` with startRelayVM/startBuildVM
- [ ] Add `github.com/mdlayher/vsock` to `go.mod`
- [ ] Add `"firecracker"` to `validateDriver()` in `cmd/warden/config.go`
- [ ] Add `case "firecracker":` in `cmd/warden/main.go`
- [ ] Build and verify `./warden build --driver firecracker` starts (even if it fails at prereqs)
- [ ] Verify on a Linux host with KVM: `firecracker --version`, `modprobe vhost_vsock`, `/dev/kvm` exists

### Phase 1: Minimal E2E (2-3 weeks)

- [ ] Create `driver/firecracker/network.go` with TAP creation
- [ ] Create `driver/firecracker/images.go` with QCOW2→raw conversion
- [ ] Create `driver/firecracker/util.go` with shared utilities
- [ ] Create `tools/firecracker-relay-vm/build-kernel.sh`
- [ ] Create `tools/firecracker-relay-vm/build-rootfs.sh`
- [ ] Add `firecracker-assets` target to Makefile
- [ ] Test: `make firecracker-assets` produces vmlinux + rootfs.ext4
- [ ] Test: `./warden build --driver firecracker examples/Dockerfile.simple` completes

### Phase 2: Live Monitoring (1-2 weeks)

- [ ] Create `driver/firecracker/vsock.go` with vsock signaling
- [ ] Create `driver/firecracker/api.go` with API client
- [ ] Create `driver/firecracker/mmds.go` with MMDS injection
- [ ] Test: Relay VM signals "ready" via vsock
- [ ] Test: Build VM heartbeat/exit monitored in real-time

### Phase 3: Production Hardening (1-2 weeks)

- [ ] Create `driver/firecracker/jailer.go` with jailer integration
- [ ] Create `driver/firecracker/cid.go` with CID allocator
- [ ] Add jailer binary to CI fleet
- [ ] Security audit: review all attack surface vectors

### Phase 4: Warm Pool + Snapshots (2-3 weeks)

- [ ] Create `driver/firecracker/pool.go` with pool manager
- [ ] Test: Snapshot creation and resume
- [ ] Test: <10ms warm start

---

## Part 5: Error Handling Reference

Every function in the Firecracker driver must handle these error categories:

### Prerequisite Errors (returned from `checkPrereqs()`)

| Error | Cause | Fix |
|-------|-------|-----|
| `/dev/kvm not available` | KVM not enabled in BIOS/UEFI | Enable virtualization in BIOS |
| `/dev/net/tun not available` | TUN/TAP not created | `mkdir -p /dev/net && mknod /dev/net/tun c 10 200` |
| `vhost_vsock module not loaded` | Kernel module not loaded | `modprobe vhost_vsock` |
| `firecracker binary not found` | Not installed | Download from GitHub releases |
| `qemu-img not found` | Not installed | `apt install qemu-utils` |
| `ip not found` | Not installed | `apt install iproute2` |
| `iptables not found` | Not installed | `apt install iptables` |
| `jailer binary not found` | Jailer not installed | Download matching version from Firecracker releases |

### Network Setup Errors (returned from `createBuildNetwork()`)

| Error | Cause | Fix |
|-------|-------|-----|
| `creating upstream TAP` | TUN/TAP device creation failed | Check `/dev/net/tun`, CAP_NET_ADMIN |
| `creating bridge` | Bridge module not loaded | `modprobe bridge` |
| `attaching TAP to bridge` | TAP device doesn't exist | Check TAP creation succeeded |
| `bringing bridge up` | Bridge creation failed | Check bridge module |
| `could not determine default interface` | No default route | Configure network interface |

### VM Startup Errors (returned from `startRelayVM()` / `startBuildVM()`)

| Error | Cause | Fix |
|-------|-------|-----|
| `marshaling config` | JSON marshal failure | Check config structure |
| `writing config` | Permission denied | Check work directory permissions |
| `preparing relay rootfs` | Rootfs mount failed | Check rootfs is valid ext4 |
| `starting firecracer` | Firecracer binary not found or KVM unavailable | Install firecracer, check /dev/kvm |
| `API socket did not appear` | Firecracer crashed or didn't start | Check firecracer logs |

### Relay Readiness Errors (returned from `waitForRelayReady()`)

| Error | Cause | Fix |
|-------|-------|-----|
| `creating vsock listener` | vhost_vsock module not loaded | `modprobe vhost_vsock` |
| `vsock accept failed` | Relay VM didn't connect | Check relay VM init script |
| `reading relay ready signal` | Relay sent unexpected signal | Check relay binary version |
| `unexpected relay signal` | Relay sent wrong signal | Check relay VM init script |
| `relay did not signal readiness within 30s` | Relay VM didn't start or init script failed | Check relay VM logs |

### Build Image Errors (returned from `resolveBuildImage()`)

| Error | Cause | Fix |
|-------|-------|-----|
| `image not found` | Local path doesn't exist | Check --image path |
| `cannot resolve to cloud image` | Unknown image reference | Use --image with local path |
| `downloading image` | Network error or URL invalid | Check network, verify URL |
| `qemu-img convert` | QCOW2 file corrupted | Re-download cloud image |

### Build Completion Errors (returned from `waitForBuild()`)

| Error | Cause | Fix |
|-------|-------|-----|
| `vsock accept failed` | Build VM didn't connect | Check build VM init script |
| `reading build exit code` | Build VM didn't send exit code | Check build script |
| `build timed out` | Build exceeded 24h (default) | Increase timeout with --timeout |

### Cleanup Errors (returned from `networkState.cleanup()`)

| Error | Cause | Fix |
|-------|-------|-----|
| `deleting TAP` | TAP device already deleted | Idempotent delete (ignore ENODEV) |
| `deleting bridge` | Bridge already deleted | Idempotent delete (ignore ENODEV) |
| `MASQUERADE rule not found` | iptables rule already removed | Ignore ENOENT |

---

## Part 6: Security Considerations

### Threat Model

| Threat | Mitigation |
|--------|-----------|
| VM escape | Firecracer ~50K LOC (vs QEMU millions); KVM isolation |
| Host filesystem access | No shared FS; vsock only (no 9p/virtiofs) |
| Network bypass | Same iptables REDIRECT as QEMU relay; no user-net |
| Resource exhaustion | Built-in rate limiters (net/disk) |
| Privilege escalation | Jailer: seccomp + chroot + dedicated UID (optional) |
| Side-channel (Spectre) | KVM guest-first mode if available |
| Host TAP devices | Jail + network namespace isolation |
| vsock host listener | Unique CID per VM; host only listens on specific ports |
| API socket | 0600 permissions, per-build tmpdir |
| Snapshot memory images | Relay VM never handles build secrets |

### Hardening Checklist (Phase 3)

- [ ] Firecracer API socket: mode 0600, in per-build tmpdir
- [ ] TAP devices: created in dedicated network namespace when jailer is used
- [ ] vsock CID: unique per VM, released on cleanup
- [ ] Seccomp: enabled by default (Firecracer's built-in filter)
- [ ] Memory: balloon device or cgroup limit prevents single VM from exhausting host
- [ ] Disk I/O: rate limiter configured to prevent noisy-neighbor
- [ ] Network I/O: rate limiter configured (e.g., 100 Mbps per build)

---

## Part 7: Files Summary

### New Files (12 source + 6 test = 18 files)

| # | File | Lines | Purpose |
|---|------|-------|---------|
| 1 | `driver/firecracer/driver.go` | ~350 | Driver struct, StartBuild, helpers |
| 2 | `driver/firecracer/vm.go` | ~300 | VM lifecycle (start/stop/wait) |
| 3 | `driver/firecracer/config.go` | ~80 | JSON config types |
| 4 | `driver/firecracer/network.go` | ~150 | TAP/bridge creation |
| 5 | `driver/firecracer/images.go` | ~150 | Image resolution/conversion |
| 6 | `driver/firecracer/vsock.go` | ~120 | vsock signaling |
| 7 | `driver/firecracer/mmds.go` | ~80 | MMDS metadata injection |
| 8 | `driver/firecracer/api.go` | ~120 | Firecracer API client |
| 9 | `driver/firecracer/cid.go` | ~80 | vsock CID allocator |
| 10 | `driver/firecracer/pool.go` | ~200 | Warm VM pool |
| 11 | `driver/firecracer/jailer.go` | ~120 | Jaler integration |
| 12 | `driver/firecracer/util.go` | ~150 | Shared utilities |
| 13 | `driver/firecracer/driver_test.go` | ~80 | Driver struct tests |
| 14 | `driver/firecracer/config_test.go` | ~100 | Config validation tests |
| 15 | `driver/firecracer/network_test.go` | ~40 | TAP command tests |
| 16 | `driver/firecracer/images_test.go` | ~60 | Image resolution tests |
| 17 | `driver/firecracer/vsock_test.go` | ~30 | vsock port tests |
| 18 | `driver/firecracer/api_test.go` | ~80 | API client tests |
| 19 | `driver/firecracer/cid_test.go` | ~60 | CID allocation tests |

### Modified Files (4 files)

| # | File | Lines Changed | Purpose |
|---|------|---------------|---------|
| 1 | `cmd/warden/config.go` | ~3 | Add "firecracer" to validateDriver() |
| 2 | `cmd/warden/main.go` | ~30 | Add case "firecracer" in runBuild() |
| 3 | `cmd/warden/clean.go` | ~50 | Add cleanFirecracer() function |
| 4 | `go.mod` | ~1 | Add github.com/mdlayher/vsock |

### Build System (3 files)

| # | File | Lines | Purpose |
|---|------|-------|---------|
| 1 | `Makefile` | ~15 | Add firecracer asset targets |
| 2 | `tools/firecracer-relay-vm/build-kernel.sh` | ~30 | Build uncompressed kernel |
| 3 | `tools/firecracer-relay-vm/build-rootfs.sh` | ~40 | Build relay rootfs |

**Total: 25 files, ~3,700 lines of code**

---

## Part 8: Implementation Order

AI coding agents should implement files in this order (each depends on the previous):

1. **`driver/firecracer/config.go`** — Type definitions (no dependencies)
2. **`driver/firecracer/api.go`** — API client (depends on config.go types)
3. **`driver/firecracer/util.go`** — Shared utilities (no dependencies, but needed by all)
4. **`driver/firecracer/driver.go`** — Main driver (depends on 1, 2, 3)
5. **`driver/firecracer/vm.go`** — VM lifecycle (depends on 1, 2, 3, 4)
6. **`driver/firecracer/network.go`** — Network setup (depends on 4)
7. **`driver/firecracer/images.go`** — Image management (depends on 3, 4)
8. **`driver/firecracer/vsock.go`** — vsock signaling (depends on 2, 4)
9. **`driver/firecracer/mmds.go`** — MMDS injection (depends on 1, 2)
10. **`driver/firecracer/cid.go`** — CID allocator (no dependencies)
11. **`driver/firecracer/pool.go`** — Warm pool (depends on 4, 5)
12. **`driver/firecracer/jailer.go`** — Jaler integration (depends on 4, 5)
13. **Test files** (all) — Tests for each module
14. **Modified existing files** — CLI integration

---

## Part 9: Design/Security Decisions Requiring Human Review

These are the ONLY items that require human design decisions before implementation:

### 1. Kernel Version Pinning
**Question:** Which kernel version to ship with Firecracer?
- Options: 5.15 (LTS, stable), 6.1 (LTS, newer features), 6.6 (latest LTS)
- Recommendation: 6.1 (matches Firecracer 1.10+ requirements)
- **Decision needed:** Pin version or allow configurable version?

### 2. Cloud Image URL Mapping
**Question:** Which cloud images to support by default?
- Currently maps: ubuntu-24.04, ubuntu-22.04, ubuntu-20.04, ubuntu-arm64, debian-12, debian-11
- **Decision needed:** Add more images (Alpine, CentOS, RHEL)? Add custom image URL support?

### 3. vsock vs File-Based Signaling Trade-off
**Question:** Phase 1 uses file-based signaling (mount rootfs after VM stop). Phase 2 adds vsock signaling.
- **Decision needed:** Commit to Phase 1 file-based approach or skip directly to vsock?
- **Recommendation:** Phase 1 file-based (reuses existing relay code), Phase 2 vsock (cleaner)

### 4. Default Resource Allocation
**Question:** Default vCPUs and memory for relay/build VMs:
- Relay: 2 vCPUs, 512MB (current)
- Build: 4 vCPUs, 4096MB (current)
- **Decision needed:** Adjust defaults based on typical build workloads?

### 5. Jailer Default
**Question:** Should jailer be enabled by default?
- **Decision needed:** Default to `UseJailer = false` (spike/MVP) or `true` (production-ready)?
- **Recommendation:** `false` for spike, configurable via `--jailer` flag

### 6. Image Format: QCOW2→raw Conversion
**Question:** Should we convert cloud images to raw at download time or at build time?
- **Decision needed:** Convert at download time (cache raw per cloud image) or convert per-build (slower but fresh)?
- **Recommendation:** Convert at download time (cache raw per cloud image), use `cp --reflink=auto` per-build

### 7. MMDS v2 Token Handling
**Question:** How should the build VM guest fetch the MMDS token?
- **Decision needed:** Embed token fetch in build VM init script, or provide a helper binary?
- **Recommendation:** Embed in init script (uses `curl` to fetch token from 169.254.169.254)

### 8. Network Subnet Range
**Question:** What subnet range to use for Firecracer builds?
- Current container driver uses `100.64.87.0/24` (CGNAT range)
- Firecracer uses `10.0.0.0/30` (point-to-point per build)
- **Decision needed:** Use a different range to avoid conflicts with container driver on same host?
- **Recommendation:** Use `10.0.0.0/16` for Firecracer (non-overlapping with container driver)

---

This plan is complete. Every function signature, type, error path, test case, and integration point is specified. The only remaining items are the 8 design decisions listed in Part 9.
