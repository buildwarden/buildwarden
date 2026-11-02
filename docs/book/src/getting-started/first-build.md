# Your First Build

## Run a warden build

The basic flow is the same regardless of driver:

```sh
warden build .                           # Container driver (default)
warden build --driver qemu .             # QEMU driver
warden build --driver vz --script ./build.sh .  # VZ driver
warden build --driver hyperv .           # Hyper-V driver
warden build --driver firecracker .      # Firecracker driver
```

The VM drivers need a one-time setup first; see
[Installation](./installation.md) and the [drivers guide](../guide/drivers.md).

For the container driver you can also point at a specific Dockerfile:

```sh
warden build ./my-project/Dockerfile.prod
```

## What you'll see

The output is consistent across all drivers:

```
[warden] Build ID: abc12345
[warden] ... (driver-specific startup messages) ...
warden-io: configuring network
warden-io: waiting for relay
warden-io: installing CA
warden-io: fetching build script
warden-io: running build.sh
... (build output) ...
[warden] Build complete
[warden] Output: warden-output
```

## Output directory

After the build completes (same structure for all drivers):

```
warden-output/
├── ledger.zst              # Cryptographic ledger (ledger if --no-compress)
├── ca.cert.pem.zst         # Ephemeral CA cert (ca.cert.pem if --no-compress)
├── build-output.log        # Captured build console output
├── artifacts/              # Build outputs posted via warden-io post
│   └── myapp.whl
├── captures/               # Captured payloads (only with --capture)
├── payloads/               # Content-addressed payload storage
├── relay.log               # Relay event log (container and vz drivers)
├── inputs/                 # Only for a build from a Dockerfile (ORCH-02)
│   ├── Containerfile       #   the Dockerfile you submitted, unchanged
│   └── build.sh            #   the script it was translated to: what ran
└── console.log             # Build container console (container driver only)
```

## Posting artifacts

To record a build output in the ledger, use `warden-io post` from within your build script or Dockerfile RUN step:

```sh
warden-io post /path/to/output.whl myapp-1.0.0.whl
```

The artifact is hashed, signed into the ledger, and saved in the output directory. This works identically in all drivers.

## Fetching context files

To pull files from the host into your build environment:

```sh
# Fetch a single file:
warden-io fetch config.json -o /tmp/config.json

# Fetch entire context:
warden-io fetch . -o /workspace
```

This works in all drivers — the relay serves context files over HTTP regardless of whether the build environment is a container or VM.

## A first Firecracker build

On an x86_64 Linux host with KVM, after the
[Firecracker setup](./installation.md#first-time-firecracker-setup):

```sh
warden doctor --driver firecracker           # every check should pass
warden image prepare --driver firecracker    # optional: caches ubuntu:24.04
warden build --driver firecracker .          # a Dockerfile FROM ubuntu:24.04
```

A Dockerfile whose `FROM` is `ubuntu:24.04`, `debian:bookworm` or
`alpine:3.21` (or a build given `--image <path>`) boots that disk image, and
the first build converts it once if you skipped `image prepare`. A
`--script` build with no image boots straight from a small initramfs instead,
which is faster but has only what your script brings:

```sh
warden build --driver firecracker --script build.sh ./src
```

If a build stops before booting with a DNS error, the host has only local
resolvers (systemd-resolved's stub with no upstream, a local dnsmasq, or
WSL's `10.255.255.254`); the error says what to change, and
[Upstream DNS](../guide/drivers.md#upstream-dns) covers it.

To look at the VMs after a build, add `--keep-vms`: both VMs keep running and
warden prints their pids. Stop them, and any left by an interrupted build,
with:

```sh
warden clean --driver firecracker
```

## Interactive debugging (container driver)

```sh
warden shell .
# Drops into isolated container — all traffic still audited
```

## Custom output location

```sh
warden build -o ./my-custom-output .
```

## Disabling compression

```sh
warden build --no-compress .
```
