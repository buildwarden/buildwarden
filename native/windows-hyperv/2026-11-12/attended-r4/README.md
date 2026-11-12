# hyperv-relay-per-build-channel, f6-validate-windows-hyperv, attended r4 (2026-11-12)

Branch at 5ac5532 (after f6-consolidate-r4). Built in a separate worktree; the privileged service was
reinstalled from this build (`%LOCALAPPDATA%\warden\validate\r5\warden.exe`) by Jeff, elevated.
Unchanged since 59d7440 (`git diff 59d7440 5ac5532 -- relay tools/relay-vm cmd/warden-io driver/winguest`
is empty), so cases 1, 3, 8 and 9 stand from `../attended/`.

| Check | Result | logs/ |
|---|---|---|
| F2 stopped service | PASS: `Privileged service ... installed, but stopped`, fix `Start-Service WardenHyperV` | `f2-doctor-stopped.txt` |
| 5 durable build | PASS (service stopped): slot warden-dev-0, ledger verifies | `c5-build.log` |
| 6 durable concurrency | PASS: A slot 0 / B slot 1; each build VM's `.253` neighbour is its own relay's MAC (A ...B1-52, B ...B1-54), one DHCP reply each; third build refused before any VM naming `warden clean --driver hyperv` | `c6*` |
| hyperv-clean (backlog `hyperv-clean-real-host`) | PASS: during a running build "Nothing to clean", its VMs untouched; after a durable `--keep-vms` build, VMs and work dir removed and the slot reads free; after an ephemeral `--keep-vms` build, VMs, work dir and the `warden-<id>` switch removed | `clean-*` |
| F5 setup re-creates the NAT | PASS: after `Remove-NetNat` and setup, `vEthernet (warden-dev-nat)` holds only 192.168.250.1/24 (192.168.240.1/20 gone) | `f5-setup.txt` |
| 2 elevated build | PASS, ledger verifies | `c2e-build.txt` |
| 4 concurrency | PASS: 3 simultaneous pairs via the service, 6/6 (pre-check at 29a57ea: 8/8) | `c4r*` |
| 7 refusals | PASS: build_link in 192.168.250.0/24 refused naming `warden-dev-nat`, before any work dir; `[vm] dns = [192.168.250.1]` refused | `c7-*` |
| **setup re-run** | **FAIL (F6)**: every `warden setup --driver hyperv` after the old shared switch is retired fails `build switches: powershell: exit status 1` | `f5-setup.txt`, `setup-rerun.txt` |

## F6

`setupSlotsScript` (driver/hyperv/setupslots.go) ends with
`$old = Get-VMSwitch -Name $net -ErrorAction SilentlyContinue` and an `if` that is false once `warden-dev`
is retired. The suppressed lookup leaves `$?` false, and `powershell -Command` exits 1 with nothing on
stderr. Reproduced non-elevated through `exec.Command` like runPS: stdout has both `SLOT=... present`
lines, stderr is empty, exit 1. So the first setup on a host works and every later one fails.
Fix: end the script with an explicit success marker (and/or `exit 0`), and have runPS fall back to
stdout's tail when stderr is empty (the error here said nothing). Tier-0: the rendered script ends with
the marker; the runPS message. (`slots.ps1` here is the script as rendered.)
