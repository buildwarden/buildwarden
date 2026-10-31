# Specifications

The BuildWarden ledger format, versioned and long-lived (RELAY-05). The relay
is a reference implementation of these documents; a third-party reader or
writer needs nothing else.

## Layout

```
specs/
  README.md                      this index
  build-ledger/v<N>.md           ledger wire format, one per major version (prose only)
  metadata-<name>/v<N>.md        a record metadata schema's document
  metadata-<name>/v<N>.schema.json   ... and its machine schema, beside it
  environment-<type>/v<N>.md     a signed environment payload's document
  environment-<type>/v<N>.schema.json  ... and its machine schema, beside it
```

A schema and its document share a major version and change together; a new
major version adds `v<N+1>.schema.json` and `v<N+1>.md` beside the old pair,
which stays for ledgers already written.

## Documents

- [Ledger wire format, v1](build-ledger/v1.md)

| `$id` | Document | Machine schema |
|-------|----------|----------------|
| `https://github.com/buildwarden/buildwarden/schemas/v1/http-open.json` | [metadata-http-open v1](metadata-http-open/v1.md) | [v1.schema.json](https://github.com/buildwarden/buildwarden/blob/main/specs/metadata-http-open/v1.schema.json) |
| `https://github.com/buildwarden/buildwarden/schemas/v1/http-headers.json` | [metadata-http-headers v1](metadata-http-headers/v1.md) | [v1.schema.json](https://github.com/buildwarden/buildwarden/blob/main/specs/metadata-http-headers/v1.schema.json) |
| `https://github.com/buildwarden/buildwarden/schemas/v1/http-body.json` | [metadata-http-body v1](metadata-http-body/v1.md) | [v1.schema.json](https://github.com/buildwarden/buildwarden/blob/main/specs/metadata-http-body/v1.schema.json) |
| `https://github.com/buildwarden/buildwarden/schemas/v1/artifact.json` | [metadata-artifact v1](metadata-artifact/v1.md) | [v1.schema.json](https://github.com/buildwarden/buildwarden/blob/main/specs/metadata-artifact/v1.schema.json) |
| `https://github.com/buildwarden/buildwarden/schemas/v1/redacted.json` | [metadata-redacted v1](metadata-redacted/v1.md) | [v1.schema.json](https://github.com/buildwarden/buildwarden/blob/main/specs/metadata-redacted/v1.schema.json) |
| `https://github.com/buildwarden/buildwarden/schemas/v1/environment.json` | [metadata-environment v1](metadata-environment/v1.md) | [v1.schema.json](https://github.com/buildwarden/buildwarden/blob/main/specs/metadata-environment/v1.schema.json) |
| `https://github.com/buildwarden/buildwarden/schemas/v1/environment-container.json` | [metadata-environment-container v1](metadata-environment-container/v1.md) (legacy) | [v1.schema.json](https://github.com/buildwarden/buildwarden/blob/main/specs/metadata-environment-container/v1.schema.json) |
| `https://github.com/buildwarden/buildwarden/schemas/v1/environment-oci-image.json` | [environment-oci-image v1](environment-oci-image/v1.md) | [v1.schema.json](https://github.com/buildwarden/buildwarden/blob/main/specs/environment-oci-image/v1.schema.json) |
| `https://github.com/buildwarden/buildwarden/schemas/v1/environment-vm-disk-chain.json` | [environment-vm-disk-chain v1](environment-vm-disk-chain/v1.md) | [v1.schema.json](https://github.com/buildwarden/buildwarden/blob/main/specs/environment-vm-disk-chain/v1.schema.json) |

## `$id` policy

A schema's `$id` is a stable name, not a location. Ledgers list these
strings in their header's `schemas` array, so they never change:

- The `$id`s above, of the form
  `https://github.com/buildwarden/buildwarden/schemas/v1/<name>.json`, date
  from before this layout and are kept for good. They do not resolve to a file;
  use the table above to find one.
- A schema added after this layout takes an `$id` that mirrors its path:
  `https://github.com/buildwarden/buildwarden/specs/<dir>/v<N>.schema.json`.
