# The Behavioral Linter

The behavioral linter is the enforcement arm of this doc set: an agent (plus a
deterministic tier) that checks a change against the standards, runs per-PR and
before cutting a release from trunk, and keeps the project's standards from
eroding.

See [`README.md`](./README.md) for the rule format.

## The load-bearing decision

**The LLM agent must not hold a hard veto over trunk.** It is tempting to let the
agent be the gate; do not. A non-deterministic model holding a silent veto either
blocks good releases on false positives or waves through regressions on false
negatives, and it is not reproducible. Deterministic checks do the blocking; the
agent informs a human who signs off. This is what keeps releases trustworthy and
the gate auditable.

---

### LINT-01 — Every rule is machine-consumable

**Severity:** MUST
**Check:** automated

Each standard carries an `ID`, a `severity` (MUST / MUST-review / SHOULD), a
`check-type` (automated / agent-judgment / hybrid), and a one-line "how to check".
A rule with no check-type is a documentation bug. The rule index is extractable
(parse the `### <ID> — <title>` headers plus the `**Severity:**` / `**Check:**`
lines) so a human and the agent read the exact same source. This is the mechanism
that lets the docs double as the linter's ruleset.

---

### LINT-02 — Two tiers, every rule mapped to one

**Severity:** MUST
**Check:** automated

- **Tier 1, deterministic:** `golangci-lint` + `go test` + custom static checks for
  the `automated`-tagged rules. Examples: a new exported symbol without a doc
  comment (GLOBAL-01); a cgo import under `cmd/warden-io` (EXEC-01); a ledger write
  outside `loop()` (RELAY-01); a surface change with no docs diff (UX-01, ORCH-03);
  a schema change with no doc change (RELAY-05); an input mutation not landing under
  `inputs/` (ORCH-02).
- **Tier 2, judgment:** the agent evaluates the `agent-judgment` rules against the
  change.

Hybrid rules split: the mechanical half runs in tier 1, the judgment half in tier 2.
Implementing the tier-1 checks is Deep-dive #2.

---

### LINT-03 — Release-gate policy: deterministic hard-fail, agent advisory-with-teeth

**Severity:** MUST
**Check:** automated

Tier 1 red blocks the release outright. Tier 2 MUST-severity findings require
explicit human sign-off (acknowledge, or dismiss with a recorded reason) before the
release proceeds; SHOULD findings are informational. The agent never auto-blocks and
never auto-approves trunk.

The sign-off gate needs a concrete home (a required PR check a human toggles, a
release-checklist item, or a branch-protection rule), tied to how releases are cut
today (GoReleaser on a tag push). That wiring is decided at implementation time.

---

### LINT-04 — Scoped to the change

**Severity:** MUST
**Check:** automated

The linter runs on a diff, never the whole repo: the PR diff per-PR, and
`last-release-tag..HEAD` at release. This keeps it from re-flagging pre-existing debt
and keeps it cheap and focused, the same spirit as gofmt-on-touched-lines (GLOBAL-02).

---

### LINT-05 — Reproducible, auditable output

**Severity:** MUST
**Check:** automated

Pinned model and low temperature. A stable, diffable structured report (machine JSON
plus human markdown, per UX-02) keyed by rule ID and `file:line`, with counts by
severity. Findings are suppressible with a recorded reason so a re-run does not nag on
an already-adjudicated item.

---

### LINT-06 — Invocation

**Severity:** SHOULD
**Check:** agent-judgment

A repo-local agent under `.kiro/agents/`, triggered two ways: per-PR (fixes are
cheapest there) and pre-release from trunk (the diff since the last release tag). The
deterministic tier is plain CI; the agent tier is handed the diff plus the extracted
rule index.

---

### LINT-07 — Bootstrap in report-only mode

**Severity:** SHOULD
**Check:** agent-judgment

Tier 2 starts advisory-only for a release or two to tune severities and suppress
false positives, then promotes to sign-off-gating once its report is trusted.
Shipping it as a hard gate on day one guarantees a noisy revolt.

---

### LINT-08 — The linter is the doc set's own drift check

**Severity:** MUST
**Check:** agent-judgment

If a rule cannot be expressed as either an automated check or an agent-judgment
prompt, it is too vague to keep and must be sharpened or dropped. Authoring a rule
and authoring its check are one activity: each rule's `Check:` line is its linter
spec.

---

## How the full release gate composes

Cutting a release from trunk runs, in order:

1. **Tier-1 deterministic** — `make lint` + `make test` + the custom static checks.
   Hard-fail.
2. **Full test matrix** — all tiers on all relevant runners (TEST-02, TEST-07).
   Hard-fail.
3. **Security suite** — if the release diff trips GLOBAL-06, the TEST-08 security
   suite plus adversarial pen-tester agent. Hard-fail once designed (Deep-dive #1).
4. **Tier-2 behavioral report** — the agent's findings, signed off by a human.
   MUST-severity findings gate on the human signature, not on the agent (LINT-03).
