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

Publish takes one feature at a time through the PR slot: it merges current
`main` and runs a **consolidation pass** against every feature that landed while
it was being built (FLOW-10, FLOW-15), and only sends the feature back when that
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

- `drivers` — which drivers are touched, or `interface` for a change made at the
  level of the driver contract or `driver.Session` (ORCH-01).
- `seams` — which versioned seams change (GLOBAL-08).
- `surfaces` — which user-facing surfaces change (UX-01, ORCH-03).
- `repos` — every repository the change works in, the one that holds the
  design first, when there is more than that one (LOOP-04).
- `areas` — the areas of the repository's area map whose files the change
  edits (FLOW-05). A design that leaves the key out is undeclared, as
  FLOW-05 defines it, and FLOW-05 then treats the item as colliding with
  every item in its repositories. `areas: []` is a declaration: the
  change edits nothing but its own design, though an item whose home
  repository has no map is undeclared whatever it declares (FLOW-05).
- `security_boundary` — which GLOBAL-06 boundary is altered, if any. Any value other
  than `none` requires a security note in the design.
- `alters_relationships` — `true` when the change alters a component's role or
  how components interact: a versioned seam (GLOBAL-08), the bootstrap model
  (BOOT rules), which component owns a responsibility (relay, collector,
  executor, orchestrator, a driver), or the driver contract or
  `driver.Session`. Changes inside one component's existing role are `false`.
  FLOW-05 limits how many such features are in flight.
- `native_validation` — which native hosts must validate the change, and whether
  each blocks merge (FLOW-09).
- `tier2_gaps` — platform validation knowingly deferred (TEST-03).
- `rules` — the rule IDs the design relies on or changes.
- `rule_changes` — each rule change the feature makes (FLOW-14), as
  `{rule, change: add | amend | retire, paths}`, `paths` naming every file it
  edits, enforcement included. Empty or absent means none.
- `concepts` — retrieval tags for the archive (FLOW-12).

Declarations are derived conservatively: a change to a shared package (`relay/`,
`ledger/`, `collector/`, `driver/`) declares every dependent driver and host, per
the TEST-02 caveat. `areas` name what the change edits, not what depends on
it: the area map says which area reaches which.

The automated part diffs the declared `drivers` / `seams` / `surfaces` against the
trees and files the change reaches. It checks the declared `areas` against
what `drivers`, `surfaces` and the `rule_changes` paths imply, at Design,
and against the paths the change edits, from Implement through Publish
and at each refresh of a feature (FLOW-03). The agent part judges the
security note.

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
| 3 | Implement | Tasks done; `make lint` and `make test` green; reference docs updated in the same change (UX-01); every edit to the rules or their enforcement cleared by an approved rule change (FLOW-14). |
| 4 | Review | Behavioral-linter tier 1 green; tier-2 report produced for the diff (LINT-04); the rule-change clearance still passes (FLOW-14). |
| 5 | Guide | User-facing narrative docs (guides, examples) written or updated. |
| 6 | Publish | Once the PR slot is free (FLOW-05): current `main` merged; consolidation section filled (FLOW-10, FLOW-15); the rule-change clearance still passes (FLOW-14); design archived with its summary on the branch (FLOW-12); PR opened (no confirmation needed, FLOW-08) with CI green (FLOW-11); blocking native validation done (FLOW-09). |
| 7 | Merge | The human merged the PR (FLOW-08). The loop confirms it; there is no step for the human to claim. The PR is ready for the merge only while its branch contains `main` (FLOW-08); when `main` moves past it, a refresh step brings it up to date (FLOW-10). |
| 8 | Close | Merged branches and validation leftovers cleaned up (FLOW-16); the design is archived on `main`. |

A transition happens only when the phase's done-when checks pass mechanically
(files present, sections filled, commands green), never on an agent's own report
that it is done. From Plan on, each done-when also checks that the queue
still records what the design declares; from Implement on, that every
path the branch changes lies in an area the design declares (FLOW-02). A
path outside them is settled by widening the declaration inside the
approved design, or by a return to Design (FLOW-06).

A **process item** (queue `kind: process`) changes the rules the loops run
under: `docs/development/`, `.claude/` or `tools/workflow/`. Its design is the
human's, settled in a design review (FLOW-19); after that the loop works on its
branch, and nothing it changes reaches the running loop until the human merges
it (FLOW-14). A feature may also change the rules of a component it works in, and
their enforcement, never a loop rule (LOOP-09), when its design review
approves each change by name (FLOW-14); `.claude/` and the
rest of `tools/workflow/` change only through a process item:

| # | Phase | Done when |
|---|-------|-----------|
| 1 | Design | The design document exists on the branch and its design review is recorded as approved. |
| 2 | Implement | The branch is pushed (FLOW-17), the design archived on it with its summary, and its PR open with CI green. |
| 3 | Merge | A human merged it into `main` (FLOW-08). Its PR is ready and is refreshed as a feature's is (FLOW-08, FLOW-10). |
| 4 | Close | As phase 8. |

A feature whose home is a repository in which the loop's gates do not
run is **tracked**. The queue records it, its repository and its pull
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
- `human` — answering blocking questions, the design review (FLOW-19), and
  escalations: a runaway budget (FLOW-13), CI repair exhausted (FLOW-11), a third
  return to Design (FLOW-06), a branch that did not delete on merge (FLOW-16).
  Also the steps of work in a repository where the loop's gates do not
  run: a tracked item's steps (FLOW-03), and a step that records the
  human's merge there (FLOW-08).
- `host:<name>` — work that needs native hardware, for example `host:macos-vz`
  (the macOS/arm64 laptop) or `host:windows-hyperv` (the Windows desktop). A
  name `<os>-<driver>` is specific to that platform; `<capability>-any` (such as
  `qemu-any` or `container-any`) runs on any native machine that has the
  capability. The workflow branch's `hosts.yaml` gives each host name to exactly
  one machine.

A worker takes only steps whose affinity it satisfies. A step is offered when
every step it needs is done and, unless it is `human`, its feature has reached
the step's phase; no worker may claim or complete a host or cloud step of a
later phase. A step is claimed with a
lease (claimant plus expiry) recorded on the `workflow` branch, and only
when FLOW-05's limits allow the claim, whoever the worker is. When the
push of a claim is rejected because the branch moved, the claim is asked
again on the new state: it stands if the step may still be claimed
there, and otherwise it is lost and the worker re-reads state. An
expired lease may be reclaimed.

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

While a process item is in progress, a worker matching one of its `holds`
(affinity patterns such as `cloud` or `host:*`) takes no new step, whatever
the class of the item the step belongs to, so disruptive work waits for the
rules to settle; a step it already claimed stays claimed, and finishing the
process item releases the hold. A hold names workers and covers work in
every repository. Otherwise a worker takes the next ready step
of the first item in that order, and keeps taking that item's steps until it
blocks: on a `human` or `host:<name>` step, an exhausted budget, a limit
below, or the human's merge (FLOW-08). Only then does it take steps from the
next item down, and it returns to the earlier item as soon as that one has a
ready step again.

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
word: no agent changes it otherwise. Two items cannot both change an
*exclusive* area; any number may change an *ordered* area, and the
later to merge is put in order by its refresh (FLOW-10). Area A *reaches* area B when code in A
depends on code in B; only exclusive areas reach and are reached. A
repository with no map is one exclusive area, and
an item that declares no areas is taken to change the whole of each
repository it names. Two features are:

- **colliding** when they share an exclusive area; when they have a
  repository in common and either declares no areas; when both change a
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
`holds`.

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
the merge, or a spent budget (FLOW-13); it counts once, and a tracked
item does not count. This limit counts by no relation and stops only a
start: work in progress goes on, so the count can pass it. A fix that
`follows` an item, the fix for a failing `main` (FLOW-09), a security
fix the human has marked urgent, a process
item and a tracked item start whatever
the count. Only the human marks a security fix urgent; the mark lifts
this limit for it, and nothing else.

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
`human` step instead of looping again. When Publish's consolidation finds work
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
  a `human` step; the worker moves on to other work.
- **Assumed** — the design proceeds on a stated default, recorded in the design as
  "proceeding with X; confirm". Assumptions are confirmed in the design review
  (FLOW-19), and must all be confirmed before planning starts.

Every question states the options considered and a recommended answer. Later
phases route questions back through Design, which escalates only what the design
has not already settled. Questions live in the feature's queue state on the
`workflow` branch and in the loop's digest, not as GitHub issues.

---

### FLOW-08 — Only a human merges to trunk

**Severity:** MUST
**Check:** automated

This is LINT-03 applied to the process. No agent approves or merges a PR to `main`,
and auto-merge is not enabled by agents. The human's merge is the sign-off: it
covers the PR, the archived design it carries, and the tier-2 behavioral report,
whose MUST findings the PR body lists. The human acknowledges them by merging, or
dismisses one with a recorded reason in a PR comment. Changes that declare a
`security_boundary` additionally need the security note acknowledged (GLOBAL-06).
There is no sign-off step in the queue for a pull request the loop can
see: the loop sees the merge (FLOW-03, phase 7). For a pull request in a
repository where the loop's gates do not run, a `human` step records the
merge (FLOW-04). The merge is still the sign-off; the step is only the
queue's record of it.

A pull request is ready for the human's merge only while its branch
contains trunk and its checks have passed on that head. Each merge to
trunk therefore leaves every other open pull request waiting for a
refresh (FLOW-10), and the loop lists none of them as ready until it
has had one. Where the trunk's host can refuse a merge that is not
ready, the human has it do so (LOOP-08). That setting is the human's,
as the merge is: no agent changes a trunk's protection. A merge the
human makes all the same is still the sign-off.

Neither pushing a feature branch (FLOW-17) nor opening its PR to `main` is a
trunk change, and neither needs confirmation. An agent opens the PR once the work
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
host step it serves is ready, or when `main` or a waiting branch has moved since
its last sweep of open backlog entries; it is bound by the unattended loop's limits
(FLOW-13) and records what it did in the digest. Every scheduled run, including
one that stops before an agent starts, also records the machine's outcome and a
one-line reason in `native/status/<machine>.yaml` on the `workflow` branch, so a
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

Work whose design or implementation itself needs the native host (for example a
platform or hypervisor update) carries `host:<name>` affinity on those phases too.

---

### FLOW-10 — Publish consolidates

**Severity:** MUST
**Check:** agent-judgment

Publish, after merging current `main`, reviews the feature against everything
that landed on `main` since its branch forked, and records in the design's
consolidation section:

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
branch contains `main` (FLOW-08). Whenever `main`
has moved past its branch, a **refresh** step merges `main` into the
branch again, runs the gates, and repeats this review, and FLOW-15's
sweep, over what landed since the branch last merged `main`, as a dated
entry in the consolidation section. The PR is ready again when the
step's checks pass. It pushes only while the pull
request is still open. A feature that has merged `main` in Publish and
has not finished Publish is refreshed the same way when `main` moves,
before its PR is opened or listed as ready. No refresh is taken while
`main` is known to
fail its own checks, except for the item that fixes it (FLOW-09): it
would fail on the same failure. A refresh that finds work beyond
conflict fixes
and wording sends the feature back as above; its pull request is then a
draft until the feature is published again, and it keeps the PR slot
(FLOW-05). A process item's open PR is refreshed the same way, without
the review.

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

The unattended loop runs once a day. Each run takes steps one at a time (read
state, claim, work, write state) until no `cloud` step is ready or the run's
budget is spent, and writes state after every step so an interrupted run loses at
most the step in progress. All state it needs lives in the repository, never in a
long-running conversation. A run spends its budget in FLOW-05 order, so the
first item in FLOW-05's order can use all of it. A scheduled native session (FLOW-09)
spends its own budget, `native_budget`, which falls back to the run's budget when
unset, so native and integration testing never compete with the cloud loop. Each
feature also has a work budget, which is a runaway check rather than a share of
the run: an open (queued or in-progress) item that spends `runaway_factor` times
its estimated steps escalates to a `human` step, and the unattended loop takes no
more of its steps. Runs are counted but never trip it. The loop posts a periodic
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
  review (FLOW-19) before the loop implements it on its branch. It is the only
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

- its branch, on the remote and locally, and any worktree checked out on it.
  GitHub deletes the remote branch on merge (the repository's "Automatically
  delete head branches" setting); Close confirms it is gone, and escalates to a
  `human` step with the exact command if it is not;
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
tree equals the branch head's tree, or a one-parent such commit whose change
from its parent has the same `git patch-id --verbatim` as the branch head's
change from its merge base with that parent), with git's safe forms
(`git branch -d`) and never by force-pushing. `git branch -d` refuses a
squash-merged branch, so that case alone deletes the local branch with
`git branch -D`, and only after `wf check` has shown the equal tree or change;
otherwise the escalation gives the human these commands, and an agent never
runs them. Caches a later build reuses (prepared images, support images)
stay. An agent without permission to delete lists the exact commands for the
human instead of skipping the step.

The automated part reports remote and local branches already merged into trunk
that no open design claims.

---

### FLOW-17 — Feature branches are pushed at checkpoints

**Severity:** MUST
**Check:** agent-judgment

An agent pushes its feature branch to the remote, without asking, whenever the
work reaches a successful checkpoint: the change is committed, and `make test`
and `make lint` are green (lint run for each platform whose build-tagged files
the change touches). The pushed branch is the backup and the hand-off point.

Opening the PR to `main` needs no confirmation either (FLOW-08). Pushing
never includes `main` itself, a force-push, rewriting history that is already on
the remote, or deleting a remote branch (FLOW-16).

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
  wording and asked on its own: approved, approved with a change, or rejected;
- each assumed question, confirmed or turned into a change (FLOW-07).

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
`design_review: required` or `skip` in the front-matter and the queue. `skip` is
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

### FLOW-20 — The componentization program has the queue to itself

**Severity:** MUST
**Check:** agent-judgment

The componentization program is the work set out in
[`componentization`](../design/componentization.md): the relay, the
drivers, the kit, the specs and the loop become separate components coupled
through written seams. A *program item* is a queue item whose design lists
that design in `related`. While this rule stands:

- **Freeze.** Intake files a new item that is not a program item on hold,
  with a blocking question only the human answers, so the loop takes none
  of its steps. Two kinds of item are exempt: a fix for a regression on
  `main` (FLOW-09) and a fix for a security problem.
- **Moves.** A program item whose change only moves code between packages,
  modules or repositories, with no change in behaviour, says so in its
  design. Where the move trips a path-coupling check, it carries a
  `Tier1-Allow` trailer whose reason names this rule (LINT-05). Review
  judges that the change is only a move.
- **Overview.** A program item may leave FLOW-15's overview sweep to the
  last item of its phase, which the phase's designs name.

Nothing else is relaxed. GLOBAL-06, the BOOT rules, the relay's ledger and
key rules (RELAY-01 to RELAY-03), FLOW-08, FLOW-14, FLOW-18 and FLOW-19
apply to program items as written.

The human ends the program with a process item that retires this rule.

The agent part: intake and the design review judge whether an item is a
program item or exempt.

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
