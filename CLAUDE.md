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

BuildWarden orchestrates two containers on an isolated Docker network (100.64.87.0/29):

- **Relay** — MITM proxy + DNS + ledger writer. Runs `cmd/relay/`.
- **Build Container** — Unprivileged container from the Dockerfile's FROM image. Network-isolated via iptables applied by a one-shot sidecar (Kubernetes init container pattern). All traffic forced through relay.

The orchestrator (`cmd/warden/`) runs on the host and manages the lifecycle:
1. Pulls the FROM image, writes environment identity to the ledger volume
2. Starts the relay (reads environment files, records as first ledger entry)
3. Starts the build container, applies iptables via a netns sidecar
4. Translates the Dockerfile to a shell script, executes it via container exec

The Dockerfile is used as a concise build configuration format. Supported directives: FROM, RUN, ENV, WORKDIR, COPY, ARG. Unsupported directives produce clear errors.

### Package boundaries

- `cmd/warden/` — Host-side binary: CLI, orchestrator (container lifecycle, config, extensions, output), inspect, Dockerfile-to-script translation. Imports `ctrctl`.
- `cmd/warden-io/` — Container-side binary: fetch context files from relay, post artifacts.
- `cmd/relay/` — Container-side binary: proxy, ledger writer, DNS, TLS interception, fairness scheduling.
- `ledger/` — Shared library: ledger wire format types, reader, and verification logic.

### Ledger format

Binary, self-describing. Magic `BLDL`, version 0x01. Ed25519-SHA512 chained signatures. CBOR metadata (not signed). Record types: open (0x01), checkpoint (0x02), close (0x03), artifact (0x04). Spec at `docs/design/Ledger-Spec.md`.

### Single-writer pattern

All ledger writes go through a channel to a single goroutine (`Ledger.loop()`). Open is synchronous (returns signature as channel ID). Checkpoint/Close/Artifact are fire-and-forget.

## Key conventions

- Container runtime is abstracted via `ctrctl.Cli` (set from config/autodetection)
- Extensions inject CA certs and env vars into the build container via `.warden/` directory
- The relay and warden-io cross-compile for linux at build time (`GOOS=linux CGO_ENABLED=0`)
- Containerfile is never modified in-place — a copy goes into `.warden/Containerfile`
- `artifacts` and `cwd` are reserved DNS hostnames that resolve to the relay IP
- Network isolation: iptables applied by `warden-netns` sidecar sharing the build container's network namespace; build container has no CAP_NET_ADMIN

## Linter settings

golangci-lint with: gocyclo (min-complexity 15), lll (line-length 99), errname, forcetypeassert. See `.golangci.yml`.
