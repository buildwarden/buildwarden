---
name: wf-publish
description: Workflow phases 0 and 6, and the loop's merges into a program trunk. Branch setup (phase 0); in Publish, merging the item's trunk, the consolidation pass, archiving the design on the branch, and opening the PR with bounded CI repair; f7-merge and the program sync on a program trunk (FLOW-20). Process items' Implement opens its PR the same way.
---

You do the branch mechanics of FLOW-03 for one feature. Rules: FLOW-05,
FLOW-08, FLOW-10, FLOW-11, FLOW-12, FLOW-15, FLOW-16, FLOW-17, FLOW-18.

An item whose queue file has `work` has a branch of the same name in each
repository an entry names, worked in its clone at
`.dev/checkouts/<owner>/<name>` (FLOW-03, FLOW-17). Each step below is done
in each of them against the item's trunk in that repository, with the fast
gate its profile file names (LOOP-01; here `make lint` and `make test`;
none in a repository with no profile), and pushed there. The item's trunk
is `dist/wf trunk -state <workflow checkout> -feature F [-repository R]`:
the repository's trunk (its `config.yaml` entry), or, for a program item
while FLOW-20 stands, its program trunk, `program/componentization`.
Below, "the item's trunk" is that branch, and `origin/<trunk>` its
remote-tracking ref; run `make enforcement TIER1_BASE=origin/<trunk>` and
`make tier1 TIER1_BASE=origin/<trunk>` against it too.

- `f0-branch-setup`: create the queue's `branch` from the item's current
  `origin/<trunk>` and push it, and each work entry's from the item's trunk
  in its repository.
- `f6-merge-main`: the PR slot is free when the tick hands you this step
  (FLOW-05). Merge the item's current `origin/<trunk>` into the branch with conflict-only
  fixes (a merge commit, never a rebase of pushed history), run `make lint`
  and `make test`, push.
- `f6-consolidate`: fill the design's Consolidation section against everything
  that landed on the item's trunk since the fork (FLOW-10) and run the FLOW-15 sweep
  (`make deadcode`, duplication, `docs/book/src/concepts/overview.md`). A
  design that declares `seams` and has no **Specification changes** section
  gets it here, from what the branch changed (LOOP-02). Fix
  wording and terminology here. Work beyond that goes back: `wf back -to 3`
  when the plan already covers it, `wf back -to 1` with the finding
  when the design must change (FLOW-06, which seeds a new design review). A
  convention that should bind future work lands in this PR only as a rule
  change the design review approved; otherwise `wf back -to 1` for a review
  of it, or a follow-up item (FLOW-10, FLOW-14). Record the trial merge's
  lines `dist/wf check` prints in the Consolidation section (FLOW-10). A regression `main` already
  has, found here, is not this branch's work: file it as the feature's
  follow-up by the `workflow-intake` skill's follow-up path (FLOW-09) and name
  it in the step's evidence. Never write into
  `.claude/` or `tools/workflow/` outside `check/` on a feature branch.
  A program item's regression its program trunk has is filed the same way.
- `f6-open-pr`: set the design's `status: archived` and fill its Archive
  summary (FLOW-12): what was built, decisions taken and rejected, and what a
  future design touching its `concepts` needs to know. Commit, push, and open
  the PR to the item's trunk without asking (FLOW-08, `gh pr create --base
  <trunk>`), and one in each repository
  whose branch has commits beyond its trunk (`gh pr create -R
  <owner>/<name>`), recorded in that work entry's `pr`; each body lists the
  tier-2 MUST findings in its repository and the item's other pull
  requests (FLOW-08). A work entry marked `first` is listed ready like any
  other; the tick's confirmation of its merge completes `f6-merged-<n>`.
  The body links the design
  (FLOW-01), the tier-2 report, and lists its MUST findings for the human to
  acknowledge at merge. A design with `rule_changes` gets a **Rule changes**
  section in the body: each rule, its change and paths, and the commit its
  approval names (FLOW-14). A design with a **Specification changes**
  section gets one in the body too, listing each specification the PR
  changes apart from the code (LOOP-02, LOOP-06). A **Follow-ups filed**
  section lists each queue item that `follows` this one, as "<slug> (follows <feature>)", or says
  none (FLOW-09). Record `pr.number` in the queue file. When the queue
  already records a PR (the item was sent back and is published again),
  push to it and mark it ready (`gh pr ready`); open no second one. Then repair CI:
  at most three attempts per distinct failure, then add a `human` step and
  stop (FLOW-11), for a program item too: its escalation, as FLOW-11 has it. A failure that does not reproduce is a flaky test:
  quarantine and file it per TEST-04, never retry until green. Set `pr.ci:
  green` only from a green check run on the PR's head, and wherever you set
  `pr.ci`, set `pr.head` to the commit the result is for (FLOW-08), for each
  pull request the item has.
  On any later push (review comments), refresh the Archive summary so it still matches the PR.
- `f6-refresh` and `x2-refresh` (any `-r<n>`): bring the branch up to date
  with the item's trunk (FLOW-08, FLOW-10). Merge `origin/<trunk>` (a merge commit,
  conflict-only fixes, never a rebase), run `make lint`, `make test` and
  `make deadcode`, and write the consolidation pass over what landed since
  the branch last merged its trunk, with the FLOW-15 sweep, as a dated entry in
  the Consolidation section naming the trunk commit merged (a process item:
  no review, FLOW-10). Refresh the Archive summary if the PR's content
  changed and run `tools/workflow/flow18-check.sh`. A gate failure the
  item's trunk has too is not this branch's work: record it with
  `dist/wf main -state <workflow checkout> [-trunk <program trunk>] -sha
  <merged trunk commit> -ci red -gates`,
  file the fix as the item's follow-up (FLOW-09) unless `main.yaml` already
  names one, record it with `-followup`, and return the step released.
  Work beyond conflict fixes and wording goes back as in `f6-consolidate`;
  before returning, convert the PR to a draft (`gh pr ready --undo`, or the
  session's draft route), which keeps it from being taken as ready
  (FLOW-10). Before the push, with a PR recorded: check that the PR is still
  open and unmerged (`gh api repos/<repo>/pulls/<n>`) and that
  `origin/<branch>` is still the commit you merged onto. If it merged or
  the branch is gone, discard the local merge, push nothing, and return
  `merged meanwhile`. Otherwise push, and with a PR recorded wait for CI on
  the new head and set `pr.ci` and `pr.head`. A feature in Publish whose PR
  is not open yet only pushes: `f6-open-pr` follows.
- `x2-implement` on a process item: after the implementation (by
  `wf-implement`), archive the design on the branch and open the PR as in
  `f6-open-pr`. A process item's PR always goes to `main` (LOOP-09).
- `f7-merge`, and an `f6-merged-<n>` whose work entry is on a program
  trunk (FLOW-08, FLOW-20): `wf next` offers them only while the merge gate
  passes and no other loop merge in the same repository holds a claim: the
  loop merges into a program trunk one pull request at a time per
  repository. Under the claim, merge at once after the gate passes, pinned
  to the head it checked; `wf check -step` fetches the program trunk and
  runs point 4 against its head as origin has it then, so take nothing else
  between that check and `gh pr merge`, and never start a second merge in
  that repository while this one is open. For each work entry the step covers (at `f7-merge` each one not
  yet merged, in turn; at `f6-merged-<n>` that entry's), run `dist/wf check
  -state <workflow checkout> -repo <code checkout> -checkouts .dev/checkouts
  -feature F -step S -repository <owner>/<name>`; on a pass, `gh pr merge
  <n> --merge --match-head-commit <pr.head> -R <owner>/<name>`, then `git
  fetch origin <program trunk>` in that repository's checkout (never
  naming the branch, which GitHub deleted on the merge: a fetch of a
  missing ref fails whole), so the merge proof reads the merged program
  trunk and not the ref from before the merge. `wf check -step` refuses a
  `-repository` the step does not cover. Return to the tick, which runs
  `dist/wf done` and `dist/wf record -kind merge` for each merge. On a refusal from GitHub,
  run the check again and stop. A point 5 failure on a protected finding
  (GLOBAL-06, a SEC rule, RELAY-01 to RELAY-03, RELAY-05, or what FLOW-20
  says never breaks) returns `back-to-design` with the finding, recorded
  `-kind protected`. Point 7 passes a relay-clause-only security note on
  its `security-note -ref design` record and Review's `security-note -ref
  diff` record on the head (a14); without them it is a posture change and
  waits for Jeff's `security-ack` record.
- `sync` (the program sync, FLOW-20; no queue item): `dist/wf sync-program
  -state <workflow checkout> -repo <code checkout> -checkouts .dev/checkouts
  -repository R` names the branch `sync/program-<main sha>`. Cut it locally
  from `origin/<program trunk>` and merge `origin/<trunk>` into it (a merge
  commit, conflict-only fixes, nothing else on the branch). Only once the
  merge and any conflict resolution succeed locally: run the fast gate,
  record the head with `dist/wf sync-program -state <workflow checkout>
  -repo <code checkout> -checkouts .dev/checkouts -repository R -worker <W>
  -head <the merge commit>` (it refuses a head that is not that merge and
  nothing more, and one whose resolution edits a rule, machinery or
  rule-running workflow path (FLOW-01); that refusal is a conflict you
  cannot resolve, and escalates as below. It prints `review` with the
  report path when the merge resolved a conflict), then push the branch and open its PR to the
  program trunk. A clean merge: once `dist/wf check -state <workflow
  checkout> -repo <code checkout> -checkouts .dev/checkouts -sync
  -repository R -pr N` passes, merge it with `gh pr merge N --merge
  --match-head-commit <head> -R R` and `git fetch origin <program trunk>`.
  A resolved conflict: return `review` with the head and the report path
  `wf sync-program` printed (`reports/sync/<head12>.json`); the tick has
  `wf-review` review the resolution and dispatches you again to merge.
  Dispatched to merge, merge only once `wf check -sync` passes, which
  reads that report and fails while it is missing or has an open MUST
  finding; otherwise stop and return. A conflict you cannot resolve
  escalates (FLOW-20): push nothing, so no sync branch is left on the
  remote; close the sync PR if one opened (`gh pr close N`, never
  `--delete-branch`), and if a `sync/program-<main sha>` branch is on the
  remote all the same, delete nothing: put the exact `git push origin
  --delete sync/program-<main sha>` command (`git -C
  .dev/checkouts/<owner>/<name> ...` for another repository) in the
  escalation's text for Jeff to run. File the item with
  `dist/wf new -state <workflow checkout> -worker <W> -feature
  program-sync-<main sha> -program -repos R -escalate "<the conflicting
  paths and both sides' commits>"` (and the title, priority and estimate
  `wf new` asks), and return both for the tick to record.

Approve or enable auto-merge on no PR (FLOW-08, LINT-03). Merge one only at
`f7-merge`, an `f6-merged-<n>` on a program trunk, or the program sync,
into a program trunk, with `--merge --match-head-commit`, after `wf check`
passes; never into a trunk, never a squash or rebase, never with
`--admin`, and never dismiss a finding; never two loop merges into one
program trunk at once. Never push `main` or a program trunk, force-push,
push a tag (the tick records the tags `wf tag-next` names as due, and a
native session creates them), or delete a remote branch or a tag
(FLOW-17); GitHub deletes the branch on merge. Before any push, run `tools/workflow/flow18-check.sh` and stop
if it fails (FLOW-18).

On a native machine (a led step, FLOW-04), a branch is pushed only with
`tools/workflow/native/native-push.sh <name>`, or `native-push.sh <name>
<owner>/<repo>` for a branch in a second repository's clone, whose worktree
is `.scratch/<owner>/<repo>/<name>`, made with `git -C
.dev/checkouts/<owner>/<repo> worktree add`; and committed with `git -C
<worktree> commit -F ../<name>.msg`, the message written beside the
worktree, never `-m` (FLOW-17).
