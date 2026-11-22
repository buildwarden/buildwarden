## Run host:windows-desktop/20261122T231002Z

Unattended native session on windows-desktop, covering hosts `windows-hyperv`, `container-any`, `linux-kvm` and `windows-any`. The prompt marked **windows-hyperv as stale** because `native.prepare` failed (the cached Hyper-V asset did not refresh), so this run took no windows-hyperv step or backlog entry and worked the other three hosts. The prompt named no leftovers and did not say that sessions were not recorded.

`wf next` offered no step to any of the machine's hosts. Every item is waiting on another worker:

- config-lookup-test-symlinked-tmp: cloud step f8-close (merged as #134)
- driver-contract: cloud step f8-close (merged as #135)
- program-north-star: human step x4-state (merged as #136)
- prebuilt-artifacts, workdir-in-context, relay-collector-failure-fails-open: human step f2-answer-q1 (FLOW-20 hold)

Nothing was claimed, and none of these hosts leads an item. main moved from be13847 to fa1dc64 (#134 to #136), but neither waiting branch moved. No lead was set or cleared, and no code or specification changed. Only the `workflow` branch was pushed.

### Moved

- **container-any, backlog `prebuilt-artifacts-windows`**: not run. feat/prebuilt-artifacts is still 49b513a (design only), under the FLOW-20 hold. Added a dated note.
- **container-any, backlog `workdir-in-context-windows`**: not run. feat/workdir-in-context is still 24376c0 (design only), under the FLOW-20 hold. Added a dated note.
- **windows-hyperv**: skipped because the host is stale (`native.prepare` failed). Its open entries (`loop-native-lead-windows-wrapper`, `loop-native-lead-cleanup`) were not touched.
- **linux-kvm, windows-any**: no step and no open entry. `firecracker-distro-firewalld` is deferred and was not retried.
- No review step ran, so there is no tier-1 or tier-2 result. No lead was set or cleared.

### Blocked on Jeff

- windows-hyperv's cached asset still fails to refresh (`native.prepare`). This is the eighth worked run in a row with the host stale, counting from 2026-11-20 23:10Z. The cause is in the wrapper's prepare log. driver-contract's windows-hyperv validation is moot now that #135 has merged, but no later Hyper-V step can run here until the refresh works.
- Both open container-any entries wait on the FLOW-20 hold. Until it ends, this machine's scheduled runs have no runnable work outside windows-hyperv.

### Follow-ups filed

- none

### Spent

- 0 queue steps and 2 backlog entries checked (neither run), against a native budget of 8.

### Notes

- FLOW-18 check: the scheduled wrapper ran it before the session started. `wf validate` passed before the push.
- Entire: enabled on this repository. This run made no commit on a feature branch, so it left no checkpoint. `entire status` was not re-checked. An earlier run saw "Claude Code hooks out of date · run 'entire doctor'", a setup command left for Jeff.
- Leftovers: the prompt named none.
- Cleanup: this run created no VMs, images, containers or worktrees. The checkout is on `main` and clean.
- Free disk at the end: 883 GiB on C:.
