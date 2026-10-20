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
- **`cmd/relay/`** runs inside a container or VM (or as a host process for VZ). It intercepts traffic, records the ledger, and generates the ephemeral CA, streaming every output to the host-side collector rather than writing build state to disk. Fully independent of the warden binary.
- **`ledger/`** is the only shared code — defines the binary ledger format and provides read/verify logic used by both `warden inspect` and the relay's test suite.

## Fail early, say what to do

A project standard for every command and driver:

1. **If a failure has a clear cause, say what to do.** A missing privilege (root
   for `warden image prepare --driver vz`, Administrator or Hyper-V
   Administrators on Windows), an unprepared image, an absent tool, a stopped
   container runtime or a bad flag is not a bug. Return a
   `driver.FixableError` (`driver.Fixable(fix, format, args...)`) whose `Fix` is
   the exact command or change that resolves it. warden prints it as
   `ERROR: <what is wrong>` plus `To fix: <what to do>`, and leaves out the
   file-a-bug link, which is reserved for failures warden cannot explain.
2. **Check it as early as it is cheap.** Do the check before any slow,
   costly or irreversible work: before a download, a disk restore, a VM boot
   or a network standup. A failure that is certain after 25 minutes is
   usually detectable in milliseconds. The bar is the successful run: a check
   belongs up front when it adds negligible time to a build or prepare that
   would have succeeded (a `stat`, a `LookPath`, a uid or token check, a
   manifest read). Checks that are themselves slow belong in
   `warden doctor --driver <d>`, not on every run.
3. **Map platform errors you recognize.** When a known limit surfaces as an
   opaque OS or framework error (for example VZErrorDomain code 6, Apple's
   two-running-macOS-VMs limit), translate it into a `FixableError` at the
   call site.
4. **Exit codes stay meaningful.** A build's own non-zero exit is a
   `driver.ExitError` and warden exits with that code; `--timeout` exits 124
   and Ctrl-C 130. Everything else exits 1.

## Extension System

Extensions implement `BeforeBuild(env *CtrEnv) error` and optionally `Env() map[string]string`. They:

1. Write files to the staged `/.warden` tree (`ExtensionContext.WardenDir`, in the build's private work dir; copied into the build container as `/.warden`)
2. Write shell scripts to its `ext.d/` (exec'd inside the build container)
3. Return env vars to inject into the Dockerfile after each `FROM` line

Current extensions:
- **truststore** — Injects CA cert into system trust store (`/etc/ssl/certs/`)
- **jks_truststore** — Creates Java KeyStore, sets MAVEN_OPTS/GRADLE_OPTS for JVM tools
- **cacerts_env** — Env vars for non-system-CA package managers (npm, pip, uv, poetry, conda, cargo, gem, nix, hex, composer, conan)
- **epoch** — Sets SOURCE_DATE_EPOCH=0 for reproducible builds

## Running Integration Tests

Integration tests require a working container runtime:

```sh
warden build examples/Dockerfile.simple
```

Verify the output:

```sh
warden inspect warden-output
```

## Cutting a Release

1. Ensure all tests pass: `make test`
2. Tag the commit: `git tag v0.x.y`
3. Push the tag: `git push origin v0.x.y`
4. GoReleaser (via GitHub Actions) builds and publishes binaries
