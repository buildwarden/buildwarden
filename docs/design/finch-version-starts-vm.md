---
feature: finch-version-starts-vm
status: draft
drivers: [container]     # driver/container/runtime.go only; no other driver tree is reached
seams: []                # the driver contract's wire and requirements are unchanged; the
                         # runtime check is the container driver's own behaviour
surfaces: [output]       # the "installed but not running" message for finch names the query
                         # that decided it; the guide shows it (UX-01)
repos: []                # this repository only
merge_first: []
areas: [driver/container, book, designs]
                         # driver/container: runtime.go and its tests; book: the guide's
                         # cli.md and configuration.md, overview.md's container pseudocode
                         # (FLOW-15); designs: this design
security_boundary: none  # no isolation, relay or ledger code is reached: the change is which
                         # host command the driver runs to see whether finch's VM is up
alters_relationships: false
design_review: required  # blocking native validation (macos-finch) and assumed questions,
                         # one flagged, rule out skip (FLOW-19)
native_validation:
  - {host: macos-finch, blocking: true, reason: "the defect is finch v1.19.0's own behaviour on macOS (its version query starts a stopped VM); no fake runtime in tier 0 shows that the status query leaves the VM stopped, or what finch prints for each state"}
tier2_gaps: []
rules: [GLOBAL-09, GLOBAL-10, GLOBAL-11, UX-01, UX-04, UX-08, UX-09, ORCH-01, TEST-01,
        TEST-02, TEST-04, FLOW-01, FLOW-02, FLOW-06, FLOW-07, FLOW-09, FLOW-12, FLOW-15,
        FLOW-19]
rule_changes: []
concepts: [container-runtime, runtime-detection, runtime-check, finch, doctor, slow-hint]
related: [driver-contract.md, runtime-cli-project-path.md]
---

# The container runtime check starts a stopped finch VM and then reports it as failed

## Problem

driver-contract's Decision 26 (r4, CI repair, Jeff 2026-11-22) made the
container runtime check the runtime's server-version query: `docker version
--format '{{.Server.Version}}'`, the same for `podman`, and `finch version`
bare. Detection gives each candidate 3 s; a named runtime's first engine call
in a build, and `warden doctor`'s check of a named runtime and its re-ask of
each candidate, give it 10 s, and a candidate that answers only in the second
window gets doctor's slow hint. The decision rests on a premise written from
finch's source: "finch's own `version` asks nerdctl in its VM and fails when
the VM is not running" (`versionQuery`'s comment in
`driver/container/runtime.go`).

On finch v1.19.0 that premise does not hold. The macbook's native run of
`driver-contract-runtime-check-finch` on `main` `c52926d`
(host:macos-finch/macbook.20261123T220740Z, log
`native/macos-finch/2026-11-23/runtime-check.log` on the `workflow` branch)
found:

- `finch version` with the VM stopped **starts the VM** and exits 0 once it is
  up, after 11.8 s (`finch info` does the same).
- `warden build --runtime finch` with the VM stopped kills the query at 10 s
  and fails with "container runtime finch is installed but not running
  (`finch version` failed)". The VM finishes starting anyway; the next build
  passes.
- `warden doctor --runtime finch` is blocked the same way after 10 s, and
  `warden doctor` with nothing named after 4.5 s (detection's 3 s and the
  re-ask cut short); both leave the VM Running.
- No slow hint is ever seen: the VM's start takes longer than either window.

So the check has a side effect the decision did not intend (it starts the
VM), its error is wrong after the fact (the runtime it calls "not running" is
starting, and running a few seconds later), and its result depends on how
fast the VM boots. A user who runs `warden doctor` to see whether finch is up
gets "not running" and a running VM.

## Prior art

- **driver-contract** (archived), Decision 26, Behaviour 20, question a28 and
  the **Defaults and overrides** rows on the container runtime: the check, its
  windows (3 s detection, 10 s for a named runtime and doctor's re-ask), the
  slow hint, the "installed but not running" error with the start hint, and
  the refusal to try detection in a named runtime's place. This design keeps
  all of them and changes only how finch's check is made, so it never starts
  the VM (Decision 1). It corrects the decision's premise about `finch
  version`, which the native run disproved.
- **runtime-cli-project-path** (archived): the supported runtimes and their
  order, unchanged.
- No design in `docs/design/` has `finch`, `runtime-check` or
  `runtime-detection` among its concepts other than driver-contract; this one
  gives those tags to the archive (FLOW-12). driver-contract already has
  front-matter.

## Decisions

1. **finch's check asks the VM's state first, and asks the version query only
   of a running VM.** For `finch`, `runtimeAnswers` runs `finch vm status`
   first. `Running` goes on to `finch version`, as today; `Stopped` or
   `Nonexistent` is "not running" at once, and the version query is not run,
   so the check never starts the VM. Both commands share the one deadline the
   caller gives (3 s for detection, 10 s for a named runtime and doctor's
   re-ask), so neither window grows. docker and podman are unchanged.
   Alternatives:
   - (b) **Lengthen finch's windows** (say 30 s) so its version query can
     finish starting the VM: a departure from Jeff's 3 s and 10 s, a build
     or a doctor that waits on a VM boot every time it is stopped, and a
     health check that changes what it checks. Rejected.
   - (c) **Let the query start the VM, and say so**: a stopped VM is started
     by any build and by doctor, with a line that says it is starting. It
     departs from Decision 26's answer for a runtime whose engine is down
     (refuse, with the start hint) and makes doctor a command with a side
     effect (UX-04 makes it a check). Rejected; it would be Jeff's call, and
     a1 names it as an option.
   - (d) **`finch vm status` alone, no version query**: a VM that is Running
     with nerdctl or containerd not answering inside it would pass, where
     today it fails. Rejected: the version query stays the check of the
     engine, as Decision 26 says.
   - (e) **Word the timeout differently** ("did not answer within 10 s; it
     may be starting"): the VM is still started by a check, and the result
     still depends on its boot time. Rejected.
2. **A state the driver does not know leaves the decision to the version
   query, as on `main`.** If `finch vm status` fails, or prints anything but
   `Running`, `Stopped` or `Nonexistent`, the driver asks `finch version`
   within what is left of the deadline, which is today's check. This is the
   case on Linux, where finch runs without a VM and has no `vm` command, and
   on a finch older than its `vm status`. The fallback is named under
   **Defaults and overrides**. Alternative: treat an unknown state as not
   running, which refuses a working finch on Linux. Rejected.
3. **The message names the query that decided.** When finch's VM state
   decides, the error reads "container runtime finch is installed but not
   running (`finch vm status`: Stopped)" (or `Nonexistent`), with the start
   hint unchanged ("start its VM: finch vm start (or finch vm init the first
   time)"); when the version query decided, it names `finch version` as
   today. docker's and podman's messages are unchanged. Alternative: keep
   "(`finch version` failed)" for every case, which names a command that was
   not run. Rejected.

## Questions

### Blocking

None.

### Assumed

- **a1. finch's check asks `finch vm status` before its version query
  (Decision 1).** Proceeding with (A): for finch, `Running` goes on to `finch
  version` within the same deadline; `Stopped` and `Nonexistent` are "not
  running" without running the version query, so no check starts the VM; the
  3 s and 10 s windows, the slow hint, the start hint and the refusal of a
  named runtime stay as Decision 26 has them. Options: (A) as stated; (B)
  lengthen finch's windows so its version query can start the VM (a
  departure from Jeff's 3 s and 10 s); (C) let the check start the VM and say
  so (a departure from Decision 26's refusal with the start hint, and doctor
  gets a side effect). Recommended: (A). Flagged: it changes the command
  Decision 26 names as finch's check, whose premise about `finch version`
  the native run disproved, and (B) and (C) would depart from Jeff's
  decision of 2026-11-22.
- **a2. An unknown or failed `finch vm status` leaves the decision to `finch
  version` (Decision 2).** Proceeding with (A): the version query decides
  within the rest of the deadline, as on `main`; this is Linux finch, which
  has no VM, and an older finch. Options: (A) as stated; (B) such a state is
  "not running"; (C) skip the status query by host (`GOOS` linux), and treat
  an unknown state elsewhere as not running. Recommended: (A), one rule on
  every host that keeps today's check wherever the state cannot be read.
- **a3. The error names `finch vm status` and the state when the state
  decided (Decision 3).** Proceeding with "container runtime finch is
  installed but not running (`finch vm status`: Stopped)", the fix unchanged.
  Options: (A) as stated; (B) keep "(`finch version` failed)". Recommended:
  (A).

## Design

All of it is in `driver/container/runtime.go`.

- `runtimeAnswers(name, timeout)` makes one context with the timeout, as
  today. For `finch` it first runs `finch vm status` under that context
  (`WaitDelay` 1 s, as the version query has) and reads its trimmed stdout:
  - `Running`: it runs the version query under the same context, as today.
  - `Stopped` or `Nonexistent`: it returns "not up" with that state, and runs
    nothing else.
  - anything else, or an error: it runs the version query under the same
    context (Decision 2).
  It returns the outcome and what decided it (the state, or the version
  query), so the messages can name it.
- `versionCommand`'s users (`notAnswering`, `checkEngine`, `doctorRuntime`'s
  named branch) name `finch vm status: <state>` when the state decided, else
  the version command as today.
- Nothing else changes: `detect`, `checkEngine`, `doctorRuntime`,
  `answeredSlowly`, the two timeouts and their uses. A stopped finch is now
  "not up" within the time `finch vm status` takes (a fraction of a second on
  the macbook, to be measured), in detection, in a build's first engine call
  and in doctor's re-ask; it is never given the slow hint, since it never
  answers the version query. The VM is left as it was found.
- `versionQuery`'s comment loses the disproved premise and says what finch
  v1.19.0 does.
- Docs (UX-01, FLOW-15): `docs/book/src/guide/cli.md` (doctor's health
  check names finch's state query), `docs/book/src/guide/configuration.md`
  (the example error for a stopped finch), and the container driver's
  pseudocode in `docs/book/src/concepts/overview.md` (the detection line:
  finch asks `vm status` first and its version query only when Running).

The area map has what this needs (`driver/container`, `book`, `designs`);
nothing is missing from it. There is no other repository.

## Defaults and overrides

- **No default changes.** The windows (3 s, 10 s), the detection order and
  the runtime values are Decision 26's and runtime-cli-project-path's, and
  stay. The overrides (`--runtime`, `WARDEN_CTR_CLI`, `[driver.container]
  cli`) are unchanged and used exclusively.
- **Fallback, named: an unreadable finch VM state** (Decision 2, a2).
  Trigger: `finch vm status` fails, or prints anything other than `Running`,
  `Stopped` or `Nonexistent` (Linux finch, an older finch). Effect: the
  version query decides within the rest of the deadline, which is `main`'s
  check. How the user sees it: as today; a failure names `finch version`, a
  success is silent. It is a fallback the source does not settle, so it is
  assumed question a2. The fallback dropped: none; today's check had no
  fallback.

## Cross-driver impact

- **container**: finch's runtime check, as above. docker and podman are
  unchanged.
- **qemu, vz, hyperv, firecracker**: unaffected; none runs a container
  runtime, and nothing in `driver/` outside `driver/container` is reached.

## Validation plan

- **Tier 0** (`make test`, every platform; TEST-02, TEST-04): the fake finch
  in `driver/container`'s runtime tests answers `vm status` from its fixture
  (Running, Stopped, Nonexistent, an unknown word, a failure) and records
  each call. Tests: a Stopped or Nonexistent finch is not up and its version
  query is never run, in detection, in `checkEngine` and in doctor's
  re-ask; Running runs the version query and its outcome decides; an unknown
  state or a failed status runs the version query (a2); the two commands
  share one deadline (a status that sleeps past it is not followed by a
  version query); the messages name `finch vm status: Stopped` or `finch
  version` per a3; `TestDoctorAsksEachRuntimeAtMostTwice` and the existing
  slow-hint tests still pass, with finch's calls counted as its status
  query plus at most one version query per ask.
- **Tier 1**: unchanged checks; the surface check covers the message text in
  the guide.
- **Tier 2, native**: **macos-finch, blocking** (FLOW-09): the defect is
  finch's own behaviour on macOS, which no fake shows. Cases, in the queue
  step `f6-validate-macos-finch`:
  1. VM Running: `finch vm status` prints `Running` (record its exit code);
     `warden doctor --runtime finch` and `warden doctor` print `container
     runtime: finch` and `ok`; a default build succeeds and its ledger
     verifies.
  2. `finch vm stop`; `finch vm status` prints `Stopped` (record the exit
     code and the time); run it three times; `finch vm status` still prints
     `Stopped` (the status query does not start the VM).
  3. VM stopped: `time warden build --runtime finch` fails within 2 s with
     "container runtime finch is installed but not running (`finch vm
     status`: Stopped)" and the start hint, leaves the output directory
     (`driver.json`, `inputs/`), and `finch vm status` is still `Stopped`
     afterwards.
  4. VM stopped: `time warden doctor --runtime finch` and `time warden
     doctor` are each blocked within 2 s with the same state, exit 1 with no
     `ok`, and leave the VM `Stopped`.
  5. VM stopped: `time warden build` with nothing named fails with
     detection's "installed but not running" naming the state, within 2 s,
     VM still `Stopped`.
  6. `finch vm start`, then a default build succeeds, and `warden doctor`
     prints `ok`.
  7. `go test ./driver/container/...` passes on macOS.
  Evidence under `native/macos-finch/<date>/`. The VM is left Running, as
  found. `finch vm remove` is not run (the `Nonexistent` state is tier 0
  only).
- **Not covered natively, no gap filed**: Linux finch and Windows finch
  (WSL2). No native host has either; the fallback (a2) keeps today's check
  where the state cannot be read, and tier 0 covers it.

## Implementation plan

1. `driver/container/runtime.go`: finch's `vm status` before its version
   query under one deadline, the outcome carrying what decided it, the
   messages naming it, `versionQuery`'s comment corrected (Decisions 1 to 3;
   GLOBAL-10, GLOBAL-11, UX-08, UX-09).
2. `driver/container/runtime_answers_test.go` and `runtime_test.go`: the
   fake finch answers `vm status`; the tests under **Validation plan**
   (TEST-01, TEST-02, TEST-04); `TestNotAnsweringNamesTheQuery` and the
   detection-order test's recorded calls updated for finch's status query.
3. Docs: `cli.md`, `configuration.md`, `overview.md`'s container pseudocode
   (UX-01, FLOW-15).
4. `make test` and `make lint` (darwin and linux, since the test fakes run
   per platform) before pushing (FLOW-17).

## Design review

None yet.

## Consolidation

Filled in at Publish.

## Rollback notes

None.

## Archive summary

Filled in at Publish.
