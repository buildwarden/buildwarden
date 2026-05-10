# Inspecting Results

## Basic inspection

After a build, inspect the ledger:

```sh
warden inspect warden-output
```

You can also point directly at a ledger file:

```sh
warden inspect warden-output/ledger.zst
```

Compressed (`.zst`) and uncompressed ledgers are handled transparently.

## Reading the output

```
════════════════════════════════════════════════════════════════════════════════
  LEDGER  scheme=ed25519-sha512  hashes=[blake2b_256 sha256 sha1 md5]  ✅
════════════════════════════════════════════════════════════════════════════════
✅ HEAD https://registry-1.docker.io/v2/library/python/manifests/3.12-slim (0 bytes)
✅ GET https://registry-1.docker.io/v2/library/python/blobs/sha256:... (64167785 bytes)
✅ GET http://deb.debian.org/debian/dists/bookworm/main/... (8690780 bytes)
✅ GET http://cwd/requirements.txt (42 bytes)
✅ GET https://files.pythonhosted.org/.../requests-2.32.3.tar.gz (131218 bytes)
✅ ARTIFACT POST http://artifacts/requests-2.32.3-py3-none-any.whl (65027 bytes)

════════════════════════════════════════════════════════════════════════════════
  SUMMARY: 255 records, 64 requests, 59.8 MB audited, 1 artifact(s)
  SIGNATURES: ✅ All 256 signatures valid
  COMPLETENESS: ✅ All channels closed
════════════════════════════════════════════════════════════════════════════════
```

Each line represents a complete request lifecycle (open → data transfer → close) with a valid signature chain.

### Record types

- **Container registry requests** — base image pulls from Docker Hub or other registries
- **Package manager fetches** — apt, pip, npm, cargo dependencies
- **`http://cwd/` requests** — source files from your build context (COPY provenance)
- **ARTIFACT** — build outputs posted back via `http://artifacts/`

### Signature verification

The ✅ indicates all signatures in the chain are valid. If any record were altered, reordered, or removed, the chain would break and show ❌.

## Verbosity levels

```sh
# Compact (default) — one line per request
warden inspect warden-output

# Tree — shows checkpoints within each request
warden inspect --verbosity 1 warden-output

# Full — all record details
warden inspect --verbosity 2 warden-output
```

## JSON output

For scripting and CI integration:

```sh
warden inspect --json warden-output
```

Returns a structured JSON document with header info, all records, and a summary object.

## Exit codes

- `0` — ledger is valid
- `1` — verification failed (signature errors or parse failure)
