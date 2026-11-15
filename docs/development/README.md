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

The standards are in two parts. The **project invariants** are this
repository's own. The **loop rules** are the
[Loop Contract](./loop-contract.md) and its detail, which the loop
runs under in every repository it works in; they are kept here until
the loop repository exists, and move to it then.
[`loop-profile.md`](./loop-profile.md) is this repository's profile
(PROFILE-01).

## Rule format

Each rule is a subsection headed by its ID and title, followed by two metadata
lines and a body:

```markdown
### RELAY-01 — Single-writer ledger

**Severity:** MUST
**Check:** automated + agent-judgment

Body describing the rule, the rationale, and how it is checked.
```

This format is LINT-01's and is the same for every component.

### ID scheme

| Prefix     | Domain                                             | File                    | Owner |
|------------|----------------------------------------------------|-------------------------|-------|
| `GLOBAL-`  | Cross-cutting principles                           | `global.md`             | `buildwarden/buildwarden` |
| `RELAY-`   | Relay (`relay/`, `cmd/relay/`)                     | `relay.md`              | `buildwarden/buildwarden` |
| `ORCH-`    | Orchestrator (`cmd/warden/`)                       | `orchestrator.md`       | `buildwarden/buildwarden` |
| `EXEC-`    | In-VM executor (`cmd/warden-io/`)                  | `in-vm-executor.md`     | `buildwarden/buildwarden` |
| `BOOT-`    | Build-environment bootstrap model (all drivers)    | `bootstrap.md`          | `buildwarden/buildwarden` |
| `UX-`      | UX / CLI / configuration                           | `ux-cli-config.md`      | `buildwarden/buildwarden` |
| `TEST-`    | Testing and dev-velocity                           | `testing.md`            | `buildwarden/buildwarden` |
| `LINT-`    | The behavioral-linter agent itself                 | `behavioral-linter.md`  | the loop |
| `FLOW-`    | Development process: queue, phases, sign-off       | `process.md`            | the loop |
| `SEC-`     | The TEST-08 security suite                         | `security-suite.md`     | `buildwarden/buildwarden` |
| `LOOP-`    | The Loop Contract                                 | `loop-contract.md`      | the loop |
| `PROFILE-` | This repository's loop profile                    | `loop-profile.md`       | `buildwarden/buildwarden` |

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

### Project invariants

1. [`global.md`](./global.md) — cross-cutting principles (comments, errors,
   tests, reproducibility, security boundaries, cross-platform, component
   independence, defaults and overrides); GLOBAL-09 to GLOBAL-11 cover the
   whole repository.
2. [`relay.md`](./relay.md) — the relay: ledger, serving, keys, modes, specs, the
   collector, and configuration through its claim (RELAY-08).
3. [`orchestrator.md`](./orchestrator.md)
4. [`in-vm-executor.md`](./in-vm-executor.md)
   - [`bootstrap.md`](./bootstrap.md) — the bootstrap model every driver shares:
     isolate, deliver the executor, start it, then everything over the relay.
     Changing the model needs a signed comparison (BOOT-07).
5. [`ux-cli-config.md`](./ux-cli-config.md)
6. [`testing.md`](./testing.md)
7. [`security-suite.md`](./security-suite.md) — the TEST-08 security suite design
   (deep-dive #1).
8. [`loop-profile.md`](./loop-profile.md) — this repository's profile under the
   Loop Contract (PROFILE-01).

### Loop rules

1. [`loop-contract.md`](./loop-contract.md) — the Loop Contract (LOOP-01 to
   LOOP-09).
2. [`process.md`](./process.md) — how features are designed, sequenced, validated,
   and signed off, by humans, attended agents, and the unattended feature loop.
   Design documents start from [`../design/TEMPLATE.md`](../design/TEMPLATE.md).
3. [`behavioral-linter.md`](./behavioral-linter.md) — the release-gate agent that
   enforces this doc set.

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
  drained by native sessions (`/workflow-native <host>`, and the scheduled
  per-machine sessions of [`native-host-loop.md`](../design/native-host-loop.md)).
