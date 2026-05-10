# Developing BuildWarden

## Prerequisites

- Go 1.26+
- A container runtime (finch, docker, or podman)
- golangci-lint (for `make lint`)

## Commands

```sh
make build     # Compile warden binary
make test      # Run unit tests
make cover     # Tests with coverage report
make lint      # Run golangci-lint
make fmt       # Format code
make tidy      # go mod tidy
make clean     # Remove built binaries
```

## Project Structure

```
cmd/warden/              Host binary — CLI, orchestrator, inspect, extensions
cmd/relay/               Container binary — proxy, ledger writer, DNS, TLS, fair scheduler
ledger/                  Shared library — wire format types, reader, verification
examples/                Example Dockerfiles
docs/design/             Specifications and design documents
```

### Key boundaries

- **`cmd/warden/`** runs on the host. It creates the network, builds the relay image, starts containers, configures iptables, runs inspect, and tears everything down.
- **`cmd/relay/`** runs inside a container. It intercepts traffic, writes the ledger, and generates the ephemeral CA. Fully independent of the warden binary.
- **`ledger/`** is the only shared code — defines the binary ledger format and provides read/verify logic used by both `warden inspect` and the relay's test suite.

## Extension System

Extensions implement `BeforeBuild(env *CtrEnv) error` and optionally `Env() map[string]string`. They:

1. Write files to `.warden/` (which gets COPY'd into the build image)
2. Write shell scripts to `.warden/ext.d/` (exec'd inside the build container)
3. Return env vars to inject into the Dockerfile after each `FROM` line

Current extensions: truststore (CA cert injection), pip (PIP_CERT), bazel (JKS truststore), epoch (SOURCE_DATE_EPOCH=0).

## Running Integration Tests

Integration tests require a working container runtime:

```sh
warden build examples/Dockerfile.simple
```

Verify the output:

```sh
warden inspect /tmp/warden-ledger-*/ledger
```

## Cutting a Release

1. Ensure all tests pass: `make test`
2. Tag the commit: `git tag v0.x.y`
3. Push the tag: `git push origin v0.x.y`
4. GoReleaser (via GitHub Actions) builds and publishes binaries
