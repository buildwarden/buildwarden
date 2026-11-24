---
feature: relay-conformance-timeout
status: archived
drivers: [qemu, vz]      # conservatively (FLOW-02): cmd/relay's host mode, the only mode that
                         # changes, is the relay qemu (a host process, --mode=host) and vz run;
                         # no driver tree changes, and the relay area is shared (areas.yaml implies.shared)
seams: []                # no specification changes: GUEST-6 already requires what the fix
                         # restores (Specification changes is therefore left out; see Design)
surfaces: []
repos: [buildwarden/buildwarden, buildwarden/buildwarden-specs]
                         # home first; the harness's netstack and oci launcher, and its own
                         # in-process relay, are in the second (as intake recorded, from seam-specs)
merge_first: []          # the two pull requests are independent: neither consumes the other's change
areas: [relay]           # cmd/relay/ingress_fd.go and its test. buildwarden-specs has no area map,
                         # so it is one exclusive area (FLOW-05); see Design, "Areas"
security_boundary: env-relay  # GLOBAL-06's relay clause: the relay itself changes; posture unchanged (Security note)
alters_relationships: false   # inside the relay's and the harness's existing roles; no seam, owner or contract changes
design_review: required  # FLOW-19: security_boundary is not none and assumed questions are open,
                         # so skip is not allowed; not a program item, so not umbrella
native_validation: []    # no host is blocking or tracked: the changed code is platform-neutral
                         # and CI's relay-host target runs it (Validation plan)
tier2_gaps: []
rules: [TEST-02, TEST-04, TEST-05, GLOBAL-06, GLOBAL-09, GLOBAL-10, GLOBAL-11, RELAY-04, ORCH-01,
        LOOP-02, LOOP-04, LOOP-07, FLOW-01, FLOW-02, FLOW-05, FLOW-07, FLOW-09, FLOW-12,
        FLOW-14, FLOW-19, FLOW-20]
rule_changes: []
concepts: [relay, host-mode, fd-ingress, netstack, gvisor, tcp-time-wait, tcp-forwarder,
           conformance, conformance-harness, oci-launcher, flaky-test, test-determinism]
related: [seam-specs.md, build-link-subnet.md, http-body-path-drift.md]
---

# Relay conformance runs fail intermittently on a relay connection timeout

## Problem

The `relay` job of `conformance.yml` turns pull requests red with unchanged
relay code and passes on a re-run (TEST-04: a flaky test is a defect). Jeff
filed it on 2026-11-22 and released it from the FLOW-20 hold. Every failure
found, from all 48 `conformance` runs on record and from `buildwarden-specs`'s
own `ci`:

| Run | Target | Case | Failure |
|---|---|---|---|
| buildwarden 45067235357 (PR #135, e8297df), job 137346008112 | relay-host | cwd serves a context file ... (GUEST-34) | `a connection to the relay's 10.100.0.1:80: context deadline exceeded` |
| buildwarden 45069728550 (PR #136, f17d7e7), job 137353768984 | relay-host | the same | the same |
| buildwarden 45073926927 (e0f3fcd), job 137366822289 | relay-host | a host-mode relay refuses loopback ... (GUEST-46) | `GET / with Host refused-paired-9-0.example ...: context deadline exceeded` |
| buildwarden 44077869082 (c889ecc), job 134020469914 | relay-host | the same (GUEST-46) | `... refused-paired-4-1.example ...: context deadline exceeded` |
| buildwarden-specs 45086692060 (main, 70457f1), job 137406838553 | in-process host, `go test` | the same (GUEST-46) | `... refused-paired-8-0.example ...: context deadline exceeded` |
| buildwarden 45062826685 (PR #135, 3efd1fe), job 137332327954 | relay-oci | once the build has completed ... (GUEST-55) | `the container bw-conformance-27069-52 was not running within 30s` |

(Run 43841571312 on `feat/seam-specs` failed too; those were real failures of
the branch then under construction, not this.)

Two different causes are behind them.

**1. Every `context deadline exceeded` is a host-mode relay that ignores a
reconnection for 60 s.** All five are on a frame link, against two different
relays (the Go relay's host mode, and the harness's own in-process relay),
and each is a new TCP connection from the build environment that the relay
never answers. The in-process relay logs every request, which shows it: in
the specs run the relay served about 40 requests within 25 ms, then nothing
for exactly 10 s (the case's bound, `within`), then the next request.

Both relays are gVisor stacks that take the build's connections with a
`tcp.Forwarder` (`cmd/relay/ingress_fd.go`; the harness's
`conformance/netstack.Capture`), because GUEST-6 has a frame-link relay accept
a connection to any address and port, which no listener can. Every case's
request says `Connection: close`, so the relay closes first and its end of
each connection then sits in TIME-WAIT for gVisor's default 60 s
(`tcp.DefaultTCPTimeWaitTimeout`). The build environment's stack picks each
connection's source port at random from 16000-65535 (gVisor's
`PickEphemeralPort` with the stack's secure RNG), so now and then it picks a
port it used earlier in the case, and so the 4-tuple of a connection the
relay still holds in TIME-WAIT. A SYN that reopens a TIME-WAIT connection is
handed on by gVisor only to a *listening* endpoint
(`Endpoint.handleTimeWaitSegments`, which looks one up with
`FindTransportEndpoint`); a forwarder is a protocol handler, not an endpoint,
so the SYN is dropped, and so is every retransmission, until the TIME-WAIT
ends 60 s later. The build environment's dial gives up at 10 s.

This was reproduced deterministically against both relays, in throwaway tests
not committed: a client stack on the frame link connects from a fixed port
40000, reads the relay's reply, closes, and connects again from port 40000.

- `conformance/netstack` (the in-process relay's stack, `Capture`): the
  first connection takes 0.7 ms, the second fails with `context deadline
  exceeded` after 10.0 s.
- `cmd/relay`'s `NewFDIngress` (the Go relay's host mode) over a real
  socket pair: 1.3 ms, then `context deadline exceeded` after 10.0 s.
- The same two with the relay stack's `tcpip.TCPTimeWaitTimeoutOption` set
  to 0: 50 and 20 consecutive reconnections from the same port, each in
  under 1 ms.

The odds match what CI saw. A case that opens *n* connections to one port
of a fresh relay collides with probability about n(n-1)/2 / 49536: about 4 %
for the GUEST-46 host case (about 66 connections), about 0.13 % for the cwd
case (about 12), and in between for the others; summed over the suite's
frame-link cases it is a few percent of runs, and CI failed 4 times in 48
runs. Both failing cases were then run on their own, repeatedly, against the
harness's in-process host relay (a throwaway test, not committed), before and
with the one-line change of D1 in `netstack.Capture`:

| Case | Runs | Failed before | Failed with D1 |
|---|---|---|---|
| a host-mode relay refuses loopback ... (GUEST-46) | 300 | 11 (3.7 %), each `context deadline exceeded` | 0 |
| cwd serves a context file ... (GUEST-34) | 3000 | 4 (0.13 %), each `a connection to the relay's 10.100.0.1:80: context deadline exceeded` | 0 |

Both rates are what the count of connections predicts, and both go to zero.
(The cwd case failing twice in CI, against the Go relay, is more than 0.13 %
gives over 48 runs; it is the same failure, the same dial, and the same
cause, and the Go relay's reproduction above shows it has the defect.)

It is not only a test problem. A real guest of the qemu or vz drivers on the
Go relay's host mode reuses a 4-tuple too (a Linux guest walks its port range
per destination and wraps), and then waits a minute for a connection the relay
must accept (GUEST-6). Builds rarely make enough connections to one captured
address and port to wrap, so it has not been seen there, but it is the same
defect.

**2. The GUEST-55 failure is the container runtime, charged to the relay.**
The relay-oci launcher (`conformance/launch/relay.go`, `startOCI`) runs `docker
run` and polls `docker inspect` until the container has a process, within
`sandbox.StartLimit` (30 s), and only then gives the relay its links and
waits, within StartLimit again, for its claim endpoint. In the failure the
relay's first line, `relay: awaiting claim on [::]:8301`, is stamped
19:57:42, the second the harness gave up: the relay's own process did not
start until about 30 s after `docker run` began. The relay prints that line
as soon as it has bound its claim listener (`serveClaim`), and the other 63
oci starts in the same suite took their normal fraction of a second (the
relay-oci and relay-container runs of `relay-guest-api` differ by 6 s over 64
cases). So the time went to the runtime starting the container, which is
neither repository's code and which one occurrence does not explain further.
What is the harness's is that it gives the runtime's start the relay's start
bound, reports it as the component under test failing to start, and records
nothing that would say where the time went next time.

## Prior art

- **`seam-specs.md`** (archived) built the harness, its sandbox, the exec and
  oci launchers, the frame link and the in-process relay, and its targets
  file `tools/conformance/targets/`. This design keeps all of it and changes
  two behaviours inside it: the TIME-WAIT of a capturing stack, and the oci
  start's bounds and report. Its rule that the harness's own timings never
  decide a verdict ("Its limits are not what a case measures", The harness,
  point 3) is what the oci change applies to the runtime.
- **`build-link-subnet.md`** (archived) gave the host-mode relay its link
  from the claim and its DHCP; nothing there changes.
- **`http-body-path-drift.md`** (archived) is the precedent for a relay
  change that is platform-neutral relay code, declares every dependent driver
  conservatively, `security_boundary: env-relay` with an unchanged posture,
  and no native validation because tier 0 and tier 1 run the changed code.
- **GUEST-6** (`relay-guest-api/v1.md`): a relay on a frame link MUST accept
  a TCP connection to any destination address and port. The fix restores
  conformance to it; it adds nothing to it.
- **TEST-04**: the fix is to the cause, not a retry; and the tier-0 tests
  added are deterministic (a fixed source port, no randomness).
- **GLOBAL-11**: no fallback is added, and none is used to hide a failure.

No other design in `docs/design/` lists `netstack`, `tcp-time-wait`,
`fd-ingress` or `oci-launcher` among its concepts; the front-matter search
found none to reuse beyond those above.

## Decisions

**D1. The relay's own stack keeps no TIME-WAIT.** Both host-mode stacks that
capture the build's connections set `tcpip.TCPTimeWaitTimeoutOption(0)`: the
Go relay's in `NewFDIngress`, and the harness's in `netstack.Stack.Capture`.
A connection the relay closes first then goes from TIME-WAIT to closed at
once, and a SYN that reuses its 4-tuple reaches the forwarder like any other.
TIME-WAIT guards against a delayed segment of an old connection being taken
for part of a new one on a path that can delay and reorder; the frame link is
a point-to-point socket pair (or an in-memory pipe) that the relay owns and
that delivers in order, so the guard buys nothing there and the 60 s cost a
connection GUEST-6 says must be accepted. The relay's final ACK lost on that
link would make the guest's FIN be answered with a reset after the guest has
everything, which ends a connection that was ending anyway. *(a1)*

Rejected:

- *A short TIME-WAIT (1 s, say).* A reused 4-tuple still stalls until it
  ends plus the guest's next SYN retransmission (1 s, then 2 s and 4 s), so a
  case's outcome would still turn on timing. It is a smaller timeout, not a
  fix.
- *Listeners for the service ports beside the forwarder.* gVisor reopens a
  TIME-WAIT connection for a listener, but GUEST-6 serves every port, and
  the forwarder would still hold every port without a listener; it also
  rests on how gVisor matches a wildcard listener in promiscuous mode.
- *A patched gVisor that hands such a SYN to the forwarder.* A fork of a
  dependency for a behaviour one option already gives.
- *Have the harness's build environment pick its source ports in order, or
  never reuse one.* The suites would then stop finding a relay defect that a
  real guest meets, and the in-process relay would keep it.
- *Retry the dial in the case, or raise `within`.* TEST-04 forbids masking a
  flake with retries, and the stall is 60 s; a case would have to wait a
  minute for a relay to do what it MUST do at once.

**D2. The oci launcher bounds the runtime's start apart from the relay's,
and says which one ran out.** In `startOCI` the wait for the container to be
running is bounded by `CommandLimit` (60 s), the bound the harness already
gives a target's own commands, and its failure says that the runtime did not
start the container and that the relay under test was never running. The
relay's own bound, StartLimit (30 s) from the moment the container is
running, to serve its claim endpoint, is unchanged. Each oci start records
when `docker run` started, when `inspect` first returned the container, when
it had a process, and when the claim endpoint answered; a failed start
prints them with the runtime's `inspect` state, so the next occurrence says
where the time went. The `inspect` poll keeps its 20 ms interval but each
call is bounded by what is left of the wait. *(a3)*

Rejected:

- *Only raise StartLimit.* That is the longer timeout the item is not to be,
  and it gives the relay longer too.
- *Retry `docker run` once.* A masked flake (TEST-04), even reported.
- *Create each case's container ahead of time.* It would take the runtime
  off a case's path but needs a pool the sandbox does not have, for one
  occurrence of a cause outside both repositories.
- *Leave it until it happens again.* The failure would read again as the
  relay failing to start, with nothing to go on.

**D3. No specification changes, and no new case.** GUEST-6 already requires
a relay to accept the connection the defect drops, so no requirement
changes, and no release of a specification follows. The regression is pinned
by a tier-0 test in each repository (Validation plan). A suite case that
reconnects from a fixed source port would catch the defect in any
implementation, but it is a change to the `relay-guest-api` suite and so a
new release of it, Jeff's to tag; it is offered as a follow-up. *(a2)*

**D4. This item moves no pin.** The Go relay's fix is all `buildwarden` needs
to make its `relay` job deterministic: it runs the pinned suites, whose build
environment is unchanged. The harness's two changes reach `buildwarden` at
its next pin of `conformance`, after Jeff tags `conformance/v0.2.1` (a
release without a suffix is his, FLOW-20). *(a4)*

**D5. The same pattern in `tools/vz-image-prep` is out of scope.**
`tools/vz-image-prep/netfwd.go` captures a VM's connections with a gVisor
forwarder too, and would stall the same way on a reused 4-tuple. It is a
different component (the `driver/vz` area), it has never been seen to fail,
and changing it would make macos-vz validation blocking (FLOW-09). *(a5)*

## Questions

### Blocking

None.

### Assumed

- **a1.** Proceeding with D1: both capturing stacks (the Go relay's host mode
  and the harness's `netstack.Capture`) set gVisor's TIME-WAIT timeout to 0,
  a fixed value with no flag or config key. Options: (a) 0, no TIME-WAIT
  (recommended: no stall at all, deterministic); (b) a short TIME-WAIT such
  as 1 s (a shorter stall, still timing-dependent); (c) leave the relay and
  make the harness avoid reusing ports (hides a real GUEST-6 defect).
  Confirm. Flagged: it changes the relay's TCP behaviour on the
  build-environment-to-relay boundary (GLOBAL-06), though not its posture.
- **a2.** Proceeding with D3: no specification change and no new suite case
  in this item; a case that reconnects from a fixed source port is offered
  to Jeff as a follow-up to file or not. Options: (a) none here (recommended:
  GUEST-6 already says it, tier 0 pins both fixes, and the item stays free of
  a release); (b) add the case to `relay-guest-api` here, which needs its
  release (Jeff's tag) and a pin move before `buildwarden` runs it. Confirm.
  Flagged: it is a reading of a seam's requirement (GUEST-6).
- **a3.** Proceeding with D2: the runtime's start is bounded by CommandLimit
  (60 s) and reported as the runtime's, the relay's bound is unchanged, and
  each oci start's phases are recorded and printed on failure. Options: (a)
  as D2 (recommended); (b) record the phases only and keep the 30 s bound
  until a second occurrence names the cause; (c) one retry of `docker run`,
  reported (rejected by TEST-04). Confirm.
- **a4.** Proceeding with D4: no pin moves here; the harness's changes reach
  `buildwarden` when `conformance/v0.2.1` is tagged and pinned later.
  Options: (a) no pin move (recommended: the CI failures here are all fixed
  by the relay change alone, and the item does not wait on a release); (b)
  wait for Jeff's tag and move the pin in this item. Confirm.
- **a5.** Proceeding with D5: `tools/vz-image-prep`'s forwarder is left as it
  is, named in the Archive summary. Options: (a) leave it (recommended); (b)
  file a follow-up item now; (c) fix it here, which adds the `driver/vz`
  area and blocking macos-vz validation. Confirm.

## Design

### The cause, as a sequence

```
build env (gVisor, random source ports)        relay (gVisor, tcp.Forwarder)
  SYN  :p1 -> 10.100.0.1:80             ->     forwarder -> handler
  GET ..., Connection: close            ->     reply, Close()         (relay closes first)
  FIN/ACK exchanged                            relay's end: TIME-WAIT 60 s on (p1, :80)
  ... n connections later, the RNG picks p1 again
  SYN  :p1 -> 10.100.0.1:80             ->     TIME-WAIT endpoint takes it; no listener
                                               to hand it to, so it is dropped
  SYN retransmitted at 1 s, 3 s, 7 s    ->     dropped, until the TIME-WAIT ends
  the case's dial gives up at 10 s
```

With D1 the relay's end is closed when its close completes, the 4-tuple is
free at once, and the reused port's SYN reaches the forwarder.

### `buildwarden/buildwarden` (this repository)

- `cmd/relay/ingress_fd.go`, `NewFDIngress`: after `stack.New`, set
  `tcpip.TCPTimeWaitTimeoutOption(0)` on the TCP protocol, with a comment
  saying why (D1), and fail the ingress's construction if the option is
  refused. This is the host mode only (RELAY-04): the container and vm modes
  take the build's connections on the kernel's sockets, whose TIME-WAIT
  answers a reopening SYN as RFC 1122 allows and which no case has seen
  stall.
- `cmd/relay/ingress_fd_test.go`: a test that connects twice from one fixed
  source port over a socket pair, the handler closing first, and requires the
  second connection to complete; it fails without the fix (after its bound)
  and passes with it in milliseconds. The client is a small gVisor stack in
  the test, as the throwaway reproduction was.
- Nothing else: no flag, config key, claim field or output changes (no
  RELAY-08 claim field: the value is not a relay setting anyone sets), and no
  reference docs change (UX-01) since no user-facing surface does. FLOW-15:
  no component's flow changes, so `overview.md` does not.

### `buildwarden/buildwarden-specs` (LOOP-04)

What is done there, on `feat/relay-conformance-timeout` from its `main`,
merged by its own pull request to its `main`, which is its trunk for this
item (not the program trunk: the item is released from the FLOW-20 hold and
is not a program item):

- `conformance/netstack/netstack.go`, `Capture`: set the same option on the
  stack that captures, with the same comment, so the harness's in-process
  relay (`conformance/relay/guest.go`, its only caller outside tests)
  conforms to GUEST-6 as the Go relay will. A stack that only dials (the
  build environment's) keeps gVisor's default. `netstack_test.go` gains the
  fixed-port reconnection test.
- `conformance/launch/relay.go`, `startOCI` and `containerPID`: D2, with a
  test in `launch/relay_test.go` that drives `containerPID` with a fake
  runtime (a script that answers `inspect` after a delay) to check the
  bound used and the message and phases a failure prints.
- `conformance/CHANGELOG.md`: a `## v0.2.1` entry naming both, and `go.work`'s
  `replace` raised to it, as the specs repository's README requires of a
  change to `conformance/`. No specification directory changes, so no
  specification's version or `CHANGELOG.md` changes.
- Invariants (LOOP-08 as the specs repository keeps them): its `make check`
  passes, including `TestAgainstTheHarness` for every specification, which
  is the check that failed on its `main`; no requirement text or schema
  changes; no tag is pushed by this item.

### Areas

The home's area is `relay` (`cmd/relay/`), which is in the map's shared set,
so it covers the declared drivers (FLOW-02). `buildwarden-specs` has no area
map in the queue state (`repos/buildwarden/buildwarden-specs/` holds none), so
under FLOW-05 it is one exclusive area and the item collides there with any
other item that names that repository, whatever it declares. With a map, the
item would name `buildwarden/buildwarden-specs:conformance`. This design
changes no map.

## Defaults and overrides

- **The host-mode relay's TCP TIME-WAIT: 0 (gVisor's default, which it had,
  is 60 s).** Obvious for a stack that owns a point-to-point, in-order link
  and must accept every connection the build opens (GLOBAL-09, D1). Nothing
  overrides it: it is not a setting. Not a fallback.
- **The harness's capturing stack's TIME-WAIT: 0**, the same, for the same
  reason; a stack that does not capture keeps gVisor's default.
- **The oci launcher's bound for the runtime to start the container:
  CommandLimit, 60 s (it was StartLimit, 30 s).** The bound the harness gives
  a target's own commands; the runtime is not the component under test. The
  relay's own bound, StartLimit (30 s) from the container running, is
  unchanged. Neither is overridable, as the harness's other timings are not.
- **No merge and no fallback.** D1's rejected listeners-beside-the-forwarder
  and D2's rejected retry would each have been a second path; neither is
  added (GLOBAL-10, GLOBAL-11).

## Cross-driver impact

- **qemu**: its relay runs as a host process in host mode (`--mode=host`,
  `driver/qemu/driver.go`), so a qemu build's relay gets D1. No qemu code
  changes.
- **vz**: its relay's host mode is the same `NewFDIngress`; it gets D1. No vz
  code changes. (`tools/vz-image-prep` is not the relay; D5.)
- **container, hyperv, firecracker**: unaffected. Their relays run in
  container or vm mode, whose connections are the kernel's sockets, not the
  FD ingress (ORCH-01).

## Security note

The boundary is the build environment's to the relay (GLOBAL-06's relay
clause: the relay itself changes). Its posture does not change. What reaches
the relay, what it records (RELAY-02), its SSRF filter, its keys and its
claim are untouched; the change only frees a closed connection's 4-tuple at
once instead of after 60 s. A build could already open as many connections
as it likes; it now cannot be stalled by its own reuse of a port, and gains
nothing else: a segment of an old connection that arrives after the close is
answered with a reset, as one for any closed connection is. The TEST-08 suite
need exercise nothing new; the tier-0 test of the reconnection is the
check.

## Validation plan

- **Tier 0** (TEST-02): the two fixed-port reconnection tests (`cmd/relay`,
  `conformance/netstack`), each failing on today's code and passing with the
  fix; the oci launcher's test with a fake runtime. All deterministic: a
  fixed port, no randomness, wall-clock used only as a failure bound
  (TEST-04).
- **Tier 1**: `conformance.yml`'s `relay` job on the pull request, every
  target, and `buildwarden-specs`'s `ci` (`make check`, whose
  `TestAgainstTheHarness` failed on its `main`).
- **The cause, checked against the record.** Done at Design (Problem): the
  two failing cases, run 300 and 3000 times in process, failed at their
  predicted rates before D1 and never with it. Implement repeats the two
  runs with the committed change (a throwaway loop, not a committed test,
  since tier 0's fixed-port tests are the deterministic check) and records
  the counts in the pull requests; a failure with D1 in place returns the
  item to Design with that finding (FLOW-06).
- **Native**: none, blocking or tracked (FLOW-09). The changed code is one
  gVisor option in platform-neutral Unix code; CI's `relay-host` target runs
  exactly that file on Linux, as macOS builds it for qemu and vz. No tier-2
  gap is deferred.

## Implementation plan

From the design as approved in r1 at e0a1b27. Tasks 1 to 8 are in
`buildwarden/buildwarden-specs` on `feat/relay-conformance-timeout`, whose
trunk is its `main`; tasks 9 to 12 in this repository; 13 and 14 span both.
There is no rework of already-implemented code (nothing of this item is
implemented yet), no approved rule change (`rule_changes: []`), and no
later-phase native step (`native_validation: []`).

1. `buildwarden-specs`: merge its `main` (ab9303c, with #6) into the branch
   before any change, so that `conformance/CHANGELOG.md`'s head is v0.2.0 and
   `go.work`'s `replace` names v0.2.0 when task 7 raises them; a merge, no
   rebase or force-push (LOOP-04, FLOW-17).
2. `buildwarden-specs`, `conformance/netstack/netstack.go`, `Stack.Capture`:
   set `tcpip.TCPTimeWaitTimeoutOption(0)` on the stack's TCP protocol, with a
   comment saying why (D1); a stack that only dials keeps gVisor's default;
   no flag or setting (GLOBAL-06, GLOBAL-09, GLOBAL-11, TEST-05; GUEST-6).
3. `buildwarden-specs`, tier 0 (TEST-02), `conformance/netstack/netstack_test.go`:
   the fixed-port reconnection test of the Validation plan: a client stack on
   the frame link connects from one fixed source port to a capturing stack
   whose handler closes first, then connects again from that port, and the
   second connection must complete; wall-clock only as the failure bound.
   Run it once against today's `Capture` to see it fail at its bound, and
   with task 2 to see it pass in milliseconds (TEST-01, TEST-04).
4. `buildwarden-specs`, `conformance/launch/relay.go`, `containerPID`: bound
   the wait for the container to be running by `CommandLimit` (60 s), not
   `sandbox.StartLimit`; its failure says that the runtime did not start the
   container and that the relay under test was never running; keep the
   `StartPoll` (20 ms) interval, each `inspect` call bounded by what is left
   of the wait. The relay's own bound, `StartLimit` from the container
   running in `sb.WaitListening`, is unchanged. No retry (D2; TEST-04,
   GLOBAL-10, GLOBAL-11).
5. `buildwarden-specs`, `conformance/launch/relay.go`, `startOCI`: record each
   oci start's phases (when `docker run` started, when `inspect` first
   returned the container, when it had a process, when the claim endpoint
   answered); a failed start prints them with the runtime's `inspect` state
   (D2; GLOBAL-11, TEST-04).
6. `buildwarden-specs`, tier 0 (TEST-02), `conformance/launch/relay_test.go`:
   drive `containerPID` with a fake runtime (a script that answers `inspect`
   after a delay) to check the bound it uses and the message and phases a
   failure prints; deterministic, no real runtime (TEST-01, TEST-04).
7. `buildwarden-specs`: a `## v0.2.1` entry at the head of
   `conformance/CHANGELOG.md`, above v0.2.0, naming task 2 and tasks 4 to 5,
   and `go.work`'s `replace` raised to v0.2.1, as its README requires; no
   specification directory, version or `CHANGELOG.md` changes, and no tag is
   pushed: `conformance/v0.2.1` is Jeff's (D3, D4; LOOP-07, FLOW-20).
8. `buildwarden-specs`: `make check` green, `TestAgainstTheHarness` for every
   specification included (its LOOP-08 invariants); commit and push the
   branch (LOOP-01, FLOW-17).
9. `buildwarden`, `cmd/relay/ingress_fd.go`, `NewFDIngress`: after
   `stack.New`, set `tcpip.TCPTimeWaitTimeoutOption(0)` on the TCP protocol
   with a comment saying why (D1), and fail the ingress's construction if the
   option is refused. Host mode only: the container and vm modes are not
   touched; no flag, config key, claim field or output (GLOBAL-06, GLOBAL-09,
   GLOBAL-11, RELAY-04, ORCH-01).
10. `buildwarden`, tier 0 (TEST-02), `cmd/relay/ingress_fd_test.go`: connect
    twice from one fixed source port over a socket pair through
    `NewFDIngress`, the handler closing first, the client a small gVisor
    stack in the test; the second connection must complete. Run it once
    without task 9 to see it fail at its bound, and with it to see it pass in
    milliseconds (TEST-01, TEST-04).
11. `buildwarden`: no reference doc or `overview.md` change is owed, since no
    user-facing surface and no component's flow changes; and no pin moves,
    `tools/conformance/*/go.mod` untouched (D4). Check the branch's diff
    against `main` holds only `cmd/relay/` and this design (UX-01, FLOW-15,
    LOOP-07, FLOW-02).
12. `buildwarden`: `make test`, `make lint` and `GOOS=darwin make lint`
    (`ingress_fd.go` is `unix`); `make conformance` for the relay targets
    (tier 1, TEST-02) as root where the session allows it, else left to the
    pull request's `conformance.yml`; commit and push (LOOP-01, TEST-01,
    FLOW-17).
13. Both: the repeated in-process runs of the Validation plan with the
    committed change, against the harness's in-process host relay: the
    GUEST-46 host case 300 times and the GUEST-34 cwd case 3000 times, in a
    throwaway loop not committed; the counts recorded in both pull requests.
    Any failure with D1 in place returns the item to Design with that
    finding (TEST-04, FLOW-06).
14. Publish: Consolidation and Archive summary (naming D5's
    `tools/vz-image-prep/netfwd.go` and D3's offered fixed-source-port suite
    case); both pull requests, `buildwarden-specs`'s to its `main` and this
    one's to `main`, with no `merge_first`; tier 1 green on each:
    `conformance.yml`'s `relay` job, every target, and `buildwarden-specs`'s
    `ci` (FLOW-08, FLOW-10, FLOW-11, FLOW-12, FLOW-15).

## Design review

- r1, 2026-11-23: approved at e0a1b27. Jeff approved the design, as read at
  630651f, in the attended session (workflow-human).
  - Decisions:
    - a1 (a), TIME-WAIT 0 on both capturing stacks;
    - a2 (a), no spec change or suite case here;
    - a3 (a), a4 (a) and a5 (a), as recommended (batch).
  - Changes folded in, at e0a1b27: the harness's next release is
    `conformance/v0.2.1`, not v0.1.1. buildwarden-specs#6 has since made
    the CHANGELOG's head v0.2.0.
  - Security note read: GLOBAL-06's relay clause, posture unchanged.
  - read at 630651f
  - sitting: 22:06-22:09 UTC, asks: 5 (1 a batch covering a3 to a5)

## Consolidation

Publish, 2026-11-24 (FLOW-10, FLOW-15), round 2, on `buildwarden` at
`a71bdce` and `buildwarden-specs` at `4d95ad8`.

- **What landed on each trunk since the fork.**
  - `buildwarden`: forked from `main` at `168cf95`; `main` at `c52926d` is
    merged into the branch at `a71bdce`. The one change between them is
    #141 (`native-loop-fixes`, a process item: `workflow-native`'s skill,
    `wf native`, `process.md` and the loop profile). It touches nothing
    under `cmd/relay/`, `relay/` or `driver/`, so the merge had no conflict
    and nothing here needed rework.
  - `buildwarden-specs`: forked from `main` at `70457f1`; `main` at
    `ab9303c` (#6, `feat/prerelease-tags`: the `release` workflow and
    `speccheck` accept a program trunk's pre-release tags, and the
    `CHANGELOG.md` head became v0.2.0) is merged into the branch at
    `cd9a82e`, before any change (plan task 1). The branch's
    `conformance/CHANGELOG.md` v0.2.1 entry sits above #6's v0.2.0, and
    `go.work`'s `replace` names v0.2.1, as the specs README asks. #6 does not
    touch `conformance/netstack/` or `conformance/launch/`.
- **Terminology and wording.** The design, code comments and the v0.2.1
  entry use the same terms: "capturing stack", "TIME-WAIT", "forwarder",
  "reused source port", "4-tuple"; and for D2 "the runtime's start" against
  "the relay's own bound, StartLimit". The implementation adds `awaitOCI`
  (the oci start after `docker run`), `ociPhases` and `ociState` to carry
  D2's record of phases and the runtime's state; the Design section named
  only `startOCI` and `containerPID`, which is wording, and the Archive
  summary names all of them.
- **FLOW-15 sweep.**
  - *Dead code.* `make deadcode` on `a71bdce` (linux, darwin, windows):
    nothing in `cmd/relay/ingress_fd.go`; the functions it lists all predate
    the fork (`relay.Relay.CAFingerprint` and `Ledger`, `driver/vz`'s
    cgo-only image-prep, firecracker's and `cmd/relay`'s Linux-only helpers
    seen from windows, `driver/hyperv`'s `runPSJSON`). The `buildwarden`
    branch adds no non-test function. In `buildwarden-specs` every added
    function has a caller: `awaitOCI` and `ociState` from `startOCI`,
    `ociPhases.String` from the failure's report, `containerLimit` from
    `awaitOCI`.
  - *Duplication.* The two fixed-port reconnection tests
    (`cmd/relay/ingress_fd_test.go`, `conformance/netstack/netstack_test.go`)
    each build a small client stack and a `connectFrom`. Kept as two on
    purpose: they are test code in different repositories, against
    different stacks (a socket pair through `NewFDIngress`, and an
    in-memory frame link through `Capture`), and `buildwarden` pins
    `conformance` v0.2.0, so it could not import the harness's helper if it
    were exported. The D1 comment is repeated beside each option for the
    same reason. `ociState`'s `inspect --format {{json .State}}` is a
    different query from `ociInspect`'s `{{.State.Pid}}`, not a copy.
  - *The overview.* `overview.md`'s relay pseudocode names no TCP timers
    and no component's flow changes; unchanged.
- **Seams.** `seams: []`; no specification changes, so no **Specification
  changes** section (LOOP-02). The `conformance` harness's v0.2.1 is a
  release of the harness, not of a specification; its tag is Jeff's, and
  no pin moves here (D4).
- **Review.** Tier 2 at `00026f894c50`: one MUST, TEST-04 in
  `buildwarden-specs` (`TestContainerPID`'s failure cases under a 300 ms
  wall-clock limit), sent back to implement and fixed at `4d95ad8` (each
  case has a one-minute bound; the two timeout cases cancel once the fake
  runtime has given its last answer). The re-review at `a71bdce00e18` has
  no open finding.
- **Rules proposed:** none. **Follow-ups filed:** none. **Regressions a
  trunk already has:** none found beyond the ones this item fixes.
- **`dist/wf check` on `a71bdce`**, its trial-merge lines:

  ```
  ok   trial merge with finch-version-starts-vm (a report, FLOW-10): clean
  ```

- **Gates:** `make lint` and `make test` pass on the consolidation commit.

## Rollback notes

Revert each repository's branch commits on its own; the two pull requests
are independent. Reverting `buildwarden`'s restores gVisor's 60 s TIME-WAIT
in the host-mode relay and the reconnection stall, and nothing else: no
state, format, flag or ledger record depends on it. Reverting
`buildwarden-specs`'s before `conformance/v0.2.1` is tagged needs nothing
more; after the tag, a v0.2.2 would carry the revert, since a released tag
is not moved.

## Archive summary

Filed 2026-11-22 on Jeff's word and released by him from the FLOW-20 hold
(a TEST-04 fix for `main`'s CI). Designed 2026-11-23, design review r1
approved at `e0a1b27` (Jeff); implemented, reviewed twice by tier 2
(`00026f894c50`: one MUST, TEST-04, fixed; `a71bdce00e18`: none open) and
published by the cloud loop 2026-11-24. Two pull requests, `buildwarden`'s
to `main` and `buildwarden-specs`'s to its `main`, neither merged first.

- **Built.**
  - `buildwarden`, `cmd/relay/ingress_fd.go` `NewFDIngress`: the host-mode
    relay's gVisor stack sets `tcpip.TCPTimeWaitTimeoutOption(0)` and fails
    construction if the option is refused. `ingress_fd_test.go`
    `TestFDIngressTakesAReusedSourcePort` connects twice from one fixed
    source port over a socket pair and fails at its bound without the fix.
  - `buildwarden-specs`, `conformance/netstack` `Stack.Capture`: the same
    option on the harness's capturing stack (the in-process relay's); a
    dialing stack keeps gVisor's default. `TestCaptureTakesAReusedSourcePort`
    pins it.
  - `buildwarden-specs`, `conformance/launch/relay.go`: the oci launcher's
    `containerPID` waits up to `CommandLimit` (60 s, `containerLimit`) for
    the runtime to start the container and says, on failure, that the
    runtime did not start it and the relay under test was never running;
    the relay's `StartLimit` still runs from the container running.
    `awaitOCI` records `ociPhases` (run started, first listed, running,
    claim endpoint answered) and a failed start prints them with
    `ociState`, the runtime's inspect state. `TestContainerPID` and
    `TestOCIStartReport` drive it with a fake runtime.
  - `conformance/CHANGELOG.md` v0.2.1 and `go.work`'s `replace` at v0.2.1;
    no tag pushed.
- **Verified.** The two failing cases run repeatedly against the harness's
  in-process host relay with the committed change: GUEST-46's host case
  0 failures in 300 runs and GUEST-34's cwd case 0 in 3000, against 11/300
  and 4/3000 before D1.
- **Decisions taken:** TIME-WAIT 0 on both capturing stacks (a1, D1); no
  specification change and no new suite case (a2, D3); the oci runtime's
  start bounded and reported apart from the relay's (a3, D2); no pin move
  (a4, D4); `tools/vz-image-prep` left as it is (a5, D5).
- **Rejected:** a short TIME-WAIT; listeners beside the forwarder; a
  patched gVisor; a harness that never reuses a source port; a retry or a
  longer `within`; only raising StartLimit; retrying `docker run`;
  pre-created containers.
- **Left open.**
  - D5: `tools/vz-image-prep/netfwd.go` captures a VM's connections with a
    gVisor forwarder too and has the same latent 60 s stall on a reused
    4-tuple. Left as it is (the `driver/vz` area, never seen to fail).
  - D3: a `relay-guest-api` suite case that reconnects from a fixed source
    port would catch the defect in any relay. Offered as a follow-up, filed
    only if Jeff wants it; it needs a release of that suite.
  - The harness's two changes reach `buildwarden`'s CI only once Jeff tags
    `conformance/v0.2.1` and the pin moves (D4).
  - The relay-oci conformance target runs only in the pull request's CI
    (`conformance.yml`), not in a cloud session, so D2's bound and report
    are checked there and by tier 0's fake runtime.
- **For future designs touching `relay`, `host-mode`, `fd-ingress`,
  `netstack`, `gvisor`, `tcp-time-wait`, `tcp-forwarder` or the
  `oci-launcher`:** a gVisor `tcp.Forwarder` never sees a SYN that reopens
  a TIME-WAIT 4-tuple, so any stack that captures with a forwarder must
  keep no TIME-WAIT (or have a listener), or it stalls a reconnection for
  60 s. The harness's own timings must not be charged to the component
  under test: bound the runtime's start by `CommandLimit` and the relay's
  by `StartLimit`, and report which ran out.
