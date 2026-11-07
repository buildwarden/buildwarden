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
rules: [TEST-01, TEST-04, ORCH-01, FLOW-19]
rule_changes: []
concepts: [testing, jks, keytool, truststore, warden-io]
related: [unified-bootstrap.md, native-loop-followups.md]
---

# TestWriteJKSMatchesKeytool skips when keytool cannot run

## Problem

`cmd/warden-io/jks_test.go` `TestWriteJKSMatchesKeytool` compares the JKS
truststore that `writeJKS` produces with the one the JDK's `keytool` writes. It
skips only when `exec.LookPath("keytool")` fails. macOS always ships
`/usr/bin/keytool`, a stub that exits non-zero with "Unable to locate a Java
Runtime" when no JDK is installed. On such a Mac the lookup succeeds, `keytool
-importcert` fails, and the test fails instead of skipping.

So `make test` fails on the macbook, on `main` as well as on every branch, and
every done-when that runs it (Implement, FLOW-03) fails there too. This now
blocks `vz-image-prepare-refresh`'s `f3-implement`. `native-loop-followups`
recorded it as environmental and did not file it. Jeff chose to skip the test
rather than install a JDK on the Mac.

## Prior art

- `unified-bootstrap` (Decision 6) introduced the pure-Go JKS writer and this
  byte comparison against `keytool`. This design keeps the comparison and changes
  only when the test skips.
- `native-loop-followups` noted the local failure (its Validation notes, "Locally").
  This design is the fix for that note.
- TEST-01: `make test` runs on every supported platform with no external
  dependencies. A test that needs a JDK must skip where there is none, not fail.
- TEST-04: a failure must be real signal. Today the Mac failure is about the
  environment, not the code.

No other design in `docs/design/` shares these `concepts`.

## Decisions

1. **Probe whether keytool can run, and skip if it cannot.** After the lookup,
   the test runs `keytool -help`. If that exits non-zero, the test skips. The
   skip message names the path and includes the probe's output, which on the Mac
   is the stub's "Unable to locate a Java Runtime".
   - *Alternative: match the macOS stub's message.* This is narrower and breaks
     if Apple rewords the message. It also misses other machines where a keytool
     is on PATH but no JVM is usable, such as a broken Linux `alternatives` link.
     Rejected.
   - *Alternative: check `/usr/libexec/java_home` on darwin.* This only works on
     macOS and only proves a JDK exists, not that this keytool runs. Rejected.
   - *Alternative: check `java -version` or `JAVA_HOME` instead of keytool.*
     This tests a different binary than the one the test runs. On the Mac
     `java` is the same kind of stub, so it would work there, but a machine can
     have a working `java` and a broken or mismatched `keytool` on PATH, or a
     `JAVA_HOME` that points elsewhere than the keytool found. Probing the very
     binary the test then runs is the only check that answers "can this test
     run here". Rejected.
   - *Alternative: a build tag (e.g. `//go:build keytool`) so the test runs only
     when asked for.* `make test` would never run the comparison unless CI
     passed the tag, which needs a change to `.github/workflows/main.yml` and
     `Makefile`, and a developer with a JDK would lose the check locally. It
     trades a missing skip for a missing test. Rejected.
   - *Alternative: install a JDK on the Mac.* Jeff declined.
   - `keytool -help` exits 0 on JDK 9 and later (checked here: exit 0 on the
     cloud container's JDK) and writes no file, so the probe has no side effect.
2. **Never skip on GitHub Actions.** If keytool is not on PATH or the probe
   fails while `GITHUB_ACTIONS=true`, the test fails and shows why. The only
   job that runs this test is `ci`'s `test` job (`make test` on
   `ubuntu-latest`); that image ships a JDK (there is no `setup-java` step, so
   the current comment "CI installs a JDK" is corrected to say the image has
   one). A runner image that loses its JDK, or a keytool whose `-help` stops
   exiting 0, must not quietly turn the only real comparison into a skip. This
   covers the not-on-PATH case too, which the draft left as a silent skip.
   - *Alternative: key on `CI`.* GitHub sets `CI=true`, but so do many other
     tools and agent harnesses to suppress prompts, on developer machines that
     may have no JDK; there the test would fail for the reason this design
     removes. `GITHUB_ACTIONS` is set only by the Actions runner. Rejected.
   - *Alternative: an explicit opt-in variable (e.g. `WARDEN_REQUIRE_KEYTOOL=1`)
     set in the `test` job.* Clearer intent, but it edits CI configuration for
     a test-only fix, and a new job that runs `make test` would have to
     remember it. Rejected; `GITHUB_ACTIONS` needs no configuration.
   - *Alternative: skip everywhere, rely on someone noticing the SKIP.* Hides
     the loss of the only real comparison (TEST-04). Rejected.
3. **Leave the comparison itself unchanged.** If keytool runs, the test behaves
   exactly as today: a failing `-importcert`, a byte mismatch, or `-list`
   rejecting our store fails the test.

## Questions

### Blocking

None.

### Assumed

None.

## Design

`TestWriteJKSMatchesKeytool` becomes:

```
skipOrFail(msg) := if env GITHUB_ACTIONS == "true": Fatal(msg)  ; CI must run the comparison
                   else: Skip(msg)
keytool, err := LookPath("keytool")
if err != nil: skipOrFail("keytool not on PATH")
out, err := run(keytool, "-help")
if err != nil:
    skipOrFail("keytool at <path> cannot run (no Java runtime?): <err>\n<out>")
... existing importcert / byte compare / -list, unchanged
```

Update the comment above the test so it says the test runs where a working
keytool is installed, and fails rather than skips on GitHub Actions, whose
`ubuntu-latest` image ships a JDK. The probe may move into
a small helper in the same test file. No production code changes.

## Cross-driver impact

None. The change is limited to one test in `cmd/warden-io`. It touches no driver
tree, the `driver.Driver` / `driver.Session` contract, or any shared package
(ORCH-01). `writeJKS` and the truststore it writes do not change.

## Validation plan

- **Tier 0.** On the macbook (stub keytool, no JDK), `go test ./cmd/warden-io
  -run TestWriteJKSMatchesKeytool -v` reports SKIP with the stub's message, and
  `make test` is green. With `GITHUB_ACTIONS=true` set, the same run fails;
with only `CI=1` set, it still skips. Where a working JDK exists (the cloud
container has one), the test runs and passes: the skip does not hide a real
failure. On CI
  (`ubuntu-latest`, with a JDK), the test runs and passes as before. The PR's
  `test` job log shows it as PASS, not SKIP.
- **Tier 1 / 2.** Not affected.
- **Native validation.** None. The change is test-only and touches no driver.
  Implement runs `make test` on the macbook, which is the environment the fix is
  for.

## Implementation plan

1. `cmd/warden-io/jks_test.go` `TestWriteJKSMatchesKeytool`: add a
   `skipOrFail` helper in the test file that calls `t.Fatal` when
   `GITHUB_ACTIONS == "true"` and `t.Skip` otherwise; route the existing
   not-on-PATH skip through it; after the lookup, run `keytool -help` and on a
   non-zero exit call `skipOrFail` with the path, the error and the probe's
   output. The `-importcert` / byte compare / `-list` steps stay unchanged
   (Decisions 1-3; TEST-01, TEST-04, ORCH-01).
2. Rewrite the comment above the test: it runs where a working keytool is
   installed, skips elsewhere, and fails rather than skips on GitHub Actions,
   whose `ubuntu-latest` image ships a JDK (replacing "CI installs a JDK")
   (Decision 2; TEST-01).
3. Tier 0 checks (TEST-02, TEST-01): in the cloud container (working JDK),
   `go test ./cmd/warden-io -run TestWriteJKSMatchesKeytool -v` passes; with a
   stub `keytool` that exits non-zero first on PATH, it skips with the stub's
   output, still skips with only `CI=1`, and fails with `GITHUB_ACTIONS=true`;
   `make test` and `make lint` are green (FLOW-17). The macbook SKIP and the
   PR `test` job's PASS are confirmed where the Validation plan places them
   (Implement on the macbook, the PR's CI); no `f6-validate-<host>` step is
   added (native_validation is empty).

No rework of other implemented code, no rule changes, no reference-doc
(UX-01) or `overview.md` (FLOW-15) update: the change is test-only and alters
no user-facing behavior or component flow.

## Design review

2026-11-07: skipped (FLOW-19), decided at Design. Every skip criterion holds: `alters_relationships: false`, no
`seams`, `security_boundary: none`, no blocking `native_validation`, no assumed
question, no `rule_changes`, and the branch changes nothing in
`docs/development/`. The change is one test in one file, with no production,
CI or driver code touched, and both alternatives that would have touched CI
configuration were rejected. Listed in the digest so the human can call it
back.

## Consolidation

Filled in at Publish.

## Rollback notes

None.

## Archive summary

Filled in at Publish.
