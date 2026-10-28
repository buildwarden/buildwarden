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

ORCH-02 requires that when the orchestrator alters a build input, both the
original and the edited form are recorded under `inputs/` in the output
directory. Every driver now translates a Dockerfile into `build.sh` the same way
(`unified-bootstrap`), but only the container driver keeps the pair, and it
writes them at the top of the output directory as `Dockerfile.submitted` and
`build.sh`. The qemu and hyperv drivers translate a Dockerfile for Linux guests
and keep neither file, so the build's record does not show what the user
submitted next to what ran.

## Prior art

- ORCH-02 (`docs/development/orchestrator.md`) names `inputs/` and the
  reserved names it must not collide with.
- `unified-bootstrap` (archived) made one translation path for every driver
  (`driver.StageBuildScript`, `script.Translate`) and retired
  `Dockerfile.actual`; this design reuses its precedence rules unchanged.
- `ledger-environment-types` left `environment.json` out of ORCH-02's reserved
  list on purpose; nothing here changes that list.

## Decisions

- **One shared helper, `driver.RecordInputs`**, called by every driver after
  `FinalizeOutput`, instead of per-driver copies (FLOW-15: one path for one
  job). Rejected: keeping the container driver's own code and copying it into
  qemu and hyperv.
- **Recorded only when the input was altered**: a Dockerfile translated to a
  script. A `--script` or a context `build.sh` / `build.ps1` runs as given, so
  there is nothing to record (ORCH-02 covers alterations). Rejected: always
  copying the script that ran, which duplicates an input the user already has
  byte for byte.
- **The same precedence decides both**: a `translates(req)` predicate shared by
  `StageBuildScript` and `RecordInputs`, so the recording cannot drift from what
  was staged.
- **Fixed names**: `inputs/Dockerfile` (the submitted file, whatever it was
  called) and `inputs/build.sh` (the translation).

## Questions

### Blocking

None.

### Assumed

- Proceeding with removing the top-level `Dockerfile.submitted` and `build.sh`
  outright, without a compatibility copy (pre-1.0 output layout). Confirm.
- Proceeding with the fixed name `inputs/Dockerfile` even when the user's file
  is named `Containerfile` or anything else. Confirm.

## Design

```
RecordInputs(outputDir, req, script):
    if not translates(req): return          # --script, context script, Windows, vz
    mkdir outputDir/inputs
    copy req.Containerfile -> inputs/Dockerfile   (unchanged)
    copy script            -> inputs/build.sh     (what the guest fetched)

translates(req) = req.Script == "" and guest is not Windows and req.Containerfile != ""
```

The container driver always translates, so its builds always get both files.

## Cross-driver impact

- **container**: `collectOutput` calls `RecordInputs` in place of writing
  `Dockerfile.submitted` and `build.sh` at the top level.
- **qemu**: calls `RecordInputs` after `FinalizeOutput`; Linux guests built from
  a Dockerfile gain `inputs/`.
- **hyperv**: the same; Windows guests never translate, so no change for them.
- **vz**: calls it with its request copy whose `Containerfile` is cleared, so it
  records nothing; the call keeps every driver's `StartBuild` the same shape
  (ORCH-01).

## Validation plan

Tier 0: unit tests for `RecordInputs` (translated, `--script`, context script,
Windows guest) and that `StageBuildScript` and `RecordInputs` agree. Tier 1/2:
the container-driver integration test (`test-integration.sh`) where a runtime is
available. Native hosts are tracked, not blocking (see front-matter).

## Implementation plan

1. Add `translates(req)` and `RecordInputs` to `driver/session.go`, used by
   `StageBuildScript` (ORCH-02, GLOBAL-01).
2. Unit tests for both, table-driven over the precedence cases (TEST-01).
3. Container driver: replace the top-level copies in `collectOutput` with
   `RecordInputs` (ORCH-02, FLOW-15).
4. qemu, hyperv and vz: call `RecordInputs` after `FinalizeOutput` (ORCH-01).
5. Update the output reference (`docs/book/src/guide/output.md`) and the
   `StartBuild` pseudocode in `docs/book/src/concepts/overview.md` (UX-01,
   FLOW-15).
6. Update the narrative layouts in `README.md` and
   `getting-started/first-build.md` (UX-01).

## Consolidation (merge round)

## Rollback notes

## Archive summary
