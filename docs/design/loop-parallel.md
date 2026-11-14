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
   branch at the start of every session.
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
    `wf commit`, the refresh step and tracked items make untrue as written;
    LOOP-01, LOOP-04 and PROFILE-01, for the Areas term and the queue's new
    field. The last three need `loop-contract` on `main`, so `x2-implement`
    starts after its merge (Implementation plan, task 1).
20. **The freeze stays.** FLOW-20 loses only its Limit bullet. This is the
    proposal's question 2, option (a), part of the approved direction.

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
  areas and 5 ordered ones. Proceeding with it as written; confirm, or
  change an area. The choices a reader might make otherwise:
  - `driver-core` is one area: `driver/*.go`, `buildlink`, `resolvers`,
    `relayvm`, `script`, `linuxguest`, `winguest`. Finer (the guest families
    apart) gives more items that are near and fewer that collide.
  - `relay`, `relaycfg`, `ledger`, `collector` and `executor` are five
    areas, not `rules.yaml`'s one shared list.
  - `cli` (`cmd/warden/`) is exclusive, so two driver items that each edit
    it collide until K1 moves the driver flags out.
  - The book, `docs/development/`, the build files, other items' designs
    and tier 1's configuration are ordered.

  Recommended: as written.
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

<!-- MORE -->
