## Run host:windows-desktop/20261116T051002Z

Unattended native session on windows-desktop (hosts `windows-hyperv`, `container-any`, `linux-kvm`). No stale hosts were named. No queue step was offered to any of the three hosts, so the run worked the backlog only: three open entries, two run on `main` 623131a and `feat/project-config-trust` 3733c4b, one not runnable yet. One item went back to Implement and one follow-up was filed. No code, specification or rule changed, and no branch was pushed but `workflow`.

### Moved

- **project-config-trust** (feature, security fix; FLOW-05 class 1, in implementation): **returned from Publish to Implement** (`wf back -to 3`, now `f3-implement-r3`), because its tier-2 gap `project-config-trust-windows` failed on a test the branch adds. Branch head left at 3733c4b; no PR was open. The gap is tracked, not blocking, but FLOW-09 returns a failure the branch causes.
- **container-any, backlog `project-config-trust-windows`**: run; **FAILED on its first case, every other case passes; the entry stays open.** Windows 11 Pro 26300, go1.26.3, podman 5.8.3, no user config file and no `WARDEN_*` variable on the host.
  - `go test -count=1 ./cmd/warden ./driver`: ok on `main`. On the branch `./cmd/warden` is ok and `./driver` **fails** in `TestLoadUpstreamCAsEscapes` (`driver/upstreamca_test.go:503-512`). The test gives a path that starts `/no/such` and holds a newline, an escape sequence and a 0xff byte, and expects the same string back, escaped. On Windows the path is made absolute, so the message names `C:\no\such...`, and the 0xff byte comes back as U+FFFD. The newline and the escape sequence are still escaped, so the product looks right and the test's expected string is what is not portable. CI does not run `make test` on Windows, so nothing else would have caught it.
  - The former user file: with `USERPROFILE` pointing at a scratch home that holds `.config\warden\config.toml`, `doctor`, `build` and `clean` each exit 1 with "`<home>\.config\warden\config.toml` is no longer read" and `To fix:`. `WARDEN_CONFIG` or `--config` naming that file clears it (`doctor` exits 0, first line `config: <path> (WARDEN_CONFIG)`); naming another file does not. PASS. Nothing was written under the real home.
  - `--config C:\...\named\warden.toml` beside a `.\warden.toml` that sets another `output_dir`: `doctor` names the named file, and a podman build SUCCEEDED into `from-named` with no `from-repo`. PASS.
  - `WARDEN_UPSTREAM_CA_CERTS=C:\...\a.pem;C:\...\b.pem`: one `upstream CA <path> (WARDEN_UPSTREAM_CA_CERTS): 1 certificate to send` line per file, on `doctor`'s stdout and on the build's stderr; the build SUCCEEDED, 23 signatures valid, all channels closed. A doubled `;;` is refused as an empty entry. PASS.
  - `--config NUL` and `WARDEN_CONFIG=NUL`, beside a `.\warden.toml` naming a driver warden lacks: `config: \\.\NUL (--config)`, `doctor` ok. PASS.
  - Baseline: the same container build with no settings SUCCEEDED on `main` and on the branch.
  - Evidence: `native/container-any/2026-11-15/project-config-trust/` (logs, the scratch Makefile and the case files). The cases ran from a scratch Makefile, target by target (`env certs userfile userfile-cleared named named-build ca-doctor ca-build nul base-build clean-builds`); each recipe is `cd <case dir> && [USERPROFILE=... | WARDEN_CONFIG=... | WARDEN_UPSTREAM_CA_CERTS=...] warden <doctor|build|clean|inspect> ...`, plus `openssl req -x509` for the two test certificates.
- **windows-hyperv, backlog `project-config-trust-hyperv`**: run; **passes, entry done.** Not elevated, privileged service reachable, relay boot VHDX stamp current.
  - A development warden run from its tree builds: `warden build --driver hyperv --guest-os windows` of a `build.ps1` SUCCEEDED on `main` and on the branch.
  - The detached prepare: the branch, run as `dist/warden.exe` with `--config` naming a file outside the working directory whose `[vm] image_refresh_after` was due, printed "refreshing it in the background" and built; `prepare.log`'s first line is `[warden] config: C:\...\.scratch\pct-hv\named\warden.toml (--config)`, and the prepare finished. `inspect --verify-disks`: both disks [ok], 11 signatures valid, all channels closed. `main` then built on the new base, both disks [ok].
  - Evidence: `native/windows-hyperv/2026-11-15/`. `main` has no `--config`, so its refresh-due build ran from a scratch Makefile (`main-refresh`: `cd named && ../../../dist/warden build --driver hyperv --guest-os windows -o ../out-main-refresh ../proj/build.ps1`; `state`: a PowerShell script listing warden processes and the Hyper-V cache directory).
- **container-any, backlog `prebuilt-artifacts-windows`**: not run. `feat/prebuilt-artifacts` is still design-only (49b513a) and held in Plan on q1.
- **linux-kvm**: nothing open (`firecracker-distro-firewalld` is deferred and was not retried).
- Check results: no review step ran, so no tier-1 or tier-2 result this run.

### Blocked on Jeff

- **A stale `dist/warden.exe` (2026-11-04) in this machine's checkout.** `make build` writes `dist/warden` here, and the old `warden.exe` beside it is what Windows starts for the background prepare (see the follow-up). This run left it as found. Delete it, or say the native session may; until then a refresh-due build from this checkout prepares the base with the 2026-11-04 code.
- **Allowlist gaps** (the scratch Makefiles above): a case that needs its own working directory, `USERPROFILE`, or a `WARDEN_*` variable together with a working directory cannot run in the `.scratch/<name>/dist/warden*` form. Reading the Hyper-V cache (`%LOCALAPPDATA%\warden\hyperv\prepare.log`) worked with the Read tool but `ls` and `cp` there were refused, so the two prepare logs in the evidence were written out from what was read.
- project-config-trust itself needs nothing from you now: the cloud loop takes `f3-implement-r3`.

### Follow-ups filed

- **detached-prepare-windows-exe (follows project-config-trust)**, priority 68, the top of the fixes. On Windows a warden whose file has no `.exe` ending, which is what `make build` writes, cannot start the background image prepare and says nothing: `driver.StartDetached` runs `exec.Command(os.Executable())`, os/exec resolves an extension-less path only by adding the `PATHEXT` endings, and the callers drop the error. No "refreshing" line, `prepare.log` left at 0 bytes, no process; the same on `main` 623131a and on the branch. With a `warden.exe` beside it, that file is started instead. A released `warden.exe` is not affected.

### Spent

- 0 queue steps and 3 backlog entries, against a native budget of 8.

### Notes

- FLOW-18 check: ok at the start and at the end of the run.
- Entire: enabled and the session is listed as active (`entire status`), with "Claude Code hooks out of date" reported; whether the checkpoint reaches buildwarden-sessions was not checked.
- The machine outside the checkout. The Hyper-V prepared base was refreshed twice, both by warden's own background prepare: first by `main`'s refresh-due build (run, as it turned out, by the stale `dist/warden.exe`), then by the branch's, which replaced it. The active base is `windows-server-2025-20261116T052354Z.vhdx` (sha256:0d6d31e3c136, grace ends 2026-11-26T05:24Z), prepared by the branch build, whose `driver/hyperv` code is `main`'s; `main` builds on it. The layers of 2026-10-30 and 2026-11-12 were retired by those prepares. `warden clean --driver hyperv`: nothing to clean. `podman-machine-default` was started for the container builds and stopped again; the builds' images (`warden-holder`, `warden-netns`, their layers, `alpine:latest`) were removed, and no container or network was left.
- `dist/warden.exe` was moved aside for one build, to show the silent failure on `main`, and put back.
- Scratch worktree and case directories removed; the checkout is on `main`, clean. Free disk at the end: 884G of 1.9T on C:.
