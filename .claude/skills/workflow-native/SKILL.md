---
name: workflow-native
description: A native-validation session on one host (macos-vz, macos-qemu, macos-finch, windows-hyperv, ...). Drains that host's queue steps and native-gap backlog in one batch, testing main plus every waiting branch. Run as "/workflow-native <host>" on that machine.
---

# Native-validation session

The `host:<name>` worker of FLOW-04, doing tier-2 validation that CI cannot
(FLOW-09, TEST-02, TEST-03). The procedure is here; the rules are there
(FLOW-14).

## Set up

On the host itself. Same set-up as `workflow-tick` step 1. Worker:
`W=host:<name>/<UTC timestamp>`; `<name>` is the argument, or ask which host
this is.

## Collect the batch

- Queue steps: repeat `dist/wf next -state .dev/workflow -worker $W` and claim
  each (`dist/wf claim`), pushing the workflow branch after each claim.
- Backlog: every `open` entry for this host in `backlog/native-gaps.yaml`.

Test in one sitting, `main` first, then each waiting branch (FLOW-09), using
the commands each design's Validation plan names and `make deadcode` where the
backlog asks for it.

## Record

- A queue step that passes: `dist/wf done ... -step S` with the evidence
  (commands, versions, short log excerpt) in the queue history.
- A failure: attach the logs under `native/<host>/<date>/` on the workflow
  branch, release the step with the failure as the reason, and return the
  feature with `dist/wf back` (a design gap) or add a fix item with the
  `workflow-intake` skill (an implementation bug).
- Backlog entries: set `status: done` with the date and evidence, or keep them
  open with what was found.

Clean up test images, containers and scratch output outside the documented
caches (FLOW-16). Commit and push the workflow branch; never push `main` or
merge anything (FLOW-08, FLOW-17). Finish with a summary per branch.
