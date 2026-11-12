# Corporate Networks

Some corporate networks filter or proxy specific traffic. This page covers common workarounds.

## Go Module Proxy

If your network blocks `proxy.golang.org`, set `GOPROXY=direct` in your Dockerfile to download modules directly from source repositories:

```dockerfile
RUN GOPROXY=direct GONOSUMCHECK=* GONOSUMDB=* go mod download
```

If your organization runs an internal Go module proxy:

```dockerfile
ENV GOPROXY=https://goproxy.corp.example.com
RUN go mod download
```

## Internal Container Registries

If your base images come from an internal registry, they'll work normally — BuildWarden intercepts all HTTPS traffic regardless of hostname:

```dockerfile
FROM registry.corp.example.com/base/golang:1.26
```

The relay will MITM the TLS connection to your internal registry and record all layer pulls in the ledger.

## HTTP Proxies

BuildWarden doesn't currently support explicit forward proxy (`http_proxy`/`https_proxy`) mode. All traffic is intercepted transparently via iptables DNAT. If your network requires an explicit proxy to reach the internet, the relay handles this — it has unrestricted outbound access and forwards traffic upstream.

## DNS Filtering and Internal Resolvers

The relay answers the build's DNS queries itself and forwards them upstream.
With the container driver it uses the resolvers of its own container, as the
runtime's network gives them. The VM drivers give their relay the host's
resolvers by default, so a build resolves names the way the machine it runs
on does, internal names included: Hyper-V reads the IPv4 DNS servers of the
default-route interface, Firecracker the host's `/etc/resolv.conf` (or
systemd-resolved's upstreams), qemu the host's `/etc/resolv.conf` and vz the
Mac's. There is
no public fallback: a network that blocks public DNS still works, and a
domain your corporate resolver blocks is blocked in the build too, the same
as building without BuildWarden.

Name the resolvers yourself when the host's are not the ones a build should
use: a VPN's split-DNS resolvers (macOS leaves its per-domain resolvers out
of `/etc/resolv.conf`), a filtering resolver only builds go through, or a
host whose only resolver is local to it. In `warden.toml` or
`~/.config/warden/config.toml`:

```toml
[vm]
dns = ["10.20.0.53", "10.20.0.54"]
```

The list is used exclusively and in order: the relay asks the next server
only when one does not answer in time, and never falls back to the host's.
An entry the relay cannot reach (a loopback address for a relay VM, one
inside the build link, on Hyper-V the host itself or its NAT, and so on)
refuses the
build before any VM boots, naming it. `warden doctor --driver <d>` prints
the resolvers a build would use and where they came from. The details are in
[Configuration](./configuration.md#the-relays-resolvers-vm-dns).

## Address Collisions with the Build Link

The VM drivers join the build VM to the relay over a private `/30`, the build
link, by default `100.127.255.252/30` (RFC 6598 shared address space, which
LANs, VPCs and VPNs normally leave alone). Inside the relay that link is a
connected route, so anything upstream at one of its four addresses (a
resolver, a private mirror, a collector, a VPN or Tailscale peer) cannot be
reached from a build: the relay refuses to dial it.

Most networks never meet this. Move the link when yours does:

- `warden doctor` prints `build link: BLOCKED: host interface address ... is
  inside the link`, or a build refuses to start because the link holds a host
  interface address;
- on Firecracker, a build stops before booting because every host resolver is
  inside the link;
- a fetch from an internal host whose address is inside the link fails
  while the same fetch works outside BuildWarden.

Pick any free `/30` and check it before you commit to it:

```sh
warden doctor --driver qemu --build-link 100.126.255.252/30
```

Then set it in `warden.toml` (or `~/.config/warden/config.toml`), so every
build and `warden doctor` use it:

```toml
[vm]
build_link = "100.126.255.252/30"
```

Changing the link needs no rebuild, refresh or re-prepare. A host network
that only contains the link, such as a carrier-grade NAT WAN on
`100.64.0.0/10`, is a doctor warning, not a collision: only the link's own
four addresses are shadowed. The rules a value must satisfy, and the
precedence of the flag and the two files, are in
[Configuration](./configuration.md#the-build-link-vm-build_link---build-link).

## Address Collisions with the Hyper-V NAT

The Hyper-V relay VM reaches upstream through warden's Internal NAT switch,
by default on `192.168.240.0/20` with the host at `192.168.240.1`. Each
build's relay takes its own free address in that prefix. If your LAN, VPN or
a mirror you fetch from uses addresses in it, set the durable network up on
another prefix, from an elevated terminal:

```sh
warden setup --driver hyperv --nat-prefix 192.168.250.0/24 --host-ip 192.168.250.1
```

Every build then reads the NAT back from the host and addresses its relay
inside it; nothing else needs changing. Setup refuses a prefix that overlaps
the build link, and one another NAT on the host already owns, naming that
NAT and how to remove it. A build checks the build link against the NAT it
reads back too, by name, before any per-build work. Elevated and
service builds use the same NAT once setup has made it.

When setup makes the NAT, the host IP is the only IPv4 address it leaves on
the NAT's vNIC, `vEthernet (warden-dev-nat)`: an address from the old prefix
is removed, and no other interface is touched. `warden doctor --driver hyperv`
prints the NAT a build would use under `Build network`, and warns about any
other address on that vNIC, naming the `Remove-NetIPAddress` that removes it.

## Certificate Trust

The relay generates an ephemeral CA per build, and `warden-io` installs it in the build environment's trust store before your build runs. Tools that use the system certificate store (apt, curl, most language package managers) trust the relay automatically, and `warden-io` also points pip, npm, Maven, Gradle, Bazel and others at it. See [Trust and the Build Baseline](../concepts/baseline.md) for the full list.
