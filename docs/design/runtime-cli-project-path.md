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
   is in **Design**; five are findings for intake (FLOW-09), two of them on a
   security boundary.

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
