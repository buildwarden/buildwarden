---
feature: rule-paths-shared
status: draft
drivers: []              # the loop's check code only (tools/workflow/check); no driver tree is reached
seams: []                # no GLOBAL-08 seam changes
surfaces: []             # `wf` is the loop's tool, not a user-facing surface of the product
repos: []                # only buildwarden/buildwarden
areas: [loop, designs]   # tools/workflow/check/rulechange.go and its test; this design
security_boundary: none
alters_relationships: false  # no product component's role or interaction changes (FLOW-02)
design_review: required  # a process item's design always gets its review (FLOW-03, FLOW-19)
native_validation: []    # a pure function of the design text and git plumbing
tier2_gaps: []
rules: [FLOW-03, FLOW-14, FLOW-19, LINT-05, LOOP-09, GLOBAL-10, TEST-01, TEST-02]
rule_changes: []         # FLOW-14's enforcement changes (check/rulechange.go), not its wording;
                         # a process item's branch is exempt from the clearance (FLOW-14)
concepts: [workflow, rule-changes, rule-clearance, wf-drops, enforcement, approvals]
related: [design-approved-rule-changes.md, merge-confirm-attended.md, loop-repository.md]
---

# wf check clears a path for every approved rule change that lists it

A process item (FLOW-03, LOOP-09). It holds no worker. It fixes one function
in `wf`'s checks, found by driver-contract's review on 2026-11-20, and it
blocks that item's Review until it merges.

Code is `main` at `8531790`.

## Problem

`cleared()` (`tools/workflow/check/rulechange.go:75-108`) builds a map from
each path an approved rule change may edit to the rule that clears it:

```go
for _, p := range rc.Paths {
	paths[p] = rule
}
```

The map holds one rule per path. When two approved rule changes list the
same file, the one read last wins and the others are lost. Two callers read
the map:

- **The rule clearance** (`rulechange.go:188-238`, the Implement and Review
  gates, FLOW-14) asks only whether a path is cleared, so it passes, but
  its "cleared:" line names one rule where several clear the file.
- **`wf drops`** (`approvedRules`, `rulechange.go:332-356`, run by `make
  enforcement`, LINT-05) collects the map's values as the rules the
  branch's approved changes name. A rule whose every path is shared with a
  later rule change is missing, so a tier-1 finding that its approved
  change drops is refused: "dropped, and no approved rule change names
  <rule>".

That is what happened on `feat/driver-contract` at `dd370cb`. UX-01, UX-03
and ORCH-03 are each approved by name at `a6c8f0b`, and all three list
`tools/tier1/surface/surface.go`. Only ORCH-03 is kept, so `make
enforcement` refuses the four UX-03 `surface-removed` drops that UX-03's
approved enforcement change makes (the surface reader reads a setting's
`Aliases` as keys). `dist/wf check` shows the same loss: `surface.go
(ORCH-03)`. The branch cannot fix it: `enforcement-check.sh` builds `wf
drops` from the merge base, and a feature branch may not edit
`tools/workflow/` (FLOW-14).

## Prior art

- **`design-approved-rule-changes.md`** made each rule change approved by
  name and wrote `cleared()`. Nothing then had two rule changes on one
  path.
- **`merge-confirm-attended.md`** changed how approvals are read per review
  entry. It did not touch `cleared()`.

## Decisions

1. **`cleared()` returns every rule that clears a path** (a1):
   `map[string][]string`, each list sorted and without duplicates. A path
   is cleared when its list is not empty.
2. **The clearance names every rule** that clears a path: "cleared:
   tools/tier1/surface/surface.go (ORCH-03, UX-01, UX-03)".
3. **`wf drops` counts every rule** in every list, so a rule is named when
   any of its approved paths is in the map.

No rule wording changes: FLOW-14 already says a change is cleared by an
approved rule change, and LINT-05 that a dropped finding must be on a rule
an approved change names. This makes the checks do what the rules say.

## Questions

### Blocking

None.

### Assumed

- **a1. How the shared path is recorded.**
  - (a) A list of rules per path, sorted and deduplicated (Decisions 1-3).
  - (b) Keep the one-rule map for the clearance, and have `approvedRules`
    read the rule names straight from each approved entry instead of
    through the map.

  Recommended: (a). One shape for both callers, and the clearance line
  stops hiding the other rules. (b) is about as small but leaves the
  clearance line naming one rule of three.

## Design

- `cleared()`: `paths map[string][]string`; for each approved entry's path,
  append the rule unless the list already has it; sort each list before
  returning.
- `ruleClearance` (`rulechange.go:188-238`): `len(ok[prefix+p]) == 0` is "in
  no approved rule change"; otherwise the good line prints
  `p + " (" + strings.Join(ok[prefix+p], ", ") + ")"`.
- `approvedRules` (`rulechange.go:350-353`): `for _, rules := range paths {
  for _, r := range rules { out[r] = true } }`.

## Defaults and overrides

None: no setting, no fallback.

## Cross-driver impact

None.

## Rule changes

None (`rule_changes: []`). The change is to FLOW-14's and LINT-05's
enforcement in `tools/workflow/check/`, on the process route (FLOW-14,
LOOP-09).

## Validation plan

- **Tier 0** (`make test`, TEST-01, TEST-02), in `check/rulechange_test.go`:
  - A design with two approved rule changes that both list one path: the
    clearance passes and names both rules, sorted.
  - `TestDrops`: a branch whose tier 1 drops a finding on the first of two
    rules sharing every path passes (it fails on `main`'s code, the
    driver-contract shape).
  - The existing clearance and `TestDrops` cases pass unchanged.
- `make lint`, `make test`, `make enforcement`,
  `tools/workflow/flow18-check.sh`.
- **The real check:** driver-contract's Review, re-run after this merges,
  shows `surface.go (ORCH-03, UX-01, UX-03)` and `make enforcement` green.
- No native host is needed.

## Implementation plan

A process item has no Plan phase; this is the work of `x2-implement`.

1. Tests first in `check/rulechange_test.go`, seen failing on `main`'s code.
2. `check/rulechange.go`: `cleared()`, `ruleClearance` and `approvedRules`
   as under **Design**.
3. `make test`, `make lint`, `make enforcement`, `flow18-check.sh`.
4. Archive this design with its summary and open the PR with CI green
   (FLOW-03 process phase 2, FLOW-12).

No state step: nothing on the `workflow` branch changes.

## Design review

Pending.

## Consolidation

A process item has no Publish phase and no consolidation pass (FLOW-03).

## Rollback notes

None.

## Archive summary

Filled in at `x2-implement` (FLOW-12).
