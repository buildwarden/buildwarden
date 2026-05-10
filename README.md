# BuildWarden

[![CI](https://github.com/buildwarden/buildwarden/actions/workflows/main.yml/badge.svg)](https://github.com/buildwarden/buildwarden/actions)
[![Go Report Card](https://goreportcard.com/badge/github.com/buildwarden/buildwarden)](https://goreportcard.com/report/github.com/buildwarden/buildwarden)
[![Release](https://img.shields.io/github/v/release/buildwarden/buildwarden)](https://github.com/buildwarden/buildwarden/releases/latest)
[![License](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](LICENSE)

BuildWarden produces a cryptographically-signed, tamper-evident ledger of **every input** to a software build — network fetches, source files, base images, and build artifacts. It enables independent verification of build integrity without trusting the build author.

## Quick Start

### Install

```sh
curl -sSfL https://raw.githubusercontent.com/buildwarden/buildwarden/main/install.sh | sh
```

Or download from [Releases](https://github.com/buildwarden/buildwarden/releases), or build from source:

```sh
make build
```

Requires a container runtime: [Finch](https://github.com/runfinch/finch), Docker, or Podman.

### Run a build

```sh
warden build
```

That's it. BuildWarden finds the Dockerfile in the current directory, runs the build in a network-audited container, and writes results to `warden-output/`:

```
warden-output/
├── ledger.zst              # Cryptographic ledger (zstd compressed)
├── Dockerfile.submitted    # What you wrote
├── Dockerfile.actual       # What actually ran (with warden injections)
├── ca.cert.pem.zst         # Ephemeral CA used for TLS interception
├── relay.log               # Relay DNS/HTTP/artifact event log
└── artifacts/              # Build outputs posted to the ledger
    └── myapp-1.0.0.whl
```

### Inspect the ledger

```sh
warden inspect warden-output
```

```
════════════════════════════════════════════════════════════════════════════════
  LEDGER  scheme=ed25519-sha512  hashes=[blake2b_256 sha256 sha1 md5]  ✅
════════════════════════════════════════════════════════════════════════════════
✅ GET https://registry-1.docker.io/v2/library/python/manifests/3.12-slim (10373 bytes)
✅ GET http://deb.debian.org/debian/dists/bookworm/main/binary-arm64/... (8690780 bytes)
✅ GET http://cwd/requirements.txt (42 bytes)
✅ GET https://files.pythonhosted.org/packages/.../requests-2.32.3.tar.gz (131218 bytes)
✅ ARTIFACT POST http://artifacts/requests-2.32.3-py3-none-any.whl (65027 bytes)

════════════════════════════════════════════════════════════════════════════════
  SUMMARY: 255 records, 64 requests, 59.8 MB audited, 1 artifact(s)
  SIGNATURES: ✅ All 256 signatures valid
  COMPLETENESS: ✅ All channels closed
════════════════════════════════════════════════════════════════════════════════
```

Every input is individually hashed and signed into the ledger — source files (via `http://cwd/`), network fetches, container images, and posted artifacts.

## How It Works

BuildWarden orchestrates two containers on an isolated network:

```
┌──────────────────────────────────────────────────────────────┐
│  Host                                                        │
│                                                              │
│  ┌──────────────┐           ┌──────────────────────────┐    │
│  │    Relay     │◄──────────│     Build Container      │    │
│  │              │           │                          │    │
│  │  • DNS       │   only    │  • Rootless DinD         │    │
│  │  • HTTP/S    │◄──conn───►│  • iptables isolated     │    │
│  │  • Ledger    │           │  • Source via relay      │    │
│  │  • Context   │           │                          │    │
│  └──────┬───────┘           └──────────────────────────┘    │
│         │                                                    │
│         ▼ external                                           │
│    ┌──────────┐                                              │
│    │ Internet │                                              │
│    └──────────┘                                              │
└──────────────────────────────────────────────────────────────┘
```

- **Relay** — MITM proxy intercepting all traffic. Records every byte to the ledger. Serves source files from the build context. Ephemeral CA per-build (destroyed on teardown).
- **Build Container** — Rootless Docker-in-Docker. Network-isolated: all HTTP/HTTPS is DNAT'd to the relay, all other traffic is dropped. Source files enter exclusively through the relay for full provenance.
- **Dynamic subnets** — Each build allocates a unique /29 from `100.64.87.0/24`, enabling concurrent builds on the same host.

## Commands

```sh
warden build [path]           # Run a build with full auditing
warden build -o ./my-output   # Custom output directory
warden build --no-compress    # Skip zstd compression of ledger
warden inspect <path>         # Verify and display a ledger
warden inspect --json <path>  # Machine-readable output
warden shell [path]           # Interactive shell in audited env
warden clean                  # Remove orphaned containers/networks
```

## Configuration

```toml
# ~/.config/warden/config.toml or ./warden.toml

[runtime]
cli = "finch"                # container runtime (finch, docker, podman)
relay_image = ""             # relay image override ("dev" = build from source)

[build]
output_dir = "warden-output" # where results are written
compress = true              # zstd compression of ledger and payloads
capture = ""                 # payload capture (none, headers, bodies, all)

[output]
color = "auto"               # auto, always, never
verbose = false
```

Environment overrides: `WARDEN_CTR_CLI`, `NO_COLOR`, `WARDEN_VERBOSE`.

## Examples

| Dockerfile | What it builds | Time |
|-----------|---------------|------|
| `examples/Dockerfile.simple` | Python `requests` wheel from sdist | ~1 min |
| `examples/Dockerfile.expanded` | Same + tests + type-check (ledger truncatability) | ~3 min |
| `examples/Dockerfile.cryptography` | Python `cryptography` (Rust/Cargo + pip + apt) | ~3 min |
| `examples/Dockerfile.pytorch-aarch64` | PyTorch from source, CUDA, aarch64 | ~3 hrs |

```sh
warden build examples/Dockerfile.simple
warden inspect warden-output
```

## Design

- [Ledger Specification](docs/design/Ledger-Spec.md)
- [Philosophy](docs/design/Philosophy.md)
- [Initial Proposal](docs/design/Initial-Proposal.md)

## Development

See [DEVELOPING.md](DEVELOPING.md) for build instructions and project structure.

## License

Apache-2.0
