# `workflow`: BuildWarden's feature-queue state

This orphan branch shares no history with `main`. It holds the bookkeeping of
the development process in
[`docs/development/process.md`](https://github.com/buildwarden/buildwarden/blob/main/docs/development/process.md):
which features are queued, what phase each is in, who holds which step, what is
blocked on a human or a native host, and what each daily run did. It lives here,
not on `main`, so the high-churn state stays out of the code history (FLOW-03).

The rules are in `docs/development/`; the procedures that read and write this
branch are the skills under `.claude/skills/` on `main` (`workflow-tick`,
`workflow-human`, `workflow-native`, `workflow-intake`). Nothing here is
hand-edited when a `wf` command can make the change.

## Layout

| Path | What |
|---|---|
| `config.yaml` | Loop settings: the priority gate, run budget, FLOW-05 limits, lease length, runaway factor. |
| `queue/<feature>.yaml` | One file per feature: its state, steps, budget, questions and history. |
| `backlog/native-gaps.yaml` | Validation deferred by merged designs, per host (FLOW-09, TEST-03). |
| `digest/<YYYY-MM-DD>.md` | One file per day (Pacific), a section per run: what moved, what is blocked on Jeff, what waits on a host, what was spent (FLOW-13). |
| `reports/<feature>/<sha12>.{md,json}` | The behavioral linter's tier-2 report for a branch head (LINT-04, LINT-05), report-only (LINT-07). |
| `native/<host>/<date>/` | Logs a native session attaches to a failure. |

## `config.yaml`

```yaml
priority_confirmed: false   # unattended runs do nothing until the human confirms the order
run_budget: {steps: 8}      # steps one run may take; calibrate against /usage
limits:
  in_progress: 5            # FLOW-05
  alters_relationships: 1   # FLOW-05: of those, past Design with alters_relationships: true
lease_minutes: 120          # FLOW-04
runaway_factor: 2           # FLOW-13: escalate at this multiple of the step estimate
```

## `queue/<feature>.yaml`

```yaml
feature: container-inputs        # == file name
title: Record container inputs under inputs/ (ORCH-02)
priority: 2                      # 1 is highest; unique
status: in_progress              # queued | in_progress | done
round: parallel                  # parallel | merge (FLOW-03)
phase: 1                         # FLOW-03 phase: 0 branch-setup .. 8 archive
design: docs/design/container-inputs.md
branch: feat/container-inputs
alters_relationships: null       # null until Design decides (FLOW-02)
source: where the item came from
pr: {number: 0, ci: pending}     # merge round only: pending | green | red
steps:
  - id: p1-design                # <p|m><phase>-<name>[-r<n>]
    phase: 1
    affinity: cloud              # cloud | human | host:<name> (FLOW-04)
    status: pending              # pending | done
    needs: [p1-answer-q1]        # step ids that must be done first
    lease: {claimed_by: "cloud/20261029T095500Z", expires: 2026-10-29T11:55:00Z}
    note: optional
budget:                          # FLOW-13 runaway check, not a share of the run
  estimate: {runs: 4, steps: 18}
  spent: {runs: 1, steps: 3}     # a claim counts a step; a run's first claim counts a run
questions:                       # FLOW-07; never GitHub issues
  - id: q1
    kind: blocking               # blocking | assumed
    status: open                 # open | answered (blocking) | confirmed (assumed)
    question: ...
    options: [...]
    recommended: ...
    answer: set by the human
history:                         # append-only
  - {at: 2026-10-29T09:55:00Z, by: cloud/20261029T095500Z, event: claimed p1-design}
```

### Steps and phases

Each phase starts with template steps, and the phase's last template step is
its **gate**: marking it done runs the phase's FLOW-03 done-when check, and
the transition happens only if the check passes.

| Phase | Parallel round | Merge round |
|---|---|---|
| 0 | `p0-branch-setup` | `m0-merge-main` |
| 1 | `p1-design` | `m1-consolidate` |
| 2 | `p2-plan` | `m2-plan` |
| 3 | `p3-implement` | `m3-implement` |
| 4 | `p4-review` | `m4-review` |
| 5 | `p5-guide` | `m5-guide` |
| 6 | `p6-publish`, then `p6-merge-round` (waits for the serial merge queue) | `m6-confirm-pr` (human), `m6-publish` |
| 7 | | `m7-sign-off` (human; its check is the merge) |
| 8 | | `m8-archive` |

Design adds the steps it finds: `p1-answer-<q>` (human) for a blocking
question, `m6-validate-<host>` (`host:<host>`) for blocking native validation.
A return to Design (FLOW-06) re-seeds phase 1 with `-r<n>` ids; the third
return adds a human `escalate` step.

## Claiming a step

Workers are `cloud`, `human` or `host:<name>`, with a `/<session>` suffix that
tells their leases apart. From a checkout of `main`, with this branch checked
out at `.dev/workflow`:

```bash
(cd tools/workflow && go build -o ../../dist/wf ./cmd/wf)
dist/wf next  -state .dev/workflow -worker cloud/$RUN
dist/wf claim -state .dev/workflow -worker cloud/$RUN -feature F -step S -new-run
git -C .dev/workflow commit -am "F: claim S" && git -C .dev/workflow push origin workflow
# ... do the step ...
dist/wf done  -state .dev/workflow -repo . -worker cloud/$RUN -feature F -step S -run
git -C .dev/workflow commit -am "F: done S" && git -C .dev/workflow push origin workflow
```

A rejected push means the branch moved under you and the claim lost (FLOW-04):
fetch, reset this checkout to `origin/workflow`, and ask `wf next` again. An
expired lease may be reclaimed. State is written after every step, so an
interrupted run loses at most the step in progress (FLOW-13).

`wf next` follows FLOW-05: the highest-priority in-progress feature's first
ready step; a new feature only when every in-progress one is blocked and fewer
than five are in progress; the cloud skips a feature with an open blocking
question, a spent runaway budget, or a need for the `alters_relationships`
slot another feature holds. Its `blocked` lines feed the digest.

Other commands: `wf validate`, `wf status`, `wf check` (done-when without
moving), `wf release`, `wf back` (FLOW-06), `wf answer` (human only, FLOW-07),
`wf new` (intake).
