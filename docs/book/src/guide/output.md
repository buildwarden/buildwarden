# Output Directory

Every `warden build` produces a self-contained output directory with everything needed to verify the build. The layout is identical for every driver.

## Structure

```
warden-output/
├── ledger.zst              # Cryptographic ledger (compressed by default)
├── ca.cert.pem.zst         # Ephemeral CA cert (compressed by default)
├── build-output.log        # Captured build console output
├── artifacts/              # Build outputs posted to the ledger
│   └── myapp-1.0.0.whl
├── captures/               # Captured payloads (only with --capture)
│   └── <name> -> ../payloads/<hash>
└── payloads/               # Content-addressed payload storage
    └── <hash>
```

The container driver also writes `Dockerfile.submitted`, `Dockerfile.actual`,
`build.sh` (the translated build script), and `relay.log`.

## File Details

### ledger.zst

The binary ledger containing chained Ed25519 signatures over every network request and source file transfer. Compressed with zstd by default (use `--no-compress` for raw).

Inspect with:
```sh
warden inspect warden-output
```

### Dockerfile.submitted

Your original Dockerfile, unchanged. Preserved for comparison with what actually ran. Written by the container driver only.

### Dockerfile.actual

The rewritten Dockerfile that BuildWarden executed. Written by the container
driver only. Differences from submitted:
- `COPY .warden /.warden` and extension setup injected after `FROM`
- `COPY` directives rewritten to `RUN curl` commands for provenance
- Environment variables from extensions added

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

Application-level log from the relay process. Written by the container driver
only. Contains:
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
