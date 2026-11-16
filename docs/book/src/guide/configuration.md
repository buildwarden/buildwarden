# Configuration

BuildWarden reads one config file, chosen by whoever runs `warden`. Every
setting can come from a flag, an environment variable, that file or its
default, in that order: a flag replaces the variable, the variable replaces
the file, and the file replaces the default.

## The config file

A command that reads configuration reads exactly one file, the first of:

1. the file `--config <path>` names;
2. else the file `WARDEN_CONFIG` names (an empty `WARDEN_CONFIG` is not set);
3. else `warden.toml` in the working directory, when it exists;
4. else none, and every setting is its default.

No other file is read and nothing is merged. A source that a higher one
replaces is not looked at: with `--config` given, neither the variable's file
nor `./warden.toml` is opened.

- The default is the working directory's `warden.toml`, whatever `[path]` is:
  `warden build /src/repo`, run from elsewhere, does not read
  `/src/repo/warden.toml`.
- A file named with `--config` or `WARDEN_CONFIG` that does not exist, is a
  directory or cannot be read is an error, and no other file is tried in its
  place. `--config ""` is an error too. A `./warden.toml` that exists but
  cannot be read is an error, not "no file".
- `./warden.toml`, which nobody named, must be a regular file once links are
  followed: a link to a device or a pipe (`/dev/zero`, a terminal) is an
  error, `not a regular file`. It is read up to 1 MiB: one that gives more
  is the error `larger than 1 MiB`, whatever size it appears to have on
  disk (a link to `/proc/self/pagemap` appears empty). A file named with
  `--config` or `WARDEN_CONFIG` is read whole, whatever kind of file it is
  and whatever its size.
- An empty file is a file: every setting is its default.
- A relative `--config` or `WARDEN_CONFIG` is relative to the working
  directory, like any path argument, so a relative `WARDEN_CONFIG` names a
  different file in each directory. A `WARDEN_CONFIG` set once for a machine
  should be absolute. A relative path inside the file (a CA path, `tmpdir`,
  `output_dir`) is relative to the working directory too.
- A file that is not valid TOML is an error for every command that reads it.

Every command that reads configuration says which file it read, without
`--verbose`. `build`, `shell`, `clean`, `setup` and `image prepare` print it
first on stderr; `warden doctor` prints it as the first line of its report.
The path is absolute, and a control character in it (a newline, an escape
sequence from a directory's name) is printed escaped (`\n`, `\x1b`), here and
in every message that names the file:

```
[warden] config: /etc/warden/ci.toml (--config)
[warden] config: /home/ci/warden.toml (WARDEN_CONFIG)
[warden] config: /src/repo/warden.toml (found in the working directory)
[warden] config: none (no --config, no WARDEN_CONFIG, no ./warden.toml): defaults apply
```

`warden inspect`, `image list`, `image verify`, `hyperv service`,
`--version` and help read no configuration: they accept `--config` and open
nothing.

A message or a fix that says "in warden.toml" means the config file in use,
whatever its name; the `config:` line gives its path.

### Building a repository you do not control

With no file named, `cd repo && warden build` reads the repository's
`warden.toml`, and that file decides how the repository is measured: the
driver, the container runtime among finch, docker and podman, the relay
image, the relay's extra trust roots, whether payloads are captured, where
the output and work directories go, and, with `--driver hyperv`, a base image
that later builds of every project boot. The `config:` line says the file was
read; it does not stop it. On a machine that builds repositories it does not
control:

- name the file: `--config /etc/warden/ci.toml`, or an absolute
  `WARDEN_CONFIG` in the machine's environment. An empty file gives the
  defaults. `WARDEN_DRIVER`, `WARDEN_CTR_CLI`, `WARDEN_IMAGE` and
  `WARDEN_UPSTREAM_CA_CERTS` pin those settings whatever file is read;
- under `sudo`, pass `--config`: `sudo` resets the environment by default, so
  a `WARDEN_CONFIG` you set is dropped and the repository's `./warden.toml`
  is read. `sudo --preserve-env=WARDEN_CONFIG`, or a sudoers `env_keep`,
  keeps the variable;
- keep `warden` outside the repository it builds. A `warden` whose executable
  is inside a source tree that declares the `github.com/buildwarden/buildwarden`
  module builds its relay and executor from that tree with a VM driver or
  with `relay_image = "dev"`, and a repository can declare that module;
- a repository's file can name any host directory as an upstream CA path and
  turn on `verbose`. The build's log then names that directory's entries and
  what was skipped there (see [Upstream CA paths](#upstream-ca-paths)). Where
  others can read the log, name the file, or pin the paths with
  `WARDEN_UPSTREAM_CA_CERTS`.
- the repository's `warden.toml` is the one file warden reads that nobody
  named, so it is read with two limits: it must be a regular file once links
  are followed, and it is read up to 1 MiB. A repository whose `warden.toml`
  is, or links to, a device, a pipe or a file that gives more than 1 MiB
  stops with `not a regular file` or `larger than 1 MiB` rather than hang
  the build. A file you name has neither limit, so name a file you control.

<!-- TRANSITIONAL(one-config-file): this subsection is removed with
     cmd/warden/config_former.go, in the change that prepares 1.0. -->
### Moving from `~/.config/warden/config.toml`

Earlier releases also read `~/.config/warden/config.toml`
(`%USERPROFILE%\.config\warden\config.toml` on Windows) and merged it under
`./warden.toml`. It is no longer read. While it exists, `build`, `shell`,
`clean`, `doctor`, `setup` and `image prepare` stop with an error that names
it, before any setting of the config file is applied, rather than drop its
settings without a word:

```
[warden] ERROR: /home/you/.config/warden/config.toml is no longer read: warden reads one config file, ./warden.toml or the one --config or WARDEN_CONFIG names
  To fix: move its settings into the warden.toml you build with, or into the environment (WARDEN_DRIVER, WARDEN_CTR_CLI, WARDEN_IMAGE, WARDEN_UPSTREAM_CA_CERTS), and delete it; or name it as the config file with --config or WARDEN_CONFIG (it is then the only one read: ./warden.toml is not)
```

The file is not opened. Naming it with `--config` or `WARDEN_CONFIG` keeps
your settings as they were, and it is then the only file read. Under `sudo`
the variable is dropped (see above), so the error comes back there; the fix
then adds: pass `--config`, or run `sudo --preserve-env=WARDEN_CONFIG`.

`WARDEN_UPSTREAM_CA_CERTS` used to add its paths to `[relay]
upstream_ca_certs`; it now replaces them. While both are set, `build`,
`shell` and `doctor` stop with an error that says so: put every path in one
of the two.

## Full Reference

```toml
[runtime]
cli = "finch"              # Container runtime: finch, docker or podman, and nothing
                           #   else (unset = detected; see The container runtime)
driver = ""                # Build driver: container, qemu, vz, hyperv, firecracker
                           #   (default: auto, the container driver)
relay_image = ""           # Relay image override (container driver)
                           #   "" = pull ghcr.io/buildwarden/relay:latest (release builds)
                           #        or build from source (dev builds)
                           #   "dev" = always build from source
                           #   "ghcr.io/buildwarden/relay:v0.2.0" = specific version

[build]
dockerfile = ""            # Override Dockerfile discovery
context = ""               # Override context directory
output_dir = "warden-output"  # Where build results are written
compress = true            # Compress ledger and payloads with zstd
capture = ""               # Payload capture mode:
                           #   "" or "none" = no capture
                           #   "headers" = save request/response headers
                           #   "bodies" = save request/response bodies
                           #   "all" = save both
tmpdir = ""                # Where each build's private work dir is created
                           #   ("" = the OS temp dir; --tmpdir wins)

[relay]
upstream_ca_certs = []     # Files or directories of PEM certificates the relay
                           #   also trusts upstream (see Upstream CA paths)
system_ca_bundle = true    # With upstream_ca_certs: keep the relay's own system
                           #   roots beside them (false: only the listed ones)

[vm]                       # Defaults for the VM-driver flags; a flag always wins
cpus = 0                   # Build VM vCPUs (0 = per-driver default; vz, qemu,
                           #   hyperv and firecracker)
memory_mb = 0              # Build VM memory in MiB (0 = per-driver default)
network = ""               # hyperv: durable network name (switches <name>-0.., NAT <name>-nat)
image_refresh_after = "7d" # hyperv: re-prepare the base in the background at this age
image_min_grace = "1d"     # hyperv: refuse builds with less evaluation grace left
windows_image = ""         # hyperv: base image URL or local path (default: Microsoft's
                           # current Server 2025 evaluation download)
windows_image_update = "auto" # hyperv: "auto" picks up new media at refresh; "reuse" keeps it
build_link = "100.127.255.252/30" # advanced: the relay-to-build link, a /30 (every VM
                           #   driver; see Advanced options; --build-link wins)
dns = ["10.20.0.53"]       # the relay's upstream DNS servers, in order, used exclusively
                           #   (every VM driver; unset: the host's; see Advanced options)

[output]
color = "auto"             # Color mode: auto, always, never
verbose = false            # Verbose output (shows container runtime commands)
```

## Upstream CA paths

`[relay] upstream_ca_certs`, or `WARDEN_UPSTREAM_CA_CERTS`, which replaces
it, lists extra roots the relay trusts for its upstream TLS connections,
beside its own system roots (which `system_ca_bundle = false` drops). They
are read once, by `build`, `shell` and `doctor`, before anything is created;
`clean`, `setup` and `image prepare` do not read them.

Each path is a file or a directory; a relative one is relative to the working
directory.

- **A file named directly** is read whatever kind of file it is, a pipe
  included (`WARDEN_UPSTREAM_CA_CERTS=<(cat a.pem b.pem)` works). A path that
  never ends, or a pipe nobody writes to, makes `warden` wait.
- **A directory** means its files whose names end in `.pem`, `.crt` or
  `.cer`, in any letter case, one level deep and in name order; links are
  followed. A subdirectory, a link to one and an entry with another ending
  are not read, and an entry that is not a regular file (a pipe, a socket, a
  device) is not opened.

Every valid PEM certificate found is sent to the relay, and nothing else: no
private key, no other block, no text between blocks. What is not sent is
skipped, with a line that says why, and the build goes on with the
certificates the path does give:

- a private key (a block whose type ends in `PRIVATE KEY`), which gets a
  `warning:` line because a key does not belong in a file of trust roots;
- another kind of block (a `CERTIFICATE REQUEST`, an `X509 CRL`), or a
  `CERTIFICATE` block with PEM headers;
- a damaged block (its BEGIN line is not complete, it has no END line, its
  END line is for another type, or its body is not base64);
- a certificate Go's X.509 parser refuses, with the parser's reason;
- in a directory, a DER-encoded file (convert it with `openssl x509 -inform
  der -in <file> -out <file>.pem`), a file with no PEM block, one whose
  blocks hold no certificate to send, one that cannot be read, and a link to
  a file that does not exist.

The lines go to stderr with `build` and `shell` (after the `config:` line;
`--quiet` does not remove them) and into the report with `doctor`. Every skip
and one line per path are printed on every run; `--verbose` adds one line per
file of a directory that gives a certificate and one per entry that was not
read:

```
[warden] warning: upstream CA /etc/warden/site.pem: block 2 is an RSA PRIVATE KEY and is not sent to the relay; a private key does not belong in a file of trust roots
[warden] upstream CA /etc/warden/site.pem ([relay] upstream_ca_certs): 1 certificate to send; 1 block skipped
[warden] upstream CA /etc/warden/roots/corp.cer: skipped: DER-encoded, and only PEM is read (to convert: openssl x509 -inform der -in <file> -out <file>.pem)
[warden] upstream CA /etc/warden/roots (WARDEN_UPSTREAM_CA_CERTS): 3 certificates to send from 2 files; 1 file skipped; 4 other entries not read
```

A path that does not exist, cannot be read or gives no certificate at all is
an error, before anything is created; `doctor` reports it as `BLOCKED` and
exits 1. An empty entry (a doubled or trailing separator, `""` in the list)
is an error too, never read as the working directory.

The lines describe the host: for each path whether it exists, the counts, the
name of each file skipped and why, and with `--verbose` the name of every
entry of a named directory. A repository's `warden.toml` can name any host
directory and turn `verbose` on; see [Building a repository you do not
control](#building-a-repository-you-do-not-control). Names and other strings
that do not come from `warden` are printed with control characters escaped.

## Environment Variables

A variable replaces the config file's key, and a flag replaces the variable:
flag, variable, file, default.

| Variable | Overrides | Description |
|----------|-----------|-------------|
| `WARDEN_DRIVER` | `runtime.driver` | Override driver selection |
| `WARDEN_IMAGE` | `runtime.relay_image` | Relay image override |
| `WARDEN_CTR_CLI` | `runtime.cli` | Container runtime: finch, docker or podman (see [The container runtime](#the-container-runtime)) |
| `WARDEN_CONFIG` | the config file | The config file to read when `--config` is not given (see [The config file](#the-config-file)); set it absolute |
| `WARDEN_UPSTREAM_CA_CERTS` | `relay.upstream_ca_certs` | Replaces `relay.upstream_ca_certs`; paths separated by `:` (`;` on Windows) |
| `NO_COLOR` | `output.color` | Disable colored output (any value) |
| `WARDEN_VERBOSE` | `output.verbose` | Enable verbose mode (any value) |

## CLI Flags

| Flag | Scope | Description |
|------|-------|-------------|
| `--config` | Global | The config file to read, the only one that is (see [The config file](#the-config-file)) |
| `--driver` | Global | Orchestration driver (container, qemu, vz, hyperv, firecracker) |
| `--runtime` | Global | Container runtime: finch, docker or podman (see [The container runtime](#the-container-runtime)) |
| `--color` | Global | Color mode |
| `-v, --verbose` | Global | Verbose output |
| `-o, --output` | build/shell | Output directory |
| `--capture` | build/shell | Payload capture mode |
| `--no-compress` | build/shell | Disable compression |
| `--timeout` | build/shell | Maximum build duration (all drivers) |
| `--image` | build/shell | Base disk image for the build VM (VM drivers) |
| `--script` | build/shell | Build script (VM drivers) |
| `--guest-os` | build/shell | Guest OS: linux (default) or windows (qemu, hyperv) |
| `--cpus` | build/shell | Build VM vCPUs (VM drivers) |
| `--memory` | build/shell | Build VM memory in MiB (VM drivers) |
| `--network` | build/shell | Durable switch to reuse (hyperv) |
| `--keep-vms` | build/shell | Leave VMs and work dir for inspection (VM drivers) |
| `--tmpdir` | build/shell | Directory for each build's private work dir (default: OS temp dir) |
| `--build-link` | build/shell/doctor | Advanced: the VM drivers' relay-to-build link, a /30 (see Advanced options) |
| `-q, --quiet` | build | Do not echo the guest build console |

## Advanced options

### The build link (`[vm] build_link`, `--build-link`)

Every VM driver joins the build VM to the relay over one private link, a
`/30`: the relay is its first host and the build VM its second. The default
is `100.127.255.252/30` (relay `100.127.255.253`, build VM
`100.127.255.254`), the last `/30` of RFC 6598 shared address space, which no
common LAN, VPC or resolver plan uses. The container driver has its own
per-build network and ignores a valid setting. A malformed `[vm] build_link`
is refused when the config file loads, by every command and with every
driver (container included), as any config error is.

Inside the relay the link is a connected route, so an upstream address inside
it (a resolver, a private mirror, a collector) is sent to the build VM
instead of upstream. Move the link only when your network uses the default
anyway:

```toml
[vm]
build_link = "100.126.255.252/30"
```

or, for one command, `--build-link 100.126.255.252/30` on `build`, `shell`
or `doctor`. Precedence: the flag, then the config file, then the default.
`warden doctor` prints the link and where it came from.

The value must be an IPv4 network address with prefix exactly `/30`
(`100.126.0.1/30` is refused with "did you mean 100.126.0.0/30"), outside
`0.0.0.0/8`, loopback, link-local, multicast and class E, and disjoint from
the ranges BuildWarden uses itself: the container driver's `100.64.87.0/24`,
Firecracker's upstream `100.64.88.0/22` and qemu's user-mode `10.0.2.0/24`. A
key is checked when the file loads and the flag when it is read, before any
work. Hyper-V's NAT is not a fixed range: it is read back from the host, so
the hyperv driver checks the link against the host's NAT at build start,
before the seed ISO or any per-build network, and in doctor, naming it (or the default `192.168.240.0/20` when the host has no NAT
yet, which is where a build would make one). Every VM build
also refuses a link that holds an address of one of the host's interfaces;
a host network that merely contains the link (a `/10` CGNAT WAN, say) is
only a doctor warning.

The link reaches the relay in its claim and the build VM in its seed, its
kernel command line or the relay's DHCP lease, per build: changing it needs
no rebuild, refresh or re-prepare. The one-time upgrade to this scheme is in
[Drivers](drivers.md#upgrading-to-the-configurable-build-link).

### The relay's resolvers (`[vm] dns`)

The relay answers the build's DNS and resolves its own upstream connections
through one list of upstream DNS servers, fixed when the build starts and
sent to the relay in its claim. Every VM driver sends one:

- **Unset (the default): the host's resolvers.** Each driver reads the
  host's DNS servers and keeps those its relay can reach: hyperv the IPv4
  servers of the default route's interface, firecracker `/etc/resolv.conf`
  (or systemd-resolved's upstreams), qemu the host's `/etc/resolv.conf`, vz
  the Mac's `/etc/resolv.conf`. A loopback or link-local server, one inside
  the build link and, per driver, one on the host itself or inside its NAT
  are left out; when none is left the build is refused, naming each server
  and why, with the fix (an off-host DNS server on the host, or `[vm] dns`).
  No public resolver stands in.
- **Set: used exclusively.** The listed servers, in order, are the relay's
  only resolvers: the host's are not read, added or tried when the listed
  ones fail.

  ```toml
  [vm]
  dns = ["10.20.0.53", "10.20.0.54"]
  ```

  The relay asks the first server and the next when one does not answer in
  time, as `resolv.conf`'s `nameserver` lines are asked; that is the one
  fallback, and it stays inside your list.

The key has no flag or environment variable: the config file's value is the
one used. Each entry must be an IPv4 address, not
unspecified, multicast or broadcast, and the list must not be empty
(`dns = []` is refused: remove the key to use the host's resolvers); this is
checked when the file loads, by every command. At build start and in
`warden doctor`, the driver refuses an entry its relay cannot reach (a
loopback server for the VM drivers whose relay is a VM, one inside the build
link, and so on), naming it, rather than drop it. The container driver
ignores `[vm]` and its relay uses its container's resolvers. `warden doctor`
prints the resolvers and where they came from.

## The container runtime

The container driver runs one of three runtimes: `finch`, `docker` or
`podman`. The runtime setting takes one of those three names, written as
they are here (lower case, no directory, no `.exe`), or is left unset. The
values are the same wherever the runtime is named. The highest of these
levels that names a runtime is the one in effect:

1. `--runtime`
2. `WARDEN_CTR_CLI`
3. `[runtime] cli` in the config file

An empty `--runtime` or `WARDEN_CTR_CLI` is not set, and the next level is
read. `cli = ""` in the file is not set either, and the runtime is detected.

`warden` runs the runtime by its name, which is looked up on `PATH`. A
runtime installed outside `PATH` needs its directory added to `PATH`.

### Detection

When no level names a runtime, BuildWarden probes `finch`, then `docker`,
then `podman`. The first one that answers `<runtime> info` within 3 seconds
is used.

### Which commands check the value

Only a command that runs the container runtime checks the setting, and it
checks the value in effect:

| Command | The runtime setting |
|---------|---------------------|
| `warden build`, `warden shell` and `warden doctor` with the container driver | checked |
| `warden clean` with the container driver, `qemu` or `vz` (under all three it removes leftover containers) | checked |
| `build`, `shell` and `doctor` with a VM driver; `clean` with `hyperv` or `firecracker`; `setup`; `image prepare` | not read |

So one `warden.toml` can hold the settings of several drivers:
`warden build --driver qemu` does not fail on a `[runtime] cli` it does not
use, and `--runtime` is accepted and ignored there. A value that a higher
level replaces is not checked either. With an unsupported `cli` in
`./warden.toml`, `warden build --runtime docker` builds with Docker, which is
also the way to build someone else's repository without editing its file.

### A value that is not supported

A value in effect that is not `finch`, `docker` or `podman` is an error. The
command stops before it looks up or runs anything and before it makes an
output directory, a container or a network, and `warden` exits 1. The message
says where the value was set and what to change:

```
[warden] ERROR: [runtime] cli in /src/repo/warden.toml is "dokcer"; the container driver supports finch, docker and podman
  To fix: set it to finch, docker or podman, or remove it; --runtime with one of those replaces it for one command
```

No other value is tried in its place: neither a lower level's nor a detected
runtime.

<!-- TRANSITIONAL(runtime-former-values): this subsection is removed with
     cmd/warden/runtime_former.go, in the change that prepares 1.0. -->
### A path, or another runtime

Earlier releases ran whatever program the setting named, from every level.
A repository's `warden.toml` could therefore make `warden` run a program of
the repository's own on the host. The setting is now limited to the three
runtimes the container driver supports, and these values stopped working:

| Before | Now |
|--------|-----|
| `--runtime /path/to/docker`, `WARDEN_CTR_CLI=/usr/local/bin/podman`, or `cli = "/opt/homebrew/bin/finch"` in a config file | Refused. Name the runtime, and put its directory on `PATH` |
| `--runtime docker.exe`, `Docker` | Refused. Write `docker` |
| `WARDEN_CTR_CLI=nerdctl`, or another runtime's program | Refused. There is no supported way to use another runtime with the container driver |
| A wrapper script, by path or by name, that only sets variables and then runs a supported runtime (`docker-ci` setting `DOCKER_HOST`) | Refused. Name the runtime and set the wrapper's variables (`DOCKER_HOST`, `DOCKER_CONTEXT`, `CONTAINER_HOST`) in the environment |
| A wrapper that does anything else: `sudo docker`, `podman --remote`, `nerdctl --namespace`, a script that picks a runtime | Refused. There is no supported replacement |
| `cli = "./tools/docker"` or `cli = "sh"` in a project's `warden.toml` | Refused. This is the hole the limit closes |

A value in one of these forms gets a message that says what to change in
place of the list of supported values: a supported runtime in another letter
case or with `.exe`, a path to one, `nerdctl`, and a path to any other
program. For example:

```
[warden] ERROR: WARDEN_CTR_CLI is "/opt/homebrew/bin/docker"; the container runtime is named, not given as a path
  To fix: set WARDEN_CTR_CLI to docker; warden looks docker up on PATH, so if it is installed outside PATH, add its directory to PATH
```

A wrapper named without a path cannot be told from a typing error and gets
the error of the section above.

The container driver runs the runtime by its name with `warden`'s arguments
and nothing else, so a wrapper that added `sudo` or arguments has nothing to
move to: the runtime has to be reachable by its name alone. For `nerdctl`,
Finch is the supported runtime built on it. A file named `docker` on your own
`PATH` that runs something else is run as `docker`; that arrangement is
neither tested nor supported.

## Driver Selection

When `driver` is empty (the default), the **container** driver is used. Use `--driver` or `WARDEN_DRIVER` to select a VM driver:

- `container` — Unprivileged container with iptables isolation (default)
- `qemu` — Two-VM topology, cross-platform, requires `qemu-system-*`
- `vz` — macOS/arm64 only, uses Virtualization.framework; needs a `warden`
  built from source (a released one has no vz driver)
- `hyperv` — Windows, two-VM topology on Hyper-V with a host-only NAT switch
- `firecracker` — x86_64 Linux with KVM, two Firecracker microVMs on a network
  slot made by `sudo warden setup --driver firecracker`; no config keys of its own

## Relay Image

For release builds (`warden --version` shows a version tag), the relay is pulled as a pre-built container image. For development builds (`version = "dev"`), it's compiled from source.

Override with `relay_image`, or `WARDEN_IMAGE`, which replaces it:
- `"dev"` — force build from source (requires Go toolchain, and a `warden`
  run from its source tree: `dist/warden` after `make build`)
- `"ghcr.io/buildwarden/relay:v0.1.0"` — pin a specific version
- `""` — automatic (pull for releases, build for dev)

Any other value must be shaped like an image reference: one that starts with
`-` or holds a space or a control character (any character that does not
print, a right-to-left override or a zero-width space among them) would reach
the container runtime's command line as an option. It is refused, with the container
driver, by `build`, `shell` and `doctor` before anything is created:

```
[warden] ERROR: [runtime] relay_image in /src/repo/warden.toml is "--volume=/:/host": not an image reference
  To fix: set it to an image reference such as ghcr.io/buildwarden/relay:v0.2.0 or to "dev", or remove it
```

Nothing more of a reference is checked: the runtime's own error says what is
wrong with a well-shaped one.
