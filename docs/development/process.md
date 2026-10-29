# Development Process Standards

How a feature moves from an idea to a merged, archived change, whether the work is
done by a human, an attended agent session, or the unattended feature loop. The
other documents in this directory say what a good change looks like; this one says
how changes are designed, sequenced, validated, and signed off so that the other
standards are actually applied.

See [`README.md`](./README.md) for the rule format.

## The shape of the process

Features move through a work queue. Each feature passes through the development
phases **twice**:

1. **Parallel round.** Each feature is designed and implemented on its own
   feature branch, against the `main` it forked from. Work goes depth first on
   the top-priority feature; others advance only while it is blocked (FLOW-05).
2. **Merge round.** Features enter a serial merge queue. The feature on deck merges
   current `main`, then repeats design as a **consolidation pass** against every
   feature that landed while it was in the parallel round, and carries the result
   through PR, CI, sign-off, and merge.

The merge round is the routine form of the consolidation that followed the Windows
driver work (the claim / tunnel / collector model). Doing it per feature keeps the
reconciliation small and keeps terminology and shared abstractions converging
rather than forking.

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
  `driver.Driver` / `driver.Session` level (ORCH-01).
- `seams` — which versioned seams change (GLOBAL-08).
- `surfaces` — which user-facing surfaces change (UX-01, ORCH-03).
- `security_boundary` — which GLOBAL-06 boundary is altered, if any. Any value other
  than `none` requires a security note in the design.
- `alters_relationships` — `true` when the change alters a component's role or
  how components interact: a versioned seam (GLOBAL-08), the bootstrap model
  (BOOT rules), which component owns a responsibility (relay, collector,
  executor, orchestrator, a driver), or the `driver.Driver` / `driver.Session`
  contract. Changes inside one component's existing role are `false`. FLOW-05
  limits how many such features are in flight.
- `native_validation` — which native hosts must validate the change, and whether
  each blocks merge (FLOW-09).
- `tier2_gaps` — platform validation knowingly deferred (TEST-03).
- `rules` — the rule IDs the design relies on or changes.
- `concepts` — retrieval tags for the archive (FLOW-12).

Declarations are derived conservatively: a change to a shared package (`relay/`,
`ledger/`, `collector/`, `driver/`) declares every dependent driver and host, per
the TEST-02 caveat.

The automated part diffs the declared `drivers` / `seams` / `surfaces` against the
trees and files the change reaches; the agent part judges the security note.

---

### FLOW-03 — Phases, transitions, and definitions of done

**Severity:** MUST
**Check:** automated

Each feature is a queue item whose state records its round, its current phase, and
its branch. The phases are:

| # | Phase | Done when |
|---|-------|-----------|
| 0 | Branch setup | Branch exists; in the merge round, current `main` is merged with conflict-only fixes. |
| 1 | Design | Design doc complete per FLOW-02; no unanswered blocking question (FLOW-07). In the merge round, the consolidation section is filled in (FLOW-10). |
| 2 | Plan | Implementation and testing tasks listed, each citing the rules it must satisfy; rework of existing code is listed as explicit tasks. |
| 3 | Implement | Tasks done; `make lint` and `make test` green; reference docs updated in the same change (UX-01). |
| 4 | Review | Behavioral-linter tier 1 green; tier-2 report produced for the diff (LINT-04). |
| 5 | Guide | User-facing narrative docs (guides, examples) written or updated. |
| 6 | Publish | Parallel round: branch pushed at each checkpoint (FLOW-17). Merge round: PR opened with the human's confirmation, CI green (FLOW-11), blocking native validation done (FLOW-09). |
| 7 | Sign-off | A human signs off the PR and its tier-2 report (FLOW-08) and merges it. |
| 8 | Archive | Design marked archived (FLOW-12); merged branches and validation leftovers cleaned up (FLOW-16). |

A transition happens only when the phase's done-when checks pass mechanically
(files present, sections filled, commands green), never on an agent's own report
that it is done. Queue state lives on the `workflow` orphan branch, not on `main`,
so that high-churn bookkeeping stays out of the code history.

---

### FLOW-04 — Steps declare who may take them

**Severity:** MUST
**Check:** automated

Every pending step carries an affinity:

- `cloud` — design, planning, implementation, review, docs, CI fixes. Taken by the
  unattended loop.
- `human` — answering blocking questions, confirming assumptions, sign-off.
- `host:<name>` — work that needs native hardware, for example `host:macos-vz`
  (the macOS/arm64 laptop) or `host:windows-hyperv` (the Windows desktop). A
  name `<os>-<driver>` is specific to that platform; `<capability>-any` (such as
  `qemu-any` or `container-any`) runs on any native machine that has the
  capability. The workflow branch's `hosts.yaml` gives each host name to exactly
  one machine.

A worker takes only steps whose affinity it satisfies. A step is claimed with a
lease (claimant plus expiry) recorded on the `workflow` branch; a push rejected
because the branch moved means the claim lost and the worker re-reads state. An
expired lease may be reclaimed.

---

### FLOW-05 — Depth first on the top-priority feature

**Severity:** SHOULD
**Check:** automated

Work goes deep, not wide. A worker takes the next ready step of the
highest-priority feature in progress, and keeps taking that feature's steps until
it blocks: on a `human` or `host:<name>` step, an exhausted budget, or the merge
queue. Only then does it take steps from the next feature down, and it returns to
the higher-priority feature as soon as that one has a ready step again.

While interfaces are still stabilizing, a feature developed in parallel is likely
to need an involved rebase and consolidation once the features ahead of it land.
Parallel features are therefore the fallback that keeps work moving while the
human is away, not a goal. A new feature starts Design only when every feature in
progress is blocked.

At most 5 features are in progress at once, and at most 1 of them declares
`alters_relationships: true` (FLOW-02), because two such features reshape the same
structure and make each other's consolidation costly. A feature whose design
discovers it alters relationships waits at the end of Design until that slot is
free. A feature waiting behind others periodically merges `main` with
conflict-only fixes, so its merge-round consolidation stays as small as it can.

---

### FLOW-06 — Implementation does not silently redesign

**Severity:** MUST
**Check:** agent-judgment

When implementation finds that the plan is no longer viable, or uncovers complexity
the design did not consider, the feature returns to Design with the finding
recorded. Design then decides; planning adds any alteration of already-implemented
code as explicit tasks. A feature that returns to Design more than twice escalates
to a `human` step instead of looping again.

---

### FLOW-07 — Questions are asked in design, in two kinds

**Severity:** MUST
**Check:** agent-judgment

Questions about intent are raised during Design, not discovered mid-implementation.
Each question is one of:

- **Blocking** — the design cannot proceed without the answer. The feature waits on
  a `human` step; the worker moves on to other work.
- **Assumed** — the design proceeds on a stated default, recorded in the design as
  "proceeding with X; confirm". Assumptions are confirmed in batches and must all
  be confirmed before sign-off (FLOW-08).

Every question states the options considered and a recommended answer. Later
phases route questions back through Design, which escalates only what the design
has not already settled. Questions live in the feature's queue state on the
`workflow` branch and in the loop's digest, not as GitHub issues.

---

### FLOW-08 — Only a human merges to trunk

**Severity:** MUST
**Check:** automated

This is LINT-03 applied to the process. No agent approves or merges a PR to `main`,
and auto-merge is not enabled by agents. The human sign-off step covers the PR, the
tier-2 behavioral report (acknowledge, or dismiss with a recorded reason), and any
unconfirmed assumptions (FLOW-07). Changes that declare a `security_boundary`
additionally need the security note acknowledged (GLOBAL-06).

Pushing a feature branch is not a trunk change and needs no confirmation
(FLOW-17); opening the PR to `main` does.

---

### FLOW-09 — Native validation: blocking or tracked

**Severity:** MUST
**Check:** automated + agent-judgment

Native validation is tier-2 testing (TEST-02) that cannot run in CI today, for
example VZ on macOS/arm64 or Hyper-V on Windows. Whether it blocks merge is decided
per host in the design:

- **Blocking** when the change is specific to that host's driver or lifecycle: it
  reaches that driver's tree, or alters behavior only that host exercises. Such a
  feature waits in Publish on a `host:<name>` step.
- **Tracked** otherwise: the gap is recorded in `tier2_gaps` and on the `workflow`
  branch's native-gap backlog, satisfying TEST-03, and the feature may merge.

Native sessions drain their host's steps and backlog in batches, testing `main` plus
every waiting branch in one sitting rather than one session per feature. Each
native machine also runs a scheduled session that starts an agent only when a
host step it serves is ready, or when `main` or a waiting branch has moved since
its last sweep of open backlog entries; it is bound by the unattended loop's limits
(FLOW-13) and records what it did in the digest. A failure
found there returns the feature (or a new fix item) to Design or Implement with the
logs attached. The full matrix before a release (TEST-02) is the backstop for every
tracked gap.

Work whose design or implementation itself needs the native host (for example a
platform or hypervisor update) carries `host:<name>` affinity on those phases too.

---

### FLOW-10 — The merge round consolidates

**Severity:** MUST
**Check:** agent-judgment

The merge-round design pass reviews the feature against everything that landed on
`main` since its branch forked, and records in the design's consolidation section:

- terminology aligned with the canonical terms used in `main` and these standards,
- shared abstractions reused instead of duplicated (one code path where two
  drivers or components do the same thing),
- versioned seams that more than one feature changed (GLOBAL-08),
- behavior or convention changes the rest of the codebase must now follow.

A convention that emerges from consolidation and should bind future work is
proposed as a new or amended rule in this directory, in the same PR or as a
follow-up item. Rule changes go through human sign-off like any other change.

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
summary section filled in: what was built, the decisions taken and the alternatives
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
top-priority feature can use all of it. A scheduled native session (FLOW-09)
spends its own budget, `native_budget`, which falls back to the run's budget when
unset, so native and integration testing never compete with the cloud loop. Each
feature also has a work budget, which
is a runaway check rather than a share of the run: exhausting it escalates to a
`human` step. The loop posts a periodic digest (what moved, what is blocked on a human,
what is waiting on a native host, what was spent), and notifies when a new
blocking question appears.

---

### FLOW-14 — Agent tooling is a thin layer over this doc set

**Severity:** MUST
**Check:** agent-judgment

Agent definitions and skills (for example under `.claude/` or `.kiro/`) carry
phase instructions and cite rule IDs; they do not restate rules. When a rule and an
agent definition disagree, the rule wins and the definition is fixed. Agents do not
change their own definitions or these standards unattended; proposed changes go
through human sign-off (FLOW-08).

---

### FLOW-15 — The merge round sweeps duplication, dead code and the overview

**Severity:** MUST
**Check:** automated + agent-judgment

FLOW-10 looks across features; this sweep looks inside the codebase the feature
leaves behind. The merge round records its results in the design's consolidation
section:

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

When a feature's PR merges, the Archive phase removes what the feature no
longer needs:

- its branch, on the remote and locally, and any worktree checked out on it;
- the branches it was stacked on, once they are merged too;
- the leftovers of its validation: test images and containers, downloaded
  media, scratch output, and anything created outside the documented cache
  locations.

The exception is a branch whose design names unfinished work meant to continue
on that same branch: its implementation plan lists the remaining tasks and says
the branch stays open for them. Follow-up work tracked anywhere else (a
deviation, a deferred question, a validation left for another host) is not a
reason to keep a branch; it starts from trunk on a new one.

Deleting a branch loses any commit not on trunk, so a branch is deleted only
after it is shown to be contained in trunk (`git branch --merged`, or the PR's
merge commit), with git's safe forms (`git branch -d`, never `-D`) and never by
force-pushing. Caches a later build reuses (prepared images, support images)
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

What still needs the human's confirmation is opening the PR to `main`. Pushing
never includes `main` itself, a force-push, rewriting history that is already on
the remote, or deleting a remote branch (FLOW-16).

---

### FLOW-18 — Agent sessions are recorded privately

**Severity:** MUST
**Check:** automated + agent-judgment

Agent sessions are recorded with the Entire Checkpoints CLI, which links each
session's prompts and transcript to the commits it produced. Recording serves
three uses: a daily run resumes the previous run's session instead of starting
cold (`entire session resume`), the Review phase and the human sign-off read why a
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
