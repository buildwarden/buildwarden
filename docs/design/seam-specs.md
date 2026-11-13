---
feature: seam-specs
status: draft
drivers: []              # no driver tree and no shared package changes (see Cross-driver impact)
seams: [relay-claim, collector, relay-guest-api, executor-initialize, bootstrap-config]
                         # each gets its contract document; none changes behaviour here
surfaces: []
security_boundary: none  # no code on a boundary changes; see the Security note
alters_relationships: true   # the seams' definition and their contract tests move to another
                         # repository, and this one depends on a tagged release of it
design_review: required
native_validation: []    # nothing blocking; two tracked gaps (Validation plan)
tier2_gaps:
  - {host: macos-vz, what: "relay suite against the host-mode Go relay on macOS"}
  - {host: qemu-any, what: "relay suite against the Go relay VM image, once a VM launcher exists"}
rules: [GLOBAL-08, GLOBAL-09, GLOBAL-10, GLOBAL-11, RELAY-02, RELAY-04, RELAY-05, RELAY-07,
        RELAY-08, EXEC-02, EXEC-04, BOOT-04, BOOT-05, BOOT-06, BOOT-07, TEST-02, TEST-03,
        TEST-04, TEST-05, TEST-07, LINT-02, LINT-05, FLOW-01, FLOW-02, FLOW-07, FLOW-08,
        FLOW-09, FLOW-12, FLOW-14, FLOW-15, FLOW-17, FLOW-18, FLOW-19, FLOW-20]
rule_changes:
  - {rule: TEST-05, change: amend, paths: [docs/development/testing.md, tools/tier1/rules.yaml]}
  - {rule: TEST-02, change: amend, paths: [docs/development/testing.md]}
  - {rule: GLOBAL-08, change: amend, paths: [docs/development/global.md, tools/tier1/rules.yaml]}
  - {rule: EXEC-04, change: amend, paths: [docs/development/in-vm-executor.md]}
concepts: [seams, specs-repo, conformance, conformance-harness, relay-claim, claim-protocol,
           collector, collector-protocol, relay-guest-api, executor-initialize,
           bootstrap-config, specs-pin, seam-contract, program]
related: [componentization.md, specs-migration.md, relay05-specs-wording.md,
          tier1-static-checks.md, build-link-subnet.md, hyperv-relay-per-build-channel.md,
          unified-bootstrap.md, relay-leaf-crl.md, ledger-environment-types.md,
          design-approved-rule-changes.md]
---

# Seam specifications and conformance suites (componentization S1)

This is a program item (FLOW-20): phase S1 of
[`componentization`](componentization.md). That design fixes what S1 is:
create `buildwarden/buildwarden-specs` (its a10), write seams 1, 2, 3, 5 and 6,
build a conformance harness that runs black-box against any relay binary or
image and against any collector, link the book and the specs both ways, and
fill `seams` in `tools/tier1/rules.yaml`. It is done when the suites pass
against the Go relay and the disk collector. This design works inside those
decisions and does not reopen them.

## Problem

Five of the seams between BuildWarden's components exist only as Go code:

| # | Seam | Where it is defined today |
|---|---|---|
| 1 | Claim protocol (`relay-claim`) | `relaycfg/` and its package comment; what each relay mode does with a claim is in `cmd/relay/` |
| 2 | Collector protocol (`collector`) | `collector/` and its package comment; the relay's side is `relay/collector_client.go` |
| 3 | Relay guest API (`relay-guest-api`) | `relay/relay.go` (`handleArtifacts`, `handleContextGet`), `relay/controlplane.go`, `relay/dns.go`, `relay/heartbeat.go`, `cmd/relay/dhcp.go` |
| 5 | `initialize` contract (`executor-initialize`) | `cmd/warden-io/main.go`, `initialize.go`, `baseline.go`, `platform_<os>.go` |
| 6 | Bootstrap config (`bootstrap-config`) | `bootstrap/bootstrap.go` |

Three things follow.

- The Rust relay (`buildwarden/buildwarden-relay`) has to implement seams 1,
  2, 3 and 6 and has nothing to implement them from but another
  implementation. Its milestone M1 takes its conformance harness from this
  item.
- GLOBAL-08 and EXEC-04 say a change to a versioned seam also changes the
  seam's contract document, and tier 1's `seam-contract` check enforces that
  for the seams `rules.yaml` lists. The list is empty, because there are no
  documents. PR #115 changed the claim (`upstream_dns`, the claim token on
  Hyper-V) with nothing to update.
- TEST-05 asks for conformance tests "so a third-party substitute has
  something to verify against". The tests that exist are Go unit tests that
  import the packages under test. A relay in another language cannot run
  them, and neither can a third party who has only a binary.

## Prior art

- [`componentization`](componentization.md): Decisions 2 and 3 (components
  coupled through written seams and tagged releases; specifications with
  black-box suites in their own repository), "a phase that touches a seam
  changes the seam's specification first, then the code", Decision 18 (a rule
  changes in the item that makes it true), and its assumptions a1 (the collector specification in
  two parts), a10 and a11 (the repository's name, and who creates it). Its
  Consolidation records what #115 added to the claim and to BOOT-06; the
  specifications describe `main` with both.
- [`specs-migration`](specs-migration.md) and
  [`relay05-specs-wording`](relay05-specs-wording.md): the layout of `specs/`
  (one directory per document family, `v<N>.md` beside `v<N>.schema.json`,
  an index in `README.md`), the rule that a schema's `$id` is a name and not a
  location, and the tier-0 test that ties the producer to the documents. The
  specs repository uses the same layout, extended (Decision 2).
- [`tier1-static-checks`](tier1-static-checks.md) built `seam-contract` and
  kept its seam list as data in `rules.yaml`, "so new seams edit data, not
  code". This item fills the list and changes no check code.
- [`build-link-subnet`](build-link-subnet.md) (the claim's `build_link`, what
  each mode does with it, the refusal that leaves a relay claimable) and
  [`hyperv-relay-per-build-channel`](hyperv-relay-per-build-channel.md)
  (`upstream_dns`, the claim token, RELAY-08, BOOT-06's boot channel) are the
  two designs whose text the claim specification is checked against.
- [`unified-bootstrap`](unified-bootstrap.md): `GET /bootstrap` and the
  `bootstrap` package (seam 6), and the baseline `initialize` applies.
- [`relay-leaf-crl`](relay-leaf-crl.md): `/ca.crl` on the reserved host and
  the CRL distribution point in every leaf (seam 3).
- [`ledger-environment-types`](ledger-environment-types.md): the claim's
  `environment` and the collector's `PUT /v1/environment`.
- [`design-approved-rule-changes`](design-approved-rule-changes.md): the
  route by which this design's four rule changes are approved by name.

All ten already carry front-matter; no retrofit is needed (FLOW-12).

Outside this repository: `buildwarden-relay` `main` at `e9b23d5`
(`spike/REPORT.md`). What the harness takes from it is in **What a relay under
test must provide**: the Rust relay's build link is raw Ethernet frames on a
socket, and its claim will arrive over vsock on VZ.

## Decisions

1. **The specifications say what `main` does, and this item changes no
   behaviour.** Where the code and a rule or a comment disagree, the
   specification records the code and marks the point as a divergence
   (**Divergences found**); none is fixed here. *Rejected:* fixing them in
   this item. Each fix is a seam change on the relay, the collector or the
   executor, which would put `relay/` and `cmd/warden-io/` in this item's
   diff, with GLOBAL-06 and every driver's native validation, and would give
   the Rust relay a target that moved while it was being written down.

2. **Repository layout: the `specs-migration` layout, extended.**

   ```
   buildwarden-specs/
     README.md                       index: each specification, its suite, its implementations
     CHANGELOG.md                    one entry per release, by specification
     relay-claim/v1.md               seam 1
     relay-claim/v1.config.schema.json     the claim's request body
     relay-claim/v1.identity.schema.json   the claim's reply
     collector/v1.md                 seam 2, in two parts (a10)
     collector/v1.complete.schema.json     the completion signal
     relay-guest-api/v1.md           seam 3
     executor-initialize/v1.md       seam 5
     bootstrap-config/v1.md          seam 6
     bootstrap-config/v1.schema.json
     conformance/                    the harness and the suites (Decision 5)
     go.mod                          module github.com/buildwarden/buildwarden-specs
   ```

   One directory per specification, one `v<N>.md` per major version, machine
   schemas beside the document they belong to. The extension over RELAY-05's
   layout: a directory may hold more than one schema for a version, named
   `v<N>.<part>.schema.json`, because a protocol has several JSON bodies where
   a ledger metadata schema has one. The directory names `build-ledger/`,
   `metadata-<name>/` and `environment-<type>/` are reserved for seam 4 (a1).
   *Rejected:* one directory per JSON body (`relay-claim-config/`,
   `relay-claim-identity/`), which scatters one protocol over several
   directories; an OpenAPI tree, see Decision 3.

3. **Document format: Markdown prose with numbered requirements, and JSON
   Schema for JSON bodies.** Each normative statement uses the RFC 2119 words
   and has a stable identifier (`CLAIM-7`, `COLL-A-3`, `GUEST-12`, `INIT-4`,
   `BOOTCFG-2`). Every document has the same sections: scope and roles;
   transport; messages and fields; ordering; errors; limits; versioning;
   conformance (which role a conforming implementation may claim, and the
   requirements of that role). A suite case names the requirements it checks,
   and the specs repository's CI fails when a requirement is neither covered
   by a case nor listed, with a reason, as not machine-checked. *Rejected:*
   OpenAPI for the HTTP seams. It cannot state the tunnel upgrade, HTTP
   trailers, DNS and DHCP answers or ordering between endpoints, so prose
   would still carry the contract and the two would drift. *Rejected:* prose
   with no identifiers, which leaves "is this sentence tested" to a reader.

4. **Versioning: a major version per document, and semver tags for the
   repository.** A document's major version is its file name, as RELAY-05
   has it for the ledger: `v1.md` is edited in place before 1.0 and a breaking
   change after 1.0 adds `v2.md` beside it. The repository is released as a
   whole, `v0.<minor>.<patch>`: documents, schemas, harness and suites move
   together, so a tag names a consistent set. Before 1.0 a minor release may
   make a passing implementation fail (a new or changed requirement or
   case), and a patch release may not (wording, harness fixes that do not
   change a verdict). `CHANGELOG.md` lists changes by specification. The first
   release is `v0.1.0`. *Rejected:* a tag per specification
   (`relay-claim/v1.3.0`): the harness spans seams, a relay implements four of
   them, and every consumer would pin four or five versions that were never
   tested as a set. *Rejected:* releasing the harness on its own version:
   a suite and the text it checks would be free to disagree.

5. **The harness is Go, lives in the specs repository, and does not import
   the kit.** `conformance/` is part of the module
   `github.com/buildwarden/buildwarden-specs`. It implements the test side of
   each protocol from the specification: a claimant and collector to test a
   relay, a relay to test a collector, a build environment, an upstream
   resolver and origin servers, and a ledger verifier written from
   `build-ledger/v1.md`. The module must not require
   `github.com/buildwarden/buildwarden`; its CI checks `go.mod` for it.
   Sharing `relaycfg`, `collector` or `ledger` with the Go relay would make
   the suite agree with that relay's bugs. Each release publishes the module
   tag and, for consumers without a Go toolchain, static `bw-conformance`
   binaries for linux, darwin and windows on amd64 and arm64 with a
   `SHA256SUMS` file. *Rejected:* Rust. The first implementation under test
   and the loop that will write the harness are Go, and a Rust consumer runs a
   binary either way. *Rejected:* a declarative HTTP test tool or shell: the
   suites need HTTP/2 inside an upgraded connection, request trailers, DNS,
   DHCP, raw Ethernet frames and ledger verification. *Rejected:* a separate
   harness repository: one more repository to create (a2), and Decision 4's
   reason. *Rejected:* Go tests in this repository (the umbrella's Decision 3
   already rejects them).

6. **A component under test is described by a target file.** The harness
   never guesses how to start a relay or a collector. A target file (TOML)
   names the role under test, a launcher, and the attachment points the
   suites need (**What a relay under test must provide**, **What a collector
   under test must provide**). This item builds three launchers: `exec` (a
   binary), `oci` (an image reference, run with docker or podman) and
   `external` (already running; addresses only). A launcher for VM images is
   specified by the same attachment points and built by the first item that
   has a VM image to test (a6). *Rejected:* flags on the harness for each
   form (`--relay-binary`, `--relay-image`): each relay form needs its own
   launch wiring (a mode flag, an fd number, a token variable), which is the
   implementation's, not the harness's, and belongs in a file kept beside the
   implementation.

7. **This repository pins a release in `tools/conformance/go.mod`.** A small
   nested module, like `tools/tier1`, requires
   `github.com/buildwarden/buildwarden-specs` at one version. `go run` builds
   the harness from that source, the Go checksum database verifies it, and the
   kit's own `go.mod` gains no dependency. That file is the seams' contract
   path for tier 1 (**Tier 1's seam list**): a change to a seam's code must
   move the pin, which it can only do after the specification changed and was
   released. On a PR the pin must be a release tag; a pseudo-version (a
   commit of an unmerged specs branch) is for work in progress and fails the
   `conformance` workflow. *Rejected:* a git submodule (tier 1 reads files
   with `git show`, which does not see into a submodule; worktrees and CI
   checkouts each need extra steps). *Rejected:* a vendored copy of the
   documents, which is the duplication the specs repository exists to remove.
   *Rejected:* a plain lock file and a download script, which would
   reimplement what the module system already verifies.

8. **The work happens in two repositories from the start of Implement.** The
   documents, harness and suites are written on a branch `feat/seam-specs` of
   the specs repository and reach its `main` through a pull request Jeff
   merges. This repository's branch holds the pin, the target files for the
   Go relay and the disk collector, the `conformance` workflow, the book
   links, tier 1's seam list and the rule changes. The loop's mechanical
   checks keep working through the pin: this repository's `conformance`
   workflow cannot pass unless the pinned specs commit exists and its suites
   pass against the Go relay and the disk collector (**How the loop treats the
   second repository**). *Rejected:* writing everything in a staging directory
   on this branch and exporting it at Publish. It keeps the loop in one
   repository, but the branch that Review reads would not be the branch the PR
   merges (the staging tree has to be deleted and the pin switched after
   Review), and the relay repository could read no draft until Publish.
   *Rejected:* waiting for P1, which the umbrella's order rules out: S1 and
   P1 are both wave 1.

9. **Seam 4 moves in a later item of phase S1, not this one** (a1).

10. **Suites for the relay and the collector now; the executor's suite with
    S2** (a11). Seams 5 and 6 get their specifications here. What the relay
    and the collector do for them (serve the bootstrap config byte for byte
    and record it; serve the endpoints `initialize` calls) is in the relay
    and collector suites. A suite that runs `warden-io initialize` itself is
    left to S2's item, which changes what `initialize` does.

11. **No Go package outside `tools/` changes.** The package comments of
    `relaycfg` and `collector` keep their protocol text for now. Pointing them
    at the specifications is a comment-only edit to shared packages that
    would declare every driver (FLOW-02); it is left to the next item that
    changes those packages.

## Questions

### Blocking

None. The repository's creation is Jeff's (umbrella a11); it is needed when Implement
starts, not for this design, and a2 says how it is asked for.

### Assumed

Each is confirmed in the design review (FLOW-19). The same entries are in the
queue file.

- **a1. Seam 4's move.** The umbrella's seam table says the ledger format
  "moves to the specs repository" and gives no phase. Options: (A) a later
  item of phase S1, filed when this one's release mechanism has run once;
  (B) in this item; (C) at M6, with RELAY-01 to RELAY-08. *Proceeding with
  A; confirm.* The move rewrites RELAY-05, tier 1's `schema-doc` root, the
  book's `specs/` mirror and its redirects, `specs/specs_test.go` and the
  `$id` policy for new schemas, each of which a third party can see. It is a
  move with no change in behaviour (FLOW-20), better reviewed alone, and it
  can use the pin and the embedded documents this item proves. Until then the
  specs repository's README links to `specs/` here, and the harness's ledger
  verifier is written from `specs/build-ledger/v1.md`. Consequence: RELAY-05
  is not changed here, although the umbrella's table lists it beside TEST-05
  for S1 (Decision 18: it changes in the item that makes the new wording
  true).
- **a2. When the repository is created, by whom, and what has to exist.**
  Options: (A) in or right after the design review, by Jeff or by that
  attended session on his word (umbrella a11); (B) now, before the review; (C) at
  Publish, after incubating on this branch (Decision 8's rejected
  alternative). *Proceeding with A; confirm.* What has to exist before
  `f3-implement` starts is listed under **What Jeff creates**. If it does not
  exist then, Implement stops and the tick records a blocking question for
  the human, as `build-vm-busybox` q1 did. No agent creates it unasked.
- **a3. Visibility.** Options: (A) public from creation; (B) private until
  `buildwarden-relay` goes public at M1. *Proceeding with A; confirm.* It
  specifies protocols of a public project, the book links to it, and the Go
  module proxy and checksum database serve only public modules: a private
  repository would need credentials in this repository's CI and in the
  relay's.
- **a4. Which loop rules bind work in the specs repository before P1.**
  The umbrella says program items run in this repository's queue under the
  current rules until P1. Options: (A) the item's queue file, design and
  reviews stay here; in the specs repository agents work only on
  `feat/seam-specs`, never push `main`, force-push or delete a branch
  (FLOW-16, FLOW-17); Jeff merges its pull request (FLOW-08) and pushes the
  release tag, or an attended session does on his word; sessions that commit
  there are recorded to the private sessions repository and the public
  repository carries no `entire/` refs (FLOW-18); the harness's Go code
  follows GLOBAL-01 to GLOBAL-04, GLOBAL-07, GLOBAL-09 to GLOBAL-11, TEST-04
  and TEST-07; (B) no rules there until P1 writes them. *Proceeding with A;
  confirm.* P1's Loop Contract replaces this paragraph when it lands.
- **a5. Harness language and form:** Go, in the specs repository, importing
  nothing from the kit, released as a module tag plus static binaries
  (Decision 5). *Confirm.*
- **a6. Launchers built now.** Options: (A) `exec`, `oci` and `external`
  now; the VM-image launcher (qemu first) is built in the specs repository
  by the relay's M2 or M3 item, or by seam 8's item, whichever first has an
  image and a CI that boots it; (B) build a qemu launcher here and validate
  it on `qemu-any`. *Proceeding with A; confirm.* The Go relay's VM form adds
  no relay code to `-mode vm`, which the `exec` launcher covers in a network
  sandbox. What a VM adds is its init and boot channel
  (`tools/relay-vm/init`, the relay seed), which belong to seam 8 (relay
  release forms), not to seams 1 to 3.
- **a7. The pin** is `tools/conformance/go.mod`, a release tag on any PR
  (Decision 7). *Confirm.*
- **a8. Versioning and release scheme** (Decision 4): `v<N>.md` per
  document, `v0.x.y` tags for the repository, the first release `v0.1.0`,
  cut by Jeff or by an attended session on his word. *Confirm.*
- **a9. Document format** (Decision 3): Markdown, requirement identifiers,
  JSON Schema 2020-12 for JSON bodies, no OpenAPI. *Confirm.*
- **a10. The collector specification is one document in two parts**, A
  (outputs a collector stores) and B (inputs and signals it serves), each
  with its own requirement prefix and its own suite, so an implementation can
  claim one part. Options: (A) one document; (B) two documents
  (`collector-outputs/`, `collector-inputs/`). *Proceeding with A; confirm.*
  The parts share transport, authentication and path rules, which one
  document states once. Nothing on the wire changes: the claim still names
  one `collector_url`.
- **a11. The executor's own suite is deferred to S2's item.** Options: (A)
  defer: S2 moves truststore generation out of `initialize`, so a suite
  written now would pin the behaviour S2 removes; (B) build it here, for
  Linux guests in a container, with Windows and macOS guests as tracked
  gaps. *Proceeding with A; confirm.* Until then EXEC-04's conformance stays
  the tests in `cmd/warden-io/`, and the amended TEST-05 says so.
- **a12. Divergences are recorded, not fixed** (Decision 1). The list is
  under **Divergences found**; the review may turn any of them into a
  follow-up item or into a change to this design. *Confirm.*
- **a13. Seam 6's name is `bootstrap-config`**, and GLOBAL-08's list of
  versioned seams gains it. The umbrella's seam table gives seam 6 no name,
  and its front-matter lists `bootstrap-model`, which is the BOOT-07 model,
  not this document. *Confirm.*
- **a14. Native validation:** nothing blocking, two tracked gaps
  (**Validation plan**). *Confirm.*
