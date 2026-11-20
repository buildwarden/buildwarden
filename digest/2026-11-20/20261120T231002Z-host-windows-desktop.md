## Run host:windows-desktop/20261120T231002Z

Unattended native session on windows-desktop (hosts `windows-hyperv`, `container-any`, `linux-kvm`, `windows-any`). The prompt named **windows-hyperv as stale** (`native.prepare` failed: the cached Hyper-V asset did not refresh), so this run took no windows-hyperv step or backlog entry and worked the other three hosts. It named no leftovers and did not say sessions were not recorded. `wf next` offered no step to container-any, linux-kvm or windows-any. driver-contract waits on its f6-validate steps (its container-any step is leased to another worker, `host:container-any/20261120T225616Z`); prebuilt-artifacts and workdir-in-context are held on human step f2-answer-q1; relay-collector-failure-fails-open waits on human step f2-design-review. Nothing was claimed and no item is led by these hosts. Main is still be13847, the same as the 22:40Z run, and neither waiting branch moved. No lead was set or cleared, no code or specification changed, and only the `workflow` branch was pushed.

### Moved

- **container-any, backlog `prebuilt-artifacts-windows`**: not run, nothing changed. feat/prebuilt-artifacts is still 49b513a, design only, held on q1. A dated note was added.
- **container-any, backlog `workdir-in-context-windows`**: not run, nothing changed. feat/workdir-in-context is still 24376c0, design only, held on f2-answer-q1. A dated note was added.
- **windows-hyperv**: skipped as stale (`native.prepare` failed). `loop-native-lead-windows-wrapper` and `loop-native-lead-cleanup` were not touched, and driver-contract's f6-validate-windows-hyperv was not taken.
- **linux-kvm, windows-any**: no step and no open entry. `firecracker-distro-firewalld` is deferred and was not retried.
- No review step ran, so this run has no tier-1 or tier-2 result. No lead was set or cleared.

### Blocked on Jeff

- windows-hyperv's cached asset failed to refresh (`native.prepare`). Until it does, driver-contract's blocking f6-validate-windows-hyperv cannot run on this machine. The wrapper's prepare log has the cause.

### Follow-ups filed

- none

### Spent

- 0 queue steps and 2 backlog entries checked (none run), against a native budget of 8.

### Notes

- FLOW-18 check: the scheduled wrapper ran it before the session started. `wf validate` passed before the push.
- Entire: enabled on this repository. This run made no commit on a feature branch, so Entire has no checkpoint to record.
- The prompt named no leftovers.
- Cleanup: this run created no VMs, images, containers or worktrees. There is no `.scratch/` directory, and the checkout is on `main`, clean.
- Free disk at the end: 881 GiB on C:.
