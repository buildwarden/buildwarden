---
name: workflow-native
description: A native-validation session on a native machine (macbook, windows-desktop). Drains that machine's host steps (macos-vz, macos-qemu, macos-finch, windows-hyperv, qemu-any, container-any, ...) and native-gap backlog in one batch, testing main plus every waiting branch. Run as "/workflow-native <host>" attended, or as "/workflow-native unattended <machine>" by the scheduled native session.
---

# Native-validation session

The `host:<name>` worker of FLOW-04, doing tier-2 validation that CI cannot
(FLOW-09, TEST-02, TEST-03), and the design or implementation steps a design
gives host affinity (FLOW-09). The procedure is here; the rules are there
(FLOW-14). Which host names a machine serves is `hosts.yaml` on the workflow
branch.

Arguments:

- `<host>`: attended, one host name; ask which host this is if it is missing.
- `unattended <machine>`: started by `tools/workflow/native/native-tick.sh`
  after `wf gate` found work for the machine. Every host the machine serves in
  `hosts.yaml`; nobody is watching.

## Never (unattended)

The `workflow-tick` skill's Never list applies unchanged: no merge, approval or
auto-merge; no push to `main`, force-push or remote branch delete; no edits to
`docs/development/`, `.claude/` or `tools/workflow/`; no secrets in prompts.
Also: never take `human` or `cloud` steps, never ask a question and wait (record
it in the queue and the digest instead), and never change anything on the
machine outside the checkout, the documented caches, and the VMs, images and
containers this session creates.

## Git on the workflow checkout (unattended)

Nobody is there to approve a prompt, and `settings.native.json` allows git on
the workflow checkout only in this form, run from the checkout root:
`git -C .dev/workflow <add|commit|push origin workflow|...>`. Never an
absolute path, and never `cd .dev/workflow && git ...`. A session that still
cannot commit or push leaves its changes for the wrapper, which saves them and
reports the run as failed.

## Set up

On the machine itself. The FLOW-18 check, `dist/wf` and the `.dev/workflow`
checkout as in `workflow-tick` step 1; the scheduled wrapper has already done
this. Workers: one per host, `W=host:<name>/<UTC timestamp>` (unattended: the
session id in the prompt as the timestamp).

## Collect the batch

For each host this session serves:

- Queue steps: repeat `dist/wf next -state .dev/workflow -worker $W` and claim
  each (`dist/wf claim`), pushing the workflow branch after each claim. A
  rejected push lost the claim (FLOW-04): reset to `origin/workflow`, ask again.
- Backlog: every `open` entry for this host in `backlog/native-gaps.yaml`.

Unattended, the batch is bounded by the native budget,
`dist/wf budget -state .dev/workflow -native` (`native_budget` in
`config.yaml`, falling back to `run_budget`; FLOW-13), counting claimed steps
and backlog entries together, taken in FLOW-05 order
for steps and file order for the backlog. Queue steps come before the backlog.

## Work

Validation steps and backlog entries: test in one sitting, `main` first, then
each waiting branch (FLOW-09), using the commands each design's Validation
plan names and `make deadcode` where the backlog asks for it.

Design or implementation steps with host affinity: check out the feature
branch and dispatch to the phase agent as in `workflow-tick` step 2.4, then
finish as in its step 2.5. Push the feature branch at each green checkpoint
(FLOW-17).

## Record

- A queue step that passes: `dist/wf done ... -step S` with the evidence
  (commands, versions, short log excerpt) in the queue history.
- A failure: attach the logs under `native/<host>/<date>/` on the workflow
  branch, release the step with the failure as the reason, and return the
  feature with `dist/wf back` (a design gap) or add a fix item with the
  `workflow-intake` skill (an implementation bug; unattended, record the
  proposed item in the digest for Jeff instead).
- Backlog entries: set `status: done` with the date and evidence, or keep them
  open with what was found.
- `dist/wf validate -state .dev/workflow` must pass before each push.

## Close

Clean up test images, VMs, containers and scratch output outside the documented
caches (FLOW-16). Switch the checkout back to `main`. Commit and push the
workflow branch; never push `main` or merge anything (FLOW-08, FLOW-17).

Attended: finish with a summary per branch.

Unattended: append a `## Run host:<machine>/<timestamp>` section to
`.dev/workflow/digest/<YYYY-MM-DD>.md` (America/Los_Angeles date), with
**Moved** (per host and branch: what ran, pass or fail, evidence),
**Blocked on Jeff** (anything this machine cannot do: a missing tool, a
failure that needs a decision, a proposed fix item), **Spent** (steps and
backlog entries against the native budget), and **Notes** (FLOW-18 check,
whether Entire recorded the session, free disk at the end). Commit and push it.
If a feature was unblocked or returned to Design, or a backlog entry failed,
notify Jeff (the PushNotification tool when available) naming the feature and
outcome. End with the digest section as the final message.
