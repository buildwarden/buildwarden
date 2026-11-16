# Configuration

BuildWarden looks for configuration in two places, merged in order:

1. `~/.config/warden/config.toml` — user-level defaults
2. `./warden.toml` — project-level overrides

CLI flags override everything.

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
upstream_ca_certs = []     # Paths to additional CA cert bundles for upstream
system_ca_bundle = true    # Include host system CA bundle in relay trust store

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

## Environment Variables

| Variable | Overrides | Description |
|----------|-----------|-------------|
| `WARDEN_DRIVER` | `runtime.driver` | Override driver selection |
| `WARDEN_IMAGE` | `runtime.relay_image` | Relay image override |
| `WARDEN_CTR_CLI` | `runtime.cli` | Container runtime: finch, docker or podman (see [The container runtime](#the-container-runtime)) |
| `WARDEN_UPSTREAM_CA_CERTS` | `relay.upstream_ca_certs` | Extra upstream CA files/dirs (colon-separated) |
| `NO_COLOR` | `output.color` | Disable colored output (any value) |
| `WARDEN_VERBOSE` | `output.verbose` | Enable verbose mode (any value) |

## CLI Flags

| Flag | Scope | Description |
|------|-------|-------------|
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
or `doctor`. Precedence: the flag, then `./warden.toml`, then
`~/.config/warden/config.toml`, then the default. `warden doctor` prints the
link and where it came from.

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

The key has no flag or environment variable; `./warden.toml` wins over
`~/.config/warden/config.toml`. Each entry must be an IPv4 address, not
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
3. `[runtime] cli` in `./warden.toml`
4. `[runtime] cli` in `~/.config/warden/config.toml`

An empty `--runtime` or `WARDEN_CTR_CLI` is not set, and the next level is
read. `cli = ""` in a file is that file's value: in `./warden.toml` it hides
the user-level file's value, and the runtime is detected.

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
[warden] ERROR: [runtime] cli in warden.toml is "dokcer"; the container driver supports finch, docker and podman
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
| `--runtime /path/to/docker`, `WARDEN_CTR_CLI=/usr/local/bin/podman`, or `cli = "/opt/homebrew/bin/finch"` in either file | Refused. Name the runtime, and put its directory on `PATH` |
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
- `vz` — macOS/arm64 only, uses Virtualization.framework
- `hyperv` — Windows, two-VM topology on Hyper-V with a host-only NAT switch
- `firecracker` — x86_64 Linux with KVM, two Firecracker microVMs on a network
  slot made by `sudo warden setup --driver firecracker`; no config keys of its own

## Relay Image

For release builds (`warden --version` shows a version tag), the relay is pulled as a pre-built container image. For development builds (`version = "dev"`), it's compiled from source.

Override with `relay_image`:
- `"dev"` — force build from source (requires Go toolchain)
- `"ghcr.io/buildwarden/relay:v0.1.0"` — pin a specific version
- `""` — automatic (pull for releases, build for dev)
