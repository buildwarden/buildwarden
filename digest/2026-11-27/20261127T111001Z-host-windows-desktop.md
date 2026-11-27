## Run host:windows-desktop/20261127T111001Z

Unattended, main 7535e6ca (unchanged since the last run), Git Bash. Hosts: container-any, linux-kvm and
windows-any were worked. **windows-hyperv was skipped this run** because the wrapper's native.prepare (the
Hyper-V relay boot VHDX refresh) failed again, so the wrapper marked that host stale; `wf native -check`
also warns on `tools/relay-vm/hyperv/refresh-cache.sh --check`. Its open gaps were not touched. No tags
were due (`wf tags-due` printed nothing).

### Moved

- **Queue steps:** none. `wf next` offered no step to any served host because every candidate is blocked:
  - sync-pr-tier1 waits on your merge of PR #145;
  - finch-version-starts-vm and relay-release-forms-spec wait on human f2-design-review;
  - prebuilt-artifacts, workdir-in-context and relay-collector-failure-fails-open wait on human f2-answer-q1.

  No led step, and no lead set or cleared.
- **Branches:** program/componentization moved to d88e25fe (#148, the loop's first REST merge, 11:05Z).
  feat/prebuilt-artifacts (49b513a9) and feat/workdir-in-context (24376c04) haven't moved and are still
  design-only. Nothing was built or rerun.
- **windows-any: prebuilt-artifacts-windows, not run.** The branch is design-only, held under FLOW-20 (q1).
- **windows-any: workdir-in-context-windows, not run.** Same reason: design-only, held under FLOW-20 (q1).
- **windows-any: native-loop-fixes-windows, partly observed. Kept open.**
  - Program-trunk fingerprint, nearly isolated: the 05:10Z run stamped idle ("nothing moved since
    2026-11-26T23:10:19Z"). Since then main and the waiting branches are unchanged, program/componentization
    moved (#148), and this run was woken. It is not fully isolated because the session cannot read the
    second repositories' trunk move times or the gate's reason line in the run log.
  - The "sweep skipped" reason is still not in `native/status/windows-desktop.yaml`.
  - Cleanup found nothing to remove: no `.scratch/` worktree or leftover run, no detached worktree, no gone
    program branch.

### Blocked on Jeff

- **windows-hyperv is still stale:** native.prepare's relay boot VHDX refresh failed again (as on
  2026-11-24 and 2026-11-26). Until it succeeds, this machine skips windows-hyperv's steps and gaps
  (loop-native-lead-windows-wrapper, loop-native-lead-cleanup).
- Merge PR #145 (sync-pr-tier1).
- native-loop-fixes-windows can only be closed with the wrapper log
  (`~/.local/state/buildwarden-native/logs/20261127T111001Z.log`: the gate's wake reason and the "sweep
  skipped" line), which the session cannot read. Either check it by hand or allow the session to read
  that directory.

### Follow-ups filed

none

### Spent

0 queue steps + 3 backlog entries (prebuilt-artifacts-windows, workdir-in-context-windows,
native-loop-fixes-windows) = 3 of the native budget of 8.

### Notes

- **FLOW-18:** the wrapper's check passed, and this prompt didn't say "sessions not recorded". Entire is
  enabled for this repository (`gh/buildwarden/buildwarden`, from the session hook); `wf native -check`
  shows Entire's hooks in the checkout and all eight clones.
- **Free disk at the end:** 910 GiB on C:.
- **Cleanup:** nothing was built or started, so there are no containers, VMs, images or scratch output to
  clean up. The checkout is on main.
- **Wrapper leftovers:** none named.
