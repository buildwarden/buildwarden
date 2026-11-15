---
feature: runtime-cli-project-path
status: draft
drivers: [container, qemu, vz]  # the setting is the container driver's; warden clean also
                         # runs the container runtime under qemu and vz, so it validates
                         # the setting there. The change is in cmd/warden (resolveRuntime
                         # and what feeds it); no driver/ tree is edited
seams: []
surfaces: [cli, config]  # config: [runtime] cli, in the user's file and the project's, and
                         # WARDEN_CTR_CLI take only finch, docker or podman. cli: --runtime
                         # takes only those three
security_boundary: host-env  # a build context can no longer choose a program warden runs on
                         # the host
alters_relationships: false  # inside the orchestrator's existing role: resolving the
                         # container runtime it hands to the container driver. No seam, no
                         # bootstrap change, no change of owner
design_review: required  # FLOW-19: a security boundary is touched, a rule change (UX-08), and
                         # two open assumed questions
native_validation:
  - {host: container-any, blocking: false, reason: "the change reaches no driver tree and its check is one string comparison, the same code on every OS; what only a Windows host with a real runtime adds is that --runtime docker.exe and a path to docker.exe get the message that names docker, that a .bat the project ships is not run, and a build with cli = \"docker\"; make test does not run on Windows in CI"}
  - {host: macos-finch, blocking: false, reason: "the change reaches no driver tree; what only this host adds is a build with cli = \"finch\" in the project's file, the default runtime CI does not run, and the message for finch given as a path; warden clean --driver vz takes the same path through cmd/warden as on Linux, where CI runs it"}
tier2_gaps:
  - {host: container-any, gap: "on a Windows host with a real runtime: go test ./cmd/warden; --runtime docker.exe and [runtime] cli set to the full path of docker.exe are each refused with the fix that names docker; a project warden.toml whose cli is a .bat the directory ships is refused on build, clean and doctor and the file is not run; cli = \"docker\" in the project's file builds (backlog runtime-cli-project-path-windows)"}
  - {host: macos-finch, gap: "a build with cli = \"finch\" in the project's warden.toml; --runtime with the full path of finch is refused on build and on clean --driver vz with the fix that names finch and its directory (backlog runtime-cli-project-path-finch)"}
rules: [GLOBAL-01, GLOBAL-04, GLOBAL-06, GLOBAL-07, GLOBAL-09, GLOBAL-10, GLOBAL-11, ORCH-01,
        ORCH-03, UX-01, UX-03, UX-04, UX-05, UX-07, UX-08, UX-09, TEST-01, TEST-02, TEST-03,
        TEST-06, TEST-08, SEC-01, SEC-03, SEC-04, LINT-01, LINT-08, FLOW-01, FLOW-02, FLOW-07,
        FLOW-09, FLOW-10, FLOW-12, FLOW-14, FLOW-15, FLOW-19, FLOW-20, LOOP-04, PROFILE-01]
rule_changes:
  - {rule: UX-08, change: amend, paths: [docs/development/ux-cli-config.md, tools/tier1/rules.yaml]}
concepts: [container-runtime, runtime-cli, supported-runtimes, config-validation,
           consumer-validates, lazy-validation, project-config, build-context-trust,
           host-execution, path-lookup, transitional-detection, security-fix]
related: [build-link-subnet.md, hyperv-relay-per-build-channel.md, design-principles.md]
---

# The container runtime is one the container driver supports, whoever names it

## Problem

`warden` reads `./warden.toml`, the file in the working directory, on every
command that loads configuration. In a cloned repository that file is the
repository's. Its `[runtime] cli` is taken as the container runtime's program
and run on the host:

- `LoadConfig` (`cmd/warden/config.go`) decodes the user's
  `~/.config/warden/config.toml` and then `./warden.toml` into one `Config`,
  and `WARDEN_CTR_CLI` and `--runtime` replace the result.
- `resolveRuntime` (`cmd/warden/main.go`) returns `cfg.Runtime.CLI` as it is
  whenever it is not empty. Nothing checks it.
- The container driver sets `ctrctl.Cli = []string{d.Runtime}` and runs it
  with `exec.Command` (`driver/container/container.go`, `images.go`,
  `environment.go`); `warden clean` does the same (`cmd/warden/clean.go`).

Go's `os/exec` uses a name that contains a path separator as a path, relative
to the working directory. So this file, in a repository:

```toml
[runtime]
cli = "./tools/docker"
```

makes `cd repo && warden build` run `repo/tools/docker` on the host, as the
user, with `warden`'s arguments, before any container or network exists.
Nothing has to be built and nothing in the output records it.

A name with no separator is no better. `warden`'s first argument to the
runtime is a fixed word (`pull`, `image`, `container`, `info`), and an
interpreter runs a file of that name from the working directory:

```toml
[runtime]
cli = "sh"
```

with a file named `pull` in the repository makes a release `warden build`
run `sh pull ghcr.io/buildwarden/relay:latest`, which executes `./pull`;
`warden clean` runs `./container` the same way. Checked for the first draft:
`sh pull x` and `sh container ls` each ran the file of that name in the
working directory. `python3`, `node`, `perl` and `ruby` behave alike.

The decoder matches a key to its field without regard to case, so
`[Runtime]` and `CLI = "./tools/docker"` are the same setting (run at
`897a4a8`: `warden clean` ran the file).

Which commands resolve the container runtime, read from the code and run
where the table says so. `cmd/` and `driver/` are the same at `ad4ea50` as
at `897a4a8`, where the first draft ran them:

| Command | Resolves the container runtime, and so runs `[runtime] cli` today |
|---|---|
| `warden build`, `warden shell`, with the container driver (the default) | yes (`newDriver`): first `<cli> pull <relay image>` (a release build) or `<cli> image build ...` (a development build) |
| `warden clean`, with every driver but `firecracker` and `hyperv`: the container driver, `qemu` and `vz` | yes (`runClean`): `<cli> container ls ...`, then `network ls`, `image ls`. Only `firecracker` and `hyperv` return before the runtime is resolved. Run at `897a4a8` with `driver = "qemu"` beside `cli = "./rt"` in the project's file, and with `--driver vz`: the file ran three times |
| `warden doctor`, with the container driver | it resolves the value and prints `container runtime: <cli>` and `ok` without running it; only autodetection runs `<name> info` |
| `build`, `shell` and `doctor` with `--driver qemu`, `vz`, `hyperv` or `firecracker`; `clean` with `firecracker` or `hyperv`; `setup` and `image prepare` with any driver | no: the runtime is not resolved |

`image list`, `image verify`, `inspect` and `hyperv service` load no
configuration.

Found on 2026-11-14 by the reviewers of `driver-contract`'s redraft. Jeff,
the same day: "File it as a security fix". The first draft of this design
(at `b8f692f`) kept the setting open to any program from the flag, the
variable and the user's file, and limited only the project's file to three
names. Jeff read it on 2026-11-15 and answered its three questions, each
departing from the draft (**Questions**). This redraft is what those answers
give, and it is simpler: there is no rule about where the value came from.

This is a feature item and not a program item: its `related` does not list
`componentization.md`. FLOW-20's freeze exempts it as a fix for a security
problem. It works only in this repository (LOOP-04).

## Prior art

- **`driver-contract`** (K1, in its design review; not on `main`). It moves
  the reader of this setting into the container driver, which reads its own
  flags, variables and keys through the kit and validates them in a probe,
  "before any real work", and only when that driver is selected (its
  Behaviour 13). That is the rule Jeff stated in a2, applied to drivers. K1
  also renames the key to `[driver.container] cli`, with `[runtime] cli` a
  deprecated second name, and records this item as a follow-up it does not
  carry. This design lands first, on today's code; what K1 has to keep is
  in **What K1 has to keep**.
- **`build-link-subnet`** and **`hyperv-relay-per-build-channel`**
  (archived). They put the checks of `[vm] build_link` and `[vm] dns` in
  `loadTOML`, "refused by every command as the file loads", and cite UX-08
  for it. The general rule of a2 says otherwise for a run that does not use
  those keys. This item does not move those checks; the UX-08 amendment
  names them (**Rule changes**, question a5).
- **`design-principles`** (archived): GLOBAL-09 to GLOBAL-11. A provided
  value that cannot be used is an error that names it and its fix; no lower
  level and no default is tried in its place.
- **`native-allowlist-scratch-tools`** (archived) accepts `WARDEN_CTR_CLI`
  in the native launcher's allowlist although it "chooses the container CLI
  `warden` runs". After this item the variable chooses among three names.
- **Rules:** GLOBAL-06 (the host and build-environment boundary), UX-04 and
  UX-09 (the error), UX-08 (who validates, and when: amended here), UX-03
  (Decision 5), UX-07 and ORCH-03 (the precedence, unchanged), UX-01 and
  FLOW-15 (the docs), SEC-01, SEC-03 and SEC-04 (the corpus entry in the
  Security note), FLOW-14 (a feature's design may change the rules of a
  component it works in: UX-08 is a project rule, in `ux-cli-config.md`).

No archived design covers which values the runtime setting may take, or
states who validates a setting. The designs this one relies on carry
front-matter already.

## Decisions

1. **The container driver's runtime is `finch`, `docker` or `podman`, or
   not set. From every source.** `--runtime`, `WARDEN_CTR_CLI`,
   `[runtime] cli` in the user's config file and `[runtime] cli` in the
   project's `warden.toml` take the same three values, and anything else is
   refused, whichever of the four set it (a1). Those are the runtimes the
   driver supports today: the names `DetectRuntime` probes, in that order,
   and the only values the flag's help, the configuration reference, the
   README and `warden doctor`'s help list. The edges:
   - *Letter case.* The match is exact and in lower case. `Docker` is
     refused, on every OS. A lookup of `Docker` finds `docker` on macOS and
     Windows and not on Linux, and the driver's own switch on the runtime's
     name (`copyExecutor`) compares exactly, so accepting it would mean one
     configuration with three behaviors.
   - *An extension.* `docker.exe` is refused, on Windows too. `os/exec`
     finds `docker.exe` for the name `docker` there (`PATHEXT`), so the name
     is enough.
   - *Surrounding space.* ` docker` and `docker ` are refused. Nothing is
     trimmed (GLOBAL-11).
   - *An empty value*, unchanged from today for each source. An empty
     `--runtime` and an empty `WARDEN_CTR_CLI` are not set, and the next
     level is read. `cli = ""` in a file is that file's value: in the
     project's file it hides the user's, and the runtime is detected.
   - *Not set anywhere:* detection, as today: the first of the three that
     answers `info`.
   - *The lookup.* An accepted name is run by that name, and `os/exec`
     looks it up on `PATH` each time, as today. What that leaves is in the
     Security note.

   `Docker` and `docker.exe` ran before this fix on some hosts, so the
   transitional detection of Decision 4 gives each a message that names
   the spelling to use.
   - *Rejected: limit only the project's file* (the first draft). Jeff: the
     setting is the container driver's, "limited to the runtimes it already
     knows it supports", from every source. One rule, and no code that has
     to know which file a value came from.
   - *Rejected: add `nerdctl`.* The driver has one switch arm for the name
     (`copyExecutor`: `finch`, `nerdctl`), but `warden` neither probes for
     it nor documents it, and no native host runs it. That arm can no
     longer be reached through `warden`; it is left for K1, which moves the
     driver, because this item edits no driver tree.
   - *Rejected: normalize* (lower the case, strip `.exe`, trim). It makes
     more spellings work, each for no gain over writing the name, and the
     value printed in output would no longer be the value written.
2. **The consumer validates, when it initializes, after the overrides, and
   only when the run uses the setting** (a2). The consumer of this setting
   today is `resolveRuntime` in `cmd/warden`: the one function that reads
   it, on behalf of the container driver it constructs and of `warden
   clean`. It checks the value in effect: the flag if set, else the
   variable, else the project's file, else the user's. So:
   - `[runtime] cli = "nonsense"` in a `warden.toml` is not an error for
     `warden build --driver hyperv`, or for any command in the last row of
     the Problem's table. The setting is not read there, as today.
   - It is an error for a command that resolves the container runtime, when
     it is the value in effect.
   - `warden build --runtime docker` in that directory builds: the value in
     effect is valid, and the file's value is not looked at. The same holds
     for `WARDEN_CTR_CLI=docker`, and for a bad value in the user's file
     that the project's file replaces.

   The commands, and what each does with a value that is not supported:

   | Command | With an unsupported value in effect |
   |---|---|
   | `warden build`, `warden shell`, container driver | the error, from `newDriver`, before the runtime is looked up or run and before an output directory, a container or a network is made. `newRequest` runs first and only parses flags and stats the build path |
   | `warden doctor`, container driver | the error; doctor reports what a build would get |
   | `warden clean`, container driver, `qemu`, `vz` | the error, before anything is listed or removed. Under `qemu` and `vz` this command cleans containers too, so the run uses the setting |
   | every other command and driver | nothing: the setting is not read. `--runtime nonsense` with `--driver qemu` is accepted and ignored, as `--runtime` is today |

   This is a general rule, and Jeff asked for it to be written as one, in
   this item: the UX-08 amendment (**Rule changes**).
   - *Rejected: validate as the file loads, on every command and driver*
     (what `[vm] build_link` does). It fails a run for a setting the run
     does not use, which a2 rules out, and it checks a value a flag
     replaced.
   - *Rejected: skip an unsupported value and use the next level or
     detection.* That is a fallback on failure (GLOBAL-10, GLOBAL-11).
3. **The value that is run is the list's own.** `resolveRuntime` returns
   the member of the supported list that the configured value equals, never
   the configured string. `Runtime.CLI` is read nowhere else (checked by
   search at `ad4ea50`), so no value reaches `exec` unchecked, and the check
   does not depend on which file set the key or how the key was spelled:
   `CLI`, `[Runtime]`, a dotted key and an inline table all end in the one
   field, and the field is what is checked.
4. **The error has a permanent part and a transitional part** (a3). It is
   an error from the first release that carries the fix; nothing warns and
   runs.
   - *Permanent:* the consumer's own validation error, a
     `driver.FixableError` (UX-04), exit 1 (UX-09). It says what was set
     (printed with `%q`), where (the flag, the variable, or the file by its
     path), the three supported values, and what to do.
   - *Transitional:* detection of the forms that ran before this fix and
     are refused now, each with a message that says what to change. It
     changes only the wording: every value it recognizes has already failed
     the permanent check. It lives in one file, behind one call, with one
     marker, and is removed later, as a deprecation warning would be
     (question a4 for when).

   Both are in **Design**.
5. **Values that work today stop working, from every source, at once
   (UX-03).** No flag or key is renamed or removed, so UX-03's alias does
   not apply as written, and its "never a silent break" is met by the
   error. The reason differs by source and is stated in **Compatibility**:
   for the project's file it is the security hole; for the flag, the
   variable and the user's file it is Jeff's decision that the driver
   supports the runtimes it knows.
6. **UX-08 is amended in this item**, and its wording names the checks on
   `main` that it does not yet describe. This item brings the runtime
   setting under the rule and moves none of the older checks (question a5).
7. **The other findings of the first draft are two queue items now**, not
   this one's: `project-config-trust` and `configuration-bugfixes` (**The
   same defect elsewhere**).

What falls away from the first draft: the project-level special case and
the table of what each source may hold; `Runtime.RefusedCLI`; and the
case-insensitive walk of the decoder's metadata. None is needed when every
value in effect is checked against one list. Which file set the value is
still noted, but only to word the error: nothing that decides what is run
depends on it.

## Questions

### Blocking

None.

### Assumed

- **a1. What may `[runtime] cli` in a project's `warden.toml` be?**
  Answered, changed. Jeff, 2026-11-15: "If this is specifically for the
  container driver, then it should be limited to container-specific
  configuration which that driver should be limited to the runtimes it
  already knows it supports". Asked whether the limit holds for every
  source or only for a project's file, he chose "Every source". Decision 1.
  - (a) Exactly `finch`, `docker` or `podman`, or empty. Was recommended,
    for the project's file only.
  - (b) Those three and `nerdctl`.
  - (c) Any name without a path separator, looked up on `PATH`.
  - (d) Nothing: the key is read from the user's file only.
- **a2. When is a bad value an error?** Answered, changed. Jeff: "Throughout
  the project, there should be a general rule that configuration checking
  should be validated by the consumer/user of that configuration on
  initialization after applying overrides like environment variables. This
  is only enforced/required for all of the software components within the
  buildwarden organization and highly recommended for those extending or
  writing their own components, but isn't enforced programmatically. But
  since it is likely that the same warden.toml may have all of the
  configuration for multiple driver implementations where there is a loop
  running warden builds for different operating systems/architectures
  configured through CLI (not unlike its own cross-platform build), then it
  is reasonably lazily validated to only the configurations for the
  specific driver(s) actually used for a specific run. In other words, it
  should allow for configuration sections/configurations that may not be
  used and so aren't validated". Asked where the rule is written, he chose
  this item, amending UX-08. Decision 2 and **Rule changes**.
  - (a) When it is the value in effect on a command that resolves the
    container runtime. Was recommended.
  - (b) As the file loads: every command that loads configuration, every
    driver, even when a flag overrides it.
- **a3. Does the fix take effect at once (UX-03)?** Answered, changed.
  Jeff: "Use the logic that would be used for the warning to instead have a
  concrete error message about what needs to be changed/updated. This logic
  will eventually be removed like the warning would be". Decision 4.
  - (a) Refuse from the release that carries the fix. Was recommended.
  - (b) Warn for one release, then refuse.
- **a4. When is the transitional detection removed?** Proceeding with (a);
  confirm. After its removal the forms it recognizes get the permanent
  error, which lists the three supported values.
  - (a) It is in the release that first carries the fix and in the two
    releases after it, and is removed from the third. That is the window
    `driver-contract` proposes for its deprecated key names ("at least two
    releases"), the only window the project has proposed so far. Publish
    files the removal as its own small item. **Recommended.**
  - (b) The release that first carries the fix and the one after it.
  - (c) Removed with the deprecated key name `[runtime] cli`, in the change
    that ends `driver-contract`'s window: one removal for the container
    driver's whole transition, at a date that depends on that item.
  - (d) Kept until 1.0.
- **a5. How do UX-08 as amended and `main` agree on the day this merges?**
  Proceeding with (a); confirm. Four checks on `main` are made more widely
  than the amended rule says (the table in **Rule changes**), and the
  umbrella's Decision 18 has a rule change in the item that makes it true.
  - (a) The amended wording lists the four by name as checks that predate
    it; each stays until the change that moves it to its consumer, and this
    item moves none. `driver-contract` already makes two of them true (the
    type of the driver-owned keys, and the two image ages) and strikes
    them. `[vm] build_link`, `[vm] dns` and the type of `warden`'s own keys
    are put to `driver-contract`'s review, whose Decision 8 keeps `warden`
    checking the link and the resolvers as the files load.
    **Recommended.**
  - (b) As (a), but `configuration-bugfixes` moves `[vm] build_link` and
    `[vm] dns` (its finding F8 already reworks those two checks). It is on
    hold behind the freeze, so the two stay listed until the program ends.
  - (c) This item also moves `[vm] build_link` and `[vm] dns` to the
    VM-driver commands, after `--build-link`. The rule lists two checks
    fewer; the security fix changes two more keys for every VM driver,
    undoes what `build-link-subnet` (its a10) and
    `hyperv-relay-per-build-channel` confirmed, and edits the lines
    `driver-contract` rewrites. The type check still has to be listed: it
    cannot move without a reader for each table.
  - (d) The wording lists nothing, and `main` is out of step with UX-08
    until those checks move: the shape Decision 18 rejected.

## Design

### The rule

| Where the runtime is named | `finch`, `docker`, `podman` | Anything else |
|---|---|---|
| `--runtime` | used | refused |
| `WARDEN_CTR_CLI` | used | refused |
| `[runtime] cli` in `./warden.toml` | used | refused |
| `[runtime] cli` in `~/.config/warden/config.toml` | used | refused |
| nowhere | the first of `finch`, `docker`, `podman` that answers `info` | |

A row is refused only when it is the row in effect, on a command that
resolves the container runtime (Decision 2). Precedence is unchanged and is
the table's order, top to bottom (`LoadConfig` and `resolveConfig` today;
ORCH-03, UX-07): the highest level that sets the value is the value
(GLOBAL-10).

### In the code

All of it is in `cmd/warden`. `driver/container` is not edited: it runs the
`Runtime` it is given, which is now always one of three constants. The list
and its check sit beside `DetectRuntime`, where the knowledge of which
runtimes exist is today; K1 moves the list, the check and detection into
the container driver.

```
containerRuntimes = [finch, docker, podman]   # the supported list, in
                                              # detection's order

LoadConfig:                    # precedence as today; each step that sets cli
                               # also notes where it came from, for the error
    load ~/.config/warden/config.toml         # source: that file's path
    load ./warden.toml                        # replaces; source: warden.toml
    WARDEN_CTR_CLI not empty                  # replaces; source: the variable
resolveConfig:
    --runtime not empty                       # replaces; source: the flag

resolveRuntime(cfg):           # the consumer: build, shell (newDriver) and
                               # doctor with the container driver; clean with
                               # every driver but firecracker and hyperv
    v := cfg.Runtime.CLI                      # the value in effect
    v == ""                -> DetectRuntime() # ranges over containerRuntimes
    v in containerRuntimes -> that member of the list
    otherwise              -> the error; nothing is looked up, run or created
```

- The source is a field beside the value (`toml:"-"`, as `BuildLinkSource`
  is). Which file set a key is decided by what the file's decode did to the
  field, not by the decoder's `IsDefined`, which compares a key's spelling
  exactly while the decoder also fills the field from `CLI` or `[Runtime]`:
  before the project's file is decoded the field is given a marker value,
  and a file that leaves the marker did not set the key, so the user's
  value and source are put back. The source is used for the error's wording
  and for nothing else: a wrong source cannot make a value run.
- A table that holds the key in two spellings (`cli = "docker"` and
  `CLI = "./rt"`) gives one of its values, which one varies from run to
  run. That value is checked, so the result is the named runtime or the
  error, never an unsupported value run.
- A wrong TOML type (`cli = 5`) fails the load as today, on every command
  that loads configuration. It is one of the checks the UX-08 amendment
  lists as older than the rule; K1 makes it the driver's.
- An unsupported value that a higher level replaces is not reported.
  Nothing reads it.
- The transitional detection is in its own file (below).

### The error

**Permanent.** For a value that is not a supported runtime and is not one
of the forms below:

```
ERROR: [runtime] cli in warden.toml is "dokcer"; the container driver supports finch, docker and podman
To fix: set it to finch, docker or podman, or remove it; --runtime with one of those replaces it for one command
```

The first clause names the source: `--runtime`, `WARDEN_CTR_CLI`,
`[runtime] cli in warden.toml`, or `[runtime] cli in <the user's file, by
its path>`. The key is named in its documented spelling, however the file
wrote it. The fix follows the source: "pass `--runtime` finch, docker or
podman, or leave it out"; "set `WARDEN_CTR_CLI` to finch, docker or podman,
or unset it"; for a file, the text above, whose last clause is the way to
build someone else's repository without editing it. The names come from
`containerRuntimes`. The value is printed with `%q`: it may come from a
file someone else wrote, and `%q` keeps control characters out of the
terminal and shows a stray space. It goes to stderr and `warden` exits 1
(UX-09: not a build's own exit).

**Transitional.** Four forms ran before this fix. For each, the same
source clause and a message of its own:

| The value | Example | What the message says | The fix it gives |
|---|---|---|---|
| A supported runtime in another letter case or with `.exe` | `Docker`, `docker.exe` | the runtime is named in lower case, without an extension | the name: `docker` |
| A path whose last element is a supported runtime (after the row above) | `/opt/homebrew/bin/docker`, `C:\Program Files\Docker\docker.exe`, `./tools/docker` | the runtime is named, not given as a path | the name, and for an absolute path: add its directory to `PATH` if the name is not found. For a relative path no directory is suggested: `warden` looks the name up on `PATH` and does not run the file |
| `nerdctl`, alone or as a path's last element | `nerdctl`, `/usr/local/bin/nerdctl` | the container driver supports `finch`, `docker` and `podman` and no longer runs another runtime | one of the three; and that there is no supported way to use `nerdctl` with the container driver |
| A path to anything else | `/usr/local/bin/docker-ci`, `./rt` | the container driver runs `finch`, `docker` or `podman` by name and no longer runs another program | one of the three; if the file wraps one of them, set what it set (`DOCKER_HOST`, `DOCKER_CONTEXT`, `CONTAINER_HOST`) in the environment |

For example:

```
ERROR: WARDEN_CTR_CLI is "/opt/homebrew/bin/docker"; the container runtime is named, not given as a path
To fix: set WARDEN_CTR_CLI to docker, and add /opt/homebrew/bin to PATH if docker is not found there
```

A bare name that is neither a supported runtime nor `nerdctl` (a wrapper
on `PATH`, a typing error, `sh`) cannot be told apart from one another, and
gets the permanent error, which already says what the supported values are.

About `nerdctl` and other runtimes, plainly: there is no supported route
today. Before this fix `WARDEN_CTR_CLI=nerdctl` ran `nerdctl`, untested and
undocumented. After it, the container driver runs three runtimes, and a
runtime of one's own is a driver of one's own, which `warden` can run only
once the driver contract (K1) has landed. Until then the answer is one of
the three; Finch is the supported runtime built on `nerdctl`. A file named
`docker` on the user's own `PATH` that runs something else is the user's
arrangement: `warden` runs it as `docker`, and it is neither tested nor
supported.

Where the transitional part lives, so that it can be deleted in one place:

- One file, `cmd/warden/runtime_former.go`, with one function that takes
  the value and returns the message and fix for a form it recognizes, or
  nothing. It uses string operations, not `path/filepath`, so `/`, `\` and
  a drive letter are read the same on every OS and its tests are one table
  everywhere.
- One call, in the function that builds the permanent error, made only
  after the value has failed the permanent check. Deleting the file and
  that call leaves the permanent error for every one of these values.
- One test file, `runtime_former_test.go`.
- One subsection of the configuration reference ("A path, or another
  runtime").
- One marker on each of the four: `TRANSITIONAL(runtime-former-values)`,
  in the file's header comment (which states why the code exists and when
  it goes: GLOBAL-01), at the call, at the head of the test file, and in an
  HTML comment above the docs subsection. A search for the marker finds
  everything to delete.
- When: question a4. Proceeding with: present in the release that first
  carries the fix and the two after it. Publish files the removal as a
  queue item whose `source` names the marker and that condition.

### Compatibility (UX-03)

A user-facing change of behavior, from every source. What stops working:

| Today | After |
|---|---|
| `--runtime /path/to/docker`, `WARDEN_CTR_CLI=/usr/local/bin/podman`, or `cli = "/opt/homebrew/bin/finch"` in either file | refused. Name the runtime, and put its directory on `PATH` |
| `--runtime docker.exe`, `Docker` | refused. Write `docker` |
| `WARDEN_CTR_CLI=nerdctl`, or another runtime's program | refused. No supported route today (above) |
| A wrapper script, by path or by name (`docker-ci`, a script that sets `DOCKER_HOST` and runs `docker`) | refused. Name the runtime and set the wrapper's variables in the environment |
| `cli = "./tools/docker"` or `cli = "sh"` in a project's `warden.toml` | refused. This is the hole |

Who is likely affected:

- A user whose runtime is installed outside `PATH`, most often in a CI
  job or a service with a short `PATH`, who pointed the flag or the
  variable at it. They add the directory to `PATH`.
- A user of `nerdctl` without Finch (containerd on Linux, Rancher Desktop
  or Lima in containerd mode). Nothing in the repository tests or documents
  that use; whether a build completes that way today was not checked.
- A user who runs the Linux `warden` under WSL against Docker Desktop
  through `docker.exe`. Docker Desktop's WSL integration provides `docker`.
  Whether a build completes through `docker.exe` today was not checked.
- A user with a wrapper around a supported runtime.
- Nobody in this repository: its tests put a fake named `finch` or
  `podman` on a `PATH` of their own (`fakeRuntime`), `test-integration.sh`
  relies on detection, the native sessions pass `--runtime finch`, and no
  script, workflow or skill sets a path (searched at `ad4ea50`, and in the
  workflow branch's state).

Why, by source. For the project's file the reason is the security hole,
and it is why the change cannot wait for a warning period. For the flag,
the variable and the user's file there is no hole: they are the user's, and
the first draft left them alone. They are limited because Jeff decided the
container driver supports the runtimes it knows, from every source (a1),
and that an unsupported value is an error at once, with a message that says
what to change (a3). The values that stop working are not documented
ones: the flag's help, the configuration reference and the README list
`finch`, `docker` and `podman`. One line is looser, the reference's
"Container runtime binary name" for `WARDEN_CTR_CLI`.

### What changes for a user

- The runtime setting takes `finch`, `docker` or `podman` and nothing
  else, wherever it is set (the table above).
- A value that is not one of them is an error on `build`, `shell` and
  `doctor` with the container driver and on `clean` with the container
  driver, `qemu` or `vz`, when it is the value in effect. Today it is run.
- It is not an error on any other command or driver, and not when a higher
  level replaces it.
- Unchanged: the precedence; detection and its order; an empty value; a
  supported name from any source; `--runtime` accepted and ignored by the
  VM drivers.

### Reference docs (UX-01) and the overview (FLOW-15)

- `docs/book/src/guide/configuration.md`: the `cli` line of the Full
  Reference; a section on the container runtime in place of "Runtime
  Autodetection": the three values and that they are the same from every
  source, the precedence, detection, which commands check the value and
  that a run with another driver does not, the error, and the marked
  subsection "A path, or another runtime" with the Compatibility table;
  the `WARDEN_CTR_CLI` row ("finch, docker or podman") and the `--runtime`
  row.
- `docs/book/src/guide/cli.md`: `--runtime <name>` under Global Flags
  (the three are the only values); one sentence in `warden doctor` and one
  in `warden clean` (an unsupported runtime in effect fails the command;
  for `clean`, under every driver that cleans containers: all but
  `firecracker` and `hyperv`).
- `cmd/warden/main.go`: the help of `--runtime` reads "container runtime
  (finch, docker, podman)" today and stays.
- `README.md` and `docs/book/src/getting-started/installation.md`: what
  they say stays true; `installation.md` gains the link to the section
  above.
- `docs/book/src/concepts/overview.md`, in `warden build`, after the
  `req := resolve flags and config` line:

  ```
      container driver: runtime := --runtime, else WARDEN_CTR_CLI, else
                  [runtime] cli (./warden.toml, then the user's file), else
                  the first of finch, docker, podman that answers; a value
                  that is not one of those three is refused here, before
                  anything is run
  ```

  with a note that `warden clean` resolves the runtime the same way under
  every driver but `firecracker` and `hyperv`.

### The same defect elsewhere

The first draft surveyed every key a project's `warden.toml` sets and
listed nine findings. Jeff had them filed on 2026-11-15 as two queue items,
and they are not this item's:

- **`project-config-trust`** (a security fix, one design): F1 to F4 and F9
  (a project's file chooses the relay image, the relay's trust roots, host
  directories and the Hyper-V base image; `warden` takes the working
  directory for its own source tree), and the question of what a project's
  file may decide at all, including that it outranks the user's own file.
- **`configuration-bugfixes`** (one feature, on hold behind the freeze):
  F5 to F8 (two documented keys that are never read; `doctor` printing `ok`
  for a runtime it never ran; a file in the working directory hiding an
  installed runtime; load-time checks skipped for a key in another letter
  case).

The survey table and the text of each finding are in this file at
`b8f692f`, and in the two items' `source`. What this item changes for them:
F8 no longer touches `[runtime] cli`, whose check does not depend on the
key's spelling; F6 and F7 stand as they were. One more observation for
`configuration-bugfixes`, found while reading for the UX-08 amendment:
`[output] color` and `--color` are not validated at all (an unknown value
is read as `auto`).

### What K1 has to keep

K1 (`driver-contract`) moves this setting out of `cmd/warden`'s `Config`:
the container driver reads it through the kit, from `--runtime`,
`WARDEN_CTR_CLI` and the config files `warden` names to it, under the key
`[driver.container] cli` with `[runtime] cli` a deprecated second name.
This item merges first; K1 meets it in its consolidation (FLOW-10). The
requirement below is for K1's Implementation plan and is handed to that
item; this design does not edit K1's.

1. **The rule.** The container driver validates its runtime against its
   supported list (`finch`, `docker`, `podman`) when it initializes: in the
   probe of `run`, and first in each other command of its own that resolves
   the runtime. From every source and under both key names, on the value
   the kit resolved after its precedence. The value run is the list's own.
   An unsupported value is this design's error, naming the source as the
   kit labels it, exit 1, before the driver looks up, runs or creates
   anything.
2. **Only when used.** A run that selects another driver does not read the
   setting (K1's Behaviour 13). After K1's Behaviour 5, `warden clean`
   under `qemu` and `vz` no longer runs the container runtime, so it no
   longer validates the setting: that one case's expected result becomes
   Behaviour 5's, and its other assertion stays (the marker file is
   absent).
3. **No fallback.** An unsupported value in effect is not replaced by a
   lower level's or by detection.
4. **A later command.** K1's `reap` and `attach` take the runtime from the
   driver's state file, written during `run`; the value written there has
   passed the check. Where the driver resolves its settings again, the
   check runs again.
5. **The transitional detection moves with the check**, stays one file
   behind one call, keeps its marker and its removal point (a4). Its
   `nerdctl` message can then name the route: a driver of one's own.
6. **The tests.** The integration cases run the built `warden` against a
   project directory and must pass on K1's branch unchanged, but for the
   one case in point 2. The unit tables move with the code. K1 adds, for
   `[driver.container] cli`, the rows this item has for `[runtime] cli`,
   the marker-file case among them: a K1 that checked the old name only
   would pass every test this item writes.
7. **UX-08.** K1's design quotes UX-08 as it is before this item and says
   it does not change it. After this merges, K1 quotes the amended rule,
   and strikes from its list the checks K1 moves to a driver: the type of
   the seven driver-owned keys and the two image ages. Whether K1 also
   takes `[vm] build_link`, `[vm] dns` and the type of `warden`'s own keys
   is question a5. K1's proposed ORCH-03 paragraph ("A driver reads its
   own ... with the same precedence and before it does any work (UX-08)")
   and its Errors table ("a driver's key, while another driver is
   selected: it is not read") are this rule applied to drivers and need no
   change for it.
8. **K1's own text.** Its Questions (a25: "whichever of the two merges
   second"), Discovery and trust ("K1 leaves it as"), Configuration ("K1
   does not carry the fix"), Risks and Security note ("is not K1's") say
   that a project's `[runtime] cli` is run as given, or leave open which
   change merges second. Once this merges, K1 is the second. Its lifecycle
   suite's example of a host setting passed as a config file is a runtime's
   name; a runtime that is not on `PATH` needs `PATH`, not a path.

If K1 were to merge first, the code this item changes would be gone and it
would return to Design (FLOW-06).

### What this item does not do

- It does not resolve a name to an absolute path once, or check who owns
  the file found. The runtime is looked up by `os/exec` as today.
- It does not move the checks of `[vm] build_link`, `[vm] dns`, the image
  ages or TOML types (a5).
- It does not fix F1 to F9.
- It does not edit `driver/container/`, the `nerdctl` arm of `copyExecutor`
  included.

## Defaults and overrides

| Default, override or fallback | Status | Why it is the obvious choice (GLOBAL-09) | What overrides it, exclusively (GLOBAL-10) |
|---|---|---|---|
| The container runtime when nobody names one: the first of `finch`, `docker`, `podman` that answers `info` | Unchanged | Today's detection | Any level that sets the runtime |
| The values the runtime setting takes: `finch`, `docker`, `podman`, from every source | **Changed**: it was any string, run as given (a1) | They are the runtimes the driver supports, probes for and documents; the same answer from every source | Nothing: another runtime is another driver |
| The spelling is exact: lower case, no extension, no surrounding space | **New** (Decision 1) | The name as documented; one behavior on every OS (GLOBAL-11) | Nothing |
| An empty `--runtime` or `WARDEN_CTR_CLI` is not set; `cli = ""` in a file is that file's value and means "detect" | Unchanged | Today's behavior, and what K1 states for every driver | As today |
| The supported list and detection's list are one list | **New** | Two lists would drift | Nothing |
| The transitional detection: a more specific message for four forms that ran before | **New**, and temporary (a3, a4) | It tells a user whose configuration stopped working what to change | Nothing; it never changes whether a value is accepted |

No merge and no fallback is added. An unsupported value is an error where
it is the value in effect: a lower level and detection are not tried in its
place (GLOBAL-10, GLOBAL-11). An unsupported value that a higher level
replaces is not used and not checked, which is precedence, not a fallback.
The transitional detection is not a fallback either: it runs only after the
permanent check has refused the value, and the outcome is the same error
with other words. No relay setting is added or moved (RELAY-08 does not
apply).

## Cross-driver impact

- **container**: the commands that resolve its runtime (`build`, `shell`,
  `doctor`, `clean`) accept `finch`, `docker` or `podman` from every source
  and refuse anything else. `driver/container/` is not edited, and a build
  whose runtime is one of the three, or detected, runs exactly as before.
- **qemu, vz**: affected through `warden clean` only. Under them that
  command also removes leftover containers, so it resolves the container
  runtime (`cmd/warden/clean.go`): today it runs whatever is configured,
  and after this it refuses a value that is not one of the three. `build`,
  `shell` and `doctor` under them do not read the setting, as today. No
  file of theirs changes.
- **hyperv, firecracker**: unaffected. No command under them resolves the
  container runtime (`clean` returns before it), so the setting is not
  read, as today.

No shared `driver/` file changes (ORCH-01).

## Rule changes

One entry: **UX-08, amend**. Paths: `docs/development/ux-cli-config.md`
(the rule) and `tools/tier1/rules.yaml` (its entry in tier 1's map). UX-08
is a project invariant of the component this item works in, not a loop rule
(FLOW-14).

**Current wording** (`docs/development/ux-cli-config.md` on `main` at
`ad4ea50`):

> ### UX-08 — Fail fast on config
>
> **Severity:** MUST
> **Check:** automated + agent-judgment
>
> Configuration errors or mismatches are validated and surfaced as early as possible,
> at load/parse/startup, before any real work begins, rather than failing deep into a
> build. The automated part checks that config validation runs at startup.

**Proposed wording:**

> ### UX-08 — Fail fast on config, in the component that uses it
>
> **Severity:** MUST
> **Check:** agent-judgment
>
> A component validates the configuration it consumes, errors and mismatches
> alike, when it initializes: after the overrides are applied (flags, environment
> variables, and the config files in precedence order, UX-07) and before any real
> work begins, rather than failing deep into a build. What it checks is the value
> in effect, not a value that a higher level replaced.
>
> Only the configuration a run uses is validated. One `warden.toml` may hold the
> settings of several drivers or components, for runs that choose among them on
> the command line; a setting the run does not use is not checked, and a bad
> value there is not an error. A config file that cannot be parsed is an error
> for every run that reads it.
>
> This is required of every component in the BuildWarden organization and
> strongly recommended for components others write. It is not enforced
> mechanically. The agent part judges, for each setting a change adds or touches,
> that the component that uses it validates it at that point, and that nothing
> validates a setting it does not use.
>
> Checks that predate this wording still run as a config file loads, whatever the
> run uses: the TOML type of every key; `[vm] build_link` and `[vm] dns`; and, on
> every `build` and `shell`, `[vm] image_refresh_after` and `image_min_grace`.
> Each stays until the change that moves it to its consumer, which takes it off
> this list. Nothing is added to the list.

**Severity and check, before and after.** MUST, unchanged. Check:
`automated + agent-judgment` before, `agent-judgment` after. The automated
half was never built: `tools/tier1/rules.yaml` maps UX-08 to `follow-up: "a
test, not a static check: validation at startup"`. Jeff's words are that
the rule "isn't enforced programmatically", so the wording says so and the
check type follows (LINT-01: the `Check:` line is the rule's linter spec;
LINT-08: the agent prompt is its third paragraph).
- *Considered: keep `automated + agent-judgment`*, with the automated part
  defined as each component's own tests of its validation. A rule whose
  automated half is "the tests exist" has no check tier 1 can run, which is
  the state the follow-up entry records today. If "isn't enforced
  programmatically" was meant only of components written by others, this is
  the alternative, and the review can choose it when it approves the
  change.

**What the enforcement change does.** `tools/tier1/rules.yaml` loses the
line `UX-08: {follow-up: ...}`. Its header says an entry is for an
`automated`-tagged rule; with UX-08 no longer tagged so, the entry would
describe a check nobody intends to build. No check stops running: none ran.
The `rule-index` self-check accepts the result (a rule that is not
`automated` needs no entry). Tier 2 (the `wf-review` agent) judges the
rule, as it judges every `agent-judgment` rule.

**Does "fail fast" survive?** Yes, for what the run uses: the consumer
validates when it initializes and before any real work, which is the
current rule's "before any real work begins, rather than failing deep into
a build". What goes is "as early as possible, at load": the earliest
possible point is the file's load, and a check there fails runs that do not
use the setting and checks values a flag replaced. GLOBAL-10 cites UX-08
for "a provided value that cannot be used is an error that names it and its
fix"; that still holds, for a value a run uses, and GLOBAL-10 is not
edited.

**Against `cmd/warden` on `main`.** Every validation of configuration
there, and whether the amended wording describes it:

| Check | Where | When | Under the amended wording |
|---|---|---|---|
| A file that is not TOML | `loadTOML` | every command that loads configuration (`build`, `shell`, `doctor`, `setup`, `image prepare`, `clean`) | conforms: `warden` reads both files on each of those runs (the wording's second paragraph) |
| The TOML type of every key (`cli = 5`, `[vm] cpus = "four"`) | the same decode | the same, whatever the run uses | **does not**: a wrong type in a section the run does not use fails it. Listed. K1 makes it the driver's for the seven driver-owned keys (its Behaviour 13) |
| `[vm] build_link` | `loadTOML`, for each file | the same, every driver; in each file, even when the other file or `--build-link` replaces it | **does not**, twice: not after the overrides, and not only for a VM-driver run. Listed (a5) |
| `[vm] dns` | `loadTOML`, for each file | the same | **does not**, likewise. Listed (a5) |
| `[vm] image_refresh_after`, `image_min_grace` | `newRequest` | `build` and `shell`, every driver, after the files are merged | **does not** under the drivers that do not read them (only qemu and hyperv do). Listed. K1 makes them the two drivers' |
| `[runtime] driver`, `WARDEN_DRIVER`, `--driver` | `resolveConfig` (`validateDriver`) | every command that loads configuration, after the overrides | conforms: each of those commands dispatches on the driver |
| `[build] capture`, `--capture` | `newRequest` | `build`, `shell`, after the override | conforms |
| `--build-link` | `resolveBuildLink` | the VM-driver commands that use the link | conforms |
| `[vm] windows_image_update` | `prepareHyperVWindows` | `image prepare --driver hyperv` | conforms |
| `[runtime] cli`, `WARDEN_CTR_CLI`, `--runtime` | nowhere: run as given | | **this item**: `resolveRuntime`, on the commands that use the runtime, after the overrides |

So `main` conforms on the day this merges because the wording lists the
four checks that do not, and this item makes the rule true for the setting
it changes. That is the decision put as a5. Moving the four here was
weighed and not chosen: the type check cannot move without a reader for
each table, which is K1's work; the two `[vm]` checks were confirmed where
they are by two archived designs and are rewritten by K1; and a security
fix that also changed how every VM driver's settings are checked would not
be one small PR.

**Why the feature needs it.** Decision 2 is an instance of the rule, and
the current wording contradicts it: "as early as possible, at load" is the
alternative Decision 2 rejects. Jeff asked for the general rule to be
written in this item (a2).

**How K1 fits.** K1 has each driver read and validate its own settings in a
probe, only when selected, and proposes an ORCH-03 paragraph that says so
and cites UX-08. That is this rule applied to drivers; K1 is the item that
takes two entries off the rule's list (**What K1 has to keep**, point 7).

**The profile (PROFILE-01).** No entry becomes stale: the Rules entry names
the directory, the Checks table names no UX-08 check, and its Safety row
already covers "every `agent-judgment` rule".

The design review approves this change by name (FLOW-19).

## Security note

**Boundary: the host OS and the build environment** (GLOBAL-06, `host-env`).
The build context is the build environment's side of it: it is what the
isolated build runs. `warden` reads one file of it, `warden.toml`, on the
host, before any isolation exists.

**The threat.** Code execution on the host, as the user who runs `warden`,
by a repository that the user only meant to build: `cd repo && warden build`
(or `shell`) with the default driver, or `warden clean` with the default
driver, `qemu` or `vz`. The file can choose its own driver, so "the default"
is no protection for `clean`. SEC-01's cell is build environment to host,
class 1 (escape), though the attacker never has to get out of anything: the
program runs before the environment is made, and the ledger does not show
it.

**How the posture changes.** It tightens, and nothing loosens. Before: any
string from the project's file, the user's file, the variable or the flag
was run. After: the only programs `warden` runs as the container runtime
are `finch`, `docker` and `podman`, by those names, found on the user's
`PATH`. That closes the original hole (`cli = "./tools/docker"`: not one of
the three, refused before anything is looked up) and the variant
(`cli = "sh"`: likewise), for every source and every spelling of the key,
because the check is on the value in effect and the value run is the list's
own constant (Decision 3). No value is executed before it is checked:
`resolveRuntime` is the only reader of the setting, and detection runs only
the three names. The first draft closed the hole for the project's file
and left the other three sources able to run any program; they no longer
can, so a process that can set `WARDEN_CTR_CLI` for `warden` (the native
launcher's allowlist lets a session do so) chooses among three runtimes and
no more.

**What remains**, each unchanged by this item:

1. A repository can still choose among `finch`, `docker` and `podman`, and
   its choice outranks the user's config file (project over user, ORCH-03).
   A user with rootless podman configured and Docker also installed can be
   moved to Docker by a repository. `WARDEN_CTR_CLI` or `--runtime` pins it.
   The same holds for the driver (`[runtime] driver`). Both are
   `project-config-trust`'s.
2. The name is found on the user's `PATH`. A `PATH` entry a repository can
   write to is the user's exposure, as for any command. Go refuses a match
   in the working directory or through a relative `PATH` entry (`ErrDot`),
   so a `docker` or `docker.exe` planted in the repository is not run; it
   can make the lookup fail (F7, `configuration-bugfixes`).
   `GODEBUG=execerrdot=0` in the user's environment turns that refusal off.
3. Values a repository controls still reach the runtime's command line: the
   relay image and the `FROM` image of its Dockerfile, in `pull`, `image
   inspect` and `container create`. With the program fixed to a container
   runtime these name images, which is what a build does; an option-shaped
   value is F1's (`project-config-trust`).
4. F1 to F4 and F9: the project's file still chooses the relay image, the
   relay's trust roots, two host directories and the Hyper-V base image,
   and a `warden` installed outside its source tree can take the repository
   for that tree (`project-config-trust`).

**What the TEST-08 suite should exercise.** Arm A gains this as a permanent
entry (SEC-03), with the attacker supplying the build context: (i) a
`warden.toml` whose `cli` is a file the context ships; (ii) `cli = "sh"`
with files named `pull`, `container`, `image` and `info` in the context;
(iii) (i) again with the key respelled (`[Runtime]`, `CLI`). Each on
`build`, `shell`, `clean` and `doctor`, and on `clean` with
`driver = "qemu"` in the same file. The proof is SEC-04's: the file would
write a marker; the marker is absent and `warden` exits 1. The suite's
matrix places the attacker inside the build environment; this finding came
from outside it, from a file the host reads before the environment exists.
The files of a build context that `warden` reads on the host
(`warden.toml`, and the Dockerfile it translates) are surface the suite
should count.

The merge acknowledges this note (FLOW-08).

## Validation plan

**Tier 0** (`make test`; the `ci` workflow on Ubuntu, and any platform
locally), in `cmd/warden`, table-driven, with `HOME` (and `USERPROFILE`) set
to a temporary directory so that no test reads the real user's file:

- By value and source. `finch`, `docker` and `podman` are accepted from
  each of the four sources. `./docker`, `tools/docker`, `/usr/bin/docker`,
  `..\x.exe`, `C:\x\docker.exe`, `C:docker`, `docker.exe`, `Docker`,
  ` docker`, `docker `, `nerdctl`, `sh`, `sudo docker` and a single space
  are refused from each of the four. Each refusal is a `FixableError`
  whose message names the source (the flag, the variable, `warden.toml`,
  the user's file by its path) and the quoted value. The table is the same
  on every OS.
- The value run is the list's: for each accepted value, what
  `resolveRuntime` returns is the list's member.
- The overrides come first. An unsupported value in the project's file
  with `--runtime docker`, and with `WARDEN_CTR_CLI=podman`: no error, and
  that runtime. An unsupported value in the user's file with a supported
  one in the project's: no error. An unsupported `--runtime` over a
  supported file value: the error, naming the flag.
- An empty value: an empty flag and an empty variable leave the lower
  level's value in effect; `cli = ""` in the project's file over `podman`
  in the user's gives detection. `TestLoadConfig_ProjectFile` and
  `TestLoadConfig_EnvOverrides` stand.
- The key in other spellings, each with `./rt` and with `sh`: `[runtime]`
  with `CLI` and with `Cli`, `[Runtime]` with `cli`, `[RUNTIME]` with
  `CLI`, a dotted `runtime.CLI` and an inline table. Each is the error,
  naming `warden.toml`; and `[Runtime]` with `CLI = "podman"` selects
  podman. One table holding both `cli = "docker"` and `CLI = "./rt"`,
  loaded 200 times: every result is `docker` or the error.
- The source when both files hold the same string: the error names the
  project's file when it sets the key and the user's when it does not.
- Not used, not validated: with `cli = "nonsense"` in the project's file,
  `newDriver` for `qemu`, `vz`, `hyperv` and `firecracker` returns no
  error, and `resolveConfig` returns none for any driver.
- Used, and nothing is run: a project directory that ships an executable
  writing a marker, as `cli = "./rt"`, as `CLI = "./rt"` and as
  `cli = "sh"` with a file named `pull`; through the calls `build` and
  `shell` make (`newDriver`), `clean` and `doctor`, the result is the error
  and the marker is absent. `clean` again with `driver = "qemu"` in the
  project's file and with `--driver vz`. The same with the executable's
  absolute path in `--runtime`, in the variable and in the user's file.
  These use a shell script and skip on Windows, as `fakeRuntime` does
  today.
- No fallback: with `cli = "podman"` in the user's file and an unsupported
  value in the project's, the result is the error, and detection is not run
  (a fake `finch` on `PATH` whose `info` writes a marker; the marker is
  absent).
- The transitional forms, one table, the same on every OS: `Docker` and
  `docker.exe` give a fix naming `docker`; `/opt/homebrew/bin/docker` and
  `C:\Program Files\Docker\docker.exe` give a fix naming `docker` and the
  directory; `./tools/docker` gives a fix naming `docker` and no directory;
  `nerdctl` and `/usr/local/bin/nerdctl` give the `nerdctl` message;
  `/usr/local/bin/docker-ci` and `./rt` give the wrapper message; `sh`,
  `dokcer` and ` docker` are not recognized and give the permanent error.
  And for every row: the permanent check refuses the value, so the
  detection cannot accept anything.
- The exit code: the error is a `FixableError`, which `main` exits 1 on
  (UX-09).
- Detection still probes `finch`, `docker`, `podman` in that order, from
  the shared list (`TestDetectRuntimeExplainsTheFix` stands).

**Tier 1** (`make integration-test`; the `integration` workflow on hosted
Ubuntu with Docker, which runs because the change reaches `cmd/` and
`test-integration.sh`): in a temporary project directory with a Dockerfile,
a `warden.toml` and an executable that writes a marker,

- `cli = "./rt"`: `warden build`, `warden clean` and `warden doctor` each
  exit 1 with `To fix:`, the marker is absent, and no output directory is
  made;
- `[Runtime]` with `CLI = "./rt"`: `warden clean` likewise;
- `cli = "./rt"` with `driver = "qemu"` in the same file, and with
  `--driver vz`: `warden clean` likewise (it needs neither qemu nor macOS:
  the error comes first);
- the executable's absolute path in `--runtime`, and in `WARDEN_CTR_CLI`,
  with no `cli` in the file: `warden doctor` exits 1, the marker is absent;
- the `cli = "./rt"` file with `--runtime docker`, and with
  `WARDEN_CTR_CLI=docker`: `warden doctor` prints
  `container runtime: docker`;
- the same file and `warden setup`: exit 0 (the run does not use the
  setting);
- `cli = "docker"`: one of the builds the script already runs is run from
  such a directory, so a named runtime from a project's file is still built
  with, at no extra build.

These run the built binary, which is why they guard the rule across K1.

**Tier 2.** No host blocks merge (FLOW-09): the change reaches no driver
tree, and the check is one comparison against a list, the same code on every
OS. Two gaps are tracked (`tier2_gaps`, and the native-gap backlog):

- `container-any` (the Windows desktop): `go test ./cmd/warden`, which CI
  does not run on Windows; `--runtime docker.exe` and `[runtime] cli` set
  to the full path of `docker.exe` are each refused with the fix that names
  `docker`; a project `warden.toml` whose `cli` is a `.bat` the directory
  ships is refused on `build`, `clean` and `doctor` and the file is not
  run; `cli = "docker"` in the project's file builds.
- `macos-finch`: a build with `cli = "finch"` in the project's file;
  `--runtime` with the full path of `finch` is refused on `build` and on
  `clean --driver vz`, with the fix that names `finch` and its directory.

The release matrix (TEST-02) is the backstop, and the release that carries
this trips GLOBAL-06, so TEST-08 runs.

## Implementation plan

One PR. Nothing already implemented is reworked beyond the functions named.

1. One `containerRuntimes` list in `cmd/warden/config.go`; `DetectRuntime`
   ranges over it, in today's order (GLOBAL-09, GLOBAL-10).
2. The source of the runtime setting, beside the value: set by `LoadConfig`
   for each file (by what the file's decode did to the field, not by
   `IsDefined`) and for `WARDEN_CTR_CLI`, and by `resolveConfig` for
   `--runtime` (UX-04, UX-07, ORCH-03).
3. `resolveRuntime` checks the value in effect against the list and
   returns the list's member, or the permanent error as a `driver.Fixable`
   that names the source, the quoted value and the three names (GLOBAL-06,
   GLOBAL-10, GLOBAL-11, UX-04, UX-08, UX-09).
4. The transitional detection: `cmd/warden/runtime_former.go`, its one
   call, its test file, each with the marker
   `TRANSITIONAL(runtime-former-values)` and the removal point a4 settles
   (UX-03, UX-04, GLOBAL-01).
5. The tier-0 tests of the Validation plan, in `cmd/warden`; the existing
   `LoadConfig` tests set `HOME` and `USERPROFILE` like the new ones
   (GLOBAL-04, GLOBAL-07, TEST-01, TEST-06).
6. The tier-1 cases in `test-integration.sh` (GLOBAL-04, TEST-02).
7. The rule change, once the review has approved it by name: UX-08's
   wording in `docs/development/ux-cli-config.md`, and the removal of its
   entry from `tools/tier1/rules.yaml` (FLOW-14, LINT-01).
8. The reference docs, with the marked subsection (UX-01, UX-05), and the
   overview pseudocode (FLOW-15).
9. `make lint` and `make test` green, then push (FLOW-17).
10. At Publish: the removal of the transitional detection filed as a queue
    item (a4), and the two tracked gaps confirmed in the backlog (FLOW-09).

## Design review

### 2026-11-15, first sitting: three questions answered in Jeff's words; folded in

Read at `b8f692f`. The sitting is not finished and has no outcome yet.
Jeff answered a1, a2 and a3, each in his own words and each departing from
the draft; the words are in **Questions**. He also had the draft's nine
findings filed as two queue items, `project-config-trust` and
`configuration-bugfixes`.

Folded in, in this redraft:

- a1: the runtime is `finch`, `docker` or `podman` from every source, not
  only from the project's file. The project-level special case,
  `RefusedCLI` and the level-aware decode are gone (Decisions 1 and 3).
- a2: the consumer validates, when it initializes, after the overrides,
  and only when the run uses the setting (Decision 2); and the general rule
  is proposed as an amendment of UX-08 (`rule_changes`, **Rule changes**).
- a3: an error at once, with a permanent part and a transitional part that
  says what to change and is removed later (Decision 4).
- `main` at `ad4ea50` (the Loop Contract, #119) merged into the branch.
  The design meets the merged rules: FLOW-14 as amended (UX-08 is a
  project rule, so this item may amend it), the template's sentence on
  other repositories (LOOP-04: none), no **Specification changes** section
  (`seams` is empty), and PROFILE-01 (no entry becomes stale).
- The survey and the nine findings are replaced by pointers to the two
  items; **What K1 has to keep** is rewritten for the new rule.

Still to come in this sitting: the UX-08 rule change, to be approved by
name, approved with a change, or rejected; the two new assumed questions,
a4 (when the transitional detection is removed) and a5 (how UX-08 and
`main` agree on the day this merges); and the outcome.

## Consolidation

Filled in at Publish (FLOW-10, FLOW-15).

## Rollback notes

None.

## Archive summary

Filled in at Publish (FLOW-12).
