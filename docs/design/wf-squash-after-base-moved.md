---
feature: wf-squash-after-base-moved
status: draft
drivers: []
seams: []
surfaces: []
security_boundary: none
alters_relationships: false
design_review: required
native_validation: []
tier2_gaps: []
rules: [FLOW-03, FLOW-08, FLOW-09, FLOW-12, FLOW-14, FLOW-16, FLOW-17, GLOBAL-01, GLOBAL-09, GLOBAL-10, GLOBAL-11, TEST-01]
rule_changes: []
concepts: [workflow, merge-detection, squash-merge, patch-id, branch-cleanup, process-item]
related: [wf-squash-merged-pr.md, wf-merged-counts-as-pushed.md]
---

# wf's merged check accepts a squash made after main moved

## Problem

On 2026-11-11, design-principles' PR #104 was squash-merged as `beb51ba`
after #105 (`f861315`) had already changed `main`. The branch head is
`42d95f0`, and its merge base with `main` is `49e5354`. `beb51ba` carries
exactly the branch's change:

| Diff | `patch-id --stable` | `patch-id --verbatim` |
|---|---|---|
| `git diff 49e5354 42d95f0` (the branch's change) | `fdc1cfd3…` | `43f1a18b…` |
| `git diff f861315 beb51ba` (the squash's change) | `fdc1cfd3…` | `43f1a18b…` |

The two diffs differ in two lines only: the `index` line of
`docs/development/process.md`, and that file's last hunk header
(`@@ -565,6` against `@@ -566,6`), because #105 added a line above it. The
trees differ (`1384cdd` against `bd79a84`), because `beb51ba` also holds #105.

x3-merge's "merged by a human (FLOW-08)" check still fails.
`check.merged()` (`tools/workflow/check/check.go:251`) collects the commits
on `origin/main` that name the PR (`:255-256`). For each branch ref still
present, it calls `refMerged()` (`:290`), which accepts the ref when it is an
ancestor of `origin/main` (`:291`) or when a PR commit's tree equals the
ref's tree (`:298-305`). Otherwise it fails with "is not contained in
origin/main, and no PR commit has its tree" (`:308-309`). The archived
design `wf-squash-merged-pr` chose tree equality on purpose. It noted that
equal trees are the normal case only because Publish merges current `main`
into the branch just before the PR, and that a squash made after `main`
moved would fail and escalate. This is that case.

Two refs trigger it here:

- the local `feat/design-principles`;
- `refs/remotes/origin/feat/design-principles`. GitHub deleted the remote
  branch (`git ls-remote` returns nothing), but the tracking ref is still
  there, at `42d95f0`, because nothing pruned it.

The failure blocks more than x3. `closed()` (`:352-353`) and `mergedEarly()`
(`:324-326`) both run `merged()`. So x4-close fails the same way while either
ref exists, even if Jeff records x3-merge done by hand (design-principles
q1, option 2). The item holds the PR slot until this lands or the refs are
removed by hand. The same happens to every PR a human merges after another
PR, which is the normal order once two items reach Merge together.

## Prior art

- **`wf-squash-merged-pr`** (archived; FLOW-16's squash clause). It added the
  equal-tree proof and the single `git branch -D` it permits. Its rejected
  alternative, "the PR-number commit alone", stays rejected. This design adds
  a third proof beside ancestry and tree equality, which are unchanged, and
  keeps that design's safety argument: deleting the branch must lose nothing
  the squash did not land. Its open follow-up stays open: a stale, unpruned
  `refs/remotes/origin/<branch>` at a head older than the PR's still fails
  the check (`wf-squash-merged-pr` "Found outside the design" and its archive
  summary). The patch-id proof does not cover it, because an older head's
  change `B..ref` lacks the head's later commits and so has a different
  patch-id. Fixing it, either with `git fetch --prune origin` before the
  Merge and Close checks or by ignoring the remote ref once `ls-remote` shows
  the branch gone, is out of scope here.
- **`wf-merged-counts-as-pushed`** (archived). It added the PR-number search
  (`(Merge pull request |\()#N([^0-9]|$)`, `check.go:256`) and the "no ref
  left" pass on that commit alone (`:278-283`). Both are reused unchanged.
- **git's patch-id** is the standard way to say "the same change" across
  different bases. `git cherry` and `git rebase` use it to skip commits that
  are already upstream. It hashes the diff with line numbers ignored, so a
  hunk's offset does not change it. It covers file modes and binary files: a
  mode change and two different binary contents at one path each gave
  different ids, tested with git 2.43.

## Decisions

1. **A squash with the branch's change counts as merged** (a1, a2).
   `refMerged()` gains a third proof, tried for each PR commit whose tree does
   not match. Let `C` be a PR commit with exactly one parent, and
   `B = git merge-base ref C^`, a single merge base. `ref` is merged when the
   change `C^..C` has the same patch-id as the change `B..ref`. That is: the
   squash applied to `main` exactly what the branch changed since it last
   shared history with that `main`.
   - The proof is the patch-id equality, not the commit's name. The PR-number
     search only says which commits to compare.
   - A head pushed past the squash has an extra change, so its patch-id
     differs and the check fails, as the tree check fails today.
   - So does a squash of an older head, which lacks the head's last change.
   - In design-principles' case, `B = 49e5354`, `C^ = f861315`, and both ids
     are `43f1a18b…`.

   *Rejected:*
   - **Tree equality against `C^` merged with `ref`.** Computing
     `git merge-tree C^ ref` and comparing it to `C`'s tree proves the same
     thing. But it re-runs a three-way merge inside the check
     (`git merge-tree --write-tree`, git >= 2.38) and its failure detail is a
     tree with conflict markers. Patch-ids compare the two changes directly
     with one diff each. Both fail on a conflict the human resolved
     (Decision 3).
   - **Any commit on `main` after `B` with the branch's patch-id, whatever
     its message.** It could credit the human's merge (FLOW-08) to a commit
     from another PR that happens to make the same change.
   - **The PR-number commit alone.** Rejected in `wf-squash-merged-pr`
     Decision 1, for the same reason: it could delete commits pushed after
     the squash.

2. **The patch-id is whitespace-exact** (a3). Both diffs are computed with
   every output-affecting option pinned (context, algorithm, indent
   heuristic, hunk merging, path prefixes, renames, relative paths, color,
   external diff and textconv):
   `git diff --no-color --no-ext-diff --no-textconv --no-renames --no-relative -U3 --diff-algorithm=myers --indent-heuristic --inter-hunk-context=0 --src-prefix=a/ --dst-prefix=b/ <a> <b> | git patch-id --verbatim`,
   taking the first field. These flags override `diff.context`,
   `color.diff`, `diff.external`, textconv drivers, `diff.renames`,
   `diff.relative`, `diff.algorithm`, `diff.indentHeuristic`,
   `diff.interHunkContext` and `diff.noprefix`/`diff.mnemonicPrefix`, so those
   settings in user or repository configuration cannot change the diffs, and
   a patch-id printed in a detail reproduces with the same command elsewhere.
   Myers and the indent heuristic are git's defaults, so the ids equal those
   of a plain `git diff` with no configuration: the incident's `43f1a18b…`
   on both sides, checked with git 2.43. `--stable` would ignore whitespace, so
   a squash that dropped a whitespace change (a YAML indent, a Makefile tab)
   would pass. `--verbatim` implies `--stable`, so the order of files does not
   matter either. An empty diff yields no patch-id, and an empty patch-id
   never matches.

   *Rejected:*
   - `--stable`: whitespace-blind.
   - Zero context (`-U0`): a3 option (b).

3. **A real conflict resolution fails and escalates** (a4). Suppose the human
   resolved a conflict in the GitHub UI, or edited the change while merging.
   Then the patch-ids differ, and the check fails as it does today. The detail
   names the ref's tree and, for each PR commit C, its tree and its change's
   patch-id, beside the ref's base B_C and patch-id from that base (or C's
   merge bases, or "N parents, not compared"). It also gives, for each C, the
   commands to compare them: `git diff <B_C> <ref>` and `git diff <C>^ <C>`,
   and, for after the comparison, the commands that remove both refs (the
   local branch and the stale remote-tracking ref), which x4-close needs gone
   (Problem). Comparing the two diffs by
   eye is the human's call, made at the existing escalation. The loop never
   decides that a different change is close enough.

   *Rejected:*
   - **A queue field, such as `merged_as: <sha>`, in which Jeff records the
     squash, accepted by the check.** It would be a new override for a rare
     case. The hand path already exists: record the step done, then delete
     the branch with the commands the failure prints. a4 option (b).

4. **FLOW-16 names the new proof** (a5). Its squash clause
   (`docs/development/process.md:500-504`) adds the patch-id case. The
   `workflow-tick` Close step (`.claude/skills/workflow-tick/SKILL.md:95-96`)
   says "only once `dist/wf check` has passed on it" instead of "on its equal
   tree". This is a process item, so the rule text changes on its branch and
   `rule_changes` stays empty (FLOW-14).

## Questions

### Blocking

None.

### Assumed

- **a1. What identifies the squash commit.** Proceeding with (a); confirm.
  Options:
  - (a) The commits the existing PR-number search finds
    (`(Merge pull request |\()#N([^0-9]|$)` anywhere in the message, on
    `origin/main`), restricted to those with one parent. Consequence: there
    is one search for all three proofs, and the patch-id carries the safety.
    A later commit whose body mentions `(#N)` is compared too. It passes only
    if it makes the same change, which is then on `main` anyway.
  - (b) As (a), but only a subject ending in `(#N)`, GitHub's squash form.
    Consequence: a stricter attribution. A squash whose subject the human
    edited fails and escalates.
  - (c) The PR's `merge_commit_sha` from the GitHub API. Consequence: it is
    authoritative. But it puts network access and a credential into
    `wf check`, which runs offline today and in its tests.

  Recommended: (a).
- **a2. The base of the branch's change.** Proceeding with (a); confirm.
  Options:
  - (a) `git merge-base ref C^`: the history the branch shared with the
    `main` the squash applied to.
  - (b) `git merge-base ref origin/main`. Consequence: the same in every
    normal case. It differs only when the branch merged `main` after the
    squash, which brings `C` into the branch's history and empties the
    comparison.

  Either way, a ref with more than one merge base (a criss-cross history)
  fails, and the detail names the merge bases (GLOBAL-11). The check does not
  choose one.

  Recommended: (a).
- **a3. How exact the patch-id is.** Proceeding with (a); confirm. Options:
  - (a) `--verbatim` with git's default three lines of context, pinned with
    `-U3` beside the other pinned flags (`--diff-algorithm=myers`,
    `--indent-heuristic`, `--inter-hunk-context=0`, `--no-relative`, fixed
    `a/` and `b/` prefixes; Decision 2). Consequence: whitespace counts, and so do context lines. If `main`
    edited a line within three lines of a branch hunk, the check fails and
    escalates although nothing conflicted. That is likely in
    `docs/development/process.md`, which most process items edit. Such a
    failure is safe, but it is noise.
  - (b) `--verbatim` with `-U0`. Consequence: an edit next to a hunk on `main`
    no longer fails the check. The proof weakens to "the same lines added and
    removed in each file, in order", without checking where they sit. A
    squash git produced cannot move a hunk, so this is still safe for
    GitHub's merges.
  - (c) `--stable`. Consequence: whitespace-blind; rejected in Decision 2.

  Recommended: (a). Change to (b) if adjacent-edit escalations show up in
  practice.
- **a4. A squash with a different change.** Proceeding with (a); confirm.
  Options:
  - (a) Fail, name both changes and the commands to compare them, and
    escalate as today. The detail also prints the deletion commands. The
    human records the step done and runs them.
  - (b) As (a), plus a queue field in which the human records the squash
    commit, which the check then accepts. Consequence: Close runs by itself
    after the human's look, but this adds an override to the queue schema
    and to `wf validate`.

  Recommended: (a).
- **a5. FLOW-16 wording.** Proceeding with (a); confirm. Options:
  - (a) The amended clause under Design: the patch-id case is added beside
    the equal tree, and `git branch -D` stays limited to a squash proved by
    `wf check`. Otherwise the escalation gives the human the deletion
    commands, and an agent never runs them.
  - (b) Keep FLOW-16's text and let the check alone carry the new proof.
    Consequence: the rule says "equal tree" while `wf check` deletes on an
    equal change. The rule and the tooling then disagree (FLOW-14).

  Recommended: (a).
- **a6. Holds nothing; design-principles.** Proceeding with (a); confirm.
  Options:
  - (a) Hold nothing. The change reaches the loop only at merge, and nothing
    in flight depends on the old behaviour. Consequence: design-principles'
    q1 is already answered with option 2, and its x3-merge is recorded done
    by hand. Once this lands, its x4-close passes on its next tick: it
    deletes the local branch with `-D` and accepts the stale tracking ref,
    because it points at the PR head `42d95f0` (a stale ref at an older head
    would still fail; see Prior art). Until then x4-close fails unless both
    refs are removed by hand (Problem). Validation does not depend on which
    of these happens (Validation plan).
  - (b) Hold `cloud` until this merges. Consequence: the queue stalls for
    one PR-slot item.

  Recommended: (a).

## Design

`refMerged(ref, prCommits)` keeps its order and returns at the first proof
that holds:

1. `ref` is an ancestor of `origin/main`: "contained in origin/main", as
   today.
2. For each PR commit `C`, `C`'s tree equals `ref`'s tree: "squash-merged as
   C (tree T)", as today.
3. For each PR commit `C` with one parent, `B = merge-base(ref, C^)` is
   unique and `pid(B, ref) == pid(C^, C) != ""`. Info: "squash-merged as C
   after main moved (change P, base B)". "After main moved" names the usual
   case only: the proof does not check that main moved.

Otherwise it fails. The detail keeps today's prefix verbatim,
`<ref> (tree T) is not contained in origin/main, and no PR commit has its
tree`, so `TestMergedBySquash` (`check_test.go:549-550`) still matches. It
then appends ` or its change: ` and, for each PR commit `C`, comma-separated:
`C`'s tree; for a one-parent `C`, its patch-id pid(C^, C), its base
B_C = merge-base(ref, C^) and pid(B_C, ref); for a commit with more than one
parent, `<sha> (N parents, not compared)`. If `git merge-base --all ref C^`
returns more than one commit, proof 3 is not tried for that `C`, and the
detail names every merge base ("merge bases X, Y") instead of a single base
and patch-id (a2, GLOBAL-11). For each compared `C` it gives
"compare: git diff B_C ref; git diff C^ C". It ends with "after comparing, a
human may delete: git branch -D <branch>; git update-ref -d
refs/remotes/origin/<branch>", where the second command is printed only when
`git ls-remote origin refs/heads/<branch>` returns nothing (GitHub deleted
the remote branch). The printed text is exact and uses the item's real
branch name. Here `pid(a, b)` runs
`git diff --no-color --no-ext-diff --no-textconv --no-renames --no-relative -U3 --diff-algorithm=myers --indent-heuristic --inter-hunk-context=0 --src-prefix=a/ --dst-prefix=b/ a b | git patch-id --verbatim`
and takes the first field. `merged()`, its callers
and the no-ref path do not change.

| Site | Change |
|---|---|
| `tools/workflow/check/check.go` `refMerged()` (`:286-310`) | the third proof; a `patchID(a, b)` helper; the failure detail extended (its current prefix kept) with patch-ids, compare and deletion commands; doc comments on `merged()` (`:246-250`) and `refMerged()` updated |
| `tools/workflow/check/check_test.go` | `TestSquashOfAnOlderHeadFails` (`:566`) builds exactly the incident: `main` moves, then the branch is squashed. It now passes, so it becomes `TestSquashAfterMainMoved`. The renamed test asserts `OK(rs)`, and that `rs[0].Info` names the patch-id proof for each ref: it contains `refs/heads/feat/demo` and `refs/remotes/origin/feat/demo`, each followed by its "after main moved" proof, with the change and base, as `TestMergedBySquash` checks "squash-merged" (GLOBAL-11 Fallback). New: a squash whose change differs from the branch's (a conflict-style edit in the squash) fails, and its detail names both patch-ids and contains `git branch -D feat/demo`; the remote-tracking deletion command appears only once the remote branch is gone. A head pushed past a squash made after `main` moved fails (the older-head case of Decision 1). A two-parent PR commit is not compared by patch-id. Setup: move main with a commit that feat/demo lacks. On a side branch cut from the moved main, apply the branch's change (`git merge --squash feat/demo` and commit, or cherry-pick its commits). On main, run `git merge --no-ff -m "Merge pull request #9 from o/side" side` and push. That commit names the PR and has two parents. Its first-parent diff (`C^1..C`) has the same patch-id as `merge-base(feat/demo, C^1)..feat/demo`. Its tree is not feat/demo's tree, and feat/demo is not an ancestor of origin/main. As a guard, the test asserts that the first-parent patch-id equals the branch's, so it would fail if two-parent commits were compared. It then asserts that the merged check fails and that the detail marks the commit as not compared (`2 parents, not compared`). Whitespace-only differences fail. A criss-cross history (two merge bases between the ref and `C^`) fails, and its detail names both bases. Configuration: with `diff.algorithm=histogram`, `diff.relative=true` and `diff.indentHeuristic=false` set in the temp repo's config (`git config`), the incident case (`TestSquashAfterMainMoved`) still passes, run from a subdirectory as well as the root, and its patch-ids equal those computed with no config set. `TestMergedBySquash` and `TestCloseAfterASquash` stay unchanged |
| `docs/development/process.md` FLOW-16 (`:500-504`) | the clause below |
| `.claude/skills/workflow-tick/SKILL.md` Close (`:95-96`) | "`git branch -D`, only once `dist/wf check` has passed on it; never `-D` otherwise" |

FLOW-16, the amended parenthesis and the sentence after it:

> … a branch is deleted only after it is shown to be contained in trunk
> (`git branch --merged`, the PR's merge commit, or, for a squash merge, a
> commit on trunk naming the PR whose tree equals the branch head's tree, or a
> one-parent such commit whose change from its parent has the same
> `git patch-id --verbatim` as the branch head's change from its merge base
> with that parent), with git's safe forms (`git branch -d`) and never by
> force-pushing. `git branch -d` refuses a squash-merged branch, so that case
> alone deletes the local branch with `git branch -D`, and only after
> `wf check` has shown the equal tree or change; otherwise the escalation
> gives the human these commands, and an agent never runs them.

## Defaults and overrides

- **Defaults:** none added or changed.
- **Overrides:** none added (a4 rejects one).
- **Fallback: the patch-id proof, named here (GLOBAL-11).**
  - *Trigger:* `refMerged()` tries it only when the ref is not an ancestor of
    `origin/main` and no PR commit's tree equals the ref's tree.
  - *How it is seen:* the check's info says which proof passed ("after main
    moved", with the patch-id and base), so the queue history shows it. The
    words name the usual case; the proof does not check that main moved.
  - *When it fails:* the detail names every value compared, for each PR
    commit.
  - *What it never does:* accept a commit that does not name the PR, or a
    change that differs.

## Cross-driver impact

None. `drivers` is empty: this changes the workflow tooling only. No driver,
relay or collector code is touched.

## Validation plan

- **Tier 0:** the `check` tests above, run in temporary repositories as the
  existing squash tests are, then `make test` and `make lint` for the
  `tools/workflow` module.
- **Tier 1:** the branch changes `tools/workflow/check/`, so CI's "main's
  enforcement" step runs tier 1 built from the merge base, with the branch's
  configuration, against the branch, and compares its findings (FLOW-14). No
  finding should disappear, because no tier-1 rule or configuration changes.
  A process item has no Review phase. Implement runs `make enforcement`
  (expect "main's tier 1 findings kept", 0 dropped) and records the result in
  Consolidation. It also runs the merge base's `wf check` against the branch
  by hand and records that too. There is no rule change to clear, because a
  process item's branch is exempt from the rule-change clearance.
- **Live, read-only, independent of queue state:** in a scratch clone of
  origin, run `git branch feat/design-principles 42d95f0` and
  `git update-ref refs/remotes/origin/feat/design-principles 42d95f0`. Then
  run the branch's `wf check` on a copy of design-principles' queue item, set
  back to x3-merge pending with pr 104. It passes, and its info names
  "squash-merged as beb51ba after main moved" with patch-id `43f1a18b…` (the
  `--verbatim` id; `fdc1cfd3…` is the `--stable` one) and base `49e5354`.
  With the refs absent, it passes on the PR commit alone. The real workflow
  checkout and queue are never touched, as `wf-squash-merged-pr` verified
  #91.
- **Native validation:** none. Nothing runs on a native host.

## Implementation plan

1. Add `patchID()` and the third proof to `refMerged()`, with the failure
   detail. Update the doc comments (FLOW-08, FLOW-16, GLOBAL-01, GLOBAL-11).
2. Tests: rename and invert `TestSquashOfAnOlderHeadFails`, asserting the
   "after main moved" info for both refs, and add the different-change,
   pushed-past (the older-head case of Decision 1), two-parent, whitespace,
   criss-cross merge-base and configuration cases (TEST-01).
3. FLOW-16 clause, then the `workflow-tick` Close wording (FLOW-14).
4. Run `make test` and `make lint`, and run the scratch-clone live check
   (Validation plan). Archive the design with its summary, then open the PR
   (x2, FLOW-12).

## Design review

## Consolidation

## Rollback notes

## Archive summary
