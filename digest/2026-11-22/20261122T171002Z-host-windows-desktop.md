## Run host:windows-desktop/20261122T171002Z

Unattended native session on windows-desktop, covering hosts `windows-hyperv`, `container-any`, `linux-kvm` and `windows-any`. The prompt marked **windows-hyperv as stale** because `native.prepare` failed (the cached Hyper-V asset did not refresh). This run took no windows-hyperv step or backlog entry and worked the other three hosts. The prompt named no leftovers and did not say that sessions were not recorded.

`wf next` offered no step to any of the machine's hosts. Each item is waiting on the human:

- config-lookup-test-symlinked-tmp: the human's merge of PR #134 (FLOW-08)
- driver-contract: human step f2-design-review-r3
- prebuilt-artifacts: human step f2-answer-q1 (FLOW-20 hold)
- workdir-in-context: human step f2-answer-q1 (FLOW-20 hold)
- relay-collector-failure-fails-open: human step f2-answer-q1 (FLOW-20 hold)

Nothing was claimed, and none of these hosts leads an item. Main is still be13847, and neither waiting branch moved. No lead was set or cleared and no code or specification changed. Only the `workflow` branch was pushed.

### Moved

- **container-any, backlog `prebuilt-artifacts-windows`**: not run. feat/prebuilt-artifacts is still 49b513a, design only, under the FLOW-20 hold. A dated note was added.
- **container-any, backlog `workdir-in-context-windows`**: not run. feat/workdir-in-context is still 24376c0, design only, under the same FLOW-20 hold. A dated note was added.
- **windows-hyperv**: skipped because the host is stale (`native.prepare` failed). Its open entries (`loop-native-lead-windows-wrapper`, `loop-native-lead-cleanup`) were not touched.
- **linux-kvm, windows-any**: no step and no open entry. `firecracker-distro-firewalld` is deferred and was not retried.
- No review step ran, so this run has no tier-1 or tier-2 result. No lead was set or cleared.

### Blocked on Jeff

- windows-hyperv's cached asset still fails to refresh (`native.prepare`); this is the seventh worked run in a row since 2026-11-20 23:10Z. driver-contract's blocking f6-validate-windows-hyperv step cannot run on this machine until the refresh works. The cause is in the wrapper's prepare log.
- Both open container-any entries wait on the FLOW-20 freeze; until it ends, this machine's scheduled runs have no runnable work outside windows-hyperv.

### Follow-ups filed

- none

### Spent

- 0 queue steps and 2 backlog entries checked (neither run), against a native budget of 8.

### Notes

- FLOW-18 check: the scheduled wrapper ran it before the session started. `wf validate` passed before the push.
- Entire: enabled on this repository. This run made no commit on a feature branch, so there is no checkpoint; `entire status` was not re-checked this run (the last run saw "Claude Code hooks out of date · run 'entire doctor'", a setup command left for Jeff).
- Leftovers: the prompt named none.
- Cleanup: this run created no VMs, images, containers or worktrees. The checkout is on `main` and clean.
- Free disk at the end: 875 GiB on C:.
