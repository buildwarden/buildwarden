---
feature: test-rules-docs-only
status: draft
drivers: []
seams: []
surfaces: []
security_boundary: none
alters_relationships: false
design_review: required
native_validation: []
tier2_gaps: []
rules: [TEST-01, TEST-02, FLOW-14]
concepts: [testing, ci, docs-only]
related: [ci-path-filters.md]
---

# TEST-01 and TEST-02 allow docs-only changes to skip the Go suites

## Problem

`ci-path-filters` (#82) skips the Go lint, test, tidy and build jobs on PRs that
change only documentation that no test reads. TEST-01 still says `make test` "is
the gate every change passes", and TEST-02 says tier 0 runs "on every push to
`main` and every PR". Jeff accepted landing #82 against that wording and
rewording the rules separately (its a8).

## Prior art

- `ci-path-filters.md`, "Proposed rule changes", drafted the wording; its
  review added that Go source and module files count as code at any depth.

## Decisions

1. **Reword, don't weaken.** Every change that can affect `make test` still
   passes it, and every push to `main` runs it; only changes confined to paths
   no test reads skip it in CI. Rejected: dropping "every change" outright.
2. **Name the docs-only set once, in TEST-02,** and have TEST-01 refer to it,
   so the classifier in `main.yml` and the rule cannot drift apart silently.

## Questions

### Blocking

None.

### Assumed

None.

## Design

TEST-01, second sentence becomes:

> `make test` (pure-Go, table-driven unit tests) runs on every supported
> platform with no external dependencies and is the gate every change that can
> affect it passes: CI skips it only for changes confined to the docs-only paths
> in TEST-02, and every push to `main` runs it.

TEST-02, "Where each tier runs today", tier 0 becomes:

> Tier 0: the `ci` workflow, on every push to `main` and on every PR that
> changes a path outside the docs-only set: `docs/**` except
> `docs/development/**`, and top-level `*.md`, with Go source and module files
> (`*.go`, `go.mod`, `go.sum`, `go.work`) never docs-only wherever they are.
> `main.yml`'s `changes` job is that set's implementation.

## Cross-driver impact

None (`drivers: []`).

## Validation plan

`make lint`: tier 1's rule-index self-check parses the edited headers;
`TestRealIndex` passes. Wording only.

## Implementation plan

1. Edit TEST-01 and TEST-02 in `docs/development/testing.md` as above
   (TEST-01, TEST-02, FLOW-14).
2. `make lint` and `make test` (TEST-01).

## Design review

- **r1, 2026-10-31: approved** (Jeff, attended session, FLOW-19).
  Approved at `4e7365d`.

## Consolidation

Filled in at Publish.

## Rollback notes

## Archive summary

Filled in at Publish.
