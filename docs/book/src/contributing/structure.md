# Project Structure

```
buildwarden/
├── cmd/
│   ├── relay/              # Relay binary (thin mode-based main)
│   │   ├── main.go            # CLI flags, mode detection, parseSubnet
│   │   ├── mode_host.go       # Host mode: FD ingress via gvisor netstack
│   │   ├── chanlistener.go    # Channel-based net.Listener (for host mode)
│   │   ├── ingress_fd.go      # gvisor netstack: Ethernet frames → TCP/UDP
│   │   └── ingress_dhcp.go    # DHCP server for FD ingress mode
│   ├── warden/             # Host binary (CLI + driver dispatch)
│   │   ├── main.go            # Cobra commands: build, shell
│   │   ├── config.go          # Config loading (TOML), runtime detection, validation
│   │   ├── host.go            # warden doctor, setup, hyperv service
│   │   ├── image.go           # warden image prepare / list (all drivers)
│   │   ├── inspect.go         # warden inspect (subcommand registration)
│   │   ├── inspect_impl.go    # warden inspect (implementation)
│   │   ├── clean.go           # warden clean
│   │   └── output.go          # Colored terminal output
│   ├── collector/          # Standalone collector (third-party orchestrators)
│   │   └── main.go            # Claim a running relay, serve inputs, land outputs
│   ├── warden-io/          # Build-environment agent binary
│   │   ├── main.go            # Subcommands: initialize, fetch, post, trust
│   │   ├── initialize.go      # Full init: network, CA, fetch script, exec, heartbeats
│   │   ├── platform_darwin.go # macOS: keychain CA install, network config
│   │   ├── platform_linux.go  # Linux: update-ca-certificates, resolv.conf
│   │   └── platform_windows.go # Windows: cert store, network config
│   └── warden-vmnet/       # macOS vmnet.framework helper (C, for future use)
│       └── main.c
├── relay/                  # Relay library (core logic)
│   ├── relay.go               # Config, Start(), MITM proxy, input/artifact handlers
│   ├── collector_client.go    # Streams outputs to / pulls inputs from the collector
│   ├── dns.go                 # DNS server (UDP, forwards upstream)
│   ├── proxy.go               # HTTP/HTTPS MITM, SSRF filter, TLS cert generation
│   ├── ledger.go              # Ledger writer (single-writer channel pattern)
│   ├── heartbeat.go           # Forwards guest heartbeats to the collector; completion
│   ├── controlplane.go        # Control plane HTTP (/health, /ca.pem, /v1/output, /v1/complete)
│   ├── capture.go             # Payload capture (streamed to the collector)
│   └── fair.go                # Fair scheduling for concurrent connections
├── collector/              # Collector library (relay's single output path + input source)
│   ├── collector.go           # HTTP handler: artifacts, captures, ledger, output, context
│   ├── signals.go             # ready / heartbeat / complete / status signalling
│   └── paths.go               # Shared artifact/context path safety rules
├── relaycfg/               # Claim protocol between orchestrator and relay
│   ├── relaycfg.go            # Serve/Claim: push config, return relay identity
│   └── tunnel.go              # Host-initiated tunnel carrying the collector (HTTP/2)
├── driver/                 # Driver interface + shared host-side build logic
│   ├── driver.go              # Driver interface, BuildRequest, BuildResult
│   ├── session.go             # Shared Session: collector, claim, wait, output finalize
│   ├── compress.go            # Zstd compression of ledger + CA cert (all drivers)
│   ├── hostutil.go            # Host-side helpers
│   ├── extension.go           # Extension interface
│   ├── extensions.go          # DefaultExtensions()
│   ├── ext_cacerts.go         # CA cert env vars extension
│   ├── ext_bazel.go           # Bazel/JKS truststore extension
│   ├── ext_epoch.go           # SOURCE_DATE_EPOCH extension
│   ├── ext_truststore.go      # System trust store extension
│   ├── subnet.go              # Subnet allocation, utilities
│   ├── container/             # Container driver (iptables isolation)
│   │   ├── container.go          # StartBuild/Exec: relay container, build container, netns
│   │   ├── containerfile.go      # Containerfile rewriting (COPY → warden-io fetch)
│   │   ├── dockerfile.go         # Dockerfile-to-shell-script translator
│   │   └── environment.go        # OCI manifest extraction, environment record
│   ├── qemu/                  # QEMU driver (cross-platform VM)
│   │   ├── driver.go             # StartBuild: prepare, boot relay VM, boot build VM, wait
│   │   ├── vm.go                 # QEMU subprocess management (relay VM + build VM args)
│   │   ├── initrd.go             # Append the relay binary to the prebuilt initramfs
│   │   ├── cloudinit.go          # Cloud-init user-data generation
│   │   ├── seediso.go            # ISO9660 seed image builder
│   │   ├── images.go             # Cloud image resolution + download + caching
│   │   ├── windows.go            # Windows guest support
│   │   ├── winacq.go             # Windows media acquisition + unattended install
│   │   └── autounattend.go       # Autounattend.xml generation
│   ├── vz/                    # VZ driver (macOS Apple Silicon)
│   │   ├── driver.go             # StartBuild: host relay, boot macOS VM, wait
│   │   ├── image.go              # IPSW restore, image preparation, personalization
│   │   ├── network.go            # Socketpair networking for build VM
│   │   ├── vm.go                 # VM lifecycle helpers
│   │   ├── vz_darwin_arm64.go    # Virtualization.framework CGo bindings
│   │   ├── vz_darwin.h           # Objective-C header
│   │   ├── vz_darwin.m           # Objective-C implementation
│   │   └── vz_stub.go            # Build stub for non-macOS platforms
│   ├── hyperv/                # Hyper-V driver (Windows)
│   │   ├── driver.go             # StartBuild: boot relay VM + build VM, claim, wait
│   │   ├── setup.go              # warden setup: durable switches + NAT
│   │   ├── doctor.go             # warden doctor preflight
│   │   ├── service.go            # Privileged helper service (network standup)
│   │   ├── evalimage.go          # Windows Server eval VHD fetch + pin
│   │   └── relayvm.go            # Relay VM lifecycle
│   └── script/                # Dockerfile-to-shell-script translator (VM drivers)
│       └── translate.go
├── ledger/                 # Shared library: wire format, reader, verifier
│   └── ledger.go
├── tools/
│   ├── relay-vm/              # Relay VM assets (Alpine + kernel)
│   │   ├── build-initramfs.sh    # Build the prebuilt relay initramfs
│   │   ├── init                  # ONE relay VM init script (qemu + hyperv)
│   │   └── hyperv/               # Hyper-V UKI boot-disk build (/sbin/relay baked in)
│   │       └── build.sh
│   ├── build-vm/             # Build VM initramfs builder (direct-boot testing)
│   │   ├── build-initramfs.sh
│   │   └── init                  # Build VM init script (exec warden-io initialize)
│   ├── vz-image-prep/        # Standalone VZ image preparation tools
│   │   ├── personalize.go        # 4-phase macOS image prep pipeline
│   │   ├── netfwd.go             # Transparent TCP/UDP forwarder (image prep)
│   │   └── uv-build-test.go      # End-to-end build validation
│   └── win-image-prep/       # Windows image preparation notes/tools
│       └── README.md
├── docs/
│   ├── book/               # mdBook documentation (guide, concepts, contributing)
│   └── design/             # Design specifications and plans
├── Dockerfile.relay        # Multi-arch relay container image
├── Makefile                # build, test, lint, codesign
├── .goreleaser.yaml        # Release configuration
└── CLAUDE.md               # AI assistant context
```

## Package Boundaries

- **`cmd/warden/`** — Host-side binary. CLI, driver dispatch, config, inspect, image management, doctor/setup. Dispatches to `driver/container`, `driver/qemu`, `driver/vz`, or `driver/hyperv` based on `--driver` flag.
- **`cmd/warden-io/`** — Build-environment agent. Runs inside the build environment (container or VM). Handles initialization (CA install, script fetch), context fetch, artifact post. Cross-compiled for linux (containers, QEMU VMs), darwin (VZ macOS VMs), and windows.
- **`cmd/relay/`** — Relay binary. Thin mode-based main that selects container/vm/host mode and calls into the `relay/` library. Cross-compiled for linux (container/VM relay) or built natively (host-mode relay for VZ).
- **`cmd/collector/`** — Standalone collector for third-party orchestrators: claims an already-running relay, serves inputs, lands outputs, exits with the build result.
- **`relay/`** — Relay library. All core logic: DNS, HTTP/HTTPS MITM proxy, ledger writer, control plane, heartbeat, fairness scheduling, capture. Keeps no build state on disk; streams every output to, and pulls every input from, the collector. Used by `cmd/relay/` in all modes.
- **`collector/`** — The host side of the relay's single output path and the source of its build inputs (artifacts, captures, ledger, build output, build script, context). Run in-process by every driver (`driver.Session`) and standalone by `cmd/collector`.
- **`relaycfg/`** — The claim protocol: a relay boots unconfigured, the orchestrator pushes the per-build config over a host-initiated tunnel and receives the relay's identity (MITM CA cert + ledger key).
- **`driver/`** — Driver interface, shared host-side build logic (`Session`: collector, claim, wait, output finalize), and implementations. Each driver implements `driver.Driver` (Name, StartBuild, Exec, Close). Extensions modify the build environment before execution.
- **`driver/container/`** — Container driver. Full Dockerfile translation, iptables isolation via netns sidecar, relay as container.
- **`driver/qemu/`** — QEMU driver. Two-VM topology with cloud images, cross-platform; Linux and Windows guests.
- **`driver/vz/`** — VZ driver. Host relay with gvisor netstack, macOS build VM.
- **`driver/hyperv/`** — Hyper-V driver. Two-VM topology with a host-only NAT switch; Windows and Linux guests. Includes host setup, doctor, and the privileged helper service.
- **`ledger/`** — Shared wire format library. Binary ledger types, reader, verifier. Used by `warden inspect` and the relay's test suite.

## Key Patterns

### Unified runtime model

All drivers follow the same execution pattern:
1. The orchestrator starts a collector and boots the relay unconfigured
2. The orchestrator claims the relay (tunnel + build config), receiving its CA cert + ledger key
3. `warden-io initialize` runs inside the build environment
4. warden-io fetches the build script from the relay's `build-script` endpoint
5. Build runs with periodic heartbeats; the relay forwards heartbeats and streams outputs (artifacts, captures, ledger, console) to the collector
6. Exit code reported to the relay, which flushes the ledger and posts `/v1/complete`

### Single-writer ledger

All ledger writes go through a channel to a single goroutine (`Ledger.loop()`). Records are strictly ordered without locks.

### Relay modes

The relay binary detects its mode at startup:
- **Container mode**: binds :53, :80, :443, :8300 directly (isolation via iptables)
- **VM mode**: same as container but relay is PID 1 in its own Alpine VM
- **Host mode**: receives connections via gvisor netstack on an inherited FD (no port binding on host); SSRF filtering active

### Extension system

Extensions implement `BeforeBuild(ctx) + Env() map[string]string`. They inject CA certs, env vars, and setup scripts into the build environment.

### Dynamic subnet allocation

Container driver allocates /29 subnets from 100.64.87.0/24 (CGNAT range). VM drivers use fixed 10.0.0.0/30 topology.

### Transparent MITM

The relay generates per-host TLS certificates on the fly, signed by an ephemeral CA. The CA is installed into the build environment's trust store by warden-io during initialization.
