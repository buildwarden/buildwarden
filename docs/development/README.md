# BuildWarden Development Standards

Guidance, process, and best practices for developing BuildWarden, whether the
developer is a human or an AI agent. These documents exist to keep a high
standard while keeping development productive and release velocity healthy.

## The core idea: the docs are the ruleset

Every standard here is written as an atomic, identified, severity-tagged **rule**.
The same document a human reads is the checklist the behavioral linter
(`behavioral-linter.md`) executes. There is one source of truth, not a pile of
prose that drifts away from a separate list of checks.

This is deliberate. The project's comment philosophy (GLOBAL-01) exists to stop
comments from drifting out of sync with code. The same instinct applies to
standards: a rule that no longer matches the code should be a visible, greppable,
reviewable line, not an unwritten assumption.

## Rule format

Each rule is a subsection headed by its ID and title, followed by two metadata
lines and a body:

```markdown
### RELAY-01 — Single-writer ledger

**Severity:** MUST
**Check:** automated + agent-judgment

Body describing the rule, the rationale, and how it is checked.
```

### ID scheme

| Prefix    | Domain                                             | File                    |
|-----------|----------------------------------------------------|-------------------------|
| `GLOBAL-` | Cross-cutting principles                           | `global.md`             |
| `RELAY-`  | Relay (`relay/`, `cmd/relay/`)                     | `relay.md`              |
| `ORCH-`   | Orchestrator (`cmd/warden/`)                       | `orchestrator.md`       |
| `EXEC-`   | In-VM executor (`cmd/warden-io/`)                  | `in-vm-executor.md`     |
| `BOOT-`   | Build-environment bootstrap model (all drivers)    | `bootstrap.md`          |
| `UX-`     | UX / CLI / configuration                           | `ux-cli-config.md`      |
| `TEST-`   | Testing and dev-velocity                           | `testing.md`            |
| `LINT-`   | The behavioral-linter agent itself                 | `behavioral-linter.md`  |
| `FLOW-`   | Development process: queue, phases, sign-off       | `process.md`            |
| `SEC-`    | The TEST-08 security suite                         | `security-suite.md`     |

### Severity

- **MUST** — a hard requirement. A deterministic violation blocks the release gate.
- **MUST-review** — high-severity; requires an explicit reviewer/human sign-off,
  because the mechanical check cannot fully judge it.
- **SHOULD** — a strong recommendation; surfaced as informational, not blocking.

### Check-type

Tells the behavioral linter which tier enforces the rule:

- **automated** — a deterministic check (a `golangci-lint`/`go test` result, a
  `go/analysis` pass, a git-diff coupling check, or a ripgrep script).
- **agent-judgment** — evaluated by the behavioral-linter agent reading the diff
  against the rule.
- **hybrid** — a mechanical part enforced automatically plus a judgment part
  evaluated by the agent (stated as `automated + agent-judgment`).

A rule with no check-type is a documentation bug (LINT-01). If a rule cannot be
expressed as either an automated check or an agent-judgment prompt, it is too
vague to keep and must be sharpened or dropped (LINT-08).

## The three applications

BuildWarden is three independently-meaningful applications, not three layers of
one monolith (see GLOBAL-08):

- **Relay** — `relay/` (library) + `cmd/relay/` (binary): the MITM proxy, DNS,
  ledger writer, control plane, fairness scheduler. It keeps no build state on
  disk; the **collector** (`collector/`, plus the standalone `cmd/collector/`) is
  its single output path and input source, reached over a tunnel the host opens
  when it claims the relay (`relaycfg/`). The collector is part of the relay's
  public contract (GLOBAL-08, RELAY-07), not a fourth application.
- **Orchestrator** — `cmd/warden/`: the host CLI, driver dispatch, config,
  inspect, and image lifecycle.
- **In-VM executor** — `cmd/warden-io/`: the build-environment guest agent
  (`initialize` / `fetch` / `post` / `trust`).

## Documents

1. [`global.md`](./global.md) — cross-cutting principles (comments, errors,
   tests, reproducibility, security boundaries, cross-platform, component
   independence).
2. [`relay.md`](./relay.md)
3. [`orchestrator.md`](./orchestrator.md)
4. [`in-vm-executor.md`](./in-vm-executor.md)
   - [`bootstrap.md`](./bootstrap.md) — the bootstrap model every driver shares:
     isolate, deliver the executor, start it, then everything over the relay.
     Changing the model needs a signed comparison (BOOT-07).
5. [`ux-cli-config.md`](./ux-cli-config.md)
6. [`testing.md`](./testing.md)
7. [`security-suite.md`](./security-suite.md) — the TEST-08 security suite design
   (deep-dive #1).
8. [`behavioral-linter.md`](./behavioral-linter.md) — the release-gate agent that
   enforces this doc set.
9. [`process.md`](./process.md) — how features are designed, sequenced, validated,
   and signed off, by humans, attended agents, and the unattended feature loop.
   Design documents start from [`../design/TEMPLATE.md`](../design/TEMPLATE.md).

## Relationship to the other guidance files

This directory is authoritative for *standards*. The other guidance files point
here rather than restate rules, so the drift this doc set prevents does not move
elsewhere:

- `AGENTS.md` is the tool-neutral map for agents; `CLAUDE.md` imports it.
- `CONTRIBUTING.md` holds setup, commands, testing, pull requests and releases;
  the mdbook's `contributing/development.md` includes it verbatim.
- The mdbook's `contributing/structure.md` describes the package layout and cites
  rule IDs for the patterns it mentions.

A new rule found in one of those files is moved here and replaced by its ID.

Agent definitions and skills (`.claude/`, `.kiro/`) are thin wrappers over this
directory: they carry phase instructions and cite rule IDs, and never restate rules
(FLOW-14).

## Open deep-dives (TODO)

Each item is a feature in the work queue on the `workflow` branch
(`queue/<slug>.yaml`, slug in brackets), where its state, priority and
questions live (FLOW-03, FLOW-07). The queue is authoritative; this list says
what each item is.

- **Deep-dive #1 — Security suite + adversarial pen-tester agent** (TEST-08)
  [`security-suite`]:
  designed in [`security-suite.md`](./security-suite.md) (SEC-01..08). Remaining
  build work: the isolated harness, the Arm-A corpus, the Arm-B agent wiring, and
  the monthly/on-demand cadence.
- **Deep-dive #2 — Tier-1 static-check extraction** (LINT-02)
  [`tier1-static-checks`]. Decide which
  `automated`-tagged rules become `go/analysis` passes vs ripgrep scripts vs
  git-diff coupling checks, and implement them.
- **Follow-up — Workflow bootstrap** [`workflow-bootstrap`]: the queue, phase
  agents, daily loop and FLOW-18 guard, designed in
  [`workflow-bootstrap.md`](../design/workflow-bootstrap.md). Bootstrap step 1.
- **Native-gap backlog** (FLOW-09, TEST-03): validation deferred by merged
  designs, kept in `backlog/native-gaps.yaml` on the `workflow` branch and
  drained by native sessions (`/workflow-native <host>`).
- **Follow-up — Container inputs under `inputs/`** (ORCH-02)
  [`container-inputs`]: record the
  submitted Dockerfile and the translated `build.sh` under `inputs/`.
  `Dockerfile.actual` was retired by `unified-bootstrap` (every driver now runs
  the same translation, and `build.sh` is what ran).
- **Follow-up — `specs/` migration** (RELAY-05) [`specs-migration`]. Perform
  the actual move of the
  ledger spec and metadata schemas into the top-level `specs/` layout; this is an
  implementation change separate from adopting the standard.
- **Feature — Firecracker driver** [`firecracker-driver`]: the plan in
  [`firecracker-driver-plan.md`](../design/firecracker-driver-plan.md).
