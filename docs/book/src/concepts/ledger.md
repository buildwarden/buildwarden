# The Ledger

The ledger is a binary file containing a cryptographic record of everything that happened during a build.

## Properties

- **Tamper-evident** — Each record signs over the previous record's signature, forming a chain. Altering any record invalidates all subsequent signatures.
- **Append-only** — Records can only be added, never modified or reordered.
- **Self-describing** — The header contains the signature scheme, hash algorithms, and the public key needed for verification.
- **Redactable metadata** — CBOR-encoded metadata is not part of signatures and can be stripped without affecting integrity.

## Structure

```
┌────────────────────────────────────┐
│  Magic bytes: BLDL                 │
│  Version: 0x01                     │
│  Header (signed, contains pubkey)  │
├────────────────────────────────────┤
│  Record 1: OPEN (request start)    │
│  Record 2: CHECKPOINT (headers)    │
│  Record 3: CLOSE (response body)   │
│  Record 4: OPEN ...                │
│  ...                               │
│  Record N: ARTIFACT (build output) │
└────────────────────────────────────┘
```

## Record Types

| Type | Byte | Meaning |
|------|------|---------|
| Open | 0x01 | Start of a request lifecycle (HTTP open) |
| Checkpoint | 0x02 | Intermediate data (headers, partial body) |
| Close | 0x03 | End of a request lifecycle (response complete) |
| Artifact | 0x04 | Build output posted to the ledger |

## Hash Block

Every record includes a hash block with multiple algorithms computed over the payload:

- blake2b_256 (32 bytes)
- sha256 (32 bytes)
- sha1 (20 bytes)
- md5 (16 bytes)

This multi-hash approach ensures compatibility with various verification systems and provides defense-in-depth against hash collisions.

## Signature Chain

```
Header.sig = Ed25519(SHA512(header_prefix))
Record[0].sig = Ed25519(SHA512(Header.sig || Record[0].body))
Record[1].sig = Ed25519(SHA512(Record[0].sig || Record[1].body))
...
```

Breaking any link in the chain invalidates all subsequent records, making tampering detectable.

## Compression

By default, the ledger is compressed with zstd when written to the output directory. `warden inspect` handles both compressed (`.zst`) and uncompressed ledgers transparently.

## Full Specification

See [Ledger Specification](../design/ledger-spec.md) for the complete binary format.
