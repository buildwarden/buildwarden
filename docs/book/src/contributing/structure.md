# Project Structure

```
buildwarden/
├── cmd/
│   ├── relay/              # Relay binary (thin mode-based main)
│   │   ├── main.go            # CLI flags, mode detection, parseSubnet
│   │   ├── mode_container.go  # Container mode: binds ports directly
│   │   ├── mode_vm.go         # VM mode: binds ports (relay is PID 1 in its VM)
│   │   ├── mode_host.go       # Host mode: FD ingress via gvisor netstack
│   │   ├── chanlistener.go    # Channel-based net.Listener (for host mode)
│   │   └── ingress_fd.go      # gvisor netstack: Ethernet frames → TCP/UDP
│   ├── warden/             # Host binary (CLI + orchestrators)
│   │   ├── main.go            # Cobra commands: build, shell, clean, inspect, image
│   │   ├── driver_script.go   # Container driver orchestrator (ScriptEnv)
│   │   ├── config.go          # Config loading (TOML), runtime detection
│   │   ├── output.go          # Colored terminal output
│   │   ├── inspect.go         # warden inspect
│   │   ├── clean.go           # warden clean
│   │   └── image.go           # warden image (VZ image management)
│   ├── warden-io/          # Build-environment agent binary
│   │   ├── main.go            # Subcommands: initialize, fetch, post, trust
│   │   ├── initialize.go      # Full init: network, CA, fetch script, exec, heartbeats
│   │   ├── platform_darwin.go # macOS: keychain CA install, network config
│   │   └── platform_linux.go  # Linux: update-ca-certificates, resolv.conf
│   └── warden-vmnet/       # macOS vmnet.framework helper (C, for future use)
│       └── main.c
├── relay/                  # Relay library (core logic)
│   ├── relay.go               # Config, Start(), MITM proxy, context/artifact handlers
│   ├── dns.go                 # DNS server (UDP, forwards upstream)
│   ├── proxy.go               # HTTP/HTTPS listener setup, TLS config
│   ├── ledger.go              # Ledger writer (single-writer channel pattern)
│   ├── heartbeat.go           # Signal-dir heartbeat writer
│   ├── controlplane.go        # Control plane HTTP (/health, /ca.pem, /v1/output, /v1/complete)
│   ├── capture.go             # Payload capture to disk
│   └── fair_test.go, e2e_bench_test.go, ledger_test.go, ledger_read_test.go
├── driver/                 # Driver interface + implementations
│   ├── driver.go              # Driver interface, BuildRequest, BuildResult
│   ├── extension.go           # Extension interface
│   ├── extensions.go          # Extension registration
│   ├── ext_cacerts.go         # CA cert env vars extension
│   ├── ext_bazel.go           # Bazel extension
│   ├── ext_epoch.go           # SOURCE_DATE_EPOCH extension
│   ├── ext_truststore.go      # System trust store extension
│   ├── subnet.go              # Subnet allocation
│   ├── container/             # Container driver (driver.Driver implementation)
│   │   └── container.go
│   ├── qemu/                  # QEMU driver
│   │   ├── driver.go             # StartBuild: prepare, boot relay VM, boot build VM, wait
│   │   ├── vm.go                 # QEMU subprocess management (relay VM + build VM args)
│   │   ├── cloudinit.go          # Cloud-init seed ISO generation (NoCloud)
│   │   ├── seediso.go            # ISO9660 image builder (pure Go + platform tools)
│   │   ├── images.go             # Cloud image resolution + download + caching
│   │   ├── signal.go             # Host-side waitForBuild (reads signal dir)
│   │   └── util.go               # File copy, binary caching, module root
│   ├── vz/                    # VZ driver (macOS Apple Silicon)
│   │   ├── driver.go             # StartBuild: host relay, boot macOS VM, wait
│   │   ├── image.go              # IPSW restore, image preparation, personalization
│   │   ├── signal.go             # WaitForBuild (reads signal dir)
│   │   ├── signal_test.go
│   │   └── vz_darwin_arm64.go    # Virtualization.framework bindings (CGo)
│   ├── script/                # Dockerfile-to-shell-script translator
│   └── security/              # Security utilities
├── ledger/                 # Shared library: wire format, reader, verifier
│   └── ledger.go
├── tools/
│   ├── relay-vm/              # Relay VM initramfs builder (Alpine + kernel modules)
│   │   ├── build-initramfs.sh
│   │   └── init                  # Relay VM init script (network, iptables, exec relay)
│   └── build-vm/             # Build VM initramfs builder (for direct-boot testing)
│       ├── build-initramfs.sh    # Cross-compiles warden-io into initramfs
│       └── init                  # Build VM init script (network, exec warden-io initialize)
├── examples/               # Demo Dockerfiles (apk, apt, pip, cargo, etc.)
├── docs/
│   ├── book/               # mdBook documentation
│   └── design/             # Design specifications and plans
├── Dockerfile              # Self-build (warden builds itself)
├── Dockerfile.relay        # Multi-arch relay container image
├── Makefile                # build, test, lint, codesign
└── CLAUDE.md               # AI assistant context
```

## Package Boundaries

- **`cmd/warden/`** — Host-side binary. CLI, orchestrators (container lifecycle, VM boot), config, extensions, inspect, image management. Imports `ctrctl` for container operations.
- **`cmd/warden-io/`** — Build-environment agent. Runs inside the build environment (container or VM). Handles initialization (CA install, script fetch), context fetch, artifact post. Cross-compiled for linux (containers, QEMU VMs) and darwin (VZ macOS VMs).
- **`cmd/relay/`** — Relay binary. Thin mode-based main that selects container/vm/host mode and calls into the `relay/` library. Cross-compiled for linux (container/VM relay) or built natively (host-mode relay for VZ).
- **`relay/`** — Relay library. All core logic: DNS, HTTP/HTTPS MITM proxy, ledger writer, control plane, heartbeat, fairness scheduling, capture. Used by `cmd/relay/` in all modes.
- **`driver/`** — Driver interface and implementations. Each driver implements `driver.Driver` (Name, StartBuild, Exec, Close).
- **`ledger/`** — Shared wire format library. Binary ledger types, reader, verifier. Used by `warden inspect` and the relay's test suite.

## Key Patterns

### Unified runtime model

All drivers follow the same execution pattern:
1. Relay starts (in a container, VM, or as a host process)
2. `warden-io initialize` runs inside the build environment
3. warden-io fetches the build script from the relay's context endpoint
4. Build runs with periodic heartbeats reported to the relay
5. Exit code reported to relay; relay writes signal files for host

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
