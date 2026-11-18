---
feature: loop-native-lead
status: archived
drivers: []              # the loop's queue code, its native wrapper and settings, and its rules;
                         # no driver tree is reached
seams: []                # no GLOBAL-08 seam changes
surfaces: []             # `wf` is the loop's tool, not a user-facing surface of the product
repos: []                # only buildwarden/buildwarden, where the queue, `wf` and the rules are today
areas: [loop, rules, designs]
security_boundary: none  # no product boundary; the push hardening is the loop's own (Design,
                         # "Pushing from a machine")
alters_relationships: false  # no product component's role or interaction changes (FLOW-02);
                         # this changes which loop worker may take an item's steps
design_review: required  # a process item's design always gets its review (FLOW-03, FLOW-19)
native_validation: []    # see Validation plan: the wrapper's first runs after the merge, tracked
tier2_gaps:
  - "native-tick.sh, native-push.sh and the new settings on macOS (launchd), with one
    run with the private session remote unset: exercised by the MacBook's first
    scheduled runs after the merge; tracked in the native-gap backlog"
  - "the same under Git Bash on the Windows desktop: its first scheduled run after the merge"
  - "the cleanup on a machine: a dirty .scratch/ worktree and a squash-merged branch that
    git branch -d refuses, each shown in native/status/<machine>.yaml after an idle run and
    after a worked run; exercised on the first runs after the merge"
rules: [FLOW-03, FLOW-04, FLOW-05, FLOW-06, FLOW-07, FLOW-08, FLOW-09, FLOW-10, FLOW-12,
        FLOW-13, FLOW-14, FLOW-16, FLOW-17, FLOW-18, FLOW-19, FLOW-20, LOOP-01, LOOP-04,
        LOOP-05, LOOP-06, LOOP-08, LOOP-09, PROFILE-01, GLOBAL-09, GLOBAL-10, GLOBAL-11,
        TEST-01, TEST-02, TEST-03]
rule_changes:
  - {rule: FLOW-04, change: amend, paths: [docs/development/process.md]}
  - {rule: FLOW-05, change: amend, paths: [docs/development/process.md]}
  - {rule: FLOW-09, change: amend, paths: [docs/development/process.md]}
  - {rule: FLOW-13, change: amend, paths: [docs/development/process.md]}
  - {rule: FLOW-16, change: amend, paths: [docs/development/process.md]}
  - {rule: FLOW-17, change: amend, paths: [docs/development/process.md]}
  - {rule: FLOW-18, change: amend, paths: [docs/development/process.md]}
  - {rule: PROFILE-01, change: amend, paths: [docs/development/loop-profile.md]}
concepts: [workflow, native-validation, native-lead, lead, host-names, capability-any, lease,
           lease-renewal, wf-renew, scheduled-sessions, native-wrapper, push-script,
           permission-denies, priority-gate, budgets, worker-identity, componentization, program]
related: [componentization.md, loop-contract.md, loop-parallel.md, alters-slot.md,
          native-host-loop.md, native-loop-followups.md, native-allowlist-scratch.md,
          native-allowlist-scratch-tools.md, wf-later-phase-host-steps.md,
          workflow-bootstrap.md]
---

# A native machine may lead an item from its first step: the lead, general host names, any-capability work under a refreshable 20-hour lease, the wrapper and the push script

A process item, and a program item under FLOW-20: P1's fourth item
([`componentization`](componentization.md), the P1 row), after
`loop-contract` (#119, `ad4ea50`), `alters-slot` (#118, `897a4a8`) and
`loop-parallel` (#121, `623131a`), and before `loop-repository` and the
move to the loop repository. Read on `origin/main` at `0cd3c02`; the queue
state on `origin/workflow` as checked out at `.dev/workflow` on 2026-11-17.

Words used throughout. A **machine** is a native computer in `hosts.yaml`
(`macbook`, `windows-desktop`). A **host name** is a name a machine
serves (`macos-vz`, `qemu-any`). A **led item** is a feature whose `lead`
names a host name, so that a machine takes its steps from Branch setup on.
The **wrapper** is `tools/workflow/native/native-tick.sh`, which a
machine's scheduler runs. The **push script** is the new
`tools/workflow/native/native-push.sh`. The **proposal** is the loop
change proposal of 2026-11-14, a document of the attended session that is
not kept in the repository; its step 4 is this item. Where it and the code
disagree the code is followed.

## Problem

Jeff approved, on 2026-11-14, a loop in which "items a native machine
leads from their first step" are one of P1's deliverables (componentization
P1 row, `docs/design/componentization.md:296`). K3, the Hyper-V driver
extracted as the pilot, "passes native validation there"
(`componentization.md:313`): its design, its implementation and most of
its tests need the Windows desktop. The loop cannot run such an item
today, for six reasons.

1. **`wf` seeds every template step as `cloud`.** The templates give
   Branch setup through Close the `cloud` affinity
   (`tools/workflow/queue/phases.go:44-69`), and `Seed` copies the
   template's affinity into every step it adds (`phases.go:405-441`).
   FLOW-09's last sentence says work whose design or implementation needs
   the native host "carries `host:<name>` affinity on those phases too"
   (`docs/development/process.md:441-442`), and intake is told to change
   the affinity by hand (`.claude/skills/workflow-intake/SKILL.md:85-86`).
   But intake writes only phase 0's step: `f1-design` and every later
   step are seeded at each phase change (`Complete`, `phases.go:609-631`)
   as `cloud`. The hand edit reaches one of the six steps the machine would
   need (phases 0 to 5).
2. **A host's claim starts no item.** `Starts` sets a queued feature in
   progress only on a `cloud` claim (`queue/next.go:130-133`). A host
   worker that claimed an edited `f0-branch-setup` would hold a step of an
   item that stays `queued`: its phase moves when the step is done, and
   its status does not (loop-contract's 2026-11-14 revision entry, in its
   note on the first entry, names this as the difference this item closes,
   `docs/design/loop-contract.md:3133-3142`).
3. **A `<capability>-any` name belongs to one machine.** FLOW-04 says
   `hosts.yaml` "gives each host name to exactly one machine"
   (`process.md:187-188`), `wf validate` rejects a name two machines
   serve (`validateNative`, `queue/hosts.go:145-170`, the error at line
   158), and the live table gives `qemu-any` to `macbook` and
   `container-any` to `windows-desktop` (`hosts.yaml` on `workflow`).
   Jeff decided on 2026-11-14 that such work goes to "whichever loop
   supports it and next can pick it up, placing a refreshable 20-hour
   lease while actively working on it" (loop-contract a15 (b) and a24,
   hand-off point 7, `loop-contract.md:1891-1928`). It is decided and
   not reopened here; this design says how.
4. **A lease is 120 minutes for every claim, and nothing renews one.**
   `Claim` sets `lease_minutes`, 120 in `config.yaml`, for every worker
   (`queue/next.go:304-311`); `wf` has no renew command
   (`tools/workflow/cmd/wf/main.go:306-311`, the command table). A design or implementation
   step on a machine can outlast two hours (a VM image build, a native
   test matrix), and the only way to keep it is to claim the step again,
   which counts one more step against the item's budget. A session that
   dies keeps its step until the lease runs out; nothing releases it
   sooner.
5. **The cloud's own guards do not bind a machine.** An open blocking
   question and a spent runaway budget stop only the cloud
   (`cloudHold`, `queue/next.go:247-260`, applied behind
   `Kind(worker) == "cloud"` at line 218). Only the cloud tick reads
   `priority_confirmed` (`workflow-tick` step 1,
   `.claude/skills/workflow-tick/SKILL.md:65-67`); `wf gate` does not
   (`queue/hosts.go:196-242`). FLOW-13 speaks of one daily run
   (`process.md:526-537`). While a machine took only validation steps
   that was harmless. A machine that leads an item would design and
   implement past a blocking question, past a runaway budget, and before
   Jeff has confirmed the order.
6. **A machine's pushes are guarded by allow rules that admit more than they
   mean.** `settings.native.json` allows `Bash(git push origin feat/*)`
   (`tools/workflow/native/settings.native.json:30`), and a mid-pattern `*`
   spans spaces and slashes (`native-run.sh`'s header,
   `tools/workflow/native/native-run.sh:16-17`; the trailing-`:*` reading is
   in `docs/design/native-allowlist-scratch.md:29-32` and `173-175`);
   nothing recorded of the matcher says it stops at a colon, and a
   `TestNativeSettingsRules` case will record the colon reading under the
   test's own matcher (a16). So `git push origin feat/x --force`, `git push origin feat/x :feat/other` (a remote
   delete) and `git push origin feat/x:workflow` all match the allow, and
   none matches a deny: the denies are prefixes (`git push --force:*`, `git
   push origin --delete:*`, lines 76-79), and the `git -C * ...` denies
   (lines 80-86) do not apply to a bare `git push`. The same holds under
   `git -C .scratch/<name> push ...`, which `Bash(git -C .scratch/*)`
   allows: `git -C * push origin main*` does not match `feat/x:main`, and no
   deny names a refspec that starts with `:`. A machine that only validated
   pushed nothing but queue state; a machine that leads an item pushes its
   branch at every checkpoint (FLOW-17).

`main` is now protected by the host, which is what this item waited for
(its `source`; loop-contract a21, `loop-contract.md:913-954`). Read with
`gh api` on 2026-11-17: the ruleset "main protection" (id 27339039,
created 2026-11-15T23:31:50Z, enforcement `active`, no bypass actors)
applies to the default branch and requires a pull request, blocks
deletion and non-fast-forward pushes, and requires the `ci-ok` check with
an up-to-date branch. So a stray push to `main` is refused by GitHub
whatever a session's settings say. Every other branch, `workflow`
included, has no protection; the repository deletes head branches on
merge, and allows auto-merge (`allow_auto_merge: true`).

## Prior art

- **`loop-contract`** (#119). Its hand-off row for this item
  (`loop-contract.md:1741`) and point 7's second bullet bind this design:
  LOOP-05 is the frame (a worker is scheduled and bounded, routed by the
  one host table, identified on every claim, set up by the owner); the
  FLOW rules the proposal lists are FLOW-04, -09, -13, -16 and -18, and
  "its design settles its own list"; Jeff's rule for `<capability>-any`
  work is decided, with six open points this design settles (a6 to a10
  below). Point 5 leaves two more here: how FLOW-13's budgets apply to a
  worker other than the daily cloud run (a14), and the
  priority-confirmation gate (a12); and it says this item changes the
  wrapper, so the wrapper's start from trunk is this item's to keep
  (a21).
- **`loop-parallel`** (#121). Its hand-off to this item
  (`docs/design/loop-parallel.md:3661-3687`): the guard applies to every
  worker and to `wf claim`, so a machine's led claims are bounded by the
  same limits as the cloud's; this item adds `lead` and decides which
  steps a machine takes; `f6-refresh` is seeded `cloud` and may stay so;
  a hold on `cloud` now stops refreshes; the PR-slot tie-break makes a
  led item of lower priority wait for a cloud-led one, and this design
  says whether that is wanted (a22); `wf commit` and a digest file per
  run already let a machine and the cloud write state side by side;
  `QUEUE.md`'s Workers section and a Lead column are left here (a24), and
  so is a hold that stops led work (Holds);
  `wf renew` is left here. It amended three clauses of FLOW-04 and left
  "its host-name and lease clauses" to this item
  (`loop-parallel.md:4158-4164`). Its **What this item does not do**
  names "the push script" as this item's (`loop-parallel.md:3745-3746`).
- **`alters-slot`** (#118): "No native push hardening (it goes with
  `loop-native-lead`)" (`docs/design/alters-slot.md:426`).
- **`native-host-loop`** (#64) and **`native-loop-followups`**: the
  wrapper, `wf gate`, the zero-token idle night, one session per machine
  under a lock, the installers, the native budget, `native/status/`, and
  "a guard against accidents, not a sandbox: `make`, `go` and
  `powershell` can run anything" (`docs/design/native-host-loop.md:100-102`).
  Kept. Its Decision "each host name belongs to exactly one machine"
  (`native-host-loop.md:46-51`) is the one Jeff changed.
- **`native-allowlist-scratch`** and **`native-allowlist-scratch-tools`**:
  the matcher's rules (a mid-pattern `*` spans spaces and slashes, a
  trailing `:*` is a trailing ` *`), `native-run.sh` as the one route for
  a branch's scripts, checked on its literal text, and the reason it
  exists ("needs no new allow rule"). The push script is the same shape
  for pushes.
- **`wf-later-phase-host-steps`**: a host step of a later phase waits for
  its phase and survives `wf back`. Kept; a led item's steps are seeded
  at their phase, so they never wait early.

## Decisions

1. **A lead is a host name on the item, and `Seed` gives it the item's
   `cloud` steps from Branch setup through Guide** (a2, a3). The queue
   file gains `lead: host:<name>`. Every template step `Seed` adds to a
   led feature in phases 0 to 5 whose template affinity is `cloud` gets
   the lead instead; the design review stays `human`, and Publish, Merge
   and Close stay `cloud`. A return (`wf back`) re-seeds with the lead
   too, since it goes through `Seed`. A process item's `cloud` hold
   therefore no longer stops a led item's phases 0-5; one that must stop
   them holds `host:*` too (Holds).
   - *Rejected: per-phase affinities set by hand, as FLOW-09 says today.*
     They reach phase 0 only (Problem 1).
   - *Rejected: a lead that is a machine name.* Routing is by host name
     everywhere else (FLOW-04, `hosts.yaml`, holds), and a host name lets
     Jeff move a capability between machines with a table edit, as he
     does today.
   - *Rejected: the lead takes Publish too.* Publish opens the PR and
     reads CI (`gh pr create`, `gh api`), and `settings.native.json`
     denies `gh api` (line 89) for good reason. The cloud's
     consolidation and gates run `make test` and `make lint` for every
     platform by `GOOS`, which is all Publish needs; native testing of
     the consolidated branch is a blocking `f6-validate-<host>` step, as
     for any feature (FLOW-09).
2. **A claim by a worker that satisfies the lead starts a queued led
   item, and the cloud cannot start one** (Problem 2). `Starts` reads
   the lead. A queued led item is offered only to its lead's machines;
   the guard (`Guard`, `queue/limits.go:177-209`) and the waiting limit
   decide the start exactly as they do for the cloud, because
   `loop-parallel` already made them every worker's. A queued process or
   tracked item still starts on any claim, as today.
3. **Only a feature may be led, and any worker may set or change a
   lead, each change on the record** (a4, a5). `wf new -lead
   host:<name>` at intake and `wf lead -worker W -feature F -host <name>
   | -clear` later, by a human, the cloud or a machine; no worker kind is
   refused. `wf lead` re-labels the item's pending, unleased template
   steps of phases 0 to 5 and writes a line in the item's queue history;
   the run that made the change lists it in its digest under **Moved**.
   A lead is routing (which worker takes the steps), not priority or
   placement: the item's order and FLOW-05's limits are unchanged, so
   LOOP-04 is not amended. A process item is never led: its work is the
   loop's own machinery, which a machine's session may not edit
   (`workflow-native` Never list, `.claude/skills/workflow-native/SKILL.md:25-38`),
   and its native work is done in an attended native session or tracked
   as native gaps, as its design says. A tracked item's steps are all
   `human`.
   - *Rejected: only the human sets a lead* (the draft's a5 option 1).
     A design or a machine that finds an item needs a native host would
     wait for Jeff's next session to move it; the history line and the
     digest's **Moved** put every change in front of him instead, and he
     may change it back with `wf lead`.
4. **Any host name may be served by several machines; a step goes to
   whichever serving worker next takes steps** (Jeff's rule for
   `<capability>-any`, loop-contract a15 (b); this design proposes the
   same for every host name (a6), to be confirmed at the review). The
   one-machine rule leaves FLOW-04 and `validateNative`. `wf validate`
   still requires that every pending host step, open gap and lead names
   a host some machine serves. Nothing else is needed for two machines
   to share a name: the lease already keeps two workers off one step,
   and `wf commit` already asks a rejected claim again (FLOW-04 as
   amended by `loop-parallel`).
5. **A new form of host name, `<os>-any`** (a7): work that needs the
   platform and its toolchain but no one driver, such as a change built
   with cgo on macOS or a PowerShell module. It is the name a led item
   uses when no driver name fits. `hosts.yaml` gains the names only on
   Jeff's word, at Close (a11).
6. **The cloud is not a capability worker unless the host table lists
   it** (a8). Jeff's word was "whichever loop supports it". The table
   says what each machine supports; the cloud is listed when Jeff lists
   it, as a machine named `cloud` whose hosts are the names it can
   serve. Nothing in this item lists it.
7. **A machine's claim is a 20-hour lease, renewed while the session
   works, released when the session ends** (a9, a10). `Claim` sets
   `host_lease_minutes` (1200 when unset) for a `host:` worker and
   `lease_minutes` for the others. `wf renew` sets a held lease's expiry
   to now plus that length, refuses a worker that does not hold it, and
   counts nothing. The wrapper releases every lease its session still
   holds when the session ends, and, at its start, after it builds `wf`
   and before the gate, since the gate does not offer a leased step,
   every lease an earlier scheduled session of the same machine left (the
   lock shows no session alive). An attended session releases its leases
   at its Close, and a lease it leaves runs to its expiry. A lease is
   therefore held for 20 hours only
   by a session that is running, by a machine that lost power mid-run
   until its next scheduled run, or by an attended session closed
   without its Close.
   - *Rejected: a short lease renewed often (a heartbeat).* It needs a
     push of the workflow branch every few minutes from a session that
     is busy, and Jeff's word is 20 hours.
   - *Rejected: the 20-hour lease for every claim, the cloud's
     included.* A cloud run takes one step in minutes, and a dead cloud
     run would hold its step through the next day's run.
8. **A claim names the machine** (a10): a machine's worker is
   `host:<name>/<machine>.<run>`, where `<run>` is the wrapper's UTC
   start. `Kind` still reads `host:<name>`, so holds, `Satisfies` and
   the guard are unchanged, and LOOP-05's "Identified" holds: the claim
   records the kind, the host name and the session, which now carries
   the machine.
9. **The cloud's guards become every unattended worker's** (a12, a13,
   a14). An open blocking question and a spent runaway budget stop every
   worker but `human`, in `wf next`'s `wait` and in `wf commit`'s
   `claimAgain` (`cloudHold` becomes `unattendedHold`; the red-main
   refresh test stays with the refresh, which is `cloud`). `wf gate`
   exits idle, with the reason, while `priority_confirmed` is false, so
   no machine starts an agent then, as the cloud starts none. FLOW-13
   says this for every unattended run, and says that each run spends its
   own budget: the cloud run `run_budget`, a machine's session
   `native_budget`, with a led step counted as one step like a
   validation step.
10. **A machine pushes a branch only through a push script on `main`**
    (a15, a16). `native-push.sh <name>` pushes `.scratch/<name>`'s
    checked-out `feat/<item>` to the same name on `origin`, one refspec
    and no `+`, so git refuses a non-fast-forward; it refuses a branch
    whose item is not in progress in the queue state, a detached head,
    any other branch name, and any argument after the name. It is on
    `main` because the wrapper resets the checkout to `origin/main` and
    `native-run.sh` refuses a branch's `tools/workflow/`
    (`native-run.sh:107`), so no branch can change it before it
    reaches `main` by Jeff's merge (FLOW-14). The two push allows go,
    and two denies, `Bash(git push*)` and `Bash(git -C * push*)`, refuse
    every push the session would run itself, and also any `git -C`
    command whose text contains ` push`. So a machine commits with `-F
    ../<name>.msg`, a file beside the worktree (a16). The queue state is pushed
    by `wf commit`, which runs git as a child process and is not seen by
    the matcher (`queue/commit.go:239-253`).
    - *Rejected: more denies, one per bad form.* Every form found so far
      is one the earlier denies missed; a list of denies cannot be shown
      complete, and an allow-everything-but list is the shape that
      failed.
    - *Rejected: no change, since `main` is protected.* The ruleset
      covers `main` alone. A force-push or delete of another item's
      branch, or of `workflow`, loses work nothing else records.
11. **Each machine cleans up after a done item, at its next run** (a18).
    Close runs on the cloud (`workflow-tick` step 2.4,
    `SKILL.md:145-152`) and cannot reach a machine's `.scratch/<name>`
    worktrees. The wrapper removes, before the gate, each `.scratch/`
    worktree whose branch belongs to a done item or is gone from
    `origin`, with `git worktree remove` (never `--force` on a dirty
    tree: a dirty one is kept and recorded as `failed` in the machine's
    status, as uncommitted workflow changes are, `native-tick.sh:115-116`,
    with the leftover note appended to the reason after the session or
    idle gate has set it, and passed to the session for its digest).
    FLOW-16 names it.
12. **A machine records its sessions as the cloud does, and takes no led
    step when it cannot** (a19). The wrapper already runs the FLOW-18
    check (`native-tick.sh:199-200`). It also runs `entire status`, and
    when that does not name the private remote it tells the session so;
    the skill then takes no led step that run (validation goes on, with
    the note), since a led step's reasons are what Jeff's design review
    and merge read (FLOW-18, second sentence).
13. **This item holds no worker** (a20). The wrapper rebuilds `wf` and
    resets to `origin/main` at every run, so each machine picks up the
    change at its next scheduled start; a session running at the merge
    keeps what it started with (LOOP-09). The one field an old `wf`
    would drop when it saves a file, `lead`, is first written after the
    merge, at Close, by an attended step (Design, "Holds").

## Questions

### Blocking

None. A process item's design is the human's, settled in its design
review (FLOW-03, FLOW-19); every choice below is asked there.

### Assumed

Each is "proceeding with the recommended option; confirm".

- **a1. The rules this item changes.**
  1. *(recommended)* FLOW-04 (the host-name, lead and lease clauses),
     FLOW-05 (one phrase: what a worker blocks on), FLOW-09 (a lead in
     place of per-phase host affinity), FLOW-13 (every unattended run,
     its budget, the guards, the priority gate), FLOW-16 (a machine's
     worktrees), FLOW-17 (a machine's pushes) and FLOW-18 (a machine's
     sessions), and PROFILE-01's Native hosts entry
     (`docs/development/loop-profile.md:29`), which an `<os>-any` name
     would make false (a7). LOOP-05 is unchanged: it cites FLOW-04 for
     who takes a step and states no one-machine rule. **The contract's
     citations, checked**, under Rule changes, shows each still holds.
  2. As 1 without FLOW-05 and FLOW-17. FLOW-05's "a `human` or
     `host:<name>` step" (`process.md:232`) is then the cloud's view
     only, and the push script enforces a rule no rule states.
  3. As 1, and LOOP-05 also states Jeff's rule. FLOW-04 would then be
     stated in two places (FLOW-14), which loop-contract rejected for the
     same rule (`loop-contract.md:366-372`).
- **a2. What a lead names.**
  1. *(recommended)* A host name, `host:<name>`, which every machine that
     serves the name satisfies.
  2. A machine name. One machine then does the whole item, which keeps
     its scratch worktrees and caches in one place, but a `-any` item
     could not move to the other machine, which is the opposite of
     Jeff's rule for `-any` work.
  3. No field: intake sets affinities by hand and `wf` keeps them. Each
     phase's steps would need a hand edit when seeded (Problem 1).
- **a3. Which steps a lead takes.**
  1. *(recommended)* The `cloud` template steps of phases 0 to 5:
     Branch setup, Design, Plan, Implement, Review and Guide. The design
     review stays `human`. Publish (`f6-merge-main`, `f6-consolidate`,
     `f6-open-pr`), the refresh, Merge and Close stay `cloud`.
  2. Through `f6-open-pr`. The machine would need `gh pr create` and to
     read CI, which `settings.native.json` denies (`gh api`, line 89).
  3. Design and Implement only. Plan, Review and Guide would hop to the
     cloud and back, each hop a day's wait for the other worker and a
     fresh checkout.
- **a4. Which items may be led.**
  1. *(recommended)* A feature only. `wf validate` rejects `lead` on a
     process item or a tracked item; a process item's native work is done
     in an attended native session or tracked as native gaps, as its
     design says (FLOW-09 as amended). A follow-up filed with no human
     present (FLOW-09) takes the repositories and areas of the item it
     follows and no lead; any worker may lead it with `wf lead` (a5).
  2. A feature, and a follow-up takes its parent's lead. A regression
     found on a machine would stay there, but a fix the cloud could make
     today would wait for a machine.
- **a5. Who sets a lead.** *Answered at the design review (2026-11-17):
  option 2, Jeff's choice, refined.* Any worker (human, cloud or a native
  machine) may set or clear a lead, with `wf lead` or `wf new -lead`;
  each change is a line in the item's queue history and is listed in that
  run's digest under **Moved**. No worker kind is refused. LOOP-04 is not
  amended: a lead is routing (which worker takes the steps), not priority
  or placement, and the item's order and FLOW-05's limits are unchanged.
  The options as drafted:
  1. *(recommended in the draft; not chosen)* The human, at intake (`wf
     new -lead`) or later (`wf lead`, which refuses a worker that is not
     `human/<name>`, as `wf relate` does). A design that finds an item
     needs a machine says so as a finding, and Jeff leads it at the
     design review.
  2. *(chosen)* Any worker, by `wf lead`. A machine could take work from
     the cloud on its own judgment; LOOP-04 says priority and placement
     are the human's.
- **a6. Several machines per host name.**
  1. *(recommended)* Any host name may be listed on several machines.
     `validateNative` drops its "served by both" error and keeps the
     check that every pending host step, open gap and lead has a
     machine. Nothing changes until Jeff lists a name twice. This goes
     beyond Jeff's decision, which covers `<capability>-any` names only,
     by also letting `<os>-<driver>` and `<os>-any` names be shared.
  2. Only `<capability>-any` names may be listed twice; `<os>-<driver>`
     and `<os>-any` stay one machine's. This is exactly Jeff's decision
     (loop-contract a15 (b), a24). It keeps a platform-specific validation on the one
     machine set up for it (the cached relay VHDX on the Windows
     desktop, `native-tick.sh:222-240`), but a second Mac could then not
     share `macos-vz`.
- **a7. `<os>-any`.**
  1. *(recommended)* Add the form to FLOW-04, for led work that needs a
     platform and no driver. `hosts.yaml` lists `macos-any` on `macbook`
     and `windows-any` on `windows-desktop` only if Jeff says so at
     Close (a11).
  2. No new form; such an item is led by a driver name (`macos-vz`)
     that happens to be on the right machine. The name then says
     something false about the work.
- **a8. Whether the cloud is a capability worker.**
  1. *(recommended)* Only when the host table lists it, as a machine
     named `cloud`; nothing lists it now. The cloud session that wrote
     this design has `/usr/bin/docker` on its path, but whether a daemon
     and outbound network are there for every scheduled run is not
     known, and `native-host-loop` found neither on 2026-10-29.
  2. The cloud serves `container-any` from the merge on. It would take
     the Windows desktop's container work unasked, on an environment no
     one has checked.
- **a9. The lease: its length, who gets it, and its renewal.**
  1. *(recommended)* `host_lease_minutes`, 1200 when unset, for every
     claim by a `host:` worker, a validation step's included; cloud and
     human claims keep `lease_minutes`. `wf renew -worker W -feature F
     -step S` sets the expiry to now plus that length; it counts nothing
     against the item's budget and is pushed with `wf commit -renewed
     F/S`, which on a rejected push renews again on the new state,
     refused unless the step is still W's, and never claims. The skill renews before dispatching a phase agent, and at each
     checkpoint push, whenever less than half the lease is left.
  2. The 20-hour lease for `-any` steps only. A led `windows-hyperv`
     item's Implement step would have two hours, and be claimed again
     (and counted) to keep it.
  3. The 20-hour lease for every claim. See Decision 7.
- **a10. What happens to a lease that is not renewed, and how a claim
  names its machine.**
  1. *(recommended)* The worker is `host:<name>/<machine>.<run>`. The
     wrapper runs `wf release -session <machine>.<run> -why "session ended
     (exit <rc>)"` from the EXIT path once the session has started,
     releasing every pending step whose lease that session holds, and, at
     its start, before the gate, `wf release -machine <machine> -stale
     -current <machine>.<run> -why "an earlier session on <machine> left
     it"`, which releases every live lease of a worker whose session names
     the machine and is not the current run. The lease's own expiry is the
     fallback when neither runs (the machine never runs again), named under
     **Defaults and overrides**. An attended `/workflow-native <host>`
     session takes the worker `host:<name>/<machine>.attended-<UTC>`. The
     machine is the one `hosts.yaml` lists for the host when only one does,
     and otherwise the session's second argument, `/workflow-native <host>
     <machine>`; with several machines and none named, the skill asks which
     machine this is, as it asks for a missing host. `wf release -machine
     <m> -stale` skips sessions whose run starts `attended-`, so a scheduled
     run never frees the step of a live attended session. The skill's Close
     runs `wf release -session <machine>.attended-<UTC> -why "attended
     session closed"`. An attended session closed without its Close keeps
     its leases until the 20-hour expiry, named under **Defaults and
     overrides**.
  2. No release; the 20-hour expiry alone. A session that hit its turn
     cap at 04:50 holds its step until 00:50 the next day, past the
     Windows desktop's next three runs.
- **a11. The queue state after the merge.**
  1. *(recommended)* Close (`x4-close`) is preceded by an attended step,
     `x4-state` (`human`), as `loop-parallel` had: on Jeff's word it
     lists any second machine for a name in `hosts.yaml` (for example
     `container-any` on `macbook`, which has finch), adds `<os>-any`
     names if a7 stands, sets `host_lease_minutes` if he wants other
     than 1200, and leads the first item. It runs once no live lease
     predates the merge. The step is not seeded: the process template
     seeds only `x1-design` to `x4-close` (`queue/phases.go:74-80`). The
     session that records this design's questions on the workflow branch
     adds it by hand, as `loop-parallel` did (3c994c4): `x4-state`, phase
     4, affinity `human`, with a `note` listing its checklist. The
     checklist: after the PR has merged and once no live lease predates
     the merge commit and both machines' `native/status/` files show a
     run after it, not before; `hosts.yaml`'s header and any second
     machine or `<os>-any` name on Jeff's word; `host_lease_minutes`;
     README.md; the three native gaps in `backlog/native-gaps.yaml`, one
     per `tier2_gaps` entry; the
     first led item. `x4-close` gets `needs: [x4-state]`. `wf` gates steps
     of one phase only by `needs` (`queue/next.go:183`), and
     `loop-parallel`'s `x4-close` had none.
  2. No state change; Jeff edits `hosts.yaml` when he wants. The rule is
     then true and unused until he does.
- **a12. The priority-confirmation gate.**
  1. *(recommended)* Every unattended worker honours it, and `wf gate`
     enforces it for machines: while `priority_confirmed` is false the
     gate exits idle with "idle: the priority order is not confirmed",
     so no agent starts, and `native/status/` records the reason, which
     the wrapper now takes from the gate's output. FLOW-13 states it.
  2. It stops a machine only from starting a queued item; validation
     and steps of items in progress go on. Finer, but a led item is
     designed and implemented in an order Jeff has not confirmed.
  3. It stays the cloud's (hand-off point 5's other option).
- **a13. The blocking-question and runaway guards.**
  1. *(recommended)* Every worker but `human`. `cloudHold` is renamed
     `unattendedHold` and applied when `Kind(worker) != "human"` in `wf
     next` (`wait`) and in `wf commit`'s second claim after a rejected
     push (`claimAgain`); its
     red-`main` test stays, and only a refresh, which is `cloud`, meets
     it.
  2. Only for a machine's led steps; a validation step of an item with
     an open blocking question is still taken. It tests a branch whose
     design is in question.
- **a14. Budgets for a worker other than the daily cloud run.**
  1. *(recommended)* Each unattended run spends its own budget: the
     cloud run `run_budget`, a machine's session `native_budget`
     (falling back to `run_budget`), counted together over its steps and
     backlog entries as today (`workflow-native`, `SKILL.md:83-87`). A
     led step counts one, like a validation step. The item's work budget
     counts every claim whoever makes it, except a refresh's first claim
     (loop-parallel a10 (d)), so the runaway check is one for all
     workers. Jeff calibrates `native_budget` once machines lead
     items.
  2. A separate `lead_budget` per machine session. One more number to
     calibrate, and a session would then split its turn cap
     (`BW_MAX_TURNS`, `native-tick.sh:251`) between two budgets.
- **a15. What the push script checks.**
  1. *(recommended)* Run from the checkout root as
     `tools/workflow/native/native-push.sh <name>`: `.scratch/<name>` is
     a worktree of this repository (the same checks as
     `native-run.sh:112-116`); its HEAD is the branch `feat/<item>`, by
     `git symbolic-ref`; `.dev/workflow/queue/<item>.yaml` exists and
     says `status: in_progress`; then it runs `git -C .scratch/<name>
     push origin refs/heads/feat/<item>:refs/heads/feat/<item>` and
     execs nothing else. A first push sets the upstream. Every refusal
     is one line on stderr and exit 2, before anything runs.
  2. As 1, and the item must also be led by, or have a pending step
     leased to, a worker of this machine. Tighter, but it needs YAML
     read in shell, or a `wf` command for it; the in-progress check
     already stops a push to a done item's or an unknown branch.
- **a16. The denies in `settings.native.json`.**
  1. *(recommended)* Remove the allows `Bash(git push origin workflow)`
     and `Bash(git push origin feat/*)` (lines 27, 30); add the denies
     `Bash(git push*)` and `Bash(git -C * push*)`; keep the existing
     denies, which the two new ones cover, so a later edit that removes
     one still leaves the others. `native_test.go`'s rule test
     (`TestNativeSettingsRules`, `native_test.go:20`) gains the two and
     fails on any allow that starts with `git push` or contains ` push `.
     The second deny also refuses `git -C .dev/workflow push`, which
     `Bash(git -C .dev/workflow:*)` (line 26) allows today. A mid-pattern
     `*` spans spaces, so the second deny also matches any `git -C`
     command whose later text contains ` push`: a commit message (`git -C
     .scratch/x commit -m "add the push script"`), `log --grep push`, or
     a pathspec. The workflow-native skill and the phase-agent sentence
     for a led step therefore require this commit form on a machine:
     write the message to `.scratch/<name>.msg`, beside the worktree and
     outside it so the tree stays clean for the cleanup, and run `git -C
     .scratch/<name> commit -F ../<name>.msg` (git reads `-F` from the
     `-C` directory). Never `-m`, never `--grep push`.
     `TestNativeSettingsRules` also records the false positive: the deny
     matches `git -C .scratch/x commit -m "a push fix"` and does not match
     `git -C .scratch/x commit -F ../x.msg`. *Answered at the design
     review (2026-11-17): option 1, with one more test case.* Using the
     test's own matcher reading, `TestNativeSettingsRules` shows that the
     old allow `Bash(git push origin feat/*)` matches `git push origin
     feat/x:workflow` and `git push origin feat/x :feat/other` (Problem
     6's colon claim), and that the new denies `git push*` and `git -C *
     push*` match them.
  2. Keep the allows and add denies for each bad form found (Decision
     10's first rejection).
  3. Keep `-m` and state as a known limit that a message or argument
     must not contain ` push`. Rejected: an unattended session cannot
     tell why it was refused.
- **a17. The same denies for every other session in this repository.**
  1. *(recommended)* No change to `.claude/settings.json`. Its sessions
     are Jeff's attended ones and the cloud's, which push feature
     branches with plain `git push` (`workflow-tick`, and this design's
     own commit); the cloud's permissions are its environment's. `main`
     is protected by the host for all of them.
  2. Add the FLOW-16/17 denies (force, delete, mirror, an empty-source
     refspec) to `.claude/settings.json`. It narrows Jeff's attended
     sessions too, which he would then have to override by hand for a
     cleanup he means.
- **a18. A machine's leftovers of a done item.**
  1. *(recommended)* The wrapper removes them before the gate, as
     Decision 11 says, and FLOW-16 names it.
  2. The `workflow-native` skill's Close does it, with an agent. It
     spends turns on an idle night's work and runs only when the gate
     finds work.
- **a19. A machine that cannot record its session.**
  1. *(recommended)* The wrapper passes "sessions not recorded" in the
     prompt when `entire status` does not name
     `buildwarden/buildwarden-sessions`; the session takes no led step,
     validates as usual, and puts the line first in its digest file.
  2. As the cloud does today: carry on and say so in the digest. A led
     design would reach Jeff's review with no recorded reasons.
- **a20. Whether this item holds a worker.**
  1. *(recommended)* None. See Decision 13 and **Holds** under Design.
  2. `host:*` from Implement until Close. Every machine's validation
     would stop for the days the PR waits for Jeff's merge and its
     refreshes.
- **a21. The cloud tick's start from trunk.**
  1. *(recommended)* `workflow-tick` step 1 moves `git fetch origin main
     workflow` above the build and, between them, checks that the
     checkout's HEAD equals `origin/main`; if it does not, the run stops
     and reports it, pushing nothing. The wrapper already resets to `origin/main`
     (`native-tick.sh:188-190`). LOOP-09 says an unattended run takes
     the loop's tools from trunk.
  2. Leave the tick as it is (hand-off point 5: "the item that next
     changes the tick"). This item changes the tick anyway: its digest's
     **Waiting on a host** line lists leads.
- **a22. The PR-slot tie-break for a led item.**
  1. *(recommended)* Unchanged. A led item publishes on the cloud
     (a3), so at the PR slot it is a cloud item like any other, and the
     higher priority goes first (FLOW-05).
  2. A led item that finished Guide first goes first. It would let a
     lower-priority item jump the queue because its worker was faster.
- **a23. GitHub settings this design leans on and does not make.**
  1. *(recommended)* None is a condition of the merge. Recommended to
     Jeff, his to set (FLOW-08, LOOP-08): a ruleset on `workflow` that
     blocks deletion and non-fast-forward pushes, since every worker
     pushes it and nothing but the denies guards it; and turning off
     `allow_auto_merge`, which no agent may use (FLOW-08) and which is
     on. No agent, skill or `wf` command makes either change.
  2. Make the `workflow` ruleset a condition of the merge. Jeff's
     setting would gate his own merge.
- **a24. `QUEUE.md`.**
  1. *(recommended)* Active gains a Lead column (`-` or the host name),
     and a **Workers** section lists each machine from `hosts.yaml` with
     its host names, its last run from `native/status/<machine>.yaml`
     (outcome, reason, since), and the items it leads or holds a lease
     on. `wf status` prints a `lead:` line under a led item.
  2. A Lead column only.
- **a25. FLOW-05's blocking phrase.**
  1. *(recommended)* "until it blocks: on a `human` or `host:<name>`
     step" (`process.md:232`) becomes "until it blocks: on a step whose
     affinity it does not satisfy". `Next` already behaves so for every
     worker (`wait`, `queue/next.go:209-226`).
  2. Leave it; it reads as the cloud's case.

## Design

### The lead

The queue file gains one optional field, on a feature only:

```yaml
lead: host:windows-hyperv   # optional (FLOW-04, FLOW-09): the host name whose machines take this
                            #   feature's cloud steps of phases 0-5; any worker may set or
                            #   clear it, each change a history line (a5)
```

- `Feature.Lead string` (`yaml:"lead,omitempty"`) in `queue/queue.go`.
  `Validate` rejects a lead that does not match `affinityRe` as a
  `host:` affinity, a lead on a process or tracked item, and (with
  `hosts.yaml` present) a lead no machine serves.
- `Seed` (`queue/phases.go:405-441`): when `f.Lead != ""` and
  `f.Phase <= 5`, a template step whose affinity is `cloud` gets
  `f.Lead`. `SeedRefresh` is unchanged (`cloud`), and so are added steps
  (`f1-answer-<q>` `human`, `f6-validate-<host>` its host).
- `Starts` (`queue/next.go:127-133`): unchanged for a process or tracked
  item (any claim, since its steps are all `human`). For a feature: a
  claim whose `Kind` equals `f.Lead` when the feature is led, and
  otherwise a `cloud` claim, as today. The comment and the README's claim
  paragraph say so.
- `wf new -lead host:<name>` (`tools/workflow/cmd/wf/main.go`, `newFeature`): sets the
  field, seeds `f0-branch-setup` with it, and writes the lead in the
  item's first history line; refuses with `-kind process` and with
  `-repos` naming another home (a tracked item). It refuses no worker
  kind (a5).
- `wf lead -worker W -feature F (-host <name> | -clear)`: a new command,
  open to any worker (`human/<name>`, `cloud/<run>` or a machine's
  `host:<name>/<machine>.<run>`); it refuses no worker kind (a5), and
  refuses a process or tracked item as `Validate` does. It sets or
  clears the field and re-labels each pending template step of phases 0
  to 5 whose lease is not held, between `cloud` and the lead. A held
  step keeps its affinity until it is done or released, so no session
  loses a step it is working. It writes one line in the item's queue
  history naming the worker, the old and new lead, the steps it changed
  and those it left. It changes no priority, order or limit: a lead is
  routing, not placement (LOOP-04 unchanged).
- The digests: `workflow-tick` and `workflow-native` list each lead
  change their run made (`wf new -lead` or `wf lead`) under **Moved**,
  from the history line.
- The intake skill (`workflow-intake`, step 4's sentence at lines 85-86)
  replaces the hand edit with `-lead host:<name>`, for an item whose work
  needs a machine from its first step.
- The workflow branch's `README.md`: the field, `wf lead`, `-lead`, and
  a led item's steps in the Steps and phases table.

What a machine then does with a led step is what `workflow-native` already
says for "Design or implementation steps with host affinity"
(`SKILL.md:95-98`): check out the branch, dispatch to the phase agent as the
tick does, finish as the tick does. Three changes there: the branch is
checked out in a worktree `.scratch/<item>` and never in the wrapper's
checkout, which stays on `main`; every push of it goes through the push
script; and the session takes no led step when the prompt says sessions are
not recorded (a19). The phase agents (`wf-publish` for `f0-branch-setup`,
`wf-design`, `wf-plan`, `wf-implement`, `wf-review`, `wf-guide`) each gain
one sentence: on a machine, a branch is pushed only with
`tools/workflow/native/native-push.sh <name>`, and committed with `git -C
.scratch/<name> commit -F ../<name>.msg`, the message written to
`.scratch/<name>.msg`, never `-m` (a16). `workflow-native`'s Git on the
workflow checkout paragraph (`SKILL.md:42-45`) drops `push origin workflow`
from its allowed form, `git -C .dev/workflow <add|commit|...>`, since a16's
`Bash(git -C * push*)` deny refuses it: the workflow branch is pushed only
by `dist/wf commit`, and a feature branch only by `native-push.sh`.

### Host names and the table

FLOW-04's host bullet has three forms: `<os>-<driver>`, `<os>-any` and
`<capability>-any`, and the table lists the machines that serve each
name, one or more. `hosts.yaml`'s schema does not change: a name listed
under two machines is how several machines serve it.

- `validateNative` (`queue/hosts.go:145-170`): drop the `servedBy`
  duplicate error; build `servedBy` as a set; keep "no machine serves
  host X" for pending steps and open gaps, and add it for a lead.
- `wf gate` (`queue/hosts.go:196-242`) is unchanged in how it finds a
  step: `Next("host:<h>/gate")` for each host of the machine. Two
  machines that both see a ready step both start a session; the first
  claim pushed wins, and the other's `wf commit -claimed` exits 4 and
  its session asks `wf next` again (`workflow-native`, `SKILL.md:75-79`).
  The gate also exits idle while `priority_confirmed` is false (a12).
- `hosts.yaml`'s header comment ("Each host name has exactly one
  machine", "goes to the always-on machine") is rewritten at `x4-state`
  to say the names may be shared.

### The lease

| Worker | Lease on claim | Renewed by | Released early by |
|---|---|---|---|
| `cloud/<run>` | `lease_minutes` (120) | claiming again, as today | `wf release`, by the run |
| `human/<name>` | `lease_minutes` | claiming again | `wf release` |
| `host:<name>/<machine>.<run>` | `host_lease_minutes` (1200 when unset) | `wf renew`, by the session | the session (`wf release`); the wrapper when the session ends; the machine's next wrapper run |
| `host:<name>/<machine>.attended-<UTC>` | `host_lease_minutes` | `wf renew`, by the session | the skill's Close (`wf release -session <machine>.attended-<UTC>`); otherwise the expiry |

- `Config.HostLeaseMinutes int` (`yaml:"host_lease_minutes"`);
  `Validate` rejects a value below 0; 0 or unset reads as 1200.
- `Claim` (`queue/next.go:304-332`) picks the length by `Kind(worker)`.
- `wf renew -state DIR -worker W -feature F -step S`: refuses unless the
  step is pending and its live lease is W's; sets the expiry; logs
  "renewed S until T"; changes no budget. `wf commit -renewed F/S` pushes
  it (a `CommitRequest.Renewed` field, parsed like `Claimed` and mutually
  exclusive with it). On a rejected push it resets to origin the same
  way, reloads the state and calls a new `renewAgain` in place of
  `claimAgain`. `renewAgain` finds F/S and calls `Renew(f, st, worker,
  now)`, the function `wf renew` uses, which refuses unless the step is
  still pending and its live lease is W's; on a refusal `wf commit` exits
  4 ("the renewal of F/S is refused on the new state: ...") and the
  session stops working on the step. It never calls `Claim`, so
  `budget.spent.steps` and `runs` are unchanged.
- `wf release -session <machine>.<run>` and `wf release -machine
  <machine> -stale`: forms of `wf release` that release every pending
  step whose lease matches, each with its own history line. `-stale`
  skips the current run's session, which the wrapper passes with
  `-current`, and every session whose run starts `attended-`, so a
  scheduled run never frees the step of a live attended session.
- The wrapper, `native-tick.sh`:
  - The worker session id in the prompt becomes `$machine.$run` (line
    248).
  - After `stage="build wf"` builds `dist/wf` (`native-tick.sh:201-203`)
    and before `stage="gate"`, a new stage `release` runs `dist/wf
    release -state .dev/workflow -machine "$machine" -stale -current
    "$machine.$run" -why "an earlier session on $machine left it"`.
    When it changed anything, it is pushed with `dist/wf commit`. A
    failure here is a `stop`.
  - The release goes before the gate because `Gate` asks `Next` as
    `host:<h>/gate`, and `wait` (`next.go:215-216`) does not offer a step
    leased to another worker. A step still held by a dead earlier
    session would otherwise make the gate exit 3 (idle) at line 217
    before anything released it, every run, until the lease expires.
  - `session_started=1` is set immediately before `claude -p`.
  - Only the `-session` release runs in `on_exit`, before
    `record_status` and whatever the exit code, guarded by `[ -x dist/wf
    ] && [ -n "$session_started" ]`: `dist/wf release -session
    "$machine.$run" -why "session ended (exit $rc)"`, pushed the same
    way. An exit during skip, sync, build or the gate therefore runs no
    release, and a failed release or push still records the status.
- The skill (`workflow-native`, Collect and Work): worker
  `W=host:<name>/<machine>.<run>` from the prompt, or, attended,
  `W=host:<name>/<machine>.attended-<UTC>` with the machine that
  `hosts.yaml` lists for the host when only one does, and otherwise the
  second argument, `/workflow-native <host> <machine>` (asked for when
  missing); `wf renew` before each
  phase-agent dispatch and at each checkpoint push when less than half
  the lease is left; `wf release` of a step it stops working; attended,
  its Close runs `wf release -session <machine>.attended-<UTC> -why
  "attended session closed"`, which the wrapper's `-stale` release never
  does for it.

### The guards

- `wait` (`queue/next.go:209-226`) and `claimAgain`
  (`queue/commit.go:323-353`, the test at lines 343-347): the hold test
  runs for `Kind(worker) != "human"` in place of `== "cloud"`, in both;
  `cloudHold` becomes `unattendedHold` with the same three tests.
  `claimAgain`'s doc comment changes from "for a cloud worker" to "for
  any worker but `human`". Without the second, a host claim whose push
  was rejected stands past an open blocking question or a spent runaway
  budget.
- `Gate` (`queue/hosts.go:196`): first, `if !s.Config.PriorityConfirmed`,
  return `Run: false` with the reason `idle\tthe priority order is not
  confirmed (FLOW-13)`. The wrapper captures `wf gate`'s output, and its
  case 3 (`native-tick.sh:217`) records `idle` with the gate's `idle`
  line as `reason` (its tab written as `: `), in place of today's fixed
  "idle: gate found no work".
- `workflow-native`'s Never list gains "no step of an item with an open
  blocking question or a spent runaway budget", which `wf next` now
  enforces; "no push but by `wf commit` and the push script"; and "no
  `git -C ... commit -m`: a commit message goes in `.scratch/<name>.msg`
  and is committed with `-F ../<name>.msg`" (a16).

### Pushing from a machine

`tools/workflow/native/native-push.sh`, POSIX `sh` like `native-run.sh`
so it runs under Git Bash on Windows:

```
tools/workflow/native/native-push.sh <name>
```

It refuses, one line on stderr and exit 2, before any push:

- not run from the checkout root, or not in a git checkout;
- a missing name, a second argument, or a name that is empty, `.`,
  `..` or has a `/`;
- `.scratch/<name>` that is not a worktree's top level, or not a
  worktree of this repository (the checks of `native-run.sh:112-116`);
- a HEAD that is not a symbolic ref, or a branch not matching
  `feat/[a-z0-9][a-z0-9-]*`;
- `.dev/workflow/queue/<item>.yaml` missing, or without the line
  `status: in_progress`.

Then it execs `git -C .scratch/<name> push --set-upstream origin
refs/heads/feat/<item>:refs/heads/feat/<item>`. No `+`, so git refuses a
non-fast-forward, and one refspec, so nothing else moves. `settings.native.json`
already allows it through `Bash(tools/workflow/*)` (line 33). In the
same file the two push allows go and the two denies come (a16). Since
`Bash(git -C * push*)` also matches a commit message that contains
` push`, a machine commits with `git -C .scratch/<name> commit -F
../<name>.msg` (the message in `.scratch/<name>.msg`, outside the
worktree), never `-m`, next to the rule of no push but by `wf commit` and
the push script.
`native_test.go` gains a table test of the script, as `TestNativeRun`
(`native_test.go:81`) tests `native-run.sh`: each refusal, a push to a
bare remote, and a non-fast-forward refused by git.

What this is and is not, as `native-host-loop` says of the settings: a
guard against an agent's mistake, not a sandbox. `make`, `go` and
`powershell` can still run `git push`. The boundary for `main` is the
host's ruleset; for every other branch it is this route, the Never list,
and the checkout the wrapper resets.

### Cleaning up on a machine

Before the gate, after the reset (`native-tick.sh:188-198`), the wrapper
lists `git worktree list --porcelain` entries under `.scratch/`. For each
on a branch `feat/<item>`: when `.dev/workflow/queue/<item>.yaml` says
`status: done`, or `origin/feat/<item>` is gone after the prune fetch, it
runs `git worktree remove .scratch/<name>` and `git branch -d
feat/<item>`. Test images, VMs and containers stay the skill's, as now.

The wrapper gathers what the cleanup leaves behind in a shell variable
`leftover`, one `; `-separated note each:

- `leftover worktree .scratch/<name> has changes` for a dirty worktree,
  which is not removed;
- `branch feat/<item> not deleted (not merged; FLOW-16)` when `git
  branch -d` refuses a squash-merged branch, which is kept.

The notes reach the run's records in three places, because the session
and the gate both overwrite `outcome` and `reason`:

- (a) After line 260 (`reason="claude exited $rc"`, and after the
  stale-hosts line 261), the wrapper appends `; $leftover` to `reason`.
  If a dirty worktree was found it also sets `outcome=failed`, whatever
  claude's exit code was.
- (b) In the idle case at line 217, it appends the notes to the idle reason,
  which it takes from the gate's output (The guards). If a worktree is dirty
  it records `outcome=failed` in place of `idle`.
- (c) It adds `Leftovers: $leftover.` to the session prompt at line 248,
  like `$skip`. The session then lists the leftovers in its digest file.

`record_status` already trims `reason` to 120 characters, so the notes
are kept short. A machine with no session has no digest; `native/status/`
is its record.

### Workers in `QUEUE.md`

`summary.go` gains the Lead column in Active and a **Workers** section
after it (a24):

```
## Workers

| Machine | Host names | Last run | Leads | Holds |
|---|---|---|---|---|
| macbook | macos-vz, macos-qemu, macos-finch, qemu-any | idle, 2026-11-17 12:40 (since 2026-11-15): idle: no host step ready and no open gaps | - | - |
| windows-desktop | windows-hyperv, container-any, linux-kvm | worked, 2026-11-17 10:10 | k3-hyperv-extract | k3-hyperv-extract/f3-implement until 2026-11-18 06:10 |
```

"Leads" lists open items whose lead a host of the machine serves;
"Holds" lists live leases whose worker names the machine. No clock is
read (`queue-summary`'s rule): the times are the files'.

### The cloud tick

- Step 1 moves `git fetch origin main workflow` above the `wf` build
  and, between them, checks that HEAD equals `origin/main`; if it does
  not, the run stops and reports it, pushing nothing (a21). When this is
  implemented, the step-1 block becomes, in order: `entire doctor`; `git
  fetch origin main workflow`; the HEAD == `origin/main` check; `go
  build ... ./cmd/wf`; the workflow worktree add/reset; `dist/wf
  validate`.
- Its Never list and step 2 are unchanged in what the cloud does; the
  cloud now meets a led item's phase 0-5 steps as `host:` steps it
  cannot take, which `wf next` already reports, and the digest's
  **Waiting on a host** lists them with their lead.

### Holds, and what the merge does to running work

This item declares no `holds` (a20).

A hold names workers (FLOW-05; `State.Hold` matches each pattern against
`Kind(worker)`, `queue/next.go:37-48`). A led item's steps of phases 0-5
are `host:<name>` steps (Decision 1), so a `cloud` hold does not stop
them. A process item that must stop all design and implementation holds
`cloud,host:*` (the workflow README's intake form, `wf new ... -kind
process -holds cloud,host:*`). That also stops every machine's validation
while it lasts. No new hold form.

- **Before the merge** nothing changes: the branch's `wf`, wrapper,
  settings and skills are not on `main`.
- **At the merge** a machine's next scheduled run resets to the new
  `main`, rebuilds `wf`, and runs the new wrapper and settings. A
  session already running keeps what it started with (LOOP-09). Its
  worker id has no machine, so the new wrapper's `-stale` release does
  not touch its leases; they expire at 120 minutes, as they were made.
- **The cloud** rebuilds `wf` at its next run. Between the merge and
  that run nothing it does changes: no item has a lead.
- **The one field an old `wf` drops**, `lead`, is written first at
  `x4-state`, once no live lease predates the merge commit and both
  machines' `native/status/` files show a run after it. An old `wf`
  that saved a led item's file would drop the lead and the item would
  fall back to the cloud for its next seeded phase: the safe side, and
  visible in the history.
- **`x4-state`** is a `human` step of a later phase. `wf` offers it
  early, and the waiting-on-human count does not count it until the item
  reaches phase 4 (loop-parallel's rule, `loop-parallel.md`, the
  waiting-on-human count). Its note's first words keep it from being done
  before the merge, and `x4-close`'s `needs` keeps Close behind it.
- **`host_lease_minutes`** unset reads as 1200 in the new `wf` and is
  unknown to an old one, which ignores it.

### What this item hands on

**To `loop-repository`.** A led item in a second repository: the push
script pushes `feat/<item>` of this repository only, and the wrapper's
checkout is this repository's (`install-macos.sh:91-94` checks
`origin`). A second checkout on each machine, the push script taking the
repository, and the `Workers` section by repository are its design's.

**To the move.** `native-push.sh` moves with `tools/workflow/native/`,
and the wrapper then runs it from the loop repository's checkout against
a component's checkout.

### What this item does not do

- No change to a repository setting: the rulesets, auto-merge and
  head-branch deletion are Jeff's (a23).
- No cloud capability worker (a8), and no second cloud run a day.
- No change to LOOP-05, the area map or the limits.
- No change to what a validation step does, beyond its lease and the
  guards.
- No change to the installers' scheduling. `install-macos.sh check` and
  `install-windows.ps1 check` gain a line that `native-push.sh` is
  present and executable, nothing else.

## Defaults and overrides

| Default | Why it is the obvious choice (GLOBAL-09) | Override (GLOBAL-10) |
|---|---|---|
| An item has no lead: its steps are `cloud` | Today's behaviour; a lead is set only when some worker finds the item needs a native host | `wf new -lead`, `wf lead` (any worker; each change a history line and a digest **Moved** entry) |
| A led item's machine takes phases 0-5; Publish on is `cloud` | Publish needs `gh`, which a machine's settings deny (a3) | None; a design that needs otherwise is a process item |
| A machine's lease is 1200 minutes | Jeff's 20 hours (loop-contract a15 (b)) | `host_lease_minutes` in `config.yaml` |
| A cloud or human lease is `lease_minutes`, 120 | Unchanged | `lease_minutes` |
| A host name may be on several machines | Jeff's rule for `<capability>-any` (loop-contract a15 (b)); this design proposes the same for every host name (a6), to be confirmed at the review; nothing changes until Jeff lists one twice | `hosts.yaml`, on Jeff's word |
| No unattended worker works while the order is unconfirmed | The cloud's gate, made every worker's (a12) | `priority_confirmed: true` |

Named fallbacks (GLOBAL-11):

- **Lease expiry when no release ran.** Trigger: a session's leases
  were not released by its wrapper (power loss) and the machine has not
  run since. The step is held until its expiry, up to 20 hours after
  the last renew; `QUEUE.md`'s Workers section shows the lease and its
  machine, and the next run of that machine releases it with a history
  line "an earlier session on <machine> left it".
- **Lease expiry after an attended session closed without its Close.**
  Trigger: an attended `/workflow-native <host>` session
  (`host:<name>/<machine>.attended-<UTC>`) ended without running its
  Close's `wf release -session`. The wrapper's `-stale` release skips
  `attended-` sessions, so the step is held until its expiry, up to 20
  hours after the last renew; `QUEUE.md`'s Workers section shows the
  lease and its machine, and Jeff may release it with `wf release`.
- **A led item without a recorded session.** Trigger: `entire status`
  does not name the private remote. The machine takes no led step that
  run and its digest file says "sessions not recorded (FLOW-18)" first;
  `native/status/` reason carries it.
- **A lead dropped by an old `wf`.** Trigger: a session built before the
  merge saved a led item's file. The next seeded phase is `cloud`; the
  history shows the save without the lead, and `wf validate` reports
  nothing (the item is valid), so the fallback is visible only in the
  history and the Lead column. This is why `lead` is written only after
  `x4-state`'s check.

No merge of values: each setting has one source.

## Cross-driver impact

None. No driver tree, shared package or driver contract is reached. The
host names `<os>-<driver>` keep naming the drivers each machine runs.

## Rule changes

Seven amendments to `docs/development/process.md`, and one to
`docs/development/loop-profile.md` (PROFILE-01). Severities and checks
are unchanged. None edits an enforcement path under FLOW-14's rule-change
paths: the code is in `tools/workflow/queue/`, `tools/workflow/native/`
and `tools/workflow/cmd/wf/`, which are the loop's machinery, and this is a process
item, whose branch is exempt from the clearance (FLOW-14). Each is
approved by name at the design review (FLOW-19).

### FLOW-04 (amend)

**Why:** the one-machine rule is the one Jeff changed (a15 (b)); a lead,
the 20-hour lease and its renewal are new; the claim names the machine.

**Current wording**, the `host:<name>` bullet and the paragraph after
the list (`process.md:183-199`):

> - `host:<name>` — work that needs native hardware, for example `host:macos-vz`
>   (the macOS/arm64 laptop) or `host:windows-hyperv` (the Windows desktop). A
>   name `<os>-<driver>` is specific to that platform; `<capability>-any` (such as
>   `qemu-any` or `container-any`) runs on any native machine that has the
>   capability. The workflow branch's `hosts.yaml` gives each host name to exactly
>   one machine.
>
> A worker takes only steps whose affinity it satisfies. A step is offered when
> every step it needs is done and, unless it is `human`, its feature has reached
> the step's phase; no worker may claim or complete a host or cloud step of a
> later phase. A step is claimed with a
> lease (claimant plus expiry) recorded on the `workflow` branch, and only
> when FLOW-05's limits allow the claim, whoever the worker is. When the
> push of a claim is rejected because the branch moved, the claim is asked
> again on the new state: it stands if the step may still be claimed
> there, and otherwise it is lost and the worker re-reads state. An
> expired lease may be reclaimed.

**Proposed wording** ("one or more" for every host name stands only if
a6 option 1 is confirmed at the review; under option 2 the sentence
says that only a `<capability>-any` name may be served by more than one
machine):

> - `host:<name>` — work that needs native hardware, for example `host:macos-vz`
>   (the macOS/arm64 laptop) or `host:windows-hyperv` (the Windows desktop). A
>   name `<os>-<driver>` is specific to that platform; `<os>-any` needs the
>   platform and no one driver; `<capability>-any` (such as `qemu-any` or
>   `container-any`) runs on any native machine that has the capability. The
>   workflow branch's `hosts.yaml` lists the machines that serve each host
>   name, one or more, and only the human changes it.
>
> A feature may be **led** by a host name, which any worker may set or
> change, each change recorded in the item's history and the run's digest
> (FLOW-09). Its steps of Branch setup through Guide that would be `cloud`
> carry that `host:<name>` instead; its design review stays `human`, and its
> Publish, Merge and Close stay `cloud`. A claim by a worker that serves the
> lead starts a queued led feature, and no other claim does.
>
> A worker takes only steps whose affinity it satisfies: a machine satisfies
> each host name the table lists for it, and a `host:<name>` step goes to
> whichever such worker next takes steps. A step is offered when
> every step it needs is done and, unless it is `human`, its feature has reached
> the step's phase; no worker may claim or complete a host or cloud step of a
> later phase. A step is claimed with a
> lease (claimant plus expiry) recorded on the `workflow` branch, and only
> when FLOW-05's limits allow the claim, whoever the worker is. A claim
> names the worker's kind and session, and a machine's session names the
> machine. When the
> push of a claim is rejected because the branch moved, the claim is asked
> again on the new state: it stands if the step may still be claimed
> there, and otherwise it is lost and the worker re-reads state. A
> machine's lease lasts 20 hours, or what the human sets in `config.yaml`;
> its session renews it while it is actively working on the step, and a
> renewal counts nothing against the item's budget. A `cloud` or `human`
> lease lasts what the human sets, 120 minutes when he sets none. When a
> machine's scheduled session ends, the machine releases every lease that
> session still holds, and a machine releases at its next run any lease an
> earlier scheduled session of it left; an attended session releases its
> leases at its close, and a lease it leaves runs to its expiry. An expired
> lease may be reclaimed.

**Severity and check:** MUST, `automated`, before and after: `wf validate`
(`validateNative`, the lead checks), `Claim`, `wf renew`, `Starts`.

### FLOW-05 (amend)

**Why:** a machine now takes an item's steps in order and blocks on a
`cloud` step (a25).

**Current wording** (`process.md:231-233`, one clause):

> Otherwise a worker takes the next ready step
> of the first item in that order, and keeps taking that item's steps until it
> blocks: on a `human` or `host:<name>` step, an exhausted budget, a limit
> below, or the human's merge (FLOW-08).

**Proposed wording:**

> Otherwise a worker takes the next ready step
> of the first item in that order, and keeps taking that item's steps until it
> blocks: on a step whose affinity it does not satisfy (FLOW-04), an
> exhausted budget, a limit below, or the human's merge (FLOW-08).

**Severity and check:** SHOULD, `automated`, unchanged; `Next` already
behaves so.

### FLOW-09 (amend)

**Why:** a lead replaces the per-phase host affinity that `wf` could not
keep (Problem 1), and any worker may set it (a5); a process item is
never led, and its native work is attended or tracked (a4); the scheduled
session now honours the priority gate (a12).

**Current wording**, the scheduled-session sentence (`process.md:418-423`)
and the last paragraph (`process.md:441-442`):

> Native sessions drain their host's steps and backlog in batches, testing `main` plus
> every waiting branch in one sitting rather than one session per feature. Each
> native machine also runs a scheduled session that starts an agent only when a
> host step it serves is ready, or when `main` or a waiting branch has moved since
> its last sweep of open backlog entries; it is bound by the unattended loop's limits
> (FLOW-13) and records what it did in the digest.

> Work whose design or implementation itself needs the native host (for example a
> platform or hypervisor update) carries `host:<name>` affinity on those phases too.

**Proposed wording:**

> Native sessions drain their host's steps and backlog in batches, testing `main` plus
> every waiting branch in one sitting rather than one session per feature. Each
> native machine also runs a scheduled session that starts an agent only when a
> host step it serves is ready, or when `main` or a waiting branch has moved since
> its last sweep of open backlog entries, and never while the human has not
> confirmed the priority order; it starts from trunk, is bound by the
> unattended loop's limits (FLOW-13) and records what it did in the digest.

> A feature whose design or implementation itself needs a native host (for
> example a platform or hypervisor update, or a driver only that platform
> builds) is **led** by that host from its first step (FLOW-04): any worker
> may name the host name as the item's lead, at intake or later, and a
> machine that serves it takes the item's steps through Guide that would
> otherwise be `cloud` (FLOW-04). Its Publish runs on the cloud, and its
> native validation is blocking or tracked as its design says. A process
> item's native work is done in an attended native session or tracked as
> native gaps, as its design says; it is never led.

**Severity and check:** MUST, `automated + agent-judgment`, unchanged:
`wf gate`, `Seed`; the design review judges a lead.

### FLOW-13 (amend)

**Why:** a machine's session is an unattended run of its own, and the
cloud's guards and gate must bind it (Problem 5; a12, a13, a14).

**Current wording** (`process.md:526-543`):

> The unattended loop runs once a day. Each run takes steps one at a time (read
> state, claim, work, write state) until no `cloud` step is ready or the run's
> budget is spent, and writes state after every step so an interrupted run loses at
> most the step in progress. All state it needs lives in the repository, never in a
> long-running conversation. A run spends its budget in FLOW-05 order, so the
> first item in FLOW-05's order can use all of it. A scheduled native session (FLOW-09)
> spends its own budget, `native_budget`, which falls back to the run's budget when
> unset, so native and integration testing never compete with the cloud loop. Each
> feature also has a work budget, which is a runaway check rather than a share of
> the run: an open (queued or in-progress) item that spends `runaway_factor` times
> its estimated steps escalates to a `human` step, and the unattended loop takes no
> more of its steps. Runs are counted but never trip it. The loop posts a periodic
> digest (what moved, what is blocked on a human, what is waiting on a native host,
> what open items spent), and notifies when a new blocking question appears. Done
> items are left out of the runaway check, of the digest's blocked and spent lines,
> of `wf status` and of `QUEUE.md`; their budgets are looked at again only in a
> retrospective the human asks for. Reading archived designs (FLOW-01, FLOW-12) and
> open native gaps from merged designs (FLOW-09) are not affected.

**Proposed wording:**

> The unattended loop runs once a day in the cloud, and each native machine runs
> its scheduled session on its own schedule (FLOW-09). Each run takes steps one
> at a time (read state, claim, work, write state) until no step it may take is
> ready or the run's budget is spent, and writes state after every step so an
> interrupted run loses at most the step in progress. All state it needs lives in
> the repository, never in a long-running conversation. No unattended run takes a
> step while the human has not confirmed the priority order. A run spends its
> budget in FLOW-05 order, so the first item in FLOW-05's order can use all of it.
> The cloud run spends `run_budget`. A scheduled native session spends its own
> budget, `native_budget`, which falls back to `run_budget` when unset, so
> native and integration testing never compete with the cloud loop; a step it
> takes for an item a machine leads counts against it like any other. Each
> feature also has a work budget, which is a runaway check rather than a share of
> the run: an open (queued or in-progress) item that spends `runaway_factor` times
> its estimated steps escalates to a `human` step, and no unattended run takes
> more of its steps, nor any step of an item with an open blocking question
> (FLOW-07). Every claim counts against the work budget, whichever worker makes
> it, except the first claim of a refresh (FLOW-10); renewing a lease does not
> (FLOW-04). Runs are counted but never trip it. The
> loop posts a periodic
> digest (what moved, what is blocked on a human, what is waiting on a native host,
> what open items spent), and notifies when a new blocking question appears. Done
> items are left out of the runaway check, of the digest's blocked and spent lines,
> of `wf status` and of `QUEUE.md`; their budgets are looked at again only in a
> retrospective the human asks for. Reading archived designs (FLOW-01, FLOW-12) and
> open native gaps from merged designs (FLOW-09) are not affected.

**Severity and check:** SHOULD, `automated`, unchanged: `unattendedHold`,
`Gate`, `wf budget`.

### FLOW-16 (amend)

**Why:** Close runs on the cloud and cannot remove a machine's worktrees
of the item (Decision 11); the new bullet is an explicit exception to
the lead-in's "the Close phase removes".

**Current wording**, the first bullet (`process.md:632-635`):

> - its branch, on the remote and locally, and any worktree checked out on it.
>   GitHub deletes the remote branch on merge (the repository's "Automatically
>   delete head branches" setting); Close confirms it is gone, and escalates to a
>   `human` step with the exact command if it is not;

**Proposed wording** (the bullet, and a new bullet after it):

> - its branch, on the remote and locally, and any worktree checked out on it.
>   GitHub deletes the remote branch on merge (the repository's "Automatically
>   delete head branches" setting); Close confirms it is gone, and escalates to a
>   `human` step with the exact command if it is not;
> - on each native machine, the worktrees and local branch its sessions made for
>   the item: not by Close, which cannot reach them, but by the machine at its
>   next scheduled run once the item is done or its branch is gone from the
>   remote, with `git worktree remove` and `git branch -d` only, reporting a
>   worktree with uncommitted changes or a branch `-d` refuses instead of
>   removing it;

**Severity and check:** MUST, `automated + agent-judgment`, unchanged.
`loop-parallel-rule-wording` (queued, priority 62) amends FLOW-16's merge
proof paragraph (`process.md:648-660`); the two touch different
paragraphs, and whichever merges second takes the other's text when it
refreshes (FLOW-10).

### FLOW-17 (amend)

**Why:** a machine's push route (Decision 10).

**Current wording**, the second paragraph (`process.md:677-679`):

> Opening the PR to `main` needs no confirmation either (FLOW-08). Pushing
> never includes `main` itself, a force-push, rewriting history that is already on
> the remote, or deleting a remote branch (FLOW-16).

**Proposed wording:**

> Opening the PR to `main` needs no confirmation either (FLOW-08). Pushing
> never includes `main` itself, a force-push, rewriting history that is already on
> the remote, or deleting a remote branch (FLOW-16). An unattended session on a
> native machine pushes a feature branch only through the loop's push script
> on trunk, which pushes one branch of an item in progress, fast-forward only,
> and the queue state only through the loop's state commit; it runs no other
> push.

**Severity and check:** MUST, `agent-judgment`, unchanged; the push
script and the settings' denies are its mechanical guard, and
`native_test.go` checks the denies.

### FLOW-18 (amend)

**Why:** a machine's led design is what Jeff's review reads the reasons
of (Decision 12, a19).

**Current wording**, the first paragraph (`process.md:688-693`):

> Agent sessions are recorded with the Entire Checkpoints CLI, which links each
> session's prompts and transcript to the commits it produced. Recording serves
> three uses: a daily run resumes the previous run's session instead of starting
> cold (`entire session resume`), the Review phase and the human's design review and
> merge read why a
> change was made alongside the diff, and past sessions become searchable memory.

**Proposed wording** (the paragraph, and one sentence after it):

> Agent sessions are recorded with the Entire Checkpoints CLI, which links each
> session's prompts and transcript to the commits it produced. Recording serves
> three uses: a daily run resumes the previous run's session instead of starting
> cold (`entire session resume`), the Review phase and the human's design review and
> merge read why a
> change was made alongside the diff, and past sessions become searchable memory.
> Every worker records its sessions this way, on whichever machine it runs. A
> native machine whose session cannot be recorded takes no step of an item it
> leads in that run, and its report says so.

**Severity and check:** MUST, `automated + agent-judgment`, unchanged.

### PROFILE-01 (amend)

**Why:** an `<os>-any` name (a7, FLOW-04 as amended) serves platform work
that has no driver.

**Current wording**, the Native hosts row (`docs/development/loop-profile.md:29`):

> | Native hosts | The host names in the loop's `hosts.yaml`. All of them serve this repository's drivers today. |

**Proposed wording:**

> | Native hosts | The host names in the loop's `hosts.yaml`. All of them serve this repository's drivers or its platform builds today. |

**Severity and check:** MUST, `agent-judgment`, unchanged.

### The contract's citations, checked

- **LOOP-05**, "Routed by affinity": "One table, kept beside the queue,
  says which host names each machine serves ... Work that any machine
  with a capability can do carries `<capability>-any`. FLOW-04 says
  which worker may take a step." Still true: the table lists several
  machines for a name, and FLOW-04 says whichever serving worker next
  takes steps. "Identified": the claim records kind, host name and
  session, and the session now names the machine. "Scheduled and
  bounded": FLOW-13 now says it for every machine. "Set up by the
  owner": the installers are unchanged.
- **LOOP-04**: priority and placement are the human's, and stay so; it
  is not amended. A lead, which any worker may set (a5), is routing:
  it says which worker takes an item's steps, not where the item sits.
  The item's order and FLOW-05's limits are unchanged, and each change
  is a history line and a digest **Moved** entry, so the human sees it.
- **LOOP-06**: a machine's run writes its own digest file, as today.
- **LOOP-08**: the amended FLOW-17 names "the loop's push script" and
  "state commit", the loop's machinery, and no agent tool, CI system or
  language. FLOW-04 names `hosts.yaml` and `config.yaml`, the loop's
  state, as it did.
- **LOOP-09**: this is a process item; the wrapper still starts from
  trunk, and the cloud tick now checks it does (a21).
- **PROFILE-01**: the Native hosts entry (`loop-profile.md:29`) would be
  made false by an `<os>-any` name, which serves no driver; it is amended
  under **PROFILE-01 (amend)** above.

## Validation plan

- **Tier 0/1** (`make test`, `make lint` in `tools/workflow`, as TEST-01):
  - `queue/phases_test.go`: `Seed` on a led feature gives phases 0-5 the
    lead and phases 6-8 `cloud`; the design review stays `human`; `Back`
    to Design re-seeds with the lead; a feature with no lead is
    unchanged (every existing case).
  - `queue/queue_test.go` / `limits_test.go` / `process_test.go`: a host worker that serves
    the lead starts a queued led feature, under the guard and the
    waiting limit; the cloud does not start it and reports the host step;
    a host worker that does not serve the lead is not offered it; an open
    blocking question and a runaway budget stop a host worker; a
    `human/<name>` worker is not stopped by them; a `human/<name>` claim
    still starts a queued process item and a queued tracked item; a
    `cloud` claim still starts a queued feature with no lead.
  - `queue/commit_test.go`: a host worker `host:<name>/<machine>.<run>`
    claims a step. Meanwhile another session pushes an open blocking
    question on that item, so the claim's push is rejected. `wf commit`
    refuses the claim on the new state with exit 4 ("the claim of
    <item>/<step> is refused on the new state: ..."), with a reason
    naming the hold. A matching case with a spent runaway budget is also
    refused. A `human` worker in the same situation is claimed again.
  - `queue/hosts_test.go`: two machines serving one name validate; a
    step, gap or lead no machine serves fails; `Gate` is idle while
    `priority_confirmed` is false, with the reason; both machines' gates
    see the same ready step; a step pending with `host:h/m.<old>` holding
    a live lease is not offered by `Gate` (`Run` false, with no gaps),
    and after `wf release -machine m -stale -current m.<new>` `Gate`
    offers it (`Run` true, with a `step` reason).
  - `queue/queue_test.go`: `Claim` gives a host worker
    `host_lease_minutes` (1200 unset) and a cloud worker `lease_minutes`;
    `lead` on a process or tracked item fails `Validate`.
  - `tools/workflow/cmd/wf`: `wf renew` (held by W, by another, expired, done), `wf
    release -session` and `-machine -stale` (the current run is spared),
    `wf lead` (by a `human/<name>`, a `cloud/<run>` and a
    `host:<name>/<machine>.<run>` worker alike, none refused; leased
    steps keep their affinity; one history line written, naming the
    worker, the old and new lead and the steps changed and left), `wf new
    -lead` (refused on a process item; any worker kind accepted; the lead
    in the first history line), each pushed through `wf commit`
    against a bare remote as `commit_test.go` does; a rejected renew is
    asked again through `-renewed`, leaves `budget.spent.steps` and
    `runs` unchanged, writes a "renewed" history line and no "claimed"
    line, keeps the host lease length, and is refused (exit 4) when the
    step was released or taken by another worker in between; the stale
    release before the gate end to end: after `wf release -machine M
    -stale -current M.r2`, a step leased to `host:h/M.r1` is offered by
    `Gate` for M, and a step leased to `host:h/M.attended-<UTC>` is
    not released.
  - `queue/summary_test.go`: the Lead column and the Workers section,
    with no clock read.
  - `native/native_test.go`: the denies (`git push*`, `git -C * push*`)
    and no `git push` allow; a `TestNativeSettingsRules` case that,
    using the test's own matcher reading, shows the old allow `Bash(git
    push origin feat/*)` matches `git push origin feat/x:workflow` and
    `git push origin feat/x :feat/other` (Problem 6's colon claim), and
    that the new denies `git push*` and `git -C * push*` match both;
    `native-push.sh`'s refusals and a push to a
    bare remote, and a non-fast-forward refused; the deny's false
    positive on a `-m` message containing ` push`, and no match on `-F
    ../x.msg`; the commit form run for real, `git -C .scratch/x commit -F
    ../x.msg` in a worktree `.scratch/x` of a throwaway repository,
    commits and leaves the worktree clean; `shellcheck` on the
    wrapper (covering the `session_started` guard) and the script as
    today; the wrapper's cleanup cases (a dirty `.scratch/` worktree, a
    squash-merged branch `git branch -d` refuses) if the cleanup is
    factored into a testable function.
  - The live queue state, read by the branch's `wf validate`, `wf next`
    for `cloud`, each host, and `wf status -md`, before and after: no
    item has a lead, so the only differences are the Workers section,
    the Lead column and the lease length a host claim would get.
- **Tier 2 / native.** No blocking validation (`native_validation: []`): the
  wrapper and the script run on each machine at its first scheduled run
  after the merge, and `x4-state` waits for both machines' `native/status/`
  to show a run after the merge (Holds). Tracked in the native-gap backlog
  at Close, as the three `tier2_gaps` above: the MacBook's launchd run and
  the Windows desktop's Git Bash run, each checking the `-stale` release,
  the worktree cleanup, and one `native-push.sh` push of a scratch branch to
  a throwaway remote. On each machine, also: a run that is killed while it
  holds a step leaves that step leased, and the next scheduled run's
  `-stale` release frees it before the gate, so the gate offers it (not
  idle) and the new session claims it; and a run while `priority_confirmed`
  is false starts no agent and shows "idle: the priority order is not
  confirmed (FLOW-13)" as the reason in `native/status/<machine>.yaml`. On
  the MacBook, also (the first `tier2_gaps` entry): a run with the private
  session remote unset takes no led step, and its digest file and
  `native/status/` say "sessions not recorded (FLOW-18)". And the cleanup
  (the third `tier2_gaps` entry): a dirty `.scratch/` worktree and a
  squash-merged branch that `git branch -d` refuses, each shown in
  `native/status/<machine>.yaml` after an idle run and after a worked run.
- **The first led item** is Jeff's to choose at `x4-state`; K3 is the
  natural one once it is filed. Until then the lead is exercised by the
  tests alone.

## Implementation plan

Each task cites the rules it must satisfy. All on `feat/loop-native-lead`.

1. **Rules.** The seven amendments in `docs/development/process.md`, word
   for word as approved (FLOW-04, -05, -09, -13, -16, -17, -18; FLOW-14),
   and PROFILE-01's Native hosts row in `docs/development/loop-profile.md`.
2. **The lead.** `Feature.Lead`, its validation, `Seed`, `Starts` and
   `takesStart`'s comment (`queue/limits.go:131-136`), which says only the
   cloud's claim sets a feature in progress, `wf new
   -lead`, `wf lead` (any worker kind; the history line), with tests
   (FLOW-04, FLOW-09; LOOP-04 unchanged, a lead being routing).
3. **Host names.** `validateNative` without the one-machine error; the
   lead check (FLOW-04, LOOP-05).
4. **The lease.** `host_lease_minutes`, `Claim`, `wf renew`, `wf commit
   -renewed` (the `renewRef` path beside `claimRef`, calling `Renew`
   through `renewAgain` rather than `claimAgain`/`Claim`), `wf release
   -session | -machine -stale -current <machine>.<run>` (sparing the
   current run and skipping `attended-` sessions), with tests
   (FLOW-04, FLOW-13).
5. **The guards.** `unattendedHold` in `wait` and in `claimAgain`, with
   tests; `Gate` and `priority_confirmed` (FLOW-07, FLOW-13).
6. **`QUEUE.md`.** The Lead column, the Workers section, `wf status`'s
   `lead:` line (FLOW-13, LOOP-06).
7. **The push script and the settings.** `native-push.sh`;
   `settings.native.json`'s allows and denies; `native_test.go`, with
   the `TestNativeSettingsRules` case that, by the test's own matcher
   reading, shows the old allow `Bash(git push origin feat/*)` matching
   `git push origin feat/x:workflow` and `git push origin feat/x
   :feat/other`, and the new denies `git push*` and `git -C * push*`
   matching both (a16) (FLOW-16, FLOW-17).
8. **The wrapper.** `native-tick.sh`: the session id with the machine, the
   `-stale` release after `build wf` and before the gate (stage `release`),
   and the `-session` release in the EXIT path (`on_exit`) before
   `record_status`, once the session has started; the worktree cleanup and
   its `leftover` notes; case 3 of the gate taking its `reason` from `wf
   gate`'s captured `idle` line; `entire status` and the prompt line; the
   installers' `check` line (FLOW-09, FLOW-16, FLOW-18, LOOP-05).
9. **Skills and agents.** `workflow-native` (worker id, the attended worker,
   its `<machine>` argument, and its Close release, worktrees, the push
   script, `wf renew` and `wf commit -renewed`, the Never list, the `commit
   -F` rule, sessions not recorded, and its Git on the workflow checkout
   paragraph (`SKILL.md:42-45`) without `push origin workflow`, leaving `git
   -C .dev/workflow <add|commit|...>` and saying the workflow branch is
   pushed only by `dist/wf commit` and a feature branch only by
   `tools/workflow/native/native-push.sh <name>`, since `Bash(git -C *
   push*)` denies every other push (a16), and each lead change its run
   made listed under **Moved** in its digest (a5)); `workflow-tick` (the
   trunk check; **Waiting on a host** lists leads; each lead change its
   run made listed under **Moved**); `workflow-intake` (`-lead` in place
   of the hand edit); `workflow-human` (`wf lead`); one sentence in each of
   the six phase agents (FLOW-14, LOOP-06).
10. **Archive.** This design archived on the branch with its summary;
    PR opened with CI green (FLOW-03 process phase 2, FLOW-12).
11. **After the merge** (`x4-state` (phase 4, `human`, added with its
    note when the draft's questions are recorded; `x4-close` needs it;
    a11), attended, on Jeff's word): the
    workflow branch's `README.md` (`lead`, `wf lead`, `wf renew`, `wf
    commit -renewed`,
    `host_lease_minutes`, the lease table); `hosts.yaml`'s header and
    any second machine or `<os>-any` name Jeff names (a11);
    `host_lease_minutes` in `config.yaml` on Jeff's word; the three
    native gaps in `backlog/native-gaps.yaml`, one per `tier2_gaps`
    entry; the first led item, on Jeff's word; once both machines have
    run on the new `main`.

## Design review

- **r1, 2026-11-17: approved at `74deb30`** (Jeff, workflow-human, FLOW-19),
  read at `f30b6b8` (the draft `c97caf0` with 21 confirmed corrections from
  a report-only check, applied before the review). Rule changes: **rule
  change FLOW-04 approved at `74deb30`** (approved with a change: "A feature
  may be **led** by a host name, which any worker may set or change, each
  change recorded in the item's history and the run's digest (FLOW-09)");
  **rule change FLOW-05 approved at `74deb30`**; **rule change FLOW-09
  approved at `74deb30`** (approved with a change: "any worker may name the
  host name as the item's lead, at intake or later", and the added
  sentence "A process item's native work is done in an attended native
  session or tracked as native gaps, as its design says; it is never
  led."); **rule change FLOW-13 approved at `74deb30`**; **rule change
  FLOW-16 approved at `74deb30`**; **rule change FLOW-17 approved at `74deb30`**;
  **rule change FLOW-18 approved at `74deb30`**; **rule change PROFILE-01
  approved at `74deb30`**. Assumptions: a5 answered **(2)**, refined: any
  worker (human, cloud or a machine) may set or clear a lead, each change
  logged in the item's history and the run's digest under **Moved**;
  LOOP-04 is not amended, a lead being routing, not priority or
  placement. a16 answered (1) with a `TestNativeSettingsRules` case for
  the colon reading. a10 (1), the attended session's machine from
  `hosts.yaml` or a second argument. Every other assumption, a1 to a4, a6
  to a9, a11 to a15 and a17 to a25, confirmed as recommended. Folded in at
  `756188e` and `74deb30`: the a5 and a16 answers, the two rule wordings above,
  and the process-item sentence through a4, Decision 3, Design, Defaults,
  the Validation and Implementation plans.

## Consolidation

A process item has no Publish phase and no consolidation pass (FLOW-03).
The implement step merges current `main` before it opens the PR, and its
open PR is refreshed like a feature's (FLOW-10).

## Rollback notes

Revert the PR. No queue file carries a `lead` or a `host_lease_minutes`
until `x4-state` writes one after the merge, so an older `wf` reads every
state this one writes before then. A machine that ran the new wrapper
picks the old one up at its next scheduled start (it resets to
`origin/main`); a lease it claimed for 20 hours before the revert runs to
its expiry or is released by hand (`wf release`).

## Archive summary

Implemented on 2026-11-18 as a process item (`x2-implement`), after design
review r1 approved the design and its eight rule changes at `74deb30` on
2026-11-17 (Jeff; FLOW-04 and FLOW-09 each with a change, FLOW-05, -13,
-16, -17, -18 and PROFILE-01 as drafted; a5 answered (2), refined; a16 (1)
with the colon-reading test case; a10 (1); every other assumption as
recommended). `main` was still at the fork point (`0cd3c02`) when the PR
was opened, so there was nothing to merge. It is P1's fourth item under
FLOW-20 (`componentization`).

- **Rule text** (`docs/development/process.md`,
  `docs/development/loop-profile.md`), word for word as approved: FLOW-04
  (the lead, which any worker may set or change, each change in the item's
  history and the run's digest; any host name served by several machines;
  a machine's 20-hour lease, renewed and released), FLOW-05, FLOW-09 (any
  worker may name the lead, at intake or later; a process item is never
  led), FLOW-13 (the guards bind every unattended run; each run spends its
  own budget), FLOW-16 (a machine's cleanup), FLOW-17 (the push script,
  the `-F` commit form), FLOW-18, and PROFILE-01's Native hosts row.
- **Built: the lead** (`queue/lead.go`, `queue/phases.go`,
  `queue/next.go`, `cmd/wf`). `Feature.Lead` (`lead: host:<name>`);
  `Seed` gives a led feature's `cloud` template steps of phases 0 to 5 to
  the lead, so a return re-seeds with it; `Starts` lets only a worker
  serving the lead start a queued led feature; `wf new -lead` and
  `wf lead -worker W -feature F -host <name> | -clear` for any worker kind,
  re-labelling pending, unleased template steps and writing a history
  line; `validateLead` refuses a lead on a process or tracked item and a
  lead no machine serves.
- **Built: host names** (`queue/hosts.go`). The one-machine error is gone
  from `validateNative`; every pending host step, open gap and lead must
  still name a served host.
- **Built: the lease** (`queue/lease.go`, `queue/next.go`,
  `queue/commit.go`). `host_lease_minutes` (1200 when unset, an error
  below 0) for a `host:` worker, `lease_minutes` for the others;
  `wf renew` (refuses a worker that does not hold the lease, counts
  nothing); `wf commit -renewed` through `renewAgain`; `wf release
  -session <machine>.<run>` and `wf release -machine <m> -stale -current
  <machine>.<run>`, sparing the current run and skipping `attended-`
  sessions. A machine's worker is `host:<name>/<machine>.<run>`.
- **Built: the guards.** `cloudHold` became `unattendedHold`, in `wait`
  and in `claimAgain`: an open blocking question and a spent runaway
  budget stop every worker but `human`; `wf gate` exits idle with the
  reason while `priority_confirmed` is false.
- **Built: `QUEUE.md`** (`queue/summary.go`). The Lead column, the
  Workers section, and `wf status`'s `lead:` line; golden files updated.
- **Built: the push script and the settings**
  (`tools/workflow/native/`). `native-push.sh <name>` pushes
  `.scratch/<name>`'s `feat/<item>` to the same name, one refspec, no
  `+`, refusing an item not in progress, a detached head, another branch
  name and extra arguments; the two push allows in `settings.native.json`
  replaced by the denies `Bash(git push*)` and `Bash(git -C * push*)`;
  `push_test.go` and the `TestNativeSettingsRules` colon cases (a16).
- **Built: the wrapper** (`native-tick.sh`, the installers). The session
  id carries the machine; a `release` stage (`-stale`) after `build wf`
  and before the gate, and the `-session` release in `on_exit` before
  `record_status`; the worktree cleanup with its `leftover` notes; the
  gate's case 3 reason from `wf gate`'s `idle` line; the `entire status`
  check and its prompt line; the installers check for the push script.
- **Skills and agents.** `workflow-native` (worker id, attended worker and
  its `<machine>` argument, Close release, worktrees, the push script,
  `wf renew`, `wf commit -renewed`, the Never list, `commit -F`, the Git
  paragraph without `push origin workflow`, lead changes under **Moved**);
  `workflow-tick` (the trunk check, **Waiting on a host** lists leads, lead
  changes under **Moved**); `workflow-intake` (`-lead`); `workflow-human`
  (`wf lead`); one sentence in each of the six phase agents.
- **Deviations from the plan, each for a reason.** (1) The cloud's `wf
  next` reports a queued led item as a blocked line, so the tick can list
  it under **Waiting on a host**; the design had it passed over. (2) The
  cleanup walks `.scratch/*/` rather than parsing `git worktree list
  --porcelain`, whose paths Git Bash spells differently. (3) The wrapper
  commits its releases as worker `native/<machine>.<run>`. (4)
  `lease_minutes <= 0` falls back to 120, as `host_lease_minutes <= 0`
  falls back to 1200. (5) `wf lead` and `wf new -lead` are tested on a
  state directory, not through `wf commit` against a bare remote.
- **Decisions taken.** A lead is a host name and `Seed` routes phases 0-5
  by it (1); only the lead's machines start a led item (2); only a feature
  is led, by any worker, every change logged (3, a5 (2)); any host name
  may be served by several machines (4); `<os>-any` names (5); the cloud
  is a capability worker only when the table lists it (6); a machine's
  20-hour lease, renewed and released (7); the claim names the machine
  (8); the guards bind every unattended worker (9); a machine pushes only
  through the push script on `main` (10); each machine cleans up after a
  done item (11); no led step without session recording (12); no worker
  is held (13).
- **Rejected.** Per-phase affinities by hand; a lead naming a machine; the
  lead taking Publish; only the human setting a lead; a heartbeat lease;
  the 20-hour lease for every claim; one deny per bad push form; no push
  change because `main` is protected.
- **For future designs touching `native-lead`, `lead`, `host-names`,
  `capability-any`, `lease`, `lease-renewal`, `wf-renew`,
  `native-wrapper`, `push-script`, `permission-denies`, `priority-gate`
  or `worker-identity`:** the push denies are a guard against an agent's
  mistake, not a sandbox (`make`, `go` and `pwsh` can still push); a
  machine's push goes only through `native-push.sh`, which lives on
  `main`, so changing it waits for a merge. A lease is held 20 hours only
  by a running session, a machine that lost power until its next run, or
  an attended session closed without its Close. A lead covers phases 0-5;
  Publish, Merge and Close stay `cloud`. Native validation of this item is
  tracked, not done: the three `tier2_gaps` are each machine's first
  scheduled runs after the merge. `loop-repository` moves this machinery
  next.
- **After the merge**, at `x4-state` (attended, on the `workflow` branch,
  once no live lease predates the merge commit and both machines'
  `native/status/` files show a run after it): `hosts.yaml`'s header and
  any second machine or `<os>-any` name on Jeff's word;
  `host_lease_minutes` in `config.yaml` on Jeff's word; `README.md`; the
  three native gaps in `backlog/native-gaps.yaml`; the first led item.
  `x4-close` must get `needs: [x4-state]` by hand when it is seeded.
