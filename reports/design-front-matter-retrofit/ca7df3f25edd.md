# Behavioral linter: design-front-matter-retrofit @ ca7df3f25edd

- Round 3, step `f4-review`. The previous report is `fdd14b44f1f8`.
- Branch `feat/design-front-matter-retrofit`, base `cd42fc73c265`.
- Scope: `git diff origin/main...feat/design-front-matter-retrofit`: 8 files, +938/-3 (LINT-04). origin/main is at 154833b (workflow-streamline, #75 and #76). It is not merged here; f6-merge-main merges it.
- Model: claude-opus-5-5. Tier 2 is report-only (LINT-07). Findings are advisory and never block on their own (LINT-03).

**Counts:** MUST 0, MUST-review 0, SHOULD 1. Suppressions: none (the previous report recorded none).

## Tier 1 (re-run by the linter at ca7df3f): green

| Check | Result |
|---|---|
| `make lint` (golangci-lint 2.6.2, go1.26.3) | pass, 0 issues (root module and tools/workflow) |
| `make test` | pass, every package ok |
| Front-matter parse (`check.ParseDesign` + `Front.problems`, scratch test via `go test -overlay`) | pass: no problems for the six retrofits (`firecracker-driver` wanted for `firecracker-driver-plan`), this design and `workflow-bootstrap`. Exactly `missing alters_relationships` for the three archived designs (a3, expected). Both retrofit notes are present. |
| Body unchanged | pass: 0 removed lines under `docs/design`. The six blocks and two notes are byte-identical to the Design section over the merge-base body. The three archived designs are identical to the merge base. |
| Rule IDs and host names | pass: every rule ID exists (83 in the index). Every host is in `hosts.yaml` except `linux-kvm`, which is new (as the design says). |
| Build-tagged files | none touched |
| Custom static checks (LINT-02) | none exist yet |

No fix was needed and no branch commit was made.

## Prior findings (fdd14b44f1f8)

| # | Finding | Status |
|---|---|---|
| 1 | MUST FLOW-02 `windows-driver-plan.md`: unsupported claim that guest admin rights cannot open a path | **fixed**: the note now relies on no in-guest control and leaves the question to the adversarial gate, which the plan does not record as run (lines 488-492) |
| 2 | SHOULD FLOW-02 `windows-driver-plan.md`: "trusted input" | **fixed**: now "input fixed at prepare". The BOOT-06 gloss added with it is slightly broad (finding 1 below). |
| 3 | SHOULD GLOBAL-06 `remote-execution-…md`: note omitted CAS staging and the RE API surface | **fixed**: both are named in Boundary and in TEST-08, and BOOT-02 is in `rules` |

## Tier 2 findings

### MUST

None.

### SHOULD

1. **FLOW-02** (BOOT-06, GLOBAL-06), `docs/design/windows-driver-plan.md:493`. The text comes from the Design section at `design-front-matter-retrofit.md:538`.
   - **What's wrong:** the gloss "(BOOT-06: it carries the startup hook, not build content)" goes further than BOOT-06 and the plan.
     - BOOT-06 forbids only content that is per-build or tied to a warden version.
     - The plan leaves open whether the toolchain is baked into the image or bootstrapped through the relay (Phasing chunk 2, line 443).
     - The gloss therefore settles a question the plan leaves open, against Decision 2.
   - **Why it is only SHOULD:** the security weight is low, because a baked toolchain is fixed at prepare. The wording also came from the previous report's own suggestion, and Jeff's p1-escalate-3 direction said to change nothing else in the notes.
   - **Suggested fix:** if the note is touched again, use "(BOOT-06: it carries the startup hook and nothing per-build)". Otherwise acknowledge it in the PR body.

## Informational

- **FLOW-19, for f6-merge-main.** Main's `Front.problems` requires `design_review` on every non-archived design. Once main merges, it will report the key missing for:
  - this design (draft);
  - `firecracker-driver-plan` and `remote-execution-and-measured-hermeticity` (draft);
  - `hyperv-driver-plan`, `vm-driver-release-plan` and `platform-compatibility` (active).

  Only `windows-driver-plan` (archived) is exempt under #76. The Validation plan's tier-0 expectation will then fail. The merge round should decide the key for this design and decide how retrofitted documents, which never went through the gate, are treated. Alternatively, it can extend the a8 hand-off to `tier1-static-checks`.
- **FLOW-12** (`process.md:281`) still says pre-standard designs are retrofitted lazily. After this feature none is left. That is a rule edit for Jeff (FLOW-14), not a finding.
- `windows-driver-plan.md:492` is 86 characters long, while the rest of the note wraps at about 78. It matches the design byte for byte, so this is cosmetic.
- The README follow-up bullet was removed by Jeff's attended step (ca7df3f), per a9. No stale reference to it remains.
- Round-4 changes checked against Jeff's answers (a2, a3, a6, p1-escalate-3): all applied. Every rule a note cites is in its document's `rules`.
- The remote-execution note's claim "only the action's cache key (`input_root_digest`) records them" is supported (proposal lines 108-121).
- UX-01 / FLOW-15: none owed.

## Routing

- Tier 1 is green, so wf-implement has nothing to fix.
- There are no MUST findings to send back.
- SHOULD finding 1 and the FLOW-19 note go in the PR body. The human acknowledges them at merge (FLOW-08).
