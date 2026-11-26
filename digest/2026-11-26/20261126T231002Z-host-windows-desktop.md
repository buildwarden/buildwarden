## Run host:windows-desktop/20261126T231002Z

Unattended, main 7535e6ca (#146 wf-gate-rest merged since this machine's last worked run), Git Bash. Hosts:
container-any, linux-kvm and windows-any were worked. **windows-hyperv was skipped this run** because the
wrapper's native.prepare (the Hyper-V relay boot VHDX refresh) failed again, so the wrapper marked that
host stale. Its open gaps were not touched. No tags were due (`wf tags-due` printed nothing).

### Moved

- **Queue steps:** none. `wf next` offered no step to any served host because every candidate is blocked:
  - sync-pr-tier1 waits on your merge of PR #145;
  - finch-version-starts-vm and relay-release-forms-spec wait on human f2-design-review;
  - prebuilt-artifacts, workdir-in-context and relay-collector-failure-fails-open wait on human f2-answer-q1.

  No led step, and no lead set or cleared.
- **Branches:** program/componentization is at e9a9073d (#147). feat/prebuilt-artifacts (49b513a9) and
  feat/workdir-in-context (24376c04) haven't moved and are still design-only. Nothing was built or rerun.
- **windows-any: prebuilt-artifacts-windows, not run.** The branch is design-only, held under FLOW-20 (q1).
- **windows-any: workdir-in-context-windows, not run.** Same reason: design-only, held under FLOW-20 (q1).
- **windows-any: native-loop-fixes-windows, partly observed. Kept open.**
  - Digest check and stamp **pass** under Git Bash. The wrapper found the 2026-11-24T23:10Z run's
    digest and stamped `outcome: worked` (916d9b45). Later runs got idle stamps.
  - The "sweep skipped" reason is not in the stamp. The run log is outside the session's readable
    directories, so it could not be checked.
  - The program-trunk fingerprint is still not isolated: program/componentization (#147) and main (#146)
    moved a minute apart.
  - Cleanup found nothing to remove: no `.scratch/` worktree, no detached worktree, no gone program
    branch, and no leftovers were named.

### Blocked on Jeff

- **windows-hyperv is still stale:** native.prepare's relay boot VHDX refresh failed again. This is the
  same cause as on 2026-11-24. Until it succeeds, this machine skips windows-hyperv's steps and gaps
  (loop-native-lead-windows-wrapper, loop-native-lead-cleanup).
- Merge PR #145 (sync-pr-tier1).
- native-loop-fixes-windows' "sweep skipped" reason can only be checked in the wrapper log
  (`~/.local/state/buildwarden-native/logs/`), which the session cannot read. Either check it by hand or
  allow the session to read that directory.

### Follow-ups filed

none

### Spent

0 queue steps + 3 backlog entries (prebuilt-artifacts-windows, workdir-in-context-windows,
native-loop-fixes-windows) = 3 of the native budget of 8.

### Notes

- **FLOW-18:** the wrapper's check passed, and this prompt didn't say "sessions not recorded". Entire is
  enabled for this repository (`gh/buildwarden/buildwarden`, from the session hook).
- **Free disk at the end:** 911 GiB on C:.
- **Cleanup:** nothing was built or started, so there are no containers, VMs, images or scratch output to
  clean up. The checkout is on main.
- **Wrapper leftovers:** none named.
