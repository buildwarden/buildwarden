---
feature: loop-parallel
status: draft
drivers: []              # the loop's queue code, its checks and its rules; no driver tree is reached
seams: []                # no GLOBAL-08 seam changes
surfaces: []             # `wf` is the loop's tool, not a user-facing surface of the product
security_boundary: none
alters_relationships: false  # no product component's role or interaction changes (FLOW-02);
                         # this changes how the loop relates the items that do
design_review: required  # a process item's design always gets its review (FLOW-03, FLOW-19)
native_validation: []    # pure functions of the queue state, and git commands every host has
tier2_gaps: []
rules: [FLOW-02, FLOW-03, FLOW-04, FLOW-05, FLOW-06, FLOW-07, FLOW-08, FLOW-09,
        FLOW-10, FLOW-11, FLOW-12, FLOW-13, FLOW-14, FLOW-15, FLOW-16, FLOW-17, FLOW-18,
        FLOW-19, FLOW-20, LOOP-01, LOOP-02, LOOP-03, LOOP-04, LOOP-05, LOOP-06, LOOP-07,
        LOOP-08, LOOP-09, PROFILE-01, LINT-02, LINT-03, GLOBAL-08, GLOBAL-09, GLOBAL-10,
        GLOBAL-11, ORCH-01, RELAY-05, TEST-01, TEST-02, TEST-04, TEST-05]
rule_changes:
  - {rule: FLOW-02, change: amend, paths: [docs/development/process.md,
      docs/design/TEMPLATE.md, tools/workflow/check/check.go,
      tools/workflow/check/declared.go, tools/workflow/check/declared_test.go]}
  - {rule: FLOW-03, change: amend, paths: [docs/development/process.md,
      tools/workflow/check/check.go, tools/workflow/check/areas.go,
      tools/workflow/check/areas_test.go, tools/workflow/check/check_test.go]}
  - {rule: FLOW-04, change: amend, paths: [docs/development/process.md]}
  - {rule: FLOW-05, change: amend, paths: [docs/development/process.md]}
  - {rule: FLOW-06, change: amend, paths: [docs/development/process.md]}
  - {rule: FLOW-08, change: amend, paths: [docs/development/process.md]}
  - {rule: FLOW-09, change: amend, paths: [docs/development/process.md]}
  - {rule: FLOW-10, change: amend, paths: [docs/development/process.md,
      tools/workflow/check/check.go, tools/workflow/check/check_test.go]}
  - {rule: FLOW-15, change: amend, paths: [docs/development/process.md]}
  - {rule: FLOW-19, change: amend, paths: [docs/development/process.md,
      tools/workflow/check/review.go, tools/workflow/check/check_test.go]}
  - {rule: FLOW-20, change: amend, paths: [docs/development/process.md]}
  - {rule: LOOP-01, change: amend, paths: [docs/development/loop-contract.md]}
  - {rule: LOOP-04, change: amend, paths: [docs/development/loop-contract.md,
      docs/design/TEMPLATE.md]}
  - {rule: PROFILE-01, change: amend, paths: [docs/development/loop-profile.md,
      tools/tier1/areas.yaml, tools/tier1/rules.yaml, tools/tier1/rules/rules.go, tools/tier1/coupling/checks.go,
      tools/tier1/coupling/areamap.go, tools/tier1/coupling/areamap_test.go]}
concepts: [workflow, queue, one-queue, parallel-items, areas, area-map, relations, collision,
           flow-05-limits, alters-slot, pr-slot, claim-guard, wf-commit, wf-next, wf-claim,
           wf-validate, refresh, consolidation, tracked-items, reconciliation,
           queue-summary, digest, merge-confirmation, main-ci, componentization, program]
related: [componentization.md, alters-slot.md, next-skips-merge-wait.md, queue-summary.md,
          regression-fixes-follow-up.md, design-approved-rule-changes.md,
          tier1-static-checks.md, wf-later-phase-host-steps.md, workflow-bootstrap.md,
          workflow-streamline.md, native-host-loop.md, design-principles.md,
          wf-squash-after-base-moved.md, loop-contract.md]
---

# Items that do not collide run side by side: repositories, areas, relations, the limits, the claim guard, the refresh step and `QUEUE.md`

A process item, and a program item under FLOW-20: P1's third item, and step 3
of the loop change Jeff approved in direction on 2026-11-14 (one queue,
parallel where items do not collide, native-led items). It follows
`alters-slot` (#118, merged) and `loop-contract` (#119, merged on
2026-11-15 as `ad4ea50`), and comes before
`loop-native-lead`, `loop-repository` and the move to the loop repository.
It holds no worker (Decision 17).

Words used throughout. An **item** is a queue item. A **feature** is an item
that is not a process item. **This repository** is `buildwarden/buildwarden`,
where the queue, `wf` and the rules are today. The **proposal** is the loop
change proposal of 2026-11-14, a document of the attended session that is not
kept in the repository; where it and the code disagree the code is followed,
and **Departures from the proposal** lists each place.

## Problem

Jeff, on 2026-11-14, asked for one queue "which allows for more
parallelization for any features that are nearly or completely
non-colliding, i.e. Hyper-V driver feature versus adding a new supported
release target to the relay". The loop cannot give him that today, for seven
reasons. Paths are under `tools/workflow/` at `897a4a8`; `loop-contract`'s
merge (`ad4ea50`) changed none of the code named here.

1. **FLOW-05's limits are for the whole queue.** At most 5 features in
   progress, `limits.alters_relationships` features holding a slot, and one
   feature in the PR slot, whatever the items change. The PR slot is the one
   that binds: a feature finished through Guide waits until the PR ahead of
   it is merged, and only Jeff merges (FLOW-08). So one open PR stops every
   other feature's Publish, including one that shares no file with it.
2. **The queue does not know what an item changes.** A queue file records a
   branch and a design path. It has no field for the repositories an item
   works in (LOOP-04 has the design name them until the queue does), and none
   for the part of the tree it changes. Nothing can tell a Hyper-V item from
   a relay item.
3. **The pairs Jeff means do exist, and the code shows them.** No driver
   tree imports `relay/`, and `relay/` imports no driver package
   (`go list -deps -test` on Linux, macOS and Windows; the table is under
   **The area map**). Of the 52 first-parent merges on `main` whose change
   this clone holds (it is shallow, and the 53rd commit is its boundary),
   seven changed `driver/hyperv/` and five changed `relay/`;
   `hyperv-ps-done` (#116: `driver/hyperv/` and one book page) and
   `relay-leaf-crl` (#70: `relay/` and one book page) share no path but the
   book. How often such pairs occur is measured under a2.
4. **`wf claim` checks no limit.** The three limits are tests inside
   `wf next`, all three for the cloud alone (`atLimit` behind `cloud` in
   `Next` and `NextFor`; the slot tests in `cloudHold`, `queue/next.go`). A step claimed by hand, or by a host on a step whose
   affinity was edited, goes over a limit and only a later `wf validate` may
   say so (`alters-slot`'s Decision 6 left this to this item). `wf validate`
   does not check the PR slot at all.
5. **The PR slot is held by the wrong states.** `prSlotHolder` counts an
   item only once its `f6-merge-main` is done, so two items can each claim
   that step at the same time; and it counts an item in Close, after its PR
   has merged.
6. **Nothing keeps a second open PR current.** With one PR open at a time
   nothing had to. CI runs on a pull request when it is opened or pushed to,
   not when its base moves (`.github/workflows/main.yml`, `on:`), and
   `allow_update_branch` is off. With several PRs open, each merge leaves the
   others built and checked against a `main` that no longer exists. Nothing
   in the loop reads CI's result on `main` either: no skill, agent or `wf`
   command looks at it, so a red `main` is noticed only when a later
   branch's gates fail on it or Jeff sees it.
7. **State pushes do not survive a second worker.** A rejected push of a
   claim means the claim is lost; a rejected push of a finished step is
   re-applied by hand (`workflow-tick` step 5); and every run appends to one
   `digest/<date>.md`, which two runs cannot both push.

Three smaller gaps are closed on the way because the same code is being
rewritten. Nothing compares the queue's `alters_relationships` with the
design after Design's done-when, so a session that writes `false` back passes
every later gate (`alters-slot`, "More holders than the limit"). A
reconciliation item (LOOP-07) has no mark in the queue, and FLOW-05's class 3
is "a feature that `follows` an item" (`Class`, `queue/queue.go`).
`limits.in_progress` absent or 0 is read as no limit, which FLOW-05 does not
say.

## Prior art

- **The proposal**, sections 3 to 8 and 10: the model (three fields, areas
  with a mode, three relations, three limits that count only what is held,
  one guard, `wf commit`, the refresh step, the view in `QUEUE.md`). This
  design keeps the model and settles what it left loose. Its `lead` field,
  host names and wrapper changes are `loop-native-lead`'s; its `work` list
  and second-repository gates are `loop-repository`'s.
- **`alters-slot`** (#118). Kept as built: a feature holds an
  `alters_relationships` slot from the claim of its Plan step until it is
  done; `holdsAltersSlot`, `altersLimit`, the limit of 1 when unset and the
  error below 1; process items neither hold nor wait; a holder keeps its slot
  through a return to Design. Its "What `loop-parallel` changes later" is
  done here: the holders are filtered by collision, the test moves into the
  guard that `wf next` and `wf claim` share for every worker, and `Validate`
  checks each holder against the holders it collides with. Its two known
  gaps (a Plan step claimed past the limit; the queue's
  `alters_relationships` never compared again) are closed.
- **`loop-contract`** (#119, on `main` since 2026-11-15 as `ad4ea50`; its
  rule files are read from `main` at that commit, and are the files its
  review approved at `7af50ff`).
  Decisions 17 to 20,
  the hand-off's `loop-parallel` row and points 5 and 7, and LOOP-04,
  LOOP-06 and LOOP-07 bind this design: one queue; an item names its
  repositories, in its design until the queue has a field; LOOP-04 cites
  FLOW-05 for the order and for what runs side by side; a reconciliation
  item is a feature that FLOW-05 ranks with the fixes (its a5 and a23,
  decided; not asked again here); loop rules change only through a process
  item. **Rule changes** checks each proposed wording against the contract's
  citations.
- **`componentization`** (#117): FLOW-20, whose Limit bullet this item
  removes; its a5 (the limit of 3 while the program runs), which stays the
  value in `config.yaml`; its Decision 18 (a rule changes in the item that
  makes it true), which is why every rule below changes here and not
  earlier.
- **`next-skips-merge-wait`** (#106) kept `wf next` free of git, and
  **`queue-summary`** (#78) made `QUEUE.md` a function of the state alone,
  with no clock. Both are kept: the relations are computed from files in the
  queue state (Decision 3).
- **`design-approved-rule-changes`** and **`tier1-static-checks`**: the
  `rule_changes` route by which a feature edits `tools/tier1/`; the path
  data in `tools/tier1/rules.yaml` (`drivers.all`, `drivers.shared`) that
  the area map starts from; `drivers-claim`, the existing check of a
  declaration against a diff.
- **`wf-later-phase-host-steps`**: a step of a later phase waits for its
  phase and survives `wf back`. The refresh step is a step added outside the
  templates, and FLOW-06 gains a clause for it.
- **`regression-fixes-follow-up`** (#103): the class order, and a
  regression's fix filed at the top of the fixes. Class 3 gains
  reconciliation items below them.
- **`wf-squash-after-base-moved`**: how `wf` proves a squash-merged branch
  is on `main` (ancestor, equal tree, or equal patch-id). Its a3 kept three
  lines of context in the patch-id and held zero context in reserve, "if
  adjacent-edit escalations show up in practice". Merging a PR that is
  behind `main` makes them routine (a14).
- **`workflow-bootstrap`** and **`workflow-streamline`**: the PR slot and
  the single round. **`native-host-loop`**: what a native session may do,
  for the boundary with `loop-native-lead`.

## Decisions

1. **Two items relate in one of three ways, by a total function of what
   they declare.** Colliding, near or disjoint (the table is under **How two
   items relate**). Process items and tracked items are outside it.
   - *Rejected: relations judged by an agent at intake or review.* The
     limits are enforced by `wf`, on every claim, with no agent in the path.
   - *Rejected: a fourth relation for items that share only an ordered
     area.* Decision 4 makes them disjoint.
2. **The queue records what the relation needs, copied from the design by
   `wf`.** New fields: `repos`, `areas`, `seams`, `rules_changed`,
   `adopts`, `collides_with` and `declared`. `wf declare` copies the
   design's front matter into the queue file and records the commit it read
   in `declared`; every later gate of an item that has `declared` compares
   the two. An item that finished Design before `wf declare` existed has no
   `declared`: nothing is compared for it, and it is undeclared.
   - *Rejected: `wf next` reads each design from git.* It reads no git today
     (`next-skips-merge-wait`), and two workers with different fetches would
     compute different relations.
   - *Rejected: the agent copies the fields by hand*, as `wf-design` copies
     `alters_relationships` today. Six fields, and nothing compared them
     afterwards.
   - *Rejected: comparing the queue with the design for every item from the
     merge on.* `seam-specs` and `driver-contract` finished Design with no
     such fields in their queue files, and their designs list seams and rule
     changes; their Plan gate would fail on the day of the merge.
3. **The area map is the repository's, on `main`, under FLOW-14; `wf`
   computes from a generated copy in the queue state** (a4). The map is
   `tools/tier1/areas.yaml`, named by the profile (PROFILE-01). `wf areas
   -sync` copies `origin/main`'s map to `areas.yaml` on the `workflow`
   branch at the start of every session. The map as written for today's
   tree is a5.
   - *Rejected: the map only on the `workflow` branch.* It could not be
     checked against the tree in the pull request that changes the tree.
   - *Rejected: `wf` reads `origin/main` on every command.* `wf next`,
     `wf validate` and `QUEUE.md` would depend on each checkout's fetch, and
     a state one worker produced could fail another's `wf validate`.
   - *Rejected: the map embedded in `wf`.* It would live under
     `tools/workflow/`, change only through a process item, and leave with
     the loop when the loop moves.
4. **An area is exclusive or ordered, and sharing only ordered areas
   relates nothing** (a2). Two items that both change the book, a build file
   or a rule file are disjoint; the refresh step puts the later one in
   order.
   - *Rejected: the proposal's reading, where a shared ordered area makes
     two items near and they take turns at the PR.* Jeff's own example
     would then open one PR at a time, since a Hyper-V item and a relay item
     each touch a book page. Measured on history, 36 of 253 pairs change
     from disjoint to near under that reading (a2).
   - **`cmd/warden/` is an ordered area** (a5 (d), Jeff, 2026-11-15). It is
     the one ordered area that holds code. Two items that both edit the
     CLI are disjoint, both PRs may be open, and the later one meets the
     other's edit when its refresh merges `main`.
   - *Rejected: `cli` exclusive* (the draft's recommendation). Two driver
     items that each add a flag or a subcommand would collide, and K1,
     which rewrites `cmd/warden`, would collide with every other item
     that edits it.
5. **`reaches` is the import graph and nothing else** (a15 (a), Jeff,
   2026-11-15). Area A reaches
   area B when a package in A, or its tests, imports a package in B,
   directly or not, on Linux, macOS or Windows. Tier 1's new `area-map`
   check fails when the map and the graph differ. The areas that hold
   the checks (`tier1`, `tier1-config`) reach what they import, like any
   other: an item that tightens a check and an item whose code the check
   would newly fail are disjoint. Where the two meet is under **Safety**.
   - *Rejected: links written by hand for couplings the graph cannot see.*
     A hand-written link can be neither checked nor kept current. A file
     one area's code reads or copies by path is placed in the area of the
     code that names and tests it: the relay VM's `init` and initramfs
     script are in `driver-core`, beside `driver/relayvm`, which tests
     them, and not in `relay` (a5). Any other such coupling is a seam, and
     the relation reads seams.
   - **Only an exclusive area has `reaches`; an ordered area reaches
     nothing and is reached by nothing** (a5 (d)). `cmd/warden` imports
     every driver and every shared package. If `cli` kept those links, an
     item that edits the CLI would be near every driver item and every
     relay item, and two driver items that both edit it would be near
     each other: they would take turns at the PR, which is what Jeff's
     answer says they do not do ("items that both touch the CLI stay
     disjoint and take turns only through the refresh"). On history the
     difference is ten pairs of 253 (a5). What is given up is under
     **Safety**.
   - *Rejected: `tier1` and `tier1-config` reach every area* (the
     draft's recommendation, a15 (b), and the proposal's rule for those
     two). An item that changes a check or adds a `rules.yaml` entry would
     be near every item in the repository and take turns at the PR with
     each. On history 17 of the 253 pairs would be near for that reason
     alone.
   - *Rejected: `rules` (`docs/development/`) reaching every area too*, as
     the proposal also has it. 22 of the 52 merges changed it, so nearly
     half of all items would take turns at the PR with every other item. A
     rule's wording checks nothing mechanically, and two items that change
     the same rule are near by its ID.
6. **Near items work side by side and take turns at the PR.** Two items are
   near when an area of one reaches an area of the other, when both change
   the same rule, or when both change a seam that has a specification. This
   is "nearly non-colliding".
7. **Each limit is one predicate over the holders, and the guard and
   `wf validate` both evaluate it.** No holder may have more than the limit
   of holders in its *set*: itself and the holders related to it. `Guard`
   applies the claim in memory and evaluates the predicate for the item
   that asks and for every holder related to it; `Validate` evaluates it
   for every holder. A state `Guard` allows therefore never fails
   `Validate`. `Guard` is called by `wf next` (and `wf next -feature`) and
   by `wf claim`, for every worker. The table is under **The limits**.
   - *Rejected: the limits left in `wf next` for the cloud*, as today.
     `loop-native-lead` lets a machine take an item's first step.
   - *Rejected: the guard counting only the holders related to the item
     that asks* (the first draft). The relations are not transitive, so a
     claim whose own count is low can put a holder it relates to over the
     limit; `wf claim` would then admit a state `wf validate` rejects, and
     every later `wf validate` and replay would fail on it.
8. **`limits.in_progress` is 5 when unset and an error below 1**, as
   `alters-slot` did for the other limit. FLOW-05 writes the 5, and the
   live `config.yaml` sets it, so nothing changes for this queue. *Dropped
   fallback:* absent or 0 read as no limit.
9. **The PR slot is held from the claim of `f6-merge-main` until the PR
   merges, a return included, and priority breaks a tie** (a12). A claim
   holds it, expired or not, as a Plan claim holds an
   `alters_relationships` slot. An item with a PR recorded holds it in
   whatever phase it is, so a refresh that sends an item back to Implement
   or Design does not free the slot while its PR is open; the PR is made a
   draft until the item is published again. An item in Close holds
   nothing. A waiting item is refused while a related item of higher
   priority waits for the slot, as the proposal has it.
   - *Rejected: first to ask.* With more than one worker, a low-priority
     item could take the slot the moment Jeff's merge frees it.
   - *Rejected: the higher priority goes first only when it could take the
     slot itself* (the first draft). An item related to two streams that
     are disjoint from each other could then wait without end: while one
     stream holds, the other stream's next item takes the slot (a12).
   - *Rejected: a return gives the slot up while the PR stays open* (the
     first draft, and what `prSlotHolder` does today for a return from
     Publish). A related item could then open its PR beside it, and the
     open PR would no longer be listed among the merges.
   - *Rejected: a tie-break at the other two limits.* `wf next` walks
     FLOW-05's order, and those slots are freed by a worker's own step, not
     by a merge between runs.
10. **`wf commit` pushes queue state, and replays it when the push is
    rejected.** A finished or released step is replayed as a change. A
    claim is not replayed: it is asked again on the new state, through
    everything `wf next` and `wf claim` check, and dropped with that
    refusal when one applies.
    - *Rejected: today's rule, a rejected push loses the claim.* Every
      unrelated push (a machine's status file, another item's step) would
      cost a claim.
    - *Rejected: `git rebase` by each skill.* `QUEUE.md` conflicts on almost
      every replay, and the skills would each resolve it by hand.
    - *Rejected: replaying a claim as a change and running `Validate`* (the
      first draft). `Validate` sees only holders: it does not see the
      tie-break, a hold a process item started in the push that won, or a
      blocking question the human opened in it.
11. **Collision is decided at intake, confirmed by Design and checked
    against the diff.** Intake records `repos` and `areas` with Jeff (a4);
    a follow-up filed with nobody there takes those of the item it
    follows. Design's done-when compares them with what the front matter
    implies.
    Implement, Review, Publish and the refresh fail on a changed path
    outside the areas. Review and consolidation add a trial merge against
    each disjoint in-progress branch, as a report.
12. **An agent may widen an item's areas only inside its approved design,
    and only while the queue stays valid; anything else returns to Design;
    narrowing is the human's.** `wf declare` enforces all three, and reads
    "its approved design" at the commit the last approval names, not on the
    branch the agent is editing.
    - *Rejected: any widening returns to Design.* A design that declares
      `drivers: [hyperv]` and whose diff reaches `driver/hyperv/` has said
      so already.
    - *Rejected: every driver in `drivers` is inside the design.* A change
      to a shared package lists every driver (FLOW-02, tier 1's
      `drivers-claim`) and edits none of their trees. A driver's area is
      inside the design only when the design declares no shared area.
    - *Rejected: an agent may narrow.* Narrowing claims more parallelism
      than the human agreed to.
13. **The refresh is one step, `f6-refresh`, taken by the cloud, seeded
    whenever `main` has moved past an open PR** (a10). It merges `main`,
    runs the gates, and carries the consolidation pass and the sweep over
    what landed. It pushes only while the PR is still open and the remote
    branch is where it found it. It does not queue a native validation
    again.
    - *Rejected: a refresh that re-queues `f6-consolidate`.* Returning to
      Publish seeds `f6-merge-main` again, so the item would give up the PR
      slot while its PR is open.
    - *Rejected: seeding only when a related change landed.* A change from
      outside the queue (a hand-merged PR, the weekly tidy) relates to
      nothing and still moves `main`.
14. **An item homed in another repository is tracked: two human steps, no
    gate, outside the limits** (a7). It shows in `QUEUE.md` with its
    repository, its PR and its relations.
    - *Rejected: the feature round with every step human.* Nine steps to
      record by hand for a task `wf` cannot check.
    - *Rejected: counting it in the limits.* What `wf` counted would rest on
      state entered by hand, and a stale entry would hold real work.
15. **A reconciliation item is marked by `adopts`, waits for its release
    behind a blocking question, and is placed below the open fixes** (a8).
    `Class` returns 3 for a feature that has `follows` or `adopts`.
    - *Rejected: `follows` naming the item that produced the release.* A
      release cut outside the queue has no item to name (`loop-contract`,
      hand-off point 7).
16. **`QUEUE.md` shows repositories, areas, lease holders, what each item
    waits for, the merges in order and the relations; each run writes its
    own digest file and `wf` builds the day's page** (a3).
17. **This item holds no worker** (a13). Nothing it changes reaches a
    running session before its merge; after it, an absent field means
    today's behaviour, an item with no `declared` passes every gate it
    passes today, and the first new field is written at Close.
    - *Rejected: `holds: [cloud, host:*]`.* A hold lasts while the item is
      in progress, from its design to its close: days during which S1 and K1
      would stop.
18. **A hold still names workers and covers work in every repository.**
    *Rejected for now:* a hold that names repositories (`loop-contract`'s
    hand-off leaves it to this item). No step of a second repository is
    taken by a worker a hold could stop until `loop-repository`, which
    adds it with the first such step.
19. **Fourteen rule changes, in this item's PR.** FLOW-02, -03, -05, -10,
    -15, -19 and -20, as the proposal lists; FLOW-04, -06 and -08, which
    `wf commit`, the refresh step, tracked items and a merge recorded in a
    second repository (a6) make untrue as written; FLOW-09, for a red
    `main` the loop itself sees (Decision 22);
    LOOP-01, LOOP-04 and PROFILE-01, for the Areas term and the queue's new
    field. The last three amend files `loop-contract` added, which are on
    `main` since its merge.
20. **The freeze stays.** FLOW-20 loses only its Limit bullet. This is the
    proposal's question 2, option (a), part of the approved direction.
21. **Disjoint PRs in one repository merge in any order** (a1). Nothing
    makes the second wait for its refresh; `QUEUE.md` says when a PR is
    behind `main`, and CI on the push to `main` is the gate.
    - *Rejected: requiring an up-to-date branch.* Each merge would block the
      rest for a cloud run.
    - *Rejected: one open PR per repository.* That is today's rule again.
22. **The loop reads CI on `main`** (a16). Each tick, and the attended session
    before it lists the PRs, records the result of `main`'s head in the
    queue state. A red `main` opens **Needs Jeff**, and the tick files its
    fix as the follow-up of the item whose merge it was (FLOW-09, amended).
    Decision 21 rests on this: without a reader, "CI on `main` is the
    gate" gates nothing.
23. **The merge check compares changes with no context lines** (a14).
    `wf`'s proof that a squashed branch is on `main` takes
    `wf-squash-after-base-moved`'s reserve option, `-U0`. A PR merged
    while behind `main`, next to a line the earlier merge edited, is the
    normal case under Decision 21 and fails the three-line comparison
    wherever the branch's ref still exists.
    - *Rejected: keeping three lines and escalating each such merge.* The
      item would stay at its merge step, holding the PR slot, until Jeff
      compared the two changes by hand.

## Questions

### Blocking

None.

### Assumed

Each is put to Jeff in the design review. a1, a3, a4 and a6 are the
proposal's questions 3, 5, 6 and 11, whose recommended answers he approved
as the direction; they are asked again here because this is where they take
effect. a9 and a11 of the first draft are withdrawn: neither was a fork
(Decisions 18 and 8). a14 to a16 are new after the independent review.

- **a1. Disjoint PRs in one repository: in what order may they merge?**
  (The proposal's question 3.) CI does not run again when a PR's base moves.
  Proceeding with (a); confirm.
  - (a) Any order. `main`'s protection then requires a pull request and
    green checks, and does not require the branch to be up to date. What
    stands in for an up-to-date branch, and how far each reaches:
    - *"Behind `main`" in `QUEUE.md`.* It is as current as the last session
      that ran `wf stale`: the daily tick, or the attended session, which
      runs it before it lists the PRs and again after each merge Jeff makes
      in it. A PR he merges on GitHub between sessions shows its last green
      run and no mark. Merging from the attended listing closes that gap.
    - *CI on the push to `main`.* The tick and the attended session read it
      (Decision 22). A red `main` opens **Needs Jeff**, and its fix is
      filed as the follow-up of the item whose merge it was, with that
      item's areas (FLOW-09, a4).
    - *The merge check.* A PR merged while behind `main` is still confirmed
      as merged (a14).
  - (b) An up-to-date branch is required. Each merge blocks the rest until
    their refresh, about one merge per cloud run.
  - (c) One open PR per repository: today's rule, per repository.

  Recommended: (a). Disjoint PRs share no exclusive area, and GitHub refuses
  a merge that conflicts in a shared file. What (a) accepts is under
  **Safety**: a break between two disjoint items that neither branch shows
  alone lands on `main` and is fixed there.
- **a2. What sharing only ordered areas costs** (the book, build files,
  rule files, other items' designs). Proceeding with (a); confirm. This
  departs from the proposal, whose section 1 put "lightly shared files"
  with "one PR at a time".
  - (a) Nothing but the refresh: the two items are disjoint, and both PRs
    may be open. The later one merges `main` at its refresh, where a
    conflict in the shared file is resolved.
  - (b) They are near and take turns at the PR, as the proposal has it.

  Recommended: (a). Measured on history, with the map below and the areas
  each merge really changed: of the 23 merges that were not loop work and
  changed fewer than 100 files, 253 pairs, (a) gives 187 disjoint, 43 near
  and 23 colliding, and (b) gives 151, 79 and 23. So (b) makes 36 pairs
  (14%) take turns that (a) lets open side by side, Jeff's own example
  among them when each item touches a book page. It changes nothing for
  the most recent work: among the last six product merges
  no pair is disjoint under either option, 8 colliding and 7 near, because
  each changed `cmd/warden/` or a package every driver imports. Under (a)
  the risk is a PR merged while behind `main` whose shared file merges
  cleanly and wrongly (two edits to `go.mod`, for example); 14 of the 187
  disjoint pairs changed a common file, most often
  `docs/development/README.md`. CI on `main` is the net, as in a1. The
  change between the two options is one row of the relation table.
- **a3. A cap on work waiting for Jeff.** (The proposal's question 5.)
  Proceeding with (a); confirm.
  - (a) None. `QUEUE.md` opens **Needs Jeff** with the count of items
    waiting on him, by kind.
  - (b) No queued item starts while 8 or more wait on him.

  Recommended: (a). A cap would stop the loop on the one resource it cannot
  add to, and the count makes the backlog visible.
- **a4. Who declares areas, and where the map lives.** (The proposal's
  question 6, with its letters.) Proceeding with (a) and (d); confirm.
  - (a) Areas are proposed at intake with Jeff and confirmed by Design; a
    design that narrows what intake recorded cannot skip its review. A
    follow-up filed with nobody there (FLOW-09) takes the repositories and
    areas of the item it follows, which Jeff agreed for that item; its
    Design may add to them, and removing one needs the review.
  - (b) Areas are declared by Design only. A follow-up is then undeclared
    until its design: it collides with every feature, cannot start while
    five are in progress, and may open its PR only when no other feature
    has one open, which makes the fix for a red `main` the slowest item in
    the queue.
  - (c) The map is on the `workflow` branch, changed in an attended session
    on Jeff's word.
  - (d) The map is on `main`, at `tools/tier1/areas.yaml`, as the profile's
    Areas entry: a feature changes it only as a rule change its design
    review approves by name (FLOW-14), and tier 1 checks it against the
    tree.

  Recommended: (a) and (d).
- **a5. The map itself**, written out under **The area map**: 16 exclusive
  areas and 5 ordered ones. `driver-core` is one area (`driver/*.go`,
  `buildlink`, `resolvers`, `relayvm`, `script`, `linuxguest`, `winguest`,
  and the relay VM's `init` and initramfs script, which `driver/relayvm`
  tests and the Hyper-V boot-disk script copies). `relay`, `relaycfg`,
  `ledger`, `collector` and `executor` are five areas. `cli`
  (`cmd/warden/`) is exclusive. The book, `docs/development/`, the build
  files, other items' designs and tier 1's configuration are ordered.
  Proceeding with (a); confirm, or change an area.
  - (a) As written.
  - (b) Finer: the guest families apart from `driver-core`, so more items
    are near and fewer collide.
  - (c) Coarser: `rules.yaml`'s shared list (`relay/`, `ledger/`,
    `collector/`, `relaycfg/`, `bootstrap/`, `driver/*.go`) as one area.
  - (d) `cli` ordered, so two driver items that each edit `cmd/warden/` do
    not collide. They collide today until K1 moves the driver flags out:
    five of the 52 merges changed it. **Chosen.**

  Recommended: (a).

  **Answered (d), Jeff, 2026-11-15:** the map as written, with `cli` an
  ordered area: "cmd/warden becomes an ordered area: items that both
  touch the CLI stay disjoint and take turns only through the refresh."
  It departs from the recommendation. Folded in: the map has 15 exclusive
  areas and 6 ordered ones; `cli` has no `reaches`, because an ordered
  area has none (Decision 5); the numbers were run again. Of the 253
  pairs, 197 are now disjoint, 34 near and 22 colliding (187, 43 and 23
  with `cli` exclusive): the one pair in which both merges changed
  `cmd/warden/` (`hyperv-clean`, #114, and `vz-image-prepare-refresh`,
  #108, which share no file) goes from colliding to disjoint, and nine
  pairs go from near to disjoint, each a pair whose only link was
  `cmd/warden` importing the other merge's package. Among the last six
  product merges, 2 pairs are disjoint, 6 near and 7 colliding (none, 7
  and 8 before). Had `cli` kept its `reaches`, the figures would be 187,
  44 and 22: the colliding pair would be near, not disjoint. What the
  answer changes for K1 is under **What changes for the queue as it
  stands**, and its risk under **Safety**.
- **a6. Steps in a second repository.** (The proposal's question 11.)
  Proceeding with (a); confirm.
  - (a) Allowed for a repository with no loop of its own, as `seam-specs`
    does: the item lists the repository in `repos`, its design says which
    invariants apply there (LOOP-08), and a `human` step records a merge
    there that `wf` cannot see. FLOW-04 and FLOW-08 gain the sentence that
    `seam-specs`' a15 asked for as an exception.
  - (b) Only through a release (LOOP-07).

  Recommended: (a).
- **a7. Whether a tracked item counts in the limits.** Proceeding with (a);
  confirm.
  - (a) It is shown with its relations and neither holds nor waits. K1 and
    the relay's M1 both define parts of seam 8, which has no specification;
    `QUEUE.md` says they collide, and nothing is enforced until
    `loop-repository` gives the relay's work steps and gates.
  - (b) It counts like any feature, on the state the human enters.

  Recommended: (a).
- **a8. A reconciliation item's mark, its wait and its place.** That it is
  a feature in class 3 is decided (`loop-contract` a5). Proceeding with (a);
  confirm.
  - (a) Marked by `adopts: {consumable, version}`. Filed on hold with a
    blocking question that names the release when the release does not exist
    yet (LOOP-04), which Jeff or an attended session answers when it does.
    Intake places it below the open fixes; a regression's fix still goes to
    the top.
  - (b) As (a), placed at the top of the fixes.
  - (c) As (a), with no default place: Jeff says each time.

  Recommended: (a). A regression means `main` is broken now; an adoption is
  planned work.
- **a10. The refresh: its trigger, its budget, and native validation.**
  Proceeding with (a), (c) and (e); confirm.
  - (a) Seeded for every item waiting on Jeff's merge whenever `main` is no
    longer contained in its branch, whatever moved `main`.
  - (b) Seeded only when a related item's PR merged.
  - (c) Each refresh is a claim and counts one step against the item's
    budget; intake's size guide adds two steps for it.
  - (d) A refresh spends no budget, like the merge confirmation.
  - (e) A refresh does not queue a blocking native validation again. The
    validation stands for the branch as it was consolidated; what merged
    since came from items disjoint from this one.
  - (f) A refresh queues each done blocking host step again when the
    commits it merged changed a file outside the ordered areas: one more
    native session per such merge, with the open PR shown as waiting on a
    host.

  Recommended: (a), (c) and (e). The cost of (a) and (c) with five PRs
  open: merged one a day, they take 4 + 3 + 2 + 1 = 10 refreshes, up to
  four on the last item, against the two the size guide adds; each is a
  merge, lint, test, deadcode, a consolidation pass and a CI wait, taken
  in class 1 ahead of new work. Merged in one sitting they take none, and
  four of the five merge behind `main` (a1). An item that waits through
  many merges trips its runaway check and shows under **Needs Jeff**,
  which is the right place for a PR that has waited that long. Under (e),
  a break between two disjoint items that only a native host shows is
  found by the matrix before a release (TEST-02): CI runs neither Hyper-V
  nor VZ, and the scheduled native session sweeps only a machine's open
  backlog gaps.
- **a12. Who gets a free PR slot when related items wait.** Proceeding
  with (a); confirm.
  - (a) The higher priority, whether or not it could take the slot at that
    moment: an item is refused while a related item of higher priority
    waits. This is the proposal's rule. A lower-priority item can then
    wait for a holder it is disjoint from, because the item ahead of it
    does.
  - (b) The higher priority only when it could take the slot itself (the
    first draft). More PRs are open at once, and a high-priority item
    related to two streams that are disjoint from each other can wait
    without end. Example: an item in `driver-core` (priority 1) is near
    every driver item. While a Hyper-V item holds, a VZ item (priority 3)
    takes the slot; Jeff merges the Hyper-V PR, the `driver-core` item is
    still refused (the VZ item holds), and the next Hyper-V item
    (priority 4) takes it; and so on while driver items keep arriving.
  - (c) The first to ask.

  Recommended: (a). It only matters once a second worker exists: an attended
  run on a named item today, a machine's led item after `loop-native-lead`.
- **a13. Whether this item holds a worker.** Proceeding with (a); confirm.
  - (a) None. Two things are then handled without a hold. A session whose
    `dist/wf` was built before the merge drops a new field when it saves a
    queue file that carries one: Close writes the first new field only
    after no lease predates the merge, and every scheduled session rebuilds
    `wf` as it starts. And `seam-specs` and `driver-contract` finished
    Design before the queue could record what they declare: no gate
    compares their queue files with their designs until a human
    `wf declare` records them, which the attended step `x4-state` does.
  - (b) `holds: [cloud, host:*]`, set by hand for the day of the merge and
    Close and removed after.

  Recommended: (a).
- **a14. The merge check when a PR merged behind `main`.** `wf` confirms a
  squash merge by one of three proofs for each ref of the branch that
  still exists: the ref is an ancestor of `main`, a PR commit has its
  tree, or a PR commit makes the same change by patch-id
  (`wf-squash-after-base-moved`). The third is computed with three lines
  of context, so it fails when the PR merged behind `main` next to a line
  the earlier merge edited. That design's a3 chose three lines and said to
  change "if adjacent-edit escalations show up in practice". Proceeding
  with (a); confirm.
  - (a) Zero context (`-U0`), that design's option (b). The proof becomes
    "the same lines added and removed in each file, in order", without the
    lines around them. A squash GitHub made cannot move a hunk. It is the
    proof Close relies on before a squashed local branch is deleted
    (FLOW-16).
  - (b) Keep three lines. Each such merge fails its confirmation wherever
    the branch's ref still exists (an attended checkout with a worktree, a
    run that worked on the branch): the item stays at its merge step,
    keeps the PR slot, and waits for Jeff to compare the two changes by
    hand.

  Recommended: (a). Under a1 (a) the case is ordinary: any two PRs that
  each add an entry to the rule index, `rules.yaml` or a book table.
- **a15. Whether the areas that hold the checks reach every area.** The
  proposal has "`tier1`, `rules` and `tier1-config` reach every area: a
  tightened check meets all code". The first draft dropped it for the
  import graph alone, on the ground that the same `make lint` finds the
  failure at `f6-merge-main` or at the refresh. That ground does not hold
  under a1 (a), where a PR may merge before its refresh. Proceeding with
  (b); confirm.
  - (a) The import graph alone. An item that tightens a check and an item
    whose code the check newly fails are disjoint: both PRs may be open
    and merge in either order, and the failure first shows on `main`.
    **Chosen.**
  - (b) `tier1` (the checks' code) and `tier1-config` (`rules.yaml`,
    `golangci.yml`, the area map) reach every area: an item that changes
    either is near every item in the repository and takes turns at the PR
    with it, so the later of the two merges `main` and meets the check
    before its PR opens. `rules` (`docs/development/`) does not: wording
    checks nothing mechanically, and 22 of the 52 merges changed it.
  - (c) As the proposal: `rules` too. Nearly half of all items would then
    take turns at the PR with every other item.

  Recommended: (b). On history it costs little: one of the 23 merges
  changed either area, and 17 of the 253 pairs become near. It does not
  cover a check that lives in a test of a code area and walks the whole
  tree (`driver/buildlink/stray_test.go` reads every file;
  `specs/specs_test.go` reads the book's `SUMMARY.md`), nor the root
  `.golangci.yml`; those are part of what a1 (a) accepts.

  **Answered (a), Jeff, 2026-11-15:** "Import graph only. A tightened
  check and the code it would fail are disjoint and meet on main, where
  the break is then fixed." It departs from the recommendation. Folded
  in: `reaches_all` is gone from the map's format and from `tier1` and
  `tier1-config`; `tier1` reaches `loop`, which it imports, and
  `tier1-config` nothing; FLOW-05's proposed sentence on what reaches
  means loses its clause on checks. The option was written for a1 (a).
  With a1 (b), which Jeff also chose, the two usually meet one step
  before `main`, at the refresh of whichever merges second; **Safety**
  says when, and when they still meet on `main` (a16).
- **a16. A regression that CI reports on `main`.** FLOW-09 has a session
  file a regression `main` already has when it finds one "while the
  feature is built, validated or consolidated". Nothing files one that CI
  reports on `main` itself. Proceeding with (a); confirm.
  - (a) The run that sees `main` red files the fix, unattended as FLOW-09
    already allows, as the follow-up of the item whose pull request that
    merge was, at the top of the fixes. When the red commit is no item's
    (a hand merge, the weekly tidy), it files nothing and **Needs Jeff**
    says `main` is red.
  - (b) It only reports: **Needs Jeff** opens with the red `main`, and the
    fix is filed when a later step's gates meet the failure, as the
    follow-up of that step's item (FLOW-09 as written), or by Jeff.

  Recommended: (a). Under a1 (a) a red `main` is the designed outcome of a
  missed collision, and each open PR's refresh would otherwise fail on it,
  run after run, until some item's session filed the fix.

## Design

### What an item declares

**In the queue file.** Seven fields, all optional. A queue file without them
behaves as it does today.

```yaml
repos: [buildwarden/buildwarden, buildwarden/buildwarden-specs]
areas: [driver/hyperv, book]
seams: [relay-claim]
rules_changed: [GLOBAL-08, TEST-05]
declared: 4c1d9e2
adopts: {consumable: buildwarden-relay, version: v0.3.0}
collides_with: [some-other-item]
```

| Field | What it is | Absent means | Written by |
|---|---|---|---|
| `repos` | The repositories the item works in, as `owner/name`, its home first. The home holds its design, its branch and its PR | Only the queue's own repository, `config.yaml`'s `repository` | `wf new -repos`, or copied from the item it `-follows`; `wf declare` |
| `areas` | Names from the home repository's area map: the areas whose files the item changes. Its own design file needs no area | *Undeclared*: the item collides with every item it shares a repository with. `areas: []` is different: declared, and it changes nothing but its own design | `wf new -areas`, or copied from the item it `-follows`; `wf declare` |
| `seams` | The design's `seams` (FLOW-02): the seams the item changes | None | `wf declare`; `wf new -seams` for a tracked item, whose design `wf` cannot read |
| `rules_changed` | The rule IDs of the design's `rule_changes` (FLOW-14) | None | `wf declare` |
| `declared` | The commit of the item's branch whose design `wf declare` last copied | The item has never been declared: it finished Design before `wf declare` existed, or has not finished it. No gate compares its queue file with its design | `wf declare` |
| `adopts` | Marks a reconciliation item (LOOP-07): the consumable and the version it adopts | Not a reconciliation item | `wf new -adopts <consumable>@<version>` |
| `collides_with` | Items the human has said this one collides with, whatever the areas say | None | `wf relate`, a human worker only |

`config.yaml` gains one key, `repository: buildwarden/buildwarden`: the
repository whose `wf` gates run and whose area map `areas` names. It has no
default. While it is unset, an item that writes `repos` fails `wf validate`
(`config: repository is not set, and <item> names repos`).

An item whose first repository is not `config.yaml`'s `repository` is
**tracked** (see **Tracked items**).

**In the design.** FLOW-02's front matter gains two keys, and the template
with it:

```yaml
repos: []                # every repository the item works in, home first, when there is
                         # more than this one (LOOP-04); empty or absent: only this one
# areas: []              # the areas the change edits (FLOW-02), from tools/tier1/areas.yaml.
                         # Left out, the item is undeclared and collides with every item
                         # in its repositories (FLOW-05)
```

`areas` is commented out in the template so that a design which says nothing
is undeclared, not declared-empty. Neither key is required: tier 1's
`design-front-matter` check does not fail a design, archived or open, for
lacking them. `seams`, `rule_changes`, `alters_relationships` and
`design_review` are the keys FLOW-02 already has.

**`wf declare -state DIR -repo DIR -worker W -feature F`** copies the design
on the item's branch into the queue file: `repos`, `areas`, `seams`, the
rule IDs of `rule_changes`, `alters_relationships` and `design_review`, and
writes the branch's commit to `declared`. It
replaces the hand copy in `wf-design`'s step 5. What it accepts depends on
where the item is and who asks:

| When | Who | What it accepts |
|---|---|---|
| Design not finished (a feature in phase 0 or 1; a process item in phase 1) | Any worker | Any value. One exception: with `design_review: skip`, a change that *narrows* is refused, with "narrowing needs the design review (FLOW-19)". Narrowing is removing a repository or an area the queue has, or declaring areas where the queue has none |
| In Plan | A `cloud` or `host:` worker | Nothing: the Plan gate compares the design with its approval (FLOW-19) |
| From Implement on | A `cloud` or `host:` worker | Only added areas, each *within the approved design* (below). Every other difference is refused, with "a design change: back to Design (FLOW-06)" |
| After Design | A `human` worker | Any value. This is how the review narrows, and how an item that finished Design before this change is declared, with or without areas. A change made in the review is part of the design the approval names |

**Within the approved design.** `wf declare` reads `drivers`, `surfaces`
and `rule_changes` from the design at the commit its last `approved at
<sha>` names (`approvals`, `check/review.go`), not from the branch, which
the same agent is editing. Only a design whose review was skipped is read
on the branch: it has no approved commit, and the skip criteria leave it no
seam and no rule change. An added area is within the design when it is:

- an area that holds a path of one of the design's `rule_changes`;
- an area the map gives one of its `surfaces` (`implies.surfaces`);
- the area of a driver in its `drivers` (`implies.drivers`), when the
  design declares no *shared* area (`implies.shared`). A design that
  declares a shared area lists every driver because tier 1's
  `drivers-claim` requires it, not because it edits their trees, so for it
  a driver's tree is a design change.

In every case the names must exist in the branch's area map, and the change
must not put a limit over: `wf declare` applies the change in memory, runs
the limits' predicate (**The limits**), and writes nothing when the change
adds a limit error. Its error
names the limit and the holders, and says "back to Design (FLOW-06)". It
logs what changed in the item's history (`declared at 4c1d9e2: areas
+driver/hyperv (within drivers: hyperv)`).

A process item is outside the relations, so none of its done-when checks
asks for a declaration. `wf declare` on one records its `repos`, which
`QUEUE.md` shows, and nothing requires it.

**`wf relate -state DIR -worker human/<name> -feature A -with B [-clear]`**
writes or clears `collides_with` on A. Only a `human` worker may run it, and
it is refused, like `wf declare`, when the result would put a limit over.

**`wf new`** gains `-repos`, `-areas`, `-seams` and `-adopts`. With
`-follows X` and no `-areas`, it copies X's `repos` and `areas` and logs
`repos and areas taken from X`; when X has none, the new item has none.
`-seams` is refused for an item homed in this repository, whose seams
`wf declare` copies.

This replaces the sentence `alters-slot` added to `wf-design`'s step 5 (its
a4): a redesign that flips `alters_relationships` to `true` past the limit is
now refused by `wf declare`, and the agent raises the blocking question as
before. The new sentence is under **Machinery edits**.

**`wf validate`** checks, for the new fields:

- `repos`: each entry is `owner/name`; none repeats; `config.yaml` sets
  `repository`; a process item's first repository is that one.
- `areas`: no name repeats. A name the copy of the map does not have is not
  an error: the item is then treated as undeclared until `main` has the
  area, and `QUEUE.md` marks the name with `*` (a design may add an area in
  its own rule change, and `main` gets it at the merge).
- `declared`: a commit id, and set whenever `seams` or `rules_changed` is,
  on an item homed in this repository.
- `adopts`: `consumable` and `version` are both set, and the item is a
  feature.
- `collides_with`: each entry names another item in the queue.
- A tracked item: `kind` is not `process`, `holds` is empty, and its round
  is `tracked`.
- The three limits, per holder (see **The limits**).

### The area map

**Where.** `tools/tier1/areas.yaml` on `main`. The profile names it
(PROFILE-01, the new **Areas** term of LOOP-01), so it is the component's
own: a feature changes it only through a `rule_changes` entry under
PROFILE-01 that its design review approves by name (FLOW-14), and the
rule-change clearance already guards every path under `tools/tier1/`. A
repository whose profile has no Areas entry is one exclusive area.

**How `wf` reads it.** `wf areas -sync -state DIR -repo DIR` writes
`origin/main`'s map to `areas.yaml` in the queue state, with a header that
names the repository and the commit, and the list of written seams (below).
Every session runs it in its set-up, after the fetch; the changed file goes
out with the session's first state push. `main`'s map is the truth, so the
copy is written even when the new map relates two items that both already
hold a limit; `wf areas -sync` then prints the limit and the holders, and
`QUEUE.md` lists them under **Needs Jeff** (see **More holders than a
limit**). `queue.Load` reads the copy as it
reads `hosts.yaml`: an optional file. Every `wf` command then computes the
same relations from the same pushed state, on every machine, with no git.
`wf areas -check` fails when the copy differs from `origin/main`'s map;
`wf areas -list` prints the areas. `wf validate -areas FILE` validates the
queue against a candidate map, which is how the design review of a map
change sees what it does to the open items.

**Format.**

```yaml
version: 1
modules: [".", tools/tier1, tools/workflow]   # where the check runs `go list`
catch_all: core
areas:
  <name>:
    mode: exclusive | ordered
    paths: [<pattern>, ...]
    reaches: [<area>, ...]
implies:
  drivers: {<driver>: <area>, ...}
  shared: [<area>, ...]
  surfaces: {<surface>: [<area>, ...], ...}
written_seams: [<seam>, ...]
```

- **A pattern** is a path from the repository root: a directory, ending in
  `/`, which matches everything under it; a file; or a file name with `*`
  in it, which matches files in that one directory. A path belongs to the
  area whose matching pattern has the longest text before any `*`, so
  `tools/relay-vm/hyperv/` wins over `tools/relay-vm/`. Two areas matching a
  path equally is an error. A path no pattern matches belongs to
  `catch_all`.
- **`mode`** is required. *Exclusive*: two items that both change the area
  collide. *Ordered*: any number of items may change it side by side, and
  the later one to merge is put in order by its refresh (Decision 4).
- **`reaches`** lists the areas this one depends on, which is the import
  graph (Decision 5). Only an exclusive area may have it, and it may name
  only exclusive areas: an ordered area with `reaches`, or one named in
  another area's `reaches`, is an error. Unknown keys are rejected, as in
  `rules.yaml`.
- **`implies`** says which area a value in a design's front matter stands
  for, so `wf` holds none of this repository's vocabulary (LOOP-08).
  `drivers` gives each driver's own area. `shared` lists the areas that
  hold a path every driver depends on (`rules.yaml`'s `drivers.shared`): a
  design that changes one of them lists every driver and edits no driver's
  tree. `surfaces` gives each surface's areas.
- **`written_seams`** lists seams that have a specification and that
  `rules.yaml`'s `seams` list does not carry. `wf areas -sync` adds the
  names in `rules.yaml`'s `seams` list, which S1 fills, so a seam becomes
  written in the relation when its contract document is listed there, with
  no second edit.

**The map for `main` at `897a4a8`**, in full. It was run against
`git ls-files`: each of the 627 tracked files falls in one named area, with
no tie, and `core` is empty today. It has 15 exclusive areas and 6 ordered
ones.

```yaml
version: 1
modules: [".", tools/tier1, tools/workflow]
catch_all: core
areas:
  driver/container:
    mode: exclusive
    paths: [driver/container/, cmd/warden-pause/]
    reaches: [collector, driver-core, executor, ledger, relaycfg]
  driver/qemu:
    mode: exclusive
    paths: [driver/qemu/]
    reaches: [collector, driver-core, executor, ledger, relaycfg]
  driver/vz:
    mode: exclusive
    paths: [driver/vz/, cmd/warden-vmnet/, tools/vz-image-prep/, entitlements.plist]
    reaches: [collector, driver-core, executor, ledger, relaycfg]
  driver/hyperv:
    mode: exclusive
    paths: [driver/hyperv/, tools/relay-vm/hyperv/]
    reaches: [collector, driver-core, executor, ledger, relaycfg]
  driver/firecracker:
    mode: exclusive
    paths: [driver/firecracker/, tools/firecracker-vm/]
    reaches: [collector, driver-core, executor, ledger, relaycfg]
  driver-core:
    mode: exclusive
    paths: [driver/, tools/build-vm/, tools/relay-vm/, tools/win-image-prep/]
    reaches: [collector, driver/firecracker, driver/hyperv, driver/qemu, executor, ledger,
              relay, relaycfg]
  relaycfg:
    mode: exclusive
    paths: [relaycfg/]
  relay:
    mode: exclusive
    paths: [relay/, cmd/relay/, Dockerfile.relay]
    reaches: [collector, ledger, relaycfg]
  ledger:
    mode: exclusive
    paths: [ledger/]
  collector:
    mode: exclusive
    paths: [collector/, cmd/collector/]
    reaches: [driver-core, executor, ledger, relaycfg]
  executor:
    mode: exclusive
    paths: [bootstrap/, cmd/warden-io/]
  specs:
    mode: exclusive
    paths: [specs/, docs/book/src/specs/]
    reaches: [collector, ledger, relay, relaycfg]
  tier1:
    mode: exclusive
    paths: [tools/tier1/]
    reaches: [loop]
  loop:
    mode: exclusive
    paths: [tools/workflow/, .claude/, .entire/]
  core:
    mode: exclusive
  cli:
    mode: ordered
    paths: [cmd/warden/]
  book:
    mode: ordered
    paths: [docs/book/, examples/, README.md, CONTRIBUTING.md, SECURITY.md, AGENTS.md,
            CLAUDE.md, LICENSE, NOTICE, logo.png]
  rules:
    mode: ordered
    paths: [docs/development/]
  designs:
    mode: ordered
    paths: [docs/design/]
  build:
    mode: ordered
    paths: [.github/, Makefile, go.mod, go.sum, .gitattributes, .gitignore, .golangci.yml,
            .goreleaser.yaml, .mise.toml, aqua.yaml, install.sh, Dockerfile,
            Dockerfile.ledger.zst, test-integration.sh]
  tier1-config:
    mode: ordered
    paths: [tools/tier1/rules.yaml, tools/tier1/golangci.yml, tools/tier1/areas.yaml]
implies:
  drivers:
    container: driver/container
    qemu: driver/qemu
    vz: driver/vz
    hyperv: driver/hyperv
    firecracker: driver/firecracker
  shared: [collector, driver-core, executor, ledger, relay, relaycfg]
  surfaces:
    cli: [cli, book]
    config: [cli, book]
    output: [book]
written_seams: [ledger-format]
```

Why each area is what it is:

| Area | Mode | Holds | Why |
|---|---|---|---|
| `driver/<name>` (five) | exclusive | A driver's tree (`rules.yaml`, `drivers.dirs`) and the assets only it uses: the pause binary (container), the vmnet helper, the image-prep tools and the signing entitlements (vz), the Hyper-V relay boot-disk scripts, the firecracker kernel and slot scripts | One driver, one area. No driver tree imports another |
| `driver-core` | exclusive | `driver/*.go` and the packages drivers share: `buildlink`, `resolvers`, `relayvm`, `script`, `linuxguest`, `winguest`; the build VM and Windows image tools the guest families use; the relay VM's `init` and `build-initramfs.sh` (`tools/relay-vm/`). `driver/` is the pattern, so a new shared package lands here | `rules.yaml`'s `drivers.shared` lists the first five and omits `buildlink` and `resolvers`, which four driver trees and `cmd/warden` import. The relay VM's `init` is here and not in `relay` because the code that depends on it is here: `driver/relayvm/init_test.go` reads it by path, `driver/hyperv/relayseed.go` writes the file it parses, and `tools/relay-vm/hyperv/build.sh` copies it. No import shows that, so the file is placed with the code that names it; a change to it is then near every driver item |
| `relaycfg` | exclusive | The claim's per-build configuration | Seam 1's package. Every driver, the relay, the collector and the CLI reach it |
| `relay` | exclusive | `relay/`, `cmd/relay/`, `Dockerfile.relay` | The Go relay, which stays here until M6. A new release target for it changes these and the build files, and no driver's |
| `ledger` | exclusive | `ledger/` | The format's reader and writer |
| `collector` | exclusive | `collector/`, `cmd/collector/` | The collector and its reference consumer |
| `executor` | exclusive | `bootstrap/`, `cmd/warden-io/` | The in-environment pieces and the bootstrap configuration they read |
| `specs` | exclusive | `specs/` and its mirror pages in the book | The ledger wire format and schemas (RELAY-05) |
| `tier1` | exclusive | Tier 1's code | A check's code is changed by few items (3 of the 52 merges), each by an approved rule change. It reaches `loop`, which it imports, and no other area: a changed check is disjoint from the code it runs over (a15 (a)) |
| `loop` | exclusive | `tools/workflow/`, `.claude/`, `.entire/` | The loop's machinery. Process items change it and are outside the relations. A feature may not: `loop-contract`'s FLOW-14 lets a feature's design declare no change to a loop rule or its enforcement |
| `core` | exclusive | Every path no pattern matches. Empty today | A new top-level tree collides with any other until the design that adds it gives it an area |
| `cli` | ordered | `cmd/warden/` | The orchestrator's entry point, and the one ordered area that holds code (a5 (d)). Five of the 52 merges changed it; three of the last six product merges did, two of them by a file or two beside one driver's tree. Such items no longer collide, and none of them is near another item because `cmd/warden` imports it: an ordered area has no `reaches` |
| `book` | ordered | The book, `examples/`, and the root documents | Changed by 16 of the 52 merges, a page or two each |
| `rules` | ordered | `docs/development/` | Changed by 22 of the 52. Two items that change the *same rule* are near by its ID, whatever the file |
| `designs` | ordered | `docs/design/`: other items' designs and the template | An item's own design needs no area. This is for a retrofit or an amendment to an archived design |
| `build` | ordered | CI workflows, `Makefile`, `go.mod`, `go.sum`, lint and release configuration, the root `Dockerfile` and its ledger, the install and integration scripts | Changed a line at a time by many items |
| `tier1-config` | ordered | `rules.yaml`, `golangci.yml`, `areas.yaml` | Each item adds its own entry, so two may change it side by side (5 of the 52 merges changed it). It reaches nothing, as every ordered area (a15 (a), a5 (d)) |

**`reaches`, as computed.** From `go list -deps -test ./...` in each of the
three modules, for Linux, macOS and Windows on amd64 and arm64: 18 runs,
about 2 s in all. `CGO_ENABLED` is 1 for macOS on arm64, so that
`driver/vz`'s cgo files are seen, and 0 for the other five: with cgo on,
macOS on amd64 fails on `driver/vz` (its Objective-C file has no cgo file
beside it there). Each package's directory is mapped to its area:

| Area | Reaches |
|---|---|
| `cli` | By import: `collector`, `driver-core`, all five drivers, `executor`, `ledger`, `relay`, `relaycfg`. The map records none of it: `cli` is ordered (a5 (d)) |
| `collector` | `driver-core`, `executor`, `ledger`, `relaycfg` (`cmd/collector` imports `driver`) |
| `driver-core` | `collector`, `executor`, `ledger`, `relay`, `relaycfg`; and `driver/firecracker`, `driver/hyperv`, `driver/qemu` through `driver/buildlink`'s tests |
| each `driver/<name>` | `collector`, `driver-core`, `executor`, `ledger`, `relaycfg` |
| `relay` | `collector`, `ledger`, `relaycfg` |
| `specs` | `collector`, `ledger`, `relay`, `relaycfg` (its tests) |
| `tier1` | `loop` (`tools/tier1` imports `tools/workflow/check`) |
| `relaycfg`, `ledger`, `executor`, `loop`, `core`, `book`, `rules`, `designs`, `build`, `tier1-config` | nothing |

`relay` is reached by `driver-core`'s tests and by no driver tree
(`cmd/warden`'s tests reach it too, which the map does not record), and it
reaches no driver package. That is why a Hyper-V item
and an item in `relay/` or `cmd/relay/` are disjoint.

**The check that the map and the graph agree** is a tier-1 check,
`area-map`, mapped to PROFILE-01 in `rules.yaml`. It is tier 1's, not
`wf`'s, because it needs the Go toolchain and the loop's tool may assume
none (LOOP-08). It reads `tools/tier1/areas.yaml` from the checkout it
checks, not from a copy embedded in the binary as `rules.yaml` is: the map
describes one tree, and `main`'s tier 1 judging a branch (FLOW-14's
comparison, `enforcement-check.sh`) must read the branch's map, which it
then does with no change to that script or to FLOW-14's list of
configuration files. A checkout with no map gets no finding. It runs when
the diff touches `areas.yaml`, `rules.yaml`, a `.go` file or a `go.mod`,
over the 18 `go list` runs above, and fails when:

- the map does not parse, an area has no `mode`, `catch_all` does not name
  an exclusive area with no paths, an ordered area has `reaches` or is
  named in one, or a `reaches` or `implies` entry names an area the map
  does not have;
- a tracked file matches two areas equally;
- a `go list` run fails. The finding is its error;
- the import graph has an edge between two areas that `reaches` lacks, or
  `reaches` lists an edge the graph does not have. The finding names a file
  and the import that makes the edge. An edge from or to an ordered area
  (`cmd/warden`'s imports) is not compared;
- a driver in `rules.yaml`'s `drivers.all` has no entry in
  `implies.drivers`, or `implies.shared` is not exactly the areas that hold
  a path of `rules.yaml`'s `drivers.shared`.

What the check cannot see is a dependency that is not an import: a script
that copies another area's file, or a test that reads one by path. The one
known today is placed by hand (`tools/relay-vm/`, above). Two more are
tests that walk the whole tree (`driver/buildlink/stray_test.go`) or read
the book (`specs/specs_test.go`); they are named under **Safety**.

A new edge between areas therefore cannot land without a map change, which
a feature can make only if its design review approved it. A feature that
finds in Implement that it needs one returns to Design (FLOW-14): a new
dependency between two areas changes which items its work is near.

### How two items relate

`relation(a, b)` is computed from the two queue files, the copy of the map
and nothing else. The first row that applies decides. "Has a repository in
common" compares the effective `repos` (an absent list is the queue's own
repository). "Undeclared" is an absent `areas`, or one that names an area
the copy of the map lacks.

| # | When | Relation | The reason `wf` prints |
|---|---|---|---|
| 1 | Either is a process item | outside | (none: a process item stops other work only through `holds`) |
| 2 | One names the other in `collides_with` | colliding | `named by the human` |
| 3 | Both list a seam that is not written | colliding | `seam <name>, which has no specification` |
| 4 | They have a repository in common that has no area map | colliding | `<repository> has no area map` |
| 5 | They have a repository in common and either is undeclared | colliding | `<item> declares no areas` |
| 6 | They share an exclusive area | colliding | `area <name>` |
| 7 | Both list a written seam | near | `seam <name>` |
| 8 | Both list the same rule in `rules_changed` | near | `rule <ID>` |
| 9 | An area of one reaches an area of the other, in either direction | near | `<area> reaches <area>` |
| 10 | Otherwise: no repository in common, or nothing shared but ordered areas, or nothing shared | disjoint | |

The function is total (row 10 takes whatever is left), symmetric, and reads
no clock and no git. Where a row could name several seams, rules or areas
it names the first in sorted order, so the text is stable. Rows 3, 7 and 8
do not ask about repositories: seams and rules count across them. A tracked
item's relations are computed by the same rows, and shown; the limits skip
it (a7).

What each relation means:

- **Disjoint**: "completely non-colliding". Neither counts against the
  other in any limit, and both PRs may be open. After one merges, the other
  is refreshed.
- **Near**: "nearly non-colliding". The two change different things that
  are linked: one's code depends on the other's, or both change the same
  rule, or both change a seam that has a specification to be read against.
  Neither counts against the other while they are built. They take turns at
  the PR, so the second is consolidated against the first as merged.
- **Colliding**: they change the same thing, or `wf` cannot tell that they
  do not. Today's FLOW-05 applies between them.

Every combination, with examples from this repository:

| Pair | Relation | Row |
|---|---|---|
| A Hyper-V item (`driver/hyperv`, `book`) and a Go relay release target (`relay`, `build`, `book`) | disjoint | 10 |
| A Hyper-V item (`driver/hyperv`) and a vz item (`driver/vz`), both with a book page | disjoint | 10 |
| A Hyper-V item and an item in `driver-core` (`driver/session.go`) | near | 9 |
| An item in `relaycfg` and any driver item or the relay | near | 9 |
| Two driver items that each also edit `cmd/warden/` (`cli`): a Hyper-V item and a vz item | disjoint (a5 (d)) | 10 |
| An item that edits only `cmd/warden/` (`cli`, `book`) and an item in `relaycfg`, `driver-core` or a driver | disjoint: `cli` is ordered and has no `reaches` | 10 |
| Two items that amend GLOBAL-08, in different areas (S2 and K1) | near | 8 |
| Two items that change `ledger-format`, in different areas | near | 7 |
| Two items that change `relay-claim`, which has no specification until S1 lands | colliding | 3 |
| An undeclared item and any feature here (`seam-specs` and `driver-contract` today, neither yet declared) | colliding | 5 |
| `seam-specs` and `driver-contract` once a human `wf declare` has recorded their seams: both list `relay-claim` and `collector` | colliding, whatever areas they are given, until S1 lists those seams' contract documents | 3 |
| An item that changes a tier-1 check (`tier1`) or adds a `rules.yaml` entry (`tier1-config`) and an item whose code the check runs over | disjoint (a15 (a)) | 10 |
| An item whose design adds the area it declares, before its merge | colliding with every feature here | 5 |
| An item here and an item homed in the relay repository, no seam or rule in common | disjoint | 10 |
| K1 here and the relay's M1 (tracked, its seams recorded at intake with `wf new -seams`), both on seam 8, which is not written | colliding, shown and not enforced | 3 |
| Two items in a repository with no map (`buildwarden-specs`) | colliding | 4 |
| Any item and a process item (`loop-contract` and `seam-specs`) | outside | 1 |
| Two reconciliation items for two drivers (`driver/qemu`; `driver/vz`) | disjoint | 10 |
| A reconciliation item and a fix in the same driver tree | colliding | 6 |
| Two items the review told apart from what the map says | colliding | 2 |

### The limits

Three limits. A *counted* item is a feature that is not tracked. Each limit
is taken by one kind of claim and held by a state the item's own steps
show. An item's *set* for a limit is the item and the holders of that limit
that relate to it. **The predicate**, one for all three: no holder's set
numbers more than the limit.

| Limit | Taken by | Held while | A holder's set | At most |
|---|---|---|---|---|
| In progress | The claim that starts a queued feature (a cloud claim today) | The feature is in progress | It and the in-progress features that *collide* with it | `limits.in_progress`; 5 when unset |
| `alters_relationships` slot | The claim of a Plan step, by a feature that declares `true` | A Plan step of it is done or carries a claim, expired or not (as `alters-slot` built it) | It and the holders that *collide* with it | `limits.alters_relationships`; 1 when unset |
| PR slot | The claim of `f6-merge-main` | The feature is in Publish and its newest `f6-merge-main` step is done or carries a claim, expired or not; or it has a PR recorded and has not reached Close, whatever its phase | It and the holders that *collide with it or are near it* | 1 |

Changes from today, beyond counting by relation:

- A claimed `f6-merge-main` holds the slot. Today only a done one does.
- An item in Close holds nothing. Today it holds until Close is done.
- An item that returned from Publish *before* its PR was opened gives the
  slot up, as today, and holds it again from the claim of its new
  `f6-merge-main`, not from re-entry.
- An item that goes back to Implement or Design *with its PR open* keeps
  the slot until that PR merges (see **The refresh step**). Today it gives
  the slot up, and a related item may open a PR beside it.
- `limits.in_progress`: 5 when unset, an error below 1 (Decision 8).

**The guard.** One function, in a new `queue/limits.go`:

```go
// Guard returns why the worker may not claim the step now, or "" (FLOW-05).
func (s *State) Guard(f *Feature, st *Step, worker string) string
```

It walks one table of the three limits, which is the only place a limit
is written. For each limit this claim would take, where the item does not
already hold it, it adds the item to the holders in memory and evaluates
the predicate for the item and for each holder related to it. It refuses
when any of those sets would number more than the limit, and names the
item whose set is full. Only an item that does not hold is asked, and only
holders are counted, so no two items wait for each other. Its reasons:

- `the in-progress limit (limit 5): it collides with a, b, c, d, e (FLOW-05)`,
  and, when the full set is a holder's,
  `the in-progress limit (limit 5): h, which collides with it, is already with a, b, c, d (FLOW-05)`;
- `an alters_relationships slot (limit 3), held by a, b, c (FLOW-05)`, the
  text `alters-slot` gave it;
  `an alters_relationships slot (limit 3): h, which collides with it, is already with a, b (FLOW-05)`;
  and `limits.alters_relationships 0 is below 1 (FLOW-05)` as before;
- `the PR slot, held by a (colliding: area driver-core) (FLOW-05)`, or
  `(near: rule GLOBAL-08)`;
- `the PR slot: a (priority 12) is ahead (FLOW-05)`, the tie-break below.

Why the holders' sets are evaluated as well as the asker's: the relations
are not transitive. H declares `driver-core`, `relay` and `collector`; P1
declares `relay`, P2 `collector` and X `driver-core`; all four alter
relationships, and the limit is 3. H collides with each of the others, and
the others are near one another. With H, P1 and P2 holding, X's own set is
X and H, which is 2. But H's set would be H, P1, P2 and X, which is 4. The
guard refuses X (case 19).

Who calls it:

- `wf next` and `wf next -feature`, in `ready`, for every worker. The two
  refusals that stay the cloud's alone (an open blocking question, a spent
  runaway budget) remain in `cloudHold`; they are FLOW-07's and FLOW-13's,
  not limits. `atLimit`, `prSlotHolder` and the slot test in `cloudHold`
  go.
- `wf claim`, in `claimable`, for every worker. A step the worker already
  holds stays claimable.
- `wf commit`, when it asks a claim again after a rejected push (below).

A human claim takes none of the three today: a human step starts no
feature, and Plan and `f6-merge-main` are cloud steps. A host claim is now
bounded: a Plan step whose affinity was edited to a host is refused past
the limit, which closes `alters-slot`'s known gap.

**`wf validate`** evaluates the same predicate, from the same table, for
every holder of each limit. Its errors name the limit, the item and the
others in its set, for example
`limit: seam-specs holds the PR slot with driver-contract (colliding: driver-contract declares no areas), limit 1 (FLOW-05)`.
Each begins with `limit:`. For a queue with no declared areas this is
today's count, and `alters-slot`'s error text is kept, after that prefix,
for the `alters_relationships` limit. Because the guard evaluates the predicate on
the state the claim would produce, a claim it allows leaves `wf validate`
clean, which is the property `alters-slot` stated for `Next` and
`Validate`.

**The tie-break at the PR slot** (a12). An item *waits for the slot* when
it is a counted feature in Publish whose newest `f6-merge-main` is pending
with no claim, it has no PR recorded, and it has no open blocking question
and no spent runaway budget. The guard refuses an item the slot while an
item that collides with it or is near it, and has a higher priority, waits
for the slot. Priority decides because both are in Publish, so both are in
FLOW-05's class 1. Priority is strict, so the waits form no cycle: the
highest-priority waiting item of any group is refused only by a holder,
and a holder waits only for Jeff's merge. The cost is that an item can
wait for a holder it is disjoint from, when the item ahead of it is
related to that holder (case 21).

**More holders than a limit.** The causes `alters-slot` listed remain
(Jeff lowers a limit; a hand edit). Two new ones are closed at the source:
`wf declare` and `wf relate` refuse a change that adds a limit error. A
third is not: a change to the map can relate two items that both already
hold. Its design review shows it against the open queue
(`wf validate -areas`), which is a view of that day's queue; when the
merged map is synced, `wf areas -sync` writes it, prints the limit and the
holders, and `QUEUE.md`'s **Needs Jeff** lists
`over a limit: a and b both hold the PR slot (near: specs reaches relay)`
until one of them merges. As with a lowered limit, no holder is stopped
(an item that holds is never refused), no related item is given the limit
meanwhile, and `wf validate` reports the `limit:` line on every run. The
tick does not stop on it: a `limit:` error that `wf validate` already
printed at the run's set-up is not the step's doing, and the run lists it
under what waits on Jeff and carries on (`workflow-tick`, step 5). A new
`reaches` edge can only make two items near, which the first two limits do
not count.

**The cases.** `limits.in_progress` 5 and `limits.alters_relationships` 3,
as in `config.yaml` today. "Today" is `wf` at `897a4a8`.

| # | State | Today | After |
|---|---|---|---|
| 1 | Live now (queue state at `0d5e755`): `loop-contract` at its merge with PR #119 open, and `loop-parallel` in Design (both process); `seam-specs` and `driver-contract` past Design, reviews pending, never declared | `wf next -worker cloud`: `blocked loop-contract waiting on the human's merge of PR #119`, and nothing for the cloud. `wf validate`: ok | The same lines. One relation: the two features collide (`driver-contract declares no areas`). `loop-contract` is a process item and holds no slot |
| 2 | Both features approved, never declared | `seam-specs` plans, then `driver-contract`: 2 of 3 slots | The same. Their Plan gates pass as today: with no `declared`, nothing is compared (case 24) |
| 3 | `seam-specs` has claimed `f6-merge-main`; `driver-contract` reaches its own | Offered: only a done `f6-merge-main` holds | Refused: `the PR slot, held by seam-specs` |
| 4 | `seam-specs` merged and in Close; `driver-contract` at `f6-merge-main` | Refused until Close is done | Offered |
| 5 | A host claims a Plan step past the limit (affinity edited) | The claim stands; the next `wf validate` fails | `wf claim` refuses it |
| 6 | Wave 2: S3, S4 and K1 declare `driver-core`; S2 declares `relay`, `executor`, `rules`, `book`; all four amend GLOBAL-08 | Four altering features, three slots: the fourth waits at Plan. One PR at a time | S3, S4 and K1 collide and hold the three slots. S2 is near each (row 8) and is in none of their sets: it plans too. All four take turns at the PR |
| 7 | Wave 4: K3 (`driver/hyperv`, `build`, `book`) and an item in the Go relay (`relay`, `book`) | One PR at a time | Disjoint: both PRs open; either merges first; the other is refreshed |
| 8 | Wave 5: five reconciliation items, one per driver (`driver/<name>`, `book`) | Five features in progress, the limit; one PR at a time; ranked as new functionality | Class 3. Pairwise disjoint: none is in another's set, and five PRs may be open |
| 9 | Five undeclared features in progress; a sixth queued | Not started | Not started: all five collide with it |
| 10 | Five declared features in progress, each disjoint from a sixth | Not started | Started |
| 11 | Two workers claim Plan for two colliding items at the last slot | The second push is rejected and its claim lost | The second push is rejected; `wf commit` asks the claim again on the new state; the guard refuses it and the reason names the holders |
| 12 | Two workers claim unrelated steps at the same moment | The second loses its claim and asks again | The second's claim is asked again on the new state and stands |
| 13 | Two near items wait at `f6-merge-main`; an attended run names the lower priority | It takes the slot | Refused: the other `is ahead` |
| 14 | An item whose approved design has `drivers: [hyperv, vz]` and `areas: [driver/hyperv]` widens to `driver/vz` in Review, while a vz item holds the PR slot | n/a | `wf declare` accepts: the item holds no PR slot yet, and it will wait for one. (A widening to `cli` through `surfaces` relates the item to nothing: `cli` is ordered) |
| 15 | The same item with its PR open finds, at its refresh, a path in `driver/vz`; another open PR declares `driver/vz` | n/a | `wf declare` refuses: two colliding items would hold the PR slot. Back to Design, keeping the slot; its PR becomes a draft (case 20) |
| 16 | `limits.in_progress` is not set | No limit | 5 |
| 17 | A process item in any state | Outside the limits | Outside |
| 18 | Three disjoint PRs open; Jeff merges one | Cannot arise | The next `wf stale` seeds a refresh on the other two; both then show "behind `main`" |
| 19 | H (`driver-core`, `relay`, `collector`), P1 (`relay`) and P2 (`collector`) hold the three slots; X (`driver-core`) reaches Plan | Refused: three holders | Refused: `h, which collides with it, is already with p1, p2`. X's own set is 2; H's would be 4 |
| 20 | An item with an open PR is sent back to Implement by its refresh; a near item reaches `f6-merge-main` | Offered: an item before Publish holds nothing | Refused: the returned item still has its PR recorded, so it holds the slot. Its PR is a draft and **Needs Jeff** says not to merge it |
| 21 | An item in `driver-core` (priority 1) and a VZ item (priority 3) wait while a Hyper-V item holds the slot | One PR at a time | The `driver-core` item is refused by the holder (near). The VZ item, disjoint from the holder, is refused too: `the PR slot: <driver-core item> (priority 1) is ahead`. When Jeff merges the Hyper-V PR the `driver-core` item goes next |
| 22 | Jeff merges a PR that is behind `main`; the earlier merge edited a line next to one of its hunks; the branch's ref still exists in the checkout | Cannot arise with one PR open | Confirmed: the changes are equal without context lines (a14). With three lines the confirmation fails and the item keeps the slot |
| 23 | A regression's fix is filed with nobody there, following an item that declared `driver/hyperv`, `book` | No areas exist: one PR at a time for every feature | It takes `driver/hyperv`, `book`, and waits only for items related to those. Filed with no areas it would collide with every feature and open its PR only when no other feature has one open (a4 (b)) |
| 24 | After this item's merge, `seam-specs` (never declared; its design lists five seams and four rule changes the queue file lacks) reaches its Plan gate | Passes on the approval | Passes on the approval: no `declared`, so the queue is not compared with the design, and the check prints `not declared: finished Design before wf declare` |

### `wf commit`: pushing queue state

`wf commit -state DIR -m MESSAGE [-claimed FEATURE/STEP -worker W [-new-run]] [PATH...]`
replaces the `git commit` and `git push origin workflow` in the four
skills.

1. It regenerates `QUEUE.md` and the digest pages (below), stages the
   paths it was given and the generated files, and commits. It stages
   nothing else: a second session may have work of its own in the same
   checkout. With nothing to commit it exits 0.
2. It pushes to `origin workflow`. On success it exits 0.
3. On a rejected push it fetches `origin/workflow`. If the checkout has
   uncommitted changes other than its own commit's, it stops with exit 5
   and names the files: a reset would discard them. Otherwise it resets
   the checkout to `origin/workflow` and brings its change onto the new
   state:
   - *Without `-claimed`* (a finished or released step, an answer, a
     digest, a status file): it replays its commit with
     `git cherry-pick --no-commit`, reloads the state and runs `Validate`.
   - *With `-claimed`*: it replays nothing. It asks the claim again on the
     new state, through what `wf claim` checks (`claimable`: the step is
     pending, its phase is reached, no one else holds it, no process item
     holds the worker, and `Guard`), and, for a cloud worker, the two
     refusals `wf next` adds (`cloudHold`: an open blocking question, a
     spent runaway budget). If none refuses, it claims the step again with
     `Claim`, which counts the step once, since the first claim's commit is
     gone.

   A conflict in a *generated* file (`QUEUE.md`, `digest/<date>.md`,
   `areas.yaml`, `main.yaml`) is settled by taking the remote's copy,
   since step 4 writes it again.
4. It regenerates the generated files, commits with the same message, and
   pushes again. It tries three times.

What ends it early:

| Outcome | Exit | With `-claimed` | Without |
|---|---|---|---|
| The claim is refused on the new state | 4 | The checkout is left at `origin/workflow`, and the message is the refusal: the guard's reason, the tie-break's, `held by process item <x>`, `blocking question <id>`, or `leased to <worker>`. The worker asks `wf next` again | |
| The replay conflicts in a queue file or any other file that is not generated | 5 | | The session's commit is restored, and the message names the files. The session re-applies its change by hand, as today |
| `Validate` fails on the replayed state with an error the remote's state did not have | 5 | | The session's commit is restored, with the lines |
| Three rejections | 5 | The claim is dropped | The commit is kept locally |

A claim is dropped, and never the other worker's: the state on the remote
is the one the claim is asked against, and the claim is the only change.
`wf commit` rewrites only its own unpushed commit and never force-pushes.
It pushes only the `workflow` branch.

### When collision is decided, and checked again

| When | What is checked | By | On failure |
|---|---|---|---|
| Intake | Jeff agrees the repositories and areas. `wf new` records them and `wf relations -feature F` shows what the new item is near or collides with | `workflow-intake`, with Jeff. A follow-up filed with nobody there (FLOW-09) takes the repositories and areas of the item it follows (`wf new -follows`) | Nothing fails: the areas are a proposal until Design |
| Design's done-when | (1) The queue has `declared`, and its `repos`, `areas`, `seams`, `rules_changed`, `alters_relationships` and `design_review` equal the design's (`wf declare` made them so). (2) Each area exists in the branch's map. (3) *Implied areas*, below. An undeclared design skips 2 and 3 | `wf done f1-design` (`check/declared.go`) | The step is released and Design fixes the design |
| The design review | Jeff sees the repositories, the areas and the relations with every open item, and may narrow, widen or name a collision | `workflow-human`; FLOW-19 | Changes, as for any review |
| Plan's done-when | For an item with `declared`: the queue still equals the design (closing the gap `alters-slot` named). `repos` and `areas` are impact fields: a design whose areas changed since its approval fails the Plan gate. An item with no `declared` is not compared, and the check says so | `check.go`, `review.go` | Back to Design (FLOW-06) |
| Implement, Review, Publish and the refresh, at each done-when | For an item with `declared`: the queue still equals the design. *The diff stays inside the areas*: every path of `git diff --name-only --no-renames origin/main...<branch>` belongs, by the branch's map, to a declared area, or is the item's own design. An undeclared item passes | `check/areas.go` | See below |
| Review and `f6-consolidate` | *A trial merge*: `git merge-tree --write-tree` of the branch with the remote branch of each in-progress feature that is disjoint from it. It is a report: clean; or conflicts in ordered files, which the later of the two settles at its refresh; or a conflict in an exclusive area, which means one of the two is outside its areas | `check/areas.go`; the lines go into the tier-2 report and the consolidation section | It does not fail the step: an item outside its areas fails its own diff check. The run's digest lists a conflict in an exclusive area under what is blocked (LOOP-06) |

**Implied areas** (Design's done-when, part 3). `areas` name what the
change edits (FLOW-02), so the check asks only that each other declaration
has somewhere to land:

- for each driver in `drivers`: its own area is declared, or a shared area
  is (`implies.shared`). A change to a shared package declares every
  driver and no driver's area (FLOW-02; ORCH-01's `drivers-claim` check),
  so
  `relay-leaf-crl`'s front matter, `drivers: [container, qemu, vz,
  hyperv]` with `relay/` alone changed, passes with `areas: [relay, book]`.
  The first draft asked for the driver's area or an area it reaches; no
  driver reaches `relay`, so that design, the relay half of Jeff's own
  example, failed it;
- for `interface` in `drivers`: a shared area is declared, or every
  driver's area is;
- for each surface in `surfaces`: its areas are declared;
- each path of a `rule_changes` entry lies in a declared area.

**When the diff leaves the areas**, the agent that finds it does one of two
things, and `wf declare` decides which is allowed:

1. *Widen inside the approved design.* The missing area is one the design,
   as approved, already implies (**Within the approved design**, under
   `wf declare`). The agent adds it to the design's `areas` on the branch
   and runs `wf declare`. Accepted, the item goes on; its relations may
   have changed, and `QUEUE.md` and the digest show them.
2. *Back to Design* (FLOW-06), when `wf declare` refuses: the area is
   outside the approved design, or the widening would put a limit over.
   The finding is the path and the area. The redesign's areas differ from
   the approved ones, so its review cannot be skipped (FLOW-19).

Narrowing is never an agent's: only a `human` worker's `wf declare` removes
an area or a repository after Design.

**What a missed area costs when nothing above catches it** is under
**Safety**.

### The refresh step

**What it is.** A step named `f6-refresh` (`-r2`, `-r3`, ... as needed) on
a feature that waits for Jeff's merge, and `x2-refresh` on a process item
that does. Affinity `cloud`; the agent is `wf-publish`. It is not a
template step: nothing seeds it at a phase's start.

**When it is seeded.** `wf stale -state DIR -repo DIR -worker W` looks at
every open item homed in this repository that has a PR recorded. For each
it first runs the merge check `wf done` runs on a merge step
(`check.merged`), because Jeff may have merged since the last
confirmation, and GitHub deletes the branch when he does:

- *Merged, and the item is at its merge step.* It seeds nothing and prints
  `merged: <item>`. The tick has confirmed merges just before, so this
  arises in an attended session, whose worker is `human` and cannot
  complete a `cloud` step (`wf done` refuses it); the next tick confirms
  the merge and moves the item to Close.
- *Merged, and the item is before its merge step* (it was sent back with
  its PR open, and Jeff merged the draft all the same). The merge is the
  sign-off and supersedes the phases (FLOW-08): `wf stale` drops the
  item's pending steps before Close, moves it to Close, and adds a `human`
  step `f8-escalate-returned` whose note is the finding that sent it back,
  for Jeff to file as a follow-up or drop.
- *Not merged, and the remote branch is gone* (the PR was closed). It
  seeds nothing and prints that.
- *Not merged, `origin/main` is not contained in `origin/<branch>`, the
  item waits for the merge, and it has no pending refresh step.* It adds
  one, sets `pr.ci` to `pending`, and writes what moved in the step's
  note: `main` at `<sha>`, the number of commits, and the files those
  commits and this branch both change.

The tick runs it right after confirming merges, every run;
`workflow-human` runs it before it lists the open PRs, and again after
each merge Jeff makes in that session. It is seeded whatever moved
`main`: another item's merge, a PR merged by hand, the weekly tidy.

**Where it ranks.** An item with a pending refresh step is in
implementation (FLOW-05's class 1), so a run refreshes open PRs before it
starts anything new.

**What it runs.**

1. Merge `origin/main` into the branch: a merge commit, conflict-only
   fixes, never a rebase (FLOW-17).
2. `make lint`, `make test`, `make deadcode` (FLOW-15).
3. The consolidation pass over *what landed since the branch last merged
   `main`* (FLOW-10), and the sweep (FLOW-15), written as a dated entry in
   the design's Consolidation section that names the `main` commit merged.
   The step's note says which files overlap; with none, the entry says what
   landed and that nothing overlaps. A failure of step 2 that `main` has
   too is not this branch's work: it is filed as the item's follow-up
   (FLOW-09), unless a fix for the red `main` is already filed
   (`main.yaml`), and the step is released.
4. Refresh the Archive summary if the PR's content changed (FLOW-12); run
   `flow18-check.sh`.
5. *Before the push*, fetch and check that the PR is still open and
   unmerged (`gh api repos/<repo>/pulls/<n>`) and that `origin/<branch>`
   is still the commit the merge in step 1 was made on. If the PR merged
   or the branch is gone, Jeff merged while the refresh ran: the agent
   discards its local merge, pushes nothing, and returns
   `merged meanwhile`; the tick's merge confirmation then moves the item
   on and drops the step. A push here would bring a deleted branch back,
   with commits `main` does not have, and Close would escalate it
   (FLOW-16).
6. Push; wait for CI on the new head and set `pr.ci`.

**Its done-when.** Current `main` merged (`origin/main` is an ancestor of
the branch); the branch pushed, which is new for a feature (the local
head equals `origin/<branch>`, the check a process item's Implement has
today; Publish's own done-when reads only the local branch); the
Consolidation section names the merged `main` commit; the rule-change
clearance; the design archived on the branch; the PR's CI green; the diff
inside the areas; the queue equal to the design. For a process item:
`main` merged, the branch pushed, the design archived, CI green. If `main`
moved again meanwhile, the check fails, the step is released, and the next
run takes it again.

**What sends the item back.** The pass in step 3 is consolidation, and
FLOW-10 already says what it does with real work: to Implement
(`wf back -to 3`) for work the plan covers, such as a lint failure a merged
check now reports on this branch's code, or to Design (`wf back -to 1`).
A path outside the areas goes through `wf declare`, as above. The refresh
never re-enters Publish, so the item keeps its PR slot. A process item has
no plan, Review or Publish to go back to (`wf back -to 3` is a feature's):
its refresh does the work its gates need on the branch, as its Implement
would, or returns it to Design with `wf back` when its design must
change.

**An item that goes back with its PR open keeps the PR slot**, since the
slot is held while a PR is recorded. Three things keep that PR from being
mistaken for one that is ready:

- the agent that returns the item converts the PR to a draft before it
  returns (in a cloud session through the session's `convert_to_draft`
  route for the pull request, elsewhere `gh pr ready --undo`). Neither
  merges, approves or closes anything (FLOW-08);
- `QUEUE.md` moves it from **Merges, in order** to the escalations, as
  `open PR #<n>, returned to Implement: do not merge`;
- when the item is published again, `f6-open-pr` finds the PR recorded,
  pushes to it and marks it ready, and opens no second one.

If Jeff closes the PR instead, an attended session removes `pr` from the
queue file on his word, and the item then holds nothing.

**Native validation.** A refresh does not queue a done `f6-validate-<host>`
step again (a10 (e)). The validation stands for the branch as it was
consolidated.

**When the step goes away.** `wf done` on the merge step drops a pending
refresh step (the PR merged first), and so does `wf back` (Publish merges
`main` again). FLOW-06 gains the clause.

**Budget.** A refresh is a claim like any other and counts one step
against the item's budget (a10). Intake's size guide adds two steps.

### `main`'s CI

Nothing reads it today. Under Decision 21 it is the gate for a PR merged
behind `main`, so the loop reads it (Decision 22).

- **What is recorded.** `wf main -state DIR -sha SHA -ci green|red|pending [-run URL] [-followup SLUG]`
  writes `main.yaml` in the queue state: the head of `main` the session
  saw, the result of its check runs, a failing run's URL, `red_since` (the
  first red commit after the last green one, kept while `main` stays
  red), and the follow-up filed for it. `wf` reads no CI itself: the
  session passes what `gh api repos/<repo>/commits/<sha>/check-runs`
  returned, as `wf-publish` passes `pr.ci`. `green` is recorded only when
  every check run on the head succeeded.
- **Who records it.** The tick, right after confirming merges and before
  `wf stale`. `workflow-human`, before it lists the open PRs.
- **What a red `main` does.** `QUEUE.md` opens **Needs Jeff** with
  `main is red at <sha> since <red_since>: <run>`, and names the follow-up
  once one is filed. The tick files the fix once, when `main.yaml` has no
  `followup`: by the intake skill's follow-up path, as the follow-up of
  the item whose pull request the `red_since` commit merged (its subject
  carries the PR number, which a queue file records), at the top of the
  fixes, with that item's repositories and areas (a16, FLOW-09). When the
  commit is no item's, it files nothing. A failure that does not
  reproduce is a flaky test and goes the way TEST-04 says, in the fix's
  Design.
- **What it does not do.** It blocks no claim and no merge. A red `main`
  fails each open PR's refresh at its gates; those steps are released, and
  taken again once `main` is green.

### Tracked items

An item whose home is another repository, such as a task of the relay's M1,
cannot be gated: `wf` sees no branch, design or PR there until
`loop-repository`. It is filed so that it shows in the one queue.

- `wf new -repos buildwarden/buildwarden-relay ...` seeds the `tracked`
  round, step letter `t`, whose two phases are Work and Merge, one step
  each: `t1-work` (human: the design, the work and the review, by Jeff or
  an attended session, until its PR is open) and `t2-merge` (human: Jeff
  merged it). `wf done` runs no check for either
  and prints `tracked: wf runs no gate in <repository> (FLOW-03)`. After
  `t2-merge` the item is done.
- `branch` and `pr` mean the home repository's. `QUEUE.md` prints the PR as
  `buildwarden/buildwarden-relay#12`. The session that does the work
  records `pr.number` in the queue file, as `wf-publish` does for a
  feature.
- Any claim starts it, as for a process item, since its steps are all
  human.
- It is a feature for FLOW-05's order: class 3 with `follows` or `adopts`,
  otherwise class 4. It is never "in implementation".
- It neither holds nor waits for a limit (a7). Its relations are computed
  and shown. `wf` cannot read its design, so `wf declare` does not apply to
  it: its `areas` mean nothing (its repository has no map here), and its
  `seams` are what intake records with `wf new -seams`.

An item homed here that also works in a second repository (`seam-specs`:
this repository and `buildwarden-specs`) is not tracked. Its gates run here
as today. It lists both repositories in `repos`, so `QUEUE.md`, its digest
lines and every claim show them (LOOP-04, LOOP-05), and it collides with
any other item that names the second repository, which has no map (row 4).
Work there is steps its design adds; a merge there is recorded by a `human`
step, which FLOW-04 and FLOW-08 now provide for (a6).

### Reconciliation items

`wf new -adopts <consumable>@<version>` writes `adopts`. The item is a
feature with the feature phases (LOOP-07); `Class` returns 3 for it.

- **What it waits for.** The release. When the release exists at intake,
  nothing: it starts in class-3 order. When it does not, intake files it
  with `wf new -hold "waits for <consumable> <version>"` (`loop-contract`'s
  a11), which writes a blocking question naming the release (LOOP-04). Jeff,
  or an attended session on his word, answers it once the release exists.
  `wf` cannot see another repository's tags, so it does not answer it
  itself. `follows` may be set as well, with its usual meaning.
- **Its place.** Intake proposes the priority just below the open fixes:
  one past the highest number held by an open class-3 item with `follows`;
  with none, the lowest number held by an open class-4 item. Jeff may place
  it elsewhere. A regression's fix still goes to the top (FLOW-09).
- **Its relations** are the ordinary ones, from its areas: one per driver
  in M6 are pairwise disjoint (case 8).

### What Jeff sees

**`QUEUE.md`** stays a function of the queue state, with no clock and no
git. For the queue as it is at `0d5e755` on the `workflow` branch
(2026-11-14 21:34 UTC), with `config.yaml`'s `repository` set, the map
synced, `main`'s CI recorded and no item yet declared, it renders as:

    # Queue

    Generated by `wf` from `queue/*.yaml`, `areas.yaml` and `main.yaml`; do not edit. Updated 2026-11-14 21:34 UTC.
    Areas: `buildwarden/buildwarden` at `897a4a8`. `main`: CI green at `897a4a8`.

    ## Needs Jeff

    6 items wait on you: 1 merge, 5 design reviews. No question or escalation.

    ### Merges, in order

    - **loop-contract**: [PR #119](https://github.com/buildwarden/buildwarden/pull/119), CI green

    ### Reviews, questions and escalations

    - **loop-parallel**: design review (`x1-design`, FLOW-19) · [design](https://github.com/buildwarden/buildwarden/blob/feat/loop-parallel/docs/design/loop-parallel.md)
    - **loop-native-lead**: design review (`x1-design`, FLOW-19)
    - **loop-repository**: design review (`x1-design`, FLOW-19)
    - **seam-specs**: design review (`f2-design-review`, FLOW-19) · [design](https://github.com/buildwarden/buildwarden/blob/feat/seam-specs/docs/design/seam-specs.md)
    - **driver-contract**: design review (`f2-design-review`, FLOW-19) · [design](https://github.com/buildwarden/buildwarden/blob/feat/driver-contract/docs/design/driver-contract.md)

    ## Active

    | Pri | Item | Kind | Repositories | Areas | Phase | Next steps | Waits for | PR | Steps |
    |---|---|---|---|---|---|---|---|---|---|
    | 60 | loop-contract | process | buildwarden/buildwarden | - | 3 merge | x3-merge (cloud) | your merge | [#119](https://github.com/buildwarden/buildwarden/pull/119) | 2 / 8 |
    | 61 | loop-parallel | process | buildwarden/buildwarden | - | 1 design | x1-design (human, leased to human/jeff) | - | - | 1 / 12 |
    | 64 | seam-specs | feature | buildwarden/buildwarden | undeclared | 2 plan | f2-design-review (human) | - | - | 2 / 30 |
    | 65 | driver-contract | feature | buildwarden/buildwarden | undeclared | 2 plan | f2-design-review (human, leased to human/jeff) | - | - | 5 / 30 |

    ## Relations

    Open features that are not disjoint. Process items are outside the relations (FLOW-05).

    | Items | Relation | Why |
    |---|---|---|
    | seam-specs, driver-contract | colliding | driver-contract declares no areas |

    ## Waiting on a host

    - **driver-contract**: `f6-validate-macos-vz` on macos-vz (from phase 6)
    - ... (unchanged)

    ## Backlog

    | Pri | Item | Kind | Repositories | Areas | Est. steps |
    |---|---|---|---|---|---|
    | 62 | loop-native-lead | process | buildwarden/buildwarden | - | 12 |
    | 63 | loop-repository | process | buildwarden/buildwarden | - | 10 |

`loop-contract`'s branch contains `origin/main`, so it is not behind.
Today's `QUEUE.md` for the same state has one **Needs Jeff** list of six
lines, and Active and Backlog tables with the columns Pri, Feature, Kind,
Phase, Next steps and "Steps spent / est.".

And for a later state, to show the parts that are empty today: case 7
after the Hyper-V pilot's PR has merged, with a relay task tracked:

    ## Needs Jeff

    3 items wait on you: 2 merges, 1 design review.

    ### Merges, in order

    - **relay-host-mode**: [PR #131](https://github.com/buildwarden/buildwarden/pull/131), CI pending. Behind `main`: refresh pending (`f6-refresh`); overlaps in `docs/book/src/guide/drivers.md`
    - **relay-m1a** (tracked): buildwarden/buildwarden-relay#12

    ### Reviews, questions and escalations

    - **adopt-relay-qemu**: design review (`f2-design-review`, FLOW-19) · [design](...)

    ## Active

    | Pri | Item | Kind | Repositories | Areas | Phase | Next steps | Waits for | PR | Steps |
    |---|---|---|---|---|---|---|---|---|---|
    | 70 | relay-host-mode | feature | buildwarden/buildwarden | relay, book | 7 merge | f6-refresh (cloud) | your merge | [#131](...) behind `main` | 16 / 18 |
    | 71 | driver-claim-conn | feature | buildwarden/buildwarden | driver-core, rules | 6 publish | f6-merge-main (cloud) | the PR slot, held by relay-host-mode (near: driver-core reaches relay) | - | 20 / 25 |
    | 72 | adopt-relay-qemu | adoption (buildwarden-relay v0.3.0) | buildwarden/buildwarden | driver/qemu, book | 2 plan | f2-design-review (human) | - | - | 3 / 12 |
    | 74 | relay-m1a | tracked | buildwarden/buildwarden-relay | - | 2 merge | t2-merge (human) | your merge | buildwarden/buildwarden-relay#12 | 2 / 2 |

What is new, by section:

- The header names the files it is built from, the commit of the map's
  copy, and `main`'s CI as last recorded.
- **Needs Jeff** opens with the count of items waiting on him, by kind
  (a3). Before the count it says `main is red at <sha> since <sha>`, with
  the run and the follow-up, when `main.yaml` says so, and lists any
  holders over a limit. It lists the merges first, in FLOW-05's order,
  each with its PR link, `pr.ci`, and "behind `main`" while a refresh
  step is pending, with the overlapping files from that step's note. The
  mark is as current as the last `wf stale` (a1). Reviews, questions and
  escalations follow, each review of an item in progress with a link to
  the design on its branch; an item sent back with its PR open is here,
  as `open PR #<n>, returned to Implement: do not merge`.
- **Active** gains Repositories, Areas (`undeclared`, or the names, `*`
  after one `main` lacks), the lease holder in Next steps, Waits for (the
  guard's reason for the item's next step, or "your merge"), and PR. Kind
  reads `adoption (<consumable> <version>)` for a reconciliation item and
  `tracked` for a tracked one. Two headers are renamed, in Active and in
  Backlog: "Feature" becomes "Item", since process items are listed too,
  and "Steps spent / est." becomes "Steps".
- **Relations** is new: every pair of open features that is near or
  colliding, with the reason. Disjoint pairs are not listed.
- **Backlog** gains Repositories and Areas.
- A **Workers** section (each machine's last run and its led items) is
  `loop-native-lead`'s.

`wf status` prints `repos:`, `areas:`, `declared:` and `adopts:` lines
under an item that has them. `wf relations [-feature F]` prints the relation rows.

**Digests.** Each run writes its own file,
`digest/<date>/<UTC start>-<worker>.md` (`20261114T105653Z-cloud.md`,
`20261115T030000Z-host-macbook.md`, `20261114T200000Z-human-jeff.md`),
with the sections it writes today, starting at its `## Run <W>` heading.
The date is the Pacific date of the run's start, as now. `wf digest`
builds the day's page, `digest/<date>.md`, from the directory, in file-name
order, under a "generated; do not edit" line; `wf commit` runs it. Two runs
no longer write the same file, and there is still one digest (LOOP-06): a
run's file is its section of the day's page. A page written by hand before
this change is moved into its directory as the first file the first time
`wf digest` meets it, so nothing is overwritten. Each step line in a run's
file names the item's repositories when it has more than this one.

### In the code

`tools/workflow/queue/`:

- `queue.go`: the seven fields on `Feature`, with `areas` a pointer to a
  list so that an absent key and an empty list stay apart through a load
  and a save (checked with the YAML library the module uses);
  `Config.Repository`; `State.Main`, loaded from `main.yaml` as an optional
  file;
  `Limits.InProgress` as `*int` with `inProgressLimit()`; `Validate`'s
  checks of the new fields, and its limit checks replaced by the
  predicate over the limits table, its errors prefixed `limit:`; `Class`
  returns 3 for `adopts`; `inImplementation` is true for an item with a
  pending refresh step.
- `areas.go` (new): the map's type, its parser (unknown keys rejected), the
  path match, and the copy's loader in `Load`.
- `relation.go` (new): `Relation(a, b)` and its reason.
- `limits.go` (new): the table, the predicate, `Guard`, the PR-slot
  holder and tie-break; `holdsAltersSlot`, `altersHolders` and `altersWait`
  move here from `next.go`.
- `next.go`: `Next`, `NextFor` and `ready` call `Guard` for every worker;
  `atLimit` and `prSlotHolder` go; `Claim` starts a tracked item.
- `phases.go`: the `tracked` round and the letter `t`; `IsRefresh`;
  `SeedRefresh`; `Complete` drops a pending refresh on a merge step; `Back`
  drops one; `Landed`, which moves an item whose PR merged while it was
  sent back to Close with its escalation step.
- `declare.go` (new): the rules of `wf declare` and `wf relate`.
- `commit.go` (new): `wf commit`, with git run on the state checkout as
  `hosts.go` already runs it on the code checkout; `MayClaim`, the checks
  `wf claim` and a claim asked again share.
- `digest.go` (new): the day's page.
- `summary.go`: the columns and sections above.

`tools/workflow/check/`:

- `declared.go` (new): reads a design's declarations, on the branch and at
  its approved commit; the comparison with the queue, for an item with
  `declared`; the implied areas.
- `areas.go` (new): the diff inside the areas; the trial merge.
- `check.go`: `Front` gains `Repos` and `Areas`; Design's done-when calls
  the declared checks; the later phases call the comparison and the diff
  check; `Refresh`; `Merged`, exported for `wf stale`; a tracked item's
  result; `patchID` passes `-U0` in place of `-U3` (a14).
- `review.go`: `impactDelta` includes `repos` and `areas`; a skip is
  invalid when Design narrowed.

`tools/workflow/cmd/wf/main.go`: `areas`, `declare`, `relate`, `relations`,
`stale`, `main`, `commit` and `digest`; `new -repos -areas -seams -adopts`,
and `-follows` copying `repos` and `areas`; `validate -areas`; `claimable`
calls `Guard`; `done` runs `check.Refresh` for a refresh step; `status`
prints the new lines.

`tools/tier1/`: `areas.yaml`, read from the checkout and not embedded;
`rules/rules.go` lists `area-map`; `rules.yaml` maps PROFILE-01 to it;
`coupling/areamap.go` is the check, and uses the parser in
`tools/workflow/queue`, which tier 1 already depends on through
`tools/workflow/check`.

### Machinery edits

By file. Each cites the rule and does not restate it (FLOW-14).

- `.claude/skills/workflow-tick/SKILL.md`. Step 1: `dist/wf areas -sync`
  after the fetch, and `dist/wf validate`, keeping any `limit:` lines.
  Step 2: after confirming merges, read `main`'s check runs and record
  them with `dist/wf main`; on a red `main` with no follow-up recorded,
  file one by the intake skill's follow-up path (FLOW-09) and record it;
  then `dist/wf stale`. Claim and state pushes go through
  `dist/wf commit` (exit 4: ask `wf next` again; exit 5: re-apply by
  hand); the dispatch table gains `f6-refresh` and `x2-refresh` for
  `wf-publish`; an agent that returns `merged meanwhile` ends its step
  with the merge confirmation, not `wf done`. Step 5: a `limit:` line that
  step 1 already printed does not fail the step; it goes under **Blocked
  on Jeff**. Step 4: the run's own digest file, then `dist/wf digest`; a
  red `main` and an item sent back with its PR open are listed under
  **Blocked on Jeff**.
- `.claude/skills/workflow-intake/SKILL.md`. Step 3: agree the
  repositories and areas with Jeff (`dist/wf areas -list`), show
  `dist/wf relations -feature <slug>` after `wf new`; `-adopts` and its
  place; a tracked item, with `-seams`; two steps on the size guide for
  refreshes. The follow-up path: no `-areas`, so `wf new -follows` takes
  the followed item's repositories and areas; a fix for a red `main`
  follows the item whose merge it was.
- `.claude/skills/workflow-human/SKILL.md`. Its worker stays
  `human/<name>`, which cannot complete a `cloud` merge step, so it does
  not confirm merges. The review's overview gains the repositories, the
  areas and the relations with open items, and `dist/wf validate -areas`
  for a map change; narrowing and `wf relate` on Jeff's word. **Open
  PRs** records `main`'s CI (`dist/wf main`), runs `dist/wf stale`, which
  skips a PR Jeff has already merged, and lists the merges in order with
  "behind `main`"; it runs both again after each merge he makes in the
  session, and tells him the next tick confirms it. **Escalations** gains
  `f8-escalate-returned`, a red `main`, and holders over a limit. A
  tracked item's steps.
- `.claude/skills/workflow-native/SKILL.md`. State pushes through
  `dist/wf commit`; the run's own digest file.
- `.claude/agents/wf-design.md`. Step 2: declare `repos` and `areas` in the
  front matter, starting from what intake recorded. Step 5: `wf declare`
  in place of the hand copy, and in place of `alters-slot`'s sentence:
  "Run `dist/wf declare` to copy the design's declarations into the queue.
  If it refuses because a limit would be exceeded, raise a blocking
  question that names the holders it printed (FLOW-07) and leave Design
  unfinished."
- `.claude/agents/wf-implement.md` and `wf-review.md`. A path outside the
  areas: widen with `wf declare` when the design covers it, otherwise
  `back-to-design` (FLOW-06). Review records the trial merge's lines in its
  report.
- `.claude/agents/wf-publish.md`. `f6-consolidate` records the trial
  merge; `f6-refresh` and `x2-refresh` as above, with the check before the
  push and the draft on a return; `f6-open-pr` on an item that has a PR
  recorded pushes to it and marks it ready.

`wf-plan` and `wf-guide` do not change.

### Holds, and what the merge does to running work

This item declares no `holds` (Decision 17, a13).

- **Before the merge** nothing changes for any session: the branch's `wf`,
  skills and rules are not on `main`, and the queue state gains nothing.
- **At the merge** `wf`, the skills, the agents and the rules change in one
  commit. A scheduled session rebuilds `dist/wf` from `main` as it starts
  and reads the skills from `main`; a session already running keeps what it
  started with until it ends (LOOP-09).
- **The state stays readable both ways.** Every new field is optional and
  absent means today's behaviour, so the new `wf` reads today's queue
  unchanged (Validation plan: the live queue through both). An old `wf`
  reads a queue file with new fields and ignores them. The one loss is an
  old `wf` *saving* such a file, which drops the fields. So the first new
  field is written at Close (the attended step `x4-state`), once no live
  lease was claimed before the merge commit's time, and attended
  sessions rebuild `dist/wf` before their next command, as after
  `alters-slot`.
- **Items already past Design.** `seam-specs` and `driver-contract`
  finished Design before the queue could record a declaration. Their
  queue files have no `seams`, no `rules_changed` and no `declared`, and
  their designs list five and four seams and four and ten rule changes.
  The new gates compare the queue with the design only for an item that
  has `declared`, so both pass their Plan, Implement, Review and Publish
  gates exactly as they do today, and the relation treats both as
  undeclared (row 5). A human `wf declare` at `x4-state` records what
  their designs declare; from then on their gates compare. Their designs
  have no `repos` or `areas` key. If a design gains one before its review
  approves it, the approval covers it. If it gains one after, that is a
  change to an impact field since the approval, and the Plan gate asks
  for a new `approved at <sha>` entry, which the attended session records
  on Jeff's word; once Plan is done nothing compares it, and a human
  `wf declare` accepts it.
- **An old skill beside a new one.** A session that started before the
  merge appends to `digest/<date>.md` by hand; the first `wf digest` moves
  that page into the day's directory and builds the page from it. A
  `tracked` step id fails an old `wf validate` loudly; none exists before
  Close.
- **`loop-contract` first.** This item amends files that `loop-contract`
  added, and thirteen of the files its PR changes are files PR #119
  changed too (`process.md`, `TEMPLATE.md`, the four skills,
  `wf-design.md`, `wf-publish.md`, `queue/queue.go` and its test,
  `cmd/wf/main.go`, and the two rule files it added). PR #119 merged on
  2026-11-15 and the branch has merged that `main` (`26a1512`), so the two
  PRs were never open against each other and this item's work starts from
  a `main` that has `loop-contract`.

### Safety

- **FLOW-08.** Nothing here merges, approves or enables auto-merge
  (LINT-03). More
  PRs are open at once and each is still Jeff's to merge. "Merges, in
  order" is the order FLOW-05 gives; he may merge in another. Making a
  returned item's PR a draft, and ready again when it is republished, is
  neither a merge nor an approval, and Jeff can undo either.
- **FLOW-16.** Close is per item and unchanged. A refresh adds commits to a
  branch that is still open, checks that it is still open before it
  pushes, and deletes nothing.
- **FLOW-17.** A refresh pushes a merge commit to the item's own branch:
  never a rebase, never a force-push, never `main`. `wf commit` pushes only
  `workflow`, rewrites only a commit it has not pushed, and never forces.
- **FLOW-18.** `wf commit` and `wf stale` touch no session recording; the
  skills run `flow18-check.sh` where they do today, and the refresh runs it
  before its push. A run's digest file carries what the run's section
  carries now.
- **`main` is unprotected today** (`gh api
  repos/buildwarden/buildwarden/branches/main/protection`: 404; no
  rulesets; `allow_auto_merge` true, `allow_update_branch` false,
  `delete_branch_on_merge` true; read on 2026-11-14). `loop-contract`'s a21
  has Jeff protect it before that PR merges. This item works either way.
  What it asks of the protection, under a1 (a): require a pull request and
  the PR's checks, and do *not* require the branch to be up to date, or a
  disjoint PR could not merge until its refresh.
- **What a missed collision costs, and where it is caught.** In order:
  1. Design's done-when, when the front matter implies an area the design
     left out. Cost: a fix to the design, before its review.
  2. The diff check at Implement, Review, Publish and the refresh, when a
     path is outside the areas. Cost: one `wf declare`, or a return to
     Design and a fresh review.
  3. The trial merge at Review and consolidation, which names the other
     item early.
  4. `make lint` and `make test` at `f6-merge-main`, `make deadcode` at
     `f6-consolidate`, and all three at each refresh, on the branch with
     `main` merged. This net holds only for a PR that is refreshed before
     it merges; under a1 (a) Jeff may merge first.
  5. GitHub, which refuses a merge that conflicts.
  6. CI on the push to `main`, which the tick and the attended session now
     read (Decision 22). A failure there is a regression on `main`: it
     opens **Needs Jeff**, and its fix is filed as the follow-up of the
     item whose merge it was and goes to the top of the fixes (FLOW-09 as
     amended, a16). Before this item nothing in the loop read that result.
  7. The full matrix before a release (TEST-02). CI runs neither Hyper-V
     nor VZ. The scheduled native session is not a net for them today:
     `wf gate` starts a session on a moved `main` only for a machine with
     an open backlog gap, and sweeps only those gaps
     (`queue/hosts.go`); no gap is open for `windows-hyperv` or
     `macos-vz`. And a refresh does not run an item's blocking native
     validation again (a10 (e)). So a break between two disjoint items
     that only a native host shows is found before a release, not before.

  What reaches 6 or 7 is a break between two items whose declarations
  were true: no shared exclusive area and no import between them. Three
  kinds are known. A semantic break at run time, such as a relay change
  and a driver that boots the relay. A whole-tree test in a code area:
  `driver/buildlink/stray_test.go` walks every file, so a literal one
  item forbids and another item adds are green apart and red together;
  `specs/specs_test.go` reads the book's `SUMMARY.md`. And a check one
  item tightens over code another item adds: no check's area reaches the
  code it runs over (a15 (a)), whether the check is tier 1's
  (`tier1`, `tier1-config`) or the root `.golangci.yml` (`build`).
- **A tightened check beside the code it would fail** (a15 (a)). The two
  items are disjoint and both PRs may be open. Whichever merges second
  has merged the first at its refresh (a1 (b)), and what happens there
  depends on what the check reads:
  - *The check item merged first.* The code item's refresh runs
    `make lint` with the tightened check over its own diff and fails. That
    is a lint failure a merged check now reports on the branch's code:
    the item goes back to Implement (FLOW-10), before its merge.
  - *The code item merged first, and the check reads the whole tree*
    (`golangci-lint`, a whole-tree test). The check item's refresh fails
    on code that is now on `main` and outside its areas. Fixing it there
    is a path outside its areas: `wf declare` refuses the widening unless
    the approved design covers it, and the item returns to Design
    (FLOW-06), before its merge.
  - *The code item merged first, and the check is diff-scoped* (tier 1's
    own checks, LINT-04, which report only what a branch changes). The
    check item's refresh is green, it merges, and nothing fails: the
    tightened check binds the other item's code when a later change
    next touches those lines, or at the next release, whose tier-1 run
    is over everything since the last tag (LINT-03, LINT-04). That is
    how every tier-1 check added so far has met code already on `main`.
  - *Either merged round the up-to-date requirement* (a1: a bypass, or a
    merge made before the setting exists). Then the two meet on `main`,
    as the answer has it: CI on the push fails where the check reads the
    whole tree, **Needs Jeff** opens with it, and the run that sees it
    files the fix (a16).
- **Two items in `cmd/warden/`** (a5 (d)). `cli` is ordered, so two items
  that edit the same CLI files are disjoint: both PRs may be open, and
  they merge one after the other, the second after its refresh has
  merged the first. Three things can then happen at that refresh. A
  textual conflict: the refresh resolves it when the fix is a conflict
  fix, and sends the item back to Implement when it is more (FLOW-10).
  A clean merge that does not build or fails a test (one item renames a
  function the other calls; both register a flag of the same name in an
  `init`, and the flag library panics when `cmd/warden`'s test binary
  starts): `make lint` and
  `make test` fail on the refreshed branch, before the merge. A clean
  merge that builds, passes and is wrong (two edits to the order of the
  same command's steps, with no test on the order): nothing catches it
  before the merge. The consolidation pass reads what landed in the
  files both changed, which the step's note lists; beyond that it rests
  on the tests `cmd/warden` has. This is the same exposure every ordered
  area has (`go.mod`, a CI workflow), now for code. The same holds for an
  item that edits only the CLI beside an item in a package the CLI
  imports: they were near while `cli` had its `reaches`, and are
  disjoint now. Where Jeff wants two such items in turn, he names the
  collision (`wf relate`).
- **Merge order.** A related item cannot open a PR while another's is
  recorded and unmerged, a returned item's included, so related PRs are
  never open together and order matters only among disjoint ones. There
  any order merges cleanly or is refused by GitHub, a PR that is behind
  `main` says so as of the last `wf stale`, and what a clean merge can
  still break is the list above.
- **The merge check.** A PR merged while behind `main` is confirmed by
  the same three proofs as today, the third without context lines (a14).
  An item whose merge cannot be confirmed stays at its merge step and
  keeps the PR slot until a human compares, as today. `wf stale` never
  seeds a refresh on a PR that has merged, and the refresh never pushes
  to one, so a deleted branch is not brought back (FLOW-16).

### What this item hands on

**To `loop-native-lead`.**

- The guard applies to every worker and to `wf claim`, so a machine that
  claims an item's first step, its Plan step or `f6-merge-main` is bounded
  by the same limits as the cloud. Its design adds `lead` and decides which
  steps a machine takes; `f6-refresh` is seeded `cloud` here and its design
  may leave it so.
- The PR-slot tie-break means a led item of lower priority waits for a
  cloud-led one that waits for the slot. Its design says whether that is
  wanted.
- `wf commit`, for a machine's state pushes, and a digest file per run, so
  a machine and the cloud no longer write one file.
- FLOW-04 is amended here in three clauses only: every claim is checked
  against FLOW-05's limits, whoever the worker is, which is the clause a
  machine's led claims rest on; a claim whose push is rejected is asked
  again on the new state; and the human step for a repository with no
  gate. Its host-name and lease clauses are untouched.
- `QUEUE.md`'s Workers section and a Lead column; a hold that stops led
  work.
- Not done here: `wf renew`. Renewing a lease is part of the 20-hour lease
  (`loop-contract` a24).

**To `loop-repository`.**

- `repos` is recorded and shown, and `config.yaml` names the home
  repository. Gates still run only here.
- The `tracked` round is the placeholder it replaces: an item homed
  elsewhere gets real phases, a branch and a PR per repository (`work`),
  and then counts in the limits (a7).
- One map per repository: the copy in the queue state becomes one file per
  repository, an item's `areas` gain a repository prefix, and
  `wf areas -sync`, `wf stale` and `wf main` run per repository.
  `wf areas -sync` hard-codes two paths of this repository that the
  profile should supply (its hand-off point 4): `tools/tier1/areas.yaml`,
  and `tools/tier1/rules.yaml`, of which it reads the `name` of each entry
  under `seams`.
- A hold that names repositories (Decision 18), and a `human` merge-recording step
  that is not offered before its phase (`seam-specs`' a15 asks; a `human`
  step is still offered early).
- What a tracked item of a private repository may show in the public
  queue, which is its a17.

### Departures from the proposal

Where this design does not do what the proposal's sections 3 to 8 say.

| The proposal | This design | Why |
|---|---|---|
| Sharing only ordered areas makes two items near: one PR at a time | They are disjoint; the refresh orders them (a2) | 36 of 253 historical pairs, Jeff's example among them, would take turns over a book page or a build file |
| "`tier1`, `rules` and `tier1-config` reach every area" | None of the three does: `reaches` is the import graph alone (a15 (a)) | Jeff's answer of 2026-11-15. An item that changes a check would otherwise take turns at the PR with every item; 22 of the 52 merges changed `docs/development/` |
| The exclusive areas include `relay`, with the relay VM image tools | `tools/relay-vm/` is in `driver-core` | Its `init` is read, written for and copied by driver code, which no import shows |
| A form for the Go relay "(`relay`, which reaches every driver) would be near" a Hyper-V item | Disjoint | The code: no driver tree imports `relay/`, and it imports none |
| `wf areas -check` compares the map with `go list` | Tier 1's `area-map` does; `wf areas -check` compares the copy with `main` | The loop's tool assumes no language (LOOP-08) |
| The map "on `main` under FLOW-14" | On `main`, with a generated copy in the queue state | `wf next` reads no git |
| A refresh that finds overlap "queues `f6-consolidate` again" | The refresh carries the pass itself | Re-entering Publish would give up the PR slot |
| A refresh "when a PR merges" | Whenever `main` is not contained in the branch | `main` also moves from outside the queue |
| A returned item's PR and the slot are not mentioned | An item keeps the PR slot while its PR is recorded, and a returned item's PR is a draft | Otherwise a related PR opens beside a stale one |
| The guard counts the holders that relate to the asker | No holder's set may exceed the limit, the asker's and each related holder's | The relations are not transitive; the guard and `wf validate` must agree |
| Nothing reads CI on `main`; nothing about the merge check | The tick records `main`'s CI and files a red `main`'s fix (FLOW-09); the patch-id proof drops its context lines (a14, a16) | Merging behind `main` relies on the first and trips the second |
| A follow-up is undeclared until its design | It takes the areas of the item it follows (a4) | Undeclared, the fix for a red `main` waits for every open PR |
| `wf check` fails when areas do not cover what `seams` imply | Seams are not mapped to areas; the relation reads them | A design may list a seam it does not edit the code of (`driver-contract` lists two "conservatively") |
| An agent may widen within the design's `drivers` and `seams` | Within the approved design's `surfaces` and `rule_changes` paths, and its `drivers` when it declares no shared area; only while no limit goes over | The same reason; a shared-package design lists every driver; and a widening can put a limit over |
| The trial merge fails the step | It is a report | The item at fault fails its own diff check |
| Seams count across repositories | They do, in the relation; a tracked item is shown and not counted (a7) | `wf` sees nothing of a tracked item |
| "A hold may name repositories" | Not yet (Decision 18) | No worker step exists there to hold |
| `wf renew` | Left to `loop-native-lead` | It belongs with the lease change |
| FLOW-17: "a `feat/<item>` branch in each" repository | Not here | `loop-repository` |
| The exclusive areas include `conformance` | Not in the map | `tools/conformance/` is not on `main`; `seam-specs` adds it |
| FLOW-02, -03, -05, -10, -15, -19, -20 | Also FLOW-04, -06, -08, -09, LOOP-01, LOOP-04, PROFILE-01 | Decision 19 |

### What this item does not do

- No `lead`, no host names, no change to the native wrapper, the push
  script or the lease: `loop-native-lead`.
- No gate, branch, PR or check in a second repository, and no `work` list:
  `loop-repository`.
- No second cloud run a day (the proposal's question 4), and no change to
  FLOW-13.
- No change to the freeze.
- No area for a tree that is not on `main`, and no stand-in for one: an
  item whose design adds the area it declares is undeclared, and collides
  with every feature here, until its merge puts the area on `main`. Two
  of the 52 merges added a tree. Relating such an item by where its new
  paths fall on `main`'s map would need a copy of part of the branch's
  map in the queue file; it is left out.
- No gain for the two features in the queue now. See below.
- No automatic answer to a reconciliation item's question.
- No re-run of native validation at a refresh (a10 (e)), and no map entry
  for a dependency that is not an import, beyond `tools/relay-vm/`.

### What changes for the queue as it stands

Nothing, at first, and Jeff should expect that.

- `seam-specs` and `driver-contract` collide today because neither is
  declared (row 5). Giving them areas does not change it: both designs
  list the seams `relay-claim` and `collector`, which have no
  specification until `seam-specs` itself merges and lists their contract
  documents in `rules.yaml` (row 3). `wf relations` then prints
  `seam-specs, driver-contract: colliding (seam collector, which has no specification)`.
- After S1 merges, the program's wave-2 items all amend GLOBAL-08, so
  they are near one another (row 8): they are built side by side, three
  of them holding the `alters_relationships` slots, and still open one PR
  at a time (case 6).
- K1 (`driver-contract`) rewrites `cmd/warden`. With `cli` exclusive it
  would have collided with every other item that edits the CLI. With
  `cli` ordered (a5 (d)) it does not on that account: it still collides
  with any item in `driver-core`, which it also rewrites, and is near
  every driver item through `driver-core`. An item that edits only the
  CLI, as `runtime-cli-project-path` may (it is in the queue now, and
  its source says it "touches cmd/warden's configuration, which
  driver-contract also changes"), is disjoint from K1 by the map once
  both are declared and share no unwritten seam. Jeff says at either
  item's review whether they go in turn (`wf relate`); if he does not,
  the later one meets the other at its refresh, and a refresh that
  lands on a rewritten `cmd/warden` is work for Implement, not a
  conflict fix (FLOW-10).
- The freeze stays, so only program items run. The first pair that may
  have PRs open together is in wave 4: a driver pilot beside an item in
  the Go relay (case 7). Wave 5's per-driver adoptions are the first
  large gain (case 8).
- The one lever for earlier gain is row 8's grain: "the same rule" could
  be read as the same bullet of GLOBAL-08, not the same rule ID. `wf` has
  no way to tell bullets apart, and two edits to one rule are what
  consolidation most often has to reconcile, so the row stays as it is.

What does change at once: the guard for every worker and for `wf claim`,
`wf commit`, the digest files, `QUEUE.md`'s new sections, `main`'s CI
being read, and the refresh for any process item's PR that waits while
`main` moves.

## Defaults and overrides

| Default, override or fallback | What it is | Why, and what overrides it |
|---|---|---|
| `repos` absent | The item works only in `config.yaml`'s `repository` | LOOP-04: "an item that names none works only in the repository that holds its design" (GLOBAL-09). A written list is used exclusively (GLOBAL-10) |
| `config.yaml`'s `repository` absent | No default. An item that names `repos` then fails `wf validate` | A guessed repository name would be a silent fallback (GLOBAL-10, GLOBAL-11) |
| `areas` absent | Undeclared: the item collides with every item it shares a repository with | Today's behaviour, and the safe side (GLOBAL-09). `areas: []` is a declaration, not an absence |
| *Named fallback: "undeclared until merged".* `areas` names an area `main`'s map lacks | The item is treated as undeclared | Its design adds the area and `main` has it only after the merge. `QUEUE.md` marks the name with `*` (GLOBAL-11) |
| *Named fallback: "no map".* The queue state has no `areas.yaml`, or a repository has none | The repository is one exclusive area: every two items in it collide | Today's behaviour (GLOBAL-09). `QUEUE.md`'s Relations gives it as the reason (GLOBAL-11) |
| `seams`, `rules_changed`, `collides_with`, `adopts` absent | None; not a reconciliation item | The obvious reading |
| `declared` absent | The item was never declared: no gate compares its queue file with its design, and the check prints that | An item that finished Design before `wf declare` existed must pass the gates it passes today (GLOBAL-09). Design's done-when requires it from this merge on, and a human `wf declare` sets it for an older item |
| `wf new -follows X` without `-areas` | The new item takes X's `repos` and `areas`; none when X has none | A fix or finish of X is in X's areas to a first approximation, and Jeff agreed those for X (a4). `-repos` and `-areas` override it, and the history line says which was used (GLOBAL-10, GLOBAL-11) |
| *Named fallback: "the design on the branch".* `wf declare` judging a widening when the design has no `approved at` entry (its review was skipped) | It reads `drivers`, `surfaces` and `rule_changes` from the branch | A skipped review has no approved commit, and the skip criteria allow no seam and no rule change. The history line says `(design on the branch: review skipped)` (GLOBAL-11) |
| `main.yaml` absent | `main`'s CI is not known: `QUEUE.md` says `main: CI not recorded`, and nothing is filed | No session has recorded it yet. Unknown is not read as green (GLOBAL-10) |
| The map's `implies.shared` absent | No area is shared: every driver in a design's `drivers` needs its own area declared | The obvious reading; the `area-map` check requires it to match `rules.yaml` in this repository |
| The merge check's context lines | 0, fixed. *Changed:* 3 | a14 |
| `limits.in_progress` absent | 5; a written value below 1 is an error | FLOW-05's written limit (Decision 8). *Changed:* absent or 0 was "no limit" |
| `limits.alters_relationships` absent | 1; below 1 an error | Unchanged (`alters-slot`) |
| The PR slot | 1, not configurable | FLOW-05 |
| A map area's `mode` | Required; no default | Exclusive by default would hide a missing line; ordered by default would be unsafe |
| A map area's `reaches`, `paths` absent | Reaches nothing; `paths` may be absent only for `catch_all`. `reaches` on an ordered area is an error, not ignored (a5 (d), GLOBAL-10) | The obvious reading |
| A design's `repos` empty or absent | Only the repository that holds the design | LOOP-04 |
| `wf new` without `-repos`, `-areas`, `-adopts` | Home repository; undeclared; not a reconciliation item | As the fields |
| `wf commit`'s tries | 3, fixed | Enough for a busy minute; then the session decides |
| `wf commit` with no paths | It stages only the generated files | Another session's uncommitted work in the same checkout is never swept in |
| A reconciliation item's place | Just below the open fixes; Jeff's word overrides | a8 |
| *Named fallback: "earlier page".* A hand-written `digest/<date>.md` when the day's directory first appears | Moved into the directory as its first file, once | So `wf digest` never overwrites a page it did not build (GLOBAL-11: the page's first section shows it) |

**Dropped fallbacks.** `limits.in_progress` read as no limit when absent or
0; a claim lost on every rejected push; the PR slot held through Close; the
PR slot given up by an item that goes back with its PR open; `interface`
as an entry of `implies.drivers` (the first draft), which let a
shared-package design pass only when it was spelled that way.

**Merges.** `wf areas -sync` unions `areas.yaml`'s `written_seams` with the
names in `rules.yaml`'s `seams` list. Neither overrides the other: a seam
in either is written, and the copy lists the result.

## Cross-driver impact

None: no driver, seam or user-facing surface changes. The area map names
each driver's tree; it describes them and changes nothing in them.

## Rule changes

Fourteen, each for the design review to approve by name (FLOW-14, FLOW-19).
"Current wording" is `main` at `ad4ea50`, which has `loop-contract`
(#119): `docs/development/process.md` for the FLOW rules,
`loop-contract.md` for LOOP-01 and LOOP-04, `loop-profile.md` for
PROFILE-01, and `docs/design/TEMPLATE.md`. Every quotation below was
compared with those files at that commit on 2026-11-15, after the branch
merged it: none differs from what the draft quoted from `main` at
`897a4a8` and from `feat/loop-contract` at `31028a5`. Where a rule has
several paragraphs, only the amended ones are quoted, each in full; the
rest of the rule is unchanged. No severity changes. One check type changes
(PROFILE-01).

`loop-contract` amends one other sentence of FLOW-03 (a feature may change
"the rules of a component it works in") and two of FLOW-14. None of the
paragraphs below is one of those.

### FLOW-02 (amend), `docs/development/process.md`, `docs/design/TEMPLATE.md`

**Why:** the relation between two items is computed from what their designs
declare, so the design has to declare where the change is made.

**Current wording.** The list of keys has no `repos` and no `areas`. The
two paragraphs after the list:

> Declarations are derived conservatively: a change to a shared package (`relay/`,
> `ledger/`, `collector/`, `driver/`) declares every dependent driver and host, per
> the TEST-02 caveat.
>
> The automated part diffs the declared `drivers` / `seams` / `surfaces` against the
> trees and files the change reaches; the agent part judges the security note.

**Proposed wording.** Two keys are added to the list, after `surfaces`:

> - `repos` — every repository the change works in, the one that holds the
>   design first, when there is more than that one (LOOP-04).
> - `areas` — the areas of the repository's area map whose files the change
>   edits (FLOW-05). A design that declares none is undeclared, and FLOW-05
>   then treats the item as colliding with every item in its repositories.

and the two paragraphs become:

> Declarations are derived conservatively: a change to a shared package (`relay/`,
> `ledger/`, `collector/`, `driver/`) declares every dependent driver and host, per
> the TEST-02 caveat. `areas` name what the change edits, not what depends on
> it: the area map says which area reaches which.
>
> The automated part diffs the declared `drivers` / `seams` / `surfaces` against the
> trees and files the change reaches. It checks the declared `areas` against
> what `drivers`, `surfaces` and the `rule_changes` paths imply, at Design,
> and against the paths the change edits, from Implement on (FLOW-03). The
> agent part judges the security note.

`TEMPLATE.md`'s front matter gains the two lines shown under **What an item
declares**.

**Severity and check:** MUST, `automated + agent-judgment`, before and
after. **Enforcement:** `tools/workflow/check/declared.go` (new) and
`check.go`: Design's done-when gains the comparison with the queue and the
implied areas. Tier 1's `design-front-matter` check is unchanged: neither
key is required.

### FLOW-03 (amend), `docs/development/process.md`

**Why:** the queue records more than a phase and a branch; two done-when
entries change; a tracked item has no done-when at all.

**Current wording**, the sentence before the phase table; rows 1 and 7 of
it; the paragraph after it; and row 3 of the process table:

> Each feature is a queue item whose state records its current phase and its
> branch. The phases are:

> | 1 | Design | Design doc complete per FLOW-02; no unanswered blocking question (FLOW-07); whether it needs a design review decided (FLOW-19). |

> | 7 | Merge | The human merged the PR (FLOW-08). The loop confirms it; there is no step for the human to claim. |

> A transition happens only when the phase's done-when checks pass mechanically
> (files present, sections filled, commands green), never on an agent's own report
> that it is done.

> | 3 | Merge | A human merged it into `main` (FLOW-08). |

**Proposed wording:**

> Each feature is a queue item whose state records its current phase, its
> branch, the repositories it works in and what its design declares
> (FLOW-02). The phases are:

> | 1 | Design | Design doc complete per FLOW-02; the queue records what it declares, and its areas cover what its other fields imply (FLOW-02); no unanswered blocking question (FLOW-07); whether it needs a design review decided (FLOW-19). |

> | 7 | Merge | The human merged the PR (FLOW-08). The loop confirms it; there is no step for the human to claim. While the PR waits and `main` moves, a refresh step keeps it current (FLOW-10). |

> A transition happens only when the phase's done-when checks pass mechanically
> (files present, sections filled, commands green), never on an agent's own report
> that it is done. From Plan on, each done-when also checks that the queue
> still records what the design declares; from Implement on, that every
> path the branch changes lies in an area the design declares (FLOW-02). A
> path outside them is settled by widening the declaration inside the
> approved design, or by a return to Design (FLOW-06).

> | 3 | Merge | A human merged it into `main` (FLOW-08). Its PR is refreshed meanwhile, as a feature's is (FLOW-10). |

and one paragraph is added before "Queue state lives on the `workflow`
orphan branch":

> A feature whose home is a repository in which the loop's gates do not
> run is **tracked**. The queue records it, its repository and its pull
> request. Its phases are two, Work and then Merge, each one `human` step
> (FLOW-04), and no done-when check runs for them. FLOW-05's limits
> neither count it nor hold it.

**Severity and check:** MUST, `automated`, before and after.
**Enforcement:** `tools/workflow/check/check.go`, `declared.go` and
`areas.go` (new): the comparison at phases 2 to 6 for an item the queue
records a declaration for, the diff check at phases 3, 4 and 6, the
refresh's done-when, and a tracked item's one-line result. Phase 7's
check, the merge confirmation, compares a squashed branch's change with
the PR commit's without context lines (`patchID`, `-U0` in place of `-U3`;
a14), so a PR merged behind `main` is still confirmed.

### FLOW-04 (amend), `docs/development/process.md`

**Why:** a tracked item's steps, and the step that records a merge in a
second repository (`seam-specs`' a15), are `human` steps that the list does
not name. `wf commit` asks a rejected claim again, so "means the claim
lost" is no longer always true; and the limits now bind every claim. Three
clauses change.

**Current wording**, the `human` bullet and the paragraph after the list:

> - `human` — answering blocking questions, the design review (FLOW-19), and
>   escalations: a runaway budget (FLOW-13), CI repair exhausted (FLOW-11), a third
>   return to Design (FLOW-06), a branch that did not delete on merge (FLOW-16).

> A worker takes only steps whose affinity it satisfies. A step is offered when
> every step it needs is done and, unless it is `human`, its feature has reached
> the step's phase; no worker may claim or complete a host or cloud step of a
> later phase. A step is claimed with a
> lease (claimant plus expiry) recorded on the `workflow` branch; a push rejected
> because the branch moved means the claim lost and the worker re-reads state. An
> expired lease may be reclaimed.

**Proposed wording:**

> - `human` — answering blocking questions, the design review (FLOW-19), and
>   escalations: a runaway budget (FLOW-13), CI repair exhausted (FLOW-11), a third
>   return to Design (FLOW-06), a branch that did not delete on merge (FLOW-16).
>   Also the steps of work in a repository where the loop's gates do not
>   run: a tracked item's steps (FLOW-03), and a step that records the
>   human's merge there (FLOW-08).

> A worker takes only steps whose affinity it satisfies. A step is offered when
> every step it needs is done and, unless it is `human`, its feature has reached
> the step's phase; no worker may claim or complete a host or cloud step of a
> later phase. A step is claimed with a
> lease (claimant plus expiry) recorded on the `workflow` branch, and only
> when FLOW-05's limits allow the claim, whoever the worker is. When the
> push of a claim is rejected because the branch moved, the claim is asked
> again on the new state: it stands if the step may still be claimed
> there, and otherwise it is lost and the worker re-reads state. An
> expired lease may be reclaimed.

**Severity and check:** MUST, `automated`, before and after. No enforcement
path under the rule-change paths: the guard and `wf commit` are in
`tools/workflow/queue/` and `cmd/wf/`. `loop-native-lead` amends other
clauses of this rule (host names, the lease).

### FLOW-05 (amend), `docs/development/process.md`

**Why:** this is the rule the item is for. It gains the relations; its
three limits count by relation; class 3 takes in reconciliation items
(`loop-contract` a5, decided); and it now states the `alters_relationships`
definition that `alters-slot` built and FLOW-20's Limit bullet carried.

**Current wording**, the whole rule after its header:

> Work goes deep, not wide. Open items are taken in class order, and within a
> class in-progress items come before queued ones, then the higher priority:
>
> 1. **In implementation**: an in-progress feature in Implement (phase 3)
>    through Close, or an in-progress process item in Implement (phase 2) or
>    Close, except while it waits for the human's merge.
> 2. **Process items** (FLOW-03), which are usually short and change the rules
>    the rest of the queue runs under.
> 3. **Fixes**: a feature that `follows` an item, fixing or finishing it. It
>    starts only once that item is done.
> 4. **New functionality**: every other feature.
>
> While a process item is in progress, a worker matching one of its `holds`
> (affinity patterns such as `cloud` or `host:*`) takes no new step, whatever
> the class of the item the step belongs to, so disruptive work waits for the
> rules to settle; a step it already claimed stays claimed, and finishing the
> process item releases the hold. Otherwise a worker takes the next ready step
> of the first item in that order, and keeps taking that item's steps until it
> blocks: on a `human` or `host:<name>` step, an exhausted budget, the PR
> slot, or the human's merge (FLOW-08). Only then does it take steps from the
> next item down, and it returns to the earlier item as soon as that one has a
> ready step again.
>
> While interfaces are still stabilizing, a feature developed in parallel is likely
> to need an involved rebase and consolidation once the features ahead of it land.
> Parallel features are therefore the fallback that keeps work moving while the
> human is away, not a goal. A queued item starts only when every item ahead of
> it is blocked.
>
> At most 5 features are in progress at once, not counting process items
> (FLOW-03), and at most 1 of those features declares `alters_relationships: true`
> (FLOW-02), because two such features reshape the same structure and make each
> other's consolidation costly. Process items are not counted because they are
> short and already ranked ahead of fixes and features; one that would disrupt
> running work declares `holds`. A feature whose design discovers it alters
> relationships waits at the end of Design until that slot is free.
>
> At most 1 feature holds the **PR slot**: from merging `main` at the start of
> Publish until its PR merges. A feature finished through Guide waits for the slot,
> so its consolidation runs against a `main` that stays put until its PR merges. A
> feature waiting behind others periodically merges `main` with conflict-only
> fixes, so its consolidation stays as small as it can.

**Proposed wording**, the whole rule after its header:

> Work goes deep, not wide. Open items are taken in class order, and within a
> class in-progress items come before queued ones, then the higher priority:
>
> 1. **In implementation**: an in-progress feature in Implement (phase 3)
>    through Close, or an in-progress process item in Implement (phase 2) or
>    Close, except while it only waits for the human's merge. An item whose
>    open PR needs a refresh (FLOW-10) is in implementation.
> 2. **Process items** (FLOW-03), which are usually short and change the rules
>    the rest of the queue runs under.
> 3. **Fixes and adoptions**: a feature that `follows` an item, fixing or
>    finishing it, which starts only once that item is done; and a
>    reconciliation item (LOOP-07), which the queue marks with `adopts` and
>    which starts once the release it adopts exists (LOOP-04). Intake
>    places a reconciliation item below the open fixes unless the human
>    says otherwise.
> 4. **New functionality**: every other feature.
>
> While a process item is in progress, a worker matching one of its `holds`
> (affinity patterns such as `cloud` or `host:*`) takes no new step, whatever
> the class of the item the step belongs to, so disruptive work waits for the
> rules to settle; a step it already claimed stays claimed, and finishing the
> process item releases the hold. A hold names workers and covers work in
> every repository. Otherwise a worker takes the next ready step
> of the first item in that order, and keeps taking that item's steps until it
> blocks: on a `human` or `host:<name>` step, an exhausted budget, a limit
> below, or the human's merge (FLOW-08). Only then does it take steps from the
> next item down, and it returns to the earlier item as soon as that one has a
> ready step again.
>
> A worker starts a queued item only when every item ahead of it is
> blocked. Two items that change the same thing are likely to need an
> involved consolidation once the first lands, so the limits below bound
> how many such items are in flight. Items that do not collide may be in
> flight, and have pull requests open, side by side.
>
> **How two items relate.** An item names the repositories it works in and
> the areas it changes (FLOW-02). A repository's *area map*, the Areas
> entry of its profile (LOOP-01), gives each area its paths, its mode and
> the areas it reaches. Two items cannot both change an *exclusive* area;
> any number may change an *ordered* area, and the later to merge is put in
> order by its refresh (FLOW-10). Area A *reaches* area B when code in A
> depends on code in B; only exclusive areas reach and are reached. A
> repository with no map is one exclusive area, and
> an item that declares no areas is taken to change the whole of each
> repository it names. Two features are:
>
> - **colliding** when they share an exclusive area; when they have a
>   repository in common and either declares no areas; when both change a
>   seam that has no specification (LOOP-02); or when the human says so in
>   a design review (FLOW-19);
> - **near** when they do not collide and an area of one reaches an area of
>   the other, both change the same rule (FLOW-14), or both change a seam
>   that has a specification;
> - **disjoint** otherwise.
>
> A process item is outside these relations: it stops other work only
> through `holds`. A tracked item (FLOW-03) is shown with its relations and
> neither holds nor waits.
>
> **The limits.** A feature *holds* a limit from the claim that takes it.
> A holder's *set* is the holder and the other holders it collides with;
> for the PR slot, collides with or is near. No holder's set may number
> more than the limit: a claim that takes a limit is allowed only while
> the claimant's set, and the set of every holder it relates to, would
> stay within it. Only holders are counted and an item that holds is
> never refused, so no two items wait for each other.
> Process items are not counted because they are short and already ranked
> ahead of fixes and features; one that would disrupt running work declares
> `holds`.
>
> - **In progress.** A feature takes it when it starts and holds it while
>   it is in progress.
> - **`alters_relationships`.** A feature that declares
>   `alters_relationships: true` (FLOW-02) takes a slot with the claim of
>   its Plan step and holds it until it is done, a return to Design
>   included, because two such features reshape the same structure and
>   make each other's consolidation costly. A feature whose design
>   discovers it alters relationships waits at the end of Design until a
>   slot is free.
> - **The PR slot.** A feature takes the PR slot with the claim of the
>   step that merges `main` at the start of Publish and holds it until its
>   PR merges, a return to Implement or Design after the PR is open
>   included. Its limit is 1, so no two features that collide or are near
>   have pull requests open together. When related features wait for the
>   slot, the higher priority goes first. A feature finished through Guide
>   waits for the slot, so its consolidation runs against a `main` that
>   the items related to it leave alone until its PR merges. A disjoint
>   item may merge meanwhile, and the refresh (FLOW-10) then brings the
>   open PR up to date.
>
> The first two limits are the values the human sets in the workflow
> branch's `config.yaml`: 5 features in progress and 1
> `alters_relationships` slot when it sets none. Every claim is checked
> against the limits, whoever the worker is (FLOW-04). A feature waiting
> behind others periodically merges `main` with conflict-only fixes, so its
> consolidation stays as small as it can.

**Severity and check:** SHOULD, `automated`, before and after.
**Enforcement:** `wf`'s queue code (`tools/workflow/queue/`), which
`rules.yaml` already names as the rule's owner; that entry does not change.

### FLOW-06 (amend), `docs/development/process.md`

**Why:** a pending refresh step is a step of a later phase that is not a
template step, and the rule as written keeps those through a return.

**Current wording**, the last two sentences:

> Other pending steps of later phases stay, and Design reconciles them with the
> new design. A native validation already done in Publish is queued again.

**Proposed wording:**

> Other pending steps of later phases stay, and Design reconciles them with the
> new design, except a pending refresh step (FLOW-10), which is dropped:
> Publish merges `main` again. A native validation already done in Publish
> is queued again.

**Severity and check:** MUST, `agent-judgment`, before and after. No
enforcement path: `Back` is in `tools/workflow/queue/`.

### FLOW-08 (amend), `docs/development/process.md`

**Why:** `wf` cannot see a merge in a repository where its gates do not
run, and the rule says the queue has no step for a merge. `seam-specs`' a15
asked for an exception for one item; this makes it the rule (a6).

**Current wording**, the paragraph's last sentence:

> There is no sign-off step in the queue: the loop sees the merge (FLOW-03, phase
> 7).

**Proposed wording:**

> There is no sign-off step in the queue for a pull request the loop can
> see: the loop sees the merge (FLOW-03, phase 7). For a pull request in a
> repository where the loop's gates do not run, a `human` step records the
> merge (FLOW-04). The merge is still the sign-off; the step is only the
> queue's record of it.

**Severity and check:** MUST, `automated`, before and after. No enforcement
change.

### FLOW-09 (amend), `docs/development/process.md`

**Why:** under Decision 21 a PR may merge behind `main`, and CI on the push
to `main` is then the gate. The rule has a session file a regression it
finds "while the feature is built, validated or consolidated"; nothing
files one that CI reports on `main` itself, and a fix filed with nobody
there has no areas, so it would collide with every feature (a4, a16).

**Current wording**, the sentence on a regression `main` already has:

> A regression `main` already has, found while the
> feature is built, validated or consolidated, is queued at once as the feature's
> immediate follow-up unless the human explicitly says otherwise: the session that
> found it files a fix item that `follows` the feature, at the top of the fixes,
> and the feature neither waits for it nor absorbs it.

**Proposed wording.** That sentence is unchanged, and two follow it:

> A failure that CI reports on `main` after a merge is such a regression:
> the run that sees it files the fix the same way, as the follow-up of the
> item whose pull request that merge was, and files nothing when the
> merge was no item's. A follow-up filed with no human present takes the
> repositories and areas of the item it follows (FLOW-02), for its design
> to confirm.

**Severity and check:** MUST, `automated + agent-judgment`, before and
after. No enforcement path: the tick records `main`'s result with
`wf main` and files through the intake skill's follow-up path, and
`wf new -follows` copies the fields (`tools/workflow/cmd/wf/`).
`rules.yaml`'s entry for the rule (a follow-up for `tier2_gaps`
consistency) does not change.

### FLOW-10 (amend), `docs/development/process.md`

**Why:** with several PRs open, a PR's consolidation goes stale each time
another merges.

**Current wording**, the rule's last paragraph:

> When consolidation finds work beyond conflict fixes and wording, the feature goes
> back: to Implement for work the plan already covers, or to Design (FLOW-06)
> when the design must change.

**Proposed wording.** That paragraph is unchanged, and one follows it:

> A PR that waits for the human's merge is kept current. Whenever `main`
> has moved past its branch, a **refresh** step merges `main` into the
> branch again, runs the gates, and repeats this review, and FLOW-15's
> sweep, over what landed since the branch last merged `main`, as a dated
> entry in the consolidation section. It pushes only while the pull
> request is still open. A refresh that finds work beyond conflict fixes
> and wording sends the feature back as above; its pull request is then a
> draft until the feature is published again, and it keeps the PR slot
> (FLOW-05). A process item's open PR is refreshed the same way, without
> the review.

**Severity and check:** MUST, `agent-judgment`, before and after.
**Enforcement:** the refresh's done-when, in `tools/workflow/check/check.go`
(FLOW-03).

### FLOW-15 (amend), `docs/development/process.md`

**Why:** the sweep's results are for the codebase the feature leaves
behind, which changes when `main` does.

**Current wording**, the rule's first paragraph:

> FLOW-10 looks across features; this sweep looks inside the codebase the feature
> leaves behind. Publish records its results in the design's consolidation
> section:

**Proposed wording:**

> FLOW-10 looks across features; this sweep looks inside the codebase the feature
> leaves behind. Publish records its results in the design's consolidation
> section, and a refresh (FLOW-10) runs it again on the branch with `main`
> merged:

**Severity and check:** MUST, `automated + agent-judgment`, before and
after. No enforcement change.

### FLOW-19 (amend), `docs/development/process.md`

**Why:** the review is where Jeff confirms what an item may run beside. A
design that claims more room than intake agreed must not skip it, and Plan
must not change the areas after it.

**Current wording**, the first bullet of the session's list; the skip
criteria; the delta sentence; and the automated part:

> - what changes and how: the components, seams and flows touched, before and
>   after, with the `overview.md` pseudocode change where a flow changes;

> `skip` is
> allowed only when the design declares `alters_relationships: false`, no `seams`,
> `security_boundary: none` and no blocking `native_validation`; raises no assumed
> question; declares no `rule_changes`; and the branch changes no rule in this
> directory.

> Whether it may be skipped is judged on what changed since the last
> approval, not on the whole design: `skip` is allowed when the impact fields
> above are unchanged from the design at its approved commit, no question is
> open, and no rule changed since that commit.

> The automated part checks the recorded decision, the skip criteria and that each
> review the queue seeded has an approval (a `rule change` line is not one): a
> design-review step's completion and the Plan gate check its approval, and the
> Plan gate fails on an impact or `rule_changes` change since it.

**Proposed wording.** The first bullet is unchanged, and one follows it:

> - the repositories and areas it declares (FLOW-02), and the open items it
>   is near or collides with, and why (FLOW-05); the human may narrow or
>   widen the areas, or name a collision the area map does not show;

and the three passages become:

> `skip` is
> allowed only when the design declares `alters_relationships: false`, no `seams`,
> `security_boundary: none` and no blocking `native_validation`; raises no assumed
> question; declares no `rule_changes`; declares no fewer repositories or
> areas than intake recorded, where declaring areas when intake recorded
> none counts as fewer; and the branch changes no rule in this directory.

> Whether it may be skipped is judged on what changed since the last
> approval, not on the whole design: `skip` is allowed when the impact fields
> above, and `repos` and `areas`, are unchanged from the design at its
> approved commit, no question is open, and no rule changed since that
> commit.

> The automated part checks the recorded decision, the skip criteria and that each
> review the queue seeded has an approval (a `rule change` line is not one): a
> design-review step's completion and the Plan gate check its approval, and the
> Plan gate fails on a change to an impact field, to `repos` or `areas`, or
> to `rule_changes` since it.

**Severity and check:** MUST, `automated + agent-judgment`, before and
after. **Enforcement:** `tools/workflow/check/review.go`: `impactDelta`
compares `repos` and `areas`; `wf declare` refuses a narrowing while the
design says `skip`.

### FLOW-20 (amend), `docs/development/process.md`

**Why:** FLOW-05 now states what the limit counts and where its value is
set, so the bullet repeats it. The value in `config.yaml` stays 3 (the
umbrella's a5). The freeze, the Moves bullet and the Overview bullet stay.

**Current wording**, the second bullet:

> - **Limit.** FLOW-05's limit on features past Design that declare
>   `alters_relationships: true` is the value the human sets in the workflow
>   branch's `config.yaml`, which may be above 1. The limit of 5 features in
>   progress stands.

**Proposed wording:** the bullet is removed. The rule's other text is
unchanged.

**Severity and check:** MUST, `agent-judgment`, before and after. No
enforcement change.

### LOOP-01 (amend), `docs/development/loop-contract.md`

**Why:** the area map is the component's, so the profile names it, and the
contract's table of what is local has to say so (LOOP-08: no loop rule
requires a path the profile does not name).

**Current wording**, the queue's row of the first table (the terms table
has no row for areas):

> | The queue | The kinds of item, the phases, the order and what blocks what (LOOP-04) | Nothing: there is one queue, and an item names the repositories it works in |

**Proposed wording:**

> | The queue | The kinds of item, the phases, the order and what blocks what (LOOP-04) | Its area map, which says what two items may change in it side by side. Otherwise nothing: there is one queue, and an item names the repositories it works in |

and the terms table gains a row, after **Rule and machinery paths**:

> | Areas | Its area map: the areas of its tree, their paths and modes, and which area reaches which | FLOW-02, FLOW-05 |

**Severity and check:** MUST, `agent-judgment`, before and after. LOOP-01
already says that a term a component does not have is `none`, and FLOW-05
says what `none` means here: one exclusive area.

### LOOP-04 (amend), `docs/development/loop-contract.md`, `docs/design/TEMPLATE.md`

**Why:** the rule says "until the queue records them", and now it does.

**Current wording**, the second to fourth sentences:

> An item names the
> repositories it works in. Until the queue records them, it names them in
> its design; an item that names none works only in the repository that
> holds its design.

and, in `TEMPLATE.md`, under **Design**:

> The behavior and structure of the change. Name each repository the item
> works in other than this one (LOOP-04); a design that names none works
> only here.

**Proposed wording:**

> An item names the
> repositories it works in: its design declares them and the queue records
> them (FLOW-02). An item that names none works only in the repository that
> holds its design.

and, in `TEMPLATE.md`:

> The behavior and structure of the change. For each repository in `repos`
> other than this one, say what is done there and which invariants apply
> (LOOP-04, LOOP-08).

**Severity and check:** MUST, `agent-judgment`, before and after. No
enforcement change.

### PROFILE-01 (amend), `docs/development/loop-profile.md`, `tools/tier1/`

**Why:** the profile gets its Areas entry, and the map a check that holds
it to the tree.

**Current wording**, the header's check line; the paragraph on the agent
part; and the Process row of the checks table (the profile's table has no
Areas row):

> **Check:** agent-judgment

> The agent part: Review judges that the diff leaves no entry stale; the
> design review of a change to this rule judges that every term is filled
> or `none` (LOOP-01).

> | Process | Tier 1's `rule-index`, `design-front-matter`, `drivers-claim` and `suppression` checks; `wf`'s done-when checks and `wf validate`; `make enforcement`; `tools/workflow/flow18-check.sh` | 1 |

**Proposed wording:**

> **Check:** automated + agent-judgment

> The automated part: tier 1's `area-map` check fails when the area map
> does not place every tracked file in one area or does not match the
> import graph. The agent part: Review judges that the diff leaves no
> entry stale; the design review of a change to this rule judges that
> every term is filled or `none` (LOOP-01).

> | Process | Tier 1's `rule-index`, `design-front-matter`, `drivers-claim`, `suppression` and `area-map` checks; `wf`'s done-when checks and `wf validate`; `make enforcement`; `tools/workflow/flow18-check.sh` | 1 |

and the profile's table gains a row, after **Rule and machinery paths**:

> | Areas | `tools/tier1/areas.yaml`: the areas, their paths and modes, which area reaches which, the area each `drivers` and `surfaces` value stands for, the areas every driver shares, and the seams that have a specification beyond the `seams` list of `tools/tier1/rules.yaml`. |

**Severity and check:** MUST before and after; the check changes from
`agent-judgment` to `automated + agent-judgment`. **Enforcement:** the new
file `tools/tier1/areas.yaml` (the map above), which tier 1 reads from the
checkout it checks and does not embed, so `tools/tier1/tier1.go`,
`tools/workflow/enforcement-check.sh` and FLOW-14's list of the
configuration files `main`'s tier 1 takes from the branch all stay as
they are; `tools/tier1/rules.yaml` gains `PROFILE-01: {check: [area-map]}`;
`tools/tier1/rules/rules.go` lists the check; `tools/tier1/coupling/`
holds it, with what it fails on under **The area map**. From this merge on,
a feature changes the map by declaring `{rule: PROFILE-01, change: amend,
paths: [tools/tier1/areas.yaml]}`.

### The contract's citations, checked

Each clause of the Loop Contract that cites a rule amended here, and
whether it is still true with the proposed wording.

| Clause | Still true? |
|---|---|
| LOOP-04: "There is one order, and FLOW-05 gives it. FLOW-05 also says which items may be in progress side by side and which wait for another. This rule adds no order and no limit of its own." | Yes. The classes are the order; the relations and limits say the rest |
| LOOP-04: "A process item ranks ahead of every item not already in implementation (FLOW-05). It stops other work only through the `holds` it declares (FLOW-05, LOOP-09)." | Yes. Class 2 is unchanged, and FLOW-05 now says outright that a process item is outside the relations |
| LOOP-04: "Every item has the phases of its kind (FLOW-03), and every step says who may take it (FLOW-04)." | Yes. FLOW-03 now gives a tracked feature its two phases, and FLOW-04 names their steps as the human's |
| LOOP-04: "the session that finds a regression trunk already has files its fix at the top of the fixes (FLOW-09), and the human may move it" | Yes. Class 3 keeps the fixes; adoptions go below them. FLOW-09's new sentence has the run that sees a red `main` file the fix the same way, at the same place |
| LOOP-04: "An item that waits on another component waits for a release (LOOP-07): it is held with a blocking question that names the release (FLOW-07), and resumes when the release exists." | Yes. Class 3 cites it for when a reconciliation item starts |
| LOOP-04: "Until the queue records them, it names them in its design" | Amended above |
| LOOP-05: "Every claim records the worker ... and the item, and through the item the repositories the work is in (LOOP-04)." | Yes, and now through a queue field |
| LOOP-05: "FLOW-04 says which worker may take a step." | Yes. The amended clauses change what a claim is checked against and what a rejected push does, and name two more `human` steps; a worker still takes only steps of its affinity |
| LOOP-06: "Each unattended run appends one section to the day's digest (FLOW-13). There is one digest, for work in every repository." | Yes. A run's file is its section; `wf digest` builds the one page |
| LOOP-06, item 1: each step's "kind and FLOW-05 class" | Yes. Class 3 is now named "Fixes and adoptions" |
| LOOP-06, item 5: "everything blocked or parked, and why" | Yes. The guard's reasons are `wf next`'s `blocked` lines; a red `main`, holders over a limit and an item sent back with its PR open are listed with them |
| LOOP-07, point 2: a reconciliation item "is a feature item (LOOP-04), filed through intake, and FLOW-05 gives its place in the order." | Yes. This is the amendment its Decision 7 left to this item |
| LOOP-08: "No loop rule may require a language, a build tool, ... a directory layout beyond what the profile names" | Yes. The map is a profile term; the import-graph check is the component's (tier 1); `wf` reads a copy of the map |
| LOOP-09: a branch "follows the old text until it merges trunk, which FLOW-05 has it do while it waits and Publish does at its start" | Yes. FLOW-05 keeps that sentence; the refresh adds a third occasion |
| LOOP-09: a process item "declares the workers it holds (FLOW-05)" | Yes. A hold still names workers (Decision 18) |
| LOOP-03, class 4: "the phase done-when checks (FLOW-03) and the queue's validity" | Yes. The new checks are done-when checks and `wf validate` |

## Validation plan

- **Tier 0**, `make test` (TEST-01, TEST-02). States are built with the
  queue's own commands (`Seed`, `Claim`, `Complete`, `Release`, `Back`,
  `Declare`), as `alters-slot`'s tests are, and each test asserts `Next`,
  `Guard` and `Validate` together.

  `tools/workflow/queue`:
  - `TestRelationTable`: one case per row of the relation table and per
    line of the examples table; `TestRelationSymmetric`: both orders give
    the same relation and reason for every pair of a generated set.
  - `TestRelationUndeclaredIsToday`: with no item declaring areas, every
    pair of features in one repository collides.
  - `TestRelationUnknownArea`: an area the map lacks makes the item
    undeclared.
  - `TestAreaMapParse` (unknown key, missing `mode`, a `catch_all` with
    paths, `reaches` on an ordered area, an ordered area named in
    `reaches`) and `TestAreaMapMatch` (directory, file, `*`, the longest prefix,
    a tie, the catch-all).
  - `TestAreasAbsentAndEmpty`: an absent `areas` and `areas: []` survive a
    save and a load as two different things.
  - `TestGuardInProgressPerSet` (cases 9, 10, 16), `TestGuardAltersSlot`
    (cases 2, 6, and the star of case 19: X is refused because H's set
    would be 4; `alters-slot`'s tests keep passing on undeclared items),
    `TestGuardPRSlot` (cases 3, 4, 7, 8, 20: a claimed step holds; near and
    colliding block; disjoint does not; Close holds nothing; a return
    before the PR opened gives the slot up and re-entry holds from the new
    claim; a return with a PR recorded keeps it, in phase 3 and in phase
    1, and the returned item's own next `f6-merge-main` is not refused),
    `TestGuardPRSlotTieBreak` (cases 13 and 21: a lower-priority item is
    refused while a related higher-priority one waits, ready or not; the
    highest-priority waiting item is refused only by a holder; over
    generated arrivals no waiting item is passed over without end).
  - `TestGuardEveryWorker` (case 5): it replaces the host half of
    `TestAltersSlotCloudOnly`, which asserts today that the host's claim
    stands.
  - `TestGuardNoMutualHold`: over generated queues, a holder is never
    refused and the waits form no cycle.
  - `TestGuardThenValidate`: over generated queues and maps, starting from
    a valid state, `Validate` reports no `limit:` error after any claim
    `Guard` allows, for each of the three limits; and the star of case 19
    by name.
  - `TestValidateLimitsPerSet`: each limit, at and over, names the item and
    the others, with the `limit:` prefix; the star, where no single
    asker-centred count is over; `TestInProgressLimitDefault` (absent, 5,
    0, -1, as `Load` reads them).
  - `TestNextUnchangedOnUndeclaredQueue`: a fixture of today's queue files
    gives the `next`, `blocked` and `validate` lines `wf` at `897a4a8`
    gives.
  - `TestClassAdopts` and `TestAdoptsValidate`; `TestTrackedRound` (seeded
    steps, any claim starts it, done without a check, outside every limit,
    never class 1; `-seams` recorded, and refused for an item homed here).
  - `TestNewFollowsCopiesAreas`: `-follows X` with no `-areas` copies X's
    `repos` and `areas` and logs it; `-areas` overrides; an undeclared X
    gives an undeclared item (case 23).
  - `TestRefreshStep`: seeded once; class 1 while pending; dropped by the
    merge step's completion and by `Back`; its completion leaves the phase
    alone. `TestStale`, on temporary git repositories: `main` moved, a
    refresh is seeded; PR merged and branch gone, none is seeded and
    `merged:` is printed; PR closed and branch gone, none; an item sent
    back whose PR then merged is moved to Close with
    `f8-escalate-returned`; run twice, nothing is added twice.
  - `TestMainCI`: `wf main` writes and reads `main.yaml`; `red_since` is
    kept across red commits and cleared by a green one; `-followup` is
    recorded once; absent, `QUEUE.md` says `CI not recorded`.
  - `TestDeclare`: each row of the `wf declare` table; a widening that
    would put a limit over is refused and changes nothing (cases 14, 15);
    a widening is judged on the design at its approved commit, so an
    agent that adds a driver to `drivers` on the branch and then asks for
    that driver's area is refused; a design that declares `relay` and
    lists every driver may not widen into `driver/hyperv`; one that
    declares only `driver/hyperv` may widen into `cli` through `surfaces`;
    a skipped review is read on the branch; `declared` is written.
    `TestRelate` (human only; refused over a limit).
  - `TestCommitReplay`, on temporary git repositories: a rejected push of a
    done step is replayed and pushed; a claim is asked again on the new
    state and stands when nothing refuses it, counted once (case 12); it
    is dropped with the guard's reason when the push that won took the
    last slot (case 11), with the tie-break's when a higher-priority
    related item finished Guide in it, with `held by process item` when a
    hold started in it, and with `blocking question` when the human opened
    one in it; a conflict in a queue file gives exit 5 without `-claimed`;
    a conflict in `QUEUE.md` is regenerated; only the paths given and the
    generated files are staged; another session's uncommitted file is
    neither committed nor reset away (exit 5 names it); nothing is
    force-pushed.
  - `TestSummary`: `QUEUE.md` for the two states shown under **What Jeff
    sees**, as golden files; `TestDigestPage` (file order, the earlier
    page, the same input giving the same page).

  `tools/workflow/check`:
  - `TestDeclaredAtDesign` (queue against design; `declared` required;
    each implied-areas rule; an undeclared design; and two fixtures from
    merged designs: `relay-leaf-crl`'s front matter with
    `areas: [relay, book]` and `http-body-path-drift`'s, which lists all
    five drivers, with `areas: [relay, specs]`, both of which pass, and
    `drivers: [hyperv]` with `areas: [book]`, which fails),
    `TestQueueMatchesDesignLater` (a queue whose `alters_relationships`
    was written back fails Plan, Implement, Review and Publish; a queue
    file with no `declared`, built from `seam-specs`' and
    `driver-contract`'s as they are today against their designs, passes
    each of those gates with the `not declared` line, and fails them once
    `declared` is set and a field differs), `TestInsideAreas` (inside;
    outside; the item's own design; a deleted and a renamed path;
    undeclared), `TestTrialMerge` (clean; ordered; exclusive; a branch
    that is not on the remote), `TestRefreshDoneWhen` (each check; the
    local head ahead of the remote fails "pushed"), `TestImpactDeltaAreas`,
    `TestSkipNarrowing`, `TestTrackedCheck`, and `TestSquashBesideEdit`:
    in a scratch repository, A edits line 5 and B line 8 of one file, B is
    squash-merged after A, and B's branch is confirmed as merged; the
    existing `wf-squash-after-base-moved` tests keep passing, including
    the ones that refuse a head pushed past the squash.

  `tools/tier1`:
  - `TestAreaMapCheck` on fixtures (a missing edge, a stale edge, a tie, a
    driver with no `implies` entry, an `implies.shared` that lacks `relay`,
    an area with both `reaches` and `reaches_all`, a `go list` error, a
    checkout with no map) and `TestRealAreaMap`, which runs the check on
    the repository's own map and tree with the 18 runs the design names,
    as `TestRealIndex` does for the rule index.
- **The live queue.** With `main`'s `dist/wf` and the branch's `wf` built
  to a scratch path, on a copy of the `workflow` checkout: `wf validate`
  prints `ok` both times; `wf next -worker cloud/check` and
  `wf next -worker host:macos-vz/check` print the same lines; the copy's
  `QUEUE.md` differs by the header, the split of **Needs Jeff**, the new
  columns and sections, and the two renamed headers, and by nothing else.
  `wf check -feature seam-specs` and `wf check -feature driver-contract`,
  run with both, give the same results, the new one adding the
  `not declared` line. Then, on the copy, with `repository` set,
  `wf areas -sync` and `wf main` run: a human `wf declare` on both
  features, after which `wf relations` prints
  `seam-specs, driver-contract: colliding (seam collector, which has no specification)`
  and `wf check` for each still passes. The outputs go in the PR
  description.
- **The map.** `make lint` runs `area-map` on the branch. The script that
  produced the tables above is re-run, with the matrix the design names,
  and its output compared with the shipped `reaches`: the draft's run
  gave no missing and no stale edge for any of the 21 areas.
- **`make lint`** for the three modules, `make enforcement` (this PR
  changes `tools/tier1/` and `tools/workflow/check/`; a process item's
  branch is exempt from the clearance, and the comparison of findings still
  runs), and `tools/workflow/flow18-check.sh`.
- **The rule text.** Each amended paragraph is compared, word for word,
  with the wording approved above (the PR's diff).
- **Native validation:** none, and no tracked gap (FLOW-09). The queue code
  is the same on every host. `wf commit` uses `git fetch`, `reset`,
  `cherry-pick` and `push`, which every checkout's git has;
  `git merge-tree --write-tree` needs git 2.38 and runs only in Review and
  Publish, which are cloud steps (the cloud's git is 2.43).

## Implementation plan

A process item has no Plan phase; this is the work of `x2-implement`.

**Size.** Large, and larger than the queue's estimate (12 steps, 3 runs)
allows for. `alters-slot` changed 6 files under `tools/` and `.claude/`
(+606, -55) and `loop-contract` 14 (+763, -45). This item adds nine Go
source files and their tests, eight `wf` commands, rewrites `next.go` and
`summary.go`, names about forty tests, amends fourteen rules and edits
eight skill and agent files. It is one step, so it carries over runs the
way a long Implement does: each run claims `x2-implement` again (one
budget step), works the tasks in order, pushes the branch at each task's
green checkpoint (FLOW-17), and releases the step naming the task it
reached. Four to six runs are likely. The estimate is Jeff's to raise at
the review (FLOW-13); the runaway check would otherwise escalate at 24
steps.

1. Merge `origin/main`, which has had `loop-contract` since `ad4ea50`:
   tasks 14 and 15 amend files it added (FLOW-14, FLOW-17, LOOP-09).
2. Tests first: each of tasks 3 to 12 starts with the tests the
   Validation plan names for it, failing on the current code (TEST-01,
   TEST-02).
3. `queue/areas.go`, `tools/tier1/areas.yaml` and tier 1's `area-map`
   check with its `rules.yaml` entry (PROFILE-01, LOOP-08, LINT-02).
4. The queue's fields, `Config.Repository`, and `Validate`'s schema checks
   (FLOW-02, FLOW-03, GLOBAL-10).
5. `queue/relation.go` (FLOW-05).
6. `queue/limits.go`: the table, the predicate, `Guard`, the PR-slot
   holder with a recorded PR, the tie-break, the in-progress default;
   `next.go` and `cmd/wf` call it; `alters-slot`'s functions move
   (FLOW-04, FLOW-05, GLOBAL-09, GLOBAL-10).
7. `Class` for `adopts`, `wf new -repos -areas -seams -adopts` and the
   copy on `-follows`, and the `tracked` round (FLOW-03, FLOW-05, FLOW-09,
   LOOP-07).
8. The refresh: `IsRefresh`, `SeedRefresh`, `wf stale` with the merge
   check first and `Landed`, the drops in `Complete` and `Back`, class 1
   while pending, `check.Refresh`; `patchID` without context lines
   (FLOW-03, FLOW-06, FLOW-08, FLOW-10, FLOW-15, FLOW-16).
9. `check/declared.go`, `check/areas.go`, `review.go`, and `wf declare`,
   `wf relate`, `wf relations` (FLOW-02, FLOW-03, FLOW-06, FLOW-19).
10. `queue/commit.go` and `wf commit` (FLOW-04, FLOW-13, FLOW-17,
    FLOW-18).
11. `summary.go`, `queue/digest.go`, `wf digest`, `wf status`, and
    `wf main` with `main.yaml` (FLOW-09, FLOW-13, LOOP-06).
12. `wf areas -sync | -check | -list` and `wf validate -areas` (FLOW-05,
    PROFILE-01).
13. `docs/development/process.md`: the eleven FLOW amendments, word for
    word as approved (FLOW-14, FLOW-19).
14. `docs/development/loop-contract.md` and `loop-profile.md`: LOOP-01,
    LOOP-04 and PROFILE-01 as approved (FLOW-14, LOOP-09).
15. `docs/design/TEMPLATE.md`: the two front-matter lines and the sentence
    under **Design** (FLOW-02, LOOP-04).
16. The four skills and four agent definitions, as listed under
    **Machinery edits** (FLOW-14).
17. `make test`, `make lint`, `make enforcement`, `flow18-check.sh`, and
    the live-queue comparison (FLOW-17, FLOW-18).
18. Archive the design with its summary, and open the PR (FLOW-12).

**Files the PR changes.** `tools/workflow/queue/`: `queue.go`, `next.go`,
`phases.go`, `summary.go` and their tests, and the new `areas.go`,
`relation.go`, `limits.go`, `declare.go`, `commit.go`, `digest.go` with
tests. `tools/workflow/check/`: `check.go`, `review.go` and their tests,
and the new `declared.go` and `areas.go` with tests.
`tools/workflow/cmd/wf/main.go`. `tools/tier1/`: `areas.yaml` (new),
`rules.yaml`, `rules/rules.go`, `coupling/checks.go`, and the
new `coupling/areamap.go` with its test. `docs/development/process.md`,
`loop-contract.md`, `loop-profile.md`. `docs/design/TEMPLATE.md` and this
design. `.claude/skills/workflow-{tick,intake,human,native}/SKILL.md` and
`.claude/agents/wf-{design,implement,review,publish}.md`. Nothing in the
product: no driver, `relay/`, `cmd/`, the book or `specs/`.

**After the merge, on the `workflow` branch** (outside the PR). These are
edits to the queue state, which the tick's inline Close does not make and
`closed()` does not check, so they have an owner: a `human` step,
`x4-state`, in this item's queue file, done in an attended session with
Jeff once the PR has merged and no live lease was claimed before the merge
commit's time. The item is not done until it is.

- `config.yaml`: `repository: buildwarden/buildwarden`; the comment on
  `limits.in_progress` becomes "5 when unset". The values do not change.
- `wf areas -sync`, which adds `areas.yaml`, and `wf main`, which adds
  `main.yaml`.
- `README.md`: the layout table (`areas.yaml`; `main.yaml`;
  `digest/<date>/`); the queue file's new fields; the `tracked` round;
  `wf next`'s paragraph (the guard, for every worker, and the three
  limits by relation); the commands (`wf commit` in place of the `git`
  lines).
- A human `wf declare` on `seam-specs` and on `driver-contract`, which
  records their seams and rule changes and sets `declared`. Both stay
  undeclared for areas, and collide (**What changes for the queue as it
  stands**).
- `seam-specs`' `repos`, on Jeff's word: its design names
  `buildwarden/buildwarden-specs` in its text, so the design's front
  matter gains `repos` first, and the review entry records it if the
  design is already approved (**Holds**). Areas for the two features are
  his to give then or leave.

## Design review

No review by Jeff yet. This draft is put to him as the design step,
`x1-design`.

- **2026-11-14: independent review of the first draft (`0cd4e7c`), before
  Jeff's review.** Two reviewers read it, one against the code and the
  queue, one for soundness. Each finding was checked against the source
  before it was acted on. What changed:
  - *The implied-areas rule failed a relay item that lists every driver*
    (both reviewers; must). True: `relay/` is in `rules.yaml`'s
    `drivers.shared`, and no driver area reaches `relay`. The map gains
    `implies.shared`; a driver is satisfied by its own area or a shared
    one; an agent may widen into a driver's area only when the approved
    design declares no shared area.
  - *The merge stops the two live features at their next gate* (must).
    True: their queue files lack what their designs declare. The queue
    gains `declared`; gates compare only an item that has it; a human
    `wf declare` at `x4-state` records the two.
  - *`Guard` and `Validate` were different predicates* (must). True for
    any limit above 1. One predicate now: no holder's set over the limit,
    evaluated by both.
  - *A refresh could send an item back with its PR open and free the PR
    slot* (must). True. The slot is held while a PR is recorded; the PR
    becomes a draft; `wf stale` handles a draft merged all the same.
  - *A replayed claim could land what `wf claim` refuses.* True. A claim
    is asked again, not replayed.
  - *The tie-break could starve a high-priority item.* True. The
    proposal's rule is restored (a12).
  - *A follow-up was the slowest item; nothing read CI on `main`; the
    "behind `main`" mark had a window.* True. A follow-up takes the
    followed item's areas; `wf main`, `main.yaml` and FLOW-09's new
    sentences (a16); the window is stated in a1.
  - *A refresh could push to a branch GitHub had deleted; an attended
    session cannot confirm merges.* True. The refresh checks before its
    push; `wf stale` checks for a merge first.
  - *A PR merged behind `main` can fail the merge check.* True
    (`patchID`, `-U3`), and reproduced: A edits line 5, B line 8, B is
    squashed after A; B's change and the squash's differ with three
    context lines and are equal with none. a14 takes `-U0`.
  - *Dropping "the checks reach every area" was a decision, and its
    reason does not hold under a1 (a).* True. Now a15, proceeding as the
    proposal has it for `tier1` and `tier1-config`.
  - *`tools/relay-vm/init` couples `relay` and `driver/hyperv` by file.*
    True. `tools/relay-vm/` moves to `driver-core`.
  - *Widening was judged on the branch's design.* True. It is judged at
    the approved commit.
  - *Nets 6 and 7 did not exist as described; the a2 evidence did not
    show what it was cited for; four counts were one too high; the
    rendering was out of date; the two live features stay colliding
    whatever areas they get; the `go list` matrix failed on one
    platform.* All true, and corrected in place.
  - Nits taken: `areas.yaml` is read from the checkout and not embedded,
    so FLOW-14 and `enforcement-check.sh` stay as they are; the size of
    `x2-implement`; FLOW-04's three clauses; the seven small statements;
    `wf commit` stages only named paths; a map change that puts holders
    over a limit; the `rules` list; a9 and a11 withdrawn as decisions;
    the Close-time state edits get an owner (`x4-state`).
  - *Not taken:* running the gates on each trial merge's tree, offered
    with the finding on the checks' areas. It is a worktree, lint and the
    tests for every pair of open disjoint branches at every Review; a15
    (b) and the reading of `main`'s CI cover the case it was offered for.
  - *Not taken:* a stand-in area for an item that adds a new tree (a
    nit). It needs part of the branch's map copied into the queue file
    and an overlay in the relation, for two of the 52 merges; the item
    stays undeclared until it merges, which is the safe side. It is
    listed under **What this item does not do**.

## Consolidation

A process item has no Publish phase and no consolidation pass (FLOW-03).
The implement step merges current `main` before it opens the PR, and after
this item's merge a process item's open PR is refreshed like a feature's.

## Rollback notes

None.

## Archive summary

Filled in when the item is implemented (FLOW-12).

