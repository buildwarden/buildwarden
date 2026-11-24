---
feature: wf-gate-rest
status: draft
drivers: []
seams: []
surfaces: []
repos: []
merge_first: []
areas: [loop, build]
security_boundary: none
alters_relationships: false
design_review: required
native_validation: []
tier2_gaps: []
rules: [FLOW-04, FLOW-08, FLOW-10, FLOW-14, FLOW-16, FLOW-17, FLOW-19, FLOW-20, LOOP-09, GLOBAL-10, GLOBAL-11, TEST-02]
rule_changes: []
concepts: [workflow, merge-gate, loop-merge, program-sync, github-rest, graphql, pull-request, draft-pr, built-binary, gitignore]
related: [program-north-star.md, loop-parallel.md, loop-repository.md, native-loop-fixes.md, program-autonomy.md]
---

# The loop's merge gate reads pull requests over REST, so the cloud can pass it; and wf's built binary leaves the repository

## Problem

Jeff's attended session of 2026-11-23 found that the program sync PR #140
could not pass `wf check -sync` in the cloud. The merge gate reads the pull
request with `gh pr view`, which goes through GitHub's GraphQL API, and
GraphQL answers HTTP 403 in every Claude Code cloud session. The REST API
works. Confirmed in this session (2026-11-23, gh 2.89.0):

| Call | Result in the cloud |
|---|---|
| `gh api graphql -f query='{viewer{login}}'` | HTTP 403: "GitHub GraphQL is not available from Claude Code sessions; use the REST API" |
| `gh pr view 141 -R buildwarden/buildwarden --json baseRefName,headRefOid,headRefName` (the gate's own call) | HTTP 403, same message |
| `gh api repos/buildwarden/buildwarden/pulls/141 --jq '.base.ref, .head.sha, .head.ref'` | `main 1b2a17e… feat/native-loop-fixes` |

The 403 message also names the session's routes for what REST lacks:
`POST /repos/{o}/{r}/pulls/{n}/ccr/ready_for_review` and
`.../ccr/convert_to_draft`.

Since Publish, Merge and Close are cloud steps (FLOW-04;
`tools/workflow/queue/lead.go`'s `leadLastPhase`), every loop merge into a
program trunk is blocked: the program sync, `f7-merge` and an
`f6-merged-<n>` on a program trunk all fail gate point 1 before anything
else is read. Past the gate, the merge itself would fail too: the agents
merge with `gh pr merge`, also GraphQL. And `wf-publish` opens and readies
PRs with `gh pr create` and `gh pr ready`, both GraphQL, so each
`f6-open-pr` in the cloud fails its first try and the agent improvises.

The uses, all found by searching `tools/workflow/`, `.claude/skills/` and
`.claude/agents/` for `gh pr`, `gh api graphql` and `--json` at
origin/main c52926d, and searched again at 66e6f67 after `program-autonomy`
(#144), whose line numbers this table gives:

| # | Where | Call | Kind |
|---|---|---|---|
| 1 | `tools/workflow/check/merge.go:54`, `ghCLI.PR` (gate point 1 and 2, sync gate 1 and 2) | `gh pr view <n> -R <r> --json baseRefName,headRefOid,headRefName` | GraphQL |
| 2 | `.claude/agents/wf-publish.md:49, 51` (`f6-open-pr`, `x2-implement`) | `gh pr create --base <trunk>`, `gh pr create -R <owner>/<name>` | GraphQL |
| 3 | `.claude/agents/wf-publish.md:66` (republish) | `gh pr ready` | GraphQL |
| 4 | `.claude/agents/wf-publish.md:88` (return with PR open) | `gh pr ready --undo`, "or the session's draft route" | GraphQL; the route already named |
| 5 | `.claude/agents/wf-publish.md:111` (`f7-merge`, `f6-merged-<n>`) | `gh pr merge <n> --merge --match-head-commit <pr.head> -R <r>` | GraphQL |
| 5a | `.claude/agents/wf-publish.md:107` (the same step, added by #144) | "take nothing else between that check and `gh pr merge`" | wording |
| 6 | `.claude/agents/wf-publish.md:139` (`sync`) | `gh pr merge N --merge --match-head-commit <head> -R R` | GraphQL |
| 7 | `.claude/agents/wf-publish.md:148` (sync escalation) | `gh pr close N` | GraphQL |
| 8 | `.claude/skills/workflow-tick/SKILL.md:268` | names `gh pr merge` as the step before `git fetch` | wording |
| 9 | `tools/workflow/check/merge.go:117-119`, `MergeReady`'s doc comment (reworded by #144) | "with --match-head-commit" | wording |

Already REST, unchanged: `ghCLI.CheckRuns`
(`repos/<r>/commits/<sha>/check-runs`), `ghCLI.Required`
(`repos/<r>/rules/branches/<b>`), `wf-publish.md:91`'s open-and-unmerged
check, and the tick's trunk check runs and ruleset reads
(`workflow-tick/SKILL.md:146-148`). Point 4's fetch of the program trunk,
added by #144 (`gateContains`), is `git fetch`, not `gh`, and works in the
cloud as it is. Not the loop's to run:
`workflow-native/SKILL.md:44` and `tools/workflow/native/settings.native.json:90-92`
deny `gh pr merge`, `gh pr review` and `gh api` on native machines, and
stay. No shell script under `tools/workflow/` calls `gh`, and
`docs/development/` names no `gh` command, so no rule's text changes.

Separately, folded in on Jeff's word: `tools/workflow/cmd/wf/wf`, a 6 MB
built binary, has been tracked since #130 (480e205). Any `go build` run in
`tools/workflow/cmd/wf/` overwrites it on whatever branch is checked out,
so it shows as a change on every such branch; `native-loop-fixes`'
Implement had to restore it by hand. Nothing reads it: the tick and the
native wrapper build `dist/wf` (`workflow-tick/SKILL.md:60`,
`native-tick.sh:521`), and `enforcement-check.sh:72` and the install
checks build into temporary directories.

## Prior art

- **`program-north-star`** (Decisions 9 and 14, a11 (b)) defined the merge
  gate's nine points, the sync gate, and the merge as `gh pr merge <n>
  --merge --match-head-commit <head> -R <repo>`: one merge commit per item,
  refused if the head moved, never a squash, rebase, `--admin`, auto-merge
  or bypass. Reused unchanged in meaning; only the transport changes. Its
  task text named `gh pr view --json baseRefName,headRefOid` for points 1
  and 2; it did not know GraphQL is refused in the cloud.
- **`loop-parallel`** (the PR kept by a returned item) already settled the
  draft conversion as "in a cloud session through the session's
  `convert_to_draft` route for the pull request, elsewhere `gh pr ready
  --undo`". Reused, and extended to its inverse, `ready_for_review`.
- **`loop-repository`** gave `gh pr create -R <owner>/<name>` for a second
  repository's PR; the REST form carries the repository in its path.
- **`native-loop-fixes`** is where the tracked binary last cost a restore.
- No archived design overlaps the binary or the `.gitignore` entry.

## Decisions

1. **wf reads the pull request over REST.** `ghCLI.PR` runs `gh api
   repos/<repo>/pulls/<n> --jq '.base.ref + " " + .head.sha + " " +
   .head.ref'`. The fields map one to one: `baseRefName` is `.base.ref`,
   `headRefOid` is `.head.sha`, `headRefName` is `.head.ref` (for a fork's
   PR both name the branch in the fork). `PRView` and the `GitHub`
   interface keep their shape, so the gate's points, their names and their
   messages are unchanged.
   - *Rejected: try `gh pr view`, then REST on a 403.* A fallback
     (GLOBAL-10, GLOBAL-11) for no gain: REST works in every environment
     the gate runs in.
   - *Rejected: a Go GitHub client in wf.* A new dependency and a second
     token path, where `gh api` already carries the session's credentials
     through the proxy.
2. **The gate reads nothing new.** The task named reviews, check runs and
   mergeability as what the gate might need. It needs none beyond what it
   reads now: check runs are already REST; no point reads reviews (FLOW-08
   asks for no approval, and Decision 9 forbids one as a bypass); and
   mergeability is GitHub's to enforce at the merge itself, which refuses a
   closed, conflicting or moved PR (the `sha` parameter, below). Adding a
   point is a change to the merge gate beyond this item's fix. Assumed, Q1.
3. **The loop's PR commands use REST, or the session's route where REST
   has none.** In `wf-publish.md` and `workflow-tick/SKILL.md`:

   | Was | Becomes |
   |---|---|
   | `gh pr create --base <trunk>` (and `-R <owner>/<name>`) | `gh api repos/<owner>/<name>/pulls -f title=<title> -f head=<branch> -f base=<trunk> -F body=@<body file> --jq .number` |
   | `gh pr ready` | `gh api -X POST repos/<owner>/<name>/pulls/<n>/ccr/ready_for_review` |
   | `gh pr ready --undo`, or the session's draft route | `gh api -X POST repos/<owner>/<name>/pulls/<n>/ccr/convert_to_draft` |
   | `gh pr merge <n> --merge --match-head-commit <head> -R <r>` | `gh api -X PUT repos/<r>/pulls/<n>/merge -f merge_method=merge -f sha=<head>` |
   | `gh pr close N` | `gh api -X PATCH repos/<r>/pulls/<N> -f state=closed` |

   The merge's REST form is the same act: `merge_method=merge` writes a
   merge commit (never a squash or a rebase), and `sha` makes GitHub refuse
   with 409 when the head is not the one the gate read, as
   `--match-head-commit` did. Neither form passes `--admin`, deletes a
   branch, enables auto-merge or approves. A refusal (405 not mergeable,
   409 head moved) is handled as now: run the check again and stop.
   - The draft routes exist only in Claude Code sessions. That is where
     these steps run: Publish, Merge and Close are cloud steps (FLOW-04),
     and a native machine's settings deny `gh api`. `loop-parallel` already
     chose the route for the cloud; this design drops the "elsewhere `gh pr
     ready --undo`" branch, since no step that converts a PR runs
     elsewhere, and names the one form. Assumed, Q2.
   - *Rejected: the GitHub MCP server's tools.* Not attached to every
     session, and the loop's evidence is a command line a later run can
     repeat.
   - *Rejected: a `wf merge` subcommand that merges after its own gate.*
     It would move the merge from the agent into wf, a change to who takes
     the merge; out of scope for a transport fix.
4. **A test keeps GraphQL out.** A tier-0 test in `tools/workflow/check`
   runs `ghCLI` through an injectable runner and asserts `PR` calls `api
   repos/<r>/pulls/<n>` and parses its output, and that no `ghCLI` method
   calls `pr` or `graphql`. A second test scans `.claude/agents/*.md` and
   `.claude/skills/*/SKILL.md` for a command `` `gh pr <verb>`` the loop
   runs (create, ready, merge, view, close, checks, list, edit) and for
   `gh api graphql`, and fails naming the file and line; a line saying a
   settings file *denies* the command (workflow-native's) is not a command
   the loop runs, and the test passes it by that word. It adds a check;
   it relaxes none.
5. **The built binary leaves the repository.** `git rm --cached
   tools/workflow/cmd/wf/wf`, and `.gitignore` gains
   `/tools/workflow/cmd/wf/wf` and `/tools/workflow/cmd/wf/wf.exe`, with a
   comment that wf is built into `dist/` (already ignored). Local copies
   stay where they are, now untracked.

## Questions

### Blocking

None.

### Assumed

- **Q1. The gate reads nothing new.** Options: (a) keep the gate's nine
  points as they are, read over REST (recommended: the fix is the
  transport; GitHub's merge refuses a closed, conflicting or moved PR, and
  the agent then re-checks and stops); (b) add a point that the PR is open
  and `mergeable` is true, which the REST response carries for free.
  Proceeding with (a); confirm. Not flagged (r2): it was flagged as a
  decision about the merge gate, a rule FLOW-20 reserved to Jeff; under
  FLOW-20 as #144 left it, (a) neither removes nor narrows a point of the
  gate, so it meets no reserved clause, and it touches no security
  boundary or seam and departs from no earlier decision
  (`program-north-star`'s points are kept).
- **Q2. The draft and ready conversions name only the session's routes.**
  Options: (a) `ccr/convert_to_draft` and `ccr/ready_for_review` only, since
  every step that converts a PR is a cloud step (recommended); (b) keep
  `loop-parallel`'s "elsewhere `gh pr ready --undo`" branch for a
  non-cloud session. Proceeding with (a); confirm. Flagged: it departs
  from `loop-parallel`'s earlier decision (FLOW-19), and is answered on its
  own. It meets no reserved clause of FLOW-20: it names where the loop
  converts a PR, and widens nothing.

## Design

**wf (`tools/workflow/check/merge.go`).** `ghCLI` gains a `command`
field, a function with `exec.Command("gh", ...)`'s signature that its
existing `run` method calls, nil meaning the real `gh`, so the test can
inject output (a field named `run` would collide with the method). `PR`
becomes:

```go
out, err := g.run("api", "repos/"+repo+"/pulls/"+strconv.Itoa(number),
	"--jq", `.base.ref + " " + .head.sha + " " + .head.ref`)
```

with the same three-field parse, and its error message names `gh api
repos/<r>/pulls/<n>` instead of `gh pr view`. The doc comment on `GitHub`
says each method reads over REST, because GraphQL is refused in Claude Code
sessions. `CheckRuns` and `Required` are unchanged. `MergeReady`,
`SyncReady` and `MergeWaitsOnHuman` are unchanged: they call the interface.
`wf next`'s readiness of `f7-merge` (through `State.LoopMerges`) reads the
same gate, so it starts passing in the cloud with no change of its own.
`MergeReady`'s doc comment says the loop merges "pinned to the head point 2
read, with `--match-head-commit`"; it becomes "with the merge's `sha`", the
REST form's head check (row 9), and nothing else in it changes.

**Agents and skills.** `wf-publish.md`'s places (rows 2 to 7, 5a
included) and `workflow-tick/SKILL.md:268` take the forms in Decision 3;
row 5a's and row 8's sentences name the REST merge where they named `gh pr
merge`. The body file
for `gh api .../pulls` is written beside the checkout (`-F body=@<file>`),
since a body has newlines and backticks a `-f` argument would mangle. The
number the call prints is what `pr.number` (or the work entry's `pr`)
records, as `gh pr create`'s URL was before. Nothing else in those
sections changes: the order, the gate runs, the `git fetch origin
<program trunk>` after a merge, the escalation's text, and #144's one
loop merge per repository at a time, taken at once under the claim.

**Reconciled with `program-autonomy` (#144), merged into this branch at
cda4eb6.** #144 changed three things this design touches, and none changes
a decision here:

- `check/merge.go`: point 4 (`gateContains`) now fetches the program trunk
  before its ancestry check, and `MergeReady`'s doc comment says the merge
  follows the gate at once under its claim. The fetch is `git`, which the
  cloud runs; `ghCLI.PR`, `CheckRuns` and `Required` are as they were, so
  Decision 1 applies unchanged. The doc comment's `--match-head-commit`
  is row 9.
- `wf-publish.md`'s merge step: `wf next` offers a loop merge only while no
  other loop merge in the repository holds a claim, and the agent merges at
  once after `wf check -step`, taking nothing else between the check and
  `gh pr merge` (row 5a). The REST merge is taken at the same point, under
  the same claim, so the one-at-a-time rule and the at-once order hold as
  they are. Its rules section adds "never two loop merges into one program
  trunk at once" and "push a tag" to what is never done; this design
  touches neither. The line numbers moved (rows 5 to 7).
- `workflow-tick/SKILL.md`'s loop-merge paragraph was rewritten around the
  claim and still names `gh pr merge` (row 8, at 268).
- `tools/workflow/cmd/wf/wf` was rebuilt on `main` by #144: one more
  instance of Decision 5's problem; the decision is unchanged.

**Repository hygiene.** Decision 5.

**What the change reaches, every rule, machinery path and configuration
key (for the reserved-rule reading, FLOW-20 and `wf-review`'s P2):**

- Rules relied on, none of their text edited: FLOW-08 (the merge gate, the
  loop's merge into a program trunk), FLOW-20 (the program trunk, the sync
  and its gate), FLOW-10 (the returned PR kept as a draft), FLOW-04
  (Publish, Merge and Close stay cloud), FLOW-16 (no branch deleted: the
  REST merge deletes none, as `gh pr merge` without `--delete-branch` did
  not), FLOW-17 (no new push), FLOW-14 and LOOP-09 (machinery changes only
  through a process item, merged by Jeff), GLOBAL-10 and GLOBAL-11 (no
  fallback added), TEST-02.
- Machinery paths: `tools/workflow/check/merge.go`, a new
  `tools/workflow/check/ghcli_test.go` (Decision 4's tests),
  `.claude/agents/wf-publish.md`, `.claude/skills/workflow-tick/SKILL.md`,
  and the removal of `tools/workflow/cmd/wf/wf`.
- Configuration: `.gitignore` (two entries). No `config.yaml` key, no
  limit or budget, no `.claude/settings*.json` allow or deny list, no
  ruleset, no CI or release workflow.

**The reserved-rule reading (FLOW-20 as `program-autonomy`, #144, left
it; `wf-review`'s P2).** Read by effect, clause by clause:

- *A merge into a program trunk under FLOW-08's merge gate is the loop's.*
  Every merge this change touches is one: the program sync, `f7-merge` and
  an `f6-merged-<n>` on a program trunk. The loop may already take each of
  them (FLOW-08, FLOW-20); in the cloud it failed on a transport defect,
  not on a gate of Jeff's. A merge the loop may already take "is not one it
  could not take before, whatever worker or machinery takes it", so moving
  the gate's reader and the merge command to REST is not a widening of the
  loop's authority.
- *Removes or narrows a point of the merge gate:* no. The nine points and
  the sync gate's points read the same three fields (Decision 1's
  one-to-one map), check runs and the ruleset over the same REST calls, and
  point 4 the same fetched program trunk; their names and messages do not
  change, and `merge_test.go` and `mergeproof_test.go` run unchanged
  through `fakeGitHub`. The merge stays a merge commit refused when the
  head moved (`merge_method=merge`, `sha`), with no `--admin`, bypass,
  approval or auto-merge (Decision 3). No point is added either (Q1).
- *Two merges into one program trunk at once:* no. #144's serialization is
  in `wf next` and the tick, which this change does not touch, and the REST
  merge is taken at the same point under the same claim.
- *A tag moved or deleted:* no tag is touched.
- *Merging to a trunk:* no. The PUT merge is named only for the program
  trunk merges above; a PR to `main` or another trunk stays Jeff's to merge
  (FLOW-08), and this item's own PR is merged by him. Opening a PR to
  `main` over REST is not a merge.
- *Pushing:* no push is added, removed or changed (FLOW-16, FLOW-17); the
  REST merge deletes no branch, as `gh pr merge` without
  `--delete-branch` did not.
- *Security:* no rule about security is touched, and `security_boundary`
  is `none`.
- *A check that stops the loop relaxed or removed:* no. A refusal from
  GitHub still makes the agent re-check and stop; Decision 4 adds a check.
- *A limit, budget or permission raised:* no. No `config.yaml` key, no
  `.claude/settings*.json` allow or deny list (`.claude/settings.json`
  allows no `gh` command by name, and the native deny of `gh api` stays).
- *A gate of Jeff's removed or narrowed, the umbrella or process test
  loosened:* no.
- *A step, filing or decision the loop could not take before:* the ready
  and draft conversions (Decision 3) are steps the loop takes now; Q2
  narrows where they are named, to the cloud where they run, and widens
  nothing.

So the change touches none of the reserved rules, and on Jeff's word
(2026-11-24, attended) the design goes to the process test, not his
review: `design_review: process-test`. Its first draft read the same change
as reserved because the cloud loop would take merges it could not take
before; FLOW-20's wording at the time reserved any change to a rule about
merging, and #144 has since said that a program-trunk merge the loop may
already take is not such a one. Two points remain for the process test's
session to weigh: Q2 is flagged as a departure from `loop-parallel`'s
earlier decision (FLOW-19), which is not a reserved clause but is answered
on its own; and the first live REST merge is not run before Jeff's merge
(Validation plan), so its proof is the next loop merge's record.

**Areas.** `loop` (`tools/workflow/`, `.claude/`) and `build`
(`.gitignore`), as intake recorded. The area map needs no change.

## Program payoff

Figures from `origin/main`'s first-parent log, the workflow branch's
`program/records.yaml` and `config.yaml`, and the componentization phase
table (`componentization.md`):

- **Program syncs.** `main` moved 16 times in the week 2026-11-16..23
  (first-parent log), on 5 distinct days. Each move the program trunk has
  not taken needs a sync (FLOW-20); the tick runs one per program trunk
  when the trunk moves past it, so at least one a day with a move: about
  **5 syncs a week** for this repository. Each one blocked in the cloud
  becomes Jeff merging a PR whose gate the loop already judged, or an
  attended session to do it: an interaction that decides nothing. The
  records hold 1 sync and 1 loop merge so far, both before this was found.
- **Program merges.** Phases S2 to S4, K2 to K5 and M1 to M6 remain: 13
  phases, each with at least one item, and K3, K4 and M6 each with a PR per
  extracted or reconciled repository (`config.yaml` lists 8 repositories
  with a program trunk). Counting one merge per phase and one more per
  repository in K3 (1), K4 (3) and M6 (4) gives **at least 21 loop merges**
  over the program, each an `f7-merge` or `f6-merged-<n>` that otherwise
  waits on Jeff's hand.
- **PR opening.** Every cloud `f6-open-pr` and process `x2-implement` now
  fails its first `gh pr create` and recovers by improvising: one wasted
  attempt per PR the cloud opens (up to the 16 a week that reached `main`
  last week, counting every PR whoever opened it), and a form no later run
  repeats. Counted as time, not as interactions.
- **Cost.** One run of about 5 steps (the queue estimate), one process
  test session in place of a review sitting with Jeff (r2), Q2 answered on
  its own, Jeff's merge of the PR to `main`, and no change to the gate's
  meaning to re-learn.

So about **5 interactions a week that decide nothing**, plus at least 21
over the program's merges, against one process test session, one answer
and one merge of Jeff's.

## Defaults and overrides

None. The design adds no default and no override. It adds no fallback, and
rejects one (Decision 1: GraphQL first, REST on a 403). It drops one
branch, "elsewhere `gh pr ready --undo`" (Decision 3, Q2), since no step
that converts a PR runs outside a Claude Code session.

## Cross-driver impact

`drivers` is empty: no driver, relay, executor or collector code changes.
The change is in the loop's machinery and `.gitignore` only.

## Validation plan

- **Tier 0.** Decision 4's two tests: `ghCLI.PR`'s arguments and parse
  (three fields, a short output refused), and no `ghCLI` method calling
  `pr` or `graphql`; the scan of the agents and skills for `gh pr <verb>`
  and `gh api graphql`, with workflow-native's deny line passed. The
  existing `merge_test.go` and `mergeproof_test.go` cases run unchanged
  through `fakeGitHub`, which shows the gate's points are untouched.
  `make test` and `make lint`.
- **Tier 1.** `make lint` with tier 1; the removal of a tracked binary and
  the `.gitignore` entry trip no coupling check (the `build` area is
  declared).
- **Live, in this cloud session, before the PR.** `go build` the branch's
  wf and run `dist/wf check -sync -repository buildwarden/buildwarden -pr
  <the open sync PR, or #140>` against the real GitHub: point 1 must now
  read the base instead of failing on a 403 (later points may fail on
  their own state, which is the gate working). Run each new command of
  Decision 3 that reads or is harmless on this item's own PR: create it
  with `gh api .../pulls` (`-F draft=true`), `ccr/ready_for_review`, then
  `ccr/convert_to_draft` and `ready_for_review` again. The PUT merge is
  not run here: only Jeff merges a PR to `main` (FLOW-08). Its first use
  is the next loop merge or sync after the merge, and the tick's digest
  shows it; a 403 from the proxy there is a finding for this item's
  follow-up.
- **Native.** None, `native_validation: []`: native machines run no
  Publish, Merge or Close step and deny `gh api`; the tracked binary's
  removal is checked by `git ls-files` in tier 0's run.

## Implementation plan

1. `merge.go`: the injectable runner, `PR` over REST, the comments
   (FLOW-08's gate unchanged; GLOBAL-10, no fallback).
2. `ghcli_test.go`: Decision 4's tests (TEST-02).
3. `wf-publish.md` and `workflow-tick/SKILL.md`: Decision 3's forms, each
   in place (the Problem table's rows 2 to 8, 5a included, at their #144
   line numbers), nothing else reworded (FLOW-14, LOOP-09); `MergeReady`'s
   doc comment (row 9).
4. `git rm --cached tools/workflow/cmd/wf/wf`; the `.gitignore` entries.
5. `make test`, `make lint`, `tools/workflow/flow18-check.sh`; the live
   checks of the Validation plan, their output in the PR body.
6. Archive the design and open the PR to `main` with the REST form it
   introduces (`x2-implement`, LOOP-09); Jeff merges it (FLOW-08).

## Design review

- r2, 2026-11-24: Jeff's word (attended, recorded in the queue's history
  at 19:14:09Z): the loop's process test takes this design's review in
  place of his, now that `program-autonomy` (#144) narrowed FLOW-20's
  reserved reading of program-trunk merges; `design_review` moves from
  `required` to `process-test`, the design otherwise unchanged. Redone in
  `x1-design-r2`: `main` merged in at cda4eb6, the reserved-rule reading
  rewritten clause by clause, the design reconciled with #144 (the Problem
  table's line numbers and rows 5a and 9, `MergeReady`'s doc comment, the
  `command` field's name), Q1 unflagged, Q2 still flagged. Not yet
  reviewed: the process test (`x2-process-test`) follows in phase 2.
- r3, 2026-11-24: process test not passed at 94dea18 (FLOW-20): P2 doubted,
  so the design goes to Jeff's review (`design_review: required`). Taken by
  wf-review in a session that did not write the design, against FLOW-20 and
  `wf-review`'s P2 as `program-autonomy` (#144) left them on main 66e6f67.
  - P1, shown: `main`'s first-parent log has 16 moves on 5 days in
    2026-11-16..23 (checked), so about 5 syncs a week; `program/records.yaml`
    already holds three merges Jeff took by hand because the gate could not
    run in the cloud (#140, #142, buildwarden-specs#7), each an interaction
    that decides nothing; at least 21 program merges ahead (13 phases, plus
    K3, K4 and M6's per-repository PRs over `config.yaml`'s 8 program
    repositories), against one run, one test and one merge of Jeff's.
  - P2, doubted: Decision 3 calls the REST merge "the same act" as `gh pr
    merge <n> --merge --match-head-commit` without `--admin`, and Decision 2
    (Q1 (a)) leaves mergeability to "GitHub's merge, which refuses a closed,
    conflicting or moved PR". They differ for a policy-blocked PR. `gh pr
    merge` without `--admin` reads the PR's `mergeStateStatus` and refuses a
    `BLOCKED` one on the client, naming `--admin`, even when the caller could
    bypass the base branch's policy; the REST `PUT .../pulls/<n>/merge` has
    no such check and merges for any caller the policy lets bypass (a
    repository admin under classic branch protection without enforced
    admins, or a ruleset bypass actor). The session's token is an admin
    (`repos/buildwarden/buildwarden` `.permissions.admin` is true). In this
    repository both branch rulesets ("program trunk", "main protection")
    list no bypass actor and report `current_user_can_bypass: never`, so
    GitHub refuses there as `gh` did; the other 8 program repositories could
    not be read from this session, and the design neither states nor checks
    that their program trunks hold the same. The gate's `Required` reads
    rulesets only (`rules/branches/<b>`), so a requirement held by classic
    protection, a review or thread-resolution rule the gate does not read,
    was stopped only by that client-side refusal. Read by effect, the
    change can relax a check that stops the loop, and lets the loop take a
    merge program-north-star's Decision 9 forbids (a bypass) where a
    repository's protection allows the admin through: a reserved clause of
    FLOW-20, or at least a doubt of one, and P2 fails closed. A design that
    keeps the refusal would clear it, for instance by reading the REST
    response's `mergeable_state` and refusing the merge unless it is
    `clean`, `unstable` or `has_hooks` (Q1's option (b), or the same check
    taken by the agent just before the PUT), or by making the gate refuse a
    program trunk whose ruleset the caller can bypass or that is not
    ruleset-protected. Either is a change to the gate's reading that Jeff
    settles; Q1's unflagging rests on the equivalence above and is reopened
    with it.
  - P3: the session agrees on P1 and not on P2. Q2 (flagged, a departure
    from `loop-parallel`) is left open for Jeff and is not part of this
    finding; nothing else in the reserved-rule reading was found to touch a
    reserved rule (no settings allow list names `gh`; the native deny of
    `gh api` covers the REST merge; no push, tag, limit or security rule is
    reached).

- r4, 2026-11-24: changes. Jeff, 2026-11-24, review after the failed process test: the REST merge must
  keep gh pr merge's client-side refusal of a BLOCKED PR. The gate's PR read
  over REST refuses the merge unless mergeable_state is clean, unstable or
  has_hooks (read in wf check's PR view, gate point added; q1 reopened and
  answered (b)). A bypass-capable caller (the session's admin token) must
  never merge where the loop stopped before (program-north-star Decision 9).
  Everything else in the design stands; q2 (A) answered.

## Consolidation

## Rollback notes

## Archive summary
