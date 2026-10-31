---
feature: pre-release-cleanup
status: archived
drivers: [container, qemu, vz, hyperv]
seams: []
surfaces: []
security_boundary: none
alters_relationships: false
design_review: skip
native_validation: []
tier2_gaps: []
rules: [GLOBAL-01, GLOBAL-03, FLOW-12, FLOW-19, LINT-03, LINT-05, TEST-02]
concepts: [release-gate, tier1, doc-comments, error-wrapping, archive]
related: [tier1-static-checks.md, native-host-loop.md, native-loop-followups.md]
---

# Clear tier 1's release-range findings before the next tag

## Problem

`tier1-static-checks` (#79) adds a release job that runs tier 1 over everything
since the previous tag. Against v0.5.0 it fails with 34 MUST findings, so the
next release would be blocked (its review's MUST 3). Jeff chose to clear them
before tagging rather than start the release range at tier 1's first commit
(option (a), declined). Six of the findings went away with #77; this feature
clears the rest:

- 23 GLOBAL-01: exported identifiers without a doc comment, or with one not in
  Go's `// Name ...` form, in `driver/`, `driver/container`, `driver/qemu`,
  `driver/vz`, `driver/hyperv/capability.go` and `driver/winguest`.
- 3 GLOBAL-03: two `fmt.Errorf` in `relaycfg/` that wrap `ctx.Err()` with `%w`
  but format the last dial error with `%v`, so `errors.Is`/`errors.As` cannot
  see the network failure; and a `panic` in `driver.RandAlphaNum`.
- 2 FLOW-02/FLOW-12: `native-host-loop.md` and `native-loop-followups.md` are
  merged features whose designs were never archived, so they are `draft`
  without `design_review`.

## Prior art

- `tier1-static-checks.md` defines the checks and the release job; its review
  report `5460fee0e61e` records MUST 3 and Jeff's decision.
- `design-front-matter-retrofit.md` (#77) gave the six pre-standard designs
  front-matter, which cleared the other six FLOW-02 findings.
- FLOW-12 and the archived `container-inputs.md` set the archive summary's
  shape.

## Decisions

1. **Fix, do not suppress,** the 23 doc-comment and the two `%w` findings. They
   sit in the library packages third-party orchestrators embed, so the docs are
   read, and the dial error is what a caller needs to classify a failure.
   Driver methods that implement `driver.Driver` get `// Name implements
   driver.Driver.` style comments. Rejected: `Tier1-Allow` trailers (Jeff:
   fix what should be fixed).
2. **`RandAlphaNum`'s panic is the one exception,** marked inline with
   `//nolint:forbidigo` and its reason: since Go 1.24 the standard library
   treats a failing `crypto/rand` source as fatal, so the panic is not an
   ordinary failure, and an error return would burden every caller with a case
   that cannot be handled. Rejected: changing the signature to return an error.
3. **Archive both native designs** with their summaries filled from their own
   Decisions, Questions and Consolidation sections and their merge PRs (#64,
   #65), as FLOW-12 asks of any merged design. This overrides
   native-loop-followups' condition (its item 7 and plan step 12) of archiving
   native-host-loop only once the Windows desktop has pushed a worked scheduled
   run: both features are merged and the release gate needs them archived. The
   unmet Windows check moves to a `windows-hyperv` backlog entry,
   `native-host-loop-windows-scheduled-run`, so it stays tracked (TEST-03).
4. **No behavior change.** Comments, two format verbs, one directive, two
   design documents.

## Questions

### Blocking

None.

### Assumed

None.

## Design

As in Decisions; the file list is the Problem's. `design_review: skip` holds
under FLOW-19: no change to relationships, seams or a security boundary, no
blocking native validation, no assumed question, and no rule changes.

## Cross-driver impact

Doc comments only in `driver/container`, `driver/qemu`, `driver/vz` and
`driver/hyperv`, plus the shared `driver/` package, so every driver is declared
(TEST-02 caveat). No driver's behavior changes.

## Validation plan

Tier 0: `make lint` and `make test`. Then, with `tools/tier1` from #79 (merged
first, or built from its branch), `tier1 -base v0.5.0` and `tier1 designs -all`
exit 0 at this branch's head: the release gate the next tag will run.

## Implementation plan

1. Doc comments for the 23 GLOBAL-01 findings, in Go's `// Name ...` form
   (GLOBAL-01).
2. `relaycfg/relaycfg.go` and `relaycfg/tunnel.go`: wrap the last dial error
   with a second `%w` (GLOBAL-03).
3. `driver/subnet.go`: `//nolint:forbidigo` with the reason on
   `RandAlphaNum`'s panic (GLOBAL-03, LINT-05).
4. Archive `native-host-loop.md` and `native-loop-followups.md`: `status:
   archived`, Archive summary filled (FLOW-12, FLOW-19).
5. `make lint`, `make test`, and `tier1 -base v0.5.0` plus `tier1 designs
   -all` clean (LINT-03, TEST-02).

## Design review

Skipped (`design_review: skip`, FLOW-19); the criteria hold as stated in
Design. Scope agreed with Jeff in the session of 2026-10-31.

## Consolidation

`main` merged at `d266143` without conflicts.

- **Landed since the fork:** #78 (queue-summary: `wf` writes `QUEUE.md`) and
  #79 (tier1-static-checks). #79 is the gate this feature clears: with it
  merged in, `make lint` runs tier 1 over this branch (0 MUST), and the release
  gate itself, `tier1 -base v0.5.0` and `tier1 designs -all`, both exit 0 at
  this head. What remains in the release range is 3 SHOULD (gofmt in three
  files this feature does not touch, left alone per GLOBAL-02) and hints.
- **Terminology and abstractions:** nothing to reconcile; no code path is added.
- **FLOW-15 sweep:** no code added or removed, so no dead code; no duplication.
  `overview.md` unchanged (no flow changes).

## Rollback notes

## Archive summary

Implemented 2026-10-31 in the attended session that merged tier 1 (#79), as
the decision on that review's MUST 3.

- **Built.** Doc comments for the 23 GLOBAL-01 findings since v0.5.0 (driver
  methods and constructors on container, qemu and vz, `driver.ErrExecNotSupported`,
  `winguest.BuildGatewayIP` and `BuildGuestCIDR`, and the seven hyperv
  capability constants in Go's `// Name ...` form); the last dial error in
  `relaycfg`'s give-up errors wrapped with a second `%w`, with
  `TestGiveUpKeepsDialError`; an inline `//nolint:forbidigo` with its reason on
  `driver.RandAlphaNum`'s `crypto/rand` panic; `native-host-loop.md` and
  `native-loop-followups.md` archived with their summaries.
- **Decisions taken:** fix rather than suppress; one stated exception (the
  `crypto/rand` panic, fatal in the standard library since Go 1.24); archive
  native-host-loop ahead of native-loop-followups' Windows condition, moving
  that check to backlog `native-host-loop-windows-scheduled-run`.
- **Rejected:** starting the release range at tier 1's first commit (the
  review's option (a)); `Tier1-Allow` trailers for the doc comments; changing
  `RandAlphaNum` to return an error.
- **Validated.** `make lint` (with tier 1) and `make test`; the release gate
  `tier1 -base v0.5.0` and `tier1 designs -all` exit 0; the new relaycfg test
  fails with `%v` restored. Tier-2 reports `964fdeccd440` (0 MUST, 3 SHOULD)
  and `8ac5085fdb4b` (all resolved).
- **For future designs touching `release-gate` or `tier1`:** the next tag's
  gate is clean from v0.5.0; three gofmt SHOULDs remain in untouched files and
  will surface when those files are next edited.
