---
feature: merge-confirm-attended
status: draft
drivers: []              # the loop's queue code, checks and skills; no driver tree is reached
seams: []                # no GLOBAL-08 seam changes
surfaces: []             # `wf` and the skills are the loop's tools, not a user-facing surface of the product
repos: []                # only buildwarden/buildwarden
areas: [rules, loop, designs]  # process.md (FLOW-03, FLOW-04); tools/workflow/queue, check and
                         # cmd/wf, the workflow-human and workflow-tick skills, wf-plan and
                         # wf-design; docs/design/TEMPLATE.md and this design
security_boundary: none
alters_relationships: false  # no product component's role or interaction changes (FLOW-02)
design_review: required  # a process item's design always gets its review (FLOW-03, FLOW-19);
                         # it also has rule changes (FLOW-14)
native_validation: []    # pure functions of the queue state, the design text and git plumbing
tier2_gaps: []
rules: [FLOW-03, FLOW-04, FLOW-05, FLOW-06, FLOW-07, FLOW-08, FLOW-10, FLOW-13, FLOW-14,
        FLOW-19, LOOP-09, GLOBAL-09, GLOBAL-10, GLOBAL-11, TEST-01, TEST-02]
rule_changes:            # the enforcement (cmd/wf/main.go, queue/phases.go, queue/queue.go,
                         # check/review.go, check/declared.go) is the loop's machinery or its
                         # checks, changed on the process route (FLOW-14). Paths list docs files
                         # only: tier 1's front-matter check refuses machinery paths under
                         # tools/workflow/ (the 2026-11-18 deviation in loop-parallel-rule-wording),
                         # so the enforcement files are named under Rule changes instead
  - {rule: FLOW-03, change: amend, paths: [docs/development/process.md]}
  - {rule: FLOW-04, change: amend, paths: [docs/development/process.md]}
concepts: [workflow, merge-confirmation, attended-session, workflow-human, state-step,
           x4-state, close, design-review, approvals, process-item, wf-done, wf-stale]
related: [loop-streamline-2.md, queue-positions.md, loop-native-lead.md, loop-parallel.md,
          flow19-design-before-plan.md, design-approved-rule-changes.md]
---

# Attended merge confirmation, a seeded state step, and entry-scoped design approvals

A process item (FLOW-03, LOOP-09). It holds no worker. It carries three
loop changes in one PR, on Jeff's word of 2026-11-20 (fewer merges):

- **A. Approvals.** `wf check` reads a design's approval from the review
  entry that records it, not from any `approved at <sha>` in the Design
  review section.
- **B. Merge confirmation.** An attended session (a `human/<name>` worker)
  may confirm a merge with `wf done` on the merge step. The done-when check
  is unchanged.
- **C. State step.** A state step recorded at review (`state_step`) is
  seeded by `wf` as `x4-state` when the item enters Close, and `x4-close`
  waits for it. This replaces the hand-added steps.

Together they give the payoff that matters: **Jeff can merge a PR, confirm
it and do its state step in one sitting.** Only `x4-close` waits for the
tick.

Words used throughout. **wf** is the loop's tool, `tools/workflow/`. Code
and rule wording are `main` at `480e205`. Queue state is the `workflow`
branch at `366abd3`. The **cross-check** is the 2026-11-20 scouting pass
that combined the three parts and listed ten questions; its Q1 (one item)
is settled, and Q2 to Q10 are a1 to a9 below.

## Problem

### A. A later mention of an older approval becomes the baseline

`approvals()` (`check/review.go:20-27`) strips rule-change approvals from
the Design review section and returns every remaining `approved at <sha>`
match (`approvedRe`, `review.go:15`) in document order. Its callers assume
one match per review:

- `reviewed()` (`review.go:46-67`) passes when `len(approvals(d)) >= want`,
  so a re-review entry with no approval still passes if its prose cites an
  older one.
- `approvalDelta()` (`review.go:91`), `skipValid()` (`review.go:191`) and
  the declaration's `Within.Commit` (`declared.go:79`) take `a[len(a)-1]`
  as the latest approval.

The bug is live. On `origin/feat/detached-prepare-windows-exe`, the Design
review reads:

- `- **r1, 2026-11-17: approved at `b7619b5`**`
- `- **r2, 2026-11-18: approved at `0c1cad5`**`, whose body says "the
  design approved at `b7619b5` declared `drivers: [qemu, hyperv]`".

`approvals()` returns `[b7619b5, 0c1cad5, b7619b5]`, so the baseline is r1's
`b7619b5`, not r2's `0c1cad5`. The queue shows `f2-plan-r2` done
(`queue/detached-prepare-windows-exe.yaml`). Its Plan gate compared against
r1, and passed only because `impactDelta` (`review.go:242-268`) does not
compare `drivers`, the one field r2 changed. `wf declare` would also be
handed r1's front matter as the approved design. The same shape nearly hit
driver-contract r3 on 2026-11-20, where the prose was reworded to avoid it.

### B. Only the tick can confirm a merge

A merge step (`x3-merge`, `f7-merge`) and Publish's `f6-merged-<n>` are
`cloud` steps (`queue/phases.go:68`, `:78`; `seedMergedFirst`). `wf next`
never offers them (`queue/next.go:224-227`). They complete only by `wf
done` in the tick's merge confirmation (`workflow-tick/SKILL.md:87-96`).

The one thing that stops an attended session doing the same is the affinity
check in `stepCmd` (`cmd/wf/main.go:586`):

```go
if o.worker == "" || !queue.Satisfies(o.worker, st.Affinity) {
```

`Satisfies` is an exact match on the worker's kind (`next.go:30`). Nothing
after it depends on the worker. `finish` (`main.go:647-670`) runs `Early`,
then the gate's done-when (`IsGate` and `runChecks`, which for a merge step
includes `merged()` in `check/check.go`), or `check.MergedFirst` for an
`f6-merged-<n>`. It then runs `recordMerges` and `Complete`.

So `workflow-human` says the session "cannot complete a `cloud` merge step,
so this session does not confirm merges: the next tick does"
(`SKILL.md:14-16`, and step 5 at `:140-142`). On 2026-11-20, PR #130
(loop-repository) merged at 19:08Z. The attended session could not confirm
it, and at `366abd3` its `x3-merge` is still pending. Every hand-added
`x4-state` and every `x4-close` waits on the next daily tick.

### C. The state step is added and linked by hand

The process template seeds only `x1-design`, `x2-implement`, `x3-merge`
and `x4-close` (`phases.go:74-80`). Four items needed work on the `workflow`
branch after their merge: loop-parallel, loop-streamline-2, queue-positions
and loop-native-lead. Each had an `x4-state` stanza written into its queue
file at review. Three also had `needs: [x4-state]` added to `x4-close` by
hand once phase 4 seeded it (`queue-positions.yaml:50-51`,
`loop-native-lead.yaml`, `done/loop-streamline-2.yaml`). loop-parallel's
`x4-close` never got it (`loop-native-lead.md:521-522`).

A hand-added `x4-state` sits pending from phase 1 and is offered early,
since human steps are (`Offered`, `next.go:54-56`). Only its note's first
words stop it being done before the merge.

## Prior art

- **`loop-streamline-2.md`** named B and C as follow-ups ("What this item
  does not do"). It judged B "not small" on the assumption that the merge
  step's affinity must change. Decision 2 below avoids that: the affinity
  stays `cloud`.
- **`queue-positions.md`** and **`loop-native-lead.md`** wrote the
  `x4-state` pattern this item automates. queue-positions shipped its own
  migration (`wf migrate -done`, `main.go`) and named it in its checklist.
  That is the model Decision 5 keeps.
- **`flow19-design-before-plan.md`** fixed the review's place and the delta
  rule. A changes only how `wf` finds the approval FLOW-19 already
  requires, not what it requires.
- **`design-approved-rule-changes.md`** made each rule change approved by
  name. Rule-change approvals (`ruleApprovedRe`, `rulechange.go:68-69`) are
  left as they are (Decision 1).
- **FLOW-08** is unchanged: confirming a merge records it and never makes
  one. The human's merge stays the sign-off.

## Decisions

1. **An approval is the first design approval in a review entry (a1).** A
   review entry is a top-level bullet of the Design review section whose
   head is the entry's ID: `r<N>`, bold or not, optionally after
   `Re-review`. Its approval is the first `approved at <sha>` in the whole
   bullet once rule-change approvals are removed. Later mentions in the same
   bullet, and bullets that are not entries, are ignored. The latest
   approval is the last entry's, in document order. `reviewed()` counts
   entries, not mentions. Rule-change approvals keep their current reading:
   `cleared` and `ownApproval` pass only when the cited commit really has
   the entry unchanged, so a mention cannot make them wrong.
   - *Rejected: any bullet whose head says "approved" (F).* It matches all
     46 archived designs, but a bullet such as "- Unchanged and still
     approved: ... approved at `<old>`" would still count.
   - *Rejected: a strict regex for the canonical head (S).* It breaks the
     test fixtures (`- r1: approved at`) and the legacy forms.
2. **A human worker may complete a merge step; it still never claims one
   (a3, a4).** In `stepCmd`, `done` accepts a worker of kind `human` (as
   well as `cloud`) on a step for which `queue.IsMerge` or
   `queue.MergedFirst` holds. The step's affinity stays `cloud`. Claim,
   release and renew stay affinity-exact. FLOW-05's order, the limits, `wf
   status`, the lead relabelling and the templates are untouched, and no
   queue file changes.
   - *Rejected: a new affinity kind for merge steps.* It touches
     `affinityRe`, the templates, `Seed`, the waiting-on-human limit,
     `summary` and lead relabelling, and needs a migration of every open
     queue file.
   - *Rejected: a new `wf confirm` command.* It is a reasonable later
     cleanup of the tick's step-2 loop, but it triples the Go.
3. **`wf done` refuses a step that is already done (a5).** With two
   confirmers, the tick and an attended session, only one completes the
   step. `wf commit`'s push-rejected path re-reads the state, so a lost
   race ends in this refusal, not a duplicate history line.
4. **The state step is a queue field, seeded at Close (a6, a7).**
   `Feature.StateStep` (`state_step`, the checklist text) is set at review
   by a new human-only command, `wf state-step -feature F -note TEXT
   [-clear]`. When `Seed` enters a process item's phase 4 and the field is
   set, it adds `x4-state` (phase 4, `human`, note = the field) before it
   links added steps. The process `close` template gets `waitsAdded`, so
   `x4-close` needs every pending added step of phase 4, `x4-state`
   included, through the existing `linkAdded`. A step that already exists
   with that ID (a hand-added one) is not added again.
5. **The migration stays a human checklist (a8).** There is no generic `wf
   migrate` run by `x4-close`. An item that needs a mechanical migration
   ships its own subcommand and names it in its checklist, as
   queue-positions did. The lease-precondition done-when check is deferred.
   The FLOW-19 mechanical-rebase exemption is split off to
   `wf-state-migration-step` (Jeff, 2026-11-20).
6. **The attended session runs only human steps after a merge.** After
   confirming, it offers the human steps the merge unblocked (a seeded
   `x4-state`). `x4-close` stays `cloud`, for the tick or an attended tick
   Jeff starts.
7. **`wf stale` names the command.** For an item at its merge step it
   prints `merged: F (confirm: wf done -feature F -step S)`, not the bare
   name.

## Questions

### Blocking

None.

### Assumed

a1 to a9 are the cross-check's Q2 to Q10.

- **a1. Which text counts as a design's approval** (cross-check Q2).
  Proceeding with (a); confirm.
  - (a) H: the first non-rule-change `approved at` in each top-level bullet
    whose head is `r<N>` (bold or not; `Re-review` allowed). The latest is
    the last such entry in document order. Other approvals in the same
    bullet, and bullets without an `r<N>` head, are ignored.
  - (b) F: the first one in each top-level bullet whose head contains
    "approved".
  - (c) S: a strict regex for the canonical head `- **r<N>, <date>:
    approved at <sha>`.

  Recommended: (a). It fixes detached-prepare-windows-exe, gives the
  current answer on every other open branch (driver-contract `33a3563`,
  prebuilt-artifacts `7a85a33`, workdir-in-context `26f6066`), and needs no
  fixture change. Two archived designs with no `r<N>` head
  (hyperv-relay-per-build-channel, human-review-shows-design) stop
  matching; no check reads an archived design again (`check.go:62-72`,
  `:222-230` load only the active item's).
- **a2. Where the review-entry head format is stated** (cross-check Q3).
  Flagged: option (b) changes FLOW-19's wording. Proceeding with (a);
  confirm.
  - (a) `TEMPLATE.md` (Design review comment, `:120-124`) and
    `workflow-human` step 3.5 only.
  - (b) Also one sentence in FLOW-19, as a third `rule_changes` entry
    (wording under Rule changes).
  - (c) A code comment only.

  Recommended: (a). FLOW-19 already requires `approved at <sha>`; where it
  sits in the entry is a format detail the template carries. This keeps the
  item at two rule changes.
- **a3. How an attended session completes a merge step** (cross-check Q4).
  Flagged: changes FLOW-04's and FLOW-03's wording. Proceeding with (a);
  confirm.
  - (a) `stepCmd` lets a human worker run `done` (only) on an `IsMerge` or
    `MergedFirst` step; the affinity stays `cloud`.
  - (b) A new affinity kind for merge steps (a migration of templates,
    limits, summary and state files).
  - (c) A new `wf confirm` command that both the tick and the attended
    session run.

  Recommended: (a). About 15 lines of Go, no state migration, and the
  done-when check that caught PR #82's empty Archive summary runs unchanged.
- **a4. Which workers may confirm, and which items** (cross-check Q5).
  Flagged: sets the scope of the new FLOW-04 sentence. Proceeding with (a);
  confirm.
  - (a) Human (plus cloud), for any item whose merge check passes.
  - (b) Any worker kind, `host:<name>` included.
  - (c) Human, only for a PR merged during the sitting.

  Recommended: (a). The check is the proof, and "merged during the sitting"
  cannot be checked by code; the skill runs the confirmation after each
  merge Jeff makes and on the `merged:` lines `wf stale` prints. A native
  machine gains nothing: its scheduled run is no more frequent than the
  tick, an attended native sitting has Jeff, who can run as `human/jeff`,
  and a native checkout may lag `origin`.
- **a5. A second confirmation** (cross-check Q6). Proceeding with (a);
  confirm.
  - (a) `wf done` refuses a step that is already done.
  - (b) Leave as is: the check re-runs and a duplicate `done` line is
    logged.

  Recommended: (a). A 3-line guard keeps history clean, and `wf commit`'s
  re-read absorbs a lost race.
- **a6. How the state step is declared** (cross-check Q7). Flagged: changes
  FLOW-03's process Close row. Proceeding with (a); confirm.
  - (a) A `state_step` queue field set at review by a human-only `wf
    state-step`; `Seed` adds `x4-state` at phase 4 unless a step with that
    ID exists.
  - (b) A design front-matter key that `wf` reads at `x1-design`.
  - (c) Keep adding `x4-state` by hand; automate only `x4-close`'s need.

  Recommended: (a). `wf` does not run `declare` on process items, so (b)
  needs a new read path in the x1 check, and (c) keeps the hand edit. Since
  the step is seeded only after the merge is confirmed, B lets Jeff do it in
  the same sitting.
- **a7. How `x4-close` comes to wait for `x4-state`** (cross-check Q8).
  Proceeding with (a); confirm.
  - (a) `waitsAdded` on the process `close` template, reusing `linkAdded`.
    The comments on `waiter` (`phases.go:168`) and `rerunPublishHostSteps`
    (`:588-592`) are generalized, since that function finds "Publish" as
    the phase whose template has `waitsAdded` (see Design, the waiter
    coupling).
  - (b) `Seed` adds `needs: [x4-state]` to `x4-close` when it seeds
    `x4-state`; `waiter` keeps meaning Publish only.

  Recommended: (a), with the comment fix and a test that `Back` on a
  process item queues no re-runs. It also covers any other phase-4 step a
  design adds. Choose (b) if `waiter` should keep meaning Publish only.
- **a8. How much of wf-state-migration-step's filed source this item
  takes** (cross-check Q9). Proceeding with (a), Jeff's word of
  2026-11-20; confirm.
  - (a) Only the seeded step (C(1)). The migration stays a human checklist
    (C(2)); the FLOW-19 mechanical-rebase exemption (C(3)) is split off to
    `wf-state-migration-step`; the lease-precondition check is deferred.
  - (b) Also let the seeded-step mechanism take a phase, so a step such as
    `x2-predecessors-merged` can be seeded too (about 15 more lines).
  - (c) All of it: a generic `wf migrate`, the precondition check and the
    FLOW-19 exemption.

  Recommended: (a). The exemption needs a structured format for approved
  text, a comparator (200 to 400 lines of Go) and MUST wording on approval
  integrity, and its case has happened once.
- **a9. detached-prepare-windows-exe, whose `f2-plan-r2` compared against
  r1** (cross-check Q10). Proceeding with (a); confirm.
  - (a) No action; cite it as evidence here.
  - (b) Re-run `wf check -phase 2` on its branch after the fix.
  - (c) Edit its Design review prose now.

  Recommended: (a). `impactDelta` does not compare `drivers`, so the
  outcome would be the same, and the item is past `f6-merge-main`.

## Design

### A. Entry-scoped approvals (`check/review.go`, `check/declared.go`)

`approvals(d)` becomes entry-aware (about 35 lines):

1. Split the Design review section into top-level bullets. A bullet starts
   at a line beginning `- ` in column 0 and runs to the next such line, a
   `#` heading, or an unindented line that is not a bullet. Indented lines,
   nested bullets included, belong to their parent.
2. Remove rule-change approvals (`ruleApprovedRe`) from the bullet first.
3. The bullet is a review entry when its head (the leading `**...**` span,
   else its first line) matches `^(?:Re-review\s+)?r\d+\b`.
4. The entry's approval is the first `approvedRe` match in the whole
   bullet. This keeps the wrapped form (`- r1: approved\n  at`, the
   existing `TestApprovalMayWrap`) and the legacy `**r1, <date>:
   approved** ... Approved at <sha>` working.
5. `approvals` returns one sha per entry with an approval, in document
   order. A new `lastApproval(d) (string, bool)` replaces `a[len(a)-1]` at
   `review.go:91`, `review.go:191` and `declared.go:79`.

`reviewed()` then counts entries with an approval. An item whose re-review
entry records no approval but cites an older one now fails, as it should.
None of the five open branches is affected.

`TEMPLATE.md`'s Design review comment and `workflow-human` step 3.5 state
the head: "each review is one top-level bullet led by `r<N>, <date>:
<outcome>`; for an approval, the head reads `approved at <sha>`, and an
earlier approval cited in the entry comes after it". `wf-plan.md:11`,
`wf-design.md:59` and `workflow-human/SKILL.md:63` change "the last
`approved at <sha>`" to "the newest review entry's `approved at <sha>`".

### B. Attended merge confirmation (`cmd/wf/main.go`, `queue/next.go`)

- **`queue.MayComplete(worker string, st *Step) bool`**, beside
  `Satisfies`: true when `Satisfies` holds, or when the worker's kind is
  `human` and `IsMerge(st)` or `MergedFirst(st)` holds.
- **`stepCmd`**: for `cmd == "done"`, the affinity test is `MayComplete`;
  claim, release and renew keep `Satisfies`. Before `finish`, `done` refuses
  a step whose status is `done`: "`<step>` is already done; nothing to
  confirm".
- **`finish`** is unchanged. A human `done` on `x3-merge` runs the phase 3
  done-when (`merged()`), `recordMerges` and `Complete`, which enters phase 4
  and calls `Seed`. Under C, `Seed` adds `x4-state` there.
- **`staleMerged`** prints `merged: F (confirm: wf done -feature F -step
  S)`, S being the item's pending merge step.
- **Comments** that say only the tick confirms are rewritten:
  `next.go:88-90` and `:175-176`, `phases.go:67` and `:226-227`, and
  `staleMerged`'s (`main.go:1192-1198`).
- **`workflow-human`**: Set up (`SKILL.md:14-16`) says the session confirms
  a merge Jeff makes in the sitting, and any `merged:` line `wf stale`
  prints. Step 5 (`:140-142`): after each merge, `dist/wf main`, `dist/wf
  stale`, then `dist/wf done -state .dev/workflow -repo . -checkouts
  .dev/checkouts -worker $W -feature F -step <x3-merge|f7-merge|f6-merged-n>`,
  `dist/wf commit`, and then the human steps the merge unblocked (a seeded
  `x4-state`). A failed check leaves the step pending; say so and go on.
- **`workflow-tick`** step 2 notes that a merge step may already be done by
  an attended session; it iterates only pending steps already.

The session must read fresh refs: `merged()` reads the clone and the local
refs. A stale checkout fails safe (the step stays pending). Step 5 already
runs `wf main` and `wf stale`, which fetch, before it confirms.

**FLOW-08 is unchanged.** Confirming runs the same check the tick runs and
writes the merge the human made into the queue. No agent merges, approves
or enables auto-merge. The human's merge is still the sign-off.

### C. The seeded state step (`queue/queue.go`, `queue/phases.go`, `cmd/wf/main.go`)

- **Field.** `Feature.StateStep string \`yaml:"state_step,omitempty"\``.
  `Validate` refuses it on an item that is not a process item.
- **Command.** `wf state-step -feature F -note TEXT [-clear] -worker
  human/<name>`. It refuses a worker that is not `human`, a non-process
  item, an empty note, and a done item. It writes one history line ("state
  step recorded: ..."). While the item is before Close it only records the
  field. In Close with `x4-close` still pending, it also adds `x4-state` at
  once and adds it to `x4-close`'s needs. It follows `wf urgent` and `wf
  move`.
- **Seed.** For a process item entering phase 4 with `StateStep` set and no
  step `x4-state<suffix>`, `Seed` appends `x4-state<suffix>` (phase 4,
  `human`, pending, note = `StateStep`) **before** it calls `linkAdded`, so
  it is among the added steps.
- **Template.** The process `close` becomes `{"close", "cloud", nil,
  true}` (`phases.go:79`). `x4-close` then needs every pending added
  phase-4 step. `IsGate` is unaffected: it counts only template steps'
  needs.
- **Landed.** `Landed` calls `Seed` and then appends
  `x4-escalate-returned` (`phases.go:413-416`). So a landed item with a
  state step gets `x4-state`, linked, and the escalation stays unlinked, as
  today.
- **Review.** `workflow-human` step 3.5: when the design says the item needs
  work on the `workflow` branch after the merge, record it with `dist/wf
  state-step` at the outcome, the note being that checklist. The hand-added
  stanza and the hand edit of `x4-close` are gone from the procedure.
  `workflow-intake`'s size row (+1 step per expected state step) stays.

#### The waiter coupling

`waiter()` (`phases.go:168-176`) is used in three places: `Seed`
(`:440`, through `linkAdded`, and `:456`), `linkAdded` (`:495`), and
`rerunPublishHostSteps` (`:593-598`). The last finds "Publish" as the phase
whose template has `waitsAdded`. With `waitsAdded` on the process `close`,
that lookup resolves to phase 4 for process items.

It does no harm today: process items get no host steps (their native work
is done in an attended native session or tracked as gaps,
`workflow-intake/SKILL.md:96-98`), so nothing is re-run. But the comments at
`:168` ("Publish's gate") and `:588-592` become wrong, and a future host
step in a process item's Close would be re-run on `Back`. The change
rewrites both comments ("the phase whose gate waits for added steps") and
adds a test that `Back` on a process item queues no re-runs. Under a7 (b)
instead, `waiter` keeps its meaning and `Seed` writes the need directly.

### What this item cannot do for itself

The `wf` that confirms this item's merge is not the one it ships until
`dist/wf` is rebuilt from the merged `main`, and an old `wf` drops an
unknown `state_step` field when it saves a file. So this item still needs
one hand-added `x4-state`, and `x4-close` must have `needs: [x4-state]`.
If the tick that confirms the merge builds `dist/wf` from the merged `main`
(its step 1 does, at each run), `waitsAdded` adds the need; if not, it is
added by hand. It is the last item that needs either.

This item's `x4-state` (`human`, phase 4, added by hand at review), on the
`workflow` branch through `dist/wf commit`, once `dist/wf` is rebuilt from
the merged `main`:

1. Check that `x4-close` needs `x4-state`; add it by hand if not.
2. `README.md:202` (the `f7-merge` row): "never offered by `wf next`;
   confirmed by the tick or by an attended session's `wf done`".
3. `README.md:205-210` (the process round): `x3-merge` the same, and
   `x4-state` (human, seeded at Close from `state_step`; `x4-close` waits
   for it).
4. `README.md:224` (the refresh paragraph): the merge step is the cloud's,
   and a human worker may also complete it with `wf done`.
5. `README.md:379` (other commands): add `wf state-step` (human only).
6. `wf validate`, then `wf commit`.

### Overlaps with other items (FLOW-05, FLOW-10)

This item declares `loop`, which is exclusive, so FLOW-05 orders it with
the other loop items. `wf-state-migration-step` (priority 6) is filed for
the split-off exemption and touches FLOW-19, not FLOW-03 or FLOW-04. If
another item amends FLOW-03's tables or FLOW-04's lead paragraph first,
the implement step re-applies the approved text over the merged wording
under FLOW-19's delta rule.

### What this item does not do

- The FLOW-19 mechanical-rebase exemption (`wf-state-migration-step`).
- A generic `wf migrate` run by `x4-close`, or a done-when check on
  `x4-state` (lease precondition).
- Seeding human steps in phases other than Close (a8 (b)).
- Letting a host worker confirm a merge (a4 (b)).
- Re-checking detached-prepare-windows-exe (a9).

## Defaults and overrides

- **Who may complete a merge step.** Cloud and human workers. No override:
  the done-when check is the same for both.
- **The review entry's approval** is its first design approval. No
  override; a citation belongs after the entry's own approval.
- **A state step.** None by default. Set only by the human with `wf
  state-step`, at review or later.
- **x4-close waits for added Close steps.** Every pending step added to a
  process item's phase 4 before Close is seeded, `x4-state` included. A
  step added later is not linked (as for Publish). No fallback.

## Cross-driver impact

None. No driver code is reached.

## Rule changes

Severity and check are unchanged for both: FLOW-03 MUST, `automated`;
FLOW-04 MUST, `automated`.

### FLOW-03 (amend), `docs/development/process.md`

**Why:** the phase tables say who confirms a merge and what Close waits
for (Decisions 2 and 4). One entry covers the three rows.

**Current wording** (`process.md:158`, the feature table's phase 7 row):

> | 7 | Merge | The human merged the PR (FLOW-08). The loop confirms it; there is no step for the human to claim. The PR is ready for the merge only while its branch contains `main` (FLOW-08); when `main` moves past it, a refresh step brings it up to date (FLOW-10). |

**Proposed:**

> | 7 | Merge | The human merged the PR (FLOW-08). The loop confirms it, in an unattended run or in an attended session (FLOW-04); there is no step for the human to claim. The PR is ready for the merge only while its branch contains `main` (FLOW-08); when `main` moves past it, a refresh step brings it up to date (FLOW-10). |

**Current wording** (`process.md:204-205`, the process table's phase 3 and
4 rows):

> | 3 | Merge | A human merged it into `main` (FLOW-08). Its PR is ready and is refreshed as a feature's is (FLOW-08, FLOW-10). |
> | 4 | Close | As phase 8. |

**Proposed:**

> | 3 | Merge | A human merged it into `main` (FLOW-08), confirmed as phase 7's merge is. Its PR is ready and is refreshed as a feature's is (FLOW-08, FLOW-10). |
> | 4 | Close | As phase 8, and its state step done when its design review recorded one: a `human` step that brings the `workflow` branch in line with the merged change. Only the human records it; the queue's tooling adds it when Close is entered, and Close's own step waits for it. |

**Enforcement:** `Seed` and the process `close` template (`phases.go`),
`StateStep` and `Validate` (`queue.go`), `wf state-step` and `stepCmd`
(`cmd/wf/main.go`).

### FLOW-04 (amend), `docs/development/process.md`

**Why:** a merge step stays `cloud`, but an attended session may complete
it (Decision 2, a3, a4). Without this sentence the rule says a worker takes
only steps whose affinity it satisfies, which `wf done` would now depart
from.

**Current wording** (`process.md:241-246`, the lead paragraph; unchanged,
and the new paragraph follows it):

> A feature may be **led** by a host name, which any worker may set or
> change, each change recorded in the item's history and the run's digest
> (FLOW-09). Its steps of Branch setup through Guide that would be `cloud`
> carry that `host:<name>` instead; its design review stays `human`, and its
> Publish, Merge and Close stay `cloud`. A claim by a worker that serves the
> lead starts a queued led feature, and no other claim does.

**Proposed:** unchanged, followed by a new paragraph before "A worker takes
only steps whose affinity it satisfies":

> A merge step, and a step confirming that a pull request its design names
> to merge first has merged (FLOW-08), stays `cloud` and is never claimed.
> A `human` worker may also complete one, which is the one exception to
> taking only steps whose affinity a worker satisfies: the same done-when
> check runs, so the step completes only once the merge has happened, and a
> step already done is not completed again. Completing it records the
> human's merge; it never makes one (FLOW-08).

**Enforcement:** `queue.MayComplete` and the done-twice refusal in
`stepCmd` (`cmd/wf/main.go`).

### FLOW-19 (only under a2 (b)), `docs/development/process.md`

Not in `rule_changes` under the recommendation. If a2 (b) is chosen, a third
entry `{rule: FLOW-19, change: amend, paths: [docs/development/process.md]}`
is added before the approval, and the sentence after "(`approved at
<sha>`)." in the paragraph at `process.md:871-877` is:

> Each review is one top-level entry led by its number (`r<N>`); its
> approval is the first `approved at <sha>` in that entry, and an earlier
> approval it cites comes after it.

**Enforcement:** `approvals` and `lastApproval` (`check/review.go`).

## Validation plan

- **Tier 0** (TEST-01, TEST-02; `make test`). States are built with `Add`,
  `Claim`, `Complete`, `Back`, `Landed` and `Seed`, never by setting
  `Phase` by hand.
  - `check`: a table test over `approvals()`: prose citing r1 inside the r2
    entry (the detached-prepare-windows-exe shape) gives `[r1, r2]`; a
    separate `- **Decisions.** ... approved at <old>` bullet is ignored; a
    nested citation bullet is ignored; the legacy `**r1, <date>:
    approved** ... Approved at` wrapped onto the next line counts; `- r1
    addendum: rule change X approved at` counts as no design approval; a
    `###` heading ends an entry; two entries, the last wins. A `reviewed()`
    case: r2 seeded, the r2 entry without an approval but citing r1, fails.
    An `approvalDelta()` case and a declaration case: the base is r2's sha
    when the r2 body cites r1. `TestApprovalMayWrap` and the rule-change
    tests pass unchanged.
  - `queue`: `MayComplete` for human, cloud and host workers on `x3-merge`,
    `f7-merge`, `f6-merged-1` and an ordinary cloud step. `Seed`: a process
    item entering phase 4 with `state_step` gets `x4-state` with the note,
    and `x4-close` needs it; without it, no `x4-state` and no need; with a
    hand-added `x4-state` already present, no duplicate and the need is
    added; `Landed` with `state_step` gives a linked `x4-state` and an
    unlinked `x4-escalate-returned`. `Back` on a process item queues no
    re-runs (the waiter coupling). `Validate` refuses `state_step` on a
    feature.
  - `cmd/wf` (modelled on `TestDoneRunsTheRefreshCheck`,
    `stale_test.go:394`, and `TestTrackedDone`, `main_test.go:387`): `wf
    done -worker human/jeff` on `x3-merge` passes when the branch is merged
    and fails with the check's output when not; `wf claim` of `x3-merge` by
    a human is still refused; a second `done` is refused; after a human
    confirmation with `state_step` set, `wf next -worker human/jeff` offers
    `x4-state`. `wf state-step`: human only, process only, empty note
    refused, `-clear`, done item refused, the immediate add in Close. `wf
    stale` prints the confirm command.
- **Text checks:** `grep` finds no "the next tick does" or "cannot complete
  a `cloud` merge step" in `.claude/`, and no "the last `approved at`" in
  `.claude/`; the FLOW-03 and FLOW-04 blocks match the approved wording
  byte for byte.
- `make test`, `make lint`, `tools/workflow/flow18-check.sh` and `make
  enforcement` green.
- **The real check:** the first sitting after the merge where Jeff merges a
  process item with a state step: merge, `wf done`, `x4-state` done, and
  only `x4-close` left for the tick.
- No native host is needed: nothing here differs by host.

## Implementation plan

A process item has no Plan phase; this is the work of `x2-implement`.

1. Tests first (Validation plan); they fail on the current code (TEST-01,
   TEST-02).
2. `check/review.go`: entry-aware `approvals`, `lastApproval`, and its use
   at `review.go:91` and `:191`; `check/declared.go:79` (FLOW-19, Decision
   1, a1). Tests: the `approvals` table test, the `reviewed`,
   `approvalDelta` and declaration cases.
3. `queue/next.go`: `MayComplete` beside `Satisfies`, and the comments at
   `:88-90` and `:175-176` (FLOW-04, Decision 2, a3, a4). Test: the
   `MayComplete` table.
4. `cmd/wf/main.go`: `stepCmd` uses `MayComplete` for `done` and refuses a
   done step (Decisions 2 and 3, a5); `staleMerged`'s line and comment
   (Decision 7). Tests: the `cmd/wf` confirmation cases and the `wf stale`
   line.
5. `queue/queue.go`: `StateStep` and `Validate` (FLOW-03, Decision 4, a6).
   `queue/phases.go`: `Seed` adds `x4-state` before `linkAdded`; the
   process `close` template takes `waitsAdded`; the comments on
   `waitsAdded`, `waiter`, `rerunPublishHostSteps`, `IsMerge` and the
   template (`:41-43`, `:67`, `:168`, `:226-227`, `:588-592`) (a7). Tests:
   the `Seed`, `Landed`, `Back` and `Validate` cases.
6. `cmd/wf/main.go`: `wf state-step` and its usage line (Decision 4, a6).
   Tests: the `wf state-step` cases and `wf next` offering `x4-state` after a
   human confirmation.
7. `docs/development/process.md`: FLOW-03's three rows and FLOW-04's
   paragraph as approved (FLOW-14).
8. `docs/design/TEMPLATE.md` Design review comment; `.claude/agents/wf-plan.md:11`,
   `.claude/agents/wf-design.md:59`; `.claude/skills/workflow-human/SKILL.md`
   (Set up, `:63`, step 3.5's entry head and `wf state-step`, step 5's
   confirmation); `.claude/skills/workflow-tick/SKILL.md` step 2's note
   (Design, A, B and C).
9. `make test`, `make lint`, `flow18-check.sh`, `make enforcement`, the
   text checks.
10. Merge current `main`, archive this design with its summary, and open the
    PR with CI green (FLOW-03 process phase 2, FLOW-12).

After the merge, `x4-state` as under "What this item cannot do for itself".
The review adds it to the queue by hand when it records the outcome; it is
the state step of the 5-step estimate (4 steps + 1, one run).

## Design review

Pending.

## Consolidation

A process item has no Publish phase and no consolidation pass (FLOW-03). The
implement step merges current `main` before it opens the PR.

## Rollback notes

None.

## Archive summary

Filled in at `x2-implement` (FLOW-12).
