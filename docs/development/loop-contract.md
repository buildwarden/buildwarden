# The Loop Contract

The rules BuildWarden's development loop runs under, in every
repository it works in. There is one loop: one queue of work, one order,
one set of workers and one place of reporting (LOOP-04 to LOOP-06). A
*component* is a repository that has adopted this contract (LOOP-08).
Its profile, its invariants, its designs and its checks are kept in it.
It has no queue of its own: an item in the queue names the repositories
it works in. `buildwarden/buildwarden` is the first component, and
`buildwarden/buildwarden-relay` is to be the second. A component adopts
the contract by reference and never copies it, so a rule changed here
changes for every component at once.

The contract is this document and its detail: the process rules
([`process.md`](./process.md), FLOW-\*) and the behavioral-linter rules
([`behavioral-linter.md`](./behavioral-linter.md), LINT-\*). Together they
are the *loop rules*. A component's own standards are its *project
invariants*. They stay in its repository and are not part of the
contract.

A LOOP rule that cites a FLOW or LINT rule for a detail leaves that
rule's severity as it is. The cited part is held at the cited rule's
severity; only what the LOOP rule states itself is held at its own.

See [`README.md`](./README.md) for the rule format.

## Where things are

| | Today | Once the loop repository exists |
|---|---|---|
| The contract and its detail | `docs/development/` in `buildwarden/buildwarden` | The loop repository |
| The queue, `config.yaml`, the host table, digests and reports | The `workflow` branch of `buildwarden/buildwarden` | The loop repository |
| `wf`, the skills and the agent definitions | `tools/workflow/` and `.claude/` in `buildwarden/buildwarden` | The loop repository |
| A component's profile, invariants, designs and checks | Its own repository | Its own repository |
| Session recordings | `buildwarden/buildwarden-sessions` (FLOW-18) | The same |

The loop repository is the one the componentization design's
Decision 13 gives the loop. The second column says what moves to it.

---

### LOOP-01 — Shared rules, a local profile

**Severity:** MUST
**Check:** agent-judgment

What is shared and what is local:

| Concern | Shared (the loop rules) | Local (each component) |
|---|---|---|
| Specifications | When a specification changes before code (LOOP-02) | The specifications, where they are kept, their format, and how a release of them is pinned |
| Checks | The classes, the tiers, when they run and what a failure does (LOOP-03) | The concrete checks and their implementations; the component's invariants |
| The queue | The kinds of item, the phases, the order and what blocks what (LOOP-04) | The areas of its tree: the loop keeps them, with the queue, as the component's area map, which says what two items may change in it side by side (FLOW-05). Otherwise nothing: there is one queue, and an item names the repositories it works in |
| Workers | Affinities, leases, budgets and the host table (LOOP-05) | Each machine's schedule and credentials; the host names a component's work uses |
| Reporting | What a run records and where (LOOP-06) | Nothing |
| Other components | Reconciliation (LOOP-07) | What it releases and what it consumes |

Each component keeps one **profile**: a document in its repository,
beside its invariants, that fills the local column. The profile is one
of the component's own rules: it carries a rule header under a prefix the
component owns, so a change to it is declared and approved like a change
to any of the component's invariants (FLOW-14). It gives the component's
entry for each of these terms:

| Term | What the component says | Used by |
|---|---|---|
| Component | Its name and repository | LOOP-04, LOOP-05, LOOP-06 |
| Trunk | The branch only a human merges to | FLOW-03, FLOW-08, FLOW-17 |
| Toolchain | Its language and build tools | LOOP-08 |
| Rules | Where its invariants are, and the prefixes it owns | LINT-01, FLOW-14, LOOP-08 |
| Designs | The design directory, the template and the sections the template adds | FLOW-01, FLOW-12 |
| Impact vocabulary | The values its designs may declare, and the keys it adds | FLOW-02 |
| Specifications | The seams it owns or implements, where each specification is, and how it pins a release | LOOP-02, LOOP-07 |
| Fast gate | The lint and test commands every change passes | FLOW-03, FLOW-17 |
| Checks | For each class, where its checks are defined, their tier, and any that do not block | LOOP-03, LINT-02 |
| Test tiers | Its tiers of tests and where each runs | FLOW-09 |
| Native hosts | The host names its work uses | FLOW-04, FLOW-09 |
| Reference documents | The documents that change with user-facing behaviour, and its guides | FLOW-03 |
| Sweep | Its dead-code command, what that may keep, and its overview document | FLOW-15 |
| Boundary rule | Its rule for changes that need a security note | FLOW-02, FLOW-08 |
| Flaky-test rule | Its rule for a failure that does not reproduce | FLOW-11 |
| Rule and machinery paths | The paths whose change is a rule change, and the loop's machinery in its tree | FLOW-14 |
| Areas | Where its area map is kept, and the command that lists its dependency edges | FLOW-02, FLOW-05 |
| Release | How a release is cut and where its gate is wired | LINT-03 |
| Consumables | What it releases, and what it consumes at which version | LOOP-07 |
| Sessions | Its session-recording configuration | FLOW-18 |

Where a FLOW or LINT rule names a path, a command or a rule of
`buildwarden/buildwarden`, a component reads it as its own profile's
entry for that term. Where the entry and the rule's example differ, the
entry is the component's. An entry may name the file that holds a fact
in place of restating it. A term the component does not have is filled
as `none`, and a rule read through a `none` term does not apply there. A
term left unfilled is a finding; it is never read as another component's
entry.

The entries the loop's tools read are also written in a **profile
file**, in the format the loop's tools document, which the profile names
and the loop's configuration records for the repository. The file is
part of the profile: a change to it is a change to the profile, and
where the file and the profile differ, that is a finding. A key the file
leaves out is the term `none`. The loop's configuration also records
each repository's trunk, and that record is the trunk the loop's tools
use, never the file's: a profile file whose trunk differs from it is a
finding, and every check of the loop's tools that reads the file fails
on it. While FLOW-20 stands, it also records a repository's program
trunk, which is not a trunk and is not in the profile file. The configuration records a repository's profile file only once
the change that adopts the repository (LOOP-08) has merged.

The agent part: the design review of a change to a profile judges that
every term is filled or `none`, and that the profile file agrees with
it; Review judges that the paths and commands a change relies on are the
profile's.

---

### LOOP-02 — Design-first always, spec-first at a seam

**Severity:** MUST
**Check:** agent-judgment

Every item has a design before implementation (FLOW-01). A *seam* is a
boundary between components that a component's rules name and its
designs declare (FLOW-02). A *specification* is a written document that
an implementation of a seam is tested against. Where a seam has one, the
specification is the source of truth for behaviour at the seam: an
implementation is tested against it, never against another component's
code.

A design that declares a seam says, under **Specification changes**,
which specification document and requirements the item changes, or that
the seam has no written specification and what defines it.

An item that changes behaviour a specification defines is also
*spec-first*:

- The design review decides the specification change the design lists
  (FLOW-19).
- In Implement the specification changes first. The code follows and is
  tested against the changed specification. A change to behaviour that
  a specification defines, with no matching specification change, is a
  finding.
- An item that implements behaviour already specified changes no
  specification; its design cites the requirements it implements.
- A specification an implementation cannot meet is not worked around in
  code. The item returns to Design with the finding (FLOW-06), the
  question is recorded in the queue (FLOW-07), and the answer is a change
  to the specification or to the design.
- A run that changes a specification reports that change apart from the
  code (LOOP-06).

Behaviour no specification defines is design-first only, at a seam or
not. Its written record is the component's contract and reference
documents, changed in the same change.

Where a specification is kept, its format, and how a component pins the
release it implements are the component's (LOOP-01), within LOOP-07.

A component may check mechanically that a change at a seam changes its
specification; that check belongs to the component's own rule, not to
this one. The agent part: the design review judges the **Specification
changes** list; Review judges that the diff changes no behaviour at a
seam beyond what the specification change states.

---

### LOOP-03 — Checks: four classes, two tiers, run at the gates

**Severity:** MUST
**Check:** agent-judgment

A component declares its checks in four classes:

1. **Build**: every declared target builds.
2. **Specification**: behaviour matches its written documents. LOOP-02's
   coupling, conformance suites and contract tests are here. Where a
   specification has a conformance suite, each of its requirements is
   covered by a case that names it, or is listed with the reason it is
   not machine-checked, and each case names the requirements it checks.
3. **Safety**: the lints, tests, audits and boundary rules the component
   declares.
4. **Process**: the loop's rules held. The phase done-when checks
   (FLOW-03) and the queue's validity are here.

Every check is in one of two tiers (LINT-02). A deterministic check
blocks: its failure stops the phase transition and the pull request
(LINT-03). There are two exceptions, each stated where it applies and
never assumed. A component may declare a deterministic check a hint or a
report, by name in its profile; it then informs. Host-bound validation
is blocking or tracked, as the item's design says (FLOW-09). A judgment
check informs: its findings go to the human, who signs off by merging or
dismisses one with a recorded reason (LINT-03, FLOW-08). No agent holds
a veto over trunk, and none approves it.

Checks run at the phase gates (FLOW-03) and in CI on the pull request,
on the change and not on the whole repository (LINT-04). A check too
expensive or too host-bound for every change runs scoped to what the
change reaches, on a native host as blocking or tracked validation
(FLOW-09), and in full before a release.

A component writes its invariants as rules in LINT-01's format, so one
rule index and one judgment agent (LINT-06) can read any component's.
The concrete checks are local: the profile says, for each class, where
its checks are defined and which tier they are in. A check with no tier
is a finding.

The agent part: the design review of a profile judges that each class is
filled, or `none` with a reason; Review judges that a check or rule a
change adds has a class and a tier.

---

### LOOP-04 — One queue, one order

**Severity:** MUST
**Check:** agent-judgment

There is one queue, for work in every repository. An item names the
repositories it works in: its design declares them and the queue records
them (FLOW-02). An item that names none works only in the repository that
holds its design. An item is one of two kinds (FLOW-03): a *feature*,
which changes a component, or a *process item*, which changes the rules
the loop runs under: the loop rules, the loop's machinery, or a
component's own invariants when no feature needs the change. Every item
has the phases of its kind (FLOW-03), and every step says who may take it
(FLOW-04).

- There is one order, and FLOW-05 gives it. FLOW-05 also says which
  items may be in progress side by side and which wait for another. This
  rule adds no order and no limit of its own.
- A process item ranks ahead of every item not already in implementation
  (FLOW-05). It stops other work only through the `holds` it declares
  (FLOW-05, LOOP-09).
- Priority is the human's. An agent places an item only where a rule
  says so: the session that finds a regression trunk already has files
  its fix at the top of the fixes (FLOW-09), and the human may move it.
  No agent otherwise reorders items.
- An item that cannot proceed says what it waits for, and its worker
  moves on (FLOW-05, FLOW-07).
- An item that waits on another component waits for a release (LOOP-07):
  it is held with a blocking question that names the release (FLOW-07),
  and resumes when the release exists.

The agent part: intake and the design review judge an item's kind and
the repositories it names.

---

### LOOP-05 — Workers are scheduled, routed and identified

**Severity:** MUST
**Check:** agent-judgment

Work is done by workers that take steps from the queue: `cloud`, `human`
or `host:<name>` (FLOW-04).

- **Scheduled and bounded.** An unattended worker runs on a schedule and
  within a budget (FLOW-13). A native machine's scheduled session starts
  an agent only when the machine has work (FLOW-09).
- **Routed by affinity.** Work that needs native hardware carries a
  host name. One table, kept beside the queue, says which host names
  each machine serves, for work in every repository. Work that any
  machine with a capability can do carries `<capability>-any`. FLOW-04
  says which worker may take a step.
- **Identified.** Every claim records the worker (its kind, its host
  name and its session) and the item, and through the item the
  repositories the work is in (LOOP-04).
- **Set up by the owner.** The scheduler on each machine is installed by
  the machine's owner, by hand. No agent installs or changes it. A
  worker's credentials are the session's or the machine's own, are
  configured outside every repository, and never appear in a prompt, a
  queue or a report (FLOW-18).

The agent part: the design review of a change to the scheduling scripts
or the worker skills judges these four.

---

### LOOP-06 — Every run reports in one place

**Severity:** MUST
**Check:** agent-judgment

Each unattended run appends one section to the day's digest (FLOW-13).
There is one digest, for work in every repository. The section records:

1. each step taken: the item, its kind and FLOW-05 class, the step, its
   outcome and the branch head it left in each repository it changed;
2. each specification the step changed, named apart from the code
   (LOOP-02);
3. the code change in a line, or the pull request that carries it;
4. the check results by tier: the deterministic checks as passed or
   failed, and the judgment tier's findings by severity with where its
   report is (LINT-05);
5. everything blocked or parked, and why: on the human, on a host, on
   another component's release, on a budget.

The judgment tier's report is kept per branch head (LINT-04, LINT-05). A
pull request's body lists the specification changes and the rule changes
it carries, and the judgment tier's MUST findings (FLOW-08).

Digests and reports are kept with the queue, not in a component's code
history.

The agent part: a run checks its section against this list before it
ends; the design review of a change to a skill that writes the digest
judges that the list is still covered.

---

### LOOP-07 — Components couple only through releases

**Severity:** MUST
**Check:** agent-judgment

Components evolve independently. One depends on another only through a
release.

1. A component lists its **consumables** in its profile: its tagged
   releases, the specifications each implements and at which versions,
   and its artifact forms.
2. A consumer adopts a release through a **reconciliation item**: an
   item in the queue that names the consumer's repository,
   `adopt <consumable> at <version>`. It is a feature item (LOOP-04),
   filed through intake, or by the loop for a pre-release on a program
   trunk (FLOW-20), and FLOW-05 gives its place in the order.
3. The item moves the consumer's pin and, where a seam changes, its
   specification first (LOOP-02), then its code, and then retires any
   in-tree equivalent.
4. Both components' checks pass after adoption. A release is never
   changed to make that so: a change the consumer needs is a new item
   that names the producer's repository, and a new release.
5. A component's trunk names only released versions of what it
   consumes, never a pre-release. A program trunk (FLOW-20) may name a
   pre-release. A branch may name an unreleased commit while its work is
   in progress.
6. A specification kept in a repository of its own is consumed the same
   way. A seam change is released by the repository that holds the
   specification and adopted by each implementation through its own
   reconciliation item, at its own time.

The agent part: Review of a reconciliation item judges points 3 to 5;
intake judges that a change one component needs from another was filed
as an item that names the producer's repository.

---

### LOOP-08 — Adopting the contract

**Severity:** MUST
**Check:** agent-judgment

A repository becomes a component by:

1. linking to this contract from its README and its agent context file.
   The link is to the contract's trunk: never a copy, and never a pinned
   version;
2. writing its profile (LOOP-01), every term filled or `none`, and its
   profile file;
3. writing its invariants as rules in LINT-01's format, under prefixes
   it names in its profile, its safety invariants among them (LOOP-03);
4. keeping designs with the front-matter and sections the detail names
   (FLOW-02, FLOW-12, FLOW-19), plus its own;
5. declaring its consumables (LOOP-07), even when its first release is a
   stub;
6. seeing that only a human merges to its trunk (FLOW-08), and
   protecting the trunk where its host offers that; deleting head
   branches on merge (FLOW-16); and recording sessions only to the
   private sessions repository (FLOW-18).

It gets no queue of its own: its work is items in the one queue that
name it (LOOP-04).

A rule prefix belongs to one owner, so a rule ID names one rule wherever
it is cited. `LOOP-`, `FLOW-` and `LINT-` are the loop's. A component
names its prefixes in its profile and takes none that another profile
names. A prefix changes owner only when its rules move, in the items
that move them. A component may cite another component's rule by ID as
binding in its own repository.

An item may work in a repository that has not adopted the contract,
such as one that holds specifications or a fork. There the worker is
bound by FLOW-08, FLOW-16, FLOW-17 and FLOW-18, and the item's design
says which invariants apply there, by rule ID. Once that repository
adopts, it and the other components couple only through releases
(LOOP-07).

The contract assumes of a component: git, a trunk and feature branches,
pull requests that a human merges, a deterministic check that can block
a pull request, Markdown rule files, a profile file in the loop's
format, and designs with front-matter. It
assumes nothing else. No loop rule may require a language, a build tool,
a test framework, a directory layout beyond what the profile names, an
agent tool, a CI system, or that a component has drivers, user-facing
surfaces, security boundaries or a public repository. Where the detail
names one of these from `buildwarden/buildwarden`, LOOP-01's reading
rule applies, and the wording is made neutral when the rule is next
changed.

The agent part: the design review of an adopting component's first item
judges the six steps, and that its prefixes are no other component's;
the design review of a process item judges that a new or amended loop
rule assumes nothing beyond the list.

---

### LOOP-09 — The contract changes only through its owner

**Severity:** MUST
**Check:** agent-judgment

The loop rules (LOOP-\*, FLOW-\*, LINT-\*), the loop's machinery (its
tools, skills and agent definitions) and the checks that enforce a loop
rule, wherever they are kept, change only through a process item
(FLOW-03, FLOW-14). Its design is settled by the owner, the human of
FLOW-08, in a design review (FLOW-19), and its merge is the owner's. While
FLOW-20 stands, a process item whose change touches none of the rules
FLOW-20 reserves to the owner is settled instead by FLOW-20's process
test, a review session that did not write the design, and is recorded;
its merge is still the owner's. Where this contract names a process
item's design review, that test stands in for it. No
component signs off a change, and no feature's design declares one. A
component that needs a loop rule changed asks for a process item.

A change binds every component from its merge, and reaches work in two
ways. An unattended run takes the loop's tools, skills and configuration
from trunk as they are when it starts, and keeps them until it ends.
Rule text and a component's checks are read from the branch being worked
on, so a branch forked before the change follows the old text until it
merges trunk, which FLOW-05 has it do while it waits and Publish does at
its start. A process item whose change cannot wait for that, or that
would disrupt running work, declares the workers it holds (FLOW-05).

A component's profile and invariants, and the checks that enforce them,
are its own. They change by either of FLOW-14's routes, in that
component.

The agent part: intake and the design review, or the process test's
session, judge that a change to a loop rule or to its enforcement is a
process item, and the process test's session that it touches none of
the rules FLOW-20 reserves to the owner.
