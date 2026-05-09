# BuildWarden Ledger Specification v2

## Purpose

A build ledger is a document providing a completeness guarantee for inputs and outputs during a build process as strictly-ordered entries with a chaining signature. It is produced by the BuildWarden relay — an ephemeral TLS-terminating proxy that observes all network traffic during an isolated build.

The ledger is designed to be:

- **Verifiable**: Any holder of the ledger can validate the signature chain using only the public certificate embedded in the header.
- **Tamper-evident**: Altering, reordering, inserting, or removing any entry invalidates all subsequent signatures.
- **Redactable**: Metadata (URLs, hostnames, internal repo names) is not part of signatures. Internal builds can strip origin information while preserving integrity.
- **Complete**: Every network resource consumed or produced during the build is recorded with content-addressable identity (size + multi-hash).

## File Layout

```
<ledger_root>/
├── ledger                  # The ledger file (header + newline-delimited entries)
├── ledger.cert.pem         # Public certificate in PEM format
├── ledger.cert.der         # Public certificate in DER format
├── payloads/
│   └── <hash>/             # Payload files stored by primary hash
├── artifacts/
│   └── <name> -> ../payloads/<hash>   # Symlinks to payload files
└── metadata/
    └── <stream>            # Append-only metadata streams
```

## Ledger File Format

The ledger file consists of a **header entry** followed by zero or more **entries**, each as a single line of JSON terminated by a newline character.

### Hash Algorithms

The ledger uses a configurable ordered set of hash algorithms. All hashes are computed for every payload. The order declared in the header is the order used for concatenation in signature generation.

Default set: `blake2b_256`, `sha256`, `sha1`, `md5`

The combined use of algorithmically diverse hashes provides stronger identification than any single algorithm. SHA-1 and MD5 are included for compatibility with existing package registries, not for security guarantees.

## Header Entry

The first line of the ledger. Establishes the chain root.

```json
{
  "entry_type": "header",
  "version": "2.0",
  "format": "json",
  "signature_scheme": "ed25519-sha512",
  "hashes": ["blake2b_256", "sha256", "sha1", "md5"],
  "environment": {
    "type": "container",
    "digest": "<container_digest>"
  },
  "payload": {
    "size": <public_cert_byte_length>,
    "hashes": {
      "blake2b_256": "<hex>",
      "sha256": "<hex>",
      "sha1": "<hex>",
      "md5": "<hex>"
    }
  },
  "signature": "<base64>"
}
```

**Signature input** (no previous signature — this is the chain root):

```
sign("header" + size_bytes + blake2b_256 + sha256 + sha1 + md5)
```

Where:
- `size_bytes` is the payload size as a little-endian 64-bit unsigned integer
- Hash values are concatenated as raw bytes in header-declared order
- The payload is the public key bytes (also written to `ledger.cert.pem`)
- The `signature_scheme` field declares which signing algorithm and digest are used

The header's signature becomes the `prev_sig` for the first subsequent entry.

## Entry Types

All entries after the header follow a common structure:

```json
{
  "entry_type": "open|checkpoint|close",
  "open_signature": "<base64>",
  "direction": "in|out",
  "payload": { ... },
  "signature": "<base64>",
  "seq": <monotonic_counter>,
  "timestamp": "<ISO-8601>",
  "metadata": { ... }
}
```

### Fields

| Field | Presence | In Signature | Description |
|-------|----------|--------------|-------------|
| `entry_type` | Always | **Yes** | One of `open`, `checkpoint`, `close` |
| `open_signature` | checkpoint, close | **Yes** | Signature of the associated open entry |
| `direction` | checkpoint, close | **Yes** | `in` (consumed by build) or `out` (produced by build) |
| `payload` | checkpoint, close | **Yes** (size + hashes) | Content identity |
| `signature` | Always | — | This entry's computed signature |
| `seq` | Always | No | Monotonic counter for navigation |
| `timestamp` | Always | No | Wall-clock time for analysis |
| `metadata` | Optional | **No** | Redactable annotations (URL, method, protocol, etc.) |

### Payload Object

```json
{
  "size": <uint64>,
  "hashes": {
    "blake2b_256": "<hex>",
    "sha256": "<hex>",
    "sha1": "<hex>",
    "md5": "<hex>"
  }
}
```

Payload contents are written to `payloads/<primary_hash>` where `primary_hash` is the first hash in the header's hash list (default: `blake2b_256`).

The `hashes` object is included in the JSON for legibility. For signature computation, the raw hash bytes are concatenated in header-declared order.

## Signature Computation

The signature scheme is declared in the header's `signature_scheme` field. Implementations must support the scheme declared in the header to verify the ledger.

### Signature Schemes

| Scheme | Algorithm | Digest | Key Type |
|--------|-----------|--------|----------|
| `ed25519-sha512` | Ed25519 | SHA-512 | Ed25519 (32-byte public key) |
| `rsa-pkcs1v15-sha512` | RSA PKCS#1 v1.5 | SHA-512 | RSA (2048-bit minimum) |

The reference implementation uses `ed25519-sha512`. The signature input construction is identical for all schemes — only the final signing primitive differs.

For all schemes, the signature input bytes are first hashed with the declared digest algorithm (SHA-512), then the digest is signed with the declared algorithm.

### Open Entry

```
digest = SHA512(prev_sig + "open")
signature = SIGN(digest)
```

- `prev_sig`: raw bytes of the previous entry's signature (decoded from base64)
- `"open"`: the entry type as UTF-8 bytes

The open entry has no payload, no direction, and no open_signature reference. Its signature serves as the **channel identifier** for all subsequent checkpoints and closes on this channel.

### Checkpoint Entry

```
digest = SHA512(prev_sig + open_sig + "checkpoint" + direction + size_bytes + hash_bytes)
signature = SIGN(digest)
```

- `open_sig`: raw bytes of the associated open entry's signature
- `direction`: `"in"` or `"out"` as UTF-8 bytes
- `size_bytes`: payload size as little-endian uint64
- `hash_bytes`: raw hash bytes concatenated in header-declared order

### Close Entry

```
digest = SHA512(prev_sig + open_sig + "close" + direction + size_bytes + hash_bytes)
signature = SIGN(digest)
```

Same structure as checkpoint.

### Digest Algorithm

The digest algorithm used for signature computation is SHA-512. This is the algorithm applied to the concatenated signature input before signing. It is distinct from the payload hash algorithms.

## Synchronous Open Protocol

The relay processes all ledger writes through a single serial channel. When a request handler needs to open a new channel:

1. Handler sends an open request to the ledger writer
2. Handler **blocks** until the ledger writer has written the open entry and returned its signature
3. Handler uses the returned signature as the channel identifier for all subsequent checkpoint/close entries on this connection
4. Handler proceeds with the actual network request

This guarantees that an open entry is always on the ledger before any data flows for that channel, and that the open's signature is available for referencing.

Checkpoint and close entries do not require synchronous confirmation — they are enqueued and written in order. The single-writer goroutine ensures strict serialization.

## HTTP Protocol Mapping

For HTTP/HTTPS requests proxied by the relay, the entry sequence is:

### GET Request (resource consumed by build)

| Seq | Type | Direction | Payload | Metadata (redactable) |
|-----|------|-----------|---------|----------------------|
| N | open | — | — | `{method: "GET", url: "...", protocol: "HTTP/1.1"}` |
| N+1 | checkpoint | out | request headers (raw bytes) | — |
| N+2 | checkpoint | in | response headers (raw bytes) | — |
| N+3 | close | in | response body | `{status: 200}` |

### POST Request (artifact produced by build)

| Seq | Type | Direction | Payload | Metadata (redactable) |
|-----|------|-----------|---------|----------------------|
| N | open | — | — | `{method: "POST", url: "...", protocol: "HTTP/1.1"}` |
| N+1 | checkpoint | out | request headers (raw bytes) | — |
| N+2 | checkpoint | out | request body | — |
| N+3 | checkpoint | in | response headers (raw bytes) | — |
| N+4 | close | in | response body | `{status: 200}` |

### Direction Semantics

- `out`: Data flowing from the build process outward (request headers, request bodies, artifact submissions)
- `in`: Data flowing into the build process (response headers, response bodies, downloaded resources)

Direction reflects build-process semantics, not raw network direction.

## Completeness

A channel is **complete** when its open entry has a corresponding close entry.

A **payload's ledger** is complete when:
1. The payload's close entry exists
2. All channels opened before that close entry are themselves closed

This handles the case where a streaming resource is partially consumed before being fully recorded — the ledger is only considered complete for a given output once all potentially-contributing inputs are also fully recorded.

## Ledger Truncation

A ledger may be truncated after any point where all channels opened up to that point are closed. This allows post-build activities (test execution, documentation generation) to be stripped without compromising the integrity of earlier artifact entries.

Truncation preserves:
- The header (always first)
- All entries up to the truncation point
- The signature chain remains valid for the retained portion

## Redactability

Metadata fields are explicitly excluded from signatures. This allows:
- Internal builds to strip proprietary repository URLs, package names, or authentication details
- The content identity (size + multi-hash) to remain, proving *what* was used without revealing *where* it came from
- Ledger integrity to be preserved after redaction

A redacted ledger is still fully verifiable — the signature chain depends only on entry types, channel references, directions, and content hashes.

## Error Handling

If a network request fails (timeout, connection reset, server error):
- The relay still emits a close entry for the channel
- The close entry's payload contains whatever was received (possibly empty, size 0)
- Metadata may indicate the error condition (e.g., `{error: "timeout"}`)
- The ledger remains valid — no open channel is left dangling

## Future Considerations

- **File-system tracing**: Extending the entry model to local file reads via syscall tracing
- **Protocol extensibility**: The `metadata.protocol` field in open entries allows future protocols (FTP, NFS) to define their own checkpoint semantics
- **Binary format**: The `format` field in the header allows future switch from JSON to a more compact binary encoding
- **Configurable hash sets**: The hash list in the header is authoritative; implementations must support at minimum the default set but may add others
