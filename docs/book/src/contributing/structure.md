# Project Structure

```
buildwarden/
├── cmd/
│   ├── relay/          # Relay container entrypoint
│   │   └── main.go
│   └── warden/         # CLI entrypoint + subcommands
│       ├── main.go     # Root command, build, shell
│       ├── clean.go    # warden clean
│       ├── inspect.go  # warden inspect (command definition)
│       └── inspect_impl.go  # Inspect logic (verification, display)
├── internal/
│   └── orchestrator/   # Host-side build lifecycle
│       ├── orchestrator.go  # Container creation, teardown, COPY rewrite
│       ├── config.go        # Config loading (TOML)
│       ├── output.go        # Colored terminal output
│       ├── build.go         # BuildConfig, BuildEnv interface
│       ├── ext.go           # Extension interface
│       ├── ext_truststore.go  # CA cert injection
│       ├── ext_pip.go       # pip cert config
│       ├── ext_bazel.go     # Bazel cert config
│       └── ext_epoch.go     # SOURCE_DATE_EPOCH
├── relay/              # Relay library (runs inside container)
│   ├── relay.go        # DNS, HTTP proxy, artifact handling, context server
│   ├── proxy.go        # MITM TLS proxy
│   ├── ledger.go       # Ledger writer (single-writer channel pattern)
│   ├── ledger_read.go  # Ledger parser/verifier
│   ├── cert.go         # Ephemeral CA generation
│   ├── fair.go         # Bandwidth fairness scheduler
│   └── network.go      # Network utilities
├── examples/           # Demo Dockerfiles
├── docs/
│   ├── book/           # mdBook documentation (this site)
│   └── design/         # Design specifications
├── Dockerfile          # Self-build (warden builds itself)
├── Dockerfile.relay    # Multi-arch relay image
├── Makefile
├── test-integration.sh
└── .github/workflows/
    ├── main.yml        # CI (lint + test + build)
    ├── release.yml     # GoReleaser on tag push
    ├── relay-image.yml # Multi-arch relay image publish
    └── auto-tidy.yml   # Weekly go mod tidy
```

## Package Boundaries

- **`internal/orchestrator/`** — Host-side only. Imports `ctrctl`. Manages container lifecycle, config, extensions, COPY rewriting.
- **`relay/`** — Container-side only. Could be used as a library. Does NOT import orchestrator.
- **`cmd/warden/`** — CLI. Wires orchestrator + cobra. Also contains inspect logic.
- **`cmd/relay/`** — Minimal entrypoint for the relay container binary.

## Key Patterns

### Single-writer ledger

All ledger writes go through a channel to a single goroutine (`Ledger.loop()`). This ensures records are strictly ordered without locks.

### Extension system

Extensions implement a two-method interface (`BeforeBuild` + `Env`). They run after the relay starts (so the CA cert is available) and inject env vars and setup scripts into the Dockerfile.

### Dynamic subnet allocation

Each build probes for an unused /29 in `100.64.87.0/24`. This enables concurrent builds without network collisions.

### Transparent MITM

The relay generates per-host TLS certificates on the fly, signed by the ephemeral CA. Clients trust it because the CA is injected into the system trust store by the TrustStore extension.
