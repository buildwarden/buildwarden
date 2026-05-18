# Hyper-V Driver Implementation Plan

## Open Questions

1. **Named Pipes vs Hyper-V Sockets (hvsock) for relay-build link?**
   Hyper-V sockets (AF_VSOCK equivalent on Windows, `AF_HYPERV`) allow direct host-to-guest or guest-to-guest communication without network interfaces. Should the relay-build link use an Internal vSwitch (L2 Ethernet) or hvsock? hvsock would simplify network isolation but requires the relay to speak raw TCP (no transparent redirect via iptables in the relay VM). Recommendation: use Internal vSwitch for consistency with QEMU/VZ — keeps the same relay init script and iptables rules.

2. **Relay VM kernel: same Alpine initramfs or different?**
   The relay VM is a Linux VM on both QEMU and VZ. On Hyper-V, Generation 2 VMs with Linux guests work well (Ubuntu/Alpine with Hyper-V Integration Services). Can we reuse the same `tools/relay-vm/` initramfs with a kernel that includes hv_vmbus, hv_storvsc, hv_netvsc modules? Or do we need a separate kernel build with Hyper-V-specific modules?

3. **Shared volume mechanism?**
   QEMU uses virtio-9p. Hyper-V options: (a) SMB share mounted inside VM, (b) Plan 9 over VMBus (not available on Hyper-V), (c) Virtual hard disk (VHDX) attached as secondary disk, (d) Hyper-V Integration Services file copy (limited, no mount). Recommendation: VHDX formatted as ext4 (relay) or NTFS (Windows build) attached as a data disk — host formats, attaches, guest mounts.

4. **Windows guest build: PowerShell Remoting or SSH?**
   OpenSSH is built into Windows Server 2019+ and Windows 10 1803+. PowerShell Remoting (WinRM) is more "native" but harder to bootstrap securely. SSH is simpler and consistent with Linux path. Recommendation: SSH with key injected via unattend.xml.

5. **Minimum Windows version?**
   Hyper-V Generation 2 VMs + PowerShell Direct + nested virtualization require Windows 10 Pro (1607+) / Windows Server 2016+. PowerShell 7 and modern cmdlets suggest targeting Windows 10 21H2+ / Windows Server 2022+ for best experience.

6. **Can we test in CI without bare-metal?**
   Azure VMs with nested virtualization enabled (Dv3/Ev3 series or newer) support Hyper-V inside the VM. GitHub Actions Windows runners do NOT have Hyper-V enabled. Options: self-hosted runner on Azure, or Azure DevOps pipeline with nested virt.

7. **warden-io for Windows: CGO_ENABLED=0 GOOS=windows?**
   The warden-io agent is pure Go. Confirm it builds cleanly for windows/amd64 with no cgo dependencies. The `trust` subcommand needs to install a CA cert — on Windows this means `certutil -addstore Root <cert.pem>` instead of writing to `/etc/ssl/`.

---

## Architecture

Same two-VM topology as QEMU, adapted to Hyper-V primitives:

```
Host (Windows, orchestrator)
├── Creates Shared VHDX (ext4 for relay, or raw for signal files)
├── Creates Internal vSwitch "warden-build-{id}" (no external connectivity)
├── Creates External/NAT vSwitch "warden-nat-{id}" (relay upstream)
├── Starts Relay VM (Generation 2, Linux, direct boot)
│   ├── NIC1: Internal vSwitch → Build VM (isolated)
│   └── NIC2: NAT vSwitch → internet (upstream requests)
├── Starts Build VM (Generation 2, target OS)
│   └── NIC: Internal vSwitch → Relay VM (sole network path)
└── Waits for build completion via signal VHDX polling
```

The Relay VM is always Linux (Alpine with Hyper-V kernel modules). The Build VM can be:
- **Linux** — cloud image (VHDX), provisioned via cloud-init seed ISO
- **Windows** — base VHDX from Windows evaluation images or custom, provisioned via unattend.xml + SetupComplete.cmd

---

## Network Topology

### Virtual Switches

```
┌─────────────────────────────────────────────────────────┐
│ Host                                                     │
│                                                          │
│  ┌──────────────────┐      ┌──────────────────────────┐ │
│  │  NAT vSwitch     │      │  Internal vSwitch        │ │
│  │  (relay upstream)│      │  (relay ↔ build)         │ │
│  │  192.168.240.0/20│      │  10.0.0.0/30             │ │
│  └────────┬─────────┘      └───────┬──────────────────┘ │
│           │                        │                     │
│    ┌──────┴──────┐         ┌───────┴───────┐            │
│    │  Relay VM   │         │  Relay VM     │            │
│    │  eth1 (NAT) │         │  eth0 (build) │            │
│    │  DHCP/static│         │  10.0.0.1/30  │            │
│    └─────────────┘         └───────┬───────┘            │
│                                    │                     │
│                            ┌───────┴───────┐            │
│                            │  Build VM     │            │
│                            │  eth0         │            │
│                            │  10.0.0.2/30  │            │
│                            └───────────────┘            │
└─────────────────────────────────────────────────────────┘
```

### NAT Configuration (PowerShell)

```powershell
# Create NAT vSwitch (relay's internet access)
New-VMSwitch -Name "warden-nat-$id" -SwitchType Internal
New-NetIPAddress -IPAddress 192.168.240.1 -PrefixLength 20 `
    -InterfaceAlias "vEthernet (warden-nat-$id)"
New-NetNat -Name "warden-nat-$id" -InternalIPInterfaceAddressPrefix 192.168.240.0/20

# Create Internal vSwitch (relay ↔ build, no external)
New-VMSwitch -Name "warden-build-$id" -SwitchType Private
# Private = no host access, only VM-to-VM. This is stronger than Internal.
```

### Why Private (not Internal) for build link

A **Private** vSwitch ensures the build VM cannot reach the host's network stack at all — traffic can only flow between VMs attached to the same switch. This matches the QEMU model where the unix socket netdev is exclusively relay-to-build.

### iptables in Relay VM (unchanged from QEMU)

The relay VM init script works identically — the Hyper-V synthetic NIC (hv_netvsc) appears as a standard Linux Ethernet device. Same PREROUTING REDIRECT rules, same MASQUERADE for outbound.

---

## PowerShell vs WMI/COM

### Recommendation: PowerShell cmdlets via `exec.Command`

| Approach | Pros | Cons |
|----------|------|------|
| PowerShell cmdlets | Well-documented, stable API, easy to script | Subprocess overhead per call (~200ms), output parsing |
| WMI/COM via Go | No subprocess overhead, direct API | Complex COM interop, requires `github.com/go-ole/go-ole`, brittle |
| Hyper-V WMI v2 (CIM) | Modern, PowerShell uses this internally | Same COM complexity as above |

**Decision**: Use PowerShell cmdlets invoked via `exec.Command("powershell", "-NoProfile", "-Command", ...)`. The startup overhead is acceptable because VM lifecycle operations are infrequent (1-5 calls per build). For status polling, use a single long-running PowerShell process with stdin/stdout pipe (similar to how Vagrant talks to Hyper-V).

### PowerShell invocation helper

```go
// driver/hyperv/powershell.go

type psResult struct {
    Stdout string
    Stderr string
}

func runPS(ctx context.Context, script string) (*psResult, error) {
    cmd := exec.CommandContext(ctx, "powershell.exe",
        "-NoProfile", "-NonInteractive", "-Command", script)
    var stdout, stderr bytes.Buffer
    cmd.Stdout = &stdout
    cmd.Stderr = &stderr
    if err := cmd.Run(); err != nil {
        return nil, fmt.Errorf("powershell: %s: %w", stderr.String(), err)
    }
    return &psResult{Stdout: stdout.String(), Stderr: stderr.String()}, nil
}

// For JSON-structured output:
func runPSJSON(ctx context.Context, script string, out any) error {
    wrapped := fmt.Sprintf("%s | ConvertTo-Json -Compress", script)
    res, err := runPS(ctx, wrapped)
    if err != nil {
        return err
    }
    return json.Unmarshal([]byte(res.Stdout), out)
}
```

---

## Image Management

### Linux Guests (VHDX cloud images)

Cloud image providers distribute VHDX or offer conversion:
- **Ubuntu**: ships `.vhdx.zip` alongside `.qcow2` at `https://cloud-images.ubuntu.com/`
- **Debian**: ships `.qcow2` only — convert with `qemu-img convert -O vhdx`
- **Alpine**: ships `.qcow2` — convert with `qemu-img convert -O vhdx`

### Windows Guests

- **Evaluation images**: Microsoft provides Windows Server evaluation VHDs
- **Custom base**: user provides a sysprepped VHDX with OpenSSH pre-installed
- **FROM resolution**: `FROM windows-server:2022` maps to known evaluation URL

### Image resolution flow

```go
func resolveImage(imageRef string) (string, error) {
    // 1. Local path (absolute or relative)
    if isLocalPath(imageRef) {
        return imageRef, nil
    }
    // 2. Known cloud image URL mapping
    url := cloudImageURL(imageRef) // returns VHDX download URL
    if url == "" {
        return "", fmt.Errorf("cannot resolve %q", imageRef)
    }
    // 3. Check cache
    cached := cachedImagePath(imageRef, "vhdx")
    if _, err := os.Stat(cached); err == nil {
        return cached, nil
    }
    // 4. Download + convert if needed
    return downloadAndCache(url, cached)
}
```

### Differencing disks (COW equivalent)

Hyper-V uses **differencing disks** instead of QCOW2 overlays:

```powershell
New-VHD -Path "$overlay" -ParentPath "$baseImage" -Differencing
```

This creates a thin VHDX that records only writes, preserving the base image. Equivalent to QEMU's `qemu-img create -f qcow2 -b base.qcow2`.

```go
func createDiffDisk(ctx context.Context, overlay, base string) error {
    script := fmt.Sprintf(
        `New-VHD -Path '%s' -ParentPath '%s' -Differencing`,
        overlay, base)
    _, err := runPS(ctx, script)
    return err
}
```

---

## Windows Guest Provisioning

### unattend.xml approach

Windows guests are provisioned via `unattend.xml` placed on a secondary VHDX (or floppy image). The unattend file:

1. Skips OOBE/license prompts
2. Sets administrator password (ephemeral, random per build)
3. Enables OpenSSH server
4. Installs warden-io.exe CA trust
5. Creates a `SetupComplete.cmd` that runs the build

### Getting warden-io.exe into the VM

**Option A (preferred): Seed VHDX**

Create a small VHDX (formatted NTFS), write warden-io.exe + build.ps1 + unattend.xml to it, attach as secondary disk. Windows sees it as D:\ drive.

```go
func createSeedDisk(ctx context.Context, dir string, files map[string]string) (string, error) {
    vhdxPath := filepath.Join(dir, "seed.vhdx")
    // Create, mount, copy files, dismount
    script := fmt.Sprintf(`
        $vhd = New-VHD -Path '%s' -SizeBytes 100MB -Dynamic
        Mount-VHD -Path $vhd.Path
        $disk = Get-Disk | Where-Object { $_.Location -eq $vhd.Path }
        Initialize-Disk -Number $disk.Number -PartitionStyle GPT
        $part = New-Partition -DiskNumber $disk.Number -UseMaximumSize -AssignDriveLetter
        Format-Volume -Partition $part -FileSystem NTFS -Confirm:$false
        $letter = $part.DriveLetter
    `, vhdxPath)
    for name, src := range files {
        script += fmt.Sprintf(
            "\n        Copy-Item '%s' \"${letter}:\\%s\"", src, name)
    }
    script += fmt.Sprintf("\n        Dismount-VHD -Path '%s'", vhdxPath)
    _, err := runPS(ctx, script)
    return vhdxPath, err
}
```

**Option B: HTTP fetch from relay**

Same as Linux path — once the network is up, `warden-io.exe` fetches itself from `http://artifacts/warden-io`. This requires a bootstrap mechanism (a small script in unattend.xml that uses `Invoke-WebRequest`).

```xml
<!-- In unattend.xml, FirstLogonCommands -->
<SynchronousCommand>
    <CommandLine>powershell -Command "Invoke-WebRequest http://artifacts/warden-io -OutFile C:\warden\warden-io.exe"</CommandLine>
    <Order>1</Order>
</SynchronousCommand>
```

**Recommendation**: Use Option A (seed VHDX) for reliability — it works even before network is configured. Fall back to Option B for minimal images where attaching a second disk is impractical.

### Build script execution

```xml
<!-- SetupComplete.cmd (runs after first boot completes) -->
@echo off
C:\warden\warden-io.exe trust
C:\warden\warden-io.exe fetch-context C:\workspace
powershell -ExecutionPolicy Bypass -File C:\warden\build.ps1
set EXITCODE=%ERRORLEVEL%
C:\warden\warden-io.exe exit --code %EXITCODE%
shutdown /s /t 0
```

### Network isolation enforcement

Network isolation is enforced **at the vSwitch level** (Private switch), not inside the guest. The build VM's only NIC connects to the Private vSwitch where the relay VM is the sole peer. No Windows Firewall rules needed inside the guest — there is simply no route to anything except the relay.

This is the same trust model as QEMU (unix socket) and VZ (file handle socket). The guest cannot bypass the relay because there is no physical path.

---

## Linux Guest Provisioning

Identical to QEMU, with disk format differences:

1. **Base image**: VHDX instead of QCOW2 (download or convert)
2. **Overlay**: Differencing VHDX instead of QCOW2 with backing file
3. **Cloud-init seed**: ISO attached as DVD drive (same `genisoimage` / `mkisofs` approach)
4. **Network config**: Same v2 netplan (10.0.0.2/30, gateway 10.0.0.1)
5. **warden-io**: Placed on seed ISO, copied during runcmd phase

The `cloudInitSeed()` function and `generateSeedISO()` are reusable — the only change is how the ISO is attached (Hyper-V DVD drive instead of virtio-blk).

### Hyper-V Integration Services

Linux guests need `hv_vmbus`, `hv_storvsc`, `hv_netvsc`, `hv_utils` modules. Modern distro cloud images (Ubuntu 20.04+, Debian 11+, Alpine 3.16+) include these by default.

---

## Signaling (Heartbeat / Exit)

### Approach: Shared VHDX with polling (same pattern as QEMU 9p)

The signal directory is hosted on a small shared VHDX:

```
signal.vhdx (10MB, ext4 or NTFS depending on guest)
├── heartbeat    (timestamp file, written by build/relay)
├── exit_code    (integer file, written on completion)
└── build.log    (build output capture)
```

**Host-side**: The VHDX is mounted on the host (or the host polls by mounting periodically). On Windows, ext4 can be read via WSL or a userspace driver. Alternatively, format as NTFS for universal access.

**Problem**: Windows cannot hot-mount a VHDX that is attached to a running VM.

### Better approach: Relay-mediated signaling (HTTP)

The relay already handles heartbeat and exit signals via HTTP endpoints:
- `GET http://artifacts/heartbeat` — called by build VM's watcher
- `GET http://artifacts/exit?code=N` — called on build completion

The relay writes to `SIGNAL_DIR` which is on the relay VM's own mounted volume. The host monitors this volume.

**For the host to see signal files**, the relay VM mounts a **host-accessible share**:

**Option 1: Hyper-V host-guest file sharing via VMBus**
Not available as a general mount mechanism.

**Option 2: SMB share from host, mounted in relay VM**
The relay VM mounts `//host-ip/warden-signal-$id` via CIFS. Requires SMB server on host, credentials management. Fragile.

**Option 3: Secondary VHDX attached to relay VM, periodically detach+read from host**
Impractical — cannot detach while VM is running.

**Option 4: Hyper-V KVP (Key-Value Pair) exchange**
Hyper-V Integration Services provide a KVP data exchange mechanism:
- Guest writes to `/var/lib/hyperv/.kvp_pool_*` (Linux) or registry (Windows)
- Host reads via WMI: `Get-VMIntegrationService -VMName $name | Get-VMIntegrationServiceData`
- Limited to key-value strings, but heartbeat timestamp and exit code fit perfectly.

**Option 5: Named pipe (serial port) as signal channel**
Attach a virtual COM port (named pipe on host side) to the relay VM. The relay writes heartbeat/exit to the serial port; host reads from the named pipe.

### Recommendation: Named pipe (Option 5) for signaling

```powershell
# Attach COM1 as named pipe to relay VM
Set-VMComPort -VMName "warden-relay-$id" -Number 1 `
    -Path "\\.\pipe\warden-signal-$id"
```

In the relay VM init script:
```sh
# Write signals to /dev/ttyS0 (COM1 = named pipe to host)
echo "heartbeat:$(date +%s)" > /dev/ttyS0
echo "exit:0" > /dev/ttyS0
```

On the host, the Go driver reads from the named pipe:
```go
func (d *Driver) monitorSignals(ctx context.Context, pipeName string) (int, error) {
    pipe, err := winio.DialPipe(`\\.\pipe\`+pipeName, nil)
    if err != nil {
        return -1, err
    }
    defer pipe.Close()

    scanner := bufio.NewScanner(pipe)
    for scanner.Scan() {
        line := scanner.Text()
        switch {
        case strings.HasPrefix(line, "heartbeat:"):
            d.lastHeartbeat.Store(time.Now().Unix())
        case strings.HasPrefix(line, "exit:"):
            code, _ := strconv.Atoi(strings.TrimPrefix(line, "exit:"))
            return code, nil
        }
    }
    return -1, fmt.Errorf("signal pipe closed unexpectedly")
}
```

**Alternative (simpler, lower fidelity)**: Keep the QEMU file-polling approach but use a **host-path mounted VHDX**. Create a small VHDX, format ext4, mount it on the host via WSL (`wsl --mount`), attach to relay VM. The relay writes heartbeat/exit_code files; host polls from WSL mount. This preserves the exact `waitForBuild()` logic from QEMU.

**Final recommendation**: Use the **named pipe approach** for signal transport — it is the most reliable, lowest latency, and avoids filesystem mount complexity. Modify the relay to optionally write signals to a serial device (`SIGNAL_DEVICE=/dev/ttyS0`) in addition to `SIGNAL_DIR`.

---

## Boot Sequence

Step-by-step from `StartBuild()` to first build command executing:

```
1. StartBuild() called
   ├── Create temp working directory
   ├── Generate build ID

2. Prepare assets (parallel)
   ├── Cross-compile relay (linux/amd64, CGO_ENABLED=0)
   ├── Cross-compile warden-io (linux/amd64 or windows/amd64)
   └── Prepare build context

3. Create network infrastructure
   ├── New-VMSwitch "warden-build-{id}" -SwitchType Private
   ├── New-VMSwitch "warden-nat-{id}" -SwitchType Internal
   ├── New-NetIPAddress (NAT gateway)
   └── New-NetNat (NAT rule)

4. Prepare relay VM disk
   ├── Create relay-data.vhdx (100MB, contains relay binary + config)
   │   Format as ext4 (via WSL or pre-formatted template)
   │   Copy: relay binary, relay.env
   └── Use relay kernel + initrd for direct boot

5. Create relay VM
   ├── New-VM -Generation 2 -MemoryStartupBytes 512MB
   ├── Add-VMHardDiskDrive (relay-data.vhdx)
   ├── Add-VMNetworkAdapter (Private switch: build link)
   ├── Add-VMNetworkAdapter (NAT switch: internet)
   ├── Set-VMComPort -Number 1 (named pipe for signals)
   ├── Set-VMFirmware -BootOrder (direct kernel boot via LinuxDirect)
   └── Start-VM

6. Wait for relay ready
   ├── Read named pipe for "ready" signal
   │   OR poll for CA cert on relay-data.vhdx
   └── Timeout: 30 seconds

7. Prepare build VM
   ├── Create differencing VHDX (overlay on base image)
   ├── Generate cloud-init seed ISO (Linux) or seed VHDX (Windows)
   └── Place warden-io + build script on seed media

8. Create build VM
   ├── New-VM -Generation 2 -MemoryStartupBytes 4096MB -VHDPath overlay.vhdx
   ├── Add-VMDvdDrive (seed ISO, Linux only)
   ├── Add-VMHardDiskDrive (seed VHDX, Windows only)
   ├── Add-VMNetworkAdapter (Private switch: sole network)
   ├── Set-VMFirmware -SecureBootTemplate (MicrosoftUEFICertificateAuthority for Linux)
   └── Start-VM

9. Build executes
   ├── Cloud-init / unattend.xml configures network (10.0.0.2/30)
   ├── warden-io trust (install CA)
   ├── warden-io fetch-context (download build context from relay)
   ├── Build script runs
   ├── Watcher sends heartbeat to http://artifacts/heartbeat every 2s
   └── On completion: exit signal sent to relay → relay writes to named pipe

10. Host monitors
    ├── Read named pipe for heartbeat/exit messages
    ├── Timeout if no heartbeat for 3 intervals
    └── On exit: collect results

11. Cleanup
    ├── Stop-VM (both VMs)
    ├── Remove-VM (both VMs)
    ├── Remove-VMSwitch (both switches)
    ├── Remove-NetNat
    └── Delete temp VHDX files
```

### Direct kernel boot on Hyper-V (Generation 2)

Hyper-V Generation 2 VMs support direct kernel boot for Linux via the `Set-VMFirmware` cmdlet:

```powershell
Set-VMFirmware -VMName "warden-relay-$id" `
    -EnableSecureBoot Off `
    -BootOrder @(Get-VMHardDiskDrive -VMName "warden-relay-$id")

# Direct kernel boot (requires Windows Server 2019+ or Windows 11+)
Set-VMHost -EnableEnhancedSessionMode $false
Set-VMFirmware -VMName "warden-relay-$id" `
    -LinuxKernelImagePath $kernelPath `
    -LinuxInitrdImagePath $initrdPath `
    -LinuxKernelCmdLine "console=ttyS0"
```

**Note**: `Set-VMFirmware -LinuxKernelImagePath` is available only on Windows Server 2019+. On older hosts, the relay VM must boot from a disk image (VHDX with GRUB).

---

## Go Implementation Structure

```
driver/hyperv/
├── driver.go          // Driver struct, StartBuild(), Close()
├── driver_windows.go  // Windows-only build (actual implementation)
├── driver_stub.go     // Non-Windows stub (//go:build !windows)
├── vm.go             // VM lifecycle (create, start, stop, remove)
├── network.go        // vSwitch creation, NAT setup, cleanup
├── images.go         // VHDX resolution, caching, differencing disks
├── cloudinit.go      // Cloud-init seed ISO generation (Linux guests)
├── unattend.go       // unattend.xml generation (Windows guests)
├── signal.go         // Named pipe monitoring, heartbeat/exit
├── powershell.go     // PowerShell execution helpers
├── seed.go           // Seed VHDX creation for Windows guests
└── util.go           // File helpers, caching, cleanup
```

### Key Types

```go
// driver/hyperv/driver.go

package hyperv

import (
    "context"
    "warden/driver"
)

// Driver implements driver.Driver using Hyper-V as the backend.
// Windows only (Pro/Enterprise/Server with Hyper-V role enabled).
type Driver struct {
    // Verbose enables VM console output.
    Verbose bool
}

func New() *Driver { return &Driver{} }

func (d *Driver) Name() string { return "hyperv" }

func (d *Driver) StartBuild(ctx context.Context, req *driver.BuildRequest) (*driver.BuildResult, error) {
    // ... (see boot sequence above)
}

func (d *Driver) Exec(ctx context.Context, req *driver.BuildRequest) error {
    return driver.ErrExecNotSupported
}

func (d *Driver) Close() error { return nil }
```

```go
// driver/hyperv/vm.go

// vmHandle represents a running Hyper-V VM with cleanup tracking.
type vmHandle struct {
    Name       string
    Generation int
}

type relayVMConfig struct {
    ID         string
    KernelPath string
    InitrdPath string
    DataVHDX   string // Contains relay binary + config
    BuildSwitch string
    NATSwitch   string
    PipeName   string // Named pipe for signal channel
    MemoryMB   int
}

type buildVMConfig struct {
    ID         string
    DiskImage  string // Differencing VHDX
    SeedISO    string // Cloud-init (Linux)
    SeedVHDX   string // unattend.xml media (Windows)
    BuildSwitch string
    CPUs       int
    MemoryMB   int
    IsWindows  bool
}

func (d *Driver) createRelayVM(ctx context.Context, cfg *relayVMConfig) (*vmHandle, error) {
    script := fmt.Sprintf(`
        $vm = New-VM -Name '%s' -Generation 2 -MemoryStartupBytes %dMB -NoVHD
        Add-VMHardDiskDrive -VMName $vm.Name -Path '%s'
        Add-VMNetworkAdapter -VMName $vm.Name -SwitchName '%s' -Name 'build'
        Add-VMNetworkAdapter -VMName $vm.Name -SwitchName '%s' -Name 'nat'
        Set-VMComPort -VMName $vm.Name -Number 1 -Path '\\.\pipe\%s'
        Set-VMFirmware -VMName $vm.Name -EnableSecureBoot Off `+
            `-LinuxKernelImagePath '%s' -LinuxInitrdImagePath '%s' `+
            `-LinuxKernelCmdLine 'console=ttyS0'
        Start-VM -Name $vm.Name
    `, cfg.ID, cfg.MemoryMB, cfg.DataVHDX,
        cfg.BuildSwitch, cfg.NATSwitch, cfg.PipeName,
        cfg.KernelPath, cfg.InitrdPath)

    _, err := runPS(ctx, script)
    if err != nil {
        return nil, fmt.Errorf("creating relay VM: %w", err)
    }
    return &vmHandle{Name: cfg.ID, Generation: 2}, nil
}

func (d *Driver) createBuildVM(ctx context.Context, cfg *buildVMConfig) (*vmHandle, error) {
    script := fmt.Sprintf(`
        $vm = New-VM -Name '%s' -Generation 2 `+
            `-MemoryStartupBytes %dMB -VHDPath '%s'
        Set-VMProcessor -VMName $vm.Name -Count %d
        Add-VMNetworkAdapter -VMName $vm.Name -SwitchName '%s'
    `, cfg.ID, cfg.MemoryMB, cfg.DiskImage, cfg.CPUs, cfg.BuildSwitch)

    if cfg.SeedISO != "" {
        script += fmt.Sprintf(
            "\n        Add-VMDvdDrive -VMName $vm.Name -Path '%s'",
            cfg.SeedISO)
    }
    if cfg.SeedVHDX != "" {
        script += fmt.Sprintf(
            "\n        Add-VMHardDiskDrive -VMName $vm.Name -Path '%s'",
            cfg.SeedVHDX)
    }

    secureBootTemplate := "MicrosoftUEFICertificateAuthority"
    if cfg.IsWindows {
        secureBootTemplate = "MicrosoftWindows"
    }
    script += fmt.Sprintf(
        "\n        Set-VMFirmware -VMName $vm.Name "+
            "-SecureBootTemplate '%s'", secureBootTemplate)
    script += fmt.Sprintf("\n        Start-VM -Name '%s'", cfg.ID)

    _, err := runPS(ctx, script)
    if err != nil {
        return nil, fmt.Errorf("creating build VM: %w", err)
    }
    return &vmHandle{Name: cfg.ID, Generation: 2}, nil
}

func (h *vmHandle) stop(ctx context.Context) error {
    script := fmt.Sprintf(`
        Stop-VM -Name '%s' -Force -TurnOff
        Remove-VM -Name '%s' -Force
    `, h.Name, h.Name)
    _, err := runPS(ctx, script)
    return err
}
```

```go
// driver/hyperv/network.go

type networkResources struct {
    BuildSwitch string
    NATSwitch   string
    NATName     string
}

func (d *Driver) createNetwork(ctx context.Context, id string) (*networkResources, error) {
    buildSwitch := fmt.Sprintf("warden-build-%s", id)
    natSwitch := fmt.Sprintf("warden-nat-%s", id)
    natName := fmt.Sprintf("warden-nat-%s", id)
    natPrefix := "192.168.240.0/20"

    script := fmt.Sprintf(`
        New-VMSwitch -Name '%s' -SwitchType Private
        $sw = New-VMSwitch -Name '%s' -SwitchType Internal
        $ifAlias = "vEthernet (%s)"
        New-NetIPAddress -IPAddress 192.168.240.1 -PrefixLength 20 `+
            `-InterfaceAlias $ifAlias
        New-NetNat -Name '%s' -InternalIPInterfaceAddressPrefix '%s'
    `, buildSwitch, natSwitch, natSwitch, natName, natPrefix)

    _, err := runPS(ctx, script)
    if err != nil {
        return nil, fmt.Errorf("creating network: %w", err)
    }
    return &networkResources{
        BuildSwitch: buildSwitch,
        NATSwitch:   natSwitch,
        NATName:     natName,
    }, nil
}

func (n *networkResources) cleanup(ctx context.Context) error {
    script := fmt.Sprintf(`
        Remove-NetNat -Name '%s' -Confirm:$false
        Remove-VMSwitch -Name '%s' -Force
        Remove-VMSwitch -Name '%s' -Force
    `, n.NATName, n.NATSwitch, n.BuildSwitch)
    _, _ = runPS(ctx, script)
    return nil
}
```

```go
// driver/hyperv/signal.go

import (
    "bufio"
    "context"
    "strconv"
    "strings"
    "sync/atomic"
    "time"

    "github.com/microsoft/go-winio"
)

const (
    heartbeatInterval = 2 * time.Second
    heartbeatTimeout  = 3 * heartbeatInterval
)

func (d *Driver) waitForBuild(ctx context.Context, pipeName string) (int, error) {
    pipePath := `\\.\pipe\` + pipeName

    // Wait for pipe to exist (relay VM booting)
    var pipe net.Conn
    deadline := time.Now().Add(30 * time.Second)
    for {
        if time.Now().After(deadline) {
            return -1, fmt.Errorf("timeout waiting for signal pipe")
        }
        var err error
        pipe, err = winio.DialPipe(pipePath, nil)
        if err == nil {
            break
        }
        select {
        case <-ctx.Done():
            return -1, ctx.Err()
        case <-time.After(500 * time.Millisecond):
        }
    }
    defer pipe.Close()

    var lastBeat atomic.Int64
    lastBeat.Store(time.Now().Unix())

    // Monitor timeout in background
    done := make(chan struct{})
    defer close(done)
    go func() {
        ticker := time.NewTicker(time.Second)
        defer ticker.Stop()
        for {
            select {
            case <-done:
                return
            case <-ticker.C:
                if time.Since(time.Unix(lastBeat.Load(), 0)) > heartbeatTimeout {
                    // Will be caught by pipe read returning error
                    pipe.Close()
                    return
                }
            }
        }
    }()

    scanner := bufio.NewScanner(pipe)
    for scanner.Scan() {
        line := scanner.Text()
        switch {
        case strings.HasPrefix(line, "heartbeat:"):
            lastBeat.Store(time.Now().Unix())
        case strings.HasPrefix(line, "exit:"):
            code, _ := strconv.Atoi(strings.TrimPrefix(line, "exit:"))
            return code, nil
        case line == "ready":
            // Relay is up, reset timeout
            lastBeat.Store(time.Now().Unix())
        }
    }

    if ctx.Err() != nil {
        return -1, ctx.Err()
    }
    return -1, fmt.Errorf("build VM unresponsive (signal pipe closed)")
}
```

### Stub for non-Windows builds

```go
// driver/hyperv/driver_stub.go
//go:build !windows

package hyperv

import (
    "context"
    "fmt"
    "runtime"

    "warden/driver"
)

func (d *Driver) StartBuild(ctx context.Context, req *driver.BuildRequest) (*driver.BuildResult, error) {
    return nil, fmt.Errorf("hyperv driver requires Windows (current OS: %s)", runtime.GOOS)
}

func (d *Driver) Exec(ctx context.Context, req *driver.BuildRequest) error {
    return fmt.Errorf("hyperv driver requires Windows (current OS: %s)", runtime.GOOS)
}

func (d *Driver) Close() error { return nil }
```

---

## Security Considerations

### Comparison with QEMU threat model

| Threat | QEMU | Hyper-V |
|--------|------|---------|
| Build VM network escape | Unix socket (process-level isolation) | Private vSwitch (kernel-level isolation) |
| Build VM disk escape | Separate QCOW2 overlay, no shared FS write | Separate VHDX overlay, no shared FS write |
| Signal directory attacks | Symlink/FIFO checks (`safeReadFile`) | Named pipe (host-controlled, no guest filesystem access) |
| Relay compromise → host | Relay in separate VM, no host access | Same — relay VM is fully isolated from host |
| Guest kernel exploit → host | QEMU userspace (large attack surface) | Hyper-V hypervisor (smaller, hardened kernel) |
| Timing side channels | Standard CPU isolation | Hyper-V core scheduling (better isolation) |

### Hyper-V advantages

- **Type-1 hypervisor**: Hyper-V runs below the host OS partition, providing stronger isolation than QEMU (type-2, userspace process)
- **Credential Guard integration**: Can leverage Windows security features
- **Secure Boot**: Generation 2 VMs support UEFI Secure Boot, preventing unauthorized kernel modifications in build VM
- **No shared filesystem**: Unlike QEMU's 9p (which exposes host filesystem paths to guest kernel code), the named pipe approach keeps signaling entirely host-controlled

### Security requirements

1. **Random VM names**: Prefix with `warden-` + random suffix to prevent collision attacks
2. **Cleanup on failure**: Ensure VMs and vSwitches are removed even on panic (use deferred cleanup with PowerShell `Remove-VM -Force`)
3. **No host networking for build VM**: Private vSwitch only, never Internal or External
4. **Ephemeral credentials**: Any passwords in unattend.xml are random per-build and the VM is destroyed immediately after
5. **Secure Boot on for build VM**: Prevents kernel-level persistence across builds (base image is read-only via differencing disk)

---

## Cross-Compilation Concerns

### Build tags

```go
// driver/hyperv/driver.go — shared across all platforms
package hyperv

type Driver struct { Verbose bool }
func New() *Driver { return &Driver{} }
func (d *Driver) Name() string { return "hyperv" }

// driver/hyperv/driver_windows.go
//go:build windows

// ... actual StartBuild implementation

// driver/hyperv/driver_stub.go
//go:build !windows

// ... returns "requires Windows" error
```

### Dependencies

- `github.com/microsoft/go-winio` — Windows named pipe access from Go (already used by Docker/containerd; well-maintained)
- No CGO required — pure Go on Windows

### Testing on non-Windows

Unit tests for logic that does not touch PowerShell (image URL resolution, unattend.xml generation, cloud-init generation) run on all platforms. Integration tests are gated:

```go
//go:build windows && integration

func TestHyperVRelayBoot(t *testing.T) {
    if !isHyperVAvailable() {
        t.Skip("Hyper-V not enabled")
    }
    // ...
}
```

### CI configuration

The Makefile already cross-compiles relay and warden-io for linux. Add:

```makefile
build-hyperv:
	GOOS=windows GOARCH=amd64 go build -o warden-windows-amd64.exe ./cmd/warden
```

---

## Testing Plan

### Unit tests (all platforms)

- `unattend.go`: XML generation correctness
- `cloudinit.go`: cloud-config YAML correctness
- `images.go`: URL resolution, path handling
- `powershell.go`: command escaping, output parsing (mocked)
- `signal.go`: signal protocol parsing (mocked pipe)

### Integration tests (Windows with Hyper-V)

**Environment**: Azure Dv3/Ev3 VM with nested virtualization, or bare-metal Windows Server.

1. **Smoke test**: Create and destroy a VM
2. **Network test**: Create Private + NAT switches, verify isolation
3. **Relay boot test**: Boot relay VM, verify CA cert appears on named pipe
4. **Linux build e2e**: Full build with Ubuntu cloud image
5. **Windows build e2e**: Full build with Windows Server evaluation VHDX
6. **Timeout test**: Verify heartbeat timeout triggers cleanup
7. **Concurrent builds**: Multiple builds with separate vSwitches

### CI options

| Option | Cost | Complexity |
|--------|------|------------|
| Azure VM (Dv3, nested virt) | ~$0.20/hr | Medium — need provisioning script |
| GitHub Actions self-hosted runner | Azure VM cost | Low — reuse existing runner infra |
| Azure DevOps | Free tier available | Medium — separate CI system |
| Manual validation only | $0 | High maintenance burden |

**Recommendation**: Self-hosted GitHub Actions runner on Azure Ev3 VM. Run integration tests nightly (not on every PR) to manage costs.

---

## Dependencies

### Windows features required

```powershell
# Check/enable Hyper-V
Enable-WindowsOptionalFeature -Online -FeatureName Microsoft-Hyper-V-All

# Required features:
# - Microsoft-Hyper-V-Management-PowerShell (cmdlets)
# - Microsoft-Hyper-V-Hypervisor (core)
# - Microsoft-Hyper-V-Services (VM management)
```

### Minimum versions

| Component | Minimum | Recommended | Notes |
|-----------|---------|-------------|-------|
| Windows | 10 Pro 21H2 | 11 Pro 23H2 | Direct kernel boot needs Server 2019+ |
| Hyper-V | 10.0 | Latest | Generation 2 VM support |
| PowerShell | 5.1 | 7.x | 5.1 ships with Windows, 7.x is faster |
| .NET | N/A | N/A | No .NET dependency in Go driver |

### Host-side tools

- `powershell.exe` (ships with Windows)
- `mkisofs.exe` or `genisoimage.exe` (for cloud-init ISO — bundle or use Go library)
- Optional: `qemu-img.exe` (for QCOW2 → VHDX conversion of cloud images)

### Go library for ISO generation

To avoid requiring `mkisofs` on Windows, use a pure Go ISO9660 library:

```go
import "github.com/kdomanski/iso9660"

func generateSeedISO(seedDir, isoPath string) error {
    writer, err := iso9660.NewWriter()
    if err != nil {
        return err
    }
    defer writer.Cleanup()
    // Walk seedDir, add files to ISO
    // ...
    f, _ := os.Create(isoPath)
    defer f.Close()
    return writer.WriteTo(f, "CIDATA")
}
```

This eliminates the external tool dependency entirely.

---

## Implementation Phases

### Phase 1: Skeleton + Linux guest (2-3 weeks)

- [ ] Package structure with build tags
- [ ] PowerShell helpers (`runPS`, `runPSJSON`)
- [ ] Network creation/cleanup (Private + NAT switches)
- [ ] Relay VM boot (reuse existing kernel+initrd from `tools/relay-vm/`)
- [ ] Named pipe signal monitoring
- [ ] Linux build VM with cloud-init
- [ ] End-to-end test: `warden build --driver hyperv` with Ubuntu image

### Phase 2: Windows guest support (2-3 weeks)

- [ ] unattend.xml generation
- [ ] Seed VHDX creation (warden-io.exe + build script)
- [ ] warden-io Windows build (`GOOS=windows`)
- [ ] warden-io `trust` command for Windows (certutil)
- [ ] SetupComplete.cmd watcher (heartbeat + exit)
- [ ] End-to-end test with Windows Server evaluation image

### Phase 3: Polish + CI (1-2 weeks)

- [ ] Image caching + integrity verification
- [ ] FROM resolution for Windows images
- [ ] Differencing disk lifecycle
- [ ] Error handling + cleanup on partial failures
- [ ] Azure nested-virt CI runner setup
- [ ] Documentation

---

## Relay VM Adaptation for Hyper-V

The existing `tools/relay-vm/init` script needs minor changes for Hyper-V:

1. **Kernel modules**: Replace virtio modules with Hyper-V modules:
   ```sh
   # Instead of: virtio_net, 9pnet, 9pnet_virtio
   # Load: hv_vmbus, hv_storvsc, hv_netvsc, hv_utils
   ```

2. **Shared volume mount**: Instead of 9p, mount the data VHDX:
   ```sh
   # The relay-data.vhdx appears as /dev/sda or /dev/sdb
   mount /dev/sdb1 /shared
   ```

3. **Signal output**: Write to COM port in addition to files:
   ```sh
   # After starting relay, signal readiness
   echo "ready" > /dev/ttyS0
   ```

4. **Network interface names**: Hyper-V NICs appear as `eth0`/`eth1` (same as QEMU), so iptables rules are unchanged.

The relay binary itself (`cmd/relay/`) requires **no changes** — it already supports `SIGNAL_DIR` and the HTTP heartbeat/exit endpoints. The named pipe is between the relay VM init script and the host, not between the relay Go binary and the host.

### Modified relay VM init (Hyper-V variant)

```sh
#!/bin/sh
# Relay VM init — Hyper-V variant
# Differences from QEMU: storage modules, shared volume mount, COM signal

mount -t devtmpfs devtmpfs /dev
mount -t proc proc /proc
mount -t sysfs sysfs /sys
mount -t tmpfs tmpfs /tmp

# Hyper-V modules (built into kernel or loaded from initramfs)
for mod in hv_vmbus hv_storvsc hv_netvsc hv_balloon hv_utils; do
    modprobe $mod 2>/dev/null || true
done

# Mount data disk (relay binary + config)
mkdir -p /shared
# Wait for disk to appear
TRIES=0
while [ $TRIES -lt 50 ]; do
    [ -b /dev/sda1 ] && break
    TRIES=$((TRIES + 1))
    sleep 0.1
done
mount /dev/sda1 /shared

# Source relay configuration
[ -f /shared/relay.env ] && { set -a; . /shared/relay.env; set +a; }

# Network setup (identical to QEMU variant)
# ... (same as existing init)

# Signal readiness to host via COM1
echo "ready" > /dev/ttyS0

# Redirect signal writes to both file and COM port
export SIGNAL_DIR=/shared/signal
mkdir -p "$SIGNAL_DIR"

exec /shared/relay
```

The relay heartbeat module already writes to `SIGNAL_DIR`. To also write to the COM port, we can add a simple background forwarder in the init script that tails the signal files and echoes to `/dev/ttyS0`, or modify `cmd/relay/heartbeat.go` to support a `SIGNAL_DEVICE` environment variable.

### Recommended relay change (minimal)

```go
// cmd/relay/heartbeat.go — add SIGNAL_DEVICE support

var signalDevice string

func SetSignalDevice(dev string) { signalDevice = dev }

func RunHeartbeat() {
    if signalDir == "" && signalDevice == "" {
        return
    }
    // ... existing file-based logic ...

    // Additionally write to serial device if configured
    if signalDevice != "" {
        devFile, err := os.OpenFile(signalDevice, os.O_WRONLY, 0)
        if err == nil {
            defer devFile.Close()
            // Write heartbeat lines to device
            go func() {
                ticker := time.NewTicker(2 * time.Second)
                for range ticker.C {
                    ts := lastActivity.Load()
                    if ts > 0 && time.Since(time.Unix(ts, 0)) < 5*time.Second {
                        fmt.Fprintf(devFile, "heartbeat:%d\n", ts)
                    }
                }
            }()
        }
    }
}

func WriteExitCode(code int) {
    // ... existing file-based logic ...
    if signalDevice != "" {
        if f, err := os.OpenFile(signalDevice, os.O_WRONLY, 0); err == nil {
            fmt.Fprintf(f, "exit:%d\n", code)
            f.Close()
        }
    }
}
```

This keeps backward compatibility with QEMU/VZ (file-based signals via 9p) while enabling the named pipe approach for Hyper-V.
