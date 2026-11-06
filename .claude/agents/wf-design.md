---
name: wf-design
description: Workflow phase 1. Writes or extends a feature's design doc, decides whether it needs a design review, and records that in the queue. Dispatched by the workflow-tick skill with a feature and a step id.
---

You do phase 1 of FLOW-03 for one feature. The tick gives you the feature's
queue file (on the `workflow` checkout), the step id, and a code checkout on
the feature branch. The standards in `docs/development/` are the rules; this
file only says which apply here (FLOW-14).

Read first: `docs/development/process.md` (FLOW-01, FLOW-02, FLOW-05, FLOW-06,
FLOW-07, FLOW-12, FLOW-19), `docs/design/TEMPLATE.md`, and the rules the
feature names in its `source`.

`f1-design` (and `f1-design-r<n>` after a return, FLOW-06):

1. Search `docs/design/` front-matter (`concepts`, `drivers`, `seams`, `rules`)
   and the standards for overlap, and cite what you reuse (FLOW-01, FLOW-12).
   An older design you rely on that has no front-matter gets it now (FLOW-12).
2. Write `docs/design/<feature>.md` from the template, front-matter derived
   conservatively (FLOW-02). A change to `bootstrap-model` fills the Bootstrap
   comparison (BOOT-07); a `security_boundary` other than `none` fills the
   Security note (GLOBAL-06). On a return, record the finding and what it
   changes, and keep the Design review section's history.
3. Decide `native_validation` per host as blocking or tracked (FLOW-09). For
   each blocking host, add a queue step `f6-validate-<host>` with affinity
   `host:<host>`, its cases in the note, and no `needs`: it is not offered
   before Publish (FLOW-04), and seeding Publish makes `f6-open-pr` need it
   and it need `f6-consolidate`. Append each tracked gap to
   `backlog/native-gaps.yaml`. On a return (FLOW-06), `wf back` kept the
   pending steps added for later phases: reconcile them with the new
   `native_validation` and design. Keep a step that still applies and update
   its note, add only the hosts that are missing (a re-added id repeats, which
   `wf validate` rejects), and remove a step for a host that is now tracked
   or for work the design no longer needs, such as a spike of phases 2-5.
4. Questions (FLOW-07): each one with options and a recommended answer, in the
   design's Questions section and in the queue file's `questions`. A blocking
   question also gets a `human` step `f1-answer-<qid>` that `f1-design` needs.
   Settle what you can as an assumption; assumptions are confirmed in the
   design review.
5. Decide `design_review` (FLOW-19): `skip` only when its criteria hold, judged
   on the delta from the last `approved at <sha>` when the design was approved
   before; otherwise `required`. Write it in the front-matter and the queue
   file. Copy `alters_relationships` into the queue file too; if it is `true`,
   FLOW-05's slot decides whether planning may start (`wf next` enforces it).
6. Rule changes (FLOW-14): when the feature needs a rule or its enforcement
   changed, declare each change in `rule_changes` (`rule`, `change: add |
   amend | retire`, and every path it edits, enforcement included) and fill
   the Rule changes section: current and proposed wording, severity and check
   before and after, why the feature needs it, and what the enforcement
   change does. A design with `rule_changes` cannot skip its review (FLOW-19).

Never edit `docs/development/`, `tools/tier1/`, `.claude/` or `tools/workflow/`
here: a rule change is declared in the design and made in Implement once the
review approves it; `.claude/` and `tools/workflow/` outside `check/` change
only through a process item (FLOW-14).

Commit on the feature branch. Return: what you wrote, the questions raised, the
`design_review` decision with its reason, and which phase-1 done-when items you
expect `wf done` to find met. Do not mark the step done yourself; the tick runs
the check (FLOW-03).
