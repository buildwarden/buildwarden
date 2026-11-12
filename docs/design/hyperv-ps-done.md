---
feature: hyperv-ps-done
status: active
drivers: [hyperv]
seams: []
surfaces: []
security_boundary: none
alters_relationships: false
design_review: skip
native_validation: []
tier2_gaps: []
rules: [TEST-01, FLOW-15, GLOBAL-11]
rule_changes: []
concepts: [hyperv, powershell, runps]
related: [hyperv-relay-per-build-channel.md]
---

# Every state-changing Hyper-V script ends on DONE

## Problem

hyperv-relay-per-build-channel (PR #115) found that `powershell -Command`
exits with the success of a script's last statement. A script that ended on
a suppressed lookup that found nothing exited 1 with no error text, which
broke every `warden setup` after the first. That feature made the slot script
and EnsureNetwork's script print `DONE` last, checked with `psDoneCheck`.

Six older scripts still end on `'OK'`:

- TeardownNetwork, CreateVM, RemoveVM and CreateDiffDisk in
  `provision_local_windows.go`;
- the relay VM script (`relayvm.go`);
- the seed ISO script (`winseed_windows.go`).

Only the seed ISO's caller checks the marker, and loosely:
`strings.Contains(out, "OK")`. Each works today, but two conventions do the
same job, and five scripts would accept output from a run that stopped early.
Consolidation r5 of that feature left them for a change of their own.

## Prior art

The `DONE` convention, `psDoneCheck` and `psError` from
hyperv-relay-per-build-channel (tasks 61 and 62), stated in `overview.md`.
This applies it to the remaining scripts unchanged.

## Decisions

1. **One marker.** Each of the six scripts prints `psDone` last. RemoveVM's
   early return for a missing VM prints it too (`'gone'; 'DONE'; return`), so
   "already gone" stays a success.
2. **One way to run them.** A new `runPSDone` is `runPS` followed by
   `psDoneCheck`. The six callers use it, and the seed ISO's `Contains("OK")`
   check goes. Rejected: checking inside `runPS` itself. Read-only probes
   (VMState, ListVMs, the doctor probes) print data, not a marker, and must
   not change.
3. **No behavior change on success.** A script that runs to its end prints
   the marker and passes. One that stops early now fails, naming what it
   printed, instead of being taken as done.

## Questions

### Blocking

None.

### Assumed

None.

## Design

Each script ends with `'%s'` formatted with `psDone`, and its caller calls
`runPSDone(script)`. `runPSDone` runs `runPS`, returns its error if any, and
otherwise returns `psDoneCheck(out)`.

## Defaults and overrides

None.

## Cross-driver impact

Only hyperv. No other driver uses runPS.

## Validation plan

- **Tier 0:** the relay VM script's last line is the marker. Windows-tagged,
  against real `powershell.exe`: `runPSDone` refuses a script without the
  marker, naming its output; accepts one with it; and accepts the early-return
  shape RemoveVM uses.
- **Lint:** `make lint` on Windows, plus GOOS=linux vet of driver/hyperv.
- **Tier 2, not blocking:** on windows-desktop, after the change, a build
  through the service and a `--keep-vms` build cleaned with
  `warden clean --driver hyperv`. These exercise CreateVM, CreateRelayVM,
  CreateDiffDisk, the seed ISO, RemoveVM and TeardownNetwork.

## Implementation plan

1. `driver/hyperv/powershell.go`: `runPSDone`.
2. The six scripts end on `psDone`, and their callers use `runPSDone`. The
   seed ISO's `Contains("OK")` check is removed.
3. `overview.md`: the runPS paragraph names every script that uses the
   convention (FLOW-15).
4. Tests as above.

## Design review

Skipped (FLOW-19): no seams, no relationship change, no security boundary, no
blocking native validation, no assumed questions, no rule changes.
