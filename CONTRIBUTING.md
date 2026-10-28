# Contributing to BuildWarden

Thank you for your interest in contributing! This guide covers setup, building,
testing, and how a change gets merged. The standards a change is held to live in
[`docs/development/`](https://github.com/buildwarden/buildwarden/tree/main/docs/development),
and the process for designing, reviewing and merging a feature is
[`process.md`](https://github.com/buildwarden/buildwarden/blob/main/docs/development/process.md).

## Setup

**All platforms:**

- Go at the version in `go.mod` (we manage it with [mise](https://mise.jdx.dev/);
  run `mise exec -- go ...` if your `GOROOT` is misconfigured)
- golangci-lint v2
- A container runtime: finch, docker, or podman

**VZ driver (macOS 13+ on Apple Silicon):**

- Xcode Command Line Tools, for cgo and Objective-C
- The host binary needs the virtualization entitlement. `make build` signs it
  ad hoc; to sign by hand:
  `codesign --sign - --entitlements entitlements.plist dist/warden`

**QEMU driver (any platform):**

- `qemu-system-aarch64` or `qemu-system-x86_64`, plus `qemu-img`
  (`brew install qemu` on macOS, `apt install qemu-system` on Linux)

**Hyper-V driver (Windows):**

- Hyper-V and its management stack enabled
- Membership in Hyper-V Administrators, or an elevated terminal
- One-time network setup from an elevated terminal: `warden setup --driver hyperv`

## Building and testing

```sh
make build               # Compile all binaries to dist/
make test                # Unit tests: run on every platform, no external dependencies
make lint                # golangci-lint
make fmt                 # gofmt -s -w
make tidy                # go mod tidy
make cover               # Tests with a coverage report
make deadcode            # Code unreachable on Linux, macOS and Windows
make integration-test    # Real builds plus ledger checks; needs a container runtime
make integration-test-vz # VZ end to end; needs macOS/arm64 and a prepared image
make clean               # Remove built binaries
```

`make build` cross-compiles the relay and `warden-io` for linux/amd64 and
linux/arm64; on macOS/arm64 it also builds `warden-io` for darwin/arm64 and signs
the host binary. Platform-specific code has build-tagged stubs, so `make test`
passes everywhere. The test tiers and what each one needs are in
[`testing.md`](https://github.com/buildwarden/buildwarden/blob/main/docs/development/testing.md).

Local builds report `version = "dev"`, which builds the relay from source. After
changing the relay or orchestrator, a quick end-to-end check is:

```sh
./warden build .                          # self-build: COPY provenance, Go modules, artifacts
./warden build examples/Dockerfile.simple # pip and Debian packages
./warden inspect warden-output            # check the ledger
./warden clean                            # if a build crashed or was interrupted
```

## Pull requests

1. Fork the repo and branch from `main`.
2. Changes that make design decisions start with a design document in
   `docs/design/`, written from
   [`TEMPLATE.md`](https://github.com/buildwarden/buildwarden/blob/main/docs/design/TEMPLATE.md)
   (FLOW-01). Dependency bumps, typo fixes and CI-only changes are labelled
   `no-design` instead.
3. Run `make lint && make test` before pushing. Keep commits focused: one logical
   change per commit.
4. Open a PR against `main`. CI runs lint, tests and cross-platform builds, and a
   maintainer signs off before merging (FLOW-08).

## Cutting a release

1. Make sure `make test` passes on `main`. The full release gate (tier-1
   checks, the test matrix, the security suite and the behavioral report) is
   described in
   [`behavioral-linter.md`](https://github.com/buildwarden/buildwarden/blob/main/docs/development/behavioral-linter.md)
   and is being built out.
2. Tag the commit and push the tag: `git tag v0.x.y && git push origin v0.x.y`.
3. GoReleaser, run by GitHub Actions, builds and publishes the binaries.

## Security

Report security issues as described in
[`SECURITY.md`](https://github.com/buildwarden/buildwarden/blob/main/SECURITY.md).
Do **not** open a public GitHub issue for them.

## License

See the [LICENSE](https://github.com/buildwarden/buildwarden/blob/main/LICENSE) file for details.
