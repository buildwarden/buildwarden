# Architecture

BuildWarden executes builds inside isolated environments while recording a
tamper-evident ledger of all network activity. Three drivers implement the
same lifecycle on different platforms:

| Driver | Flag | Platform | Isolation mechanism |
|--------|------|----------|---------------------|
| Container | `--driver container` (default) | Any Docker/Podman host | iptables sidecar |
| QEMU | `--driver qemu` | macOS, Linux, Windows | Topological (socket pair) |
| VZ | `--driver vz` | macOS/arm64 only | Topological (socketpair, no vmnet) |

All three share a single runtime model:

```
relay starts --> warden-io initialize --> fetch build.sh --> run with heartbeats --> report exit
```

## High-level topology

```
                    Container driver
                    ~~~~~~~~~~~~~~~~
  +-----------+     isolated Docker network (100.64.x.0/29)     +-----------------+
  |   Relay   |<-------- only allowed connection -------------->| Build Container |
  | container |                                                 |  (FROM image)   |
  +-----+-----+                                                 +-----------------+
        |                                                        iptables via
        v  external                                              netns sidecar
   [ Internet ]


                    QEMU driver
                    ~~~~~~~~~~~
  +------------+     Unix socket pair     +------------+
  |  Relay VM  |<------------------------>|  Build VM  |
  |  (Alpine)  |   sole network path      | (cloud img)|
  +-----+------+                          +------------+
        |
        v  external
   [ Internet ]


                    VZ driver
                    ~~~~~~~~~
  +------------------+     socketpair (raw Ethernet)     +-------------+
  | Relay (host proc)|<--------------------------------->|  macOS VM   |
  | gvisor netstack  |    sole network path              | (IPSW img)  |
  +--------+---------+                                   +-------------+
           |
           v  external
      [ Internet ]
```

In every case the build environment has exactly one network path: through the
relay. The relay proxies allowed traffic to the internet and records it in the
ledger.

## Components

### Orchestrator (`cmd/warden/`)

Runs on the host. Responsibilities:

1. Translate the Dockerfile into a shell build script
2. Place the build script and context files for the relay to serve
3. Prepare the relay (build container image, cross-compile binary, or launch host process)
4. Start the relay
5. Start the build environment (container, VM, or macOS VM)
6. Apply network isolation (iptables sidecar or topological)
7. Wait for build completion (heartbeats, exit code)
8. Collect output (ledger, artifacts)
9. Teardown

### Relay (`relay/` library, `cmd/relay/` binary)

The relay is the network gateway for the build environment. Core services:

- **MITM HTTPS proxy** — intercepts TLS using an ephemeral CA
- **DNS server** — resolves `cwd` and `artifacts` to itself, forwards all else upstream
- **Context file server** — serves build script and context via `http://cwd/`
- **Artifact store** — accepts uploads via `http://artifacts/<name>`
- **Ledger writer** — records requests with chained Ed25519 signatures
- **Control plane** — health check, CA distribution, heartbeat, exit reporting
- **Fairness scheduler** — prevents single-origin starvation of concurrent requests

`cmd/relay/` is a thin main that selects a mode:

| Mode | Flag | Used by | Behavior |
|------|------|---------|----------|
| container | `--mode=container` | Container driver | Binds ports directly on container interface |
| vm | `--mode=vm` | QEMU driver | Binds ports directly inside relay VM |
| host | `--mode=host --fd=N` | VZ driver | Reads raw Ethernet from FD via gvisor netstack; SSRF filtering active |

### warden-io (`cmd/warden-io/`)

The sole agent inside the build environment. Subcommands:

- `initialize` — configure DNS, wait for relay, install CA, fetch build.sh, run it, report exit
- `fetch` — download a file from the relay context endpoint
- `post` — upload an artifact to the relay
- `trust` — install the ephemeral CA into system trust stores

Platform-specific initialization handles network configuration differences
between Linux (containers, VMs) and macOS (VZ).

### Ledger (`ledger/`)

Binary format with chained Ed25519-SHA512 signatures. Magic `BLDL`, version
`0x01`. CBOR metadata. Record types: open, checkpoint, close, artifact. The
library provides write (single-writer via channel), read, and verification.

### Driver interface (`driver/`)

Each driver implements a common interface covering:

- Environment preparation (image pull, VM boot)
- Relay startup (mode-specific)
- Build environment startup
- Network isolation application
- Teardown

## Driver details

### Container driver

```
warden build .
  |-- Cross-compile relay + warden-io for linux/amd64
  |-- Create isolated Docker network (100.64.x.0/29)
  |-- Start relay container (mounts context read-only)
  |-- Start build container from Dockerfile's FROM image
  |-- Start iptables sidecar (shares build container netns, applies rules, exits)
  |-- docker cp warden-io into build container
  |-- docker exec warden-io initialize --gateway=<relay_ip>
  |-- Wait for exit
  '-- Teardown containers + network
```

Network isolation: a privileged one-shot sidecar enters the build container's
network namespace, applies iptables rules that allow traffic only to the relay
IP, then exits. The build container has no `CAP_NET_ADMIN`.

### QEMU driver

```
warden build .
  |-- Cross-compile relay + warden-io for linux
  |-- Prepare relay VM (Alpine direct-boot: kernel + initrd with relay binary)
  |-- Prepare build VM (cloud image QCOW2 + cloud-init seed ISO with warden-io)
  |-- Create Unix socket pair
  |-- Boot relay VM (NIC attached to socket A)
  |-- Boot build VM (NIC attached to socket B)
  |-- warden-io initialize runs via cloud-init
  |-- Wait for exit
  '-- Teardown VMs
```

Network isolation is topological: the build VM's only NIC connects to the
socket pair whose other end is the relay VM. No host networking, no bridge.
Accelerator selection: HVF (macOS), KVM (Linux), WHPX (Windows), TCG (fallback).

### VZ driver

```
warden build .
  |-- Prepare macOS disk image (warden image prepare: personalize, inject warden-io)
  |-- Start relay as host process: --mode=host --fd=3
  |     '-- gvisor netstack processes raw Ethernet frames from socketpair
  |-- Boot macOS VM via Virtualization.framework
  |     '-- Single NIC: VZFileHandleNetworkDeviceAttachment on socketpair
  |-- warden-io initialize runs on first boot (system daemon)
  |-- Wait for exit
  '-- Teardown VM + relay process
```

Network isolation is topological: the VM has one network interface backed by a
socketpair. The other end is the relay process using gvisor netstack to
parse Ethernet frames in userspace. No vmnet, no iptables.

Image preparation (`warden image prepare`) is a separate step:
1. Restore IPSW to disk image
2. Personalize with Apple signing server
3. Install warden-io binary and system daemon plist
4. Result is a reusable prepared image

## Lifecycle (common)

```
warden build .
  |-- Translate Dockerfile -> build script (build.sh)
  |-- Place build script in context directory
  |-- Start relay
  |-- Start build environment
  |-- Apply network isolation
  |-- warden-io initialize:
  |     |-- Configure network (point DNS at relay)
  |     |-- Wait for relay health check (http://cwd/healthz)
  |     |-- Fetch + install ephemeral CA
  |     |-- Fetch build.sh from relay context endpoint
  |     |-- Run build script with periodic heartbeats
  |     '-- Report exit code to relay control plane
  |-- Collect output (ledger, artifacts)
  '-- Teardown
```

## Communication paths

All build-environment communication flows through the relay's HTTP API:

| Endpoint | Purpose |
|----------|---------|
| `http://cwd/healthz` | Relay readiness probe |
| `http://cwd/ca` | Ephemeral CA certificate |
| `http://cwd/build.sh` | Build script |
| `http://cwd/<path>` | Context files |
| `http://cwd/control/heartbeat` | Heartbeat (POST) |
| `http://cwd/control/exit` | Exit code report (POST) |
| `http://artifacts/<name>` | Artifact upload (POST) |

External HTTPS traffic is transparently proxied (MITM) and recorded in the
ledger. External HTTP traffic is proxied and recorded without interception.

## Network isolation

See [Network Isolation](./network-isolation.md) for iptables rule details
(container driver). For QEMU and VZ drivers, isolation is topological — the
build environment physically cannot reach anything except the relay.
