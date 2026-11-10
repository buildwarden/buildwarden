# Relay Standards

Standards for the relay: the `relay/` library and the `cmd/relay/` binary. The
relay is the MITM proxy, DNS resolver, ledger writer, control plane, and fairness
scheduler, and it streams everything it records to the host-side collector
(RELAY-07). RELAY-08 also applies wherever the relay is configured: its claim
interface (`relaycfg/`), the relay VM init, the drivers and any other claimant.
It is the most security- and correctness-sensitive component, and it sits on the
path of every byte of build traffic.

See [`README.md`](./README.md) for the rule format. GLOBAL-06 fires on any change
to the relay.

---

### RELAY-01 — Single-writer ledger

**Severity:** MUST
**Check:** automated + agent-judgment

All ledger writes go through `Ledger.loop()`. `Open` is synchronous and returns
the signature as the channel ID; `Checkpoint`, `Close`, and `Artifact` are
fire-and-forget over the channel. No goroutine writes ledger records directly.

The automated part greps for ledger-write calls outside the loop; the agent part
judges new write sites.

---

### RELAY-02 — All bytes sent to the build match the ledger

**Severity:** MUST
**Check:** automated + agent-judgment

Every byte the relay sends to the build system must match what it records to the
ledger. If the relay alters anything (headers included), it records the altered
form it actually sent, so the record reflects what the build **consumed**.

This follows from the ledger's philosophy of **completeness of inputs**
(faithfully recording what the build received), not completeness of
origins/sources (what an upstream sent). Both are visible, but input fidelity is
paramount.

The invariant holds regardless of what backs the relay: a direct upstream proxy,
forwarding to an internal or air-gapped endpoint, or replaying pre-recorded
artifacts from a prior capture. Build systems routinely hash downloads to validate
soundness, so any mutation of served-versus-recorded bytes breaks real builds.

Future features may specifically amend this for metadata/headers.

The automated part is a round-trip test asserting sent-bytes equal recorded-bytes;
the agent part judges new serving paths.

---

### RELAY-03 — Both keys are memory-only, per-relay

**Severity:** MUST-review
**Check:** agent-judgment

Neither the Ed25519 ledger-signing key nor the MITM CA key is ever written to disk
or into the ledger.

- The **signing key** is the ledger's validity anchor and is the higher-stakes of
  the two.
- The **CA key** exists only for the inner build's trust, so it must be generated
  fresh at each relay instantiation and never reused between builds.

The agent evaluates new key handling; a grep for key-material persistence supports
it.

---

### RELAY-04 — Three-mode parity

**Severity:** MUST
**Check:** agent-judgment

A relay change states which of its three modes it touches:

- **container** — bind ports.
- **vm** — PID 1 in the Alpine relay VM.
- **host** — FD ingress plus SSRF filter.

SSRF-filter changes are security-critical, so GLOBAL-06 fires.

---

### RELAY-05 — Versioned, long-lived spec and schema docs

**Severity:** MUST
**Check:** automated + agent-judgment

The ledger format is strongly versioned: one spec document per major version, with
prior versions preserved for historical reference and support. Metadata schemas
BuildWarden defines or uses get their own schema documents under the same
per-version discipline, so additional metadata layouts can be added without
disturbing existing ones. The reader stays backward-compatible across the versions
it claims to support.

The relay is a reference implementation. Pre-1.0 it may still drive frequent schema
updates; post-1.0, breaking format changes are rare and deliberate given how long
these documents live.

**Target layout** — documentation and machine-enforced schema live side by side
and are enforced to match:

```
specs/
  build-ledger/
    v1.md                ledger wire-format spec (prose; binary format)
    v2.md                (future major versions kept alongside)
  metadata-http/
    v1.md                schema documentation
    v1.schema.json       machine-enforced schema, beside its doc
  metadata-<name>/
    v1.md
    v1.schema.json
  environment-<type>/
    v1.md                signed environment payload schema documentation
    v1.schema.json
  README.md              index of the specs
```

`build-ledger/` is prose only (it describes a binary wire format, with no JSON
counterpart). Each `metadata-<name>/` carries the doc and the machine schema
together. `environment-<type>/` holds the schemas of signed environment
payloads (an OCI image, a VM disk chain), paired like `metadata-<name>/`.

**Schema identity.** A schema's `$id` is a stable name, not a location. Ids
already written into ledgers, including the pre-migration
`https://github.com/buildwarden/buildwarden/schemas/v1/<name>.json` forms,
never change; new schemas take ids that mirror their `specs/` path.

**Enforced-to-match**, split by what is checkable:

- Deterministic: a machine schema and its doc are co-located, share the major
  version, and CI fails if one changes in a commit without the other. A new schema
  version with no matching doc, or vice versa, is a hard fail.
- Agent-judgment: field-level parity, i.e. the doc actually describes what the
  schema enforces and has not drifted in meaning.

The layout is in place: the ledger spec and every schema live under `specs/`,
and tier 1's `schema-doc` check enforces the deterministic half.

---

### RELAY-06 — Hot-path performance

**Severity:** SHOULD
**Check:** automated + agent-judgment

The relay is on the path of every byte of build traffic. Hot-path changes run
against the relay end-to-end benchmark and justify any regression past a set
threshold.

---

### RELAY-07 — Diskless relay; the collector is the only output path

**Severity:** MUST
**Check:** automated + agent-judgment

The relay keeps no build state on disk. Every output (ledger, artifacts, payloads,
build console output) streams to the host-side collector, and every input it serves
(build context, build script) is pulled from the collector, over the tunnel the host
opens when it claims the relay. This holds in all three modes (RELAY-04) and for
every driver, which is what lets the relay VM run without a data disk and keeps the
output directory identical across drivers.

A new output or input is added to the collector protocol, never as a relay-side
file or a driver-specific side channel. The collector protocol is part of the
relay's versioned public interface (GLOBAL-08).

The automated part flags filesystem writes in `relay/` outside test code; the agent
part judges new output and input paths.

---

### RELAY-08 — The relay is configured through its claim

**Severity:** MUST
**Check:** agent-judgment

Any configuration of the relay happens in the claim interface explicitly: a
field of `relaycfg.Config`, pushed on `/v1/claim`. Only what the relay needs in
order to receive a claim at all may reach it another way, on its boot channel:
its own address (the address it accepts the claim on included), prefix and
gateway, and the claim token. The boot channel is a per-build seed or the kernel
command line in vm mode, and the flags and environment its launcher passes in
host and container mode (RELAY-04). Everything else that configures the relay for
a build (its upstream resolvers, the build link, the collector, capture, upstream
TLS) is a claim field, in every mode and for every claimant.

- A new relay setting is a new claim field, added as a change to the relay's
  versioned interface (GLOBAL-08), never a new flag, environment variable, seed
  entry or kernel command-line key.
- A setting travels on one channel. It is not given on both the boot channel and
  the claim, and a boot-channel value never stands in for an absent claim field;
  what the relay does when an optional field is absent is a default named under
  GLOBAL-09 and GLOBAL-10 in `relaycfg`'s package doc.
- Launch wiring that is the same for every build of a driver and picks the
  relay's mode and build-side endpoint (`-mode`, `-fd`, `-build-if`) is how the
  relay is started, not configuration for a build.

BOOT-06 states the other half: no boot asset or image carries the boot-channel
values; they come per build. A relay setting the diff delivers outside the claim
is a finding under this rule; its fix is to move it into the claim, or, where
that needs a claim field the design did not plan, a return to Design (FLOW-06).
