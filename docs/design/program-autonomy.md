---
feature: program-autonomy
status: archived
drivers: []
seams: []
surfaces: []
repos: []
merge_first: []
areas: [loop, rules]
security_boundary: none
alters_relationships: false
design_review: required  # touches rules FLOW-20 reserves to Jeff: merging, tags, budgets
native_validation: []
tier2_gaps: []
rules: [FLOW-04, FLOW-05, FLOW-08, FLOW-10, FLOW-11, FLOW-13, FLOW-14, FLOW-16, FLOW-17,
        FLOW-19, FLOW-20, LOOP-07, LOOP-09, LINT-03, GLOBAL-10]
rule_changes:
  - {rule: FLOW-08, change: amend, paths: [docs/development/process.md]}
  - {rule: FLOW-13, change: amend, paths: [docs/development/process.md]}
  - {rule: FLOW-17, change: amend, paths: [docs/development/process.md]}
  - {rule: FLOW-20, change: amend, paths: [docs/development/process.md]}
concepts: [workflow, merge-gate, program-trunk, loop-merge, pre-release, tag, budget, runaway,
           reserved-rules, process-test, tag-due, native-session]
related: [componentization.md, program-north-star.md, budgets-active-only.md,
          merge-confirm-attended.md, loop-native-lead.md]
---

# Program autonomy: program-trunk merges, pre-release tags and post-merge budgets

## Problem

Three things still stop the program on Jeff where there is nothing for him
to decide (Jeff, 2026-11-24):

1. **Merging.** FLOW-08 already lets the loop merge a program item's PR
   into its program trunk behind the merge gate, but FLOW-20 reserves to
   Jeff "any change to a rule about ... merging". Read by that word, every
   process item that touches the loop's program-trunk merge machinery goes
   to his review even when it admits nothing new: `wf-gate-rest` went to
   him only for that. Parallelism is growing (several cloud runs, eight
   repositories with a program trunk), and nothing yet says the loop
   merges into one program trunk one pull request at a time.
2. **Tags.** FLOW-17 and FLOW-20 let the loop push `-dev.N`, `-rc.N` and
   the final tag, but a cloud session's `git push` of a tag gets HTTP 403
   from the session's own git proxy, before GitHub's ruleset (no
   `X-Github-Request-Id` on the receive-pack response). Every tag
   `wf tag-next` names therefore waits for an attended session. The
   REST write paths a cloud session could use instead (`git/refs`,
   `git/tags`) are refused by the same proxy ("Write access to this
   GitHub API path is not permitted through this proxy"; r2's finding).
3. **Budgets.** FLOW-13 counts every claim against the work budget and
   holds every step of a runaway item. `relay-conformance-timeout`'s Close
   was held at 16 of 8 steps after its PR had merged: a hold on a step that
   changes nothing in the repositories, with one possible answer.

## Prior art

- `program-north-star.md` (FLOW-20): the loop merges into program trunks
  behind FLOW-08's merge gate, merge commits only, pinned to the head; the
  reserved-rules list and the process test. Kept; this narrows what the
  list reserves for program-trunk merges and pre-release tags.
- `wf-gate-rest` (open): the merge gate read over REST; its review went to
  Jeff because it touches merging. `sync-pr-tier1` (open): tier 1 on a
  sync PR; its review went to Jeff because it touches merging and relaxes
  a check.
- `budgets-active-only.md`: done items left out of the runaway check. This
  extends the same idea to the post-merge steps of open items.
- `merge-confirm-attended.md`: merge confirmations need no claim and spend
  no run budget. This makes it a rule and extends it to every step from
  Merge on.
- `wf tag-next` and `check/tags.go`: the pre-release naming and its
  checks. Kept; a tag recorded as due now counts as taken when the next
  one is named (D3).
- `loop-native-lead.md` and `native-push.sh`: the one push an unattended
  native session makes itself, through a script on trunk that refuses
  everything but its one case, under `settings.native.json`'s push
  denies. D3's tag script follows the same pattern. `program/records.yaml`
  (FLOW-20): the program's append-only records, which already carry a
  repository, a head and a ref, and a `tag` kind.

## Decisions

**D1. Program-trunk merges: keep the gate, add per-repository
serialization and a recheck under it.** The loop's merge into a program
trunk keeps all nine gate points, merge commits only, pinned to the head
it checked (`--match-head-commit`, or the REST merge's `sha`). Added:
- *One at a time per repository.* A loop merge step (`f7-merge`, an
  `f6-merged-<n>` on a program trunk, a program sync) is offered only while
  no other loop merge step in the same repository is claimed under a live
  lease. The claim is a state commit on the `workflow` branch, so two
  workers racing for it are serialized by that push; the loser re-reads
  and gets nothing.
- *Recheck under the claim.* Inside the claim the loop fetches the program
  trunk, runs the gate on that head (point 4 against it) and merges at
  once. A program trunk moves only by merged PRs, all of them loop merges
  under the same serialization, so nothing can move it between the gate
  and the merge; a head that moved fails GitHub's pinned merge.
- *Server-side belt (setup, Jeff).* Each program trunk's ruleset requires
  its checks and branches up to date, and no approving review (the loop
  never approves, FLOW-08). Not a fallback: a second guard on the same
  condition (Q3).
Escalations that stay Jeff's: gate point 7 (a security note with a
posture change), point 9 (rule changes he approved), a tier-2 MUST finding
the loop may not defer when the posture or contract would change, a sync
conflict the loop cannot resolve, a CI failure Publish cannot repair
(FLOW-11), and every merge to a trunk.
*Rejected:* GitHub's merge queue (not available for every repository on
the plan, and it rebuilds merge commits the gate did not check); a lock
file on the program trunk (a push the loop may not make); serializing
across repositories (no shared state to protect, slows the program).

**D2. Reserved rules: "merging" means merging to a trunk; "tags" means
release tags without a suffix.** Jeff made program-trunk merges and
pre-release tags the loop's (2026-11-24), as pushes of feature branches
already are. A change to how the loop takes them is reserved only by its
effect: when it removes or narrows a gate point, drops the one-at-a-time
rule, lets a tag move or be deleted, or by the other clauses (widening
authority, raising a budget, relaxing a check). Consequences:
- `wf-gate-rest` reads the same gate and admits nothing new; under the
  amendment it would take the process test. Its review is already seeded
  as Jeff's and the amendment lands only with this item's merge, so it
  stays with Jeff now (Q5).
- `sync-pr-tier1` still relaxes a MUST check on a PR the loop merges; that
  clause, not merging, keeps it with Jeff after the amendment too.
*Rejected:* dropping merging and tags from the list outright (a change
that weakened the gate would then pass the process test).

**D3. The cloud loop records each tag as due; the next native session
creates it.** (r2; replaces r1's REST route, which the cloud session's
proxy refuses, as it does a git push of a tag.)
- *Due.* After a loop merge on a program trunk the tick runs
  `wf tag-next` as today, with a new `-due -worker $W`: for each tag it
  names, it appends a `tag-due` record to `program/records.yaml` (repo,
  `head` = the exact commit it checked, `ref` = the tag) and the tick
  commits it with the step's state. The cloud pushes no tag and calls no
  tag API. A tag is *due* while it has a `tag-due` record and no `tag`
  record for the same repository and ref. `wf record` refuses a second
  `tag-due` or `tag` record for one repository and ref, and `tag-next`
  counts a due tag as taken (the next is `-dev.2` while `-dev.1` is due;
  a head with a due pre-release gets none), so a trunk that moves before
  the tag exists never names one twice.
- *Handed to the native session.* `wf gate` gains a reason `tag\t<repo>\t<tag>`
  for each due tag in a repository the machine lists in `hosts.yaml`,
  and a due tag wakes the machine as a ready step does, whatever the
  stamp. `native-tick.sh` passes those lines into the session's prompt as
  "Tags due: ...". A new `wf tags-due -state .dev/workflow -machine <m>`
  prints `<repo>\t<tag>\t<sha>\t<program trunk>` for the same set. Either
  machine that lists the repository may create it; there is no claim, as
  creation is create-only and the loser's push is refused (below).
- *Created.* The `workflow-native` skill, attended or unattended, does
  the due tags first, before the batch, outside the native budget (D4:
  nothing from Merge on counts). For each it runs
  `tools/workflow/native/native-tag.sh <owner>/<repo> <tag>`, a script on
  trunk like `native-push.sh`: it refuses a tag that `wf tags-due` does
  not list, a clone whose origin is another repository, and any further
  argument; fetches the clone's origin with its tags; refuses when the
  due commit is not in `origin/<program trunk>`; refuses when the remote
  already has the tag (`git ls-remote`); creates an annotated tag at the
  due commit (`git tag -a <tag> <sha> -m "<tag>: due at <record time>,
  wf tag-next"`), and execs `git push origin refs/tags/<tag>:refs/tags/<tag>`,
  one refspec, no `+`, no `--force`, so git refuses to replace a tag that
  appeared meanwhile. The push carries Jeff's credentials on the machine,
  as `native-push.sh`'s do, so it triggers `release.yml`. The session then
  runs `wf record -kind tag -repository R -head <sha> -ref <tag>` and
  commits it with `dist/wf commit`.
- *Refused.* A remote tag already at the due commit is another machine's:
  record `tag` (a refusal as a duplicate means it is recorded) and go on.
  At any other commit, or any other refusal: the tag stays due, the
  digest lists it under **Blocked on Jeff**, and Jeff is notified. No
  retry by another route (GLOBAL-10).
- *Permissions.* `settings.native.json` keeps every push deny; the script
  is the one new route, as `native-push.sh` is for branches. The exact
  allow entry is Jeff's (Q6).
- *Latency.* A tag waits for the next native run on a machine that lists
  its repository; `windows-desktop` is always on. A consumer's pin moves
  only to a tag that exists, so `-rc.N`'s "pinned at the latest
  pre-release" waits with it. Which machines list which repositories is
  Jeff's setup (Q7).
*Rejected:* the REST refs API or a git push from the cloud session (both
refused by its proxy, r2's finding; any workaround of the proxy rejected
outright); a tag workflow per repository dispatched over REST (as in r1:
nine privileged CI/release changes, an App token secret and the Actions
app's bypass); a `program/tags-due.yaml` list (a second file kept in step
with the records, which already hold repo, head and ref and the `tag`
record that closes a due one); a host step on the native-gap backlog (a
gap is keyed to one host while a tag needs a machine that lists a
repository, it wakes the gate only when a trunk moves or on the sweep,
and it counts against the native budget); a queue step on the item (the
tag belongs to the program trunk, not the item, which may be closed, and
a sync's tags have no item).

**D4. Budgets count only steps before Merge.** A claim counts against the
item's work budget, and the run's budget, only for a step before the
item's Merge phase (feature phase 7, process phase 3), excluding
`f6-merged-<n>` and the first claim of a refresh as today. From Merge on
(a merge or its confirmation, `f6-merged-<n>`, Close, a state step) and
the run's records and digest, nothing counts and the runaway check holds
nothing. The loop merge itself is exempt too: the gate decides it, it
does no work of its own, a failing one is bounded by FLOW-11, and holding
it lets an approved PR rot (Q4).
*Rejected:* exempting only confirmations (keeps Close held, today's case).

## Questions

### Blocking

None.

### Assumed

- **Q1. How the loop creates tags.** Flagged: tags are a reserved rule
  and the session policy is Jeff's. Options: (a) REST refs API from the
  cloud session; (b) a dispatched tag workflow per repository; (c) native
  sessions: the cloud records each tag `wf tag-next` names as due, and the
  next native session creates it with Jeff's credentials, create only
  (D3). Answer (Jeff, r2, 2026-11-24): (c). (a) was confirmed in r1 and
  then found refused by the cloud session's proxy, as is a git push of a
  tag.
- **Q2. Reading of the reserved list.** Flagged: departs from FLOW-20's
  reserved list as Jeff wrote it. Options: (a) narrow "merging" to a trunk
  and "tags" to release tags without a suffix, the loop's program-trunk
  merges and pre-release tags reserved only by effect (D2); (b) drop
  merging and tags from the list; (c) unchanged. Recommended: (a).
- **Q3. Program-trunk rulesets require up-to-date branches.** Flagged:
  setup on nine repositories, Jeff's. Options: (a) yes, with required
  checks and no approving review; (b) no, rely on the loop's
  serialization. Recommended: (a).
- **Q4. The loop merge step is outside the budget.** Options: (a) yes,
  every step from Merge on (D4); (b) only confirmations and Close.
  Recommended: (a).
- **Q5. `wf-gate-rest` and `sync-pr-tier1`.** Options: (a) both stay with
  Jeff now (the amendment is not on `main`); `wf-gate-rest`-like changes
  take the process test after this merges, `sync-pr-tier1`-like ones stay
  his (relaxed check); (b) re-route `wf-gate-rest` to the process test on
  Jeff's word now. Recommended: (a); he is in the session and it is one
  review.
- **Q6. The allow entry for the tag script.** Flagged: a new permission
  in `settings.native.json` is Jeff's. Options: (a) add
  `"Bash(tools/workflow/native/native-tag.sh:*)"` to `allow`, every deny
  unchanged; the script is already inside the existing
  `Bash(tools/workflow/*)`, and the named entry makes the tag route
  explicit, as the list names no other script; (b) rely on
  `Bash(tools/workflow/*)` alone, no change to the file; (c) allow
  `Bash(git -C .dev/checkouts/* push origin refs/tags/*)` and narrow the
  deny `Bash(git -C * push*)` so it does not shadow it (deny wins over
  allow): `*` spans spaces, so it would admit `--force`, a delete
  refspec and other refs, which is why `native-push.sh` exists.
  Recommended: (a).
- **Q7. Which machines create tags, and as whom.** Flagged: setup on
  Jeff's machines and rulesets, and `hosts.yaml` is his to edit. A
  machine creates tags only for the repositories it lists, from its clone
  at `.dev/checkouts/<owner>/<repo>`; today neither lists
  `buildwarden-specs` or `buildwarden-ai-workflow`, `windows-desktop` does
  not list the qemu and vz drivers, and `macbook` not the hyperv and
  firecracker ones. Options: (a) `windows-desktop` (always on) lists all
  nine repositories, `macbook` unchanged; (b) both machines list all nine
  (either creates any tag, at the cost of more backlog sweeps, since a
  listed trunk that moves re-sweeps the machine's open gaps); (c) add
  only specs and ai-workflow, to `windows-desktop`. In each, Jeff clones
  the added repositories on the machine, and each repository's
  release-tags ruleset lets his account create `-dev.N`, `-rc.N` and the
  program trunk's final tag (the Claude app's bypass from r1 is no longer
  used by the loop). Recommended: (a).

## Design

**Merging (D1).** `queue/next.go`: a loop merge step in repository R is
not offered while another item's loop merge step in R holds a live lease;
`wf next` prints why. `check/merge.go`: `MergeReady` fetches the program
trunk and runs point 4 against the fetched head, inside the claim.
`wf-publish.md` and the tick's skill say: merge at once after the gate,
pinned to the head, then `wf done`; never two merges in one repository at
once. Nothing else in the gate changes.

**Tags (D3).** Cloud side: `queue/records.go` gains the kind `tag-due`,
`Due(records, repo)`, and the refusal of a duplicate `tag-due` or `tag`
for one repository and ref. `check/tags.go`'s `TagNext` and `TagNextRC`
read due tags with the repository's tags (`nextPre`, the head's
pre-release check). `cmd/wf/main.go`: `tag-next -due -worker` appends a
`tag-due` record per tag at the head it checked; new `wf tags-due
-machine <m>`. The tick's skill replaces `git tag` / `git push origin
<tag>` with `dist/wf tag-next ... -due -worker $W`, committed with
`program/records.yaml`, and its Never list says the cloud creates no
tag; its digest lists the tags made due under **Moved**, and those due
longer than a day under **Notes**.
Native side: `queue/hosts.go`'s `Gate` adds a `tag` reason, with
`Run`, for each due tag in a repository the machine lists. `native-tick.sh`
adds the gate's `tag` lines to the prompt ("Tags due: <repo> <tag>, ...").
New `tools/workflow/native/native-tag.sh` (D3's checks; one line on
stderr and exit 2 for every refusal before anything runs; otherwise git
push is exec'd). `settings.native.json`: Q6. The `workflow-native`
skill: a **Tags** section before **Collect the batch** (run
`wf tags-due`, `native-tag.sh` and `wf record -kind tag` for each,
refusals as D3 says, attended too); its Never list's "no tag" becomes "no
tag but a due one, by `native-tag.sh`"; the digest's **Moved** lists each
tag created, **Blocked on Jeff** each refused. Naming, the `-rc`
conditions and never-move-or-delete are unchanged. No repository outside
this one changes; Q7's setup is outside the repositories.

**Budgets (D4).** `next.go`'s claim increments `Spent.Steps` only for a
budgeted step (new `Budgeted(f, st)`: phase before Merge, not
`f6-merged-<n>`, not a refresh's first claim). `Runaway` and
`NeedsReestimate` no longer hold a step that is not budgeted
(`limits.go`, `next.go`); the waiting-on-human count drops an item whose
only pending steps are unbudgeted. The tick's run budget skips the same
steps.

**What the change reaches** (for the reserved-rule reading):
- Rules amended: FLOW-08, FLOW-13, FLOW-17, FLOW-20 (below). Relied on,
  unedited: FLOW-04, FLOW-05, FLOW-10, FLOW-11, FLOW-14, FLOW-16, FLOW-19,
  LOOP-07, LOOP-09, LINT-03, GLOBAL-10.
- Machinery: `tools/workflow/check/merge.go`, `tools/workflow/check/tags.go`,
  `tools/workflow/queue/next.go`, `tools/workflow/queue/queue.go`,
  `tools/workflow/queue/limits.go`, `tools/workflow/queue/records.go`,
  `tools/workflow/queue/hosts.go`, `tools/workflow/cmd/wf/main.go`,
  `tools/workflow/native/native-tag.sh` (new),
  `tools/workflow/native/native-tick.sh`, their tests
  (`native_test.go` included); `.claude/agents/wf-publish.md`,
  `.claude/agents/wf-review.md` (P2's list),
  `.claude/skills/workflow-tick/SKILL.md`,
  `.claude/skills/workflow-native/SKILL.md`.
- Configuration: no `config.yaml` key changes (`run_budget`,
  `runaway_factor` keep their values; what they count narrows). No
  `.claude/settings*.json` change. `tools/workflow/native/settings.native.json`
  gains one allow entry (Q6), its denies unchanged. `hosts.yaml` on the
  workflow branch: Jeff's edit (Q7). Outside the repository, Jeff's
  setup: the clones and release-tags rulesets of Q7, and Q3's
  program-trunk rulesets. No CI or release workflow changes.
- Areas: `loop` and `rules`, as intake recorded; the area map needs no
  change. Only this repository.

## Program payoff

Figures from the workflow branch (`program/records.yaml`, the queue's
histories) and `wf-gate-rest`'s count from `componentization.md`:

- **Tags.** At least 21 loop merges remain over the program (wf-gate-rest's
  count: 13 phases plus the per-repository merges of K3, K4 and M6), each
  naming at least one `-dev.N`; plus one `-rc.N` and one final tag for each
  of the 9 repositories: **at least 39 tags**, each of which today needs an
  attended session because the cloud's push is refused; under D3 the next
  native run creates each without Jeff. At the program's
  pace of 3 to 5 loop merges a week, **3 to 5 interactions a week** that
  decide nothing.
- **Runaway holds after a merge.** One today (relay-conformance-timeout's
  Close, raised on Jeff's word with one possible answer); `budgets-active-only`
  was raised the same way with only "the merge and Close" left. About
  **1 a week**, rising with parallelism.
- **Reviews for merging alone.** One of the three process items filed on
  2026-11-23..24 (`wf-gate-rest`) went to Jeff only for touching merging:
  about **1 a week** while the loop's machinery is still being fixed.

Total about **5 to 7 interactions a week** removed, against this item's
cost: about 6 steps and one review (this sitting).

## Defaults and overrides

- The run's and the item's budgets now count only steps before Merge (D4);
  no override, `runaway_factor` and `run_budget` unchanged.
- Tags are created only by a native session's `native-tag.sh`, at the
  commit the `tag-due` record names (D3); no fallback to the cloud, the
  REST API or another push (GLOBAL-10): a refused tag stays due, shows
  under **Blocked on Jeff** and notifies him.
- Which machine creates a due tag: any that lists its repository in
  `hosts.yaml` (Q7); none is preferred.
- No other default, override, merge or fallback.

## Cross-driver impact

None: the change is to the loop's rules and machinery only.

## Rule changes

**FLOW-08** (MUST, automated; unchanged severity; check gains the
serialization in `wf next` and the recheck in `MergeReady`). Current:
"The loop merges with a merge commit, never a squash or a rebase, pinned
to the head it checked, and records the merge (FLOW-20)." Proposed: "The
loop merges with a merge commit, never a squash or a rebase, pinned to
the head it checked, and records the merge (FLOW-20). It merges into a
program trunk one pull request at a time per repository: no loop merge in
a repository is taken while another holds its claim, and under the claim
the loop reads the program trunk's head again, runs the gate on it and
merges at once." Why: D1.

**FLOW-13** (SHOULD, automated; unchanged). Current: "Every claim counts
against the work budget, whichever worker makes it, except the first claim
of a refresh (FLOW-10); renewing a lease does not (FLOW-04)." Proposed:
"Only a step that changes the item's repositories counts: every claim of
a step before the item's Merge phase counts against the work budget and
the run's, whichever worker makes it, except the first claim of a refresh
(FLOW-10); renewing a lease does not (FLOW-04). From Merge on (a merge or
its confirmation, an `f6-merged-<n>`, Close and its state step), and the
records and digest a run writes, nothing counts against either budget,
and the runaway check holds none of it." And "no unattended run takes
more of its steps" becomes "no unattended run takes more of its steps
before Merge". Why: D4. Enforcement: `Budgeted` in the claim and in the
runaway holds.

**FLOW-17** (MUST, agent-judgment; unchanged severity and check).
Current: "or a tag, except a pre-release tag or a program trunk's final
tag FLOW-20 lets the loop push, and a release tag an attended session
pushes on the human's word; no tag is moved or deleted. An unattended
session on a native machine pushes a feature branch only through the
loop's push script on trunk, which pushes one branch of an item in
progress, fast-forward only, and the queue state only through the loop's
state commit; it runs no other push." Proposed: "or a tag, except a
pre-release tag or a program trunk's final tag FLOW-20 lets the loop
create, and a release tag an attended session pushes on the human's
word; no tag is moved or deleted. The loop creates its tags only on a
native machine: a cloud session records each tag `wf tag-next` names as
due, at the commit it checked, and pushes none; the next native session
on a machine that lists the repository creates it, as an annotated tag
at that commit, through the loop's tag script on trunk, which only
creates a due tag that the remote does not have, and records it. An
unattended session on a native machine pushes a feature branch only
through the loop's push script on trunk, which pushes one branch of an
item in progress, fast-forward only, a due tag only through the loop's
tag script, and the queue state only through the loop's state commit; it
runs no other push." Why: D3. Enforcement: the cloud session's proxy
refuses every tag write; `settings.native.json` denies every push the
session runs itself; `native-tag.sh` refuses all but a due tag the
remote lacks and pushes one refspec without `+`; `wf record` refuses a
duplicate `tag-due` or `tag`; `tag-next` counts a due tag as taken.

**FLOW-20** (MUST, automated + agent-judgment; unchanged). Current: "...;
and any change to a rule about security, merging, pushing or tags."
Proposed: "...; and any change to a rule about security, merging to a
trunk, pushing, or release tags without a suffix. A merge into a program
trunk under FLOW-08's merge gate, and a pre-release or final tag this rule
lets the loop create, are the loop's: a change to how it takes them is
reserved only when it removes or narrows a point of the merge gate, lets
the loop take two merges into one program trunk at once, lets a tag move
or be deleted, or meets another clause above; a merge or tag the loop may
already take is not one it could not take before, whatever worker or
machinery takes it." Why: D2. Enforcement: `wf-review.md`'s P2 list
follows the wording.

**Enforcement files** (named here, not in `rule_changes.paths`, which FLOW-02
limits to rule documents, FLOW-14):
- FLOW-08: `tools/workflow/check/merge.go`, `tools/workflow/queue/next.go`,
  `.claude/agents/wf-publish.md`, `.claude/skills/workflow-tick/SKILL.md`.
- FLOW-13: `tools/workflow/queue/next.go`, `tools/workflow/queue/queue.go`,
  `tools/workflow/queue/limits.go`, `.claude/skills/workflow-tick/SKILL.md`.
- FLOW-17: `tools/workflow/cmd/wf/main.go`, `tools/workflow/check/tags.go`,
  `tools/workflow/queue/records.go`, `tools/workflow/queue/hosts.go`,
  `tools/workflow/native/native-tag.sh`, `tools/workflow/native/native-tick.sh`,
  `tools/workflow/native/settings.native.json`,
  `.claude/skills/workflow-tick/SKILL.md`, `.claude/skills/workflow-native/SKILL.md`.
- FLOW-20: `.claude/agents/wf-review.md` (P2's list).

## Validation plan

Tier 0: unit tests in `tools/workflow/queue` (a second loop merge in the
same repository not offered under a live lease, offered in another
repository; post-Merge claims not counted; a runaway item's Close
offered; `wf record` refuses a duplicate `tag-due` or `tag`; the gate
wakes a machine that lists a due tag's repository, with a `tag` reason,
and not one that does not; `tags-due` lists only due tags),
`tools/workflow/check` (the recheck reads the fetched head; `tag-next`
names `-dev.2` while `-dev.1` is due and none for a head with a due
pre-release), `cmd/wf` (`tag-next -due` writes one record per tag at the
checked head) and `tools/workflow/native` (`native-tag.sh` against a
local bare remote: creates an annotated tag at the due commit; refuses a
tag not due, one the remote has, a commit outside the program trunk,
another repository's clone and an extra argument; `native-tick.sh`
passes the gate's tag lines into the prompt). Tier 1: `make test`,
`make lint`. No native host (`native_validation: []`): `native-tag.sh`
is POSIX sh like `native-push.sh`, whose tests in `native_test.go` run in
tier 1 the same way; no tier-2 gap. Live: the first due tag after the
merge, created by the next native run once Q7's setup is in place.

## Implementation plan

Done in x2-implement-r2, one commit per task, each pushed green:

1. `queue`: `Budgeted(f, st)` and `MergePhase` (`phases.go`); the claim
   counts `Spent.Steps` only for a budgeted step (`next.go`); `Runaway` and
   `NeedsReestimate` hold an item only while it has a pending budgeted step
   (`queue.go`), and `unattendedHold` holds only a budgeted step, so the
   waiting-on-human count and `QUEUE.md` drop an item whose only pending
   steps are from Merge on (FLOW-13). Tests: `budget_test.go`, the summary
   golden (a runaway before Merge listed, the same spend at the merge step
   not). 5c475ad.
2. `queue/next.go`, `queue/commit.go`: `mergeBusy` refuses a loop merge
   step, in `wf next` (its blocked line names the other merge) and in
   `MayClaim`, while another loop merge step in one of its unmerged
   repositories holds a live lease (FLOW-08). Test:
   `TestLoopMergeOneAtATime`. 01ebc5c.
3. `check/merge.go`: point 4 fetches the program trunk before it reads it,
   so `wf check -step` under the claim checks the head the merge goes into
   (FLOW-08). Test: `TestMergeReadyFetchesTheProgramTrunk`. 4c9d5f8.
4. `queue/records.go`: the kind `tag-due` (repository, `-head`, `-ref`
   required), the refusal of a second `tag-due` or `tag` for one
   repository and tag, and `Due`. `check/tags.go`: `TagNext` and
   `TagNextRC` return the head they checked, count a due tag as taken and
   name none for a head with a pre-release due. `cmd/wf`: `tag-next -due
   -worker W` and `tags-due [-machine M]` (FLOW-17). Tests: `TestTagDue`,
   `TestTagNextCountsDueTags`, `TestTagNextDue`. 5d4f38c.
5. `queue/hosts.go`: the gate's `tag` reason; `native/native-tag.sh`;
   `native-tick.sh`'s "Tags due" in the prompt (`tags_due`);
   `settings.native.json`'s `Bash(tools/workflow/native/native-tag.sh:*)`,
   denies unchanged (Q6) (FLOW-17). Tests: `TestGateWakesForADueTag`,
   `TestNativeTag` (a local bare remote), `TestNativeTickTagsDue`,
   `TestNativeTagRule`, the matcher cases in `TestNativePushRules`.
   f83c1a2.
6. `process.md`: the FLOW-08, FLOW-13, FLOW-17 and FLOW-20 amendments, in
   the approved wording. bfc647f.
7. `.claude/`: `wf-publish.md` (one merge at a time, at once under the
   claim; no tag push), `wf-review.md` P2 (the amended reading), the tick's
   skill (tags made due, never created; the run budget counts only steps
   before Merge; Moved and Notes lines for due tags) and the native skill
   (a **Tags** section before the batch; its Never list) (FLOW-14).
   a9e4bd5.

Departures, both within D3: `wf tags-due` prints a fifth field, the time
the tag was recorded due, which `native-tag.sh` puts in the tag's message
(`due at <record time>`) and the tick's **Notes** reads for a tag due over
a day; and `native-tag.sh` creates a tag of the queue's own repository from
the checkout itself, as `native-push.sh`'s one-argument form does, since
the machine has no clone of it under `.dev/checkouts/`.

## State step

What x4-state does on the `workflow` branch after the merge (q7 (a),
Jeff's edit of `hosts.yaml`):

- `hosts.yaml`: `windows-desktop` lists all nine repositories under
  `repositories` (buildwarden, relay, ai-workflow, specs and the five
  drivers), so it creates every due tag; `macbook` unchanged.
- Check: `dist/wf validate -state .dev/workflow` passes, and `dist/wf
  native -state .dev/workflow -repo . -machine windows-desktop -check` on
  the machine once Jeff has cloned the added repositories there.

Outside the repositories, Jeff's setup: those clones on `windows-desktop`;
each repository's release-tags ruleset lets his account create `-dev.N`,
`-rc.N` and the program trunk's final tag (Q7); and the program-trunk
rulesets of Q3.

## Design review

- r1, 2026-11-24: approved at 8739c23. Jeff approved the design as read
  at 8739c23, in the attended session (workflow-human).
  - Each rule change was asked on its own:
    - rule change FLOW-08 approved at 8739c23
    - rule change FLOW-13 approved at 8739c23
    - rule change FLOW-17 approved at 8739c23
    - rule change FLOW-20 approved at 8739c23
  - q1 (a): tags are created with the REST refs API. Jeff confirms the
    session policy allows it.
  - q2 (a): the reserved reading is narrowed.
  - q3 yes: program-trunk rulesets require their checks and up-to-date
    branches, with no approving review. Jeff sets them up.
  - q4 and q5 as recommended (batch).
  - Nothing was folded in.
  - sitting: 17:59-18:08 UTC, asks: 9 (1 a batch covering q4 and q5)
- r2, 2026-11-24: changes, in the same sitting, after r1's approval.
  r1 approved at 8739c23 stands except q1 and D3 (Jeff, 2026-11-24, same
  sitting): cloud sessions cannot create tags at all. A git push of a tag
  gets HTTP 403 from the session's git proxy, and the REST write paths
  git/tags and git/refs are refused ('Write access to this GitHub API path
  is not permitted through this proxy'). Jeff chose native sessions: the
  cloud loop records each tag wf tag-next names as due, and the next macbook
  or windows-desktop native session creates it with Jeff's credentials, by
  git push of an annotated tag at the checked commit, create only, never
  moved or deleted. Redo D3, q1 and the FLOW-17 amend's wording for that
  route (the rest of r1, including the FLOW-08, FLOW-13 and FLOW-20 amends,
  stays approved), say how a due tag is recorded and handed to the native
  skill, and re-review only that delta.
  - Redone in x1-design-r2: D3 (due records in `program/records.yaml`,
    the gate's `tag` reason, `native-tag.sh`), Q1's answer (c), the
    FLOW-17 amendment's wording and paths, and what follows from D3 only:
    Problem item 2, Prior art, Design's Tags paragraph and what the change
    reaches, a Defaults line, the Validation and Implementation plans, and
    the Program payoff's tags line. New: Q6 (the allow entry) and Q7 (the
    machines and rulesets). Everything else is as approved at 8739c23.

- r3, 2026-11-24: approved at f0bddeb. Jeff re-reviewed only r2's delta
  (D3, q1, the FLOW-17 wording, q6 and q7) in the same sitting.
  - rule change FLOW-17 approved at f0bddeb, superseding its approval at
    8739c23. FLOW-08, FLOW-13 and FLOW-20 stand approved at 8739c23.
  - q1 (c): native sessions create due tags.
  - q6 (a): `Bash(tools/workflow/native/native-tag.sh:*)` is added to
    `settings.native.json`'s allow list; no deny changes.
  - q7 (a): windows-desktop lists all nine repositories and creates every
    tag; the macbook is unchanged. Jeff clones the added repositories.
  - read at f0bddeb
  - sitting: 18:13-18:16 UTC, asks: 5

- r4, 2026-11-24: approved at 94225f9. Implement found that tier 1's
  FLOW-02 check refuses machinery paths in `rule_changes.paths` (16 MUST).
  Fold-in at 94225f9: each entry's paths narrow to
  `docs/development/process.md`, and the enforcement files are named in the
  Rule changes section. The rule wording is unchanged.
  - rule change FLOW-08 approved at 94225f9
  - rule change FLOW-13 approved at 94225f9
  - rule change FLOW-17 approved at 94225f9
  - rule change FLOW-20 approved at 94225f9
  - Everything else stands as approved in r1 and r3.
  - read at 94225f9
  - sitting: 18:47-18:49 UTC, asks: 1

## Consolidation

A process item has no Publish phase and no consolidation pass (FLOW-03).
Nothing landed on `main` since the fork: the branch forked at `505da76`
(#143, relay-conformance-timeout), which was still `origin/main` when the
PR was opened, so no merge was needed.

## Rollback notes

Revert the PR's merge commit on `main`. The `workflow` branch then holds
two things the reverted `wf` does not know: `tag-due` records in
`program/records.yaml` (an unknown kind; remove them, or create each tag
on Jeff's word and record it as `tag`, before the next `wf validate`) and
`hosts.yaml`'s nine repositories for `windows-desktop` (harmless; keep or
narrow at Jeff's word). The run budgets and runaway holds go back to
counting every claim. Jeff's rulesets (Q3, Q7) need no change on a revert.

## Archive summary

Implemented 2026-11-24 on `feat/program-autonomy` as a process item
(`x2-implement-r2` and `x2-implement-r3`, cloud worker
`cloud/20261124T174900Z`), after design reviews r1 (`8739c23`), r3
(`f0bddeb`) and r4 (`94225f9`, the `rule_changes` paths fold-in), all
Jeff's in one attended sitting. One PR, this repository only.

- **Built: D1, one loop merge at a time per repository** (`queue/next.go`,
  `queue/commit.go`, `check/merge.go`). `mergeBusy` refuses a loop merge
  step (`f7-merge`, `f6-merged-<n>` on a program trunk) in `wf next` and
  `MayClaim` while another one in the same unmerged repository holds a live
  lease, and `wf next` names the other merge. Gate point 4 fetches the
  program trunk before reading it, so `wf check -step` under the claim checks
  the head the merge goes into. `wf-publish.md` and the tick merge at once
  after the gate, pinned to the head.
- **Built: D2, the reserved reading** (FLOW-20, `wf-review.md`'s P2 list).
  "Merging" is merging to a trunk and "tags" release tags without a suffix;
  program-trunk merges and pre-release tags are reserved only by effect.
- **Built: D3, due tags created by native sessions** (`queue/records.go`,
  `check/tags.go`, `cmd/wf`, `queue/hosts.go`, `native/native-tag.sh`,
  `native/native-tick.sh`, `settings.native.json`). The `tag-due` record
  kind, with a refusal of a second `tag-due` or `tag` for one repository and
  tag; `wf tag-next -due -worker W` records each tag it names at the head it
  checked, and counts a due tag as taken; `wf tags-due [-machine M]`; the
  native gate's `tag` reason wakes a machine that lists a due tag's
  repository; `native-tick.sh` passes "Tags due" into the prompt;
  `native-tag.sh` creates an annotated tag at the due commit, create only,
  one refspec without `+`, after its refusals; the allow entry
  `Bash(tools/workflow/native/native-tag.sh:*)`, denies unchanged (Q6). The
  tick makes tags due and creates none; the native skill does due tags first,
  outside its budget.
- **Built: D4, budgets before Merge** (`queue/phases.go`, `queue/next.go`,
  `queue/queue.go`, `queue/limits.go`). `Budgeted(f, st)`: a claim counts
  only for a step before the Merge phase, not `f6-merged-<n>`, not a
  refresh's first claim; `Runaway`, `NeedsReestimate` and the unattended
  hold apply only to a pending budgeted step, so `QUEUE.md` and the
  waiting-on-human count drop an item whose only pending steps are from
  Merge on. The tick's run budget skips the same steps.
- **Rule changes**, as approved at `94225f9` (FLOW-17's wording at
  `f0bddeb`, the others at `8739c23`): FLOW-08 (serialization and the
  recheck), FLOW-13 (budgets before Merge), FLOW-17 (tags created only by a
  native session's tag script), FLOW-20 (the reserved reading).
- **Deviations, within D3.** `wf tags-due` prints a fifth field, the time
  the tag was made due, which goes into the tag's message and the tick's
  Notes; `native-tag.sh` creates a tag of this repository from the checkout
  itself, as `native-push.sh`'s one-argument form does.
- **Decisions taken.** Keep all nine gate points and add per-repository
  serialization plus a recheck under the claim, with program-trunk rulesets
  as a second guard (D1, Q3); narrow the reserved list by effect (D2, Q2);
  due tags recorded in `program/records.yaml` and created by the next native
  run with Jeff's credentials (D3, Q1 (c)); every step from Merge on
  unbudgeted, the loop merge included (D4, Q4); `wf-gate-rest` and
  `sync-pr-tier1` stay with Jeff now (Q5); `windows-desktop` lists all nine
  repositories (Q7 (a)).
- **Rejected.** GitHub's merge queue, a lock file on the program trunk and
  cross-repository serialization (D1); dropping merging and tags from the
  reserved list (D2); the REST refs API and a git push of a tag from the
  cloud (both refused by the session's proxy, r2's finding), a dispatched tag
  workflow per repository, a separate `tags-due.yaml`, a native-gap host
  step and a queue step for tags (D3); exempting only confirmations from the
  budget (D4); a broadened git-push allow for native sessions (Q6 (c)).
- **Validated.** Tier 0 and tier 1 (`make test`, `make lint`), including
  `native-tag.sh` against a local bare remote (`TestNativeTag`). No native
  host and no tier-2 gap. Live: the first due tag after the merge, created
  by the next `windows-desktop` run once Jeff's setup is in place.
- **For future designs touching `workflow`, `merge-gate`, `program-trunk`,
  `loop-merge`, `pre-release`, `tag`, `budget`, `runaway`,
  `reserved-rules`, `process-test`, `tag-due` or `native-session`:** a
  cloud session creates no tag by any route; a tag the loop takes exists
  first as a `tag-due` record and is created only by `native-tag.sh`, and a
  due tag counts as taken for `tag-next`. Two loop merges never run in one
  repository at once, and the gate is re-run on the fetched program trunk
  under the claim. Budgets and runaway holds count only steps before Merge
  (`Budgeted`). A change to the loop's program-trunk merges or pre-release
  tags is Jeff's only by its effect on the gate, the serialization or
  tag immutability.
- **After the merge**, at `x4-state` (the State step section above):
  `hosts.yaml` lists all nine repositories for `windows-desktop`,
  `wf validate`, and Jeff's setup outside the repositories (the clones, the
  release-tags rulesets, the program-trunk rulesets).
