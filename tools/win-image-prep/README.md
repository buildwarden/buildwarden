# Windows build-image preparation (qemu driver)

Goal: give Windows guests the **same first-class, automated image lifecycle**
the vz driver gives macOS (`warden image restore` / `prepare` / `list`, with
per-build COW clones). BuildWarden ships no Windows media; you either point it at
licensed media you supply or let it auto-download a Microsoft evaluation ISO.

## Settled design decisions

| Decision | Choice |
|----------|--------|
| Acquisition | **Both**: operator-supplied licensed media *and* auto-download of a Microsoft evaluation ISO. |
| Prep model | **Full unattended install from ISO** (reproducible, zero manual steps) via a generated `Autounattend.xml`. |
| Firmware (Win11 TPM/Secure Boot) | **LabConfig bypass** (no swtpm dependency). See "Future" below. |
| Toolchain | **Minimal image**; the build toolchain (Miniforge / VS Build Tools) is bootstrapped through the relay at build time, so its download is witnessed in the ledger. Not baked into the image. |

## How prep works (Mac-parity)

`warden image restore` for Windows will:

1. **Acquire** the base media — a supplied `--iso <path>`, or an http(s) URL
   that is downloaded + cached + sha256-verified (like the qemu cloud-image
   cache). Microsoft publishes an official **Windows 11 Arm64 ISO**
   ([microsoft.com/software-download/windows11arm64](https://www.microsoft.com/en-us/software-download/windows11arm64),
   explicitly permitted for creating VMs) and an x64 Enterprise **evaluation**
   ISO. Windows installs **without a product key** and runs unactivated, which
   is enough for development/testing; a valid license is needed to produce
   distributable artifacts.
2. **Generate `Autounattend.xml`** (`autounattend.go`) that drives a fully
   unattended install: LabConfig bypass keys (TPM/Secure Boot/CPU/RAM/storage),
   a `specialize`-pass **`BypassNRO`** key + an `oobeSystem`
   **`International-Core`** component so OOBE runs with **no interactive
   screens** (region/keyboard auto-answered, the network page auto-skips), an
   ephemeral autologon local admin, and a first-logon command that registers a
   **startup Scheduled Task** running the WARDEN seed's `warden-run.ps1` on
   every boot. Only the **NetKVM** virtio-net driver is injected from the
   virtio-win ISO (the OS disk is NVMe — see below — so no `viostor`/`vioscsi`
   is needed). The answer file's dynamic values are XML-escaped: the
   FirstLogonCommands PowerShell contains a raw `&`, and an unescaped `&`
   makes Windows Setup reject the whole answer file at PreFinalize.
3. **Run the install headlessly under qemu**, driven over a **QMP control
   socket** the driver owns: it clears the firmware "Press any key to boot from
   CD" prompt (Down-arrow, so a stray press can't hit a Setup button), and
   **ejects the install ISO on the first guest reboot** so subsequent reboots
   boot the installed disk. When Setup finishes, the guest autologs on, runs the
   first-logon task + `shutdown /s`, qemu exits, and the driver captures the
   resulting qcow2 as the cached base image.
4. Each `warden build` gets an instant **COW overlay** off that base (existing
   qemu behavior).

### Validated qemu device model (win-arm64)

The device model matters — these choices are load-bearing and were validated
against the real Win11 25H2 ARM64 installer:

- **OS disk on emulated NVMe** (`-device nvme,...,bootindex=0`), **not**
  virtio-blk. `stornvme.sys` is in-box on both win-64 and win-arm64, so Setup
  detects the disk with zero driver injection. virtio-blk needs `viostor`
  injected via `DriverPaths`, whose driver-ISO **drive letter is
  nondeterministic** run-to-run, causing intermittent "no disk found" at the
  Setup disk-selection screen. NVMe also gives a `bootindex` handle.
- **`bootindex`**: OS disk `bootindex=0`, install/virtio/answer media on
  `usb-storage` with `bootindex=1..3`. edk2/OVMF **ignores `-boot order`**;
  per-device `bootindex` is the working lever. On first boot the empty NVMe
  disk is probed first (then the CD boots Setup); after install it boots the
  Windows Boot Manager directly instead of dropping to the UEFI shell.
- **Writable edk2 vars pflash** (a per-image copy of `edk2-arm-vars.fd`) is
  reused across the install's reboots so the boot entry `bcdboot` writes
  survives. A fresh/read-only vars store is a common cause of the reboot→shell
  drop.
- ramfb display, USB keyboard/tablet, `virtio-net-pci` NIC (user-net),
  `virtio-rng`.

### Timing advisory

A full hands-off install on an Apple-Silicon Mac (qemu `-accel hvf`, 8 vCPU /
8 GiB) takes **~85–90 minutes** end-to-end (measured 87m29s), the bulk of it
Setup applying the image. Budget accordingly; the driver's install timeout
defaults to 4h and is overridable via `WARDEN_QEMU_INSTALL_TIMEOUT_MIN`.
Debug knobs: `WARDEN_QEMU_VNC=127.0.0.1:0,password=on` +
`WARDEN_QEMU_VNC_PASSWORD=<pw>` to watch, `WARDEN_QEMU_MONITOR=<sock>` for an
HMP socket (e.g. `screendump`).

`warden image prepare` re-applies the boot task / agent to an existing image
after a warden upgrade (the vz `ReprepareImage` analog).

## Runtime seed (per build)

Unchanged from chunk 3a: the driver attaches a CD-ROM labelled **`WARDEN`** with
`warden-io.exe` + `warden-run.ps1`. The prepared image's startup task runs
`warden-run.ps1`, which runs `warden-io initialize --gateway=10.0.0.1
--ip=10.0.0.2/30` — static network (no DHCP on the isolated link), install the
per-build CA, fetch `build.ps1` from the relay, run it, report the exit code.

| Host | Address |
|------|---------|
| relay gateway | `10.0.0.1` |
| build guest   | `10.0.0.2/30` |

## Future / opt-in (prioritize on explicit request)

**swtpm + Secure Boot.** Instead of the LabConfig bypass, run the `swtpm`
software TPM and boot the Secure-Boot OVMF variant so Win11 installs in a
"supported" configuration. Deliberately deferred: the build guest is untrusted
by design (topological isolation, witness-the-wire), so an emulated TPM adds
nothing to the ledger's integrity and does not affect compiler output. Firmware
is built as a pluggable policy, so this can be added as a `--secure-boot` prep
option **without reworking the driver** — prioritize it if a concrete
requirement appears (e.g. a customer needing a "supported" Win11 config, or a
future measured-boot BuildWarden feature). It also becomes the fallback if Win11
**ARM64** Setup turns out to reject the LabConfig bypass (to be confirmed against
the real installer in 3b).

## Status

**win-arm64: validated, fully hands-off.** `warden image restore --os windows
--arch arm64 --iso <win11-arm64.iso>` runs an unattended install end-to-end and
captures a bootable Windows 11 ARM64 base image with zero manual steps
(verified: the captured image boots straight to an autologon desktop). The
`Autounattend.xml` generator, WARDEN seed + `warden-run.ps1` bootstrap,
`warden-io.exe` build, guest agent, media acquisition/download, the `warden
image` command surface, and the QMP-driven install orchestration are all
implemented and exercised on the real installer.

Next: exercise a real `warden build --guest-os windows` against the base image
(L1 `warden-io` network/CA/script path) and the isolation validation. Native
**win-64** is Phase 2 on x86-64 with a Hyper-V driver (qemu can't accelerate
x86-64 on Apple Silicon — TCG emulation is too slow for a pipeline); the
research and plan both point there. See `docs/design/windows-driver-plan.md`.
