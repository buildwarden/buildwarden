---
feature: componentization
status: draft
drivers: [interface]     # the program's reach: every driver. This item's own PR changes no driver.
seams: [relay-claim, collector, relay-guest-api, executor-initialize, ledger-format, bootstrap-model]
                         # the program's reach, for retrieval. This item's own PR changes none;
                         # each program item declares what it changes.
surfaces: [cli, config]  # the program's reach (the `warden` dispatcher). This item's PR: none.
security_boundary: env-relay  # declared conservatively for the program: the truststore moves
                         # into the relay and the relay is rewritten. See the Security note.
alters_relationships: true
design_review: required
native_validation: []    # this item's PR is rules and docs only
tier2_gaps: []
rules: [GLOBAL-06, GLOBAL-08, BOOT-01, BOOT-02, BOOT-05, BOOT-07, RELAY-04, RELAY-05, RELAY-07,
        ORCH-01, EXEC-04, TEST-05, FLOW-01, FLOW-03, FLOW-05, FLOW-14, FLOW-15, FLOW-19, FLOW-20]
rule_changes:
  - {rule: FLOW-20, change: add, paths: [docs/development/process.md]}
concepts: [componentization, component-independence, driver-contract, driver-binaries, kit,
           seams, specs-repo, conformance, relay-split, relay-release-forms, host-mode,
           truststore, build-link-contract, collector, loop-contract, feature-freeze, program]
related: [unified-bootstrap.md, tier1-static-checks.md, specs-migration.md,
          build-link-subnet.md, firecracker-driver.md,
          design-approved-rule-changes.md, workflow-streamline.md, native-host-loop.md]
---

# Componentization: the relay, the drivers, the kit, the specs and the loop become separate components

This is an umbrella design. It fixes where the program ends up, the seams
between the components, the order of the work, the rule changes it will need
and the feature freeze that goes with it. It does not design any one phase:
every phase below is its own queue item with its own design, which lists this
document in `related`.

## Problem

Three efforts were planned separately and turn out to be one.

1. **Drivers cannot be added from outside.** `driver.Driver` has four methods,
   but the real contract is much wider. `cmd/warden` names the drivers in
   about fifty places (`newDriver`, doctor, setup, image prepare, list and
   verify, clean, the Hyper-V service), and `driver.BuildRequest` is a union
   of driver-specific fields. An integrator who wants a driver for internal
   infrastructure has to fork the orchestrator. GLOBAL-08 says so on purpose:
   `driver.Driver` is "not covered by this obligation". Jeff wants each driver
   to be its own binary and its own repository on a common library, open to
   drivers the project does not write (2026-11-09, 2026-11-11).
2. **The relay is being rewritten against seams nobody wrote down.** The relay
   fork (decided 2026-11-07) is a Rust rewrite in `buildwarden/buildwarden-relay`
   that implements the existing claim and collector protocols and ships as a
   unikernel, a Linux appliance and native binaries. `tools/tier1/rules.yaml`
   records that those protocols and the executor's `initialize` contract "have
   no contract document yet", so its `seams` list is empty. Today the
   specification is the Go code.
3. **The loop's rules live in one repository.** `docs/development/process.md`,
   the `workflow` branch, `wf` and the skills govern this repository only. A
   second repository with its own queue needs a shared contract, and the relay
   repository already needs one.

Each of the three needs the same things: the seams written as specifications
that two languages can be tested against, a loop that spans repositories, and
a change to GLOBAL-08. Done one after another, they would change the same
rules and the same packages three times. Done together, the work is too broad
for the queue's normal limits, so Jeff has decided to freeze other feature
work until most of it has landed (2026-11-12).

## Prior art

- [`unified-bootstrap`](unified-bootstrap.md) signed BOOT-05 as it stands:
  the executor derives the whole baseline, the Java truststore included, from
  the CA. This program departs from that for the truststore (Decision 9), so
  it carries a Bootstrap comparison.
- [`tier1-static-checks`](tier1-static-checks.md) built the `seam-contract`
  check and left its seam list empty for want of contract documents. Phase S1
  fills it.
- [`specs-migration`](specs-migration.md) set the layout of `specs/` and the
  schema-document rule (RELAY-05). The same layout moves to the specs
  repository and is extended to the other seams.
- [`build-link-subnet`](build-link-subnet.md) made the build link a per-build
  value carried in the claim. The build-link contract (Decision 10) states the
  isolation guarantee that design relied on.
- [`firecracker-driver`](firecracker-driver.md), Decision 4: durable network
  slots made by `warden setup`. That is the case the second class of
  build-link conformance exists for.
- [`design-approved-rule-changes`](design-approved-rule-changes.md): a
  feature's design review may approve a rule change by name. Program items
  use that route for the rule changes listed under **Design**.
- [`workflow-streamline`](workflow-streamline.md) and
  [`native-host-loop`](native-host-loop.md): the loop that phase P1 moves.

Outside this repository, and restated here where the design relies on them:

- The relay fork's kickoff and decisions table (2026-11-07) and the Loop
  Contract draft. They are not yet in any repository; P1 and M1 give them
  homes.
- The VZ spike report, `spike/REPORT.md` on `buildwarden-relay` `main`
  (merged 2026-11-11), and the VZ networking report, `spike/vznet/REPORT.md`
  on its branch `spike/vz-net-entitlements`.

## Decisions

Decisions 1 to 17 are Jeff's, taken between 2026-11-07 and 2026-11-12. The
alternatives are the ones weighed in those sessions.

1. **One program under one umbrella design.** Alternatives: finish the relay
   split first, then the drivers; or the reverse. Both lost because the relay
   and the drivers need the same written seams and the same GLOBAL-08 change,
   and the loop change gates both.
2. **Each component gets its own repository and its own queue**, coupled only
   through written seams and tagged releases. The end state is the component
   table under **Design**.
3. **Seams are specifications with black-box conformance suites, in their own
   repository.** Alternative: Go contract tests in this repository (TEST-05 as
   it stands). It lost because a Rust relay cannot share them. The
   `buildwarden` book and the specs repository link to each other.
4. **Drivers become separate binaries on a public Go kit, then separate
   modules, then separate repositories.** Alternatives: only complete the
   in-tree contract (does not let an integrator ship a driver); drivers as RPC
   plugins (a second protocol to version, and no gain over a binary that links
   the kit). Hyper-V is extracted first as the pilot.
5. **A top-level `warden` dispatcher that works like git's CLI.** It finds a
   driver binary by name and runs it. Alternative: users call driver binaries
   directly; it lost because driver auto-detection and one entry point are
   worth keeping.
6. **The guest families are public, unstable kit packages for now.**
   `driver/linuxguest` (qemu, firecracker) and `driver/winguest` (qemu,
   hyperv) hold what drivers share because they boot the same kind of guest.
   Alternatives: a copy in each driver (the Windows guest contract would be
   defined twice and drift); their own module now (a repository and a release
   before any driver is extracted). They can be split out later with the
   executor.
7. **The executor stays with the kit for now.** The in-environment pieces
   should be split out eventually unless a strong reason against appears.
8. **The collector is specified like every other seam and must stay easy to
   split out.** Alternative collectors (streaming to cloud storage, other
   non-disk stores) are a goal for later, not for this program.
9. **Java truststore generation moves from the executor into the relay.** It
   is generic, it can be tested with the relay, and it should settle early
   with the ledger specification. The relay serves JKS and PKCS12. The Go
   relay serves JKS only; PKCS12 arrives with the Rust relay.
10. **The build-link contract has two classes of conformance over one
    behavioural guarantee.** The guarantee: from the build, only the relay
    answers; the host, other builds and the internet do not; nothing from an
    earlier build is visible. *By construction*: no host endpoint and a fresh
    link per build (vz, qemu after it inherits a socket pair, Hyper-V's
    per-build private switch). *By configuration*: host-owned devices, where
    the driver checks the configuration before each build (firecracker, whose
    slots are a host bridge and TAPs). Alternatives: name firecracker as an
    exception (does not work for an integrator's driver); loosen the wording
    until firecracker passes (the by-construction property is then stated
    nowhere). The class is documented with each driver and is not recorded in
    the ledger: a driver may move from one class to the other as its
    environment's features or its implementation change, and the ledger's
    metadata should not have to change when it does.
11. **The relay ships native binaries usable at host level on every supported
    operating system and architecture, as well as unikernel images.** More
    release forms, such as cloud images, may follow. Host mode is built during
    M3.
12. **The Linux appliance uses the kernel network path, shared with the OCI
    image.** The unikernel and host mode use the relay's own network stack
    over raw frames. Alternative: the relay's own stack on the appliance too.
    It lost because a container runtime gives the relay a kernel interface
    anyway, so one path for every form is not available, and the appliance is
    worth more as a form that does not depend on the newest code.
13. **The loop gets its own public repository**, `buildwarden-ai-workflow` or
    similar. Alternative: the sessions repository; it is private, so the queue
    and reports would stop being visible beside a public project. Work is
    both spec-first and design-first, depending on what is being implemented.
14. **Feature freeze.** Once this design is approved, all other feature
    development stops until the large majority of the program has landed.
    Queue items now on hold are not carried forward: they are closed now, and
    each goes through intake again afterwards (see **Items closed for
    re-intake**).
15. **BuildWarden carries forks of Hermit** (`hermit-kernel`, `hermit-loader`,
    `hermit-rs`). Nothing is raised with `hermit-os` until BuildWarden is
    verified fully working and performant, so there is a working example to
    point to. That point is the end of M6: every driver has adopted the Rust
    relay and the Go relay is retired.
16. **`buildwarden-relay` goes public at M1.**
17. **An index of known drivers** lets a driver be updated on its own
    schedule. It is the program's last phase and needs its own design.

Decisions 18 and 19 are this design's.

18. **A rule changes in the item that makes it true.** This item's PR adds
    one rule, FLOW-20. Every other rule change the program needs is declared
    by the program item whose code makes the new wording true, and approved
    by name in that item's design review (FLOW-14). Alternative: change every
    rule now. It lost because `main` would contradict its own rules until
    each phase landed.
19. **No waiver of the per-item design.** An earlier draft waived it for
    move-only changes. FLOW-19 already lets a review be skipped for an item
    that changes no relationship, seam or boundary and needs no blocking
    native validation, which is what a pure move is. A move that does need
    blocking native validation gets its review.

## Questions

### Blocking

None.

### Assumed

All twelve were settled in the design review of 2026-11-12. Each entry says
what was drafted and what was decided.

- **a1.** Confirmed. The collector specification is written in two parts, the
  outputs a collector stores and the inputs and signals it serves, so a
  different collector can take over the outputs later.
- **a2.** Changed. Drafted as a storage boundary inside the collector in K1.
  Decided: the collector gets no storage boundary in this program; it is
  designed with the first alternative collector.
- **a3.** Confirmed. Host mode is built for Linux first, then macOS, and for
  Windows only once a frame transport for it is designed. The Go relay
  refuses host mode on Windows, and nothing on Windows hands frames to a host
  process today.
- **a4.** Confirmed. The freeze exempts a fix for a regression on `main` and a
  fix for a security problem, and Jeff ends it no earlier than the pilot
  driver's release (wave 4).
- **a5.** Confirmed. FLOW-05's `alters_relationships` limit is raised from 1
  to 3 in `config.yaml` while the program runs; the limit of 5 features in
  progress is unchanged.
- **a6.** Confirmed. FLOW-15's overview sweep is deferred, within a phase, to
  the phase's last item.
- **a7.** Changed. Drafted as the signed environment record naming the
  build-link class. Decided: the class is not recorded in the ledger
  (Decision 10 gives the reason).
- **a8.** Changed. Drafted as leaving the held queue items on hold until P1.
  Decided: they are closed now (see **Items closed for re-intake**).
- **a9.** Changed. Drafted as "verified fully working and performant" meaning
  the first driver's adoption of the unikernel relay. Decided: it means the
  end of M6 (Decision 15).
- **a10.** Confirmed. The specs repository is `buildwarden/buildwarden-specs`.
- **a11.** Changed. Drafted as Jeff alone creating repositories and changing
  their visibility. Decided: Jeff does, or an attended session does on his
  word in that session.
- **a12.** Confirmed with the acceptance of the Bootstrap comparison, which
  covers the program's three model changes.

## Design

### Where the program ends

| Component | Repository | Language | Holds |
|---|---|---|---|
| Relay | `buildwarden/buildwarden-relay` | Rust | `relay-core`; host-mode binaries; the Hermit unikernel images; the Linux appliance; the OCI image |
| Hermit forks | `buildwarden/hermit-kernel`, `hermit-loader`, `hermit-rs` | Rust | Upstream plus carried commits |
| Loop | `buildwarden/buildwarden-ai-workflow` (name provisional) | n/a | The Loop Contract, the queue, run reports, `wf`, skills and agents |
| Specs and conformance | `buildwarden/buildwarden-specs` (a10) | Language-neutral, plus a test harness | One specification and one black-box suite per seam |
| Kit | `buildwarden/buildwarden` | Go | Session, claim client, ledger reader, script translator, the driver contract, `drivertest`, the `warden` dispatcher, the book |
| Collector | With the kit for now | Go | The disk collector, behind its specification |
| Executor and guest-side pieces | With the kit for now | Go | `warden-io`; `linuxguest`, `winguest` |
| Drivers | One repository each: container, qemu, vz, hyperv, firecracker | Go | One driver, as its own binary built on the kit |
| Driver index | To be designed (K5) | n/a | Known drivers and their current releases |

The Go relay (`relay/`, `cmd/relay`, `tools/relay-vm`) stays in this
repository until every driver has adopted the Rust relay, and is then retired.

### The seams

Every boundary between components is one of these. Seams 1 to 8 each get a
specification and a black-box conformance suite in the specs repository. Seam
9 is a Go API. Seam 10 is a document.

| # | Seam | Between | Written today | Change in the program |
|---|---|---|---|---|
| 1 | Claim protocol (`relay-claim`) | Driver and relay | No | Written down. The connection it runs over is supplied by the driver (TCP or vsock). |
| 2 | Collector protocol (`collector`) | Relay and collector | A package comment in `collector/collector.go` | Written down, with a suite that runs against any collector as well as any relay (a1). |
| 3 | Relay guest API (`relay-guest-api`) | Relay and executor | No | Written down. Truststore endpoints added. |
| 4 | Ledger format and schemas (`ledger-format`) | Relay and every reader | Yes, `specs/` | Moves to the specs repository. |
| 5 | `initialize` contract (`executor-initialize`) | Driver and executor | No | Written down. |
| 6 | Bootstrap config | Driver and executor, through the relay | Code only (`bootstrap/`) | Written down. |
| 7 | Build-link contract | Driver and the isolation guarantee | No | New (Decision 10). One shared suite proves the guarantee on every driver. |
| 8 | Relay release forms | Relay releases and drivers | No | New: each form (host-mode binary per platform, unikernel image, appliance image, OCI image) with a published hash, and the devices a relay VM needs. The list is open-ended. |
| 9 | Driver contract | Kit and drivers | The four-method `driver.Driver` | New Go API: registration, doctor, setup, images, clean, driver-owned configuration, the relay source and the claim connection; plus the convention by which the dispatcher finds and runs a driver binary. |
| 10 | Loop Contract | Every component's loop | Draft, outside any repository | Finished against the running loop. |

### The phases

Four tracks. The relay's milestones keep the numbers of its kickoff, which has
no M4.

**Track P, process**

| Phase | Work | Done when |
|---|---|---|
| P0 | This design, as a process item. | Jeff approves it and merges its PR. The merge starts the freeze. |
| P1 | Finish the Loop Contract against the running loop, covering spec-first and design-first work. Split `docs/development/` into project invariants and loop rules. Move the queue, `wf`, the skills and the agents to the loop repository. Make `wf` work across repositories, with holds per component. | This repository's loop and the relay's loop each complete one item from the loop repository. |

**Track S, seams**

| Phase | Work | Done when |
|---|---|---|
| S1 | Create the specs repository. Write seams 1, 2, 3, 5 and 6. Build the conformance harness so it runs against any relay binary or image, and against any collector. Link the book and the specs both ways. Fill `seams` in `tools/tier1/rules.yaml`. | The suites pass against the Go relay and the disk collector. |
| S2 | Truststore endpoints: the specification, then the Go relay serves JKS and the executor fetches it. | `cmd/warden-io/jks.go` is gone and the keytool comparison runs against the relay. |
| S3 | The build-link contract and its suite. Each driver declares its class. | The suite passes on every driver. |
| S4 | The claim connection: `driver.Session` takes a connection from the driver, not a TCP address. | Every driver claims through it. |

**Track K, kit and drivers**

| Phase | Work | Done when |
|---|---|---|
| K1 | Complete the driver contract in-tree, including the relay source. `cmd/warden` stops naming drivers. `drivertest` exists. | Adding a driver touches only its own directory. |
| K2 | Each driver becomes its own Go module and binary in-tree. `warden` becomes the dispatcher. A tier-1 check limits each driver to the kit's public packages. | The check passes for all five, and `warden` runs each driver as a separate binary. |
| K3 | Extract the Hyper-V driver as the pilot. | It releases from its own repository and passes native validation there. |
| K4 | Extract the remaining drivers. | This repository holds the kit, the collector, the executor, the dispatcher and the book. |
| K5 | The driver index. | The dispatcher can report and fetch a newer release of one driver. |

**Track R, relay** (run from the relay repository's own queue)

| Milestone | Work |
|---|---|
| M0 | VZ spike. Done: Hermit boots under VZ on the forks; the relay has two NICs, with the build link as a socket pair and the host reached over vsock. |
| M1 | Workspace, lints, CI, the Loop Contract adopted, reproducible image tooling. The repository goes public. The conformance harness comes from S1. |
| M2 | `relay-core` on the Linux appliance and the OCI image, over the kernel path. UDP and DNS on the build link. |
| M3 | The Hermit unikernel on firecracker, qemu and vz, over the frame path, with throughput measured. Host mode on Linux first (a3). |
| M5 | Truststore endpoints to the S2 specification, PKCS12 included. |
| M6 | Reconciliation: one "adopt relay version X" item per driver in that driver's queue. Then the Go relay is retired. |

**Order**

| Wave | Runs together | Gate |
|---|---|---|
| 0 | The queue drain, then P0 | P0 merged; the freeze starts |
| 1 | P1, S1, M1 | Loop repository live; suites pass on the Go relay |
| 2 | S2, S3, S4, K1, M2 | Driver contract complete |
| 3 | K2, M3 | Modules isolated; unikernel and host mode pass the suites |
| 4 | K3, M5 | Pilot driver released |
| 5 | M6 per driver, K4 | Go relay retired |
| 6 | K5 | Driver index live |

What sets the order: S1 comes before M1's real work, because the relay has no
written target until then. P1 comes before K3, because a second repository
needs a loop that spans repositories. K1 does not depend on the relay and can
start as soon as P0 merges. M6 needs K1's relay source, not driver extraction.

**Later, not scheduled.** The program's job is to leave each of these easy:
alternative collectors and a collector repository (seam 2; the collector's
storage boundary is designed with the first of them); the executor
and guest families in their own repository (seams 3, 5 and 6); more relay
release forms such as cloud images (seam 8); upstreaming to `hermit-os`
(Decision 15).

### How program items run

Each phase is one or more queue items, filed through intake after this item
merges. Every one has its own design, written from the template, with this
document in `related`; that is what makes it a *program item* (FLOW-20). A
phase that touches a seam changes the seam's specification first, then the
code (Decision 13). FLOW-19 decides, as for any item, whether its review can
be skipped.

Until P1 lands, program items run in this repository's queue under the rules
as they are, plus FLOW-20. The relay's milestones run in the relay
repository's queue.

New repositories (the loop, the specs, one per driver) are created, and
`buildwarden-relay` is made public, by Jeff, or by an attended session on his
word in that session (a11).

### Items closed for re-intake

Six queue items were on hold when this design was reviewed. They are removed
from the queue now and go through intake again after the freeze (Decision
14). Their queue files, with each item's source text, questions and history,
are on the `workflow` branch at `1a2252f` (`git show 1a2252f:queue/<item>.yaml`).
No remote branch is deleted.

| Item | Phase when closed | Branch on the remote | Design |
|---|---|---|---|
| `baked-asset-staleness` | 2, Plan | `feat/baked-asset-staleness` (`f68a8b1`) | On the branch, with assumptions a1 to a7 unreviewed |
| `relay-host-subnet-claim-only` | 0, not started | none | none |
| `relay-init-dhcp-mask-default` | 0, not started | none | none |
| `vz-ipsw-latest-reuse` | 0, not started | none | none |
| `cross-driver-image-update` | 0, not started | none | none |
| `security-suite` | 2, Plan | `feat/security-suite` (`e05df23`) | On the branch, narrowed, awaiting its design review |

`security-suite` is still named by `docs/development/README.md` (deep-dive
#1), by the SEC-02, SEC-06 and SEC-08 owners in `tools/tier1/rules.yaml`,
and by two entries in the native-gap backlog. Those pointers are stale until
it is re-intaken; they are left as they are.

### Rule changes the program will make

Listed for direction. None is approved by name here; each is declared and
approved in the design of the item shown (Decision 18).

| Rule | Change | Item |
|---|---|---|
| GLOBAL-08 | The driver contract becomes a supported seam, unstable before 1.0. The seam list gains the truststore endpoints, the build-link contract and the claim connection. | K1; S2, S3, S4 for their seams |
| BOOT-02, BOOT-05, the `bootstrap.md` mapping | As in the Bootstrap comparison. | K2; S2; S3 |
| RELAY-04 | Three-mode parity is restated for the Rust relay's release forms and its two network paths. | M6 |
| RELAY-01 to RELAY-08 | Move to the relay or specs repository, restated where they describe Go internals. | M6 |
| RELAY-05, TEST-05 | Contract tests for the seams become the specs repository's suites. | S1 |
| ORCH-01 | Driver parity becomes passing `drivertest`. | K1 |
| FLOW-*, LINT-* | Move to the loop repository as the Loop Contract's detail. | P1 |
| FLOW-20 | Retired. | A process item, when Jeff ends the program |

### Risks

- **Two relays until M6.** Every seam change lands twice. The freeze keeps
  other seam changes out.
- **Three Hermit forks with deliberate deviations**, such as fail-closed
  entropy, that may never go upstream.
- **Unproven in the Rust relay:** UDP and DNS on the build link, throughput
  on the frame path, and the async runtime on Hermit.
- **Host mode on Windows** has no precedent and no driver that would use it
  (a3).
- **What firecracker's unprivileged per-build preflight can check** of its
  slot configuration is not yet known; `doctor` checks the rules as root. S3
  has to settle it for the by-configuration class.
- **P1 pauses the loop** while the queue moves.
- **One person merges in every repository.**
- **A public Go API** has to be kept usable for integrators.

## Defaults and overrides

None. This item changes no behaviour. The defaults the program will add are
settled in the designs of the items that add them: which driver the
dispatcher picks when none is named and how it finds a driver binary (K2),
which relay release form each driver uses and how it is overridden (K1, M6),
and where a driver's own configuration lives (K1).

## Cross-driver impact

This item's PR changes no driver. Across the program every driver is touched
the same way: it implements the completed driver contract (K1), becomes a
module and a binary (K2), declares its build-link class (S3), supplies its
claim connection (S4), moves to its own repository (K3, K4) and adopts the
Rust relay (M6). What differs by driver:

| Driver | Build-link class | Relay form expected at M6 | Other |
|---|---|---|---|
| container | By configuration (a host-owned bridge; S3 decides how the contract's "build VM" wording covers it) | OCI image | |
| qemu | By construction, once it inherits a socket pair in place of the named `vlan.sock` | Unikernel or appliance | Uses `linuxguest` and `winguest` |
| vz | By construction (socket pair) | Unikernel, in a second VM; host mode stays available | The relay moves from a host process to a VM |
| hyperv | By construction (a private switch per build) | Appliance | The pilot extraction; uses `winguest` |
| firecracker | By configuration (durable bridge and TAP slots) | Unikernel or appliance | Uses `linuxguest` |

The relay form is each driver's own decision in its M6 item; the column is
what the relay's milestones are built to allow.

## Bootstrap comparison

Required because the program changes the model in three places (BOOT-07).
The review must accept this section explicitly (a12). The edits to
`bootstrap.md` are made later, by the items named, each declaring its
`rule_changes` with the wording agreed here.

### Current model

The diagram in [`bootstrap.md`](../development/bootstrap.md), unchanged on
`main`. The lines that matter here:

```
  (2) deliver the executor ----------------->  warden-io + a launcher
      per build, version-matched

                                                 |-- GET  /ca.pem         trust
                                                 |-- GET  /bootstrap      trust policy
                                                 |-- baseline, derived from the CA
                                                 |     by initialize itself
```

### Proposed model

```
  (2) deliver the executor ----------------->  warden-io + a launcher
      per build, from the driver's own
      kit version

                                                 |-- GET  /ca.pem         trust
                                                 |-- GET  /bootstrap      trust policy
                                                 |-- GET  the Java truststores (JKS, PKCS12),
                                                 |     generated by the relay from its CA
                                                 |-- baseline: installs the CA and the
                                                 |     truststores, sets the build-system
                                                 |     integrations
```

Everything else in the diagram stands. The per-driver mapping gains, in
column (1), the build-link class of each row.

### Per invariant

| Invariant | Current | Proposed | Per-driver effect | Item |
|---|---|---|---|---|
| BOOT-01 isolation precedes execution | "Topological isolation satisfies this by construction." Firecracker's row already describes isolation that rests on host configuration. | The requirement is unchanged. The rule refers to the build-link contract, and each mapping row names its class. | firecracker and container are named by-configuration; the others by-construction. qemu changes its link to an inherited socket pair. | S3 |
| BOOT-02 the executor is the only out-of-band input | "It is delivered per build and matches the orchestrator's version." | It is delivered per build and comes from the kit version the driver was built from. The relay is versioned separately; the executor and the relay agree through the relay guest API's version. | None in what is delivered. Each driver binary carries its own executor version. D2 (vz) is unchanged. | K2 |
| BOOT-03 one start, then hands off | As written | Unchanged | None | |
| BOOT-04 build inputs come from the relay | CA, script and context fetched by the executor | Unchanged | None | |
| BOOT-05 setup is the executor's domain | The baseline, the Java truststore included, is "derived from the CA" by `initialize`. "The orchestrator generates no setup material." "The guest runs no setup code it did not ship with." | `initialize` still applies the baseline. The Java truststores are fetched from the relay, which generates them from its CA. The orchestrator still generates no setup material, and the guest still runs no setup code it did not ship with: a truststore is data. | Every driver alike; the change is in the relay and the executor. | S2 |
| BOOT-06 images carry hooks, not content | As written | Unchanged | None | |

The vz row's wording does not change when its relay moves into a VM at M6:
"one socketpair NIC to the relay" describes both.

### Gains

- The truststore bytes the build receives are recorded in the ledger like
  every other byte the relay serves (RELAY-02). Today the executor writes them
  locally and they are in no record.
- One implementation of truststore generation, tested with the relay, where
  Java keystore formats are easy to get subtly wrong.
- The relay can be released on its own schedule without the executor having
  to match it build for build.
- Isolation that rests on configuration is named as such, per driver, where a
  reviewer can see it.

### Costs

- The relay gains code that builds keystores from its CA. It is new code on
  the environment-relay boundary (Security note).
- The executor depends on one more relay endpoint, so a relay that lacks it
  fails the build. The relay guest API's version has to say so.
- Version skew between a driver's executor and the relay becomes possible,
  where today they are built together. The conformance suites are what keeps
  them honest.
- The record gains the truststore bytes and loses nothing.

## Rule changes

### FLOW-20 (add), `docs/development/process.md`

**Why:** the freeze, the one raised limit and the two relaxations are
decisions that bind every worker and every intake for months. They belong in
the rulebook, where the loop reads them, and in one rule that can be retired
whole when the program ends.

**Current wording:** none; `process.md` ends at FLOW-19.

**Proposed wording:**

> ### FLOW-20 — The componentization program has the queue to itself
>
> **Severity:** MUST
> **Check:** agent-judgment
>
> The componentization program is the work set out in
> [`componentization`](../design/componentization.md): the relay, the
> drivers, the kit, the specs and the loop become separate components coupled
> through written seams. A *program item* is a queue item whose design lists
> that design in `related`. While this rule stands:
>
> - **Freeze.** Intake files a new item that is not a program item on hold,
>   with a blocking question only the human answers, so the loop takes none
>   of its steps. Two kinds of item are exempt: a fix for a regression on
>   `main` (FLOW-09) and a fix for a security problem.
> - **Limit.** FLOW-05's limit on features past Design that declare
>   `alters_relationships: true` is the value the human sets in the workflow
>   branch's `config.yaml`, which may be above 1. The limit of 5 features in
>   progress stands.
> - **Moves.** A program item whose change only moves code between packages,
>   modules or repositories, with no change in behaviour, says so in its
>   design. Where the move trips a path-coupling check, it carries a
>   `Tier1-Allow` trailer whose reason names this rule (LINT-05). Review
>   judges that the change is only a move.
> - **Overview.** A program item may leave FLOW-15's overview sweep to the
>   last item of its phase, which the phase's designs name.
>
> Nothing else is relaxed. GLOBAL-06, the BOOT rules, the relay's ledger and
> key rules (RELAY-01 to RELAY-03), FLOW-08, FLOW-14, FLOW-18 and FLOW-19
> apply to program items as written.
>
> The human ends the program with a process item that retires this rule.
>
> The agent part: intake and the design review judge whether an item is a
> program item or exempt.

**Severity and check:** new; MUST and agent-judgment. No enforcement change:
the freeze works through the existing blocking-question hold, the limit
through the existing `config.yaml` value, and suppression through the
existing trailer.

## Security note

**Boundary:** environment to relay, declared for the program as a whole. This
item's own PR changes rules and documentation only.

**How the posture changes across the program:**

- **Truststore generation moves into the relay** (S2, M5). The relay builds
  JKS and PKCS12 keystores from the CA certificate it already serves at
  `ca.pem`. They hold a public certificate and no key. The route is
  unauthenticated, as `ca.pem` is, and the keystore password stays the fixed
  public value Java tooling expects.
- **The relay is rewritten in Rust and gains release forms** (M1 to M6). Every
  change to the relay is high-severity under GLOBAL-06. The conformance suites
  from S1 are the check that the new relay behaves as the old one at its
  seams; RELAY-01 to RELAY-03 are not relaxed.
- **The unikernel runs on carried forks of Hermit** (Decision 15), including
  an entropy source that refuses to boot without a hardware random device.
- **The build-link contract** (S3) touches the host-to-environment boundary
  too. It weakens nothing: it states today's guarantee as behaviour, tests it
  on every driver, and names which drivers rest on host configuration.
- **Drivers from outside the project** (K1 onward) run with the host user's
  privileges like any driver. The kit cannot make an integrator's driver
  safe; the build-link suite and `drivertest` are what an integrator runs to
  show theirs is.

**What the TEST-08 suite should exercise:** for S2, that the served keystores
contain exactly the relay's CA and that the ledger records their bytes; for
S3, the contract's cases from inside the build on every driver (the relay
answers; the host, another build and the internet do not; nothing persists
between builds); for M6, the existing relay cases against each release form.

## Validation plan

This item: `make lint` (tier 1's rule index and design front-matter checks)
and `make test` on the branch. It changes no code, so there is no native
validation and no tier-2 gap.

The program: each seam's conformance suite is the validation of that seam,
run against the Go relay first and against each Rust release form as it
appears. `drivertest` and the build-link suite validate each driver. Every
program item's design states its own tiers and native hosts (FLOW-09); the
gates in the order table are the points at which a wave's validation must be
complete.

## Implementation plan

For this item's PR (process phase 2):

1. Add FLOW-20 to `docs/development/process.md` with the approved wording
   (FLOW-14, LINT-01).
2. `.claude/skills/workflow-intake/SKILL.md`: step 3 cites FLOW-20 for an
   item that is not a program item (FLOW-14: skills cite, not restate).
3. `AGENTS.md`: a row in "Where things are" pointing to this design.
4. Archive this design on the branch with its summary (FLOW-12), and open the
   PR.

After the merge, in an attended session and outside the PR: set
`limits.alters_relationships` to 3 in the workflow branch's `config.yaml`
(a5),
and file P1, S1 and K1 through intake.

## Design review

- **r1, 2026-11-12: approved at `530beda`** (Jeff; read at `612b837`, page
  https://claude.ai/artifact/DPiTkfmfoMxKmsggAGeJSd). The Bootstrap
  comparison accepted for all three model changes (BOOT-07), and the Security
  note acknowledged (GLOBAL-06). Assumptions a1, a3, a4, a5, a6 and a10
  confirmed as drafted; a12 confirmed with the comparison. Folded in at
  `530beda`, by Jeff's word in the sitting: a2, no storage boundary for the
  collector in this program; a7, the build-link class is not recorded in the
  ledger, because a driver may change class as its environment or
  implementation changes; a8, the held queue items are closed now, with
  `security-suite` included on a separate question that named its stale
  pointers; a9, contact with `hermit-os` waits for the end of M6; a11, an
  attended session may create repositories or change visibility on Jeff's
  word in that session.
- **rule change FLOW-20 approved at `530beda`**, as worded under **Rule
  changes**.

## Consolidation

To be filled in when the PR is prepared.

## Rollback notes

None.

## Archive summary

To be filled in when the PR is prepared.
