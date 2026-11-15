---
feature: seam-specs
status: draft
drivers: []              # no driver tree and no shared package changes (see Cross-driver impact)
seams: [relay-claim, collector, relay-guest-api, executor-initialize, bootstrap-config]
                         # each gets its contract document, the collector seam two (a10);
                         # none changes behaviour here
surfaces: []
security_boundary: none  # no code on a boundary changes; see the Security note
alters_relationships: true   # the seams' definition and their contract tests move to another
                         # repository, and this one depends on a tagged release of it
design_review: required
native_validation: []    # nothing blocking; two tracked gaps (Validation plan)
tier2_gaps:
  - {host: macos-vz, what: "relay suite against the host-mode Go relay on macOS; deferred until the exec launcher has a macOS form"}
  - {host: qemu-any, what: "relay suite against the Go relay VM image; deferred until a VM launcher exists"}
rules: [GLOBAL-01, GLOBAL-02, GLOBAL-03, GLOBAL-04, GLOBAL-06, GLOBAL-07, GLOBAL-08, GLOBAL-09,
        GLOBAL-10, GLOBAL-11, RELAY-02, RELAY-04, RELAY-05, RELAY-07, RELAY-08, EXEC-02,
        EXEC-03, EXEC-04, BOOT-02, BOOT-03, BOOT-04, BOOT-05, BOOT-06, BOOT-07, TEST-02,
        TEST-03, TEST-04, TEST-05, TEST-07, LINT-02, LINT-05, UX-01, FLOW-01, FLOW-02,
        FLOW-03, FLOW-04, FLOW-05, FLOW-06, FLOW-07, FLOW-08, FLOW-09, FLOW-10, FLOW-11,
        FLOW-12, FLOW-14, FLOW-15, FLOW-16, FLOW-17, FLOW-18, FLOW-19, FLOW-20, LOOP-01,
        LOOP-02, LOOP-03, LOOP-04, LOOP-06, LOOP-07, LOOP-08, LOOP-09, PROFILE-01]
rule_changes:
  - {rule: TEST-05, change: amend, paths: [docs/development/testing.md, tools/tier1/rules.yaml]}
  - {rule: TEST-02, change: amend, paths: [docs/development/testing.md]}
  - {rule: GLOBAL-08, change: amend, paths: [docs/development/global.md, tools/tier1/rules.yaml,
                                              tools/tier1/coupling/checks_test.go]}
  - {rule: EXEC-04, change: amend, paths: [docs/development/in-vm-executor.md]}
  - {rule: PROFILE-01, change: amend, paths: [docs/development/loop-profile.md]}
concepts: [seams, specs-repo, conformance, conformance-harness, relay-claim, claim-protocol,
           collector, collector-protocol, collector-outputs, collector-inputs,
           relay-guest-api, executor-initialize, bootstrap-config, specs-pin,
           specs-release, seam-contract, openapi, program]
related: [componentization.md, specs-migration.md, relay05-specs-wording.md,
          tier1-static-checks.md, build-link-subnet.md, hyperv-relay-per-build-channel.md,
          unified-bootstrap.md, relay-leaf-crl.md, ledger-environment-types.md,
          design-approved-rule-changes.md, loop-contract.md, alters-slot.md]
---

# Seam specifications and conformance suites (componentization S1)

This is a program item (FLOW-20): phase S1 of
[`componentization`](componentization.md). That design fixes what S1 is:
create `buildwarden/buildwarden-specs` (its a10), write seams 1, 2, 3, 5 and 6,
build a conformance harness that runs black-box against any relay binary or
image and against any collector, link the book and the specs both ways, and
fill `seams` in `tools/tier1/rules.yaml`. It is done when the suites pass
against the Go relay and the disk collector. This design works inside those
decisions and does not reopen them. Where it does less than those words,
**Where this design is narrower than the umbrella** lists it.

The umbrella was amended in place on 2026-11-14 by `loop-contract` (its
**Amendments**), and is read here as amended. S1's row is as it was.
The wave-1 gate now reads "The one queue works across repositories; suites
pass on the Go relay". This item's done-when is its second half; the first
is P1's, and this item does not wait for it (Decision 8).

It works in two repositories: this one and `buildwarden/buildwarden-specs`
(LOOP-04; **Design**).

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

The book describes three of them in part.
`docs/book/src/concepts/architecture.md` has the collector's endpoint
table, the claim's steps, `build_link`'s validation and its by-mode table,
and the ordering rule, and #115 updated it; under "Communication paths" it
also lists the relay guest API's endpoints as the build sees them. It is an explanation for the book's
readers: it leaves out replies, limits and error cases, nothing checks it
against the code, and no implementation can be tested against it.

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
  specifications describe `main` with both. `loop-contract` amended it in
  place on 2026-11-14 (its **Amendments**): one queue for work in every
  repository, the relay's milestones as items in that queue, and the
  wave-1 gate. Decision 2 gained a sentence on the one queue, and seam
  10's row and the FLOW and LINT row of the rule table were reworded.
  What this design reads of them is as it was: Decision 2's coupling
  through written seams and tagged releases, Decision 3, Decision 18,
  the rows of seams 1 to 6 and 9, S1's row, and the rule table's rows
  for S1 and K1.
- [`loop-contract`](loop-contract.md) (the first item of P1, merged as
  `ad4ea50`, #119, on 2026-11-15): the Loop Contract, LOOP-01 to LOOP-09,
  this repository's profile, PROFILE-01, and the amended FLOW-01, FLOW-03
  and FLOW-14. This design is brought under them: the specification comes
  first at a seam and the design lists the change (LOOP-02, **Specification
  changes**); a suite's cases name the requirements they check and every
  requirement is covered or listed (LOOP-03, Decision 3); an item names
  the repositories it works in (LOOP-04, Decision 8); a run reports a
  specification change apart from the code (LOOP-06); a consumer's trunk
  names only a release (LOOP-07, Decision 7); what binds a worker in a
  repository that has not adopted the contract (LOOP-08, a4); a feature's
  design declares no change to a loop rule (LOOP-09, FLOW-14, a15); and
  the profile entries this item makes stale are a rule change it declares
  (PROFILE-01, **Rule changes**).
- [`alters-slot`](alters-slot.md) (a process item, merged as `897a4a8`,
  #118, on 2026-11-14): `wf next` and `wf validate` count the features
  that hold an `alters_relationships` slot against `config.yaml`'s limit
  of 3, a slot being held from the claim of the Plan step (**Beside
  `driver-contract` in the queue**). It changed `tools/workflow/`, two
  settings files and one agent definition, and no file a specification is
  taken from.
- [`specs-migration`](specs-migration.md) and
  [`relay05-specs-wording`](relay05-specs-wording.md): the layout of `specs/`
  (one directory per document family, `v<N>.md` beside `v<N>.schema.json`,
  an index in `README.md`), the rule that a schema's `$id` is a name and not a
  location, and the tier-0 test that ties the producer to the documents. The
  specs repository uses the same layout, extended (Decision 2).
- [`tier1-static-checks`](tier1-static-checks.md) built `seam-contract` and
  kept its seam list as data in `rules.yaml`, "so new seams edit data, not
  code". This item fills the list. The check's code does not change;
  one test of it does (**Rule changes**, GLOBAL-08).
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
  route by which this design's five rule changes are approved by name.

All twelve already carry front-matter; no retrofit is needed (FLOW-12).

One design is not on `main` and so is not in `related`. This design
depends on it, cites it by section or question ID, and quotes none of its
rule text as final:

- `driver-contract` (K1, on `feat/driver-contract`, read at `a6c8f0b`). It
  is a redraft, sent back by its first review on 2026-11-14. Its second
  review approved it on 2026-11-15, at `a6c8f0b` (its Design review
  entry, at `c767992`): every question answered, its eleven rule changes
  approved by name, its Security note accepted, and its amendments to
  the umbrella, which are made in its PR (its a18), put to Jeff once
  more and left to stand. It has not merged.
  What this design relies on is among the answers, each confirmed
  as that design recommended: `warden` sends the claim, at an endpoint
  the driver reports (its a13); `warden` starts the default collector,
  one per build with a fresh token, and the driver starts none (its a12);
  seam 9's specification and suite start in this repository and move to
  the specs repository in K2 (its a17); seam 8 is to be filed as its own
  S-track item (its a11); and a shared collector is noted in the
  umbrella's "Later, not scheduled" and not queued (its a22). Seam 9 is
  a command contract: `warden` finds a `warden-driver-<name>` binary and
  runs commands on it, `warden` keeps the session, the collector, the
  claim and the reporting, and a driver starts the relay and the
  environment. It amends GLOBAL-08, TEST-05 and PROFILE-01 and adds an
  entry to tier 1's seam list, as this design does; **With
  `driver-contract`** says which sentences each design owns, and gives
  in full the one row of the profile that both reword. It also amends
  the umbrella: seams 1, 6 and 9, its S4 row and, among others, the
  TEST-05 row of its rule table (its **Amendments to the umbrella
  design**). **Seam 1** below is written to hold under either wording,
  and **Amendments to the umbrella design** here says where the two
  designs' amendments meet.

Outside this repository: `buildwarden-relay` `main` at `e9b23d5`
(`spike/REPORT.md`). What the harness takes from it is in **What a relay under
test must provide**: the Rust relay's build link is raw Ethernet frames on a
socket, and its claim will arrive over vsock on VZ.

**Revisions before the design review.** The design review began on
2026-11-15 and has not ended (**Design review**, which records what its
first sitting changed). Before it, the design was revised five times
after `f1-design` was done, and none of the five is a return to Design
(FLOW-06):

- 2026-11-14, `618ae78`: the collector's lifetime is written per session
  token, on Jeff's word in the `driver-contract` design review (the
  collector is to be pluggable).
- 2026-11-14, `d60aff3` to `6e56fed`, the revision before review. It
  folds in what Jeff settled that day in the attended session: one queue
  for work in every repository (his answers to `loop-contract`'s
  questions and the change proposal he approved as the direction), and
  the redrafted driver
  contract. It also acts on an
  independent reader's fifteen concerns. What changed:
  - for one queue: Decisions 8 and 12, a4 and a15, **How the loop treats
    the second repository** (rewritten), **Beside `driver-contract` in the
    queue** (new), and the note on PROFILE-01 under **Rule changes**;
  - for the driver contract: seam 1's roles and transport, seam 2's roles,
    the collector target's two attachments, the proposed home of D5 to D8,
    **What the specs repository leaves room for** (new), Decisions 2 and 5,
    and **With `driver-contract`** (rewritten, with one sentence of the
    proposed TEST-05 reworded);
  - for the concerns: **Where this design is narrower than the umbrella**
    (new) and a6; the `macos-vz` gap, now deferred (a14); the push trigger,
    the final pin (task 12) and the order in which D17 and D18 are
    reproduced; tier 1's seam list (Decision 7, one file added, what the
    check holds, a17); the book's third table; the proposed TEST-02
    sentence and EXEC-04's bullets; and the default of `make conformance`
    (a18).

  Questions a4, a6, a14 and a15 are reworded and a17 and a18 are new; none
  is retired.
- 2026-11-14, a pass on an independent review of that revision. Its
  findings on the design were verified against the sources and taken:
  - `alters-slot` merged as `897a4a8` while the revision was written, so
    **Prior art** and **Beside `driver-contract` in the queue** state it as
    done;
  - the claim specification gives the start of the build environment to
    no role, and three places that tied a collector to its claimant or to
    one build are reworded (seam 1's Ordering, seam 6's roles, the
    collector suites and the read-back);
  - a15, Decision 12 and **The release step** name FLOW-08's "no sign-off
    step in the queue" beside FLOW-04's list, and a15 names the later
    items that would carry a rule change;
  - a4 asks only the list of invariants, the four loop rules being settled
    by Jeff's answer to `loop-contract` a17;
  - the PROFILE-01 note is rewritten for the order Jeff approved, in which
    the return it describes is expected;
  - FLOW-05 joins `rules`, and a15 says that FLOW-14 on `main` does not
    yet bar a feature's design from a loop rule.

  a4 and a15 are reworded again; no question is added or retired.
- 2026-11-15, after `loop-contract` merged (`ad4ea50`, #119). This branch
  merged `main` (`99f38de`), and the design was brought under the merged
  rules. What changed:
  - *The rule change owed.* PROFILE-01 is declared in `rule_changes` and
    under **Rule changes**, with the current and proposed wording of the
    Specifications and Consumables entries and of one row of the Checks
    table. The note that said it was owed is replaced by the entry. The
    other four rule changes were checked against FLOW-14 as amended;
    their wording is unchanged.
  - *The template.* **Specification changes** is new (LOOP-02), filled
    from **What each specification covers**, and the item's second
    repository is named at the head of **Design** (LOOP-04).
  - *Citations.* Where the design cited `loop-contract`'s questions or
    its branch, it cites the rule: LOOP-04 for one queue and the
    repositories an item names, LOOP-08 for work in a repository that
    has not adopted the contract and for trunk protection, LOOP-07 for a
    reconciliation item and for what a trunk may pin, LOOP-09 and FLOW-14
    for who may change a loop rule. LOOP-03 and LOOP-06 are cited where
    the design already did what they ask. The umbrella is cited as
    amended, with its wave-1 gate.
  - *What the merge made untrue.* That `loop-contract` is in review and
    its rules proposed; that FLOW-14 on `main` does not bar a feature's
    design from a loop rule; that `alters-slot`'s design is not on this
    branch; and the three orders of the PROFILE-01 note, of which the
    one that happened is the merge before this design's review.
  - *`driver-contract`* is read at `5808805` in place of `0c29fc3`: five
    of the answers Jeff has given in its second review are what this
    design relied on as proposals (**Prior art**). Its GLOBAL-08 and
    TEST-05 changes are word for word as at `0c29fc3`, so the ownership
    table stands, with a row added for PROFILE-01.
  - *Plan.* Task 9 gains the profile's entries.

  No question is added or retired, and no option or recommended answer
  changes. a4 and a15 cite the rules in place of `loop-contract`'s
  questions; a15 also says what LOOP-09 adds to it; a6 records that the
  question it cites in `driver-contract` is answered. `design_review`
  stays `required` and `alters_relationships` `true`.
- 2026-11-15, a pass on an independent review of that update. Each
  finding was checked against its source. What changed:
  - *`driver-contract` had moved.* The update read it at `5808805`. Its
    `a280ba9`, committed half a minute after this design declared
    PROFILE-01, declares a PROFILE-01 amendment of its own over five
    rows of the profile, two of which this design also rewords: the
    Specifications entry and the tier-1 Specification row of the Checks
    table. It is now read at `bb4d7c8`. **With `driver-contract`** has
    a row for each part of the profile, the two shared rows in full as
    they read with both changes, `loop-profile.md` among the files the
    second to merge resolves, and what is pointed out to that design's
    review. This design's own PROFILE-01 wording is unchanged.
  - *a15 and LOOP-04.* LOOP-04 holds an item that waits on another
    component's release with a blocking question that names the
    release, which is option B's mechanism. a15 and Decision 12 now say
    so, why the bullet does not bind here, and why A is still proposed.
    a15 also records that `loop-parallel`'s draft design proposes the
    two clauses that would make the step the rule. No option and no
    recommended answer changes.
  - *Smaller.* Prior art says what `loop-contract` amended in the
    umbrella's Decision 2, seam 10 and rule table; Part B's requirement
    prefix is named, `COLL-B-<n>`.

Nothing in any of the five is approved.

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
     README.md                 index: each specification, its newest release, its implementations
     go.work                   the seven modules; the harness version in progress
     relay-claim/              seam 1; module .../relay-claim, tags relay-claim/v0.x.y
       v1.md
       v1.openapi.json                 its HTTP exchange (a9)
       v1.config.schema.json           the claim's request body
       v1.identity.schema.json         the claim's reply
       CHANGELOG.md  go.mod
       cases/                          its suite: the cases, each citing CLAIM identifiers
       cmd/bw-conformance-relay-claim/ the command that runs them
     collector-outputs/        seam 2, the outputs a collector stores (a10)
       v1.md  v1.openapi.json  CHANGELOG.md  go.mod  cases/  cmd/...
     collector-inputs/         seam 2, the inputs and signals it serves (a10)
       v1.md  v1.openapi.json  v1.complete.schema.json  v1.status.schema.json
       CHANGELOG.md  go.mod  cases/  cmd/...
     relay-guest-api/          seam 3
       v1.md  v1.artifacts.openapi.json  v1.cwd.openapi.json  v1.control.openapi.json
       CHANGELOG.md  go.mod  cases/  cmd/...
     executor-initialize/      seam 5
       v1.md  CHANGELOG.md  go.mod  cmd/...      no cases yet (a11)
     bootstrap-config/         seam 6
       v1.md  v1.schema.json  CHANGELOG.md  go.mod  cases/  cmd/...
     conformance/              the harness the six share (Decision 5, a19);
       CHANGELOG.md  go.mod            module .../conformance, tags conformance/v0.x.y
   ```

   One directory per specification, and the directory is the unit that is
   versioned and released (a8, Decision 4). It holds everything a release
   of that specification is made of: the document, one `v<N>.md` per major
   version; its machine schemas; its OpenAPI description where it is an
   HTTP protocol; its suite, the cases that cite its requirements; the
   command that runs them; and its own `CHANGELOG.md`. Each directory is a
   Go module, `github.com/buildwarden/buildwarden-specs/<directory>`, so
   that a tag `<directory>/vX.Y.Z` is a version of it that a `go.mod` can
   name (Decision 7). `conformance/` is the one directory that is not a
   specification: the code every suite shares (Decision 5).

   The extension over RELAY-05's
   layout: a directory may hold more than one schema for a version, named
   `v<N>.<part>.schema.json`, because a protocol has several JSON bodies where
   a ledger metadata schema has one. A specification that is an HTTP
   protocol also holds its OpenAPI description, `v<N>.openapi.json`, or
   `v<N>.<part>.openapi.json` where it defines more than one HTTP server
   (a9, **The OpenAPI descriptions**). A schema's `$id` is a name that mirrors
   its path in the specs repository,
   `https://github.com/buildwarden/buildwarden-specs/<directory>/v<N>.<part>.schema.json`
   (RELAY-05's schema-identity rule, applied to the new repository), and does
   not change once released; a schema that moves in from this repository
   keeps the `$id` it was released with. The directory names `build-ledger/`,
   `metadata-<name>/` and `environment-<type>/` are reserved for seam 4,
   which M6 moves in (a1), and `driver-contract/` for seam 9 (**What the
   specs repository leaves room for**).
   Seam 2 has two directories because it is two specifications
   (a10): a directory is a specification, not a seam.
   *Rejected:* one directory per JSON body (`relay-claim-config/`,
   `relay-claim-identity/`), which scatters one protocol over several
   directories; an OpenAPI tree apart from the documents, where a
   description and the prose it must agree with would sit in different
   places. *Rejected:* one
   `collector/v1.md` in two parts, each with its own requirement prefix
   and suite, which this design proposed until the review (a10): Jeff
   chose two documents.

3. **Document format: Markdown prose with numbered requirements, JSON
   Schema for JSON bodies, and an OpenAPI description beside the prose for
   the HTTP seams** (a9, Jeff's answer of 2026-11-15). Each normative
   statement uses the RFC 2119 words
   and has a stable identifier (`CLAIM-7`, `COLLOUT-3`, `COLLIN-2`,
   `GUEST-12`, `INIT-4`, `BOOTCFG-2`). Every document has the same
   sections: scope and roles;
   transport; messages and fields; ordering; errors; limits; versioning;
   conformance (which role a conforming implementation may claim, and the
   requirements of that role). A suite case names the requirements it checks,
   which are requirements of its own specification, and the specs
   repository's CI fails, specification by specification, when a
   requirement is neither covered by a case of that specification's suite
   nor listed, with a reason, as not machine-checked (**The harness**).
   That is the
   coverage LOOP-03 asks of a specification that has a suite. The
   specifications that are HTTP protocols also carry an OpenAPI 3.1
   description of their paths, methods, parameters, headers, bodies and
   status codes (**The OpenAPI descriptions**). The requirements stay the
   contract: they are what a case cites and what the coverage check
   counts, and where the OpenAPI file and a requirement disagree, the
   requirement stands. Checks in the specs repository's `ci` hold the file
   to the requirements. *Rejected:* OpenAPI in place of the numbered
   requirements, which is how this design read the option until the
   review. It cannot state what follows the tunnel upgrade, HTTP trailers,
   DNS and DHCP answers or ordering between endpoints, so prose would
   still carry part of the contract with nothing to cite. Jeff's answer
   keeps the requirements and adds the description. *Rejected:* prose
   with no identifiers, which leaves "is this sentence tested" to a reader.

4. **Versioning: a tag per specification, and each release carries the
   build of the tooling that tests it** (a8, Jeff's answer of 2026-11-15,
   read with his remark on a7: "I assumed that a specific release of the
   specs themselves is coincident with the build of the conformance tools
   that test them").

   - *What a release is.* A release of a specification is a tag
     `<directory>/vX.Y.Z` on a commit of the specs repository's `main`,
     for example `relay-claim/v0.1.0`. That is Go's tag form for a module
     in a subdirectory, so the tag is also the version a `go.mod` names.
     The tag covers that directory: the document, its schemas, its
     OpenAPI files, its cases and its command, as they are at that
     commit. The six specifications have six independent version
     numbers.
   - *What a release carries.* The `release` workflow builds that
     specification's command, `bw-conformance-<directory>`, with
     `go install <command>@<tag>`: from the module as the tag serves it,
     with the harness and every other module at the versions its `go.mod`
     names. It builds a static binary for linux, darwin and windows on
     amd64 and arm64, and attaches those binaries and a `SHA256SUMS` file
     to the tag's release. So the tooling that tests a release of a
     specification is part of that release: the same tag names the text
     and the cases, and the binaries are their build.
   - *What minor and patch mean,* for each specification on its own,
     before its 1.0. A minor release may make a passing implementation
     fail, or changes what an implementation must do: a requirement
     added, changed or removed, with the schema and OpenAPI operation
     that state it, or a case added or changed that can change a verdict.
     A patch release cannot make a passing implementation fail: wording,
     an example or an OpenAPI file corrected to agree with the
     requirements, a case or a newer harness that removes a wrong
     failure.
   - *The document's version and the tag's.* The file name is the major
     version of the protocol or format, as RELAY-05 has it for the
     ledger: `v1.md` is version 1, the one the `/v1/` paths name. The
     tag's version is that of the text and its suite. While it is
     `v0.x.y`, `v1.md` is edited in place and may still change
     incompatibly (GLOBAL-08's posture before 1.0). `<directory>/v1.0.0`
     is the release from which `v1.md` changes only compatibly. A
     breaking change after it adds `v2.md` beside `v1.md` and is released
     under the module's next major version, which Go spells with a `/v2`
     path; the item that first needs one says how. So from 1.0 on the
     tag's major version is the highest `v<N>.md` in the directory, and
     before it the tag's major is 0 and the document is `v1`. A schema's
     `$id` and an OpenAPI file's `info.version` carry the document's
     `v<N>` and never a tag's version, so neither changes with a release.
   - *The harness* is tagged too, `conformance/v0.x.y`, and is not a
     specification: it has no requirement, no binary and no release
     assets. Each specification's `go.mod` names the harness version it
     is released with (Decision 5, a19).
   - *No unreleased change on `main`.* A pull request that changes a
     directory adds the new version to that directory's `CHANGELOG.md`,
     which `ci` checks, and the tag is pushed on its merge commit. So the
     newest release of every specification is what `main` holds, and that
     set is tested together on every pull request (**CI**). A tag is
     never moved.
   - *The first releases* are seven tags on one commit:
     `conformance/v0.1.0`, then `bootstrap-config/v0.1.0`,
     `collector-inputs/v0.1.0`, `collector-outputs/v0.1.0`,
     `executor-initialize/v0.1.0`, `relay-claim/v0.1.0` and
     `relay-guest-api/v0.1.0` (who pushes them: a15). Each tag is pushed
     in a push of its own. GitHub starts no workflow run for a push that
     carries more than three tags, and the `release` workflow runs on a
     tag's push. The harness's tag goes first, and its `release` run is
     green before the next tag is pushed.

   **What this costs beside one tag for the repository,** which this
   design proposed and Jeff declined:

   - A component implements several specifications at once, and the
     harness drives it through all of them. So a run of one
     specification's cases also depends on the other protocols as the
     harness of that specification's release speaks them. The six version
     numbers are independent as documents and as tags. As a tested set
     they are not: the set the specs repository tests together is the
     newest release of each (above). Any other mix is tested only by the
     implementation that pins it. Here such a mix always builds, each
     suite with its own release's harness (Decision 7, a20); whether a
     suite whose harness is older can still drive the component is what
     that run finds.
   - An implementation states six versions where it stated one, and a
     relay is run by five commands where it was run by one (**The
     harness**). This repository states them in six small modules, with
     twelve files, where one tag would have been one module (Decision 7).
   - A change that needs both a specification and the harness is two
     tags, pushed in order. The first release is seven tags, seven
     `release` runs and 36 binaries with six `SHA256SUMS` files (six
     specifications, three operating systems, two architectures), where
     one tag would have been one run and six binaries.
   - A fix to the harness reaches a released binary only through a new
     release of that specification, and it reaches the suite this
     repository runs the same way, because each suite here is built from
     what its release names (Decision 7, a20). A fix to a launcher or to
     the sandbox that every suite needs is seven tags, six `CHANGELOG.md`
     entries and 36 binaries, and six pins moved here.
   - A change to one specification can force releases of others. When it
     changes what the harness's model of a protocol must do (a field a
     claimant must send, a step a build environment takes), a suite that
     drives a component through that protocol can test an
     implementation of the change only once it is released again with
     the new harness.
   - The suites one component is run through here need not share a
     harness version. Two specifications pinned at releases made at
     different times are each built with the harness their own release
     names, so one run of a relay can use two versions of the launchers
     and of the models. The report names the harness version beside each
     specification's (Decision 7, a20).
   - There are seven modules to keep building. The harness is a module
     of its own that is tagged in the same merge as the specification
     that needs it (a19), so three pieces of machinery keep them building
     while a harness version has no tag yet: a line in the workspace
     file, a command that tidies each specification's `go.mod`, and a
     release built with `go install` and not in a checkout (**Releases
     and how they are consumed**). A branch elsewhere that pins an
     unmerged commit needs one extra line in each module whose
     specification names such a version (Decision 7).

   *Rejected:* one tag for the repository as a whole (`v0.x.y`), with one
   `bw-conformance` binary: a8's option A, which this design proposed
   because the harness spans seams and a relay implements several. Jeff
   chose a tag per specification. *Rejected:* versioning the harness
   apart from the documents and releasing its binary on its own (a8's
   option C): a suite and the text it checks would be free to disagree,
   and a specification's release would carry no tooling. *Rejected:* a
   plain git tag per directory over one Go module: a `go.mod` could then
   name only one version for all six, which is not what a7's answer asks
   of the pin.

5. **The harness is Go, lives in the specs repository under
   `conformance/`, and does not import the kit; each specification's
   cases live in its own module** (a5, as a8 reshapes it; that the
   shared code is a module of its own, tagged on its own, is a19, Jeff's
   answer of 2026-11-15).

   - *The harness* is the module
     `github.com/buildwarden/buildwarden-specs/conformance`. It holds
     what every suite shares and no case: the target-file format and the
     launchers, the Linux sandbox, the frame link, the runner and its
     report, the checks `ci` runs (layout, coverage, OpenAPI), and the
     test side of each protocol, written from the specifications. Those
     are a claimant and collector to test a relay, a relay to test a
     collector, a build environment, an upstream resolver and origin
     servers, and a ledger verifier written from this repository's
     `specs/build-ledger/v1.md`. Each role is a package the suites share.
     The verifier checks the header, every record's signature and chain,
     and the hash blocks, and reads the metadata fields its cases need by
     the names `specs/metadata-<name>/v1.md` gives them. It holds no copy
     of a schema file from `specs/`; validating metadata against those
     schemas stays with `specs/specs_test.go` here until M6 moves seam 4
     (a1).
   - *A specification's suite* is the package `cases/` in its module:
     the cases that cite its requirements, written against the harness.
     Its command, `cmd/bw-conformance-<directory>`, runs them against a
     target. The module requires the harness at one tagged version.
   - *No module requires the kit.* None of the seven may require
     `github.com/buildwarden/buildwarden`; `ci` checks each `go.mod` for
     it. Sharing `relaycfg`, `collector` or `ledger` with the Go relay
     would make a suite agree with that relay's bugs.
   - *What is released* is Decision 4's: a tag per module and, for each
     specification, its static `bw-conformance-<directory>` binaries with
     a `SHA256SUMS` file, for consumers without a Go toolchain.

   The harness's models are written from the documents, so a
   specification change that changes what a model must do changes the
   harness in the same pull request, and both are tagged on its merge.
   The harness keeps a table of the version of each specification its
   models were written from. Every command prints it with its own
   version, and every report carries it. The table is kept by hand, in
   the pull request that changes a model.

   *Rejected:* Rust. The first implementation under test
   and the loop that will write the harness are Go, and a Rust consumer runs a
   binary either way. *Rejected:* a declarative HTTP test tool or shell: the
   suites need HTTP/2 inside an upgraded connection, request trailers, DNS,
   DHCP, raw Ethernet frames and ledger verification. *Rejected:* a separate
   harness repository: one more repository to create (a2), and a suite
   that could be released apart from the text it checks. *Rejected:* Go
   tests in this repository (the umbrella's Decision 3 already rejects
   them). *Rejected:* each specification's module holding its own role
   models and requiring the others' for theirs. A relay's claim cannot be
   tested without a collector, and a collector that claims cannot be
   tested without a relay's claim endpoint, so the modules would require
   one another in a circle. *Rejected:* the other homes for the shared
   code, which a19 lists with what each costs.

6. **A component under test is described by a target file.** The harness
   never guesses how to start a relay or a collector. A target file (TOML)
   names the role under test, a launcher, and the attachment points the
   suites need (**What a relay under test must provide**, **What a collector
   under test must provide**). It names no specification and no version:
   the commands that are run say what it is tested against. The format is
   the harness's, and so are the launchers. This item builds three: `exec` (a
   binary), `oci` (an image reference, run by the container runtime the
   target file names) and
   `external` (already running; addresses only). A launcher for VM images is
   specified by the same attachment points and built by the first item that
   has a VM image to test (a6). *Rejected:* flags on the harness for each
   form (`--relay-binary`, `--relay-image`): each relay form needs its own
   launch wiring (a mode flag, an fd number, a token variable), which is the
   implementation's, not the harness's, and belongs in a file kept beside the
   implementation.

7. **This repository pins a release of each specification in a module of
   its own, `tools/conformance/<directory>/go.mod`** (a7, as Jeff
   answered it for a tag per specification, and a20, his answer of
   2026-11-15, which makes the one file of a7 six).

   ```
   tools/conformance/
     bootstrap-config/      go.mod  go.sum
     collector-inputs/      go.mod  go.sum
     collector-outputs/     go.mod  go.sum
     executor-initialize/   go.mod  go.sum
     relay-claim/           go.mod  go.sum
     relay-guest-api/       go.mod  go.sum
     targets/               the five target files (**The harness**)
   ```

   Each of the six directories is a small nested module, like
   `tools/tier1`, with the name of the specification it pins. It holds
   two files and no Go code. Its `go.mod` requires that one
   specification's module at one version and names its command in a
   `tool` directive. `targets/` is not a module. For `relay-claim`:

   ```
   module github.com/buildwarden/buildwarden/tools/conformance/relay-claim

   go 1.26

   tool github.com/buildwarden/buildwarden-specs/relay-claim/cmd/bw-conformance-relay-claim

   require (
       github.com/buildwarden/buildwarden-specs/conformance v0.1.0 // indirect
       github.com/buildwarden/buildwarden-specs/relay-claim v0.1.0 // indirect
       // and the harness's own dependencies, each marked the same way
   )
   ```

   That is the file as `go mod tidy` wrote it in a model of the layout,
   with the model's module names replaced by the real ones (**Releases
   and how they are consumed** says what was tried). The `tool`
   directive keeps the requirement through `go mod tidy` although the
   module holds no Go code, and `go tool bw-conformance-relay-claim`, run
   in that module, builds the command from the pinned source.
   `go mod tidy` marks every line `// indirect`, the pin among them,
   because the module reaches each only through the `tool` directive.
   The `relay-claim` line is the pin. The `conformance` line is the
   harness, at the version the pinned release names; it is not a pin,
   and nobody edits it. The other lines are the harness's third-party
   modules, at the versions the release lists. The `go` line is the one
   the pinned release's own `go.mod` has (below); in the model that was
   `go 1.26`. `go.sum` holds the hashes of all of them, the harness
   included, and the Go checksum database verifies each. The kit's own
   `go.mod` gains no dependency.

   *Each command is built from exactly what its release names.* Go builds
   one module with one version of each dependency. A module that requires
   one specification takes its whole build list from that specification's
   own `go.mod`: the harness version it names and the third-party
   versions it lists. That is the list the `release` workflow's
   `go install <command>@<tag>` builds from (Decision 4). So what tests a
   pinned release here is built from the same modules, at the same
   versions, as that release's binaries: in the model the two builds
   reported the same module versions with the same hashes. The source
   is the release's, and so is what Go takes from a module's `go` line.
   The compiler is the Go that runs here, which is this repository's
   unless the release names a newer one (both below). Two
   specifications pinned at releases with different harness versions run
   each with its own, and any set of pins builds, the newest of each or
   not: the model built one specification at a release with a newer
   harness beside another at a release with an older one, where the
   harness had renamed a function the older cases call.

   *A module is written from nothing, never edited in place.* Moving a
   pin with `go get` leaves the other lines where they were: in the
   model, taking a specification back to an older release kept the newer
   harness line, `go mod tidy` accepted it, and the older cases did not
   compile. So `make pin-specs` writes each module it moves from nothing:
   the `module` line, the `go` line, the `tool` directive and the one
   requirement, then `go mod tidy`, with the old `go.sum` removed. The
   `pin` job does the same for the versions the files name and fails on
   a difference (**CI**), which is what holds the harness line and the
   third-party lines to what the release names. Nothing else moves a
   pin: the repository has no dependency bot, and one that is added
   later is not given these six directories, because its `go get` is the
   in-place edit this paragraph rules out and it would not rewrite the
   book page.

   *The `go` line is the release's.* `make pin-specs` writes on each
   module's `go` line what the pinned specification's own `go.mod` names
   at that version, which it reads before it writes the module, from
   the file `go list -m <module>@<version>` reports as `GoMod`. It does
   not write this repository's. In a module that holds no Go code
   the line still decides something: Go takes a build's default
   `GODEBUG` settings from the `go` line of the module the build is run
   in, and `go install <command>@<tag>`, which builds the release's
   binaries, takes them from the specification's. In a trial, a
   specification that names `go 1.24` was built with Go 1.26.3. In a
   pin module with `go 1.24`, `go tool` built a command whose
   `go version -m` output is that of the `go install` build, line for
   line, and `go build` there wrote the same file as `go install`, byte
   for byte. In a pin module with this repository's `go 1.26.3` the
   build lost its `DefaultGODEBUG` line, which there set `tlssha1` and
   `urlstrictcolons` among others. The line
   also does not depend on the Go that writes it: Go 1.26.3 and
   Go 1.26.11 wrote the same `go.mod` and `go.sum`, which the `pin`
   job's comparison needs, and a change to this repository's own `go`
   line touches no pin. `go mod tidy` raises a line that is lower than
   one a dependency names and writes no `toolchain` line; a
   specification's `go.mod` is tidy at its release (**Releases and how
   they are consumed**), so the line stays as written. A release that
   names a newer Go than the one that runs here cannot be built here
   until this repository's Go moves, as its `go install` cannot: with
   `GOTOOLCHAIN=local` the move of the pin stops with Go's message
   ("requires go >= 1.27 (running go 1.26.3; GOTOOLCHAIN=local)") and
   writes no `go.sum`, and with `GOTOOLCHAIN=auto` Go fetches that
   toolchain. The released binaries need no Go. *Rejected:* this
   repository's own `go` line, from the root `go.mod`. It is as
   repeatable, but the build then differs from the release's as the
   trial shows, and each change of this repository's Go would rewrite
   the six modules.

   *No workspace.* This repository has no `go.work`, and this item adds
   none. A workspace that lists the six modules gives them one build
   list, and so one harness for all six: in the model the mixed set
   above then failed to compile, as it does in one module. The root
   module, `tools/tier1` and `tools/workflow` are not joined by a
   workspace either; `make test` and `make lint` enter each. The six
   modules hold no Go package, so `go build ./...`, `make test`,
   `make lint`, `make tidy` and the `auto-tidy` workflow, which work on
   the root module, do not see them. The repository vendors nothing. The
   recipes that use the six modules set `GOWORK=off`, so that a `go.work`
   a developer keeps above the checkout cannot join them.

   *Each seam's own contract path.* Each module's `go.mod` is the
   contract path tier 1 checks for the seam its specification belongs
   to: a change to a file listed for `relay-claim` must change
   `tools/conformance/relay-claim/go.mod`, and so on. Seam 2 has two
   specifications and so two pin files, and tier 1's list gives it an
   entry for each. **Tier 1's seam list** says what the check holds and
   what it leaves to the suites and to Review.

   *A release on trunk, a commit only on a branch.* On a pull request and
   on `main` every module names its specification at a release tag, its
   harness line is a release tag too, and it carries no `replace` line.
   The harness's third-party lines are as the release names them, and
   one of those may be a commit. An unreleased commit of the specs
   repository is for work in progress: the `conformance` workflow runs
   the suites with it on a branch push and fails it on a pull request.
   LOOP-07 has the same rule for a component's trunk and its branches
   (point 5), and leaves how a release is pinned to the component, which
   says it in its profile (LOOP-01; **Rule changes**, PROFILE-01).

   *One command moves the pins,* `make pin-specs`, in one of two forms.
   `SPECS="<directory>@<version> ..."` moves the named specifications to
   release tags and leaves the other modules as they are, and
   `SPECS=latest` moves all six to their newest releases, the set the
   specs repository tested together (Decision 4). `REF=<commit>` pins
   all six at one commit of the specs repository, for a branch in
   progress. A specification's `go.mod` at such a commit may name a
   harness version that is not tagged yet. In each module where it does,
   this form also writes one `replace` line that points the harness
   module at the same commit. A module whose specification names a
   harness version that has its tag gets no such line, and is built with
   that harness, as its release is. The command asks whether the version
   exists before it writes the module; nothing is tried and then
   repaired. After `REF`, all six are at a commit, so the move back to
   releases names all six or is `SPECS=latest`. In either form the
   command rewrites the `go.mod` and `go.sum` of each module it moves
   and the one book page that links into the specs repository (**The
   book and the specifications link both ways**), and builds each
   command it moved.

   *Which sets are tested.* The newest release of each specification is
   tested as a set in the specs repository, and `SPECS=latest` is the
   set this design supports. An implementation may hold one
   specification at an older release while it moves another. Here that
   set builds, and each suite runs as its release built it. What it does
   not promise is that the older suite can still drive the component: a
   suite reaches a relay through the claim, the collector and the build
   as the harness of its own release speaks them, and a relay that has
   moved to a newer release of one of those protocols may refuse an
   older claimant. Such a suite fails, and the remedy is a release of
   that specification with the newer harness (Decision 4). A set other
   than the newest of each is tested by this repository's `conformance`
   workflow and by nothing else.

   *Rejected:* one `go.mod` for all six specifications, which is what
   a7's answer named and what this design had until a20 was answered.
   Go builds one module with one harness, the highest any pin names. A
   specification pinned at a release whose harness is older was then
   tested here with a newer harness than its binaries carry, a set that
   was not the newest of each could pass every check the `pin` job then
   had and not compile, and tier 1 had one contract path for five seams
   (a20, option A). *Rejected:* the released binaries, fetched and
   checked against a file of hashes (a20, option B): it is the lock file
   and download script below, and a branch in progress pins a commit
   that has no binaries. *Rejected:* a git
   submodule (tier 1 reads files
   with `git show`, which does not see into a submodule; worktrees and CI
   checkouts each need extra steps). *Rejected:* a vendored copy of the
   documents, which is the duplication the specs repository exists to remove.
   *Rejected:* a plain lock file and a download script, which would
   reimplement what the module system already verifies. *Rejected:* a pin
   file for each seam beside one `go.mod`. Each would repeat a version
   that `go.mod` already states, with a test to keep the two alike.

8. **This item works in two repositories, from the one queue, from the
   start of Implement.** It names them here, as LOOP-04 has an item do
   until the queue records them: `buildwarden/buildwarden`, which
   holds its design, its queue item and its reviews, and
   `buildwarden/buildwarden-specs` (a4). The documents, harness and suites
   are written on a branch `feat/seam-specs` of the specs repository and
   reach its `main` through a pull request Jeff merges. This repository's
   branch holds the pins, the target files for the Go relay and the disk
   collector, the `conformance` workflow, the book links, tier 1's seam list
   and the rule changes. `wf` runs no gate in the specs repository. The
   loop's mechanical checks reach it through the pins: this repository's
   `conformance` workflow cannot pass unless the pinned specs commit exists
   and its suites pass against the Go relay and the disk collector, first on
   a push of this branch during Implement and again on the pull request
   (**How the loop treats the second repository**). *Rejected:* writing
   everything in a staging directory on this branch and exporting it at
   Publish. It keeps the work in one repository, but the branch that Review
   reads would not be the branch the PR merges (the staging tree has to be
   deleted and the pins switched after Review), and the relay repository
   could read no draft until Publish. *Rejected:* waiting for
   `loop-repository`, the P1 item that gives `wf` its gates, branches and
   pull requests in a second repository. The umbrella's order puts S1 in
   wave 1, the relay has no written target until S1, and an item may work in
   a repository that has not adopted the Loop Contract (LOOP-08).

9. **Seam 4 moves at M6, with RELAY-01 to RELAY-08, and not in phase S1**
   (a1, Jeff's answer of 2026-11-15). The ledger format and its schemas
   stay under `specs/` in this repository until then, and RELAY-05 is not
   changed by this item or by any other item of S1. The umbrella's seam
   table says the format "moves to the specs repository" and names no
   phase, and its table of rule changes lists RELAY-05 for S1; both are
   amended to name M6 (**Amendments to the umbrella design**). The
   harness's ledger verifier is written from `specs/build-ledger/v1.md`
   here (Decision 5). *Rejected:* a later item of phase S1, which this
   design proposed until the review; Jeff chose M6, where the rule that
   governs the format moves too. *Rejected:* in this item (a1, option B).

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

12. **The releases and the pins' move to them are one human step, after
    Review** (a15). Nothing is merged or tagged in the specs repository
    until the suites have passed on hosted runners for this branch and tier 2
    has read the specs branch. Then `f6-release-specs`: Jeff reads the tier-2
    report and merges the specs pull request. Only he merges it (FLOW-08).
    The tags are then pushed on the merge commit, the harness's first and
    then one for each specification, each in its own push (Decision 4),
    and the pins moved on this branch, by him or by
    the attended session on his word. The merge is the sign-off, as FLOW-08
    has it. The step is the queue's record of a merge in a repository `wf`
    cannot see, and what the tags and the pins' move wait on. Two sentences
    of the rules as they stand do not provide for such a step: FLOW-04's
    list of what a `human` step is for, and FLOW-08's "There is no sign-off
    step in the queue". This design declares a change to neither, and may
    not: both are loop rules (LOOP-09, FLOW-14). a15 asks for the step as
    an exception, and says which later item makes it unnecessary and
    which would carry a rule change if Jeff wants one.
    *Rejected:* `f6-open-pr` moves the pins. Its definition in
    `.claude/agents/wf-publish.md` has no such duty, and a feature cannot
    change `.claude/` (FLOW-14). *Rejected:* no step, and a blocking
    question raised in Publish that names the release (a15, option B).
    It is how LOOP-04 holds an item that waits on another component's
    release, and it needs no exception. It is not taken because the
    release would be asked for only once the item holds the PR slot,
    and nothing would show in the queue before then; it is what the
    design falls back to if the review does not grant the step.
    *Rejected:* releasing before Review and moving the pins in
    Implement. The specs content would be merged and tagged before tier 2
    had read it, and a finding would cost a second release.

13. **The collector's outputs specification is normative for the wire; the
    disk layout is the disk collector's** (a16). `collector-outputs`
    requires what a collector accepts, what it replies and in what order.
    The layout `warden inspect`
    reads is specified as the disk collector's, in a section other collectors
    need not meet. *Rejected:* an abstract object model for any collector's
    storage. Nothing on `main` defines one, and the umbrella's review put the
    collector's storage boundary off to the first alternative collector (its
    a2).

## Questions

### Blocking

None. The repository's creation is Jeff's (umbrella a11). By his answer
to a2 it is created during the design review, before the approval, and by
Jeff himself (**What Jeff creates**); Implement checks it before it starts.
The release at the end is Jeff's too, and is a queue step, not a question
(a15). Whether that step may exist is a15.

### Assumed

**Answered on 2026-11-15.** Jeff answered a1 to a18 in the first sitting
of the design review. Each entry below keeps its options and says which
he chose. Twelve were confirmed as recommended: a3 to a6 and a11 to a18.
Five depart from what the design proposed, and the design is revised for
each: a1 (C), a2 (B), a8 (B), a9 (A, and OpenAPI for the HTTP seams as
well) and a10 (B). a7 was restated for a8's answer and then confirmed.
a19 and a20 were raised by the fold-in of a8 and by its review, and he
answered both as the sitting continued, each as recommended: a19 (A),
which the design already stood on, and a20 (C), which it did not, and
for which it is revised (Decision 7).

Each is confirmed in the design review (FLOW-19). The same entries are in the
queue file. a1 to a16 date from `f1-design`; the revision of 2026-11-14
reworded a4, a6, a14 and a15 and added a17 and a18, and the pass on its
review reworded a4 and a15 again. None is retired. The revision of
2026-11-15, after `loop-contract` merged, changed no option and no
recommended answer: a4 and a15 cite the merged rules where they cited
`loop-contract`'s questions, a15 gains what LOOP-09 adds, and a6 records
an answer given in `driver-contract`'s review. The pass on its review,
the same day, added to a15 what LOOP-04 says of an item that waits for a
release; its recommended answer is still A.

- **a1. Seam 4's move.** The umbrella's seam table says the ledger format
  "moves to the specs repository" and gives no phase. Options: (A) a later
  item of phase S1, filed when this one's release mechanism has run once;
  (B) in this item; (C) at M6, with RELAY-01 to RELAY-08.
  **Answered 2026-11-15: (C), at M6, with RELAY-01 to RELAY-08.** The
  design proposed A; it is revised (Decision 9). The move rewrites
  RELAY-05, tier 1's `schema-doc` root, the book's `specs/` mirror and its
  redirects, `specs/specs_test.go` and the `$id` policy for new ledger
  schemas, each of which a third party can see. With C all of that is
  M6's, in the item that moves the relay's rules, and no item of S1 is
  filed for it. Until then the specs repository's README names the ledger
  format as specified under `specs/` here and links to it, and the
  harness's ledger verifier is written from `specs/build-ledger/v1.md`.
  Consequence: RELAY-05 is not changed in S1, although the umbrella's
  table lists it beside TEST-05 for S1. The umbrella's seam 4 row and that
  row of its rule table are amended to name M6 (**Amendments to the
  umbrella design**), for Jeff's approval by name.
- **a2. When the repository is created, by whom, and what has to exist.**
  Options: (A) in or right after the design review, by Jeff or by that
  attended session on his word (umbrella a11); (B) now, before the review;
  (C) at Publish, after incubating on this branch (Decision 8's rejected
  alternative). **Answered 2026-11-15: (B), now, before the review's
  approval.** The design proposed A; it is revised. What is created was
  confirmed with Jeff in the review session, right before creation
  (umbrella a11): the name `buildwarden/buildwarden-specs`; public (a3);
  a first commit that carries a README stub and the Apache-2.0 `LICENSE`
  and no `NOTICE` file. Jeff creates it himself, because the cloud session
  that holds the review cannot create a repository; the session then
  attaches it. **What Jeff creates** lists the settings and what Implement
  checks before it starts. If the repository is missing then, or a branch
  cannot be pushed to it, Implement stops and the tick records a blocking
  question for the human, as `build-vm-busybox` q1 did. No agent creates
  it, and no agent adds a licence file to it.
- **a3. Visibility.** Options: (A) public from creation; (B) private until
  `buildwarden-relay` goes public at M1.
  **Answered 2026-11-15: (A), public from creation.**
  It specifies protocols of a public project, the book links to it, and the Go
  module proxy and checksum database serve only public modules: a private
  repository would need credentials in this repository's CI and in the
  relay's.
- **a4. Which invariants apply to work in the specs repository, which has
  not adopted the Loop Contract.** *(Reworded 2026-11-14 for one queue,
  and again after the independent review: the question is now the list of
  invariants alone. On 2026-11-15 its citations were brought up to the
  merged rules; the options and the answer proposed are as they were.)*
  There is one queue, for work in every repository, and an item names
  the repositories it works in (LOOP-04). This item names two
  (Decision 8). The specs repository has no profile and no invariants of
  its own, so it is not a component; it adopts the contract when Jeff
  files the item that does that. What binds a worker in such a
  repository is not asked here. LOOP-08 says it: the worker is bound
  there by FLOW-08, FLOW-16, FLOW-17 and FLOW-18, and the item's design
  says which invariants apply, by rule ID. That rule is Jeff's answer to
  `loop-contract` a17 of 2026-11-14, which also declined what was this
  question's option B, that the specs repository adopt first and S1
  wait. Its option C, no rules there, cannot stand beside the
  four loop rules. What is left for this review is the list. Options:
  - (A) GLOBAL-01 to GLOBAL-04, GLOBAL-07, GLOBAL-09 to GLOBAL-11, TEST-04
    and TEST-07 for the harness's Go code, and RELAY-05's schema-identity
    rule for the schemas. These are the rules a reader can judge from the
    code and the schemas alone. Review's tier-2 report judges the first
    group and the specs repository's `ci` holds the second (**How the loop
    treats the second repository**).
  - (B) A wider list: A, and also GLOBAL-05 for the harness and its
    release binaries, a GLOBAL-06 note for the sandbox, which runs as root
    and creates namespaces, and UX-02, UX-04, UX-05 and UX-09 for the
    `bw-conformance` command line and its report. Tier 2 then reads more
    in a repository where no tier 1 runs. GLOBAL-05 and GLOBAL-06 are
    written for a build under `warden` and for the product's three
    boundaries, and the UX rules for `warden`'s surfaces, so each would be
    applied to something it was not written for.
  - (C) Only what the specs repository's own CI can enforce: the layout
    and `$id` check, `go vet`, `go test` and the check that the module
    does not require the kit. Review then reads nothing in the specs
    repository, and the harness's code is held to no rule of this
    project.

  **Answered 2026-11-15: (A), the rules a reader can judge from the
  code and the schemas alone.**
  Naming the list in the design is the
  normal way to work in such a repository, not a stopgap until P1: it is
  the list LOOP-08 asks an item's design for. The **Security note** says
  what the sandbox does whichever option is taken.
- **a5. Harness language, home and form** (Decision 5). Options: (A) Go, in
  the specs repository under `conformance/`, importing nothing from the kit,
  released as a module tag plus static `bw-conformance` binaries with
  checksums; (B) Rust; (C) a separate harness repository; (D) Go tests in
  this repository that import the kit, which the umbrella's Decision 3
  rejects.
  **Answered 2026-11-15: (A).**
  *(2026-11-15: a8's answer does
  not change this one. It changes what the words cover: the shared
  harness stays under `conformance/`, each specification's cases move
  into its own module, and the module tag and the `bw-conformance`
  binaries are one set for each specification, `bw-conformance-<directory>`
  (Decisions 4 and 5).)*
- **a6. Launchers built now.** Options: (A) `exec`, `oci` and `external`
  now; the VM-image launcher (qemu first) is built in the specs repository
  by the relay's M2 or M3 item, or by seam 8's item, whichever first has an
  image and a CI that boots it; (B) build a qemu launcher here and validate
  it on `qemu-any`.
  **Answered 2026-11-15: (A), `exec`, `oci` and `external` now.**
  The Go relay's VM form adds
  no relay code to `-mode vm`, which the `exec` launcher covers in a network
  sandbox. What a VM adds is its init and boot channel
  (`tools/relay-vm/init`, the relay seed), which belong to seam 8 (relay
  release forms), not to seams 1 to 3. No phase writes seam 8 yet:
  Jeff's answer to `driver-contract` a11 is that it is filed as its own
  S-track item, to exist before M6 adopts a release, and none is filed.
  *(Reworded 2026-11-14: one sentence added. On 2026-11-15 the sentence
  before this note was brought up to that answer.)* A is narrower than
  the umbrella's S1 row, which has the harness run "against any relay
  binary or image": the OCI image is run here, and the Go relay VM image
  and the Hyper-V relay VM are run by no suite when S1 is done (**Where
  this design is narrower than the umbrella**).
- **a7. How this repository pins a specs release** (Decision 7). Options:
  (A) `tools/conformance/go.mod`, a nested module that requires the specs
  module at one version: a release tag on a pull request and on `main`, a
  pseudo-version only on a branch while work is in progress; the file is
  tier 1's contract path for the five seams; (B) a git submodule; (C) a
  vendored copy of the documents; (D) a lock file and a download script.
  **Answered 2026-11-15: (A), restated for a tag per specification.**
  When first asked, Jeff said: "What is meant by this repository and
  where by comparison are the specifications? I assumed that a specific
  release of the specs themselves is coincident with the build of the
  conformance tools that test them". The question was then put again
  with a8's answer in it, and he chose A in this form:
  `tools/conformance/go.mod` names each specification's module at a
  version, released tags on a pull request and on `main`, an unreleased
  commit only on a branch in progress. The design is revised
  (Decision 7). The file is still tier 1's contract path for the five
  seams.
  *(2026-11-15, later in the sitting: a20's answer, (C), replaces that
  one file with six, `tools/conformance/<directory>/go.mod`, one for
  each specification. The answer recorded above is as he gave it and is
  not edited. What it settled stands: a nested module that names a
  specification at a version, a release tag on a pull request and on
  `main`, an unreleased commit only on a branch in progress. Each file
  is tier 1's contract path for the seam its specification belongs to,
  so seam 2 has two (Decision 7, **Tier 1's seam list**).)*
- **a8. Versioning and release scheme** (Decision 4). Options: (A) a major
  version per document (`v<N>.md`, edited in place before 1.0) and semver
  tags `v0.x.y` for the repository as a whole; a minor release may make a
  passing implementation fail, a patch release may not; the first release is
  `v0.1.0`; (B) a tag per specification; (C) the harness versioned apart
  from the documents. **Answered 2026-11-15: (B), a tag per
  specification.** The design proposed A; it is revised. Read with his
  remark on a7, each specification is versioned and tagged on its own,
  and a specification's release carries the build of the conformance
  tooling that tests that specification. Decision 4 says what a release
  is in git, what it carries, what minor and patch mean for each
  specification, how a document's `v<N>` and a tag's version relate, and
  what the scheme costs beside the single tag. Decisions 2, 5 and 7,
  **The harness**, **Releases and how they are consumed**, **CI** and
  the proposed wording of four rules follow it. It raised two
  questions, both answered since: a19, where the code that every suite
  shares lives, and a20, which build of a release's tooling this
  repository runs.
- **a9. Document format** (Decision 3). Options: (A) Markdown prose with RFC
  2119 words and a stable identifier per requirement that suite cases cite,
  plus JSON Schema 2020-12 for JSON bodies; the specs repository's CI fails
  on a requirement with no case and no stated reason; (B) OpenAPI for the
  HTTP seams; (C) prose without requirement identifiers. **Answered
  2026-11-15, changed, in Jeff's words:** "Similar to the Markdown with
  requirement IDs and CI failing on requirements without test cases or
  conformance testing, but also with the OpenAPI for the HTTP seams as
  well". So (A), and for the HTTP seams an OpenAPI description too. The
  design proposed A alone and had B as a rejected alternative; it is
  revised (Decision 3, **The OpenAPI descriptions**). Four specifications
  get one: `relay-claim`, `collector-outputs`, `collector-inputs` and
  `relay-guest-api`. The requirements stay the contract, and checks hold
  the OpenAPI files to them.
- **a10. How the collector specification's two parts are laid out**
  (umbrella a1): the outputs a collector stores, and the inputs and
  signals it serves. Options: (A) one document, `collector/v1.md`, with
  Part A and Part B, each with its own requirement prefix and its own
  suite; (B) two documents (`collector-outputs/`, `collector-inputs/`).
  **Answered 2026-11-15: (B), two documents.** The design proposed A; it
  is revised (**Seam 2**). Each is a specification of its own, with its
  directory, its requirement prefix (`COLLOUT-<n>`, `COLLIN-<n>`), its
  suite and, under a8, its releases, so an implementation can claim one.
  The two share transport, authentication, the path rule and the session.
  Those are written in full in both documents and kept the same by a
  check, and are not put in a third document, because each is released on
  its own (**Seam 2** gives the reason and the cost). Nothing on the wire
  changes: the claim still names one `collector_url`. Seam 2 stays one
  seam, `collector`, in the umbrella's numbering, in GLOBAL-08 and in
  tier 1's seam list.
  *(2026-11-15, a20: in tier 1's list the seam `collector` now has two
  entries, one for each specification's pin. It is still one seam, and
  both entries carry its name.)*
- **a11. The executor's own suite is deferred to S2's item.** Options: (A)
  defer: S2 moves truststore generation out of `initialize`, so a suite
  written now would pin the behaviour S2 removes; (B) build it here, for
  Linux guests in a container, with Windows and macOS guests as tracked
  gaps.
  **Answered 2026-11-15: (A), deferred to S2's item.**
  Until then EXEC-04's conformance stays
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
  GLOBAL-06 and every driver's native validation.
  **Answered 2026-11-15: (A), record each and fix none here.**
- **a13. Seam 6's name.** The umbrella's seam table gives seam 6 no name.
  Options: (A) `bootstrap-config`, added to GLOBAL-08's executor bullet and
  to tier 1's seam list; (B) `bootstrap-model`, which the umbrella's
  front-matter lists and which is also the name of the BOOT-07 model.
  **Answered 2026-11-15: (A), `bootstrap-config`.**
- **a14. Native validation** (**Validation plan**). *(Reworded
  2026-11-14.)* Options: (A) nothing blocking; two tracked gaps, both
  deferred: `macos-vz` until the `exec` launcher has a macOS form, and
  `qemu-any` until a VM launcher exists; (B) build the macOS form of the
  launcher in this item, and keep the `macos-vz` gap open, to be run once
  the item has merged; (C) block on `macos-vz`, which needs B's launcher;
  (D) record no gaps.
  **Answered 2026-11-15: (A), nothing blocking, and two tracked gaps,
  both deferred.**
  At `f1-design` the
  `macos-vz` gap was recorded open. Nothing this item builds can start that
  run: the relay targets run on Linux only, and the sandbox is Linux
  namespaces. An open entry wakes the MacBook's scheduled session for a run
  it cannot make.
- **a15. The release step, and what it rests on** (Decision 12).
  *(Reworded 2026-11-14, and again after the independent review.)* After
  Review three things happen in order: Jeff merges the specs pull request,
  which is FLOW-08's merge in that repository and his alone; a release is
  tagged; and the pin on this branch moves to the tag. `wf` records one
  branch and one pull request for an item, in this repository, so it
  cannot see the first, and the loop has to wait for it somewhere Jeff
  will see. Two sentences of the rules as they stand do not provide for a
  queue step that does that:
  - FLOW-04 (MUST, automated) lists what a `human` step is for: blocking
    questions, the design review and escalations. A step for a merge in a
    second repository is none of those.
  - FLOW-08 (MUST, automated), which binds in the specs repository
    (LOOP-08), says "There is no sign-off step in the queue:
    the loop sees the merge". The loop cannot see this one.

  The merge remains the sign-off, and nothing else signs off. The step is
  only the queue's record that the merge happened, and what the tag and
  the pin's move wait on. This design declares a change to neither rule: a
  feature's design may not declare a change to a loop rule (FLOW-14 as
  amended, LOOP-09). *(On 2026-11-15 this paragraph and the next were
  brought up to the merged rules; the options and the answer proposed
  are as they were.)*

  LOOP-09 is on `main` since this question was written, and bears on
  option A. It gives every change to a loop rule to a process item, and it
  provides for no exception granted in a feature's design review. Option A
  changes no rule's text and ends with this item; whether an exception
  that the owner of the rules grants for one item is within LOOP-09 is
  his to judge at this review. Options B, C and D ask for none.

  LOOP-04 merged with it, and bears on option B. *(Added 2026-11-15, on
  the review of that day's update.)* Its last bullet holds an item that
  waits on another component "with a blocking question that names the
  release (FLOW-07)", and the item "resumes when the release exists".
  That is option B's mechanism, and it is what this design already does
  when a return to Implement needs a second release (**The release
  step**). The bullet does not bind here. The specs repository has not
  adopted the contract and is not a component (LOOP-08). And this item
  does not wait on work that is another item's: the release is its own
  product, made by the merge of its own pull request in a repository it
  names (LOOP-04). What the queue lacks is a way to see that merge, not
  a way to wait for a release. The bullet still shows what the merged
  rules do where a release is waited for, and B asks for no exception.
  Options:
  - (A) A `human` queue step `f6-release-specs` in phase 6, as an exception
    to those two sentences that this review grants for this item and no
    other. Jeff merges the specs pull request. The tag is then pushed and
    the pin moved on this branch (`make pin-specs`, `make test`,
    `make lint`, push), by him or by the attended session on his word. The
    step needs `f2-design-review`, so `wf` does not offer it before this
    question is confirmed. It cannot be held back until Review: `wf`
    offers a `human` step whatever its phase, and a step can only need one
    that already exists. Its note says not before Review is done, there is
    no specs pull request to merge until Implement's task 11, and the PR's
    `pin` job stops a pin that is not a release tag.
  - (B) No step now. The Publish step that finds the pin on a
    pseudo-version stops and raises a blocking question that asks for the
    release, with its answer step, as Implement does for a missing
    repository (a2). That is inside FLOW-04's list, and a step that
    answers a question is no sign-off step. Nothing shows in the queue
    until Publish, the release cannot be made while the item waits for the
    PR slot, and the slot is held while Jeff releases.
  - (C) No step: `f6-open-pr` opens the PR on the pseudo-version, the `pin`
    job fails, and FLOW-11's escalation asks Jeff for the release. The PR
    is red until then.
  - (D) S1 waits for `loop-repository`.

  **Answered 2026-11-15: (A), the human step `f6-release-specs`, as an
  exception for this item only.** Under a8's answer the step pushes seven
  tags and moves six pins, where this question's text says the tag and
  the pin (Decision 12).
  The recommended answer is unchanged by
  LOOP-09 and LOOP-04, for two reasons neither rule answers. Under A the
  step is in the queue from the design review, where Jeff sees it; under
  B nothing shows until Publish. And under A the release can be made as
  soon as Review is done, before the item takes the PR slot (FLOW-05);
  B raises its question in Publish, and the slot is held while Jeff
  merges and the tag is pushed. If Jeff judges that the exception is not
  one to grant in a feature's review (LOOP-09), B is the answer the
  merged rules already provide for, at that cost. The design then drops
  the step: Decision 12, **The release step** and task 13 are rewritten
  for the question, and Design removes `f6-release-specs` from the queue
  file.

  Under A no rule changes, and the exception ends with this item. Three
  things could change, each in a later item and none in this design:
  - *What makes the step unnecessary* is a tool change, not a rule change.
    `loop-repository`, the P1 item that gives `wf` its gates, branches and
    pull requests in a second repository, lets an item record the specs
    pull request. The loop then confirms its merge as it confirms this
    repository's (FLOW-03, phase 7), with no step for Jeff to claim, and
    FLOW-08's sentence is true of the specs repository as written.
  - *If Jeff wants the rules to provide for such a step in the meantime*,
    the change is one clause in FLOW-04's list and one in FLOW-08's
    sentence, and a process item carries it (FLOW-14). Two of P1's are
    placed for it. `loop-parallel` brings in items tracked in another
    repository, whose steps are Jeff's or an attended session's (the
    change proposal, section 3). `loop-native-lead` is the item whose
    list of rules has FLOW-04 (`loop-contract`, **What this item hands to
    the later items**); FLOW-08 is on no later item's list there.
    This design proposes `loop-parallel`: it is the earlier, and it is
    where such steps come to exist. The proposal has FLOW-08 staying as
    written, so its clause is an addition Jeff would ask of that item.
    *(Added 2026-11-15.)* `loop-parallel`'s design, in Design and not
    reviewed (read at `486ac8f`, a draft), now proposes both clauses and
    asks them as its a6: a `human` step records a merge in a repository
    where the loop's gates do not run. If that is approved and merges
    before this item reaches `f6-release-specs`, the rules provide for
    the step by then, and the exception covers only the time between.
  - *Holding a `human` step back until its phase* is a change to `wf` and
    to FLOW-04's sentence on when a step is offered ("unless it is
    `human`, its feature has reached the step's phase"), for the same
    item.
- **a16. What the collector's outputs specification is normative for**
  (Decision 13; "Part A of the collector specification" when it was
  asked, before a10's answer). Options: (A) the wire only: the requests a
  collector
  accepts, its replies and their order. The disk layout (`artifacts/<name>`,
  `payloads/<hash>`, `captures/<name>`, `ledger`, `environment.json`,
  `build-output.log`) is specified as the disk collector's, and a way to
  read stored bytes back is something a target file may give the harness,
  not a requirement on a collector. (B) An object model for any collector:
  each output is an object a reader can fetch by its name with the bytes
  sent.
  **Answered 2026-11-15: (A), the wire only.**
  The umbrella's review (its a2) decided
  that the collector gets no storage boundary in this program and that one
  is designed with the first alternative collector. B would decide part of
  it here.
- **a17. Which relay files tier 1's seam list names** (**Tier 1's seam
  list**). *(New 2026-11-14.)* `relay/relay.go`, `relay/proxy.go` and
  `cmd/relay/` (all but `dhcp.go`) hold part of seams 1 and 3: the reserved
  hosts' handlers, the proxy, and what each mode does with a claim. They
  also hold most of the rest of the relay. Options: (A) leave them off the
  list. A change there needs no pin move. The pinned suite fails a change
  in behaviour that a case tests; an addition that no case tests (a new
  `artifacts` path, a new step in one mode's claim) is caught only by
  Review's judgment of GLOBAL-08. (B) List them. Every change to the relay's
  core then needs a pin move, or a `Tier1-Allow` trailer that says why it
  is not a seam change.
  **Answered 2026-11-15: (A), left off the list.**
  The changes the
  program still makes to the Go relay's seams are items that change the
  specification first by their own designs (S2's truststore endpoints),
  the freeze (FLOW-20) keeps other changes few, and the Go relay is retired
  at M6. Under either option the check asks only that the pin moved, to any
  release; whether that release changed the right specification is Review's
  to judge.
  *(2026-11-15, a20: each seam now has its own pin, so the check asks
  for the pin of the seam whose file changed. Whether the release it
  moved to states the change is still Review's to judge. The answer
  stands.)*
- **a18. What `make conformance` runs when no target is named**
  (**Defaults and overrides**). *(New 2026-11-14.)* The relay targets run
  only on Linux; the disk collector's runs on Linux, macOS and Windows.
  Options: (A) every target whose file declares the host's OS (`os`), with
  a line printed for each target left out and why. A target named in
  `TARGETS` that cannot run on the host is an error, and each CI job names
  its targets. (B) Every target in the directory, as at `f1-design`. The
  default then fails on macOS and Windows, and a developer there has to
  know to name `collector-disk`.
  **Answered 2026-11-15: (A), the targets that declare the host's OS.**
  A is not a
  fallback (GLOBAL-11): which targets run is decided from what each file
  declares, before anything starts, and nothing is tried after a failure.
- **a19. Where the code that every suite shares lives, and how it is
  versioned** (Decisions 4 and 5). *(New 2026-11-15, raised by a8's
  answer.)* With a tag per specification, the
  launchers, the sandbox, the target-file format, the runner and the test
  side of each protocol are used by all six suites and belong to no one
  specification. A relay cannot be tested on one seam without the others
  (**The harness**), so that code cannot simply be divided among the
  six. Options:
  - (A) One more module in the specs repository, `conformance/`, tagged
    on its own (`conformance/v0.x.y`). Each specification's module names
    the harness version it is released with, and its binaries are built
    with exactly that. A change that needs both a specification and the
    harness is one pull request and one merge, with the harness's tag
    pushed first. Cost: a seventh version number, though no
    implementation pins it. And in such a pull request a specification
    names a harness version that has no tag yet. Go does not build that
    as it stands once the harness imports anything outside the standard
    library, which it must. Three pieces of machinery make it build, each
    tried in a model (**Releases and how they are consumed**): a
    `replace` line in the workspace file that names the harness version
    in progress, edited each time that version rises; a command that
    tidies each specification's `go.mod` through a temporary `replace`,
    with a `ci` check that the result is complete; and a release built
    with `go install <command>@<tag>`, because a build in a checkout at
    the tag fails. A specification's `go.sum` never lists the harness. An
    implementation's branch in progress carries one `replace` line. The
    first real tags cannot be tried until they are pushed.
  - (B) The same module, tagged, and released ahead: a harness change is
    merged and tagged in a pull request of its own, before the
    specification that needs it. Nothing then names a version that does
    not exist, so none of A's machinery is needed: `go mod tidy` runs as
    it is, a release builds from a checkout, and no branch carries a
    `replace` line. Cost: two merges by Jeff for every change that needs
    both, and two pull requests for the first release, the second
    changing only `go.mod` files. A harness change is also merged before
    an implementation has run against it, unless that implementation's
    branch carries the `replace` line after all.
  - (C) The same module, never tagged: each specification's `go.mod`
    names the commit of the harness it was written against. Six tags,
    not seven, and the harness has no version of its own, which is the
    closest to "a release of a specification is the specification and
    the build of its tools". Cost: a released specification names an
    unreleased commit, and through it so does this repository's `main`,
    which sits badly with LOOP-07's point 5; and the specs repository
    must merge by merge commit, because a squashed pull request leaves
    the named commit outside `main`.
  - (D) One `bw-conformance` binary, built from a module that requires
    all six specifications. One command for a user, as before a8. Cost:
    its version is a set of six versions, which is the single tag under
    another name, and a specification's release no longer carries the
    tooling that tests it, which is what Jeff said he assumed.
  - (E) No shared module: each specification's module carries its own
    copy of the shared code. No shared version at all. Cost: six copies
    of the launchers, the sandbox and the models to keep alike, and
    every fix made six times.

  **Answered 2026-11-15: (A), a seventh module `conformance/`, tagged on
  its own; a change that needs both the harness and a specification is
  one pull request and one merge.** As recommended, and as the design
  already stood. It is the option in which a release names everything it
  was built from by a tag, and a change that spans the harness and a
  specification is still one merge. Its machinery is the specs
  repository's own and `ci` holds it; what it spares is a second merge
  by Jeff for each such change, and those are expected to be common
  while the specifications are below 1.0. B would have removed the
  machinery at the price of that second merge, and is not taken. The
  cases stay in each specification's module, and what Decision 4 says a
  tag per specification costs is the same under every option.

- **a20. Which build of a specification's tooling this repository
  runs** (Decision 7). *(New 2026-11-15, raised by a8's and a7's answers
  and by the review of their fold-in. The paragraph and the options are
  as they were put to Jeff, when the design had one `go.mod`.)* A
  release of
  a specification carries binaries built with the harness version its
  `go.mod` names (Decision 4). `tools/conformance/go.mod` names all six
  specifications in one module, and Go builds one module with one
  version of each dependency: the highest harness version any pin names.
  So a specification pinned here at a release whose harness is older is
  tested here with a newer harness than its own binaries have. When a7
  was first put to him Jeff said he assumed that "a specific release of
  the specs themselves is coincident with the build of the conformance
  tools that test them". On this repository's path that holds for the
  cases and not always for the harness under them. Options:
  - (A) As the design has it: one `go.mod`, `go tool`, one harness for
    all six commands. One file to read and to move, which is what a7's
    answer names, and one release of any specification brings a harness
    fix to all six suites here. Cost: this repository and an
    implementation that runs the released binaries, as the Rust relay
    may, can test the same pinned release with different harness
    versions; a set of pins that is not the newest of each can fail to
    build (Decision 7); and the six version numbers are independent for
    an implementation only as long as the harness stays compatible with
    every release it pins.
  - (B) The released binaries. `tools/conformance/go.mod` still names
    the six versions, and `make conformance` fetches each release's
    binary for the host and checks it against a hash kept in this
    repository. What runs is the release's own build, the one the relay
    repository runs. Cost: a file of hashes beside the `go.mod`, which
    `make pin-specs` rewrites, and a download step, which together are
    the lock file and script a7's option D was; a pin can move only once
    the release's binaries exist; and a branch in progress pins a commit
    that has no binaries, so the source build stays as a second path.
  - (C) A `go.mod` for each specification,
    `tools/conformance/<directory>/go.mod`, each naming one
    specification. Go then builds each command from exactly the modules
    its release names, which is what the `release` workflow builds from,
    so the six versions are independent here as they are for the
    binaries, and a mixed set always builds (tried in the model). Tier 1
    can be given each seam's own pin as its contract path, so the check
    asks for the pin of the seam that changed; the `collector` seam has
    two. Cost: six small modules and six `go.sum` files where a7's
    answer names one file; a harness fix reaches a suite here only
    through that specification's release, as for the binaries; and a7's
    text, Decision 7, **Tier 1's seam list**, the `pin` job and the
    proposed wording of TEST-05, GLOBAL-08, EXEC-04 and PROFILE-01 change
    where they name `tools/conformance/go.mod`.

  **Answered 2026-11-15: (C), a `go.mod` for each specification under
  `tools/conformance/<directory>/`.** As recommended. The design stood
  on A until he answered, because a7's recorded answer names one file
  and a fold-in does not change a recorded answer; it is now revised
  (Decision 7). C is the option in which what a8 chose is also true
  here: a specification's version is independent of the other five, and
  what tests a pinned release is built from what that release names. It
  keeps Go's own verification, which B gives up, and it removes the set
  that does not build. What it changed: a7 gains a note and keeps its
  recorded answer; Decision 7 and Decision 4's list of costs;
  `make conformance`, `make pin-specs` and the `pin` job; tier 1's seam list,
  where each seam has its own contract path and seam 2 two entries; and
  one path in the proposed wording of TEST-05, GLOBAL-08, EXEC-04 and
  two rows of PROFILE-01, re-derived before those four are put to him by
  name (**Rule changes**). The proposed TEST-02 sentence names the
  directory `tools/conformance/` and not a file, and is unchanged.

## Design

This item works in one repository other than this one:
`buildwarden/buildwarden-specs` (LOOP-04). Jeff creates it during the
design review (a2, **What Jeff creates**). It has not adopted the Loop
Contract, and **How the loop treats the second repository** says what
binds a worker there (LOOP-08).
The item reads `buildwarden/buildwarden-relay` and changes nothing in it.

Nothing in the relay, the collector, the executor or any driver changes. The
bootstrap model (BOOT-07) is untouched: its diagram, invariants, mapping and
deviation register stand, and this design carries no Bootstrap comparison.

### Where this design is narrower than the umbrella

The umbrella fixes what S1 is. In three places this design does less than
its words, or other than its table. Each is asked as a question, so that
the review accepts it knowingly. None reopens a decision of the umbrella.
Jeff's answer to a1 changes two of its sentences, which name no phase or
name S1 for what is now M6's; they are listed under **Amendments to the
umbrella design**.

| The umbrella says | This design | Asked as |
|---|---|---|
| S1's row: the harness runs "against any relay binary or image" | It runs against a binary (`exec`), an OCI image (`oci`) and a relay that is already running (`external`). No launcher boots a VM image, so when S1 is done no suite has run the Go relay VM image or the Hyper-V relay VM | a6 |
| "Seams 1 to 8 each get a specification and a black-box conformance suite in the specs repository" | Seam 5, and the executor's side of seams 3 and 6, get a specification and no suite. Seam 4 does not move in S1: it moves at M6 | a11, a1 |
| Its table of rule changes, "listed for direction": S1 changes RELAY-05 and TEST-05 | TEST-05, and also TEST-02, GLOBAL-08, EXEC-04 and PROFILE-01, a rule that did not exist when the table was written. RELAY-05 is left to M6, with seam 4's move | a1; **Rule changes**; **Amendments to the umbrella design** |

### Amendments to the umbrella design

Jeff's answer to a1 places the ledger format's move at M6, with RELAY-01
to RELAY-08. Two sentences of [`componentization`](componentization.md)
say otherwise or say nothing, and a third records the change. Each is
listed with its line on `main` at `ad4ea50`, its wording there and the
wording proposed. The umbrella was amended in place by `loop-contract` on
2026-11-14; the wording quoted is the amended text. They are made in this
item's PR (task 10), as `loop-contract`'s were. Jeff approved the three
by name on 2026-11-15, on the wording he read at `e19cc1f`, U2 in both
of its forms (**Design review**). The proposed wording in the table and
in the joined row below is byte for byte what he read.

| # | Line | Where | Wording on `main` | Proposed wording |
|---|---|---|---|---|
| U1 | 278 | **The seams**, seam 4's row, the column "Change in the program" | "Moves to the specs repository." | "Moves to the specs repository at M6, with RELAY-01 to RELAY-08." |
| U2 | 404, 405 | **Rule changes the program will make**, two rows | "\| RELAY-01 to RELAY-08 \| Move to the relay or specs repository, restated where they describe Go internals. \| M6 \|" and "\| RELAY-05, TEST-05 \| Contract tests for the seams become the specs repository's suites. \| S1 \|" | "\| RELAY-01 to RELAY-08 \| Move to the relay or specs repository, restated where they describe Go internals. RELAY-05 moves with the ledger format (seam 4). \| M6 \|" and "\| TEST-05 \| Contract tests for the seams become the specs repository's suites. \| S1 \|" |
| U3 | after 677 | **Amendments**, a new paragraph after `loop-contract`'s | None. | "**2026-11-15, by `seam-specs`**, approved in its design review. Jeff placed the ledger format's move to the specs repository at M6, with RELAY-01 to RELAY-08 (its a1). Amended in place: seam 4's row of **The seams**; the RELAY-01 to RELAY-08 row and the RELAY-05 and TEST-05 row of **Rule changes the program will make**. The wording each had is in [`seam-specs`](seam-specs.md), under **Amendments to the umbrella design**." |

Read and not amended:

- *S1's row* of Track S says "Write seams 1, 2, 3, 5 and 6", which already
  leaves seam 4 out.
- *M6's row* of Track R names the per-driver adoption items and the Go
  relay's retirement. It does not list the rules' move, which the rule
  table gives to M6, and so it does not list the ledger format's either.
  Which M6 item moves the format is for M6's intake to say.
- *"Seams 1 to 8 each get a specification and a black-box conformance
  suite in the specs repository"* stays true of seam 4 once M6 has moved
  it.
- The umbrella's Decisions are untouched: none names a phase for seam 4.
- *Nothing for a8, a9 or a10.* The umbrella's a1 has the collector
  specification "written in two parts"; two documents are those parts.
  Its components table gives the specs repository "One specification and
  one black-box suite per seam"; seam 2 now has two of each, which this
  design reads as what a1 already said. Decision 2 couples components
  through "tagged releases", which a tag per specification still is, and
  the umbrella says nothing of the documents' format.

`driver-contract` amends the umbrella too, in its own PR (its **Amendments
to the umbrella design**, read at `a6c8f0b`, where its review approved
it on 2026-11-15; its a18). Its list and this one meet in two places:

- *Line 405, the row U2 rewords.* U2 takes "RELAY-05, " out of the row's
  first cell. `driver-contract` changes its last cell, from "S1" to "S1;
  K1 for the driver suite", and leaves the rest. With both, the row
  reads:

  > | TEST-05 | Contract tests for the seams become the specs repository's suites. | S1; K1 for the driver suite |

  U2 was put to Jeff in both forms, and he approved it in both: as in
  the table above, which is the row if this item merges first, and
  joined as here.
- *The **Amendments** section.* Each design adds a dated paragraph after
  `loop-contract`'s. The two stand in the order of their dates, each as
  its review approved it; U3 is this design's.

The second of the two to merge keeps both changes to line 405 when it
merges `main` (`f6-merge-main`), as it does for the rule text (**With
`driver-contract`**), and brings its quotation of the line's current
wording up to date in Consolidation (FLOW-10). If the merge needed a
word that neither review approved, that is a return to Design (FLOW-06).

Pointed out to that design, which is not edited from here: it
quotes line 405 as current with "RELAY-05, TEST-05" in its first cell,
which is stale by U2 if this item merges first. None of its other
amendments is to a line listed here. One is to a sentence this design
reads and does not amend: at line 269 it makes "Seams 1 to 8" "Seams 1 to
9", and adds where seam 9's specification starts.

### What each specification covers

The lists below are what the documents must state, taken from `main` at
`b903f65`. `main` at `ad4ea50`, which this branch has merged, changed none
of the files they are taken from. The file named in each heading is where
the behaviour is defined today. A specification may say more; it may not
say less, and where it would differ from this list the code decides
(Decision 1).

#### Seam 1, `relay-claim/v1.md` (`relaycfg/`, `cmd/relay/main.go`, `vmlink.go`, `mode_host.go`)

Roles: the *relay* serves one claim; the *claimant* pushes it. The claimant
is a role, defined by what it does: it opens the connection, serves the
collector protocol on the tunnel when it selects the tunnel, and sends the
claim. The specification names no component for it. On `main` the claimant
is `driver.Session`, which each driver and `cmd/collector` start. Under
the redrafted driver contract `warden` starts it for every driver and
the driver reports the endpoint to claim at (its Decisions and a13,
which Jeff answered as that design recommended; its amendment to the
umbrella's seam 1, approved with it, reads "Claimant (`warden`'s
session) and relay"). Neither the roles nor the wire differ
between the two, so the specification holds under either.

- **Transport.** HTTP/1.1 over a connection the claimant opens to the
  relay's claim endpoint. Today that is TCP, port 8301 by default
  (`relaycfg.DefaultPort`, the Go relay's `-claim-addr`). The relay never
  dials the claimant. The specification keeps "a byte stream the claimant
  opens" apart from "TCP", so that a later transport edits one section.
  The umbrella gives that change to S4. The redrafted driver contract
  has K1 settle who claims, and proposes that what remains of S4 is this
  section and a claim endpoint that is not TCP (its **The boundary with S4
  and K2**; the S4 row is the umbrella's, and its rewording has not been
  put to Jeff).
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
  | `collector_token` | string | Sent as the bearer token on every collector request; empty sends none. It is the session token: it names this build's session at the collector (seam 2) |
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
  ends the relay after the 200 (D19). The build environment may be started
  as soon as the claim returns, by whoever starts it: on `main` whoever
  holds the session (a driver, or the orchestrator that runs
  `cmd/collector`); under the redrafted driver contract the driver, once
  `warden` has told it of the claim (its `claimed` message). The
  specification gives that start to no role. The build environment
  waits for `GET /health` on port 8300 to answer before its first request,
  as `initialize` does.
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

#### Seam 2, `collector-outputs/v1.md` and `collector-inputs/v1.md` (`collector/`, `relay/collector_client.go`)

Seam 2 is one seam, the collector protocol, written as two specifications
(a10, Jeff's answer of 2026-11-15): the outputs a collector stores, and
the inputs and signals it serves. Each has its own directory, its own
requirement prefix (`COLLOUT-<n>`, `COLLIN-<n>`), its own suite and its
own releases (a8), so a collector may implement one and claim it. Nothing
on the wire changes: the claim still names one `collector_url`, and a
relay sends the requests of both to it.

Roles, in both: the *collector* serves; the *relay* is the only client of
this protocol. Who starts a collector, and where it runs, is outside it: on
`main` the session starts one in the claimant's process for each build, and
under the redrafted driver contract `warden` does, one per build with a
fresh token (its a12, answered as that design recommended).

**Stated in both documents: transport, authentication, paths and the
session.**

- *Transport.* HTTP, over the tunnel or to a `collector_url`.
- *Authentication.* Where a session has a token, every `/v1/` endpoint
  except `/v1/status` requires it in the `Authorization` header, and the
  collector answers 401 `unauthorized` to a request without a token it
  knows; `/healthz` and `/v1/status` never require one. The relay sends
  `Authorization: Bearer <token>`; the collector also takes the token with
  no scheme (D8).
- *Paths.* A name carried in a path is a relative slash path of at most
  255 bytes whose segments use `[A-Za-z0-9._-]`. Each document adds the
  rule for its own names.
- *The session.* The protocol's unit is the build *session*: one build's
  requests, identified by the collector token they carry (a collector
  with no token has one session). Every rule of both documents holds per
  session. Completion is terminal for the session: after
  `POST /v1/complete`, which `collector-inputs` specifies, the collector
  owes the relay nothing more and may close the connection at once. How
  many sessions an instance serves is the collector's own, declared in
  its target file: the reference disk collector (`cmd/collector`) serves
  exactly one and exits, with status 0 for `exit_code` 0 and 1 otherwise.
  A collector that serves several sessions, and any client-facing side
  (opening a session, supplying inputs, watching console and status,
  fetching outputs), is out of scope for this item and not specified here
  (the umbrella's a2).

These four are stated once here and written in full in each document,
under that document's own identifiers. They are not put in a third
document that both cite. Each specification is released on its own (a8),
so a shared part would be one more released thing with a version of its
own: a collector that implements only the outputs would have two versions
to state, and a requirement of the shared part would have its cases in
two other specifications' suites. Repeated, each document is whole at
each of its releases. The cost of repeating is drift, and a check in the
specs repository's `ci` removes it: the two sections must be the same
text but for the identifiers' prefix. A change to them therefore changes
both documents in one pull request and is released as a new version of
each.

**`collector-outputs/v1.md`, the outputs a collector stores**

| Endpoint | Stored as | Notes |
|---|---|---|
| `PUT /v1/artifacts/<name>` | `artifacts/<name>` | Streamed. 400 for an unsafe name. Limit 4 GiB by default. A failed upload leaves nothing |
| `PUT /v1/captures/<name>` | `payloads/<hash>`, and `captures/<name>` referring to it | Chunked, with the trailer `Warden-Hash`: 64 hex digits, the BLAKE2b-256 of the body as the relay computed it. Identical content is stored once. The disk collector makes `captures/<name>` a symlink to `../payloads/<hash>`, or a copy where it cannot create one |
| `PUT /v1/ledger` | `ledger` | One request for the whole build, written as it arrives. If the stream breaks, what arrived is kept: every record is signed, so a truncated ledger verifies up to its last record |
| `PUT /v1/environment` | `environment.json` | The environment record's signed payload, at most 1 MiB. `PUT` only |
| `POST /v1/output` | `build-output.log`, appended | Chunks in arrival order; boundaries carry no meaning |

Its own names: an artifact name's segments start and end with an
alphanumeric; a capture name is made safe by replacing every byte outside
`[A-Za-z0-9._-]` with `_`. A successful write is 200 with an empty body; a
body over a limit is 413.

`collector-outputs` is normative for the wire: what a collector accepts,
what it replies and in what order (a16). The "Stored as" column is the
disk collector's layout, the one `warden inspect` reads. The specification
gives it a section of its own, which a collector that is not a directory
need not meet. The disk collector's directory may also hold files that are
not outputs: the kit's session writes the relay's CA there as
`ca.cert.pem`.

The relay's side, as this document requires it: `PUT /v1/ledger` is opened
before any other output; `PUT /v1/environment` precedes the environment
record; and the ledger and every other output are delivered before the
session's completion, the relay ending the ledger request and waiting for
its reply first.

**`collector-inputs/v1.md`, the inputs and signals a collector serves**

| Endpoint | Reply |
|---|---|
| `GET /v1/build-script` | 200, `application/octet-stream`; 404 when the build has none |
| `GET /v1/bootstrap` | 200, `application/json`, the bootstrap config (seam 6); 404 when the build has none |
| `GET /v1/context/<path>` | A file: 200 with `Warden-Context: file`. A directory, the root, or a directory with a trailing slash: 200 with `Warden-Context: list` and `text/plain`, the sorted safe file paths beneath it, relative to the context root, one per line. 403 for an unsafe path; 404 for a missing one, for no context, and for a file named with a trailing slash. A symlink that leaves the context is neither served nor listed |
| `POST /v1/ready` | 200. The relay is up |
| `POST /v1/heartbeat` | 200. The build is alive |
| `POST /v1/complete` | 200. Body `{"exit_code": int, "message": string, "error": string}`; 400 for a body that is not JSON. The first completion is the build's result. A collector may close the connection once it has the completion, before the reply (D16) |
| `GET /v1/status` | The first completion plus `received_at`, or `{"state":"running"}`. A relay never calls it; it can be reached only where a collector listens, which the disk collector does not. It takes no token (D1), so it is specified only for a collector with one session; what it answers on a collector that serves several is left to that collector's client-facing side |
| `GET /healthz` | 200 `ok`. As `/v1/status`: not used by a relay |

Its own names: a context path's segments end with an alphanumeric and may
start with `.`, `-` or `_`.

A session's signals and its lifetime: a collector may give up on a session
whose `ready` does not arrive (the kit's session waits 90 s from the start
of the claim) and, once a first heartbeat has arrived, on one whose
heartbeats stop (the kit: 30 s). Both limits are the collector's own.

The relay's side, as this document requires it: `ready` is sent before the
claim is answered (seam 1, Ordering); a heartbeat is forwarded every 2 s
while the build showed activity (its own `/heartbeat`, or any proxied
request) in the last 5 s; an input that is not 200 becomes a 404 to the
build; signals accept 200 or 204, a failed signal is logged, and none
blocks the relay; and `POST /v1/complete` is sent only after the ledger
request has been answered, the relay not depending on the completion's
reply.

The per-session lifetime is as it was before a10's answer: the session and
its end are in the part both documents state, and the two limits are with
the signals they concern.

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

Roles: the *producer* (whoever supplies a session's inputs to the
collector; on `main` the claimant's session, which starts the collector
with them), the *relay* (carries it), the *executor* (reads it).

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

### The OpenAPI descriptions

Jeff's answer to a9 adds an OpenAPI description to the specifications
that are HTTP protocols, beside the Markdown and not in place of it.

**Which specifications get one,** decided from the code on `main`:

| Specification | Over HTTP | OpenAPI file |
|---|---|---|
| `relay-claim` | Yes: `POST /v1/tunnel` and `POST /v1/claim` on the claim endpoint (`relaycfg.go`'s mux) | `v1.openapi.json` |
| `collector-outputs` | Yes: its five endpoints (`collector.go`'s mux) | `v1.openapi.json` |
| `collector-inputs` | Yes: its eight endpoints, `/healthz` and `/v1/status` among them | `v1.openapi.json` |
| `relay-guest-api` | In part. Three HTTP servers with fixed paths: the reserved host `artifacts`, the reserved host `cwd`, and the control plane on port 8300. DHCP, ARP and DNS are not HTTP, and the proxy for every other host is HTTP with no path of its own | `v1.artifacts.openapi.json`, `v1.cwd.openapi.json`, `v1.control.openapi.json` |
| `executor-initialize` | No: a command line, its steps and its exit. The requests it makes are the client's side of `relay-guest-api` | None |
| `bootstrap-config` | No: a JSON document. The two paths that carry it are `collector-inputs`' and `relay-guest-api`'s | None; its JSON Schema is its machine description |

The guest API has three files because an OpenAPI document keys its
operations by path alone, and the three servers' paths overlap: `/ca.pem`
is on `artifacts` and on the control plane, and on `artifacts` any other
`POST` path is an artifact's name.

**The version and the form.** OpenAPI 3.1, in JSON: the file's `openapi`
field is `3.1.0` and its `jsonSchemaDialect` is JSON Schema 2020-12. 3.1
is the first version whose schema language is JSON Schema 2020-12, so the
file can refer to the schema files the specification already has and
defines none of its own. 3.0 cannot, and 3.2 adds nothing these files
use. JSON, not YAML, because the other machine files of the layout are
JSON and the check that reads the file then needs no second parser.
`info.version` is the document's major version, `v<N>`, as the file name
has it; a release's version is its tag's.

**Which is normative.** The requirements in `v<N>.md` are. They are the
unit a suite case cites and the coverage check counts (Decision 3), and
they say things OpenAPI cannot. The OpenAPI file describes the paths,
methods, parameters, headers, bodies and status codes that the
requirements state, and adds none. Where the two disagree, the
requirement stands and the file is at fault. It describes the protocol
as a conforming client uses it: what the Go relay or the disk collector
does beyond that (D3, D7, D8, D9, D10) is in each document's "Known
divergences", in prose.

**How agreement is checked,** in the specs repository's `ci`, for each
directory that holds an OpenAPI file:

1. *The file is valid.* It validates against the OpenAPI Initiative's
   JSON Schema for 3.1, a copy of which is kept in the repository.
2. *One definition of each body.* Every JSON request or response body
   whose shape the specification defines is a `$ref` to a schema file in
   the same directory, and the OpenAPI file defines no schema of its own
   for one. Every schema file in the directory is referred to by at least
   one operation. So a body has one definition, the one the documents'
   examples and the suite's cases are already checked against. The JSON
   bodies are the claim's request and reply, the completion's body and
   the reply to `GET /v1/status`; the last has a schema for this check,
   `collector-inputs/v1.status.schema.json`.
3. *Every operation and every status is named by a requirement.* Each
   operation, and each response status under it, carries
   `x-requirements`: the identifiers of the requirements that state it.
   The check fails on an operation or a status with none, and on an
   identifier that is not in the directory's `v<N>.md`.

What these do not catch: a requirement that states an endpoint, a status
or a header the file lacks. No check parses prose for that. Review reads
the two side by side, and the reader who finds one files it against the
OpenAPI file. A check that validated the suite's own traffic against the
file was considered and is not built here: the cases send requests the
description does not allow on purpose (a wrong method, an unknown path),
so it would need its own list of exceptions.

An OpenAPI file refers only to schemas in its own directory. A body that
another specification defines, the bootstrap config on
`GET /v1/bootstrap` and on `http://artifacts/bootstrap`, is described as
`application/json` and named in prose, so that no specification's files
depend on another's directory.

**What OpenAPI cannot say, and stays prose only:**

- *Ordering.* The ledger request before any other output, `ready` before
  the claim is answered, the outputs before the completion, one claim and
  one tunnel for a relay's life. The file can give the 409 its status and
  not the state that produces it.
- *The tunnel.* The file states the `101` and its two headers. That the
  connection then carries HTTP/2 with prior knowledge and the roles
  reversed is prose.
- *Timing.* The heartbeat's 2 s and 5 s, the collector's limits on a
  session, the health poll.
- *Trailers and streams.* The `Warden-Hash` request trailer, bodies sent
  as they are produced, and the limits on a stream's size.
- *Names that span path segments.* An artifact's name and a context path
  are slash paths of several segments, and an OpenAPI path parameter may
  not hold an unescaped `/` (3.1.0, Path Templating). Four of the six
  files have such a path: `PUT /v1/artifacts/<name>`,
  `GET /v1/context/<path>`, `POST /<name>` on `artifacts` and
  `GET /<path>` on `cwd`. Each file writes it as one template with one
  parameter, `/v1/artifacts/{name}`, marks the parameter
  `x-multi-segment: true`, and leaves the rule for the name to the
  requirements the operation names. Check 3 still ties the operation and
  its statuses to those requirements. A client generated from the file
  would send the slashes escaped, which is not how a relay sends them;
  the requirement, and not the file, says what a collector must accept.
- *Whatever is not one path.* "Any other method is 405" and "anything
  else is 404", routing by the request's `Host` whatever its port, and
  the proxy for every other host with its byte-exact reply (RELAY-02).
- *The link below HTTP.* The frame path and the kernel path, ARP, DHCP
  and DNS.
- *Differences by mode* (RELAY-04) and the known divergences.

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
| D16 | 2 | The collector records the completion before it replies, and `cmd/collector` closes the tunnel as soon as it has it. The relay can see a failed `POST /v1/complete` for a build that completed; it logs it | `collector-inputs`' 200 reply |
| D17 | 1 | In host mode, the refusal for an absent `upstream_dns` with no usable nameserver in the relay's `/etc/resolv.conf` is a 400 that closes the relay's frame socket. The claim is released, but a second claim builds its netstack on a closed descriptor | "A refused claim leaves the relay claimable" (`build-link-subnet`); vm and container mode keep to it |
| D18 | 3 | After completion, a request the relay would record sends on the ledger's closed channel in a connection goroutine that has no recover, which ends the relay process | Nothing written: completion had no stated after-state |
| D19 | 1, 3 | In vm and container mode the claim's 200 and `ready` are sent once the listeners' goroutines are started, not once the ports are bound. A port that fails to bind ends the relay after the 200, and a failed bind of UDP port 53 is not reported at all | "The relay is up" (`POST /v1/ready`); host mode has its listeners first |

Seam 2's divergences are listed in the document they concern: D2, D3 and
D4 in `collector-outputs`, D1 and D16 in `collector-inputs`, and D8 in
both, with the authentication they state alike. D3's four endpoints are
all outputs.

D17 and D18 are read from the source and have not been run. Implement
reproduces each once, by hand against a relay built from `main`, before
the task that writes its specification: D17 before `relay-claim/v1.md`
(task 2) and D18 before `relay-guest-api/v1.md` (task 4). Neither needs
the harness. If one does not reproduce, the specification states what the
relay was seen to do (Decision 1), and the row here is corrected at
Consolidation; what was seen is a finding for Design (FLOW-06) only if it
is a divergence this table does not record.

The suites do not assert the header's scheme (D8), send nothing recorded
after a completion (D16, D18), give the D17 case its own relay, and wait
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
| D5, D6, D7, D8 (the claim and the token) | With the item that next edits the claim specification, or after the freeze. That item was S4's; the redrafted driver contract proposes that most of S4 is absorbed into K1, which changes neither the claim's fields nor its replies, and that the rest of S4 (the transport section) moves to wave 3 |
| D17, D18, D19 (the Go relay's robustness) | Not fixed in the Go relay, which the program retires. The specification allows what the Go relay does, and another relay may do better. Jeff may class D18 as a security fix |
| D1, D2, D3, D4, D16 (the collector) | After the freeze, or with the first alternative collector |
| D9, D10, D11, D15 (the guest API) | With the Rust relay's M-items, which implement the specification, or after the freeze |

### The harness

Each specification has a *suite*: the cases that cite its requirements,
in its own module, run by its own command (Decisions 2 and 5). The
command, `bw-conformance-<directory>`, runs that suite against one
*target* and reports each case as pass, fail, or not applicable to what
the target declares (its mode, its link kind, the forms it can be started
as). There is no skip for a case the target should pass, and no retry
(TEST-04). What the six commands share is the harness, the module
`conformance/`: they are six thin programs over one body of code.

**A component is tested through several suites.** A relay cannot be
driven on one seam without the others: to check one reply of the guest
API, the harness must claim the relay, be its collector and be its build.
So every suite's cases for a relay start from the same thing, a claimed
relay with the harness's own claimant, collector, build environment and
upstream around it, which the harness's models supply. What differs
between suites is which requirements the cases assert. A case lives in
the module of the specification whose requirements it checks, and cites
only that specification's identifiers. Where one behaviour spans two
specifications, each states its own half as its own requirement, with
its own case: that `ready` precedes the claim's 200 is a requirement of
`relay-claim`'s Ordering and one of `collector-inputs`, the relay's side.
The harness holds no case and cites nothing.

This design calls the suites a relay target runs *the relay suites*, and
those a collector target runs *the collector suites*:

| Target | Suites it runs, and the role each tests |
|---|---|
| A relay | `relay-claim` (relay), `collector-outputs` and `collector-inputs` (the relay as their client), `relay-guest-api` (relay), `bootstrap-config` (the relay as its carrier) |
| A collector | `collector-outputs` and `collector-inputs` (collector), `bootstrap-config` (as it is produced), and for a target that is also a claimant, `relay-claim` (claimant) |

**What the suites check.**

- *Against a relay,* the harness is the claimant and the collector (over
  the tunnel), the build environment, and the upstream world. The case
  groups, by the specification that holds them:
  - `relay-claim`: claim authorization and refusal (a refused claim
    leaves the relay claimable; a spent relay stops listening); field
    validation, one case per rule in seam 1's table; `build_link` and
    `upstream_dns` by mode; ordering at the claim; upstream TLS against
    `upstream_ca_certs`; and that the records of the ledger verify under
    the reply's `ledger_public_key`.
  - `collector-outputs`, the relay's side: the ledger request before any
    other output, the environment record, capture modes and the capture
    trailer, and that every output is delivered before the completion.
  - `collector-inputs`, the relay's side: `ready`, the heartbeat's
    forwarding, an input that is not 200, and the completion signal.
  - `relay-guest-api`: DHCP; DNS (reserved names, list order, the 5 s
    failover, SERVFAIL); on a frame link, ARP and the capture of
    connections to any address; each `artifacts`, `cwd` and
    control-plane endpoint with its error replies; interception over
    HTTP and TLS, including the leaf's chain and CRL distribution point;
    refused upstream dials; and the RELAY-02 round trip, in which the
    harness's ledger verifier checks the hash of each body the build
    received against the bytes it received.
  - `bootstrap-config`: the relay serves the collector's document byte
    for byte and records it.
- *Against a collector,* the harness is the relay. `collector-outputs`
  and `collector-inputs` each drive the endpoints of their own
  specification, over the tunnel or at the base URL of a `listens`
  target, with their error cases. `collector-outputs` also checks what a
  target with a read-back directory stored, against the disk layout.
  `bootstrap-config` checks that the document the collector serves
  validates against the schema. For a target that is also a claimant, as
  `cmd/collector` is, `relay-claim` accepts the tunnel and the claim and
  checks them as the claimant's role (the tunnel before the claim, the
  claim's body against its schema, the token).

**Seams, roles and suites.** Each requirement of each specification is
covered by a case of its own suite or is listed there as not
machine-checked (Decision 3):

| Seam | Specification | Role | Checked by |
|---|---|---|---|
| 1 | `relay-claim` | relay | Its suite, against a relay target |
| 1 | `relay-claim` | claimant | Its suite, against a collector target that claims (the tunnel and claim such a target sends). On `main` `cmd/collector` and every driver claim through the same `driver.Session`, so this checks the claimant of every build. The redraft keeps that code as `warden`'s session in K1, so the same holds there; it lists the session's move into `warden`'s own package among what K2 settles, and what checks `warden`'s claimant after that is K2's to say |
| 2 | `collector-outputs` | collector | Its suite, against a collector target |
| 2 | `collector-outputs` | relay | Its suite, against a relay target |
| 2 | `collector-inputs` | collector | Its suite, against a collector target |
| 2 | `collector-inputs` | relay | Its suite, against a relay target |
| 3 | `relay-guest-api` | relay | Its suite, against a relay target |
| 3 | `relay-guest-api` | build environment | No cases yet (a11) |
| 5 | `executor-initialize` | executor | No cases yet (a11); the Go tests in `cmd/warden-io/` |
| 6 | `bootstrap-config` | relay, which carries it | Its suite, against a relay target |
| 6 | `bootstrap-config` | producer | Its suite, against a collector target: the document served validates against the schema |
| 6 | `bootstrap-config` | executor, which reads it | No cases yet (a11); the Go tests in `bootstrap/` and `cmd/warden-io/` |

The umbrella says "one specification and one black-box suite per seam" and
that seams 1 to 8 each get a suite. When this item lands, seam 5 and the
executor's side of seams 3 and 6 have a specification and no cases. S1's
done-when (the suites pass against the Go relay and the disk collector)
does not ask for them; a11 puts the deferral to the review.
`executor-initialize` still has its module, its releases and its command,
so that it is pinned and versioned like the others. Until S2's item adds
cases the command lists the requirements and runs nothing.

**The coverage check is per specification** (a9, LOOP-03). For each
directory, `ci` fails when an identifier in its `v<N>.md` is neither
cited by a case in its own `cases/` nor listed in that directory, with a
reason, as not machine-checked. It also fails on a case that cites an
identifier its own document does not have. A case in another
specification's module never counts, and the harness has nothing to
count: the rule that a case cites only its own specification is what
makes the check a matter of one directory. Every requirement of
`executor-initialize` is listed, with the reason that its suite is S2's
(a11).

**What a user runs.** One command per specification:

```
bw-conformance-relay-claim run -target relay-host.toml
bw-conformance-relay-claim requirements
bw-conformance-relay-claim version
```

`run` runs the suite's cases for the roles the target has, and says so,
without failing, when it has none for that target. `requirements` lists
the specification's requirements, each with the cases that cite it or
the reason it has none. `version` prints the specification's version,
the harness's, and the table of the specification versions the harness's
models were written from (Decision 5). A target file names no version:
which versions a component is tested against is said by the commands
that are run, in this repository by the pins (Decision 7). A report
names, for each case, the specification, its version, the harness's
version and the requirements the case cites, so a run of a relay names
the five versions it tested.

To run a relay through everything it implements takes the five commands
of the table above; a collector that claims takes four. Each starts its
own instances. In this repository `make conformance` does it: for each
target it runs every pinned specification's command, each from its own
module (Decision 7), and prints one table per target, a row for each
specification with its version, the harness version it was built with
and its counts.

**Each suite is tested against the harness itself.** A module's
`go test ./...` runs its cases against the harness's own model of the
role under test (the relay cases against the harness's relay, the
collector cases against its collector), so a suite and the model it was
written beside cannot disagree unnoticed.

**Instances.** A case that ends an instance gets a fresh one: every
accepted claim, every 500, the host-mode refusal of D17, and every
completion (a relay after `/exit`, D18; a collector after
`POST /v1/complete`, which ends its session and, for the disk collector, the
instance, D16). The completion is the last request such a case
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
   inherited connection is added by the item that writes that transport
   into the claim specification (S4, or what the redrafted driver contract
   leaves of it).
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

1. *How the harness's relay reaches it.* The file names one of two
   attachments. `claims`: the target is also the claimant. It is started
   against a claim address, opens the tunnel and sends the claim, and the
   file gives a command line with placeholders for the claim address, the
   output location, the context directory, the build script and the capture
   mode. For the disk collector:
   `collector --relay {claim} --dir {out} --context {context} --script {script} --capture {capture}`,
   with `WARDEN_CLAIM_TOKEN` in its environment. Each start gives the
   harness one session, so it starts one for each case that sends a
   completion. `listens`: the target is a collector that is already
   serving, and the file gives its base URL and a way to get a session
   token for each case; the harness's relay dials it as a `collector_url`
   that is not the tunnel, and seam 1's claimant cases do not apply. This
   item builds `claims`. `listens` is named so that the format does not
   assume a collector is its own claimant or serves one build; it is built
   by the first item that has such a collector to test. That is the shared
   local or remote collector Jeff described in the `driver-contract`
   review, which this program does not design (its a22, answered: one
   clause for the umbrella's "Later, not scheduled", and nothing queued).
2. *What it can be started as.* The file says whether the collector can run
   with no build script, no context, no bootstrap config and no token, and
   whether it keeps serving after a completion. `cmd/collector` can run
   without a script or a context. It always serves a bootstrap config
   (`{"version":1,"trust":{"mode":"exclusive"}}`, the kit's choice for a
   Linux guest), always sets a collector token, serves exactly one session
   and exits at the first
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
   layout, or a command that produces one, read after the session's
   completion (for the disk collector, after it has exited). Files the
   layout does not name (`ca.cert.pem`) are ignored.
   Without it, `collector-outputs` checks replies only, and its report says
   so.

The reply to `POST /v1/complete` from a collector that exits at completion
is not asserted (D16). A collector's log and exit status are attached to a
failure and never asserted on, as a relay's are.

**Target files for this repository** live in `tools/conformance/targets/`,
beside the six pin modules and in none of them (Decision 7):
`relay-host.toml`, `relay-vm.toml` and `relay-container.toml` (`exec`, the
relay binary with `-mode host`, `vm` or `container`), `relay-oci.toml`
(`oci`, the image built from `Dockerfile.relay`, whose entry point passes
no `-mode` and so runs container mode) and `collector-disk.toml`. Each file
declares the host operating systems it can run on (`os`): `linux` for the
four relay targets, whose sandbox is Linux namespaces, and `linux`,
`darwin` and `windows` for `collector-disk` (a18).

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

### What the specs repository leaves room for

Under the redrafted driver contract, seam 9's specification and
suite start in this repository, as `specs/driver-contract/v1.md` with its
schema and `conformance/driver/`, and move to the specs repository in K2,
where "the suite joins S1's harness as a `driver` role" (its Decision 13,
**Home** and a17, which Jeff answered as that design recommended; the
design was approved on 2026-11-15). This item builds none of that.
It keeps the following open, each at no cost now:

1. *The directory name* `driver-contract/` is reserved (Decision 2), as
   seam 4's names are. With a tag per specification, seam 9's move in K2
   adds one more specification directory: a module of its own, with its
   document, its schema, its cases and its command, released under its own
   tags, `driver-contract/v0.x.y`. It changes no other specification's
   version, and what it needs of the harness is a harness release.
2. *The layout check* accepts a version's one schema as
   `v<N>.schema.json`, RELAY-05's form, as well as
   `v<N>.<part>.schema.json`. The driver contract's schema and seam 4's
   schemas have the first form.
3. *A schema that moves in keeps its `$id`* (Decision 2). The rule that an
   `$id` mirrors the path applies to a schema first released in the specs
   repository.
4. *The roles are packages* (Decision 5). The driver suite plays `warden`
   against a driver binary: it claims the relay the driver started and
   receives the build. In this repository it proposes to import `relaycfg`
   and `collector` for that. In the specs repository it cannot, because the
   no module there requires the kit, so it would use the harness's own
   claimant, collector and ledger verifier. Those are packages of the
   `conformance` module, which holds no case and so depends on no suite.
5. *A target file's role is not a closed pair.* `relay` and `collector`
   are the roles built here, and a file that names any other is an error
   that says which roles this harness knows. Nothing in the format assumes
   the component under test is reached over a network: a driver is a
   process spoken to on its standard streams, which K2 adds to the `exec`
   launcher.
6. *The harness core builds and runs on Linux, macOS and Windows.* Only
   the relay targets' sandbox is Linux-only. The driver suite's static tier
   runs on all three.
7. *"Not applicable" comes only from what the target declares.* The
   driver suite takes its skips from the driver's own `describe`, which is
   the same state.

Not prepared here, and K2's to design: how a release that changes the
driver contract reaches `warden` and its pin, which by then is one more
module under `tools/conformance/`; the driver suite's lifecycle tier,
which needs a host
the driver runs on; and the requirement identifiers of the driver
specification.

One naming point for both reviews. After K1 this repository would have
`conformance/driver/` (the driver suite's source, K1) beside
`tools/conformance/` (the pins and the target files, this item), a
`driver-contract` CI job beside the `conformance` workflow, and
`make driver-conformance` beside `make conformance`. They do not collide.
This design keeps its names and proposes none to K1.

### Releases and how they are consumed

- A release is a tag `<directory>/v0.x.y` on the specs repository's
  `main`, one specification at a time (Decision 4). The `release`
  workflow runs for the tag: it builds that specification's
  `bw-conformance-<directory>` binaries with `go install <command>@<tag>`
  and attaches them with `SHA256SUMS`. A `conformance/v0.x.y` tag releases
  the harness and attaches nothing.
- This repository names a release of each specification in a module of
  its own, `tools/conformance/<directory>/go.mod` (Decision 7). For each
  pinned specification `make conformance` runs
  `go tool bw-conformance-<directory>` in that specification's module,
  against the target files.
- A seam change goes in this order: a pull request to the specs repository
  changes the specification's document and its cases, and its
  `CHANGELOG.md` names the version it will be released as; the
  implementing branch pins that pull request's head commit
  (`make pin-specs REF=<commit>`), and the `conformance` workflow's run
  for that push shows the suites passing on hosted runners; the
  implementing branch is reviewed with the specs branch; Jeff merges the
  specs pull request and the tags are pushed on its merge commit, one for
  each directory it changed, each in its own push, the harness's first;
  the pins move to tags (`make pin-specs SPECS=...`), each specification
  that changed to its new release and the rest back to the releases
  they were at, and the implementing branch opens its PR, where the
  `pin` job requires tags.
  That is LOOP-02's order, the specification first and the code tested
  against it, and LOOP-07's rule for a pin: a branch may name an
  unreleased commit while its work is in progress, and trunk names only a
  release (point 5).
  Until M6 there are two relays, and each moves its own pins when it
  implements a release; an older pin keeps passing. An item that only
  moves a consumer's pin to a newer release, with no seam change of its
  own, is a reconciliation item (LOOP-07): a feature in the one queue,
  filed through intake, whose place FLOW-05 gives. Today FLOW-05 ranks it
  as any feature. Jeff decided at `loop-contract`'s review that it ranks
  with the fixes, and `loop-parallel` is the item that amends FLOW-05 to
  say so (its a5 and a23).
- A change to one specification releases that specification, and
  another only when the harness's model of a protocol changes with it:
  a released binary keeps the harness of its own release, so a suite
  that must drive a component through the changed protocol is released
  again with the new harness (Decision 4). Otherwise the other five keep
  their versions, and an implementation that does not implement the
  change moves nothing. That is what a tag per specification buys.
- A tag is never moved. A failure found after a release is fixed by a new
  release of the directory at fault: a patch release when no passing
  implementation can fail by it, otherwise the next minor. It needs its
  own pull request in the specs repository and Jeff's merge.
- The harness is released when it changes. A specification takes a newer
  harness by a release of its own, whose `go.mod` names it; until then
  its binaries carry the harness they were released with, and so does
  the suite this repository builds from that release. A harness fix
  that every binary needs is therefore a release of each of the six,
  and a move of each pin here.
- The relay repository consumes the same releases with its own target
  files: through pin modules of its own if its CI has Go, or through the
  released binaries of the specifications it implements, each checked
  against its `SHA256SUMS`. It runs released binaries, never a build of
  an unreleased commit, on its trunk. That wiring is M1's, in the relay
  repository. The relay has no
  queue of its own (LOOP-08): its milestones are items in the one queue
  that name the relay repository once P1 lets the queue hold them, and
  until then they are worked outside the queue (the umbrella's Track R and
  **How program items run**, as amended).

**How the seven modules are built and released together.** These are the
mechanics behind Decisions 4, 5 and 7, stated so that Plan can order
them. The first three follow from a19's answer: the harness is a module
of its own, and a specification may name a harness version before that
version has its tag. The last two would hold wherever the harness
lived.

They were tried on 2026-11-15 with Go 1.26.3, in a model of the layout:
local git repositories, nested modules tagged `<directory>/vX.Y.Z`, and a
harness that imports a module outside the standard library, as the real
one will (a DNS library, a network stack, a TOML parser). The first trial
used modules that import nothing, and with those the workspace needs
none of this, which is how the first three were missed until the review
of the fold-in.

- *The workspace names the harness version in progress.* A
  specification's `go.mod` names the harness version it will be released
  with. In a pull request that changes the harness, that version has no
  tag yet, and every build, vet and test in the workspace then fails
  with "unknown revision", because Go reads the `go.mod` of the version
  that is named. `go.work` therefore carries one line,
  `replace <the harness module> v0.x.y => ./conformance`, for the
  version at the head of `conformance/CHANGELOG.md`. With it they pass.
  Go refuses the line without a version, so a pull request that raises
  the harness's version edits it, and `ci` checks it against the
  `CHANGELOG.md`. A specification that still names an older harness
  version, which has its tag, builds in the workspace too, against the
  tree's harness.
- *Each specification's `go.mod` is tidied through a temporary
  `replace`.* `go mod tidy` cannot resolve a harness version that has no
  tag. `make tidy` in the specs repository adds a `replace` of the
  harness by `../conformance` to the specification's `go.mod`, runs
  `go mod tidy` outside the workspace, and drops the line. The `go.mod`
  then lists the harness's own dependencies at the versions the harness
  names, and `go.sum` their hashes; `go.sum` has no line for the harness
  itself. `ci` runs the same command and fails on a difference, and
  fails a `go.mod` that carries a `replace`. This is what makes a
  release repeatable: built from a `go.mod` that had not been tidied,
  the same tag was built with a newer version of a third-party module
  as soon as one was published.
- *The release build is `go install <command>@<tag>`,* for each
  operating system and architecture, with `CGO_ENABLED=0`. It builds
  from what the tag serves, with exactly the versions the module's
  `go.mod` lists, which is what a `go.mod` that names the tag gets. A
  `go build` in a checkout at the tag, outside the workspace, fails: the
  specification's `go.sum` has no line for the harness. The harness's
  tag is pushed before the specifications' tags that name it, because
  `go install` resolves it.
- *An implementation's branch in progress* pins a commit, where a
  specification may name a harness version that does not exist yet.
  `make pin-specs REF=<commit>` therefore adds a `replace` line for the
  harness to each pin module whose specification names such a version
  (Decision 7). The `pin` job fails a `replace` line as it fails an
  unreleased commit.
- *Not tried:* the module proxy and the checksum database, which the
  model went around: the first resolution of real tags, of a
  specification that names a harness tag pushed moments before. It first
  happens in `f6-release-specs`. If it fails, no tag is moved: the fault
  is fixed in a pull request and released as the next patch versions.
  Nor was the real harness's set of dependencies tried; task 1 tries
  the mechanics with one of them.

**How this repository's six pin modules are built.** These are the
mechanics behind Decision 7 as a20's answer has it. They were tried on
2026-11-15, after that answer, with Go 1.26.3, in the same model: two
specifications, each released twice, the later releases naming a harness
version that renames a function the earlier cases call, and a
third-party module that has a newer version than the harness names.

- *One module for each specification, written from nothing.* A `go.mod`
  with the `module` and `go` lines, the specification's command in a
  `tool` directive and one requirement, then `go mod tidy`. The result
  is Decision 7's sample: the specification, the harness at the version
  that release names and the harness's third-party module at the
  version it names, not the newer one, each marked `// indirect`, and a
  `go.sum` that lists all three. `go tool bw-conformance-<directory>`,
  run in the module, builds and runs the command.
- *The same build as the release.* `go version -m` printed the same
  module lines, with the same hashes, for the binary `go tool` built in
  the pin module and for the one `go install <command>@<tag>` built.
- *The `go` line,* tried on the second review of the fold-in. In the
  model every module names `go 1.26`, so the line had made no
  difference. With a specification that names `go 1.24`, built with
  Go 1.26.3: `go install <command>@<tag>` and `go build` of the command
  in a pin module whose line is `go 1.24` wrote the same file, with the
  same SHA-256, and the binary `go tool` built there printed the same
  `go version -m` lines as both (`go tool` keeps its own build in the
  cache, which is not that file). With `go 1.26.3` on the pin module's
  line, `go build` wrote a different file and both builds' `go version
  -m` lacked the `DefaultGODEBUG` line. A pin module written
  with `go 1.25` for a release that names `go 1.26` was raised to
  `go 1.26` by `go mod tidy`, with no `toolchain` line. A release that
  names `go 1.27` stopped `go mod tidy` under `GOTOOLCHAIN=local`, with
  no `go.sum` written, and under `GOTOOLCHAIN=auto` Go tried to fetch
  that toolchain. Its `go` line could still be read, from the file
  `go list -m -f '{{.GoMod}}' <module>@<version>` names, run outside
  the pin module; for a release or a commit that the running Go can
  build, `go list -m -json` also gives it as `GoVersion`. Go 1.26.3 and
  Go 1.26.11 wrote the same two files for one pin; Go 1.26.0, older
  than a pin module's `go 1.26.3`, refused it under `GOTOOLCHAIN=local`.
- *A mixed set builds and runs.* One specification at its newer
  release beside the other at its older one: each command ran and
  reported its own harness version.
- *A workspace undoes it.* With a `go.work` that lists the two modules,
  the older specification was built with the newer harness and did not
  compile. With `GOWORK=off` beside that file both ran again. So there
  is no `go.work`, and the recipes set `GOWORK=off`.
- *`go get` in place does too.* Taking the newer pin back to the older
  release with `go get` left the harness line at the newer version,
  `go mod tidy` kept it and `go mod tidy -diff` reported nothing to
  change, and the command did not compile. Written again from nothing,
  the module named the older harness and ran.
- *A branch in progress.* A commit on a branch of the model's specs
  repository raised the harness's version without a tag and named it in
  one specification. That specification's pin module did not resolve
  without a `replace` line and ran with one that points the harness at
  the same commit. The other specification's module, pinned at the same
  commit, named a tagged harness, needed no line, and ran with that
  harness. Whether a named version has its tag is read with
  `go list -m <harness>@<version>`, which answered rightly for both.
- *The `pin` job's criteria,* as a script over `go mod edit -json`: it
  passed the mixed set of releases, and failed a commit, a `replace`
  line, a harness line raised by hand, and a second specification
  required in one module.
- *The root module is not affected.* `go build ./...`, `go vet ./...`
  and `go list ./...` in a root module above the six did not enter
  them, and its `go mod tidy` changed nothing.
- *Not tried:* the real module names, proxy and checksum database, as
  above; `go tool` for a command that needs the real harness's
  dependencies; the builds on `macos-latest` and `windows-latest`,
  where the `collector` job builds and runs the commands too; and a
  pin module written by a later minor version of Go than 1.26, which
  the `pin` job's comparison would meet if a runner's Go ran ahead of a
  developer's.

### CI

**Specs repository** (`.github/workflows/` there):

- `ci`, on pull requests and `main`, over every directory, built from the
  working tree (the workspace):
  - the layout check (every `v<N>.schema.json` or
    `v<N>.<part>.schema.json` beside its `v<N>.md`; each schema's `$id` as
    Decision 2 has it; schemas valid 2020-12; the documents' JSON examples
    validate against them);
  - for each specification, requirement identifiers unique, and each
    covered by a case of its own suite or listed as not machine-checked
    (**The harness**);
  - the three OpenAPI checks (**The OpenAPI descriptions**: each file
    valid 3.1, every JSON body a reference to a schema file beside it,
    every operation and status named by a requirement);
  - the check that the two collector documents state their common part
    alike (**Seam 2**);
  - `go vet` and `go test ./...` in each of the seven modules, which
    includes each suite's test against the harness's own counterpart role
    (**The harness**). Because every module is built against the tree,
    this is also what holds a harness change to every specification's
    cases: the set on `main` is always tested together;
  - a check that no module's `go.mod` requires the kit;
  - the module checks (**Releases and how they are consumed**): each
    specification's `go.mod` and `go.sum` are what `make tidy` writes, no
    `go.mod` carries a `replace`, and the `replace` line of `go.work`
    names the harness version at the head of `conformance/CHANGELOG.md`;
  - the release checks: a pull request that changes a directory adds a
    version to that directory's `CHANGELOG.md`, higher than its newest
    tag, and a specification whose directory changed names in its
    `go.mod` the harness version at the head of `conformance/CHANGELOG.md`.
- `implementations`, weekly and on demand, not a merge gate: the newest
  release of each specification's suite against each known implementation
  at its released version (the Go relay image first), published as a
  table in the run summary. It is not a gate because a specification
  change is merged before the code that implements it.
- `release`, on the push of a tag `<directory>/v*`: it checks that the
  tag is on `main` and is the version at the head of that directory's
  `CHANGELOG.md`, then builds and attaches that specification's binaries
  (**Releases and how they are consumed**). For `conformance/v*` it runs
  the checks and attaches nothing. It can also be started by hand for a
  tag that already exists (`workflow_dispatch`, with the tag as its
  input), so that a run that was missed or that failed is made again
  without touching the tag. Tags are pushed one to a push, because a
  push of more than three tags starts no run.
- `flow18`, the guard this repository has, so the public repository never
  carries `entire/` refs (FLOW-18).

**This repository:** a new `conformance` workflow, shaped like
`integration`. Its paths are every package the relay and the disk collector
are built from (`go list -deps ./cmd/relay ./cmd/collector`), so that a
change to a shared package runs the suites (TEST-02's caveat): `relay/**`,
`relaycfg/**`, `collector/**`, `bootstrap/**`, `ledger/**`, `cmd/relay/**`,
`cmd/collector/**`, `driver/*.go`, `driver/buildlink/**`,
`driver/script/**`, `go.mod`, `go.sum`, `Dockerfile.relay`, the `Makefile`,
`tools/conformance/**` (the six pin modules and the target files, so a
move of any pin starts a run) and the workflow itself.
`cmd/warden-io/**` is not among them, because no suite runs the executor
yet (a11). It runs:

- on pull requests that reach those paths;
- on a push to any branch other than `main` that changes one of those
  paths. This is how a branch gets hosted runs before it has a pull
  request: GitHub offers a manual run only for a workflow that is already
  on the default branch. The list is the same as for pull requests, so
  every commit that could change a verdict starts a run, and the latest run
  on a branch covers its head: a commit after it that changed a listed path
  would have started its own. At `f1-design` the push trigger was
  `tools/conformance/**` and the workflow alone; the rule changes, the book
  and a merge of `main` would then have left the head with no run. A branch
  with an open pull request gets two runs for such a push, one on its head
  and one on the merge with `main`;
- weekly on `main`, and on demand.

Jobs: `pin` (each of the six modules under `tools/conformance/` is
there, names its own specification at a release tag and no other module
of the specs repository but the harness, whose line is a release tag
too, and carries no `replace` line; and `make pin-specs`, given the
versions the six files name, changes no file, which holds each `go.mod`
and `go.sum` to what that release names (Decision 7); not run on a
branch push, where an unreleased commit is allowed); `relay` on
`ubuntu-latest`, which runs
`make conformance TARGETS="relay-host relay-vm relay-container relay-oci"`;
`collector` on `ubuntu-latest`, `macos-latest` and `windows-latest`, which
runs `make conformance TARGETS=collector-disk` (the disk collector, whose
capture links differ on Windows). For each target `make conformance` runs
every pinned specification's suite (**The harness**), so the jobs name
targets and not suites. Each job names its targets, so a target
the default leaves out on some host can never make CI green (a18). It is
tier 1 in TEST-02's terms. In the profile's Checks table it is a check of
the Specification class, deterministic and blocking (LOOP-03; **Rule
changes**, PROFILE-01).

### Tier 1's seam list

`tools/tier1/rules.yaml`, replacing `seams: []` and the comment above it:

```yaml
# GLOBAL-08, EXEC-04: versioned seams, each with the code that defines it
# and the contract path a change to that code must also touch. For a seam
# specified in the specs repository that path is the file that pins the
# release of its specification here (TEST-05). The collector seam has two
# specifications and so two entries, one for each pin.
seams:
  - name: relay-claim
    code: [relaycfg/relaycfg.go, relaycfg/tunnel.go, relaycfg/buildlink.go]
    contract: tools/conformance/relay-claim/go.mod
  - name: collector
    code: [collector/collector.go, collector/paths.go, collector/store.go,
           relay/collector_client.go]
    contract: tools/conformance/collector-outputs/go.mod
  - name: collector
    code: [collector/collector.go, collector/paths.go, collector/signals.go,
           collector/store.go, relay/collector_client.go]
    contract: tools/conformance/collector-inputs/go.mod
  - name: relay-guest-api
    code: [relay/controlplane.go, relay/heartbeat.go, relay/dns.go, cmd/relay/dhcp.go]
    contract: tools/conformance/relay-guest-api/go.mod
  - name: executor-initialize
    code: [cmd/warden-io/main.go, cmd/warden-io/initialize.go,
           cmd/warden-io/baseline.go, cmd/warden-io/jks.go,
           cmd/warden-io/platform_darwin.go, cmd/warden-io/platform_linux.go,
           cmd/warden-io/platform_windows.go, cmd/warden-io/trust_linux.go]
    contract: tools/conformance/executor-initialize/go.mod
  - name: bootstrap-config
    code: [bootstrap/bootstrap.go]
    contract: tools/conformance/bootstrap-config/go.mod
```

Files are named one by one because the check's globs cannot leave tests out.

Each seam's contract path is the pin of its own specification (a20,
Decision 7). Until a20 was answered all five named one file,
`tools/conformance/go.mod`.

**Within what the check's code accepts.** `seam-contract` reads
`contract` as one path: the field is a string
(`tools/tier1/rules/rules.go`, `Seam.Contract`), and the parser on
`main` refuses a list in it. The check's code is not edited. FLOW-14
has Review and CI run tier 1 built from the merge base's code with the
branch's `rules.yaml`, so the list has to be one that code reads. It
is: one path for each entry, and an entry for each pin. This was tried
on 2026-11-15 in a clone of this repository at `ad4ea50`, with the list
above and `main`'s check code. `tools/workflow/enforcement-check.sh`
passed, and tier 1 reported as the next paragraphs say. The same code
given `contract: [<path>, <path>]` stopped with "cannot unmarshal !!seq
into string".

**One test of the check changes with the list.** `TestSeverities`
(`tools/tier1/coupling/checks_test.go`) runs every coupling check with
the real `rules.yaml` over a fixture that changes `cmd/warden-io/jks.go`,
and fails on a finding whose severity is not in its table. The list
puts that file on the `executor-initialize` seam, so `seam-contract`
now reports the fixture, rightly, and the table gains
`"GLOBAL-08": report.Must`. In the same clone, `go test ./...` in
`tools/tier1` failed without that line ("GLOBAL-08: severity "MUST",
want """) and passed with it, in every package there and in
`tools/workflow`; `enforcement-check.sh` passed with the test edited and
with TEST-05's new owner string in place. The file is under
`tools/tier1/`, a rule path, and is among GLOBAL-08's `paths` (**Rule
changes**). Until the second review of a20's fold-in the design said
that only `rules.yaml` changed there: the fixture's file has been on the
list since the revision of 2026-11-13 (`5ebe806`), and the test was not
run against the list.

**Seam 2 has two entries, both named `collector`.** It is one seam with
two specifications (a10) and so two pins, and an entry holds one path.
The code is not divided as the documents are. `collector/signals.go`
holds only the handlers `collector-inputs` specifies, and is listed in
that entry alone. The other four hold code of both: `collector.go` the
routes and the handlers of both, `paths.go` the names of both,
`store.go` the outputs' files and the listing of the context, and
`relay/collector_client.go` the relay's side of both. They are listed
in both entries. Nothing in tier 1 needs an entry's name to be unique;
the name appears only in the finding, which also names the pin that is
missing.

**What the check holds.** `seam-contract` reports a change to a listed
file when the diff does not also change the contract path of that
file's entry (`tools/tier1/coupling/checks.go`, `SeamContract`). So a
change to a file of one seam asks for that seam's pin, and a pin of
another seam does not satisfy it: in the trial a change to
`relaycfg/relaycfg.go` with only the `collector-inputs` pin moved was
reported. For seam 2, a change to `signals.go` asks for the
`collector-inputs` pin. A change to one of the four files listed twice
asks for both pins, and is reported once for each pin the diff does not
change. That is more than the rule asks when the change concerns one of
the two specifications. Such a change moves that specification's pin
and carries a trailer for the other,
`Tier1-Allow: GLOBAL-08 <path>: <reason>` (LINT-05), whose reason says
that the other specification is not changed. A trailer is matched by
rule and path, so it clears both entries' findings on that file: with
it the check holds nothing for the file, as with any trailer, and tier 1
prints the trailer and its reason for Review to judge. The two entries
never ask less than one pin for a change on the seam.

What the check still cannot tell is whether the release a pin moved to
states the change. That is Review's to judge (the agent half of
GLOBAL-08 and EXEC-04), from that specification's own `CHANGELOG.md`.
The proposed GLOBAL-08 paragraph states the obligation, "changes its
specification first", and names the pin files only as what the
automated part checks.

*Rejected:* one entry for seam 2 that names one of its two pins. A
change on the other specification's side would be asked for a pin that
has nothing to do with it, and not for its own. *Rejected:* a list in
`contract`, satisfied by any one of its paths. It would fit seam 2
exactly. It needs a change to the check's code and a `rules.yaml` that
`main`'s code cannot read, so the merge base's tier 1 could not run on
this branch and the change would rest on Review alone (FLOW-14). The
two entries need neither, and where they differ from it they ask for
more.

**What is left off** (a17). Three sets of relay code are on a seam and not
on the list. `relay/relay.go` holds the reserved hosts' handlers
(`handleArtifacts`, `handleContextGet`) with the CA and the ledger
recording. `relay/proxy.go` holds interception and the proxy, which is
most of what the relay does. `cmd/relay/`, all but `dhcp.go`, holds what
each mode does with a claim, with the rest of the binary. Listing them
would ask most relay changes for a pin move or a `Tier1-Allow` trailer.
What covers them is weaker than the check: the `conformance` workflow runs
on any change under `relay/` and `cmd/relay/`, and the pinned suite fails
a change to behaviour that a case tests. A change that adds behaviour no
case tests (a new `artifacts` path, a new step in one mode's claim) passes
the pinned suite and needs no pin move. Only Review's judgment of GLOBAL-08
stands against that.

**The executor's entry.** The executor has no suite (a11), so nothing
covers its files that way, and the entry lists every non-test file of
`cmd/warden-io/`: the invocation and the steps (`main.go`,
`initialize.go`), the baseline (`baseline.go`, and `jks.go` for the
truststore's alias and password), the per-OS values and each OS's
`installCA` (`platform_<os>.go`), and Linux's exclusive trust store
(`trust_linux.go`). Its contract path is the `executor-initialize` pin:
that specification states every step the executor takes, its use of the
relay guest API among them (**Seam 5**). Until S2's suite exists, a
change to them is held by
this check, by the Go tests in `cmd/warden-io/` under `make test`, and by
EXEC-04's agent-judged part. The specification says what each trust mode
means for what the build trusts, and names the per-OS procedure without
fixing its steps (EXEC-03), so a change to the steps that keeps the outcome
carries a trailer. At `f1-design` `trust_linux.go` was left off for that
reason while the three platform files, each of which holds `installCA`,
were listed: a trust change needed a pin move or a trailer on macOS, on
Windows and in Linux's append mode, and nothing in Linux's exclusive mode.
Listing the file treats the four alike.

The list is shared with `driver-contract`, whose redraft proposes one
entry, `driver-contract`, with a document in this repository as its
contract (`specs/driver-contract/v1.md`). The two designs' entries name no
file in common. **With `driver-contract`** says what the second to merge
does with the list and the comment above it.

A change that touches a listed file and changes no behaviour (a refactor)
carries a `Tier1-Allow: GLOBAL-08 <path>: <reason>` trailer (LINT-05), as
a prose-only fix to a specification does under RELAY-05. A change to a Go
file's comments or layout alone is not counted by the check and needs
none.

### The book and the specifications link both ways

- **Book to specifications.** A new page,
  `docs/book/src/concepts/seams.md` ("Component seams", listed in
  `SUMMARY.md` at the head of the Specifications part), has one row per
  seam: what it is between, its specification (two for seam 2, a10), the
  package that implements it here, and the suite that checks it. It is the
  only file in this
  repository that links into the specs repository (a row links the
  seam's OpenAPI description too, where it has one). Each row's links name
  the ref its specification is pinned at: the tag `<directory>/v0.x.y`,
  or, while the pin is an unreleased commit, that commit. The page also
  shows each pinned version. `make pin-specs` rewrites the refs together
  with the pins. A tier-0 test in `specs/specs_test.go` fails when a row's
  ref and that specification's version in
  `tools/conformance/<directory>/go.mod` disagree, so the book
  never points at a release CI does not test. `specs/README.md` gains one
  sentence pointing to the page for the seams that are not the ledger.
- **`concepts/architecture.md` keeps what explains and gives up what
  enumerates.** Its sections on the collector and the claim keep their
  narrative (why the config is pushed, the tunnel, what the build link is
  for, the ordering rule and its reason), and each opens by saying that the
  specification is the contract, with a link to the seams page. The
  collector's endpoint table, `build_link`'s validation rules and the
  by-mode table are replaced by a sentence and that link. So is the
  endpoint table under "Communication paths", which lists the relay guest
  API as the build sees it (seam 3): the paragraphs after it, on the
  control plane's port, the two reserved names and the collector being a
  plane the build never sees, stay. The headings stay, so links into the
  page keep working. *Rejected:* keeping the tables and marking them
  non-normative. They would be a third description of the same protocol
  (code, specification, book), kept in step by hand.
- **Specifications to book.** The specs repository's README has an
  "Implementations" table (the Go relay, the disk collector and the executor
  in `buildwarden/buildwarden`; the Rust relay when it is public), and each
  document's header links the book's page for the concept
  (`https://buildwarden.github.io/buildwarden/`) and names its reference
  implementation. Until M6 moves seam 4 (a1), the README says that the
  ledger format is specified in `buildwarden/buildwarden` under `specs/`,
  links to it there, and says that it moves at M6.
- `AGENTS.md` gains a row for the seam specifications and their pins,
  `CONTRIBUTING.md` a short section on `make conformance` and
  `make pin-specs`, which says that a pin is moved by that command and
  never by `go get`, and `contributing/structure.md` an entry for
  `tools/conformance/`, its six pin modules and its target files, and
  the new duty of `specs/specs_test.go`.
- `concepts/overview.md` is unchanged: no component's flow changes
  (FLOW-15), so the phase's overview sweep (FLOW-20) has nothing from this
  item.

### How the loop treats the second repository

There is one queue, the one on this repository's `workflow` branch, for
work in every repository, and an item names the repositories it works in
(LOOP-04). This item names two:

- `buildwarden/buildwarden`, its home. The design, the queue item, both
  reviews, the branch and the pull request `wf` records are here.
- `buildwarden/buildwarden-specs`. The queue file has no field for it; the
  design is where it is named until the queue records it (LOOP-04), which
  `loop-parallel` is to make it do.

Working in a second repository from this queue is the normal pattern. It
is not a stopgap that ends with P1.

**The rules that bind there.** The specs repository has not adopted the
Loop Contract: it has no profile and no invariants of its own, so it is
not a component, and it adopts when Jeff files the item that does that.
Until then the worker is bound there by four loop rules, which LOOP-08
names, and this design lists the invariants that apply, by rule ID, as
LOOP-08 asks; a4 asks Jeff to confirm the list. The first four rows
are the loop rules; the last two are the list:

| Rule | In the specs repository | Held by |
|---|---|---|
| FLOW-08 | Only Jeff merges to its `main`. No agent approves a pull request, merges one or enables auto-merge. His merge is the sign-off; before it, the session that does `f6-release-specs` lists in the specs pull request's body the tier-2 MUST findings that are in the specs repository, so that the merge acknowledges them there. The one sentence that does not hold there is "no sign-off step in the queue" (a15) | `main`'s protection (**What Jeff creates**); the workers' settings; the step's note |
| FLOW-16 | The branch is deleted when its pull request merges; no agent deletes a remote branch | The repository's setting. `wf`'s Close checks only this repository, so the session that does `f6-release-specs` confirms the branch is gone and names it in the step's evidence |
| FLOW-17 | Agents push `feat/seam-specs` at each green checkpoint (`go vet` and `go test ./...` in each module there), and never `main`, a force-push or a rewrite of pushed history. Release tags are pushed only in `f6-release-specs` | The rule; `main`'s protection |
| FLOW-18 | Sessions that commit there are recorded to the private sessions repository, and the public repository carries no `entire/` refs | `.entire/settings.json` and the `flow18` workflow there (task 1) |
| GLOBAL-01 to GLOBAL-04, GLOBAL-07, GLOBAL-09 to GLOBAL-11, TEST-04, TEST-07 | The harness's Go code: the `conformance` module and each specification's cases and command | Review's tier-2 report, which reads the specs branch; tier 1 does not run there |
| RELAY-05's schema-identity rule | Each schema's `$id` (Decision 2) | The specs repository's `ci` layout check |

No other rule of this repository binds there, and none of them is
enforced there by `wf` or by tier 1.

**What `wf` does not do there.** Until `loop-repository`, `wf` runs no
gate in the specs repository and records no branch or pull request of it.
The specs branch, its pull request and the commits this branch pinned are
named in the steps' evidence and in the design's Consolidation. A run
that changes the specs branch records its head in the run's digest
section, and names the specifications it changed apart from the code
(LOOP-06, points 1 and 2). What ties
the two repositories together mechanically is the pins, below.

**What changes later, and what does not need to.** `loop-parallel` gives
the queue a field for an item's repositories. `loop-repository` gives `wf`
its gates, branches, pull requests and checks in a second repository; an
item like this one would then record its specs pull request, and the
human step below would not be needed (a15). When the specs repository
adopts the contract it becomes a component, coupled to the others only
through releases (LOOP-08, LOOP-07). That is already how this repository
consumes it: by its pins.

| Phase | In this repository | In the specs repository |
|---|---|---|
| Plan | Every task names its repository | |
| Implement | The pins (every specification at the specs branch's head commit), target files, `make conformance` and `make pin-specs`, the workflow, book pages, tier 1's seam list, the rule changes. `make lint` and `make test` green (FLOW-03). Task 8 is done when the `conformance` workflow's run for the pushed head is green on all three runners, and task 12, the last, when the run for the final pins is; the step's evidence names both runs | Documents, schemas, harness, suites and CI on `feat/seam-specs`, pushed at each green checkpoint (FLOW-17); its pull request opened in task 11, before the final pins, so Jeff can read it early |
| Review | Tier 1 and the tier-2 report on this branch. Review confirms that the latest `conformance` run on the branch is green and that every pin is the specs pull request's head | The same tier-2 report also reads the specs branch at the pinned commit: the documents against the code lists in this design, and the harness against the rules in the table above. Its findings say which repository they are in |
| Publish | `f6-merge-main`, `f6-consolidate` and `f6-open-pr`, as for any feature. `f6-open-pr` finds the pins on the tags; if it does not, the PR's `pin` job fails. The PR's body lists the specification changes, from **Specification changes**, and the rule changes (LOOP-06, FLOW-08) | `f6-release-specs` (human, a15): Jeff reads the tier-2 report's MUST findings, which the session has listed in the specs pull request's body, and merges the pull request. The seven tags are then pushed on the merge commit, one push for each tag, `conformance/v0.1.0` first and then each specification's `v0.1.0` (Decision 4), and the pins moved on this branch (`make pin-specs SPECS=latest`, `make test`, `make lint`, commit, push), by him or by the attended session on his word |

The mechanical link between the two is the pins. Before the release, the
suites have passed on hosted runners against the exact commit the branch
pins (the push run). After it, this repository's PR cannot have green CI
(FLOW-03, phase 6) unless every pin names a release tag and those
releases' suites pass here. The pins' move comes after Review and changes
thirteen files: the `go.mod` and `go.sum` of each of the six modules
under `tools/conformance/`, and `docs/book/src/concepts/seams.md`. The
PR's CI runs everything on the result.

**The release step.** `f6-release-specs` is a `human` step that neither
FLOW-04's list nor FLOW-08's "no sign-off step in the queue" provides
for; a15 asks Jeff for it as an exception to both, for this item. It does
not sign anything off. The merge does that (FLOW-08), and the step records
that the merge happened where `wf` cannot see it. `wf` offers a `human`
step whatever its phase, once the steps it
needs are done. The step needs `f2-design-review`, so it is not offered,
and `QUEUE.md` does not list it, before the review has confirmed a15. From
then until Review is done it is offered and must not be done: the tier-2
report and the green hosted run are what the merge rests on (FLOW-08).
`wf` cannot hold it back further, because a step can only need one that
exists and Review's step is seeded with its phase. Three things stand in
for that: the step's note says so; no specs pull request exists to merge
before task 11; and a step marked done early, or without the pins' move,
is caught by the `pin` job, which stops the PR. If the item returns to
Design (FLOW-06), the need no longer holds the step back: the review step
it names is done, or `wf back` dropped it and the need with it. The step
is then offered again until the next review. Design keeps the step and its
note, and cannot add a need on the next review's step, which does not
exist until Plan is seeded.

If the specs pull request changes between Review and its merge, the tags'
tree differs from the commit Review read. The session that moves the pins
compares the two in the specs repository and, if they differ, records the
difference in the step's evidence and in the message of the commit
that moves them. The
`conformance` workflow judges the result; a failure is repaired under
FLOW-11 or goes back under FLOW-06.

If the item goes back after the tags (FLOW-06, FLOW-10) and the specs
content has to change, the change is a new branch and pull request in the
specs repository and a new release of each directory it changes
(**Releases and how they are consumed**). On a return to Design, the
Design step adds a human step
`f6-release-specs-r<n>` for it. On a return to Implement alone, Implement
stops with a blocking question that names the release it needs, as it does
for a missing repository (a2).

### Beside `driver-contract` in the queue

This item and `driver-contract` both declare `alters_relationships: true`
and both are past Design. `alters-slot` (merged as `897a4a8`, #118) made
`wf next` and `wf validate` count the features that hold a slot against
`config.yaml`'s limit of 3, a slot being held from the claim of the Plan
step. Under that limit this item and `driver-contract` do not hold each
other's Plan, and the two may be planned and implemented side by side.
They still take the PR slot one at a time (FLOW-05). Before that merge
`wf next` gave the cloud loop one slot whatever the limit said, and each
of the two would have held the other's Plan; that is no longer so.

Nothing in this design depends on which of the two merges first for that
reason. The order matters in one place only, the rule text and the tier-1
configuration both edit, and **With `driver-contract`** says what the
second to merge does.

### What Jeff creates

**The repository, during the design review** (a2, Jeff's answer of
2026-11-15: now, before the review's approval). Jeff creates it himself.
The cloud session that holds the review cannot create a repository, so
the session's part is to attach it once it exists. What is created was
confirmed with him in that session, right before creation (umbrella a11):

1. The repository `buildwarden/buildwarden-specs`, public (a3), **with a
   first commit on `main`**, so that no agent ever pushes `main`. The
   commit carries a README stub and the Apache-2.0 `LICENSE`, the licence
   this repository has, and nothing else. It carries no `NOTICE` file.
   This repository's `NOTICE` reads "Copyright Amazon.com, Inc. or its
   affiliates"; it is not copied, and no agent adds one.
2. On it: `main` protected (pull request required, no force-push, no
   deletion), "Automatically delete head branches" on (FLOW-16), Actions
   enabled. The repository is public, so its host offers protection.
   LOOP-08 asks protection of a component's trunk where its host offers
   it (step 6); the specs repository is not yet a component, and is
   protected from its creation so that FLOW-08 is held there by more than
   the workers' settings.
3. The Claude GitHub App installed on it. That is what lets the loop's
   sessions push branches and open pull requests there; the session then
   attaches the repository.

The repository did not exist when this was written, during the first
sitting of the review on 2026-11-15. Creating it before the approval
fixes its name (umbrella a10), its visibility (a3) and its licence file,
each confirmed with him, and nothing else: nothing is pushed to it
before Implement, and a design that is sent back leaves it with its
stub.

**What Implement checks before it starts** (task 0). The repository
exists and is public. `main` has one commit, with a README and the
Apache-2.0 `LICENSE` and no `NOTICE`. The session can read it, push the
branch `feat/seam-specs` and open a pull request. The protection of
`main`, the head-branch setting and Actions are read where the session's
credentials can read them; what it could not read is named in the step's
evidence, and nothing is tested by trying a push to `main`. If the
repository is missing, differs from this list, or refuses the branch,
Implement stops with a blocking question (FLOW-07) and changes nothing.

After Review: `f6-release-specs` (a15). The merge of its pull request,
which is Jeff's alone (FLOW-08), preferably as a merge commit so that the
commits this branch pinned while in progress stay in its history; then the
seven tags on the merge commit, each pushed on its own,
`conformance/v0.1.0` first and then one `v0.1.0` for each specification
(a8, Decision 4), and the pins' move here, which the attended session
may do on his word.

Not needed: packages, Pages, secrets, or any change to `buildwarden-relay`.

## Defaults and overrides

This item adds no default, override or fallback to the relay, the collector,
the executor, `warden` or any driver, and no relay setting: the claim gains
and loses no field (RELAY-08).

It adds these to the tooling:

| Default | Why it is the obvious one | Override, used exclusively |
|---|---|---|
| `make conformance` runs every target in `tools/conformance/targets/` whose file declares the host's OS, and prints one line for each target it leaves out, with the reason (a18) | A developer who names nothing wants every check that can run on their machine. On Linux that is the whole check CI runs; on macOS and Windows it is the disk collector's | `TARGETS="relay-host collector-disk"` runs exactly those. A named target that cannot run on the host is an error |
| Each specification's suite comes from the release its own module pins, `tools/conformance/<directory>/go.mod`, built with the harness that release names | They are the releases this repository claims to implement, each as it was released | `SPECS_DIR=<checkout>` runs every suite from a local checkout of the specs repository instead, and prints that the run is not the pins. CI never sets it |
| For each target, `make conformance` runs the suite of every pinned specification; a suite with no case for the target's role says so and does not fail | A component is conformant to what it implements only if all of them pass | `SUITES="relay-claim relay-guest-api"` runs exactly those specifications' suites |
| `relay-oci.toml` names its container runtime, `runtime = "docker"` | It is what the hosted runner has; an `oci` target must name one, and the harness has no default of its own | `OCI_RUNTIME=podman make conformance` runs that target with `podman` and nothing else |

`make pin-specs` has no default: it is given `SPECS` or `REF`
(Decision 7), and with neither, or with both, it is an error that says
so. `SPECS=latest` is a value the caller writes, not a default. The
`replace` line `REF` writes in a module is not a fallback either: it is
written when the specification names a harness version that has no tag,
which the command reads before it writes the module, and not after a
build has failed (GLOBAL-11). `GOWORK=off` is set by the recipes and is
not a setting. The `go` line of a pin module is read from the release
it pins and is not a setting either (Decision 7). The
harness's own timings (the readiness poll, a case's time limit) are
constants of a harness release, not settings.

No fallback is added, and three that might be expected are rejected:

- A target that was named, or that declares the host's OS, and cannot run
  there (a relay target named off Linux, or run without root or user
  namespaces; `relay-oci` without the runtime it names) is an error that
  names what is missing. It is not skipped, the harness does not look for
  another runtime, and `exec` is not tried in place of `oci` (GLOBAL-11).
  The default's leaving out a target whose file does not declare the host's
  OS is not a fallback: it is decided from the file before anything
  starts, and it is printed (a18).
- A failed case is not retried (TEST-04).
- A pin that cannot be fetched fails the run. There is no cached or bundled
  copy of an older release, and a specification whose pin cannot be
  fetched is not left out of the run.

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
that seam's pin.

## Specification changes

The five seams this design declares have no written specification today.
Each is defined by Go code, which the table in **Problem** names. This
item writes the first version of each. It changes no behaviour
(Decision 1), so it changes no existing requirement: every requirement is
new and states what `main` does.

| Seam | Specification document, in the specs repository | Requirements | Checked by |
|---|---|---|---|
| 1 `relay-claim` | `relay-claim/v1.md`, with `v1.openapi.json`, `v1.config.schema.json` and `v1.identity.schema.json` | `CLAIM-<n>`, all new | Its suite: against a relay (relay role) and against a collector that claims (claimant role) |
| 2 `collector` | Two documents (a10): `collector-outputs/v1.md` with `v1.openapi.json`, and `collector-inputs/v1.md` with `v1.openapi.json`, `v1.complete.schema.json` and `v1.status.schema.json` | `COLLOUT-<n>` and `COLLIN-<n>`, all new | Each one's suite: against a collector, and against a relay (the relay's side) |
| 3 `relay-guest-api` | `relay-guest-api/v1.md`, with `v1.artifacts.openapi.json`, `v1.cwd.openapi.json` and `v1.control.openapi.json` | `GUEST-<n>`, all new | Its suite, against a relay (relay role). The build environment's side: no cases yet (a11) |
| 5 `executor-initialize` | `executor-initialize/v1.md` | `INIT-<n>`, all new | No cases yet (a11); the Go tests in `cmd/warden-io/` |
| 6 `bootstrap-config` | `bootstrap-config/v1.md`, with `v1.schema.json` | `BOOTCFG-<n>`, all new | Its suite: against a relay (as carried) and against a collector (as produced). As the executor reads it: no cases yet (a11) |

Each row's document is released under its own version (a8). The six
first releases are each `v0.1.0`.

The operations of the six OpenAPI files, which four specifications
have, are new too (a9). They add no requirement: each operation and
status names the requirements that state it (**The OpenAPI
descriptions**).

**What changes in them.** Each document is new. What it must state is
listed seam by seam under **What each specification covers**, and that
list, with Decision 1, is what the design review decides (LOOP-02). The
identifiers are given when the documents are written (Decision 3), so the
design names the prefixes and not the numbers. Each document also lists
the divergences that concern its seam (**Divergences found**) under
"Known divergences", as the code behaves.

**Where the change is made.** In `buildwarden/buildwarden-specs`, on its
branch `feat/seam-specs`, in one pull request that Jeff merges
(`f6-release-specs`, a15). The releases this repository will pin are the
six specifications' `v0.1.0` (Decision 4), each in its own module,
`tools/conformance/<directory>/go.mod` (Decision 7). Until those releases
exist, this branch pins a commit of the specs branch.
No specification under `specs/` in this repository changes.
`specs/README.md` gains one sentence that points to the book's seams
page, and `specs/specs_test.go` one test (**The book and the
specifications link both ways**).

**The order in Implement** (LOOP-02). The documents are written first
(tasks 2 to 5), then the harness and the suites (tasks 6 and 7), then the
Go relay and the disk collector are tested against them (task 8). No code
in this repository follows a specification here, because no behaviour
changes. A case that fails because a document misread the code is
corrected in the document. One that fails because the code does
something this design does not record returns the item to Design with
the finding (FLOW-06). No case is weakened and no code is changed to
make a suite pass.

**How it is reported** (LOOP-06). A run that changes a document names it
in its digest section apart from the code, and the pull request's body
lists the six documents under a heading of their own.

**Seams this design does not declare.**

- Seam 4, the ledger format (`ledger-format`): no change. Its
  specification stays in this repository, `specs/build-ledger/v1.md` and
  the schema documents under `specs/`, until M6 moves it with RELAY-01 to
  RELAY-08 (a1). The harness's ledger verifier implements
  the reader's side of that document (the header, each record's
  signature and chain, the hash blocks) and reads the metadata fields its
  cases need by the names `specs/metadata-<name>/v1.md` gives them. Those
  documents carry no requirement identifiers to cite. The verifier reads
  the format from this repository: its package comment names the document
  and the commit of `buildwarden/buildwarden` it was written from, and
  the specs repository holds no copy of the document or of a schema.
  RELAY-05 keeps a version 1 ledger readable, so a verifier written from
  `v1.md` stays right until the format's version changes. A change to the
  format before M6 is a RELAY-05 change in this repository, and its design
  says whether a suite case reads what it changes.
- The bootstrap model (`bootstrap-model`, BOOT-07): untouched.
- Seam 9, the driver contract: `driver-contract`'s.

After this item a change to one of the five seams is a pull request to
the specs repository, a release of the specification it changes, and a
move of that specification's pin here (**Releases and how they are
consumed**).

## Rule changes

Five amendments: TEST-05, TEST-02, GLOBAL-08, EXEC-04 and PROFILE-01. Each
is made in Implement, after the review approves it by name (FLOW-14).
`driver-contract` amends three of the same rules, GLOBAL-08, TEST-05
and PROFILE-01; **With `driver-contract`**, after the five, says which
sentences each design owns.

**None is a loop rule.** A feature's design may declare a change to the
invariants and profile of a component it works in and to their
enforcement, and never to a LOOP, FLOW or LINT rule or its enforcement
(FLOW-14 as amended, LOOP-09). The five were checked against that on
2026-11-15:

- *The rules.* GLOBAL-, EXEC-, TEST- and PROFILE- are prefixes whose
  owner is `buildwarden/buildwarden` in the ID scheme table of
  `docs/development/README.md`, and the four files edited
  (`global.md`, `in-vm-executor.md`, `testing.md`, `loop-profile.md`) are
  on its list of project invariants.
- *The enforcement.* `tools/tier1/rules.yaml` holds entries for rules of
  both kinds, and the rule ID on the `rule_changes` entry decides, not
  the path. This item edits two things in it: the `seams` list, which is
  the data of `seam-contract`, the check of GLOBAL-08 and EXEC-04, and
  TEST-05's owner string. No FLOW or LINT entry is touched. The check's
  code does not change; one test of it does. `TestSeverities`
  (`tools/tier1/coupling/checks_test.go`) runs the coupling checks with
  the real `rules.yaml`, and its table of expected severities gains
  GLOBAL-08 (**GLOBAL-08**, Enforcement change). The table's other rows
  are BOOT-07, ORCH-01 and GLOBAL-04; it holds no row for a loop rule,
  and none of its rows changes.
- *What this item would want of a loop rule* it asks as a question and
  does not declare: FLOW-04's list and FLOW-08's sentence (a15).

Two things this item builds are near a loop rule and change none. They
are stated so that the design review can judge them (LOOP-09):

- The `pin` job of the `conformance` workflow fails a pull request in
  which a specification's pin is not a release tag. That is this
  repository's own check of how it pins releases (TEST-05 and GLOBAL-08
  as proposed; the profile's
  Specifications entry). LOOP-07, point 5, says the same of any
  component's trunk and is agent-judged; the job is not its enforcement
  and no loop rule names it.
- The specs repository gets `.entire/settings.json` and a `flow18`
  workflow (task 1). They are how a worker bound there by FLOW-18 keeps
  to it (LOOP-08), set up as this repository has them. The loop's own
  check, `tools/workflow/flow18-check.sh`, and this repository's `flow18`
  workflow are not edited.

The umbrella's table of rule changes, which is "for direction", gives S1
two: RELAY-05 and TEST-05. This design declares TEST-05 and leaves RELAY-05
to M6, which moves seam 4 (a1; the table's row is amended, **Amendments to
the umbrella design**). It also declares four that the table does not give
S1: TEST-02, GLOBAL-08, EXEC-04 and PROFILE-01. The table gives GLOBAL-08
to K1 and to S2, S3 and S4 for their seams; S1 needs it because S1 is the
item in which the seams' contract documents come to exist. PROFILE-01 did
not exist when the table was written.

### TEST-05 (amend), `docs/development/testing.md`, `tools/tier1/rules.yaml`

**Why:** after this item the contract tests for the relay's and the
collector's seams are the specs repository's suites, run black-box. The rule
as written describes Go tests inside this repository. This design rewrites
the rule's title and its whole body as it is on `main`; `driver-contract`
proposes one paragraph more at the rule's end.

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
> to verify against. Each specification there is released under its own
> version, and a release carries the suite that checks it. This repository
> pins a tagged release of each specification in
> `tools/conformance/<specification>/go.mod` and runs those releases'
> suites against the components it holds, in the `conformance` workflow
> (TEST-02):
>
> - against the Go relay in each of its modes (RELAY-04) and as its OCI
>   image: the suites of the claim protocol, of both specifications of the
>   collector protocol (its outputs, and its inputs and signals) as the
>   relay is their client, of the relay guest API, with the RELAY-02
>   byte-exact round-trip (sent equals recorded), and of the bootstrap
>   config as the relay carries it;
> - against the disk collector (`cmd/collector`): the suites of both
>   specifications of the collector protocol, of the claim protocol as
>   its claimant, and of the bootstrap config as it is produced.
>
> The contract tests of a seam specified there are added to the suite of
> its specification in the specs repository, not to this repository. Two
> contracts stay as tests
> here until a suite covers them: RELAY-05's signature round-trip and reader
> backward-compatibility (`relay/`, `ledger/`), and EXEC-04's protocol
> conformance (`cmd/warden-io/`, `bootstrap/`), whose specifications
> (`executor-initialize`, and `bootstrap-config` as the executor reads it)
> have no cases yet.
>
> The automated part is the `conformance` workflow and `make test`; the
> agent part judges whether a seam change added its cases to the suite.

Reworded on 2026-11-15 for a8 and a10, in the review's first sitting. The
wording Jeff read at `e4801d6` said "pins one tagged release of the specs
repository" and "runs that release's suites", and its two bullets began
"the relay suite" and "the collector suites" and spoke of "both parts of
the collector protocol", and its third paragraph ended "have no suite
yet", where a specification now has a suite with no cases. The current
wording above is `main`'s, checked again against `ad4ea50`.

Reworded again on 2026-11-15 for a20, in one path. The wording at
`e19cc1f` read "pins a tagged release of each specification in
`tools/conformance/go.mod`"; it now names
`tools/conformance/<specification>/go.mod`. No other word of the rule
changes. In the rule text `<specification>` is a specification's name,
which is the name of its directory in the specs repository and of its
module here (`<directory>` elsewhere in this design).

**Severity and check:** MUST and `automated + agent-judgment`, before and
after.

**Enforcement change:** in `tools/tier1/rules.yaml` the TEST-05 entry's owner
changes from `"make test (the round-trip tests exist as tests)"` to
`"the conformance workflow (the specs repository's suites at the pinned
releases) and make test (the ledger round-trip tests)"`. No check code
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
>   runners, on PRs and on pushes to a branch other than `main` that change
>   a package the relay or the disk collector is built from, a file their
>   builds read, the pins of the specification releases and the target
>   files (`tools/conformance/`) or the workflow itself; weekly on `main`;
>   and on demand.
>   `conformance.yml`'s `paths` list is that set's implementation.

The sentence names the kinds of path and points to the workflow for the
list, as the tier-0 bullet does with `main.yml`'s `changes` job. At
`f1-design` it copied the list, which would have been a second copy to
keep in step, and it left out the workflow file itself and the wider push
trigger (**CI**).

Reworded on 2026-11-15 for a8, in four words: the sentence Jeff read at
`e4801d6` said "the specs pin and its target files
(`tools/conformance/`)". The bullet's wording on `main` was checked again
against `ad4ea50`.

Not changed by a20, and approved by name on 2026-11-15 on the wording at
`e19cc1f` (**Design review**). The sentence names the directory
`tools/conformance/`, which holds the six pin modules and the target
files as it held the one module, and names no file in it. The proposed
wording above is byte for byte what it was at `e19cc1f`.

**Severity and check:** unchanged (MUST; automated + agent-judgment). No
enforcement change: the workflow is new and is none of the jobs FLOW-14
names.

### GLOBAL-08 (amend), `docs/development/global.md`, `tools/tier1/rules.yaml`, `tools/tier1/coupling/checks_test.go`

**Why:** the contract documents of five seams now exist, in another
repository, and the bootstrap config becomes a written seam. The rule's
automated part says "contract document"; for these five, what this
repository holds of a specification's document is the file that pins
its release.

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
> (TEST-05), each released under its own version; the collector protocol
> is two specifications, its outputs and its inputs and signals. A change
> to one of those five seams changes its specification first: that
> specification is released, and the change here moves its pin,
> `tools/conformance/<specification>/go.mod`, to that release. For them
> those pin files are the contract documents the automated part checks.
> The ledger format stays in `specs/` (RELAY-05).

The paragraph states the obligation ("changes its specification first")
and names the pin files only as what the automated part checks. The
check asks for the pin of the seam whose file changed, and for both
pins of seam 2 where a file holds code of both its specifications;
**Tier 1's seam list** says what that holds and what is left to Review.

Reworded on 2026-11-15 for a8 and a10. The paragraph Jeff read at
`e4801d6` had no clause on versions or on the collector's two
specifications, and read "the specification is released, and the change
here moves the pin (`tools/conformance/go.mod`) to that release. For
them the pin is the contract document the automated part checks." Its
last sentence is unchanged by a1: the ledger format stays in `specs/`
until M6, whose item rewrites the sentence with RELAY-05. The executor
bullet, above, is unchanged, and `main`'s wording was checked again
against `ad4ea50`.

Reworded again on 2026-11-15 for a20, in two sentences of the added
paragraph. At `e19cc1f` they read "the change here moves its pin in
`tools/conformance/go.mod` to that release. For them that file is the
contract document the automated part checks." They now name
`tools/conformance/<specification>/go.mod` and read "those pin files are
the contract documents". The executor bullet and the rest of the
paragraph are as at `e19cc1f`.

**Severity and check:** unchanged (MUST; automated + agent-judgment).

**Enforcement change:** `tools/tier1/rules.yaml` gets the `seams` entries
and the comment under **Tier 1's seam list**, in place of `seams: []`:
six entries for the five seams, two of them for `collector`. The
`seam-contract`
check's code does not change; it starts reporting a change to a listed file
that does not also change the pin its entry names, which for a file
listed in both `collector` entries is both pins. Because this edits
`tools/tier1/`, Review also runs the merge base's tier 1 with the branch's
configuration (FLOW-14). That code reads the list as written, which was
tried at `ad4ea50` (**Tier 1's seam list**), and it should report
nothing new on this branch, which touches none of the listed files.

One test of the check changes with the data. `TestSeverities`
(`tools/tier1/coupling/checks_test.go`) runs every check over a fixture
that changes `cmd/warden-io/jks.go`, with the real `rules.yaml`. The
list puts that file on the `executor-initialize` seam, so the test's
table of expected severities gains GLOBAL-08 as MUST:

```go
want := map[string]report.Severity{"BOOT-07": report.Hint,
	"ORCH-01": report.Must, "GLOBAL-04": report.Hint,
	"GLOBAL-08": report.Must}[f.Rule]
```

The third line is added, and the second ends with a comma where on
`main` it closes the table. No other test and no code under
`tools/tier1/` changes. The edit was tried with the list at `ad4ea50`:
without it `make test` fails in `tools/tier1`, and with it every
package there passes (**Tier 1's seam list**).

What differs from `e19cc1f` in this enforcement change, where the rule
text differs as said above:

- *The list.* At `e19cc1f` it had five entries, one for each seam, and
  each named `tools/conformance/go.mod` as its contract. It now has six,
  two of them for `collector`, each with its own specification's pin.
- *The comment above `seams`,* in two places. Its second sentence ended
  "the file that pins the release of each specification this repository
  implements (TEST-05)." and now ends "the file that pins the release of
  its specification here (TEST-05).", and it gains a third sentence, for
  the collector seam's two entries.
- *The test,* and its file among this entry's paths. At `e19cc1f` the
  paths were `global.md` and `rules.yaml` alone, and the design said
  that no file but `rules.yaml` changed under `tools/tier1/`. The second
  review of a20's fold-in ran the test against the list and found that
  it fails. The fixture's file was already on the list at `e19cc1f`.

### EXEC-04 (amend), `docs/development/in-vm-executor.md`

**Why:** the same change, for the executor's rule. Its list of what the
executor's interface is gains the bootstrap config, so that it and
GLOBAL-08's executor bullet name the same three things. At `f1-design`
only the last paragraph was amended, and the two rules would have listed
the interface differently.

**Current wording** (the rule's body):

> The executor exposes a versioned public interface under GLOBAL-08:
>
> - the `initialize` invocation contract: how an orchestrator launches it, its env,
>   args, and exit reporting; and
> - its side of the relay protocol: CA install, artifact `POST`, heartbeat, and exit
>   report.
>
> Changes follow the pre/post-1.0 seam posture and update the contract document. The
> automated part requires a change to the contract surface to also change its contract
> doc.

**Proposed wording:**

> The executor exposes a versioned public interface under GLOBAL-08:
>
> - the `initialize` invocation contract: how an orchestrator launches it, its env,
>   args, and exit reporting;
> - the bootstrap config it reads (`bootstrap/`); and
> - its side of the relay protocol: CA install, artifact `POST`, heartbeat, and exit
>   report.
>
> Changes follow the pre/post-1.0 seam posture and change the specification
> first (GLOBAL-08): `executor-initialize`, `bootstrap-config` and
> `relay-guest-api` in the specs repository, each released under its own
> version. The automated part requires a change to the contract surface,
> as `tools/tier1/rules.yaml` lists it, to also change the pin of the
> specification it is listed under
> (`tools/conformance/<specification>/go.mod`).

Reworded on 2026-11-15 for a8: the last paragraph Jeff read at `e4801d6`
ended "to also move the specs pin (`tools/conformance/go.mod`)" and had no
clause on versions. `main`'s wording was checked again against `ad4ea50`.

Reworded again on 2026-11-15 for a20, in the end of the last sentence.
At `e19cc1f` it ended "to also change the file that pins those releases
(`tools/conformance/go.mod`)". There is no longer one such file. Tier
1's list puts the files of `cmd/warden-io/` under `executor-initialize`
and `bootstrap/bootstrap.go` under `bootstrap-config`, and the check
asks for that specification's pin. The executor's use of the relay
guest API is in `cmd/warden-io/` and is stated by `executor-initialize`
(**Seam 5**), so the `relay-guest-api` pin is asked of a change to the
relay's files and not of the executor's. Nothing else in the rule
changes.

**Severity and check:** unchanged (MUST; automated + agent-judgment). No
enforcement change beyond GLOBAL-08's.

### PROFILE-01 (amend), `docs/development/loop-profile.md`

**Why:** PROFILE-01 is this repository's profile under the Loop Contract.
It says of itself: "An entry that no longer matches the repository is a
finding, and the change that makes it stale changes the entry." This item
makes three of its statements stale. It pins specification releases,
where the Specifications entry says none is pinned. It consumes those
releases, where the Consumables entry says the repository consumes nothing.
And it adds the `conformance` workflow, a deterministic check of the
Specification class that the Checks table does not list. LOOP-01 asks the
Specifications entry for "the seams it owns or implements, where each
specification is, and how it pins a release", and LOOP-07 asks the
profile for the specifications each release implements, so the proposed
wording says those too.

This entry was owed from the day `loop-contract` merged ahead of this
item (its **What `seam-specs` and `driver-contract` owe the profile**).
Until 2026-11-15 this design carried a note in its place, which set out
three orders for that merge, this design's review and this item's pull
request. The one that happened is the merge before the review, so the
entry is here before the review and costs no return to Design.

`driver-contract` changes one of the three rows below as well, the
Specifications entry. The wording here is this item's change to each row
as it is on `main`. **With `driver-contract`** gives the Specifications
entry as it reads with both changes; the other two rows are changed by
this item alone. That design was approved on 2026-11-15 at `a6c8f0b`,
with its PROFILE-01 change. The approval covers its own clause and
sentence in the Specifications entry; this design's sentences in that
entry are approved here, in the wording below.

**Current wording, the Specifications row:**

> | Specifications | `specs/`: the ledger wire format and the metadata and environment schemas (RELAY-05), mirrored in the book. For the seams of GLOBAL-08, their contract documents are the `seams` list of `tools/tier1/rules.yaml` and their tests are TEST-05's. No specifications release is pinned today. |

**Proposed wording, the same row.** The first two sentences are kept word
for word, and the third is replaced by three:

> | Specifications | `specs/`: the ledger wire format and the metadata and environment schemas (RELAY-05), mirrored in the book. For the seams of GLOBAL-08, their contract documents are the `seams` list of `tools/tier1/rules.yaml` and their tests are TEST-05's. The claim protocol, the collector protocol, the relay guest API, the `initialize` contract and the bootstrap config are specified in `buildwarden/buildwarden-specs`, each specification released under its own version. This repository implements them and pins a release of each specification in `tools/conformance/<specification>/go.mod`; those files are their contracts in that list (GLOBAL-08). Each pin is a release tag on `main` and on a pull request to it; a branch may pin a commit of an unmerged specifications branch while its work is in progress. |

**Current wording, the Consumables row:**

> | Consumables | Releases: the `v*` tags. Consumes: nothing. |

**Proposed wording, the same row:**

> | Consumables | Releases: the `v*` tags. A release that carries the files `tools/conformance/<specification>/go.mod` implements each seam specification at the version its file pins. Consumes: the releases of the seam specifications in `buildwarden/buildwarden-specs`, each with its conformance suite, at the versions in those files. |

**Current wording, the Specification row of the Checks table** (the row
for tier 1; the row beneath it, for the judgment half, is not changed):

> | Specification | The coupling checks that `tools/tier1/rules.yaml` maps to RELAY-05, GLOBAL-08, EXEC-04, UX-01, UX-03, ORCH-03 and BOOT-07; the contract tests of TEST-05; the `keytool` job | 1 |

**Proposed wording, the same row:**

> | Specification | The coupling checks that `tools/tier1/rules.yaml` maps to RELAY-05, GLOBAL-08, EXEC-04, UX-01, UX-03, ORCH-03 and BOOT-07; the `conformance` workflow, which runs TEST-05's suites at the pinned specification releases and whose `pin` job requires release tags on a pull request; the contract tests of TEST-05 that run in `make test`; the `keytool` job | 1 |

Reworded on 2026-11-15 for a8, in all three rows. At `e4801d6` the
Specifications row read "pins one release of that repository" and "The
pin is a release tag", with no clause on versions; the Consumables row
read "at the version that file pins" and "Consumes: one release of
`buildwarden/buildwarden-specs`, the seam specifications and their
conformance suites, at the version in `tools/conformance/go.mod`"; and
the Checks row read "at the pinned specifications release and whose `pin`
job requires a release tag". The three rows' wording on `main` was
checked again against `ad4ea50`.

Reworded again on 2026-11-15 for a20, in two of the three rows. In the
Specifications row one sentence changes: at `e19cc1f` it read "pins a
release of each specification in `tools/conformance/go.mod`, which is
their contract in that list (GLOBAL-08)"; it now names
`tools/conformance/<specification>/go.mod` and reads "those files are
their contracts in that list". In the Consumables row both sentences
that named the file change: at `e19cc1f` they read "A release that
carries `tools/conformance/go.mod` implements each seam specification
at the version that file pins for it" and "at the versions in
`tools/conformance/go.mod`". The Checks row names no file and is byte
for byte as at `e19cc1f`.

The tier in that table is LINT-02's: 1 is deterministic and blocks. The
`conformance` workflow is also tier 1 of TEST-02's test tiers, which the
proposed TEST-02 sentence says; the two numbers agree here and mean
different things. The paragraph under the table, "Declared as not
blocking", is not changed: this item adds no hint and no report to this
repository. The specs repository's `implementations` workflow is not a
merge gate, and it is that repository's, not an entry of this profile.

**Entries that are not reworded,** and why this item leaves each true:

| Entry | Why it stands |
|---|---|
| Impact vocabulary | It points to the `seams` list of `tools/tier1/rules.yaml` for the seam names, and this item fills that list. The entry states no name of its own |
| Test tiers | It points to TEST-02, which this design amends |
| Toolchain | "Go, at the version in each module's `go.mod`" takes in the six nested modules under `tools/conformance/`. Each names the Go its pinned release names, which this repository's Go is at least (Decision 7) |
| Fast gate | `make conformance` is not part of it. `make test` gains one tier-0 test |
| Reference documents | No surface changes (`surfaces: []`). The new book page is a concept page |
| Sweep | No Go package is added to this repository's module, and the modules under `tools/conformance/` hold no Go code |
| Release | This repository's release is as it was. The specs repository's `release` workflow is its own |
| Sessions | This repository's are as they were. The specs repository's `.entire/settings.json` is its own |
| Component, Trunk, Rules, Designs, Native hosts, Boundary rule, Flaky-test rule, Rule and machinery paths | Nothing they state changes |

`driver-contract` rewords the Impact vocabulary and Reference documents
entries, and the Safety row of Checks, for what K1 changes. Nothing this
item builds gives a reason to.

One thing was seen in passing and is not this item's. The Impact
vocabulary entry says the seam names are the `seams` list of
`rules.yaml`. The template's examples and the `seams` of other designs
include `ledger-format` and `bootstrap-model`, which are not in that list
today, when it is empty, and are not in it after this item. The review may
place that where it belongs.

**Severity and check:** MUST and `agent-judgment`, before and after. After
the change every term is still filled and none is `none`, which is what
the design review of a change to this rule judges (LOOP-01).

**Enforcement change:** none. PROFILE-01 has no automated part and no
entry in `tools/tier1/rules.yaml`. The file is under `docs/development/`,
so the rule-change clearance covers the edit through this entry (FLOW-14).

### With `driver-contract`: which sentences each design owns

This design's own wording for TEST-05, GLOBAL-08, EXEC-04 and PROFILE-01
was reworded on 2026-11-15, in the first sitting of its review, for a tag
per specification and two collector documents (a8, a10), and again the
same day for a `go.mod` for each specification (a20). Which sentences
each design owns did not change by either, and the joined row and the
joined owner string below carry the new words.

`driver-contract` is read here at `a6c8f0b`, the commit its design
review approved on 2026-11-15 (its Design review entry, at `c767992`).
The approval names its eleven rule changes, GLOBAL-08, TEST-05 and
PROFILE-01 among them, and for PROFILE-01 "four rows, the Specifications
row in both forms (K1's alone, and joined with `seam-specs`'
sentences)". Neither item has merged, and either may merge first.

Its GLOBAL-08 and TEST-05 changes, and the entry it adds to tier 1's seam
list, are word for word what this section was first written against at
`0c29fc3`; they were compared again at `a6c8f0b`. Its PROFILE-01 change
has moved since this section read it at `bb4d7c8`. It has withdrawn the
words it added to the tier-1 Specification row of Checks, so it changes
four rows of the profile where it changed five, and one row is changed
by both designs: the Specifications entry. It now has a table of its own
for that rule, which agrees with the rows below.

| Text | This design owns | `driver-contract` proposes | Neither changes |
|---|---|---|---|
| GLOBAL-08, from "Versioned seams" | The executor bullet, which gains the bootstrap config. One new paragraph, placed directly before the rule's last paragraph | A third bullet, the driver contract. The paragraph that begins "Pre-1.0 these seams may still move", rewritten | The relay bullet. The last paragraph ("The automated part requires ...") |
| TEST-05 | The title, and the whole body as it is on `main` | One new paragraph at the rule's end | |
| `rules.yaml`, `seams` | Six entries for its five seams, two of them for `collector` (a20), and the comment above the list | One entry, `driver-contract` | |
| `tools/tier1/coupling/checks_test.go` | One entry added to `TestSeverities`' table, under **GLOBAL-08** | Nothing. Its entry lists neither file of that test's fixture | |
| `rules.yaml`, TEST-05's owner | The string, as under **TEST-05** | The string on `main`, with "the driver suite's static tier runs there" added inside its parentheses | |
| TEST-02, EXEC-04 | All of each amendment | Nothing | |
| PROFILE-01, the Specifications entry | The three sentences that replace its last one, "No specifications release is pinned today." | In its first sentence, "and the driver contract (`specs/driver-contract/`, GLOBAL-08)" after "(RELAY-05)". A new second sentence, "The driver contract's conformance suite is `conformance/driver/`." It keeps the last sentence as it is on `main` | The sentence that begins "For the seams of GLOBAL-08" |
| PROFILE-01, the tier-1 Specification row of Checks | The clause on the `conformance` workflow, placed before "the contract tests of TEST-05", and the words "that run in `make test`" after it | Nothing. At `bb4d7c8` it added ", the driver conformance suite (`conformance/driver/`) among them" after "the contract tests of TEST-05"; it has withdrawn those words | The clause on the coupling checks; "the `keytool` job"; the tier |
| PROFILE-01, the Consumables entry | All of it | Nothing. Its table of entries reads "Consumes: nothing" as still true of K1, and says that this design rewrites the entry | |
| PROFILE-01, the Impact vocabulary and Reference documents entries, and the tier-1 Safety row of Checks | Nothing | One sentence added to the first; the first sentence of the second rewritten; the `driver-contract` job added to the third | |
| PROFILE-01, the rest | Nothing | Nothing | The rule's header and paragraphs, every other entry, the other rows of Checks and "Declared as not blocking" |

A third item amends PROFILE-01. `loop-parallel`, a process item whose
design is in review and not approved (read at `e692fad`), adds an Areas
row to the profile's table and changes nothing else in the rule. Its
draft at `486ac8f` also changed the header's check line, the paragraph on
the agent part and the Process row of Checks; it no longer does. The
Areas row is none that either design here changes, and whichever merges
after it keeps it.

The result reads the same whichever merges first. GLOBAL-08: the relay
bullet, the executor bullet, the driver-contract bullet, the "Pre-1.0"
paragraph, this design's paragraph, the last paragraph. TEST-05: this
design's title and body, then the driver contract's paragraph. For
PROFILE-01 the two designs' words go into one row, the Specifications
entry, which is given in full.

**The Specifications entry, with both changes:**

> | Specifications | `specs/`: the ledger wire format and the metadata and environment schemas (RELAY-05) and the driver contract (`specs/driver-contract/`, GLOBAL-08), mirrored in the book. The driver contract's conformance suite is `conformance/driver/`. For the seams of GLOBAL-08, their contract documents are the `seams` list of `tools/tier1/rules.yaml` and their tests are TEST-05's. The claim protocol, the collector protocol, the relay guest API, the `initialize` contract and the bootstrap config are specified in `buildwarden/buildwarden-specs`, each specification released under its own version. This repository implements them and pins a release of each specification in `tools/conformance/<specification>/go.mod`; those files are their contracts in that list (GLOBAL-08). Each pin is a release tag on `main` and on a pull request to it; a branch may pin a commit of an unmerged specifications branch while its work is in progress. |

In order: `driver-contract`'s first sentence and its sentence on the
suite; the sentence neither changes; this design's three. "No
specifications release is pinned today." is not in it whichever merges
first, because after this item one is pinned. The row holds until K2,
which moves the driver contract's specification and declares that change
itself.

**The tier-1 Specification row of Checks** is changed by this design
alone, now that `driver-contract` has withdrawn its words there. With
both items merged it reads as under **PROFILE-01** above. Its words "the
contract tests of TEST-05 that run in `make test`" take in the driver
suite's static tier, which runs in `make test` (that design's proposed
TEST-05 and owner string); that design reads the row the same way. Until
this pass the section gave a joined form of the row, with that design's
withdrawn words in it. It is removed, and is not put to Jeff.

This design's review is asked to judge the Specifications entry in both
forms: as under **PROFILE-01** above, which is what it says if this item
merges first, and joined as here, which is what it says once both have
merged. The Consumables entry and the Checks row have one form each.

Both forms are given as they read with a20's answer. `driver-contract`'s
approval at `a6c8f0b` took the row "in both forms" too, and its joined
form quotes this design's three sentences as they stood before a8 and
a20. What that approval covers in the row is that design's own clause
and sentence, which are word for word the same in its joined row and in
the one above. This design's three sentences are approved in this
review, in the wording here, and the joined row above is the row as it
will read once both have merged.

**Pointed out to `driver-contract`.** That design is not edited from
here, and its review has ended. One point is live. Its own joined
Specifications row, approved with its PROFILE-01 change at `a6c8f0b`,
quotes this design's three sentences as they were at `e4801d6`: "pins
one release of that repository in `tools/conformance/go.mod`" and "The
pin is a release tag". a8's answer changed them the same day, and a20's
changed the second of the three again, to the wording above: "each
specification released under its own version", "pins a release of each
specification in `tools/conformance/<specification>/go.mod`; those
files are their contracts in that list" and "Each pin is a release tag".
That design says its quotation "is read again against the outcome" if
this review changes the three sentences, and it has. On the reading
stated at the end of this section, what that approval covers in the row
is that design's own clause and sentence; this design's three sentences
are approved in this review, in the wording here. Whichever merges
second corrects its quotation in Consolidation (FLOW-10).

The three points this section made against that design at `bb4d7c8` are
answered in it: it says this design declares a PROFILE-01 change and
rewrites the Consumables entry, it gives a joined Specifications row,
and it no longer changes the Checks row.

Three sentences of this design were written, or reworded in the revision
of 2026-11-14, so that both sets are true together:

- *TEST-05.* "The contract tests of a seam specified there are added to
  the suite of its specification in the specs repository" read "A seam's
  contract tests are added ...", which the driver suite, in this
  repository until K2, would have made false.
- *GLOBAL-08.* This design's paragraph names its five seams and says "For
  them those pin files are the contract documents". It says nothing of a seam
  specified elsewhere, and "The ledger format stays in `specs/`" does not
  say that only the ledger format is there, so the driver contract's
  bullet, which names `specs/driver-contract/v1.md`, stands beside it.
- *The comment above `seams`.* Its first sentence is true of any entry,
  its second speaks only of a seam specified in the specs repository,
  and its third, added for a20, only of the collector seam. The
  redraft says it corrects the comment "to match" and gives no wording.

**What the second to merge does**, in `f6-merge-main` and
`f6-consolidate`:

1. It merges `main`. Git will report conflicts. In `global.md` the
   executor bullet and the new third bullet are adjacent lines. In
   `rules.yaml` the `seams` list and the TEST-05 owner are the same
   lines. In `loop-profile.md` the Specifications entry is one line,
   changed by both. Each is resolved by keeping both, in the order
   above.
2. `seams` holds all seven entries, this design's six and that
   design's one, under this design's comment. If this
   item is second, it replaces the comment it finds; if `driver-contract`
   is second, it leaves this one and may add a sentence for its own entry.
3. TEST-05's owner is the two halves joined: `"the conformance workflow
   (the specs repository's suites at the pinned releases) and make test (the
   ledger round-trip tests; the driver suite's static tier runs there)"`.
4. The Specifications entry of the profile is the joined row above, word
   for word.
5. It brings its own quoted "current wording" up to date in Consolidation
   (FLOW-10). The redraft quotes GLOBAL-08 from "Versioned seams" to the
   rule's end, all of TEST-05, and four rows of the profile; this design
   quotes the executor bullet, all of TEST-05, and three rows of the
   profile.

Every sentence stays as its review approved it, so the second to merge
needs no new rule change and no new review: `tools/tier1/rules.yaml` and
the three rule files (`global.md`, `testing.md`, `loop-profile.md`) are
approved paths of both designs, and Review judges that each edit matches
the approved wording (FLOW-14). If the merge needed a sentence reworded,
that is a return to Design (FLOW-06).

For the profile that rests on one reading, on which `driver-contract`'s
approval is read here and which this review is asked to confirm: what a
design's review approves in a row is the words the design adds or
replaces, and not the rest of the row as it quoted it from `main`. Both
designs already read GLOBAL-08 and TEST-05 that way. On it,
`driver-contract` merging second adds its clause and its sentence to the
Specifications entry it finds, which no longer has the sentence its
quotation keeps, and its Consolidation corrects the quotation. Read the
other way, the second to merge has a row its review did not approve, and
returns to Design for it (FLOW-06).

The redraft says the same of GLOBAL-08, the seam list and the owner string
from its side (its **Rule changes**, GLOBAL-08 and TEST-05), and of
PROFILE-01 that whichever lands second "merges `main` and keeps both", with
the quotation refreshed in Consolidation, and it gives the joined
Specifications row. It does not say which comment stands above `seams`
or give the joined owner string; for those, whichever design merges
second is read against this section.

## Security note

Not required: `security_boundary` is `none`. No code on the host, environment
or relay boundary changes, and no relay code changes at all (GLOBAL-06).

What the review should still know: the specifications state today's posture
on the environment-to-relay boundary, and the relay suites are the check the
umbrella's Security note relies on when the Rust relay replaces the Go one.
Their cases therefore include the refusals, not only the happy paths: a claim
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
  amended rules, and `rules.yaml` parses with the new `seams` entries,
  in the branch's tier 1 and in the merge base's (`make enforcement`,
  FLOW-14); tier 1's own tests pass with the list, `TestSeverities`
  with GLOBAL-08 in its table (**Rule changes**, GLOBAL-08);
  `tier1 designs` accepts this design; the book-link test checks each
  row's ref against that specification's pin, read from its own module.
  In the specs repository:
  its `ci` workflow, which for every specification runs the layout check,
  the coverage check, the OpenAPI checks where it has such a file, and
  its suite against the harness's own roles, and which holds the two
  collector documents' common part alike (a8, a9, a10).
- **Tier 1** (the `conformance` workflow): the relay suites (the five
  specifications' cases for a relay, **The harness**) against
  `dist/conformance/relay` in host, vm and container mode and against the
  image built from `Dockerfile.relay`, on `ubuntu-latest`; the collector
  suites (four specifications' cases) against `dist/conformance/collector`
  on `ubuntu-latest`,
  `macos-latest` and `windows-latest`. This is S1's done-when: the suites
  pass against the Go relay and the disk collector. The first green run is a
  push run on this branch during Implement, before anything is merged or
  tagged in the specs repository. The last push run of Implement is on the
  final pins, at the specs pull request's head (task 12), and it is the run
  Review and `f6-release-specs` rest on. The pull request's run repeats it
  on the six tags, with the `pin` job, which also writes each of the six
  modules again from the version it names and fails on a difference
  (Decision 7). The existing `integration` workflow
  does
  not run for this branch's paths and is not affected.
- **Tier 2, native:** nothing blocks. The change reaches no driver's tree
  and alters no behaviour a native host exercises (FLOW-09). Two gaps are
  tracked, in `tier2_gaps` and on the native-gap backlog, and both are
  `deferred`, so no scheduled native session is woken for either (a14):
  - `macos-vz`: the relay suites against the host-mode relay on macOS,
    where vz runs it. The frame link is the same code as on Linux; what
    differs is the platform (the link kickstart, socketpair behaviour). It
    cannot run with what this item builds: the relay targets declare Linux
    only, because the `exec` launcher's sandbox (the relay's upstream
    network and its `/etc/resolv.conf`) is Linux namespaces. The entry's
    note names what it waits on: a macOS form of the launcher, in which
    the harness's upstream is loopback aliases in the documentation ranges
    with ports 53, 80 and 443, which needs root, and the cases for the
    resolver default are not applicable. The first item that needs the
    relay suites on macOS builds it and sets the entry `open`; that is
    expected to be the Rust relay's host mode on VZ (the umbrella's M3).
    At `f1-design` the entry was `open` and named `dist/relay`; the
    revision before review corrected both.
  - `qemu-any`: the relay suites against the Go relay VM image
    (`tools/relay-vm`). It cannot run until a VM launcher exists (a6), so
    its backlog entry is `deferred`, with a note that names what it waits
    on, and no scheduled native session is woken for it. The item that
    builds the launcher sets it `open`; the Archive summary tells it to.
  The Hyper-V relay VM is the same relay binary in vm mode behind a
  different boot channel, which is seam 8's; no entry is added for it.

## Implementation plan

Tasks name their repository: **S** is `buildwarden/buildwarden-specs`, on
its branch `feat/seam-specs`; **K** is this repository, on this branch.

0. (Human, umbrella a11, during the design review) Jeff creates the
   repository as **What Jeff creates** says (a2). Implement checks it
   first, against the list there, and stops with a blocking question if it
   is missing or differs.
1. **S** The layout: `README.md`, the seven directories each with its
   `go.mod` and `CHANGELOG.md`, the workspace file `go.work` with its
   `replace` line, `make tidy`,
   `.entire/settings.json` naming the private sessions repository, and
   the `ci` and `flow18` workflows (FLOW-17, FLOW-18, a4), the `ci` checks
   of the layout, of the modules, of the OpenAPI files (a9) and of a
   release (a8) among them. This task also shows that the mechanics
   hold, with a harness that already imports one of its real
   dependencies and a specification that names a harness version with no
   tag: the workspace builds, vets and tests, `make tidy` leaves a
   complete `go.mod`, and `ci` is green on the branch (**Releases and how
   they are consumed**).
2. **S** `relay-claim/v1.md` with its two schemas and its OpenAPI file
   (GLOBAL-08, RELAY-08, RELAY-04, a9). D17 is reproduced by hand first,
   against a relay built from `main` (**Divergences found**).
3. **S** `collector-outputs/v1.md`, and `collector-inputs/v1.md` with the
   schemas of the completion's body and of the reply to `GET /v1/status`,
   each with its OpenAPI file; the part both state,
   written in each, and the check that keeps the two the same (RELAY-07,
   umbrella a1, a10, a9).
4. **S** `relay-guest-api/v1.md` with its three OpenAPI files (BOOT-04,
   RELAY-02, a9). D18 is reproduced by hand first, in the same way.
5. **S** `executor-initialize/v1.md` and `bootstrap-config/v1.md` with its
   schema (EXEC-04, BOOT-05).
6. **S** The harness, the module `conformance/`: target files, the
   `exec`, `oci` and `external` launchers, the Linux sandbox (network
   namespaces, the mount namespace for the relay's `/etc/resolv.conf`,
   the order of a container-mode relay's interfaces), the frame link, the
   claimant, collector, relay, resolver and origin roles, the ledger
   verifier, and the runner and its report (GLOBAL-03, GLOBAL-07,
   GLOBAL-09 to GLOBAL-11).
7. **S** Each specification's suite, in its own module with its command:
   the cases for a relay in `relay-claim`, `collector-outputs`,
   `collector-inputs`, `relay-guest-api` and `bootstrap-config`, then the
   cases for a collector in the four that have them, each case naming
   requirements of its own specification; `executor-initialize`'s
   command, with its requirements listed and the reason they have no
   case; each module's test against the harness's own roles; and the
   coverage check for each specification (TEST-04, TEST-05, TEST-07).
8. **K** `tools/conformance/` (the six pin modules, each with its one
   requirement and its `tool` directive, and the five target files in
   `targets/`); the `Makefile`'s
   `conformance` target, which
   builds `dist/conformance/relay`, `dist/conformance/collector` and the
   `relay-oci` image and runs each specification's command from its own
   module with `GOWORK=off`, and its `pin-specs` target, which writes
   each module it moves from nothing; and the `conformance`
   workflow with its `pin` job (TEST-02, TEST-05, GLOBAL-09 to
   GLOBAL-11). The first commit of this task shows that the mechanics
   hold with the real module names and the real harness's dependencies:
   a pin module at the specs branch's head resolves, with its `replace`
   line, and `go tool` builds its command on each of the three runners
   (**Releases and how they are consumed**). Done when the
   workflow's run for the pushed head is green on all three runners. A
   case that fails because the
   specification misread the code is fixed in **S**, and one that fails
   because the code misbehaves in a way this design does not record goes
   back to Design (FLOW-06, Decision 1).
9. **K** The rule changes, each as approved: TEST-05, TEST-02, GLOBAL-08,
   EXEC-04 and `tools/tier1/rules.yaml` (the six `seams` entries, their
   comment and TEST-05's owner), the one entry added to the table in
   `tools/tier1/coupling/checks_test.go` that GLOBAL-08's enforcement
   change gives (the check's code does not change; one test of it
   does), and PROFILE-01's two entries and one row of its Checks table
   in `docs/development/loop-profile.md` (FLOW-14, LINT-02, LOOP-01).
   Done when `make test` passes, in `tools/tier1` as in the other two
   modules, and `make enforcement` passes: the merge base's tier 1 reads
   the list.
10. **K** The book: `concepts/seams.md` and its entry in `SUMMARY.md`,
    the edits to `concepts/architecture.md` (its three tables),
    `contributing/structure.md`, the sentence in `specs/README.md`, the ref
    test in `specs/specs_test.go`, `AGENTS.md` and `CONTRIBUTING.md`
    (UX-01, FLOW-15). And the amendments to the umbrella,
    `docs/design/componentization.md`, each as approved (U1 to U3,
    **Amendments to the umbrella design**).
11. **S** The `release` and `implementations` workflows; each directory's
    `CHANGELOG.md` names `v0.1.0`, and each specification's `go.mod` names
    the harness at `v0.1.0`; open the pull request (FLOW-08).
12. **K** The final pins, as Implement's last commit:
    `make pin-specs REF=<the specs pull request's head commit>`,
    `make test`, `make lint`, push. Done when the `conformance` run for
    that push is green on all three runners and each of the six modules
    names the specs pull request's head. So the commit Review reads in
    the specs repository is the one the pull request would merge, and
    the head Review reads here has a green run. A later commit to the
    specs branch in Implement repeats this task.
13. (Human, `f6-release-specs`, after Review) The tier-2 MUST findings
    that are in the specs repository are listed in the specs pull
    request's body, and Jeff merges it (FLOW-08). The seven tags are then
    pushed on the merge commit, one push for each: `conformance/v0.1.0`
    first, its `release` run green before the next, and then each
    specification's `v0.1.0`. The six pins are moved here with
    `make pin-specs SPECS=latest`, which writes each of the six modules
    again at its tag and drops its `replace` line, by him or by the
    attended session on his word (a15). The step is done when the seven
    `release` runs are green, the six specifications' with their
    binaries attached and the harness's with none, and this branch's
    `conformance` run on the six tags is green.

No item is filed for seam 4 after the merge: its move is M6's (a1).

Owed to later items, and written into the Archive summary so that their
intake finds it:

- seam 4's move to the specs repository, at M6, with RELAY-01 to RELAY-08
  (a1): RELAY-05, tier 1's `schema-doc` root, the book's `specs/` mirror
  and its redirects, `specs/specs_test.go`, and the reserved directory
  names (Decision 2). The harness's ledger verifier then reads the format
  from its own repository;

- the executor's suite, with S2's item (a11); the umbrella's S2 row does not
  name it;
- the VM-image launcher, with the first item that has a VM image to test
  (a6); that item also sets the `qemu-any` backlog entry `open`;
- the macOS form of the `exec` launcher, with the first item that needs
  the relay suites on macOS (a14); that item also sets the `macos-vz`
  backlog entry `open`;
- the specs repository's adoption of the Loop Contract, which is an item
  Jeff files (a4, LOOP-08), and the driver contract's move there in K2
  (**What the specs repository leaves room for**);
- each divergence the review turns into work, in the item the review names
  (**Divergences found**).

## Design review

**2026-11-15, first sitting: eighteen questions answered; five answers
folded in.** Jeff read the design at `e4801d6` and answered a1 to a18 in
the attended session. The review has not ended. When this entry was
written, the five rule changes, the amendments to the umbrella, the two
questions the fold-in raised and the outcome were all still to come;
the last entry of this section says which of them have happened since.
Nothing in this section records an outcome.

Confirmed as recommended, with no change to the design beyond marking
the question answered:

- a3: public from creation.
- a4: the invariants that apply in the specs repository are GLOBAL-01 to
  GLOBAL-04, GLOBAL-07, GLOBAL-09 to GLOBAL-11, TEST-04 and TEST-07 for
  the harness's Go code, and RELAY-05's schema-identity rule for the
  schemas.
- a5: the harness is Go, in the specs repository, and imports nothing
  from the kit.
- a6: the `exec`, `oci` and `external` launchers now.
- a11: the executor's suite is deferred to S2's item.
- a12: each divergence is recorded as the code behaves, and none is fixed
  here.
- a13: seam 6 is `bootstrap-config`.
- a14: nothing blocks natively; two tracked gaps, both deferred.
- a15: the human step `f6-release-specs`, as an exception to FLOW-04's
  list and FLOW-08's sentence, for this item only.
- a16: the collector's outputs specification is normative for the wire
  only.
- a17: `relay/relay.go`, `relay/proxy.go` and `cmd/relay/` stay off tier
  1's seam list.
- a18: `make conformance` with no target runs the targets that declare
  the host's OS.

Five answers depart from what the design proposed. Each was folded in
during the sitting, in its own commit; none is a return to Design
(FLOW-06), and `f2-design-review` stayed claimed throughout.

- **a1, (C): seam 4 moves at M6, with RELAY-01 to RELAY-08** (`c94b814`).
  Decision 9 and a1 are rewritten. No later S1 item is owed for the
  ledger's move; the move is listed among the work owed to M6. The ledger
  format stays under `specs/` here, RELAY-05 is not changed, and the
  harness's ledger verifier reads the format from this repository. The
  umbrella's seam 4 row and two rows of its rule table are listed for
  amendment (**Amendments to the umbrella design**, new).
- **a2, (B): the repository is created now, before the approval**
  (`fa65b2e`). **What Jeff creates**, a2, the Blocking paragraph and task
  0 are rewritten to what Jeff confirmed for creation:
  `buildwarden/buildwarden-specs`, public, a README stub and the
  Apache-2.0 `LICENSE` and no `NOTICE`, the protection of `main` and the
  Claude GitHub App. Jeff creates it himself, because the cloud session
  cannot. What Implement checks first is listed, and Implement's blocking
  question stays for a repository that is still missing.
- **a10, (B): two collector documents** (`e361370`). Seam 2 is written as
  `collector-outputs/v1.md` and `collector-inputs/v1.md`, with the
  prefixes `COLLOUT` and `COLLIN`. What the two share is written in full
  in both and kept alike by a check. Seam 2 stays one seam, `collector`,
  in GLOBAL-08 and in tier 1's list. Decisions 2, 3 and 13, a10, a16's
  title, D16's row, the suites' table, **Specification changes** and task
  3 follow.
- **a9: the numbered requirements as proposed, and OpenAPI for the HTTP
  seams as well** (`c09f668`). **The OpenAPI descriptions** is new: four
  specifications get OpenAPI 3.1 files in JSON, the requirements stay
  normative, three `ci` checks hold the files to them, and what OpenAPI
  cannot say stays prose. Decisions 2 and 3, a9, **CI**, **Specification
  changes** and tasks 1 to 4 follow.
- **a8, (B), with a7 as restated: a tag per specification, each release
  carrying the build of the tooling that tests it** (`b6f3d21`). Each
  specification is a directory and a Go module, tagged
  `<directory>/vX.Y.Z`, with its cases and its command; the shared harness
  is the module `conformance/`; `tools/conformance/go.mod` names the six
  specifications each at a version. Decisions 2, 4, 5, 7 and 12, a5's
  note, a7, a8, **The harness** (its opening, to the instances), **What
  the specs repository leaves room for**, **Releases and how they are
  consumed**, **CI**, **Tier 1's seam list** (the comment and what the
  check holds), the book's page, **How the loop treats the second
  repository**, **Defaults and overrides**, **Specification changes**,
  the **Validation plan** and tasks 1, 6 to 8 and 11 to 13 are rewritten.
  Decision 4 states what the scheme costs beside the single tag.

What the fold-in did to the rule changes, which had not then been put to
Jeff: the proposed wording of TEST-05, GLOBAL-08, EXEC-04 and PROFILE-01 (three
rows, and the joined Specifications row under **With `driver-contract`**)
is re-derived for a8 and a10, and each entry says what the wording he
read at `e4801d6` was. TEST-02's sentence changed in four words, for six
pins. `rule_changes` is unchanged: the same five rules and the same
paths. The wording on `main` was checked again at `ad4ea50`.

Two questions were new, and Jeff has answered both since (the last
entry of this section). **a19**: where the code every suite shares lives
and how it is versioned. The design stood on its option A, the module
`conformance/` with tags of its own. **a20**: which build of a release's
tooling this repository runs. Until it was answered the design stood on
a7's recorded answer, one `go.mod`, and recommended a `go.mod` for each
specification.

In the queue file: a19 and a20 added, open at the time; the note of
`f6-release-specs` rewritten for seven tags, each in its own push, and
`make pin-specs SPECS=latest`; no step added or removed; no recorded
answer touched. `design_review` stays `required`, `alters_relationships`
stays `true`, and `native_validation` and the two tracked gaps are as
they were.

**The same sitting: a pass on an independent review of the fold-in.**
Each finding was checked against its source before it was acted on, and
all held. No recorded answer and no decision of Jeff's changes by it.
What changed (`8b5fa1b` to the commit that adds this paragraph):

- *The mechanics behind a19's option A* had been tried only with modules
  that import nothing. With a harness that imports a third-party module,
  as the real one must, the workspace does not build a harness version
  that has no tag, a specification's `go.mod` cannot be tidied, and a
  build in a checkout at the tag fails. **Releases and how they are
  consumed** now states what makes each work, as tried again: a
  versioned `replace` line in `go.work`, `make tidy` through a temporary
  `replace` with a `ci` check, and a release built with
  `go install <command>@<tag>`. a19's options A and B carry those costs,
  and task 1 proves them with a real dependency.
- *`driver-contract` had moved,* to `a6c8f0b`: Jeff approved its eleven
  rule changes by name that evening. It withdrew its words in the tier-1
  Specification row of Checks, so the joined Checks row this design gave
  is removed, and one profile row is shared, the Specifications entry.
  Its own joined Specifications row quotes this design's sentences as
  they were before a8; that is pointed out. `loop-parallel`'s PROFILE-01
  change is read at `e692fad`.
- *The umbrella.* `driver-contract` also amends line 405, the row U2
  rewords, and adds a dated paragraph where U3 does. U2 is given in both
  forms, and the second to merge keeps both.
- *The release step.* Each tag is pushed in its own push, because GitHub
  starts no run for a push of more than three tags; the `release`
  workflow can be started by hand for an existing tag; the step is done
  on seven green runs, six with binaries.
- *What a tag per specification costs* is stated in full in Decision 4:
  the counts, what a harness fix costs, and that one specification's
  change can force releases of others. Decision 7 said that a mixed set
  of pins can fail to build and that `SPECS=latest` is the supported
  set; a20's answer has since removed that failure (below).
- *a20 was new* (above).
- *OpenAPI.* `collector-inputs` gains a schema for the reply to
  `GET /v1/status`, so that the check on JSON bodies can hold, and the
  list of what OpenAPI cannot say gains names that span path segments.
- *Smaller.* Six documents and six OpenAPI files where two sentences
  counted five and four; "no cases yet" for "no suite yet" in the
  proposed TEST-05; the `pin` job's rule and Decision 7's sample as
  `go mod tidy` writes it.

**The same sitting, continued, 2026-11-15: a19 and a20 answered; the
amendments to the umbrella and TEST-02 approved by name; a20's answer
folded in.** Jeff read the design at `e19cc1f`. The review has not
ended, and nothing in this entry records an outcome.

- **a19, (A): a seventh module `conformance/`, tagged on its own; a
  change that needs both the harness and a specification is one pull
  request and one merge.** As recommended, and as the design already
  stood. Nothing changes but the wording that treated it as open.
- **a20, (C): a `go.mod` for each specification, under
  `tools/conformance/<directory>/`.** As recommended. The design had
  stood on A, one `go.mod`, which a7's recorded answer names, and is
  revised (`56964d5` to the head this entry is pushed with). Each command
  is built from exactly what its release names, a mixed set of pins
  always builds, and each seam has its own contract path in tier 1.
- **The amendments to the umbrella, U1, U2 and U3, by name,** on the
  wording he read at `e19cc1f`; U2 in both of its forms, the row alone
  and the row joined with `driver-contract`'s change to the same line.
- **The rule change TEST-02, by name,** on the wording he read at
  `e19cc1f`. Its sentence names the directory `tools/conformance/` and
  no file in it, so a20's answer does not touch it.

The wording of U1, U2 in both forms, U3 and TEST-02 is byte for byte
what it was at `e19cc1f`. A script compares the three table rows, the
joined row and TEST-02's proposed wording in this file with the same
lines of that commit, and was run on the head this entry is pushed
with.

What a20's answer changed:

- *Decision 7* is rewritten: six small modules under `tools/conformance/`
  beside `targets/`, each requiring one specification and naming its
  command in a `tool` directive; the file as `go mod tidy` writes it;
  why a module is written from nothing by `make pin-specs` and never
  edited in place; why there is no `go.work`, and `GOWORK=off` in the
  recipes; `REF` and its `replace` line, now in each module whose
  specification names a harness version with no tag; and the two
  options not taken, among the rejected alternatives.
- *Decision 4's list of costs.* The mixed set that does not build and
  the one harness for all six commands are gone. Stated in their place:
  six modules and twelve files; a harness fix reaches a suite here only
  through that specification's release; and the suites one component is
  run through need not share a harness version.
- *The mechanics* were tried in the model, with Go 1.26.3, and are under
  **Releases and how they are consumed** with what was not tried.
- *The `pin` job* checks each of the six modules and writes each again
  from the version it names (**CI**).
- *Tier 1's seam list.* Each seam's contract path is its own
  specification's pin. `seam-contract` reads one path for an entry and
  its code is not edited, so seam 2, with two specifications, has two
  entries, both named `collector`; a file that holds code of both is
  asked for both pins. Tried with `main`'s check code at `ad4ea50`,
  where `tools/workflow/enforcement-check.sh` passed (FLOW-14).
- *The rule changes still to be put to him.* The proposed wording of
  TEST-05, GLOBAL-08 and EXEC-04, and of the Specifications and
  Consumables rows of PROFILE-01, names
  `tools/conformance/<specification>/go.mod` where it named the one
  file, with the plural that follows. The Specifications row is given
  alone and joined, as it will read. PROFILE-01's Checks row names no
  file and is unchanged. Each entry says what the wording at `e19cc1f`
  was. `rule_changes` named the same five rules and the same paths when
  this was written; the pass recorded below adds one path to GLOBAL-08's
  entry.
- *Smaller.* a7, a8, a10 and a17 each gain a dated note, and no recorded
  answer is edited. `make conformance`, the book's ref test, **How the
  loop treats the second repository**, **Defaults and overrides**,
  **Specification changes**, the **Validation plan** and tasks 8, 9, 12
  and 13 name the six modules.
- *`driver-contract`* is stated as approved: its design review approved
  it on 2026-11-15 at `a6c8f0b` (its entry is at `c767992`), with its
  PROFILE-01 change and the Specifications row "in both forms". Its
  joined form quotes this design's sentences as they stood before a8
  and a20. That approval is read here as covering its own clause and
  sentence in the row; this design's sentences are approved in this
  review (**With `driver-contract`**).

In the queue file: the text of a19 and a20 and the note of
`f6-release-specs` are brought in line with this; the recorded answers
are as the review session wrote them, and no step is added, removed or
changed in status. `design_review` stays `required`,
`alters_relationships` stays `true`, and `native_validation` and the
two tracked gaps are as they were.

**The same sitting: a pass on a second independent review, of a20's
fold-in.** Each of its six findings was checked against its source, and
re-run where it reported a trial; all six held. No answer of Jeff's
changes by it, and the wording of U1, U2 in both forms, U3 and TEST-02
is still byte for byte that of `e19cc1f`: the same script was run on
the head this paragraph is pushed with.

- *A test of tier 1 changes with the seam list.* `TestSeverities`
  (`tools/tier1/coupling/checks_test.go`) runs the coupling checks with
  the real `rules.yaml` over a fixture that changes
  `cmd/warden-io/jks.go`, which the list puts on a seam. With the list
  in place `make test` failed in `tools/tier1`; with GLOBAL-08 added to
  the test's table it passed, and `enforcement-check.sh` passed. The
  design had said that only `rules.yaml` changes under `tools/tier1/`.
  `rule_changes` gains that file among GLOBAL-08's paths, before
  GLOBAL-08 is put to Jeff; the five rules are the same five. The
  entry's Enforcement change gives the edit, **Tier 1's seam list** the
  trial, and task 9 and the **Validation plan** name the test. The
  fixture's file was already on the list at `e19cc1f`, so this is not
  a20's doing.
- *The `go` line of a pin module.* The design did not say what
  `make pin-specs` writes there, and said that the compiler is always
  this repository's. Decision 7 now says both. The review proposed the
  root module's `go` line. Tried, that builds a command that differs
  from the release's own binary, because Go takes a build's default
  `GODEBUG` settings from that line; with the line the pinned release's
  `go.mod` names, the build's settings were the release's. So the design
  writes the release's line, and the root module's is a rejected
  alternative there. A release that names a newer Go than runs here
  cannot be pinned until this repository's Go moves, which Decision 7
  states with what Go prints. The sample's `go` line is the model's.
- *A quotation of the proposed GLOBAL-08,* under **With
  `driver-contract`**, was the sentence as it stood at `e19cc1f`. It is
  the proposed sentence again.
- *The comment above `seams`* changed in two places for a20, and the
  design named one. GLOBAL-08's entry now lists what differs from
  `e19cc1f` in its enforcement change: the list, the comment in both
  places, and the test.
- *Smaller.* "The pins" where a sentence written for one file still
  said "the pin"; the paragraphs the fold-in left unwrapped are
  wrapped.

The proposed rule text of TEST-05, GLOBAL-08, EXEC-04 and PROFILE-01 is
not changed by this pass: it is that of `9bbc8ef`. What changes in a
rule change is GLOBAL-08's paths and its Enforcement change. In the
queue file nothing changes: a19, a20 and the note of `f6-release-specs`
name no `go` line and no file under `tools/tier1/`.

Still to come: the rule changes TEST-05, GLOBAL-08, EXEC-04 and
PROFILE-01, each to be put to Jeff by name on the head this entry is
pushed with (PROFILE-01 as the Specifications row in both forms, the
Consumables row and the tier-1 Specification row of Checks; GLOBAL-08
with its three paths and the edit to the test); then the outcome. This
entry writes none of the lines FLOW-14's check reads for a rule change:
those are recorded with the outcome.

### 2026-11-15, first sitting, outcome: approved

- **r1, 2026-11-15: approved at `64a4a95`** (Jeff; read at `e4801d6`,
  `e19cc1f` and `64a4a95` as his answers were folded in; page
  https://claude.ai/artifact/GMZYnBWqKVGDAP8DfpiHVd). All twenty
  questions are answered, as the entries above record; a1, a2, a7, a8,
  a9, a10 and a20 depart from the draft and are folded in. Confirmed
  with the outcome, as a mechanic the design settled without a question:
  a pin module's `go` line is the pinned release's own (Decision 7). The
  three amendments to the umbrella design (U1, U2 in both forms, U3)
  were approved by name on the wording at `e19cc1f`, which is
  byte-identical at `64a4a95`; so was TEST-02. TEST-05, GLOBAL-08,
  EXEC-04 and PROFILE-01 were approved by name on the wording at
  `64a4a95`. `buildwarden/buildwarden-specs` was created by Jeff during
  the review (first commit `aeda265`: `README.md` and the Apache-2.0
  `LICENSE`; `main` protected by a ruleset: pull request required, no
  force-push, no deletion) and is attached to the session.
- **rule change TEST-05 approved at `64a4a95`**, as worded under **Rule
  changes**.
- **rule change TEST-02 approved at `64a4a95`**, as worded there.
- **rule change GLOBAL-08 approved at `64a4a95`**, as worded there, with
  its enforcement change (six `seams` entries; one table entry in
  `TestSeverities`).
- **rule change EXEC-04 approved at `64a4a95`**, as worded there.
- **rule change PROFILE-01 approved at `64a4a95`**: three rows, the
  Specifications row in both forms (alone, and joined with
  `driver-contract`'s clause and sentence).

## Consolidation

Filled in at Publish.

## Rollback notes

None.

## Archive summary

Filled in at Publish.
