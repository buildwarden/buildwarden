---
feature: workflow-bootstrap
status: active
round: parallel
drivers: []
seams: []
surfaces: []
security_boundary: none
alters_relationships: false
native_validation: []
tier2_gaps: []
rules: [FLOW-03, FLOW-04, FLOW-05, FLOW-07, FLOW-08, FLOW-13, FLOW-14, FLOW-16, FLOW-17, FLOW-18, LINT-03, LINT-07]
concepts: [workflow, feature-queue, leases, phase-agents, daily-loop, session-recording]
related: []
---

# Workflow bootstrap: the queue, the phase agents, and the daily loop

## Problem

`process.md` defines how features move from idea to archive, worked by the
unattended loop, attended sessions and native hosts, but none of it exists yet:
no queue state, no agent definitions, no loop, and no guard that agent sessions
stay out of this public repository. Bootstrap step 1 of `process.md` is to stand
it up.

## Prior art

No archived design covers process tooling. The design is `process.md` itself
(FLOW-01..18) and `behavioral-linter.md` (LINT-03, LINT-07); this document
records only the decisions taken to implement them. Session recording reuses
Jeff's `entire enable` output (`chore/entire-sessions`), merged unchanged.

## Decisions

- **Queue state on the `workflow` orphan branch** (FLOW-03): `config.yaml`,
  `queue/<feature>.yaml`, `backlog/native-gaps.yaml`, `digest/<date>.md`,
  `reports/<feature>/<sha>.{md,json}`. The schema is in that branch's
  `README.md`. Rejected: state on `main` (churn in code history) and GitHub
  issues (FLOW-07 keeps questions in queue state).
- **A small queue tool, `wf`**, in its own Go module under `tools/workflow/`.
  It picks the next step in FLOW-05 order, takes and releases leases
  (FLOW-04), counts budgets (FLOW-13), and runs each phase's done-when checks
  (FLOW-03), so no transition rests on an agent's report. Rejected: agents
  editing YAML by hand (the checks would be judgment, not mechanical), a shell
  script over `yq` (no dependable YAML tool in every session), and adding a
  YAML dependency to the product module.
- **Step ids** are `<round letter><phase>-<name>` (`p1-design`,
  `m6-confirm-pr`), with `-r<n>` when a phase is re-entered after a return to
  Design (FLOW-06). Each phase's last template step is its gate.
- **Thin agent definitions** (FLOW-14): one subagent per phase under
  `.claude/agents/wf-*.md` and four entry-point skills under `.claude/skills/`
  (`workflow-tick`, `workflow-human`, `workflow-native`, `workflow-intake`).
  Each cites rule IDs and the done-when it must reach; none restates a rule.
- **The daily loop** is a scheduled task at `CRON_TZ=America/Los_Angeles
  55 2 * * *` that starts a fresh session and runs `/workflow-tick`. All state
  is on the `workflow` branch, so nothing depends on a long-lived conversation
  (FLOW-13). Until the human sets `priority_confirmed: true`, a run only
  writes a digest saying so.
- **Review is report-only** (LINT-07): `wf-review` writes the tier-2 report;
  the Review gate requires the report to exist and tier 1 to be green, never
  a clean tier-2 report.
- **FLOW-18 guard**: `tools/workflow/flow18-check.sh` fails if
  `.entire/settings.json` does not name `github:buildwarden/buildwarden-sessions`
  as `checkpoint_remote`, or if `origin` carries any `entire/` ref. It runs in
  CI on PRs, on pushes to `main`, and daily (a pushed ref triggers no
  workflow), and at the start and end of every tick and before every publish.

## Questions

### Blocking

1. **Priority order of the seeded queue.** Options: the order below; Firecracker
   first; the security suite first. Recommended: the order below. Unattended
   runs wait on this answer (`priority_confirmed`).

   1. `container-inputs` (ORCH-02): small, and the trial run.
   2. `tier1-static-checks` (LINT-02, Deep-dive #2): makes the Review gate
      deterministic for everything after it.
   3. `specs-migration` (RELAY-05): a file move the static checks then guard.
   4. `firecracker-driver`: large; its plan commit `f9b07c5` is on Jeff's Mac
      and must be pushed before it starts (a `human` step).
   5. `security-suite` (TEST-08, Deep-dive #1): large; Arm B needs an isolated
      harness.
   6. `design-front-matter-retrofit` (FLOW-12): mostly done as designs rely on
      old plans; this item sweeps whatever is left.

### Assumed

- Proceeding with a run budget of 8 steps; Jeff calibrates against `/usage`
  and edits `run_budget` in `config.yaml`. Confirm.
- Proceeding with an unattended run that carries on without the Entire CLI,
  flagging "sessions not recorded" first in the digest, rather than stopping.
  Confirm.
- Proceeding with feature branches named `feat/<feature>`. Confirm.
- Proceeding with this design merging as one PR, outside the loop it
  creates, and its queue item sitting at sign-off. Confirm.

## Design

```
  scheduled task (02:55 PT) --> fresh session --> /workflow-tick
     |
     |  dist/wf next --worker cloud/<run>     (FLOW-05 order, leases, budgets)
     |  dist/wf claim ... ; push workflow    (rejected push = lost claim)
     |  Agent(wf-<phase>) on the feature branch
     |  dist/wf done ... -run                (done-when checks, FLOW-03)
     |  push workflow                        (state after every step)
     v
  digest/<date>.md  -- blocked on Jeff --> /workflow-human
                    -- waiting on host --> /workflow-native <host>
```

`wf next` takes the highest-priority in-progress feature's first ready step;
a step is ready when its `needs` are done, its affinity matches the worker, its
lease is free or expired, and, for the cloud, the feature has no open blocking
question, has not spent twice its step estimate, and does not need the one
`alters_relationships` slot another feature holds. Only when every in-progress
feature is blocked, and fewer than five are in progress, does it start the
next queued one. A parallel-round feature that finishes Publish waits on a
`p6-merge-round` step that is ready only when no other feature is in the merge
round.

## Cross-driver impact

None: no driver, seam or user-facing surface changes.

## Validation plan

Tier 0: `tools/workflow` unit tests cover FLOW-05 ordering, leases, the
runaway check, the alters slot, the serial merge queue, both rounds' phase
walk, the return to Design, and the done-when checks against a scratch git
repository. The FLOW-18 check was run against a scratch remote carrying
`refs/entire/...` and `refs/heads/entire/...` refs. An attended tick on
`container-inputs` exercises the whole loop once. No native host is involved.

## Implementation plan

1. `tools/workflow`: the `wf` command, queue model and done-when checks, with
   tests (FLOW-03, FLOW-04, FLOW-05, FLOW-13, TEST-01).
2. `tools/workflow/flow18-check.sh` and the `flow18` CI workflow (FLOW-18).
3. Merge `chore/entire-sessions` (FLOW-18).
4. `.claude/agents/wf-*.md` and `.claude/skills/workflow-*` (FLOW-14,
   LINT-03, LINT-07).
5. The `workflow` orphan branch: README, config, seeded queue, native-gap
   backlog (FLOW-03, FLOW-09).
6. Point `docs/development/README.md` and `process.md` at the new state, and
   list `tools/workflow` in `contributing/structure.md` (FLOW-14, UX-01).
7. The daily scheduled task (FLOW-13).

## Rollback notes

## Archive summary
