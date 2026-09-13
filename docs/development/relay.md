# Relay Standards

Standards for the relay: the `relay/` library and the `cmd/relay/` binary. The
relay is the MITM proxy, DNS resolver, ledger writer, control plane, and fairness
scheduler. It is the most security- and correctness-sensitive component, and it
sits on the path of every byte of build traffic.

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
```

`build-ledger/` is prose only (it describes a binary wire format, with no JSON
counterpart). Each `metadata-<name>/` carries the doc and the machine schema
together.

**Enforced-to-match**, split by what is checkable:

- Deterministic: a machine schema and its doc are co-located, share the major
  version, and CI fails if one changes in a commit without the other. A new schema
  version with no matching doc, or vice versa, is a hard fail.
- Agent-judgment: field-level parity, i.e. the doc actually describes what the
  schema enforces and has not drifted in meaning.

Performing this migration (moving the existing ledger spec, folding the current
top-level `schemas/` into `specs/`) is an implementation change separate from
adopting the standard. See the README's follow-up list.

---

### RELAY-06 — Hot-path performance

**Severity:** SHOULD
**Check:** automated + agent-judgment

The relay is on the path of every byte of build traffic. Hot-path changes run
against the relay end-to-end benchmark and justify any regression past a set
threshold.
