---
feature: program-north-star
status: draft
drivers: []              # rule text, skills, CI triggers and the queue's tools; no driver tree is reached
seams: []                # no GLOBAL-08 seam changes; FLOW-20 says how program items may change seams
surfaces: []             # `wf`, the skills and CI are the loop's tools, not a user-facing surface of the product
repos: []                # only buildwarden/buildwarden; the specifications repository's own release
                         # workflow change is an item there against its main, filed by the state
                         # step and merged by Jeff in the one-time setup (Decision 16)
areas: [rules, loop, designs, build, specs, book]
                         # rules: process.md, behavioral-linter.md, loop-contract.md, loop-profile.md,
                         # ux-cli-config.md, orchestrator.md; loop: tools/workflow and .claude/;
                         # designs: this design; build: .github/workflows/{main,flow18,release,relay-image}.yml
                         # and .goreleaser.yaml; specs: tools/conformance/{check-pins,pin-specs}.sh;
                         # book: CONTRIBUTING.md's release and pin paragraphs
security_boundary: none  # no product boundary moves; what the loop may merge is the process's own gate (FLOW-08)
alters_relationships: false  # no product component's role or interaction changes (FLOW-02)
design_review: required  # a process item's design always gets its review (FLOW-03, FLOW-19);
                         # it also has rule changes (FLOW-14)
native_validation: []    # pure functions of the queue state, rule text and CI configuration
tier2_gaps: []
rules: [FLOW-02, FLOW-03, FLOW-04, FLOW-05, FLOW-06, FLOW-07, FLOW-08, FLOW-09, FLOW-10,
        FLOW-11, FLOW-13, FLOW-14, FLOW-16, FLOW-17, FLOW-18, FLOW-19, FLOW-20, LINT-03,
        LINT-05, LOOP-01, LOOP-02, LOOP-04, LOOP-07, LOOP-08, LOOP-09, PROFILE-01, GLOBAL-06,
        GLOBAL-08, GLOBAL-10, GLOBAL-11, UX-01, UX-03, UX-04, ORCH-03, RELAY-05, TEST-01,
        TEST-02, TEST-05, TEST-08]
rule_changes:            # the enforcement under tools/workflow/ and .claude/ is the loop's machinery,
                         # changed on the process route (FLOW-14, LOOP-09). Paths list rule paths
                         # only: tier 1's front-matter check refuses machinery paths, so those files
                         # are named under Rule changes instead. release.yml is listed because its
                         # tier1 job is a rule-running job (PROFILE-01); the trigger-only edits to
                         # main.yml and flow18.yml touch no job and are not rule paths
  - {rule: FLOW-20, change: amend, paths: [docs/development/process.md]}
  - {rule: FLOW-03, change: amend, paths: [docs/development/process.md]}
  - {rule: FLOW-04, change: amend, paths: [docs/development/process.md]}
  - {rule: FLOW-05, change: amend, paths: [docs/development/process.md]}
  - {rule: FLOW-06, change: amend, paths: [docs/development/process.md]}
  - {rule: FLOW-08, change: amend, paths: [docs/development/process.md]}
  - {rule: FLOW-13, change: amend, paths: [docs/development/process.md]}
  - {rule: FLOW-17, change: amend, paths: [docs/development/process.md]}
  - {rule: FLOW-19, change: amend, paths: [docs/development/process.md,
                                          docs/design/TEMPLATE.md]}
  - {rule: LINT-03, change: amend, paths: [docs/development/behavioral-linter.md,
                                          .github/workflows/release.yml]}
  - {rule: LOOP-01, change: amend, paths: [docs/development/loop-contract.md]}
  - {rule: LOOP-07, change: amend, paths: [docs/development/loop-contract.md]}
  - {rule: PROFILE-01, change: amend, paths: [docs/development/loop-profile.md]}
  - {rule: UX-03, change: amend, paths: [docs/development/ux-cli-config.md]}
  - {rule: ORCH-03, change: amend, paths: [docs/development/orchestrator.md]}
concepts: [workflow, componentization, program, program-trunk, freeze, hold, limits,
           waiting-on-human, process-item, program-payoff, compatibility, interfaces, intake,
           loop-merge, merge-gate, squash, umbrella-test, records, pre-release, pins]
related: [componentization.md, loop-parallel.md, alters-slot.md, merge-confirm-attended.md,
          queue-positions.md, loop-repository.md, seam-specs.md]
---

# The componentization program is the north star

A process item (FLOW-03, LOOP-09), and a program item: it lists
`componentization.md` in `related` (FLOW-20). It holds no worker. It amends
FLOW-20 so that the program has the queue's priority until it completes; it
gives the program a **program trunk** in each repository it works in, which
the loop merges program items into without Jeff, ending with one squash
merge of each program trunk into `main` that Jeff makes; it takes Jeff out
of every program step that has nothing for him to judge; and it lets the
loop tag pre-releases on program trunks.

Words used throughout. **wf** is the loop's tool, `tools/workflow/`. The
**program** is the work `componentization.md` sets out. A **hold** is an open
blocking question that keeps an item's steps from being offered (FLOW-07).
The **program trunk** is the branch `program/componentization` of a
repository (Decision 8). A **loop merge** is the loop's merge of a pull
request into a program trunk (Decision 9). The **records** are the
workflow branch's `program/records.yaml` (Decision 13). Rule wording and
code are `main` at `be13847`; queue state is the `workflow` branch as of
2026-11-21 19:30 UTC. The **scouts** are five read-only passes of
2026-11-21 (queue rules, compatibility rules, program shape; then trunk and
merge, gates and tags); the figures below are theirs, rechecked against the
files named.

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

For loop and process work during the program:

> it has to either reduce the number of non-meaningful human interactions
> or speed up the workflow by enough that it will reduce the time spent by
> more than its implementation time

And, later the same day, on where the program's work lands and what waits
on him:

> the componentization will effectively have a branch of the main
> buildwarden repository that is effectively used and pushed to instead of
> main with one of the final steps -- once all components are working as
> intended when sourced from their own repositories -- will be the large
> squash merge to main from that branch. Until that point, no publish step
> should wait on me besides any initially necessary for the existing
> scheduled runs to know what to do. I should say much of this will extend
> to the other repositories as well where tagging/dev releases are allowed
> to happen against these prototype feature branches under
> staging/pre-production version tags. I want as little of the forward
> momentum be blocked on me that isn't where I offer notable meaningful
> input that isn't easily understood by my intents and design thus far.

The same day he held two security fixes (relay-collector-failure-fails-open,
as a follow-up of the relay separation; vm-from-host-path) and two features
(prebuilt-artifacts, workdir-in-context) until the program completes; let
config-lookup-test-symlinked-tmp finish; and kept wf-state-migration-step
queued under the loop-work bar. There is no full release before the program
ends, and usage is low.

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
   with every item in this repository. With a copy of the state, a scout saw
   `wf next` print `none` once the two ready steps were leased.
3. **Holds that decide nothing come back every day.** `QUEUE.md`'s Needs
   Jeff opens "3 items in progress wait on you (limit 8): 3 questions", all
   three holds Jeff already decided. On 2026-11-20 the cloud run took 0
   steps at 8 of 8 waiting.
4. **A hold does not say when it ends.** `wf new -hold` (`PutOnHold`,
   `queue/queue.go:831-855`) writes the option `resume it`.
5. **Compatibility work costs the program review time.** UX-03
   (`ux-cli-config.md:45-47`) and ORCH-03 (`orchestrator.md:70-71`) require
   aliases and windows; K1 spent assumed questions a5, a25 and its guard
   question on them. FLOW-20 relaxes neither.
6. **Every program merge is Jeff's.** FLOW-08 (`process.md:468-469`): "No
   agent approves or merges a PR to a trunk, in any repository". wf reads
   one trunk per repository (`Config.Trunk`, `queue/queue.go:91`), CI runs
   only on pushes and pull requests to `main` (`main.yml:4-7`,
   `flow18.yml:6-9`), and a merge step is never offered to a worker
   (`queue/next.go:190`). Each of the 15 to 20 program designs left would
   wait once more on Jeff at its merge, with nothing for him to judge
   beyond what its review and tier 2 already judged.
7. **Every program design waits on Jeff's review.** FLOW-19's skip
   criteria refuse any seam, any `alters_relationships: true`, any assumed
   question and any rule change (`process.md:893-899`), and a question that
   "touches ... a seam" is always flagged (`:875-877`). Program designs do
   all of these by nature. driver-contract (K1) alone has had two design
   reviews, two question answers and one escalation
   (`queue/driver-contract.yaml:102-215`) and is at its fourth design.
8. **Pre-release tags would misbehave.** `.goreleaser.yaml:66-69` sets no
   `prerelease`, so `v0.7.0-dev.1` would publish as the Latest release;
   `relay-image.yml:47` moves `:latest` on every `v*` tag; `release.yml:71`
   would diff a full release against the last dev tag;
   `tools/conformance/check-pins.sh:17` refuses any pin with a suffix.
9. **The program's end is stated three ways.** Decision 14 of
   `componentization.md` freezes other work "until the large majority of the
   program has landed" (`:173-174`); a4 says Jeff ends it "no earlier than
   the pilot driver's release (wave 4)" (`:222-224`); Jeff now says "until
   it's complete", which his second statement defines as all components
   working when sourced from their own repositories.

## Prior art

- **`componentization.md`** (P0) set the program, its phases and Order, the
  freeze (Decision 14, a4), the `alters_relationships` limit of 3 (a5), and
  repositories created by Jeff or on his word (a11).
- **FLOW-20** as written (`process.md:920-950`) defines a program item by
  `related`, the freeze, Moves and Overview. This item amends it; it adds no
  second rule.
- **`loop-parallel.md`** and **`alters-slot.md`** made the limits count per
  collision set, so declared areas, not larger numbers, free disjoint work.
- **`merge-confirm-attended.md`** gave the process Close its seeded state
  step, which carries this item's queue changes.
- **`loop-repository.md`** (Decisions 2-3) made `config.yaml`'s
  `repositories` the only place wf reads a trunk, and LOOP-01 made a
  profile file whose trunk differs from it a failing finding. That is why
  the program trunk is a second entry beside `trunk: main`, not a new
  value of it (a11).
- **`seam-specs.md`** (a15, Decision 12) has Jeff, or an attended session
  on his word, push specification release tags. Pre-release tags on a
  program trunk are the new case.
- **wf's merge proof** already accepts a squash merge: `merged()` greps the
  trunk for `(Merge pull request |\()#N` (`check/check.go:514-515`), and
  FLOW-16 already covers deleting a squash-merged branch.
- **GLOBAL-08** already lets seams and orchestrator internals move before
  1.0 (`global.md:147-151`), spec-first.

## Decisions

1. **The program is the north star until it completes (a1).** It completes
   when every component works as intended when built from its own
   repository, and the program trunks have landed on their trunks by
   Jeff's squash merges (Decision 12). Jeff may end it earlier.
2. **Non-program work is held, fixes included (a2).** Intake files an item
   that is neither a program item nor a process item on hold, a security or
   regression fix included, unless Jeff releases it. Never held: the fix
   for a failing `main` or a failing program trunk, and a fix that
   `follows` a program item. The urgent mark does not release a hold.
3. **A hold names when it resumes, and a held item counts toward no limit
   (a3).** A hold question gains `resumes`; wf treats an item with an open
   question that has `resumes` as held: it holds no limit, does not wait on
   the human, and `QUEUE.md` lists it once under **Held**.
4. **Parallelism comes from declared areas, not new limits (a4).** The
   numbers stay; a program item declares its repositories and areas by the
   end of its Design at the latest, and its design is scoped to one review.
5. **No new order rule.** With non-program work held, program items lead;
   intake places a program item above any released non-program item of its
   class. Process items stay class 2 (LOOP-04).
6. **Loop work meets Jeff's bar (a5).** A process item filed while FLOW-20
   stands carries a **Program payoff** section, judged at its design
   review. wf-state-migration-step stays queued, as Jeff decided on
   2026-11-21; its own review judges its payoff.
7. **Interfaces may break for the program, within fixed limits (a6 to
   a9).** Go APIs, internals and the driver contract (GLOBAL-08); a seam by
   a new pre-1.0 release of its specification first; CLI flags, subcommands,
   config keys, environment variables and output forms with no alias or
   window, only where the program needs it, a released form refused with an
   error naming its replacement (UX-04). Never broken: the ledger wire
   format and its signatures (additive only), the reader's support for
   every ledger a release wrote, a written schema `$id`, and every security
   boundary. A program trunk may lack a capability for a while when a
   design names the gap; `main` never does, and no gap is open at the
   final squash.
8. **Each repository the program works in has a program trunk (a11).**
   - Name: `program/componentization`, in `buildwarden/buildwarden` and
     `buildwarden/buildwarden-specs` now, and in each program repository
     when it is created (a19).
   - `config.yaml` records it beside the trunk:
     `program_trunk: program/componentization` in the repository's entry.
     `trunk: main` stays, and stays what the profile file's `trunk` is
     compared with (LOOP-01), so no branch in flight fails its checks.
   - It is not a trunk: FLOW-08's "only a human merges" keeps meaning
     `main`. It is still never pushed to directly, force-pushed or deleted
     by an agent; it moves only by merged pull requests.
   - A program item that is not a process item, changes no loop
     machinery path (`.claude/`, `tools/workflow/`) and changes no CI or
     release configuration (`.github/`, `.goreleaser.yaml`) works against its
     repository's program trunk: Branch setup forks from it, refreshes
     (FLOW-10) and Publish merge it, the PR goes to it, red-trunk holds and
     `wf stale` read it, and its design is archived on it. Every other item
     (process items, a program item that changes CI or release
     configuration, Jeff's released non-program items,
     config-lookup-test-symlinked-tmp) keeps `main`.
   - A queue mark `program: true` says which items are program items; wf
     checks it against the design's `related` at Design's done-when.
9. **The loop merges program pull requests into the program trunk (a12).**
   - A new cloud step, `f7-merge`, replaces the human's merge for a PR to a
     program trunk. `wf next` offers it only while `wf check -step
     f7-merge` passes the **merge gate**, on the PR's current head:
     1. the PR's base is the item's program trunk;
     2. the head is the recorded `pr.head` and the remote branch head;
     3. every check run on that head completed with conclusion success,
        skipped or neutral, none failed, cancelled, timed out or needing
        action, and every check the program trunk's ruleset requires is
        among them and succeeded;
     4. the branch contains the program trunk's head (FLOW-08's readiness);
     5. the tier-2 report on the workflow branch is for that head, and no
        MUST finding is open other than one Decision 10 lets the loop defer;
     6. no human step of the item is pending: no blocking question, no
        escalation, no design review not taken, no `host:` blocking
        validation pending (FLOW-09);
     7. `security_boundary` is `none`, or its only trip is GLOBAL-06's
        relay clause with no posture change (a15), the umbrella test or
        design review accepted it, and Review recorded that the diff stays
        within it; an item with any other `security_boundary` waits at
        `f7-merge` on Jeff, who acknowledges its security note there;
     8. every pull request its design names to merge first has merged;
     9. the rule-change clearance passes (FLOW-14).
   - The agent merges with `gh pr merge <n> --squash --match-head-commit
     <head> -R <repo>`: one squash commit per item on the program trunk,
     refused by GitHub if the head moved after the check. Never `--admin`,
     auto-merge, an approval or a bypass. `wf done` then runs the existing
     merge proof and records `work[].merged`, and `wf record` appends the
     merge to the records.
   - One loop merge per program trunk per run; the others wait for their
     refresh, as FLOW-08 already has them.
   - A process item's PR, and any change to the loop's rules or machinery,
     never goes to a program trunk except as part of `main` merged in by the
     program sync (Decision 14) (LOOP-09): it goes to `main`, and Jeff
     merges it as today, so every scheduled run takes it from `main`.
10. **Tier-2 MUST findings on a program PR (a13).** Review fixes a MUST
    finding in a fix round and re-reviews the new head. One still open
    after one fix round is **deferred**: Review records the finding, the
    head and its reason in the records, and the loop may merge. A MUST
    finding under GLOBAL-06, a SEC rule, RELAY-01 to RELAY-03 or RELAY-05,
    or the never-break list of Decision 7 is never deferred: the item waits
    at `f7-merge` on Jeff, who fixes the course or dismisses it with a
    recorded reason, as FLOW-08 has it today. Every deferred finding is
    listed in the landing PR's body, where Jeff's merge acknowledges or he
    dismisses it (LINT-03). The loop never dismisses a finding.
11. **Fewer human gates for program items (a14 to a17).** A step that
    waits on Jeff is kept only where he offers meaningful input his
    intents and the program's design do not already give.
    - **The umbrella test** takes a program feature's design review in
      place of Jeff when every point holds (Design, *The umbrella test*):
      the design implements one named phase row or a filed split or
      follow-up of one; no flagged question; no rule change and no loop
      machinery change; no boundary posture change; no
      interface break FLOW-20 does not allow; repositories and areas no
      fewer than intake recorded; and an independent reviewer session
      agrees. A pass is recorded as `umbrella test passed at <sha>` and is
      an approval for the Plan gate.
    - **Flagging for program items**: a question that touches a seam is
      flagged only when the seam's change departs from what the program's
      design names for the item's phase.
    - **Security (a15)**: a design whose only GLOBAL-06 trip is "any change
      to the relay itself", with no change to a boundary's posture, passes
      the test with its security note read by the independent session; the
      note's acknowledgment moves to the landing PR. A posture change goes
      to Jeff.
    - **Escalations (a16)**: a program item's return to Design escalates
      only after its fourth return, not its second; its first runaway
      budget raises its estimate to its spend so far, recorded, and its
      second escalates.
    - **Filing (a17)**: the loop files, unattended and recorded, the items
      of a phase row once the Order's gate before its wave is met, the
      reconciliation items that adopt a program pre-release on a program
      trunk, and additions to an area map that a reviewed or
      umbrella-passed program design names (a new repository's map, the
      areas a move creates, an area split in two).
    - **Repositories (a19)**: a new program repository of the umbrella's
      list is created private, by Jeff or an attended session, on the
      standing word of his approval of that list; making it public stays
      his.
    - **What always reaches Jeff** is listed under Design, *What still
      reaches Jeff*.
12. **The program ends with one human squash merge per repository.** A
    process item, `program-land`, filed on hold by this item's state step
    (resumes: every program trunk has its last phase merged), closes the
    program. Its done-when is Jeff's check: every component works as
    intended when built from its own repository at its program trunk, at
    the other components' `-rc` tags. Its design names, as pull requests to merge
    first, one PR per repository from `program/componentization` to its
    trunk, whose body wf renders from the records (`wf program-body`). Jeff
    squash-merges each, specifications first; full release tags follow
    from `main` as today. Its own PR retires FLOW-20 and every
    program-trunk clause, and its state step drops `program_trunk` from
    `config.yaml`. Before his squash merge, Jeff tags each program trunk's
    head `program/componentization-final` (or has `program-land`'s state
    step do so on his word), so the per-item history stays reachable.
13. **Records.** Everything this item lets the loop decide in Jeff's place
    (a loop merge, a deferred MUST finding, an umbrella test, a security
    note read under a15, a re-estimate, a fourth return, a loop-filed item,
    an area-map addition, a pre-release tag, a sync's conflict resolution)
    is appended by `wf record` to
    the workflow branch's `program/records.yaml`, named in the run's digest
    and `QUEUE.md` under **Decided for you**, and listed again in the
    landing PRs. Jeff may call any back at any time: a blocking question on
    an open item, or a follow-up program item for a merged one.
14. **The loop merges `main` into each program trunk.** When `main` moves
    past a program trunk (in practice, a process item's merge), the loop
    cuts `sync/program-<main sha>` from the program trunk, merges `main`
    with conflict-only fixes, runs the fast gate, opens a PR to the program
    trunk and merges it through checks 1 to 4 of the merge gate, and also
    check 5 on the resolution diff when the merge had conflicts, with
    `--merge` (a merge commit), so `main` stays an ancestor of the program
    trunk and the landing PRs stay conflict-free. A conflict it cannot
    resolve escalates.
15. **Pre-release tags (a18).** On a program trunk, after a loop merge that
    changes a consumable (the profile's Consumables entry), the loop may tag
    `vX.Y.Z-dev.N` (`<directory>/vX.Y.Z-dev.N` in the specifications
    repository) on that merge commit once its checks pass, where X.Y.Z is
    the next minor after the latest release without a suffix (`v0.1.0`
    when the repository has none); and `-rc.N` once the component's last
    phase has merged on its program trunk and its CI and conformance suites
    pass at the latest `-dev` (or `-rc`) tags of what it consumes. Each
    publishes as a
    GitHub pre-release that moves no `latest`, and is never moved or
    deleted. On a program trunk a pre-release is a release for LOOP-07, and
    pins may name one; `main` pins only releases without a suffix. A
    release without a suffix stays Jeff's, from `main`, through the full
    gate.
16. **What only Jeff can do, once.** Review and merge this item's PR; run
    its state step (with the program trunks' creation on his word in it);
    merge `specs-prerelease-tags` in the specifications repository;
    set each program trunk's ruleset and the tag ruleset; confirm the
    GitHub App may merge; change the nightly task's prompt, which today
    says "Never merge". After that, the loop needs nothing from him to
    advance a program trunk except what Design, *What still reaches Jeff*,
    lists, ending with the landing merges.
17. **Enforcement stays small where judgment is the check.** The hold
    counting, the trunk choice, the merge gate, escalation carve-outs and
    the records are wf code. Program membership, the payoff, each
    interface break and the umbrella test's points U1 to U5 stay agent
    judgment at intake, Design and the independent session (U7).

## Questions

### Blocking

None.

### Assumed

- **a1. When the program ends.** Flagged: departs from componentization a4
  and Decision 14, and sets FLOW-20's closing paragraph.
  - (a) When every component works as intended built from its own
    repository at its program trunk, and Jeff has squash-merged each
    program trunk into its trunk (`program-land`); earlier by his word.
  - (b) When its last phase has merged on the program trunks (wave 6, K5),
    with the landing merges after the program.
  - (c) No earlier than the pilot driver's release (wave 4), as a4 said.

  Recommended: (a). It is his second statement in its own terms, and it
  puts the landing inside the program, so FLOW-20 covers the program
  trunks until they are gone.
- **a2. Security and regression fixes during the program.** Flagged:
  removes FLOW-20's two exemptions, which componentization a4 confirmed,
  and sets the Hold bullet and FLOW-05's urgent sentence.
  - (a) Held like other non-program work unless Jeff releases one. The fix
    for a failing `main` or program trunk and a fix that `follows` a
    program item are never held. Marking a fix urgent does not release its
    hold.
  - (b) Keep both exemptions; Jeff may hold a fix whose code the program
    replaces.
  - (c) Keep FLOW-20 as written; the 2026-11-21 holds stay one-off.

  Recommended: (a). It is what Jeff did on 2026-11-21, and the red-trunk
  exception keeps refreshes running (FLOW-10). A non-program follow-up an
  unattended session files is held with the followed item's resume point.
- **a3. Held items and the limits.** Flagged: sets FLOW-05's counting
  sentences.
  - (a) A hold names its resume point (`resumes`); a held item holds no
    limit, does not wait on the human, and is listed once under **Held**.
  - (b) No code: Jeff raises `in_progress` and `waiting_on_human` by hand.
  - (c) Close the held items for re-intake, as componentization a8 did.

  Recommended: (a). About 60 lines of Go, and it removes the daily
  restating of decided holds.
- **a4. The limits for program items.** Flagged: sets FLOW-20's Areas
  bullet.
  - (a) Numbers unchanged (5, 3, PR slot 1, 8); a program item declares its
    areas by the end of its Design, scoped to one review.
  - (b) The in-progress and `alters_relationships` limits do not count
    program items.
  - (c) Raise the numbers (for example 8 and 5).

  Recommended: (a). The limits count per collision set, so declared
  disjoint items already run side by side; 48 of `QUEUE.md`'s 52
  non-disjoint pairs exist only because one side is undeclared.
- **a5. Jeff's bar for loop work.** Flagged: sets FLOW-20's Loop work
  bullet, in his words.
  - (a) Every process item filed while FLOW-20 stands carries a **Program
    payoff** section, judged at its design review; one that neither removes
    interactions that decide nothing nor saves more program time than it
    costs is held or closed.
  - (b) Judged at intake only.
  - (c) As (a), and `wf check` requires the section.

  Recommended: (a). The review is already Jeff's; (c) is code for a check
  the review makes anyway.
- **a6. CLI flags, subcommands, config keys, environment variables and
  output forms.** Flagged: departs from UX-03 and ORCH-03 for program items.
  - (a) A program item may rename or remove one with no alias or window,
    only where the program's change needs it; a form a tagged release
    exposed is refused with an error naming its replacement (UX-04); one no
    release exposed may simply go; the reference docs change with it.
  - (b) Only forms no release exposed may break.
  - (c) UX-03 as written.

  Recommended: (a). "Even if it is temporarily incompatible or breaking",
  with "mostly similar" kept by the need test. A removed config key is
  silently ignored today (`cmd/warden/config.go`), so the item that removes
  a released key adds that error.
- **a7. The ledger.** Flagged: sets which ledger rules FLOW-20 keeps.
  - (a) `specs/build-ledger/v1.md` and its signatures change only
    additively (RELAY-05); the reader reads every ledger a release wrote; a
    written schema `$id` never changes.
  - (b) A v2 allowed per item on Jeff's approval.
  - (c) Frozen until the program ends.

  Recommended: (a). It is his "mostly similar specs" made exact.
- **a8. Temporary gaps.** Flagged: sets the Interfaces bullet; touches
  GLOBAL-06.
  - (a) A program trunk may lack a capability for a while when the design
    that removes it names the gap and the item that restores it; the
    program trunk still passes its checks, no such state opens a boundary,
    `main` never has a gap, and `program-land` requires none open.
  - (b) Every merge keeps every driver and command fully working.
  - (c) Freely.

  Recommended: (a). The program trunk is where the breaks belong; `main`
  stays whole for anyone who builds it until the landing.
- **a9. Tier 1's UX-03 check on a deliberate break.**
  - (a) A `Tier1-Allow` trailer whose reason names FLOW-20 (LINT-05), as
    Moves does.
  - (b) Tier 1 compares surfaces against the latest release tag.

  Recommended: (a). It costs an agent a trailer, not Jeff an interaction.
- **a10. One item or two.**
  - (a) One item: holds, interfaces, program trunk, fewer gates and
    pre-releases, reviewed in one sitting.
  - (b) Split: this draft's holds and interfaces, then a second process
    item for the program trunk, gates and tags.

  Recommended: (a). Jeff asked for the initial gate to be what the
  scheduled runs need and nothing after it; two items mean two merges and
  two state steps, and the program trunk is what makes the interface
  breaks safe to merge. The sitting takes the 15 rule changes in
  question-call groups (FLOW-19).
- **a11. The program trunk's shape.** Flagged: sets LOOP-01, PROFILE-01's
  Trunk row and FLOW-20's Program trunk bullet.
  - (a) `program/componentization` in each repository the program works in,
    recorded as `program_trunk` beside `trunk: main` in `config.yaml`;
    chosen per item (program, not process, no machinery, CI or release
    configuration path). Created at
    `main`'s head in the state step, on Jeff's word in it.
  - (b) Switch the entry's `trunk` to the program branch for everything.
  - (c) No program trunk; program items merge to `main` as today.

  Recommended: (a). (b) would fail every branch in flight on LOOP-01's
  profile check, move config-lookup-test-symlinked-tmp and the loop's own
  process items off `main` (which the scheduled runs read, LOOP-09), and
  leave FLOW-08 forbidding the loop's merges. (c) is what he asked to stop.
- **a12. Who merges into a program trunk, and how.** Flagged: amends
  FLOW-08, FLOW-03, FLOW-04 and LINT-03.
  - (a) The loop, at `f7-merge`, under the nine-point merge gate; squash per
    item pinned to the head; `main` merged in by the loop with a merge
    commit (Decision 14).
  - (b) As (a), with merge commits for every item.
  - (c) Jeff merges each PR into the program trunk.

  Recommended: (a). His "no publish step should wait on me". Squash keeps
  one commit per item, which makes the landing PR readable, and wf's merge
  proof already accepts it; the merge commit for `main` keeps `main` an
  ancestor so the landing PR has no conflicts.
- **a13. Tier-2 MUST findings on a program PR.** Flagged: amends LINT-03's
  sign-off; touches GLOBAL-06.
  - (a) The loop fixes a MUST in one fix round; one still open is deferred
    and recorded, and signed off at the landing merge, except one under
    GLOBAL-06, a SEC rule, RELAY-01 to RELAY-03, RELAY-05 or FLOW-20's
    never-break list, which waits on Jeff. The loop never dismisses.
  - (b) Every open MUST waits on Jeff at `f7-merge`.
  - (c) Every MUST is deferred, security ones included.

  Recommended: (a). A MUST the loop cannot fix in one round is usually a
  judgment about the program's direction, which the landing PR still puts
  before him; the ones that could open a boundary or break what never
  breaks stop at once. (b) is the safe side if he prefers it, at a merge
  wait per such item.
- **a14. The umbrella test for program design reviews.** Flagged: amends
  FLOW-19 (a third review outcome and the flag rule for seams).
  - (a) A program feature's review is taken by the test U1 to U7 (Design)
    and recorded for his later reading; a flagged question, a rule change,
    a loop machinery change, a boundary posture change or a failed point
    sends it to him. Process items keep his review (LOOP-09).
  - (b) As (a), but a seam-touching question always flags, so most program
    reviews still reach him.
  - (c) No: every non-trivial program design keeps his review.

  Recommended: (a). The program's phases, seams and order are his already
  approved design; a design that implements a phase row within it, and
  that an independent session agrees does, is what he called "easily
  understood by my intents and design thus far".
- **a15. GLOBAL-06's relay clause under the umbrella test.** Flagged:
  touches a security boundary.
  - (a) A design whose only trip is "any change to the relay itself", with
    no boundary posture change, passes U4 with its security note read by
    the independent session; Jeff acknowledges the note at the landing
    merge. A posture change (a new listener or egress path, key, CA or
    trust-store handling, the SSRF filter, netns or iptables, the socketpair
    topology) goes to him. TEST-08 runs at each `-rc` and the landing.
  - (b) Every program design with `security_boundary` other than `none` is
    reviewed by Jeff.

  Recommended: (a). Every M-track item and S2, S4 trip the relay clause;
  (b) puts the whole relay track back in his queue, design by design.
- **a16. Escalations on program items.** Flagged: amends FLOW-06 and
  FLOW-13.
  - (a) A return to Design escalates after the fourth, not the second;
    the first runaway budget raises the estimate to the spend so far; both
    recorded; the second runaway escalates.
  - (b) FLOW-06 and FLOW-13 as written.

  Recommended: (a). A re-design still meets the umbrella test or reaches
  him, so the extra returns cost steps, not judgment; the second runaway
  still stops a loop that is not converging.
- **a17. Filing without Jeff.** Flagged: amends LOOP-07.2 and FLOW-05's area
  map sentence.
  - (a) The loop files a phase row's items once the Order's gate before its
    wave is met, reconciliation items that adopt a pre-release on a program
    trunk, and additive area-map changes a reviewed or umbrella-passed
    program design names; each recorded. Changing an area's mode or
    `reaches`, removing one, and relating or narrowing items stay his.
  - (b) Jeff at intake and in attended sessions, as today.

  Recommended: (a). The phase rows and their order are already his; a
  pre-release adoption is mechanical; a new repository's map is what lets
  its items run side by side at all.
- **a18. Pre-release tags and pins.** Flagged: amends FLOW-17, LOOP-07.5
  and PROFILE-01's Specifications, Release and Consumables rows.
  - (a) The loop tags `-dev.N` after a loop merge that changes a
    consumable, and `-rc.N` once the component's last phase has merged and
    its suites pass at the latest pre-releases of what it consumes, on
    program trunks only, published as pre-releases; pins on a
    program trunk and PRs to it may name a pre-release; `main` pins only
    releases without a suffix.
  - (b) `-dev.N` only; `-rc.N` and full releases are Jeff's.
  - (c) No tags: program trunks pin commits (LOOP-07.5 already allows).

  Recommended: (a). His "staging/pre-production version tags": `-dev` is
  staging, `-rc` is pre-production. Semver orders them below the release.
- **a19. Program repositories.** Flagged: departs from componentization
  a11 (Jeff's word in each session).
  - (a) His approval of this design is the standing word for the
    umbrella's list (the relay, the loop, one repository per driver):
    each is created private when its phase starts, by him or an attended
    session, with its `config.yaml` entry, its program trunk and its
    `hosts.yaml` line for a machine that already serves its host. Going
    public stays his, one action per repository.
  - (b) Each repository on his word when its phase starts, as a11 says.

  Recommended: (a). The list is in the umbrella he approved; asking again
  per repository decides nothing new.

## Design

### The rule

FLOW-20 is rewritten (full wording under **Rule changes**): **Hold**
replaces **Freeze**; **Areas**, **Loop work** and **Interfaces** as in the
earlier draft (Interfaces' gap sentence now names the program trunk);
three new bullets, **Program trunk**, **Fewer human gates** and
**Pre-releases**; **Moves** and **Overview** unchanged. The closing list of
what stays as written now reads FLOW-08 and FLOW-19 "as this rule reads
them". The other rules gain one clause each that points to FLOW-20, so an
agent reading one alone does not act against it.

### The program trunk

**Configuration.** `queue.RepositoryEntry` (`queue/queue.go:68-75`) gains
`ProgramTrunk string \`yaml:"program_trunk,omitempty"\``; `Validate` refuses
one equal to `trunk`. `queue.Feature` gains `Program bool
\`yaml:"program,omitempty"\``. A new `Config.TrunkFor(repo, f)` returns the
program trunk when the entry has one, `f.Program` is true, the item is not
a process item, and its declared areas reach no machinery, `.github/` or
`.goreleaser.yaml` path; otherwise
`Config.Trunk(repo)`. `Config.Trunk` stays the only reader of `trunk`, and
LOOP-01's profile comparison (`check/profile.go:104-106`) is unchanged.

**Who reads it.**

- `check/repos.go:117` (`trunk()`) and every check that reads the trunk
  through it (`mainMerged`, `merged`, `archivedOn`, the rule-change base)
  take `TrunkFor`.
- `wf stale` and `wf areas -check` compare each item with its own trunk;
  `wf areas -rev` and `wf drops -base` default to the program trunk while a
  program item is in flight.
- `wf main -trunk <t>` writes the trunk's record: `main.yaml` for the trunk,
  `program.yaml` beside it for the program trunk, same fields. The red-trunk
  hold (FLOW-10) is per trunk.
- `wf trunk -feature F [-repository R]` prints the item's trunk for agents.
- `check.Design`'s declared check (`declaredStill`) fails when the queue's
  `program` disagrees with the design's `related`.

**Items in flight.** driver-contract and relay-release-forms-spec get
`program: true` in the state step. driver-contract has no PR; its next
merge of the trunk merges `origin/program/componentization`, which starts
equal to `main`, so the merge is empty. config-lookup-test-symlinked-tmp
finishes on `main` unchanged.

**CI.** `main.yml:4-7` and `flow18.yml:6-9` add `'program/**'` to
`branches` for push and pull_request. The tier-1 job already diffs against
`github.event.pull_request.base.sha` (`main.yml:98`), so it works for any
base.

### The merge gate and the loop merge

- **`check.MergeReady(env, f)`**, run as `wf check -feature F -step
  f7-merge`, checks Decision 9's points in order: `gh pr view --json
  baseRefName,headRefOid` for 1 and 2; `gh api
  repos/<o>/<r>/commits/<sha>/check-runs` and the trunk's
  `rules/branches/<t>` for 3; `mainMerged` against the program trunk for 4;
  the review report for 5 (`check/review.go`, reading deferred findings
  from the records); the item's steps and questions for 6; the security
  record for 7; `mergedFirst` for 8; `ruleClearance` for 9. Each failure
  names its point.
- **`queue/next.go:32-43, 190-193`**: a merge step whose item's
  `TrunkFor` is a program trunk is `f7-merge`, `cloud`, offered once
  `MergeReady` passes; every other merge step is still never offered.
  `MayComplete` is unchanged. **`queue/limits.go:379-393`**: a PR at
  `f7-merge` does not wait on the human; one held at point 5 on a
  protected finding, or at point 7 on a posture change, does.
- **`check/check.go:148, 502`**: the label becomes "merged (FLOW-08)"; the
  proof (`:514-515`) is unchanged and already accepts a squash.
- **The agent** (`wf-publish`, step `f7-merge`): run the check; on a pass,
  `gh pr merge <n> --squash --match-head-commit <pr.head> -R <repo>`; then
  `dist/wf done` and `dist/wf record -kind merge`. On a refusal from GitHub,
  run the check again and stop.
- **Program sync** (Decision 14): `wf sync-program -repository R` prints
  whether `main` is ahead of the program trunk and the branch name to use;
  the tick dispatches `wf-publish` with step `sync`, which merges with
  `--merge --match-head-commit` after `MergeReady`'s points 1 to 4, and
  point 5 on a tier-2 review of the resolution diff when the merge had
  conflicts; the resolution is recorded.

### The umbrella test

Design writes `design_review: umbrella` for a program feature it judges to
pass, and records the points. wf seeds `f2-umbrella`, a cloud step that
`wf-review` takes in a fresh session: it reads the design against
`componentization.md` and the records, and either writes the line
`r<N>, <date>: umbrella test passed at <sha> (FLOW-20)` with each point's
evidence, or writes the failing point and sets `design_review: required`,
which seeds Jeff's review as today.

- **U1.** Program item: `related` lists `componentization.md`, and the
  design implements one named phase row (P, S, K or M), a filed split of
  one, or a follow-up of a program item, within that row's Work and Done
  when.
- **U2.** No blocking question, and no flagged assumed question (FLOW-19,
  with the program's seam clause). Each recommendation cites the umbrella
  decision or recorded answer of Jeff's it follows.
- **U3.** `rule_changes` is empty (a rule change needs Jeff's clearance,
  FLOW-14), and the design's `areas` reach no rule or machinery path of the
  profile (LOOP-09).
- **U4.** `security_boundary` is `none`, or the only trip is GLOBAL-06's
  relay clause and the Security note states that no boundary's posture
  changes (a15).
- **U5.** Each interface break is one FLOW-20's Interfaces bullet allows.
- **U6.** `repos` and `areas` declared, no fewer than intake recorded.
- **U7.** The independent session agrees.

`check/review.go`'s `approvals` accepts `umbrella test passed at <sha>` as
an approval, so the Plan gate's delta check works as for Jeff's: a later
change to an impact field, `repos`, `areas` or `rule_changes` needs a new
pass. Each pass is recorded and listed in the digest, as a skipped review is
today.

Design also settles a question from the umbrella or a recorded answer
before it raises one, citing the source, and records "settled by <ref>, not
asked".

### What still reaches Jeff

- The design review of a process item, and of a program design that fails
  the umbrella test.
- A blocking question the umbrella and his recorded answers do not settle.
- A MUST finding Decision 10 does not let the loop defer.
- The security note of a program PR that changes a boundary's posture, at
  `f7-merge`.
- CI repair exhausted (FLOW-11), a branch that did not delete (FLOW-16), a
  second runaway budget, a fifth return to Design, a sync conflict the loop
  cannot resolve.
- A process item's merge to `main` and its state step, and the merge of
  a program item that changes CI or release configuration (Decision 8).
- Releasing a held item, an urgent mark, the priority order, `hosts.yaml`
  beyond a19, an area map change other than an addition.
- Making a repository public; a release without a suffix.
- The landing merges (Decision 12), with the records in their bodies.

### Pre-release tags

- `wf tag-next -repository R` prints the next tag for the program trunk's
  head (`vX.Y.Z-dev.N`, or the specification's `<directory>/vX.Y.Z-dev.N`
  per changed directory, the harness first), or nothing when the last loop
  merge changed no consumable or the head's checks are not green. The tick
  pushes it with `git push origin <tag>` and records it.
- `.goreleaser.yaml:66-69` gains `prerelease: auto` and `make_latest:
  '{{ if .Prerelease }}false{{ else }}true{{ end }}'`.
- `relay-image.yml:47-52` adds `:latest` only for a tag with no `-`.
- `release.yml:71`'s `git describe` gains `--exclude '*-*'` for a release
  without a suffix, so a full release diffs against the last full one.
- `tools/conformance/check-pins.sh:17` keeps its pattern for a base of
  `main` and accepts `-(dev|rc)\.N` for the spec and harness when
  `GITHUB_BASE_REF` is a `program/` branch; a `replace` line stays refused.
- `tools/conformance/pin-specs.sh` (`:32, 119, 131, 147`) accepts a
  pre-release named explicitly, and `SPECS=newest` (the highest version,
  pre-releases included), refused when the current branch is `main`.
- The specifications repository's release workflow (`speccheck tag -main`,
  `gh release create` without `--prerelease`) is changed by an item in that
  repository, `specs-prerelease-tags`, which the state step files against
  its `main`; Jeff merges it as part of the one-time setup (Decision 16),
  and `wf tag-next` prints nothing for that repository until it has merged.
- The landing order: specifications squash and full tags; this repository's
  pins move to them on the program trunk, then its landing and full tag;
  then the relay; then the drivers, at the kit's full tag.

### Skills and agents

- **`workflow-tick`**:
  - Never (`SKILL.md:20-21`): "Approve or enable auto-merge on any PR. Merge
    one only at `f7-merge` or the program sync, into a program trunk, with
    `--match-head-commit`, after `wf check` passes (FLOW-08, LINT-03);
    never into a trunk, never with `--admin`, and never dismiss a finding.
    Push `main` or a program trunk, force-push, rewrite pushed history, or
    delete a remote branch or a tag."
  - Step 2 (`:101-131`): record each trunk and each program trunk; refresh
    against each item's own trunk; run the program sync when `main` is
    ahead.
  - After dispatch: `wf tag-next` and its push; `wf record` for each
    decision the run made.
  - Digest: **Held** (one line), **Decided for you** (the run's records),
    and Blocked on Jeff without held items or loop-merged PRs.
- **`workflow-human`**: step 2 shows held items apart; step 3 checks a
  process item's Program payoff, a program design's interface breaks and
  areas, and lists the umbrella passes since the last sitting for reading;
  item 5 (`SKILL.md:145-150`) does not offer a PR to a program trunk ("the
  loop merges it"), lists those held at `f7-merge` on a protected finding,
  and the records since the last sitting; a new last item walks the
  landing PRs once `program-land` resumes.
- **`workflow-intake`**: the hold wording of the earlier draft (`-hold
  "<why>" -resumes "<when>"`), `-program` for a program item, and an
  unattended path for a17's filings, recorded.
- **`workflow-native`** (`SKILL.md:3, 36, 130, 206`): tests each trunk and
  program trunk, then the waiting branches; never pushes `main` or a
  program trunk, and merges nothing; `settings.native.json:89`'s deny of
  `gh pr merge` stays.
- **`wf-publish`** (`.claude/agents/wf-publish.md:16, 19, 41, 66, 91`):
  every `origin/main` and `main` becomes "the item's trunk (`dist/wf
  trunk`)"; new steps `f7-merge` and `sync`; the Never line as in
  workflow-tick.
- **`wf-review`**: the `f2-umbrella` step; each MUST and SHOULD finding
  recorded with the head it was found on; a deferral with its reason.
- **`wf-design`**: the umbrella self-check and `design_review: umbrella`;
  settle a question from the umbrella before raising it.

## Defaults and overrides

- **A hold's resume point.** `program-end` unless Jeff names another.
  `-resumes` is required with `-hold`.
- **A non-program item during the program.** Held; only Jeff's word
  releases it.
- **An item's trunk.** `main`; the program trunk only for a program item
  that is not a process item and touches no machinery, `.github/` or
  `.goreleaser.yaml` path, in a repository
  whose entry has `program_trunk`. No fallback: an entry without
  `program_trunk` gives `main`, and wf names it in `wf trunk`'s output.
- **The merge method.** Squash per item, merge commit for the program sync.
  No other method is used, and no loop merge runs without the gate.
- **A MUST finding the loop could not fix.** Deferred and recorded, except
  the protected ones, which wait on Jeff.
- **Design review of a program feature.** The umbrella test; on any failed
  point, Jeff.
- **Pre-release version.** The next minor after the latest release without
  a suffix, `v0.1.0` when the repository has none; `-dev.N` counting from 1
  per minor; `-rc.N` likewise.
- **Interface breaks.** None by default; each named in its design. A
  removed released form errors; trying the new interface and then the old
  one is a fallback under GLOBAL-10 and GLOBAL-11 and is named or not made.
- **The limit numbers** stay in `config.yaml`, Jeff's to change.

## Cross-driver impact

None. No driver code is reached by this item. Program items name the
drivers they touch in their own designs.

## Program payoff

This item is a process item filed while FLOW-20 stands, so it meets its own
bar (Decision 6).

- **Interactions removed.**
  - Holds: 3 decided holds restated in `QUEUE.md` and the digest each run,
    21 a week, go to 0.
  - Merges: each program PR's merge, 15 to 20 left, go to 0, plus each
    sync of `main` (would have been a merge each).
  - Reviews: program feature reviews that pass the umbrella test go to 0;
    K1 alone has had 5 review, answer and escalation rounds. At half of 15
    to 20 designs passing, 8 to 10 sittings.
  - Alias and window questions: a few per program design.
  - Specification tags: each `-dev` tag (today his push, seam-specs a15).
- **Time saved.** Three in-progress places and an `alters_relationships`
  slot are freed now; a program PR merges in the run that makes it ready,
  not at his next sitting; pins can follow a pre-release the same day.
- **Cost.** 10 steps (9 + 1 state step) over two runs, one review sitting,
  and his one-time actions (Decision 16).

## What this item does not do

- Change FLOW-09: a program item still waits for its blocking native
  validation. Tracked gaps are already the design's choice.
- Let the loop answer a LOOP-04 release wait by itself when the tag
  appears; a follow-up process item may, under the bar.
- Count the PR slot per trunk: with non-program work held, `main` has no
  feature PRs to share it with.
- Create any repository, or change `main`'s protection.

## Rule changes

Each block quotes `main` at `be13847`. Severity and check are unchanged
unless said.

### FLOW-20 (amend), `docs/development/process.md`

MUST; check changes from `agent-judgment` to `automated + agent-judgment`.
**Why:** Decisions 1-17.

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
> **Check:** automated + agent-judgment
>
> The componentization program is the work set out in
> [`componentization`](../design/componentization.md): the relay, the
> drivers, the kit, the specs and the loop become separate components coupled
> through written seams. A *program item* is a queue item whose design lists
> that design in `related`. The program is the north star until it completes
> (the human, 2026-11-21): work is prioritized to finish it, with as many of
> its items in flight at once as the relations allow, even where an item
> breaks an interface for a while, and as little of it as possible waits on
> the human where the program's design already gives the answer. While this
> rule stands:
>
> - **Hold.** An item that is neither a program item nor a process item is
>   held unless the human releases it; a fix for a security problem or for a
>   regression is held like any other. Intake files it on hold, with a
>   blocking question only the human answers that names when it resumes: the
>   program's end, unless the human names another point. An item already
>   queued or in progress that is neither is held the same way unless the
>   human has released it, and keeps its branch and design; on resume it
>   returns to Design for what the program changed (FLOW-06). Never held:
>   the fix for a failing `main` or program trunk (FLOW-09, FLOW-10), and a
>   fix that `follows` a program item. A held item holds no limit and does
>   not wait on the human (FLOW-05). The human releases a held item by
>   answering its question, or files one unheld at intake; marking a fix
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
>   steps and review. Its design review declines one that does neither, and
>   the item is then held or closed.
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
>     not a program break. A form a tagged release exposed is refused with
>     an error that names its replacement (UX-04), never silently ignored;
>     one no release exposed may simply go. The reference docs change with
>     it (UX-01), with a migration note for a released form. Where tier 1
>     flags the removal, the commit carries a `Tier1-Allow` trailer whose
>     reason names this rule (LINT-05).
>
>   These never break: the ledger wire format (`specs/build-ledger/v1.md`)
>   and its signatures, which change only additively; the reader's support
>   for every ledger a release wrote; a schema `$id` already written
>   (RELAY-05); and any security boundary (GLOBAL-06). A program trunk may
>   lack a capability for a while when the design that removes it names the
>   gap and the item that restores it; the program trunk still passes its
>   own checks, no such state opens a boundary, and no gap is open when it
>   lands. Compatibility kept by trying the new interface and then the old
>   one is a fallback (GLOBAL-10, GLOBAL-11).
> - **Program trunk.** Each repository the program works in has a *program
>   trunk*, `program/componentization`, which the loop's configuration
>   records beside its trunk (LOOP-01). It is not a trunk, and it moves only
>   by merged pull requests. A program item that is not a process item and
>   changes none of the loop's machinery or the repository's CI and release
>   configuration works against it wherever these
>   rules name the trunk an item forks from, merges, is refreshed against,
>   opens its pull request to, or is archived on. The loop merges such an
>   item's pull request into it when FLOW-08's merge gate passes, as one
>   squash commit, and merges the trunk into it, as a merge commit, whenever
>   the trunk moves past it. A process item, and any change to the loop's
>   rules or machinery, goes to the trunk by the human's merge (LOOP-09).
> - **Fewer human gates.** A program feature's design review is taken by the
>   *umbrella test* when every point holds: the design implements one phase
>   of the program's design, or a filed split or follow-up of one, within
>   that phase's work; no blocking or flagged question is open; it declares
>   no rule change (FLOW-14), and changes no loop machinery and no check that
>   enforces a loop rule (LOOP-09);
>   its `security_boundary` is `none`, or its only trip is GLOBAL-06's
>   relay clause and its security note shows no boundary's posture changes;
>   each interface break is one this rule allows; it declares no fewer
>   repositories or areas than intake recorded; and a review session that
>   did not write the design agrees. Any other program design is reviewed by
>   the human (FLOW-19). A program item's return to Design escalates after
>   its fourth return, not its second (FLOW-06); its first runaway budget
>   raises its estimate to its spend so far, and its second escalates
>   (FLOW-13). The loop files a phase's items once the gate before its wave
>   is met, a reconciliation item that adopts a pre-release on a program
>   trunk (LOOP-07), and an area a program item's design, reviewed or passed
>   by the umbrella test, adds to a map (FLOW-05). New program repositories,
>   of the program's list, are created private, by the human or an attended
>   session, on the standing word of the human's approval of that list;
>   making one public stays the human's.
> - **Records.** Each decision this rule lets the loop take in the human's
>   place (a loop merge, a deferred tier-2 MUST finding, an umbrella test, a
>   security note read under it, a re-estimate, a return past the second, a
>   filed item, an area added, a pre-release tag, a program sync's conflict
>   resolution) is recorded in the
>   workflow branch's program records, named in the run's digest, and listed
>   again in the pull request that lands the program trunk. The human may
>   call any back: a blocking question on an open item, or a follow-up
>   program item for a merged one.
> - **Pre-releases.** On a program trunk, the loop may tag a pre-release,
>   `vX.Y.Z-dev.N` (`<directory>/vX.Y.Z-dev.N` for a specification) after a
>   merge that changes a consumable, and `-rc.N` once the component's last
>   phase has merged on its program trunk and its CI and conformance suites
>   pass at the latest pre-releases of what it consumes, on a commit whose
>   checks passed. Each is
>   published as a pre-release that moves no `latest`, and is never moved
>   or deleted. On a program trunk a pre-release is a release for LOOP-07; a
>   trunk names only releases without a suffix. A release without a suffix
>   is the human's, cut from the trunk through the full release gate
>   (LINT-03).
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
> RELAY-05, RELAY-08), the SEC rules, UX-01, UX-04, FLOW-14, FLOW-18 and the
> LOOP rules apply to program items as written, and FLOW-08 and FLOW-19 as
> this rule reads them.
>
> The program ends when every component works as intended when built from
> its own repository, and the human has squash-merged each program trunk
> into its trunk, or earlier by the human's word. A process item that the
> loop holds until the program's last phase has merged lands it: its design
> names one pull request per repository from the program trunk to the
> trunk, whose body lists the program records, and its own change retires
> this rule. Its Close lists every item held until the program's end, and
> the human answers each hold, resuming the item or closing it.
>
> The automated part: wf chooses an item's trunk, checks the queue's program
> mark against the design's `related`, takes an umbrella-test line as an
> approval, and counts a held item toward no limit. The agent part: intake
> and Design judge whether an item is a program item; the design review or
> the umbrella test's session judges a Program payoff, each interface break
> and each point of the test.

**Enforcement:** `Config.TrunkFor`, `Feature.Program`, the declared check,
`approvals` (`check/review.go`), and the escalation carve-outs
(`queue/phases.go:599-605`, `queue/limits.go`, `Runaway`).

### FLOW-03 (amend), `docs/development/process.md`

MUST, `automated`. **Why:** phase 7 names the human's merge (a12).

**Current wording** (`process.md:158`):

> | 7 | Merge | The human merged the PR (FLOW-08). The loop confirms it, in an unattended run or in an attended session (FLOW-04); there is no step for the human to claim. The PR is ready for the merge only while its branch contains `main` (FLOW-08); when `main` moves past it, a refresh step brings it up to date (FLOW-10). |

**Proposed:**

> | 7 | Merge | The human merged the PR (FLOW-08), or, for a PR to a program trunk, the loop merged it once FLOW-08's merge gate passed (FLOW-20). The loop confirms it, in an unattended run or in an attended session (FLOW-04); there is no step for the human to claim. The PR is ready for the merge only while its branch contains its trunk, `main`, or, for a PR to a program trunk, that program trunk (FLOW-08); when that moves past it, a refresh step brings it up to date (FLOW-10). |

**Enforcement:** the merge proof (`check/check.go:501-541`) with
`TrunkFor`.

### FLOW-04 (amend), `docs/development/process.md`

MUST, `automated`. **Why:** the loop's merge is a step it takes (a12).

**Current wording** (`process.md:248-250`):

> A merge step, and a step confirming that a pull request its design names
> to merge first has merged (FLOW-08), stays `cloud` and is never offered
> to a worker (FLOW-05).

**Proposed:**

> A merge step, and a step confirming that a pull request its design names
> to merge first has merged (FLOW-08), stays `cloud` and is never offered
> to a worker (FLOW-05), except the merge of a pull request into a program
> trunk, which is offered to a `cloud` worker while FLOW-08's merge gate
> passes (FLOW-20).

**Enforcement:** `queue/next.go:32-43, 190-193`.

### FLOW-05 (amend), `docs/development/process.md`

SHOULD, `automated`. **Why:** held items hold no limit (a3); a PR the loop
merges does not wait on the human (a12); the loop adds areas a program
design names (a17).

**Current wording** (`process.md:337-339`):

> the queue state, where the Areas entry of the repository's profile says
> (LOOP-01), and it changes only in an attended session, on the human's
> word: no agent changes it otherwise.

**Proposed:**

> the queue state, where the Areas entry of the repository's profile says
> (LOOP-01), and it changes only in an attended session, on the human's
> word: no agent changes it otherwise, except that the loop adds the areas
> a program item's design, reviewed or passed by FLOW-20's umbrella test,
> names (FLOW-20).

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

**Current wording** (`process.md:400-408`):

> question, a pull request waiting for
> the merge, or a spent budget (FLOW-13); it counts once, and a tracked
> item does not count. This limit counts by no relation and stops only a
> start: work in progress goes on, so the count can pass it. A fix that
> `follows` an item, the fix for a failing `main` (FLOW-09), a security
> fix the human has marked urgent, a process
> item and a tracked item start whatever
> the count. Only the human marks a security fix urgent; the mark lifts
> this limit for it, and nothing else.

**Proposed:**

> question, a pull request waiting for
> the human's merge, or held at a program trunk's merge on a finding or a
> security note only the human may clear (FLOW-08), or a spent budget
> (FLOW-13); it counts once, and
> neither a tracked item nor an item on hold (`resumes`) counts. This limit
> counts by no relation and stops only a
> start: work in progress goes on, so the count can pass it. A fix that
> `follows` an item, the fix for a failing `main` or program trunk
> (FLOW-09), a security fix the human has marked urgent, a process
> item and a tracked item start whatever
> the count. Only the human marks a security fix urgent; the mark lifts
> this limit for it, and nothing else: it does not release a hold
> (FLOW-20).

**Enforcement:** `Question.Resumes`, `Feature.Held` (`queue/queue.go`),
`counted` and `WaitsOnHuman` (`queue/limits.go`), the **Held** subsection
(`queue/summary.go`), `-resumes` on `wf new -hold`.

### FLOW-06 (amend), `docs/development/process.md`

MUST, `agent-judgment`. **Why:** a16.

**Current wording** (`process.md:430-431`):

> delta (FLOW-19). A feature that returns to Design more than twice escalates to a
> `human` step instead of looping again.

**Proposed:**

> delta (FLOW-19). A feature that returns to Design more than twice escalates to a
> `human` step instead of looping again; a program item, more than four times
> (FLOW-20).

**Enforcement:** `queue/phases.go:599-605`.

### FLOW-08 (amend), `docs/development/process.md`

MUST, `automated`. **Why:** a12, a13.

**Current wording** (`process.md:463`, `:468-473`):

> ### FLOW-08 — Only a human merges to trunk
>
> This is LINT-03 applied to the process. No agent approves or merges a PR
> to a trunk, in any repository, and auto-merge is not enabled by agents. The human's merge is the sign-off: it
> covers the PR, the archived design it carries, and the tier-2 behavioral report,
> whose MUST findings the PR body lists. The human acknowledges them by merging, or
> dismisses one with a recorded reason in a PR comment. Changes that declare a
> `security_boundary` additionally need the security note acknowledged (GLOBAL-06).

**Proposed:**

> ### FLOW-08 — Only a human merges to trunk; the loop merges to a program trunk
>
> This is LINT-03 applied to the process. No agent approves or merges a PR
> to a trunk, in any repository, and auto-merge is not enabled by agents. The human's merge is the sign-off: it
> covers the PR, the archived design it carries, and the tier-2 behavioral report,
> whose MUST findings the PR body lists. The human acknowledges them by merging, or
> dismisses one with a recorded reason in a PR comment. Changes that declare a
> `security_boundary` additionally need the security note acknowledged (GLOBAL-06).
>
> While FLOW-20 stands, the loop merges a program item's pull request into
> its program trunk, which is not a trunk, and nothing else, except the
> program sync: a pull request that merges the trunk into the program
> trunk, which it merges as a merge commit, pinned to its head, once the
> first four points of the merge gate below pass, and the fifth on its
> conflict resolution when it had one (FLOW-20). It merges a program item's
> pull request only while the *merge gate* passes on its head: the base is
> the program trunk; the head is the one recorded and the remote's; every
> check on it passed or was skipped, and the required ones passed; the
> branch contains the program trunk; the tier-2 report is for that head and
> has no open MUST finding but one FLOW-20's records defer; no human step of
> the item is pending; its `security_boundary` is `none`, or its only trip
> is GLOBAL-06's relay clause with no change to a boundary's posture, which
> its review accepted (otherwise the human acknowledges its security note
> before the merge); each pull request its design names to merge first has
> merged; and the rule-change clearance passes. It squash-merges, pinned to
> that head, and records the merge (FLOW-20). The loop defers a MUST finding
> only after one fix round, and never one under GLOBAL-06, a SEC rule,
> RELAY-01 to RELAY-03 or RELAY-05, or that breaks what FLOW-20 says never
> breaks; it never dismisses one. The human's merge of the program trunk
> into its trunk is the sign-off for everything the loop merged there, and
> its pull request lists each deferred finding and each security note to
> acknowledge.

**Current wording** (`process.md:489-491`):

> ready, the human has it do so (LOOP-08). That setting is the human's,
> as the merge is: no agent changes a trunk's protection. A merge the
> human makes all the same is still the sign-off.

**Proposed:**

> ready, the human has it do so (LOOP-08). That setting is the human's,
> as the merge is: no agent changes a trunk's or a program trunk's
> protection. A merge the
> human makes all the same is still the sign-off.

**Enforcement:** `check.MergeReady`, `f7-merge` (`queue/next.go`), the
records (`wf record`, `wf program-body`).

### FLOW-13 (amend), `docs/development/process.md`

MUST, `automated`. **Why:** a16.

**Current wording** (`process.md:647-648`):

> the run: an open (queued or in-progress) item that spends `runaway_factor` times
> its estimated steps escalates to a `human` step, and no unattended run takes

**Proposed:**

> the run: an open (queued or in-progress) item that spends `runaway_factor` times
> its estimated steps escalates to a `human` step (a program item's first
> time, its estimate is raised to its spend so far instead, and recorded,
> FLOW-20), and no unattended run takes

**Enforcement:** `Schedule.Runaway` and the escalation seeding in
`queue/limits.go` and `queue/phases.go`.

### FLOW-17 (amend), `docs/development/process.md`

MUST, `agent-judgment`. **Why:** program trunks and pre-release tags (a11,
a18).

**Current wording** (`process.md:806-809`):

> Opening the PR to its trunk needs no confirmation either (FLOW-08).
> Pushing never includes any repository's trunk itself, a force-push,
> rewriting history that is already on the remote, or deleting a remote
> branch (FLOW-16). An unattended session on a native machine pushes a

**Proposed:**

> Opening the PR to its trunk or program trunk needs no confirmation either
> (FLOW-08). Pushing never includes any repository's trunk itself, a
> program trunk other than its creation at its trunk's head on the human's
> word, a force-push, rewriting history that is already on the remote,
> deleting a remote branch (FLOW-16), or a tag, except a pre-release tag
> FLOW-20 lets the loop push and a tag an attended session pushes on the
> human's word (a release tag, or a program trunk's final tag); no tag is
> moved or deleted. An unattended
> session on a native machine pushes a

### FLOW-19 (amend), `docs/development/process.md`

MUST, `automated + agent-judgment`. **Why:** a14.

**Current wording** (`process.md:875-877`):

> answered on its own. A question is flagged, and always answered on its
> own, when it touches a security boundary or a seam, or departs from
> prior art or from an earlier decision of the human's.

**Proposed:**

> answered on its own. A question is flagged, and always answered on its
> own, when it touches a security boundary or a seam, or departs from
> prior art or from an earlier decision of the human's. For a program item
> (FLOW-20), a question that touches a seam is flagged only when the seam's
> change departs from what the program's design names for the item's
> phase.

**Current wording** (`process.md:892-893`):

> Design decides whether the review is needed and records it as
> `design_review: required` or `skip` in the front-matter and the queue. `skip` is

**Proposed:**

> Design decides whether the review is needed and records it as
> `design_review: required`, `skip`, or, for a program feature, `umbrella`
> (FLOW-20), in the front-matter and the queue. An `umbrella` review is taken
> by FLOW-20's umbrella test; its line `umbrella test passed at <sha>` is an
> approval, and a failed point makes it `required`. `skip` is

`docs/design/TEMPLATE.md`'s front-matter comment becomes `required | skip
| umbrella (FLOW-19, FLOW-20)`.

**Enforcement:** `check/review.go` (`approvals`, the Plan gate,
`reviewDecision` at `:107`), `queue/queue.go:1313`'s `design_review`
validation, the `f2-umbrella` step's seeding.

### LINT-03 (amend), `docs/development/behavioral-linter.md`

MUST, `automated`. **Why:** a13, a18.

**Current wording** (`behavioral-linter.md:59-65`):

> Tier 1 red blocks the release outright. Tier 2 MUST-severity findings require
> explicit human sign-off (acknowledge, or dismiss with a recorded reason) before the
> release proceeds; SHOULD findings are informational. The agent never auto-blocks and
> never auto-approves trunk.
>
> Per PR, the sign-off is the human's merge (FLOW-08): the PR body lists the
> MUST findings, and a dismissal is recorded as a PR comment.

**Proposed:**

> Tier 1 red blocks the release outright. Tier 2 MUST-severity findings require
> explicit human sign-off (acknowledge, or dismiss with a recorded reason) before the
> release proceeds; SHOULD findings are informational. The agent never auto-blocks,
> never auto-approves trunk, and never dismisses a MUST finding.
>
> Per PR, the sign-off is the human's merge (FLOW-08): the PR body lists the
> MUST findings, and a dismissal is recorded as a PR comment. A pull request the
> loop merges into a program trunk carries its findings, any MUST finding it
> deferred among them, into the program's records, and the human's merge of the
> program trunk into its trunk signs them off (FLOW-08, FLOW-20). A pre-release a
> program trunk tags needs tier 1 green on its commit; the full gate below is
> for a release without a suffix.

**Enforcement:** `release.yml:71` gains `--exclude '*-*'` in its `tier1`
job, so a release without a suffix is diffed against the last one.

### LOOP-01 (amend), `docs/development/loop-contract.md`

MUST, as written. **Why:** a11.

**Current wording** (`loop-contract.md:102-106`):

> leaves out is the term `none`. The loop's configuration also records
> each repository's trunk, and that record is the trunk the loop's tools
> use, never the file's: a profile file whose trunk differs from it is a
> finding, and every check of the loop's tools that reads the file fails
> on it.

**Proposed:**

> leaves out is the term `none`. The loop's configuration also records
> each repository's trunk, and that record is the trunk the loop's tools
> use, never the file's: a profile file whose trunk differs from it is a
> finding, and every check of the loop's tools that reads the file fails
> on it. While FLOW-20 stands, it also records a repository's program
> trunk, which is not a trunk and is not in the profile file.

**Enforcement:** `RepositoryEntry.ProgramTrunk` and `Validate`.

### LOOP-07 (amend), `docs/development/loop-contract.md`

MUST, `agent-judgment`. **Why:** a17, a18.

**Current wording** (`loop-contract.md:321-324`, `:331-333`):

> 2. A consumer adopts a release through a **reconciliation item**: an
>    item in the queue that names the consumer's repository,
>    `adopt <consumable> at <version>`. It is a feature item (LOOP-04),
>    filed through intake, and FLOW-05 gives its place in the order.

> 5. A component's trunk names only released versions of what it
>    consumes. A branch may name an unreleased commit while its work is in
>    progress.

**Proposed:**

> 2. A consumer adopts a release through a **reconciliation item**: an
>    item in the queue that names the consumer's repository,
>    `adopt <consumable> at <version>`. It is a feature item (LOOP-04),
>    filed through intake, or by the loop for a pre-release on a program
>    trunk (FLOW-20), and FLOW-05 gives its place in the order.

> 5. A component's trunk names only released versions of what it
>    consumes, never a pre-release. A program trunk (FLOW-20) may name a
>    pre-release. A branch may name an unreleased commit while its work is
>    in progress.

**Enforcement:** `tools/conformance/check-pins.sh` by base branch.

### PROFILE-01 (amend), `docs/development/loop-profile.md`

MUST, as written. **Why:** a11, a18.

**Current wording** (`loop-profile.md:26`, the last sentence of `:31`,
the end of `:41`, and `:42`, `:43`):

> | Trunk | `main`. |

> Each pin is a release tag on `main` and on a pull request to it; a branch may pin a commit of an unmerged specifications branch while its work is in progress. |

> It is not one of this repository's rule files: it changes in an attended session, on the human's word (FLOW-05). `tools/tier1/area-edges.sh` prints this repository's dependency edges, which the loop compares the map's `reaches` with. |

> | Release | A `v*` tag push runs the `release` workflow (GoReleaser), whose `tier1` job is the gate's deterministic half (LINT-03), and the `relay-image` workflow. The human's sign-off of a tier-2 report over the release diff has no wired step yet. |

> | Consumables | Releases: the `v*` tags. A release that carries the files `tools/conformance/<specification>/go.mod` implements each seam specification at the version its file pins. Consumes: the releases of the seam specifications in `buildwarden/buildwarden-specs`, each with its conformance suite, at the versions in those files. |

**Proposed:**

> | Trunk | `main`. While FLOW-20 stands, the program trunk is `program/componentization`, recorded in the loop's configuration only (LOOP-01). |

> Each pin is a release tag without a suffix on `main` and on a pull request to it; on the program trunk and a pull request to it, a pin may also be a pre-release tag (`-dev.N`, `-rc.N`, FLOW-20); a branch may pin a commit of an unmerged specifications branch while its work is in progress. |

> It is not one of this repository's rule files: it changes in an attended session, on the human's word, or by the loop's addition of areas a program design names (FLOW-05, FLOW-20). `tools/tier1/area-edges.sh` prints this repository's dependency edges, which the loop compares the map's `reaches` with. |

> | Release | A `v*` tag push runs the `release` workflow (GoReleaser), whose `tier1` job is the gate's deterministic half (LINT-03), and the `relay-image` workflow. A tag with a suffix, which only the loop pushes and only on the program trunk (FLOW-20), publishes as a GitHub pre-release and moves neither the Latest release nor the relay image's `latest`; a tag without one is the human's, from `main`. The human's sign-off of a tier-2 report over the release diff has no wired step yet. |

> | Consumables | Releases: the `v*` tags; those with a suffix are consumed only by program trunks (LOOP-07). A release that carries the files `tools/conformance/<specification>/go.mod` implements each seam specification at the version its file pins. Consumes: the releases of the seam specifications in `buildwarden/buildwarden-specs`, each with its conformance suite, at the versions in those files. |

**Enforcement:** `check-pins.sh`, `pin-specs.sh`, `.goreleaser.yaml`,
`relay-image.yml`. `loop-profile.yaml` is unchanged.

### UX-03 (amend), `docs/development/ux-cli-config.md`

MUST, `automated + agent-judgment`. **Why:** a6.

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
trailer naming FLOW-20 (a9).

### ORCH-03 (amend), `docs/development/orchestrator.md`

MUST, `automated + agent-judgment`. **Why:** a6.

**Current wording** (`orchestrator.md:70-73`):

> read is an error. Configuration precedence stays `flag > environment variable >
> the config file > default`. Defaults are sane and changes are additive. A new
> or changed config key is documented in the same change; this is the config half
> of the docs-coupling rule (UX-01).

**Proposed:**

> read is an error. Configuration precedence stays `flag > environment variable >
> the config file > default`. Defaults are sane and changes are additive, except
> where FLOW-20 lets a program item break a key. A new
> or changed config key is documented in the same change; this is the config half
> of the docs-coupling rule (UX-01).

**Enforcement:** unchanged.

## Security note

`security_boundary: none`: no product boundary (host, inner build
environment, relay) changes. The change is to who may merge into a
non-release branch. Its limits: the loop never merges into `main`; it merges
only under the merge gate, pinned to the head GitHub will check; a finding
that could open a boundary is never deferred; a program PR that changes a
boundary's posture waits on Jeff's acknowledgment of its security note; a
change to CI or release configuration, which runs with signing and package
write access on the loop's tags, is never loop-merged; and every program trunk lands
on `main` only by Jeff's squash merge, with each deferred finding and
security note listed. A pre-release is never Latest and never moves the
relay image's `latest`. The program trunk's ruleset (required checks, no
force-push or deletion) is Jeff's, and no agent changes it.

## Validation plan

- **Tier 0** (TEST-01, TEST-02; `make test`), states built with `Add`,
  `PutOnHold`, `Claim` and `Complete`:
  - `queue`: `Held` and the limits as in the earlier draft; `TrunkFor`
    gives the program trunk only for a program, non-process item with no
    machinery, `.github/` or `.goreleaser.yaml` area in an entry that has
    one, and `main` for one that reaches `.github/`; `Validate` refuses a program
    trunk equal to the trunk; a merge step on a program trunk is offered to
    a cloud worker only when the gate passes and never otherwise; a PR at
    `f7-merge` does not wait on the human; a program item's third return
    does not escalate and its fifth does; its first runaway raises the
    estimate and records it, its second escalates.
  - `check`: `MergeReady` fails on each of its nine points alone, with
    fixtures (a base other than the program trunk; a moved head; a failed or
    missing required check; a branch behind; an open protected MUST; a
    pending human step; a boundary not accepted, or a posture change; a
    merged-first PR open; an uncleared rule change) and passes with none,
    including one whose push-event job on the same head was skipped; the merge proof accepts a
    squash on the program trunk; `approvals` accepts an umbrella line and
    the Plan gate fails on an impact field changed after it; the declared
    check fails on a `program` mark that disagrees with `related`.
  - `summary`: **Held** and **Decided for you**.
  - `cmd/wf`: `-resumes`; `wf trunk`; `wf main -trunk`; `wf record`
    appends; `wf program-body` renders every record kind; `wf tag-next`
    gives `v0.7.0-dev.1` after `v0.6.0`, then `-dev.2`, nothing on a
    non-consumable merge, `v0.1.0-dev.1` in a repository with no release,
    and nothing for the specifications repository until
    `specs-prerelease-tags` has merged.
- **Scripts:** `check-pins.sh` accepts `v1.2.0-dev.3` with
  `GITHUB_BASE_REF=program/componentization` and refuses it with `main`;
  `pin-specs.sh` refuses `SPECS=newest` on `main`.
- **CI:** `actionlint` on the changed workflows; a GoReleaser `check`.
- **Text checks:** each amended block matches the approved wording byte for
  byte; `grep` finds no "Never merge" without its exception in `.claude/`.
- `make test`, `make lint`, `tools/workflow/flow18-check.sh`, `make
  enforcement`.
- **The real check, after the state step:** `dist/wf status` shows the held
  items under **Held**; `dist/wf trunk -feature driver-contract` prints
  `program/componentization`; a PR to the program trunk gets the `ci` and
  `flow18` checks; the first program merge is recorded in
  `program/records.yaml`.
- No native host is needed.

## Implementation plan

A process item has no Plan phase; this is the work of `x2-implement`. Each
step writes its tests first, sees them fail on the code before it (TEST-01,
TEST-02), and ends with `make test` green.

1. Holds: `queue/queue.go` `Question.Resumes`, `Feature.Held`,
   `PutOnHold(resumes)`; `queue/limits.go` `counted`, `WaitsOnHuman`;
   `queue/summary.go` **Held**; `cmd/wf/main.go` `-resumes` (FLOW-05, a3).
2. Program trunk: `RepositoryEntry.ProgramTrunk`, `Feature.Program`,
   `Config.TrunkFor`, `Validate`; `check/repos.go` `trunk()`; the declared
   check; `wf trunk`, `wf main -trunk`, `wf stale`, `wf areas`, `wf drops`,
   `wf new -program` (FLOW-20, LOOP-01, a11).
3. Merge gate: `check.MergeReady`, `wf check -step f7-merge`; the merge
   label; `queue/next.go` and `queue/phases.go` seed and offer `f7-merge`;
   `queue/limits.go` waiting rule; `wf sync-program` (FLOW-03, FLOW-04,
   FLOW-05, FLOW-08, a12).
4. Records: `wf record`, `wf program-body`, **Decided for you** in
   `QUEUE.md` and the digest (FLOW-20, a13).
5. Gates: `design_review: umbrella` (the validators `queue/queue.go:1313`
   and `check/review.go:107`, and `TEMPLATE.md`'s comment), the
   `f2-umbrella` step, `approvals`;
   the FLOW-06 and FLOW-13 carve-outs (FLOW-19, a14 to a16).
6. Pre-releases: `wf tag-next`; `.goreleaser.yaml`, `relay-image.yml`,
   `release.yml`, `check-pins.sh`, `pin-specs.sh`; `CONTRIBUTING.md`'s pin
   and release paragraphs (`:186-187, :205-219`) (a18).
7. CI triggers: `main.yml` and `flow18.yml` add `'program/**'`.
8. Rules: FLOW-20, FLOW-03, FLOW-04, FLOW-05, FLOW-06, FLOW-08, FLOW-13,
   FLOW-17, FLOW-19 in `process.md`; LINT-03; LOOP-01 and LOOP-07;
   PROFILE-01; UX-03; ORCH-03; as approved (FLOW-14).
9. Skills and agents as under **Design, Skills and agents**:
   `workflow-tick`, `workflow-human`, `workflow-intake`, `workflow-native`,
   `wf-publish`, `wf-review`, `wf-design`; the workflow branch's
   `README.md` schema text goes in the state step.
10. `make test`, `make lint`, `flow18-check.sh`, `make enforcement`, the
    text checks; merge current `main`, archive this design with its
    summary, and open the PR to `main` with CI green (FLOW-03 process phase
    2, FLOW-12). Jeff merges it.

**State step** (`x4-state`, recorded at review with `wf state-step`;
Jeff's, after the merge, in one attended sitting):

1. `config.yaml`: `program_trunk: program/componentization` on the
   `buildwarden/buildwarden` and `buildwarden/buildwarden-specs` entries.
2. Create each program trunk at its `main` head, on Jeff's word in this
   sitting (FLOW-17): `git push origin
   origin/main:refs/heads/program/componentization` in each repository.
3. `resumes:` on each open hold: `program-end` for prebuilt-artifacts,
   workdir-in-context, vm-from-host-path, configuration-bugfixes,
   auto-tidy-pull-request, relay-exit-code-unreadable;
   relay-collector-failure-fails-open: "the relay's separation (M1 onward)
   has merged"; runtime-former-values-removal: "the change that prepares
   1.0". config-lookup-test-symlinked-tmp is released and stays on `main`;
   wf-state-migration-step stays queued.
4. `program: true` on driver-contract and relay-release-forms-spec.
5. File `program-land` (process, held, resumes: "every program trunk has
   the program's last phase merged") and `specs-prerelease-tags` (item in
   `buildwarden/buildwarden-specs`, trunk `main`, Jeff's merge: its release
   workflow accepts program-trunk pre-release tags with `--prerelease`).
6. Create `program/records.yaml` (empty list) and the workflow branch's
   `README.md` schema for `resumes`, `program`, `program_trunk`,
   `design_review: umbrella` and the records.
7. `dist/wf status`, `QUEUE.md`; `dist/wf commit`.

**What only Jeff can do, once** (outside the repository; no agent does
these, FLOW-08):

- **Rulesets.** On `program/componentization` in each repository (an
  organization ruleset on `program/**` covers new repositories too):
  require a pull request with no approving review (the App cannot approve
  its own); require the `ci` and `flow18` checks and the branch up to date;
  allow squash and merge commits, not rebase; block force-push and
  deletion; no bypass for the App. A tag ruleset that lets the App create
  `v*-dev.*`, `v*-rc.*` and `*/v*-dev.*`, `*/v*-rc.*` tags, never update or
  delete any tag, and reserves bare `v*` tags to him, where GitHub's
  patterns allow. `main`'s protection is unchanged.
- **GitHub App permissions.** Confirm the loop's App has `contents: write`
  and `pull_requests: write` on each repository, which a merge needs (it
  already pushes branches and opens PRs; merging is not verified).
- **The nightly task's prompt.** It says "Never merge" today; change it to
  "merge only as the `workflow-tick` skill allows: a program item's PR into
  its program trunk at `f7-merge`, and the program sync". The scheduled
  native sessions merge nothing and need no change.
- **`specs-prerelease-tags`**: merge it in the specifications repository
  once its PR is ready.
- **The landing merges** at the program's end (Decision 12), the
  `program/componentization-final` tags before them, and the full release
  tags after them.

Estimate: 10 steps (9 + 1 state step), two runs; the queue's estimate of 5
is raised at the review, on Jeff's word.

## Design review

Pending.

## Consolidation

A process item has no Publish phase and no consolidation pass (FLOW-03).

## Rollback notes

Setting `program_trunk` off in `config.yaml` returns every item to `main`
in wf; the program trunk's content then lands by an early `program-land`.
A tag already pushed stays (never moved or deleted).

## Archive summary

Filled in at the item's PR (FLOW-12).
