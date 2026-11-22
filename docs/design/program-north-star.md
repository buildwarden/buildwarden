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
                         # and .goreleaser.yaml; specs: tools/conformance/{check-pins,pin-specs}.sh,
                         # specs/README.md's layout and specs/specs_test.go (the ledger's working
                         # draft, a7); book: CONTRIBUTING.md's release and pin paragraphs
security_boundary: none  # no product boundary moves; what the loop may merge is the process's own gate (FLOW-08)
alters_relationships: false  # no product component's role or interaction changes (FLOW-02)
design_review: required  # a process item's design gets its review (FLOW-03, FLOW-19); this one
                         # changes rules on merging, pushing and tags, so FLOW-20's process test
                         # (a5 (b)) would send it to the human as well; it also has rule changes
                         # (FLOW-14)
native_validation: []    # pure functions of the queue state, rule text and CI configuration
tier2_gaps: []
rules: [FLOW-02, FLOW-03, FLOW-04, FLOW-05, FLOW-06, FLOW-07, FLOW-08, FLOW-09, FLOW-10,
        FLOW-11, FLOW-13, FLOW-14, FLOW-16, FLOW-17, FLOW-18, FLOW-19, FLOW-20, LINT-03, LINT-04,
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
  - {rule: FLOW-07, change: amend, paths: [docs/development/process.md]}
  - {rule: FLOW-08, change: amend, paths: [docs/development/process.md]}
  - {rule: FLOW-13, change: amend, paths: [docs/development/process.md]}
  - {rule: FLOW-14, change: amend, paths: [docs/development/process.md]}
  - {rule: FLOW-17, change: amend, paths: [docs/development/process.md]}
  - {rule: FLOW-19, change: amend, paths: [docs/development/process.md,
                                          docs/design/TEMPLATE.md]}
  - {rule: LINT-03, change: amend, paths: [docs/development/behavioral-linter.md,
                                          .github/workflows/release.yml]}
  - {rule: LINT-04, change: amend, paths: [docs/development/behavioral-linter.md,
                                          .github/workflows/release.yml]}
  - {rule: LOOP-01, change: amend, paths: [docs/development/loop-contract.md]}
  - {rule: LOOP-07, change: amend, paths: [docs/development/loop-contract.md]}
  - {rule: LOOP-09, change: amend, paths: [docs/development/loop-contract.md]}
  - {rule: PROFILE-01, change: amend, paths: [docs/development/loop-profile.md]}
  - {rule: UX-03, change: amend, paths: [docs/development/ux-cli-config.md]}
  - {rule: ORCH-03, change: amend, paths: [docs/development/orchestrator.md]}
concepts: [workflow, componentization, program, program-trunk, freeze, hold, limits,
           waiting-on-human, process-item, program-payoff, process-test, compatibility,
           interfaces, ledger-working-draft, intake, loop-merge, merge-gate, merge-commit,
           umbrella-test, escalation, records, pre-release, pins, human-gates]
related: [componentization.md, loop-parallel.md, alters-slot.md, merge-confirm-attended.md,
          queue-positions.md, loop-repository.md, seam-specs.md]
---

# The componentization program is the north star

A process item (FLOW-03, LOOP-09), and a program item: it lists
`componentization.md` in `related` (FLOW-20). It holds no worker. It amends
FLOW-20 so that the program has the queue's priority until it completes; it
gives the program a **program trunk** in each repository it works in, which
the loop merges program items into without Jeff, each as a merge commit,
ending with one squash merge of each program trunk into `main` that Jeff
makes; it lets only three kinds of step wait on Jeff during the program
(setup, component design, and work that needs his privileges or
configuration), and the escalations of a program item that does not
converge that he kept, while the loop decides everything else, records it
and lets him call it back; it lets a process item that touches no rule
FLOW-20 reserves to him (one that widens the loop's authority, or changes a
rule about security, merging, pushing or tags) pass a **process test** in
place of his review; and it lets the loop tag
pre-releases on program trunks. Jeff answered its eighteen questions on
2026-11-22, at `8cb13d8`; four answers depart from the recommendation (a5,
a7, a11, a15), and the design is revised for each.

Words used throughout. **wf** is the loop's tool, `tools/workflow/`. The
**program** is the work `componentization.md` sets out. A **hold** is an open
blocking question that keeps an item's steps from being offered (FLOW-07).
The **program trunk** is the branch `program/componentization` of a
repository (Decision 8). A **loop merge** is the loop's merge of a pull
request into a program trunk (Decision 9). The **records** are the
workflow branch's `program/records.yaml` (Decision 13). The three **kinds**
of step that may wait on Jeff (Decision 11) are **setup** (kind 1),
**component design** (kind 2) and **privileged** work (kind 3); a step of
none of them, and no **escalation** Jeff kept (a15 (b)), is **decided by the
loop**: recorded, named in the digest and `QUEUE.md` under **Decided for
you**, listed in the landing PRs, and callable back. The **process test**
settles a process item's design in Jeff's place when it touches no
reserved rule (Decision 6). Rule wording and
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

And, after the first draft of this design, on what that leaves for him:

> Generally speaking, what I expect to be brought in the loop on are: (1) an
> initial bulk-setup of the necessary repositories/infrastructure; (2)
> meaningful design questions/reviews around the components, not on the
> strategy to implement; (3) work/testing requiring elevated privileges or
> configuration setup from me directly.

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
10. **The rules send Jeff steps of none of his three kinds.** An item that
    returns to Design a third time (FLOW-06, `queue/phases.go:605`), spends
    its runaway budget (FLOW-13) or exhausts its CI repairs (FLOW-11)
    escalates to a `human` step about a stuck item, not a design. FLOW-07
    makes every blocking question his, a question on how to implement
    included. componentization a11 asks his word for each repository as its
    phase starts, rather than in one setup. Jeff kept escalation for an
    item that does not converge, later than today (a15 (b)); the design
    moves the thresholds rather than removing the steps, and FLOW-11 stays
    as written.

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
  value of it (a10).
- **`seam-specs.md`** (a15, Decision 12) has Jeff, or an attended session
  on his word, push specification release tags. Pre-release tags on a
  program trunk are the new case.
- **wf's merge proof** already accepts a merge commit: `merged()` greps the
  trunk for `(Merge pull request |\()#N` (`check/check.go:514-515`), which
  a merge commit's subject matches, and `refMerged` first asks whether the
  branch head is an ancestor of the trunk (`check/check.go:591-593`),
  which a merge commit makes true; its tree comparisons are for a squash,
  which no loop merge makes. FLOW-16 already covers deleting a merged
  branch.
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
   Nothing waits on Jeff for a hold: holding is the default he set on
   2026-11-21, releasing one is his word whenever he gives it, never a step,
   and at the program's end the loop resumes each (Decision 12).
3. **A hold names when it resumes, and a held item counts toward no limit
   (a3).** A hold question gains `resumes`; wf treats an item with an open
   question that has `resumes` as held: it holds no limit, does not wait on
   the human, and `QUEUE.md` lists it once under **Held**.
4. **Parallelism comes from declared areas, not new limits (a4).** The
   numbers stay; a program item declares its repositories and areas by the
   end of its Design at the latest, and its design is scoped to one review.
5. **No new order rule.** With non-program work held, program items lead;
   intake places a program item above any released non-program item of its
   class. Process items stay class 2 (LOOP-04). The order is the loop's to
   follow, not a step: Jeff may move an item whenever he says so.
6. **Loop work meets Jeff's bar; only reserved rule changes reach him
   (a5 (b)).** A process item filed while FLOW-20 stands carries a
   **Program payoff** section. Its design goes to Jeff's review (kind 2)
   only when it touches a **reserved rule**, the two classes of his
   answer: it widens the loop's own authority, read by its effect, or it
   changes a rule about security, merging, pushing or tags. FLOW-20's Loop
   work bullet holds the one definition, and Design, *The process test*,
   lists the cases: widening covers a step, merge, push, tag, filing or
   decision the loop may take in his place that it could not before, a
   gate of his removed or narrowed, either test of this design loosened, a
   check that stops the loop relaxed, and a limit, budget or permission
   raised. Any other process item
   passes the **process test** in his place (Design, *The process test*):
   P1, the payoff is shown; P2, no reserved rule is touched; P3, a review
   session that did not write the design agrees. A pass is recorded
   (Decision 13) and is the approval for its Implement. Its merge to
   `main` stays his (kind 3), so he still sees every process item once,
   at its merge, with its state step. This amends LOOP-09 and FLOW-14,
   which make every process item's design his, FLOW-03's process route,
   and FLOW-19's review values. wf-state-migration-step stays queued, as Jeff decided on
   2026-11-21; it takes the process test, or his review if it touches a
   reserved rule.
7. **Interfaces may break for the program, within fixed limits (a6 to
   a8).** Go APIs, internals and the driver contract (GLOBAL-08); a seam by
   a new pre-1.0 release of its specification first; CLI flags, subcommands,
   config keys, environment variables and output forms with no alias or
   window, only where the program needs it, a released form refused with an
   error naming its replacement (UX-04). Never broken: the ledger wire
   format v1 and its signatures (additive only during the program), the
   reader's support for every ledger a release wrote, a written schema
   `$id`, and every security boundary. If the program needs a ledger v2,
   it is developed as a **working draft** beside v1 on the program trunk,
   marked unreleased, written by no tagged build, and finalized and
   released as v2 only at the program's landing (a7; Design, *The
   ledger's working draft*). A program trunk may lack a capability for a
   while when a design names the gap; `main` never does, and no gap is
   open at the final squash.
8. **Each repository the program works in has a program trunk (a10).**
   - Name: `program/componentization`, in `buildwarden/buildwarden` and
     `buildwarden/buildwarden-specs`, and in each program repository, all
     created in the one-time setup (a18).
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
9. **The loop merges program pull requests into the program trunk (a11 (b)).**
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
        validation pending (FLOW-09); after Decision 11, every such step is
        of one of the three kinds or an escalation (a15 (b));
     7. `security_boundary` is `none`, or its only trip is GLOBAL-06's
        relay clause with no posture change (a14), the umbrella test or
        design review accepted it, and Review recorded that the diff stays
        within it; an item with any other `security_boundary` changes a
        component's security posture, component design (kind 2), and waits
        at `f7-merge` on Jeff, who acknowledges its security note there;
     8. every pull request its design names to merge first has merged;
     9. the rule-change clearance passes (FLOW-14).
   - The agent merges with `gh pr merge <n> --merge --match-head-commit
     <head> -R <repo>`: one merge commit per item on the program trunk,
     whose first parent is the program trunk's previous head, so `git log
     --first-parent` on the program trunk reads one commit per item and the
     item's own commits stay beneath it; refused by GitHub if the head
     moved after the check. Never a squash or a rebase, `--admin`,
     auto-merge, an approval or a bypass. After fetching the program trunk,
     `wf done` runs the existing merge proof, which finds the head an ancestor of the program trunk
     and records the `Merge pull request #N` commit as `work[].merged`,
     and `wf record` appends the merge to the records.
   - One loop merge per program trunk per run; the others wait for their
     refresh, as FLOW-08 already has them.
   - A process item's PR, and any change to the loop's rules or machinery,
     never goes to a program trunk except as part of `main` merged in by the
     program sync (Decision 14) (LOOP-09): it goes to `main`, and Jeff
     merges it as today, whether his review or the process test settled
     its design, so every scheduled run takes it from `main`. A
     merge to `main` is privileged (kind 3): `main`'s protection reserves it
     to him, and no agent changes that (FLOW-08). So is the merge of a
     program item that changes CI or release configuration, which runs with
     the release's signing and package-write credentials.
10. **Tier-2 MUST findings on a program PR (a12).** Review fixes a MUST
    finding in a fix round and re-reviews the new head. One still open
    after one fix round is **deferred**: Review records the finding, the
    head and its reason in the records, and the loop may merge. A MUST
    finding under GLOBAL-06, a SEC rule, RELAY-01 to RELAY-03 or RELAY-05,
    or the never-break list of Decision 7 is never deferred and never
    merged over: the item returns to Design with the finding (FLOW-06) for
    a plan that conforms, recorded. It reaches Jeff only when the loop
    judges the finding mistaken, since only he may dismiss it, or when the
    design would need the posture or contract the finding protects to
    change; both are component design (kind 2). Every deferred finding is
    listed in the landing PR's body, where Jeff's merge acknowledges or he
    dismisses it (LINT-03). The loop never dismisses a finding.
11. **Only three kinds of step wait on Jeff, and the escalations he kept
    (a5, a13 to a16, a18).** In his words of 2026-11-21: (1) **setup**, the
    initial bulk setup of the program's repositories and infrastructure
    (Decision 16); (2) **component design**, a question or review about a
    component's seams, boundaries, contracts or security posture, not
    about the strategy to implement it, and the design of a process item
    that touches a reserved rule (a5, Decision 6); (3) **privileged**
    work, which needs his privileges or configuration: a merge to `main`, a
    change to CI or release configuration, a release without a suffix,
    making a repository public, the machines of `hosts.yaml`, a native step
    only his attended machine can take, and a failure only those can fix.
    Besides these, an **escalation** of a program item that does not
    converge reaches him, later than today's rules send it (a15 (b),
    below). Every other step the draft sent him is decided by the loop,
    recorded (Decision 13) and callable back. Design, *What reaches Jeff*,
    lists each gate with its kind, and what the loop does instead where it
    has none.
    - **Process items (a5 (b))**: the process test takes the design review
      of a process item that touches no reserved rule (Decision 6). A
      failed P2, or a session that cannot judge P2, sets `design_review:
      required` and seeds Jeff's review (kind 2); a failed P1 holds the
      item until the program's end, recorded; a disagreement on P1 alone
      is a failed P1. His merge of it to `main` is kind 3 either way.
    - **The umbrella test** takes a program feature's design review in
      place of Jeff (Design, *The umbrella test*). Its component-design
      points U1 to U5 send a design to him when one fails: the design
      implements components, seams and boundaries the program's design
      names for one phase (or a filed split, follow-up or re-sequencing of
      them); no component-design question is open; no rule change and no
      loop machinery change; no boundary posture change; no interface
      break FLOW-20 does not allow. Its loop points U6 and U7 (the areas
      checked against the map and the dependency edges; an independent
      reviewer session agrees) return a failing design to Design, not to
      him. A pass is recorded as `umbrella test passed at <sha>` and is an
      approval for the Plan gate.
    - **Questions on a program item (a13).** A question is blocking, or
      flagged, only when it is component design (a seam's change the
      program's design does not name for the item's phase, a component's
      boundary or ownership, a contract, a security posture) or needs setup
      or privileged work. Any other question, however hard, is about the
      strategy to implement: it is assumed, proceeds on Design's
      recommendation, and is confirmed by the umbrella test's session and
      recorded. A design follows Jeff's recorded decisions; departing from
      one on component design flags the question, and departing from one
      on strategy is recorded and listed first under **Decided for you**.
    - **Security (a14)**: a design whose only GLOBAL-06 trip is "any change
      to the relay itself", with no change to a boundary's posture, passes
      the test with its security note read by the independent session; the
      note's acknowledgment moves to the landing PR. A posture change is
      component design (kind 2) and goes to Jeff, at its design and at
      `f7-merge`.
    - **An item that does not converge (a15 (b))** escalates later than
      FLOW-06 and FLOW-13 have it, and as FLOW-11 has it: its third and
      fourth returns to Design do not escalate, and the return after the
      fourth (its fifth, "more than four times") escalates to a `human`
      step, as the earlier draft had it; its first runaway budget raises
      its estimate to its spend so far, and its second escalates; a CI
      failure Publish cannot repair in three attempts escalates (FLOW-11
      as written); a program sync whose conflict the loop cannot resolve
      leaves the program trunk where it is and escalates (Decision 14). A
      return past the second and a re-estimate are recorded; each
      escalation is a `human` step, named in the digest as an escalation,
      not as one of the three kinds.
    - **Filing (a16)**: the loop files, unattended and recorded, the items
      of a phase row once the Order's gate before its wave is met, the
      reconciliation items that adopt a program pre-release on a program
      trunk, and the relations between items their designs name; and it
      makes the area-map changes a reviewed or umbrella-passed program
      design names, or that bring the map's `reaches` in line with
      `tools/tier1/area-edges.sh`.
    - **Repositories (a18)**: every program repository on the umbrella's
      list is created private by Jeff in the one-time setup (kind 1), its
      CI and flow18 workflows committed to its `main` and their job names
      required by its ruleset before its program trunk is cut, so nothing
      waits on him when its phase starts. A
      repository not on the list is a new component, so proposing one is
      component design (kind 2); making one public is privileged (kind 3).
    - **Holds, the urgent mark and the order** are Jeff's word whenever he
      gives it; no step waits on it (Decisions 2 and 5).
12. **The program ends with one human squash merge per repository.** A
    process item, `program-land`, filed on hold by this item's state step
    (resumes: every program trunk has its last phase merged), closes the
    program. Its done-when is the loop's evidence, shown in each landing
    PR: every component, built from its own repository at its program
    trunk, passes its CI and conformance suites at the other components'
    `-rc` tags, and TEST-08 and the blocking native validations pass at
    those heads; and no ledger working draft is still marked unreleased
    (Decision 7): if one was opened, the program item that finalizes it as
    v2 has merged on its program trunk. Its design names, as pull requests
    to merge first, one PR
    per repository from `program/componentization` to its trunk, whose
    body wf renders from the records (`wf program-body`). Jeff
    squash-merges each, specifications first (kind 3: merges to `main`);
    his merge is the sign-off for everything the loop merged there, a
    finalized ledger v2 included. Full release tags follow from `main` as
    today, his (kind 3); the first of them is the release of ledger v2,
    if there is one. Before the landing PRs open, the loop tags each
    program trunk's head `program/componentization-final`, recorded, so
    the per-item merge commits, which his squash does not carry to
    `main`, stay reachable. Its own PR retires FLOW-20 and every program-trunk
    clause, and its state step drops `program_trunk` from `config.yaml`.
    Its Close resumes every item held until the program's end, each
    returning to Design for what the program changed, recorded; Jeff may
    close any of them instead.
13. **Records.** Everything this item lets the loop decide in Jeff's place
    is appended by `wf record` to the workflow branch's
    `program/records.yaml`: a loop merge, a deferred MUST finding, a
    protected finding sent back to Design, an umbrella test, a process
    test, a security note read under a14, a question settled as assumed, a
    departure from a strategy decision of his, a return to Design past the
    second, a re-estimate, a sync's conflict resolution, a loop-filed item
    or relation, an area-map change, a pre-release or final tag, and a
    hold resumed at the end. Each is named in the run's digest and `QUEUE.md` under **Decided
    for you**, and listed again in the landing PRs. Jeff may call any back
    at any time: a blocking question on an open item, or a follow-up
    program item for a merged one.
14. **The loop merges `main` into each program trunk.** When `main` moves
    past a program trunk (in practice, a process item's merge), the loop
    cuts `sync/program-<main sha>` from the program trunk, merges `main`
    with conflict-only fixes, runs the fast gate, opens a PR to the program
    trunk and merges it once the sync's own check passes (the sync
    counterpart of the merge gate's points 1 to 4: base, head, checks, and
    a head that contains both `main` and the program trunk; Design, *The
    merge gate and the loop merge*), and also point 5 on the resolution
    diff when the merge had conflicts, with
    `--merge`, a merge commit like every loop merge (Decision 9), so
    `main` stays an ancestor of the program trunk and the landing PRs stay
    conflict-free. A conflict it cannot resolve escalates (a15 (b)): the
    loop leaves the program trunk where it is, keeps no sync branch
    pushed, and files a program item `program-sync-<main sha>` whose first
    step is a `human` escalation carrying the conflict; Jeff says how it
    resolves, and the item's Design follows his word, through the umbrella
    test and the merge gate. Both are recorded. Program items go on
    against the program trunk meanwhile; the next sync waits for that
    item, and `program-land` needs every such item merged.
15. **Pre-release tags (a17).** On a program trunk, after a loop merge that
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
    release without a suffix stays Jeff's (kind 3), from `main`, through
    the full gate.
16. **What only Jeff can do, once: the setup (kind 1).** Review and merge
    this item's PR; run its state step (with the program trunks' creation
    on his word in it); create every program repository on the umbrella's
    list, commit its CI and flow18 workflows to its `main` and add their
    job names to its ruleset's required checks, before its program trunk
    is cut (a18); merge `specs-prerelease-tags` in the specifications
    repository; set each program trunk's ruleset, the tag ruleset and
    automatic deletion of head branches; confirm the GitHub App may merge;
    change the nightly task's prompt, which today says "Never merge". After
    that, nothing advances a program trunk on Jeff except the steps of kind
    2 and 3 that Design, *What reaches Jeff*, lists, ending with the
    landing merges.
17. **Enforcement stays small where judgment is the check.** The hold
    counting, the trunk choice, the merge gate, the later escalation
    thresholds, the process test's step and approval line, and the records
    are wf code. Program membership, the payoff, each interface break, a
    question's kind, the umbrella test's points U1 to U6 and the process
    test's P1 and P2 stay agent judgment at intake, Design and the
    independent session (U7, P3).

## Questions

### Blocking

None.

### Assumed

**Answered on 2026-11-22.** Jeff answered a1 to a18 in his review of the
design at `8cb13d8`. Each entry below keeps its options and its
recommendation, and says which he chose. Fourteen were confirmed as
recommended: a1 to a4, a6, a8 to a10, a12 to a14 and a16 to a18. Four
depart from the recommendation, and the design is revised for each: a5
(b), a7 (his own wording), a11 (b) and a15 (b). At a follow-up later the
same day he confirmed a5 (b) and a15 (b) as the design reads them, and
moved each new program repository's CI into the setup sitting (a18); each
answer says so.

Eighteen questions; all but a9 are flagged, each because it sets the
wording of a rule or departs from an earlier decision of Jeff's. Earlier
drafts asked a9's tier-1 trailer on its own; it is the mechanism of a6's
choice and is now one of a6's options.

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

  **Answered by Jeff, 2026-11-22: (a), as recommended.** The program ends
  after the landing merges.
- **a2. Security and regression fixes during the program.** Flagged:
  removes FLOW-20's two exemptions, which componentization a4 confirmed,
  and sets the Hold bullet and FLOW-05's urgent sentence.
  - (a) Held like other non-program work unless Jeff releases one. The fix
    for a failing `main` or program trunk and a fix that `follows` a
    program item are never held. Marking a fix urgent does not release its
    hold. Releasing is his word whenever he gives it, not a step; a held
    fix is listed under **Held** so he sees it; the loop resumes each hold
    at its resume point.
  - (b) Keep both exemptions; Jeff may hold a fix whose code the program
    replaces.
  - (c) Keep FLOW-20 as written; the 2026-11-21 holds stay one-off.

  Recommended: (a). It is what Jeff did on 2026-11-21, and the red-trunk
  exception keeps refreshes running (FLOW-10). A non-program follow-up an
  unattended session files is held with the followed item's resume point.
  No release before the program ends exposes a held fix's code to users of
  a newer release.

  **Answered by Jeff, 2026-11-22: (a), as recommended.** Fixes are held
  unless he releases them.
- **a3. Held items and the limits.** Flagged: sets FLOW-05's counting
  sentences.
  - (a) A hold names its resume point (`resumes`); a held item holds no
    limit, does not wait on the human, and is listed once under **Held**.
  - (b) No code: Jeff raises `in_progress` and `waiting_on_human` by hand.
  - (c) Close the held items for re-intake, as componentization a8 did.

  Recommended: (a). About 60 lines of Go, and it removes the daily
  restating of decided holds.

  **Answered by Jeff, 2026-11-22: (a), as recommended.** A hold names its
  resume point and counts toward no limit.
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

  **Answered by Jeff, 2026-11-22: (a), as recommended.** The limits are
  unchanged; a program item declares its areas by the end of its Design.
- **a5. Process items during the program: who reviews them, against what
  bar.** Flagged: Jeff's three kinds do not name process items, which
  change the rules the loop runs under and are his today (LOOP-09); it sets
  FLOW-20's Loop work bullet, in his words, and how FLOW-20 reads LOOP-09.
  - (a) Every process item's design review stays Jeff's, as component
    design (kind 2): its rules decide what the loop may do in his place,
    so they are the loop component's contract and authority. It carries a
    **Program payoff** section, judged at that review; one that neither
    removes interactions that decide nothing nor saves more program time
    than it costs is held or closed.
  - (b) Only a process item that widens the loop's own authority, or
    changes a rule about security, merging, pushing or tags, goes to Jeff;
    any other passes a *process test* like the umbrella test (payoff shown,
    no such rule touched, an independent session agrees) and is recorded.
    Its merge to `main` stays his (kind 3). Amends LOOP-09 and FLOW-14
    besides, and adds a test to the machinery.
  - (c) None go to him for review: every process item passes the process
    test; only its merge to `main` stays his.

  Recommended: (a). During the program the payoff bar keeps process items
  few, and each still ends at his merge to `main` (kind 3), so (b) saves a
  review but not the touch, and has the loop judge which of its own rule
  changes widen its authority; (c) would let the loop rewrite this
  design's gates without him. (b) is the step to take later if his process
  reviews turn out to be mostly wording. Judging the payoff at intake
  only, or having `wf check` require the section, were variants of (a)
  the earlier draft offered; the review makes that check anyway.

  **Answered by Jeff, 2026-11-22: (b), not the recommendation.** Only a
  process item that widens the loop's own authority, or that changes a
  rule about security, merging, pushing or tags, goes to him for design
  review; any other passes the process test (payoff shown, no such rule
  touched, an independent session agrees) and is recorded, and its merge
  to `main` stays his (kind 3). The design is revised: Decisions 6 and
  11, Design, *The process test* and *What reaches Jeff*, FLOW-20's Loop
  work and Human gates bullets, and new amendments to LOOP-09, FLOW-14,
  FLOW-03's process route and FLOW-19. The reserved rules are his two
  classes, read by effect: loosening either test, or removing or
  narrowing a gate of his, widens the loop's authority, so the loop
  cannot use the process test to loosen the process test (Decision 6).
  **Confirmed by Jeff at the follow-up, 2026-11-22:** the reserved rules
  are his two classes, judged by effect, as the design says.
- **a6. CLI flags, subcommands, config keys, environment variables and
  output forms.** Flagged: departs from UX-03 and ORCH-03 for program
  items.
  - (a) A program item may rename or remove one with no alias or window,
    only where the program's change needs it; a form a tagged release
    exposed is refused with an error naming its replacement (UX-04); one no
    release exposed may simply go; the reference docs change with it.
    Where tier 1's UX-03 check flags the break, the commit carries a
    `Tier1-Allow` trailer whose reason names FLOW-20 (LINT-05), as Moves
    does.
  - (b) As (a), but tier 1 compares surfaces against the latest release
    tag instead of taking a trailer.
  - (c) Only forms no release exposed may break.
  - (d) UX-03 as written.

  Recommended: (a). "Even if it is temporarily incompatible or breaking",
  with "mostly similar" kept by the need test. A removed config key is
  silently ignored today (`cmd/warden/config.go`), so the item that removes
  a released key adds that error. The trailer costs an agent a line, not
  Jeff an interaction; (b) is a tier-1 change no item needs yet.

  **Answered by Jeff, 2026-11-22: (a), as recommended.** A break carries a
  `Tier1-Allow` trailer citing FLOW-20.
- **a7. The ledger.** Flagged: sets which ledger rules FLOW-20 keeps.
  - (a) `specs/build-ledger/v1.md` and its signatures change only
    additively (RELAY-05); the reader reads every ledger a release wrote; a
    written schema `$id` never changes.
  - (b) A v2 allowed per item on Jeff's approval.
  - (c) Frozen until the program ends.

  Recommended: (a). It is his "mostly similar specs" made exact.

  **Answered by Jeff, 2026-11-22, in his words:** "Additive only to start.
  If there needs to be a v2, then it is finalized only at the end of the
  entirety of the program. Or in other words, there is allowed to be a
  development/working copy that eventually is 'released' as v2 if there
  are reasonable updates/changes to be made". That is (a) with a v2
  working draft allowed beside it: v1 changes only additively during the
  program; a v2 may be developed as a working draft on the program trunk,
  marked unreleased, and is finalized and released as v2 only at the
  program's landing; the reader still reads every ledger a release wrote.
  The design is revised: Decisions 7 and 12, Design, *The ledger's working
  draft*, and FLOW-20's Interfaces bullet.
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

  **Answered by Jeff, 2026-11-22: (a), as recommended.** Named temporary
  gaps on a program trunk.
- **a9. One item or two.**
  - (a) One item: holds, interfaces, program trunk, human gates and
    pre-releases, reviewed in one sitting.
  - (b) Split: this draft's holds and interfaces, then a second process
    item for the program trunk, gates and tags.

  Recommended: (a). Jeff asked for the initial gate to be what the
  scheduled runs need and nothing after it; two items mean two merges and
  two state steps, and the program trunk is what makes the interface
  breaks safe to merge. The sitting takes the 19 rule changes in
  question-call groups (FLOW-19); there were 17 at `8cb13d8`, then the
  answers dropped FLOW-11 and added LOOP-09 and FLOW-14, and a later
  check added LINT-04.

  **Answered by Jeff, 2026-11-22: (a), as recommended.** One item.
- **a10. The program trunk's shape.** Flagged: sets LOOP-01, PROFILE-01's
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

  **Answered by Jeff, 2026-11-22: (a), as recommended.**
  `program/componentization` as `program_trunk` per repository.
- **a11. Who merges into a program trunk, and how.** Flagged: amends
  FLOW-08, FLOW-03, FLOW-04 and LINT-03.
  - (a) The loop, at `f7-merge`, under the nine-point merge gate; squash per
    item pinned to the head; `main` merged in by the loop with a merge
    commit (Decision 14).
  - (b) As (a), with merge commits for every item.
  - (c) Jeff merges each PR into the program trunk.

  Recommended: (a). His "no publish step should wait on me". Squash keeps
  one commit per item, which makes the landing PR readable, and wf's merge
  proof already accepts it; the merge commit for `main` keeps `main` an
  ancestor so the landing PR has no conflicts. A merge to `main`, and a
  program item's change to CI or release configuration, stay his as
  privileged work (kind 3).

  **Answered by Jeff, 2026-11-22: (b), not the recommendation.** The loop
  merges program PRs into the program trunk at `f7-merge`, under the
  merge gate, with a merge commit for every item, not a squash. The
  design is revised: Decisions 9 and 14, Design, *The merge gate and the
  loop merge*, Defaults, FLOW-08's and FLOW-20's wording, the program
  trunks' ruleset, and the Validation plan. The landing PR stays readable
  by its first-parent history, one merge commit per item, and wf's merge
  proof already accepts a merge commit (Prior art). The landing merge to
  `main` stays Jeff's squash, as his statement of 2026-11-21 has it.
- **a12. Tier-2 MUST findings on a program PR.** Flagged: amends LINT-03's
  sign-off; touches GLOBAL-06.
  - (a) The loop fixes a MUST in one fix round; one still open is deferred
    and recorded, and signed off at the landing merge, except one under
    GLOBAL-06, a SEC rule, RELAY-01 to RELAY-03, RELAY-05 or FLOW-20's
    never-break list, which is never deferred: the item returns to Design
    for a plan that conforms, recorded, and reaches Jeff only to dismiss
    the finding or to change the posture or contract it protects (kind 2).
    The loop never dismisses.
  - (b) As (a), but a protected finding still open after the fix round
    waits on Jeff at `f7-merge` at once, as the earlier draft had it.
  - (c) Every open MUST waits on Jeff at `f7-merge`.

  Recommended: (a). A protected finding still open says the change does
  not yet hold a component's posture or contract; making it hold is the
  strategy to implement, which the loop owns, and only accepting a
  different posture or contract is his. (b) is the safe side if he wants
  to see every protected finding before the loop re-plans.

  **Answered by Jeff, 2026-11-22: (a), as recommended.** One fix round;
  deferred and signed off at the landing; a protected finding is never
  deferred and goes back to Design.
- **a13. Program design reviews and questions.** Flagged: amends FLOW-07
  (which questions block), FLOW-19 (a third review outcome and the flag
  rule) and FLOW-20's Human gates bullet.
  - (a) A program feature's review is taken by the umbrella test U1 to U7
    (Design) and recorded for his later reading; a failed component-design
    point (U1 to U5) sends it to him, a failed loop point (U6, U7 on
    anything else) back to Design. On a program item a question blocks, or
    is flagged, only when it is component design or needs setup or
    privileged work; any other is assumed on Design's recommendation and
    confirmed by the umbrella session, recorded.
  - (b) As (a), but every blocking question stays his (FLOW-07 as
    written), and a question that touches a seam always flags, so most
    program reviews still reach him.
  - (c) No: every non-trivial program design keeps his review.

  Recommended: (a). The program's phases, seams and order are his already
  approved design; a design that implements a phase row within it, and
  that an independent session agrees does, is what he called "easily
  understood by my intents and design thus far", and a question on how to
  implement it is the "strategy to implement" he asked not to be brought
  in on.

  **Answered by Jeff, 2026-11-22: (a), as recommended.** The umbrella test
  U1 to U7 takes program reviews.
- **a14. Security posture under the umbrella test.** Flagged: touches a
  security boundary. Under every option, a change to a component's
  security posture is component design (kind 2) and stays his.
  - (a) A design whose only trip is GLOBAL-06's "any change to the relay
    itself", with no boundary posture change, passes U4 with its security
    note read by the independent session; Jeff acknowledges the note at the
    landing merge. A posture change (a new listener or egress path, key, CA
    or trust-store handling, the SSRF filter, netns or iptables, the
    socketpair topology) goes to him, at its design and at `f7-merge`.
    TEST-08 runs at each `-rc` and the landing.
  - (b) Every program design with `security_boundary` other than `none` is
    reviewed by Jeff.

  Recommended: (a). Every M-track item and S2, S4 trip the relay clause;
  (b) puts the whole relay track back in his queue, design by design,
  though most of it changes no posture.

  **Answered by Jeff, 2026-11-22: (a), as recommended.** Only posture
  changes reach him.
- **a15. A program item that does not converge.** Flagged: amends FLOW-06,
  FLOW-11 and FLOW-13, and removes their escalations for program items.
  - (a) No escalation: returns to Design do not escalate (the budget
    bounds them); the first runaway budget raises the estimate to the
    spend so far, the second returns the item to Design for a new plan by
    a fresh session, and after that it is held until no other program item
    has a ready step; a CI failure Publish cannot repair returns it to
    Design with the failure as the finding, unless only privileged work
    can fix it (kind 3); a sync conflict the loop cannot resolve is filed
    as a program item (Decision 14). Each recorded, and listed first under
    **Decided for you**.
  - (b) The earlier draft: escalate after the fourth return and at the
    second runaway; repair exhaustion and an unresolved sync conflict
    escalate.
  - (c) FLOW-06, FLOW-11 and FLOW-13 as written.

  Recommended: (a). None of these is a design question: an item that does
  not converge needs a different plan, which is the strategy he asked to
  stay out of. FLOW-13's run budget still caps each run's spend, and the
  hold puts a non-converging item behind every other program step, so it
  costs only idle time; the record puts it in front of him without
  blocking on him. (b) is the safe side if he wants to hear of a stuck
  item at once.

  **Answered by Jeff, 2026-11-22: (b), not the recommendation.** A
  program item still escalates, but later: after its fourth return to
  Design and at its second runaway budget; repair exhaustion and an
  unresolved sync conflict escalate. The design is revised: Decisions 11
  and 14, Design, *What reaches Jeff*, the Skills and agents, FLOW-06's and
  FLOW-13's wording, FLOW-20's Human gates bullet, and the Validation
  plan; FLOW-11 is no longer amended, since it already escalates.
  **Confirmed by Jeff at the follow-up, 2026-11-22:** the fifth return
  escalates, meaning more than four returns, as the design says.
- **a16. Filing and area maps without Jeff.** Flagged: amends LOOP-07.2,
  FLOW-05's area-map sentence and PROFILE-01's Areas row.
  - (a) The loop files a phase row's items once the Order's gate before its
    wave is met, reconciliation items that adopt a pre-release on a program
    trunk, and the relations between items their designs name; it makes
    the area-map changes a reviewed or umbrella-passed program design names
    (a new repository's map, an area a move creates or empties, a split, a
    mode) or that bring `reaches` in line with `area-edges.sh`; each
    recorded.
  - (b) As (a), but only additions to an area map; changing a mode or
    `reaches`, removing an area, and relating or narrowing items stay his.
  - (c) Jeff at intake and in attended sessions, as today.

  Recommended: (a). The phase rows and their order are already his; a
  pre-release adoption is mechanical; an area map is the loop's
  scheduling, derived from the code's edges and the designs, not a
  component's design, and keeping its changes his would block the moves
  that create and empty areas throughout the program.

  **Answered by Jeff, 2026-11-22: (a), as recommended.** The loop files
  phase items and changes area maps, recorded.
- **a17. Pre-release tags and pins.** Flagged: amends FLOW-17, LOOP-07.5
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

  **Answered by Jeff, 2026-11-22: (a), as recommended.** The loop tags
  `-dev.N` and `-rc.N` on program trunks.
- **a18. Program repositories.** Flagged: departs from componentization
  a11 (Jeff's word in each session).
  - (a) Jeff creates every repository on the umbrella's list (the relay,
    the loop, one repository per driver) in the one-time setup, private,
    each with an initial commit, its `config.yaml` entry, its program trunk,
    the organization rulesets, automatic deletion of head branches, the
    loop's App, and its `hosts.yaml` line for a machine that already serves
    its host. A repository not on the list is a new component, proposed in
    a program design and reviewed by him (kind 2). Going public stays his,
    one action per repository (kind 3).
  - (b) Each is created when its phase starts, by him or an attended
    session, on the standing word of his approval of the list.
  - (c) Each on his word when its phase starts, as a11 says.

  Recommended: (a). His "initial bulk-setup of the necessary
  repositories/infrastructure": one sitting instead of one per phase, and
  an empty private repository costs nothing until its phase starts.

  **Answered by Jeff, 2026-11-22: (a), as recommended.** He bulk-creates
  the program repositories in one setup sitting. **Changed by Jeff at the
  follow-up, 2026-11-22:** in that sitting he also commits each new
  program repository's CI and flow18 workflows to its `main`, and adds
  their job names to its ruleset's required checks, before its program
  trunk is cut, so nothing waits on him when the repository's phase
  starts. The design is revised: Decisions 11 and 16, Design, *What
  reaches Jeff*, Defaults and overrides, the Program payoff, the
  Validation plan, and the Implementation plan's state step and setup
  list. No rule's proposed wording changes.

## Design

### The rule

FLOW-20 is rewritten (full wording under **Rule changes**): **Hold**
replaces **Freeze**; **Areas**, **Loop work** and **Interfaces** as in the
earlier draft (Interfaces' gap sentence now names the program trunk);
new bullets, **Program trunk**, **Human gates**, **Records** and
**Pre-releases**; **Moves** and **Overview** unchanged. **Loop work** gains
the process test (a5 (b)); **Interfaces** gains the ledger's working draft
(a7). **Human gates** states Jeff's three kinds as the only steps that wait
on him, besides the escalations he kept (a15 (b)), and says what the loop
does in place of each gate it removes. The closing list of what stays as
written now names FLOW-11 and the LOOP rules other than LOOP-07 and
LOOP-09 as applying as written, and FLOW-06, FLOW-07, FLOW-08, FLOW-13,
FLOW-14, FLOW-19, LOOP-07 and LOOP-09 "as this rule reads them". The other rules gain one
clause each that points to FLOW-20, so an agent reading one alone does not
act against it: FLOW-06 and FLOW-13 for an item that does not converge,
FLOW-07 and FLOW-19 for questions and reviews, and LOOP-09, FLOW-14 and
FLOW-03's process route for the process test.

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

**The trunk is pinned.** The inputs to that choice change: areas are
declared by the end of Design (a4), after Branch setup has forked the
branch, and a later design can redeclare them. So the item records its
trunk per repository (`trunk` on its work entry) the first time it is set,
at `f0-branch-setup` or at Design's `wf declare`, whichever comes first,
and `TrunkFor` returns the recorded value from then on. `Validate` and
`wf declare` refuse an area or `program` change that would choose a
different trunk once one is recorded; the item goes back to Design to be
split instead (FLOW-06), so a branch forked from `main` is never checked
against the program trunk, and a branch that has merged the program trunk
never opens its PR to `main`.

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

- **`check.MergeReady(env, f, repo)`**, per repository, checked against
  `TrunkFor(repo, f)`, run as `wf check -feature F -step f7-merge
  [-repository R]` (every work entry when `-repository` is left out),
  checks Decision 9's points in order: `gh pr view --json
  baseRefName,headRefOid` for 1 and 2; `gh api
  repos/<o>/<r>/commits/<sha>/check-runs` and the program trunk's
  `rules/branches/<t>` for 3, whose required list is read from that
  ruleset and never assumed; `mainMerged` against the program trunk for 4;
  the review report for 5 (`check/review.go`, reading deferred findings
  from the records); the item's steps and questions for 6; the security
  record for 7; `mergedFirst` for 8; `ruleClearance` for 9. Each failure
  names its point and its repository.
- **`queue/next.go:32-43, 190-193`**: a merge step whose item's
  `TrunkFor` is a program trunk in every repository it has a branch in is
  `f7-merge`, `cloud`, offered once `MergeReady` passes for each of them;
  an `f6-merged-<n>` (`seedMergedFirst`, `queue/phases.go:489-502`) whose
  work entry's trunk is a program trunk is likewise a `cloud` merge step,
  offered once `MergeReady` passes for that repository, and the agent
  merges that repository's PR at it. Every other merge step and
  `f6-merged-<n>` is still never offered, and `mergedFirstWait` keeps "the
  human's merge" for those only. `MayComplete` is unchanged.
  **`queue/limits.go:379-409`**: a PR at `f7-merge`, and an
  `f6-merged-<n>` on a program trunk, do not wait on the human
  (`waitsOnHuman`'s `|| isMergedFirst(st)` skips them); one held at point
  7 on a posture change, or at point 5 on a protected finding the loop has
  asked him to dismiss, does. A protected finding otherwise returns the
  item to Design (Decision 10), which `wf-review` seeds with the finding
  as for any return.
- **`check/check.go:148, 502`**: the label becomes "merged (FLOW-08)"; the
  proof (`:514-515`, `refMerged` at `:591-593`) is unchanged: a merge
  commit leaves the PR's head an ancestor of the program trunk, which is
  its first test, and `mergeCommit` records the `Merge pull request #N`
  commit its grep finds. Its squash branches stay for `main`'s merges and
  are never reached on a program trunk.
- **The agent** (`wf-publish`, step `f7-merge`, or an `f6-merged-<n>` on a
  program trunk): for each work entry the step covers (at `f7-merge`, each
  one not yet merged, in turn; at `f6-merged-<n>`, that entry's), run the
  check for its repository; on a pass, `gh pr merge <n> --merge
  --match-head-commit <pr.head> -R <repo>`, then `git fetch origin
  <program trunk> <branch>` in that repository's checkout, so the merge
  proof reads the merged `origin/<program trunk>` and not the ref from
  before the merge; then `dist/wf done` and `dist/wf record -kind merge`.
  On a refusal from GitHub, run the check again and stop. The program
  trunk's ruleset allows merge commits only, so a squash or rebase by
  mistake is refused by GitHub too.
- **Program sync** (Decision 14): a sync has no queue item, so it has its
  own check. `wf sync-program -repository R` prints whether `main` is
  ahead of the program trunk and, when it is, the branch
  `sync/program-<main sha>`: the agent cuts it from the program trunk,
  merges `origin/main` into it (a merge commit, conflict-only fixes), runs
  the fast gate and pushes it, so the PR's head contains both the program
  trunk and `main` and the ruleset's up-to-date requirement holds. A PR
  whose head is `main`'s own commit never would. `check.SyncReady(env,
  repo, pr)`, run as `wf check -sync -repository R -pr N`, passes when the
  PR's base is the program trunk; its head is the one `wf sync-program`
  recorded for the branch and the remote's; every check on it passed or
  was skipped and the ruleset's required ones passed; and the head
  contains both `origin/main` and the program trunk's head. The tick
  dispatches `wf-publish` with step `sync`, which merges with `--merge
  --match-head-commit` once `SyncReady` passes, and, when the merge had
  conflicts, once a tier-2 review of the resolution diff, for that head,
  has no open MUST finding; the resolution is recorded. A conflict it
  cannot resolve escalates (a15 (b)): close the sync PR if one opened,
  `wf new -program -escalate "<conflict>"` the item
  `program-sync-<main sha>`, whose first step is a `human` escalation
  carrying the conflicting paths and both sides' commits, and record both;
  the next sync waits until that item has merged.

### The umbrella test

Design writes `design_review: umbrella` for a program feature it judges to
pass, and records the points. wf seeds `f2-umbrella`, a cloud step that
`wf-review` takes in a fresh session: it reads the design against
`componentization.md` and the records, and either writes the line
`r<N>, <date>: umbrella test passed at <sha> (FLOW-20)` with each point's
evidence, or writes the failing point. A failed component-design point (U1
to U5) sets `design_review: required`, which seeds Jeff's review as today.
A failed loop point (U6, or U7 on anything but U1 to U5) returns the item
to Design with the session's finding (FLOW-06), recorded, and the next
design takes the test again.

The component-design points (kind 2 when one fails):

- **U1.** Program item: `related` lists `componentization.md`, and the
  design implements the components, seams and boundaries the program's
  design names for one phase row (P, S, K or M), or a filed split,
  follow-up or re-sequencing of them, within that row's Work and Done
  when. A design that changes only the order, the split or the method is
  within U1; one that adds a component, moves a boundary or a component's
  ownership, or adds a seam the program does not name, is not.
- **U2.** No blocking question and no flagged assumed question (FLOW-07,
  FLOW-19, with the program's clauses: only component design, setup or
  privileged work blocks or flags). Each recommendation cites the umbrella
  decision or recorded answer of Jeff's it follows, or says it settles a
  question of strategy.
- **U3.** `rule_changes` is empty (a component invariant is its contract
  and needs Jeff's clearance, FLOW-14), and the design's `areas` reach no
  rule or machinery path of the profile (a loop rule is a process item's,
  LOOP-09, a5).
- **U4.** `security_boundary` is `none`, or the only trip is GLOBAL-06's
  relay clause and the Security note states that no boundary's posture
  changes (a14).
- **U5.** Each interface break is one FLOW-20's Interfaces bullet allows.
  A change to the ledger's working draft is one when the reviewed design
  that opened the draft, or a later decision of Jeff's, names it (Design,
  *The ledger's working draft*); opening the draft is not.

The loop points (decided by the loop when one fails):

- **U6.** `repos` and `areas` declared. Where they are fewer than intake
  recorded, or intake recorded none, the session checks them against the
  area map and `tools/tier1/area-edges.sh` and records the check; FLOW-19's
  "no fewer than intake" otherwise sends a narrowing to Jeff, and would
  send every program item intake left undeclared.
- **U7.** The independent session agrees, point by point.

`check/review.go`'s `approvals` accepts `umbrella test passed at <sha>` as
an approval, so the Plan gate's delta check works as for Jeff's: a later
change to an impact field, `repos`, `areas` or `rule_changes` needs a new
pass. Each pass is recorded and listed in the digest, as a skipped review is
today.

Design also settles a question from the umbrella or a recorded answer
before it raises one, citing the source, and records "settled by <ref>, not
asked". A question of strategy it cannot settle that way it asks as
assumed, with its recommendation, and the umbrella session confirms or
returns it.

### The process test

Jeff's answer to a5 (b): a process item filed while FLOW-20 stands goes to
his design review only when it touches a reserved rule; any other passes
the process test, the umbrella test's counterpart for loop work.

**The reserved rules.** These are the two classes of Jeff's answer, and
no more. FLOW-20's Loop work bullet holds the rule's one definition;
FLOW-14, LOOP-09 and FLOW-03 cite it. A process item touches a reserved
rule when its change, read by its effect and not by the files it names:

- widens the loop's own authority: after it, the loop may take without
  Jeff a step, merge, push, tag, filing or decision it could not take
  before (a step of his becomes the loop's); a gate of his is removed or
  narrowed; the umbrella test or the process test is loosened; a check
  that stops the loop is relaxed or removed; or a limit, budget or
  permission is raised (`config.yaml`'s limits, budgets and
  `runaway_factor`, the allow lists of `.claude/settings*.json`);
- or changes a rule about security (GLOBAL-06, the SEC rules, RELAY-01 to
  RELAY-03, RELAY-05, RELAY-08, the BOOT rules), merging (FLOW-08,
  LINT-03, this design's merge gate), pushing (FLOW-16, FLOW-17) or tags
  and releases (FLOW-17's tag clause, LOOP-07.5, FLOW-20's Pre-releases
  bullet, the release workflows).

Read by effect, the first class keeps the loop from using the process test
to loosen either test, or to take back a gate this design leaves with
Jeff, whichever rule the change edits. A change that widens nothing and
touches no rule of the second class is not reserved, even when it edits
the loop's configuration or a rule on who reviews or answers what: lowering
a limit, or making more questions blocking, takes the process test.

**The points.**

- **P1.** The **Program payoff** section shows, with figures and how they
  were reached, FLOW-20's bar: interactions removed that decide nothing,
  per week, or program time saved over the item's own cost.
- **P2.** No reserved rule is touched. Design lists every rule, machinery
  path and configuration key the change reaches, and the session checks
  each against the list above. A doubt is a failure: P2 fails closed.
- **P3.** A review session that did not write the design agrees on P1
  and P2, point by point.

**The steps.** The gate follows the feature round's pattern, where the
design review sits in the phase after Design beside the step that needs
it. While any repository entry has `program_trunk`, the `process`
template (`queue/phases.go:75-84`) seeds `x1-design` as `cloud`, and
Design records `design_review: process-test` or `required`. Phase 2 then
holds `{x2-process-test cloud | x2-design-review human}` with
`x2-implement` needing it: `seeded()` (`queue/phases.go:97`) chooses
between the two by `f.DesignReview` when phase 2 is entered, after
`x1-design` has recorded it, so the choice is never made at seed time
before Design has run. `IsDesignReview` (`:209-213`) recognizes
`x2-design-review`, and a new `IsProcessTest` recognizes
`x2-process-test`, so `finish()` runs `check.Review` on either. Once
FLOW-20 is retired, phase 1 is `x1-design`, `human`, as today, and phase
2 is `x2-implement` alone.

- **The branch.** A process item has no Branch setup phase, and today
  the human creates `feat/<slug>` at intake (workflow-intake item 6). A
  cloud `wf-design` at `x1-design` creates `feat/<slug>` from `origin/main`
  and pushes it when it does not exist yet, before writing the design.
- **Holds.** A cloud `x1-design` claim starts the item, so a process item
  in Design or at `x2-process-test` would hold the workers its `holds`
  names. `State.Hold` (`queue/next.go`) skips a process item still before
  Implement and an item on hold, so neither holds a worker.
- **Jeff's attended path.** Claims are affinity-exact (`queue.Satisfies`,
  `cmd/wf/main.go:599`), so Jeff's attended session takes a cloud
  `x1-design` as a cloud worker, through `/workflow-tick attended
  <feature>`, not as `human/jeff`; workflow-human claims and completes
  only `x2-design-review`.

The `x2-process-test` session writes `r<N>, <date>: process test passed at
<sha> (FLOW-20)` with each point's evidence, or the failing point:

- a failed P2 sets `design_review: required`, which seeds
  `x2-design-review` in place of `x2-process-test`, Jeff's review (kind
  2), recorded;
- a failed P1 puts the item on hold, `resumes: program-end`, recorded,
  with `wf hold -feature F -why "<P1's finding>" -resumes program-end`:
  `PutOnHold` (`queue/queue.go:837-855`) refuses an item that is not
  queued or already has a question, so it is extended for an in-progress
  item: it takes the next free `q<N>` and makes the current phase's
  pending gate (here `x2-implement`) need the answer step. Jeff may
  release it whenever he says, and P1 is judged again then;
- a disagreement on P1 alone is a failed P1; any disagreement touching P2
  is a failed P2.

`check/review.go`'s `approvals` accepts `process test passed at <sha>` as
the approval phase 2's gate asks for; the Plan gate's delta check on
`rule_changes`, `repos` and `areas` runs at `x2-implement`'s start against
that sha, so a later change to those needs a new pass. The state step, if
the design proposes one, is recorded by Jeff at his merge of the item to
`main` and taken in the same sitting (FLOW-03: "at the design review or
later"), so it stays his. Once FLOW-20 is retired the template seeds as
today, and `process-test` is refused by the validators.

### The ledger's working draft

Jeff's answer to a7: "Additive only to start. If there needs to be a v2,
then it is finalized only at the end of the entirety of the program."
`specs/README.md` keeps one document per major version, the new one beside
the old (RELAY-05), and that layout decides where the draft lives.

- **Where.** Beside v1, as the next major version's file, in the
  directory that holds the ledger format at the time:
  `specs/build-ledger/v2.md` in this repository, or the specifications
  repository's `build-ledger/` once the format moves there at M6
  (componentization's seam 4), on that repository's program trunk only.
  A metadata schema the draft adds or changes takes the same layout:
  `metadata-<name>/v<N>.md` and `v<N>.schema.json` beside its document,
  with the `$id` that mirrors its path (`specs/README.md`'s `$id`
  policy).
- **How it is marked unreleased.** Each draft document opens with a
  status block, `> **Working draft, unreleased (FLOW-20).** Not a
  released version of the ledger format. No released or pre-released
  build writes it; it may change until the program lands, when it is
  finalized as v<N>.`, and `specs/README.md`'s Documents list names it
  "v<N> (working draft, unreleased)". Its book mirror page is as for any
  document. The header version byte it names (`0x02`) is the final one;
  the draft is not a version of the format until the landing, so RELAY-05's
  one document per major version holds.
- **Who writes it.** Only tests and conformance suites. No build tagged
  from a program trunk, `-dev` or `-rc`, writes a draft ledger by default
  or by any option it documents, so every ledger a release wrote, a
  pre-release's included, is v1 and the reader keeps reading it; a
  draft `$id` is written by no release, so it may still change.
- **Who decides.** v1 changes only additively throughout the program.
  The design that opens the draft changes the ledger's contract, which is
  component design (kind 2): it is reviewed by Jeff, and says why v1
  cannot grow additively. A later change to the draft passes the umbrella
  test when that reviewed design, or a later decision of his, names it
  (U5); a change to what the format guarantees (verifiable, tamper-evident,
  complete) is a posture change and goes to him (kind 2).
- **Finalizing.** A program item, named by `program-land`'s design to
  merge first on that repository's program trunk, removes the status
  blocks, writes the final index row, and makes v2 what the writer
  writes, keeping v1's reader. Jeff's landing merge signs it off, and the
  full release tagged from `main` after it releases v2.
- **Enforcement.** `specs/specs_test.go` gains `checkDrafts`: a document
  with the status block is listed in the index as unreleased, and fails
  when `GITHUB_BASE_REF` (on a pull request) or `GITHUB_REF_NAME` (on a
  push) is `main`, so a draft reaches `main` only finalized, by the
  landing. `specs/README.md`'s Layout gains the line for a working draft.

### What reaches Jeff

Every gate the earlier draft sent to Jeff during the program, with its
kind, or *escalation* for one Jeff kept under a15 (b); a gate of neither
is decided by the loop, and the last column says what the loop does
instead. Every "decided by the loop" entry is recorded (Decision 13) and
callable back.

| Gate | Kind | Instead |
|---|---|---|
| This item's review and merge, its state step, program trunks, rulesets, App permissions, nightly prompt, `specs-prerelease-tags` (Decision 16) | 1 setup | |
| Creating the program repositories on the umbrella's list, each with its CI and flow18 workflows on `main` and their job names required, before its program trunk is cut (a18) | 1 setup | |
| A process item's design review when it touches a reserved rule: widens the loop's authority, read by effect, or changes a rule on security, merging, pushing or tags (a5 (b); the cases in FLOW-20's Loop work bullet) | 2 design | |
| Any other process item's design review (a5 (b)) | none | the process test, P1 to P3, recorded; a failed P2 goes to him, a failed P1 holds the item |
| A process item's state step (FLOW-03) | 3 privileged | recorded and taken by him with his merge of it to `main` |
| A program design failing U1 to U5: a new component, a boundary moved, a seam the program does not name, a rule change, a posture change, a break FLOW-20 does not allow | 2 design | |
| A blocking or flagged question on component design (a13) | 2 design | |
| A blocking question of strategy | none | assumed on Design's recommendation, confirmed by the umbrella session |
| A departure from a recorded decision of his on strategy | none | made, listed first under **Decided for you** |
| A program design failing U6, or U7 on anything but U1 to U5 | none | back to Design with the finding |
| A program PR's security note with a posture change, at `f7-merge` (a14) | 2 design | |
| A security note whose only trip is the relay clause (a14) | none | read by the independent session; acknowledged by the landing merge |
| A tier-2 MUST the loop could not fix in one round, not protected (a12) | none | deferred; signed off by the landing merge |
| A protected MUST finding (a12) | none, then 2 | back to Design to conform; to Jeff only to dismiss it or to change the posture or contract it protects |
| A program item's third or fourth return to Design (FLOW-06, a15 (b)) | none | it returns to Design again, recorded |
| A program item's fifth return to Design: more than four (FLOW-06, a15 (b)) | escalation | |
| A program item's first runaway budget (FLOW-13, a15 (b)) | none | estimate raised to its spend so far by `wf reestimate`, recorded |
| A program item's second runaway budget (FLOW-13, a15 (b)) | escalation | |
| CI repair exhausted on a program item (FLOW-11, a15 (b)) | escalation | as FLOW-11 has it for any item |
| A sync conflict the loop cannot resolve (Decision 14, a15 (b)) | escalation | the program trunk stays; `program-sync-<sha>` is filed with the escalation as its first step |
| A branch that did not delete (FLOW-16) | 3 privileged | as written; the setup's automatic deletion prevents it, and it blocks no program step |
| A process item's merge to `main` (FLOW-08) | 3 privileged | |
| The merge of a program item that changes CI or release configuration (Decision 8) | 3 privileged | a new program repository's first CI is not one: it is committed in the setup (a18) |
| A program item's merge into its program trunk (a11 (b)) | none | `f7-merge` under the merge gate, a merge commit |
| Releasing a held item; an urgent mark; the priority order (a2, Decision 5) | none | the hold stays, the order is FLOW-05's; his word overrides whenever given, and nothing waits on it |
| Filing phase items and reconciliation items; relating items (a16) | none | filed by the loop |
| An area-map change (a16) | none | made by the loop when a reviewed or umbrella-passed design names it or `area-edges.sh` shows it |
| A `hosts.yaml` change | 3 privileged | a new repository's line is part of the setup (kind 1) |
| A native step only his attended machine can take, or a machine not serving (FLOW-09) | 3 privileged | |
| A repository not on the umbrella's list (a18) | 2 design | |
| Making a repository public (a18) | 3 privileged | |
| A `-dev` or `-rc` tag (a17) | none | tagged by the loop |
| A release without a suffix | 3 privileged | |
| The `program/componentization-final` tags (Decision 12) | none | pushed by `program-land` |
| `program-land`'s design review (a5 (b): it retires FLOW-20's merge, push and tag clauses) | 2 design | |
| Opening a ledger working draft (a7) | 2 design | later changes it names pass the umbrella test; finalizing is signed off by the landing merge |
| "Every component works as intended" (Decision 12) | none | the loop's evidence, in each landing PR |
| The landing merges, with the records in their bodies (Decision 12) | 3 privileged | |
| Answering each hold at the program's end | none | `program-land`'s Close resumes each; he may close any |

**The runaway, counted.** Today a runaway is no event: `State.Runaway`
(`queue/queue.go:1425`) tests `spent >= runaway_factor * estimate`, which
makes the item wait on the human and stops unattended claims
(`unattendedHold`, `queue/next.go`); no step is seeded, and nothing raises
an estimate after `wf new`. So:

- `wf reestimate -feature F` sets `Budget.Estimate.Steps` to
  `Budget.Spent.Steps`, logs a `re-estimated (FLOW-13)` event on the item
  and appends a program record. It is cloud-allowed only for a program
  item with no earlier `re-estimated` event; otherwise it is the human's.
- `State.Runaway` counts those events: a runaway program item with none
  is not waiting on the human and is not held from unattended claims, and
  the tick runs `wf reestimate` for it before it claims anything else in
  the run; a program item with one, or any other item, is a runaway as
  today, an escalation that waits on the human.

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
- The landing order: the specifications repository's landing squash and full tags; this repository's
  pins move to them on the program trunk, then its landing and full tag;
  then the relay; then the drivers, at the kit's full tag.

### Skills and agents

- **`workflow-tick`**:
  - Never (`SKILL.md:20-21`): "Approve or enable auto-merge on any PR. Merge
    one only at `f7-merge` or the program sync, into a program trunk, with
    `--merge --match-head-commit`, after `wf check` passes (FLOW-08,
    LINT-03); never into a trunk, never a squash or rebase, never with
    `--admin`, and never dismiss a finding.
    Push `main` or a program trunk, force-push, rewrite pushed history, or
    delete a remote branch or a tag."
  - Step 2 (`:101-131`): record each trunk and each program trunk; refresh
    against each item's own trunk; run the program sync when `main` is
    ahead.
  - After dispatch: `wf tag-next` and its push; `wf record` for each
    decision the run made. At `f7-merge` and an `f6-merged-<n>` on a
    program trunk, `git fetch origin <program trunk> <branch>` between
    `gh pr merge` and `dist/wf done`, so the merge proof reads the merged
    trunk.
  - Digest: **Held** (one line), **Decided for you** (the run's records,
    a return past the second, a re-estimate and a departure from his
    decisions first), and Blocked on Jeff without held items or
    loop-merged PRs, each step there named with its kind (setup, design,
    privileged) or as an escalation.
  - A program item's first runaway raises its estimate with `wf
    reestimate`, recorded; its second runaway, a fifth return, repair
    exhaustion and an unresolved sync conflict escalate (a15 (b)), each
    named as an escalation.
  - `x2-process-test` is dispatched to `wf-review` in a fresh session.
- **`workflow-human`**: step 2 shows held items apart; items 0 and 3
  claim and complete a process item's `x2-design-review`, not
  `x1-design`, which is `cloud` while FLOW-20 stands (Jeff takes it, when
  he wants to, through `/workflow-tick attended <feature>`), and skip a
  process item whose `design_review` is `process-test`; step 3 checks a
  process item's Program payoff, a program design's interface breaks and
  areas, and lists the umbrella and process-test passes since the last
  sitting for reading; a process item at its merge shows its process-test
  line and its proposed state step, which Jeff records there;
  item 5 (`SKILL.md:145-150`) does not offer a PR to a program trunk ("the
  loop merges it"), lists those held at `f7-merge` on a posture change or
  a protected finding the loop asks him to dismiss, and the records since
  the last sitting; a new last item walks the landing PRs once
  `program-land` resumes.
- **`workflow-intake`**: the hold wording of the earlier draft (`-hold
  "<why>" -resumes "<when>"`), `-program` for a program item, an
  unattended path for a16's filings, recorded, and, for a process item
  filed while FLOW-20 stands, a note that its Design is `cloud`, that
  `wf-design` creates its branch (item 6's "the human creates
  `feat/<slug>`" holds only once FLOW-20 is retired), and that it takes
  the process test unless it touches a reserved rule.
- **`workflow-native`** (`SKILL.md:3, 36, 130, 206`): tests each trunk and
  program trunk, then the waiting branches; never pushes `main` or a
  program trunk, and merges nothing; `settings.native.json:89`'s deny of
  `gh pr merge` stays.
- **`wf-publish`** (`.claude/agents/wf-publish.md:16, 19, 41, 66, 91`):
  every `origin/main` and `main` becomes "the item's trunk (`dist/wf
  trunk`)"; new steps `f7-merge` (each work entry's PR in turn) and
  `sync`, and an `f6-merged-<n>` on a program trunk, all merging with
  `--merge` and fetching the program trunk and branch before `dist/wf
  done` (Design, *The merge gate and the loop merge*); the sync builds
  its branch by merging `origin/main` into the program trunk and checks
  with `wf check -sync`; the Never line as in workflow-tick; repair exhaustion (`:58-59`)
  escalates as written, for a program item too (a15 (b)); a sync conflict
  it cannot resolve files `program-sync-<sha>` with its escalation.
- **`wf-review`**: the `f2-umbrella` step, U6's area check included; the
  `x2-process-test` step, P1 to P3, with the reserved-rule list it checks
  P2 against and its fail-closed rule; each MUST and SHOULD finding
  recorded with the head it was found on; a deferral with its reason; a
  protected finding still open sends the item back to Design.
- **`wf-design`**: the umbrella self-check and `design_review: umbrella`;
  for a process item, creating and pushing `feat/<slug>` from
  `origin/main` when it does not exist, the Program payoff section, the
  list of rules,
  machinery paths and configuration keys it reaches, and `design_review:
  process-test` or `required` by the reserved-rule list; settle a question
  from the umbrella before raising it; on a program item, ask a question
  of strategy as assumed, never blocking, and flag only component design;
  a design that changes the ledger format opens or changes its working
  draft as Design, *The ledger's working draft*, says.

## Defaults and overrides

- **A hold's resume point.** `program-end` unless Jeff names another.
  `-resumes` is required with `-hold`.
- **A non-program item during the program.** Held; only Jeff's word
  releases it, and nothing waits on that word. At the program's end the
  loop resumes it.
- **An item's trunk.** `main`; the program trunk only for a program item
  that is not a process item and touches no machinery, `.github/` or
  `.goreleaser.yaml` path, in a repository
  whose entry has `program_trunk`. No fallback: an entry without
  `program_trunk` gives `main`, and wf names it in `wf trunk`'s output.
- **The merge method.** A merge commit for every loop merge, each item's
  and the program sync's (a11 (b)); the program trunk's ruleset allows no
  other. No loop merge runs without the gate. The landing merge into
  `main` is Jeff's squash.
- **A MUST finding the loop could not fix.** Deferred and recorded, except
  the protected ones, which send the item back to Design; Jeff only to
  dismiss one or to change what it protects.
- **Design review of a program feature.** The umbrella test; on a failed
  component-design point (U1 to U5), Jeff; on a failed loop point, back to
  Design.
- **A question on a program item.** Assumed, on Design's recommendation,
  unless it is component design or needs setup or privileged work.
- **A program item that does not converge.** Escalates later (a15 (b)):
  its third and fourth returns to Design loop again, its fifth escalates;
  its first runaway raises the estimate, its second escalates; repair
  exhaustion and an unresolved sync conflict escalate.
- **Design review of a process item.** The process test, unless the item
  touches a reserved rule, when it is Jeff's; any doubt on P2 sends it to
  him.
- **The ledger.** v1, additively. A v2 only as a working draft on the
  program trunk, written by no tagged build, finalized at the landing.
- **A step that waits on Jeff.** Only setup, component design or
  privileged work (Decision 11), and the escalations of a15 (b); anything
  else is decided by the loop and recorded.
- **Program repositories.** All on the umbrella's list, created private in
  the one-time setup, each with its CI and flow18 workflows on `main` and
  their job names required by its ruleset before its program trunk is
  cut.
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
  - Escalations of a stuck program item: each third and fourth return
    and each first runaway go to 0; a fifth return, a second runaway, a
    repair exhaustion and an unresolved sync conflict still reach him, as
    he chose (a15 (b)). K1's one escalation so far was at its third
    return, so under this rule it would not have reached him.
  - Process item reviews: each process item that touches no reserved rule
    goes from a design review to the process test (a5 (b)); he still sees
    it once, at its merge to `main`.
  - Questions of strategy: each blocking question on how to implement a
    program item becomes an assumed one the umbrella session confirms.
  - Repositories: one word per repository as its phase starts (a11 of
    componentization) becomes one setup sitting (a18), which also takes
    each new repository's CI merge and required checks.
  - The program's end: answering each of 8 holds becomes the loop's
    resume.
- **Time saved.** Three in-progress places and an `alters_relationships`
  slot are freed now; a program PR merges in the run that makes it ready,
  not at his next sitting; pins can follow a pre-release the same day.
- **Cost.** 12 steps (11 + 1 state step) over two runs, one review
  sitting, and his one-time actions (Decision 16).

## What this item does not do

- Change FLOW-09: a program item still waits for its blocking native
  validation. Tracked gaps are already the design's choice.
- Let the loop answer a LOOP-04 release wait by itself when the tag
  appears; a follow-up process item may, under the bar.
- Count the PR slot per trunk: with non-program work held, `main` has no
  feature PRs to share it with.
- Create any repository itself (Jeff does, in the setup, a18), or change
  `main`'s protection.
- Let the loop merge to `main`, or let the process test take a process
  item that widens the loop's authority or changes a rule on security,
  merging, pushing or tags (a5 (b)).
- Let any build tagged before the landing write a ledger v2, or change
  v1 other than additively (a7).

## Rule changes

Each block quotes `main` at `be13847`. Severity and check are unchanged
unless said. Jeff's answers of 2026-11-22 add LOOP-09 and FLOW-14 (a5 (b))
and a second FLOW-03 block, and drop the FLOW-11 amendment of `8cb13d8`:
under a15 (b) a program item's repair exhaustion escalates as FLOW-11
already says, and FLOW-20 names FLOW-11 among the rules that apply as
written.

### FLOW-20 (amend), `docs/development/process.md`

MUST; check changes from `agent-judgment` to `automated + agent-judgment`.
**Why:** Decisions 1-17, with Jeff's answers of 2026-11-22.

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
>   steps and review. Its design review, or the process test's session,
>   declines one that does neither, and the item is then held until the
>   program's end, unless the human releases or closes it. The rules this
>   rule *reserves to the human* are, read by a change's effect and not by
>   the files it names: any change that widens the loop's own authority,
>   that is, lets the loop take without the human a step, merge, push, tag,
>   filing or decision it could not take before, removes or narrows a gate
>   of the human's, loosens the umbrella test or the process test, relaxes
>   or removes a check that stops the loop, or raises a limit, budget or
>   permission of the loop's; and any change to a rule about security,
>   merging, pushing or tags. A process item whose change touches none of
>   them is settled by the *process test* in place of the human's review
>   (LOOP-09, FLOW-14): its Program payoff section shows this bar is met;
>   it touches none of the reserved rules; and a review session that did
>   not write the design agrees on each point. A doubt about the second
>   point sends the design to the human's review (FLOW-19); one whose
>   payoff the session does not find shown is held until the program's end.
>   The pass is recorded, and the item's merge to the trunk stays the
>   human's (FLOW-08).
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
>   (RELAY-05); and any security boundary (GLOBAL-06). If the program needs
>   a new major version of the ledger format, it is developed as a *working
>   draft* beside v1 on the program trunk, marked unreleased in the document
>   and in the specifications index, and written by no build tagged from
>   the program trunk; the design that opens it is component design, and it
>   is finalized and released as that version only when the program lands.
>   A program trunk may
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
>   item's pull request into it when FLOW-08's merge gate passes, and merges
>   the trunk into it whenever the trunk moves past it, each as a merge
>   commit, never a squash or a rebase. A process item, and any change to
>   the loop's rules or machinery, goes to the trunk by the human's merge
>   (LOOP-09).
> - **Human gates.** A step waits on the human only when it is one of three
>   kinds (the human, 2026-11-21): *setup*, the one-time setup of the
>   program's repositories and infrastructure; *component design*, a
>   question or review about a component's seams, boundaries, contracts or
>   security posture, not about the strategy to implement it, and the
>   design of a process item the process test may not settle (LOOP-09); and
>   *privileged* work, which needs
>   the human's privileges or configuration: a merge to a trunk, a change
>   to CI or release configuration, a release without a suffix, making a
>   repository public, the machines of the hosts table (LOOP-05), a native
>   step only the human's attended machine can take, and a failure only
>   those can fix. An escalation of a program item that does not converge,
>   below, also reaches the human. The loop decides everything else, and
>   records it:
>   - A program feature's design review is taken by the *umbrella test*
>     when every component-design point holds: the design implements the
>     components, seams and boundaries the program's design names for one
>     of its phases, or a filed split, follow-up or re-sequencing of them,
>     within that phase's work; no blocking or flagged question is open; it
>     declares no rule change (FLOW-14), and changes no loop machinery and
>     no check that enforces a loop rule (LOOP-09); its
>     `security_boundary` is `none`, or its only trip is GLOBAL-06's relay
>     clause and its security note shows no boundary's posture changes;
>     and each interface break is one this rule allows. A review session
>     that did not write the design judges each point, and checks the
>     item's repositories and areas against the area map and the
>     repository's dependency edges; a failure of that check, or a
>     disagreement on anything but a component-design point, returns the
>     design to Design (FLOW-06). A design that fails a component-design
>     point is reviewed by the human (FLOW-19).
>   - On a program item, a question is blocking, or flagged, only when it
>     is component design or needs setup or privileged work; any other
>     question proceeds as assumed on Design's recommendation, and the
>     umbrella test's session confirms it (FLOW-07, FLOW-19). A design
>     follows the human's recorded decisions: one that departs from a
>     decision on component design flags the question, and one that
>     departs from a decision on strategy records the departure.
>   - A program item that does not converge escalates later than other
>     items: it escalates when it returns to Design more than four times
>     (FLOW-06), and its first runaway budget raises its estimate to its
>     spend so far, recorded, so that only its second escalates (FLOW-13).
>     A CI failure Publish cannot repair escalates as FLOW-11 says. A program
>     sync whose conflict the loop cannot resolve leaves the program trunk
>     as it is, and escalates: the loop files a program item that resolves
>     it, whose first step is the escalation.
>   - A tier-2 MUST finding the loop may not defer (FLOW-08) returns the
>     item to Design for a change that conforms; it reaches the human only
>     to be dismissed, or when the posture or contract it protects would
>     have to change.
>   - The loop files a phase's items once the gate before its wave is met,
>     a reconciliation item that adopts a pre-release on a program trunk
>     (LOOP-07), and the relations between items that their designs name;
>     and it changes an area map where a program item's design, reviewed or
>     passed by the umbrella test, names the change, or where the change
>     brings the map's `reaches` in line with the repository's dependency
>     edges (FLOW-05). It places what it files (FLOW-05): a phase's items by
>     the phase order of the program's design, and any other program item
>     as intake places one, above every released item of its class that is
>     not a program item; and it records each placement.
>   - Releasing a hold, marking a fix urgent and the order of the queue are
>     the human's word whenever given; no step waits on it.
>   - Every repository on the program's list is created private in the
>     setup. A repository not on it is a new component, and proposing one
>     is component design; making one public is privileged work.
> - **Records.** Each decision this rule lets the loop take in the human's
>   place (a loop merge, a deferred tier-2 MUST finding, a protected one
>   sent back to Design, an umbrella test, a process test, a security note
>   read under the umbrella test, a question settled as assumed, a departure
>   from a decision on strategy, a return to Design past the second, a
>   re-estimate, a program sync's conflict resolution, a filed item or
>   relation, an area-map change, a pre-release or final tag, a hold resumed at the program's
>   end) is recorded in the
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
> RELAY-05, RELAY-08), the SEC rules, UX-01, UX-04, FLOW-11, FLOW-18 and
> the LOOP rules other than LOOP-07 and LOOP-09 apply to program items as
> written; FLOW-06, FLOW-07, FLOW-08, FLOW-13, FLOW-14, FLOW-19, LOOP-07
> and LOOP-09 apply as this rule reads them.
>
> The program ends when every component works as intended when built from
> its own repository, and the human has squash-merged each program trunk
> into its trunk, or earlier by the human's word. A process item that the
> loop holds until the program's last phase has merged lands it: its design
> names one pull request per repository from the program trunk to the
> trunk, whose body lists the program records and the evidence that each
> component works when built from its own repository, and its own change
> retires this rule. Before the landing pull requests open, the loop tags
> each program trunk's head `program/componentization-final`, recorded, so
> the per-item merge commits stay reachable; it is never moved or deleted.
> Its Close resumes every item held until the program's
> end, each returning to Design for what the program changed, and records
> it; the human may close any of them instead.
>
> The automated part: wf chooses an item's trunk, checks the queue's program
> mark against the design's `related`, takes an umbrella-test or
> process-test line as an approval, counts a held item toward no limit,
> seeds a program item's escalations at the later points above, and merges
> with a merge commit only; a test refuses a ledger working draft on a
> pull request or push to the trunk. The agent part: intake and Design
> judge whether an item is a program item and which kind a question is;
> the design review, the umbrella test's session or the process test's
> session judges a Program payoff, each interface break, each reserved
> rule and each point of its test.

**Enforcement:** `Config.TrunkFor` and the recorded trunk, `Feature.Program`,
the declared check, `approvals` (`check/review.go`), the `x2-process-test`
seeding (`queue/phases.go:75-84`, `seeded`, `IsProcessTest`), `wf hold`,
the later escalation thresholds (`queue/phases.go:599-605` for returns;
`State.Runaway` at `queue/queue.go:1425` counting `wf reestimate`'s
events), `check.SyncReady`, `wf-publish`'s `--merge`, and `checkDrafts`
(`specs/specs_test.go`).

### FLOW-03 (amend), `docs/development/process.md`

MUST, `automated`. **Why:** phase 7 names the human's merge (a11); the
process route names the human's review (a5 (b)).

**Current wording** (`process.md:158`):

> | 7 | Merge | The human merged the PR (FLOW-08). The loop confirms it, in an unattended run or in an attended session (FLOW-04); there is no step for the human to claim. The PR is ready for the merge only while its branch contains `main` (FLOW-08); when `main` moves past it, a refresh step brings it up to date (FLOW-10). |

**Proposed:**

> | 7 | Merge | The human merged the PR (FLOW-08), or, for a PR to a program trunk, the loop merged it once FLOW-08's merge gate passed (FLOW-20). The loop confirms it, in an unattended run or in an attended session (FLOW-04); there is no step for the human to claim. The PR is ready for the merge only while its branch contains its trunk, `main`, or, for a PR to a program trunk, that program trunk (FLOW-08); when that moves past it, a refresh step brings it up to date (FLOW-10). |

**Current wording** (`process.md:191-195`, and phases 1 and 2 of the
process route, `:202-203`):

> A **process item** (queue `kind: process`) changes the rules the loops run
> under: `docs/development/`, `.claude/` or `tools/workflow/`. Its design is the
> human's, settled in a design review (FLOW-19); after that the loop works on its
> branch, and nothing it changes reaches the running loop until the human merges
> it (FLOW-14).

> | 1 | Design | The design document exists on the branch and its design review is recorded as approved. |
> | 2 | Implement | The branch is pushed (FLOW-17), the design archived on it with its summary, and its PR open with CI green. |

**Proposed:**

> A **process item** (queue `kind: process`) changes the rules the loops run
> under: `docs/development/`, `.claude/` or `tools/workflow/`. Its design is
> settled by the human in a design review (FLOW-19), or, while FLOW-20 stands
> and the change touches none of the rules FLOW-20 reserves to the human, by
> FLOW-20's process test; after that the loop works on its
> branch, and nothing it changes reaches the running loop until the human merges
> it (FLOW-14).

> | 1 | Design | The design document exists on the branch and its design review is recorded as approved; while FLOW-20 stands, the design document exists on the branch and Design has recorded whether it takes the process test or the human's design review (FLOW-19, FLOW-20). |
> | 2 | Implement | While FLOW-20 stands, its design review recorded as approved, or its process test as passed, before implementing starts (FLOW-20). The branch is pushed (FLOW-17), the design archived on it with its summary, and its PR open with CI green. |

**Why** (second and third blocks, added): the process test's gate sits in
phase 2, beside `x2-implement`, as the feature round's design review sits
beside Plan, so wf chooses it after Design has recorded `design_review`.

**Enforcement:** the merge proof (`check/check.go:501-541`) with
`TrunkFor`; `approvals` and the `process` template's `x2-process-test` or
`x2-design-review`.

### FLOW-04 (amend), `docs/development/process.md`

MUST, `automated`. **Why:** the loop's merge is a step it takes (a11 (b));
a program item escalates later (a15 (b)); the umbrella test and the process
test take design reviews (a13, a5 (b)).

**Current wording** (`process.md:227-229`):

> - `human` — answering blocking questions, the design review (FLOW-19), and
>   escalations: a runaway budget (FLOW-13), CI repair exhausted (FLOW-11), a third
>   return to Design (FLOW-06), a branch that did not delete on merge (FLOW-16).

**Proposed:**

> - `human` — answering blocking questions, the design review (FLOW-19),
>   except one FLOW-20's umbrella test or process test takes, which is
>   `cloud`, and escalations: a runaway budget (FLOW-13; a program item's
>   second), CI repair exhausted (FLOW-11), a third return to Design
>   (FLOW-06; a program item's fifth), a program sync whose conflict the
>   loop cannot resolve (FLOW-20), a branch that did not delete on merge
>   (FLOW-16).

**Current wording** (`process.md:248-250`):

> A merge step, and a step confirming that a pull request its design names
> to merge first has merged (FLOW-08), stays `cloud` and is never offered
> to a worker (FLOW-05).

**Proposed:**

> A merge step, and a step confirming that a pull request its design names
> to merge first has merged (FLOW-08), stays `cloud` and is never offered
> to a worker (FLOW-05), except the merge of a pull request into a program
> trunk, and the confirmation of one into a program trunk, which are
> offered to a `cloud` worker that merges it while FLOW-08's merge gate
> passes for its repository (FLOW-20).

**Enforcement:** `queue/next.go:32-43, 190-193, 242-270`;
`queue/limits.go:409`.

### FLOW-05 (amend), `docs/development/process.md`

SHOULD, `automated`. **Why:** held items hold no limit (a3); a PR the loop
merges does not wait on the human (a11); the loop changes an area map as a
program design names or the dependency edges show (a16); the loop places
the items it files (a16).

**Current wording** (`process.md:311-313`, from "the human places"):

> the human places an item, at intake or by moving it, except where a rule
> places it (FLOW-09, LOOP-04), and the position asked for is kept as nearly
> as the item's class, and its status within that class, allow.

**Proposed:**

> the human places an item, at intake or by moving it, except where a rule
> places it (FLOW-09, LOOP-04, FLOW-20), and the position asked for is kept
> as nearly as the item's class, and its status within that class, allow.

**Current wording** (`process.md:337-339`):

> the queue state, where the Areas entry of the repository's profile says
> (LOOP-01), and it changes only in an attended session, on the human's
> word: no agent changes it otherwise.

**Proposed:**

> the queue state, where the Areas entry of the repository's profile says
> (LOOP-01), and it changes only in an attended session, on the human's
> word: no agent changes it otherwise, except that, while FLOW-20 stands,
> the loop makes a change that a program item's design, reviewed or passed
> by FLOW-20's umbrella test, names, or that brings the map's `reaches` in
> line with the repository's dependency edges, and records it (FLOW-20).

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
> the human's merge, or held at a program trunk's merge on a security note
> or a finding only the human may clear (FLOW-08), or a spent budget
> (FLOW-13), which a program item's first is not (FLOW-20); it counts once, and
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

MUST, `agent-judgment`. **Why:** a15 (b).

**Current wording** (`process.md:430-431`):

> delta (FLOW-19). A feature that returns to Design more than twice escalates to a
> `human` step instead of looping again.

**Proposed:**

> delta (FLOW-19). A feature that returns to Design more than twice escalates to a
> `human` step instead of looping again; a program item, more than four times
> (FLOW-20). Each of a program item's returns past the second is recorded.

**Enforcement:** `queue/phases.go:599-605`.

### FLOW-07 (amend), `docs/development/process.md`

MUST, `agent-judgment`. **Why:** a13.

**Current wording** (`process.md:450-451`):

> - **Blocking** — the design cannot proceed without the answer. The feature waits on
>   a `human` step; the worker moves on to other work.

**Proposed:**

> - **Blocking** — the design cannot proceed without the answer. The feature waits on
>   a `human` step; the worker moves on to other work. For a program item
>   (FLOW-20), a question is blocking only when it is component design or
>   needs setup or privileged work; any other proceeds as assumed, on its
>   recommendation.

**Enforcement:** none new; Design and the umbrella test's session judge a
question's kind (`wf-design`, `wf-review`).

### FLOW-08 (amend), `docs/development/process.md`

MUST, `automated`. **Why:** a11 (b), a12, a14.

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
> program sync below. It merges a program item's pull request, in each
> repository the item has one, only while the *merge gate* passes on its
> head:
>
> 1. the base is the program trunk;
> 2. the head is the one recorded for the item's pull request and the
>    remote branch's head;
> 3. every check on it passed or was skipped, none failed, and every check
>    the program trunk's protection requires passed;
> 4. the branch contains the program trunk;
> 5. the tier-2 report is for that head and has no open MUST finding but
>    one FLOW-20's records defer;
> 6. no human step of the item is pending;
> 7. its `security_boundary` is `none`, or its only trip is GLOBAL-06's
>    relay clause with no change to a boundary's posture, which its review
>    accepted; otherwise the human acknowledges its security note before
>    the merge;
> 8. each pull request its design names to merge first has merged;
> 9. the rule-change clearance passes.
>
> The *program sync* is a pull request whose head merges the trunk into
> the program trunk. The loop merges it once points 1 to 4 pass, where the
> head in point 2 is the one the sync step recorded and point 4 also needs
> the head to contain the trunk, and point 5 on its conflict-resolution
> commit when it had one (FLOW-20). The loop merges with a merge commit,
> never a squash or a rebase, pinned to the head it checked, and records
> the merge (FLOW-20). The loop defers a MUST finding
> only after one fix round, and never one under GLOBAL-06, a SEC rule,
> RELAY-01 to RELAY-03 or RELAY-05, or that breaks what FLOW-20 says never
> breaks: such a finding returns the item to Design for a change that
> conforms (FLOW-20). It never dismisses one. The human's merge of the program trunk
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

**Enforcement:** `check.MergeReady` (points 1 to 9, per repository),
`check.SyncReady` (the sync), `f7-merge` and `f6-merged-<n>` on a program
trunk (`queue/next.go`), the records (`wf record`, `wf program-body`).

### FLOW-13 (amend), `docs/development/process.md`

SHOULD, `automated`. **Why:** a15 (b).

**Current wording** (`process.md:647-648`):

> the run: an open (queued or in-progress) item that spends `runaway_factor` times
> its estimated steps escalates to a `human` step, and no unattended run takes

**Proposed:**

> the run: an open (queued or in-progress) item that spends `runaway_factor` times
> its estimated steps escalates to a `human` step (a program item's first
> time, its estimate is raised to its spend so far instead, and recorded; its
> second escalates, FLOW-20), and no unattended run takes

**Enforcement:** `State.Runaway` (`queue/queue.go:1425`), which counts
the item's `re-estimated (FLOW-13)` events, and `wf reestimate`
(`cmd/wf/main.go`), which raises the estimate and logs the event (Design,
*What reaches Jeff*, *The runaway, counted*).

### FLOW-14 (amend), `docs/development/process.md`

MUST, `automated + agent-judgment`. **Why:** a5 (b).

**Current wording** (`process.md:671-678`):

> change their own definitions or these standards on `main` without the human.
> There are two routes, and both reach the running loop only through the human's
> merge (FLOW-08):
>
> - **A process item** (FLOW-03), whose design the human settles in a design
>   review (FLOW-19) before the loop implements it on its branch. It is the only
>   route for `.claude/` and for `tools/workflow/` outside `check/`, the loop's
>   own machinery, for a loop rule (LOOP-09), and the route for a rule change
>   no feature needs.

**Proposed:**

> change their own definitions or these standards on `main` without the human.
> There are two routes, and both reach the running loop only through the human's
> merge (FLOW-08):
>
> - **A process item** (FLOW-03), whose design the human settles in a design
>   review (FLOW-19), or, while FLOW-20 stands, whose design passes FLOW-20's
>   process test because it touches none of the rules FLOW-20 reserves to
>   the human, before the loop implements it on its branch. It is the only
>   route for `.claude/` and for `tools/workflow/` outside `check/`, the loop's
>   own machinery, for a loop rule (LOOP-09), and the route for a rule change
>   no feature needs.

**Enforcement:** `approvals` accepts `process test passed at <sha>` for a
process item only; the process item branch's exemption from the clearance
is unchanged.

### FLOW-17 (amend), `docs/development/process.md`

MUST, `agent-judgment`. **Why:** program trunks and the loop's tags (a10,
a17, Decision 12).

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
> deleting a remote branch (FLOW-16), or a tag, except a pre-release tag or
> a program trunk's final tag FLOW-20 lets the loop push, and a release tag
> an attended session pushes on the human's word; no tag is moved or
> deleted. An unattended
> session on a native machine pushes a

### FLOW-19 (amend), `docs/development/process.md`

MUST, `automated + agent-judgment`. **Why:** a13, a14, a5 (b).

**Current wording** (`process.md:875-877`):

> answered on its own. A question is flagged, and always answered on its
> own, when it touches a security boundary or a seam, or departs from
> prior art or from an earlier decision of the human's.

**Proposed:**

> answered on its own. A question is flagged, and always answered on its
> own, when it touches a security boundary or a seam, or departs from
> prior art or from an earlier decision of the human's. For a program item
> (FLOW-20), a question is flagged only when it is component design: when
> it changes a seam in a way the program's design does not name for the
> item's phase, changes a security boundary's posture, a component's
> boundary or a contract, or departs from an earlier decision of the
> human's on component design.

**Current wording** (`process.md:892-893`):

> Design decides whether the review is needed and records it as
> `design_review: required` or `skip` in the front-matter and the queue. `skip` is

**Proposed:**

> Design decides whether the review is needed and records it as
> `design_review: required`, `skip`, or, while FLOW-20 stands, `umbrella` for
> a program feature or `process-test` for a process item (FLOW-20), in the
> front-matter and the queue. An `umbrella` review is taken by FLOW-20's
> umbrella test; its line `umbrella test passed at <sha>` is an approval; a
> failed component-design point makes it `required`, and any other failed
> point returns the feature to Design (FLOW-06). A `process-test` review is
> taken by FLOW-20's process test; its line `process test passed at <sha>` is
> an approval; a failed or doubtful reserved-rule point makes it `required`.
> `skip` is

`docs/design/TEMPLATE.md`'s front-matter comment becomes `required | skip
| umbrella | process-test (FLOW-19, FLOW-20)`.

**Enforcement:** `check/review.go` (`approvals`, the Plan gate,
`reviewDecision` at `:107`), `queue/queue.go:1313`'s `design_review`
validation, the `f2-umbrella` and `x2-process-test` steps' seeding.

### LINT-03 (amend), `docs/development/behavioral-linter.md`

MUST, `automated`. **Why:** a12, a17.

**Current wording** (`behavioral-linter.md:59-65`, to "PR comment."; the
rest of line 65 and what follows it, from "For releases, the gate needs a
concrete home", are kept unchanged after the proposed text):

> Tier 1 red blocks the release outright. Tier 2 MUST-severity findings require
> explicit human sign-off (acknowledge, or dismiss with a recorded reason) before the
> release proceeds; SHOULD findings are informational. The agent never auto-blocks and
> never auto-approves trunk.
>
> Per PR, the sign-off is the human's merge (FLOW-08): the PR body lists the
> MUST findings, and a dismissal is recorded as a PR comment. ...

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
> program trunk tags needs tier 1 green on its commit; the full gate above is
> for a release without a suffix. ...

**Enforcement:** `release.yml:71` gains `--exclude '*-*'` in its `tier1`
job, so a release without a suffix is diffed against the last one
(LINT-04).

### LINT-04 (amend), `docs/development/behavioral-linter.md`

MUST, `automated`. **Why:** a17: the loop tags pre-releases, so "the last
release tag" must not mean a `-dev` or `-rc` tag.

**Current wording** (`behavioral-linter.md:77-78`):

> The linter runs on a diff, never the whole repo: the PR diff per-PR, and
> `last-release-tag..HEAD` at release.

**Proposed:**

> The linter runs on a diff, never the whole repo: the PR diff per-PR, and
> `last-release-tag..HEAD` at release, where the last release tag is the
> last tag without a suffix (a pre-release, FLOW-20, is not one).

**Enforcement:** `release.yml:71`'s `git describe --exclude '*-*'`.

### LOOP-01 (amend), `docs/development/loop-contract.md`

MUST, as written. **Why:** a10.

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

MUST, `agent-judgment`. **Why:** a16, a17.

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

### LOOP-09 (amend), `docs/development/loop-contract.md`

MUST, `agent-judgment`. **Why:** a5 (b).

**Current wording** (`loop-contract.md:409-415`, `:430-431`):

> The loop rules (LOOP-\*, FLOW-\*, LINT-\*), the loop's machinery (its
> tools, skills and agent definitions) and the checks that enforce a loop
> rule, wherever they are kept, change only through a process item
> (FLOW-03, FLOW-14). Its design is settled by the owner, the human of
> FLOW-08, in a design review (FLOW-19), and its merge is the owner's. No
> component signs off a change, and no feature's design declares one. A
> component that needs a loop rule changed asks for a process item.

> The agent part: intake and the design review judge that a change to a
> loop rule or to its enforcement is a process item.

**Proposed:**

> The loop rules (LOOP-\*, FLOW-\*, LINT-\*), the loop's machinery (its
> tools, skills and agent definitions) and the checks that enforce a loop
> rule, wherever they are kept, change only through a process item
> (FLOW-03, FLOW-14). Its design is settled by the owner, the human of
> FLOW-08, in a design review (FLOW-19), and its merge is the owner's. While
> FLOW-20 stands, a process item whose change touches none of the rules
> FLOW-20 reserves to the owner is settled instead by FLOW-20's process
> test, a review session that did not write the design, and is recorded;
> its merge is still the owner's. Where this contract names a process
> item's design review, that test stands in for it. No
> component signs off a change, and no feature's design declares one. A
> component that needs a loop rule changed asks for a process item.

> The agent part: intake and the design review, or the process test's
> session, judge that a change to a loop rule or to its enforcement is a
> process item, and the process test's session that it touches none of
> the rules FLOW-20 reserves to the owner.

**Enforcement:** none new in this repository's checks; the
`x2-process-test` step and its approval line (FLOW-14, FLOW-20).

### PROFILE-01 (amend), `docs/development/loop-profile.md`

MUST, as written. **Why:** a10, a16, a17.

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

> It is not one of this repository's rule files: it changes in an attended session, on the human's word, or by the loop, as a program design names or as the dependency edges below show (FLOW-05, FLOW-20). `tools/tier1/area-edges.sh` prints this repository's dependency edges, which the loop compares the map's `reaches` with. |

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
trailer naming FLOW-20 (a6).

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
that could open a boundary is never deferred or merged over, and sends the
item back to Design; a change to a component's security posture is
component design, so the design that makes it is reviewed by Jeff and the
program PR that carries it waits on his acknowledgment of its security
note; the decisions this item moves from Jeff to the loop (questions of
strategy, returns to Design past the second, a first runaway's
re-estimate, holds, area maps, filing) open no boundary; a
change to CI or release configuration, which runs with signing and package
write access on the loop's tags, is never loop-merged; and every program trunk lands
on `main` only by Jeff's squash merge, with each deferred finding and
security note listed. Each loop merge is a merge commit pinned to the
checked head, so the program trunk keeps every item's reviewed commits
as they were checked. The process test never takes a process item that
widens the loop's authority, read by its effect (loosening either test,
removing or narrowing a gate of Jeff's, relaxing a check that stops the
loop, raising a limit, budget or permission included), or that changes a
rule on security, merging, pushing or tags, and a doubt sends the item to
Jeff; every process item still reaches `main` only by his merge.
A ledger working draft is written by no tagged build, so no ledger a
user holds depends on an unreleased format, and the format's guarantees
(verifiable, tamper-evident, complete) change only by his review. A
pre-release is never Latest and never moves the relay image's `latest`.
The program trunk's ruleset (required checks, merge commits only, no
force-push or deletion) is Jeff's, and no agent changes it.

## Validation plan

- **Tier 0** (TEST-01, TEST-02; `make test`), states built with `Add`,
  `PutOnHold`, `Claim` and `Complete`:
  - `queue`: `Held` and the limits as in the earlier draft; `TrunkFor`
    gives the program trunk only for a program, non-process item with no
    machinery, `.github/` or `.goreleaser.yaml` area in an entry that has
    one, and `main` for one that reaches `.github/`; once recorded at
    `f0-branch-setup`, the trunk stays, and `wf declare` refuses an area
    change that would flip it; `Validate` refuses a program
    trunk equal to the trunk; a merge step on a program trunk is offered to
    a cloud worker only when the gate passes and never otherwise; a
    two-repository program item's `f6-merged-<n>` and `f7-merge` are both
    offered as cloud merge steps, each under its repository's gate, and
    the item merges with no human step; a PR at
    `f7-merge` or an `f6-merged-<n>` on a program trunk does not wait on
    the human, and one held on a posture change
    does; a program item's third and fourth returns to Design do not
    escalate and are recorded, and its fifth seeds a `human` escalation,
    while a feature's third still does; a runaway program item with no
    `re-estimated` event is not waiting on the human and `wf reestimate`
    raises its estimate and logs the event, recorded and not a `human`
    step; with one such event, its next runaway waits on the human; a
    second `wf reestimate` is refused to a cloud worker;
    a program item's repair exhaustion escalates as any item's; an
    unresolved sync conflict files `program-sync-<sha>` whose first step is
    a `human` escalation, and the next sync waits for it.
  - `queue`, the process test: while an entry has `program_trunk`, a
    process item's `x1-design` is `cloud`, and a human worker cannot claim
    it; phase 2 seeds `x2-process-test` for `design_review: process-test`
    and `x2-design-review`, `human`, for `required`, chosen when phase 2 is
    entered; `x2-implement` waits for the one seeded, and `wf done` on
    either runs the approval check; a failed P1 on an in-progress item
    that already has questions is held with the next free `q<N>` and
    `resumes: program-end`, and `x2-implement` needs its answer; a held
    process item, and one in Design or at `x2-process-test`, holds no
    worker its `holds` names; with no `program_trunk`
    anywhere, the template seeds as today and `process-test` is refused.
  - `check`: `MergeReady` fails on each of its nine points alone, with
    fixtures (a base other than the program trunk; a moved head; a failed or
    missing required check; a branch behind; an open protected MUST; a
    pending human step; a boundary not accepted, or a posture change; a
    merged-first PR open; an uncleared rule change) and passes with none,
    including one whose push-event job on the same head was skipped, and
    reads point 3's required checks from the ruleset; `SyncReady` passes a
    sync head that merges `origin/main` into the program trunk, and fails
    one whose head is `main`'s own commit, a moved head, or a failed
    required check; after a merge commit on the program trunk, `wf done`
    passes once `origin/<program trunk>` is fetched and fails on the
    unfetched ref; the
    merge proof accepts a merge commit (`--merge`) on the program trunk,
    recording the `Merge pull request #N` commit, and still accepts a squash
    on `main`; `approvals` accepts an umbrella line and the Plan gate fails
    on an impact field changed after it; `approvals` accepts a process-test
    line on a process item only, and `x2-implement`'s delta check fails on
    `rule_changes` changed after it; the declared check fails on a
    `program` mark that disagrees with `related`.
  - `summary`: **Held**, and **Decided for you** with a departure from a
    strategy decision, a return past the second and a re-estimate listed
    first; an escalation listed under
    Blocked on Jeff as an escalation, not a kind.
  - `specs`: `checkDrafts` passes a document with the working-draft block
    listed as unreleased with `GITHUB_BASE_REF=program/componentization`,
    fails it with `GITHUB_BASE_REF=main` or `GITHUB_REF_NAME=main`, and
    fails one the index does not list as unreleased.
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
  `program/componentization`; a PR to the program trunk gets the check
  runs its ruleset requires (`ci-ok` and `sessions-private` in this
  repository, and in each new program repository the job names Jeff
  required at setup, which its program trunk's workflows run); the first program merge is a merge commit on the
  program trunk, recorded in `program/records.yaml`; the program trunk's
  ruleset shows merge commits as the only method allowed.
- No native host is needed.

## Implementation plan

A process item has no Plan phase; this is the work of `x2-implement`. Each
step writes its tests first, sees them fail on the code before it (TEST-01,
TEST-02), and ends with `make test` green.

1. Holds: `queue/queue.go` `Question.Resumes`, `Feature.Held`,
   `PutOnHold(resumes)`, extended for an in-progress item (the next free
   `q<N>`, and the current phase's pending gate needs the answer step);
   `wf hold -feature F -why ... -resumes ...` for an existing item;
   `State.Hold` (`queue/next.go`) skips a held item and a process item
   still before Implement; `queue/limits.go` `counted`, `WaitsOnHuman`;
   `queue/summary.go` **Held**; `cmd/wf/main.go` `-resumes` (FLOW-05, a3).
2. Program trunk: `RepositoryEntry.ProgramTrunk`, `Feature.Program`,
   `Config.TrunkFor`, the trunk recorded on each work entry at
   `f0-branch-setup` or `wf declare` and returned from then on, `Validate`
   and `wf declare` refusing a change that would flip it; `check/repos.go`
   `trunk()`; the declared check; `wf trunk`, `wf main -trunk`, `wf stale`, `wf areas`, `wf drops`,
   `wf new -program` (FLOW-20, LOOP-01, a10).
3. Merge gate: `check.MergeReady(env, f, repo)` against `TrunkFor(repo,
   f)`, point 3 reading the required checks from the ruleset, `wf check
   -step f7-merge [-repository R]`; the merge label; the merge proof's
   test for a merge commit on a program trunk, after the fetch;
   `queue/next.go` and `queue/phases.go` seed and offer `f7-merge`, and
   offer an `f6-merged-<n>` on a program trunk as a cloud merge step
   under its repository's gate (`mergedFirstWait` keeps "the human's
   merge" for the others); `queue/limits.go` waiting rule, with
   `waitsOnHuman` no longer counting an `f6-merged-<n>` on a program
   trunk; `check.SyncReady(env, repo, pr)` and `wf check -sync
   -repository R -pr N`; `wf sync-program`, which prints the branch
   `sync/program-<main sha>` built by merging `origin/main` into the
   program trunk (FLOW-03, FLOW-04,
   FLOW-05, FLOW-08, a11 (b), a12); a protected finding seeds a return to
   Design; the sync's unresolved conflict files `program-sync-<sha>` with
   a `human` escalation as its first step (`wf new -escalate`).
4. Records: `wf record`, `wf program-body`, **Decided for you** in
   `QUEUE.md` and the digest (FLOW-20, Decision 13).
5. Gates: `design_review: umbrella` and `process-test` (the validators
   `queue/queue.go:1313` and `check/review.go:107`, and `TEMPLATE.md`'s
   comment), the `f2-umbrella` step, the `process` template's `cloud`
   `x1-design` and, in phase 2, `{x2-process-test cloud |
   x2-design-review human}` with `x2-implement` needing it, while FLOW-20
   stands (`queue/phases.go:75-84`; `seeded()` choosing by
   `f.DesignReview`; `IsDesignReview` extended and `IsProcessTest` added,
   so `wf done` runs `check.Review` on either); `wf-design` creating a
   process item's branch; `approvals` for both lines,
   `x2-implement`'s delta check, a failed P1's hold (`wf hold`, step 1);
   U6's area check; the FLOW-06 threshold for a program item (more than
   four returns); FLOW-13's: `wf reestimate -feature F` (cloud-allowed
   only for a program item with no earlier re-estimate) sets
   `Estimate.Steps` to `Spent.Steps` and logs `re-estimated (FLOW-13)`
   with a program record, and `State.Runaway` counts those events, so a
   program item with none is offered to the tick for the re-estimate and
   one with one escalates; with
   no FLOW-11 carve-out (FLOW-07, FLOW-14, FLOW-19, LOOP-09, a5 (b), a13
   to a15 (b)).
6. Pre-releases: `wf tag-next`; `.goreleaser.yaml`, `relay-image.yml`,
   `release.yml`, `check-pins.sh`, `pin-specs.sh`; `CONTRIBUTING.md`'s pin
   and release paragraphs (`:186-187, :205-219`) (a17).
7. CI triggers: `main.yml` and `flow18.yml` add `'program/**'`.
8. The ledger's working draft (a7): `specs/README.md`'s Layout gains the
   working-draft line and the status block's wording; `specs/specs_test.go`
   gains `checkDrafts` (RELAY-05, TEST-05). No draft is opened here.
9. Rules: FLOW-20, FLOW-03, FLOW-04, FLOW-05, FLOW-06, FLOW-07, FLOW-08,
   FLOW-13, FLOW-14, FLOW-17, FLOW-19 in `process.md`; LINT-03 and
   LINT-04; LOOP-01,
   LOOP-07 and LOOP-09; PROFILE-01; UX-03; ORCH-03; as approved (FLOW-14).
10. Skills and agents as under **Design, Skills and agents**:
   `workflow-tick` (the fetch after a merge, `wf reestimate`),
   `workflow-human` (items 0 and 3 claim and complete a process item's
   `x2-design-review`, not `x1-design`, and skip a process item whose
   `design_review` is `process-test`), `workflow-intake`, `workflow-native`,
   `wf-publish` (`--merge`, each work entry's PR, the fetch before `wf
   done`, the sync branch, no repair carve-out), `wf-review` (the
   `x2-process-test` step), `wf-design` (the process-test self-check and
   the branch);
   the workflow branch's `README.md` schema text goes in the state step.
11. `make test`, `make lint`, `flow18-check.sh`, `make enforcement`, the
    text checks; merge current `main`, archive this design with its
    summary, and open the PR to `main` with CI green (FLOW-03 process phase
    2, FLOW-12). Jeff merges it.

**State step** (`x4-state`, recorded at review with `wf state-step`;
Jeff's, after the merge, in one attended sitting, the setup of kind 1):

1. `config.yaml`: `program_trunk: program/componentization` on the
   `buildwarden/buildwarden` and `buildwarden/buildwarden-specs` entries,
   and an entry with it for each program repository Jeff has created
   (a18), with its `hosts.yaml` line.
2. Create each program trunk at its `main` head, on Jeff's word in this
   sitting (FLOW-17): `git push origin
   origin/main:refs/heads/program/componentization` in each repository;
   in a new program repository, only once its CI and flow18 workflows
   are on `main` and their job names are in its ruleset's required
   checks (a18, below), so the trunk starts with them.
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
   `design_review: umbrella` and `process-test`, and the records.
7. `dist/wf status`, `QUEUE.md`; `dist/wf commit`.

**What only Jeff can do, once** (outside the repository; no agent does
these, FLOW-08; all kind 1):

- **Repositories (a18).** Create every repository on the umbrella's list
  (the relay, the loop, one per driver), private, each with an initial
  commit on `main`, its CI and flow18 workflows committed to `main`
  (triggered on `main` and `'program/**'`, as this repository's are), the
  loop's App installed, and "Automatically delete head
  branches" on, which FLOW-16's Close relies on; set the same option on
  `buildwarden/buildwarden` and `buildwarden/buildwarden-specs` where it is
  off.
- **Rulesets.** On `program/componentization` in each repository (an
  organization ruleset on `program/**` covers new repositories too):
  require a pull request with no approving review (the App cannot approve
  its own); require the branch up to date; allow merge commits only, not
  squash or rebase (a11 (b)); block force-push and deletion; no bypass for
  the App. Required checks are matched by check-run (job) name, not by
  workflow name, so they are set per repository: in
  `buildwarden/buildwarden`, `ci-ok` (`main.yml`) and `sessions-private`
  (`flow18.yml`); in `buildwarden/buildwarden-specs`, the job names of its
  own CI; in each new program repository, the job names of the CI and
  flow18 workflows he commits to its `main` in this sitting, added before
  its program trunk is cut, so the trunk starts with its CI and its
  ruleset requires those checks from its first pull request. `MergeReady`'s
  point 3 reads the required list from `rules/branches/<t>` and never
  assumes one, and refuses a loop merge into a program trunk whose
  ruleset requires none, so a repository whose CI was missed at setup
  waits for it rather than merging unchecked. Nothing about a new
  repository's CI waits on him when its phase starts.
- **Tag ruleset.** One that lets the App create
  `v*-dev.*`, `v*-rc.*` and `*/v*-dev.*`, `*/v*-rc.*` tags, never update or
  delete any tag, and reserves bare `v*` tags to him, where GitHub's
  patterns allow; it also lets the App create
  `program/componentization-final`. `main`'s protection is unchanged.
- **GitHub App permissions.** Confirm the loop's App has `contents: write`
  and `pull_requests: write` on each repository, which a merge needs (it
  already pushes branches and opens PRs; merging is not verified).
- **The nightly task's prompt.** It says "Never merge" today; change it to
  "merge only as the `workflow-tick` skill allows: a program item's PR into
  its program trunk at `f7-merge`, and the program sync". The scheduled
  native sessions merge nothing and need no change.
- **`specs-prerelease-tags`**: merge it in the specifications repository
  once its PR is ready.
- Not once, but kind 3 and his alone: **the landing merges** at the
  program's end (Decision 12) and the full release tags after them.

Estimate: 12 steps (11 + 1 state step), two runs; the queue's estimate of 5
is raised at the review, on Jeff's word. Steps 1, 3 and 5 grew with the
independent check's fixes, so the review checks whether two runs still
hold before raising it.

## Design review

- **r1, 2026-11-22: approved at `60f5666`** (Jeff, workflow-human,
  FLOW-19); read at `8cb13d8` for the questions, and at `e290e4d` and
  `60f5666` for the rule changes. Page:
  https://claude.ai/artifact/RuRdk7xVctRAWoxA26NFc8.
- **Decisions.** a1 to a18 answered (Questions, Assumed): fourteen as
  recommended; a5 (b), a7 in his own words, a11 (b) and a15 (b) changed.
  Folded in at `93bb6e2`, then an independent check (four report-only
  checkers, each finding given to a refuter: 32 findings, 29 confirmed and
  applied at `e290e4d`). Follow-ups the fold-in raised, answered the same
  sitting and folded in at `60f5666`: a15 (b), the fifth return escalates
  (more than four); a5 (b), the reserved rules are his two classes judged
  by effect, not the wider set the first fold-in drafted; a18, each new
  program repository's CI and flow18 workflows go to its `main`, with
  their job names required, in the setup sitting before its program
  trunk is cut. Implementation choices the check left open (wf-design
  creates a process item's branch; an attended path to a cloud
  `x1-design` goes through `/workflow-tick attended`) were not asked:
  they are strategy, not his to decide (his three kinds).
- **Rule changes**, each asked on its own (FLOW-14), all approved as
  worded at `60f5666`; 19 entries (17 at `8cb13d8`, plus FLOW-14 and
  LOOP-09 from a5 (b) and LINT-04 from the check, less FLOW-11, which
  now applies as written):
- **rule change FLOW-20 approved at `60f5666`**, as worded under **Rule changes**.
- **rule change FLOW-03 approved at `60f5666`**, as worded under **Rule changes**.
- **rule change FLOW-04 approved at `60f5666`**, as worded under **Rule changes**.
- **rule change FLOW-05 approved at `60f5666`**, as worded under **Rule changes**.
- **rule change FLOW-06 approved at `60f5666`**, as worded under **Rule changes**.
- **rule change FLOW-07 approved at `60f5666`**, as worded under **Rule changes**.
- **rule change FLOW-08 approved at `60f5666`**, as worded under **Rule changes**.
- **rule change FLOW-13 approved at `60f5666`**, as worded under **Rule changes**.
- **rule change FLOW-14 approved at `60f5666`**, as worded under **Rule changes**.
- **rule change FLOW-17 approved at `60f5666`**, as worded under **Rule changes**.
- **rule change FLOW-19 approved at `60f5666`**, as worded under **Rule changes**.
- **rule change LINT-03 approved at `60f5666`**, as worded under **Rule changes**.
- **rule change LINT-04 approved at `60f5666`**, as worded under **Rule changes**.
- **rule change LOOP-01 approved at `60f5666`**, as worded under **Rule changes**.
- **rule change LOOP-07 approved at `60f5666`**, as worded under **Rule changes**.
- **rule change LOOP-09 approved at `60f5666`**, as worded under **Rule changes**.
- **rule change PROFILE-01 approved at `60f5666`**, as worded under **Rule changes**.
- **rule change UX-03 approved at `60f5666`**, as worded under **Rule changes**.
- **rule change ORCH-03 approved at `60f5666`**, as worded under **Rule changes**.
- **State step** recorded with `wf state-step` from the Implementation
  plan's **State step**; the setup outside the repository (**What only
  Jeff can do, once**) is his at the merge.
- sitting: 2026-11-22, asks: 44 (in 11 batches)

## Consolidation

A process item has no Publish phase and no consolidation pass (FLOW-03).

## Rollback notes

Setting `program_trunk` off in `config.yaml` returns every item to `main`
in wf; the program trunk's content then lands by an early `program-land`.
A tag already pushed stays (never moved or deleted).

## Archive summary

Filled in at the item's PR (FLOW-12).
