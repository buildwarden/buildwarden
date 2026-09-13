# Orchestrator Standards

Standards for the orchestrator: `cmd/warden/`. The orchestrator is the host CLI,
driver dispatch, config, extensions, inspect, and image lifecycle. It is the
opinionated integrator (GLOBAL-08): it enforces strong security boundaries by
default and treats the relay and executor as fixed, non-substitutable dependencies.

See [`README.md`](./README.md) for the rule format.

---

### ORCH-01 — Driver parity / no cross-contamination

**Severity:** MUST
**Check:** automated + agent-judgment

A build-lifecycle change is expressed at the `driver.Driver` interface level, or it
explicitly names which driver(s) it touches (container / qemu / vz, plus planned
hyperv / firecracker) and asserts the others are unaffected. This is the standing,
permanent form of the cross-contamination check: a change made for one driver must
not silently alter the behavior of another.

The automated part compares which `driver/*` trees the diff actually reaches
against what the change claims; the agent part judges whether shared code was
altered in a driver-specific way.

---

### ORCH-02 — Build/config inputs are read-only; mutations are captured

**Severity:** MUST
**Check:** automated + agent-judgment

The orchestrator treats all build/config inputs as read-only. This is not limited
to a Containerfile; it covers any input form. If the orchestrator alters an input,
both the original and the edited form are recorded to the output directory under
`inputs/`.

The `inputs/` namespace must not collide with the ledger filename or the relay's
recorded-artifact layout in the output directory (currently `ledger`, `payloads/`,
`artifacts/`). Keeping orchestrator-captured inputs in their own `inputs/` subtree
is the clean way to prevent accidental collision with relay-recorded artifacts.

This intentionally requires an orchestrator change to create and populate
`inputs/`. It is the orchestrator-side expression of the same
completeness-of-inputs philosophy behind RELAY-02.

The automated part checks that input mutations land under `inputs/` and do not use
a reserved output name; the agent part judges new input-handling paths.

---

### ORCH-03 — Config-surface discipline

**Severity:** MUST
**Check:** automated + agent-judgment

Configuration precedence stays `flag > warden.toml > ~/.config/warden`. Defaults
are sane and changes are additive. A new or changed config key is documented in the
same change; this is the config half of the docs-coupling rule (UX-01).

The automated part is a config-key diff against a docs diff.

---

### ORCH-04 — Transient resources are torn down; durable outputs are not

**Severity:** MUST
**Check:** agent-judgment

Teardown covers only the **transient** resources an operation allocates up to
finalization: networks, subnets, containers, VMs, temp dirs, per-build
pflash/vars, downloaded install media, mounts, and partial or aborted image builds.
Anything the orchestrator allocates in this class has a matching teardown reachable
from the clean path.

It explicitly does **not** touch **durable outputs** that are the deliberate product
of an operation: a finalized cached base image, or a completed ledger/output
directory. Those are removed only via explicit commands (`warden image` management,
an explicit clean target), never by the automatic teardown path. A freshly prepared
image is safe from its own build's cleanup.

The agent distinguishes transient allocation from durable output.

---

### ORCH-05 — Windows host is a required 1.0 feature

**Severity:** MUST
**Check:** automated + agent-judgment

The core orchestrator running on Windows as a first-class host binary is a required
1.0 feature and the current primary development thrust, not a nice-to-have that may
regress. The actual portability rule is GLOBAL-07 (cross-platform by default); this
rule records that Windows host support is a 1.0 acceptance criterion and that CI
builds `cmd/warden` for all supported host targets.

The qemu driver is the cross-platform build path, so ORCH-01 (driver parity) and
GLOBAL-07 (portability) reinforce each other here.
