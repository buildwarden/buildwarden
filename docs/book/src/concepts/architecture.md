# Architecture

BuildWarden executes builds inside isolated environments while recording a
tamper-evident ledger of all network activity. Four drivers implement the
same lifecycle on different platforms:

| Driver | Flag | Platform | Isolation mechanism |
|--------|------|----------|---------------------|
| Container | `--driver container` (default) | Any Docker/Podman host | iptables sidecar |
| QEMU | `--driver qemu` | macOS, Linux, Windows | Topological (separate VM) |
| VZ | `--driver vz` | macOS/arm64 only | Topological (socketpair, no vmnet) |
| Hyper-V | `--driver hyperv` | Windows | Topological (separate VM, host-only NAT) |

All four share a single runtime model: `warden` (the orchestrator) boots an
unconfigured relay, claims it with the build's configuration, then boots the
build guest.

```
relay boots unconfigured --> warden claims it (tunnel + config) --> build guest boots --> warden-io initialize --> fetch build script --> run with heartbeats --> report exit
```

The relay keeps no build state on disk and mounts nothing. It streams every
output (artifacts, captures, the ledger, build console) to, and pulls every
input (the build script, context files) from, a **collector** that `warden`
runs on the host. The relay never dials the host and `warden` never listens on
any network: the collector is served to the relay over a tunnel `warden` opens
on the relay's claim port.

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


                    Hyper-V driver
                    ~~~~~~~~~~~~~~
  +------------+     Private build switch     +-------------+
  |  Relay VM  |<---------------------------->|  Build VM   |
  |  (Alpine)  |   sole network path          | (Windows)   |
  +-----+------+                              +-------------+
        |  Internal NAT switch
        v  external
   [ Internet ]
```

In every case the build environment has exactly one network path: through the
relay. The relay proxies allowed traffic to the internet and records it in the
ledger.

## Components

### Orchestrator (`cmd/warden/`)

Runs on the host. Responsibilities:

1. Resolve the build script (translate a Dockerfile, or stage build.sh / build.ps1)
2. Start a collector that serves the build script and context, and receives all outputs
3. Prepare and start the relay, unconfigured (build container image, cross-compile binary, or launch host process / boot relay VM)
4. Claim the relay: open the collector tunnel, push the build config, receive the relay identity (MITM CA cert + ledger public key)
5. Start the build environment (container, VM, or macOS VM)
6. Apply network isolation (iptables sidecar or topological)
7. Wait for build completion (heartbeats, exit code) or failure (hung guest, dropped tunnel)
8. Finalize output (optional zstd compression)
9. Teardown

The host-side build logic (`driver.Session`) is shared by every driver;
drivers differ only in how they start the relay and the build guest, and in
the claim address they hand the session.

### Collector (`collector/` library, `cmd/collector/` binary)

The collector is the relay's single output path and the source of its build
inputs, so the relay needs no shared filesystem. The relay authenticates every
request with a bearer token. Endpoints:

| Endpoint | Purpose |
|----------|---------|
| `PUT /v1/artifacts/<name>` | Store a build artifact |
| `PUT /v1/captures/<name>` | Store a captured payload (hash in the `Warden-Hash` trailer) |
| `PUT /v1/ledger` | Stream the ledger for the whole build |
| `PUT /v1/environment` | The environment record's signed payload, landed as `environment.json` |
| `POST /v1/output` | Append build console output |
| `GET /v1/build-script` | The staged build script |
| `GET /v1/context/<path>` | A context file, or a directory listing |
| `POST /v1/ready` | Relay is up |
| `POST /v1/heartbeat` | Build guest is alive |
| `POST /v1/complete` | Build finished (JSON exit code) |
| `GET /v1/status` | Last completion, else running |

The ledger is streamed throughout the build, so even a relay that dies mid-build
leaves a truncated-but-verifiable ledger. `cmd/collector` is the standalone host
side for third-party orchestrators: it claims an already-running relay, serves
inputs, lands outputs, and exits with the build result.

### Claim protocol (`relaycfg/`)

A relay boots unconfigured and listens for a single claim on port 8301. The
orchestrator:

1. Opens a tunnel (`POST /v1/tunnel` on the claim port, upgraded to a raw
   connection) and serves the collector to the relay over it as HTTP/2.
2. Pushes the build config (`POST /v1/claim`): collector URL (the tunnel),
   collector bearer token, capture mode, upstream CA PEMs, system-CA flag, and
   an optional environment record: a typed descriptor of what the build runs on
   (the container image manifest, or for VM drivers a `vm-disk-chain/v1`
   descriptor of the boot disks and per-build seed files; see the ledger
   spec's *Environment Record*). The relay signs it as the ledger's first
   record and lands the payload as `<output>/environment.json`.
3. Receives the relay identity: the ephemeral MITM CA certificate (written to
   `<output>/ca.cert.pem`) and the ledger's Ed25519 public key.

The claim listener closes after one claim, before the build guest boots.
Pushing the config (rather than having the relay fetch it) keeps the relay
ignorant of host topology: any setup where the host can reach the claim port
works, with no host listener or firewall rule, relays can be started warm and
claimed later, and concurrent relays never race for a fixed config endpoint. An
optional per-instance claim token (`WARDEN_CLAIM_TOKEN`) stops another local
process from claiming a warm relay first.

### Relay (`relay/` library, `cmd/relay/` binary)

The relay is the network gateway for the build environment. Core services:

- **MITM HTTPS proxy** — intercepts TLS using an ephemeral CA
- **DNS server** — resolves `cwd` and `artifacts` to itself, forwards all else upstream
- **Build-input serving** — serves the build script and context to the guest, pulling them from the collector
- **Artifact store** — accepts uploads via `http://artifacts/<name>`, streamed to the collector
- **Ledger writer** — records requests with chained Ed25519 signatures, streamed to the collector
- **Control plane** — health check, CA distribution, heartbeat, exit reporting
- **Fairness scheduler** — prevents single-origin starvation of concurrent requests

`cmd/relay/` is a thin main that selects a mode:

| Mode | Flag | Used by | Behavior |
|------|------|---------|----------|
| container | `--mode=container` | Container driver | Binds ports directly on container interface |
| vm | `--mode=vm` | QEMU, Hyper-V drivers | Binds ports directly inside the relay VM |
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
  |-- Start a collector serving the build script + context
  |-- Create isolated Docker network (100.64.x.0/29)
  |-- Start relay container, unconfigured, with its claim port published on 127.0.0.1
  |-- Claim the relay (tunnel + build config); write ca.cert.pem
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
  |-- Start a collector serving the build script + context
  |-- Prepare relay VM (Alpine direct-boot; the current relay binary is appended to the prebuilt initramfs as a second cpio at /sbin/relay)
  |-- Prepare build VM (cloud image QCOW2 + cloud-init seed ISO with warden-io)
  |-- Boot relay VM, unconfigured, with its claim port forwarded to a free 127.0.0.1 port (user-net hostfwd)
  |-- Claim the relay (tunnel + build config); write ca.cert.pem
  |-- Boot build VM (sole network path is the relay VM)
  |-- warden-io initialize runs via cloud-init
  |-- Wait for exit
  '-- Teardown VMs
```

The prebuilt initramfs (`tools/relay-vm/build-initramfs.sh`) only needs
rebuilding when the init script or the base image changes; the relay binary is
appended fresh each build.

Network isolation is topological: the build VM's only NIC connects to the
socket pair whose other end is the relay VM. No host networking, no bridge.
Accelerator selection: HVF (macOS), KVM (Linux), WHPX (Windows), TCG (fallback).

### VZ driver

```
warden build .
  |-- Prepare macOS disk image (warden image prepare: personalize, inject warden-io)
  |-- Start a collector serving the build script + context
  |-- Start relay as host process on 127.0.0.1:<free port>, unconfigured
  |     '-- gvisor netstack processes raw Ethernet frames from socketpair
  |-- Claim the relay (tunnel + build config); write ca.cert.pem
  |-- Boot macOS VM via Virtualization.framework
  |     '-- Single NIC: VZFileHandleNetworkDeviceAttachment on socketpair
  |-- warden-io initialize runs on first boot (system daemon)
  |-- Wait for exit
  '-- Teardown VM + relay process
```

Network isolation is topological: the VM has one network interface backed by a
socketpair. The other end is the relay process using gvisor netstack to
parse Ethernet frames in userspace. No vmnet, no iptables.

Image preparation (`warden image prepare --driver vz`) is a separate step:
1. Restore IPSW to disk image
2. Personalize with Apple signing server
3. Install warden-io binary and system daemon plist
4. Result is a reusable prepared image

### Hyper-V driver

```
warden build --driver hyperv .
  |-- Start a collector serving the build script + context
  |-- Boot relay VM (its /sbin/relay is baked into a UKI boot disk), unconfigured
  |     '-- Reachable on its static host-only NAT IP 192.168.240.2:8301
  |-- Claim the relay (tunnel + build config); write ca.cert.pem
  |-- Boot build VM (sole network path is the relay VM)
  |-- warden-io initialize runs on first boot
  |-- Wait for exit
  '-- Teardown VMs
```

Network isolation is topological, like QEMU. The relay VM and build VM sit on
a Private build switch; the relay reaches upstream through an Internal NAT
switch. The claim endpoint is only on the host-only NAT switch, so the build
VM cannot reach it. One-time host setup (`warden setup --driver hyperv`) creates
the durable switches; the relay VM's `/sbin/relay` is baked into its UKI boot
disk by `tools/relay-vm/hyperv/build.sh`.

## Lifecycle (common)

```
warden build .
  |-- Resolve the build script (translate Dockerfile, or stage build.sh / build.ps1)
  |-- Start a collector serving the build script + context
  |-- Start relay (unconfigured)
  |-- Claim the relay (open tunnel, push config, receive CA + ledger key)
  |-- Start build environment
  |-- Apply network isolation
  |-- warden-io initialize:
  |     |-- Configure network (point DNS at relay)
  |     |-- Wait for relay health check (http://artifacts/health)
  |     |-- Fetch + install ephemeral CA
  |     |-- Fetch the build script (http://artifacts/build-script)
  |     |-- Run build script with periodic heartbeats
  |     '-- Report exit code to the relay control plane
  |-- Outputs (ledger, artifacts, console) stream to the collector throughout
  '-- Teardown
```

## Communication paths

The build environment talks only to the relay (guest-facing API, unchanged):

| Endpoint | Purpose |
|----------|---------|
| `http://artifacts/health` | Relay readiness probe |
| `http://artifacts/ca.pem` | Ephemeral CA certificate |
| `http://artifacts/build-script` | Build script |
| `http://cwd/<path>` | Context files |
| `http://artifacts/heartbeat` | Heartbeat |
| `http://artifacts/exit?code=<n>` | Exit code report |
| `http://artifacts/<name>` | Artifact upload |

The relay's control plane binds `:8300`; `artifacts` and `cwd` are reserved
hostnames that resolve to the relay. The relay in turn serves build inputs
from, and streams outputs to, the host collector over the `/v1/*` API (a
separate plane the build environment never sees).

External HTTPS traffic is transparently proxied (MITM) and recorded in the
ledger. External HTTP traffic is proxied and recorded without interception.

## Network isolation

See [Network Isolation](./network-isolation.md) for iptables rule details
(container driver). For the QEMU, VZ, and Hyper-V drivers, isolation is
topological: the build environment physically cannot reach anything except the
relay.
