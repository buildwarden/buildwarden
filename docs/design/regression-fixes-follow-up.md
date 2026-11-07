---
feature: regression-fixes-follow-up
status: archived
drivers: []
seams: []
surfaces: []
security_boundary: none
alters_relationships: false
design_review: required
native_validation: []
tier2_gaps: []
rules: [FLOW-03, FLOW-04, FLOW-05, FLOW-06, FLOW-08, FLOW-09, FLOW-10, FLOW-12,
  FLOW-13, FLOW-14, FLOW-15, FLOW-17, FLOW-19, GLOBAL-01, TEST-01, TEST-02]
rule_changes: []
concepts: [workflow, queue, priority, queue-order, follow-up, regression, intake,
  wf-next, process-item]
related: [native-loop-followups.md, queue-summary.md, workflow-streamline.md,
  budgets-active-only.md, flow19-design-before-plan.md]
---

# Queue order by class, and regression fixes queued as immediate follow-ups

## Problem

On 2026-11-06 Jeff gave two rules for the queue. Verbatim:

1. "as a general process change, fixes for known regressions when there are
   larger updates should always be queued as immediate follow-ups to the
   existing work item unless explicitly said otherwise."
2. "process updates are also prioritized over code since they're usually
   short like the ones we're doing now. So in order of priority 1. Items
   already in implementation 2. Workflow/process updates 3. Fixes for
   implemented items (often may can be a result of needing the process
   changes) 4. New functionality"

Both were applied by hand. The `workflow` branch shows how:

- **The regressions were found and not filed.** build-link-subnet's native
  runs found two regressions, both already on `main`. The vz image-prepare
  finding is in `native/macos-vz/2026-11-05/` and in PR #98's improvised
  "For Jeff" list, item 4. The busybox finding is a "Side finding (not
  blocking)" under the macbook run's **Notes** (`digest/2026-11-05.md:170`,
  `:180-187`). workflow-native already asked for more: an unattended session
  records "the proposed item in the digest for Jeff"
  (`.claude/skills/workflow-native/SKILL.md:88-92`) under **Blocked on Jeff**,
  "a proposed fix item" (`:110-115`). The session judged the finding a side
  note, so it landed in Notes. Neither finding had a queue item until Jeff's
  session the next day.
- **Placing them took three hand renumberings.**
  - `6dd3c88` moved security-suite from 19 to 29 "behind the queued fixes".
  - `e42a6ef` queued vz-image-prepare-refresh at 24 and build-vm-busybox at
    25 with `wf new`. `State.Add` (`tools/workflow/queue/queue.go:217-239`)
    moves every item at or below the new number down one, and logs only on
    the new item. Six items moved. flow19-design-before-plan (26 -> 28),
    wf-later-phase-host-steps (27 -> 29) and budgets-active-only (28 -> 30),
    the three items then in implementation, got no history line at all.
    hyperv-relay-per-build-channel (24 -> 33), baked-asset-staleness
    (25 -> 34) and security-suite (29 -> 37) moved further than `wf new`'s one
    per insert, so they were partly set by hand, and their "(Jeff; renumbered
    by wf new)" lines were written by hand.
  - `f2c1cdc` renumbered every open item to 38-47 by class. Each history line
    names the class: "this item: 1. in implementation", "2. process",
    "3. fix" or "4. new functionality".
- **The numbers do not give the order Jeff set.** `State.Next`
  (`queue/next.go:85-107`) takes every in-progress item before any queued one:
  `nextQueued` (`:112-131`) runs only once every in-progress item is
  blocked. security-suite is in progress (phase 1, class 4, priority 47), and
  the four fixes are queued (43-46). Today the fixes can start only because
  security-suite waits on blocking question q1. `6dd3c88` says so: "q1 records
  the scoping ... so wf next skips it and the fixes can start". Once q1 is
  answered, security-suite's design goes ahead of every fix, whatever its
  number.
- **The process lane is in the wrong place.** `sortFeatures`
  (`queue.go:186-198`) puts every process item ahead of every feature, and
  `Next` walks in-progress items first. So an in-progress process item, even
  one in phase 1, goes ahead of an in-progress feature in Implement, which
  Jeff's class 1 puts first. And a queued process item waits behind every
  in-progress feature with a ready step, which Jeff's class 2 puts behind.
- **Nothing records which items are fixes.** Class 3 in `f2c1cdc` was a
  judgment made from each item's `source` text. Four items were given it:
  the two regressions, and hyperv-relay-per-build-channel and
  baked-asset-staleness, which build-link-subnet's Decisions 9 and 10 deferred
  to follow-ups.

## Prior art

- **FLOW-05** (`docs/development/process.md:171-205`): "Process items
  (FLOW-03) come before every feature, whatever their priority" (`:176-177`).
  After that, depth first on "the highest-priority feature in progress"
  (`:181-185`), and "A new feature starts Design only when every feature in
  progress is blocked" (`:190-191`). This item replaces the first rule with
  the four classes. It keeps depth first inside a class, and holds, the
  in-progress limit, the alters slot and the PR slot unchanged. Its check
  stays `automated` (`:174`): everything this item adds to FLOW-05 is
  mechanical.
- **FLOW-03** (`:98-138`): the phase tables this item uses to define "in
  implementation". It also sets `kind: process` (`:118-130`), from which
  class 2 is derived.
- **FLOW-04**: a step is offered once its item reaches the step's phase.
  Unchanged; the class order only chooses between items.
- **FLOW-06, FLOW-09** (`:293-295`): "A failure found there returns the
  feature (or a new fix item) to Design or Implement with the logs attached."
  FLOW-09's check is `automated + agent-judgment` (`:272`). This item says
  which failures are a fix item, and puts that judgment here.
- **FLOW-12**: the item's `source` records where it came from. It stays the
  human-readable evidence, and the new field is the link wf can read.
- **FLOW-13** (`:368-369`): "A run spends its budget in FLOW-05 order, so the
  top-priority feature can use all of it." Reworded for classes.
- **FLOW-14** (`:385-427`): this is a process item. It changes
  `docs/development/`, `.claude/` and `tools/workflow/` on its own branch after
  its review, so `rule_changes: []` and the wording is in **Design**. Nothing
  it changes reaches the running loop until Jeff merges it (Decision 9).
- **native-loop-followups** (`docs/design/native-loop-followups.md`,
  section 6): added `kind` and the process lane: "Process items sort ahead of
  every feature, whatever their `priority`. `priority` stays unique across the
  queue." The lane stays, below class 1. Uniqueness stays.
- **queue-summary** (`docs/design/queue-summary.md`): QUEUE.md's Kind column,
  which now shows the fix link.
- **budgets-active-only** (#100): done items are left out of `wf status` and
  QUEUE.md. A done parent is what releases its follow-ups (Decision 7).
- **flow19-design-before-plan** (#101): Plan follows the design review, so
  for a feature, Plan does not mean "in implementation" (Decision 2).

## Decisions

1. **The class order is mechanical in `wf next`, derived, and computed when
   read.** A function `Class()` derives each open item's class from fields
   the queue already has, plus one new field (Decision 3):
   - 1: in implementation (Decision 2);
   - 2: `kind: process`;
   - 3: a feature with `follows` set;
   - 4: every other feature.

   The order key is class, then status (`in_progress`, then `queued`), then
   lower `priority`. Done items have no class and sort after every open item,
   by priority, for `wf status -all`. Class depends on status and phase,
   which change inside a command (a claim, a phase completing), so the order
   is never stored: one method `Ordered()` sorts a copy at each call, and
   `Next`, `Listed` and the QUEUE.md writers (`writeActive`, `writeBacklog`,
   `needsHuman`, `hostSteps`) all walk `Ordered()`. `sortFeatures` at Load
   and Add stays, for a stable starting order.

   `Next` walks in-progress and queued items together in that order. It
   counts in-progress items before the walk, and for a queued item keeps
   `nextQueued`'s checks: `Hold(nil, ...)`, the cloud in-progress limit, and
   `Starts` for a cloud claim or a process item. A queued item starts only
   when every item ahead of it is blocked. `priority` orders items within a
   class only.
   - **Rejected: priority numbers that intake sets by the rule, with no code
     change.** Numbers cannot place a queued fix ahead of an in-progress
     class-4 feature, because `Next` takes in-progress items first
     (Problem). Every reclassification, such as a feature reaching Implement
     or a process item arriving, would need another renumbering like
     `f2c1cdc`.
   - **Rejected: a stored `class` field.** Class 1 changes with the phase and
     class 2 is `kind`, so a stored copy would go stale. Only class 3 needs
     new data.
   - **Rejected: re-sorting after every status or phase change.** Every
     mutator (`Claim`, `Complete`, `Back`, `Release`) would have to remember
     it; sorting at read costs a few dozen comparisons.
   - **Rejected: priority alone within a class, without in-progress first.**
     A newly queued item with a lower number would preempt a started one of
     the same class. That would make work parallel, which FLOW-05 avoids.
     Intake's rule becomes the same thing in class terms: a new item never
     jumps an in-progress item of its own class without Jeff's say (task 7).
2. **"In implementation" means in progress, past Plan, and not waiting for
   the human's merge.** That is an in-progress feature in phase 3
   (Implement) to 6 (Publish) or 8 (Close), or an in-progress process item in
   phase 2 (Implement) or 4 (Close). A feature in phase 7 or a process item in
   phase 3 waits on Jeff's merge, and its pending `f7-merge` or `x3-merge` is
   a cloud step `ready()` returns (`phases.go:61-62`, `:72`;
   `TestPRSlotIsSerial`). It takes its own class: 2 for a process item, 3 or 4
   for a feature. So a merge check does not go ahead of class-2 and class-3
   work. workflow-tick confirms merges before `wf next` anyway
   (`workflow-tick/SKILL.md:61-65`). This matches `f2c1cdc`: the three process
   items at `x2-implement` were "1. in implementation", and security-suite
   (in progress, phase 1) was "4. new functionality".
   - **Rejected: any `in_progress` item.** That is today's rule, and it puts
     security-suite ahead of the fixes, the case Jeff ruled against in
     `6dd3c88`.
   - **Rejected: from phase 2 (Plan).** Under #101 Plan follows the design
     review, but no code exists yet, so pausing at Plan costs nothing to
     resume. Implement is where depth first saves rework.
   - **Rejected: keep the merge phases in class 1, or make `ready` skip an
     unmerged `*-merge` step.** The first puts merge checks ahead of every
     process item's step. The second changes `ready` and `TestPRSlotIsSerial`
     for a case the class already handles.
3. **A fix names the item it follows: `follows: <item>`.** This is one
   optional field on the queue file. It holds the name of another item in
   the queue, done or open. On a feature it puts the item in class 3 and
   gates its start (Decision 7). On a process item it is recorded but changes
   nothing: the item is class 2 either way. `wf validate` fails when
   `follows` names a missing item or the item itself. `wf new` takes
   `-follows <item>`. QUEUE.md's Kind column shows `fix (follows <item>)`,
   and `wf status` adds a line `follows: <item>`. A regression fix and a
   deferred follow-up (hyperv-relay-per-build-channel) look the same; nothing
   reads the difference (Decision 6).
   - **Rejected: `class: fix|feature`.** It records that the item is a fix
     but not what it fixes. The parent is what a designer and Jeff need, and
     what the start gate reads.
   - **Rejected: links in `source` only.** That is today's state, and wf
     cannot read it.
4. **Holds override the class order.** An in-progress process item's
   `holds` (FLOW-05, `next.go:34-46`) stop every matching worker's new steps,
   class 1 included, as today. A hold is the process item's own declared
   disruption, settled in its design review; a process item that disrupts
   nothing holds `[]`.
   - **Rejected: class 1 ignores holds.** A hold exists because running work
     would break under the change in progress, and work in implementation is
     the work most exposed to it.
5. **Only a regression already on `main` is a follow-up. A failure the item
   caused goes back to the item.** A *regression* here is a failure of
   behavior the item did not set out to change that `main` already has,
   found while the item is built, validated or consolidated (vz: "Also on
   main before #98"; busybox: "main and branch"). It is filed as the item's
   follow-up, and the item does not wait for it or absorb it. A failure the
   item's own branch causes, whether of its own claims or of behavior `main`
   has working, returns the item to Design or Implement (FLOW-06, FLOW-09),
   as today: merging it would land a known regression on `main`, and its fix
   is code on the item's branch. Jeff can say otherwise for either.
   - **Rejected: every regression, whatever its cause, is a follow-up and the
     parent merges.** It lands known regressions on `main` on purpose, and
     the follow-up cannot be built until the parent merges. Jeff's rule
     speaks of "known regressions" next to "larger updates"; both of his
     cases were already on `main`.
6. **The session that finds a regression files its follow-up at once.**
   - **Who files it:** the attended or unattended session that found it:
     workflow-native, wf-implement (a `make test` failure outside the change
     that `main` also has), wf-review (a tier-1 or tier-2 finding on `main`'s
     behavior) or wf-publish (consolidation). It uses workflow-intake's
     follow-up path: `wf new -follows <item> -priority <n>`, a small estimate
     (7 to 9 steps), the evidence in `-source`, and the logs under `native/`
     as today. An unattended session files it without Jeff. This replaces
     "record the proposed item in the digest"
     (`workflow-native/SKILL.md:88-92`).
   - **Where it sorts:** the top of class 3, by priority alone. `-priority`
     is the lowest number held by an open class-3 item; with class 3 empty,
     the lowest held by an open class-4 item; with neither, one past the
     highest open number. `wf new` renumbers the rest down
     (`queue.go:224-229`). Several found together are queued in the order
     found, each at the number after the last. That reproduces `e42a6ef`: vz
     at 24 and busybox at 25, ahead of hyperv and baked. No regression-versus-
     deferred distinction is kept: a newer regression goes ahead of older
     fixes, and Jeff reorders within the class if he wants otherwise.
   - **Where it is reported:** the step's evidence; the PR body's new
     **Follow-ups filed** section (wf-publish, `f6-open-pr`); and a
     **Follow-ups filed** line in the digest (workflow-tick and
     workflow-native), each as "<slug> (follows <item>)". It is not listed
     under **Blocked on Jeff**: it does not wait on him.
   - **"Unless explicitly said otherwise":** Jeff's word, recorded in the
     queue history. He can fold the fix into the larger item (`wf back` with
     the finding), place it elsewhere (clear `follows` to make it class 4, or
     set its priority), or leave it unfiled. The session follows what he says
     and does not apply the default.
   - **Rejected: the digest proposes and Jeff files.** That is what happened
     to busybox: a Notes line on 2026-11-05, filed a day later.
7. **A follow-up starts only once its parent is done.** `Next` does not offer
   a queued feature whose `follows` names an open item, to any worker, and
   reports it as blocked: "follows <parent>, not yet done (FLOW-05)". Once the
   parent closes (phase 8), the follow-up is class 3 and starts ahead of new
   functionality. This is what makes it the parent's *immediate follow-up*:
   it is never built on a `main` that lacks the parent, and never merges
   first. A follow-up of an unimplemented parent (security-suite in phase 1,
   say) waits behind it instead of jumping it. The cost is small: the parent
   is in class 1 or 2 by the time a regression is found, so it is worked
   depth first. `NextFor` (an attended run on one named item) skips the gate:
   naming the item is Jeff's explicit word.
   - **Rejected: no wait, relying on the PR slot.** The PR slot
     (`prSlotHolder`, `next.go:196-208`) is held only by an in-progress
     feature from its `f6-merge-main` through Close, and never by a process
     item. For a parent in phases 3 to 5 nothing stops the follow-up merging
     first, and a deferred follow-up such as hyperv-relay-per-build-channel
     needs the parent's code.
   - **Rejected: hold only the follow-up's `f3-implement`.** One more
     `cloudHold` case, and the follow-up's design would be written against a
     branch that may still change in review.
8. **`priority` stays unique across the queue. `wf new` logs each
   renumbering.** `Validate`'s uniqueness check (`queue.go:294-304`) is
   unchanged. `Add` adds a history line to every item it moves: "priority
   <a> -> <b> (renumbered by wf new: <slug> queued at <n>)". The hand-written
   lines in `e42a6ef`, and the three it left out, become mechanical.
   - **Rejected: priority unique per class, or among open items only.** That
     changes `Validate`, `Add` and every reader for no ordering gain. Numbers
     are compared only within a class anyway.
9. **Migration: set `follows` on the four fixes at x4-close, and renumber
   nothing.** On the `workflow` branch, `follows: build-link-subnet` goes on
   vz-image-prepare-refresh, build-vm-busybox, hyperv-relay-per-build-channel
   and baked-asset-staleness, each with a history line. build-link-subnet is
   done (phase 8), so the gate does not hold them. `f2c1cdc` already put every
   open item in class order: process items at 41-42, fixes at 43-46,
   security-suite at 47. security-suite is not touched. It stays in progress
   in phase 1, class 4, with q1 open.
   - **Before the merge:** today's `Next` runs, and nothing this branch
     changes reaches the loop (FLOW-14). The fixes go first only while q1 is
     open. **For Jeff at this review:** leave security-suite's q1 unanswered
     until this item's x4-close, or start the fixes first. The workflow-human
     session holding this review records that on q1 as a note; no permanent
     skill text carries it.
   - **From the merge to x4-close:** the fixes are class 4 and queued, and
     security-suite is class 4 and in progress. q1 is still open, and
     x4-close follows x3-merge in the same tick, so the order holds.
   - **After x4-close:** a `wf` built before the merge drops `follows` when it
     saves a file (`readYAML` uses `yaml.Unmarshal`, non-strict;
     `queue.go:267-276`). workflow-tick builds `dist/wf` from its checkout
     (`workflow-tick/SKILL.md:43-44`) and workflow-native and workflow-human
     reuse that setup. x4-close's digest entry tells each machine and human
     session to rebuild `dist/wf` from the merged `main` before its next run,
     and the next retrospective (FLOW-13) checks that `follows` is still set
     on the four fixes.
   - **Rejected: compact renumbering of open items.** It is noise in seven
     histories, and the order does not change.

## Questions

### Blocking

None.

### Assumed

- **a1. Mechanical or by numbers** (Decision 1). Proceeding with a class
  derived in `wf next` at each read, depth first within a class. Options:
  - (a) as stated;
  - (b) priority numbers that intake and workflow-human set by the rule, with
    no code change;
  - (c) mechanical, with priority alone within a class (no in-progress
    first).

  Recommended: (a).
- **a2. What "in implementation" means** (Decision 2). Proceeding with in
  progress, Implement or later, and not waiting for the merge: feature phase
  3 to 6 or 8, process phase 2 or 4. Options:
  - (a) as stated;
  - (b) the same, with the merge phases (feature 7, process 3) included;
  - (c) from Plan, feature phase 2 or later;
  - (d) any in-progress item, which is today's order.

  Recommended: (a).
- **a3. The link to the parent** (Decision 3). Proceeding with one optional
  field `follows: <item>`. Options:
  - (a) as stated;
  - (b) `class: fix|feature`;
  - (c) no field: `source` text and priority numbers.

  Recommended: (a).
- **a4. Holds and class 1** (Decision 4). Proceeding with holds overriding
  the class order, as today. Options:
  - (a) as stated;
  - (b) class-1 items ignore holds.

  Recommended: (a).
- **a5. Which failures are follow-ups** (Decision 5). Proceeding with a
  regression `main` already has as a follow-up, and a failure the item's
  branch causes returned to the item (FLOW-09). Options:
  - (a) as stated;
  - (b) every regression, whatever its cause, is a follow-up and the parent
    merges;
  - (c) every failure returns to the item; nothing is a follow-up without
    Jeff.

  Recommended: (a).
- **a6. Who files a follow-up, and where it is reported** (Decision 6).
  Proceeding with the session that found it, unattended included, reported
  in the step's evidence, the PR's **Follow-ups filed** section and the
  digest's **Follow-ups filed** line. Options:
  - (a) as stated;
  - (b) unattended sessions propose it in the digest and Jeff files it, as
    today;
  - (c) only Jeff files, attended or not.

  Recommended: (a).
- **a7. Where a regression fix sorts** (Decision 6). Proceeding with the top
  of class 3 by priority alone, several in the order found. Options:
  - (a) as stated;
  - (b) after the parent's other open follow-ups;
  - (c) wherever intake and Jeff choose, with no default.

  Recommended: (a).
- **a8. Waiting for the parent** (Decision 7). Proceeding with a queued
  follow-up not offered until its parent is done, except in an attended run
  that names it. Options:
  - (a) as stated;
  - (b) no wait: the PR slot orders the merges;
  - (c) wait only at `f3-implement`.

  Recommended: (a).
- **a9. Migration** (Decision 9). Proceeding with `follows:
  build-link-subnet` on the four fixes at x4-close, no renumbering,
  security-suite untouched, and q1 left unanswered until x4-close. Options:
  - (a) as stated;
  - (b) as stated, plus compact renumbering of the open items;
  - (c) `follows` on the two regressions only, so hyperv-relay-per-build-channel
    and baked-asset-staleness become class 4;
  - (d) as stated, but q1 may be answered before x4-close, accepting that
    security-suite's design goes first.

  Recommended: (a).

## Design

`docs/development/process.md`:

- **Shape** (`:14-15`): "Work goes depth first on the first item in FLOW-05's
  order; others advance only while it is blocked (FLOW-05)."
- **FLOW-05** title (`:171`): "Depth first, in class order". The first
  paragraph (`:176-185`) becomes:

  > Work goes deep, not wide. Open items are taken in class order, and
  > within a class in-progress items come before queued ones, then the
  > higher priority:
  >
  > 1. **In implementation**: an in-progress feature in Implement (phase 3)
  >    through Close, or an in-progress process item in Implement (phase 2)
  >    or Close, except while it waits for the human's merge.
  > 2. **Process items** (FLOW-03), which are usually short and change the
  >    rules the rest of the queue runs under.
  > 3. **Fixes**: a feature that `follows` an item, fixing or finishing it.
  >    It starts only once that item is done.
  > 4. **New functionality**: every other feature.
  >
  > While a process item is in progress, a worker matching one of its
  > `holds` ... [unchanged; holds apply to every class] ... Otherwise a worker
  > takes the next ready step of the first item in that order, and keeps
  > taking that item's steps until it blocks: on a `human` or `host:<name>`
  > step, an exhausted budget, or the PR slot. Only then does it take steps
  > from the next item down, and it returns to the earlier item as soon as
  > that one has a ready step again.

- **FLOW-05**, second paragraph (`:190-191`): "A queued item starts only when
  every item ahead of it is blocked."
- **FLOW-09** (`:293-295`), replacing "A failure found there returns the
  feature (or a new fix item) to Design or Implement with the logs attached":

  > A failure the feature's branch causes returns the feature to Design or
  > Implement with the logs attached (FLOW-06). A regression `main` already
  > has, found while the feature is built, validated or consolidated, is
  > queued at once as the feature's immediate follow-up unless the human
  > explicitly says otherwise: the session that found it files a fix item
  > that `follows` the feature, at the top of the fixes, and the feature
  > neither waits for it nor absorbs it.

- **FLOW-13** (`:368-369`): "so the first item in FLOW-05's order can use all
  of it."

Queue (`tools/workflow/queue`):

```text
Class(f):  1  in_progress and (feature phase 3-6 or 8, or process phase 2 or 4)
           2  kind: process
           3  follows != ""
           4  otherwise
Ordered(): open items by class, then in_progress before queued, then
           priority; done items after, by priority
Next:      count in_progress; walk Ordered():
             in_progress: as today (hold, ready, blocked reason)
             queued: Hold(nil); cloud in-progress limit; follows an open
                     item -> blocked; ready -> Pick{Starts}
NextFor:   as today; no follows gate
```

## Cross-driver impact

None. No driver or product code changes.

## Security note

`security_boundary: none`. No build-environment boundary changes. One new
input: an unattended session can now queue a fix item. It goes through `wf
new` and `wf validate`, and through the same phases, design review and
human merge as any feature, and the digest lists it for Jeff.

## Validation plan

- **Tier 0, `tools/workflow/queue`:**
  - New `TestClass`: each class from kind, status, phase and `follows`. A
    process item is class 2 in phases 1 and 3 and class 1 in phases 2 and 4.
    A feature is class 3 or 4 in phases 2 and 7 and class 1 in phases 3 and
    8. A done item has no class.
  - New `TestNextTakesClassOrder`:
    - a queued fix starts ahead of an in-progress class-4 feature with a
      ready cloud step (the security-suite case), and at the in-progress
      limit it does not start;
    - an in-progress feature in Implement is taken ahead of an in-progress
      class-2 process item's `x3-merge` (the process item holds nothing);
    - an in-progress process item that holds `cloud` still blocks a class-1
      feature's cloud step;
    - a phase-7 feature's `f7-merge` comes after a queued fix's start;
    - a human worker gets a queued process item's `x1-design` ahead of an
      in-progress class-4 feature's human step.
  - New `TestFollowUpWaitsForParent`: a queued feature that `follows` an
    open item is not offered and is reported blocked; once the parent is
    done it starts; `NextFor` offers it either way.
  - New `TestOrderFollowsPhaseChanges`: a queued feature claimed, or a
    feature completing phase 2, moves in `Ordered()` without a reload.
  - `TestListed`: `Listed(true)` becomes `p a q d` (done items after the open
    ones); `Listed(false)` stays `p a q`.
  - `TestPRSlotIsSerial` passes unchanged: a (phase 7) is class 4 and b
    (phase 6) is class 1, so b is walked first in both orders, blocks on the
    PR slot, and a's `f7-merge` follows, with `blocked[0]` b's PR-slot
    reason.
  - `TestNextIsDepthFirst`, `TestNextStartsFeatureOnlyWhenAllBlocked` and
    `TestNextForOneFeature` pass unchanged: every item is class 4.
  - `process_test.go`: `TestProcessSortsFirstAndStartsOnAnyClaim` and the
    holds tests pass unchanged. New `TestImplementingFeatureSortsBeforeProcess`.
  - New `TestValidateFollows`: an unknown item and the item itself fail; a
    done parent passes.
  - New `TestAddLogsRenumbering`: each moved item gets the history line.
  - `TestLoadSaveRoundTrip` carries `follows`.
  - `summary_test.go`: `TestSummaryGolden` and `testdata/queue.md`, where a
    fix row shows `fix (follows <item>)`; and a new case where a phase change
    then `Save` writes the rows in the new class order.
- **`tools/workflow/cmd/wf`** has no `_test.go`. `new -follows` and the
  `status` line are covered through `Add` and checked by hand.
- **Check in use:** at x4-close, `dist/wf validate` and `dist/wf next -worker
  cloud/...` on the migrated queue name a fix ahead of security-suite, with
  q1 still open and with q1 answered in a scratch copy.
- **No native validation.**

## Implementation plan

Confirmed at x2-implement, after the review (process item, FLOW-03), as the
task list, one commit each. Tasks 1 to 5 and 7 to 11 are done; task 6's tests
landed with the code task they cover (2, 3 and 4), so each commit is green,
and a last commit split `Next` and `Validate` under the lint's complexity
limit without changing behavior. Task 14 is x4-close's.

1. `docs/development/process.md` (FLOW-05, FLOW-09, FLOW-13, FLOW-14): the
   Shape line (`:14-15`), FLOW-05's title (`:171`), its order paragraph
   (`:176-185`) and its "starts only when" sentence (`:190-191`), FLOW-09's
   failure sentence (`:293-295`), and FLOW-13's budget sentence
   (`:368-369`), as in Design.
2. `tools/workflow/queue/queue.go` (FLOW-05, GLOBAL-01; Decisions 1, 3, 8):
   - a `Follows` field (`yaml:"follows,omitempty"`) with its comment;
   - `Class()`, `inImplementation()` and `Ordered()`;
   - `sortFeatures` and `Listed` use the order key, with their comments;
   - `validate`: `follows` names another item in the queue (a State-level
     check beside the priority check);
   - `Add`: the renumbering history line.
3. `tools/workflow/queue/next.go` (FLOW-05; Decisions 1, 4, 7): `Next` counts
   in-progress items, walks `Ordered()`, folds `nextQueued`'s start checks
   into that walk, and adds the follows gate. Its doc comment (`:78-84`,
   "highest-priority in-progress feature") is rewritten for classes. The
   `Blocked` reasons are kept.
4. `tools/workflow/queue/summary.go` (Decisions 1, 3): `writeActive`,
   `writeBacklog`, `needsHuman` and `hostSteps` walk `Ordered()`; `kindLabel`
   shows `fix (follows <item>)`.
5. `tools/workflow/cmd/wf/main.go` (Decision 3): `new -follows`, the usage
   comment, and `status`'s `follows:` line.
6. Tier-0 tests (TEST-01, TEST-02), per the Validation plan, in
   `queue_test.go`, `process_test.go`, `summary_test.go` and
   `testdata/queue.md`.
7. `.claude/skills/workflow-intake/SKILL.md` (FLOW-05; Decisions 1, 3, 6):
   - step 3 (`:22-24`): "a new item never jumps the top-priority feature in
     progress without his say" becomes "the class order decides where an item
     can go, priority places it within its class, and a new item never jumps
     an in-progress item of its own class without his say";
   - a fix or follow-up of an item sets `-follows`;
   - a regression's default placement (the top of class 3), and that Jeff's
     explicit word overrides it;
   - the unattended follow-up path, with no Jeff, a small estimate and the
     evidence in `-source`.
8. `.claude/skills/workflow-native/SKILL.md` (FLOW-09; Decisions 5, 6):
   - Record (`:88-92`): a failure the branch causes goes back with `wf back`;
     a regression `main` already has is filed as a follow-up by the
     workflow-intake path, attended or not, and named in the step's evidence;
   - Close, unattended digest (`:110-115`): "a proposed fix item" leaves
     **Blocked on Jeff**, and a **Follow-ups filed** line lists each one as
     "<slug> (follows <item>)".
9. `.claude/agents/` (Decisions 5, 6):
   - `wf-publish.md` `f6-consolidate` (`:15-24`): a regression `main` already
     has, found in consolidation, is filed as a follow-up;
   - `wf-publish.md` `f6-open-pr` (`:25-32`): the PR body gains a
     **Follow-ups filed** section listing the item's follow-ups;
   - `wf-implement.md`: a `make test` failure outside the change that `main`
     also has is filed as a follow-up, not fixed in place;
   - `wf-review.md`: a tier-1 or tier-2 finding on behavior `main` already
     has is filed as a follow-up, not a MUST on this item.
10. `.claude/skills/workflow-tick/SKILL.md` Digest (`:136-150`; Decision 6): a
    **Follow-ups filed** line lists each follow-up filed this run, so Jeff can
    say otherwise.
11. `.claude/skills/workflow-human/SKILL.md` (FLOW-05; Decisions 1, 3):
    - step 1 (`:26-29`): show the class order; reorder within a class by
      priority, and across classes only by Jeff's word on `follows` or `kind`;
    - steps 2 and 3 (`:30`, `:34`): "highest-priority feature first" becomes
      "first in FLOW-05's order".
12. `docs/book/src/concepts/overview.md` (FLOW-15): checked. It describes
    components, not the workflow, so no pseudocode changes are expected.
13. Consolidation, then the gates (FLOW-10, FLOW-17): `make lint`, `make
    test`, `make enforcement`, `tools/workflow/flow18-check.sh`, and `wf
    check` for this item.
14. At x4-close, on the `workflow` branch (FLOW-03; Decision 9):
    - `README.md`: the `follows` line in the queue schema (`:44-52`), the
      process-item ordering sentence (`:108`), and the `wf next` paragraph
      (`:145-149`);
    - `follows: build-link-subnet` and a history line on the four fixes;
    - `dist/wf validate`, which regenerates QUEUE.md, and the "Check in use"
      above;
    - the digest entry asking every machine and human session to rebuild
      `dist/wf` from the merged `main`.

## Design review

- **r1, 2026-11-06: approved at `393f437`** (Jeff, workflow-human,
  x1-design, FLOW-19). He reviewed it alongside
  native-allowlist-scratch-tools on one review page, with the assumptions
  first. **a1 to a9 are confirmed as drafted, option (a).** Per a9, Jeff
  leaves security-suite's q1 unanswered until this item's x4-close.

## Consolidation

Filled in at x2-implement (FLOW-10, FLOW-15), against `origin/main` at
`ab148f9`, the fork point; nothing has landed since.

- **Landed before the fork:** #99 (later-phase host steps), #100 (budgets on
  open items only) and #101 (design review before Plan). This item builds on
  all three: `Offered` and `ready` from #99 are unchanged, `Listed` keeps
  #100's open-only default and puts done items last under `-all`, and
  Decision 2's "in implementation" starts at phase 3 because #101 moved Plan
  after the review.
- **Not yet landed, same files:** native-allowlist-scratch-tools, on its own
  branch, edits `.claude/skills/workflow-native/SKILL.md` in other hunks
  (its setup section and Work). `git merge-tree` of the two heads merges
  clean. Whichever merges second takes current `origin/main` in a merge
  commit before Jeff merges it.
- **Terminology:** "class" and "in implementation" are FLOW-05's words, used
  the same in `process.md`, `Class()`, the skills and the agents. A fix is a
  feature with `follows` set; "follow-up" names the relation, in FLOW-09,
  workflow-intake's section and every **Follow-ups filed** line.
- **Shared code:** `Next` and `NextFor` share `atLimit`; `Listed` and the four
  QUEUE.md writers share `Ordered`, so the order is computed in one place.
  `nextQueued` is gone, folded into `Next`'s walk.
- **Sweep:** `make lint` is green, `unused` included for
  `tools/workflow`; `make deadcode` scans the main module, which this item
  does not touch, and reports only its existing driver and relay entries.
  Duplication: none found by reading; the one tier-1 hint, GLOBAL-04 on `cmd/wf`, is expected: that
  command has no tests, and `new -follows` and the `status` line were checked
  by hand on a scratch copy of the live queue. `overview.md` describes
  components, not the workflow, so it has no pseudocode to change (FLOW-15).
- **Check in use, before migration:** `dist/wf` built from this branch loads
  the live queue unchanged. `wf status` lists native-allowlist-scratch-tools
  and this item (class 1, process phase 2) first, then security-suite (class
  4, in progress, q1 open), then the four fixes (class 4, queued, no
  `follows` yet). `wf next -worker cloud/...` starts vz-image-prepare-refresh,
  as today, because q1 blocks security-suite. On a scratch copy with Decision
  9's `follows: build-link-subnet` on the four fixes, `wf validate` passes,
  QUEUE.md shows `fix (follows build-link-subnet)`, `wf status` lists the
  fixes ahead of security-suite, and `wf next` still starts
  vz-image-prepare-refresh with q1 answered.
- **Rules proposed:** none beyond this design's own FLOW-05, FLOW-09 and
  FLOW-13 text.

## Rollback notes

None yet.

## Archive summary

Implemented 2026-11-06 by the cloud loop (attended, x2-implement), approved
at design review r1 (`393f437`, Jeff; a1 to a9 as drafted, option (a)).

- **Rule text** (`docs/development/process.md`): FLOW-05 is "Depth first, in
  class order": open items go by class (1 in implementation, 2 process items,
  3 fixes that `follows` an item, 4 new functionality), then in-progress
  before queued, then priority; holds still apply to every class. FLOW-09
  splits failures: one the branch causes returns the item (FLOW-06); a
  regression `main` already has is filed at once as the item's immediate
  follow-up unless the human says otherwise. The Shape line and FLOW-13's
  budget sentence say "the first item in FLOW-05's order". Severities and
  checks unchanged.
- **Built: class order in `wf`** (`tools/workflow/queue`). `Class()` derives
  the class from status, kind, phase and the new optional `follows` field;
  "in implementation" is an in-progress feature in phase 3 to 6 or 8, or an
  in-progress process item in phase 2 or 4, so an item waiting on the
  human's merge drops to its own class. `Ordered()` sorts a copy at each read
  (done items last, by priority); `Next`, `Listed` and QUEUE.md's
  `writeActive`, `writeBacklog`, `needsHuman` and `hostSteps` walk it.
  `Next` walks in-progress and queued items together, with `nextQueued`'s
  start checks folded in, and does not start a queued feature whose `follows`
  names an open item ("follows <parent>, not yet done (FLOW-05)"). `NextFor`
  skips that gate. `Next` and `NextFor` share `atLimit`.
- **Built: `follows` tooling.** `wf validate` fails when `follows` names a
  missing item or the item itself; `wf new -follows <item>`; QUEUE.md's Kind
  reads `fix (follows <item>)`; `wf status` prints `follows: <item>`. `Add`
  writes "priority <a> -> <b> (renumbered by wf new: <slug> queued at <n>)"
  on every item it moves. `priority` stays unique across the queue.
- **Agents and skills:** workflow-intake places by class, then priority,
  sets `-follows` for a fix, and has the follow-up path (top of class 3, a
  small estimate, the evidence in `-source`, unattended included).
  workflow-native, wf-implement, wf-review and wf-publish file a regression on
  `main` as a follow-up at once; wf-publish's PR body, and the tick and
  native digests, carry **Follow-ups filed** as "<slug> (follows <item>)".
  workflow-human shows the class order and reorders across classes only by
  Jeff's word on `follows` or `kind`.
- **Decisions taken:** a mechanical, derived class read at each call (a1);
  in implementation from Implement, merge phases excluded (a2); one
  `follows` field (a3); holds override class (a4); only a regression already
  on `main` is a follow-up (a5); the finding session files it and reports it
  (a6); the top of class 3 by priority, several in the order found (a7); a
  follow-up waits for its parent except in an attended named run (a8);
  `follows: build-link-subnet` on the four fixes at x4-close, nothing
  renumbered, q1 left open until then (a9).
- **Rejected:** priority numbers set by hand with no code change; a stored
  `class`; re-sorting in every mutator; priority alone within a class;
  "any in-progress item" or "from Plan" as class 1; merge phases in class 1;
  `class: fix|feature`; links in `source` only; class 1 ignoring holds; every
  regression a follow-up whatever its cause; the digest proposing and Jeff
  filing; no wait for the parent, or a wait at `f3-implement` only; priority
  unique per class; compact renumbering.
- **For future designs touching `queue-order`, `priority`, `follow-up`,
  `intake` or `wf-next`:** class is computed, never stored, so a new status
  or phase changes it only through `Class()`; priority compares only within
  a class. A `wf` built before this change drops `follows` when it saves a
  queue file (non-strict YAML), so every machine and human session must
  rebuild `dist/wf` from `main` after the merge; x4-close sets the field on
  the four fixes, updates the workflow branch `README.md`, and posts that
  rebuild notice in the digest. A regression fix and a deferred follow-up
  look the same in the queue; nothing reads the difference.
