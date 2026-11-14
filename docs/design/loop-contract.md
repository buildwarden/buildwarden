---
feature: loop-contract
status: draft
drivers: []              # rules and documents; no driver tree is reached
seams: []                # the Loop Contract is seam 10 of the componentization design, a
                         # document between the loop and the repositories it works in.
                         # It is not a GLOBAL-08 versioned seam, and this item changes
                         # none of those.
surfaces: []             # `wf` is the loop's tool, not a user-facing surface of the product
security_boundary: none
alters_relationships: false  # no product component's role or interaction changes (FLOW-02).
                         # The contract governs how the one loop works in every
                         # component's repository.
                         # See "What this item does not declare" under Design.
design_review: required  # a process item's design always gets its review (FLOW-03, FLOW-19)
native_validation: []    # rules, documents and one small `wf` change; nothing host-specific
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
  - {rule: LOOP-04, change: add, paths: [docs/development/loop-contract.md,
                                         docs/design/TEMPLATE.md]}
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
           intake-hold, one-queue, componentization, program]
related: [componentization.md, workflow-bootstrap.md, workflow-streamline.md,
          native-host-loop.md, design-approved-rule-changes.md, tier1-static-checks.md,
          specs-migration.md, regression-fixes-follow-up.md]
---

# The Loop Contract: finished against the running loop, with `docs/development/` split into project invariants and loop rules

This is the first item of phase P1 of the componentization program
([`componentization`](componentization.md), a program item under FLOW-20).
It finishes the Loop Contract and splits the standards. P1's other items are
`alters-slot`, `loop-parallel`, `loop-native-lead`, `loop-repository` and,
last, the move of the loop to its own repository. The move is not yet filed;
this design calls it `loop-move`, which is a provisional name. This design
says what it hands to each.

It was revised on 2026-11-14 for one queue, after Jeff stopped its first
review sitting and settled the direction (**Design review**). That section's
third entry lists every change from the text he read at `2889919`. Each one
is a question he confirms or an amendment he approves by name. At the second
sitting, the same day, he answered all twenty questions on the text at
`04cc9bb`. Three answers departed from it (a5, a15 part b, a19) and are
folded in; the fourth entry lists them.

## Problem

BuildWarden's loop has rules for one repository. They are in
`docs/development/process.md` (FLOW-01 to FLOW-20) and
`docs/development/behavioral-linter.md` (LINT-01 to LINT-08), enforced by
`wf` (`tools/workflow/`), tier 1 (`tools/tier1/`), four skills and six agent
definitions under `.claude/`, with state on the `workflow` branch. All of it
names this repository's paths, commands and rules: `make lint`, `docs/design/`,
`overview.md`, GLOBAL-06, the drivers.

The componentization program gives every component its own repository. As
approved on 2026-11-12 it also gave each its own queue (Decision 2 there)
and moved the loop to its own public repository (Decision 13). On 2026-11-14
Jeff chose one queue for work in every repository, and kept the move to the
loop repository, as the last item of P1 (**Design review**, a19; the
umbrella's sentences that one queue changes are listed under **Amendments
to the umbrella design**). The relay repository
already exists, is written in Rust, and has "the Loop Contract adopted" in
its first milestone (M1). What it
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
   invariants, and which are loop rules that bind work in every repository
   and move to the loop repository in P1's last item. The umbrella says
   "FLOW-*, LINT-*: move to the loop repository as the Loop Contract's
   detail" and leaves the rest to P1.
3. **A way for a second component to read rules written for this one.** The
   relay cannot run `make lint` or update `overview.md`. The contract has to
   say what is the component's to define, and must not assume a language or
   a toolchain.

The PR also carries one small change to the loop's machinery, `wf new
-hold` (a11). A second, the refreshed Entire hook configuration (a12), is
carried only if `alters-slot`'s review does not take it. Neither the
umbrella's P1 scope nor this item's `source` names them. They are here
because each needs a process item to reach `main` (FLOW-14) and this one
holds nothing. Each is its own question, so the review can move either out
without touching the rest. A third, the
`alters_relationships` limit that `wf next` does not read, was found while
reading `wf` for this design and was a13 in the text read at `2889919`. It
is now its own item, `alters-slot`, filed on 2026-11-14 at the head of the
open items (a13 is retired here).

This item holds no worker (its queue file has no `holds`), so nothing in its
PR may disrupt running work. S1 (`seam-specs`) awaits its design review, and
K1 (`driver-contract`) is back in Design after its review of 2026-11-14.
Their branches keep the rule text they forked with until they
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
  Jeff's direction of 2026-11-14 is later than both: one queue. The loop
  repository of Decision 13 stands, and the move to it is P1's last item
  (a19). Each sentence of the umbrella that one queue changes is listed
  under **Amendments to the umbrella design**, for his approval by name.
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
- `seam-specs` (S1, on `feat/seam-specs`, awaiting its design review)
  proposes how a seam change is ordered across two repositories and how this
  repository pins a specifications release. The contract states the order
  and leaves the pin to the component (LOOP-02), so it does not depend on how
  that review ends.
- `alters-slot` (a process item, in Design; its design is on
  `feat/alters-slot` at `ed99489`, not yet reviewed) takes over a13:
  `wf next` and `wf validate` count the items that alter relationships
  against `limits.alters_relationships`. Its Decision 8 and its a2 propose
  that its PR also carries the two Entire files of a12 here, in a commit of
  their own. It is meant to merge first, so a12 now recommends that it
  does.
- `driver-contract` (K1) was sent back to Design on 2026-11-14 to become a
  contract of commands that `warden` calls on a driver binary. Whether that
  contract gets a written specification is for its redraft to say. This
  design no longer uses it as an example (LOOP-02, a8).

Outside this repository, read for this design and not changed by it:

- **The Loop Contract draft**, Jeff, 2026-11-07
  (https://claude.ai/artifact/UY5WXovAA5phiAHU4LDbK9, read at revision 7).
  Its eight sections, three decided points and five blanks are the starting
  material. The document carries no date; 2026-11-07 is the date the relay's
  M1 design gives it.
- **The relay fork's decisions table** (read at revision 20) and
  **kickoff** (revision 12), both 2026-11-07, for what work in the relay
  repository needs of the loop: decisions 2, 3, 5, 6, 7, 8 and 16 of the
  table, and the kickoff's "Loop contract" and "Design constraints"
  sections.
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
- **The loop change proposal**, 2026-11-14, put to Jeff in the attended
  session after the first review sitting. It is in no repository. Its
  summary and his answer are recorded under **Design review**. This design
  takes from it only what the contract has to say today: one queue, an item
  that names its repositories, and the order of the later items. What it
  proposes for FLOW-05, FLOW-04 and FLOW-09 belongs to those items and is
  not written here.

## Decisions

Three points were decided by Jeff in the draft. One is carried as given: a
change to the contract is gated on Jeff only. The other two, that the
contract lives in its own repository apart from the project (the umbrella's
Decision 13 names it) and that the queue and the iteration reports live
there too, are carried as well: they are met when P1's last item moves the
loop there (Decision 19, a19). Until then the contract is in
`docs/development/` here and the queue and the reports are on the
`workflow` branch. The decisions below are this design's. Every
one that departs from the draft, or from the text Jeff read at `2889919`, is
also a question, so the review confirms it: the larger departures each have
their own (a4 to a8, a10, a14, a16 to a21), and the smaller ones are asked
together in a15. Decisions 17 to 20 are new since `2889919`. Decisions 7,
12 and 19 are rewritten for Jeff's answers to a5, a15 (part b) and a19 at
the second sitting. What the answers to a5 and a19 leave open is a23 and
a22. The answer to a15 (b) raises no question of this item's: its open
points are `loop-native-lead`'s (hand-off, point 7).

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
   written specification, such as the bootstrap model, whose contract
   document is `bootstrap.md` (BOOT-07).
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
7. **A reconciliation item is a feature item, and FLOW-05 ranks it with
   the fixes** (a5; Jeff's answer of 2026-11-14, which chose the third
   option over the one this design recommended). The draft files
   "adopt consumable X at version Y" as process work. In the running loop a
   process item changes rules and has no Plan, Review, Guide or
   consolidation; an adoption changes code and needs all four. So it is a
   feature. Jeff ranks it in FLOW-05's class 3, ahead of new
   functionality: "A feature that FLOW-05 orders ahead of new work,
   alongside regression fixes. Needs a FLOW-05 amendment." LOOP-07 says
   that it is a feature and that FLOW-05 gives its place in the order.
   The class itself is FLOW-05's to state, and `loop-parallel` amends
   FLOW-05 and `wf` to state it, with his decision recorded in the
   hand-off (point 7) so that item does not reopen it (a23). *Rejected:*
   the priority intake gives it among new functionality, which this design
   recommended at `04cc9bb`. Jeff chose otherwise. *Rejected:* a process
   item, as the draft has it. *Rejected:* "ranked with the fixes" written
   into LOOP-07. FLOW-05 defines class 3 as a feature that `follows` an
   item, and `wf` computes the class from that field alone, so `main`
   would contradict the contract on merge (the umbrella's Decision 18),
   and the order would be stated in two rules (FLOW-14). *Rejected:*
   amending FLOW-05 and `wf` in this PR (a23's second option). a1 keeps
   this PR to what the rules say, `loop-parallel` amends FLOW-05 and the
   queue's schema next, and no reconciliation item can exist before a
   second component releases something.
8. **The split is made in place, by file, and no file moves in this PR**
   (a2). After it, three files are wholly loop-side
   (`loop-contract.md`, `process.md`, `behavioral-linter.md`), the others
   are wholly project-side, and the README says which is which. *Rejected:*
   moving the loop files into `docs/development/loop/` now. Tier 1's rule
   index globs `docs/development/*.md` only, so the move needs an
   enforcement change, and every link to `process.md` would break now and
   again when the files leave. *Rejected:* making no change until the loop
   repository exists. The relay's M1 would have nothing to adopt until P1's
   last item (Decision 19).
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
    design may declare a change to the invariants and profile of a
    component it works in, and to the checks that enforce them (FLOW-14's
    second route).
    It may not declare a change to a LOOP, FLOW or LINT rule, or to a check
    that enforces one, wherever that check is kept. The rule ID on the
    `rule_changes` entry decides, not the path: `tools/tier1/` and
    `tools/workflow/check/` hold checks of both kinds.
    *Rejected:* keeping the second route for loop rules. A rule that
    binds work in every repository should not change as a side effect of
    one component's feature, and once the rules move to the loop repository
    a feature's PR cannot edit them.
11. **The contract shares no project invariant.** GLOBAL-09 to GLOBAL-11,
    GLOBAL-06 and the testing rules stay this repository's. Another
    component declares its own invariants and may cite one of these by ID
    as binding there (the specs repository's a4 in `seam-specs` does).
    *Rejected:* promoting a "BuildWarden-wide" subset into the contract. The
    draft keeps concrete invariants local, and a Rust relay and a Go kit do
    not share a comment or error-wrapping rule.
12. **The table of host names is shared loop state; schedules and
    credentials are each machine's; who takes a step is FLOW-04's to say**
    (a15). The draft puts "which machines" in the
    local column. The same two machines serve every component, and a
    machine runs one native session at a time (`native-tick.sh` takes a
    lock). A table per component could not hold that. One `hosts.yaml`
    beside the queue says which host names each machine serves, for work
    in every repository. A component's profile lists the host names its
    work uses.
    The contract does not say which worker takes a step: LOOP-05 cites
    FLOW-04 for that. Today FLOW-04 and `hosts.yaml` give every host name,
    `<capability>-any` included, to exactly one machine. Jeff decided on
    2026-11-14 (a15, part b) that work any machine with a capability can
    do goes to "whichever loop supports it and next can pick it up, placing
    a refreshable 20-hour lease while actively working on it". That
    changes FLOW-04, the host table and the lease, and `loop-native-lead`
    makes the change (hand-off, point 7). *Rejected:* the one-machine rule
    for `<capability>-any` stated in LOOP-05, as read at `04cc9bb`. It is
    the rule Jeff changed. *Rejected:* his rule stated in LOOP-05 now.
    `wf validate` rejects a host name that two machines serve and a lease
    is 120 minutes, so `main` would contradict the contract on merge (the
    umbrella's Decision 18), and FLOW-04 would be stated in two places
    (FLOW-14).
13. **The line between this item and P1's later items** (a1): this item
    changes what the rules say, and says only what is true of the loop as
    it runs today. `loop-parallel`, `loop-native-lead` and
    `loop-repository` change what `wf` does and the FLOW rules that go with
    it, and P1's last item moves the loop to its repository. See **What
    this item hands to the later items**.
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
15. **A repository is a component once it has adopted the contract; an
    item may work in one that has not** (a17). The umbrella's component
    table lists the specs repository, the loop repository and the Hermit
    forks beside the relay and the kit. With one queue the open point is no
    longer when each gets a queue. It is which rules bind a worker in a
    repository that has no profile and no invariants of its own, and S1
    already works in one (the specs repository; `seam-specs`, its a4). The
    contract says which rules bind there, and leaves when each repository
    adopts to the item that does it. *Rejected:* requiring every repository
    to adopt before any work in it. S1 could not run in wave 1.
16. **The draft's specification invariant is kept where a specification has
    a conformance suite** (a14). "Every behaviour in the spec has a test;
    every test maps to a spec section" becomes LOOP-03's class 2 for such a
    specification. *Rejected:* requiring it of every specification now. The
    ledger format under `specs/` has contract tests and coupling checks and
    no mapping of sections to tests, so `main` would contradict the rule on
    merge (the umbrella's Decision 18).
17. **One queue, one order and one place of reporting; an item names the
    repositories it works in** (a18). This is Jeff's direction of
    2026-11-14. A component is still a repository that has adopted the
    contract, and its profile, invariants, designs and checks stay in it.
    It has no queue of its own. Until the queue has a field for them, an
    item names its repositories in its design, and one that names none
    works only in the repository that holds its design; the PR adds the
    template sentence and the skill lines that ask for it, so the clause is
    true on merge. *Rejected:* a queue per component, which
    the umbrella's Decision 2 approved and this design carried at
    `2889919`. Jeff: splitting the queues apart "will make it harder to
    track". One set of workers serves every repository in either case, so
    separate queues would have added a rule for dividing a run between
    them and saved nothing. *Rejected:* one queue with a digest per
    component. The reports would be split again, which works against the
    tracking he asked for.
18. **LOOP-04 says there is one queue and cites FLOW-05 for the order and
    for what may run side by side; it states no relation between items and
    no limit** (a18). FLOW-05 as it stands is then the contract's detail on
    the day this merges, and the contract stays true when `loop-parallel`
    changes FLOW-05. A hold is treated the same way: LOOP-04 and LOOP-09
    say that a process item declares `holds`, and FLOW-05 says what a hold
    names, which today is workers. One clause of the text read at
    `2889919` that the direction does not touch is kept, reworded: an item
    that waits on another component waits for a release, held by a blocking
    question. *Rejected:* writing the relations and limits of the
    change proposal into the contract now. `main` would contradict its own
    rules until `loop-parallel` lands (the umbrella's Decision 18), and
    FLOW-05 would be stated in two places (FLOW-14). *Rejected:* saying in
    LOOP-04 and LOOP-09 that a hold may name repositories. FLOW-05 gives a
    hold only affinity patterns and `wf validate` rejects any other, so
    the clause would not be true on merge; the change proposal gives it to
    FLOW-05, in `loop-parallel`. *Rejected:* holding
    this item until `loop-parallel` has landed. The relay's M1 would have
    nothing to adopt meanwhile, and this review would wait for an item not
    yet filed.
19. **The move to the loop repository stays in P1, as its last item**
    (a19; Jeff's answer of 2026-11-14, which chose the second option over
    the one this design recommended). The umbrella's Decision 13 stands:
    the loop gets its own public repository, and P1 is not done until the
    contract, the queue, the reports, `wf`, the skills and the agents are
    in it. P1's items, in order, are `loop-contract`, `alters-slot`,
    `loop-parallel`, `loop-native-lead`, `loop-repository` (work in a
    second repository from the one queue) and then the move. The move is
    not yet filed. This design calls it `loop-move`, a provisional name;
    the repository's own name is confirmed with Jeff right before it is
    created (the umbrella's a11). This item moves nothing (Decision 8). It
    makes the split by file, so that the last item's change is only a
    move, and it lists what moves, what stays and which links change
    (hand-off, points 2 and 3). Wherever the text read at `04cc9bb` said
    "would move if the loop repository is created", it now says that it
    moves in P1's last item. *Rejected:* deferring the move with no date,
    which this design recommended at `04cc9bb` because the move pauses
    every worker and the queue is already one, and public, where it is.
    Jeff kept the move. *Rejected:* the move as P1's second item, as read
    at `2889919`. The item named `loop-repository` now makes the one queue
    work in a second repository, and the move follows it. *Rejected:*
    dropping the umbrella's Decision 13. Whether wave 2 waits for the move,
    or only for the queue working across repositories, is a22.
20. **The umbrella's wording is amended in this item's PR, sentence by
    sentence, each approved by name** (a20). The sentences are listed
    under **Amendments to the umbrella design**. *Rejected:* leaving the
    umbrella as approved and letting this design override it. FLOW-20
    defines the program by that document, and later designs retrieve its
    archive summary; two archived designs would disagree on whether the
    relay has a queue. *Rejected:* a process item of its own for the
    amendments. It is one more sitting for wording this review already
    decides. FLOW-20's own text is not amended here: what it says about
    the queue and its limits changes with FLOW-05, in `loop-parallel`.

## Questions

### Blocking

None. A process item's design is settled in its design review, where every
assumption below is asked.

### Assumed

Jeff answered twenty at the second sitting on 2026-11-14: a1 to a12 and
a14 to a21, on the text at `04cc9bb`. Three answers departed from the
recommended option (a5, a15 part b, a19) and are folded in; each of those
three shows the option he chose and what it changed. Two are open, a22
and a23, which the answers to a19 and a5 raised: each is "proceeding with
the recommended option; confirm". The same twenty-two are in the queue
file. a13 is retired (see **Retired** below).

Against the text read at `2889919`:

| | Questions |
|---|---|
| Unchanged | a2, a3, a6, a7, a11, a14, a16 |
| Reworded for one queue or the deferred loop repository | a1, a4, a5, a9, a10 (one clause: "once" the files leave becomes "if"), a15 (item a), a17 |
| Reworded for another reason | a8 (its example no longer depends on `driver-contract`); a12 (`alters-slot`'s design proposes to carry the files, and the recommendation is now that it does) |
| New | a18 (one queue), a19 (the loop repository deferred), a20 (the umbrella's amendments), a21 (trunk protection: `main` has none and the relay cannot have it while private; a finding of this revision, not part of the direction) |
| Retired | a13 (now `alters-slot`) |

- **a1. Where the line between `loop-contract` and P1's later items
  falls.** *(reworded)*
  1. *(recommended)* This item: the contract text (LOOP-01 to LOOP-09),
     worded for one queue and true of the loop as it runs today; this
     repository's profile (PROFILE-01); the split made in place; the three
     amendments (FLOW-01, FLOW-03, FLOW-14); the template's new section; the
     skill and agent edits that make the contract true here; and the
     amendments to the umbrella's wording (a20). The later items get what
     the hand-off table gives each (**What this item hands to the later
     items**). `loop-parallel`: FLOW-05 and the rules that go with it (the
     change proposal lists FLOW-02, -03, -05, -10, -15, -19 and -20), and
     the queue's record of the repositories an item names.
     `loop-native-lead`: the rules for an item a native machine leads (the
     proposal lists FLOW-04, -09, -13, -16 and -18). `loop-repository`:
     `wf`'s gates and checks in a second repository, and `wf` reading a
     profile. Each item's design settles its own list. The move to the
     loop repository: P1's last item, not yet filed (a19). *(At `04cc9bb`
     this sentence read "deferred (a19)"; a19's answer changed it.)*
  2. This item also rewords FLOW and LINT to neutral terms (see a9).
  3. This item waits for `loop-parallel` and lands with the FLOW-05 change.
     The relay's M1 would have nothing to cite meanwhile, and this review
     would wait for an item not yet filed.
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
- **a4. Classes of work.** *(reworded)* The draft has two classes, with
  process work at the top of the queue and outside the feature queue.
  1. *(recommended)* FLOW-05's four classes stand (in implementation,
     process items, fixes, new functionality). "Feature" and "process" are
     the two kinds of item in the one queue. A process item ranks
     second and stops other work only through the `holds` it declares.
  2. The draft's two classes: every process item ahead of everything,
     including items in implementation. FLOW-05 is amended, and an
     in-flight implementation is interrupted by every process item.
- **a5. The kind of a reconciliation item** ("adopt consumable X at
  version Y"). *(reworded)*
  1. *(recommended at `04cc9bb`)* A feature item in the queue that names the consumer's
     repository, at the priority Jeff gives it at intake. It changes code,
     so it gets Plan, Review, Guide and consolidation, and its design
     review.
  2. A process item, as the draft says. It would skip the tier-2 report and
     the consolidation pass, and FLOW-03's definition of a process item
     would have to grow to cover code.
  3. *(chosen)* A feature item that FLOW-05 ranks with the fixes (class
     3). Needs a FLOW-05 amendment.

  **Answered 2026-11-14, second sitting: option 3.** As put to him: "A
  feature that FLOW-05 orders ahead of new work, alongside regression
  fixes. Needs a FLOW-05 amendment." A reconciliation item is a feature
  item in the queue that names the consumer's repository, with Plan,
  Review, Guide, consolidation and its design review as in option 1, and
  it ranks in FLOW-05's class 3, not at the priority intake gives it among
  new functionality. Folded in: Decision 7; LOOP-07's point 2; row 9 of
  the table of disagreements; the row of **Defaults and overrides**; the
  hand-off (the `loop-parallel` row and point 7). Which item amends
  FLOW-05 and `wf` is a23.
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
  cannot be met.** *(reworded: the example)*
  1. *(recommended)* Spec-first applies to behaviour that a seam's written
     specification defines. A declared seam without one (the bootstrap
     model, whose contract document is `bootstrap.md`) is design-first, and
     its design says what defines it. Whether the driver contract gets a
     specification is for `driver-contract`'s redraft to say; the rule
     reads the same either way. The item
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
  one.** *(reworded)*
  1. *(recommended)* Through its profile and LOOP-01's reading rule, as of
     this PR. The rewording of FLOW and LINT to neutral terms is a process
     item of its own, filed after P1, with the profile's term list as its
     specification. *(reworded: there is no loop repository's queue for it
     to be first in)*
  2. Reword in this PR.
  3. Reword in one of P1's later items, which amend FLOW rules anyway. The
     rewording would then be reviewed together with a change in what the
     rules require. *(reworded: `loop-repository` is no longer a move)*
- **a10. Who may change a loop rule.**
  1. *(recommended)* Only a process item (LOOP-09). FLOW-14 is amended by
     two sentences and FLOW-03 by one: a feature's design may not declare a
     change to a LOOP, FLOW or LINT rule, or to a check that enforces one.
     The guard is the design review, which refuses such a `rule_changes`
     entry; no enforcement code changes. Once the loop files leave this
     repository (P1's last item), the rule text and `tools/workflow/` are
     out of a feature's reach. Tier 1's checks of FLOW and LINT rules
     (`rule-index`, `design-front-matter`, `drivers-claim`) stay in
     `tools/tier1/` here, and the review remains their only guard until
     `loop-repository` decides where tier 1's generic checks live. *(At
     `04cc9bb` "once" the files leave was "if"; a19's answer makes it
     "once" again, as read at `2889919`.)*
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
  `.entire/settings.json`). *(reworded)*
  `alters-slot`'s design (`feat/alters-slot` at `ed99489`, its Decision 8
  and a2, not yet reviewed) proposes that its PR carries the same two
  files, and that item is meant to merge first. The recommendation here
  has therefore moved from the first option to the third, so the two
  designs do not each recommend their own PR.
  1. This PR carries the two files as that commit has them. `.claude/`
     reaches `main` only through a process item (FLOW-14), and this one
     holds nothing. This is the fallback if `alters-slot`'s review answers
     its a2 with "`loop-contract` carries it". *(recommended at
     `2889919`)*
  2. Its own process item.
  3. *(recommended)* `alters-slot` carries it, and it is dropped here,
     task 10 and the two rows of the PR's table with it. *(reworded: this
     option named `loop-repository`, "since that item reworks `.claude/`",
     which it no longer does)*
  4. Drop it.
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
       component. Contract: one host table, beside the queue, for work in
       every repository; a component's profile lists the host names its
       work uses (Decision 12). *(reworded: "shared by every component")*
     - (b) *Any-OS work.* Draft: it "goes to whichever taker is free".
       Contract, as read at `04cc9bb`: a `<capability>-any` host name
       belongs to one machine, as `hosts.yaml` gives it today (FLOW-04).
       *(Changed by Jeff's answer, below. The contract now says that such
       work carries `<capability>-any` and cites FLOW-04 for who takes
       it.)*
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

  **Answered 2026-11-14, second sitting: all but (b).** (a), (c), (d),
  (e), (f) and (g) stand as proposed. On (b), Jeff: "All except 'b' are
  good. B should be changed to whichever loop supports it and next can
  pick it up, placing a refreshable 20-hour lease while actively working
  on it". So work that any machine with a capability can do goes to
  whichever worker has the capability and next takes steps, and that
  worker holds it under a 20-hour lease that it can refresh while it is
  actively working. It does not belong to one machine. Today FLOW-04,
  `hosts.yaml` and `wf validate` give each host name to exactly one
  machine, and a lease is 120 minutes for every claim, so his rule needs
  FLOW-04, the host table and the lease to change. Under a1 those are a
  later item's, and `loop-native-lead` is the one that changes FLOW-04,
  the host names and the wrapper. Folded in: LOOP-05 no longer states the
  one-machine rule and cites FLOW-04 for who takes a step; Decision 12;
  row 17 of the table of disagreements; and the hand-off, which records
  his rule as decided with the points that item settles (point 7). FLOW-04
  stands as it is until that item lands.
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
- **a17. Repositories that have not adopted the contract.** *(reworded:
  the second half, "where a loop process item is queued", falls away with
  the queue per component; every item is in the one queue)*
  1. *(recommended)* A repository is a component once it has done LOOP-08's
     steps. Before that, an item may work in it; the worker is bound there
     by FLOW-08, FLOW-16, FLOW-17 and FLOW-18, and the item's design says
     which invariants apply there, by rule ID (as `seam-specs` a4 does).
     Once it adopts, it and the other components couple only through
     releases (LOOP-07). The specs repository and the Hermit forks adopt
     when Jeff files the item that does it.
  2. Every repository in the umbrella's component table adopts (a profile
     and its invariants) before any work in it. S1 would wait for P1.
  3. The contract says nothing; each such item states its own rules, as
     `seam-specs` a4 does now.
- **a18. One queue.** *(new)* The draft has a queue per component ("nothing
  in one component's queue blocks another component's queue"), the
  umbrella's Decision 2 approved that, and the text read at `2889919`
  carried it. Jeff's note of 2026-11-14 asks for a single queue, and his
  answer to the change proposal approved it as the direction. This question
  confirms how the contract says it. One thing is outside the queue on the
  day this merges: the relay's M1a, which its design plans to work by hand
  (its a17). LOOP-04 binds the work the loop does, and the loop does not
  work in the relay repository until P1's later items let it (U13).
  1. *(recommended)* One queue, one order and one place of reporting, for
     work in every repository. An item names the repositories it works in
     (in its design until the queue has a field, `loop-parallel`), and one
     that names none works only in the repository that holds its design;
     the design template, the intake skill, the design review and
     `wf-design` each gain a line so that this is asked and written.
     A component is a repository that has adopted the contract, and has no
     queue of its own. LOOP-04 cites FLOW-05 for the order and for what may
     run side by side, and states no relation between items and no limit,
     so it is true with FLOW-05 as it stands and stays true when
     `loop-parallel` changes FLOW-05. A hold names workers, as in FLOW-05;
     whether one may also name repositories is `loop-parallel`'s. The
     preamble, LOOP-01 and LOOP-05 to LOOP-09 are reworded from a
     component's queue to items and repositories, and the FLOW-03 and
     FLOW-14 amendments from "its own component's" rules to those "of a
     component it works in"; the reworded clauses are listed under
     **Design review**. LOOP-04 keeps the sentence on an item that waits
     for another component's release. LOOP-08 has six steps where it had
     seven: adoption no longer includes having a queue.
  2. The same, with the relations between items and the limits written
     into LOOP-04 now. `main` would contradict it until `loop-parallel`
     lands, and FLOW-05 would be stated twice.
  3. Hold this item until `loop-parallel` has landed, and write LOOP-04
     against the new FLOW-05.
  4. A queue per component, as read at `2889919`.
- **a19. The move to the loop repository.** *(new at `04cc9bb`, where its
  title was "The loop repository is deferred")* Two of the draft's
  three decided points put the contract, the queue and the reports in a
  repository of their own, and the umbrella's Decision 13 names it. The
  text read at `2889919` made the move P1's second item.
  1. *(recommended at `04cc9bb`)* Deferred, with no date. The contract and its detail
     stay in `docs/development/` here, the queue and the reports on the
     `workflow` branch, and `wf`, the skills and the agents where they are.
     The split by file is made as designed, so that a move stays a move.
     The contract's "Where things are" table has a column "If a loop
     repository is created". Taking the move up again is a process item
     Jeff files.
  2. *(chosen)* Keep the move in P1, as its last item after
     `loop-repository`'s new scope.
  3. Drop it: the loop's rules and tools stay in `buildwarden/buildwarden`
     for good, and the umbrella's Decision 13 is withdrawn. The contract's
     table then loses its second column.

  **Answered 2026-11-14, second sitting: option 2.** The move is not
  deferred. It is P1's last item, after `loop-repository`, so the
  umbrella's Decision 13 stands and P1 is not done until the loop has
  moved. Folded in: Decision 19; the contract's "Where things are" table
  and the sentence under it; the split and the cross-references; the
  README paragraph; the hand-off, which gains the move's item
  (`loop-move`, a provisional name) and what it gets; what a second
  component needs; and the amendments to the umbrella, re-derived one by
  one. What the answer leaves open is a22.
- **a20. The amendments to the umbrella design.** *(new)* One queue changes
  sentences of
  [`componentization`](componentization.md), an archived design whose
  Decisions 1 to 17 are Jeff's. *(At `04cc9bb` this read "One queue and the
  deferred loop repository change sentences"; a19's answer takes the
  deferral's amendments back.)* They are listed, each with its line, its
  current wording and its proposed wording, under **Amendments to the
  umbrella design**.
  1. *(recommended)* All twenty-two as listed, made in this item's PR, in
     place, with a new **Amendments** section in the umbrella that records
     the date, this item and which sentences changed; the wording each had
     stays in this design. Decision 2,
     Decision 13 and a5 keep their original sentence and gain the
     amendment after it. FLOW-20's text is not touched; `loop-parallel`
     declares that change.
  2. The same, with the ones Jeff names worded differently or left out.
  3. The umbrella's text is left as approved, and only the **Amendments**
     section is added, saying which sentences no longer hold. A reader of
     the Decisions or of the archive summary would then meet the old
     wording first.
  4. No change to the umbrella in this PR; a process item of its own
     carries them.

  **Answered 2026-11-14, second sitting: option 1**, with the amendments
  that were worded for a deferred loop repository re-derived for a19's
  answer and shown to him again before they are approved. Of the
  twenty-two, nine stand as read, six are reworded and seven are withdrawn
  (**Amendments to the umbrella design**). Decision 13 is no longer
  amended, so the option's "Decision 2, Decision 13 and a5" is now
  Decision 2 and a5.
- **a21. Trunk protection: `main` has none, and the relay cannot have it
  while it is private.** *(new; a finding, not part of the direction)*
  LOOP-08's step 6, as read at `2889919`, asks a component to protect its
  trunk "so that only a human merges" (FLOW-08). The same rule says no loop
  rule may require a public repository. Read with `gh api` on 2026-11-14:
  - `buildwarden/buildwarden` is public and deletes head branches on merge.
    `main` has no branch protection and no ruleset. The text read at
    `2889919` listed the setting as not verified and said nothing in the
    nine rules contradicts `main`.
  - `buildwarden/buildwarden-relay` is private and keeps head branches on
    merge. Its protection endpoint answers 403, "Upgrade to GitHub Pro or
    make this repository public to enable this feature". Its M1a plans
    "branch protection once the repository is public".

  So the first component does not meet step 6 today, and the second cannot
  while it is private on the current plan. The change proposal found both
  and names protecting `main` as what `loop-native-lead` waits for.
  1. *(recommended)* Step 6 asks for protection where the host offers it:
     "seeing that only a human merges to its trunk (FLOW-08), and
     protecting the trunk where its host offers that". Jeff protects `main`
     before this item's PR merges, since the host offers it here; which
     protection is his to choose, and the PR body lists it under "Before
     the merge". The relay may adopt while private. It protects its trunk
     from the day it is public, as its M1a plans. Until then FLOW-08 is
     held there as it is here today: by the rule and the workers'
     settings, with no repository setting behind it. The relay also turns
     on the deletion of head branches at adoption (FLOW-16). The proposed
     text of LOOP-08 has this wording.
  2. Step 6 as read at `2889919`, protection required outright, and Jeff
     protects `main` before the merge. "A public repository" then leaves
     LOOP-08's list of what no loop rule may require, or is qualified: a
     private repository on a plan without protection cannot adopt, and the
     relay's M1b waits for the repository to go public or for the plan to
     change.
  3. Step 6 says "only a human merges to its trunk (FLOW-08)" and does not
     ask for protection anywhere. FLOW-08 then rests on the agents' deny
     rules in every repository, this one included.
  4. Leave the rule as read and the settings as they are, and record the
     gap: the contract asks for it, neither repository has it, and
     `loop-native-lead` closes it for `main`.
- **a22. What the wave-1 gate waits for.** *(new on 2026-11-14, raised by
  a19's answer; open)* The umbrella's order table gives wave 1 (P1, S1,
  M1) the gate "Loop repository live; suites pass on the Go relay" (line
  326), and wave 2 is S2, S3, S4, K1 and M2. As approved on 2026-11-12, P1
  moved the loop as part of making it span repositories, so the loop
  repository being live and the loop spanning repositories were one
  event. They are now two: `loop-repository` makes the one queue work in a
  second repository, and the move is the item after it. The umbrella gives
  one reason for where P1 sits: "P1 comes before K3, because a second
  repository needs a loop that spans repositories" (lines 334-335). K1 is
  a wave-2 item and is already in Design, which the umbrella allows ("can
  start as soon as P0 merges").
  1. *(recommended)* Wave 2 waits for the queue working across
     repositories, and not for the move. The gate reads "The one queue
     works across repositories; suites pass on the Go relay" (U12): an
     item in this repository and an item in the relay repository have each
     completed from the one queue, which is what `loop-repository`
     delivers. The move runs beside wave 2. P1 is done when the loop has moved
     (U9), and "P1 comes before K3" is left as written, so the move lands
     before wave 4. Nothing in wave 2 depends on where the loop's files
     are: S2 to S4 and K1 work in this repository and the specs
     repository, and M2 needs the queue to hold a relay item. The move
     holds workers while it runs, whichever wave it runs beside.
  2. Wave 2 waits for the move. The gate stays as the umbrella has it,
     "Loop repository live; suites pass on the Go relay", and U12 is
     withdrawn. S2, S3, S4 and M2 then wait for all six of P1's items, the
     last of which is not yet filed. The queue moves with fewer items in
     flight.
  3. As option 1, and K3 does not wait for the move either. Lines 334-335
     are amended to say that the queue working across repositories comes
     before K3. That is one more amendment, and the move could then slip
     behind the driver extractions, each of which adds a repository whose
     links to the contract the move has to change. U22 would come back if
     it slipped past K4.
- **a23. Which item makes a reconciliation item's rank true.** *(new on
  2026-11-14, raised by a5's answer; open)* Jeff's answer to a5 needs a
  FLOW-05 amendment. FLOW-05's class 3 is "a feature that `follows` an
  item, fixing or finishing it. It starts only once that item is done",
  and `wf` computes the class from the `follows` field alone (`Class`, in
  `tools/workflow/queue/queue.go`). So the rank is true only once
  FLOW-05's text and that code change together (the umbrella's
  Decision 18). No reconciliation item exists today, and none can before a
  second component releases something that another adopts; the first the
  umbrella plans are M6's.
  1. *(recommended)* `loop-parallel` makes it true. LOOP-07 says a
     reconciliation item is a feature item and that FLOW-05 gives its
     place in the order, which holds on merge with FLOW-05 as it stands.
     `loop-parallel` amends FLOW-05's class 3 and gives `wf` the means to
     compute it. The hand-off records Jeff's decision as taken (point 7),
     so that item's design settles how and does not ask whether.
     `loop-parallel` amends FLOW-05 and the queue's schema anyway, and a1
     leaves `wf` and the FLOW rules to the later items. Until it lands,
     FLOW-05 ranks a reconciliation item as it ranks any feature: with new
     functionality, or with the fixes if intake files it with `follows`.
  2. This item's PR makes it true. `rule_changes` gains FLOW-05 (amend,
     `docs/development/process.md`): class 3 becomes fixes and
     reconciliation items, and LOOP-07 can then say "ranked with the
     fixes". The machinery it needs: a field in the queue file that marks
     a reconciliation item, read by `Class` in
     `tools/workflow/queue/queue.go`; a flag on `wf new`
     (`cmd/wf/main.go`); the Kind column of `QUEUE.md`
     (`queue/summary.go`, `kindLabel`); tests in `queue/queue_test.go`;
     the intake skill; and the `workflow` branch's `README.md` after the
     merge. It is a second `wf` change in a PR that a1 keeps to the rules,
     FLOW-05 is then amended by two items in a row, and the field is
     designed before the queue records an item's repositories, which is
     `loop-parallel`'s.
  3. This item's PR amends FLOW-05's text only, with no field: intake
     files a reconciliation item with `follows`, naming the item that
     produced the release. It needs no code. But `wf validate` rejects a
     `follows` that names no item in the queue, so a release cut outside
     the queue, such as the stub pre-release the relay's M1a plans by
     hand, has nothing to follow, and FLOW-05 would promise a class that
     `wf` cannot always give.

### Retired

The `workflow` branch's `README.md` gives an assumed question two states,
`open` and `confirmed`, and only the human moves it from one to the other
(`wf answer`, FLOW-07). It has no state for a question that no longer
applies. A retired question is therefore taken out of the queue file's
`questions`, with a line in the item's `history`, and is kept here with the
reason. Its number is not reused.

- **a13. `wf next` allows one feature past Design with
  `alters_relationships: true`, whatever `config.yaml` says.** Retired on
  2026-11-14. It is now the process item `alters-slot`, filed that day by
  Jeff's intake at the head of the open items (the first step of the change
  proposal's migration). Its recommended option at `2889919` was to fix
  `wf next` in this PR; its second option was "its own small process item,
  filed now, if S1 or K1 would reach Plan before this item merges", which
  is what happened. The task (10 at `2889919`), the row of **Defaults and
  overrides** for an unset `limits.alters_relationships`, the dropped
  fallback and the tests leave this design with it; `alters-slot`'s design
  decides them.

## Design

### The contract's structure

One document, `docs/development/loop-contract.md`, with a short preamble
(that there is one loop, what a component is, how a cited rule's severity
is read), a table of where the loop's parts are, and nine rules. Its full
proposed text is under **Rule changes**.

| Rule | Title | Draft section | Detail it cites |
|---|---|---|---|
| LOOP-01 | Shared rules, a local profile | 1, and 8 steps 1 and 2 | Every FLOW and LINT rule a profile term serves; the term table names them |
| LOOP-02 | Design-first always, spec-first at a seam | 2 | FLOW-01, FLOW-02, FLOW-06, FLOW-07, FLOW-19 |
| LOOP-03 | Checks: four classes, two tiers, run at the gates | 3 | LINT-01 to LINT-04, LINT-06, FLOW-03, FLOW-08, FLOW-09 |
| LOOP-04 | One queue, one order | 4 | FLOW-03, FLOW-04, FLOW-05, FLOW-07, FLOW-09 |
| LOOP-05 | Workers are scheduled, routed and identified | 5 | FLOW-04, FLOW-09, FLOW-13, FLOW-18 |
| LOOP-06 | Every run reports in one place | 6 | FLOW-05, FLOW-08, FLOW-13, LINT-04, LINT-05 |
| LOOP-07 | Components couple only through releases | 7 | LOOP-01, LOOP-02, LOOP-04, FLOW-05 |
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
| 6, decided | Where iterations report | Today: `digest/<YYYY-MM-DD>.md` on the `workflow` branch, one section per run; the tier-2 report per branch head in `reports/<item>/<sha12>.{md,json}`; each machine's last scheduled run in `native/status/<machine>.yaml`. All of it stays with the queue, and moves with it to the loop repository in P1's last item (a19). |

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
  PR's checks green (FLOW-11). No repository setting requires it: `main`
  has no branch protection (a21).
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
| 8 | The contract lives in its own repository, "likely" the sessions repository. | The umbrella's Decision 13: its own public repository; the sessions repository is private. On 2026-11-14 Jeff kept that, with the move as P1's last item. | The contract is in `docs/development/` here, split by file so that the move is only a move, until P1's last item moves it (Decision 19). | a19 |
| 9 | A reconciliation item is "process work" at the top of the consumer's queue. | A process item changes rules or machinery and has four phases with no Review (FLOW-03). The umbrella's M6: 'one "adopt relay version X" item per driver in that driver's queue' (amended here for one queue). FLOW-05's class 3, the fixes, is a feature that `follows` an item, and `wf` computes the class from that field alone (`queue.go`, `Class`). | A feature item in the queue that names the consumer's repository, which FLOW-05 ranks with the fixes, ahead of new functionality (Jeff, 2026-11-14). LOOP-07 says it is a feature and cites FLOW-05 for its place. FLOW-05 and `wf` gain the class in `loop-parallel` (a23). | a5, a23 |
| 10 | "Nothing in one component's queue blocks another component's queue." | One queue on the `workflow` branch and one set of workers: one cloud run budget, two machines, one session per machine. The umbrella as approved gave each component a queue (Decision 2) and asked P1 for "holds per component". | The draft's sentence was written for separate queues. Jeff has since chosen one queue for work in every repository (2026-11-14, **Design review**), so it has no counterpart in the contract: FLOW-05 says which items wait for which and which may run side by side (LOOP-04). What the sentence was after, that work in one repository does not stop unrelated work in another, is for `loop-parallel` to give FLOW-05. The contract does not state it. | a18 |
| 11 | Task takers: "which machines" is local to each component. | `hosts.yaml` gives each host name to exactly one machine, for the whole loop (FLOW-04). | The host table is shared; schedules and credentials are each machine's (Decision 12, LOOP-05). | a15 (a) |
| 12 | Routing for the relay: Firecracker and QEMU work to the Windows desktop. | `hosts.yaml`: `qemu-any` is on `macbook` (the qemu driver runs only on aarch64 hosts); `linux-kvm` on `windows-desktop` is experimental. | Routing is `hosts.yaml`, not contract text. The relay's profile names host names, not machines. | a15 (c) |
| 13 | "Each taker ... records which repo and which queue item it took." | The lease and history record the worker and the item. The queue file has no field for an item's repositories; an item that works outside this one says so in its design (`seam-specs`). | LOOP-05: the claim records the item, and the item names its repositories (LOOP-04). The queue's field for them is `loop-parallel`'s. | a18 |
| 14 | A change to the contract is gated on Jeff only. | Both of FLOW-14's routes end in Jeff's design review and merge, but the second lets a feature's design carry a rule change. | Loop rules: process items only (LOOP-09). | a10 |
| 15 | Section 7 names the relay's consumable by "the provisioning protocol, the connector protocol". | The decisions table's rows 9 and 11, the same day: no new protocol; the existing claim and collector protocols (GLOBAL-08). | The contract names no protocol. A consumable lists the specifications it implements (LOOP-07). | none (decided) |
| 16 | Section 3, class 2: "every behaviour in the spec has a test; every test maps to a spec section". | No mapping of specification sections to tests (the blanks table, first row). `seam-specs` proposes one for the specs repository. | Kept as class 2 for a specification that has a conformance suite; not required of one without (Decision 16, LOOP-03). | a14 |
| 17 | Section 5: "any-OS work goes to whichever taker is free". | A `<capability>-any` host name belongs to exactly one machine (`hosts.yaml`, FLOW-04): `qemu-any` to `macbook`, `container-any` to `windows-desktop`. `wf validate` rejects a name that two machines serve. A lease is 120 minutes for every claim (`config.yaml`), and nothing refreshes one short of claiming the step again. | Jeff, 2026-11-14: it goes to "whichever loop supports it and next can pick it up, placing a refreshable 20-hour lease while actively working on it". That is the draft's sentence, with a lease. LOOP-05 says such work carries `<capability>-any` and cites FLOW-04 for who takes a step; it states no one-machine rule. FLOW-04, the host table and the lease change in `loop-native-lead` (hand-off, point 7), and FLOW-04 stands as it is until then. | a15 (b) |
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
for the later items (hand-off, point 5).

### Design-first and spec-first: which applies when

A *seam* is a boundary between components that the component's rules name.
Here these are GLOBAL-08's versioned seams, which a design declares in
`seams` (FLOW-02). A *specification* is a written document that a second
implementation of a seam is tested against. Today one seam has one: the
ledger format and its schemas (`specs/`). The umbrella's seams 1 to 8 get
theirs as S1 to S3 write them, and until then the Go code on `main` defines
them (the umbrella's Archive summary). `bootstrap-model`,
`windows-guest-seed` and `image-manifest`, which designs declare in `seams`
today, have none. Whether seam 9, the driver contract, gets one is for
`driver-contract`'s redraft to say: Jeff sent it back on 2026-11-14 to
become a contract of commands on a driver binary. The table reads the same
either way. With a specification it is the first row, and without one the
third.

| The item changes | Order | Rules |
|---|---|---|
| Behaviour a specification defines: a message, field, ordering, error, limit or format at a seam | Design, design review, then in Implement the specification change first and the code second, the code tested against the changed specification | FLOW-01, FLOW-02 (`seams`), FLOW-19, LOOP-02 |
| An implementation of behaviour already specified (the Rust relay's milestones; a second collector) | Design, citing the requirements it implements. No specification change. The code is tested by the seam's conformance suite. | FLOW-01, LOOP-02 |
| A declared seam that has no written specification (the bootstrap model; a seam S1 to S3 have not yet written) | Design, design review, then code. The seam's contract document, where the component's rules name one, changes in the same change. The design says the seam has no specification and what defines it. | FLOW-01, FLOW-02 (`seams`), FLOW-19, LOOP-02; here GLOBAL-08, EXEC-04, BOOT-07 |
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

**What `seam-specs` and `driver-contract` owe the profile.** Neither is
approved, and either may merge before or after this item. If this
item merges first, `seam-specs` changes two entries, Specifications (the
specs repository and its pin) and Consumables (the specifications release
this repository consumes), so its design needs a PROFILE-01 entry in
`rule_changes` before its review. `driver-contract` is being redrafted, so
what it changes cannot be read yet. The test is the same for it: a fact a
profile entry states (the sweep's kept list, the Specifications entry if
the driver contract gets a specification) needs a PROFILE-01 entry, and a
fact in a file the profile points to does not. If either merges first, this item's
implement step writes the profile against `main` as it is then
(Implementation plan, task 2). This design does not edit theirs; the point
is for their design reviews.

A term a component does not have is filled as `none`, and a rule read
through a `none` term does not apply there (a component with no user-facing
surface owes no reference docs). A term left unfilled is a finding; the
reader does not fall back to `buildwarden/buildwarden`'s entry.

### The split, rule by rule

**Project invariant** rules stay in `buildwarden/buildwarden`. **Loop**
rules bind work in every repository; they stay here too, until P1's last
item moves them to the loop repository (a19). 61 existing rules
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
| GLOBAL-08 | Component independence | Project | It is about the product's components. The contract's "component" is a repository that has adopted the contract, defined in its preamble. |
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
| FLOW-20 | The componentization program | Loop | It is temporary and names this repository's design by a relative link. Its text is not changed here: what it says of the queue and its limits is `loop-parallel`'s to change. Jeff's process item retires it whole. |
| LOOP-01 to LOOP-09 | The Loop Contract | Loop | New. |

Unnumbered text follows its file. `behavioral-linter.md`'s "How the full
release gate composes" and `process.md`'s "Bootstrap order" are on the loop
side; `global.md`'s "Deployment modes and the ledger's role" is on the
project side. "Bootstrap order" is history by now; a later process item may
drop it.

### Rule IDs, cross-references, `rules.yaml` and tier 1

- **Rule IDs.** None changes, and none is reused. LOOP is a new prefix of
  the loop, and PROFILE a new prefix of this repository. The contract names
  the loop's three prefixes and each component names its own in its profile
  (LOOP-08), so an ID names one rule wherever it is cited.
- **Cross-references by ID** stay as they are. The project files cite six
  loop rules (FLOW-02, FLOW-06, FLOW-09, FLOW-14, FLOW-19, LINT-03), and
  the two loop files cite 22 project rules by ID. When the loop files
  move, those citations cross repositories and are still exact.
- **Cross-references by link or path.** This PR moves no file, so none
  breaks. What the move has to change, kept here for P1's last item
  (a19), from a search of
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
  (`seam-contract`, `schema-doc`). After the move, `rules.yaml`'s LINT and
  FLOW entries have no rule file here to match; the item that makes
  the move decides whether tier 1 reads the loop's rule files from the
  loop repository or the loop carries its own map.
- **Tier 1's rule index** (`tools/tier1/rules`, `Dir = "docs/development"`,
  a non-recursive glob that skips `README.md`) parses the new
  `loop-contract.md` like any rule file: nine headers, each with its
  severity and check. `loop-profile.md` adds one, PROFILE-01.
- **The README index** is restructured, not rewritten, and no section
  moves in this PR: the ID scheme table gains the `LOOP-` and `PROFILE-`
  rows and an Owner column, and the Documents list is split into "Project
  invariants" and "Loop rules". The Rule changes section gives each README
  section its side and what the move does with it.
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
| `docs/design/TEMPLATE.md` | A **Specification changes** section; one sentence under **Design** asking a design to name the repositories it works in other than this one | `rule_changes`, LOOP-02 and LOOP-04 |
| `.claude/skills/workflow-tick/SKILL.md` | The digest's three additions, LOOP-06 cited; the Never list's exception for an approved rule change excludes loop rules (LOOP-09) | Machinery, a process item's to change (FLOW-14) |
| `.claude/skills/workflow-native/SKILL.md` | The same three additions in the unattended digest section; the same qualifier in its Never list | Machinery |
| `.claude/skills/workflow-human/SKILL.md` | The design review's walkthrough, step 3: confirm the repositories the design names (LOOP-04), go over **Specification changes** (LOOP-02), and refuse a feature's `rule_changes` entry that names a loop rule (LOOP-09) | Machinery |
| `.claude/skills/workflow-intake/SKILL.md` | Step 3: agree which repositories the item works in, and put any other than this one in `-source` for Design to name (LOOP-04); a change to a loop rule is a process item, never a feature's rule change (LOOP-09). Steps 3 and 4: `wf new -hold` (a11) | Machinery |
| `.claude/agents/wf-publish.md` | The PR body's **Specification changes** section; at `f6-consolidate`, a design that declares `seams` and predates the section gets it (LOOP-02) | Machinery |
| `.claude/agents/wf-design.md` | Name the repositories the item works in other than this one (LOOP-04) and fill **Specification changes** when `seams` is not empty (LOOP-02), each even when the branch's template predates it; a feature's design declares no loop-rule change (LOOP-09) | Machinery |
| `tools/workflow/cmd/wf/main.go`, `queue/queue.go`, `queue/queue_test.go` | `wf new -hold` (a11) | Machinery |
| `.claude/settings.json` | Only if `alters-slot` does not carry it (a12): as in `ac8b41a` | Machinery |
| `.entire/settings.json` | Only if `alters-slot` does not carry it (a12): as in `ac8b41a` | Ordinary: `wf`'s clearance does not class it as machinery (`kindOf`) |
| `AGENTS.md` | Two rows: the contract and the profile | Ordinary |
| `README.md` | One sentence in **Development** linking to the contract, which LOOP-08's step 1 asks of a component's README | Ordinary |
| `docs/design/componentization.md` | The sentences listed under **Amendments to the umbrella design**, and a new **Amendments** section (a20) | Ordinary: an archived design, not a rule path. FLOW-20's text in `process.md` is not edited |
| The `workflow` branch's `README.md` | `wf new -hold` | Not in the PR. Edited on that branch after the merge, in the attended session in which Jeff merges; the PR body lists it under "After the merge" |

Nothing else. No rule file other than `process.md` is edited, `tools/tier1/`
and `tools/workflow/check/` are untouched, and no file moves. Against the
table read at `2889919`: the row for `tools/workflow/queue/next.go` is gone
with a13; the rows for `docs/design/componentization.md` and `README.md`
are new; the two settings files are carried only if `alters-slot` does not
carry them; the template, `workflow-human`, `workflow-intake` and
`wf-design` each gain the line on an item's repositories; and the
`workflow` branch's `README.md` no longer gains a line on the limit
`wf next` reads.

### The machinery changes (a11, and a12 if it stays here)

Each is a proposal the review can drop without touching the rest. A third,
the `alters_relationships` limit in `wf next`, was here at `2889919` as
a13. It is now `alters-slot`'s (see **Retired** under Questions).

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
reorder does not touch. Only `.claude/settings.json` needs a process item;
`.entire/settings.json` travels with it because the two were generated
together. `alters-slot`'s design proposes to carry both in a commit of
their own (its Decision 8 and a2), and it is meant to merge first, so a12
recommends that it does and that they are dropped here. If its review
moves them out, this PR takes the two files as that commit has them.

### What a second component needs in order to adopt

LOOP-08 is the rule. For the relay repository, concretely:

1. **A link, not a copy.** Its README and its agent context file link to
   the contract: `docs/development/loop-contract.md` on this repository's
   `main`. The link changes once, when P1's last item moves the contract
   to the loop repository. It follows the contract's trunk
   and is not pinned, so a change reaches every component at once.
2. **A profile**, every term filled or `none`. For the relay: Rust; gates
   that build its three crates for two architectures, lint and test; its
   designs under `docs/design/`; the seams it implements (claim, collector,
   relay guest API, ledger format) at a pinned specifications release;
   host names from the shared table for its VM work.
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
5. **Its consumables declared** in the profile: the relay's release forms
   (the umbrella's seam 8), the first of them the stub pre-release its M1a
   cuts.
6. **The repository set up as FLOW-08, FLOW-16 and FLOW-18 need:** only a
   human merges to trunk, with trunk protected where the host offers it;
   head branches deleted on merge; sessions recorded only to the private
   sessions repository and the FLOW-18 check run on it. As read on
   2026-11-14 the relay repository is private, keeps head branches on
   merge, and cannot be protected on the current plan (a21). It turns on
   branch deletion at adoption and protects its trunk once it is public.

Beyond the six steps: **its work in the one queue.** The relay has no
queue of its own. Its items are in the one queue and name
`buildwarden/buildwarden-relay`. This is not a step of LOOP-08. It is what
the tools owe before the loop's gates run there: the queue recording that
an item works in another repository (`loop-parallel`); `wf`'s checks,
branches and pull requests in a second repository, `wf` reading its
profile, and what of a private repository's work may be written to the
public queue and reports (`loop-repository`); and the workers' access to
it, which is the owner's to set up (LOOP-05).

Steps 1 to 6 need nothing from the later items. The point beyond them is
what `loop-repository` delivers: an item in this repository and an item in
the relay repository each complete from the one queue. Under a22's
recommended option that is the wave-1 gate (U12). P1's done-when asks for
the same after the move, from the queue in the loop repository (U9).

**Against the relay's M1 design** (`feat/m1-foundation` at `13ce286`,
unreviewed, written against this design at `2c7ff0e`). It splits adoption
the same way. Its M1a does what waits for nothing: the invariants as CI
jobs, a plain table of them in `docs/invariants.md`, the FLOW-18 guard,
branch protection once the repository is public, and the stub pre-release
that is its first consumable. Its M1b waits for this item to merge (task
10: the link, the profile, the invariants rewritten as rules in LINT-01's
format, the design template) and for the loop repository (task 11: the
queue and the relay's first item). So step 6 as far as the host allows,
the stub pre-release of step 5 and the content of step 3 are M1a; steps 1,
2 and 4, the form of step 3 and the declaration of step 5 are its task 10;
the point beyond the six steps is its task 11. It differs from this design
in three places. The first two are
for its task 10 to take up, and the third for its task 11 and its a17:

- *Routing.* It takes the relay's routing from the draft's section 5
  (Hyper-V, Windows and Linux-host work to the Windows desktop). Under
  LOOP-05 a profile names host names, and the host table, not the
  profile, says which machine serves a name; `hosts.yaml` has `qemu-any`
  on the MacBook (a15, c).
- *Registering a prefix.* It expects a prefix table in the contract, and a
  change through this repository's process to register the relay's. This
  design no longer has that table: the relay names its prefix in its own
  profile, and nothing in the contract changes (Decision 9, a16).
- *No queue of its own.* It quotes the umbrella's "the relay's milestones
  run in the relay repository's queue", notes that no such queue exists
  (its a17), and plans in task 11 "a queue for the relay" in the loop
  repository and the relay's first item taken from it. There is one queue.
  The relay's items are in it and name the relay's repository. Its task 11
  becomes the relay's first item run through `wf`'s gates from the one
  queue, which does not wait for the loop repository: the queue is on this
  repository's `workflow` branch until P1's last item moves it. How M1a is worked before the
  tools can do that is its a17's to settle with Jeff; the change proposal's
  recommendation is by hand after its design review, recorded in the queue
  once `loop-parallel` lands. That design is not edited from here.

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

### What this item hands to the later items

**The line.** This item changes what the rules say, in place, and the
contract says only what is true of the loop as it runs today. The later
items change what `wf` does, and each declares the FLOW rules it changes.
After this item merges, nothing has moved and `wf` behaves as before except
for a11.

P1 was two items at `2889919`. It is now six. The first five are in the
order of the change proposal's migration, which Jeff approved as the
direction. The sixth is the move to the loop repository, which he kept in
P1 as its last item at the second sitting (a19). The contract cites the
FLOW rules the later items change and does not write their changes.

| Item | State on 2026-11-14 | What it gets from this item |
|---|---|---|
| `alters-slot` | Filed, in Design, holds nothing; its design is on `feat/alters-slot` at `ed99489`, not yet reviewed; meant to merge first | a13 as read at `2889919`: the finding, its options, the task, the tests and the row of **Defaults and overrides**. a12's two files, which its design proposes to carry (its Decision 8 and a2). Nothing in the contract depends on it. |
| `loop-parallel` | Not yet filed | LOOP-04 as the frame its FLOW-05 change fits: the contract says that FLOW-05 orders the queue, says what may run side by side and says what a hold names, and nothing more. LOOP-04's and LOOP-05's requirement that an item names its repositories, which the queue file does not yet record. LOOP-06's one digest. FLOW-20's text and the umbrella's a5, as far as its design changes what they say. The FLOW rules the change proposal lists for it are FLOW-02, -03, -05, -10, -15, -19 and -20; its design settles its own list. Jeff's decision that a reconciliation item ranks with the fixes, for its FLOW-05 change to state (point 7; a23). Point 5 below. |
| `loop-native-lead` | Not yet filed; after `main` is protected | LOOP-05 as the frame for its changes: a worker is scheduled and bounded, routed by the one host table, identified on every claim, and set up by the owner. The FLOW rules the change proposal lists for it are FLOW-04, -09, -13, -16 and -18; its design settles its own list. Jeff's decision on `<capability>-any` work, for its FLOW-04 change to state: whichever worker has the capability and next takes steps, under a refreshable 20-hour lease (point 7). The two open points of point 5 that concern an unattended worker's start. |
| `loop-repository` | Queued; retitled and its `holds` removed on 2026-11-14 | The profile's term list as the specification of what `wf` stops hard-coding (point 4). LOOP-08's six steps as what the relay does to adopt, and the relay's first item through `wf`'s gates as the test that the queue works across repositories: the wave-1 gate under a22's recommended option (U12), and what P1's done-when asks for again after the move (U9). A branch and a pull request for an item in each repository it works in, and LOOP-06's branch head per repository. The native side: a second checkout and its toolchain on the native machines. What of a private repository's work is written to the public queue, digest and reports. Point 5 below. |
| `loop-move` (a provisional name) | Not yet filed; P1's last item, after `loop-repository` (a19) | The split (point 2). The table of what moves and what stays (point 3). The links and paths the move changes (**Rule IDs, cross-references, `rules.yaml` and tier 1**). The contract's "Where things are" table, which is the only part of the contract's text the move edits (point 1). What its design settles is under point 5. The repository's own name is confirmed with Jeff right before it is created (the umbrella's a11). P1 is done when this item is (U9); whether wave 2 waits for it is a22. |

**1. The finished contract text**: `docs/development/loop-contract.md` as
merged. It is worded so that a later item can change a FLOW rule it cites
without changing the contract. Where such an item's design finds a clause
that would no longer hold, or wants the contract to say more, that is a
rule change it declares (LOOP-09). The contract's "Where
things are" table has a column for today and one for once the loop
repository exists. Editing that table is the only change to the contract's
text the move needs.

**2. The split.** Three files that are wholly loop-side
(`loop-contract.md`, `process.md`, `behavioral-linter.md`) and the README's
loop-side sections, listed under **Rule changes**. Moving them is a
move in FLOW-20's sense.

**3. The list of what moves and what stays**, for `loop-move`. It is the
table read at `2889919`.

| Moves to the loop repository | Stays in `buildwarden/buildwarden` |
|---|---|
| `docs/development/loop-contract.md`, `process.md`, `behavioral-linter.md`; the README's loop-side sections, as the Rule changes section lists them | The other eight rule files; `loop-profile.md`; the README's project-side sections |
| `tools/workflow/` whole: `cmd/wf`, `queue/`, `check/`, `native/`, `flow18-check.sh`, `enforcement-check.sh` | `tools/tier1/` (it imports `tools/workflow/check` through a `replace`, which the move has to resolve) |
| `.claude/skills/workflow-{tick,human,native,intake}`, `.claude/agents/wf-{design,plan,implement,review,guide,publish}.md` | `.claude/settings.json`, `.entire/settings.json`, `AGENTS.md`, `CLAUDE.md` |
| The `workflow` branch's content: `queue/`, `config.yaml`, `hosts.yaml`, `backlog/`, `digest/`, `reports/`, `native/`, `QUEUE.md`, `README.md` | `docs/design/` and its `TEMPLATE.md`; `.github/workflows/` |

**4. The profile's term list as the specification of what `wf` must stop
hard-coding**, for `loop-repository`. Where each term is fixed in the tools
today:

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

**5. What the contract requires that the tools do not yet do**, each with
the item whose design builds it:

- the repositories an item names, recorded in the queue and so on every
  claim (LOOP-04, LOOP-05), and shown in `QUEUE.md` and on the item's
  digest lines: `loop-parallel`. Today an item's design names them;
- the branch head a step left in each repository it changed (LOOP-06,
  point 1), for an item with a branch in a second repository:
  `loop-repository`, which gives an item its branches and pull requests
  there. Until then every branch head the loop leaves is in this
  repository, and the digest is already one;
- `wf` reading a component's profile in place of the paths and commands it
  hard-codes (LOOP-01, point 4): `loop-repository`;
- `wf`'s gates, branches and pull requests in a second repository, and how
  a session working in that repository's checkout runs `wf`, the skills
  and the agents, which are in this one until the move (LOOP-08):
  `loop-repository`;
- where tier 1's generic checks (`rule-index`, `design-front-matter`,
  `suppression`) live, since the relay needs them and cannot use the Go
  analyzers: `loop-repository`;
- how a checkout of this repository gets `wf`, the skills and the agents
  once they are in the loop repository, and how CI here then runs
  `flow18-check.sh`, `enforcement-check.sh` and tier 1's use of
  `tools/workflow/check` (the `replace` in point 3): `loop-move`;
- the links and paths listed under **Rule IDs, cross-references,
  `rules.yaml` and tier 1**, and the contract's "Where things are" table:
  `loop-move`.

The contract leaves these open, and the item named decides each in its own
design:

- what FLOW-05's limits count once items that name different repositories
  are in one queue: `loop-parallel`. The contract says only that FLOW-05
  says it (LOOP-04);
- whether a hold may name repositories as well as workers: `loop-parallel`,
  with its FLOW-05 change. The contract says that a process item declares
  `holds` (LOOP-04, LOOP-09) and leaves what a hold names to FLOW-05. Today
  a hold names workers and covers work in every repository;
- what an item that works in a private repository may put in the queue,
  the digest and the tier-2 reports, which are public (LOOP-06, LOOP-08):
  `loop-repository`, with the relay design's a17. The relay is private
  until its M1's scan;
- how FLOW-13's budgets apply to a worker other than the daily cloud run:
  `loop-native-lead`;
- how a run is made to start from trunk. `native-tick.sh` resets its
  checkout to `origin/main`; the cloud tick builds `wf` from whatever its
  checkout holds and does not check (LOOP-09). The item that next changes
  the tick or the wrapper; `loop-native-lead` changes the wrapper;
- the priority-confirmation gate, which only the cloud tick reads: make it
  a rule every unattended worker honours, or leave it the cloud's.
  `loop-native-lead`, since it is the item that lets a worker other than
  the cloud take an item's first step;
- which workers the move holds and for how long, since the queue moves
  under running work (the umbrella's risk "P1 pauses the loop while the
  queue moves"), and what becomes of the `workflow` branch and of the open
  items' queue files and history: `loop-move`;
- whether the loop repository is itself a component, with a profile and
  invariants for `wf`'s code, and whether GLOBAL-09 to GLOBAL-11 bind it
  by citation (LOOP-08; the split table's row for GLOBAL-09): `loop-move`;
- the repository's name, which the umbrella gives as
  `buildwarden-ai-workflow` "or similar" and "name provisional":
  `loop-move`, with Jeff, right before the repository is created (the
  umbrella's a11).

Three points of the hand-off at `2889919` have fallen away with the queue
per component: `holds` scoped per component, a digest per component, and
how a worker divides a run between queues. A fourth, the loop repository
as a component with its own queue, is `loop-move`'s again, without the
queue.

**6. Follow-ups this item names and does not do:** the neutral rewording of
FLOW and LINT (a9), a process item filed after P1; and a11 or a12 if the
review moves either out.

**7. Decided at this item's review, and made true by a later item.** Jeff
decided what follows at the second sitting, on 2026-11-14. The rule that
states it changes in the item named, because a rule changes in the item
that makes it true (the umbrella's Decision 18). That item's design takes
the decision as given. It settles the open points listed with it, as its
own questions, and does not ask the decision again.

- **A reconciliation item ranks with the fixes** (a5). As put to Jeff and
  chosen: "A feature that FLOW-05 orders ahead of new work, alongside
  regression fixes. Needs a FLOW-05 amendment." `loop-parallel` amends
  FLOW-05's class 3 to take in a reconciliation item (LOOP-07) and gives
  `wf` the means to compute the class (a23). What is true today: class 3
  is a feature that `follows` an item; `Class` reads that field alone
  (`tools/workflow/queue/queue.go`); `wf validate` requires `follows` to
  name an item in the queue; and a queued fix is not started until the
  item it follows is done (`waitsForParent`, `queue/next.go`). Its design
  settles:
  - how the queue marks a reconciliation item: a field of its own, or
    `follows` naming the item that produced the release, which has
    nothing to name when a release was cut outside the queue;
  - what such an item waits for. LOOP-04 has it wait for a release, held
    by a blocking question, and class 3's "starts only once that item is
    done" is written for a fix;
  - its place among the fixes, where FLOW-09 files a regression fix "at
    the top of the fixes".

  Until that item lands, FLOW-05 ranks a reconciliation item as it ranks
  any feature. None can exist before a second component releases
  something that another adopts.

- **Work that any machine with a capability can do is not one machine's**
  (a15, part b). Jeff's words: "B should be changed to whichever loop
  supports it and next can pick it up, placing a refreshable 20-hour lease
  while actively working on it". A step with a `<capability>-any` host
  name goes to whichever worker has the capability and next takes steps,
  and that worker holds it under a 20-hour lease that it can refresh
  while it is actively working. `loop-native-lead` amends FLOW-04 to say
  so and changes the host table and the lease to match. What is true
  today: FLOW-04 says `hosts.yaml` "gives each host name to exactly one
  machine"; `wf validate` rejects a name that two machines serve
  (`validateNative`, `tools/workflow/queue/hosts.go`); `hosts.yaml` has
  `qemu-any` on `macbook` and `container-any` on `windows-desktop`; a
  lease is `lease_minutes`, 120, for every claim (`config.yaml`; `Claim`,
  `queue/next.go`); an expired lease may be reclaimed (FLOW-04); and
  there is no refresh. The holder of a live lease may claim the step
  again, which sets a new expiry and counts one more step against the
  item's budget. Its design settles:
  - whether the 20-hour refreshable lease is for `<capability>-any` work
    only, or for every claim;
  - what "actively working" means for a refresh: what a worker does to
    refresh, how often, and whether a refresh counts against the item's
    budget;
  - what happens to a lease that is not refreshed. Today a lease is
    reclaimed only at its expiry, and at 20 hours a session that died
    would hold its step for most of a day unless a missed refresh frees
    it sooner;
  - how the host table lets several machines serve one
    `<capability>-any` name, and what `wf validate` and `wf gate` then
    check;
  - which workers have a capability: the machines the host table lists,
    or the cloud worker too where it can do the work. His word is
    "whichever loop";
  - whether a claim then names the machine. A worker claims as
    `host:<name>/<session>`, so two machines serving one name are told
    apart only by the session (LOOP-05, "Identified").

  LOOP-05 cites FLOW-04 for who takes a step and states no one-machine
  rule, so it holds before that item and after it.

### Amendments to the umbrella design

One queue changes sentences of
[`componentization`](componentization.md), which Jeff approved on
2026-11-12 and whose Decisions 1 to 17 are his. Each is listed here with its
line on `main` at `b903f65`, its current wording and its proposed wording.
They are made in this item's PR once he approves them at the design review
(a20).

**Re-derived on 2026-11-14 for a19's answer.** The list Jeff read at
`04cc9bb` had twenty-two amendments, for one queue and for a deferred loop
repository. He confirmed a20's first option and, by a19, kept the move to
the loop repository in P1. Each amendment was read again against the
umbrella's lines. The numbers are kept, so that this list can be compared
with the one he read:

- **Nine stand as read:** U1, U3, U5, U7, U10, U11, U12, U17 and U18. They
  follow from one queue. U12 is worded on a22's recommended option, and is
  withdrawn under its second.
- **Six are reworded:** U8, U9, U13, U15, U20 and U21. Each spoke of one
  queue and of the deferral. The last column says what changed.
- **Seven are withdrawn:** U2, U4, U6, U14, U16, U19 and U22. Each said
  only that the loop repository is deferred, and the umbrella's sentence is
  right again.

The six reworded ones are put to him again, by number, before the
amendments are approved.

The umbrella was read in full for this list. The amendments change one
decision, Decision 2, and add none; Decision 13 is not amended. Decision 2
and a5 keep the sentence that was approved and gain the amendment after it;
the other sentences are replaced, and U21 records that they were. U22 is
out of line order because it was found after the others were numbered.

**FLOW-20's own text is not amended here.** The umbrella quotes it in full
under its **Rule changes** (lines 535 to 569), as the record of what was
approved on 2026-11-12; that record stands. The rule in `process.md` says
the program "has the queue to itself", has a Limit bullet, and says "the
loop" becomes a separate component. What of that changes with one queue is
a change to how FLOW-05 counts, and `loop-parallel` declares it with its
FLOW-05 change. This item's `rule_changes` do not name FLOW-20.

| # | Line | Where | Current wording | Proposed wording | Against the list read at `04cc9bb` |
|---|---|---|---|---|---|
| U1 | 56-58 | Problem, point 3 | "A second repository with its own queue needs a shared contract, and the relay repository already needs one." | "A second repository worked by the same loop needs a shared contract, and the relay repository already needs one." | As read. |
| U2 | 88-89 | Prior art | "[`workflow-streamline`] and [`native-host-loop`]: the loop that phase P1 moves." | Withdrawn: the umbrella's sentence stands. | Read: "the loop that phase P1 extends to more than one repository." P1 moves the loop again, in its last item. |
| U3 | 109-111 | Decision 2 | "**Each component gets its own repository and its own queue**, coupled only through written seams and tagged releases. The end state is the component table under **Design**." | The same two sentences, followed by: "*Amended 2026-11-14 (`loop-contract`):* a repository each, and one queue for all of them. There is one queue, one order and one place of reporting for work in every repository, and an item names the repositories it works in. Jeff's reason: separate queues would be harder to track." | As read. |
| U4 | 165-168 | Decision 13 | "**The loop gets its own public repository**, `buildwarden-ai-workflow` or similar. Alternative: the sessions repository; it is private, so the queue and reports would stop being visible beside a public project. Work is both spec-first and design-first, depending on what is being implemented." | Withdrawn: the umbrella's sentence stands. | Read: the same three sentences followed by an amendment that deferred the repository. Decision 13 stands as approved (a19). |
| U5 | 221-223 | Questions, a5 | "**a5.** Confirmed. FLOW-05's `alters_relationships` limit is raised from 1 to 3 in `config.yaml` while the program runs; the limit of 5 features in progress is unchanged." | The same sentence, followed by: "*Amended 2026-11-14 (`loop-contract`):* with one queue for work in every repository (Decision 2), both limits are that queue's. FLOW-05 says which items count against each, and `loop-parallel` is the item that may change that." | As read. |
| U6 | 249 | Where the program ends, the Loop row | "\| Loop \| `buildwarden/buildwarden-ai-workflow` (name provisional) \| n/a \| The Loop Contract, the queue, run reports, `wf`, skills and agents \|" | Withdrawn: the umbrella's row stands. | Read: "With the kit for now (`buildwarden/buildwarden`); a repository of its own is deferred (Decision 13)", holding "the one queue". The table is where the program ends, and the loop ends in its own repository. "The queue" in the row is already singular. |
| U7 | 277 | The seams, seam 10, "Between" | "Every component's loop" | "The loop and every repository it works in" | As read. |
| U8 | 289 | Track P, the P1 row, "Work" | "Finish the Loop Contract against the running loop, covering spec-first and design-first work. Split `docs/development/` into project invariants and loop rules. Move the queue, `wf`, the skills and the agents to the loop repository. Make `wf` work across repositories, with holds per component." | "Finish the Loop Contract against the running loop, covering spec-first and design-first work. Split `docs/development/` into project invariants and loop rules. Make the one queue work across repositories: items that run side by side where FLOW-05 lets them, items a native machine leads from their first step, and `wf`'s gates and checks in a second repository. Then move the queue, `wf`, the skills and the agents to the loop repository. The items, in order, are `loop-contract`, `alters-slot`, `loop-parallel`, `loop-native-lead`, `loop-repository` and, last, the move." | Reworded. The list of items read "... `loop-native-lead` and `loop-repository`", and the last sentence read "The move to a loop repository is deferred (Decision 13)." The approved row's sentence on the move is back, placed last, and the list of items gains the move. The rest is as read. |
| U9 | 289 | The P1 row, "Done when" | "This repository's loop and the relay's loop each complete one item from the loop repository." | "One item in this repository and one item in the relay repository each complete from the one queue in the loop repository." | Reworded. Read: "... each complete from the one queue." The approved row's "from the loop repository" is back: P1 is not done until the loop has moved. |
| U10 | 310 | Track R, heading | "**Track R, relay** (run from the relay repository's own queue)" | "**Track R, relay** (items in the one queue that name the relay repository, once P1 lets the queue hold them)" | As read. |
| U11 | 319 | Track R, the M6 row | "Reconciliation: one "adopt relay version X" item per driver in that driver's queue. Then the Go relay is retired." | "Reconciliation: one "adopt relay version X" item per driver, each an item in the one queue that names that driver's repository. Then the Go relay is retired." | As read. |
| U12 | 326 | Order, wave 1, "Gate" | "Loop repository live; suites pass on the Go relay" | "The one queue works across repositories; suites pass on the Go relay" | As read, and now a question (a22). The wording is its recommended option: wave 2 does not wait for the move. Under its second option U12 is withdrawn and the umbrella's gate stands. |
| U13 | 354-356 | How program items run | "Until P1 lands, program items run in this repository's queue under the rules as they are, plus FLOW-20. The relay's milestones run in the relay repository's queue." | "Program items run in the one queue, which is on this repository's `workflow` branch until P1's last item moves it to the loop repository. Until P1 lands they run under the rules as they are, plus FLOW-20, and the queue's tools work in this repository only: an item that works in another repository says so in its design, and the relay's milestones are worked outside the queue until P1 lets it hold them." | Reworded. The first sentence read "Program items run in the one queue, on this repository's `workflow` branch." It now says until when. The second sentence is as read. |
| U14 | 358-360 | How program items run | "New repositories (the loop, the specs, one per driver) are created, and `buildwarden-relay` is made public, by Jeff, or by an attended session on his word in that session (a11)." | Withdrawn: the umbrella's sentence stands. | Read: "(the specs, one per driver, and the loop's if Decision 13 is taken up)". The loop's repository is created in P1. |
| U15 | 397 | Rule changes the program will make, the FLOW and LINT row | "\| FLOW-\*, LINT-\* \| Move to the loop repository as the Loop Contract's detail. \| P1 \|" | "\| FLOW-\*, LINT-\* \| Become the Loop Contract's detail, in place, and move with it to the loop repository in P1's last item. The FLOW rules that one queue changes are amended by P1's later items, each declaring its own. \| P1 \|" | Reworded. The first two sentences read "Become the Loop Contract's detail, in place. They would move with it if the loop repository is created (Decision 13)." The last sentence is as read. |
| U16 | 413 | Risks | "**P1 pauses the loop** while the queue moves." | Withdrawn: the umbrella's sentence stands. | Read: "**P1 changes the loop's rules and tools while the loop runs.** The queue does not move, so nothing pauses for a move. ..." The queue moves in P1's last item, so the risk is as approved. |
| U17 | 743-745 | Archive summary, "Built by this PR" | "each phase (P1, S1 to S4, K1 to K5, and M1 to M6 in the relay's queue) is its own queue item with its own design." | "each phase (P1, S1 to S4, K1 to K5, and M1 to M6) is its own queue item with its own design." | As read. |
| U18 | 751-753 | Archive summary, "Decisions taken" | "A repository and a queue per component, coupled through written seams and tagged releases (2)." | "A repository per component, coupled through written seams and tagged releases, and one queue for work in all of them (2, as amended on 2026-11-14)." | As read. |
| U19 | 764-765 | Archive summary, "Decisions taken" | "The loop gets its own public repository (13)." | Withdrawn: the umbrella's sentence stands. | Read: "(13; deferred on 2026-11-14, and the loop stays in this repository meanwhile)". |
| U20 | 809-811 | Archive summary, "For future designs" | "Until P1 lands, program items run in this repository's queue under the rules as they are, plus FLOW-20. The relay's milestones run in the relay repository's queue." | "There is one queue for work in every repository, on this repository's `workflow` branch until P1's last item moves it to the loop repository, and an item names the repositories it works in (the Loop Contract, LOOP-04). Until P1 lands, program items run under the rules as they are, plus FLOW-20, and the relay's milestones are worked outside the queue." | Reworded. The first sentence read "There is one queue, on this repository's `workflow` branch, for work in every repository, ...". It now says until when. The second sentence is as read. |
| U21 | after 654 | A new section, **Amendments**, between **Design review** and **Consolidation** | None. | "**2026-11-14, by `loop-contract`**, approved in its design review. Jeff chose one queue for work in every repository in place of a queue per component. The loop repository of Decision 13 stands, and the move to it is P1's last item. Amended in place: Problem point 3; Decision 2; a5; seam 10; the P1 row; the heading of Track R; the M6 row; the wave-1 gate; the first paragraph of **How program items run**; the FLOW and LINT row of **Rule changes the program will make**; three sentences of the **Archive summary**. The wording each had is in [`loop-contract`](loop-contract.md), under **Amendments to the umbrella design**. FLOW-20 as quoted under **Rule changes** is the record of what was approved on 2026-11-12 and is not amended." | Reworded. The second sentence read "... and deferred the loop repository." The list of what is amended in place loses the seven withdrawn: the Prior art line on the loop, Decision 13, the Loop row of the component table, the K4 row's done-when, the sentence on new repositories, the risk on P1 and one sentence of the Archive summary. It loses "the wave-1 gate" as well under a22's second option. |
| U22 | 307 | Track K, the K4 row, "Done when" | "This repository holds the kit, the collector, the executor, the dispatcher and the book." | Withdrawn: the umbrella's sentence stands. | Read: "... the book and, while Decision 13 is deferred, the loop." P1 comes before K3 (lines 334-335), so the loop has moved by the end of K4. |

**Read and left as written**, each for the reason given:

- *The title (line 27)*, "the relay, the drivers, the kit, the specs and the
  loop become separate components". FLOW-20's first paragraph quotes the
  phrase. It stands: the loop becomes a separate component when P1's last
  item moves it.
- *Problem, lines 60-62*, "a loop that spans repositories": still what P1
  delivers.
- *Prior art, lines 93-95*, "P1 and M1 give them homes": P1 gives the
  contract its home, in `docs/development/` with this item and in the loop
  repository with its last.
- *The M1 row (line 315)*, "the Loop Contract adopted": stands. Adoption no
  longer includes a queue (LOOP-08).
- *Order, lines 334-335*, "P1 comes before K3, because a second repository
  needs a loop that spans repositories": stands. With the move in P1, it
  is also what puts the move before wave 4 (a22).
- *The risk "One person merges in every repository" (line 414)*: stands.
- *FLOW-20 as quoted (lines 535-569)* and the two sentences on its limit
  (lines 571-574): the record of the approval.
- *The implementation plan's "After the merge" (lines 633-636)* and the
  archive summary's "What the merge does" (lines 746-750): what was done on
  2026-11-12.
- *Rejected, lines 779-780*, "The loop in the private sessions repository":
  still the rejected alternative.
- *The front matter*: no key changes. The amended file still parses as an
  archived design (tier 1's `design-front-matter`).
- *The seven sentences whose amendments are withdrawn* (lines 88-89,
  165-168, 249, 307, 358-360, 413 and 764-765): as approved.

**What the amendments do not settle**, and the item that does: which items
FLOW-05's limits count and what FLOW-20's Limit bullet then says
(`loop-parallel`); how the relay's M1 is worked before the queue can hold
it (the relay design's a17, with Jeff); whether wave 2 waits for the move
(a22); the loop repository's name (`loop-move`, with Jeff, under the
umbrella's a11).

### What this item does not declare

- **`alters_relationships: false`.** FLOW-02 lists what alters
  relationships: a versioned seam, the bootstrap model, which product
  component owns a responsibility, the driver contract. This item changes
  none. It changes how the *loop* works across repositories, and the
  umbrella already declared that for the program. The practical side: `wf` counts every
  in-progress item that declares `true` and is past Design toward FLOW-05's
  slot, process items included, so `true` here would hold the slot against
  `seam-specs` and `driver-contract` while this item is implemented.
- **`seams: []`.** The umbrella numbers the Loop Contract as seam 10 and
  calls it "a document". FLOW-02's `seams` are GLOBAL-08's versioned seams.
- **No second repository.** This item works only in
  `buildwarden/buildwarden`: its `main`, and its `workflow` branch for the
  one README line after the merge. It names no other (LOOP-04).
- **No `holds`.** The PR amends three rule paragraphs, adds documents,
  amends an archived design's wording, and makes one small machinery
  change, or two if a12 stays here. A run that starts after the merge
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
| The repositories an item works in, when its design names none | The repository that holds its design, only (LOOP-04). For every open item today that is `buildwarden/buildwarden`. | Today's behaviour, now stated: the queue file has no field for an item's repositories, and an item that works elsewhere says so in its design, as `seam-specs` does. The override is the design naming them, and what it names is used as given: an item works only in the repository that holds its design and in those the design names. The queue's field and what its absence means are `loop-parallel`'s to propose. | a18 |
| A profile term left unfilled | A finding (LOOP-01). | No fallback to `buildwarden/buildwarden`'s entry. Another component's `make lint` is not a predictable meaning for a Rust repository (GLOBAL-11). | a9 |
| A profile term filled as `none` | The rules read through it do not apply in that component. | Explicit, written by the component, and reviewed with the profile. | a9 |
| The contract a component follows | The contract's trunk. | "A rule changed here changes everywhere at once" (the draft). There is no override: a component cannot pin an older contract (LOOP-08). | a15 (g) |
| The kind of a reconciliation item | Feature. | It changes code. No override. | a5 |
| The class of a reconciliation item | What FLOW-05 gives it (LOOP-07). Decided: with the fixes, class 3. Until `loop-parallel` amends FLOW-05, that is what FLOW-05 gives any feature: new functionality, or the fixes when the item `follows` another. | The contract names no class of its own, so the order is stated once, in FLOW-05 (FLOW-14). Within a class the human's priority orders the items (LOOP-04). There is no implied rank: a reconciliation item is in class 3 only once FLOW-05 says so and `wf` computes it. | a5, a23 |
| The tier of a new check | None. | A check without a tier is a finding (LOOP-03), as a rule without a check type is (LINT-01). | a6 |
| What a failed deterministic check does | It blocks. | A hint or a report is declared by name in the profile, and host-bound validation is blocking or tracked in the design (FLOW-09). Nothing is non-blocking by omission. | a6 |
| The severity of a cited detail | The cited rule's own. | A LOOP rule that cites FLOW-05, FLOW-13 or LINT-06 does not raise them from SHOULD. Raising one is a rule change of its own. | none: it changes nothing |

The row for an unset `limits.alters_relationships`, here at `2889919`,
has left with a13 for `alters-slot`.

The 20-hour lease of Jeff's answer to a15 (b) is not a default of this
item. `lease_minutes` stays 120, for every claim, until
`loop-native-lead` changes it (hand-off, point 7).

**Fallbacks.** None is added and none is dropped. The reading this design
dropped at `2889919`, `wf validate` treating an unset
`limits.alters_relationships` as "no limit", is `alters-slot`'s to decide.

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

**Why:** the contract the loop runs under in every repository has to be a
rule file that the loop reads, in the format the rest are in (LINT-01),
before a second component can adopt it. The nine rules are new statements; where the
running loop already has the rule, they cite it.

**Current wording:** none; the file is new.

**Proposed wording**, the whole file:

> # The Loop Contract
>
> The rules BuildWarden's development loop runs under, in every
> repository it works in. There is one loop: one queue of work, one order,
> one set of workers and one place of reporting (LOOP-04 to LOOP-06). A
> *component* is a repository that has adopted this contract (LOOP-08).
> Its profile, its invariants, its designs and its checks are kept in it.
> It has no queue of its own: an item in the queue names the repositories
> it works in. `buildwarden/buildwarden` is the first component, and
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
> | The queue, `config.yaml`, the host table, digests and reports | The `workflow` branch of `buildwarden/buildwarden` | The loop repository |
> | `wf`, the skills and the agent definitions | `tools/workflow/` and `.claude/` in `buildwarden/buildwarden` | The loop repository |
> | A component's profile, invariants, designs and checks | Its own repository | Its own repository |
> | Session recordings | `buildwarden/buildwarden-sessions` (FLOW-18) | The same |
>
> The loop repository is the one the componentization design's
> Decision 13 gives the loop. The second column says what moves to it.
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
> | The queue | The kinds of item, the phases, the order and what blocks what (LOOP-04) | Nothing: there is one queue, and an item names the repositories it works in |
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
> | Component | Its name and repository | LOOP-04, LOOP-05, LOOP-06 |
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
> ### LOOP-04 — One queue, one order
>
> **Severity:** MUST
> **Check:** agent-judgment
>
> There is one queue, for work in every repository. An item names the
> repositories it works in. Until the queue records them, it names them in
> its design; an item that names none works only in the repository that
> holds its design. An item is one of two kinds (FLOW-03): a *feature*,
> which changes a component, or a *process item*, which changes the rules
> the loop runs under: the loop rules, the loop's machinery, or a
> component's own invariants when no feature needs the change. Every item
> has the phases of its kind (FLOW-03), and every step says who may take it
> (FLOW-04).
>
> - There is one order, and FLOW-05 gives it. FLOW-05 also says which
>   items may be in progress side by side and which wait for another. This
>   rule adds no order and no limit of its own.
> - A process item ranks ahead of every item not already in implementation
>   (FLOW-05). It stops other work only through the `holds` it declares
>   (FLOW-05, LOOP-09).
> - Priority is the human's. An agent places an item only where a rule
>   says so: the session that finds a regression trunk already has files
>   its fix at the top of the fixes (FLOW-09), and the human may move it.
>   No agent otherwise reorders items.
> - An item that cannot proceed says what it waits for, and its worker
>   moves on (FLOW-05, FLOW-07).
> - An item that waits on another component waits for a release (LOOP-07):
>   it is held with a blocking question that names the release (FLOW-07),
>   and resumes when the release exists.
>
> The agent part: intake and the design review judge an item's kind and
> the repositories it names.
>
> ---
>
> ### LOOP-05 — Workers are scheduled, routed and identified
>
> **Severity:** MUST
> **Check:** agent-judgment
>
> Work is done by workers that take steps from the queue: `cloud`, `human`
> or `host:<name>` (FLOW-04).
>
> - **Scheduled and bounded.** An unattended worker runs on a schedule and
>   within a budget (FLOW-13). A native machine's scheduled session starts
>   an agent only when the machine has work (FLOW-09).
> - **Routed by affinity.** Work that needs native hardware carries a
>   host name. One table, kept beside the queue, says which host names
>   each machine serves, for work in every repository. Work that any
>   machine with a capability can do carries `<capability>-any`. FLOW-04
>   says which worker may take a step.
> - **Identified.** Every claim records the worker (its kind, its host
>   name and its session) and the item, and through the item the
>   repositories the work is in (LOOP-04).
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
> ### LOOP-06 — Every run reports in one place
>
> **Severity:** MUST
> **Check:** agent-judgment
>
> Each unattended run appends one section to the day's digest (FLOW-13).
> There is one digest, for work in every repository. The section records:
>
> 1. each step taken: the item, its kind and FLOW-05 class, the step, its
>    outcome and the branch head it left in each repository it changed;
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
> 2. A consumer adopts a release through a **reconciliation item**: an
>    item in the queue that names the consumer's repository,
>    `adopt <consumable> at <version>`. It is a feature item (LOOP-04),
>    filed through intake, and FLOW-05 gives its place in the order.
> 3. The item moves the consumer's pin and, where a seam changes, its
>    specification first (LOOP-02), then its code, and then retires any
>    in-tree equivalent.
> 4. Both components' checks pass after adoption. A release is never
>    changed to make that so: a change the consumer needs is a new item
>    that names the producer's repository, and a new release.
> 5. A component's trunk names only released versions of what it
>    consumes. A branch may name an unreleased commit while its work is in
>    progress.
> 6. A specification kept in a repository of its own is consumed the same
>    way. A seam change is released by the repository that holds the
>    specification and adopted by each implementation through its own
>    reconciliation item, at its own time.
>
> The agent part: Review of a reconciliation item judges points 3 to 5;
> intake judges that a change one component needs from another was filed
> as an item that names the producer's repository.
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
> 6. seeing that only a human merges to its trunk (FLOW-08), and
>    protecting the trunk where its host offers that; deleting head
>    branches on merge (FLOW-16); and recording sessions only to the
>    private sessions repository (FLOW-18).
>
> It gets no queue of its own: its work is items in the one queue that
> name it (LOOP-04).
>
> A rule prefix belongs to one owner, so a rule ID names one rule wherever
> it is cited. `LOOP-`, `FLOW-` and `LINT-` are the loop's. A component
> names its prefixes in its profile and takes none that another profile
> names. A prefix changes owner only when its rules move, in the items
> that move them. A component may cite another component's rule by ID as
> binding in its own repository.
>
> An item may work in a repository that has not adopted the contract,
> such as one that holds specifications or a fork. There the worker is
> bound by FLOW-08, FLOW-16, FLOW-17 and FLOW-18, and the item's design
> says which invariants apply there, by rule ID. Once that repository
> adopts, it and the other components couple only through releases
> (LOOP-07).
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
> judges the six steps, and that its prefixes are no other component's;
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
> would disrupt running work, declares the workers it holds (FLOW-05).
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
The later items can give LOOP-01, LOOP-04 and LOOP-05 an automated half
once `wf` reads profiles and records an item's repositories.

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
and every run leaves that on `main`. Making it a check is listed for the
later items (hand-off, point 5).

LOOP-04 holds on merge with FLOW-05 as it stands. There is one queue
today, on the `workflow` branch. "FLOW-05 says which items may be in
progress side by side and which wait for another" is true of today's
FLOW-05, whose answer is its limits on features in progress, on features
that alter relationships and on the PR slot; it stays true if
`loop-parallel` changes that answer, because LOOP-04 does not repeat it.
A hold is the same: LOOP-04 and LOOP-09 say a process item declares
`holds`, and FLOW-05 says they are affinity patterns, which is what
`wf validate` accepts. The sentence on an item that waits for a release
asks for what the loop has: a blocking question and its answer step
(FLOW-07).

The clauses on an item's repositories (LOOP-04, LOOP-05, LOOP-06) hold in
the form the loop has today, and the PR adds what carries them. The queue
file has no field, so an item names them in its design: the template's
**Design** section asks, `wf-design` writes it, intake asks and passes it
in `source`, and the design review confirms it (tasks 5, 7 and 8). An item
that names none works only in the repository that holds its design. Of the
open items, `seam-specs` names a second repository in its design and the
others name none. A claim records the item, and the item's design gives
the repositories; every branch head a step leaves is in this repository
until `loop-repository` gives an item a branch in another.

LOOP-05's bullet on routing holds with FLOW-04 as it stands. `hosts.yaml`
lists the host names each machine serves, and FLOW-04 says who may take a
step: today it gives every host name, `<capability>-any` included, to
exactly one machine. The bullet stays true when `loop-native-lead` changes
FLOW-04 for Jeff's answer to a15 (b), because LOOP-05 does not repeat
FLOW-04's answer.

One piece of work is outside the queue on the day this merges: the relay's
M1a, which its design plans to work by hand, in attended or directly
dispatched sessions (its a17, unreviewed). LOOP-04 binds the work the loop
does, and the loop does not work in the relay repository until
`loop-parallel` can record such an item and `loop-repository` can run its
gates there (U13, U20). Whether M1a is filed before then, as an item with
human steps only, is for that design's review.

LOOP-07 and LOOP-08 bind from the first adoption. LOOP-07's point 2 says
that FLOW-05 gives a reconciliation item its place in the order, and
FLOW-05 does that for every feature today. No reconciliation item exists,
and Jeff's ranking of one with the fixes (a5) becomes FLOW-05's text in
`loop-parallel` (a23). Two clauses of LOOP-08
are not met by this repository today, and each is closed before or by the
merge. Step 1 asks for a link to the contract from the README:
`README.md` links to `docs/development/` and not to the contract, and task
11 adds the link. Step 6 asks for a protected trunk where the host offers
it: `main` has no branch protection, the host offers it, and a21 has Jeff
set it before the merge. The other steps are met: the profile and the
rules exist, the template has the sections, the consumables are declared
in the profile, head branches are deleted on merge, and sessions go only
to the private sessions repository. With a21 settled as recommended,
nothing in the nine contradicts `main` (the umbrella's Decision 18).

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
  > repository's own. The **loop rules** are the
  > [Loop Contract](./loop-contract.md) and its detail, which the loop
  > runs under in every repository it works in; they are kept here until
  > the loop repository exists, and move to it then.
  > [`loop-profile.md`](./loop-profile.md) is this repository's profile
  > (PROFILE-01).

- The **Documents** list becomes two lists. "Project invariants":
  `global.md`, `relay.md`, `orchestrator.md`, `in-vm-executor.md` with
  `bootstrap.md`, `ux-cli-config.md`, `testing.md`, `security-suite.md`,
  and `loop-profile.md`. "Loop rules": `loop-contract.md`,
  `process.md`, `behavioral-linter.md`. Each entry keeps its present
  description.
- The **Rule format** section gains one sentence: "This format is LINT-01's
  and is the same for every component."

Nothing else in the README changes, and no section moves. Each section's
side, and what the move does with it in P1's last item:

| README section | Side | When the loop files move |
|---|---|---|
| The core idea | Loop: it is LINT-01's and LINT-08's premise | Copied to the loop repository's index; a short form stays here with the two-part paragraph |
| Rule format, with Severity and Check-type | Loop (LINT-01, LINT-02) | Moves; this README links to it |
| ID scheme | Both: each row has an owner | The loop's three rows move; the rest stay |
| The three applications | Project | Stays |
| Documents | Both, already in two lists | The "Loop rules" list becomes a link |
| Relationship to the other guidance files | Project, except its last paragraph, which restates FLOW-14 | Stays; the last paragraph moves |
| Open deep-dives | Project: they are this repository's queue items | Stays |

### LOOP-02 and LOOP-04 (add), `docs/design/TEMPLATE.md`

**Why:** LOOP-02 asks a design that changes a seam to list its
specification changes, and LOOP-04 asks an item to name the repositories
it works in, which it does in its design until the queue records them. The
template is where a design learns both.

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

For LOOP-04, the text under **Design** gains a sentence. Current wording:

> The behavior and structure of the change.

Proposed wording:

> The behavior and structure of the change. Name each repository the item
> works in other than this one (LOOP-04); a design that names none works
> only here.

**Enforcement:** none added. `wf`'s done-when asks for neither the
section nor the sentence. The design review does (the agent parts of
LOOP-02 and LOOP-04), and the `workflow-human` skill's walkthrough gains
both (Implementation plan, task 7). A design approved before this merges
is not sent back for the section: `wf-publish` adds it at
`f6-consolidate`, after it has merged `main` (task 7). The one open design
that works in a second repository, `seam-specs`, already names it.

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

> A feature may also change the rules of a component it works in, and
> their enforcement, never a loop rule (LOOP-09), when its design review
> approves each change by name (FLOW-14); `.claude/` and the
> rest of `tools/workflow/` change only through a process item:

At `2889919` the proposed wording began "A feature may also change its own
component's rules and their enforcement". With one queue an item has no
component of its own: it names the repositories it works in (LOOP-04).

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
>   declare a change to the invariants and profile of a component it works
>   in and to their enforcement, and never to a loop rule (LOOP-\*,
>   FLOW-\*, LINT-\*) or its enforcement (LOOP-09). A *rule
>   change* is an edit to
>   this directory (a rule, its severity or check type, the README index), to

The rest of FLOW-14 is unchanged. At `2889919` the new sentence read "to
its own component's invariants and profile"; it is reworded for the same
reason as FLOW-03's.

**Severity and check:** unchanged (MUST; automated + agent-judgment). The
new sentence is in the agent part: the design review refuses a feature's
`rule_changes` entry that names a loop rule, whatever its paths. The rule
ID on the entry decides. `tools/tier1/` and `tools/workflow/check/` stay
rule-change paths a feature may edit, because they hold the checks of
project rules too (`driver-contract` changes `tools/tier1/surface/` under
UX-01), and an edit there is cleared only under the rule it enforces.

The automated clearance (`tools/workflow/check/rulechange.go`) is not
changed: it still clears any approved path, and does not look at the rule
ID's prefix (a10's third option would make it). What the move changes, in
P1's last item: the loop's rule files and
`tools/workflow/`, `check/` included, leave this repository, so a
feature's PR here cannot edit them. Tier 1's checks of FLOW and LINT rules
(`rule-index`, `design-front-matter`, `drivers-claim`) stay in
`tools/tier1/` and remain guarded by the review alone; `loop-repository`
decides where tier 1's generic checks live once a second repository needs
them.

## Validation plan

A rules-and-documents item with one small change to `wf` (a11) and edits
to the skills and agent definitions. There is no native validation and no tier-2
gap (FLOW-09, TEST-03): nothing in the PR is specific to a host's driver or
lifecycle.

- **Tier 0** (TEST-02), on the branch:
  - `make lint`: `golangci-lint` over the three modules, then tier 1 on
    the diff from `origin/main`. The checks that bear on this PR:
    `rule-index` (LINT-01, LINT-02: the nine LOOP headers and PROFILE-01
    each parse with a severity and a check, and no `automated` rule lacks a
    `rules.yaml` entry); `design-front-matter` (FLOW-02) on this design
    and on the amended umbrella, whose front matter does not change; the
    design-in-the-diff check (FLOW-01), which the `wf` change needs and
    this design satisfies; `tests-with-change` (GLOBAL-04) for the `wf`
    change.
  - `make test`: `tools/tier1`'s `TestRealIndex` parses the real rule
    files, the two new ones included; `tools/workflow`'s tests, in the
    `queue` package, cover `wf new -hold` (the item it writes validates;
    `Next` gives the cloud no step of it and, while it is queued, no
    `blocked` line; answering `q1` completes the step and the first step is
    offered).
  - `make enforcement`: a no-op, since the branch changes neither
    `tools/tier1/` nor `tools/workflow/check/` (FLOW-14).
- **CI on the PR:** the `ci` workflow, in which `docs/development/*` turns
  on the rule tests and the `tools/` changes turn on lint, test and build,
  and the `flow18` workflow, which reads
  `.entire/settings.json`, refreshed if a12 stays here (FLOW-18).
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
- **The amendments to the umbrella** have no automated check beyond its
  front matter. The implement step applies each amendment that stands or
  is reworded, as approved (fifteen of U1 to U22; the seven withdrawn ones
  change nothing), and the PR body shows the diff of
  `docs/design/componentization.md` beside this design's table.
- **Tiers 1 and 2:** not reached. The integration workflow's paths
  (`driver/`, the shared packages, `cmd/`) are not touched.

A process item has no Review phase (FLOW-03), so the `wf` change gets
no tier-2 report. It is kept small and tested for that reason, and the
human's merge is its review (FLOW-08).

Read for this revision, on 2026-11-14, with `gh api` (read-only):
`buildwarden/buildwarden` is public, deletes head branches on merge, and
has no branch protection and no ruleset on `main`. At `2889919` this was
listed as not verified. LOOP-08's step 6 asks a component to protect its
trunk where the host offers it, so the first component does not meet it
today (a21). FLOW-08 holds
here by practice and by the agents' deny rules, not by a repository
setting.

Not verified for this design: how the scheduled cloud task obtains its
checkout, which LOOP-09's "from trunk" rests on for the cloud run; it is
outside the repository. Nor was it established whether a phase agent's
definition is read when the session starts or from the tree after the tick
switches to the feature branch.

The relay repository's settings were read the same way on 2026-11-14:
`buildwarden/buildwarden-relay` is private, does not delete head branches
on merge, and its branch-protection endpoint answers 403 ("Upgrade to
GitHub Pro or make this repository public to enable this feature"). At
`cf44244` they were listed as not read (a21).

## Implementation plan

For this item's PR (process phase 2). Task 9 is dropped if the review
moves a11 elsewhere, and task 8's `-hold` wording goes with it. Task 10 is
done only if `alters-slot` has not carried the two files (a12).
Task 12 follows a20. At `2889919` there was a task for the
`alters_relationships` limit in `wf next` (its task 10); it has left with
a13, and the tasks after it are renumbered.

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
   to `docs/design/TEMPLATE.md`, and the sentence on an item's
   repositories under **Design** (LOOP-02, LOOP-04, FLOW-14).
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
   in the overview, confirm with Jeff the repositories the design names,
   or that it names none (LOOP-04); go over **Specification changes**
   where the design has it (LOOP-02); and refuse a feature's
   `rule_changes` entry that names a LOOP, FLOW or LINT rule (LOOP-09).
   `.claude/agents/wf-design.md`: step 2 names, under **Design**, each
   repository the item works in other than this one (LOOP-04) and fills
   **Specification changes** when `seams` is not empty (LOOP-02), each
   even when the branch's template predates it, and step 6 says a
   feature's design declares no change to a loop rule (LOOP-09, FLOW-14).
   `.claude/agents/wf-publish.md`: the PR body gains a **Specification
   changes** section when the design has one, and `f6-consolidate` adds the
   section to a design that declares `seams` and has none (LOOP-02,
   LOOP-06).
8. `.claude/skills/workflow-intake/SKILL.md`, step 3: agree with Jeff
   which repositories the item works in, and put any other than this one
   in `-source`, for Design to name (LOOP-04); a change to a loop
   rule is a process item and never a feature's rule change (LOOP-09,
   FLOW-14). With a11: step 3 says an item that is not a program item and
   not exempt is filed with `-hold`, and step 4 shows the flag (FLOW-20).
9. `wf new -hold "<why>"`: the flag in `tools/workflow/cmd/wf/main.go`,
   what it writes as a function in `tools/workflow/queue/`, tested in
   `queue/queue_test.go` (FLOW-20, FLOW-07, GLOBAL-04, GLOBAL-10).
10. Only if `alters-slot` has not carried them (a12): take
    `.claude/settings.json` and `.entire/settings.json` as commit
    `ac8b41a` has them, and run `tools/workflow/flow18-check.sh` (FLOW-18,
    FLOW-14).
11. `AGENTS.md`: two rows under "Where things are", for the Loop Contract
    and for this repository's profile (FLOW-14). `README.md`: one sentence
    in **Development** that links to `docs/development/loop-contract.md`
    (LOOP-08, step 1).
12. Amend `docs/design/componentization.md`: the amendments that stand or
    are reworded (U1, U3, U5, U7 to U13, U15, U17, U18, U20), as approved,
    each in place, and its new **Amendments** section (U21). U12 follows
    a22. The seven withdrawn ones (U2, U4, U6, U14, U16, U19, U22) change
    nothing. Its front matter and FLOW-20's text in `process.md` are not
    edited (a20, FLOW-12, FLOW-20).
13. `make lint` and `make test` green, the branch pushed (FLOW-17,
    TEST-01).
14. Merge current `main`, redo task 2's check of the profile against it,
    archive this design on the branch with its summary, and open the PR.
    The PR body carries a "Before the merge" line, if a21 is confirmed as
    recommended, and an "After the merge" list (FLOW-12, FLOW-08).

Before the merge, if a21 is confirmed as recommended: Jeff protects `main`.

After the merge, outside the PR, in the attended session in which Jeff
merges: document `wf new -hold` in the `workflow` branch's `README.md`
(FLOW-03); tell the relay's M1 that the contract's text is final and where
it is, that the link changes once when P1's last item moves the contract
to the loop repository, and that the relay has no queue of its own
(LOOP-08); and, if this
item merged before `seam-specs`, see that its design carries the
PROFILE-01 entry.

## Design review

### 2026-11-14, first sitting: changes requested before the walkthrough

Read at `2889919`. Outcome: **changes**. Jeff stopped the review at its first
question (the order of the walkthrough). No rule change was approved and no
assumed question was answered; he gave no preference on a11, a12 or a13.

His note, in full:

> So I think I want to potentially redo part of the loop contract design.
> Overall since I only have a limited amount of AI capacity under specific
> areas, instead of trying to split the queues apart which will make it harder
> to track, instead I think I want a single queue but which allows for more
> parallelization for any features that are nearly or completely non-colliding,
> i.e. Hyper-V driver feature versus adding a new supported release target to
> the relay. I'd like that to potentially allow for native loops to then be
> able to take features that will be reliant on local research/testing to be
> able to do so all the way from the initial phases. That may require a
> redesign of the process, so I'd definitely want to see how this changes
> before going further along these lines.

What it asks of this design, as read by the session that held the sitting:

1. **One queue, not one per component.** This departs from the umbrella's
   Decision 2 ("a repository and a queue per component") and from P1's "holds
   per component", and reaches LOOP-04 to LOOP-09 here, a1, a4, a5, a15, a17,
   and the hand-off to `loop-repository`.
2. **More items in parallel where they do not collide**, in place of
   FLOW-05's fixed limits (five in progress, one item that alters
   relationships past Design, one open feature PR). His example: a Hyper-V
   driver feature beside a new release target for the relay.
3. **A native loop may take an item from its first phase** when the work
   rests on local research or testing. Today a host worker takes only
   `f6-validate-<host>` steps and native gaps (FLOW-04, FLOW-09).
4. **He sees how the process changes before the design goes further.** The
   next revision is put to him as a change proposal first, and the design
   text follows what he settles.

The design below is still the text as read at `2889919`.

### 2026-11-14, same day: the direction settled

A change proposal was put to Jeff: one queue (the existing one) in which an
item names the repositories it works in, the areas it changes and who leads
it; three relations between two items (disjoint, near, colliding) that decide
which of FLOW-05's limits count; a machine able to lead an item from its
first step through consolidation; and a migration in steps (the
`alters_relationships` limit fixed first as its own process item,
`alters-slot`; then the parallel rules; then native-led items; then
`loop-repository`, retitled, for work in a second repository). It named what
needs his word in the umbrella (Decision 2 loses the queue per component;
the loop repository of Decision 13 is deferred; P1 and `loop-repository` are
reworded), and recommended: read "limited AI capacity" as what the cloud
cannot do; keep the freeze until the parallel rules have run on program
items; the order parallel rules, native leads, second repository; defer the
loop repository.

Jeff, on that summary: "Yes, these all read as the direction I'm looking
for".

What follows for this design: LOOP-04 is rewritten for one queue; the
preamble, LOOP-01 and LOOP-05 to LOOP-09 are reworded from a component's
queue to an item; a13 leaves this item for `alters-slot`; the amendments to
the umbrella's wording are listed for approval at the next sitting. The
collision rule itself (FLOW-05) is the later item's, not this one's. The
revision is put to him as a delta against `2889919`.

### 2026-11-14, the revision for one queue: the delta against `2889919`

Not yet reviewed, and nothing in it is approved. This entry records what
changed between the text Jeff read at `2889919` and this revision, so the
next sitting can go over the delta (FLOW-06). The two entries above are the
finding and are kept as written. Each change below is confirmed under the
question named, or approved by number under **Amendments to the umbrella
design**.

**Two notes on the first entry.** Its point 3 says a host worker "takes
only `f6-validate-<host>` steps and native gaps (FLOW-04, FLOW-09)". That
is what happens, and not what the rules say: FLOW-09 already allows
`host:<name>` affinity on Design and Implement, and intake sets it by hand
(`workflow-intake`, step 4). What stops a native-led item is the tooling:
`wf` seeds every template step as `cloud`, and a host's claim starts no
item. `loop-native-lead`'s scope is that difference. Its last sentence,
"The design below is still the text as read at `2889919`", was true when
the entry was written (`129c6bd`) and stopped being true with this
revision.

**What did not change.** The nine-rule structure and every rule's severity
and check. LOOP-02 and LOOP-03, word for word. PROFILE-01 and the profile's
tables. The split by file and the side of every rule. Rule IDs and prefixes
(a16). Who may change a loop rule (a10; LOOP-09's first and third
paragraphs). The FLOW-01 amendment, and the FLOW-03 and FLOW-14 amendments
but for one phrase (the table below). The template's **Specification
changes** section. The skill and agent edits, which gain the lines on an
item's repositories. a11. The thirteen `rule_changes` entries, of which
LOOP-04's gains a path (the template). `design_review: required`,
`alters_relationships: false`, no `holds`.

**The contract's proposed text and the other rule wordings** (a18; a19 for
the "Where things are" table; a21 for LOOP-08's step 6):

| Where | At `2889919` | Now |
|---|---|---|
| Preamble, first paragraph | "The rules every BuildWarden component's development loop runs under. A *component* is a repository that has adopted this contract (LOOP-08) and has its own queue of work." | One loop: one queue, one order, one set of workers, one place of reporting. A component is a repository that has adopted; its profile, invariants, designs and checks are kept in it; it has no queue of its own; an item names the repositories it works in. |
| "Where things are" | Second column "Once the loop repository exists"; first row label "Queues, ..." | "If a loop repository is created"; "The queue, ..."; and a sentence: "A loop repository is deferred (the componentization design's Decision 13). The second column says what would move to one." |
| LOOP-01, the shared and local table, row "The queue" | Local: "The items" | Local: "Nothing: there is one queue, and an item names the repositories it works in" |
| LOOP-01, the term table, row "Component" | Used by LOOP-05, LOOP-06 | Used by LOOP-04, LOOP-05, LOOP-06 |
| LOOP-04 | "One queue per component, one order". Each component has its own queue. Where a loop process item is queued. `holds` "name the workers it stops and the components it stops them in". "No item needs a step of an item in another component's queue. An item that waits on another component waits for a release (LOOP-07): it is put on hold with a blocking question that names the release (FLOW-07), and resumes when the release exists." How a worker divides a run between queues, and whether limits and budgets count per component, "are not yet rules". The agent part judged an item's kind and queue, "and that it names no step in another queue". | "One queue, one order". One queue for work in every repository. An item names its repositories, in its design until the queue records them, and one that names none works only in the repository that holds its design. The two kinds. FLOW-05 gives the order and says which items may be in progress side by side and which wait; "This rule adds no order and no limit of its own". A process item stops other work "only through the `holds` it declares (FLOW-05, LOOP-09)": the rule no longer says what a hold names, which is FLOW-05's. The bullets on priority and on an item that cannot proceed are unchanged. The sentence on waiting for a release is kept, as "it is held with a blocking question"; the sentence before it, on a step in another queue, is gone. The clauses on where a loop process item is queued and on dividing a run are gone. The agent part judges an item's kind and the repositories it names. |
| LOOP-05 | Steps taken "from a queue". The host table "shared by every component". "A worker works on one component's item at a time. Every claim records the worker ..., the component and the item." | "from the queue". The host table "kept beside the queue ... for work in every repository". "Every claim records the worker ... and the item, and through the item the repositories the work is in." The sentence on one component's item at a time is gone: one item may name two repositories. |
| LOOP-06 | "Every run reports in one place per component"; a section in the digest "of each component it worked in"; point 1 ends "the branch head it left". | "Every run reports in one place"; "There is one digest, for work in every repository"; point 1 ends "the branch head it left in each repository it changed". |
| LOOP-07 | A reconciliation item "in its own queue"; a change the consumer needs is "a new item in the producer's queue"; adopted by each implementation "in its own queue"; intake judges it "was filed in the producer's queue". | "an item in the queue that names the consumer's repository"; "a new item that names the producer's repository"; "through its own reconciliation item"; "was filed as an item that names the producer's repository". |
| LOOP-08 | Seven steps; the seventh, "having a queue and workers". Step 6 began "protecting its trunk so that only a human merges (FLOW-08)". "An item in a component's queue may work in a repository that has not adopted"; the design says "which of its component's invariants apply"; "the two couple only through releases". | Six steps, and after them: "It gets no queue of its own: its work is items in the one queue that name it". Step 6 begins "seeing that only a human merges to its trunk (FLOW-08), and protecting the trunk where its host offers that" (a21, not the direction). "An item may work in a repository that has not adopted"; the design says "which invariants apply there, by rule ID"; "it and the other components couple only through releases". The agent part judges six steps. |
| LOOP-09, second paragraph, last sentence | "declares the workers it holds, in the components it affects (FLOW-05)" | "declares the workers it holds (FLOW-05)" |
| FLOW-03, the amended sentence | "A feature may also change its own component's rules and their enforcement, never a loop rule (LOOP-09), ..." | "A feature may also change the rules of a component it works in, and their enforcement, never a loop rule (LOOP-09), ..." |
| FLOW-14, the new sentence | "A feature's design may declare a change to its own component's invariants and profile and to their enforcement, ..." | "A feature's design may declare a change to the invariants and profile of a component it works in and to their enforcement, ..." |
| `TEMPLATE.md`, under **Design** | "The behavior and structure of the change." (unchanged by the text read) | The same, followed by: "Name each repository the item works in other than this one (LOOP-04); a design that names none works only here." |
| `README.md` of `docs/development/`, the new paragraph in **The core idea** | The loop rules are what "every BuildWarden component's loop runs under; they move to the loop repository (LOOP-08)" | What "the loop runs under in every repository it works in; they are kept here, and would move to a loop repository if one is created" |

One thing the direction's summary allowed and the contract does not say: a
hold that names repositories. The change proposal gives "A hold may name
repositories" to FLOW-05, in `loop-parallel`. FLOW-05 as it stands gives a
hold only affinity patterns, and `wf validate` rejects any other
(`queue.go`, `holdRe`), so a clause in LOOP-04 or LOOP-09 would not be true
on merge. The contract says a process item declares `holds` and leaves what
one names to FLOW-05 (Decision 18).

**The design around it.**

- *The front matter.* The comments on `seams`, `alters_relationships` and
  `native_validation`;
  `one-queue` added to `concepts`; LOOP-04's `rule_changes` entry gains
  `docs/design/TEMPLATE.md`.
- *The opening paragraph, Problem and Prior art.* One queue and the
  deferred loop repository; P1's items; `alters-slot`, `driver-contract`'s
  return to Design and the change proposal as prior art.
- *Decisions.* The preamble (two of the draft's three decided points are
  deferred). Decisions 3 (its example), 8, 10, 12, 13 and 15 reworded.
  Decisions 17 to 20 new.
- *Questions.* As the table at the head of **Assumed** lists them: seven
  unchanged, nine reworded, four new, a13 retired.
- *The contract's structure.* The titles of LOOP-04 and LOOP-06 in the
  table, and the sentence on the preamble.
- *The draft against the running loop.* Rows 8, 9, 10 and 13 of the
  disagreement table, and the last row of the blanks table. Row 10 records
  that the draft's sentence was written for separate queues and that Jeff
  has since chosen one. The line on the `integration` workflow now says
  that no repository setting requires it.
- *The component profile.* What `seam-specs` and `driver-contract` owe
  it, and `wf` reading a profile is now `loop-repository`'s in its new
  scope.
- *The split and the cross-references.* "Moves to the loop repository" is
  "would move if the loop repository is created" throughout, the rows for
  GLOBAL-08 and FLOW-20 in the split table included.
- *What this item's PR changes.* The `next.go` row is out; the rows for the
  umbrella and for the top-level `README.md` are in; the two settings
  files are carried only if `alters-slot` does not carry them.
- *The machinery changes.* Three became one, or two if a12 stays here.
- *What a second component needs.* The relay has no queue of its own; the
  list follows LOOP-08's six steps by number; the relay's settings as
  read; a third difference from its M1 design is listed.
- *The hand-off.* Rewritten: to `alters-slot`, `loop-parallel`,
  `loop-native-lead` and `loop-repository`, and to the deferred move.
- *Amendments to the umbrella design.* New: U1 to U22 (a20).
- *What this item does not declare.* The reason for
  `alters_relationships: false` reworded; a line that the item works in no
  second repository; the count of machinery changes.
- *Defaults and overrides.* The row for an unset
  `limits.alters_relationships` is out; the row for an item whose design
  names no repository is in.
- *Rule changes.* **What is true on merge** is rewritten for one queue.
  The paragraph under the FLOW-14 amendment says "if" the loop repository
  is created.
- *Validation and implementation plans.* The `alters_relationships` tests
  and task are out; the umbrella's amendments are task 12; tasks 5, 7 and
  8 gain the lines on an item's repositories; task 11 gains the README
  link; the "Before the merge" line and the note to the relay's M1 after
  the merge are new.

**Changed for a reason other than the direction**, and so pointed out:

- *LOOP-02's example.* The rule's text named no example and is unchanged.
  The design used the driver contract, "a Go API", as its example of a
  declared seam with no written specification (Decision 3, a8, the section
  on which applies when and its table). Jeff has since sent
  `driver-contract` back to become a contract of commands on a driver
  binary. The example is now the bootstrap model, and the design says the
  rule reads the same whether or not the driver contract gets a
  specification.
- *Trunk protection* (a21). `main` has none; at `2889919` the setting was
  listed as not verified. The relay repository is private and cannot be
  protected on the current plan, while LOOP-08 says no loop rule may
  require a public repository. LOOP-08's step 6 is reworded to ask for
  protection where the host offers it, and a21 puts that and three other
  options to Jeff.
- *The link from `README.md`.* LOOP-08's step 1 asks it of a component's
  README. This repository's has none, and the PR as read at `2889919` did
  not add one. Task 11 does.
- *a12's recommendation.* `alters-slot`'s design proposes to carry the two
  Entire files and is meant to merge first, so a12 now recommends that it
  does, with this PR as the fallback.
- *What `driver-contract` owes the profile.* It is being redrafted, so the
  sentence now gives the test and no list.
- *The umbrella's a5.* The change proposal left a5 to `loop-parallel`.
  U5 adds only a pointer: the limits are the one queue's, and FLOW-05 says
  who is counted. It states no new limit.
- *The umbrella's K4 row* (U22). Its done-when lists what this repository
  holds at the end of K4. With the loop repository deferred, that includes
  the loop.

**After an independent read of the revision at `cf44244`.** Two reviewers
read it against the rules, the tools, both repositories' settings and the
change proposal. Their findings were checked against the source and all
but two parts were taken. What that pass changed, beyond accuracy fixes:
the clause on a hold naming repositories left LOOP-04 and LOOP-09; LOOP-04
says where an item names its repositories and what naming none means, and
the template, the skills and `wf-design` gain the lines that carry it;
LOOP-04 keeps the sentence on waiting for a release, which `cf44244` had
dropped without a reason; LOOP-08's step 6 and a21 take in the relay's
settings; the FLOW-03 and FLOW-14 amendments lose "its own component's";
U22; the README link; a12's recommendation; and the hand-off gains the
private repository's reports and the per-repository branch head for
`loop-repository`. Not taken: editing the first two entries of this
section, which are records (the two notes above stand in for the edits),
and committing the queue file, which the review session writes with its
own state.

## Consolidation

A process item has no Publish phase and no consolidation pass (FLOW-03).
The implement step merges current `main` before it opens the PR and records
here what it reconciled, the profile's entries in particular (task 14).

## Rollback notes

None.

## Archive summary

Filled in by the implement step when it archives the design on the branch
(FLOW-03, process phase 2; FLOW-12).
