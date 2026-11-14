---
feature: alters-slot
status: draft
drivers: []              # the loop's queue code; no driver tree is reached
seams: []
surfaces: []             # `wf` is the loop's tool, not a user-facing surface of the product
security_boundary: none
alters_relationships: false  # no product component's role or interaction changes (FLOW-02);
                         # this changes how the loop counts the features that do
design_review: required  # a process item's design always gets its review (FLOW-03, FLOW-19)
native_validation: []    # pure functions of the queue state, the same on every host
tier2_gaps: []
rules: [FLOW-02, FLOW-03, FLOW-04, FLOW-05, FLOW-06, FLOW-07, FLOW-12, FLOW-14, FLOW-18,
        FLOW-19, FLOW-20, GLOBAL-09, GLOBAL-10, GLOBAL-11, TEST-01, TEST-02]
rule_changes: []         # the code is made to match FLOW-05 and FLOW-20 as written
concepts: [workflow, queue, wf-next, wf-validate, alters-relationships, alters-slot,
           flow-05-limits, process-item, one-queue, entire-hooks]
related: [componentization.md, design-principles.md, next-skips-merge-wait.md,
          workflow-bootstrap.md, regression-fixes-follow-up.md]
---

# `wf next` and `wf validate` count the features that hold an `alters_relationships` slot

A process item, and a program item under FLOW-20: it is the first step of the
loop change Jeff approved in direction on 2026-11-14 (one queue, parallel where
items do not collide, native-led items). It is small on purpose: one design
sitting and one merge. It holds no worker.

## Problem

FLOW-05 limits how many features that declare `alters_relationships: true` are
in flight, and makes a feature that finds it alters relationships wait at the
end of Design until a slot is free. FLOW-20 makes that limit the value in the
workflow branch's `config.yaml`, which is 3 since 2026-11-12. The code does
neither (`tools/workflow/queue/`, at `b903f65`):

- **`wf next` ignores the value.** `altersHolder` (`next.go`) returns the
  first other in-progress item that declares `true` and is in phase 2 or
  later. One such item is enough to stop the cloud, whatever the limit says.
- **A slot is "held" by finishing Design, so items hold each other.**
  Completing `f1-design` moves an item to phase 2 (`Complete`, `phases.go`),
  before its review and before any Plan step is taken. From then on it counts
  as the holder for every other item, and every other such item counts as the
  holder for it. `cloudHold` applies the test to every cloud step of phase 2
  or later, so an item already in Implement stops too.
- **`wf validate` counts differently.** `Validate` (`queue.go`) counts every
  in-progress item in phase 2 or later that declares `true`, fails when the
  count is above the limit, and reads an unset limit as no limit. So it fails
  on a state `wf done` produces by itself: one more item finishing Design than
  the limit allows. Both skills and intake need `wf validate` green before a
  push (`workflow-tick` step 5, `workflow-native`, `workflow-intake` step 4),
  so that `wf done` could not be pushed.

Today's `Next` and `Validate`, run in memory against the live queue moved
forward step by step (rows 1 and 2), with the limit changed (rows 4 and 5),
and on four seeded items (row 3). Nothing was written:

| State | `wf next -worker cloud` today | `wf validate` today |
|---|---|---|
| Both reviews approved, neither planned | seam-specs "held by driver-contract", driver-contract "held by seam-specs": `none`, for good | ok |
| seam-specs in Implement; driver-contract finishes Design, reviewed or not | seam-specs `f3-implement` "held by driver-contract"; driver-contract's Plan "held by seam-specs" | ok |
| Four such items past Design, limit 3 | each "held by" another: `none` | fails: "4 features past Design alter relationships, limit 3" |
| Two such items past Design, limit 1 | each held by the other | fails: "2 features ... limit 1" |
| The same, limit unset | each held by the other | ok (no limit read) |

`seam-specs` (S1) awaits its design review in phase 2. `driver-contract` (K1)
is back in Design and is being redrafted now. When that redraft finishes, the
second row applies: seam-specs cannot plan or implement, and neither can
driver-contract, until someone edits the queue by hand.

`tools/workflow/` outside `check/` changes only through a process item
(FLOW-14). This was question a13 of `loop-contract`, whose review stopped on
2026-11-14; it is lifted out so the fix does not wait for that design.

## Prior art

- **`loop-contract` a13** (`feat/loop-contract`, "The three machinery
  changes", unreviewed) proposed counting the *other* items past Design
  against the limit. That fixes the two live items at limit 3 and nothing
  else: four items past Design still hold each other, two still do at limit
  1, and `Validate` still fails on the state. This design keeps a13's limit
  (the config value, 1 when unset) and its dropped fallback, and changes what
  is counted. a13 retires from `loop-contract` at its next revision.
- **`design-principles`** (#104), Decision 9, took process items out of the
  in-progress limit and said the `alters_relationships` limit "still counts
  every in-progress item that declares it, process items included". The rule
  text the same PR wrote says "of those features". This design follows the
  rule text (Decision 4, question a3).
- **`next-skips-merge-wait`** (#106) kept `wf next` free of git: it reads the
  queue state, and the clock only for leases. `Validate` and `Summary` read no
  clock. Kept: what holds a slot is read from the item's own steps, with no
  clock and no git.
- **`componentization`** (#117): FLOW-20's Limit bullet and the value 3 (its
  a5). The loop change proposal of 2026-11-14 (section 3, "The three limits")
  gave the principle used here: a limit counts only items that already hold
  something, so no two items hold each other.
- **`workflow-bootstrap`** introduced the slot as "the one
  `alters_relationships` slot another feature holds"; **`regression-fixes-follow-up`**
  (#103) left it unchanged when it added the class order.

## Decisions

1. **A slot is held by a plan.** A feature holds an `alters_relationships`
   slot from the claim of its Plan step until the item is done (a1). An item
   that has only finished Design, reviewed or not, holds nothing.
   - *Rejected: from the end of Design, as today, with the limit read from
     config* (a13 as drafted). Items that finish Design together hold each
     other as soon as there are more of them than the limit.
   - *Rejected: from Implement.* Any number of items could plan at once, and
     the ones over the limit would then wait with a finished plan going
     stale.
   - *Rejected: a `slot` field `wf claim` writes.* New schema for what the
     steps already say, and `loop-parallel` reworks this function next.
   - *Rejected: only a live lease holds.* `Validate` would need a clock, and
     `wf next` and `wf validate` could disagree across an expiry.
2. **One function gives the holders and one gives the limit; `wf next` and
   `wf validate` both call them.** `wf next` withholds a Plan step from a
   feature that holds no slot when the holders number the limit or more.
   `wf validate` fails when the holders number more than the limit. A holder
   is never refused, so refusals only run from an item without a slot to
   items with one, and no cycle can form.
3. **The limit is `limits.alters_relationships`; 1 when `config.yaml` does
   not set it; a written value below 1 is an error.** FLOW-05 writes "at most
   1" and FLOW-20 makes it "the value the human sets".
   - *Dropped fallback:* `wf validate` reading an unset limit as no limit.
   - *Rejected: 0 means no limit*, as `limits.in_progress` reads it. FLOW-05
     has no unlimited case for this limit.
4. **Process items neither hold a slot nor wait for one** (a3). FLOW-05 and
   FLOW-20 both say "features", a process item has no Plan step, and FLOW-05
   gives a disruptive process item `holds` for this purpose.
   - *Rejected: counted, as the code and `design-principles` Decision 9 have
     it.* A rule change would then wait behind features, against the class
     order, and FLOW-05's sentence would need rewording.
5. **A holder keeps its slot through a return to Design** (a4). Its plan and
   code are still in flight; it finishes before a waiting item starts
   (FLOW-05, depth first).
   - *Rejected: the slot is given up on a return, as today.* A waiting item of
     lower priority takes it during the returned item's review, and the
     half-built item then waits until some holder is merged and closed.
6. **The wait stays where it is: for the cloud, in `cloudHold`.** `wf claim`
   is not changed. Every claim the skills make follows `wf next`. A Plan
   claim made by hand past the limit fails the next `wf validate`, which
   names the holders.
   - *Rejected: a guard in `wf claim` now.* `loop-parallel` adds one guard for
     `wf next` and `wf claim` covering all three limits.
7. **No rule text changes.** The code is made to match FLOW-05 and FLOW-20
   (see "The rule text stays").
8. **The PR also carries the refreshed Entire hook configuration** (a2), as
   its own commit, so the review can drop it without touching the rest.

## Questions

### Blocking

None.

### Assumed

- **a1. What holds a slot.** Proceeding with (a); confirm.
  - (a) From the claim of the item's Plan step until the item is done.
  - (b) From the end of Design, as today, with only the limit fixed.
  - (c) From Implement (Plan done).

  Recommended: (a). Under (b) items still hold each other once more of them
  finish Design than the limit. Under (c) nothing bounds how many plan at
  once, and the ones over the limit wait with a finished plan (Decision 1).
- **a2. The refreshed Entire hook configuration** parked as commit `ac8b41a`
  on `claude/intelligent-ride-54y60a`: `.claude/settings.json` (the
  session-start hook also reports an `entire` on PATH that fails to run) and
  `.entire/settings.json` (key order only). `.claude/` reaches `main` only
  through a process item (FLOW-14), and this is the next one to merge.
  `loop-contract`'s a12 asks the same and drops it if this carries it.
  Proceeding with (a); confirm.
  - (a) This PR carries the two files as that commit has them, in a commit
    of its own.
  - (b) `loop-contract` carries it, as its a12 has it.
  - (c) Its own process item.
  - (d) Drop it.

  Recommended: (a).
- **a3. Whether a process item that declares `alters_relationships: true`
  counts.** Today it holds the slot once it is in Implement and its
  `x2-implement` waits for the slot, and `design-principles` Decision 9 said
  so in passing. FLOW-05 and FLOW-20 say "features". No open process item
  declares `true`. Proceeding with (a); confirm.
  - (a) It neither holds a slot nor waits for one; `holds` is how a process
    item stops other work.
  - (b) It counts as today: it holds from the claim of `x2-implement` and
    waits at it. FLOW-05's sentence is then reworded, a rule change.
  - (c) It holds a slot but never waits.

  Recommended: (a).
- **a4. Whether a holder keeps its slot when it returns to Design**
  (FLOW-06). Today it gives it up at once. Proceeding with (a); confirm.
  - (a) It keeps it while it has a finished plan: a return from Implement or
    later. A return from Plan itself drops the unfinished Plan step
    (`Back`), and the slot with it, since there is no plan and no code.
  - (b) Every return gives the slot up; the item waits for one again at its
    next Plan step.

  Recommended: (a). Its cost is one edge, set out under "More holders than
  the limit".

## Design

### Definitions

- **The limit** is `limits.alters_relationships` in the workflow branch's
  `config.yaml`, or 1 when the key is absent.
- **A feature holds a slot** when it is in progress, is not a process item,
  its queue file says `alters_relationships: true`, and one of its Plan steps
  (`f2-plan`, in any `-r<n>` form) is done or carries a claim. A claim is a
  lease with a claimant, expired or not: an abandoned claim keeps the slot
  for its item until that Plan step is taken up again or released
  (`wf release`).
- **A Plan step waits for a slot** when its feature declares `true`, holds no
  slot, and the features holding one number the limit or more. `wf next`
  then does not offer it to the cloud and reports
  `an alters_relationships slot (limit 3), held by a, b, c (FLOW-05)`.

Nothing else waits. A feature past Plan holds a slot by definition, so the
old tests on later steps (`st.Phase >= 2 && f.Phase < 6`) go. Human and host
steps never waited for the slot and still do not: a design review is offered
while every slot is held. `NextFor` (an attended run on a named item) goes
through the same path and waits the same way.

### In the code

- `queue.go`: `Limits.AltersRelationships` becomes `*int` (as `NativeBudget`
  is a pointer), so an absent key and a written 0 differ. `altersLimit()`
  returns the value, or 1 when absent. `Validate` reports
  `config: limits.alters_relationships N is below 1` for a written value
  below 1, and `N features hold an alters_relationships slot, limit L
  (FLOW-05): <names>` when the holders number more than the limit. Its
  signature does not change.
- `next.go`: `altersHolder` is replaced by `holdsAltersSlot()` on a feature
  and `altersHolders()` on the state, the list both callers use. `cloudHold`
  withholds only a Plan step, as defined above.
- `phases.go`: `IsPlan(st)`, beside `IsDesignReview` and `IsMerge`, parsed by
  `stepTemplates`.
- `cmd/wf/main.go`, `summary.go`, `hosts.go`, `check/`: unchanged.

### The cases

Limit 3 unless said. "Holders" is after this change.

| # | State | Today | After |
|---|---|---|---|
| 1 | Live now: seam-specs past Design, review pending; driver-contract in Design | `next`: none (human review, a leased step). `validate`: ok | The same output. Holders: none |
| 2 | seam-specs approved; driver-contract in Design | seam-specs `f2-plan` offered | The same; the claim makes seam-specs a holder (1 of 3) |
| 3 | Both past Design, reviews pending | none (human steps) | The same. Holders: none |
| 4 | Both approved, neither planned | Each held by the other, for good | seam-specs `f2-plan` (priority 62), then driver-contract's when seam-specs blocks. 2 of 3 |
| 5 | seam-specs in Implement; driver-contract finishes Design | seam-specs stops; driver-contract's Plan never starts | seam-specs goes on; driver-contract plans once approved. 2 of 3 |
| 6 | Four altering features finish Design | All held; `validate` fails, so the fourth `wf done f1-design` cannot be pushed | `validate` ok (no holder). Plan goes to the first three in FLOW-05 order; the fourth waits, naming them |
| 7 | Three hold, a fourth waits; one holder is done | n/a | The fourth's Plan is offered |
| 8 | Limit 1 or unset: A in Implement, B approved | Each held by the other; `validate` fails at 1 and passes when unset | A goes on; B waits for A; `validate` ok (1 holder) |
| 9 | A holder returns to Design from Implement or later | Gives the slot up | Keeps it (a4) |
| 10 | An item returns to Design from Plan itself | Gives the slot up | Holds nothing: `Back` drops the unfinished Plan step |
| 11 | Plan claimed, lease expired, not released | Holds (phase 2) | Holds; any cloud run may take that Plan step up again |
| 12 | A redesign changes `true` to `false` | Slot freed | Slot freed |
| 13 | A process item declares `true`, in Implement | Holds the slot; its `x2-implement` waits for it | Neither (a3) |
| 14 | A written limit below 1 | `next` uses 1; `validate` reads no limit | `validate` fails on the value; `next` offers no Plan step to an altering feature |
| 15 | loop-contract, alters-slot (process, `false`) | No effect | No effect |

### More holders than the limit

`wf next` then keeps every holder moving and gives no other altering feature
its Plan step; `wf validate` fails and names the holders. Three things
produce it, none of them `wf next`:

- Jeff lowers the limit under the holders. As today, lower it once the
  holders number the new value or fewer.
- A Plan step is claimed by hand with `wf claim`, which checks no limit until
  `loop-parallel`. The skills claim only what `wf next` offers, so this takes
  a command typed outside them; the next `wf validate` fails, and the claim
  is released.
- A feature that already has a plan returns to Design, and the redesign
  changes `alters_relationships` from `false` to `true` while every slot is
  held. Writing `true` makes it a holder over the limit. FLOW-05 says a
  feature that finds it alters relationships waits for a slot, but this one
  is already built, so which item pauses is Jeff's call. Design leaves the
  queue's value as it was and raises a blocking question naming the holders
  (FLOW-07); Jeff raises the limit (FLOW-20) or the item waits in Design
  until a holder is done. No agent text is added for this case: a session
  that writes `true` anyway is stopped by `wf validate` before its push, with
  the holders named. It is the cost of a4 (a); under a4 (b) it cannot
  happen.

### The rule text stays

FLOW-05, as written today:

> At most 5 features are in progress at once, not counting process items
> (FLOW-03), and at most 1 of those features declares `alters_relationships:
> true` (FLOW-02), because two such features reshape the same structure and
> make each other's consolidation costly. Process items are not counted
> because they are short and already ranked ahead of fixes and features; one
> that would disrupt running work declares `holds`. A feature whose design
> discovers it alters relationships waits at the end of Design until that
> slot is free.

FLOW-20's Limit bullet:

> **Limit.** FLOW-05's limit on features past Design that declare
> `alters_relationships: true` is the value the human sets in the workflow
> branch's `config.yaml`, which may be above 1. The limit of 5 features in
> progress stands.

The code is being made to match the rule on three points: the limit is the
config value; the counted items are features; and a feature waiting at the
end of Design is waiting *for* a slot, so it cannot be the one holding it.
The rule does not say when a slot is given back; "until the item is done" is
what the code already does, and a4 settles the return to Design. Under a4 (a)
a holder that has returned to Design is counted while it is in Design again,
which reads FLOW-20's "features past Design" as past their first Design; that
reading is confirmed with a4. `loop-parallel`
rewrites FLOW-05's limits and removes FLOW-20's Limit bullet, and writes this
definition into the rule then. `rule_changes` is empty. If the review picks
a3 (b), FLOW-05's first sentence needs rewording and this design returns
with that entry.

### What `loop-parallel` changes later

The holder test (a feature, declares `true`, Plan claimed or done) stays.
Three things change around it:

- `altersHolders()` takes the item that asks and returns only the holders
  that *collide* with it; the limit is then per asking item. Without `areas`
  every item collides with every other in its repository, which is this
  design's behaviour.
- The test moves from `cloudHold` into the one guard `wf next` and `wf claim`
  share, for every worker; `loop-native-lead` then lets a machine take a led
  item's Plan step under it.
- `Validate` checks each holder against the holders it collides with.

### What this item does not do

- No collision relation, `areas`, `repos` or `lead`: `loop-parallel` and
  `loop-native-lead`.
- No guard in `wf claim`, and no change to the in-progress limit or the PR
  slot.
- No `wf new -hold` (`loop-contract` a11).
- No native push hardening (it goes with `loop-native-lead`).
- No change to a rule, a skill or an agent definition.

## Defaults and overrides

| Default, override or fallback | What it is | Why, and what overrides it |
|---|---|---|
| `limits.alters_relationships` absent | 1 | FLOW-05's written limit (GLOBAL-09). A value in `config.yaml` is used exclusively, by `wf next` and `wf validate` alike (GLOBAL-10). A written value below 1 cannot be used: `wf validate` names it (GLOBAL-10, GLOBAL-11). Settled by FLOW-05 and FLOW-20, so no question. |

**Fallbacks.** None added. One is dropped: `wf validate` reading an absent
limit as no limit (Decision 3). **Merges.** None. The Entire hook
configuration (a2) adds no default.

## Cross-driver impact

None: no driver, seam or user-facing surface changes.

## Validation plan

- **Tier 0**, `tools/workflow/queue` (`make test`, TEST-01, TEST-02). The
  tests build each state with `Seed`, `Complete`, `Claim`, `Release` and
  `Back`, never by setting `Phase` by hand, and assert `Next` and `Validate`
  together:
  - `TestAltersSlotHeldFromPlanClaim`: finished Design holds nothing; the
    claim holds; `Release` gives it back; Plan done holds; the item done
    frees it (cases 2, 7).
  - `TestAltersSlotNoMutualHold`: cases 4, 5 and 6, and a holder is never
    refused.
  - `TestAltersSlotLimit`: absent, 1 and 3, with holders from none to one
    over the limit (cases 8, 14, and "More holders than the limit").
  - `TestAltersSlotAcrossReturns`: cases 9, 10 and 12.
  - `TestAltersSlotExpiredClaim`: case 11.
  - `TestAltersSlotSkipsProcessItems`: case 13; it replaces the last block of
    `TestLimitSkipsProcessItems`, which asserts the opposite today.
  - `TestAltersSlot` goes: it asserts the mutual hold and the failing
    `Validate`.
- **The live queue.** With `main`'s `dist/wf` and with the branch's `wf`
  built to a scratch path, on a copy of the `workflow` checkout: `wf validate`
  prints `ok` both times, and `wf next -worker cloud/check` prints the same
  lines for the state the queue is in that day (case 1 today). The outputs go
  in the PR description.
- **a2:** `tools/workflow/flow18-check.sh` passes; both files parse as JSON;
  `git diff ac8b41a -- .claude/settings.json .entire/settings.json` is empty.
- **`make lint`**, and `make enforcement` (a no-op: neither `tools/tier1/`
  nor `tools/workflow/check/` changes).
- **Native validation:** none, and no tracked gap (FLOW-09). Nothing here
  depends on the host.

## Implementation plan

A process item has no Plan phase; this is the work of `x2-implement`.

1. Tests first: add the tests above and change the `state()` helper for the
   pointer; they fail on the current code (TEST-01, TEST-02).
2. `queue.go`: the `*int` limit, `altersLimit`, and `Validate`'s two messages
   (GLOBAL-10, FLOW-05, FLOW-20).
3. `phases.go`, `next.go`: `IsPlan`, `holdsAltersSlot`, `altersHolders`, the
   new test in `cloudHold`; remove `altersHolder`; bring `Next`'s comment up
   to date (FLOW-05).
4. If a2 stands: a separate commit that takes `.claude/settings.json` and
   `.entire/settings.json` as `ac8b41a` has them (FLOW-14, FLOW-18).
5. `make test`, `make lint`, `flow18-check.sh`, and the live-queue comparison.
6. Archive this design with its summary and open the PR (FLOW-12).

Files the PR changes: `tools/workflow/queue/queue.go`, `next.go`, `phases.go`
and `queue_test.go`; `docs/design/alters-slot.md`; and, with a2,
`.claude/settings.json` and `.entire/settings.json`. Nothing under
`docs/development/`, `tools/workflow/cmd/` or `tools/workflow/check/`.

After the merge, at `x4-close`, on the `workflow` branch (outside the PR):

- `README.md`, the `config.yaml` sample: the comment on
  `alters_relationships` becomes "FLOW-05: features holding an
  `alters_relationships` slot; 1 when unset".
- `README.md`, the `wf next` paragraph: "or a need for the
  `alters_relationships` slot another feature holds" becomes "or a Plan step
  that needs an `alters_relationships` slot when the limit's worth of
  features hold one", followed by: "A feature that declares
  `alters_relationships: true` holds a slot from the claim of its Plan step
  until it is done: while one of its Plan steps is done or carries a claim,
  expired or not. `wf release` on an unfinished Plan step gives it back.
  `wf next` and `wf validate` count the same holders against
  `limits.alters_relationships`. Process items neither hold a slot nor wait
  for one."
- `config.yaml`: the same comment change. The value stays 3.
- `dist/wf`: each cloud run and each scheduled native session rebuilds it
  from `main` as it starts; an attended session rebuilds it before its next
  `wf` command. Until then an older `wf` differs only in the states of cases
  4 to 8.

## Design review

None yet.

## Consolidation

A process item has no Publish phase and no consolidation pass (FLOW-03). The
implement step merges current `main` before it opens the PR.

## Rollback notes

None.

## Archive summary

Filled in when the PR is opened (FLOW-12).
