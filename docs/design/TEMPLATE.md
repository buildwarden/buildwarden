---
# Copy this file to docs/design/<feature>.md. See FLOW-01, FLOW-02 and FLOW-12 in
# docs/development/process.md. Delete comments once filled in.
feature: <feature-slug>
status: draft            # draft | active | archived
round: parallel          # parallel | merge
drivers: []              # container, qemu, vz, hyperv, firecracker; or [interface]
seams: []                # e.g. relay-claim, collector, executor-initialize, ledger-format,
                         # bootstrap-model (BOOT-07: needs the Bootstrap comparison below)
surfaces: []             # e.g. cli, config, output
security_boundary: none  # none | host-env | env-relay | env-beyond (GLOBAL-06)
alters_relationships: false  # true if a component's role or interactions change (FLOW-02, FLOW-05)
native_validation: []    # - {host: windows-hyperv, blocking: true, reason: "..."}
tier2_gaps: []           # platform validation knowingly deferred (TEST-03)
rules: []                # rule IDs relied on or changed, e.g. ORCH-01, UX-01
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

"Proceeding with X; confirm." Each must be confirmed before sign-off.

## Design

The behavior and structure of the change.

## Cross-driver impact

For each driver named in `drivers`, what changes; for the others, why they are
unaffected (ORCH-01).

## Bootstrap comparison

Required when the design changes the bootstrap model (BOOT-07); delete otherwise.
The current and proposed model diagrams, a table with one row per invariant
(BOOT-01 to BOOT-06: current, proposed, per-driver effect), and what the change
gains and costs. Sign-off must accept this section explicitly.

## Security note

Required when `security_boundary` is not `none` (GLOBAL-06): the boundary, how its
posture changes, and what the TEST-08 suite should exercise.

## Validation plan

Tier 0 / 1 / 2 coverage (TEST-02), which native hosts validate the change and
whether each blocks merge (FLOW-09), and any deferred gaps.

## Implementation plan

Ordered tasks, each citing the rules it must satisfy. Rework of already-implemented
code is listed as explicit tasks (FLOW-06).

## Consolidation (merge round)

Filled in during the merge round (FLOW-10): features landed since the fork,
terminology aligned, abstractions shared, seams reconciled, rules proposed.

## Rollback notes

Anything validated, found wanting, and reverted, with the evidence.

## Archive summary

Filled in on merge (FLOW-12): what was built, decisions taken and rejected, and what
a future design touching these `concepts` needs to know.
