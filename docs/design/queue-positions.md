---
feature: queue-positions
status: draft
drivers: []              # the loop's queue code and skills; no driver tree is reached
seams: []                # no GLOBAL-08 seam changes
surfaces: []             # `wf` is the loop's tool, not a user-facing surface of the product
repos: []                # only buildwarden/buildwarden
areas: [rules, loop, designs]  # process.md (FLOW-05); tools/workflow/queue and cmd/wf, the
                         # workflow-intake and workflow-human skills; this design
security_boundary: none
alters_relationships: false  # no product component's role or interaction changes (FLOW-02)
design_review: required  # a process item's design always gets its review (FLOW-03, FLOW-19);
                         # it also has a rule change (FLOW-14)
native_validation: []    # pure functions of the queue state and git plumbing, the same on every host
tier2_gaps: []
rules: [FLOW-03, FLOW-04, FLOW-05, FLOW-09, FLOW-10, FLOW-13, FLOW-14, FLOW-19, LOOP-04,
        LOOP-09, GLOBAL-09, GLOBAL-10, GLOBAL-11, TEST-01, TEST-02]
rule_changes:
  - {rule: FLOW-05, change: amend, paths: [docs/development/process.md,
      tools/workflow/queue/queue.go, tools/workflow/queue/commit.go,
      tools/workflow/queue/migrate.go, tools/workflow/cmd/wf/main.go]}
concepts: [workflow, queue, priority, queue-order, queue-position, renumbering, done-folder,
           wf-new, wf-move, wf-commit, replay, history-noise, retrospective, process-item,
           old-wf-compatibility]
related: [regression-fixes-follow-up.md, budgets-active-only.md, queue-summary.md,
          workflow-streamline.md, alters-slot.md, workflow-bootstrap.md]
---

# Priorities are the open items' queue positions, and done items leave the numbering and the queue folder

A process item (FLOW-03, LOOP-09), filed on Jeff's word on 2026-11-17 and
placed first. It holds no worker (a8): the one change an older `wf` would
misread, the move of done items to `queue/done/`, is made at `x4-state`
once every session running an older `wf` has ended.

Words used throughout. **wf** is the loop's tool, `tools/workflow/`. **The
order** is FLOW-05's: class, then in progress before queued, then priority.
A **position** is an open item's 1-indexed place in the order. **Old wf**
is a `wf` built from `main` before this item merges. Current wording and
code are `main` at `0cd3c02`; queue state is the `workflow` branch at
`6433c05`.

## Problem

Jeff wants an item's `priority` to be its position: the first open item in
the order is 1, the next 2, with done items outside the numbering and out
of `queue/`. Today none of that holds:

- **Numbers are kept by hand.** `wf new` shifts every item at or below the
  new one's number down by one (`queue.go:507-534`, `Add`), done items
  included, and nothing renumbers when an item starts, changes class or
  closes. On 2026-11-17 the open items were renumbered by hand to 1-15 and
  the 45 done items set aside to 1000 + their old number, each with a
  history line, until this item lands.
- **The hand numbering is not the order.** `wf status` at `6433c05` lists
  `loop-parallel-rule-wording` (3), `loop-native-lead` (4) and
  `loop-repository` (5) before `queue-positions` (1) and
  `loop-streamline-2` (2): the first three are process items in Implement
  (phase 2), so class 1, and come before the class-2 queued process items
  whatever their numbers (`process.md:211-214`, `Class`,
  `queue.go:474-504`). A number the order overrides is the kind
  of drift Jeff asked to end. (The order itself is right; only the numbers
  disagree with it.)
- **Uniqueness spans done items.** `Validate` refuses two items with one
  number across the whole queue (`queue.go:663-672`), so a done item holds
  its number for ever, and every `wf new` above it shifts it and logs it.
- **The history is noisy.** 232 of the 1,956 history lines in `queue/` are
  priority changes; 156 of them are `wf new`'s "priority N -> N+1
  (renumbered by wf new: X queued at P)", one per shifted item, done items
  included.
- **Done items crowd `queue/`.** 45 of its 60 files are done. `wf` reads
  them where it needs them: `follows` (`validateFollows`, `waitsForParent`),
  `-follows` at intake (`Inherit`), and `wf status -all` (FLOW-13's
  retrospective). A done folder needs `wf` to read it there.

## Prior art

- **regression-fixes-follow-up** (archived): made the order mechanical,
  computed at each read (`Ordered`), with `priority` ordering items within
  a class only. Its Decision 1 rejected "re-sorting after every status or
  phase change" because every mutator would have to remember it. This
  design keeps the order computed at read and the sort key unchanged, and
  answers that objection with one hook, `State.Save`, which every mutating
  command already calls (Decision 1). It also rejected "priority numbers
  that intake sets by the rule, with no code change"; this design is the
  code change that makes the numbers follow the order.
- **budgets-active-only** (archived): done items are left out of `wf
  status`, `QUEUE.md`, the digest's lines and the runaway check, and seen
  again only in a retrospective (FLOW-13). This design keeps that and moves
  their files.
- **queue-summary** (archived): `QUEUE.md` is generated from the state and
  committed with it by `wf commit`; the same generation step is where this
  design catches what no command renumbered (Decision 1).
- **workflow-streamline** (archived): `wf migrate`, a one-off state
  migration run by hand after a merge. The layout migration here is a
  second one (`wf migrate -done`).
- **alters-slot** (archived) and **loop-native-lead** (approved, unmerged,
  `origin/feat/loop-native-lead` at `dbd3032`): a state change an old `wf`
  would misread is made at a `human` `x4-state` step once no worker runs an
  old `wf` (`loop-native-lead.md`, "Holds, and what the merge does to
  running work"). This design uses the same kind of step, with a stronger
  precondition, because its old-`wf` failure is not on the safe side
  (Decision 8).
- **LOOP-04**: "Priority is the human's. An agent places an item only where
  a rule says so ... No agent otherwise reorders items." Renumbering
  changes numbers, never the order, so it places nothing; `wf move` is
  human-only (Decision 5).

## Decisions

1. **wf renumbers in `State.Save`, and `wf commit` renumbers whatever no
   command did** (a1). `Renumber` sorts the open items in the order and
   sets each one's `priority` to its position. `Save(f)`, which every
   command that changes state calls, runs it and writes `f` and every item
   whose number moved, so the state is in positions as soon as a command
   returns. `wf commit`'s generation step (`generate`, `commit.go`) runs it
   again, for what no command wrote: a hand edit (Jeff's word on `follows`
   or `kind`), an old `wf`'s write, and a replay (Decision 11). Renumbering
   is idempotent and never changes the order: after it, the sort key's
   third field equals the position, so sorting again gives the same list.
   Every event Jeff named is covered, because each is a command that saves
   (`wf new`, `wf move`, a claim that starts an item, `wf done` changing a
   phase or closing, the merge step's completion) or a hand edit that is
   committed.
   - **Rejected: renumber only at `wf commit`.** `wf status` and `wf
     validate` would show stale numbers between a command and its commit,
     and `wf new` could not print where the item went.
   - **Rejected: renumber at each named event, in `Add`, `Claim`,
     `Complete`, `Back`.** The objection regression-fixes-follow-up raised:
     every mutator must remember it, and a hand edit is missed.
2. **A renumber writes no history line** (a2). The number is derived from
   the order, like `QUEUE.md`; the events that move an item already have
   their lines ("started (FLOW-05)", "entered phase N", "closed; feature
   done"), and the workflow branch's git log keeps every value. A
   deliberate placement logs one line, on the placed item only: "queued at
   position K" (with "(asked P)" when they differ) and "moved to position
   K from M (asked P): <why>".
   - **Rejected: one compact line on the item whose change caused the
     renumber.** It repeats the line that change already wrote.
   - **Rejected: a line per moved item, as today.** The noise Jeff named.
3. **The 232 existing priority lines stay** (a3). History is a record;
   the done ones leave `queue/` with their items, which takes 166 of them
   out of sight.
4. **`-priority P` is the position asked for, kept as nearly as the order
   allows** (a4). `wf new` and `wf move` place the item ahead of the item
   now at P, clamped to the stretch of the order the item may occupy: after
   every item of an earlier class or, in its class, of an earlier status,
   and before every item of a later one. A queued class-4 item asked at 1
   while classes 1 to 3 hold 9 items lands at 10. `wf new` prints the
   position, and its history line says what was asked.
   - **Rejected: refuse a position the order does not allow.** Intake would
     have to count the classes itself, which is what the order is for.
5. **A new human-only command, `wf move -feature F -to N [-why "..."]`,
   replaces hand-edited numbers** (a5). With renumbering, a hand edit that
   sets `F` to 3 ties with the item already at 3 and is resolved by the
   tie-break (Decision 10), not by Jeff's intent. `wf move` places as
   Decision 4 says, renumbers, and logs one line on `F`. It refuses a worker
   that is not `human/<name>` (LOOP-04) and a done item. `workflow-human`
   step 1 uses it in place of "renumber the `priority` fields as he says".
   - **Rejected: keep hand edits, and let a moved number win ties.** Whether
     a number was moved is not in the state, only in the diff.
6. **Done items live in `queue/done/<feature>.yaml`, carry no `priority`,
   and list in the order they closed** (a6). The file keeps the feature's
   name, which `Load` checks against the file name, and which every design
   and skill uses. `wf status -all` lists done items after the open ones,
   ordered by the time of the last `done <step>` line in each one's history
   (Complete writes it with "closed; feature done"), or the last history
   line when there is none; its priority column shows `-`. That is the
   implementation order Jeff mentioned, without encoding it in names.
   - **Rejected: `queue/done/<YYYY-MM-DD>-<feature>.yaml`.** It breaks the
     name check and every `queue/<feature>.yaml` reference, for an order
     the history already holds.
   - **Rejected: a stored `closed` sequence number.** A second numbering to
     keep.
7. **The set-aside is undone by dropping done items' numbers, with no
   history line** (a7). `wf migrate -done` removes `priority` from each
   done item as it moves it (`priority` becomes `omitempty`). The move is
   the layout's, not the item's; the commit records it.
8. **No hold. The layout changes at `x4-state`, a `human` step, once no
   worker runs an old `wf`** (a8). What an old `wf` does with each state:
   - *New numbers, old layout* (from the merge to `x4-state`): an old `wf`
     reads the same files and fields. Its `Validate` requires numbers
     unique across every item, done ones included, so until the layout
     changes the new `wf` keeps done items' numbers clear of the open
     numbering: a done item in `queue/` numbered 1 to N (N open items) gets
     the number after the highest held. The 45 set-aside items already
     are; this covers an item that closes in the window. An old `wf new`
     still shifts items and logs it; the next new commit renumbers. One
     cost: the new `wf` rewrites the `priority` line of every item whose
     position moved, and an old `wf`'s replay treats any conflict in a
     queue file as hard (`commit.go:383-404`). Where `kind` is omitted,
     `priority:` and `status:` are adjacent lines, so an old-`wf` session
     that starts or closes an item the remote has just renumbered can get
     exit 5 ("re-apply the change by hand"). That is accepted for the
     window, and is one more reason to keep it short.
   - *New layout* (after `x4-state`): an old `wf` globs `queue/*.yaml` and
     no longer sees done items. `waitsForParent` then reads a done parent
     as done, which is right, but `validateFollows` reports every open item
     that follows a done one ("follows X, which is not another item in the
     queue"; two today), `collides_with` naming a done item is reported the
     same way (`queue.go:774-777`), and `wf new -follows <done item>` is
     refused (`main.go:652`, `Find`). A tick told that `wf validate` must
     pass would "fix what the agent wrote into the queue file"
     (`workflow-tick/SKILL.md:171-172`), that is, edit the `follows`. So
     the layout waits for every session running an old `wf` to have ended.
   `x4-state`'s precondition is therefore about sessions ending, not
   leases: a lease exists only while a step is claimed, so an old-`wf`
   session between two steps holds none, and a new run having started does
   not prove an earlier one ended. The step proceeds when every cloud run
   that started before the merge commit has written its digest, each
   machine's `native/status/` file shows a run that both started and
   finished after the merge, and Jeff confirms that no attended session
   from before the merge is still open and that he has rebuilt `dist/wf`.
   Only the `native/status/` part is shared with `loop-native-lead`
   (whose old-`wf` failure, losing a lead, falls on the safe side, so it
   checks leases only). The layout switch is the directory itself: `wf`
   writes done items to
   `queue/done/` only once it exists, which `wf migrate -done` creates.
   - **Rejected: `holds: cloud,host:*`.** A hold lasts while the item is in
     progress, Design included: days of a stopped loop for one moment.
   - **Rejected: move at the first new `wf` commit after the merge.** An
     old `wf` still running would see the new layout at once.
9. **The field stays `priority`** (a9). An old `wf` reads it; a renamed
   field would read as 0 for every item and fail its uniqueness check. The
   rule calls it the item's position.
10. **Ties break by the later placement, then by name** (a10). The stored
    numbers are positions after every save, so a tie arises only from a
    hand edit, an old `wf`, or a replay. The later placement is the later
    "queued at position" or "moved to position" line (or a legacy "queued
    at priority" line), so a replayed `wf new` still lands where it was
    asked. The key is total, so the order is deterministic.
11. **A replay resolves a conflict confined to `priority` lines, then
    renumbers** (a11). On a rejected push `wf commit` cherry-picks the
    session's commit onto the remote state (`replay`, `commit.go:372`). Both
    sides may have renumbered the same items, which conflicts on the
    `priority` line. For each conflicting `queue/` file, the replay
    re-merges it (`git merge-file`) with each side's `priority` line set to
    the remote's value, keeping the session's value when the session's
    change wrote a placement line for that item; if the file then merges
    cleanly it is taken, otherwise the conflict is hard, as today. The
    replay then renumbers before its validation, and `generate` writes the
    positions. A claim is asked again on the new state (`claimAgain`), so
    it needs none of this.
    - **Rejected: leave priority conflicts hard.** Two workers each starting
      or closing an item in one window would make the second re-apply by
      hand (exit 5), for numbers wf recomputes anyway.

## Questions

### Blocking

None.

### Assumed

- **a1. When renumbering runs.** Proceeding with (a); confirm.
  - (a) In `State.Save`, so every command that changes state renumbers, and
    again in `wf commit`'s generation step for hand edits, an old `wf` and
    replays.
  - (b) Only in `wf commit`.
  - (c) At each named event, in each command.

  Recommended: (a) (Decision 1).
- **a2. How history logs a renumber.** Proceeding with (a); confirm.
  - (a) Not at all; a deliberate placement (intake, `wf move`) logs one
    line on the placed item.
  - (b) One compact line on the item whose change caused it ("renumbered 6
    items below").
  - (c) A line per moved item, as today.

  Recommended: (a) (Decision 2).
- **a3. The 232 existing priority lines.** Proceeding with (a); confirm.
  - (a) Kept.
  - (b) `wf migrate -done` deletes the lines wf wrote ("renumbered by wf
    new") from every item; the hand-written ones stay.

  Recommended: (a) (Decision 3).
- **a4. What `-priority P` means at intake.** Proceeding with (a); confirm.
  - (a) The position asked for, kept as nearly as the order allows; `wf
    new` prints the position and logs "(asked P)" when they differ.
  - (b) Refused when the order does not allow it.

  Recommended: (a) (Decision 4).
- **a5. How Jeff reorders.** Proceeding with (a); confirm.
  - (a) `wf move -feature F -to N [-why]`, human workers only.
  - (b) Hand edits of `priority`, as today, resolved by the tie-break.

  Recommended: (a) (Decision 5).
- **a6. The done folder.** Proceeding with (a); confirm.
  - (a) `queue/done/<feature>.yaml`, no `priority`; `wf status -all` lists
    done items in the order they closed.
  - (b) `queue/done/<YYYY-MM-DD>-<feature>.yaml`.
  - (c) (a) plus a stored `closed` sequence number.

  Recommended: (a) (Decision 6).
- **a7. Undoing the set-aside.** Proceeding with (a); confirm.
  - (a) Done items lose `priority` as they move, with no history line.
  - (b) The same, with one history line each ("left the numbering").

  Recommended: (a) (Decision 7).
- **a8. Holding workers while the layout changes.** Proceeding with (a);
  confirm.
  - (a) No hold. The new `wf` keeps done numbers clear of the open ones
    until the layout changes, and the move is made at `x4-state`, a `human`
    step, once every session that started before the merge has ended
    (pre-merge cloud runs have digests, each machine's `native/status/`
    shows a run started and finished after the merge, Jeff confirms his
    attended sessions). Old-`wf` sessions in the window may meet exit 5 on
    a renumbered file.
  - (b) The same, with `x4-state` a `cloud` step: the run that takes it
    checks digests and `native/status/`, but cannot see Jeff's attended
    sessions.
  - (c) `holds: cloud,host:*` for the item's life.

  Recommended: (a) (Decision 8).
- **a9. The field's name.** Proceeding with (a); confirm.
  - (a) `priority`, read as the position.
  - (b) Renamed `position`.

  Recommended: (a) (Decision 9).
- **a10. Ties.** Proceeding with (a); confirm.
  - (a) The later placement first, then name.
  - (b) Name only.

  Recommended: (a) (Decision 10).
- **a11. A replay's priority conflicts.** Proceeding with (a); confirm.
  - (a) Resolved by re-merging with the remote's `priority` lines (the
    session's for an item it placed), then renumbered.
  - (b) Hard, as today: exit 5, re-applied by hand.

  Recommended: (a) (Decision 11).

## Design

### Positions

`Renumber` takes the open items in the order and sets `priority` to 1..N.
The sort key becomes (class, in progress first, priority, later placement
first, name); done items sort after every open item, by the time they
closed (Decision 6). `Ordered`, `Listed`, `Next`, the guard and `QUEUE.md`
are unchanged: they already walk the order, and relative order is what the
PR slot's tie-break compares (`prSlotAhead`, `limits.go:350-372`, both
items in Publish, so in one class and status).

Placement (`place`, used by `Add` and `Move`): take the open items in the
order without the placed item; the item may stand anywhere from just after
the last item whose (class, status) sorts before its own to just before
the first whose sorts after it; put it at index P-1 clamped to that
stretch; number 1..N. `Add` no longer shifts anything itself.

Until `queue/done/` exists, a done item in `queue/` numbered 1..N gets the
number after the highest held (Decision 8). Once it exists, done items
carry no number.

### Reading and writing the folder

- `Load` reads `queue/*.yaml` and, when it exists, `queue/done/*.yaml`, with
  the same name check, into the one `Features` list, so every reader of
  done items (`Find`, `validateFollows`, `waitsForParent`, `Inherit`,
  `Listed(true)`) works unchanged. An item found in both is an error.
- `Save` writes an item to `queue/done/` when it is done and the folder
  exists, otherwise to `queue/`, and removes the file from the other place.
- `wf commit` takes a named `queue/F.yaml` to mean `queue/done/F.yaml` too,
  and the reverse, so a close commits the move. It adds the counterpart
  only when it exists in the worktree, the index or `HEAD` (which covers
  the deleted side of a move): `git add -A --` and `git commit --only --`
  fail on a pathspec that matches nothing (`commit.go:203-218`), and before
  `x4-state` `queue/done/` does not exist at all, so an unconditional pair
  would fail every claim commit (`commit.go:122`). Its generation step also
  commits every queue file whose only uncommitted change is its `priority`
  line, and moves a done item an old `wf` left in `queue/` when that file
  has no uncommitted change. A file with other uncommitted changes outside
  the session's paths is left alone, as `--only` leaves it today.
- `Validate`: the uniqueness check across the queue goes. An open item's
  `priority` must be 1 or more; a done item in `queue/done/` has none;
  an open item in `queue/done/` is an error. Numbers that are not yet
  positions are not an error: `Save` and `wf commit` fix them.
- `wf status` prints the position; `-all` prints `-` for done items, in
  the order they closed.

### Commands

- `wf new -priority P`: places (Decision 4), prints `queued\tF\tposition K`
  and logs "queued at position K" or "queued at position K (asked P)".
- `wf move -worker human/<name> -feature F -to N [-why "..."]`: places,
  logs "moved to position K from M (asked N): <why>", saves. Refused for a
  worker that is not human, a done item, or N < 1.
- `wf migrate -done -worker human/<name>`: once, at `x4-state`. Creates
  `queue/done/`, moves every done item into it without its `priority`,
  renumbers the open items, and prints one line per move. Refused when the
  folder already exists. The existing `wf migrate` (workflow-streamline) is
  unchanged.

### FLOW-05, the skills and the README

FLOW-05 gains one paragraph (Rule changes). The skills change in this PR:

- `workflow-intake` step 3 (`SKILL.md:39-42`): "the priority (show `dist/wf
  status` ... without his say)" becomes "the position (show `dist/wf
  status`, which lists FLOW-05's order with each open item's position:
  the class order decides where an item can go, `wf new` places it as
  near the asked position as its class and status allow and prints where
  it went, and a new item never goes ahead of an in-progress item of its
  own class)".
- `workflow-intake` step 5 (`:88-89`): "(`queue/`, since `wf new` may
  renumber other items; ...)" becomes "(`queue/`, since `wf new`
  renumbers the open items; ...)".
- `workflow-intake`, a regression's follow-up (`:112-115`): "`-priority`:
  the top of the fixes. That is the lowest number held by ... `wf new`
  renumbers the rest and logs each move." becomes "`-priority`: the top of
  the fixes: the position of the first open class-3 item (a feature with
  `follows` set, not yet in implementation); with none, of the first open
  class-4 item; with neither, one past the last open position. `wf new`
  renumbers the rest." Line 127's "set its priority" becomes "move it with
  `wf move`".
- `workflow-human` step 1 (`SKILL.md:37-43`): "renumber the `priority`
  fields as he says" becomes "move each item he places with `dist/wf move
  -feature F -to N -why "<his reason>"` (wf renumbers the rest)", and
  "`priority` reorders items within a class only" becomes "a position
  moves an item within its class only".

The workflow branch's `README.md` changes at `x4-state` (Implementation
plan).

### Overlaps with other items (FLOW-05, FLOW-10)

Nothing serializes these items. Process items are outside the area
relations and are not counted against the limits (`process.md:266-267,
277-279`), and none of them declares `holds`, so they may be in
implementation with PRs open side by side, all editing
`tools/workflow/queue`. Each implement step merges current `main` before it
opens its PR (Consolidation), and a PR whose base moved is refreshed
(FLOW-10) before it merges, so whichever merges second rebases on the first
and keeps both edits; the hunks it must reconcile are named below. If Jeff
wants them in sequence, that takes a hold or a `needs` step, not the area
map.

- **loop-streamline-2** (drafted alongside this one): edits
  `workflow-intake` step 3's size guide (next to the priority sentence
  here), `workflow-human`, `commit.go`'s request (`-new-run` goes), the
  claim code, and the workflow `README.md` at its own `x4-state`. Its
  `x4-state` and this one's touch different README lines. To reconcile:
  `commit.go`'s request (its `-new-run` removal beside this item's path
  pairing) and `generate`, `queue.go`'s claim next to `Save`, and the
  intake skill's step 3.
- **loop-parallel-rule-wording** (approved, unmerged, `823bdd5`): amends
  FLOW-05's **How two items relate**, not the paragraph added here, and
  deletes lines at `workflow-intake/SKILL.md:25` and
  `workflow-human/SKILL.md:72`, not the lines changed here.
- **loop-native-lead** (approved, unmerged, `dbd3032`): amends FLOW-05's
  "Otherwise a worker takes the next ready step ... blocks" clause, in the
  paragraph after the one added here; edits `workflow-intake` step 4,
  `workflow-human` (`wf lead`), the claim code and the README. To
  reconcile: `queue.go`'s `Feature` fields and `Save`, the claim code, and
  the README's Layout and command rows.
- **loop-repository** (approved, unmerged, design at
  `origin/feat/loop-repository`): edits `queue.go`'s `Feature`, `Validate`
  and `Problems`, and the README's Layout. To reconcile: `Validate`
  (this item drops the uniqueness check there) and the Layout rows.

The FLOW-05 wording below is therefore drafted on `main` at `0cd3c02`:
neither approved item changes the lines it replaces.

### What this item does not do

- Change the order, the classes or any limit.
- Rewrite past history (a3).
- Ship a general state-migration step; `loop-streamline-2` files that as a
  follow-up. If one exists by the time this item implements, `wf migrate
  -done` becomes one of its migrations.

## Defaults and overrides

| Default, override or fallback | What it is | Why, and what overrides it |
|---|---|---|
| An open item's `priority` | Its position in the order, set by wf | Jeff's word (the source) and FLOW-05 as amended. Overridden only by placing the item, `wf new -priority` or `wf move -to`, used exclusively (GLOBAL-10); a hand edit is renumbered (a5). |
| *Named fallback: "clamped placement".* A position the order does not allow | The nearest position it does allow | Intake need not count classes (a4). Seen in `wf new`'s and `wf move`'s output and the item's history line, "(asked P)" (GLOBAL-11). |
| Ties in stored numbers | Later placement first, then name | Deterministic, and a replayed placement keeps its place (a10). Arises only from hand edits, an old `wf` or a replay; renumbering removes it at the next save. |
| *Named fallback: "old layout".* No `queue/done/` | Done items stay in `queue/`, numbered clear of the open ones | Keeps an old `wf`'s uniqueness check true until `x4-state` (a8). Ends when `wf migrate -done` creates the folder; `wf status` shows done numbers past N meanwhile (GLOBAL-11). |
| `wf move -why` absent | The history line has no reason | The move itself is the record; Jeff may give one. |
| `wf status -all`'s done order | The order they closed | Jeff's "implementation order" (a6). |

**Merges.** A replay re-merges a queue file with the remote's `priority`
lines (a11), seen as wf commit's existing "replayed onto origin/workflow"
line. **Dropped:** the uniqueness of numbers across done items, and `wf
new`'s per-item renumber lines.

## Cross-driver impact

None: no driver, seam or user-facing surface changes.

## Rule changes

### FLOW-05 (amend), `docs/development/process.md`

**Why:** the numbers become positions that wf keeps, and done items leave
the numbering (Decisions 1, 6).

**Current wording** (`process.md:208-223`, the opening and the class list;
no new text replaces it):

> Work goes deep, not wide. Open items are taken in class order, and within a
> class in-progress items come before queued ones, then the higher priority:
>
> 1. **In implementation**: ...
> ...
> 4. **New functionality**: every other feature.

**Proposed wording:** unchanged, followed by a new paragraph between item 4
and "While a process item is in progress" (`process.md:225`):

> An open item's priority is its position in this order, 1 for the first;
> a done item has none. The queue's tooling (`wf`) renumbers the open items
> whenever a change moves one: an item queued or moved, started, changing
> class, or done. Renumbering changes the numbers, never the order. Only
> the human places an item, at intake or by moving it, except where a rule
> places it (FLOW-09, LOOP-04), and the position asked for is kept as nearly
> as the item's class, and its status within that class, allow.

**Severity and check:** SHOULD, `automated`, before and after.
**Enforcement:** `Renumber` in `Save` and in `wf commit`'s generation step
(`queue.go`, `commit.go`); `place` for `wf new` and `wf move`, and `wf move`
refusing a worker that is not human (`queue.go`, `cmd/wf/main.go`); `wf
migrate -done` (`migrate.go`); `Validate` no longer requiring numbers
unique across done items.

The approved items' amendments of FLOW-05 (`loop-parallel-rule-wording`'s
**How two items relate** and opening, `loop-native-lead`'s blocking clause)
are in other paragraphs; this paragraph applies on top of either.

## Validation plan

- **Tier 0**, `tools/workflow/queue` and `cmd/wf` (`make test`, TEST-01,
  TEST-02). States are built with `Add`, `Claim`, `Complete`, `Back` and
  `Seed`, never by setting `Phase` by hand.
  - `TestRenumberFollowsOrder`: after each event (an item queued, started,
    a process item entering Implement, a feature's merge confirmed into
    Close, an item closed, `wf back`), the open items' numbers are 1..N in
    `Ordered`, and `Ordered` is the same before and after `Renumber`.
  - `TestPlace`: inside the stretch, above it (clamped, "(asked P)"), past
    the end; a queued item never ahead of an in-progress one of its class.
    One history line on the placed item and none on the others.
  - `TestMove`: human only; refused for a done item and N < 1; the line.
  - `TestTieBreak`: equal numbers order by the later placement, then name.
  - `TestDoneFolder`: `Load` reads both places; an item in both is an
    error; `Save` moves a closing item and removes its old file; `follows`
    a done item in `queue/done/` validates and releases the fix; `Listed(true)`
    lists done items in the order they closed, legacy items without a
    `done <step>` line by their last line.
  - `TestOldLayoutNumbers`: with no `queue/done/`, an item closing at
    position 2 is numbered past the highest, and numbers stay unique across
    every item (an old `wf`'s check).
  - `TestMigrateDone`: moves, drops numbers, renumbers, refuses a second
    run.
  - `commit.go` tests (the existing git fixtures): a renumbered file with
    only its `priority` changed is committed; one with other uncommitted
    changes outside the paths is not; a close commits the move; a plain
    `wf commit ... queue/F.yaml` (and a claim commit) succeeds before
    `queue/done/` exists, and for an open item never done; a replay
    whose conflict is only `priority` lines resolves and renumbers; a
    replayed `wf new` keeps its asked place; a conflict elsewhere in the
    file stays hard.
  - The `Validate` uniqueness test and `wf new`'s renumber-line
    expectations change with Decision 2; `testdata` summaries are updated
    where their states' numbers were not positions.
- **The live queue.** On a copy of the `workflow` checkout: `main`'s `wf
  status` and the branch's list the same order; after the branch's `wf
  migrate -done`, `wf status` numbers 1..N in that order, `wf status -all`
  lists the 45 done items from `queue/done/` in the order they closed,
  `wf validate` prints `ok`, and `main`'s `wf validate` on the migrated
  copy reports exactly the two `follows` findings Decision 8 predicts. The
  outputs go in the PR description.
- **`make lint`**, `tools/workflow/flow18-check.sh`, and `make
  enforcement`.
- **Native validation:** none, and no tracked gap (FLOW-09).

## Implementation plan

A process item has no Plan phase; this is the work of `x2-implement`.

1. Tests first (Validation plan); they fail on the current code (TEST-01,
   TEST-02).
2. `queue.go`: `priority` `omitempty`; `Load` from both places; the sort
   key and done order; `Renumber`, `place`, `Move`; `Add` through `place`;
   `Save` by status and folder; `Validate`'s changes (FLOW-05, Decisions 1,
   4-6, 8-10).
3. `commit.go`: renumber and move in `generate`; the path pairing, only
   for a counterpart that exists; the
   replay's re-merge and renumber before validation (Decision 11).
4. `migrate.go`: `MigrateDone` (Decision 7).
5. `cmd/wf/main.go`: `wf move`, `wf migrate -done`, `wf new`'s output, `wf
   status`'s columns, the usage text.
6. `process.md`: FLOW-05's paragraph as approved (FLOW-14).
7. `.claude/skills/workflow-intake/SKILL.md` and
   `.claude/skills/workflow-human/SKILL.md` as under **Design**.
8. `make test`, `make lint`, `flow18-check.sh`, the live-queue comparison.
9. Archive this design with its summary and open the PR (FLOW-12).

After the merge, `x4-state` (`human`, needing `x3-merge`; `x4-close` needs
it), on the `workflow` branch, once Decision 8's precondition holds:

- `dist/wf migrate -done -state .dev/workflow -worker human/<name>`, then
  `wf validate`, then `wf commit ... queue/`.
- `README.md`: the Layout row `queue/<feature>.yaml` gains
  "`queue/done/<feature>.yaml`: done items, without a priority, read for
  `follows` and `wf status -all`"; the sample's `priority: 2 # 1 is
  highest; unique` becomes `# the position in FLOW-05's order, 1 first;
  wf renumbers it; none once done`; the command list gains "`wf move`
  (human only)" and `wf migrate -done`.

The review adds `x4-state` to the queue file when it records the outcome;
it is the state step of the 5-step estimate.

## Design review

None yet.

## Consolidation

A process item has no Publish phase and no consolidation pass (FLOW-03). The
implement step merges current `main` before it opens the PR.

## Rollback notes

None.

## Archive summary

Filled in at Implement, in the PR (FLOW-12).
