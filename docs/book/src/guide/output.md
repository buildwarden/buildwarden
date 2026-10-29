# Output Directory

Every `warden build` produces a self-contained output directory with everything needed to verify the build. The layout is identical for every driver.

## Structure

```
warden-output/
├── ledger.zst              # Cryptographic ledger (compressed by default)
├── ca.cert.pem.zst         # Ephemeral CA cert (compressed by default)
├── environment.json        # Signed environment descriptor (what the build ran on)
├── build-output.log        # Captured build console output
├── artifacts/              # Build outputs posted to the ledger
│   └── myapp-1.0.0.whl
├── captures/               # Captured payloads (only with --capture)
│   └── <name> -> ../payloads/<hash>
└── payloads/               # Content-addressed payload storage
    └── <hash>
```

A build from a Dockerfile also gets `inputs/`, holding the Dockerfile you
submitted, kept as `Containerfile`, and the `build.sh` it was translated to
(ORCH-02):

```
warden-output/
└── inputs/
    ├── Containerfile       # Your Dockerfile, unchanged
    └── build.sh            # The translated build script: what ran
```

The container driver always builds from a Dockerfile, so its output always has
`inputs/`; the qemu and hyperv drivers write it for a Linux guest built from a
Dockerfile. A `--script` or a `build.sh` / `build.ps1` from the context runs as
given, so it is not copied. The container driver also writes `console.log`. The container and vz drivers,
whose relay runs as a container or host process, also write `relay.log`.

## File Details

### ledger.zst

The binary ledger containing chained Ed25519 signatures over every network request and source file transfer. Compressed with zstd by default (use `--no-compress` for raw).

Inspect with:
```sh
warden inspect warden-output
```

### inputs/Containerfile

Your original Dockerfile, unchanged. It is kept under the fixed name
`Containerfile` whatever it was called (`Dockerfile`, `Containerfile`,
`Dockerfile.simple`, ...). Preserved for comparison with what actually ran.

### inputs/build.sh

The build script translated from your Containerfile: what the build environment
actually ran. Each `COPY` became one `warden-io fetch` per file, resolved
against the build context on the host. The relay also recorded these exact
bytes in the ledger when the guest fetched them.

### console.log

The build container's own console: `warden-io`'s setup steps followed by the
build's output as the container printed it. It is the place to look when a
build fails before the relay is reachable. Written by the container driver only.

### environment.json

The payload of the ledger's environment record: a descriptor of what the build
ran on, whose hash the relay signed as the ledger's first record. For the
container driver it is the base image's manifest (`oci-image/v1`); for the VM
drivers it is a `vm-disk-chain/v1` descriptor listing the boot disks (hash,
size, path, and provenance) and the per-build seed files. `warden inspect`
checks it against the ledger, and `warden inspect --verify-disks` re-hashes the
disks it names. Absent when the build recorded no environment.

### ca.cert.pem

The ephemeral CA certificate generated for this build's TLS interception. The
private key existed only in the relay's memory and was destroyed on teardown;
the orchestrator receives only the public certificate when it claims the relay,
and writes it here.

### build-output.log

The build's console output, streamed from the guest to the collector as the
build runs. Written for every driver (suppressed from the terminal, but not
from this file, by `-q/--quiet`).

### relay.log

Application-level log from the relay process. Written by the container and vz
drivers (on vz it is echoed to the terminal only with `-v`). Contains:
- DNS resolution events
- Artifact storage confirmations
- Error messages (if any)

### artifacts/

Named build outputs. Each artifact was:
1. POSTed to `http://artifacts/<name>` from within the build
2. Hashed (blake2b_256, sha256, sha1, md5)
3. Signed into the ledger
4. Streamed to the collector and saved to disk

## Compression

By default, the ledger and CA certificate are compressed with zstd. This
applies to every driver (previously the container driver only).

Disable with:
```sh
warden build --no-compress .
```

Or in config:
```toml
[build]
compress = false
```

## Custom Location

```sh
warden build -o /path/to/my-output .
```

Or in config:
```toml
[build]
output_dir = "/path/to/my-output"
```
