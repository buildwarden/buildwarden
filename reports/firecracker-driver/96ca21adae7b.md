# Behavioral linter: firecracker-driver @ 96ca21adae7b

- Round `feature`, step `f4-review`, second pass. Previous report: [`0c800385066e`](0c800385066e.md).
- Branch `feat/firecracker-driver`, base `6899b79b91a0` (merge base). `origin/main` is still `e418871`.
- Scope: `git diff origin/main...feat/firecracker-driver`: 90 files, +12237/-434 (LINT-04). This pass also checked the fix range `0c80038..96ca21a` (a969a0d..96ca21a, 11 commits) one finding at a time.
- Model: claude-opus-5-5. Tier 2 is report-only (LINT-07). Findings are advisory and never block on their own (LINT-03).

**Counts:** MUST 0, MUST-review 0, SHOULD 4 (was MUST 1, MUST-review 1, SHOULD 10). Suppressions: none carried over (the previous report had none; tier 1 applied no `Tier1-Allow` trailers).

**Resolved:** 1, 2 (with a residual, finding 13), 3, 4, 5, 7, 8, 9, 10, 11. **Carried forward:** 6, 12. **New:** 13, 14.

**Back to Design:** no.

## Tier 1 (re-run by the linter at 96ca21a): green

| Check | Result |
|---|---|
| `make lint` (windows host) | pass: 0 issues in the root module, `tools/workflow` and `tools/tier1`. Then `dist/tier1 -base origin/main`: exit 0 |
| `GOOS=linux make lint`, `GOOS=darwin make lint` | pass, same tier-1 result (FLOW-17) |
| `make test` | pass: 29 ok lines, no FAIL |
| Linux tier 0 | Cross-compiled linux/amd64 test binaries for `driver/firecracker`, `driver`, `driver/linuxguest`, `driver/relayvm`, `tools/firecracker-vm` and `cmd/warden`, run under WSL: all PASS. `TestWaitBuild*`, `TestSetup*` and the pid-file tests also passed `-count=50 -cpu=1`. |
| Custom static checks: `dist/tier1 -json -base origin/main` | exit 0: no findings, no suppressions, 1 hint (BOOT-07 bootstrap-surface, `driver/firecracker/deliver_linux.go`), as before. See finding 6. |

No fix was needed and no branch commit was made.

## Previous findings

| # | Rule | Was | Now | How |
|---|---|---|---|---|
| 1 | LINT-08 | MUST | resolved | a969a0d untracks `cmd/warden/warden.exe`; `.gitignore` gains `warden*.exe`. The blob is still in branch history (see Informational). |
| 2 | GLOBAL-06 | MUST-review | resolved, residual in 13 | c069b25: setup refuses while a slot has a TAP with carrier 1 or its lock held, holds the free slots' locks to the end, and skips the address flush when the /30 is already right. Tested. |
| 3 | UX-04 | SHOULD | resolved | 89e7d34: under sudo, doctor checks the build user's kvm group and pid files, creates no lock file, and skips reflink. |
| 4 | UX-04 | SHOULD | resolved | 0e500f4: the slot is taken before staging. |
| 5 | FLOW-06 | SHOULD | resolved | b0bef63: Implementation notes record each departure with a reason. One sentence overstates the finding-2 fix (finding 13). |
| 6 | BOOT-07 | SHOULD | carried forward | Still true. |
| 7 | TEST-02 | SHOULD | resolved | 2849826 widens the path filter. |
| 8 | GLOBAL-05 | SHOULD | resolved | df7a581 pins and hashes busybox-static and the guest config, and fixes the CI comment. |
| 9 | ORCH-04 | SHOULD | resolved | 5262866: `warden clean` reaps `*.raw.tmp` whose convert lock is free. |
| 10 | ORCH-04 | SHOULD | resolved | 98b0028: exe goes last in the record; an unreadable `/proc` fails the VM start. |
| 11 | TEST-04 | SHOULD | resolved | 937d3db: the grace is an injected channel, and the test orders its events. |
| 12 | TEST-08 | SHOULD | carried forward | Still true. |

## Tier 2 findings

### SHOULD

6. **BOOT-07** (FLOW-14), `docs/development/bootstrap.md:195`. *Carried forward unchanged.*
   - **What's wrong:** the tier-1 hint is acceptable: the firecracker row is a model change accepted at review r1 (a2), and the doc edit moved to the process item `bootstrap-firecracker-row` (priority 17, x1-design still pending with Jeff). Until that item lands:
     - bootstrap.md still names `driver/qemu/cloudinit.go`, which this branch deletes;
     - `rules.yaml` already has the new surface, so the doc that wins and the machine list disagree;
     - bootstrap.md has no firecracker row.
   - **Suggested fix:** say this in the PR body. Either Jeff takes the process item right after merge, or the path correction (correcting the doc to match the code, not a model change) is folded in here.
12. **TEST-08** (SEC-03, SEC-01), `docs/design/firecracker-driver.md:1052`. *Carried forward unchanged.*
   - **What's wrong:** plan step 13 (add the 18 TEST-08 cases to the corpus) cannot be done yet, because no corpus exists. Nothing outside the design tracks the cases: `queue/security-suite.yaml` does not mention firecracker.
   - **Suggested fix:** add the case table to `queue/security-suite.yaml` or the backlog. In the PR body, note that the shipping release trips GLOBAL-06.
13. **GLOBAL-06** (ORCH-04, FLOW-06), `driver/firecracker/setup.go:227`. *New: the residual of finding 2.*
   - **What's wrong:** the race Implement noted is real, and wider than "a user's first build".
     - `quiesceSlots` treats a slot with no lock file as free and holds nothing for it. `acquireSlot` skips the slots setup holds and creates and takes the first slot with no file.
     - So a build that starts while setup runs lands on exactly the slots setup cannot hold. These are slots never used since they were created: those above the host's peak concurrency, those added by growing `--slots`, or every slot when the lock dir does not exist yet.
     - That build can boot its VMs while `flushSteps` has removed the WARDEN-FC jumps and the bridge table. This is finding 2's window, for as long as the two overlap. Taking the slot before staging (finding 4) makes this more likely when the raw image is cached and reflink works. Preflight does not check the rules, because that needs root.
     - The race gets wider in two cases:
       - `ownerLockDir` guesses the lock dir. A user whose shell sets `XDG_CACHE_HOME`, under a sudo that resets the environment, has every lock file "missing".
       - `slots.sh`'s `idle()` checks carrier only, so it misses a build that holds its lock but has not started its VMMs.
     - The Implementation note (`firecracker-driver.md:1224-1227`) calls it first-build-only and says refusing "leaves no window", which overstates the fix.
   - **Suggested fix:** close the gap, either way:
     - (a) Setup creates each missing slot lock file before trying it: O_CREAT 0600, fchown to the owner, and the same for any directory it creates. Every existing slot then has a lock setup can hold.
     - (b) A setup gate: setup holds a root-owned `/run/buildwarden/firecracker-setup.lock` LOCK_EX for the reload. `acquireSlot` takes it LOCK_SH (flock needs only read access) until its VMMs run, and when it is held fails with a "setup is running, retry" fix. (b) also covers a wrong lock-dir guess and `slots.sh`.
     - Add a fake-host test with a slot that has no lock file, and correct the Implementation note.
     - If Jeff accepts the residual instead, describe it accurately in the note and in the PR body's security note.
14. **BOOT-07** (GLOBAL-06), `tools/tier1/rules.yaml:107`. *New.*
   - **What's wrong:** `setup.go` belongs on the bootstrap surface.
     - BOOT-07's surface is each driver's isolation, executor delivery and start code. `setup()` drives the isolation reload, and `quiesceSlots` now decides whether the reload may run.
     - A later change that weakens `quiesceSlots` would quietly reopen finding 2, and the automated check would not flag it: only `slots*.go`, `vm*.go` and `deliver_linux.go` are listed.
     - `posture.go` is the same kind of code: it refuses setup on hosts whose firewall state would leave the slots unisolated (a11).
     - This diff is unaffected, since the hint already fires on `slots.go`.
   - **Suggested fix:** add `driver/firecracker/setup.go` and `driver/firecracker/posture.go` to the surface in this branch. That is a list correction, not a model change. Carry the two paths into `bootstrap-firecracker-row`'s bootstrap.md edit (finding 6).

## Informational

- **BOOT-07 hint** (`deliver_linux.go`): an accepted model change; the doc edit is deferred by the review's decision. See finding 6.
- **warden.exe in history:** finding 1 is fixed in the tree, but 10a1f61's 18 MB blob stays in the branch's history (no force-push, FLOW-17).
  - A squash merge keeps it out of main. A merge commit or a rebase merge brings it in.
  - The PR body should say so; the merge method is Jeff's call (FLOW-08).
  - No rule or tier-1 check refuses a committed binary. The optional check from finding 1 is still unqueued (a LINT-08 doc-set gap).
- **GLOBAL-06:** the PR body should carry the design's security note, the finding-2 fix, and finding 13 unless it is closed.
- **ORCH-01:** `drivers.all` gains firecracker; Publish should tell the open branches (FLOW-10).
- **CI:** `firecracker.yml` has still never run. Its first run is the PR, and a red first run is a FLOW-11 repair item, not a gate (a8).
- **Main:** still at e418871. Re-run tier 1 after Publish merges main.
- **Pre-existing, out of scope (LINT-04):** an unresolvable FROM falls back silently to direct boot (qemu's behavior, moved unchanged).

## Routing

- **Tier 1:** green, so nothing goes back for tier 1.
- **Back to wf-implement:** no MUST or MUST-review findings remain. Findings 13 and 14 are small in-feature fixes, worth doing before Publish if the tick sends work back.
- **PR body:** findings 6, 12, 13 and 14, for acknowledgement at merge (FLOW-08). Put finding 13 in the security note, since GLOBAL-06 needs Jeff's sign-off at merge either way (LINT-03).
