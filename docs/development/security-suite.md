# Security Testing Suite (Deep-dive #1)

The design behind TEST-08: how BuildWarden security-tests a release that trips a
security boundary (GLOBAL-06). This document is the full form of the TEST-08 stub.

See [`README.md`](./README.md) for the rule format.

## What "winning" means for an adversary

BuildWarden's promise is faithful measurement of build-time I/O across the
boundaries in GLOBAL-06. An adversary in the build environment has two distinct
ways to win, and they are not the same:

- **Class 1 — Isolation escape / influence beyond the boundary.** The build
  reaches or affects something it should not: the host OS, the relay's keys or
  control plane, other builds, or the network past the relay.
- **Class 2 — Measurement evasion / ledger blinding.** The build moves bytes in
  or out **without the relay faithfully recording it**. This is the
  BuildWarden-specific class and the heart of "influence beyond what the relay is
  measuring." The ledger claims completeness of inputs (RELAY-02); any unrecorded
  channel breaks the promise even if nothing "escaped."

## What a finding is (proof standard)

The goal is to prove a hole is **open** (that a malicious actor *could* exploit
it), not to carry out the malicious action. A finding demonstrates an exploitable
hole through a **measurable, clearly-evident, reproducible outcome**, using benign
actions.

For Class 2 the proof is self-contained, which is why no omniscient external
observer is needed: the agent moves a **known token** across the boundary through
a channel that either should not exist (Class 1) or that the ledger does not
faithfully record (Class 2), then inspects the ledger against it. **The ledger is
its own oracle** — demonstrate the evident transfer, then show the ledger's
absence or mismatch. The known token can be as simple as exhibiting the specific
hash of an early-release `warden` binary carried through the illicit channel.

---

### SEC-01 — Threat model: two classes across three boundaries

**Severity:** MUST
**Check:** agent-judgment

Coverage is the matrix of {isolation escape, measurement evasion} against the
three GLOBAL-06 boundaries. Every corpus test and every agent objective maps to a
cell.

| Boundary | Class 1 (escape) | Class 2 (evasion) |
|---|---|---|
| build-env ↔ host OS | guest/container escape, host FS / `/proc` access, privileged syscalls, hypervisor surface, shared-folder / vmnet / socketpair abuse, reaching other builds | pivoting through the host to egress unrecorded |
| build-env ↔ relay | extract MITM CA / signing key (RELAY-03), drive the control plane, tamper ledger writes, abuse artifact endpoint / reserved hostnames | make the relay serve bytes ≠ recorded (RELAY-02), smuggle via unsigned CBOR metadata, DNS-tunnel through the relay resolver, artifact endpoint as exfil |
| build-env ↔ beyond relay | direct egress bypassing the relay (alt DNS, raw socket, IPv6, ICMP), TLS-MITM bypass | any egress/ingress absent from the ledger; timing/covert channels; a synthetic canary contacted with no ledger entry |

**VM build environments are the priority.** Container isolation is inherently
weaker (shared kernel), so container coverage is best-effort and lower-severity,
while VM boundaries (vz, qemu, future hyperv/firecracker) carry the suite's weight.

---

### SEC-02 — Two arms

**Severity:** MUST
**Check:** automated (Arm A) + agent-judgment (Arm B)

- **Arm A — deterministic corpus (red-team-as-code).** Known attack attempts
  encoded as tests, each asserting the boundary holds and/or the activity is
  recorded. Reproducible, CI-runnable, hard-gating.
- **Arm B — adversarial agent.** An open-ended pen-tester placed in the
  build-environment position with the explicit goal of demonstrating a Class-1 or
  Class-2 hole per the SEC-01 matrix.

---

### SEC-03 — Findings crystallize into the corpus

**Severity:** MUST
**Check:** agent-judgment

Every confirmed Arm-B finding is fixed and then encoded as a permanent Arm-A
regression test. The corpus grows monotonically. Arm B discovers; Arm A remembers.
This is what captures the expensive agent's creativity as cheap deterministic
checks and pins a patched hole shut forever.

---

### SEC-04 — Proof standard: evident, measurable, non-destructive

**Severity:** MUST
**Check:** agent-judgment

A finding demonstrates an exploitable hole via a clearly-evident, measurable,
reproducible outcome (for example, exhibiting a known early-release-binary hash
moved through an illicit or unrecorded channel, then showing the ledger against
it). The agent stops at proving the open door and never performs the actual
malice.

---

### SEC-05 — Synthetic surface, expand only when necessary

**Severity:** MUST
**Check:** agent-judgment

A synthetic in-harness "outside" presents minimal, known surface (an endpoint
serving/expecting a known token). The security agent may **propose** additions to
simulate specific external surface-area presentations, but only when the existing
surface cannot encompass the attack being demonstrated. Expansions are
human-reviewed proposals, never agent self-service.

---

### SEC-06 — Gate policy, honest about asymmetry

**Severity:** MUST
**Check:** automated

Arm A all-green is a hard gate. An Arm-B confirmed (evident, ledger-checked)
finding hard-blocks the release until fixed. Arm-B "no finding" is necessary but
not sufficient: it is human-signed and never auto-rendered as "secure." Agent
silence is not a security proof. This is the SEC-specific form of LINT-03: the
agent holds neither the veto nor the guarantee.

---

### SEC-07 — Harness blast-radius and scope

**Severity:** MUST
**Check:** agent-judgment

The adversarial agent runs real (benign-outcome) attempts, so it runs in a
disposable, ephemeral, maximally-isolated runner, at least as isolated as
production, so a genuine escape cannot touch the CI host or other work. Crashing
or over-taxing the outer host is explicitly **out of scope for now** (a reasonable
future expansion, but it conflicts with the blast-radius rule and is not the
point).

---

### SEC-08 — Trigger and cadence

**Severity:** MUST
**Check:** automated

- **Arm A** (cheap, deterministic) runs per-PR on boundary-touching changes and
  pre-release.
- **Arm B** (the adversarial agent) runs on a **monthly schedule** and
  **on-demand before any major platform/boundary update**, on the largest
  available model at highest effort. Not per-PR; the cost is deliberate and bounded
  to that cadence.

---

## Honest coverage limit

SEC-04 makes the ledger its own oracle for Class 2, which is elegant but bounded:
it only catches evasions the agent thinks to demonstrate. A channel nobody tests
still passes. This is inherent to pen-testing (you find what you look for), and it
is exactly why SEC-03 crystallization matters: the Arm-A corpus is the accumulated
memory of everything anyone ever thought to look for. State this limit plainly
rather than implying the suite proves absence of holes.

## Remaining build work

This document is the design. Implementation is separate:

- the isolated, disposable harness (SEC-07);
- the Arm-A deterministic corpus and its per-PR/pre-release wiring;
- the Arm-B agent definition (largest model, highest effort) and the synthetic
  surface (SEC-05);
- the monthly + on-demand cadence (SEC-08), e.g. a scheduled job, once the agent
  and harness exist.
