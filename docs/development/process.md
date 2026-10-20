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

1. **Parallel round.** Features are designed and implemented concurrently, each on
   its own feature branch, against the `main` they forked from.
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
| 6 | Publish | Parallel round: branch pushed. Merge round: PR opened, CI green (FLOW-11), blocking native validation done (FLOW-09). |
| 7 | Sign-off | A human signs off the PR and its tier-2 report (FLOW-08) and merges it. |
| 8 | Archive | Design marked archived (FLOW-12); feature branch deleted. |

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
  (the macOS/arm64 laptop) or `host:windows-hyperv` (the Windows desktop).

A worker takes only steps whose affinity it satisfies. A step is claimed with a
lease (claimant plus expiry) recorded on the `workflow` branch; a push rejected
because the branch moved means the claim lost and the worker re-reads state. An
expired lease may be reclaimed.

---

### FLOW-05 — Finish before starting

**Severity:** SHOULD
**Check:** automated

Workers rank steps by: merge-round before parallel-round, then later phase before
earlier, then queue priority. The parallel round holds a limited number of features
in progress (default 3); a new feature does not start design while the limit is
reached. During the parallel round a feature branch periodically merges `main` with
conflict-only fixes, so the merge-round consolidation stays small.

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
has not already settled.

---

### FLOW-08 — Only a human merges to trunk

**Severity:** MUST
**Check:** automated

This is LINT-03 applied to the process. No agent approves or merges a PR to `main`,
and auto-merge is not enabled by agents. The human sign-off step covers the PR, the
tier-2 behavioral report (acknowledge, or dismiss with a recorded reason), and any
unconfirmed assumptions (FLOW-07). Changes that declare a `security_boundary`
additionally need the security note acknowledged (GLOBAL-06).

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
every waiting branch in one sitting rather than one session per feature. A failure
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

The unattended loop takes one step per session: read state, claim, work, write
state, exit. All state it needs lives in the repository, never in a long-running
conversation. Each feature has a work budget; exhausting it escalates to a `human`
step. The loop posts a periodic digest (what moved, what is blocked on a human,
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

## Bootstrap order

The process starts with this ordering, highest first. The windows-driver
integration line (`integrate/windows-driver`) and this doc set have both landed on
`main`.

1. **Hyper-V Phase 5a — Windows-guest consolidation** (`host:windows-hyperv`,
   attended on the Windows desktop). Re-validate what the windows-driver
   integration pass applied to the Hyper-V driver, then consolidate the Windows
   guest lifecycle with qemu, per Phase 5a in
   [`vm-driver-release-plan.md`](../design/vm-driver-release-plan.md), including
   its rollback notes.
2. **Reconcile legacy guidance.** Reduce `CLAUDE.md`, `AGENTS.md`, `DEVELOPING.md`,
   `CONTRIBUTING.md`, and the mdbook `contributing/` tree to pointers here,
   promoting any rule that exists only there.
3. **Stand up the workflow.** Create the `workflow` orphan branch and the agent
   definitions (FLOW-14), and run behavioral-linter tier 2 in report-only mode
   (LINT-07).
4. **Normal queue**, in priority order: the open items in the README, then feature
   work such as the Firecracker driver.
