# UX / CLI / Configuration Standards

Cross-cutting standards for the user-facing surface shared by all three binaries:
command-line ergonomics, output, and configuration. These serve both human
developers and AI agents driving the tools.

See [`README.md`](./README.md) for the rule format.

---

### UX-01 — Docs-coupling (the anti-drift keystone)

**Severity:** MUST
**Check:** automated + agent-judgment

Any new or changed user-facing surface (a CLI flag, subcommand, config key, or
output format) updates its reference documentation
(`docs/book/src/guide/{cli,configuration}.md`) in the same change. This is the
mechanical enforcement of the same "don't let prose drift from reality" instinct
behind the comment rule (GLOBAL-01).

The automated part is a surface diff against a docs diff.

---

### UX-02 — Stable machine-readable output

**Severity:** MUST
**Check:** automated + agent-judgment

Every human-facing output has a stable, documented machine-readable form (for
example `inspect --json`). Color and decoration are for humans only and auto-disable
when stdout is not a TTY or a structured format is requested. This keeps the tool
first-class for AI agents as well as humans.

The automated part checks TTY detection and the presence of a structured mode.

---

### UX-03 — Backward-compatible surfaces

**Severity:** MUST
**Check:** automated + agent-judgment

Flags, subcommands, and config keys are additive. A rename or removal ships an alias
plus a deprecation notice with a documented removal window, never a silent break.
Post-1.0 this tightens, consistent with the seam-stability posture (GLOBAL-08).

The automated part flags a removed or renamed surface with no alias or deprecation.

---

### UX-04 — Actionable errors

**Severity:** MUST
**Check:** agent-judgment

A user-facing error states what failed and the concrete fix or next step, not just
the raw cause.

---

### UX-05 — Consistent CLI shape

**Severity:** SHOULD
**Check:** agent-judgment

Cobra command and flag conventions stay consistent across binaries: naming style,
the `--driver` / `--runtime` patterns, and the split between global and per-command
flags. The surface should be predictable for a human and scriptable for an agent.

---

### UX-06 — Short flags follow standard conventions

**Severity:** SHOULD
**Check:** agent-judgment

CLI arguments provide short forms using standardized shortening practices when the
short form is unambiguous and does not conflict with common cross-CLI conventions
(`-v` for verbose, `-h` for help, and so on). Do not mint a short flag that collides
with a widely-expected meaning; when in doubt, long-form only.

---

### UX-07 — Minimal config, predictable defaults, layered overrides

**Severity:** MUST
**Check:** automated + agent-judgment

Configuration can be minimal: predictable defaults cover the common use-cases, and
any of them is easy to override at three levels with clear precedence,
invocation > project > user (`flag > warden.toml > ~/.config/warden`, the same order
as ORCH-03). A first-time user should get a sensible run with little or no config.

The automated part checks that defaults resolve with no config file present.

---

### UX-08 — Fail fast on config

**Severity:** MUST
**Check:** automated + agent-judgment

Configuration errors or mismatches are validated and surfaced as early as possible,
at load/parse/startup, before any real work begins, rather than failing deep into a
build. The automated part checks that config validation runs at startup.
