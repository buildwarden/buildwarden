---
feature: design-approved-rule-changes
status: draft
drivers: []
seams: []
surfaces: []
security_boundary: none
alters_relationships: false
design_review: required
native_validation: []
tier2_gaps: []
rules: [FLOW-03, FLOW-06, FLOW-08, FLOW-10, FLOW-14, FLOW-19, BOOT-07, LINT-01, LINT-04]
concepts: [workflow, design-review, rule-changes, process-item, enforcement, tier1, wf-check]
related: [human-review-shows-design.md]
---

# A rule change a feature needs is approved in its design review and made in its Implement

## Problem

FLOW-14 allows exactly one route for a change to the standards in
`docs/development/`: a process item, whose design the human settles in its
own review. Every phase agent and the `workflow-tick` skill repeat it as a
"never edit `docs/development/` on a feature branch" line. That is right for
a change nobody's feature needs, but it is wrong for a feature whose design
deliberately changes a rule as part of what it does.

build-link-subnet is the case that surfaced it (workflow-human, 2026-11-04):

- Its design proposed a BOOT-03 wording change (a launcher may take its
  address from a DHCP lease served by the relay), as BOOT-07 asks.
- The plan made it task 49. Implement could not do it, because FLOW-14
  forbids the edit on a feature branch.
- Review r1 (`878d510bc61b`) raised it as a MUST-review. Without the
  wording, main would contradict BOOT-03 as written the moment the feature
  merged.
- The only way out today is a separate process item, designed, reviewed,
  implemented and merged on its own. That splits one decision across two
  reviews and two PRs, and the feature's PR lands either before its rule
  (contradicting it) or blocked behind it.

FLOW-10 already says a convention that emerges at consolidation "is proposed
as a new or amended rule ... in the same PR or as a follow-up item", so the
standards are not consistent about it either.

Jeff's rule (2026-11-04): if a feature's design intentionally requires
altering one or more development rules or their enforcement, those changes
are reviewed explicitly in the design review. If approved, the feature is
cleared to make them as part of its implementation.

## Prior art

- **FLOW-14:** the process item route. It stays; this design adds a second
  route next to it, and it does not change who decides (the human, FLOW-19)
  or how a change reaches the running loop (the human's merge, FLOW-08).
- **FLOW-19:** the review already lists "rules changed" among the risks, and
  a design that changes a rule cannot skip its review ("the branch changes no
  rule in this directory"). What is missing is an explicit approval of each
  change and the clearance that follows from it.
- **BOOT-07:** a bootstrap-model change needs a Bootstrap comparison and the
  review's acceptance of it. It is the one rule that already expects a
  feature to change the standards it is reviewed against. It does not say
  who edits `bootstrap.md`.
- **bootstrap-firecracker-row** (process item, merged as #91): the BOOT-07
  surface list and Firecracker row, done as a process item because no
  feature route existed.
- **human-review-shows-design** (approved, not yet implemented): the review
  puts the design in front of Jeff. A rule change's before and after wording
  is part of what he must see.
- **The `Tier1-Allow` trailer:** a per-commit waiver for a tier 1 finding.
  It waives a finding; it does not change a rule, and it stays as it is.

## Decisions

1. **Scope: the rules and their enforcement, not the loop's own machinery.**
   A *rule change* is any edit to:
   - `docs/development/*.md`: adding, amending, or retiring a rule, its
     severity or its check type, and the README index;
   - the code that enforces rules: `tools/tier1/` (checks and `rules.yaml`),
     `tools/workflow/check/` (done-when checks), and the rule-running lines
     of `.github/workflows/`;
   - `docs/design/TEMPLATE.md` where a rule names its keys.

   `.claude/` (agents and skills) and the rest of `tools/workflow/` (queue,
   claiming, `wf` commands) stay process-item only. Those are the machinery
   running the very feature that would change them, so a change there is the
   loop editing itself mid-flight. The FLOW-14 route is kept for them, and
   for rule changes no feature needs.
   - **Rejected: every guarded path** (`.claude/` and all of
     `tools/workflow/` too). A feature's Implement could rewrite the agent
     that reviews it.
   - **Rejected: the rule text only, with enforcement still a process item.**
     A rule whose check lags its text fails the release gate or misfires on
     the next feature, which is the split this design removes.

2. **A design declares each rule change in its front-matter and in a
   section.** There is a new front-matter key, `rule_changes`, one entry per
   change:

   ```yaml
   rule_changes:
     - {rule: BOOT-03, change: amend, paths: [docs/development/bootstrap.md]}
     - {rule: TEST-09, change: add, paths: [docs/development/testing.md, tools/tier1/rules.yaml]}
   ```

   `change` is `add`, `amend` or `retire`. `paths` lists every file the
   change edits, enforcement included. There is also a new design section,
   **Rule changes**: for each entry, the rule's current wording and the
   proposed wording (in full for an amended paragraph), its severity and
   check before and after, why the feature needs it, and what the
   enforcement change does. `rules` keeps listing rules relied on or
   changed. Empty, or the key absent, means no rule change, and every
   archived design is valid without it.
   - **Rejected: the body section alone.** The check in Decision 5 needs
     something mechanical to compare the diff against.
   - **Rejected: rule IDs only, with paths derived.** A new rule has no ID
     to look up yet, and enforcement files are not named by ID.

3. **The design review approves each rule change by name.** In the FLOW-19
   session, each `rule_changes` entry is shown with its before and after
   wording and asked as its own question: approve, approve with a change, or
   reject. The Design review entry records them on one line per change, for
   example `rule change BOOT-03 approved at <sha>`, beside the design's
   `approved at <sha>`.
   - A rejected change sends the feature back to Design (FLOW-06), unless
     the review keeps the feature and drops the change.
   - A design with any `rule_changes` cannot skip its review. This extends
     FLOW-19's skip criteria, which today only look at whether the branch
     already changed a rule.
   - **Rejected: one approval for the design covers its rule changes.**
     Jeff's rule asks for an explicit review, and a rule outlives the
     feature.

4. **Approved changes are planned and made in Implement on the feature
   branch.** Plan lists each approved change as a task citing the rule (as
   task 49 was). Implement makes it with the rest of the change. Review
   judges the diff against the rules *as changed* and checks that the edit
   matches the approved wording. Publish's PR body gets a **Rule changes**
   section listing each one with its approval commit. A change reaches the
   running loop only through the human's merge (FLOW-08).

   FLOW-14 gains the second route. FLOW-10's "in the same PR" stays allowed
   only for a change the review approved; a convention found at
   consolidation that is not approved goes back to Design for a review
   (FLOW-06), or becomes a follow-up item.

5. **`wf check` enforces the clearance mechanically.** It runs at the
   Implement and Review done-when (FLOW-03) and at Publish: every file the
   branch changes under the paths in Decision 1 must be in the `paths` of a
   `rule_changes` entry that the Design review approved (`rule change <ID>
   approved at <sha>`, with that entry present in the front-matter at that
   commit). An unlisted edit fails the check by name. Changes under
   `.claude/` or the rest of `tools/workflow/` on a feature branch fail it
   outright (Decision 1). FLOW-14's check becomes `automated +
   agent-judgment`; Review judges whether the wording matches what was
   approved.
   - **Rejected: agent judgment only.** That is today's state, and a
     prohibition repeated in six agent files is what failed for task 49.

6. **An enforcement change is checked by main's enforcement, not its own.**
   When a branch changes `tools/tier1/` or `tools/workflow/check/`, Review
   and CI also run those tools built from the merge base against the
   branch. Every finding from main's tier1 that the branch's own tier1 no
   longer reports must be one the approved change names. Otherwise a
   feature could weaken a check and then pass it.
   - **Rejected: trust the branch's tools,** for the reason given.
   - **Rejected: forbid enforcement changes on a feature branch,** which is
     Decision 1's rejected option.

7. **A rule change found after the review is not cleared.** If Plan,
   Implement or Consolidation finds the feature needs a rule change the
   review did not approve, the feature goes back to Design (FLOW-06), and
   the re-review covers only the new change (FLOW-19's delta rule). Under
   the design-before-plan trial run on build-link-subnet r3, a rule change
   found in planning is one of the reasons for a second review.

8. **build-link-subnet is the first user.** Once this item merges,
   build-link-subnet's BOOT-03 change gets a short review addendum
   (`rule change BOOT-03 approved at <sha>`, with the Bootstrap comparison
   accepted as amended by a14 (d), which Review r1 also asked for), and a
   `rule_changes` entry. Task 49 is done in an Implement round before
   Publish. Until then it stays open, as Review r1 recorded.

## Questions

### Blocking

None.

### Assumed

- **a1. Scope** (Decision 1). Proceeding with `docs/development/`,
  `tools/tier1/`, `tools/workflow/check/`, the rule-running CI lines and
  TEMPLATE's rule keys, and with `.claude/` and the rest of
  `tools/workflow/` left process-item only. Options:
  - (a) as stated;
  - (b) every FLOW-14-guarded path, `.claude/` and all of `tools/workflow/`
    included;
  - (c) the rule text only, enforcement still a process item;
  - (d) as stated, but FLOW-* and LINT-* rules (the loop's own process and
    linter) also stay process-item only.

  Recommended: (a).
- **a2. Declaration** (Decision 2). Proceeding with a `rule_changes`
  front-matter key `{rule, change, paths}` plus a **Rule changes** section
  with before and after wording. Options:
  - (a) as stated;
  - (b) the section only, without the key;
  - (c) the key only, the wording kept in Decisions.

  Recommended: (a).
- **a3. Approval** (Decision 3). Proceeding with each change asked as its
  own question and recorded as `rule change <ID> approved at <sha>`.
  Options:
  - (a) as stated;
  - (b) the design's one approval covers its listed rule changes.

  Recommended: (a).
- **a4. Mechanical check** (Decision 5). Proceeding with a `wf check` at
  Implement, Review and Publish that fails on any guarded edit not in an
  approved entry's `paths`. Options:
  - (a) as stated;
  - (b) Review's judgment only;
  - (c) as stated, and also in tier 1 so CI enforces it on every PR.

  Recommended: (a).
- **a5. Enforcement changes** (Decision 6). Proceeding with main's tier1
  and check also run against a branch that changes them, and every finding
  the branch's tools drop named by the approved change. Options:
  - (a) as stated;
  - (b) the branch's own tools only;
  - (c) no enforcement changes on a feature branch.

  Recommended: (a).
- **a6. Found after the review** (Decision 7). Proceeding with back to
  Design and a delta re-review. Options:
  - (a) as stated;
  - (b) Review may approve a small rule change found in Implement, with
    Jeff's sign-off left to the merge;
  - (c) always a follow-up process item.

  Recommended: (a).
- **a7. build-link-subnet** (Decision 8). Proceeding with BOOT-03 approved
  in a review addendum once this item merges, and task 49 done before
  Publish. Options:
  - (a) as stated;
  - (b) carry the BOOT-03 wording in this item's own PR;
  - (c) build-link-subnet publishes without it, with a follow-up process
    item for BOOT-03.

  Recommended: (a).

## Security note

`security_boundary: none`: no build-environment boundary changes. The change
widens what an unattended agent may edit on a feature branch, from nothing in
`docs/development/` to what the human approved by name. It is bounded at four
points:

- the review approves each change and its paths at a commit;
- `wf check` refuses any guarded edit outside them;
- main's enforcement checks the branch's (Decision 6);
- the human's merge remains the only way into the running loop.

The loop's own agents and queue tooling stay outside the route (Decision 1).

## Validation plan

- **Tier 0, `tools/workflow/check`:**
  - `rule_changes` parsing, plus `Problems` for a bad `change` value and an
    empty `paths`.
  - The skip criteria refuse `skip` with a non-empty `rule_changes`.
  - The clearance check over a git fixture:
    - an approved edit passes;
    - an unlisted guarded edit fails, naming the path;
    - an entry added after the approval commit fails;
    - a `.claude/` edit on a feature branch fails;
    - a process item's branch is unaffected.
- **Tier 0, Decision 6:** a fixture branch that drops a tier1 finding passes
  only when its approved change names the rule.
- **The check against itself:** this item's own branch is a process item, so
  the new check must not refuse it.
- **No native validation.**

## Implementation plan

Written at x2-implement, after the review (process item, FLOW-03). Each task
is one commit on this branch.

1. `docs/development/process.md` (FLOW-14, FLOW-19, FLOW-10, FLOW-03,
   FLOW-02, LINT-01):
   - FLOW-02 lists the `rule_changes` key;
   - FLOW-03's feature table names the rule-change clearance at Implement,
     Review and Publish, and the process-item paragraph says which paths stay
     process-item only;
   - FLOW-10's "same PR" is allowed only for a change the review approved;
     otherwise back to Design (FLOW-06) or a follow-up item;
   - FLOW-14 gains the second route (Decisions 1, 4, 5, 6, 7) and becomes
     `automated + agent-judgment`;
   - FLOW-19 gains the per-change approval line and the `rule_changes` skip
     criterion.
2. `docs/development/bootstrap.md` BOOT-07 (BOOT-07, FLOW-14): a model change
   a feature needs edits `bootstrap.md` in its Implement as an approved rule
   change, or in a process item.
3. `docs/design/TEMPLATE.md` (FLOW-02, FLOW-19): the `rule_changes` key and
   the **Rule changes** section.
4. `tools/workflow/check` (FLOW-03, FLOW-14, FLOW-19, GLOBAL-01, TEST-01,
   TEST-02 tier 0):
   - `Front.RuleChanges` and its `Problems` (bad `change`, bad rule ID, empty
     or unclearable `paths`); tier1's FLOW-02 check reuses `Problems`;
   - rule-change approvals (`rule change <ID> approved at <sha>`) parsed apart
     from the design's own approvals, so they do not count as one;
   - Design requires the **Rule changes** section when `rule_changes` is
     non-empty; the skip criteria refuse `skip` with a non-empty
     `rule_changes`;
   - the clearance check at phases 3, 4 and 6, process items exempt;
   - tests over git fixtures for each Validation plan case.
5. Decision 6 (FLOW-14, LINT-02, LINT-04, TEST-01): `wf drops` compares
   main's tier1 report with the branch's and fails on a dropped finding no
   approved change names; `tools/workflow/enforcement-check.sh` builds tier1
   from the merge base, runs both, and compares; `make enforcement` runs it;
   the CI `tier1` job runs it. Tier-0 test of the comparison over a fixture.
6. `tools/tier1/rules.yaml` (LINT-01, LINT-02): FLOW-14's owner.
7. Agents and skills (FLOW-14): wf-design, wf-plan, wf-implement, wf-review,
   wf-publish, workflow-tick, workflow-native, and a small addition to
   workflow-human step 3.
8. `overview.md` (FLOW-15): no component flow changes; none expected.
9. Consolidation, `make lint`, `make test` (FLOW-10, FLOW-17).

## Design review

- **r1, 2026-11-04: approved at `9f88987`** (Jeff, workflow-human, x1-design,
  FLOW-19). The design was published as a review page at `9f88987`, with
  the seven assumed questions first.
  - **All seven confirmed as drafted (a):** a1 scope (rules and their
    enforcement; `.claude/` and the rest of `tools/workflow/` stay
    process-item only), a2 the `rule_changes` key plus a **Rule changes**
    section, a3 each change approved by name, a4 the `wf check` at
    Implement, Review and Publish, a5 main's enforcement also run against a
    branch that changes it, a6 a change found after the review goes back to
    Design, a7 build-link-subnet's BOOT-03 approved in a review addendum
    once this item merges, with task 49 done before its Publish.

## Consolidation

Filled in at x2-implement.

## Rollback notes

Revert the PR. Designs that declared `rule_changes` stay valid: an unknown
key is ignored by the reverted check, and their rule edits are already
merged.

## Archive summary

Filled in at x2-implement.
