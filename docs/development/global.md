# Global Standards

Cross-cutting principles that apply to all three applications. See
[`README.md`](./README.md) for the rule format and severity/check-type meanings.

---

### GLOBAL-01 — Comments: doc public interfaces, explain only the non-obvious

**Severity:** MUST
**Check:** automated + agent-judgment

Exported identifiers carry Go-style doc comments (`// Name ...`). Implementation
comments are reserved for *why* a piece of code exists, or the specific
algorithm/approach when the implementation is dense or complex to walk through.
They are never a restatement of what the code plainly does. This keeps comments
from drifting out of sync with the code and the documentation they describe.

The automated part flags a new exported identifier with no doc comment. The
agent part judges whether an implementation comment is redundant, or whether it
genuinely explains a non-obvious "why" or a dense approach.

---

### GLOBAL-02 — Formatting: gofmt on touched lines only

**Severity:** SHOULD
**Check:** automated + agent-judgment

Code is `gofmt -s` clean. gofmt is intentionally not a hard CI gate: Go
toolchain-version skew causes repo-wide formatting drift in files nobody touched.
So the linter flags only the lines a change actually touched that are not
gofmt-clean, never pre-existing whole-repo drift. Do not reformat the whole repo
over a toolchain version difference.

---

### GLOBAL-03 — Errors: wrap, don't panic, assert safely

**Severity:** MUST
**Check:** automated + agent-judgment

Errors are wrapped with `%w` to preserve the chain. Library paths do not `panic`
for ordinary failures. Type assertions use the comma-ok form (enforced by the
`forcetypeassert` linter).

---

### GLOBAL-04 — Tests land with the change

**Severity:** MUST
**Check:** automated + agent-judgment

New behavior and bug fixes land with tests in the same change, table-driven where
it fits. This rule is only "don't merge untested behavior"; the tiering, the
platform matrix, and the velocity tradeoffs are in [`testing.md`](./testing.md).

---

### GLOBAL-05 — Reproducibility: enable it, don't enforce it

**Severity:** SHOULD
**Check:** agent-judgment

Do not unnecessarily introduce vectors that stop an otherwise-reproducible build
from reproducing (gratuitous wall-clock reads, unsorted output, map-iteration
order in a path that was previously deterministic). Keep the environment set up
to encourage and enable reproducibility (for example `SOURCE_DATE_EPOCH=0`).

Forcing all builds to reproduce is an explicit non-goal. Many builds are not
reproducible by default, and that is acceptable. This is a "did you needlessly
break something that was working" check, not a "prove this build reproduces" gate.

---

### GLOBAL-06 — Security-boundary changes are high-severity

**Severity:** MUST-review
**Check:** agent-judgment

The security-relevant boundaries are the ones separating the **host OS**, the
**inner build environment**, and the **relay** from one another. Any change that
alters the security posture of one of those boundaries, plus **any change to the
relay itself**, is flagged high-severity and requires an explicit security note
in the change.

CA / key / trust-store handling, the SSRF filter, netns/iptables isolation, and
the socketpair topology are examples *because* they define or cross those
boundaries; they are not the definition. The definition is the boundary.

Scope: this rule binds the default reference stack. See *Deployment modes and the
ledger's role* below.

---

### GLOBAL-07 — Cross-platform by default

**Severity:** MUST
**Check:** automated + agent-judgment

Every component builds and runs across its supported platforms with no gratuitous
host-OS assumptions: no hardcoded `/` path separators, unix-only syscalls,
POSIX-only exec/signal patterns, or `/tmp` hardcoding. Platform-specific logic
goes behind build-tagged files (`*_windows.go` / `*_unix.go` / `platform_<os>.go`)
with stubs so every target compiles.

The orchestrator's Windows-host support is a specific 1.0 instance of this
principle (ORCH-05), not a special case. The automated part builds each component
for its supported targets in CI; the agent part judges the portability of new
code.

---

### GLOBAL-08 — Component independence (asymmetric)

**Severity:** MUST
**Check:** automated + agent-judgment

The three applications are not symmetrically swappable, and the asymmetry is
intentional.

- The **relay** and **executor** are independently usable and expose strong,
  interface-versioned contracts, so a third party can consume either without the
  default orchestrator. For example: a vendor (such as AWS CodeBuild) reuses the
  relay and executor as-is but writes its own orchestrator over proprietary,
  non-public isolation and resource management; or a relay-only deployment uses
  the ledger as a logging document rather than a security document.
- The **orchestrator** is deliberately not composable that way. It is the
  opinionated integrator: it enforces strong security boundaries by default and
  treats the relay and executor as fixed, non-substitutable strong dependencies.
  It is not required to support swapping in a different relay or executor, and it
  is not engineered to run in a reduced or logging-only mode.

**Versioned seams** (same pre/post-1.0 posture as the ledger format in RELAY-05):

- Relay public interface: control plane, `ca.cert.pem` handoff, DNS / artifact /
  heartbeat protocol.
- Executor public interface: the `initialize` invocation contract plus its side
  of the relay protocol.

Pre-1.0 these seams may still move. Post-1.0 they are stable and change rarely and
deliberately, with explicit interface versioning. The `driver.Driver` interface
and other orchestrator internals are **not** covered by this obligation; the
orchestrator may treat them as internal extension points and stay rigid about its
expected components.

The automated part requires a change touching a versioned seam to also change that
seam's contract document.

---

## Deployment modes and the ledger's role

The **default reference stack** is the relay plus the opinionated orchestrator
plus the executor. In that stack the ledger is a **security document**, and the
security-boundary rules (GLOBAL-06) bind it fully.

Running the relay standalone with the ledger as a **logging document** rather than
a security artifact is a supported *use of the relay* (GLOBAL-08). Such a
deployment owns its own boundary decisions, and GLOBAL-06 does not bind it. We do
not engineer the default orchestrator for that mode.

This scoping matters for the behavioral linter: GLOBAL-06 severity applies to
changes in the reference stack, and a downstream integrator running relay-only in
logging mode is responsible for its own security posture.
