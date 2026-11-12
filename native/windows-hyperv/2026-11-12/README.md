# windows-hyperv 2026-11-12: hyperv-relay-per-build-channel f6-validate-windows-hyperv

`host:windows-hyperv/20261112T111001Z`, unattended. Windows 11 Pro 26300 host,
Hyper-V Administrators, **not elevated**. Branch
`feat/hyperv-relay-per-build-channel` at **59d7440** (consolidation r3), built
with `make -C .scratch/hrpbc build`. Host state is unchanged since 2026-11-11:
main's privileged service is installed at protocol 1, the old shared
`warden-dev` switch and `warden-dev-nat` (192.168.240.0/20) are there, the
relay boot VHDX is at stamp 1, and the prepared base's evaluation grace ended
2026-11-09T17:57Z.

## Results

| Case | Result | Log |
|---|---|---|
| (0) doctor, stale service | Passes the non-elevated half. Doctor prints `Privileged service ... stale: it speaks protocol 1, this warden 2` with "from an elevated shell, run `warden hyperv service uninstall`, then `warden hyperv service install`", `Network .... BLOCKED` and a `Not ready` verdict naming the same fix. It never says "not installed". | `branch-59d7440-doctor.log` |
| (0) non-elevated build | Refused with `hyperv build: the privileged service speaks protocol 1, this warden 2`, naming uninstall then install. The refusal comes out before any relay or VM output. `Get-VM` is outside the session allowlist, so no VM listing was taken. | `branch-59d7440-build-refused.log` |
| (0) after an elevated reinstall | Not run: needs elevation. | |
| (1) doctor, stamp-1 VHDX | The `BLOCKED: hyperv relay boot VHDX: link-channel stamp stale ... tools/relay-vm/hyperv/refresh-cache.sh` line is still shown (in the doctor log). The build-refusal half was not reached: the stale-service refusal comes first, and the service reinstall needs elevation. | `branch-59d7440-doctor.log` |
| (2)-(9) | Not run, for the reasons the 2026-11-11 README gives: the base image re-prepare, the elevated reinstall and setup, and the host DNS changes for cases 7 and 8. | |

The S7 finding (the Windows-only `probeService` mapping is untested) is
covered natively by case 0 as above. The relay boot VHDX is still at stamp 1,
so case 1 can be repeated.
