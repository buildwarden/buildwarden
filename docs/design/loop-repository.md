---
feature: loop-repository
status: draft
drivers: []              # the loop's tools, skills and rules; no driver tree is reached
seams: []                # no GLOBAL-08 seam changes; the Loop Contract is seam 10 of the
                         # umbrella, a document, and the profile file is part of it (LOOP-01)
surfaces: []             # `wf` is the loop's tool, not a user-facing surface of the product
repos: []                # works only in buildwarden/buildwarden; the relay adopts in its own item
areas: [loop, tier1, rules, designs]
security_boundary: none
alters_relationships: false  # no product component's role or interaction changes (FLOW-02);
                         # this changes where the loop's gates run
design_review: required  # a process item's design always gets its review (FLOW-03, FLOW-19)
native_validation: []    # the native wrapper's change is tested by its Go test and on each
                         # machine by its owner at re-install; see Validation plan
tier2_gaps: []
rules: [FLOW-02, FLOW-03, FLOW-04, FLOW-05, FLOW-08, FLOW-09, FLOW-10, FLOW-12, FLOW-13,
        FLOW-14, FLOW-16, FLOW-17, FLOW-18, FLOW-19, FLOW-20, LOOP-01, LOOP-02, LOOP-03,
        LOOP-04, LOOP-05, LOOP-06, LOOP-07, LOOP-08, LOOP-09, PROFILE-01, LINT-01, LINT-02,
        LINT-03, LINT-04, LINT-05, GLOBAL-09, GLOBAL-10, GLOBAL-11, TEST-01]
rule_changes:
  - {rule: FLOW-03, change: amend, paths: [docs/development/process.md,
      tools/workflow/check/check.go, tools/workflow/check/check_test.go]}
  - {rule: FLOW-04, change: amend, paths: [docs/development/process.md]}
  - {rule: FLOW-08, change: amend, paths: [docs/development/process.md,
      tools/workflow/check/check.go, tools/workflow/check/check_test.go]}
  - {rule: FLOW-16, change: amend, paths: [docs/development/process.md]}
  - {rule: FLOW-17, change: amend, paths: [docs/development/process.md]}
  - {rule: FLOW-02, change: amend, paths: [tools/workflow/check/check.go,
      tools/workflow/check/check_test.go]}            # enforcement only: no text change
  - {rule: FLOW-14, change: amend, paths: [tools/workflow/check/rulechange.go,
      tools/workflow/check/workflows.go, tools/workflow/check/rulechange_test.go]}
                                                      # enforcement only: no text change
  - {rule: LOOP-01, change: amend, paths: [docs/development/loop-contract.md,
      tools/workflow/check/profile.go, tools/workflow/check/profile_test.go]}
  - {rule: LOOP-08, change: amend, paths: [docs/development/loop-contract.md]}
  - {rule: PROFILE-01, change: amend, paths: [docs/development/loop-profile.md,
      docs/development/loop-profile.yaml]}
  - {rule: LINT-01, change: amend, paths: [tools/tier1/rules/rules.go,
      tools/tier1/rules/rules_test.go, tools/workflow/check/ruleindex.go,
      tools/workflow/check/ruleindex_test.go]}        # a13 (b): code moves, no text change
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
merged, and `loop-native-lead`, queued ahead of it and not yet designed. The
move of the loop to its own repository is P1's last item and comes after
it (`loop-contract` a19). Under `loop-contract` a22, answered (1), wave 2
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
   `Feature.PR` (`queue.go:97`, `:117`, `:144-151`) are the home's. A work
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
   | Sessions | `tools/workflow/flow18-check.sh:13` (`buildwarden/buildwarden-sessions`) |
   | Component | `install-macos.sh:93-94` and `install-windows.ps1:115` (`origin` must be `buildwarden/buildwarden`) |
   | Native assets | `native-tick.sh:223-240` runs `tools/relay-vm/hyperv/refresh-cache.sh`, a product script inside loop machinery |

   The relay has no `make lint`, no `docs/book/`, no drivers and no
   `security_boundary` values of GLOBAL-06's. Read through LOOP-01, every
   one of these is its profile's entry; `wf` cannot read a profile.
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
   2026-11-14 the relay repository is private until its M1's scan
   (`loop-contract` a21). What of a private repository's work may be written
   to the public state is open: the contract leaves it to this item "with
   the relay design's a17" (`loop-contract`, hand-off point 5).
8. **Tier 1's generic checks are in Go analyzers' company.** `rule-index`,
   `design-front-matter` and `suppression` enforce loop rules (LINT-01,
   FLOW-02, LINT-05) and are what the relay needs for LOOP-03's Process
   class; they live in `tools/tier1/`, beside the Go analyzers the relay
   cannot use (`loop-contract`, hand-off point 5). `design-front-matter`
   already calls `wf`'s own `check.Front.Problems`
   (`tools/tier1/coupling/frontmatter.go:12`).

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
  (its Decision 18 left it here); a `human` merge-recording step not offered
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
   needs: trunk, branch pattern, designs, fast gate, impact vocabulary,
   reference documents, rule and machinery paths, the edges command, the
   sessions remote and native preparation. This repository's is
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
   the change (FLOW-14). *Rejected:* reading it from trunk. It would be
   safer on paper, but a branch can already change `make lint` by editing
   the `Makefile`, and a change that adds a gate would be checked with the
   old one.
3. **The queue state names every repository an item may name.**
   `config.yaml` gains `repositories`, one entry per repository: its name,
   and either the path of its profile file (adopted) or its trunk (not
   adopted). `repository` stays and still names the queue's own. An item
   that names a repository with no entry fails `wf validate`. Entries are
   added in an attended session on Jeff's word, as the area map is
   (FLOW-05). *Rejected:* reading the trunk from the remote's default
   branch: a value the loop did not write down, used when nothing is given
   (GLOBAL-10).
4. **An item records a branch and a pull request per repository** (a3).
   `branch` and `pr` stay the home's. A new list, `work`, holds one entry
   per other repository the item names: `repo`, `branch`, `pr` (number, CI,
   head) and, after its merge, `merged`. The branch has the home's name in
   every repository (a4). An item recorded before this lands has no `work`
   and keeps today's behaviour (Defaults and overrides).
5. **The gates run in each repository the item has a branch in**, each
   through its own profile; a repository that has not adopted gets the
   branch, trunk and pull-request checks and no fast gate, because it has
   no profile to name one (LOOP-08's last paragraphs). The checks are
   listed under **The gates, per repository**.
6. **The loop sees every merge it records** (a5). The `human` step that
   records a merge the loop cannot see (FLOW-04, FLOW-08) is retired for
   any repository with an entry in `config.yaml`, which is every
   repository an item may name (Decision 3). The human merges the item's
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
10. **A hold still names workers** (a9). `loop-parallel`'s Decision 18 is
    kept: no hold names a repository.
11. **This item holds no worker** (a10). Every change is additive or reads
    an absent field as today.
12. **`wf` reaches second repositories through checkouts the session
    names** (a11): `-checkouts DIR`, holding one clone per repository at
    `DIR/<owner>/<name>`. The skills clone them into `.dev/checkouts/`,
    which is ignored (`.gitignore:12`) and inside the session's working
    directory, so the agents' permissions cover it. Sessions start in this
    repository's checkout, where `wf`, the skills and the agents are, until
    the move.
13. **One tier-2 report per branch state** (a12), named by the home's head as
    today, its JSON recording the head of every repository it read.
14. **The generic checks move into `wf`** (a13): `rule-index` and
    `suppression` join `design-front-matter`'s code in
    `tools/workflow/check/`, run as `wf rules`, reading the profile for the
    rule paths. Tier 1 keeps calling them through its existing `replace`,
    so this repository's CI does not change; a component's CI runs `wf
    rules` built from the loop's trunk.
15. **The native side keeps one dedicated checkout per repository a machine
    serves** (a14), cloned by the machine's owner at install (LOOP-05), each
    reset to its trunk at the start of every run and fingerprinted by the
    gate. The product script the wrapper runs moves into this repository's
    profile file as native preparation for `windows-hyperv`.
16. **Order with the other P1 items** (a15). This design is reviewed now;
    its Implement waits for `loop-native-lead`'s merge, whose wrapper the
    native part builds on, and for `loop-parallel-rule-wording`'s, which
    also amends FLOW-03 and FLOW-16.
17. **What proves the item is not its done-when** (a16). This item is done
    when its PR merges (FLOW-03, process items). The wave-1 gate is met
    later, when the relay's first item and an item here complete from the
    queue, after the relay adopts.

## Questions

### Blocking

- **q1. A private repository's work in the public queue.** (The relay
  design's a17; `loop-contract`, hand-off point 5.) The `workflow` branch
  is in a public repository: `QUEUE.md`, the digests, the tier-2 reports
  (`reports/<item>/<sha12>.{md,json}`, LINT-04) and every queue file's
  `source`, history, questions and step notes are public. An item in the
  relay would put there its design's title and questions, its branch heads,
  its pull request numbers, its CI results, and a tier-2 report that quotes
  its diff. Its design itself is in the relay repository. As read on
  2026-11-14 the relay is private until its M1's scan (`loop-contract`
  a21), and it goes public at M1 (umbrella Decision 16). This session could
  not read it on 2026-11-17 (its GitHub access does not include it), so its
  visibility today is not confirmed here. Only Jeff can decide what of a
  private repository may be published, and the answer decides whether this
  design needs a second, private store for reports.
  - **(A)** The loop works only in public repositories. A private
    repository may be named by a tracked item, whose title intake writes
    with Jeff; no gate, report or digest line beyond that. The relay's
    first item from the queue is one taken after it goes public at M1; its
    M1a (which makes it public) stays by hand as its a17 plans. `wf
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

  Recommended: **(A)**. It needs no new store, publishes nothing Jeff has
  not chosen to, and costs only the order of the relay's work, which M1
  already puts first.

### Assumed

- **a1. Where `wf` reads a component's profile.** Proceeding with (a);
  confirm.
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
- **a2. The branch's profile file or trunk's.** Proceeding with (a);
  confirm.
  - (a) The branch's, as LOOP-09 has a component's checks read, guarded
    by the rule-change clearance: the file is under a rule path, so a
    branch that changes it unapproved fails Implement, Review and Publish
    (FLOW-14).
  - (b) Trunk's. A gate the change itself adds is then first run after the
    merge.

  Recommended: (a). The `Makefile` is read from the branch today.
- **a3. How the queue records a second repository's branch and pull
  request.** Proceeding with (a); confirm.
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
- **a4. A second repository's branch name.** Proceeding with (a); confirm.
  - (a) The home's branch name, in every repository (FLOW-17 as amended
    below). The profile's branch pattern is read from the home's profile.
  - (b) Each repository's own pattern from its profile.

  Recommended: (a). One name is what a reader searches for, and no profile
  so far has another pattern.
- **a5. The order of an item's merges.** Proceeding with (a); confirm.
  - (a) Each pull request is listed ready on its own (FLOW-08); the Merge
    phase is done when the loop has seen every one merge. A design whose
    home pull request needs another's merge first (a release to pin, as
    `seam-specs` did) says so, and the step it adds for the release
    `needs` that merge: `wf` seeds a confirmation step per `work` entry,
    `f6-merged-<n>` in Publish for one the design marks `first`, and
    `f7-merge` for the rest, all done by the tick's confirmation and never
    offered (FLOW-05).
  - (b) `wf` enforces the home last: the home's pull request is opened
    only after every other has merged. Simple, and wrong for an item whose
    other pull requests need nothing first.
  - (c) As `seam-specs`: a `human` step records each merge in a second
    repository. The retired clause stays in FLOW-04 and FLOW-08.

  Recommended: (a).
- **a6. Items homed in a second repository.** Proceeding with (a);
  confirm.
  - (a) An adopted home (a `profile` in its `config.yaml` entry) gets the
    feature round, its gates and FLOW-05's limits; a home that has not
    adopted stays tracked, as `loop-parallel` built it. A process item is
    homed in the queue's repository (`validateKind`, `queue.go:850-858`,
    unchanged).
  - (b) The tracked round is retired: an item is homed only in an adopted
    repository, and work in one that has not adopted is a second
    repository of an item homed elsewhere.

  Recommended: (a). (b) would make the specs repository unable to home a
  change of its own until it adopts, which Jeff has not filed.
- **a7. Where a second repository's state lives.** Proceeding with (a);
  confirm.
  - (a) `repos/<owner>/<name>/areas.yaml` and `main.yaml`, added beside the
    root's `areas.yaml` and `main.yaml`, which stay the queue
    repository's.
  - (b) Every repository's under `repos/`, the root's two files moved by
    `wf migrate`. One layout, and a run that started before the merge with
    the old `wf` then writes a root `main.yaml` nobody reads (LOOP-09: a
    run keeps its tools until it ends).

  Recommended: (a); the move may make (b) when it rewrites the state.
- **a8. Which repository a bare area name means.** Proceeding with (a);
  confirm.
  - (a) The item's home's. Areas of another repository are
    `<owner>/<name>:<area>`.
  - (b) The queue's repository's, whatever the home.

  Recommended: (a). A relay item's design then names its own areas bare,
  and every queue file today is homed here, so nothing changes meaning.
- **a9. A hold that names repositories** (`loop-parallel` Decision 18 left
  it here). Proceeding with (a); confirm.
  - (a) Not added. A hold names workers and covers work in every
    repository (FLOW-05, `process.md:225-230`, unchanged).
  - (b) `holds` entries may be `<affinity>@<owner>/<name>`, holding that
    worker's steps in that repository only. No item needs it yet, and it
    is the sentence `loop-parallel` rejected for want of a case.

  Recommended: (a).
- **a10. This item's holds.** The queue file's `source` once said "Holds
  cloud and host:* while in progress"; the holds were removed on
  2026-11-14 and left to this design. Proceeding with (a); confirm.
  - (a) None. Every field is additive and reads as today when absent
    (Decisions 4 and 8): `work` absent is no second branch; a
    `repositories` entry absent for the queue's repository is read from
    `repository` and today's paths (Defaults and overrides); the root's
    `areas.yaml` and `main.yaml` stay. A run that started before the merge
    keeps its tools and touches none of the new files.
  - (b) `holds: [cloud, host:*]` from `x3-merge` on. A hold lasts while the
    item is in progress, so in practice from its claim: days of S1-, K1-
    and fix work stopped for nothing that breaks.

  Recommended: (a).
- **a11. How `wf` finds a second repository's checkout.** Proceeding with
  (a); confirm.
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
- **a12. The tier-2 report for an item with several branches.** Proceeding
  with (a); confirm.
  - (a) One report per state of the item's branches, at
    `reports/<item>/<home sha12>.{md,json}` as today; the JSON gains
    `heads`, the head of each repository the report read. The Review
    check passes when the report for the home's head exists and its
    `heads` equal the item's current heads.
  - (b) One report per repository's head. Two reports for one review of
    one change.

  Recommended: (a).
- **a13. Where tier 1's generic checks live.** Proceeding with (b);
  confirm.
  - (a) They stay in `tools/tier1/`; a component's CI builds this
    repository's `tier1` and runs only those three. It carries the Go
    analyzers and golangci-lint with it.
  - (b) `rule-index` and `suppression` move into `tools/workflow/check/`
    beside `Front.Problems`, which `design-front-matter` already calls; `wf
    rules [-base REV]` runs the three on a checkout, with the rule paths,
    prefixes and template from its profile file. Tier 1 calls the same
    functions through its `replace`, so its report and this repository's
    CI are unchanged. A component's CI runs `go run
    github.com/buildwarden/buildwarden/tools/workflow/cmd/wf@main rules`,
    which follows the contract's trunk as LOOP-08's link does; the move
    changes that path once.
  - (c) Each component writes its own. Three implementations of one rule.
  - (d) Left to the move.

  Recommended: (b).
- **a14. The native side.** Proceeding with (a); confirm.
  - (a) `hosts.yaml` gains, per machine, `repositories`: those whose host
    steps it takes (default: the queue's own only, which is today). The
    owner clones each one once, beside the dedicated checkout, at
    `.dev/checkouts/<owner>/<name>`, and installs its toolchain; the install
    scripts check each clone's `origin` and cleanliness and run the
    profile file's `toolchain_check` command, and never clone or install
    (LOOP-05: set up by the owner). Each run resets every clone to its
    trunk, the gate's fingerprint covers each repository's trunk and
    in-progress branches, and `wf gate` offers a host step only on a
    machine that lists its repository. `refresh-cache.sh` moves from
    `native-tick.sh` into this repository's profile file, as `native:
    prepare:` for `windows-hyperv`, which the wrapper runs from the
    repository's checkout.
  - (b) Host steps in a second repository wait for the move.
  - (c) The wrapper clones a second repository on demand when a host step
    there is ready. An agent then sets up a machine (LOOP-05).

  Recommended: (a). It is written against `native-tick.sh` at `0cd3c02`;
  `loop-native-lead` changes the wrapper first, and this part is re-derived
  on its wrapper at Implement (a15).
- **a15. Order with `loop-native-lead` and `loop-parallel-rule-wording`.**
  Both are process items queued ahead of this one (priorities 62 and 63,
  this one 64), neither designed yet. `loop-native-lead` rewrites the
  native wrapper; `loop-parallel-rule-wording` amends FLOW-03 (phases 7
  and 8) and FLOW-16 (the merge proof). Proceeding with (a); confirm.
  - (a) This design is reviewed now. Its `x2-implement` starts after both
    have merged; the rule wording below is re-based on theirs and the
    native part on `loop-native-lead`'s wrapper, and if either changes a
    sentence quoted here, the design comes back for a review of that
    change only (FLOW-19).
  - (b) The review waits for both to merge.

  Recommended: (a). The questions here do not depend on theirs.
- **a16. What shows the queue works across repositories.** Proceeding with
  (a); confirm.
  - (a) This item is done at its merge, like any process item (FLOW-03).
    The first live use is an item that names `buildwarden/buildwarden-specs`
    and is declared after the merge (such as
    `relay-collector-failure-fails-open` if it is re-declared, or the next
    one intake files). The wave-1 gate (`loop-contract` a22) is met when a
    relay item and an item here have each completed from the queue, which
    needs the relay's adoption (its M1b task 10), its `config.yaml` entry,
    the workers' access to it, and q1's answer. Jeff judges the gate; no
    `wf` check does.
  - (b) This item's Close waits for a relay item to complete.

  Recommended: (a). (b) would keep a process item in progress for weeks,
  and a process item in progress ranks ahead of every new feature
  (FLOW-05).
- **a17. Which front-matter keys are the loop's and which a profile's.**
  `Front.Problems` requires twelve keys (`check.go:670-672`). LOOP-08 says
  no loop rule may require that a component has drivers, user-facing
  surfaces or security boundaries. Proceeding with (a); confirm.
  - (a) `wf` requires the loop's keys (`feature`, `status`, `seams`,
    `alters_relationships`, `design_review`, `native_validation`, `rules`,
    `concepts`) and the keys the profile file adds, with the values it
    gives. This repository's file adds `drivers`, `surfaces`,
    `security_boundary` (GLOBAL-06's four values) and `tier2_gaps`
    (TEST-03), so nothing changes here. FLOW-02's text is not changed: it
    is read through LOOP-01 in another component, and is made neutral when
    next amended (LOOP-08).
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
trunk: main
branch: feat/{item}
designs:
  dir: docs/design
  template: docs/design/TEMPLATE.md
fast_gate:                    # run in the checkout, each must exit 0 (FLOW-03, FLOW-17)
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
areas:
  edges: [tools/tier1/area-edges.sh]
sessions:
  remote: buildwarden/buildwarden-sessions
toolchain_check: [go, version]
native:
  prepare:
    windows-hyperv: [[tools/relay-vm/hyperv/refresh-cache.sh]]
```

A key absent from the file is a term the component fills as `none`
(LOOP-01): no fast gate, no reference documents, no machinery. `wf` reads a
key it requires and finds absent as `none`, never as this repository's
value. A file `wf` cannot parse, or a `version` it does not know, fails
every check that reads it, naming the file.

The relay writes its own at adoption, under its own path, as part of
LOOP-08's step 2 (amended below). Its values are its own: a `cargo` gate,
no `drivers`, its own `security_boundary` set if it keeps one.

### `config.yaml`

```yaml
repository: buildwarden/buildwarden      # the queue's own, unchanged
repositories:
  - name: buildwarden/buildwarden
    profile: docs/development/loop-profile.yaml
  - name: buildwarden/buildwarden-specs  # not adopted: no profile, its trunk given
    trunk: main
  # - name: buildwarden/buildwarden-relay
  #   profile: <its path>                # added at its adoption, on Jeff's word
  #   private: true                      # q1 (A)
```

`wf validate` fails an item that names a repository with no entry, an entry
with both `profile` and `trunk` or neither, and (under q1 (A)) an item that
is not tracked and names a `private` repository. An entry absent for the
queue's own repository is read as `profile:
docs/development/loop-profile.yaml`, with a `wf validate` notice, until an
attended session writes it (Defaults and overrides).

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
the design's front-matter key `merge_first` (a list of repositories). It
refuses to drop an entry whose branch exists. `wf validate` checks that
`work` and `repos` agree once declared. An item declared before this item
merges has no `work`; it keeps today's behaviour until it is declared again
(Defaults and overrides).

### The gates, per repository

`check.Env` gains `Checkouts`, the directory of `-checkouts`, and the
checker runs each per-repository check against the clone of the repository
it concerns, with that repository's trunk (`origin/<trunk>`) and profile
file read from the branch there (a2). Per phase (FLOW-03), at `0cd3c02`'s
line numbers:

| Phase | Today (`check.go`) | With `work` |
|---|---|---|
| 0 Branch setup | `branchOnRemote` (`:245-248`) | in each repository |
| 1 Design | `design` (`:570-606`): front-matter, sections | home only; keys and values from the home's profile file (a17) |
| 2 Plan | `plan`, `reviewed`, `approvalDelta`, `declaredStill` | home only; `impactDelta` also compares `work`'s repositories |
| 3 Implement | `onBranchHead`, `make lint`, `make test`, `docsTouched("docs/")`, `ruleClearance`, `declaredStill`, `insideAreas` | each repository: clean branch head, its fast gate, its reference documents, its rule paths cleared against the home design's `rule_changes`, its areas from its map. No fast gate or reference documents where it has no profile |
| 4 Review | as 3, plus `tier2Report`, `trialMerge` | as 3 in each; one report whose `heads` cover every branch (a12); a trial merge onto each trunk |
| 5 Guide | `docsTouched(guides)` | each adopted repository's guides |
| 6 Publish | `mainMerged`, consolidation names `origin/main`, `ruleClearance`, `archivedOn`, `prGreen` | trunk merged and named in **Consolidation** for each repository; archived in the home; a pull request green on its pushed head in each; `f6-merged-<n>` confirmed for each `first` entry |
| 7 Merge | `merged` (`:348-430`) | each pull request merged, by the same proofs, against its own trunk |
| 8 Close | `closed` | branches gone in each repository (FLOW-16) |

A repository whose branch has no commit beyond its trunk at Publish gets no
pull request; its entry is closed with the home's. `wf stale` seeds a
refresh when any of the item's branches does not contain its trunk, and the
refresh (FLOW-10) merges each. `wf main -repository <owner>/<name>` records
a trunk in `repos/<owner>/<name>/main.yaml`; without the flag it records
the queue repository's in `main.yaml`, as today. `wf areas -check
-repository ...` compares that repository's map with the edges its
profile's command prints.

`kindOf` (`rulechange.go:37-46`) and `workflowRunsRe` (`workflows.go:17-18`)
read `rules.paths`, `rules.machinery` and `rules.rule_jobs` from the
profile file of the repository whose path they classify. A process item's
branch stays exempt from the clearance (FLOW-14), and a process item is
homed here (a6), so a change to the loop's machinery is still a process
item's.

### Branches, pull requests and the skills

- `wf-publish` opens a pull request in each repository with commits
  (`gh pr create -R <owner>/<name>`), records it in the `work` entry and
  repairs its CI as today (FLOW-11). It lists in each pull request's body
  the tier-2 MUST findings in that repository and the item's other pull
  requests (FLOW-08).
- The tick's merge confirmation (`workflow-tick` step 2) runs `wf done` on
  `f6-merged-<n>` and `f7-merge`; `wf` proves each merge in its clone.
  Close (`workflow-tick` step 4, the inline Close) deletes the local branch
  in each clone and escalates a remote branch that did not delete, per
  repository (FLOW-16).
- `workflow-tick` step 1 and `workflow-native` clone or fetch each
  repository named by an in-progress item into `.dev/checkouts/` (cloud),
  or check the owner's clones (native), and pass `-checkouts .dev/checkouts`
  to every `wf` command that takes `-repo`. A clone that cannot be fetched
  (no access) stops that item's steps with a `blocked` line naming the
  repository; the run goes on with other items.
- `wf-implement`, `wf-review` and `wf-publish` run the fast gate the
  profile file names, in the clone the step concerns, and push there
  (FLOW-17). Each cites the profile in place of `make lint` and `make
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

`wf gate` offers a host step only to a machine that lists the step's
repository (the item's home, or the `work` entry the step's `repo` names;
a host step gains an optional `repo`). The fingerprint
(`queue/hosts.go:257-279`) hashes, for each listed repository, its trunk and
the in-progress items' branches there. `native-tick.sh` resets each clone
to its trunk, as it does the dedicated checkout at `:190`, and leaves each
on its trunk at the end (`:264-265`). It runs the `native.prepare` command
for each host name the machine serves from the repository whose profile
names it, in that repository's clone. The install scripts gain the check
of each clone (its `origin` is the listed repository; it is clean) and run
the profile's `toolchain_check`, and report a missing clone with the
`git clone` command for the owner to run.

### What Jeff sets up

None of this is done by an agent (LOOP-05, FLOW-08, umbrella a11):

- The `repositories` entries in `config.yaml`, in an attended session,
  when a repository is first named and at the relay's adoption.
- The workers' access to each repository: the cloud environment's GitHub
  access and toolchain (this session's GitHub access did not include
  `buildwarden/buildwarden-relay` on 2026-11-17), and each native
  machine's clone and toolchain.
- Each repository's settings: head-branch deletion on merge (FLOW-16),
  trunk protection where the host offers it (FLOW-08, LOOP-08 step 6). As
  read on 2026-11-17, `buildwarden/buildwarden-specs` is public and
  deletes head branches on merge.
- q1's answer, and the relay's visibility.

### What this item does not do

- No repository is created, and no repository setting is changed.
- No move: `wf`, the skills, the agents, the rules and the queue stay here
  (`loop-move`).
- No adoption of the contract by the relay or the specs repository. The
  relay adopts in its own item (its M1b task 10).
- No change to FLOW-05's relations or limits. An item homed in an adopted
  second repository counts in the limits as a feature here does (a6); the
  relations already read areas per repository.
- No hold that names a repository (a9).
- No second store for a private repository's reports, unless q1 is (B).

### Rule changes in brief

Amended text: FLOW-03, FLOW-04, FLOW-08, FLOW-16, FLOW-17, LOOP-01, LOOP-08,
PROFILE-01. Enforcement only, no text change: FLOW-02 (`Front.Problems`
reads the profile's keys), FLOW-14 (`kindOf` and `workflowRunsRe` read the
profile), LINT-01 and LINT-05's checks moved into `wf` (a13). The wording
is under **Rule changes**.

## Defaults and overrides

| Default | Why it is the obvious one (GLOBAL-09) | Override, used exclusively (GLOBAL-10) |
|---|---|---|
| `work` absent: the item has no branch in a second repository, and its gates run in the home only | It is what every queue file means today | `wf declare` writes `work` from the design's `repos` |
| A `config.yaml` entry absent for the queue's own repository: its profile file is `docs/development/loop-profile.yaml` | It is the file this item adds; the queue's repository is always adopted | The entry, once written |
| A key absent from a profile file: the term is `none` | LOOP-01: a term the component does not have is `none` | The key |
| A bare area name: an area of the item's home (a8) | Every queue file today is homed here; a relay design names its own areas bare | `<owner>/<name>:<area>` |
| `hosts.yaml` `repositories` absent: the queue's own only | It is what every machine serves today | The list |
| The branch name in a second repository: the home's (a4) | One name per item | None |
| `wf main` and `wf areas -check` without `-repository`: the queue's repository | Today's behaviour | `-repository` |

No fallback: a missing clone, an unreadable profile file or a repository
with no entry fails and names what to fix (GLOBAL-11). The one transition
named here: an item declared before the merge with `repos` beyond its home
keeps the `human` merge-recording step it may already have until its Close;
it is not converted (FLOW-04's old clause binds its branch until it merges
`main`, LOOP-09).

## Cross-driver impact

None. No driver tree is reached; `tools/relay-vm/hyperv/refresh-cache.sh`
is named in the profile file and not changed.

## Rule changes

Each entry quotes the current text at `0cd3c02` and gives the proposed
text in full for each amended paragraph. Severities and check types do not
change. Under a15 the FLOW-03 and FLOW-16 paragraphs are re-based on
`loop-parallel-rule-wording`'s wording if it merges first, and come back
for review only if a sentence quoted here changed.

### FLOW-03 (amend), `docs/development/process.md`

**Why:** the done-when checks hold in each repository an item has a branch
in, a home that has adopted gets real phases, and row 3 names this
repository's commands (LOOP-08: neutral when next changed).

**Current wording,** row 3 of the feature table:

> | 3 | Implement | Tasks done; `make lint` and `make test` green; reference docs updated in the same change (UX-01); every edit to the rules or their enforcement cleared by an approved rule change (FLOW-14). |

**Proposed:**

> | 3 | Implement | Tasks done; the fast gate green (LOOP-01; here `make lint` and `make test`); reference docs updated in the same change (UX-01); every edit to the rules or their enforcement cleared by an approved rule change (FLOW-14). |

**Current wording,** the paragraph after the table:

> A transition happens only when the phase's done-when checks pass mechanically
> (files present, sections filled, commands green), never on an agent's own report
> that it is done. From Plan on, each done-when also checks that the queue
> still records what the design declares; from Implement on, that every
> path the branch changes lies in an area the design declares (FLOW-02). A
> path outside them is settled by widening the declaration inside the
> approved design, or by a return to Design (FLOW-06).

**Proposed:**

> A transition happens only when the phase's done-when checks pass mechanically
> (files present, sections filled, commands green), never on an agent's own report
> that it is done. From Plan on, each done-when also checks that the queue
> still records what the design declares; from Implement on, that every
> path each of its branches changes lies in an area the design declares
> in that repository's map (FLOW-02). A
> path outside them is settled by widening the declaration inside the
> approved design, or by a return to Design (FLOW-06).
>
> An item has a branch in each repository it works in, under one name, and
> the queue records each branch and its pull request. Where a done-when
> names the branch, the fast gate, `main`, the reference documents or the
> PR, it holds in each of those repositories, read through that
> repository's profile (LOOP-01). In a repository that has not adopted the
> contract (LOOP-08) only the branch, trunk and pull-request checks run:
> there is no profile to name a gate. The design and its review are the
> home's.

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
per repository**; the tracked test in `check.go:46-52` reads the home's
`config.yaml` entry (no `profile`) in place of "not the queue's
repository" (`Tracked`, `queue.go:627-632`).

### FLOW-04 (amend), `docs/development/process.md`

**Why:** the loop sees every merge it records (Decision 6), so the
merge-recording step has no case left but a tracked item.

**Current wording:**

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

**Enforcement:** none; `wf` does not restrict what a `human` step is for.

### FLOW-08 (amend), `docs/development/process.md`

**Why:** an item may have pull requests in several repositories, each
merged by the human and each seen by the loop.

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
> first, and a step that needs one of those merges waits until the loop
> has seen it.

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

**Enforcement:** `merged` (`check.go:348-430`) runs per `work` entry
against its trunk; `wf stale` reads each trunk.

### FLOW-16 (amend), `docs/development/process.md`

**Why:** an item's branches are in several repositories, and the deletion
setting is the host's (LOOP-08: no CI system or host assumed).

**Current wording:**

> - its branch, on the remote and locally, and any worktree checked out on it.
>   GitHub deletes the remote branch on merge (the repository's "Automatically
>   delete head branches" setting); Close confirms it is gone, and escalates to a
>   `human` step with the exact command if it is not;

**Proposed:**

> - its branch in each repository it works in, on the remote and locally,
>   and any worktree checked out on it. The host deletes a remote branch on
>   merge where the repository is set to (on GitHub, "Automatically delete
>   head branches"); Close confirms each is gone, and escalates to a
>   `human` step with the exact command for each that is not;

**Enforcement:** `closed` per repository; the inline Close of
`workflow-tick`.

### FLOW-17 (amend), `docs/development/process.md`

**Why:** the pushes and the gate are per repository, and the commands are
the profile's.

**Current wording:**

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

**Enforcement:** none beyond the `pushed` check at phase 2 of a process
item and the remote heads the gates read.

### LOOP-01 (amend), `docs/development/loop-contract.md`

**Why:** `wf` reads a profile through a file (Decision 1), and the file has
to be the profile's, not a second source.

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
> leaves out is the term `none`.

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
fails on an unknown `version`.

### LOOP-08 (amend), `docs/development/loop-contract.md`

**Why:** adoption now includes the profile file, and the contract's list of
what it assumes must say so.

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
> loop's tools read, and a change to an entry it holds changes both in the
> same change.

**Current wording,** the Native hosts row:

> | Native hosts | The host names in the loop's `hosts.yaml`. All of them serve this repository's drivers today. |

**Proposed:**

> | Native hosts | The host names in the loop's `hosts.yaml`. All of them serve this repository's drivers today. Before a session on a machine that serves `windows-hyperv`, the native wrapper runs `tools/relay-vm/hyperv/refresh-cache.sh` (the profile file's `native.prepare`). |

**Enforcement:** none; the agent part as written.

### FLOW-02, FLOW-14, LINT-01 and LINT-05: enforcement only

No text changes. `Front.Problems` takes the profile's keys and values
(a17); `kindOf` and `workflowRunsRe` take the profile's paths and pattern;
under a13 (b), the rule index's parser (`tools/tier1/rules/rules.go`) and
the suppression check move into `tools/workflow/check/`, and tier 1 calls
them there. For this repository each check gives the same result on the
same input, which the tests below pin.

## Validation plan

- **Tier 0/1** (`make test`, `make lint`; TEST-01): `wf`'s tests build
  their repositories with `git init` as they do today
  (`check_test.go`, `cmd/wf/*_test.go`). New cases, with two local bare
  repositories as remotes:
  1. A profile file round-trips; an absent key is `none`; an unknown
     `version` fails naming the file.
  2. The front-matter check with this repository's profile file gives
     today's result for every archived design (`AllDesigns` over
     `docs/design/`, as tier 1's `design-front-matter` does now).
  3. `kindOf` and `workflowRunsRe` with this repository's profile file
     classify every path of the tree at `main` and every path the merges since #100 changed as today.
  4. An item with a `work` entry: phase 0 fails until both branches exist;
     phase 3 runs each repository's fast gate in its clone and none where
     the repository has no profile; a path outside the second
     repository's areas fails `insideAreas`; phase 6 needs a green PR in
     each; phase 7 passes only when both merges are proved; Close fails
     while either branch remains.
  5. `first`: `f6-merged-<n>` passes only on the merge of that entry, and
     a design-added step that needs it is not offered before.
  6. `wf stale` seeds a refresh when only the second branch is behind its
     trunk.
  7. An item homed in a repository with a profile is not tracked; one
     homed in a repository with only `trunk` is.
  8. `wf validate`: a repository with no entry; an entry with both or
     neither of `profile` and `trunk`; `work` that disagrees with `repos`;
     under q1 (A), a non-tracked item naming a `private` repository.
  9. `wf gate`: a host step in a repository the machine does not list is
     not offered; the fingerprint changes when the second trunk moves.
  10. A queue state from `0cd3c02` (no `work`, no `repositories`) gives
      the same `wf next`, `wf status -md` and `wf validate` output before
      and after (a10).
  11. Under a13 (b): tier 1's report on `main` is byte-for-byte the same
      before and after the move of the two checks.
- **Native:** `native_test.go` covers the wrapper's new resets and the
  prepare command with stub repositories. Each machine's owner re-runs the
  install script after the merge (LOOP-05); a missing clone is reported,
  not made. Not blocking: nothing host-specific changes in behaviour, and
  the Hyper-V preparation runs the same script from the same checkout.
- **First live use:** a16 (a).

## Implementation plan

After `loop-native-lead` and `loop-parallel-rule-wording` have merged
(a15), on `feat/loop-repository`:

1. `docs/development/loop-profile.yaml` and its parser,
   `tools/workflow/check/profile.go`, with tests 1 (LOOP-01, PROFILE-01).
2. `config.yaml`'s `repositories` in `queue.Config`, with its validation
   and its default for the queue's own repository (test 8; GLOBAL-09,
   GLOBAL-10).
3. `work` in `queue.Feature`, `wf declare` writing it, `merge_first` in the
   template's front-matter comment (FLOW-02, FLOW-03).
4. `check.Env.Checkouts`, `-checkouts`, and the checker per repository
   (Decision 5): every `origin/main` of Problem point 3 becomes the
   repository's trunk (tests 4, 6).
5. `Front.Problems`, `kindOf` and `workflowRunsRe` from the profile file
   (tests 2, 3; FLOW-02, FLOW-14 enforcement).
6. The merge confirmations per `work` entry and `first` (test 5; FLOW-08).
7. The tracked test from `config.yaml` (test 7; FLOW-03).
8. `repos/<owner>/<name>/` state, `wf main -repository`, `wf areas -check
   -repository`, `wf stale` per repository (FLOW-05, FLOW-10).
9. `hosts.yaml`'s `repositories`, the gate's fingerprint, a host step's
   `repo` (test 9; FLOW-09).
10. `native-tick.sh`, `install-macos.sh`, `install-windows.ps1` on
    `loop-native-lead`'s wrapper: clones reset, `native.prepare` read from
    the profile file, the install checks (a14; LOOP-05).
11. Under a13 (b): `wf rules`; the rule index and suppression moved into
    `tools/workflow/check/`, tier 1 calling them (test 11; LINT-01,
    LINT-05).
12. Under q1 (A): `private` and its validation.
13. The skills and agents: `workflow-tick` (clones, `-checkouts`, merge
    confirmation per repository, Close per repository, digest heads),
    `workflow-native`, `workflow-intake` (repositories need an entry),
    `wf-publish` (a PR per repository), `wf-implement` and `wf-review` (the
    profile's fast gate) (FLOW-14: cite, not restate).
14. The rule text: FLOW-03, -04, -08, -16, -17, LOOP-01, LOOP-08 and
    PROFILE-01 as approved (FLOW-14).
15. Test 10 on the live state; archive the design with its summary, and
    open the PR (FLOW-12).

After the merge, in an attended session and outside the PR: the
`workflow` branch's `README.md` documents `repositories`, `work`, `repos/`,
`hosts.yaml`'s `repositories` and the profile file's format; `config.yaml`
gains the entries for this repository and `buildwarden/buildwarden-specs`
on Jeff's word.

## Design review

None yet.

## Consolidation

Filled in at Publish.

## Rollback notes

None.

## Archive summary

Filled in at Publish.
