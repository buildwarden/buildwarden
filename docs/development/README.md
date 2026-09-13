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
| `UX-`     | UX / CLI / configuration                           | `ux-cli-config.md`      |
| `TEST-`   | Testing and dev-velocity                           | `testing.md`            |
| `LINT-`   | The behavioral-linter agent itself                 | `behavioral-linter.md`  |

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
  ledger writer, control plane, fairness scheduler.
- **Orchestrator** — `cmd/warden/`: the host CLI, driver dispatch, config,
  extensions, inspect, and image lifecycle.
- **In-VM executor** — `cmd/warden-io/`: the build-environment guest agent
  (`initialize` / `fetch` / `post` / `trust`).

## Documents

1. [`global.md`](./global.md) — cross-cutting principles (comments, errors,
   tests, reproducibility, security boundaries, cross-platform, component
   independence).
2. [`relay.md`](./relay.md)
3. [`orchestrator.md`](./orchestrator.md)
4. [`in-vm-executor.md`](./in-vm-executor.md)
5. [`ux-cli-config.md`](./ux-cli-config.md)
6. [`testing.md`](./testing.md)
7. [`behavioral-linter.md`](./behavioral-linter.md) — the release-gate agent that
   enforces this doc set.

## Relationship to the other guidance files

`CLAUDE.md`, `AGENTS.md`, `DEVELOPING.md`, `CONTRIBUTING.md`, and the mdbook
`contributing/` tree predate this doc set. Where they overlap, this directory is
authoritative for *standards*; the others should be trimmed to point here rather
than restate rules, or the drift this doc set is meant to prevent simply moves
elsewhere. (Trimming them is follow-up work, tracked below.)

## Open deep-dives (TODO)

- **Deep-dive #1 — Security regression suite + adversarial pen-tester agent**
  (TEST-08). Design the suite contents, how the adversarial agent is scoped and
  scored, and the pass/fail criteria. Becomes a mandatory release-gate input for
  releases that trip GLOBAL-06.
- **Deep-dive #2 — Tier-1 static-check extraction** (LINT-02). Decide which
  `automated`-tagged rules become `go/analysis` passes vs ripgrep scripts vs
  git-diff coupling checks, and implement them.
- **Follow-up — Reconcile legacy guidance files** (`CLAUDE.md`, `AGENTS.md`,
  `DEVELOPING.md`) down to pointers at this directory.
- **Follow-up — `specs/` migration** (RELAY-05). Perform the actual move of the
  ledger spec and metadata schemas into the top-level `specs/` layout; this is an
  implementation change separate from adopting the standard.
