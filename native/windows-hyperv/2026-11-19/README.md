# detached-prepare-windows-exe f6-validate-windows-hyperv: pass

host:windows-hyperv/windows-desktop.20261119T171002Z, unattended. Windows 11 Pro 26300, Hyper-V
Administrators (not elevated), privileged service reachable, relay boot VHDX link-channel stamp
current, go1.26.3 windows/amd64. Branch feat/detached-prepare-windows-exe at b4f86e2 (merged with
main e9315c2), in worktree .scratch/dpwe, built with `make -C .scratch/dpwe build` (dist/warden, no
extension). Cases driven by the scratch Makefile here (`make -C .scratch/dpwe-nv <target>`):
`test` runs go test in the worktree, `refresh` runs
`<W> build --config cfg/warden.toml --driver hyperv --guest-os windows -o out-<TAG> proj/build.ps1`
with `[vm] image_refresh_after = "1m"`, `state` runs state.ps1 (warden processes with
ExecutablePath and CommandLine via Win32_Process, prepare.log), `hold-log` runs hold.ps1,
`mkexe`/`rmexe` add and delete dist/warden.exe.

1. `go test ./driver/ ./driver/hyperv/ ./driver/qemu/`: ok. TestStartDetachedRunsNamedFile,
   TestStartDetachedFailedStart, TestDetachedArgs, TestEnsureFreshBaseReportsFailedStart,
   TestEnsureFreshBaseStartsRefresh, TestEnsureFreshWindowsLayerReports (3 subtests) all PASS on
   Windows. (case1-go-test.log)
2. No warden.exe: "prepared base is 3.5d old; refreshing it in the background", build SUCCEEDED
   (example.com 200, 577 bytes). The background process is
   `...\.scratch\dpwe\dist\warden --config ...\cfg\warden.toml image prepare --driver hyperv`;
   prepare.log's first line is the config line; the prepare finished
   (windows-server-2025-20261119T171325Z.vhdx, sha256:007b40a02ce9) and doctor shows it active. A
   build during the prepare printed no refreshing line and started no second process.
   (case2-*.log)
3. warden.exe beside dist/warden: the refresh started from `dist\warden`, not warden.exe; prepare
   finished (20261119T171732Z, sha256:468501402ce0); warden.exe deleted after. (case3-*.log)
4. prepare.log held with FileShare None: "prepared base is 4m old, and its background refresh did
   not start: open ...\prepare.log: The process cannot access the file because it is being used by
   another process.; run `warden image prepare --driver hyperv` to refresh it", build SUCCEEDED, no
   prepare process. Cosmetic only: the Windows error text ends in a period, so the line reads
   "process.; run". (case4-*.log)
5. `go build -o dist/warden.exe ./cmd/warden`, run as dist/warden.exe: refreshing line printed,
   build SUCCEEDED, the prepare ran from `dist\warden.exe` and finished (20261119T172521Z,
   sha256:b05780449fcf); warden.exe deleted after. (case5-*.log)

No warden-* VMs left; both slots free at the end.
