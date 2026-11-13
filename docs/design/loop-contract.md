---
feature: loop-contract
status: draft
drivers: []              # rules and documents; no driver tree is reached
seams: []                # the Loop Contract is seam 10 of the componentization design, a
                         # document between loops. It is not a GLOBAL-08 versioned seam,
                         # and this item changes none of those.
surfaces: []             # `wf` is the loop's tool, not a user-facing surface of the product
security_boundary: none
alters_relationships: false  # no product component's role or interaction changes (FLOW-02).
                         # The contract governs how component loops work side by side.
                         # See "What this item does not declare" under Design.
design_review: required  # a process item's design always gets its review (FLOW-03, FLOW-19)
native_validation: []    # rules, documents and two small `wf` changes; nothing host-specific
tier2_gaps: []
rules: [FLOW-01, FLOW-02, FLOW-03, FLOW-04, FLOW-05, FLOW-06, FLOW-07, FLOW-08, FLOW-09,
        FLOW-10, FLOW-11, FLOW-12, FLOW-13, FLOW-14, FLOW-15, FLOW-16, FLOW-17, FLOW-18,
        FLOW-19, FLOW-20, LINT-01, LINT-02, LINT-03, LINT-04, LINT-05, LINT-06, LINT-07,
        LINT-08, GLOBAL-04, GLOBAL-06, GLOBAL-08, GLOBAL-09, GLOBAL-10, GLOBAL-11, RELAY-05,
        TEST-01, TEST-02, TEST-03, TEST-04, TEST-05, UX-01, LOOP-01, LOOP-02, LOOP-03,
        LOOP-04, LOOP-05, LOOP-06, LOOP-07, LOOP-08, LOOP-09, PROFILE-01]
rule_changes:
  - {rule: LOOP-01, change: add, paths: [docs/development/loop-contract.md,
                                         docs/development/README.md]}
  - {rule: LOOP-02, change: add, paths: [docs/development/loop-contract.md,
                                         docs/design/TEMPLATE.md]}
  - {rule: LOOP-03, change: add, paths: [docs/development/loop-contract.md]}
  - {rule: LOOP-04, change: add, paths: [docs/development/loop-contract.md]}
  - {rule: LOOP-05, change: add, paths: [docs/development/loop-contract.md]}
  - {rule: LOOP-06, change: add, paths: [docs/development/loop-contract.md]}
  - {rule: LOOP-07, change: add, paths: [docs/development/loop-contract.md]}
  - {rule: LOOP-08, change: add, paths: [docs/development/loop-contract.md]}
  - {rule: LOOP-09, change: add, paths: [docs/development/loop-contract.md]}
  - {rule: PROFILE-01, change: add, paths: [docs/development/loop-profile.md,
                                            docs/development/README.md]}
  - {rule: FLOW-01, change: amend, paths: [docs/development/process.md]}
  - {rule: FLOW-03, change: amend, paths: [docs/development/process.md]}
  - {rule: FLOW-14, change: amend, paths: [docs/development/process.md]}
concepts: [loop-contract, component-profile, loop-rules, project-invariants, spec-first,
           design-first, reconciliation, consumables, run-report, contract-adoption,
           intake-hold, componentization, program]
related: [componentization.md, workflow-bootstrap.md, workflow-streamline.md,
          native-host-loop.md, design-approved-rule-changes.md, tier1-static-checks.md,
          specs-migration.md, regression-fixes-follow-up.md]
---

# The Loop Contract: finished against the running loop, with `docs/development/` split into project invariants and loop rules

This is the first of the two items of phase P1 of the componentization
program ([`componentization`](componentization.md), a program item under
FLOW-20). It finishes the Loop Contract and splits the standards. The second
item, `loop-repository`, creates the loop repository and moves things into
it. This design says what the first hands to the second.

## Problem

BuildWarden's loop has rules for one repository. They are in
`docs/development/process.md` (FLOW-01 to FLOW-20) and
`docs/development/behavioral-linter.md` (LINT-01 to LINT-08), enforced by
`wf` (`tools/workflow/`), tier 1 (`tools/tier1/`), four skills and six agent
definitions under `.claude/`, with state on the `workflow` branch. All of it
names this repository's paths, commands and rules: `make lint`, `docs/design/`,
`overview.md`, GLOBAL-06, the drivers.

The componentization program gives every component its own repository and
its own queue (Decision 2 there), and moves the loop to its own public
repository (Decision 13). The relay repository already exists, is written in
Rust, and has "the Loop Contract adopted" in its first milestone (M1). What it
would adopt today is Jeff's draft of 2026-11-07, a document outside any
repository: eight sections with "fill in" markers and research tasks that ask
what the running loop actually does, and several statements that the running
loop has since settled differently.

Three things are missing:

1. **A finished contract.** The draft's blanks filled from the loop as it
   runs, each disagreement between the draft and the loop resolved, and the
   one new process the program needs written down: work that changes a seam
   changes its specification first (Decision 13), beside the design-first
   rule this repository already has (FLOW-01).
2. **A line through `docs/development/`.** Which rules are this project's own
   invariants and stay, and which are loop rules and move. The umbrella says
   "FLOW-*, LINT-*: move to the loop repository as the Loop Contract's
   detail" and leaves the rest to P1.
3. **A way for a second component to read rules written for this one.** The
   relay cannot run `make lint` or update `overview.md`. The contract has to
   say what is the component's to define, and must not assume a language or
   a toolchain.

The PR also carries three small changes to the loop's machinery (a11 to
a13). Neither the umbrella's P1 scope nor this item's `source` names them.
They are here because each needs a process item to reach `main` (FLOW-14)
and this is the one in flight that holds nothing; a13 was found while
reading `wf` for this design. Each is its own question, so the review can
move any of them out without touching the rest.

This item holds no worker (its queue file has no `holds`), so nothing in its
PR may disrupt running work. S1 (`seam-specs`) and K1 (`driver-contract`) are
in Design now. Their branches keep the rule text they forked with until they
next merge `main`, and from then on work under what this item merged (see
**What this item does not declare**).

## Prior art

- [`componentization`](componentization.md): the umbrella. Its Decision 2
  (a repository and a queue per component), Decision 13 (the loop repository;
  work is both spec-first and design-first), Decision 18 (a rule changes in
  the item that makes it true), seam 10 (the Loop Contract), phase P1 and the
  row "FLOW-*, LINT-*: move to the loop repository as the Loop Contract's
  detail" are what this design carries out. Where Jeff's draft (2026-11-07)
  and the umbrella (approved 2026-11-12) disagree, the umbrella is later and
  wins; each case is listed under **The draft against the running loop**.
- [`workflow-bootstrap`](workflow-bootstrap.md) stood the loop up: the
  `workflow` branch, `wf`, the skills and agents, FLOW-18's guard.
  [`workflow-streamline`](workflow-streamline.md) gave it one feature round,
  the design review and process items. The contract describes the loop those
  two built, and does not redesign it.
- [`native-host-loop`](native-host-loop.md): the scheduled native sessions,
  `hosts.yaml`, `wf gate`, and the rule that the scheduler on each machine is
  installed by hand. The draft's section 5 is filled from it.
- [`design-approved-rule-changes`](design-approved-rule-changes.md): the two
  routes by which a rule changes (FLOW-14). This design narrows the second
  route for loop rules (LOOP-09, a10).
- [`regression-fixes-follow-up`](regression-fixes-follow-up.md): FLOW-05's
  four classes. The draft has two; the four stand (a4).
- [`tier1-static-checks`](tier1-static-checks.md): the rule index
  (`tools/tier1/rules`), `rules.yaml`, the `seam-contract` and `schema-doc`
  checks. The split is shaped so that none of them changes in this PR.
- [`specs-migration`](specs-migration.md): the layout of `specs/` and
  RELAY-05, which is what the running loop has today in place of the draft's
  specification handling.
- `seam-specs` (S1, in Design on `feat/seam-specs`, not yet reviewed)
  proposes how a seam change is ordered across two repositories and how this
  repository pins a specifications release. The contract states the order
  and leaves the pin to the component (LOOP-02), so it does not depend on how
  that review ends.

Outside this repository, read for this design and not changed by it:

- **The Loop Contract draft**, Jeff, 2026-11-07
  (https://claude.ai/artifact/UY5WXovAA5phiAHU4LDbK9, read at revision 7).
  Its eight sections, three decided points and five blanks are the starting
  material. The document carries no date; 2026-11-07 is the date the relay's
  M1 design gives it.
- **The relay fork's decisions table** (read at revision 20) and
  **kickoff** (revision 12), both 2026-11-07, for what the relay's loop
  needs: decisions 2, 3, 5, 6, 7, 8 and 16 of the table, and the kickoff's
  "Loop contract" and "Design constraints" sections.
- **`buildwarden/buildwarden-relay`**, read on 2026-11-13 from a local clone:
  `main` at `e9b23d5` holds the M0 spike (`spike/`, with its Cargo manifests
  and a Makefile), a licence and a placeholder README. The branch
  `feat/m1-foundation` at `13ce286` adds only
  `docs/design/m1-foundation.md`, a complete draft, not yet reviewed and
  still being revised (it was at `9dc0ff5` an hour earlier). It has no rule
  files, queue or agent definitions. Its sections **The Loop Contract
  adopted** and **Rules mirrored** plan the adoption in two parts: M1a,
  which waits for nothing, and M1b, which waits for this item and for
  `loop-repository`. It is written against this design as it stood at
  `2c7ff0e`. Where its plan and this design now differ is listed under
  **What a second component needs in order to adopt**.

## Decisions

Three points were decided by Jeff in the draft and are carried as given: the
contract lives in its own repository, apart from the project (the umbrella's
Decision 13 names it); the queue and the iteration reports live there too;
and a change to the contract is gated on Jeff only. The decisions below are
this design's. Every one that departs from the draft is also a question, so
the review confirms it: the larger departures each have their own (a4 to a8,
a10, a14, a16, a17), and the smaller ones are asked together in a15.

1. **The finished contract is nine rules, LOOP-01 to LOOP-09, in one
   document, with FLOW-\* and LINT-\* as its detail** (a3). The nine follow
   the draft's eight sections plus its decided point on change control. Each
   holds only what is true of every component and cites the FLOW and LINT
   rules that carry the detail. *Rejected:* numbering the new statements
   FLOW-21 onward. The contract would then be indistinguishable from its
   detail, and the draft's sections would have no home. *Rejected:* prose
   with no rule IDs. Every standard here is an identified rule with a
   severity and a check (LINT-01), and skills cite IDs (FLOW-14). *Rejected:*
   restating FLOW-03 to FLOW-19 in component-neutral words inside the
   contract. Two texts of one rule drift, which is what FLOW-14's "cite, do
   not restate" exists to prevent.
2. **A component profile fills the local column, and the detail is read
   through it** (a9). Each component keeps one document that says what each
   project-specific term in the detail means there: its gate commands, its
   design directory, its trunk, its overview, its boundary rule. LOOP-01
   lists the terms. A FLOW or LINT rule that names a path, command or rule
   of `buildwarden/buildwarden` is read, in another component, as that
   component's profile entry for the term. *Rejected:* rewording all 28 FLOW
   and LINT rules to neutral terms in this PR. It changes every process rule
   under running work, it is 28 current-and-proposed wordings to approve in
   one sitting, and nothing would read the neutral words until a second
   component adopts. The rewording is a later process item (a9). *Rejected:*
   leaving the terms unexplained until then. The relay's M1 adopts the
   contract and cannot follow `make lint`.
3. **Spec-first is added to design-first and is scoped to seams** (a8).
   Every item has a design before implementation (FLOW-01). An item that
   changes behaviour a specification defines also changes that specification
   before the code. *Rejected:* the draft's wording, "a behaviour change with
   no matching spec change fails review", for all behaviour. Most behaviour
   in this repository (a driver's lifecycle, the CLI) has no specification;
   its written record is the reference docs and the overview, kept in the
   same change (UX-01, FLOW-15). That includes a declared seam with no
   written specification, such as the driver contract, which is a Go API.
   *Rejected:* letting a specification change
   stand in for the design. A seam change is the kind of decision the design
   review exists for (FLOW-19 names seams).
4. **FLOW-05's four classes stand; "feature" and "process" are the two
   kinds of item** (a4). The draft's two classes of work become the queue's
   `kind`. Its "process work goes to the top of the queue" is FLOW-05's
   class 2, behind items already in implementation, plus `holds`.
5. **Checks have four classes and two tiers, and only deterministic checks
   block** (a6). The draft's classes (build, specification, safety, process)
   say what a component declares. LINT-02's tiers say how each check is
   made. The draft's "block merge on failure" holds for a deterministic
   check, with two exceptions the loop already has: host-bound validation is
   blocking or tracked per design (FLOW-09), and a component may declare a
   deterministic check a hint or a report (here `tests-with-change` and
   `make deadcode`). LINT's load-bearing decision, that no agent holds a
   veto over trunk, stands for tier 2.
6. **The run is the unit of reporting, and the digest is the report** (a7).
   The draft's five items map onto the digest's sections; three small
   additions close the gaps.
7. **A reconciliation item is a feature item** (a5). The draft files
   "adopt consumable X at version Y" as process work. In the running loop a
   process item changes rules and has no Plan, Review, Guide or
   consolidation; an adoption changes code and needs all four.
8. **The split is made in place, by file, and no file moves in this PR**
   (a2). After it, three files are wholly loop-side
   (`loop-contract.md`, `process.md`, `behavioral-linter.md`), the others
   are wholly project-side, and the README says which is which. *Rejected:*
   moving the loop files into `docs/development/loop/` now. Tier 1's rule
   index globs `docs/development/*.md` only, so the move needs an
   enforcement change, and every link to `process.md` would break now and
   again when the files leave. *Rejected:* making no change until the
   repository exists. The next item would then have to decide the split, add
   the contract and move everything at once, while holding every worker.
9. **Rule IDs never change, and each prefix has one owner** (a16). A rule
   keeps its ID when its file moves. Prefixes are unique across components.
   LOOP, FLOW and LINT are the loop's, and the contract says so. A component
   names its own prefixes in its profile: here GLOBAL, RELAY, ORCH, EXEC,
   BOOT, UX, TEST, SEC and PROFILE. A citation by ID therefore stays valid
   across repositories. *Rejected:* a component prefix on every ID
   (`bw:GLOBAL-06`). It would rewrite the citations in every design under
   `docs/design/` and in the code comments. *Rejected:* a table of every
   component's prefixes inside LOOP-08. A loop rule changes only through a
   loop process item (Decision 10), so the relay could not take a prefix in
   its M1, and the move of RELAY-01 to RELAY-08 at M6 would need a loop
   process item beside the M6 item that the umbrella's Decision 18 says
   declares it.
10. **Loop rules change only through a process item** (a10). A feature's
    design may declare a change to its own component's invariants and
    profile, and to the checks that enforce them (FLOW-14's second route).
    It may not declare a change to a LOOP, FLOW or LINT rule, or to a check
    that enforces one, wherever that check is kept. The rule ID on the
    `rule_changes` entry decides, not the path: `tools/tier1/` and
    `tools/workflow/check/` hold checks of both kinds.
    *Rejected:* keeping the second route for loop rules. Once they are in
    another repository a feature's PR cannot edit them, and a rule that
    binds every component should not change as a side effect of one
    component's feature.
11. **The contract shares no project invariant.** GLOBAL-09 to GLOBAL-11,
    GLOBAL-06 and the testing rules stay this repository's. Another
    component declares its own invariants and may cite one of these by ID
    as binding there (the specs repository's a4 in `seam-specs` does).
    *Rejected:* promoting a "BuildWarden-wide" subset into the contract. The
    draft keeps concrete invariants local, and a Rust relay and a Go kit do
    not share a comment or error-wrapping rule.
12. **The table of host names is shared loop state; schedules and
    credentials are each machine's** (a15). The draft puts "which machines" in the
    local column. The same two machines serve every component, a host name
    belongs to exactly one machine (FLOW-04), and a machine runs one native
    session at a time (`native-tick.sh` takes a lock). One `hosts.yaml`
    beside the queues is the only way those stay true. A component's
    profile lists the host names its work uses.
13. **The line between this item and `loop-repository`** (a1): this item
    changes what the rules say; that item changes where things live and
    what `wf` reads. See **What this item hands to `loop-repository`**.
14. **The profile is one project-owned rule, PROFILE-01, and it points to
    facts kept elsewhere.** The profile sits in `docs/development/`, so a
    change to it is a rule change (FLOW-14) and needs a rule ID to be
    declared under. It gets its own, under a prefix this repository owns,
    so a feature may change it by FLOW-14's second route like any project
    invariant. Where a fact already lives in a project file
    (`tools/tier1/rules.yaml`, `hosts.yaml`, TEST-02, the design template),
    the entry names the file and does not restate it, so a feature that
    changes the fact does not also have to amend the profile. *Rejected:*
    declaring a profile change under LOOP-01. LOOP-01 is a loop rule, which
    a feature may not name (Decision 10), so every profile change would
    need an exception to that. *Rejected:* keeping the profile outside
    `docs/development/` as an ordinary file. Its entries say what the loop
    enforces here (the gates, the rule paths), and those should change only
    with the design review's approval.
15. **A repository is a component once it has adopted the contract; until
    then it is worked from a component's queue** (a17). The umbrella's
    component table lists the specs repository, the loop repository and the
    Hermit forks beside the relay and the kit, and its Decision 2 gives each
    component a repository and a queue. It does not say when each gets its
    queue, and S1 already works in the specs repository from this
    repository's queue (`seam-specs`, its a4). The contract says which rules
    bind a worker in a repository that has not adopted, and leaves when each
    adopts to the item that does it. *Rejected:* requiring every repository
    to adopt before any work in it. S1 could not run in wave 1.
16. **The draft's specification invariant is kept where a specification has
    a conformance suite** (a14). "Every behaviour in the spec has a test;
    every test maps to a spec section" becomes LOOP-03's class 2 for such a
    specification. *Rejected:* requiring it of every specification now. The
    ledger format under `specs/` has contract tests and coupling checks and
    no mapping of sections to tests, so `main` would contradict the rule on
    merge (the umbrella's Decision 18).

## Questions

### Blocking

None. A process item's design is settled in its design review, where every
assumption below is asked.

### Assumed

Each is "proceeding with the recommended option; confirm". The same
seventeen are in the queue file.

- **a1. Where the line between `loop-contract` and `loop-repository`
  falls.**
  1. *(recommended)* This item: the contract text (LOOP-01 to LOOP-09), this
     repository's profile (PROFILE-01), the split made in place, the three
     amendments (FLOW-01, FLOW-03, FLOW-14), the template's new section, and
     the skill and agent edits that make the contract true here. `loop-repository`: creates the
     repository, moves the loop-side files, `wf`, the skills, the agents and
     the `workflow` branch's content, and changes `wf` to read a profile,
     span components and scope holds per component.
  2. This item also rewords FLOW and LINT to neutral terms (see a9).
  3. This item waits for the repository and lands the contract there
     directly. It would then need `holds`, and the relay's M1 would have
     nothing to cite meanwhile.
- **a2. The form of the split in this PR.**
  1. *(recommended)* In place and by file, no file moved: the loop side is
     `loop-contract.md`, `process.md` and `behavioral-linter.md`; the README
     gains two parts and an owner column.
  2. Move the three files to `docs/development/loop/` now. Needs a change to
     tier 1's rule loader and breaks links twice.
  3. A marker on every rule (`**Scope:** loop`). A third metadata line that
     tier 1 would have to parse, for information the file already gives.
- **a3. How the contract's statements are identified.**
  1. *(recommended)* A new prefix, `LOOP-`, nine rules, in
     `docs/development/loop-contract.md`.
  2. FLOW-21 to FLOW-29 in `process.md`.
  3. Unnumbered prose with the FLOW and LINT rules as the only IDs.
- **a4. Classes of work.** The draft has two classes, with process work at
  the top of the queue and outside the feature queue.
  1. *(recommended)* FLOW-05's four classes stand (in implementation,
     process items, fixes, new functionality). "Feature" and "process" are
     the two kinds of item in one queue per component. A process item ranks
     second and stops other work only through the `holds` it declares.
  2. The draft's two classes: every process item ahead of everything,
     including items in implementation. FLOW-05 is amended, and an
     in-flight implementation is interrupted by every process item.
- **a5. The kind of a reconciliation item** ("adopt consumable X at
  version Y").
  1. *(recommended)* A feature item in the consumer's queue, at the priority
     Jeff gives it at intake. It changes code, so it gets Plan, Review,
     Guide and consolidation, and its design review.
  2. A process item, as the draft says. It would skip the tier-2 report and
     the consolidation pass, and FLOW-03's definition of a process item
     would have to grow to cover code.
  3. A feature item that FLOW-05 ranks with the fixes (class 3). Needs a
     FLOW-05 amendment.
- **a6. What a failed check does.** The draft: invariants "run on every loop
  iteration and block merge on failure".
  1. *(recommended)* The draft's four classes name what a component
     declares. Each check is tier 1 or tier 2 (LINT-02). A tier-1 check
     blocks, unless the component declares it a hint or a report (here
     `tests-with-change` and `make deadcode`). Tier 2 informs the human, who
     signs off by merging (LINT-03, FLOW-08). Checks run at the phase gates
     and in CI, scoped to the change (LINT-04), not on every step.
     Host-bound validation blocks or is tracked per design (FLOW-09).
  2. Every class blocks, the judgment tier included. Reverses LINT's
     load-bearing decision and ends report-only mode (LINT-07).
- **a7. The report of a run.** The draft lists five things every iteration
  reports.
  1. *(recommended)* The digest section of each run is the report
     (FLOW-13). It gains three things: each **Moved** line names the item's
     kind and FLOW-05 class; a step that changed a specification gets a
     **Specification changes** line apart from the code; a review step's
     line gives the check results by tier. The PR body gains a
     **Specification changes** section beside **Rule changes**.
  2. A separate report file per step, with the draft's five headings.
     A second record of what the digest, the queue history and the tier-2
     report already hold.
- **a8. When work is spec-first, and what happens to a specification that
  cannot be met.**
  1. *(recommended)* Spec-first applies to behaviour that a seam's written
     specification defines. A declared seam without one (the driver
     contract, a Go API) is design-first, and its design says what defines
     it. The item
     still has a design; the design lists the specification changes; in
     Implement the specification change comes before the code. An item that implements
     already-specified behaviour changes no specification and cites the
     requirements. A specification that cannot be met is a finding: back to
     Design (FLOW-06), recorded as a question in the queue (FLOW-07), and
     resolved by a specification change or a design change.
  2. The draft as written: all behaviour is specified, and an unmet section
     is marked as an open question inside the specification. This
     repository would need a specification of every driver and of the CLI
     first, and questions would live in two places.
  3. A specification-only item needs no design.
- **a9. How a second component reads FLOW and LINT rules written for this
  one.**
  1. *(recommended)* Through its profile and LOOP-01's reading rule, as of
     this PR. The rewording of FLOW and LINT to neutral terms is the first
     process item in the loop repository's own queue after P1, with the
     profile's term list as its specification.
  2. Reword in this PR.
  3. Reword in `loop-repository`'s PR. It then is not a move, and is
     reviewed while every worker is held.
- **a10. Who may change a loop rule.**
  1. *(recommended)* Only a process item (LOOP-09). FLOW-14 is amended by
     two sentences and FLOW-03 by one: a feature's design may not declare a
     change to a LOOP, FLOW or LINT rule, or to a check that enforces one.
     The guard is the design review, which refuses such a `rule_changes`
     entry; no enforcement code changes. Once the loop files leave this
     repository, the rule text and `tools/workflow/` are out of a feature's
     reach. Tier 1's checks of FLOW and LINT rules (`rule-index`,
     `design-front-matter`, `drivers-claim`) stay in `tools/tier1/` here,
     and the review remains their only guard until `loop-repository`
     decides where they live.
  2. Leave FLOW-14 as it is until the files move.
  3. Also make `wf` refuse, in a feature's design, a `rule_changes` entry
     that names a LOOP, FLOW or LINT rule
     (`tools/workflow/check/rulechange.go`). A mechanical guard now, at the
     cost of an enforcement change in this PR.
- **a11. How intake files an item on hold** (FLOW-20's freeze). `wf new` has
  no way; the six items closed on 2026-11-12 had been held by hand-editing a
  blocking question into each queue file, and for five of them an answer
  step.
  1. *(recommended)* `wf new` gains `-hold "<why>"`, in this PR. It writes
     what the hand-edit wrote for a queued item: question `q1`, blocking and
     open, and a human step `<f0|x1>-answer-q1` that the item's first step
     needs.
     `wf answer` already completes that step. The intake skill uses the
     flag.
  2. The same flag, added by `loop-repository`.
  3. No tool change: the intake skill spells out the YAML to add by hand.
  4. A `hold:` field that `wf next` skips, with no question. Cleaner, but
     FLOW-20 says "with a blocking question only the human answers" and
     would need amending.
- **a12. The refreshed Entire hook configuration** (commit `ac8b41a` on
  `claude/intelligent-ride-54y60a`: `.claude/settings.json` and
  `.entire/settings.json`).
  1. *(recommended)* This PR carries the two files as that commit has them.
     `.claude/` reaches `main` only through a process item (FLOW-14), and
     this is the one in flight that holds nothing.
  2. Its own process item.
  3. `loop-repository` carries it, since that item reworks `.claude/`.
  4. Drop it.
- **a13. `wf next` allows one feature past Design with
  `alters_relationships: true`, whatever `config.yaml` says.** FLOW-20 makes
  the limit "the value the human sets in the workflow branch's
  `config.yaml`", which is 3 since 2026-11-12. `wf validate` reads the value
  (`queue.go`, `Validate`); `wf next` does not (`next.go`, `altersHolder`
  returns the first other holder). `seam-specs` and `driver-contract` both
  declare `true` and are both in Design, so the second to reach Plan will be
  reported as waiting for the slot. Who is counted does not change: every
  in-progress item past Design that declares `true`, process items included,
  as both functions count today.
  1. *(recommended)* This PR fixes `wf next` to count holders against the
     configured limit, with a test. It is `tools/workflow/` outside
     `check/`, so only a process item can.
  2. Its own small process item, filed now, if S1 or K1 would reach Plan
     before this item merges.
  3. Leave it to `loop-repository`.
- **a14. The draft's specification invariant**: "every behaviour in the spec
  has a test; every test maps to a spec section". The loop has no such
  mapping today.
  1. *(recommended)* It is LOOP-03's class 2 for a specification that has a
     conformance suite: each requirement is covered by a case that names it,
     or is listed with the reason it is not machine-checked, and each case
     names the requirements it checks. How requirements are identified is
     the specification's own format (`seam-specs`, unreviewed, proposes
     identifiers such as `CLAIM-7` and a coverage check in the specs
     repository's CI). A specification without a suite, which is the ledger
     format under `specs/` until it moves, is held by the coupling checks
     and contract tests it has.
  2. The draft as written, for every specification. `specs/` would fail it
     on merge until its sections are tagged to tests.
  3. Leave it out of the contract; each specifications repository decides.
- **a15. The smaller departures from the draft, confirmed together.** Each
  is "the contract says this where the draft says that".
  1. *(recommended)* All seven as proposed:
     - (a) *Host table.* Draft: "which machines" is local to each
       component. Contract: one host table shared by every component; a
       component's profile lists the host names its work uses
       (Decision 12).
     - (b) *Any-OS work.* Draft: it "goes to whichever taker is free".
       Contract: a `<capability>-any` host name belongs to one machine, as
       `hosts.yaml` gives it today (FLOW-04).
     - (c) *Relay routing.* Draft, section 5: a routing list for the relay
       fork. Contract: no routing in the rule text; routing is the host
       table. The draft sends QEMU work to the Windows desktop, and
       `hosts.yaml` has `qemu-any` on the MacBook.
     - (d) *The local column.* Draft: for change reporting, "where it is
       posted" is local; for reconciliation, "the dates and owners".
       Contract: reports are kept with the queue (the draft's own decided
       point), so nothing about reporting is local; for reconciliation the
       local part is what the component releases and consumes, and the date
       is the priority Jeff gives the item.
     - (e) *When a process change reaches a loop.* Draft: a loop "re-reads
       its configuration before taking its next feature task". Contract: an
       unattended run takes the tools, skills and configuration from trunk
       when it starts and keeps them to its end; rule text reaches a branch
       when it merges trunk (LOOP-09).
     - (f) *Where a component writes its side.* Draft: "the component's
       README and loop configuration". Contract: the README and the agent
       context file link to the contract, and the profile is the loop
       configuration.
     - (g) *No pinning.* Draft: adoption is "by reference, never by
       copying". Contract: the reference is to the contract's trunk and
       cannot be pinned to a version.
  2. The draft's wording for the ones Jeff names.
- **a16. How a component's invariants are written, and where prefixes are
  registered.**
  1. *(recommended)* As rules in LINT-01's format, under prefixes the
     component names in its profile. A prefix is unique across components,
     and the design review of a profile judges that. A component may also
     cite another component's rule by ID as binding (the relay's M1 design
     does, with GLOBAL and TEST rules). A prefix changes owner in the items
     that move its rules, with no change to the contract.
  2. The same, with every prefix registered in a table in LOOP-08. Each new
     prefix, and the move of RELAY at M6, then needs a loop process item.
  3. A component may keep its invariants as a table of each invariant and
     the check that enforces it, with no rule IDs of its own (what the
     relay's M1 design keeps as `docs/invariants.md` until its M1b). The
     judgment tier would have no rule index to read there.
- **a17. Repositories that have not adopted the contract, and where a loop
  process item is queued.**
  1. *(recommended)* A repository is a component once it has done LOOP-08's
     steps. Before that, an item in a component's queue may work in it; the
     worker is bound there by FLOW-08, FLOW-16, FLOW-17 and FLOW-18, and the
     item's design says which of its component's invariants apply (as
     `seam-specs` a4 does). The loop repository is a component from
     `loop-repository`'s merge: loop process items are queued there, and
     their `holds` name components. The specs repository and the Hermit
     forks adopt when Jeff files the item that does it.
  2. Every repository in the umbrella's component table adopts (a profile
     and a queue) before any work in it. S1 would wait for P1.
  3. The contract says nothing; each such item states its own rules, as
     `seam-specs` a4 does now.

## Design

### The contract's structure

One document, `docs/development/loop-contract.md`, with a short preamble
(what a component is, how a cited rule's severity is read), a table of where
the loop's parts are today, and nine rules. Its full proposed
text is under **Rule changes**.

| Rule | Title | Draft section | Detail it cites |
|---|---|---|---|
| LOOP-01 | Shared rules, a local profile | 1, and 8 steps 1 and 2 | Every FLOW and LINT rule a profile term serves; the term table names them |
| LOOP-02 | Design-first always, spec-first at a seam | 2 | FLOW-01, FLOW-02, FLOW-06, FLOW-07, FLOW-19 |
| LOOP-03 | Checks: four classes, two tiers, run at the gates | 3 | LINT-01 to LINT-04, LINT-06, FLOW-03, FLOW-08, FLOW-09 |
| LOOP-04 | One queue per component, one order | 4 | FLOW-03, FLOW-04, FLOW-05, FLOW-07, FLOW-09 |
| LOOP-05 | Workers are scheduled, routed and identified | 5 | FLOW-04, FLOW-09, FLOW-13, FLOW-18 |
| LOOP-06 | Every run reports in one place per component | 6 | FLOW-05, FLOW-08, FLOW-13, LINT-04, LINT-05 |
| LOOP-07 | Components couple only through releases | 7 | LOOP-01, LOOP-02 |
| LOOP-08 | Adopting the contract | 8, and the decided point on where the contract lives | FLOW-02, FLOW-08, FLOW-12, FLOW-16, FLOW-17, FLOW-18, FLOW-19, LINT-01 |
| LOOP-09 | The contract changes only through its owner | The decided point on who gates a change | FLOW-03, FLOW-05, FLOW-08, FLOW-14, FLOW-19 |

The contract and its detail are the *loop rules*: LOOP-\*, FLOW-\*, LINT-\*.
Everything else in `docs/development/` is a *project invariant* of
`buildwarden/buildwarden`. A component's *profile* is its own document
(here `docs/development/loop-profile.md`, the rule PROFILE-01) that fills
the terms LOOP-01 lists.

### The draft against the running loop

"The running loop" is `main` at `b903f65` and the `workflow` branch at
`174a6cc`.

#### The draft's blanks, filled

| Draft | Asks for | What the loop does today |
|---|---|---|
| 2, "fill in" | Sectioning or tagging rules that map specification sections to tests or features | There is no tagging of sections to tests. What exists: (a) the layout `specs/<dir>/v<N>.md` with `v<N>.schema.json` beside it, one pair per major version (RELAY-05); tier 1's `schema-doc` check fails a commit that changes one without the other (`tools/tier1/rules.yaml`, `schema_doc`). (b) Tier 1's `seam-contract` check requires a change to a versioned seam to change its contract document (GLOBAL-08, EXEC-04); its `seams` list is empty, so it checks nothing until S1 fills it. (c) Contract tests for the seams (TEST-05) and `specs/specs_test.go`. (d) A design maps a feature to seams through its `seams` front-matter (FLOW-02), and to rules through `rules`. `seam-specs` (unreviewed) proposes requirement identifiers that suite cases name, and a check in the specs repository's CI that every requirement is covered. That is the draft's specification invariant in the form a specification with a suite can hold (LOOP-03, class 2; a14). |
| 3, research task | The main loop's current checks, one per line | The list under **The checks the loop runs today**. |
| 4, research task | The queue's location and format | The orphan branch `workflow` of `buildwarden/buildwarden`: `queue/<item>.yaml` (state, steps with affinity and lease, budget, questions, history), `config.yaml` (budgets, limits, lease length, runaway factor), `hosts.yaml`, `backlog/native-gaps.yaml`, generated `QUEUE.md`. The schema is that branch's `README.md`; `wf` (`tools/workflow/cmd/wf`) is the only writer where a command exists (FLOW-03, FLOW-04). |
| 5, "fill in" | Schedule cadence | The cloud loop runs once a day (FLOW-13; at 02:55 Pacific according to a comment in `install-macos.sh`, the scheduled task itself being outside the repository) and takes up to `run_budget.steps` steps, 24 today. Each native machine has a scheduled session that starts an agent only when `wf gate` finds work (FLOW-09): `macbook` daily at 04:40 local (launchd, which runs a missed slot on wake), `windows-desktop` at 04:10 and every 6 hours (Task Scheduler). Each spends `native_budget.steps`, 8 today. |
| 5, "fill in" | Taker identity or credential model | A worker is `cloud`, `human` or `host:<name>`, with a `/<session>` suffix (`cloud/<UTC timestamp>`, `human/jeff`, `host:macos-vz/<timestamp>`). The identity is in every lease and history line (FLOW-04). A claim is a lease pushed to the `workflow` branch; a rejected push loses the claim; an expired lease (120 minutes) may be reclaimed. Credentials are the session's or the machine's own git access, set up outside the repository. A scheduled native session is a headless agent under a fixed allowlist (`tools/workflow/native/settings.native.json`), run as the logged-on user (a launchd agent on macOS; on Windows a task that is not elevated). No secret goes into a prompt (FLOW-18). |
| 6, decided | Where iterations report | Today: `digest/<YYYY-MM-DD>.md` on the `workflow` branch, one section per run; the tier-2 report per branch head in `reports/<item>/<sha12>.{md,json}`; each machine's last scheduled run in `native/status/<machine>.yaml`. All of it moves with the queue (`loop-repository`). |

#### The checks the loop runs today

Grouped by the draft's four classes, from `main` at `b903f65`. "T1" is a
deterministic check, and it blocks unless its line says otherwise. "T2" is
the judgment tier, report-only today (LINT-07). This list answers the
draft's research task. It is not copied into the profile, which names where
each class's checks are defined so that it does not go stale when a feature
adds one (PROFILE-01).

**Build**

- `make build` on macOS and Ubuntu, then `dist/warden --help` (CI `build`).
  It cross-builds the relay and the executor for linux/amd64 and
  linux/arm64. `rules.yaml` gives "the CI matrix" as the owner of
  GLOBAL-07's and ORCH-05's automated parts. T1.
- `go mod tidy` leaves `go.mod` and `go.sum` unchanged, in the root module
  and in `tools/tier1` (CI `tidy`). T1.
- `mdbook build docs/book` (CI `book`). T1.
- The `relay-image` workflow builds the relay's OCI image on pushes to
  `main` that reach the relay, and on `v*` tags. It does not run on pull
  requests.

**Specification**

- `schema-doc`: a schema and its document change together (RELAY-05). T1.
- `seam-contract`: a seam change changes its contract document (GLOBAL-08,
  EXEC-04). T1, empty list today.
- `surface-docs` and `surface-removed`: a change to the user-facing surface
  changes its reference docs, and removes nothing without them (UX-01,
  ORCH-03, UX-03). T1.
- `bootstrap-surface`: a change to the bootstrap surface changes
  `bootstrap.md` (BOOT-07). T1.
- Contract and round-trip tests (TEST-05), `specs/specs_test.go`, and the
  keytool comparison on three operating systems (CI `keytool`). T1.
- Field-level parity of a document and its schema (RELAY-05's judgment
  half). T2.

**Safety**

- `golangci-lint` over the root module, `tools/workflow` and `tools/tier1`
  (`make lint`; CI `lint`), including `forcetypeassert` (GLOBAL-03). T1.
- Tier 1's analyzers: `exported-doc` (GLOBAL-01), `gofmt-touched`
  (GLOBAL-02), `error-wrap` (GLOBAL-03), `executor-no-cgo` (EXEC-01),
  `large-literal` (EXEC-05), `ledger-single-writer` (RELAY-01),
  `relay-diskless` (RELAY-07). T1.
- `make test` (TEST-01): the root module, `tools/workflow` and
  `tools/tier1`, on Ubuntu (CI `test`); `tools/tier1`'s tests also on macOS
  and Windows (CI `tier1-test`). T1.
- `tests-with-change` (GLOBAL-04). T1, a hint: it is reported and never
  fails (`tools/tier1/coupling/checks.go`).
- `make integration-test` on PRs that reach the container driver or a
  shared package, weekly on `main`, and on demand (TEST-02, tier 1 of the
  test tiers; workflow `integration`). T1 where it runs. `main.yml` names
  `ci-ok` as "the one check to require", and `integration` is a separate
  workflow, so it blocks through the loop's Publish gate, which needs the
  PR's checks green (FLOW-11). Whether branch protection also requires it
  was not read.
- The `firecracker` workflow: TEST-02's tier 2 for the firecracker driver on
  a hosted runner with KVM, on PRs that reach its paths and on demand. Its
  header says it is not required for merge until it has passed on
  `feat/firecracker-driver`. T1, not yet blocking.
- Native validation on the machines in `hosts.yaml` (TEST-02's tier 2,
  FLOW-09). Deterministic; blocking or tracked per design.
- Security-boundary changes carry a security note (GLOBAL-06). T2, and the
  human's acknowledgement (FLOW-08).
- Every `agent-judgment` rule and the judgment half of every hybrid rule,
  judged on the diff by the `wf-review` agent (LINT-02, LINT-06). T2.

**Process**

- `rule-index`: every rule has a severity and a check, and every
  `automated` rule is mapped in `rules.yaml` (LINT-01, LINT-02). T1.
- `design-front-matter`, "code changed with no design in the diff", and
  `drivers-claim`, which compares a design's `drivers` with the trees the
  diff reaches (FLOW-01, FLOW-02, FLOW-12, ORCH-01). T1.
- `suppression`: a `Tier1-Allow` trailer carries a reason (LINT-05). T1.
- `wf`'s done-when checks at each phase gate (FLOW-03): sections filled,
  the design review's approval and its delta (FLOW-19), the rule-change
  clearance (FLOW-14), the tier-2 report present (LINT-04, LINT-05), the PR
  green (FLOW-11), the human's merge (FLOW-08), the branch gone (FLOW-16).
  T1.
- `wf validate`: the queue's schema, FLOW-05's limits, `hosts.yaml`, the
  native status files. T1.
- `make enforcement`: `main`'s tier 1 against a branch that changes the
  enforcement (FLOW-14). T1.
- `tools/workflow/flow18-check.sh`, before every push by a loop session, on
  every push and PR, and daily (FLOW-18; workflow `flow18`). T1.
- `make deadcode` at Publish (FLOW-15). T1 to run; its output is a report
  that Publish judges, not a gate.

**The release gate.** A `v*` tag push runs the `release` workflow. Its
`tier1` job runs tier 1 over everything since the previous release tag and
checks the front-matter of every design, and the `release` job needs it:
that is the gate's deterministic half (LINT-03; `rules.yaml` names "the
release workflow's tier1 job" as LINT-03's owner). The rest of the gate as
`behavioral-linter.md` composes it is not wired to that workflow: the full
test matrix, the security suite for a release that trips GLOBAL-06 (TEST-08,
SEC-01 to SEC-08, not yet built), and the human's sign-off of a tier-2
report over the release diff.

#### Where the draft and the running loop disagree

| # | Draft | Running loop | Resolution | Question |
|---|---|---|---|---|
| 1 | Two classes of work, feature and process. Process work "never enters the feature queue", "goes to the top of the queue and runs before any further feature work". | One queue; `kind: feature` or `process`; four classes in order: in implementation, process items, fixes, new functionality (FLOW-05). A process item stops other workers only through declared `holds`. | FLOW-05 stands. The draft's classes are the two kinds. | a4 |
| 2 | "A loop that picks up a process change re-reads its configuration before taking its next feature task." | Per run, and in two parts. `wf`, the skill and `config.yaml` are taken when a run starts and kept to its end: the native session resets its checkout to `origin/main` and builds `wf` from it (`native-tick.sh`); the cloud tick builds `wf` from the checkout it starts in and fetches, without resetting (`workflow-tick` step 1; every run ends by switching back to `main`). Rule text and tier 1 are read from each feature branch's own tree (`wf-design`, `wf-review`, `make lint`), so a branch forked before a rule change follows the old text until it merges `main` (FLOW-05's periodic merge, `f6-merge-main`). Nothing a process item changes reaches a run until the human merges it (FLOW-03, FLOW-14). | LOOP-09 states both parts. A change that cannot wait for the next run or the next merge of trunk declares `holds`. | a15 (e) |
| 3 | Invariants in four classes that "run on every loop iteration and block merge on failure". | Two tiers by how a check is made (LINT-02). Tier 1 blocks; tier 2 informs and the human signs off (LINT-03); tier 2 is report-only (LINT-07). Checks run at phase gates and in CI, on the diff (FLOW-03, LINT-04). Expensive tiers are scoped, and a native gap may be tracked (TEST-02, TEST-03, FLOW-09). | Classes and tiers are two axes (LOOP-03). Only tier 1 blocks. | a6 |
| 4 | Iteration output: item and class, spec diff, code diff summary, invariant results in all four classes, what is parked or blocked. | A run takes several steps on several items. The digest has a section per run (Moved, Blocked on Jeff, Follow-ups filed, Design reviews skipped, Waiting on a host, Spent, Notes); the tier-2 report is per branch head. No class is printed, and a specification change is not reported apart from code. | The run's digest section is the report (LOOP-06), with three additions. | a7 |
| 5 | Spec-first for all behaviour: "a behaviour change with no matching spec change fails review". | Design-first (FLOW-01). Coupling to written documents exists per kind of behaviour: schemas (RELAY-05), seams (GLOBAL-08), user-facing surfaces (UX-01), the bootstrap model (BOOT-07), component flows (FLOW-15). | Design-first always; spec-first where a seam has a written specification (LOOP-02). | a8 |
| 6 | "A spec section an agent cannot satisfy is marked as an open question in the spec." | A finding returns the item to Design (FLOW-06). Questions live in the queue and the digest (FLOW-07). | The queue holds the question; the answer may be a specification change. | a8 |
| 7 | "Specs live in the main BuildWarden project's specification directory" for now. | The umbrella's Decision 3 and a10, five days later: `buildwarden/buildwarden-specs`, created by S1. `specs/` here holds the ledger format until it moves. | The umbrella. The contract leaves the location to each component's profile. | none (decided) |
| 8 | The contract lives in its own repository, "likely" the sessions repository. | The umbrella's Decision 13: its own public repository; the sessions repository is private. | The umbrella. | none (decided) |
| 9 | A reconciliation item is "process work" at the top of the consumer's queue. | A process item changes rules or machinery and has four phases with no Review (FLOW-03). The umbrella's M6: 'one "adopt relay version X" item per driver in that driver's queue'. | A feature item, at the priority intake agrees. | a5 |
| 10 | "Nothing in one component's queue blocks another component's queue." | One set of workers serves every queue: one cloud run budget, two machines, one session per machine. The umbrella's P1 asks for "holds per component". | True of items: no item needs a step in another queue (LOOP-04). Workers are shared, and a process item's `holds` are scoped per component (`loop-repository` builds it). How a worker divides a run between queues, and which limits and budgets count per component, are left open for that item (hand-off, point 5). | a1 |
| 11 | Task takers: "which machines" is local to each component. | `hosts.yaml` gives each host name to exactly one machine, for the whole loop (FLOW-04). | The host table is shared; schedules and credentials are each machine's (Decision 12, LOOP-05). | a15 (a) |
| 12 | Routing for the relay: Firecracker and QEMU work to the Windows desktop. | `hosts.yaml`: `qemu-any` is on `macbook` (the qemu driver runs only on aarch64 hosts); `linux-kvm` on `windows-desktop` is experimental. | Routing is `hosts.yaml`, not contract text. The relay's profile names host names, not machines. | a15 (c) |
| 13 | "Each taker ... records which repo and which queue item it took." | The lease and history record the worker and the item. There is one repository, so none is recorded. | LOOP-05 requires the component once there is more than one; the field is `loop-repository`'s. | a1 |
| 14 | A change to the contract is gated on Jeff only. | Both of FLOW-14's routes end in Jeff's design review and merge, but the second lets a feature's design carry a rule change. | Loop rules: process items only (LOOP-09). | a10 |
| 15 | Section 7 names the relay's consumable by "the provisioning protocol, the connector protocol". | The decisions table's rows 9 and 11, the same day: no new protocol; the existing claim and collector protocols (GLOBAL-08). | The contract names no protocol. A consumable lists the specifications it implements (LOOP-07). | none (decided) |
| 16 | Section 3, class 2: "every behaviour in the spec has a test; every test maps to a spec section". | No mapping of specification sections to tests (the blanks table, first row). `seam-specs` proposes one for the specs repository. | Kept as class 2 for a specification that has a conformance suite; not required of one without (Decision 16, LOOP-03). | a14 |
| 17 | Section 5: "any-OS work goes to whichever taker is free". | A `<capability>-any` host name belongs to exactly one machine (`hosts.yaml`, FLOW-04): `qemu-any` to `macbook`, `container-any` to `windows-desktop`. | FLOW-04 stands (LOOP-05). | a15 (b) |
| 18 | Section 1, local column: change reporting, "where it is posted"; reconciliation, "the dates and owners". | Reports are on the `workflow` branch with the queue, which is also the draft's own decided point. A reconciliation has no date; an item has a priority. | Nothing about reporting is local; for reconciliation the local part is what the component releases and consumes (LOOP-01). | a15 (d) |
| 19 | Section 8, steps 1 and 2: link from "the component's README and loop configuration"; fill the local column "in the component's own loop configuration". | No file is called the loop configuration. Agents start from `AGENTS.md`; `config.yaml` holds budgets and limits for the whole loop. | The README and the agent context file link to the contract; the profile is the component's loop configuration (LOOP-08). | a15 (f) |
| 20 | "A component adopts the contract by reference, never by copying." | Nothing yet adopts. | By reference to the contract's trunk, which cannot be pinned (LOOP-08). | a15 (g) |

Where the draft and the loop agree, the contract cites the rule. The final
scheduler setup on each machine is the owner's, by hand
([`native-host-loop`](native-host-loop.md), `install-macos.sh`,
`install-windows.ps1`). A blocked item names what blocks it (`wf next`'s
`blocked` lines, FLOW-07). Feature work is ordered by the queue's priority
and agents do not reorder it (FLOW-05), with one exception that a rule
states: the session that finds a regression `main` already has files its
fix at the top of the fixes, attended or not, and `wf new` renumbers the
items below it (FLOW-09, the `workflow-intake` skill); the human may move
it. LOOP-04 says both.

One gate is not in any rule. The cloud tick takes no step until the human
has confirmed the priority order (`priority_confirmed` in `config.yaml`,
`workflow-tick` step 1). No FLOW rule states it, and the scheduled native
session does not read it. The contract therefore does not state it either;
whether to make it a rule that every unattended worker honours is listed
for `loop-repository` (hand-off, point 5).

### Design-first and spec-first: which applies when

A *seam* is a boundary between components that the component's rules name.
Here these are GLOBAL-08's versioned seams, which a design declares in
`seams` (FLOW-02). A *specification* is a written document that a second
implementation of a seam is tested against. Today one seam has one: the
ledger format and its schemas (`specs/`). The umbrella's seams 1 to 8 get
theirs as S1 to S3 write them, and until then the Go code on `main` defines
them (the umbrella's Archive summary). Seam 9, the driver contract, is a Go
API and gets none. Nor do `bootstrap-model`, `windows-guest-seed` and
`image-manifest`, which designs declare in `seams` today.

| The item changes | Order | Rules |
|---|---|---|
| Behaviour a specification defines: a message, field, ordering, error, limit or format at a seam | Design, design review, then in Implement the specification change first and the code second, the code tested against the changed specification | FLOW-01, FLOW-02 (`seams`), FLOW-19, LOOP-02 |
| An implementation of behaviour already specified (the Rust relay's milestones; a second collector) | Design, citing the requirements it implements. No specification change. The code is tested by the seam's conformance suite. | FLOW-01, LOOP-02 |
| A declared seam that has no written specification (the driver contract; the bootstrap model; a seam S1 to S3 have not yet written) | Design, design review, then code. The seam's contract document, where the component's rules name one, changes in the same change. The design says the seam has no specification and what defines it. | FLOW-01, FLOW-02 (`seams`), FLOW-19, LOOP-02; here GLOBAL-08, EXEC-04, BOOT-07 |
| Behaviour inside one component, with no seam (a driver's lifecycle, the CLI, a tool) | Design, then code. The component's reference documents change in the same change. | FLOW-01; here UX-01, FLOW-15 |
| Nothing that carries a design decision (a dependency bump, a typo, a CI-only change) | Exempt, labelled `no-design` | FLOW-01 |
| The loop's rules or machinery (a process item) | The human's design and its review, then the implementation | FLOW-03, FLOW-14, LOOP-09 |

Design always comes first because the design review is where a seam change
is decided (FLOW-19 goes over "the components, seams and flows touched").
The specification change is how that decision is written down for every
implementation, so it is made before any code relies on it. Where the
specification is in another repository, the order across the two is the
component's to state in its profile; `seam-specs` proposes this
repository's (a specifications PR, a pin to its commit while the work is in
progress, the release, then a pin to the tag).

A design that declares `seams` fills a new template section,
**Specification changes**. For each seam it gives the specification
document and the requirements the item changes, and where the change is
made; or it says that the seam has no written specification and what
defines it. A run that changes a specification reports it
apart from the code (LOOP-06), and the PR body lists it. That is what the
draft's "spec diffs are first-class review artifacts" becomes.

### The component profile

LOOP-01 lists the terms the detail uses that a component defines. This
repository's profile, `docs/development/loop-profile.md`, fills each one;
its full proposed text is under **Rule changes**. It is a document that
agents and reviewers read. `wf` does not read it yet: making `wf` read a
profile is `loop-repository`'s (see the hand-off table for where each term
is hard-coded today).

The profile sits in `docs/development/`, so a change to it is a rule change
(FLOW-14). That is deliberate: the gate commands and the rule paths are
what the loop enforces here. The change is declared under the profile's own
rule, PROFILE-01, which this repository owns, so a feature's design may
carry it (Decision 14).

Two things keep that from becoming a cost on every feature. First, an entry
points to the project file that holds a fact and does not restate it: the
seam and driver lists are `tools/tier1/rules.yaml`'s, the host names are
`hosts.yaml`'s, the test tiers are TEST-02's. Second, the entries that do
state a fact are the ones that exist today only as examples inside FLOW and
LINT rules: the fast gate's commands, what the sweep may keep, the release
entry. For those the profile becomes the component's own statement. A
feature that needs one changed amends PROFILE-01 and not the loop rule,
which keeps the umbrella's Decision 18 workable for program items once loop
rules are closed to features (Decision 10).

**What `seam-specs` and `driver-contract` owe the profile.** Both are in
Design, unreviewed, and either may merge before or after this item. If this
item merges first, `seam-specs` changes two entries, Specifications (the
specs repository and its pin) and Consumables (the specifications release
this repository consumes), so its design needs a PROFILE-01 entry in
`rule_changes` before its review. `driver-contract` changes none that this
design can see: its new seam name and its rule changes are in files the
profile points to. If it needs its public kit packages in the sweep's kept
list, that is a PROFILE-01 entry too. If either merges first, this item's
implement step writes the profile against `main` as it is then
(Implementation plan, task 2). This design does not edit theirs; the point
is for their design reviews.

A term a component does not have is filled as `none`, and a rule read
through a `none` term does not apply there (a component with no user-facing
surface owes no reference docs). A term left unfilled is a finding; the
reader does not fall back to `buildwarden/buildwarden`'s entry.

### The split, rule by rule

**Project invariant** rules stay in `buildwarden/buildwarden`. **Loop**
rules move to the loop repository with `loop-repository`. 61 existing rules
and the new PROFILE-01 are project invariants; 28 existing rules and the 9
new LOOP rules are loop rules.

| Rule | Title | Side | Why, where it is not obvious |
|---|---|---|---|
| GLOBAL-01 | Comments | Project | |
| GLOBAL-02 | Formatting: gofmt on touched lines | Project | |
| GLOBAL-03 | Errors | Project | |
| GLOBAL-04 | Tests land with the change | Project | |
| GLOBAL-05 | Reproducibility | Project | |
| GLOBAL-06 | Security-boundary changes are high-severity | Project | FLOW-02, FLOW-08 and FLOW-20 cite it, but the boundaries are this product's. The profile names it as this component's boundary rule. |
| GLOBAL-07 | Cross-platform by default | Project | |
| GLOBAL-08 | Component independence | Project | It is about the product's components. The contract's "component" is a repository with a queue, defined in LOOP-01. |
| GLOBAL-09 | Least surprise | Project | They bind `tools/` and `.claude/` too, and the design template's **Defaults and overrides** section serves them. They are still design principles of this codebase (Decision 11). The loop repository cites them by ID if Jeff wants them to bind `wf` there. |
| GLOBAL-10 | Explicit over implicit | Project | As GLOBAL-09. |
| GLOBAL-11 | Minimal magic | Project | As GLOBAL-09. It cites FLOW-06, by ID. |
| RELAY-01 | Single-writer ledger | Project | The umbrella moves RELAY-01 to RELAY-08 to the relay or specs repository at M6. Until then they are this repository's. |
| RELAY-02 | Bytes sent match the ledger | Project | |
| RELAY-03 | Keys are memory-only | Project | |
| RELAY-04 | Three-mode parity | Project | |
| RELAY-05 | Versioned spec and schema docs | Project | This is the nearest thing to the draft's specification handling, but it states the ledger's layout. LOOP-02 is the shared rule; RELAY-05 is this component's detail of it. |
| RELAY-06 | Hot-path performance | Project | |
| RELAY-07 | Diskless relay | Project | |
| RELAY-08 | Configured through its claim | Project | |
| ORCH-01 | Driver parity | Project | |
| ORCH-02 | Inputs are read-only | Project | |
| ORCH-03 | Config-surface discipline | Project | |
| ORCH-04 | Transient resources are torn down | Project | |
| ORCH-05 | Windows host | Project | |
| EXEC-01 | Pure Go, no cgo | Project | |
| EXEC-02 | Minimal-environment robustness | Project | |
| EXEC-03 | Trust-install parity | Project | |
| EXEC-04 | Versioned public contract | Project | |
| EXEC-05 | Embed large assets | Project | |
| BOOT-01 | Isolation precedes execution | Project | |
| BOOT-02 | The executor is the only out-of-band input | Project | |
| BOOT-03 | One start, then hands off | Project | |
| BOOT-04 | Build inputs come from the relay | Project | |
| BOOT-05 | Setup is the executor's domain | Project | |
| BOOT-06 | Images carry hooks, not content | Project | |
| BOOT-07 | The model changes only by a signed comparison | Project | It reads like process and cites FLOW-14 and FLOW-19, but what it protects is this product's bootstrap model. The template's **Bootstrap comparison** section stays with it. |
| UX-01 | Docs-coupling | Project | The loop's phase 3 done-when cites it. The profile names it as this component's reference-docs rule. |
| UX-02 | Machine-readable output | Project | |
| UX-03 | Backward-compatible surfaces | Project | |
| UX-04 | Fail early, say what to do | Project | |
| UX-05 | Consistent CLI shape | Project | |
| UX-06 | Short flags | Project | |
| UX-07 | Minimal config, layered overrides | Project | |
| UX-08 | Fail fast on config | Project | |
| UX-09 | Exit codes | Project | |
| TEST-01 | Fast, universal gate | Project | It defines this component's fast gate (`make test`). The loop's rule is that a component has one (LOOP-03). |
| TEST-02 | Tiered tests | Project | It names the `workflow` branch's `hosts.yaml` and FLOW-09. The tiers and which CI workflow runs each are this component's; FLOW-09 is the loop's half. |
| TEST-03 | Not gated on owning every platform | Project | It names `tier2_gaps` and the native-gap backlog, both loop state, to say when this component's platform path counts as done. |
| TEST-04 | Flaky tests are defects | Project | FLOW-11 cites it. The profile names it as this component's flaky-test rule. |
| TEST-05 | Contract tests for the seams | Project | S1 amends it. |
| TEST-06 | Coverage is signal | Project | |
| TEST-07 | Parallelize | Project | |
| TEST-08 | Security suite on boundary-tripping releases | Project | |
| SEC-01 | Threat model | Project | |
| SEC-02 | Two arms | Project | |
| SEC-03 | Findings crystallize into the corpus | Project | |
| SEC-04 | Proof standard | Project | |
| SEC-05 | Synthetic surface | Project | |
| SEC-06 | Gate policy | Project | It cites LINT-03, by ID. |
| SEC-07 | Harness blast-radius | Project | |
| SEC-08 | Trigger and cadence | Project | A cadence, but of this product's security suite. |
| PROFILE-01 | This repository's loop profile | Project | New. It is the component's side of LOOP-01, and a feature may amend it (Decision 14). |
| LINT-01 | Every rule is machine-consumable | Loop | The rule format is what lets one tier-2 agent read any component's rules. The README's **Rule format** section goes with it. |
| LINT-02 | Two tiers | Loop | Its examples (`golangci-lint`, `go test`, GLOBAL-01, EXEC-01) are this component's; read through the profile. `tools/tier1/` itself stays here. |
| LINT-03 | Release-gate policy | Loop | Its wiring (GoReleaser on a tag push) is this component's release entry. |
| LINT-04 | Scoped to the change | Loop | |
| LINT-05 | Reproducible, auditable output | Loop | It cites UX-02 for the report's shape, by ID. |
| LINT-06 | Invocation | Loop | |
| LINT-07 | Report-only bootstrap | Loop | |
| LINT-08 | The linter is the doc set's drift check | Loop | |
| FLOW-01 | Design before implementation | Loop | Amended here to point to LOOP-02. |
| FLOW-02 | Designs declare their impact | Loop | The keys `drivers`, `surfaces` and `security_boundary`, their values, and the "shared package" derivation are this component's impact vocabulary (profile). |
| FLOW-03 | Phases and definitions of done | Loop | Amended here (LOOP-09). `make lint`, `make test` and the docs paths are profile terms. |
| FLOW-04 | Steps declare who may take them | Loop | |
| FLOW-05 | Depth first, in class order | Loop | |
| FLOW-06 | No silent redesign | Loop | |
| FLOW-07 | Questions, in two kinds | Loop | |
| FLOW-08 | Only a human merges | Loop | |
| FLOW-09 | Native validation | Loop | Host names are in the shared host table; which hosts a design may block on is the component's. |
| FLOW-10 | Publish consolidates | Loop | |
| FLOW-11 | CI repair is bounded | Loop | |
| FLOW-12 | Designs are archived | Loop | The archive stays in each component (`docs/design/`), and so does its `TEMPLATE.md`. |
| FLOW-13 | Unattended steps are bounded and visible | Loop | |
| FLOW-14 | Tooling is a thin layer over the rules | Loop | Amended here (LOOP-09). Its list of rule-change paths is this component's. |
| FLOW-15 | Publish sweeps duplication, dead code and the overview | Loop | Every specific in it is this component's: `make deadcode`, the kept-exported packages, `driver/vz`, `overview.md`. The loop's rule is that Publish sweeps; the profile says with what. |
| FLOW-16 | Merged branches are cleaned up | Loop | |
| FLOW-17 | Branches are pushed at checkpoints | Loop | |
| FLOW-18 | Sessions are recorded privately | Loop | One private repository serves every component. `.entire/settings.json` and the CI job stay in each component. |
| FLOW-19 | Design review | Loop | |
| FLOW-20 | The componentization program | Loop | It is temporary and names this repository's design by a relative link. It moves with the rest and is retired whole by Jeff's process item. |
| LOOP-01 to LOOP-09 | The Loop Contract | Loop | New. |

Unnumbered text follows its file. `behavioral-linter.md`'s "How the full
release gate composes" and `process.md`'s "Bootstrap order" move with the
loop rules; `global.md`'s "Deployment modes and the ledger's role" stays.
"Bootstrap order" is history by now; `loop-repository` may drop it.

### Rule IDs, cross-references, `rules.yaml` and tier 1

- **Rule IDs.** None changes, and none is reused. LOOP is a new prefix of
  the loop, and PROFILE a new prefix of this repository. The contract names
  the loop's three prefixes and each component names its own in its profile
  (LOOP-08), so an ID names one rule wherever it is cited.
- **Cross-references by ID** stay as they are. The project files cite six
  loop rules (FLOW-02, FLOW-06, FLOW-09, FLOW-14, FLOW-19, LINT-03), and
  the two loop files cite 22 project rules by ID. After the move those
  citations cross repositories and are still exact.
- **Cross-references by link or path.** This PR moves no file, so none
  breaks. What `loop-repository` has to change at the move, from a search of
  `main` at `b903f65` and of the `workflow` branch for `process.md` and
  `behavioral-linter.md`:
  - *Relative links, which break:* `AGENTS.md` (one),
    `docs/development/README.md` (two, in its Documents list) and
    `docs/development/testing.md` (one). The other direction breaks too:
    `process.md` links to `README.md`, `../design/TEMPLATE.md`,
    `../design/componentization.md`, `../book/src/concepts/overview.md` and
    three archived designs, and `behavioral-linter.md` links to
    `README.md`.
  - *Absolute links to this repository's `main`, which will return 404:*
    `CONTRIBUTING.md` (two) and the `workflow` branch's `README.md` (one).
  - *Plain paths in prose, to reword:*
    `docs/book/src/concepts/overview.md`, the header comment of
    `docs/design/TEMPLATE.md`, the README's opening section and ID scheme
    table, and three definitions that move with the loop (`workflow-tick`,
    `wf-review`, `wf-design`).
  - *Left as written:* 16 archived designs name the two files as plain
    paths, many inside `rule_changes.paths`. They record what was true when
    each merged.
- **`tools/tier1/rules.yaml`** does not change here. Its self-check asks
  for an entry only for a rule tagged `automated`, and the nine LOOP rules
  and PROFILE-01 are `agent-judgment`: what is mechanical about them is
  already a check of the FLOW rule they cite (`wf`), or of a component rule
  (`seam-contract`, `schema-doc`). At the move, `rules.yaml`'s LINT and FLOW entries have no
  rule file here to match; `loop-repository` decides whether tier 1 reads
  the loop's rule files from the loop repository or the loop carries its
  own map.
- **Tier 1's rule index** (`tools/tier1/rules`, `Dir = "docs/development"`,
  a non-recursive glob that skips `README.md`) parses the new
  `loop-contract.md` like any rule file: nine headers, each with its
  severity and check. `loop-profile.md` adds one, PROFILE-01.
- **The README index** is restructured, not rewritten, and no section
  moves in this PR: the ID scheme table gains the `LOOP-` and `PROFILE-`
  rows and an Owner column, and the Documents list is split into "Project
  invariants" and "Loop rules". The Rule changes section gives each README
  section its side and what `loop-repository` does with it.
- **`wf`'s checks** read `docs/development/` as a prefix
  (`tools/workflow/check/rulechange.go`, `review.go`), and CI's `changes`
  job matches `docs/development/*`. Both cover the two new files with no
  change.

### What this item's PR changes

| Path | Change | Declared as |
|---|---|---|
| `docs/development/loop-contract.md` | New: the contract, LOOP-01 to LOOP-09 | `rule_changes`, LOOP-01 to LOOP-09 |
| `docs/development/loop-profile.md` | New: this repository's profile, one rule | `rule_changes`, PROFILE-01 |
| `docs/development/README.md` | The two parts, the `LOOP-` and `PROFILE-` rows and the Owner column | `rule_changes`, LOOP-01 and PROFILE-01 |
| `docs/development/process.md` | FLOW-01 (a paragraph added), FLOW-03 (one sentence), FLOW-14 (two sentences) | `rule_changes`, FLOW-01, FLOW-03, FLOW-14 |
| `docs/design/TEMPLATE.md` | A **Specification changes** section | `rule_changes`, LOOP-02 |
| `.claude/skills/workflow-tick/SKILL.md` | The digest's three additions, LOOP-06 cited; the Never list's exception for an approved rule change excludes loop rules (LOOP-09) | Machinery, a process item's to change (FLOW-14) |
| `.claude/skills/workflow-native/SKILL.md` | The same three additions in the unattended digest section; the same qualifier in its Never list | Machinery |
| `.claude/skills/workflow-human/SKILL.md` | The design review's walkthrough, step 3: go over **Specification changes** (LOOP-02), and refuse a feature's `rule_changes` entry that names a loop rule (LOOP-09) | Machinery |
| `.claude/skills/workflow-intake/SKILL.md` | Step 3: a change to a loop rule is a process item, never a feature's rule change (LOOP-09). Steps 3 and 4: `wf new -hold` (a11) | Machinery |
| `.claude/agents/wf-publish.md` | The PR body's **Specification changes** section; at `f6-consolidate`, a design that declares `seams` and predates the section gets it (LOOP-02) | Machinery |
| `.claude/agents/wf-design.md` | Fill **Specification changes** when `seams` is not empty, even when the branch's template predates it (LOOP-02); a feature's design declares no loop-rule change (LOOP-09) | Machinery |
| `tools/workflow/cmd/wf/main.go`, `queue/queue.go`, `queue/queue_test.go` | `wf new -hold` (a11) | Machinery |
| `tools/workflow/queue/next.go`, `queue.go`, their tests | `wf next` honours `limits.alters_relationships` (a13) | Machinery |
| `.claude/settings.json` | As in `ac8b41a` (a12) | Machinery |
| `.entire/settings.json` | As in `ac8b41a` (a12) | Ordinary: `wf`'s clearance does not class it as machinery (`kindOf`) |
| `AGENTS.md` | Two rows: the contract and the profile | Ordinary |
| The `workflow` branch's `README.md` | `wf new -hold`; the limit `wf next` reads | Not in the PR. Edited on that branch after the merge, in the attended session in which Jeff merges; the PR body lists it under "After the merge" |

Nothing else. No rule file other than `process.md` is edited, `tools/tier1/`
and `tools/workflow/check/` are untouched, and no file moves.

### The three machinery changes (a11, a12, a13)

Each is a proposal the review can drop without touching the rest.

**`wf new -hold "<why>"` (a11).** Today `wf new` seeds a queued item whose
first step the loop may take at once. FLOW-20's freeze needs the opposite.
It was produced by hand. On 2026-11-11 five items each got a question `q1`
(`kind: blocking`, `status: open`, one option "resume it"), a human step,
and a `needs` on the step it gates: for the four queued items the step
`f0-answer-q1`, needed by `f0-branch-setup`; for the one in progress,
`f2-answer-q1`, needed by `f2-design-review`. `security-suite` had been held
on 2026-11-08 with a question `q2` and its status set back to queued.

With `-hold`, `wf new` writes the queued-item form: `q1`, the step
`f0-answer-q1`, the `needs` on `f0-branch-setup`, and a history line
`on hold (q1): <why>`. For a process item the step is `x1-answer-q1` and
`x1-design` needs it. Holding an item that is already in progress stays a
hand-edit; intake never needs it.

Nothing else in `wf` changes. `wf next` already skips a step whose need is
not done. For a held item that is still queued it prints nothing: a queued
item with no ready step is passed over without a `blocked` line (`next.go`,
`Next`). The hold shows in `QUEUE.md` under **Needs Jeff**, as any open
blocking question does, which is how the six items showed before they were
closed. `wf answer -question q1` already marks the question answered and
completes the step. `cmd/wf` has no test file, so the flag is only parsed
there; what it writes is a function of the `queue` package, tested in
`queue/queue_test.go`.

**The Entire hook configuration (a12).** `ac8b41a` changes one hook command
in `.claude/settings.json` (session start also reports an `entire` on PATH
that fails to run) and reorders two keys in `.entire/settings.json`.
`flow18-check.sh` reads `.strategy_options.checkpoint_remote`, which the
reorder does not touch. The PR takes the two files as that commit has them.
Only `.claude/settings.json` needs a process item; `.entire/settings.json`
travels with it because the two were generated together.

**The `alters_relationships` limit (a13).** `altersHolder` (`next.go`)
becomes a count of the other in-progress features past Design that alter
relationships, and the cloud is held only when that count has reached the
limit. The limit is `limits.alters_relationships`, or 1 when `config.yaml`
does not set it, read through one function that `Validate` uses too. Who is
counted stays as it is in both functions today: every in-progress item past
Design that declares `true`. A test covers a limit of 3 with two holders. See **Defaults and overrides**.

### What a second component needs in order to adopt

LOOP-08 is the rule. For the relay repository, concretely:

1. **A link, not a copy.** Its README and its agent context file link to
   the contract. Until `loop-repository` lands, the link is to
   `docs/development/loop-contract.md` on this repository's `main`;
   afterwards to the loop repository. The link follows the contract's trunk
   and is not pinned, so a change reaches every component at once.
2. **A profile**, every term filled or `none`. For the relay: Rust; gates
   that build its three crates for two architectures, lint and test; its
   designs under `docs/design/`; the seams it implements (claim, collector,
   relay guest API, ledger format) at a pinned specifications release;
   host names from the shared table for its VM work; consumables, the
   relay's release forms (the umbrella's seam 8).
3. **Its own invariants as rules** in LINT-01's format, under prefixes it
   names in its profile, and its safety invariants among them:
   `forbid(unsafe_code)` in the relay crates, a dependency audit, the
   no-filesystem and no-process lints (the kickoff's design constraints).
   `RELAY-` is `buildwarden/buildwarden`'s until M6 moves those rules, so
   the relay takes another prefix until then. It may cite this repository's
   rules by ID as binding beside its own (Decision 11).
4. **A design template** with the contract's core front-matter and
   sections, plus its own. Its `m1-foundation.md` is written from this
   repository's template and has the same keys.
5. **The repository set up as FLOW-08, FLOW-16 and FLOW-18 need:** trunk
   protected so only a human merges, head branches deleted on merge,
   sessions recorded only to the private sessions repository and the
   FLOW-18 check run on it.
6. **A queue and workers**, which need `loop-repository`: a queue for the
   component beside this one's, `wf` reading its profile, and the workers'
   access to it.

Steps 1 to 5 need nothing from `loop-repository`. Step 6 is P1's done-when:
"this repository's loop and the relay's loop each complete one item from the
loop repository".

**Against the relay's M1 design** (`feat/m1-foundation` at `13ce286`,
unreviewed, written against this design at `2c7ff0e`). It splits adoption
the same way. Its M1a does what waits for nothing: the invariants as CI
jobs, a plain table of them in `docs/invariants.md`, the FLOW-18 guard,
branch protection once the repository is public, and the stub pre-release
that is its first consumable. Its M1b waits for this item to merge (task
10: the link, the profile, the invariants rewritten as rules in LINT-01's
format, the design template) and for the loop repository (task 11: the
queue and the relay's first item). So step 5 and the content of step 3 are
M1a; steps 1, 2 and 4, and the form of step 3, are its task 10; step 6 is
its task 11. It differs from this design in two places, both for its task
10 to take up:

- *Routing.* It takes the relay's routing from the draft's section 5
  (Hyper-V, Windows and Linux-host work to the Windows desktop). Under
  LOOP-05 a profile names host names and the host table gives each to a
  machine; `hosts.yaml` has `qemu-any` on the MacBook (a15, c).
- *Registering a prefix.* It expects a prefix table in the contract, and a
  change through this repository's process to register the relay's. This
  design no longer has that table: the relay names its prefix in its own
  profile, and nothing in the contract changes (Decision 9, a16).

**What the contract must not assume about a component.** LOOP-08 lists
these in the rule itself:

- its language, build tool, test framework or linter (no `make`, `go`,
  `cargo` or `golangci-lint` in a LOOP rule);
- its directory layout, beyond a place for designs and a place for rules
  that its profile names;
- that it has drivers, user-facing surfaces, a book, an overview or
  security boundaries (each is a profile term that may be `none`);
- which agent tool runs it (FLOW-14 already names `.claude/` and `.kiro/`
  as examples);
- which CI system it uses, only that a deterministic check can block a PR;
- that it is public;
- that its specifications are in its own repository.

What it does assume: git, a trunk and feature branches, pull requests that
a human merges, Markdown rule files in LINT-01's format, designs with
front-matter, and one owner.

### What this item hands to `loop-repository`

**The line.** This item changes what the rules say, in place. That item
changes where things live and what the tools read. After this item merges,
nothing has moved and `wf` behaves as before except for a11 and a13.

**1. The finished contract text**: `docs/development/loop-contract.md` as
merged, whose "Where things are" table has a column for today and one for
after the move. Editing that table is the only change to the contract's
text the move needs.

**2. The split.** Three files that are wholly loop-side
(`loop-contract.md`, `process.md`, `behavioral-linter.md`) and the README's
loop-side sections, listed under **Rule changes**. Moving them is a move in
FLOW-20's sense.

**3. The list of what moves and what stays.**

| Moves to the loop repository | Stays in `buildwarden/buildwarden` |
|---|---|
| `docs/development/loop-contract.md`, `process.md`, `behavioral-linter.md`; the README's loop-side sections, as the Rule changes section lists them | The other eight rule files; `loop-profile.md`; the README's project-side sections |
| `tools/workflow/` whole: `cmd/wf`, `queue/`, `check/`, `native/`, `flow18-check.sh`, `enforcement-check.sh` | `tools/tier1/` (it imports `tools/workflow/check` through a `replace`, which the move has to resolve) |
| `.claude/skills/workflow-{tick,human,native,intake}`, `.claude/agents/wf-{design,plan,implement,review,guide,publish}.md` | `.claude/settings.json`, `.entire/settings.json`, `AGENTS.md`, `CLAUDE.md` |
| The `workflow` branch's content: `queue/`, `config.yaml`, `hosts.yaml`, `backlog/`, `digest/`, `reports/`, `native/`, `QUEUE.md`, `README.md` | `docs/design/` and its `TEMPLATE.md`; `.github/workflows/` |

**4. The profile's term list as the specification of what `wf` must stop
hard-coding.** Where each term is fixed in the tools today:

| Profile term | Hard-coded in |
|---|---|
| Gates | `check.go` (`make lint`, `make test`); `wf-implement`, `wf-review`, `wf-publish` |
| Component | `install-macos.sh` and `install-windows.ps1` (each checks that `origin` is `buildwarden/buildwarden`) |
| Trunk and branches | `check.go`, `review.go` and `rulechange.go` (`origin/main`); `enforcement-check.sh`; `native-tick.sh`; `queue/hosts.go` (the native gate's fingerprint of `origin/main` and each in-progress item's `origin/<branch>`) |
| Designs and template | `cmd/wf/main.go` (`docs/design/<item>.md`, `feat/<item>`); `check.go` (`docs/design/TEMPLATE.md`) |
| Impact vocabulary | `check.go`, `Front.Problems`: the required keys and the four `security_boundary` values |
| Reference docs and guides | `check.go`: `docs/`, `docs/book/src/guide/`, `docs/book/src/getting-started/`, `examples/`, `README.md` |
| Rule and machinery paths | `rulechange.go`, `kindOf`; `review.go` |
| Sweep | `wf-publish` (`make deadcode`, `overview.md`) |
| Sessions | `flow18-check.sh` (`buildwarden/buildwarden-sessions`) |
| Loop state | The four skills and `native-tick.sh` (`.dev/workflow`, `origin/workflow`) |
| Native assets | `native-tick.sh` runs `tools/relay-vm/hyperv/refresh-cache.sh`, a product script inside loop machinery |

**5. What the contract requires and only that item can build:**

- `holds` scoped per component (LOOP-04, LOOP-09; the umbrella's P1);
- the component recorded on every queue item and claim (LOOP-05);
- one host table for all components, and a digest per component (LOOP-05,
  LOOP-06);
- the loop repository as a component: its own queue, where loop process
  items sit, and its profile (LOOP-04, a17);
- how a component's checkout gets the skills, the agents and `wf` once
  they are in another repository, and how CI here runs `flow18-check.sh`,
  `enforcement-check.sh` and tier 1's use of `check`;
- where tier 1's generic checks (`rule-index`, `design-front-matter`,
  `suppression`) live, since the relay needs them and cannot use the Go
  analyzers;
- the links and paths listed under **Rule IDs, cross-references,
  `rules.yaml` and tier 1**.

The contract leaves these open, and that item's design decides them:

- how a worker divides a run between queues, and whether one run may work
  in two components (LOOP-04 orders each queue and says nothing across
  them);
- whether FLOW-05's limits (five in progress, the `alters_relationships`
  limit, the PR slot) and FLOW-13's budgets count per component or across
  them;
- how a run is made to start from trunk. `native-tick.sh` resets its
  checkout to `origin/main`; the cloud tick builds `wf` from whatever its
  checkout holds and does not check (LOOP-09);
- the priority-confirmation gate, which only the cloud tick reads: make it
  a rule every unattended worker honours, or leave it the cloud's.

**6. Follow-ups this item names and does not do:** the neutral rewording of
FLOW and LINT (a9), as the loop repository's first process item; and a11,
a12 or a13 if the review moves any of them there.

### What this item does not declare

- **`alters_relationships: false`.** FLOW-02 lists what alters
  relationships: a versioned seam, the bootstrap model, which product
  component owns a responsibility, the driver contract. This item changes
  none. It changes how component *loops* relate, and the umbrella already
  declared that for the program. The practical side: `wf` counts every
  in-progress item that declares `true` and is past Design toward FLOW-05's
  slot, process items included, so `true` here would hold the slot against
  `seam-specs` and `driver-contract` while this item is implemented.
- **`seams: []`.** The umbrella numbers the Loop Contract as seam 10 and
  calls it "a document". FLOW-02's `seams` are GLOBAL-08's versioned seams.
- **No `holds`.** The PR amends three rule paragraphs, adds documents, and
  makes three small machinery changes. A run that starts after the merge
  has the new `wf` and skills, while a branch forked before it keeps the
  old rule text until it merges `main`. (Whether an agent definition is
  read when the session starts or from the branch's tree was not
  established; either way it is the new one once the branch merges
  `main`.) That mix is
  safe here because nothing new contradicts the old text. The FLOW-01
  paragraph and the template section add an obligation that `wf-design`
  meets even on an older template, and that Publish supplies for a design
  already approved (`wf-publish`, at `f6-consolidate`). The FLOW-03 and
  FLOW-14 sentences close a route that neither in-flight design uses: the
  `rule_changes` of `seam-specs` and `driver-contract` name only project
  rules. The digest additions are the tick's and do not depend on a
  branch's tree.

## Defaults and overrides

The item's `source` settles none of these; each is tied to the question the
review confirms it under.

| Default, override or fallback | What it is | Why it is the obvious choice (GLOBAL-09), and what overrides it (GLOBAL-10) | Question |
|---|---|---|---|
| `wf new` without `-hold` | The item is queued and the loop may take it, as today. | Unchanged behaviour. `-hold "<why>"` is given explicitly and is used as given: the item is held, and the reason is the question's text. There is no implied hold: intake decides under FLOW-20 and passes the flag. | a11 |
| `limits.alters_relationships` unset | 1 | FLOW-05's written limit is "at most 1". A value in `config.yaml` is used exclusively, by `wf next` and `wf validate` alike. Today `wf next` uses 1 whatever is set, and `wf validate` treats an unset value as no limit; both read the one value after this. | a13 |
| A profile term left unfilled | A finding (LOOP-01). | No fallback to `buildwarden/buildwarden`'s entry. Another component's `make lint` is not a predictable meaning for a Rust repository (GLOBAL-11). | a9 |
| A profile term filled as `none` | The rules read through it do not apply in that component. | Explicit, written by the component, and reviewed with the profile. | a9 |
| The contract a component follows | The contract's trunk. | "A rule changed here changes everywhere at once" (the draft). There is no override: a component cannot pin an older contract (LOOP-08). | a15 (g) |
| The kind of a reconciliation item | Feature. | It changes code. No override. | a5 |
| The tier of a new check | None. | A check without a tier is a finding (LOOP-03), as a rule without a check type is (LINT-01). | a6 |
| What a failed deterministic check does | It blocks. | A hint or a report is declared by name in the profile, and host-bound validation is blocking or tracked in the design (FLOW-09). Nothing is non-blocking by omission. | a6 |
| The severity of a cited detail | The cited rule's own. | A LOOP rule that cites FLOW-05, FLOW-13 or LINT-06 does not raise them from SHOULD. Raising one is a rule change of its own. | none: it changes nothing |

**Fallbacks.** None is added. One existing reading is dropped as a
fallback: `wf validate` treating an unset `limits.alters_relationships` as
"no limit" (a13).

**Merges.** None.

## Cross-driver impact

None. `drivers` is empty: no driver tree, shared package or driver document
changes (ORCH-01). The rules that bind drivers are all project invariants
and none is edited.

## Rule changes

Thirteen entries: ten rules added (LOOP-01 to LOOP-09 and PROFILE-01) and
three amended (FLOW-01, FLOW-03, FLOW-14). No severity or check of an
existing rule changes, and no enforcement code changes for any of them:
`tools/tier1/` and `tools/workflow/check/` are untouched. Tier 1's
`rule-index` check parses the ten new rules as it parses every rule, and
they need no `rules.yaml` entry because none is tagged `automated`.

### LOOP-01 to LOOP-09 (add), `docs/development/loop-contract.md`

**Why:** the contract every component's loop runs under has to be a rule
file that the loop reads, in the format the rest are in (LINT-01), before a
second component can adopt it. The nine rules are new statements; where the
running loop already has the rule, they cite it.

**Current wording:** none; the file is new.

**Proposed wording**, the whole file:

> # The Loop Contract
>
> The rules every BuildWarden component's development loop runs under. A
> *component* is a repository that has adopted this contract (LOOP-08) and
> has its own queue of work. `buildwarden/buildwarden` is the first, and
> `buildwarden/buildwarden-relay` is to be the second. A component adopts
> the contract by reference and never copies it, so a rule changed here
> changes for every component at once.
>
> The contract is this document and its detail: the process rules
> ([`process.md`](./process.md), FLOW-\*) and the behavioral-linter rules
> ([`behavioral-linter.md`](./behavioral-linter.md), LINT-\*). Together they
> are the *loop rules*. A component's own standards are its *project
> invariants*. They stay in its repository and are not part of the
> contract.
>
> A LOOP rule that cites a FLOW or LINT rule for a detail leaves that
> rule's severity as it is. The cited part is held at the cited rule's
> severity; only what the LOOP rule states itself is held at its own.
>
> See [`README.md`](./README.md) for the rule format.
>
> ## Where things are
>
> | | Today | Once the loop repository exists |
> |---|---|---|
> | The contract and its detail | `docs/development/` in `buildwarden/buildwarden` | The loop repository |
> | Queues, `config.yaml`, the host table, digests and reports | The `workflow` branch of `buildwarden/buildwarden` | The loop repository |
> | `wf`, the skills and the agent definitions | `tools/workflow/` and `.claude/` in `buildwarden/buildwarden` | The loop repository |
> | A component's profile, invariants, designs and checks | Its own repository | Its own repository |
> | Session recordings | `buildwarden/buildwarden-sessions` (FLOW-18) | The same |
>
> ---
>
> ### LOOP-01 — Shared rules, a local profile
>
> **Severity:** MUST
> **Check:** agent-judgment
>
> What is shared and what is local:
>
> | Concern | Shared (the loop rules) | Local (each component) |
> |---|---|---|
> | Specifications | When a specification changes before code (LOOP-02) | The specifications, where they are kept, their format, and how a release of them is pinned |
> | Checks | The classes, the tiers, when they run and what a failure does (LOOP-03) | The concrete checks and their implementations; the component's invariants |
> | The queue | The kinds of item, the phases, the order and what blocks what (LOOP-04) | The items |
> | Workers | Affinities, leases, budgets and the host table (LOOP-05) | Each machine's schedule and credentials; the host names a component's work uses |
> | Reporting | What a run records and where (LOOP-06) | Nothing |
> | Other components | Reconciliation (LOOP-07) | What it releases and what it consumes |
>
> Each component keeps one **profile**: a document in its repository,
> beside its invariants, that fills the local column. The profile is one
> of the component's own rules: it carries a rule header under a prefix the
> component owns, so a change to it is declared and approved like a change
> to any of the component's invariants (FLOW-14). It gives the component's
> entry for each of these terms:
>
> | Term | What the component says | Used by |
> |---|---|---|
> | Component | Its name and repository | LOOP-05, LOOP-06 |
> | Trunk | The branch only a human merges to | FLOW-03, FLOW-08, FLOW-17 |
> | Toolchain | Its language and build tools | LOOP-08 |
> | Rules | Where its invariants are, and the prefixes it owns | LINT-01, FLOW-14, LOOP-08 |
> | Designs | The design directory, the template and the sections the template adds | FLOW-01, FLOW-12 |
> | Impact vocabulary | The values its designs may declare, and the keys it adds | FLOW-02 |
> | Specifications | The seams it owns or implements, where each specification is, and how it pins a release | LOOP-02, LOOP-07 |
> | Fast gate | The lint and test commands every change passes | FLOW-03, FLOW-17 |
> | Checks | For each class, where its checks are defined, their tier, and any that do not block | LOOP-03, LINT-02 |
> | Test tiers | Its tiers of tests and where each runs | FLOW-09 |
> | Native hosts | The host names its work uses | FLOW-04, FLOW-09 |
> | Reference documents | The documents that change with user-facing behaviour, and its guides | FLOW-03 |
> | Sweep | Its dead-code command, what that may keep, and its overview document | FLOW-15 |
> | Boundary rule | Its rule for changes that need a security note | FLOW-02, FLOW-08 |
> | Flaky-test rule | Its rule for a failure that does not reproduce | FLOW-11 |
> | Rule and machinery paths | The paths whose change is a rule change, and the loop's machinery in its tree | FLOW-14 |
> | Release | How a release is cut and where its gate is wired | LINT-03 |
> | Consumables | What it releases, and what it consumes at which version | LOOP-07 |
> | Sessions | Its session-recording configuration | FLOW-18 |
>
> Where a FLOW or LINT rule names a path, a command or a rule of
> `buildwarden/buildwarden`, a component reads it as its own profile's
> entry for that term. Where the entry and the rule's example differ, the
> entry is the component's. An entry may name the file that holds a fact
> in place of restating it. A term the component does not have is filled
> as `none`, and a rule read through a `none` term does not apply there. A
> term left unfilled is a finding; it is never read as another component's
> entry.
>
> The agent part: the design review of a change to a profile judges that
> every term is filled or `none`; Review judges that the paths and commands
> a change relies on are the profile's.
>
> ---
>
> ### LOOP-02 — Design-first always, spec-first at a seam
>
> **Severity:** MUST
> **Check:** agent-judgment
>
> Every item has a design before implementation (FLOW-01). A *seam* is a
> boundary between components that a component's rules name and its
> designs declare (FLOW-02). A *specification* is a written document that
> an implementation of a seam is tested against. Where a seam has one, the
> specification is the source of truth for behaviour at the seam: an
> implementation is tested against it, never against another component's
> code.
>
> A design that declares a seam says, under **Specification changes**,
> which specification document and requirements the item changes, or that
> the seam has no written specification and what defines it.
>
> An item that changes behaviour a specification defines is also
> *spec-first*:
>
> - The design review decides the specification change the design lists
>   (FLOW-19).
> - In Implement the specification changes first. The code follows and is
>   tested against the changed specification. A change to behaviour that
>   a specification defines, with no matching specification change, is a
>   finding.
> - An item that implements behaviour already specified changes no
>   specification; its design cites the requirements it implements.
> - A specification an implementation cannot meet is not worked around in
>   code. The item returns to Design with the finding (FLOW-06), the
>   question is recorded in the queue (FLOW-07), and the answer is a change
>   to the specification or to the design.
> - A run that changes a specification reports that change apart from the
>   code (LOOP-06).
>
> Behaviour no specification defines is design-first only, at a seam or
> not. Its written record is the component's contract and reference
> documents, changed in the same change.
>
> Where a specification is kept, its format, and how a component pins the
> release it implements are the component's (LOOP-01), within LOOP-07.
>
> A component may check mechanically that a change at a seam changes its
> specification; that check belongs to the component's own rule, not to
> this one. The agent part: the design review judges the **Specification
> changes** list; Review judges that the diff changes no behaviour at a
> seam beyond what the specification change states.
>
> ---
>
> ### LOOP-03 — Checks: four classes, two tiers, run at the gates
>
> **Severity:** MUST
> **Check:** agent-judgment
>
> A component declares its checks in four classes:
>
> 1. **Build**: every declared target builds.
> 2. **Specification**: behaviour matches its written documents. LOOP-02's
>    coupling, conformance suites and contract tests are here. Where a
>    specification has a conformance suite, each of its requirements is
>    covered by a case that names it, or is listed with the reason it is
>    not machine-checked, and each case names the requirements it checks.
> 3. **Safety**: the lints, tests, audits and boundary rules the component
>    declares.
> 4. **Process**: the loop's rules held. The phase done-when checks
>    (FLOW-03) and the queue's validity are here.
>
> Every check is in one of two tiers (LINT-02). A deterministic check
> blocks: its failure stops the phase transition and the pull request
> (LINT-03). There are two exceptions, each stated where it applies and
> never assumed. A component may declare a deterministic check a hint or a
> report, by name in its profile; it then informs. Host-bound validation
> is blocking or tracked, as the item's design says (FLOW-09). A judgment
> check informs: its findings go to the human, who signs off by merging or
> dismisses one with a recorded reason (LINT-03, FLOW-08). No agent holds
> a veto over trunk, and none approves it.
>
> Checks run at the phase gates (FLOW-03) and in CI on the pull request,
> on the change and not on the whole repository (LINT-04). A check too
> expensive or too host-bound for every change runs scoped to what the
> change reaches, on a native host as blocking or tracked validation
> (FLOW-09), and in full before a release.
>
> A component writes its invariants as rules in LINT-01's format, so one
> rule index and one judgment agent (LINT-06) can read any component's.
> The concrete checks are local: the profile says, for each class, where
> its checks are defined and which tier they are in. A check with no tier
> is a finding.
>
> The agent part: the design review of a profile judges that each class is
> filled, or `none` with a reason; Review judges that a check or rule a
> change adds has a class and a tier.
>
> ---
>
> ### LOOP-04 — One queue per component, one order
>
> **Severity:** MUST
> **Check:** agent-judgment
>
> Each component has its own queue. An item is one of two kinds (FLOW-03):
> a *feature*, which changes the component, or a *process item*, which
> changes the rules the loops run under: the loop rules, the loop's
> machinery, or a component's own invariants when no feature needs the
> change. Both kinds are in a queue, and every queue has the same phases
> (FLOW-03), step affinities (FLOW-04) and order (FLOW-05).
>
> - A process item that changes a loop rule or the loop's machinery is
>   queued where the loop rules are kept. One that changes only a
>   component's own invariants or profile is in that component's queue.
> - A process item ranks ahead of every item not already in implementation
>   (FLOW-05). It stops other work only through the `holds` it declares,
>   which name the workers it stops and the components it stops them in.
> - Priority is the human's. An agent places an item only where a rule
>   says so: the session that finds a regression trunk already has files
>   its fix at the top of the fixes (FLOW-09), and the human may move it.
>   No agent otherwise reorders items.
> - An item that cannot proceed says what it waits for, and its worker
>   moves on (FLOW-05, FLOW-07).
> - No item needs a step of an item in another component's queue. An item
>   that waits on another component waits for a release (LOOP-07): it is
>   put on hold with a blocking question that names the release (FLOW-07),
>   and resumes when the release exists.
> - FLOW-05 orders each queue. How a worker divides its run between
>   queues, and whether FLOW-05's limits and FLOW-13's budgets count per
>   component or across them, are not yet rules. A process item sets them
>   when a second queue exists (LOOP-09).
>
> The agent part: intake and the design review judge an item's kind and
> queue, and that it names no step in another queue.
>
> ---
>
> ### LOOP-05 — Workers are scheduled, routed and identified
>
> **Severity:** MUST
> **Check:** agent-judgment
>
> Work is done by workers that take steps from a queue: `cloud`, `human`
> or `host:<name>` (FLOW-04).
>
> - **Scheduled and bounded.** An unattended worker runs on a schedule and
>   within a budget (FLOW-13). A native machine's scheduled session starts
>   an agent only when the machine has work (FLOW-09).
> - **Routed by affinity.** Work that needs a particular machine carries a
>   host name. One table, shared by every component, gives each host name
>   to exactly one machine (FLOW-04). Work that any machine with a
>   capability can do carries `<capability>-any`, which the table also
>   gives to one machine.
> - **Identified.** A worker works on one component's item at a time.
>   Every claim records the worker (its kind, its host name and its
>   session), the component and the item.
> - **Set up by the owner.** The scheduler on each machine is installed by
>   the machine's owner, by hand. No agent installs or changes it. A
>   worker's credentials are the session's or the machine's own, are
>   configured outside every repository, and never appear in a prompt, a
>   queue or a report (FLOW-18).
>
> The agent part: the design review of a change to the scheduling scripts
> or the worker skills judges these four.
>
> ---
>
> ### LOOP-06 — Every run reports in one place per component
>
> **Severity:** MUST
> **Check:** agent-judgment
>
> Each unattended run appends one section to the day's digest (FLOW-13)
> of each component it worked in. The section records:
>
> 1. each step taken: the item, its kind and FLOW-05 class, the step, its
>    outcome and the branch head it left;
> 2. each specification the step changed, named apart from the code
>    (LOOP-02);
> 3. the code change in a line, or the pull request that carries it;
> 4. the check results by tier: the deterministic checks as passed or
>    failed, and the judgment tier's findings by severity with where its
>    report is (LINT-05);
> 5. everything blocked or parked, and why: on the human, on a host, on
>    another component's release, on a budget.
>
> The judgment tier's report is kept per branch head (LINT-04, LINT-05). A
> pull request's body lists the specification changes and the rule changes
> it carries, and the judgment tier's MUST findings (FLOW-08).
>
> Digests and reports are kept with the queue, not in a component's code
> history.
>
> The agent part: a run checks its section against this list before it
> ends; the design review of a change to a skill that writes the digest
> judges that the list is still covered.
>
> ---
>
> ### LOOP-07 — Components couple only through releases
>
> **Severity:** MUST
> **Check:** agent-judgment
>
> Components evolve independently. One depends on another only through a
> release.
>
> 1. A component lists its **consumables** in its profile: its tagged
>    releases, the specifications each implements and at which versions,
>    and its artifact forms.
> 2. A consumer adopts a release through a **reconciliation item** in its
>    own queue: `adopt <consumable> at <version>`. It is a feature item,
>    filed through intake at the priority the human gives it.
> 3. The item moves the consumer's pin and, where a seam changes, its
>    specification first (LOOP-02), then its code, and then retires any
>    in-tree equivalent.
> 4. Both components' checks pass after adoption. A release is never
>    changed to make that so: a change the consumer needs is a new item in
>    the producer's queue and a new release.
> 5. A component's trunk names only released versions of what it
>    consumes. A branch may name an unreleased commit while its work is in
>    progress.
> 6. A specification kept in a repository of its own is consumed the same
>    way. A seam change is released by the repository that holds the
>    specification and adopted by each implementation in its own queue, at
>    its own time.
>
> The agent part: Review of a reconciliation item judges points 3 to 5;
> intake judges that a change one component needs from another was filed
> in the producer's queue.
>
> ---
>
> ### LOOP-08 — Adopting the contract
>
> **Severity:** MUST
> **Check:** agent-judgment
>
> A repository becomes a component by:
>
> 1. linking to this contract from its README and its agent context file.
>    The link is to the contract's trunk: never a copy, and never a pinned
>    version;
> 2. writing its profile (LOOP-01), every term filled or `none`;
> 3. writing its invariants as rules in LINT-01's format, under prefixes
>    it names in its profile, its safety invariants among them (LOOP-03);
> 4. keeping designs with the front-matter and sections the detail names
>    (FLOW-02, FLOW-12, FLOW-19), plus its own;
> 5. declaring its consumables (LOOP-07), even when its first release is a
>    stub;
> 6. protecting its trunk so that only a human merges (FLOW-08), deleting
>    head branches on merge (FLOW-16), and recording sessions only to the
>    private sessions repository (FLOW-18);
> 7. having a queue and workers (LOOP-04, LOOP-05).
>
> A rule prefix belongs to one owner, so a rule ID names one rule wherever
> it is cited. `LOOP-`, `FLOW-` and `LINT-` are the loop's. A component
> names its prefixes in its profile and takes none that another profile
> names. A prefix changes owner only when its rules move, in the items
> that move them. A component may cite another component's rule by ID as
> binding in its own repository.
>
> An item in a component's queue may work in a repository that has not
> adopted the contract, such as one that holds specifications or a fork.
> There the worker is bound by FLOW-08, FLOW-16, FLOW-17 and FLOW-18, and
> the item's design says which of its component's invariants apply. Once
> that repository adopts, the two couple only through releases (LOOP-07).
>
> The contract assumes of a component: git, a trunk and feature branches,
> pull requests that a human merges, a deterministic check that can block
> a pull request, Markdown rule files, and designs with front-matter. It
> assumes nothing else. No loop rule may require a language, a build tool,
> a test framework, a directory layout beyond what the profile names, an
> agent tool, a CI system, or that a component has drivers, user-facing
> surfaces, security boundaries or a public repository. Where the detail
> names one of these from `buildwarden/buildwarden`, LOOP-01's reading
> rule applies, and the wording is made neutral when the rule is next
> changed.
>
> The agent part: the design review of an adopting component's first item
> judges the seven steps, and that its prefixes are no other component's;
> the design review of a process item judges that a new or amended loop
> rule assumes nothing beyond the list.
>
> ---
>
> ### LOOP-09 — The contract changes only through its owner
>
> **Severity:** MUST
> **Check:** agent-judgment
>
> The loop rules (LOOP-\*, FLOW-\*, LINT-\*), the loop's machinery (its
> tools, skills and agent definitions) and the checks that enforce a loop
> rule, wherever they are kept, change only through a process item
> (FLOW-03, FLOW-14). Its design is settled by the owner, the human of
> FLOW-08, in a design review (FLOW-19), and its merge is the owner's. No
> component signs off a change, and no feature's design declares one. A
> component that needs a loop rule changed asks for a process item.
>
> A change binds every component from its merge, and reaches work in two
> ways. An unattended run takes the loop's tools, skills and configuration
> from trunk as they are when it starts, and keeps them until it ends.
> Rule text and a component's checks are read from the branch being worked
> on, so a branch forked before the change follows the old text until it
> merges trunk, which FLOW-05 has it do while it waits and Publish does at
> its start. A process item whose change cannot wait for that, or that
> would disrupt running work, declares the workers it holds, in the
> components it affects (FLOW-05).
>
> A component's profile and invariants, and the checks that enforce them,
> are its own. They change by either of FLOW-14's routes, in that
> component.
>
> The agent part: intake and the design review judge that a change to a
> loop rule or to its enforcement is a process item.

**Severity and check:** all nine are MUST and `agent-judgment`. Each ends
with the prompt the agent part is judged by (LINT-08). The MUST is for what
a LOOP rule states itself. Where it cites FLOW-05, FLOW-13 or LINT-06,
which are SHOULD, the cited part stays SHOULD; the contract's preamble says
so, and no existing rule's severity is raised. No enforcement change.
`loop-repository` can give LOOP-01, LOOP-04 and LOOP-05 an automated half
once `wf` reads profiles and records components.

**What is true on merge.** LOOP-01 to LOOP-03 and LOOP-06 hold in this
repository as soon as the PR merges: the profile exists and says which
checks do not block, the template has the section, the design review goes
over it, and the skills write the three additions. LOOP-03's coverage
clause asks for nothing yet: no specification here has a conformance suite
until S1's lands. LOOP-09's first paragraph holds through the amended
FLOW-03 and FLOW-14 and the design review. Its second paragraph describes
the loop as it runs, with one part that rests on practice and not on a
check: the native session resets to `origin/main` before it builds `wf`,
and the cloud tick does not. It starts from the scheduled task's checkout
and every run leaves that on `main`. Making it a check is listed for
`loop-repository`. In LOOP-04 and LOOP-05 the clauses that speak of more
than one component (a hold's components, where a loop process item is
queued, the component on a claim) ask for nothing while there is one.
LOOP-07 and LOOP-08 bind from the first adoption. Nothing in the nine
contradicts `main` (the umbrella's Decision 18).

### PROFILE-01 (add), `docs/development/loop-profile.md`

**Why:** LOOP-01 requires every component to have a profile and to keep it
as one of its own rules, so this repository needs one in the same PR. The
rule ID is what lets a later feature declare a change to it (FLOW-14).

**Current wording:** none; the file is new.

**Proposed wording.** A title, the rule header, and one table with a row
per term, in LOOP-01's order:

> ### PROFILE-01 — This repository's loop profile
>
> **Severity:** MUST
> **Check:** agent-judgment
>
> The table below is `buildwarden/buildwarden`'s profile under the Loop
> Contract (LOOP-01). Where an entry names a file, that file holds the
> fact. An entry that no longer matches the repository is a finding, and
> the change that makes it stale changes the entry.
>
> The agent part: Review judges that the diff leaves no entry stale; the
> design review of a change to this rule judges that every term is filled
> or `none` (LOOP-01).

| Term | This repository's entry |
|---|---|
| Component | `buildwarden`, in `buildwarden/buildwarden`: the kit, the orchestrator, the collector, the executor, the drivers and, until every driver has adopted the Rust relay, the Go relay. |
| Trunk | `main`. |
| Toolchain | Go, at the version in each module's `go.mod`; `make`; `golangci-lint`. |
| Rules | `docs/development/`. The files and prefixes are the rows of `README.md`'s ID scheme table whose owner is `buildwarden/buildwarden`. |
| Designs | `docs/design/<item>.md`, written from `docs/design/TEMPLATE.md`, on the branch `feat/<item>`. The template's own sections are the ones it ties to a project rule: **Defaults and overrides** (GLOBAL-09 to GLOBAL-11), **Cross-driver impact** (ORCH-01), **Bootstrap comparison** (BOOT-07), **Security note** (GLOBAL-06). |
| Impact vocabulary | The keys and values in `docs/design/TEMPLATE.md`'s front-matter. The driver names, the shared packages that declare every dependent driver, and the seam names are the `drivers` and `seams` lists of `tools/tier1/rules.yaml` (ORCH-01, GLOBAL-08). The `security_boundary` values are GLOBAL-06's. |
| Specifications | `specs/`: the ledger wire format and the metadata and environment schemas (RELAY-05), mirrored in the book. For the seams of GLOBAL-08, their contract documents are the `seams` list of `tools/tier1/rules.yaml` and their tests are TEST-05's. No specifications release is pinned today. |
| Fast gate | `make lint` and `make test` (TEST-01). `make lint` is also run as `GOOS=<os> make lint` for each platform whose build-tagged files the change touches (FLOW-17). |
| Checks | The table below. |
| Test tiers | TEST-02, which says where each tier runs. |
| Native hosts | The host names in the loop's `hosts.yaml`. All of them serve this repository's drivers today. |
| Reference documents | The surface and its documents are the `surface` entry of `tools/tier1/rules.yaml` (UX-01, ORCH-03). Guides: `docs/book/src/guide/`, `docs/book/src/getting-started/`, `examples/`, `README.md`. |
| Sweep | `make deadcode`, on Linux, macOS and Windows. Kept with a stated reason: the exported API of `relay/`, `collector/`, `ledger/`, `relaycfg/` and `bootstrap/`, and the cgo-only code of `driver/vz`. The overview is `docs/book/src/concepts/overview.md`. |
| Boundary rule | GLOBAL-06. |
| Flaky-test rule | TEST-04. |
| Rule and machinery paths | Rule changes: `docs/development/`, `tools/tier1/`, `tools/workflow/check/`, the rule-running jobs of `.github/workflows/`, `docs/design/TEMPLATE.md`. Machinery: `.claude/` and `tools/workflow/` outside `check/`. |
| Release | A `v*` tag push runs the `release` workflow (GoReleaser), whose `tier1` job is the gate's deterministic half (LINT-03), and the `relay-image` workflow. The human's sign-off of a tier-2 report over the release diff has no wired step yet. |
| Consumables | Releases: the `v*` tags. Consumes: nothing. |
| Sessions | `.entire/settings.json`, whose `checkpoint_remote` is `buildwarden/buildwarden-sessions`; `tools/workflow/flow18-check.sh`; the `flow18` workflow. |

The Checks entry, by class. Tier 1 is deterministic and blocks; tier 2 is
the `wf-review` agent, report-only (LINT-07):

| Class | Where its checks are defined | Tier |
|---|---|---|
| Build | The `build`, `tidy` and `book` jobs of `.github/workflows/main.yml` | 1 |
| Specification | The coupling checks that `tools/tier1/rules.yaml` maps to RELAY-05, GLOBAL-08, EXEC-04, UX-01, UX-03, ORCH-03 and BOOT-07; the contract tests of TEST-05; the `keytool` job | 1 |
| | The judgment half of those rules | 2 |
| Safety | `make lint` (`golangci-lint`, and tier 1's analyzers as `rules.yaml` maps them) and `make test`; the `integration` workflow (TEST-02's tier 1) | 1 |
| | Native validation on a host | 1; blocking or tracked per design (FLOW-09) |
| | Every `agent-judgment` rule and the judgment half of every hybrid rule | 2 |
| Process | Tier 1's `rule-index`, `design-front-matter`, `drivers-claim` and `suppression` checks; `wf`'s done-when checks and `wf validate`; `make enforcement`; `tools/workflow/flow18-check.sh` | 1 |

Declared as not blocking: `tests-with-change` (GLOBAL-04), a hint; `make
deadcode` (FLOW-15), a report that Publish judges; and the `firecracker`
workflow, until its header's condition is met.

The table states today's values and would change if a merge before this one
changes them (`seam-specs` proposes a specifications pin).

### LOOP-01 and PROFILE-01 (add), `docs/development/README.md`

**Why:** the README is the index of the rule files and holds the ID
scheme. It has to list the two new prefixes and say which side each file
is on, so the split is visible where a reader starts.

**Current wording**, the ID scheme table's header and the documents list's
lead-in:

> | Prefix    | Domain                                             | File                    |
>
> ## Documents

with nine numbered documents in one list.

**Proposed wording:**

- The ID scheme table gains an Owner column (`buildwarden/buildwarden` or
  "the loop") and two rows:

  > | `LOOP-`    | The Loop Contract                                 | `loop-contract.md`      | the loop |
  > | `PROFILE-` | This repository's loop profile                    | `loop-profile.md`       | `buildwarden/buildwarden` |

- A new paragraph ends the first section, **The core idea**:

  > The standards are in two parts. The **project invariants** are this
  > repository's own and stay here. The **loop rules** are the
  > [Loop Contract](./loop-contract.md) and its detail, which every
  > BuildWarden component's loop runs under; they move to the loop
  > repository (LOOP-08). [`loop-profile.md`](./loop-profile.md) is this
  > repository's profile (PROFILE-01).

- The **Documents** list becomes two lists. "Project invariants":
  `global.md`, `relay.md`, `orchestrator.md`, `in-vm-executor.md` with
  `bootstrap.md`, `ux-cli-config.md`, `testing.md`, `security-suite.md`,
  and `loop-profile.md`. "Loop rules": `loop-contract.md`,
  `process.md`, `behavioral-linter.md`. Each entry keeps its present
  description.
- The **Rule format** section gains one sentence: "This format is LINT-01's
  and is the same for every component."

Nothing else in the README changes, and no section moves. Each section's
side, for `loop-repository`:

| README section | Side | At the move |
|---|---|---|
| The core idea | Loop: it is LINT-01's and LINT-08's premise | Copied to the loop repository's index; a short form stays here with the two-part paragraph |
| Rule format, with Severity and Check-type | Loop (LINT-01, LINT-02) | Moves; this README links to it |
| ID scheme | Both: each row has an owner | The loop's three rows move; the rest stay |
| The three applications | Project | Stays |
| Documents | Both, already in two lists | The "Loop rules" list becomes a link |
| Relationship to the other guidance files | Project, except its last paragraph, which restates FLOW-14 | Stays; the last paragraph moves |
| Open deep-dives | Project: they are this repository's queue items | Stays |

### LOOP-02 (add), `docs/design/TEMPLATE.md`

**Why:** LOOP-02 asks a design that changes a seam to list its
specification changes, and the template is where a design learns its
sections.

**Current wording:** no such section. The front-matter comment on `seams`
reads:

> seams: []                # e.g. relay-claim, collector, executor-initialize, ledger-format,
>                          # bootstrap-model (BOOT-07: needs the Bootstrap comparison below)

**Proposed wording.** The comment gains a line:

> seams: []                # e.g. relay-claim, collector, executor-initialize, ledger-format,
>                          # bootstrap-model (BOOT-07: needs the Bootstrap comparison below).
>                          # Not empty: fill Specification changes below (LOOP-02)

and a section is added between **Cross-driver impact** and **Bootstrap
comparison**:

> ## Specification changes
>
> Required when `seams` is not empty (LOOP-02); delete otherwise. For each
> seam: the specification document and the requirements the item changes,
> what changes in them, and where the change is made (the path here, or
> the other repository's pull request and the release this repository
> will pin). An item that only implements existing requirements lists
> them and says "no change". For a seam with no written specification,
> say so and name what defines it.

**Enforcement:** none added. `wf`'s done-when does not ask for the
section. The design review does (LOOP-02's agent part), and the
`workflow-human` skill's walkthrough gains it (Implementation plan, task
7). A design approved before this merges is not sent back for it:
`wf-publish` adds the section at `f6-consolidate`, after it has merged
`main` (task 7).

### FLOW-01 (amend), `docs/development/process.md`

**Why:** FLOW-01 is where an agent learns what comes before code. It has
to say that at a seam the specification comes before code too.

**Current wording**, the rule's first paragraph:

> Every feature has a design document under `docs/design/`, written from
> [`TEMPLATE.md`](../design/TEMPLATE.md), before implementation begins, and the PR
> that lands the feature links it. Changes that carry no design decisions
> (dependency bumps, typo fixes, CI-only changes) are exempt and are labelled
> `no-design` on the PR.

**Proposed wording.** That paragraph is unchanged, and one follows it:

> A feature that changes behaviour a specification defines is also
> spec-first: the specification changes before the code that implements
> it, and the design lists the change (LOOP-02).

**Severity and check:** unchanged (MUST; automated + agent-judgment). No
enforcement change.

### FLOW-03 (amend), `docs/development/process.md`

**Why:** FLOW-03 restates FLOW-14's second route in one sentence. Left as
it is, it would say a feature may change "the rules" beside a FLOW-14 that
says not the loop's.

**Current wording**, in the paragraph that defines a process item:

> A feature may also change the rules and their enforcement when
> its design review approves each change by name (FLOW-14); `.claude/` and the
> rest of `tools/workflow/` change only through a process item:

**Proposed wording:**

> A feature may also change its own component's rules and their
> enforcement, never a loop rule (LOOP-09), when its design review
> approves each change by name (FLOW-14); `.claude/` and the
> rest of `tools/workflow/` change only through a process item:

**Severity and check:** unchanged (MUST; automated). No enforcement
change.

### FLOW-14 (amend), `docs/development/process.md`

**Why:** the draft's decided point, that a change to the contract is gated
on Jeff only, and LOOP-09. FLOW-14's second route lets a feature's design
carry a change to any rule in this directory, loop rules included.

**Current wording**, the first route's last sentence and the second
route's first two sentences:

> It is the only
> route for `.claude/` and for `tools/workflow/` outside `check/`, the loop's
> own machinery, and the route for a rule change no feature needs.

> - **A rule change a feature's design declares.** A *rule change* is an edit to
>   this directory (a rule, its severity or check type, the README index), to

**Proposed wording:**

> It is the only
> route for `.claude/` and for `tools/workflow/` outside `check/`, the loop's
> own machinery, for a loop rule (LOOP-09), and the route for a rule change
> no feature needs.

> - **A rule change a feature's design declares.** A feature's design may
>   declare a change to its own component's invariants and profile and to
>   their enforcement, and never to a loop rule (LOOP-\*, FLOW-\*, LINT-\*)
>   or its enforcement (LOOP-09). A *rule
>   change* is an edit to
>   this directory (a rule, its severity or check type, the README index), to

The rest of FLOW-14 is unchanged.

**Severity and check:** unchanged (MUST; automated + agent-judgment). The
new sentence is in the agent part: the design review refuses a feature's
`rule_changes` entry that names a loop rule, whatever its paths. The rule
ID on the entry decides. `tools/tier1/` and `tools/workflow/check/` stay
rule-change paths a feature may edit, because they hold the checks of
project rules too (`driver-contract` changes `tools/tier1/surface/` under
UX-01), and an edit there is cleared only under the rule it enforces.

The automated clearance (`tools/workflow/check/rulechange.go`) is not
changed: it still clears any approved path, and does not look at the rule
ID's prefix (a10's third option would make it). What the move changes: the
loop's rule files and `tools/workflow/`, `check/` included, leave this
repository, so a feature's PR here cannot edit them. Tier 1's checks of
FLOW and LINT rules (`rule-index`, `design-front-matter`, `drivers-claim`)
stay in `tools/tier1/` and remain guarded by the review alone until
`loop-repository` decides where tier 1's generic checks live.

## Validation plan

A rules-and-documents item with two small changes to `wf` and edits to the
skills and agent definitions. There is no native validation and no tier-2
gap (FLOW-09, TEST-03): nothing in the PR is specific to a host's driver or
lifecycle.

- **Tier 0** (TEST-02), on the branch:
  - `make lint`: `golangci-lint` over the three modules, then tier 1 on
    the diff from `origin/main`. The checks that bear on this PR:
    `rule-index` (LINT-01, LINT-02: the nine LOOP headers and PROFILE-01
    each parse with a severity and a check, and no `automated` rule lacks a
    `rules.yaml` entry); `design-front-matter` (FLOW-02) on this design; the
    design-in-the-diff check (FLOW-01), which the `wf` changes need and
    this design satisfies; `tests-with-change` (GLOBAL-04) for the two `wf`
    changes.
  - `make test`: `tools/tier1`'s `TestRealIndex` parses the real rule
    files, the two new ones included; `tools/workflow`'s tests, in the
    `queue` package, cover `wf new -hold` (the item it writes validates;
    `Next` gives the cloud no step of it and, while it is queued, no
    `blocked` line; answering `q1` completes the step and the first step is
    offered) and the configured
    `alters_relationships` limit (3 with two holders offers a step; 1 and
    unset hold it; `wf validate` agrees in each case).
  - `make enforcement`: a no-op, since the branch changes neither
    `tools/tier1/` nor `tools/workflow/check/` (FLOW-14).
- **CI on the PR:** the `ci` workflow, in which `docs/development/*` turns
  on the rule tests and the `tools/` changes turn on lint, test and build,
  and the `flow18` workflow, which reads the refreshed
  `.entire/settings.json` (FLOW-18).
- **By hand, in the implement step:** `wf validate` against the `workflow`
  checkout before and after, to show the `wf` built from the branch reads
  today's queue unchanged; `wf new -hold` and `wf answer` once against a
  scratch copy of the queue.
- **The skill and agent edits** have no automated check. The digest's
  three additions need no command the scheduled native session's allowlist
  lacks (`dist/wf` is allowed, and the rest is text). The first cloud
  digest and the first scheduled native digest on each machine after the
  merge are read against LOOP-06's list, and a miss is a follow-up process
  item.
- **Tiers 1 and 2:** not reached. The integration workflow's paths
  (`driver/`, the shared packages, `cmd/`) are not touched.

A process item has no Review phase (FLOW-03), so the two `wf` changes get
no tier-2 report. They are kept small and tested for that reason, and the
human's merge is their review (FLOW-08).

Not verified for this design: the repository's branch protection and its
"delete head branches" setting, which LOOP-08's step 6 and the profile's
Checks entry rest on, and how the scheduled cloud task obtains its
checkout, which LOOP-09's "from trunk" rests on for the cloud run. Both
are outside the repository. Nor was it established whether a phase agent's
definition is read when the session starts or from the tree after the tick
switches to the feature branch.

## Implementation plan

For this item's PR (process phase 2). Tasks 9 to 11 are dropped if the
review moves a11, a12 or a13 elsewhere; task 8's `-hold` wording goes with
a11.

1. Add `docs/development/loop-contract.md` with LOOP-01 to LOOP-09 as
   approved (FLOW-14, LINT-01, LINT-08).
2. Add `docs/development/loop-profile.md` with PROFILE-01 and its two
   tables as approved. Write each entry against `main` as it is when this
   step runs: if `seam-specs` or `driver-contract` has merged, the
   Specifications, Consumables and Checks entries follow what it merged
   (LOOP-01, LOOP-03, PROFILE-01).
3. Restructure `docs/development/README.md`: the Owner column, the `LOOP-`
   and `PROFILE-` rows, the two-part paragraph, the two document lists and
   the sentence in **Rule format** (LOOP-01, LINT-01).
4. Amend FLOW-01, FLOW-03 and FLOW-14 in `docs/development/process.md` to
   the approved wording (FLOW-14).
5. Add the **Specification changes** section and the `seams` comment line
   to `docs/design/TEMPLATE.md` (LOOP-02, FLOW-14).
6. `.claude/skills/workflow-tick/SKILL.md`. Step 4: each **Moved** line
   names the item's kind and FLOW-05 class, beside the existing **Process
   items** section; a step that changed a specification adds a
   **Specification changes** line; a review step's line gives tier 1's
   result and the tier-2 counts by severity with the report's path. Cite
   LOOP-06 and do not restate it (FLOW-14). The Never list: the exception
   for a rule change a design review approved does not cover a loop rule
   (LOOP-09). `.claude/skills/workflow-native/SKILL.md`: its unattended
   digest section follows the same list, and its Never list gets the same
   qualifier.
7. `.claude/skills/workflow-human/SKILL.md`, the design review's step 3:
   go over **Specification changes** where the design has it (LOOP-02),
   and refuse a feature's `rule_changes` entry that names a LOOP, FLOW or
   LINT rule (LOOP-09). `.claude/agents/wf-design.md`: step 2 fills
   **Specification changes** when `seams` is not empty, even when the
   branch's template predates it (LOOP-02), and step 6 says a feature's
   design declares no change to a loop rule (LOOP-09, FLOW-14).
   `.claude/agents/wf-publish.md`: the PR body gains a **Specification
   changes** section when the design has one, and `f6-consolidate` adds the
   section to a design that declares `seams` and has none (LOOP-02,
   LOOP-06).
8. `.claude/skills/workflow-intake/SKILL.md`, step 3: a change to a loop
   rule is a process item and never a feature's rule change (LOOP-09,
   FLOW-14). With a11: step 3 says an item that is not a program item and
   not exempt is filed with `-hold`, and step 4 shows the flag (FLOW-20).
9. `wf new -hold "<why>"`: the flag in `tools/workflow/cmd/wf/main.go`,
   what it writes as a function in `tools/workflow/queue/`, tested in
   `queue/queue_test.go` (FLOW-20, FLOW-07, GLOBAL-04, GLOBAL-10).
10. `wf next` counts the in-progress items past Design that alter
    relationships against `limits.alters_relationships`, 1 when unset,
    through one function that `Validate` shares, with tests (FLOW-05,
    FLOW-20, GLOBAL-04, GLOBAL-09, GLOBAL-10).
11. Take `.claude/settings.json` and `.entire/settings.json` as commit
    `ac8b41a` has them, and run `tools/workflow/flow18-check.sh` (FLOW-18,
    FLOW-14).
12. `AGENTS.md`: two rows under "Where things are", for the Loop Contract
    and for this repository's profile (FLOW-14).
13. `make lint` and `make test` green, the branch pushed (FLOW-17,
    TEST-01).
14. Merge current `main`, redo task 2's check of the profile against it,
    archive this design on the branch with its summary, and open the PR.
    The PR body carries an "After the merge" list (FLOW-12, FLOW-08).

After the merge, outside the PR, in the attended session in which Jeff
merges: document `wf new -hold` and the limit `wf next` reads in the
`workflow` branch's `README.md` (FLOW-03); tell the relay's M1b that the
contract's text is final and where it is (LOOP-08); and, if this item
merged before `seam-specs`, see that its design carries the PROFILE-01
entry.

## Design review

No review yet.

## Consolidation

A process item has no Publish phase and no consolidation pass (FLOW-03).
The implement step merges current `main` before it opens the PR and records
here what it reconciled, the profile's entries in particular (task 14).

## Rollback notes

None.

## Archive summary

Filled in by the implement step when it archives the design on the branch
(FLOW-03, process phase 2; FLOW-12).
