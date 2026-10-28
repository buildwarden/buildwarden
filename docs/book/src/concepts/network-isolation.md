# Network Isolation

BuildWarden enforces strict network isolation on the build environment so that
every byte entering or leaving passes through the relay and is recorded in the
ledger. The mechanism differs per driver, but the guarantee is identical.

## The Trust Boundary

All drivers enforce the same invariant: **every byte entering or leaving the
build environment passes through the relay and is recorded in the ledger.**

The relay is the single point of network egress. It terminates TLS (via an
injected CA), logs DNS queries, and writes network events to the signed ledger.
Whether the build environment is a container or a virtual machine, isolation
ensures no traffic can bypass this path.

## Container Driver — iptables Isolation

The build's network namespace is isolated before the build container exists,
in the Kubernetes pod pattern:

1. A **namespace holder** (`warden-holder`, a single static binary that does
   nothing) starts on the per-build bridge network and owns the namespace. It
   runs with no capabilities, a read-only root and no shell.
2. A privileged **sidecar** (`warden-netns`) joins the holder's namespace,
   applies the iptables rules below, and exits.
3. The **build container** is created from your `FROM` image inside that
   namespace (`--network container:<holder>`), with `warden-io initialize` as
   its entrypoint, and only then started.

Nothing from your image runs before the rules are in place, including the
image's own `ENTRYPOINT`, which is replaced. The build container has no
`CAP_NET_ADMIN` and cannot modify the rules. Release builds run the holder and
sidecar images by digest, so a different image carrying the same name is never
run privileged.

The relay IP is on the same bridge network as the holder.

### NAT rules (DNAT)

All outgoing HTTP, HTTPS, and DNS traffic is redirected to the relay:

```
iptables -t nat -A OUTPUT -p udp --dport 53 -j DNAT --to-destination <relay>:53
iptables -t nat -A OUTPUT -p tcp --dport 53 -j DNAT --to-destination <relay>:53
iptables -t nat -A OUTPUT -p tcp --dport 80 -j DNAT --to-destination <relay>:80
iptables -t nat -A OUTPUT -p tcp --dport 443 -j DNAT --to-destination <relay>:443
```

### Filter rules (DROP)

After DNAT, only relay and loopback traffic is permitted:

```
iptables -A OUTPUT -d <relay> -j ACCEPT
iptables -A OUTPUT -d 127.0.0.0/8 -j ACCEPT
iptables -A OUTPUT -j DROP
```

### Why the build container cannot escape

- Isolation precedes execution — no code from the image runs on an open network
- No `CAP_NET_ADMIN` — cannot flush or modify iptables rules
- No route to any host other than relay and loopback
- The sidecar exits after applying rules — no persistent privileged process
- The holder runs no code that could use the namespace before it is isolated

## QEMU Driver — Topological Isolation

Two VMs are connected by a Unix domain socket pair (QEMU `-netdev stream`).
There is physically no network path from the build VM to the internet that does
not pass through the relay VM.

### Network topology

- **Relay VM** has two interfaces:
  - `eth0`: socket to the build VM
  - `eth1`: user-net (QEMU SLIRP) to the internet
- **Build VM** has ONE interface: the socket to the relay VM

No iptables are needed on the build VM — there is no other path. The hypervisor
enforces the topology; the build VM cannot create new interfaces.

### Relay VM iptables

The relay VM uses iptables to ensure build traffic is processed by the relay
process before reaching the internet:

- `PREROUTING`: redirects inbound build traffic to the local relay process
- `FORWARD DROP`: prevents any unproxied egress from the build VM
- `POSTROUTING MASQUERADE`: NAT for the relay's own upstream connections
- `INPUT`: the build VM may reach the relay control plane (`:8300`) but is dropped from the claim port (`:8301`), so it cannot claim or re-claim the relay

## VZ Driver — Topological Isolation

The macOS build VM has a single `VZFileHandleNetworkDeviceAttachment` backed by
a socketpair. There is no vmnet.framework, no bridging — the socketpair IS the
entire network.

### How it works

- The relay runs on the host and reads raw Ethernet frames from its end of the
  socketpair via gvisor netstack
- The VM sees one network interface connected to the relay — nothing else
- SSRF filtering is active in host-mode relay (blocks connections to
  loopback/link-local/metadata IPs)
- The VM cannot create additional network interfaces
  (Virtualization.framework constraint)

### Why this is stronger than iptables

There is no software firewall to misconfigure or bypass. The isolation is
structural: the only network device the VM possesses connects exclusively to
the relay process. No amount of privilege inside the VM can change this.

## Hyper-V Driver — Topological Isolation

Like QEMU, the Hyper-V driver uses two VMs and the hypervisor enforces the
topology. The relay VM and build VM share a Private build switch; the relay
reaches upstream through an Internal NAT switch with a static host-only address
(`192.168.240.2/20`, created by `warden setup --driver hyperv`). The build VM
has one interface, on the Private switch, so its only path is the relay.

The same relay-VM iptables rules apply (the init script is shared with QEMU):
`PREROUTING` redirect, `FORWARD DROP`, `POSTROUTING MASQUERADE`, and an `INPUT`
rule that lets the build VM reach the control plane but drops it from the claim
port. The claim endpoint lives only on the host-only NAT switch, so the build
VM cannot reach it even before iptables.

## Common DNS Behavior

All drivers configure the build environment to use the relay as its DNS server:

- `artifacts` and `cwd` resolve to the relay itself (reserved hostnames for
  context transfer and artifact upload)
- All other queries are forwarded upstream and logged in the ledger
- IPv6 is disabled in VM drivers to prevent bypass via link-local addresses

## What Isolation Prevents

| Threat | How it is blocked |
|--------|-------------------|
| Direct internet access bypassing the relay | All traffic forced through relay by topology or iptables |
| DNS tunneling to external servers | All DNS queries go through the relay |
| Metadata service access (169.254.169.254) | Blocked by SSRF filter in host mode; unreachable in VM modes |
| Build environment modifying its own network | No CAP_NET_ADMIN (container); no additional interfaces (VM) |

## What Isolation Does NOT Prevent

These are availability concerns, not integrity violations — the ledger still
records everything:

- Build environment sending excessive traffic through the relay
- Build environment attempting to DoS the relay with many connections
- Build environment consuming all available bandwidth

The relay applies fairness scheduling to mitigate resource exhaustion, but these
are not considered security violations because the invariant (all traffic is
recorded) is preserved.
