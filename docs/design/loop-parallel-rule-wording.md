---
feature: loop-parallel-rule-wording
status: draft
drivers: []              # the loop's rules and two printed strings in wf (queue/relation.go, check/declared.go); no driver tree is reached
seams: []                # no GLOBAL-08 seam changes
surfaces: []             # `wf` is the loop's tool, not a user-facing surface of the product
repos: []                # only this repository
areas: [rules, loop, designs]  # docs/development/process.md; tools/workflow/ (relation.go,
                         # check/declared.go and their tests); docs/design/TEMPLATE.md
security_boundary: none
alters_relationships: false  # no product component's role or interaction changes (FLOW-02)
design_review: required  # a process item's design always gets its review (FLOW-03, FLOW-19)
native_validation: []    # rule text and two printed strings; no host-specific behaviour
tier2_gaps: []
rules: [FLOW-02, FLOW-03, FLOW-05, FLOW-06, FLOW-08, FLOW-10, FLOW-12, FLOW-14, FLOW-15,
        FLOW-16, FLOW-17, FLOW-19, FLOW-20, LOOP-01, LOOP-02, LOOP-04, LOOP-08, LOOP-09,
        GLOBAL-09, GLOBAL-10, GLOBAL-11, TEST-01, TEST-02]
rule_changes:
  - {rule: FLOW-02, change: amend, paths: [docs/development/process.md,
      docs/design/TEMPLATE.md, tools/workflow/check/declared.go]}
  - {rule: FLOW-03, change: amend, paths: [docs/development/process.md]}
  - {rule: FLOW-05, change: amend, paths: [docs/development/process.md]}
                         # FLOW-05's enforcement (queue/relation.go, its tests and golden
                         # files, cmd/wf/declare_test.go) is the loop's machinery, changed
                         # on the process route (FLOW-14); named in its Enforcement line
  - {rule: FLOW-16, change: amend, paths: [docs/development/process.md]}
concepts: [workflow, queue, areas, undeclared, declared-empty, relations, pr-slot,
           done-when, wf-declare, merge-confirmation, merge-tree-proof, squash-merge,
           branch-cleanup, rule-wording, componentization, program]
related: [componentization.md, loop-parallel.md, loop-contract.md, alters-slot.md,
          wf-squash-after-base-moved.md, wf-squash-merged-pr.md]
---

# The rule wording `loop-parallel` left: `areas: []` is declared-empty, the PR-slot paragraph, FLOW-03's area checks at Merge and Close and before `wf declare`, and FLOW-16's merge proof

A process item, and a program item under FLOW-20: a follow-up of
`loop-parallel` (#121, merged as `623131a`), filed on Jeff's decisions of
2026-11-16 on the review of that item's implementation. Those decisions
sent F4, F6 and F5's wording for phases 7 and 8 to one follow-up
rule-change item, and changed nothing on `loop-parallel`'s branch
(`docs/design/loop-parallel.md:5606-5608`). The design's archive summary
adds FLOW-16's merge proof to the same list
(`docs/design/loop-parallel.md:5717-5720`). It is a program item outside
the P1 list at `docs/design/componentization.md:296`, which does not name
it: a follow-up of `loop-parallel`, it runs before `loop-native-lead` and
`loop-repository` because the queue ranks it ahead of `loop-native-lead`
(priority 62 against 63; a11). It adds no item to P1's list, which
componentization (archived) keeps as approved. It holds no worker: it
changes rule text, two printed strings and their tests, and nothing a
running worker does.

Words used throughout. **wf** is the loop's tool, `tools/workflow/`.
**Undeclared** and **declared-empty** are `wf`'s two readings of an item's
areas, defined in **Problem**. "Current wording" is `main` at `0cd3c02`,
which has `loop-parallel`'s fourteen rule changes word for word.

## Problem

`loop-parallel` landed fourteen rule changes approved by name at `636f18b`
and the code that enforces them. Its implementation's review found four
places where the approved wording, or wording older than it, says
something other than what `wf` does. Jeff kept the wording as approved
and the code as built, and sent the four here. In each case the code is
the one he chose, so the rule is the one that moves (a1).

1. **F4: `areas: []` reads as undeclared.** FLOW-02 says "A design that
   declares none is undeclared, and FLOW-05 then treats the item as
   colliding with every item in its repositories"
   (`docs/development/process.md:79-81`). FLOW-05 says "an item that
   declares no areas is taken to change the whole of each repository it
   names" and that two features collide "when they have a repository in
   common and either declares no areas" (`process.md:253-259`). A design
   that writes `areas: []` "declares none" in plain reading. `wf` does
   not read it that way:

   - `queue.Feature.Areas` is a pointer so that "Nil, the key absent, is
     undeclared ... An empty list is a declaration that it changes
     nothing but its own design" (`tools/workflow/queue/queue.go:103-108`),
     and `TestAreasAbsentAndEmpty` keeps the two apart through a save and a
     load (`queue/areas_test.go:259-262`).
   - `State.Undeclared` is true when the areas are absent, when the
     item's home repository has no map, or when they name an area the map
     lacks, and "An empty list is a declaration"
     (`queue/relation.go:94-105`). The relation's row 5 uses it
     (`relation.go:165-181`).
   - `insideAreas` passes an undeclared item, and fails every path of a
     declared-empty item's diff but its own design
     (`check/areas.go:20-54`).
   - `QUEUE.md` prints `undeclared` for an absent key and `none` for an
     empty list (`queue/summary.go:534-545`); `areasText` prints
     `undeclared` for an absent key, "which an empty list is not"
     (`check/declared.go:120-128`).

   The rule also names only one of `wf`'s three ways to be undeclared
   (the key absent); the other two, a home repository with no map and an
   area the map lacks, are in `loop-parallel`'s **Defaults and
   overrides** (`docs/design/loop-parallel.md:3835-3837`) and in the code,
   not in the rule.

   And the reason `wf` prints for row 5 is `<item> declares no areas`
   (`relation.go:180`), which is what a declared-empty item does, and is
   untrue of an item that names an area the map lacks, the case
   `TestRelationUnknownArea` checks (`queue/relation_test.go:324-334`). The
   live `QUEUE.md` shows it on every pair with an undeclared item
   (workflow branch at c0b3295, `QUEUE.md:50-94`). Design's done-when prints
   "undeclared: the design declares no areas" for an absent key
   (`check/declared.go:147-149`), the same ambiguity.

2. **F6: "one feature at a time through the PR slot".** The opening of
   `process.md` says "Publish takes one feature at a time through the PR
   slot: it merges current `main` and runs a consolidation pass ..."
   (`process.md:27-33`). That sentence predates the limits. Under
   FLOW-05 as amended the PR slot's limit is 1 per related set: "no two
   features that collide or are near have pull requests open together",
   and "Items that do not collide may be in flight, and have pull
   requests open, side by side; their pull requests merge one at a time"
   (`process.md:240-242, 290-299`). The paragraph also omits the refresh,
   which runs the consolidation pass again whenever `main` moves past an
   open PR (FLOW-10, `process.md:471-488`).

3. **F5: FLOW-03's area checks at phases 7 and 8, and before `wf
   declare`.** FLOW-03 says "From Plan on, each done-when also checks
   that the queue still records what the design declares; from Implement
   on, that every path the branch changes lies in an area the design
   declares" (`process.md:135-139`). The code runs the first check at
   phases 2 to 6 and at the refresh, and the second at phases 3 to 6 and
   at the refresh; phase 7 runs only the merge confirmation and phase 8
   only the cleanup checks (`check/check.go:53-103`, `Refresh` at
   `check.go:135-150`). "From Plan on" and "from Implement on" therefore
   overclaim for Merge and Close. FLOW-02's sentence that points at the
   same check, "against the paths the change edits, from Implement on
   (FLOW-03)" (`process.md:104-108`), overclaims the same way (a5).

   The rule also does not name the exemption the code makes for an item
   the queue records no declaration for. `declaredStill` passes such an
   item with "not declared: finished Design before wf declare"
   (`check/declared.go:240-250`), and `insideAreas` passes it while its
   areas are absent (`areas.go:23-27`). One open item is in that state
   today: `prebuilt-artifacts`, which finished `f1-design` on 2026-11-15
   before #121 merged and is in Plan with no `declared` (workflow branch,
   `queue/prebuilt-artifacts.yaml`). The exemption is what lets it pass
   its gates; the rule as written says those gates check it.

4. **FLOW-16's merge proof** (the review's F-P2, and design-code finding
   2). FLOW-16 says a squash-merged branch is shown contained in trunk by
   "a commit on trunk naming the PR whose tree equals the branch head's
   tree, or a one-parent such commit whose change from its parent has the
   same `git patch-id --verbatim` as the branch head's change from its
   merge base with that parent", and that `git branch -D` runs only
   "after `wf check` has shown the equal tree or change"
   (`process.md:648-660`). That is `wf-squash-after-base-moved`'s proof
   (#107, `97a13f7`). `loop-parallel` replaced it: `wf` now proves a
   squash C of a ref only if `git merge-tree --write-tree
   --merge-base=B C^ <ref>`, B the ref's single merge base with C^, writes
   exactly C's tree (Decision 23 and a14 (d), Jeff, 2026-11-16;
   `docs/design/loop-parallel.md:562-646`; `check/check.go:383-481`). No
   code computes a patch-id any more. `CONTRIBUTING.md:17-19` already
   names the merge-tree proof; the rule is the one place left that names
   the old one. Decision 23 replaced the three-line patch-id proof FLOW-16
   names (`loop-parallel.md:562-646`; its zero-context variant, a14 (a),
   was withdrawn on 2026-11-16 for proving merged two refs that are not,
   `loop-parallel.md:618-633`), so FLOW-16 states as the safety condition
   for `git branch -D` a proof `wf` no longer runs.

None of the four changes what a worker does. Each makes a rule say what
`wf` already enforces, so that an agent reading the rule, or a human
reading `QUEUE.md`, reaches the same answer as the code.

## Prior art

- **`loop-parallel`** (#121, archived): the fourteen rule changes this
  item rewords, and the code they describe. Its rule-change sections
  (`loop-parallel.md:3880-4760`) are the model for this design's **Rule
  changes**; its **Defaults and overrides** rows for `areas` absent, the
  named fallback "area not in the map" and `areas: []`
  (`loop-parallel.md:3835-3836`) are the semantics FLOW-02 and FLOW-05
  now state; Decision 23 is the proof FLOW-16 now names. Its review
  decisions of 2026-11-16 are this item's source.
- **`wf-squash-after-base-moved`** (#107, archived): wrote FLOW-16's
  current patch-id sentence (`97a13f7`), with `rule_changes: []` and the
  rule wording approved in its review. Its third proof is the one
  Decision 23 replaced.
- **`wf-squash-merged-pr`** (archived): the tree-equality proof, the
  second of the three, which FLOW-16 keeps unchanged.
- **`alters-slot`** (#118, archived): the precedent for a small process
  item that makes code and rule agree in one sitting. It went the other
  way (code to rule, `rule_changes: []`); here the code is Jeff's choice
  and the rule moves (a1).
- **`loop-contract`** (#119, archived): LOOP-09 (a loop rule changes only
  through a process item, which this is) and LOOP-08's "made neutral
  when the rule is next changed", which its Decision 2 and a9 read as
  the job of one neutral-rewording item after P1 (a10).
- **`componentization`** (#117): FLOW-20 and P1's order. This item is a
  program item: it amends rules a P1 item changed, and lists the umbrella
  in `related`.

Nothing in `docs/design/` overlaps otherwise: a search for `declared-empty`,
`declares no areas`, `patch-id` and `PR slot` finds only the designs above.

## Decisions

1. **The rules move to the code, not the code to the rules** (a1). In each
   of the four places Jeff kept the code as built (2026-11-16) and asked
   for the wording to follow. The code changes are the printed reason
   for relation row 5, which the source names ("relation.go's 'declares
   no areas' text and its tests move with it"); and, beyond the source,
   the matching Design done-when text in `check/declared.go:148` (a4).
   - *Rejected: make `wf` match the approved wording.* That would read
     `areas: []` as undeclared (dropping a declaration `loop-parallel`'s
     a-questions and tests keep on purpose), run the area checks at Merge
     and Close (where the branch is merged or deleted and `git diff
     origin/main...<branch>` has nothing to say), and restore a proof
     Decision 23 replaced.

2. **`undeclared` is a defined term in FLOW-05, with all three of `wf`'s
   cases** (a2). FLOW-05 defines it once, where the relations are
   defined, and FLOW-02 points at it. The cases are the key absent, a
   home repository (the first the item names and the one that holds its
   design; FLOW-02, LOOP-04) with no map, and an area the map lacks until the map
   has it, which are `State.Undeclared`'s three (`relation.go:99-105`).
   - *Rejected: name only the absent key and leave the other two to the
     design's named fallback.* FLOW-05's check is `automated`; a reader
     of the rule would mispredict `wf` for an item homed where there is
     no map, or one that asks for an area not yet added.

3. **`areas: []` is a declaration that the change edits nothing but its
   own design** (a3). This is what `insideAreas` enforces: with the list
   empty every path but the design is outside it (`areas.go:33-46`). The
   rules say so in FLOW-02, FLOW-05 and the template's comment (a9). It
   holds for a declared item: an item whose home repository has no map is
   undeclared whatever its `areas` say (`relation.go:99-102`), and FLOW-02
   and FLOW-05 say that too (the review's change, 2026-11-17).
   - *Rejected: refuse `areas: []` in `wf declare`.* No open item uses
     it today (no queue file on the workflow branch has `areas: []`), so
     refusing it costs nothing now; but it is the honest declaration for
     a design-only item, and the code, tests and `QUEUE.md`'s `none` all
     treat it as one.

4. **Row 5's reason becomes `<item> is undeclared`** (a4). It uses the
   term the rule now defines and the word `QUEUE.md`'s Areas column
   already prints. The Design done-when text for an absent key becomes
   "undeclared: the design leaves areas out".

5. **FLOW-03 gives the checks' ranges as the code runs them, and names
   the merge confirmation, Close and the undeclared-before-`wf declare`
   exemption** (a6; FLOW-02's sentence on the same range, a5). The
   comparison runs from Plan through Publish
   and at each refresh of a feature; the diff check from Implement
   through Publish and at each refresh of a feature; Merge's confirmation
   and Close run neither; a feature's refresh, at Merge included, runs
   both, and a process item's refresh neither. An item with no recorded
   declaration is not compared, and its diff check reads the areas the
   queue records, which for such an item are none.

6. **The opening paragraph describes the PR slot as FLOW-05 has it** (a7).
   It is listed as part of FLOW-05's rule change, because the slot is
   FLOW-05's.

7. **FLOW-16 names the merge-tree proof, in the form `wf` runs it** (a8):
   a one-parent commit C on trunk naming the PR, B the single merge base
   of the head and C^, and `git merge-tree --write-tree --merge-base=B C^
   <head>` writing exactly C's tree. The merge-driver override `wf`
   applies (F-P1; `check.go:487-512`) stays in the code and in
   `loop-parallel`'s Decision 23, not in the rule.

8. **No neutral rewording here** (a10). This item changes FLOW-02, -03,
   -05 and -16, and leaves their this-repository names (`wf`,
   `TEMPLATE.md`) as they are, as `loop-parallel` did, under
   `loop-contract`'s Decision 2 and a9.

9. **No worker is held** (a11). A worker running the old `wf` prints the
   old reason text; a worker reading the old rule reaches the same
   decisions, because the code did not change.

## Questions

### Blocking

None. Every choice below has a default that keeps `wf`'s behaviour as
Jeff accepted it on 2026-11-16.

### Assumed

- **a1. Which side moves.** Proceeding with (a); confirm.
  - (a) *The rules move to the code* in all four places, plus the two
    printed strings (Decision 1). Consequence: no behaviour changes; the
    item is rule text, two strings, and their tests and golden files.
  - (b) *The code moves to the rules* where it can: `areas: []` becomes
    undeclared, the area checks run at Merge and Close. Consequence: a
    behaviour change to a loop that just went parallel, tests rewritten,
    a worker hold needed while it lands; and FLOW-16 cannot go this way
    at all, since Decision 23 replaced the patch-id proof.
  - Recommendation: (a).

- **a2. How FLOW-05 defines undeclared.** Proceeding with (a); confirm.
  - (a) *All three of `wf`'s cases*: the `areas` key absent, a home
    repository (the first the item names and the one that holds its
    design; FLOW-02, LOOP-04) with no area map, or an area the map lacks, until the map
    has it (Decision 2).
  - (b) *Only the absent key*, the other two left to `loop-parallel`'s
    named fallback. Shorter, but an agent reading the rule mispredicts
    `wf` in two cases.
  - (c) *A pointer*, "undeclared as `wf` reads it". Shortest, and makes
    the rule depend on the tool's code, which LOOP-08 and FLOW-14 run
    the other way.
  - Recommendation: (a).

- **a3. What `areas: []` means.** Proceeding with (a); confirm.
  - (a) *A declaration that the change edits nothing but its own design*,
    as `insideAreas` enforces (Decision 3).
  - (b) *Not allowed*: `wf declare` refuses it and the rule says so. No
    item uses it today, so the cost is a code change and a test, and the
    loss of the one honest declaration for a design-only item.
  - Recommendation: (a).

- **a4. The printed reason for relation row 5.** Proceeding with (a);
  confirm.
  - (a) `<item> is undeclared`, the rule's term and `QUEUE.md`'s word.
    One string in `relation.go:180`, its comment at `relation.go:94-98`,
    eight test expectations, two golden files and one `wf` command test
    (Implementation plan, task 5).
  - (b) *A reason per cause*: `<item> leaves areas out`, `<item>'s home
    repository has no area map`, `<item> names <area>, which the map
    lacks`. More to read in `QUEUE.md`, and `Undeclared` would have to
    return its cause; `QUEUE.md`'s Areas column already marks an
    unknown area with `*` (`summary.go:534-550`).
  - (c) *Leave it*: the rule then defines a term the printed reason does
    not use, and the reason stays untrue of a map-lacks item.
  - Recommendation: (a).
  - And the Design done-when text: (a) also rewords
    `check/declared.go:148` to "undeclared: the design leaves areas out"
    (beyond the source, which names only relation.go), so that both texts
    use the rule's term; (a′) relation.go only, as the source names, and
    declared.go keeps "the design declares no areas". Recommendation: (a);
    Jeff, confirm that this is in scope.

- **a5. FLOW-02's sentence "from Implement on (FLOW-03)".** The source
  names F5 as FLOW-03's wording; FLOW-02's last paragraph repeats the
  same range (`process.md:104-108`). Proceeding with amending it too, to
  "from Implement through Publish and at each refresh of a feature
  (FLOW-03)", in
  FLOW-02's change, which this item makes anyway for F4; confirm.
  Alternative: leave it, and FLOW-02 keeps overclaiming for Merge and
  Close. Recommendation: amend.

- **a6. How FLOW-03 states the exemption before `wf declare`.**
  Proceeding with (a); confirm.
  - (a) *Name it in the rule as the code makes it*: an item the queue
    records no declaration for, because it finished Design before the
    queue recorded declarations, is not compared with its design, and
    its diff check reads the areas the queue records, so with none it is
    undeclared and no path is outside them. Consequence: true for
    `prebuilt-artifacts` today and for any item like it. Once no open
    item lacks `declared`, the sentence describes nothing; a later item
    may retire it with the code's branch.
  - (b) *Close the exemption instead*: Jeff runs a human `wf declare` on
    `prebuilt-artifacts`, as `x4-state` did for `seam-specs` and
    `driver-contract`, and a later item removes the branch from
    `declaredStill`. The rule then needs no sentence, but that is a code
    change and a human step this item's source does not ask for.
  - Recommendation: (a), and (b)'s `wf declare` on `prebuilt-artifacts`
    at Jeff's convenience, which needs no rule change.

- **a7. The opening paragraph (F6): where it is listed, and how much it
  says.** Proceeding with (a) and (i); confirm.
  - Listing: (a) under FLOW-05's rule change, the slot being FLOW-05's;
    (b) under FLOW-10's, the consolidation pass being FLOW-10's.
    `process.md:27-33` is outside every rule heading, but it is in this
    directory, so it is a rule change by FLOW-14's definition and the
    review approves it by name.
  - Wording: (i) the full paragraph under **Rule changes**: the slot by
    relation, disjoint PRs side by side and merging one at a time, and
    the refresh; (ii) the smallest edit, "one feature at a time" to "one
    feature at a time among the features related to it", leaving the
    refresh out.
  - Recommendation: (a), (i).

- **a8. How much of the merge-tree proof FLOW-16 names.** Proceeding with
  (a); confirm.
  - (a) *The command and its single merge base*, as under **Rule
    changes**; the merge-driver override and the git version stay in the
    code, `loop-parallel`'s Decision 23 and `CONTRIBUTING.md`.
  - (b) *Also the override*: "with git's own text merge for every file,
    whatever its merge driver". Complete, but it is implementation, and
    FLOW-16 would change again if the override does.
  - (c) *No command*: "a one-parent such commit whose tree is what
    merging the branch head onto its parent writes". Shorter, and leaves
    the merge base, the point of the proof, unnamed.
  - Recommendation: (a).

- **a9. The template's `areas` comment.** `docs/design/TEMPLATE.md:13-15`
  says what leaving the key out means and not what `[]` means.
  Proceeding with adding "`[]`: the change edits nothing but its own
  design" (and a semicolon at the end of the line before it) to the
  comment, in FLOW-02's change; confirm. Alternative: leave the template,
  which is accurate as far as it goes. Recommendation: add it.

- **a10. LOOP-08's "made neutral when the rule is next changed".** This
  item changes four FLOW rules and leaves their this-repository names
  (`wf`, `wf declare`, `wf check`, `TEMPLATE.md`, the PR slot's `main`)
  where they are. Proceeding as `loop-parallel` did (its FLOW-02 section,
  `loop-parallel.md:4017-4054`): the neutral rewording is one process
  item after P1, under `loop-contract`'s Decision 2 and a9; confirm.
  Alternative: make the four rules neutral here, which needs the
  profile terms the neutral-rewording item is to write. Recommendation:
  defer, as before.

- **a11. Holds, and order with `loop-native-lead`.** Proceeding with no
  holds, and with this item before `loop-native-lead` by queue priority
  (62 against 63) and `follows: loop-parallel`, not by P1's list in
  `componentization.md:296`, which does not name it; `loop-native-lead`'s
  source also names FLOW-16; confirm. If this merges
  first, `loop-native-lead`'s design quotes FLOW-16 as this item leaves
  it; if it is ever reordered, whichever merges second re-quotes the
  other's wording at its refresh (FLOW-10). Recommendation: as proposed.

- **a12. Four stale skill and agent lines, outside the source's scope.**
  `wf-review`, `workflow-intake`, `workflow-tick` and `workflow-human`
  still say `wf areas -list` and `-check` "exit 1 while the queue state
  has no `areas.yaml`, which is expected until `loop-parallel`'s
  `x4-state` writes it" (`.claude/agents/wf-review.md:24`,
  `.claude/skills/workflow-intake/SKILL.md:25`,
  `.claude/skills/workflow-tick/SKILL.md:90`,
  `.claude/skills/workflow-human/SKILL.md:72`). `x4-state` is done and
  `areas.yaml` exists, so the lines describe a past state. Proceeding with
  leaving them out of this item, whose source says "rule text only", and
  filing them as a small process item on Jeff's word; confirm.
  Alternative: fold the four deletions in here, since this item's areas
  already include `loop`. Recommendation: leave out; fold in only if
  Jeff prefers one PR.
  **Changed on Jeff's word, 2026-11-17, after the review: fold in.** The
  four clauses are deleted here (Implementation plan, task 9), and
  `skill-areas-yaml-lines`, filed for them, is closed as merged into this
  item.

## Design

### What changes

Four rule changes in `docs/development/process.md`, one comment in
`docs/design/TEMPLATE.md`, two printed strings in `wf`
(queue/relation.go, check/declared.go) with their tests, and the four
stale clauses of a12 deleted from the skills and an agent.
The exact wording is under **Rule changes**. In summary:

| Finding | Where | Before | After |
|---|---|---|---|
| F4 | FLOW-02 `areas` bullet; TEMPLATE comment | "A design that declares none is undeclared" | Leaving the key out is undeclared, as FLOW-05 defines it; `areas: []` declares the change edits only its own design, for an item whose home has a map |
| F4 | FLOW-05, How two items relate | "an item that declares no areas"; "either declares no areas" | *Undeclared* defined: key absent, home with no map, or an area the map lacks; a declared item's `[]` changes only its design; "either is undeclared" |
| F4 | `relation.go` row 5; `declared.go` Design text | `<item> declares no areas`; "the design declares no areas" | `<item> is undeclared`; "the design leaves areas out" |
| F6 | `process.md` opening | "Publish takes one feature at a time through the PR slot" | The slot by relation; disjoint PRs side by side, merging one at a time with `main` contained; the refresh repeats the pass |
| F5 | FLOW-03 paragraph after the table; FLOW-02 last paragraph | "From Plan on"; "from Implement on" | "From Plan through Publish, and at each refresh of a feature"; "from Implement through Publish, and at each refresh of a feature"; the merge confirmation and Close run neither; a feature's refresh, at Merge too, runs both and a process item's neither; the item with no recorded declaration |
| F-P2 | FLOW-16 | patch-id `--verbatim`; "the equal tree or change" | `git merge-tree --write-tree --merge-base=B C^ <head>` writes C's tree; "the equal tree or that merge" |

### What a worker sees

Nothing changes in what `wf next`, `wf claim`, `wf done` or `wf check`
decide. The one visible difference is the text of a colliding relation
in `QUEUE.md`'s relations table, in `wf relations` and in a refused claim:

```
before: | detached-prepare-windows-exe, driver-contract | colliding | driver-contract declares no areas |
after:  | detached-prepare-windows-exe, driver-contract | colliding | driver-contract is undeclared |

before: the PR slot, held by a (colliding: a declares no areas) (FLOW-05)
after:  the PR slot, held by a (colliding: a is undeclared) (FLOW-05)
```

and Design's done-when for a design with no `areas` key:

```
before: undeclared: the design declares no areas, and the item collides with every item in its repositories (FLOW-05)
after:  undeclared: the design leaves areas out, and the item collides with every item in its repositories (FLOW-05)
```

An old `wf` and a new one compute the same relation; only the reason
string differs, and nothing parses it.

### Why the merge confirmation and Close need neither check (F5)

Phase 7's done-when only confirms the merge (`c.merged()`); while the PR
waits at Merge, the refreshes that keep it current run both checks
(`Refresh`, check.go:135-150). A process item's refresh runs neither,
since it has no declaration (`if f.Round != "process"`).

Recorded here, not in the rule. At Merge the pull request is the
branch Publish checked, kept up to date by refreshes, each of which runs
both checks again (`Refresh`, `check.go:135-150`); a change that needs
more goes back to Implement or Design (FLOW-06, FLOW-10), whose gates run
them. At Close the branch has merged and is being deleted, so `git diff
origin/main...<branch>` is empty or the branch is gone. Running the
checks there would add a way for Close to fail on a branch that no longer
exists, and nothing to find.

### The exemption before `wf declare` (F5)

`wf declare` exists from #121's merge (2026-11-16T01:45Z). An item that
finished Design before then has no `declared`, and Design's own gate,
which now requires it (`declared.go:138-140`), will not run for it again
unless it returns to Design. `declaredStill` says so and passes
(`declared.go:240-250`). `insideAreas` asks `Undeclared`, which is true
while its `areas` key is absent (`areas.go:23-27`). The rule's new
sentence states both. Today that is `prebuilt-artifacts` (in Plan); it
was `seam-specs` and `driver-contract` until `x4-state` declared them by
hand. a6 (b) would close it the same way.

### FLOW-16's proof in the rule (F-P2)

The rule names what `refMerged` and `mergeTreeProof` do
(`check.go:383-481`): first an ancestor of trunk, then a PR commit with
the ref's tree, then, for a one-parent PR commit C with a single merge
base B between the ref and C^, `git merge-tree --write-tree
--merge-base=B C^ <ref>` printing exactly C's tree. With no merge base or
several, the third proof does not hold and the item falls to the hand
confirmation FLOW-16 already describes. `merged` runs it for the local
and the remote ref when each exists (`check.go:342-381`); the rule keeps
"the branch head", which covers both. The workflow-tick skill's Close
text ("`git branch -D`, only once `dist/wf check` has passed on it",
`.claude/skills/workflow-tick/SKILL.md:145-148`) needs no change.

### Order and other items

- `loop-native-lead` (63, queue priority) also amends FLOW-16 (its source); it will quote
  this wording if this merges first (a11).
- `driver-contract` (K1) changes no FLOW rule; `loop-parallel` carried its
  FLOW-02 bullets. Nothing here touches them.
- The workflow branch's `README.md` already describes `[]` correctly
  ("[] declared, changes nothing but its design", `README.md:70-71`) and
  needs no change.

### What this item does not do

- No behaviour change to `wf`, the guard, the limits or any gate.
- No neutral rewording (a10), no edit to an archived design
  (`loop-parallel.md`'s relation table keeps the reason text it was
  approved with), no change to the area map.
- The four stale skill and agent lines of a12 are deleted here, Jeff's
  fold-in of 2026-11-17; nothing else in `.claude/` changes.

## Defaults and overrides

None added or changed. The rules now state five that `loop-parallel`
already made, without changing them:

| What | Default | Override |
|---|---|---|
| `areas` absent | Undeclared: collides with every item it shares a repository with (FLOW-05) | Declare areas, or `[]` |
| `areas` names an area the map lacks | Undeclared until an attended session adds the area (the named fallback "area not in the map", GLOBAL-11; `QUEUE.md` marks it `*`) | The map changes on Jeff's word |
| *Named fallback: "no map".* The item's home repository has no area map | Undeclared: the repository is one exclusive area, and the item collides with every item in it (FLOW-05) | An attended session adds the map on Jeff's word; `QUEUE.md`'s Relations gives the missing map as the reason (GLOBAL-11) |
| `declared` absent (the item finished Design before `wf declare`) | Not compared with its design; its diff check reads the areas the queue records, so with none it is undeclared (FLOW-03) | A human `wf declare` sets it; the check prints "not declared: finished Design before wf declare" (GLOBAL-11) |
| *Named fallback: "no single merge base".* A squash with no single merge base between the ref and C^ | The third proof does not hold; Close's escalation gives Jeff the commands (FLOW-16, GLOBAL-10) | None: Jeff compares and deletes by hand |

## Cross-driver impact

None. No driver tree, driver contract or `driver.Session` is reached; the
change is loop rule text and two strings in the loop's tool.

## Rule changes

Four, each for the design review to approve by name (FLOW-14, FLOW-19).
"Current wording" is `main` at `0cd3c02`: `docs/development/process.md`
and `docs/design/TEMPLATE.md`, each quoted from that commit on
2026-11-17. Where a rule has several paragraphs only the amended ones are
quoted, each in full. No severity and no check type changes. A process
item's branch is exempt from the FLOW-14 clearance (`process.md:586-587`);
the entries are listed so the review approves each by name.

### FLOW-02 (amend), `docs/development/process.md`, `docs/design/TEMPLATE.md`, `tools/workflow/check/declared.go`

**Why:** F4 (`areas: []` reads as undeclared) and a5 (the check's range).

**Current wording**, the `areas` bullet (`process.md:79-81`) and the last
paragraph (`process.md:104-108`):

> - `areas` — the areas of the repository's area map whose files the change
>   edits (FLOW-05). A design that declares none is undeclared, and FLOW-05
>   then treats the item as colliding with every item in its repositories.

> The automated part diffs the declared `drivers` / `seams` / `surfaces` against the
> trees and files the change reaches. It checks the declared `areas` against
> what `drivers`, `surfaces` and the `rule_changes` paths imply, at Design,
> and against the paths the change edits, from Implement on (FLOW-03). The
> agent part judges the security note.

**Proposed wording:**

> - `areas` — the areas of the repository's area map whose files the change
>   edits (FLOW-05). A design that leaves the key out is undeclared, as
>   FLOW-05 defines it, and FLOW-05 then treats the item as colliding with
>   every item in its repositories. `areas: []` is a declaration: the
>   change edits nothing but its own design, though an item whose home
>   repository has no map is undeclared whatever it declares (FLOW-05).

> The automated part diffs the declared `drivers` / `seams` / `surfaces` against the
> trees and files the change reaches. It checks the declared `areas` against
> what `drivers`, `surfaces` and the `rule_changes` paths imply, at Design,
> and against the paths the change edits, from Implement through Publish
> and at each refresh of a feature (FLOW-03). The agent part judges the
> security note.

**`TEMPLATE.md`**, current (`TEMPLATE.md:13-15`):

```yaml
# areas: []              # the areas the change edits (FLOW-02), by the names of the
                         # area map (`wf areas -list`). Left out, the item is undeclared
                         # and collides with every item in its repositories (FLOW-05)
```

proposed (a9):

```yaml
# areas: []              # the areas the change edits (FLOW-02), by the names of the
                         # area map (`wf areas -list`). Left out, the item is undeclared
                         # and collides with every item in its repositories (FLOW-05);
                         # []: the change edits nothing but its own design
```

**Severity and check:** MUST, `automated + agent-judgment`, before and
after. **Enforcement:** `tools/workflow/check/declared.go:148`, the text
for a design with no `areas` key, from "undeclared: the design declares
no areas, and the item collides ..." to "undeclared: the design leaves
areas out, and the item collides ...". `declared_test.go` matches on
"undeclared" (`declared_test.go:98`) and needs no change. No check
changes what it decides.

### FLOW-03 (amend), `docs/development/process.md`

**Why:** F5. The two ranges overclaim for Merge and Close, and the
exemption for an item with no recorded declaration is unnamed.

**Current wording**, the paragraph after the phase table
(`process.md:133-139`):

> A transition happens only when the phase's done-when checks pass mechanically
> (files present, sections filled, commands green), never on an agent's own report
> that it is done. From Plan on, each done-when also checks that the queue
> still records what the design declares; from Implement on, that every
> path the branch changes lies in an area the design declares (FLOW-02). A
> path outside them is settled by widening the declaration inside the
> approved design, or by a return to Design (FLOW-06).

**Proposed wording:**

> A transition happens only when the phase's done-when checks pass mechanically
> (files present, sections filled, commands green), never on an agent's own report
> that it is done. From Plan through Publish, and at each refresh of a
> feature (FLOW-10), each done-when also checks that the queue still
> records what the design declares; from Implement through Publish, and
> at each refresh of a feature, that every path the branch changes lies
> in an area the design declares (FLOW-02). A path outside them is
> settled by widening the declaration inside the approved design, or by a
> return to Design (FLOW-06). The merge confirmation (phase 7's
> done-when) and Close run neither; a feature's refresh runs both,
> including one taken while the pull request waits at Merge, and a
> process item's refresh runs neither. An item the queue records
> no declaration for, because it finished Design before the queue
> recorded declarations, is not compared with its design; its diff check
> reads the areas the queue records, so with none it is undeclared
> (FLOW-05) and no path lies outside them.

**Severity and check:** MUST, `automated`, before and after.
**Enforcement:** none changes. The sentence describes `Phase`
(`check/check.go:53-103`), `Refresh` (`check.go:135-150`),
`declaredStill` (`check/declared.go:240-250`) and `insideAreas`
(`check/areas.go:20-54`) as they are. A process item's table is
untouched: its round runs neither check (`check.go:183-202`).

### FLOW-05 (amend), `docs/development/process.md`

**Why:** F4 (the definition of undeclared, and `[]`) and F6 (the opening
paragraph's PR slot), and the printed reason that goes with F4.

**Current wording**, the end of **How two items relate**'s first
paragraph and the first bullet after it (`process.md:251-260`):

> ... Area A *reaches* area B when code in A
> depends on code in B; only exclusive areas reach and are reached. A
> repository with no map is one exclusive area, and
> an item that declares no areas is taken to change the whole of each
> repository it names. Two features are:
>
> - **colliding** when they share an exclusive area; when they have a
>   repository in common and either declares no areas; when both change a
>   seam that has no specification (LOOP-02); or when the human says so in
>   a design review (FLOW-19);

and the opening of `process.md`, outside every rule heading
(`process.md:27-33`):

> Publish takes one feature at a time through the PR slot: it merges current
> `main` and runs a **consolidation pass** against every feature that landed while
> it was being built (FLOW-10, FLOW-15), and only sends the feature back when that
> pass finds real work. This is the routine form of the consolidation that
> followed the Windows driver work (the claim / tunnel / collector model). Doing it
> per feature keeps the reconciliation small and keeps terminology and shared
> abstractions converging rather than forking.

**Proposed wording** (a2, a3):

> ... Area A *reaches* area B when code in A
> depends on code in B; only exclusive areas reach and are reached. A
> repository with no map is one exclusive area. An item is *undeclared*
> when its `areas` are left out, when its home repository, the first it
> names and the one that holds its design (FLOW-02, LOOP-04), has no map,
> or when they name an area the map lacks, until the map has it;
> an undeclared item is taken to change the whole of each repository it
> names. A declared item whose areas are `[]` changes nothing but its own
> design (FLOW-02). Two features are:
>
> - **colliding** when they share an exclusive area; when they have a
>   repository in common and either is undeclared; when both change a
>   seam that has no specification (LOOP-02); or when the human says so in
>   a design review (FLOW-19);

and the opening (a7 (i)):

> Publish takes a feature through the PR slot (FLOW-05): no two features
> that collide or are near have pull requests open together, while
> disjoint ones may, and their pull requests merge one at a time, each
> only while its branch contains `main` (FLOW-08). Publish merges current
> `main` and runs a **consolidation pass** against every feature that landed while
> it was being built (FLOW-10, FLOW-15), and a refresh runs it again whenever
> `main` moves past the branch; either sends the feature back only when the
> pass finds real work. This is the routine form of the consolidation that
> followed the Windows driver work (the claim / tunnel / collector model). Doing it
> per feature keeps the reconciliation small and keeps terminology and shared
> abstractions converging rather than forking.

**Severity and check:** SHOULD, `automated`, before and after.
**Enforcement:** the printed reason of relation row 5 (a4):
`tools/workflow/queue/relation.go:180` returns `<item> is undeclared`
instead of `<item> declares no areas`, and the comment on `Undeclared`
(`relation.go:94-98`) says "has no areas the map can read" in place of
"declares no areas the map can be read with". Its expectations move
with it: `queue/relation_test.go:116, 118, 216, 219, 231, 329, 331` and
the comment at `:25`; `queue/limits_test.go:216`;
`queue/testdata/queue-today.md:46` and `queue-later.md:42-49`; and
`cmd/wf/declare_test.go:129-139`. The relation each pair gets, and every
guard decision, is unchanged.

### FLOW-16 (amend), `docs/development/process.md`

**Why:** F-P2 / design-code 2. The rule names the patch-id proof that
`loop-parallel`'s Decision 23 replaced on 2026-11-16 (a14 (d)); `wf`
proves a squash by merge-tree.

**Current wording**, the third paragraph (`process.md:648-660`):

> Deleting a branch loses any commit not on trunk, so a branch is deleted only
> after it is shown to be contained in trunk (`git branch --merged`, the PR's
> merge commit, or, for a squash merge, a commit on trunk naming the PR whose
> tree equals the branch head's tree, or a one-parent such commit whose change
> from its parent has the same `git patch-id --verbatim` as the branch head's
> change from its merge base with that parent), with git's safe forms
> (`git branch -d`) and never by force-pushing. `git branch -d` refuses a
> squash-merged branch, so that case alone deletes the local branch with
> `git branch -D`, and only after `wf check` has shown the equal tree or change;
> otherwise the escalation gives the human these commands, and an agent never
> runs them. Caches a later build reuses (prepared images, support images)
> stay. An agent without permission to delete lists the exact commands for the
> human instead of skipping the step.

**Proposed wording** (a8):

> Deleting a branch loses any commit not on trunk, so a branch is deleted only
> after it is shown to be contained in trunk (`git branch --merged`, the PR's
> merge commit, or, for a squash merge, a commit on trunk naming the PR whose
> tree equals the branch head's tree, or a one-parent such commit C whose
> tree is exactly what merging the branch head onto C's parent writes, from
> the single merge base B of the two:
> `git merge-tree --write-tree --merge-base=B C^ <head>`), with git's safe forms
> (`git branch -d`) and never by force-pushing. `git branch -d` refuses a
> squash-merged branch, so that case alone deletes the local branch with
> `git branch -D`, and only after `wf check` has shown the equal tree or
> that merge; otherwise the escalation gives the human these commands, and
> an agent never runs them. Caches a later build reuses (prepared images,
> support images) stay. An agent without permission to delete lists the
> exact commands for the human instead of skipping the step.

**Severity and check:** MUST, `automated + agent-judgment`, before and
after. **Enforcement:** none changes; the sentence describes `refMerged`
and `mergeTreeProof` (`check/check.go:383-481`) as they are.

### Against LOOP-08 and LOOP-09

LOOP-09: each of the four is a loop rule, changed by a process item whose
design Jeff settles (`loop-contract.md:389-416`). LOOP-08's agent part
asks the review to judge that "a new or amended loop rule assumes nothing
beyond the list" (`loop-contract.md:383-385`): the amendments add git
commands (git is on the list), the queue's `areas` and area map (the
loop's own), and the PR slot and refresh (FLOW-05, FLOW-10, already in
the rules). They name `wf`, `TEMPLATE.md` and `main` where the current
wording already does; making them neutral is deferred (a10).

## Validation plan

- **Tier 0/1 (TEST-01, TEST-02).** `make test` and `make lint` (both run
  `tools/workflow`, `Makefile:33-35, 66-68`): the reason text's tests,
  golden files and `wf` command test pass with the new string; nothing
  else moves. `make enforcement` (`Makefile:89-90`), since the branch
  changes `tools/workflow/check/`: the merge base's tier 1 reports
  nothing the branch's no longer does (FLOW-14's comparison runs in CI
  for every branch, a process item's included).
- **Text checks**, run by hand at Implement and recorded in the PR:
  `grep -rn "declares no areas" tools/ docs/development/ .claude/`
  finds nothing; `grep -rn "patch-id" docs/development/` finds nothing;
  `grep -rn "one feature at a time" docs/development/` finds nothing;
  `grep -rn "which is expected until" .claude/` finds nothing;
  each proposed paragraph appears in `process.md` byte for byte as
  approved (FLOW-14's agent part).
- **Tier 1's `rule-index`** passes: no rule is added, retired or renamed.
- **No native validation.** The change is text and two strings; no
  host-specific behaviour (`native_validation: []`, no `tier2_gaps`).
- **After the merge.** The next `wf` build on the workflow branch
  regenerates `QUEUE.md` with `is undeclared` in its relations table; no
  attended step is needed.

## Implementation plan

For `x2-implement`, after the review approves; each task cites what it
must satisfy.

1. Merge current `main` into the branch (FLOW-10, FLOW-17).
2. `process.md`: FLOW-02's `areas` bullet and last paragraph, word for
   word as approved (FLOW-02, FLOW-14).
3. `process.md`: FLOW-03's paragraph after the phase table, as approved
   (FLOW-03, FLOW-14).
4. `process.md`: FLOW-05's end of **How two items relate** and its
   **colliding** bullet, and the opening paragraph at lines 27-33, as
   approved (FLOW-05, FLOW-14).
5. `wf`: `queue/relation.go:180` returns `<item> is undeclared`, and the
   `Undeclared` comment is reworded; update `queue/relation_test.go`,
   `queue/limits_test.go`, `queue/testdata/queue-today.md`,
   `queue/testdata/queue-later.md` and `cmd/wf/declare_test.go` to the new
   text (FLOW-05, TEST-01, LOOP-09).
6. `check/declared.go:148`: "the design leaves areas out" (FLOW-02,
   TEST-01).
7. `process.md`: FLOW-16's third paragraph, as approved (FLOW-16,
   FLOW-14).
8. `TEMPLATE.md`: the `areas` comment's added line (FLOW-02, FLOW-14).
9. Delete the stale clause, that `wf areas -list` and `-check` "exit 1
   while the queue state has no `areas.yaml`, which is expected until
   `loop-parallel`'s `x4-state` writes it", and any sentence that only
   explains it, from `.claude/agents/wf-review.md`,
   `.claude/skills/workflow-intake/SKILL.md`,
   `.claude/skills/workflow-tick/SKILL.md` and
   `.claude/skills/workflow-human/SKILL.md` (a12, Jeff's fold-in;
   LOOP-09: a process item's branch). Nothing else in those files changes.
10. `make test`, `make lint`, `make enforcement`; the text checks of the
   Validation plan (TEST-01, FLOW-17); push.
11. Archive this design on the branch with its summary, and open the PR
    to `main`, with CI green (FLOW-03 process table, FLOW-12, FLOW-08).

## Design review

- **r1, 2026-11-17: approved at `d0b4d46`** (Jeff, workflow-human, FLOW-19),
  read at `2fb5808`. Rule changes: **rule change FLOW-02 approved at
  `d0b4d46`** (approved with a change: the `[]` sentence adds "though an item
  whose home repository has no map is undeclared whatever it declares
  (FLOW-05)"); **rule change FLOW-03 approved at `d0b4d46`** (approved with a
  change: "a feature's refresh runs both, including one taken while the
  pull request waits at Merge, and a process item's refresh runs
  neither"); **rule change FLOW-05 approved at `d0b4d46`** (approved with a
  change: "A declared item whose areas are `[]` changes nothing but its
  own design (FLOW-02)"), the opening paragraph included; **rule change
  FLOW-16 approved at `d0b4d46`**, as drafted. Assumptions a1 to a12
  confirmed as recommended: a1 (a), a2 (a), a3 (a), a4 (a) with
  `check/declared.go:148` in scope, a5 amend, a6 (a), a7 (a) (i), a8 (a),
  a9 add, a10 defer, a11 as proposed, a12 file separately. Folded in at
  `d0b4d46` besides the three wording changes, all found by a report-only
  check of the draft against `main` at 0cd3c02: Decision 3 and the summary
  table qualified for a home with no map; the patch-id rationale (Problem
  4, Decision 1, a1, FLOW-16's Why) says Decision 23 replaced the proof
  rather than that Jeff rejected it as unsafe; seven citations corrected;
  a9's quoted line matches the template block; Decision 5's question
  references; the Validation plan's two `grep` checks take `-r`.
- **r1 addendum, 2026-11-17** (Jeff, workflow-human): a12 changed from
  "file separately" to **fold in**, on the out-of-band streamlining
  assessment's proposal to batch wording-only process items. Task 9 deletes
  the four stale clauses; `skill-areas-yaml-lines` is closed as merged into
  this item. No rule wording changes; the approval at `d0b4d46` stands for
  the rule changes, and this addendum covers the added task.

## Consolidation

A process item has no Publish phase and no consolidation pass (FLOW-03).
The implement step merges current `main` before it opens the PR, and its
open PR is refreshed like a feature's (FLOW-10).

## Rollback notes

None.

## Archive summary

(Filled in at Implement, before the PR opens: FLOW-12.)
