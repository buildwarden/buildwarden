# Testing and Dev-Velocity Standards

How BuildWarden manages testing to keep a high standard while keeping development
productive and release velocity healthy. The central tension is the test matrix:
unit tests run anywhere, but the value paths are expensive and host-constrained
(VZ needs macOS/arm64 plus a prepared image plus codesigning; qemu needs a base
image; the container path needs a runtime; the Windows guest needs a long
image-prep). None of that can gate every PR without killing velocity, so the whole
section is built around **tiering**.

See [`README.md`](./README.md) for the rule format.

---

### TEST-01 — Fast, universal gate

**Severity:** MUST
**Check:** automated + agent-judgment

`make test` (pure-Go, table-driven unit tests) runs on every supported platform
with no external dependencies and is the gate every change that can affect it
passes: CI skips it only for changes confined to the docs-only paths in TEST-02,
and every push to `main` runs it. Platform-specific
code has build-tagged stubs so this layer compiles and runs everywhere; this is the
*testing* reason the GLOBAL-07 / EXEC-01 stub discipline exists, not just
cross-compilation. A slow or flaky universal gate is itself a velocity defect.

---

### TEST-02 — Tiered tests, scoped to the change, full at release

**Severity:** MUST
**Check:** automated + agent-judgment

Three tiers:

0. **Unit / universal** — runs everywhere (TEST-01).
1. **Integration** — needs a container runtime.
2. **Platform / VM end-to-end** — VZ, qemu, and the Windows guest.

The expensive tiers are not on the critical path of every PR. They run on the
relevant CI runner scoped to what changed, on a schedule, and **in full before a
release**. Per-PR pays for the fast gate plus the tiers the change actually touches.
Where no CI runner can host a tier (VZ, Hyper-V), it runs in native sessions
under FLOW-09 until one can.

Where each tier runs today:

- Tier 0: the `ci` workflow, on every push to `main` and on every PR that
  changes a path outside the docs-only set: `docs/**` except
  `docs/development/**`, `docs/book/src/specs/**` and
  `docs/book/src/SUMMARY.md`, and top-level `*.md`, with Go source and module files
  (`*.go`, `go.mod`, `go.sum`, `go.work`) never docs-only wherever they are.
  `main.yml`'s `changes` job is that set's implementation.
- Tier 1: the `integration` workflow (`make integration-test` on a hosted
  Ubuntu runner with Docker). It runs on PRs that reach `driver/container/`,
  the `driver/` root, a shared package (`relay/`, `ledger/`, `collector/`),
  `cmd/`, `test-integration.sh`, a `Dockerfile` or `go.mod`; weekly on
  `main`; and on demand for a branch that has the workflow
  (`gh workflow run integration.yml --ref <branch>`). The `conformance`
  workflow (TEST-05) is tier 1 too: it runs the seams' suites on hosted
  runners, on PRs and on pushes to a branch other than `main` that change
  a package the relay or the disk collector is built from, a file their
  builds read, the pins of the specification releases and the target
  files (`tools/conformance/`) or the workflow itself; weekly on `main`;
  and on demand.
  `conformance.yml`'s `paths` list is that set's implementation.
- Tier 2: native sessions (FLOW-09) on the machines in the `workflow`
  branch's `hosts.yaml`.

**Caveat, baked into the rule:** scoping tests to touched paths is where tiering can
quietly fail. A change that looks container-only can break VZ through a shared
package (`relay/`, `ledger/`, `driver/`). So the scoping must be conservative: a
change to a shared package triggers all dependent tiers, and the release full-matrix
is the backstop that catches whatever the per-PR scoping missed.

---

### TEST-03 — Contribution is not gated on owning every platform

**Severity:** MUST
**Check:** automated + agent-judgment

Because platform logic is unit-tested via stubs everywhere and real platform
behavior is validated in its CI tier, a developer or agent can validate a change
without owning every OS or VM. A platform path counts as "done" when its stubbed
unit coverage exists **and** its tier-2 validation exists or the gap is explicitly
tracked. A gap is tracked in the design's `tier2_gaps` and on the workflow's
native-gap backlog; FLOW-09 decides when a gap may not be deferred. This is the concrete resolution of the EXEC-03 trust-install-parity
concern: the parity logic is unit-tested on every platform, and the actual CA
install is validated per-guest in tier 2.

---

### TEST-04 — Flaky tests are defects; tests are deterministic

**Severity:** MUST
**Check:** agent-judgment

A flaky test is quarantined and fixed, never masked with blind retries. Flakiness in
an expensive tier is especially corrosive because it erodes trust in the release
gate. Unit-tier tests avoid wall-clock, network, and unseeded randomness; they use
golden fixtures and seeded inputs so a failure is real signal.

---

### TEST-05 — Conformance suites for the seams

**Severity:** MUST
**Check:** automated + agent-judgment

The claim protocol, the collector protocol and the relay guest API (three
of the GLOBAL-08 versioned seams) are specified in the specs repository,
`buildwarden/buildwarden-specs`, and checked there by black-box
conformance suites, so a substitute written in any language has something
to verify against. Each specification there is released under its own
version, and a release carries the suite that checks it. This repository
pins a tagged release of each specification in
`tools/conformance/<specification>/go.mod` and runs those releases'
suites against the components it holds, in the `conformance` workflow
(TEST-02):

- against the Go relay in each of its modes (RELAY-04) and as its OCI
  image: the suites of the claim protocol, of both specifications of the
  collector protocol (its outputs, and its inputs and signals) as the
  relay is their client, of the relay guest API, with the RELAY-02
  byte-exact round-trip (sent equals recorded), and of the bootstrap
  config as the relay carries it;
- against the disk collector (`cmd/collector`): the suites of both
  specifications of the collector protocol, of the claim protocol as
  its claimant, and of the bootstrap config as it is produced.

The contract tests of a seam specified there are added to the suite of
its specification in the specs repository, not to this repository. Two
contracts stay as tests
here until a suite covers them: RELAY-05's signature round-trip and reader
backward-compatibility (`relay/`, `ledger/`), and EXEC-04's protocol
conformance (`cmd/warden-io/`, `bootstrap/`), whose specifications
(`executor-initialize`, and `bootstrap-config` as the executor reads it)
have no cases yet.

The automated part is the `conformance` workflow and `make test`; the
agent part judges whether a seam change added its cases to the suite.

The driver contract carries a conformance suite that runs against a driver
binary (`conformance/driver/`), so a driver written by anyone, in any
language, has something to verify against. Its static tier runs in tier 0
against every driver binary this repository builds; its lifecycle tier runs
real builds on a host that can run the driver (TEST-02).

---

### TEST-06 — Coverage is signal, not a target

**Severity:** SHOULD
**Check:** agent-judgment

Cover behavior, and cover the security-critical paths (GLOBAL-06) heavily. Do not
chase a coverage percentage for its own sake.

---

### TEST-07 — Parallelize; amortize expensive init

**Severity:** SHOULD
**Check:** automated + agent-judgment

Tests run in parallel wherever safe, especially at the CI level. Expensive
initialization (VM boot, base image, container runtime, codesign) is amortized at a
sensible granularity: roughly one environment per OS per application, not one per
test. Unit tests use `t.Parallel()` where they are free of shared state. The goal is
that adding a test costs a test's worth of time, not an environment's worth.

---

### TEST-08 — Security suite on boundary-tripping releases

**Severity:** MUST
**Check:** automated (Arm A corpus) + agent-judgment (Arm B adversarial)

When a release includes changes that trip GLOBAL-06 (security-boundary), a dedicated
security testing suite runs against it: a deterministic attack-regression corpus
(Arm A) plus an adversarial pen-tester agent (Arm B) that demonstrates whether a
hole is open for a malicious actor to exploit, across the two failure classes
(isolation escape, measurement evasion) and the three boundaries.

The full design lives in [`security-suite.md`](./security-suite.md) (SEC-01..08). It
is a mandatory input to the release gate for boundary-tripping releases (see
[`behavioral-linter.md`](./behavioral-linter.md)).
