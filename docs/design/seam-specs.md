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
  - {host: qemu-any, what: "relay suite against the Go relay VM image; deferred until a VM launcher exists"}
rules: [GLOBAL-01, GLOBAL-02, GLOBAL-03, GLOBAL-04, GLOBAL-06, GLOBAL-07, GLOBAL-08, GLOBAL-09,
        GLOBAL-10, GLOBAL-11, RELAY-02, RELAY-04, RELAY-05, RELAY-07, RELAY-08, EXEC-02,
        EXEC-03, EXEC-04, BOOT-02, BOOT-03, BOOT-04, BOOT-05, BOOT-06, BOOT-07, TEST-02,
        TEST-03, TEST-04, TEST-05, TEST-07, LINT-02, LINT-05, UX-01, FLOW-01, FLOW-02,
        FLOW-03, FLOW-04, FLOW-06, FLOW-07, FLOW-08, FLOW-09, FLOW-10, FLOW-11, FLOW-12,
        FLOW-14, FLOW-15, FLOW-16, FLOW-17, FLOW-18, FLOW-19, FLOW-20]
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

Five of the seams between BuildWarden's components have no specification.
Each is defined by Go code:

| # | Seam | Where it is defined today |
|---|---|---|
| 1 | Claim protocol (`relay-claim`) | `relaycfg/` and its package comment; what each relay mode does with a claim is in `cmd/relay/` |
| 2 | Collector protocol (`collector`) | `collector/` and its package comment; the relay's side is `relay/collector_client.go` |
| 3 | Relay guest API (`relay-guest-api`) | `relay/relay.go` (`handleArtifacts`, `handleContextGet`), `relay/controlplane.go`, `relay/dns.go`, `relay/heartbeat.go`, `cmd/relay/dhcp.go` |
| 5 | `initialize` contract (`executor-initialize`) | `cmd/warden-io/main.go`, `initialize.go`, `baseline.go`, `platform_<os>.go` |
| 6 | Bootstrap config (`bootstrap-config`) | `bootstrap/bootstrap.go` |

The book describes two of them. `docs/book/src/concepts/architecture.md` has
the collector's endpoint table, the claim's steps, `build_link`'s validation
and its by-mode table, and the ordering rule, and #115 updated it. It is an
explanation for the book's readers: it leaves out replies, limits and error
cases, nothing checks it against the code, and no implementation can be
tested against it.

Three things follow.

- The Rust relay (`buildwarden/buildwarden-relay`) has to implement seams 1,
  2, 3 and 6 and has nothing to implement them from but another
  implementation. Its milestone M1 takes its conformance harness from this
  item.
- GLOBAL-08 and EXEC-04 say a change to a versioned seam also changes the
  seam's contract document, and tier 1's `seam-contract` check enforces that
  for the seams `rules.yaml` lists. The list is empty, because there are no
  documents. PR #115 changed the claim (`upstream_dns`, the claim token on
  Hyper-V) and updated the book page; there was no contract document to
  update and no check that asked for one.
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

One design is not on `main` and so is not in `related`: `driver-contract`
(K1, the other wave-1 item of track K, in Design on `feat/driver-contract`).
It also amends GLOBAL-08 and adds an entry to tier 1's seam list, whose
contract is a document in this repository. **Rule changes** says how the two
amendments stay apart.

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
   a ledger metadata schema has one. A schema's `$id` is a name that mirrors
   its path in the specs repository,
   `https://github.com/buildwarden/buildwarden-specs/<directory>/v<N>.<part>.schema.json`
   (RELAY-05's schema-identity rule, applied to the new repository), and does
   not change once released. The directory names `build-ledger/`,
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
   resolver and origin servers, and a ledger verifier written from this
   repository's `specs/build-ledger/v1.md`. The verifier checks the header,
   every record's signature and chain, and the hash blocks, and reads the
   metadata fields its cases need by the names `specs/metadata-<name>/v1.md`
   gives them. It holds no copy of a schema file from `specs/`; validating
   metadata against those schemas stays with `specs/specs_test.go` here until
   seam 4 moves (a1). The module must not require
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
   binary), `oci` (an image reference, run by the container runtime the
   target file names) and
   `external` (already running; addresses only). A launcher for VM images is
   specified by the same attachment points and built by the first item that
   has a VM image to test (a6). *Rejected:* flags on the harness for each
   form (`--relay-binary`, `--relay-image`): each relay form needs its own
   launch wiring (a mode flag, an fd number, a token variable), which is the
   implementation's, not the harness's, and belongs in a file kept beside the
   implementation.

7. **This repository pins a release in `tools/conformance/go.mod`.** A small
   nested module, like `tools/tier1`, requires
   `github.com/buildwarden/buildwarden-specs` at one version and names the
   harness in a `tool` directive. The directive keeps the requirement through
   `go mod tidy` although the module holds no Go code, and
   `go tool bw-conformance` builds the harness from that source. The Go
   checksum database verifies it, and the kit's own `go.mod` gains no
   dependency. That file is the contract path tier 1 checks for the five
   seams this item lists (**Tier 1's seam list**): a change to their code
   must move the pin, which it can only do after the specification changed
   and was released. On a pull request and on `main` the pin must be a
   release tag. A pseudo-version (a commit of an unmerged specs branch) is
   for work in progress: the `conformance` workflow runs the suites with it
   on a branch push and fails it on a pull request. One command moves the
   pin, `make pin-specs REF=<tag or commit>`: it rewrites the requirement,
   `go.sum` and the one book page that links into the specs repository
   (**The book and the specifications link both ways**). *Rejected:* a git
   submodule (tier 1 reads files
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
   pass against the Go relay and the disk collector, first on a push of this
   branch during Implement and again on the pull request (**How the loop
   treats the second repository**). *Rejected:* writing everything in a
   staging directory
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
    left to S2's item, which changes what `initialize` does. Until it exists
    the executor's seam files are held by tier 1's pin check and by the Go
    tests in `cmd/warden-io/`, not by a suite (**Tier 1's seam list**).

11. **No package of the relay, the collector, the executor or a driver
    changes.** The package comments of
    `relaycfg` and `collector` keep their protocol text for now. Pointing them
    at the specifications is a comment-only edit to shared packages that
    would declare every driver (FLOW-02); it is left to the next item that
    changes those packages.

12. **The release and the pin's move to it are one human step, after
    Review** (a15). Nothing is merged or tagged in the specs repository
    until the suites have passed on hosted runners for this branch and tier 2
    has read the specs branch. Then `f6-release-specs`: Jeff reads the tier-2
    report and merges the specs pull request, and the tag is pushed and the
    pin moved on this branch, by him or by the attended session on his word.
    *Rejected:* `f6-open-pr` moves the pin. Its definition in
    `.claude/agents/wf-publish.md` has no such duty, and a feature cannot
    change `.claude/` (FLOW-14). *Rejected:* releasing before Review and
    moving the pin in Implement. The specs content would be merged and
    tagged before tier 2 had read it, and a finding would cost a second
    release.

13. **The collector specification is normative for the wire; the disk
    layout is the disk collector's** (a16). Part A requires what a collector
    accepts, what it replies and in what order. The layout `warden inspect`
    reads is specified as the disk collector's, in a section other collectors
    need not meet. *Rejected:* an abstract object model for any collector's
    storage. Nothing on `main` defines one, and the umbrella's review put the
    collector's storage boundary off to the first alternative collector (its
    a2).

## Questions

### Blocking

None. The repository's creation is Jeff's (umbrella a11); it is needed
when Implement starts, not for this design, and a2 says how it is asked for.
The release at the end is Jeff's too, and is a queue step, not a question
(a15).

### Assumed

Each is confirmed in the design review (FLOW-19). The same entries are in the
queue file.

- **a1. Seam 4's move.** The umbrella's seam table says the ledger format
  "moves to the specs repository" and gives no phase. Options: (A) a later
  item of phase S1, filed when this one's release mechanism has run once;
  (B) in this item; (C) at M6, with RELAY-01 to RELAY-08. *Proceeding with
  A; confirm.* The move rewrites RELAY-05, tier 1's `schema-doc` root, the
  book's `specs/` mirror and its redirects, `specs/specs_test.go` and the
  `$id` policy for new ledger schemas, each of which a third party can see. It is a
  move with no change in behaviour (FLOW-20), better reviewed alone, and it
  can use the pin and the release steps this item proves. Until then the
  specs repository's README links to `specs/` here, and the harness's ledger
  verifier is written from `specs/build-ledger/v1.md`. Consequence: RELAY-05
  is not changed here, although the umbrella's table lists it beside TEST-05
  for S1 (the umbrella's Decision 18: a rule changes in the item that makes
  the new wording true).
- **a2. When the repository is created, by whom, and what has to exist.**
  Options: (A) in or right after the design review, by Jeff or by that
  attended session on his word (umbrella a11); (B) now, before the review;
  (C) at Publish, after incubating on this branch (Decision 8's rejected
  alternative). *Proceeding with A; confirm.* What has to exist before
  `f3-implement` starts is listed under **What Jeff creates**. If it does not
  exist then, Implement stops and the tick records a blocking question for
  the human, as `build-vm-busybox` q1 did. No agent creates it unasked,
  and which licence files it carries is Jeff's choice (**What Jeff
  creates**).
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
- **a5. Harness language, home and form** (Decision 5). Options: (A) Go, in
  the specs repository under `conformance/`, importing nothing from the kit,
  released as a module tag plus static `bw-conformance` binaries with
  checksums; (B) Rust; (C) a separate harness repository; (D) Go tests in
  this repository that import the kit, which the umbrella's Decision 3
  rejects. *Proceeding with A; confirm.*
- **a6. Launchers built now.** Options: (A) `exec`, `oci` and `external`
  now; the VM-image launcher (qemu first) is built in the specs repository
  by the relay's M2 or M3 item, or by seam 8's item, whichever first has an
  image and a CI that boots it; (B) build a qemu launcher here and validate
  it on `qemu-any`. *Proceeding with A; confirm.* The Go relay's VM form adds
  no relay code to `-mode vm`, which the `exec` launcher covers in a network
  sandbox. What a VM adds is its init and boot channel
  (`tools/relay-vm/init`, the relay seed), which belong to seam 8 (relay
  release forms), not to seams 1 to 3.
- **a7. How this repository pins a specs release** (Decision 7). Options:
  (A) `tools/conformance/go.mod`, a nested module that requires the specs
  module at one version: a release tag on a pull request and on `main`, a
  pseudo-version only on a branch while work is in progress; the file is
  tier 1's contract path for the five seams; (B) a git submodule; (C) a
  vendored copy of the documents; (D) a lock file and a download script.
  *Proceeding with A; confirm.*
- **a8. Versioning and release scheme** (Decision 4). Options: (A) a major
  version per document (`v<N>.md`, edited in place before 1.0) and semver
  tags `v0.x.y` for the repository as a whole; a minor release may make a
  passing implementation fail, a patch release may not; the first release is
  `v0.1.0`; (B) a tag per specification; (C) the harness versioned apart
  from the documents. *Proceeding with A; confirm.*
- **a9. Document format** (Decision 3). Options: (A) Markdown prose with RFC
  2119 words and a stable identifier per requirement that suite cases cite,
  plus JSON Schema 2020-12 for JSON bodies; the specs repository's CI fails
  on a requirement with no case and no stated reason; (B) OpenAPI for the
  HTTP seams; (C) prose without requirement identifiers. *Proceeding with A;
  confirm.*
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
  the tests in `cmd/warden-io/`, and the amended TEST-05 says so. The
  umbrella says seams 1 to 8 each get a suite, and its S2 row does not name
  this one. With A, seam 5 has no suite when S1 is done, and S2's intake has
  to carry it: it is on the list of work owed after the merge
  (**Implementation plan**).
- **a12. What to do with the nineteen divergences** (D1 to D19,
  **Divergences found**; Decision 1). Options: (A) record each in its
  specification as the code behaves and list it under "Known divergences";
  fix none here; the review may turn any into a follow-up item, and says
  what kind of item it is under FLOW-20; (B) fix them in this item, which
  puts `relay/`, `collector/` and `cmd/warden-io/` in the diff, with
  GLOBAL-06 and every driver's native validation. *Proceeding with A;
  confirm.*
- **a13. Seam 6's name.** The umbrella's seam table gives seam 6 no name.
  Options: (A) `bootstrap-config`, added to GLOBAL-08's executor bullet and
  to tier 1's seam list; (B) `bootstrap-model`, which the umbrella's
  front-matter lists and which is also the name of the BOOT-07 model.
  *Proceeding with A; confirm.*
- **a14. Native validation** (**Validation plan**). Options: (A) nothing
  blocking; two tracked gaps: `macos-vz`, open, and `qemu-any`, deferred
  until a VM launcher exists; (B) block on `macos-vz`; (C) record no gaps.
  *Proceeding with A; confirm.*
- **a15. The release step, and who moves the pin** (Decision 12). FLOW-04
  lists what a `human` step is for (blocking questions, the design review,
  escalations), and a release in a second repository is none of those.
  `f6-open-pr`'s definition has no pin move, and a feature cannot change it.
  Options: (A) a `human` queue step `f6-release-specs` in phase 6. It covers
  the merge of the specs pull request, the tag, and the pin's move on this
  branch (`make pin-specs`, `make test`, `make lint`, push), done by Jeff or
  by the attended session on his word. `wf` lists a human step for Jeff from
  the day it is added; this one is not to be done before Review is done, and
  its note says so. (B) No queue step: `f6-open-pr` opens the PR on the
  pseudo-version, the `pin` job fails, and FLOW-11's escalation asks Jeff
  for the release; the PR is red until then. (C) `f6-open-pr`'s session
  moves the pin as its first act, on this design's word. (D) A process item
  first, to teach the loop about a release in a second repository; that is
  P1's subject and would hold S1 behind it. *Proceeding with A; confirm.*
- **a16. What Part A of the collector specification is normative for**
  (Decision 13). Options: (A) the wire only: the requests a collector
  accepts, its replies and their order. The disk layout (`artifacts/<name>`,
  `payloads/<hash>`, `captures/<name>`, `ledger`, `environment.json`,
  `build-output.log`) is specified as the disk collector's, and a way to
  read stored bytes back is something a target file may give the harness,
  not a requirement on a collector. (B) An object model for any collector:
  each output is an object a reader can fetch by its name with the bytes
  sent. *Proceeding with A; confirm.* The umbrella's review (its a2) decided
  that the collector gets no storage boundary in this program and that one
  is designed with the first alternative collector. B would decide part of
  it here.

## Design

Nothing in the relay, the collector, the executor or any driver changes. The
bootstrap model (BOOT-07) is untouched: its diagram, invariants, mapping and
deviation register stand, and this design carries no Bootstrap comparison.

### What each specification covers

The lists below are what the documents must state, taken from `main` at
`b903f65`. The file named in each heading is where the behaviour is defined
today. A specification may say more; it may not say less, and where it would
differ from this list the code decides (Decision 1).

#### Seam 1, `relay-claim/v1.md` (`relaycfg/`, `cmd/relay/main.go`, `vmlink.go`, `mode_host.go`)

Roles: the *relay* serves one claim; the *claimant* (a driver's session, or
`cmd/collector`) pushes it.

- **Transport.** HTTP/1.1 over a connection the claimant opens to the
  relay's claim endpoint. Today that is TCP, port 8301 by default
  (`relaycfg.DefaultPort`, the Go relay's `-claim-addr`). The relay never
  dials the claimant. S4 changes what the connection is; the specification
  keeps "a byte stream the claimant opens" apart from "TCP" so that S4 edits
  one section.
- **Authorization.** Both endpoints take `POST` only; anything else is 405
  `POST only`, decided before the token is looked at. A relay started with a
  claim token requires it in the `Authorization` header and answers 401
  `unauthorized` otherwise, comparing in constant time; a 401 does not consume
  the claim. A claimant sends `Authorization: Bearer <token>`; the relay also
  takes the token with no scheme (D8). A relay started without a token
  accepts any request. The token
  is a pre-claim value: it reaches the relay on its boot channel (RELAY-08,
  BOOT-06; `WARDEN_CLAIM_TOKEN` for the Go binary), never in the claim.
- **`POST /v1/tunnel`.** The claimant sends `Connection: Upgrade`,
  `Upgrade: warden-tunnel` and `Content-Length: 0`. The relay answers
  `101 Switching Protocols` with the same two headers. From then on the
  connection carries HTTP/2 with prior knowledge and no TLS, with the roles
  reversed: the relay is the HTTP/2 client and the claimant serves the
  collector protocol on it. One tunnel per relay: a second request, or one
  made while a claim is being processed or after one succeeded, is 409
  `tunnel already open`. The tunnel
  outlives a refused claim.
- **`POST /v1/claim`.** A JSON body of at most 16 MiB; an unknown field is
  refused. Fields:

  | Field | Type | Meaning and validation |
  |---|---|---|
  | `collector_url` | string, required | `http://collector.tunnel` selects the open tunnel; any other value is a base URL the relay dials itself. A value the relay cannot use, or a collector it cannot reach or that refuses the ledger stream, fails the relay's start (D6) |
  | `collector_token` | string | Sent as the bearer token on every collector request; empty sends none |
  | `capture_mode` | string, optional | `""`, `none`, `headers`, `bodies`, `all` |
  | `upstream_ca_certs` | array of PEM strings, optional | Extra roots for the relay's upstream TLS. An entry that holds no PEM certificate is ignored (D5) |
  | `upstream_system_ca` | boolean, optional | `false` leaves the system roots out, and only together with `upstream_ca_certs` |
  | `environment` | object, optional | `type` (optional), `payload` and `metadata` (both base64, both required). The payload is at most 1 MiB and is a JSON object that names its own type: a member `type` that is a non-empty string, or the shape of an OCI or Docker image manifest or index, whose type is `oci-image/v1`. `type`, when given, must equal it. The metadata decodes as a CBOR map |
  | `build_link` | object, optional | `prefix`, `relay`, `guest`: IPv4; the prefix is its own network address, length 8 to 30; relay and guest are distinct hosts inside it, neither its network nor its broadcast address; nothing in 0.0.0.0/8, loopback, link-local, multicast or class E |
  | `upstream_dns` | array of strings, optional | When present: not empty; each an IPv4 address (port 53 implied), not unspecified, multicast or broadcast, and not inside `build_link` |

- **Replies.** 200 with `application/json`:
  `{"ca_cert": <PEM>, "ledger_public_key": <hex Ed25519 key>}`. 400
  `invalid claim: ...` for a body that does not decode (the text of an
  unknown field is `unknown field "<name>"`, which a claimant reads to
  recognise an older relay), an invalid `build_link` or `upstream_dns`, a
  missing `collector_url`, or the tunnel selected with none open; 400 also
  for a claim the relay's mode refuses (`claim refused: ...`). After a 400
  the relay stays claimable, with one exception in host mode (D17). 409
  `already claimed` while another claim is being processed or after one
  succeeded. 500 `relay start failed: ...` when the relay could not start
  with a claim it accepted, an unusable `environment` or `collector_url`
  among the causes (D6); that relay is spent.
- **One claim.** After a 200 the relay stops accepting connections on the
  claim endpoint. The tunnel stays open for the build.
- **Ordering.** The relay replies 200 only after it has configured its build
  segment (address and DHCP responder, where it serves one), opened the
  ledger stream, landed the environment payload, created its CA, begun
  starting its build-facing listeners and sent `POST /v1/ready` (a failed
  `ready` is logged and does not fail the claim). A host-mode relay has its
  listeners before it replies. A vm- or container-mode relay may still be
  binding its ports when the claim returns, and a port it then fails to bind
  ends the relay after the 200 (D19). A claimant may start the build
  environment as soon as the claim returns; the build environment waits for
  `GET /health` on port 8300 to answer before its first request, as
  `initialize` does.
- **By mode (RELAY-04).** `build_link`: a vm-mode relay replaces the build
  interface's IPv4 addresses with `relay/len`, serves the guest's lease there
  and refuses a link that collides with another interface's address or
  route; without the field it keeps the address already on the interface, and
  refuses the claim if there is none. A host-mode relay builds its netstack
  on the link; without the field it uses its `--subnet`. A container-mode
  relay refuses a claim that carries one. `upstream_dns` absent: the usable
  nameservers of the relay's own `/etc/resolv.conf`, excluding its own
  address, 127.0.0.1 and the build link; none usable refuses the claim
  (in host mode after the relay has built its netstack, D17).
  A relay that has a build link (vm and host mode, with or without the
  field) refuses upstream dials into it.
- **The claimant's side.** It opens the tunnel before the claim when it
  selects the tunnel, retries connection failures (the kit retries every
  250 ms inside a 90 s limit; the numbers are the kit's), treats any reply
  other than 101 or 200 as final, and refuses an identity with no `ca_cert`.

#### Seam 2, `collector/v1.md` (`collector/`, `relay/collector_client.go`)

Roles: the *collector* serves; the *relay* is its only client.

Common to both parts: HTTP, over the tunnel or to a `collector_url`. When the
collector has a token, every `/v1/` endpoint except `/v1/status` requires it
in the `Authorization` header and answers 401 `unauthorized` without it;
`/healthz` and `/v1/status` never do. The relay sends
`Authorization: Bearer <token>`; the collector also takes the token with no
scheme (D8). Names: an artifact name is a relative
slash path of at most 255 bytes whose segments use `[A-Za-z0-9._-]` and start
and end with an alphanumeric; a context path is the same except that a
segment may start with `.`, `-` or `_`; a capture name is made safe by
replacing every other byte with `_`. A successful write is 200 with an empty
body; a body over a limit is 413.

**Part A, outputs a collector stores**

| Endpoint | Stored as | Notes |
|---|---|---|
| `PUT /v1/artifacts/<name>` | `artifacts/<name>` | Streamed. 400 for an unsafe name. Limit 4 GiB by default. A failed upload leaves nothing |
| `PUT /v1/captures/<name>` | `payloads/<hash>`, and `captures/<name>` referring to it | Chunked, with the trailer `Warden-Hash`: 64 hex digits, the BLAKE2b-256 of the body as the relay computed it. Identical content is stored once. The disk collector makes `captures/<name>` a symlink to `../payloads/<hash>`, or a copy where it cannot create one |
| `PUT /v1/ledger` | `ledger` | One request for the whole build, written as it arrives. If the stream breaks, what arrived is kept: every record is signed, so a truncated ledger verifies up to its last record |
| `PUT /v1/environment` | `environment.json` | The environment record's signed payload, at most 1 MiB. `PUT` only |
| `POST /v1/output` | `build-output.log`, appended | Chunks in arrival order; boundaries carry no meaning |

Part A is normative for the wire: what a collector accepts, what it replies
and in what order (a16). The "Stored as" column is the disk collector's
layout, the one `warden inspect` reads. The specification gives it a section
of its own, which a collector that is not a directory need not meet. The
disk collector's directory may also hold files that are not outputs: the
kit's session writes the relay's CA there as `ca.cert.pem`.

**Part B, inputs and signals a collector serves**

| Endpoint | Reply |
|---|---|
| `GET /v1/build-script` | 200, `application/octet-stream`; 404 when the build has none |
| `GET /v1/bootstrap` | 200, `application/json`, the bootstrap config (seam 6); 404 when the build has none |
| `GET /v1/context/<path>` | A file: 200 with `Warden-Context: file`. A directory, the root, or a directory with a trailing slash: 200 with `Warden-Context: list` and `text/plain`, the sorted safe file paths beneath it, relative to the context root, one per line. 403 for an unsafe path; 404 for a missing one, for no context, and for a file named with a trailing slash. A symlink that leaves the context is neither served nor listed |
| `POST /v1/ready` | 200. The relay is up |
| `POST /v1/heartbeat` | 200. The build is alive |
| `POST /v1/complete` | 200. Body `{"exit_code": int, "message": string, "error": string}`; 400 for a body that is not JSON. The first completion is the build's result. A collector may close the connection once it has the completion, before the reply (D16) |
| `GET /v1/status` | The first completion plus `received_at`, or `{"state":"running"}`. A relay never calls it; it can be reached only where a collector listens, which the disk collector does not |
| `GET /healthz` | 200 `ok`. As `/v1/status`: not used by a relay |

**A collector's lifetime.** One collector serves one build. It may give up
on a relay whose `ready` does not arrive (the kit's session waits 90 s from
the start of the claim) and, once a first heartbeat has arrived, on a build
whose heartbeats stop (the kit: 30 s). Both limits are the collector's own.
Completion is terminal: after `POST /v1/complete` the collector owes the
relay nothing more and may close the connection at once. A relay therefore
delivers the ledger and every other output before it sends the completion,
and does not depend on the reply. What a collector does next is its own:
`cmd/collector` exits, with status 0 for `exit_code` 0 and 1 otherwise.

**The relay's side**, which the relay suite checks: `PUT /v1/ledger` is
opened before any other output; at completion the relay ends that request
and waits for its reply before it sends `POST /v1/complete`;
`PUT /v1/environment` precedes the environment record; `ready` is sent
before the claim is answered (seam 1, Ordering); a heartbeat is forwarded
every 2 s while the build showed activity (its own `/heartbeat`, or any
proxied request) in the last 5 s; an input that is not 200 becomes a 404 to
the build; signals accept 200 or 204, a failed signal is logged, and none
blocks the relay.

#### Seam 3, `relay-guest-api/v1.md` (`relay/relay.go`, `controlplane.go`, `dns.go`, `heartbeat.go`, `proxy.go`, `cmd/relay/dhcp.go`)

Roles: the *relay* serves; the *build environment* (the executor, and the
build's own tools) is the client. Everything here is on the build link.

- **Address.** Where the relay serves the lease (vm mode with a
  `build_link`, host mode always), it answers every DHCP DISCOVER with an
  OFFER and every REQUEST with an ACK for the link's guest address, with the
  relay as server identifier, router and DNS server, the link's mask and an
  infinite lease, echoing the request's `xid` and `chaddr`; it answers
  nothing else. It keeps no state, so the build can only ever be told its
  own address.
- **On a frame link** (the Go relay's host mode; the second network path of
  the umbrella's Decision 12, which the Rust relay's host mode and unikernel
  use). The relay is the build's whole network below IP. It answers ARP
  requests for its own address and for no other, and takes the build's MAC
  address from the first frame it receives. The Go relay also broadcasts a
  gratuitous ARP for its own address every 500 ms for the first 60 s, which
  a macOS guest needs before it treats the link as up. It accepts a TCP
  connection to any destination address and any port, and the port alone
  selects the service: 443 is TLS interception, 8300 the control plane,
  every other port the HTTP proxy. UDP is served only on port 53 of the
  relay's own address. So on a frame link the build connects to the
  addresses DNS gave it, and the relay captures the connections itself.
- **On a kernel interface** (vm and container mode; Decision 12's first
  path). The relay serves only on its own address, ports 53, 80, 443 and
  8300. Bringing the build's connections to other addresses onto those ports
  is not the relay's: the relay VM's init redirects them
  (`tools/relay-vm/init`), and the container driver rewrites them in the
  build's namespace. That part is the driver's (seam 7).
- **DNS**, port 53 (UDP; TCP as well where the relay binds kernel sockets).
  `artifacts` and `cwd` resolve to the relay's own address with TTL 0; any
  other query type for those names gets an empty answer. Every other query is
  forwarded to the relay's resolvers in list order, the next one after a 5 s
  silence; if none answers, the reply is SERVFAIL.
- **The reserved host `artifacts`** (by the request's `Host`, port ignored):

  | Request | Reply | Effect |
  |---|---|---|
  | `/heartbeat` | 200 `ok` | Marks the build alive |
  | `/exit?code=<n>` | 200 `ok` | Ends the ledger, flushes it to the collector, then `POST /v1/complete` with `exit_code` n (0 when absent). Terminal (After completion, below) |
  | `/ca.pem` | 200, `application/x-pem-file` | The relay's CA, the same bytes as the claim's `ca_cert`. Not recorded |
  | `/ca.crl` | 200, `application/pkix-crl` | The CA's empty CRL, which every leaf names. Not recorded |
  | `/build-script` | 200, `application/octet-stream`; 404 | The collector's build script, byte for byte, recorded in the ledger |
  | `/bootstrap` | 200, `application/json`; 404 | The collector's bootstrap config, byte for byte, recorded |
  | `POST /<name>` | 200 `artifact stored: <name> (<n> bytes)`; 400 unsafe name; 413 over 2 GiB; 500 when the collector fails | Streams the body to `PUT /v1/artifacts/<name>` and records an artifact; an empty name is `unnamed` |
  | anything else | 404 `not found` | |

- **The reserved host `cwd`**, `GET` only: a context file (200, recorded), a
  listing for the root, a directory or a trailing slash (200 `text/plain`,
  not recorded), 403 for an unsafe path, 404 for a missing one.
- **The control plane**, HTTP on port 8300 of the relay's address:
  `/health` (200 `ok`), `/ca.pem`, `POST /v1/output` (the body goes to the
  collector's `/v1/output`; 256 MiB in total per build, then 413) and
  `POST /v1/complete?code=<n>` (as `/exit`).
- **Every other host.** Port 80 is an HTTP/1.x proxy keyed on `Host`. Port
  443 terminates TLS with a leaf for the SNI name, signed by the relay's CA
  and naming `http://artifacts/ca.crl` as its CRL distribution point, then
  proxies the HTTP inside. The response reaches the build as the upstream
  sent it (RELAY-02). An upstream the relay cannot reach, or one it refuses
  to dial (the build link in vm and host mode; loopback, private, link-local
  and metadata addresses as well in host mode), is 502 `upstream unreachable`. How the
  build's connections reach these ports depends on the link (above).
- **After completion.** `/exit` and `/v1/complete` are terminal for the
  guest API: the ledger has ended, and the build environment sends nothing
  more. The specification leaves a relay's replies after completion
  undefined and requires only that nothing more is recorded. The Go relay
  keeps its listeners and still answers `/heartbeat`, `/ca.pem`, `/ca.crl`,
  listings and a repeated `/exit`; any request it would record (a proxied
  request, `/build-script`, `/bootstrap`, a `cwd` file, an artifact upload)
  ends its process (D18).
- **Versioning.** The API carries no version today beyond `/v1/` on two
  control-plane paths. The umbrella's proposed BOOT-02 has the executor and
  the relay "agree through the relay guest API's version"; that version is
  S2's to add with the truststore endpoints, and this document says so.

#### Seam 5, `executor-initialize/v1.md` (`cmd/warden-io/`)

Roles: the *launcher* (a driver's delivery and start, BOOT-02, BOOT-03)
invokes; the *executor* runs.

- **Invocation.** `warden-io initialize` (alias `init`) with `--gateway`
  (the relay's address), `--ip` (the build's own address with its prefix
  length), `--script` (the local name the build script is saved under, which
  picks its interpreter), `--workdir` (created and entered before the
  script) and `--hold` (stop after setup, write the environment to
  `<data dir>/shell.env` and wait). Each value flag takes `--flag=value` or
  `--flag value`. `WARDEN_GATEWAY` and `WARDEN_IP` stand in for the first two
  when the flag is absent.
- **Steps, in order,** all over the relay: point DNS at the gateway
  (per OS; `--ip` only for a launcher that does not address the NIC itself);
  poll `http://<gateway>:8300/health` every 500 ms for at most 60 s;
  `GET http://artifacts/ca.pem`; `GET http://artifacts/bootstrap` (404 is
  the empty config; any other status, or a config it cannot parse, stops);
  install the CA per the trust mode (EXEC-03); apply the baseline; enter the
  workdir; `GET http://artifacts/build-script` into the temporary directory;
  run it.
- **The baseline** (BOOT-05; not configurable): `SOURCE_DATE_EPOCH=0`,
  `UV_NATIVE_TLS=1`; the system CA bundle's path, when one is found, in
  `SSL_CERT_FILE`, `REQUESTS_CA_BUNDLE`, `CURL_CA_BUNDLE`,
  `NODE_EXTRA_CA_CERTS`, `PIP_CERT`, `CARGO_HTTP_CAINFO`,
  `NIX_SSL_CERT_FILE` and `HEX_CACERTS_PATH`; a Java truststore
  `<data dir>/certs.jks` holding the CA under the alias `warden`, password
  `changeit`, named in `MAVEN_OPTS`, `GRADLE_OPTS` and the system bazelrc.
  The executor's own directory is put first on the script's `PATH`.
- **Per guest OS:** the data directory (`/etc/warden`; `%ProgramData%\warden`
  on Windows), the default script name (`build.sh`; `build.ps1`), the
  interpreter (`/bin/bash` when it exists, else `/bin/sh`; PowerShell, or
  `cmd.exe /c` for `.cmd` and `.bat`), and the trust-store procedure.
- **While the script runs:** its stdout and stderr go to the console and, in
  chunks of at most 32 KiB, to `POST http://<gateway>:8300/v1/output`; after
  the first failed chunk the executor stops sending and says so on stderr,
  and the build continues. `GET http://artifacts/heartbeat` every 2 s.
- **Exit.** `POST http://artifacts/exit?code=<n>` with the script's exit
  code, after the last output chunk; the process then exits with that code.
- **The other subcommands**, as the executor's use of seam 3: `fetch`
  (`http://cwd/<path>`, a listing then each file for a directory), `post`
  (`POST http://artifacts/<name>`) and `trust` (the CA, appended).

#### Seam 6, `bootstrap-config/v1.md` (`bootstrap/`)

Roles: the *producer* (the claimant's collector), the *relay* (carries it),
the *executor* (reads it).

- The document: `{"version": 1, "trust": {"mode": "append" | "exclusive"}}`.
  It carries policy only: no path, file content, command or environment
  variable (BOOT-05).
- Reading is strict: an unknown field, trailing data, a version other than 1
  or an unknown mode is an error, so an executor never guesses at a newer
  config. An empty body, and a 404 from the relay, are the empty config,
  whose trust mode is `append`.
- Path: `GET /v1/bootstrap` on the collector, `GET http://artifacts/bootstrap`
  on the relay, which serves the collector's bytes unchanged and records
  them.
- What the kit's producer sends (`bootstrap.ForGuest`): `exclusive` for a
  Linux guest, `append` for Windows and macOS. That choice is the kit's
  policy, stated as such and not required of other producers.

### Divergences found

Places where the code differs from a rule, from its own comment, or from the
rest of the same protocol. Each specification records the code's behaviour
and lists the point under "Known divergences"; none is changed here (a12).

| # | Seam | What the code does | What it differs from |
|---|---|---|---|
| D1 | 2 | `GET /v1/status` is served without the token | `collector`'s package comment: "all but /healthz require Authorization" |
| D2 | 2 | A capture with a missing or malformed `Warden-Hash` trailer is 500 | Every other bad request is 4xx |
| D3 | 2 | `POST` is accepted where the protocol says `PUT` (artifacts, captures, ledger), and `PUT` where it says `POST` (`/v1/output`); `/v1/environment` alone is strict | The handlers' own "PUT only" and "POST only" replies |
| D4 | 2 | The collector does not check a capture's body against its trailer hash | Nothing written; the specification states that the hash is the relay's claim |
| D5 | 1 | An unknown `capture_mode` is accepted and means no capture. An `upstream_ca_certs` entry with no PEM certificate in it is ignored, which with `upstream_system_ca` false leaves the relay trusting no upstream | GLOBAL-10: a provided value that cannot be used is an error |
| D6 | 1 | An unusable `environment` (empty, over 1 MiB, a `type` that differs from the payload's, metadata that is not a CBOR map) or `collector_url` (not a URL the relay can request, or a collector that cannot be reached or refuses the ledger stream) fails the relay's start: 500, and the relay is spent | Every other invalid field: 400, and the relay stays claimable |
| D7 | 1 | The relay upgrades any authorized `POST /v1/tunnel`, whether or not it asks for `warden-tunnel` | The claimant always sends the upgrade headers |
| D8 | 1, 2 | Both sides accept the token with no `Bearer` scheme. The relay strips `Bearer` and any spacing after it; the collector strips exactly `Bearer ` | The header every client sends, `Authorization: Bearer <token>`; and each other |
| D9 | 3 | The fixed `artifacts` paths ignore the method: `GET /exit?code=3` ends the build | The executor uses `POST` for `/exit` |
| D10 | 3 | A request to `cwd` that is not a `GET` is proxied as if `cwd` were an upstream host | `cwd` is a reserved name |
| D11 | 3 | The `/v1/output` request that crosses the 256 MiB limit is cut short and still answered 200; the next one is 413 | A limit that is reported when it is hit |
| D12 | 3, 5 | `http://artifacts/health` does not exist (404). `initialize` with no `--gateway` and no `WARDEN_GATEWAY` polls it and gives up after 60 s. Every driver passes `--gateway` | `waitForRelay`'s no-gateway path |
| D13 | 5 | A failure before the script runs (network, relay wait, CA, bootstrap config, baseline, workdir, script fetch) exits 1 without telling the relay, so the collector receives no completion | EXEC-02: failures "produce a structured exit reported back through the relay" |
| D14 | 5 | `initialize` ignores a flag it does not know | GLOBAL-10 |
| D15 | 3 | DNS over TCP is served in vm and container mode and not in host mode, where a TCP connection to port 53 reaches the HTTP proxy | The other two modes |
| D16 | 2 | The collector records the completion before it replies, and `cmd/collector` closes the tunnel as soon as it has it. The relay can see a failed `POST /v1/complete` for a build that completed; it logs it | Part B's 200 reply |
| D17 | 1 | In host mode, the refusal for an absent `upstream_dns` with no usable nameserver in the relay's `/etc/resolv.conf` is a 400 that closes the relay's frame socket. The claim is released, but a second claim builds its netstack on a closed descriptor | "A refused claim leaves the relay claimable" (`build-link-subnet`); vm and container mode keep to it |
| D18 | 3 | After completion, a request the relay would record sends on the ledger's closed channel in a connection goroutine that has no recover, which ends the relay process | Nothing written: completion had no stated after-state |
| D19 | 1, 3 | In vm and container mode the claim's 200 and `ready` are sent once the listeners' goroutines are started, not once the ports are bound. A port that fails to bind ends the relay after the 200, and a failed bind of UDP port 53 is not reported at all | "The relay is up" (`POST /v1/ready`); host mode has its listeners first |

D17 and D18 are read from the source and have not been run. Implement
reproduces each once before the specification states it (task 8).

The suite does not assert the header's scheme (D8), sends nothing recorded
after a completion (D16, D18), gives the D17 case its own relay, and waits
for the listeners (D19); **The harness** says how.

Three of the nineteen stand against a rule, all MUST and agent-judged: D5
and D14 against GLOBAL-10, and D13 against EXEC-02. D15, D17 and D19 are
differences between the relay's modes; RELAY-04 asks a relay change to say
which modes it touches and does not require the modes to behave alike, so
they break no rule as written. D9, D13 and D18 have a security or
measurement angle (**Security note**). None takes away something `main`
once did: each dates from the change that added the code (D17 from #115's
`upstream_dns` default).

What kind of item fixes one is Jeff's to say (FLOW-20): a program item, an
exempt fix (a regression or a security problem), or an item after the
freeze. This design proposes, and the review decides:

| Divergence | Proposed |
|---|---|
| D13, D14, D12 (`initialize`) | With S2's item, which changes `initialize` and builds the executor's suite |
| D5, D6, D7, D8 (the claim and the token) | With S4's item, which changes the claim's connection, or after the freeze |
| D17, D18, D19 (the Go relay's robustness) | Not fixed in the Go relay, which the program retires. The specification allows what the Go relay does, and another relay may do better. Jeff may class D18 as a security fix |
| D1, D2, D3, D4, D16 (the collector) | After the freeze, or with the first alternative collector |
| D9, D10, D11, D15 (the guest API) | With the Rust relay's M-items, which implement the specification, or after the freeze |

### The harness

`bw-conformance` runs one or more *suites* against one *target* and reports
each case as pass, fail, or not applicable to what the target declares (its
mode, its link kind, the forms it can be started as).
There is no skip for a case the target should pass, and no retry (TEST-04).

**Suites.**

- `relay`: the harness is the claimant and the collector (over the tunnel),
  the build environment, and the upstream world. It covers seam 1 (relay
  role), seam 2 (the relay's side), seam 3, and seam 6 as carried by the
  relay. Case groups: claim authorization and refusal (a refused claim leaves
  the relay claimable; a spent relay stops listening); field validation, one
  case per rule in the table above; `build_link` and `upstream_dns` by mode;
  ordering at claim and at completion; DHCP; DNS (reserved names, list
  order, the 5 s failover, SERVFAIL); on a frame link, ARP and the capture
  of connections to any address; each `artifacts`, `cwd` and
  control-plane endpoint with its error replies; interception over HTTP and
  TLS, including the leaf's chain and CRL distribution point and upstream
  TLS against `upstream_ca_certs`; refused upstream dials; capture modes and
  the capture trailer; the environment record; and the RELAY-02 round trip,
  in which the harness's ledger verifier checks every record's signature
  against `ledger_public_key` and the hash of each body the build received
  against the bytes it received.
- `collector-outputs` and `collector-inputs`: the harness is the relay. It
  accepts the tunnel and the claim, checks them as seam 1's claimant role
  (the tunnel before the claim, the claim's body against its schema, the
  token), then drives the endpoints of Part A or Part B over the tunnel
  with their error cases. `collector-outputs` also checks what a target with
  a read-back directory stored, against the disk layout.

**Seams and suites.** The suites are per component under test, not per
seam, because a relay cannot be driven on one seam without the others. Each
requirement of each specification belongs to one suite or is listed as not
machine-checked (Decision 3):

| Seam | Role | Checked by |
|---|---|---|
| 1 `relay-claim` | relay | `relay` |
| 1 | claimant | `collector-outputs`, `collector-inputs` (the tunnel and claim the collector under test sends) |
| 2 `collector`, Part A | collector | `collector-outputs` |
| 2, Part B | collector | `collector-inputs` |
| 2 | relay | `relay` |
| 3 `relay-guest-api` | relay | `relay` |
| 3 | build environment | No suite yet (a11) |
| 5 `executor-initialize` | executor | No suite yet (a11); the Go tests in `cmd/warden-io/` |
| 6 `bootstrap-config` | relay, which carries it | `relay` |
| 6 | producer | `collector-inputs`: the document served validates against the schema |
| 6 | executor, which reads it | No suite yet (a11); the Go tests in `bootstrap/` and `cmd/warden-io/` |

The umbrella says "one specification and one black-box suite per seam" and
that seams 1 to 8 each get a suite. When this item lands, seam 5 and the
executor's side of seams 3 and 6 have a specification and no suite. S1's
done-when (the suites pass against the Go relay and the disk collector)
does not ask for one; a11 puts the deferral to the review.

**Instances.** A case that ends an instance gets a fresh one: every
accepted claim, every 500, the host-mode refusal of D17, and every
completion (a relay after `/exit`, D18; a collector after
`POST /v1/complete`, D16). The completion is the last request such a case
sends. Cases that leave an instance usable (401s, the other refused claims,
validation) share one (TEST-07). After a claim returns, the harness polls
the relay's `/health` on port 8300, every 100 ms for at most 10 s, before
its first build-side request, and asserts nothing about the listeners at the
moment of the 200 (D19).

**What a relay under test must provide.** A target file states these; the
launcher makes them true.

1. *A fresh, unclaimed relay on demand.* A relay takes one claim, so the
   harness starts many. `exec` runs an argv with an environment; `oci` runs
   an image; `external` is told how to ask for one, or runs the cases one
   relay allows.
2. *The claim endpoint*: an address the harness can connect to, and the
   claim token if the relay was started with one. TCP now; a vsock or
   inherited connection is added with S4.
3. *The build link*, in one of two kinds. `frames`: a socket carrying one
   Ethernet frame per datagram, which is the Go relay's host mode (an
   inherited socketpair, `-fd`) and the Rust relay's build NIC; the harness
   brings its own network stack and takes its lease from the relay's DHCP.
   `ip`: a kernel interface in a network namespace or container network the
   harness joins, for vm and container mode. The kind also tells the harness
   where to connect: on `frames`, to the address DNS returned for the
   origin, which the relay captures; on `ip`, to the relay's own address
   with the origin's name in `Host` or SNI, because redirection there is the
   driver's (seam 3). The file also gives the link the harness should send
   as `build_link`, or says the relay's address is assigned by the launcher
   (container mode). A container-mode relay takes the first non-loopback
   IPv4 address it finds as its own (`relay.DetectSelfIP`); with one
   interface for the build and one for the upstream, which it finds depends
   on their order. Task 6 settles how the launcher orders them and the
   target file records it.
4. *The upstream network is the harness's.* The claim's `upstream_dns` holds
   addresses only, port 53 implied, so the harness's resolver must listen on
   port 53 of an address the relay reaches, and its origin servers on ports
   80 and 443. A host-mode relay refuses to dial loopback, private and
   link-local addresses, so those servers sit on addresses outside those
   ranges. On Linux the `exec` and `oci` launchers build this themselves: a
   network namespace or container network that is the relay's whole upstream,
   addressed from the documentation ranges (192.0.2.0/24, 198.51.100.0/24,
   203.0.113.0/24). That needs root or unprivileged user namespaces. The
   harness's origin CA goes in `upstream_ca_certs` with
   `upstream_system_ca: false`.
5. *Its resolver configuration.* The default for an absent `upstream_dns`
   reads the relay's `/etc/resolv.conf`, a fixed path, so the launcher
   supplies that file: `exec` starts the relay in a mount namespace with the
   file bound over it, and `oci` mounts it into the container. `external`
   cannot, and the default's cases are not applicable to it.
6. *Its mode* (`host`, `vm` or `container`, RELAY-04), which selects the
   cases whose expected result differs by mode.

Results are observed only through the protocols: what the harness's collector
received and in what order, what the build side was answered, and the ledger.
A relay's log and exit status are attached to a failure and never asserted
on.

**What a collector under test must provide.**

1. *A way to start it against a relay address*: a command line with
   placeholders for the claim address, the output location, the context
   directory, the build script and the capture mode. For the disk collector:
   `collector --relay {claim} --dir {out} --context {context} --script {script} --capture {capture}`,
   with `WARDEN_CLAIM_TOKEN` in its environment. One instance serves one
   build, so the harness starts one for each case that sends a completion.
2. *What it can be started as.* The file says whether the collector can run
   with no build script, no context, no bootstrap config and no token, and
   whether it keeps serving after a completion. `cmd/collector` can run
   without a script or a context. It always serves a bootstrap config
   (`{"version":1,"trust":{"mode":"exclusive"}}`, the kit's choice for a
   Linux guest), always sets a collector token, and exits at the first
   completion. For this target the cases for a 404 from `GET /v1/bootstrap`,
   a collector with no token, `GET /v1/status` after a completion and "the
   first completion wins" are therefore reported not applicable. They run
   in the specs repository's self-test against the harness's own collector,
   and against any target that declares the form.
3. *Its limits are not what a case measures.* The harness sends `ready` as
   a relay does, before it answers the claim, and a case that has sent a
   heartbeat sends one every 2 s until it ends, so the disk collector's 90 s
   and 30 s limits never decide a verdict.
4. *A way to read back what it stored* (optional): a directory in the disk
   layout, or a command that produces one, read after the collector has
   exited. Files the layout does not name (`ca.cert.pem`) are ignored.
   Without it, `collector-outputs` checks replies only, and its report says
   so.

The reply to `POST /v1/complete` from a collector that exits at completion
is not asserted (D16). A collector's log and exit status are attached to a
failure and never asserted on, as a relay's are.

**Target files for this repository** live in `tools/conformance/targets/`:
`relay-host.toml`, `relay-vm.toml` and `relay-container.toml` (`exec`, the
relay binary with `-mode host`, `vm` or `container`), `relay-oci.toml`
(`oci`, the image built from `Dockerfile.relay`, whose entry point passes
no `-mode` and so runs container mode) and `collector-disk.toml`.

`make build` produces nothing these can name on every platform (it writes
`dist/warden-relay-linux-<arch>` and builds no collector), and
`Dockerfile.relay` expects `relay-<arch>` files that only the `relay-image`
workflow makes. So `make conformance` builds what it tests, into
`dist/conformance/`: `relay` (`./cmd/relay` for the host, `CGO_ENABLED=0`;
the relay targets run on Linux only), `collector` (`./cmd/collector` for
the host, `collector.exe` on Windows), and for `relay-oci` a build context
holding `relay-<arch>`, from which it builds the image
`buildwarden-relay:conformance` with `Dockerfile.relay`. The `Makefile`
gains two targets, `conformance` and `pin-specs`.

### Releases and how they are consumed

- A release is a tag `v0.x.y` on the specs repository's `main` (Decision 4).
  Its `release` workflow builds the `bw-conformance` binaries and attaches
  them with `SHA256SUMS`.
- This repository names one release in `tools/conformance/go.mod`
  (Decision 7). `make conformance` runs `go tool bw-conformance` in that
  module against the target files.
- A seam change goes in this order: a pull request to the specs repository
  changes the document and the suite; the implementing branch pins that
  pull request's head commit, and the `conformance` workflow's run for that
  push shows the suites passing on hosted runners; the implementing branch
  is reviewed with the specs branch; Jeff merges the specs pull request and
  a release is tagged; the pin moves to the tag (`make pin-specs`) and the
  implementing branch opens its PR, where the `pin` job requires the tag.
  Until M6 there are two relays, and each moves its own pin when it
  implements the release; an older pin keeps passing.
- A tag is never moved. A failure found after a release is fixed by a new
  one: a patch release when no verdict changes, otherwise the next minor.
  It needs its own pull request in the specs repository and Jeff's merge.
- The relay repository consumes the same release with its own target files,
  through the module if its CI has Go or through the release binary and its
  checksum. That wiring is M1's, in the relay's queue.

### CI

**Specs repository** (`.github/workflows/` there):

- `ci`, on pull requests and `main`: the layout check (every
  `v<N>.<part>.schema.json` beside its `v<N>.md`; schemas valid 2020-12;
  the documents' JSON examples validate against them); requirement
  identifiers unique, and each covered by a case or listed as not
  machine-checked; `go vet` and `go test ./...`, which includes a
  self-test that runs each suite against the harness's own counterpart role
  (the `relay` suite against the harness's relay, the collector suites
  against its collector), so a suite and the model it was written from
  cannot disagree unnoticed; and a check that `go.mod` does not require the
  kit.
- `implementations`, weekly and on demand, not a merge gate: the released
  suites against each known implementation at its released version (the Go
  relay image first), published as a table in the run summary. It is not a
  gate because a specification change is merged before the code that
  implements it.
- `release`, on a `v*` tag.
- `flow18`, the guard this repository has, so the public repository never
  carries `entire/` refs (FLOW-18).

**This repository:** a new `conformance` workflow, shaped like
`integration`. Its paths are every package the relay and the disk collector
are built from (`go list -deps ./cmd/relay ./cmd/collector`), so that a
change to a shared package runs the suites (TEST-02's caveat): `relay/**`,
`relaycfg/**`, `collector/**`, `bootstrap/**`, `ledger/**`, `cmd/relay/**`,
`cmd/collector/**`, `driver/*.go`, `driver/buildlink/**`,
`driver/script/**`, `go.mod`, `go.sum`, `Dockerfile.relay`, the `Makefile`,
`tools/conformance/**` and the workflow itself. `cmd/warden-io/**` is not
among them, because no suite runs the executor yet (a11). It runs:

- on pull requests that reach those paths;
- on a push to any branch other than `main` that changes
  `tools/conformance/**` or the workflow. This is how a branch gets hosted
  runs before it has a pull request: GitHub offers a manual run only for a
  workflow that is already on the default branch;
- weekly on `main`, and on demand.

Jobs: `pin` (the version in `tools/conformance/go.mod` is a release tag;
not run on a branch push, where a pseudo-version is allowed); `relay` on
`ubuntu-latest` (the four relay targets); `collector` on `ubuntu-latest`,
`macos-latest` and `windows-latest` (the disk collector, whose capture
links differ on Windows). It is tier 1 in TEST-02's terms.

### Tier 1's seam list

`tools/tier1/rules.yaml`, replacing `seams: []` and the comment above it:

```yaml
# GLOBAL-08, EXEC-04: versioned seams, each with the code that defines it
# and the contract path a change to that code must also touch. For a seam
# specified in the specs repository that path is the pin of the release this
# repository implements (TEST-05).
seams:
  - name: relay-claim
    code: [relaycfg/relaycfg.go, relaycfg/tunnel.go, relaycfg/buildlink.go]
    contract: tools/conformance/go.mod
  - name: collector
    code: [collector/collector.go, collector/paths.go, collector/signals.go,
           collector/store.go, relay/collector_client.go]
    contract: tools/conformance/go.mod
  - name: relay-guest-api
    code: [relay/controlplane.go, relay/heartbeat.go, relay/dns.go, cmd/relay/dhcp.go]
    contract: tools/conformance/go.mod
  - name: executor-initialize
    code: [cmd/warden-io/main.go, cmd/warden-io/initialize.go,
           cmd/warden-io/baseline.go, cmd/warden-io/jks.go,
           cmd/warden-io/platform_darwin.go, cmd/warden-io/platform_linux.go,
           cmd/warden-io/platform_windows.go]
    contract: tools/conformance/go.mod
  - name: bootstrap-config
    code: [bootstrap/bootstrap.go]
    contract: tools/conformance/go.mod
```

Files are named one by one because the check's globs cannot leave tests out.

Two sets of relay code are on a seam and left off the list on purpose.
`relay/relay.go` holds the reserved hosts' handlers together with the proxy
path, the CA and the ledger recording, and `cmd/relay/` holds the claim's
per-mode steps. Listing them would ask most relay changes for a pin move or
a `Tier1-Allow` trailer. The `relay` suite covers them instead: the
`conformance` workflow runs on any change under `relay/` and `cmd/relay/`,
and a change in behaviour there fails the pinned suite.

The executor has no suite (a11), so nothing covers its files that way, and
the entry lists every file that holds what `executor-initialize/v1.md`
states: the invocation and the steps (`main.go`, `initialize.go`), the
baseline (`baseline.go`, and `jks.go` for the truststore's alias and
password) and the per-OS values (`platform_<os>.go`). Until S2's suite
exists, a change to them is held by this check, by the Go tests in
`cmd/warden-io/` under `make test`, and by EXEC-04's agent-judged part.
`trust_linux.go` is not listed: the trust-store procedure is EXEC-03's, and
the specification names it without fixing its steps.

The list is shared with `driver-contract`, which adds an entry whose
contract is a document in this repository. The entries do not overlap, and
whichever item lands second merges the list.

A change that touches a listed file and changes no behaviour (a refactor, a
comment) carries a `Tier1-Allow: GLOBAL-08 <path>: <reason>` trailer
(LINT-05), as a prose-only fix to a specification does under RELAY-05.

### The book and the specifications link both ways

- **Book to specifications.** A new page,
  `docs/book/src/concepts/seams.md` ("Component seams", listed in
  `SUMMARY.md` at the head of the Specifications part), has one row per
  seam: what it is between, its specification, the package that implements
  it here, and the suite that checks it. It is the only file in this
  repository that links into the specs repository, and all its links name
  one ref: the pinned tag, or, while the pin is a pseudo-version, the commit
  that pseudo-version names. `make pin-specs` rewrites that ref together
  with the pin. A tier-0 test in `specs/specs_test.go` fails when the page's
  ref and the version in `tools/conformance/go.mod` disagree, so the book
  never points at a release CI does not test. `specs/README.md` gains one
  sentence pointing to the page for the seams that are not the ledger.
- **`concepts/architecture.md` keeps what explains and gives up what
  enumerates.** Its sections on the collector and the claim keep their
  narrative (why the config is pushed, the tunnel, what the build link is
  for, the ordering rule and its reason), and each opens by saying that the
  specification is the contract, with a link to the seams page. The
  collector's endpoint table, `build_link`'s validation rules and the
  by-mode table are replaced by a sentence and that link. The headings
  stay, so links into the page keep working. *Rejected:* keeping the tables
  and marking them non-normative. They would be a third description of the
  same protocol (code, specification, book), kept in step by hand.
- **Specifications to book.** The specs repository's README has an
  "Implementations" table (the Go relay, the disk collector and the executor
  in `buildwarden/buildwarden`; the Rust relay when it is public), and each
  document's header links the book's page for the concept
  (`https://buildwarden.github.io/buildwarden/`) and names its reference
  implementation. Until seam 4 moves, the README links the ledger
  specification under `specs/` here.
- `AGENTS.md` gains a row for the seam specifications and the pin,
  `CONTRIBUTING.md` a short section on `make conformance` and
  `make pin-specs`, and `contributing/structure.md` an entry for
  `tools/conformance/` and the new duty of `specs/specs_test.go`.
- `concepts/overview.md` is unchanged: no component's flow changes
  (FLOW-15), so the phase's overview sweep (FLOW-20) has nothing from this
  item.

### How the loop treats the second repository

Until P1, the queue item, the design and both reviews stay here (a4).

| Phase | In this repository | In the specs repository |
|---|---|---|
| Plan | Every task names its repository | |
| Implement | The pin (a pseudo-version of the specs branch's head), target files, `make conformance` and `make pin-specs`, the workflow, book pages, tier 1's seam list, the rule changes. `make lint` and `make test` green (FLOW-03). Task 8 is done when the `conformance` workflow's run for the pushed head is green on all three runners; the step's evidence names the run | Documents, schemas, harness, suites and CI on `feat/seam-specs`, pushed at each green checkpoint (FLOW-17); its pull request opened when Implement ends, so Jeff can read it early |
| Review | Tier 1 and the tier-2 report on this branch. Review confirms that the `conformance` run for the branch's head is green | The same tier-2 report also reads the specs branch at the pinned commit: the documents against the code lists in this design, and the harness against the rules in a4. Its findings say which repository they are in |
| Publish | `f6-merge-main`, `f6-consolidate` and `f6-open-pr`, as for any feature. `f6-open-pr` finds the pin on the tag; if it does not, the PR's `pin` job fails | `f6-release-specs` (human, a15): Jeff reads the tier-2 report's MUST findings and merges the pull request. The tag `v0.1.0` is pushed and the pin moved on this branch (`make pin-specs REF=v0.1.0`, `make test`, `make lint`, commit, push), by him or by the attended session on his word |

The mechanical link between the two is the pin. Before the release, the
suites have passed on hosted runners against the exact commit the branch
pins (the push run). After it, this repository's PR cannot have green CI
(FLOW-03, phase 6) unless the pin names a release tag and that release's
suites pass here. The pin's move comes after Review and changes three
files: `tools/conformance/go.mod`, its `go.sum`, and
`docs/book/src/concepts/seams.md`. The PR's CI runs everything on the
result.

`f6-release-specs` is a human step, so `wf` lists it for Jeff from the day
this design adds it, and Publish's gate waits only for steps still pending
when Publish is seeded. It is not to be done before Review is done: the
tier-2 report and the green hosted run are what the merge rests on
(FLOW-08). Done early, or without the pin's move, it is caught by the `pin`
job, which stops the PR.

If the specs pull request changes between Review and its merge, the tag's
tree differs from the commit Review read. The session that moves the pin
compares the two in the specs repository and, if they differ, records the
difference in the step's evidence and in the pin commit's message. The
`conformance` workflow judges the result; a failure is repaired under
FLOW-11 or goes back under FLOW-06.

If the item goes back after the tag (FLOW-06, FLOW-10) and the specs
content has to change, the change is a new branch and pull request in the
specs repository and a new release (**Releases and how they are
consumed**). On a return to Design, the Design step adds a human step
`f6-release-specs-r<n>` for it. On a return to Implement alone, Implement
stops with a blocking question that names the release it needs, as it does
for a missing repository (a2).

### What Jeff creates

Before `f3-implement` starts (a2), by Jeff or by an attended session on his
word in that session (umbrella a11):

1. The repository `buildwarden/buildwarden-specs`, public (a3), **with a
   first commit on `main`**, so that no agent ever pushes `main`: a README
   stub and the licence files Jeff chooses. This design proposes Apache-2.0,
   as here. This repository's `NOTICE` reads "Copyright Amazon.com, Inc. or
   its affiliates"; whether the new repository carries it is Jeff's call,
   and no agent copies it.
2. On it: `main` protected (pull request required, no force-push, no
   deletion), "Automatically delete head branches" on (FLOW-16), Actions
   enabled.
3. Push access to branches, and pull requests, for the credentials the loop's
   sessions use; and the repository added to what the cloud environment may
   reach.

After Review: `f6-release-specs` (a15). The merge of its pull request,
preferably as a merge commit so that the commits this branch pinned while in
progress stay in its history; the tag `v0.1.0` (a8); and the pin's move
here.

Not needed: packages, Pages, secrets, or any change to `buildwarden-relay`.

## Defaults and overrides

This item adds no default, override or fallback to the relay, the collector,
the executor, `warden` or any driver, and no relay setting: the claim gains
and loses no field (RELAY-08).

It adds these to the tooling:

| Default | Why it is the obvious one | Override, used exclusively |
|---|---|---|
| `make conformance` runs every target in `tools/conformance/targets/` | A developer who names nothing wants the whole check CI runs | `TARGETS="relay-host collector-disk"` runs exactly those |
| The harness comes from the module version in `tools/conformance/go.mod` | It is the release this repository claims to implement | `SPECS_DIR=<checkout>` runs the harness from a local checkout of the specs repository instead, and prints that the run is not the pin. CI never sets it |
| The harness runs every suite the target's role has | A target is conformant only if all of them pass | `-suite <name>`, repeatable, runs exactly those |
| `relay-oci.toml` names its container runtime, `runtime = "docker"` | It is what the hosted runner has; an `oci` target must name one, and the harness has no default of its own | `OCI_RUNTIME=podman make conformance` runs that target with `podman` and nothing else |

`make pin-specs` has no default: `REF` is required. The harness's own
timings (the readiness poll, a case's time limit) are constants of a
release, not settings.

No fallback is added, and three that might be expected are rejected:

- A target that cannot run on the host (the relay targets off Linux, or
  without root or user namespaces; `relay-oci` without the runtime it names)
  is an error that names what is missing. It is not skipped, the harness
  does not look for another runtime, and `exec` is not tried in place of
  `oci` (GLOBAL-11).
- A failed case is not retried (TEST-04).
- A pin that cannot be fetched fails the run. There is no cached or bundled
  copy of an older release.

The specifications record defaults and named fallbacks that already exist and
that earlier designs settled; none changes: the absent `upstream_dns` default
and the resolver list's in-order failover (`hyperv-relay-per-build-channel`),
what each mode does with an absent `build_link`, host mode's `--subnet`
included (`build-link-subnet`), the empty bootstrap config and its `append`
trust mode (`unified-bootstrap`), and the disk collector's copy where it
cannot create a capture symlink.

## Cross-driver impact

No driver is named in `drivers`, and none is affected. No file under
`driver/` changes, and no shared package (`relay/`, `relaycfg/`,
`collector/`, `ledger/`, `bootstrap/`) changes either (Decision 11), so no
driver's build path differs after this item. Every driver's later seam
changes are affected the same way: a change to a listed seam file must move
the pin.

## Rule changes

Four amendments. Each is made in Implement, after the review approves it by
name (FLOW-14).

The umbrella's table of rule changes, which is "for direction", gives S1
two: RELAY-05 and TEST-05. This design declares TEST-05 and leaves RELAY-05
to seam 4's move (a1). It also declares three that the table does not give
S1: TEST-02, GLOBAL-08 and EXEC-04. The table gives GLOBAL-08 to K1 and to
S2, S3 and S4 for their seams; S1 needs it because S1 is the item in which
the seams' contract documents come to exist.

### TEST-05 (amend), `docs/development/testing.md`, `tools/tier1/rules.yaml`

**Why:** after this item the contract tests for the relay's and the
collector's seams are the specs repository's suites, run black-box. The rule
as written describes Go tests inside this repository.

**Current wording:**

> ### TEST-05 — Contract tests for the seams
>
> **Severity:** MUST
> **Check:** automated + agent-judgment
>
> The relay and executor public contracts (the GLOBAL-08 versioned seams) carry
> conformance / round-trip tests, so a third-party substitute has something to verify
> against:
>
> - RELAY-02 byte-exact round-trip (sent equals recorded),
> - RELAY-05 signature round-trip plus reader backward-compatibility,
> - EXEC-04 protocol conformance.

**Proposed wording:**

> ### TEST-05 — Conformance suites for the seams
>
> **Severity:** MUST
> **Check:** automated + agent-judgment
>
> The claim protocol, the collector protocol and the relay guest API (three
> of the GLOBAL-08 versioned seams) are specified in the specs repository,
> `buildwarden/buildwarden-specs`, and checked there by black-box
> conformance suites, so a substitute written in any language has something
> to verify against. This repository pins one tagged release of the specs
> repository in `tools/conformance/go.mod` and runs that release's suites
> against the components it holds, in the `conformance` workflow (TEST-02):
>
> - the relay suite against the Go relay in each of its modes (RELAY-04) and
>   as its OCI image: the claim protocol, the relay's side of the collector
>   protocol, the relay guest API, the bootstrap config as the relay
>   carries it, and the RELAY-02 byte-exact round-trip (sent equals
>   recorded);
> - the collector suites against the disk collector (`cmd/collector`): both
>   parts of the collector protocol, and the claimant's side of the claim
>   protocol.
>
> A seam's contract tests are added to its suite in the specs repository,
> not to this repository. Two contracts stay as tests here until a suite
> covers them: RELAY-05's signature round-trip and reader
> backward-compatibility (`relay/`, `ledger/`), and EXEC-04's protocol
> conformance (`cmd/warden-io/`, `bootstrap/`), whose specifications
> (`executor-initialize`, and `bootstrap-config` as the executor reads it)
> have no suite yet.
>
> The automated part is the `conformance` workflow and `make test`; the
> agent part judges whether a seam change added its cases to the suite.

**Severity and check:** MUST and `automated + agent-judgment`, before and
after.

**Enforcement change:** in `tools/tier1/rules.yaml` the TEST-05 entry's owner
changes from `"make test (the round-trip tests exist as tests)"` to
`"the conformance workflow (the specs repository's suites at the pinned
release) and make test (the ledger round-trip tests)"`. No check code
changes.

### TEST-02 (amend), `docs/development/testing.md`

**Why:** TEST-02 lists where each tier runs today. The `conformance`
workflow is a new tier-1 run and the list would be incomplete without it.

**Current wording** (the tier-1 bullet of "Where each tier runs today"):

> - Tier 1: the `integration` workflow (`make integration-test` on a hosted
>   Ubuntu runner with Docker). It runs on PRs that reach `driver/container/`,
>   the `driver/` root, a shared package (`relay/`, `ledger/`, `collector/`),
>   `cmd/`, `test-integration.sh`, a `Dockerfile` or `go.mod`; weekly on
>   `main`; and on demand for a branch that has the workflow
>   (`gh workflow run integration.yml --ref <branch>`).

**Proposed wording** (the same bullet, with one sentence added at its end):

> - Tier 1: the `integration` workflow (`make integration-test` on a hosted
>   Ubuntu runner with Docker). It runs on PRs that reach `driver/container/`,
>   the `driver/` root, a shared package (`relay/`, `ledger/`, `collector/`),
>   `cmd/`, `test-integration.sh`, a `Dockerfile` or `go.mod`; weekly on
>   `main`; and on demand for a branch that has the workflow
>   (`gh workflow run integration.yml --ref <branch>`). The `conformance`
>   workflow (TEST-05) is tier 1 too: it runs the seams' suites on hosted
>   runners, on PRs that reach a package the relay or the disk collector is
>   built from (`relay/`, `relaycfg/`, `collector/`, `bootstrap/`,
>   `ledger/`, `cmd/relay/`, `cmd/collector/`, the `driver/` root,
>   `driver/buildlink/`, `driver/script/`), `go.mod`, `go.sum`,
>   `Dockerfile.relay`, the `Makefile` or `tools/conformance/`; on a branch
>   push that changes `tools/conformance/`; weekly on `main`; and on demand.

**Severity and check:** unchanged (MUST; automated + agent-judgment). No
enforcement change: the workflow is new and is none of the jobs FLOW-14
names.

### GLOBAL-08 (amend), `docs/development/global.md`, `tools/tier1/rules.yaml`

**Why:** the contract documents of five seams now exist, in another
repository, and the bootstrap config becomes a written seam. The rule's
automated part says "contract document"; for these five, what this
repository holds of that document is the pin.

**Current wording** (the executor bullet of "Versioned seams"):

> - Executor public interface: the `initialize` invocation contract plus its
>   side of the relay protocol.

**Proposed wording** (the same bullet):

> - Executor public interface: the `initialize` invocation contract, the
>   bootstrap config it reads (`bootstrap/`), plus its side of the relay
>   protocol.

**Added,** as a new paragraph before the rule's last one. The last
paragraph ("The automated part requires a change touching a versioned seam
to also change that seam's contract document.") is not changed:

> The claim protocol, the collector protocol, the relay guest API, the
> `initialize` contract and the bootstrap config are written as
> specifications in the specs repository, `buildwarden/buildwarden-specs`
> (TEST-05). A change to one of those five changes its specification first:
> the specification is released, and the change here moves the pin
> (`tools/conformance/go.mod`) to that release. For them the pin is the
> contract document the automated part checks. The ledger format stays in
> `specs/` (RELAY-05).

**With `driver-contract`.** K1's design amends GLOBAL-08 as well: it adds a
third bullet (the driver contract) and rewrites the paragraph that begins
"Pre-1.0 these seams may still move". It leaves the executor bullet and the
last paragraph as they are, and so does this design leave the sentences K1
changes. The two sets of sentences do not meet. Whichever item lands second
merges `main` and keeps both; each sentence stays as its review approved
it, so no new rule change is needed. If the merge would need a sentence
reworded, that is a return to Design (FLOW-06). K1's quoted "current
wording" spans the whole seam list, so if S1 lands first that quotation is
stale by this item's bullet and paragraph; that is K1's to refresh.

**Severity and check:** unchanged (MUST; automated + agent-judgment).

**Enforcement change:** `tools/tier1/rules.yaml` gets the `seams` entries
and the comment under **Tier 1's seam list**, in place of `seams: []`. The
`seam-contract`
check's code does not change; it starts reporting a change to a listed file
that does not also change `tools/conformance/go.mod`. Because this edits
`tools/tier1/`, Review also runs the merge base's tier 1 with the branch's
configuration (FLOW-14); it should report nothing new on this branch, which
touches none of the listed files.

### EXEC-04 (amend), `docs/development/in-vm-executor.md`

**Why:** the same change, for the executor's rule.

**Current wording** (the rule's last paragraph):

> Changes follow the pre/post-1.0 seam posture and update the contract document. The
> automated part requires a change to the contract surface to also change its contract
> doc.

**Proposed wording:**

> Changes follow the pre/post-1.0 seam posture and change the specification
> first (GLOBAL-08): `executor-initialize`, `bootstrap-config` and
> `relay-guest-api` in the specs repository. The automated part requires a
> change to the contract surface, as `tools/tier1/rules.yaml` lists it, to
> also move the specs pin (`tools/conformance/go.mod`).

**Severity and check:** unchanged (MUST; automated + agent-judgment). No
enforcement change beyond GLOBAL-08's.

## Security note

Not required: `security_boundary` is `none`. No code on the host, environment
or relay boundary changes, and no relay code changes at all (GLOBAL-06).

What the review should still know: the specifications state today's posture
on the environment-to-relay boundary, and the relay suite is the check the
umbrella's Security note relies on when the Rust relay replaces the Go one.
Its cases therefore include the refusals, not only the happy paths: a claim
without or with the wrong token, a claim that would put a resolver or an
upstream dial inside the build link, the host-mode dial filter, unsafe
artifact and context names, and the RELAY-02 round trip. D9, D13 and D18
are the divergences with a security or measurement angle: a build can end
itself with a `GET`; a failed setup reports nothing; and a build can end the
relay process after its own completion, when the ledger is already with the
collector. All three are today's behaviour.

The harness's sandbox runs as root on CI runners and creates network and
mount namespaces, virtual links and container networks there. It runs only
the relay binary or image named in the target file.

## Validation plan

- **Tier 0** (`make test`, `make lint`): the book-link test (links to the
  specs repository name the pinned ref); tier 1's rule index parses the
  amended rules, and `rules.yaml` parses with the new `seams` entries;
  `tier1 designs` accepts this design. In the specs repository: its `ci`
  workflow, including the suites' self-test.
- **Tier 1** (the `conformance` workflow): the `relay` suite against
  `dist/conformance/relay` in host, vm and container mode and against the
  image built from `Dockerfile.relay`, on `ubuntu-latest`; the two collector
  suites against `dist/conformance/collector` on `ubuntu-latest`,
  `macos-latest` and `windows-latest`. This is S1's done-when: the suites
  pass against the Go relay and the disk collector. The first green run is a
  push run on this branch during Implement, before anything is merged or
  tagged in the specs repository; the pull request's run repeats it on the
  tag, with the `pin` job. The existing `integration` workflow does not run
  for this branch's paths and is not affected.
- **Tier 2, native:** nothing blocks. The change reaches no driver's tree
  and alters no behaviour a native host exercises (FLOW-09). Two gaps are
  tracked, in `tier2_gaps` and on the native-gap backlog:
  - `macos-vz`: the `relay` suite against the host-mode relay on macOS,
    where vz runs it. The frame link is the same code as on Linux; what
    differs is the platform (the link kickstart, socketpair behaviour). The
    harness's upstream needs loopback aliases in the documentation ranges and
    ports 53, 80 and 443, so the session needs root; if it cannot have it,
    it records the entry as blocked with that reason.
  - `qemu-any`: the `relay` suite against the Go relay VM image
    (`tools/relay-vm`). It cannot run until a VM launcher exists (a6), so
    its backlog entry is `deferred`, with a note that names what it waits
    on, and no scheduled native session is woken for it. The item that
    builds the launcher sets it `open`; the Archive summary tells it to.
  The Hyper-V relay VM is the same relay binary in vm mode behind a
  different boot channel, which is seam 8's; no entry is added for it.

## Implementation plan

Tasks name their repository: **S** is `buildwarden/buildwarden-specs`, on
its branch `feat/seam-specs`; **K** is this repository, on this branch.

0. (Human, umbrella a11, before this phase) The repository exists as **What
   Jeff creates** says. Implement checks it first and stops with a blocking
   question if it does not (a2).
1. **S** Layout, `README.md`, `CHANGELOG.md`, `go.mod`, `.entire/settings.json`
   naming the private sessions repository, and the `ci` and `flow18`
   workflows (FLOW-17, FLOW-18, a4).
2. **S** `relay-claim/v1.md` with its two schemas (GLOBAL-08, RELAY-08,
   RELAY-04).
3. **S** `collector/v1.md`, parts A and B, with the completion schema
   (RELAY-07, umbrella a1).
4. **S** `relay-guest-api/v1.md` (BOOT-04, RELAY-02).
5. **S** `executor-initialize/v1.md` and `bootstrap-config/v1.md` with its
   schema (EXEC-04, BOOT-05).
6. **S** The harness core: target files, the `exec`, `oci` and `external`
   launchers, the Linux sandbox (network namespaces, the mount namespace
   for the relay's `/etc/resolv.conf`, the order of a container-mode
   relay's interfaces), the frame link, the claimant, collector, relay,
   resolver and origin roles, and the ledger verifier (GLOBAL-03, GLOBAL-07,
   GLOBAL-09 to GLOBAL-11).
7. **S** The `relay` suite, then the `collector-outputs` and
   `collector-inputs` suites, each case naming its requirements; the
   self-test; the requirement-coverage check (TEST-04, TEST-05, TEST-07).
8. **K** `tools/conformance/` (the module with its `tool` directive, and
   the five target files); the `Makefile`'s `conformance` target, which
   builds `dist/conformance/relay`, `dist/conformance/collector` and the
   `relay-oci` image, and its `pin-specs` target; and the `conformance`
   workflow (TEST-02, TEST-05, GLOBAL-09 to GLOBAL-11). D17 and D18 are
   reproduced once by hand. Done when the workflow's run for the pushed
   head is green on all three runners. A case that fails because the
   specification misread the code is fixed in **S**, and one that fails
   because the code misbehaves in a way this design does not record goes
   back to Design (FLOW-06, Decision 1).
9. **K** The rule changes, each as approved: TEST-05, TEST-02, GLOBAL-08,
   EXEC-04, and `tools/tier1/rules.yaml` (FLOW-14, LINT-02).
10. **K** The book: `concepts/seams.md` and its entry in `SUMMARY.md`,
    the edits to `concepts/architecture.md`, `contributing/structure.md`,
    the sentence in `specs/README.md`, the ref test in
    `specs/specs_test.go`, `AGENTS.md` and `CONTRIBUTING.md` (UX-01,
    FLOW-15).
11. **S** The `release` and `implementations` workflows; open the pull
    request (FLOW-08).
12. (Human, `f6-release-specs`, after Review) Jeff merges the specs pull
    request. The tag `v0.1.0` is pushed and the pin moved here with
    `make pin-specs REF=v0.1.0`, by him or by the attended session on his
    word (a15).

After the merge, filed through intake as a program item: seam 4's move
(a1).

Owed to later items, and written into the Archive summary so that their
intake finds it:

- the executor's suite, with S2's item (a11); the umbrella's S2 row does not
  name it;
- the VM-image launcher, with the first item that has a VM image to test
  (a6); that item also sets the `qemu-any` backlog entry `open`;
- each divergence the review turns into work, in the item the review names
  (**Divergences found**).

## Design review

None yet.

## Consolidation

Filled in at Publish.

## Rollback notes

None.

## Archive summary

Filled in at Publish.
