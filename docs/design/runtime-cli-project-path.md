---
feature: runtime-cli-project-path
status: archived
drivers: [container, qemu, vz]  # the setting is the container driver's; warden clean also
                         # runs the container runtime under qemu and vz, so it validates
                         # the setting there. The change is in cmd/warden (resolveRuntime
                         # and what feeds it); no driver/ tree is edited
seams: []
surfaces: [cli, config]  # config: [runtime] cli, in the user's file and the project's, and
                         # WARDEN_CTR_CLI take only finch, docker or podman. cli: --runtime
                         # takes only those three
security_boundary: host-env  # a build context can no longer make warden run a program on the
                         # host other than finch, docker or podman
alters_relationships: false  # inside the orchestrator's existing role: resolving the
                         # container runtime it hands to the container driver. No seam, no
                         # bootstrap change, no change of owner
design_review: required  # FLOW-19: a security boundary is touched, a rule change (UX-08), and
                         # three open assumed questions
native_validation:
  - {host: container-any, blocking: false, reason: "the change reaches no driver tree and its check is one string comparison, the same code on every OS; what only a Windows host with a real runtime adds is that --runtime docker.exe and a path to docker.exe get the message that names docker, that a .bat the project ships is not run, and a build with cli = \"docker\"; make test does not run on Windows in CI"}
  - {host: macos-finch, blocking: false, reason: "the change reaches no driver tree; what only this host adds is a build with cli = \"finch\" in the project's file, the default runtime CI does not run, and the message for finch given as a path; warden clean --driver vz takes the same path through cmd/warden as on Linux, where CI runs it"}
tier2_gaps:
  - {host: container-any, gap: "on a Windows host with a real runtime: go test ./cmd/warden; --runtime docker.exe and [runtime] cli set to the full path of docker.exe are each refused with the fix that names docker; a project warden.toml whose cli is a .bat the directory ships is refused on build, clean and doctor and the file is not run; cli = \"docker\" in the project's file builds (backlog runtime-cli-project-path-windows)"}
  - {host: macos-finch, gap: "a build with cli = \"finch\" in the project's warden.toml; --runtime with the full path of finch is refused on build and on clean --driver vz with the fix that names finch (backlog runtime-cli-project-path-finch)"}
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
- **`project-config-trust`** (in Design; not on `main`; its draft read at
  `2c38687` on its branch). Jeff settled its scope on 2026-11-15, in
  `driver-contract`'s review: "there should only be one authoritative
  warden.toml for any build". `warden` will read one file, `./warden.toml`
  by default or the one named with a new `--config` flag or `WARDEN_CONFIG`
  variable, with the environment and the command line; the user-level
  `~/.config/warden/config.toml` goes away. That item carries the change
  and amends ORCH-03 and UX-07, and this item is written to fit it
  (**One config file**).
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
  (Decision 5), UX-07 and ORCH-03 (the precedence, which this item does not
  change and `project-config-trust` amends), UX-01 and FLOW-15 (the docs),
  SEC-01, SEC-03 and SEC-04 (the corpus entry in the
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
   refused, whichever of the four set it (a1). There are four on `main`
   today; the user's file is one of them only until `project-config-trust`
   removes it, and the rule does not depend on how many files there are:
   the value in effect is checked against one list. The three are the
   runtimes the driver supports today: the names `DetectRuntime` probes,
   in that order, and the only values the flag's help, the configuration
   reference, the README and `warden doctor`'s help list. The edges:
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
     project's file it hides the user's (while there are two files), and
     the runtime is detected.
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
   variable, else the project's file, else the user's while that file is
   still read. So:
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
   | every other command and driver | nothing: the setting is not read. `warden build --runtime nonsense --driver qemu` is accepted and the flag ignored, as `--runtime` is there today |

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
6. **UX-08 is amended in this item.** Its wording names the four checks on
   `main` that it does not yet describe, and it is about settings held in a
   config file or the environment: a flag a run cannot use may still be
   refused at startup, as `main` does (question a6). This item brings the
   runtime setting under the rule and moves none of the older checks
   (question a5). The wording names no level of config file, so the change
   to one file needs no second amendment of it.
7. **The other findings of the first draft are two queue items now**, not
   this one's: `project-config-trust` and `configuration-bugfixes` (**The
   same defect elsewhere**).

What falls away from the first draft: the project-level special case and
the table of what each source may hold; `Runtime.RefusedCLI`; and the
case-insensitive walk of the decoder's metadata. None is needed when every
value in effect is checked against one list. Which file set the value is
still noted while there are two files, but only to word the error: nothing
that decides what is run depends on it.

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
- **a4. When is the transitional detection removed?** *Answered
  2026-11-15: (d), kept until 1.0. The design proceeded with (a) and is
  changed to (d).* After its removal the forms it recognizes get the permanent
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
  - (d) Kept until 1.0. **Chosen.**
- **a5. How do UX-08 as amended and `main` agree on the day this merges?**
  *Answered 2026-11-15: (a).* Four checks of file settings on `main` are
  made more widely than the amended rule says (the table in **Rule
  changes**), and the umbrella's Decision 18 has a rule change in the item
  that makes it true. Flags are a6.
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
- **a6. Does "only what a run uses is validated" reach flags?** *Answered
  2026-11-15: (a).* Jeff's words are about one `warden.toml` that holds
  the settings of several drivers. A flag is given to one run, and `main`
  refuses a flag the run cannot use: `--script`, `--image`, `--cpus`,
  `--memory`, `--keep-vms` and `--build-link` without a VM driver;
  `--network` without `hyperv`; `--guest-os` with a value that is not
  `linux` or `windows`, under every driver; `setup --slots` without
  `hyperv` or `firecracker`; `image prepare --update` without `vz`.
  `--runtime` is the exception: under a driver that does not use it, it is
  accepted and ignored.
  - (a) No. The rule's second paragraph covers settings in a config file
    or the environment, which serve several runs. For a flag it says only
    that a command may refuse, at startup, one its run cannot use. Every
    flag check on `main` conforms, `--runtime` being ignored conforms too,
    and nothing on `main` changes. **Recommended.**
  - (b) Yes: a flag the run does not use is not checked either. The flag
    checks above are then five more entries in the rule's list of older
    checks, each until some change removes it, and in the end `warden
    build --cpus 4` with the container driver is accepted and the flag
    ignored.
  - (c) The opposite, as a requirement: a flag the run cannot use is
    always refused. `--runtime` under the VM drivers then does not conform
    on `main`, and this item or a later one has to refuse it, which breaks
    a command line that passes `--runtime` with every driver.

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
(GLOBAL-10). The fourth row is there while `main` reads the user-level
file; `project-config-trust` removes the file and the row, and the other
rows stand as written.

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
    keep cli and its source; cli := unsetCLI  # a string no TOML file can hold
    load ./warden.toml                        # replaces; source: warden.toml
    cli == unsetCLI -> put the kept two back  # the file did not set the key
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
  exactly while the decoder also fills the field from `CLI` or `[Runtime]`.
  Before the project's file is decoded the field is given a marker,
  `unsetCLI`, and a file that leaves the marker did not set the key, so the
  user's value and source are put back. The marker is a constant that
  holds a byte which is not valid UTF-8 (`"\xff"`). No TOML file can
  produce it: the decoder refuses that byte in a file, and an escape in a
  TOML string always gives valid UTF-8 (run against the decoder `main`
  pins, BurntSushi/toml v1.6.0: a raw `0xff` in a basic, a literal and a
  multi-line string each fail the decode; `\xff` and `\u00ff` give the
  two bytes of U+00FF; `\uD800` fails). So a file cannot pass for one that
  left the key unset, and an unsupported value in the project's file is
  never replaced by the user's (Decision 2, GLOBAL-10). A marker a file
  could write, a NUL-prefixed string for one, would not do: `"\u0000..."`
  decodes without error. The marker never leaves `LoadConfig`. The source
  is used for the error's wording and for nothing else: a wrong source
  cannot make a value run.
- That step exists only because `main` reads two files. With one file
  (`project-config-trust`) the source of a file's value is that file, and
  the marker and the put-back go.
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
`containerRuntimes`. The value is printed once, in the first line, with
`%q`: it may come from a file someone else wrote, and `%q` keeps control
characters out of the terminal and shows a stray space. No other part of
the message is taken from the value: the fix is made of the list's names,
the source's own name and fixed text. That holds for the transitional
messages too. The error goes to stderr and `warden` exits 1 (UX-09: not a
build's own exit).

**Transitional.** Four forms ran before this fix. For each, the same
source clause and a message of its own:

| The value | Example | What the message says | The fix it gives |
|---|---|---|---|
| A supported runtime in another letter case or with `.exe` | `Docker`, `docker.exe` | the runtime is named in lower case, without an extension | the name: `docker` |
| A path whose last element is a supported runtime (after the row above) | `/opt/homebrew/bin/docker`, `C:\Program Files\Docker\docker.exe`, `./tools/docker` | the runtime is named, not given as a path | the name; and that `warden` looks it up on `PATH`, so a runtime installed outside `PATH` needs its directory added there. No directory is printed |
| `nerdctl`, alone or as a path's last element | `nerdctl`, `/usr/local/bin/nerdctl` | the container driver supports `finch`, `docker` and `podman` and no longer runs another runtime | one of the three; and that there is no supported way to use `nerdctl` with the container driver |
| A path to anything else | `/usr/local/bin/docker-ci`, `./rt` | the container driver runs `finch`, `docker` or `podman` by name and no longer runs another program | one of the three; if the program only set variables for one of them (`DOCKER_HOST`, `DOCKER_CONTEXT`, `CONTAINER_HOST`), set those in the environment; and that there is no supported way to run another program as the runtime |

For example:

```
ERROR: WARDEN_CTR_CLI is "/opt/homebrew/bin/docker"; the container runtime is named, not given as a path
To fix: set WARDEN_CTR_CLI to docker; warden looks docker up on PATH, so if it is installed outside PATH, add its directory to PATH
```

The name in a fix is the list's member that the value's last element
matched, not text cut from the value. The directory in the value is not
repeated. When the value comes from a project's `warden.toml`, that
directory is the repository's choice, and `warden` would be telling the
user, in its own voice, to put a directory someone else named on `PATH`:
the exposure the Security note leaves with the user. It would also carry
whatever characters the directory holds into the `To fix:` line. One
wording for every source and for absolute and relative paths alike keeps
the detection to "is the last element a supported runtime".

A bare name that is neither a supported runtime nor `nerdctl` (a wrapper
on `PATH`, a typing error, `sh`) cannot be told apart from one another, and
gets the permanent error, which already says what the supported values are.

About a wrapper, plainly. A script that only set variables and then ran a
supported runtime is replaced by setting those variables. A wrapper that
did anything else (ran the runtime through `sudo`, added arguments such as
`podman --remote` or a `nerdctl` namespace, chose between runtimes) has no
supported replacement: the container driver runs the runtime by its name
with `warden`'s arguments and nothing else.

About `nerdctl` and other runtimes, plainly: there is no supported route
today. Before this fix `WARDEN_CTR_CLI=nerdctl` ran `nerdctl`, untested and
undocumented. After it, the container driver runs three runtimes, and a
runtime of one's own is a driver of one's own, which `warden` can run only
once the driver contract (K1) has landed. Until then the answer is one of
the three; Finch is the supported runtime built on `nerdctl`. A file named
`docker` on the user's own `PATH` that runs something else is the user's
arrangement: `warden` runs it as `docker`, and it is neither tested nor
supported. That is also all there is for a wrapper that did more than set
variables.

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
- When: at 1.0 (a4, answered 2026-11-15). The detection is in every
  release before 1.0 and is removed in the change that prepares 1.0.
  Publish files the removal as a queue item, on hold until then, whose
  `source` names the marker and that condition.

### Compatibility (UX-03)

A user-facing change of behavior, from every source. What stops working:

| Today | After |
|---|---|
| `--runtime /path/to/docker`, `WARDEN_CTR_CLI=/usr/local/bin/podman`, or `cli = "/opt/homebrew/bin/finch"` in either file | refused. Name the runtime, and put its directory on `PATH` |
| `--runtime docker.exe`, `Docker` | refused. Write `docker` |
| `WARDEN_CTR_CLI=nerdctl`, or another runtime's program | refused. No supported route today (above) |
| A wrapper script, by path or by name, that only sets variables and runs a supported runtime (`docker-ci` setting `DOCKER_HOST`) | refused. Name the runtime and set the wrapper's variables in the environment |
| A wrapper that does anything else: `sudo docker`, `podman --remote`, `nerdctl --namespace`, a script that picks a runtime | refused. No supported route (above). By path it gets the transitional message; a bare name on `PATH` gets the permanent error, which does not mention wrappers |
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
- A user with a wrapper around a supported runtime. One whose wrapper
  only sets variables sets them in the environment. One whose wrapper adds
  `sudo` or arguments has nothing to move to: a rootful runtime reached
  through `sudo`, or a remote podman reached through arguments and not
  through `CONTAINER_HOST`, stops working with `warden` until it is
  reachable by the runtime's name alone.
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
  supported name from any source; `--runtime` accepted and ignored by
  `build`, `shell` and `doctor` under a VM driver, by `clean` under
  `hyperv` and `firecracker`, and by `setup` and `image prepare`.

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
  directory for its own source tree), and the whole-file question, which
  Jeff settled on 2026-11-15: there is one authoritative `warden.toml` for
  a build, `./warden.toml` by default or the file named with `--config` or
  `WARDEN_CONFIG`, which is then the only one read; the user-level file
  goes away, and one that still exists is an error that says what to do.
  So a project's file no longer outranks a file of the user's: the user
  names the file.
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

### One config file

`project-config-trust` removes the user-level file, and this item is
written to fit that (its queue item, Jeff, 2026-11-15). The order of
merging is this item, then `project-config-trust`, then K1; that item's
draft says the same and asks how the queue holds it behind this merge (its
a7).

- The rule does not count files. The value in effect is checked against
  one list, and that is true with two files, with one, and with a file
  named by `--config`.
- What this item writes for two files, because `main` has two: the fourth
  row of the rule's table, the marker step in `LoadConfig`, "else the
  user's" in `resolveRuntime`'s comment and the reference, the error that
  names the user's file by its path, and the tests of those. That item
  removes them with the file. `runtime_former.go` and its marker are not
  among them.
- UX-08 as proposed here names no level of config file, so that item's
  declared edit to it (one clause, "the config files in precedence order")
  has nothing left to edit.
- If that item were to merge first, this one meets one file when it merges
  `main`: the fourth row and the marker step are not written, and nothing
  else in this design changes.

### What K1 has to keep

K1 (`driver-contract`) moves this setting out of `cmd/warden`'s `Config`:
the container driver reads it through the kit, from `--runtime`,
`WARDEN_CTR_CLI` and the one config file `warden` names to it, under the
key `[driver.container] cli` with `[runtime] cli` a deprecated second name.
This item merges first; K1 meets it in its consolidation (FLOW-10). The
requirement below is for K1's Implementation plan and is handed to that
item; this design does not edit K1's. K1's design at `4c3c8fc` already
carries points 1 to 6, read from this redraft before it was committed;
point 4 is stricter here than there.

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
   driver's state file, written during `run`. The value written there has
   passed the check, and the value read back is compared with the list
   again before it is run: one string comparison, so that "the value run
   is the list's own" does not rest on the contents of a file. Where the
   driver resolves its settings again, the check runs again.
5. **The transitional detection moves with the check**, stays one file
   behind one call, keeps its marker and its removal point (a4). Its
   `nerdctl` message can then name the route: a driver of one's own.
6. **The tests.** The integration cases run the built `warden` against a
   project directory and must pass on K1's branch unchanged, but for the
   one case in point 2. The unit tables move with the code. K1 adds, for
   `[driver.container] cli`, the rows this item has for `[runtime] cli`,
   the marker-file case among them: a K1 that checked the old name only
   would pass every test this item writes.
7. **UX-08.** After this merges, K1 quotes the amended rule, and strikes
   from its list the checks K1 moves to a driver: the type of the seven
   driver-owned keys and the two image ages. That is an edit of UX-08 that
   K1 has to declare (its a8 says so). Whether K1 also takes `[vm]
   build_link`, `[vm] dns` and the type of `warden`'s own keys is question
   a5; K1's design keeps them where they are (its Decision 8). K1's
   proposed ORCH-03 paragraph ("A driver reads its own ... with the same
   precedence and before it does any work (UX-08)") and its Errors table
   ("a driver's key, while another driver is selected: it is not read")
   are this rule applied to drivers and need no change for it.
8. **K1's own text.** Its redraft of 2026-11-15 records that K1 is the
   second to merge and that the rule has no levels. What is left for it
   from this design: point 4 as written here, and the commit of this
   design it names.

If K1 were to merge first, the code this item changes would be gone and it
would return to Design (FLOW-06).

### What this item does not do

- It does not resolve a name to an absolute path once, or check who owns
  the file found. The runtime is looked up by `os/exec` as today.
- It does not move the checks of `[vm] build_link`, `[vm] dns`, the image
  ages or TOML types (a5).
- It does not fix F1 to F9, and it does not remove the user-level file
  (**One config file**).
- It does not change which flags a command refuses (a6).
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
> alike, when it initializes: after the overrides are applied (flags and
> environment variables over file configuration, in the order UX-07 gives) and
> before any real work begins, rather than failing deep into a build. What it
> checks is the value in effect, not a value that a higher level replaced.
>
> Only the configuration a run uses is validated. A `warden.toml` and the
> environment may hold the settings of several drivers or components, for runs
> that choose among them on the command line; a setting there that the run does
> not use is not checked, and a bad value in it is not an error. A flag is given
> to one run: a command may refuse, at startup, a flag that its run cannot use.
> A config file that cannot be parsed is an error for every run that reads it.
>
> This is required of every component in the BuildWarden organization and
> highly recommended for components others write. It is not enforced
> mechanically. The check judges, for each setting a change adds or touches, that
> the component that uses it validates it at that point, and that nothing
> validates a file or environment setting the run does not use.
>
> Four checks predate this wording and are made whatever the run uses. As a
> config file loads: the TOML type of every key, `[vm] build_link`, and
> `[vm] dns`. On every `build` and `shell`: `[vm] image_refresh_after` and
> `image_min_grace`. Each stays until the change that moves it to its consumer,
> which takes it off this list. Nothing is added to the list.

**Severity and check, before and after.** MUST, unchanged. Check:
`automated + agent-judgment` before, `agent-judgment` after. The automated
half was never built: `tools/tier1/rules.yaml` maps UX-08 to `follow-up: "a
test, not a static check: validation at startup"`. Jeff's words are that
the rule "isn't enforced programmatically", so the wording says so and the
check type follows (LINT-01: the `Check:` line is the rule's linter spec;
LINT-08: the agent prompt is its third paragraph).
- *The wording names no level of config file.* It says "file
  configuration, in the order UX-07 gives", so it reads the same on `main`
  today (a project's file over the user's) and after
  `project-config-trust` (one file), and that item need not amend it
  again.
- *Flags.* The second paragraph is about what Jeff described: settings
  kept in a file, and in the environment, that serve several runs. For a
  flag it only permits what `main` does. Whether it should reach flags is
  a6.
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
there, settings first and then flags, and whether the amended wording
describes it (read from every fixable error `cmd/warden` returns at
`ad4ea50`; errors about one command's arguments, such as a missing ISO or
an unknown subcommand, are left out):

| Check | Where | When | Under the amended wording |
|---|---|---|---|
| A file that is not TOML | `loadTOML` | every command that loads configuration (`build`, `shell`, `doctor`, `setup`, `image prepare`, `clean`) | conforms: `warden` reads both files on each of those runs (the wording's second paragraph) |
| The TOML type of every key (`cli = 5`, `[vm] cpus = "four"`) | the same decode | the same, whatever the run uses | **does not**: a wrong type in a section the run does not use fails it. Listed. K1 makes it the driver's for the seven driver-owned keys (its Behaviour 13) |
| `[vm] build_link` | `loadTOML`, for each file | the same, every driver; in each file, even when the other file or `--build-link` replaces it | **does not**, twice: not after the overrides, and not only for a VM-driver run. Listed (a5) |
| `[vm] dns` | `loadTOML`, for each file | the same | **does not**, likewise. Listed (a5) |
| `[vm] image_refresh_after`, `image_min_grace` | `newRequest` | `build` and `shell`, every driver, after the files are merged | **does not** under the drivers that do not read them (only qemu and hyperv do). Listed. K1 makes them the two drivers' |
| `[runtime] driver`, `WARDEN_DRIVER`, `--driver` | `resolveConfig` (`validateDriver`) | every command that loads configuration, after the overrides | conforms: each of those commands dispatches on the driver |
| `[build] capture`, `--capture` | `newRequest` | `build`, `shell`, after the override | conforms |
| The value of `--build-link` | `resolveBuildLink` | the VM-driver commands that use the link | conforms |
| `[vm] windows_image_update` | `prepareHyperVWindows` | `image prepare --driver hyperv` | conforms |
| `--timeout`; `--tmpdir` and `[build] tmpdir` | `newRequest`; `clean` under `firecracker` and `hyperv` for the directory | `build`, `shell`, and that `clean` | conforms: each of those runs uses them |
| `[runtime] cli`, `WARDEN_CTR_CLI`, `--runtime` | nowhere: run as given | | **this item**: `resolveRuntime`, on the commands that use the runtime, after the overrides |
| `--script`, `--image`, `--cpus`, `--memory`, `--keep-vms`, `--build-link` without a VM driver; `--network` without `hyperv` | `validateFlagsForDriver`, from `newRequest` | `build`, `shell` | conforms: a flag the run cannot use, refused at startup (the flag sentence; a6) |
| `--guest-os`: a value that is not `linux` or `windows`, under every driver; `windows` without `qemu` or `hyperv` | `validateGuestOS`, from `newRequest` | `build`, `shell` | conforms, likewise. The container driver never reads the flag, so `warden build --guest-os bogus` fails there for a flag the run does not use |
| `--build-link` without a VM driver | `doctor` | `doctor` | conforms, likewise |
| `--slots` without `hyperv` or `firecracker` | `setup` | `setup` | conforms, likewise |
| `--update` without `vz`; `--os` against the driver; the source given twice | `runImagePrepare` (`checkImageUpdate`, `imageSource`) | `image prepare` | conforms: the first likewise, the others are the run's own arguments |
| `--runtime` under a driver whose command does not resolve the runtime | nowhere: accepted and ignored | | conforms: the flag sentence permits a refusal and does not require one |

Not in the table, because nothing in `cmd/warden` validates them: the
relay image, the CA paths, the output directory, the Hyper-V base image
and `[output] color`. They are passed on as given, and are no nearer to
UX-08 and no further from it under the new wording than under the old.
`project-config-trust` (F1 to F4) and `configuration-bugfixes` hold them.

So `main` conforms on the day this merges for three reasons: the wording
lists the four checks of file settings that do not; its second paragraph
is about file and environment settings and permits the flag checks; and
this item makes the rule true for the setting it changes. This PR changes
`resolveRuntime` and what feeds it, and no other check. The first is the
decision put as a5 and the second as a6. Moving the four here was
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

**How `project-config-trust` fits.** It amends ORCH-03 and UX-07, the
order this rule points to, and removes the user-level file. Nothing in the
wording above has to change for it (**One config file**).

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

1. A repository can still choose among `finch`, `docker` and `podman`,
   and the driver (`[runtime] driver`), through the `warden.toml` in the
   working directory. On `main` today that file also outranks the user's
   own config file (project over user, ORCH-03): a user with rootless
   podman configured there and Docker also installed can be moved to
   Docker by a repository. `WARDEN_CTR_CLI` or `--runtime` pins it. Jeff
   settled the rest on 2026-11-15, for `project-config-trust`: one
   authoritative `warden.toml`, `./warden.toml` by default or the file
   named with `--config` or `WARDEN_CONFIG`, which is then the only one
   read, and no user-level file. After that item the way not to act on a
   repository's file is to name one's own; what, if anything, guards the
   default is that item's open question (its a4).
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

5. The error prints the value, which a repository's file can set. It is
   printed once, quoted with `%q`, and no other part of the message comes
   from it: no directory from the value is suggested for `PATH`, and
   control characters do not reach the terminal.

**What the TEST-08 suite should exercise.** Arm A gains this as a permanent
entry (SEC-03), with the attacker supplying the build context: (i) a
`warden.toml` whose `cli` is a file the context ships; (ii) `cli = "sh"`
with files named `pull`, `container`, `image` and `info` in the context;
(iii) (i) again with the key respelled (`[Runtime]`, `CLI`). Each on
`build`, `shell`, `clean` and `doctor`, and on `clean` with
`driver = "qemu"` in the same file. The proof is SEC-04's: the file would
write a marker; the marker is absent and `warden` exits 1. Before the fix
the marker is written on `build`, `shell` and `clean`, which run the
file; `doctor` only printed the value, so there the exit code is the
proof. The suite's
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
- The marker. A project file that leaves the key unset, and no project
  file at all, each restore the user's value and its source. No project
  file can yield the marker: a file whose `cli` holds the marker's bytes
  fails to load, and its escaped spellings (`\xff`, `\u00ff`) are
  unsupported values that give the error naming `warden.toml`, with a
  supported value in the user's file not used in their place. After
  `LoadConfig` the field is never the marker.
- Not used, not validated: with `cli = "nonsense"` in the project's file,
  `newDriver` for `qemu`, `vz`, `hyperv` and `firecracker` returns no
  error, and `resolveConfig` returns none for any driver. Through the
  commands that return before the runtime is resolved: `clean` under
  `hyperv` and under `firecracker`, and `doctor` under each of the four VM
  drivers, return no error about the runtime.
- Used, and nothing is run: a project directory that ships an executable
  writing a marker, as `cli = "./rt"`, as `CLI = "./rt"` and as
  `cli = "sh"` with a file named `pull`; through the calls `build` and
  `shell` make (`newDriver`), `clean` and `doctor`, the result is the error
  and the marker is absent. `clean` again with `driver = "qemu"` in the
  project's file and with `--driver vz`. The same with the executable's
  absolute path in `--runtime`, in the variable and in the user's file.
  These use a shell script and skip on Windows, as `fakeRuntime` does
  today. What fails before the fix differs by command, and each case
  asserts the one that does. `clean` runs the file on `main`, so there the
  absent marker discriminates. `newDriver` and `doctor` do not run a
  configured runtime on `main` (the driver runs it later; `doctor` prints
  it and `ok`), so there the `FixableError` and exit 1 discriminate, and
  the absent marker only guards against a fix that starts running it.
- No fallback: with `cli = "podman"` in the user's file and an unsupported
  value in the project's, the result is the error, and detection is not run
  (a fake `finch` on `PATH` whose `info` writes a marker; the marker is
  absent).
- The transitional forms, one table, the same on every OS: `Docker` and
  `docker.exe` give a fix naming `docker`; `/opt/homebrew/bin/docker`,
  `C:\Program Files\Docker\docker.exe` and `./tools/docker` give the
  same fix, naming `docker` and `PATH` and no directory;
  `nerdctl` and `/usr/local/bin/nerdctl` give the `nerdctl` message;
  `/usr/local/bin/docker-ci` and `./rt` give the wrapper message; `sh`,
  `dokcer` and ` docker` are not recognized and give the permanent error.
  And for every row: the permanent check refuses the value, so the
  detection cannot accept anything.
- Nothing of the value outside the quotes: for a path whose directory
  holds an escape character and a newline, from each source, the value
  appears in the message once, quoted, and neither the directory nor
  either character appears anywhere else in the message or the fix.
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
  made. On `main` the marker is written by `build` and `clean`; `doctor`
  exits 0 without running the file, so its exit code is what the case
  checks;
- `[Runtime]` with `CLI = "./rt"`: `warden clean` likewise;
- `cli = "./rt"` with `driver = "qemu"` in the same file, and with
  `--driver vz`: `warden clean` likewise (it needs neither qemu nor macOS:
  the error comes first);
- the executable's absolute path in `--runtime`, and in `WARDEN_CTR_CLI`,
  with no `cli` in the file: `warden doctor` exits 1, the marker is absent;
- the `cli = "./rt"` file with `--runtime docker`, and with
  `WARDEN_CTR_CLI=docker`: `warden doctor` prints
  `container runtime: docker`;
- the same file and `warden setup`, and `warden clean --driver hyperv`:
  exit 0 (the run does not use the setting);
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
  `clean --driver vz`, with the fix that names `finch`.

The release matrix (TEST-02) is the backstop, and the release that carries
this trips GLOBAL-06, so TEST-08 runs.

## Implementation plan

Plan r1 (phase 2, `f2-plan`, 2026-11-15, attended), written from the design
as its review approved it at `a2afd08` (review entry r1, recorded at
`119a0e1`), after reading the code it reaches on the branch at `119a0e1`:
`cmd/warden/config.go` (`LoadConfig`, `loadTOML`, `DetectRuntime`), `main.go`
(`resolveConfig`, `resolveRuntime`, `newDriver`, `prepare`, `main`),
`clean.go` (`runClean`), `host.go` (`doctor`, `setup`), the tests beside
them, `test-integration.sh`, the reference docs, and UX-08 with its entry in
tier 1's map. Design's ten-line outline of this section is at `119a0e1`;
this replaces it. One PR, one `f3-implement` step. Planning found the design
workable as written and sends nothing back.

Tasks 1 to 3, 6 and 16 rework code that exists (FLOW-06). Task 17 is the
approved rule change (FLOW-14). Tasks 18 to 20 are the reference docs and
`overview.md` (UX-01, FLOW-15). Tasks 6 to 14 are tier 0 and 15 and 16 are
tier 1 (TEST-02); tier 0 stays pure Go and table-driven, with no network and
no container runtime (TEST-01). All code is in `cmd/warden`: no file under
`driver/` is edited, `driver/container/` and the `nerdctl` arm of
`copyExecutor` included (ORCH-01). Under `docs/development/`, `tools/tier1/`,
`.claude/` and `tools/workflow/` only the two paths of task 17 are edited
(FLOW-14). No host blocks merge (FLOW-09): the two tracked gaps are open in
the native-gap backlog as `runtime-cli-project-path-windows` and
`runtime-cli-project-path-finch`, and no later-phase host step exists or is
added.

**Plan notes.** Where the design's text is silent on how a case is run, the
plan takes the reading below. None changes a decision, a behavior, an impact
field or the rule change; each is for the Review, and for Design only if the
reading is wrong.

*Note A (task 11: `clean` under `hyperv` and `firecracker`, tier 0).* Called
in process, those two commands do their real work before they return:
`hyperv.Clean` removes the VMs, the switch and the work directory of every
build whose lock is not held, `--keep-vms` leftovers included, and
`firecracker.Clean` kills VMMs and removes work directories the same way.
The cases also run with `HOME` moved, and on Linux and macOS that moves the
lock directory (`driver.LockDir` is under `os.UserCacheDir`), so a build
that is running would look abandoned. On the Windows desktop the tracked
gap runs `go test ./cmd/warden` beside real Hyper-V VMs. The design names
the case and not how it runs. Reading: the case removes nothing. Both rows
run with the user cache directory (`HOME`, `XDG_CACHE_HOME`, `LocalAppData`)
and `--tmpdir` at empty temporary directories, so that the driver's clean
finds nothing; the `hyperv` row is skipped on Windows, the one OS where
`hyperv.Clean` reaches Hyper-V and what it lists cannot be redirected. What
the row proves, that `runClean` returns before the runtime is resolved, is
the same Go code on every OS and is run on Linux and macOS, and by tier 1
(task 15). Tier 1 skips `clean --driver hyperv` on a Windows host for the
same reason; it runs on Linux and macOS (added at Implement, on the Review's
finding: task 15 and the Validation plan list the case without the
condition).

*Note B (task 11: `doctor` under the VM drivers, tier 0).* `doctor` ends
the process with `os.Exit(1)` when the checks of `hyperv` or `firecracker`
fail, which they do on every host but their own. Reading: the four rows run
`doctor` in a child process, the test binary re-executed (`os.Args[0]`),
which is pure Go and needs nothing installed (TEST-01), and assert that its
output holds no error about the runtime: neither the configured value nor
`[runtime] cli`. `doctor` runs no build under any driver (read at
`119a0e1`). It is not read-only under every driver, as this note first
said: under `firecracker` its reflink probe makes the driver's work
directory in the user cache and writes and removes probe files there
(corrected at Consolidation, on the Review's finding S1). The four rows
therefore give the child an empty user cache directory (`XDG_CACHE_HOME`,
`LocalAppData`), as Note A's rows have (added after Publish, with the fix
of S1).

*Note C (task 16: the build from a project's file, tier 1).* The design
writes `cli = "docker"`. The script runs on any host where a runtime answers
and relies on detection (the design's Compatibility says so), and a fixed
`docker` would fail it on a host with only Finch or Podman. Reading: the
file names the runtime `warden doctor` reports on that host, which is
`docker` on the CI runner.

*Note D (the Security note's entry for the TEST-08 suite).* No task adds it
to Arm A: the corpus is not built yet (`security-suite.md`, "Remaining build
work"), so there is no file to add it to. Its proof (SEC-04: the marker is
absent and `warden` exits 1) is what tasks 12 and 15 assert, in tiers 0 and
1, and the note's list stays in the archived design for the item that builds
the corpus (SEC-03, FLOW-12).

1. **One supported list.** `containerRuntimes` (`finch`, `docker`, `podman`,
   in detection's order) in `cmd/warden/config.go`, beside `DetectRuntime`,
   which ranges over it in place of its own literal. Detection's order, its
   3-second `info` probe and its two errors are unchanged (Decision 1;
   GLOBAL-09, GLOBAL-10).
2. **The source of the setting, beside the value.** A field next to
   `RuntimeConfig.CLI`, `toml:"-"`, as `BuildLinkSource` is. `LoadConfig`
   follows the design's pseudocode (**In the code**): the user's file, by
   its path; then the value and its source are kept and the field is set to
   `unsetCLI` (the constant `"\xff"`, with a comment that says why no TOML
   file can produce it: GLOBAL-01); the project's file, whose decode
   replaces the marker when the file sets the key in any spelling, an empty
   value included; a field still equal to the marker gets the kept value
   and source back; a non-empty `WARDEN_CTR_CLI` replaces both. The marker
   never leaves `LoadConfig`, and `IsDefined` is not used for this key.
   `resolveConfig` sets the source where a non-empty `--runtime` replaces
   the value. Precedence and the meaning of an empty value are unchanged
   (ORCH-03, UX-07, GLOBAL-10); the source feeds the error's wording and
   nothing else (UX-04).
3. **`resolveRuntime` validates the value in effect** (Decisions 2 and 3):
   empty, `DetectRuntime()`; equal to a member of `containerRuntimes`, that
   member, never the configured string; anything else, the error of task 4,
   with nothing looked up, run or created. The comparison is exact: nothing
   is trimmed, lowered or stripped (GLOBAL-11). Its three callers
   (`newDriver`, `runClean`, `doctor`) already return its error before they
   run or create anything, so `clean.go` and `host.go` need no edit; task 12
   proves it. No other command starts reading the setting (UX-08,
   GLOBAL-06, GLOBAL-10).
4. **The permanent error** (**The error**, Permanent): a `driver.Fixable`
   whose first clause names the source (`--runtime`, `WARDEN_CTR_CLI`,
   `[runtime] cli in warden.toml`, `[runtime] cli in <the user's file by its
   path>`), then the value once with `%q`, then the three names; the fix is
   the design's wording for that source. The names come from
   `containerRuntimes`, and no part of the message but the quoted value is
   taken from the value. It leaves `warden` through `main`'s `FixableError`
   path, exit 1 (UX-04, UX-09, GLOBAL-06).
5. **The transitional detection** (**The error**, Transitional; Decision 4;
   a4): `cmd/warden/runtime_former.go`, one function from the value to a
   message and a fix for the four forms of the design's table, in the
   table's order, or nothing. String operations only, no `path/filepath`,
   so `/`, `\` and a drive letter read the same on every OS (GLOBAL-07). The
   name in a fix is the list's member the last element matched; no
   directory and no other text from the value is printed (GLOBAL-06). One
   call, in the function that builds the error of task 4, reached only
   after the value failed the check of task 3. The marker
   `TRANSITIONAL(runtime-former-values)` is in the file's header comment,
   which says why the code exists and that it is removed in the change
   that prepares 1.0 (GLOBAL-01), and at the call (UX-03, UX-04).
6. **Tier 0, the existing `LoadConfig` tests reworked** (TEST-01, TEST-02).
   The five `TestLoadConfig_*` tests in `config_test.go` set `HOME` and
   `USERPROFILE` to a temporary directory and clear `WARDEN_CTR_CLI`, as
   the new tests do, through one helper that also writes a user file and a
   project file and resets the flag variables a case sets (`flagRuntime`,
   `flagDriver`, `flagTempDir`). No test in `cmd/warden` reads the real
   user's file. `TestLoadConfig_ProjectFile` and
   `TestLoadConfig_EnvOverrides` keep their assertions (GLOBAL-04,
   GLOBAL-07).
7. **Tier 0, by value and source** (TEST-01, TEST-02; Validation plan, its
   first two bullets and its last two). One table, the same on every OS
   (GLOBAL-07): the three names accepted from each of the four sources, and
   what `resolveRuntime` returns is the list's member; the design's
   fourteen other values refused from each of the four, each a
   `FixableError` naming the source and the quoted value. The error is not
   an `exitStatusError`, so `main` exits 1 on it (UX-09).
   `TestDetectRuntimeExplainsTheFix` stands, and detection probes the
   shared list in its order (GLOBAL-04, GLOBAL-06, UX-04).
8. **Tier 0, the overrides come first, and the empty value** (TEST-01,
   TEST-02; Validation plan, "The overrides come first", "An empty value",
   "The source when both files hold the same string"): an unsupported file
   value under `--runtime docker` and under `WARDEN_CTR_CLI=podman` is no
   error; an unsupported value in the user's file under a supported one in
   the project's is no error; an unsupported `--runtime` over a supported
   file value is the error naming the flag; an empty flag and an empty
   variable leave the lower level in effect; `cli = ""` in the project's
   file over `podman` in the user's gives detection; the error names the
   file that set the key (UX-08, ORCH-03, UX-07, GLOBAL-10).
9. **Tier 0, the marker** (TEST-01, TEST-02; Validation plan, "The
   marker"): a project file that leaves the key unset, and no project file,
   each restore the user's value and source; a file whose `cli` holds the
   marker's raw byte fails to load; `\xff` and `ÿ` are unsupported
   values that give the error naming `warden.toml`, with a supported value
   in the user's file not used in their place; after `LoadConfig` the field
   is never the marker (GLOBAL-10, GLOBAL-11).
10. **Tier 0, the key in other spellings** (TEST-01, TEST-02; Validation
    plan, "The key in other spellings"): the six spellings, each with
    `./rt` and with `sh`, give the error naming `warden.toml`; `[Runtime]`
    with `CLI = "podman"` selects podman; one table holding `cli =
    "docker"` and `CLI = "./rt"`, loaded 200 times, gives `docker` or the
    error every time (GLOBAL-06, GLOBAL-04).
11. **Tier 0, not used, not validated** (TEST-01, TEST-02; Validation plan,
    "Not used, not validated"; Decision 2), with `cli = "nonsense"` in the
    project's file: `newDriver` for `qemu`, `vz`, `hyperv` and
    `firecracker` returns no error; `resolveConfig` returns none for any
    driver; `clean` under `hyperv` and under `firecracker`, run as Note A
    says, and `doctor` under each of the four VM drivers, run as Note B
    says, give no error about the runtime (UX-08).
12. **Tier 0, used, and nothing is run** (TEST-01, TEST-02; Validation
    plan, "Used, and nothing is run" and "No fallback"; SEC-04). A project
    directory that ships an executable writing a marker, as `cli =
    "./rt"`, as `CLI = "./rt"` and as `cli = "sh"` with a file named
    `pull`: through `newDriver` (what `build` and `shell` call), `runClean`
    and `doctor` the result is the error and the marker is absent;
    `runClean` again with `driver = "qemu"` in the file and with `--driver
    vz`; the same with the executable's absolute path in `--runtime`, in
    the variable and in the user's file. No fallback: `podman` in the
    user's file and an unsupported value in the project's give the error,
    and a fake `finch` on `PATH` whose `info` writes a marker is not run.
    These use a shell script and skip on Windows, as `fakeRuntime` does.
    Each case asserts what fails before the fix, as the design lists it by
    command, and is seen to fail against `resolveRuntime` as it is at
    `119a0e1` (GLOBAL-04, GLOBAL-06, GLOBAL-10, GLOBAL-11).
13. **Tier 0, the transitional forms** (TEST-01, TEST-02; Validation plan,
    "The transitional forms"): `cmd/warden/runtime_former_test.go`, headed
    by the marker `TRANSITIONAL(runtime-former-values)`, one table, the
    same on every OS, with the design's rows: the fix names `docker` for
    `Docker` and `docker.exe`; the three paths to `docker` give that name
    and `PATH` and no directory; the two `nerdctl` rows give its message;
    `/usr/local/bin/docker-ci` and `./rt` give the wrapper message; `sh`,
    `dokcer` and ` docker` are not recognized. Every row of the detection
    has a case (TEST-06), and for every row the check of task 3 refuses the
    value, so the detection cannot accept anything (UX-03, UX-04,
    GLOBAL-07).
14. **Tier 0, nothing of the value outside the quotes** (TEST-01, TEST-02;
    Validation plan, the bullet of that name; Security note, point 5): for
    a path whose directory holds an escape character and a newline, from
    each source, the value is in the message once, quoted, and neither the
    directory nor either character is anywhere else in the message or the
    fix. It lives with the permanent error's tests, not in
    `runtime_former_test.go`: it holds without the detection and stays when
    the detection is removed (GLOBAL-06, UX-04).
15. **Tier 1, the refusal cases in `test-integration.sh`** (TEST-02;
    Validation plan, Tier 1, all but its last bullet; SEC-04). A temporary
    project directory with a Dockerfile, a `warden.toml` and an executable
    `rt` that writes a marker; `$WARDEN` is made absolute first, because the
    cases change directory. The cases are the design's: `cli = "./rt"` on
    `build`, `clean` and `doctor` (exit 1, `To fix:`, no marker, no output
    directory); `[Runtime]` with `CLI = "./rt"` on `clean`; `cli = "./rt"`
    with `driver = "qemu"` in the file, and with `--driver vz`, on `clean`;
    the executable's absolute path in `--runtime` and in `WARDEN_CTR_CLI`,
    with no `cli` in the file, on `doctor` (exit 1, no marker); the `./rt`
    file under `--runtime docker` and under
    `WARDEN_CTR_CLI=docker` on `doctor`, which prints `container runtime:
    docker`; the `./rt` file with `warden setup` and with `warden clean
    --driver hyperv`, exit 0. They count in the script's `PASS` and `FAIL`
    as its other assertions do (GLOBAL-04, GLOBAL-06, UX-08, UX-09).
16. **Tier 1, a named runtime from a project's file still builds**
    (TEST-02; Validation plan, Tier 1, its last bullet). One of the three
    builds the script already runs is run from a directory whose
    `warden.toml` names the runtime, as Note C says, at no extra build; its
    existing assertions stay (GLOBAL-04, FLOW-06).
17. **The rule change UX-08, amend** (`rule change UX-08 approved at
    a2afd08`; FLOW-14). Paths: `docs/development/ux-cli-config.md` and
    `tools/tier1/rules.yaml`. In the first, the UX-08 block (heading,
    severity, check and body) is replaced by the wording under **Rule
    changes**, "Proposed wording", copied word for word and not
    paraphrased: the heading "UX-08 — Fail fast on config, in the component
    that uses it", severity MUST, check `agent-judgment`, and its four
    paragraphs (LINT-01, LINT-08). In the second, the line `UX-08:
    {follow-up: ...}` is removed. Both in one commit, so that the
    `rule-index` self-check never sees one without the other. Nothing else
    in either file changes, GLOBAL-10 is not edited, and
    `docs/development/loop-profile.md` needs no edit (PROFILE-01).
18. **`docs/book/src/guide/configuration.md`** (UX-01; **Reference docs**):
    the `cli` line of the Full Reference; a section on the container
    runtime in place of "Runtime Autodetection", with the three values and
    that they are the same from every source, the precedence, detection,
    which commands check the value and that a run with another driver does
    not, and the error; inside it the subsection "A path, or another
    runtime" with the Compatibility table, under an HTML comment that
    carries `TRANSITIONAL(runtime-former-values)`; the `WARDEN_CTR_CLI` row
    ("finch, docker or podman") and the `--runtime` row (UX-03, UX-07).
19. **`docs/book/src/guide/cli.md` and `installation.md`** (UX-01, UX-05):
    in `cli.md`, `--runtime <name>` under Global Flags says the three are
    the only values, one sentence in `warden doctor` and one in `warden
    clean` say that an unsupported runtime in effect fails the command (for
    `clean`, under every driver but `firecracker` and `hyperv`);
    `docs/book/src/getting-started/installation.md` gains the link to the
    section of task 18. The help of `--runtime` in `cmd/warden/main.go` and
    `README.md` stay as they are: what they say stays true.
20. **`docs/book/src/concepts/overview.md`** (FLOW-15): in `warden build`,
    after the `req := resolve flags and config` line, the design's five
    lines on how the container driver's runtime is resolved and refused,
    and the note that `warden clean` resolves it the same way under every
    driver but `firecracker` and `hyperv`.
21. **Green, then push** (FLOW-17). `make lint` (which runs tier 1), `make
    test` and `make enforcement` (the branch changes `tools/tier1/`:
    FLOW-14) pass in the feature checkout; `make integration-test` is run
    where a container runtime answers, and the PR's `integration` workflow
    runs it otherwise (TEST-02, TEST-03); the branch is pushed at each
    green checkpoint, never forced.
22. **At Publish** (FLOW-09, FLOW-10). The removal of the transitional
    detection is filed as a queue item on hold until 1.0, whose `source`
    names the marker `TRANSITIONAL(runtime-former-values)` and that
    condition (a4; UX-03). The two tracked gaps are confirmed open in the
    native-gap backlog (TEST-03). The merge of `main` checks the order the
    design assumes (this item, then `project-config-trust`, then K1): if
    either has merged first, **One config file** and **What K1 has to
    keep** say what follows (FLOW-06).

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
  `RefusedCLI` and the decision by level are gone (Decisions 1 and 3).
  Which file set the value is still noted, for the error's wording only,
  while `main` reads two files.
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

An independent review read the redraft at `3bf8784` the same day. What it
changed:

- Jeff's decision of the same day, recorded on `project-config-trust`
  (one authoritative `warden.toml`; the user-level file goes away), is now
  fitted: **One config file**; UX-08's proposed wording names no level of
  config file; the pointers to that item state its settled scope.
- UX-08's second paragraph is about settings in a config file or the
  environment, and a flag a run cannot use may be refused at startup. The
  table against `main` now lists the flag checks, which the first table
  left out. New question a6.
- The transitional messages print no directory taken from the value.
- The marker that tells whether the project's file set the key is
  specified as a string no TOML file can produce.
- The Compatibility table says that a wrapper which does more than set
  variables has no supported replacement.
- What K1 has to keep: the runtime read back from the state file is
  compared with the list again.
- The tests say which assertion fails before the fix for each command.

Not done here, and for the attended session: the two follow-up choices
this redraft rests on ("Every source"; the rule written in this item,
amending UX-08) are in the dispatch and in this design, and the queue's
recorded answers to a1 and a2 end before them. The queue item's title
still states the first draft's scope.

Still to come in this sitting: the UX-08 rule change, to be approved by
name, approved with a change, or rejected; the three new assumed
questions, a4 (when the transitional detection is removed), a5 (how UX-08
and `main` agree on the day this merges) and a6 (whether the rule reaches
flags); and the outcome.

### 2026-11-15, first sitting, outcome: approved

- **r1, 2026-11-15: approved at `a2afd08`** (Jeff; read at `b8f692f` and,
  after the redraft on his answers, at `534b768`; page
  https://claude.ai/artifact/Wchjm4BS8LognkSHAMiQup). Folded in at
  `a2afd08`, by his word in the sitting: a4 (d), the transitional detection
  is kept until 1.0, where the design proceeded with removal after two more
  releases. Confirmed as drafted, each asked on its own: a5 (a), the amended
  UX-08 lists the four older checks and this item moves none; a6 (a), the
  rule covers file and environment settings and a command may refuse a flag
  its run cannot use. a1 to a3 are as recorded in the entry above, with the
  two follow-ups answered in the same sitting: the limit to the supported
  runtimes holds for every source, and the general rule is written in this
  item as an amendment to UX-08. The Security note is acknowledged
  (GLOBAL-06). The nine separate findings were filed on his word as
  `project-config-trust` (one security design) and `configuration-bugfixes`
  (one feature, on hold behind the freeze).
- **rule change UX-08 approved at `a2afd08`**, as worded under **Rule
  changes**, with check type `agent-judgment`.

## Consolidation

Publish, 2026-11-15, on the branch at `ebfdd8a` (FLOW-10, FLOW-15).

**`main` since the fork.** The branch forked at `897a4a8` (#118,
alters-slot). One PR has landed since: #119, the Loop Contract (`ad4ea50`),
merged into the branch during the design review (`33b25c4`). `main` is
still at `ad4ea50`, so Publish's merge had nothing to do and made no merge
commit. #119 and this branch change no file in common. What #119 asks of
this item is met, as the design review's entry records: UX-08 is a project
invariant (`ux-cli-config.md`) and not a loop rule, so a feature may amend
it (FLOW-14 as amended, LOOP-09); `seams` is empty, so there is no
**Specification changes** section and none is added here (LOOP-02); the
item works only in this repository (LOOP-04); no entry of the profile
names UX-08 or a check of it (PROFILE-01, searched again here).

**The order of merging** (task 22). This item, then `project-config-trust`,
then `driver-contract` (K1); `prebuilt-artifacts` after
`project-config-trust`. None of the three is on `main`, so this item
merges onto two config files and onto the code it was designed against:
**One config file** and **What K1 has to keep** stand as written, and
neither of their "if it were to merge first" cases applies.

**What each sibling inherits.** Each design names this item; they were
read on their branches, which this step does not edit.

- **`project-config-trust`** (in its design review; read at `0bda630`).
  It keeps the rule and its code untouched: `containerRuntimes`,
  `supportedRuntime`, `resolveRuntime`, the permanent error
  (`unsupportedRuntime`), and `runtime_former.go` with its marker. It
  reworks what this item wrote for two files, by the names they have on
  the branch: the marker `unsetCLI` and the keep and put-back around the
  project file's decode in `LoadConfig`; the user file as a value of
  `RuntimeConfig.CLISource`, and with it the error that names that file
  by its path; in `configuration.md`, the user-level file in the
  precedence and "the user-level file's value" under the empty value; the
  overview's "(`./warden.toml`, then the user's file)", which becomes
  "(the config file)"; and in `runtime_test.go` the `user` level of the
  tables (`nameRuntime`, `inRuntimeLevels`), `TestRuntimeUnsetMarker`, and
  the cases of a user file under a project file. The fourth row of this
  design's rule table goes with them. The helper those tests share is
  `main`'s `inConfigFiles` (`vmdns_test.go`), which this branch extended
  to clear `WARDEN_CTR_CLI` and `WARDEN_DRIVER` and to reset the flags a
  case sets; its temporary `HOME` stays. The guide pages added in phase 5
  (`drivers.md`, `first-build.md`, `installation.md`) name no user-level
  file and need nothing from that item. UX-08 as merged here names no
  level of config file, so that item changes no word of it; it amends
  UX-07, the order UX-08 points to. Its own transitional code copies this
  item's shape (one file, one marker, removed at 1.0) and gets its own
  removal item. Its hold behind this item's merge (its a7) is a blocking
  question its review session raises; it is not in the queue yet.
- **`driver-contract`** (K1; approved at `a6c8f0b`, held behind
  `project-config-trust`'s merge). It carries **What K1 has to keep** as
  its own hand-off, the stricter point 4 included (the runtime read back
  from the state file is compared with the list again), so point 8 has
  nothing left open. What moves into the container driver, by name: the
  list and its check (`containerRuntimes`, `supportedRuntime`,
  `runtimeNames`), `DetectRuntime`, `resolveRuntime`'s three outcomes,
  `unsupportedRuntime`, and `runtime_former.go` with its call and its
  tests under the same marker; `CLISource` becomes the kit's label of the
  source. The tables of `runtime_test.go` move with the code and gain the
  rows for `[driver.container] cli`. Test 0 of `test-integration.sh` runs
  the built `warden` and passes there unchanged but for `clean` under
  `qemu` and `vz` (K1's Behaviour 5). UX-08: K1 edits the last paragraph
  only, and the paragraph on this branch is byte for byte the one K1's
  design quotes as current (compared here), so K1's condition for making
  that edit holds once this merges. Left for K1 in `driver/container`,
  which this item does not edit: the `nerdctl` arm of `copyExecutor`,
  which no value can reach through `warden` any more. Three fixed
  strings in `cmd/warden` name the three runtimes beside the list and
  stay as they are, as the design says of the flag's help: the help of
  `--runtime`, `warden doctor`'s help, and `DetectRuntime`'s "tried
  finch, docker, podman". They move with the driver.
- **`prebuilt-artifacts`** (in Design; read at `36e8051`). It shares no
  code with this item. It takes the convention (an error at once for
  what stops working, the detection of the old state kept until 1.0
  under a `TRANSITIONAL(<name>)` marker) and UX-08 as amended, for when
  its lookup is validated.
- **`configuration-bugfixes`** (on hold): as **The same defect
  elsewhere** says. F8 no longer reaches `[runtime] cli`; F6 and F7
  stand.

**A convention, not made a rule here.** "A change that refuses what
worked before is an error at once with a message that says what to
change, and the code that words it is transitional, in one file under one
marker, removed at 1.0" is Jeff's answer to a3 and a4 for this item.
`project-config-trust` follows it by his answer in its own review, and
`prebuilt-artifacts`' draft proposes it. K1 does not for its renamed
keys, which work and warn as UX-03 says; its design notes that a release
then has two conventions. It is written in no rule: the only rule change
this review approved is UX-08, and UX-03 is not edited (FLOW-10,
FLOW-14). Whether it becomes a rule is Jeff's to decide;
`project-config-trust`'s design leaves the same question to the review
that is asked first. Nothing is filed for it here.

**Versioned seams** (GLOBAL-08). None: `seams` is empty, the branch
changes no seam, and #119 changed none.

**Rule changes.** UX-08, as approved at `a2afd08`: the block in
`ux-cli-config.md` and the removed line of `tools/tier1/rules.yaml`
(commit `8c4c4fc`). Nothing else under `docs/development/`,
`tools/tier1/`, `tools/workflow/` or `.claude/` is in the diff. The other
citation of UX-08 on `main`, GLOBAL-10's, still holds for a value a run
uses.

**Terminology.** Code, reference, guides, `overview.md` and this design
use the same terms: the *container runtime* is the program and the
*container driver* its consumer; a runtime is *supported* or is refused;
what is checked is the *value in effect*; a *level* is a place in the
precedence, the word UX-07 and GLOBAL-10 use on `main`, and the *source*
is where the value in effect was named (`CLISource`, as `main`'s
`BuildLinkSource`); the error has a *permanent* part and a *transitional*
part. Wording fixed here:

- *The two phrases the Review left.* The transitional message for a path
  to another program says "if the program only set variables" and "no
  supported way to run another program as the runtime"; this design's
  table said "the file" and "a program of one's own". The binary's words
  are kept and the table's fourth row now has them. The message's first
  clause already names a file (`[runtime] cli in warden.toml`), so "the
  file" in its fix would read as the config file, and "another program"
  repeats the words of the message's own first line. The tests pin the
  binary's text (`runtime_former_test.go`).
- *Note B.* Its sentence "`doctor` is read-only under every driver" was
  wrong for `firecracker` and is corrected in place (Review S1).
- *The marker is in five places, not four.* The Guide added an upgrade
  note to `installation.md` that links to the transitional subsection of
  the reference, so it carries `TRANSITIONAL(runtime-former-values)` and
  goes with it. `runtime_former.go`'s header comment said "all four" and
  now lists the five: the file, its call in `unsupportedRuntime`, its
  test file, the subsection "A path, or another runtime" of
  `configuration.md`, and that note. **Where the transitional part
  lives** above lists the first four. A search for the marker finds those
  five and this design, which is kept; the header comment says so and
  names each place by its file (after Publish, below).

**Plan notes A to D, and what Implement did beyond the plan** (FLOW-06;
each is in the commit log, none is silent).

- *Note A* stands, with the sentence Implement added on the Review's
  word (`9af1a71`): tier 1 skips `clean --driver hyperv` on a Windows
  host too.
- *Note B* stands as corrected above. At Publish the four `doctor` rows
  did not move the user cache directory (Review S1); they do since the
  fix after Publish, below, and the note says so.
- *Note C* stands: the tier-1 build names the runtime `warden doctor`
  reports on the host, which is `docker` on the CI runner.
- *Note D* stands. The Security note's list for the TEST-08 suite is not
  added to Arm A, because the corpus is not built
  (`docs/development/security-suite.md`, "Remaining build work"), and no
  queue item builds it, so there is nothing to hand the list to. It stays
  in this design, which the item that builds the corpus finds by its
  `concepts` (SEC-03, FLOW-12). Its proof is asserted in tiers 0 and 1.
- *Task 12, widened.* The never-run cases also call `runBuild` and
  `runShell`, beside `newDriver` (`8e41bd8`): more coverage, the same
  assertions.

**FLOW-15 sweep.**

- *Dead code.* `make deadcode` on linux, darwin and windows, exit 0: 66
  lines (17, 22 and 27), the same list as `main` at `ad4ea50`, compared
  line for line. None is in `cmd/warden` or in a file this branch
  touches. The 13 functions unreachable on all three are `main`'s: ten
  in `driver/vz`, FLOW-15's cgo case, which is checked on a macOS host,
  and three exported methods of the library packages `relay/` and
  `collector/`. The branch adds no unreachable function and removes
  none.
- *Duplication.* One list (`containerRuntimes`) serves detection, the
  check, the error's names and the transitional messages; `DetectRuntime`
  lost its own literal. `Runtime.CLI` has one reader outside
  `LoadConfig` and `resolveConfig`, which set it: `resolveRuntime`, with
  the error it builds. No
  second test helper was added for the two config files: the new tests
  use `main`'s `inConfigFiles`. `main` also has `inConfigDirs`
  (`buildlink_test.go`), an older helper for `build_link` alone; it
  predates the branch, and `project-config-trust` turns the two-file
  cases of both into one-file cases, so the two are not merged here.
  `shipPrograms`
  (programs in a project directory that write a marker) and `main`'s
  `fakeRuntime` (a fake on a `PATH` of its own) do different jobs.
- *The overview.* `overview.md`'s `warden build` has the five lines on
  how the container driver's runtime is resolved and refused, and the
  note on `warden clean`; both match `resolveRuntime` and `runClean` as
  built. No other component's flow changes.

**Review findings for the PR body** (report
`reports/runtime-cli-project-path/9af1a7173fa5.md`: 0 MUST, 0
MUST-review, 2 SHOULD, both in test code. Both were open at Publish, for
the human to acknowledge at merge, FLOW-08; both are fixed since, in the
commit after `9cb1dc0`: **After Publish**, below).

- *S1 (TEST-01), fixed after Publish.* `TestDoctorIgnoresAnUnusedRuntime`
  moved `HOME` but not the user cache directory, so with `XDG_CACHE_HOME`
  set `make test` left `warden/firecracker/work` in the caller's cache.
  Fix: set `XDG_CACHE_HOME` and `LocalAppData` for the child, as the
  `clean` rows do.
- *S2 (TEST-02), fixed after Publish.* Test 3 of `test-integration.sh`
  ran its build with the caller's environment; where `WARDEN_CTR_CLI` was
  set, the file's value was never read and the case passed for the wrong
  reason. Fix: run that build and its `doctor` call under
  `env -u WARDEN_CTR_CLI -u WARDEN_DRIVER`. On the CI runner neither
  variable is set.
- *For the human's eye, not a finding.* The transitional message for a
  path in a project's file does not end with "`--runtime` with one of
  those replaces it for one command", as the permanent error for a file
  does. That is the design's table as approved, and the reference says
  it. Adding the clause would be a wording change for Design.

**Validation.** `make lint`, `make test` and `make enforcement` pass at
this step's head. At Publish `make integration-test` had not run for this
branch: no container runtime answers on the host Publish runs on, and the
PR's `integration` workflow was its first run (Test 0 of the script
passed on its own at Review, 12 of 12). That run has passed since:
**After Publish**, below. No host blocks merge (FLOW-09). The two
tracked gaps are open in the native-gap backlog, as task 22 asks:
`runtime-cli-project-path-windows` (`container-any`) and
`runtime-cli-project-path-finch` (`macos-finch`).

**Follow-ups.** Consolidation found no regression that `main` already
has and files none (FLOW-09). The removal of the transitional detection
at 1.0 (a4 (d), task 22) is filed by the attended session that runs this
step's dispatch, not by this step: a queue item on hold until the change
that prepares 1.0, whose `source` names the marker
`TRANSITIONAL(runtime-former-values)`, the five places it is in, and
that condition. It is its own item and does not `follow` this one: the
design asks for no more, and it has nothing to fix. It is filed:
`runtime-former-values-removal` (**After Publish**, below).

**After Publish** (2026-11-15, PR #120 open at `9cb1dc0`; FLOW-12). One
commit after `9cb1dc0` changes test code, one header comment and this
design, and no behavior of `warden`.

- *Tier 1 has run.* The PR's `integration` workflow passed on `9cb1dc0`:
  42 assertions, none failed, Test 0's twelve among them, and Test 3's
  build from a directory whose `warden.toml` names the runtime ("built
  with cli = "docker" in the project's warden.toml"). Task 16 is
  verified on the CI runner. All 22 checks of the PR passed on that
  commit.
- *The removal at 1.0 is filed.* Queue item
  `runtime-former-values-removal` (workflow branch, `7e9d092`): priority
  73, on hold until the change that prepares 1.0 and not before this
  item's PR has merged. It does not `follow` this item. Its `source`
  names the marker and the five places.
- *S1 is fixed.* The four rows of `TestDoctorIgnoresAnUnusedRuntime` set
  `XDG_CACHE_HOME` and `LocalAppData` to empty temporary directories
  before the child's environment is copied. Seen on Linux with
  `XDG_CACHE_HOME` at a scratch directory: before, the test left
  `warden/firecracker/work` there; after, it leaves nothing, and neither
  does `go test ./cmd/warden` as a whole. No other test of
  `runtime_test.go` or `runtime_former_test.go` runs a command under a
  VM driver without that: the `clean` rows of Note A already move the
  cache, `clean` under `qemu` and `vz` is refused before any work, and
  the `newDriver` rows construct a driver and write nothing.
- *S2 is fixed.* Test 3's `doctor` call and its build run under
  `env -u WARDEN_CTR_CLI -u WARDEN_DRIVER`. Seen with fakes of `docker`
  and `podman` on `PATH`, `WARDEN_CTR_CLI=podman` in the caller and
  `cli = "docker"` in the project's file: the build line ran `podman`
  before and runs `docker` now. Test 0's twelve cases already run
  through `rt_warden`, which removes both variables and sets
  `WARDEN_CTR_CLI` only in the two cases that are about it; they pass,
  12 of 12, with `WARDEN_CTR_CLI=bogus` in the caller. Tests 1 and 2
  are `main`'s, name no runtime in a file, and are not changed.
- *The marker's places.* `runtime_former.go`'s header comment names each
  of the five by its file, and says that a search also finds this
  design, which is kept.

## Rollback notes

None.

## Archive summary

Designed 2026-11-14 and 2026-11-15 as a security fix, exempt from FLOW-20's
freeze. The first draft limited only the project's file; the design review
of 2026-11-15 changed a1, a2 and a3 in Jeff's words and approved the
redraft at `a2afd08`, with rule change UX-08 (amend) approved by name at
the same commit. Planned (22 tasks) and implemented the same day; tier 2 at
`9af1a7173fa5`: 0 MUST, 0 MUST-review, 2 SHOULD, both fixed after Publish.
`main` moved once since the fork (#119, merged into the branch during the
review). Published 2026-11-15 as PR #120; this summary was refreshed the
same day, for the commit after `9cb1dc0` (FLOW-12).

- **The hole.** `[runtime] cli` in the working directory's `warden.toml`
  was run with `exec.Command` as given, so a cloned repository could make
  `warden build`, `shell` or `clean` run a program it ships, on the host,
  before any isolation exists and outside the ledger: `cli =
  "./tools/docker"`, or `cli = "sh"` with a file named `pull` beside it.
  `warden doctor` printed the value without running it.
- **Built** (all in `cmd/warden`; no `driver/` tree is edited). The
  container runtime is `finch`, `docker` or `podman`, or not set, from
  every source: `--runtime`, `WARDEN_CTR_CLI`, and `[runtime] cli` in
  either config file. One list, `containerRuntimes` (`config.go`), serves
  detection, the check (`supportedRuntime`), the error's names and the
  transitional messages. `resolveRuntime` (`main.go`) is the only reader
  of `Runtime.CLI`: empty means `DetectRuntime`, a member of the list
  returns the list's own string, and anything else is the error from
  `unsupportedRuntime`, before anything is looked up, run or created. Its
  callers are `newDriver` (`build`, `shell`), `doctor` and `runClean`
  (every driver but `firecracker` and `hyperv`). The match is exact:
  no letter case folded, no `.exe` stripped, nothing trimmed. The error is
  a `driver.FixableError`, exit 1; it names the source (`CLISource`, a
  field beside the value), prints the value once with `%q`, and takes no
  other text from it. `LoadConfig` tells whether the project's file set
  the key with a marker no TOML file can hold (`unsetCLI`, `"\xff"`), not
  with the decoder's `IsDefined`, so every spelling of the key is seen.
- **Built: the transitional part.** `runtime_former.go`
  (`formerRuntimeValue`) words the refusal for the four forms that ran
  before the fix: a supported runtime in another letter case or with
  `.exe`; a path to a supported runtime; `nerdctl`; a path to any other
  program. It only words: every value it sees has already been refused.
  The marker `TRANSITIONAL(runtime-former-values)` is in five places: that
  file, its call in `unsupportedRuntime`, `runtime_former_test.go`, the
  subsection "A path, or another runtime" of `configuration.md`, and the
  upgrade note in `installation.md`. All five go at 1.0 (a4), by the
  queue item `runtime-former-values-removal`; a search for the marker
  also finds this design, which is kept.
- **Built: the rule.** UX-08 as amended (`ux-cli-config.md`, check type
  `agent-judgment`; its follow-up line leaves `tools/tier1/rules.yaml`): a
  setting held in a config file or the environment is validated by its
  consumer, when it initializes, after the overrides, and only when the
  run uses it. It lists four older checks on `main` that it does not yet
  describe, and this item moves none (a5). A command may still refuse, at
  startup, a flag its run cannot use (a6).
- **Built: tests and docs.** `runtime_test.go` covers the four sources,
  six spellings of the key, `build`, `shell`, `clean` and `doctor`, that
  the shipped program is never run (a marker file stays absent) and that
  an unsupported value has no fallback; with `resolveRuntime` put back to
  `main`'s body, 8 test functions fail. Test 0 of `test-integration.sh`
  runs the built `warden` against project directories, and Test 3 builds
  with the runtime named in the project's file, with the caller's
  `WARDEN_CTR_CLI` and `WARDEN_DRIVER` removed so that the file's value
  is the one read. The PR's `integration` workflow passed on `9cb1dc0`
  (42 assertions, Test 3's build with `cli = "docker"` among them). The
  tests that run `clean` or `doctor` under a VM driver give it an empty
  user cache directory: firecracker's `doctor` writes there. Reference:
  `configuration.md` (the container runtime section) and `cli.md`; guides:
  `drivers.md`, `first-build.md`, `installation.md`; `overview.md`'s
  `warden build` and its note on `warden clean`.
- **Decisions taken.** Three values from every source, not only from the
  project's file (1, a1). The consumer validates the value in effect, and
  a run that does not use the setting does not read it: `[runtime] cli =
  "nonsense"` is no error under `--driver hyperv`, and `--runtime docker`
  builds in a directory whose file is refused (2, a2). The value run is
  the list's own string, never the configured one (3). An error from the
  first release, with a permanent part and a transitional part kept until
  1.0 (4, a3, a4). A break for values that ran before, from every source,
  with no warning period (5, UX-03): a wrapper that does more than set
  variables, and `nerdctl` without Finch, have no supported replacement.
  UX-08 amended here, naming no level of config file (6).
- **Rejected.** Limiting only the project's file (the first draft, with
  `Runtime.RefusedCLI` and a walk of the decoder's metadata). Any bare
  name looked up on `PATH` (`cli = "sh"` shows a name is not enough).
  Adding `nerdctl`. Normalizing case, `.exe` or space. Validating as the
  file loads, on every command and driver. Skipping an unsupported value
  for the next level or detection (GLOBAL-10, GLOBAL-11). A release that
  warns and still runs the value. Removing the transitional detection
  after two releases. Printing a directory from the value in a fix.
  Resolving the name to an absolute path or checking the file's owner.
- **Closed after Publish.** Review SHOULDs S1 (TEST-01:
  `TestDoctorIgnoresAnUnusedRuntime` did not move the user cache
  directory, and firecracker's doctor writes a probe there) and S2
  (TEST-02: Test 3 of `test-integration.sh` inherited the caller's
  `WARDEN_CTR_CLI`) are fixed in the commit after `9cb1dc0`. `make
  integration-test` had its first run in the PR's `integration` workflow,
  on `9cb1dc0`, and passed; no container runtime answers on the host the
  loop built this on.
- **Open.** Two tracked native gaps, neither blocking:
  `runtime-cli-project-path-windows` (`container-any`) and
  `runtime-cli-project-path-finch` (`macos-finch`).
  The Security note's entry for the TEST-08 suite (Arm A) is in this
  design only, because the corpus is not built. The removal of the
  transitional detection at 1.0 is a queue item of its own,
  `runtime-former-values-removal`: priority 73, on hold until the change
  that prepares 1.0; it does not `follow` this item. Whether "an
  error at once, with transitional wording kept until 1.0" becomes a rule
  is Jeff's to decide; it is written in no rule here.
- **Not this item's.** What a project's file still chooses (the relay
  image, the relay's trust roots, host directories, the Hyper-V base
  image, the driver, and which of the three runtimes) and the user-level
  config file are `project-config-trust`'s (F1 to F4, F9). F5 to F8 are
  `configuration-bugfixes`'. The order of merging is this item, then
  `project-config-trust`, then `driver-contract` (K1).
- **For future designs touching `container-runtime`, `runtime-cli`,
  `supported-runtimes`, `config-validation`, `consumer-validates`,
  `lazy-validation`, `project-config`, `build-context-trust`,
  `host-execution`, `path-lookup` or `transitional-detection`:** a value a
  build context can set never reaches `exec` as a program; compare it with
  a fixed list and run the list's own string. Keep one reader of such a
  setting, and check the value in effect, so that the check does not
  depend on which file set the key or how the key was spelled. Add a
  runtime by adding it to `containerRuntimes` (K1 moves the list, the
  check, detection and the transitional file into the container driver;
  **What K1 has to keep** is the hand-off, and it compares the runtime
  read back from the driver's state file with the list again). An
  unsupported value in effect is an error, never a fallback. A new setting
  follows UX-08 as amended: validated by its consumer, after the
  overrides, only when the run uses it. An error that quotes a value from
  a file someone else wrote prints it once with `%q` and builds the fix
  from fixed text. What is left open on this boundary is in the Security
  note: the name is looked up on the user's `PATH`, and a repository
  still chooses among the three runtimes until `project-config-trust`.
  Code that exists only to word a refusal of what once worked sits in one
  file, behind one call, under one `TRANSITIONAL(<name>)` marker.
