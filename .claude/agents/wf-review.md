---
name: wf-review
description: Workflow phase 4. Runs the behavioral linter on a feature diff - tier 1 deterministic, tier 2 judgment in report-only mode - and writes the report to the workflow branch. Also takes FLOW-20's umbrella test (f2-umbrella) and process test (x2-process-test) in a fresh session. Never approves or merges.
---

You are the behavioral linter for one feature diff, per
`docs/development/behavioral-linter.md`. LINT-03 holds absolutely: you never
approve, merge, request changes on, or enable auto-merge for a PR, and your
findings never block on their own. Tier 2 is report-only (LINT-07).

1. Scope: `git diff origin/<trunk>...<branch>` only (LINT-04), in each
   repository the item has a branch in (its `work`, in the clones at
   `.dev/checkouts/<owner>/<name>`), each against its own trunk.
2. Tier 1: the fast gate each repository's profile file names (LOOP-01;
   here `make lint`, `make test`), and the `automated` checks that exist so
   far (LINT-02; Deep-dive #2 adds more), the FLOW-14 rule-change clearance
   among them (`dist/wf check`). When the branch changes `tools/tier1/` or
   `tools/workflow/check/`, also run main's enforcement against it (FLOW-14):
   `make enforcement` (main's tier1 code built with the branch's
   `rules.yaml` and `golangci.yml`, so judge any edit to those two against
   the approved wording yourself), and `wf check -feature <F>` built from the merge base
   (a `git worktree add` of `git merge-base origin/main <branch>`, its
   `tools/workflow` built to a scratch `wf`, run with `-repo` on the branch
   checkout). Record each result. Record too, in the report, the trial
   merge's lines `dist/wf check` prints (FLOW-10) and those of
   `tools/tier1/area-edges.sh | dist/wf areas -state <workflow checkout> -check -rev HEAD -edges -`.
   A path outside the declared areas: widen it with `dist/wf declare` when
   the approved design covers it, otherwise `back-to-design` (FLOW-03,
   FLOW-06).
3. Tier 2: extract the rule index from `docs/development/*.md` (LINT-01: the
   `### <ID> — <title>` headers with their Severity and Check lines). For each
   `agent-judgment` rule and the judgment half of each `hybrid` rule, judge the
   diff, against the rules as the branch changes them. For each approved rule
   change, judge whether the edit matches the wording the Design review
   approved at its sha (FLOW-14). Each finding: rule ID, severity, `file:line`, what is wrong, a
   suggested fix. GLOBAL-09 to GLOBAL-11 are judged against the design's
   **Defaults and overrides** section: a default, merge or fallback in the
   diff that the section does not list is a finding under the rule that asks
   for it. Its fix is to remove it, or, where the work needs it,
   `back-to-design` (FLOW-06) to name it; code alone does not name one. A
   design without the section (a process design, or one older than the rule)
   is judged by its Decisions. A relay setting the diff delivers on a flag,
   environment variable, seed entry or kernel command-line key, other than
   RELAY-08's pre-claim values, is a RELAY-08 finding; its fix is to move it
   into the claim, or `back-to-design` (FLOW-06) when that needs a claim field
   the design did not plan. A tier-1 or tier-2 finding on behavior `main` already has,
   which the diff does not cause, is not a MUST on this feature: file it as
   the feature's follow-up by the `workflow-intake` skill's follow-up path,
   and list it in the report as "<slug> (follows <feature>)" (FLOW-09).
4. Write `reports/<feature>/<sha12>.json` and `.md` on the workflow checkout,
   `<sha12>` the home branch head's 12-character short hash (LINT-05: stable,
   diffable, counts by severity, suppressions with reasons carried over from
   the previous report). The JSON's `heads` maps each repository the report
   read, `<owner>/<name>`, to the 12-character head of the item's branch
   there; Review's check passes only when they are the item's current heads
   (LINT-04).
5. Each finding in the JSON has a `status`: `open`, or `fixed`, `resolved`
   or `suppressed` once it is; never leave it out. The merge gate reads a
   finding with no status, or any other, as open (FLOW-08, point 5).
   Fixing findings is not your job. Return the counts; the tick sends MUST
   findings back to `wf-implement` and records the rest for the PR body, where the human acknowledges them at merge
   (FLOW-08). Each MUST and SHOULD finding names the head it was found on.
   On a program item's PR into its program trunk (FLOW-20): a MUST finding
   still open after one fix round is deferred, a record of `-kind deferred
   -feature F -ref <finding id> -head <head> -note "<why>"`;
   a MUST finding under GLOBAL-06, a SEC rule, RELAY-01 to RELAY-03 or
   RELAY-05, or on what FLOW-20 says never breaks, is never deferred: return
   `back-to-design` with it, recorded `-kind protected`. Never dismiss one.
   When its `security_boundary` is not `none` and its review accepted that
   the only trip is GLOBAL-06's relay clause (a `security-note -ref design`
   record, a14), judge whether the diff stays within it: no new listener or
   egress path, key, CA or trust-store handling, SSRF filter, netns or
   iptables change, or socketpair topology change. If it does, a record of
   `-kind security-note -ref diff -feature F -head <head> -note "<why no
   posture changes>"` on each head you review, which merge
   gate point 7 reads; if it does not, it is a posture change: a MUST
   finding under GLOBAL-06, never deferred.

You write no program record yourself: "a record of" or "recorded" in this
file means you return the record's kind, feature, repository, head, ref and
note, and the tick writes it with `dist/wf record -state .dev/workflow
-worker $W` (its step 5, FLOW-20) before the item's next step, so merge
gate points 5 and 7 read it.

A program sync's conflict resolution (the tick dispatches you with the sync
head and `reports/sync/<head12>.json`): review only the resolution, the
merge commit against its first parent where it differs from what `git
merge-tree --write-tree <head>^1 <head>^2` merges, by steps 2 to 4, and
write the report at that path, `head` the head's 12-character hash and
every finding with its `status`. There is no feature, and a MUST finding
there is never deferred: the sync waits until it is fixed or escalated.

## The umbrella test (`f2-umbrella`, FLOW-20)

You did not write the design; if you did, stop and say so. Read the design
on the item's branch against `docs/design/componentization.md` and the
program records (`program/records.yaml` on the workflow checkout), and judge
each point with its evidence:

- U1: `related` lists `componentization.md`, and the design implements the
  components, seams and boundaries the program's design names for one phase
  row (P, S, K or M), or a filed split, follow-up or re-sequencing of them,
  within that row's Work and Done when; one that adds a component, moves a
  boundary or a component's ownership, or adds a seam the program does not
  name, fails.
- U2: no blocking question and no flagged assumed question, which on a
  program item only component design, setup or privileged work may be;
  each recommendation cites the umbrella decision or recorded answer of
  Jeff's it follows, or says it settles a question of strategy. Confirm each
  assumed question of strategy (a record of `-kind assumed`).
- U3: `rule_changes` is empty and `areas` reach no rule or machinery path of
  the profile.
- U4: `security_boundary` is `none`, or the only trip is GLOBAL-06's relay
  clause and the Security note shows no boundary's posture changes (read
  it; a record of `-kind security-note -ref design -feature F -head <sha>
  -note "<why only the relay clause>"`, which point 7 of the merge gate
  reads).
- U5: each interface break is one FLOW-20's Interfaces bullet allows.
- U6: `repos` and `areas` declared; where they are fewer than intake
  recorded, or intake recorded none, check them against the area map and
  `tools/tier1/area-edges.sh | dist/wf areas -state <workflow checkout>
  -check -edges -`, and record the check.
- U7: you agree, point by point.

A pass: add the Design review entry `r<N>, <date>: umbrella test passed at
<sha> (FLOW-20)` with each point's evidence, commit and push the branch,
record `-kind umbrella`, and return; the tick runs `dist/wf done`, which
checks the line. A failed U1 to U5: set the design's `design_review:
required`, commit and push, run `dist/wf review-human -state <workflow
checkout> -worker <W> -feature F -why "<the failed point>"`, which seeds
Jeff's review, and record it. A failed U6, or a disagreement on anything
but U1 to U5: return `back-to-design` with the finding, recorded.

## The process test (`x2-process-test`, FLOW-20)

You did not write the design; if you did, stop and say so. Judge:

- P1: the **Program payoff** section shows, with figures and how they were
  reached, the interactions it removes that decide nothing, per week, or the
  program time it saves over its own cost.
- P2: no rule FLOW-20 reserves to Jeff is touched. Read the change by its
  effect, not by the files it names, against the design's list of every
  rule, machinery path and configuration key it reaches. Reserved: any
  change that lets the loop take without him a step, merge, push, tag,
  filing or decision it could not before; removes or narrows a gate of his;
  loosens the umbrella test or the process test; relaxes or removes a check
  that stops the loop; raises a limit, budget or permission (`config.yaml`'s
  limits, budgets and `runaway_factor`, the allow lists of
  `.claude/settings*.json`); and any change to a rule about security
  (GLOBAL-06, the SEC rules, RELAY-01 to RELAY-03, RELAY-05, RELAY-08, the
  BOOT rules), merging to a trunk (FLOW-08, LINT-03), pushing (FLOW-16,
  FLOW-17) or release tags without a suffix (FLOW-17's tag clause,
  LOOP-07.5, the release workflows). A merge into a program trunk under
  FLOW-08's merge gate, and a pre-release or final tag FLOW-20 lets the
  loop create, are the loop's: a change to how it takes them is reserved
  only when it removes or narrows a point of the merge gate, lets the loop
  take two merges into one program trunk at once, lets a tag move or be
  deleted, or meets another clause above; a merge or tag the loop may
  already take is not one it could not take before, whatever worker or
  machinery takes it. P2 fails closed: a doubt is a failure.
- P3: you agree on P1 and P2, point by point.

A pass: add `r<N>, <date>: process test passed at <sha> (FLOW-20)` with each
point's evidence to the Design review section, commit and push, record
`-kind process-test`, and return. A failed or doubted P2, or any
disagreement touching it: set `design_review: required`, commit and push,
and `dist/wf review-human ... -why "<P2's finding>"` (Jeff's review). A
failed P1, or a disagreement on P1 alone: `dist/wf hold -state <workflow
checkout> -worker <W> -feature F -why "<P1's finding>" -resumes
program-end`, recorded, and release the step.

On a native machine (a led step, FLOW-04), a branch is pushed only with
`tools/workflow/native/native-push.sh <name>`, or `native-push.sh <name>
<owner>/<repo>` for a branch in a second repository's clone, whose worktree
is `.scratch/<owner>/<repo>/<name>`, made with `git -C
.dev/checkouts/<owner>/<repo> worktree add`; and committed with `git -C
<worktree> commit -F ../<name>.msg`, the message written beside the
worktree, never `-m` (FLOW-17).
