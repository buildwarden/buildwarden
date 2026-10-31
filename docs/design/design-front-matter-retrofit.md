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
rules: [FLOW-01, FLOW-02, FLOW-06, FLOW-07, FLOW-09, FLOW-12, FLOW-14, GLOBAL-06, GLOBAL-08, RELAY-03, RELAY-04, BOOT-02, BOOT-06, TEST-02, TEST-03, TEST-08, LINT-02]
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
| `hyperv-driver-plan.md` | Hyper-V driver plan; Windows-guest path implemented, Linux-guest phase not started |
| `platform-compatibility.md` | Host x target x driver matrix; a living reference |
| `remote-execution-and-measured-hermeticity.md` | Vision for an RE API backend; not started |
| `vm-driver-release-plan.md` | Cross-driver roadmap; active (Phase 5a done, later phases open) |
| `windows-driver-plan.md` | Windows guest plan; Phase 1 merged, Phase 2 delivered through Hyper-V |

Three archived designs (`hyperv-windows-prepare.md`,
`ledger-environment-types.md`, `unified-bootstrap.md`) have front-matter but
predate the `alters_relationships` key. They stay as written (a3, Decision 8);
the permanent archive-wide check handed to `tier1-static-checks` has to allow
for them (a8).

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
- `workflow-bootstrap.md` already has complete front-matter (it is archived on
  `main` since #63) and is untouched. The designs that landed on `main` after
  this branch forked (`native-host-loop.md`, `native-loop-followups.md`,
  `relay-leaf-crl.md`) were written with front-matter and are out of scope;
  the merge round checks them (FLOW-10).
- `tools/workflow/check` (`ParseDesign`, `Front.problems`) is the mechanical
  definition of "complete front-matter" that the retrofit is validated against.
- The `firecracker-driver` queue item notes that its design "retrofits
  front-matter (FLOW-12)" onto `firecracker-driver-plan.md`. This feature does
  it first; that feature amends it (a7).
- `tier1-static-checks` plans a front-matter check over the designs a diff
  touches plus an on-demand `tier1 designs -all` over the archive; that is
  where a8 hands the permanent check.

## Decisions

1. **Retrofit every pre-standard design now, not lazily** (a1, confirmed). The
   lazy rule leaves each new design to discover and retrofit the same
   documents, which duplicates work and risks two branches writing conflicting
   front-matter for one file. Rejected: lazy only (the README wording; slower
   to make the archive searchable).
2. **Existing lines are never edited; the retrofit only adds** (a4,
   confirmed). Each document gets a front-matter block prepended. The two
   documents whose `security_boundary` is not `none` and whose body has no
   security section (`windows-driver-plan.md`,
   `remote-execution-and-measured-hermeticity.md`) also get a short
   `## Security note (retrofit)` section appended at the end of the file. No
   existing line changes, so the diff against `main` is added lines only and
   mechanically checkable. Stale status banners, missing Archive summaries and
   the template's other section headings are not added.
   Why the notes rather than an exception (review 7b11f043ab61, MUST 3-4):
   FLOW-02 is a MUST ("any value other than `none` requires a security note in
   the design"), and confirming an assumption cannot waive it. An exception is
   a rule change, which is Jeff's to make (FLOW-14), not something a design
   grants itself. Lowering the boundary to `none` would understate it, against
   conservative derivation. And `remote-execution-and-measured-hermeticity` is
   a draft that the next design will build on; it changes the relay itself, so
   GLOBAL-06 makes it high-severity, and a boundary with no note would be
   inherited silently. The notes record what each document's own text says
   about the boundary and what TEST-08 should exercise; they add no new
   design. Rejected: front-matter only with a recorded FLOW-02 exception (a4
   (b)); rewriting the documents into template shape (large, opinionated, and
   history the archive should keep as written).
3. **`feature` is the matching queue slug where one exists, else the file's
   base name** (a6, Jeff's answer). Of the six documents only
   `firecracker-driver-plan.md` has a matching queue item, so it gets
   `feature: firecracker-driver`; the other five take their base names, since
   no queue item names them. `firecracker-spike-plan.md` (on
   `feat/firecracker-driver`, the queue item's own design) will carry the same
   slug. Jeff accepts that shared value; a YAML comment on the line records it,
   and the `firecracker-driver` design resolves it, for example by archiving
   the older plan as superseded (a7). Rejected (Jeff): the file base name for
   every document (the recommended default, which avoided the shared slug).
4. **`status` reflects the document's current standing** (a2, Jeff's
   answer): `archived` when its work is merged and it is no longer edited
   (`windows-driver-plan`); `active` when it is still maintained or still has
   unbuilt scope (`vm-driver-release-plan`, `platform-compatibility`, and
   `hyperv-driver-plan`, whose Phase 1 Linux build guest is not built:
   `vm-driver-release-plan.md` Phase 5, "Linux guests on Hyper-V not
   started"); `draft` when nothing it describes is built
   (`firecracker-driver-plan`, `remote-execution-and-measured-hermeticity`).
   `hyperv-driver-plan` stays `active` until that phase is built or dropped;
   its `status` comment says so. `round: parallel` for all six, since none went
   through a merge round. Rejected (Jeff): `archived` for `hyperv-driver-plan`
   with a comment naming the unbuilt phase (the recommended default; a search
   on `status` alone would read it as complete).
5. **Values are derived conservatively from the text as written** (FLOW-02).
   Where a plan reaches a shared package (`relay/`, `ledger/`,
   `cmd/warden-io`), `drivers` lists every dependent driver. Where it is unclear
   whether a plan alters relationships, `alters_relationships: true`. `rules`
   maps the plan's subject to the current rule IDs that govern it, since the
   plans predate rule IDs, and includes every rule its appended security note
   relies on (Decision 12).
6. **`security_boundary` takes the single most severe boundary the document
   alters** (a5, confirmed), ordered `host-env` > `env-relay` > `env-beyond`.
   Where a document's existing body has a security section, a YAML comment
   points to it as the security note. Where it has none, the comment points to
   the appended `## Security note (retrofit)` (Decision 2).
   Rejected: proposing a list-valued `security_boundary` (a FLOW-02 change for
   human sign-off; not needed to make the archive searchable).
7. **`native_validation` and `tier2_gaps` on retrofitted documents are
   descriptive**: they record which host the plan's work needs and which gaps
   the document itself names. They create no queue steps and no backlog entries;
   the one tracked gap named (`vm-driver-release-plan.md`, qemu Windows guest
   package) is already `qemu-windows-guest-package` in the native-gap backlog.
   Hosts follow `drivers` (review 7b11f043ab61, SHOULD 6): the primary hosts a
   plan targets keep the blocking value the plan gives them, and every further
   driver that `drivers` lists because of a shared-package change (Decision 5)
   gets one `blocking: false` regression entry on its host: `container` on
   `container-any`, `qemu` on `qemu-any`, `vz` on `macos-vz`, `hyperv` on
   `windows-hyperv`. This covers the VZ and Linux+qemu regression check that
   `windows-driver-plan.md` names in its Phase 2 hand-off, and the "no
   regression on container/qemu/vz" condition on `hyperv-driver-plan.md`'s
   relay output sink. `vm-driver-release-plan` (a roadmap whose phases name
   their own hosts) and `platform-compatibility` (a reference, not a change)
   are not widened this way. The host names are ones already used in the
   archive and the native-gap backlog; `linux-kvm` remains the one new name.
8. **The three archived designs are left untouched** (a3, Jeff's answer).
   `hyperv-windows-prepare.md`, `ledger-environment-types.md` and
   `unified-bootstrap.md` keep their front-matter as merged, without an
   `alters_relationships` key, so `Front.problems` reports "missing
   alters_relationships" on each, and only that. Round 1 added the key as
   `true` to all three (7b11f04); Implement reverts those three lines, so each
   file is again identical to `main`. Consequence for a8: an archive-wide
   check must exempt or tolerate these three; this goes to
   `tier1-static-checks` with the hand-off (Implementation plan task 13).
   Rejected (Jeff): adding `true` to all three (the recommended default; it
   edits merged, archived designs).
9. **Rework after review 7b11f043ab61** (p1-design-r2). The findings were
   against values this design chose, so they were fixed here and Implement
   re-applied the blocks:
   - `firecracker-driver-plan`: `drivers` widens to every relay-dependent
     driver, because the plan changes the relay binary (relay-to-host vsock
     "CA ready" and "ledger complete" signals) and shares `util.go` with qemu.
     `seams` gains `relay-claim`, because the plan injects a per-build
     `relay.env`, extracts the CA from the relay, and hands the warm pool's
     resumed relay its build config over vsock, which is the claim GLOBAL-08
     names. Decision 5 now applies to it as to the Hyper-V and Windows plans.
     `rules` gains RELAY-04 (a relay change states its modes) and RELAY-03:
     the warm pool snapshots a relay after "CA generated, listeners up", so
     every restored relay would share one CA key, which RELAY-03 forbids. The
     retrofit records this in a YAML comment for the `firecracker-driver`
     design to settle and does not edit the plan (Decision 2).
   - `windows-driver-plan`, `remote-execution-and-measured-hermeticity`:
     appended security notes (Decision 2).
   - `hyperv-driver-plan`: the review's SHOULD (an `archived` status hides the
     unbuilt Linux guest) was first met with a comment on `status`; Jeff's a2
     answer replaces that with `active` (Decision 4).
   - `windows-driver-plan`, `hyperv-driver-plan`: regression hosts added to
     `native_validation` (Decision 7). The same rule adds them to
     `firecracker-driver-plan` (its widened `drivers`) and
     `remote-execution-and-measured-hermeticity` (no primary host yet).
10. **Rework after review fdd14b44f1f8** (p1-design-r3). All three findings
    were in the security-note text this design prescribes, so they are fixed
    here. Every claim in a note must be one its own document supports
    (Decision 2: the notes add no design). These note texts were never
    applied: p2-plan-r3 returned the feature before Implement, so the branch
    still carries the round-2 notes and round 4 applies these (task 0).
    - `windows-driver-plan` (MUST): the note no longer says administrator
      rights inside the guest cannot open a path. The plan argues isolation
      from host-enforced topology only, its guest has an autologon admin, and
      its adversarial isolation check (Phasing chunk 4, graduation gate) is
      not in Phase 1's "Done & validated" list. The note now says the
      argument relies on no in-guest control, and leaves the
      guest-administrator question to that gate.
    - `windows-driver-plan` (SHOULD): the prepared image is "input fixed at
      prepare" (BOOT-06), not "trusted input"; the plan calls the guest
      "untrusted by design".
    - `remote-execution-and-measured-hermeticity` (SHOULD): the Boundary
      bullet adds CAS input staging outside the relay (lifecycle step 4,
      against BOOT-02 today) and the RE API services as a new network-facing
      surface; the TEST-08 bullet adds digest-mismatched CAS blobs and
      unauthenticated Execute or ActionCache writes. `security_boundary`
      stays `env-relay`: both changes are about what reaches the build
      environment and the ledger, not the host-to-environment boundary.
11. **Out of scope**: `docs/book/src/design/` (the ledger spec, philosophy
    and initial proposal are book pages governed by RELAY-05, not archive
    designs); `firecracker-spike-plan.md` and `container-inputs.md`, which are
    on other feature branches and get front-matter from their own design
    phases; `TEMPLATE.md`; the designs added to `main` after the fork, which
    already carry front-matter.
12. **Round 4, after the FLOW-06 escalation p1-escalate-3** (p1-design-r4).
    p2-plan-r3 found that the round-3 notes cite rules their documents'
    `rules` omit, against FLOW-02 ("the rule IDs the design relies on or
    changes") and Decision 5. Jeff decided:
    - `windows-driver-plan` `rules` gains BOOT-06: its note says the prepared
      image carries the startup hook, not build content (BOOT-06). BOOT-02 and
      BOOT-04 were already listed.
    - `remote-execution-and-measured-hermeticity` `rules` gains BOOT-02: its
      note says CAS input staging outside the relay is something BOOT-02 does
      not allow today. RELAY-03, GLOBAL-06 and TEST-08, which the note also
      cites, were already listed.
    - This design's own `rules` gains BOOT-02 and BOOT-06 (Decisions 10 and
      12 rely on them), and FLOW-06, under which every rework round ran.
    - The notes' text is otherwise unchanged from round 3. Each note's opening
      line cites FLOW-02 and FLOW-12 to say why the note was added; those
      describe the retrofit, not the document's subject, so they are not
      added to the documents' `rules`.
    - The answers to a2, a3, a6 and a9 that differ from the recommendation are
      applied: Decisions 3, 4 and 8 and Implementation plan task 13. Round 4
      changes three front-matter lines (firecracker `feature`, hyperv
      `status`, and the two `rules` lines), reverts the three archived-design
      lines, and applies the round-3 notes. Everything else on the branch at
      0638781 stays.

## Questions

### Blocking

None.

### Assumed

All nine were confirmed by Jeff on 2026-10-31 (queue history); four were
answered other than recommended (a2, a3, a6, a9), and the design follows each
answer.

- **a1. Scope.** Answered: (a) all six now (as recommended). Options: (a) all
  six now; (b) only those a queued feature relies on; (c) lazily per FLOW-12
  wording. Recommended: (a). Applied in Decision 1.
- **a2. Status mapping.** Answered: (b) as (a), but `hyperv-driver-plan`
  `active` until its Linux-guest phase is built or dropped (not the
  recommendation). Options: (a) `archived` for `hyperv-driver-plan` and
  `windows-driver-plan`, `active` for `vm-driver-release-plan` and
  `platform-compatibility`, `draft` for `firecracker-driver-plan` and
  `remote-execution-and-measured-hermeticity`, with a comment on
  `hyperv-driver-plan`'s `status` naming its unbuilt Linux-guest phase; (b) as
  (a) but `hyperv-driver-plan` `active` until that phase is built or dropped;
  (c) every retrofitted document `archived`; (d) every one `active`.
  Recommended: (a). Applied in Decision 4 and the `hyperv-driver-plan` block.
- **a3. Complete the archived designs.** Answered: (c) leave archived designs
  untouched (not the recommendation). Options: (a) add
  `alters_relationships: true` to `hyperv-windows-prepare.md`,
  `ledger-environment-types.md` and `unified-bootstrap.md` (conservative);
  (b) judge each, possibly `false` for `hyperv-windows-prepare`; (c) leave
  archived designs untouched. Recommended: (a). Applied in Decision 8; the
  round-1 lines are reverted (task 7).
- **a4. Add only; no existing line edited.** Answered: (a) front-matter plus
  the two appended security notes (as recommended). Options: (a) a prepended
  block on every document plus a short `## Security note (retrofit)` appended
  to `windows-driver-plan.md` and
  `remote-execution-and-measured-hermeticity.md`, no Archive summary, stale
  banners left as written; (b) front-matter only, recording a FLOW-02
  exception for the two documents (a rule change for Jeff, FLOW-14); (c) as
  (a), plus a one-line retrofit banner; (d) as (a), plus Archive summaries for
  the two archived plans. Recommended: (a). Applied in Decision 2.
- **a5. One security boundary per document.** Answered: (a) single value plus
  comment (as recommended). Options: (a) the single most severe boundary,
  others named in a YAML comment that also points to the document's security
  note; (b) propose a FLOW-02 amendment allowing a list. Recommended: (a).
  Applied in Decision 6.
- **a6. Feature slug.** Answered: (b) the matching queue slug where one exists
  (not the recommendation); Jeff accepts that `firecracker-driver-plan` then
  shares `feature: firecracker-driver` with `firecracker-spike-plan`, and
  `firecracker-driver`'s design resolves it. Options: (a) the file's base
  name; (b) the matching queue slug where one exists. Recommended: (a).
  Applied in Decision 3 and all six blocks (only the firecracker block
  differs from the base name).
- **a7. Firecracker overlap.** Answered: (a) retrofit here;
  `firecracker-driver` amends (as recommended). Options: (a) retrofit
  `firecracker-driver-plan.md` here, the `firecracker-driver` design then
  amends it (for example to `archived` once `firecracker-spike-plan.md`
  supersedes it); (b) skip it and leave it to `firecracker-driver`.
  Recommended: (a).
- **a8. Keeping the archive complete.** Answered: (a) hand it to
  `tier1-static-checks` (as recommended). Options: (a) hand a permanent
  archive-wide front-matter check to `tier1-static-checks` (LINT-02), since
  `tools/workflow/` is not edited unattended (FLOW-14); (b) add a test under
  `tools/workflow/check` in an attended step of this feature; (c) no
  permanent check. Recommended: (a). The hand-off now also says that the three
  archived designs lack `alters_relationships` (a3, Decision 8).
- **a9. README follow-up.** Answered: (b) an attended step removes it on this
  branch (not the recommendation). Options: (a) the human removes the
  "Follow-up - Design front-matter retrofit" bullet in
  `docs/development/README.md` at sign-off, with the exact edit in the PR
  description; (b) an attended step removes it on this branch. Recommended:
  (a). Applied as the queued human step `p3-remove-readme-bullet` (FLOW-14:
  agents do not edit the standards unattended); the PR description no longer
  carries the edit (task 13).

## Design

Each listed document gets the block below prepended, followed by a blank line,
before its existing first line. Keys follow `TEMPLATE.md` order. A leading
comment marks the block as a retrofit. `windows-driver-plan.md` and
`remote-execution-and-measured-hermeticity.md` also get the security note
given under their block appended after their existing last line, separated by
one blank line. The note's heading is written outside its fence, because
`check.ParseDesign` splits sections on any line starting `## `, fenced or not.
No existing line changes (Decision 2).

The code branch at 0638781 carries the round-2 blocks and the round-2 notes.
Four block lines below differ from it (Decision 12), the two notes below
replace the round-2 notes (Decision 10), and the three archived designs return
to their `main` content (Decision 8). The Implementation plan gives each
change.

### `firecracker-driver-plan.md`

```yaml
---
# Retrofitted (FLOW-12) onto a pre-standard plan; body unchanged.
feature: firecracker-driver           # queue slug (a6); shared with firecracker-spike-plan.md until the firecracker-driver design resolves it
status: draft
round: parallel
drivers: [container, qemu, vz, hyperv, firecracker]  # relay/ gains vsock "CA ready"/"ledger complete" signals; util.go shared with qemu
seams: [relay-claim, relay-guest-api, collector]     # per-build relay.env and CA extraction; warm-pool config injected over vsock; outputs read from relay rootfs predate RELAY-07
surfaces: [cli, config]
security_boundary: host-env           # also env-relay; see "Security Considerations"
alters_relationships: true            # warm-pool manager, vsock signaling, relay claim over vsock
native_validation:
  - {host: linux-kvm, blocking: true, reason: "driver/firecracker runs only on a Linux KVM host"}
  - {host: container-any, blocking: false, reason: "regression: relay/ vsock signaling change"}
  - {host: qemu-any, blocking: false, reason: "regression: relay/ change and util.go shared with qemu"}
  - {host: macos-vz, blocking: false, reason: "regression: relay/ vsock signaling change"}
  - {host: windows-hyperv, blocking: false, reason: "regression: relay/ vsock signaling change"}
tier2_gaps: []
rules: [ORCH-01, ORCH-03, ORCH-04, GLOBAL-06, GLOBAL-07, GLOBAL-08, RELAY-03, RELAY-04, RELAY-07, BOOT-01, BOOT-02, BOOT-04, TEST-02, TEST-08]  # RELAY-03: warm-pool snapshot is taken after CA generation, so restored relays would share a CA key
concepts: [firecracker, microvm, vsock, mmds, jailer, warm-pool, snapshot-resume, tap-networking, two-vm-topology]
related: [vm-driver-release-plan.md]
---
```

### `hyperv-driver-plan.md`

```yaml
---
# Retrofitted (FLOW-12) onto a pre-standard plan; body unchanged.
feature: hyperv-driver-plan
status: active                          # Windows-guest path merged; active until the Phase 1 Linux build guest is built or dropped (vm-driver-release-plan Phase 5)
round: parallel
drivers: [container, qemu, vz, hyperv]  # output sink added to relay/ (shared package)
seams: [relay-claim, collector, relay-guest-api]
surfaces: [cli, config]
security_boundary: host-env             # also env-relay; see "Security Considerations", "Privilege & Elevation Model"
alters_relationships: true
native_validation:
  - {host: windows-hyperv, blocking: true, reason: "driver/hyperv runs only on Hyper-V"}
  - {host: container-any, blocking: false, reason: "regression: relay/ output sink; localSink must stay the default"}
  - {host: qemu-any, blocking: false, reason: "regression: relay/ output sink; localSink must stay the default"}
  - {host: macos-vz, blocking: false, reason: "regression: relay/ output sink; localSink must stay the default"}
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
# Retrofitted (FLOW-12) onto a pre-standard proposal; existing lines unchanged, security note appended.
feature: remote-execution-and-measured-hermeticity
status: draft
round: parallel
drivers: [container, qemu, vz, hyperv]  # ledger/ and relay changes reach every driver
seams: [ledger-format]
surfaces: [cli, config, output]
security_boundary: env-relay            # per-action relay ledgers and cache provenance; see "Security note (retrofit)"
alters_relationships: true              # BuildWarden as an RE API backend; ledgers composed from cached results
native_validation:
  - {host: container-any, blocking: false, reason: "regression: ledger/ and relay changes reach every driver"}
  - {host: qemu-any, blocking: false, reason: "regression: ledger/ and relay changes reach every driver"}
  - {host: macos-vz, blocking: false, reason: "regression: ledger/ and relay changes reach every driver"}
  - {host: windows-hyperv, blocking: false, reason: "regression: ledger/ and relay changes reach every driver"}
tier2_gaps: []
rules: [RELAY-01, RELAY-02, RELAY-03, RELAY-05, GLOBAL-05, GLOBAL-06, GLOBAL-08, BOOT-02, TEST-08]
concepts: [remote-execution, re-api, measured-hermeticity, action-cache, provenance, environment-record, cache-invalidation, bazel]
related: [ledger-environment-types.md]
---
```

Appended after the existing last line: one blank line, the heading line
`## Security note (retrofit)`, one blank line, then this text:

```markdown
Added when front-matter was retrofitted (FLOW-02, FLOW-12). It restates what
this proposal's text implies for GLOBAL-06; it is not a security design. The
first design that builds on this proposal writes its own note and settles the
open points below.

- **Boundary:** env-relay, and the relay itself. A parent build's ledger
  incorporates environment and network records from cached sub-ledgers that
  its own relay did not observe ("Cache hit behavior"), and each worker relay
  writes an environment record as the first ledger entry. Both change what a
  signed ledger attests, which GLOBAL-06 makes high-severity. The proposal
  also implies two further boundary changes. Inputs are staged from
  content-addressed storage into the sandbox outside the relay ("Action
  execution lifecycle", step 4), which BOOT-02 does not allow today; only the
  action's cache key (`input_root_digest`) records them. And the RE API
  services (Execute, ContentAddressableStorage, ActionCache) are a new
  network-facing surface whose contents feed parent ledgers.
- **How the posture changes:** a parent ledger's claims now rest on the action
  cache and on other relays' signatures, as well as on its own observation. A
  forged, tampered or replayed cache entry (`ledger_digest`,
  `environment_digest`, `hermetic`) would put false provenance into a validly
  signed parent ledger. Open points: verify each sub-ledger's signature chain
  and the worker relay's key before incorporating it; recompute `hermetic`
  from the verified sub-ledger, never from the stored flag; mark incorporated
  records as incorporated rather than observed; keep one fresh relay per
  action (RELAY-03), as "Worker model" already requires.
- **What TEST-08 should exercise:** a sub-ledger with a broken signature, a
  wrong `ledger_digest` or an unknown signing key is rejected, not
  incorporated; a cache entry whose stored `hermetic` disagrees with its
  ledger; deduplication by hash block cannot hide a record whose content
  differs; an action marked `requires-network` reaches the network only
  through a relay (its worker's, or the parent build's under `no-remote`);
  CAS blobs that differ from their digests; an unauthenticated Execute or
  ActionCache write.
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
# Retrofitted (FLOW-12) onto a pre-standard plan; existing lines unchanged, security note appended.
feature: windows-driver-plan
status: archived
round: parallel
drivers: [container, qemu, vz, hyperv]  # cmd/warden-io changes reach every driver
seams: [executor-initialize, relay-guest-api, windows-guest-seed]
surfaces: [cli, config]
security_boundary: env-relay            # Windows CA trust install and topological isolation; see "Security note (retrofit)"
alters_relationships: true              # executor gains a Windows guest; build.ps1 input model
native_validation:
  - {host: macos-qemu, blocking: true, reason: "Phase 1 win-arm64 under qemu on Apple Silicon"}
  - {host: windows-hyperv, blocking: true, reason: "Phase 2 native win-64 on Hyper-V"}
  - {host: macos-vz, blocking: false, reason: "regression check the plan requires: VZ path untouched"}
  - {host: qemu-any, blocking: false, reason: "regression check the plan requires: Linux+qemu disk, seed and serial unchanged"}
  - {host: container-any, blocking: false, reason: "regression: cmd/warden-io changes reach the container driver"}
tier2_gaps: []
rules: [EXEC-01, EXEC-02, EXEC-03, EXEC-04, ORCH-01, ORCH-05, BOOT-02, BOOT-04, BOOT-06, GLOBAL-06, GLOBAL-07, TEST-02, TEST-08]
concepts: [windows-guest, warden-io-windows, build-ps1, conda-forge, image-prepare, autounattend, licensing, win-arm64]
related: [hyperv-driver-plan.md, vm-driver-release-plan.md, platform-compatibility.md, hyperv-windows-prepare.md]
---
```

Appended after the existing last line: one blank line, the heading line
`## Security note (retrofit)`, one blank line, then this text:

```markdown
Added when front-matter was retrofitted (FLOW-02, FLOW-12). It gathers what
this plan already says about GLOBAL-06; it adds no design.

- **Boundary:** env-relay. The Windows guest is a new kind of inner build
  environment behind the same relay.
- **How the posture changes:** `warden-io` on Windows imports the relay's
  per-build CA into the machine root store (`certutil -addstore -f Root`) and
  sets the CA environment variables, so every Schannel and OpenSSL client in
  the guest trusts the relay's MITM CA for that build. Isolation stays
  topological and host-enforced: the guest's single NIC faces the relay VM,
  which redirects :80, :443 and :53 and drops forwarding. There is no
  in-guest firewall, so the isolation argument does not rely on any in-guest
  control; whether a guest administrator can open a path is what the
  adversarial gate below must show, and the plan does not record that gate
  as run. The prepared Windows image (autounattend, first-boot task) is input fixed at
  prepare, before the build (BOOT-06: it carries the startup hook, not build
  content); per build, only the executor and its seed cross outside the
  relay (BOOT-02).
- **What TEST-08 should exercise:** egress from the Windows guest that
  bypasses the relay (direct IP, other ports, IPv6, SMB/NetBIOS, ICMP,
  DNS-over-HTTPS); that the per-build CA does not persist into the prepared
  image or a later build; that the seed carries nothing beyond the executor,
  launcher and network configuration; and the adversarial "escape the build
  env" agent the plan's own Phasing and graduation gates require.
```

### Archived designs

`hyperv-windows-prepare.md`, `ledger-environment-types.md` and
`unified-bootstrap.md` are not changed (a3, Decision 8). On this branch each
is byte-identical to its `main` content.

## Cross-driver impact

None. The change touches only Markdown under `docs/design/` (front-matter,
plus two appended security notes) and, through Jeff's attended step, one
bullet in `docs/development/README.md`; no driver tree, shared package, seam
or user-facing surface changes, so `drivers`, `seams` and `surfaces` are
empty (ORCH-01). The `drivers` values written into the retrofitted documents
describe those documents, not this change.

## Validation plan

- **Tier 0, parse.** Every `docs/design/*.md` on the branch except
  `TEMPLATE.md` parses with `check.ParseDesign`, and `Front.problems(want)`,
  where `want` is the queue slug for `firecracker-driver-plan`
  (`firecracker-driver`) and the base name for every other file (a6),
  reports:
  - nothing for the six retrofitted documents, this design and
    `workflow-bootstrap.md`;
  - exactly `missing alters_relationships` for `hyperv-windows-prepare.md`,
    `ledger-environment-types.md` and `unified-bootstrap.md` (a3, expected);
  - and `Sections` of the two noted files contains
    `Security note (retrofit)`.

  `problems` is unexported, so this runs through `go test -overlay` with a
  scratch `_test.go` kept in the session scratchpad; nothing under
  `tools/workflow/` is created or edited (FLOW-14, a8). Its output goes in
  the implementation commit message. Phase 1 (round 4) ran exactly this
  check on the end state these blocks and notes produce, and got that
  result.
- **Diff checks.** Against the merge base with `main`: each retrofitted
  document shows only added lines (the leading block, plus the appended note
  for the two noted files); the three archived designs show no diff. The six
  blocks and two notes are byte-identical to Design.
- `make test` and `make lint` stay green (nothing they cover changes).
- **Tier 1 / 2.** None. No native host validates this change: it reaches no
  driver tree and alters no host's behavior, so FLOW-09 adds no
  `m6-validate-<host>` step and no tracked gap.

## Implementation plan

Round 4 (p1-design-r4, after Jeff's decision on p1-escalate-3; p2-plan-r4 may
refine it). Every assumed question is answered (a1-a9), so no task is
conditional. Each task carries one status. **Done** means it shipped and
round 4 does not change it: Implement does not touch it again, and the task
names the commit. **Rework** means it shipped and round 4 changes it:
Implement redoes it as the task says (FLOW-06). **To do** means it is not
done yet, or is a check that must run again on the round-4 tree.

The starting point is `feat/design-front-matter-retrofit` at 0638781 or
later. Before starting, `git pull --ff-only`: Jeff's attended step
`p3-remove-readme-bullet` may have added a commit that touches only
`docs/development/README.md`; keep it. If a push is later rejected because
that commit arrived meanwhile, `git pull --rebase` (local, unpushed commits
only) and push again, never with force. Merging `main` is the merge round's
job, not Implement's. No task edits an original body line of a retrofitted
document, `tools/workflow/`, `docs/development/` or `.claude/` (Decision 2,
FLOW-14). Each edit below replaces one whole line; the old line is given so
Implement can check it before replacing it. Make one commit per file, citing
the task, the file and Decision 12 (or 8 or 10).

0. **To do**, the round-4 procedure (FLOW-06, FLOW-02). Apply tasks 1, 2, 4,
   6 and 7 in that order on the branch, then run tasks 8-10. Tasks 3 and 5 are
   **Done**. Afterwards every block and note on the branch is byte-identical
   to Design.
1. **Rework** (round 2: ba605ac), `firecracker-driver-plan.md` (FLOW-02,
   FLOW-12, a6). Replace line 3, `feature: firecracker-driver-plan`, with the
   `feature:` line of its Design block (the slug `firecracker-driver` plus
   the comment). No other line changes.
2. **Rework** (round 2: 4805056), `hyperv-driver-plan.md` (FLOW-02, FLOW-12,
   a2). Replace line 4, the `status: archived` line with its comment, with
   the `status:` line of its Design block (`active` plus the comment). No
   other line changes.
3. **Done** (51aedd3), `platform-compatibility.md` block (FLOW-02,
   FLOW-12).
4. **Rework** (round 2: d5f83c3), `remote-execution-and-measured-hermeticity.md`
   (FLOW-02, FLOW-06, GLOBAL-06, TEST-08).
   - Replace line 17, `rules: [RELAY-01, RELAY-02, RELAY-03, RELAY-05,
     GLOBAL-05, GLOBAL-06, GLOBAL-08, TEST-08]`, with the `rules:` line of its
     Design block, which adds BOOT-02 before TEST-08 (Decision 12).
   - Replace the round-2 note text, that is every line after the blank line
     that follows the `## Security note (retrofit)` heading, through the end
     of the file, with the note from Design, verbatim, ending in a newline
     (Decision 10). Leave the heading line, the blank lines around it and
     every original body line as they are.
5. **Done** (894629d), `vm-driver-release-plan.md` block (FLOW-02, FLOW-12,
   TEST-03). Its `tier2_gaps` entry names the gap already tracked as
   `qemu-windows-guest-package`, so no backlog entry is added.
6. **Rework** (round 2: fdd14b4), `windows-driver-plan.md` (FLOW-02,
   FLOW-06, GLOBAL-06, TEST-08, BOOT-06).
   - Replace line 18, the `rules:` line, with the `rules:` line of its Design
     block, which adds BOOT-06 after BOOT-04 (Decision 12).
   - Replace the round-2 note text exactly as in task 4, with this file's
     note from Design (Decision 10). The "Phase 2 pending" banner stays.
7. **Rework** (round 1: 7b11f04), the three archived designs (FLOW-02,
   FLOW-12, a3). In each of `hyperv-windows-prepare.md`,
   `ledger-environment-types.md` and `unified-bootstrap.md`, delete line 9,
   `alters_relationships: true`, which round 1 added. Equivalently,
   `git checkout $(git merge-base origin/main HEAD) -- <file>`. Acceptance:
   `git diff $(git merge-base origin/main HEAD) HEAD -- <file>` is empty.
   One commit may cover all three, citing a3 and Decision 8.
8. **To do**, on the round-4 tree: the tier-0 parse check (TEST-02 tier 0,
   TEST-01, FLOW-02), as the Validation plan states.
   - Run `check.ParseDesign` and `Front.problems(want)` over every
     `docs/design/*.md` on the branch except `TEMPLATE.md`, with `want` the
     queue slug `firecracker-driver` for `firecracker-driver-plan.md` and the
     base name otherwise. Use the real parser through `go test -overlay` with
     a scratch `_test.go` kept in the session scratchpad, so nothing under
     `tools/workflow/` is created or edited (FLOW-14, a8).
   - Pass: no problems in the six retrofitted documents, this design and
     `workflow-bootstrap.md`; exactly `missing alters_relationships` in each
     of the three archived designs (a3); `Sections` of the two noted files
     contains `Security note (retrofit)`.
   - `native-host-loop.md`, `native-loop-followups.md` and `relay-leaf-crl.md`
     are on `origin/main` only and are not in scope.
   - Paste the output into the last rework commit's message.
9. **To do**, on the round-4 tree: the diff checks (TEST-02 tier 0, FLOW-12,
   FLOW-02). `origin/main` has moved since the branch point (to cc6119e; it
   touched no file here), so diff against the merge base `MB=$(git
   merge-base origin/main HEAD)`.
   - (a) For each of the six retrofitted files, `git diff -U0 $MB HEAD --
     <file>` shows 0 removed lines. `windows-driver-plan.md` and
     `remote-execution-and-measured-hermeticity.md` have exactly two hunks:
     the leading block plus its trailing blank line, then, at the end of the
     file, the blank line, the heading line, the blank line and the note.
     Each of the other four has exactly one hunk: the block plus its blank
     line.
   - (b) The three archived designs have no diff against `$MB` (task 7).
   - (c) The six blocks and two notes are byte-identical to Design.
   - (d) `git diff --stat 0638781 HEAD` lists only the four reworked
     retrofits, the three archived designs, this design (if a later plan
     commit touches it) and, if Jeff's step has landed,
     `docs/development/README.md`; the two **Done** files are untouched.
   - (e) `git diff -U0 0638781 HEAD` changes exactly one line in
     `firecracker-driver-plan.md` (3) and in `hyperv-driver-plan.md` (4); in
     the two noted files, the `rules:` line plus only lines after the
     `## Security note (retrofit)` heading.

   Record the result with task 8's.
10. **To do**: run `make test` and `make lint` (TEST-01, FLOW-17). Put the
    go1.26.3 toolchain that golangci-lint needs on `PATH`. No build-tagged
    file changes, so no per-platform lint run is owed. Push
    `feat/design-front-matter-retrofit` after the rework commits, without
    force (FLOW-16, FLOW-17).
11. **To do**, reference docs and overview (UX-01, FLOW-15): none are owed.
    The change has no user-facing surface (`surfaces: []`) and changes no
    component flow, so `docs/book/src/guide/` and
    `docs/book/src/concepts/overview.md` stay as they are. Record
    "UX-01/FLOW-15: none" in the last rework commit's message so that Review
    can check it.
12. **To do**, native validation (FLOW-09, TEST-03): none. This feature adds
    no `m6-validate-<host>` step and no backlog entry. The retrofitted
    `native_validation` values, including `linux-kvm`, which no machine in
    `hosts.yaml` serves yet, are descriptive only (Decision 7).
13. **To do**, for the PR description written at Publish (FLOW-08, LINT-02,
    FLOW-12). It contains:
    - the archive-wide front-matter check handed to `tier1-static-checks`
      (a8), with the note that `hyperv-windows-prepare.md`,
      `ledger-environment-types.md` and `unified-bootstrap.md` lack
      `alters_relationships` by decision (a3), so `tier1 designs -all` must
      exempt or tolerate them;
    - the RELAY-03 warm-pool CA point recorded in the firecracker block's
      `rules` comment, and the `feature: firecracker-driver` value it shares
      with `firecracker-spike-plan.md`, both for the `firecracker-driver`
      design (a6, a7);
    - the review's informational suggestion of a follow-up item to refresh
      the stale `platform-compatibility.md` body, for intake (FLOW-12).

    It does not carry the `docs/development/README.md` edit: under a9 Jeff
    removes the "Follow-up - Design front-matter retrofit" bullet on this
    branch in the attended step `p3-remove-readme-bullet` (FLOW-14). Publish
    only checks that the bullet is gone from the branch, and if it is not,
    says so in the PR description rather than editing the README.

## Consolidation (merge round)

Filled in during the merge round (FLOW-10).

## Rollback notes

None.

## Archive summary

Filled in on merge (FLOW-12).
