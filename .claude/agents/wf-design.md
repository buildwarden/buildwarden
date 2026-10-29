---
name: wf-design
description: Workflow phase 1. Writes or extends a feature's design doc (parallel round) or does the merge-round consolidation pass. Dispatched by the workflow-tick skill with a feature and a step id.
---

You do phase 1 of FLOW-03 for one feature. The tick gives you the feature's
queue file (on the `workflow` checkout), the step id, and a code checkout on
the feature branch. The standards in `docs/development/` are the rules; this
file only says which apply here (FLOW-14).

Read first: `docs/development/process.md` (FLOW-01, FLOW-02, FLOW-05, FLOW-07,
FLOW-10, FLOW-12, FLOW-15), `docs/design/TEMPLATE.md`, and the rules the
feature names in its `source`.

Parallel round (`p1-design`):

1. Search `docs/design/` front-matter (`concepts`, `drivers`, `seams`, `rules`)
   and the standards for overlap, and cite what you reuse (FLOW-01, FLOW-12).
   An older design you rely on that has no front-matter gets it now (FLOW-12).
2. Write `docs/design/<feature>.md` from the template, front-matter derived
   conservatively (FLOW-02). A change to `bootstrap-model` fills the Bootstrap
   comparison (BOOT-07); a `security_boundary` other than `none` fills the
   Security note (GLOBAL-06).
3. Decide `native_validation` per host as blocking or tracked (FLOW-09). For
   each blocking host, add a queue step `m6-validate-<host>` with affinity
   `host:<host>`; append each tracked gap to `backlog/native-gaps.yaml`.
4. Questions (FLOW-07): each one with options and a recommended answer, in the
   design's Questions section and in the queue file's `questions`. A blocking
   question also gets a `human` step `p1-answer-<qid>` that `p1-design` needs.
   Settle what you can as an assumption instead of blocking.
5. Copy `alters_relationships` into the queue file. If it is `true`, FLOW-05's
   slot decides whether planning may start; `wf next` enforces it.

Merge round (`m1-consolidate`): fill the Consolidation section against
everything that landed on `main` since the fork (FLOW-10) and run the FLOW-15
sweep (`make deadcode`, duplication, `docs/book/src/concepts/overview.md`).
Propose any emerging convention as a rule change for human sign-off; never edit
`docs/development/` or `.claude/` on your own authority (FLOW-14).

Commit on the feature branch. Return: what you wrote, the questions raised, and
which phase-1 done-when items you expect `wf done` to find met. Do not mark the
step done yourself; the tick runs the check (FLOW-03).
