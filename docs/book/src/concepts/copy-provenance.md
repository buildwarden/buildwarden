# COPY Provenance

Docker `COPY` directives introduce build inputs that traditionally bypass network auditing — files go directly from the host into the image layer. BuildWarden rewrites these directives so that every source file flows through the relay and gets recorded in the ledger.

## How It Works

1. The orchestrator mounts the build context into the relay container (read-only)
2. The relay serves these files at `http://cwd/<path>`
3. `COPY` directives in the Dockerfile are rewritten to `RUN curl` commands that fetch each file from the relay
4. Each file transfer creates a ledger entry with full hash verification

## Example Transformation

Original Dockerfile:
```dockerfile
COPY requirements.txt /app/
COPY src/ /app/src/
```

What actually runs:
```dockerfile
RUN mkdir -p /app/ && curl -fsSL -o /app/requirements.txt "http://cwd/requirements.txt"
RUN mkdir -p /app/src/ && \
    printf '%s\n' 'src/main.py' 'src/lib.py' 'src/utils.py' | \
    xargs -P8 -I{} sh -c 'mkdir -p "/app/src/$(dirname "{}")" && curl -fsSL -o "/app/src/{}" "http://cwd/{}"'
```

## What Gets Excluded

- `.git/` — not part of the build context
- `.warden/` — ephemeral BuildWarden infrastructure
- Files matching `.dockerignore` patterns

## Concurrency

For directories with many files, downloads run with `xargs -P8` (8 parallel fetches). Since transfers are over localhost to the relay, this adds negligible overhead.

## Large Contexts and Tarballs

If your build context has thousands of files and you don't need per-file provenance, create a tarball first:

```dockerfile
# In your build script, before warden build:
tar czf context.tar.gz src/

# In Dockerfile:
COPY context.tar.gz /tmp/
RUN tar xzf /tmp/context.tar.gz -C /app && rm /tmp/context.tar.gz
```

This produces a single ledger entry for the tarball rather than thousands of individual file entries.

## Multi-stage Builds

`COPY --from=<stage>` directives (copies between build stages) are left unchanged — they don't reference the build context and don't need provenance tracking.

## Supported Flags

| Flag | Handling |
|------|----------|
| `--chown=user:group` | Appended as `&& chown -R user:group <dest>` |
| `--chmod=755` | Appended as `&& chmod -R 755 <dest>` |
| `--from=stage` | Left as-is (multi-stage copy) |

## In the Ledger

Source file entries appear as:
```
✅ GET http://cwd/src/main.py (1523 bytes)
✅ GET http://cwd/src/lib.py (4201 bytes)
✅ GET http://cwd/requirements.txt (42 bytes)
```

Each with full blake2b_256, sha256, sha1, and md5 hashes in the hash block.
