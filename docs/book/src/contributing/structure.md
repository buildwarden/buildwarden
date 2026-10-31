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
│   │   ├── initialize.go      # Full init: network, CA + bootstrap config, baseline, script, heartbeats
│   │   ├── baseline.go        # Always-on baseline: Java truststore, bazelrc, CA and default env
│   │   ├── jks.go             # Minimal JKS writer (matches keytool byte for byte)
│   │   ├── trust_linux.go     # Exclusive system trust, OpenSSL subject hash
│   │   ├── platform_darwin.go # macOS: keychain CA install, network config
│   │   ├── platform_linux.go  # Linux: CA install, resolv.conf
│   │   └── platform_windows.go # Windows: cert store, network config
│   ├── warden-pause/       # Container driver's namespace holder (blocks until signalled)
│   │   └── main.go
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
├── bootstrap/              # The bootstrap config the executor fetches (trust policy)
│   └── bootstrap.go
├── collector/              # Collector library (relay's single output path + input source)
│   ├── collector.go           # HTTP handler: artifacts, captures, ledger, output, inputs
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
│   ├── subnet.go              # Subnet allocation, utilities
│   ├── container/             # Container driver (iptables isolation)
│   │   ├── container.go          # StartBuild/Exec: holder, sidecar, create, copy, start
│   │   ├── images.go             # Holder + sidecar images: by digest, or built for dev
│   │   ├── images/               # holder.Containerfile, netns.Containerfile
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
│   └── script/                # Dockerfile-to-shell-script translator (every driver)
│       └── translate.go          # COPY expanded against the context on the host
├── ledger/                 # Shared library: wire format, reader, verifier
│   └── ledger.go
├── specs/                  # Ledger format specs, versioned (RELAY-05); published in the book
│   ├── README.md             # Index: layout, $id -> file table, $id policy
│   ├── build-ledger/v1.md    # Wire format (prose only)
│   ├── metadata-<name>/      # A record metadata schema: v<N>.md beside v<N>.schema.json
│   ├── environment-<type>/   # A signed environment payload: v<N>.md beside v<N>.schema.json
│   └── specs_test.go         # Tier-0: relay header, schemas and book mirror agree
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
│   ├── win-image-prep/       # Windows image preparation notes/tools
│   │   └── README.md
│   ├── tier1/                # Tier-1 static checks, run by make lint (own Go module; LINT-02)
│   │   ├── cmd/tier1/            # Entry point: flags, stages, exit codes
│   │   ├── diff/                 # Base, changed paths and lines, Tier1-Allow trailers
│   │   ├── rules/                # Rule index from docs/development/, rules.yaml self-check
│   │   ├── coupling/             # Path-set checks; design front-matter (FLOW-02)
│   │   ├── surface/              # Cobra flags and toml keys at base and HEAD
│   │   ├── analyzers/            # go/analysis passes for the relay and the executor
│   │   ├── golangci/             # Runs golangci.yml on changed lines, maps issues to rules
│   │   ├── rules.yaml            # Every automated rule: check, owner or follow-up
│   │   └── report.schema.json    # The -json output
│   └── workflow/             # Feature-loop tooling (own Go module; FLOW-03, FLOW-18)
│       ├── cmd/wf/               # Queue tool: next step, leases, done-when checks, native gate
│       ├── native/               # Scheduled native sessions: wrapper, installers, permissions
│       └── flow18-check.sh       # Sessions stay out of the public repo
├── docs/
│   ├── book/               # mdBook documentation (guide, concepts, contributing)
│   ├── design/             # Design documents: the archive, plus TEMPLATE.md
│   └── development/        # Development standards and process (the rulebook)
├── Dockerfile.relay        # Multi-arch relay container image
├── Makefile                # build, test, lint, codesign
├── .goreleaser.yaml        # Release configuration
├── AGENTS.md               # Agent context: a map into docs/development and the book
└── CLAUDE.md               # Imports AGENTS.md for Claude Code
```

## Package Boundaries

- **`cmd/warden/`** — Host-side binary. CLI, driver dispatch, config, inspect, image management, doctor/setup. Dispatches to `driver/container`, `driver/qemu`, `driver/vz`, or `driver/hyperv` based on `--driver` flag.
- **`cmd/warden-io/`** — Build-environment agent, and the only thing the host puts in the build environment. Bootstraps itself from the relay (CA and trust policy, the baseline, the build script), runs the build, fetches context files, posts artifacts. Cross-compiled for linux (containers, QEMU VMs), darwin (VZ macOS VMs), and windows (QEMU and Hyper-V VMs).
- **`cmd/warden-pause/`** — The container driver's namespace holder: owns the build's network namespace while it is isolated, and does nothing else.
- **`cmd/relay/`** — Relay binary. Thin mode-based main that selects container/vm/host mode and calls into the `relay/` library. Cross-compiled for linux (container/VM relay) or built natively (host-mode relay for VZ).
- **`cmd/collector/`** — Standalone collector for third-party orchestrators: claims an already-running relay, serves inputs, lands outputs, exits with the build result.
- **`relay/`** — Relay library. All core logic: DNS, HTTP/HTTPS MITM proxy, ledger writer, control plane, heartbeat, fairness scheduling, capture. Keeps no build state on disk; streams every output to, and pulls every input from, the collector. Used by `cmd/relay/` in all modes.
- **`collector/`** — The host side of the relay's single output path and the source of its build inputs (artifacts, captures, ledger, build output, build script, bootstrap config, context).
- **`bootstrap/`** — The bootstrap config: the declarative trust policy the executor fetches from the relay (BOOT-05). Policy only; no paths, contents or commands. Run in-process by every driver (`driver.Session`) and standalone by `cmd/collector`.
- **`relaycfg/`** — The claim protocol: a relay boots unconfigured, the orchestrator pushes the per-build config over a host-initiated tunnel and receives the relay's identity (MITM CA cert + ledger key).
- **`driver/`** — Driver interface, shared host-side build logic (`Session`: collector, claim, wait, output finalize), and implementations. Each driver implements `driver.Driver` (Name, StartBuild, Exec, Close).
- **`driver/container/`** — Container driver. Namespace holder isolated by the iptables sidecar before the build container exists; build container created with `warden-io initialize` as its entrypoint; relay as container.
- **`driver/qemu/`** — QEMU driver. Two-VM topology with cloud images, cross-platform; Linux and Windows guests.
- **`driver/vz/`** — VZ driver. Host relay with gvisor netstack, macOS build VM.
- **`driver/hyperv/`** — Hyper-V driver. Two-VM topology with a host-only NAT switch; Windows guests (a Linux guest seed is not yet implemented). Includes host setup, doctor, and the privileged helper service.
- **`ledger/`** — Shared wire format library. Binary ledger types, reader, verifier. Used by `warden inspect` and the relay's test suite.
- **`specs/`** — The ledger format's documents, not code: one wire-format spec per ledger major version and each schema's document beside its machine schema (RELAY-05). `docs/book/src/specs/` mirrors it with one-line includes. Its only Go file is a test.

## Key Patterns

### Runtime model

Every driver follows one sequence: start a collector, boot the relay unconfigured,
claim it, isolate the build environment, deliver and start `warden-io`, and stream
everything back through the relay. The step-by-step pseudocode is in
[How It Works](../concepts/overview.md), and the bootstrap rules every driver
follows are in
[`docs/development/bootstrap.md`](https://github.com/buildwarden/buildwarden/blob/main/docs/development/bootstrap.md).

### Single-writer ledger

All ledger writes go through one goroutine (`Ledger.loop()`), so records are
strictly ordered without locks. The rule is RELAY-01 in
[`docs/development/relay.md`](https://github.com/buildwarden/buildwarden/blob/main/docs/development/relay.md).

### Relay modes

The relay binary detects its mode at startup:
- **Container mode**: binds :53, :80, :443, :8300 directly (isolation via iptables)
- **VM mode**: same as container but relay is PID 1 in its own Alpine VM
- **Host mode**: receives connections via gvisor netstack on an inherited FD (no port binding on host); SSRF filtering active

### Trust and the baseline

`warden-io initialize` installs the relay CA and a fixed baseline itself, from the
relay; support for a new tool goes in `cmd/warden-io/baseline.go` so it reaches
every driver. See [Trust and the Build Baseline](../concepts/baseline.md).

### Dynamic subnet allocation

Container driver allocates /29 subnets from 100.64.87.0/24 (CGNAT range). VM drivers use fixed 10.0.0.0/30 topology.

### Transparent MITM

The relay generates per-host TLS certificates on the fly, signed by an ephemeral CA. The CA is installed into the build environment's trust store by warden-io during initialization.
