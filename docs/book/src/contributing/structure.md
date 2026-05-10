# Project Structure

```
buildwarden/
├── cmd/
│   ├── relay/          # Relay container binary
│   │   ├── main.go        # Entrypoint, wires up ledger + listeners
│   │   ├── relay.go       # DNS, HTTP proxy, artifact handling, context server
│   │   ├── proxy.go       # MITM TLS proxy, connection handling
│   │   ├── ledger.go      # Ledger writer (single-writer channel pattern)
│   │   ├── ledger_read.go # Re-exports shared types from ledger/
│   │   ├── fair.go        # Bandwidth fairness scheduler (DRR)
│   │   └── network.go     # Network utilities
│   └── warden/         # Host binary (CLI + orchestrator + inspect)
│       ├── main.go         # Root command, build, shell
│       ├── orchestrator.go # Container creation, teardown, COPY rewrite
│       ├── config.go       # Config loading (TOML), runtime detection
│       ├── output.go       # Colored terminal output
│       ├── build.go        # BuildConfig, BuildEnv interface
│       ├── ext.go              # Extension interface
│       ├── ext_truststore.go   # System CA trust store setup
│       ├── ext_jks_truststore.go # JKS keystore for JVM (maven, gradle, bazel)
│       ├── ext_cacerts_env.go  # CA env vars (npm, pip, uv, nix, gem, etc.)
│       ├── ext_epoch.go        # SOURCE_DATE_EPOCH
│       ├── cert.go         # Certificate subject hash computation
│       ├── clean.go        # warden clean
│       ├── inspect.go      # warden inspect (command definition)
│       └── inspect_impl.go # Inspect logic (verification, display)
├── ledger/             # Shared library — ledger wire format
│   └── ledger.go       # Types, reader, verifier
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

- **`cmd/warden/`** — Host-side binary. Imports `ctrctl`. Contains the CLI, orchestrator (container lifecycle, config, extensions, COPY rewriting), and inspect logic.
- **`cmd/relay/`** — Container-side binary. Proxy, ledger writer, DNS, TLS interception. Fully independent of cmd/warden.
- **`ledger/`** — The only shared code. Defines the binary ledger wire format types and provides read/verify logic used by `warden inspect` and the relay's test suite.

## Key Patterns

### Single-writer ledger

All ledger writes go through a channel to a single goroutine (`Ledger.loop()`). This ensures records are strictly ordered without locks.

### Extension system

Extensions implement a two-method interface (`BeforeBuild` + `Env`). They run after the relay starts (so the CA cert is available) and inject env vars and setup scripts into the Dockerfile.

### Dynamic subnet allocation

Each build probes for an unused /29 in `100.64.87.0/24`. This enables concurrent builds without network collisions.

### Transparent MITM

The relay generates per-host TLS certificates on the fly, signed by the ephemeral CA. Clients trust it because the CA is injected into the system trust store by the TrustStore extension.
