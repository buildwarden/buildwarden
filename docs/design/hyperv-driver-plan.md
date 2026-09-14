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

## Privilege & Elevation Model

The Hyper-V driver is the only BuildWarden driver that requires host-level OS privilege, and getting the elevation UX right is a shipping requirement, not a footnote. This section is the authoritative design for how the driver acquires privilege and what the local-dev and shippable experiences look like. It shapes package structure, so build the `Provisioner` spine (below) before the VM/boot code, and build the dev-setup stub (Phase 0) before anything that needs a network.

### Two Windows privilege tiers

The privileged operations do not all need the same level, and the whole strategy follows from that split:

| Operation set | Cmdlets | Privilege required | UAC prompt |
|---|---|---|---|
| Host network standup | `New-VMSwitch`, `New-NetIPAddress`, `New-NetNat`, `Remove-NetNat` | Full **Administrator** | Yes |
| VM lifecycle | `New-VM`, `Start-VM`, `Stop-VM`, `Remove-VM`, `Add-VMNetworkAdapter`, `New-VHD` (differencing), `Set-VMFirmware`, `Set-VMComPort` | Local **Hyper-V Administrators** group | No |

The key consequence: only the host-network standup forces full Administrator. A developer who is a member of Hyper-V Administrators can run the entire per-build VM path with no elevation and no UAC prompt, as long as the switch/NAT already exist. Everything below is built to isolate that one full-admin operation and do it as rarely as possible (ideally once).

### Provisioner interface (the code-reuse spine)

Every privileged operation sits behind a single interface so that the same logic serves both the in-process path and the service path:

```go
// driver/hyperv/provision.go

type Provisioner interface {
    // Full-admin ops
    EnsureNetwork(ctx context.Context, spec NetworkSpec) (*NetworkResources, error)
    TeardownNetwork(ctx context.Context, id string) error
    // Hyper-V Administrators ops
    CreateVM(ctx context.Context, cfg VMSpec) (*vmHandle, error)
    StartVM(ctx context.Context, name string) error
    StopVM(ctx context.Context, name string) error
    RemoveVM(ctx context.Context, name string) error
    CreateDiffDisk(ctx context.Context, overlay, base string) error
    // ... com port, seed disk, etc.
}
```

Two implementations:

- `localProvisioner` — in-process; calls hcsshim / PowerShell directly. Used whenever the current process already holds the required privilege.
- `clientProvisioner` — marshals the identical calls over a named pipe to the privileged service.

The service host instantiates a `localProvisioner` and exposes it over the pipe. `localProvisioner` is therefore the single source of truth for all privileged logic; only the transport is duplicated. This is the minimal client/server wrapper: the interface is the reuse boundary.

### Three runtime outcomes for `warden --driver hyperv`

Resolved **per operation**, not by one global "am I admin?" check. A naive `IsUserAnAdmin()` boolean is a bug here: a Hyper-V Administrators member reads as "not admin" yet can do every VM op directly, and a partial boolean would send them down path 1 and then fail on `New-VMSwitch`.

1. **Process holds the privilege for the op** (full admin, or Hyper-V Admins for VM ops) → `localProvisioner` performs it directly.
2. **Process lacks it AND the network service is installed and running** → `clientProvisioner` delegates the op to the service.
3. **Process lacks it AND no service** → fast-error, naming both remediations: rerun from a terminal launched as Administrator, or run the one-time `warden hyperv setup` (for the network) / install the service, both under admin.

In practice the host-network standup is the only operation that routinely lands in outcome 2 or 3; VM ops resolve to outcome 1 under Hyper-V Administrators. Detection at startup probes token elevation and Hyper-V Administrators group membership, then caches a per-op capability verdict.

### The privileged service (staged LATER, not a Phase-1 prerequisite)

The service is a LocalSystem Windows service that is a thin wrapper hosting `localProvisioner` behind a named pipe. It is the best local-dev UX (zero prompts ever, the Docker Desktop model) but it is **not** needed to prove the driver, and it carries an installer, an uninstaller, lifecycle/recovery config, and version lockstep. Do not build it before the driver runs end to end.

Mandatory security properties (this is a privileged endpoint that will run `New-VM` / `New-VMSwitch` on request):

- **Typed op set only.** The pipe carries structured requests (`CreateVM{name, vhdx, switch}`), never raw PowerShell or arbitrary argument strings.
- **ACL the pipe** to Hyper-V Administrators (or a dedicated group) and authenticate the caller.
- **Protocol version handshake** on connect. A stale service speaking an old protocol against a new `warden` is a real failure mode; `warden` should detect the mismatch and offer to reinstall/upgrade (itself a one-time elevated op).

This same `Provisioner`-behind-RPC boundary is what a third-party Windows orchestrator reusing the relay will need, so the interface work also pays into the standalone-relay goal.

### `warden hyperv setup`

**Network provisioning policy (decided 2026-09-14):** when the driver *can* stand up a network — full Administrator, or the LocalSystem service — it creates a **fresh, ephemeral per-build network** and tears it down afterward. That is the cleanest isolation and avoids the collisions and side-effects of a shared, long-lived switch. The durable named switch below is a **lower-privilege fallback only**: it exists so a non-elevated Hyper-V Administrators member (which cannot stand up a network) still has an isolated path to reuse. An elevated run never reuses it. `doctor` surfaces the durable switch as available for lower-privilege use.

**Shippable form:** a one-time, elevated command that idempotently creates a durable, named vSwitch + NAT + host IP for the lower-privilege path. After it runs once, a non-elevated `warden build --driver hyperv --switch <name>` runs under Hyper-V Administrators with no elevation. It must detect insufficient privilege and emit a clear, actionable error instead of surfacing a raw PowerShell failure.

**Incremental-dev stub (BUILD THIS FIRST, Phase 0):** a minimal `warden hyperv setup` that creates exactly one durable, reused switch (Private or Internal, plus its NAT + host IP) under a fixed dev name (e.g. `warden-dev`). Run it **once** from an elevated shell. A **non-elevated** build then reuses that switch via `--switch <name>` (or the `WARDEN_HYPERV_SWITCH` env var), skipping per-build network standup entirely (an elevated build ignores it and stands up a fresh network). This is what lets the implementation agent iterate on the VM/boot/signal/build harness without persistent admin and without running the gateway elevated.

Carry these reuse hazards into the stub (they are the durable-network dangers, and they apply equally to the shippable form):

- **Idempotent create, and verify-on-each-build rather than trust persistence.** Re-assert every build that the switch is still Private/Internal, expected port ACLs are present, and no rogue host vNIC bridges the build VM out. Persistence does not equal correctness; a prior crash or a manual edit can silently weaken isolation.
- **The build VM must NEVER get a direct NAT route to the internet.** NAT exists only for the relay VM's controlled egress. A build VM with straight NAT defeats the entire audit/MITM model.
- **Pick a non-colliding subnet.** WSL2, Docker Desktop, and the Hyper-V Default Switch all allocate NATs/prefixes; an overlapping prefix silently breaks routing. Detect existing `New-NetNat` allocations and fail clearly rather than clobbering.
- **Serialize builds on a shared dev switch.** The static `10.0.0.2/30` relay+build topology fits exactly one pair, so two concurrent builds on one reused switch clash on IPs. Guard with a host lock, or allocate a per-build subnet.

These hazards are precisely why an elevated run does **not** reuse a shared switch: the fresh per-build `createNetwork()` + teardown path shown earlier under Network Topology sidesteps all of them (no persistence to verify, no cross-build IP clashes, no shared-switch collisions). The durable reused switch accepts these hazards as the cost of running without elevation, and must guard them (verify-on-each-build, NAT collision detection, a host lock). So the default is fresh per-build whenever the process can stand up a network (full Administrator, or the LocalSystem service); the durable switch is the reuse path for the non-elevated Hyper-V Administrators case only. Phase 0 delivers the durable switch (it is what unblocks the non-elevated dev loop); the ephemeral per-build path is the standing default, not an option.

### `warden hyperv doctor` (capability preflight)

`warden hyperv doctor` is a read-only preflight that reports exactly which capabilities the CLI has and which it still needs authorization for, then resolves the runtime outcome (1/2/3 from above) per operation and prints the precise next action. It is especially valuable for agent-based development by other contributors: an implementation agent (or a new human contributor) can run it first to learn whether it can iterate directly, must delegate to the service, or needs a one-time elevated `warden hyperv setup`, instead of discovering the boundary through a mid-build `ACCESS_DENIED`.

**Detection is two-layer:**

1. **Query the process token up front** (no side effects, needs no privilege):
   - Full-Administrator / elevation state via `GetTokenInformation(TokenElevation)` (`golang.org/x/sys/windows`: `windows.OpenCurrentProcessToken()`, then `token.IsElevated()`). Decides whether the host-network standup ops are available.
   - Hyper-V Administrators membership via `CheckTokenMembership` against the well-known SID `S-1-5-32-578` (`DOMAIN_ALIAS_RID_HYPER_V_ADMINS`). Decides whether the VM-lifecycle ops are available without elevation.
2. **Classify the real error as a backstop.** Token detection has UAC filtered-token edge cases and the true authority is enforced by HCS, so the op path must still map a cmdlet `ERROR_ACCESS_DENIED` (`WIN32 5`) to the same remediation message rather than surfacing a raw PowerShell failure. Detection chooses the path; error classification catches a wrong guess.

**Token-scope caveat to surface in the output:** the token checks reflect the *current process's* token. Because the gateway spawns `warden` non-elevated, `doctor` will correctly report "not elevated" there; the elevated verdict only appears when `warden` is launched from an elevated terminal (a different token) or routed through the service. `doctor` should state which token it read so the result is not misleading.

**Checks (all cheap and read-only):**

| Capability | How detected | Privilege to detect |
|---|---|---|
| Hypervisor present | `Win32_ComputerSystem.HypervisorPresent` (CIM) | read-only |
| Hyper-V feature enabled | `Get-WindowsOptionalFeature`, or the `vmms` service exists | read-only |
| Full Administrator (network standup) | token `IsElevated()` | none |
| Hyper-V Administrators (VM lifecycle) | `CheckTokenMembership(S-1-5-32-578)` | none |
| Durable/dev switch present | `Get-VMSwitch <name>` | read-only |
| Network service reachable | dial the named pipe | none |

**Example output shape:**

```
$ warden hyperv doctor
Hyper-V hypervisor present ............ yes
Hyper-V feature enabled ............... yes
Token read ............................ current process (non-elevated)
Full Administrator .................... no   (needed for: New-VMSwitch, New-NetNat, New-NetIPAddress)
Hyper-V Administrators ................ yes  (covers: New-VM, Start-VM, Remove-VM, adapters, VHDX)
Dev switch 'warden-dev' ............... not found
Privileged service .................... not installed

Resolved:
  VM lifecycle ......... OK (direct, Hyper-V Administrators)
  Network standup ...... BLOCKED
    -> run `warden hyperv setup` once from an elevated shell, or
    -> launch warden from an Administrator terminal, or
    -> install the network service (one-time, elevated)
```

`doctor` exits non-zero when a required capability for the requested mode is unmet, so it is usable as a CI/agent gate. The same detection routine backs the per-operation outcome resolution in the driver itself, so `doctor` is a thin CLI surface over logic the driver already needs.

### Cloud / headless fleets (Azure, AWS)

The elevation pain largely evaporates on a non-GUI fleet: there is no interactive desktop, so there is no interactive UAC, and automation runs already-elevated as a service account (WinRM/SSH/service). The service model is the natural fit there, and setup is baked into the golden image. The real gate is **nested-virtualization SKU availability**:

- **AWS:** always available on bare-metal (`.metal`) instances, and since February 2026 also on virtual Nitro instances built on Intel Xeon 6 (C8i / M8i / R8i). See [EC2 nested virtualization docs](https://docs.aws.amazon.com/en_us/AWSEC2/latest/UserGuide/amazon-ec2-nested-virtualization.html) and the [launch note](https://aws.amazon.com/about-aws/whats-new/2026/02/amazon-ec2-nested-virtualization-on-virtual).
- **Azure:** Dv3/Ev3 and newer support it; B-series burstable does **not**; ARM SKUs (Dpsv5) run KVM, not Hyper-V. See the [nested virtualization guide](https://learn.microsoft.com/azure/lab-services/concept-nested-virtualization-template-vm) and [enable-nested-virtualization](https://learn.microsoft.com/en-us/windows-server/virtualization/hyper-v/enable-nested-virtualization).
- Windows **Server** enables Hyper-V via `Install-WindowsFeature Hyper-V` (+ reboot), and the auto-created Default Switch is not reliably present, so the durable `warden hyperv setup` path is mandatory there rather than optional.

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

## Output egress: streaming sink (decided 2026-09-14)

The relay VM produces the ledger and, more importantly, build artifacts that can
be very large — PyTorch-style wheels (100s of MB), container images (GBs). The
relay-VM topology has no host-shared path (unlike qemu's 9p or vz's host-process
relay), Hyper-V cannot host-mount a VHDX attached to a running VM, and mounting
needs elevation anyway — so routing artifacts through a data VHDX would mean a
full second copy of GB-scale data plus elevation. Instead:

**The relay streams outputs to the host as they are produced, never buffering a
full artifact.** A new output-sink seam in the `relay` library:

- `localSink` — current behavior (write to `LEDGER_DIR`/output on local fs).
  Remains the DEFAULT, so the container/qemu/vz drivers are behaviourally
  unchanged (no Phase-3 macOS-loop-back regression).
- `httpSink` — streams each artifact as an HTTP `PUT ${SINK_URL}/<name>` with a
  chunked/streamed body; the artifact handler `io.Copy`s the incoming build-env
  upload straight through to the sink connection (constant memory, GB-safe). The
  ledger streams the same way.
- Selected by config: `OUTPUT_SINK_URL` set → `httpSink`, else `localSink`.

The receiver is a **generalized, reusable component**, not something bespoke to
the Hyper-V driver: a `collector` package with a standalone `cmd/collector`
binary (symmetric with `cmd/relay` / `cmd/warden-io`), which drivers embed
**in-process** (one implementation, two entrypoints — exactly how `relay` is both
a library and `cmd/relay`). It is the counterpart to the relay's `httpSink`:
receive streamed artifacts/ledger over chunked HTTP and land them in the output
dir. A third-party orchestrator on any platform can run `cmd/collector` (or front
`SINK_URL` with S3/MinIO/WebDAV) and reuse the relay + warden-io unchanged — the
third of three composable primitives (relay = witness/egress, warden-io =
in-guest agent, collector = ingress/sink).

On Hyper-V the driver embeds the collector in-process, bound to the
internal-switch IP (`192.168.240.1`, never `0.0.0.0`) and gated by a per-build
bearer token; only the relay VM can reach it (the build VM is on the isolated
Private switch). Because the relay-VM topology gives the host no shared view of
the guest fs, **the Hyper-V driver has no local sink** — the collector sink is
its only option; `localSink` remains for container/qemu/vz where the relay writes
to a host-shared path. Artifacts flow build-env → relay → collector in one
streamed hop; nothing large lands in the relay VM, so its VHDXs stay tiny.

Protocol choice: plain **HTTP with streamed/chunked bodies** — the relay is
already an HTTP server, HTTP bodies stream by definition, and it is the
lowest-common-denominator standard. It also serves the standalone-relay goal: a
third-party orchestrator collects by running a trivial HTTP endpoint, or fronts
`SINK_URL` with S3/MinIO/WebDAV. (gRPC and S3-multipart were considered; they add
dependencies and complexity a reliable host-local hop does not need.)

**Future nice-to-have — make the streaming sink the default across all drivers.**
Investigate whether the sink seam (`httpSink`, or the abstraction generally)
should REPLACE the local-fs / 9p / host-process output paths as the default for
container/qemu/vz too, unifying output egress on one streamed, well-known
protocol instead of per-driver mechanisms. Upside: one code path, no 9p/host-fs
coupling, uniform backpressure/capacity behaviour, and the same
third-party-collectable seam everywhere. Deferred and gated on not regressing the
existing drivers' zero-copy fast paths; revisit alongside the HCS boot item after
Phase 1.

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
   ├── Set-VMFirmware -EnableSecureBoot Off -FirstBootDevice <relay VHDX> (UEFI/GRUB boot)
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

### Relay VM boot: GRUB/EFI bootable VHDX (verified 2026-09-14)

**Ground truth (Windows 11 Pro 26200, Hyper-V PowerShell module 2.0):** the
Hyper-V PowerShell module exposes **no** Linux direct-kernel-boot parameters.
`Set-VMFirmware` and `New-VM` have no `LinuxKernelImagePath` /
`LinuxInitrdImagePath` / `LinuxKernelCmdLine` (an earlier draft of this plan
assumed these — they do not exist in the module; the capability was conflated
with QEMU's `-kernel`). Generation-2 VMs here boot UEFI from a disk only.

So the relay VM boots from a **UEFI-bootable VHDX** that wraps the audited
minimal initramfs built by `tools/relay-vm/hyperv/`:

- GPT VHDX with an EFI System Partition (FAT32).
- GRUB2 at `\EFI\BOOT\BOOTX64.EFI` (the firmware's default fallback path), plus
  our `vmlinuz` + `initramfs.cpio.gz` on the ESP.
- `grub.cfg`: `linux /vmlinuz console=ttyS0 ...` + `initrd /initramfs.cpio.gz`.
- Secure Boot **off** (unsigned kernel/bootloader).

```powershell
Set-VMFirmware -VMName "warden-relay-$id" -EnableSecureBoot Off `
    -FirstBootDevice (Get-VMHardDiskDrive -VMName "warden-relay-$id")
```

This keeps the minimal, audited Alpine initramfs (the relay trust-boundary
choice) and stays entirely on the PowerShell driver. The build VM is unaffected
(cloud images are already bootable VHDXs).

**Future nice-to-have — HCS / `hcsshim` direct kernel boot.** The Host Compute
Service (`github.com/microsoft/hcsshim`) *can* direct-boot a Linux kernel+initrd
with no bootloader — it is how Windows boots LCOW (Linux Containers on Windows)
utility VMs. Adopting it would drop the GRUB/EFI packaging step entirely and is
the more elegant long-term boot path, at the cost of the lower-level HCS API
(JSON compute-system schema via a cgo/DLL wrapper) and a heavy dependency.
Deferred; revisit after Phase 1 proves the driver.

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

Elevation staging is folded into the phase order: build the privilege spine and the dev-setup stub FIRST (Phase 0) so the harness can be developed without persistent admin, and defer the full service to LAST (Phase 4) since it is not needed to prove the driver.

### Phase 0: Privilege foundation + dev harness (do first)

- [x] `Provisioner` interface + `localProvisioner` in-process implementation (interface + network standup done; VM-lifecycle ops in progress)
- [x] `warden hyperv setup` **dev stub**: create one durable reused switch (Private/Internal) + NAT + host IP under a fixed dev name (`warden-dev`), idempotent
- [~] Driver switch reuse: `--switch <name>` / `WARDEN_HYPERV_SWITCH` parsed by setup/doctor; driver-level skip-standup lands with `StartBuild`
- [x] Per-operation capability detection (token elevation + Hyper-V Administrators membership), plus a stale-token cross-check against persistent group membership
- [x] `warden hyperv doctor`: read-only capability preflight (token + group + feature + switch + service), resolves outcome per op, exits non-zero when blocked so it works as a CI/agent gate
- [x] Outcome-3 fast-error with both remediations (elevated terminal, or one-time setup) when an op cannot be satisfied
- [x] No service yet. Dev loop = Jeff runs the setup stub once from an elevated shell, then the non-elevated gateway iterates against the reused switch (VM ops run under Hyper-V Admins) — validated live: `doctor` resolves VM lifecycle direct + network reuse

**Exit criteria:** `warden --driver hyperv` can create and destroy VMs against the reused dev switch with the gateway running non-elevated.

### Phase 1: Skeleton + Linux guest (2-3 weeks)

- [ ] Package structure with build tags
- [ ] PowerShell helpers (`runPS`, `runPSJSON`)
- [ ] Network via `Provisioner`: fresh per-build `EnsureNetwork` + teardown when the process can stand up a network (elevated / service); reuse the durable switch when non-elevated
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
- [ ] **Shippable `warden hyperv setup`**: durable named switch/NAT/IP, idempotent, privilege detection with a clear actionable error (not a raw PowerShell failure)
- [ ] Azure nested-virt CI runner setup
- [ ] Documentation

### Phase 4: Privileged service (later; optional for first ship)

- [ ] `clientProvisioner` (marshals `Provisioner` calls over the pipe)
- [ ] LocalSystem Windows service host wrapping `localProvisioner`
- [ ] Installer / uninstaller / recovery config; `warden` can install/upgrade it (one-time elevated op)
- [ ] Security hardening: typed op set only, pipe ACL'd to Hyper-V Administrators, caller authentication, protocol version handshake
- [ ] Outcome-2 delegation wired end to end

**Exit criteria:** `warden build --driver hyperv` runs with zero prompts from a non-elevated shell with no dev switch pre-created.

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
