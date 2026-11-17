---
feature: loop-repository
status: draft
drivers: []              # the loop's tools, skills and rules; no driver tree is reached
seams: []                # no GLOBAL-08 seam changes; the Loop Contract is seam 10 of the
                         # umbrella, a document, and the profile file is part of it (LOOP-01)
surfaces: []             # `wf` is the loop's tool, not a user-facing surface of the product
repos: []                # works only in buildwarden/buildwarden; the relay adopts in its own item
areas: [loop, tier1, tier1-config, rules, designs]
security_boundary: none
alters_relationships: false  # no product component's role or interaction changes (FLOW-02);
                         # this changes where the loop's gates run
design_review: required  # a process item's design always gets its review (FLOW-03, FLOW-19)
native_validation: []    # the native wrapper's change is tested by its Go test and on each
                         # machine by its owner at re-install; see Validation plan
tier2_gaps: []
rules: [FLOW-02, FLOW-03, FLOW-04, FLOW-05, FLOW-06, FLOW-08, FLOW-09, FLOW-10, FLOW-11,
        FLOW-12, FLOW-13, FLOW-14, FLOW-15, FLOW-16, FLOW-17, FLOW-18, FLOW-19, FLOW-20,
        LOOP-01, LOOP-03, LOOP-04, LOOP-05, LOOP-06, LOOP-08, LOOP-09, PROFILE-01, LINT-01,
        LINT-03, LINT-04, LINT-05, GLOBAL-06, GLOBAL-09, GLOBAL-10, GLOBAL-11, UX-01,
        TEST-01, TEST-03]
rule_changes:
  - {rule: FLOW-03, change: amend, paths: [docs/development/process.md,
      tools/workflow/check/check.go, tools/workflow/check/check_test.go,
      tools/workflow/check/areas.go, tools/workflow/check/areas_test.go,
      tools/workflow/check/review.go]}
  - {rule: FLOW-04, change: amend, paths: [docs/development/process.md]}
  - {rule: FLOW-08, change: amend, paths: [docs/development/process.md,
      tools/workflow/check/check.go, tools/workflow/check/check_test.go,
      tools/workflow/check/mergeproof_test.go]}
  - {rule: FLOW-10, change: amend, paths: [docs/development/process.md]}
                                                      # its enforcement, `wf stale` in
                                                      # tools/workflow/cmd/wf/main.go, is
                                                      # machinery, which rule_changes may not
                                                      # name: it changes as this item's own
  - {rule: FLOW-16, change: amend, paths: [docs/development/process.md,
      tools/workflow/check/check.go, tools/workflow/check/check_test.go]}
  - {rule: FLOW-17, change: amend, paths: [docs/development/process.md]}
  - {rule: FLOW-02, change: amend, paths: [docs/development/process.md,
      docs/design/TEMPLATE.md, tools/workflow/check/check.go,
      tools/workflow/check/check_test.go, tools/tier1/coupling/frontmatter.go,
      tools/tier1/coupling/frontmatter_test.go, tools/workflow/check/frontmatter.go,
      tools/workflow/check/frontmatter_test.go]}
  - {rule: FLOW-12, change: amend, paths: [tools/tier1/coupling/frontmatter.go,
      tools/tier1/coupling/frontmatter_test.go, tools/workflow/check/frontmatter.go,
      tools/workflow/check/frontmatter_test.go]}      # enforcement only: the pre-standard
                                                      # handling moves with the check (a13 (b))
  - {rule: FLOW-14, change: amend, paths: [tools/workflow/check/rulechange.go,
      tools/workflow/check/workflows.go, tools/workflow/check/rulechange_test.go,
      tools/workflow/check/workflows_test.go]}
                                                      # enforcement only: no text change
  - {rule: LOOP-01, change: amend, paths: [docs/development/loop-contract.md,
      tools/workflow/check/profile.go, tools/workflow/check/profile_test.go]}
  - {rule: LOOP-08, change: amend, paths: [docs/development/loop-contract.md]}
  - {rule: PROFILE-01, change: amend, paths: [docs/development/loop-profile.md,
      docs/development/loop-profile.yaml]}
  - {rule: LINT-01, change: amend, paths: [tools/tier1/rules/rules.go,
      tools/tier1/rules/rules_test.go, tools/workflow/check/ruleindex.go,
      tools/workflow/check/ruleindex_test.go]}
  - {rule: LINT-05, change: amend, paths: [tools/tier1/diff/diff.go,
      tools/tier1/diff/diff_test.go, tools/tier1/rules.yaml,
      tools/workflow/check/suppression.go, tools/workflow/check/suppression_test.go]}
                                                      # LINT-01, LINT-05 and FLOW-02's check
                                                      # (a13 (b)): code moves, no rule text
                                                      # change; LINT-05's owner in rules.yaml
  - {rule: LINT-04, change: amend, paths: [tools/workflow/check/check.go,
      tools/workflow/check/check_test.go]}  # enforcement only: tier2Report checks the
                                            # report's heads cover every branch (a12)
concepts: [workflow, queue, one-queue, second-repository, component-profile, profile-file,
           work-list, branches-per-repository, pull-requests-per-repository, area-map,
           tracked-items, native-checkouts, private-repository, tier1-generic-checks,
           wave-1-gate, componentization, program]
related: [componentization.md, loop-contract.md, loop-parallel.md, alters-slot.md,
          seam-specs.md, native-host-loop.md, workflow-streamline.md,
          tier1-static-checks.md, design-approved-rule-changes.md]
---

# Work in a second repository from the one queue: `wf`'s gates and checks, branches and pull requests per repository, and the native side

A process item, and a program item under FLOW-20: P1's fifth item
([`componentization`](componentization.md), phase P1, wave 1). It follows
`loop-contract` (#119), `alters-slot` (#118) and `loop-parallel` (#121), all
merged, and `loop-parallel-rule-wording` and `loop-native-lead`, queued
ahead of it, both designed and their reviews approved on 2026-11-17, and
neither merged (a15). The move of the loop to its own repository is P1's
last item and comes after it (`loop-contract` a19). Under `loop-contract` a22, answered (1), wave 2
waits for what this item delivers: an item in this repository and an item
in the relay repository each completed from the one queue.

Words used throughout. An **item** is a queue item. **This repository** is
`buildwarden/buildwarden`, where the queue, `wf`, the skills and the rules
are today. A **second repository** is any other repository an item names in
`repos`. A repository is **adopted** once it has done LOOP-08's steps, and
then it is a component with a profile. The **proposal** is the loop change
proposal of 2026-11-14 (step 5 of its migration), a document of the attended
session that is not kept in the repository. Paths are at `0cd3c02`
(`origin/main` when this was drafted, 2026-11-17).

## Problem

`loop-parallel` gave the queue the repositories an item works in (`repos`,
`tools/workflow/queue/queue.go:99-102`) and showed them in `QUEUE.md`, and
stopped there: "`repos` is recorded and shown, and `config.yaml` names the
home repository. Gates still run only here" (`loop-parallel`, **To
`loop-repository`**, line 3690). Everything `wf` checks, it checks in one
checkout of this repository, with this repository's paths and commands.

1. **`wf` has one checkout.** `check.Env` holds one `Repo`, "a checkout of
   buildwarden/buildwarden with origin fetched"
   (`tools/workflow/check/check.go:21-30`), and every check runs `git` there
   (`checker.git`, `check.go:224-235`). The skills pass `-repo .` from the
   `buildwarden/buildwarden` checkout (`workflow-tick` step 1,
   `.claude/skills/workflow-tick/SKILL.md:38-53`). A branch in another
   repository is not in it.
2. **An item has one branch and one pull request.** `Feature.Branch` and
   `Feature.PR` (`queue.go:98`, `:129`, `:144-150`) are the home's. A work
   item that also changes the specs repository, such as `seam-specs` or the
   open `relay-collector-failure-fails-open`, has its second branch and pull
   request named only in steps' evidence and its design. `seam-specs`
   needed a `human` step, `f6-release-specs`, to record a merge the loop
   could not see, and its review granted it as an exception to FLOW-04 and
   FLOW-08 (its a15); `loop-parallel` then wrote the exception into both
   rules (FLOW-04 at `process.md:177-182`; FLOW-08 at `:379-383`).
3. **The gates are this repository's.** Every term of the profile that `wf`
   uses is written into its code. `loop-contract` listed them (its hand-off,
   point 4); as read at `0cd3c02`:

   | Profile term | Where `wf` or the skills fix it |
   |---|---|
   | Fast gate | `check.go:61-73` (`c.make("lint")`, `c.make("test")`); `check.go:270-285` runs `make <target>`; `wf-implement`, `wf-review`, `wf-publish` |
   | Trunk | `origin/main` in `check.go:165`, `:251`, `:298`, `:352-398`, `:540`, `:563`; `areas.go:28`; `review.go:189`; `rulechange.go:190`; `cmd/wf/main.go:263`, `:286`, `:940-972`, `:1145`, `:1173`; `queue/hosts.go:257-261` |
   | Designs and template | `cmd/wf/main.go:601` (`docs/design/<item>.md`, `feat/<item>`); `check.go:713` and `rulechange.go:314-321` (`docs/design/TEMPLATE.md`) |
   | Impact vocabulary | `Front.Problems`, `check.go:669-703`: the required keys, among them `drivers`, `surfaces`, `security_boundary` and `tier2_gaps`, and GLOBAL-06's four `security_boundary` values |
   | Reference documents | `check.go:70` (`docs/`) and `:81-82` (the guides) |
   | Rule and machinery paths | `kindOf`, `rulechange.go:37-46`; the rule-running jobs, `workflowRunsRe`, `workflows.go:13-18` |
   | Areas | the edges come from `tools/tier1/area-edges.sh`, which the tick pipes in (`workflow-tick` step 2) |
   | Trunk CI and ruleset | `workflow-tick` step 2: `gh api repos/buildwarden/buildwarden/commits/<sha>/check-runs` and `gh api repos/buildwarden/buildwarden/rules/branches/main`, recorded by `wf main` into the root `main.yaml` (`SKILL.md:79-87`, again at `:115-116`) |
   | Component | `install-macos.sh:93-94` and `install-windows.ps1:115` (`origin` must be `buildwarden/buildwarden`) |
   | Native assets | `native-tick.sh:223-240` runs `tools/relay-vm/hyperv/refresh-cache.sh`, a product script inside loop machinery |

   The relay has no `make lint`, no `docs/book/`, no drivers and no
   `security_boundary` values of GLOBAL-06's. Read through LOOP-01, every
   one of these is its profile's entry; `wf` cannot read a profile.

   FLOW-18 for a second repository (where its agent sessions are recorded)
   is out of scope. `flow18-check.sh` stays this repository's, and a later
   process item adds `sessions.remote` with its reader.
4. **An item homed in a second repository gets no gates at all.** It is
   *tracked*: two `human` steps and no done-when check (FLOW-03,
   `process.md:157-161`; `check.go:46-52`). That was `loop-parallel`'s
   placeholder until this item (its a7: "nothing is enforced until
   `loop-repository` gives the relay's work steps and gates"). The relay's
   M1 cannot be worked from the queue in any other way.
5. **The state holds one map and one trunk.** `areas.yaml` is "the area map
   of `config.yaml`'s `repository`" and `main.yaml` records one `main`
   (the `workflow` branch's `README.md`, **Layout**). `wf stale`, `wf main`
   and `wf areas -check` read one repository. FLOW-05 already says a map is
   per repository and a repository with no map is one exclusive area
   (`process.md:241-252`).
6. **The native machines have one checkout.** `native-tick.sh` resets one
   dedicated checkout of this repository to `origin/main` and runs `wf gate`
   on it (`native-tick.sh:6-7`, `:185-211`). The gate's fingerprint hashes
   that checkout's `origin/main` and branches (`queue/hosts.go:257-279`). A
   host step in the relay repository has no checkout to run in and no
   toolchain to run with.
7. **The relay is private, and the queue is public.** The `workflow` branch,
   its digests and its tier-2 reports are in a public repository. As read on
   2026-11-14 the relay repository is private (`loop-contract` a21), until
   its M1's scan (`loop-contract`, hand-off point 5). What of
   a private repository's work may be written to the public state was left
   to this item "with the relay design's a17" (`loop-contract`, hand-off
   point 5). Jeff answered q1 (C) on 2026-11-17: it is published as a
   public repository's is, from its first item, since the relay goes public
   at M1 (Decision 19).
8. **Tier 1's generic checks are in Go analyzers' company.** `rule-index`,
   `design-front-matter` and `suppression` enforce loop rules (LINT-01,
   FLOW-02, LINT-05) and are what the relay needs for LOOP-03's Process
   class; they live in `tools/tier1/`, beside the Go analyzers the relay
   cannot use (`loop-contract`, hand-off point 5). `design-front-matter`
   calls `wf`'s own `check.Front.Problems`
   (`tools/tier1/coupling/frontmatter.go:12`), but the check itself
   (`Validate`, `TouchedDesigns`, `AllDesigns`, FLOW-12's pre-standard rule
   and `design_tolerate`) is in `tools/tier1/coupling/`.

## Prior art

- **[`loop-contract`](loop-contract.md)** (#119). Its hand-off row for this
  item and points 4 and 5 are the scope: the profile's terms as what `wf`
  stops hard-coding; LOOP-08's six steps as what the relay does to adopt; a
  branch and a pull request per repository and LOOP-06's head per
  repository; the native side; a private repository's work in the public
  state; where tier 1's generic checks live; how a session reaches a second
  repository's checkout while `wf`, the skills and the agents are here. Its
  a17 (an item may work in a repository that has not adopted; there the
  worker is bound by FLOW-08, -16, -17 and -18), a21 (protection where the
  host offers it) and a22 (wave 2 waits for this item, not for the move)
  are answered and not asked again.
- **[`loop-parallel`](loop-parallel.md)** (#121). Kept as built: `repos`,
  its home first; `areas` and the map per repository in FLOW-05; the
  relations; the tracked round; `config.yaml`'s `repository`. Its hand-off
  **To `loop-repository`** (lines 3688-3711) is the rest of the scope: the
  tracked round replaced by real phases for an item homed elsewhere; one map
  per repository with areas prefixed by repository; `wf stale`, `wf main`
  and `wf areas -check` per repository; a hold that may name repositories
  (its Decision 18 left it here: "No step of a second repository is taken
  by a worker a hold could stop until `loop-repository`, which adds it
  with the first such step", `loop-parallel.md:458-460`), with the cost the
  hand-off names (`:3704-3707`): while a hold on `cloud` lasts, no refresh
  is taken, so no second merge, in any repository the held worker serves;
  a `human` merge-recording step not offered
  before its phase (`seam-specs`' a15). Its Decision 17 (an absent field
  means today's behaviour, so the merge holds no worker) is the model for
  a10 here.
- **[`seam-specs`](seam-specs.md)** (#125). The working example of an item
  in two repositories: **How the loop treats the second repository**
  (lines 3199-3250) lists what binds there and what `wf` does not do; its
  a15's `f6-release-specs` is the step this item makes unnecessary for a
  repository the loop can read.
- **[`alters-slot`](alters-slot.md)**. The shape of a small process item's
  questions and its "What this item does not do". Nothing of its counting
  changes here.
- **[`native-host-loop`](native-host-loop.md)** and
  **[`workflow-streamline`](workflow-streamline.md)**: the native wrapper,
  its dedicated checkout and its gate stamp, which a10 and a14 extend.
- **[`tier1-static-checks`](tier1-static-checks.md)**: the three generic
  checks and the `replace` through which tier 1 imports
  `tools/workflow/check` (`tools/tier1/go.mod:5`).
- **The proposal**, step 5: the `work` list and second-repository gates.
  It is not in the repository; where it and the code disagree the code is
  followed.

## Decisions

1. **The profile has a file the loop's tools read** (a1). Each adopted
   repository keeps, beside its profile, a YAML file with the entries `wf`
   needs: branch pattern, designs, fast gate, impact vocabulary,
   reference documents, rule and machinery paths, the edges command and
   native preparation, and its trunk, which must equal the trunk its
   `config.yaml` entry gives (Decision 3). This repository's is
   `docs/development/loop-profile.yaml`. LOOP-01 says the file is part of
   the profile, so a change to it is a change to the profile. *Rejected:*
   parsing the Markdown table (its entries are prose and name files that
   hold facts, PROFILE-01, `loop-profile.md:8-11`); writing the entries
   into the queue state's `config.yaml` (a component's own checks would
   then change outside its repository, against LOOP-09's last paragraph).
2. **`wf` reads the profile file from the branch it checks** (a2), the same
   as it reads the design and as LOOP-09 has a component's checks read. The
   file is under a rule path (`docs/development/` here), so a branch that
   changes it fails the rule-change clearance unless its review approved
   the change (FLOW-14). Only the clearance is the exception: paths are
   classified for it with the profile file's `rules` keys (`paths`,
   `machinery`, `rule_jobs`) as they are at the branch's merge base with its
   trunk (`git show <merge-base>:<profile path>` in each repository, the
   merge base taken against that repository's trunk), never as the branch
   has them. The profile file's own path is always a rule-change path,
   whatever `rules.paths` says. An absent `rules` key at the branch does not
   narrow the classification. This matches how FLOW-14 already runs the
   merge base's tier 1 and `wf check` at Review. The fast gate and the other
   keys (reference documents, designs) are still read from the branch. The
   trunk is never read from a profile file: `wf` takes it from the
   repository's `config.yaml` entry (Decision 3), so a command with no
   branch to read (`wf main`, `wf stale`, `wf areas -check`, the native
   reset) knows it, and a branch cannot choose its own merge base; a
   profile file whose `trunk` differs from the entry's fails every check
   that reads it, naming both. *Rejected:* reading it from trunk. It would be
   safer on paper, but a branch can already change `make lint` by editing
   the `Makefile`, and a change that adds a gate would be checked with the
   old one.
3. **The queue state names every repository an item may name.**
   `config.yaml` gains `repositories`, one entry per repository: its name,
   its trunk, and, once it has adopted, the path of its profile file. An
   entry with no `trunk` fails `wf validate`; one with no `profile` is a
   repository that has not adopted. A repository's entry gains its
   `profile` only after the pull request that adopts it (LOOP-08's steps,
   an item of its own, reviewed as such) has merged. Until then the
   repository has not adopted for `wf`, so the adopting pull request is
   checked as any branch there is (Decision 5): other work in that
   repository is not blocked while it is open, and its own profile and
   rule edits are not cleared by the rule-change clearance, which Jeff
   accepted (2026-11-17); its review is what approves them.
   `repository` stays and still names the queue's own. An item
   that names a repository with no entry fails `wf validate`. Entries are
   added in an attended session on Jeff's word, as the area map is
   (FLOW-05). The entries for every repository named by an item already in
   the queue (here `buildwarden/buildwarden` and
   `buildwarden/buildwarden-specs`) are written before the merge, each
   with its `trunk` only, so no item fails at the merge for an absent
   entry and no absent entry is read leniently (GLOBAL-11), the queue's
   own repository's included. This repository is no exception to the rule
   above: this item's pull request is the change that adopts the profile
   file here, so `buildwarden/buildwarden`'s entry gains `profile:
   docs/development/loop-profile.yaml` only after that pull request has
   merged, in the same attended sitting, on Jeff's word, immediately after
   the merge and before the next run, cloud or native (Implementation plan
   task 14b). Until then the new `wf` reads this repository as not
   adopted: it would compute every item homed here as tracked (a6), so
   `wf validate` fails each one whose round is `feature` ("a tracked
   item's round is tracked", `queue.go:863-864`); it runs no fast gate or
   reference-document check here; no path here is a rule path for the
   clearance; none of the front-matter keys this repository's profile
   adds is required; and no `native.prepare` is read. That is not
   acceptable for a run, so no run takes a step in that window: the
   sitting merges when no run is in progress and writes the `profile`
   before any run starts. Every branch here whose merge base with `main`
   predates the merge (every branch open at it), other than a process
   item's, which is exempt (FLOW-14), has the profile file at neither its
   merge base nor its head, so once the entry names the file that branch fails the
   rule-change clearance, naming the file, until it next merges `main`
   (Decision 5): a one-time cost, accepted by Jeff (2026-11-17).
   *Rejected:* reading the trunk from the remote's default
   branch: a value the loop did not write down, used when nothing is given
   (GLOBAL-10); and reading it from the profile file, which a command that
   checks no branch cannot reach without already knowing the trunk, and
   which a branch could point at itself (Decision 2).
4. **An item records a branch and a pull request per repository** (a3).
   `branch` and `pr` stay the home's. A new list, `work`, holds one entry
   per other repository the item names: `repo`, `branch`, `pr` (number, CI,
   head) and, after its merge, `merged`. The branch has the home's name in
   every repository (a4). An item recorded before this lands has no `work`
   and keeps today's behaviour (Defaults and overrides).
5. **The gates run in each repository the item has a branch in**, each
   through its own profile; a repository that has not adopted gets the
   branch, area, trunk and pull-request checks and no fast gate or
   reference-document checks, because it has no profile to name them
   (LOOP-08's last paragraphs), and, while its `config.yaml` entry names
   no `profile`, no path there is a rule path for the clearance, the
   adopting pull request's included (Decision 3). Where the entry names a
   `profile`, which it does only once the adoption has merged, the
   clearance reads it as Decision 2 says. The
   checks are listed under **The gates, per repository**.
6. **The loop sees every merge it records** (a5). The `human` step that
   records a merge the loop cannot see (FLOW-04, FLOW-08) is retired for a
   pull request of an item the loop runs (not tracked), in any repository
   with an entry in `config.yaml`, which is every repository an item may
   name (Decision 3); a tracked item's Merge stays its `human` step
   (FLOW-03). The human merges the item's
   pull requests in the order its design gives; a step the design adds
   after one of them (a release, a pin) needs that merge.
7. **A home that has adopted gets the feature round; one that has not
   stays tracked** (a6). The relay, once adopted, homes items with real
   phases, gates and limits. A process item is still homed in the queue's
   repository, where the loop's rules are until the move.
8. **State for a second repository is added beside the existing files**
   (a7): `repos/<owner>/<name>/areas.yaml` and `repos/<owner>/<name>/main.yaml`.
   `areas.yaml` and `main.yaml` at the root stay the queue repository's.
   Nothing moves, so nothing running breaks at the merge.
9. **Areas are named in the item's home, and prefixed elsewhere** (a8). A
   bare area name is an area of the item's home repository's map; an area
   of another repository is written `<owner>/<name>:<area>`. Every queue
   file today is homed here, so every bare name keeps its meaning.
10. **A hold still names workers** (a9). This departs from
    `loop-parallel`'s hand-off of Decision 18: a hold on `cloud` stops
    refreshes and second merges in every repository the worker serves,
    accepted because a hold exists to keep workers off the loop's tools
    while a process item changes them, and a worker uses the same `wf`,
    skills and agents in every repository it serves.
11. **This item holds no worker** (a10). Every change is additive or reads
    an absent field as today.
12. **`wf` reaches second repositories through checkouts the session
    names** (a11): `-checkouts DIR`, holding one clone per repository at
    `DIR/<owner>/<name>`. The skills clone them into `.dev/checkouts/`,
    which is ignored (`.gitignore:11`) and inside the session's working
    directory, so the agents' permissions cover it. Sessions start in this
    repository's checkout, where `wf`, the skills and the agents are, until
    the move.
13. **One tier-2 report per branch state** (a12), named by the home's head as
    today, its JSON recording the head of every repository it read.
14. **The generic checks move into `wf`** (a13): into
    `tools/workflow/check/` move `tools/tier1/rules/rules.go` (the rule
    index and the rule-to-check map parse), the Tier1-Allow trailer parsing
    of `tools/tier1/diff/diff.go` (`trailerRe`, `allowRe`, `trailer`,
    `Allow`), and the design-front-matter check of
    `tools/tier1/coupling/frontmatter.go` (`Validate`, `TouchedDesigns`,
    `AllDesigns`, FLOW-12's pre-standard rule and `design_tolerate`). They
    run as `wf rules`, reading the profile for the rule paths. Tier 1 keeps
    thin wrappers that call them through its existing `replace`,
    so this repository's CI does not change; a component's CI runs `wf
    rules` built from the loop's trunk.
15. **The native side keeps one dedicated checkout per repository a machine
    serves** (a14), cloned by the machine's owner at install (LOOP-05), each
    reset to its trunk at the start of every run and fingerprinted by the
    gate. A host name may be served by several machines (`loop-native-lead`
    a6, FLOW-04 as it approves it): a host step goes to a machine that
    serves its host name and lists its repository, and `wf validate` fails
    one only when no machine serving the name lists the repository. The
    product script the wrapper runs moves into this repository's profile
    file as native preparation for `windows-hyperv`. This part is built on
    `loop-native-lead`'s host table and wrapper (its push script and its
    machine cleanup), and adds to them: the push script takes an optional
    `<owner>/<repo>` argument for a branch in a clone, worked in a worktree
    at `.scratch/<owner>/<repo>/<name>` that the session makes with `git -C
    .dev/checkouts/<owner>/<repo> worktree add` after a `fetch` there,
    each now allowed by its own narrow allow; the cleanup
    loops over each clone; `wf gate` offers a led item's steps only to a
    machine that lists every repository the item works in; and `wf lead
    -host`, `wf new -lead` and `wf declare` refuse a lead no serving
    machine can take. FLOW-04's paragraph on taking steps is amended to
    say so.
16. **Order with the other P1 items** (a15). Both predecessors are
    designed and approved: `loop-parallel-rule-wording` (priority 62) at
    `d0b4d46`, recorded at `6d134b0`, and `loop-native-lead` (64) at
    `74deb30`, recorded at `dbd3032`; this item is 65. Every paragraph this
    design amends that either also amends is drafted on top of its approved
    text, quoted under **Rule changes**: of `loop-parallel-rule-wording`'s,
    FLOW-02's `areas` bullet, its last paragraph, the template's `areas`
    comment and FLOW-03's paragraph after the table; of
    `loop-native-lead`'s, FLOW-04's paragraph on taking steps, FLOW-16's
    first bullet with its new machine-cleanup bullet, FLOW-17's second
    paragraph, and PROFILE-01's Native hosts row. The native
    part builds on `loop-native-lead`'s host table and wrapper. This design
    is reviewed now; its Implement waits, through a `human` step
    `x2-predecessors-merged` that `x2-implement` needs, for both merges, and
    the only re-base then is to check that their merged text of those
    paragraphs equals the approved text quoted here, with a review of the
    delta only if it differs.
17. **What proves the item is not its done-when** (a16). This item is done
    when its PR merges (FLOW-03, process items). The wave-1 gate is met
    later, when the relay's first item and an item here complete from the
    queue, after the relay adopts.
18. **The loop's front-matter keys, and the profile's** (a17). `wf`
    requires the loop's eight keys (`feature`, `status`, `seams`,
    `alters_relationships`, `design_review`, `native_validation`, `rules`,
    `concepts`) and the keys and values the home's profile file adds; this
    repository's adds `drivers`, `surfaces`, `security_boundary` (GLOBAL-06's
    four values) and `tier2_gaps`, so nothing changes here. FLOW-02 is made
    neutral in this change (LOOP-08: "made neutral when the rule is next
    changed"). *Rejected:* every component's designs carrying all twelve
    keys, `none` or `[]` where the term is `none`, which LOOP-08 forbids a
    loop rule to require.
19. **A private repository's work is published as a public one's** (q1,
    answered (C) by Jeff on 2026-11-17). An item in a private repository
    writes to the public queue state what any item writes: its title,
    questions, steps, branch heads, pull requests, CI results, tier-2
    reports and digest lines, from its first item. The relay goes public at
    M1. So there is no `private` key in `config.yaml`, no `wf validate`
    rule for one, and no second store. *Rejected:* (A), the loop working
    only in public repositories, which made the wave-1 gate wait for the
    relay to be public; (B), a second, private store for reports.

## Questions

### Blocking

- **q1. A private repository's work in the public queue.** **Answered
  (C)** (Jeff, 2026-11-17): a private repository's work is published as a
  public one's from its first item; the relay goes public at M1. There is
  no `private` key, no `wf validate` rule for it and no second store
  (Decision 19). The question as asked:

  (The relay design's a17; `loop-contract`, hand-off point 5.) The
  `workflow` branch is in a public repository: `QUEUE.md`, the digests, the tier-2 reports
  (`reports/<item>/<sha12>.{md,json}`, LINT-04) and every queue file's
  `source`, history, questions and step notes are public. An item in the
  relay would put there its design's title and questions, its branch heads,
  its pull request numbers, its CI results, and a tier-2 report that quotes
  its diff. Its design itself is in the relay repository. As read on
  2026-11-14 the relay is private (`loop-contract` a21) until its M1's
  scan (`loop-contract`, hand-off point 5), and it goes public at M1
  (umbrella Decision 16). This session could
  not read it on 2026-11-17 (its GitHub access does not include it), so its
  visibility today is not confirmed here. Only Jeff can decide what of a
  private repository may be published, and the answer decides whether this
  design needs a second, private store for reports.
  - **(A)** The loop works only in public repositories. A private
    repository may be named by a tracked item, whose title intake writes
    with Jeff; no gate, report or digest line beyond that. The relay's
    first item from the queue is one taken after it goes public at M1; its
    M1a stays by hand as its a17 plans. `wf
    validate` fails an item that names a repository whose `config.yaml`
    entry says `private: true`, unless the item is tracked. Cost: the
    wave-1 gate waits for the relay to be public.
  - **(B)** A private repository's item writes metadata only to the public
    state: title, phases, step ids, branch heads, pull request numbers, CI
    result, and tier-1 results as pass or fail. Its tier-2 report and its
    digest detail are kept in the private repository (on a `workflow`
    branch of its own, read by `wf` through its checkout), and `QUEUE.md`
    links nothing there. Cost: a second store, a `wf` code path for it, and
    LOOP-06's "one place of reporting" split in two until it goes public.
  - **(C)** As for a public repository: Jeff accepts that the relay's work
    is published from its first item, since it is to be public at M1.
    Cost: whatever M1's scan would have caught in the diff is quoted in a
    public report first.

  Recommended was **(A)**: it needs no new store, publishes nothing Jeff
  has not chosen to, and costs only the order of the relay's work, which
  M1 already puts first. Jeff chose (C).

### Assumed

- **a1. Where `wf` reads a component's profile.** **Confirmed (a)** (Jeff, 2026-11-17).
  - (a) A YAML file beside the profile, which the profile names and
    `config.yaml`'s `repositories` entry points to; its format is
    documented in the `workflow` branch's `README.md` with the rest of the
    state's schema. LOOP-01 and PROFILE-01 are amended to say so.
  - (b) `wf` parses the profile's Markdown table. Its entries are prose,
    and several name the file that holds the fact (PROFILE-01's Checks
    entry is "The table below").
  - (c) The entries are written into `config.yaml` per repository. A
    component's gate would then change in the queue state, outside its
    repository and its review.

  Recommended: (a).
- **a2. The branch's profile file or trunk's.** **Confirmed (a)** (Jeff, 2026-11-17).
  - (a) The branch's, as LOOP-09 has a component's checks read, guarded
    by the rule-change clearance: the clearance classifies paths with the
    `rules` keys of the merge base's profile file, and the profile file's
    own path is always a rule-change path. A branch that changes the file
    unapproved, or narrows `rules` to free its own edits, therefore fails
    Implement, Review and Publish (FLOW-14). This is the same reason
    FLOW-14 runs the merge base's tier 1 and `wf check`.
  - (b) Trunk's. A gate the change itself adds is then first run after the
    merge.

  Recommended: (a). The `Makefile` is read from the branch today.
- **a3. How the queue records a second repository's branch and pull
  request.** **Confirmed (a)** (Jeff, 2026-11-17).
  - (a) `branch` and `pr` stay the home's; `work` lists the others, one
    entry each: `{repo, branch, pr: {number, ci, head}, merged}`. No
    existing file changes.
  - (b) `work` lists every repository, the home included, and `branch`
    and `pr` are retired by `wf migrate`. One shape for all, and every
    queue file, `wf-publish` and the skills change at once.
  - (c) `branch` is one name for every repository, and only the pull
    requests are a list.

  Recommended: (a). (b) is the cleaner shape and could be done by the
  move, which rewrites the state anyway.
- **a4. A second repository's branch name.** **Confirmed (a)** (Jeff, 2026-11-17).
  - (a) The home's branch name, in every repository (FLOW-17 as amended
    below). The profile's branch pattern is read from the home's profile.
  - (b) Each repository's own pattern from its profile.

  Recommended: (a). One name is what a reader searches for, and no profile
  so far has another pattern.
- **a5. The order of an item's merges.** **Confirmed (a)** (Jeff, 2026-11-17).
  - (a) Each pull request is listed ready on its own (FLOW-08); the Merge
    phase is done when the loop has seen every one merge. A design whose
    home pull request needs another's merge first (a release to pin, as
    `seam-specs` did) says so, and the step it adds for the release
    `needs` that merge: for each `work` entry the design marks `first`,
    `wf` seeds a confirmation step `f6-merged-<n>` in Publish, `<n>` being
    the entry's position in `work`, counted from 1. The other entries get
    no step of their own: the existing `f7-merge` step proves them, and
    passes only once every pull request has merged. Each is done by the
    tick's confirmation and never offered (FLOW-05).
  - (b) `wf` enforces the home last: the home's pull request is opened
    only after every other has merged. Simple, and wrong for an item whose
    other pull requests need nothing first.
  - (c) As `seam-specs`: a `human` step records each merge in a second
    repository. The retired clause stays in FLOW-04 and FLOW-08.

  Recommended: (a).
- **a6. Items homed in a second repository.** **Confirmed (a)** (Jeff, 2026-11-17).
  - (a) An adopted home (a `profile` in its `config.yaml` entry) gets the
    feature round, its gates and FLOW-05's limits; a home that has not
    adopted stays tracked, as `loop-parallel` built it. A process item is
    homed in the queue's repository. `validateKind` (`queue.go:850-858`)
    refuses one whose `repos[0]` is not `config.yaml`'s `repository`. It
    now checks this directly rather than through `tracked`, because
    `tracked` now means "the home has no profile", and an adopted second
    repository is not tracked. `State.Problems` (`queue.go:683`), where
    `tracked` is computed (`:689`) and passed to `f.validate` (`:697`),
    computes a separate `homedHere := len(f.Repos) == 0 || f.Repos[0] ==
    s.Config.Repository`, threaded through `Feature.validate`
    (`:827-831`) into `validateKind`, and the process case becomes
    `f.IsProcess() && !homedHere`. Its message keeps "a process item is
    homed in the queue's repository, not %s" and drops the parenthetical
    "(an item homed in another repository is tracked)", which an adopted
    home makes false. The
    `tracked` argument still drives only the feature-round checks. When
    `config.repository` is unset (`:690-696`), the current fallback stays.
  - (b) The tracked round is retired: an item is homed only in an adopted
    repository, and work in one that has not adopted is a second
    repository of an item homed elsewhere.

  Recommended: (a). (b) would make the specs repository unable to home a
  change of its own until it adopts, which Jeff has not filed.
- **a7. Where a second repository's state lives.** **Confirmed (a)** (Jeff, 2026-11-17).
  - (a) `repos/<owner>/<name>/areas.yaml` and `main.yaml`, added beside the
    root's `areas.yaml` and `main.yaml`, which stay the queue
    repository's.
  - (b) Every repository's under `repos/`, the root's two files moved by
    `wf migrate`. One layout, and a run that started before the merge with
    the old `wf` then writes a root `main.yaml` nobody reads (LOOP-09: a
    run keeps its tools until it ends).

  Recommended: (a); the move may make (b) when it rewrites the state.
- **a8. Which repository a bare area name means.** **Confirmed (a)** (Jeff, 2026-11-17).
  - (a) The item's home's. Areas of another repository are
    `<owner>/<name>:<area>`.
  - (b) The queue's repository's, whatever the home.

  Recommended: (a). A relay item's design then names its own areas bare,
  and every queue file today is homed here, so nothing changes meaning.
- **a9. A hold that names repositories** (`loop-parallel` Decision 18 left
  it here: "No step of a second repository is taken by a worker a hold
  could stop until `loop-repository`, which adds it with the first such
  step", `loop-parallel.md:458-460`). **Confirmed (a)** (Jeff, 2026-11-17).
  - (a) Not added. A hold names workers and covers work in every
    repository (FLOW-05, `process.md:225-230`, unchanged).
  - (b) `holds` entries may be `<affinity>@<owner>/<name>`, holding that
    worker's steps in that repository only, so a process item holds the
    worker only in the repository it changes.

  Recommended: (a). It departs from `loop-parallel`'s hand-off, which
  expected the qualifier with the first step of a second repository. The
  reason: a hold exists to keep workers off the loop's tools while a
  process item changes them, and a worker uses the same `wf`, skills and
  agents in every repository it serves, so a hold scoped to one repository
  would let the worker run on the tools being changed elsewhere. Its cost:
  a process item that holds `cloud` also stops refreshes, and with them
  second merges, in every repository named in its `work` (and in every
  repository the `cloud` worker serves) for the whole hold, which can be
  days.
- **a10. This item's holds.** The queue file's `source` says "Holds cloud
  and host:* while in progress"; its `holds` were removed on 2026-11-14
  and the decision left to this design. **Confirmed (a)** (Jeff, 2026-11-17).
  - (a) None. Every field is additive and reads as today when absent
    (Decisions 4 and 8): `work` absent is no second branch; the root's
    `areas.yaml` and `main.yaml` stay. The new `repositories` entries
    (`buildwarden/buildwarden` and `buildwarden/buildwarden-specs`) are
    written in `config.yaml`, with their `trunk` only, in the attended
    sitting that merges the PR, before the merge (Decision 3; the old `wf`
    reads `config.yaml` without strict fields, `queue.go:594`, and ignores
    them), and `buildwarden/buildwarden`'s `profile` is added in the same
    sitting immediately after the merge and before the next run, as for
    any adoption, so items already declared, among them the in-progress
    `relay-collector-failure-fails-open` whose `repos` names
    `buildwarden/buildwarden-specs`, meet no absent entry at the merge and
    validate unchanged once the profile is added (task 14b), before the
    next run. The sitting merges when no run is in progress (Decision 3),
    so no run meets the window between the merge and task 14b.
  - (b) `holds: [cloud, host:*]` for the whole time the item is in
    progress, from its claim: days of S1-, K1- and fix work stopped for
    nothing that breaks.

  Recommended: (a).
- **a11. How `wf` finds a second repository's checkout.** **Confirmed (a)** (Jeff,
  2026-11-17).
  - (a) `-checkouts DIR`, a directory of clones at `DIR/<owner>/<name>`,
    passed by the skills as `.dev/checkouts`. A check that needs a
    repository with no clone there fails and names the flag and the path
    (GLOBAL-10). `-repo` stays the queue repository's checkout.
  - (b) A default of `<-repo>/.dev/checkouts` when the flag is absent.
    Obvious, but a second name for the same thing, and the skills pass it
    anyway.
  - (c) `-checkout owner/name=DIR`, repeated. Exact, and a long line in
    every skill.

  Recommended: (a).
- **a12. The tier-2 report for an item with several branches.** **Confirmed (a)**
  (Jeff, 2026-11-17).
  - (a) One report per state of the item's branches, at
    `reports/<item>/<home sha12>.{md,json}` as today; the JSON gains
    `heads`, the head of each repository the report read. The Review
    check passes when the report for the home's head exists and its
    `heads` equal the item's current heads.
  - (b) One report per repository's head. Two reports for one review of
    one change.

  Recommended: (a).
- **a13. Where tier 1's generic checks live.** **Confirmed (b)** (Jeff, 2026-11-17).
  - (a) They stay in `tools/tier1/`; a component's CI builds this
    repository's `tier1` and runs only those three. It carries the Go
    analyzers and golangci-lint with it.
  - (b) The three checks' code moves into `tools/workflow/check/`:
    `tools/tier1/rules/rules.go` (the rule index and the rule-to-check map
    parse); the Tier1-Allow trailer parsing of `tools/tier1/diff/diff.go`
    (`trailerRe`, `allowRe`, `trailer`, `Allow`); and the
    design-front-matter check of `tools/tier1/coupling/frontmatter.go`
    (`Validate`, `TouchedDesigns`, `AllDesigns`, FLOW-12's pre-standard
    rule and `design_tolerate`). Tier 1 keeps thin wrappers that call these
    through its `replace`. `wf rules [-base REV]` runs the three on a
    checkout, with the rule paths,
    prefixes and template from its profile file. Tier 1 calls the same
    functions through its `replace`, so its report and this repository's
    CI are unchanged. A component's CI runs `go run
    github.com/buildwarden/buildwarden/tools/workflow/cmd/wf@main rules`,
    which follows the contract's trunk as LOOP-08's link does; the move
    changes that path once.
  - (c) Each component writes its own. Three implementations of one rule.
  - (d) Left to the move.

  Recommended: (b).
- **a14. The native side.** **Confirmed (a)** (Jeff, 2026-11-17), with
  the host-table check as restated below for a host name several machines
  serve: it fails only when no machine serving the host name lists the
  repository, naming them all.
  - (a) `hosts.yaml` gains, per machine, `repositories`: those whose host
    steps it takes (default: the queue's own only, which is today). The
    owner clones each one once, inside the dedicated checkout, at
    `.dev/checkouts/<owner>/<name>` (ignored), and installs its toolchain; the install
    scripts check each clone's `origin` and cleanliness and run the
    profile file's `toolchain_check` command, and never clone or install
    (LOOP-05: set up by the owner). Each run resets every clone to its
    trunk, the gate's fingerprint covers each repository's trunk and
    in-progress branches, and `wf gate` offers a host step only on a
    machine that serves its host name and lists its repository. A host
    name may be served by several machines (`loop-native-lead` a6), so `wf
    validate` fails a host step only when no machine serving its host name
    lists its repository, and the error names every machine that serves
    the name, the host name and the repository, for the owner to add to one
    of them. A led item (`loop-native-lead`) is checked the same way for
    its lead: its led steps run the gates in each repository the item works
    in, so it fails when no machine serving the lead lists all of them,
    and `wf gate` offers its led steps only to a machine that serves the
    lead and lists every repository in the item's `repos`. `wf lead
    -host`, `wf new -lead` and `wf declare` refuse to create a lead that
    no such machine exists for, so no worker commits a state `wf validate`
    fails.
    `refresh-cache.sh` moves from `native-tick.sh` into this repository's
    profile file, as `native: prepare:` for `windows-hyperv`, which the
    wrapper runs from the repository's checkout. A failed prepare marks
    only its hosts stale for the run, as today. The machine's push script
    (`loop-native-lead`'s `native-push.sh`) gains an optional repository
    argument for a branch in a clone, whose worktree is
    `.scratch/<owner>/<repo>/<name>` in the dedicated checkout (`<name>`
    being the push script's worktree-name argument), made by the session
    with `git -C .dev/checkouts/<owner>/<repo> worktree add`, which,
    with the `fetch` before it, `settings.native.json` gains two narrow
    allows for (`Bash(git -C .dev/checkouts/* fetch*)` and `Bash(git -C
    .dev/checkouts/* worktree*)`); and the wrapper's worktree
    cleanup loops over each clone (**The native side**).
  - (b) Host steps in a second repository wait for the move.
  - (c) The wrapper clones a second repository on demand when a host step
    there is ready. An agent then sets up a machine (LOOP-05).

  Recommended: (a). The host-table check and the wrapper's part are
  written against `loop-native-lead`'s approved host table (several
  machines per host name, its a6) and wrapper; the line numbers cited are
  `native-tick.sh` at `0cd3c02`, and both are re-derived on its merged
  wrapper and host table at Implement (a15).
- **a15. Order with `loop-parallel-rule-wording` and `loop-native-lead`.**
  Both are process items queued ahead of this one
  (`loop-parallel-rule-wording` 62, `loop-native-lead` 64, this one 65, on
  the `workflow` branch on 2026-11-17). Both are designed and their reviews
  approved on 2026-11-17, and neither is implemented:
  `loop-parallel-rule-wording` approved at `d0b4d46` (recorded at
  `6d134b0`), `loop-native-lead` approved at `74deb30` (recorded at
  `dbd3032`).
  - `loop-parallel-rule-wording` amends FLOW-02 (the `areas` bullet, the
    last paragraph, the template's `areas` comment), FLOW-03 (the paragraph
    after the table), FLOW-05 (**How two items relate**, and
    `process.md`'s opening paragraph) and FLOW-16 (the third paragraph,
    the merge proof). This design also amends the first four: FLOW-02's
    `areas` bullet, its last paragraph, the template's `areas` comment and
    FLOW-03's paragraph after the table (its "From Plan on ... from
    Implement on" sentence). It amends neither FLOW-05 nor FLOW-16's third
    paragraph.
  - `loop-native-lead` amends FLOW-04 (the `host:<name>` bullet and the
    paragraph after the list), FLOW-05 (the blocking clause), FLOW-09,
    FLOW-13, FLOW-16 (its first bullet, unchanged in text, and a new
    machine-cleanup bullet after it), FLOW-17 (the second paragraph),
    FLOW-18 and PROFILE-01 (the Native hosts row). This design also amends
    FLOW-04's paragraph on taking steps (a machine takes a host step only
    in a repository it lists, a14), FLOW-16's first bullet, where the new
    bullet follows, FLOW-17's second paragraph (`main` read as each
    repository's trunk) and PROFILE-01's Native hosts row. Its FLOW-04
    entry also amends the `human` bullet, and its FLOW-17 entry the first
    paragraph, neither of which `loop-native-lead` changes. The native
    part (a14) builds on `loop-native-lead`'s host table, in which any
    host name may be served by several machines (its a6), and on its
    wrapper.

  Each overlapping entry under **Rule changes** quotes the current wording
  at `0cd3c02`, the predecessor's approved text, and a proposed wording
  drafted on top of the approved text, so the proposed text is the final
  combined one. **Confirmed (a)** (Jeff, 2026-11-17).
  - (a) This design is reviewed now. When the design review is recorded,
    the attended session seeds a `human` step `x2-predecessors-merged`
    (phase 2, affinity `human`), and `x2-implement` lists it in `needs:
    [x2-predecessors-merged]`; this needs no change to `wf`. An attended
    session (`/workflow-human`) marks the step done once `wf` shows both
    `loop-parallel-rule-wording` and `loop-native-lead` done with their PRs
    merged. The only re-base then is to check that each one's merged text
    of the overlapping paragraphs listed above equals the approved text
    quoted here; where it does, nothing changes, and where it differs the
    design comes back for a review of that delta only (FLOW-19). The
    native part is implemented on `loop-native-lead`'s merged wrapper and
    host table. While the step is pending the item counts as waiting on
    the human (FLOW-05), which is acceptable: a process item starts
    whatever the count.
  - (b) The review waits for both to merge.

  Recommended: (a). The questions here do not depend on theirs.
- **a16. What shows the queue works across repositories.** **Confirmed (a)** (Jeff,
  2026-11-17).
  - (a) This item is done at its merge, like any process item (FLOW-03).
    The first live use is an item that names `buildwarden/buildwarden-specs`
    and is declared after the merge (such as
    `relay-collector-failure-fails-open` if it is re-declared, or the next
    one intake files). The wave-1 gate (`loop-contract` a22) is met when a
    relay item and an item here have each completed from the queue, which
    needs the relay's adoption (its M1b task 10), its `config.yaml` entry
    with the `profile` added once that adoption has merged (Decision 3),
    and the workers' access to it; under q1 (C) it does not wait for the
    relay to be public. Jeff judges the gate; no
    `wf` check does.
  - (b) This item's Close waits for a relay item to complete.

  Recommended: (a). (b) would keep a process item in progress for weeks,
  and a process item in progress ranks ahead of every new feature
  (FLOW-05).
- **a17. Which front-matter keys are the loop's and which a profile's.**
  `Front.Problems` requires twelve keys (`check.go:671-673`). LOOP-08 says
  no loop rule may require that a component has drivers, user-facing
  surfaces or security boundaries. **Confirmed (a)** (Jeff, 2026-11-17).
  - (a) `wf` requires the loop's keys (`feature`, `status`, `seams`,
    `alters_relationships`, `design_review`, `native_validation`, `rules`,
    `concepts`) and the keys the profile file adds, with the values it
    gives. This repository's file adds `drivers`, `surfaces`,
    `security_boundary` (GLOBAL-06's four values) and `tier2_gaps`
    (TEST-03), so nothing changes here. This item amends FLOW-02, so its
    text is made neutral in the same change (LOOP-08: "made neutral when
    the rule is next changed"): the `drivers`, `surfaces`,
    `security_boundary` and `tier2_gaps` bullets are marked as keys this
    repository's profile file adds, `alters_relationships` and the
    conservative-derivation paragraph name this repository's components,
    packages, drivers and hosts as its examples, and the last paragraph
    (what the automated part diffs and checks) names `drivers`,
    `surfaces` and the security note as this repository's (**Rule
    changes**, FLOW-02).
  - (b) Every component's designs carry all twelve, `none` or `[]` where
    the term is `none`.

  Recommended: (a).

## Design

### The profile file

`docs/development/loop-profile.yaml`, this repository's, with today's values
(each line is where the value is fixed today, Problem point 3):

```yaml
# The entries of PROFILE-01 that the loop's tools read (LOOP-01). Schema:
# the workflow branch's README.md. A change here is a change to PROFILE-01.
version: 1
component: buildwarden
trunk: main                   # must equal config.yaml's entry, which is what wf reads (Decision 2)
branch: feat/{item}
designs:
  dir: docs/design
  template: docs/design/TEMPLATE.md
# Run in the checkout on the host platform, each must exit 0 (FLOW-03, FLOW-17).
# wf runs these on the host platform; the per-platform GOOS=<os> make lint of
# PROFILE-01 stays the agent's (FLOW-17) and is not in the file.
fast_gate:
  - [make, lint]
  - [make, test]
impact:                       # keys this component adds to the loop's, and their values
  keys: [drivers, surfaces, security_boundary, tier2_gaps]
  values:
    security_boundary: [none, host-env, env-relay, env-beyond]
reference_docs:
  implement: [docs/]          # Implement: changed when the design declares surfaces (UX-01)
  guides: [docs/book/src/guide/, docs/book/src/getting-started/, examples/, README.md]
rules:
  dir: docs/development       # LINT-01's files; the prefixes are its README's ID table
  paths: [docs/development/, tools/tier1/, tools/workflow/check/, docs/design/TEMPLATE.md]
  machinery: [.claude/, tools/workflow/]
  rule_jobs: 'tier1|golangci-lint|tools/workflow|\bmake\s+(lint|enforcement)\b'
  index: tools/tier1/rules.yaml  # LINT-01's rule-to-check map and design_tolerate
areas:
  edges: [tools/tier1/area-edges.sh]
toolchain_check: [go, version]
native:
  prepare:
    windows-hyperv: [[tools/relay-vm/hyperv/refresh-cache.sh]]
  check:                        # run by the install script (LOOP-05)
    windows-hyperv: [[tools/relay-vm/hyperv/refresh-cache.sh, --check]]
```

`rules.index` absent is `none`: `rule-index` then checks only the rule IDs,
with no rule-to-check map and no `design_tolerate`.

A key absent from the file is a term the component fills as `none`
(LOOP-01): no fast gate, no reference documents, no machinery. `wf` reads a
key it requires and finds absent as `none`, never as this repository's
value. A file `wf` cannot parse, or a `version` it does not know, fails
every check that reads it, naming the file.

The relay writes its own at adoption, under its own path, as part of
LOOP-08's step 2 (amended below). Its values are its own: a `cargo` gate,
no `drivers`, its own `security_boundary` set if it keeps one. `wf` reads
it once the adopting pull request has merged and the relay's
`config.yaml` entry has gained its `profile` (Decision 3).

### `config.yaml`

```yaml
repository: buildwarden/buildwarden      # the queue's own, unchanged
repositories:
  - name: buildwarden/buildwarden
    trunk: main                          # every entry gives its trunk; wf reads it here only
    profile: docs/development/loop-profile.yaml   # added right after this item's PR merges (task 14b)
  - name: buildwarden/buildwarden-specs  # not adopted: no profile
    trunk: main
  # - name: buildwarden/buildwarden-relay
  #   trunk: main
  #   profile: <its path>                # added once its adoption PR has merged
```

`wf validate` fails an item that names a repository with no entry, and an
entry with no `trunk`. `wf` reads a repository's trunk from its entry
only, never from a profile file; a profile file whose `trunk` differs
from the entry's fails every check that reads that file, naming both
values (Decision 2). An entry gains its `profile` only after the pull
request that adopts the repository has merged (Decision 3). An entry
records no
visibility: a private repository's work is published as a public one's
(q1 (C), Decision 19). The two entries above are
written in the attended sitting that merges this item's PR, before the
merge, on Jeff's word, with their `trunk` only (Decision 3, a10, task
14a), so no item already in the queue meets an absent entry at the
merge. `buildwarden/buildwarden`'s `profile` is added in the same
sitting immediately after the merge, before the next run, as for any
adoption (task 14b); until it is, the new `wf` reads this repository as
not adopted, which no run may meet (Decision 3). An entry absent for the queue's own
repository fails as any other absent entry does; it is not read as a
default profile path (Decision 3, GLOBAL-11).

### The queue file

```yaml
repos: [buildwarden/buildwarden, buildwarden/buildwarden-specs]
branch: feat/relay-collector-failure-fails-open    # the home's, as today
pr: {number: 131, ci: green, head: 1a2b3c4}        # the home's, as today
work:
  - repo: buildwarden/buildwarden-specs
    branch: feat/relay-collector-failure-fails-open
    first: true                                    # a5 (a): merges in Publish
    pr: {number: 7, ci: green, head: 9f8e7d6}
    merged: 4c3b2a1                                # the merge the loop confirmed
```

`wf declare` writes `work` from the design's `repos`: one entry per
repository after the home, with the home's branch name, and `first` from
the design's front-matter key `merge_first` (a list of repositories,
added to FLOW-02 below). It
refuses to drop an entry whose branch exists. `wf validate` checks that
`work` and `repos` agree once declared. An item declared before this item
merges has no `work`; it keeps today's behaviour until it is declared again
(Defaults and overrides).

### The gates, per repository

`check.Env` gains `Checkouts`, the directory of `-checkouts`, and the
checker runs each per-repository check against the clone of the repository
it concerns, with that repository's trunk (`origin/<trunk>`, the trunk
its `config.yaml` entry gives) and profile
file read from the branch there, except the clearance's `rules` keys, read
at the merge base (a2). Per phase (FLOW-03), at `0cd3c02`'s
line numbers:

| Phase | Today (`check.go`) | With `work` |
|---|---|---|
| 0 Branch setup | `branchOnRemote` (`:245-248`) | in each repository |
| 1 Design | `design` (`:570-606`): front-matter, sections | home only; keys and values from the home's profile file (a17) |
| 2 Plan | `plan`, `reviewed`, `approvalDelta`, `declaredStill` | home only; `impactDelta` also compares `work`'s repositories |
| 3 Implement | `onBranchHead`, `make lint`, `make test`, `docsTouched("docs/")`, `ruleClearance`, `declaredStill`, `insideAreas` | each repository: clean branch head, its fast gate, its reference documents, its rule paths cleared against the home design's `rule_changes`, its areas from its map. No fast gate or reference documents where it has no profile, and no path there is a rule path while its entry names no `profile` |
| 4 Review | as 3, plus `tier2Report`, `trialMerge` | as 3 in each; one report whose `heads` cover every branch (a12); a trial merge onto each trunk |
| 5 Guide | `docsTouched(guides)` | each adopted repository's guides |
| 6 Publish | `mainMerged`, consolidation names `origin/main`, `ruleClearance`, `archivedOn`, `prGreen` | trunk merged and named in **Consolidation** for each repository; archived in the home; a pull request green on its pushed head in each; `f6-merged-<n>` confirmed for each `first` entry |
| 7 Merge | `merged` (`:348-430`) | each pull request merged, by the same proofs, against its own trunk |
| 8 Close | `closed` | branches gone in each repository (FLOW-16) |

A repository whose branch has no commit beyond its trunk at Publish gets no
pull request; its entry is closed with the home's. `wf stale` seeds a
refresh when any of the item's branches does not contain its trunk, and the
refresh (FLOW-10, amended below to one consolidation per trunk) merges
each. `wf main -repository <owner>/<name>` records
a trunk in `repos/<owner>/<name>/main.yaml`; without the flag it records
the queue repository's in `main.yaml`, as today. `wf areas -check
-repository ...` compares that repository's map with the edges its
profile's command prints.

`kindOf` (`rulechange.go:37-46`) and `workflowRunsRe` (`workflows.go:17-18`)
read `rules.paths`, `rules.machinery` and `rules.rule_jobs` from the
profile file of the repository whose path they classify, as that file is
at the merge base of the branch they check with that repository's trunk
(`git show <merge-base>:<profile path>`), never from the branch. The
profile file's own path, as `config.yaml`'s `repositories` entry names it,
is always a rule path in `kindOf`, whatever any copy of it says, and so is
its path on the branch if the branch moves it: a change to the file always
needs an approved `rule_changes` entry, or a process item.

Which rule applies depends on the repository's `config.yaml` entry:

- **The entry names no `profile`.** The repository has not adopted
  (Decision 3), and it has no rule or machinery paths: the loop's rules
  and machinery are not kept there, so the clearance classifies no path
  there as a rule path (LOOP-01: a term it does not have is `none`;
  Defaults and overrides). This holds for a branch that adds a profile
  file there too, the adopting pull request among them: `wf` reads no
  profile file of a repository until its entry names one. So the
  adopting pull request blocks no other branch in that repository while
  it is open, and its own profile and rule edits are not cleared by the
  clearance; its own review approves them (Decision 3, accepted by
  Jeff).
- **The entry names a `profile`.** It gains one, on Jeff's word, only
  after the adopting pull request has merged (Decision 3). The clearance
  reads the file at that path at the merge base, as above. A branch that
  forked before the adoption merged has the file at neither its merge
  base nor its head, and fails the clearance, naming the file and that
  the branch must merge its trunk, until it does. Where the merge base has
  no file there and the branch has one (a branch that adds the file other
  than through the merged adoption), every path the branch adds or changes
  under the new file's rule and machinery paths is classified as a rule
  path (Defaults and overrides). A file the merge base has but cannot be
  parsed fails the clearance, naming the file.

A process item's branch stays exempt from the clearance (FLOW-14), and a
process item is homed here (a6), so a change to the loop's machinery is
still a process item's.

### Branches, pull requests and the skills

- `wf-publish` opens a pull request in each repository with commits
  (`gh pr create -R <owner>/<name>`), records it in the `work` entry and
  repairs its CI as today (FLOW-11). It lists in each pull request's body
  the tier-2 MUST findings in that repository and the item's other pull
  requests (FLOW-08).
- The tick's merge confirmation (`workflow-tick` step 2) runs `wf done` on
  `f6-merged-<n>` and `f7-merge`; `wf` proves each merge in its clone.
  Close (`workflow-tick` step 2, item 4: the inline Close) deletes the local branch
  in each clone and escalates a remote branch that did not delete, per
  repository (FLOW-16).
- `workflow-tick` step 1 and `workflow-native` clone or fetch each
  repository named by an in-progress item into `.dev/checkouts/` (cloud),
  or check the owner's clones (native), and pass `-checkouts .dev/checkouts`
  to every `wf` command that takes `-repo`. A clone that cannot be fetched
  (no access) stops that item's steps with a `blocked` line naming the
  repository; the run goes on with other items.
- `workflow-tick` step 2 reads each trunk: for this repository and for each
  repository named by an in-progress item's `repos`, it reads that
  trunk head's check runs and its ruleset with `gh api
  repos/<owner>/<name>/commits/<sha>/check-runs` and `gh api
  repos/<owner>/<name>/rules/branches/<trunk>`, records them with `wf main
  -repository <owner>/<name>` (no flag for this repository), and pipes the
  command named by the profile's `areas.edges`, run in that repository's
  clone, into `wf areas -check -repository <owner>/<name> -checkouts
  .dev/checkouts`. The re-record after a `main.yaml` conflict does the same
  per repository. A red trunk files its follow-up in that repository
  (FLOW-09).
- `wf-implement`, `wf-review` and `wf-publish` run the fast gate the
  profile file names, in the clone the step concerns, and push there
  (FLOW-17), and, where the profile's Fast gate entry says so, its
  per-platform lint (here `GOOS=<os> make lint` for each platform whose
  build-tagged files the change touches). Each cites the profile in place of `make lint` and `make
  test`.
- The run's digest section gives the branch head left in each repository
  (LOOP-06, point 1). The tick already writes "the branch head it left";
  the line gains the repository when it is not this one.
- `workflow-intake` asks for the repositories as today (`loop-parallel`);
  it refuses a repository with no `config.yaml` entry and says an attended
  session adds it.

### The native side

As a14 (a). `hosts.yaml`:

```yaml
machines:
  - name: windows-desktop
    hosts: [windows-hyperv, container-any, linux-kvm]
    repositories: [buildwarden/buildwarden]   # absent: the queue's own, which is today
```

`wf gate` offers a host step only to a machine that serves its host name
and lists the step's repository (the item's home, or the `work` entry the
step's `repo` names; a host step gains an optional `repo`). FLOW-04, as
`loop-native-lead` approves it, lets `hosts.yaml` list several machines
for one host name (its a6), and a step goes to whichever serving machine
next takes steps; here, to whichever of those also lists the repository.
`wf validate` fails a host step whose repository (its `repo`, else the
item's home) no machine serving its host name lists (an absent
`repositories` lists only the queue's own), naming every machine that
serves the host name, the host name and the repository, to be added to
the `repositories` of one of them (GLOBAL-11). It checks a led item's lead
the same way against every repository the item works in, since a led step
runs the gates in each: it fails when no machine serving the lead lists
them all, naming those machines and the repositories missing from each.
`wf gate` likewise offers a led item's steps only to a machine that
serves the lead and lists every repository in the item's `repos` (the
queue's own when `repos` is empty): with `container-any` served by
machines A and B and only A listing the relay, a led step of an item in
this repository and the relay is offered on A and never on B.
The commands that set a lead or the repositories never create that state.
`wf lead -host <name>` and `wf new -lead host:<name>` refuse a lead no
machine serving it can take for every repository of the item's `repos`
(the queue's own when `repos` is empty), with the same message. `wf
declare` refuses, with the same message, a `work` that would leave a led
item's lead with no serving machine that lists every repository; its Plan
step then fails, and the message says that Jeff adds the repository to a
machine's `repositories` or that a worker clears the lead with `wf lead
-clear`. `wf declare` never clears a lead itself: a lead says where the
work must run, and dropping it would send that work to `cloud`.
The fingerprint
(`queue/hosts.go:257-279`) hashes, for each listed repository, its trunk and
the in-progress items' branches there. `native-tick.sh` resets each clone
to its trunk (the one its `config.yaml` entry gives), as it does the dedicated checkout at `:190`, and leaves each
on its trunk at the end (`:264-265`). It runs the `native.prepare` command
for each host name the machine serves from the repository whose profile
names it, in that repository's clone. A `native.prepare` command that
exits non-zero marks stale, for this run, the host names it is listed
under. The session still runs, told to skip those hosts' steps and gaps,
and the digest says why, as `native-tick.sh:223-240` does today. The
machine's other hosts are not blocked. The install scripts gain the check
of each clone (its `origin` is the listed repository; it is clean), run
the profile's `toolchain_check` and the `native.check` command of each
host name the machine serves (`install-windows.ps1:65`'s `refresh-cache.sh
--check` today), and report a missing clone with the
`git clone` command for the owner to run.

`loop-native-lead`'s push script and worktree cleanup are written for the
dedicated checkout; each gains a form for a clone:

- **The push script.** `tools/workflow/native/native-push.sh <name>
  [<owner>/<repo>]`. Without the second argument it is unchanged. With it,
  still run from the dedicated checkout's root, it also refuses, one line
  on stderr and exit 2, before any push: a repository argument that is not
  `<owner>/<repo>` of `[A-Za-z0-9_.-]` characters, or is the queue's own
  repository (the one-argument form serves that); a third argument;
  `.dev/checkouts/<owner>/<repo>` that is not a clone whose `origin` URL
  names `<owner>/<repo>`; `.scratch/<owner>/<repo>/<name>` that is not a
  worktree's top level or not a worktree of that clone (its `git
  rev-parse --git-common-dir` is not the clone's); and an item whose
  `.dev/workflow/queue/<item>.yaml` does not name `<owner>/<repo>` in
  `repos`, besides the approved checks (branch pattern, `status:
  in_progress`). Then it execs `git -C .scratch/<owner>/<repo>/<name>
  push --set-upstream origin refs/heads/feat/<item>:refs/heads/feat/<item>`,
  which pushes to that clone's `origin`. The worktrees live in the
  dedicated checkout's ignored `.scratch/`, not in the clone, so the clone
  stays clean for the install check and the reset.
- **Making the worktree.** The session makes it itself, as it does a
  worktree of the dedicated checkout under `loop-native-lead`: `git -C
  .dev/checkouts/<owner>/<repo> fetch origin`, then `git -C
  .dev/checkouts/<owner>/<repo> worktree add
  ../../../../.scratch/<owner>/<repo>/<name>` on `feat/<item>` (the path
  is relative to the clone). Today `settings.native.json` allows `git
  worktree` only in the session's working directory, the dedicated
  checkout (`Bash(git worktree:*)`, line 24), and `git -C` only in
  `.dev/workflow` and `.scratch/` (lines 26 and 28), so it gains two
  narrow allows, `Bash(git -C .dev/checkouts/* fetch*)` and `Bash(git -C
  .dev/checkouts/* worktree*)`, and no broader one: any other git command
  in a clone (a `commit`, `checkout`, `reset` or `config` there) is not
  allowed, and the commit is made in the worktree under `.scratch/`,
  which line 28 already allows. The denies stay as
  approved: `Bash(git -C * push*)` (`loop-native-lead`'s a16) and the
  `git -C * push origin main*`, `--force`, `-f`, `--delete` and
  `--mirror` denies still refuse every push from a clone, so the push
  script stays the only push route. A clone the session leaves on
  another branch or dirty is reset by the wrapper at its next run, as the
  dedicated checkout is.
- **The phase agents' sentence** (`loop-native-lead`'s, in `wf-publish`,
  `wf-design`, `wf-plan`, `wf-implement`, `wf-review` and `wf-guide`)
  becomes: on a machine, a branch is pushed only with
  `tools/workflow/native/native-push.sh <name>`, or `native-push.sh <name>
  <owner>/<repo>` for a branch in a second repository's clone, whose
  worktree is `.scratch/<owner>/<repo>/<name>`, made with `git -C
  .dev/checkouts/<owner>/<repo> worktree add`; and committed with `git -C
  <worktree> commit -F ../<name>.msg`, the message written beside the
  worktree, never `-m`. `workflow-native` says the same.
- **The cleanup.** After the reset, for each clone the machine lists, the
  wrapper lists `git -C .dev/checkouts/<owner>/<repo> worktree list
  --porcelain` entries under `.scratch/<owner>/<repo>/` and applies the
  approved rule there: a worktree on `feat/<item>` is removed, and its
  branch deleted with `git branch -d`, when the item's queue file says
  `status: done` or `origin/feat/<item>` is gone from that clone after its
  prune fetch. A dirty worktree or a branch `-d` refuses goes into
  `leftover` as approved, its note naming the repository.

### What Jeff sets up

Each of these is done by Jeff, or by an attended session on his word in
that session (umbrella a11); none by an unattended agent (LOOP-05,
FLOW-08):

- The later `repositories` entries in `config.yaml`, in an attended
  session: an entry when a repository is first named after this item,
  and a repository's `profile` only after the pull request that adopts it
  has merged, this repository's and the relay's included (Decision 3).
  (The entries for this repository and `buildwarden/buildwarden-specs`
  are written before this item's merge with their `trunk` only,
  Implementation plan task 14a; this repository's `profile` is added in
  the same sitting immediately after the merge, before the next run,
  task 14b.)
- The workers' access to each repository: the cloud environment's GitHub
  access and toolchain (this session's GitHub access did not include
  `buildwarden/buildwarden-relay` on 2026-11-17), and each native
  machine's clone and toolchain.
- Each repository's settings: head-branch deletion on merge (FLOW-16),
  trunk protection where the host offers it (FLOW-08, LOOP-08 step 6). As
  read on 2026-11-17, `buildwarden/buildwarden-specs` is public and
  deletes head branches on merge.
- The relay's visibility, which is his. Under q1 (C) the loop publishes
  its work as a public repository's whichever it is (Decision 19).

### What this item does not do

- No repository is created, and no repository setting is changed.
- No move: `wf`, the skills, the agents, the rules and the queue stay here
  (`loop-move`).
- No adoption of the contract by the relay or the specs repository. The
  relay adopts in its own item (its M1b task 10).
- No change to FLOW-05's relations or limits. An item homed in an adopted
  second repository counts in the limits as a feature here does (a6); the
  relations already read areas per repository.
- No hold that names a repository (a9), a departure from `loop-parallel`'s
  hand-off of its Decision 18: a hold on `cloud` stops refreshes and second
  merges in every repository the worker serves while it lasts.
- No `private` key, no `wf validate` rule for one, and no second store
  for a private repository's reports: its work is published as a public
  one's from its first item (q1 (C), Decision 19).

### Rule changes in brief

Amended text: FLOW-02 (`merge_first`, prefixed areas, and the keys this
repository's profile adds marked as its, LOOP-08; its enforcement,
`Front.Problems`, also reads the profile's keys), FLOW-03, FLOW-04,
FLOW-08, FLOW-10, FLOW-16, FLOW-17, LOOP-01, LOOP-08, PROFILE-01.
Enforcement only, no text change: FLOW-03's area check (`insideAreas`,
`tools/workflow/check/areas.go`, per repository), FLOW-12 (the
pre-standard handling moves with the front-matter check), FLOW-14 (`kindOf`
and `workflowRunsRe` read the profile at the merge base), LINT-01 and
LINT-05's checks moved into `wf` (a13; LINT-05's owner line in
`rules.yaml`), LINT-04 (`tier2Report` checks the report's `heads`, a12).
The wording is under **Rule changes**.

## Defaults and overrides

| Default | Why it is the obvious one (GLOBAL-09) | Override, used exclusively (GLOBAL-10) |
|---|---|---|
| `work` absent: the item has no branch in a second repository, and its gates run in the home only | It is what every queue file means today | `wf declare` writes `work` from the design's `repos` |
| `first` absent on a `work` entry (the repository is not in `merge_first`): its merge is confirmed by the existing `f7-merge` step, in the Merge phase, with no step of its own (a5) | Most other pull requests need nothing merged first; Merge is where every pull request merges today (FLOW-08) | `merge_first` in the design's front matter, which `wf declare` writes as `first: true`, so the merge is confirmed by `f6-merged-<n>` in Publish |
| A host step's `repo` absent: the step concerns the item's home | Every host step today is in this repository, the home of every queue file | `repo`, naming a `work` entry |
| `-checkouts` absent: `wf` reads no second repository. An item with no `work` behaves as today, and a check that needs a second repository's clone fails, naming the flag and the expected path (a11) | No item before this one names a second repository; a silent default directory would be a second name for what the skills already pass (a11 (b)) | `-checkouts DIR` |
| The repository's entry names a `profile` (added only after the adoption merged, Decision 3) and the merge base has no file there while the branch has one (a branch that adds the file other than through the merged adoption): every path the branch adds or changes under the new file's rule and machinery paths is a rule path for the clearance (a2) | The branch cannot be trusted to classify its own edits, and there is no earlier file to read | None |
| A `native.prepare` command exits non-zero: its hosts are stale for the run, and the session runs with their steps skipped and the reason in the digest | It is what `native-tick.sh` does today; one stale asset should not block the machine's other hosts | None |
| A repository with no profile (its `config.yaml` entry gives its `trunk` and no `profile`): no fast gate, no reference documents, and no rule or machinery paths for the clearance, whatever profile file a branch adds there; its branch, area, trunk and pull-request checks run (Decision 5) | LOOP-01: a term the component does not have is `none`; the loop's rules and machinery are not kept there | A `profile` in its entry, added on Jeff's word once its adoption pull request has merged; the adopting pull request itself is checked with none (Decision 3) |
| A repository's visibility: not recorded; an item in a private repository writes to the public queue state what any item writes (q1 (C), Decision 19) | Jeff's answer: the relay goes public at M1, and a second store or a gate on visibility would serve only until then | None |
| A key absent from a profile file: the term is `none` | LOOP-01: a term the component does not have is `none` | The key |
| A bare area name: an area of the item's home (a8) | Every queue file today is homed here; a relay design names its own areas bare | `<owner>/<name>:<area>` |
| `hosts.yaml` `repositories` absent: the queue's own only | It is what every machine serves today | The list |
| The branch name in a second repository: the home's (a4) | One name per item | None |
| `wf main` and `wf areas -check` without `-repository`: the queue's repository | Today's behaviour | `-repository` |

Beyond the prepare fallback above, no fallback: a missing clone, an
unreadable profile file, a repository with no entry (the queue's own
included), or a host step whose repository no machine serving its host
name lists fails and names what to fix
(GLOBAL-11). The transitions named here: the `repositories` entries for
every repository an item already in the queue names (this repository and
`buildwarden/buildwarden-specs`, the latter named by the in-progress
`relay-collector-failure-fails-open`) are written before the merge with
their `trunk` only (Implementation plan task 14a), so no item meets an
absent entry at the merge; this repository's `profile` is added in the
same sitting immediately after the merge, before the next run (task 14b),
since until then the new `wf` reads it as not adopted (Decision 3), and
once it is, `wf validate` passes on that item unchanged; and an item
declared before the merge with `repos` beyond its home keeps the `human` merge-recording step it may already have
until its Close; it is not converted (FLOW-04's old clause binds its branch
until it merges `main`, LOOP-09).

An accepted cost (Jeff, 2026-11-17): when a repository's entry gains its
`profile` (this repository's at task 14b, the relay's after its
adoption), every branch open there that forked before the adoption merged,
other than a process item's, which is exempt (FLOW-14), fails the rule-change clearance, naming the file, until it merges its
trunk (Decision 5). A branch before Publish gets no automatic refresh for
it.

## Cross-driver impact

None. No driver tree is reached; `tools/relay-vm/hyperv/refresh-cache.sh`
is named in the profile file and not changed.

## Rule changes

Each entry quotes the current text at `0cd3c02` and gives the proposed
text in full for each amended paragraph. Severities and check types do not
change.

Two process items ahead of this one amend some of the same paragraphs,
and both are designed and approved (a15): `loop-parallel-rule-wording`,
approved at `d0b4d46` (recorded at `6d134b0`), and `loop-native-lead`,
approved at `74deb30` (recorded at `dbd3032`). The overlapping paragraphs:

| Paragraph | Also amended by |
|---|---|
| FLOW-02, the `areas` bullet | `loop-parallel-rule-wording` |
| FLOW-02, the last paragraph | `loop-parallel-rule-wording` |
| `TEMPLATE.md`, the `areas` comment | `loop-parallel-rule-wording` |
| FLOW-03, the paragraph after the table | `loop-parallel-rule-wording` |
| FLOW-04, the paragraph on taking steps (after the lead paragraph `loop-native-lead` adds) | `loop-native-lead` |
| FLOW-16, the first bullet (and the bullet `loop-native-lead` adds after it) | `loop-native-lead` |
| FLOW-17, the second paragraph | `loop-native-lead` |
| PROFILE-01, the Native hosts row | `loop-native-lead` |

For each, the entry gives (i) the current wording at `0cd3c02`, (ii) the
predecessor's approved proposed text, quoted from its design at the
recorded commit, and (iii) the proposed wording, drafted on (ii), so it is
the final combined text. This item keeps every change the approved text
makes, and changes it only where the entry's **Proposed** line says,
naming the words it replaces. The predecessors' other amendments are to
paragraphs this design does not touch: `loop-parallel-rule-wording`'s
FLOW-05 (**How two items relate** and `process.md`'s opening paragraph)
and FLOW-16 (the third paragraph, the merge proof); `loop-native-lead`'s
FLOW-04 (the `host:<name>` bullet and the lead paragraph; this design
amends the `human` bullet and the paragraph on taking steps), FLOW-05
(the blocking clause), FLOW-09, FLOW-13 and FLOW-18. This design
amends no FLOW-05 text. At
`x2-predecessors-merged` the only re-base is to check that the merged
text of each overlapping paragraph equals the approved text quoted here;
a review of the delta follows only if it differs (FLOW-19).

### FLOW-02 (amend), `docs/development/process.md`, `docs/design/TEMPLATE.md`

**Why:** a design names the repositories whose pull requests merge first
(a5), and the areas it edits in each repository it works in (a8). The
change also makes the rule neutral (LOOP-08: neutral when the rule is next
changed), as row 3 of FLOW-03 is: the keys a profile adds are marked as
this repository's in the list and in the two paragraphs after it, so no
part of FLOW-02 treats `drivers`, `surfaces` or the security note as every
component's (a17).

**Current wording,** the `drivers` bullet (`process.md:73-74`):

> - `drivers` — which drivers are touched, or `interface` for a change made at the
>   level of the driver contract or `driver.Session` (ORCH-01).

**Current wording,** the rest of the list of front-matter keys
(`process.md:75-97`):

> - `seams` — which versioned seams change (GLOBAL-08).
> - `surfaces` — which user-facing surfaces change (UX-01, ORCH-03).
> - `repos` — every repository the change works in, the one that holds the
>   design first, when there is more than that one (LOOP-04).
> - `areas` — the areas of the repository's area map whose files the change
>   edits (FLOW-05). A design that declares none is undeclared, and FLOW-05
>   then treats the item as colliding with every item in its repositories.
> - `security_boundary` — which GLOBAL-06 boundary is altered, if any. Any value other
>   than `none` requires a security note in the design.
> - `alters_relationships` — `true` when the change alters a component's role or
>   how components interact: a versioned seam (GLOBAL-08), the bootstrap model
>   (BOOT rules), which component owns a responsibility (relay, collector,
>   executor, orchestrator, a driver), or the driver contract or
>   `driver.Session`. Changes inside one component's existing role are `false`.
>   FLOW-05 limits how many such features are in flight.
> - `native_validation` — which native hosts must validate the change, and whether
>   each blocks merge (FLOW-09).
> - `tier2_gaps` — platform validation knowingly deferred (TEST-03).
> - `rules` — the rule IDs the design relies on or changes.
> - `rule_changes` — each rule change the feature makes (FLOW-14), as
>   `{rule, change: add | amend | retire, paths}`, `paths` naming every file it
>   edits, enforcement included. Empty or absent means none.
> - `concepts` — retrieval tags for the archive (FLOW-12).

**Current wording,** the paragraph after the list (`process.md:99-102`):

> Declarations are derived conservatively: a change to a shared package (`relay/`,
> `ledger/`, `collector/`, `driver/`) declares every dependent driver and host, per
> the TEST-02 caveat. `areas` name what the change edits, not what depends on
> it: the area map says which area reaches which.

**Current wording,** the last paragraph (`process.md:104-108`):

> The automated part diffs the declared `drivers` / `seams` / `surfaces` against the
> trees and files the change reaches. It checks the declared `areas` against
> what `drivers`, `surfaces` and the `rule_changes` paths imply, at Design,
> and against the paths the change edits, from Implement on (FLOW-03). The
> agent part judges the security note.

**As approved in `loop-parallel-rule-wording` (`d0b4d46`, recorded at
`6d134b0`),** the `areas` bullet and the last paragraph; the other bullets
and the paragraph after the list are as at `0cd3c02`:

> - `areas` — the areas of the repository's area map whose files the change
>   edits (FLOW-05). A design that leaves the key out is undeclared, as
>   FLOW-05 defines it, and FLOW-05 then treats the item as colliding with
>   every item in its repositories. `areas: []` is a declaration: the
>   change edits nothing but its own design, though an item whose home
>   repository has no map is undeclared whatever it declares (FLOW-05).

> The automated part diffs the declared `drivers` / `seams` / `surfaces` against the
> trees and files the change reaches. It checks the declared `areas` against
> what `drivers`, `surfaces` and the `rule_changes` paths imply, at Design,
> and against the paths the change edits, from Implement through Publish
> and at each refresh of a feature (FLOW-03). The agent part judges the
> security note.

**Proposed:** the same list, with the `drivers`, `surfaces`,
`security_boundary`, `alters_relationships` and `tier2_gaps` bullets made
neutral, the `areas` bullet replaced, a bullet added after `repos`, and a
paragraph added after the list; the `seams`, `native_validation`, `rules`,
`rule_changes` and `concepts` bullets are unchanged. The two paragraphs
after the list are made neutral too (below). The changed and added
bullets, in the list's order (the `areas` bullet drafted on the approved
text above: it replaces "the areas of the repository's area map" with
"the areas of an area map", adds two sentences on the bare and prefixed
forms after "(FLOW-05).", and keeps the rest of the approved bullet word
for word):

> - `drivers` — here: which drivers are touched, or `interface` for a
>   change made at the level of the driver contract or `driver.Session`
>   (ORCH-01).
> - `surfaces` — here: which user-facing surfaces change (UX-01, ORCH-03).
> - `repos` — every repository the change works in, the one that holds the
>   design first, when there is more than that one (LOOP-04).
> - `merge_first` — the repositories in `repos`, other than the home, whose
>   pull request merges before the home's is listed ready (FLOW-08). Empty
>   or absent means none.
> - `areas` — the areas of an area map whose files the change edits
>   (FLOW-05). A bare name is an area of the map of the item's home
>   repository; an area of another repository the item works in is written
>   `<owner>/<name>:<area>`. A design that leaves the key out is
>   undeclared, as FLOW-05 defines it, and FLOW-05 then treats the item as
>   colliding with every item in its repositories. `areas: []` is a
>   declaration: the change edits nothing but its own design, though an
>   item whose home repository has no map is undeclared whatever it
>   declares (FLOW-05).
> - `security_boundary` — here: which GLOBAL-06 boundary is altered, if
>   any. Any value other than `none` requires a security note in the
>   design.
> - `alters_relationships` — `true` when the change alters a component's
>   role or how components interact: a versioned seam (GLOBAL-08), or which
>   component owns a responsibility; here also the bootstrap model (BOOT
>   rules), which of relay, collector, executor, orchestrator or a driver
>   owns a responsibility, or the driver contract or `driver.Session`.
>   Changes inside one component's existing role are `false`. FLOW-05
>   limits how many such features are in flight.
> - `tier2_gaps` — here: platform validation knowingly deferred (TEST-03).

and after the list:

> A key whose bullet begins "here:" is one this repository's profile file
> adds to the loop's keys (LOOP-01, its impact keys), and the rest of the
> bullet is this repository's entry for that term; a design in a component
> whose profile file does not add it leaves it out. Elsewhere, in a bullet
> of one of the loop's keys and in the paragraphs below, "here" introduces
> this repository's examples of the term.

The paragraph after the list, proposed:

> Declarations are derived conservatively: a change to a shared package
> declares everything that depends on it (here a change to `relay/`,
> `ledger/`, `collector/` or `driver/` declares every dependent driver and
> host, per the TEST-02 caveat). `areas` name what the change edits, not
> what depends on it: the area map says which area reaches which.

The last paragraph, proposed, drafted on the approved text above. It keeps
that text's "from Implement through Publish and at each refresh of a
feature (FLOW-03)" and its third sentence's opening word for word, and
replaces three phrases to make the keys and the security note neutral
(a17): "diffs the declared `drivers` / `seams` / `surfaces`" becomes
"diffs the declared `seams`, and the keys the profile file adds that
name trees or files (here `drivers` and `surfaces`),"; "what `drivers`,
`surfaces` and the `rule_changes` paths imply" becomes "what those
profile keys and the `rule_changes` paths imply (here `drivers` and
`surfaces`)"; and "judges the security note." becomes "judges the
security note, where the profile's keys require one (here a
`security_boundary` other than `none`).":

> The automated part diffs the declared `seams`, and the keys the profile
> file adds that name trees or files (here `drivers` and `surfaces`),
> against the trees and files the change reaches. It checks the declared
> `areas` against what those profile keys and the `rule_changes` paths
> imply (here `drivers` and `surfaces`), at Design, and against the paths
> the change edits, from Implement through Publish and at each refresh of
> a feature (FLOW-03). The agent part judges the
> security note, where the profile's keys require one (here a
> `security_boundary` other than `none`).

**Template**, current (`TEMPLATE.md:13-15` at `0cd3c02`):

```yaml
# areas: []              # the areas the change edits (FLOW-02), by the names of the
                         # area map (`wf areas -list`). Left out, the item is undeclared
                         # and collides with every item in its repositories (FLOW-05)
```

as approved in `loop-parallel-rule-wording` (`d0b4d46`, recorded at
`6d134b0`):

```yaml
# areas: []              # the areas the change edits (FLOW-02), by the names of the
                         # area map (`wf areas -list`). Left out, the item is undeclared
                         # and collides with every item in its repositories (FLOW-05);
                         # []: the change edits nothing but its own design
```

proposed, the approved comment whose last line, "[]: the change edits
nothing but its own design", gains a period after "its own design" and
is followed by the sentence "A bare name is the home's map;
<owner>/<name>:<area> another repository's", wrapped over that line and
one more:

```yaml
# areas: []              # the areas the change edits (FLOW-02), by the names of the
                         # area map (`wf areas -list`). Left out, the item is undeclared
                         # and collides with every item in its repositories (FLOW-05);
                         # []: the change edits nothing but its own design. A bare name
                         # is the home's map; <owner>/<name>:<area> another repository's
```

and after `repos:` (`TEMPLATE.md:11-12`), add verbatim

```yaml
merge_first: []          # repositories in repos, not the home, whose PR merges before the home's is ready (FLOW-08); empty or absent: none
```

**Severity and check:** unchanged (MUST; automated + agent-judgment).

**Enforcement:** `Front.Problems` takes the profile's keys and values
(a17); `Front` parses `merge_first` and checks that each entry is in `repos`
and is not the home, and that each prefixed area names a repository in
`repos`.

### FLOW-03 (amend), `docs/development/process.md`

**Why:** the done-when checks hold in each repository an item has a branch
in, a home that has adopted gets real phases, row 3 names this
repository's commands (LOOP-08: neutral when next changed), and Publish
waits for the merge of each pull request the design names to merge first
(the `first` merge, a5).

**Current wording,** row 3 of the feature table:

> | 3 | Implement | Tasks done; `make lint` and `make test` green; reference docs updated in the same change (UX-01); every edit to the rules or their enforcement cleared by an approved rule change (FLOW-14). |

**Proposed:**

> | 3 | Implement | Tasks done; the fast gate green (LOOP-01; here `make lint` and `make test`); reference docs updated in the same change (UX-01); every edit to the rules or their enforcement cleared by an approved rule change (FLOW-14). |

**Current wording,** row 6 of the feature table:

> | 6 | Publish | Once the PR slot is free (FLOW-05): current `main` merged; consolidation section filled (FLOW-10, FLOW-15); the rule-change clearance still passes (FLOW-14); design archived with its summary on the branch (FLOW-12); PR opened (no confirmation needed, FLOW-08) with CI green (FLOW-11); blocking native validation done (FLOW-09). |

**Proposed:**

> | 6 | Publish | Once the PR slot is free (FLOW-05): current `main` merged; consolidation section filled (FLOW-10, FLOW-15); the rule-change clearance still passes (FLOW-14); design archived with its summary on the branch (FLOW-12); PR opened (no confirmation needed, FLOW-08) with CI green (FLOW-11); blocking native validation done (FLOW-09); each pull request its design names to merge first merged, and the loop has seen it (FLOW-08). |

**Current wording,** the paragraph after the table (`process.md:133-139`):

> A transition happens only when the phase's done-when checks pass mechanically
> (files present, sections filled, commands green), never on an agent's own report
> that it is done. From Plan on, each done-when also checks that the queue
> still records what the design declares; from Implement on, that every
> path the branch changes lies in an area the design declares (FLOW-02). A
> path outside them is settled by widening the declaration inside the
> approved design, or by a return to Design (FLOW-06).

**As approved in `loop-parallel-rule-wording` (`d0b4d46`, recorded at
`6d134b0`):**

> A transition happens only when the phase's done-when checks pass mechanically
> (files present, sections filled, commands green), never on an agent's own report
> that it is done. From Plan through Publish, and at each refresh of a
> feature (FLOW-10), each done-when also checks that the queue still
> records what the design declares; from Implement through Publish, and
> at each refresh of a feature, that every path the branch changes lies
> in an area the design declares (FLOW-02). A path outside them is
> settled by widening the declaration inside the approved design, or by a
> return to Design (FLOW-06). The merge confirmation (phase 7's
> done-when) and Close run neither; a feature's refresh runs both,
> including one taken while the pull request waits at Merge, and a
> process item's refresh runs neither. An item the queue records
> no declaration for, because it finished Design before the queue
> recorded declarations, is not compared with its design; its diff check
> reads the areas the queue records, so with none it is undeclared
> (FLOW-05) and no path lies outside them.

**Proposed**, the approved paragraph with "every path the branch changes
lies in an area the design declares (FLOW-02)" replaced by "every path
each of its branches changes lies in an area the design declares in that
repository's map (FLOW-02)", the rest word for word, and a new paragraph
after it:

> A transition happens only when the phase's done-when checks pass mechanically
> (files present, sections filled, commands green), never on an agent's own report
> that it is done. From Plan through Publish, and at each refresh of a
> feature (FLOW-10), each done-when also checks that the queue still
> records what the design declares; from Implement through Publish, and
> at each refresh of a feature, that every path each of its branches
> changes lies in an area the design declares in that repository's map
> (FLOW-02). A path outside them is
> settled by widening the declaration inside the approved design, or by a
> return to Design (FLOW-06). The merge confirmation (phase 7's
> done-when) and Close run neither; a feature's refresh runs both,
> including one taken while the pull request waits at Merge, and a
> process item's refresh runs neither. An item the queue records
> no declaration for, because it finished Design before the queue
> recorded declarations, is not compared with its design; its diff check
> reads the areas the queue records, so with none it is undeclared
> (FLOW-05) and no path lies outside them.
>
> An item has a branch in each repository it works in, under one name, and
> the queue records each branch and its pull request. Where a done-when
> names the branch, the fast gate, `main`, the reference documents or the
> PR, it holds in each of those repositories, read through that
> repository's profile (LOOP-01). In a repository that has not adopted the
> contract (LOOP-08), which the loop's configuration records with no
> profile, there is no profile to name a gate: the fast gate and the
> reference-document checks do not run there, no path there is a rule
> path, and the branch, area, trunk and pull-request checks do run. The
> design and its review are the home's. "`main`" in the tables above reads
> as each repository's trunk.

**Current wording,** the tracked paragraph:

> A feature whose home is a repository in which the loop's gates do not
> run is **tracked**. The queue records it, its repository and its pull
> request. Its phases are two, Work and then Merge, each one `human` step
> (FLOW-04), and no done-when check runs for them. FLOW-05's limits
> neither count it nor hold it.

**Proposed:**

> A feature whose home has adopted the contract has the phases above,
> whichever repository that is. A feature whose home has not adopted it is
> **tracked**. The queue records it, its repository and its pull
> request. Its phases are two, Work and then Merge, each one `human` step
> (FLOW-04), and no done-when check runs for them. FLOW-05's limits
> neither count it nor hold it.

**Enforcement:** `check.Phase` runs per repository as under **The gates,
per repository**, with `f6-merged-<n>` per `first` entry (The gates, per
repository) and `insideAreas` (`tools/workflow/check/areas.go`) against
each repository's map; the tracked test in `check.go:46-52` reads the
home's `config.yaml` entry (no `profile`) in place of "not the queue's
repository" (`Tracked`, `queue.go:627-632`). The tracked test moves to the
profile check, and the process-home check stays on `repos[0] !=
repository`: `validateKind` takes a separate `homedHere` (a6).

### FLOW-04 (amend), `docs/development/process.md`

**Why:** the loop sees every merge it records (Decision 6), so the
merge-recording step has no case left but a tracked item; and a machine
takes a host step only in a repository it serves (a14), which the rule
must say as `wf gate` and `wf validate` enforce it.

`loop-native-lead`, as approved (`74deb30`, recorded at `dbd3032`),
amends FLOW-04's `host:<name>` bullet and the paragraph after the list,
which it splits into a paragraph on the lead and one on taking and
claiming steps. It does not amend the `human` bullet, so for that bullet
the current wording is this entry's basis. Its approved host bullet
("`hosts.yaml` lists the machines that serve each host name, one or
more") and its lead paragraph are kept as approved; its paragraph on
taking steps is this entry's basis for the second amendment below.

**Current wording,** the `human` bullet (`process.md:177-182`):

> - `human` — answering blocking questions, the design review (FLOW-19), and
>   escalations: a runaway budget (FLOW-13), CI repair exhausted (FLOW-11), a third
>   return to Design (FLOW-06), a branch that did not delete on merge (FLOW-16).
>   Also the steps of work in a repository where the loop's gates do not
>   run: a tracked item's steps (FLOW-03), and a step that records the
>   human's merge there (FLOW-08).

**Proposed:**

> - `human` — answering blocking questions, the design review (FLOW-19), and
>   escalations: a runaway budget (FLOW-13), CI repair exhausted (FLOW-11), a third
>   return to Design (FLOW-06), a branch that did not delete on merge (FLOW-16).
>   Also a tracked item's steps (FLOW-03), and a step its design adds that
>   only the human may take, such as cutting a release.

**Current wording,** the paragraph after the list (`process.md:190-199`),
which `loop-native-lead` splits into its lead paragraph and the paragraph
on taking steps below:

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

**As approved in `loop-native-lead` (`74deb30`, recorded at `dbd3032`),**
the paragraph on taking steps, after the lead paragraph:

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

**Proposed**, the approved paragraph with one clause inserted in its
first sentence after "a machine satisfies each host name the table lists
for it,", and nothing else changed. The inserted words are "for a step in
a repository the table lists for that machine (only the queue's own
repository when it lists none; a led item's step is in every repository
the item works in),":

> A worker takes only steps whose affinity it satisfies: a machine satisfies
> each host name the table lists for it, for a step in a repository the
> table lists for that machine (only the queue's own repository when it
> lists none; a led item's step is in every repository the item works
> in), and a `host:<name>` step goes to whichever such worker next takes
> steps. A step is offered when
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

**Enforcement:** the `human` bullet, none; `wf` does not restrict what a
`human` step is for. The paragraph on taking steps: `wf gate` offers a
host step only to a machine that serves its host name and lists its
repository, and a led item's steps only to a machine that serves the
lead and lists every repository in the item's `repos`; `validateNative`
fails a host step or a lead no serving machine can take, with `wf lead
-host`, `wf new -lead` and `wf declare` refusing to create that state
(a14; **The native side**).

### FLOW-08 (amend), `docs/development/process.md`

**Why:** an item may have pull requests in several repositories, each
merged by the human and each seen by the loop. The change also makes the
merge ban neutral (LOOP-08), as row 3 of FLOW-03 is, so it covers each
repository's trunk (the profile's Trunk term, LOOP-01).

**Current wording,** the first paragraph's first sentence:

> This is LINT-03 applied to the process. No agent approves or merges a PR to `main`,
> and auto-merge is not enabled by agents.

**Proposed:**

> This is LINT-03 applied to the process. No agent approves or merges a PR
> to a trunk, in any repository, and auto-merge is not enabled by agents.

**Current wording,** the end of the first paragraph:

> There is no sign-off step in the queue for a pull request the loop can
> see: the loop sees the merge (FLOW-03, phase 7). For a pull request in a
> repository where the loop's gates do not run, a `human` step records the
> merge (FLOW-04). The merge is still the sign-off; the step is only the
> queue's record of it.

**Proposed:**

> There is no sign-off step in the queue: the loop sees the merge of each
> pull request an item records, in whichever repository it is (FLOW-03,
> phase 7). Only a tracked item's merge is recorded by a `human` step
> (FLOW-04); the merge is still the sign-off, and the step only the queue's
> record of it. An item with pull requests in several repositories is
> signed off by the merge of each. Its design names any that must merge
> first. Each of those is opened in Publish and is ready for the merge as
> any other is, and the home's Publish, and any step that needs one of
> those merges, waits until the loop has seen it (FLOW-03, phase 6).

**Current wording,** the second paragraph's second sentence:

> Each merge to
> trunk therefore leaves every other open pull request waiting for a
> refresh (FLOW-10), and the loop lists none of them as ready until it
> has had one.

**Proposed:**

> Each merge to
> a trunk therefore leaves every other open pull request to that trunk
> waiting for a refresh (FLOW-10), and the loop lists none of them as
> ready until it has had one.

**Current wording,** the third paragraph's first sentence:

> Neither pushing a feature branch (FLOW-17) nor opening its PR to `main` is a
> trunk change, and neither needs confirmation.

**Proposed:**

> Neither pushing a feature branch (FLOW-17) nor opening its PR to its
> trunk is a trunk change, and neither needs confirmation.

**Enforcement:** `merged` (`check.go:348-430`) runs per `work` entry
against its trunk; the refreshes per trunk are FLOW-10's (below).

### FLOW-10 (amend), `docs/development/process.md`

**Why:** an item has a branch, and possibly a pull request, in each
repository it works in. Each branch is consolidated against its own trunk.

**Current wording,** `process.md:451-453`:

> Publish, after merging current `main`, reviews the feature against everything
> that landed on `main` since its branch forked, and records in the design's
> consolidation section:

**Proposed:**

> Publish, after merging each trunk the item has a branch on (FLOW-03),
> reviews the feature against everything that landed on each since the
> branch forked, and records in the design's consolidation section:

**Current wording,** the refresh paragraph (`process.md:471-488`):

> A PR that waits for the human's merge may be merged only while its
> branch contains `main` (FLOW-08). Whenever `main`
> has moved past its branch, a **refresh** step merges `main` into the
> branch again, runs the gates, and repeats this review, and FLOW-15's
> sweep, over what landed since the branch last merged `main`, as a dated
> entry in the consolidation section. The PR is ready again when the
> step's checks pass. It pushes only while the pull
> request is still open. A feature that has merged `main` in Publish and
> has not finished Publish is refreshed the same way when `main` moves,
> before its PR is opened or listed as ready. No refresh is taken while
> `main` is known to
> fail its own checks, except for the item that fixes it (FLOW-09): it
> would fail on the same failure. A refresh that finds work beyond
> conflict fixes
> and wording sends the feature back as above; its pull request is then a
> draft until the feature is published again, and it keeps the PR slot
> (FLOW-05). A process item's open PR is refreshed the same way, without
> the review.

**Proposed:**

> A PR that waits for the human's merge may be merged only while its
> branch contains its trunk (FLOW-08). Whenever a trunk has moved past the
> item's branch on it, a **refresh** step merges that trunk into that
> branch again, runs the gates in that repository, and repeats this review
> and FLOW-15's sweep over what landed since that branch last merged its
> trunk, as a dated entry in the consolidation section naming the trunk.
> The PR is ready again when the step's checks pass. It pushes only while
> the pull request is still open. A feature that has merged its trunks in
> Publish and has not finished Publish is refreshed the same way when a
> trunk moves, before its PR is opened or listed as ready. No refresh of a
> branch is taken while its trunk is known to fail its own checks (the
> trunk's `main.yaml`, `repos/<owner>/<name>/main.yaml` for a second
> repository), except for the item that fixes that trunk (FLOW-09): it
> would fail on the same failure. A refresh that finds work beyond
> conflict fixes and wording sends the feature back as above; its pull
> request is then a draft until the feature is published again, and it
> keeps the PR slot (FLOW-05). A process item's open PR is refreshed the
> same way, without the review.

"Its pull request" reads per repository throughout.

**Enforcement:** `stale`, `staleItem` and `staleSeed`
(`tools/workflow/cmd/wf/main.go:828-900`) check each `work` entry's branch
against its trunk. Each reads the red-trunk state for that trunk from the
`main.yaml` `wf main -repository` writes, and seeds one refresh naming the
behind branches. That code is the loop's machinery and changes as this
process item's own, not under `rule_changes`.

### FLOW-16 (amend), `docs/development/process.md`

**Why:** an item's branches are in several repositories, and the deletion
setting is the host's (LOOP-08: no CI system or host assumed).

**Current wording,** the first bullet (`process.md:632-635`):

> - its branch, on the remote and locally, and any worktree checked out on it.
>   GitHub deletes the remote branch on merge (the repository's "Automatically
>   delete head branches" setting); Close confirms it is gone, and escalates to a
>   `human` step with the exact command if it is not;

**As approved in `loop-native-lead` (`74deb30`, recorded at `dbd3032`),**
the first bullet unchanged and a new bullet after it:

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

(`loop-parallel-rule-wording` amends FLOW-16's third paragraph, the merge
proof, which this entry does not touch.)

**Proposed**, the first bullet (which `loop-native-lead` keeps as at
`0cd3c02`) made per repository: "its branch" becomes "its branch in each
repository it works in", "GitHub deletes the remote branch on merge (the
repository's "Automatically delete head branches" setting)" becomes "The
host deletes a remote branch on merge where the repository is set to (on
GitHub, "Automatically delete head branches")", "Close confirms it is
gone" becomes "Close confirms each is gone", and "with the exact command
if it is not" becomes "with the exact command for each that is not". The
approved new bullet with ", in each repository's checkout there" inserted
after "for the item", and "gone from the remote" replaced by "gone from
that repository's remote":

> - its branch in each repository it works in, on the remote and locally,
>   and any worktree checked out on it. The host deletes a remote branch on
>   merge where the repository is set to (on GitHub, "Automatically delete
>   head branches"); Close confirms each is gone, and escalates to a
>   `human` step with the exact command for each that is not;
> - on each native machine, the worktrees and local branch its sessions made for
>   the item, in each repository's checkout there: not by Close, which cannot
>   reach them, but by the machine at its
>   next scheduled run once the item is done or its branch is gone from
>   that repository's
>   remote, with `git worktree remove` and `git branch -d` only, reporting a
>   worktree with uncommitted changes or a branch `-d` refuses instead of
>   removing it;

**Enforcement:** `closed` per repository; the inline Close of
`workflow-tick`.

### FLOW-17 (amend), `docs/development/process.md`

**Why:** the pushes and the gate are per repository, and the commands are
the profile's.

`loop-native-lead`, as approved (`74deb30`, recorded at `dbd3032`),
amends FLOW-17's second paragraph (a machine pushes only through the
loop's push script, "which pushes one branch of an item in progress,
fast-forward only"), and not the first paragraph. Its added sentence
already covers a branch in any repository the item works in; the push
script takes a repository argument for a branch in a clone (a14, **The
native side**). The paragraph's first two sentences, which it keeps as
at `0cd3c02`, still name `main`; FLOW-03's reading of "`main`" as each
repository's trunk covers only its own tables, and a repository that has
not adopted has no profile to read `main` through (Decision 5). So, as
FLOW-08's entry does, this entry amends both paragraphs so that the push
ban covers each repository's trunk.

**Current wording,** the first paragraph (`process.md:672-675`):

> An agent pushes its feature branch to the remote, without asking, whenever the
> work reaches a successful checkpoint: the change is committed, and `make test`
> and `make lint` are green (lint run for each platform whose build-tagged files
> the change touches). The pushed branch is the backup and the hand-off point.

**Proposed:**

> An agent pushes its feature branch to the remote, without asking, whenever the
> work reaches a successful checkpoint: the change is committed, and the
> repository's fast gate is green (LOOP-01; here `make test` and `make lint`,
> lint run for each platform whose build-tagged files the change touches). An
> item that works in several repositories has a branch of the same name in
> each, pushed to that repository at its own checkpoints. The pushed branch is
> the backup and the hand-off point.

**Current wording,** the second paragraph (`process.md:677-679`):

> Opening the PR to `main` needs no confirmation either (FLOW-08). Pushing
> never includes `main` itself, a force-push, rewriting history that is already on
> the remote, or deleting a remote branch (FLOW-16).

**As approved in `loop-native-lead` (`74deb30`, recorded at `dbd3032`):**

> Opening the PR to `main` needs no confirmation either (FLOW-08). Pushing
> never includes `main` itself, a force-push, rewriting history that is already on
> the remote, or deleting a remote branch (FLOW-16). An unattended session on a
> native machine pushes a feature branch only through the loop's push script
> on trunk, which pushes one branch of an item in progress, fast-forward only,
> and the queue state only through the loop's state commit; it runs no other
> push.

**Proposed**, the approved paragraph with "Opening the PR to `main`"
replaced by "Opening the PR to its trunk" and "Pushing never includes
`main` itself" replaced by "Pushing never includes any repository's
trunk itself", the rest word for word (its "the loop's push script on
trunk" is the loop's own trunk, and stays):

> Opening the PR to its trunk needs no confirmation either (FLOW-08).
> Pushing never includes any repository's trunk itself, a force-push,
> rewriting history that is already on the remote, or deleting a remote
> branch (FLOW-16). An unattended session on a native machine pushes a
> feature branch only through the loop's push script on trunk, which
> pushes one branch of an item in progress, fast-forward only, and the
> queue state only through the loop's state commit; it runs no other
> push.

**Enforcement:** none beyond the `pushed` check at phase 2 of a process
item and the remote heads the gates read; on a machine, the push
script and `settings.native.json`'s denies, as `loop-native-lead`
approves them, with the clone push form of **The native side**.

### LOOP-01 (amend), `docs/development/loop-contract.md`

**Why:** `wf` reads a profile through a file (Decision 1), and the file has
to be the profile's, not a second source; the trunk the tools use is the
loop's configuration's, not the file's (Decisions 2 and 3), and the
configuration names the file only once the adoption has merged
(Decision 3).

**Current wording,** the paragraph after the table of terms:

> Where a FLOW or LINT rule names a path, a command or a rule of
> `buildwarden/buildwarden`, a component reads it as its own profile's
> entry for that term. Where the entry and the rule's example differ, the
> entry is the component's. An entry may name the file that holds a fact
> in place of restating it. A term the component does not have is filled
> as `none`, and a rule read through a `none` term does not apply there. A
> term left unfilled is a finding; it is never read as another component's
> entry.

**Proposed:**

> Where a FLOW or LINT rule names a path, a command or a rule of
> `buildwarden/buildwarden`, a component reads it as its own profile's
> entry for that term. Where the entry and the rule's example differ, the
> entry is the component's. An entry may name the file that holds a fact
> in place of restating it. A term the component does not have is filled
> as `none`, and a rule read through a `none` term does not apply there. A
> term left unfilled is a finding; it is never read as another component's
> entry.
>
> The entries the loop's tools read are also written in a **profile
> file**, in the format the loop's tools document, which the profile names
> and the loop's configuration records for the repository. The file is
> part of the profile: a change to it is a change to the profile, and
> where the file and the profile differ, that is a finding. A key the file
> leaves out is the term `none`. The loop's configuration also records
> each repository's trunk, and that record is the trunk the loop's tools
> use, never the file's: a profile file whose trunk differs from it is a
> finding, and every check of the loop's tools that reads the file fails
> on it. The configuration records a repository's profile file only once
> the change that adopts the repository (LOOP-08) has merged.

**Current wording,** the agent part:

> The agent part: the design review of a change to a profile judges that
> every term is filled or `none`; Review judges that the paths and commands
> a change relies on are the profile's.

**Proposed:**

> The agent part: the design review of a change to a profile judges that
> every term is filled or `none`, and that the profile file agrees with
> it; Review judges that the paths and commands a change relies on are the
> profile's.

**Enforcement:** `tools/workflow/check/profile.go` parses the file and
fails on an unknown `version`, and on a `trunk` that differs from the
repository's `config.yaml` entry (Decision 2); `wf validate` and the
checker read the trunk from that entry only (Decision 3).

### LOOP-08 (amend), `docs/development/loop-contract.md`

**Why:** adoption now includes the profile file, and the contract's list of
what it assumes must say so. The change that adopts is its own item, and
the loop's configuration records the profile file only after it merges
(Decision 3); LOOP-01's new paragraph says so, and LOOP-08's steps need
no other words for it.

**Current wording,** step 2:

> 2. writing its profile (LOOP-01), every term filled or `none`;

**Proposed:**

> 2. writing its profile (LOOP-01), every term filled or `none`, and its
>    profile file;

**Current wording,** the first sentence of the assumptions paragraph:

> The contract assumes of a component: git, a trunk and feature branches,
> pull requests that a human merges, a deterministic check that can block
> a pull request, Markdown rule files, and designs with front-matter.

**Proposed:**

> The contract assumes of a component: git, a trunk and feature branches,
> pull requests that a human merges, a deterministic check that can block
> a pull request, Markdown rule files, a profile file in the loop's
> format, and designs with front-matter.

**Enforcement:** none.

### PROFILE-01 (amend), `docs/development/loop-profile.md`

**Why:** this repository's profile file, and the native preparation moved
out of the wrapper (a14).

**Current wording:**

> The table below is `buildwarden/buildwarden`'s profile under the Loop
> Contract (LOOP-01). Where an entry names a file, that file holds the
> fact. An entry that no longer matches the repository is a finding, and
> the change that makes it stale changes the entry.

**Proposed:**

> The table below is `buildwarden/buildwarden`'s profile under the Loop
> Contract (LOOP-01). Where an entry names a file, that file holds the
> fact. An entry that no longer matches the repository is a finding, and
> the change that makes it stale changes the entry. The profile file is
> `docs/development/loop-profile.yaml` (LOOP-01): it holds the entries the
> loop's tools read, other than the trunk, which they read from the loop's
> configuration, and a change to an entry it holds changes both in the
> same change. Its `trunk` repeats the Trunk entry and must equal the
> loop's configuration (LOOP-01). The file holds the fast gate's
> host-platform commands; the per-platform lint is the entry's alone.

**Current wording,** the Native hosts row (`loop-profile.md:29`):

> | Native hosts | The host names in the loop's `hosts.yaml`. All of them serve this repository's drivers today. |

**As approved in `loop-native-lead` (`74deb30`, recorded at `dbd3032`):**

> | Native hosts | The host names in the loop's `hosts.yaml`. All of them serve this repository's drivers or its platform builds today. |

**Proposed**, the approved row with one sentence added:

> | Native hosts | The host names in the loop's `hosts.yaml`. All of them serve this repository's drivers or its platform builds today. Before a session on a machine that serves `windows-hyperv`, the native wrapper runs `tools/relay-vm/hyperv/refresh-cache.sh` (the profile file's `native.prepare`), and the install script runs it with `--check` (its `native.check`). |

**Enforcement:** none; the agent part as written.

### FLOW-12, FLOW-14, LINT-01, LINT-04 and LINT-05: enforcement only

No rule text changes. `kindOf` and `workflowRunsRe` take the profile's
paths and pattern, read at the merge base (a2). Under a13 (b), the rule
index's parser (`tools/tier1/rules/rules.go`), the suppression check (the
`Tier1-Allow` trailer check in `Diff.trailer`, `tools/tier1/diff/diff.go`,
check ID `suppression`) and the front-matter check with FLOW-12's
pre-standard handling (`tools/tier1/coupling/frontmatter.go`) move into
`tools/workflow/check/` (`ruleindex.go`, `suppression.go`,
`frontmatter.go`), and tier 1 calls them there. `tier2Report` checks that
the report's `heads` cover every branch of the item (LINT-04, a12). For
this repository each check gives the same result on the same input, which
the tests below pin.

LINT-05's owner line in `tools/tier1/rules.yaml` changes; this is the only
text change, to `rules.yaml`, not to the rule:

- current: `LINT-05: {owner: "tier1 itself: report.schema.json and Tier1-Allow trailers"}`
- proposed: `LINT-05: {owner: "tier1's report.schema.json; Tier1-Allow trailers by tools/workflow/check (suppression), which tier1 and wf rules call"}`

## Validation plan

- **Tier 0/1** (`make test`, `make lint`; TEST-01): `wf`'s tests build
  their repositories with `git init` as they do today
  (`check_test.go`, `cmd/wf/*_test.go`). New cases, with two local bare
  repositories as remotes:
  1. A profile file round-trips; an absent key is `none`; an unknown
     `version` fails naming the file. A profile file whose `trunk`
     differs from its `config.yaml` entry's fails, naming both values;
     a branch whose profile file sets `trunk` to the branch's own name
     still has its merge base taken against the entry's trunk, so its
     clearance diff is not empty (Decision 2).
  2. The front-matter check with this repository's profile file gives
     today's result for every archived design (`AllDesigns` over
     `docs/design/`, as tier 1's `design-front-matter` does now).
  3. `kindOf` and `workflowRunsRe` with this repository's profile file
     classify every path of the tree at `main` and every path the merges since #100 changed as today.
     The clearance reads them at the merge base: a branch that edits
     `docs/development/loop-profile.yaml` to remove `docs/development/`
     from `rules.paths`, and also edits `docs/development/process.md`,
     fails `ruleClearance` on both paths, with no approved `rule_changes`
     entry. The same holds for a branch that deletes the `rules` key, and
     for one that edits `rules.rule_jobs` to stop a CI job counting as a
     rule job while changing that job (`workflowGuardChanged` still flags
     it). A process item's branch making the same edits stays exempt.
     Where the entry names no `profile`: no path is a rule path, including
     on an adopting branch that adds a profile file and rule files there,
     which passes the clearance, and a second branch open in that
     repository at the same time passes it too (Decision 3). Once the
     adopting branch has merged and the entry gains its `profile`: a
     branch forked before the adoption, with the file at neither its merge
     base nor its head, fails the clearance naming the file, and passes
     once it merges the trunk; a branch that adds the file where the
     merge base has none has every path it changes under the new file's
     rule and machinery paths classified as a rule path; a file
     unparseable at the merge base fails the clearance, naming the
     file.
  4. An item with a `work` entry: phase 0 fails until both branches exist;
     phase 3 runs each repository's fast gate in its clone and none where
     the repository has no profile; a path outside the second
     repository's areas fails `insideAreas`; phase 4 fails while the
     report's `heads` lack the second repository's head or name a stale
     one, and runs a trial merge onto each trunk; phase 5 checks each
     adopted repository's guides and none where the repository has no
     profile; phase 6 needs a green PR in each; phase 7 passes only when
     both merges are proved; Close fails while either branch remains.
  5. `first`: `f6-merged-<n>` passes only on the merge of that entry, and
     a design-added step that needs it is not offered before.
  6. `wf stale` seeds a refresh when only the second branch is behind its
     trunk.
  7. An item homed in a repository with a profile is not tracked; one
     homed in a repository whose entry names no `profile` is. A process item whose
     `repos[0]` is an adopted second repository (one with a `profile`)
     fails `wf validate` with the process-home error.
  8. `wf validate`: a repository with no entry; an entry with no `trunk`;
     `work` that disagrees with `repos`;
     a `merge_first` entry not in `repos`, or the home; a prefixed area
     naming a repository not in `repos`.
  9. With `hosts.yaml` listing `container-any` on two machines (FLOW-04
     as `loop-native-lead` approves it): a host step in the relay is
     offered by `wf gate` only on the machine whose `repositories` list
     it, and `wf validate` passes; with neither listing it, the step is
     offered on neither and `wf validate` fails it, naming both machines,
     the host name and the repository (including the case where
     `repositories` is absent and the repository is not the queue's own).
     With one machine serving the name, the same holds for it alone. A led
     item whose second repository no machine serving the lead lists fails
     `wf validate`, naming them. With `container-any` served by two
     machines and only one listing every repository of a led item's
     `repos`, `wf gate` offers its led steps only on that one, and never
     on the other, whose other host steps it still offers. `wf lead -host`
     and `wf new -lead` refuse
     a lead no serving machine lists every repository of the item's `repos`
     for, with the same message, and accept it once one machine does; `wf
     declare` on a led item refuses a `work` that leaves no such machine,
     leaving the lead and the queue file unchanged. The fingerprint
     changes when the second trunk moves.
  10. A copy of the `workflow` branch's state at `c0b3295` (no `work`, no
      `repositories`), checked in as a test fixture under
      `tools/workflow/cmd/wf/testdata/`, gives the same `wf next`, `wf
      status -md` and `wf validate` output with the old `wf` and with the
      new `wf` given the same state plus the `repositories` entries as
      they stand after task 14b (`buildwarden/buildwarden` with `trunk:
      main` and its `profile`, `buildwarden/buildwarden-specs` with
      `trunk: main`) (a10). It
      includes the in-progress `relay-collector-failure-fails-open`, whose
      `repos` names `buildwarden/buildwarden-specs`: `wf validate` passes
      on it with no new error or notice. With only task 14a's entries
      (each `trunk` only), the new `wf` reports no absent entry, and reads
      this repository as not adopted: `wf validate` gives the tracked-round
      error for each item homed here whose round is `feature`, which is
      why task 14b comes before the next run (Decision 3).
  11. Under a13 (b): tier 1's report on `main` is byte-for-byte the same
      before and after the move of the three checks' code.
  12. A stub repository with a minimal profile file (no `rules.index`, no
      `impact` keys): `wf rules` and `wf rules -base REV` run the three
      checks, `rule-index` checking only the rule IDs; `Front.Problems`
      accepts a design there with no `drivers`, `surfaces`,
      `security_boundary` or `tier2_gaps`, and still fails one missing a
      loop key (a13 (b), a17).
  13. `wf main -repository <owner>/<name>` writes
      `repos/<owner>/<name>/main.yaml` and leaves the root `main.yaml`
      unchanged; without the flag it writes the root `main.yaml` as today.
      `wf areas -check -repository <owner>/<name>` compares
      `repos/<owner>/<name>/areas.yaml` with the edges piped in, and
      reports a difference as the root check does (Decision 8).
  14. `wf declare` on a design whose `repos` names two repositories after
      the home and whose `merge_first` names the second writes `work` with
      one entry for each, the home's branch name in both and `first: true`
      on the second only; re-declaring with one repository removed refuses
      while that entry's branch exists on its remote, and drops the entry
      once it does not (Decision 4).
  15. A check that needs a second repository's clone fails, naming
      `-checkouts` and the expected path `DIR/<owner>/<name>`, both when
      the flag is absent and when the clone is missing under it (Decision
      12, a11).
- **Native:** `native_test.go` covers the wrapper's new resets and the
  prepare command with stub repositories, including a stub prepare that
  exits 1: the session still starts, its prompt carries the stale-host
  skip for that host only, and hosts with no prepare are unaffected. Its
  table test of `native-push.sh` gains: a push from a worktree
  `.scratch/<owner>/<repo>/<name>` of a second repository's clone to that
  clone's bare remote, with `<owner>/<repo>` in the item's `repos`; the
  same refused when the item's `repos` does not name the repository, when
  the clone's `origin` names another repository, when the worktree belongs
  to another clone, when the argument is the queue's own repository, and
  with a third argument; and the one-argument form unchanged.
  `TestNativeSettingsRules` gains cases by its own matcher: `git -C
  .dev/checkouts/o/r fetch origin` is allowed by the new `Bash(git -C
  .dev/checkouts/* fetch*)` and `git -C .dev/checkouts/o/r worktree add
  ../../../../.scratch/o/r/x feat/x` by the new `Bash(git -C
  .dev/checkouts/* worktree*)`; other git commands in a clone (`git -C
  .dev/checkouts/o/r commit -m x`, `... checkout main`, `... reset --hard`,
  `... config user.name x`) are not allowed; `git -C .dev/checkouts/o/r
  push origin feat/x`, `... push --force ...` and `... push origin main`
  are refused by the existing denies (`git -C * push*` and its narrower
  siblings). If the
  cleanup is factored into a testable function (as `loop-native-lead`
  allows), it gains a case for a done item's worktree in a clone. Each
  machine's owner re-runs the
  install script after the merge (LOOP-05); a missing clone is reported,
  not made. Not blocking: nothing host-specific changes in behaviour, and
  the Hyper-V preparation runs the same script from the same checkout.
- **First live use:** a16 (a).

## Implementation plan

When the design review is recorded, the attended session seeds the `human`
step `x2-predecessors-merged` (phase 2), which `x2-implement` needs; an
attended session marks it done once `loop-parallel-rule-wording` and
`loop-native-lead` are both done with their PRs merged, after checking
that their merged text of each overlapping paragraph equals the approved
text quoted under **Rule changes**, and sends the item back for a review
of the delta only if it differs (a15). Then, on `feat/loop-repository`:

1. `docs/development/loop-profile.yaml` and its parser,
   `tools/workflow/check/profile.go`, with tests 1 (LOOP-01, PROFILE-01).
2. `config.yaml`'s `repositories` in `queue.Config`, with its validation,
   an absent entry failing for the queue's own repository as for any
   other, and an entry with no `trunk` failing; every command takes a
   repository's trunk from its entry only (tests 1, 8; GLOBAL-10,
   GLOBAL-11).
3. `work` in `queue.Feature`, `wf declare` writing it; FLOW-02's text in
   `process.md` amended (`merge_first`, prefixed `areas`, the profile's
   keys marked as this repository's, the conservative-derivation paragraph
   and the last paragraph neutral, the `areas` bullet and the last
   paragraph as the combined text over `loop-parallel-rule-wording`'s) and
   the template's `merge_first` line and `areas` comment
   (`TEMPLATE.md:11-15`, prefix form) (test 14; FLOW-02 text and template,
   FLOW-03).
4. `check.Env.Checkouts`, `-checkouts`, and the checker per repository
   (Decision 5): every `origin/main` of Problem point 3 becomes the
   repository's trunk (tests 4, 6, 15).
5. `Front.Problems` from the profile file, with `merge_first` and prefixed
   areas; `kindOf` and `workflowRunsRe` from the merge base's profile file,
   the profile file's own path always a rule path (tests 2, 3, 8, 12;
   FLOW-02, FLOW-14).
6. The merge confirmations per `work` entry and `first` (test 5; FLOW-08).
6a. `tier2Report` reads the report JSON's `heads` and passes only when
    they equal the item's current head in every repository, and
    `wf-review` writes `heads` into the JSON (a12; test 4; LINT-04).
7. The tracked test from `config.yaml`, and the process-home check on
   `repos[0]` through `homedHere` (test 7; FLOW-03).
8. `repos/<owner>/<name>/` state, `wf main -repository`, `wf areas -check
   -repository`, `wf stale` per repository (tests 6, 13; FLOW-05, FLOW-10).
9. `hosts.yaml`'s `repositories`, the gate's fingerprint, a host step's
   `repo`, and `validateNative`'s check on `loop-native-lead`'s host table:
   a host step or a lead fails only when no machine serving its host name
   lists its repositories, the error naming all of them; `wf gate` offers
   a led item's steps only to a machine that serves the lead and lists
   every repository in the item's `repos`; `wf lead -host`, `wf new
   -lead` and `wf declare` refuse to create such a lead; FLOW-04's
   paragraph on taking steps amended (test 9; FLOW-04, FLOW-09).
10. `native-tick.sh`, `install-macos.sh`, `install-windows.ps1` on
    `loop-native-lead`'s wrapper: clones reset, `native.prepare` read from
    the profile file, the install checks; `native-push.sh`'s optional
    `<owner>/<repo>` argument and its checks, worktrees at
    `.scratch/<owner>/<repo>/<name>` made with `git -C
    .dev/checkouts/<owner>/<repo> worktree add`, the two narrow allows
    `Bash(git -C .dev/checkouts/* fetch*)` and `Bash(git -C
    .dev/checkouts/* worktree*)` in
    `tools/workflow/native/settings.native.json` with their
    `TestNativeSettingsRules` cases, the cleanup's loop over
    each clone,
    and the six phase agents' and `workflow-native`'s push sentence (a14,
    **The native side**; the `native_test.go` cases under Validation plan,
    Native; LOOP-05, FLOW-16, FLOW-17).
    `install-windows.ps1:65`'s `refresh-cache.sh --check` read from the
    profile file's `native.check` (`native.check.windows-hyperv:
    [[tools/relay-vm/hyperv/refresh-cache.sh, --check]]`).
11. Under a13 (b): `wf rules`; moved into `tools/workflow/check/` from
    `tools/tier1/rules/rules.go` (the rule index and the rule-to-check map
    parse, read from the profile's `rules.index`), from
    `tools/tier1/diff/diff.go` (the Tier1-Allow trailer parsing) and from
    `tools/tier1/coupling/frontmatter.go` (the front-matter check, FLOW-12's
    pre-standard rule and `design_tolerate`), tier 1 keeping thin wrappers
    that call them; `rules.yaml`'s LINT-05 owner updated (tests 11, 12;
    LINT-01, LINT-05, FLOW-02, FLOW-12).
12. The skills and agents: `workflow-tick` (clones, `-checkouts`, each
    repository's trunk CI and ruleset read with `gh api
    repos/<owner>/<name>/...` and recorded with `wf main -repository`, the
    profile's `areas.edges` command piped into `wf areas -check
    -repository`, merge confirmation per repository, Close per repository,
    digest heads),
    `workflow-native`, `workflow-intake` (repositories need an entry),
    `wf-publish` (a PR per repository), `wf-implement` and `wf-review` (the
    profile's fast gate; `wf-review` also the report's `heads`, task 6a)
    (FLOW-14: cite, not restate).
13. The rule text: FLOW-02, -03, -04, -08, -10, -16, -17, LOOP-01, LOOP-08
    and PROFILE-01 as approved, the overlapping paragraphs as the combined
    text over the predecessors' merged wording (FLOW-14, a15).
14. Test 10 on the live `workflow` state as well as its fixture; archive
    the design with its summary, and open the PR (FLOW-12). The PR body
    lists task 14a under "Before the merge" and task 14b under "Right
    after the merge".
14a. In the attended sitting that merges the PR, before the merge and
    before the next run, on Jeff's word: write `config.yaml`
    `repositories` on the `workflow` branch, with `buildwarden/buildwarden`
    (`trunk: main`) and `buildwarden/buildwarden-specs` (`trunk: main`),
    no `profile` in either (Decision 3), and document
    `repositories`, `work`, `repos/`, `hosts.yaml`'s `repositories` and the
    profile file's format in the `workflow` branch's `README.md`. The old
    `wf` ignores the new key. The PR is not listed ready for merge until
    these edits are drafted.
14b. In the same sitting, on Jeff's word, immediately after the merge and
    before the next run, cloud or native, as the post-merge state step:
    add `profile: docs/development/loop-profile.yaml` to
    `buildwarden/buildwarden`'s entry in `config.yaml` on the `workflow`
    branch, and run `wf validate` with the merged `wf`. The sitting merges
    when no run is in progress, so no run meets the window in which this
    repository reads as not adopted (Decision 3). Branches open here that
    forked before the merge fail the clearance until they next merge
    `main` (the cost Jeff accepted, Defaults and overrides).

After that, in an attended session and outside the PR, only later
repositories' entries are added, and a repository's `profile` once the
pull request that adopts it has merged, such as the relay's (Decision 3).

## Design review

- **r1, 2026-11-17: approved at `61f6fbe`** (Jeff, workflow-human, FLOW-19),
  read at `b6fa193` (the draft `dfbdcc1`, with three report-only check
  rounds and a reconciliation with the two predecessors as approved,
  applied before the review: `95aeff8`, `38435c2`, `74f7651`, `a6bcefd`,
  `85bc432`, `b6fa193`). Blocking q1 answered **(C)**: a private
  repository's work is published as a public one's from its first item.
  Assumptions a1 to a17 confirmed as recommended (a13 (b)). Choices the
  reconciliation surfaced, decided: trunk only in `config.yaml`; the three
  native additions kept (repository-qualified host satisfaction in
  FLOW-04, the lead and declare refusals, `native-push.sh <name>
  [<owner>/<repo>]`); an adopting repository's `profile` added only after
  its adoption merges, this repository's included (task 14b, right after
  this item's merge, before the next run); two narrow native allows
  (`git -C .dev/checkouts/* fetch*`, `... worktree*`) in place of a broad
  one; the one-time clearance cost on branches open at an adoption
  accepted. Rule changes: **rule change FLOW-02 approved at `61f6fbe`**; **rule change FLOW-03 approved at `61f6fbe`**; **rule change FLOW-04 approved at `61f6fbe`**; **rule change FLOW-08 approved at `61f6fbe`**; **rule change FLOW-10 approved at `61f6fbe`**; **rule change FLOW-16 approved at `61f6fbe`**; **rule change FLOW-17 approved at `61f6fbe`**; **rule change LOOP-01 approved at `61f6fbe`**; **rule change LOOP-08 approved at `61f6fbe`**; **rule change PROFILE-01 approved at `61f6fbe`**; the enforcement-only changes to FLOW-12,
  FLOW-14, LINT-01, LINT-04 and LINT-05 (and LINT-05's owner line in
  `tools/tier1/rules.yaml`) approved with them. Folded in at `07718d5`
  and `61f6fbe`: the home's `profile` after the merge, the narrow allows, the
  accepted cost. Implement waits for `x2-predecessors-merged` (a15); at
  that step the merged text of each overlapping paragraph is compared
  with the approved text quoted under **Rule changes**, and a delta
  review follows only if it differs.

## Consolidation

Filled in at Publish.

## Rollback notes

None.

## Archive summary

Filled in at Publish.
