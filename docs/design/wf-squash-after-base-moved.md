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
rules: [FLOW-03, FLOW-08, FLOW-09, FLOW-14, FLOW-16, GLOBAL-09, GLOBAL-10, GLOBAL-11]
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
  the squash did not land.
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
     thing. But it re-runs a merge inside the check, and it fails on any
     conflict, including one the human resolved identically. Patch-ids
     compare the two changes directly.
   - **Any commit on `main` after `B` with the branch's patch-id, whatever
     its message.** It could credit the human's merge (FLOW-08) to a commit
     from another PR that happens to make the same change.
   - **The PR-number commit alone.** Rejected in `wf-squash-merged-pr`
     Decision 1, for the same reason: it could delete commits pushed after
     the squash.

2. **The patch-id is whitespace-exact** (a3). Both diffs are computed with
   fixed options, so user or repository configuration cannot change them:
   `git diff --no-color --no-ext-diff --no-textconv --no-renames -U3 <a> <b>`,
   piped to `git patch-id --verbatim`. `--stable` would ignore whitespace, so
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
   names, for each PR commit, its tree and its change's patch-id, beside the
   ref's tree and patch-id. It also gives the commands to compare them:
   `git diff <B> <ref>` and `git diff <C>^ <C>`. Comparing the two diffs by
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
    `-U3`. Consequence: whitespace counts, and so do context lines. If `main`
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
    escalate as today. The human records the step done and deletes the
    branch by hand.
  - (b) As (a), plus a queue field in which the human records the squash
    commit, which the check then accepts. Consequence: Close runs by itself
    after the human's look, but this adds an override to the queue schema
    and to `wf validate`.

  Recommended: (a).
- **a5. FLOW-16 wording.** Proceeding with the amended clause under Design;
  confirm. `git branch -D` stays limited to a squash proved by `wf check`.
  It may now be proved by the change as well as the tree.
- **a6. Holds nothing; design-principles q1.** Nothing in flight depends on
  the old behaviour. Once this merges, design-principles' x3-merge passes on
  its next tick. x4-close then deletes the local branch with `-D` and passes.
  The stale remote-tracking ref also passes, because it points at the PR head
  `42d95f0`. If Jeff answers q1 by recording x3 done by hand before then,
  x4-close still fails until this lands or both refs are removed by hand
  (Problem). Confirm, and answer q1 knowing this.

## Design

`refMerged(ref, prCommits)` keeps its order and returns at the first proof
that holds:

1. `ref` is an ancestor of `origin/main`: "contained in origin/main", as
   today.
2. For each PR commit `C`, `C`'s tree equals `ref`'s tree: "squash-merged as
   C (tree T)", as today.
3. For each PR commit `C` with one parent, `B = merge-base(ref, C^)` is
   unique and `pid(B, ref) == pid(C^, C) != ""`. Info: "squash-merged as C
   after main moved (change P, base B)".

Otherwise it fails. The detail names `ref`'s tree, `ref`'s patch-id and its
base, and each PR commit's tree and patch-id. It ends with "compare: git diff
B ref; git diff C^ C". Here `pid(a, b)` runs
`git diff --no-color --no-ext-diff --no-textconv --no-renames -U3 a b |
git patch-id --verbatim` and takes the first field. `merged()`, its callers
and the no-ref path do not change.

| Site | Change |
|---|---|
| `tools/workflow/check/check.go` `refMerged()` (`:286-310`) | the third proof; a `patchID(a, b)` helper; the failure detail with patch-ids and compare commands; doc comments on `merged()` (`:246-250`) and `refMerged()` updated |
| `tools/workflow/check/check_test.go` | `TestSquashOfAnOlderHeadFails` (`:566`) builds exactly the incident: `main` moves, then the branch is squashed. It now passes, so it becomes `TestSquashAfterMainMoved`. New: a squash whose change differs from the branch's (a conflict-style edit in the squash) fails, and its detail names both patch-ids. A head pushed past a squash made after `main` moved fails. A two-parent PR commit is not compared by patch-id. Whitespace-only differences fail. `TestMergedBySquash` and `TestCloseAfterASquash` stay unchanged |
| `docs/development/process.md` FLOW-16 (`:500-504`) | the clause below |
| `.claude/skills/workflow-tick/SKILL.md` Close (`:94-96`) | "`git branch -D`, only once `dist/wf check` has passed on it; never `-D` otherwise" |

FLOW-16, the amended parenthesis and the sentence after it:

> … a branch is deleted only after it is shown to be contained in trunk
> (`git branch --merged`, the PR's merge commit, or, for a squash merge, a
> commit on trunk naming the PR whose tree equals the branch head's tree or,
> when trunk moved before the squash, whose change has the same patch-id as
> the branch's change from its merge base), with git's safe forms
> (`git branch -d`) and never by force-pushing. `git branch -d` refuses a
> squash-merged branch, so that case alone deletes the local branch with
> `git branch -D`, and only after `wf check` has shown the equal tree or
> change.

## Defaults and overrides

- **Defaults:** none added or changed.
- **Overrides:** none added (a4 rejects one).
- **Fallback: the patch-id proof, named here (GLOBAL-11).**
  - *Trigger:* `refMerged()` tries it only when the ref is not an ancestor of
    `origin/main` and no PR commit's tree equals the ref's tree.
  - *How it is seen:* the check's info says which proof passed ("after main
    moved", with the patch-id and base), so the queue history shows it.
  - *When it fails:* the detail names every value compared.
  - *What it never does:* accept a commit that does not name the PR, or a
    change that differs.

## Cross-driver impact

None. `drivers` is empty: this changes the workflow tooling only. No driver,
relay or collector code is touched.

## Validation plan

- **Tier 0:** the `check` tests above, run in temporary repositories as the
  existing squash tests are, then `make test` and `make lint` for the
  `tools/workflow` module.
- **Tier 1:** the branch changes `tools/workflow/check/`, so Review and CI
  also run the merge base's `wf check` (FLOW-14). There is no rule change to
  clear, because the branch is a process item.
- **Live, read-only:** with the branch's `wf` build, design-principles'
  x3-merge check passes against `beb51ba` with both refs present, as
  `wf-squash-merged-pr` verified #91.
- **Native validation:** none. Nothing runs on a native host.

## Implementation plan

1. Add `patchID()` and the third proof to `refMerged()`, with the failure
   detail. Update the doc comments (FLOW-08, FLOW-16, GLOBAL-01, GLOBAL-11).
2. Tests: rename and invert `TestSquashOfAnOlderHeadFails`, and add the
   different-change, pushed-past, two-parent and whitespace cases (TEST-01).
3. FLOW-16 clause, then the `workflow-tick` Close wording (FLOW-14).
4. Run `make test` and `make lint`, and run the live read-only check on
   design-principles. Archive the design with its summary, then open the PR
   (x2, FLOW-12).

## Design review

## Consolidation

## Rollback notes

## Archive summary
