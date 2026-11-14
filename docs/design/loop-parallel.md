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
rules: [FLOW-01, FLOW-02, FLOW-03, FLOW-04, FLOW-05, FLOW-06, FLOW-07, FLOW-08, FLOW-09,
        FLOW-10, FLOW-11, FLOW-12, FLOW-13, FLOW-14, FLOW-15, FLOW-16, FLOW-17, FLOW-18,
        FLOW-19, FLOW-20, LOOP-01, LOOP-04, LOOP-05, LOOP-06, LOOP-07, LOOP-08, LOOP-09,
        PROFILE-01, LINT-01, LINT-02, LINT-03, LINT-04, GLOBAL-09, GLOBAL-10, GLOBAL-11,
        ORCH-01, TEST-01, TEST-02]
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
      tools/tier1/areas.yaml, tools/tier1/rules.yaml, tools/tier1/tier1.go,
      tools/tier1/rules/rules.go, tools/tier1/coupling/checks.go,
      tools/tier1/coupling/areamap.go, tools/tier1/coupling/areamap_test.go]}
concepts: [workflow, queue, one-queue, parallel-items, areas, area-map, relations, collision,
           flow-05-limits, alters-slot, pr-slot, claim-guard, wf-commit, wf-next, wf-claim,
           wf-validate, refresh, consolidation, tracked-items, reconciliation,
           queue-summary, digest, componentization, program]
related: [componentization.md, alters-slot.md, next-skips-merge-wait.md, queue-summary.md,
          regression-fixes-follow-up.md, design-approved-rule-changes.md,
          tier1-static-checks.md, wf-later-phase-host-steps.md, workflow-bootstrap.md,
          workflow-streamline.md, native-host-loop.md, design-principles.md]
---

# Items that do not collide run side by side: repositories, areas, relations, the limits, the claim guard, the refresh step and `QUEUE.md`

A process item, and a program item under FLOW-20: P1's third item, and step 3
of the loop change Jeff approved in direction on 2026-11-14 (one queue,
parallel where items do not collide, native-led items). It follows
`alters-slot` (#118, merged) and `loop-contract` (approved at `7af50ff`,
being implemented), and comes before `loop-native-lead`, `loop-repository`
and the move to the loop repository. It holds no worker (Decision 17).

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
reasons. Paths are under `tools/workflow/` at `897a4a8`.

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
   **The area map**). Of the last 53 first-parent commits on `main`, eight
   changed `driver/hyperv/` and six changed `relay/`; `hyperv-ps-done`
   (#116: `driver/hyperv/` and one book page) and `relay-leaf-crl` (#70:
   `relay/` and one book page) share no path but the book.
4. **`wf claim` checks no limit.** The three limits are tests inside
   `wf next`, two of them for the cloud alone (`atLimit`, `cloudHold` in
   `queue/next.go`). A step claimed by hand, or by a host on a step whose
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
   others built and checked against a `main` that no longer exists.
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
- **`loop-contract`** (approved at `7af50ff` on `feat/loop-contract`; its
  rule files are read from that branch at `9fbc8e9`). Decisions 17 to 20,
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
   `adopts` and `collides_with`. `wf declare` copies the design's front
   matter into the queue file, and every later gate compares the two.
   - *Rejected: `wf next` reads each design from git.* It reads no git today
     (`next-skips-merge-wait`), and two workers with different fetches would
     compute different relations.
   - *Rejected: the agent copies the fields by hand*, as `wf-design` copies
     `alters_relationships` today. Six fields, and nothing compared them
     afterwards.
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
     two items near and they take turns at the PR.* Four of the last six
     product merges changed the book, so most pairs would still open one PR
     at a time, Jeff's own example among them.
5. **`reaches` is the import graph, exactly.** Area A reaches area B when a
   package in A, or its tests, imports a package in B, directly or not, on
   Linux, macOS or Windows. Tier 1's new `area-map` check fails when the map
   and the graph differ.
   - *Rejected: links written by hand for couplings the graph cannot see*
     (a relay VM image that a driver boots). A hand-written link can be
     neither checked nor kept current. Such a coupling is a seam, and the
     relation reads seams.
   - *Rejected: the proposal's "`tier1`, `rules` and `tier1-config` reach
     every area".* Every item with a rule change would then take turns at
     the PR with every other item. What that guards against, a tightened
     check failing another branch, is found by the same `make lint` either
     way: at `f6-merge-main`, or at the refresh.
6. **Near items work side by side and take turns at the PR.** Two items are
   near when an area of one reaches an area of the other, when both change
   the same rule, or when both change a seam that has a specification. This
   is "nearly non-colliding".
7. **The three limits count only the holders related to the item that
   asks, and one table drives the guard and `wf validate`.** The table is
   under **The limits**. `Guard` is called by `wf next` (and `wf next
   -feature`) and by `wf claim`, for every worker; `Validate` walks the same
   table over the holders.
   - *Rejected: the limits left in `wf next` for the cloud*, as today.
     `loop-native-lead` lets a machine take an item's first step.
   - *Rejected: a separate implementation in `Validate`.* That is how the
     two disagreed before `alters-slot`.
8. **`limits.in_progress` is 5 when unset and an error below 1** (a11), as
   `alters-slot` did for the other limit. *Dropped fallback:* absent or 0
   read as no limit.
9. **The PR slot is held from the claim of `f6-merge-main` until the PR
   merges, and priority breaks a tie** (a12). A claim holds it, expired or
   not, as a Plan claim holds an `alters_relationships` slot. An item in
   Close holds nothing. A waiting item is refused while a related item of
   higher priority is ready to take the slot.
   - *Rejected: first to ask.* With more than one worker, a low-priority
     item could take the slot the moment Jeff's merge frees it.
   - *Rejected: a tie-break at the other two limits.* `wf next` walks
     FLOW-05's order, and those slots are freed by a worker's own step, not
     by a merge between runs.
10. **`wf commit` pushes queue state, and replays it when the push is
    rejected.** A replayed claim stands only if the step is still free and
    the limits still hold on the new state; otherwise it is dropped.
    - *Rejected: today's rule, a rejected push loses the claim.* Every
      unrelated push (a machine's status file, another item's step) would
      cost a claim.
    - *Rejected: `git rebase` by each skill.* `QUEUE.md` conflicts on almost
      every replay, and the skills would each resolve it by hand.
11. **Collision is decided at intake, confirmed by Design and checked
    against the diff.** Intake records `repos` and `areas` with Jeff (a4).
    Design's done-when compares them with what the front matter implies.
    Implement, Review, Publish and the refresh fail on a changed path
    outside the areas. Review and consolidation add a trial merge against
    each disjoint in-progress branch, as a report.
12. **An agent may widen an item's areas only inside its approved design,
    and only while the queue stays valid; anything else returns to Design;
    narrowing is the human's.** `wf declare` enforces all three.
    - *Rejected: any widening returns to Design.* A design that declares
      `drivers: [hyperv]` and whose diff reaches `driver/hyperv/` has said
      so already.
    - *Rejected: an agent may narrow.* Narrowing claims more parallelism
      than the human agreed to.
13. **The refresh is one step, `f6-refresh`, taken by the cloud, seeded
    whenever `main` has moved past an open PR** (a10). It merges `main`,
    runs the gates, and carries the consolidation pass and the sweep over
    what landed.
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
    today's behaviour, and the first new field is written at Close.
    - *Rejected: `holds: [cloud, host:*]`.* A hold lasts while the item is
      in progress, from its design to its close: days during which S1 and K1
      would stop.
18. **A hold still names workers and covers work in every repository**
    (a9). *Rejected for now:* a hold that names repositories. No step of a
    second repository is taken by a worker a hold could stop until
    `loop-repository`.
19. **Thirteen rule changes, in this item's PR.** FLOW-02, -03, -05, -10,
    -15, -19 and -20, as the proposal lists; FLOW-04, -06 and -08, which
    `wf commit`, the refresh step, tracked items and a merge recorded in a
    second repository (a6) make untrue as written;
    LOOP-01, LOOP-04 and PROFILE-01, for the Areas term and the queue's new
    field. The last three need `loop-contract` on `main`, so `x2-implement`
    starts after its merge (Implementation plan, task 1).
20. **The freeze stays.** FLOW-20 loses only its Limit bullet. This is the
    proposal's question 2, option (a), part of the approved direction.
21. **Disjoint PRs in one repository merge in any order** (a1). Nothing
    makes the second wait for its refresh; `QUEUE.md` says when a PR is
    behind `main`, and CI on the push to `main` is the gate.
    - *Rejected: requiring an up-to-date branch.* Each merge would block the
      rest for a cloud run.
    - *Rejected: one open PR per repository.* That is today's rule again.

## Questions

### Blocking

None.

### Assumed

Each is put to Jeff in the design review. The first, the third, the fourth
and the sixth are the proposal's questions 3, 5, 6 and 11, whose recommended
answers he approved as the direction; they are asked again here because this
is where they take effect.

- **a1. Disjoint PRs in one repository: in what order may they merge?**
  (The proposal's question 3.) CI does not run again when a PR's base moves.
  Proceeding with (a); confirm.
  - (a) Any order. After a merge, `QUEUE.md` marks each other open PR
    "behind `main`" until its refresh is done. A PR merged while behind
    relies on CI on the push to `main`; a failure there is fixed by an item
    that `follows` the one that broke it (FLOW-09). `main`'s protection
    then requires a pull request and green checks, and does not require the
    branch to be up to date.
  - (b) An up-to-date branch is required. Each merge blocks the rest until
    their refresh, about one merge per cloud run.
  - (c) One open PR per repository: today's rule, per repository.

  Recommended: (a). Disjoint PRs share no exclusive area, and GitHub refuses
  a merge that conflicts in a shared file.
- **a2. What sharing only ordered areas costs** (the book, build files,
  rule files, other items' designs). Proceeding with (a); confirm. This
  departs from the proposal, whose section 1 put "lightly shared files"
  with "one PR at a time".
  - (a) Nothing but the refresh: the two items are disjoint, and both PRs
    may be open. The later one merges `main` at its refresh, where a
    conflict in the shared file is resolved.
  - (b) They are near and take turns at the PR, as the proposal has it.

  Recommended: (a). Under (b), a Hyper-V item and a Go relay item that each
  touch one book page open their PRs one after the other, each waiting for
  Jeff's merge of the one before; that is most pairs of product features.
  Under (a) the risk is a PR merged while behind `main` whose shared file
  merges cleanly and wrongly (two edits to `go.mod`, for example); CI on
  `main` is the net, as in a1. The change between the two options is one
  row of the relation table.
- **a3. A cap on work waiting for Jeff.** (The proposal's question 5.)
  Proceeding with (a); confirm.
  - (a) None. `QUEUE.md` opens **Needs Jeff** with the count of items
    waiting on him, by kind.
  - (b) No queued item starts while 8 or more wait on him.

  Recommended: (a). A cap would stop the loop on the one resource it cannot
  add to, and the count makes the backlog visible.
- **a4. Who declares areas, and where the map lives.** (The proposal's
  question 6.) Proceeding with (a) and (c); confirm.
  - (a) Areas are proposed at intake with Jeff and confirmed by Design; a
    design that narrows what intake recorded cannot skip its review.
  - (b) Areas are declared by Design only.
  - (c) The map is on `main`, at `tools/tier1/areas.yaml`, as the profile's
    Areas entry: a feature changes it only as a rule change its design
    review approves by name (FLOW-14), and tier 1 checks it against the
    tree.
  - (d) The map is on the `workflow` branch, changed in an attended session
    on Jeff's word.

  Recommended: (a) and (c).
- **a5. The map itself**, written out under **The area map**: 16 exclusive
  areas and 5 ordered ones. `driver-core` is one area (`driver/*.go`,
  `buildlink`, `resolvers`, `relayvm`, `script`, `linuxguest`, `winguest`).
  `relay`, `relaycfg`, `ledger`, `collector` and `executor` are five areas.
  `cli` (`cmd/warden/`) is exclusive. The book, `docs/development/`, the
  build files, other items' designs and tier 1's configuration are
  ordered. Proceeding with (a); confirm, or change an area.
  - (a) As written.
  - (b) Finer: the guest families apart from `driver-core`, so more items
    are near and fewer collide.
  - (c) Coarser: `rules.yaml`'s shared list (`relay/`, `ledger/`,
    `collector/`, `relaycfg/`, `bootstrap/`, `driver/*.go`) as one area.
  - (d) `cli` ordered, so two driver items that each edit `cmd/warden/` do
    not collide. They collide today until K1 moves the driver flags out.

  Recommended: (a).
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
- **a9. Whether a hold may name repositories.** (`loop-contract`'s hand-off
  leaves it to this item.) Proceeding with (a); confirm.
  - (a) Not yet: a hold names workers and covers work in every repository.
  - (b) Now: `holds` entries may carry a repository.

  Recommended: (a). `loop-repository` adds it with the first worker step in
  a second repository.
- **a10. The refresh: its trigger and its budget.** Proceeding with (a)
  and (c); confirm.
  - (a) Seeded for every item waiting on Jeff's merge whenever `main` is no
    longer contained in its branch, whatever moved `main`.
  - (b) Seeded only when a related item's PR merged.
  - (c) Each refresh is a claim and counts one step against the item's
    budget; intake's size guide adds two steps for it.
  - (d) A refresh spends no budget, like the merge confirmation.

  Recommended: (a) and (c). Under (c), an item that waits through many
  merges trips its runaway check and shows under **Needs Jeff**, which is
  the right place for a PR that has waited that long.
- **a11. `limits.in_progress` when `config.yaml` does not set it.**
  Proceeding with (a); confirm.
  - (a) 5, FLOW-05's written value; a written value below 1 is an error.
  - (b) As today: absent or 0 means no limit.

  Recommended: (a). The live `config.yaml` writes 5, so nothing changes for
  this queue.
- **a12. Who gets a free PR slot when two related items wait.**
  Proceeding with (a); confirm.
  - (a) The higher priority, when it is ready to take the slot itself. The
    other is refused and its reason names the one ahead.
  - (b) The first to ask.

  Recommended: (a). It only matters once a second worker exists: an attended
  run on a named item today, a machine's led item after `loop-native-lead`.
- **a13. Whether this item holds a worker.** Proceeding with (a); confirm.
  - (a) None. The cost is one narrow case: a session whose `dist/wf` was
    built before the merge saves a queue file that already carries a new
    field, and drops the field. Close writes the first new field only after
    no lease predates the merge, and every scheduled session rebuilds `wf`
    as it starts.
  - (b) `holds: [cloud, host:*]`, set by hand for the day of the merge and
    Close and removed after.

  Recommended: (a).

## Design

### What an item declares

**In the queue file.** Six fields, all optional. A queue file without them
behaves as it does today.

```yaml
repos: [buildwarden/buildwarden, buildwarden/buildwarden-specs]
areas: [driver/hyperv, book]
seams: [relay-claim]
rules_changed: [GLOBAL-08, TEST-05]
adopts: {consumable: buildwarden-relay, version: v0.3.0}
collides_with: [some-other-item]
```

| Field | What it is | Absent means | Written by |
|---|---|---|---|
| `repos` | The repositories the item works in, as `owner/name`, its home first. The home holds its design, its branch and its PR | Only the queue's own repository, `config.yaml`'s `repository` | `wf new -repos`; `wf declare` |
| `areas` | Names from the home repository's area map: the areas whose files the item changes. Its own design file needs no area | *Undeclared*: the item collides with every item it shares a repository with. `areas: []` is different: declared, and it changes nothing but its own design | `wf new -areas`; `wf declare` |
| `seams` | The design's `seams` (FLOW-02): the seams the item changes | None | `wf declare` |
| `rules_changed` | The rule IDs of the design's `rule_changes` (FLOW-14) | None | `wf declare` |
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
rule IDs of `rule_changes`, `alters_relationships` and `design_review`. It
replaces the hand copy in `wf-design`'s step 5. What it accepts depends on
where the item is and who asks:

| When | Who | What it accepts |
|---|---|---|
| Design not finished (a feature in phase 0 or 1; a process item in phase 1) | Any worker | Any value. One exception: with `design_review: skip`, a change that *narrows* is refused, with "narrowing needs the design review (FLOW-19)". Narrowing is removing a repository or an area the queue has, or declaring areas where the queue has none |
| In Plan | A `cloud` or `host:` worker | Nothing: the Plan gate compares the design with its approval (FLOW-19) |
| From Implement on | A `cloud` or `host:` worker | Only added areas, each *within the design*: the area the map gives a driver in the design's `drivers` or a surface in its `surfaces` (`implies`, below), or an area that holds a path of one of its `rule_changes`. Every other difference is refused, with "a design change: back to Design (FLOW-06)" |
| After Design | A `human` worker | Any value. This is how the review narrows, and how an item already past Design is given areas. A change made in the review is part of the design the approval names |

In every case the names must exist in the branch's area map, and the result
must pass `wf validate`: `wf declare` applies the change in memory, runs
`Validate`, and writes nothing when a limit would be exceeded. Its error
names the limit and the holders, and says "back to Design (FLOW-06)". It
logs what changed in the item's history (`declared: areas +driver/hyperv
(within drivers: hyperv)`).

**`wf relate -state DIR -worker human/<name> -feature A -with B [-clear]`**
writes or clears `collides_with` on A. Only a `human` worker may run it, and
it is refused, like `wf declare`, when the result would put a limit over.

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
out with the session's first state push. `queue.Load` reads the copy as it
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
- **`reaches`** lists the areas this one depends on (Decision 5). Unknown
  keys are rejected, as in `rules.yaml`.
- **`implies`** says which area a value in a design's front matter stands
  for, so `wf` holds none of this repository's vocabulary (LOOP-08).
- **`written_seams`** lists seams that have a specification and that
  `rules.yaml`'s `seams` list does not carry. `wf areas -sync` adds the
  names in `rules.yaml`'s `seams` list, which S1 fills, so a seam becomes
  written in the relation when its contract document is listed there, with
  no second edit.

**The map for `main` at `897a4a8`**, in full. It was run against
`git ls-files`: every tracked file falls in a named area, and `core` is
empty today.

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
    paths: [driver/, tools/build-vm/, tools/win-image-prep/]
    reaches: [collector, driver/firecracker, driver/hyperv, driver/qemu, executor, ledger,
              relay, relaycfg]
  relaycfg:
    mode: exclusive
    paths: [relaycfg/]
  relay:
    mode: exclusive
    paths: [relay/, cmd/relay/, tools/relay-vm/, Dockerfile.relay]
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
  cli:
    mode: exclusive
    paths: [cmd/warden/]
    reaches: [collector, driver-core, driver/container, driver/firecracker, driver/hyperv,
              driver/qemu, driver/vz, executor, ledger, relay, relaycfg]
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
    interface: driver-core
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
| `driver-core` | exclusive | `driver/*.go` and the packages drivers share: `buildlink`, `resolvers`, `relayvm`, `script`, `linuxguest`, `winguest`; the build VM and Windows image tools the guest families use. `driver/` is the pattern, so a new shared package lands here | `rules.yaml`'s `drivers.shared` lists the first five and omits `buildlink` and `resolvers`, which four driver trees and `cmd/warden` import |
| `relaycfg` | exclusive | The claim's per-build configuration | Seam 1's package. Every driver, the relay, the collector and the CLI reach it |
| `relay` | exclusive | `relay/`, `cmd/relay/`, the relay VM image tools, `Dockerfile.relay` | The Go relay, which stays here until M6 |
| `ledger` | exclusive | `ledger/` | The format's reader and writer |
| `collector` | exclusive | `collector/`, `cmd/collector/` | The collector and its reference consumer |
| `executor` | exclusive | `bootstrap/`, `cmd/warden-io/` | The in-environment pieces and the bootstrap configuration they read |
| `cli` | exclusive | `cmd/warden/` | The orchestrator's entry point. Three of the last six product merges changed it, two of them by a file or two beside one driver's tree; such items collide until K1 moves the per-driver flags out |
| `specs` | exclusive | `specs/` and its mirror pages in the book | The ledger wire format and schemas (RELAY-05) |
| `tier1` | exclusive | Tier 1's code | A check's code is changed by few items, each by an approved rule change |
| `loop` | exclusive | `tools/workflow/`, `.claude/`, `.entire/` | The loop's machinery. Process items change it and are outside the relations; a feature reaches it only through `tools/workflow/check/` (FLOW-14) |
| `core` | exclusive | Every path no pattern matches. Empty today | A new top-level tree collides with any other until the design that adds it gives it an area |
| `book` | ordered | The book, `examples/`, and the root documents | Changed by 13 of the last 53 merges, a page or two each |
| `rules` | ordered | `docs/development/` | Changed by 23 of 53. Two items that change the *same rule* are near by its ID, whatever the file |
| `designs` | ordered | `docs/design/`: other items' designs and the template | An item's own design needs no area. This is for a retrofit or an amendment to an archived design |
| `build` | ordered | CI workflows, `Makefile`, `go.mod`, `go.sum`, lint and release configuration, the root `Dockerfile` and its ledger, the install and integration scripts | Changed a line at a time by many items |
| `tier1-config` | ordered | `rules.yaml`, `golangci.yml`, `areas.yaml` | Each item adds its own entry |

**`reaches`, as computed.** From `go list -deps -test` in each of the three
modules, for Linux, macOS and Windows, on amd64 and arm64, with cgo on (so
`driver/vz` is seen), mapping each package's directory to its area:

| Area | Reaches |
|---|---|
| `cli` | `collector`, `driver-core`, all five drivers, `executor`, `ledger`, `relay`, `relaycfg` |
| `collector` | `driver-core`, `executor`, `ledger`, `relaycfg` (`cmd/collector` imports `driver`) |
| `driver-core` | `collector`, `executor`, `ledger`, `relay`, `relaycfg`; and `driver/firecracker`, `driver/hyperv`, `driver/qemu` through `driver/buildlink`'s tests |
| each `driver/<name>` | `collector`, `driver-core`, `executor`, `ledger`, `relaycfg` |
| `relay` | `collector`, `ledger`, `relaycfg` |
| `specs` | `collector`, `ledger`, `relay`, `relaycfg` (its tests) |
| `tier1` | `loop` (`tools/tier1` imports `tools/workflow/check`) |
| `relaycfg`, `ledger`, `executor`, `loop`, `core`, every ordered area | nothing |

`relay` is reached by `driver-core`'s and `cmd/warden`'s tests and by no
driver tree, and it reaches no driver package. That is why a Hyper-V item
and a Go relay item are disjoint.

**The check that the map and the graph agree** is a tier-1 check,
`area-map`, mapped to PROFILE-01 in `rules.yaml`. It is tier 1's, not
`wf`'s, because it needs the Go toolchain and the loop's tool may assume
none (LOOP-08). It runs when the diff touches `areas.yaml`, a `.go` file or
a `go.mod`, and fails when:

- the map does not parse, an area has no `mode`, `catch_all` does not name
  an exclusive area with no paths, or a `reaches` or `implies` entry names
  an area the map does not have;
- a tracked file matches two areas equally;
- the import graph has an edge between two areas that `reaches` lacks, or
  `reaches` lists an edge the graph does not have. The finding names a file
  and the import that makes the edge;
- a driver in `rules.yaml`'s `drivers.all`, or `interface`, has no entry in
  `implies.drivers`.

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
| An item in `relaycfg` and any driver item, the relay or the CLI | near | 9 |
| Two driver items that each also edit `cmd/warden/` (`cli`) | colliding | 6 |
| Two items that amend GLOBAL-08, in different areas (S2 and K1) | near | 8 |
| Two items that change `ledger-format`, in different areas | near | 7 |
| Two items that change `relay-claim`, which has no specification until S1 lands | colliding | 3 |
| An undeclared item and any feature here (`seam-specs` and `driver-contract` today) | colliding | 5 |
| An item whose design adds the area it declares, before its merge | colliding with every feature here | 5 |
| An item here and an item homed in the relay repository, no seam or rule in common | disjoint | 10 |
| K1 here and the relay's M1 (tracked), both on seam 8, which is not written | colliding, shown and not enforced | 3 |
| Two items in a repository with no map (`buildwarden-specs`) | colliding | 4 |
| Any item and a process item (`loop-contract` and `seam-specs`) | outside | 1 |
| Two reconciliation items for two drivers (`driver/qemu`; `driver/vz`) | disjoint | 10 |
| A reconciliation item and a fix in the same driver tree | colliding | 6 |
| Two items the review told apart from what the map says | colliding | 2 |

### The limits

Three limits. A *counted* item is a feature that is not tracked. Each limit
is taken by one kind of claim, held by a state the item's own steps show,
and counts the holders that relate to the item that asks. Only an item that
does not hold is asked, and only holders are counted, so no two items wait
for each other.

| Limit | Taken by | Held while | Counts | At most |
|---|---|---|---|---|
| In progress | The claim that starts a queued feature (a cloud claim today) | The feature is in progress | In-progress features that *collide* with the asker | `limits.in_progress`; 5 when unset |
| `alters_relationships` slot | The claim of a Plan step, by a feature that declares `true` | A Plan step of it is done or carries a claim, expired or not (as `alters-slot` built it) | Holders that *collide* with the asker | `limits.alters_relationships`; 1 when unset |
| PR slot | The claim of `f6-merge-main` | The feature is in Publish and its newest `f6-merge-main` step is done or carries a claim, expired or not; or it waits for the human's merge (phase 7) | Holders that *collide with or are near* the asker | 1 |

Changes from today, beyond counting by relation:

- A claimed `f6-merge-main` holds the slot. Today only a done one does.
- An item in Close holds nothing. Today it holds until Close is done.
- An item that returned from Publish and has entered it again holds the
  slot from the claim of its new `f6-merge-main`, not from re-entry.
- `limits.in_progress`: 5 when unset, an error below 1 (a11).

**The guard.** One function, in a new `queue/limits.go`:

```go
// Guard returns why the worker may not claim the step now, or "" (FLOW-05).
func (s *State) Guard(f *Feature, st *Step, worker string) string
```

It walks one table of the three limits, which is the only place a limit
is written. For each: if this claim would take
the limit and the item does not already hold it, it lists the holders that
relate to the item (`Relation`), and refuses when they number the limit or
more. Its reasons:

- `the in-progress limit (limit 5): a, b, c, d, e collide with it (FLOW-05)`
- `an alters_relationships slot (limit 3), held by a, b, c (FLOW-05)`, the
  text `alters-slot` gave it, and
  `limits.alters_relationships 0 is below 1 (FLOW-05)` as before;
- `the PR slot, held by a (colliding: area cli) (FLOW-05)`, or
  `(near: rule GLOBAL-08)`;
- `the PR slot: a (priority 12) is ahead and ready (FLOW-05)`, the
  tie-break below.

Who calls it:

- `wf next` and `wf next -feature`, in `ready`, for every worker. The two
  refusals that stay the cloud's alone (an open blocking question, a spent
  runaway budget) remain in `cloudHold`; they are FLOW-07's and FLOW-13's,
  not limits. `atLimit`, `prSlotHolder` and the slot test in `cloudHold`
  go.
- `wf claim`, in `claimable`, for every worker. A step the worker already
  holds stays claimable.
- `wf commit`, after a replay (below).

A human claim takes none of the three today: a human step starts no
feature, and Plan and `f6-merge-main` are cloud steps. A host claim is now
bounded: a Plan step whose affinity was edited to a host is refused past
the limit, which closes `alters-slot`'s known gap.

**`wf validate`** walks the same table over the holders: for each limit and
each item that holds it, the holders that relate to it, plus itself, may not
number more than the limit. Its errors name the limit, the item and the
others, for example
`seam-specs holds the PR slot with driver-contract (colliding: driver-contract declares no areas), limit 1 (FLOW-05)`.
For a queue with no declared areas this is today's count: `alters-slot`'s
error text is kept for the `alters_relationships` limit.

**The tie-break at the PR slot** (a12). An item *waits for the slot* when
it is a counted feature in Publish whose newest `f6-merge-main` is pending
with no claim, and it has no open blocking question and no spent runaway
budget. It is *ready* when, in addition, no item that collides with it or
is near it holds the slot. The guard refuses an item the slot while a
related item of higher priority is ready. Priority decides because both
are in Publish, so both are in FLOW-05's class 1. The order is strict, so
the highest-priority ready item is never refused and the slot is always
taken by someone.

**More holders than a limit.** The causes `alters-slot` listed remain
(Jeff lowers a limit; a hand edit). Three new ones are closed at the
source: `wf declare` and `wf relate` refuse a change that would put a limit
over, and a map change is shown against the open queue in its design review
(`wf validate -areas`). A new `reaches` edge can only make two items near,
which the first two limits do not count.

**The cases.** `limits.in_progress` 5 and `limits.alters_relationships` 3,
as in `config.yaml` today. "Today" is `wf` at `897a4a8`.

| # | State | Today | After |
|---|---|---|---|
| 1 | Live now: `loop-contract` in Implement and `loop-parallel` in Design (process); `seam-specs` and `driver-contract` past Design, reviews pending, undeclared | `wf next -worker cloud`: nothing for the cloud. `wf validate`: ok | The same lines. One relation: the two features collide (`driver-contract declares no areas`) |
| 2 | Both features approved, undeclared | `seam-specs` plans, then `driver-contract`: 2 of 3 slots | The same |
| 3 | `seam-specs` has claimed `f6-merge-main`; `driver-contract` reaches its own | Offered: only a done `f6-merge-main` holds | Refused: `the PR slot, held by seam-specs` |
| 4 | `seam-specs` merged and in Close; `driver-contract` at `f6-merge-main` | Refused until Close is done | Offered |
| 5 | A host claims a Plan step past the limit (affinity edited) | The claim stands; the next `wf validate` fails | `wf claim` refuses it |
| 6 | Wave 2: S3, S4 and K1 declare `driver-core`; S2 declares `relay`, `executor`, `rules`, `book`; all four amend GLOBAL-08 | Four altering features, three slots: the fourth waits at Plan. One PR at a time | S3, S4 and K1 collide and hold the three slots. S2 is near each (row 8) and is not counted with them: it plans too. All four take turns at the PR |
| 7 | Wave 4: K3 (`driver/hyperv`, `build`, `book`) and an item in the Go relay (`relay`, `book`) | One PR at a time | Disjoint: both PRs open; either merges first; the other is refreshed |
| 8 | Wave 5: five reconciliation items, one per driver (`driver/<name>`, `book`) | Five features in progress, the limit; one PR at a time; ranked as new functionality | Class 3. Pairwise disjoint: none counts against another, and five PRs may be open |
| 9 | Five undeclared features in progress; a sixth queued | Not started | Not started: all five collide with it |
| 10 | Five declared features in progress, each disjoint from a sixth | Not started | Started |
| 11 | Two workers claim Plan for two colliding items at the last slot | The second push is rejected and its claim lost | The second push is rejected; its replay fails `Validate`; the claim is dropped and the reason names the holders |
| 12 | Two workers claim unrelated steps at the same moment | The second loses its claim and asks again | The second's claim is replayed and stands |
| 13 | Two near items wait at `f6-merge-main`; an attended run names the lower priority | It takes the slot | Refused: the other `is ahead and ready` |
| 14 | An item in Review widens to `cli`, inside its design, while a `cli` item holds the PR slot | n/a | `wf declare` accepts: the item holds no PR slot yet, and it will wait for one |
| 15 | An item with an open PR finds, at its refresh, a path in `cli`; another open PR declares `cli` | n/a | `wf declare` refuses: two colliding items would hold the PR slot. Back to Design |
| 16 | `limits.in_progress` is not set | No limit | 5 |
| 17 | A process item in any state | Outside the limits | Outside |
| 18 | Three disjoint PRs open; Jeff merges one | Cannot arise | `wf stale` seeds a refresh on the other two; both show "behind `main`" |

### `wf commit`: pushing queue state

`wf commit -state DIR -m MESSAGE [-claimed FEATURE/STEP -worker W]` replaces
the `git commit` and `git push origin workflow` in the four skills.

1. It regenerates `QUEUE.md` and the digest pages (below), stages
   everything in the state checkout, and commits. With nothing to commit it
   exits 0.
2. It pushes to `origin workflow`. On success it exits 0.
3. On a rejected push it fetches `origin/workflow`, resets the checkout to
   it, and replays its commit on top with `git cherry-pick --no-commit`. A
   conflict in a *generated* file (`QUEUE.md`, `digest/<date>.md`,
   `areas.yaml`) is settled by taking the remote's copy, since step 5
   writes it again.
4. It reloads the state and runs `Validate`. With `-claimed`, the named
   step must still be pending and leased to `W`.
5. It regenerates the generated files, commits with the same message, and
   pushes again. It tries three times.

What ends it early:

| Outcome | Exit | With `-claimed` | Without |
|---|---|---|---|
| The replay conflicts in a queue file or any other file that is not generated | 4 or 5 | The checkout is left at `origin/workflow`, the claim is gone, and the message is `claim lost: <file> changed under it (FLOW-04)`. Exit 4: the worker asks `wf next` again | The session's commit is restored, and the message names the files. Exit 5: the session re-applies its change by hand, as today |
| `Validate` fails on the replayed state | 4 or 5 | The claim is dropped the same way, and the message is `Validate`'s line, which names the limit and the holders | Exit 5, with the lines |
| Three rejections | 5 | The claim is dropped | The commit is kept locally |

A claim is dropped, and never the other worker's: the state on the remote
was valid, and the replayed claim is the only change. `wf commit` rewrites
only its own unpushed commit and never force-pushes. It pushes only the
`workflow` branch.

### When collision is decided, and checked again

| When | What is checked | By | On failure |
|---|---|---|---|
| Intake | Jeff agrees the repositories and areas. `wf new` records them and `wf relations -feature F` shows what the new item is near or collides with | `workflow-intake`, with Jeff. A follow-up filed with nobody there (FLOW-09) records none and is undeclared | Nothing fails: the areas are a proposal until Design |
| Design's done-when | (1) The queue's `repos`, `areas`, `seams`, `rules_changed`, `alters_relationships` and `design_review` equal the design's (`wf declare` made them so). (2) Each area exists in the branch's map. (3) *Implied areas*: for each driver in `drivers`, its area is declared, or an area it reaches is (a change to a shared package declares every dependent driver, FLOW-02); for each surface in `surfaces`, its areas are declared; each path of a `rule_changes` entry lies in a declared area. An undeclared design skips 2 and 3 | `wf done f1-design` (`check/declared.go`) | The step is released and Design fixes the design |
| The design review | Jeff sees the repositories, the areas and the relations with every open item, and may narrow, widen or name a collision | `workflow-human`; FLOW-19 | Changes, as for any review |
| Plan's done-when | The queue still equals the design (closing the gap `alters-slot` named). `repos` and `areas` are impact fields: a design whose areas changed since its approval fails the Plan gate | `check.go`, `review.go` | Back to Design (FLOW-06) |
| Implement, Review, Publish and the refresh, at each done-when | The queue still equals the design. *The diff stays inside the areas*: every path of `git diff --name-only --no-renames origin/main...<branch>` belongs, by the branch's map, to a declared area, or is the item's own design. An undeclared item passes | `check/areas.go` | See below |
| Review and `f6-consolidate` | *A trial merge*: `git merge-tree --write-tree` of the branch with the remote branch of each in-progress feature that is disjoint from it. It is a report: clean; or conflicts in ordered files, which the later of the two settles at its refresh; or a conflict in an exclusive area, which means one of the two is outside its areas | `check/areas.go`; the lines go into the tier-2 report and the consolidation section | It does not fail the step: an item outside its areas fails its own diff check. The run's digest lists a conflict in an exclusive area under what is blocked (LOOP-06) |

**When the diff leaves the areas**, the agent that finds it does one of two
things, and `wf declare` decides which is allowed:

1. *Widen inside the design.* The missing area is one the approved design
   already implies (its `drivers`, its `surfaces`, a `rule_changes` path).
   The agent adds it to the design's `areas` on the branch and runs
   `wf declare`. Accepted, the item goes on; its relations may have
   changed, and `QUEUE.md` and the digest show them.
2. *Back to Design* (FLOW-06), when `wf declare` refuses: the area is
   outside the design, or the widening would put a limit over. The finding
   is the path and the area. The redesign's areas differ from the approved
   ones, so its review cannot be skipped (FLOW-19).

Narrowing is never an agent's: only a `human` worker's `wf declare` removes
an area or a repository after Design.

### The refresh step

**What it is.** A step named `f6-refresh` (`-r2`, `-r3`, ... as needed) on
a feature that waits for Jeff's merge, and `x2-refresh` on a process item
that does. Affinity `cloud`; the agent is `wf-publish`. It is not a
template step: nothing seeds it at a phase's start.

**When it is seeded.** `wf stale -state DIR -repo DIR` looks at every open
item homed in this repository that waits for the human's merge and has a PR
recorded. If `origin/main` is not contained in `origin/<branch>` and the
item has no pending refresh step, it adds one, sets `pr.ci` to `pending`,
and writes what moved in the step's note: `main` at `<sha>`, the number of
commits, and the files those commits and this branch both change. The tick
runs it right after confirming merges, every run; `workflow-human` runs it
before it lists the open PRs. It is seeded whatever moved `main`: another
item's merge, a PR merged by hand, the weekly tidy.

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
   landed and that nothing overlaps.
4. Refresh the Archive summary if the PR's content changed (FLOW-12); run
   `flow18-check.sh`; push; wait for CI on the new head and set `pr.ci`.

**Its done-when** is Publish's, run again: current `main` merged; the
branch pushed; the Consolidation section names the merged `main` commit;
the rule-change clearance; the design archived on the branch; the PR's CI
green; the diff inside the areas; the queue equal to the design. For a
process item: `main` merged, the branch pushed, the design archived, CI
green. If `main` moved again meanwhile, the check fails, the step is
released, and the next run takes it again.

**What sends the item back.** The pass in step 3 is consolidation, and
FLOW-10 already says what it does with real work: to Implement
(`wf back -to 3`) for work the plan covers, such as a lint failure a merged
check now reports on this branch's code, or to Design (`wf back -to 1`).
A path outside the areas goes through `wf declare`, as above. The refresh
never re-enters Publish, so the item keeps its PR slot; an item that goes
back to Implement or Design gives the slot up, as a return from Publish
does today, and its PR stays open until it is published again.

**When the step goes away.** `wf done` on the merge step drops a pending
refresh step (the PR merged first), and so does `wf back` (Publish merges
`main` again). FLOW-06 gains the clause.

**Budget.** A refresh is a claim like any other and counts one step
against the item's budget (a10). Intake's size guide adds two steps.

### Tracked items

An item whose home is another repository, such as a task of the relay's M1,
cannot be gated: `wf` sees no branch, design or PR there until
`loop-repository`. It is filed so that it shows in the one queue.

- `wf new -repos buildwarden/buildwarden-relay ...` seeds the `tracked`
  round, step letter `t`: `t1-work` (human: the design, the work and the
  review, by Jeff or an attended session, until its PR is open) and
  `t2-merge` (human: Jeff merged it). `wf done` runs no check for either
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
  and shown.

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
git. For today's queue, with `config.yaml`'s `repository` set and no item
yet declaring areas, it renders as:

    # Queue

    Generated by `wf` from `queue/*.yaml` and `areas.yaml`; do not edit. Updated 2026-11-14 20:58 UTC.
    Areas: `buildwarden/buildwarden` at `897a4a8`.

    ## Needs Jeff

    5 items wait on you: 5 design reviews. No merge, question or escalation.

    ### Merges, in order

    Nothing to merge.

    ### Reviews, questions and escalations

    - **loop-parallel**: design review (`x1-design`, FLOW-19) · [design](https://github.com/buildwarden/buildwarden/blob/feat/loop-parallel/docs/design/loop-parallel.md)
    - **loop-native-lead**: design review (`x1-design`, FLOW-19)
    - **loop-repository**: design review (`x1-design`, FLOW-19)
    - **seam-specs**: design review (`f2-design-review`, FLOW-19) · [design](https://github.com/buildwarden/buildwarden/blob/feat/seam-specs/docs/design/seam-specs.md)
    - **driver-contract**: design review (`f2-design-review`, FLOW-19) · [design](https://github.com/buildwarden/buildwarden/blob/feat/driver-contract/docs/design/driver-contract.md)

    ## Active

    | Pri | Item | Kind | Repositories | Areas | Phase | Next steps | Waits for | PR | Steps |
    |---|---|---|---|---|---|---|---|---|---|
    | 60 | loop-contract | process | buildwarden/buildwarden | - | 2 implement | x2-implement (cloud, leased to cloud/20261112T230554Z) | - | - | 2 / 8 |
    | 61 | loop-parallel | process | buildwarden/buildwarden | - | 1 design | x1-design (human, leased to human/jeff) | - | - | 1 / 12 |
    | 64 | seam-specs | feature | buildwarden/buildwarden | undeclared | 2 plan | f2-design-review (human) | - | - | 2 / 30 |
    | 65 | driver-contract | feature | buildwarden/buildwarden | undeclared | 2 plan | f2-design-review (human) | - | - | 4 / 30 |

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

- **Needs Jeff** opens with the count of items waiting on him, by kind
  (a3). It lists the merges first, in FLOW-05's order, each with its PR
  link, `pr.ci`, and "behind `main`" while a refresh step is pending, with
  the overlapping files from that step's note. Reviews, questions and
  escalations follow, each review of an item in progress with a link to
  the design on its branch.
- **Active** gains Repositories, Areas (`undeclared`, or the names, `*`
  after one `main` lacks), the lease holder in Next steps, Waits for (the
  guard's reason for the item's next step, or "your merge"), and PR. Kind
  reads `adoption (<consumable> <version>)` for a reconciliation item and
  `tracked` for a tracked one.
- **Relations** is new: every pair of open features that is near or
  colliding, with the reason. Disjoint pairs are not listed.
- **Backlog** gains Repositories and Areas.
- A **Workers** section (each machine's last run and its led items) is
  `loop-native-lead`'s.

`wf status` prints `repos:`, `areas:` and `adopts:` lines under an item
that has them. `wf relations [-feature F]` prints the relation rows.

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

- `queue.go`: the six fields on `Feature`, with `areas` a pointer to a
  list so that an absent key and an empty list stay apart through a load
  and a save (checked with the YAML library the module uses);
  `Config.Repository`;
  `Limits.InProgress` as `*int` with `inProgressLimit()`; `Validate`'s
  checks of the new fields, and its limit checks replaced by a walk of the
  limits table; `Class` returns 3 for `adopts`; `inImplementation` is true
  for an item with a pending refresh step.
- `areas.go` (new): the map's type, its parser (unknown keys rejected), the
  path match, and the copy's loader in `Load`.
- `relation.go` (new): `Relation(a, b)` and its reason.
- `limits.go` (new): the table, `Guard`, the `Validate` walk, the PR-slot
  holder and tie-break; `holdsAltersSlot`, `altersHolders` and `altersWait`
  move here from `next.go`.
- `next.go`: `Next`, `NextFor` and `ready` call `Guard` for every worker;
  `atLimit` and `prSlotHolder` go; `Claim` starts a tracked item.
- `phases.go`: the `tracked` round and the letter `t`; `IsRefresh`;
  `SeedRefresh`; `Complete` drops a pending refresh on a merge step; `Back`
  drops one.
- `declare.go` (new): the rules of `wf declare` and `wf relate`.
- `commit.go` (new): `wf commit`, with git run on the state checkout as
  `hosts.go` already runs it on the code checkout.
- `digest.go` (new): the day's page.
- `summary.go`: the columns and sections above.

`tools/workflow/check/`:

- `declared.go` (new): reads a design's declarations; the comparison with
  the queue; the implied areas.
- `areas.go` (new): the diff inside the areas; the trial merge.
- `check.go`: `Front` gains `Repos` and `Areas`; Design's done-when calls
  the declared checks; the later phases call the comparison and the diff
  check; `Refresh`; a tracked item's result.
- `review.go`: `impactDelta` includes `repos` and `areas`; a skip is
  invalid when Design narrowed.

`tools/workflow/cmd/wf/main.go`: `areas`, `declare`, `relate`, `relations`,
`stale`, `commit` and `digest`; `new -repos -areas -adopts`;
`validate -areas`; `claimable` calls `Guard`; `done` runs `check.Refresh`
for a refresh step; `status` prints the new lines.

`tools/tier1/`: `areas.yaml`; `tier1.go` embeds it; `rules/rules.go` lists
`area-map`; `rules.yaml` maps PROFILE-01 to it; `coupling/areamap.go` is
the check, and uses the parser in `tools/workflow/queue`, which tier 1
already depends on through `tools/workflow/check`.

### Machinery edits

By file. Each cites the rule and does not restate it (FLOW-14).

- `.claude/skills/workflow-tick/SKILL.md`. Step 1: `dist/wf areas -sync`
  after the fetch. Step 2: after confirming merges, `dist/wf stale`; claim
  and state pushes through `dist/wf commit` (exit 4: ask `wf next` again;
  exit 5: re-apply by hand); the dispatch table gains `f6-refresh` and
  `x2-refresh` for `wf-publish`; a `wf done` that finds its refresh step
  dropped by a merge carries on. Step 4: the run's own digest file, then
  `dist/wf digest`.
- `.claude/skills/workflow-intake/SKILL.md`. Step 3: agree the
  repositories and areas with Jeff (`dist/wf areas -list`), show
  `dist/wf relations -feature <slug>` after `wf new`; `-adopts` and its
  place; a tracked item; two steps on the size guide for refreshes. The
  follow-up path: no areas, so undeclared.
- `.claude/skills/workflow-human/SKILL.md`. The review's overview gains the
  repositories, the areas and the relations with open items, and
  `dist/wf validate -areas` for a map change; narrowing and `wf relate` on
  Jeff's word; **Open PRs** confirms merges, runs `dist/wf stale`, and
  lists the merges in order with "behind `main`"; a tracked item's steps.
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
  merge; `f6-refresh` and `x2-refresh` as above.

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
  field is written at Close (`x4-close`), after the step has checked that
  no live lease was claimed before the merge commit's time, and attended
  sessions rebuild `dist/wf` before their next command, as after
  `alters-slot`.
- **An old skill beside a new one.** A session that started before the
  merge appends to `digest/<date>.md` by hand; the first `wf digest` moves
  that page into the day's directory and builds the page from it. A
  `tracked` step id fails an old `wf validate` loudly; none exists before
  Close.
- **`loop-contract` first.** This item amends files that `loop-contract`
  adds. `x2-implement` starts by merging `origin/main` and releases the
  step, naming the reason, if `docs/development/loop-contract.md` is not
  there. Both PRs edit `process.md`, `TEMPLATE.md`, `workflow-intake` and
  `cmd/wf/main.go`; the second to merge is brought up to date by its own
  refresh step once this item has landed, and by a hand merge of `main`
  before then.

### Safety

- **FLOW-08.** Nothing here merges, approves or enables auto-merge. More
  PRs are open at once and each is still Jeff's to merge. "Merges, in
  order" is the order FLOW-05 gives; he may merge in another.
- **FLOW-16.** Close is per item and unchanged. A refresh adds commits to a
  branch that is still open and deletes nothing.
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
  4. `make lint`, `make test` and `make deadcode` at `f6-merge-main` and at
     each refresh, on the branch with `main` merged.
  5. GitHub, which refuses a merge that conflicts.
  6. CI on the push to `main`. A failure there is a regression on `main`:
     its fix is filed as the follow-up of the item that merged last
     (FLOW-09) and goes to the top of the fixes.
  7. The native sweep, which `wf gate` starts when `main` has moved, and
     the full matrix before a release (TEST-02). CI runs neither Hyper-V
     nor VZ, so a break between two disjoint items that only a native host
     shows is found here.

  What reaches 6 or 7 is a semantic break between two items whose
  declarations were true: no shared file and no import between them. That
  is the risk the direction accepted; the map cannot remove it.
- **Merge order.** Related PRs are never open together, so order matters
  only among disjoint ones, where any order is safe by construction and a
  PR that is behind `main` says so.

### What this item hands on

**To `loop-native-lead`.**

- The guard applies to every worker and to `wf claim`, so a machine that
  claims an item's first step, its Plan step or `f6-merge-main` is bounded
  by the same limits as the cloud. Its design adds `lead` and decides which
  steps a machine takes; `f6-refresh` is seeded `cloud` here and its design
  may leave it so.
- The PR-slot tie-break means a led item of lower priority waits for a
  cloud-led one that is ready. Its design says whether that is wanted.
- `wf commit`, for a machine's state pushes, and a digest file per run, so
  a machine and the cloud no longer write one file.
- FLOW-04 is amended here in two sentences only (the replayed claim; the
  human step for a repository with no gate). Its host-name and lease
  clauses are untouched.
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
  `wf areas -sync` and `wf stale` run per repository. `tools/tier1/areas.yaml`
  is one more path `wf` hard-codes that the profile should supply (its
  hand-off point 4).
- A hold that names repositories (a9), and a `human` merge-recording step
  that is not offered before its phase (`seam-specs`' a15 asks; a `human`
  step is still offered early).
- What a tracked item of a private repository may show in the public
  queue, which is its a17.

### Departures from the proposal

Where this design does not do what the proposal's sections 3 to 8 say.

| The proposal | This design | Why |
|---|---|---|
| Sharing only ordered areas makes two items near: one PR at a time | They are disjoint; the refresh orders them (a2) | Most pairs of product features share a book page |
| "`tier1`, `rules` and `tier1-config` reach every area" | `reaches` is the import graph, exactly | Decision 5 |
| A form for the Go relay "(`relay`, which reaches every driver) would be near" a Hyper-V item | Disjoint | The code: no driver tree imports `relay/`, and it imports none |
| `wf areas -check` compares the map with `go list` | Tier 1's `area-map` does; `wf areas -check` compares the copy with `main` | The loop's tool assumes no language (LOOP-08) |
| The map "on `main` under FLOW-14" | On `main`, with a generated copy in the queue state | `wf next` reads no git |
| A refresh that finds overlap "queues `f6-consolidate` again" | The refresh carries the pass itself | Re-entering Publish would give up the PR slot |
| A refresh "when a PR merges" | Whenever `main` is not contained in the branch | `main` also moves from outside the queue |
| `wf check` fails when areas do not cover what `seams` imply | Seams are not mapped to areas; the relation reads them | A design may list a seam it does not edit the code of (`driver-contract` lists two "conservatively") |
| An agent may widen within the design's `drivers` and `seams` | Within its `drivers`, `surfaces` and `rule_changes` paths, and only while `wf validate` passes | The same reason; and a widening can put a limit over |
| The trial merge fails the step | It is a report | The item at fault fails its own diff check |
| Seams count across repositories | They do, in the relation; a tracked item is shown and not counted (a7) | `wf` sees nothing of a tracked item |
| "A hold may name repositories" | Not yet (a9) | No worker step exists there to hold |
| `wf renew` | Left to `loop-native-lead` | It belongs with the lease change |
| FLOW-17: "a `feat/<item>` branch in each" repository | Not here | `loop-repository` |
| The exclusive areas include `conformance` | Not in the map | `tools/conformance/` is not on `main`; `seam-specs` adds it |
| FLOW-02, -03, -05, -10, -15, -19, -20 | Also FLOW-04, -06, -08, LOOP-01, LOOP-04, PROFILE-01 | Decision 19 |

### What this item does not do

- No `lead`, no host names, no change to the native wrapper, the push
  script or the lease: `loop-native-lead`.
- No gate, branch, PR or check in a second repository, and no `work` list:
  `loop-repository`.
- No second cloud run a day (the proposal's question 4), and no change to
  FLOW-13.
- No change to the freeze.
- No area for a tree that is not on `main`. `seam-specs` and
  `driver-contract` stay undeclared, and so collide, until Jeff gives them
  areas in an attended session or their designs declare them.
- No automatic answer to a reconciliation item's question.

## Defaults and overrides

| Default, override or fallback | What it is | Why, and what overrides it |
|---|---|---|
| `repos` absent | The item works only in `config.yaml`'s `repository` | LOOP-04: "an item that names none works only in the repository that holds its design" (GLOBAL-09). A written list is used exclusively (GLOBAL-10) |
| `config.yaml`'s `repository` absent | No default. An item that names `repos` then fails `wf validate` | A guessed repository name would be a silent fallback (GLOBAL-10, GLOBAL-11) |
| `areas` absent | Undeclared: the item collides with every item it shares a repository with | Today's behaviour, and the safe side (GLOBAL-09). `areas: []` is a declaration, not an absence |
| *Named fallback: "undeclared until merged".* `areas` names an area `main`'s map lacks | The item is treated as undeclared | Its design adds the area and `main` has it only after the merge. `QUEUE.md` marks the name with `*` (GLOBAL-11) |
| *Named fallback: "no map".* The queue state has no `areas.yaml`, or a repository has none | The repository is one exclusive area: every two items in it collide | Today's behaviour (GLOBAL-09). `QUEUE.md`'s Relations gives it as the reason (GLOBAL-11) |
| `seams`, `rules_changed`, `collides_with`, `adopts` absent | None; not a reconciliation item | The obvious reading |
| `limits.in_progress` absent | 5; a written value below 1 is an error | FLOW-05's written limit (a11). *Changed:* absent or 0 was "no limit" |
| `limits.alters_relationships` absent | 1; below 1 an error | Unchanged (`alters-slot`) |
| The PR slot | 1, not configurable | FLOW-05 |
| A map area's `mode` | Required; no default | Exclusive by default would hide a missing line; ordered by default would be unsafe |
| A map area's `reaches`, `paths` absent | Reaches nothing; `paths` may be absent only for `catch_all` | The obvious reading |
| A design's `repos` empty or absent | Only the repository that holds the design | LOOP-04 |
| `wf new` without `-repos`, `-areas`, `-adopts` | Home repository; undeclared; not a reconciliation item | As the fields |
| `wf commit`'s tries | 3, fixed | Enough for a busy minute; then the session decides |
| A reconciliation item's place | Just below the open fixes; Jeff's word overrides | a8 |
| *Named fallback: "earlier page".* A hand-written `digest/<date>.md` when the day's directory first appears | Moved into the directory as its first file, once | So `wf digest` never overwrites a page it did not build (GLOBAL-11: the page's first section shows it) |

**Dropped fallbacks.** `limits.in_progress` read as no limit when absent or
0; a claim lost on every rejected push; the PR slot held through Close.

**Merges.** `wf areas -sync` unions `areas.yaml`'s `written_seams` with the
names in `rules.yaml`'s `seams` list. Neither overrides the other: a seam
in either is written, and the copy lists the result.

## Cross-driver impact

None: no driver, seam or user-facing surface changes. The area map names
each driver's tree; it describes them and changes nothing in them.

## Rule changes

Thirteen, each for the design review to approve by name (FLOW-14, FLOW-19).
"Current wording" is `docs/development/process.md` on `main` at `897a4a8`
for the FLOW rules. For LOOP-01, LOOP-04 and PROFILE-01 it is the text
approved at `7af50ff` in `loop-contract`'s review, read from
`feat/loop-contract` at `9fbc8e9`; those files reach `main` with that
item's merge, which this item's Implement waits for. Where a rule has
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

> An item whose home is a repository in which the loop's gates do not run
> is **tracked**. The queue records it, its repository and its pull
> request; its two steps, the work and then the merge, are the human's
> (FLOW-04); and no done-when check runs for it. FLOW-05's limits neither
> count it nor hold it.

**Severity and check:** MUST, `automated`, before and after.
**Enforcement:** `tools/workflow/check/check.go`, `declared.go` and
`areas.go` (new): the comparison at phases 2 to 6, the diff check at phases
3, 4 and 6, the refresh's done-when, and a tracked item's one-line result.

### FLOW-04 (amend), `docs/development/process.md`

**Why:** a tracked item's steps, and the step that records a merge in a
second repository (`seam-specs`' a15), are `human` steps that the list does
not name. `wf commit` replays a rejected claim, so "means the claim lost"
is no longer always true; and the limits now bind every claim.

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
> push of a claim is rejected because the branch moved, the claim is
> replayed on the new state: it stands if the step is still free and the
> limits still allow it, and otherwise it is lost and the worker re-reads
> state. An expired lease may be reclaimed.

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
> depends on code in B. A repository with no map is one exclusive area, and
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
> **The limits.** Each counts only what other items already hold, and an
> item that holds is never refused, so no two items wait for each other.
> Process items are not counted because they are short and already ranked
> ahead of fixes and features; one that would disrupt running work declares
> `holds`.
>
> - **In progress.** A queued feature starts only while the in-progress
>   features that collide with it number fewer than the limit.
> - **`alters_relationships`.** A feature that declares
>   `alters_relationships: true` (FLOW-02) holds a slot from the claim of
>   its Plan step until it is done, a return to Design included. It takes
>   its Plan step only while the features that hold a slot and collide with
>   it number fewer than the limit, because two such features reshape the
>   same structure and make each other's consolidation costly. A feature
>   whose design discovers it alters relationships waits at the end of
>   Design until a slot is free.
> - **The PR slot.** A feature holds the PR slot from the claim of the step
>   that merges `main` at the start of Publish until its PR merges. It
>   takes that step only while no feature that collides with it or is near
>   it holds the slot; when two such features wait, the higher priority
>   goes first. A feature finished through Guide waits for the slot, so its
>   consolidation runs against a `main` that the items related to it leave
>   alone until its PR merges. A disjoint item may merge meanwhile, and the
>   refresh (FLOW-10) then brings the open PR up to date.
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
> entry in the consolidation section. A refresh that finds work beyond
> conflict fixes and wording sends the feature back as above. A process
> item's open PR is refreshed the same way, without the review.

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

> | Areas | `tools/tier1/areas.yaml`: the areas, their paths and modes, which area reaches which, the area each `drivers` and `surfaces` value stands for, and the seams that have a specification beyond the `seams` list of `tools/tier1/rules.yaml`. |

**Severity and check:** MUST before and after; the check changes from
`agent-judgment` to `automated + agent-judgment`. **Enforcement:** the new
file `tools/tier1/areas.yaml` (the map above); `tools/tier1/tier1.go`
embeds it; `tools/tier1/rules.yaml` gains `PROFILE-01: {check: [area-map]}`;
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
| LOOP-04: a regression's fix is filed "at the top of the fixes (FLOW-09)" | Yes. Class 3 keeps the fixes; adoptions go below them |
| LOOP-04: "An item that waits on another component waits for a release (LOOP-07): it is held with a blocking question that names the release (FLOW-07), and resumes when the release exists." | Yes. Class 3 cites it for when a reconciliation item starts |
| LOOP-04: "Until the queue records them, it names them in its design" | Amended above |
| LOOP-05: "Every claim records the worker ... and the item, and through the item the repositories the work is in (LOOP-04)." | Yes, and now through a queue field |
| LOOP-05: "FLOW-04 says which worker may take a step." | Yes. The two amended sentences change what a claim is checked against and what a rejected push does, not who may take a step |
| LOOP-06: "Each unattended run appends one section to the day's digest (FLOW-13). There is one digest, for work in every repository." | Yes. A run's file is its section; `wf digest` builds the one page |
| LOOP-06, item 1: each step's "kind and FLOW-05 class" | Yes. Class 3 is now named "Fixes and adoptions" |
| LOOP-06, item 5: "everything blocked or parked, and why" | Yes. The guard's reasons are `wf next`'s `blocked` lines |
| LOOP-07, point 2: a reconciliation item "is a feature item (LOOP-04), filed through intake, and FLOW-05 gives its place in the order." | Yes. This is the amendment its Decision 7 left to this item |
| LOOP-08: "No loop rule may require a language, a build tool, ... a directory layout beyond what the profile names" | Yes. The map is a profile term; the import-graph check is the component's (tier 1); `wf` reads a copy of the map |
| LOOP-09: a branch "follows the old text until it merges trunk, which FLOW-05 has it do while it waits and Publish does at its start" | Yes. FLOW-05 keeps that sentence; the refresh adds a third occasion |
| LOOP-09: a process item "declares the workers it holds (FLOW-05)" | Yes. A hold still names workers (a9) |
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
    paths) and `TestAreaMapMatch` (directory, file, `*`, the longest prefix,
    a tie, the catch-all).
  - `TestAreasAbsentAndEmpty`: an absent `areas` and `areas: []` survive a
    save and a load as two different things.
  - `TestGuardInProgressPerSet` (cases 9, 10, 16), `TestGuardAltersSlot`
    (cases 2, 6; `alters-slot`'s tests keep passing on undeclared items),
    `TestGuardPRSlot` (cases 3, 4, 7, 8: a claimed step holds; near and
    colliding block; disjoint does not; Close holds nothing; re-entry holds
    from the new claim), `TestGuardPRSlotTieBreak` (case 13, and that the
    highest-priority ready item is never refused).
  - `TestGuardEveryWorker` (case 5): it replaces the host half of
    `TestAltersSlotCloudOnly`, which asserts today that the host's claim
    stands.
  - `TestGuardNoMutualHold`: over generated queues, a holder is never
    refused and some waiting item is always offered a free slot.
  - `TestValidateLimitsPerSet`: each limit, at and over, names the item and
    the others; `TestInProgressLimitDefault` (absent, 5, 0, -1, as `Load`
    reads them).
  - `TestNextUnchangedOnUndeclaredQueue`: a fixture of today's queue files
    gives the `next`, `blocked` and `validate` lines `wf` at `897a4a8`
    gives.
  - `TestClassAdopts` and `TestAdoptsValidate`; `TestTrackedRound` (seeded
    steps, any claim starts it, done without a check, outside every limit,
    never class 1).
  - `TestRefreshStep`: seeded once; class 1 while pending; dropped by the
    merge step's completion and by `Back`; its completion leaves the phase
    alone.
  - `TestDeclare`: each row of the `wf declare` table; a widening that
    would put a limit over is refused and changes nothing (cases 14, 15);
    `TestRelate` (human only; refused over a limit).
  - `TestCommitReplay`, on temporary git repositories: a rejected push of a
    done step is replayed and pushed; a replayed claim that fails
    `Validate` is dropped (case 11) and one that passes stands (case 12); a
    conflict in a queue file gives exit 4 with `-claimed` and 5 without; a
    conflict in `QUEUE.md` is regenerated; nothing is force-pushed.
  - `TestSummary`: `QUEUE.md` for the two states shown under **What Jeff
    sees**, as golden files; `TestDigestPage` (file order, the earlier
    page, the same input giving the same page).

  `tools/workflow/check`:
  - `TestDeclaredAtDesign` (queue against design; each implied-areas rule;
    an undeclared design), `TestQueueMatchesDesignLater` (a queue whose
    `alters_relationships` was written back fails Plan, Implement, Review
    and Publish), `TestInsideAreas` (inside; outside; the item's own
    design; a deleted and a renamed path; undeclared), `TestTrialMerge`
    (clean; ordered; exclusive; a branch that is not on the remote),
    `TestRefreshDoneWhen`, `TestImpactDeltaAreas`, `TestSkipNarrowing`,
    `TestTrackedCheck`.

  `tools/tier1`:
  - `TestAreaMapCheck` on fixtures (a missing edge, a stale edge, a tie, a
    driver with no `implies` entry) and `TestRealAreaMap`, which runs the
    check on the repository's own map and tree, as `TestRealIndex` does for
    the rule index.
- **The live queue.** With `main`'s `dist/wf` and the branch's `wf` built
  to a scratch path, on a copy of the `workflow` checkout: `wf validate`
  prints `ok` both times; `wf next -worker cloud/check` and
  `wf next -worker host:macos-vz/check` print the same lines; the copy's
  `QUEUE.md` differs only by the new columns and sections. Then, on the
  copy, with `repository` set and `wf areas -sync` run: `seam-specs` given
  areas by a human `wf declare`, and the relations printed. The outputs go
  in the PR description.
- **The map.** `make lint` runs `area-map` on the branch. The script that
  produced the tables above is re-run and its output compared with the
  shipped `reaches`.
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

1. Merge `origin/main`. If `docs/development/loop-contract.md` is not on
   it, release the step with that reason: tasks 14 and 15 amend files
   `loop-contract` adds (FLOW-14, LOOP-09).
2. Tests first: each of tasks 3 to 12 starts with the tests the
   Validation plan names for it, failing on the current code (TEST-01,
   TEST-02).
3. `queue/areas.go`, `tools/tier1/areas.yaml` and tier 1's `area-map`
   check with its `rules.yaml` entry (PROFILE-01, LOOP-08, LINT-02).
4. The queue's fields, `Config.Repository`, and `Validate`'s schema checks
   (FLOW-02, FLOW-03, GLOBAL-10).
5. `queue/relation.go` (FLOW-05).
6. `queue/limits.go`: the table, `Guard`, the `Validate` walk, the
   in-progress default; `next.go` and `cmd/wf` call it; `alters-slot`'s
   functions move (FLOW-04, FLOW-05, GLOBAL-09, GLOBAL-10).
7. `Class` for `adopts`, `wf new -repos -areas -adopts`, and the `tracked`
   round (FLOW-03, FLOW-05, LOOP-07).
8. The refresh: `IsRefresh`, `SeedRefresh`, `wf stale`, the drops in
   `Complete` and `Back`, class 1 while pending, `check.Refresh` (FLOW-03,
   FLOW-06, FLOW-10, FLOW-15).
9. `check/declared.go`, `check/areas.go`, `review.go`, and `wf declare`,
   `wf relate`, `wf relations` (FLOW-02, FLOW-03, FLOW-06, FLOW-19).
10. `queue/commit.go` and `wf commit` (FLOW-04, FLOW-13, FLOW-17,
    FLOW-18).
11. `summary.go`, `queue/digest.go`, `wf digest`, `wf status` (FLOW-13,
    LOOP-06).
12. `wf areas -sync | -check | -list` and `wf validate -areas` (FLOW-05,
    PROFILE-01).
13. `docs/development/process.md`: the ten FLOW amendments, word for word
    as approved (FLOW-14, FLOW-19).
14. `docs/development/loop-contract.md` and `loop-profile.md`: LOOP-01,
    LOOP-04 and PROFILE-01 as approved (FLOW-14, LOOP-09).
15. `docs/design/TEMPLATE.md`: the two front-matter lines and the sentence
    under **Design** (FLOW-02, LOOP-04).
16. The four skills and four agent definitions, as listed under
    **Machinery edits** (FLOW-14).
17. `make test`, `make lint`, `make enforcement`, `flow18-check.sh`, and
    the live-queue comparison (FLOW-17, FLOW-18).
18. Add `loop-contract.md` to this design's `related`, archive the design
    with its summary, and open the PR (FLOW-12).

**Files the PR changes.** `tools/workflow/queue/`: `queue.go`, `next.go`,
`phases.go`, `summary.go` and their tests, and the new `areas.go`,
`relation.go`, `limits.go`, `declare.go`, `commit.go`, `digest.go` with
tests. `tools/workflow/check/`: `check.go`, `review.go` and their tests,
and the new `declared.go` and `areas.go` with tests.
`tools/workflow/cmd/wf/main.go`. `tools/tier1/`: `areas.yaml` (new),
`rules.yaml`, `tier1.go`, `rules/rules.go`, `coupling/checks.go`, and the
new `coupling/areamap.go` with its test. `docs/development/process.md`,
`loop-contract.md`, `loop-profile.md`. `docs/design/TEMPLATE.md` and this
design. `.claude/skills/workflow-{tick,intake,human,native}/SKILL.md` and
`.claude/agents/wf-{design,implement,review,publish}.md`. Nothing in the
product: no driver, `relay/`, `cmd/`, the book or `specs/`.

**After the merge, at `x4-close`, on the `workflow` branch** (outside the
PR), once no live lease was claimed before the merge commit's time:

- `config.yaml`: `repository: buildwarden/buildwarden`; the comment on
  `limits.in_progress` becomes "5 when unset". The values do not change.
- `wf areas -sync`, which adds `areas.yaml`.
- `README.md`: the layout table (`areas.yaml`; `digest/<date>/`); the
  queue file's new fields; the `tracked` round; `wf next`'s paragraph (the
  guard, for every worker, and the three limits by relation); the commands
  (`wf commit` in place of the `git` lines).
- `seam-specs`' `repos`, on Jeff's word in an attended session, since its
  design already names `buildwarden/buildwarden-specs`. Areas for
  `seam-specs` and `driver-contract` are his to give then or leave.

## Design review

No review yet. This draft is put to Jeff as the design step, `x1-design`.

## Consolidation

A process item has no Publish phase and no consolidation pass (FLOW-03).
The implement step merges current `main` before it opens the PR, and after
this item's merge a process item's open PR is refreshed like a feature's.

## Rollback notes

None.

## Archive summary

Filled in when the item is implemented (FLOW-12).

