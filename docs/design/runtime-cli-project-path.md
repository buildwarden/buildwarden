---
feature: runtime-cli-project-path
status: draft
drivers: [container]     # the setting is the container driver's. The change is in cmd/warden
                         # (config load and resolveRuntime); no driver/ tree is edited
seams: []
surfaces: [cli, config]  # config: [runtime] cli in a project's warden.toml takes only finch,
                         # docker or podman. cli: the help text of --runtime (a name or a path)
security_boundary: host-env  # a build context can no longer choose a program warden runs on
                         # the host
alters_relationships: false  # inside the orchestrator's existing role: reading its own
                         # configuration. No seam, no bootstrap change, no change of owner
design_review: required  # FLOW-19: a security boundary is touched, and three assumed questions
native_validation:
  - {host: container-any, blocking: false, reason: "the change reaches no driver tree and its check is the same code on every OS; what only a Windows host with a real runtime adds is the refusal of a .exe or .bat path and a build with cli = \"docker\" in the project's file, and make test does not run on Windows in CI"}
  - {host: macos-finch, blocking: false, reason: "the change reaches no driver tree; what only this host adds is a build with cli = \"finch\" in the project's file, the default runtime CI does not run"}
tier2_gaps:
  - {host: container-any, gap: "on a Windows host with a real runtime: go test ./cmd/warden; a project warden.toml whose cli is a .bat the directory ships is refused on build, clean and doctor and the file is not run; cli = \"docker\" in the project's file builds (backlog runtime-cli-project-path-windows)"}
  - {host: macos-finch, gap: "a build with cli = \"finch\" in the project's warden.toml, and the refusal of a path there, with finch installed (backlog runtime-cli-project-path-finch)"}
rules: [GLOBAL-04, GLOBAL-06, GLOBAL-07, GLOBAL-09, GLOBAL-10, GLOBAL-11, ORCH-01, ORCH-03,
        UX-01, UX-03, UX-04, UX-05, UX-07, UX-08, UX-09, TEST-01, TEST-02, TEST-03, TEST-06,
        TEST-08, SEC-01, SEC-03, SEC-04, FLOW-01, FLOW-02, FLOW-07, FLOW-09, FLOW-10, FLOW-12,
        FLOW-15, FLOW-19, FLOW-20]
rule_changes: []
concepts: [container-runtime, runtime-cli, project-config, config-levels, config-trust,
           build-context-trust, host-execution, path-lookup, security-fix]
related: [build-link-subnet.md, design-principles.md]
---

# A project's warden.toml names the container runtime; it does not choose a program to run

## Problem

`warden` reads `./warden.toml`, the file in the working directory, on every
command that loads configuration. In a cloned repository that file is the
repository's. Its `[runtime] cli` is taken as the container runtime's program
and run on the host:

- `LoadConfig` (`cmd/warden/config.go`) decodes the user's
  `~/.config/warden/config.toml` and then `./warden.toml` into one `Config`,
  so after loading nothing says which file a value came from.
- `resolveRuntime` (`cmd/warden/main.go`) returns `cfg.Runtime.CLI` as it is
  whenever it is not empty.
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

Which commands run it, read from the code at `897a4a8`:

| Command (container driver, the default) | Runs `[runtime] cli` from `./warden.toml` |
|---|---|
| `warden build`, `warden shell` | yes: first `<cli> pull <relay image>` (a release build) or `<cli> image build ...` (a development build) |
| `warden clean` | yes: `<cli> container ls ...`, then `network ls`, `image ls` |
| `warden doctor` | **no**. It prints `container runtime: <cli>` and `ok` without running it; only autodetection runs `<name> info`. The item's `source` and the report that filed it name `doctor`; that part is not so on `main` |
| any command with `--driver qemu`, `vz`, `hyperv` or `firecracker` | no: the runtime is not resolved |

`warden build <path>` from another directory reads that directory's
`warden.toml`, not the one beside `<path>`; the exposure is running `warden`
with the repository as the working directory, which is the documented way.

Found on 2026-11-14 by the reviewers of `driver-contract`'s redraft, whose
design records it under Risks as carried over and to be filed. Jeff, the same
day: "File it as a security fix". The rule he chose for a driver's file in
that review (its a16) is the one this item applies to the runtime's program: a
project's `warden.toml` may name a thing to run, never give a file path; a
path comes from the command line, the environment or the user's own config
file.

This is a feature item and not a program item: its `related` does not list
`componentization.md`. FLOW-20's freeze exempts it as a fix for a security
problem.

## Prior art

- **`driver-contract`** (K1, in its design review; not on `main`). Its
  Decision 5 and "Discovery and trust" give the rule for `[runtime] driver`:
  the project's file may name a driver, a path is refused, and a path comes
  from `--driver`, `WARDEN_DRIVER` or the user's config. It leaves
  `[runtime] cli` as it is and files this follow-up. It also moves the reader
  of `runtime.cli` into the container driver, which reads the files `warden`
  names in `config_files`, each with a `level` (`project` or `user`), and says
  that the level is carried so that this follow-up "can refuse a path in
  `runtime.cli` from the project's file inside the driver, with no change to
  the contract". This design lands first, on today's code, and says what K1
  has to keep (**What K1 has to keep**). It departs from K1's rule in one
  place: for the runtime, "a name" is not enough (Decision 1).
- **`build-link-subnet`** (archived). The precedent for knowing which file set
  a key: `loadTOML` uses the decoder's metadata (`md.IsDefined`) and records
  `BuildLinkSource` and `DNSSource`. This design uses the same metadata to
  tell the project's `cli` from the user's. It does not copy that design's
  "refused by every command as the file loads" (Decision 2).
- **`design-principles`** (archived): GLOBAL-09 to GLOBAL-11. A provided value
  that cannot be used is an error that names it and its fix; no lower level
  and no default is tried in its place.
- **`native-allowlist-scratch-tools`** (archived) notes that `WARDEN_CTR_CLI`
  "chooses the container CLI `warden` runs" and accepts it, because the
  environment is the user's. That stays so here.
- **Rules:** GLOBAL-06 (the host and build-environment boundary), UX-04,
  UX-08 and UX-09 (the refusal), UX-03 (Decision 5), UX-07 and ORCH-03 (the
  precedence, unchanged), UX-01 and FLOW-15 (the docs), SEC-01, SEC-03 and
  SEC-04 (the corpus entry in the Security note).

No archived design covers which values a project's file may set, or treats
the build context as untrusted input to `warden`'s own configuration. The
designs this one relies on carry front-matter already.

## Decisions

1. **From a project's `warden.toml`, `[runtime] cli` is one of `finch`,
   `docker` or `podman`, or empty. Anything else is refused.** Those are the
   names `warden` itself probes for (`DetectRuntime`), in one list that both
   uses share. The comparison is exact and the same on every OS: `Docker`,
   `docker.exe`, ` docker`, `nerdctl`, `sh`, `./docker`, `/usr/bin/docker`,
   `tools\docker.exe` and `C:docker` are all refused. An empty string stays
   what it is today, "not set here": it hides the user's value and the
   runtime is detected. Assumed question a1.
   - *Rejected: any name without a path separator, looked up on `PATH`* (the
     wording of the item's `source`, and K1's rule for a driver). It leaves
     the hole open. `warden`'s first argument to the runtime is a fixed word
     (`pull`, `image`, `container`, `info`), and an interpreter runs a file of
     that name from the working directory:

     ```toml
     [runtime]
     cli = "sh"
     ```

     with a file named `pull` in the repository makes a release `warden build`
     run `sh pull ghcr.io/buildwarden/relay:latest`, which executes
     `./pull`; `warden clean` runs `./container` the same way. Checked here:
     `sh pull x` and `sh container ls` each ran the file of that name in the
     working directory. `python3`, `node`, `perl` and `ruby` behave alike, and
     other programs on a `PATH` do damage with `warden`'s arguments and no
     file at all (`git pull <relay_image>`, with `relay_image` also set by
     the project). A driver's name does not have this problem, because
     `warden` puts `warden-driver-` in front of it.
   - *Rejected: refuse only a value with a path separator, or only an
     absolute path.* The first is the option above. The second refuses the
     harmless case and allows the harmful one: the repository's own file is a
     relative path.
   - *Rejected: allow a path that resolves outside the project directory.* It
     needs symlink-safe path comparison on three OSes, and a path outside the
     repository can still be a file the repository's author expects to exist
     (`/bin/sh`, with the `pull` file above).
   - *Rejected: add `nerdctl`* (the driver knows the name: `copyExecutor`
     switches on `finch` and `nerdctl`). `warden` neither probes for it nor
     documents it, and a user who has it names it at their own level. The
     list can grow later without breaking anything; it cannot shrink.
   - *Rejected: stop reading the key from the project's file.* A project that
     says `cli = "podman"` is documented, tested
     (`TestLoadConfig_ProjectFile`) and harmless, and removing it breaks more
     users than the fix needs to.
2. **The refusal is made when a command resolves the container runtime and
   the project's value is the one in effect.** That is `build`, `shell`,
   `clean` and `doctor` with the container driver, when neither `--runtime`
   nor `WARDEN_CTR_CLI` is set. It happens in `resolveRuntime`, before
   anything is looked up, run or created. A flag or the variable replaces the
   project's value as it always has (ORCH-03, GLOBAL-10), so
   `warden build --runtime docker` works in a repository whose file is
   refused, without editing the repository. Under another driver the key is
   not read, as today. Assumed question a2.
   - *Rejected: refuse as the file loads, on every command and driver, even
     when a flag overrides it* (what `[vm] build_link` does). It is not less
     safe, but the only way past it is to edit the repository's file, which
     changes the build context of a build whose point is to record its
     inputs. And K1 could not keep it: after K1 a driver's key is read only
     by that driver and only when it is selected (its Behaviour 13), so the
     rule would change again there.
   - *Rejected: skip the refused value and use the user's config or
     detection.* That is a fallback on failure (GLOBAL-10, GLOBAL-11), and it
     would hide from the user that the repository tried.
3. **The refused value never reaches the field that is run.** When the
   project's file sets a value that is not allowed, loading clears
   `Runtime.CLI` and keeps the value apart, for the message only. No code
   path can run it by reading `Runtime.CLI`, whatever it forgets to check.
   - *Rejected: keep the value in `Runtime.CLI` with a note of its source and
     check the source in `resolveRuntime`.* One missed check runs the file.
4. **The message names the file, the value and the three ways out; exit 1.**
   A `driver.FixableError` (UX-04), so `ERROR:` and `To fix:` with no
   bug-report link, and exit code 1 (UX-09). The value is printed with `%q`:
   it comes from a file someone else wrote, and `%q` keeps control characters
   out of the terminal. Text in **Design**.
5. **No deprecation window (UX-03).** The key is neither renamed nor removed,
   so UX-03's alias-and-notice does not apply as written, but a
   configuration that works today stops working, in the release that carries
   the fix. A window in which the value still runs with a warning is a window
   in which the hole is open, and the warning would be printed by a `warden`
   that is already running the repository's program, to the person it is
   running against. The refusal is not silent: it says what to do. Who is
   affected and what they do instead is in **Design**. Assumed question a3.
6. **Only the project's file is restricted.** `--runtime`, `WARDEN_CTR_CLI`
   and `[runtime] cli` in `~/.config/warden/config.toml` take any name or
   path and are used as given, as today. They are the user's.
   - *Not done: refuse a relative path in the user's config file*, as K1 does
     for a driver's path there. It is a different question (what a relative
     path in a file read from every directory means), the item's `source`
     limits it to the project's file, and K1 can align the two when the
     driver reads the key. It is listed under what remains in the Security
     note.
7. **Other keys of a project's file are reported, not changed.** The survey
   is in **Design**. It lists seven findings for intake (FLOW-09); four of
   them (F1 to F4) have this item's root, a project's file read as if the
   user wrote it.

## Questions

### Blocking

None.

### Assumed

- **a1. What may a project's `warden.toml` set `[runtime] cli` to?**
  Proceeding with (a); confirm.
  - (a) Exactly `finch`, `docker` or `podman`, the names `warden` probes
    for, or empty. **Recommended.**
  - (b) Those three and `nerdctl`.
  - (c) Any name without a path separator, looked up on `PATH`, as the
    item's `source` words it. This does not close the hole: `cli = "sh"` and
    a file named `pull` in the repository run that file on the host
    (Decision 1).
  - (d) Nothing: the key is read from the user's file only.
- **a2. When is a refused value an error?** Proceeding with (a); confirm.
  - (a) When it is the value in effect on a command that resolves the
    container runtime (`build`, `shell`, `clean`, `doctor`, container
    driver). `--runtime` and `WARDEN_CTR_CLI` replace it without an error,
    so a user can build a repository whose file is refused without editing
    it. This is also what K1 can keep unchanged. **Recommended.**
  - (b) As the file loads: every command that loads configuration, every
    driver, even when a flag overrides it, as `[vm] build_link` is. The way
    out is to edit the repository's `warden.toml`, and K1 narrows it again
    to "when the container driver is selected".
- **a3. The fix takes effect at once, with no release in which the old
  value still runs with a warning (UX-03).** Proceeding with (a); confirm.
  - (a) Refuse from the release that carries the fix. **Recommended.**
  - (b) Warn for one release, then refuse. The hole stays open for that
    release.

## Design

### The rule, by where the value is

| Where `cli` is set | `finch`, `docker`, `podman` | Another name (`nerdctl`, a wrapper on `PATH`) | A path |
|---|---|---|---|
| `--runtime` | yes | yes | yes; a relative one is relative to the working directory, like any path argument |
| `WARDEN_CTR_CLI` | yes | yes | yes, likewise |
| `[runtime] cli` in `~/.config/warden/config.toml` | yes | yes | yes |
| `[runtime] cli` in `./warden.toml` | yes | **refused** | **refused** |

The first three rows are unchanged. Precedence is unchanged: the flag, then
the variable, then `./warden.toml`, then the user's file, then detection
(`LoadConfig` and `resolveConfig` today; ORCH-03, UX-07). The highest level
that sets the value is the value (GLOBAL-10). A name is looked up on `PATH`
by `os/exec` each time it is run, as today.

### In the code

All of it is in `cmd/warden`. `driver/container` is not edited: it runs the
`Runtime` it is given.

```
knownRuntimes = [finch, docker, podman]   # one list: detection's order and
                                          # the names a project may use

LoadConfig:
    load ~/.config/warden/config.toml     # [runtime] cli: any string, as today
    load ./warden.toml
        if the file defines [runtime] cli:           # toml metadata, IsDefined
            "" or one of knownRuntimes -> Runtime.CLI = it         # as today
            anything else              -> Runtime.CLI = ""         # never the
                                          Runtime.RefusedCLI = it  # field run
    WARDEN_CTR_CLI not empty           -> Runtime.CLI = it; RefusedCLI = ""

resolveConfig:
    --runtime not empty                -> Runtime.CLI = it; RefusedCLI = ""

resolveRuntime:          # build, shell (newDriver), clean, doctor: container
    RefusedCLI != ""  -> the refusal; nothing is looked up, run or created
    Runtime.CLI != "" -> Runtime.CLI                               # as today
    else              -> DetectRuntime()                           # as today
```

- `loadTOML` is told which level it loads, so the rule applies to
  `./warden.toml` and not to the user's file. The decoder's metadata says
  whether this file defined the key, so a value the user's file set and the
  project's file left alone is not checked.
- `RefusedCLI` is not a TOML key (`toml:"-"`), like `BuildLinkSource`.
- `resolveRuntime` is the one place the three callers get the runtime, and
  `Runtime.CLI` is read nowhere else (checked by search at `897a4a8`). On
  `build` and `shell` it runs after `newRequest`, which only parses flags and
  stats the build path; nothing is created before it.
- A wrong TOML type (`cli = 5`) fails the load as today.
- A refused value in a file whose `cli` a flag or the variable replaces is
  not reported. Nothing reads it.

### The refusal

```
ERROR: warden.toml sets [runtime] cli to "./tools/docker"; a project's warden.toml may name the container runtime only as finch, docker or podman
To fix: pass --runtime <name or path> or set WARDEN_CTR_CLI, either of which replaces the value in warden.toml; or change [runtime] cli in warden.toml to finch, docker or podman; or remove it there, and /home/you/.config/warden/config.toml or detection decides
```

It goes to stderr like every error, and `warden` exits 1 (UX-09: not a
build's own exit). The names come from `knownRuntimes`, the value is printed
with `%q`, and the user's file is printed as the path `LoadConfig` reads
(`os.UserHomeDir`; on Windows under `%USERPROFILE%`). `warden doctor` reports
it too, as it reports the refusal a build would get.

### What changes for a user

| `./warden.toml` has | Today | After |
|---|---|---|
| no `cli` | the user's value, else detection | the same |
| `cli = ""` | detection (it hides the user's value) | the same |
| `cli = "finch"`, `"docker"` or `"podman"` | that runtime | the same |
| `cli = "/opt/homebrew/bin/docker"`, `"./tools/x"` | that file is run | refused on `build`, `shell`, `clean`, `doctor` unless `--runtime` or `WARDEN_CTR_CLI` is set |
| `cli = "nerdctl"`, or another program on `PATH` | that program is run | refused, likewise |

What to do instead:

- The project's own file: write the name (`docker`), or leave `cli` out and
  let each machine decide.
- A machine whose runtime is elsewhere or has another name: `cli` in
  `~/.config/warden/config.toml`, or `WARDEN_CTR_CLI` (a CI job's
  environment), or `--runtime` for one command.
- Someone else's repository whose file is refused: `--runtime docker`, or
  the variable. No edit to the repository.

### Reference docs (UX-01) and the overview (FLOW-15)

- `docs/book/src/guide/configuration.md`: the `cli` line of the Full
  Reference; a section on the container runtime in place of "Runtime
  Autodetection", with the table above, the refusal and its three ways out,
  and the one-line reason; `WARDEN_CTR_CLI` ("name or path") and `--runtime`
  in their tables.
- `docs/book/src/guide/cli.md`: `--runtime <name or path>` under Global
  Flags.
- `cmd/warden/main.go`: the `--runtime` help text says a name or a path.
- `docs/book/src/getting-started/installation.md`: the sentence on
  overriding detection stays true; it gains the link to the section above.
- `docs/book/src/concepts/overview.md`, in `warden build`, after the
  `req := resolve flags and config` line:

  ```
      container driver: runtime := --runtime, else WARDEN_CTR_CLI, else
                  [runtime] cli (from ./warden.toml only finch, docker or
                  podman: anything else is refused here, before anything is
                  run), else the first of finch, docker, podman that answers
  ```

### The same defect elsewhere

Every key `Config` (`cmd/warden/config.go`) reads from `./warden.toml`, and
where its value goes. "Read" means the code was read at `897a4a8`; nothing in
this table was run except where Decision 1 says so.

| Key in `./warden.toml` | Where the value goes | Verdict |
|---|---|---|
| `[runtime] cli` | run on the host | **this item** |
| `[runtime] driver` | checked against the five names (`validateDriver`) | not affected. K1 adds a path here and carries its own rule (a16) |
| `[runtime] relay_image` | `<runtime> pull <value>`, and the relay container is started from it; `"dev"` builds `./cmd/relay` with the host's `go` from the tree `driver.ModuleRoot` finds | **finding F1** |
| `[relay] upstream_ca_certs`, `system_ca_bundle` | each path is read on the host (`LoadUpstreamCAs`; a directory's `.pem`, `.crt`, `.cer`) and sent to the relay in its claim as upstream trust roots | **finding F2** |
| `[build] output_dir`, `tmpdir` | host directories `warden` creates and writes into | **finding F3** |
| `[vm] windows_image` | the URL or host path `warden image prepare --driver hyperv` fetches or registers when given no `--source` | **finding F4** |
| `[build] dockerfile`, `context` | nothing: `BuildCfg` declares them and no code reads them | **finding F5** (not security) |
| `[build] capture`, `compress` | an enum and a bool | not affected |
| `[vm] cpus`, `memory_mb`, `image_refresh_after`, `image_min_grace`, `windows_image_update` | numbers, ages, an enum | not affected |
| `[vm] build_link`, `dns` | parsed and checked as the file loads; addresses, not paths | not this defect. `dns` lets a project choose the relay's resolvers, which belongs with F2's question |
| `[vm] network` | the Hyper-V switch name, put into PowerShell through `psEscapeSingle` at the sites read (`ensurenet.go`, `relayvm.go`, `provision_local_windows.go`, `capability_windows.go`) | not this defect where read; not every interpolation was traced |
| `[output] color`, `verbose` | an enum and a bool | not affected |

The findings, for intake. None is fixed here (FLOW-09), and each is as read
from the code, not reproduced:

- **F1. A project's file chooses the relay.** `relay_image` is pulled and
  run as the relay, the component that records and signs the ledger, so the
  repository under measurement can supply the thing that measures it (the
  relay boundary; measurement evasion in SEC-01's terms). The container gets
  no mount and no privilege, so this is not host execution by itself. Two
  things make it worse. `relay_image = "dev"` builds the relay from source,
  and `driver.ModuleRoot` looks for that source by walking up from the
  `warden` executable and then from the working directory: with `warden`
  installed outside a source tree, a repository whose `go.mod` declares
  `module github.com/buildwarden/buildwarden` is that tree, and the host's
  `go build` runs on it. And the value is passed to the runtime's command
  line as it is, so a value that starts with `-` is read as an option.
- **F2. A project's file chooses the relay's upstream trust roots, by host
  path.** `upstream_ca_certs` names files anywhere the user can read; their
  bytes go to the relay. With `system_ca_bundle = false` they are the only
  roots. Alone, the bytes reach only the relay; with F1 the relay is the
  repository's, and a host file leaves the host.
- **F3. A project's file chooses host directories.** `output_dir` is created
  and filled with the build's output, which includes files the build posts;
  `tmpdir` is created and holds each build's work dir, and `warden clean`
  with `--driver firecracker` or `hyperv` removes the `warden-<driver>-*`
  directories it finds there whose build is no longer running. Top-level
  names are fixed, and nothing here is run, so the severity is lower.
  `tmpdir = "."` puts the work dir inside the build context, which the
  collector serves to the build; what that exposes was not checked.
- **F4. A project's file chooses the Hyper-V base image.** `windows_image`
  from `./warden.toml` is what `warden image prepare --driver hyperv`
  downloads or registers into the user's cache, where later builds of every
  project use it. Only `cmd/warden/image.go` and the head of
  `hyperv.FetchEvalImage` were read.
- **F5. `[build] dockerfile` and `[build] context` do nothing.** The
  configuration reference documents both (UX-01).
- **F6. `warden doctor` prints `ok` for a configured runtime it never ran.**
  With `cli` set at any level it prints the value and `ok`; only detection
  runs `<name> info` (UX-04).
- **F7. On Windows, a `docker.exe` in the working directory makes the lookup
  of `docker` fail.** Go's `LookPath` returns `ErrDot` when a name is found
  in the working directory and `PATH` has a different file of that name or
  none, and
  `DetectRuntime` treats any lookup error as "not installed", so a
  repository can make detection skip a runtime or report that none is
  installed. It cannot make the file run. Read in Go 1.26.3's
  `os/exec/lp_windows.go`; not run on Windows (UX-04).

F1 to F4 share one root with this item: the project's file is read as if the
user wrote it. Whether a project's file should be limited as a whole (names
and numbers, no host paths, nothing that selects trusted code) is a question
for Jeff that this item does not open.

### What K1 has to keep

K1 (`driver-contract`) moves `runtime.cli` out of `cmd/warden`'s `Config`:
the container driver reads it, through the kit, from the flag, the variable
and the files in `config_files`, each with its `level`. This item merges
first; K1 meets it in its consolidation (FLOW-10). The requirement below is
for K1's Implementation plan and is handed to that item; this design does not
edit K1's.

1. **The rule.** When the container driver's `runtime` setting resolves to a
   value read from a `config_files` entry whose `level` is `project`, the
   value is exactly `finch`, `docker` or `podman`, or empty. Anything else
   fails the command with this design's message and fix, exit 1, before the
   driver looks up, runs or creates anything: in the probe of `run`, and on
   whichever other commands of the driver's resolve the runtime (what
   `warden shell`, `doctor` and `clean` reach).
2. **The other levels.** A value from `--runtime`, `WARDEN_CTR_CLI` or a
   `user`-level file is used as given, a path included, and replaces a
   project value whether or not that value would have been refused.
3. **No fallback.** A refused project value is not replaced by a `user`-level
   value or by detection.
4. **The refused value is never the one run.** However the kit stores a
   resolved setting, a refused value does not sit where the code that runs
   the runtime reads.
5. **What a later command reads.** K1's `reap` and `attach` take the runtime
   from the driver's state file, written during `run`. The value written
   there has passed this rule, and that file is not in the build context.
6. **The lifecycle suite.** K1's suite passes a host's settings as a
   `config_files` entry "at the `project` level", the container runtime
   among its examples. A runtime given as a path goes through
   `--driver-arg --runtime=<path>` instead.
7. **K1's own text.** Its "Discovery and trust", Risks and Security note say
   `[runtime] cli` from a project's file "is run as given, today and after
   K1". That is no longer so once this merges; K1's Design updates it.

**The guard is this item's tests.** The integration cases run the built
`warden` against a project directory and do not care where the reader lives:
they must pass on K1's branch unchanged. The unit tests move with the code
they test (to the container driver or the kit) and keep their tables: the
refused values, the three levels that still take a path, no fallback, and
the marker file that shows nothing was run.

If K1 were to merge first, the code this item changes would be gone and it
would return to Design (FLOW-06).

### What this item does not do

- It does not change what `--runtime`, `WARDEN_CTR_CLI` or the user's file
  accept (Decision 6).
- It does not resolve a name to an absolute path once, or check who owns the
  file found. K1 decides lookup for drivers; the runtime is looked up by
  `os/exec` as today.
- It does not fix F1 to F7.
- It changes no rule (`rule_changes: []`).

## Defaults and overrides

| Default, override or fallback | Status | Why it is the obvious choice (GLOBAL-09) | What overrides it, exclusively (GLOBAL-10) |
|---|---|---|---|
| The container runtime when nobody names one: the first of `finch`, `docker`, `podman` that answers `info` | Unchanged | Today's detection | Any level that sets `cli` |
| `[runtime] cli` in `./warden.toml`: one of `finch`, `docker`, `podman`, or empty | **Changed**: it was any string, run as given (a1) | The file may come with someone else's build context; a name from `warden`'s own list can only select a runtime the user already has, and is what the reference documents for the key | `--runtime`, then `WARDEN_CTR_CLI`: either replaces it, refused or not (a2) |
| `[runtime] cli` in the user's file, `WARDEN_CTR_CLI`, `--runtime`: any name or path | Unchanged | They are the user's | Each by the level above it |
| The names a project may use are the names detection probes | **New**: one list | Two lists would drift, and "the runtimes `warden` knows" is one idea | Nothing; a user names another program at their own level |

No merge and no fallback is added. A refused project value is an error where
it is the value in effect: the user's file and detection are not tried in its
place (GLOBAL-10, GLOBAL-11). A project value that a flag or the variable
replaces is not used and not checked, which is precedence, not a fallback.
No relay setting is added or moved (RELAY-08 does not apply).

## Cross-driver impact

- **container**: the commands that resolve its runtime (`build`, `shell`,
  `clean`, `doctor`) refuse a project's `cli` that is not one of the three
  names. `driver/container/` is not edited, and a build whose runtime comes
  from any other level, or from one of the three names, runs exactly as
  before.
- **qemu, vz, hyperv, firecracker**: unaffected. They never resolve the
  container runtime, so a project's `cli` is not read under them, refused or
  not, as it is not read today. No file of theirs and no shared `driver/`
  file changes (ORCH-01).

## Security note

**Boundary: the host OS and the build environment** (GLOBAL-06, `host-env`).
The build context is the build environment's side of it: it is what the
isolated build runs. `warden` reads one file of it, `warden.toml`, on the
host, before any isolation exists.

**The threat.** Code execution on the host, as the user who runs `warden`,
by a repository that the user only meant to build: `cd repo && warden build`
(or `shell`, or `clean`) with the default driver. SEC-01's cell is build
environment to host, class 1 (escape), though the attacker never has to get
out of anything: the program runs before the environment is made, and the
ledger does not show it.

**How the posture changes.** It tightens, and nothing loosens. Before: any
string from the project's file was run. After: the project's file can select
one of three names, which `os/exec` finds on the user's `PATH`; every other
value is refused before anything is looked up or run, and the refused value
is never stored where the runtime is read from (Decision 3).

**What remains**, each unchanged by this item:

1. A repository can still choose among `finch`, `docker` and `podman`, and
   its choice outranks the user's config file (project over user, ORCH-03).
   A user with rootless podman configured and Docker also installed can be
   moved to Docker by a repository. `WARDEN_CTR_CLI` or `--runtime` pins it.
2. The name is found on the user's `PATH`. A `PATH` entry a repository can
   write to is the user's exposure, as for any command. Go refuses a match in
   the working directory or through a relative `PATH` entry (`ErrDot`), so a
   planted `docker` there is not run; it can make the lookup fail (F7).
   `GODEBUG=execerrdot=0` in the user's environment turns that refusal off.
3. Values a repository controls still reach the runtime's command line: the
   relay image (F1) and the `FROM` image of its Dockerfile, in `pull`,
   `image inspect` and `container create`. With the program fixed to a
   container runtime these name images, which is what a build does; an
   option-shaped value is F1's.
4. A relative path the user gives (`--runtime ./x`, the variable, their own
   config file) is resolved against the working directory.
5. F1 to F4: the project's file still chooses the relay image, the relay's
   trust roots, two host directories and the Hyper-V base image.

**What the TEST-08 suite should exercise.** Arm A gains this as a permanent
entry (SEC-03), with the attacker supplying the build context: (i) a
`warden.toml` whose `cli` is a file the context ships; (ii) `cli = "sh"`
with files named `pull`, `container`, `image` and `info` in the context.
Each on `build`, `shell`, `clean` and `doctor`. The proof is SEC-04's: the
file would write a marker; the marker is absent and `warden` exits 1. The
suite's matrix places the attacker inside the build environment; this
finding came from outside it, from a file the host reads before the
environment exists. The files of a build context that `warden` reads on the
host (`warden.toml`, and the Dockerfile it translates) are surface the suite
should count.

The merge acknowledges this note (FLOW-08).

## Validation plan

**Tier 0** (`make test`; the `ci` workflow on Ubuntu, and any platform
locally), in `cmd/warden`, table-driven, with `HOME` (and `USERPROFILE`) set
to a temporary directory so that no test reads the real user's file:

- The project's file, by value: empty, `finch`, `docker`, `podman` are
  accepted; `./docker`, `tools/docker`, `/usr/bin/docker`, `..\x.exe`,
  `C:\x\docker.exe`, `C:docker`, `docker.exe`, `Docker`, ` docker`,
  `nerdctl` and `sh` are refused. Each refusal is a `FixableError` whose
  message names `warden.toml` and the quoted value and whose fix names
  `--runtime`, `WARDEN_CTR_CLI` and the user's file. The table is the same
  on every OS.
- The other levels: a path in the user's file, in `WARDEN_CTR_CLI` and in
  `--runtime` is returned as given; each of the last two also replaces a
  refused project value with no error.
- No fallback: with `cli = "podman"` in the user's file and a refused value
  in the project's, the result is the refusal, and detection is not run (a
  fake `finch` on `PATH` whose `info` writes a marker; the marker is absent).
- Nothing is run: a project directory that ships an executable writing a
  marker, as `cli = "./rt"` and as `cli = "sh"` with a file named `pull`;
  through the calls `build` and `shell` make (`newDriver`), `clean` and
  `doctor`, the error is the refusal and the marker is absent. These use a
  shell script and skip on Windows, as `fakeRuntime` does today.
- The exit code: the refusal is a `FixableError`, which `main` exits 1 on
  (UX-09).
- Detection still probes `finch`, `docker`, `podman` in that order from the
  shared list (`TestDetectRuntimeExplainsTheFix` stands).

**Tier 1** (`make integration-test`; the `integration` workflow on hosted
Ubuntu with Docker, which runs because the change reaches `cmd/` and
`test-integration.sh`): in a temporary project directory with a Dockerfile,
a `warden.toml` and an executable that writes a marker,

- `cli = "./rt"`: `warden build`, `warden clean` and `warden doctor` each
  exit 1 with `To fix:`, the marker is absent, and no output directory is
  made;
- the same file with `--runtime docker`, and with `WARDEN_CTR_CLI=docker`:
  `warden doctor` prints `container runtime: docker`;
- `cli = "docker"`: one of the builds the script already runs is run from
  such a directory, so a named runtime from a project's file is still built
  with, at no extra build.

These run the built binary, which is why they guard the rule across K1.

**Tier 2.** No host blocks merge (FLOW-09): the change reaches no driver
tree, and the check is one comparison against a list, the same code on every
OS. Two gaps are tracked (`tier2_gaps`, and the native-gap backlog):

- `container-any` (the Windows desktop): `go test ./cmd/warden`, which CI
  does not run on Windows; a project `warden.toml` whose `cli` is a `.bat`
  the directory ships is refused on `build`, `clean` and `doctor` and the
  file is not run; `cli = "docker"` in the project's file builds.
- `macos-finch`: a build with `cli = "finch"` in the project's file, and the
  refusal of a path there.

The release matrix (TEST-02) is the backstop, and the release that carries
this trips GLOBAL-06, so TEST-08 runs.

## Implementation plan

One PR. Nothing already implemented is reworked beyond the functions named.

1. One `knownRuntimes` list in `cmd/warden/config.go`; `DetectRuntime`
   ranges over it, in today's order (GLOBAL-09, GLOBAL-10).
2. `loadTOML` takes the level it loads. For the project's file, a defined
   `[runtime] cli` that is neither empty nor in the list clears
   `Runtime.CLI` and sets `Runtime.RefusedCLI`; `WARDEN_CTR_CLI` and
   `--runtime`, when not empty, clear `RefusedCLI` (GLOBAL-06, GLOBAL-10,
   ORCH-03, UX-07).
3. `resolveRuntime` returns the refusal when `RefusedCLI` is set, as a
   `driver.Fixable` with the message and fix of **The refusal** (UX-04,
   UX-08, UX-09, GLOBAL-11).
4. The tier-0 tests of the Validation plan, in `cmd/warden`; the existing
   `LoadConfig` tests set `HOME` and `USERPROFILE` like the new ones
   (GLOBAL-04, GLOBAL-07, TEST-01, TEST-06).
5. The tier-1 cases in `test-integration.sh` (GLOBAL-04, TEST-02).
6. The reference docs and the `--runtime` help text (UX-01, UX-05), and the
   overview pseudocode (FLOW-15).
7. `make lint` and `make test` green, then push (FLOW-17).

## Design review

None yet.

## Consolidation

Filled in at Publish (FLOW-10, FLOW-15).

## Rollback notes

None.

## Archive summary

Filled in at Publish (FLOW-12).
