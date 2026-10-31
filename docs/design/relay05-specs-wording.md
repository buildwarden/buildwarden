---
feature: relay05-specs-wording
status: draft
drivers: []
seams: []
surfaces: []
security_boundary: none
alters_relationships: false
design_review: required
native_validation: []
tier2_gaps: []
rules: [RELAY-05, FLOW-14]
concepts: [specs, schemas, ledger-format, rules]
related: [specs-migration.md]
---

# RELAY-05 matches the `specs/` layout as built

## Problem

`specs-migration` moves the ledger spec and schemas into `specs/`. RELAY-05
still describes that move as future work, has no place for the signed
environment payload schemas, and says nothing about what a schema's `$id`
means after files move. The design proposed four wording changes (its
"Proposed rule changes"); this item makes them, since rules change only
through the human (FLOW-14).

## Prior art

- `specs-migration.md`, Decisions 2 to 3 and "Proposed rule changes",
  approved at its design review r1 (2026-10-31).

## Decisions

1. **Land after `specs-migration` merges,** so the rule never describes a
   layout that is not on `main`. Rejected: folding the wording into that
   feature's PR (FLOW-14 keeps rule changes in a process item).
2. **The four changes, nothing more:** the `environment-<type>/` family in
   the target layout; the `$id` policy (a stable name, not a location; the
   pre-migration `.../schemas/v1/<name>.json` ids stay for good; new ids mirror
   their `specs/` path); `specs/README.md` allowed as the index; the "migration
   is separate" paragraph replaced by a statement that the layout is in place.
   Plus removing the `docs/development/README.md` follow-up entry.

## Questions

### Blocking

None.

### Assumed

None.

## Design

In `docs/development/relay.md`, RELAY-05:

- Target layout adds:
  ```
    environment-<type>/
      v1.md              signed environment payload schema documentation
      v1.schema.json
    README.md            index of the specs
  ```
  and the paragraph after it: "`environment-<type>/` holds the schemas of
  signed environment payloads (OCI image, VM disk chain), paired like
  `metadata-<name>/`."
- New paragraph, **Schema identity:** "A schema's `$id` is a stable name, not a
  location. Ids already written into ledgers, including the pre-migration
  `https://github.com/buildwarden/buildwarden/schemas/v1/<name>.json` forms,
  never change; new schemas take ids that mirror their `specs/` path."
- The final paragraph ("Performing this migration ... See the README's
  follow-up list.") becomes: "The layout is in place: the ledger spec and every
  schema live under `specs/`, and tier 1's `schema-doc` check enforces the
  deterministic half."

In `docs/development/README.md`: remove "Follow-up — `specs/` migration".

## Cross-driver impact

None.

## Validation plan

`make lint`: tier 1's rule-index self-check re-parses RELAY-05; `make test`
(`TestRealIndex`).

## Implementation plan

1. Edit RELAY-05 as above and drop the README follow-up, after
   `specs-migration` merges (RELAY-05, FLOW-14).
2. `make lint`, `make test` (TEST-01).

## Design review

Pending.

## Consolidation

Filled in at Publish.

## Rollback notes

## Archive summary

Filled in at Publish.
