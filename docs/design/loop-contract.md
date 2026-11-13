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
        LINT-08, GLOBAL-06, GLOBAL-08, GLOBAL-09, GLOBAL-10, GLOBAL-11, RELAY-05, TEST-02,
        TEST-03, TEST-04, TEST-05, UX-01, LOOP-01, LOOP-02, LOOP-03, LOOP-04, LOOP-05,
        LOOP-06, LOOP-07, LOOP-08, LOOP-09]
rule_changes:
  - {rule: LOOP-01, change: add, paths: [docs/development/loop-contract.md,
                                         docs/development/loop-profile.md,
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
  - {rule: FLOW-01, change: amend, paths: [docs/development/process.md]}
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

This item holds no worker (its queue file has no `holds`), so nothing in its
PR may disrupt running work: S1 (`seam-specs`) and K1 (`driver-contract`) are
in Design now and will run under whatever this merges.

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
  Its eight sections, two decided points and five blanks are the starting
  material.
- **The relay fork's decisions table** (read at revision 20) and
  **kickoff** (revision 12), both 2026-11-07, for what the relay's loop
  needs: decisions 2, 3, 5, 6, 7, 8 and 16 of the table, and the kickoff's
  "Loop contract" and "Design constraints" sections.
- **`buildwarden/buildwarden-relay`** at `cc40728` (`feat/m1-foundation`): a
  placeholder README, `docs/design/m1-foundation.md` started with three
  front-matter keys, and no rules, queue, agents or build files yet.

## Decisions

Two points were decided by Jeff in the draft and are carried as given: the
contract lives in its own repository, apart from the project (the umbrella's
Decision 13 names it), and a change to the contract is gated on Jeff only.
The decisions below are this design's. Those that settle a disagreement
between the draft and the running loop are also questions (a1 to a13), so the
review confirms each on its own.

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
   same change (UX-01, FLOW-15). *Rejected:* letting a specification change
   stand in for the design. A seam change is the kind of decision the design
   review exists for (FLOW-19 names seams).
4. **FLOW-05's four classes stand; "feature" and "process" are the two
   kinds of item** (a4). The draft's two classes of work become the queue's
   `kind`. Its "process work goes to the top of the queue" is FLOW-05's
   class 2, behind items already in implementation, plus `holds`.
5. **Checks have four classes and two tiers, and only deterministic checks
   block** (a6). The draft's classes (build, specification, safety, process)
   say what a component declares. LINT-02's tiers say how each check is run
   and what a failure does. The draft's "block merge on failure" holds for
   tier 1; LINT's load-bearing decision, that no agent holds a veto over
   trunk, stands for tier 2.
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
9. **Rule IDs never change, and each prefix has one owner.** A rule keeps
   its ID when its file moves. Prefixes are unique across components and
   are registered in the contract: LOOP, FLOW and LINT belong to the loop;
   GLOBAL, RELAY, ORCH, EXEC, BOOT, UX, TEST and SEC to
   `buildwarden/buildwarden`. A citation by ID therefore stays valid across
   repositories. *Rejected:* a component prefix on every ID
   (`bw:GLOBAL-06`). It would rewrite every citation in 53 archived designs
   and in the code comments.
10. **Loop rules change only through a process item** (a10). A feature's
    design may declare a change to its own component's invariants and
    profile (FLOW-14's second route), not to LOOP, FLOW or LINT rules.
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
    credentials are each machine's.** The draft puts "which machines" in the
    local column. The same two machines serve every component, a host name
    belongs to exactly one machine (FLOW-04), and a machine runs one native
    session at a time (`native-tick.sh` takes a lock). One `hosts.yaml`
    beside the queues is the only way those stay true. A component's
    profile lists the host names its work uses.
13. **The line between this item and `loop-repository`** (a1): this item
    changes what the rules say; that item changes where things live and
    what `wf` reads. See **What this item hands to `loop-repository`**.

## Questions

### Blocking

None. A process item's design is settled in its design review, where every
assumption below is asked.

### Assumed

Each is "proceeding with the recommended option; confirm". The same thirteen
are in the queue file.

- **a1. Where the line between `loop-contract` and `loop-repository`
  falls.**
  1. *(recommended)* This item: the contract text (LOOP-01 to LOOP-09), this
     repository's profile, the split made in place, the two amendments
     (FLOW-01, FLOW-14), the template's new section, and the skill and agent
     edits that make the contract true here. `loop-repository`: creates the
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
     declares. Each check is tier 1 or tier 2 (LINT-02). Tier 1 blocks.
     Tier 2 informs the human, who signs off by merging (LINT-03, FLOW-08).
     Checks run at the phase gates and in CI, scoped to the change (LINT-04),
     not on every step. Host-bound validation blocks or is tracked per
     design (FLOW-09).
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
  1. *(recommended)* Spec-first applies to behaviour at a seam that has, or
     by the program is to have, a specification. The item still has a
     design; the design lists the specification changes; in Implement the
     specification change comes before the code. An item that implements
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
     one sentence: a feature's design may not declare a change to a LOOP,
     FLOW or LINT rule. Checked in the design review; no enforcement code
     changes, since the move makes it physical.
  2. Leave FLOW-14 as it is until the files move.
  3. Also teach `wf`'s rule-change clearance to refuse those three files on
     a feature branch now (`tools/workflow/check/rulechange.go`).
- **a11. How intake files an item on hold** (FLOW-20's freeze). `wf new` has
  no way; the six items closed on 2026-11-12 were held by hand-editing a
  blocking question and an answer step into each queue file.
  1. *(recommended)* `wf new` gains `-hold "<why>"`, in this PR. It writes
     what the hand-edit wrote: question `q1`, blocking and open, and a human
     step `<f0|x1>-answer-q1` that the item's first step needs.
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
  reported as waiting for the slot.
  1. *(recommended)* This PR fixes `wf next` to count holders against the
     configured limit, with a test. It is `tools/workflow/` outside
     `check/`, so only a process item can.
  2. Its own small process item, filed now, if S1 or K1 would reach Plan
     before this item merges.
  3. Leave it to `loop-repository`.

## Design

### The contract's structure

One document, `docs/development/loop-contract.md`, with a short preamble, a
table of where the loop's parts are today, and nine rules. Its full proposed
text is under **Rule changes**.

| Rule | Title | Draft section | Detail it cites |
|---|---|---|---|
| LOOP-01 | Shared rules, a local profile | 1, and 8 steps 1 and 2 | FLOW-14, LINT-01 |
| LOOP-02 | Design-first always, spec-first at a seam | 2 | FLOW-01, FLOW-02, FLOW-06, FLOW-07, FLOW-19 |
| LOOP-03 | Checks: four classes, two tiers, run at the gates | 3 | LINT-01 to LINT-08, FLOW-03, FLOW-09 |
| LOOP-04 | One queue per component, one order | 4 | FLOW-03, FLOW-04, FLOW-05, FLOW-06, FLOW-07 |
| LOOP-05 | Workers are scheduled, routed and identified | 5 | FLOW-04, FLOW-09, FLOW-13, FLOW-18 |
| LOOP-06 | Every run reports in one place per component | 6 | FLOW-13, LINT-05, FLOW-08 |
| LOOP-07 | Components couple only through releases | 7 | FLOW-08, FLOW-10, LOOP-02 |
| LOOP-08 | Adopting the contract | 8 | FLOW-08, FLOW-16, FLOW-18, LINT-01 |
| LOOP-09 | The contract changes only through its owner | Open questions (the two decided points) | FLOW-03, FLOW-05, FLOW-14, FLOW-19 |

The contract and its detail are the *loop rules*: LOOP-\*, FLOW-\*, LINT-\*.
Everything else in `docs/development/` is a *project invariant* of
`buildwarden/buildwarden`. A component's *profile* is its own document
(here `docs/development/loop-profile.md`) that fills the terms LOOP-01
lists.

### The draft against the running loop

"The running loop" is `main` at `b903f65` and the `workflow` branch at
`174a6cc`.

#### The draft's blanks, filled

| Draft | Asks for | What the loop does today |
|---|---|---|
| 2, "fill in" | Sectioning or tagging rules that map specification sections to tests or features | There is no tagging of sections to tests. What exists: (a) the layout `specs/<dir>/v<N>.md` with `v<N>.schema.json` beside it, one pair per major version (RELAY-05); tier 1's `schema-doc` check fails a commit that changes one without the other (`tools/tier1/rules.yaml`, `schema_doc`). (b) Tier 1's `seam-contract` check requires a change to a versioned seam to change its contract document (GLOBAL-08, EXEC-04); its `seams` list is empty, so it checks nothing until S1 fills it. (c) Contract tests for the seams (TEST-05) and `specs/specs_test.go`. (d) A design maps a feature to seams through its `seams` front-matter (FLOW-02), and to rules through `rules`. `seam-specs` (unreviewed) proposes requirement identifiers that suite cases name; that belongs to the specifications repository, not to the contract (LOOP-03, class 2). |
| 3, research task | The main loop's current checks, one per line | The list under **The checks the loop runs today**. |
| 4, research task | The queue's location and format | The orphan branch `workflow` of `buildwarden/buildwarden`: `queue/<item>.yaml` (state, steps with affinity and lease, budget, questions, history), `config.yaml` (budgets, limits, lease length, runaway factor), `hosts.yaml`, `backlog/native-gaps.yaml`, generated `QUEUE.md`. The schema is that branch's `README.md`; `wf` (`tools/workflow/cmd/wf`) is the only writer where a command exists (FLOW-03, FLOW-04). |
| 5, "fill in" | Schedule cadence | The cloud loop runs once a day (FLOW-13; 02:55 Pacific, per `install-macos.sh`) and takes up to `run_budget.steps` steps, 24 today. Each native machine has a scheduled session that starts an agent only when `wf gate` finds work (FLOW-09): `macbook` daily at 04:40 local (launchd, which runs a missed slot on wake), `windows-desktop` at 04:10 and every 6 hours (Task Scheduler). Each spends `native_budget.steps`, 8 today. |
| 5, "fill in" | Taker identity or credential model | A worker is `cloud`, `human` or `host:<name>`, with a `/<session>` suffix (`cloud/<UTC timestamp>`, `human/jeff`, `host:macos-vz/<timestamp>`). The identity is in every lease and history line (FLOW-04). A claim is a lease pushed to the `workflow` branch; a rejected push loses the claim; an expired lease (120 minutes) may be reclaimed. Credentials are the session's or the machine's own git access, set up outside the repository. A scheduled native session is a headless agent under a fixed allowlist (`tools/workflow/native/settings.native.json`), run as the logged-on user and not elevated. No secret goes into a prompt (FLOW-18). |
| 6, decided | Where iterations report | Today: `digest/<YYYY-MM-DD>.md` on the `workflow` branch, one section per run; the tier-2 report per branch head in `reports/<item>/<sha12>.{md,json}`; each machine's last scheduled run in `native/status/<machine>.yaml`. All of it moves with the queue (`loop-repository`). |

#### The checks the loop runs today

Grouped by the draft's four classes. "T1" blocks; "T2" is the judgment tier,
report-only today (LINT-07).

**Build**

- `make build` on macOS and Ubuntu, then `dist/warden --help` (CI `build`). T1.
- `go mod tidy` leaves `go.mod` and `go.sum` unchanged, in the root module
  and in `tools/` (CI `tidy`). T1.
- `mdbook build docs/book` (CI `book`). T1.
- Each component builds for its supported targets (GLOBAL-07, ORCH-05: the
  CI matrix). T1.

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
  `relay-diskless` (RELAY-07), `drivers-claim` (ORCH-01). T1.
- `make test` (TEST-01): on Ubuntu for every module (CI `test`), and
  `go test ./...` on Ubuntu, macOS and Windows (CI `tier1-test`). T1.
- `tests-with-change` (GLOBAL-04). T1.
- `make integration-test` on PRs that reach the container driver or a
  shared package, weekly on `main`, and on demand (TEST-02, tier 1 of the
  test tiers; workflow `integration`). T1 where it runs.
- Native validation on the machines in `hosts.yaml` (TEST-02's tier 2,
  FLOW-09): blocking or tracked per design.
- Security-boundary changes carry a security note (GLOBAL-06). T2, and the
  human's acknowledgement (FLOW-08).
- Every `agent-judgment` rule and the judgment half of every hybrid rule,
  judged on the diff by the `wf-review` agent (LINT-02, LINT-06). T2.

**Process**

- `rule-index`: every rule has a severity and a check, and every
  `automated` rule is mapped in `rules.yaml` (LINT-01, LINT-02). T1.
- `design-front-matter`, and "code changed with no design in the diff"
  (FLOW-01, FLOW-02, FLOW-12). T1.
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
- `make deadcode` at Publish (FLOW-15). Reported and judged, not a gate.

The release gate adds the full test matrix and, for a release that trips
GLOBAL-06, the security suite (TEST-08, SEC-01 to SEC-08, not yet built).

#### Where the draft and the running loop disagree

| # | Draft | Running loop | Resolution | Question |
|---|---|---|---|---|
| 1 | Two classes of work, feature and process. Process work "never enters the feature queue", "goes to the top of the queue and runs before any further feature work". | One queue; `kind: feature` or `process`; four classes in order: in implementation, process items, fixes, new functionality (FLOW-05). A process item stops other workers only through declared `holds`. | FLOW-05 stands. The draft's classes are the two kinds. | a4 |
| 2 | "A loop that picks up a process change re-reads its configuration before taking its next feature task." | Every run starts from `origin/main`, builds `wf` from it and reads `config.yaml` (`workflow-tick` step 1; `native-tick.sh`). Nothing a process item changes reaches a run until the human merges it (FLOW-03, FLOW-14). | Already true, per run. LOOP-09 states it. | none |
| 3 | Invariants in four classes that "run on every loop iteration and block merge on failure". | Two tiers by how a check is made (LINT-02). Tier 1 blocks; tier 2 informs and the human signs off (LINT-03); tier 2 is report-only (LINT-07). Checks run at phase gates and in CI, on the diff (FLOW-03, LINT-04). Expensive tiers are scoped, and a native gap may be tracked (TEST-02, TEST-03, FLOW-09). | Classes and tiers are two axes (LOOP-03). Only tier 1 blocks. | a6 |
| 4 | Iteration output: item and class, spec diff, code diff summary, invariant results in all four classes, what is parked or blocked. | A run takes several steps on several items. The digest has a section per run (Moved, Blocked on Jeff, Follow-ups filed, Design reviews skipped, Waiting on a host, Spent, Notes); the tier-2 report is per branch head. No class is printed, and a specification change is not reported apart from code. | The run's digest section is the report (LOOP-06), with three additions. | a7 |
| 5 | Spec-first for all behaviour: "a behaviour change with no matching spec change fails review". | Design-first (FLOW-01). Coupling to written documents exists per kind of behaviour: schemas (RELAY-05), seams (GLOBAL-08), user-facing surfaces (UX-01), the bootstrap model (BOOT-07), component flows (FLOW-15). | Design-first always, spec-first at a seam (LOOP-02). | a8 |
| 6 | "A spec section an agent cannot satisfy is marked as an open question in the spec." | A finding returns the item to Design (FLOW-06). Questions live in the queue and the digest (FLOW-07). | The queue holds the question; the answer may be a specification change. | a8 |
| 7 | "Specs live in the main BuildWarden project's specification directory" for now. | The umbrella's Decision 3 and a10, five days later: `buildwarden/buildwarden-specs`, created by S1. `specs/` here holds the ledger format until it moves. | The umbrella. The contract leaves the location to each component's profile. | none (decided) |
| 8 | The contract lives in its own repository, "likely" the sessions repository. | The umbrella's Decision 13: its own public repository; the sessions repository is private. | The umbrella. | none (decided) |
| 9 | A reconciliation item is "process work" at the top of the consumer's queue. | A process item changes rules or machinery and has four phases with no Review (FLOW-03). The umbrella's M6: 'one "adopt relay version X" item per driver in that driver's queue'. | A feature item, at the priority intake agrees. | a5 |
| 10 | "Nothing in one component's queue blocks another component's queue." | One set of workers serves every queue: one cloud run budget, two machines, one session per machine. The umbrella's P1 asks for "holds per component". | True of items: no item needs a step in another queue (LOOP-04). Workers are shared, and a process item's `holds` are scoped per component (`loop-repository` builds it). | a1 |
| 11 | Task takers: "which machines" is local to each component. | `hosts.yaml` gives each host name to exactly one machine, for the whole loop (FLOW-04). | The host table is shared; schedules and credentials are each machine's (Decision 12, LOOP-05). | none |
| 12 | Routing for the relay: Firecracker and QEMU work to the Windows desktop. | `hosts.yaml`: `qemu-any` is on `macbook` (the qemu driver runs only on aarch64 hosts); `linux-kvm` on `windows-desktop` is experimental. | Routing is `hosts.yaml`, not contract text. The relay's profile names host names, not machines. | none |
| 13 | "Each taker ... records which repo and which queue item it took." | The lease and history record the worker and the item. There is one repository, so none is recorded. | LOOP-05 requires the component once there is more than one; the field is `loop-repository`'s. | a1 |
| 14 | A change to the contract is gated on Jeff only. | Both of FLOW-14's routes end in Jeff's design review and merge, but the second lets a feature's design carry a rule change. | Loop rules: process items only (LOOP-09). | a10 |

Where the draft and the loop agree, the contract cites the rule: the final
scheduler setup on each machine is the owner's, by hand
([`native-host-loop`](native-host-loop.md), `install-macos.sh`,
`install-windows.ps1`); feature work is ordered by the queue's priority and
agents do not reorder it (FLOW-05, and `priority_confirmed` in
`config.yaml`); a blocked item names what blocks it (`wf next`'s `blocked`
lines, FLOW-07).

<!-- more sections follow -->
