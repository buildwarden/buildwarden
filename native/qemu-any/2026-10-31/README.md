# host:qemu-any/20261031T114004Z (unattended, macbook)

main cc6119e, QEMU 11.1.1 (Homebrew), relay initramfs built from main
(`make relay-vm`), macOS host on arm64.

## Linux: pass

`dist/warden build --driver qemu --image ~/Library/Caches/warden/images/alpine_3_21.qcow2 -q --timeout 20m`
of a build.sh running `apk add --no-cache jq`: build SUCCEEDED.

`dist/warden inspect --verify-disks`:

```
LEDGER  scheme=ed25519-sha512  hashes=[blake2b_256 sha256 sha1 md5]  ✅
ENVIRONMENT  vm-disk-chain/v1  payload verified
  qemu linux/arm64
  disk     alpine_3_21.qcow2  197.5 MB  sha256:ebaef97d702d  local  [ok]
  seed     5 per-build files
...
SUMMARY: 22 records, 7 requests, 2.8 MB audited
SIGNATURES: ✅ All 23 signatures valid
COMPLETENESS: ✅ All channels closed
```

Tamper check: a copy of the output with one seed size in environment.json
changed (82 -> 83) reports `ENVIRONMENT vm-disk-chain/v1 payload MISMATCH`,
"environment.json does not match the signed environment record", exit 1.

Descriptor: linux-main-cc6119e.environment.json.

## Windows: fail (timeout, guest silent)

`dist/warden build --driver qemu --guest-os windows --image ~/Library/Caches/warden/images/windows-arm64.qcow2`
of a build.ps1 (PowerShell version + one Invoke-WebRequest), twice:
`-q --timeout 40m` and, without `-q`, `--timeout 25m`. Both:

```
[warden] preparing build environment...
[warden] starting relay...
[warden] starting build...
[warden] running...
warden: build FAILED: timed out after 25m0s (--timeout)
```

No guest console output at all, and the ledger stayed at 1131 bytes (the
ENVIRONMENT record only): the guest never fetched /bootstrap. The descriptor
written at start (windows-main-cc6119e.environment.json) is well formed:
vm-disk-chain/v1, qemu windows/arm64, disk windows-arm64.qcow2 (12.8 GB,
sha256:bc917e5249d4) plus nvram windows-arm64-vars.fd, both source kind
`prepared` (prepared_by "warden dev", inputs install-iso sha256:638aa2c88e94,
virtio-win-iso sha256:303f7ae40dad, autounattend-iso sha256:8ae8f5774f41),
3 seed files. So the cached base is ISO-installed. Not established: when it
was prepared (the session cannot stat the cache dir). If it predates b9e3d34
(2026-10-26, "qemu: generalize the Windows install") and the shared Windows
guest package, it would not run main's bootstrap, which fits a silent guest. Re-preparing
it needs the install ISO (backlog qemu-windows-guest-package).
