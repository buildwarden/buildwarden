# BuildWarden Ledger Specification

## Purpose

A build ledger is a document providing a completeness guarantee for inputs and outputs during a build process as strictly-ordered entries with a chaining signature. It is produced by the BuildWarden relay — an ephemeral TLS-terminating proxy that observes all network traffic during an isolated build.

The ledger is designed to be:

- **Verifiable**: Any holder of the ledger can validate the signature chain using only the public key embedded in the header. Verification requires no knowledge of hash algorithm names or metadata schemas — only the declared sizes.
- **Tamper-evident**: Altering, reordering, inserting, or removing any entry invalidates all subsequent signatures.
- **Redactable**: Metadata is CBOR-encoded, not part of signatures, and can be stripped or modified without affecting ledger integrity.
- **Complete**: Every network resource consumed or produced during the build is recorded with content-addressable identity (size + ordered hash block).
- **Deterministic**: All signed content has a fixed byte layout derivable from header parameters. No parsing ambiguity exists in the signed portion of any record.

## Conventions

- All multi-byte integers are **big-endian** (network byte order).
- Strings in the binary prefix of the header are **null-terminated UTF-8**.
- CBOR objects follow [RFC 8949](https://www.rfc-editor.org/rfc/rfc8949.html).
- Signature inputs are the raw concatenation of the specified fields — no additional framing or separators.

## File Layout

```
<ledger_root>/
├── ledger                  # The binary ledger file
├── ledger.cert.pem         # Public certificate in PEM format (convenience copy)
├── payloads/
│   └── <hex_hash>          # Payload files stored by primary hash
├── artifacts/
│   └── <name>              # Artifact files (or symlinks to payloads/)
└── metadata/
    └── <stream>            # Optional append-only metadata streams
```

## Ledger File Structure

The ledger file is a concatenation of:

```
[Header] [Record 0] [Record 1] ... [Record N]
```

There are no delimiters between records. Record boundaries are determined by the record structure and sizes declared in the header.

---

## Header

The header establishes the cryptographic parameters, embeds the public key, and provides informational metadata.

### Binary Prefix

| Offset | Size | Field | Description |
|--------|------|-------|-------------|
| 0 | 4 | Magic | ASCII `BLDL` (0x42 0x4C 0x44 0x4C) |
| 4 | 1 | Version | Ledger spec version (0x01 for this spec) |
| 5 | variable | Signature scheme | Null-terminated UTF-8 string (e.g., `ed25519-sha512\0`) |
| — | 2 | Signature size | uint16 — byte length of all signatures in this ledger |
| — | 2 | Hash block size | uint16 — total byte length of concatenated hashes per payload |
| — | 2 | Public key length | uint16 — byte length of the public key |
| — | variable | Public key bytes | Raw public key (length per above field) |

The binary prefix ends immediately after the public key bytes. Its total size is:

```
5 + len(signature_scheme) + 1 + 2 + 2 + 2 + public_key_length
```

### Header Signature

Immediately following the binary prefix:

| Size | Field | Description |
|------|-------|-------------|
| signature-size | Header signature | Signs all bytes of the binary prefix |

**Signature input**: All bytes from offset 0 through the end of the public key bytes (the entire binary prefix).

The header signature serves as the **chain root** — it becomes the `prev_sig` for the first record.

### Header Metadata (CBOR)

Following the header signature:

| Size | Field | Description |
|------|-------|-------------|
| 4 | Metadata length | uint32 — byte length of the CBOR object that follows |
| variable | CBOR object | Informational header metadata |

The header metadata is a CBOR map containing:

```cbor
{
  "hashes": ["blake2b_256", "sha256", "sha1", "md5"],
  "schemas": [
    "https://github.com/buildwarden/buildwarden/schemas/http-open.json",
    "https://github.com/buildwarden/buildwarden/schemas/http-headers.json",
    "https://github.com/buildwarden/buildwarden/schemas/http-body.json",
    "https://github.com/buildwarden/buildwarden/schemas/artifact.json",
    "https://github.com/buildwarden/buildwarden/schemas/redacted.json"
  ],
  "environment": {
    "type": "vm-disk-chain/v1"
  }
}
```

| Key | Type | Description |
|-----|------|-------------|
| `hashes` | array of text | Ordered list of hash algorithm names. The concatenation of their outputs equals `hash_block_size` bytes. |
| `schemas` | array of text | Ordered list of JSON Schema URLs. Index position maps to the schema index byte in records. |
| `environment` | map | `{"type": <environment type>}`: an untrusted label echoing the type the [environment record](#environment-record)'s signed payload names. Absent when the ledger has no environment record. Ledgers written before environment types were specified carry `{"type": "container"}`, which readers treat as `oci-image/v1`. |

The header metadata is **not signed** and may be modified without invalidating the ledger. It is informational context for consumers.

**Note**: The `schemas` list may be updated after ledger creation (e.g., appending new schemas) without invalidating the signature chain, since header metadata is not signed. Existing schema indices remain stable.

---

## Record Types

| Byte Value | Name | Description |
|------------|------|-------------|
| 0x01 | `open` | Opens a new channel |
| 0x02 | `checkpoint` | Intermediate data on an open channel |
| 0x03 | `close` | Closes a channel |
| 0x04 | `artifact` | Closes a channel, marking the payload as a build artifact |

---

## Record Layout

### Open Record

```
[Record type: 1 byte]           — 0x01
[Previous signature]            — signature-size bytes
[Payload size: 8 bytes]         — int64, big-endian (typically 0)
[Hash block]                    — hash-block-size bytes (OMITTED if payload size = 0)
[Record signature]              — signature-size bytes
[Schema index: 1 byte]          — 0-254 = index, 255 = no metadata
[Metadata length: 4 bytes]      — uint32 (OMITTED if schema index = 255)
[CBOR metadata]                 — (OMITTED if schema index = 255)
```

Open records do **not** include an open-signature field.

**Signature input**:
```
record_type(0x01) + prev_sig + payload_size
```

When payload size = 0 (the common case), the signature input is:
```
0x01 + prev_sig + 0x0000000000000000
```

When payload size ≠ 0, the hash block is appended to the signature input:
```
0x01 + prev_sig + payload_size_bytes + hash_block
```

### Checkpoint Record

```
[Record type: 1 byte]           — 0x02
[Previous signature]            — signature-size bytes
[Open signature]                — signature-size bytes
[Payload size: 8 bytes]         — int64, big-endian (sign = direction)
[Hash block]                    — hash-block-size bytes (OMITTED if payload size = 0)
[Record signature]              — signature-size bytes
[Schema index: 1 byte]          — 0-254 = index, 255 = no metadata
[Metadata length: 4 bytes]      — uint32 (OMITTED if schema index = 255)
[CBOR metadata]                 — (OMITTED if schema index = 255)
```

**Signature input**:
```
record_type(0x02) + prev_sig + open_sig + payload_size_bytes + hash_block
```

When payload size = 0:
```
0x02 + prev_sig + open_sig + 0x0000000000000000
```

### Close Record

```
[Record type: 1 byte]           — 0x03
[Previous signature]            — signature-size bytes
[Open signature]                — signature-size bytes
[Payload size: 8 bytes]         — int64, big-endian (sign = direction)
[Hash block]                    — hash-block-size bytes (OMITTED if payload size = 0)
[Record signature]              — signature-size bytes
[Schema index: 1 byte]          — 0-254 = index, 255 = no metadata
[Metadata length: 4 bytes]      — uint32 (OMITTED if schema index = 255)
[CBOR metadata]                 — (OMITTED if schema index = 255)
```

**Signature input**: Identical structure to checkpoint.
```
record_type(0x03) + prev_sig + open_sig + payload_size_bytes + hash_block
```

### Artifact Record

```
[Record type: 1 byte]           — 0x04
[Previous signature]            — signature-size bytes
[Open signature]                — signature-size bytes
[Payload size: 8 bytes]         — int64, big-endian
[Hash block]                    — hash-block-size bytes (OMITTED if payload size = 0)
[Record signature]              — signature-size bytes
[Schema index: 1 byte]          — 0-254 = index, 255 = no metadata
[Metadata length: 4 bytes]      — uint32 (OMITTED if schema index = 255)
[CBOR metadata]                 — (OMITTED if schema index = 255)
```

**Signature input**: Identical structure to close.
```
record_type(0x04) + prev_sig + open_sig + payload_size_bytes + hash_block
```

The artifact record closes its channel (same semantics as `close`). Its record type structurally identifies the payload as a build output.

**Constraints**:
- Payload size of 0 is valid (empty artifact).
- When payload size is non-zero, it SHOULD be negative (indicating `out` direction), though validators MUST NOT reject based on sign alone — the record type is authoritative.

---

## Payload Size and Direction

The payload size field is a **signed 64-bit big-endian integer**.

| Value | Meaning |
|-------|---------|
| > 0 (positive) | Data flowing **into** the build (response bodies, downloaded resources) |
| < 0 (negative) | Data flowing **out of** the build (request bodies, artifact submissions) |
| = 0 | No payload / directionless |

The absolute value of the payload size is the byte length of the payload content. When non-zero, the hash block immediately follows the payload size field.

When payload size = 0, the hash block is **omitted entirely** — no bytes are present for it. This applies uniformly to all record types.

---

## Hash Block

The hash block is the raw concatenation of hash digests in the order declared by the header's `hashes` array. Its total size equals the header's `hash_block_size` field.

Example with default hashes `["blake2b_256", "sha256", "sha1", "md5"]`:

| Hash | Output Size |
|------|-------------|
| blake2b_256 | 32 bytes |
| sha256 | 32 bytes |
| sha1 | 20 bytes |
| md5 | 16 bytes |
| **Total** | **100 bytes** |

The hash block size declared in the header would be 100 (0x0064).

Payload content is written to `payloads/<primary_hash_hex>` where the primary hash is the first algorithm in the header's hash list.

---

## Signature Computation

### Signature Schemes

The signature scheme is declared as a null-terminated string in the header. The scheme name encodes both the signing algorithm and the digest algorithm.

| Scheme | Signing Algorithm | Digest | Key Size | Signature Size |
|--------|-------------------|--------|----------|----------------|
| `ed25519-sha512` | Ed25519 | SHA-512 | 32 bytes | 64 bytes |
| `rsa-pkcs1v15-sha512` | RSA PKCS#1 v1.5 | SHA-512 | variable | variable |

For all schemes:
1. Concatenate the signature input bytes as specified per record type.
2. Compute the digest (SHA-512) of the concatenated input.
3. Sign the digest with the private key using the declared algorithm.

### Chain Integrity

Each record's signature incorporates the previous record's signature (`prev_sig`), forming a hash chain:

```
Header sig ← Record 0 sig ← Record 1 sig ← ... ← Record N sig
```

Altering, removing, or reordering any record breaks the chain for all subsequent records.

---

## Metadata

### Schema Index

Each record carries a 1-byte schema index:

| Value | Meaning |
|-------|---------|
| 0–254 | Index into the header's `schemas` array |
| 255 | No metadata attached |

When schema index = 255, no metadata length or CBOR bytes follow. When schema index is 0–254, a 4-byte uint32 length followed by that many bytes of CBOR-encoded metadata immediately follow.

Metadata is **never** part of the signature input. It can be freely added, modified, or stripped without affecting ledger validity.

### One Schema Per Record

Each record has at most one metadata attachment. The schema index identifies which schema the CBOR object conforms to.

---

## Default Schemas

BuildWarden defines the following default schemas. Their indices are determined by position in the header's `schemas` array (not fixed by this spec).

### `http-open`

Attached to `open` records for HTTP/HTTPS channels.

```cbor
{
  "method": "GET",
  "url": "https://example.com/path",
  "protocol": "HTTP/1.1"
}
```

| Field | Type | Description |
|-------|------|-------------|
| `method` | text | HTTP method |
| `url` | text | Full request URL |
| `protocol` | text | Protocol version |

### `http-headers`

Attached to `checkpoint` records carrying HTTP headers as payload.

```cbor
{
  "headers": [
    ["X-Amz-Request-Id", "abc123"],
    ["X-Custom-Header", "value"],
    ["Authorization", "<redacted>"],
    ["Cookie", "<redacted>"],
    ["Set-Cookie", "<redacted>"]
  ]
}
```

| Field | Type | Description |
|-------|------|-------------|
| `headers` | array of [name, value] | Non-standard headers. Auth-related headers (`Authorization`, `Cookie`, `Set-Cookie`, `Proxy-Authorization`) are listed with value `<redacted>`. Standard/structural HTTP headers (those defined in HTTP/1.1 and HTTP/2 base specifications) are omitted — they exist in the raw payload bytes. |

### `http-body`

Attached to `checkpoint` or `close` records carrying HTTP body content as payload.

```cbor
{
  "status": 200
}
```

| Field | Type | Description |
|-------|------|-------------|
| `status` | unsigned int | HTTP response status code (present only for response bodies) |

### `artifact`

Attached to `artifact` records.

```cbor
{
  "name": "my-binary",
  "context": {
    "content_type": "application/octet-stream"
  }
}
```

| Field | Type | Description |
|-------|------|-------------|
| `name` | text | Filename as stored in the ledger's `artifacts/` directory |
| `context` | map | Freeform key-value map for additional artifact metadata |

### `redacted`

Attached to any record whose original metadata has been stripped or was never recorded.

```cbor
{
  "owner": "build.example.com"
}
```

| Field | Type | Description |
|-------|------|-------------|
| `owner` | text | Identifier of the party responsible for or owning the redacted information |

### `environment`

Attached to the [environment record](#environment-record)'s `open` and `close`.

```cbor
{
  "type": "oci-image/v1",
  "reference": "docker.io/library/python:3.12-slim",
  "digest": "sha256:<64 hex>",
  "mediaType": "application/vnd.oci.image.manifest.v1+json"
}
```

| Field | Type | Description |
|-------|------|-------------|
| `type` | text | `"environment"` on the open record; on the close record an echo of the payload's environment type (unsigned; the payload is authoritative) |
| `driver` | text | `vm-disk-chain/v1`: the driver that booted the VM |
| `reference`, `digest`, `mediaType` | text | `oci-image/v1`: see [`oci-image/v1`](#oci-imagev1) |

---

## Environment Record

The first records after the header describe the build environment: an `open`
followed by a `close` on the environment metadata schema
(`schemas/v1/environment.json`; ledgers written before typed environments
reference `environment-container.json`). The close record's payload size is
`−len(payload)` and its hash block hashes the **environment payload**, a
descriptor of what the build ran on. The payload itself is not in the ledger:
it is written beside it as `environment.json` in the output directory (by the
collector, the relay's only output path), and a verifier hashes that file and
compares it with the signed hash block.

| # | Type | Payload Dir | Payload Content | Schema |
|---|------|-------------|-----------------|--------|
| 1 | open | — (0) | — | `environment` (`{"type": "environment"}`) |
| 2 | close | out (−) | Environment payload | `environment` (`{"type": <environment type>, ...}`) |

A ledger may have no environment record (for example one written by a
third-party orchestrator that sends none).

### What the record proves

The relay signs what the **orchestrator asserts**. The orchestrator is trusted:
it builds the payload host-side, before the build guest starts, from files the
guest never writes. The guest is not trusted and has no way to influence the
payload. The relay does not inspect the environment itself; the record binds
the orchestrator's statement into the signed chain, and it is exactly as
trustworthy as the orchestrator and the host it ran on. It is not a
measurement taken inside the guest.

### Environment types

Because the header label, the schema index and record metadata are all
unsigned, **the payload names its own type inside the signed bytes**. A reader
takes the type from the payload, never from the label or metadata; a relay
refuses an environment whose declared type disagrees with the type its payload
names, so an honest ledger's label always matches.

Each environment type is specified by a section here and a JSON schema
(`schemas/v1/environment-<type>.json`) stating what the payload must contain.
Adding a type is a new section and schema; the wire format does not change.
Type names carry their own major version (`/v1`).

#### `oci-image/v1`

A container image. Schema: `environment-oci-image.json`.

- **Payload**: the image's OCI image manifest or index (or Docker v2
  equivalent), exactly as the container runtime returned it. For compatibility
  with ledgers written before typed environments this is the one type whose
  payload is not a wrapper object: it is identified by the manifest's own
  `schemaVersion` together with `layers`, `manifests` or a manifest/index
  `mediaType`.
- **Metadata** (unsigned): `reference` (as the user gave it), `digest` (the
  registry digest of the pulled reference), `mediaType`.
- **Verify**: hash the payload against the record. `digest` equals
  `sha256(payload)` only when the runtime returned the registry's exact bytes;
  runtimes may re-serialize the manifest, and the digest may name an index, so
  a mismatch there is informational.

#### `vm-disk-chain/v1`

A virtual machine booted from an ordered chain of disk layers plus per-build
seed files. Schema: `environment-vm-disk-chain.json`.

```json
{
  "type": "vm-disk-chain/v1",
  "driver": "hyperv",
  "guest_os": "windows",
  "arch": "amd64",
  "generation": 1,
  "layers": [
    {"role": "disk", "name": "windows-server-2025.vhd", "format": "vhd",
     "path": "C:\\Users\\me\\AppData\\Local\\warden\\hyperv\\images\\windows-server-2025.vhd",
     "sha256": "<64 hex>", "size": 10737418240,
     "source": {"kind": "url", "url": "https://...", "pinned": true}}
  ],
  "seed": [
    {"name": "build-script", "sha256": "<64 hex>", "size": 120},
    {"name": "warden-io.exe", "sha256": "<64 hex>", "size": 9123456}
  ]
}
```

| Field | Description |
|-------|-------------|
| `type` | `"vm-disk-chain/v1"`. |
| `driver` | The driver that booted the VM (`qemu`, `vz`, `hyperv`). |
| `guest_os`, `arch` | Guest OS (`linux`, `windows`, `macos`) and Go architecture name. |
| `generation` | Hyper-V VM generation; omitted elsewhere. |
| `layers` | Ordered files the VM boots from. Entries with `role: disk` form the disk chain **base-first**: each later disk is a child (differencing / backing-file overlay) of the one before. The per-build writable child is created empty from the last disk and is not a layer. Other roles: `kernel`, `initrd` (direct boot), `nvram` (UEFI variables, VZ auxiliary storage), `platform` (VZ hardware model, machine identifier). |
| `layers[].sha256`, `size` | SHA-256 and size of the file as booted. |
| `layers[].path` | Host path at build time, where a verifier finds the file. Host-local; it may reveal the host's directory layout. |
| `layers[].source` | Provenance. `kind: url` — downloaded from `url`; `pinned` when it verified against an expected hash supplied independently of the download. `kind: local` — supplied by the user, or changed since warden recorded it. `kind: prepared` — produced by warden: `prepared_by` is `"warden <version>"` and `inputs` lists the hashed inputs (`{name, sha256, size?, url?}`) it was produced from, e.g. the install ISO or IPSW, or for a prepared differencing layer the preparation script and payloads. |
| `seed` | Per-build files handed to the guest (agent binary, bootstrap script, answer files, cloud-init data, the build script as `build-script`), sorted by `name`. They are per-build and generally not available to re-check later; their hashes record what the guest was given. |

The encoding is deterministic for a given descriptor (fixed member order, sorted
seed).

**Verify**: (1) hash `environment.json` against the record; (2) for each layer,
hash the file at `path` (or a copy obtained elsewhere) and compare `sha256` and
`size`. Disks are never embedded in the ledger.

---

## HTTP Protocol Mapping

For HTTP/HTTPS requests proxied by the relay, the typical entry sequence is:

### GET Request (resource consumed by build)

| # | Type | Payload Dir | Payload Content | Schema |
|---|------|-------------|-----------------|--------|
| 1 | open | — (0) | — | `http-open` |
| 2 | checkpoint | out (−) | Request headers (raw bytes) | `http-headers` |
| 3 | checkpoint | in (+) | Response headers (raw bytes) | `http-headers` |
| 4 | close | in (+) | Response body | `http-body` |

### POST Request (data sent from build)

| # | Type | Payload Dir | Payload Content | Schema |
|---|------|-------------|-----------------|--------|
| 1 | open | — (0) | — | `http-open` |
| 2 | checkpoint | out (−) | Request headers (raw bytes) | `http-headers` |
| 3 | checkpoint | out (−) | Request body | `http-body` |
| 4 | checkpoint | in (+) | Response headers (raw bytes) | `http-headers` |
| 5 | close | in (+) | Response body | `http-body` |

### Artifact Submission

| # | Type | Payload Dir | Payload Content | Schema |
|---|------|-------------|-----------------|--------|
| 1 | open | — (0) | — | `http-open` |
| 2 | checkpoint | out (−) | Request headers (raw bytes) | `http-headers` |
| 3 | artifact | out (−) | Artifact body | `artifact` |

---

## Redactability

Metadata is excluded from all signatures. This allows:

- Stripping URLs, hostnames, and internal identifiers from `http-open` metadata.
- Replacing any record's metadata with the `redacted` schema (changing the schema index byte and CBOR content).
- Removing metadata entirely (setting schema index to 255 and removing the length + CBOR bytes).

After redaction, the signature chain remains fully valid. The content identity (payload size + hash block) is preserved, proving *what* was transferred without revealing *where* it came from.

The `redacted` schema provides attribution — identifying who owns or is responsible for the redacted information — enabling downstream consumers to request the full metadata from the appropriate party if needed.

---

## Completeness

A channel is **complete** when its open entry has a corresponding close or artifact entry.

A ledger is **complete** when all opened channels are closed.

A **payload's provenance** is complete when:
1. The channel containing the payload is closed.
2. All channels opened before that close are themselves closed.

---

## Verification Algorithm

A minimal verifier needs only:
- The binary prefix fields (signature scheme, signature size, hash block size, public key)
- The ability to compute the declared digest (SHA-512) and verify the declared signature scheme

```
1. Read binary prefix, extract: sig_scheme, sig_size, hash_block_size, public_key
2. Verify header signature over the binary prefix bytes
3. Set prev_sig = header_signature
4. Skip header metadata (read 4-byte length, skip that many bytes)
5. For each record:
   a. Read record_type (1 byte)
   b. Read prev_sig_field (sig_size bytes) — must equal prev_sig
   c. If record_type != 0x01: read open_sig (sig_size bytes)
   d. Read payload_size (8 bytes, signed int64)
   e. If payload_size != 0: read hash_block (hash_block_size bytes)
   f. Reconstruct signature input from fields read in (a–e)
   g. Read record_signature (sig_size bytes)
   h. Verify record_signature against signature input using public_key
   i. Set prev_sig = record_signature
   j. Read schema_index (1 byte)
   k. If schema_index != 255: read metadata_length (4 bytes), skip that many bytes
6. If all signatures verify: ledger is valid
```

Note: The verifier does not need a CBOR parser, knowledge of hash algorithm names, or awareness of metadata schemas. It operates entirely on the deterministic byte layout.

---

## Error Handling

If a network request fails (timeout, connection reset, server error):
- The relay still emits a close entry for the channel.
- The close entry's payload size may be 0 (nothing received).
- Metadata may indicate the error condition.
- No open channel is left dangling in a well-formed ledger.

---

## Future Considerations

- **File-system tracing**: Additional record types for local file I/O via syscall tracing.
- **Payload recording configuration**: Configurable rules for which payloads are stored to disk vs. only hashed.
- **Auto-redaction**: Configuration-driven redaction of metadata for specific hostnames at ledger-creation time.
- **Additional signature schemes**: The null-terminated scheme string and explicit size fields allow new schemes without format changes.
- **Compression**: CBOR metadata could be compressed; the length-prefix framing supports this transparently.

---

## Appendix: Worked Example

Given: `ed25519-sha512` scheme, default hash set `[blake2b_256, sha256, sha1, md5]`.

Header parameters:
- Signature size: 64 bytes
- Hash block size: 100 bytes (32 + 32 + 20 + 16)
- Public key length: 32 bytes

### Record sizes (excluding metadata)

| Record Type | Fixed Size |
|-------------|-----------|
| Open (payload=0) | 1 + 64 + 8 + 64 + 1 = **138 bytes** |
| Open (payload≠0) | 1 + 64 + 8 + 100 + 64 + 1 = **238 bytes** |
| Checkpoint/Close/Artifact (payload=0) | 1 + 64 + 64 + 8 + 64 + 1 = **202 bytes** |
| Checkpoint/Close/Artifact (payload≠0) | 1 + 64 + 64 + 8 + 100 + 64 + 1 = **302 bytes** |

Metadata adds: 4 bytes (length) + CBOR content length.
