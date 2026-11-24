---
feature: program-autonomy
status: draft
drivers: []
seams: []
surfaces: []
repos: []
merge_first: []
areas: [loop, rules]
security_boundary: none
alters_relationships: false
design_review: required  # touches rules FLOW-20 reserves to Jeff: merging, tags, budgets
native_validation: []
tier2_gaps: []
rules: [FLOW-04, FLOW-05, FLOW-08, FLOW-10, FLOW-11, FLOW-13, FLOW-14, FLOW-16, FLOW-17,
        FLOW-19, FLOW-20, LOOP-07, LOOP-09, LINT-03, GLOBAL-10]
rule_changes:
  - {rule: FLOW-08, change: amend, paths: [docs/development/process.md,
      tools/workflow/check/merge.go, tools/workflow/queue/next.go,
      .claude/agents/wf-publish.md, .claude/skills/workflow-tick/SKILL.md]}
  - {rule: FLOW-13, change: amend, paths: [docs/development/process.md,
      tools/workflow/queue/next.go, tools/workflow/queue/queue.go,
      tools/workflow/queue/limits.go, .claude/skills/workflow-tick/SKILL.md]}
  - {rule: FLOW-17, change: amend, paths: [docs/development/process.md,
      tools/workflow/cmd/wf/main.go, tools/workflow/check/tags.go,
      .claude/skills/workflow-tick/SKILL.md]}
  - {rule: FLOW-20, change: amend, paths: [docs/development/process.md,
      .claude/agents/wf-review.md]}
concepts: [workflow, merge-gate, program-trunk, loop-merge, pre-release, tag, budget, runaway,
           reserved-rules, process-test]
related: [componentization.md, program-north-star.md, budgets-active-only.md,
          merge-confirm-attended.md]
---

# Program autonomy: program-trunk merges, pre-release tags and post-merge budgets

## Problem

Three things still stop the program on Jeff where there is nothing for him
to decide (Jeff, 2026-11-24):

1. **Merging.** FLOW-08 already lets the loop merge a program item's PR
   into its program trunk behind the merge gate, but FLOW-20 reserves to
   Jeff "any change to a rule about ... merging". Read by that word, every
   process item that touches the loop's program-trunk merge machinery goes
   to his review even when it admits nothing new: `wf-gate-rest` went to
   him only for that. Parallelism is growing (several cloud runs, eight
   repositories with a program trunk), and nothing yet says the loop
   merges into one program trunk one pull request at a time.
2. **Tags.** FLOW-17 and FLOW-20 let the loop push `-dev.N`, `-rc.N` and
   the final tag, but a cloud session's `git push` of a tag gets HTTP 403
   from the session's own git proxy, before GitHub's ruleset (no
   `X-Github-Request-Id` on the receive-pack response). Every tag
   `wf tag-next` names therefore waits for an attended session.
3. **Budgets.** FLOW-13 counts every claim against the work budget and
   holds every step of a runaway item. `relay-conformance-timeout`'s Close
   was held at 16 of 8 steps after its PR had merged: a hold on a step that
   changes nothing in the repositories, with one possible answer.

## Prior art

- `program-north-star.md` (FLOW-20): the loop merges into program trunks
  behind FLOW-08's merge gate, merge commits only, pinned to the head; the
  reserved-rules list and the process test. Kept; this narrows what the
  list reserves for program-trunk merges and pre-release tags.
- `wf-gate-rest` (open): the merge gate read over REST; its review went to
  Jeff because it touches merging. `sync-pr-tier1` (open): tier 1 on a
  sync PR; its review went to Jeff because it touches merging and relaxes
  a check.
- `budgets-active-only.md`: done items left out of the runaway check. This
  extends the same idea to the post-merge steps of open items.
- `merge-confirm-attended.md`: merge confirmations need no claim and spend
  no run budget. This makes it a rule and extends it to every step from
  Merge on.
- `wf tag-next` and `check/tags.go`: the pre-release naming and its
  checks. Kept unchanged; only how the named tag is created changes.

## Decisions

**D1. Program-trunk merges: keep the gate, add per-repository
serialization and a recheck under it.** The loop's merge into a program
trunk keeps all nine gate points, merge commits only, pinned to the head
it checked (`--match-head-commit`, or the REST merge's `sha`). Added:
- *One at a time per repository.* A loop merge step (`f7-merge`, an
  `f6-merged-<n>` on a program trunk, a program sync) is offered only while
  no other loop merge step in the same repository is claimed under a live
  lease. The claim is a state commit on the `workflow` branch, so two
  workers racing for it are serialized by that push; the loser re-reads
  and gets nothing.
- *Recheck under the claim.* Inside the claim the loop fetches the program
  trunk, runs the gate on that head (point 4 against it) and merges at
  once. A program trunk moves only by merged PRs, all of them loop merges
  under the same serialization, so nothing can move it between the gate
  and the merge; a head that moved fails GitHub's pinned merge.
- *Server-side belt (setup, Jeff).* Each program trunk's ruleset requires
  its checks and branches up to date, and no approving review (the loop
  never approves, FLOW-08). Not a fallback: a second guard on the same
  condition (Q3).
Escalations that stay Jeff's: gate point 7 (a security note with a
posture change), point 9 (rule changes he approved), a tier-2 MUST finding
the loop may not defer when the posture or contract would change, a sync
conflict the loop cannot resolve, a CI failure Publish cannot repair
(FLOW-11), and every merge to a trunk.
*Rejected:* GitHub's merge queue (not available for every repository on
the plan, and it rebuilds merge commits the gate did not check); a lock
file on the program trunk (a push the loop may not make); serializing
across repositories (no shared state to protect, slows the program).

**D2. Reserved rules: "merging" means merging to a trunk; "tags" means
release tags without a suffix.** Jeff made program-trunk merges and
pre-release tags the loop's (2026-11-24), as pushes of feature branches
already are. A change to how the loop takes them is reserved only by its
effect: when it removes or narrows a gate point, drops the one-at-a-time
rule, lets a tag move or be deleted, or by the other clauses (widening
authority, raising a budget, relaxing a check). Consequences:
- `wf-gate-rest` reads the same gate and admits nothing new; under the
  amendment it would take the process test. Its review is already seeded
  as Jeff's and the amendment lands only with this item's merge, so it
  stays with Jeff now (Q5).
- `sync-pr-tier1` still relaxes a MUST check on a PR the loop merges; that
  clause, not merging, keeps it with Jeff after the amendment too.
*Rejected:* dropping merging and tags from the list outright (a change
that weakened the gate would then pass the process test).

**D3. Tags are created through GitHub's REST refs API, never by git
push.** A new `wf tag` runs `tag-next`'s checks, then for each tag it names
calls `POST /repos/<R>/git/refs` with `refs/tags/<tag>` and the exact sha
it checked. The API refuses an existing ref (422), so a tag can never be
moved by it; `wf tag` never calls the update or delete endpoints. The ref
is created by the session's GitHub identity (the Claude app, which Jeff
gives the release-tags ruleset's bypass), so it triggers `release.yml` as
a push would. Whether the session's policy permits this call is Jeff's
(Q1); it has not been tried.
*Rejected:* a tag workflow in each repository dispatched over REST (nine
CI/release changes, which are privileged; a `GITHUB_TOKEN`-created tag
does not trigger `release.yml`, so it needs an App token secret and the
Actions app's bypass, or a `workflow_call` into each release workflow);
native sessions pushing tags (latency of their schedule, an offline
machine holds the program, and the native push script pushes branches
only, FLOW-17). Working around the proxy's 403 in any other way:
rejected outright.

**D4. Budgets count only steps before Merge.** A claim counts against the
item's work budget, and the run's budget, only for a step before the
item's Merge phase (feature phase 7, process phase 3), excluding
`f6-merged-<n>` and the first claim of a refresh as today. From Merge on
(a merge or its confirmation, `f6-merged-<n>`, Close, a state step) and
the run's records and digest, nothing counts and the runaway check holds
nothing. The loop merge itself is exempt too: the gate decides it, it
does no work of its own, a failing one is bounded by FLOW-11, and holding
it lets an approved PR rot (Q4).
*Rejected:* exempting only confirmations (keeps Close held, today's case).

## Questions

### Blocking

None.

### Assumed

- **Q1. How the loop creates tags.** Flagged: tags are a reserved rule
  and the session policy is Jeff's. Options: (a) REST refs API from the
  cloud session, if its policy allows it; (b) a dispatched tag workflow
  per repository; (c) native sessions push tags. Recommended: (a); if
  Jeff's answer is that the policy does not allow it, the design returns
  to Design for (b) (FLOW-06), with no fallback in code (GLOBAL-10).
- **Q2. Reading of the reserved list.** Flagged: departs from FLOW-20's
  reserved list as Jeff wrote it. Options: (a) narrow "merging" to a trunk
  and "tags" to release tags without a suffix, the loop's program-trunk
  merges and pre-release tags reserved only by effect (D2); (b) drop
  merging and tags from the list; (c) unchanged. Recommended: (a).
- **Q3. Program-trunk rulesets require up-to-date branches.** Flagged:
  setup on nine repositories, Jeff's. Options: (a) yes, with required
  checks and no approving review; (b) no, rely on the loop's
  serialization. Recommended: (a).
- **Q4. The loop merge step is outside the budget.** Options: (a) yes,
  every step from Merge on (D4); (b) only confirmations and Close.
  Recommended: (a).
- **Q5. `wf-gate-rest` and `sync-pr-tier1`.** Options: (a) both stay with
  Jeff now (the amendment is not on `main`); `wf-gate-rest`-like changes
  take the process test after this merges, `sync-pr-tier1`-like ones stay
  his (relaxed check); (b) re-route `wf-gate-rest` to the process test on
  Jeff's word now. Recommended: (a); he is in the session and it is one
  review.

## Design

**Merging (D1).** `queue/next.go`: a loop merge step in repository R is
not offered while another item's loop merge step in R holds a live lease;
`wf next` prints why. `check/merge.go`: `MergeReady` fetches the program
trunk and runs point 4 against the fetched head, inside the claim.
`wf-publish.md` and the tick's skill say: merge at once after the gate,
pinned to the head, then `wf done`; never two merges in one repository at
once. Nothing else in the gate changes.

**Tags (D3).** `cmd/wf/main.go` gains `wf tag` with `tag-next`'s flags; it
reuses `check/tags.go` for the names and checks, prints each tag and sha
it creates, and fails on any refusal (an existing ref included) without
retrying. The tick's skill replaces `git tag` / `git push origin <tag>`
with `dist/wf tag ...`, then `wf record -kind tag` as today. Naming, the
`-rc` conditions and never-move-or-delete are unchanged.

**Budgets (D4).** `next.go`'s claim increments `Spent.Steps` only for a
budgeted step (new `Budgeted(f, st)`: phase before Merge, not
`f6-merged-<n>`, not a refresh's first claim). `Runaway` and
`NeedsReestimate` no longer hold a step that is not budgeted
(`limits.go`, `next.go`); the waiting-on-human count drops an item whose
only pending steps are unbudgeted. The tick's run budget skips the same
steps.

**What the change reaches** (for the reserved-rule reading):
- Rules amended: FLOW-08, FLOW-13, FLOW-17, FLOW-20 (below). Relied on,
  unedited: FLOW-04, FLOW-05, FLOW-10, FLOW-11, FLOW-14, FLOW-16, FLOW-19,
  LOOP-07, LOOP-09, LINT-03, GLOBAL-10.
- Machinery: `tools/workflow/check/merge.go`, `tools/workflow/check/tags.go`,
  `tools/workflow/queue/next.go`, `tools/workflow/queue/queue.go`,
  `tools/workflow/queue/limits.go`, `tools/workflow/cmd/wf/main.go`, their
  tests; `.claude/agents/wf-publish.md`, `.claude/agents/wf-review.md`
  (P2's list), `.claude/skills/workflow-tick/SKILL.md`.
- Configuration: no `config.yaml` key changes (`run_budget`,
  `runaway_factor` keep their values; what they count narrows). No
  `.claude/settings*.json` change. Outside the repository, Jeff's setup:
  the Claude app's bypass on each repository's release-tags ruleset (done
  for specs), and Q3's program-trunk rulesets. No CI or release workflow
  changes.
- Areas: `loop` and `rules`, as intake recorded; the area map needs no
  change. Only this repository.

## Program payoff

Figures from the workflow branch (`program/records.yaml`, the queue's
histories) and `wf-gate-rest`'s count from `componentization.md`:

- **Tags.** At least 21 loop merges remain over the program (wf-gate-rest's
  count: 13 phases plus the per-repository merges of K3, K4 and M6), each
  naming at least one `-dev.N`; plus one `-rc.N` and one final tag for each
  of the 9 repositories: **at least 39 tags**, each of which today needs an
  attended session because the cloud's push is refused. At the program's
  pace of 3 to 5 loop merges a week, **3 to 5 interactions a week** that
  decide nothing.
- **Runaway holds after a merge.** One today (relay-conformance-timeout's
  Close, raised on Jeff's word with one possible answer); `budgets-active-only`
  was raised the same way with only "the merge and Close" left. About
  **1 a week**, rising with parallelism.
- **Reviews for merging alone.** One of the three process items filed on
  2026-11-23..24 (`wf-gate-rest`) went to Jeff only for touching merging:
  about **1 a week** while the loop's machinery is still being fixed.

Total about **5 to 7 interactions a week** removed, against this item's
cost: about 6 steps and one review (this sitting).

## Defaults and overrides

- The run's and the item's budgets now count only steps before Merge (D4);
  no override, `runaway_factor` and `run_budget` unchanged.
- Tags are created only by `wf tag` over REST (D3); no fallback to
  `git push` (GLOBAL-10): a refusal fails the step and shows in the digest.
- No other default, override, merge or fallback.

## Cross-driver impact

None: the change is to the loop's rules and machinery only.

## Rule changes

**FLOW-08** (MUST, automated; unchanged severity; check gains the
serialization in `wf next` and the recheck in `MergeReady`). Current:
"The loop merges with a merge commit, never a squash or a rebase, pinned
to the head it checked, and records the merge (FLOW-20)." Proposed: "The
loop merges with a merge commit, never a squash or a rebase, pinned to
the head it checked, and records the merge (FLOW-20). It merges into a
program trunk one pull request at a time per repository: no loop merge in
a repository is taken while another holds its claim, and under the claim
the loop reads the program trunk's head again, runs the gate on it and
merges at once." Why: D1.

**FLOW-13** (SHOULD, automated; unchanged). Current: "Every claim counts
against the work budget, whichever worker makes it, except the first claim
of a refresh (FLOW-10); renewing a lease does not (FLOW-04)." Proposed:
"Only a step that changes the item's repositories counts: every claim of
a step before the item's Merge phase counts against the work budget and
the run's, whichever worker makes it, except the first claim of a refresh
(FLOW-10); renewing a lease does not (FLOW-04). From Merge on (a merge or
its confirmation, an `f6-merged-<n>`, Close and its state step), and the
records and digest a run writes, nothing counts against either budget,
and the runaway check holds none of it." And "no unattended run takes
more of its steps" becomes "no unattended run takes more of its steps
before Merge". Why: D4. Enforcement: `Budgeted` in the claim and in the
runaway holds.

**FLOW-17** (agent-judgment; unchanged). Current: "or a tag, except a
pre-release tag or a program trunk's final tag FLOW-20 lets the loop push,
and a release tag an attended session pushes on the human's word; no tag
is moved or deleted." Proposed: "or a tag, except a pre-release tag or a
program trunk's final tag FLOW-20 lets the loop create, which it creates
only with `wf tag` through GitHub's REST API at the commit `wf tag-next`
checked, never by a git push, and a release tag an attended session pushes
on the human's word; no tag is moved or deleted." Why: D3. Enforcement:
`wf tag` calls only the create endpoint.

**FLOW-20** (MUST, automated + agent-judgment; unchanged). Current: "...;
and any change to a rule about security, merging, pushing or tags."
Proposed: "...; and any change to a rule about security, merging to a
trunk, pushing, or release tags without a suffix. A merge into a program
trunk under FLOW-08's merge gate, and a pre-release or final tag this rule
lets the loop create, are the loop's: a change to how it takes them is
reserved only when it removes or narrows a point of the merge gate, lets
the loop take two merges into one program trunk at once, lets a tag move
or be deleted, or meets another clause above; a merge or tag the loop may
already take is not one it could not take before, whatever worker or
machinery takes it." Why: D2. Enforcement: `wf-review.md`'s P2 list
follows the wording.

## Validation plan

Tier 0: unit tests in `tools/workflow/queue` (a second loop merge in the
same repository not offered under a live lease, offered in another
repository; post-Merge claims not counted; a runaway item's Close
offered), `tools/workflow/check` (the recheck reads the fetched head) and
`cmd/wf` (`wf tag` builds the create call with the checked sha and refuses
on 422, against a fake `gh`). Tier 1: `make test`, `make lint`. No native
host (`native_validation: []`); no tier-2 gap. Live: the first `wf tag`
after the merge, on specs, where the bypass is in place.

## Implementation plan

1. `queue`: `Budgeted`, the claim count and runaway holds (FLOW-13), tests.
2. `queue/next.go`: per-repository loop-merge serialization (FLOW-08), tests.
3. `check/merge.go`: the recheck under the claim (FLOW-08), tests.
4. `cmd/wf`: `wf tag` over `check/tags.go` (FLOW-17, FLOW-20), tests.
5. `process.md`: the four amendments, as approved.
6. `.claude/`: `wf-publish.md`, `wf-review.md` P2, the tick's skill (FLOW-14).

## Design review

- r1, 2026-11-24: approved at 8739c23. Jeff approved the design as read
  at 8739c23, in the attended session (workflow-human).
  - Each rule change was asked on its own:
    - rule change FLOW-08 approved at 8739c23
    - rule change FLOW-13 approved at 8739c23
    - rule change FLOW-17 approved at 8739c23
    - rule change FLOW-20 approved at 8739c23
  - q1 (a): tags are created with the REST refs API. Jeff confirms the
    session policy allows it.
  - q2 (a): the reserved reading is narrowed.
  - q3 yes: program-trunk rulesets require their checks and up-to-date
    branches, with no approving review. Jeff sets them up.
  - q4 and q5 as recommended (batch).
  - Nothing was folded in.
  - sitting: 17:59-18:08 UTC, asks: 9 (1 a batch covering q4 and q5)
- r2, 2026-11-24: changes, in the same sitting, after r1's approval.
  r1 approved at 8739c23 stands except q1 and D3 (Jeff, 2026-11-24, same
  sitting): cloud sessions cannot create tags at all. A git push of a tag
  gets HTTP 403 from the session's git proxy, and the REST write paths
  git/tags and git/refs are refused ('Write access to this GitHub API path
  is not permitted through this proxy'). Jeff chose native sessions: the
  cloud loop records each tag wf tag-next names as due, and the next macbook
  or windows-desktop native session creates it with Jeff's credentials, by
  git push of an annotated tag at the checked commit, create only, never
  moved or deleted. Redo D3, q1 and the FLOW-17 amend's wording for that
  route (the rest of r1, including the FLOW-08, FLOW-13 and FLOW-20 amends,
  stays approved), say how a due tag is recorded and handed to the native
  skill, and re-review only that delta.

## Consolidation

## Rollback notes

## Archive summary
