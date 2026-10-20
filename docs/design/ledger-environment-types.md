---
feature: ledger-environment-types
status: draft
round: parallel
drivers: [container, qemu, vz, hyperv]
seams: [ledger-format, relay-claim, collector]
surfaces: [cli, output]
security_boundary: env-relay
native_validation:
  - {host: windows-hyperv, blocking: true, reason: "hyperv descriptor (eval VHD / --image chain, seed hashes) only runs on Hyper-V"}
  - {host: macos-vz, blocking: true, reason: "vz descriptor (prepared disk.img, aux-storage, platform files) only runs on macOS/arm64"}
  - {host: qemu-any, blocking: true, reason: "qemu descriptor (cloud image / prepared Windows qcow2 / kernel+initrd) needs a qemu host"}
  - {host: container-ci, blocking: false, reason: "container keeps its payload; only the header label and the new environment.json output change"}
tier2_gaps: []
rules: [GLOBAL-05, GLOBAL-06, GLOBAL-08, RELAY-01, RELAY-05, RELAY-07, ORCH-01, ORCH-02, ORCH-04, UX-01, FLOW-07]
concepts: [ledger, environment-record, provenance, image-prepare, layer-hash-cache, claim, collector]
related: [vm-driver-release-plan.md, hyperv-driver-plan.md]
---

# Typed ledger environment records

## Problem

Every ledger's header said `environment: {type: "container"}`, whatever the
driver, and only the container driver sent an environment record (its OCI
manifest). A qemu, vz or Hyper-V ledger therefore claimed to be a container
build and said nothing about the disk it booted. Hyper-V Phase 5a adds a
"prepare once" derived Windows base (Microsoft eval VHD -> warden-prepared
differencing layer -> per-build child), and the provenance from Microsoft's
pristine image to each build has to be auditable from the signed ledger.

## Prior art

- `ledger-spec.md` (the v1 wire format): only the record type, `prev_sig`,
  payload size and hash block are signed. Header metadata, the schema index and
  CBOR record metadata are unsigned and may be redacted. This design keeps that
  format byte-for-byte; it adds content conventions on top of it (RELAY-05).
- The container driver's environment record (open + close, payload = OCI
  manifest, digest in metadata) is reused unchanged as `oci-image/v1`.
- `relaycfg.Config.Environment` (the claim, GLOBAL-08) already carries the
  payload; the relay already writes it as the first ledger record.
- `hyperv/evalimage.go` already hashes and pins the eval VHD on fetch, and
  qemu writes a `.sha256` sidecar on download: both become feeders of the
  shared layer-hash cache below.

## Decisions

1. **Environment types are specified, not free-form.** Each type has a section
   in `ledger-spec.md` and a JSON schema (`schemas/v1/environment-<type>.json`)
   describing what the payload must contain. Adding a type is a new section and
   schema; the wire format does not change. *Rejected:* a per-driver free-form
   map in the header (unsigned, unverifiable).
2. **The payload names its own type inside the signed bytes.** Because the
   schema index and metadata are unsigned, a verifier must not learn the type
   from them. `vm-disk-chain/v1` payloads are JSON objects with a top-level
   `"type": "vm-disk-chain/v1"`. `oci-image/v1` is the one exception, kept for
   compatibility: its payload is the runtime's OCI/Docker manifest, identified
   by its own `schemaVersion` / `mediaType`. *Rejected:* wrapping the manifest
   in an envelope (changes every container ledger's environment hash for no
   verification gain).
3. **The relay derives the header label from the payload.** The header's
   `environment` becomes `{type: <descriptor type>}`. The relay sniffs the type
   from the payload (`ledger.EnvironmentType`) and refuses a claim whose declared
   `Environment.Type` disagrees, so the label can only ever echo the signed
   bytes. The label stays documented as untrusted (it is unsigned). A ledger
   with no environment record has no `environment` key. *Rejected:* trusting the
   claim's type field alone (could disagree with the signed payload).
4. **The payload lands in the output directory.** The relay `PUT`s the exact
   bytes it hashed to the collector's new `/v1/environment` endpoint, which
   writes `<output>/environment.json` (RELAY-07: the collector is the only output
   path, and a third-party orchestrator using `cmd/collector` gets it too).
   Before this, the payload was hashed and discarded, so nothing could verify
   it. *Rejected:* the orchestrator writing the file itself (third-party
   orchestrators would each have to reimplement it).
5. **Verification is two-step.** (a) Hash `environment.json` with the header's
   hash algorithms and compare to the signed environment record's hash block;
   (b) for `vm-disk-chain/v1`, optionally re-hash each layer's file at its
   recorded `path` and compare `sha256`/`size`. Disks are never embedded in
   the ledger. (b) is opt-in (`warden inspect --verify-disks`) because layers
   are tens of GB.
6. **The orchestrator caches layer hashes.** A shared `driver.LayerCache`
   (one JSON file in each driver's image cache directory) stores, per absolute
   path, `{sha256, size, mtime, parent_sha256, source}`. Fetch/prepare commands
   record the hash they already computed plus provenance; a build reuses the
   entry while size, mtime and the parent's hash are unchanged, and otherwise
   re-hashes and drops the provenance to `local` (a modified file is no longer
   the fetched one). `warden image verify --driver <d>` re-hashes every entry
   fully and reports drift. The descriptor is reassembled per build from cached
   layer hashes plus freshly hashed seed files.
7. **The relay signs what the orchestrator asserts.** The descriptor is built
   host-side, before the guest boots, from files the guest never writes. The
   relay does not and cannot check the disk; it binds the orchestrator's
   statement into the signed chain. Stated in the spec.

## Questions

### Blocking

None.

### Assumed

- **Layer-hash cache location.** Proceeding with one shared `driver.LayerCache`
  file per driver cache directory (`<hyperv cache>/images/layers.json`,
  `<warden cache>/images/layers.json` for qemu, `<vz cache>/layers.json`)
  instead of adding hash fields to each driver's own image manifest (hyperv
  `index.json`, qemu `.sha256` sidecars, vz none). The fetch/prepare paths feed
  it. Options: (a) shared cache file per driver dir [recommended: one code path,
  ORCH-01]; (b) per-driver manifest fields (three implementations). Confirm.
- **Paths in the signed descriptor.** Proceeding with each layer carrying its
  host `path` so `inspect --verify-disks` can find it. This puts host-local
  paths (e.g. a user profile directory) into a signed, shareable ledger.
  Options: (a) include paths [recommended, needed for step-5b without extra
  flags]; (b) omit, and make the verifier pass a disk directory. Confirm.
- **Seed contents.** Proceeding with seed = every file in the per-build guest
  seed directory (warden-io, warden-run.ps1, unattend.xml, cloud-init files)
  plus the staged build script, hashed each build. Unattend files contain a
  per-build random admin password; only its SHA-256 is recorded (20 random
  alphanumerics, not brute-forceable). Confirm.
- **ORCH-02 reserved names.** Proceeding without editing
  `docs/development/orchestrator.md`; `environment.json` should be added to
  ORCH-02's reserved output names in a reviewed standards change. Confirm.
- **Container runtimes without a manifest.** Proceeding with no environment
  record (and a warning) when the runtime returns only `image inspect` JSON or
  nothing: that payload names no type, so the relay would refuse it. Before,
  such builds recorded the inspect JSON under the container label. Options:
  (a) no record [recommended]; (b) define a `container-inspect/v1` type.
  Confirm.
- **Relay VM image.** Proceeding without describing the relay's own boot image
  (initrd / UKI VHDX). It is the measuring instrument, not the build
  environment; a later `relay` block can add it. Confirm.

## Design

### Header

```cbor
"environment": { "type": "vm-disk-chain/v1" }
```

Unsigned label, copied from the payload. Absent when there is no environment
record. Ledgers written before this change carry `{"type": "container"}`, which
readers treat as `oci-image/v1`.

### Environment record

Unchanged framing: the first records after the header are an `open` and a
`close` on the environment schema (now `schemas/v1/environment.json`), the
close carrying `-len(payload)` and the payload's hash block. Record metadata is
`{type, ...}` with type-specific informational fields (`reference`, `digest`,
`mediaType` for `oci-image/v1`). The old `environment-container.json` stays
published for ledgers that reference it.

### `oci-image/v1`

Payload: the manifest bytes the container runtime returned. Metadata
`digest` is the registry digest of the pulled reference; it equals
`sha256(payload)` only when the runtime returned the registry's exact bytes
(docker re-indents, and the digest may name an index), so inspect reports a
mismatch as informational, not as a failure.

### `vm-disk-chain/v1`

```json
{
  "type": "vm-disk-chain/v1",
  "driver": "hyperv",
  "guest_os": "windows",
  "arch": "amd64",
  "generation": 1,
  "layers": [
    {"role": "disk", "name": "windows-server-2025.vhd", "format": "vhd",
     "path": "C:\\...\\images\\windows-server-2025.vhd",
     "sha256": "...", "size": 1234,
     "source": {"kind": "url", "url": "https://...", "pinned": true}}
  ],
  "seed": [
    {"name": "build-script", "sha256": "...", "size": 10},
    {"name": "warden-io.exe", "sha256": "...", "size": 10}
  ]
}
```

- `layers` is ordered. Entries with `role: disk` form the disk chain base-first
  (each later disk is a child of the previous); the per-build writable child is
  created empty from the last disk and is not a layer. Other roles are files the
  VM boots with: `kernel`, `initrd`, `nvram` (UEFI vars / aux storage),
  `platform` (vz hardware model, machine id).
- `source.kind` is `url` (downloaded; `pinned` when verified against an
  expected hash), `local` (supplied by the user or modified since fetch), or
  `prepared` (produced by warden: `prepared_by: "warden <version>"` and
  `inputs`, each `{name, sha256, size?, url?}`). **Extension point:** the Phase
  5a Hyper-V prepared differencing layer is a `role: disk, source.kind:
  prepared` entry after the eval VHD, whose `inputs` list the preparation
  script and payload hashes.
- `seed` is sorted by name.

The Go types live in `ledger/environment.go` so third-party orchestrators and
verifiers share them.

### Collector

`PUT /v1/environment` writes `<dir>/environment.json` (1 MiB cap, fixed
name). `environment.json` is a new reserved output name; adding it to ORCH-02's
list is a standards change left for sign-off (see Assumed).

### Inspect

The header line shows the decoded type. A new **ENVIRONMENT** block (all
verbosities, and `environment` in `--json`) prints the descriptor summary and
verification: `payload: verified | MISMATCH | missing`, per-layer
`ok | MISMATCH | missing | skipped` under `--verify-disks`, and for
`oci-image/v1` the digest comparison. A payload mismatch fails inspect (exit 1);
a disk mismatch under `--verify-disks` also fails.

### `warden image verify`

`warden image verify [--driver d]` re-hashes every layer-cache entry for the
driver(s), updates entries whose file changed (dropping provenance), drops
entries whose file is gone, and exits non-zero on any drift.

## Cross-driver impact

- **container:** manifest payload unchanged; metadata gains `type`;
  `Environment.Type = oci-image/v1`; header label becomes `oci-image/v1`; gains
  `environment.json`. A runtime that yields no manifest now records no
  environment (see Assumed).
- **qemu:** describes the cloud image (source URL from the layer cache, seeded
  at download) or prepared Windows qcow2 plus its UEFI vars, or kernel+initrd
  for direct boot; seed is the cloud-init / WARDEN seed dir plus build script.
- **vz:** describes `disk.img` (prepared from the IPSW URL; the IPSW hash is an
  input when known), `aux-storage` (nvram), `hardware-model`, `machine-id`
  (platform); seed is the build script (warden-io is baked into the prepared
  image).
- **hyperv:** describes the eval VHD (URL + pinned from the eval manifest via
  the layer cache) or `--image` disk, `generation`; seed is the WARDEN seed dir
  plus build script. Linux Hyper-V guests are not built today.
- **cmd/collector:** still passes no environment; its ledgers simply have none.

## Security note

Boundary: the relay (GLOBAL-06 fires on any relay change) and the host/relay
collector seam.

- What the ledger proves does not grow beyond what it signs: the relay signs the
  orchestrator's assertion about the environment. The orchestrator is trusted;
  the guest is not and never touches the descriptor (built before boot, from
  host files). The spec says so explicitly so nobody reads the record as a
  measurement.
- The header label is unsigned; readers must take the type from the payload.
  The relay refuses a declared/sniffed mismatch so honest ledgers never
  disagree.
- New relay-to-host write: `PUT /v1/environment` writes one fixed file name
  with a 1 MiB cap, authenticated like every collector endpoint. A compromised
  relay can already write the ledger; it gains no new path or size.
- Layer-cache poisoning: the cache is host-local, written only by warden; an
  attacker who can write it can already replace the disk. Cached hashes are
  bound to size+mtime, and `image verify` / `inspect --verify-disks` re-hash.
- Signed host paths and seed hashes leak host layout but no secrets (see
  Assumed).

TEST-08 should exercise: a claim whose declared type disagrees with the payload
is refused; a tampered `environment.json` fails inspect.

## Validation plan

- Tier 0: unit tests for `ledger` type sniffing/validation, hash-block
  verification, `LayerCache` (hit, mtime/size/parent invalidation, provenance
  drop, verify), each driver's descriptor builder, relay header label and
  mismatch refusal, collector endpoint, inspect human/JSON output and
  `--verify-disks`.
- Tier 1: container e2e in CI covers the unchanged container payload.
- Tier 2 (blocking, native): run one build per driver and `warden inspect
  --verify-disks <out>`: Hyper-V on the Windows desktop (eval VHD), vz on the
  macOS laptop, qemu on any qemu host (Linux cloud image and, where prepared,
  Windows qcow2).

## Implementation plan

1. `ledger/environment.go`: types, `EnvironmentType`, validation, hash block
   helper (RELAY-05).
2. `relaycfg.Environment.Type`; relay header label from payload, mismatch
   refusal, `PUT /v1/environment`; `defaultSchemas` index 5 ->
   `environment.json` (GLOBAL-06, RELAY-01, RELAY-07).
3. Collector `/v1/environment` (RELAY-07, ORCH-02).
4. `driver/layercache.go`, `driver/envdesc.go`: cache + descriptor helpers
   (ORCH-01).
5. Per driver: container sets the type; qemu, vz, hyperv build descriptors and
   pass them to `ClaimRelay`; fetch/prepare paths seed the cache (ORCH-01).
6. `warden inspect` environment block + `--verify-disks`; `warden image
   verify` (UX-01).
7. Docs: `ledger-spec.md` environment section and types, schemas, collector
   protocol, CLI reference (UX-01, RELAY-05).

## Consolidation (merge round)

To be filled in during the merge round. Hyper-V Phase 5a (prepared layer)
consumes the `prepared` source kind defined here.

## Rollback notes

None yet.

## Archive summary

To be filled in on merge.
