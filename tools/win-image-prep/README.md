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
   unattended install: LabConfig bypass keys, virtio-win driver load in
   WinPE (so Setup sees the virtio boot disk/NIC and installs them boot-start),
   headless OOBE, an ephemeral autologon local admin, and a first-logon command
   that registers a **startup Scheduled Task** running the WARDEN seed's
   `warden-run.ps1` on every boot.
3. **Run the install headlessly under qemu** with the install ISO, the
   `Autounattend` floppy/ISO, and the signed
   [virtio-win](https://github.com/virtio-win/virtio-win-pkg-scripts) driver ISO
   attached, then capture the resulting qcow2 as the cached base image.
4. Each `warden build` gets an instant **COW overlay** off that base (existing
   qemu behavior).

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

Implemented and unit-tested: the `Autounattend.xml` generator, the WARDEN seed +
`warden-run.ps1` bootstrap, the `warden-io.exe` build, and the guest agent.
Pending: acquisition/download plumbing, the `warden image` Windows command
surface, and the live unattended-install orchestration + device tuning — the
**3b** milestone, validated on a real `win-arm64` install. Native `win-64`
is Phase 2 on x86-64. See `docs/design/windows-driver-plan.md`.
