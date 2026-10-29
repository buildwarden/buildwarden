---
feature: container-inputs
status: draft
round: parallel
drivers: [container, qemu, vz, hyperv]
seams: []
surfaces: [output]
security_boundary: none
alters_relationships: false
native_validation:
  - {host: macos-vz, blocking: false, reason: "vz never translates; the shared call is a no-op there, covered by unit tests"}
  - {host: macos-qemu, blocking: false, reason: "qemu Linux guests translate a Dockerfile; the shared helper is unit-tested and the qemu call is one line"}
  - {host: windows-hyperv, blocking: false, reason: "Windows guests never translate; Linux guests on Hyper-V share the helper"}
tier2_gaps:
  - "qemu and hyperv Linux-guest builds with a Dockerfile: inputs/ checked by unit test only, not end to end"
rules: [ORCH-02, ORCH-01, UX-01, FLOW-15, GLOBAL-01, TEST-01, TEST-02, TEST-03]
concepts: [inputs, output-layout, dockerfile-translation, provenance]
related: [unified-bootstrap.md]
---

# Record translated inputs under `inputs/`

## Problem

Since `unified-bootstrap`, a Dockerfile build no longer runs the Dockerfile:
the host translates it into `build.sh`, rewriting each `COPY` into explicit
`warden-io fetch` calls, and the guest runs that script. The files those calls
fetch come through the relay, so the ledger already records every one of them.
What the ledger does not hold is the pair that defines the build: the
Dockerfile the user submitted and the script it became. Those two are the
build's inputs now, and ORCH-02 says inputs the orchestrator produces or alters
are recorded under `inputs/`.

Today only the container driver keeps the pair, at the top of the output
directory as `Dockerfile.submitted` and `build.sh`. The qemu and hyperv drivers
translate a Dockerfile for Linux guests and keep neither, so their record does
not show what the user submitted next to what ran.

## Prior art

- ORCH-02 (`docs/development/orchestrator.md`) names `inputs/` and the
  reserved names it must not collide with.
- `unified-bootstrap` (archived) made one translation path for every driver
  (`driver.StageBuildScript`, `script.Translate`) and retired
  `Dockerfile.actual`; this design reuses its precedence rules unchanged.
- `ledger-environment-types` left `environment.json` out of ORCH-02's reserved
  list on purpose; nothing here changes that list.

## Decisions

- **One shared helper, `driver.RecordInputs`**, called by every driver that
  translates, straight after staging, instead of per-driver copies (FLOW-15:
  one path for one job). Rejected: keeping the container driver's own code and
  copying it into qemu and hyperv.
- **`inputs/` holds the Dockerfile and its translation, not the `COPY`
  sources**: the translation turns every `COPY` into a `warden-io fetch`, and
  the relay records each fetched file in the ledger, so copying them again
  under `inputs/` would duplicate the ledger. Rejected: snapshotting the build
  context.
- **Recorded when a Dockerfile was translated**: that is when the orchestrator
  produces an input the user did not write. A `--script` or a context
  `build.sh` / `build.ps1` runs as given, and the guest fetches it through the
  relay like any other input. Rejected: always copying the script that ran,
  which duplicates a file the user already has byte for byte.
- **The same precedence decides both**: a `translates(req)` predicate shared by
  `StageBuildScript` and `RecordInputs`, so the recording cannot drift from what
  was staged.
- **vz does not call it**: it never translates (see Cross-driver impact).
- **Recorded at staging, before the build starts**, in every driver: a failed
  build keeps its inputs, the Containerfile is copied moments after it was
  translated rather than at the end of the build, and a failure to record
  fails the build before anything runs (one policy for all drivers, ORCH-01).
  Changed in Review: the first implementation recorded after the build, which
  dropped inputs on failure and handled errors differently per driver.
- **Fixed names, the open-standard one for the submitted file**:
  `inputs/Containerfile` holds the submitted file whatever it was called
  (`Dockerfile`, `Containerfile`, or any file passed as the build path or set
  as the `dockerfile` config key), and `inputs/build.sh` holds the
  translation. `Containerfile` is the tool-neutral name for the format (the
  open-standard name, in Jeff's answer), and BuildWarden already uses it
  internally (`BuildRequest.Containerfile`, `script.Translate`), so the record
  names the format rather than one vendor's tool. A fixed name means a
  reader, or a verifier script, finds the submitted file without knowing what
  the user called it. Changed after Publish (Jeff,
  answering a2): the first implementation used `inputs/Dockerfile`. Rejected:
  `inputs/Dockerfile` (one vendor's name for an open format); keeping the
  submitted base name (the record's layout would vary per build, and a file
  such as `ci.dockerfile` could land under any name).

## Questions

### Blocking

None.

### Assumed

- a1, **confirmed** (Jeff): remove the top-level `Dockerfile.submitted` and
  `build.sh` outright, without a compatibility copy (pre-1.0 output layout).
- a2, **answered** (Jeff rejected the assumed `inputs/Dockerfile`): always
  record the submitted file as `inputs/Containerfile`, the open-standard name,
  whatever its original name. Applied throughout this revision.

## Design

```
RecordInputs(outputDir, req, script):     # container, qemu, hyperv
    if not translates(req): return          # --script, context script, Windows, vz
    mkdir outputDir/inputs
    copy req.Containerfile -> inputs/Containerfile  (unchanged bytes, any original name)
    copy script            -> inputs/build.sh       (what the guest fetched)

translates(req) = req.Script == "" and guest is not Windows and req.Containerfile != ""
```

The container driver always translates, so its builds always get both files.

The resulting layout:

```
<output>/
└── inputs/
    ├── Containerfile   # the file submitted (Dockerfile, Containerfile, ...), unchanged
    └── build.sh        # its translation: what ran
```

Only the recorded name changes with a2; the user still submits a `Dockerfile`
or a `Containerfile` as today (`cmd/warden/config.go` looks for both), and the
reference docs keep saying "your Dockerfile" where they mean the file the user
wrote, naming `inputs/Containerfile` only for the recorded copy.

## Cross-driver impact

- **container**: `setup` calls `RecordInputs` once the output directory
  exists, and `collectOutput` no longer writes `Dockerfile.submitted` and
  `build.sh` at the top level.
- **qemu**: calls `RecordInputs` right after `StageBuildScript`; Linux guests
  built from a Dockerfile gain `inputs/`.
- **hyperv**: the same; Windows guests never translate, so no change for them.
- **vz**: unchanged. macOS guests run a shell script and never translate (the
  driver clears `Containerfile` before staging), so there is nothing to record.
  An earlier draft called `RecordInputs` there anyway for symmetry; it was
  dropped in Implement because the no-op call pushed `StartBuild` over the
  gocyclo limit.

## Validation plan

Tier 0: unit tests for `RecordInputs` (translated, `--script`, context script,
Windows guest) and that `StageBuildScript` and `RecordInputs` agree. The
translated case submits a file named `Dockerfile` and asserts it lands as
`inputs/Containerfile` and that no `inputs/Dockerfile` exists, so the
fixed-name rule is tested against the common original name. Tier 1/2:
the container-driver integration test (`test-integration.sh`) where a runtime is
available. Native hosts are tracked, not blocking (see front-matter).

## Implementation plan

1. Add `translates(req)` and `RecordInputs` to `driver/session.go`, used by
   `StageBuildScript` (ORCH-02, GLOBAL-01).
2. Unit tests for both, table-driven over the precedence cases (TEST-01).
3. Container driver: replace the top-level copies in `collectOutput` with
   `RecordInputs` (ORCH-02, FLOW-15).
4. qemu and hyperv: call `RecordInputs` right after `StageBuildScript`
   (ORCH-01).
5. Update the output reference (`docs/book/src/guide/output.md`) and the
   `StartBuild` pseudocode in `docs/book/src/concepts/overview.md` (UX-01,
   FLOW-15).
6. Update the layout in `README.md` (UX-01). The Guide phase updates the
   narrative in `getting-started/first-build.md`.

Rework for a2 (FLOW-06; steps 1 to 6 shipped with `inputs/Dockerfile`):

7. `driver/session.go`: `RecordInputs` writes `inputs/Containerfile`, and its
   doc comment says so (ORCH-02).
8. `driver/session_test.go`: `TestRecordInputsOnlyWhenTranslated` submits a
   file named `Dockerfile`, reads `inputs/Containerfile`, and asserts
   `inputs/Dockerfile` is absent (TEST-01).
9. Reference docs (UX-01): `docs/book/src/guide/output.md` (the layout block
   and the `### inputs/Dockerfile` entry, retitled `### inputs/Containerfile`),
   `README.md`'s layout, and `docs/book/src/getting-started/first-build.md`'s
   layout. `overview.md` names no file and needs no change (FLOW-15).
10. Check no `inputs/Dockerfile` remains in code, tests or reference docs:
    `grep -rn 'inputs/Dockerfile\|"inputs", "Dockerfile"' driver cmd docs/book
    README.md test-integration.sh` finds nothing (this design keeps the name
    only as the rejected option).

## Consolidation (merge round)

## Rollback notes

## Archive summary
