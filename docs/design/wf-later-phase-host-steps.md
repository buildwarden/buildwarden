---
feature: wf-later-phase-host-steps
status: draft
drivers: []
seams: []
surfaces: []
security_boundary: none
alters_relationships: false
design_review: required
native_validation: []
tier2_gaps: []
rules: [FLOW-03, FLOW-04, FLOW-05, FLOW-06, FLOW-09, FLOW-14, FLOW-19, TEST-01, TEST-02]
rule_changes: []
concepts: [workflow, queue, native-validation, wf-back, wf-gate, step-readiness, process-item]
related: [workflow-streamline.md, native-host-loop.md]
---

# Later-phase steps wait for their phase and survive `wf back`

## Problem

Features carry steps for phases they have not reached yet. wf-design step 3
(`.claude/agents/wf-design.md` lines 25-28) adds a `f6-validate-<host>` step
for each blocking native host (FLOW-09). In practice Plan sessions have done
the adding: build-link-subnet's four were written at `f2-plan` (`1f62e7f`),
`f2-plan-r2` (`504cf53`) and `f2-plan-r3` (`da097dc`), although wf-plan has no
instruction to add queue steps. Native sessions add more: firecracker-driver's
human step `f6-provision-linux-kvm-dns` was added in phase 4 by
`host:linux-kvm/20261102T231002Z`. `wf` handles these steps badly in two ways,
and in a third that follows from them.

1. **They are offered before their phase.** `ready()`
   (`tools/workflow/queue/next.go` lines 117-151) skips a step only when it
   is not pending or its `needs` are not done (line 127). It never compares
   the step's phase with the feature's. `Gate` (`tools/workflow/queue/hosts.go`
   line 200) calls `Next`, so a pending phase-6 host step with no `needs`
   wakes its machine from the moment it is written. QUEUE.md decides
   readiness separately, in `readySteps()` (`summary.go` lines 73-90), with
   the same gap: the golden `testdata/queue.md` line 18 lists
   `f6-validate-windows-hyperv` as a next step of a phase-2 item.
   - On 2026-11-01, `wf gate` woke windows-desktop for firecracker-driver
     `f6-validate-linux-kvm` while the feature was in phase 2 and its branch
     was still design-only (digest 2026-11-01, run
     `host:windows-desktop/20261101T111001Z`). The run proposed this fix and
     stopped the step with a hand-added need on `f2-spike-linux-kvm`
     (workflow `e3eb12c`).
   - Since then every such step has carried hand-added `needs` to keep the
     machines asleep. build-link-subnet's four `f6-validate-*` steps were given
     `f2-design-review` (`1f62e7f`), then `f2-design-review-r2` (`504cf53`),
     then `f3-implement` (`be6657b`). The 2026-11-04 digest told Jeff to add
     `f3-implement` to their `needs` when he marked the review done,
     "Otherwise the native machines will be offered them." The steps' own
     notes repeat the instruction ("add f3-implement to its needs once
     Implement is seeded").
2. **`wf back` deletes them.** `Back` (`tools/workflow/queue/phases.go` lines
   155-188) keeps only steps that are done or in a phase before the target
   (lines 165-171). Every pending step of a later phase goes, along with its
   note. Phase entry seeds only template steps (`Seed`, lines 121-142), so
   nothing brings them back.
   - firecracker-driver, `f6-consolidate` back to Implement on 2026-11-03:
     `f6-validate-linux-kvm` and `f6-provision-linux-kvm-dns` were restored by
     hand (workflow `7616804`).
   - build-link-subnet, `back -to 3` on 2026-11-05: the four `f6-validate-*`
     steps were restored from git with `f3-implement-r2` added to their
     `needs` (workflow `ab54c55`; digest 2026-11-04, attended section).
   - Each return to Design drops them as well, and the next Plan re-added
     them (`504cf53`, `da097dc`).
3. **The Publish gate does not wait for them.** wf-design adds each
   validation as a step "that `f6-open-pr` needs". `f6-open-pr`, however, is
   only created when phase 6 is seeded, and its template `needs` are just
   `f6-consolidate` (`phases.go` lines 49-50). That link has been added by hand
   every time: firecracker-driver `c9c9212` and build-link-subnet `007616b`.
   The phase-6 done-when check (`tools/workflow/check/check.go` lines 65-75)
   does not look at native validation. So without the hand edit, the PR opens
   before the blocking validation runs, even though FLOW-03 lists "blocking
   native validation done" for Publish.

## Prior art

- **FLOW-03:** a phase's gate is its last template step, and a phase ends
  once all of its steps are done (`Complete`, `phases.go` lines 193-212).
  Publish is done only when blocking native validation is done, after
  `main` is merged and consolidation is filled.
- **FLOW-04:** a worker takes only steps whose affinity it satisfies. The
  rule says nothing about phase.
- **FLOW-06:** returns to Design or Implement. `Back` implements both.
- **FLOW-09:** a blocking host makes the feature "wait in Publish on a
  `host:<name>` step". The scheduled session starts an agent only "when a
  host step it serves is ready".
- **workflow-streamline** (archived): introduced the single round, the
  phase-6 table with `f6-validate-<host>` (line 163) and `wf back -to 3`. It
  did not say what a return does to steps added for later phases.
- **native-host-loop** (archived): `wf gate` is `Next` for each of the
  machine's hosts. Fixing readiness in `ready()` therefore fixes the gate as
  well.
- **flow19-design-before-plan** (queued process item, priority 26, ahead of
  this one; no design on `main` yet, so not in `related`): moves the design
  review to right after Design. It changes the phase-1 and phase-2 templates in `phases.go`. This design reads the
  templates through `isTemplate` and the Publish flag (Decision 4), so it
  holds under either order; the tests are written against the templates
  current when x2-implement runs.
- **The queue schema** (workflow branch `README.md`, "Steps and phases"):
  "Design adds the steps it finds: ... `f6-validate-<host>`". The schema has
  no field marking a step as added, and this design does not add one
  (Decision 2).

## Decisions

1. **A host or cloud step is offered only once its feature has reached the
   step's phase.** One predicate, `Offered(f, st)`, holds when
   `st.Phase <= f.Phase` or the step is `human`. It is used by `ready()`
   (`next.go` line 127), by `leasedTo()` (`next.go` lines 49-58), and by
   QUEUE.md's `readySteps()` (`summary.go` lines 73-90), which feeds both
   "Needs Jeff" and the Active "Next steps" column. A new `Early(f, st)
   error` in the queue package returns the refusal, and `wf claim`
   (`claimable`, `cmd/wf/main.go` lines 257-271) and `wf done` (the `done`
   branch, lines 244-249) call it. The message names the way out: "`<id>` is a
   phase `<n>` step; `<feature>` is in phase `<m>` (FLOW-04); release it with
   the evidence". Process items number their steps with their own phases, so
   the comparison works for them too. Hand edits to the queue file are not
   affected.
   - **Human steps stay offered early.** Jeff can provision for a later phase
     ahead of time, as with `f6-provision-linux-kvm-dns`, and the step shows
     under "Needs Jeff" as soon as it exists. A human step never wakes a
     machine.
   - **Rejected: gate every affinity.** A provisioning step would appear, and
     block, only in Publish, after the machines that need it have waited.
   - **Rejected: a gate on a named step** (for example, ready only once
     `f3-implement` is done). That is the hand-added `needs` written into
     code. A return makes the named step's id stale, as `ab54c55` showed.
   - **Rejected: offer host steps from phase 4 or 5, so native machines start
     sooner.** The branch still changes after that. FLOW-09 says the feature
     waits *in Publish*.
2. **`wf back` keeps the steps added for later phases.** A step is a
   *template step* if its name, with any `-r<n>` suffix removed, is one of
   its phase's template names (the same test `IsGate` makes, `phases.go`
   lines 100-118). `Back` still keeps every done step and every step of a
   phase before the target. It still drops every pending step of the target
   phase, and every pending template step of later phases, since those are
   seeded again on re-entry. What changes: pending non-template steps of
   later phases are kept, whoever added them. Back also:
   - clears each kept step's lease and logs it, because the work it covered
     is going to change. A native session still running such a step then
     gets Decision 1's refusal from `wf done`, and releases the step with its
     evidence instead (Implementation plan task 9);
   - removes from every kept step's `needs` the ids it dropped, so `wf
     validate` ("needs unknown step", `queue.go` line 381) stays green and no
     step waits on an id that no longer exists.

   A kept repair step, such as firecracker-driver's DNS provisioning step, is
   then a pending non-template step of Publish, so re-entering Publish links
   it to the new gate (Decision 4). That is intended: validation needs it
   anyway. On a return to Design, Design reconciles the kept steps with the
   new `native_validation` (Decision 5).

   There is no data-model change, because the template is the record of
   which steps are template steps.
   - **Rejected: keep the pending steps and re-point their `needs` at the new
     round's `f3-implement-r<n>`.** With Decision 1 those `needs` do nothing.
     Re-pointing them would carry the hand workaround into code.
   - **Rejected: reseed them from the design's `native_validation`.** It would
     throw away the notes on each step (build-link-subnet's list the cases to
     run). It would also make `wf` parse the design, and it would not cover
     human steps such as the DNS one.
   - **Rejected: an `added_by` or `origin` field on Step.** The template
     already says the same thing, and the field would need a migration for
     every open item.
   - **Rejected: keep a held lease across the return.** The session's `wf
     done` would then record a validation of a branch that is about to change.
3. **A Publish host step already done is queued again by a return.** This is
   an addition beyond the two reported defects; no incident has hit it yet.
   The case it covers: two hosts validate in Publish, one passes, the other
   fails and returns the feature. The passed validation would otherwise
   satisfy the next round's `f6-open-pr` on a branch that has changed. `Back`
   groups the phase-6 host steps by base name (the id with `retryRe`
   stripped). For each base name with a done step and no pending step, it
   appends one pending copy, `<base>-r<n>` with `n` one past the base name's
   highest suffix (2 when there is none), with the same affinity and the
   latest step's note, and logs it. A done step in another phase (a phase-2
   spike) stays done, and Design decides whether to redo it.
   - **Rejected: leave done validations done.** A stale validation would
     satisfy the new gate.
   - **Rejected: flip the done step back to pending.** Done steps are
     history (`Complete`, and the `-r<n>` convention in `Seed`).
4. **Seeding Publish links its added steps both ways.** The `open-pr`
   template carries a flag, `waitsAdded`. When `Seed` seeds a phase whose
   gate has it, the gate needs every pending non-template step of that phase
   already in the queue, and each such `host:*` step needs the phase's
   `consolidate` step (`f6-consolidate[-r<n>]`). So the PR waits for
   blocking validation (FLOW-03), and validation runs on the branch after
   `main` is merged and consolidation is done, which is the branch the PR
   opens on. Human steps get no extra need, so provisioning can still be done
   early. Steps added after Publish is seeded (a FLOW-11 repair step) are not
   linked, so the gate cannot deadlock on them; a session that adds one adds
   the link it needs, as today. No other phase and no process-item template
   has the flag.
   - **Rejected: link the gate of every phase.** Phase 2's gate is
     `f2-design-review`. firecracker-driver's `f2-spike-linux-kvm` (phase 2,
     `host:linux-kvm`) would have held the review, done on 2026-11-01, until
     the spike ran on 2026-11-02, reordering FLOW-19.
   - **Rejected: offer validation from the start of Publish.** It would run
     before `f6-merge-main` (which waits for the PR slot, FLOW-05) and
     `f6-consolidate`, both of which change the branch, and a validation
     done then would still satisfy `f6-open-pr`.
   - **Rejected: a phase-6 done-when check that every `f6-validate-*` step is
     done.** The gate would run first, open the PR, and only then fail its
     check. The `needs` link stops it being claimed at all.
5. **Design owns the later-phase steps, and reconciles them on a return.**
   wf-design step 3 adds each `f6-validate-<host>` with affinity
   `host:<host>`, its cases in the note, and no `needs`. On
   `f1-design-r<n>` it reconciles the steps Back kept against the new
   `native_validation`: keep an existing step and update its note, add only
   hosts that are missing, and remove a step for a host that is now tracked.
   It does the same for kept steps of phases 2-5, such as a spike the new
   design no longer needs. A re-added id would repeat, which `wf validate`
   already rejects (`queue.go` line 364). wf-plan may refine a kept step's
   note with the cases its plan assigns, but does not add or remove such
   steps; a missing blocking host is a design gap (FLOW-06).
   - **Rejected: let Plan keep adding them.** Plan has no rule for it, and
     every return so far has re-added them at Plan.
6. **The workarounds go, from new work only.** The tick and human-session
   digests stop asking for `needs` to be added by hand. In-flight queue files
   are not edited. build-link-subnet (phase 6, `f6-merge-main` and
   `f6-consolidate` done) has four pending `f6-validate-*` steps whose
   `needs` are all done, and an `f6-open-pr` that already needs them. Under
   Decision 1 those steps are offered exactly as they are today. Their extra
   `needs` are harmless, and the hand-edit instructions in their notes become
   stale but harmless. This item holds no workers (its queue file has no
   `holds`), so native validation of build-link-subnet continues while it is
   built.
   - **Rejected: strip the redundant `needs` and notes from
     build-link-subnet.** It is a queue edit with no effect, on a feature
     that is about to be validated.
7. **The rules say it once.** FLOW-04 gains the readiness rule. FLOW-06 gains
   what a return does to later-phase steps. FLOW-09 says the Publish gate
   needs the blocking host steps and that they run after consolidation. As a
   process item this change edits `docs/development/process.md`, `.claude/`
   and `tools/workflow/` directly on its branch after the review (FLOW-03,
   FLOW-14 as amended by #96). The `rule_changes` route is for features, and
   FLOW-14 exempts a process item's branch from the clearance. So
   `rule_changes` is `[]`, and the wording is approved here as part of the
   design.

## Questions

### Blocking

None.

### Assumed

- **a1. Readiness scope** (Decision 1). Proceeding with host and cloud steps
  of a later phase neither offered nor claimable nor completable, and human
  steps offered early. Options:
  - (a) as stated;
  - (b) every affinity gated, human steps included;
  - (c) `ready()` and QUEUE.md only, with `wf claim`/`wf done` left as they
    are.

  Recommended: (a).
- **a2. What `wf back` keeps** (Decision 2). Proceeding with pending
  non-template steps of later phases kept, their leases cleared, and dropped
  ids pruned from `needs`. Options:
  - (a) as stated;
  - (b) keep only pending `host:*` steps of later phases;
  - (c) keep them and re-point their `needs` at the new round's step;
  - (d) reseed validation steps from the design's `native_validation`.

  Recommended: (a).
- **a3. Validation already done** (Decision 3). An addition with no incident
  behind it; Jeff may defer it. Proceeding with one fresh pending copy per
  done phase-6 host step's base name on any return. Options:
  - (a) as stated;
  - (b) defer it: leave done validations done, and Design re-adds one if
    needed;
  - (c) a fresh copy of every done later-phase host step, spikes included.

  Recommended: (a).
- **a4. Gate linking** (Decision 4). Proceeding with a `waitsAdded` flag on
  `open-pr` only: the gate needs the phase's pending added steps, and their
  `host:*` steps need `consolidate`. Options:
  - (a) as stated;
  - (b) as (a), without the `consolidate` need, so validation starts with
    Publish;
  - (c) link only `<x><n>-validate-<host>` steps to the gate;
  - (d) no linking; keep wf-design's instruction and the hand edit.

  Recommended: (a).
- **a5. Who adds later-phase steps** (Decision 5). Proceeding with Design
  adding and reconciling them, and Plan refining notes only. Options:
  - (a) as stated;
  - (b) Plan adds them, since it writes the cases.

  Recommended: (a).
- **a6. In-flight items** (Decision 6). Proceeding with no edit to
  build-link-subnet or any other open queue file. Options:
  - (a) as stated;
  - (b) strip build-link-subnet's redundant `needs` and notes at
    x2-implement.

  Recommended: (a).

## Design

**Readiness** (`next.go`). The skip condition in `ready()` becomes:

```go
if st.Status != "pending" || !Offered(f, st) || !allDone(st.Needs, done) {
	continue
}
```

A skipped later-phase step adds nothing to `waits`; the feature's
current-phase steps already explain why it is blocked. `leasedTo()` and
`readySteps()` apply `Offered` too. `Early(f, st)` returns nil when
`Offered` holds and the refusal otherwise; `claimable` and the `done` branch
in `main.go` return its error.

**Template steps** (`phases.go`). `isTemplate(f, st) bool` parses the id with
`stepIDRe`, strips `retryRe`, and looks the name up in `templates[round][phase]`.
`IsGate` uses the same parse. `template` gains `waitsAdded bool`, set on the
feature round's `open-pr`.

**Back**, in order:

1. Keep a step if it is done, or its phase is before `to`, or (pending,
   phase after `to`, and not a template step). Record the ids dropped.
2. For each kept pending step of a later phase, clear the lease and log
   `kept <id> pending for phase <n>`. Remove the dropped ids from the `needs`
   of every kept step.
3. Group the phase-6 `host:*` steps by base name. For each group with a done
   step and no pending step, append one pending copy as in Decision 3, with
   no `needs`, and log it.
4. Set the phase, log `back to ...`, and `Seed`, as today. The escalation
   code after it still finds the seeded `f1-design-r<n>` as the last step,
   because the kept and copied steps are appended before `Seed`.

**Seed.** Before appending the template steps, collect the phase's pending
non-template steps. If the phase's gate template has `waitsAdded`, add those
ids to the gate's `needs` in queue order, and add the seeded `consolidate` id
to each collected `host:*` step's `needs`.

**QUEUE.md** (`summary.go`). `readySteps()` uses `Offered`, so a phase-6 host
step no longer shows as a next step in phase 2. `hostSteps()` (lines 175-190)
still lists every pending host step, with the suffix `(from phase <n>)` on
one whose phase the feature has not reached, so Jeff and the native sessions
can see it exists but is not yet offered.

**Flow on build-link-subnet as it would now run.** Design adds the four
`f6-validate-*` steps with notes and no `needs`. None is offered in phases
1-5. A return to Implement keeps them and their notes; a return to Design
keeps them for Design to reconcile. Entering phase 6 seeds `f6-open-pr`
needing `f6-consolidate` and the four, and the four needing
`f6-consolidate`. They are offered once consolidation is done. If one fails
and returns the feature, each one already done is queued again.

## Cross-driver impact

None. No driver or product code changes. Every native machine sees the
change only through `wf gate` and `wf next`, which offer host steps later.

## Security note

`security_boundary: none`. The change is to the queue tool's scheduling on the
workflow branch. It touches no build environment, relay, ledger or collector.
It also widens nothing an agent may do. Decision 1 narrows what a worker can
claim or complete, and Decision 4 adds waits before validation and before a
PR opens.

## Validation plan

- **Tier 0, `tools/workflow/queue`** (TEST-01, TEST-02):
  - `ready()`: a pending phase-6 host step with no `needs` is not offered
    while the feature is in phases 2-5, and is offered once it is in phase 6.
    A pending phase-6 human step is offered in phase 2.
  - `Gate`: in `hosts_test.go`, a machine is not woken by a later-phase host
    step, and is woken once the feature reaches that phase. This is the
    2026-11-01 case.
  - `Early`: nil for a step of the current or an earlier phase and for a
    later-phase human step; the FLOW-04 refusal for a later-phase host step.
  - `leasedTo`: a held worker's lease on a later-phase host step is not
    returned.
  - `Back(-to 3)` from phase 6, with four pending `f6-validate-*` steps that
    have notes and a lease, and a pending human repair step: all five are
    kept, with their notes, without the lease, and without any dropped id in
    `needs`. The pending `f6-open-pr` is dropped. `Validate()` is clean.
    Re-entering phase 6 seeds `f6-open-pr-r2` needing `f6-consolidate-r2`,
    the four and the repair step, and the four needing `f6-consolidate-r2`.
    This is the `ab54c55` case.
  - `Back(-to 1)` from phase 3 with a pending phase-2 spike that is not a
    template step and a pending phase-6 step: both are kept, and the
    template steps of phases 1-3 are dropped and reseeded per the current
    templates. The existing third-return escalation test still passes.
  - Two returns: a done `f6-validate-x` and `Back(-to 3)` give one pending
    `f6-validate-x-r2`; a second `Back` before it runs adds nothing; once
    `-r2` is done, a third gives `f6-validate-x-r3`, not `-r2-r2`. A done
    phase-2 host step stays as it is.
  - `Seed` on phase 6 links the gate and `consolidate` as in Decision 4, and
    a step added after seeding is not linked. `Seed` on phase 2 leaves
    `f2-design-review` (or whichever template step is the gate) without the
    added spike in its `needs`.
  - `Validate()` reports a repeated `f6-validate-x` id, the result of a
    Design that re-adds a kept step.
  - The QUEUE.md golden (`testdata/queue.md`): the phase-2 item's Next steps
    drop `f6-validate-windows-hyperv`, and "Waiting on a host" shows it with
    `(from phase 6)`. A phase-6 human step on the same item appears under
    "Needs Jeff" and in its Next steps.
- **Existing tests:** `TestCompleteWalksOneRound`,
  `TestBackToImplementKeepsDesign` and
  `TestBackToDesignEscalatesOnTheThirdReturn` pass unchanged.
- **Against live state:** at x2-implement, run `wf validate` and `wf next`
  for each host against a copy of the workflow branch. build-link-subnet's
  four validations are still offered, and nothing else changes.
- **No native validation:** the change is host-independent Go plus text.

## Implementation plan

Written at x2-implement, after the review (process item, FLOW-03). One commit
per task on this branch.

1. `tools/workflow/queue/next.go` (FLOW-04, FLOW-09; Decision 1): `Offered`
   and `Early`; the condition in `ready()` and `leasedTo()`.
2. `tools/workflow/queue/phases.go` (FLOW-03, FLOW-06, FLOW-09; Decisions
   2-4): `isTemplate` shared with `IsGate`; the `waitsAdded` flag on
   `open-pr`; `Back`'s keep set, lease clearing, `needs` pruning and
   base-name re-run copies; `Seed`'s Publish linking. Update the doc comments
   on `Back`, `Seed` and `templates`.
3. `tools/workflow/cmd/wf/main.go` (FLOW-04; Decision 1): `claimable` and the
   `done` branch of `stepCmd` return `queue.Early`'s error. Add `[-to 1|3]`
   to the usage line for `back` (line 14), and fix `back`'s doc comment
   (line 300), which names only Design.
4. `tools/workflow/queue/summary.go` and `testdata/queue.md` (Decision 1):
   `readySteps()` uses `Offered`, which covers "Needs Jeff" and the Active
   Next-steps column; the `(from phase <n>)` suffix in `hostSteps()`.
5. Tests in `queue_test.go`, `hosts_test.go` and `summary_test.go` (with its
   golden fixture) for each Validation plan case (TEST-01, TEST-02 tier 0).
   The claim and done refusals are covered through `Early`, since `cmd/wf`
   has no tests.
6. `docs/development/process.md` (FLOW-14 process route; Decision 7):
   - **FLOW-04:** after the affinity list, add "A step is offered when every
     step it needs is done and, unless it is `human`, its feature has reached
     the step's phase; no worker may claim or complete a host or cloud step
     of a later phase."
   - **FLOW-06:** at the end, add "A return drops every pending step of the
     phase it returns to and the pending template steps of later phases,
     which are seeded again on re-entry. Other pending steps of later phases
     stay, and Design reconciles them with the new design. A native
     validation already done in Publish is queued again."
   - **FLOW-09:** "Such a feature waits in Publish on a `host:<name>` step"
     gains "which runs after consolidation and which the PR's opening needs".

   Severity and check stay as they are for all three.
7. `.claude/agents/wf-design.md` step 3 (FLOW-09, FLOW-06; Decision 5): add
   `f6-validate-<host>` with affinity `host:<host>`, its cases in the note,
   and no `needs`; Publish's seeding links it. On a return, reconcile the
   kept later-phase steps with the new `native_validation`: keep and update,
   add only missing hosts, remove steps for hosts now tracked or work no
   longer needed.
8. `.claude/agents/wf-plan.md` step 2 or 3 (FLOW-06; Decision 5): Plan may
   refine the note of a later-phase step Design added; it does not add or
   remove such steps, and a missing blocking host goes back to Design.
9. `.claude/skills/workflow-native/SKILL.md`, "Record" (line 86; Decisions 1
   and 2): when `wf done` refuses a step as a later-phase step (the feature
   was returned while it ran), release it with the evidence as the reason
   and leave it pending. `workflow-tick` changes in no text: it never told
   an agent to add `needs` by hand; that instruction appeared only in the
   2026-11-04 digest and in the steps' notes.
10. The workflow branch `README.md`, "Steps and phases": update the
    paragraph starting "Design adds the steps it finds" so it says that
    later-phase host steps wait for their phase, survive `wf back`, run after
    consolidation and are needed by Publish's gate. Commit this on the
    workflow branch once the PR merges (x3-merge), since that is when the
    loop runs the new `wf`.
11. `overview.md` (FLOW-15): no component flow changes; none expected.
12. Consolidation, `make lint`, `make test` (FLOW-10, FLOW-17). Archive the
    design and open the PR (FLOW-12, FLOW-08).

## Design review

- **r1, 2026-11-06: approved at `6824812`** (Jeff, x1-design, FLOW-19). He
  reviewed this design alongside flow19-design-before-plan,
  wf-later-phase-host-steps and budgets-active-only, all on one review page
  (published 2026-11-05 with every assumption first). He agreed with every
  recommendation: a1 to a6 are confirmed as drafted, option (a).

## Consolidation

Filled in at Publish (FLOW-10, FLOW-15): features landed since the fork,
terminology aligned, abstractions shared, seams reconciled, rules proposed, and
the dead-code, duplication and overview sweep.

## Rollback notes

## Archive summary

Filled in at Publish, in the feature's PR, and refreshed when the PR changes
(FLOW-12): what was built, decisions taken and rejected, and what a future design
touching these `concepts` needs to know.
