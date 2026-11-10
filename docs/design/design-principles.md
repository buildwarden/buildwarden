---
feature: design-principles
status: draft
drivers: []
seams: []
surfaces: []
security_boundary: none
alters_relationships: false
design_review: required
native_validation: []
tier2_gaps: []
rules: [FLOW-05, GLOBAL-01, GLOBAL-06, GLOBAL-08, GLOBAL-09, GLOBAL-10, GLOBAL-11,
  RELAY-04, RELAY-08, BOOT-06, UX-01, UX-03,
  UX-04, UX-07, UX-08, ORCH-03, FLOW-01, FLOW-02, FLOW-03, FLOW-06, FLOW-07,
  FLOW-08, FLOW-09, FLOW-10, FLOW-12, FLOW-14, FLOW-15, FLOW-17, FLOW-19, LINT-01, LINT-02,
  LINT-04, TEST-01, TEST-02]
rule_changes: []
concepts: [design-principles, defaults, overrides, fallback, least-surprise,
  explicit-over-implicit, minimal-magic, design-review, process-item,
  relay-claim, claim-protocol, relay-configuration, boot-channel,
  in-progress-limit]
related: [hyperv-relay-per-build-channel.md, flow19-design-before-plan.md,
  regression-fixes-follow-up.md, design-approved-rule-changes.md,
  build-link-subnet.md]
---

# Design principles: least surprise, explicit over implicit, minimal magic

## Problem

On 2026-11-08, in the design review of hyperv-relay-per-build-channel (r1,
read at `0df0a64`, recorded at `3b928e0`), Jeff gave three design principles
for all work from now on. Verbatim, from the queue file's `source`
(`.dev/workflow/queue/design-principles.yaml`):

> a set of design principles going forwards are:
> * Principle of least surprise: The default option should be a sane and
>   ideally obvious choice
> * Explicit is better than implicit: when a value is provided, it is to be
>   used exclusively instead of the default, not as a fallback unless
>   explicitly named as such.
> * Minimal Magic: Predictability of behavior is more important than
>   resiliency against failures. Good and reasonably-obvious defaults combined
>   with predictable simple behaviors when they are overridden make stronger
>   design considerations

The r1 design he was reviewing had a runtime fallback at almost every point
where a value could be missing or a path could fail:

- the relay init's resolver, `${UPSTREAM_DNS:-8.8.8.8}`
  (`tools/relay-vm/init:156-158` on `main`), and its "dhcp failed ... using
  static" path to `NAT_IP=192.168.240.2` (`:61-62`, `:121-122`);
- r1's question a3 offered "(b) the same with a public-resolver fallback" and
  "(d) a new `[vm] dns` key" as a later follow-up (`0df0a64`, design lines
  410-415);
- SMBIOS "kept as the fallback if the DVD path fails native validation"
  (`:154`), and "one durable build at a time ... kept as the fallback if
  native validation finds VLAN filtering does not hold" (`:311`, and a4 at
  `:416-421`);
- the Windows guest's static `--ip` racing the relay's lease.

Jeff's r1 changes applied the principles case by case: a3 became an optional
`[vm] dns` key "used explicitly, with no fallback to the host's resolvers",
and a6 dropped the Windows static address. Round 2 (`7be917d`, on
`origin/feat/hyperv-relay-per-build-channel`) added Decision 11, "No runtime
fallback anywhere in the design; contingencies are named and chosen by a
return to Design", with a table of every fallback it settled, and question
a12 deferred one more case to this item ("(b) belongs with the
`design-principles` item if it wants `--ip` itself to be explicit
everywhere"). Intake queued this item at 2026-11-08T22:08:49Z; the
2026-11-09 digest lists it under **Blocked on Jeff** for `x1-design`.

The principles live today in one queue `source` and in one design's Prior art.
No rule in `docs/development/` states them, so:

- `wf-design` (`.claude/agents/wf-design.md:20-24`) has nothing to apply when
  it writes the next design's Decisions, and nothing tells it that a fallback
  needs a name;
- tier 2 (`.claude/agents/wf-review.md:22-27`) judges the diff against the rule
  index, which has no such rule, so an unnamed fallback added in Implement is
  not a finding;
- the design review (FLOW-19, `docs/development/process.md:562-570`;
  `.claude/skills/workflow-human/SKILL.md:60-65`) has no topic for defaults
  and fallbacks, so catching them depends on Jeff reading for them, as he did
  at r1.

The rules that come closest cover configuration only: UX-07's "predictable
defaults, layered overrides" (`docs/development/ux-cli-config.md:105-117`) and
ORCH-03's "Defaults are sane" (`docs/development/orchestrator.md:62-71`). The
r1 fallbacks were in a boot script, a channel choice and a guest's address,
none of them config.

On 2026-11-09, in the r2 re-review of hyperv-relay-per-build-channel (read at
`7be917d`, recorded at `8d5beaf` on
`origin/feat/hyperv-relay-per-build-channel`), Jeff added a rule for the
relay, to go into this item as well as into that feature's BOOT-06 rewording:

> From here onwards, any configuration of the relay should happen in the
> claim interface explicitly.

Clarified in the same sitting: only what the relay needs in order to receive
a claim at all (its own address, prefix and gateway, and the claim token) may
come from the boot channel (a per-build seed or the kernel command line);
everything else that configures the relay for a build (its upstream DNS, the
build link) comes in the claim. The case that raised it was r2's `[vm] dns`:
it sets the relay's upstream resolvers, which the relay does not need before
the claim, yet r2 delivered it on Hyper-V's seed and on Firecracker's command
line (`warden.dns`). The re-review moved it into the claim, and asked r3 to
reword the proposed BOOT-06 paragraph, which says "The relay gets those from
its claim, its kernel command line or a per-build relay seed".

No rule says this either. RELAY-04 asks a relay change to name its modes,
GLOBAL-08 makes the claim a versioned interface, and build-link-subnet moved
the build link into the claim, but nothing tells the next design that adds a
relay setting where it goes, or tier 2 that a new flag or command-line key
is a finding; and the relay still takes some per-build configuration from
outside the claim (Design, **Relay configuration found outside the claim**).

In the same sitting, recording the approved design of rule-running-line-match
failed. Claiming its `x1-design` started the item (`Claim`,
`tools/workflow/queue/next.go:297-300`, starts a queued process item on any
worker's claim), and `dist/wf validate` then rejected the state with "6
features in progress, limit 5 (FLOW-05)"; `dist/wf claim` and `dist/wf done`
do not check the count. FLOW-05's limit (`process.md:205`, "At most 5
features are in progress at once") is enforced by `validate`
(`tools/workflow/queue/queue.go:359-370`) and by `atLimit` (`next.go:129-139`),
and both count every item with `status: in_progress`, process items included.
With five features open, starting a process item breaks `validate` for every
session, though FLOW-05 ranks process items ahead of fixes and features. Jeff
decided that process items do not count toward the limit (Decision 9).

## Prior art

- **UX-07** (`ux-cli-config.md:105-117`): minimal config, predictable
  defaults, three levels with precedence invocation > project > user. It is
  the config instance of least surprise, and its precedence is explicit over
  implicit across levels. It does not say what a set value does to the
  default (replace or merge), or what happens when a set value fails.
- **ORCH-03** (`orchestrator.md:62-71`): the same precedence; "Defaults are
  sane and changes are additive". Unchanged here; GLOBAL-09 cites it.
- **UX-08** (`ux-cli-config.md:121-128`): config errors at load, before work.
  It is where a provided value that cannot be used gets refused under
  GLOBAL-10. Unchanged.
- **UX-04** (`ux-cli-config.md:53-79`): a failure names its fix. It is what
  minimal magic puts in place of a fallback. Unchanged.
- **UX-03** (`ux-cli-config.md:40-49`): backward-compatible surfaces, an alias
  plus a deprecation for a rename. A documented alias is a second name for the
  same value, not a fallback, so it is consistent with GLOBAL-10 and
  unchanged. (The item's intake note calls it "UX-03 precedence"; precedence
  is UX-07 and ORCH-03.)
- **GLOBAL-01 to GLOBAL-08** (`docs/development/global.md`): the
  cross-cutting principles, described in the README as "cross-cutting
  principles" (`docs/development/README.md:88-90`). None covers defaults or
  fallbacks. The file's preamble scopes it to "all three applications"
  (`global.md:3`). The new rules go after GLOBAL-08, before the "Deployment
  modes" section (`global.md:155`).
- **GLOBAL-06** (`global.md:76-79`) and its design section: GLOBAL-06 is
  MUST-review, agent-judgment, with no `rules.yaml` entry, yet `wf check`
  requires a filled **Security note** when `security_boundary` is not `none`
  (`check.go:380-382`). That is the model for Decision 3: a section checked as
  part of the Design done-when ("design doc complete", FLOW-03,
  `process.md:110`; `rules.yaml:38`, FLOW-03 owned by `wf`), while the rule it
  serves stays agent-judgment.
- **hyperv-relay-per-build-channel r2** (`7be917d`, not yet on `main`):
  Decision 11 and question a13 are the first application, and the model for
  how a design records it: each former fallback, what replaces it, and a
  contingency named for a return to Design rather than tried at run time. Its
  BOOT-06 amendment ("no default stands in for a channel", approved at
  `0df0a64`) is a bootstrap-specific instance and is not restated here.
- **FLOW-14** (`process.md:401-443`): this is a process item, the route for a
  rule change no feature needs and the only route for `.claude/` and for
  `tools/workflow/` outside `check/`. Its branch is exempt from the
  rule-change clearance, so `rule_changes: []`; the rule wording is in
  **Design**, as in flow19-design-before-plan and regression-fixes-follow-up.
- **FLOW-07** (`process.md:239-256`): every assumption is a question with
  options and a recommendation, confirmed in the review. Decision 4 uses it
  for defaults a design adds.
- **LINT-01, LINT-02** and the rule index (`tools/tier1/rules/rules.go:1-3`,
  `tools/tier1/rules.yaml:1-7`): every rule needs a Severity and a Check line,
  and every `automated`-tagged rule needs exactly one entry in `rules.yaml`.
  Agent-judgment rules need none.
- **The design section check** (`tools/workflow/check/check.go:376-379`,
  `:413-417`): at a feature's Design done-when, `wf check` fails a section
  that is empty or still the template's text. A process item's phase 1 runs
  no section checks (`check.go:97-103`). The section parser
  (`check.go:538-557`) splits on any line that starts with `## `, inside a
  code fence too.
- **LINT-04** (`behavioral-linter.md:72-81`): tier 2 judges the diff only,
  which is why existing code is not re-judged until a change touches it
  (Decision 6).
- **`wf-review` steps 3 and 5** (`.claude/agents/wf-review.md:28-31`,
  `:36-38`): a finding on behavior `main` already has, which the diff does not
  cause, is filed as the feature's follow-up (FLOW-09); the tick sends MUST
  findings back to `wf-implement`, and the rest go to the PR body for the
  human at merge (FLOW-08).
- **The claim interface** (`relaycfg/relaycfg.go:1-57`, `Config` at
  `:91-110`): a relay boots unconfigured and the orchestrator pushes "the
  per-build Config"; "Pushing (rather than having the relay fetch its config
  at boot) keeps the relay ignorant of the host's topology: the orchestrator
  chooses every address" (`:9-12`). `Config` carries the collector, capture,
  upstream TLS, the environment record and `build_link`. The claim token is
  the one value fixed before the claim ("fixed when the relay is created",
  `:26-27`), read from `WARDEN_CLAIM_TOKEN` (`:79`, `cmd/relay/main.go:114`):
  the precedent for the pre-claim exception. The decoder refuses unknown
  fields (`:54-57`), so a new claim field meets an old relay as a loud
  refusal, not a silent default.
- **build-link-subnet** (archived): Decision 3 added `build_link` to the
  claim; Decision 4 rejected keeping the relay's build address on a kernel
  command-line key, and rejected keeping `--subnet` as host mode's channel
  ("two channels for one value, and the relay contract would differ by
  mode"), but kept `--subnet` as host mode's fallback for a claim without the
  link, and vm mode's use of an address already on the interface. Decision 9
  deferred Hyper-V's fixed relay address, token and upstream DNS to
  hyperv-relay-per-build-channel, because Hyper-V had no pre-claim channel.
  RELAY-08 states Decision 3 and 4's reasoning for every relay setting.
- **RELAY-04** (`relay.md:72-83`) and **GLOBAL-08**: a relay change names
  the modes it touches; the claim is part of the relay's versioned public
  interface (RELAY-07 says the same of the collector protocol,
  `relay.md:169-171`), so a new claim field follows GLOBAL-08. Unchanged;
  RELAY-08 cites both.
- **BOOT-06** (`bootstrap.md:154-162`) and its amendment in
  hyperv-relay-per-build-channel (approved at `0df0a64`; r3 rewords it after
  the re-review recorded at `8d5beaf`): no boot asset or image carries a
  network value the orchestrator chooses per build. That is the boot-asset
  half (what an image may not bake); RELAY-08 is the relay half (which
  channel a relay setting travels on). Each cites the other; neither restates
  the other, and this item does not edit BOOT-06.

## Decisions

1. **The principles become three rules, GLOBAL-09 to GLOBAL-11, in
   `docs/development/global.md`,** after GLOBAL-08 and before "Deployment
   modes and the ledger's role". Each keeps Jeff's wording as its first
   sentence; the rest says how it is judged and which existing rules apply
   it. Their wording is in **Design**. `global.md`'s preamble (`:3`) gains
   their wider scope (question a5), so the file's header and its rules agree.
   - **Rejected: UX-10 to UX-12 in `ux-cli-config.md`.** That file is the
     user-facing surface. The r1 fallbacks it would have needed to catch were
     a boot script, a channel and a guest's address, none of which is a flag
     or key.
   - **Rejected: a new file and prefix (`principles.md`, `DESIGN-`).** Three
     rules do not need a new domain; the README already names `global.md`
     "cross-cutting principles", and a new prefix means a new ID-scheme row
     and a new place to look.
   - **Rejected: one rule with three parts.** A finding names one rule ID
     with one severity, and the three differ in severity (Decision 2).
2. **Severity and check: GLOBAL-09 SHOULD; GLOBAL-10 MUST; GLOBAL-11 MUST;
   all three agent-judgment.** "Sane and ideally obvious" is a judgment that
   should inform, not block. Explicit over implicit is a yes/no question about
   the code (is the provided value used alone, or merged with or replaced by
   the default, and is that named?). Minimal magic is the same kind: a path
   that runs only on failure is there, and named, or not. A tier-2 MUST under
   GLOBAL-10 or GLOBAL-11 goes back to `wf-implement` as every MUST does
   (`wf-review.md:36-38`). There it has two fixes: remove the unnamed merge
   or fallback so the provided value is used alone and a failure stops with
   its fix, or, when the work needs it, return `back-to-design` (FLOW-06) so
   the design names it in **Defaults and overrides** and the review confirms
   it. Code alone never names one.
   - **Rejected: all three MUST.** A tier-2 MUST on "not obvious enough"
     sends work back to Implement over taste.
   - **Rejected: all three SHOULD.** The r1 review changed three decisions on
     exactly these grounds; an informational finding would go to the PR body
     and not stop the code.
   - **Rejected: MUST-review.** That severity asks for a human sign-off on the
     finding itself. Here the human's sign-off is already in the path: a
     fallback that stays is named by a return to Design and confirmed in the
     design review (FLOW-19), and one that goes is removed in Implement with
     nothing left to sign. A second sign-off at merge adds a wait, not a
     check.
3. **One cheap automated part: a required `## Defaults and overrides`
   section in `docs/design/TEMPLATE.md`, which `wf check` checks is filled at
   a feature's Design done-when, as it checks the Security note.** The
   section lists each default the design adds or changes, why it is the
   obvious one (GLOBAL-09), what overrides it, and each merge or fallback by
   name with its trigger (GLOBAL-10, GLOBAL-11); "None" is a valid fill. The
   check is the existing `section filled` check (`check.go:413-417`): one name
   added to the list at `:376-377`. As with GLOBAL-06 and the Security note
   (Prior art), it is template enforcement under FLOW-03's Design done-when
   ("design doc complete"), which `rules.yaml:38` already gives to `wf`. It
   does not test the rules themselves, so all three stay agent-judgment and
   `tools/tier1/rules.yaml` is unchanged. Each rule states what the section
   must hold for it, so a tier-2 finding names the rule it breaks: a default
   the section does not list is GLOBAL-09 (SHOULD); a merge, or a default
   tried after a provided value, that it does not name is GLOBAL-10 (MUST); a
   path that runs only on failure that it does not name is GLOBAL-11 (MUST).
   - **Rejected: GLOBAL-10 `automated + agent-judgment` with a `rules.yaml`
     entry for the section check.** The check tests that a section has text,
     not that a provided value is used exclusively; tagging GLOBAL-10 with it
     would claim an automated check of the rule that does not exist.
   - **Rejected: a front-matter field (`defaults: [...]`).** Defaults need a
     sentence each (why, what overrides, what triggers a fallback); YAML would
     hold names without reasons, and the front-matter is for routing (FLOW-02).
   - **Rejected: a static check for fallbacks in code** (a grep for
     `fallback`, `:-` in shell, a second `if err == nil` path). The word
     appears for unrelated reasons (`cmd/relay/mode_host.go:23` names a
     default-segment variable `fallback`), and most fallbacks do not say so. It would
     be noisy and easy to miss.
   - **Rejected: the section in the template with no check.** A required
     section that nothing checks gets skipped; the check costs one list entry
     and a test.
4. **Where the rules are applied, each by citation (FLOW-14):**
   - `docs/design/TEMPLATE.md`: the new section (Decision 3), after
     **Design**.
   - `wf-design` step 2: fill the section; each default, override or named
     fallback in it that the item's `source` or an existing rule does not
     already settle is an assumed question (FLOW-07), so the review confirms
     each one. Decisions that drop a fallback say so as a rejected
     alternative.
   - `wf-review` step 3: the section is what GLOBAL-09 to GLOBAL-11 are judged
     against; a default, merge or fallback in the diff that the section does
     not list is a finding under the rule Decision 3 names, and its fix is the
     one Decision 2 names. A design with no such section (a process design,
     which `wf check` does not check, or one written before this item) is
     judged by its Decisions: a merge or fallback they do not name is a
     finding. Tier 2 already picks the new rules up from the index
     (`wf-review.md:22-25`); the added sentences name the reference.
   - `wf-implement` step 5: a fallback the work seems to need that the
     section does not name is a finding for Design (FLOW-06), not code.
   - `workflow-human` step 0 (`SKILL.md:20-25`), where process designs are
     settled: a process design fills **Defaults and overrides** like a
     feature's, though `wf check` does not check it.
   - FLOW-19's topic list (`process.md:562-570`) and `workflow-human` step 3.3
     (`SKILL.md:60-65`): the review covers the section, each named fallback
     in particular.
   - RELAY-08 (Decision 8): `wf-design` step 2, for any relay setting a
     design adds or moves, names the claim field that carries it, or the
     pre-claim value it is; `wf-review` step 3 (tier 2) names RELAY-08's
     finding, a relay setting the diff delivers outside the claim, and its
     fix. No other agent or skill changes for it.
   - **Not changed:** `wf-plan`, `wf-publish`, `wf-guide`, `workflow-tick`,
     `workflow-intake`, `workflow-native`, `AGENTS.md`. Plan works from the
     approved design, Publish only links `wf-review`'s tier-2 report
     (`wf-publish.md:32`), `wf-guide` does not design or review, and the
     other skills do not design. AGENTS.md is a map and already points at
     `docs/development/`.
   - **Rejected: a question per default, always.** FLOW-07 says settle what
     you can; a default the source fixes ("an optional `[vm] dns` key, used
     exclusively") needs no second confirmation.
5. **Overlapping rules are cross-referenced, not rewritten.** The new rules
   cite UX-03, UX-04, UX-07, UX-08 and ORCH-03 for their config instances.
   One existing rule gains one sentence: UX-07 ends with "A value set at any
   level replaces the default and the lower levels' value (GLOBAL-10)", so a
   reader of the config rule finds what a set value does. FLOW-19 gains one
   topic bullet (Decision 4). UX-03, UX-04, UX-08 and ORCH-03 are unchanged;
   GLOBAL-10 says that ORCH-03's "changes are additive" is about changes to
   the config surface, not about how an override resolves. The README's
   description of `global.md` adds "defaults and overrides" and the wider
   scope.
   - **Rejected: no edit to any existing rule.** UX-07 is the rule a config
     designer reads first, and it is silent on replace versus merge.
   - **Rejected: folding UX-07's and ORCH-03's "sane defaults" into
     GLOBAL-09.** That rewrites rules with automated parts (UX-07's
     follow-up, ORCH-03's `surface-docs`) for no gain.
6. **Existing fallbacks are not audited now.** Future designs apply the
   rules. The ones found while drafting are listed in **Design** so the next
   design to touch each knows; this item files none. When a later diff
   touches one without causing it, tier 2 handles it the way it handles any
   finding on behavior `main` already has: it files the feature's follow-up
   (`wf-review.md:28-31`, FLOW-09), and the follow-up's design judges it.
   This item adds no exception to that path, so fallbacks become items one at
   a time, as work reaches them.
   - **Rejected: an exception in `wf-review` for the listed fallbacks.** It
     would make an archived design's list a standing suppression, and a
     fallback a diff reaches is the one worth judging.
   - **Rejected: an audit now, with a follow-up per fallback.** Some are
     deliberate and harmless (a pure-Go ISO writer when no external tool is
     installed), and judging each is design work for the feature that owns
     the code, which FLOW-01 already routes.
   - **Rejected: a tracked list (a backlog file).** A second list drifts; the
     design archive is searchable by `concepts: [fallback]`.
7. **Out of scope.** No principle beyond Jeff's three, his relay rule
   (Decision 8) and the limit exemption (Decision 9). No product code
   changes: hyperv-relay-per-build-channel a12 (b), making `--ip` on Windows
   strict for every caller of `warden-io initialize`, is an
   `executor-initialize` seam change and belongs to a feature design, not this
   item; this item leaves a12 to that review. No change to
   `docs/book/src/concepts/overview.md`: no component flow changes (FLOW-15).
8. **Jeff's relay rule becomes RELAY-08 in `docs/development/relay.md`,
   after RELAY-07: MUST, agent-judgment, no automated part** (questions a9,
   a10). Its first sentence is Jeff's; the rest is his clarification and how
   it is judged. Its wording is in **Design**.
   - **Home.** It is about one component's interface, so it lives with that
     component's rules, where a relay designer and tier 2 read for a relay
     change. `relay.md`'s preamble scopes it to `relay/` and `cmd/relay/`;
     RELAY-08 also binds whatever configures the relay (`relaycfg/`, the
     relay VM init, the drivers, `cmd/collector`), so the preamble gains a
     clause saying so, as GLOBAL-09 to GLOBAL-11 widen `global.md`'s.
   - **Severity: MUST.** Whether a relay setting arrives in the claim or on
     another channel is a yes/no question about the diff, and the re-review
     sent a design back on exactly that. A tier-2 MUST goes to
     `wf-implement`, whose fixes are to move the setting into the claim or,
     where that needs a new claim field the design did not plan (GLOBAL-08),
     `back-to-design` (FLOW-06). Not MUST-review: GLOBAL-06 already asks a
     human sign-off on every relay change (`relay.md:9-10`).
   - **Check: agent-judgment, no automated part.** No cheap check tests the
     rule: telling a pre-claim value from a setting needs the reading
     question a10 settles, and the channels differ by mode (a kernel
     command-line key, a seed file, a flag, an environment variable).
     `tools/tier1/rules.yaml` is unchanged (LINT-02).
   - **Boot channel.** Jeff named the seed and the kernel command line,
     which are vm mode's. Host mode (vz) and container mode have no boot
     asset; their equivalent is the flags and environment the launcher passes
     (`driver/vz/driver.go:329-334`, `driver/container/container.go:633-640`).
     RELAY-08 counts those as the boot channel, so the same four values may
     ride them and nothing else (RELAY-04's three modes). The address the
     relay accepts the claim on is part of "its own address". Launch wiring
     that is the same for every build of a driver and picks the relay's mode
     and its build-side endpoint (`-mode`, `-fd`, `-build-if`) is how the
     relay is started, not configuration for a build (question a10).
   - **With GLOBAL-10.** A setting travels on one channel: not on both the
     boot channel and the claim, and a boot-channel value never stands in for
     an absent claim field. What the relay does when an optional field is
     absent is a default named under GLOBAL-09 and GLOBAL-10, in `relaycfg`'s
     package doc. Host mode's `--subnet` without `build_link` breaks both
     rules, and is found, not fixed, here (Decision 6).
   - **With BOOT-06.** RELAY-08 cites BOOT-06 by ID for the boot-asset half
     and does not restate it; hyperv-relay-per-build-channel r3's BOOT-06
     paragraph cites RELAY-08 for the channel. Either can merge first:
     RELAY-08's text holds against BOOT-06 as it reads on `main` today, and
     against its amended wording.
   - **Existing relay configuration outside the claim is not audited**, as
     Decision 6 says of fallbacks; it is listed in **Design** for the next
     design that touches each. hyperv-relay-per-build-channel r3 already
     moves upstream DNS into the claim.
   - **Rejected: GLOBAL-12 in `global.md`.** A cross-cutting home suits a
     general rule ("each component is configured through its versioned
     interface"); Jeff stated it for the relay, and the executor and
     orchestrator have no claim. Kept as option (b) of a9.
   - **Rejected: BOOT-06 alone.** BOOT-06 is about images and boot assets;
     vz and container relays have none, and a claim field is not bootstrap.
     The re-review asked for both.
   - **Rejected: a tier-1 allowlist of `cmd/relay`'s flags, the relay's
     environment reads and the init's `warden.*` keys.** It tests that the
     set does not grow, not that a value is pre-claim; the init's keys change
     under hyperv-relay-per-build-channel r3; and today's entries outside the
     claim would have to be allowlisted, a standing suppression Decision 6
     rejects.
9. **Process items do not count toward FLOW-05's in-progress limit** (Jeff,
   2026-11-09, in this item's design review). The limit bounds how many
   features build in parallel, because each costs a later rebase and
   consolidation (FLOW-05); a process item is short and already ranked ahead
   of fixes and features (one that would disrupt running work pauses it
   through `holds`), so counting it only stops the rules from changing while
   five features wait. The limit's value (`limits.in_progress: 5`) is
   unchanged, and so is the `alters_relationships` limit, which still counts
   every in-progress item that declares it, process items included. Two
   counts change, each to skip `f.IsProcess()`: `validate`'s, which a started
   process item pushed over the limit, and `atLimit`'s, so an in-progress
   process item takes no cloud slot from a queued feature. The start check
   needs no change: the limit binds only cloud workers (`mayStart`,
   `NextFor`), and a queued process item's first step, `x1-design`, is human
   (`phases.go`), so a human already starts one at any count (`Claim`).
   - **Rejected: raise the limit to 7.** It keeps process items competing for
     feature slots, and the number would drift with the queue's mix. It is
     used only as the interim until every session's `dist/wf` counts process
     items out (Design).
   - **Rejected: keep the count and wait for a slot.** An approved process
     item would sit unrecorded behind work it is meant to unblock, as
     rule-running-line-match did behind jks-test-skip-without-jdk.
   - **Rejected: a separate limit for process items.** Process items are
     short and ranked first, so few are open at once (two when this was
     decided: this item and rule-running-line-match); a second number adds
     config without a case for it.

## Questions

### Blocking

None.

### Assumed

- **a1. Where the principles live** (Decision 1). Proceeding with GLOBAL-09
  to GLOBAL-11 in `global.md`. Options:
  - (a) as stated;
  - (b) UX-10 to UX-12 in `ux-cli-config.md`;
  - (c) a new `principles.md` with its own prefix.

  Recommended: (a).
- **a2. Severity** (Decision 2). Proceeding with GLOBAL-09 SHOULD, GLOBAL-10
  MUST, GLOBAL-11 MUST, all agent-judgment; a MUST goes back to Implement,
  which removes the merge or fallback or returns to Design to name it.
  Options:
  - (a) as stated;
  - (b) all three MUST;
  - (c) all three SHOULD (guidance in the PR body, never blocking).

  Recommended: (a).
- **a3. The automated part** (Decision 3). Proceeding with a required
  `## Defaults and overrides` design section, checked filled by `wf check`
  at a feature's Design done-when ("None" passes) as template enforcement
  under FLOW-03, the way the Security note serves GLOBAL-06; all three rules
  agent-judgment and `rules.yaml` unchanged. Options:
  - (a) as stated;
  - (b) the same check, with GLOBAL-10 tagged `automated + agent-judgment`
    and a `rules.yaml` entry owning the check;
  - (c) the section in the template, unchecked.

  Recommended: (a).
- **a4. What minimal magic allows** (GLOBAL-11's wording, Design).
  Proceeding with: a path, source or value that runs only when the first
  fails is allowed only when it is named under GLOBAL-10, as Jeff's second
  principle allows ("unless explicitly named as such"); otherwise the
  operation stops with its fix. A bounded retry of the same operation with
  the same inputs, reported as it happens, is not another path. A
  contingency that is not named as a run-time fallback is chosen by a return
  to Design (hyperv-relay-per-build-channel Decision 11). Options:
  - (a) as stated;
  - (b) no run-time fallback at all, named or not: Decision 11's stance as
    the general rule, and GLOBAL-10's named exception covers merges only;
  - (c) as (a), with retries also needing a name.

  Recommended: (a); (b) goes further than the principles as stated.
- **a5. Scope** (`global.md`'s preamble, Design). Proceeding with every
  design and change in the repository, the workflow tooling (`tools/`,
  `.claude/`) included, stated in the preamble. Options:
  - (a) as stated;
  - (b) the three applications and their build assets only, `global.md`'s
    current scope;
  - (c) user-facing surfaces only (flags, config, output).

  Recommended: (a).
- **a6. Existing fallbacks** (Decision 6). Proceeding with no audit: the ones
  found are listed in Design, and a diff that reaches one gets a follow-up
  through tier 2's existing path. Options:
  - (a) as stated;
  - (b) as (a), but `wf-review` files no follow-up for a fallback this
    design lists;
  - (c) an audit now, a follow-up item per fallback that breaks GLOBAL-10 or
    GLOBAL-11.

  Recommended: (a).
- **a7. Existing rules** (Decision 5). Proceeding with cross-references from
  the new rules, one sentence added to UX-07, one topic added to FLOW-19, and
  UX-03, UX-04, UX-08 and ORCH-03 unchanged. Options:
  - (a) as stated;
  - (b) the new rules cite the others, and no existing rule is edited (FLOW-19
    included; the topic lives in the skill only);
  - (c) fold UX-07's and ORCH-03's "sane defaults" into GLOBAL-09.

  Recommended: (a).
- **a8. hyperv-relay-per-build-channel a12 (b)** (Decision 7). Proceeding
  with leaving `--ip` on Windows to that feature's review; this item changes
  no product code. Options:
  - (a) as stated;
  - (b) this item also makes `--ip` strict on Windows for every caller (an
    `executor-initialize` seam change, Go code in `cmd/warden-io`).

  Recommended: (a).
- **a9. Where the relay rule lives, and how it is judged** (Decision 8).
  Proceeding with RELAY-08 in `relay.md`, after RELAY-07, MUST,
  agent-judgment, no `rules.yaml` entry, citing BOOT-06 and cited by
  hyperv-relay-per-build-channel r3's BOOT-06 paragraph. Options:
  - (a) as stated;
  - (b) GLOBAL-12 in `global.md`, the same wording and severity, beside
    GLOBAL-09 to GLOBAL-11;
  - (c) no separate rule: BOOT-06's r3 paragraph alone carries it;
  - (d) as (a), SHOULD (guidance in the PR body, never blocking).

  Recommended: (a). (b) fits if the rule is meant to grow beyond the relay.
- **a10. What counts as the boot channel and as configuration for a build**
  (Decision 8). Proceeding with: the boot channel is the seed and kernel
  command line in vm mode, and the launcher's flags and environment in host
  and container mode; the four values that may ride it are the relay's own
  address (the claim listen address included), prefix, gateway and the claim
  token; per-driver launch wiring (`-mode`, `-fd`, `-build-if`) is how the
  relay is started, not configuration for a build. Options:
  - (a) as stated;
  - (b) as (a), but launch wiring is configuration too, so it moves into the
    claim when a design next touches it (`-build-if` and `-fd` name the build
    side, which `build_link` already describes);
  - (c) the boot channel is only the seed and the kernel command line, as
    Jeff named them; host and container mode take nothing but the claim
    token outside the claim.

  Recommended: (a). (c) cannot hold: a host-mode or container relay must be
  told where to listen for the claim before it has one.

## Design

### The rules (`docs/development/global.md`, after GLOBAL-08)

`global.md`'s preamble (`:3-4`) becomes: "Cross-cutting principles that apply
to all three applications; GLOBAL-09 to GLOBAL-11 apply to all work in this
repository, the workflow tooling (`tools/`, `.claude/`) included. See
[`README.md`](./README.md) for the rule format and severity/check-type
meanings."

```markdown
### GLOBAL-09 — Least surprise: the default is the obvious choice

**Severity:** SHOULD
**Check:** agent-judgment

The default option is a sane and ideally obvious choice. A default is what
happens when nobody names a value: a flag or config key left unset, an argument
a caller omits, a choice the code makes on its own. It serves the common case and
is what a user who has not read the code would expect. A design lists each
default it adds or changes, why it is the obvious one and what overrides it, in
its **Defaults and overrides** section (`docs/design/TEMPLATE.md`); a default in
the diff that the section does not list is a finding under this rule. UX-07 and
ORCH-03 apply this to configuration.

---

### GLOBAL-10 — Explicit over implicit: a provided value is used exclusively

**Severity:** MUST
**Check:** agent-judgment

When a value is provided (a flag, a config key at any level, an environment
variable, an argument or field a caller sets), it is used exclusively instead of
the default:

- it is not merged with the default or a lower level's value, or appended to it;
- the default is not tried when the provided value fails. A provided value
  that cannot be used is an error that names it and its fix (UX-04, UX-08).

A merge or a fallback is allowed only when it is explicitly named as one: in the
surface's reference docs (UX-01) and in the design's **Defaults and overrides**
section, with what triggers it. A merge or fallback in the diff that the section
does not name is a finding under this rule. Layered precedence (UX-07, ORCH-03)
is this rule across levels: the highest level that sets a value is the value.
ORCH-03's "changes are additive" is about changes to the config surface, not
about how an override resolves. A documented deprecated alias (UX-03) is a
second name for the same value, not a fallback.

---

### GLOBAL-11 — Minimal magic: predictable over resilient

**Severity:** MUST
**Check:** agent-judgment

Predictability of behavior is more important than resiliency against failures.
Good, reasonably obvious defaults (GLOBAL-09) with predictable, simple behavior
when they are overridden (GLOBAL-10) make the stronger design. By default, when
an operation fails, it stops and says what failed and how to fix it (UX-04). A
path, source or value that runs only on failure is allowed only when it is named
as a fallback under GLOBAL-10; one the design's **Defaults and overrides**
section does not name is a finding under this rule. A bounded retry of the same
operation with the same inputs, reported as it happens, is not another path. A
contingency for a failure found in validation that is not named as a run-time
fallback is chosen by a return to Design (FLOW-06).
```

### Other rule and index edits

- `docs/development/ux-cli-config.md`, UX-07 (`:114-115`): after "...appear
  in the reference docs." add "A value set at any level replaces the default
  and the lower levels' value (GLOBAL-10)."
- `docs/development/process.md`, FLOW-19 (`:567`): after the bullet "the
  risks and what is deferred: ..." add the bullet "- the defaults, overrides
  and named fallbacks the design adds or changes (GLOBAL-09 to GLOBAL-11);".
  FLOW-19's automated part is unchanged.
- `docs/development/README.md` (`:88-90`): `global.md`'s description adds
  "defaults and overrides" and that GLOBAL-09 to GLOBAL-11 cover the whole
  repository. The ID-scheme table is unchanged (the `GLOBAL-` prefix exists).
- `tools/tier1/rules.yaml`: unchanged. The three rules are agent-judgment,
  and the section check is FLOW-03's, already owned by `wf` (`:38`).

### RELAY-08 (`docs/development/relay.md`, after RELAY-07)

`relay.md`'s preamble (`:3-10`) gains, after "the host-side collector
(RELAY-07).": "RELAY-08 also applies wherever the relay is configured: its
claim interface (`relaycfg/`), the relay VM init, the drivers and any other
claimant."

```markdown
### RELAY-08 — The relay is configured through its claim

**Severity:** MUST
**Check:** agent-judgment

Any configuration of the relay happens in the claim interface explicitly: a
field of `relaycfg.Config`, pushed on `/v1/claim`. Only what the relay needs in
order to receive a claim at all may reach it another way, on its boot channel:
its own address (the address it accepts the claim on included), prefix and
gateway, and the claim token. The boot channel is a per-build seed or the kernel
command line in vm mode, and the flags and environment its launcher passes in
host and container mode (RELAY-04). Everything else that configures the relay for
a build (its upstream resolvers, the build link, the collector, capture, upstream
TLS) is a claim field, in every mode and for every claimant.

- A new relay setting is a new claim field, added as a change to the relay's
  versioned interface (GLOBAL-08), never a new flag, environment variable, seed
  entry or kernel command-line key.
- A setting travels on one channel. It is not given on both the boot channel and
  the claim, and a boot-channel value never stands in for an absent claim field;
  what the relay does when an optional field is absent is a default named under
  GLOBAL-09 and GLOBAL-10 in `relaycfg`'s package doc.
- Launch wiring that is the same for every build of a driver and picks the
  relay's mode and build-side endpoint (`-mode`, `-fd`, `-build-if`) is how the
  relay is started, not configuration for a build.

BOOT-06 states the other half: no boot asset or image carries the boot-channel
values; they come per build. A relay setting the diff delivers outside the claim
is a finding under this rule; its fix is to move it into the claim, or, where
that needs a claim field the design did not plan, a return to Design (FLOW-06).
```

- `docs/development/README.md` (`:91`): `relay.md`'s entry gains a
  description, "— the relay: ledger, serving, keys, modes, specs, the
  collector, and configuration through its claim (RELAY-08)". The ID-scheme
  row (`:38`) is unchanged (the `RELAY-` prefix exists).
- `tools/tier1/rules.yaml`: unchanged for RELAY-08 too (agent-judgment,
  Decision 8).
- `docs/development/bootstrap.md` BOOT-06: not edited here.
  hyperv-relay-per-build-channel r3 rewords its proposed paragraph to say
  that the relay gets only its pre-claim values from the seed or command
  line, citing RELAY-08; whichever merges second leaves the other's text as
  it is.

### The template section (`docs/design/TEMPLATE.md`, after **Design**, `:48-50`)

A level-2 heading, `Defaults and overrides`, with this body:

```markdown
Each default this design adds or changes, why it is the obvious choice
(GLOBAL-09), and what overrides it, used exclusively (GLOBAL-10). Each merge or
fallback, named, with what triggers it and how the user sees it (GLOBAL-10,
GLOBAL-11). "None" when the design adds or changes no default, override, merge
or fallback.
```

The heading is not quoted inside the fence: `wf check`'s section parser
(`check.go:538-557`) starts a section at any line beginning `## `, fenced or
not, so a fenced heading would end the quoting design's **Design** section
early. A feature design that quotes the template does the same.

### The check (`tools/workflow/check/check.go:376-377`)

`"Defaults and overrides"` joins the list `"Problem", "Prior art",
"Decisions", "Design", "Cross-driver impact", "Validation plan"` that
`design()` passes to `section()`. Nothing else changes: `section()` already
fails an empty section or one equal to the template's text. A process item's
phase 1 runs no section checks (`:97-103`), so process designs are not
checked, though they use the same template and fill the section
(`workflow-human` step 0, below).

Effect on the live queue: the check runs only at a feature's phase-1
done-when. Of the open features, cross-driver-image-update and
vz-ipsw-latest-reuse (queued, phase 0) will need the section;
baked-asset-staleness, hyperv-relay-per-build-channel and security-suite
(phase 2), and the features past them, need it only if they return to Design.
No design is retrofitted. The check does not depend on the order of branches:
`section()` reads the template from the feature's branch (`check.go:497`),
and where that template predates this item its text for the section is
empty, so a `dist/wf` built from the merged `main` still fails an empty or
missing section. A feature branch cut before this merges needs the section
all the same; `wf-design`'s new sentence tells its drafter so.

### The agent and skill edits

- `.claude/agents/wf-design.md` step 2 (`:20-24`): "Fill **Defaults and
  overrides** (GLOBAL-09 to GLOBAL-11). Each default, override or named
  fallback there that the item's `source` or an existing rule does not
  settle is an assumed question (step 4), and a fallback the design drops is
  a rejected alternative in its decision."
- `.claude/agents/wf-design.md` step 2 also: "Fill it even when the branch's
  `TEMPLATE.md` predates it."
- `.claude/agents/wf-review.md` step 3 (`:22-27`): "GLOBAL-09 to GLOBAL-11 are
  judged against the design's **Defaults and overrides** section: a default,
  merge or fallback in the diff that the section does not list is a finding
  under the rule that asks for it. Its fix is to remove it, or, where the work
  needs it, `back-to-design` (FLOW-06) to name it; code alone does not name
  one. A design without the section (a process design, or one older than the
  rule) is judged by its Decisions."
- `.claude/agents/wf-implement.md` step 5 (`:28-30`): "...uncovers
  complexity the design missed, a fallback the design's **Defaults and
  overrides** does not name among them (GLOBAL-10), stop..."
- `.claude/skills/workflow-human/SKILL.md` step 0 (`:20-25`): "A process
  design fills **Defaults and overrides** like a feature's (GLOBAL-09 to
  GLOBAL-11); `wf check` does not check it."
- `.claude/skills/workflow-human/SKILL.md` step 3.3 (`:60-65`): the risks
  step adds "the **Defaults and overrides** section, each named fallback in
  particular (GLOBAL-09 to GLOBAL-11)".
- `.claude/agents/wf-design.md` step 2, for RELAY-08: "A relay setting the
  design adds or moves names the claim field that carries it (RELAY-08), or
  says which of the relay's pre-claim values it is."
- `.claude/agents/wf-review.md` step 3, for RELAY-08: "A relay setting the
  diff delivers on a flag, environment variable, seed entry or kernel
  command-line key, other than RELAY-08's pre-claim values, is a RELAY-08
  finding; its fix is to move it into the claim, or `back-to-design`
  (FLOW-06) when that needs a claim field the design did not plan."

### The in-progress limit (Decision 9)

- `docs/development/process.md` FLOW-05 (`:205-208`): the paragraph's first
  sentence becomes "At most 5 features are in progress at once, not counting
  process items (FLOW-03), and at most 1 of those features declares
  `alters_relationships: true` (FLOW-02), because two such features reshape
  the same structure and make each other's consolidation costly." A new
  sentence follows it: "Process items are not counted because they are short
  and already ranked ahead of fixes and features; one that would disrupt
  running work declares `holds`." The rest of the paragraph is unchanged.
- `tools/workflow/queue/queue.go` `validate` (`:359-364`): only the
  `inProgress++` at `:360` skips process items (`if !f.IsProcess() {
  inProgress++ }`); the outer `if f.Status == "in_progress"`, the alters count
  at `:361-363` and the error text (`:367-370`) are unchanged.
- `tools/workflow/queue/next.go`:
  - `atLimit` (`:129-139`) skips process items in its count;
  - `Next` (`:79-115`) and `NextFor` (`:252-262`): no behavior change (the
    limit binds only the cloud, which never takes a queued process item's
    human first step). Their doc comments say the limit counts no process
    items: `NextFor`'s "the in-progress limit still counts every feature"
    becomes "the in-progress limit counts every feature but process items".
- The `workflow` branch, at x4-close: `config.yaml`'s comment on
  `limits.in_progress` and `README.md`'s (`:37`, `:151`) say process items are
  not counted.

**Interim.** Until every session's `dist/wf` is rebuilt from the merged
`main` (Implementation plan step 9), `dist/wf validate`, which
`workflow-tick`, `workflow-native` and `workflow-intake` pass before they push,
still counts process items. So, before a process item is started with the
features at the limit, `human/jeff` raises `limits.in_progress` in the
`workflow` branch's `config.yaml` by the number of open process items started
that way (5 to 7 on 2026-11-09: this item and rule-running-line-match), in one
commit whose message and config comment cite this decision and say it is
temporary. Each such item's x4-close lowers it by one, and this item's
x4-close restores 5, in the same commit as its rebuild digest entry. No
skill's `validate` gate changes, and the cloud loop never raises it.

### Fallbacks found while drafting (not audited; Decision 6)

Listed for the next design that touches each. None is judged here.

- `cmd/warden/config.go:106-108`: `WARDEN_UPSTREAM_CA_CERTS` is appended to
  `relay.upstream_ca_certs` from the config files, not used instead of them.
  It is an environment override applied after the files (`:95-115`), outside
  UX-07's three levels, so GLOBAL-10 covers it directly; whether
  `configuration.md:65`'s "Extra upstream CA files/dirs" names the merge is
  for the design that touches it.
- `tools/relay-vm/init:61-62`, `:121-122`, `:156-158`: the static NAT
  defaults and `8.8.8.8`; hyperv-relay-per-build-channel removes them.
- `cmd/warden-io/platform_linux.go:91`: CA install falls back to appending
  to a bundle file when `update-ca-certificates` fails.
- `driver/container/environment.go:97`: a full image inspect when the native
  manifest read fails.
- `driver/linuxguest/seediso.go:12-20`: a pure-Go ISO writer when the
  platform tool fails.
- `driver/qemu/winacq.go:165-166`: read-only `-bios` when no UEFI vars
  template exists.
- `driver/hyperv/evalimage.go:184-186`: the eval manifest as "the manifest
  fallback in `resolveBuildBaseVHDX`".

### Relay configuration found outside the claim (not audited; Decisions 6, 8)

Listed for the next design that touches each, as the fallbacks above are.
None is judged here, and `wf-review` gets no exception for them.

Configuration for a build that comes from outside the claim:

- `tools/relay-vm/init:26`, `:73`, `:156-158`, with
  `driver/firecracker/vm.go:62-64`: `warden.dns` on Firecracker's kernel
  command line sets the relay VM's upstream resolvers (written to
  `/etc/resolv.conf`, `8.8.8.8` when unset). hyperv-relay-per-build-channel
  r3 moves upstream DNS into the claim and drops `warden.dns` from the seed
  and the command line (its re-review r2, `8d5beaf`).
- `relay/relay.go:368-383`, `:386-405`: the relay's upstream resolver from
  the `UPSTREAM_DNS` environment variable, else `/etc/resolv.conf`, else
  `8.8.8.8:53`. No warden driver sets `UPSTREAM_DNS` (the init's variable of
  that name is not exported), so it reaches only a third-party or hand-run
  relay; also a GLOBAL-10/GLOBAL-11 case.
- `cmd/relay/main.go:40-41`, `cmd/relay/mode_host.go:22-25`: host mode's
  `--subnet` (default `10.100.0.0/30`) as the build segment when the claim
  carries no `build_link`, kept by build-link-subnet Decision 4 as the
  fallback for an older orchestrator; vz no longer passes it
  (`driver/vz/driver.go:327-333`). Also a GLOBAL-10 case (a boot-channel
  value standing in for an absent claim field).
- `cmd/relay/vmlink.go:38-46`: vm mode without `build_link` keeps the
  address already on its build interface (an older init's, or a third-party
  image's) as its self address and blocked prefix.

Pre-claim values and launch wiring, inside RELAY-08's exception (question
a10), listed so the next design sees each relay's whole boot channel:

- `tools/relay-vm/init:21-24`, `:62-74`: `warden.net`, `warden.nat_ip`,
  `warden.nat_prefix`, `warden.nat_gw` and `warden.claim_token`, the relay
  VM's own upstream address, prefix, gateway and token; Firecracker passes
  all of them (`driver/firecracker/vm.go:62-64`), qemu `warden.net=dhcp` and
  the token (`driver/qemu/vm.go:35`). The init's static defaults
  (`192.168.240.2/20` via `192.168.240.1`, `:62-64`) are hyperv-relay-per-
  build-channel's to remove (BOOT-06 as amended).
- `driver/vz/driver.go:329-334`: the vz host-mode relay's `--claim-addr`
  and `WARDEN_CLAIM_TOKEN` (pre-claim), and `--mode=host --fd=3` (launch
  wiring).
- `driver/container/container.go:633-640`: the container relay's address,
  assigned by the runtime (`--ip`), and `WARDEN_CLAIM_TOKEN` (pre-claim).
- `tools/relay-vm/init:165-171`: `/sbin/relay -mode vm -build-if <if>`
  (launch wiring).

## Defaults and overrides

None. This item adds or changes no product default, override or fallback.

## Cross-driver impact

None. No driver or product code changes.

## Security note

`security_boundary: none`. No build-environment boundary changes. GLOBAL-11
narrows how failures are handled in future designs (a stop with a fix instead
of an unnamed second path), which at a trust boundary removes code paths; a
fallback that remains is named in the design and confirmed in its review, and
at a GLOBAL-06 boundary also in its Security note. RELAY-08 narrows the
channels that configure the relay to the claim, which is authenticated by the
claim token and reachable only host-locally (`relaycfg/relaycfg.go:23-27`),
plus the four pre-claim values; it changes no relay code here.

## Validation plan

- **Tier 0, `tools/workflow/check`:**
  - `check_test.go`'s `tmpl` constant (`:13`) gains the template's
    **Defaults and overrides** text, and its `design` constant (`:32-82`) a
    filled section ("None."), so `TestDesignAndPlanPass` still passes at
    phases 0 to 2.
  - `TestDesignFailures` (`:172-191`): the bad design's section replaced by
    the template's text; the failures include `section filled: Defaults and
    overrides`.
  - `TestProcessChecks` (`:375-399`) gains a case: a process item's phase 1
    with a design that has no **Defaults and overrides** section passes on
    presence and review alone (`check.go:97-103`).
  - A new case in `TestDesignFailures`: a feature at phase 1 whose branch
    template has no **Defaults and overrides** section and whose design has
    none fails `section filled: Defaults and overrides` (the branch-order
    case in Design).
  - `rulechange_test.go` passes unchanged with the updated `design` constant.
- **Tier 0, `tools/workflow/queue` (Decision 9):**
  - `queue_test.go`: a state at `limits.in_progress: 1` with one feature and
    one process item in progress validates; with two features in progress it
    still fails `features in progress, limit 1 (FLOW-05)`; a process item
    with `alters_relationships: true` past Design still counts toward the
    alters limit.
  - Beside `TestNextStartsFeatureOnlyWhenAllBlocked` (`queue_test.go:68-86`,
    its limit assertion at `:82-85`): at `limits.in_progress: 1` with one
    in-progress process item and one queued feature, `Next("cloud")` starts
    the queued feature; with one in-progress feature instead, it starts none.
  - At `limits.in_progress: 1` with one feature in progress, `human/jeff`
    takes and claims a queued process item's `x1-design`; the item becomes
    `in_progress` and `Validate()` returns no error.
  - `TestNextForOneFeature` (`:88-98`): `NextFor("q", "cloud")` on a queued
    feature is not refused when the only other in-progress item is a process
    item at `limits.in_progress: 1`.
- **Tier 1:** `make lint` runs the rule index (`rules.go`): the four new
  headers (GLOBAL-09 to GLOBAL-11, RELAY-08) parse with valid Severity and
  Check lines, and as agent-judgment rules they need no `rules.yaml` entry. `make enforcement`, because the
  branch changes `tools/workflow/check/` (FLOW-14).
- **Check in use:** `dist/wf check` built from the branch, run against a
  scratch copy of a queued feature at phase 1 with and without the section.
- **No native validation.** No tier-2 gaps.

## Implementation plan

Confirmed at x2-implement, after the review (process item, FLOW-03), one
commit each.

1. `docs/development/global.md` (FLOW-14, LINT-01): the preamble's scope
   sentence, and GLOBAL-09, GLOBAL-10 and GLOBAL-11 as worded in Design,
   after GLOBAL-08's `---` and before "Deployment modes and the ledger's
   role".
2. `docs/development/ux-cli-config.md` UX-07, `docs/development/process.md`
   FLOW-19, `docs/development/README.md` (FLOW-14, FLOW-19): the sentence, the
   topic bullet and the description, as in Design. `tools/tier1/rules.yaml`
   is not changed (LINT-02: agent-judgment rules need no entry).
   `docs/development/relay.md` (FLOW-14, LINT-01, RELAY-04, GLOBAL-08): the
   preamble's clause and RELAY-08 as worded in Design, after RELAY-07; the
   README's `relay.md` description (`:91`) in the same commit.
   `tools/tier1/rules.yaml` is not changed for it either. `bootstrap.md` is
   not touched (BOOT-06 is hyperv-relay-per-build-channel's).
3. `docs/design/TEMPLATE.md` (FLOW-01, FLOW-02, FLOW-14): the **Defaults and
   overrides** section after **Design**.
4. `tools/workflow/check/check.go` and `check_test.go` (FLOW-03, TEST-01,
   TEST-02, GLOBAL-01): the section in `design()`'s list, and the tests in
   the Validation plan.
5. `docs/development/process.md` FLOW-05 and `tools/workflow/queue/queue.go`,
   `next.go` and `queue_test.go` (FLOW-05, FLOW-14, TEST-01, TEST-02): the
   limit exemption and its tests, as in Design (Decision 9).
6. `.claude/agents/wf-design.md`, `wf-review.md`, `wf-implement.md` and
   `.claude/skills/workflow-human/SKILL.md` steps 0 and 3.3 (FLOW-14,
   FLOW-07, FLOW-06, FLOW-19, RELAY-08): the sentences in Design, citing rule
   IDs, restating no rule; `wf-design.md` step 2 and `wf-review.md` step 3
   also take the RELAY-08 sentences.
7. `docs/book/src/concepts/overview.md` (FLOW-15): checked; no component flow
   changes, so no pseudocode change is expected.
8. Consolidation, then the gates (FLOW-10, FLOW-17): `make lint`, `make
   test`, `make enforcement`, `tools/workflow/flow18-check.sh`, and `wf
   check` for this item.
9. At x4-close, on the `workflow` branch (FLOW-03): the digest entry asking
   every machine and human session to rebuild `dist/wf` from the merged
   `main`, so the section check and the limit exemption run everywhere;
   `config.yaml`'s and `README.md`'s limit comments; and `limits.in_progress`
   lowered back to 5 from the interim raise (Decision 9).

## Design review

- **r1, 2026-11-09: approved at `d7f3467`** (Jeff; read at `e76cb36`, page
  https://claude.ai/artifact/BTfCd65SvfmtCth7JQBFvp). Assumptions a1 to a10
  confirmed as drafted, (a) each: GLOBAL-09 to GLOBAL-11 in `global.md`
  (SHOULD, MUST, MUST, agent-judgment); the **Defaults and overrides**
  section checked filled by `wf check`; named fallbacks allowed, bounded
  retries unnamed; whole-repository scope; no audit; one sentence in UX-07
  and one FLOW-19 topic; Windows `--ip` left to hyperv-relay-per-build-channel;
  RELAY-08 in `relay.md`, MUST, agent-judgment; the boot channel and launch
  wiring as in a10. Folded in at `d7f3467`, by Jeff's word in the sitting:
  Decision 9, process items not counted toward FLOW-05's in-progress limit
  (rule-running-line-match's start pushed `wf validate` to 6 of 5), with the
  interim raise of `limits.in_progress` to 7 until the rebuilt `dist/wf` runs
  everywhere. The fold-in's citations and tests were checked against the code
  before the commit. No `rule_changes` (process item, FLOW-14).

## Consolidation

Filled in at x2-implement (FLOW-10, FLOW-15), with the branch on `main` at
`49e5354` (its merge base; `main` has not moved since the branch was cut).

- **Landed as designed.** GLOBAL-09 to GLOBAL-11 and the preamble's scope
  (`global.md`); RELAY-08 and its preamble clause (`relay.md`); UX-07's
  sentence, FLOW-19's topic bullet, FLOW-05's limit sentences and the
  README's `global.md` and `relay.md` descriptions; the template's **Defaults
  and overrides** section; `"Defaults and overrides"` in `design()`'s section
  list (`check.go`); `validate` and `atLimit` counting no process items toward
  the in-progress limit, the alters count unchanged, and the `Next` and
  `NextFor` doc comments; the agent and skill sentences. The tests are the
  Validation plan's: in `check_test.go`, the template and design constants,
  the failing section in `TestDesignFailures`, the older-template case
  (`TestDefaultsSectionOnOlderTemplate`, via a `repoWith` helper) and a
  process item's phase 1 without the section in `TestProcessChecks`; in
  `queue_test.go`, `TestLimitSkipsProcessItems` (validate, both limits),
  `TestNextStartsFeatureBesideProcessItem`, `TestHumanStartsProcessItemAtLimit`
  and the process-item case in `TestNextForOneFeature`. The new check and
  queue tests fail with `check.go`, `queue.go` and `next.go` reverted.
- **Not changed, as planned.** `tools/tier1/rules.yaml` (the four rules are
  agent-judgment; tier 1 parses their headers with no finding),
  `bootstrap.md`, `wf-plan`, `wf-publish`, `wf-guide`, `workflow-tick`,
  `workflow-intake`, `workflow-native`, `AGENTS.md`.
- **`overview.md` (FLOW-15, step 7).** Checked: no component flow changes,
  and the page does not describe the queue's limits or the design sections,
  so its pseudocode is unchanged.
- **Overlap with open branches.** None of `feat/*` changes
  `docs/development/`, `docs/design/TEMPLATE.md`, `tools/workflow/` or
  `.claude/` against `main`; each carries only its design. Two designs refer
  to this item: hyperv-relay-per-build-channel r3 is to reword its proposed
  BOOT-06 paragraph to cite RELAY-08 (RELAY-08 holds against BOOT-06 as it
  reads on `main`), and rule-running-line-match is the other process item the
  interim `limits.in_progress` raise counts. Features whose branch predates
  the template change fill the section from `wf-design`'s sentence; no design
  is retrofitted.
- **Left for x4-close (step 9, the `workflow` branch):** the rebuild digest
  entry, `config.yaml`'s and `README.md`'s limit comments, and
  `limits.in_progress` back to 5.

## Rollback notes

None yet.

## Archive summary

Filled in at x2-implement, in the item's PR (FLOW-12).
