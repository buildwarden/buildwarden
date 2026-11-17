---
feature: workdir-in-context
status: draft
drivers: [container, qemu, vz, hyperv, firecracker]  # conservatively (FLOW-02): the change is in
                         # cmd/warden and in shared code of driver/ (hostutil.go's NewWorkDir, the
                         # translator in driver/script, a new driver/imageref) that every driver
                         # uses. Every driver makes its work directory through NewWorkDir; the
                         # container, qemu, firecracker and hyperv (Linux guest) drivers translate
                         # a Containerfile. No driver/<name>/ tree is edited
seams: []                # the collector serves the context as it does (/v1/context/, collector
                         # protocol unchanged); no claim, executor, bootstrap or ledger change
surfaces: [cli, config, output]  # cli: --tmpdir inside the build context is refused. config:
                         # [build] tmpdir likewise. output: two new errors with their fixes (the
                         # work directory's placement; a FROM option or a FROM value that is not an
                         # image reference)
repos: []                # only this repository
areas: [driver-core, cli, book, designs]
                         # driver-core: driver/hostutil.go, driver/script/, driver/imageref/ (new);
                         # cli: cmd/warden; book: cli.md, configuration.md, overview.md (UX-01,
                         # FLOW-15); designs: this design
security_boundary: host-env  # what of the host the build environment can read through the
                         # collector (a work directory inside the context), and what a build
                         # context puts on the host's container runtime command line (FROM). The
                         # work directory also holds a relay's claim token on hyperv and
                         # firecracker, which reaches env-relay (Security note)
alters_relationships: false  # inside the orchestrator's and the translator's existing roles: two
                         # refusals before any driver work. No seam, no bootstrap change, no change
                         # of owner, no change to driver.Driver or driver.Session
design_review: required  # FLOW-19: a security boundary and three assumed questions
native_validation:
  - {host: container-any, blocking: false, reason: "no driver tree is edited and the checks are the same Go code on every OS; what only the Windows desktop adds is the work-directory check on NTFS (case-insensitive names, a drive letter, %TEMP% as the default, an 8.3 short name) and go test ./cmd/warden ./driver/... there, which CI does not run on Windows"}
  - {host: macos-finch, blocking: false, reason: "no driver tree is edited; what only a Mac adds is the work-directory check on APFS (case-insensitive names, TMPDIR under /var/folders, which is /private/var through a link) and the FROM refusal seen with finch, the default runtime CI does not run"}
tier2_gaps:
  - {host: container-any, gap: "on the Windows desktop: go test ./cmd/warden ./driver/...; warden build --tmpdir .\\tmp . and --tmpdir with the context's path in another letter case are each refused before an output directory exists; TEMP set inside the context is refused naming TEMP; --tmpdir C:\\elsewhere builds (backlog workdir-in-context-windows)"}
  - {host: macos-finch, gap: "on the Mac with finch: warden build --tmpdir ./tmp . and --tmpdir with the context's path in another letter case are each refused before an output directory exists; a context under /tmp with --tmpdir under /private/tmp/<context> is refused; a default build (TMPDIR under /var/folders) succeeds; a Dockerfile with FROM --platform=linux/arm64 alpine is refused before finch is run (backlog workdir-in-context-macos)"}
rules: [GLOBAL-01, GLOBAL-03, GLOBAL-06, GLOBAL-07, GLOBAL-09, GLOBAL-10, GLOBAL-11, ORCH-01,
        ORCH-02, ORCH-03, RELAY-08, UX-01, UX-04, UX-08, TEST-01, TEST-02, TEST-03, TEST-08,
        SEC-01, FLOW-01, FLOW-02, FLOW-05, FLOW-07, FLOW-09, FLOW-12, FLOW-15, FLOW-19, FLOW-20,
        LOOP-02, LOOP-04]
rule_changes: []
concepts: [work-dir, tmpdir, build-context, context-serving, collector, dockerfile-translation,
           from-directive, image-reference, option-injection, container-runtime-cli, claim-token,
           relay-seed, file-identity, security-fix]
related: [project-config-trust.md, hyperv-relay-per-build-channel.md, runtime-cli-project-path.md,
          container-inputs.md]
---

# A work directory inside the build context, and a FROM value on the runtime's command line

## Problem

Two security findings, filed on Jeff's word in `project-config-trust`'s
design review (its a11 (a); that design's **What this item does not do**).
Neither is a regression on `main`, and FLOW-20 exempts a security fix from
the freeze, so the freeze does not hold this item and it is not a program
item (it does not list `componentization.md` in `related`). It waits for
driver-contract (K1) instead, on Jeff's answer to a1 (**Order with
driver-contract**).

**1. A work directory inside the build context is served to the build.**
Every build makes a private work directory, `warden-<driver>-XXXXXXXX`,
under `--tmpdir`, else `[build] tmpdir`, else the OS temp dir
(`driver.NewWorkDir`, `cmd/warden/main.go` `resolveTempDir`). Nothing
stops that directory from being inside the build context: `--tmpdir .`,
`tmpdir = "tmp"` in a `warden.toml` read from the context (relative
paths in the file are relative to the working directory), or `TMPDIR`
(`TMP`/`TEMP` on Windows) pointing into it. The collector serves the whole
context root to the build, files and a recursive listing
(`GET /v1/context/<path>`, `collector/collector.go` `handleContext`,
`collector/store.go` `contextFiles`), confined by `os.Root` to the
context but with nothing excluded inside it. The build can then list and
fetch its own work directory, and every other build's work directory
under the same temp dir, while they exist.

Traced on `main` at `0cd3c02`, what each driver keeps there:

| Driver | In its work directory | Secret in it |
|---|---|---|
| container | `stage/build.sh`; `executor/warden-io`; for a dev relay, `relay-image/` (the relay binary and its Containerfile); the support images' build contexts | none: the claim token goes to the relay container in `--env` (`startRelayContainer`) |
| qemu | `build-overlay.qcow2` (the build VM's disk), `cidata/` and `seed.iso`, `relay-initrd`, `vlan.sock`; Windows: `build-vars.fd`, `wardenseed/`, `seed.img` | none on disk: the token is on the relay VM's kernel command line, a process argument (`driver/qemu/vm.go`) |
| vz | `build-disk.img` and `aux-storage` (clones of the build VM's disks), the ad-hoc signed `relay` | none: the token is in the relay child's environment (`driver/vz/driver.go`) |
| hyperv | `relay-seed/warden-relay.conf` (mode 0600) and its ISO, `relay.vhdx`, the build overlay, `seed/` and its ISO or VHDX | **the claim token**, in `warden-relay.conf` and the seed ISO (`driver/hyperv/relayseed.go`; hyperv-relay-per-build-channel Decision 3) |
| firecracker | `relay.json` and `build.json` (the VMMs' config files, mode 0600), `relay.console` and `build.console`, `kernel-cmdline`, `cidata/`, `seed.iso`, `build-initrd`, `relay-initrd` | **the claim token**, in `relay.json`'s boot arguments (`driver/firecracker/vm.go` `relayArgs`, `vm_linux.go` `startVMM`) |

Mode 0600 does not help: the collector runs as the user who owns the
files. What a build gains, as far as traced: the claim token authenticates
the claim and the tunnel (RELAY-08), but the build VM cannot reach the
claim port (the relay VM's init drops 8301 on the build interface,
hyperv-relay-per-build-channel), and its own relay is already claimed by
the time it runs; another concurrent build's token is reachable only from
the host. The rest is the build's own disks, public binaries and console
logs. So no escalation from the build side was found, but host-private
files of this and other builds reach the build environment, which is the
host-env boundary GLOBAL-06 names, and a token is exfiltrated through the
relay only as recorded bytes. K1 (driver-contract) adds a third tenant:
warden's stage directory, `<tmpdir>/warden-stage-<id>/` (**Order with
driver-contract**).

**2. A Dockerfile's FROM value reaches the container runtime's command
line unchecked.** The translator takes the first field after `FROM` as the
image (`parseFrom`, `driver/script/translate.go`), and the container
driver passes it as an argument to `<runtime> pull`, `image inspect
--format ... <value>` and, in an option's position, `container create ...
--entrypoint /.warden/warden-io <value> initialize ...`
(`driver/container/container.go`, `environment.go`). So `FROM
--platform=linux/amd64 ubuntu`, ordinary Dockerfile syntax, makes the
image `--platform=linux/amd64`, and a build context puts an option on the
host runtime's command line. It is the defect `project-config-trust` fixed
for the relay image only (its F1, `checkRelayImage`).

The source asked this item to establish whether `pull` fails first.
Established with the docker CLI 29.9.2, whose argument parsing is
client-side, so no daemon was needed: the value is always one token (it
is split on white space), and every option-shaped token fails `pull` with
a usage error ("'docker pull' requires 1 argument", or an unknown flag)
and exit 1, so the driver stops there with `error pulling image
--platform=linux/amd64`, a wrong error. One family passes: `--help` and
`-h` make `pull` print help and exit 0; `image inspect` then prints help
and exits 0, the digest is not found, and the driver records no
environment with only a warning (`prepareEnvironment` returns `nil, nil`
on that failure); `container create` prints help and exits 0, and its
output is taken as the container's ID; the build fails at the next step
that uses that "ID". So on docker it is a wrong error, and with `--help` a
build that gets as far as starting the relay and records a ledger with no
environment, not an escape. podman and finch were not observed here.
Under the VM drivers an option-shaped value cannot reach a command line
(`linuxguest.ResolveImage` stats it as a path or finds no cloud image for
it), but the qemu driver then boots its default image silently, with the
script translated from the same Containerfile.

## Prior art

- **project-config-trust** (archived): filed both findings (**What this
  item does not do**), and fixed the relay image's form with
  `checkRelayImage` (`cmd/warden/config.go`): a value that starts with `-`,
  or holds a space, an invalid UTF-8 byte or an unprintable character, is
  "not an image reference", with a fix. This item reuses that check for
  `FROM` and moves it to one place both use (FLOW-15's duplication
  sweep: one check, not two).
- **runtime-cli-project-path** (archived): the same class from the other
  side, a build context making warden run a program on the host. Its
  Decision 1 rejected normalizing a value instead of refusing it; this
  item refuses too.
- **hyperv-relay-per-build-channel** (archived): put the Hyper-V claim
  token in the relay seed "under the build's work directory (removed with
  it)", readable by the user. Its threat table assumes the work directory
  is private to the host. This item makes that assumption hold.
- **hyperv** `checkDirVisible` (`driver/hyperv/driver_windows.go`): the
  precedent for refusing a work directory's location before anything is
  made, with a fix that names `--tmpdir`.
- **container-inputs** (archived): ORCH-02's `inputs/`, and the translator
  that every driver's Linux guest shares. Unchanged here.
- **The collector's confinement** (`os.Root`, `collector/store.go`): a
  symlink in the context that points out of it is never served. That is
  why a work directory reached by a link from the context is already safe,
  and only one that is really inside the context leaks.
- **componentization** a2: the collector gets no storage boundary in the
  program. This item adds none either: it refuses the placement instead of
  teaching the collector what to hide.

No archived design overlaps beyond these (searched: `concepts`
`work-dir`, `tmpdir`, `collector`, `dockerfile-translation`,
`claim-token`, `relay-seed`; `rules` GLOBAL-06, ORCH-02, RELAY-08).

## Decisions

1. **A work directory inside the build context is refused, before any
   driver work.** `newRequest` (`cmd/warden/main.go`, which `build` and
   `shell` share) checks the directory work directories would be made in,
   the resolved `--tmpdir`, else `[build] tmpdir`, else the OS temp dir,
   against the resolved build context, right after both are resolved and
   before the output directory or anything else exists (UX-08). It is
   refused when it is the context or inside it. The error names the value
   and where it came from, and the fix:

   ```
   warden: the work directory would be inside the build context /src/app,
   which the build can read: --tmpdir is /src/app/tmp
   To fix: pass --tmpdir a directory outside the build context
   ```

   The source is `--tmpdir`, `[build] tmpdir in <file>`, or for the
   default `the OS temp dir (TMPDIR)` (`TMP`/`TEMP` on Windows), each
   fix naming what to change. A context inside the temp dir (`/tmp/app`
   with work directories in `/tmp`) is allowed: the work directory is a
   sibling, not a child.
   - *Rejected: the collector skips work directories when it serves the
     context.* It needs the collector to know every driver's naming and
     K1's stage directory, hides files the user put in the context under
     a `warden-*` name, and is the storage boundary componentization a2
     left out of the program. A silent exclusion is also magic
     (GLOBAL-11).
   - *Rejected: make the work directory somewhere else when the value is
     inside the context.* A provided value replaced by another is a
     fallback GLOBAL-10 forbids.
   - *Rejected: the drivers keep secrets out of the work directory.*
     Hyper-V has no other channel for the relay's pre-claim values
     (tools/relay-vm/README.md), firecracker's VMM takes its config as a
     file, and the rest of what a work directory holds is still the
     host's. It fixes the token and not the boundary.
   - *Rejected: check only `--tmpdir` and `[build] tmpdir`.* `TMPDIR` in
     the context is the same leak by another name.

2. **The check compares file identity, walking up.** From the deepest
   part of the temp dir path that exists, each directory up to the root
   is compared with the context by `os.SameFile`. That holds on a
   case-insensitive file system (APFS, NTFS), through symlinks (macOS's
   `/tmp` and `/var` are links into `/private`), and through a second
   name for the same directory (a bind mount, an 8.3 short name), where
   a string prefix test is wrong in each.
   - *Rejected: `filepath.EvalSymlinks` and a prefix test.* It is
     case-sensitive where the file system is not, and does not see a bind
     mount or a short name.

3. **`driver.NewWorkDir` runs the same check as a backstop.** The request
   carries `ContextDir` and `TempDir`, so the one function that makes every
   driver's work directory refuses it too, after its `MkdirAll`, on the
   real directory. A caller other than `newRequest` (a test, `cmd/collector`,
   a driver K1 starts as its own process with `context_dir` and `tmpdir`)
   is covered. One function, `driver.CheckWorkDirPlacement(tempDir,
   contextDir)`, does both; `newRequest` wraps its error with the source
   and the fix.

4. **`FROM` is parsed as `FROM <image> [AS <name>]`, and anything else is
   an error before anything is run.** `parseFrom` returns an error:
   - an option (a field that starts with `-`, such as `--platform=...`):
     `FROM --platform=linux/amd64 ubuntu: FROM options are not supported
     (--platform=linux/amd64); remove it`, the build running on the
     host's platform as it does today;
   - an image that is not an image reference by `checkRelayImage`'s test
     (it starts with `-`, or holds an unprintable character or invalid
     UTF-8): `FROM <value>: not an image reference`;
   - more fields than `<image> AS <name>` (`AS` in any letter case, as
     Docker reads it): `FROM ...: unexpected <field>`;
   - nothing after `FROM`: `FROM needs an image`.

   The translator is the only producer of the base image on `main` (the
   container driver's `stageBuildScript`, `linuxguest.ContainerfileImage`)
   and on K1's branch (`cmd/warden/stage.go` `baseImage`, which fills
   `start`'s `base_image`), so no driver receives an unchecked value.
   - *Rejected: honour `--platform` (pass it to `pull` and `create`).* A
     feature, not a fix; the VM drivers cannot honour it; it adds a value
     to the ledger's environment record.
   - *Rejected: skip options and take the image.* `--platform` would be a
     provided value silently ignored (GLOBAL-10).
   - *Rejected: put `--` before the image in each runtime command.* Each
     of finch, docker and podman parses its own arguments; it would need
     native validation on each and still gives the wrong error.
   - *Rejected: the full image-reference grammar (`distribution/reference`).*
     It refuses `FROM ./disk.qcow2` and `FROM /abs/image`, which the VM
     drivers resolve as a local image today (`ResolveImage`), and
     `FROM ${BASE}`, which fails with its own error later today. Changing
     those is a2's question, not this fix.
   - *Rejected: also check in the container driver before `pull`.* A
     second copy of one check at a point that only ever receives the
     translator's output; it adds `driver/container` to the areas and to
     the overlap with K1, which rewrites that driver.

5. **The image-reference test moves to one function both use.** A new
   package, `driver/imageref`, holds `Check(ref string) error`: the test
   `checkRelayImage` has, unchanged. `checkRelayImage` calls it and keeps
   its message and fix; the translator calls it for `FROM`. It is a new
   package because `driver/script` cannot import `driver` (which imports
   it), and `cmd/warden` imports both.

## Questions

### Blocking

None.

### Assumed

- **a1. The order with driver-contract (K1).** Answered (b) in the design
  review (Jeff, 2026-11-17): this item waits for K1; **Order with
  driver-contract** says how.
  K1 (in Design r3 on `feat/driver-contract`, waiting on its blocking q2)
  rewrites the build sequence: warden stages the script in
  `<tmpdir>/warden-stage-<id>/`, `start` carries `context_dir`, `tmpdir`
  and `base_image`, and the container driver's `Run` and `setup` change
  (its branch already edits `cmd/warden/main.go`, `driver/container/container.go`
  and adds `cmd/warden/stage.go`). The two items collide: K1 declares no
  areas, and both edit `cmd/warden` and `driver/` (FLOW-05). This item
  ranks ahead (priority 70 against 72) and is a security fix. Collision
  already keeps their pull requests apart (the PR slot's limit is 1), so
  no `collides_with` is needed; it is the human's to set in any case
  (`wf relate`).
  - (a) **This item first; K1 takes it in on its next merge of `main`
    and in its refresh.** What survives K1 unchanged: the work-directory
    check stays in `newRequest`, where warden still resolves `tmpdir` and
    the context and before it sends either, and it covers K1's stage
    directory, which is under `tmpdir`; `NewWorkDir`'s backstop goes where
    K1 puts `NewWorkDir`, in the kit; the `FROM` check stays in
    `driver/script`, which K1's `baseImage` calls, so `base_image` in
    `start` has passed it. What K1 then does: its specification's
    `base_image` row says the value has passed the translator's check,
    and `baseImage`'s "a parse error is the translation's to report"
    holds as it is. **Recommended**: the fix is small, ahead in order, and
    does not wait for an item with a blocking question and five blocking
    native validations still to run.
  - (b) This item waits for K1's PR to merge and is built on K1's
    structure. A security fix waits for the largest item in the queue.
  - (c) K1 absorbs both fixes. K1 grows again, and the fix waits with it.

- **a2. `FROM` as a host path in the VM drivers, and the silent default
  image.** Proceeding with (a); confirm. Found while tracing finding 2:
  under qemu and firecracker, `FROM /abs/x` or `FROM ./x` is used as the
  build VM's base disk as it is (`linuxguest.ResolveImage`), so a build
  context names a host file to boot. qemu opens it as qcow2 (`-F qcow2`),
  whose header can name a backing file of its own anywhere on the host.
  And `ContainerfileImage` returns empty on any translation error, so
  with `--script` a `FROM` that does not parse boots the driver's default
  image with no message (a fallback no design names, GLOBAL-11). Neither
  is an option on a command line, both are older than this item, and both
  change documented VM behaviour.
  - (a) **Out of this item.** The session that records this design's
    review files them through intake, on Jeff's word, as one security item
    for the VM drivers (proposed slug `vm-from-host-path`), with this
    paragraph as its source. **Recommended**: it needs its own trace of
    the qcow2 backing chain and of firecracker's `EnsureRaw`, and reaches
    `driver/qemu`, `driver/firecracker` and `driver/linuxguest` behaviour
    with native validation on qemu-any.
  - (b) This item fixes them too: refuse a path in `FROM` (a local image
    goes through `--image`), and make `ContainerfileImage`'s error the
    build's. It adds two driver trees and blocking native validation.
  - (c) Recorded here only.

- **a3. Other host directories that can lie inside the context.**
  Proceeding with (a); confirm. The same leak by other names: the output
  directory, `warden-output` by default, is inside the context for
  `warden build .`, so a build can read its own outputs as they land, and
  earlier builds' outputs and ledgers; and the cache
  (`os.UserCacheDir()/warden`, which `XDG_CACHE_HOME` moves) can be put
  inside it. Neither was in the source.
  - (a) **Unchanged and out of this item**, recorded under **What this
    item does not do** for Jeff to file or drop. **Recommended**: the
    output directory in the context is the default layout; refusing it
    breaks `warden build .`, and hiding it is the collector storage
    boundary componentization a2 left out. What it shows is the build's
    own outputs and their ledgers, each fetch recorded, not the host's
    secrets.
  - (b) Refuse an output directory or cache inside the context, as the
    work directory. Breaks the default.
  - (c) The collector does not serve the output directory. A collector
    change, against a2.

## Design

### The work directory

```
newRequest(cfg, args):
    ...
    tempDir, source := resolveTempDir(cfg)     # --tmpdir, else [build] tmpdir
                                               # (with its file), else ""
    dockerfile, contextDir, script := resolveBuildInput(...)
    dir := tempDir, else os.TempDir()          # what NewWorkDir will use
    if err := driver.CheckWorkDirPlacement(dir, contextDir); err != nil:
        refuse, naming the value and source (the OS default names TMPDIR,
        or TMP/TEMP on Windows), with the fix               # nothing exists yet
    ... output dir, upstream CAs, the request

driver.CheckWorkDirPlacement(tempDir, contextDir):
    if contextDir == "": nil                   # nothing is served
    ctx := stat(contextDir)
    p := the deepest existing directory of abs(tempDir)
    for p, then each parent of p up to the root:
        if os.SameFile(stat(p), ctx): error "inside the build context"
    nil

driver.NewWorkDir(req, name):
    MkdirAll(req.TempDir)
    CheckWorkDirPlacement(req.TempDir or os.TempDir(), req.ContextDir)   # backstop
    MkdirTemp(...)
```

`warden clean` reads `--tmpdir` to find leftovers and makes nothing, so it
does not check. `image prepare` makes its work directories in the cache,
not under `tmpdir`.

### FROM

```
parseFrom(rest) -> (image, error):
    fields := strings.Fields(rest)
    if none: error "FROM needs an image"
    if fields[0] starts with "-": error "FROM options are not supported (<f>); remove it"
    if imageref.Check(fields[0]) fails: error "FROM <value>: not an image reference"
    if len == 2, or len == 3 and fields[1] is not AS (any case), or len > 3:
        error "FROM ...: unexpected <field>"
    image = fields[0]
```

The error reaches the user through each driver's existing translation
error (`translating dockerfile: ...` in the container driver,
`translating <file>: ...` in `driver.StageBuildScript`), before any
runtime command or VM, as an unsupported directive does today. A `FROM`
that parses keeps its behaviour, `AS <name>` included.

### Other repositories

None: `repos` is empty. The collector protocol, the claim and the
bootstrap config, which `buildwarden/buildwarden-specs` specifies, are
unchanged; no pin moves.

### Order with driver-contract

As a1 (b), Jeff's answer in the design review: **K1 merges first, and this
item is built on K1's structure.** This item's Plan starts only after
driver-contract's PR has merged to `main`. When it has, the item returns
to Design once (FLOW-06) for the delta K1's structure brings, and that
delta is reviewed (FLOW-19). The delta is where each check lands:
`newRequest`'s refusal stays where warden resolves `tmpdir` and the
context, before it sends either in `start`, and covers K1's stage
directory under `tmpdir`; `NewWorkDir`'s backstop goes where K1 puts
`NewWorkDir`, in the kit, on the `context_dir` and `tmpdir` that `start`
carries; the `FROM` check stays in `driver/script`, which K1's `baseImage`
(`cmd/warden/stage.go`) calls, and K1's driver contract specification
says `base_image` has passed it. The Decisions, the messages and the
Validation plan are unchanged by the order; the Implementation plan below
was drafted on `main`'s structure and is redone by Plan on K1's. The area
map has the areas this item needs; no change to it is asked (FLOW-05).

### What this item does not do

- The output directory and the cache inside the context (a3).
- `FROM` as a host path in the VM drivers, and `ContainerfileImage`'s
  silent default image (a2).
- `FROM ${ARG}` substitution, a second `FROM`, or `--platform` support.
- Anything in the collector: it serves the context as it does.

## Defaults and overrides

No default is added or changed. The temp dir's resolution is unchanged:
`--tmpdir`, else `[build] tmpdir`, else the OS temp dir (`TMPDIR`; on
Windows `TMP`, `TEMP`, `USERPROFILE`), the highest that is set used
exclusively (GLOBAL-10). The new refusal applies to whichever is used, and
a refused value is never replaced by a lower one or by the OS default:
there is no fallback (GLOBAL-10, GLOBAL-11). The `FROM` value has no
default; a `FROM` that does not parse is an error, never a default image,
in the translation (Decision 4).

One existing fallback is touched by this area and left as it is:
`linuxguest.ContainerfileImage` returns no image when the Containerfile
does not translate, and the VM driver then boots its default image. It is
not named in the reference docs; whether it goes is a2's.

## Cross-driver impact

- **Every driver**: a work directory inside the build context is refused
  by `newRequest` before the driver is called, and by `NewWorkDir` as a
  backstop. A build that placed it there no longer runs; nothing else
  changes.
- **container**: `FROM` with an option, or an image that is not a
  reference, is refused at translation, before any runtime command. Today
  it fails at `pull` with a wrong error (or, with `--help`, after the relay
  starts).
- **qemu, firecracker, hyperv (Linux guests)**: the same refusal at
  `StageBuildScript`'s translation. Today `FROM --platform=... ubuntu`
  builds there on the default image, silently; it is now an error. With
  `--script` the script is not translated, and `ContainerfileImage`'s
  fallback (a2) is unchanged.
- **vz**: ignores a Containerfile, so only the work directory check
  applies.
- **hyperv Windows guests, qemu Windows guests**: no Containerfile is
  translated (`translates`), so only the work directory check applies.

## Security note

**Boundary.** host-env: the host's files reaching the build environment
through the collector, and a build context's text reaching the host's
container runtime command line. The Hyper-V and firecracker work
directories hold the relay's claim token (env-relay).

**Posture before.** A work directory, and with it the build's disks,
seeds, VMM configs, consoles and, on hyperv and firecracker, the relay's
claim token, of this build and of every concurrent build using the same
temp dir, can be inside what `/v1/context/` serves, by a flag, a config
key (including in a project's own `warden.toml`) or `TMPDIR`. A `FROM`
value is one token placed as an argument of three runtime commands, the
last in an option's position; on docker 29.9.2 every such token fails
`pull` except `--help`/`-h`, which reaches `create` and a relay start,
and records a ledger with no environment.

**Posture after.** Work directories are never inside the context, by
file identity, checked before anything exists and again where each is
made. A `FROM` value that reaches a driver is an image reference by the
same test as the relay image, with no option before it. Neither check
depends on the runtime or the OS.

**What remains.** The output directory inside the context (the default
for `warden build .`) shows the build its own outputs and earlier
builds' (a3). `FROM` as a host path in the VM drivers, and the qcow2
backing chain behind it (a2). podman and finch's parsing of an
option-shaped image was not observed; after this item none reaches them.

**What TEST-08's suite should exercise** (SEC-01, build-env ↔ host,
class 1, host FS access, reaching other builds): a build whose project
`warden.toml` sets `tmpdir = "tmp"` and whose script lists
`/v1/context/` (`warden-io fetch` of `tmp/`) is refused before it runs;
a build started with `TMPDIR` inside the context likewise; on hyperv and
firecracker a build that tries to fetch `warden-relay.conf` or
`relay.json` through the context finds no such path. A Dockerfile whose
`FROM` is `--help`, `--platform=linux/amd64 alpine`, or `-v` is refused
with no runtime process started (corpus: the runtime binary replaced by a
recorder that must not be called).

## Validation plan

- **Tier 0 (`make test`, Linux CI):** `driver.CheckWorkDirPlacement`:
  equal, nested, nested and not yet existing, reached through a symlink to
  the context, a context inside the temp dir (allowed), unrelated, empty
  context. `NewWorkDir` refuses a request whose `TempDir` is inside its
  `ContextDir`. `newRequest`: `--tmpdir .`, `[build] tmpdir = "tmp"`
  (message names the file), `TMPDIR` inside the context, each refused with
  no output directory created; `--tmpdir` elsewhere passes.
  `driver/imageref.Check` takes over `checkRelayImage`'s cases, whose
  messages are unchanged (its tests stay). Translator: `FROM alpine`,
  `FROM alpine AS build`, `FROM alpine as build` translate as today;
  `FROM --platform=linux/amd64 alpine`, `FROM --help`, `FROM -v`,
  `FROM alpine extra`, `FROM alpine AS`, `FROM` alone, and an unprintable
  byte each fail with their message; the container driver's and
  `StageBuildScript`'s errors carry it.
- **Tier 1 (`integration` workflow, container driver on docker):** a
  context with `FROM --platform=linux/amd64 alpine` fails before any
  `docker` command (no container, network or image of the build exists
  after); `warden build --tmpdir ./tmp .` fails before the output
  directory exists.
- **Tier 2 (native), tracked, none blocking** (FLOW-09): the change edits
  no driver tree, and the checks are runtime- and OS-independent Go.
  What only a native host adds is the file-identity check on a
  case-insensitive file system and the OS defaults: the Windows desktop
  (`container-any`) and the Mac (`macos-finch`), recorded in `tier2_gaps`
  and the native-gap backlog (`workdir-in-context-windows`,
  `workdir-in-context-macos`). The release's full matrix (TEST-02) is the
  backstop.

## Implementation plan

1. `driver/imageref`: `Check(ref string) error`, the test of
   `checkRelayImage` moved as it is, with its tests. `checkRelayImage`
   calls it; its message and fix are unchanged. (GLOBAL-01, GLOBAL-03,
   FLOW-15, TEST-01)
2. `driver/script`: `parseFrom` returns `(string, error)` per Decision 4;
   `handleDirective` returns its error; tests. (GLOBAL-03, GLOBAL-11,
   UX-04, TEST-01)
3. `driver`: `CheckWorkDirPlacement(tempDir, contextDir)` by file identity
   (Decision 2); `NewWorkDir` calls it after `MkdirAll`; tests. (GLOBAL-06,
   GLOBAL-07, GLOBAL-03, TEST-01)
4. `cmd/warden`: `resolveTempDir` also returns its source; `newRequest`
   calls `CheckWorkDirPlacement` after `resolveBuildInput` and before the
   output directory, with a fixable error naming the value, its source and
   the fix; tests that nothing is created. (UX-04, UX-08, GLOBAL-10,
   TEST-01)
5. Integration: the two tier-1 cases in `test-integration.sh` or the
   `integration` workflow's script. (TEST-02)
6. Reference docs (UX-01, ORCH-03): `docs/book/src/guide/cli.md`
   (`--tmpdir`, and the work directory paragraph: never inside the build
   context, and the error), `configuration.md` (`[build] tmpdir`, its
   comment). Overview (FLOW-15): `warden build`'s pseudocode gains the
   refusal; the translator's `FROM x` line becomes `FROM image [AS name]`
   with options refused.
7. `make test`, `make lint`, and `GOOS=windows make lint` and
   `GOOS=darwin make lint` for the shared files (FLOW-17, GLOBAL-07).

No already-implemented code of this item is reworked.

## Design review

None yet.

## Consolidation

To be filled in at Publish (FLOW-10, FLOW-15).

## Rollback notes

None.

## Archive summary

To be filled in at Publish (FLOW-12).
