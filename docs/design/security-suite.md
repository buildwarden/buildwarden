---
feature: security-suite
status: draft
drivers: []              # the suite changes no driver tree; it runs against every driver (Cross-driver impact)
seams: []                # the case catalogue and the result report are suite-internal, not GLOBAL-08 seams
surfaces: []             # contributor-facing only: make targets and CONTRIBUTING.md (UX-01 still applies to those docs)
security_boundary: none  # tests the GLOBAL-06 boundaries, alters none; the suite adds no hook to a production path (Decision 9)
alters_relationships: false  # no component's role or interaction changes; the suite drives `warden build` as a user does
design_review: required  # rule_changes and assumed questions (FLOW-19)
native_validation:
  - {host: macos-vz, blocking: false, reason: "the suite changes no vz code; the first tier-2 security run on vz is a tracked gap"}
  - {host: qemu-any, blocking: false, reason: "the suite changes no qemu code; the first tier-2 security run on qemu (Linux and Windows guests) is a tracked gap"}
  - {host: windows-hyperv, blocking: false, reason: "the suite changes no hyperv code; the first tier-2 security run on Hyper-V is a tracked gap"}
tier2_gaps:
  - "macos-vz: first tier-2 run of the security cases (backlog security-suite-tier2-macos-vz)"
  - "qemu-any: first tier-2 run of the security cases, Linux and Windows guests (backlog security-suite-tier2-qemu-any)"
  - "windows-hyperv: first tier-2 run of the security cases (backlog security-suite-tier2-windows-hyperv)"
rules: [TEST-02, TEST-03, TEST-04, TEST-06, TEST-07, TEST-08, SEC-01, SEC-02, SEC-03, SEC-04, SEC-05, SEC-06, SEC-07, SEC-08, GLOBAL-06, LINT-02, LINT-03, RELAY-02, BOOT-02, UX-01, FLOW-02, FLOW-07, FLOW-09, FLOW-12, FLOW-14, FLOW-19]
rule_changes:
  - {rule: TEST-08, change: amend, paths: [docs/development/security-suite.md, docs/development/README.md, tools/tier1/rules.yaml]}
  - {rule: SEC-02, change: amend, paths: [tools/tier1/rules.yaml]}
  - {rule: SEC-06, change: amend, paths: [tools/tier1/rules.yaml]}
  - {rule: SEC-08, change: amend, paths: [tools/tier1/rules.yaml]}
concepts: [security-suite, security-testing, case-catalogue, arm-a-corpus, adversarial-agent, ledger-oracle, synthetic-surface, canary, security-report, release-gate, findings-disclosure, tiers, escape-vectors]
related: [vm-driver-release-plan.md, platform-compatibility.md, tier1-static-checks.md, build-link-subnet.md, firecracker-driver.md, ledger-environment-types.md, relay-leaf-crl.md, unified-bootstrap.md, windows-driver-plan.md, remote-execution-and-measured-hermeticity.md, http-body-path-drift.md]
---

# The TEST-08 security suite: structure, tiers and process (Deep-dive #1)

## Problem

TEST-08 requires a security suite for every release that trips GLOBAL-06, and
[`security-suite.md`](../development/security-suite.md) designs it as rules
(SEC-01..08): a deterministic corpus (Arm A) and an adversarial agent (Arm B),
gated as SEC-06 says. None of it exists. `tools/tier1/rules.yaml` points
TEST-08, SEC-02, SEC-06 and SEC-08 at "the security suite (security-suite
item)", and the release gate's step 3 in `behavioral-linter.md` is "hard-fail
once designed".

Meanwhile the features that crossed a boundary have each handed the suite a
list of what it "should exercise" (their Security notes, GLOBAL-06), and those
lists have no home. build-link-subnet's Consolidation handed its list over in
this item's queue history (2026-11-05), with cases that ran on no host. The
security property each one names is tested today, if at all, by tier-0 tests
scattered across packages and by native runs whose evidence lives in queue
notes, with nothing that says which property is covered where.

**Scope (q1, Jeff, 2026-11-07).** This design covers how the suite is
organised and run: where its code and cases live, how a case is declared, the
tiers and where each runs, how the handed-over cases are recorded, how results
are reported and kept, and the process items the suite needs. A case appears
here as a name and the property it checks. How any case's in-build half
behaves, and every attack technique, are outside this design; the first
unattended attempt at a design with that content was halted (queue history,
2026-11-01).

## Prior art

- **[`security-suite.md`](../development/security-suite.md) (SEC-01..08)** is
  the design of record; this feature builds it. Reused whole: the two classes
  across three boundaries (SEC-01) become required case metadata; the two arms
  (SEC-02); crystallization (SEC-03) becomes the rule that a regression case
  lands with its fix; the proof standard and the ledger as its own oracle
  (SEC-04) become the harness's one assertion shape; the synthetic surface
  (SEC-05) becomes the harness's canary; the gate (SEC-06) and cadence
  (SEC-08) map onto CI jobs and the release workflow. Its "Remaining build
  work" list is this design's scope, less the items that are process items
  (Decision 10).
- **[`vm-driver-release-plan.md`](vm-driver-release-plan.md), Security Testing
  Framework.** Reused: the split between an in-environment half and a
  host-side harness that boots the environment through the driver and judges
  the outcome from outside, and the per-host×driver×target scope. Departed
  from: it keeps a flat list of techniques and wants the attacker to be a
  separate black-box agent run as a CI gate on every driver or relay PR. Here
  the corpus is a catalogue of properties keyed to SEC-01 cells, the agent is
  Arm B on SEC-08's cadence (not per PR), and the plan's list is input to
  Arm B's brief (a process item), not restated.
- **[`platform-compatibility.md`](platform-compatibility.md)** makes "security
  validated" a graduation criterion for "Supported". The suite's coverage
  report (Decision 7) is what that criterion reads: a driver graduates when
  its blocking cases pass at tier 2 and Arm B has signed off.
- **[`tier1-static-checks.md`](tier1-static-checks.md)** assigned TEST-08,
  SEC-02, SEC-06 and SEC-08 to this item as owner. Reused: its `rules.yaml`
  owner convention, which this design updates to name the concrete
  enforcement (Rule changes).
- **[`ci-path-filters.md`](ci-path-filters.md)** and TEST-02's "where each tier
  runs": the suite's tier-1 and tier-2 jobs reuse the existing `integration`
  and `firecracker` workflows and their path filters rather than a new
  trigger.
- **Security notes that handed over cases** (GLOBAL-06, "what TEST-08 should
  exercise"): [`build-link-subnet.md`](build-link-subnet.md),
  [`firecracker-driver.md`](firecracker-driver.md),
  [`ledger-environment-types.md`](ledger-environment-types.md),
  [`relay-leaf-crl.md`](relay-leaf-crl.md),
  [`unified-bootstrap.md`](unified-bootstrap.md),
  [`windows-driver-plan.md`](windows-driver-plan.md) and the draft
  [`remote-execution-and-measured-hermeticity.md`](remote-execution-and-measured-hermeticity.md).
  [`http-body-path-drift.md`](http-body-path-drift.md) handed over nothing.
  Each becomes named cases with an `origin` (Design, "Initial catalogue").
  firecracker-driver also noted attacker positions SEC-01's matrix does not
  name (another local user, the operator); Decision 3 records them.
- **[`native-host-loop.md`](native-host-loop.md)** and FLOW-09: native
  sessions drain tier-2 work in batches and record outcomes on the `workflow`
  branch. Tier-2 security runs reuse that, through a process item (Decision
  10), and their first runs are tracked gaps.
- **`SECURITY.md`**: private reporting for vulnerabilities. Reused for Arm-B
  findings and failing tier-2 results (Decision 8).

## Decisions

1. **The suite lives in a top-level `security/` directory, with one
   catalogue.** `security/cases.yaml` lists every case; `security/` also holds
   the catalogue loader and its tier-0 checks, the harness for tiers 1 and 2,
   and the result report format. *Alternatives:* a directory per case with
   its own front-matter file (lost: invites prose that drifts into steps,
   and spreads the cross-case checks over hundreds of files); case metadata
   as Go struct literals in each test (lost: tier-0 cases live in many
   packages, and Arm B and the coverage report would have to compile Go to
   read the corpus); GitHub issues (lost: not versioned with the code, and
   public).

2. **Tier-0 cases stay beside the code they test.** The catalogue links a case
   to its tests by reference (`<package dir>:<TestName>`), and a tier-0 check
   fails when a reference does not resolve. *Alternative:* move every
   security-relevant unit test under `security/` (lost: tests would leave the
   package whose internals they use, and GLOBAL-04 couples tests to the
   change in that package).

3. **A case declares its cell, scope and gate; nothing about technique.**
   Required fields (Design, "Case declaration"): `id`, `intent` (one line, a
   property), `class` and `boundary` (the SEC-01 cell), `position` (the
   attacker's position), `rules`, `arm`, `tier`, `drivers` (and `guests` where
   it matters), `gate`, `status`, `tests`, `origin`. `position` defaults to
   the build environment, which is SEC-01's assumption; the other positions
   firecracker-driver found (a compromised relay, another local user, the
   operator, a crafted image) map to existing cells, so SEC-01's wording does
   not change (question a6).

4. **Tiers follow TEST-02.** Tier 0 is `make test` (every PR that is not
   docs-only, every platform). Tier 1 is the `integration` workflow with a
   container runtime. Tier 2 is platform/VM: on CI where a runner can host it
   (Firecracker in `firecracker.yml` on hosted KVM), in native sessions
   otherwise (vz, qemu, Hyper-V). Arm B is not a tier; it runs on its own
   isolated runner (SEC-07). *Alternative:* a separate tier scheme for the
   suite (lost: two meanings of "tier 2" in one repository).

5. **One assertion shape: the outcome, then the ledger.** Every tier-1/2 case
   ends in one of two verdicts the harness computes: the boundary held (the
   attempt failed observably), or the activity is in the ledger (SEC-04's
   oracle, against a fresh token from the harness's canary). A case passes
   when its declared expectation holds; anything else is a failure, never a
   retry (TEST-04). Each harness run includes the control case
   (`oracle-control-recorded`) so a broken oracle cannot pass everything.

6. **Gate per case: `blocking` or `tracked`.** A blocking case fails its job
   (CI) or its native step. A tracked case runs and is reported but does not
   fail the job; it carries a `reason`. Container cases are blocking when they
   hold today and tracked, with the reason, where the shared kernel makes the
   property best-effort (SEC-01: VM boundaries carry the suite's weight)
   (question a7). `status: planned` records a case with no test yet; the
   coverage report shows it as a gap.

7. **Results are a report per run, kept by tier.** The harness writes a JSON
   report (schema `security/report.schema.json`): the commit, the catalogue's
   digest, the host, the drivers and guests, and one result per case (`pass`,
   `fail`, `skip` with reason, `error`). Tier 0's results are the test log; a
   tier-0 test named in the catalogue is enough. Tier-1 and CI tier-2 reports
   are workflow artifacts. Native tier-2 reports go to the `workflow` branch
   under `reports/security/<host>/` (through the native-session process item).
   `go run ./security/cmd/coverage` renders the coverage report from the
   catalogue and any reports given: per SEC-01 cell and driver, which cases
   exist, at which tier, and their last result.

8. **What is public and what is private.** The catalogue is public and holds
   properties only. A public report carries a case id and its outcome, never
   logs that would describe an open hole: a failing tier-2 result keeps its
   logs in the private sessions record (FLOW-18) and is reported per
   `SECURITY.md`. An Arm-B finding is recorded as a draft GitHub security
   advisory, never in the catalogue or the queue, until it is fixed; its
   regression case lands with the fix, with `origin` naming the published
   advisory (SEC-03) (question a3).

9. **No test hooks in production paths.** The suite drives `warden build` and
   the collector as a user or a third-party orchestrator does, reads ledgers
   through `ledger/`, and runs its synthetic outside (SEC-05) host-side. A
   case that cannot be written without a hook in `relay/`, a driver or the
   executor goes back to Design (FLOW-06), because such a hook would change
   `security_boundary`.

10. **Arm B, its cadence, its runner and the native-session step are process
    items, listed here and not designed.** They live in `.claude/` and the
    `workflow` branch, which a feature may not change (FLOW-14). This feature
    gives them what they consume: the catalogue, the report format, the
    coverage report and the harness (Design, "Process items"). Until they
    land, SEC-06's Arm-B half of the gate is the human's sign-off at release,
    as SEC-06 already requires for "no finding" (question a10).

11. **The release workflow runs the suite.** `release.yml` gains a `security`
    job: tier 0 and the tier-1 cases, hard-fail on any blocking case. Tier-2
    results for the release and Arm B's sign-off are confirmed by the human
    in the release, from the coverage report (question a5). This closes the
    release gate's step 3 in `behavioral-linter.md` without editing it: the
    step already says "hard-fail once designed".

## Questions

### Blocking

None. q1 (the scope of this design) was answered by Jeff on 2026-11-07:
narrow it to the harness structure, front-matter, tiers and process items,
with no detailed attack techniques.

### Assumed

- **a1. One catalogue file.** Proceeding with `security/cases.yaml` as the
  single catalogue (Decision 1). Options: one file (recommended); a file per
  case; metadata in Go. Confirm.
- **a2. Tier-0 cases stay where they are.** Proceeding with references from
  the catalogue to existing tests, checked at tier 0 (Decision 2). Options:
  reference in place (recommended); move under `security/`. Confirm.
- **a3. Findings stay private until fixed.** Proceeding with draft GitHub
  security advisories for Arm-B findings and the private sessions record for
  failing tier-2 logs; the public catalogue and reports carry ids and
  outcomes only (Decision 8). Options: draft advisories (recommended); a
  private repository for findings; findings in the public catalogue marked
  open. Confirm.
- **a4. Native validation is tracked on every host.** The suite changes no
  driver, so FLOW-09 makes no host blocking; the first tier-2 run on
  macos-vz, qemu-any and windows-hyperv is a backlog gap, and Firecracker's
  runs in CI. Options: tracked everywhere (recommended); block on one native
  host to prove the harness off CI (qemu-any would be the cheapest). Confirm.
- **a5. The release gate's tier-2 and Arm-B halves are confirmed by the
  human.** Proceeding with a `security` job in `release.yml` for tiers 0 and
  1, and the human confirming tier-2 results and Arm B's sign-off from the
  coverage report (Decision 11). Options: human confirmation (recommended
  until native reports are routine); the release job reading the `workflow`
  branch's reports for the tagged commit and failing when a host has none.
  Confirm.
- **a6. Attacker position is metadata, SEC-01 unchanged.** Proceeding with a
  `position` field mapped to SEC-01's cells (Decision 3). Options: metadata
  only (recommended); amend SEC-01 to name the positions (a rule change this
  design would then declare). Confirm.
- **a7. Container cases.** Proceeding with container cases blocking where they
  hold today and tracked with a reason where the shared kernel makes them
  best-effort (Decision 6). Options: as stated (recommended); all container
  cases tracked. Confirm.
- **a8. Remote-execution cases are recorded as planned.** The draft
  remote-execution design's cases enter the catalogue as `status: planned`,
  with no test until that feature exists. Options: record them now
  (recommended, so the hand-off is not lost); leave them in that design.
  Confirm.
- **a9. Implement writes the in-build halves.** The tier-1/2 cases' in-build
  halves are benign probes against the harness's own canary (SEC-04,
  SEC-05), written in Implement from each case's intent. If an unattended
  Implement step is halted as f1-design was, the step is released for an
  attended session, not narrowed further. Options: as stated (recommended);
  plan the in-build halves attended from the start. Confirm.
- **a10. Process items are filed after this review.** The four process items
  (Design, "Process items") are filed with `workflow-intake` after this
  review approves the design, so they build on the approved catalogue and
  report format; this feature does not wait for them. Options: file after
  the review (recommended); file now; fold them into this feature (not
  allowed: `.claude/` changes only through a process item, FLOW-14).
  Confirm.
- **a11. Rule changes.** Proceeding with the four amendments in Rule changes:
  `rules.yaml` owners name the concrete enforcement, and
  `security-suite.md`'s "Remaining build work" and the README's Deep-dive #1
  entry say what was built and what remains. Confirm each by name.

## Design

### Layout

```
security/
  cases.yaml              the catalogue: every case, Arm A and planned
  catalogue.go            loader and the field types and enums below
  catalogue_test.go       tier 0: the catalogue's own checks (below)
  report.schema.json      the per-run result report (Decision 7)
  report/                 writer and reader for the report
  harness/                tier 1/2: build-tagged `security`; boots builds via
                          dist/warden, runs the synthetic outside, applies the
                          oracle, writes the report
  cases/<id>/             a tier-1/2 case's in-build half (Implement, a9)
  cmd/coverage/           renders the coverage report
```

`make test` runs `security/catalogue_test.go` with every other tier-0 test.
`make security-test` runs the harness for the tier-1 and tier-2 cases the
host can run (a case whose driver is absent is `skip` with the reason), and
takes `DRIVER=` to narrow it. CONTRIBUTING.md and
`docs/book/src/contributing/structure.md` gain both (UX-01).

### Case declaration

One entry per case in `security/cases.yaml`:

| Field | Values | Meaning |
|---|---|---|
| `id` | kebab-case, unique, never reused | the case's name in reports and history |
| `intent` | one line | the property checked, e.g. "a guest cannot reach a host address outside the relay"; never steps |
| `class` | `1` escape, `2` evasion | SEC-01 class |
| `boundary` | `host-env`, `env-relay`, `env-beyond` | SEC-01/GLOBAL-06 boundary |
| `position` | `build-env` (default), `relay`, `local-user`, `operator`, `image` | where the attacker is assumed to be (Decision 3) |
| `rules` | rule IDs | the SEC rule the case serves and the product rules whose property it checks (e.g. `RELAY-02`, `BOOT-02`) |
| `arm` | `A` | Arm-B findings enter as Arm A once fixed (SEC-03) |
| `tier` | `0`, `1`, `2`; a list when the property is checked at several | TEST-02 tier |
| `drivers` | `container`, `qemu`, `vz`, `hyperv`, `firecracker`, or `interface` | where the property must hold; `guests` (`linux`, `windows`, `macos`) narrows it |
| `gate` | `blocking`, `tracked` (+ `reason`) | Decision 6 |
| `status` | `implemented`, `planned` | `planned` is a coverage gap |
| `tests` | `<dir>:<TestName>` (tier 0), `security/cases/<id>` (tier 1/2) | where it is checked |
| `origin` | a design path, `SEC-03:<advisory>`, or `suite` | who handed the case over |

The tier-0 catalogue test checks: the schema and enums; unique ids; every
`implemented` case has `tests` and each reference resolves (the package dir
exists and declares that test function, read with `go/parser`; a tier-1/2
case's directory exists); every `origin` design exists; every `tracked` case
has a reason; and every SEC-01 cell for the VM drivers has at least one case,
`planned` included, so a gap is visible rather than silent. Retiring a case
keeps its id in a `retired:` list, so ids are never reused.

### Tiers and where they run

| Tier | What | Runs | Gate |
|---|---|---|---|
| 0 | the catalogue's checks; the linked unit, fuzz and golden tests (validation, refusal, parsing, signing) | `make test`: `ci` workflow, every non-docs-only PR and push to `main`, all platforms | blocking cases fail the PR |
| 1 | cases needing a real build on the container driver: harness + canary + oracle | `integration` workflow (its path filters gain `security/**`), weekly on `main`, release | blocking cases fail the job |
| 2 (CI) | Firecracker cases | `firecracker.yml` (path filters gain `security/**`) | as tier 1 |
| 2 (native) | vz, qemu (Linux and Windows guests), Hyper-V cases | native sessions, through process item P3; pre-release full matrix | blocking cases fail the native step; until P3, a backlog gap |
| Arm B | the adversarial agent | an isolated disposable runner (SEC-07), monthly and on demand (SEC-08) | a confirmed finding blocks the release (SEC-06); "no finding" is human-signed |

SEC-08's "per-PR on boundary-touching changes" is met by tier 0 on every PR
and tiers 1 and 2 on the paths the `integration` and `firecracker` workflows
already watch, which are the boundary code (relay, collector, ledger,
relaycfg, the drivers' shared paths).

### The harness (tiers 1 and 2)

For each selected case: start the synthetic outside (a host-side canary that
serves and expects a fresh per-run token, SEC-05), run a build through
`dist/warden build` with the case's in-build half as the build, then judge:
the expected verdict (Decision 5), read from the build's outcome, the canary's
log and the build's ledger (verified through `ledger/`, RELAY-02). Builds are
grouped per driver so one environment serves many cases where the cases allow
it (TEST-07). A new surface for the canary is a human-reviewed change to the
harness (SEC-05), never something a case adds for itself.

### Initial catalogue

The cases handed over so far, as names and the property each checks. Tier and
driver are what the catalogue records; `T0` names an existing tier-0 test
file that the entry links. Plan fixes every field.

*From build-link-subnet (queue history 2026-11-05, Security note):*

| id | intent | tier | drivers |
|---|---|---|---|
| link-claim-overlap-refused | a claim whose build link overlaps the relay's upstream interface is refused and leaves the relay unclaimed | 0 (`relaycfg/buildlink_test.go`, `cmd/relay/vmlink_test.go`) | interface |
| link-claim-invalid-refused | a claim whose build link fails validation is refused and leaves the relay unclaimed | 0 (`relaycfg/buildlink_test.go`) | interface |
| relay-second-claim-refused | a relay accepts exactly one claim | 0 (`relaycfg/`) | interface |
| link-addresses-blocked | a build cannot resolve or reach the build link's own addresses through the relay, for the default and a configured link | 0 (`relay/buildlink_test.go`), 2 | qemu, firecracker, hyperv, vz |
| dhcp-vm-one-offer | in vm mode the relay's DHCP responder serves only the build interface and keeps one fixed offer per build, whatever the guest sends | 0 (`cmd/relay/dhcp_test.go`, `FuzzDHCPReply`), 2 | qemu, firecracker, hyperv |
| dhcp-vm-no-upstream-offer | the relay makes no DHCP offer on its upstream side | 2 | qemu, firecracker, hyperv |
| dhcp-host-one-offer | in host mode the guest cannot take an address other than its offer, or the relay's | 2 | vz |
| guest-source-dropped | traffic carrying the guest's address that leaves the host without the relay is dropped | 2 | firecracker |
| link-host-resolver-refused | a host resolver inside the link is refused before any VM boots | 0 | firecracker |
| link-overlap-refused-before-boot | a configured link overlapping a BuildWarden range or a host interface is refused before any VM boots | 0 (`driver/buildlink`) | qemu, firecracker, hyperv, vz |
| stale-launcher-refused | a launcher built for another link is refused before any VM boots | 0 (`driver/hostlink_test.go`) | qemu, firecracker, hyperv, vz |

*From firecracker-driver (Security note, TEST-08 table):*

| id | intent | position | tier | drivers |
|---|---|---|---|---|
| guest-no-host-address | a guest cannot reach any host address except through the relay, over IPv4 or IPv6 | build-env | 2 | firecracker; all VM drivers once their runs exist |
| proxy-refuses-host-destinations | the relay's proxy refuses destinations on the host, its networks and other builds' relays | build-env | 0 (`relay/`), 2 | interface |
| console-no-host-input | the guest's console carries no input from the host | build-env | 0, 2 | firecracker |
| vmm-blast-radius | what the VMM process can open matches the blast radius the driver documents | build-env | 2 | firecracker |
| slot-network-exclusive | another local user cannot attach to a held build slot's network | local-user | 2 | firecracker |
| relay-ports-closed-local | a running relay's guest-facing ports are unreachable from other local users and processes; only the claim port answers, already claimed | local-user | 2 | firecracker |
| orphan-recovery | after warden is killed mid-build, the next build on that slot cleans up and succeeds | operator | 2 | firecracker |
| image-no-host-file | a custom image that refers to a host file is refused at conversion and no host bytes reach the rootfs | image | 0, 2 | qemu, firecracker |
| relay-per-build-identity | concurrent builds get distinct CA and signing keys and ledgers with no cross-entries | build-env | 2 | firecracker; all VM drivers |
| relay-no-host-connect | a compromised relay cannot open a connection to the host | relay | 2 | firecracker |
| relay-no-spoofed-egress | a compromised relay cannot send with a source address that is not its own | relay | 2 | firecracker |
| slot-state-isolated | concurrent builds never share connection state, and each ledger is complete | build-env | 2 | firecracker |
| build-death-no-complete-ledger | a build that dies early fails promptly and no partial ledger is accepted as complete | build-env | 1, 2 | interface |

*Suite-wide (SEC-01, SEC-04; also windows-driver-plan's and unified-bootstrap's
egress cases):*

| id | intent | tier | drivers |
|---|---|---|---|
| egress-only-through-relay | a build reaches the synthetic outside only through the relay; the ledger oracle confirms no other path delivered the token | 1, 2 | container, qemu, firecracker, hyperv, vz; Linux and Windows guests |
| oracle-control-recorded | an HTTPS fetch of the canary through the relay is in the ledger (the oracle's control case, run with every harness run) | 1, 2 | all |
| outputs-stay-contained | a build's output and context paths cannot place files outside their directory | 0 (`collector/store_test.go`) | interface |

*From ledger-environment-types, relay-leaf-crl, unified-bootstrap and
windows-driver-plan:*

| id | intent | tier | drivers |
|---|---|---|---|
| claim-type-mismatch-refused | a claim whose declared environment type disagrees with its payload is refused | 0 | interface |
| environment-record-tamper-detected | a tampered environment record fails `inspect` | 0 | interface |
| crl-response-fixed | the CRL response does not depend on the request | 0 | interface |
| crl-path-not-upload | the CRL path cannot be used to store an artifact | 0 | interface |
| crl-verifies-against-ca | the CRL verifies against the served CA and no other key | 0 | interface |
| ca-signs-only-leaves | no request makes the relay sign anything but leaf certificates and the CRL with the CA key | 0 | interface |
| entrypoint-no-egress | an image's entrypoint has no path out but the relay | 1 | container |
| container-network-immutable | the build container cannot change its network rules | 1 | container |
| support-image-pinned | a substituted image under a support image's tag is not run | 0, 1 | container |
| bootstrap-config-strict | a bootstrap config with unknown keys or path-like values is rejected | 0 | interface |
| ca-not-persisted | the per-build CA does not persist into a prepared image or a later build | 2 | hyperv, qemu (Windows guests), vz |
| seed-carries-only-bootstrap | the per-build seed carries nothing beyond the executor, launcher and network configuration (BOOT-02) | 0, 2 | hyperv, qemu |

*From remote-execution-and-measured-hermeticity (draft; `status: planned`,
a8):* `subledger-verified-before-use` (a sub-ledger with a bad signature,
digest or key is rejected, not incorporated), `hermetic-recomputed` (the
`hermetic` flag comes from the verified sub-ledger, never storage),
`dedup-cannot-hide` (deduplication cannot hide a record whose content
differs), `network-actions-through-relay` (an action that needs the network
reaches it only through a relay), `cas-digest-enforced` (a blob that differs
from its digest is refused), `remote-api-authenticated` (an unauthenticated
Execute or cache write is refused).

`vm-driver-release-plan.md`'s technique list and the categories in SEC-01's
matrix that no case above covers are not restated: they are input to Arm B's
brief (P1), and each confirmed finding becomes a case here (SEC-03).

### Process items (FLOW-14; listed, not designed)

- **P1. The pen-tester agent (Arm B).** An agent definition under
  `.claude/agents/` on the largest model at highest effort (SEC-08), briefed
  by SEC-01..07 and the coverage report's gaps; it writes results in this
  design's report format and files findings as draft advisories (Decision 8).
- **P2. Arm B's cadence.** The monthly scheduled task and the on-demand
  trigger before a major platform or boundary update (SEC-08), and the
  release sign-off step for Arm B's "no finding" (SEC-06), in the
  `workflow-*` skills.
- **P3. Tier-2 security runs in native sessions.** `workflow-native` runs
  `make security-test` for its hosts on `main` and waiting branches, writes
  reports to `reports/security/<host>/`, and drains this design's backlog
  gaps.
- **P4. Arm B's isolated runner (SEC-07).** A disposable, ephemeral host at
  least as isolated as production, with a host name in `hosts.yaml`; Jeff
  provisions it, so the item may need a human step.

## Cross-driver impact

No driver's code changes (Decision 9); `drivers` is empty for that reason.
Every driver is a target:

- **container**: tier-1 cases in the `integration` workflow; best-effort
  cases are `tracked` (Decision 6).
- **firecracker**: tier-2 cases in `firecracker.yml` on hosted KVM.
- **qemu** (Linux and Windows guests), **vz**, **hyperv**: tier-2 cases in
  native sessions once P3 lands; their first runs are the tracked gaps in
  `tier2_gaps` and the native-gap backlog.

The harness drives each through `warden build`, so a driver needs nothing
suite-specific; a case that finds a driver needs a hook goes back to Design
(Decision 9).

## Rule changes

Each is made in Implement after the review approves it by name (FLOW-14).

### TEST-08 (amend): the suite's status in the standards, and its owner

- **Paths:** `docs/development/security-suite.md`,
  `docs/development/README.md`, `tools/tier1/rules.yaml`.
- **Current:** `security-suite.md` ends with "Remaining build work: This
  document is the design. Implementation is separate:" followed by four
  bullets (the harness; the Arm-A corpus and its wiring; the Arm-B agent and
  the synthetic surface; the cadence). The README's Deep-dive #1 entry says
  "Remaining build work: the isolated harness, the Arm-A corpus, the Arm-B
  agent wiring, and the monthly/on-demand cadence." `rules.yaml`:
  `TEST-08: {owner: "the security suite (security-suite item)"}`.
- **Proposed:** `security-suite.md`'s section becomes "Where it is built":
  "Arm A, its catalogue, harness, result reports and CI and release wiring are
  in `security/` (design: `docs/design/security-suite.md`). Remaining: the
  Arm-B agent, its cadence and isolated runner, and tier-2 runs in native
  sessions, each a process item (FLOW-14)." The README entry keeps its
  heading and says the same in one sentence, naming the process items as
  open. `rules.yaml`: `TEST-08: {owner: "security/: the case catalogue's
  tier-0 checks (make test), make security-test in the integration,
  firecracker and release workflows"}`.
- **Severity and check:** unchanged (MUST; automated + agent-judgment). No
  rule text of TEST-08 or SEC-01..08 changes.
- **Why:** after this feature the standards would describe built work as
  missing, and the owner would name an item that is done.
- **Enforcement:** none changes. `rules.yaml`'s owner field is read by the
  rule-index self-check only for presence.

### SEC-02, SEC-06, SEC-08 (amend): owners name the enforcement

- **Path:** `tools/tier1/rules.yaml`.
- **Current:** each `{owner: "the security suite (security-suite item)"}`.
- **Proposed:** SEC-02: `{owner: "security/ (Arm A: the catalogue and make
  security-test); Arm B: the pen-tester agent process item"}`. SEC-06:
  `{owner: "the blocking cases in the ci, integration, firecracker and
  release workflows; Arm B's sign-off is human (LINT-03)"}`. SEC-08:
  `{owner: "the integration and firecracker workflows' path filters and the
  release workflow (Arm A); the Arm-B cadence process item"}`.
- **Severity and check:** unchanged (MUST; SEC-02 automated + agent-judgment,
  SEC-06 and SEC-08 automated).
- **Why:** tier 2 of the behavioral linter reads owners to know what enforces
  a rule; "the security-suite item" stops meaning anything once it is done.
- **Enforcement:** none changes.

## Security note

`security_boundary` is `none`: the suite tests every GLOBAL-06 boundary and
alters none, because it adds nothing to a production path (Decision 9). Two
things it does add need care. Its records could disclose an open hole, so
public records carry properties, ids and outcomes only, and findings stay
private until fixed (Decision 8). And Arm B runs real attempts, so it runs
only on the isolated runner of P4 (SEC-07), never on CI or a native machine.
SEC's honest-coverage limit stands: the coverage report shows which cells
have cases, not that a cell has no hole.

## Validation plan

- **Tier 0:** the catalogue test (schema, references, origins, cell coverage)
  against the real catalogue and against fixtures with each defect; the
  report writer and reader round-trip against the schema; the coverage
  renderer on golden fixtures; the linked tier-0 cases run in `make test` as
  today. All pure Go, every platform (TEST-01).
- **Tier 1:** `make security-test` in the `integration` workflow: the
  container cases, the oracle control case, and a harness self-test (a case
  rigged to pass and one rigged to fail must be reported as such, so a
  harness that passes everything is caught).
- **Tier 2 (CI):** the Firecracker cases in `firecracker.yml`.
- **Tier 2 (native):** tracked, not blocking (a4): macos-vz, qemu-any and
  windows-hyperv each get a native-gap backlog entry for the first run,
  drained once P3 lands or by hand in an attended native session.
- **Deferred:** Arm B in full (P1, P2, P4); remote-execution cases (a8).

## Implementation plan

Plan writes the ordered tasks from the approved design. In outline:

1. `security/catalogue.go` and `cases.yaml` with the initial catalogue, every
   reference resolved or the case `planned` (SEC-01, SEC-03, TEST-08).
2. The tier-0 catalogue test (TEST-01, TEST-04).
3. `report.schema.json`, `security/report/` and the coverage command
   (SEC-06, TEST-08).
4. The harness: canary, build driving, oracle, report; the self-test
   (SEC-04, SEC-05, TEST-04, TEST-07).
5. The in-build halves of the tier-1 and tier-2 cases (SEC-04; a9).
6. CI: `security/**` in the `integration` and `firecracker` path filters;
   `make security-test` in both; the `security` job in `release.yml`
   (SEC-06, SEC-08, TEST-02).
7. Docs: CONTRIBUTING.md and `contributing/structure.md` (UX-01).
8. The approved rule changes (FLOW-14).

## Design review

## Consolidation

## Rollback notes

## Archive summary
