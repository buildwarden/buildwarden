## Run host:windows-desktop/20261116T111001Z

Unattended native session on windows-desktop (hosts `windows-hyperv`, `container-any`, `linux-kvm`). No stale hosts were named. No queue step was offered to any of the three hosts, so the run worked the backlog only: two open entries, both on `container-any`. One was rerun on `main` 8c41eaa and `feat/project-config-trust` ce32836 and is now done; the other is not runnable yet. No code, specification or rule changed, no item moved phase, and no branch was pushed but `workflow`.

### Moved

- **container-any, backlog `project-config-trust-windows`** (project-config-trust: feature, security fix, in Publish; branch head ce32836, untouched): **passes, entry done.** Windows 11 Pro 26300, go1.26.3.
  - The one case that failed on 2026-11-15 was rerun: `go test -count=1 ./cmd/warden ./driver` is ok on `main` and on the branch, and `TestLoadUpstreamCAsEscapes`, run alone with `-v`, reports PASS on the branch (it ran; it was not skipped).
  - The other cases (the former user file, `--config` over `.\warden.toml`, `WARDEN_UPSTREAM_CA_CERTS` with two files, `--config NUL`, the podman builds) were **not rerun**. They passed in the 05:10Z run on 3733c4b and `main` 623131a. Since then the branch changed `driver/upstreamca_test.go` only (ce32836) and `main` two `.claude/skills` files only (#122), so the warden those cases ran is the same code. Say so if you want them rerun on ce32836 anyway.
  - Evidence: `native/container-any/2026-11-16/project-config-trust-go-test.log`; the earlier cases: `native/container-any/2026-11-15/project-config-trust/`.
- **container-any, backlog `prebuilt-artifacts-windows`**: not run, nothing changed. `feat/prebuilt-artifacts` is still 49b513a (design only), and the item is held in Plan on q1.
- **windows-hyperv**: no open entry and no step.
- **linux-kvm**: nothing open (`firecracker-distro-firewalld` is deferred and was not retried).
- Check results: no review step ran, so no tier-1 or tier-2 result this run.

### Blocked on Jeff

- Nothing new from this run. Still standing from the 05:10Z run: the stale `dist/warden.exe` (2026-11-04) in this machine's checkout, left as found (delete it, or say the native session may).
- `prebuilt-artifacts-windows` stays `open` while its item is held on q1, so this machine's gate keeps finding work it cannot run. Deferring the entry until the hold lifts would stop that.

### Follow-ups filed

- none

### Spent

- 0 queue steps and 2 backlog entries, against a native budget of 8.

### Notes

- FLOW-18 check: ok at the start and at the end of the run.
- Entire: enabled and the session is listed as active (`entire status`), with "Claude Code hooks out of date" reported; whether the checkpoint reaches buildwarden-sessions was not checked.
- Nothing outside the checkout was touched: no VM, container, image or podman machine was started. The scratch worktree (`.scratch/pct`) was removed; the checkout is on `main`, clean. Free disk at the end: 884G of 1.9T on C:.
