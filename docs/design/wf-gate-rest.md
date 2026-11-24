---
feature: wf-gate-rest
status: archived
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
rule_changes:
  - {rule: FLOW-08, change: amend, paths: [docs/development/process.md, tools/workflow/check/merge.go, tools/workflow/check/merge_test.go]}
concepts: [workflow, merge-gate, loop-merge, program-sync, github-rest, graphql, pull-request, draft-pr, built-binary, gitignore, mergeable-state, branch-protection-bypass]
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
`docs/development/` names no `gh` command, so the transport changes no
rule's text; point 10 (Decision 2) amends FLOW-08's list of the gate's
points (Rule changes).

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
  or bypass. Reused: the transport changes, and point 10 (Decision 2)
  keeps its "never a bypass" now that the merge's client-side refusal of
  a blocked PR is gone with `gh pr merge`. Its
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

1. **wf reads the pull request over REST, its mergeability with it.**
   `ghCLI.PR` runs `gh api repos/<repo>/pulls/<n> --jq '.base.ref + " " +
   .head.sha + " " + .head.ref + " " + (.mergeable_state // "unknown")'`.
   The first three fields map one to one: `baseRefName` is `.base.ref`,
   `headRefOid` is `.head.sha`, `headRefName` is `.head.ref` (for a fork's
   PR both name the branch in the fork). The fourth is new (r3): GitHub's
   `mergeable_state`, a null read as `unknown`, which `PRView` carries as
   `Mergeable` for point 10 (Decision 2). The `GitHub` interface keeps its
   shape, so points 1 to 9, their names and their messages are unchanged.
   - *Rejected: try `gh pr view`, then REST on a 403.* A fallback
     (GLOBAL-10, GLOBAL-11) for no gain: REST works in every environment
     the gate runs in.
   - *Rejected: a Go GitHub client in wf.* A new dependency and a second
     token path, where `gh api` already carries the session's credentials
     through the proxy.
2. **The gate gains point 10: GitHub reports the PR mergeable (r3, Jeff's
   r4 finding; Q1 answered (b)).** A new point of the merge gate and of the
   sync gate, not an extension of an existing one. Point 3 is about check
   runs and the checks the ruleset requires; `blocked` also covers what no
   point reads: a required review or conversation resolution, classic
   branch protection, any requirement the caller could bypass. Folding it
   into point 3 would make that point's name and FLOW-08's wording of it
   false, so the gate names it on its own, as `merge gate 10: GitHub
   reports the PR mergeable (FLOW-08)` and `sync gate 10: ...` (FLOW-20).
   - **What it reads.** The `mergeable_state` from point 1's REST read
     (Decision 1), so no second call when GitHub has computed it. It
     passes only on `clean`, `unstable` or `has_hooks`, the states in which
     `gh pr merge` without `--admin` merges; it fails on `blocked`,
     `dirty`, `behind`, `draft` and every other value, naming the state.
     `unstable` (a check that is not required failed) still meets point 3,
     which refuses any failed run, so the gate passes it only where point 3
     also holds.
   - **While GitHub computes it.** GitHub answers `mergeable: null` and
     `mergeable_state: unknown` until a background job has computed it,
     which a read of the PR starts. On `unknown` the gate reads the PR
     again, up to 5 reads in all, 2 seconds apart (about 8 seconds at
     most), and uses the first read that is not `unknown`; a re-read whose
     head is not the one point 2 read fails the point ("head moved"), and
     a 5th `unknown` fails it ("GitHub has not computed mergeability"). The
     agent then stops as on any failed gate; the next tick or step runs the
     gate again. The loop never merges on `unknown`. The count and the
     interval are constants, assumed, Q3.
   - **Why it keeps the refusal.** It is the client-side check `gh pr
     merge` made, now made by the gate before the merge: a caller whose
     token could bypass the base branch's policy (the session's admin
     token, a ruleset bypass actor, an admin under classic protection
     without enforced admins) sees `blocked` and does not merge, as with
     `gh` it was refused and told to pass `--admin`. So the loop never
     merges where it stopped before (`program-north-star` Decision 9).
     Between the gate and the PUT the window is the one `gh pr merge` had
     between reading `mergeStateStatus` and its merge mutation, taken at
     once under the claim (#144); the head is pinned by `sha` (Decision 3).
   - *Rejected: Q1 (a), the gate reads nothing new.* The process test (r3)
     showed the REST merge merges a `blocked` PR for a caller that can
     bypass, so (a) relaxes a check that stopped the loop.
   - *Rejected: the agent reads `mergeable_state` just before the PUT.* A
     second reader outside wf, in prose; the gate is wf's, is tested and
     already runs at that moment.
   - *Rejected: refuse a program trunk whose ruleset the caller can bypass
     or that has no ruleset.* It reads only rulesets and the caller's own
     bypass; a review rule or classic protection would still need GitHub's
     verdict, which `mergeable_state` is.
   - *Rejected: merge on `unknown` and let GitHub refuse.* GitHub's PUT
     does not refuse a bypassing caller, which is the defect.
3. **The loop's PR commands use REST, or the session's route where REST
   has none.** In `wf-publish.md` and `workflow-tick/SKILL.md`:

   | Was | Becomes |
   |---|---|
   | `gh pr create --base <trunk>` (and `-R <owner>/<name>`) | `gh api repos/<owner>/<name>/pulls -f title=<title> -f head=<branch> -f base=<trunk> -F body=@<body file> --jq .number` |
   | `gh pr ready` | `gh api -X POST repos/<owner>/<name>/pulls/<n>/ccr/ready_for_review` |
   | `gh pr ready --undo`, or the session's draft route | `gh api -X POST repos/<owner>/<name>/pulls/<n>/ccr/convert_to_draft` |
   | `gh pr merge <n> --merge --match-head-commit <head> -R <r>` | `gh api -X PUT repos/<r>/pulls/<n>/merge -f merge_method=merge -f sha=<head>` |
   | `gh pr close N` | `gh api -X PATCH repos/<r>/pulls/<N> -f state=closed` |

   The merge's REST form is the same act once the gate's point 10 has
   passed (Decision 2): `merge_method=merge` writes a merge commit (never a
   squash or a rebase), and `sha` makes GitHub refuse with 409 when the
   head is not the one the gate read, as `--match-head-commit` did. The
   PUT alone lacks `gh pr merge`'s client-side refusal of a `BLOCKED` PR
   (r3); point 10, run at once before it, is that refusal, so the PUT is
   taken only where `gh pr merge` would have merged. Neither form passes
   `--admin`, deletes a branch, enables auto-merge or approves. A refusal
   (405 not mergeable, 409 head moved) is handled as now: run the check
   again and stop. `wf-publish.md`'s merge and sync steps say the PUT is
   taken only after `wf check` passed, point 10 included, and never on a
   failed or `unknown` point 10.
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

- **Q1. Does the gate read the PR's mergeability?** Options: (a) keep the
  gate's nine points as they are, read over REST; (b) add a point that
  GitHub reports the PR mergeable, from the same REST response's
  `mergeable_state`. Proceeded with (a) in r1 and r2; reopened by the
  process test (r3), which found (a) drops `gh pr merge`'s refusal of a
  `BLOCKED` PR for a caller that can bypass. **Answered (b)** (Jeff,
  2026-11-24, r4): point 10 passes only on `clean`, `unstable` or
  `has_hooks`, re-reads a bounded number of times while `unknown`, and
  never merges on `unknown` (Decision 2).
- **Q2. The draft and ready conversions name only the session's routes.**
  Options: (a) `ccr/convert_to_draft` and `ccr/ready_for_review` only, since
  every step that converts a PR is a cloud step (recommended); (b) keep
  `loop-parallel`'s "elsewhere `gh pr ready --undo`" branch for a
  non-cloud session. Proceeding with (a); confirm. Flagged: it departs
  from `loop-parallel`'s earlier decision (FLOW-19), and is answered on its
  own. It meets no reserved clause of FLOW-20: it names where the loop
  converts a PR, and widens nothing. **Answered (a)** (Jeff, 2026-11-24,
  attended): `loop-parallel`'s "elsewhere `gh pr ready --undo`" branch is
  dropped.
- **Q3. How long point 10 waits on `unknown`.** Options: (a) up to 5 reads
  2 seconds apart, constants in `merge.go` (recommended: GitHub usually
  computes mergeability within a few seconds of the first read, the wait
  is short enough for `wf next`, which runs the gate through
  `State.LoopMerges`, and a later tick retries anyway); (b) one read,
  refusing on `unknown` and leaving the retry to the next tick (a wasted
  merge step on most first reads); (c) a `config.yaml` key (a setting no
  one needs to tune, GLOBAL-09). Proceeding with (a); confirm. Not
  flagged: either way the gate never merges on `unknown`, so the choice
  touches no security boundary or seam, and departs from no earlier
  decision; Jeff named the bound, not its size (r4).

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

with a four-field parse (the fourth into `PRView.Mergeable`), and its
error message names `gh api repos/<r>/pulls/<n>` instead of `gh pr view`.

Point 10 is `(*checker).gateMergeable(view PRView)`, called by `MergeReady`
after `gateHead` (only when points 1 and 2 held, as the later points are)
and by `SyncReady` after its point 2 checks. It passes on `view.Mergeable`
in `clean`, `unstable`, `has_hooks`; on `unknown` it sleeps
`mergeableWait` (2 s, a package variable the tests set to zero) and calls
`github().PR` again, at most `mergeableReads` (5) reads in all counting
point 1's, and fails when a re-read's head differs from `view.Head` or the
last read is still `unknown`; any other state fails at once. Its message
names the state and the number of reads, e.g. `mergeable_state blocked`
or `mergeable_state unknown after 5 reads`. `MergeReady`'s and
`SyncReady`'s doc comments list point 10. The doc comment on `GitHub`
says each method reads over REST, because GraphQL is refused in Claude Code
sessions. `CheckRuns` and `Required` are unchanged. `MergeWaitsOnHuman` is
unchanged; `MergeReady` and `SyncReady` change only by point 10's call.
`wf next`'s readiness of `f7-merge` (through `State.LoopMerges`) reads the
same gate, so it starts passing in the cloud with no change of its own,
and offers `f7-merge` only while point 10 holds; on `unknown` it waits
the bounded re-reads (Q3), at most about 8 seconds per loop merge read.
`MergeReady`'s doc comment says the loop merges "pinned to the head point 2
read, with `--match-head-commit`"; it becomes "with the merge's `sha`", the
REST form's head check (row 9), and nothing else in it changes.

**Agents and skills.** `wf-publish.md`'s places (rows 2 to 7, 5a
included) and `workflow-tick/SKILL.md:268` take the forms in Decision 3;
row 5a's and row 8's sentences name the REST merge where they named `gh pr
merge`, and the merge and sync steps say the PUT follows only a passing
`wf check`, point 10 included, never a failed or `unknown` one (Decision
3). The body file
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

- Rules: FLOW-08 amended, its gate's list gaining point 10 and the sync's
  sentence naming it (Rule changes). Relied on, their text not edited:
  FLOW-20 (the program trunk, the sync
  and its gate), FLOW-10 (the returned PR kept as a draft), FLOW-04
  (Publish, Merge and Close stay cloud), FLOW-16 (no branch deleted: the
  REST merge deletes none, as `gh pr merge` without `--delete-branch` did
  not), FLOW-17 (no new push), FLOW-14 and LOOP-09 (machinery changes only
  through a process item, merged by Jeff), GLOBAL-10 and GLOBAL-11 (no
  fallback added), TEST-02.
- Machinery paths: `docs/development/process.md` (FLOW-08's text),
  `tools/workflow/check/merge.go`, `tools/workflow/check/merge_test.go`
  (point 10's cases and the fixture's `Mergeable`), a new
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
- *Removes or narrows a point of the merge gate:* no; it adds one. Points
  1 to 9 and the sync gate's points read the same three fields (Decision
  1's one-to-one map), check runs and the ruleset over the same REST calls,
  and point 4 the same fetched program trunk; their names and messages do
  not change, and `merge_test.go`'s and `mergeproof_test.go`'s cases run
  as they are, with `fakeGitHub`'s fixture reading `clean`. Point 10
  (Decision 2) keeps the refusal `gh pr merge` made on the client for a
  `BLOCKED` PR, which the PUT alone dropped (r3): with it, the loop takes
  no merge it would not have taken with `gh`, whatever the caller could
  bypass. The merge stays a merge commit refused when the head moved
  (`merge_method=merge`, `sha`), with no `--admin`, bypass, approval or
  auto-merge (Decision 3).
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
- *A check that stops the loop relaxed or removed:* no. `gh pr merge`'s
  client-side refusal moves into the gate as point 10, which also refuses
  `unknown`; a refusal from GitHub still makes the agent re-check and
  stop; Decision 4 adds a check.
- *A limit, budget or permission raised:* no. No `config.yaml` key, no
  `.claude/settings*.json` allow or deny list (`.claude/settings.json`
  allows no `gh` command by name, and the native deny of `gh api` stays).
- *A gate of Jeff's removed or narrowed, the umbrella or process test
  loosened:* no.
- *A step, filing or decision the loop could not take before:* the ready
  and draft conversions (Decision 3) are steps the loop takes now; Q2
  narrows where they are named, to the cloud where they run, and widens
  nothing.

So the change touches none of the reserved rules by relaxing one; it
amends FLOW-08, a loop rule, by adding a gate point, which a process item
may do (FLOW-14, LOOP-09) and which needs a review (FLOW-19, a design with
`rule_changes`). `design_review` is `required`: the process test (r3)
doubted P2, and Jeff's review (r4) asked for point 10 and re-reviews only
this delta. The first live REST merge is not run before Jeff's merge
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
- **Cost.** One run of about 5 steps (the queue estimate) plus this
  redesign (r3, one step); the process test that did not pass (r3) and
  Jeff's two short reviews, r4 and the re-review of this delta; Q2 and Q3
  answered; Jeff's merge of the PR to `main`. One more point of the gate
  to learn, which says in its message what GitHub reported, and up to
  about 8 seconds of waiting on a loop merge GitHub has not computed yet
  (Q3).
- **What point 10 adds besides the fix.** The three merges Jeff took by
  hand (#140, #142, buildwarden-specs#7, r3's P1) would each have passed
  it; a PR GitHub reports `blocked` or `dirty` now stops at the gate with
  GitHub's reason instead of at the merge's 405.

So about **5 interactions a week that decide nothing**, plus at least 21
over the program's merges, against one process test, two reviews, two
answers and one merge of Jeff's.

## Defaults and overrides

One default, no override: point 10 re-reads an `unknown` `mergeable_state`
up to 5 reads, 2 seconds apart, constants in `merge.go` (Decision 2,
assumed as Q3; not a `config.yaml` key, GLOBAL-09). The design adds no
fallback: after the last read the gate refuses, and the merge is never
taken on `unknown`. It
rejects one (Decision 1: GraphQL first, REST on a 403). It drops one
branch, "elsewhere `gh pr ready --undo`" (Decision 3, Q2), since no step
that converts a PR runs outside a Claude Code session.

## Cross-driver impact

`drivers` is empty: no driver, relay, executor or collector code changes.
The change is in the loop's machinery and `.gitignore` only.

## Rule changes

**FLOW-08 (amend).** Severity MUST and check `automated`, before and after.
Paths: `docs/development/process.md` (the rule's text),
`tools/workflow/check/merge.go` (its enforcement, `MergeReady` and
`SyncReady`), `tools/workflow/check/merge_test.go` (its tests).

Current, the gate's list and the sync's sentence:

> 8. each pull request its design names to merge first has merged;
> 9. the rule-change clearance passes.
>
> The *program sync* is a pull request whose head merges the trunk into
> the program trunk. The loop merges it once points 1 to 4 pass, where the
> head in point 2 is the one the sync step recorded and point 4 also needs
> the head to contain the trunk, and point 5 on its conflict-resolution
> commit when it had one (FLOW-20).

Proposed:

> 8. each pull request its design names to merge first has merged;
> 9. the rule-change clearance passes;
> 10. GitHub reports the pull request mergeable by its base branch's
>     protection for any caller, read before the merge (`mergeable_state`
>     `clean`, `unstable` or `has_hooks`), never while it is still being
>     computed.
>
> The *program sync* is a pull request whose head merges the trunk into
> the program trunk. The loop merges it once points 1 to 4 and 10 pass,
> where the head in point 2 is the one the sync step recorded and point 4
> also needs the head to contain the trunk, and point 5 on its
> conflict-resolution commit when it had one (FLOW-20).

The rest of FLOW-08 is unchanged.

*Why the feature needs it.* Moving the merge from `gh pr merge` to the
REST PUT (Decision 3) drops the client-side refusal of a `BLOCKED` PR, and
the session's token can bypass (r3). The gate is FLOW-08's, so the point
that keeps the refusal is written into its list, not only into wf.

*What the enforcement change does.* `MergeReady` and `SyncReady` gain
`gateMergeable` (Design), which reads the `mergeable_state` from point 1's
REST read, re-reads a bounded number of times while it is `unknown`, and
fails the point on anything but `clean`, `unstable` or `has_hooks`. `wf
check -step` and `wf check -sync` print it as `merge gate 10` and `sync
gate 10`; `wf next` offers a loop merge only while it holds. It adds a
check and relaxes none.

## Validation plan

- **Tier 0.** Decision 4's two tests: `ghCLI.PR`'s arguments and parse
  (four fields, a null `mergeable_state` read as `unknown`, a short output
  refused), and no `ghCLI` method calling `pr` or `graphql`; the scan of
  the agents and skills for `gh pr <verb>` and `gh api graphql`, with
  workflow-native's deny line passed. Point 10, in `merge_test.go`
  through `fakeGitHub`, which gains a queue of states its `PR` returns in
  turn, with `mergeableWait` zero, each for `MergeReady` and for
  `SyncReady`:
  - a `blocked` PR refused, naming `blocked`, with every other point
    passing (the bypass case: the fake answers as GitHub does for any
    caller);
  - a `dirty` PR refused, naming `dirty`;
  - an `unknown` PR refused after 5 reads, naming `unknown`, and `PR`
    called exactly 5 times;
  - `unknown` then `clean` passes on the 2nd read;
  - `unknown` then a read with another head refused ("head moved");
  - `behind` and `draft` refused; `clean`, `unstable` and `has_hooks`
    each pass.
  The existing `merge_test.go` and `mergeproof_test.go` cases run as they
  are, the fixture reading `clean`, which shows points 1 to 9 are
  untouched. `make test` and `make lint`.
- **Tier 1.** `make lint` with tier 1; the removal of a tracked binary and
  the `.gitignore` entry trip no coupling check (the `build` area is
  declared).
- **Live, in this cloud session, before the PR.** `go build` the branch's
  wf and run `dist/wf check -sync -repository buildwarden/buildwarden -pr
  <the open sync PR, or #140>` against the real GitHub: point 1 must now
  read the base instead of failing on a 403 (later points may fail on
  their own state, which is the gate working), and point 10 must print
  the state GitHub reports. Run each new command of
  Decision 3 that reads or is harmless on this item's own PR: create it
  with `gh api .../pulls` (`-F draft=true`), `ccr/ready_for_review`, then
  `ccr/convert_to_draft` and `ready_for_review` again, reading
  `mergeable_state` with Decision 1's `--jq` after each: `draft` while a
  draft, then what `main`'s protection gives (`blocked` while its required
  checks or review are pending), the live case of a refused state. The PUT
  merge is
  not run here: only Jeff merges a PR to `main` (FLOW-08). Its first use
  is the next loop merge or sync after the merge, and the tick's digest
  shows it; a 403 from the proxy there is a finding for this item's
  follow-up.
- **Native.** None, `native_validation: []`: native machines run no
  Publish, Merge or Close step and deny `gh api`; the tracked binary's
  removal is checked by `git ls-files` in tier 0's run.

## Implementation plan

1. `merge.go`: the injectable runner, `PR` over REST with
   `mergeable_state` (Decision 1), point 10's `gateMergeable` in
   `MergeReady` and `SyncReady` with its bounded re-reads (Decision 2,
   Q3), the comments (FLOW-08 as amended; GLOBAL-10, no fallback).
2. `ghcli_test.go`: Decision 4's tests; `merge_test.go`: the fake's queue
   of states, the fixture's `clean`, and point 10's cases (Validation
   plan) (TEST-02).
3. `process.md`: FLOW-08's amendment, worded as Rule changes approves it
   and nothing else (FLOW-14, LOOP-09).
4. `wf-publish.md` and `workflow-tick/SKILL.md`: Decision 3's forms, each
   in place (the Problem table's rows 2 to 8, 5a included, at their #144
   line numbers), and the merge and sync steps' "only after a passing
   `wf check`, point 10 included", nothing else reworded (FLOW-14,
   LOOP-09); `MergeReady`'s doc comment (row 9).
5. `git rm --cached tools/workflow/cmd/wf/wf`; the `.gitignore` entries.
6. `make test`, `make lint`, `tools/workflow/flow18-check.sh`; the live
   checks of the Validation plan, their output in the PR body.
7. Archive the design and open the PR to `main` with the REST form it
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
  Redone in `x1-design-r3`: Decision 2 rewritten as gate point 10, a new
  point of the merge gate and the sync gate rather than an extension of
  point 3 (its reasons there), with bounded re-reads on `unknown` (Q3,
  assumed) and never a merge on `unknown`; Decision 1 reads
  `mergeable_state`; Decision 3 says the PUT follows point 10; Q1 answered
  (b), Q2 (a); FLOW-08 amended (`rule_changes`, Rule changes); the
  reserved-rule reading, tests, Implementation plan and payoff cost
  updated. Not yet reviewed: Jeff re-reviews this delta from 947bc42.

- r5, 2026-11-24: approved at 692681a. Jeff re-reviewed r4's delta (gate
  point 10, Decisions 1 to 3, the FLOW-08 amendment and q3) in the
  attended session.
  - rule change FLOW-08 approved at 692681a
  - q1 (b), the mergeable_state check (r4); q2 (A) (Jeff, 2026-11-24);
    q3 (A), 5 reads 2 s apart.
  - read at 692681a
  - sitting: 19:40-19:42 UTC, asks: 3

## Consolidation

## Rollback notes

## Archive summary

Implemented 2026-11-24 in `x2-implement` (process item, attended), after
Jeff approved r3 at 692681a (r5).

- **Built.** `ghCLI.PR` reads `gh api repos/<r>/pulls/<n>` (base, head,
  head branch and `mergeable_state`, a null read as `unknown`) through an
  injectable `command`; `CheckRuns` and `Required` were already REST.
  `MergeReady` and `SyncReady` end with point 10, `gateMergeable`: it
  passes on `clean`, `unstable` or `has_hooks`, re-reads an `unknown`
  state up to 5 reads 2 seconds apart, and fails on any other state, on a
  re-read with another head, and on a 5th `unknown`. FLOW-08 lists point
  10 and the program sync names it. `wf-publish.md` opens PRs with `gh api
  .../pulls`, readies and drafts them with the session's
  `ccr/ready_for_review` and `ccr/convert_to_draft`, merges with `PUT
  .../pulls/<n>/merge` (`merge_method=merge`, `sha`) only after a passing
  `wf check`, point 10 included, and closes with `PATCH state=closed`; the
  tick skill names the merge's `PUT`. `tools/workflow/cmd/wf/wf` is
  untracked and ignored, with `wf.exe`.
- **Decisions taken:** REST only, no GraphQL-then-REST fallback; point 10
  as a new point of both gates rather than part of point 3 (Q1 (b), Jeff,
  r4), so a caller able to bypass the branch's protection never merges
  where `gh pr merge` refused; the session's routes as the only draft and
  ready forms (Q2 (a)); 5 reads 2 s apart as constants (Q3 (a)).
- **Rejected:** a Go GitHub client; the GitHub MCP tools; a `wf merge`
  subcommand; the agent reading `mergeable_state` before the PUT; refusing
  program trunks whose ruleset the caller can bypass; merging on
  `unknown`.
- **Validated.** Tier 0: `ghcli_test.go` (the REST call and its four-field
  parse, no `ghCLI` method running `gh pr` or `graphql`, and a scan of
  `.claude/agents/*.md` and `.claude/skills/*/SKILL.md` that fails on any
  `gh pr <verb>` or `gh api graphql` outside a line saying a settings file
  denies it); point 10's cases in `merge_test.go` for both gates, the
  existing cases unchanged with the fixture reading `clean`. Live in the
  cloud: the branch's `wf check -sync ... -pr 140` read point 1 over REST
  (the 403 is gone) and point 10 refused the merged PR's null state as
  `unknown` after 5 reads. The PR to `main` was opened with the REST form.
  The PUT merge first runs at the next loop merge or sync after Jeff's
  merge.
- **For a future design touching these concepts.** GraphQL is refused in
  Claude Code sessions; every `gh` call the loop makes is REST or a
  session `ccr/` route, and `TestAgentsUseREST` keeps it so. The REST merge
  does not refuse a policy-blocked PR for a caller that can bypass it;
  point 10 is that refusal, and any new merge path must run it first.
  `mergeable_state` is null on a merged or closed PR, which point 10 reads
  as `unknown` and refuses.
