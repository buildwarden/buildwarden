# Behavioral lint: driver-contract @ dd370cbbf4f4

- **Branch:** `feat/driver-contract`. **Head:** `dd370cbbf4f4`. **Base:** merge base `480e205c969f`. `origin/main` is now `85317906854d`; Publish merges it (FLOW-10).
- **Heads:** `buildwarden/buildwarden` at `dd370cbbf4f4`.
- **Scope:** `git diff origin/main...feat/driver-contract`: 191 files, +35512/-2901, 94 commits (LINT-04).
- **Design:** `docs/design/driver-contract.md`. r3 was approved at `33a3563`; the rule changes were approved at `a6c8f0b`.
- **Previous report:** none. This is the first report for this item.
- **Model:** claude-opus-5-5. Tier 2 is **report-only** (LINT-07): these findings inform the human sign-off (FLOW-08) and never block on their own (LINT-03).
- **Rule index:** 100 rules from `docs/development/*.md` (44 agent-judgment, 42 hybrid, 14 automated).

## Tier 1 (deterministic): red

| Check | Result |
|---|---|
| `make lint` | pass: golangci-lint 0 issues x3; tier1 0 MUST, 0 MUST-review, 0 SHOULD, 5 hints |
| `make test` | pass |
| `GOOS=windows make lint` | pass |
| `GOOS=darwin make lint` | pass |
| `make enforcement` (FLOW-14) | **FAIL**, exit 2 |
| `dist/wf check` | rule changes cleared; queue matches the design; diff inside the declared areas (see below) |
| `wf check` built from the merge base `480e205` | same lines as `dist/wf check` |

**`make enforcement`** prints four lines:

```
FAIL main's tier 1 findings kept (FLOW-14): dropped, and no approved rule change names UX-03: UX-03 surface-removed cmd/warden/config.go:0: key "cli" was removed with no alias or deprecation; ...
(the same for "relay_image", "windows_image", "windows_image_update")
wf: the branch drops main's tier 1 findings without an approved rule change
```

**The cause is main's code, not the branch.** UX-03 is approved: "rule change UX-03 approved at `a6c8f0b`: enforcement only", with paths `tools/tier1/surface/*`. The drop is exactly what that change does: the surface reader now reads a driver Setting's `Aliases` as keys, so it finds the seven renamed keys under their old names.

`wf drops` loses the approval in `tools/workflow/check/rulechange.go`:

- `cleared()` (lines 75-108) builds a map keyed by path, with one rule per path.
- `approvedRules()` (lines 331-356) collects that map's values.
- When several approved rule changes list the same path, only the last one survives.

`dist/wf check` itself prints `tools/tier1/surface/surface.go (ORCH-03)`. UX-01, UX-03 and ORCH-03 share that path, so UX-01 and UX-03 are lost. The same happens elsewhere:

- `orchestrator.md`: only ORCH-03 survives of ORCH-01 to ORCH-04.
- `rules.yaml`: only TEST-05 survives of GLOBAL-08, ORCH-01 and TEST-05.

The branch does not touch `tools/workflow/`. `enforcement-check.sh` builds `wf drops` from the merge base, so a fix on this branch cannot clear it. See **Follow-up** below.

**`dist/wf check`** (before this report; lint and test skipped without `-run`):

```
ok   checkout is the clean branch head: HEAD dd370cbb..., feat/driver-contract dd370cbb...
ok   rule changes cleared (FLOW-14): cleared: docs/development/bootstrap.md (BOOT-02), docs/development/global.md (GLOBAL-08), docs/development/loop-profile.md (PROFILE-01), docs/development/orchestrator.md (ORCH-03), docs/development/testing.md (TEST-05), docs/development/ux-cli-config.md (UX-08), tools/tier1/rules.yaml (TEST-05), tools/tier1/surface/surface.go (ORCH-03), tools/tier1/surface/surface_test.go (ORCH-03), tools/tier1/surface/testdata/driver.go.txt (ORCH-03)
ok   queue records what the design declares (FLOW-03): as declared at 028ffe2
ok   diff inside the declared areas (FLOW-03): undeclared: the item collides with every item in its repositories, and no path is outside it (FLOW-05)
```

**`tools/tier1/area-edges.sh | dist/wf areas -state .dev/workflow -check -rev HEAD -edges -`:** 25 differences at `dd370cb`. The merge base `480e205` has none.

- 19 `catch-all` lines: every file of `conformance/driver/`, and `cmd/warden-driver-container/main.go`, "is in no area's paths (it falls to core)".
- 6 `reaches` lines: `core lacks collector, driver-core, driver/container, executor, ledger, relaycfg (cmd/warden-driver-container depends on ...)`.

The item is undeclared, so no path is outside its areas and nothing is widened. See the informational FLOW-05 note.

**Trial merge (FLOW-10):** recorded under **Trial merge** at the end, from `dist/wf check -run` after this report was written.

## Counts

| Severity | Count |
|---|---|
| MUST | 2 |
| MUST-review | 1 |
| SHOULD | 0 |

## Findings

### 1. FLOW-06 (MUST): a blocked doctor prints the driver's "ok" before exiting 1

**Where:** `cmd/warden/host.go:417`, and `host.go:419`, `driver/container/command.go:136`, `cmd/warden/upstreamca_test.go:281,294`. Related: UX-09, UX-04.

**What happens:** warden's upstream-CA check blocks, and `doctorBinary` still runs the container driver's doctor. That doctor prints `container runtime: <rt>` and `ok` (command.go:136) straight to stdout. Only then does warden exit 1. A driver binary that lists no doctor does the same: `host.go:419-423` prints `ok` before it exits 1 when blocked. `main` printed no `ok` when blocked.

**Why it is a finding:** the approved design keeps this behaviour. Its Behaviour section says "Exit codes (UX-09): ... and 1 for a blocked doctor with no extra line" (design line 4392). The change is not among the nineteen accepted changes. Implement kept it and changed TestUpstreamCALinesOfDoctor's two blocked cases to expect `ok`, so the test now holds the deviation. An `ok` line followed by exit 1 also contradicts itself, for a user and for a script.

**Fix (no design change needed):** DRV-25 says only that doctor reports on stdout and returns `blocked`. It does not say who prints `ok`, so make `ok` warden's:

1. Drop `ok` from the container driver's doctor (`driver/container/command.go:136`).
2. Have `doctorBinary` print `ok` only when neither warden's part nor the driver's result is blocked, and not at `host.go:419` when blocked.
3. Restore the blocked cases of TestUpstreamCALinesOfDoctor to `main`'s expectation.

An unblocked doctor still prints `container runtime: <rt>` then `ok`, as the macos-finch validation expects. To keep the line instead, the change goes back to Design (FLOW-06) to add it to Behaviour.

### 2. TEST-04 (MUST): a signal test orders its events with a 10 ms sleep

**Where:** `cmd/warden/buildseq_test.go:316`.

**What happens:** TestBuildSequenceSignalBeforeTheRelay sends `os.Interrupt` into a buffered signals channel (capacity 1, line 179), sleeps 10 ms, then opens the gate, and the driver sends relay-started. On a loaded or `-race` runner, the loop may not yet have taken the signal when the gate opens. Its `select` then sees the signal and relay-started both ready and picks either one. If it picks relay-started first, the claim goes ahead and the test fails.

**Fix:** wait for the signal to be handled instead of sleeping. Either:

- have the test's `stopSignals` close a channel, and wait on it before `close(gate)`; or
- make this case's signals channel unbuffered, so the send returns only once the loop has taken the signal.

### 3. GLOBAL-06 (MUST-review): two security boundaries change

**Where:** `cmd/warden/driverlookup.go:17`, `cmd/warden/buildseq.go:362`, design Security note (line 6407). Related: TEST-08, FLOW-08.

The design declares both changes (`security_boundary: host-env`, and env-relay):

- **host-env:** the code that builds the isolation is now chosen at run time by a lookup: PATH, then beside warden, then linked. There is no ownership or mode check (a16).
- **env-relay:** warden now makes the claim token and claims every driver's relay.

The design's Security note covers both. In the code read, the one kept guard works as the note says: a config file names a driver, never a file (`findByPath`, `sourceConfig`). Relative or empty PATH entries are errors, Windows takes only `.exe`, and a non-executable file is skipped and listed.

**Fix:** none. The human acknowledges this at merge (FLOW-08), against the design's Security note.

## Follow-up (proposed, not filed)

**`wf-drops-shared-rule-paths`.** `wf drops` names only the last approved rule for a path that several approved rule changes share. This is the cause of the red `make enforcement` above.

**Why not filed:**

- It is the enforcement of a loop rule (FLOW-14) in `tools/workflow/check`. LOOP-09 therefore makes the fix a process item. The kind and FLOW-20's hold need Jeff's agreement (workflow-intake step 3).
- A feature follow-up (`-follows driver-contract`) would start only once this item is done. `make enforcement` stays red until the fix is on `main`, because `enforcement-check.sh` builds `wf drops` from the merge base. The two would deadlock.

**Jeff decides:** a process item ahead of this one, or a recorded dismissal of the FLOW-14 failure at merge.

**Suggested fix:** in `approvedRules`, collect every rule whose approval clears, not the values of the path-keyed map. Or have `cleared` return path -> set of rules.

## Informational (not counted)

- **FLOW-05, the area map.** The r3 review recorded that `core` gains `reaches: [collector, relaycfg]`, to be saved at merge. `wf areas -check` also reports:
  - `cmd/warden-driver-container/` in no area;
  - `core` lacking driver-core, driver/container, executor and ledger through it.

  Before saving the map, place `cmd/warden-driver-container/` (for example with driver/container) or widen core's reaches. That is Jeff's call, at merge.
- **FLOW-14, rule-change wording.** Every sentence the branch adds to the six rule files is verbatim in the design as approved. The one exception is loop-profile's Specifications row, which keeps `main`'s later specs-repository sentences unchanged. The `rules.yaml` edits match the approved enforcement text: the `seams` entry, `drivers.shared`, and TEST-05's owner. The `surface/` edits are the approved UX-01, UX-03 and ORCH-03 enforcement.
- **FLOW-06, Implement's other recorded deviations.** Judged, and none needs Design:
  - DRV-C5's exception placed after the ignoring clause.
  - hyperv's windows-image keys listing `run`.
  - container's `relay_image` listing `run` and `doctor`; `main`'s doctor checked the relay image.
  - linked drivers listing `reap`.
  - help titles.
  - image list's end-to-end case runs in process.
  - the signal and shell cases are Unix-only; native validation covers them.
  - `WARDEN_FAKE_TEARDOWN`, read by the fake only.
  - the drivercmd exit-code order fix (DRV-21).
  - make build of warden-driver-container moved to task 22.
  - integration skips shell and exited; macos-finch case 9 runs both tiers.
  - DRV-33 and DRV-55 named by the probe and clean cases.
  - the fake writes its relay PID.
- **TEST-05.** The Inputs case was not run against a real relay here. The integration workflow runs it against warden-driver-container with the real relay, so confirm that run is green on the PR.
- **FLOW-15.** The old-name table and common keys are written twice: `cmd/warden/drivercall.go:290-303` and `driver/drivercmd/settings.go:157-180`. This is probably deliberate, since warden checks a driver against the specification and not the kit. It belongs in the duplication sweep, and a test that the two tables agree would keep them in step.
- **GLOBAL-04.** tier1 gave five tests-with-change hints. Four are the suite's test doubles and CLI, exercised under `make test`, plus a one-line `main`. The fifth is `driver/linuxguest`. These are hints, not findings.
- **ORCH-04.** warden closes the driver's input straight after `stop`. The kit's `pipeHost.Wait` can then return `interrupted` in place of the stop's reason when both are ready. The container driver ignores the value and its teardown takes no context, so nothing changes today.

## Judged clean

- **GLOBAL-09 to GLOBAL-11**, against **Defaults and overrides**:
  - the teardown wait: 60 s, `[build] teardown_timeout` refused at load, and the default never used in its place;
  - the driver lookup order;
  - the relay source: beside the binary, else the source tree, and no fallback after a failure;
  - `default_image_os`, which is in v1.md.

  No unlisted default, merge or fallback was found in the non-test diff.
- **RELAY-08.** No new relay setting. The claim token moves from the driver to warden and reaches the relay as before (`relaycfg.TokenEnv`, pre-claim).
- **ORCH-04.** A driver binary tears down on stop or on the end of its input. warden kills and reaps after the teardown wait, and a failed reap leaves the record and lock for `warden clean`.
- **UX-08.** A driver's keys are read by the driver that owns them, in its probe. warden's own keys are read at load.
- **PROFILE-01.** The four changed rows match the change.

## Suppressions

None.

## Trial merge (FLOW-10)

After this report was written, `dist/wf check -state .dev/workflow -feature driver-contract -repo <checkout> -run` exits 0 and prints **no trial-merge line** at phase 4. Its lines are all `ok`:

- clean head;
- `make lint`;
- `make test`;
- report `.md` and `.json`;
- report heads (LINT-04);
- rule changes cleared;
- queue matches the design;
- diff inside the declared areas.

It does not run `make enforcement`, so its exit 0 does not include the FLOW-14 failure in Tier 1 above.
