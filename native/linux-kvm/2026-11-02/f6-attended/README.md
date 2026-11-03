# f6-validate-linux-kvm, attended (2026-11-02)

This is the attended native run `host:linux-kvm/20261103T005339Z`, with Jeff watching. It ran on windows-desktop's WSL2 Ubuntu 22.04 against feat/firecracker-driver at **824abd7**. It replaces the first attempt (`../f6-README.md`), which ran at 96ca21a and stopped in preflight on WSL's DNS resolver. Jeff's fix for that was a static /etc/resolv.conf with 8.8.8.8 and 8.8.4.4.

## Setup

- **Host:** kernel 6.18.54.1-microsoft-standard-WSL2-warden+ and Firecracker v1.17.0. The FORWARD policy was DROP (from warden-fc-posture.service), the uplink eth0 has an MTU of 1280, and br_netfilter was loaded.
- **Binaries:** warden, warden-io-linux-amd64 and warden-relay-linux-amd64 were cross-compiled on the Windows host from 824abd7 and staged in ~/fcval/bin.
- **Boot assets** (staged beside the binary, under their release names):
  - `firecracker-vmlinux`: the spike's Linux 6.1.191 build, sha256 169495b7…. Its kernel.config is identical to the branch's (418cf99c…).
  - `firecracker-relay-initramfs.cpio.gz`, sha256 108034f5…, rebuilt from 824abd7 with `ARCH=x86_64 NO_MODULES=1`.
  - `firecracker-build-initramfs.cpio.gz`, sha256 617e18e6…, rebuilt from 824abd7.
- **Image:** the cached prepared image ubuntu:24.04, `noble-server-cloudimg-amd64-04e8f2461a16.raw`.

## Results

All checks are on 824abd7.

| Check | Result |
|---|---|
| `sudo warden setup --driver firecracker --slots 4` | ok: 16 interfaces, bridge table and WARDEN-FC chains, ip_forward=1 |
| `warden doctor` (as jeff, and with sudo) | all ok right after setup. The one warning: no reflinks on ext4. See the finding below. |
| Direct-boot build | SUCCEEDED in 5.3 s. Guest kernel 6.1.191, HTTP fetch through the relay, direct TCP to 1.1.1.1:22 blocked, 11 signatures valid |
| Ubuntu 24.04 disk-path build with COPY and HTTPS | SUCCEEDED in 46 s. COPY arrived, https://pypi.org/simple/six/ returned 16337 bytes, 13 signatures valid |
| Exclusive trust | the trust store holds 1 certificate, and the console shows `applying baseline (trust exclusive)` |
| Egress to an address not proxied | direct TCP to 1.1.1.1:22 and to 8.8.8.8:853 both blocked |
| Two concurrent builds with br_netfilter loaded | both SUCCEEDED, on slots 0 and 1 |
| Build VM killed mid-build | warden exited 1 10.1 s after the kill (the completion grace) with "the build VM stopped before the build completed" and the console tail. No VMMs left. |
| warden `kill -9` mid-build | Pdeathsig took both VMMs down (2 before, 0 after). The next build took slot 0 and SUCCEEDED in 5.2 s. |
| Another local user reaching a running relay | as `nobody`, :8300 and :8301 were both blocked. As jeff, :8300 was blocked too. |
| Teardown | the work dir is empty, no firecracker processes remain, doctor shows no orphans, slots free |
| Ledgers | all seven outputs: every signature valid, every channel closed (`warden inspect`) |

## Distro cases

Jeff decided on 2026-11-02 to record the step as passing with these gaps documented:

- **FORWARD policy ACCEPT, where setup should refuse:** passed at 96ca21a (`../f6-forward-accept.log`), and posture.go has not changed since. Auto mode blocked a rerun because it briefly sets the policy to ACCEPT.
- **Local resolver on a LAN address:** not run, because auto mode blocks swapping resolv.conf. Backlog `firecracker-distro-lan-resolver`.
- **firewalld:** not installed on this host. Backlog `firecracker-distro-firewalld`.

## Findings (not blocking)

1. **WSL distro restarts loosen the slot rp_filter.**
   - **What happened:** WSL shuts an idle distro down and boots it again on the next `wsl.exe`. The utility VM and its network namespace keep running, so the slots, the nft table and the iptables chains survive. But each distro boot runs systemd-sysctl, and Ubuntu's `/usr/lib/sysctl.d/50-default.conf` sets `net.ipv4.conf.*.rp_filter = 2`. That changed the bridge and upstream TAP from strict (1) to loose (2). doctor caught it ("8 sysctl(s) not as setup sets them").
   - **Impact:** builds still ran, because preflight does not check sysctls. Isolation held, because the bridge-table and WARDEN-FC anti-spoof rules were intact.
   - **Scope:** this is specific to WSL. On a real Linux host a reboot also removes the interfaces.
   - **Possible fix, for Jeff:** a preflight check of the slot's sysctls, with the fix "re-run setup", or a note in the drivers guide's WSL section.
2. **The CLI resolves `--script` against the current directory, not the build context.** This is shared behaviour on main (`driver.StageBuildScript` copies `req.Script` as given). The cli.md example `warden build --driver vz --script build.sh ./my-project` reads as if the path were relative to the project.
3. **An expected build failure prints a bug-report link.** When the build VM dies, the error is not marked fixable, so the CLI adds "If this is unexpected, please file a bug". This is minor.

## Cleanup

`slots.sh down` removed the slots, ~/fcval was removed, and the .scratch worktree was removed. The cached raw image stays in ~/.cache/warden/firecracker/images.
