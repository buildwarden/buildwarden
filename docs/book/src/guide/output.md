# Output Directory

Every `warden build` produces a self-contained output directory with everything needed to verify the build.

## Structure

```
warden-output/
├── ledger.zst              # Cryptographic ledger (compressed)
├── Dockerfile.submitted    # The original Dockerfile you provided
├── Dockerfile.actual       # What BuildWarden actually ran
├── ca.cert.pem.zst         # Ephemeral CA cert (compressed)
├── relay.log               # Relay application log
└── artifacts/              # Build outputs posted to the ledger
    └── myapp-1.0.0.whl
```

## File Details

### ledger.zst

The binary ledger containing chained Ed25519 signatures over every network request and source file transfer. Compressed with zstd by default (use `--no-compress` for raw).

Inspect with:
```sh
warden inspect warden-output
```

### Dockerfile.submitted

Your original Dockerfile, unchanged. Preserved for comparison with what actually ran.

### Dockerfile.actual

The rewritten Dockerfile that BuildWarden executed. Differences from submitted:
- `COPY .warden /.warden` and extension setup injected after `FROM`
- `COPY` directives rewritten to `RUN curl` commands for provenance
- Environment variables from extensions added

### ca.cert.pem

The ephemeral CA certificate generated for this build's TLS interception. The private key existed only in the relay container's memory and was destroyed on teardown.

### relay.log

Application-level log from the relay process. Contains:
- DNS resolution events
- Artifact storage confirmations
- Error messages (if any)

### artifacts/

Named build outputs. Each artifact was:
1. POSTed to `http://artifacts/<name>` from within the Dockerfile
2. Hashed (blake2b_256, sha256, sha1, md5)
3. Signed into the ledger
4. Saved to disk

## Compression

By default, the ledger and CA cert are compressed with zstd. Already-compressed files (zip, gzip, zstd, bzip2) are detected by magic bytes and stored as-is.

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
