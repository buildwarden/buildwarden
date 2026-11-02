# AGENTS.md

Context for AI agents working in this repository. It is a map, not a rulebook:
every standard lives in [`docs/development/`](docs/development/README.md), and
when this file and a rule disagree, the rule wins (FLOW-14).

BuildWarden runs a build in an isolated environment whose only network path is a
**relay**. The relay records every byte the build receives to a signed,
tamper-evident ledger and streams all of its outputs to a host-side
**collector**.

## Where things are

| You need | Read |
|---|---|
| The standards (rule IDs, severities, checks) | [`docs/development/README.md`](docs/development/README.md) and the files it lists |
| How a feature is designed, built, reviewed and merged | [`docs/development/process.md`](docs/development/process.md) |
| How each component works, as pseudocode | [`docs/book/src/concepts/overview.md`](docs/book/src/concepts/overview.md) |
| The package layout | [`docs/book/src/contributing/structure.md`](docs/book/src/contributing/structure.md) |
| The bootstrap model every driver follows | [`docs/development/bootstrap.md`](docs/development/bootstrap.md) |
| Designs, past and in progress | [`docs/design/`](docs/design/); new ones start from [`TEMPLATE.md`](docs/design/TEMPLATE.md) |
| The feature loop: queue state, phase agents, entry points | the `workflow` branch's `README.md`; [`.claude/skills/`](.claude/skills/) (`workflow-tick`, `workflow-human`, `workflow-native`, `workflow-intake`) and [`.claude/agents/`](.claude/agents/) |
| The ledger wire format and its metadata schemas | [`specs/`](specs/README.md): `build-ledger/v1.md`, and each schema's `v<N>.md` beside its `v<N>.schema.json` |
| Setup, commands, tests, releases | [`CONTRIBUTING.md`](CONTRIBUTING.md) |
| User-facing behavior (CLI, config, output) | [`docs/book/src/guide/`](docs/book/src/guide/) |

## Working here

- Start a feature with a design (FLOW-01); check `docs/design/` for archived
  designs that overlap first.
- Before pushing: `make test` and `make lint`, the latter for each platform whose
  build-tagged files you touched (FLOW-17). If `go` misbehaves, run it through
  `mise exec -- go ...`.
- Push your feature branch at each green checkpoint; never push `main`,
  force-push, or delete a remote branch (FLOW-16, FLOW-17). Open the PR to `main`
  once the work is ready, without asking; merging it is the human's call (FLOW-08).
- A user-facing change updates its reference docs in the same change (UX-01), and
  a change to a component's flow updates its pseudocode in `overview.md`
  (FLOW-15).
