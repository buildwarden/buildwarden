---
feature: loop-streamline-2
status: draft
drivers: []              # the loop's review procedure, intake guide and run counting; no driver tree is reached
seams: []
surfaces: []             # `wf` and the skills are the loop's tools, not a user-facing surface of the product
repos: []                # only buildwarden/buildwarden
areas: [rules, loop, designs]  # process.md (FLOW-19); .claude/ skills and wf-design, tools/workflow/queue
                         # and cmd/wf (run counting); docs/design/TEMPLATE.md (the flag)
security_boundary: none
alters_relationships: false  # no product component's role or interaction changes
design_review: required  # a process item's design always gets its review (FLOW-03, FLOW-19)
native_validation: []    # skill text and one pure counting change in the queue code
tier2_gaps: []
rules: [FLOW-03, FLOW-05, FLOW-07, FLOW-08, FLOW-10, FLOW-13, FLOW-14, FLOW-19,
        LOOP-09, GLOBAL-09, GLOBAL-10, GLOBAL-11, TEST-01, TEST-02]
rule_changes:
  - {rule: FLOW-19, change: amend, paths: [docs/development/process.md]}
concepts: [workflow, design-review, workflow-human, workflow-intake, assumptions,
           rule-changes, sitting-load, process-item, estimates, budget, run-counting]
related: [workflow-streamline.md, design-approved-rule-changes.md,
          flow19-design-before-plan.md, human-review-shows-design.md]
---

# Fewer round-trips in the human review

A process item (FLOW-03, LOOP-09). It holds no worker: it changes how the
attended session asks and records, and how `wf claim` counts runs.

## Problem

The 2026-11-17 sitting reviewed 5 designs and 3 intakes in about 2.5 hours
and asked Jeff 108 questions. About 90 were required by the rules as written:

- **61 assumed questions, one per AskUserQuestion entry**
  (`workflow-human/SKILL.md:92`, FLOW-07, FLOW-19 `process.md:733`). Across
  the queue, 242 assumptions were confirmed and roughly 83-86% went as
  recommended. The overrides that remain are substantive (a6 "None of the
  options", a16 "This is overcontrolled"), so they must stay one step away.
- **22 rule-change approvals, each asked on its own** (FLOW-19
  `process.md:731-732`, Jeff's rule of 2026-11-04 in
  `design-approved-rule-changes.md`). All 22 came back "Approve" unchanged.
- **No load limit.** The skill says "one feature per sitting"
  (`SKILL.md:50`). The sitting held five reviews plus intakes done in
  between.

The process items themselves cost little once designed: 23 of the 25 done
ones spent exactly 3 counted steps, median 1 run. The queue does not show
this, for three reasons:

- The intake size guide has no process row (`workflow-intake/SKILL.md:56`),
  so queue-positions was estimated at 12 steps / 2 runs by analogy with a
  feature.
- x1-design is claimed and done in the same second at the end of the sitting
  (`queue/loop-repository.yaml:75-78`). Behind that step were 35 asks.
- Three done items show 0 runs spent (loop-parallel, alters-slot,
  relay05-specs-wording). `wf claim` counts a run only when the caller passes
  `-new-run`, and only the tick's skill passes it.

A trivial process change also costs two sittings: intake now, and the design
review in a later session.

## Prior art

- **`workflow-streamline.md`** cut steps, PRs and gates for features. This
  item does the same for the human review and keeps its rule: one design
  review and one owner merge per change (LOOP-09).
- **`design-approved-rule-changes.md`** is where each rule change came to be
  asked on its own (its Decision 3, Jeff's rule of 2026-11-04: "reviewed
  explicitly in the design review"). Batch approval would reverse it, so it is
  offered as an option in a2 and not assumed.
- **`human-review-shows-design.md`** made the design visible before the
  walkthrough and set one AskUserQuestion entry per assumption, with the
  page threshold at more than 4 assumptions (one call's worth). That
  threshold is reused here as the definition of a large design (a3).
- **`flow19-design-before-plan.md`** fixed the review's place (after Design,
  before Plan) and its delta rule. Neither changes.

## Decisions

1. **Assumed questions are confirmed in one answer, except flagged ones.**
   The session shows a table of every assumption with its recommendation,
   then asks one entry: "All unflagged as recommended", "Name exceptions"
   (free text), or "One by one". Each exception, and each flagged question,
   is then asked on its own as today. A question is **flagged** when it
   touches a security boundary or a seam, departs from prior art or an
   earlier decision of Jeff's, or has no recommendation. Each answer is still
   recorded per question with `wf answer` (FLOW-07 is unchanged). On
   11-17 this would have cut 61 asks to about 11.
   - *Rejected: grouping only (up to 4 entries per call, each answered).*
     It already fits the current text, cutting 61 to about 16. It is the
     fallback when Jeff answers "One by one".
   - *Rejected: no flags.* The overrides that carry weight cluster in exactly
     those kinds of question.
2. **Rule changes: per-rule answers, grouped (a2 (a)), unless Jeff takes
   the batch.** Up to 4 `rule_changes` entries per AskUserQuestion call,
   each its own entry with current and proposed wording. 22 round-trips go
   down to about 6, and each change is still approved by name. The batch with
   a veto (a2 (b)) cuts that to about 3 but reverses Jeff's 2026-11-04 rule.
   The proposed FLOW-19 wording below is given for both answers.
3. **A sitting is capped by load.** A sitting takes design reviews in
   FLOW-05 order until the next review would take the sitting past 30 asks.
   A large design (a3) is reviewed alone. At 30 asks the session offers to
   stop, and Jeff may say go on. An intake Jeff asks for mid-review is noted
   and done after that review's outcome.
   - *Rejected: a hard stop.* Stopping is Jeff's call, and a review cut in
     half costs more than finishing it.
4. **Trivial process changes get a same-sitting lane.** When intake files a
   process item that meets a4's criteria, the session offers to draft its
   design and review it right away, as `workflow-human` step 3 does. LOOP-09's
   owner review and owner merge stay, and so does every rule-change ask. No
   rule text changes: nothing requires the review to happen in a later
   session.
   - *Rejected: skip x1-design for trivial items.* That conflicts with
     LOOP-09 and binds every repository under the contract.
5. **The intake size guide gets a process row:** 4 steps / 1 run, plus 1
   step for each expected state step (`x4-state`). The FLOW-13 runaway check
   (2x) then fires at 8 or 10 steps, not 24.
6. **x1-design records its real cost.** It is claimed when the sitting
   starts reviewing it and marked done at the outcome. The Design review entry
   gets one line, `sitting: <start>-<end> UTC, asks: <n> (<m> in batches)`
   (a6).
7. **`wf claim` works out new runs itself.** A claim counts a run when the
   claiming worker is not `human/*` and the item's history has no earlier
   claim by that worker. Cloud workers are named per run (`cloud/<run>`) and
   host workers per session, so this matches "first claim of this feature in
   this run". The `-new-run` flag is removed from `wf claim` and `wf commit`,
   and from the tick skill and the workflow README (a5).
   - *Dropped fallback:* counting a run only when the caller says so.
     Attended ticks and native sessions forgot, which is why three items show
     0 runs.
   - Past history is not rewritten. The shared intake/close timestamps of
     older items stay as recorded.
8. **Merge confirmation in the human session and a `wf` state-migration
   step are not included.** Each is a follow-up (see "What this item does
   not do").

## Questions

### Blocking

None.

### Assumed

- **a1. Who flags a question.** Proceeding with (a); confirm.
  - (a) The design marks it: `Flagged: <reason>` on the question, which
    `wf-design` writes. The review session may add a flag but never remove
    one.
  - (b) The review session judges each question at the sitting.

  Recommended: (a). The flag is then on the page before the sitting and in
  the archive. Under (b) the judgment would happen while Jeff waits.
- **a2. How rule changes are approved.** Flagged: departs from Jeff's rule
  of 2026-11-04 (`design-approved-rule-changes.md`, Decision 3). Proceeding
  with (a); confirm.
  - (a) Each on its own, with up to 4 per AskUserQuestion call, each its own
    entry with current and proposed wording. FLOW-19 gains a clause that
    allows grouping. About 6 round-trips on 11-17.
  - (b) One batch answer with a veto: every entry is shown with its wording,
    and one answer approves all of them, naming any entry rejected or
    changed. Each still gets its own `rule change <ID> approved at <sha>`
    line. About 3 round-trips on 11-17.
  - (c) As today: one call per entry.

  Recommended: (a). Rule text is the loop's MUST layer, and two of 11-17's
  post-approval checks found defects in approved wording, so reading each
  change is still worth the click. (b) is a sound choice if Jeff no longer
  wants that.
- **a3. What makes a design large** (reviewed alone in its sitting).
  Proceeding with (a); confirm.
  - (a) More than 4 assumed questions, or more than 4 rule changes. This is
    the existing page threshold (`SKILL.md:55-56`) applied to both kinds of
    ask.
  - (b) Only the 30-ask cap decides, with no per-design rule.

  Recommended: (a). It reuses a known number, and one large design already
  fills most of a sitting.
- **a4. What counts as a trivial process change** for the same-sitting
  lane. Proceeding with (a); confirm.
  - (a) Jeff states the exact edit at intake; the change is text only (rules,
    skills, agents, template), with no `tools/workflow/` code, no state step
    and no `holds`; it fits the sitting's ask cap.
  - (b) Any process item that Jeff calls small.

  Recommended: (a). Code or a state migration needs a draft that can be
  checked, which is what a separate sitting gives.
- **a5. Run counting.** Proceeding with (a); confirm.
  - (a) `wf` works it out from history (Decision 7) and removes `-new-run`.
  - (b) Keep the flag, and make native sessions and attended ticks pass it.

  Recommended: (a). A flag every caller must remember is what went wrong.
- **a6. Where x1-design's cost is recorded.** Proceeding with (a); confirm.
  - (a) Claim at the start of the sitting's review of it and done at the
    outcome, with the `sitting:`/`asks:` line in the Design review entry. No
    schema change.
  - (b) New queue fields written by `wf done -asks <n>`.

  Recommended: (a). A retrospective reads the design and the history
  anyway, and a field would need a `wf` schema change and a migration.

## Design

### The review sitting (`workflow-human`)

- **Set up / step 3 opening (`SKILL.md:49-50`).** "One feature per sitting"
  becomes the load cap of Decision 3: designs in FLOW-05's order while the
  sitting stays at or under 30 asks, a large design (a3) alone, a stop
  offered at 30, and intakes after the current review's outcome. An *ask* is
  one AskUserQuestion entry Jeff answers.
- **Step 0 and step 3:** claim x1-design (or the design-review step) as
  `human/jeff` when its walkthrough starts, not at the end (a6).
- **3.3 rule changes:** grouped as a2 settles (or batched under (b)).
- **3.4 assumptions:** the table, then the single batch entry of
  Decision 1, then the exceptions and flagged questions on their own, up to
  4 per call. Each is recorded with `dist/wf answer`. An answer from the
  batch is recorded as `as recommended (batch), Jeff, <date>`.
- **3.5 outcome:** the Design review entry gains the `sitting:`/`asks:`
  line.

### Designs (`wf-design`, `TEMPLATE.md`)

`wf-design` step 4 marks each flagged assumed question `Flagged: <reason>`,
for the reasons in Decision 1 (a1). The template's **Assumed** comment says
the same in one line.

### Intake (`workflow-intake`)

- Step 3's size guide gains the process row (Decision 5).
- After step 5, for a process item that meets a4: offer the same-sitting
  lane. On yes, draft the design on the item's branch from the template
  (Problem, Decisions, Questions, Rule changes, Implementation plan, Design
  review) and hold `workflow-human` step 3 on it in this session. Its asks
  count toward the cap.

### Run counting (`tools/workflow/`)

`State.Claim` drops its `newRun` parameter and computes it: true when the
worker does not start with `human/` and no history event by that worker
starts with `claimed `. `wf claim` and `wf commit -claimed` lose `-new-run`;
`commit.go`'s request loses `NewRun`. `workflow-tick` (`SKILL.md:105,
124-125`) drops the flag from its commands.

### The workflow branch (`x4-state`)

The workflow branch's `README.md` documents `-new-run` (lines 197-198), and
line 155 says `wf stale` "first confirms merges". In fact it only names a
merged item at its merge step: `staleMerged` in `cmd/wf/main.go`, "that step
is the cloud's ... an attended session cannot", which is what
`workflow-human/SKILL.md:16` says. Both lines are corrected after the merge,
in an `x4-state` step (`cloud`, needing `x3-merge`, with `x4-close` needing
it). The review adds that step when it records the outcome. This is the
expected state step of the 5-step estimate.

### Overlaps with other items (FLOW-05, FLOW-10)

All four items declare `loop`, which is exclusive, so FLOW-05 orders them.
Whichever merges second rebases on the first and keeps both edits.

- **queue-positions** (being drafted alongside this one) likely edits
  `workflow-intake/SKILL.md` step 3 (the priority sentence, next to the size
  guide), the workflow `README.md`, and `tools/workflow/queue/`.
- **loop-parallel-rule-wording** (approved, unmerged) deletes the `areas.yaml`
  clause in `workflow-intake/SKILL.md:25`, `workflow-human/SKILL.md:72` and
  `workflow-tick/SKILL.md`, and edits `TEMPLATE.md`'s `areas` comment.
- **loop-native-lead** (approved, unmerged) edits `workflow-intake` step 4,
  `workflow-human` (`wf lead`), `workflow-tick`'s claim paragraph, one
  sentence in each phase agent (`wf-design` included), the queue's claim
  code, and the workflow `README.md`.

None of them amends FLOW-19 or FLOW-07, so the wording below is drafted on
main at `0cd3c02`.

### What this item does not do

- **Merge confirmation in the observing human session.** `x3-merge` is a
  `cloud` step by the template (`phases.go`), and `wf stale` deliberately
  leaves it to the tick. Moving it means changing the step's affinity and
  FLOW-04's routing (which loop-native-lead amends), and the done-when check
  must keep running (it caught PR #82's empty Archive summary). That is not
  small. It becomes a follow-up process item.
- **A `wf` state-migration step in place of hand-added `x4-state`.** It
  needs a new subcommand and a scoped exemption from FLOW-19's delta review.
  It becomes a follow-up process item.
- Rewriting past history (0-run spends, shared timestamps).
- The habits the assessment lists as allowed under today's text (fewer
  pre-review checks, drafting dependent designs in sequence). They need no
  item.

## Defaults and overrides

- **The batch answer for unflagged assumptions** is offered by default
  (GLOBAL-09: most answers accept the recommendation). It is overridden by
  Jeff answering "One by one", which asks every assumption on its own for
  that design (GLOBAL-10). There is no fallback.
- **The ask cap, 30 per sitting** (from the item's source). Overridden by
  Jeff's word in the sitting to go on. It is named when reached ("30 asks:
  stop here?"), never applied silently (GLOBAL-11).
- **Large design: more than 4 assumed questions or more than 4 rule
  changes** (a3). No override; Jeff can still choose to hold two reviews in
  one sitting by saying so, which is the ask-cap override.
- **Process size row: 4 steps / 1 run, +1 per state step** (from the source).
  Overridden by the estimate Jeff agrees at intake, as for features.
- **Run counting.** No override. *Dropped fallback:* the caller's
  `-new-run`.
- **The same-sitting lane** is offered only when a4's criteria hold, and
  taken only on Jeff's yes.

## Cross-driver impact

None. No driver code is reached.

## Rule changes

### FLOW-19 (amend), `docs/development/process.md`

Severity MUST and check `automated + agent-judgment`, both unchanged. The
automated part is unchanged: it checks the recorded decision, the skip
criteria and the approvals, and a `rule change` line is still per ID. Only
the two bullets of the opening list change.

Current (main `0cd3c02`, `process.md:731-733`):

> - each `rule_changes` entry (FLOW-14), shown with its current and proposed
>   wording and asked on its own: approved, approved with a change, or rejected;
> - each assumed question, confirmed or turned into a change (FLOW-07).

Proposed, under a2 (a):

> - each `rule_changes` entry (FLOW-14), shown with its current and proposed
>   wording and asked on its own: approved, approved with a change, or
>   rejected. Several entries may be put to the human together, each
>   answered on its own;
> - each assumed question, confirmed or turned into a change (FLOW-07). The
>   human may confirm, in one answer, every assumed question that is not
>   flagged as recommended, naming any exceptions; each exception is then
>   asked on its own. A question is flagged, and always asked on its own,
>   when it touches a security boundary or a seam, departs from prior art or
>   from an earlier decision of the human's, or has no recommended answer.

Under a2 (b), the first bullet instead reads:

> - the `rule_changes` entries (FLOW-14), each shown with its current and
>   proposed wording, approved in one answer that names any entry rejected
>   or approved with a change;

Under a2 (c), the first bullet is unchanged.

**Why:** the review's grain is set in the rule (FLOW-14 puts the substance
in the rule, not the skill), and the skill text must not depart from it.
FLOW-07 needs no change: assumptions are still "confirmed in the design
review", each one recorded.

## Validation plan

- **Tier 0** (TEST-01, TEST-02): `queue` tests for `Claim`. The first claim
  by `cloud/A` counts a run, a second claim by `cloud/A` (a retry after a
  release) does not, a claim by `cloud/B` does, a `human/jeff` claim never
  does, and a `host:<name>/<s>` first claim does. A `cmd/wf` test shows
  `-new-run` is refused as an unknown flag.
- **Text checks:** grep that no `-new-run` remains in `.claude/`, `tools/`
  or the README after `x4-state`; FLOW-19's two bullets match the approved
  wording; the size-guide row and the cap appear once each.
- `make test`, `make lint`, `make enforcement` green.
- **The real check:** the next sitting held under the new skill. It records
  its `sitting:`/`asks:` line, and its ask count should land near the
  estimates above (61 to about 11, 22 to about 6).
- No native host is needed: nothing here differs by host.

## Implementation plan

1. FLOW-19's two bullets as approved (FLOW-14, LOOP-09).
2. `workflow-human`: the cap and intake deferral (step 3 opening), the claim
   at the start (steps 0 and 3), grouped or batched rule changes (3.3), the
   batch answer and flags (3.4), and the `sitting:`/`asks:` line (3.5).
3. `wf-design` step 4 and `TEMPLATE.md`: `Flagged: <reason>` (a1).
4. `workflow-intake`: the process size row, and the same-sitting lane (a4).
5. `tools/workflow/queue/next.go` `Claim`, `commit.go`, `cmd/wf/main.go`:
   run inference, `-new-run` removed; tests as in the Validation plan;
   `workflow-tick` lines 105 and 124-125 (a5).
6. Archive this design on the branch with its summary; PR opened with CI
   green (FLOW-03 process phase 2, FLOW-12).
7. After the merge, `x4-state`: the workflow `README.md` lines 155 and
   197-198, through `dist/wf commit`.
8. File the two follow-ups of "What this item does not do" at intake, on
   Jeff's word.

## Design review

None yet.

## Consolidation

## Rollback notes

## Archive summary
