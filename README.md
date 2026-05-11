<p align="center">
  <img src="logo.png" alt="BuildWarden" width="200">
</p>

# BuildWarden

[![CI](https://github.com/buildwarden/buildwarden/actions/workflows/main.yml/badge.svg)](https://github.com/buildwarden/buildwarden/actions)
[![Go Report Card](https://goreportcard.com/badge/github.com/buildwarden/buildwarden)](https://goreportcard.com/report/github.com/buildwarden/buildwarden)
[![Release](https://img.shields.io/github/v/release/buildwarden/buildwarden)](https://github.com/buildwarden/buildwarden/releases/latest)
[![License](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](LICENSE)

**BuildWarden** produces a cryptographically-signed, tamper-evident ledger of every network input to a software build. It records what was downloaded, from where, and when — without requiring the build author to manually attest what went into it.

The result is a verifiable receipt that anyone can inspect after the fact: auditors, security teams, downstream consumers, or the authors themselves.

## Why

Build systems pull hundreds of dependencies from dozens of registries. Today, if you want to know exactly what went into a build, you either trust the author's word or parse unreliable build logs. BuildWarden closes that gap by recording the ground truth at the network layer — automatically, for any build system, with no changes to the build itself.

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

### Build something

```sh
warden build
```

That's it. BuildWarden finds the Dockerfile in the current directory, runs the build inside a network-audited container, and writes results to `warden-output/`:

```
warden-output/
├── ledger.zst              # Signed ledger of all network I/O
├── artifacts/              # Build outputs posted to the ledger
│   └── myapp
├── Dockerfile.submitted    # What you wrote
├── Dockerfile.actual       # What actually ran (with warden injections)
├── ca.cert.pem.zst         # Ephemeral CA (destroyed after build)
└── relay.log               # Relay event log
```

### Inspect it

```sh
warden inspect warden-output
```

```
════════════════════════════════════════════════════════════════════════════════
  LEDGER  scheme=ed25519-sha512  hashes=[blake2b_256 sha256 sha1 md5]  ✅
════════════════════════════════════════════════════════════════════════════════
✅ GET https://registry-1.docker.io/v2/library/python/manifests/3.12-slim (10373 bytes)
✅ GET http://deb.debian.org/debian/dists/bookworm/main/binary-arm64/... (8690780 bytes)
✅ GET https://files.pythonhosted.org/packages/.../requests-2.32.3.tar.gz (131218 bytes)
✅ ARTIFACT POST http://artifacts/myapp (9218691 bytes)

════════════════════════════════════════════════════════════════════════════════
  SUMMARY: 255 records, 64 requests, 59.8 MB audited, 1 artifact(s)
  SIGNATURES: ✅ All 256 signatures valid
  COMPLETENESS: ✅ All channels closed
════════════════════════════════════════════════════════════════════════════════
```

Every request is individually hashed (BLAKE2b + SHA-256 + SHA-1 + MD5) and signed into an Ed25519 chain. Tampering with any byte invalidates the chain from that point forward.

## How It Works

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
│  │  • Artifacts │           │                          │    │
│  └──────┬───────┘           └──────────────────────────┘    │
│         │                                                    │
│         ▼ upstream                                           │
│    ┌──────────┐                                              │
│    │ Internet │                                              │
│    └──────────┘                                              │
└──────────────────────────────────────────────────────────────┘
```

1. The build container is network-isolated — all DNS, HTTP, and HTTPS traffic is forced through the relay via iptables DNAT rules.
2. The relay intercepts TLS with a per-build ephemeral CA (injected into the container's trust store). It forwards requests upstream, verifying the real server certificates.
3. Every request/response pair is hashed and signed into the ledger in real time. The signature chain ensures records cannot be reordered, inserted, or removed without detection.
4. Source files enter the build exclusively through the relay (Dockerfile `COPY` directives are rewritten to fetch via HTTP), giving full provenance over local files too.
5. Build outputs can be posted back to the ledger as artifacts (`curl -X POST http://artifacts/<name>`), binding them to the same signature chain as their inputs.

## Package Manager Support

BuildWarden works transparently with all major package managers. CA trust is configured automatically — no changes to your Dockerfile needed:

| Ecosystem | Managers |
|-----------|----------|
| System | apt, apk, dnf |
| Python | pip, uv, poetry, conda, pipenv |
| Node.js | npm, yarn, pnpm |
| Rust | cargo |
| Go | go modules |
| Java | maven, gradle, bazel |
| Ruby | gem, bundler |
| PHP | composer |
| .NET | nuget |
| C/C++ | conan, vcpkg |
| Nix | nix-env, nix-build |
| Elixir | hex, mix |

## Commands

```sh
warden build [path]           # Run a build with full network auditing
warden build -o ./my-output   # Custom output directory
warden build --capture all    # Save request/response payloads to disk
warden inspect <path>         # Verify and display a ledger
warden inspect --json <path>  # Machine-readable output
warden shell [path]           # Interactive shell in audited environment
warden clean                  # Remove orphaned containers/networks/volumes
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

```sh
warden build examples/Dockerfile.simple
warden build examples/Dockerfile.npm
warden build examples/Dockerfile.cargo
```

See [`examples/`](examples/) for Dockerfiles covering Python, Node.js, Rust, Go, Java, Ruby, PHP, .NET, Nix, Elixir, and more.

## Ledger Format

The binary ledger is self-describing and compact:

- **Header**: magic bytes, Ed25519 public key, hash algorithm list, CBOR metadata
- **Records**: open/checkpoint/close/artifact — each signed with the previous signature as input (chained)
- **Payload direction**: positive = inbound (downloads), negative = outbound (uploads/artifacts)
- **Metadata**: CBOR-encoded, not covered by signatures (freely redactable for privacy)

Full specification: [Ledger Spec](https://buildwarden.github.io/buildwarden/design/ledger-spec.html)

## Documentation

Full documentation is available at **[buildwarden.github.io/buildwarden](https://buildwarden.github.io/buildwarden/)**.

- [Ledger Specification](https://buildwarden.github.io/buildwarden/design/ledger-spec.html) — Binary format, record types, signature scheme
- [Philosophy](https://buildwarden.github.io/buildwarden/design/philosophy.html) — Design principles and threat model
- [Initial Proposal](https://buildwarden.github.io/buildwarden/design/initial-proposal.html) — Original problem statement

## Development

```sh
make build     # Compile warden + relay binaries
make test      # Run unit tests (97 tests, race-clean)
make lint      # golangci-lint
make cover     # Tests with coverage report
```

See [DEVELOPING.md](DEVELOPING.md) for project structure and extension system.

## License

Apache-2.0. See [LICENSE](LICENSE).
