# BuildWarden

BuildWarden is a platform for build executors that combines network-isolated build containers with an ephemeral HTTPS Man-in-the-Middle relay. It logs all requests and responses during the build process and produces a verifiable, non-falsifiable ledger of all network inputs and outputs.

This creates a significantly more exhaustive manifest of all possible software included within created artifacts, allowing build providers to independently attest that a build was secure — separately from the source author's own attestations.

## Architecture

BuildWarden orchestrates two containers on an isolated network:

- **Relay** (`10.0.87.2`) — An SSL-terminating MITM proxy that intercepts all HTTP/HTTPS traffic, records it to the ledger, and forwards requests externally. Also serves as the DNS resolver for the build container.
- **Build Container** (`10.0.87.3`) — A Docker-in-Docker environment where the actual build runs. Network-isolated via iptables to only communicate with the relay.

The relay generates an ephemeral certificate at startup, signs every ledger entry with it, and the private key is destroyed when the relay container is removed — making the ledger tamper-evident after the fact.

## Building

```sh
./build.sh build        # Produces: warden, ledger-inspect
./build.sh test         # Run unit tests and integration test
./build.sh fmt          # Format code
./build.sh lint         # Run linters
./build.sh tidy         # go mod tidy
```

## Usage

### Running a build

```sh
./warden build -f Dockerfile ./path/to/context
```

Set `WARDEN_CTR_CLI` to select the container runtime (default: `docker`):

```sh
WARDEN_CTR_CLI=finch ./warden build -f Dockerfile ./context
WARDEN_CTR_CLI=podman ./warden build -f Dockerfile ./context
```

The ledger output directory is printed at the end of the build.

### Interactive shell

```sh
./warden shell ./path/to/context
```

### Inspecting a ledger

```sh
./ledger-inspect /path/to/ledger
```

Validates the full signature chain, displays entries in a compact tree format, and reports completeness.

## Ledger Format

See [docs/design/Ledger-Spec-v2.md](docs/design/Ledger-Spec-v2.md) for the full specification.

Key properties:
- **Chained signatures** — Each entry signs over the previous entry's signature, making the ledger tamper-evident and order-sensitive.
- **Multi-hash payload identity** — blake2b_256, sha256, sha1, md5 for every payload.
- **Redactable metadata** — URLs and hostnames are not part of signatures, allowing internal builds to strip origin information while preserving integrity.
- **Asynchronous channels** — Concurrent requests are tracked via open/checkpoint/close entries linked by the open entry's signature.

## Test Dockerfiles

| File | Description | Architecture |
|------|-------------|--------------|
| `buildctx/Dockerfile.simple` | Alpine + pip install numpy in venv | Multi-arch |
| `buildctx/Dockerfile.llvmlite-multiarch` | Ubuntu + apt + wget LLVM source | Multi-arch |
| `buildctx/Dockerfile.tf-amd64` | TensorFlow build environment | x86_64 only |
| `buildctx/Dockerfile.tfexample-amd64` | Full TensorFlow from-source build | x86_64 only |

## Design Documents

- [Initial Proposal](docs/design/Initial-Proposal.md)
- [Philosophy](docs/design/Philosophy.md)
- [Relay and Ledger Spec (v1)](docs/design/Relay-and-Ledger-Spec.md)
- [Ledger Spec v2](docs/design/Ledger-Spec-v2.md)

## Security

See [CONTRIBUTING](CONTRIBUTING.md#security-issue-notifications) for more information.

## License

This project is licensed under the Apache-2.0 License.
