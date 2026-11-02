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
make lint                # golangci-lint, then the tier-1 checks on your diff
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

### Tier-1 checks

`make lint` ends by building `dist/tier1` from `tools/tier1/` and running it
over the diff from `origin/main` to `HEAD` (committed changes only). These are
the deterministic checks of the behavioral linter (LINT-02): the coupling
checks (a design in the diff, its `drivers` claim, the bootstrap surface, the
ledger schemas and their spec, the CLI and config surface and its guide
pages), `go/analysis` passes for the relay and the executor, and a
golangci-lint config limited to the lines you changed.

- **The base** is the merge base of `TIER1_BASE` (default `origin/main`) and
  `HEAD`. If it cannot be resolved, tier 1 exits 2 and prints the fix,
  usually `git fetch origin main`. It never falls back to checking the whole
  repository. Use `make tier1 TIER1_BASE=v0.5.0` to check everything since a
  release.
- **Other platforms.** The analyzers load packages for the host `GOOS`. If
  you changed build-tagged files for another platform, also run
  `GOOS=<os> make lint`.
- **No design.** A change that needs no design (FLOW-01) passes
  `TIER1_FLAGS=-no-design`; in CI the `no-design` label does the same.
- **Design front-matter.** Every `docs/design/*.md` the change adds, modifies
  or renames must have front-matter that passes the same FLOW-02 validator
  `wf` uses, with the file name (without `.md`) as the `feature`. An existing
  design that never had front-matter predates FLOW-02 and is listed as a hint
  rather than failed (FLOW-12); a new design without front-matter fails.
  `dist/tier1 designs` prints the verdict for each touched design, and
  `dist/tier1 designs -all` for every design. Archived designs kept as merged
  although they lack a later key are listed under `design_tolerate` in
  `tools/tier1/rules.yaml`, and their tolerated problems are reported without
  failing.
- **Suppressing a finding.** Add a trailer to any commit in the change:
  `Tier1-Allow: <RULE-ID> <path-glob>: <reason>`, for example
  `Tier1-Allow: UX-01 cmd/warden/*: guide pages follow in #123`. The reason is
  required. Every applied suppression is printed in its own section, so the
  review and the sign-off see it (LINT-05). `*` matches within one path
  element, `**` across elements, and a glob ending in `/` matches a whole
  directory.
- **Exit codes.** 0 means clean, or only SHOULD findings and hints. 1 means an
  unsuppressed MUST or MUST-review finding. 2 means a usage or environment
  error, such as an unresolvable base or a missing golangci-lint. Hints never
  fail; they are signals for the tier-2 review, such as a package whose code
  changed with no test change (GLOBAL-04).
- **Output.** `dist/tier1 -json` prints the report in the form
  `tools/tier1/report.schema.json` declares, keyed by rule and `file:line`.
- **Whole-tree audit.** `dist/tier1 -all` also runs the checks of invariants
  that hold over the whole tree, today the `specs/` layout (RELAY-05), even
  when the diff does not touch them.
- **The rule index.** `dist/tier1 rules [-json]` prints every rule parsed from
  `docs/development/`. `tools/tier1/rules.yaml` maps each `automated`-tagged
  rule to a tier-1 check, an outside owner or `follow-up`, and tier 1 fails a
  change to the standards that leaves a rule unmapped.

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
4. Open a PR against `main`. On a PR that changes code, CI runs lint, tests and
   cross-platform builds. A PR that changes only `docs/` (outside
   `docs/development/`) or top-level Markdown files runs tier 1, the FLOW-18
   check and, when it touches `docs/book/`, an mdBook build. The `ci-ok` check
   sums up the run. A maintainer signs off before merging (FLOW-08).

## Cutting a release

1. Make sure `make test` passes on `main`. The release workflow runs tier 1
   against the previous `v*` tag and checks every design's front-matter
   (`tier1 designs -all`) before it publishes; run
   `make tier1 TIER1_BASE=<previous tag>` first to see what it will report.
   The full release gate (tier-1 checks, the test matrix, the security suite
   and the behavioral report) is described in
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
