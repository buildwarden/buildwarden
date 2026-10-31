---
feature: native-host-loop
status: draft
round: parallel
drivers: []
seams: []
surfaces: []
security_boundary: none
alters_relationships: false
native_validation:
  - {host: macos-vz, blocking: true, reason: "The launchd install and one gated run can only be checked on the Mac."}
  - {host: windows-hyperv, blocking: true, reason: "The Task Scheduler install under Git Bash can only be checked on the Windows desktop."}
tier2_gaps: []
rules: [FLOW-04, FLOW-08, FLOW-09, FLOW-13, FLOW-14, FLOW-16, FLOW-17, FLOW-18, TEST-02, TEST-03]
concepts: [workflow, native-validation, scheduled-sessions, hosts, gate]
related: [workflow-bootstrap.md]
---

# Native-host loop: scheduled native sessions on the Mac and the Windows desktop

## Problem

The daily cloud tick (FLOW-13) can only take `cloud` steps. Work that needs
native hardware (FLOW-09) waits until Jeff sits down at a machine and runs
`/workflow-native <host>`, so the native-gap backlog only shrinks by hand, and
a feature that blocks on a `host:` step waits for a free evening. On
2026-10-29 the backlog held eight open gaps across five host names, and
container-inputs was close to merge with a `make integration-test` the cloud
cannot run (no container runtime, no outbound network).

Two machines can do this work: Jeff's MacBook (arm64, sometimes asleep or
away) and an always-on Windows desktop. Cowork scheduled tasks are not the
answer: their "this computer" shell is a Linux VM, which has no
Virtualization.framework, Hyper-V, HVF/WHPX-accelerated qemu or codesign.
Claude Code has to run natively on each machine.

## Prior art

`workflow-bootstrap.md` set up the queue, the `host:<name>` affinity
(FLOW-04), the native-gap backlog and the attended `workflow-native` skill.
This design adds the scheduler and a gate in front of that skill. No archived
design covers native scheduling.

## Decisions

- **One scheduled session per machine**, not one per host name. A machine
  serves several host names. `hosts.yaml` on the workflow branch lists them,
  and each host name belongs to exactly one machine. `wf validate` enforces
  this, and also checks that every pending host step and open gap names a host
  some machine serves. Routing `qemu-any` to another machine is then a state
  edit, not a code change.
- **Host names say how specific the work is** (FLOW-04). A name of the form
  `<os>-<driver>` is platform-specific (`macos-vz`, `windows-hyperv`). A name
  of the form `<capability>-any` runs on any machine that has the capability
  (`qemu-any`, `container-any`) and goes to the always-on Windows desktop.
  The Mac keeps only Mac-specific work:
  - `macbook`: macos-vz, macos-qemu, macos-finch.
  - `windows-desktop`: windows-hyperv, qemu-any, container-any.
- **A zero-token gate, `wf gate`.** Before any agent starts, the wrapper runs
  `wf gate -machine M`. It exits 0 in either of two cases: a host step the
  machine serves is ready, or the machine has open backlog gaps and something
  has moved since the last finished session. "Moved" means the fingerprint of
  `origin/main` and the in-progress features' branches has changed, or
  `-sweep-days` (default 7) have passed. Otherwise it exits 3 and no agent
  starts. The stamp is recorded from the state the session started from, so a
  push during a session triggers the next one. Rejected: an agent that checks
  for work itself, which spends tokens every idle night; and shell parsing of
  YAML, which is fragile and would have to be written twice.
- **One wrapper for both machines**, `tools/workflow/native/native-tick.sh`,
  run by launchd on the Mac and under Git Bash on Windows (Claude Code on
  Windows already needs Git Bash). The wrapper:
  - takes a per-machine lock;
  - skips the run when the Mac is on battery or the disk has less than 40G
    free;
  - refuses to run if the dedicated checkout is dirty;
  - resets that checkout to `origin/main` and the workflow worktree to
    `origin/workflow`;
  - runs the FLOW-18 check, builds `wf` and runs the gate;
  - only then runs `claude -p` on `workflow-native unattended <machine>`;
  - afterwards puts the checkout back on `main`, whatever the session did.
- **Installers that check before they install.** `install-macos.sh` and
  `install-windows.ps1` each take `check`, `install` and `uninstall`. `check`
  changes nothing. It reports tools, push access, the machine's entry in
  `hosts.yaml`, free disk, and per-host capabilities: qemu, finch, codesign,
  Hyper-V without elevation, and a working container runtime. `install`
  refuses while `check` reports a problem.
- **Schedules.**
  - Mac: daily at 04:40 local, after the 02:55 cloud tick. launchd runs a
    missed slot on wake. The installer prints the
    `sudo pmset repeat wakeorpoweron` line rather than running sudo itself.
  - Windows: every 6 hours from 04:10. The gate makes idle runs free, so the
    always-on machine can also pick up host steps raised by attended daytime
    sessions.
- **Least privilege that still works.**
  - `settings.native.json` is passed with `--settings`. It allows the tools
    native work needs and denies pushes to `main`, force-pushes, remote
    deletes, `gh` merges, approvals and API calls, `sudo`, and web access.
  - The Windows task runs as Jeff, only while he is logged on, without
    elevation. Hyper-V access comes from the "Hyper-V Administrators" group.
  - This is a guard against accidents, not a sandbox: `make`, `go` and
    `powershell` can run anything. The dedicated checkout, the skill's Never
    list and FLOW-08 are the real boundary.
- **The skill grows an unattended mode** instead of a second skill (FLOW-14):
  - one worker per host;
  - the batch bounded by `run_budget.steps`;
  - host-affinity design and implementation steps dispatched to the phase
    agents as the tick does;
  - no `human` or `cloud` steps and no waiting on questions;
  - a `## Run host:<machine>/<ts>` section in the day's digest;
  - a push notification only when something moved or failed.
- **This change merges as one attended PR, outside the queue**, as
  workflow-bootstrap did. The loop may not edit `.claude/` or `tools/workflow/`
  unattended (FLOW-14).

## Questions

### Blocking

None.

### Assumed

All six confirmed by Jeff on 2026-10-29, in the Cowork session that planned
`native-loop-followups.md`:

- **Confirmed:** `qemu-any` and `container-any` on the Windows desktop. If its
  first `check` shows no qemu or no container runtime, that host name moves to
  the `macbook` in `hosts.yaml`.
- **Confirmed:** re-homing `container-inputs-linux-vm-guests` to `qemu-any`,
  adding `container-inputs-integration-test` (`container-any`), and leaving
  `qemu-windows-guest-package` and `qemu-linux-exclusive-trust` on
  `macos-qemu`.
- **Confirmed:** the schedules: the Mac daily at 04:40, Windows every 6 hours.
- **Superseded:** the native batch counting against `run_budget.steps`. Jeff
  chose a separate `native_budget` instead (`native-loop-followups.md`, item
  3).
- **Confirmed:** notifications through PushNotification when available,
  otherwise the digest. A morning summary task now adds one daily
  notification (`native-loop-followups.md`, item 5).
- **Confirmed:** this change merged outside the queue (PR #64). It is
  retro-recorded as a done process item (`native-loop-followups.md`, item 6).

## Design

```
 cloud tick 02:55 PT --> host: steps / gaps on the workflow branch
                                     |
   Mac launchd 04:40 ----------+     |     +---- Windows Task Scheduler, every 6 h
   (missed slot runs on wake)  |     |     |     (Git Bash)
                               v     v     v
              tools/workflow/native/native-tick.sh <machine>
                 lock, power/disk, clean checkout, reset to origin
                 flow18-check, build dist/wf
                 dist/wf gate -machine M ------ exit 3: idle, no agent, stop
                               | exit 0
                 claude -p "workflow-native unattended M"
                     --settings settings.native.json
                    per host in hosts.yaml: wf next/claim -> test or phase agent
                    -> wf done / release / back -> push workflow
                    digest "## Run host:M/<ts>", notify if something moved
                 checkout back to main; stamp the gate
```

## Cross-driver impact

None at runtime: no driver, seam or user-facing surface changes. The work this
loop runs exercises the vz, qemu, container and hyperv drivers, as attended
native sessions already do.

## Security note

No product security boundary changes. Operationally, an unattended agent runs
on Jeff's own machines with his git credentials. The mitigations:

- A dedicated checkout that is reset on every run.
- A deny list for trunk, force and delete pushes and for merges (FLOW-08,
  FLOW-17).
- No sudo and no elevation.
- Cleanup of what the session creates (FLOW-16).
- FLOW-18 checks at the start of each run.

`claude` stays logged in on both machines, as it already is for attended use.

## Validation plan

- Tier 0: `tools/workflow` unit tests cover the gate:
  - a ready host step runs it, and a leased one does not;
  - a first sweep, an idle re-check, the 7-day re-sweep and a moved `main`;
  - done gaps are ignored;
  - the stamp round trip;
  - `hosts.yaml` validation, including duplicate hosts and unserved steps and
    gaps;
  - loading the optional files.

  `shellcheck` passes on both shell scripts. The gate was also run against the
  real workflow state: a first sweep for both machines, then idle after
  `-record`.
- Native, blocking:
  - On the Mac: `install-macos.sh check`, then `install`, then one
    `launchctl kickstart` run. It drains at least one macos-* gap or records
    why it could not.
  - On Windows: `install-windows.ps1 check`, then `install`, then one
    `Start-ScheduledTask` run, which takes the container-inputs integration
    test.
  - In each digest section: Entire recorded the session, and no push other
    than to `workflow` and `feat/` branches.

## Implementation plan

1. `wf gate`, the `hosts.yaml` model and validation, with tests (FLOW-04,
   FLOW-09).
2. `tools/workflow/native/`: the wrapper, both installers and
   `settings.native.json`.
3. The `workflow-native` skill's unattended mode, and the tick digest's
   per-machine "Waiting on a host" (FLOW-14).
4. `process.md` FLOW-04 host names and FLOW-09 scheduled sessions;
   `structure.md`; the development README.
5. Workflow branch: `hosts.yaml`, the README layout row, and the two backlog
   changes above.
6. Jeff, on each machine: a dedicated clone, `check`, `install`, one manual
   run.

## Rollback notes

Run `install-macos.sh uninstall` and `install-windows.ps1 uninstall`, plus
`sudo pmset repeat cancel` if a wake was set. Deleting `hosts.yaml` turns its
validation off. `wf gate` is only called by the wrapper.

## Archive summary
