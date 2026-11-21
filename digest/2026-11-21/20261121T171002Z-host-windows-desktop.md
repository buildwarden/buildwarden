## Run host:windows-desktop/20261121T171002Z

Unattended native session on windows-desktop, covering hosts `windows-hyperv`, `container-any`, `linux-kvm` and `windows-any`. The prompt marked **windows-hyperv as stale** because `native.prepare` failed (the cached Hyper-V asset did not refresh). This run took no windows-hyperv step or backlog entry and worked the other three hosts. The prompt named no leftovers and did not say that sessions were not recorded.

`wf next` offered no step to container-any, linux-kvm or windows-any. Each item is waiting on a human step:

- config-lookup-test-symlinked-tmp: f2-design-review
- driver-contract: f1-escalate-3
- prebuilt-artifacts: f2-answer-q1
- workdir-in-context: f2-answer-q1
- relay-collector-failure-fails-open: f2-design-review

Nothing was claimed, and none of these hosts leads an item. Main is still be13847, and neither waiting branch moved. No lead was set or cleared and no code or specification changed. Only the `workflow` branch was pushed.

### Moved

- **container-any, backlog `prebuilt-artifacts-windows`**: not run, nothing changed. feat/prebuilt-artifacts is still 49b513a, design only, and is held on q1 (driver-contract waits on f1-escalate-3). A dated note was added.
- **container-any, backlog `workdir-in-context-windows`**: not run, nothing changed. feat/workdir-in-context is still 24376c0, design only, and is held on f2-answer-q1. A dated note was added.
- **windows-hyperv**: skipped because the host is stale (`native.prepare` failed). `loop-native-lead-windows-wrapper` and `loop-native-lead-cleanup` were not touched.
- **linux-kvm, windows-any**: no step and no open entry. `firecracker-distro-firewalld` is deferred and was not retried.
- No review step ran, so this run has no tier-1 or tier-2 result. No lead was set or cleared.

### Blocked on Jeff

- windows-hyperv's cached asset still fails to refresh (`native.prepare`). This is the fourth run in a row: 23:10Z, 05:10Z, 11:10Z and now 17:10Z. When driver-contract reaches Publish again, its blocking f6-validate-windows-hyperv step cannot run on this machine until the refresh works. The cause is in the wrapper's prepare log.

### Follow-ups filed

- none

### Spent

- 0 queue steps and 2 backlog entries checked (neither run), against a native budget of 8.

### Notes

- FLOW-18 check: the scheduled wrapper ran it before the session started. `wf validate` passed before the push.
- Entire: enabled on this repository, with checkpoints going to buildwarden/buildwarden-sessions. This run made no commit on a feature branch, so there is no checkpoint. `entire status` reports "Claude Code hooks out of date · run 'entire doctor'". That is a setup command and was left for Jeff.
- Leftovers: the prompt named none.
- Cleanup: this run created no VMs, images, containers or worktrees. `.scratch/` is empty, and the checkout is on `main` and clean.
- Free disk at the end: 879 GiB on C:.
