---
feature: project-config-trust
status: draft
drivers: [container, qemu, vz, hyperv, firecracker]  # conservatively (FLOW-02): the change is in
                         # cmd/warden and in four shared files of driver/ (hostutil.go, session.go,
                         # detach.go, and driver.go for one field of the request) that every
                         # driver uses. In the driver/<name>/ trees only
                         # test files change: a new main_test.go in hyperv and in qemu, which
                         # names the source tree once for the package, and one helper in
                         # hyperv/environment_test.go, which fails where it skipped
seams: []                # the claim's upstream_ca_certs field carries what it carried, PEM
                         # certificates; relaycfg/ is not edited
surfaces: [cli, config, output]  # cli: --config, a global flag. config: one file, found by
                         # --config, WARDEN_CONFIG or ./warden.toml; ~/.config/warden/config.toml is
                         # no longer read; WARDEN_UPSTREAM_CA_CERTS replaces the key. output: one
                         # "config:" line saying which file was read, and the "upstream CA" lines
                         # (each skip, and a count per configured path)
security_boundary: host-env  # which file warden acts on, on the host, before any isolation
                         # exists, and which source tree a development warden builds from. It
                         # also reaches env-relay: the file chooses the relay image and its
                         # upstream trust roots (Security note)
alters_relationships: false  # inside the orchestrator's existing role, reading its own
                         # configuration and finding its own source. No seam, no bootstrap change,
                         # no change of owner, no change to driver.Driver or driver.Session
design_review: required  # FLOW-19: a security boundary, two rule changes, assumed questions
native_validation:
  - {host: container-any, blocking: false, reason: "no driver's own code is changed and the lookup is the same code on every OS; what only a Windows host adds is go test ./cmd/warden and ./driver there (CI does not run make test on Windows), the transitional error naming the file under %USERPROFILE%, a drive-letter path in --config, and WARDEN_UPSTREAM_CA_CERTS split on ';'"}
  - {host: windows-hyperv, blocking: false, reason: "in driver/hyperv only test files are added or edited, and CI runs them; what only this host adds is the detached image prepare reading the file its build read (the argument list is built in shared code and unit-tested), and a development warden run from its tree still finding its guest binaries from the executable"}
  - {host: macos-vz, blocking: false, reason: "driver/vz is not edited; what only this host adds is sudo warden image prepare --driver vz, where sudo keeps the user's home and drops WARDEN_CONFIG (which file is read, and which error a ~/.config/warden/config.toml gets), and a development warden run from its tree still staging the relay and warden-io from the executable"}
  - {host: qemu-any, blocking: false, reason: "in driver/qemu only a test file is added, and CI runs it; what only this host adds is a development warden run from its tree still finding the relay VM assets in the source tree, by walking up from the executable"}
tier2_gaps:
  - {host: container-any, gap: "on the Windows desktop: go test ./cmd/warden ./driver; with %USERPROFILE%\\.config\\warden\\config.toml present, warden doctor exits 1 naming that path, and WARDEN_CONFIG set to it clears the error; --config C:\\x\\warden.toml is read and .\\warden.toml is not; WARDEN_UPSTREAM_CA_CERTS=C:\\a.pem;C:\\b.pem names two files, each with its upstream CA line on stderr; --config NUL reads no settings (backlog project-config-trust-windows)"}
  - {host: windows-hyperv, gap: "a hyperv build with --config naming a file outside the working directory whose [vm] image_refresh_after is due starts the detached image prepare, and its log's config line names that file; a development warden run from its tree builds (backlog project-config-trust-hyperv)"}
  - {host: macos-vz, gap: "sudo warden image prepare --driver vz and warden build --driver vz with ~/.config/warden/config.toml present each exit 1 naming the file, under sudo naming the file in the home sudo gives, and the fix under sudo names --config; with WARDEN_CONFIG set in the user's environment, sudo warden image prepare prints a config line that does not name that file (sudo dropped the variable), and with sudo --preserve-env=WARDEN_CONFIG or --config it names it; a development warden run from its tree builds with --driver vz (backlog project-config-trust-vz)"}
  - {host: qemu-any, gap: "a development warden run from its tree builds with --driver qemu, the relay VM assets found in the tree; a copy of that warden outside the tree, run inside the tree, reports the missing source tree and builds nothing (backlog project-config-trust-qemu)"}
rules: [GLOBAL-01, GLOBAL-04, GLOBAL-06, GLOBAL-07, GLOBAL-09, GLOBAL-10, GLOBAL-11, ORCH-01,
        ORCH-03, RELAY-08, UX-01, UX-03, UX-04, UX-05, UX-06, UX-07, UX-08, UX-09, TEST-01,
        TEST-02, TEST-03, TEST-06, TEST-08, SEC-01, SEC-03, SEC-04, FLOW-01, FLOW-02, FLOW-05,
        FLOW-06, FLOW-07, FLOW-08, FLOW-09, FLOW-10, FLOW-12, FLOW-14, FLOW-15, FLOW-17,
        FLOW-19, FLOW-20, LOOP-04, LOOP-09, PROFILE-01]
rule_changes:
  - {rule: ORCH-03, change: amend, paths: [docs/development/orchestrator.md]}
  - {rule: UX-07, change: amend, paths: [docs/development/ux-cli-config.md]}
concepts: [config-file, config-lookup, config-precedence, config-levels, config-trust,
           build-context-trust, environment-variables, user-config-removed,
           transitional-detection, module-root, relay-image, upstream-ca, security-fix]
related: [build-link-subnet.md, hyperv-relay-per-build-channel.md,
          guest-binary-cache-per-tree.md, design-principles.md]
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
  `warden`'s own source tree. So any `warden` whose guest binaries, relay
  VM assets or relay source are not beside it builds them from, or takes
  them from, a repository that declares `module
  github.com/buildwarden/buildwarden`: a development `warden` outside its
  tree, and also an installed release with a VM driver or with
  `relay_image = "dev"` (**F9: the source tree**).
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
   error that names the file and says to move its settings into
   `warden.toml` or the environment. That detection is transitional and
   removed later.

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

- **`runtime-cli-project-path`** (design approved, r1 at `a2afd08`, with
  its UX-08 change approved by name; not on `main`; its design read at
  `119a0e1`, and again at `8e41bd8`, its branch's head on 2026-11-15, in
  Implement, where only its Implementation plan differs; its code is
  cited here as designed, not as built). It merges before this item. It limits the container runtime to `finch`, `docker` or
  `podman` from every source, checked in `resolveRuntime` on the value in
  effect; it amends UX-08 (the consumer validates, after the overrides,
  only what the run uses); and it sets the pattern this design copies for a
  change that refuses what worked before: an error at once, with a
  transitional part in one file behind one marker
  (`TRANSITIONAL(runtime-former-values)`), which Jeff decided is kept until
  1.0 (its a4, answered (d)). Its section **One config file** is written to
  fit this item. Its findings F1 to F4 and F9 are this item's `source`; F5
  to F8 are `configuration-bugfixes`'.
- **`driver-contract`** (K1, in its design review, its eleven rule
  changes approved by name and its outcome pending; not on `main`; read
  at `a6c8f0b`). Since its Decision 23 it is written on one file: the
  contract hands a driver one `config_file` (a string, the absolute path
  of the file `warden` read, absent when there is none), never a list and
  with no `level`. It quotes ORCH-03 and UX-07 as this design proposed
  them at `539fd38`, waits for this item's merge before its Plan (its
  hold is in the queue, its `q1`), and kept one guard, that no config
  file names a driver by path (its a26, answered (A)). What each owes the
  other is in **How it sits with the other items**.
- **`prebuilt-artifacts`** (a security fix, filed 2026-11-15 on Jeff's
  answer to this design's a6; priority 66; in Design, with no design
  committed when this was written: its branch is at `ad4ea50`, and it is
  cited from its queue item, workflow `85669f6`). It takes compilation
  out of `warden` after this item has merged. What each owes the other
  is in **F9: the source tree** (the split) and **How it sits with the
  other items**.
- **`build-link-subnet`** (archived) added `BuildLinkSource`, and
  **`hyperv-relay-per-build-channel`** (archived) added `DNSSource` the
  same way: the base name of the file that set a key (`warden.toml` or
  `config.toml`), which `warden doctor` prints. With one file the label is
  that file's name; the mechanism stays.
- **`guest-binary-cache-per-tree`** (merged, #112). It keyed
  `GuestBinary`'s cache by the source tree `ModuleRoot` returns, so that
  two checkouts never stage each other's builds, and left how the tree is
  found as it was ("Find the source tree (`ModuleRoot`). None: refuse,
  naming the binary"). This design changes how the tree is found (F9) and
  leaves the cache and its key alone.
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
  here), UX-08 (as `runtime-cli-project-path` amends it; this item changes
  no word of it), UX-01 and FLOW-15 (the docs), UX-03 (**Compatibility**), UX-04 and
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
   setting. The edges:
   - *A named file that cannot be read is an error*, never a reason to try
     the next source (GLOBAL-10): it does not exist, it is a directory, or
     it cannot be opened. The error names the path and whether the flag or
     the variable gave it.
   - *`--config` given with an empty value is an error; an empty
     `WARDEN_CONFIG` is not set* (a8). `warden --config
     "$CFG" build` with `CFG` unset must not hand the build to the working
     directory's file without a word, which is what falling through would
     do. An empty variable is "not set" for every `WARDEN_*` variable
     today, and `WARDEN_CONFIG= warden build` is how a shell unsets one for
     a single command.
   - *To read no settings at all, name an empty file* (`--config /dev/null`
     on Unix, `--config NUL` on Windows). There is no flag for it.
   - *The default is the working directory's file, and is read only when it
     exists.* It is `./warden.toml` of the directory `warden` runs in,
     whatever `[path]` names: `warden build /src/repo` from another
     directory does not read `/src/repo/warden.toml`, today or after. A
     `./warden.toml` that exists and cannot be read, or is not TOML, is an
     error, as today for the syntax (GLOBAL-11: today any `stat` failure is
     read as "no file"; after this, only "does not exist" is).
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
   - *Rejected: a command that reads no configuration refuses the flag.*
     The same reason: it is `driver-contract`'s to do for every flag.
3. **`~/.config/warden/config.toml` is not read, and its existence is an
   error** (Jeff's decision 4), on every command that reads configuration,
   whichever way the config file was chosen. The message leads with what
   Jeff decided it says: move the settings into the `warden.toml` you build
   with, or into the environment. One exception, which is this design's
   own and was confirmed in the review (a10): when the file `--config` or
   `WARDEN_CONFIG` names *is* that file, it is the config file, there is
   nothing stale, and the error does not fire; the message gives that as
   its second way out. The error is transitional: one file of code, one
   marker, removed at 1.0 (a3).
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
   replaces it from this item on, and while the transitional detection
   stands, setting both is an error that says so (a5).
   That error is raised where the CA paths are used (`build`, `shell` and
   `doctor`), not as the file loads: `clean`, `setup` and `image prepare`
   use no CA path and do not fail on it (UX-08 as amended: only what a run
   uses is validated).
   - *Rejected: keep the append and name it as a merge* (GLOBAL-10 allows a
     named merge). It would be the one variable of six that adds, in the
     item that writes "a value set at any level replaces the lower levels'
     value" for the environment.
5. **A relative path in the file is relative to the working directory**, as
   today and as the same path given by a flag or a variable is (a1). The
   file's own location carries no meaning: Jeff's "the
   warden.toml's location in the working directory is simply for ease of
   use".
6. **No environment variable is added for the keys that have none**
   (a2). A machine's own settings go in the file the
   machine names with `WARDEN_CONFIG`.
7. **What a config file may decide is not limited by where the file was
   found.** A `./warden.toml` read by default decides what a named file
   decides. `warden` says which file it read, on every command that reads
   one, so that the default is never silent (a4). The
   protection for a repository the executor does not control is to name the
   file: `--config`, or `WARDEN_CONFIG` once in the machine's environment.
   The variable does not reach a `warden` run under `sudo`, which resets
   the environment by default: there the file is named with `--config`, or
   `sudo` is told to keep the variable (**Security note**, what remains).
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
   - an upstream CA path is read once, before anything is created, and
     only valid PEM certificates are sent to the relay (a9, as Jeff
     answered it: "send valid pem certificates, log to stderr which files
     are specifically sent versus skipped with reason"). The rule is per
     file, and is the same for a file named directly and for each `*.pem`,
     `*.crt` and `*.cer` file of a directory that is named. What is not a
     valid PEM certificate (a private key or any other PEM block, a DER
     file, a certificate that does not parse) is not sent, and each such
     skip is a line on stderr, with its reason, on every run; a
     certificate beside it is still sent. A configured path that does not
     exist, cannot be read, or gives no certificate at all is an error.
     Each configured path gets one line with the count of its
     certificates, and `--verbose` adds the list per file (**F1 to F4, one
     by one**, F2);
   - `WARDEN_UPSTREAM_CA_CERTS` is split on the OS's path-list separator
     (`;` on Windows), not on `:`, which cuts a drive letter in two.

   `warden doctor` makes the first two checks too, for the driver it
   reports on, and prints the same CA lines in its report, so that it
   does not print `ok` for a relay image or a CA path a build would
   refuse.
   - *Rejected in the review: refuse a file that holds anything but
     certificates* (a9's option (a), which this design proposed at
     `3c0caab`). It stopped three kinds of build that run today: a
     certificate with its key in one file, a directory that also holds a
     DER file, a bundle with one certificate Go's parser refuses. Jeff's
     answer keeps them running and has each skip said on every run; what
     it gives up is in **Defaults and overrides**.
   - *Rejected: skip without a line* (a9's option (c)). The skip is
     allowed because it is said (GLOBAL-10, GLOBAL-11).
9. **`warden` finds its source tree from its own executable only** (F9;
   a6). `driver.ModuleRoot` stops walking up from the working directory,
   and matches the module line exactly. A `warden` that
   needs the tree and is not inside it (a development build copied or
   installed elsewhere; a release used with a VM driver or with
   `relay_image = "dev"`) reports that, with the fix, instead of building
   from whatever tree the working directory is in.

   This is the small fix, and it is all this item does about F9. Jeff's
   answer to a6 goes further than any option put to him: `warden` is to
   need no source tree and to compile nothing. He gave that to its own
   security item, `prebuilt-artifacts`, filed the same day and designed
   in parallel, which merges after this one and then removes the tree
   lookup and every `go build` `warden` runs (**F9: the source tree**,
   the split).
   - *Rejected by Jeff, for this item: remove compilation here.* Asked
     where it is built, he chose its own item, so that the hole closes
     with this item's merge and K1 is not held behind the native
     validation that taking every `go build` out of `warden` needs.
   - *Rejected: a variable that names the tree* (a6's option (b)), and
     *the tree the compiler recorded* (its (c)). Neither is wanted by an
     answer that ends with no tree at all.
10. **A background `warden image prepare` reads the file its build read.**
    A qemu Windows or hyperv build whose prepared image is due for refresh
    starts `warden image prepare` detached (`driver.StartDetached`). Today
    the child finds `./warden.toml` by itself, through the working
    directory it inherits. With `--config` on the parent that would be a
    different file, so the parent passes `--config <absolute path>` to the
    child when it read a file.
11. **Two rule changes**, each made true by this item's code (the
    umbrella's Decision 18): ORCH-03 and UX-07 state the one file and the
    order with the environment in it. UX-08 is not changed: as
    `runtime-cli-project-path`'s review approved it, it names no level of
    config file (**Rule changes**).
12. **Other defects found are handed on, not fixed**, each to a named
    item (a11; **What this item does not do**). The review session
    recorded the hand-offs in the queue on 2026-11-15 (workflow
    `4c59842`): four observations in `configuration-bugfixes`' `source`,
    and the two security findings as the new item `workdir-in-context`.
13. **`warden.toml` in a message means the config file in use.** Nineteen
    fix strings and messages name the file as `warden.toml` ("set `[vm]
    dns` in warden.toml ..."), seventeen of them in four drivers' trees and
    `driver/hostlink.go`. The seventeen stay as they are: Jeff's name for
    the one file is "the warden.toml", whichever path it has, and the
    `config:` line at the top of the same output gives that path (a4).
    The reference says so in
    one sentence. The two messages of `cmd/warden` that name the
    file for a key's bad value (`[vm] image_refresh_after` and
    `image_min_grace`; `[vm] windows_image_update`) name the file that was
    read, by its path, where the build link's message names it by its
    source label.
    - *Rejected: reword the seventeen to "the config file".* It edits
      built code in `driver/qemu`, `driver/firecracker`, `driver/vz` and
      `driver/hyperv` for a wording change, in the item that otherwise
      edits none, and `driver-contract` rewrites those drivers' reading of
      configuration next.
    - *Rejected: pass the path to the drivers so that each message names
      it.* A new field on every driver's request for the text of a fix,
      which K1's `config_file` gives them anyway.

## Questions

### Blocking

None.

### Assumed

**State after the review's first sitting, 2026-11-15.** The eleven that
were asked are answered, and each carries its answer below; Jeff read the
design at `3c0caab`. Nine were confirmed as recommended: a1 to a5, a7, a8,
a10 and a11. Two were answered otherwise, with none of the offered options
taken, and are folded into the design: a9 (what a CA path may hold: every
valid PEM certificate is sent, the rest is skipped and logged) and a6 (this
item keeps the lookup fix as designed, and a new item, `prebuilt-artifacts`,
removes compilation). Each question's text and options are kept as they
were put, so "Proceeding with" in an answered question describes what was
asked, not always what the design now does. One question is new and open:
a12, raised by a9's fold-in.

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

  **Answered 2026-11-15: (a), confirmed as recommended.** The working
  directory, always (Decision 5).
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
    where a variable per driver setting would be decided. Fourteen keys
    then have no variable and five have neither a flag nor a variable, so
    UX-07 is worded for a knob that has fewer than three levels (**Rule
    changes**).
  - (b) Variables for the settings that belong to a machine or its network
    and have neither flag nor variable: `[vm] dns` and `[relay]
    system_ca_bundle` (and `WARDEN_BUILD_LINK` beside `--build-link`). The
    corporate-network guide's settings then need no file. Three new names,
    chosen before `driver-contract` has settled which keys stay `warden`'s.
  - (c) A variable for every key, by one naming scheme. Every setting is
    then reachable from the environment (seven keys would still have no
    flag). Fourteen new names, several of them for keys `driver-contract`
    renames.

  **Answered 2026-11-15: (a), confirmed as recommended.** None added
  (Decision 6).
- **a3. When is the transitional detection removed?** It makes two errors:
  the one for an existing `~/.config/warden/config.toml`, and the one for
  `WARDEN_UPSTREAM_CA_CERTS` set together with the key (a5). After its
  removal an old user file is ignored without a word, and the variable
  replaces the key without a word. Proceeding with (a); confirm.
  - (a) Kept until 1.0, which is what Jeff answered on 2026-11-15 for
    `runtime-cli-project-path`'s transitional detection (its a4, (d)). One
    removal point for both items' transitional code. Publish files this
    item's removal as its own small queue item whose `source` names the
    marker and 1.0. **Recommended.**
  - (b) The release that first carries the change and the two after it,
    removed from the third. Sooner than the other item's, so two dates.
  - (c) The release that first carries the change and the one after it.
  - (d) Removed by `driver-contract`, when it rewrites configuration
    loading. The date then depends on that item, and its diff grows.

  **Answered 2026-11-15: (a), confirmed as recommended.** At 1.0, the
  removal point of `runtime-cli-project-path`'s detection.
- **a4. What, if anything, protects the default: `./warden.toml` in a
  working directory that is someone else's repository?** Proceeding with
  (b); confirm. Under every option but (d), `cd repo && warden build` acts
  on the repository's file: it chooses the driver, the relay image, the
  relay's trust roots, where output and work directories go, and (with
  `--driver hyperv`) the base image put in the user's cache. The protection
  is to name the file, and under `sudo` to name it with `--config`, because
  `sudo` drops `WARDEN_CONFIG` by default.
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

  **Answered 2026-11-15: (b), confirmed as recommended.** A `config:`
  line on every command that reads configuration, and nothing refused by
  where the file was found (Decision 7).
- **a5. `WARDEN_UPSTREAM_CA_CERTS` when `[relay] upstream_ca_certs` is also
  set.** Today the variable's paths are added to the file's. Proceeding
  with (a); confirm.
  - (a) The variable replaces the key (GLOBAL-10), and while the
    transitional detection stands, setting both is an error that says the
    meaning changed and to put every path in one of them. Nobody gets fewer
    trust roots without being told. The error is raised on the commands
    that use the CA paths (`build`, `shell`, `doctor`), not on `clean`,
    `setup` or `image prepare`. **Recommended.**
  - (b) The variable replaces the key at once, with no error. Someone who
    relied on the sum gets upstream TLS failures and has to work out why.
  - (c) The append stays and is named as a merge in the reference and in
    **Defaults and overrides** (GLOBAL-10 allows it). No behaviour changes;
    the environment is then a level that replaces for five variables and
    adds for one.

  **Answered 2026-11-15: (a), confirmed as recommended.** The variable
  replaces the key, and both set is a transitional error on `build`,
  `shell` and `doctor` (Decision 4).
- **a6. How `warden` finds its source tree (F9).** Today: walking up from
  the executable, then from the working directory. Proceeding with (a);
  confirm.
  - (a) From the executable only. `make build` puts `dist/warden` in the
    tree, with the guest binaries beside it, which is how CONTRIBUTING, CI
    and the native sessions run it, so nothing they do changes. What stops
    working: `go run ./cmd/warden` and a `go install`ed development
    `warden` (whose executable is in a cache or in `GOBIN`); and an
    installed release used with a VM driver, or with `relay_image = "dev"`,
    from inside a checkout, which works today only through the
    working-directory step (the release ships no Linux or Windows guest
    binary under the name `warden` looks for, and no relay VM assets). Each
    gets an error that says to run `make build` and use `dist/warden`. Four
    tests in two drivers' packages rely on the working-directory step today
    and get the tree named once per package instead (**F9: the source
    tree**). No new surface. **Recommended.**
  - (b) (a), plus a variable (`WARDEN_SOURCE_DIR`) that names the tree and
    is used exclusively. `go run`, `go install` and a release pointed at a
    checkout keep working for someone who sets it. One more variable
    outside the reference's knobs (UX-07), and one more thing the
    environment can point at code.
  - (c) The tree the binary was compiled from (the path the compiler
    recorded). Nothing to set, and `go run` works. It fails under
    `-trimpath`, and a binary that reads its own build paths is magic
    (GLOBAL-11).

  **Answered 2026-11-15: changed.** None of the three as written; this
  item keeps (a), and a new item does what the answer asks for. Jeff's
  words: "So warden shouldn't need its own source directory for anything
  after componentization.  It should instead fully be based on
  pre-compiled existing binaries/artifacts which are managed and
  installed and it may have development tooling for helping with that,
  but it does not manage any compilation.  Those are purely development
  processes". Asked what this fix does now: "Remove compilation now.
  warden looks for necessary artifacts  to be on PATH and searches its
  own directory if they're not found or not on or above a minimum
  version". Asked where that is built, he chose its own security item,
  started now and designed in parallel. `project-config-trust` keeps the
  small lookup fix, which is (a) as designed here (the tree found from
  the executable only; the three test files), so the hole closes with
  this item's merge and K1 is not held behind native validation. The new
  item then removes the tree lookup and every `go build`. It is filed:
  `prebuilt-artifacts`, priority 66, branch `feat/prebuilt-artifacts`, in
  Design (workflow `85669f6`). Folded in as Decision 9, the split under
  **F9: the source tree**, **How it sits with the other items** and item
  8 of the Security note's **What remains**. No word of (a)'s design
  changed.
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

  **Answered 2026-11-15: (a), confirmed as recommended.** The hold is a
  blocking question that names `runtime-cli-project-path`'s merge. It is
  not in the queue file yet: the review session raises it when it records
  this design's approval, and Jeff answers it once that PR has merged.
- **a8. What an empty `--config` or `WARDEN_CONFIG` means.** `warden
  --config "$CFG" build` with `CFG` unset passes an empty value. The next
  source down is, in the end, `./warden.toml`, which in someone else's
  repository is the file the executor meant not to use. For `--driver` and
  `--runtime`, where an empty value is "not set" today, the next source is
  not an untrusted file. Proceeding with (a); confirm.
  - (a) `--config` given with an empty value is an error (cobra tells a
    flag that was given from one that was not); an empty `WARDEN_CONFIG` is
    not set, as every empty `WARDEN_*` variable is today. The typing
    mistake on the command line is caught. `WARDEN_CONFIG="$CFG" warden
    build` with `CFG` unset still reads `./warden.toml`; the `config:` line
    shows it (**Security note**, what remains). **Recommended.**
  - (b) Both are an error when set and empty. Nothing falls through. It
    breaks `WARDEN_CONFIG= warden build`, the shell's way to unset a
    variable for one command, and is the one `WARDEN_*` variable whose
    empty value is not "not set".
  - (c) Both are "not set", as an empty `--driver`, `--runtime` and
    `WARDEN_CTR_CLI` are. One meaning of empty everywhere, and the silent
    case above for the flag as well.
  - (d) Empty means "read no file". A typing mistake then builds with the
    defaults and not with the repository's file, also without a word.

  **Answered 2026-11-15: (a), confirmed as recommended.** An empty
  `--config` is an error; an empty `WARDEN_CONFIG` counts as not set
  (Decision 1).

  The other edges of the lookup are not asked, because a rule or Jeff's
  decision settles each: a named file that cannot be read is an error and
  nothing else is tried (GLOBAL-10); a `./warden.toml` that exists and
  cannot be read is an error, not "no file" (GLOBAL-11); `--config` is a
  global flag (UX-05). Reading no settings by naming an empty file, with no
  `--no-config`, is Decision 1's rejected alternative. The review confirms
  them with the Decisions.
- **a9. What a path named as an upstream CA may hold** (Decision 8). A
  path is a file or a directory; of a directory, the files whose names end
  in `.pem`, `.crt` or `.cer` are read, not its subdirectories. Today the
  bytes of each file are sent to the relay whatever they are, and the relay
  uses the PEM certificates it can parse and skips the rest without a word.
  Proceeding with (a); confirm.
  - (a) Certificates only, one rule per file, the same for a file named
    directly and for each file read from a named directory. Text outside
    PEM blocks (the comments of a distribution bundle) is allowed and not
    sent. An error naming the file: no PEM certificate in it; a
    certificate that does not parse; any other PEM block. An error naming
    the directory: no `.pem`, `.crt` or `.cer` file in it. What works
    today and stops: a file that holds a certificate and its private key,
    which is sent whole today; a directory that also holds a DER-encoded
    `.crt` or `.cer` (common for a root exported on Windows), which is
    skipped by the relay today; a bundle with one certificate Go's parser
    refuses (a negative serial number, for one), which the relay skips
    today. In the last two the build ran, without the root the user
    thought was added; after this it stops and says which file (GLOBAL-10:
    "a provided value that cannot be used is an error that names it and
    its fix"). **Recommended.**
  - (b) As (a) for what must not leave the host and for a path that gives
    nothing (another PEM block; a file or a directory with no certificate
    at all), and as today for what the relay skips: a file in a named
    directory that holds no PEM, and a certificate that does not parse,
    are not sent and each is named on one line of stderr. No build that
    works today stops for a DER file or one odd certificate; the skipped
    root is said once and then scrolls away.
  - (c) Send the certificates and drop everything else without an error. A
    combined file keeps working; a path that names the wrong file sends
    nothing and says nothing.
  - (d) Leave it: the file is the executor's choice. The early read is
    still made.

  **Answered 2026-11-15: changed.** None of the four as written; nearest
  is (b), with a private key or another PEM block skipped too, where (b)
  refused the file. Jeff's words: "send valid pem certificates, log to
  stderr which files are specifically sent versus skipped with reason".
  Two follow-ups, each asked on its own. What is still an error that
  stops the build before it starts: **"Also when nothing is sent"**, so a
  named path that does not exist or cannot be read is an error, and so is
  a named path that yields no certificate at all; everything else (a key
  beside a certificate, a DER file, a certificate that does not parse, a
  file in a directory with no certificate) is skipped with a logged
  reason. How much is logged: **"Skips always, sends counted"**, so every
  skipped file gets a stderr line with its reason on every run, sent
  files get one line per configured path with the count, and the per-file
  list of what was sent appears with `--verbose`. Folded in as Decision 8
  and as F2 of **F1 to F4, one by one**, which gives every line and every
  error.

  The answer leaves edges. Each but one is settled by his words, by a
  rule or by what `main` does today, and F2 says which: the system roots
  are not a configured path; a path from `WARDEN_UPSTREAM_CA_CERTS` is
  held to the same rule; an entry of a named directory that cannot be
  read stays the error it is today; links are followed, as today; the
  same certificate in two files is sent twice, as today; a private key
  gets a `warning:` line; `warden doctor` prints the lines in its report,
  on stdout. The review confirms them with the design. The one that is a
  real fork is a12.
- **a10. May `--config` or `WARDEN_CONFIG` name
  `~/.config/warden/config.toml` itself, and which fix does the
  transitional message give first?** Jeff's decision 4 is an error "saying
  to move its settings into warden.toml or the environment". Keeping the
  old path in use by naming it is this design's own addition. It is not a
  second level: one file is read. Under every option a per-user file can
  stay in use for good, because decision 2 lets `WARDEN_CONFIG` name any
  file; the options differ in whether it may stay at the old path, and in
  what the message says first. Proceeding with (b); confirm.
  - (a) The error does not fire when the named file is that file, and the
    message gives that first ("to keep using this file, set
    `WARDEN_CONFIG=...`"): one line in a shell profile and nothing moves.
    It reads as the user-level file surviving under another name.
  - (b) The error does not fire when the named file is that file, and the
    message leads with Jeff's words (move the settings into the
    `warden.toml` you build with, or into the environment) and gives
    naming the file second. A file the executor names is the config file
    whatever its path, so no path is special. **Recommended.**
  - (c) No exemption: while the detection stands, that path is an error
    even when it is the file named, and the message says only to move the
    settings. Nearest to the decision as worded. Someone who wants a
    per-user file moves it (`mv` to another path, and `WARDEN_CONFIG`),
    and one path cannot be named until 1.0 and can be afterwards.

  **Answered 2026-11-15: (b), confirmed as recommended.** Naming that
  file is allowed, and the message leads with "move its settings"
  (Decision 3).
- **a11. Where the defects this item found and does not fix are
  recorded.** They are in **What this item does not do**: four
  observations that are not security matters, and two that reach the host
  from the build context without any config file (a work directory placed
  inside the build context is served to the build; a Dockerfile's `FROM`
  value reaches the container runtime's command line unchecked). This
  agent may edit only its own queue item. Proceeding with (a); confirm.
  - (a) The session that records this design's review appends the four
    observations to `configuration-bugfixes`' `source` (it is queued, on
    hold behind the freeze, and has no design yet), and files the two
    others through intake as one security item, proposed slug
    `workdir-in-context`, whose `source` carries both. Nothing waits on
    either. **Recommended.**
  - (b) This item traces and fixes the two security findings itself. They
    are in `driver/` and the collector, not in configuration; the item
    grows by two fixes in every driver, and a blocking native validation
    with them.
  - (c) They stay recorded in this design only, and are found again by
    whoever reads it.

  **Answered 2026-11-15: (a), confirmed as recommended,** and done the
  same day by the review session (workflow `4c59842`): the four
  observations are appended to `configuration-bugfixes`' `source`, and the
  two security findings are filed as one item, `workdir-in-context`
  (priority 67, queued). Nothing of this item waits on either.
- **a12. The entries of a named CA directory that are not read because
  of their names.** New on 2026-11-15, raised by a9's answer. A directory
  path means its `.pem`, `.crt` and `.cer` files, one level deep; its
  other entries (a file of another ending, a subdirectory) are not
  opened, today and after. The answer has every skipped file get a line
  on every run. It was given for files that are read and hold something
  other than a valid PEM certificate; whether an entry that is never
  opened is a skipped file, his words do not say. The case is common: on
  this host `/etc/ssl/certs` has 248 entries, of which 124 are read, 123
  are hash-named links (`002c0b4f.0`) and one is a subdirectory.
  Proceeding with (a); confirm.
  - (a) Counted on every run, listed with `--verbose`. The path's line
    ends "; 124 other entries not read", and with `--verbose` each is a
    line with its reason (its name's ending, or that it is a directory).
    A root saved as `corp.der` or `corp.cert` is in the count on every
    run and is named on request. **Recommended.**
  - (b) One line each, on every run: "skips always" to the letter. A
    system certificate directory then prints 124 lines on every build,
    and the lines that matter (a private key, a DER file) are among
    them.
  - (c) Not mentioned at all: by the reference they are not CA files. A
    root saved under another ending then gives no line and no count,
    which is the silent case the answer is against.

## Design

It works only in this repository.

### How the config file is found

```
configFile():                                  # once per command that reads configuration
    --config given, empty     -> error (a8)
    --config given            -> that path, source "--config"
    WARDEN_CONFIG not empty   -> that path, source "WARDEN_CONFIG"
    ./warden.toml exists      -> it, source "found in the working directory"
    else                      -> none

LoadConfig():
    path, source := configFile()
    path named and cannot be read  -> the error; nothing else is tried
    former user config exists, and is not path  -> the transitional error (below)
    none                      -> defaults
    else                      -> decode that one file
    apply the environment variables, then (resolveConfig) the flags
```

- `./warden.toml` is the working directory's, not the build context's:
  `warden build /src/repo`, run from elsewhere, looks in the directory it
  runs in.
- A relative `--config` or `WARDEN_CONFIG` is relative to the working
  directory, like any path argument, so a relative `WARDEN_CONFIG` names a
  different file in each directory. A value set once for a machine should
  be absolute, and the reference says so. Symbolic links are followed.
- The file is decoded once, by the decoder used today. A file that is not
  TOML is an error for every command that reads it (UX-08 as amended: "a
  config file that cannot be parsed is an error for every run that reads
  it").
- The errors, each a `driver.FixableError` (UX-04), exit 1 (UX-09):

  ```
  ERROR: config file /etc/warden/ci.toml (from --config) does not exist
    To fix: pass --config an existing file, or drop it to use WARDEN_CONFIG or ./warden.toml
  ```

  ```
  ERROR: config file /home/ci/warden.toml (from WARDEN_CONFIG) does not exist
    To fix: set WARDEN_CONFIG to an existing file, or unset it to use ./warden.toml
  ```

  ```
  ERROR: --config was given an empty path
    To fix: pass --config the path of a config file, or drop the flag to use WARDEN_CONFIG or ./warden.toml
  ```

  ```
  ERROR: cannot read config file /src/repo/warden.toml (found in the working directory): permission denied
    To fix: make it readable, or name another file with --config or WARDEN_CONFIG
  ```

  A path is printed absolute, whatever form it was given in. A directory
  gets the first or second form with "is a directory". The syntax error
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
  `--verbose` (a4).
- `warden doctor`'s source label for the build link and the resolvers
  (`build link 100.126.255.252/30 (warden.toml)`) stays the base name of
  the file that set the key, as today; with one file it is that file's
  name, and the `config:` line gives its path. The label `config.toml`
  appears only when the named file is called that.
- A fix that says "in warden.toml" (the drivers' messages about `[vm]
  dns`, `[vm] build_link` and `[vm] network`) means the config file in
  use, whose path this line gives (Decision 13).

### The user-level file

What stops: `~/.config/warden/config.toml` is not read, on any OS (it is
`%USERPROFILE%\.config\warden\config.toml` on Windows), and nothing is
merged under `./warden.toml`.

The transitional error, on `build`, `shell`, `clean`, `doctor`, `setup` and
`image prepare`, once the config file is chosen and before it is read:

```
ERROR: /home/you/.config/warden/config.toml is no longer read: warden reads one config file, ./warden.toml or the one --config or WARDEN_CONFIG names
  To fix: move its settings into the warden.toml you build with, or into the environment (WARDEN_DRIVER, WARDEN_CTR_CLI, WARDEN_IMAGE, WARDEN_UPSTREAM_CA_CERTS), and delete it; or name it as the config file with --config or WARDEN_CONFIG (it is then the only one read: ./warden.toml is not)
```

- The path is the one `LoadConfig` reads today: `os.UserHomeDir()` joined
  with `.config/warden/config.toml`. With no home directory there is
  nothing to look for.
- "Exists" is `os.Stat` succeeding; the file is not opened.
- A named config file that cannot be read is reported first: it is what
  the user just asked for.
- It does not fire when `--config` or `WARDEN_CONFIG` names that same file
  (`os.SameFile`), which is the message's second way out (a10).
- `warden doctor` exits 1 with it, like every other command: doctor
  reports what a build would get.
- The commands that read no configuration do not look.
- It is not a check of a setting, so UX-08's "only the configuration a
  run uses is validated" does not reach it: nothing in the file is read.
  It reports a source of settings that has stopped applying, on every
  command where it used to apply.

Under `sudo`. The home is the one `sudo` gives the process, as today: the
user's on macOS, root's on most Linux setups. And `sudo` resets the
environment by default (`env_reset`), so a `WARDEN_CONFIG` the user set is
not there: `WARDEN_CONFIG=/etc/warden/ci.toml sudo -n sh -c 'echo
"[$WARDEN_CONFIG]"'` prints `[]` on this host. Three commands are run
under `sudo` and load configuration: `sudo warden image prepare --driver
vz` and `sudo warden setup --driver firecracker` (the guides), and `sudo
dist/warden doctor --driver firecracker` (the firecracker workflow). So
someone who followed the second
way out with the variable meets the error again under `sudo` where the
home is kept. When `SUDO_USER` is set, the fix therefore ends with one
more sentence:

```
  ... Under sudo, WARDEN_CONFIG is dropped unless sudo is told to keep it: pass --config, or run sudo --preserve-env=WARDEN_CONFIG
```

The docs subsection says the same.

Where it lives, so that it is deleted in one place, as
`runtime-cli-project-path` does for its own:

- One file, `cmd/warden/config_former.go`, with two functions: one that
  returns the error above or nothing, and one that returns the a5 error for
  the variable and the key set together, or nothing.
- Two calls, each a single line: the first in `LoadConfig`; the second in
  `resolveUpstreamCAs`, the function that resolves the CA paths for the
  commands that use them (**F1 to F4, one by one**, F2). `LoadConfig` does
  not raise the a5 error: it would fail `clean`, `setup` and `image
  prepare`, which use no CA path (UX-08 as amended: "nothing validates a
  file or environment setting the run does not use").
- One test file, `config_former_test.go`.
- One subsection of the configuration reference ("Moving from
  `~/.config/warden/config.toml`").
- One marker on each: `TRANSITIONAL(one-config-file)`, in the file's header
  comment (which says why the code exists and when it goes, GLOBAL-01), at
  the two calls, at the head of the test file, and in an HTML comment above
  the docs subsection. A search for the marker finds everything to delete.
- When: a3 (at 1.0). Publish files the removal as a queue item whose
  `source` names the marker and the condition.

### Who is affected, and what they do instead

| Today | After |
|---|---|
| Settings in `~/.config/warden/config.toml`, no `./warden.toml` | The error. Move the settings into the `warden.toml` built with or into the environment; or name that file with `--config` or `WARDEN_CONFIG`, which keeps everything as it was (a10) |
| Settings in both files, merged | The error. One file per build: combine them, or name the user file and stop using the project's |
| Only `./warden.toml` | Nothing changes, but for the `config:` line |
| No file | Nothing changes, but for the `config:` line |
| `WARDEN_CONFIG` set in a user's environment, `warden` run under `sudo` | `sudo` drops the variable: `./warden.toml` is read, or none. Pass `--config`, or `sudo --preserve-env=WARDEN_CONFIG` |
| `WARDEN_UPSTREAM_CA_CERTS` and `[relay] upstream_ca_certs` both set | The a5 error on `build`, `shell` and `doctor` while the detection stands; afterwards the variable replaces the key |
| A CA file that holds a certificate and its private key | Builds, as today. The certificate is sent and the key is not, where today the whole file is; a `warning:` line says so on every run (a9) |
| A CA directory that also holds a DER-encoded `.crt` or `.cer`, or a bundle with a certificate Go cannot parse | Builds, as today, without that root, as today. A line names the file and the reason on every run, where today the relay drops it without a word (a9) |
| A CA path that gives no certificate at all: a DER file, a key or a wrong file named directly, or a directory with no certificate in a `.pem`, `.crt` or `.cer` file | An error naming the path, before anything is created. Today the build runs with no root added (a9) |
| Any configured CA path | One line per path on stderr with the number of certificates, on `build` and `shell`; in the report of `doctor` (a9) |
| `relay_image` or `WARDEN_IMAGE` starting with `-` or holding a space | Refused |
| A development `warden` run from outside its source tree, inside the tree (`go run`, `go install`, a copy) | An error naming `make build` and `dist/warden` (a6) |
| An installed release with `--driver qemu`, `firecracker` or `vz`, or with `relay_image = "dev"`, run inside a buildwarden checkout | The same error. Today it takes the guest binaries, the relay VM assets or the relay's source from the checkout it is run in (a6) |

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
| `[relay] upstream_ca_certs` | the file's list **plus** `WARDEN_UPSTREAM_CA_CERTS`'s, split on `:`, folded together in `LoadConfig` | **no**: a merge. Changed here (a5), and the split with it; resolved in `resolveUpstreamCAs` |
| `[relay] system_ca_bundle` | file, `true` | yes. Honoured only together with CA paths: `false` alone does nothing, silently. Handed on |
| `[build] output_dir` | `-o`, file, `warden-output` | yes |
| `[build] capture` | `--capture`, file, none | yes |
| `[build] compress` | off when the file says `false`, else on unless `--no-compress` | as far as the flag can say: it can only turn compression off, so `--no-compress=false` does not undo the file's `false`. Handed on |
| `[build] tmpdir` | `--tmpdir`, file, the OS temp dir | yes |
| `[build] dockerfile`, `context` | never read | F5, `configuration-bugfixes` |
| `[vm] cpus`, `memory_mb` | flag, file, the driver's default; `0` is "not set" at both | yes |
| `[vm] network` | `--network`, file, `warden-dev` (hyperv) | yes |
| `[vm] build_link` | `--build-link`, file, the default | yes. The file's value is checked as it loads even when the flag replaces it: one of the checks UX-08 as amended lists by name |
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
    further: the runtime's own error does that. `warden doctor` makes the
    same check under the container driver, through the same function, so
    it does not print `ok` for a value a build refuses.

    ```
    ERROR: [runtime] relay_image in /src/repo/warden.toml is "--volume=/:/host": not an image reference
      To fix: set it to an image reference such as ghcr.io/buildwarden/relay:v0.2.0 or to "dev", or remove it
    ```

    (`WARDEN_IMAGE is ...` when the variable set it.) The check is on the
    value in effect, after the overrides, and only when the run uses it
    (UX-08 as amended). `driver-contract` moves it with the setting.
  - *A defect regardless:* `relay_image = "dev"` builds from a tree that
    can be the working directory's. That is F9, fixed here.
- **F2, the trust roots.** With the executor's own file, naming extra
  roots is what the key is for.
  - *A defect regardless:* the files are read when the relay is claimed,
    after the output directory, the work directory and the relay exist, so
    a mistyped path fails deep into the build (UX-08). **Fixed here:**
    one function in `cmd/warden`, `resolveUpstreamCAs`, resolves the paths
    in effect (the variable's, else the key's; `LoadConfig` no longer
    folds the variable into the key, so that the function sees both), makes
    the transitional a5 call, reads the paths through
    `driver.LoadUpstreamCAs`, and prints what that function reports.
    `newRequest` calls it for every driver before anything is created, and
    `warden doctor` calls it too, for the driver it reports on. No other
    command uses the paths, and none checks them.

    The files are read once per run. Today `ClaimRelay` reads them, from
    the paths in the request. After this the request carries what was
    read: `driver.BuildRequest`'s `UpstreamCACerts`, today the paths,
    becomes the certificates (`driver/driver.go`), and `ClaimRelay` sends
    them and opens no file. So what the lines below say is what the claim
    carries, and a file that changes between the check and the claim
    changes nothing.
  - *A defect regardless:* the bytes of each file are sent to the relay
    whatever they are. A path that names the wrong file sends that file,
    and a bundle with a private key in it sends the key. The relay then
    uses what it can parse and drops the rest without a word. **Fixed
    here**, as Jeff answered a9: "send valid pem certificates, log to
    stderr which files are specifically sent versus skipped with reason".

    **Terms.** A *configured path* is one entry of the list in effect:
    `WARDEN_UPSTREAM_CA_CERTS`'s entries when the variable is set, else
    those of `[relay] upstream_ca_certs`. Each is a file or a directory,
    and a relative one is relative to the working directory (a1). The
    system roots are not a configured path: `warden` reads no host file
    for them. They are the relay's own (`x509.SystemCertPool` inside the
    relay, `relay/proxy.go`), added by the relay unless `system_ca_bundle
    = false`. They get no line and no count here, and they do not count
    towards "this path gives a certificate".

    **What is sent.** Every valid PEM certificate found under every
    configured path, and nothing else. Per file, the same for a file named
    directly and for each file read from a named directory:
    - the file is read whole, and its PEM blocks are taken in order;
    - a block is sent when its type is `CERTIFICATE`, it carries no PEM
      headers, and it parses as an X.509 certificate (Go's
      `x509.ParseCertificate`). Those three conditions are the ones the
      relay's trust pool applies today (`AppendCertsFromPEM`), so, file
      by file, the certificates sent are the ones the relay takes from
      that file today, and what it drops today without a word is what is
      skipped here with a line;
    - it is sent re-encoded from its DER bytes, so no other byte of the
      file reaches the claim: not the text between blocks (a distribution
      bundle's comments), not a neighbouring block;
    - the same certificate found in two files is sent twice and counted
      in each, as today; the relay's pool keeps one. On this host
      `/etc/ssl/certs`, named as a directory, gives 245 certificates from
      124 files, 122 of them a second time from the bundle file.

    **A directory** is read one level deep, as today: its entries whose
    names end in `.pem`, `.crt` or `.cer`, in any letter case, in name
    order. A symbolic link is followed, as today (a distribution's
    certificate directory is made of them). An entry that is a directory,
    or a link to one, is not read, and an entry with another ending is not
    opened; both are the "other entries" of a12.

    **What is skipped, each with its line.** A skip never stops the run
    and never changes the exit code. The lines are shown as `build` and
    `shell` print them; a path is absolute, and a block is numbered by its
    position among the file's PEM blocks, from 1.

    A block that is not sent, in a file whose other blocks are still
    looked at:

    ```
    [warden] warning: upstream CA /etc/warden/site.pem: block 2 is an RSA PRIVATE KEY and is not sent to the relay; a private key does not belong in a file of trust roots
    [warden] upstream CA /etc/warden/site.pem: block 3 skipped: a CERTIFICATE REQUEST block, not a CERTIFICATE
    [warden] upstream CA /etc/warden/bundle.pem: block 41 skipped: a CERTIFICATE that does not parse (x509: negative serial number)
    [warden] upstream CA /etc/warden/bundle.pem: block 7 skipped: a CERTIFICATE block with PEM headers, which the relay's trust pool does not take
    [warden] upstream CA /etc/warden/bundle.pem: skipped after block 122: text that begins a PEM block and does not complete one
    ```

    A file of a named directory that gives nothing:

    ```
    [warden] upstream CA /etc/warden/roots/corp.cer: skipped: DER-encoded, and only PEM is read (to convert: openssl x509 -inform der -in corp.cer -out corp.pem)
    [warden] upstream CA /etc/warden/roots/notes.crt: skipped: no PEM block in it
    [warden] upstream CA /etc/warden/roots/keys.pem: skipped: it holds 2 PEM blocks and none is a certificate that can be sent
    [warden] upstream CA /etc/warden/roots/pipe.pem: skipped: not a regular file
    ```

    - *The private key line is stronger than the others*: it is a
      `warning:`, the only one here, and it says in so many words that the
      key is not sent. Every other skip is a root that was not added; this
      one is a secret in a place `warden` was pointed at, often by a file
      in a repository. A block is a private key when its type ends in
      `PRIVATE KEY` (`PRIVATE KEY`, `RSA PRIVATE KEY`, `EC PRIVATE KEY`,
      `ENCRYPTED PRIVATE KEY`, `OPENSSH PRIVATE KEY`). It is still a skip,
      not an error: the answer lists "a key beside a certificate" among
      what is skipped.
    - *The third line of the second group* follows the block lines of the
      same file, which say why each block was not sent.
    - *DER* is recognised when a file with no PEM block parses whole as
      one certificate; any other file with no PEM block, an empty one
      included, gets "no PEM block in it".
    - *Not a regular file*: a FIFO, a socket or a device under one of the
      three endings is not opened. Today `warden` opens it, and on a FIFO
      waits for ever.
    - *Text outside PEM blocks* that begins no block is not sent and gets
      no line. It is the one thing dropped without a word: it is not a
      file and not a certificate, a distribution's bundle carries a
      comment before every block, and none of it leaves the host.
    - A block's type and a file's name are printed with control characters
      escaped. Both can come from the repository being built.

    **What is an error**, each a `driver.FixableError` (UX-04), exit 1
    (UX-09), raised before the output directory, the work directory or the
    relay exists. The paths are taken in order and the first error stops;
    the lines of the paths before it, and the skip lines of the path that
    fails, have been printed.

    A configured path that does not exist:

    ```
    ERROR: upstream CA path /etc/warden/roots.pem (from [relay] upstream_ca_certs) does not exist
      To fix: name an existing file or directory of PEM certificates, or remove the path
    ```

    A configured path that cannot be read: a file that cannot be opened, a
    directory that cannot be listed, or a path that is neither a regular
    file nor a directory ("is not a file or a directory"):

    ```
    ERROR: cannot read upstream CA path /etc/ssl/private (from WARDEN_UPSTREAM_CA_CERTS): permission denied
      To fix: make it readable by the user warden runs as, or remove the path
    ```

    A file of a named directory, under one of the three endings, that
    cannot be read (permission denied, a link to nothing). It is an error
    today, at the claim, and stays one: the answer makes "cannot be read"
    an error and names skipping for what is read and is not a valid PEM
    certificate, and a read that fails stops by default (GLOBAL-11):

    ```
    ERROR: cannot read upstream CA file /etc/warden/roots/corp.pem, in /etc/warden/roots (from [relay] upstream_ca_certs): permission denied
      To fix: make it readable, or take it out of the directory
    ```

    A configured path that gives no certificate at all. For a file named
    directly the reason is in the error, and for a directory the skip
    lines above it give the reasons:

    ```
    ERROR: upstream CA path /etc/warden/corp.cer (from [relay] upstream_ca_certs) gives no certificate to send: it is DER-encoded, and only PEM is read
      To fix: convert it (openssl x509 -inform der -in corp.cer -out corp.pem) and name the result, or remove the path
    ```

    ```
    ERROR: upstream CA path /etc/warden/site.key (from [relay] upstream_ca_certs) gives no certificate to send: it holds 1 PEM block and none is a certificate that can be sent
      To fix: name a file or directory that holds at least one PEM certificate, or remove the path
    ```

    ```
    ERROR: upstream CA path /etc/warden/roots (from WARDEN_UPSTREAM_CA_CERTS) gives no certificate to send: 2 files skipped, 4 other entries not read
      To fix: put at least one PEM certificate in a .pem, .crt or .cer file there, or remove the path
    ```

    (A directory with no entry under the three endings reads "it holds no
    .pem, .crt or .cer file".) The error is per path: a path that gives
    nothing is the wrong path, whatever the other paths give.

    An empty entry in the list (a doubled or trailing separator in the
    variable, `""` in the key). It is an error today, as a failed `stat`
    of an empty name, and stays one; it is never read as the working
    directory, which is what an empty entry of `PATH` means:

    ```
    ERROR: WARDEN_UPSTREAM_CA_CERTS has an empty entry
      To fix: remove the doubled or trailing separator
    ```

    **What is logged.** On every run that has a configured path:
    - *every skip*, one line each, as above;
    - *one line per configured path*, after the lines of its files, with
      the number of certificates and where the path was configured:

      ```
      [warden] upstream CA /etc/warden/corp.pem ([relay] upstream_ca_certs): 2 certificates to send
      [warden] upstream CA /etc/warden/site.pem ([relay] upstream_ca_certs): 1 certificate to send; 1 block skipped
      [warden] upstream CA /etc/warden/roots (WARDEN_UPSTREAM_CA_CERTS): 3 certificates to send from 2 files; 2 files skipped; 4 other entries not read
      ```

      A part whose count is zero is left out. "Blocks skipped" counts
      every block that was not sent, a private key included. "Other
      entries not read" is a12 (a).

    With `--verbose` (or `WARDEN_VERBOSE`, or `[output] verbose`), also:
    - *the per-file list of what is sent*, one line for each file of a
      named directory that gives a certificate, in name order among the
      skip lines (for a file named directly, the path's own line is that
      line):

      ```
      [warden] upstream CA /etc/warden/roots/a.pem: 2 certificates to send
      [warden] upstream CA /etc/warden/roots/b.crt: 1 certificate to send
      ```

    - *each other entry that was not read* (a12 (a)):

      ```
      [warden] upstream CA /etc/warden/roots/README: not read: its name does not end in .pem, .crt or .cer
      [warden] upstream CA /etc/warden/roots/old: not read: a directory
      ```

    With no configured path nothing is read and nothing is printed.

    The lines say "to send", not "sent": they are printed when the files
    are read, before the relay exists, and a build that stops before its
    claim has sent nothing. What they count is what the claim carries.

    **Where the lines go.**
    - `build` and `shell`: stderr, with the `[warden]` prefix, after the
      `config:` line (a4), which stays the first line, and before anything
      is created. `--quiet` does not remove them: it silences the echo of
      the guest's console, not `warden`'s own lines. Stdout is not
      touched.
    - `doctor`: in its report, on stdout, without the prefix, as its
      `config:` line and its build-link lines are: the report is what
      `doctor` is run for, and a line of it on another stream would be
      lost to `warden doctor > report`. The lines say "to send" there too.
      An error is reported the way `doctor` reports a refusal a build
      would get, and `doctor` then exits 1:

      ```
      upstream CA: BLOCKED: upstream CA path /etc/warden/roots.pem (from [relay] upstream_ca_certs) does not exist
      upstream CA: to fix: name an existing file or directory of PEM certificates, or remove the path
      ```

    - `clean`, `setup`, `image prepare` and the detached prepare use no CA
      path: they read none and print none.
    - No flag is added (UX-05): `--verbose` is the existing global flag,
      and there is none to silence a skip. The exit code says nothing
      about skips (UX-09): 0, or the build's own.

    **What works today and stops.** Less than under (a) as this design
    proposed it at `3c0caab`. A certificate with its key in one file, a
    named directory that also holds a DER file, and a bundle with a
    certificate Go cannot parse all keep building, as today; each now has
    its line, and the key no longer leaves the host. What stops:
    - *a configured path that gives no certificate*: a DER file, a key
      file or a wrong file named directly; a directory with no `.pem`,
      `.crt` or `.cer` file, or with none that holds a certificate. Today
      the build runs, with no root added and, for a file, with the file's
      bytes sent to the relay;
    - nothing else. A path that does not exist, a path or a directory's
      file that cannot be read, and an empty entry are errors today, at
      the claim; they are the same errors, before anything is created.

    On this host the distribution's bundle parses whole (122 of 122
    blocks sent, none skipped; Go 1.26.3), and `/etc/ssl/certs` as a
    directory gives no skip line: 124 files read, each a certificate, and
    124 other entries (123 hash-named links, one subdirectory).

    The same function serves the standalone `cmd/collector`
    (`--upstream-ca`, a comma-separated list), which gets the same rule
    and the same errors. It prints the skip lines and the per-path lines
    on stderr, each path's source given as `--upstream-ca`; it has no
    `--verbose`, so it prints no per-file list. It is not in the
    reference.

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
    was not checked). It is filed as its own security finding, in
    `workdir-in-context` (**What this item does not do**), because the flag
    does the same and the answer is in the drivers.
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
`goBuildLinux` (the relay image, `warden-io` and the holder built from
source) and by `relayvm.Find` (the relay VM's kernel and initramfs for qemu
and Firecracker, when they are not beside `warden`). Today it walks up from
the executable's directory and then from the working directory, taking the
first `go.mod` that *contains* the module's declaration.

Who reaches it. The lookup does not depend on the version: any `warden`
that does not find what it needs beside its executable asks for the tree.

- *A development `warden`* (version `dev`), for everything: the relay
  image, `warden-io`, the holder, the guest binaries, the relay VM assets.
  Inside its tree (`dist/warden`) the first step finds it. Outside (`go
  install`, `go run`, a copy) the second step takes the working
  directory's.
- *An installed release with a VM driver.* `GuestBinary` looks beside the
  executable for `warden-io-linux-<arch>`, `warden-relay-linux-<arch>` and
  `warden-io-windows-<arch>.exe`. The release archive carries the Linux
  ones as `warden-io` and `warden-relay` (`.goreleaser.yaml`), `install.sh`
  installs `warden` and `warden-io` only, and no release carries the relay
  VM assets. So `--driver qemu` and `firecracker`, and `vz` as
  `install.sh` installs it (the archive has the two macOS binaries under
  the names looked for; the script does not install them), go to
  `ModuleRoot` for a guest binary, and today that is the working
  directory's tree. Under `vz` the relay built that way runs on the host
  (`startHostRelay`). There is no Windows release, so `hyperv` is always a
  build from source.
- *An installed release with `relay_image = "dev"`* (or
  `WARDEN_IMAGE=dev`): the container driver builds the relay from source
  whatever the version (`resolveRelayImage`), from the same tree.

In each case a repository that declares the module supplies the source
that `go build` compiles on the host into the relay or the executor. With
the default `./warden.toml`, the repository can also ask for it: `driver =
"qemu"` or `relay_image = "dev"`.

After:

```
ModuleRoot():
    walk up from the directory of the warden executable
        a go.mod with the line "module github.com/buildwarden/buildwarden" -> that directory
    else -> a fixable error
```

- The working directory is not looked at. A repository that declares the
  module is not `warden`'s source unless `warden` was built into it.
- The module line is matched whole. Today a nested module of this
  repository (`tools/tier1`) or a sibling whose path starts the same would
  match.
- When it is not found, `ModuleRoot` itself returns the `FixableError`.
  `GuestBinary` and `goBuildLinux` already wrap its error with `%w`, and
  `main` finds a `FixableError` through the wrapping (`errors.As`), so the
  callers say what they were doing and no file of `driver/container` is
  edited. The message names the executable and its version and does not
  call a release a development build:

  ```
  ERROR: building ./cmd/relay from source: /home/you/go/bin/warden (version dev) is not inside a buildwarden source tree
    To fix: run 'make build' in warden's source tree and use its dist/warden
  ```

  ```
  ERROR: warden-io-linux-amd64 not found next to warden, and no source tree to build it from: /usr/local/bin/warden (version 0.2.0) is not inside a buildwarden source tree
    To fix: run 'make build' in warden's source tree and use its dist/warden
  ```

  The fix is the one that is true in every case. "Install a release" is
  not offered: it is a fix only for the container driver. A release asked
  for `relay_image = "dev"` gets the first form; the reference's Relay
  Image section says that `dev` needs a `warden` run from its tree.
- `relayvm.Find` already treats "no tree" as "look beside the binary only";
  that stays, and its caller's existing message about missing assets is
  the one shown.
- What stops working, beside the attack: `go run ./cmd/warden` and a `go
  install`ed development `warden`; and an installed release used with a VM
  driver, or with `relay_image = "dev"`, from inside a buildwarden
  checkout, which works today only because the checkout is the working
  directory. The installation guide's "First-time QEMU Setup" builds the
  relay VM assets in a checkout and does not say to build or run
  `dist/warden` there; it gains `make build` and `dist/warden` (**Reference
  docs**).
- Tests: a `go test` binary lives in a temporary directory, so a test that
  reaches `ModuleRoot` relies on the working-directory step today. Run on a
  copy of `ad4ea50` with that step deleted, `go test ./... -v` differs
  from `main` in four tests and in nothing else. Three fail:
  `TestStageSeedDir` and `TestStageSeedDirConfiguredLink`
  (`driver/hyperv/winseed_test.go`) and `TestWindowsSeedDirRunScript`
  (`driver/qemu/windows_test.go`). One stops running and says nothing:
  `TestDescribeBuildEvalImage` (`driver/hyperv/environment_test.go`) goes
  from PASS to SKIP, because its helper `stageTestSeed` skips when the
  guest binary cannot be staged. All four cross-build `warden-io` through
  `GuestBinary`. So:
  - `driver` gains one documented, test-only setter of the tree;
  - `driver/hyperv` and `driver/qemu` each gain a `main_test.go` whose
    `TestMain` sets the tree once for the package, found by walking up from
    the test's own directory, so no test names it by itself and a new test
    cannot forget to;
  - `stageTestSeed`'s `t.Skipf` becomes `t.Fatalf`, so a missing tree
    cannot hide a test again.

  Those are the only edits in a driver's tree: three test files, all run
  by CI on Linux. Tests behind `darwin` and `windows` build tags were
  searched for callers of `GuestBinary`, `relayvm.Find` and `goBuildLinux`
  and none was found; they were not run. No environment variable is added
  (a6).

**The split with `prebuilt-artifacts`** (a6). Jeff's answer is that
`warden` should need no source tree at all and compile nothing: it finds
its executor, its relay and the relay VM artifacts already built, on
`PATH` and then in its own directory, at or above a minimum version, and
building them is development tooling's work. He put that in its own
security item and kept this item's fix small.

- *What this item closes.* F9 as it was found: no `warden`, of any
  version and with any driver, takes the working directory's tree for
  its source. That is the hole a repository can open by being cloned and
  built.
- *What this item leaves.* `warden` still compiles. A `warden` whose
  executable is inside a tree that declares the module still runs `go
  build` on that tree (`GuestBinary`; `goBuildLinux` for a development
  version or `relay_image = "dev"`) and still takes the relay VM assets
  from it (`relayvm.Find`). The Security note says what that exposes
  until the other item merges (**What remains**, 8).
- *What `prebuilt-artifacts` does, after this item has merged.* As its
  queue item states it (workflow `85669f6`; its design is being written
  and nothing of it is cited here as designed): it removes
  `ModuleRoot`'s use by `warden` and every `go build` `warden` runs,
  which are `GuestBinary`'s cross-build, the container driver's
  `goBuildLinux` and the source build behind `relay_image = "dev"`, and
  `relayvm.Find`'s lookup in the tree; the Makefile builds and places
  the artifacts. With no caller of `ModuleRoot` left in `warden`, it
  also deletes what this item adds for the tests: the test-only setter
  of the tree and the two `main_test.go` `TestMain`s. This item's two
  error messages that say to run `make build` and use `dist/warden`,
  and its sentences in `CONTRIBUTING.md`, the installation guide and
  the Relay Image section, are that item's to replace.
- *What this item does for that.* What it adds for the tree is small
  and in one place each, so that it is deleted whole: one setter, one
  `TestMain` per package, no test that names the tree by itself. And
  its tier-1 case and corpus entry for F9 state the property that
  outlives both items: a `warden` outside any source tree, run in a
  repository that declares the module, runs no `go`.

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
    file**: one file, the three ways it is found, that the default is the
    working directory's file whatever `[path]` is, a named file that is
    missing, an empty file, the `config:` line, relative paths (a1) and
    that a `WARDEN_CONFIG` set for a machine should be absolute, that
    `warden.toml` in a message means the config file in use (Decision 13),
    and a paragraph for someone building a repository they do not control:
    name the file; what a `./warden.toml` there decides; that under
    `sudo` the file is named with `--config`, or with `sudo
    --preserve-env=WARDEN_CONFIG`, because `sudo` drops the variable; and
    that `warden` is installed outside the repository it builds (Security
    note, **What remains**, 8);
  - the transitional subsection "Moving from
    `~/.config/warden/config.toml`", under its marker, with the `sudo`
    case;
  - **Environment Variables**: `WARDEN_CONFIG`; `WARDEN_UPSTREAM_CA_CERTS`
    "replaces `relay.upstream_ca_certs`; paths separated by `:` (`;` on
    Windows)"; one sentence on the order (flag, variable, file, default);
  - **CLI Flags**: `--config`, Global;
  - the `[relay]` lines of the Full Reference and a short **Upstream CA
    paths** section beside them: a path is a file or a directory, and a
    directory means its `.pem`, `.crt` and `.cer` files; every valid PEM
    certificate is sent and nothing else is; what is skipped and why (a
    private key, another block, a DER file and how to convert it, a
    certificate that does not parse), named as a skip that does not stop
    the build (GLOBAL-10); the lines, and what `--verbose` adds; the
    errors (a path that does not exist, cannot be read or gives no
    certificate); and that the system roots are the relay's own, which
    the comment on `system_ca_bundle` ("host system CA bundle") is
    corrected to say;
  - the Relay Image section (what is refused; `dev` needs a `warden` run
    from its source tree);
  - the two sentences that name both files: the build link's "Precedence:
    the flag, then `./warden.toml`, then `~/.config/warden/config.toml`,
    then the default" and `[vm] dns`'s "`./warden.toml` wins over
    `~/.config/warden/config.toml`".
- `docs/book/src/guide/cli.md`: `--config <path>` under Global Flags;
  `warden doctor` (the `config:` line; the source label is the config
  file's name; the upstream CA lines, and that a CA path a build would
  refuse is BLOCKED); `--verbose` (it adds the per-file list of upstream
  CA certificates).
- `docs/book/src/guide/corporate-networks.md`: its three mentions of
  `~/.config/warden/config.toml` and "the two files" become the config
  file, with the `WARDEN_CONFIG` route for a machine-wide setting.
- `README.md`: the Configuration block's first comment (`# ./warden.toml,
  or the file --config or WARDEN_CONFIG names`) and its list of variables.
- `CONTRIBUTING.md`: one sentence, that a development `warden` is run from
  its tree (`dist/warden`).
- `docs/book/src/getting-started/installation.md`, "First-time QEMU
  Setup": `make build` before the two asset scripts, and that the VM
  drivers are run as `dist/warden` from the tree (the Firecracker section
  already starts with `make build`). These two, and the Relay Image
  section's sentence on `dev`, are true from this item's merge until
  `prebuilt-artifacts` merges, which rewrites them (a6).
- `cmd/warden`: the help text of `--config`; `doctor`'s long help where it
  lists the build link's sources; the two messages that name the file for
  a bad `[vm]` value (Decision 13). The drivers' fix strings that say "in
  warden.toml" are not edited.
- `docs/book/src/concepts/overview.md`, in `warden build`, in place of the
  `req := resolve flags and config` line:

  ```
      cfg := the one config file: --config, else WARDEN_CONFIG, else
             ./warden.toml if it exists, else none; a named file that
             cannot be read is an error
      req := resolve each setting: flag, else its variable, else cfg,
             else the default (driver, runtime, output dir, ...)
      req.UpstreamCACerts := the valid PEM certificates under each
             upstream CA path (WARDEN_UPSTREAM_CA_CERTS, else [relay]
             upstream_ca_certs), read once, before any driver work; each
             skip and each path's count logged; a path that is missing,
             unreadable or gives none is an error
  ```

  and, in the claim's line of `StartBuild`, "upstream CAs" reads "the
  upstream CA certificates of the request".

  `runtime-cli-project-path` adds a `container driver: runtime := ...` line
  there that says "(`./warden.toml`, then the user's file)"; it becomes
  "(the config file)".
- `install.sh` names no config file and does not change; nor does the rest
  of `installation.md`.

### Compatibility (UX-03)

No flag, subcommand or key is removed or renamed, so UX-03's alias does not
apply as written and tier 1's `surface-removed` check sees nothing. Four
things that work today stop: the user-level file; the sum of the variable
and the key for CA paths; a CA path that gives no certificate at all (F2
lists the cases; a path that gives some keeps working and says what it
skipped); and a `warden` that takes its source tree from the working
directory (a6 lists who). UX-03's "never a
silent break" is met for each by an error that says what to change, at
once and not after a warning period, as Jeff decided for
`runtime-cli-project-path`. The first two errors are transitional; the
other two are the permanent checks' own messages.

### How it sits with the other items

The order of merging: `runtime-cli-project-path`, then this item, then
`driver-contract` (K1) and `prebuilt-artifacts`, each of which waits for
this item and neither for the other as far as this design goes. The two
sibling designs that exist say the same order for the first three
(`runtime-cli-project-path` at `119a0e1`, **One config file**;
`driver-contract` at `a6c8f0b`, Decision 23).

**`runtime-cli-project-path` first.** Its design is approved (r1 at
`a2afd08`) and it is being implemented (its branch at `8e41bd8`). It is
small, and it closes host execution
through `[runtime] cli` without waiting for this. What each owes the
other:

- This item keeps its rule and its code: the runtime is `finch`, `docker`
  or `podman` from every source, checked in `resolveRuntime`; its
  transitional file `runtime_former.go` and marker stay untouched, and are
  kept until 1.0 (its a4), as this item's are (a3).
- This item reworks what that one writes for two files, which its design
  lists: the fourth row of its rule's table, the marker step in
  `LoadConfig` that tells whether the project's file set the key, "else
  the user's" in `resolveRuntime`'s comment and the reference, the error
  that names the user's file by its path, its overview line, and the tests
  of those. Its tests' temporary `HOME` stays, now so that no test meets
  the transitional error.
- UX-08: that item amends it, and the wording its review approved says
  "flags and environment variables over file configuration, in the order
  UX-07 gives". It names no level of config file, so this item changes no
  word of UX-08 (**Rule changes**).
- The hold (a7, answered (a)): a blocking question on this item that
  names that item's merge, "resume when `runtime-cli-project-path`'s PR
  has merged". It is the review session's to raise, when it records this
  design's approval, with the `human` step that `f2-plan` then needs; this
  agent writes no step and no blocking question into the queue file. Until
  it is raised nothing holds `f2-plan` but the review step itself, so the
  session raises it in the same sitting as the approval, before the step
  is marked done. Jeff answers it once that PR has merged.

**`driver-contract` (K1) after.** K1 is written on one file and waits for
this item's merge before its Plan (its Decision 23). What each owes the
other:

- This item gives K1 one function that says which file was read and how
  (`configFile`), the `--config` flag and `WARDEN_CONFIG`, and no level.
  K1's request field `config_file` is that file's absolute path, absent
  when there is none.
- K1's quotation of UX-07 is stale. At `a6c8f0b` K1 still quotes ORCH-03
  and UX-07 as this design proposed them at `539fd38`. ORCH-03's proposed
  wording has not changed since, so that quotation is exact. UX-07's
  changed at `8ba86ad`: K1 quotes "any of them is easy to override at
  three levels with clear precedence", and the proposal has read "easy to
  override, at up to three levels with clear precedence" since then, with
  the sentence on knobs reworded (**Rule changes**). The sentence K1 leans
  on, "A value set at any level replaces the default and the lower
  levels' value (GLOBAL-10)", is as it was, so nothing K1 decides rests
  on the stale words. The proposal has not changed in this sitting
  either: both wordings are byte for byte as at `3c0caab`. K1 re-quotes
  when it next reads this design; this agent does not edit K1's.
- K1's ORCH-03 paragraph on drivers is added after the paragraph this
  item rewrites, and K1 changes no word of this item's sentences (its
  **Rule changes**).
- K1's a26 asked what becomes of its one guard, that a config file names
  a driver and never a path, now that there is no user's own config. It
  was answered (A) on 2026-11-15: no config file names a driver by path,
  in `./warden.toml` and in a named file alike. That agrees with this
  item's Decision 7 (what a file may decide is not limited by where it
  was found), which a4's answer confirmed; K1's risk that a defaulted
  `./warden.toml` might come to be limited is therefore closed. This item
  leaves `[runtime] driver` as the five names it is today.
- K1 moves `relay_image` and its check to the container driver; the CA
  check stays with the claim, which K1 gives to `warden`; the transitional
  file and the `config:` line are `warden`'s own and stay in `cmd/warden`.
  K1 keeps `[vm] build_link` and `[vm] dns` as `warden`'s keys, so the
  drivers' fix strings that name them keep meaning the one config file
  (Decision 13).
- This item's tier-1 cases run the built `warden` and must pass on K1's
  branch unchanged.
- K1's hold is K1's and is already in the queue: a blocking question
  that names this item's merge (its `q1`, with the step `f2-answer-q1`
  that its `f2-plan` needs), written by the attended session on
  2026-11-15, during K1's review and before its outcome (workflow
  `ea23718`). An earlier draft of this design said it would be raised
  when K1's review outcome was recorded; K1's design explains why it was
  raised sooner, and this design follows it.
- Which config file a build read is recorded nowhere but in the
  `config:` line. An earlier draft of this design left that to K1's
  build record. That record does carry `config_file`, but it lives in
  the lock directory, for `reap` and `attach`, and is removed when a
  build ends cleanly; `driver.json`, in the output, does not name the
  config file, and K1 adds nothing to the output for it (K1 at
  `a6c8f0b`, **What K1 takes from the two configuration items**). If the
  output is to name the file, that is a field for some item to add, and
  neither design proposes it.

**`prebuilt-artifacts` after.** Jeff's answer to a6. It is in Design, and
what follows is from its queue item and from this design's side only.

- This item gives it a `ModuleRoot` that no longer looks at the working
  directory, so the hole is closed on `main` while that item is built,
  and three test files and one setter that are written to be deleted
  whole (**F9: the source tree**, the split).
- It removes `ModuleRoot`'s use by `warden`, every `go build` `warden`
  runs and the tree lookup of the relay VM assets, and with them this
  item's setter, its two `TestMain`s, its two "run `make build`"
  messages and its three doc sentences about running `dist/warden` from
  the tree.
- This item's tier-1 case and corpus entry for F9 must hold on that
  item's branch: a `warden` outside any source tree, run in a repository
  that declares the module, runs no `go`. What it prints and how it
  exits there is that item's to say.
- Its merge needs this item's first: Jeff's "the new item then removes
  the tree lookup". How its queue item is held is its own to record,
  as K1's is.
- K1 is not held behind it. That is the reason Jeff gave for the split:
  taking every `go build` out of `warden` needs native validation of the
  VM drivers, and K1 waits only for this item.

**`configuration-bugfixes`** (queued, on hold behind the freeze) was
handed four observations, and **`workdir-in-context`** (a security fix,
priority 67, queued) was filed with the two security findings, both by
the review session on 2026-11-15 (a11; workflow `4c59842`; **What this
item does not do**). This item waits on neither, and neither waits on
it.

### What this item does not do

- It does not limit what a config file may set by where the file was
  found (Decision 7).
- It does not record in the build's output which config file was used,
  and nor does `driver-contract`: its build record names the file but is
  not part of the output and is removed when a build ends cleanly. The
  `config:` line is the only record (**How it sits with the other
  items**).
- It does not change `[runtime] driver`, or what `runtime-cli-project-path`
  decides for `[runtime] cli`.
- It does not move the load-time checks of `[vm] build_link` and `[vm]
  dns` to their consumers; UX-08 as amended lists them, and they are K1's
  or `configuration-bugfixes`'.
- It does not reword the drivers' messages that say "in warden.toml"
  (Decision 13).
- It does not change what a release ships. That a release has no Linux
  guest binary under the name `GuestBinary` looks for is how the VM
  drivers come to be run from a source tree, which the installation guide
  describes; this item only stops the working directory standing in for
  that tree.
- It does not remove compilation. `warden` still runs `go build`, on the
  tree its executable is in, and still takes the relay VM assets from
  that tree. `prebuilt-artifacts` removes both, after this item merges,
  and with them `ModuleRoot`'s use by `warden` and the two `TestMain`s
  and the test-only setter this item adds (a6; **F9: the source tree**,
  the split).

**Handed on.** This agent edits only its own queue item, so the hand-offs
were recorded by the session that holds this design's review (a11,
answered (a)). Each has a named home:

| What | Where it goes | Who records it, and when |
|---|---|---|
| `[relay] system_ca_bundle = false` with no CA path does nothing, silently (GLOBAL-10) | `configuration-bugfixes`, appended to its `source` | the review session; done 2026-11-15, workflow `4c59842` |
| `--no-compress` and `-v` cannot undo the file (`--no-compress=false`, `--verbose=false`), and `WARDEN_VERBOSE=0` turns verbose on | `configuration-bugfixes`, likewise | done, likewise |
| A mistyped local `[vm] windows_image` is sent to the HTTP client, whose error does not say the path was not found (UX-04) | `configuration-bugfixes`, likewise | done, likewise |
| The hyperv and qemu drivers read variables that are in no reference (`WARDEN_HYPERV_BUILD_IMAGE`, `WARDEN_RELAY_BOOT_VHDX`, `WARDEN_QEMU_VNC` and others), which UX-07 calls ad hoc before and after this item | `configuration-bugfixes`, likewise | done, likewise |
| **A work directory inside the build context is served to the build** (below) | the security item `workdir-in-context` (priority 67, queued), filed through intake | the review session, with Jeff; done 2026-11-15, workflow `4c59842` |
| **A Dockerfile's `FROM` value reaches the container runtime's command line unchecked** (below) | the same item's `source` | done, likewise |
| Removing compilation from `warden`: every `go build` it runs, `ModuleRoot`'s use, the tree lookup of the relay VM assets, and this item's test-only setter and `TestMain`s (a6) | the security item `prebuilt-artifacts` (priority 66, in Design), after this item merges | the review session, on Jeff's word; filed 2026-11-15, workflow `4c59842` |
| The removal of the transitional detection, at 1.0 (a3) | its own small queue item | Publish |
| The corpus entries of the Security note | `security-suite` (SEC-03) | Publish |

None of the first four is a security matter. The two in bold are, and
neither is a regression (so FLOW-09 does not file them) and neither is
held by the freeze (FLOW-20 exempts a security fix):

- **A work directory inside the build context.** `--tmpdir` or `[build]
  tmpdir` can place each build's work directory under the context
  directory (`tmpdir = "."`, or `--tmpdir .`), and the collector serves the
  whole context root to the build (`/v1/context/`). What each driver keeps
  in its work directory that a build should not read was not traced (a
  relay VM's seed carries the claim token, `tools/relay-vm/README.md`;
  whether it is kept in the work directory was not checked). The flag does
  the same as the key, and the answer is in the drivers.
- **`FROM` as an option.** The translator takes the first field after
  `FROM` as the image (`parseFrom`, `driver/script/translate.go`), and the
  container driver passes it to `<runtime> pull <value>`, `image inspect
  <value>` and, in an option's position, `container create ... <value>
  initialize ...` (`driver/container/container.go`, `environment.go`). So
  `FROM --platform=linux/amd64 ubuntu`, which is ordinary Dockerfile
  syntax, makes the image `--platform=linux/amd64`, and a build context
  can put an option on the host runtime's command line. Read, not run: as
  far as read, `pull` with an option and no image fails before the later
  commands, which would make this a wrong error and not an escape; that
  is for the item to establish. It is a build input, not configuration,
  and it is the same defect as the relay image's (F1), which this item
  fixes for the relay image only.

## Defaults and overrides

"Absent" below means: no flag, the variable unset or empty, and the key not
in the config file (or no config file).

| Default, override or fallback | Status | Why it is the obvious choice (GLOBAL-09) | What overrides it, exclusively (GLOBAL-10) |
|---|---|---|---|
| The config file when none is named: `./warden.toml` of the working directory (not of the build context `[path]` names), if it exists | **Changed**: it was that file over `~/.config/warden/config.toml` | Jeff's decision 2: the working directory, for ease of use | `--config`, else `WARDEN_CONFIG`; the named file is the only one read, and a named file that cannot be read is an error, not a return to the default |
| No `./warden.toml` and none named: no config file | **Changed**: the user file was read | A first run needs no file (UX-07) | As above |
| `~/.config/warden/config.toml` | **Removed** as a source. Its existence is an error while the transitional detection stands (a3), unless it is the file named (a10) | Jeff's decisions 1 and 4 | Nothing reads it; `--config` or `WARDEN_CONFIG` can name it |
| An empty `WARDEN_CONFIG` | **New**: not set | What every empty `WARDEN_*` variable means today | Nothing (a8) |
| `--config` given with an empty value | **New**: an error, not "not set" (a8). Unlike an empty `--driver` or `--runtime`, which are not set | The next source down can be someone else's `./warden.toml` | Nothing |
| A relative path in the config file: relative to the working directory | **Unchanged**, stated for the first time (a1) | The same as the path given by a flag or a variable | An absolute path |
| Every setting: flag, else its variable, else the file, else its default | **Unchanged** in the code; the rules now name the variable's level | The closer to the command, the stronger | Each level by the one above it |
| `[relay] upstream_ca_certs` absent: the system roots only | Unchanged | | `WARDEN_UPSTREAM_CA_CERTS`, else the key |
| A CA path that is a directory: its `.pem`, `.crt` and `.cer` files, one level deep, links followed | **Unchanged** in which files are read | What the reference says a directory is | Nothing |
| What is sent of a CA file: its valid PEM certificates | **Changed** (a9): the whole file was sent, whatever it held | A trust root is a certificate; nothing else under the path has a use in the relay | Nothing |
| A block or a file under a CA path that is not a valid PEM certificate (a private key or another block, a DER file, a certificate that does not parse): skipped, and the run goes on | **New, and named as a continuation on failure** (a9, Jeff's answer; below). On `main` the relay drops the same things without a word | The builds that run today keep running | Nothing: no flag makes a skip an error. A path that gives no certificate at all is an error |
| The upstream CA lines: every skip, and one line per configured path with its count, on every run; the per-file list with `--verbose` | **New** (a9) | A root that was not added should not need a flag to learn; the list of what was added is long and rarely wanted | `--verbose`, `WARDEN_VERBOSE` or `[output] verbose` for the list; nothing silences a skip |
| The entries of a CA directory that are not read by name: counted on the path's line, listed with `--verbose` | **New** (a12, open) | 124 such entries in a system certificate directory | `--verbose` for the list |
| The system roots: the relay's own, with no line and no count on the host | **Unchanged** | `warden` reads no host file for them | `system_ca_bundle = false`, with a CA path |
| `WARDEN_UPSTREAM_CA_CERTS` with the key also set | **Changed**: it replaced nothing and was added to the key's list; now it replaces it (a5), and both set is an error, on the commands that use the paths, while the transitional detection stands | One rule for every variable | Nothing is over the variable; the key has no flag |
| The separator of `WARDEN_UPSTREAM_CA_CERTS` | **Changed**: `:` everywhere; now the OS's (`;` on Windows) | What `PATH` uses on each OS (GLOBAL-07) | Nothing |
| The relay image absent: the release image, or a source build for a development `warden` | Unchanged | | `WARDEN_IMAGE`, else `relay_image` |
| `warden`'s source tree, for any version that needs it: found by walking up from its executable | **Changed**: the working directory was tried second (a6). It lasts until `prebuilt-artifacts` merges, after which `warden` looks for no tree | A binary's source is where the binary was built | Nothing |
| The `config:` line: printed by every command that reads configuration | **New** (a4) | Which file is in force should not need a flag to learn | Nothing; `doctor` prints it on stdout |
| A background `image prepare`: the file its build read | **Changed**: it looked in the working directory by itself | One build, one file | Nothing |

**Fallbacks and merges.** Two merges and one fallback are removed, and
one continuation on failure is added and named (below):

- the merge of the user's file under the project's (two files decoded into
  one struct) is gone;
- the merge of `WARDEN_UPSTREAM_CA_CERTS` into the key's list is gone;
- the fallback, `ModuleRoot`'s second place to look (the working
  directory, tried when the first found nothing), is gone, and the design
  drops it as a rejected alternative (Decision 9).

The lookup of the config file is an ordered choice among what is *set*,
not a fallback on failure: a source that is set and cannot be used is an
error. One case does pass to the next source without a word and is named
here: an empty `WARDEN_CONFIG` counts as not set (a8). The transitional
detection is not a fallback: it only produces errors.

**The skip under a CA path, named** (GLOBAL-10, GLOBAL-11). What triggers
it: a PEM block or a file under a configured CA path that is not a valid
PEM certificate (the reasons are F2's lines). What happens: it is not
sent, a line says so, and the run goes on with the certificates the path
does give. Honestly placed against the two rules:

- It is not GLOBAL-10's fallback. Nothing is tried in place of what was
  skipped, no default stands in for it, and a path that gives nothing is
  the error the rule asks for ("a provided value that cannot be used is
  an error that names it and its fix").
- It is what GLOBAL-11 allows only when named: a path that could not be
  used in full does not stop the run. So it is named, here and in the
  reference (UX-01), with its trigger.
- What keeps it from being silent: a line for every skip on every run,
  without `--verbose`; a count on the path's line; a `warning:` for a
  private key; an error when a path gives nothing.
- What it costs, and the lines do not undo: a build can run without a
  root its executor believes was added, and the notice is a line among a
  build's output. The failure then shows later, as an upstream TLS error
  inside the build. Option (a) would have stopped the build at the
  start. Jeff chose the lines.
- It is not new behaviour for the build: `main` runs these builds too,
  and says nothing. `warden` now skips first, so nothing reaches the
  relay to be dropped there; the relay's own silent drop is unchanged for
  any other claimant, and is `seam-specs`' to record (its D5).

One thing under a CA path is dropped with no line: text outside PEM
blocks that begins no block (F2).

No relay setting is added or moved (RELAY-08). The upstream trust roots
keep their claim field, `upstream_ca_certs`, and `upstream_system_ca`; the
relay image is the container driver's launch argument, as today.

## Cross-driver impact

No file of a `driver/<name>/` tree that is built into `warden` is edited
(ORCH-01), the drivers' fix strings that say "in warden.toml" included
(Decision 13). Three test files are: a new `main_test.go` in `driver/hyperv`
and in `driver/qemu`, which names the source tree once for the package,
and `driver/hyperv/environment_test.go`, whose helper fails where it
skipped (**F9: the source tree**). Four shared files of `driver/` are
edited (`hostutil.go`, `session.go`, `detach.go`, and `driver.go` for the
request's CA field), so every driver is declared:

- **container**: the relay image in effect is checked before anything is
  created. A build from source (a development `warden`, or `relay_image =
  "dev"`) finds its source from the executable only (`goBuildLinux`, which
  is not edited: `ModuleRoot`'s error carries the fix). CA paths are read
  once, before the driver is asked for anything, and the claim carries
  their valid PEM certificates; no driver reads a CA file or prints a CA
  line.
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

Two entries. ORCH-03 and UX-07 are project invariants of the component
this item works in (`cmd/warden`), not loop rules (FLOW-14, LOOP-09). The
current wording is quoted from `docs/development/` on `main` at `ad4ea50`.
UX-08 is not changed; why is below.

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
> any of them is easy to override, at up to three levels with clear precedence,
> command line > environment > config file (`flag > environment variable >
> warden.toml`, the same order as ORCH-03). There is one config file for a run, in
> the control of whoever runs the build: `warden.toml` in the working directory by
> default, or the file `--config` or `WARDEN_CONFIG` names, which is then the only
> one read. No user-level or system-level file is read. A first-time user should get
> a sensible run with little or no config. Driver and build knobs are flags and
> config keys, with an environment variable where one is documented and never an
> ad-hoc one; a knob need not have every level, and each level it has takes part
> in that precedence and appears in the reference docs. A value set at any level
> replaces the default and the lower levels' value (GLOBAL-10).
>
> The automated part checks that defaults resolve with no config file present.

Four things change: the levels are the command line, the environment and
the config file (they were invocation, project and user); "at three
levels" becomes "at up to three levels", and the sentence on knobs says a
knob need not have every level; the two sentences on the one file are new;
and the sentence on knobs admits a documented variable, because the
environment is now a level of the rule and six knobs have had one all
along.

Why "up to". The current rule's three levels were two files and the
command line, and every key had both files, so each could be set at two
levels at least. With one file, a key that has neither a flag nor a
variable can be set at one level only. Of the twenty keys in use, six
have a variable, fourteen have none, and five have neither flag nor
variable (`[vm] dns`, `[relay] system_ca_bundle`, the two image ages,
`[vm] windows_image_update`); four have all three levels (**Who is
affected**, the second table). "Any of them is easy to override at three
levels", kept as it is, would be false of `main` on the day this merges
for sixteen keys, and would read as requiring a2's option (c), a variable
for every key. a2 was answered (a), none added, so some knobs have fewer
levels; the wording was written to hold under each of a2's options, and
the answer changes no word of it.

### UX-08, not changed

Path: none. `runtime-cli-project-path` amends UX-08 and merges first; its
review approved the change by name at `a2afd08`. An earlier draft of this
design declared one clause of it, because that item's wording then said
"the config files in precedence order". The wording that was approved
says:

> after the overrides are applied (flags and environment variables over file
> configuration, in the order UX-07 gives)

and "A `warden.toml` and the environment may hold the settings of several
drivers", "A config file that cannot be parsed is an error for every run
that reads it". Nothing in it names a level of config file or more than
one file, so it reads the same over one file, and that design says so
(its **One config file**). This item's `rule_changes` therefore has no
UX-08 entry.

What UX-08 as amended asks of this item, which adds or touches four
checks:

| Check | Where, and when | Under UX-08 as amended |
|---|---|---|
| The relay image's shape | `newRequest` and `doctor`, on the value in effect, under the container driver only | conforms: after the overrides, by the commands that use it |
| CA paths: read once; a path that is missing, unreadable or gives no certificate is refused | `resolveUpstreamCAs`, from `newRequest` and `doctor`, on the paths in effect | conforms, likewise; `clean`, `setup` and `image prepare` do not check them. A skip is not a check that fails: it is logged and the run goes on |
| The variable and the key both set (a5, transitional) | the same function | conforms, likewise. Raised as the file loads it would fail three commands for a setting they do not use |
| The config file itself: named and unreadable; not TOML; `--config` empty | `LoadConfig`, every command that reads configuration | conforms: "a config file that cannot be parsed is an error for every run that reads it", and the file is what every such run reads |

The transitional error for `~/.config/warden/config.toml` validates no
setting (nothing in that file is read), so the rule's list of older checks
gains nothing: "Nothing is added to the list" holds.

### Severity and check, before and after

ORCH-03 and UX-07: MUST, `automated + agent-judgment`, unchanged.

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

UX-07's "at up to three levels" is true of every key: each has the file;
thirteen have a flag, six a variable, four both; five have the file only.
Each flag and each of the six variables is in the reference. Had the rule
kept "at three levels", sixteen keys would not conform (above).

Three things on `main` sit badly with UX-07 before this item and equally
after it, and are not made worse: the two one-way flags and
`WARDEN_VERBOSE` (a level that can turn a setting one way only), and the
drivers' undocumented variables ("ad-hoc environment variables"). They
are in `configuration-bugfixes`' `source` (a11).

### How the items' rule changes sit together

| Item | Rule | File | Merges |
|---|---|---|---|
| `runtime-cli-project-path` | UX-08 (rewritten; approved), and its line in `tools/tier1/rules.yaml` | `ux-cli-config.md` | first |
| this item | ORCH-03; UX-07 | `orchestrator.md`, `ux-cli-config.md` | second |
| `driver-contract` | ORCH-03 (a paragraph on drivers, and the check's reach), GLOBAL-08, ORCH-01, ORCH-02 and others | `orchestrator.md` and others | third |

UX-07 and UX-08 are neighbouring sections of one file; the two items edit
different rules. UX-08 as amended points at UX-07 for the order ("in the
order UX-07 gives"), and UX-07 as amended here gives it: command line,
environment, config file. K1's ORCH-03 text is added after the paragraph
this item rewrites.

**Not changed: UX-03.** Its text is about flags, subcommands and keys, and
none is removed. Jeff's decision that a change which refuses what worked
before is an error at once, with a transitional message, is applied here as
it is in `runtime-cli-project-path`, which does not propose to write it
into UX-03 either. If it is to be a rule, it is that rule's, and a question
for the review of whichever item is asked first.

**The profile (PROFILE-01).** No entry goes stale: its Reference documents
row points at the `surface` entry of `rules.yaml`, which does not change.

The design review approves each of the two by name (FLOW-19).

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

F9 is not limited to a development `warden`. The lookup is the same for
every version, and three kinds of `warden` reach the working directory's
tree today (**F9: the source tree**): a development build outside its
tree, with any driver; an installed release with `--driver qemu` or
`firecracker`, or `vz` as `install.sh` installs it, for the guest binaries
(under `vz` the relay so built runs on the host); and an installed release
with `relay_image = "dev"` or `WARDEN_IMAGE=dev`. Each needs `go` on the
host's `PATH` for the binaries; the relay VM's kernel and initramfs are
taken from the tree as files. With the default `./warden.toml` the repository can select
the second and third itself (`driver = "qemu"`, `relay_image = "dev"`).

**How the posture changes.**

- *Tightens:* no `warden`, development build or release, takes the working
  directory for its source tree (F9, closed for every driver and every
  version). A value that would be read as an option of the container
  runtime is refused (F1). Only valid PEM certificates are sent to the
  relay as trust roots: a private key, and any other byte under a CA
  path, stays on the host. The paths are read once, before anything is
  created, so what was checked is what is sent (F2). An
  executor can now make `warden` ignore the working directory's
  `warden.toml` wherever it is run, with `--config` or `WARDEN_CONFIG`.
  Before this the only way was to run `warden` from another directory and
  give the repository as `[path]`: the file has always been the working
  directory's, not the build context's.
- *Changes shape:* the user-level file is gone, so nothing "outranks the
  user's file". The environment is the level over the file.
- *Not tightened, by the answer to a9:* a CA path with a root that
  cannot be used (a DER file, a certificate that does not parse) still
  builds without that root, as on `main`; it now says so. A skip fails
  closed: the relay trusts fewer roots than the executor meant, never
  more, and a path that gives none stops the build.
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
   `WARDEN_IMAGE` pins it), the relay's trust roots (certificates only,
   but any certificate the user can read: a path in the repository's file
   can name a directory of the host, and the certificates there go to the
   relay the same file chose; `WARDEN_UPSTREAM_CA_CERTS` pins the paths),
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
4. **`WARDEN_CONFIG` does not survive `sudo`.** `sudo` resets the
   environment by default, so `sudo warden image prepare --driver vz`,
   `sudo warden setup --driver firecracker` and any other `sudo warden`
   run in a repository reads that repository's `./warden.toml` although
   the executor named another file in the machine's environment. The
   protection there is `--config`, or `sudo
   --preserve-env=WARDEN_CONFIG`, or a sudoers `env_keep`. `warden` does
   not detect this in general; the `config:` line shows which file was
   read, and the transitional message says it when it fires under `sudo`.
5. **Two ways a named file is not the one read.** An empty
   `WARDEN_CONFIG` is not set (a8), so `WARDEN_CONFIG="$CFG" warden build`
   with `CFG` unset reads `./warden.toml`; and a relative `WARDEN_CONFIG`
   names a different file in each directory, which in a repository can be
   the repository's own. A machine-wide value should be absolute. The
   `config:` line shows the file in both cases.
6. **A relative path in a named file** is looked up in the working
   directory (a1 (a)), so a relative CA path in an executor's own file
   reads certificates from the repository being built.
7. **A work directory inside the build context, and a `FROM` value read as
   an option** (the two findings filed as `workdir-in-context`, a11).
   Neither needs a config file, and both stay open until that item
   merges.
8. **`warden` still compiles, from the tree its executable is in, until
   `prebuilt-artifacts` merges** (a6). This item stops the working
   directory standing in for that tree and nothing more. Three things
   stand until that item removes compilation:
   - A `warden` built in a tree is as trusted as that tree. Unchanged:
     running `dist/warden` from a checkout runs the host's `go build` on
     that checkout's `cmd/relay` and `cmd/warden-io`.
   - A `warden` placed inside the repository being built is inside that
     repository's tree: a CI job that unpacks a release into the
     checkout's `./bin`, or a repository that ships the binary. If the
     repository declares the module, the walk up from the executable
     finds the repository's `go.mod`, and with a VM driver, or with
     `relay_image = "dev"`, which the repository's own `warden.toml` can
     set, `warden` builds the relay or the executor from the repository.
     Keep `warden` outside what it builds; the reference says so.
   - The `go` that compiles is the first on `PATH`, which is the
     environment's (3).
9. **The upstream CA lines name host files.** They print, into the
   build's log, the path of each file skipped and why (that it holds a
   private key, for one), and the number of certificates under each
   path. No content of a file is printed, and names and block types are
   escaped. With the default file in someone else's repository the paths
   are the repository's choice, so the log of such a build can say which
   files of a host directory end in `.pem`, `.crt` or `.cer` and which
   hold a key. The log is the executor's.

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
  a marker: `warden` exits 1 and the marker is absent; and the same with a
  release-versioned `warden` and a context `warden.toml` that sets
  `relay_image = "dev"`;
- (iii) `relay_image` starting with `-`, from the file and from
  `WARDEN_IMAGE`: exit 1, and a fake runtime on `PATH` records no `pull`;
- (iv) `upstream_ca_certs` naming a file that holds a known token and no
  certificate: exit 1 before any container exists, and the token is in no
  claim; and naming a file that holds a certificate followed by a
  `PRIVATE KEY` block whose body carries a known token: the build is not
  stopped, stderr has the `warning:` line, and the token is in no claim
  and in no file of the output.

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
  `./warden.toml` beside it is not used; `--config ""` is the error and
  the `./warden.toml` beside it is not used; an empty variable is not set;
  an empty file gives the defaults; a relative `--config`; a `[path]`
  argument naming a directory that holds a `warden.toml` does not make
  that file the one read.
- *The `config:` line:* its four forms; on stdout for `doctor`, on stderr
  for the others.
- *The transitional error:* with the user file present, each of the six
  commands' config load fails with it, naming the path; with `--config` or
  `WARDEN_CONFIG` naming another file, the same; with either naming that
  file, no error (a10); with no home directory, no error; the file is
  never opened (it is unreadable in the test); a named file that does not
  exist is reported in its place; with `SUDO_USER` set the fix ends with
  the `sudo` sentence, and without it does not.
- *The precedence table:* for every key in **The precedence, key by key**,
  the flag over the variable over the file over the default, where each
  level exists.
- *CA paths:* the variable alone, the key alone; both set is the
  transitional error in `newRequest` and in `doctor`, and is not an error
  for the config load of `clean`, `setup` and `image prepare`, which
  succeed with both set; the variable is split on the OS's separator (a
  Windows-only test for `;` and a drive letter); an empty entry is the
  error; a missing path, and a path that gives no certificate, each fail
  in `newRequest`, before any directory is made, and make `doctor` print
  BLOCKED and exit 1.
- *The upstream CA lines:* for a directory holding a certificate, a
  certificate with its key, a DER `.cer` and a `README`, `newRequest`
  succeeds and stderr has, in order, the `warning:` line, the DER skip
  line and the path's line with its counts; with `--verbose` also the
  per-file lines and the `README` line; without it neither; `--quiet`
  changes nothing; nothing is written to stdout; `doctor` prints the
  same lines on stdout without the prefix; with no configured path
  nothing is printed; the lines follow the `config:` line.
- *The relay image:* empty, `dev` and references pass; values starting
  with `-`, holding a space, or holding a control character are refused,
  from the file and from the variable, by `newRequest` and by `doctor`
  under the container driver; not checked under a VM driver.
- *Messages:* a bad `[vm] image_refresh_after` and a bad `[vm]
  windows_image_update` name the path of the file read (Decision 13).
- *Exit codes:* each new error is a `FixableError`, which `main` exits 1
  on (UX-09).

In `driver`:

- `LoadUpstreamCAs`, table-driven, each case asserting the bytes
  returned, the report (per path and per file: certificates, skips with
  their reasons, other entries) and the error:
  - *sent:* a certificate; a bundle with comments between blocks, of
    which only the blocks come back, re-encoded; the same certificate in
    two files, returned twice;
  - *skipped, with the certificate beside it still returned:* a private
    key block of each of the five types, and no byte of the key in what
    is returned; a block of another type; a certificate block that does
    not parse; a certificate block with PEM headers; a block that is
    begun and not completed;
  - *skipped, in a directory that still gives a certificate:* a DER
    `.cer`; a file with no PEM block; an empty file; a file whose blocks
    are all skipped; a FIFO named `x.pem`, which is not opened (Unix);
  - *not read:* a subdirectory, a link to a directory and a file of
    another ending, each counted and none opened;
  - *errors, with nothing returned:* a path that does not exist; a file
    that cannot be read; a directory that cannot be listed; a file of a
    directory that cannot be read, and a link to nothing there (Unix,
    not as root); a path that is neither a file nor a directory; a DER
    file, a key-only file and an empty file each named directly; a
    directory with no `.pem`, `.crt` or `.cer` file; a directory whose
    files are all skipped; an empty entry; the second of two paths
    failing after the first gave certificates;
  - a block type and a file name holding a control character are escaped
    in the report's lines.
- `ClaimRelay` sends the request's certificates and opens no file: with
  the CA file deleted after `newRequest`, the claim carries what was
  read.
- `ModuleRoot`: with the working directory inside a tree that declares the
  module and the executable outside one, it fails with a `FixableError`,
  once with `WardenVersion` `dev` and once with a release version, and
  neither message says "development"; the error wrapped by `GuestBinary`
  is still found as a `FixableError`; a `go.mod` that declares
  `github.com/buildwarden/buildwarden/tools/tier1`, or a longer path, is
  not a match.
- `driver/hyperv` and `driver/qemu`: the four tests that cross-build
  `warden-io` pass with the tree named by the package's `TestMain`, and
  `stageTestSeed` fails, not skips, when the binary cannot be staged.
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
  output directory is made; `doctor` exits 1 naming the file; `clean`
  exits 0;
- a CA directory holding a certificate, a certificate with its key and a
  DER `.cer`: `doctor` exits 0 and prints the `warning:` line, the DER
  skip line and the path's line; one of the builds the script already
  runs is given that directory, succeeds, and has the three lines on
  stderr;
- `--config ""`: `build` exits 1 with `To fix:`, in a directory whose
  `warden.toml` sets `output_dir = "from-repo"`, and no `from-repo` is
  made;
- F9: `dist/warden` copied to a temporary directory and run, with a `go`
  first on `PATH` that writes a marker, in a directory that declares the
  module: exit 1 with `To fix:`, the marker absent;
- one of the builds the script already runs is run with `--config`, so a
  named file is built with at no extra build.

**Tier 2.** No host blocks merge (FLOW-09): no driver's own code is
changed (the three test files added or edited in `driver/hyperv` and
`driver/qemu` run in CI), the lookup and the checks are the same code on every OS, and
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
   calls (one in `LoadConfig`, one in `resolveUpstreamCAs`), its test
   file, each under `TRANSITIONAL(one-config-file)`; the `sudo` sentence
   (GLOBAL-01, UX-03, UX-04).
4. `resolveUpstreamCAs` in `cmd/warden`: `WARDEN_UPSTREAM_CA_CERTS`
   replaces the key and is split on the OS's separator; `LoadConfig` no
   longer folds the variable into the key (GLOBAL-07, GLOBAL-10, UX-08).
5. `driver.LoadUpstreamCAs` by the rule of F2: it returns the valid PEM
   certificates, re-encoded, and a report of what each path and file
   gave and what was skipped and why, or the error; it prints nothing.
   `resolveUpstreamCAs` prints the report (the skips and the per-path
   lines always, the per-file and not-read lines with `--verbose`) on
   stderr for `build` and `shell` and into the report for `doctor`;
   `newRequest` and `doctor` call it before anything is created. The
   request carries the certificates (`driver.BuildRequest`,
   `driver/driver.go`), `ClaimRelay` sends them and reads no file, and
   `cmd/collector` reads `--upstream-ca` through the same function
   (GLOBAL-06, GLOBAL-10, GLOBAL-11, UX-04, UX-08).
6. The relay image check, one function called by `newRequest` and by
   `doctor`, under the container driver (GLOBAL-06, UX-04, UX-08).
7. `driver.ModuleRoot`: the executable only, the module line whole, a
   `FixableError` of its own that names the executable and its version,
   and the test-only setter; a `main_test.go` with a `TestMain` in
   `driver/hyperv` and in `driver/qemu` names the tree for the four tests
   that need it, and `stageTestSeed` in
   `driver/hyperv/environment_test.go` fails where it skipped (GLOBAL-04,
   GLOBAL-06, GLOBAL-11, UX-04).
8. `driver.StartDetached` passes the config file (GLOBAL-10).
9. Rework of what was written for two files (FLOW-06, UX-01, GLOBAL-04):
   - `runtime-cli-project-path`'s: the fourth row of its rule's table, the
     marker step in `LoadConfig`, `resolveRuntime`'s comment and the
     message that names the user's file, its reference text, its overview
     line and the tests of those;
   - `main`'s own two-file tests: the user-over-project cases of
     `cmd/warden/buildlink_test.go` and `cmd/warden/vmdns_test.go` (their
     helper writes both files) become one-file cases, with the labels
     `warden doctor` prints;
   - the `LoadConfig` tests that do not set `HOME`
     (`TestLoadConfig_Defaults`, `TestLoadConfig_UpstreamCACertsEnv` and
     the others in `cmd/warden/config_test.go`) get a temporary `HOME` and
     `USERPROFILE`, so that a developer's own `~/.config/warden/config.toml`
     does not fail them; `TestLoadConfig_UpstreamCACertsEnv` asserts the
     variable's two paths on `LoadConfig`'s result and moves to
     `resolveUpstreamCAs`.
10. The two `cmd/warden` messages that name the file for a bad `[vm]`
    value name the path read (Decision 13; UX-04).
11. The tier-0 tests of the Validation plan (GLOBAL-04, TEST-01, TEST-06).
12. The tier-1 cases in `test-integration.sh` (GLOBAL-04, TEST-02).
13. The reference docs, the README, the corporate-network guide, the
    installation guide's QEMU setup and the overview pseudocode (UX-01,
    FLOW-15).
14. The rule changes, once the review has approved each by name: ORCH-03 in
    `docs/development/orchestrator.md` and UX-07 in
    `docs/development/ux-cli-config.md` (FLOW-14).
15. `make lint` and `make test` green, then push (FLOW-17).
16. At Publish: the removal of the transitional detection filed as a queue
    item (a3), and the corpus entries of the Security note handed to
    `security-suite` (SEC-03).

Not Plan's, and done before it: on 2026-11-15 the review session
appended the four observations to `configuration-bugfixes`' `source` and
filed the two security findings as `workdir-in-context` (a11; workflow
`4c59842`; the table in **What this item does not do**). Still the review
session's, when it records the approval: the hold behind
`runtime-cli-project-path`'s merge (a7).

## Design review

None yet.

## Consolidation

Filled in at Publish (FLOW-10, FLOW-15).

## Rollback notes

None.

## Archive summary

Filled in at Publish (FLOW-12).
