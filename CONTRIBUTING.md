# Contributing to BuildWarden

Thank you for your interest in contributing! This guide covers development setup, testing, and the PR process.

## Development Setup

**Prerequisites (all platforms):**
- Go 1.25+ (we use [mise](https://mise.jdx.dev/) to manage versions)
- golangci-lint v2
- A container runtime: finch, docker, or podman

**macOS (VZ driver):**
- Xcode Command Line Tools (for CGo / Objective-C compilation)
- macOS 13+ on Apple Silicon
- After building, sign the binary for the virtualization entitlement:
  ```sh
  codesign --sign - --entitlements entitlements.plist dist/warden
  ```
  `make build` does this automatically with ad-hoc signing.

**QEMU driver (any platform):**
- `qemu-system-aarch64` (or `qemu-system-x86_64`) and `qemu-img`
- macOS: `brew install qemu`
- Linux: `apt install qemu-system`

## Building and Testing

```sh
make build       # Compile all binaries to dist/
make test        # Run unit tests
make lint        # golangci-lint
make fmt         # gofmt -s -w
make tidy        # go mod tidy
make cover       # Tests with coverage report
```

`make build` cross-compiles the relay and warden-io for linux/amd64 and linux/arm64. On macOS/arm64 it also builds warden-io for darwin/arm64 and codesigns the host binary.

## Testing

- `make test` runs all unit tests and works on all platforms.
- VZ-specific code uses build stubs on non-macOS, so tests pass everywhere.
- Integration tests (`make integration-test`) require a running container runtime.
- VZ integration tests (`make integration-test-vz`) require macOS/arm64 with a prepared VM image and a codesigned test binary. The Makefile handles signing.

## Pull Requests

1. Fork the repo and create a branch from `main`.
2. Make your change. Run `make lint && make test` before pushing.
3. Keep commits focused — one logical change per commit.
4. Open a PR against `main`. CI will run lint, test, and cross-platform builds.

## Code of Conduct

This project has adopted the [Amazon Open Source Code of Conduct](https://aws.github.io/code-of-conduct).

## Security

If you discover a security issue, please report it via our [vulnerability reporting page](http://aws.amazon.com/security/vulnerability-reporting/). Do **not** create a public GitHub issue.

## License

See the [LICENSE](LICENSE) file for details.
