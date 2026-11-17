---
feature: relay-collector-failure-fails-open
status: draft
drivers: [container, qemu, vz, hyperv, firecracker]
                         # relay/ is shared (FLOW-02, TEST-02 caveat): every driver's build is
                         # served by the changed handlers; no driver tree changes
seams: [relay-guest-api, bootstrap-config]
                         # relay-guest-api: GUEST-25, GUEST-26 (and the paragraph after it),
                         # GUEST-34 and the Errors table change: a collector failure is 502,
                         # not 404. bootstrap-config: BOOTCFG-3 changes the same way.
                         # collector-inputs and executor-initialize are implemented unchanged
                         # (Specification changes)
surfaces: []             # no warden flag, key or output file changes; the build's own console
                         # shows a different reply (HTTP 502 for HTTP 404) only when the
                         # collector fails
repos: [buildwarden/buildwarden, buildwarden/buildwarden-specs]
                         # home first; the two specifications and their suites change in the
                         # second (LOOP-02, TEST-05), and this one pins their new releases
areas: [relay, executor, specs, book, designs]
                         # relay: relay/relay.go and its tests; executor: tests only, under
                         # cmd/warden-io (no seam code changes there, Decision 5); specs: the
                         # two pin modules under tools/conformance/; book: overview.md's
                         # pseudocode (FLOW-15) and the refs make pin-specs rewrites in
                         # concepts/seams.md; designs: this document
security_boundary: env-relay  # GLOBAL-06: any change to the relay itself; here the relay stops
                         # turning a collector failure into "no bootstrap config", which
                         # downgraded exclusive trust to append (Security note)
alters_relationships: false  # no component's role changes and no one talks to anyone new: the
                         # relay still serves the same three inputs from the same collector;
                         # one reply for one failure case changes, inside the relay's role
                         # (question a3)
design_review: required  # FLOW-19: seams, a security boundary and assumed questions rule out skip
native_validation: []    # platform-neutral relay code, run by tier 0 and by the conformance
                         # suites against the relay in host, vm and container mode; the
                         # executor's branch is shared Go on every guest OS (Validation plan)
tier2_gaps: []
rules: [GLOBAL-06, GLOBAL-08, GLOBAL-09, GLOBAL-10, GLOBAL-11, RELAY-02, RELAY-04, RELAY-08,
        EXEC-04, UX-04, TEST-01, TEST-02, TEST-05, TEST-08, FLOW-01, FLOW-02, FLOW-05, FLOW-07,
        FLOW-09, FLOW-12, FLOW-14, FLOW-15, FLOW-19, FLOW-20, LOOP-02, LOOP-04, PROFILE-01]
rule_changes: []
concepts: [relay, collector, relay-served-inputs, bootstrap-config, trust-mode, fail-closed,
           relay-guest-api, specs-release, specs-pin, security-fix]
related: [seam-specs.md, unified-bootstrap.md, relay-leaf-crl.md, http-body-path-drift.md]
---

# A collector failure is served as a failure, not as "none" (fails closed)

## Problem

The relay serves three inputs to the build from its collector:
`http://artifacts/build-script`, `http://artifacts/bootstrap` and
`http://cwd/<path>`. Each handler in `relay/relay.go` asks the collector
(`GET /v1/build-script`, `/v1/bootstrap`, `/v1/context/<path>`) and, when the
reply is anything but a clean `200`, answers the build `404`:

- `handleBuildScript` (`relay/relay.go:719-726`): `err != nil || status != 200`
  gives `404 build script unavailable`;
- `handleBootstrap` (`:732-738`): the same gives `404 no bootstrap config`. Its
  comment (`:728-731`) says "A collector without one gives 404, which the
  executor reads as the empty config": true for a collector that has none,
  but the code does the same for a collector that fails;
- `handleContextGet` (`:751-755`): the same gives `404 not found: <path>`.

A `404` means "the collector has none". The executor reads a `404` from
`/bootstrap` as the empty config, whose trust mode is `append` (BOOTCFG-10,
`cmd/warden-io/initialize.go` `fetchBootstrap`). So when a producer has set
`{"trust": {"mode": "exclusive"}}` and the collector fails to serve it (a
`500`, a reset stream, a tunnel that drops, a body cut short), the build runs
with the system's roots trusted besides the relay's. The provided value failed
and the default was used in its place: GLOBAL-10's "the default is not tried
when the provided value fails", and GLOBAL-11's "when an operation fails, it
stops". The failure is silent: the build log says `trust append`, as for a
build that asked for nothing.

The other two inputs fail closed today only by luck of their readers: the
executor stops on any non-`200` for the build script and for `warden-io
fetch`. A build script that reads `http://cwd/<path>` itself and treats `404`
as "absent" (an optional config file, a lock file) is misled the same way.

This is divergence N24 of `seam-specs` (its table "Found during Implement: N1
to N33 (r2)"), run in its stage 8e against `relay-host`, `relay-vm` and
`relay-container`: a collector's `500` gives the build `404`. The written
specifications at `v0.1.0` state that behaviour as required (BOOTCFG-3) or
leave it undefined (the paragraph after GUEST-26, the sentence after GUEST-34),
so the fix is spec-first (LOOP-02).

**Why now.** Jeff filed it on 2026-11-16 in `seam-specs`' r2 design review as
a security fix exempt from the FLOW-20 freeze. It is not marked urgent; `wf
urgent` is Jeff's (FLOW-05).

**FLOW-20.** This item is not a program item: its `related` does not list
`componentization.md`. It is exempt from the freeze as "a fix for a security
problem" (FLOW-20; `componentization` a4): a build that asked for exclusive
trust gets append trust when the collector fails, which fails open across the
env-relay boundary's trust posture. Nothing FLOW-20 relaxes for program items
(moves, the overview sweep) is taken here; GLOBAL-06, RELAY-01 to RELAY-03,
FLOW-08, FLOW-14, FLOW-18 and FLOW-19 apply as written.

## Prior art

- [`seam-specs`](seam-specs.md) (archived): N24's row, its Security note
  ("a failing collector turns a producer's `exclusive` into `append`"), and its
  **Defaults and overrides**, which names this as a fallback of today's code
  that the design "neither adds nor allows", a row of class (c). It also set
  how a specification changes: in `buildwarden/buildwarden-specs`, released as
  `<specification>/vX.Y.Z`, and pinned here by `make pin-specs` in
  `tools/conformance/<specification>/go.mod` (its Decisions 4 and 7). Reused
  as is.
- [`unified-bootstrap`](unified-bootstrap.md) (archived): the bootstrap config
  and its `404 => empty config` reading, which stays. This item does not
  change what a `404` means; it stops the relay from sending one that is not
  true.
- [`relay-leaf-crl`](relay-leaf-crl.md) and
  [`http-body-path-drift`](http-body-path-drift.md) (archived): the precedent
  for a change inside the relay's existing role that touches a seam:
  `alters_relationships: false`, every driver declared because `relay/` is
  shared, `security_boundary: env-relay`.
- The relay's own `502` (GUEST-44 to GUEST-46): the reply the relay already
  gives when the upstream it proxies to fails. The collector is the upstream
  of the three inputs, so the same status reads the same way (Decision 1).
- GUEST-29: the relay's `500 storage error` when the collector does not store
  an artifact. Considered as the status to reuse and not taken (Decision 1).
- Rules relied on: GLOBAL-10 and GLOBAL-11 (the defect), GLOBAL-06 (the note),
  GLOBAL-08 and TEST-05 (the seam change, the pin, the suites), LOOP-02 (spec
  first), RELAY-02 (what the build receives is what is recorded), RELAY-08 (no
  relay setting is added), UX-04 (the failure names itself), FLOW-15 (the
  overview).

No archived design overlaps beyond these (searched `concepts`, `seams` and
`rules` for `relay-guest-api`, `bootstrap-config`, `collector`,
`relay-served-inputs` and GLOBAL-10).

## Decisions

1. **A collector failure is `502 Bad Gateway`.** For each of the three inputs,
   when the collector's reply is not `200` and not `404`, when the request to
   it fails, or when its `200` body cannot be read in full, the relay answers
   `502` with a short body that names the input and the reason, and records
   nothing. A `404` from the collector stays `404` (the collector has none).
   - *500 Internal Server Error*, as GUEST-29's storage error: lost because
     `500` says the relay itself failed, and the relay is the gateway here,
     as it is for GUEST-46's proxy `502`.
   - *503 Service Unavailable*: lost; it invites a retry, and this item adds
     no retry (GLOBAL-11).
   - *Pass the collector's status through* (`500` as `500`, `403` as `403`):
     lost; it gives the build a status vocabulary the relay does not control
     and makes "unreachable" need a status of its own anyway.
   - *Keep `404` and add a header saying it is a failure*: lost; every reader
     that does not know the header still fails open, which is the defect.
   Question a1.
2. **On `cwd`, only the collector's `404` is "not there".** Today any
   non-`200` is `404`, and GUEST-34 states it so. After the change: `404` from
   the collector is `404 not found: <path>`; any other status, `403`
   included, is `502`. The relay already refuses an unsafe path itself with
   `403` before it asks (GUEST-33), so a collector's `403` means the two
   disagree on the path rule, which is a failure, not an absence. Question a2.
3. **Nothing is recorded for a `502`.** As for today's `404`: the ledger
   records served inputs (GUEST-49), and a `502` serves none. RELAY-02 holds:
   the build received no input, and none is recorded.
4. **The body is fixed text, the detail goes to the relay's log.** The Go
   relay's body is `collector failed: <input>: <reason>` and a line feed,
   where `<input>` is `build script`, `bootstrap config` or the context path,
   and `<reason>` is one of `HTTP <n>`, `unreachable` or `read failed`. The
   Go error (which can carry the collector's address or the tunnel's state)
   is logged by the relay, not sent to the build. The body is the Go relay's,
   not a requirement, as the other bodies of the Errors table are.
5. **The executor's code does not change.** BOOTCFG-11 already has it stop on
   any status but `200` and `404`, and `fetchBuildScript` and `warden-io
   fetch` stop on any non-`200`. The executor's error reads `bootstrap
   config: HTTP 502`, which with the relay's log names the failure (UX-04).
   Adding the relay's body to that error was considered and not taken: it
   edits `cmd/warden-io/initialize.go`, which is in the
   `executor-initialize` seam's code list, and so would move that seam's pin
   for no change in its specification. The executor gains tests only: that a
   `502` from `/bootstrap` stops setup before any CA is installed, and that a
   `502` from `/build-script` stops the build (question a5).
6. **No retry and no fallback.** The relay asks the collector once per
   request, as today. A bounded retry of the same request would be allowed by
   GLOBAL-11 but is not needed to fix the defect, and is not added.
7. **Spec first, two new releases.** `relay-guest-api` and `bootstrap-config`
   change in `buildwarden/buildwarden-specs`, with their cases, and are
   released as `relay-guest-api/v0.2.0` and `bootstrap-config/v0.2.0` (an
   incompatible change while the version is `v0.x.y`: a relay that meets
   `v0.1.0`'s BOOTCFG-3 fails `v0.2.0`'s). This repository moves those two
   pins with `make pin-specs` and nothing else. Question a4.

## Questions

### Blocking

None.

### Assumed

- **a1. The status for a collector failure.** Proceeding with `502` for all
  three inputs; confirm. Options: (a) `502` (recommended: the relay is the
  gateway to the collector for these inputs, as it is to the upstream in
  GUEST-46); (b) `500`, as GUEST-29's storage error; (c) `503`; (d) the
  collector's own status passed through, with `502` for unreachable.
- **a2. What counts as "not there" on `cwd`.** Proceeding with: only the
  collector's `404` is `404`; any other non-`200`, its `403` included, is
  `502`; confirm. Options: (a) as proceeding (recommended: one meaning per
  status, and the relay already refuses unsafe paths itself); (b) pass the
  collector's `403` through as `403` and make every other failure `502`;
  (c) leave `cwd` as it is and fix only `build-script` and `bootstrap`, whose
  readers are the executor's (lost: a build script's own reads of `cwd` fail
  open the same way, and N24 names all three).
- **a3. `alters_relationships`.** Proceeding with `false`; confirm. FLOW-02
  lists "a versioned seam" among what alters relationships. The seam's
  requirements change here, but no component's role or who talks to whom
  changes: the relay serves the same three inputs from the same collector,
  and one reply for one failure changes. `relay-leaf-crl` added an endpoint to
  the same seam under `false`. Options: (a) `false` (recommended); (b) `true`,
  conservatively, which takes an `alters_relationships` slot at Plan's claim
  (limit 3, `config.yaml`).
- **a4. The release numbers.** Proceeding with `relay-guest-api/v0.2.0` and
  `bootstrap-config/v0.2.0`; confirm. Options: (a) `v0.2.0` for both
  (recommended: a requirement a `v0.1.0` relay meets is now failed, which is
  an incompatible change, and the `v0.x.y` documents are edited in place
  under their **Versioning**); (b) `v0.1.1` for both, as a fix of a defect the
  first release wrote down; (c) wait and fold the change into the next
  release another item makes (lost: it holds a security fix for an
  unscheduled release). `collector-inputs`, `collector-outputs`,
  `relay-claim`, `executor-initialize` and the harness `conformance` are not
  released by this item.
- **a5. No executor code change.** Proceeding with tests only under
  `cmd/warden-io/`, so that the `executor-initialize` pin does not move;
  confirm. Options: (a) tests only (recommended, Decision 5); (b) also print
  the relay's reply body in the executor's error, which edits
  `initialize.go` and `main.go`, both in the `executor-initialize` seam's
  code list, and so needs a release of that specification with no change in
  it, or a `Tier1-Allow` trailer.
- **a6. The order with `driver-contract` (K1) and `workdir-in-context`.**
  Proceeding with no order between them beyond FLOW-05's; confirm. K1's queue
  item declares no areas, so it collides with every item in this repository
  (FLOW-05), this one included, although its design says that "no file in
  `relay/`, `cmd/relay`, `relaycfg/`, `collector/` or `cmd/warden-io`
  changes" and that no protocol of seams 1, 2, 3, 5 or 6 changes. K1's
  Design r3 also amends TEST-05 and PROFILE-01 to pin one release of the
  specs repository in `tools/conformance/go.mod`, where `main` pins each
  specification in `tools/conformance/<specification>/go.mod`. Whichever
  merges second reconciles the pin in its refresh (FLOW-10): if K1 lands
  first, this item's two releases are pinned through K1's single module.
  `workdir-in-context` (areas `driver-core`, `cli`, `book`, `designs`) is
  near this item: `driver-core` reaches `relay` and `executor`, and both edit
  `book` (ordered). It changes what the context serves; this item, how a
  failure to serve it is reported; the two do not touch the same code.
  Options: (a) as proceeding (recommended: this item is small, ahead of K1 in
  priority, 78 against 72, and a security fix; FLOW-05's PR slot keeps the
  PRs apart); (b) wait for K1; (c) ask Jeff to name a `collides_with` with
  `workdir-in-context` (not needed: they are near, and the PR slot covers
  near).
- **a7. The `relay-guest-api` seam's code list.** Proceeding without changing
  it; confirm. `tools/tier1/rules.yaml` lists `relay/controlplane.go`,
  `relay/heartbeat.go`, `relay/dns.go` and `cmd/relay/dhcp.go` for the seam,
  not `relay/relay.go`, which holds every `artifacts` and `cwd` handler, so
  the seam-contract check does not see this change. This item moves the pin
  anyway (Decision 7). Options: (a) leave the list; Jeff files adding
  `relay/relay.go` as its own item if he wants it (recommended: it is a
  GLOBAL-08 enforcement change, a rule change by FLOW-14, and adding it here
  would make every later edit of `relay/relay.go` move a pin); (b) declare it
  here as a `rule_changes` entry for GLOBAL-08 on `tools/tier1/rules.yaml`,
  which adds the `tier1-config` area.

## Design

### The relay (`relay/relay.go`)

One helper serves the three handlers, so the rule is written once:

```
serveFromCollector(input, path):
    data, kind, status, err := sink.fetch(path)
    switch:
        err != nil, no reply            -> log; 502 "collector failed: <input>: unreachable"
        err != nil, reply 200 cut short -> log; 502 "collector failed: <input>: read failed"
        status == 404                   -> 404, today's body ("build script unavailable",
                                           "no bootstrap config", "not found: <path>")
        status != 200                   -> log; 502 "collector failed: <input>: HTTP <status>"
        status == 200                   -> as today: serve, and record unless a listing
```

`collectorClient.fetch` (`relay/collector_client.go:212`) already returns the
status for a non-`200` and the read error for a body that fails, so it does
not change. It is in the `collector` seam's code list; leaving it untouched
leaves that pin alone. The handlers' comments are corrected: `handleBootstrap`'s
"A collector without one gives 404" becomes true as written.

What does not change: the `403` for an unsafe context path (GUEST-33), which
the relay decides before it asks; the listing's handling (GUEST-34, COLLIN-29);
the recording of served inputs (GUEST-49); N13 (the client follows a
collector's redirects) and N14 (an input over 2 GiB is cut at the limit with
no error, so it is served and not a `502`). Both are rows of other items
(`seam-specs`' table: with the Rust relay's M-items); this item does not touch
them.

### The executor (`cmd/warden-io/`, `bootstrap/`)

No code change (Decision 5). Tests are added for the reply this item makes the
relay send: `fetchBootstrap` returns an error for a `502` and `setUp` installs
no CA after it; `fetchBuildScript` returns an error for a `502`. They are
tier-0 tests here, because `executor-initialize` and `bootstrap-config` as the
executor reads it have no suite cases yet (TEST-05).

### The overview (FLOW-15)

`docs/book/src/concepts/overview.md` changes in two places:

- `initialize`: `cfg := GET http://artifacts/bootstrap  # 404 => empty config`
  gains `; any other non-200 => stop`.
- The relay's request loop: the `/bootstrap, /build-script` and `cwd` lines
  gain `collector has none -> 404; collector fails -> 502, not recorded`.

### `buildwarden/buildwarden-specs` (LOOP-04)

Work there is a branch `feat/relay-collector-failure-fails-open` and a pull
request to its `main`, which Jeff merges (FLOW-08); the releases are tagged on
the merge commit, each tag in a push of its own, as `seam-specs` did. The
changes are in **Specification changes** below, with the suites' cases:

- `relay-guest-api/cases/`: the `cwd` case that has the collector answer `500`
  for `broken.txt` expects `502` (`cases/cwd.go`, `cwdServed`); new cases for
  `/build-script` and `/bootstrap` with a collector that answers `500`, one
  that resets the stream, and one that answers `404`.
- `bootstrap-config/cases/relay_cases.go`: `collectorFails` and
  `collectorUnreachable` expect `502` and no record; `collectorHasNone` keeps
  `404`.
- `CHANGELOG.md` of each gains `v0.2.0`.

The specs repository has not adopted the Loop Contract (LOOP-08), so the
loop's gates do not run there and `wf` does not see its pull request. The
worker there is bound by FLOW-08 (Jeff merges), FLOW-16 (the branch is
deleted on merge, never by force), FLOW-17 (pushed at green checkpoints,
never `main`, never forced) and FLOW-18 (the session is recorded privately).
The invariants that apply there, by rule ID: TEST-05 (a release carries the
suite that checks it; the contract tests are the suite's cases), GLOBAL-08
(a seam changes its specification first), LOOP-02 (the specification is the
source of truth at the seam), and the specs repository's own convention that
a change adds the version it will be released as to `CHANGELOG.md`. How the
queue records that merge is in the Implementation plan.

The Go relay answers both releases' cases on this branch through a pin to the
specs branch's commit while the work is in progress, and through the release
tags before the pull request here is opened (PROFILE-01's Specifications
entry; the `conformance` workflow's `pin` job).

### The Rust relay

`componentization`'s risk "every seam change lands twice" applies: the Rust
relay (M2 on) implements the specifications, so it meets `v0.2.0` by being
tested against it. Nothing is done for it here.

### The area map (FLOW-05)

The map's `written_seams` lists only `ledger-format`, though `relay-guest-api`
and `bootstrap-config` have had written specifications since `seam-specs`
merged (#125). Read as it stands, FLOW-05 counts both as seams with no
specification, so this item collides with any other item that changes either
seam, where the rule as `main` words it would make them near. This design
does not edit the map; the design review may have it changed (FLOW-19).

### RELAY-08

No relay setting is added or moved. The `502` is the relay's behaviour, not a
configuration, and no claim field, flag, environment variable or boot-channel
value is added.

## Defaults and overrides

- **Removed fallback: a failed collector read as "none".** Today, when the
  collector fails to serve the build script, the bootstrap config or a context
  file, the relay answers `404`, and the executor then uses the empty
  bootstrap config, trust mode `append`, in place of the producer's value.
  That is a fallback no reference document names (GLOBAL-10, GLOBAL-11);
  `seam-specs` named it as one of today's code that it neither added nor
  allowed. This design drops it: the failure is a `502`, and the executor
  stops (BOOTCFG-11). The fallback is a rejected alternative of Decision 1.
- **Unchanged default: no bootstrap config is the empty config.** When the
  collector answers `404` for `/v1/bootstrap` (the session has none), the
  relay answers `404` and the executor uses the empty config, `append`
  (BOOTCFG-10, COLLIN-14). That default is `unified-bootstrap`'s, documented
  in `bootstrap-config`, and is not changed here. What overrides it is a
  config the producer serves, used exclusively.
- No other default, override, merge or fallback is added or changed. No
  retry is added (Decision 6).

## Cross-driver impact

Every driver's build is served by the changed handlers, in every relay mode
(RELAY-04): container (host-side relay container), qemu, hyperv and
firecracker (relay VM, vm mode), vz (host mode). The change is the same code
on each; no driver tree changes, and no driver does anything different. A
driver whose collector works as it should sees no difference. The executor
behaves the same on Linux, macOS and Windows guests: the branch that stops on
a non-`200`, non-`404` bootstrap reply is shared Go code
(`cmd/warden-io/initialize.go`).

## Specification changes

Spec-first (LOOP-02): the two specifications change in
`buildwarden/buildwarden-specs` and are released before this repository's pull
request is opened; here, only the pins move, in
`tools/conformance/relay-guest-api/go.mod` and
`tools/conformance/bootstrap-config/go.mod`, by `make pin-specs
SPECS="relay-guest-api@v0.2.0 bootstrap-config@v0.2.0"`. That is the contract
document GLOBAL-08 and TEST-05 name for each seam, as `main` words them: a
change to one of the five specified seams "changes its specification first:
that specification is released, and the change here moves its pin". The
suites' cases change in the specs repository, not here (TEST-05).

**`relay-guest-api` (to `v0.2.0`):**

| Requirement | Today (`v0.1.0`) | Proposed |
|---|---|---|
| GUEST-25 | `200` from the collector is served and recorded; "When the collector has none (`404`), it MUST answer `404`" | Adds: "When the collector answers any other status, or the request to it fails, or its `200` body cannot be read in full, the relay MUST answer `502` and MUST NOT record anything; the Go relay's body is `collector failed: build script: <reason>` and a line feed, where `<reason>` is `HTTP <n>`, `unreachable` or `read failed`." |
| GUEST-26 | As GUEST-25, for `/bootstrap` | The same addition, with `bootstrap config`. The sentence "what a `404` means to the executor, [is] `bootstrap-config`'s" stays |
| The paragraph after GUEST-26 | "The Go relay also answers `404` ... when the collector answers any other status, or cannot be reached; this version does not define the reply in that case." | Removed: the reply is defined by GUEST-25 and GUEST-26 |
| GUEST-34 | Third bullet: "anything but `200` (the path does not exist): `404`"; then "The Go relay answers `404` as well when the collector cannot be reached; this version does not define that reply." | Third bullet: "`404` (the path does not exist): `404`, with the body `not found: <path>` ...". New fourth bullet: "any other status, a request that fails, or a `200` body that cannot be read in full: `502`, not recorded; the Go relay's body is `collector failed: <path>: <reason>` and a line feed." The sentence after is removed |
| Errors table | `artifacts` `404` for none; `cwd` `404` | Adds rows: `artifacts` `502` `collector failed: build script: <reason>`, `collector failed: bootstrap config: <reason>` (Go relay), when the collector failed to serve it (GUEST-25, GUEST-26); `cwd` `502` `collector failed: <path>: <reason>` (Go relay) (GUEST-34) |
| Known divergences, N24 | The Go relay serves a failure as "none" | Removed, with a line in `CHANGELOG.md` naming the fix |
| Unchanged | GUEST-33 (`403` for an unsafe path), GUEST-49 and GUEST-50 (what is recorded), GUEST-44 to GUEST-46 (the proxy's `502`) | |

**`bootstrap-config` (to `v0.2.0`):**

| Requirement | Today (`v0.1.0`) | Proposed |
|---|---|---|
| BOOTCFG-3 | "When the collector answers `GET /v1/bootstrap` other than `200`, or cannot be reached, a relay MUST answer `GET http://artifacts/bootstrap` with `404` and record nothing. To the executor that is the empty config (BOOTCFG-10)." | "When the collector answers `GET /v1/bootstrap` with `404`, a relay MUST answer `GET http://artifacts/bootstrap` with `404` and record nothing; to the executor that is the empty config (BOOTCFG-10). When the collector answers any other status but `200`, or the request to it fails, or its body cannot be read in full, a relay MUST answer `502` (`relay-guest-api`, GUEST-26) and record nothing, and MUST NOT answer `404`; to the executor that is a failure, and it stops (BOOTCFG-11)." |
| BOOTCFG-10, BOOTCFG-11 | `404` is the empty config; any status but `200` and `404` stops the executor | Unchanged. The executor already meets both (`fetchBootstrap`) |
| Known divergences, N24 | A collector's failure turns `exclusive` into `append` | Removed, with a line in `CHANGELOG.md` |
| Conformance section | "A relay conforms ... when it meets BOOTCFG-1 to BOOTCFG-3" | Unchanged in form; BOOTCFG-3 is the amended one |

**Implemented unchanged:**

- `collector-inputs` `v0.1.0`: COLLIN-13 and COLLIN-14 (a collector's `404`
  for none), COLLIN-15 to COLLIN-17 and COLLIN-29 (the context and its
  listing). No requirement of the collector or of the relay's request
  changes; COLLIN-14's "A relay and an executor read the 404 as the empty
  config" stays true. No change, no new release.
- `executor-initialize` `v0.1.0`: no change, no new release (Decision 5).
- `collector-outputs`, `relay-claim`: not touched.

## Security note

**Boundary.** env-relay: the relay is the build environment's only source of
its bootstrap config, and that config sets which roots the build trusts.
GLOBAL-06 also flags any change to the relay itself.

**Posture before.** A producer's `exclusive` trust (only the relay's CA is a
root, so every TLS connection the build makes goes through the relay's
interception and is recorded) becomes `append` (the system's roots trusted as
well) whenever the collector fails to serve the config. The relay is still
the only network path, so the build still cannot reach anything except through
the relay; what is lost is the producer's guarantee that the build trusts only
the relay's CA, which matters, for example, to a build environment whose
system roots or `ca-certificates` hooks the producer does not control, or to a
check that every TLS session the build accepts is one the relay made. The
failure is silent: the build reports `trust append` as if none was asked for.
A collector failure can be caused from outside the build (a disk full on the
host, a collector restart, a dropped tunnel), not by the build itself, which
has no path to the collector.

**Posture after.** A collector failure on the bootstrap config stops the build
before any CA is installed (BOOTCFG-11), with `bootstrap config: HTTP 502` in
the build's log and the cause in the relay's. The other two inputs give the
build a `502` it cannot mistake for absence. Nothing else changes: no new
input, no new record, no new endpoint, no new setting, and the `502` body
carries fixed text only, so no host detail (the collector's address, the
tunnel's state) reaches the build.

**What TEST-08 should exercise.** A collector that answers `GET /v1/bootstrap`
with `500`, that resets the stream, and that cuts a `200` body short, while it
holds `{"trust": {"mode": "exclusive"}}`: the build must stop with no CA
installed and no build script run, and must never run with `append`. The same
for `build-script` (no script runs) and for `cwd` (the build sees `502`, not
`404`). Arm A can carry these as corpus cases; the specification suites carry
the relay's half.

## Validation plan

- **Tier 0 (`make test`).** Relay: a test per handler with an in-process
  collector that answers `500`, `404` and a reset stream, and one whose `200`
  body is cut short, checking the status, the body and that nothing is
  recorded (`relay/relay_collector_test.go`, beside its existing `cwd` `404`
  case). Executor: the tests of Decision 5 (`cmd/warden-io/`).
- **Tier 1 (`conformance` workflow, TEST-05).** The `relay-guest-api` and
  `bootstrap-config` suites at `v0.2.0` against the Go relay in host, vm and
  container mode and as its image. This is where N24 was found, and the cases
  that asserted `404` now assert `502`. The `integration` workflow runs the
  container driver end to end as today.
- **Tier 2 / native.** None blocking and none tracked: the change is
  platform-neutral relay code that the suites run in every relay mode, and
  the executor branch it relies on is shared Go code, already run by tier 0.
  No driver's lifecycle changes (FLOW-09). The full matrix before a release
  (TEST-02) is the backstop.
- **TEST-08.** The change trips GLOBAL-06, so the next release's security
  suite includes the cases in the Security note.

## Implementation plan

Plan writes the tasks; the order the design expects, each with its rules:

1. In `buildwarden/buildwarden-specs`, on `feat/relay-collector-failure-fails-open`:
   amend `relay-guest-api/v1.md` and `bootstrap-config/v1.md` as in
   **Specification changes**, change and add the cases, add `v0.2.0` to each
   `CHANGELOG.md`; `go vet` and `go test ./...` in each changed module; push;
   open the pull request (LOOP-02, TEST-05, FLOW-17).
2. Here: `make pin-specs REF=<that branch's head>` so the branch runs the
   changed suites (PROFILE-01).
3. Here: the relay change in `relay/relay.go` (Design), with its tier-0 tests
   and corrected comments (GLOBAL-10, GLOBAL-11, UX-04, RELAY-02, TEST-01).
4. Here: the executor tests (BOOTCFG-11, EXEC-04, TEST-01).
5. Here: `overview.md` (FLOW-15).
6. Release: Jeff merges the specs pull request (FLOW-08); the tags
   `relay-guest-api/v0.2.0` and `bootstrap-config/v0.2.0` are pushed on the
   merge commit, each in a push of its own; then `make pin-specs
   SPECS="relay-guest-api@v0.2.0 bootstrap-config@v0.2.0"`, `make test`,
   `make lint`, push (GLOBAL-08, TEST-05). FLOW-04 and FLOW-08 provide a
   `human` step that records a merge in a repository where the loop's gates
   do not run; Plan adds it as `f6-release-specs`, phase 6, needing the
   design review step once the queue has seeded it, so that it is not offered
   before then, and `f6-open-pr` needs it. Until it is done the `pin` job
   fails the pull request here, which is the backstop.

## Design review

Pending.

## Consolidation

Filled in at Publish.

## Rollback notes

None.

## Archive summary

Filled in at Publish.
