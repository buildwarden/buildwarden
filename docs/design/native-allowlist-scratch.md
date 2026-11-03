---
feature: native-allowlist-scratch
status: active
drivers: []
seams: []
surfaces: []
security_boundary: none
alters_relationships: false
design_review: required
native_validation: []
tier2_gaps: []
rules: [FLOW-13, FLOW-14, FLOW-16, FLOW-17, FLOW-18]
concepts: [native-session, permissions, workflow]
related: []
---

# Native allowlist: rules that match what the skill runs

## Problem

The unattended native session runs under
`tools/workflow/native/settings.native.json`. Two macbook runs (digests
2026-10-30 and 2026-10-31) found that the skill's documented way to run a
branch's binaries, `.scratch/<name>/dist/warden ...`, is refused, and that
`tools/workflow/flow18-check.sh` needs approval nobody is there to give. The
first run worked around it by copying branch binaries into `dist/`, which
mixes branch and `main` builds in one place.

The cause is the rule syntax. In Claude Code a trailing `:*` is the same as a
trailing ` *`: the rule's text, then a space, then anything. So
`Bash(.scratch/:*)` matches only a command that is literally `.scratch/`
followed by a space, which never occurs. Four rules have this shape:

| Rule | Meant to allow | Actually matches |
|---|---|---|
| `Bash(.scratch/:*)` | `.scratch/<name>/dist/warden ...` | nothing |
| `Bash(git -C .scratch/:*)` | `git -C .scratch/<name> ...` | nothing |
| `Bash(tools/workflow/:*)` | `tools/workflow/flow18-check.sh`, `native/*.sh` | nothing |
| `Bash(dist/warden:*)`, `Bash(./warden:*)` | the built CLI | not `dist/warden.exe` on Windows |

## Prior art

- The Claude Code permissions reference ("Wildcard patterns"): `*` stands for
  any text anywhere in the rule; `:*` is only a trailing ` *`; compound
  commands are split on `&&`, `||`, `;`, `|`, `&` and newlines and every part
  must match; rules match the command text and are not a security boundary
  around a program.
- The native skill (`.claude/skills/workflow-native/SKILL.md`): branch work in
  `.scratch/<name>` worktrees, driven with `git -C .scratch/<name>`, binaries
  run as `.scratch/<name>/dist/warden`.

## Decisions

1. **Replace the four rules with mid-pattern wildcards:**
   `Bash(.scratch/*/dist/warden*)`, `Bash(git -C .scratch/*)`,
   `Bash(tools/workflow/*)`, `Bash(dist/warden*)` and `Bash(./warden*)`. No
   space before the final `*` in the warden rules, so `warden.exe` on Windows
   matches too. Rejected: `Bash(.scratch/*)`, which would run any file a
   branch put under `.scratch/`, not only the CLI it built.
2. **Close the push gap the `git -C` rule opens.** Today's denies name
   `git push ...` and `git -C .dev/workflow push ...`; a working
   `git -C .scratch/*` allow would let `git -C .scratch/x push origin main`
   through. Add denies that do not depend on the directory:
   `Bash(git -C * push origin main*)`, `Bash(git -C * push origin HEAD:main*)`,
   `Bash(git -C * push --force*)`, `Bash(git -C * push -f*)`,
   `Bash(git -C * push * --force*)`, `Bash(git -C * push origin --delete*)`
   and `Bash(git -C * push --mirror*)`. Branch protection on `main` remains
   the real boundary; these keep the rule list honest (FLOW-16, FLOW-17).
3. **Record, do not change, root inside WSL.** `Bash(wsl:*)` lets the session
   run `wsl -d Ubuntu sudo ...`, which is how the 2026-11-02 run did
   `sudo warden setup --driver firecracker`, even though `Bash(sudo:*)` is
   denied. That is intended: Jeff gave the WSL user passwordless sudo because
   that distro is used only for BuildWarden testing (2026-11-02), and the
   firecracker driver's native validation needs root for `warden setup`. The
   file gains a `deny` for `Bash(wsl --unregister*)` so a session cannot
   delete the distro, and this design is the record of the decision.
4. **Out of scope:** the macbook's `~/Downloads` sandbox block (a different
   mechanism), and any new tool the skill does not already document.

## Questions

### Blocking

None.

### Assumed

None.

## Design

In `tools/workflow/native/settings.native.json`:

- `allow`: replace `Bash(git -C .scratch/:*)`, `Bash(.scratch/:*)`,
  `Bash(tools/workflow/:*)`, `Bash(./warden:*)` and `Bash(dist/warden:*)` with
  `Bash(git -C .scratch/*)`, `Bash(.scratch/*/dist/warden*)`,
  `Bash(tools/workflow/*)`, `Bash(./warden*)` and `Bash(dist/warden*)`.
- `deny`: add the seven `git -C * push ...` rules of Decision 2 and
  `Bash(wsl --unregister*)`.

No other file changes; the skill already documents these commands.

## Cross-driver impact

None.

## Bootstrap comparison

Not applicable.

## Security note

The allow changes widen nothing beyond what the skill already documents: the
CLI a branch built, `git` in a scratch worktree, and the repository's own
workflow scripts. Decision 2 narrows what a working `git -C` rule would
otherwise allow. Decision 3 records an existing capability (root in the WSL
test distro) rather than adding one.

## Validation plan

The next scheduled native run on each machine (macbook, windows-desktop) runs
`tools/workflow/flow18-check.sh` and `.scratch/<name>/dist/warden version`
without a denial, and its digest says so. `make lint` and `make test` for the
JSON change.

## Implementation plan

1. Edit `settings.native.json` as above.
2. `make lint` and `make test`; archive this design (FLOW-12) and open the PR.

## Design review

- **r1, 2026-11-02, Jeff: approved** as drafted, approved at 91ff7f0.
  Decisions confirmed: the mid-pattern wildcard rules (D1); the `git -C *
  push` denies (D2); root in the WSL test distro through `Bash(wsl:*)` stays,
  recorded here, with a `wsl --unregister` deny (D3).

## Consolidation

## Rollback notes

Revert the commit; the old rules simply prompt again.

## Archive summary
