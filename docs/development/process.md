# Development Process Standards

How a feature moves from an idea to a merged, archived change, whether the work is
done by a human, an attended agent session, or the unattended feature loop. The
other documents in this directory say what a good change looks like; this one says
how changes are designed, sequenced, validated, and signed off so that the other
standards are actually applied.

See [`README.md`](./README.md) for the rule format.

## The shape of the process

Features move through a work queue. Each feature passes through the development
phases **once**, on its own feature branch. Work goes depth first on the
first item in FLOW-05's order; others advance only while it is blocked
(FLOW-05).

The human takes part at three points, and the loop does everything else:

1. **Questions** Design cannot settle alone (FLOW-07).
2. **The design review**: one interactive session in which the human goes over
   a non-trivial design, after Design and before Plan, and again only when a
   later phase finds the design must change (FLOW-19).
3. **The merge** of the feature's PR, which is the sign-off (FLOW-08). The PR
   carries the archived design (FLOW-12).

Publish takes a feature through the PR slot (FLOW-05): no two features
that collide or are near have pull requests open together, while
disjoint ones may, and their pull requests merge one at a time, each
only while its branch contains `main` (FLOW-08). Publish merges current
`main` and runs a **consolidation pass** against every feature that landed while
it was being built (FLOW-10, FLOW-15), and a refresh runs it again whenever
`main` moves past the branch; either sends the feature back only when the
pass finds real work. This is the routine form of the consolidation that
followed the Windows driver work (the claim / tunnel / collector model). Doing it
per feature keeps the reconciliation small and keeps terminology and shared
abstractions converging rather than forking.

Work is done by **workers** that pull steps from the queue. A worker is the
unattended cloud loop, a human-attended session, or a session on a specific native
host. Every step declares which kind of worker may take it (FLOW-04), so one state
machine covers all three.

---

### FLOW-01 — Design before implementation

**Severity:** MUST
**Check:** automated + agent-judgment

Every feature has a design document under `docs/design/`, written from
[`TEMPLATE.md`](../design/TEMPLATE.md), before implementation begins, and the PR
that lands the feature links it. Changes that carry no design decisions
(dependency bumps, typo fixes, CI-only changes) are exempt and are labelled
`no-design` on the PR.

A feature that changes behaviour a specification defines is also
spec-first: the specification changes before the code that implements
it, and the design lists the change (LOOP-02).

The design phase starts by searching archived designs (FLOW-12) and these standards
for overlap, and cites what it reuses.

The automated part checks that a non-exempt PR links a design document with valid
front-matter; the agent part judges whether the design actually covers the change.

---

### FLOW-02 — Designs declare their impact

**Severity:** MUST
**Check:** automated + agent-judgment

A design's front-matter declares what the change reaches, so the behavioral linter
can compare the claim against the diff and the queue can route the work:

- `drivers` — here: which drivers are touched, or `interface` for a
  change made at the level of the driver contract or `driver.Session`
  (ORCH-01).
- `seams` — which versioned seams change (GLOBAL-08).
- `surfaces` — here: which user-facing surfaces change (UX-01, ORCH-03).
- `repos` — every repository the change works in, the one that holds the
  design first, when there is more than that one (LOOP-04).
- `merge_first` — the repositories in `repos`, other than the home, whose
  pull request merges before the home's is listed ready (FLOW-08). Empty
  or absent means none.
- `areas` — the areas of an area map whose files the change edits
  (FLOW-05). A bare name is an area of the map of the item's home
  repository; an area of another repository the item works in is written
  `<owner>/<name>:<area>`. A design that leaves the key out is
  undeclared, as FLOW-05 defines it, and FLOW-05 then treats the item as
  colliding with every item in its repositories. `areas: []` is a
  declaration: the change edits nothing but its own design, though an
  item whose home repository has no map is undeclared whatever it
  declares (FLOW-05).
- `security_boundary` — here: which GLOBAL-06 boundary is altered, if
  any. Any value other than `none` requires a security note in the
  design.
- `alters_relationships` — `true` when the change alters a component's
  role or how components interact: a versioned seam (GLOBAL-08), or which
  component owns a responsibility; here also the bootstrap model (BOOT
  rules), which of relay, collector, executor, orchestrator or a driver
  owns a responsibility, or the driver contract or `driver.Session`.
  Changes inside one component's existing role are `false`. FLOW-05
  limits how many such features are in flight.
- `native_validation` — which native hosts must validate the change, and whether
  each blocks merge (FLOW-09).
- `tier2_gaps` — here: platform validation knowingly deferred (TEST-03).
- `rules` — the rule IDs the design relies on or changes.
- `rule_changes` — each rule change the feature makes (FLOW-14), as
  `{rule, change: add | amend | retire, paths}`, `paths` naming every file it
  edits, enforcement included. Empty or absent means none.
- `concepts` — retrieval tags for the archive (FLOW-12).

A key whose bullet begins "here:" is one this repository's profile file
adds to the loop's keys (LOOP-01, its impact keys), and the rest of the
bullet is this repository's entry for that term; a design in a component
whose profile file does not add it leaves it out. Elsewhere, in a bullet
of one of the loop's keys and in the paragraphs below, "here" introduces
this repository's examples of the term.

Declarations are derived conservatively: a change to a shared package
declares everything that depends on it (here a change to `relay/`,
`ledger/`, `collector/` or `driver/` declares every dependent driver and
host, per the TEST-02 caveat). `areas` name what the change edits, not
what depends on it: the area map says which area reaches which.

The automated part diffs the declared `seams`, and the keys the profile
file adds that name trees or files (here `drivers` and `surfaces`),
against the trees and files the change reaches. It checks the declared
`areas` against what those profile keys and the `rule_changes` paths
imply (here `drivers` and `surfaces`), at Design, and against the paths
the change edits, from Implement through Publish and at each refresh of
a feature (FLOW-03). The agent part judges the
security note, where the profile's keys require one (here a
`security_boundary` other than `none`).

---

### FLOW-03 — Phases, transitions, and definitions of done

**Severity:** MUST
**Check:** automated

Each feature is a queue item whose state records its current phase, its
branch, the repositories it works in and what its design declares
(FLOW-02). The phases are:

| # | Phase | Done when |
|---|-------|-----------|
| 0 | Branch setup | Branch exists on the remote. |
| 1 | Design | Design doc complete per FLOW-02; the queue records what it declares, and its areas cover what its other fields imply (FLOW-02); no unanswered blocking question (FLOW-07); whether it needs a design review decided (FLOW-19). |
| 2 | Plan | The design review recorded as approved, or validly skipped, before planning starts (FLOW-19). Implementation and testing tasks listed, each citing the rules it must satisfy; rework of existing code is listed as explicit tasks. The design's impact fields and `rule_changes` unchanged since its last approval, when there is one (FLOW-19); a skipped review is rechecked by its skip criteria. |
| 3 | Implement | Tasks done; the fast gate green (LOOP-01; here `make lint` and `make test`); reference docs updated in the same change (UX-01); every edit to the rules or their enforcement cleared by an approved rule change (FLOW-14). |
| 4 | Review | Behavioral-linter tier 1 green; tier-2 report produced for the diff (LINT-04); the rule-change clearance still passes (FLOW-14). |
| 5 | Guide | User-facing narrative docs (guides, examples) written or updated. |
| 6 | Publish | Once the PR slot is free (FLOW-05): current `main` merged; consolidation section filled (FLOW-10, FLOW-15); the rule-change clearance still passes (FLOW-14); design archived with its summary on the branch (FLOW-12); PR opened (no confirmation needed, FLOW-08) with CI green (FLOW-11); blocking native validation done (FLOW-09); each pull request its design names to merge first merged, and the loop has seen it (FLOW-08). |
| 7 | Merge | The human merged the PR (FLOW-08), or, for a PR to a program trunk, the loop merged it once FLOW-08's merge gate passed (FLOW-20). The loop confirms it, in an unattended run or in an attended session (FLOW-04); there is no step for the human to claim. The PR is ready for the merge only while its branch contains its trunk, `main`, or, for a PR to a program trunk, that program trunk (FLOW-08); when that moves past it, a refresh step brings it up to date (FLOW-10). |
| 8 | Close | Merged branches and validation leftovers cleaned up (FLOW-16); the design is archived on `main`. |

A transition happens only when the phase's done-when checks pass mechanically
(files present, sections filled, commands green), never on an agent's own report
that it is done. From Plan through Publish, and at each refresh of a
feature (FLOW-10), each done-when also checks that the queue still
records what the design declares; from Implement through Publish, and
at each refresh of a feature, that every path each of its branches
changes lies in an area the design declares in that repository's map
(FLOW-02). A path outside them is
settled by widening the declaration inside the approved design, or by a
return to Design (FLOW-06). The merge confirmation (phase 7's
done-when) and Close run neither; a feature's refresh runs both,
including one taken while the pull request waits at Merge, and a
process item's refresh runs neither. An item the queue records
no declaration for, because it finished Design before the queue
recorded declarations, is not compared with its design; its diff check
reads the areas the queue records, so with none it is undeclared
(FLOW-05) and no path lies outside them.

An item has a branch in each repository it works in, under one name, and
the queue records each branch and its pull request. Where a done-when
names the branch, the fast gate, `main`, the reference documents or the
PR, it holds in each of those repositories, read through that
repository's profile (LOOP-01). In a repository that has not adopted the
contract (LOOP-08), which the loop's configuration records with no
profile, there is no profile to name a gate: the fast gate and the
reference-document checks do not run there, no path there is a rule
path, and the branch, area, trunk and pull-request checks do run. The
design and its review are the home's. "`main`" in the tables above reads
as each repository's trunk.

A **process item** (queue `kind: process`) changes the rules the loops run
under: `docs/development/`, `.claude/` or `tools/workflow/`. Its design is
settled by the human in a design review (FLOW-19), or, while FLOW-20 stands
and the change touches none of the rules FLOW-20 reserves to the human, by
FLOW-20's process test; after that the loop works on its
branch, and nothing it changes reaches the running loop until the human merges
it (FLOW-14). A feature may also change the rules of a component it works in, and
their enforcement, never a loop rule (LOOP-09), when its design review
approves each change by name (FLOW-14); `.claude/` and the
rest of `tools/workflow/` change only through a process item:

| # | Phase | Done when |
|---|-------|-----------|
| 1 | Design | The design document exists on the branch and its design review is recorded as approved; while FLOW-20 stands, the design document exists on the branch and Design has recorded whether it takes the process test or the human's design review (FLOW-19, FLOW-20). |
| 2 | Implement | While FLOW-20 stands, its design review recorded as approved, or its process test as passed, before implementing starts (FLOW-20). The branch is pushed (FLOW-17), the design archived on it with its summary, and its PR open with CI green. |
| 3 | Merge | A human merged it into `main` (FLOW-08), confirmed as phase 7's merge is. Its PR is ready and is refreshed as a feature's is (FLOW-08, FLOW-10). |
| 4 | Close | As phase 8, and its state step done when the human recorded one: a `human` step that brings the `workflow` branch in line with the merged change. Only the human records it, at the design review or later; the queue's tooling adds it when Close is entered, or at once when it is recorded during Close, and Close's own step waits for it. |

A feature whose home has adopted the contract has the phases above,
whichever repository that is. A feature whose home has not adopted it is
**tracked**. The queue records it, its repository and its pull
request. Its phases are two, Work and then Merge, each one `human` step
(FLOW-04), and no done-when check runs for them. FLOW-05's limits
neither count it nor hold it.

Queue state lives on the `workflow` orphan branch, not on `main`,
so that high-churn bookkeeping stays out of the code history.

---

### FLOW-04 — Steps declare who may take them

**Severity:** MUST
**Check:** automated

Every pending step carries an affinity:

- `cloud` — design, planning, implementation, review, docs, CI fixes. Taken by the
  unattended loop.
- `human` — answering blocking questions, the design review (FLOW-19),
  except one FLOW-20's umbrella test or process test takes, which is
  `cloud`, and escalations: a runaway budget (FLOW-13; a program item's
  second), CI repair exhausted (FLOW-11), a third return to Design
  (FLOW-06; a program item's fifth), a program sync whose conflict the
  loop cannot resolve (FLOW-20), a branch that did not delete on merge
  (FLOW-16).
  Also a tracked item's steps (FLOW-03), and a step its design adds that
  only the human may take, such as cutting a release.
- `host:<name>` — work that needs native hardware, for example `host:macos-vz`
  (the macOS/arm64 laptop) or `host:windows-hyperv` (the Windows desktop). A
  name `<os>-<driver>` is specific to that platform; `<os>-any` needs the
  platform and no one driver; `<capability>-any` (such as `qemu-any` or
  `container-any`) runs on any native machine that has the capability. The
  workflow branch's `hosts.yaml` lists the machines that serve each host
  name, one or more, and only the human changes it.

A feature may be **led** by a host name, which any worker may set or
change, each change recorded in the item's history and the run's digest
(FLOW-09). Its steps of Branch setup through Guide that would be `cloud`
carry that `host:<name>` instead; its design review stays `human`, and its
Publish, Merge and Close stay `cloud`. A claim by a worker that serves the
lead starts a queued led feature, and no other claim does.

A merge step, and a step confirming that a pull request its design names
to merge first has merged (FLOW-08), stays `cloud` and is never offered
to a worker (FLOW-05), except the merge of a pull request into a program
trunk, and the confirmation of one into a program trunk, which are
offered to a `cloud` worker that merges it while FLOW-08's merge gate
passes for its repository (FLOW-20).
A `human` or `host:<name>` worker may also complete one, which is the one exception to
taking only steps whose affinity a worker satisfies: the same done-when
check runs, so the step completes only once the merge has happened, and a
step already done is not completed again. Completing it records the
human's merge; it never makes one (FLOW-08).

A worker takes only steps whose affinity it satisfies: a machine satisfies
each host name the table lists for it, for a step in a repository the
table lists for that machine (only the queue's own repository when it
lists none; a led item's step is in every repository the item works
in), and a `host:<name>` step goes to whichever such worker next takes
steps. A step is offered when
every step it needs is done and, unless it is `human`, its feature has reached
the step's phase; no worker may claim or complete a host or cloud step of a
later phase. A step is claimed with a
lease (claimant plus expiry) recorded on the `workflow` branch, and only
when FLOW-05's limits allow the claim, whoever the worker is. A claim
names the worker's kind and session, and a machine's session names the
machine. When the
push of a claim is rejected because the branch moved, the claim is asked
again on the new state: it stands if the step may still be claimed
there, and otherwise it is lost and the worker re-reads state. A
machine's lease lasts 20 hours, or what the human sets in `config.yaml`;
its session renews it while it is actively working on the step, and a
renewal counts nothing against the item's budget. A `cloud` or `human`
lease lasts what the human sets, 120 minutes when he sets none. When a
machine's scheduled session ends, the machine releases every lease that
session still holds, and a machine releases at its next run any lease an
earlier scheduled session of it left; an attended session releases its
leases at its close, and a lease it leaves runs to its expiry. An expired
lease may be reclaimed.

---

### FLOW-05 — Depth first, in class order

**Severity:** SHOULD
**Check:** automated

Work goes deep, not wide. Open items are taken in class order, and within a
class in-progress items come before queued ones, then the higher priority:

1. **In implementation**: an in-progress feature in Implement (phase 3)
   through Close, or an in-progress process item in Implement (phase 2) or
   Close, except while it only waits for the human's merge. An item whose
   open PR needs a refresh (FLOW-10) is in implementation.
2. **Process items** (FLOW-03), which are usually short and change the rules
   the rest of the queue runs under.
3. **Fixes and adoptions**: a feature that `follows` an item, fixing or
   finishing it, which starts only once that item is done; and a
   reconciliation item (LOOP-07), which the queue marks with `adopts` and
   which starts once the release it adopts exists (LOOP-04). Intake
   places a reconciliation item below the open fixes unless the human
   says otherwise.
4. **New functionality**: every other feature.

An open item's priority is its position in this order, 1 for the first;
a done item has none. The queue's tooling (`wf`) renumbers the open items
whenever a change moves one: an item queued or moved, started, changing
class, or done. Renumbering changes the numbers, never the order. Only
the human places an item, at intake or by moving it, except where a rule
places it (FLOW-09, LOOP-04, FLOW-20), and the position asked for is kept
as nearly as the item's class, and its status within that class, allow.

While a process item is in progress, a worker matching one of its `holds`
(affinity patterns such as `cloud` or `host:*`) takes no new step, whatever
the class of the item the step belongs to, so disruptive work waits for the
rules to settle; a step it already claimed stays claimed, and finishing the
process item releases the hold. A hold names workers and covers work in
every repository. Otherwise a worker takes the next ready step
of the first item in that order, and keeps taking that item's steps until it
blocks: on a step whose affinity it does not satisfy (FLOW-04), an
exhausted budget, a limit below, or the human's merge (FLOW-08). Only then
does it take steps from the next item down, and it returns to the earlier
item as soon as that one has a ready step again.

A worker starts a queued item only when every item ahead of it is
blocked. Two items that change the same thing are likely to need an
involved consolidation once the first lands, so the limits below bound
how many such items are in flight. Items that do not collide may be in
flight, and have pull requests open, side by side; their pull requests
merge one at a time (FLOW-08).

**How two items relate.** An item names the repositories it works in and
the areas it changes (FLOW-02). A repository's *area map* gives each
area its paths, its mode and the areas it reaches. The map is kept with
the queue state, where the Areas entry of the repository's profile says
(LOOP-01), and it changes only in an attended session, on the human's
word: no agent changes it otherwise, except that, while FLOW-20 stands,
the loop makes a change that a program item's design, reviewed or passed
by FLOW-20's umbrella test, names, or that brings the map's `reaches` in
line with the repository's dependency edges, and records it (FLOW-20). Two items cannot both change an
*exclusive* area; any number may change an *ordered* area, and the
later to merge is put in order by its refresh (FLOW-10). Area A *reaches* area B when code in A
depends on code in B; only exclusive areas reach and are reached. A
repository with no map is one exclusive area. An item is *undeclared*
when its `areas` are left out, when its home repository, the first it
names and the one that holds its design (FLOW-02, LOOP-04), has no map,
or when they name an area the map lacks, until the map has it;
an undeclared item is taken to change the whole of each repository it
names. A declared item whose areas are `[]` changes nothing but its own
design (FLOW-02). Two features are:

- **colliding** when they share an exclusive area; when they have a
  repository in common and either is undeclared; when both change a
  seam that has no specification (LOOP-02); or when the human says so in
  a design review (FLOW-19);
- **near** when they do not collide and an area of one reaches an area of
  the other, both change the same rule (FLOW-14), or both change a seam
  that has a specification;
- **disjoint** otherwise.

A process item is outside these relations: it stops other work only
through `holds`. A tracked item (FLOW-03) is shown with its relations and
neither holds nor waits.

**The limits.** A feature *holds* a limit from the claim that takes it.
A holder's *set* is the holder and the other holders it collides with;
for the PR slot, collides with or is near. No holder's set may number
more than the limit: a claim that takes a limit is allowed only while
the claimant's set, and the set of every holder it relates to, would
stay within it. Only holders are counted and an item that holds is
never refused, so no two items wait for each other.
Process items are not counted because they are short and already ranked
ahead of fixes and features; one that would disrupt running work declares
`holds`. An item on hold, whose open blocking question names when it
resumes (`resumes`), is not counted while it is held.

- **In progress.** A feature takes it when it starts and holds it while
  it is in progress.
- **`alters_relationships`.** A feature that declares
  `alters_relationships: true` (FLOW-02) takes a slot with the claim of
  its Plan step and holds it until it is done, a return to Design
  included, because two such features reshape the same structure and
  make each other's consolidation costly. A feature whose design
  discovers it alters relationships waits at the end of Design until a
  slot is free.
- **The PR slot.** A feature takes the PR slot with the claim of the
  step that merges `main` at the start of Publish and holds it until its
  PR merges, a return to Implement or Design after the PR is open
  included. Its limit is 1, so no two features that collide or are near
  have pull requests open together. When related features wait for the
  slot, the higher priority goes first. A feature finished through Guide
  waits for the slot, so its consolidation runs against a `main` that
  the items related to it leave alone until its PR merges. A disjoint
  item may merge meanwhile; the open PR then waits for its refresh
  (FLOW-10) before it may merge (FLOW-08).

**Waiting on the human.** A fourth limit bounds what waits on the
human: a queued feature does not start while that many items, or more,
wait on the human. An item
waits on the human when it is in progress and has a `human` step of a
phase it has reached that could be taken now, an open blocking
question, a pull request waiting for
the human's merge, or held at a program trunk's merge on a security note
or a finding only the human may clear (FLOW-08), or a spent budget
(FLOW-13), which a program item's first is not (FLOW-20); it counts once, and
neither a tracked item nor an item on hold (`resumes`) counts. This limit
counts by no relation and stops only a
start: work in progress goes on, so the count can pass it. A fix that
`follows` an item, the fix for a failing `main` or program trunk
(FLOW-09), a security fix the human has marked urgent, a process
item and a tracked item start whatever
the count. Only the human marks a security fix urgent; the mark lifts
this limit for it, and nothing else: it does not release a hold
(FLOW-20).

The in-progress limit, the `alters_relationships` limit and the waiting
limit are the values the human sets in the workflow branch's
`config.yaml`: 5 features in progress, 1 `alters_relationships` slot
and 8 items waiting when it sets none. Every claim is checked
against the limits, whoever the worker is (FLOW-04). A feature waiting
behind others periodically merges `main` with conflict-only fixes, so its
consolidation stays as small as it can.

---

### FLOW-06 — Implementation does not silently redesign

**Severity:** MUST
**Check:** agent-judgment

When implementation finds that the plan is no longer viable, or implementation
or planning finds that the design must change or uncovers complexity it did not
consider, the feature returns to Design with the finding recorded. Design then
decides; planning adds any alteration of already-implemented code as explicit
tasks, and the changed design goes through a design review again, judged on its
delta (FLOW-19). A feature that returns to Design more than twice escalates to a
`human` step instead of looping again; a program item, more than four times
(FLOW-20). Each of a program item's returns past the second is recorded. When Publish's consolidation finds work
the plan already covers, the feature returns to Implement instead, with no new
review. A return drops every pending step of the phase it returns to and the
pending template steps of later phases, which are seeded again on re-entry.
Other pending steps of later phases stay, and Design reconciles them with the
new design, except a pending refresh step (FLOW-10), which is dropped:
Publish merges `main` again. A native validation already done in Publish
is queued again.

---

### FLOW-07 — Questions are asked in design, in two kinds

**Severity:** MUST
**Check:** agent-judgment

Questions about intent are raised during Design, not discovered mid-implementation.
Each question is one of:

- **Blocking** — the design cannot proceed without the answer. The feature waits on
  a `human` step; the worker moves on to other work. For a program item
  (FLOW-20), a question is blocking only when it is component design or
  needs setup or privileged work; any other proceeds as assumed, on its
  recommendation.
- **Assumed** — the design proceeds on a stated default, recorded in the design as
  "proceeding with X; confirm". Assumptions are confirmed in the design review
  (FLOW-19), and must all be confirmed before planning starts.

Every question states the options considered and a recommended answer. Later
phases route questions back through Design, which escalates only what the design
has not already settled. Questions live in the feature's queue state on the
`workflow` branch and in the loop's digest, not as GitHub issues.

---

### FLOW-08 — Only a human merges to trunk; the loop merges to a program trunk

**Severity:** MUST
**Check:** automated

This is LINT-03 applied to the process. No agent approves or merges a PR
to a trunk, in any repository, and auto-merge is not enabled by agents. The human's merge is the sign-off: it
covers the PR, the archived design it carries, and the tier-2 behavioral report,
whose MUST findings the PR body lists. The human acknowledges them by merging, or
dismisses one with a recorded reason in a PR comment. Changes that declare a
`security_boundary` additionally need the security note acknowledged (GLOBAL-06).

While FLOW-20 stands, the loop merges a program item's pull request into
its program trunk, which is not a trunk, and nothing else, except the
program sync below. It merges a program item's pull request, in each
repository the item has one, only while the *merge gate* passes on its
head:

1. the base is the program trunk;
2. the head is the one recorded for the item's pull request and the
   remote branch's head;
3. every check on it passed or was skipped, none failed, and every check
   the program trunk's protection requires passed;
4. the branch contains the program trunk;
5. the tier-2 report is for that head and has no open MUST finding but
   one FLOW-20's records defer;
6. no human step of the item is pending;
7. its `security_boundary` is `none`, or its only trip is GLOBAL-06's
   relay clause with no change to a boundary's posture, which its review
   accepted; otherwise the human acknowledges its security note before
   the merge;
8. each pull request its design names to merge first has merged;
9. the rule-change clearance passes;
10. GitHub reports the pull request mergeable by its base branch's
    protection for any caller, read before the merge (`mergeable_state`
    `clean`, `unstable` or `has_hooks`), never while it is still being
    computed.

The *program sync* is a pull request whose head merges the trunk into
the program trunk. The loop merges it once points 1 to 4 and 10 pass,
where the
head in point 2 is the one the sync step recorded and point 4 also needs
the head to contain the trunk, and point 5 on its conflict-resolution
commit when it had one (FLOW-20). The loop merges with a merge commit,
never a squash or a rebase, pinned to the head it checked, and records
the merge (FLOW-20). It merges into a program trunk one pull request at
a time per repository: no loop merge in a repository is taken while
another holds its claim, and under the claim the loop reads the program
trunk's head again, runs the gate on it and merges at once. The loop
defers a MUST finding
only after one fix round, and never one under GLOBAL-06, a SEC rule,
RELAY-01 to RELAY-03 or RELAY-05, or that breaks what FLOW-20 says never
breaks: such a finding returns the item to Design for a change that
conforms (FLOW-20). It never dismisses one. The human's merge of the program trunk
into its trunk is the sign-off for everything the loop merged there, and
its pull request lists each deferred finding and each security note to
acknowledge.

There is no sign-off step in the queue: the loop sees the merge of each
pull request an item records, in whichever repository it is (FLOW-03,
phase 7). Only a tracked item's merge is recorded by a `human` step
(FLOW-04); the merge is still the sign-off, and the step only the queue's
record of it. An item with pull requests in several repositories is
signed off by the merge of each. Its design names any that must merge
first. Each of those is opened in Publish and is ready for the merge as
any other is, and the home's Publish, and any step that needs one of
those merges, waits until the loop has seen it (FLOW-03, phase 6).

A pull request is ready for the human's merge only while its branch
contains trunk and its checks have passed on that head. Each merge to
a trunk therefore leaves every other open pull request to that trunk
waiting for a refresh (FLOW-10), and the loop lists none of them as
ready until it has had one. Where the trunk's host can refuse a merge that is not
ready, the human has it do so (LOOP-08). That setting is the human's,
as the merge is: no agent changes a trunk's or a program trunk's
protection. A merge the
human makes all the same is still the sign-off.

Neither pushing a feature branch (FLOW-17) nor opening its PR to its
trunk is a trunk change, and neither needs confirmation. An agent opens the PR once the work
is ready (Publish, or a fix); the human's review is the merge, or comments on the
PR when more is wanted.

---

### FLOW-09 — Native validation: blocking or tracked

**Severity:** MUST
**Check:** automated + agent-judgment

Native validation is tier-2 testing (TEST-02) that cannot run in CI today, for
example VZ on macOS/arm64 or Hyper-V on Windows. Whether it blocks merge is decided
per host in the design:

- **Blocking** when the change is specific to that host's driver or lifecycle: it
  reaches that driver's tree, or alters behavior only that host exercises. Such a
  feature waits in Publish on a `host:<name>` step, which runs after
  consolidation and which the PR's opening needs.
- **Tracked** otherwise: the gap is recorded in `tier2_gaps` and on the `workflow`
  branch's native-gap backlog, satisfying TEST-03, and the feature may merge.

Native sessions drain their host's steps and backlog in batches, testing `main` plus
every waiting branch in one sitting rather than one session per feature. Each
native machine also runs a scheduled session that starts an agent only when a
host step it serves is ready, or when a trunk, a program trunk or a waiting
branch of a repository it lists has moved since its last finished sweep of open
backlog entries, and never while the human has not confirmed the priority order; it starts from trunk, is bound by the
unattended loop's limits (FLOW-13) and records what it did in the digest.
Every scheduled run, including one that stops before an agent starts, also
records the machine's outcome and a one-line reason in `native/status/<machine>.yaml` on the `workflow` branch, so a
silent failure is still visible. A failure
the feature's branch causes returns the feature to Design or Implement with the
logs attached (FLOW-06). A regression `main` already has, found while the
feature is built, validated or consolidated, is queued at once as the feature's
immediate follow-up unless the human explicitly says otherwise: the session that
found it files a fix item that `follows` the feature, at the top of the fixes,
and the feature neither waits for it nor absorbs it.
A failure that CI reports on `main` after a merge is such a regression:
the run that sees it files the fix the same way, as the follow-up of the
item whose pull request that merge was, and files nothing when the
merge was no item's. A follow-up filed with no human present takes the
repositories and areas of the item it follows (FLOW-02), for its design
to confirm. The full matrix before a release (TEST-02) is the backstop for every
tracked gap.

A feature whose design or implementation itself needs a native host (for
example a platform or hypervisor update, or a driver only that platform
builds) is **led** by that host from its first step (FLOW-04): any worker
may name the host name as the item's lead, at intake or later, and a
machine that serves it takes the item's steps through Guide that would
otherwise be `cloud` (FLOW-04). Its Publish runs on the cloud, and its
native validation is blocking or tracked as its design says. A process
item's native work is done in an attended native session or tracked as
native gaps, as its design says; it is never led.

---

### FLOW-10 — Publish consolidates

**Severity:** MUST
**Check:** agent-judgment

Publish, after merging each trunk the item has a branch on (FLOW-03),
reviews the feature against everything that landed on each since the
branch forked, and records in the design's consolidation section:

- terminology aligned with the canonical terms used in `main` and these standards,
- shared abstractions reused instead of duplicated (one code path where two
  drivers or components do the same thing),
- versioned seams that more than one feature changed (GLOBAL-08),
- behavior or convention changes the rest of the codebase must now follow.

A convention that emerges from consolidation and should bind future work is
proposed as a new or amended rule in this directory. It lands in the same PR
only as a rule change the design review approved (FLOW-14); otherwise the
feature goes back to Design for a review of it (FLOW-06), or the rule becomes a
follow-up item. Rule changes go through the human's merge like any other change.

When consolidation finds work beyond conflict fixes and wording, the feature goes
back: to Implement for work the plan already covers, or to Design (FLOW-06)
when the design must change.

A PR that waits for the human's merge may be merged only while its
branch contains its trunk (FLOW-08). Whenever a trunk has moved past the
item's branch on it, a **refresh** step merges that trunk into that
branch again, runs the gates in that repository, and repeats this review
and FLOW-15's sweep over what landed since that branch last merged its
trunk, as a dated entry in the consolidation section naming the trunk.
The PR is ready again when the step's checks pass. It pushes only while
the pull request is still open. A feature that has merged its trunks in
Publish and has not finished Publish is refreshed the same way when a
trunk moves, before its PR is opened or listed as ready. No refresh of a
branch is taken while its trunk is known to fail its own checks (the
trunk's `main.yaml`, `repos/<owner>/<name>/main.yaml` for a second
repository), except for the item that fixes that trunk (FLOW-09): it
would fail on the same failure. A refresh that finds work beyond
conflict fixes and wording sends the feature back as above; its pull
request is then a draft until the feature is published again, and it
keeps the PR slot (FLOW-05). A process item's open PR is refreshed the
same way, without the review.

"Its pull request" reads per repository throughout.

---

### FLOW-11 — CI repair is bounded

**Severity:** MUST
**Check:** automated

The Publish phase fixes CI failures for at most three attempts per distinct
failure before escalating to a `human` step. A failure that does not reproduce is
treated as a flaky test under TEST-04: quarantined and filed as a defect, never
retried until green.

---

### FLOW-12 — Merged designs are archived for reuse

**Severity:** MUST
**Check:** automated

On merge, the feature's design document has its status set to `archived` and its
summary section filled in. Publish makes both changes on the feature branch, so
they are part of the feature's PR and land in the same merge as the code, and
refreshes the summary whenever the PR changes after review. The summary records
what was built, the decisions taken and the alternatives
rejected, and what a future design touching the same `concepts` needs to know. The
archive is `docs/design/` itself; retrieval is by front-matter (`concepts`,
`drivers`, `seams`, `rules`). Designs written before this standard carry no
front-matter and are retrofitted when a new design first relies on them.

---

### FLOW-13 — Unattended steps are bounded and visible

**Severity:** SHOULD
**Check:** automated

The unattended loop runs once a day in the cloud, and each native machine runs
its scheduled session on its own schedule (FLOW-09). Each run takes steps one
at a time (read state, claim, work, write state) until no step it may take is
ready or the run's budget is spent, and writes state after every step so an
interrupted run loses at most the step in progress. All state it needs lives in
the repository, never in a long-running conversation. No unattended run takes a
step while the human has not confirmed the priority order. A run spends its
budget in FLOW-05 order, so the first item in FLOW-05's order can use all of it.
The cloud run spends `run_budget`. A scheduled native session spends its own
budget, `native_budget`, which falls back to `run_budget` when unset, so
native and integration testing never compete with the cloud loop; a step it
takes for an item a machine leads counts against it like any other. Each
feature also has a work budget, which is a runaway check rather than a share of
the run: an open (queued or in-progress) item that spends `runaway_factor` times
its estimated steps escalates to a `human` step (a program item's first
time, its estimate is raised to its spend so far instead, and recorded; its
second escalates, FLOW-20), and no unattended run takes
more of its steps before Merge, nor any step of an item with an open
blocking question (FLOW-07). Only a step that changes the item's
repositories counts: every claim of a step before the item's Merge phase
counts against the work budget and the run's, whichever worker makes it,
except the first claim of a refresh (FLOW-10); renewing a lease does not
(FLOW-04). From Merge on (a merge or its confirmation, an
`f6-merged-<n>`, Close and its state step), and the records and digest a
run writes, nothing counts against either budget, and the runaway check
holds none of it. Runs are counted but never trip it. The
loop posts a periodic
digest (what moved, what is blocked on a human, what is waiting on a native host,
what open items spent), and notifies when a new blocking question appears. Done
items are left out of the runaway check, of the digest's blocked and spent lines,
of `wf status` and of `QUEUE.md`; their budgets are looked at again only in a
retrospective the human asks for. Reading archived designs (FLOW-01, FLOW-12) and
open native gaps from merged designs (FLOW-09) are not affected.

---

### FLOW-14 — Agent tooling is a thin layer over this doc set

**Severity:** MUST
**Check:** automated + agent-judgment

Agent definitions and skills (for example under `.claude/` or `.kiro/`) carry
phase instructions and cite rule IDs; they do not restate rules. When a rule and an
agent definition disagree, the rule wins and the definition is fixed. Agents do not
change their own definitions or these standards on `main` without the human.
There are two routes, and both reach the running loop only through the human's
merge (FLOW-08):

- **A process item** (FLOW-03), whose design the human settles in a design
  review (FLOW-19), or, while FLOW-20 stands, whose design passes FLOW-20's
  process test because it touches none of the rules FLOW-20 reserves to
  the human, before the loop implements it on its branch. It is the only
  route for `.claude/` and for `tools/workflow/` outside `check/`, the loop's
  own machinery, for a loop rule (LOOP-09), and the route for a rule change
  no feature needs.
- **A rule change a feature's design declares.** A feature's design may
  declare a change to the invariants and profile of a component it works
  in and to their enforcement, and never to a loop rule (LOOP-\*,
  FLOW-\*, LINT-\*) or its enforcement (LOOP-09). A *rule
  change* is an edit to
  this directory (a rule, its severity or check type, the README index), to
  the code that enforces the rules (`tools/tier1/`, `tools/workflow/check/`,
  and the jobs of `.github/workflows/` that run tier 1, `golangci-lint` or
  `tools/workflow`, and the `needs:` entries that name them), or to
  `docs/design/TEMPLATE.md`. The design lists each one
  in its `rule_changes` front-matter (FLOW-02) and its **Rule changes**
  section, with the current and proposed wording, severity and check, and what
  the enforcement change does. The design review approves each change by name
  (FLOW-19); Plan gives each approved change a task, and Implement makes it on
  the feature branch. A rule change found after the review is not cleared: the
  feature goes back to Design (FLOW-06) for a review of that change.

The automated part runs at the Implement, Review and Publish done-when
(FLOW-03): every file a feature branch changes under the rule-change paths
above must be in the `paths` of a `rule_changes` entry that the Design review
section records as `rule change <ID> approved at <sha>`, with that entry in the
design's front-matter at that commit; a change to `.claude/` or to
`tools/workflow/` outside `check/` fails outright. A process item's branch is
exempt. When a branch changes `tools/tier1/` or `tools/workflow/check/`, Review
and CI also run tier 1 built from the merge base's code with the branch's
configuration (`tools/tier1/rules.yaml` and `golangci.yml`) against the branch,
and every finding it reports that the branch's tier 1 no longer reports must be
on a rule an approved `rule_changes` entry names; Review also runs the merge
base's `wf check`. A configuration edit is an approved path that Review judges.
The agent part judges whether each edit matches the approved wording.

---

### FLOW-15 — Publish sweeps duplication, dead code and the overview

**Severity:** MUST
**Check:** automated + agent-judgment

FLOW-10 looks across features; this sweep looks inside the codebase the feature
leaves behind. Publish records its results in the design's consolidation
section, and a refresh (FLOW-10) runs it again on the branch with `main`
merged:

- **Dead code.** `make deadcode` reports code unreachable on Linux, macOS and
  Windows, tests included. A function unreachable on all three is removed, or kept
  with a stated reason: exported API of the library packages third-party
  orchestrators embed (`relay/`, `collector/`, `ledger/`, `relaycfg/`,
  `bootstrap/`), or cgo-only code the cross-platform run cannot see
  (`driver/vz`), which is checked on a macOS host instead.
- **Duplication.** Two implementations of one job are merged into one path: two
  translators, two CA installers, or one setup step done both on the host and in
  the executor. Where the copies have drifted, the merged path keeps the correct
  behavior and the design says which one that was.
- **The overview.** [`concepts/overview.md`](../book/src/concepts/overview.md)
  holds pseudocode for each core component. A change to a component's flow
  updates its pseudocode in the same change, so users and agents can read how
  BuildWarden works without reverse-engineering it.

---

### FLOW-16 — Merged branches are cleaned up

**Severity:** MUST
**Check:** automated + agent-judgment

When a feature's PR merges, the Close phase removes what the feature no
longer needs, with no PR and no human step:

- its branch in each repository it works in, on the remote and locally,
  and any worktree checked out on it. The host deletes a remote branch on
  merge where the repository is set to (on GitHub, "Automatically delete
  head branches"); Close confirms each is gone, and escalates to a
  `human` step with the exact command for each that is not;
- on each native machine, the worktrees and local branch its sessions made for
  the item, in each repository's checkout there: not by Close, which cannot
  reach them, but by the machine at its
  next scheduled run once the item is done or its branch is gone from
  that repository's
  remote, with `git worktree remove` and `git branch -d` only, reporting a
  worktree with uncommitted changes or a branch `-d` refuses instead of
  removing it;
- the branches it was stacked on, once they are merged too;
- the leftovers of its validation: test images and containers, downloaded
  media, scratch output, and anything created outside the documented cache
  locations.

Work a design names as continuing on the same branch starts again from trunk
under the same branch name, since the merge deletes the branch; its
implementation plan lists the remaining tasks. Follow-up work tracked anywhere
else (a
deviation, a deferred question, a validation left for another host) is not a
reason to keep a branch; it starts from trunk on a new one.

Deleting a branch loses any commit not on trunk, so a branch is deleted only
after it is shown to be contained in trunk (`git branch --merged`, the PR's
merge commit, or, for a squash merge, a commit on trunk naming the PR whose
tree equals the branch head's tree, or a one-parent such commit C whose
tree is exactly what merging the branch head onto C's parent writes, from
the single merge base B of the two:
`git merge-tree --write-tree --merge-base=B C^ <head>`), with git's safe forms
(`git branch -d`) and never by force-pushing. `git branch -d` refuses a
squash-merged branch, so that case alone deletes the local branch with
`git branch -D`, and only after `wf check` has shown the equal tree or
that merge; otherwise the escalation gives the human these commands, and
an agent never runs them. Caches a later build reuses (prepared images,
support images) stay. An agent without permission to delete lists the
exact commands for the human instead of skipping the step.

The automated part reports remote and local branches already merged into trunk
that no open design claims.

---

### FLOW-17 — Feature branches are pushed at checkpoints

**Severity:** MUST
**Check:** agent-judgment

An agent pushes its feature branch to the remote, without asking, whenever the
work reaches a successful checkpoint: the change is committed, and the
repository's fast gate is green (LOOP-01; here `make test` and `make lint`,
lint run for each platform whose build-tagged files the change touches). An
item that works in several repositories has a branch of the same name in
each, pushed to that repository at its own checkpoints. The pushed branch is
the backup and the hand-off point.

Opening the PR to its trunk or program trunk needs no confirmation either
(FLOW-08). Pushing never includes any repository's trunk itself, a
program trunk other than its creation at its trunk's head on the human's
word, a force-push, rewriting history that is already on the remote,
deleting a remote branch (FLOW-16), or a tag, except a pre-release tag or
a program trunk's final tag FLOW-20 lets the loop create, and a release
tag an attended session pushes on the human's word; no tag is moved or
deleted. The loop creates its tags only on a native machine: a cloud
session records each tag `wf tag-next` names as due, at the commit it
checked, and pushes none; the next native session on a machine that
lists the repository creates it, as an annotated tag at that commit,
through the loop's tag script on trunk, which only creates a due tag that
the remote does not have, and records it. An unattended session on a
native machine pushes a feature branch only through the loop's push
script on trunk, which pushes one branch of an item in progress,
fast-forward only, a due tag only through the loop's tag script, and the
queue state only through the loop's state commit; it runs no other
push.

---

### FLOW-18 — Agent sessions are recorded privately

**Severity:** MUST
**Check:** automated + agent-judgment

Agent sessions are recorded with the Entire Checkpoints CLI, which links each
session's prompts and transcript to the commits it produced. Recording serves
three uses: a daily run resumes the previous run's session instead of starting
cold (`entire session resume`), the Review phase and the human's design review and
merge read why a
change was made alongside the diff, and past sessions become searchable memory.
Every worker records its sessions this way, on whichever machine it runs. A
native machine whose session cannot be recorded takes no step of an item it
leads in that run, and its report says so.

The recordings go only to the private `buildwarden/buildwarden-sessions`
repository, named by the `checkpoint_remote` setting in `.entire/settings.json`.
They are never pushed to this public repository, and the CLI's temporary shadow
branches, which can hold unredacted data, are never pushed anywhere. Secret
redaction is best-effort, so a session does not handle secrets in its prompts.

Session memory is supplementary: the standards and the archived designs
(FLOW-12) are authoritative, and where they disagree with a recalled session, they
win.

The automated part checks that the public remote carries no `entire/` refs and
that the settings name the private remote; the agent part judges what a session
put into its prompts.

---

### FLOW-19 — Non-trivial designs get a design review

**Severity:** MUST
**Check:** automated + agent-judgment

After Design and before Plan, the human reviews a non-trivial design in one
interactive session with an agent (the `workflow-human` skill), not on a PR or
as a list of questions. The session goes over, at a high level:

- what changes and how: the components, seams and flows touched, before and
  after, with the `overview.md` pseudocode change where a flow changes;
- the repositories and areas it declares (FLOW-02), and the open items it
  is near or collides with, and why (FLOW-05); the human may narrow or
  widen the areas, name a collision the area map does not show, or have
  the map changed;
- the Validation plan and what Plan will work from: native validation steps,
  tier-2 gaps, and, on a re-review, the finding and the tasks it touches;
- the risks and what is deferred: native validation, tier-2 gaps, rules changed;
- the defaults, overrides and named fallbacks the design adds or changes
  (GLOBAL-09 to GLOBAL-11);
- each `rule_changes` entry (FLOW-14), shown with its current and proposed
  wording and answered on its own: approved, approved with a change, or
  rejected. Up to one question call's worth of entries may be shown
  together;
- each assumed question, confirmed or turned into a change (FLOW-07). The
  human may confirm, in one answer, every assumed question that is not
  flagged as recommended, naming any exceptions; each exception is then
  answered on its own. A question is flagged, and always answered on its
  own, when it touches a security boundary or a seam, or departs from
  prior art or from an earlier decision of the human's. For a program item
  (FLOW-20), a question is flagged only when it is component design: when
  it changes a seam in a way the program's design does not name for the
  item's phase, changes a security boundary's posture, a component's
  boundary or a contract, or departs from an earlier decision of the
  human's on component design.

It ends approved, or with changes that send the feature back to Design with the
human's notes as the finding. The design's **Design review** section records each
review: its date, its outcome, the decisions taken, and for an approval, the
commit of the design as approved, with any changes made in the sitting folded in
(`approved at <sha>`). Each approved rule change gets its own line, `rule change
<ID> approved at <sha>`; one "approved with a change" names that same commit. A
rejected rule change sends the feature back to Design (FLOW-06), unless the
review keeps the feature and the design drops the change.

Plan works from the approved design. A reason Plan finds to change it is a
finding for Design (FLOW-06), and the fresh review follows the delta rule below.
Otherwise there is no second review.

Design decides whether the review is needed and records it as
`design_review: required`, `skip`, or, while FLOW-20 stands, `umbrella` for
a program feature or `process-test` for a process item (FLOW-20), in the
front-matter and the queue. An `umbrella` review is taken by FLOW-20's
umbrella test; its line `umbrella test passed at <sha>` is an approval; a
failed component-design point makes it `required`, and any other failed
point returns the feature to Design (FLOW-06). A `process-test` review is
taken by FLOW-20's process test; its line `process test passed at <sha>` is
an approval; a failed or doubtful reserved-rule point makes it `required`.
`skip` is
allowed only when the design declares `alters_relationships: false`, no `seams`,
`security_boundary: none` and no blocking `native_validation`; raises no assumed
question; declares no `rule_changes`; declares no fewer repositories or
areas than intake recorded, where declaring areas when intake recorded
none counts as fewer; and the branch changes no rule in this directory.
Every skipped review is listed in the digest, so the human can call
one back. An archived design need not carry `design_review`: it is past the
gate, and designs archived before this rule never had the key.

When a feature returns to Design (FLOW-06), a fresh review follows the new
design. Whether it may be skipped is judged on what changed since the last
approval, not on the whole design: `skip` is allowed when the impact fields
above, and `repos` and `areas`, are unchanged from the design at its
approved commit, no question is open, and no rule changed since that
commit. A minimal change after an approval
needs no re-approval.

The automated part checks the recorded decision, the skip criteria and that each
review the queue seeded has an approval (a `rule change` line is not one): a
design-review step's completion and the Plan gate check its approval, and the
Plan gate fails on a change to an impact field, to `repos` or `areas`, or
to `rule_changes` since it. The agent part
judges whether the session covered the design.

---

### FLOW-20 — The componentization program is the north star

**Severity:** MUST
**Check:** automated + agent-judgment

The componentization program is the work set out in
[`componentization`](../design/componentization.md): the relay, the
drivers, the kit, the specs and the loop become separate components coupled
through written seams. A *program item* is a queue item whose design lists
that design in `related`. The program is the north star until it completes
(the human, 2026-11-21): work is prioritized to finish it, with as many of
its items in flight at once as the relations allow, even where an item
breaks an interface for a while, and as little of it as possible waits on
the human where the program's design already gives the answer. While this
rule stands:

- **Hold.** An item that is neither a program item nor a process item is
  held unless the human releases it; a fix for a security problem or for a
  regression is held like any other. Intake files it on hold, with a
  blocking question only the human answers that names when it resumes: the
  program's end, unless the human names another point. An item already
  queued or in progress that is neither is held the same way unless the
  human has released it, and keeps its branch and design; on resume it
  returns to Design for what the program changed (FLOW-06). Never held:
  the fix for a failing `main` or program trunk (FLOW-09, FLOW-10), and a
  fix that `follows` a program item. A held item holds no limit and does
  not wait on the human (FLOW-05). The human releases a held item by
  answering its question, or files one unheld at intake; marking a fix
  urgent does not release it.
- **Areas.** A program item names its repositories and areas (FLOW-02) at
  intake where the program's design gives them, and by the end of its
  Design at the latest, so that only real collisions keep it waiting
  (FLOW-05). Its design is scoped so that its review fits one sitting; a
  larger one is split at Design into items that can run side by side.
- **Loop work.** A process item (FLOW-03) carries a **Program payoff**
  section. It shows, with figures and how they were reached, either the
  human interactions it removes that decide nothing (a confirmation with
  one possible answer, a decision restated on every run, a step only the
  human may complete with nothing to judge), counted per week, or the
  program time it saves before the program ends against its own cost in
  steps and review. Its design review, or the process test's session,
  declines one that does neither, and the item is then held until the
  program's end, unless the human releases or closes it. The rules this
  rule *reserves to the human* are, read by a change's effect and not by
  the files it names: any change that widens the loop's own authority,
  that is, lets the loop take without the human a step, merge, push, tag,
  filing or decision it could not take before, removes or narrows a gate
  of the human's, loosens the umbrella test or the process test, relaxes
  or removes a check that stops the loop, or raises a limit, budget or
  permission of the loop's; and any change to a rule about security,
  merging to a trunk, pushing, or release tags without a suffix. A merge
  into a program trunk under FLOW-08's merge gate, and a pre-release or
  final tag this rule lets the loop create, are the loop's: a change to
  how it takes them is reserved only when it removes or narrows a point
  of the merge gate, lets the loop take two merges into one program trunk
  at once, lets a tag move or be deleted, or meets another clause above;
  a merge or tag the loop may already take is not one it could not take
  before, whatever worker or machinery takes it. A process item whose
  change touches none of
  them is settled by the *process test* in place of the human's review
  (LOOP-09, FLOW-14): its Program payoff section shows this bar is met;
  it touches none of the reserved rules; and a review session that did
  not write the design agrees on each point. A doubt about the second
  point sends the design to the human's review (FLOW-19); one whose
  payoff the session does not find shown is held until the program's end.
  The pass is recorded, and the item's merge to the trunk stays the
  human's (FLOW-08).
- **Interfaces.** A program item may break an interface for the program,
  and its design names each break:
  - the Go API of any package, the driver contract, and the internals of
    the drivers, relay, executor and collector (GLOBAL-08), within the
    BOOT rules and RELAY-08;
  - a seam's behaviour, by a new pre-1.0 release of its specification
    first (GLOBAL-08, LOOP-02), each implementation adopting it as LOOP-07
    sets out;
  - a CLI flag, subcommand, config key, environment variable or output
    form, with no alias or removal window (UX-03, ORCH-03), only where the
    program's change needs it; the CLI and config otherwise keep their
    shape (the human: "mostly similar"), and a rename for its own sake is
    not a program break. A form a tagged release exposed is refused with
    an error that names its replacement (UX-04), never silently ignored;
    one no release exposed may simply go. The reference docs change with
    it (UX-01), with a migration note for a released form. Where tier 1
    flags the removal, the commit carries a `Tier1-Allow` trailer whose
    reason names this rule (LINT-05).

  These never break: the ledger wire format (`specs/build-ledger/v1.md`)
  and its signatures, which change only additively; the reader's support
  for every ledger a release wrote; a schema `$id` already written
  (RELAY-05); and any security boundary (GLOBAL-06). If the program needs
  a new major version of the ledger format, it is developed as a *working
  draft* beside v1 on the program trunk, marked unreleased in the document
  and in the specifications index, and written by no build tagged from
  the program trunk; the design that opens it is component design, and it
  is finalized and released as that version only when the program lands.
  A program trunk may
  lack a capability for a while when the design that removes it names the
  gap and the item that restores it; the program trunk still passes its
  own checks, no such state opens a boundary, and no gap is open when it
  lands. Compatibility kept by trying the new interface and then the old
  one is a fallback (GLOBAL-10, GLOBAL-11).
- **Program trunk.** Each repository the program works in has a *program
  trunk*, `program/componentization`, which the loop's configuration
  records beside its trunk (LOOP-01). It is not a trunk, and it moves only
  by merged pull requests. A program item that is not a process item and
  changes none of the loop's machinery or the repository's CI and release
  configuration works against it wherever these
  rules name the trunk an item forks from, merges, is refreshed against,
  opens its pull request to, or is archived on. The loop merges such an
  item's pull request into it when FLOW-08's merge gate passes, and merges
  the trunk into it whenever the trunk moves past it, each as a merge
  commit, never a squash or a rebase. A process item, and any change to
  the loop's rules or machinery, goes to the trunk by the human's merge
  (LOOP-09).
- **Human gates.** A step waits on the human only when it is one of three
  kinds (the human, 2026-11-21): *setup*, the one-time setup of the
  program's repositories and infrastructure; *component design*, a
  question or review about a component's seams, boundaries, contracts or
  security posture, not about the strategy to implement it, and the
  design of a process item the process test may not settle (LOOP-09); and
  *privileged* work, which needs
  the human's privileges or configuration: a merge to a trunk, a change
  to CI or release configuration, a release without a suffix, making a
  repository public, the machines of the hosts table (LOOP-05), a native
  step only the human's attended machine can take, and a failure only
  those can fix. An escalation of a program item that does not converge,
  below, also reaches the human. The loop decides everything else, and
  records it:
  - A program feature's design review is taken by the *umbrella test*
    when every component-design point holds: the design implements the
    components, seams and boundaries the program's design names for one
    of its phases, or a filed split, follow-up or re-sequencing of them,
    within that phase's work; no blocking or flagged question is open; it
    declares no rule change (FLOW-14), and changes no loop machinery and
    no check that enforces a loop rule (LOOP-09); its
    `security_boundary` is `none`, or its only trip is GLOBAL-06's relay
    clause and its security note shows no boundary's posture changes;
    and each interface break is one this rule allows. A review session
    that did not write the design judges each point, and checks the
    item's repositories and areas against the area map and the
    repository's dependency edges; a failure of that check, or a
    disagreement on anything but a component-design point, returns the
    design to Design (FLOW-06). A design that fails a component-design
    point is reviewed by the human (FLOW-19).
  - On a program item, a question is blocking, or flagged, only when it
    is component design or needs setup or privileged work; any other
    question proceeds as assumed on Design's recommendation, and the
    umbrella test's session confirms it (FLOW-07, FLOW-19). A design
    follows the human's recorded decisions: one that departs from a
    decision on component design flags the question, and one that
    departs from a decision on strategy records the departure.
  - A program item that does not converge escalates later than other
    items: it escalates when it returns to Design more than four times
    (FLOW-06), and its first runaway budget raises its estimate to its
    spend so far, recorded, so that only its second escalates (FLOW-13).
    A CI failure Publish cannot repair escalates as FLOW-11 says. A program
    sync whose conflict the loop cannot resolve leaves the program trunk
    as it is, and escalates: the loop files a program item that resolves
    it, whose first step is the escalation.
  - A tier-2 MUST finding the loop may not defer (FLOW-08) returns the
    item to Design for a change that conforms; it reaches the human only
    to be dismissed, or when the posture or contract it protects would
    have to change.
  - The loop files a phase's items once the gate before its wave is met,
    a reconciliation item that adopts a pre-release on a program trunk
    (LOOP-07), and the relations between items that their designs name;
    and it changes an area map where a program item's design, reviewed or
    passed by the umbrella test, names the change, or where the change
    brings the map's `reaches` in line with the repository's dependency
    edges (FLOW-05). It places what it files (FLOW-05): a phase's items by
    the phase order of the program's design, and any other program item
    as intake places one, above every released item of its class that is
    not a program item; and it records each placement.
  - Releasing a hold, marking a fix urgent and the order of the queue are
    the human's word whenever given; no step waits on it.
  - Every repository on the program's list is created private in the
    setup. A repository not on it is a new component, and proposing one
    is component design; making one public is privileged work.
- **Records.** Each decision this rule lets the loop take in the human's
  place (a loop merge, a deferred tier-2 MUST finding, a protected one
  sent back to Design, an umbrella test, a process test, a security note
  read under the umbrella test, a question settled as assumed, a departure
  from a decision on strategy, a return to Design past the second, a
  re-estimate, a program sync's conflict resolution, a filed item or
  relation, an area-map change, a pre-release or final tag, a hold resumed at the program's
  end) is recorded in the
  workflow branch's program records, named in the run's digest, and listed
  again in the pull request that lands the program trunk. The human may
  call any back: a blocking question on an open item, or a follow-up
  program item for a merged one.
- **Pre-releases.** On a program trunk, the loop may tag a pre-release,
  `vX.Y.Z-dev.N` (`<directory>/vX.Y.Z-dev.N` for a specification) after a
  merge that changes a consumable, and `-rc.N` once the component's last
  phase has merged on its program trunk and its CI and conformance suites
  pass at the latest pre-releases of what it consumes, on a commit whose
  checks passed. Each is
  published as a pre-release that moves no `latest`, and is never moved
  or deleted. On a program trunk a pre-release is a release for LOOP-07; a
  trunk names only releases without a suffix. A release without a suffix
  is the human's, cut from the trunk through the full release gate
  (LINT-03).
- **Moves.** A program item whose change only moves code between packages,
  modules or repositories, with no change in behaviour, says so in its
  design. Where the move trips a path-coupling check, it carries a
  `Tier1-Allow` trailer whose reason names this rule (LINT-05). Review
  judges that the change is only a move.
- **Overview.** A program item may leave FLOW-15's overview sweep to the
  last item of its phase, which the phase's designs name.

Nothing else is relaxed. GLOBAL-06, GLOBAL-09 to GLOBAL-11, the BOOT
rules, the relay's ledger, key and claim rules (RELAY-01 to RELAY-03,
RELAY-05, RELAY-08), the SEC rules, UX-01, UX-04, FLOW-11, FLOW-18 and
the LOOP rules other than LOOP-07 and LOOP-09 apply to program items as
written; FLOW-06, FLOW-07, FLOW-08, FLOW-13, FLOW-14, FLOW-19, LOOP-07
and LOOP-09 apply as this rule reads them.

The program ends when every component works as intended when built from
its own repository, and the human has squash-merged each program trunk
into its trunk, or earlier by the human's word. A process item that the
loop holds until the program's last phase has merged lands it: its design
names one pull request per repository from the program trunk to the
trunk, whose body lists the program records and the evidence that each
component works when built from its own repository, and its own change
retires this rule. Before the landing pull requests open, the loop tags
each program trunk's head `program/componentization-final`, recorded, so
the per-item merge commits stay reachable; it is never moved or deleted.
Its Close resumes every item held until the program's
end, each returning to Design for what the program changed, and records
it; the human may close any of them instead.

The automated part: wf chooses an item's trunk, checks the queue's program
mark against the design's `related`, takes an umbrella-test or
process-test line as an approval, counts a held item toward no limit,
seeds a program item's escalations at the later points above, and merges
with a merge commit only; a test refuses a ledger working draft on a
pull request or push to the trunk. The agent part: intake and Design
judge whether an item is a program item and which kind a question is;
the design review, the umbrella test's session or the process test's
session judges a Program payoff, each interface break, each reserved
rule and each point of its test.

---

## Bootstrap order

Done so far: the windows-driver integration line and this doc set landed on
`main`; Hyper-V Phase 5a is complete (re-validation in
[`vm-driver-release-plan.md`](../design/vm-driver-release-plan.md), consolidation
in the archived [`hyperv-windows-prepare.md`](../design/hyperv-windows-prepare.md));
and the legacy guidance files (`CLAUDE.md`, `AGENTS.md`, `CONTRIBUTING.md`, the
mdbook `contributing/` tree) now point here instead of restating rules, with
`DEVELOPING.md` folded into `CONTRIBUTING.md` and UX-04 / UX-09.

Remaining, highest first:

1. **Stand up the workflow**
   ([`workflow-bootstrap.md`](../design/workflow-bootstrap.md)): the `workflow`
   orphan branch with the queue state and the native-gap backlog, the agent
   definitions under `.claude/` (FLOW-14), the daily loop (FLOW-13), session
   recording to the private sessions repository (FLOW-18), and
   behavioral-linter tier 2 in report-only mode (LINT-07).
2. **Normal queue**, in the priority order recorded on the `workflow` branch:
   the open items in the README, then feature work such as the Firecracker
   driver.
