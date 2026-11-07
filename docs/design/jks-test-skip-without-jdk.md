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
rules: [TEST-01, TEST-04, FLOW-19]
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
   - *Alternative: install a JDK on the Mac.* Jeff declined.
2. **Never skip on CI.** If the probe fails while `CI` is set (GitHub Actions
   sets `CI=true`), the test fails and shows the probe output. CI runs
   `make test` on `ubuntu-latest`, which has a JDK, so the comparison keeps
   running there. A runner image that loses its JDK, or a keytool whose `-help`
   stops exiting 0, must not quietly turn the only real comparison into a skip.
   The `CI` variable needs no new configuration.
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
keytool := LookPath("keytool")        ; not found → Skip("keytool not on PATH")
out, err := run(keytool, "-help")
if err != nil:
    msg := "keytool at <path> cannot run (no Java runtime?): <err>\n<out>"
    if env CI != "": Fatal(msg)       ; CI must run the comparison
    Skip(msg)
... existing importcert / byte compare / -list, unchanged
```

Update the comment above the test so it says the test runs where a working
keytool is installed and fails rather than skips on CI. The probe may move into
a small helper in the same test file. No production code changes.

## Cross-driver impact

None. The change is limited to one test in `cmd/warden-io`. It touches no driver
tree, the `driver.Driver` / `driver.Session` contract, or any shared package
(ORCH-01). `writeJKS` and the truststore it writes do not change.

## Validation plan

- **Tier 0.** On the macbook (stub keytool, no JDK), `go test ./cmd/warden-io
  -run TestWriteJKSMatchesKeytool -v` reports SKIP with the stub's message, and
  `make test` is green. With `CI=1` set, the same run fails. On CI
  (`ubuntu-latest`, with a JDK), the test runs and passes as before. The PR's
  `test` job log shows it as PASS, not SKIP.
- **Tier 1 / 2.** Not affected.
- **Native validation.** None. The change is test-only and touches no driver.
  Implement runs `make test` on the macbook, which is the environment the fix is
  for.

## Implementation plan

Filled in at Plan (FLOW-03).

## Design review

Skipped (FLOW-19). Every skip criterion holds: `alters_relationships: false`, no
`seams`, `security_boundary: none`, no blocking `native_validation`, no assumed
question, no `rule_changes`, and the branch changes nothing in
`docs/development/`.

## Consolidation

Filled in at Publish.

## Rollback notes

None.

## Archive summary

Filled in at Publish.
