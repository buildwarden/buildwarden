## Run host:windows-desktop/20261119T171002Z

Unattended native session on windows-desktop (hosts `windows-hyperv`, `container-any`, `linux-kvm`). The prompt named no stale hosts and no leftovers. `wf next` offered one queue step, detached-prepare-windows-exe's blocking `f6-validate-windows-hyperv`. It was claimed and passed: all five cases of the design's Validation plan passed. The two open container-any entries could not run (their branches are design-only and held) and got dated notes. No lead was set or cleared, no code or specification changed, and only the `workflow` branch was pushed.

### Moved

- **windows-hyperv, detached-prepare-windows-exe `f6-validate-windows-hyperv`**: PASS. This is a feature item, FLOW-05 class: a follow-up of project-config-trust, in phase 6 (publish). The branch head stayed at b4f86e2 (merged with main e9315c2). It was built with `make build`, so the binary is `dist/warden` with no extension. Test setup: Windows 11 Pro 26300, go1.26.3, Hyper-V Administrators, not elevated. Each build ran with `[vm] image_refresh_after = "1m"` given by `--config`.
  1. `go test ./driver/ ./driver/hyperv/ ./driver/qemu/`: ok on Windows. This includes TestStartDetachedRunsNamedFile and TestStartDetachedFailedStart.
  2. No `warden.exe` in `dist/`:
     - The build printed "refreshing it in the background" and SUCCEEDED.
     - The background prepare ran as `...\dist\warden --config <file> image prepare --driver hyperv`.
     - `prepare.log` starts with the config line.
     - The prepare finished (sha256:007b40a02ce9) and the new base is active.
     - A build run during the prepare started no second one.
  3. With a `warden.exe` beside `dist/warden`: the prepare started from `dist\warden`, not from `warden.exe`.
  4. With `prepare.log` locked (FileShare None): the build printed the new line, which names the open error and `warden image prepare --driver hyperv`. The build still SUCCEEDED and no prepare started. One cosmetic flaw: the Windows error text ends in a period, so the line reads "process.; run". Nothing was filed for it.
  5. Run as `dist/warden.exe`: the prepare ran from `dist\warden.exe` and finished.

  Evidence: `native/windows-hyperv/2026-11-19/` (README, logs, scratch Makefile, scripts). After this step the item waits on cloud step f6-open-pr.
- **container-any, backlog `prebuilt-artifacts-windows`**: not run. feat/prebuilt-artifacts is still 49b513a, design only, held on q1. A dated note was added.
- **container-any, backlog `workdir-in-context-windows`**: not run. feat/workdir-in-context is still 24376c0, design only, held on f2-answer-q1. A dated note was added.
- **linux-kvm**: no step and no open entry. `firecracker-distro-firewalld` is deferred and was not retried.
- No review step ran, so this run has no tier-1 or tier-2 result. No lead was set or cleared.

### Blocked on Jeff

- Nothing new from this run.

### Follow-ups filed

- none

### Spent

- 1 queue step and 2 backlog entries, against a native budget of 8.

### Notes

- FLOW-18 check: the scheduled wrapper ran it before the session started. `wf validate` passed before each push.
- Entire: enabled on this repository. It reports "Claude Code hooks out of date". This was left alone, because `entire doctor` is a setup command.
- The prompt named no leftovers.
- Cleanup:
  - The `.scratch/dpwe` worktree and the `.scratch/dpwe-nv` scratch output were removed.
  - No warden-* VMs are left, and both slots are free.
  - Three prepared layers were made this run. The prepare retired the older ones itself, and the latest is windows-server-2025-20261119T172521Z, whose grace ends on 2026-11-29.
  - The checkout is on `main`, clean.
  - Free disk at the end: 883 GiB on C:.
