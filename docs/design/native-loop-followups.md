---
feature: native-loop-followups
status: draft
round: parallel
drivers: []
seams: []
surfaces: []
security_boundary: none
alters_relationships: false
native_validation:
  - {host: macos-vz, blocking: false, reason: "The status file and the native budget are exercised by the Mac's next scheduled run; the trap is tested here against a scratch bare remote."}
  - {host: windows-hyperv, blocking: false, reason: "The Windows machine is not installed yet; its first run exercises the same wrapper under Git Bash."}
tier2_gaps: []
rules: [FLOW-03, FLOW-04, FLOW-05, FLOW-08, FLOW-09, FLOW-13, FLOW-14, FLOW-17, FLOW-18, TEST-02, TEST-03]
concepts: [workflow, native-validation, scheduled-sessions, integration-ci, budget, machine-status, process-lane, notifications]
related: [native-host-loop.md, workflow-bootstrap.md]
---

# Native-loop follow-ups: tier 1 in CI, a native budget, machine status, a process lane

## Problem

The native-host loop (`native-host-loop.md`, PR #64) went live on
2026-10-29 and left these gaps:

- **Tier 1 never runs.** TEST-02 says tier 1 (integration with a container
  runtime) runs on CI, but no workflow runs `test-integration.sh`. Cloud
  sessions have no container runtime, so container-inputs is waiting on a
  `make integration-test` that nobody has run. The script is also broken:
  it calls `./warden`, but `make build` writes `dist/warden`. The Mac's
  first native run reported this.
- **One budget for two loops.** A native run spends the same
  `run_budget.steps` as the cloud tick. Integration and native testing
  should never compete with the cloud loop for budget.
- **Silent failures.** A run that fails before Claude starts pushes nothing.
  That happened when the Mac's `claude` was not logged in. Then on
  2026-10-29 the Mac's first real run did its work but could not commit or
  push it, so its results sat uncommitted in the dedicated checkout, and
  the next run's reset would have deleted them. The cause:
  - The session ran git as `git -C <absolute path>/.dev/workflow ...` and as
    `cd .dev/workflow && git ...`.
  - `settings.native.json` only allows the relative form
    `git -C .dev/workflow:*`, so every attempt needed an approval nobody
    was there to give.
- **No morning picture.** Jeff learns what happened overnight only by reading
  the digest.
- **Process changes are invisible to the loops.** A change to
  `docs/development/`, `.claude/` or `tools/workflow/` alters the rules the
  loops run under. Today it merges outside the queue, as workflow-bootstrap
  and native-host-loop did, so no loop sees it coming.

## Prior art

- `native-host-loop.md`:
  - `hosts.yaml` and `wf gate`, both reused unchanged.
  - The wrapper, which this design extends with a status trap.
  - The skill's unattended mode, whose batch now counts against
    `native_budget`.
- `workflow-bootstrap.md`: the queue model, step templates, done-when checks
  and the one-PR-outside-the-queue precedent. The process lane makes that
  precedent a queue kind.
- TEST-02 says where tier 1 should run. FLOW-13 covers the run budget, and
  FLOW-14 says why process work is `human` throughout.

## Decisions

All confirmed by Jeff on 2026-10-29 in the Cowork session that produced the
hand-off plan, except where marked *(this design)*.

### 1. Tier 1 in CI

- `.github/workflows/integration.yml` runs `make integration-test` on
  `ubuntu-latest`, with Go set up from `go.mod`. Hosted Ubuntu runners have a
  rootful Docker daemon, which is the privilege the relay's network isolation
  needs.
- Triggers:
  - `workflow_dispatch` with a `ref` input, to test any branch;
  - a weekly schedule (Mondays 06:00 UTC), to test `main`;
  - `pull_request` on the paths the TEST-02 caveat makes shared:
    - `driver/container/**`, `driver/*.go`;
    - `relay/**`, `ledger/**`, `collector/**`, `cmd/**`;
    - `test-integration.sh`, `Dockerfile*`, `go.mod`.
- On failure, the job uploads the warden output directories
  (`/tmp/warden-integ-*`) as an artifact.
- The script's binary path is fixed: `WARDEN` defaults to `./dist/warden`
  and can be overridden. The job also links `./warden` to `dist/warden`, so
  older refs whose script still says `./warden` run too, `feat/container-inputs`
  among them.
- *(this design)* **Proving it before merge.** GitHub only offers
  `workflow_dispatch` for workflows on the default branch, so the
  "one green dispatch run" check cannot happen before this PR merges.
  - Until then, a temporary `push` trigger on `feat/native-loop-followups`
    runs the job twice: once on the branch head and once on
    `feat/container-inputs`.
  - The commit that removes the temporary trigger lands before Jeff opens
    the PR.
  - The two `workflow_dispatch` runs that FLOW-03 asks for happen after
    merge, recorded as a follow-up below.
- *(Jeff decides at sign-off)* whether `integration` becomes a required check.

### 2. Fallback: an automatic tier-1 step on a native host

Only if hosted runners cannot run tier 1. `wf` would seed an `integration`
step with `host:container-any` affinity in Publish whenever a design's
`drivers` includes `container` or it reaches a shared package. This item is
skipped when item 1 is green (see the Validation plan for the outcome).

### 3. A separate native budget

- `config.yaml` gains `native_budget: {steps: N}`, starting at 8.
- `queue.Config` loads it, and `wf validate` rejects a value below 1.
- A missing `native_budget` falls back to `run_budget.steps`.
- `wf budget -native` prints the effective number, so the skill does not
  re-implement the fallback.
- The `workflow-native` skill's unattended batch counts against it. This
  replaces the native-host-loop assumption that native runs spend
  `run_budget`.

### 4. Machine status files

- `native-tick.sh` writes `native/status/<machine>.yaml` on the workflow
  branch from an `EXIT` trap. The fields:
  - `machine`;
  - `last_run` (UTC);
  - `outcome`: `worked`, `idle`, `skipped` or `failed`;
  - `reason`: one line;
  - `since`: when this outcome began.
- The trap is set right after the lock is taken, so it covers every later
  exit path: `skip:` (on battery, low disk), `stop:` (dirty checkout, no
  `claude`, `wf gate` error), idle, a failed `claude`, and any `set -e`
  failure. That last one is reported as the stage that was running.
  - *(this design)* "Another session holds the lock" is the one exit path
    that writes nothing. The session that holds the lock writes the status
    when it finishes, and a second writer would race it.
- Reasons are fixed strings, so an unchanged situation gives an unchanged
  file:
  - `skip: low disk` rather than the free gigabytes;
  - `idle: gate found no work` rather than the gate's timestamped line.
- **When to push:** only when `outcome` or `reason` differs from the file on
  `origin/workflow`, or that file's `last_run` is more than 24 hours old.
  `since` carries over while the outcome is unchanged.
- **How to push:**
  - The wrapper commits `<machine>: status <outcome>` and runs
    `git push origin workflow`.
  - On a rejected push it fetches, resets the worktree, writes the file again
    and retries, up to 3 times.
  - A push that still fails is logged, and the wrapper keeps its own exit
    status.
- *(this design)* **Uncommitted session work is never reset away.** If the
  session left uncommitted changes in `.dev/workflow`, the trap first saves
  them to `$state/leftover/<run>/`: a `git diff` patch plus copies of
  untracked files.
  - The outcome becomes `failed` with
    `reason: session left uncommitted workflow changes`.
  - Only then does the trap reset the worktree.
  - This is what turned 2026-10-29's lost push into something Jeff only
    heard about by reading the log.
- Never in the file: log contents, paths outside the repo, or anything beyond
  the one-line reason (FLOW-18).
- `wf validate` checks the files:
  - `machine` matches the file name and names a machine in `hosts.yaml`;
  - `outcome` is one of the four values;
  - `reason` is one line of at most 120 characters;
  - `last_run` and `since` are set, and `since` is not after `last_run`.
- *(this design)* **The permission fix** in the same item:
  - The skill names the one form `settings.native.json` already allows:
    `git -C .dev/workflow ...` from the checkout root, never an absolute path
    or a `cd`.
  - Widening the allow list (`git -C .dev/workflow push origin HEAD:workflow`,
    git and binaries under `.scratch/` for testing a branch) widens what an
    unattended agent may do. That is Jeff's decision, so it is left out of
    this change and listed as a question below.

### 5. One morning summary notification

- A cloud scheduled task runs daily at 07:40 America/Los_Angeles. It is
  read-only, attaches buildwarden/buildwarden, and needs no device.
- Push notifications are on and email is off.
- Prompt, verbatim:

  > In a fresh clone of buildwarden/buildwarden, fetch `origin/workflow`. Read today's `digest/<date>.md` (America/Los_Angeles date): every `## Run` section, cloud and `host:*`. Also read `native/status/*.yaml` and the output of `dist/wf status` (build it with `cd tools/workflow && go build -o ../../dist/wf ./cmd/wf`). Change nothing and push nothing.
  >
  > Write at most 6 lines:
  > 1. What moved overnight, per machine.
  > 2. What needs Jeff (blocking questions, confirmations, sign-offs, failures that went back to Design).
  > 3. Any machine whose status is `failed`, or whose `last_run` is more than 2 days old.
  >
  > If nothing moved, nothing new needs Jeff, and every machine is fresh, reply exactly "Quiet night." and nothing else.

- Whether a quiet run still sends a notification is the platform's decision.

### 6. A process lane at the top of the queue

- **Queue model.** A feature gains two fields:
  - `kind`: `feature` (the default) or `process`;
  - `holds`: a list of affinity patterns such as `cloud`, `host:*` or
    `host:macos-vz`. Only a process item may have `holds`.
- **Ordering.** Process items sort ahead of every feature, whatever their
  `priority`. `priority` stays unique across the queue.
- **Holds.**
  - While a process item is `in_progress`, `wf next` gives a worker that
    matches one of its `holds` no new pick. It reports
    `held by process item <name>` instead.
  - `wf claim` refuses a new claim from a held worker in the same way.
  - A step the worker already holds a live lease on is still offered to that
    worker, can be claimed again, and can be finished.
  - Finishing the process item releases the hold.
  - *(this design)* `wf gate` treats a held host as having no ready step,
    and it does not start a backlog sweep for a machine whose hosts are all
    held.
  - A process item's own steps are never held by its own `holds`.
- **Steps.** *(this design)* A process item has its own round, `process`,
  with step prefix `x`. Its phases are numbered 1 to 4, and every step has
  `human` affinity (FLOW-14):

  | Phase | Step | Done when |
  |---|---|---|
  | 1 | `x1-design` | The design file exists on the branch. |
  | 2 | `x2-implement` | The branch is pushed (FLOW-17). |
  | 3 | `x3-sign-off` | The branch is merged into `origin/main` (FLOW-08). |
  | 4 | `x4-archive` | The design is archived on `main` with its summary filled, and the branch is deleted (FLOW-12, FLOW-16). |

  The plan named done-when checks for `x1`, `x3` and `x4` only. `x2` gets
  the parallel round's Publish check (branch pushed), because every phase's
  last step is a gate (FLOW-03).
- **Starting.** A claim on a queued process item's step starts it, by any
  worker. That differs from features, which only the cloud starts, because
  process items are human throughout.
- **Intake.** `wf new` takes `-kind process` and `-holds a,b`.
- **Skills.**
  - `workflow-tick` lists process items first in its digest, with what each
    one holds.
  - `workflow-human` shows them first.
  - `workflow-intake` knows the kind and when to use it.
- **Dogfood.**
  - This work is queued as `native-loop-followups`, with `kind: process` and
    `holds: []`.
  - `native-host-loop` is retro-recorded as a done process item that points
    at PR #64.
  - *(this design)* Both queue files go onto the workflow branch only after
    this PR merges. The `wf` on `main` rejects round `process` and `x` step
    ids, and every loop validates with the `wf` built from `main`.

### 7. Close out `native-host-loop`

- Move its six assumptions to confirmed, citing Jeff's answers of
  2026-10-29. The native-budget assumption is confirmed as superseded by
  item 3.
- Archive it with a summary once both machines have pushed a successful
  `## Run host:<machine>/...` digest section:
  - the Mac's is on the workflow branch (`68e2633`);
  - Windows is not installed yet.

  The archive therefore lands as a follow-up commit or PR.

## Questions

### Blocking

None.

### Assumed

- Proceeding with the temporary `push` trigger to prove item 1 before
  merge, removed before the PR is opened, with the `workflow_dispatch` runs
  after merge. Confirm.
- Proceeding with "lock held by another session" as the one exit that writes
  no status. Confirm.
- Proceeding with saving a session's uncommitted workflow changes under
  `$state/leftover/` and reporting `failed`, rather than having the
  wrapper commit them. Confirm.
- Proceeding with `x2-implement`'s done-when as "branch pushed". Confirm.
- Proceeding with `wf gate` honoring holds. Confirm.
- Proceeding without widening `settings.native.json`. The Mac's
  2026-10-29 run also could not run a branch binary from a `.scratch/`
  worktree. Options:
  - allow `Bash(git -C .scratch/:*)` and `Bash(.scratch/:*)`;
  - test branches in the dedicated checkout itself, one at a time;
  - leave branch tests to attended runs.

  Recommended: the first, since `make:*` already lets the agent run
  anything. Confirm.

## Design

```
config.yaml       native_budget {steps}          --> wf budget -native --> workflow-native batch
queue/<f>.yaml    kind: process, holds: [...]    --> sort first; wf next / claim / gate honor holds
native-tick.sh    EXIT trap --> native/status/<machine>.yaml on workflow
                      (push on change or >24 h; save leftovers first)
CI                integration.yml: dispatch(ref) | weekly | PR on shared paths --> make integration-test
cloud task 07:40  reads digest + native/status + wf status --> one notification
```

`queue.Load` sorts on (not process, priority). `Next` first collects the
in-progress process items whose `holds` match `Kind(worker)`, using
`path.Match`. If any match, it returns only a step already leased to that
worker, plus a `held by process item <name>` line. The phase templates gain
a `process` round, step ids gain the `x` prefix, and the phase names gain a
process table for `wf status`. `check.Phase` switches on the process round
to the four checks above.

## Cross-driver impact

No driver, seam or user-facing surface changes. The integration job builds
and runs the container driver, as `make integration-test` always has. The
`test-integration.sh` path fix changes no assertion.

## Security note

No product boundary changes (`security_boundary: none`).

Operationally:

- The status file carries only fixed one-line reasons (FLOW-18).
- The leftover save stays on the local machine.
- The CI job has `contents: read` and no secrets.
- The permission additions reach only the workflow checkout and binaries the
  session built itself.

## Validation plan

- **Tier 0.** Unit tests in `tools/workflow`:
  - native budget: loading, the fallback and validation;
  - process items: they sort first, holds block matching workers and report
    the holder, holds release when the item is done, and a claimed step is
    unaffected;
  - `wf claim` under a hold;
  - the gate under a hold;
  - process templates, ids and transitions;
  - `wf new -kind process`;
  - status-file validation.

  `make test`, `make lint` and `tools/workflow/flow18-check.sh` pass before
  each push.
- **Wrapper.** `shellcheck` passes. The trap is exercised against a scratch
  bare remote, with stand-in `claude` scripts, for each outcome:
  - worked, idle, skipped, failed (claude) and failed (stage);
  - no push when nothing changed;
  - a push after 24 hours;
  - a rejected push followed by a retry;
  - leftovers saved.
- **Tier 1.** The integration job is green on this branch's head and on
  `feat/container-inputs`, via the temporary push trigger. When the latter is
  green, the backlog entry `container-inputs-integration-test` is set to done
  with the run URL. After merge: one `workflow_dispatch` run on `main` and
  one with `ref: feat/container-inputs`.
- **Native**, not blocking: the Mac's next scheduled run writes its status
  file. Windows is covered by its first run after install.

## Implementation plan

1. `test-integration.sh` binary path and `.github/workflows/integration.yml`,
   with the temporary push trigger (TEST-02).
2. TEST-02 in `docs/development/testing.md`: say where tier 1 runs (TEST-02,
   FLOW-14).
3. `native_budget` in `queue.Config`, `wf validate` and `wf budget`, with
   tests (FLOW-13).
4. Process round, `kind` and `holds`: ordering, holds in `Next`, `claim` and
   `Gate`, templates, checks, `wf new -kind -holds`, with tests (FLOW-03,
   FLOW-04, FLOW-05, FLOW-14).
5. Status-file validation in `wf validate`, with tests (FLOW-09, FLOW-18).
6. `native-tick.sh`: the EXIT trap, push-on-change, retry and leftover save;
   `settings.native.json` additions. Tested with shellcheck and a scratch
   remote (FLOW-09, FLOW-17, FLOW-18).
7. Skills (FLOW-14):
   - `workflow-native`: `native_budget` and the exact git command forms;
   - `workflow-tick`: process items first in the digest, with their holds;
   - `workflow-human`: process items first;
   - `workflow-intake`: the kind.
8. Rules: FLOW-03 (the process lane), FLOW-05 (process items first and
   holds), FLOW-13 (the native budget); FLOW-09 for machine status (FLOW-14).
9. Workflow branch:
   - now: `native_budget: {steps: 8}`, the README schema (`kind`, `holds`,
     `native_budget`, the `native/status/` layout row);
   - after merge: the two process queue items.
10. Remove the temporary push trigger before the PR (FLOW-08).
11. The morning scheduled task (item 5), created with Jeff's approval.
12. `native-host-loop.md`: confirm its assumptions now; archive once Windows
    has pushed a run (FLOW-12).

## Consolidation (merge round)

## Rollback notes

## Archive summary
