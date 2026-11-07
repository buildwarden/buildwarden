---
feature: jks-test-skip-without-jdk
status: draft
drivers: []
seams: []
surfaces: []
security_boundary: none
alters_relationships: false
design_review: skip
native_validation: []
tier2_gaps: []
rules: [TEST-01, TEST-02, TEST-04, GLOBAL-07, FLOW-11, FLOW-19]
rule_changes: []
concepts: [testing, jks, keytool, truststore, warden-io, ci, github-actions, java]
related: [unified-bootstrap.md, native-loop-followups.md, ci-path-filters.md]
---

# Keytool tests skip when keytool cannot run, and a CI job runs them with Java

## Problem

`cmd/warden-io/jks_test.go` `TestWriteJKSMatchesKeytool` compares the JKS
truststore that `writeJKS` produces with the one the JDK's `keytool` writes. It
skips only when `exec.LookPath("keytool")` fails. macOS always ships
`/usr/bin/keytool`, a stub that exits non-zero with "Unable to locate a Java
Runtime" when no JDK is installed. On such a Mac the lookup succeeds, `keytool
-importcert` fails, and the test fails instead of skipping.

So `make test` fails on the macbook, on `main` as well as on every branch, and
so does every done-when that runs it (Implement, FLOW-03). This now blocks
`vz-image-prepare-refresh`'s `f3-implement`. `native-loop-followups` recorded it
as environmental and did not file it. Jeff chose to skip the test rather than
install a JDK on the Mac.

A skip alone has a cost. The comparison runs on CI only because `ubuntu-latest`
happens to ship a JDK. Nothing guarantees that, and nothing runs it on macOS or
Windows. Jeff's direction (attended, 2026-11-07): "separate [the keytool tests]
as a separate CI job which installs Java on the platform as a part of test
setup."

## Prior art

- **`unified-bootstrap`** (Decision 6) introduced the pure-Go JKS writer and this
  byte comparison against `keytool`. This design keeps the comparison as it is.
- **`native-loop-followups`** noted the local failure (Validation notes,
  "Locally"). This design is the fix for that note.
- **`ci-path-filters`** gave `main.yml` its `changes` job, the `gotests` gate and
  the single required `ci-ok` aggregator. The new job reuses that gate and joins
  `ci-ok`'s `needs`, the pattern that design set for every job.
- **The `tier1-test` job** is `main.yml`'s existing `go test ./...` matrix over
  `ubuntu-latest`, `macos-latest` and `windows-latest` (GLOBAL-07). The new job
  uses the same matrix.
- **Opt-in test gates already exist** in the form `WARDEN_<AREA>=1`, for example
  `WARDEN_HYPERV_LIVE` and `WARDEN_HYPERV_LIVE_FETCH`. Those gates turn a test on.
  The new variable turns a skip into a failure, but it follows the same style.
- **TEST-01**: `make test` has no external dependencies and passes everywhere. A
  test that needs a JDK skips where there is none.
- **TEST-04**: a failure must be real signal.
- **TEST-02**: places tier 0 in the `ci` workflow, gated by the `changes` job's
  docs-only set. The new job is in that workflow, under that gate (see
  Decision 6).

## Decisions

1. **Probe whether keytool can run, and skip if it cannot.** After the lookup,
   the test runs `keytool -help`. If that exits non-zero, the test skips. The
   skip message names the path and includes the probe's output, which on the Mac
   is the stub's "Unable to locate a Java Runtime". There is no special case for
   `CI`.
   - *Alternative: match the macOS stub's message.* This is narrower and breaks
     if Apple rewords it. It also misses a keytool that is on PATH with no usable
     JVM, such as a broken Linux `alternatives` link. Rejected.
   - *Alternative: check `/usr/libexec/java_home` on darwin.* This only works on
     macOS and proves a JDK exists, not that this keytool runs. Rejected.
   - *Alternative: check `java -version` or `JAVA_HOME` instead of keytool.*
     This tests a different binary than the one the test runs. A machine can
     have a working `java` and a broken or mismatched `keytool` on PATH.
     Rejected.
   - *Alternative: a build tag (`//go:build keytool`).* A developer with a JDK
     would lose the check in `make test` unless they knew to pass the tag.
     Rejected. The dedicated job (Decision 2) gets CI coverage without the tag.
   - *Alternative: install a JDK on the Mac.* Jeff declined.
   - `keytool -help` exits 0 on JDK 9 and later (the cloud container's JDK was
     checked at d8d8a03) and writes no file, so the probe has no side effect.
2. **A dedicated `keytool` CI job does the real comparison (Jeff, 2026-11-07).**
   The job lives in `main.yml`, runs on the `tier1-test` matrix (`ubuntu-latest`,
   `macos-latest`, `windows-latest`) and installs a JDK with
   `actions/setup-java@v4`, using `distribution: temurin` and `java-version: '21'`
   (the current LTS). Pinning the distribution and major version keeps the
   reference `keytool` stable across runner image changes. On `macos-latest`,
   setup-java puts the real JDK's `bin` ahead of `/usr/bin` on PATH, so the stub
   is never reached. The job runs only the keytool tests:
   `go test ./cmd/warden-io -run Keytool -count=1 -v`, with
   `WARDEN_REQUIRE_KEYTOOL=1` set.
   - *Rejected: the earlier drafts' environment guards.* The first draft
     (4fa31a1) failed the test instead of skipping whenever `CI` was set. The
     revision at d8d8a03, implemented at a5ea356, keyed the same behavior on
     `GITHUB_ACTIONS=true`. Both kept the comparison only on the runner image
     that happened to have Java, and covered no platform beyond that. Jeff
     replaced them with the dedicated job.
3. **`WARDEN_REQUIRE_KEYTOOL=1` turns a skip into a failure.** When it is set, a
   keytool that is missing from PATH, or whose probe fails, fails the test with
   the same message the skip would show. So the dedicated job either compares the
   bytes or goes red; it can never pass by skipping. Why this name:
   - The `WARDEN_` prefix is the one the repo's test gates use.
   - `REQUIRE` says what it does: a requirement, not an opt-in to extra tests the
     way `*_LIVE` is.
   - `KEYTOOL` names the tool rather than a single test, so a future keytool test
     is covered by the same variable.
   - Generic `CI`, as Decision 2 shows, says nothing about whether a JDK was set
     up.
   The value `1` matches the other gates. Any other value, or no value, means
   "may skip".
4. **The existing `test` job keeps running the keytool test unchanged.** On
   `ubuntu-latest` it finds the runner's JDK and runs the comparison. Without
   one it skips.
   - *Alternative: exclude the keytool tests from `make test` in CI.* This would
     need a `-skip` pattern or a second variable in the Makefile or the job, so
     CI's `make test` would no longer be the command contributors run locally.
     The duplicate run on Ubuntu costs about a second. Rejected.
   - The guarantee comes from the dedicated job. The `test` job's run is a bonus
     that may silently become a skip if the image drops Java, and that is
     acceptable.
5. **The new job uses the `gotests` gate and joins `ci-ok`.** It runs under
   `if: needs.changes.outputs.gotests == 'true'`, the same gate as `test` and
   `tier1-test`. A narrower `cmd/warden-io`-only filter would add a new `changes`
   output for a job that takes under a minute plus JDK setup. It is listed in
   `ci-ok`'s `needs`, so it gates merges with no change to branch protection
   (FLOW-11).
6. **No rule changes.** I checked `docs/development/` for what adding a job must
   update:
   - **TEST-02** says tier 0 runs in "the `ci` workflow", on the triggers and
     docs-only set that `main.yml`'s `changes` job implements. It names the
     workflow and its gate, not its jobs. The new job is in that workflow under
     that gate, so the wording holds as written.
   - **TEST-01** still holds: `make test` keeps no external dependency, because
     without a JDK the test skips.
   - Nothing in `docs/development/` lists `main.yml`'s jobs, and no tier-1 or
     `tools/workflow` check reads them.
   `rule_changes` is therefore empty.

## Questions

### Blocking

None.

### Assumed

None. Jeff answered the CI approach directly (Decision 2), and the rest follows
from it.

## Design

**Test (`cmd/warden-io/jks_test.go`).** A helper finds a keytool that can run, or
decides the test cannot:

```
func requireKeytool(t) string:
    path, err := LookPath("keytool")
    if err: unavailable(t, "keytool not on PATH")
    out, err := run(path, "-help")
    if err: unavailable(t, "keytool at <path> cannot run (no Java runtime?): <err>\n<out>")
    return path

func unavailable(t, msg):
    if Getenv("WARDEN_REQUIRE_KEYTOOL") == "1": Fatal(msg + " (WARDEN_REQUIRE_KEYTOOL=1)")
    Skip(msg)
```

`TestWriteJKSMatchesKeytool` calls `requireKeytool` first. Its import,
byte-compare and `-list` steps are unchanged. The test name keeps `Keytool`, so
`-run Keytool` selects it. The comment above the test is updated to say:

- the test skips where no keytool runs;
- `WARDEN_REQUIRE_KEYTOOL=1` makes that a failure;
- the `keytool` CI job sets the variable and runs the test on all three
  platforms.

**CI (`.github/workflows/main.yml`).**

```
  # The keytool byte comparison, with a JDK on every platform (design
  # jks-test-skip-without-jdk). make test skips it where keytool cannot run.
  keytool:
    needs: changes
    if: needs.changes.outputs.gotests == 'true'
    runs-on: ${{ matrix.os }}
    strategy:
      matrix:
        os: [ubuntu-latest, macos-latest, windows-latest]
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version: stable }
      - uses: actions/setup-java@v4
        with: { distribution: temurin, java-version: '21' }
      - run: go test ./cmd/warden-io -run Keytool -count=1 -v
        env: { WARDEN_REQUIRE_KEYTOOL: '1' }
```

`ci-ok`'s `needs` gains `keytool`.

**Contributor docs.** In `CONTRIBUTING.md`, the paragraph after the command list
gains one sentence: the keytool comparison skips without a JDK, and CI's
`keytool` job runs it on every platform. No user-facing surface changes (UX-01).

## Cross-driver impact

None. The change touches one test in `cmd/warden-io`, the `ci` workflow and
`CONTRIBUTING.md`. It does not touch any driver tree, the `driver.Driver` /
`driver.Session` contract, or any shared package (ORCH-01). `writeJKS` and the
truststore it writes do not change.

## Validation plan

- **Tier 0, locally (macbook, stub keytool, no JDK).**
  - `go test ./cmd/warden-io -run Keytool -v` reports SKIP with the stub's
    message, and `make test` is green.
  - `WARDEN_REQUIRE_KEYTOOL=1 go test ./cmd/warden-io -run Keytool -v` FAILS
    with the same message. This is the job's fail-when-it-cannot-compare path,
    shown without a CI run.
- **CI on the PR.**
  - All three `keytool` matrix legs show
    `--- PASS: TestWriteJKSMatchesKeytool`, not SKIP, in their `-v` logs. That
    is the byte comparison against Temurin 21's keytool on Linux, macOS and
    Windows.
  - `ci-ok` lists `keytool` among its needs and is green.
  - The `test` job still passes on `ubuntu-latest`.
- **A first run on Windows or macOS may find a real platform difference**, for
  example a line-ending or path issue in the test, or a byte mismatch against
  that platform's keytool. That is a finding to fix in Implement, not a reason
  to drop the leg.
- **Tier 1 / 2.** Not affected.
- **Native validation.** None. The change is test and CI only and touches no
  driver. Implement runs `make test` on the macbook, the environment the skip is
  for.

## Implementation plan

Plan rewrites this section after the return. The return's rework (FLOW-06),
against what was implemented at e74e98c:

1. **Change the helper in `cmd/warden-io/jks_test.go`.** Replace `skipOrFail`'s
   `GITHUB_ACTIONS == "true"` check with `WARDEN_REQUIRE_KEYTOOL == "1"`, and add
   the variable's name to the failure message. The probe and the not-on-PATH
   path stay as implemented (Decisions 1 and 3).
2. **Rewrite the test's comment** to match the new behavior (see Design).
3. **Add the `keytool` job to `.github/workflows/main.yml`** and add it to
   `ci-ok`'s `needs` (Decisions 2 and 5).
4. **Add the `CONTRIBUTING.md` sentence** (see Design).
5. **Validate** as the Validation plan says.

The superseded plan (f2-plan at 1d51299) is in this file's history.

## Design review

- **2026-11-07, at Design (d8d8a03): skipped (FLOW-19).** Every skip criterion
  held.
- **2026-11-07, return r1 (Jeff's attended answer): skipped (FLOW-19).**
  - *Finding:* Jeff replaced the environment-variable guard, which made the test
    fail instead of skip on CI, with a dedicated CI job that installs Java on
    each platform: "separate [the keytool tests] as a separate CI job which
    installs Java on the platform as a part of test setup."
  - *What it changes:*
    - Decisions 2 to 6 are new or rewritten.
    - The test's fail-instead-of-skip switch becomes `WARDEN_REQUIRE_KEYTOOL=1`.
    - `main.yml` gains the `keytool` job.
    - `CONTRIBUTING.md` gains a sentence.
    - The Implementation plan's rework is listed above.
  - *Why the skip still holds:* the impact fields are unchanged:
    - `alters_relationships: false`;
    - no `seams`;
    - `security_boundary: none`;
    - no `native_validation`.

    In addition, no question is open, `rule_changes` is empty, and no rule in
    `docs/development/` changes (Decision 6). The CI approach is Jeff's own
    decision.

## Consolidation

Filled in at Publish.

## Rollback notes

None.

## Archive summary

Filled in at Publish.
