# Orchestrator Standards

Standards for the orchestrator: `cmd/warden/`. The orchestrator is the host CLI,
driver dispatch, config, inspect, and image lifecycle. It is the
opinionated integrator (GLOBAL-08): it enforces strong security boundaries by
default and treats the relay and executor as fixed, non-substitutable dependencies.

See [`README.md`](./README.md) for the rule format.

---

### ORCH-01 — Driver parity / no cross-contamination

**Severity:** MUST
**Check:** automated + agent-judgment

A build-lifecycle change is expressed in the driver contract (GLOBAL-08) and
held by its conformance suite (TEST-05), or it explicitly names which
driver(s) it touches and asserts the others are unaffected. This is the
standing, permanent form of the cross-contamination check: a change made for
one driver must not silently alter the behavior of another.

Lifecycle that every driver shares lives in `warden`: staging, the session
(`driver.Session`) with its collector and claim, the wait, and reporting. A
driver holds what its platform needs: starting the relay, the build
environment, and their teardown. Helpers that drivers built on the kit share
live under `driver/`; such a driver re-implements one only where its platform
genuinely differs, and the design says why.

The claim is the design's `drivers` front-matter (FLOW-02). The automated part
compares which `driver/*` trees the diff actually reaches against that claim,
and runs the driver suite against each driver binary this repository builds
(TEST-05); the agent part judges whether shared code was altered in a
driver-specific way.

---

### ORCH-02 — Build/config inputs are read-only; mutations are captured

**Severity:** MUST
**Check:** automated + agent-judgment

The orchestrator treats all build/config inputs as read-only. This is not limited
to a Containerfile; it covers any input form. If the orchestrator alters an input,
both the original and the edited form are recorded to the output directory under
`inputs/`.

`warden` stages the build script and records an altered input
(`driver.RecordInputs`) before it starts the driver. A driver alters no input:
it is given the staged script and the build context and changes neither. A
failed build therefore keeps its inputs, and a failure to record fails the
build before anything runs.

The `inputs/` namespace must not collide with the ledger filename or the relay's
recorded-artifact layout in the output directory (currently `ledger.zst`,
`ca.cert.pem.zst`, `build-output.log`, `relay.log`, `artifacts/`, `captures/`,
`payloads/`). Keeping orchestrator-captured inputs in their own `inputs/` subtree
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

`warden` reads one config file: the one `--config` names, else the one
`WARDEN_CONFIG` names, else `warden.toml` in the working directory when it
exists. It reads no other file and merges none; a named file that cannot be
read is an error. Configuration precedence stays `flag > environment variable >
the config file > default`. Defaults are sane and changes are additive. A new
or changed config key is documented in the same change; this is the config half
of the docs-coupling rule (UX-01).

`warden` reads its own flags and keys. A driver reads its own, from the config
file `warden` names to it, with the same precedence and before it does any
work (UX-08): through the kit (`driver/drivercmd`), or as the driver contract
requires of a driver written without it (GLOBAL-08). A driver run as a
separate binary does so in its own process. The keys the driver contract
names as common to drivers live in `[driver]`; a driver's overrides of
those, and its own keys, live in `[driver.<name>]`, which is read before
`[driver]`.

The automated part is a config-key diff against a docs diff, over `cmd/warden`
and the drivers this repository builds.

---

### ORCH-04 — Transient resources are torn down; durable outputs are not

**Severity:** MUST
**Check:** agent-judgment

Teardown covers only the **transient** resources an operation allocates up to
finalization: networks, subnets, containers, VMs, temp dirs, per-build
pflash/vars, downloaded install media, mounts, and partial or aborted image builds.
Anything the orchestrator allocates in this class has a matching teardown reachable
from the clean path.

A driver run as a separate binary tears down what it created for a build
when `warden` tells it to stop or when its `run` input ends, whichever
comes first, and finishes that teardown even when it can no longer write
its output, so a killed `warden` alone leaves nothing running. What a
killed driver leaves is reachable from the build ID and the record `warden`
kept for that build: `reap <build-id>` removes it, reports success only
when nothing is left, and is run by `warden` after a driver that died and
by `warden clean` for every build no running `warden` holds. What `warden`
itself staged for a build is removed the same way.

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
