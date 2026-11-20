---
# Copy this file to docs/design/<feature>.md. See FLOW-01, FLOW-02 and FLOW-12 in
# docs/development/process.md. Delete comments once filled in.
feature: <feature-slug>
status: draft            # draft | active | archived
drivers: []              # container, qemu, vz, hyperv, firecracker; or [interface]
seams: []                # e.g. relay-claim, collector, executor-initialize, ledger-format,
                         # bootstrap-model (BOOT-07: needs the Bootstrap comparison below).
                         # Not empty: fill Specification changes below (LOOP-02)
surfaces: []             # e.g. cli, config, output
repos: []                # every repository the item works in, home first, when there is
                         # more than this one (LOOP-04); empty or absent: only this one
merge_first: []          # repositories in repos, not the home, whose PR merges before the home's is ready (FLOW-08); empty or absent: none
# areas: []              # the areas the change edits (FLOW-02), by the names of the
                         # area map (`wf areas -list`). Left out, the item is undeclared
                         # and collides with every item in its repositories (FLOW-05);
                         # []: the change edits nothing but its own design. A bare name
                         # is the home's map; <owner>/<name>:<area> another repository's
security_boundary: none  # none | host-env | env-relay | env-beyond (GLOBAL-06)
alters_relationships: false  # true if a component's role or interactions change (FLOW-02, FLOW-05)
design_review: required  # required | skip (FLOW-19: skip only when its criteria hold)
native_validation: []    # - {host: windows-hyperv, blocking: true, reason: "..."}
tier2_gaps: []           # platform validation knowingly deferred (TEST-03)
rules: []                # rule IDs relied on or changed, e.g. ORCH-01, UX-01
rule_changes: []         # FLOW-14: each rule change this feature makes, e.g.
                         # - {rule: BOOT-03, change: amend, paths: [docs/development/bootstrap.md]}
                         # change: add | amend | retire; paths: every file it edits
concepts: []             # archive retrieval tags, e.g. collector, claim, image-prepare
related: []              # other docs/design/*.md this builds on or supersedes
---

# <Feature title>

## Problem

What is wrong or missing, for whom, and why now.

## Prior art

Archived designs and rules this reuses or departs from (FLOW-01), and why.

## Decisions

Each decision with the alternatives considered and why they lost.

## Questions

### Blocking

Unanswered questions that stop the design (FLOW-07). Each with options and a
recommended answer.

### Assumed

"Proceeding with X; confirm." Each is confirmed in the design review (FLOW-19).
Mark one `Flagged: <reason>` when it touches a security boundary or a seam, or
departs from prior art or an earlier decision of the human's.

## Design

The behavior and structure of the change. For each repository in `repos`
other than this one, say what is done there and which invariants apply
(LOOP-04, LOOP-08).

## Defaults and overrides

Each default this design adds or changes, why it is the obvious choice
(GLOBAL-09), and what overrides it, used exclusively (GLOBAL-10). Each merge or
fallback, named, with what triggers it and how the user sees it (GLOBAL-10,
GLOBAL-11). "None" when the design adds or changes no default, override, merge
or fallback.

## Cross-driver impact

For each driver named in `drivers`, what changes; for the others, why they are
unaffected (ORCH-01).

## Specification changes

Required when `seams` is not empty (LOOP-02); delete otherwise. For each
seam: the specification document and the requirements the item changes,
what changes in them, and where the change is made (the path here, or
the other repository's pull request and the release this repository
will pin). An item that only implements existing requirements lists
them and says "no change". For a seam with no written specification,
say so and name what defines it.

## Bootstrap comparison

Required when the design changes the bootstrap model (BOOT-07); delete otherwise.
The current and proposed model diagrams, a table with one row per invariant
(BOOT-01 to BOOT-06: current, proposed, per-driver effect), and what the change
gains and costs. Sign-off must accept this section explicitly.

## Rule changes

Required when `rule_changes` is not empty (FLOW-14); delete otherwise. For each
entry: the rule's current wording and the proposed wording (in full for an
amended paragraph), its severity and check before and after, why the feature
needs it, and what the enforcement change does. The design review approves each
one by name (FLOW-19).

## Security note

Required when `security_boundary` is not `none` (GLOBAL-06): the boundary, how its
posture changes, and what the TEST-08 suite should exercise.

## Validation plan

Tier 0 / 1 / 2 coverage (TEST-02), which native hosts validate the change and
whether each blocks merge (FLOW-09), and any deferred gaps.

## Implementation plan

Ordered tasks, each citing the rules it must satisfy. Rework of already-implemented
code is listed as explicit tasks (FLOW-06).

## Design review

One entry per review (FLOW-19): the date, the outcome (approved, or changes),
the decisions taken, and for an approval, `approved at <sha>`, the commit the
approved design was read at, and one `rule change <ID> approved at <sha>` line
per approved rule change (FLOW-14). A re-review also says what changed since the
last approval. Each review is one top-level bullet led by `r<N>, <date>:
<outcome>`; for an approval, the head reads `approved at <sha>`, and an
earlier approval cited in the entry comes after it: `wf` reads an entry's
first `approved at <sha>` as its approval.

## Consolidation

Filled in at Publish (FLOW-10, FLOW-15): features landed since the fork,
terminology aligned, abstractions shared, seams reconciled, rules proposed, and
the dead-code, duplication and overview sweep.

## Rollback notes

Anything validated, found wanting, and reverted, with the evidence.

## Archive summary

Filled in at Publish, in the feature's PR, and refreshed when the PR changes
(FLOW-12): what was built, decisions taken and rejected, and what a future design
touching these `concepts` needs to know.
