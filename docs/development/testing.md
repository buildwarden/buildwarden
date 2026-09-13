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
with no external dependencies and is the gate every change passes. Platform-specific
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
tracked. This is the concrete resolution of the EXEC-03 trust-install-parity
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

### TEST-05 — Contract tests for the seams

**Severity:** MUST
**Check:** automated + agent-judgment

The relay and executor public contracts (the GLOBAL-08 versioned seams) carry
conformance / round-trip tests, so a third-party substitute has something to verify
against:

- RELAY-02 byte-exact round-trip (sent equals recorded),
- RELAY-05 signature round-trip plus reader backward-compatibility,
- EXEC-04 protocol conformance.

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
