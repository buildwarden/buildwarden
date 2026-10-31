---
feature: specs-migration
status: draft
drivers: []
seams: [ledger-format]
surfaces: []
security_boundary: none
alters_relationships: false
design_review: required
native_validation: []
tier2_gaps: []
rules: [RELAY-05, GLOBAL-08, GLOBAL-04, GLOBAL-07, LINT-02, LINT-05, UX-01, TEST-02, FLOW-01, FLOW-02, FLOW-12, FLOW-14, FLOW-19]
concepts: [specs, ledger-spec, metadata-schema, environment-type, schema-id, schema-doc, tier1, mdbook]
related: [tier1-static-checks.md, ledger-environment-types.md, ci-path-filters.md]
---

# Move the ledger spec and schemas to `specs/` (RELAY-05)

## Problem

RELAY-05 sets a target layout for the ledger format's documents: one prose
spec per ledger major version under `specs/build-ledger/`, and each metadata
schema in its own `specs/metadata-<name>/` directory with its machine schema
(`v<N>.schema.json`) beside its document (`v<N>.md`). The rule says the
migration is separate work, and `docs/development/README.md` lists it as the
follow-up `specs-migration`.

Today the layout is the old one:

- `docs/book/src/design/ledger-spec.md` is one document. It covers the binary
  wire format, the field tables of all six default metadata schemas, and the
  environment types.
- `schemas/v1/*.json` holds nine schemas with no document beside them: the six
  metadata schemas the relay lists in every ledger header
  (`relay/ledger.go` `defaultSchemas`), the legacy
  `environment-container.json`, and the two environment payload schemas
  (`environment-oci-image.json`, `environment-vm-disk-chain.json`).
- Tier 1's `schema-doc` check (`tools/tier1/rules.yaml`) pairs all of
  `schemas/` with the whole spec. Because that spec is also prose for the
  binary format, "spec changed, no schema did" is only a hint. The
  co-location and same-major-version parts of RELAY-05 are not checked.

Until each schema has its own document, RELAY-05's "enforced to match" cannot
be checked per schema. `tier1-static-checks` deferred the reverse direction
to this feature.

## Prior art

- **RELAY-05** (`docs/development/relay.md`) sets the layout this design uses
  and the deterministic and agent-judgment split of "enforced to match".
- **`tier1-static-checks.md`** (archived) built `schema-doc` with its path
  pairs kept as data in `rules.yaml`, so that this feature would only need to
  change one table. Its history run (PR #62) demoted the reverse direction to
  a hint. Its Consolidation proposes that the reverse direction become a MUST
  once each `metadata-<name>/` schema is paired with its document. This design
  does that (Decision 6). It also reuses the existing `Tier1-Allow` trailer
  (LINT-05) for the rare prose-only fix.
- **`ledger-environment-types.md`** (archived) added the typed environment
  record, the `environment.json` metadata schema, the legacy
  `environment-container.json`, and one payload schema per environment type.
  Those payload schemas describe signed payload bytes, not record metadata,
  so this design gives them their own family (Decision 3). That design's rule
  stands: adding a type adds a section and a schema and leaves the wire format
  alone. After this feature, the "section" is a document under `specs/`.
- **`ci-path-filters.md`** (archived) classifies changed paths in
  `main.yml`'s `changes` job and builds the book on PRs. A new top-level
  `specs/` needs a classifier case (Decision 8).
- **GLOBAL-08** places the ledger format among the versioned seams, with
  RELAY-05's pre-1.0/post-1.0 posture. The format does not change here. Only
  its documents move, along with the places where third parties find them
  (Decision 2, a5).

All three related designs already carry front-matter. No FLOW-12 retrofit is
needed.

## Decisions

1. **Wire format and reader behavior stay byte-identical.** A ledger written
   after this feature has the same bytes as one written before. The relay's
   `defaultSchemas` strings and the `ledger/` reader do not change. The
   reader's environment detection (`strings.Contains(schema, "environment")`
   in `ledger/environment.go`) keeps working because the URIs keep their
   form. *Rejected:* rewriting the header URIs to the new paths. That would
   change every new ledger's header and every third-party reader that keys on
   the URI, for nothing gained.

2. **Schema `$id`s are stable names, not locations** (a2). Every existing
   `$id` stays `https://github.com/buildwarden/buildwarden/schemas/v1/<name>.json`,
   unchanged and kept forever, because ledgers already in the field carry
   these strings. They have never resolved: a GitHub file URL needs
   `/blob/<ref>/`. They are identifiers, and `specs/README.md` maps each one to
   its file. A schema added after this feature takes an `$id` that mirrors its
   path, `https://github.com/buildwarden/buildwarden/specs/<dir>/v<N>.schema.json`.
   *Rejected:* (a) new `$id`s for the moved files, which would break the match
   between the URIs in the header and the `$id`s; (b) making the URIs
   resolvable through Pages, which is a separate decision that can be queued
   as a follow-up.

3. **Directory mapping: one directory per schema** (a1). Moves are
   `git mv`, so history follows:

   | Today | After |
   |---|---|
   | `docs/book/src/design/ledger-spec.md` (wire format) | `specs/build-ledger/v1.md` |
   | `schemas/v1/http-open.json` | `specs/metadata-http-open/v1.schema.json` + `v1.md` |
   | `schemas/v1/http-headers.json` | `specs/metadata-http-headers/v1.schema.json` + `v1.md` |
   | `schemas/v1/http-body.json` | `specs/metadata-http-body/v1.schema.json` + `v1.md` |
   | `schemas/v1/artifact.json` | `specs/metadata-artifact/v1.schema.json` + `v1.md` |
   | `schemas/v1/redacted.json` | `specs/metadata-redacted/v1.schema.json` + `v1.md` |
   | `schemas/v1/environment.json` | `specs/metadata-environment/v1.schema.json` + `v1.md` |
   | `schemas/v1/environment-container.json` (legacy) | `specs/metadata-environment-container/v1.schema.json` + `v1.md` (marked legacy) |
   | `schemas/v1/environment-oci-image.json` | `specs/environment-oci-image/v1.schema.json` + `v1.md` |
   | `schemas/v1/environment-vm-disk-chain.json` | `specs/environment-vm-disk-chain/v1.schema.json` + `v1.md` |

   RELAY-05's `metadata-http/` example has no single counterpart. The layout
   allows one `v<N>.schema.json` per directory per version, and there are
   three HTTP schemas, each indexed separately in the header. *Rejected:* one
   `metadata-http/` holding three schemas, which breaks the one-pair-per-version
   rule that the check relies on.

   Environment payload schemas get an `environment-<type>/` family (a3). An
   environment type names its own major version (`oci-image/v1`), so it maps
   directly to `environment-oci-image/v1.*`. These schemas describe the signed
   payload, not unsigned record metadata, and the spec relies on that
   difference ("the payload is authoritative"). *Rejected:*
   `metadata-environment-<type>/`, which would file signed payload schemas
   under "metadata". Adding the family to RELAY-05's layout text is a proposed
   rule change (Proposed rule changes).

4. **Split the content.** `build-ledger/v1.md` keeps the binary format:
   header, record types and layout, hash block, signatures, schema index, the
   environment record's general rules ("What the record proves" and the
   payload-names-its-type rule), the HTTP mapping, redactability,
   completeness, verification, and the worked example. Its "Default Schemas"
   section becomes a table of index, name, `$id` and document link. Each
   per-schema field table, and each per-type section under "Environment
   types", moves into that schema's `v<N>.md`. The doc gives the schema's
   purpose, its `$id`, which records it applies to, the field table, and any
   compatibility notes, such as the legacy `container` label.
   *Rejected:* keeping the field tables in the ledger spec and adding thin
   documents next to each schema. The same information would then live in two
   places, and the pair check could not catch drift between them.

   While moving the text, fix one drift: the header example in the spec lists
   URIs without `/v1/` and leaves out `environment.json`. It will match
   `defaultSchemas` (RELAY-05, agent-judgment parity).

5. **The book publishes `specs/` without copies.** mdBook's `src` stays
   `docs/book/src`. A `docs/book/src/specs/` tree mirrors `specs/`. Each
   mirror page holds a single line, `{{#include ../../../../../specs/<dir>/v<N>.md}}`
   (or `../../../../specs/README.md` for the section index). Because the
   mirror keeps the same relative layout, links between specs written as
   `../metadata-http-open/v1.md` resolve both on GitHub and in the book.
   Links to `.schema.json` files use absolute
   `https://github.com/buildwarden/buildwarden/blob/main/specs/...` URLs,
   because the book does not copy the JSON. `SUMMARY.md` gets a "Specifications"
   part in place of the "Ledger Specification" entry. The old published URL
   `design/ledger-spec.html`, which `README.md` linked and third parties may
   have bookmarked, keeps working through `[output.html.redirect]` in
   `book.toml`. Heading anchors are unchanged because the headings are.
   *Rejected:* (a) a symlink, which fails GLOBAL-07 on Windows checkouts;
   (b) a CI copy step, which leaves a stale tree for local `mdbook serve`;
   (c) leaving the spec in the book and only moving schemas, which leaves
   RELAY-05's layout half done.

6. **`schema-doc` becomes per-directory, and both directions become MUST**
   (a4). `rules.yaml` stops listing path pairs and describes the layout:

   ```yaml
   # RELAY-05: under specs/, each <dir>/v<N>.schema.json has a v<N>.md beside
   # it, and the two change together. prose_only dirs hold documents only.
   schema_doc:
     root: specs/
     prose_only: [build-ledger]
   ```

   `SchemaDoc` then reports:

   - **Change together (MUST, both directions).** A changed
     `specs/<dir>/v<N>.schema.json` without its `v<N>.md`, or the reverse, in
     a directory that is not prose-only. The reverse direction was a hint only
     because one document covered both the binary format and every schema. A
     paired document describes only its schema, so RELAY-05's MUST applies as
     written. A legitimate prose-only fix carries a
     `Tier1-Allow: RELAY-05 specs/<dir>/*: <reason>` trailer (LINT-05).
   - **Co-located, same major (MUST), checked on HEAD's tree when the diff
     touches `specs/`, or with `-all`.** Every `v<N>.schema.json` has a
     `v<N>.md` and vice versa. A prose-only directory holds no
     `.schema.json`. Files under `specs/<dir>/` are named `v<N>.md` or
     `v<N>.schema.json`. `specs/README.md` is the only file allowed at the
     root.

   This is a check change, not a rule change: RELAY-05 already requires both
   directions and the hard fail. *Rejected:* listing nine explicit pairs in
   `rules.yaml`. A new schema directory would go unchecked until someone added
   a row, and "a new schema version with no matching doc is a hard fail"
   would not hold.

7. **A tier-0 test ties producer, specs and book together.**
   `specs/specs_test.go` (package `specs_test`, test-only) does three things.
   It builds a ledger with `relay.NewLedger` and reads its header with
   `ledger`, then checks that every URI in it is the `$id` of exactly one
   `specs/*/v*.schema.json`. It checks that each schema parses as JSON with a
   unique `$id` and the 2020-12 `$schema`. It checks that each
   `specs/**/*.md` has its mirror page in `docs/book/src/specs/` with the
   matching include, listed in `SUMMARY.md`. The test sits in `specs/` and
   uses only exported APIs, so `relay/` and `ledger/` stay untouched and
   ORCH-01's shared-package claim does not apply (a6). *Rejected:* putting the
   test in `relay/` or `ledger/`. A test file there would make tier 1's
   `drivers-claim` require every driver in the front-matter for a change that
   reaches none.

8. **CI and docs follow the move.** The `changes` job in `main.yml` gets a
   case `specs/*) book=true specs=true ;;`, and `specs=true` forces
   `gotests=true`, because the test above reads the schemas. Without that
   case, `specs/` would fall to `*/*) code=true`. The case goes before
   `*/*`; a `.go` file in `specs/` still matches `*.go` first. `docs.yml`
   adds `specs/**` to its push paths so the deployed book updates. Other
   references move to the new paths: `README.md` (both links, to
   `specs/build-ledger/v1.html`), `AGENTS.md` (the ledger wire format row),
   `docs/book/src/concepts/ledger.md`, `docs/book/src/contributing/structure.md`
   (add `specs/`), and the comment in `tools/tier1/coupling/checks.go`.
   Archived designs keep their old paths because they are history (FLOW-12).
   GoReleaser, the Dockerfiles and `install.sh` do not ship `schemas/`, so
   they are unaffected.

9. **`schemas/` is removed outright** (a5). Pre-1.0, with `$id`s kept,
   nothing resolves against the directory. The PR title uses a non-`docs:`
   prefix (for example `specs:`), so the GoReleaser changelog, which drops
   `^docs`, lists the move. *Rejected:* a `schemas/README.md` stub, which
   helps only human browsers and keeps a second top-level home that the rule
   says to fold away.

### Proposed rule changes (FLOW-14, not made on this branch)

For the human to make in a process item, or to fold into this feature's
merge:

- **RELAY-05:** (1) add `environment-<type>/v<N>.md` and `v<N>.schema.json`
  to the target layout, for the signed environment payload schemas;
  (2) state that a schema's `$id` is a stable name, not a location: the
  pre-migration `.../schemas/v1/<name>.json` ids stay for good, and new ids
  mirror their `specs/` path; (3) allow `specs/README.md` as the index;
  (4) replace "Performing this migration … is an implementation change
  separate from adopting the standard" with a statement that the layout is
  in place.
- **`docs/development/README.md`:** remove the "Follow-up — `specs/`
  migration" entry once this merges.
- **Optional sharpening:** RELAY-05's "prior versions preserved" could gain
  a deterministic half: deleting a `specs/<dir>/v<N>.*` file fails. This is
  not built here. Suggested as a follow-up.

## Questions

### Blocking

None.

### Assumed

- **a1 — Directory mapping.** Proceeding with one `specs/` directory per
  existing schema, named after the schema (`metadata-http-open/` and so on),
  as in Decision 3's table; confirm.
  Options: (a) one directory per schema [recommended]; (b) a grouped
  `metadata-http/` holding three schemas; (c) one directory per schema with
  short names (`metadata-open/`).
- **a2 — `$id` policy.** Proceeding with every existing `$id`, and the
  relay's header URIs, unchanged forever. New schemas take ids that mirror
  their `specs/` path. Confirm.
  Options: (a) keep the legacy ids, new ids mirror paths [recommended];
  (b) re-id everything to `specs/` paths and change the header URIs;
  (c) keep the legacy `schemas/v1/` form for new ids too.
- **a3 — Environment payload schemas.** Proceeding with an
  `environment-<type>/` family, with the RELAY-05 layout amendment proposed;
  confirm.
  Options: (a) `environment-<type>/` [recommended]; (b)
  `metadata-environment-<type>/`, which fits today's layout text literally;
  (c) keep the type schemas inside `build-ledger/`.
- **a4 — Reverse direction MUST.** Proceeding with `schema-doc`'s "document
  changed, schema did not" raised from hint to MUST for paired directories.
  A prose-only fix uses a `Tier1-Allow` trailer. Confirm.
  Options: (a) MUST, as RELAY-05 reads [recommended]; (b) keep it a hint.
- **a5 — Compatibility posture.** Proceeding with these choices. `schemas/`
  is removed with no stub. The old book URL redirects. Raw-file URLs under
  `schemas/v1/` stop working, noted in the release changelog. Confirm.
  Options: (a) remove, redirect the book page, changelog note
  [recommended]; (b) keep `schemas/v1/` copies for one release; (c) a
  `schemas/README.md` stub.
- **a6 — Front-matter.** Proceeding with these values, which count the seam
  but not a relationship change. Confirm.
  - `seams: [ledger-format]`, declared conservatively because the seam's
    contract documents and their published locations move, though its bytes
    do not.
  - `alters_relationships: false`: no component's role or interactions
    change, so FLOW-05's slot does not apply.
  - `drivers: []`: no driver tree or shared package is touched.

  Options: (a) as above [recommended]; (b) `seams: []`, since the format
  does not change; (c) also `alters_relationships: true`.

## Design

After the change:

```
specs/
  README.md                         index: layout, the $id -> file table, $id policy
  build-ledger/v1.md                binary wire format (prose only)
  metadata-http-open/v1.md          + v1.schema.json   header index 0
  metadata-http-headers/v1.md       + v1.schema.json   index 1
  metadata-http-body/v1.md          + v1.schema.json   index 2
  metadata-artifact/v1.md           + v1.schema.json   index 3
  metadata-redacted/v1.md           + v1.schema.json   index 4
  metadata-environment/v1.md        + v1.schema.json   index 5
  metadata-environment-container/v1.md + v1.schema.json  legacy, pre-typed ledgers
  environment-oci-image/v1.md       + v1.schema.json   payload of oci-image/v1
  environment-vm-disk-chain/v1.md   + v1.schema.json   payload of vm-disk-chain/v1
  specs_test.go                     tier-0 conformance (Decision 7)
docs/book/src/specs/...             one-line {{#include}} mirrors of every .md above
```

The format flows are unchanged. The relay writes the same header and
records, and `warden inspect` reads them the same way. `overview.md`'s
pseudocode has no flow to update (FLOW-15).

**Changing a schema after this lands:** edit `v<N>.schema.json` and
`v<N>.md` together. Tier 1 fails if one changes without the other. A new
major version adds `v<N+1>.schema.json` and `v<N+1>.md` beside the old pair,
which stays. A new metadata layout adds a directory, a book mirror page and
a `SUMMARY.md` entry. `specs_test.go` fails if the mirror or the entry is
missing. A new ledger major adds `build-ledger/v2.md`.

**Tier 1 after the change:** see Decision 6. `rules.yaml`'s `schema_doc`
changes from a list of `{code, doc}` pairs to `{root, prose_only}`, and the
`Config` type in `tools/tier1` changes with it. This feature's own PR adds
every pair and its document together, and removes `schemas/` (outside
`root`), so it passes its own check.

## Cross-driver impact

None. No file under `driver/` changes, and neither do the shared packages
(`relay/`, `ledger/`, `collector/`, `relaycfg/`, `bootstrap/`). Every driver
writes, through the relay, a ledger that is byte-identical before and after.
The environment payload each driver asserts (`oci-image/v1` for container,
`vm-disk-chain/v1` for qemu, vz and hyperv) keeps its schema content. Only
the schema's file location changes.

## Validation plan

- **Tier 0 (TEST-02):** `specs/specs_test.go` (Decision 7). In
  `tools/tier1/coupling/checks_test.go`, scratch-repo cases for the new
  `SchemaDoc`:
  - a schema without its document, and a document without its schema (both
    MUST);
  - both changed (clean);
  - `build-ledger/v1.md` alone (clean);
  - a `v2.schema.json` added with no `v2.md` on HEAD (MUST);
  - a `.schema.json` in `build-ledger/` (MUST);
  - a stray file name (MUST);
  - a `Tier1-Allow` suppression applied.

  The existing `schemas/` cases are replaced.
- **Tier 1:** `make lint` on the branch. This includes tier 1 against its
  own diff and the `rules.yaml` self-check. `tools/tier1` tests run on
  Linux, macOS and Windows in CI (`tier1-test`, GLOBAL-07), because the
  check compares paths.
- **Book:** `mdbook build docs/book` (v0.4.44, as CI pins) succeeds. The
  rendered `specs/build-ledger/v1.html` has the cross-links resolved. The
  redirect page `design/ledger-spec.html` exists and forwards to it. The
  plan verifies the include and redirect behavior first (task 1), because
  this session could not run mdBook.
- **Native validation:** none. No driver or host behavior changes, so no
  host is blocking and none is tracked (FLOW-09). `tier2_gaps` is empty.

## Implementation plan

1. **Prove the book mechanism** (RELAY-05, GLOBAL-07, UX-01). With mdBook
   0.4.44, confirm on a scratch book three things: `{{#include}}` reads a
   file outside `src`; relative `.md` links in included content resolve
   through a mirrored tree; and `[output.html.redirect]` emits the old page.
   If the include path is refused, return to Design with the finding
   (FLOW-06). Do not switch to a symlink or a copy on your own.
2. **Move the files with history** (RELAY-05). `git mv` each schema and the
   spec to its Decision 3 path, in a commit with no content edits, so blame
   and rename detection follow. Each `$id` stays byte-identical (Decision 2).
3. **Split the spec** (RELAY-05, UX-01).
   - Cut each per-schema field table and each environment-type section from
     `build-ledger/v1.md` into its `v1.md`, adding purpose, `$id`, the
     records it applies to, and its schema link.
   - Replace "Default Schemas" with the index table.
   - Fix the header example's URIs to match `defaultSchemas`.
   - Mark `metadata-environment-container/v1.md` legacy, with the
     `container`-label compatibility note.
   - Write `specs/README.md` (layout, `$id` to file table, `$id` policy).
   - Check field-level parity of each document against its schema
     (RELAY-05 agent-judgment).
4. **Book mirror and redirect** (UX-01, RELAY-05).
   - Add `docs/book/src/specs/` mirror pages.
   - Replace the `SUMMARY.md` entry with a "Specifications" part.
   - Add the `book.toml` redirect `"/design/ledger-spec.html" = "../specs/build-ledger/v1.html"`.
   - Remove `docs/book/src/design/ledger-spec.md`.
   - `mdbook build docs/book` passes.
5. **Tier-0 conformance test** (RELAY-05, TEST-02, GLOBAL-04).
   `specs/specs_test.go` as in Decision 7, using only exported `relay` and
   `ledger` APIs. Confirm it fails when a header URI has no file, when an
   `$id` is duplicated, and when a mirror page is missing.
6. **Tier 1 `schema-doc`** (RELAY-05, LINT-02, LINT-05, GLOBAL-07).
   - Change `rules.yaml` `schema_doc` to `{root: specs/, prose_only:
     [build-ledger]}` and update the config type and its loader.
   - Rewrite `SchemaDoc` per Decision 6, with both directions MUST and the
     HEAD-tree co-location check. List the tree with git, not the
     filesystem, using forward-slash paths.
   - Update the `checks.go` comment.
   - Replace the `checks_test.go` cases per the Validation plan.
   - `go test ./...` in `tools/tier1`.
7. **CI classifier and docs deploy** (TEST-02, FLOW-01). In `main.yml`'s
   `changes` job, add the `specs/*` case before `*/*` and make `specs=true`
   force `gotests=true`. In `docs.yml`, add `specs/**` to the push paths.
8. **References** (UX-01, FLOW-12). Point every remaining reference at the
   new paths, using `grep -rn 'schemas/\|ledger-spec'` outside `docs/design/`
   to find them:
   - `README.md` (both links), `AGENTS.md`;
   - `docs/book/src/concepts/ledger.md`;
   - `docs/book/src/contributing/structure.md` (add `specs/` to the tree and
     the package list).

   Leave archived designs as they are.
9. **Remove `schemas/`** (RELAY-05). Drop the empty directory after task 2.
   Confirm nothing outside `docs/design/` names it.
10. **Gate** (FLOW-01, FLOW-17). `make test` and `make lint` green,
    including tier 1 on this diff. Push.

## Design review

## Consolidation

## Rollback notes

## Archive summary
