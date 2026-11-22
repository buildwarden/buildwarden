---
name: wf-design
description: Workflow phase 1. Writes or extends a feature's design doc, decides whether it needs a design review (the human's, or while FLOW-20 stands the umbrella test or the process test), and records that in the queue. Dispatched by the workflow-tick skill with a feature and a step id; also a process item's cloud x1-design while FLOW-20 stands.
---

You do phase 1 of FLOW-03 for one feature. The tick gives you the feature's
queue file (on the `workflow` checkout), the step id, and a code checkout on
the feature branch. The standards in `docs/development/` are the rules; this
file only says which apply here (FLOW-14).

Read first: `docs/development/process.md` (FLOW-01, FLOW-02, FLOW-05, FLOW-06,
FLOW-07, FLOW-12, FLOW-19), `docs/development/loop-contract.md` (LOOP-02,
LOOP-04, LOOP-09; from `origin/main` when the branch predates it),
`docs/design/TEMPLATE.md`, and the rules the feature names in its `source`.

`f1-design` (and `f1-design-r<n>` after a return, FLOW-06):

1. Search `docs/design/` front-matter (`concepts`, `drivers`, `seams`, `rules`)
   and the standards for overlap, and cite what you reuse (FLOW-01, FLOW-12).
   An older design you rely on that has no front-matter gets it now (FLOW-12).
2. Write `docs/design/<feature>.md` from the template, front-matter derived
   conservatively (FLOW-02). A change to `bootstrap-model` fills the Bootstrap
   comparison (BOOT-07); a `security_boundary` other than `none` fills the
   Security note (GLOBAL-06). Declare `repos` and `areas` in the
   front-matter, starting from what intake recorded in the queue file
   (FLOW-02). A design that needs an area or a link the area map lacks says
   so under **Design** and never edits the map (FLOW-05). Fill **Defaults and overrides** (GLOBAL-09 to
   GLOBAL-11). Each default, override or named fallback there that the item's
   `source` or an existing rule does not settle is an assumed question (step
   4), and a fallback the design drops is a rejected alternative in its
   decision. Fill it even when the branch's `TEMPLATE.md` predates it. Under
   **Design**, say for each repository in `repos` other than this one what
   is done there (LOOP-04), and fill **Specification changes** when `seams` is not empty
   (LOOP-02), each even when the branch's template predates it. A relay
   setting the design adds or moves names the claim field that carries it
   (RELAY-08), or says which of the relay's pre-claim values it is. On a
   return, record the finding and what it changes, and keep the Design review
   section's history.
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
   design review. Mark an assumed question `Flagged: <reason>` when it
   touches a security boundary or a seam, or departs from prior art or an
   earlier decision of Jeff's; a flagged question is always answered on its
   own (FLOW-19).
5. Decide `design_review` (FLOW-19; step 6 or 7 while FLOW-20 stands): `skip` only when its criteria hold, judged
   on the delta from the newest review entry's `approved at <sha>` when the design was approved
   before; otherwise `required`. A `required` review is seeded at the start
   of phase 2, before Plan, and Plan works from the design it approves.
   Write it in the front-matter. Commit, then run
   `dist/wf declare -state <workflow checkout> -repo . -worker <the tick's worker> -feature <F>`
   to copy the design's declarations into the queue: `repos`, `areas`,
   `seams`, the `rule_changes` rules, `alters_relationships` and
   `design_review`; if `alters_relationships` is `true`, FLOW-05's slot
   decides whether planning may start. If it refuses because a limit would
   be exceeded, raise a blocking question that names the holders it printed
   (FLOW-07) and leave Design unfinished.
6. While FLOW-20 stands (a `config.yaml` repositories entry has
   `program_trunk`), on a program item (its design lists
   `componentization.md` in `related`, and the queue marks it `program`):
   - Settle a question from the umbrella design or a recorded answer of
     Jeff's before raising it, citing the source, and record "settled by
     <ref>, not asked". A question of strategy, however hard, is assumed,
     never blocking, on your recommendation (a record of `-kind assumed`);
     only component design (a seam's change the program's design
     does not name for the item's phase, a component's boundary or
     ownership, a contract, a security posture), setup or privileged work is
     blocking or flagged (FLOW-07, FLOW-19). Follow Jeff's recorded
     decisions; a departure from one on strategy is recorded (`-kind
     departure`), and one on component design flags its question.
   - You write no program record yourself: return each one's kind,
     feature, repository, head, ref and note, and the tick writes it with
     `dist/wf record -state .dev/workflow -worker $W` (its step 5,
     FLOW-20).
   - Judge the umbrella test's points U1 to U6 yourself (`wf-review`'s
     **The umbrella test**). When they hold, write `design_review: umbrella`,
     with the points' evidence under **Design**; otherwise `required`.
   - A design that changes the ledger format opens or changes its working
     draft beside v1 on the program trunk, as `specs/README.md` says; opening
     one is component design and goes to Jeff's review.
7. A process item's cloud `x1-design` while FLOW-20 stands: create
   `feat/<slug>` from `origin/main` and push it when it does not exist yet,
   before writing the design. Write the design from the template with a
   **Program payoff** section (FLOW-20's bar, with figures and how they
   were reached) and the list of every rule, machinery path and
   configuration key the change reaches. Then write `design_review:
   process-test` when it touches none of the rules FLOW-20 reserves to Jeff
   (`wf-review`'s **The process test**, P2, read by effect), or `required`
   when it touches one or you doubt it, and run `dist/wf declare`. The
   process test or his review follows in phase 2. A state step the design
   proposes is recorded by Jeff at his merge of the item (FLOW-03).
8. Rule changes (FLOW-14): when the feature needs a rule or its enforcement
   changed, declare each change in `rule_changes` (`rule`, `change: add |
   amend | retire`, and every path it edits, enforcement included) and fill
   the Rule changes section: current and proposed wording, severity and check
   before and after, why the feature needs it, and what the enforcement
   change does. A design with `rule_changes` cannot skip its review (FLOW-19).
   A feature's design declares no change to a loop rule (LOOP-\*, FLOW-\*,
   LINT-\*) or to its enforcement: that needs a process item (LOOP-09,
   FLOW-14).

Never edit `docs/development/`, `tools/tier1/`, `.claude/` or `tools/workflow/`
here: a rule change is declared in the design and made in Implement once the
review approves it; `.claude/` and `tools/workflow/` outside `check/` change
only through a process item (FLOW-14).

Commit on the feature branch. Return: what you wrote, the questions raised, the
`design_review` decision with its reason, and which phase-1 done-when items you
expect `wf done` to find met. Do not mark the step done yourself; the tick runs
the check (FLOW-03).

On a native machine (a led step, FLOW-04), a branch is pushed only with
`tools/workflow/native/native-push.sh <name>`, or `native-push.sh <name>
<owner>/<repo>` for a branch in a second repository's clone, whose worktree
is `.scratch/<owner>/<repo>/<name>`, made with `git -C
.dev/checkouts/<owner>/<repo> worktree add`; and committed with `git -C
<worktree> commit -F ../<name>.msg`, the message written beside the
worktree, never `-m` (FLOW-17).
