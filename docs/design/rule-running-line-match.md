---
feature: rule-running-line-match
status: draft
drivers: []
seams: []
surfaces: []
security_boundary: none
alters_relationships: false
design_review: required
native_validation: []
tier2_gaps: []
rules: [FLOW-03, FLOW-08, FLOW-09, FLOW-10, FLOW-12, FLOW-14, FLOW-15, FLOW-17, FLOW-19,
  GLOBAL-01, LINT-03, TEST-01, TEST-02, UX-01]
rule_changes: []
concepts: [workflow, rule-changes, enforcement, wf-check, ci-workflows, process-item]
related: [design-approved-rule-changes.md]
---

# The FLOW-14 clearance guards the CI jobs that run the rules, not every line naming them

## Problem

FLOW-14 (`docs/development/process.md:420-421`) makes "the lines of
`.github/workflows/` that run tier 1, `golangci-lint` or `tools/workflow`" a
rule change, which a feature may edit only under an approved `rule_changes`
entry. `wf check` enforces that in `tools/workflow/check/rulechange.go`:
`ruleRunningRe` (lines 25-27) is `tier1|golangci-lint|tools/workflow`, and
`ruleChangeFiles` (lines 129-160) marks a workflow file guarded when any added
or removed line of its `-U0` diff matches it (lines 154-156). That is a match
on names, not on what runs: a job key, a `needs:` entry or a YAML comment that
mentions one of the three is enough.

The incident. jks-test-skip-without-jdk (branch
`feat/jks-test-skip-without-jdk`) adds a `keytool` CI job in `501323a`
(2026-11-09) and, so that the required check covers it, changes `ci-ok`'s
`needs: [changes, lint, tier1, tier1-test, test, tidy, build, book]`
(`.github/workflows/main.yml:205`) to include `keytool`. The removed and added
`needs:` lines name the `tier1` job, so the clearance fails at the
f3-implement-r2 done-when (implementation complete at `216ce36`; the step was
released on 2026-11-09 at 11:10Z, queue history of jks-test-skip-without-jdk,
and digest `2026-11-09.md` lines 54-64, the FLOW-14 explanation at 61-64). No
rule and no enforcement changed: the new job runs
`go test ./cmd/warden-io -run Keytool`, and `ci-ok` runs the same `jq`.

The item's rule-change record was already confused. On 2026-11-07 the queue
was set to `design_review: required` (`9106126`, whose commit message records
a TEST-02 approval by Jeff) though the design declared no rule change; digest
`2026-11-08.md` lines 40-50 reported the mismatch, and Jeff set it back to
`design_review: skip` on 2026-11-08 at 19:03Z (`72e5474`: "no TEST-02 rule
change was meant"). That record predates the workflow edit and is not caused
by this check; it is context for why a false FLOW-14 failure on this item
must not be answered with another approval nobody meant.

The same name match also misses real enforcement edits. Changing the `tier1`
job's `fetch-depth: 0` (`main.yml:89`), which tier 1's `-base` diff depends
on, or its `BASE` expression (line 98), names none of the three words and
passes; a comment edit on a `setup-go` line that says "golangci-lint reads
this toolchain's export data" (lines 73 and 92) fails.

## Prior art

- **FLOW-14** (`docs/development/process.md:401-443`): the rule-change paths
  include "the lines of `.github/workflows/` that run tier 1, `golangci-lint`
  or `tools/workflow`"; the automated part runs at the Implement, Review and
  Publish done-when; a process item's branch is exempt and is the route for a
  change to `tools/workflow/` and for a rule change no feature needs.
- **`docs/design/design-approved-rule-changes.md`** (archived), which this
  item follows. Decision 1 (lines 79-81) scopes "the rule-running lines of
  `.github/workflows/`". Its Consolidation (lines 382-385) records the
  implementation's reading, "any added or removed workflow line that names
  `tier1`, `golangci-lint` or `tools/workflow`", and its Archive summary
  (lines 470-471) repeats it for future designs. This design replaces that
  reading; it keeps Decision 1's intent (other workflow edits stay ordinary
  code) and the rest of the clearance (`kindOf`, `cleared`, `Drops`)
  unchanged.
- **`docs/design/ci-path-filters.md`** (archived) introduced the `changes`
  classifier and `ci-ok` aggregator in `main.yml`; `lint` and `tier1` run
  after `changes`, and `lint` is skipped when `changes` says `code=false`
  (`main.yml:66-67`). `ci-ok` (lines 201-212) is the one required check.
- **LINT-03** (`docs/development/behavioral-linter.md:54`): tier 1 red blocks
  the release. In `release.yml` that is `release`'s
  `needs: [support-images, tier1]` (line 76).
- **The workflows today.** Jobs whose steps run one of the three:
  `main.yml` `lint` (lines 65-79) and `tier1` (lines 82-121); `release.yml`
  `tier1` (lines 57-73); `flow18.yml` `sessions-private`
  (`tools/workflow/flow18-check.sh`, line 22); `auto-tidy.yml` `auto-tidy`
  (`make lint`, line 21, which runs `golangci-lint` and tier 1,
  `Makefile:55-59`). Jobs that need one: `main.yml` `ci-ok` (`lint`,
  `tier1`) and `release.yml` `release` (`tier1`). `integration.yml` has a job
  *keyed* `tier1` (line 31) that runs `make integration-test` (TEST-02's
  tier 1, not the linter). `main.yml`'s `tier1-test` runs `go test` in
  `tools/tier1`, and `tidy` runs `go mod tidy` at the root and in
  `tools/tier1` (lines 159-162); neither runs a rule.
- **The YAML library.** `tools/workflow/go.mod` already requires
  `go.yaml.in/yaml/v3` v3.0.5 (the successor import path of
  `gopkg.in/yaml.v3`), imported by `tools/workflow/check/check.go:14` and
  `tools/workflow/queue/queue.go:19`. No new dependency.

## Decisions

Decisions 1-3 fix the false positive within FLOW-14's current scope (what
runs the rules). Widening that scope to the jobs and settings around them is
a separate choice, left to Jeff as a4 and not taken here.

1. **A workflow is read as YAML, and a job is *rule-running* when a step's
   `run:` or `uses:` value, or the job's own `uses:`, invokes one of them.**
   The pattern is `tier1|golangci-lint|tools/workflow|\bmake\s+(lint|enforcement)\b`.
   `make lint` and `make enforcement` are added because they run
   `golangci-lint` and tier 1 (`Makefile:55-59, 78-79`) without naming them.
   Before matching, whole-line shell comments (lines matching `^\s*#`) are
   removed from each `run:` value, so a script's comment does not make its
   job rule-running (`main.yml:46`, `# TestRealIndex (tools/tier1/rules) ...`
   inside `changes`' script, is the case today). Job keys, job and step
   `name:`, `needs:`, `if:`, `env:`, `with:`, `working-directory:` and YAML
   comments are not searched. On today's workflows this selects exactly the
   five jobs listed in Prior art.
   Rejected:
   - **Keep the line match, excluding `needs:` lines and job keys.** It still
     fires on YAML comments (`main.yml:73, 92`), on a `needs:` written as a
     block list or on a key it does not know, and it still misses the unnamed
     lines of a rule-running job (`fetch-depth`, `BASE`, `if:`).
   - **Match `run:` and `uses:` lines textually.** A `run: |` block spans
     lines whose continuation lines carry no key, so a line diff cannot tell
     which step a line belongs to.
   - **Search `run:` values with comments left in.** `changes` would be
     rule-running for a comment, and any future comment naming `tier1` in an
     ordinary job's script would guard that job: the incident in small.
   - **Also strip trailing comments (`cmd  # tier1`).** Telling a shell `#`
     comment from a `#` inside quotes or `${#x}` needs a shell parser. The
     residual false positive (a trailing comment naming a tool) is accepted;
     it fails closed, and the fix is to move the comment to its own line.
   - **Also search `working-directory:`.** It would make `tier1-test` and
     `tidy` (`working-directory: tools/tier1`) rule-running, though testing
     or tidying tier 1's module runs no rule.
2. **A rule-running job is guarded whole, by its body, not its key.** A file
   changes enforcement when the multiset of its rule-running jobs' bodies
   differs between the merge base and the branch. A body is the job's mapping
   decoded from YAML (anchors and aliases resolved) and re-encoded
   canonically (encoding/json, keys sorted), so YAML comments and formatting
   do not count and everything that shapes the run (every step with its
   `with:` and `env:`, the checkout's `fetch-depth`, the job's `if:`,
   `needs:`, `runs-on:`, `strategy:`, `permissions:`) does. A job key is not
   part of its body, so renaming a rule-running job alone, and updating the
   `needs:` lists that name it, is not a change.
   Rejected:
   - **Only the matching steps.** It misses `fetch-depth: 0` on the `tier1`
     job's checkout step (`main.yml:89`), the `setup-go` step's
     `go-version-file` (the toolchain whose export data `golangci-lint`
     reads, lines 73 and 92), and job-level `if:`, `needs:` and `runs-on:`
     (an `if: false` stops the job).
   - **Keying bodies by job name.** A rename that moves no step would need a
     rule change; the key runs nothing.
3. **No job may stop needing a rule-running job.** For every job of a file,
   the pairs (that job's key, the body of a rule-running job it lists in
   `needs:`) on the merge base must all be present on the branch; pairs may be
   added. This guards the gates that consume the rules' results: `ci-ok`
   needing `lint` and `tier1` (the required check, `main.yml:205`) and
   `release` needing `tier1` (LINT-03, `release.yml:76`). The incident passes,
   because adding `keytool` drops no pair; dropping `tier1` from `ci-ok` or
   from `release` fails, as it does under the line match today. The
   downstream job's key is part of the pair because a gate's name is what
   branch protection requires (`ci-ok`); the needed job is identified by
   body, so Decision 2's rename passes. The rest of a downstream job's body
   (`ci-ok`'s `jq` step) stays ordinary code, as it is today.
   Rejected:
   - **Leave downstream jobs entirely ordinary.** It would open a hole the
     line match closes: removing `tier1` from a `needs:` line removes a line
     that names it, which is how the incident itself failed. Dropping the
     release gate would then pass.
   - **Guard downstream jobs whole.** Adding any job to `ci-ok`'s `needs:` is
     the incident again.
   - **Compare the needed bodies as one multiset, without the downstream
     key.** `ci-ok` dropping `tier1` while a new job adds it would pass.
4. **Added, deleted and unparseable files.** A file missing on one side
   counts as having no jobs there, so adding or deleting a workflow with a
   rule-running job is guarded. A `.yml` or `.yaml` file under
   `.github/workflows/` that does not parse on either side fails the
   clearance with the file and the parser's message (fail closed, and says
   why); GitHub would reject that file anyway. Other files under the
   directory are ordinary. The base side is the merge base of `origin/main`
   and the branch, as the current `base...branch` diff already uses.
   Rejected: treating an unparseable file as guarded with the usual "is in
   no approved rule change" message, which hides the cause.
5. **FLOW-14's guarded scope is amended from lines to jobs.** In
   `docs/development/process.md:420-421`, "and the lines of
   `.github/workflows/` that run tier 1, `golangci-lint` or `tools/workflow`"
   becomes "and the jobs of `.github/workflows/` that run tier 1,
   `golangci-lint` or `tools/workflow`, and the `needs:` entries that name
   them". This changes what the rule covers, not only its words: it drops
   lines that merely name a tool and adds the unnamed keys of a rule-running
   job (the Security note lists both). It goes through the process route
   (FLOW-14: "the route for a rule change no feature needs"), so
   `rule_changes` stays empty; severity and check type are unchanged, and the
   design review (`design_review: required`) is where Jeff approves the new
   scope. The archived design's Consolidation and Archive summary stay as
   written (archived designs are history); this design's Archive summary
   supersedes their reading.
   Rejected: leaving "lines". The old reading took it literally, and a rule
   that no longer describes what its check enforces misleads the next design
   that relies on it.

## Questions

### Blocking

None.

### Assumed

- **a1. What makes a job rule-running** (Decision 1). Proceeding with a
  step's `run:` (whole-line comments removed) or `uses:`, or the job's
  `uses:`, matching
  `tier1|golangci-lint|tools/workflow|\bmake\s+(lint|enforcement)\b`.
  Options:
  - (a) as stated;
  - (b) the same without removing comments (`changes` becomes rule-running
    through its `# TestRealIndex (tools/tier1/rules)` comment);
  - (c) any scalar in the job's steps, `working-directory:` and `with:`
    included (guards `tier1-test` and `tidy` too).

  Recommended: (a).
- **a2. What a change to a rule-running job is** (Decision 2). Proceeding
  with any change to its canonical body; job keys excluded. Options:
  - (a) as stated;
  - (b) only the matching steps' bodies (misses the checkout's `fetch-depth`,
    `setup-go`'s `go-version-file` and job-level `if:`/`needs:`/`runs-on:`);
  - (c) whole bodies keyed by job name, so a rename alone is a rule change.

  Recommended: (a).
- **a3. Jobs downstream of a rule-running job** (Decision 3). Proceeding with
  guarding only their `needs:` pairs on rule-running jobs: a job may add but
  never drop one, so `ci-ok` and `release.yml`'s `release` (LINT-03) keep
  needing `tier1`. Options:
  - (a) as stated;
  - (b) leave them ordinary code (dropping `tier1` from `ci-ok` or `release`
    would pass, which fails today);
  - (c) guard their bodies whole (the incident fails again).

  Recommended: (a); it keeps every protection the line match gives on these
  jobs and adds nothing a feature routinely edits.
- **a4. Scope beyond the jobs that run the rules** (a scope change to FLOW-14,
  not part of the fix). Proceeding with none: the jobs a rule-running job
  needs (`changes`, whose outputs gate `lint`'s `if:`), the workflow-level
  keys (`on:` with its triggers and `paths:`, `env:`, `permissions:`,
  `concurrency:`) and `ci-ok`'s `jq` step stay ordinary code, as they are
  today unless a changed line happens to name a tool. Options:
  - (a) as stated;
  - (b) also guard, transitively, the jobs a rule-running job needs and the
    top-level keys of a file with a rule-running job. Then a feature that
    adds a path class to the `changes` classifier (`main.yml:42-59`) or edits
    `main.yml`'s `on:` needs an approved rule change;
  - (c) (b) plus downstream jobs' steps (`ci-ok`'s `jq`).

  Recommended: (a). The incident is a false positive, and the fix should not
  add approvals for the classifier edits features make routinely; each of
  these gaps is visible on the PR (a skipped `lint`, a missing run) to the
  human merging (FLOW-08). If Jeff wants (b), it is its own process item.
- **a5. FLOW-14's wording** (Decision 5). Proceeding with amending the
  parenthesis to name jobs and the `needs:` entries that name them, through
  this process item. Options:
  - (a) as stated;
  - (b) keep "lines" and record the reading only in this design;
  - (c) move the definition of a guarded workflow edit to a comment in
    `rulechange.go` and have FLOW-14 cite it.

  Recommended: (a).

## Design

`tools/workflow/check/workflows.go` (new, package `check`):

```
workflowRunsRe = tier1|golangci-lint|tools/workflow|\bmake\s+(lint|enforcement)\b

workflowGuardChanged(old, new []byte) (bool, error):
  a, err := guardedParts(old)   // old nil: empty parts
  b, err := guardedParts(new)
  if multiset(a.bodies) != multiset(b.bodies): return true
  if a.edges is not a subset of b.edges: return true
  return false, nil

guardedParts(text) -> {bodies []string, edges set[(string, string)]}:
  decode text into map[string]any (yaml v3); error -> return it
  jobs := doc["jobs"] as map[string]map[string]any
  runs(job) := job.uses matches, or any step.uses matches, or any step.run
      with lines matching ^\s*# removed matches workflowRunsRe
  body(k) := canonical JSON of jobs[k]
  bodies := {body(k) | runs(jobs[k])}
  edges := {(j, body(k)) | k in jobs[j].needs (string or list), runs(jobs[k])}
```

`ruleChangeFiles` keeps its `--name-only` pass and replaces the `-U0` patch
scan (`rulechange.go:140-158`) with: `git merge-base base branch`; `git diff
--name-status --no-renames base...branch -- .github/workflows/`; for each
`.yml`/`.yaml` path, the old text from `git show <merge-base>:<path>` unless
the status is `A`, and the new from `git show <branch>:<path>` unless it is
`D`; `workflowGuardChanged`; true marks the file `rulePath`, an error returns
`"<path>: <error>"`, which `ruleClearance` already reports as a failure
(`rulechange.go:172-176`). `ruleRunningRe` and its comment go; `kindOf`'s
comment (lines 38-39) names `workflowGuardChanged`.

`docs/development/process.md:420-421`: Decision 5's wording.

No other surface: `wf check`'s output names are unchanged, `ruleClearance`,
`cleared`, `Drops` and `enforcement-check.sh` are untouched. A branch whose
workflows were cleared before stays cleared; one that only named a tool now
passes outright; one that changes an unnamed key of a rule-running job now
needs an approval it did not before.

The follow-on. With this on `main`, jks-test-skip-without-jdk's
f3-implement-r2 done-when passes unchanged: its branch changes
`.github/workflows/main.yml`, `CONTRIBUTING.md`, `cmd/warden-io/jks_test.go`
and its design (`git diff --name-only` from the merge base to `216ce36`); in
`main.yml` the new `keytool` job is not rule-running, the `lint` and `tier1`
bodies are unchanged, and `ci-ok`'s pairs on `lint` and `tier1` are kept. No
rebase or edit of that branch is needed; x4-close notes it in that item's
queue history.

## Cross-driver impact

None: `drivers: []`. The change is loop tooling (`tools/workflow/check/`) and
one rule's wording; no driver, relay, ledger or collector code changes.

## Security note

`security_boundary: none`. The clearance is a guard against a feature
changing enforcement without the human's approval; branch protection on
`main` and the human's merge (FLOW-08) are the boundary. The change both
narrows and widens what it guards:

- **Narrower.** Lines that only name a tool: additions to a `needs:` list,
  job keys, YAML comments, whole-line comments in a `run:` script (`changes`'
  `# TestRealIndex (tools/tier1/rules)`), and the `tier1-test` and `tidy`
  jobs' `working-directory: tools/tier1`. None of them runs a rule.
- **Wider.** Every key of a rule-running job (`fetch-depth`,
  `go-version-file`, `if:`, `env:`, `runs-on:`) and `make lint`/`make
  enforcement` steps (`auto-tidy.yml`). Each could stop or weaken a tier 1 or
  `golangci-lint` run and passed the line match.
- **Kept.** A job may not stop needing a rule-running job (Decision 3):
  dropping `tier1` from `ci-ok`'s `needs:` (`main.yml:205`) or from
  `release.yml`'s `release` (line 76, the LINT-03 release gate) fails, as it
  does today because the removed line names `tier1`.
- **Unchanged gaps (a4).** The `changes` classifier, which gates `lint`
  through its outputs, the workflow-level keys (`on:`, `paths:`,
  `permissions:`) and `ci-ok`'s `jq` step stay ordinary code; today they are
  guarded only when a changed line happens to name a tool. A feature could
  make `changes` skip `lint` or drop a trigger without a rule change. The
  effect is visible on the PR (a skipped `lint`, a missing run) to the human
  merging (FLOW-08). Closing these is the scope change a4 (b) describes.
- **Parsing.** `go.yaml.in/yaml/v3` decodes only workflow files from the
  repository's own history, in the loop's checkout; an unparseable file fails
  closed (Decision 4). Anchors and aliases are resolved before comparison, so
  an alias cannot hide a change.

## Validation plan

- **Tier 0** (TEST-01, TEST-02), `make test` (`Makefile:33-36`):
  - `tools/workflow/check/workflows_test.go`, table-driven over
    `workflowGuardChanged` with a `main.yml`-shaped fixture (`changes`,
    `lint`, `tier1`, `tier1-test`, `ci-ok`):
    - the incident: `keytool` job added and listed in `ci-ok`'s `needs:` →
      unchanged;
    - `ci-ok`'s `needs:` rewritten as a block list → unchanged;
    - a comment edit on `lint`'s `setup-go` line → unchanged;
    - job key `tier1:` renamed to `tier1-static:` with `ci-ok`'s `needs:`
      updated → unchanged (Decision 2);
    - a new step `run: ./dist/tier1 -base x` in a non-rule-running job →
      changed;
    - a new job whose step is `uses: golangci/golangci-lint-action@v8` →
      changed;
    - `golangci-lint` version changed in `lint` → changed;
    - `fetch-depth: 0` changed in `tier1` → changed;
    - `tier1` dropped from `ci-ok`'s `needs:` → changed (Decision 3);
    - a `release.yml`-shaped file: `tier1` dropped from `release`'s
      `needs:` → changed; `support-images` dropped → unchanged;
    - a new job that needs `tier1` → unchanged;
    - a whole-line comment naming `tier1` added to a non-rule-running job's
      `run: |` script → unchanged (Decision 1);
    - `changes`' script changed, and `on.pull_request.types` changed →
      unchanged (a4 (a));
    - `make lint` step edited in an `auto-tidy`-shaped file → changed;
    - an `integration.yml`-shaped file: job keyed `tier1` running
      `make integration-test`, its steps and `on.pull_request.paths` edited
      → unchanged;
    - added and deleted file with a rule-running job (`old` or `new` nil) →
      changed; with none → unchanged;
    - unparseable YAML on either side → error.
  - `tools/workflow/check/rulechange_test.go`, `TestRuleClearance`: the
    fixtures at lines 122-123 and 150-156 become workflow-shaped
    (`jobs: {build: {steps: [{run: make build}]}}` and a job whose step runs
    `./dist/tier1 -base x`), keeping both expectations. A new case commits a
    baseline `.github/workflows/main.yml` to `main` (merged into
    `feat/demo`), then replays the incident on the branch and expects the
    FLOW-14 result to pass with no `rule_changes`, then adds a `tier1` step
    and expects "`.github/workflows/main.yml` is in no approved rule change".
- `make lint` (FLOW-17; no build-tagged file is touched), `make test`,
  `make enforcement` (this branch changes `tools/workflow/check/`, so
  Decision 6 of the followed design runs; tier 1's findings are unchanged),
  `dist/wf check -feature rule-running-line-match`.
- **Replay at x2-implement:** `dist/wf check -feature
  jks-test-skip-without-jdk` built from this branch, against
  `feat/jks-test-skip-without-jdk` at `216ce36`: the FLOW-14 result passes.
  Recorded in Consolidation.
- No tier 1 integration or tier 2 change; no native validation (FLOW-09): the
  check is platform-independent Go run by the cloud loop.

## Implementation plan

Concrete at x2-implement (process item, FLOW-03); one commit each:

1. `tools/workflow/check/workflows.go` (new; FLOW-14, GLOBAL-01):
   `workflowRunsRe`, `guardedParts` and `workflowGuardChanged` as in Design,
   with comments saying why each part is guarded (Decisions 1-4).
2. `tools/workflow/check/rulechange.go` (FLOW-14, GLOBAL-01): replace the
   `-U0` patch scan in `ruleChangeFiles` with the merge-base, `--name-status`
   and `git show` reads feeding `workflowGuardChanged`; drop `ruleRunningRe`;
   update the `workflowsDir` and `kindOf` comments.
3. `tools/workflow/check/workflows_test.go` (new) and
   `tools/workflow/check/rulechange_test.go` (TEST-01, TEST-02): the tier-0
   cases in the Validation plan.
4. `docs/development/process.md` (FLOW-14): Decision 5's wording at lines
   420-421; no severity or check change.
5. `docs/book/src/concepts/overview.md` and
   `docs/book/src/contributing/structure.md` (FLOW-15, UX-01): checked, no
   change expected (neither describes `wf check`'s files or the clearance).
6. The replay against `216ce36`, recorded in Consolidation; `make lint`,
   `make test`, `make enforcement`, `tools/workflow/flow18-check.sh`,
   `dist/wf validate` and `dist/wf check -feature rule-running-line-match`
   (FLOW-10, FLOW-17); Consolidation filled. Archive (FLOW-12) and the PR
   (FLOW-08) are Publish's.
7. At x4-close, on the `workflow` branch (FLOW-03, FLOW-09): note in
   jks-test-skip-without-jdk's history that f3-implement-r2 is unblocked and
   its done-when should pass unchanged.

## Design review

- **r1, 2026-11-09: approved at `647457a`** (Jeff, workflow-human,
  x1-design, FLOW-19). The design was presented on a review page with the
  assumptions first. **a1 to a5 are confirmed as drafted, option (a).**

## Consolidation

x2-implement, 2026-11-10 (FLOW-10, FLOW-15).

- **What landed.** `tools/workflow/check/workflows.go` (new):
  `workflowRunsRe`, `guardedParts`, `runsRules` and `workflowGuardChanged`
  as in Design. `rulechange.go`: `ruleChangeFiles` reads each changed
  `.yml`/`.yaml` under `.github/workflows/` at the merge base and on the
  branch (`git diff --name-status -z`, `git show`) and passes both to
  `workflowGuardChanged`; `ruleRunningRe` is gone; the `workflowsDir` and
  `kindOf` comments name the new guard. An unparseable side fails the
  clearance as `<path>: merge base: <parser message>` (or `branch:`).
  `process.md` FLOW-14 carries Decision 5's wording, severity and check
  unchanged.
- **One departure, within the design.** The `-z` form of `--name-status` is
  used so no path is quoted; the Design names `--name-status` without it.
  A lint pass (`lll`) added a fifth commit wrapping lines in the new files.
- **Tests.** `workflows_test.go`: every Validation plan case over
  `main.yml`-, `release.yml`-, `auto-tidy.yml`- and `integration.yml`-shaped
  fixtures, plus an anchor/alias case; added and deleted files have their own
  test, unparseable sides theirs. `rulechange_test.go`: the
  `TestRuleClearance` workflow fixtures are workflow-shaped with the same
  expectations; `TestRuleClearanceWorkflowJobs` commits a baseline
  `main.yml` on `main`, replays the incident (passes with no
  `rule_changes`), adds a `tier1` step (fails, "is in no approved rule
  change") and an unparseable file (fails, naming it).
- **Today's workflows.** Run over `.github/workflows/` at `49e5354`, the
  guard selects exactly the five jobs of Prior art: `main.yml` `lint` and
  `tier1` (two `needs:` pairs, from `ci-ok`), `release.yml` `tier1` (one,
  from `release`), `flow18.yml` `sessions-private`, `auto-tidy.yml`
  `auto-tidy`. `changes` (its `# TestRealIndex (tools/tier1/rules)`
  comment), `tier1-test`, `tidy` and `integration.yml`'s `tier1` are not.
- **Replay.** `wf check -feature jks-test-skip-without-jdk` against
  `feat/jks-test-skip-without-jdk` at `216ce36`: with `wf` built from this
  branch, "ok rule changes cleared (FLOW-14): the branch changes no rule or
  its enforcement"; with `wf` built from `origin/main` (`49e5354`), "FAIL
  ... `.github/workflows/main.yml` is in no approved rule change". The
  other phase-3 results there (checkout not on that branch, `make` skipped
  without `-run`) are the replay's setup, not the item's.
- **Gates.** `make lint` (golangci-lint 2.15, go1.26; 0 issues, tier 1 0
  findings), `make test`, `make enforcement` ("main's tier 1 findings kept":
  0 dropped), `tools/workflow/flow18-check.sh`, `(cd tools/workflow && go
  test ./...)` and `wf validate` pass.
- **Docs (FLOW-15, UX-01).** `docs/book/src/concepts/overview.md` and
  `docs/book/src/contributing/structure.md` checked: neither describes
  `wf check`'s files or the clearance, so no change. No other doc,
  agent definition or skill restates the old "lines" reading outside
  archived designs, which stay as written (Decision 5).
- **Overlap.** PR #104 (`feat/design-principles`) edits `process.md`'s
  FLOW-05 and FLOW-19 and `check.go`'s `design()` section list; this branch
  edits only FLOW-14's parenthesis and does not touch `check.go`, so either
  can merge first.

## Rollback notes

None yet.

## Archive summary

Filled in at Publish (FLOW-12).
