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

A privileged sidecar container shares the build container's network namespace.
It applies iptables rules then exits (Kubernetes init-container pattern). The
build container has no `CAP_NET_ADMIN` and cannot modify the rules after they
are applied.

The relay IP is on the same Docker bridge network as the build container.

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

- No `CAP_NET_ADMIN` — cannot flush or modify iptables rules
- No route to any host other than relay and loopback
- The sidecar exits after applying rules — no persistent privileged process

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
