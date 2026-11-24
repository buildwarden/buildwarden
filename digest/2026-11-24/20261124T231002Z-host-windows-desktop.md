## Run host:windows-desktop/20261124T231002Z

Unattended, main 66e6f670 (#137 to #144 merged since this machine's last worked run), Git Bash. Hosts:
container-any, linux-kvm and windows-any were worked. **windows-hyperv was skipped this run** because the
wrapper's native.prepare (`tools/relay-vm/hyperv/refresh-cache.sh`, to refresh the cached Hyper-V relay
boot VHDX) failed, so the wrapper marked that host stale. Its gaps (loop-native-lead-windows-wrapper,
loop-native-lead-cleanup) were not touched. No tags were due.

### Moved

- **Queue steps:** none. `wf next` offered no step to any served host because every candidate is blocked:
  - sync-pr-tier1 and wf-gate-rest wait on your merge of PRs #145 and #146;
  - finch-version-starts-vm and relay-release-forms-spec wait on human f2-design-review;
  - prebuilt-artifacts, workdir-in-context and relay-collector-failure-fails-open wait on human f2-answer-q1.

  No led step, and no lead set or cleared.
- **Branches:** program/componentization is at 10d1f99e. feat/prebuilt-artifacts (49b513a9) and
  feat/workdir-in-context (24376c04) haven't moved and are still design-only. Nothing was built or rerun.
- **windows-any: prebuilt-artifacts-windows, not run.** The branch is design-only, held under FLOW-20 (q1).
- **windows-any: workdir-in-context-windows, not run.** Same reason: design-only, held under FLOW-20 (q1).
- **windows-any: native-loop-fixes-windows, partly observed. Kept open.**
  - This is the first Windows session since #141 merged.
  - Under Git Bash, hand_on copied nine saved leftovers to `.scratch/leftover/<run>/`, and the prompt
    named each of them as Unfinished.
  - The gate woke on "a trunk, a program trunk or a waiting branch moved". main also moved, so this doesn't
    isolate a program-trunk-only move.
  - The cleanup noted nothing because there was nothing here to clean.
  - The digest check, the stamp and the "sweep skipped" reason run after the session exits. The next
    windows-desktop run can read them in `logs/20261124T231002Z.log` and `native/status/windows-desktop.yaml`.

### Blocked on Jeff

- **windows-hyperv is stale:** `refresh-cache.sh` failed in native.prepare. The wrapper log has only the
  stale line, with no cause. Until the Hyper-V relay boot VHDX refresh succeeds, this machine skips
  windows-hyperv's steps and gaps.
- **The wrapper was down for four runs before this one** (2026-11-23T23:10Z, and 05:10Z, 11:10Z and 17:10Z
  today). Each stopped before the gate with "no clone of buildwarden/buildwarden-driver-container". The
  clone is now present: `wf native -check` passes for all eight clones. This needs nothing from you
  unless you didn't add it.
- Merge PRs #145 (sync-pr-tier1) and #146 (wf-gate-rest).

### Follow-ups filed

none

### Spent

0 queue steps + 3 backlog entries (prebuilt-artifacts-windows, workdir-in-context-windows,
native-loop-fixes-windows) = 3 of the native budget of 8.

### Notes

- **Unfinished runs:** the prompt named runs 20261103T111001Z, 20261108T111002Z, 20261108T231002Z,
  20261109T171002Z, 20261110T171002Z, 20261110T231002Z, 20261111T111002Z, 20261112T231002Z and
  20261115T231002Z. Each saved leftover is empty: uncommitted.patch is 0 bytes, commits/ is empty, and
  untracked.tar is the 10240-byte empty archive (md5 1276481102f218c981e0324180bafd9f, the same for all
  nine). So there was no step or backlog evidence to record from them.
- **FLOW-18:** the wrapper's check passed, and this prompt didn't say "sessions not recorded". Entire is
  enabled, syncs checkpoints to buildwarden/buildwarden-sessions, and lists this session as active.
- **Free disk at the end:** 904 GiB on C:.
- **Cleanup:** nothing was built or started, so there are no containers, VMs, images or scratch output to
  clean up. The checkout is on main.
- **Wrapper leftovers:** none named.
