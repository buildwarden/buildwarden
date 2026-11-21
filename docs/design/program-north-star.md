---
feature: program-north-star
status: draft
drivers: []              # rule text, skills and the queue's limit counting; no driver tree is reached
seams: []                # no GLOBAL-08 seam changes; FLOW-20 says how program items may change seams
surfaces: []             # `wf` and the skills are the loop's tools, not a user-facing surface of the product
repos: []                # only buildwarden/buildwarden
areas: [rules, loop, designs]  # process.md (FLOW-05, FLOW-20), ux-cli-config.md (UX-03),
                         # orchestrator.md (ORCH-03); tools/workflow/queue and cmd/wf; the
                         # workflow-intake, workflow-tick and workflow-human skills; this design
security_boundary: none
alters_relationships: false  # no product component's role or interaction changes (FLOW-02)
design_review: required  # a process item's design always gets its review (FLOW-03, FLOW-19);
                         # it also has rule changes (FLOW-14)
native_validation: []    # pure functions of the queue state and rule text
tier2_gaps: []
rules: [FLOW-02, FLOW-03, FLOW-05, FLOW-06, FLOW-07, FLOW-08, FLOW-09, FLOW-10, FLOW-14,
        FLOW-18, FLOW-19, FLOW-20, LOOP-02, LOOP-04, LOOP-07, LOOP-09, GLOBAL-06, GLOBAL-08,
        GLOBAL-10, GLOBAL-11, UX-01, UX-03, UX-04, ORCH-03, RELAY-05, LINT-05, TEST-01, TEST-02]
rule_changes:            # the enforcement (queue/limits.go, queue/queue.go, queue/summary.go,
                         # cmd/wf/main.go) is the loop's machinery, changed on the process route
                         # (FLOW-14). Paths list docs files only: tier 1's front-matter check refuses
                         # machinery paths under tools/workflow/, so those files are named under
                         # Rule changes instead
  - {rule: FLOW-20, change: amend, paths: [docs/development/process.md]}
  - {rule: FLOW-05, change: amend, paths: [docs/development/process.md]}
  - {rule: UX-03, change: amend, paths: [docs/development/ux-cli-config.md]}
  - {rule: ORCH-03, change: amend, paths: [docs/development/orchestrator.md]}
concepts: [workflow, componentization, program, freeze, hold, limits, waiting-on-human,
           process-item, program-payoff, compatibility, deprecation, interfaces, intake]
related: [componentization.md, loop-parallel.md, alters-slot.md, merge-confirm-attended.md,
          queue-positions.md]
---

# The componentization program is the north star

A process item (FLOW-03, LOOP-09), and a program item: it lists
`componentization.md` in `related` (FLOW-20). It holds no worker. It
amends FLOW-20 so that the program has the queue's priority until it
completes, with as much of it in flight as the relations allow, and says
which interfaces a program item may break on the way and which never break.

Words used throughout. **wf** is the loop's tool, `tools/workflow/`. The
**program** is the work `componentization.md` sets out. A **hold** is an open
blocking question that keeps an item's steps from being offered (FLOW-07).
Rule wording and code are `main` at `be13847`; queue state is the `workflow`
branch as of 2026-11-21 19:30 UTC. The **scouts** are three read-only passes
of 2026-11-21 (queue rules, compatibility rules, program shape); the figures
below are theirs, rechecked against the files named.

## Problem

Jeff, 2026-11-21, in his words:

> make this an explicit process override that basically states this
> large-scale refactor is effectively the north star project until it's
> complete. All work prioritization is based on achieving that goal under
> maximum parallelization, even if it is temporarily incompatible or
> breaking with the existing interfaces. While the CLI, config, and ledger
> will largely be expected to keep mostly similar interfaces/specs, every
> delay increases overall development costs where-ever it could be
> prioritized and parallelized post-refactor.

And for loop and process work during the program:

> it has to either reduce the number of non-meaningful human interactions
> or speed up the workflow by enough that it will reduce the time spent by
> more than its implementation time

The same day he held two security fixes (relay-collector-failure-fails-open,
as a follow-up of the relay separation; vm-from-host-path) and two features
(prebuilt-artifacts, workdir-in-context) until the program completes. There
is no release before the program ends, and usage is low.

The rules as written do not say this, and in places say the opposite:

1. **FLOW-20 exempts what Jeff held.** Its freeze exempts "a fix for a
   regression on `main` (FLOW-09) and a fix for a security problem"
   (`process.md:931-934`). Each of Jeff's four holds is a hand-written
   exception to the rule.
2. **Held items still fill the limits.** Three of the five in-progress
   places (`config.yaml` `limits.in_progress: 5`) are held by items Jeff
   froze: prebuilt-artifacts, workdir-in-context and
   relay-collector-failure-fails-open. `counted()` (`queue/limits.go:129`)
   and `WaitsOnHuman()` (`limits.go:382-389`) know nothing of a hold, so the
   three also hold an in-progress place each, prebuilt-artifacts holds an
   `alters_relationships` slot from its Plan claim, and all three count as
   waiting on Jeff. driver-contract (K1) declares no `areas`, so it collides
   with every item in this repository and its in-progress set is every
   feature in progress. With a copy of the state, the queue-rules scout saw
   `wf next` print `none` once the two ready steps were leased, and pass
   over the next program item without a line.
3. **Holds that decide nothing come back every day.** `QUEUE.md`'s Needs
   Jeff opens "3 items in progress wait on you (limit 8): 3 questions", all
   three holds Jeff already decided, each restated in full. The digest's
   Blocked on Jeff lists them too. On 2026-11-20 the cloud run took 0 steps
   at 8 of 8 waiting.
4. **A hold does not say when it ends.** `wf new -hold` (`PutOnHold`,
   `queue/queue.go:831-855`) writes the option `resume it`. Jeff's holds of
   2026-11-21 were written by hand to name the program's end.
5. **Compatibility work costs the program review time.** UX-03
   (`ux-cli-config.md:45-47`) requires an alias and a removal window for
   every renamed or removed flag and config key, and ORCH-03
   (`orchestrator.md:70-71`) says config changes are additive. K1's design
   spent assumed questions a5, a25 and its guard question on aliases for
   seven renamed keys and eleven ignored flags. FLOW-20 relaxes neither.
6. **Process items rank ahead of all of it.** FLOW-05 class 2 puts every
   process item ahead of every fix and feature. wf-state-migration-step
   (position 2, queued) needs a 200 to 400 line comparator for a case seen
   once (`merge-confirm-attended.md`, a8). Nothing asks a process item to
   meet Jeff's bar.
7. **The program's end is stated three ways.** Decision 14 of
   `componentization.md` freezes other work "until the large majority of the
   program has landed" (`:173-174`); a4 says Jeff ends it "no earlier than
   the pilot driver's release (wave 4)" (`:222-224`); Jeff now says "until
   it's complete".

## Prior art

- **`componentization.md`** (P0) set the program, its phases and order, and
  the freeze (Decision 14, a4). a5 raised the `alters_relationships` limit
  from 1 to 3 for the program; a8 closed the then-held items for re-intake
  rather than leave them on hold. This item keeps a5's number and keeps
  Jeff's 2026-11-21 holds as holds (he chose that, not closure).
- **FLOW-20** as written (`process.md:920-950`) defines a program item by
  `related`, the freeze, Moves and Overview. This item amends it; it adds no
  second rule.
- **`loop-parallel.md`** and **`alters-slot.md`** made the limits count per
  collision set. That is why declared areas, not larger numbers, are what
  frees disjoint program work (a5 below).
- **`merge-confirm-attended.md`** gave the process Close its seeded state
  step (`state_step`), which carries this item's queue changes.
- **GLOBAL-08** already lets seams and orchestrator internals move before
  1.0 (`global.md:147-151`), spec-first (LOOP-02, LOOP-07.5 allows a branch
  to name an unreleased commit). For internals and seams the Interfaces
  bullet restates what is allowed; its real change is to UX-03 and ORCH-03.

## Decisions

1. **The program is the north star until it completes (a1).** It ends when
   its last phase has merged (the Order's wave 6, K5), or earlier by Jeff's
   word. The process item that retires FLOW-20 lists every item held until
   the program's end, and Jeff answers each.
2. **Non-program work is held, fixes included (a2, a3).** Intake files an item
   that is neither a program item nor a process item on hold, a security or
   regression fix included, unless Jeff releases it. Two kinds are program
   work and never held: the fix for a failing `main` (no branch is refreshed
   while `main` fails, FLOW-10, so holding it would stop the program), and a
   fix that `follows` a program item. The urgent mark does not release a hold.
3. **A hold names when it resumes, and a held item counts toward no limit
   (a4).** A hold question gains `resumes` (free text: `program-end`, a
   named item's merge, or Jeff's words). wf treats an item with an open
   question that has `resumes` as held: it holds no limit, does not wait on
   the human, and `QUEUE.md` lists it once under **Held** with its resume
   point, not under Needs Jeff. `wf new -hold` requires `-resumes`.
   - *Rejected: Jeff raises `in_progress` and `waiting_on_human` by hand.*
     No code, but the numbers drift with every hold, and the holds stay in
     Needs Jeff.
   - *Rejected: close the held items for re-intake (a8's route).* Jeff chose
     holds; closing loses the branches' place.
4. **Parallelism comes from declared areas, not new limits (a5).** The
   limit numbers stay. A program item declares its repositories and areas
   at intake where the program's design gives them, and by the end of its
   Design at the latest. Its design is scoped so its review fits one
   sitting; a larger one is split at Design into items that can run side by
   side.
5. **No new order rule (a6).** With non-program work held, program items
   lead by default. Intake places a program item above any released
   non-program item of its class unless Jeff says otherwise. LOOP-04 is
   unchanged: process items stay class 2.
6. **Loop work meets Jeff's bar (a7, a8).** A process item filed while
   FLOW-20 stands carries a **Program payoff** section, judged at its design
   review; one that neither removes such interactions nor saves more time
   than it costs is held or closed. It applies to P1's remaining move too.
   wf-state-migration-step stays queued; its design shows its payoff (a8).
7. **Interfaces may break for the program, within fixed limits (a9 to a12).**
   A program item may break Go APIs, internals and the driver contract (as
   GLOBAL-08 already allows), a seam by a new pre-1.0 release of its
   specification first, and CLI flags, subcommands, config keys, environment
   variables and output forms with no alias or window, only where the
   program's change needs it; the CLI and config otherwise keep their shape
   (the human: "mostly similar"), and a rename for its own sake is not a
   program break. A released form is refused with an error naming its
   replacement (UX-04), never silently ignored. These never break: the ledger
   wire format and its signatures (additive only), the reader's support for
   every ledger a release wrote, a written schema `$id`, and every security
   boundary. `main` may lack a capability for a while when the design names
   the gap.
8. **Enforcement stays small.** The hold counting is wf code, because the
   limits are (FLOW-05, automated). Program membership, the payoff and each
   interface break stay agent judgment at intake and the design review, as
   FLOW-20's check says today. A tier-1 UX-03 finding on a deliberate break
   is answered with a `Tier1-Allow` trailer naming FLOW-20 (a12), as Moves
   already does.

## Questions

### Blocking

None.

### Assumed

- **a1. When the program ends.** Flagged: departs from componentization a4
  ("no earlier than the pilot driver's release (wave 4)") and Decision 14
  ("the large majority"), and sets FLOW-20's closing paragraph.
  - (a) When its last phase has merged (wave 6, K5), or earlier by Jeff's
    word.
  - (b) No earlier than the pilot driver's release (wave 4), as a4 said.
  - (c) When Jeff says, with no phase named.

  Recommended: (a). It is "until it's complete" in the program's own terms,
  and the earlier exit stays his.
- **a2. Security and regression fixes during the program.** Flagged:
  removes FLOW-20's two exemptions, which componentization a4 confirmed, and
  sets the Hold bullet's wording.
  - (a) Held like other non-program work unless Jeff releases one. The fix
    for a failing `main` and a fix that `follows` a program item are
    program work and never held.
  - (b) Keep both exemptions; Jeff may hold a fix whose code the program
    replaces, and each digest lists it as an open security problem.
  - (c) Keep FLOW-20 as written; the 2026-11-21 holds stay one-off.

  Recommended: (a). It is what Jeff did on 2026-11-21 for two security
  fixes, and the red-`main` exception keeps the program's own refreshes
  running (FLOW-10). A non-program follow-up filed by an unattended session
  is held with the followed item's resume point.
- **a3. The urgent mark.** Flagged: sets FLOW-05's waiting-limit sentence.
  - (a) It does not release a hold: Jeff releases a security fix first, and
    may then mark it urgent.
  - (b) Marking a fix urgent also releases its hold.

  Recommended: (a). Release and urgency are two decisions; (b) would let one
  word do both without the release being recorded.
- **a4. Held items and the limits.** Flagged: sets FLOW-05's counting
  sentences and adds wf code.
  - (a) A hold names its resume point (`resumes`); an item with an open hold
    holds no limit, does not wait on the human, and is listed once under
    **Held** in `QUEUE.md` and the digest.
  - (b) No code: Jeff raises `in_progress` and `waiting_on_human` in
    `config.yaml` by the number held.
  - (c) Close the held items for re-intake, as componentization a8 did.

  Recommended: (a). About 60 lines of Go, and it removes the daily restating
  of decided holds, which is exactly Jeff's "non-meaningful human
  interactions".
- **a5. The limits for program items.** Flagged: sets FLOW-20's Areas
  bullet.
  - (a) Numbers unchanged (5, 3, PR slot 1, 8); a program item declares its
    areas by the end of its Design, and its design is scoped to one
    sitting's review.
  - (b) The in-progress and `alters_relationships` limits neither count nor
    refuse program items.
  - (c) Raise the numbers (for example 8 and 5).

  Recommended: (a). The limits count per collision set (FLOW-05), so
  declared, disjoint items already run side by side; the scouts found 48 of
  QUEUE.md's 52 non-disjoint pairs exist only because one side is
  undeclared. K1 is back at Design (`f1-design-r4`) and would declare its
  areas there. (b) lets colliding program items pile up consolidations the
  limits exist to bound.
- **a6. Where program items rank.**
  - (a) No new order rule: non-program items are held, so program items
    lead; intake places a program item above any released non-program item
    of its class unless Jeff says otherwise.
  - (b) wf ranks program items ahead of non-program items within classes 3
    and 4 (a program mark on the queue item).
  - (c) Program items ahead of process items too (amends LOOP-04, LOOP-09).

  Recommended: (a). With (a2) the only unheld non-program items are ones
  Jeff released, and he places them. (b) needs a queue mark wf cannot derive
  today (the queue does not record `related`).
- **a7. Jeff's bar for loop work.** Flagged: sets FLOW-20's Loop work
  bullet, in his words.
  - (a) Every process item filed while FLOW-20 stands, P1's remaining move
    included, carries a **Program payoff** section (interactions that decide
    nothing removed, per week; or program time saved before the end against
    its steps and review), judged at its design review; one that does
    neither is held or closed.
  - (b) Judged at intake only, no design section.
  - (c) As (a), and `wf check` requires the section to be present.

  Recommended: (a). The design review is already a human step, so the bar
  costs no extra interaction; (c) is code for a check the review makes
  anyway.
- **a8. wf-state-migration-step** (queued, position 2).
  - (a) Held until the program's end, by the state step.
  - (b) Left at position 2; its own review judges its payoff.
  - (c) Closed.

  Recommended: (b). Jeff kept it queued on 2026-11-21 under this bar; its
  own review judges its payoff.
- **a9. CLI flags, subcommands, config keys, environment variables and
  output forms.** Flagged: departs from UX-03 and ORCH-03 for program
  items.
  - (a) A program item may rename or remove one with no alias or window, only
    where the program's change needs it; the CLI and config otherwise keep
    their shape (the human: "mostly similar"), and a rename for its own sake
    is not a program break. A form a tagged release exposed (v0.6.0 or
    earlier) is refused with an error naming its replacement (UX-04), never
    silently ignored; one no release exposed may simply go. Reference docs
    change in the same change (UX-01), with a migration note for a released
    form.
  - (b) Only forms no release exposed may break; released ones keep UX-03.
  - (c) UX-03 as written.

  Recommended: (a). "Even if it is temporarily incompatible or breaking",
  with no release before the end and low usage; the error keeps the break
  loud. Note that a removed config key is silently ignored today
  (`cmd/warden/config.go` decodes without checking undecoded keys), so the
  item that removes a released key adds that error.
- **a10. The ledger.** Flagged: sets which ledger rules FLOW-20 keeps.
  - (a) `specs/build-ledger/v1.md` and its signatures change only
    additively (new metadata schema versions under RELAY-05); the reader
    reads every ledger a release wrote; a written schema `$id` never
    changes.
  - (b) A v2 allowed per item on Jeff's approval, the reader keeping v1.
  - (c) Frozen until the program ends.

  Recommended: (a). It is Jeff's "mostly similar specs" made exact, and it
  is what RELAY-05 already says.
- **a11. Temporary gaps on `main`.** Flagged: sets the Interfaces bullet;
  touches GLOBAL-06.
  - (a) `main` may lack a capability for a while when the design that
    removes it names the gap and the item, or a filed program item,
    restores it before the program ends; `main` still passes its own checks
    and no such state opens a security boundary.
  - (b) Every merge keeps every driver and command fully working.
  - (c) Freely, since nothing releases before the end.

  Recommended: (a). It allows the parallel breaks Jeff asks for without a
  boundary ever open on `main` (two relays coexist until M6).
- **a12. Tier 1's UX-03 check on a deliberate break.**
  - (a) The breaking commit carries a `Tier1-Allow` trailer whose reason
    names FLOW-20 (LINT-05), as Moves does; no tier-1 code changes.
  - (b) Tier 1 compares surfaces against the latest release tag, so
    unreleased removals pass without a trailer (`tools/tier1/surface`).

  Recommended: (a). It costs an agent a trailer, not Jeff an interaction;
  (b) is tier-1 code that only saves trailers.

## Design

### The rule

FLOW-20 is rewritten (full wording under **Rule changes**) with five
bullets, two of them unchanged:

- **Hold** replaces **Freeze** (Decisions 1-3).
- **Areas** (Decision 4).
- **Loop work** (Decision 6).
- **Interfaces** (Decision 7).
- **Moves** and **Overview**, unchanged.

The closing paragraph lists what stays as written, now naming GLOBAL-09 to
GLOBAL-11, RELAY-05, RELAY-08, the SEC rules, UX-01, UX-04 and the LOOP
rules beside the ones it named. The program's end and its retirement are one
paragraph. FLOW-05 gains two clauses (holds), and UX-03 and ORCH-03 each
gain a pointer to FLOW-20, so an agent reading either alone does not apply
the window to a program item.

### wf: holds that name their end and count toward nothing

- **`Question.Resumes`** (`resumes,omitempty`, `queue/queue.go:285-293`):
  when the item resumes. Free text; `program-end` by convention.
- **`Feature.Held() bool`**: an open blocking question with `Resumes` set.
- **`counted()`** (`limits.go:129`) returns false for a held item. Its
  callers then skip it: the holders of the three relation limits
  (`limits.go:146`), the start (`takesStart`), the PR-slot wait and the
  urgent check.
- **`WaitsOnHuman()`** (`limits.go:382-389`) skips a held item, as it skips
  a tracked one.
- **`PutOnHold(why, resumes, ...)`** (`queue.go:831-855`): `resumes`
  required; it is stored on q1, and the option and recommendation read
  `resume: <resumes>`. `wf new -hold WHY` refuses a missing `-resumes`
  (`cmd/wf/main.go`, where `-hold` is parsed, about `:326` and `:811`).
- **`QUEUE.md`** (`queue/summary.go`, `writeNeedsHuman` about `:187-206`): a
  held item leaves "Reviews, questions and escalations" and "Not counted";
  a new **Held** subsection after Needs Jeff lists each in one line:
  `<slug>: held until <resumes>`. The waiting count no longer includes it.

Holding an item already in progress stays a hand edit in an attended
session (as `PutOnHold`'s comment says today); the state step does the
present ones. Answering a hold (`wf answer`) is unchanged and releases it.

### Skills

- **`workflow-intake`**:
  - `SKILL.md:41-43`: "agree too whether the item is a program item; one that
    is not, and is not a process item, is filed on hold (FLOW-20), with
    `-hold "<why>" -resumes "<when>"`, the resume point being the program's end
    unless Jeff names another, whatever the item fixes, unless Jeff says in
    this sitting to file it unheld. A program item gets its `-repos` and
    `-areas` now where the program's design gives them, and is placed above
    any released non-program item of its class. A process item's design will
    carry a Program payoff (FLOW-20): say so."
  - `:53-62`: held items do not count toward the waiting limit; there is no
    security exemption left to judge; offer `-urgent` only for a security
    fix Jeff releases that follows no item, and say the mark does not
    release a hold.
  - `:104-110` (the same-sitting lane): "it is a program item, or Jeff
    releases it in this sitting", and its Program payoff stated.
  - A regression's follow-up (`:121-145`): a new bullet. While FLOW-20
    stands, a follow-up of a program item is filed as usual; a follow-up of
    any other item adds `-hold "<the regression>" -resumes "<the followed
    item's resume point, else program-end>"`. The fix for a red `main` is
    never held.
- **`workflow-tick`** (`SKILL.md:250-255`): the digest gains a **Held** line
  (slug and resume point, one line for all), and Blocked on Jeff no longer
  lists a held item.
- **`workflow-human`**: step 2 (`SKILL.md:47-50`) adds that held items are
  shown apart with their resume points and not asked about again; step 3
  adds that a process item's review checks its Program payoff (FLOW-20) and
  a program item's review checks each interface break its design names and
  that it declares its areas.

## Defaults and overrides

- **A hold's resume point.** `program-end` unless Jeff names another at
  intake. No default in wf: `-resumes` is required with `-hold`.
- **A non-program item during the program.** Held. Overridden only by
  Jeff's word, at intake or by answering the hold.
- **Interface breaks.** None by default; each is named in its design and
  approved at its review. A released CLI or config form that is removed
  gets an error naming its replacement, never a silent fallback; a path
  that tries the new interface and then the old one is a fallback under
  GLOBAL-10 and GLOBAL-11 and is named or not made.
- **The limit numbers** stay in `config.yaml`, Jeff's to change.

## Cross-driver impact

None. No driver code is reached by this item. Program items that break an
interface name the drivers they touch in their own designs.

## Program payoff

This item is a process item filed while FLOW-20 stands, so it meets its own
bar (Decision 6).

- **Interactions removed.** Today 3 decided holds open Needs Jeff in every
  `QUEUE.md` and the digest's Blocked on Jeff on each run: at one run a day,
  21 restatements a week, more as holds are added. After it, 0. Program
  designs stop asking alias and window questions: K1 alone asked 3 (a5, a25
  and its guard question); with 15 to 20 program designs left, a few
  questions each.
- **Time saved.** Three in-progress places and an `alters_relationships`
  slot are freed now, so the program's next items (S2's and S3's specs,
  seam 8, M1 once its repository is listed) can start beside K1 instead of
  after the held items, which never move until the program ends.
- **Cost.** 5 steps (4 + 1 state step) and one review sitting.

## What this item does not do

These came up in scouting. They are Jeff's to start, each by intake or in an
attended session, and none needs a rule change here:

- Filing the remaining phases now (S2, the S3 spec, M1, K2) with their areas
  declared from the program's phase rows.
- Adding `buildwarden/buildwarden-relay` to `config.yaml`'s `repositories`,
  so M1 can be queued (an attended state change).
- An area map for a repository not yet adopted (buildwarden-specs). FLOW-05
  keeps a map where the repository's profile says; allowing one earlier is a
  FLOW-05 and wf change of its own.
- Raising `native_budget` while K2, S3 and M6 fan out to every host.
- Deferring P1's move to the loop repository: under Decision 6 its design
  shows its payoff or is held.

## Rule changes

### FLOW-20 (amend), `docs/development/process.md`

Severity and check unchanged: MUST, `agent-judgment` (the automated part of
the hold lives in FLOW-05's limits).

**Why:** Jeff's decision of 2026-11-21 (Decisions 1-8).

**Current wording** (`process.md:920-950`):

> ### FLOW-20 — The componentization program has the queue to itself
>
> **Severity:** MUST
> **Check:** agent-judgment
>
> The componentization program is the work set out in
> [`componentization`](../design/componentization.md): the relay, the
> drivers, the kit, the specs and the loop become separate components coupled
> through written seams. A *program item* is a queue item whose design lists
> that design in `related`. While this rule stands:
>
> - **Freeze.** Intake files a new item that is not a program item on hold,
>   with a blocking question only the human answers, so the loop takes none
>   of its steps. Two kinds of item are exempt: a fix for a regression on
>   `main` (FLOW-09) and a fix for a security problem.
> - **Moves.** A program item whose change only moves code between packages,
>   modules or repositories, with no change in behaviour, says so in its
>   design. Where the move trips a path-coupling check, it carries a
>   `Tier1-Allow` trailer whose reason names this rule (LINT-05). Review
>   judges that the change is only a move.
> - **Overview.** A program item may leave FLOW-15's overview sweep to the
>   last item of its phase, which the phase's designs name.
>
> Nothing else is relaxed. GLOBAL-06, the BOOT rules, the relay's ledger and
> key rules (RELAY-01 to RELAY-03), FLOW-08, FLOW-14, FLOW-18 and FLOW-19
> apply to program items as written.
>
> The human ends the program with a process item that retires this rule.
>
> The agent part: intake and the design review judge whether an item is a
> program item or exempt.

**Proposed:**

> ### FLOW-20 — The componentization program is the north star
>
> **Severity:** MUST
> **Check:** agent-judgment
>
> The componentization program is the work set out in
> [`componentization`](../design/componentization.md): the relay, the
> drivers, the kit, the specs and the loop become separate components coupled
> through written seams. A *program item* is a queue item whose design lists
> that design in `related`. The program is the north star until it completes
> (the human, 2026-11-21): work is prioritized to finish it, with as many of
> its items in flight at once as the relations allow, even where an item
> breaks an interface for a while. While this rule stands:
>
> - **Hold.** An item that is neither a program item nor a process item (Loop
>   work, below) is held unless the human releases it; a fix for a security
>   problem or for a regression is held like any other. Intake files it on
>   hold, with a blocking question only the human answers that names when it
>   resumes: the program's end, unless the human names another point. An item
>   already queued or in progress that is neither a program item nor a process
>   item is held the same way unless the human has released it, and keeps its
>   branch and design; on resume it returns to Design for what the program
>   changed (FLOW-06). Two kinds of item are program work and are never held:
>   the fix for a failing `main` (FLOW-09), since no branch is refreshed while
>   `main` fails (FLOW-10), and a fix that `follows` a program item. A held
>   item holds no limit and does not wait on the human (FLOW-05). The human
>   releases a held item by answering its question, or files one unheld at
>   intake; a released item takes its class (FLOW-05), and marking a fix
>   urgent does not release it.
> - **Areas.** A program item names its repositories and areas (FLOW-02) at
>   intake where the program's design gives them, and by the end of its
>   Design at the latest, so that only real collisions keep it waiting
>   (FLOW-05). Its design is scoped so that its review fits one sitting; a
>   larger one is split at Design into items that can run side by side.
> - **Loop work.** A process item (FLOW-03) carries a **Program payoff**
>   section. It shows, with figures and how they were reached, either the
>   human interactions it removes that decide nothing (a confirmation with
>   one possible answer, a decision restated on every run, a step only the
>   human may complete with nothing to judge), counted per week, or the
>   program time it saves before the program ends against its own cost in
>   steps and review. Its design review declines one that neither removes
>   such interactions nor saves more program time than its own cost, and the
>   item is then held or closed.
> - **Interfaces.** A program item may break an interface for the program,
>   and its design names each break:
>   - the Go API of any package, the driver contract, and the internals of
>     the drivers, relay, executor and collector (GLOBAL-08), within the
>     BOOT rules and RELAY-08;
>   - a seam's behaviour, by a new pre-1.0 release of its specification
>     first (GLOBAL-08, LOOP-02), each implementation adopting it as LOOP-07
>     sets out;
>   - a CLI flag, subcommand, config key, environment variable or output
>     form, with no alias or removal window (UX-03, ORCH-03), only where the
>     program's change needs it; the CLI and config otherwise keep their
>     shape (the human: "mostly similar"), and a rename for its own sake is
>     not a program break. A form a
>     tagged release exposed is refused with an error that names its
>     replacement (UX-04), never silently ignored; one no release exposed
>     may simply go. The reference docs change with it (UX-01), with a
>     migration note for a released form. Where tier 1 flags the removal,
>     the commit carries a `Tier1-Allow` trailer whose reason names this
>     rule (LINT-05).
>
>   These never break: the ledger wire format (`specs/build-ledger/v1.md`)
>   and its signatures, which change only additively; the reader's support
>   for every ledger a release wrote; a schema `$id` already written
>   (RELAY-05); and any security boundary (GLOBAL-06). `main` may lack a
>   capability for a while when the design that removes it names the gap
>   and the item, or a filed program item, restores it before the program
>   ends; `main` still passes its own checks, and no such state opens a
>   boundary. Compatibility kept by trying the new interface and then the
>   old one is a fallback (GLOBAL-10, GLOBAL-11).
> - **Moves.** A program item whose change only moves code between packages,
>   modules or repositories, with no change in behaviour, says so in its
>   design. Where the move trips a path-coupling check, it carries a
>   `Tier1-Allow` trailer whose reason names this rule (LINT-05). Review
>   judges that the change is only a move.
> - **Overview.** A program item may leave FLOW-15's overview sweep to the
>   last item of its phase, which the phase's designs name.
>
> Nothing else is relaxed. GLOBAL-06, GLOBAL-09 to GLOBAL-11, the BOOT
> rules, the relay's ledger, key and claim rules (RELAY-01 to RELAY-03,
> RELAY-05, RELAY-08), the SEC rules, UX-01, UX-04, FLOW-08, FLOW-14,
> FLOW-18, FLOW-19 and the LOOP rules apply to program items as written.
>
> The program ends when its last phase has merged (the program's Order,
> wave 6), or earlier by the human's word. The human ends it with a process
> item that retires this rule; its Close lists every item held until the
> program's end, and the human answers each hold, resuming the item or
> closing it.
>
> The agent part: intake and the design review judge whether an item is a
> program item, and the design review judges a Program payoff and each
> interface break.

**Enforcement:** none new for FLOW-20 itself; the hold's effect on the
limits is FLOW-05's (below).

### FLOW-05 (amend), `docs/development/process.md`

Severity and check unchanged: SHOULD, `automated`.

**Why:** a held item holds no limit and does not wait on the human, and the
urgent mark does not release a hold (Decisions 2 and 3, a3, a4).

**Current wording** (`process.md:369-373`, from "Only holders"):

> Only holders are counted and an item that holds is
> never refused, so no two items wait for each other.
> Process items are not counted because they are short and already ranked
> ahead of fixes and features; one that would disrupt running work declares
> `holds`.

**Proposed:**

> Only holders are counted and an item that holds is
> never refused, so no two items wait for each other.
> Process items are not counted because they are short and already ranked
> ahead of fixes and features; one that would disrupt running work declares
> `holds`. An item on hold, whose open blocking question names when it
> resumes (`resumes`), is not counted while it is held.

**Current wording** (`process.md:401-408`):

> the merge, or a spent budget (FLOW-13); it counts once, and a tracked
> item does not count. This limit counts by no relation and stops only a
> start: work in progress goes on, so the count can pass it. A fix that
> `follows` an item, the fix for a failing `main` (FLOW-09), a security
> fix the human has marked urgent, a process
> item and a tracked item start whatever
> the count. Only the human marks a security fix urgent; the mark lifts
> this limit for it, and nothing else.

**Proposed:**

> the merge, or a spent budget (FLOW-13); it counts once, and neither a
> tracked item nor an item on hold (`resumes`) counts. This limit counts
> by no relation and stops only a
> start: work in progress goes on, so the count can pass it. A fix that
> `follows` an item, the fix for a failing `main` (FLOW-09), a security
> fix the human has marked urgent, a process
> item and a tracked item start whatever
> the count. Only the human marks a security fix urgent; the mark lifts
> this limit for it, and nothing else: it does not release a hold
> (FLOW-20).

**Enforcement:** `Question.Resumes` and `Feature.Held` (`queue/queue.go`),
`counted` and `WaitsOnHuman` (`queue/limits.go`), the **Held** subsection
(`queue/summary.go`), `-resumes` on `wf new -hold` (`cmd/wf/main.go`).

### UX-03 (amend), `docs/development/ux-cli-config.md`

Severity and check unchanged: MUST, `automated + agent-judgment`.

**Why:** UX-03 read alone would hold a program item to the window FLOW-20
lifts (a9).

**Current wording** (`ux-cli-config.md:45-47`):

> Flags, subcommands, and config keys are additive. A rename or removal ships an alias
> plus a deprecation notice with a documented removal window, never a silent break.
> Post-1.0 this tightens, consistent with the seam-stability posture (GLOBAL-08).

**Proposed:**

> Flags, subcommands, and config keys are additive. A rename or removal ships an alias
> plus a deprecation notice with a documented removal window, never a silent break.
> While FLOW-20 stands, a program item may rename or remove one without the alias
> and window, as FLOW-20 says, and still never silently.
> Post-1.0 this tightens, consistent with the seam-stability posture (GLOBAL-08).

**Enforcement:** unchanged; a deliberate break carries a `Tier1-Allow`
trailer naming FLOW-20 (a12).

### ORCH-03 (amend), `docs/development/orchestrator.md`

Severity and check unchanged: MUST, `automated + agent-judgment`.

**Why:** the config half of the same promise (a9).

**Current wording** (`orchestrator.md:70-73`, the paragraph's last three
sentences):

> read is an error. Configuration precedence stays `flag > environment variable >
> the config file > default`. Defaults are sane and changes are additive. A new
> or changed config key is documented in the same change; this is the config half
> of the docs-coupling rule (UX-01).

**Proposed** (one clause added to the second sentence):

> read is an error. Configuration precedence stays `flag > environment variable >
> the config file > default`. Defaults are sane and changes are additive, except
> where FLOW-20 lets a program item break a key. A new
> or changed config key is documented in the same change; this is the config half
> of the docs-coupling rule (UX-01).

**Enforcement:** unchanged.

## Validation plan

- **Tier 0** (TEST-01, TEST-02; `make test`), states built with `Add`,
  `PutOnHold`, `Claim` and `Complete`:
  - `queue`: `Held` is true for an open blocking question with `resumes`,
    false once answered and false without `resumes`. With five features in
    progress, three of them held, a sixth colliding feature may start (it
    is refused when none is held). A held item with a Plan claim holds no
    `alters_relationships` slot. `WaitsOnHuman` skips a held item.
    `PutOnHold` refuses an empty `resumes` and writes `resume: <resumes>`.
  - `summary`: a held item appears once under **Held** with its resume
    point, not in Needs Jeff, and the waiting count excludes it.
  - `cmd/wf`: `wf new -hold WHY` without `-resumes` is refused; with it,
    q1 carries `resumes`.
- **Text checks:** the FLOW-20, FLOW-05, UX-03 and ORCH-03 blocks match the
  approved wording byte for byte; `grep` finds no "exempt there" or "FLOW-20's
  exemption" in `.claude/`.
- `make test`, `make lint`, `tools/workflow/flow18-check.sh`, `make
  enforcement`.
- **The real check:** after the state step, `dist/wf status` shows the held
  items under **Held**, the waiting count without them, and
  driver-contract's in-progress set without them.
- No native host is needed.

## Implementation plan

A process item has no Plan phase; this is the work of `x2-implement`. Each
step writes its tests first, sees them fail on the code before it (TEST-01,
TEST-02), and ends with `make test` green.

1. `queue/queue.go`: `Question.Resumes`, `Feature.Held`, and `PutOnHold`
   with `resumes` (FLOW-05, Decision 3, a4).
2. `queue/limits.go`: `counted` and `WaitsOnHuman` skip a held item; their
   comments (FLOW-05, a4).
3. `queue/summary.go`: the **Held** subsection; held items leave the other
   Needs Jeff lists and the waiting count (a4).
4. `cmd/wf/main.go`: `-resumes`, required with `-hold`; the usage text
   (`:20`, `:58-61`).
5. `docs/development/process.md`: FLOW-20 and FLOW-05 as approved;
   `docs/development/ux-cli-config.md` UX-03 and
   `docs/development/orchestrator.md` ORCH-03 as approved (FLOW-14).
6. Skills as under **Design, Skills**: `workflow-intake`, `workflow-tick`,
   `workflow-human`.
7. `make test`, `make lint`, `flow18-check.sh`, `make enforcement`, the text
   checks.
8. Merge current `main`, archive this design with its summary, and open the
   PR with CI green (FLOW-03 process phase 2, FLOW-12).

**State step** (`x4-state`, recorded at review with `wf state-step`;
Jeff's, after the merge):

1. Add `resumes:` to each open hold question:
   - `program-end`: prebuilt-artifacts, workdir-in-context,
     vm-from-host-path, configuration-bugfixes, auto-tidy-pull-request,
     relay-exit-code-unreadable;
   - relay-collector-failure-fails-open: "the relay's separation (M1
     onward) has merged", as Jeff held it;
   - runtime-former-values-removal: "the change that prepares 1.0", as held
     on 2026-11-15.
2. config-lookup-test-symlinked-tmp: released by Jeff (2026-11-21) to
   finish; not held. wf-state-migration-step stays queued (a8 (b)).
3. The `workflow` branch's `README.md` schema: `resumes` on a question.
4. `config.yaml` limits unchanged (a5).
5. `dist/wf status` and `QUEUE.md`: the held items under **Held**, the
   waiting count without them; `dist/wf commit`.

Estimate: 5 steps (4 + 1 state step), one run, as filed.

## Design review

Pending.

## Consolidation

A process item has no Publish phase and no consolidation pass (FLOW-03).

## Rollback notes

None.

## Archive summary

Filled in at the item's PR (FLOW-12).
