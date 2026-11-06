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

## DNS Filtering

The relay forwards DNS queries to the upstream resolver configured in the container runtime's network. If certain domains are blocked at the DNS level by your corporate resolver, those blocks will be reflected in the build (same as building without BuildWarden).

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

## Certificate Trust

The relay generates an ephemeral CA per build, and `warden-io` installs it in the build environment's trust store before your build runs. Tools that use the system certificate store (apt, curl, most language package managers) trust the relay automatically, and `warden-io` also points pip, npm, Maven, Gradle, Bazel and others at it. See [Trust and the Build Baseline](../concepts/baseline.md) for the full list.
