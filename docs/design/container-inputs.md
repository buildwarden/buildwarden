---
feature: container-inputs
status: draft
round: merge
drivers: [container, qemu, vz, hyperv]
seams: []
surfaces: [output]
security_boundary: none
alters_relationships: false
native_validation:
  - {host: macos-vz, blocking: false, reason: "vz never translates and does not call RecordInputs (dropped in Implement), so its code is unchanged"}
  - {host: qemu-any, blocking: false, reason: "qemu Linux guests translate a Dockerfile; the shared helper is unit-tested and the qemu call is one line. Validated end to end 2026-10-30 (backlog container-inputs-linux-vm-guests, done)"}
  - {host: windows-hyperv, blocking: false, reason: "Windows guests never translate and Hyper-V builds no Linux guests yet, so the hyperv call records nothing today. A Windows build writing no inputs/ was checked 2026-10-30"}
tier2_gaps: []
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
- a3, **assumed** (merge round, open for sign-off): the convention this
  feature settles, that a driver records an input it alters at staging,
  before the build starts, through the shared `driver.RecordInputs`, is
  proposed as an amendment to ORCH-02 in a follow-up process item after this
  merges, not in this PR (FLOW-14: `docs/development/` changes only at human
  sign-off). Options: a follow-up process item (recommended); fold the
  amendment into this PR at sign-off; no rule, leaving the convention to this
  design. See Consolidation.

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
  Hyper-V does not build Linux guests yet (`buildSeed` refuses them before
  staging), so today the call records nothing; it is there so the Linux-guest
  path, when it lands, records inputs like qemu without another change.
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
fixed-name rule is tested against the common original name. Tier 1: the
container-driver integration test (`test-integration.sh`) checks `inputs/`
after Test 2. Since native-loop-followups it runs in CI (the `integration`
workflow, on PRs that reach the `driver/` root or `test-integration.sh`), so
this feature's PR runs it; it passed on the branch in run 40961607311 (29
passed, backlog `container-inputs-integration-test`, done). Tier 2: native
hosts are tracked, not blocking (see front-matter); the qemu Linux-guest
build was validated on `qemu-any` (backlog `container-inputs-linux-vm-guests`,
done), and no gap remains open.

## Implementation plan

All twelve tasks are done. Tasks 1 to 6 shipped in the first round (p3
Implement, p5 Guide) with the submitted file recorded as `inputs/Dockerfile`;
tasks 7 to 12, the a2 rework (FLOW-06), shipped in the second round
(`ca2f897` to `ddd11fe`). The merge round adds none.

1. Done. Add `translates(req)` and `RecordInputs` to `driver/session.go`,
   `translates` shared with `StageBuildScript` (ORCH-02, GLOBAL-01).
2. Done. Tier 0 unit tests (TEST-02), table-driven over the precedence cases
   (translated, `--script`, context script, Windows guest):
   `TestRecordInputsOnlyWhenTranslated` and the `StageBuildScript` cases it
   agrees with (TEST-01, TEST-03).
3. Done. Container driver: `setup` calls `RecordInputs` once the output
   directory exists, and `collectOutput` no longer writes the top-level
   `Dockerfile.submitted` and `build.sh` (ORCH-02, ORCH-01, FLOW-15; a1).
4. Done. qemu and hyperv call `RecordInputs` right after `StageBuildScript`,
   failing the build before it starts if recording fails (ORCH-01, ORCH-02).
   vz does not call it (Cross-driver impact).
5. Done. Output reference `docs/book/src/guide/output.md` and the
   `StartBuild` pseudocode in `docs/book/src/concepts/overview.md` (UX-01,
   FLOW-15). Its `inputs/Dockerfile` wording is reworked in task 10.
6. Done. Layout in `README.md` (UX-01); the Guide phase updated the narrative
   in `docs/book/src/getting-started/first-build.md` (UX-01). Both are
   reworked in task 10.
7. Done. Rework `driver/session.go`: `RecordInputs` copies `req.Containerfile` to
   `inputs/Containerfile` (today `filepath.Join(dir, "Dockerfile")`), and its
   doc comment names `<output>/inputs/Containerfile` (ORCH-02, a2).
8. Done. Rework the tier 0 test `TestRecordInputsOnlyWhenTranslated` in
   `driver/session_test.go` (TEST-01, TEST-02): rename the submitted fixture
   from `Containerfile` to `Dockerfile`, so the test proves the fixed name
   rather than an unchanged base name; read `inputs/Containerfile` and fix
   the failure message; in the translated case assert `inputs/Dockerfile`
   does not exist; the run-as-given cases keep asserting no `inputs/` (ORCH-02).
9. Done. Tier 1 check in `test-integration.sh` (TEST-02, container driver, where a
   runtime is available): after Test 2's `examples/Dockerfile.simple` build,
   assert `inputs/Containerfile` matches the submitted file byte for byte,
   `inputs/build.sh` exists, and `inputs/Dockerfile` does not, with the
   script's `green`/`red` and PASS/FAIL counters (ORCH-02, TEST-03). The qemu
   and hyperv end-to-end gap was tracked in `tier2_gaps`, and has since closed
   (Validation plan).
10. Done. Rework the reference docs in the same change (UX-01):
    `docs/book/src/guide/output.md` (the `inputs/` layout block, and the
    `### inputs/Dockerfile` entry retitled `### inputs/Containerfile`, saying
    the submitted file is kept under that fixed name whatever it was called);
    the `inputs/` lines of `README.md`'s layout; the `inputs/` lines of
    `docs/book/src/getting-started/first-build.md`'s layout. Prose that means
    the file the user wrote keeps "your Dockerfile" (Design). `overview.md`
    names no file and needs no change (FLOW-15).
11. Done. Sweep for the old name (ORCH-02, UX-01): `grep -rn
    'inputs/Dockerfile\|"inputs", "Dockerfile"\|(dir, "Dockerfile")' driver cmd
    docs/book README.md test-integration.sh` returns only the absence
    assertions added in tasks 8 and 9; this design keeps the name only as the
    rejected option.
12. Done. Gate before pushing (TEST-01, FLOW-17): `make test`, and `make lint` for
    linux (only untagged files in `driver/` change); push
    `feat/container-inputs`.

Merge round (m2): the consolidation pass adds no tasks, and no rework of
implemented code is owed (FLOW-06). What `main` brought in (`c9940d6`) reaches
this feature only in `test-integration.sh` (the `dist/warden` path and the
`extract_output_dir` fix, which Test 2's `inputs/` checks already use) and in
`overview.md` (the unrecorded `/ca.crl` route, apart from `StartBuild`'s
`inputs/` line), so neither the code nor the reference docs owe a change
(UX-01, FLOW-15). The sweep in task 11 still returns only the absence
assertions. Tasks 1 to 12 are done; the merged head re-runs tier 1 in the PR's
`integration` workflow and is re-reviewed in m4 (FLOW-03, FLOW-10, TEST-02).
The ORCH-02 amendment (a3) is a sign-off decision, not a task here (FLOW-14).

## Consolidation (merge round)

Rebased onto `main` at `cd42fc7` (Jeff, 2026-10-29); `main` merged at
`cc6119e` in `c9940d6` (m0), without conflicts.

- **Landed since the fork:** #64 native-host-loop, #65
  native-loop-followups, #67 relay-gzip-framing, #69 native-leftovers, #66
  workflow-bootstrap-archive, #70 relay-leaf-crl and #71 its archive. None
  touches `driver/session.go`, the container, qemu or vz drivers, or the
  output layout. The overlaps:
  - native-host-loop moved this design's native gaps to `<capability>-any`
    host names (`hosts.yaml`): the qemu Linux-guest gap is on `qemu-any`, not
    `macos-qemu`, so `native_validation` now names `qemu-any`. Both backlog
    entries this feature owns are done (Validation plan), so `tier2_gaps` is
    empty. The `container-inputs-linux-vm-guests` entry's `what` still says
    `inputs/Dockerfile`; its evidence already notes the branch writes
    `inputs/Containerfile`, and the entry is closed, so it is left as is.
  - native-loop-followups made tier 1 a CI workflow and fixed
    `test-integration.sh` (`dist/warden`, the `build SUCCEEDED (output: ...)`
    line). This branch's `inputs/` checks sit in Test 2 and use the
    `OUTPUT_DIR` that the fixed `extract_output_dir` sets, so the merge needed
    no change; the earlier green run applied those fixes as a patch, and the
    PR now runs the workflow on the merged script.
  - #69 changed `driver/hyperv/buildassets.go` and its test (the relay boot
    VHDX cache), apart from the `RecordInputs` call in `driver_windows.go`.
  - relay-leaf-crl added `/ca.crl`, served unrecorded and written to no
    output file, so ORCH-02's reserved output names are unchanged.
  - FLOW-08 and the workflow tooling (process items, holds, `native_budget`,
    PRs opened without confirmation) change how m6 Publish opens the PR, not
    this design.
- **Terminology:** `inputs/`, `Containerfile` (the format, a2) and "your
  Dockerfile" (the file the user wrote) match ORCH-02, `BuildRequest` and the
  reference docs. No design that landed uses another term for them.
- **Abstractions:** one path. `driver.RecordInputs` and `StageBuildScript`
  share `translates`, and container, qemu and hyperv call the one helper. The
  container driver's own top-level copy is gone (task 3); no other code
  copies the Dockerfile or `build.sh` into the output.
- **Seams:** none declared, and no versioned seam changed on `main` overlaps.
- **Stale items fixed here:** the plan called tasks 7 to 12 open; they are
  done. The vz `native_validation` reason said the shared call is a no-op
  there; vz has not called `RecordInputs` since Implement. The hyperv call is
  now the one that records nothing (no Linux guests yet), which Cross-driver
  impact says, with the reason it stays.
- **FLOW-15 sweep:** `make deadcode` (linux, darwin, windows) reports nothing
  this feature added; `RecordInputs` and `translates` are reachable on all
  three. Unreachable on all three, as on `main`: `Collector.BytesWritten`,
  `Relay.CAFingerprint` and `Relay.Ledger` (exported library API), and
  `driver/vz` helpers (cgo-only, checked on macOS). Duplication: none, as
  above. `overview.md` already has `inputs/` in the `StartBuild` pseudocode
  ("not vz"), and nothing that landed changes that flow.
- **Rule proposed for sign-off (FLOW-10, FLOW-14; a3):** amend ORCH-02 with
  the convention this feature settled, for example: "A driver that alters an
  input records it when it stages the input, before the build starts, through
  the shared `driver.RecordInputs`, so a failed build keeps its inputs and a
  failure to record fails the build before anything runs." Not made here.
- **For the archive:** the `docs/development/README.md` follow-up
  "Container inputs under `inputs/`" is this feature and is removed when it is
  archived.

## Rollback notes

## Archive summary
