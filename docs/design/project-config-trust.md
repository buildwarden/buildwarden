---
feature: project-config-trust
status: draft
drivers: [container, qemu, vz, hyperv, firecracker]  # conservatively (FLOW-02): the change is in
                         # cmd/warden and in three shared files of driver/ (hostutil.go, session.go,
                         # detach.go) that every driver uses. In the driver/<name>/ trees only
                         # two test files change (hyperv, qemu), to name the source tree
seams: []                # the claim's upstream_ca_certs field carries what it carried, PEM
                         # certificates; relaycfg/ is not edited
surfaces: [cli, config, output]  # cli: --config, a global flag. config: one file, found by
                         # --config, WARDEN_CONFIG or ./warden.toml; ~/.config/warden/config.toml is
                         # no longer read; WARDEN_UPSTREAM_CA_CERTS replaces the key. output: one
                         # "config:" line saying which file was read
security_boundary: host-env  # which file warden acts on, on the host, before any isolation
                         # exists, and which source tree a development warden builds from. It
                         # also reaches env-relay: the file chooses the relay image and its
                         # upstream trust roots (Security note)
alters_relationships: false  # inside the orchestrator's existing role, reading its own
                         # configuration and finding its own source. No seam, no bootstrap change,
                         # no change of owner, no change to driver.Driver or driver.Session
design_review: required  # FLOW-19: a security boundary, three rule changes, assumed questions
native_validation:
  - {host: container-any, blocking: false, reason: "no driver's own code is changed and the lookup is the same code on every OS; what only a Windows host adds is go test ./cmd/warden and ./driver there (CI does not run make test on Windows), the transitional error naming the file under %USERPROFILE%, a drive-letter path in --config, and WARDEN_UPSTREAM_CA_CERTS split on ';'"}
  - {host: windows-hyperv, blocking: false, reason: "in driver/hyperv only a test file is edited, and CI runs it; what only this host adds is the detached image prepare reading the file its build read (the argument list is built in shared code and unit-tested), and a development warden run from its tree still finding its guest binaries from the executable"}
  - {host: macos-vz, blocking: false, reason: "driver/vz is not edited; what only this host adds is sudo warden image prepare --driver vz with a ~/.config/warden/config.toml present (which home sudo gives), and a development warden run from its tree still staging the relay and warden-io from the executable"}
  - {host: qemu-any, blocking: false, reason: "in driver/qemu only a test file is edited, and CI runs it; what only this host adds is a development warden run from its tree still finding the relay VM assets in the source tree, by walking up from the executable"}
tier2_gaps:
  - {host: container-any, gap: "on the Windows desktop: go test ./cmd/warden ./driver; with %USERPROFILE%\\.config\\warden\\config.toml present, warden doctor exits 1 naming that path, and WARDEN_CONFIG set to it clears the error; --config C:\\x\\warden.toml is read and .\\warden.toml is not; WARDEN_UPSTREAM_CA_CERTS=C:\\a.pem;C:\\b.pem names two files; --config NUL reads no settings (backlog project-config-trust-windows)"}
  - {host: windows-hyperv, gap: "a hyperv build with --config naming a file outside the working directory whose [vm] image_refresh_after is due starts the detached image prepare, and its log's config line names that file; a development warden run from its tree builds (backlog project-config-trust-hyperv)"}
  - {host: macos-vz, gap: "sudo warden image prepare --driver vz and warden build --driver vz with ~/.config/warden/config.toml present each exit 1 naming the file, under sudo naming the file in the home sudo gives; a development warden run from its tree builds with --driver vz (backlog project-config-trust-vz)"}
  - {host: qemu-any, gap: "a development warden run from its tree builds with --driver qemu, the relay VM assets found in the tree; a copy of that warden outside the tree, run inside the tree, reports the missing source tree and builds nothing (backlog project-config-trust-qemu)"}
rules: [GLOBAL-01, GLOBAL-04, GLOBAL-06, GLOBAL-07, GLOBAL-09, GLOBAL-10, GLOBAL-11, ORCH-01,
        ORCH-03, RELAY-08, UX-01, UX-03, UX-04, UX-05, UX-06, UX-07, UX-08, UX-09, TEST-01,
        TEST-02, TEST-03, TEST-06, TEST-08, SEC-01, SEC-03, SEC-04, FLOW-01, FLOW-02, FLOW-05,
        FLOW-06, FLOW-07, FLOW-08, FLOW-09, FLOW-10, FLOW-12, FLOW-14, FLOW-15, FLOW-17,
        FLOW-19, FLOW-20, LOOP-04, LOOP-09, PROFILE-01]
rule_changes:
  - {rule: ORCH-03, change: amend, paths: [docs/development/orchestrator.md]}
  - {rule: UX-07, change: amend, paths: [docs/development/ux-cli-config.md]}
  - {rule: UX-08, change: amend, paths: [docs/development/ux-cli-config.md]}
concepts: [config-file, config-lookup, config-precedence, config-levels, config-trust,
           build-context-trust, environment-variables, user-config-removed,
           transitional-detection, module-root, relay-image, upstream-ca, security-fix]
related: [build-link-subnet.md, design-principles.md]
---

# One warden.toml, in the control of whoever runs the build

## Problem

`warden` reads two config files on every command that loads configuration:
`~/.config/warden/config.toml` and then `./warden.toml`, the second over the
first (`LoadConfig`, `cmd/warden/config.go`). Two MUST rules say so: ORCH-03
("Configuration precedence stays `flag > warden.toml > ~/.config/warden`")
and UX-07 ("invocation > project > user"). There is no flag and no variable
that names another file.

In a cloned repository `./warden.toml` is the repository's. The design of
`runtime-cli-project-path` surveyed what that file decides on the host and
filed what it found here (its first draft, `b8f692f`, section "The same
defect elsewhere"):

- **F1.** `[runtime] relay_image` chooses the relay, the component that
  records and signs the ledger, and a value that starts with `-` reaches the
  container runtime's command line as an option.
- **F2.** `[relay] upstream_ca_certs` names host paths whose bytes are read
  and sent to the relay as upstream trust roots; `system_ca_bundle = false`
  makes them the only roots.
- **F3.** `[build] output_dir` and `tmpdir` name host directories that
  `warden` creates and writes into.
- **F4.** `[vm] windows_image` names the Hyper-V base image that goes into
  the user's cache, where builds of every project use it.
- **F9.** Not a key: `driver.ModuleRoot` takes the working directory for
  `warden`'s own source tree, so a development `warden` installed outside
  its tree builds the relay and `warden-io` from a repository that declares
  `module github.com/buildwarden/buildwarden`.
- **The whole file.** `./warden.toml` outranks the user's own file for
  every key, so a repository can move a user onto a driver that isolates
  less, or change what the user chose to capture.

Each of these was read as a question of what a *project's* file may decide
against the *user's*. Jeff, 2026-11-15, in the design review of
`driver-contract`, asked which of the two files wins:

> What do you mean by 'yours'? warden.toml is meant to be effectively in
> full control by a build executor and there shouldn't be any precedence
> necessary. Configuration should only be through a local 'warden.toml',
> environment variables, and command-line arguments. By default, the
> warden.toml's location in the working directory is simply for ease of
> use, but there should only be one authoritative warden.toml for any build

and then decided, each asked on its own (the queue item's history):

1. The user-level file goes away. `warden` reads one `warden.toml`, plus
   environment variables and the command line. ORCH-03 and UX-07 are
   amended.
2. The file is `./warden.toml` by default; a new `--config` flag and
   `WARDEN_CONFIG` variable name another file, which is then the only one
   read.
3. This item carries the change; `driver-contract` and
   `runtime-cli-project-path` are written to fit it.
4. An existing `~/.config/warden/config.toml` makes `warden` stop with an
   error that names the file and says what to do. That detection is
   transitional and removed later.

So there is no project level and no user level, and the findings change
shape. With one file in the control of whoever runs the build (the
*executor*), F1 to F4 are the executor's own choices when the file is the
executor's. What is left is (a) the default: `./warden.toml` in a working
directory that is someone else's repository; (b) the parts of F1 to F4 that
are defects whoever owns the file; and (c) F9, which no config file causes.

This is a feature item, not a program item: its `related` does not list
`componentization.md`. FLOW-20's freeze exempts it as a fix for a security
problem.

## Prior art

- **`runtime-cli-project-path`** (in its design review; not on `main`; read
  at `3bf8784` on its branch, cited here as proposed). It merges before this
  item. It limits the container runtime to `finch`, `docker` or `podman`
  from every source, checked in `resolveRuntime` on the value in effect; it
  proposes an amendment of UX-08 (the consumer validates, after the
  overrides, only what the run uses); and it sets the pattern this design
  copies for a change that refuses what worked before: an error at once,
  with a transitional part in one file behind one marker
  (`TRANSITIONAL(runtime-former-values)`), removed after a window (its a4).
  Its findings F1 to F4 and F9 are this item's `source`; F5 to F8 are
  `configuration-bugfixes`'.
- **`driver-contract`** (K1, in its design review; not on `main`; read at
  `8f61932`). It names the config files to a driver in `config_files`, each
  with a `level` (`project` or `user`), proposes an ORCH-03 paragraph on
  drivers, and decides that a project's file may name a driver but not give
  a path (its a16). All three were written for two files. Jeff's decision 3
  has K1 fit this design; what that means is in **How it sits with the
  other items**.
- **`build-link-subnet`** (archived). It added `BuildLinkSource` and
  `DNSSource`, the name of the file that set a key (`warden.toml` or
  `config.toml`), which `warden doctor` prints. With one file the label is
  that file's name; the mechanism stays.
- **`design-principles`** (archived): GLOBAL-09 to GLOBAL-11. Its list
  "Fallbacks found while drafting" names `WARDEN_UPSTREAM_CA_CERTS`, which
  is appended to the files' list instead of replacing it, as "for the design
  that touches it". This design touches it (Decision 4).
- **`native-allowlist-scratch-tools`** (archived). Its launcher,
  `tools/workflow/native/native-run.sh`, passes any `WARDEN_*` variable to a
  `warden` under test, and its design accepts that `WARDEN_CTR_CLI` chooses
  a program. `WARDEN_CONFIG` is one more such variable and gives a native
  session nothing it does not already have: the session writes the working
  directory's `warden.toml` today.
- **Rules:** GLOBAL-06 (the boundary), GLOBAL-09 to GLOBAL-11 (defaults,
  exclusive overrides, no silent fallback), ORCH-03 and UX-07 (amended
  here), UX-08 (as `runtime-cli-project-path` proposes it, plus one clause
  here), UX-01 and FLOW-15 (the docs), UX-03 (**Compatibility**), UX-04 and
  UX-09 (the errors), UX-05 and UX-06 (the flag), SEC-01, SEC-03 and SEC-04
  (the corpus entries), the umbrella's Decision 18 (a rule changes in the
  item that makes it true).

No archived design covers how the config file is found, or removes a config
level. The designs relied on carry front-matter already.

## Decisions

1. **One file, found in this order, and no other is read or merged**
   (Jeff's decisions 1 and 2): the file `--config <path>` names; else the
   file `WARDEN_CONFIG` names; else `./warden.toml` when it exists; else no
   file, and the defaults apply. The flag is over the variable, as for every
   setting. The edges (assumed question a8):
   - *A named file that cannot be read is an error*, never a reason to try
     the next source (GLOBAL-10): it does not exist, it is a directory, or
     it cannot be opened. The error names the path and whether the flag or
     the variable gave it.
   - *An empty value is not set*: `--config ""` and `WARDEN_CONFIG=` fall
     through to the next source, as an empty `--runtime`, `--driver` and
     `WARDEN_CTR_CLI` do today.
   - *To read no settings at all, name an empty file* (`--config /dev/null`
     on Unix). There is no flag for it.
   - *The default is read only when it exists.* A `./warden.toml` that
     exists and cannot be read, or is not TOML, is an error, as today for
     the syntax. Today any `stat` failure is read as "no file"; after this,
     only "does not exist" is.
   - *A file that is named is the only one.* With `--config` or
     `WARDEN_CONFIG` set, `./warden.toml` is not opened, parsed or checked.
     A variable the flag replaces is not opened either.
   - *Rejected: fall back to `./warden.toml` when the named file is
     missing.* A fallback on failure (GLOBAL-10, GLOBAL-11), and in someone
     else's repository it would hand the build to the repository's file
     because of a typing mistake.
   - *Rejected: a separate `--no-config`.* One more flag for what an empty
     file already says.
2. **`--config` is a global flag** (`rootCmd.PersistentFlags`), beside
   `--driver`, `--runtime`, `--color` and `--verbose` (UX-05), with no short
   form (UX-06: `-c` has no settled meaning across CLIs). The commands that
   read configuration are `build`, `shell`, `clean`, `doctor`, `setup` and
   `image prepare`. On the others (`inspect`, `image list`, `image verify`,
   `hyperv service`, `--version`, help) the flag is accepted and the file
   is not opened, as `--driver` is accepted there today.
   - *Rejected: register it on the six commands only.* It would be the one
     configuration flag that is not global, and `driver-contract` already
     proposes to refuse flags a command does not use (its Behaviour 2), for
     every flag at once.
3. **`~/.config/warden/config.toml` is not read, and its existence is an
   error** (Jeff's decision 4), on every command that reads configuration,
   whichever way the config file was chosen. One exception: when the file
   `--config` or `WARDEN_CONFIG` names *is* that file, it is the config
   file and there is nothing stale. The error is transitional: one file of
   code, one marker, removed after a window (assumed question a3).
   - Why it fires even with `--config` or `WARDEN_CONFIG` set: the point of
     the error is that settings which applied yesterday do not silently stop
     applying. They stop applying whichever file is read now. A machine that
     sets `WARDEN_CONFIG` for its builds and still carries an old user file
     with `[vm] dns` in it would otherwise lose its resolvers without a
     word.
   - *Rejected: only when no file is named.* It is the silent case above.
   - *Rejected: a warning.* Jeff, in `runtime-cli-project-path`'s review:
     "Use the logic that would be used for the warning to instead have a
     concrete error message about what needs to be changed/updated. This
     logic will eventually be removed like the warning would be".
   - *Rejected: read the file once more and print what to copy.* The
     message names the file; reading it is what stops.
4. **The precedence is flag, then environment variable, then the file, then
   the default, for every key, and the rules say so.** That is the order
   the code already applies; ORCH-03 and UX-07 leave the environment out.
   Every key was checked against it (**The precedence, key by key**). One
   does not follow it, and is changed: `WARDEN_UPSTREAM_CA_CERTS` is
   appended to `[relay] upstream_ca_certs` instead of replacing it. It
   replaces it from this item on, and for the transitional window setting
   both is an error that says so (assumed question a5).
   - *Rejected: keep the append and name it as a merge* (GLOBAL-10 allows a
     named merge). It would be the one variable of six that adds, in the
     item that writes "a value set at any level replaces the lower levels'
     value" for the environment.
5. **A relative path in the file is relative to the working directory**, as
   today and as the same path given by a flag or a variable is (assumed
   question a1). The file's own location carries no meaning: Jeff's "the
   warden.toml's location in the working directory is simply for ease of
   use".
6. **No environment variable is added for the keys that have none**
   (assumed question a2). A machine's own settings go in the file the
   machine names with `WARDEN_CONFIG`.
7. **What a config file may decide is not limited by where the file was
   found.** A `./warden.toml` read by default decides what a named file
   decides. `warden` says which file it read, on every command that reads
   one, so that the default is never silent (assumed question a4). The
   protection for a repository the executor does not control is to name the
   file: `--config`, or `WARDEN_CONFIG` once in the machine's environment.
   - *Rejected: refuse some values from a file found by default* (the relay
     image, CA paths, the base image). It is a second level by another
     name: "the file I named" over "the file I did not", with a list of
     keys the lower one may not set, which is the two-level rule Jeff has
     just removed. It would also refuse the common case, an executor's own
     project with `relay_image = "dev"` in its own `./warden.toml`, until
     they pass `--config ./warden.toml` to say the same thing.
   - *Rejected: no default file.* Jeff's decision 2 keeps it, for ease of
     use.
   - *Rejected: remember which files the user approved* (as `direnv allow`
     does). It needs per-user state to be read on every run, which is the
     user level again, and a prompt in a tool that prompts for nothing.
8. **Three things are fixed whoever owns the file**, because each is a
   defect with the executor's own file too (**F1 to F4, one by one**):
   - a relay image that is not shaped like an image reference (it starts
     with `-`, or holds a space or a control character) is refused, from
     the file and from `WARDEN_IMAGE`;
   - an upstream CA file is read and checked before anything is created,
     and only its certificates are sent to the relay: a file with no
     certificate, a certificate that does not parse, or a PEM block of any
     other kind (a private key) is an error that names the file (assumed
     question a9);
   - `WARDEN_UPSTREAM_CA_CERTS` is split on the OS's path-list separator
     (`;` on Windows), not on `:`, which cuts a drive letter in two.
9. **A development `warden` finds its source tree from its own executable
   only** (F9; assumed question a6). `driver.ModuleRoot` stops walking up
   from the working directory, and matches the module line exactly. A
   development `warden` whose executable is not inside its source tree
   reports that, with the fix, instead of building from whatever tree the
   working directory is in.
10. **A background `warden image prepare` reads the file its build read.**
    A qemu Windows or hyperv build whose prepared image is due for refresh
    starts `warden image prepare` detached (`driver.StartDetached`). Today
    the child finds `./warden.toml` by itself, through the working
    directory it inherits. With `--config` on the parent that would be a
    different file, so the parent passes `--config <absolute path>` to the
    child when it read a file.
11. **Three rule changes**, each made true by this item's code (the
    umbrella's Decision 18): ORCH-03 and UX-07 state the one file and the
    order with the environment in it; UX-08, as
    `runtime-cli-project-path` will have amended it, loses one plural
    (**Rule changes**).
12. **Other defects found are handed on, not fixed** (**What this item
    does not do**).

## Questions

### Blocking

None.

### Assumed

- **a1. A relative path in the config file (`[build] output_dir`,
  `tmpdir`, `[relay] upstream_ca_certs`, a local `[vm] windows_image`) is
  relative to what, now that the file may be elsewhere?** Proceeding with
  (a); confirm.
  - (a) The working directory, always: as today, and as `-o`, `--tmpdir`
    and `WARDEN_UPSTREAM_CA_CERTS` are. One rule for a path however it is
    given, and nothing changes for an existing file. Cost: the same file
    means different absolute paths from different directories, and a
    relative CA path in a file kept outside the project is looked up inside
    whatever directory `warden` runs in, a repository included. The
    reference says to write absolute paths in a file kept outside the
    project. **Recommended.**
  - (b) The directory that holds the config file, always: the convention of
    many tools, and a file then means the same wherever it is used. Nothing
    changes for `./warden.toml`. Cost: `output_dir = "out"` in a file under
    `/etc/warden/` writes every project's output to `/etc/warden/out`; "this
    project's `out`" can then be said only with `-o`. And a path means one
    thing in the file and another in a flag.
  - (c) By key: where a run writes (`output_dir`, `tmpdir`) is relative to
    the working directory; what the file points at (CA files, a local base
    image) is relative to the file. Each key does the natural thing, at the
    cost of two rules to remember.
  - (d) A relative path is refused in a file that `--config` or
    `WARDEN_CONFIG` names. It avoids the question and breaks the copy of a
    project's file used from elsewhere.
- **a2. Does this item add environment variables, so that a setting can
  still be given once per machine without a file?** Today six keys have a
  variable (`[runtime] driver`, `cli`, `relay_image`, `[relay]
  upstream_ca_certs`, `[output] verbose`, and `color` through `NO_COLOR`);
  fourteen keys in use have none (the list is in **Who is affected**).
  Proceeding with (a); confirm.
  - (a) None added. A machine's settings go in a file the machine names:
    `WARDEN_CONFIG=/path/warden.toml` in its environment. That file is then
    the only one read, so a project's own `./warden.toml` is not, and a
    user who had both a user file and project files keeps one file per
    build. **Recommended**: it is the model as Jeff stated it, and
    `driver-contract` moves seven of the keys to the drivers, whose kit is
    where a variable per driver setting would be decided.
  - (b) Variables for the settings that belong to a machine or its network
    and have neither flag nor variable: `[vm] dns` and `[relay]
    system_ca_bundle` (and `WARDEN_BUILD_LINK` beside `--build-link`). The
    corporate-network guide's settings then need no file. Three new names,
    chosen before `driver-contract` has settled which keys stay `warden`'s.
  - (c) A variable for every key, by one naming scheme. Every setting is
    then reachable at every level. Fourteen new names, several of them for
    keys `driver-contract` renames.
- **a3. When is the transitional detection removed?** It makes two errors:
  the one for an existing `~/.config/warden/config.toml`, and the one for
  `WARDEN_UPSTREAM_CA_CERTS` set together with the key (a5). After its
  removal an old user file is ignored without a word, and the variable
  replaces the key without a word. Proceeding with (a); confirm.
  - (a) The window `runtime-cli-project-path` gets for its own (its a4,
    still open; it recommends the release that first carries the change and
    the two after it, removed from the third). One window for both items'
    transitional code, and Publish files this item's removal as its own
    small queue item whose `source` names the marker. **Recommended.**
  - (b) The release that first carries the change and the one after it.
  - (c) Kept until 1.0.
  - (d) Removed by `driver-contract`, when it rewrites configuration
    loading. The date then depends on that item, and its diff grows.
- **a4. What, if anything, protects the default: `./warden.toml` in a
  working directory that is someone else's repository?** Proceeding with
  (b); confirm. Under every option but (d), `cd repo && warden build` acts
  on the repository's file: it chooses the driver, the relay image, the
  relay's trust roots, where output and work directories go, and (with
  `--driver hyperv`) the base image put in the user's cache. The protection
  is to name the file.
  - (a) Nothing more than `--config`, `WARDEN_CONFIG` and the reference
    saying so. `warden doctor` and `--verbose` say which file was read.
    The default is silent on an ordinary run.
  - (b) (a), and one line on every command that reads configuration,
    without `--verbose`, saying which file was read and how it was chosen
    (`[warden] config: /src/repo/warden.toml (found in the working
    directory)`). It stops nothing, and nobody reads it on the hundredth
    run; it does put the fact in every log, the same for a named file, with
    no special case. **Recommended.**
  - (c) (b), but only when the file was found by default. Less output for
    someone who names their file; a special case, and a log then does not
    show which file a named run used.
  - (d) Refuse given values from a file found by default (the relay image,
    CA paths, the Hyper-V base image, a directory outside the working
    directory). It does protect the unwary run. It is a two-level rule,
    which Jeff removed on 2026-11-15, and it refuses an executor's own
    `./warden.toml` until they name it (Decision 7).
- **a5. `WARDEN_UPSTREAM_CA_CERTS` when `[relay] upstream_ca_certs` is also
  set.** Today the variable's paths are added to the file's. Proceeding
  with (a); confirm.
  - (a) The variable replaces the key (GLOBAL-10), and while the
    transitional detection stands, setting both is an error that says the
    meaning changed and to put every path in one of them. Nobody gets fewer
    trust roots without being told. **Recommended.**
  - (b) The variable replaces the key at once, with no error. Someone who
    relied on the sum gets upstream TLS failures and has to work out why.
  - (c) The append stays and is named as a merge in the reference and in
    **Defaults and overrides** (GLOBAL-10 allows it). No behaviour changes;
    the environment is then a level that replaces for five variables and
    adds for one.
- **a6. How a development `warden` finds its source tree (F9).** Today:
  walking up from the executable, then from the working directory.
  Proceeding with (a); confirm.
  - (a) From the executable only. `make build` puts `dist/warden` in the
    tree, which is how CONTRIBUTING, CI and the native sessions run it, so
    nothing they do changes. `go run ./cmd/warden` and a `go install`ed
    development `warden` (whose executable is in a cache or in `GOBIN`) get
    an error that says to run `make build` and use `dist/warden`. Three
    tests in two drivers' test files rely on the working-directory step
    today and name the tree through a test-only setter instead (**F9: the
    source tree**). No new surface. **Recommended.**
  - (b) (a), plus a variable (`WARDEN_SOURCE_DIR`) that names the tree and
    is used exclusively. `go run` and `go install` keep working for someone
    who sets it. One more variable outside the reference's knobs (UX-07),
    and one more thing the environment can point at code.
  - (c) The tree the binary was compiled from (the path the compiler
    recorded). Nothing to set, and `go run` works. It fails under
    `-trimpath`, and a binary that reads its own build paths is magic
    (GLOBAL-11).
- **a7. How the queue holds this item behind `runtime-cli-project-path`'s
  merge.** Both items edit `LoadConfig` and the configuration reference,
  and this one edits UX-08 as that one leaves it. `follows` does not fit:
  `wf next` holds only a *queued* item behind the item it follows, this
  item is in progress, and the field would also re-class it as a fix of
  that item. A blocking question raised now would hold Design itself
  (phase 1 is not done with one open). Proceeding with (a); confirm.
  - (a) The design review, when it approves, raises a blocking question
    that names the merge ("resume when `runtime-cli-project-path`'s PR has
    merged"), LOOP-04's form of a wait applied to an item in this
    repository; the loop then takes none of this item's steps, and Jeff
    answers it once that PR is merged. Plan and Implement start from a
    `main` that has it. **Recommended.**
  - (b) No hold. FLOW-05 takes the items in priority order (64 before 65),
    so this one is planned and implemented while that one waits for its
    merge, and meets it in consolidation (FLOW-10), which then has real
    work in `LoadConfig`, the reference and UX-08 and may send it back to
    Implement.
- **a8. The edges of the lookup** (Decision 1). Proceeding with all five;
  confirm.
  - A named file that cannot be read is an error; nothing else is tried.
  - An empty `--config` or `WARDEN_CONFIG` is not set.
  - Reading no settings is done by naming an empty file; alternative: a
    `--no-config` flag.
  - `--config` is global and is not opened by a command that reads no
    configuration; alternative: such a command refuses it.
  - A `./warden.toml` that exists and cannot be read is an error;
    alternative: as today, treated as absent.
- **a9. What a file named as an upstream CA may hold** (Decision 8).
  Today its bytes are sent to the relay whatever they are. Proceeding with
  (a); confirm.
  - (a) Certificates only. Text outside PEM blocks (the comments of a
    distribution bundle) is allowed and not sent; a file with no
    certificate, a certificate that does not parse, or any other PEM block
    is an error naming the file and the block. A file that holds a
    certificate and its private key, which is sent whole today, is refused.
    **Recommended.**
  - (b) Send the certificates and drop everything else without an error. A
    combined file keeps working; a file that is the wrong file sends
    nothing and says nothing.
  - (c) Leave it: the file is the executor's choice.

## Design

It works only in this repository.

### How the config file is found

```
configFile():                                  # once per command that reads configuration
    --config not empty        -> that path, source "--config"
    WARDEN_CONFIG not empty   -> that path, source "WARDEN_CONFIG"
    ./warden.toml exists      -> it, source "found in the working directory"
    else                      -> none

LoadConfig():
    former user config        # transitional; see below
    path, source := configFile()
    path named and cannot be read  -> the error; nothing else is tried
    none                      -> defaults
    else                      -> decode that one file
    apply the environment variables, then (resolveConfig) the flags
```

- A relative `--config` or `WARDEN_CONFIG` is relative to the working
  directory, like any path argument. Symbolic links are followed.
- The file is decoded once, by the decoder used today. A file that is not
  TOML is an error for every command that reads it (UX-08 as proposed: "a
  config file that cannot be parsed is an error for every run that reads
  it").
- The errors, each a `driver.FixableError` (UX-04), exit 1 (UX-09):

  ```
  ERROR: config file /etc/warden/ci.toml (from --config) does not exist
    To fix: pass --config an existing file, or drop it to use WARDEN_CONFIG or ./warden.toml
  ```

  ```
  ERROR: config file ci.toml (from WARDEN_CONFIG) does not exist
    To fix: set WARDEN_CONFIG to an existing file, or unset it to use ./warden.toml
  ```

  ```
  ERROR: cannot read config file /src/repo/warden.toml (found in the working directory): permission denied
    To fix: make it readable, or name another file with --config or WARDEN_CONFIG
  ```

  A directory gets the second form with "is a directory". The syntax error
  keeps today's text, with the path of the one file.

### What says which file was read

One line, the same text wherever it appears:

```
config: /etc/warden/ci.toml (--config)
config: /home/ci/warden.toml (WARDEN_CONFIG)
config: /src/repo/warden.toml (found in the working directory)
config: none (no --config, no WARDEN_CONFIG, no ./warden.toml): defaults apply
```

The path is absolute.

- `warden doctor` prints it as the first line of its report, on stdout.
- `build`, `shell`, `clean`, `setup` and `image prepare` print it on
  stderr with the `[warden]` prefix, before anything else, without
  `--verbose` (a4). Under a4 (a) they would print it only with `--verbose`
  (or `WARDEN_VERBOSE`, or `[output] verbose`).
- `warden doctor`'s source label for the build link and the resolvers
  (`build link 100.126.255.252/30 (warden.toml)`) stays the base name of
  the file that set the key, as today; with one file it is that file's
  name, and the `config:` line gives its path. The label `config.toml`
  appears only when the named file is called that.

### The user-level file

What stops: `~/.config/warden/config.toml` is not read, on any OS (it is
`%USERPROFILE%\.config\warden\config.toml` on Windows), and nothing is
merged under `./warden.toml`.

The transitional error, on `build`, `shell`, `clean`, `doctor`, `setup` and
`image prepare`, once the config file is chosen and before it is read:

```
ERROR: /home/you/.config/warden/config.toml is no longer read: warden reads one config file, ./warden.toml or the one --config or WARDEN_CONFIG names
  To fix: to keep using this file, set WARDEN_CONFIG=/home/you/.config/warden/config.toml (it is then the only config file read: ./warden.toml is not); or move its settings into the warden.toml you build with, or into the environment (WARDEN_DRIVER, WARDEN_CTR_CLI, WARDEN_IMAGE, WARDEN_UPSTREAM_CA_CERTS), and delete it
```

- The path is the one `LoadConfig` reads today: `os.UserHomeDir()` joined
  with `.config/warden/config.toml`. Under `sudo` that is the home `sudo`
  gives the process, as today. With no home directory there is nothing to
  look for.
- "Exists" is `os.Stat` succeeding; the file is not opened.
- A named config file that cannot be read is reported first: it is what
  the user just asked for.
- It does not fire when `--config` or `WARDEN_CONFIG` names that same file
  (`os.SameFile`): the first way out in the message is one line in a shell
  profile, with no file moved.
- `warden doctor` exits 1 with it, like every other command: doctor
  reports what a build would get.
- The commands that read no configuration do not look.

Where it lives, so that it is deleted in one place, as
`runtime-cli-project-path` does for its own:

- One file, `cmd/warden/config_former.go`, with two functions: one that
  returns the error above or nothing, and one that returns the a5 error for
  the variable and the key set together, or nothing.
- Two calls in `LoadConfig`, each a single line.
- One test file, `config_former_test.go`.
- One subsection of the configuration reference ("Moving from
  `~/.config/warden/config.toml`").
- One marker on each: `TRANSITIONAL(one-config-file)`, in the file's header
  comment (which says why the code exists and when it goes, GLOBAL-01), at
  the two calls, at the head of the test file, and in an HTML comment above
  the docs subsection. A search for the marker finds everything to delete.
- When: a3. Publish files the removal as a queue item whose `source` names
  the marker and the condition.

### Who is affected, and what they do instead

| Today | After |
|---|---|
| Settings in `~/.config/warden/config.toml`, no `./warden.toml` | The error. `WARDEN_CONFIG=<that file>` keeps everything as it was |
| Settings in both files, merged | The error. One file per build: combine them, or name the user file and stop using the project's |
| Only `./warden.toml` | Nothing changes, but for the `config:` line |
| No file | Nothing changes, but for the `config:` line |
| `WARDEN_UPSTREAM_CA_CERTS` and `[relay] upstream_ca_certs` both set | The a5 error during the window; afterwards the variable replaces the key |
| A CA file that also holds a private key, or is not a certificate file | Refused, naming the file (a9) |
| `relay_image` or `WARDEN_IMAGE` starting with `-` or holding a space | Refused |
| A development `warden` run from outside its source tree, inside the tree | An error naming `make build` and `dist/warden` (a6) |

What can be set without a file, today and after (a2 adds none):

| Key | Flag | Variable |
|---|---|---|
| `[runtime] driver` | `--driver` | `WARDEN_DRIVER` |
| `[runtime] cli` | `--runtime` | `WARDEN_CTR_CLI` |
| `[runtime] relay_image` | none | `WARDEN_IMAGE` |
| `[relay] upstream_ca_certs` | none | `WARDEN_UPSTREAM_CA_CERTS` |
| `[relay] system_ca_bundle` | none | none |
| `[build] output_dir` | `-o` | none |
| `[build] capture` | `--capture` | none |
| `[build] compress` | `--no-compress` (off only) | none |
| `[build] tmpdir` | `--tmpdir` | none (the OS temp dir follows `TMPDIR`) |
| `[vm] cpus`, `memory_mb` | `--cpus`, `--memory` | none |
| `[vm] network` | `--network` | none |
| `[vm] build_link` | `--build-link` | none |
| `[vm] dns` | none | none |
| `[vm] image_refresh_after`, `image_min_grace` | none | none |
| `[vm] windows_image` | `--source` on `image prepare` | none |
| `[vm] windows_image_update` | none | none |
| `[output] color` | `--color` | `NO_COLOR` (off only) |
| `[output] verbose` | `-v` (on only) | `WARDEN_VERBOSE` (on only) |

So a setting kept per machine that has neither a flag nor a variable
(`[vm] dns`, `system_ca_bundle`, the image ages, `windows_image_update`,
and `relay_image` and CA paths for someone who prefers a file) goes in the
file the machine names with `WARDEN_CONFIG`.

### The precedence, key by key

The order is flag, then variable, then the file, then the default, read
from `LoadConfig`, `resolveConfig` and each `resolve*` function at
`ad4ea50`. "Follows" means: where a level exists for the key, it replaces
the levels below.

| Key | As the code has it | Follows |
|---|---|---|
| `[runtime] driver` | `--driver`, `WARDEN_DRIVER`, file, `container` | yes |
| `[runtime] cli` | `--runtime`, `WARDEN_CTR_CLI`, file, detection | yes |
| `[runtime] relay_image` | `WARDEN_IMAGE`, file, the release image or a source build | yes |
| `[relay] upstream_ca_certs` | the file's list **plus** `WARDEN_UPSTREAM_CA_CERTS`'s, split on `:` | **no**: a merge. Changed here (a5), and the split with it |
| `[relay] system_ca_bundle` | file, `true` | yes. Honoured only together with CA paths: `false` alone does nothing, silently. Handed on |
| `[build] output_dir` | `-o`, file, `warden-output` | yes |
| `[build] capture` | `--capture`, file, none | yes |
| `[build] compress` | off when the file says `false`, else on unless `--no-compress` | as far as the flag can say: it can only turn compression off, so `--no-compress=false` does not undo the file's `false`. Handed on |
| `[build] tmpdir` | `--tmpdir`, file, the OS temp dir | yes |
| `[build] dockerfile`, `context` | never read | F5, `configuration-bugfixes` |
| `[vm] cpus`, `memory_mb` | flag, file, the driver's default; `0` is "not set" at both | yes |
| `[vm] network` | `--network`, file, `warden-dev` (hyperv) | yes |
| `[vm] build_link` | `--build-link`, file, the default | yes. The file's value is checked as it loads even when the flag replaces it: one of the checks UX-08 as proposed lists by name |
| `[vm] dns` | file, the host's resolvers | yes |
| `[vm] image_refresh_after`, `image_min_grace` | file, the driver's default | yes |
| `[vm] windows_image` | `--source`, file, the registry's default | yes |
| `[vm] windows_image_update` | file, `auto` | yes |
| `[output] color` | `--color`, `NO_COLOR`, file, `auto` | yes; `NO_COLOR` can only say `never` |
| `[output] verbose` | on when any of `-v`, `WARDEN_VERBOSE`, the file says so | as far as the flag and the variable can say: neither can turn it off, and `WARDEN_VERBOSE=0` turns it on. Handed on |

Before this item the same table has one more column, the user's file under
the project's; every key followed it there too, the two files being decoded
in order into one struct.

### F1 to F4, one by one

For each: what it is now, what is a defect whoever owns the file, and
where that is fixed.

- **F1, the relay image.** With the executor's own file, choosing the
  relay is the executor's business: it is how a release is pinned and how a
  development build is asked for. With the default file in someone else's
  repository, the repository chooses the component that measures it (what
  remains, in the Security note).
  - *A defect regardless:* the value is passed to `<runtime> pull <value>`
    and to the container's creation as it is, so one that starts with `-`
    is read by the runtime as an option. **Fixed here.** `newRequest`
    checks the relay image in effect (the variable, else the file) when the
    container driver is selected, the only driver that reads it: empty and
    `dev` are as today; anything else must not start with `-` and must hold
    no space or control character. It does not validate a reference
    further: the runtime's own error does that.

    ```
    ERROR: [runtime] relay_image in /src/repo/warden.toml is "--volume=/:/host": not an image reference
      To fix: set it to an image reference such as ghcr.io/buildwarden/relay:v0.2.0 or to "dev", or remove it
    ```

    (`WARDEN_IMAGE is ...` when the variable set it.) The check is on the
    value in effect, after the overrides, and only when the run uses it
    (UX-08 as proposed). `driver-contract` moves it with the setting.
  - *A defect regardless:* `relay_image = "dev"` builds from a tree that
    can be the working directory's. That is F9, fixed here.
- **F2, the trust roots.** With the executor's own file, naming extra
  roots is what the key is for.
  - *A defect regardless:* the files are read when the relay is claimed,
    after the output directory, the work directory and the relay exist, so
    a mistyped path fails deep into the build (UX-08). **Fixed here:**
    `newRequest` reads and checks them once, for every driver, before
    anything is created.
  - *A defect regardless:* the bytes of each file are sent to the relay
    whatever they are. A path that names the wrong file sends that file,
    and a bundle with a private key in it sends the key. **Fixed here**
    (a9): `driver.LoadUpstreamCAs` sends the certificates and refuses
    everything else.

    ```
    ERROR: upstream CA file /etc/ssl/private/site.pem holds a PRIVATE KEY block; only certificates are sent to the relay
      To fix: name a file that holds only PEM certificates, in [relay] upstream_ca_certs or WARDEN_UPSTREAM_CA_CERTS
    ```

    This also takes away the worst of what remains: with the default file
    in someone else's repository, F1 and F2 together could send any file
    the user can read to a relay the repository chose. After this only
    certificates leave.
  - *A defect regardless:* the variable is appended and split on `:`.
    **Fixed here** (Decisions 4 and 8).
  - *Handed on:* `system_ca_bundle = false` with no CA paths does nothing
    and says nothing (GLOBAL-10): `configuration-bugfixes`.
- **F3, the directories.** With the executor's own file, where output and
  work directories go is theirs to say, as `-o` and `--tmpdir` are. Nothing
  here is a defect of the file being read.
  - *Not fixed, and not a config-file matter:* a work directory placed
    inside the build context (`tmpdir = "."`, or `--tmpdir .`) is inside
    what the collector serves to the build (`/v1/context/`, the whole
    context root). What a driver's work directory holds that a build should
    not read was not traced (a relay VM's seed carries the claim token,
    `tools/relay-vm/README.md`; whether it is kept in the work directory
    was not checked). It is listed for intake as its own security finding (**What
    this item does not do**), because the flag does the same and the answer
    is in the drivers.
- **F4, the Hyper-V base image.** With the executor's own file, this is how
  an operator pins a base ("an explicit URL is the operator's choice",
  `driver/hyperv/evalimage.go`). With the default file in someone else's
  repository it is the one effect that outlasts the run: `warden image
  prepare --driver hyperv`, or a hyperv build whose base is due for refresh
  (the file sets the ages too), fetches the repository's image into the
  user's cache, where later builds of every project boot it (what remains).
  - *A defect regardless, new with `--config`:* the background prepare
    would read a different file than its build. **Fixed here** (Decision
    10).
  - *Handed on:* a value that is neither a URL nor an existing file is
    sent to the HTTP client, whose error does not say that the path was not
    found (UX-04): `configuration-bugfixes`. Read: `cmd/warden/image.go`,
    and `FetchEvalImage`, `registerLocalImage`, `isLocalImage` and
    `resolveRemote` in `driver/hyperv/evalimage.go`; how the path reaches
    PowerShell afterwards was not traced.
- **The whole file** (the driver and the runtime outranking the user's
  file). There is no user's file to outrank. The executor's standing
  choices are in the environment (`WARDEN_DRIVER`, `WARDEN_CTR_CLI`), which
  is over the file, or in the file they name.

### F9: the source tree

`driver.ModuleRoot` (`driver/hostutil.go`) is used by `GuestBinary` (the
relay and `warden-io` cross-built for a guest), by the container driver's
`goBuildLinux` (the relay image, `warden-io` and the holder of a development
build) and by `relayvm.Find` (the relay VM's kernel and initramfs for qemu
and Firecracker, when they are not beside `warden`). Today it walks up from
the executable's directory and then from the working directory, taking the
first `go.mod` that *contains* the module's declaration.

After:

```
ModuleRoot():
    walk up from the directory of the warden executable
        a go.mod with the line "module github.com/buildwarden/buildwarden" -> that directory
    else -> error
```

- The working directory is not looked at. A repository that declares the
  module is not `warden`'s source unless `warden` was built into it.
- The module line is matched whole. Today a nested module of this
  repository (`tools/tier1`) or a sibling whose path starts the same would
  match.
- When it is not found, the callers' errors gain the fix, as a
  `FixableError` where the source tree is the only way left (a development
  build's relay, `warden-io` and holder):

  ```
  ERROR: this is a development build of warden and its source tree was not found above /home/you/go/bin
    To fix: build it in its tree with 'make build' and run dist/warden, or install a release
  ```

  `relayvm.Find` already treats "no tree" as "look beside the binary only";
  that stays.
- A release `warden` is affected only where it reaches `ModuleRoot` at
  all: with no `warden-io` or guest binary beside it. It then reports the
  missing file as today, and no longer builds one from the working
  directory.
- Tests: a `go test` binary lives in a temporary directory, so a test that
  reaches `ModuleRoot` relies on the working-directory step today. Run on a
  copy of `ad4ea50` with that step deleted, `go test ./...` fails in three
  tests and passes everywhere else: `TestStageSeedDir` and
  `TestStageSeedDirConfiguredLink` (`driver/hyperv/winseed_test.go`) and
  `TestWindowsSeedDirRunScript` (`driver/qemu/windows_test.go`), each of
  which cross-builds `warden-io` through `GuestBinary`. They are in other
  packages than `driver`, so `driver` gains one documented, test-only
  setter of the tree, which those two test files call with the tree found
  from the test's own directory. Those are the only edits in a driver's
  tree, both to test files that CI runs on Linux. No environment variable
  is added (a6).

### The background image prepare

`driver.StartDetached(logPath, args...)` gains the config file: `cmd/warden`
records the absolute path of the file it read in a package variable of
`driver`, as it records `driver.WardenVersion` today, and `StartDetached`
puts `--config <that path>` in front of `args` when it is set. With no file
read, nothing is added; the child then finds none either, unless one has
appeared in the working directory since. The child's first log line is its
`config:` line. The two callers (`driver/qemu`, `driver/hyperv`) are not
edited.

### Reference docs (UX-01) and the overview (FLOW-15)

- `docs/book/src/guide/configuration.md`:
  - the opening ("two places, merged in order") becomes **The config
    file**: one file, the three ways it is found, a named file that is
    missing, an empty file, the `config:` line, relative paths (a1), and a
    paragraph for someone building a repository they do not control (name
    the file; what a `./warden.toml` there decides);
  - the transitional subsection "Moving from
    `~/.config/warden/config.toml`", under its marker;
  - **Environment Variables**: `WARDEN_CONFIG`; `WARDEN_UPSTREAM_CA_CERTS`
    "replaces `relay.upstream_ca_certs`; paths separated by `:` (`;` on
    Windows)"; one sentence on the order (flag, variable, file, default);
  - **CLI Flags**: `--config`, Global;
  - the `[relay]` lines of the Full Reference (certificates only), the
    Relay Image section (what is refused);
  - the two sentences that name both files: the build link's "Precedence:
    the flag, then `./warden.toml`, then `~/.config/warden/config.toml`,
    then the default" and `[vm] dns`'s "`./warden.toml` wins over
    `~/.config/warden/config.toml`".
- `docs/book/src/guide/cli.md`: `--config <path>` under Global Flags;
  `warden doctor` (the `config:` line; the source label is the config
  file's name).
- `docs/book/src/guide/corporate-networks.md`: its three mentions of
  `~/.config/warden/config.toml` and "the two files" become the config
  file, with the `WARDEN_CONFIG` route for a machine-wide setting.
- `README.md`: the Configuration block's first comment (`# ./warden.toml,
  or the file --config or WARDEN_CONFIG names`) and its list of variables.
- `CONTRIBUTING.md`: one sentence, that a development `warden` is run from
  its tree (`dist/warden`), if a6 (a) stands.
- `cmd/warden`: the help text of `--config`; `doctor`'s long help where it
  lists the build link's sources.
- `docs/book/src/concepts/overview.md`, in `warden build`, in place of the
  `req := resolve flags and config` line:

  ```
      cfg := the one config file: --config, else WARDEN_CONFIG, else
             ./warden.toml if it exists, else none; a named file that
             cannot be read is an error
      req := resolve each setting: flag, else its variable, else cfg,
             else the default (driver, runtime, output dir, ...)
  ```

  `runtime-cli-project-path` adds a `container driver: runtime := ...` line
  there that says "(`./warden.toml`, then the user's file)"; it becomes
  "(the config file)".
- `install.sh` and `docs/book/src/getting-started/installation.md` name no
  config file and do not change.

### Compatibility (UX-03)

No flag, subcommand or key is removed or renamed, so UX-03's alias does not
apply as written and tier 1's `surface-removed` check sees nothing. Three
things that work today stop: the user-level file, the sum of the variable
and the key for CA paths, and a CA file with something other than
certificates in it. UX-03's "never a silent break" is met for each by an
error that says what to change, at once and not after a warning period, as
Jeff decided for `runtime-cli-project-path`. The first two errors are
transitional; the third is the permanent check's own message.

### How it sits with the other items

The order of merging: `runtime-cli-project-path`, then this item, then
`driver-contract` (K1).

**`runtime-cli-project-path` first.** It is small and in review, and it
closes host execution through `[runtime] cli` without waiting for this.
What each owes the other:

- This item keeps its rule and its code: the runtime is `finch`, `docker`
  or `podman` from every source, checked in `resolveRuntime`; its
  transitional file `runtime_former.go` and marker stay untouched.
- This item reworks what that one wrote for two files: "else the project's
  file, else the user's" in `resolveRuntime`'s comment and the reference
  becomes "else the config file"; an error that names the file a value came
  from names the one file; its overview line (above); its tests' temporary
  `HOME` stays, now so that no test meets the transitional error.
- UX-08: that item amends it; this item changes one clause of what it
  leaves (**Rule changes**). If its wording merges without the plural,
  this item's entry edits nothing.
- The hold: a7.

**`driver-contract` (K1) after.** K1 is written on one file and needs this
item merged before its Implement. What each owes the other:

- This item gives K1 one function that says which file was read and how
  (`configFile`), the `--config` flag and `WARDEN_CONFIG`, and no level.
- K1's `config_files` then has at most one entry and no `level`; its
  requirements on reading only the files named (C3), on precedence (C8) and
  on what a build record keeps follow. K1's design says which.
- K1's ORCH-03 paragraph on drivers is added to ORCH-03 as this item
  leaves it, and its "the order of the files does not change" goes.
- K1's rule that a project's file may name a driver but not give a path
  (its a16) rested on the two levels. With one file in the executor's
  control K1 decides it again; this item leaves `[runtime] driver` as the
  five names it is today.
- K1 moves `relay_image` and its check to the container driver; the CA
  check stays with the claim, which K1 gives to `warden`; the transitional
  file and the `config:` line are `warden`'s own and stay in `cmd/warden`.
- This item's tier-1 cases run the built `warden` and must pass on K1's
  branch unchanged.
- K1's hold is K1's to record, in the same form as a7: a blocking question
  that names this item's merge, raised when its Plan is done.

**`configuration-bugfixes`** (on hold behind the freeze) is handed four
more observations (**What this item does not do**).

### What this item does not do

- It does not limit what a config file may set by where the file was
  found (Decision 7).
- It does not record in the build's output which config file was used.
  `driver-contract`'s build record does.
- It does not change `[runtime] driver`, or what `runtime-cli-project-path`
  decides for `[runtime] cli`.
- It does not move the load-time checks of `[vm] build_link` and `[vm]
  dns` to their consumers; UX-08 as proposed lists them, and they are K1's
  or `configuration-bugfixes`'.
- Handed to `configuration-bugfixes`, none a security matter: `[relay]
  system_ca_bundle = false` alone does nothing, silently; `--no-compress`
  and `-v` cannot undo the file (`--no-compress=false`, `--verbose=false`),
  and `WARDEN_VERBOSE=0` turns verbose on; a mistyped local `[vm]
  windows_image` gives an HTTP error; the hyperv and qemu drivers read
  variables that are in no reference (`WARDEN_HYPERV_BUILD_IMAGE`,
  `WARDEN_RELAY_BOOT_VHDX`, `WARDEN_QEMU_VNC` and others), which UX-07
  calls ad hoc before and after this item.
- Listed for intake as a security finding of its own (not a regression, so
  FLOW-09 does not file it; a security fix, so FLOW-20 would not hold it):
  **a work directory inside the build context is served to the build.**
  `--tmpdir` or `[build] tmpdir` can place each build's work directory
  under the context directory, and the collector serves the whole context
  root to the build. What each driver keeps in its work directory was not
  traced. Proposed slug: `workdir-in-context`.
- Not traced: whether a `FROM` image in a Dockerfile that starts with `-`
  reaches the runtime's command line as the relay image did. It is a build
  input, not configuration; it belongs with the finding above or with the
  translator.

## Defaults and overrides

"Absent" below means: no flag, the variable unset or empty, and the key not
in the config file (or no config file).

| Default, override or fallback | Status | Why it is the obvious choice (GLOBAL-09) | What overrides it, exclusively (GLOBAL-10) |
|---|---|---|---|
| The config file when none is named: `./warden.toml`, if it exists | **Changed**: it was that file over `~/.config/warden/config.toml` | Jeff's decision 2: the working directory, for ease of use | `--config`, else `WARDEN_CONFIG`; the named file is the only one read, and a named file that cannot be read is an error, not a return to the default |
| No `./warden.toml` and none named: no config file | **Changed**: the user file was read | A first run needs no file (UX-07) | As above |
| `~/.config/warden/config.toml` | **Removed** as a source. Its existence is an error while the transitional detection stands (a3), unless it is the file named | Jeff's decisions 1 and 4 | Nothing reads it; `WARDEN_CONFIG` can name it |
| An empty `--config` or `WARDEN_CONFIG` | **New**: not set | What an empty `--driver`, `--runtime` and `WARDEN_CTR_CLI` mean today | Nothing (a8) |
| A relative path in the config file: relative to the working directory | **Unchanged**, stated for the first time (a1) | The same as the path given by a flag or a variable | An absolute path |
| Every setting: flag, else its variable, else the file, else its default | **Unchanged** in the code; the rules now name the variable's level | The closer to the command, the stronger | Each level by the one above it |
| `[relay] upstream_ca_certs` absent: the system roots only | Unchanged | | `WARDEN_UPSTREAM_CA_CERTS`, else the key |
| `WARDEN_UPSTREAM_CA_CERTS` with the key also set | **Changed**: it replaced nothing and was added to the key's list; now it replaces it (a5), and both set is an error during the window | One rule for every variable | Nothing is over the variable; the key has no flag |
| The separator of `WARDEN_UPSTREAM_CA_CERTS` | **Changed**: `:` everywhere; now the OS's (`;` on Windows) | What `PATH` uses on each OS (GLOBAL-07) | Nothing |
| The relay image absent: the release image, or a source build for a development `warden` | Unchanged | | `WARDEN_IMAGE`, else `relay_image` |
| The source tree of a development `warden`: found by walking up from its executable | **Changed**: the working directory was tried second (a6) | A binary's source is where the binary was built | Nothing |
| The `config:` line: printed by every command that reads configuration | **New** (a4) | Which file is in force should not need a flag to learn | Nothing; `doctor` prints it on stdout |
| A background `image prepare`: the file its build read | **Changed**: it looked in the working directory by itself | One build, one file | Nothing |

**Fallbacks and merges.** None is added, and two are removed:

- the merge of the user's file under the project's (two files decoded into
  one struct) is gone;
- the merge of `WARDEN_UPSTREAM_CA_CERTS` into the key's list is gone;
- `ModuleRoot`'s second place to look (the working directory, tried when
  the first found nothing) is gone, and the design drops it as a rejected
  alternative (Decision 9).

The lookup of the config file is an ordered choice among what is *set*,
not a fallback on failure: a source that is set and cannot be used is an
error. The transitional detection is not a fallback: it only produces
errors.

No relay setting is added or moved (RELAY-08). The upstream trust roots
keep their claim field, `upstream_ca_certs`, and `upstream_system_ca`; the
relay image is the container driver's launch argument, as today.

## Cross-driver impact

No file of a `driver/<name>/` tree that is built into `warden` is edited
(ORCH-01); two test files are, in `driver/hyperv` and `driver/qemu`, to name
the source tree (**F9: the source tree**). Three shared files of `driver/`
are edited, so every driver is declared:

- **container**: the relay image in effect is checked before anything is
  created. A development build finds its source from the executable only
  (`goBuildLinux`). CA files are checked early and sent as certificates.
- **qemu, firecracker**: the relay VM's kernel and initramfs, when not
  beside `warden`, come from the tree the executable is in and no longer
  from the working directory's (`relayvm.Find`); the guest binaries
  likewise (`GuestBinary`). CA files as above. qemu's background Windows
  prepare is started with the build's config file.
- **vz**: the guest binaries likewise; CA files as above.
- **hyperv**: the guest binaries likewise; CA files as above; the
  background base prepare is started with the build's config file.

For every driver the config file is found the new way, by `cmd/warden`,
before the driver is constructed. A build whose settings come out the same
runs exactly as before.

## Rule changes

Three entries. ORCH-03, UX-07 and UX-08 are project invariants of the
component this item works in (`cmd/warden`), not loop rules (FLOW-14,
LOOP-09). The current wording is quoted from `docs/development/` on `main`
at `ad4ea50`.

### ORCH-03, amend

Path: `docs/development/orchestrator.md`.

**Current wording** (the rule's body):

> Configuration precedence stays `flag > warden.toml > ~/.config/warden`. Defaults
> are sane and changes are additive. A new or changed config key is documented in the
> same change; this is the config half of the docs-coupling rule (UX-01).
>
> The automated part is a config-key diff against a docs diff.

**Proposed wording:**

> `warden` reads one config file: the one `--config` names, else the one
> `WARDEN_CONFIG` names, else `warden.toml` in the working directory when it
> exists. It reads no other file and merges none; a named file that cannot be
> read is an error. Configuration precedence stays `flag > environment variable >
> the config file > default`. Defaults are sane and changes are additive. A new
> or changed config key is documented in the same change; this is the config half
> of the docs-coupling rule (UX-01).
>
> The automated part is a config-key diff against a docs diff.

Two things change: the first two sentences are new, and the precedence
names the environment and the default and drops `~/.config/warden`. "Stays"
is kept: the order with the environment in it is the order the code has
applied all along.

### UX-07, amend

Path: `docs/development/ux-cli-config.md`. The title stays ("Minimal
config, predictable defaults, layered overrides").

**Current wording** (the rule's body):

> Configuration can be minimal: predictable defaults cover the common use-cases, and
> any of them is easy to override at three levels with clear precedence,
> invocation > project > user (`flag > warden.toml > ~/.config/warden`, the same order
> as ORCH-03). A first-time user should get a sensible run with little or no config.
> Driver and build knobs are flags and config keys, not ad-hoc environment variables,
> so they take part in that precedence and appear in the reference docs. A value set
> at any level replaces the default and the lower levels' value (GLOBAL-10).
>
> The automated part checks that defaults resolve with no config file present.

**Proposed wording:**

> Configuration can be minimal: predictable defaults cover the common use-cases, and
> any of them is easy to override at three levels with clear precedence,
> command line > environment > config file (`flag > environment variable >
> warden.toml`, the same order as ORCH-03). There is one config file for a run, in
> the control of whoever runs the build: `warden.toml` in the working directory by
> default, or the file `--config` or `WARDEN_CONFIG` names, which is then the only
> one read. No user-level or system-level file is read. A first-time user should get
> a sensible run with little or no config. Driver and build knobs are flags and
> config keys, and a knob's environment variable is a documented one, not an ad-hoc
> one, so they take part in that precedence and appear in the reference docs. A
> value set at any level replaces the default and the lower levels' value
> (GLOBAL-10).
>
> The automated part checks that defaults resolve with no config file present.

Three things change: the three levels are the command line, the environment
and the config file (they were invocation, project and user); the two
sentences on the one file are new; and the sentence on knobs admits a
documented variable, because the environment is now a level of the rule and
six knobs have had one all along.

### UX-08, amend (one clause, on top of `runtime-cli-project-path`'s)

Path: `docs/development/ux-cli-config.md`.

**Current wording on `main` at `ad4ea50`:**

> Configuration errors or mismatches are validated and surfaced as early as possible,
> at load/parse/startup, before any real work begins, rather than failing deep into a
> build. The automated part checks that config validation runs at startup.

This item changes nothing in that text. `runtime-cli-project-path` proposes
to replace it (its design at `3bf8784`, **Rule changes**), and merges
first. Its first paragraph, as proposed:

> A component validates the configuration it consumes, errors and mismatches
> alike, when it initializes: after the overrides are applied (flags, environment
> variables, and the config files in precedence order, UX-07) and before any real
> work begins, rather than failing deep into a build. What it checks is the value
> in effect, not a value that a higher level replaced.

**Proposed wording**, of the parenthesis in that paragraph only:

> (flags, environment variables and the config file, in UX-07's order)

The rest of UX-08 as that item leaves it holds for one file as written
("One `warden.toml` may hold the settings of several drivers"; "A config
file that cannot be parsed is an error for every run that reads it";
"Checks that predate this wording still run as a config file loads"). If
its wording has merged without "the config files", this entry edits nothing
and the review's approval of it is unused. If that item's amendment is not
approved at all, this entry falls with it: nothing in `main`'s UX-08 names
a file.

### Severity and check, before and after

ORCH-03 and UX-07: MUST, `automated + agent-judgment`, unchanged. UX-08:
whatever `runtime-cli-project-path` leaves (it proposes MUST,
`agent-judgment`); this item changes neither.

### What the enforcement change does

Nothing changes in the enforcement, and no file under `tools/tier1/`,
`tools/workflow/check/` or `.github/workflows/` is edited.

- ORCH-03's automated part is tier 1's `surface-docs` check
  (`tools/tier1/surface/surface.go`): it reads the cobra flag names and the
  `toml` struct tags of `cmd/warden` at the merge base and at the head, and
  fails when one is added or removed and neither `cli.md` nor
  `configuration.md` changed. This item adds one flag, `config`, and the
  reference changes with it, so the check passes as it stands. It does not
  read environment variables or how a file is found, before or after.
- No key is removed, so `surface-removed` (UX-03) reports nothing.
- UX-07's automated part is not a static check: `rules.yaml` records it as
  `follow-up: "a test, not a static check: defaults with no config"`. The
  entry stays. This item's tier-0 tests include that case
  (`TestLoadConfig_Defaults`, extended: no file named, none in the working
  directory, no variable).

### Does `main` conform on the day this merges?

For what the amendments say, yes, because this item's code is what makes
them true (the umbrella's Decision 18): one file is read; `--config` and
`WARDEN_CONFIG` exist; a named file that cannot be read is an error; the
order is flag, variable, file, default for every key, the CA variable
included.

Three things on `main` sit badly with UX-07 before this item and equally
after it, and are not made worse: the two one-way flags and `WARDEN_VERBOSE`
(a level that can turn a setting one way only), and the drivers'
undocumented variables ("ad-hoc environment variables"). Both are handed to
`configuration-bugfixes`.

### How the items' rule changes sit together

| Item | Rule | File | Merges |
|---|---|---|---|
| `runtime-cli-project-path` | UX-08 (rewritten), and its line in `tools/tier1/rules.yaml` | `ux-cli-config.md` | first |
| this item | ORCH-03; UX-07; one clause of UX-08 | `orchestrator.md`, `ux-cli-config.md` | second |
| `driver-contract` | ORCH-03 (a paragraph on drivers, and the check's reach), GLOBAL-08, ORCH-01, ORCH-02 and others | `orchestrator.md` and others | third |

UX-07 and UX-08 are neighbouring sections of one file; the two items edit
different paragraphs. K1's ORCH-03 text is added after the paragraph this
item rewrites.

**Not changed: UX-03.** Its text is about flags, subcommands and keys, and
none is removed. Jeff's decision that a change which refuses what worked
before is an error at once, with a transitional message, is applied here as
it is in `runtime-cli-project-path`, which does not propose to write it
into UX-03 either. If it is to be a rule, it is that rule's, and a question
for the review of whichever item is asked first.

**The profile (PROFILE-01).** No entry goes stale: its Reference documents
row points at the `surface` entry of `rules.yaml`, which does not change.

The design review approves each of the three by name (FLOW-19).

## Security note

**Boundary: the host OS and the build environment** (GLOBAL-06,
`host-env`). The build context is the build environment's side: it is what
the isolated build runs. `warden` reads files of it on the host before any
isolation exists: `warden.toml`, when it is found there by default, and,
through `ModuleRoot`, a `go.mod`. The change also reaches the boundary
between the build environment and the relay: the config file chooses the
relay image and the relay's upstream trust roots.

**The threat.** A repository that a user only meant to build, acting on the
host or on the measurement through what `warden` reads from the working
directory. In SEC-01's terms: build environment to host, class 1 (F9: the
host's `go build` runs on the repository's tree, and its `cmd/relay` and
`cmd/warden-io` become the relay and the executor; F2 with F1: a host file
leaves the host); build environment to relay, class 2 (F1: the measured
supplies the measurer). As with `[runtime] cli`, the attacker escapes
nothing: it happens before the environment exists.

**How the posture changes.**

- *Tightens:* a development `warden` no longer takes the working directory
  for its source tree (F9, closed for every driver). A value that would be
  read as an option of the container runtime is refused (F1). Only
  certificates are sent to the relay as trust roots, and they are checked
  before anything is created (F2). An executor can now make `warden`
  ignore the working directory's `warden.toml` altogether, with `--config`
  or `WARDEN_CONFIG`; before this there was no way to.
- *Changes shape:* the user-level file is gone, so nothing "outranks the
  user's file". The environment is the level over the file.
- *Loosens, for one kind of user:* someone who kept `driver = "qemu"` (or
  a relay image, or CA settings) in `~/.config/warden/config.toml` and
  builds repositories without a `warden.toml` was covered by that file.
  After this they are covered only once they act on the transitional
  error: `WARDEN_CONFIG` naming the file (which then also shuts out the
  repository's file, more than they had before), or `WARDEN_DRIVER`. The
  error does not let the build run until they have.

**What remains**, each stated in the reference:

1. **The default.** `cd repo && warden build`, with no file named, acts on
   the repository's `warden.toml`. That file chooses the driver (`driver =
   "container"` is the one that isolates least; `--driver` or
   `WARDEN_DRIVER` pins it), the container runtime among the three, the
   relay image (so the repository can supply the relay that measures it;
   `WARDEN_IMAGE` pins it), the relay's trust roots (certificates only),
   whether payloads are captured, and where the output and work directories
   are created. The `config:` line says the file was read; it does not stop
   it (a4).
2. **The cache.** With `--driver hyperv`, or `driver = "hyperv"` in the
   repository's file, `warden image prepare`, or a build whose base is due
   for refresh, fetches `[vm] windows_image` into the user's cache, and
   later builds of every project boot it. This outlasts the run, and it is
   the strongest reason to name the file on a machine that builds
   repositories it does not control.
3. **The environment is trusted.** `WARDEN_CONFIG`, like `WARDEN_CTR_CLI`
   and `WARDEN_IMAGE`, is the executor's. A tool that sets variables from a
   repository's files (a shell hook, a CI step that sources a file) hands
   the repository the same control.
4. **A relative path in a named file** is looked up in the working
   directory (a1 (a)), so a relative CA path in an executor's own file
   reads certificates from the repository being built.
5. **A work directory inside the build context** (the finding listed for
   intake).
6. **A development `warden` built in a tree is as trusted as that tree.**
   Unchanged: running `dist/warden` from a checkout builds from that
   checkout.

**What the TEST-08 suite should exercise.** Arm A gains permanent entries
(SEC-03), the attacker supplying the build context and the proof being
SEC-04's marker:

- (i) with `WARDEN_CONFIG` naming an empty file, and again with `--config`,
  a context `warden.toml` that sets `output_dir` to a marker directory and
  `relay_image` to an image that does not exist: the build uses the
  defaults, the marker directory is absent;
- (ii) a context that declares `module github.com/buildwarden/buildwarden`
  with its own `cmd/relay` and `cmd/warden-io`, built by a development
  `warden` copied outside its tree, with a `go` first on `PATH` that writes
  a marker: `warden` exits 1 and the marker is absent;
- (iii) `relay_image` starting with `-`, from the file and from
  `WARDEN_IMAGE`: exit 1, and a fake runtime on `PATH` records no `pull`;
- (iv) `upstream_ca_certs` naming a file that holds a known token and no
  certificate: exit 1 before any container exists, and the token is in no
  claim.

As `runtime-cli-project-path` notes, the suite's matrix places the attacker
inside the build environment; these come from files the host reads before
it exists, and the suite should count that surface.

The merge acknowledges this note (FLOW-08).

## Validation plan

**Tier 0** (`make test`; the `ci` workflow on Ubuntu, and any platform
locally). In `cmd/warden`, table-driven, each test with `HOME` and
`USERPROFILE` set to a temporary directory and the working directory a
temporary one:

- *The lookup:* the flag over the variable over `./warden.toml` over none;
  with a file named, a `./warden.toml` that is not TOML is not an error
  (it is not opened); a named file that does not exist, is a directory, or
  cannot be read is the error, with its source in the message, and the
  `./warden.toml` beside it is not used; an empty flag and an empty
  variable fall through; an empty file gives the defaults; a relative
  `--config`.
- *The `config:` line:* its four forms; on stdout for `doctor`, on stderr
  for the others.
- *The transitional error:* with the user file present, each of the six
  commands' config load fails with it, naming the path; with `--config` or
  `WARDEN_CONFIG` naming another file, the same; with either naming that
  file, no error; with no home directory, no error; the file is never
  opened (it is unreadable in the test).
- *The precedence table:* for every key in **The precedence, key by key**,
  the flag over the variable over the file over the default, where each
  level exists.
- *CA paths:* the variable alone, the key alone; both set is the
  transitional error; the variable is split on the OS's separator (a
  Windows-only test for `;` and a drive letter); a missing path fails in
  `newRequest`, before any directory is made.
- *The relay image:* empty, `dev` and references pass; values starting
  with `-`, holding a space, or holding a control character are refused,
  from the file and from the variable; not checked under a VM driver.
- *Exit codes:* each new error is a `FixableError`, which `main` exits 1
  on (UX-09).

In `driver`:

- `LoadUpstreamCAs`: a certificate; a bundle with comments between blocks;
  a directory; a file with no certificate; a certificate block that does
  not parse; a file with a certificate and a private key; in each refusal
  nothing is returned.
- `ModuleRoot`: with the working directory inside a tree that declares the
  module and the executable outside one, it fails; a `go.mod` that declares
  `github.com/buildwarden/buildwarden/tools/tier1`, or a longer path, is
  not a match.
- `StartDetached`'s argument list: `--config <path>` first when a file was
  read, nothing added when none was.

**Tier 1** (`make integration-test`; the `integration` workflow on hosted
Ubuntu with Docker, which runs because the change reaches `cmd/`, `driver/`
and `test-integration.sh`), with the built `dist/warden`:

- a project directory whose `warden.toml` sets `output_dir = "from-repo"`:
  a build with `--config` naming a file elsewhere that sets `output_dir =
  "from-named"` writes there and makes no `from-repo`; the same with
  `WARDEN_CONFIG`; `warden doctor`'s first line names the file each time;
- `--config /nonexistent`: `build`, `clean` and `doctor` exit 1 with `To
  fix:`, and no output directory is made;
- with `HOME` a temporary directory holding `.config/warden/config.toml`:
  `build`, `clean` and `doctor` exit 1 naming it; with `WARDEN_CONFIG` set
  to that path, `doctor` passes;
- `relay_image = "--help"` in the file: `build` exits 1 before any
  container exists;
- a CA path naming a file with no certificate: `build` exits 1, and no
  output directory is made;
- F9: `dist/warden` copied to a temporary directory and run, with a `go`
  first on `PATH` that writes a marker, in a directory that declares the
  module: exit 1, the marker absent;
- one of the builds the script already runs is run with `--config`, so a
  named file is built with at no extra build.

**Tier 2.** No host blocks merge (FLOW-09): no driver's own code is
changed (the two test files edited in `driver/hyperv` and `driver/qemu` run
in CI), the lookup and the checks are the same code on every OS, and
`ModuleRoot`'s first step, the only one a `warden` run from its tree ever
needed, is unchanged. Four gaps are tracked (`tier2_gaps`, and the
native-gap backlog under the ids given there): the Windows desktop
(`container-any`), `windows-hyperv`, `macos-vz` and `qemu-any`.
`macos-finch`, `macos-qemu` and `linux-kvm` add nothing of their own: the
container path is the one CI's integration run takes, and the others reach
the same shared code.

The release matrix (TEST-02) is the backstop, and the release that carries
this trips GLOBAL-06, so TEST-08 runs.

## Implementation plan

For Plan to refine. One PR, started from a `main` that has
`runtime-cli-project-path` (a7).

1. `configFile` and the one-file `LoadConfig` in `cmd/warden/config.go`:
   the lookup, the errors for a named file, the single decode; the
   `--config` flag in `main.go` (GLOBAL-09, GLOBAL-10, GLOBAL-11, ORCH-03,
   UX-04, UX-05, UX-06, UX-07, UX-09).
2. The `config:` line: `doctor` on stdout, the other five on stderr
   (GLOBAL-09, UX-01).
3. The transitional detection: `cmd/warden/config_former.go`, its two
   calls, its test file, each under `TRANSITIONAL(one-config-file)`
   (GLOBAL-01, UX-03, UX-04).
4. `WARDEN_UPSTREAM_CA_CERTS` replaces the key and is split on the OS's
   separator (GLOBAL-07, GLOBAL-10).
5. `driver.LoadUpstreamCAs` sends certificates only, and `newRequest`
   calls it before anything is created (GLOBAL-06, UX-08).
6. The relay image check in `newRequest`, under the container driver
   (GLOBAL-06, UX-04, UX-08).
7. `driver.ModuleRoot`: the executable only, the module line whole, the
   fixable error at its callers, and the test-only setter; the three tests
   in `driver/hyperv/winseed_test.go` and `driver/qemu/windows_test.go`
   name their tree through it (GLOBAL-06, GLOBAL-11, UX-04).
8. `driver.StartDetached` passes the config file (GLOBAL-10).
9. Rework of what `runtime-cli-project-path` wrote for two files:
   `resolveRuntime`'s comment and messages, its reference text and its
   overview line (FLOW-06, UX-01).
10. The tier-0 tests of the Validation plan (GLOBAL-04, TEST-01, TEST-06).
11. The tier-1 cases in `test-integration.sh` (GLOBAL-04, TEST-02).
12. The reference docs, the README, the corporate-network guide and the
    overview pseudocode (UX-01, FLOW-15).
13. The rule changes, once the review has approved each by name: ORCH-03 in
    `docs/development/orchestrator.md`, UX-07 and the clause of UX-08 in
    `docs/development/ux-cli-config.md` (FLOW-14).
14. `make lint` and `make test` green, then push (FLOW-17).
15. At Publish: the removal of the transitional detection filed as a queue
    item (a3), and the corpus entries of the Security note handed to
    `security-suite` (SEC-03).

## Design review

None yet.

## Consolidation

Filled in at Publish (FLOW-10, FLOW-15).

## Rollback notes

None.

## Archive summary

Filled in at Publish (FLOW-12).
