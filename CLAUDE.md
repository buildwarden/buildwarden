# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build Commands

```sh
make build     # Compile warden binary
make test      # Run unit tests
make cover     # Tests with coverage report
make lint      # golangci-lint
make fmt       # gofmt -s -w .
make tidy      # go mod tidy
```

Go version: 1.26.3 (managed via mise). Use `mise exec -- go ...` if GOROOT is misconfigured.

## Architecture

BuildWarden has four drivers (container, qemu, vz, hyperv) sharing a unified runtime model:

```
relay boots unconfigured → warden claims it (tunnel + config) → build guest boots → warden-io initialize → fetch build script → run with heartbeats → report exit
```

The relay keeps no build state on disk; it streams every output to, and pulls every input from, a host-side **collector** over a tunnel the orchestrator opens on the relay's claim port.

**Drivers:**
- **Container** (`--driver container`, default) — Relay as sidecar container, iptables isolation via netns sidecar, `warden-io initialize` exec'd in build container.
- **QEMU** (`--driver qemu`) — Two-VM topology (relay VM + build VM). Topological isolation. Works on macOS/Linux/Windows; Linux and Windows guests.
- **VZ** (`--driver vz`) — macOS/arm64 only. Relay as host process (gvisor netstack, FD ingress). macOS build VM with single socketpair interface.
- **Hyper-V** (`--driver hyperv`) — Windows. Two-VM topology on Hyper-V with a host-only NAT switch; Windows and Linux guests. One-time `warden setup` creates the durable switches.

**Key components:**
- **Relay** (`relay/` library, `cmd/relay/` binary) — MITM proxy, DNS, ledger writer, control plane, heartbeat. Three modes: container (bind ports), vm (PID 1 in Alpine VM), host (FD ingress + SSRF filter).
- **warden-io** (`cmd/warden-io/`) — Build-environment agent. Subcommands: initialize (full lifecycle), fetch (context files), post (artifacts), trust (CA install).
- **Orchestrator** (`cmd/warden/`) — CLI, driver selection, Dockerfile translation, extensions, inspect.

### Package boundaries

- `cmd/warden/` — Host-side binary: CLI, driver dispatch, config, inspect, image management, doctor/setup.
- `cmd/warden-io/` — Build-environment agent: initialize (network, CA, fetch script, exec with heartbeats, report exit), fetch, post, trust. Cross-compiled for linux, darwin, and windows.
- `cmd/relay/` — Thin mode-based main. Selects container/vm/host mode, calls `relay.Start()`.
- `cmd/collector/` — Standalone collector for third-party orchestrators (claims a running relay, serves inputs, lands outputs).
- `relay/` — Relay library: DNS, HTTP/HTTPS MITM, ledger writer, control plane, heartbeat, capture, fairness. Streams outputs to / pulls inputs from the collector (`collector_client.go`); no on-disk build state.
- `collector/` — The relay's single output path and input source. Run in-process by every driver (`driver.Session`) and standalone by `cmd/collector`.
- `relaycfg/` — Claim protocol: push the per-build config over a host-initiated tunnel, return the relay identity.
- `driver/` — Driver interface, shared host-side build logic (`session.go`), extensions, subnet allocation.
- `driver/container/` — Container driver: Dockerfile translation, iptables isolation, relay container.
- `driver/qemu/` — QEMU driver: two-VM topology, cloud images, cross-platform.
- `driver/vz/` — VZ driver: host relay (gvisor netstack), macOS build VM.
- `driver/hyperv/` — Hyper-V driver: two-VM topology, host-only NAT switch, host setup/doctor, privileged helper service.
- `driver/script/` — Dockerfile-to-script translator (shared by VM drivers).
- `ledger/` — Shared library: ledger wire format types, reader, verification logic.

### Ledger format

Binary, self-describing. Magic `BLDL`, version 0x01. Ed25519-SHA512 chained signatures. CBOR metadata (not signed). Record types: open (0x01), checkpoint (0x02), close (0x03), artifact (0x04). Spec at `docs/design/Ledger-Spec.md`.

### Single-writer pattern

All ledger writes go through a channel to a single goroutine (`Ledger.loop()`). Open is synchronous (returns signature as channel ID). Checkpoint/Close/Artifact are fire-and-forget.

## Key conventions

- Container runtime is abstracted via `ctrctl.Cli` (set from config/autodetection)
- Extensions inject CA certs and env vars into the build environment via `.warden/` directory
- The relay and warden-io cross-compile for linux at build time (`GOOS=linux CGO_ENABLED=0`); warden-io also builds for darwin/arm64 (VZ driver)
- Containerfile is never modified in-place — a copy goes into `.warden/Containerfile`
- `artifacts` and `cwd` are reserved DNS hostnames that resolve to the relay IP
- Network isolation: iptables (container), topological via separate VM (qemu, vz, hyperv)
- All drivers use `warden-io initialize` as the build-environment entry point

## Linter settings

golangci-lint with: gocyclo (min-complexity 15), lll (line-length 99), errname, forcetypeassert. See `.golangci.yml`.
