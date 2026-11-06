---
feature: budgets-active-only
status: draft
drivers: []
seams: []
surfaces: []
security_boundary: none
alters_relationships: false
design_review: required
native_validation: []
tier2_gaps: []
rules: [FLOW-03, FLOW-04, FLOW-05, FLOW-13, FLOW-14, FLOW-19]
rule_changes: []
concepts: [workflow, budget, runaway, digest, queue-summary, wf-status, retrospective]
related: [queue-summary.md, workflow-bootstrap.md, workflow-streamline.md]
---

# Budgets gate open work only; done items wait for a retrospective

## Problem

The per-feature budget exists to stop a runaway loop (FLOW-13). The digests
use it as a scorecard instead, and they keep scoring items that are finished.
In `digest/2026-11-04.md` on the `workflow` branch:

- The cloud run lists, under **Blocked on Jeff → Runaway budgets (FLOW-13)**,
  "bootstrap-firecracker-row: 3 runs against 1. It is done." and
  http-body-path-drift at "3 runs against 1 and 12/9 steps, over 2x on
  runs".
- The attended run lists human-review-shows-design ("2 runs against 1 ... It
  is done") and http-body-path-drift ("4 runs against 1, 13 steps against 9.
  It is done") the same way.
- Both runs' **Spent → Per feature, spent against estimate** list the done
  items with ", done" appended. The cloud run's **Process items** also lists
  wf-squash-merged-pr, done the day before.

None of these was a runaway as the code defines it. `State.Runaway`
(`tools/workflow/queue/queue.go:409-418`) counts steps only and fires at
`runaway_factor` (2, `config.yaml`) times the step estimate. 13 of 9 steps is
below 18, and 3 of 4 is below 8. The digest writer flagged runs on its own
reading, and runs overrun for reasons that are not loops, such as a design
reviewed on one day and implemented the next. A done item cannot loop at all,
so listing it under **Blocked on Jeff** asks him to decide something there is
nothing to decide about.

Jeff's rule (2026-11-05): "generally speaking all archived/done features should
be set aside and fully ignored except during any explicit retrospectives.
Over-budget only matters when it comes to being a gate against rampant loops
that might otherwise drastically go over."

Where done items show up today:

- **The `workflow-tick` skill's Digest section**
  (`.claude/skills/workflow-tick/SKILL.md:138-147`) asks for "runaway budgets
  (FLOW-13)" under Blocked on Jeff and "each touched feature's spent against
  estimate, in steps and runs" under Spent. Neither line says to leave out
  done items, and the closing step of a done item counts as touching it.
- **`wf status`** (`tools/workflow/cmd/wf/main.go:160-181`) prints every
  feature in the queue, done ones included, each with its steps and runs
  against estimate. Today that is 28 files, most of them done.
- **`QUEUE.md`** (`tools/workflow/queue/summary.go:21-35`) ends with
  **Recently done** (`summary.go:211-230`): every item done within 7 days,
  as name, PR number and date, 21 lines in today's file. It carries no
  budget figures.

The gates skip done items, but only by accident of their callers.
`State.Next` (`queue/next.go:66-89`) looks at `in_progress` features, then
`nextQueued` (`next.go:91-112`) falls back to `queued` ones; both reach
`cloudHold`'s runaway check (`next.go:158-162`) through `ready`. `NextFor`
(`next.go:216-241`, `wf next -feature F`) filters on no status: a done
feature is safe there only because all its steps are done, so `ready`
returns before `cloudHold`. `needsHuman`, which feeds QUEUE.md's **Needs
Jeff**, skips `done` (`summary.go:106-109`) and lists a runaway for every
other item, queued ones included (`summary.go:125-129`). `Next` reports
blocked reasons for in-progress items only; `nextQueued` reports none.
`Runaway` itself does not look at status.

## Prior art

- **FLOW-13** (`docs/development/process.md:351-370`): "Each feature also has a
  work budget, which is a runaway check rather than a share of the run:
  exhausting it escalates to a `human` step." The digest reports "what was
  spent". This design keeps the check and narrows the report.
- **FLOW-04** (`process.md:152`) lists a runaway budget among the escalations
  a human takes. That stays.
- **FLOW-01** and **FLOW-12**: a design starts from overlapping archived
  designs, which are kept for reuse. That reading of done items' designs is
  research, not a retrospective, and is unchanged.
- **FLOW-09** and **TEST-03**: open native gaps in
  `backlog/native-gaps.yaml` ("validation deferred by merged designs", 4
  open) come from done items and stay open work. Unchanged.
- **workflow-bootstrap** (`docs/design/workflow-bootstrap.md`) introduced the
  budget and the runaway factor. The factor is in steps, which the code and
  the `config.yaml` comment ("at this multiple of its step estimate") both
  say.
- **queue-summary** (`docs/design/queue-summary.md`, Design item 6) added
  QUEUE.md's **Recently done** on purpose, so Jeff could see what finished
  in the week. Decision 3 reverses that; a2 asks Jeff to confirm.
- **workflow-streamline** (`docs/design/workflow-streamline.md`, a4):
  escalations are the only other human steps, and a runaway is one of them.
  That is unchanged.
- No rule or design defines a retrospective. A search of
  `docs/development/`, `.claude/`, `tools/workflow/` and the `workflow`
  branch's `README.md` finds the word nowhere.
- **Ordering.** Two queued process items ahead of this one touch the same
  files: flow19-design-before-plan (priority 26) changes FLOW-19/FLOW-03 and
  the `workflow-human` review flow, and wf-later-phase-host-steps (priority
  27) changes `queue/next.go` and its tests. This item (priority 28) rebases
  onto both. The `workflow-human` step numbers cited below, and when the
  implementation plan is written, may shift under flow19-design-before-plan.

## Decisions

1. **A runaway is a gate on an open item, in steps only.** An *open* item is
   one whose queue status is `queued` or `in_progress` (QUEUE.md's
   **Active** section is narrower: in-progress only). `State.Runaway`
   returns false for a `done` item. No current caller changes behavior,
   but `NextFor` and any later caller stop relying on "no pending step" for
   it. The threshold stays `runaway_factor` times the step estimate. Runs
   are counted but never a runaway, and the digest does not call a run
   overrun one. The factor, the `wf next` skip and the escalation to Jeff
   all stay as they are.
   - **Rejected: also flag runs past the factor.** A run is a calendar day
     with any claim, so waiting on a review, a host or the PR slot adds runs
     without any looping. Every runs-based flag in the 2026-11-04 digest was
     that kind of wait.
   - **Rejected: drop the per-feature budget.** Jeff wants the gate kept.

2. **Digests leave done items' budgets out.** In the `workflow-tick` Digest
   section:
   - **Blocked on Jeff** lists a runaway budget only as QUEUE.md's **Needs
     Jeff** does: one line per open item for which `Runaway` is true. That
     is the one source; the writer does not judge budgets itself.
   - **Spent** lists this run's steps against `run_budget.steps`, and each
     open item the run touched with its steps and runs spent against its
     estimate. Nothing in it is flagged as over budget. An item this run
     closed (`f8-close`, `x4-close`) is reported in **Moved**, as today,
     and not in **Spent**.
   - **Process items** lists only queued and in-progress ones, as its
     opening line already says. A process item closed this run goes in
     **Moved**.

   **Waiting on a host** still lists open native gaps, though they come from
   merged designs (FLOW-09). The `workflow-native` digest already reports
   only the native budget and needs no change.
   - **Rejected: list done items once more on the run after they close.**
     The close step's **Moved** line already does that, on the run that
     closes the item.

3. **`wf status` and QUEUE.md show open items only.**
   - `wf status` prints queued and in-progress items by default. A new
     `-all` flag adds done items with their spent against estimate, for a
     retrospective.
   - QUEUE.md drops **Recently done**. `Summary` keeps Needs Jeff, Active,
     Waiting on a host and Backlog, none of which lists a done item.
   - `wf status -md` prints QUEUE.md as before, so it changes with it. `-all`
     does not apply to `-md`.
   - The readers of `wf status` need none of the done items and need no
     change: `workflow-human` (`SKILL.md:27`) and `workflow-intake`
     (`SKILL.md:23`) use it to set priority, `workflow-tick`
     (`SKILL.md:134`) for process-item holds, and the 07:40 cloud summary
     task (prompt in `docs/design/native-loop-followups.md:180-186`) for
     the morning lines. That prompt stays as it is.
   - **Rejected: keep Recently done as it is.** It holds no budget figures,
     so it does not cause the false flags. But it lists done items in a page
     Jeff reads daily, which his rule sets aside, and it repeats what each
     close step's **Moved** line already reported.
   - **Rejected: a separate `wf retro` command.** One flag on the command
     that already prints the queue is enough.

4. **An explicit retrospective is one Jeff asks for.** A retrospective
   happens when Jeff asks for one in an attended session, by name or by
   asking how done items went. The agent then runs `dist/wf status -all`
   and reads the done items' queue history, digests and archived designs.
   No unattended run holds one. Citing past digests or archived designs as a
   design's evidence, as Problem does here, is FLOW-01 research and not a
   retrospective. FLOW-13 gains one sentence (see Design); `workflow-human`
   gains a short item.
   - **Rejected: a scheduled retrospective.** Jeff did not ask for one, and
     it would bring done items back into the unattended runs.

5. **The rule changes through this process item** (FLOW-14's first route).
   FLOW-14 says a process item's branch is exempt from the rule-change
   clearance and that `rule_changes` is a feature's declaration, so
   `rule_changes` is `[]`. The FLOW-13 wording is in Design below so the
   review approves it with the design.

## Questions

### Blocking

None.

### Assumed

- **a1. What counts as a runaway** (Decision 1). Proceeding with steps only,
  for open items only, and `Runaway` false for a done item. Options:
  - (a) as stated;
  - (b) also flag runs past `runaway_factor` times the runs estimate, for
    open items;
  - (c) as stated, but leave `Runaway` alone and rely on its callers.

  Recommended: (a).
- **a2. QUEUE.md's Recently done** (Decision 3). It has no budget figures;
  the question is whether a list of done items belongs in QUEUE.md at all.
  Removing it reverses queue-summary's Design item 6. Proceeding with
  removing it. Options:
  - (a) remove it, under the set-aside rule; the week's finishes stay in
    the digests' **Moved** and in `wf status -all`;
  - (b) keep it as today (names, PR numbers, dates), as queue-summary
    intended;
  - (c) keep it but shorten the window from 7 days to 1.

  Recommended: (a).
- **a3. The digest's Spent line for an open item** (Decision 2).
  Proceeding with steps and runs, never flagged. Options:
  - (a) steps only, for example "build-link-subnet: 15/120 steps";
  - (b) steps and runs, as today, with nothing flagged, for example
    "build-link-subnet: 15/120 steps, 2/6 runs";
  - (c) steps against the runaway threshold, for example "15 of 240
    (runaway at 2x)".

  Recommended: (b), the smallest change.
- **a4. Retrospective** (Decision 4). Proceeding with "Jeff asks in an
  attended session" and `wf status -all`. Options:
  - (a) as stated;
  - (b) as stated, plus a standing retrospective section in the first digest
    of each week;
  - (c) no flag; a retrospective reads the queue files directly.

  Recommended: (a).

## Design

**FLOW-13** (`docs/development/process.md:363-368`), the sentences from
"Each feature also has a work budget" to the end of the paragraph, after the
change:

> Each feature also has a work budget, which is a runaway check rather than
> a share of the run: an open (queued or in-progress) item that spends
> `runaway_factor` times its estimated steps escalates to a `human` step, and
> the unattended loop takes no more of its steps. Runs are counted but never
> trip it. The loop posts a periodic digest (what moved, what is blocked on a
> human, what is waiting on a native host, what open items spent), and
> notifies when a new blocking question appears. Done items are left out of
> the runaway check, of the digest's blocked and spent lines, of `wf status`
> and of `QUEUE.md`; their budgets are looked at again only in a
> retrospective the human asks for. Reading archived designs (FLOW-01,
> FLOW-12) and open native gaps from merged designs (FLOW-09) are not
> affected.

Severity (SHOULD) and check (automated) are unchanged.

**`tools/workflow`:**

- `queue/queue.go:411`, `Runaway`: return false when `f.Status == "done"`.
  Update its doc comment to "open item". Reword the `Budget` comment
  (`queue.go:114`): runs and steps are counted; only steps trip the runaway
  check, and only for open items.
- `cmd/wf/main.go:160-181`, `status`: skip done features unless `-all`. Add
  the flag next to `-md` (`main.go:114`) and to the usage comment
  (`main.go:7`). The filtering lives in a small `queue` helper,
  `(*State).Listed(all bool) []*Feature`, so it can be tested; `cmd/wf` has
  no tests.
- `queue/summary.go`: remove the `Recently done` section (line 33),
  `recentlyDone` (211-230) and `recentDays` (15). Update the doc comment on
  `Summary` (17-20).

**Skills:**

- `workflow-tick` Digest (`SKILL.md:132-147`): the bullets of Decision 2,
  citing FLOW-13.
- `workflow-human` step 4 (`SKILL.md:69-72`): "a runaway budget" becomes "a
  runaway budget on an open item (FLOW-13)". A new last item: a
  retrospective only when Jeff asks for one, using `dist/wf status -all`
  (Decision 4).
- `workflow-intake` (`SKILL.md:26`): "The runaway check escalates at twice
  the step estimate" becomes "at `runaway_factor` (2) times the step
  estimate, while the item is open (FLOW-13)".
- `workflow-native`: no change (Decision 2).

**`workflow` branch `README.md`** (not on `main`): line 19's QUEUE.md row
drops "and what finished in the last 7 days"; line 69's budget comment says
open items only; lines 151-153 list `wf status [-all]`.

## Security note

`security_boundary: none`. The change is to reporting in contributor tooling
and to agent instructions. No build-environment boundary, relay, ledger or
collector code changes. The runaway gate on open items is unchanged, so the
unattended loop is bounded exactly as before.

## Validation plan

- **Tier 0, `tools/workflow/queue`:**
  - `TestLeasesAndRunaway` (`queue_test.go:99`): a done feature with steps
    spent past the factor is not `Runaway`. An in-progress one still is, and
    `wf next` still skips it.
  - `summaryState` (`summary_test.go:13-54`): add a done item spent past the
    factor, and update its doc comment (13-15), which names "done items
    inside and outside the last 7 days". The golden `testdata/queue.md`
    loses **Recently done** and must not mention that item or the existing
    `recent` and `old` items.
  - `Listed`: `false` returns queued and in-progress features in FLOW-05
    order; `true` adds the done ones.
- **Check after merge:** the first tick on the new skill writes a digest
  with no done item under **Blocked on Jeff** or **Spent**. The QUEUE.md it
  regenerates has no **Recently done**.
- No native validation.

## Implementation plan

Written now and reviewed with the design (FLOW-19). Each task is one commit
on `feat/budgets-active-only`, rebased onto flow19-design-before-plan and
wf-later-phase-host-steps (Prior art, Ordering).

1. `docs/development/process.md` FLOW-13 as in Design (FLOW-13, FLOW-14).
2. `tools/workflow/queue/queue.go`: `Runaway`, the `Budget` comment and a
   new `Listed` helper, with tests in `queue_test.go` (FLOW-13, TEST-01,
   GLOBAL-01).
3. `tools/workflow/queue/summary.go`: remove **Recently done**. Update
   `summary_test.go`, including the `summaryState` comment, and regenerate
   `testdata/queue.md` with `-update` (FLOW-13, TEST-01).
4. `tools/workflow/cmd/wf/main.go`: `status` uses `Listed`, and add the
   `-all` flag and its usage line (FLOW-13, UX-01 for the usage text).
5. Skills: `workflow-tick` Digest, `workflow-human` step 4 and the
   retrospective item, `workflow-intake` line 26 (FLOW-14: cite FLOW-13, do
   not restate it).
6. `make test` and `make lint` (FLOW-17). Archive this design with its
   summary and open the PR (FLOW-12, FLOW-08).
7. After merge, at x4-close: the `workflow` branch `README.md` lines 19, 69
   and 151-153, and a `wf validate` so QUEUE.md regenerates from the new
   `dist/wf`.

`overview.md` is unchanged: no component flow changes (FLOW-15).

## Design review

- **r1, 2026-11-06: approved at `dada2bd`** (Jeff, x1-design, FLOW-19). He
  reviewed this design alongside flow19-design-before-plan,
  wf-later-phase-host-steps and budgets-active-only, all on one review page
  (published 2026-11-05 with every assumption first). He agreed with every
  recommendation: a1, a2 and a4 are confirmed as drafted, option (a), and
  a3 as option (b): steps and runs, with nothing flagged.

## Consolidation

`origin/main` at `9787fb4` merged at `4cc23c8` before the edits.

- **Landed since the fork:** #97 (deadcode with the module's toolchain) and
  #98 (build-link-subnet). Neither touches `docs/development/process.md`,
  `.claude/` or `tools/workflow/`, so nothing here needed reconciling.
- **Not yet landed, same files:** flow19-design-before-plan (priority 26)
  and wf-later-phase-host-steps (27) are on their own branches. The plan
  said this item rebases onto both; it was implemented first instead, with
  its edits held to the lines Design names, so whichever merges second
  resolves a small conflict: FLOW-13's paragraph, `workflow-tick` Digest,
  `workflow-human` steps 4 and 6, and `queue/queue.go` near `Runaway`.
- **Terminology:** "open item" (queued or in progress) is used the same way
  in FLOW-13, the `Runaway`, `Budget` and `Listed` comments, and the three
  skills.
- **Shared abstractions:** `Listed` reuses the FLOW-05 comparator, which
  `sortFeatures` now takes as a slice so both sort the same way.
- **FLOW-15 sweep:** `recentlyDone` and `recentDays` are removed with their
  only caller; no other dead code or duplication. `overview.md` is
  unchanged: no component flow changes.
- **Left for x4-close (Implementation plan task 7):** the `workflow`
  branch `README.md` lines and the `wf validate` that regenerates
  `QUEUE.md`.

## Rollback notes

## Archive summary
