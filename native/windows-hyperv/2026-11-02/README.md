# windows-hyperv 2026-11-02: Hyper-V build after firecracker-driver's cmd/warden wiring

host:windows-hyperv/20261102T231002Z, unattended, backlog entry
`firecracker-hyperv-cli-wiring`. Windows 11 Pro 26300 host, Hyper-V
Administrators (not elevated), prepared base
windows-server-2025-20261030T175627Z (eval grace ends 2026-11-09), cached
relay boot VHDX built from relay source 3515f310b2f5 (current for main
e418871; `tools/relay-vm/hyperv/refresh-cache.sh` stamp).

`build.ps1` is the guest script: HTTPS through the relay with
Invoke-WebRequest and curl.exe (no `--ssl-no-revoke`), a direct TCP connect to
1.1.1.1:8080, the relay CA count in LocalMachine\Root, and a file write.

| Binary | Result | Logs |
|---|---|---|
| main e418871 (`go build ./cmd/warden`) | SUCCEEDED; both HTTPS fetches 200 (577 and 16337 bytes), direct egress not connected, 1 warden root; `inspect --verify-disks`: ENVIRONMENT vm-disk-chain/v1 verified, both disks [ok], 20 signatures valid, all channels closed | `main-e418871-*.log` |
| feat/firecracker-driver 96ca21a (`go build -C .scratch/firecracker -o ../../dist/warden ./cmd/warden`) | SUCCEEDED, same guest output; `inspect --verify-disks`: both disks [ok], 20 signatures valid, all channels closed | `branch-96ca21a-*.log` |

The branch binary ran from the main checkout's `dist/` (settings.native.json's
`.scratch/:*` allow does not match `.scratch/<name>/dist/warden`; see
native-allowlist-scratch), so `driver.ModuleRoot()` resolved to main's tree
for the on-demand `warden-io-windows-amd64.exe` cross-build. The branch does
not change cmd/warden-io, cmd/relay, driver/hyperv or the shared relay,
ledger and collector packages (`git diff --stat origin/main...` is empty for
them), so the guest binaries are the same either way; what the branch run
exercises is the cmd/warden wiring and the driver/ root changes (hostlock.go)
on the Hyper-V path, which is this entry's question.
