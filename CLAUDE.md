# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build Commands

```sh
make build     # Compile warden + ledger-inspect
make test      # Run unit tests
make cover     # Tests with coverage report
make lint      # golangci-lint
make fmt       # gofmt -s -w .
make tidy      # go mod tidy
```

Go version: 1.26.3 (managed via mise). Use `mise exec -- go ...` if GOROOT is misconfigured.

## Architecture

BuildWarden orchestrates two containers on an isolated Docker network (10.0.87.0/29):

- **Relay** (10.0.87.2) — MITM proxy + DNS + ledger writer. Runs `cmd/relay/`.
- **Build Container** (10.0.87.3) — Rootless DinD, network-isolated via iptables. All traffic forced through relay.

The orchestrator (`internal/orchestrator/`) runs on the host and manages the lifecycle. The relay (`relay/`) runs inside a container and writes the binary ledger.

### Package boundaries

- `internal/orchestrator/` — Host-side: container lifecycle, config, extensions, output. Imports `ctrctl`.
- `relay/` — Container-side: proxy, ledger, DNS, certs. Could be used as a library. Does NOT import orchestrator.
- `cmd/ledger-inspect/` — Standalone verifier. Imports `relay/` only for read types.

### Ledger format

Binary, self-describing. Magic `BLDL`, version 0x01. Ed25519-SHA512 chained signatures. CBOR metadata (not signed). Record types: open (0x01), checkpoint (0x02), close (0x03), artifact (0x04). Spec at `docs/design/Ledger-Spec.md`.

### Single-writer pattern

All ledger writes go through a channel to a single goroutine (`Ledger.loop()`). Open is synchronous (returns signature as channel ID). Checkpoint/Close/Artifact are fire-and-forget.

## Key conventions

- Container runtime is abstracted via `ctrctl.Cli` (set from config/autodetection)
- Extensions inject CA certs and env vars into the build container via `.warden/` directory
- The relay cross-compiles for linux at build time (`GOOS=linux CGO_ENABLED=0`)
- Containerfile is never modified in-place — a copy goes into `.warden/Containerfile`
- `artifacts` is a reserved DNS hostname that resolves to the relay IP

## Linter settings

golangci-lint with: gocyclo (min-complexity 15), lll (line-length 99), errname, forcetypeassert. See `.golangci.yml`.
