# Windows build-image preparation (qemu driver)

The qemu driver boots a **prepared** Windows guest image, the same way the vz
driver boots a prepared macOS image. BuildWarden ships no Windows media: you
supply a licensed image (retail/OEM/Volume, or a Microsoft evaluation image) and
prepare it once. This mirrors `tools/vz-image-prep` for macOS.

Pass it at build time:

```sh
warden build --driver qemu --guest-os windows \
  --image /path/to/windows.qcow2 --script build.ps1 ./context
```

## What the driver provides at run time

For each build the driver attaches a second CD-ROM volume labelled **`WARDEN`**
containing:

- `warden-io.exe` — the guest agent (cross-compiled `windows/<arch>`).
- `warden-run.ps1` — a first-boot bootstrap that copies `warden-io.exe` to
  `C:\warden\`, runs `warden-io initialize --gateway=10.0.0.1 --ip=10.0.0.2/30`
  (which sets the static network, installs the per-build CA, fetches `build.ps1`
  from the relay, runs it, and reports the exit code), then shuts the VM down.

The build script (`build.ps1`) is served by the relay at `http://cwd/build.ps1`;
`warden-io` fetches it — it is not placed on the seed.

## What the prepared image must contain

1. **virtio-win drivers** (net + block). The isolated relay<->build link and the
   boot disk are virtio devices; a stock Windows image has no in-box virtio
   drivers, so install the signed
   [virtio-win](https://github.com/virtio-win/virtio-win-pkg-scripts) package
   before capturing the image. (Alternatively the driver can be tuned to use
   emulated e1000e/AHCI devices; that is a 3b tuning decision.)
2. **A boot-time task that runs the seed bootstrap.** Create a task that, at
   startup, finds the `WARDEN`-labelled volume and runs its `warden-run.ps1`,
   e.g. a Scheduled Task (trigger: at startup, highest privileges):

   ```powershell
   $action = New-ScheduledTaskAction -Execute 'powershell.exe' `
     -Argument '-NoProfile -ExecutionPolicy Bypass -Command "$v = Get-Volume -FileSystemLabel WARDEN; & ($v.DriveLetter + ':\warden-run.ps1')"'
   $trigger = New-ScheduledTaskTrigger -AtStartup
   Register-ScheduledTask -TaskName 'warden-run' -Action $action -Trigger $trigger `
     -User 'SYSTEM' -RunLevel Highest
   ```

3. **Autologon / no interactive gate** so the VM reaches the startup task
   headlessly (no login prompt blocking the build). For Windows 11, the image
   must also have been installed in a way that does not require a TPM/Secure-Boot
   gate under qemu, or qemu must be given a vTPM (a 3b decision).

## Network

The guest is assigned a static address on the isolated link (no DHCP):

| Host | Address |
|------|---------|
| relay gateway | `10.0.0.1` |
| build guest   | `10.0.0.2/30` |

These match the Linux cloud-init `network-config` and are set by `warden-io
initialize` via the `--gateway` / `--ip` arguments baked into `warden-run.ps1`.

## Status

The provisioning and agent plumbing here is implemented and unit-tested. The
end-to-end boot (and any device tuning — virtio vs emulated, vTPM/Secure Boot)
is the **3b** milestone, validated locally on a Windows 11 ARM image under HVF,
with the native `win-64` build deferred to Phase 2 on an x86-64 host. See
`docs/design/windows-driver-plan.md`.
