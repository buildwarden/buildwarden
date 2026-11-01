---
feature: http-body-path-drift
status: draft
drivers: [container, qemu, vz, hyperv]  # relay/ is shared (FLOW-02, TEST-02 caveat); no driver tree changes
seams: [ledger-format]                  # the metadata bytes the relay writes into ledgers change; the v1 contract does not
surfaces: []
security_boundary: env-relay            # GLOBAL-06: any change to the relay itself; posture unchanged, see Security note
alters_relationships: false             # the producer is brought back to the existing v1 schema; no role or seam contract changes
design_review: required                 # security_boundary, seams and assumed questions all rule out skip (FLOW-19)
native_validation: []                   # the change is platform-neutral relay code, fully exercised by tier 0 (see Validation plan)
tier2_gaps: []
rules: [RELAY-01, RELAY-02, RELAY-04, RELAY-05, GLOBAL-06, GLOBAL-08, TEST-01, TEST-02, FLOW-02, FLOW-07, FLOW-19]
concepts: [ledger, metadata-schema, http-body, relay-served-inputs, schema-conformance, specs]
related: [specs-migration.md, ledger-environment-types.md, relay-leaf-crl.md]
---

# Relay writes a `path` field under `http-body`, whose schema forbids extra fields

## Problem

`relay/relay.go` `recordServed` records each relay-served input (the build
script, the bootstrap config, and every build-context file fetched from
`http://cwd/<path>`) as an `OPEN` record under `http-open` and a `CLOSE` record
under `http-body` (header index 2). The close metadata it writes is

```go
closeMeta, _ := cbor.Marshal(map[string]any{"path": path})
```

but [`specs/metadata-http-body/v1.schema.json`](../../specs/metadata-http-body/v1.schema.json)
allows only `status` and sets `additionalProperties: false`, and its document
says "No other fields are allowed". Every ledger the relay writes therefore
carries metadata that does not conform to the schema its own header names.
A third-party reader that validates metadata against the published schemas,
which RELAY-05 says is all it should need, rejects or flags these records.

The drift was found by the specs-migration review and refuted there as
pre-existing (see the archived [`specs-migration.md`](specs-migration.md),
Consolidation and Archive summary). History: `b716d9e` (2026-09-24) introduced
`{"path": "build-script"}`, and `4ea4f50` (2026-10-11, diskless relay) widened
it to the bootstrap config and every context file. v0.5.0 had no such field;
**v0.6.0 (2026-10-31) is the only release whose ledgers carry it**.

The decision to make is whether the schema moves to v2 to admit `path`, or the
producer stops writing it.

## Prior art

- **RELAY-05** (relay.md): one document per major schema version, prior
  versions kept, the reader stays backward-compatible, and a schema's `$id` is
  a stable name that never changes. The relay is the reference implementation
  of the specs, not the other way round.
- **`specs-migration.md`** (archived, `concepts: specs, metadata-schema,
  schema-id, schema-doc`): established the `specs/<dir>/v<N>.md` +
  `v<N>.schema.json` layout, kept every pre-migration `$id` for good, and says
  a new schema version is a new pair with a new `$id`, never an edited one.
  It also made tier 1's `schema-doc` check per directory, both directions MUST,
  with a `Tier1-Allow` trailer for the rare prose-only fix to one half. This
  design reuses all three.
- **`ledger-environment-types.md`** (archived, `concepts: ledger,
  environment-record`): the precedent for adding a schema under the
  `environment-<type>` naming while leaving older ids and their documents in
  place for ledgers already written (`metadata-environment-container` kept as
  legacy). Option C below would follow it; this design does not need to.
- **`relay-leaf-crl.md`** (archived): the precedent for front-matter on a
  relay-only change: all four drivers declared because `relay/` is shared,
  `security_boundary: env-relay` for GLOBAL-06.
- **The ledger spec** ([`specs/build-ledger/v1.md`](../../specs/build-ledger/v1.md),
  Metadata): metadata is CBOR, never part of the signature input, and "can be
  freely added, modified, or stripped without affecting ledger validity". So
  neither option touches signature verification of old or new ledgers.
- **Other `http-body` producers** in `relay/relay.go` already conform: the
  request-body checkpoint writes `{}`, and upstream response closes (streamed
  and bodiless) write `{"status": <code>}`. Only `recordServed` deviates.

No archived design depends on `path`; `cmd/warden inspect` (`metaSummary`)
reads only `method`, `url`, `name` and `reference`, and no other code in the
tree reads close metadata under `http-body`.

## Decisions

### D1: change the producer, not the schema (assumed a1)

`recordServed` writes `{"status": 200}` under `http-body` v1 instead of
`{"path": ...}`.

The `path` value carries no information the ledger lacks: the matching `OPEN`
record's `http-open` metadata already holds the full request URL
(`http://cwd/src/main.c`, `http://artifacts/build-script`,
`http://artifacts/bootstrap`), and `path` is that URL's path with the leading
slash removed. A relay-served input is a response body the relay sent with
status 200 (it serves nothing else; failures are 404 and are not recorded), so
`status: 200` is exactly what the v1 document asks a response body to carry,
and it makes served inputs look like every other response body to a reader.

Alternatives:

- **B. Empty map `{}`.** Also conforms, but the v1 document reserves the empty
  map for request bodies ("a request body's metadata is an empty map"), so a
  reader could no longer tell a served input from an uploaded request body by
  metadata alone. Lost to A.
- **C. Schema v2 admitting `path`.** A new `specs/metadata-http-body/v2.md` +
  `v2.schema.json` with a new `$id`
  (`https://github.com/buildwarden/buildwarden/specs/metadata-http-body/v2.schema.json`),
  the relay's header list and index 2 switching to it, and the ledger spec's
  Default Schemas table, `specs/README.md` and `specs_test.go` updated. This
  changes a versioned seam's contract (GLOBAL-08) for a redundant field, would
  make the change `alters_relationships: true` and take FLOW-05's single slot,
  and still leaves the v0.6.0 ledgers non-conforming to the v1 schema they name
  (a header cannot be re-pointed after the fact without breaking index
  stability for other readers). Lost: high cost, no information gained.
- **D. Widen v1 in place** (add `path` to the v1 schema). Forbidden by
  RELAY-05: a published version's meaning does not change under ledgers already
  written against it. Rejected.

### D2: declare `seams: [ledger-format]` with `alters_relationships: false`

The bytes the relay writes into a ledger's metadata change, so FLOW-02's
conservative derivation declares the ledger-format seam. The seam's contract,
the v1 schemas and the ledger spec, does not change: the producer is brought
back into conformance with it. That is a change inside the relay's existing
role, so `alters_relationships` is `false` (the same pairing the archived
`specs-migration.md` used).

### D3: document the v0.6.0 deviation where readers look (assumed a2)

`specs/metadata-http-body/v1.md` gains a short "Known producer deviation" note:
ledgers written by the v0.6.0 relay (and unreleased builds from `b716d9e`
onward) carry a `path` text field in `http-body` close records of relay-served
inputs; it duplicates the path of the matching `OPEN` record's `url`, readers
should ignore it, and current relays do not write it. The schema file does not
change, so the commit carries a `Tier1-Allow: RELAY-05
specs/metadata-http-body/v1.md: ...` trailer, the route `schema-doc` provides
for a prose-only fix to one half of a pair.

### D4: guard against the next drift with a conformance test (assumed a3)

A relay test runs the existing end-to-end collector scenario, reads back every
record of the landed ledger, resolves each record's schema index through the
header's `schemas` list to its `specs/*/v*.schema.json` file by `$id`, and,
for a schema with `additionalProperties: false`, fails on any top-level
metadata key not in its `properties`. It needs no JSON Schema dependency and
would have caught this bug; it covers every metadata producer the scenario
reaches, not only `recordServed`.

## Questions

### Blocking

None. The schema-vs-producer choice is settled as assumption a1 because the
producer change is the one that leaves every published contract intact and is
trivially reversible; the design review confirms it.

### Assumed

- **a1. Producer change writing `{"status": 200}` (D1).** Options: (A) producer
  writes `{"status": 200}`; (B) producer writes `{}`; (C) schema v2 admitting
  `path`, new `$id`, header index 2 moves to it; (D) widen v1 in place.
  Recommended: A. Proceeding with A; confirm.
- **a2. Note the v0.6.0 deviation in `specs/metadata-http-body/v1.md` (D3).**
  Options: (A) note in the http-body v1 document, with a `Tier1-Allow` trailer
  since its schema is unchanged; (B) note in `specs/build-ledger/v1.md`
  (prose-only, no trailer needed, but away from the field table readers
  consult); (C) no note, relying on the release notes. Recommended: A.
  Proceeding with A; confirm.
- **a3. Add a shallow metadata-conformance test over the relay's end-to-end
  ledger (D4).** Options: (A) top-level key check against each schema's
  `properties` where `additionalProperties: false`, no new dependency; (B) full
  JSON Schema validation with a new third-party validator dependency; (C) only
  a targeted assertion that served-input closes carry `{"status": 200}`.
  Recommended: A (it includes C's assertion). Proceeding with A; confirm.

## Design

Before:

```
recordServed(req, data, path):
    open  = ledger.Open(http-open, {method: GET, url: req.URL, protocol})
    ledger.Close(open, len(data), hash(data), http-body, {path: path})   # not in v1
```

After:

```
recordServed(req, data):
    open  = ledger.Open(http-open, {method: GET, url: req.URL, protocol})
    ledger.Close(open, len(data), hash(data), http-body, {status: 200})  # v1-conformant
```

The three callers (`handleBuildScript`, `handleBootstrap`, `handleContextGet`)
drop the third argument. Nothing else in the relay changes: the served bytes,
the `OPEN` record, the hash block, the record order and the header's `schemas`
list are identical, and the ledger stays a v1 ledger naming the same six
`$id`s.

Readers: `warden inspect` is unaffected (it never read `path`). A reader of a
v0.6.0 ledger sees the extra key and, per the D3 note, ignores it; signature
verification never involved metadata.

`docs/book/src/concepts/overview.md` needs no change: its relay pseudocode says
served inputs are "recorded" and does not describe their metadata, and the
flow is unchanged (FLOW-15). No user-facing surface changes (UX-01).

## Cross-driver impact

`relay/` is shared, so all four drivers are declared (FLOW-02). For each of
container, qemu, vz and hyperv the guest fetches the build script, bootstrap
config and context files from the same relay endpoints, and the only
difference it can observe is none: the HTTP response is byte-identical
(RELAY-02). The ledger each driver's build produces differs only in those
close records' metadata. No driver tree changes. RELAY-04: the change sits in
mode-independent request handling, so container, vm and host modes are
touched identically. Firecracker has no driver yet.

## Security note

**Boundary.** env-relay (GLOBAL-06 fires on any change to the relay). The
change alters what the relay writes into the ledger, the security document of
the default stack, for inputs it serves across that boundary.

**Posture.** Unchanged. The bytes sent to the build and the hash recorded for
them are identical (RELAY-02), the change goes through `Ledger.Close` on the
single writer (RELAY-01), and no key, CA or filtering code is touched
(RELAY-03, RELAY-04). Metadata is outside the signature input, so the change
can neither weaken nor strengthen tamper evidence. No information is lost: the
dropped `path` is derivable from the matching `OPEN` record's `url`, which is
metadata too and is the field consumers already read. The fix
removes a way for a strict reader to reject an honest ledger, which is a small
gain in verifiability.

**TEST-08.** Nothing new for the security suite to exercise; the conformance
test (D4) is tier 0.

## Validation plan

- **Tier 0** (TEST-01, TEST-02): the D4 conformance test over the relay's
  end-to-end collector scenario, which reaches all three served-input paths
  plus upstream-free artifact and heartbeat traffic; existing relay and
  `specs/specs_test.go` tests stay green. `make test`, `make lint`.
- **Tier 1**: `schema-doc` passes with the `Tier1-Allow` trailer on the
  prose-only `v1.md` edit.
- **Tier 2 / native** (FLOW-09): none required, nothing deferred. The relay
  always runs as Linux code (container, Alpine relay VM, Hyper-V relay UKI),
  `recordServed` has no platform-specific branch, and the tier-0 test drives
  the real code path in process. No host is blocking and no gap goes to the
  native backlog; the release full matrix remains the backstop.

## Implementation plan

1. **Producer fix** (rework of existing code, FLOW-06): in `relay/relay.go`,
   `recordServed` drops its `path` parameter and writes
   `{"status": http.StatusOK}` under `schemaHTTPBody`; update its doc comment
   and the three callers. Rules: RELAY-01 (write stays on `Ledger.Close`),
   RELAY-02 (served bytes unchanged), RELAY-05 (conforms to v1).
2. **Conformance test** (D4): a relay test that reuses the
   `relay_collector_test.go` harness, reads the landed ledger with the
   `ledger` package, maps header `$id`s to `specs/*/v*.schema.json` files, and
   checks top-level keys for schemas with `additionalProperties: false`; plus
   an explicit assertion that the three served-input closes carry
   `{"status": 200}`. Rules: TEST-01, TEST-02, TEST-04 (deterministic),
   GLOBAL-07 (`io/fs`-style forward-slash paths as `specs_test.go` does).
3. **Spec note** (D3): add the "Known producer deviation" paragraph to
   `specs/metadata-http-body/v1.md`; commit with
   `Tier1-Allow: RELAY-05 specs/metadata-http-body/v1.md: prose-only erratum for v0.6.0 ledgers; schema unchanged (http-body-path-drift)`.
   Rules: RELAY-05 (documents stay accurate for ledgers already written).
4. `make test`, `make lint`; push the branch (FLOW-17).

## Design review

Pending (`design_review: required`).

## Consolidation

Filled in at Publish (FLOW-10, FLOW-15).

## Rollback notes

None.

## Archive summary

Filled in at Publish (FLOW-12).
