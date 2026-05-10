# Development Setup

## Prerequisites

- Go 1.26+ (managed via [mise](https://mise.jdx.dev/))
- A container runtime (finch, docker, or podman)
- golangci-lint

## Building

```sh
make build     # Compile warden binary
make test      # Run unit tests
make lint      # golangci-lint
make fmt       # Format code
make tidy      # go mod tidy
```

## Running in Development

Local builds use `version = "dev"` which triggers building the relay from source (requires Go toolchain):

```sh
./warden build examples/Dockerfile.simple
```

## Integration Tests

The integration test suite runs real builds and validates ledger contents:

```sh
make integration-test
```

This takes several minutes (runs full build + inspect cycles).

## Testing Changes

After modifying the relay or orchestrator:

1. `make build` — compile
2. `make test` — unit tests
3. `make lint` — lint
4. `./warden build .` — self-build (exercises COPY provenance, Go modules, artifact posting)
5. `./warden build examples/Dockerfile.simple` — simple demo (exercises pip, Debian packages)

## Cleaning Up

If builds crash or get interrupted:

```sh
./warden clean
```
