# hyperv-relay-per-build-channel, f6-validate-windows-hyperv, attended r5 (2026-11-12)

Branch at 61bd447 (r5 code); the head 2080a28 adds only docs (overview.md, the design) on top of it.
The privileged service was reinstalled from the 61bd447 build (`%LOCALAPPDATA%\warden\validate\r6\warden.exe`), elevated, by Jeff.

| Check | Result | logs/ |
|---|---|---|
| F6: `warden setup --driver hyperv --nat-prefix 192.168.250.0/24 --host-ip 192.168.250.1 --slots 2`, twice in a row, with the NAT and both slots already present | PASS: both print "Durable dev network ready", slots `present` | `f6-setup1.txt`, `f6-setup2.txt` |
| Smoke: two simultaneous ephemeral builds through the r5 service (its EnsureNetwork also ends on DONE now) | PASS, 2/2, ledger verifies, no VMs or per-build switches left | `r5-a.log`, `r5-b.log` |

Everything else for this step stands from the earlier attended runs today. The r5 delta (5ac5532..61bd447) touches only the PowerShell script endings and runPS's error text:
- `../attended/`: cases 0-3 and 5-9 at 59d7440. Cases 1, 3, 8 and 9 cover code unchanged since then.
- `../attended-r4/`: F2, F5, cases 2, 4, 5, 6 and 7, and hyperv-clean at 5ac5532.
