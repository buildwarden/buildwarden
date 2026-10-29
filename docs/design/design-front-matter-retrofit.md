---
feature: design-front-matter-retrofit
status: draft
round: parallel
drivers: []
seams: []
surfaces: []
security_boundary: none
alters_relationships: false
native_validation: []
tier2_gaps: []
rules: [FLOW-01, FLOW-02, FLOW-07, FLOW-09, FLOW-12, FLOW-14, GLOBAL-06, GLOBAL-08, TEST-02, TEST-03, LINT-02]
concepts: [design-archive, front-matter, retrofit, archive-retrieval]
related: [workflow-bootstrap.md, firecracker-driver-plan.md, hyperv-driver-plan.md, platform-compatibility.md, remote-execution-and-measured-hermeticity.md, vm-driver-release-plan.md, windows-driver-plan.md, hyperv-windows-prepare.md, ledger-environment-types.md, unified-bootstrap.md]
---

# Retrofit FLOW-02 front-matter onto the older designs

## Problem

FLOW-12 makes `docs/design/` the archive and retrieves from it by front-matter
(`concepts`, `drivers`, `seams`, `rules`). Six documents there predate FLOW-02
and carry none, so a design phase searching the front-matter cannot find them:

| Document | What it is |
|---|---|
| `firecracker-driver-plan.md` | Plan for a Firecracker driver; not started |
| `hyperv-driver-plan.md` | Hyper-V driver plan; implemented, partly superseded by the integration pass |
| `platform-compatibility.md` | Host x target x driver matrix; a living reference |
| `remote-execution-and-measured-hermeticity.md` | Vision for an RE API backend; not started |
| `vm-driver-release-plan.md` | Cross-driver roadmap; active (Phase 5a done, later phases open) |
| `windows-driver-plan.md` | Windows guest plan; Phase 1 merged, Phase 2 delivered through Hyper-V |

Three archived designs (`hyperv-windows-prepare.md`,
`ledger-environment-types.md`, `unified-bootstrap.md`) have front-matter but
predate the `alters_relationships` key, so their front-matter is incomplete
against today's template.

FLOW-12 retrofits lazily, "when a new design first relies on them", and the
README follow-up says the same. Jeff moved this item from priority 7 to 3
("retrofit early"), so the archive is made searchable before the queued
features (Firecracker, security suite, tier-1 checks) start designing against
it.

## Prior art

- FLOW-02 (the keys and the conservative-derivation rule), FLOW-12 (archive and
  retrofit), and `TEMPLATE.md` (key order and value vocabulary).
- The archived designs with front-matter set the vocabulary reused here:
  seams `relay-claim`, `collector`, `ledger-format`, `executor-initialize`,
  `relay-guest-api`, `bootstrap-model`, `image-manifest`, `windows-guest-seed`;
  hosts `windows-hyperv`, `macos-vz`, `macos-qemu`. No new seam names are
  introduced; one new host name, `linux-kvm`, is needed for Firecracker.
- `workflow-bootstrap.md` (active) already has complete front-matter and is
  untouched.
- `tools/workflow/check` (`Front.problems`) is the mechanical definition of
  "complete front-matter" that the retrofit is validated against.
- The `firecracker-driver` queue item notes that its design "retrofits
  front-matter (FLOW-12)" onto `firecracker-driver-plan.md`. This feature does
  it first; that feature amends it (see Assumed a7).

## Decisions

1. **Retrofit every pre-standard design now, not lazily** (a1). The lazy rule
   leaves each new design to discover and retrofit the same documents, which
   duplicates work and risks two branches writing conflicting front-matter for
   one file. Rejected: lazy only (the README wording; slower to make the archive
   searchable).
2. **Front-matter only; document bodies are not edited** (a4). The retrofit
   prepends a front-matter block and changes nothing below it, so the diff is
   mechanical and reviewable. Stale status banners, missing Archive summaries
   and the template's section headings are not added. Rejected: rewriting the
   documents into template shape (large, opinionated, and history the archive
   should keep as written).
3. **`feature` is the file's base name** (a6), for example
   `feature: firecracker-driver-plan`. Rejected: the feature's queue slug, which
   collides where a queue item names a different design
   (`firecracker-driver` uses `firecracker-spike-plan.md`).
4. **`status` reflects the document's current standing** (a2): `archived` when
   its work is merged and it is no longer edited (`hyperv-driver-plan`,
   `windows-driver-plan`); `active` when it is still maintained
   (`vm-driver-release-plan`, `platform-compatibility`); `draft` when nothing it
   describes is built (`firecracker-driver-plan`,
   `remote-execution-and-measured-hermeticity`). `round: parallel` for all six,
   since none went through a merge round.
5. **Values are derived conservatively from the text as written** (FLOW-02).
   Where a plan reaches a shared package (`relay/`, `ledger/`,
   `cmd/warden-io`), `drivers` lists every dependent driver. Where it is unclear
   whether a plan alters relationships, `alters_relationships: true`. `rules`
   maps the plan's subject to the current rule IDs that govern it, since the
   plans predate rule IDs.
6. **`security_boundary` takes the single most severe boundary the document
   alters** (a5), ordered `host-env` > `env-relay` > `env-beyond`. Where a
   document's existing body has a security section, a YAML comment points to it
   as the security note; no `## Security note` section is added (Decision 2).
   Rejected: proposing a list-valued `security_boundary` (a FLOW-02 change for
   human sign-off; not needed to make the archive searchable).
7. **`native_validation` and `tier2_gaps` on retrofitted documents are
   descriptive**: they record which host the plan's work needs and which gaps
   the document itself names. They create no queue steps and no backlog entries;
   the one tracked gap named (`vm-driver-release-plan.md`, qemu Windows guest
   package) is already `qemu-windows-guest-package` in the native-gap backlog.
8. **The three archived designs gain `alters_relationships: true`** (a3): each
   changes a versioned seam or moves a responsibility between components
   (`unified-bootstrap`: bootstrap model; `ledger-environment-types`: ledger
   format and claim; `hyperv-windows-prepare`: Windows guest code moved from two
   drivers into the shared `driver/winguest`). No other key is changed.
9. **Out of scope**: `docs/book/src/design/` (the ledger spec, philosophy and
   initial proposal are book pages governed by RELAY-05, not archive designs);
   `firecracker-spike-plan.md` and `container-inputs.md`, which are on other
   feature branches and get front-matter from their own design phases;
   `TEMPLATE.md`.

## Questions

### Blocking

None.

### Assumed

- **a1. Scope.** Proceeding with retrofitting all six pre-standard designs in
  this feature; confirm. Options: (a) all six now; (b) only those a queued
  feature relies on; (c) lazily per FLOW-12 wording. Recommended: (a).
- **a2. Status mapping.** Proceeding with `archived` for `hyperv-driver-plan`
  and `windows-driver-plan`, `active` for `vm-driver-release-plan` and
  `platform-compatibility`, `draft` for `firecracker-driver-plan` and
  `remote-execution-and-measured-hermeticity`; confirm. Options: (a) this
  mapping; (b) every retrofitted document `archived`; (c) every one `active`.
  Recommended: (a).
- **a3. Complete the archived designs.** Proceeding with adding
  `alters_relationships: true` to `hyperv-windows-prepare.md`,
  `ledger-environment-types.md` and `unified-bootstrap.md`; confirm. Options:
  (a) add `true` to all three (conservative); (b) judge each, possibly `false`
  for `hyperv-windows-prepare`; (c) leave archived designs untouched.
  Recommended: (a).
- **a4. Front-matter only.** Proceeding without editing document bodies (no
  Archive summary, no Security note section, stale banners such as
  `windows-driver-plan.md`'s "Phase 2 pending" left as written); confirm.
  Options: (a) front-matter only; (b) also add a one-line retrofit banner;
  (c) also write Archive summaries for the two archived plans. Recommended: (a).
- **a5. One security boundary per document.** Proceeding with the single most
  severe boundary, others named in a YAML comment; confirm. Options: (a) single
  value; (b) propose a FLOW-02 amendment allowing a list. Recommended: (a).
- **a6. Feature slug.** Proceeding with `feature` = the file's base name;
  confirm. Options: (a) base name; (b) the matching queue slug where one exists.
  Recommended: (a).
- **a7. Firecracker overlap.** Proceeding with retrofitting
  `firecracker-driver-plan.md` here; the `firecracker-driver` design then
  amends it (for example to `archived` once `firecracker-spike-plan.md`
  supersedes it) instead of writing it from scratch; confirm. Options: (a)
  retrofit here; (b) skip it and leave it to `firecracker-driver`.
  Recommended: (a); this item is priority 3, that one 6.
- **a8. Keeping the archive complete.** Proceeding without a permanent
  archive-wide front-matter check in this feature, because `tools/workflow/`
  is not edited unattended (FLOW-14); the check is proposed to
  `tier1-static-checks` (LINT-02); confirm. Options: (a) hand it to
  `tier1-static-checks`; (b) add a test under `tools/workflow/check` in an
  attended step of this feature; (c) no permanent check. Recommended: (a).
- **a9. README follow-up.** Proceeding with leaving the "Follow-up - Design
  front-matter retrofit" bullet in `docs/development/README.md` for the human
  to remove at sign-off (FLOW-14: agents do not edit the standards unattended);
  the PR description gives the exact edit; confirm. Options: (a) human removes
  it at sign-off; (b) an attended step removes it in this branch.
  Recommended: (a).

## Design

Each listed document gets the block below prepended, followed by a blank line,
before its existing first line. Keys follow `TEMPLATE.md` order; no other line
changes. A leading comment marks the block as a retrofit.

### `firecracker-driver-plan.md`

```yaml
---
# Retrofitted (FLOW-12) onto a pre-standard plan; body unchanged.
feature: firecracker-driver-plan
status: draft
round: parallel
drivers: [firecracker]
seams: [relay-guest-api, collector]   # vsock heartbeat/exit; outputs read from relay rootfs predate RELAY-07
surfaces: [cli, config]
security_boundary: host-env           # also env-relay; see "Security Considerations"
alters_relationships: true            # warm-pool manager, vsock signaling
native_validation:
  - {host: linux-kvm, blocking: true, reason: "driver/firecracker runs only on a Linux KVM host"}
tier2_gaps: []
rules: [ORCH-01, ORCH-03, ORCH-04, GLOBAL-06, GLOBAL-07, GLOBAL-08, RELAY-07, BOOT-01, BOOT-02, BOOT-04, TEST-02, TEST-08]
concepts: [firecracker, microvm, vsock, mmds, jailer, warm-pool, snapshot-resume, tap-networking, two-vm-topology]
related: [vm-driver-release-plan.md]
---
```

### `hyperv-driver-plan.md`

```yaml
---
# Retrofitted (FLOW-12) onto a pre-standard plan; body unchanged.
feature: hyperv-driver-plan
status: archived
round: parallel
drivers: [container, qemu, vz, hyperv]  # output sink added to relay/ (shared package)
seams: [relay-claim, collector, relay-guest-api]
surfaces: [cli, config]
security_boundary: host-env             # also env-relay; see "Security Considerations", "Privilege & Elevation Model"
alters_relationships: true
native_validation:
  - {host: windows-hyperv, blocking: true, reason: "driver/hyperv runs only on Hyper-V"}
tier2_gaps: []
rules: [ORCH-01, ORCH-03, ORCH-04, ORCH-05, GLOBAL-06, GLOBAL-07, GLOBAL-08, RELAY-07, BOOT-01, BOOT-02, UX-04, TEST-02]
concepts: [hyperv, windows-host, vswitch, nat, elevation, privileged-service, unattend, differencing-disk, output-egress, collector, uki-relay-boot]
related: [vm-driver-release-plan.md, windows-driver-plan.md, hyperv-windows-prepare.md]
---
```

### `platform-compatibility.md`

```yaml
---
# Retrofitted (FLOW-12) onto a pre-standard reference; body unchanged.
feature: platform-compatibility
status: active
round: parallel
drivers: [container, qemu, vz, hyperv]
seams: []
surfaces: []
security_boundary: none
alters_relationships: false
native_validation: []
tier2_gaps: []
rules: [ORCH-01, ORCH-05, GLOBAL-07, TEST-02, TEST-03, TEST-08]
concepts: [platform-matrix, host-target, driver-availability, licensing, network-isolation, graduation-criteria]
related: [windows-driver-plan.md, vm-driver-release-plan.md]
---
```

### `remote-execution-and-measured-hermeticity.md`

```yaml
---
# Retrofitted (FLOW-12) onto a pre-standard proposal; body unchanged.
feature: remote-execution-and-measured-hermeticity
status: draft
round: parallel
drivers: [container, qemu, vz, hyperv]  # ledger/ changes reach every driver
seams: [ledger-format]
surfaces: [cli, config, output]
security_boundary: env-relay            # per-action relay ledgers and cache provenance; no security section yet
alters_relationships: true              # BuildWarden as an RE API backend; ledgers composed from cached results
native_validation: []
tier2_gaps: []
rules: [RELAY-01, RELAY-02, RELAY-05, GLOBAL-05, GLOBAL-06, GLOBAL-08]
concepts: [remote-execution, re-api, measured-hermeticity, action-cache, provenance, environment-record, cache-invalidation, bazel]
related: [ledger-environment-types.md]
---
```

### `vm-driver-release-plan.md`

```yaml
---
# Retrofitted (FLOW-12) onto a pre-standard roadmap; body unchanged.
feature: vm-driver-release-plan
status: active
round: parallel
drivers: [container, qemu, vz, hyperv, firecracker]
seams: [relay-claim, collector, ledger-format, image-manifest, windows-guest-seed]
surfaces: [cli, config, output]
security_boundary: host-env             # also env-relay; see "Security Testing Framework"
alters_relationships: true
native_validation:
  - {host: macos-vz, blocking: true, reason: "Phase 1 vz driver runs only on macOS/arm64"}
  - {host: windows-hyperv, blocking: true, reason: "Phase 5 and 5a Hyper-V Windows-guest lifecycle"}
  - {host: macos-qemu, blocking: false, reason: "shared Windows guest package on qemu, tracked as qemu-windows-guest-package"}
tier2_gaps:
  - "shared Windows guest package (driver/winguest) on qemu under macOS: not yet validated natively"
rules: [ORCH-01, ORCH-04, ORCH-05, GLOBAL-06, GLOBAL-07, GLOBAL-08, RELAY-05, RELAY-07, TEST-02, TEST-03, TEST-08, SEC-01, UX-04, UX-07]
concepts: [release-plan, driver-matrix, security-testing, escape-vectors, image-lifecycle, relay-vm-distribution, ledger-compatibility, image-prepare, claim, collector]
related: [hyperv-driver-plan.md, windows-driver-plan.md, firecracker-driver-plan.md, hyperv-windows-prepare.md, platform-compatibility.md]
---
```

### `windows-driver-plan.md`

```yaml
---
# Retrofitted (FLOW-12) onto a pre-standard plan; body unchanged.
feature: windows-driver-plan
status: archived
round: parallel
drivers: [container, qemu, vz, hyperv]  # cmd/warden-io changes reach every driver
seams: [executor-initialize, relay-guest-api, windows-guest-seed]
surfaces: [cli, config]
security_boundary: env-relay            # Windows CA trust install and topological isolation; no separate security section
alters_relationships: true              # executor gains a Windows guest; build.ps1 input model
native_validation:
  - {host: macos-qemu, blocking: true, reason: "Phase 1 win-arm64 under qemu on Apple Silicon"}
  - {host: windows-hyperv, blocking: true, reason: "Phase 2 native win-64 on Hyper-V"}
tier2_gaps: []
rules: [EXEC-01, EXEC-02, EXEC-03, EXEC-04, ORCH-01, ORCH-05, BOOT-02, BOOT-04, GLOBAL-06, GLOBAL-07, TEST-02]
concepts: [windows-guest, warden-io-windows, build-ps1, conda-forge, image-prepare, autounattend, licensing, win-arm64]
related: [hyperv-driver-plan.md, vm-driver-release-plan.md, platform-compatibility.md, hyperv-windows-prepare.md]
---
```

### Archived designs

`hyperv-windows-prepare.md`, `ledger-environment-types.md` and
`unified-bootstrap.md` each gain one line, `alters_relationships: true`,
after `security_boundary` (template order). Nothing else changes.

## Cross-driver impact

None. The change touches only Markdown front-matter under `docs/design/`; no
driver tree, shared package, seam or user-facing surface changes, so `drivers`,
`seams` and `surfaces` are empty (ORCH-01). The `drivers` values written into
the retrofitted documents describe those documents, not this change.

## Validation plan

- **Tier 0.** Every `docs/design/*.md` except `TEMPLATE.md` parses with
  `check.ParseDesign` and reports no `Front.problems` other than the expected
  feature-versus-queue mismatch (the retrofitted documents have no queue item).
  `problems` is unexported, so this runs as a scratch test inside the package
  that is never committed (a8); its output goes in the implementation commit
  message. (Phase 1 already parsed each proposed block with `ParseDesign`.)
- A diff check: for each retrofitted document, `git diff` shows only added
  lines at the top of the file (Decision 2).
- `make test` and `make lint` stay green (nothing they cover changes).
- **Tier 1 / 2.** None. No native host validates this change: it reaches no
  driver tree and alters no host's behavior, so FLOW-09 adds no
  `m6-validate-<host>` step and no tracked gap.

## Implementation plan

Each task names the assumed questions it rests on; a different answer touches
only the tasks that name it. No task edits a document body, `tools/workflow/`,
`docs/development/` or `.claude/` (Decision 2, FLOW-14).

1. Prepend the `firecracker-driver-plan.md` block from Design, then one blank
   line, before the existing first line (FLOW-02, FLOW-12, GLOBAL-08).
   Assumes a1, a2 (`draft`), a5, a6, a7; under a7 (b) this task is dropped.
2. Prepend the `hyperv-driver-plan.md` block (FLOW-02, FLOW-12, GLOBAL-06).
   Assumes a1, a2 (`archived`), a5, a6.
3. Prepend the `platform-compatibility.md` block (FLOW-02, FLOW-12). Assumes
   a1, a2 (`active`), a6.
4. Prepend the `remote-execution-and-measured-hermeticity.md` block (FLOW-02,
   FLOW-12, GLOBAL-06). Assumes a1, a2 (`draft`), a5, a6.
5. Prepend the `vm-driver-release-plan.md` block (FLOW-02, FLOW-12, TEST-03:
   its `tier2_gaps` entry names the already-tracked
   `qemu-windows-guest-package` gap, so no backlog entry is added). Assumes
   a1, a2 (`active`), a5, a6.
6. Prepend the `windows-driver-plan.md` block (FLOW-02, FLOW-12, GLOBAL-06).
   Assumes a1, a2 (`archived`), a4 (its "Phase 2 pending" banner stays), a5,
   a6.
7. Rework of already-archived designs (FLOW-06, FLOW-02): insert
   `alters_relationships: true` directly after `security_boundary` in
   `hyperv-windows-prepare.md`, `ledger-environment-types.md` and
   `unified-bootstrap.md`; no other line changes. Assumes a3 (a); under a3 (b)
   the value per file changes, under a3 (c) this task is dropped.
8. Tier-0 parse check (TEST-02 tier 0, TEST-01, FLOW-02, FLOW-03): run
   `check.ParseDesign` and `Front.problems(<base name>)` over every
   `docs/design/*.md` except `TEMPLATE.md`, using the real parser through
   `go test -overlay` with a scratch `_test.go` kept in the session
   scratchpad, so nothing under `tools/workflow/` is created or edited
   (FLOW-14, a8). Pass: no problems for the nine touched files, and
   `alters_relationships` set on each; `workflow-bootstrap.md` and this design
   report only the expected feature-versus-base-name result, if any. Paste the
   output into the implementation commit message. (Planning ran this check on
   the proposed blocks: all nine passed.) Assumes a6 (the base-name comparison)
   and a8.
9. Body-unchanged diff check (TEST-02 tier 0, FLOW-12, GLOBAL-08): for each of
   the nine files, `git diff origin/main -- <file>` shows only added lines,
   and only inside the leading front-matter block (plus its trailing blank
   line for the six retrofits). Record the result with task 8's. Assumes a4.
10. Run `make test` and `make lint` (TEST-01, FLOW-17); no build-tagged file
    changes, so no per-platform lint run is owed. Commit the nine files and
    push `feat/design-front-matter-retrofit` (FLOW-17).
11. Reference docs and overview (UX-01, FLOW-15): none owed. The change has no
    user-facing surface (`surfaces: []`) and changes no component flow, so
    `docs/book/src/guide/` and `docs/book/src/concepts/overview.md` are
    untouched; record "UX-01/FLOW-15: none" in the commit message so the
    review phase can check it.
12. Native validation (FLOW-09, TEST-03): none. No `m6-validate-<host>` step
    and no backlog entry; the retrofitted `native_validation` values are
    descriptive only (Decision 7).
13. For the PR description, written at Publish (FLOW-08, FLOW-14, LINT-02):
    the exact `docs/development/README.md` edit that retires the "Follow-up -
    Design front-matter retrofit" bullet, for the human to apply at sign-off
    (a9); and the archive-wide front-matter check proposed to
    `tier1-static-checks` (a8). Under a9 (b) or a8 (b) these become attended
    steps on this branch instead.

## Consolidation (merge round)

Filled in during the merge round (FLOW-10).

## Rollback notes

None.

## Archive summary

Filled in on merge (FLOW-12).
