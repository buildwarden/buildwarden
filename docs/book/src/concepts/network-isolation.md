# Network Isolation

BuildWarden enforces strict network isolation on the build container so that all traffic must flow through the relay for auditing.

## Isolation Layers

### Layer 1: iptables NAT (DNAT)

All outgoing HTTP, HTTPS, and DNS traffic is redirected to the relay:

```
# DNS (UDP and TCP)
iptables -t nat -A OUTPUT -p udp --dport 53 -j DNAT --to <relay>:53
iptables -t nat -A OUTPUT -p tcp --dport 53 -j DNAT --to <relay>:53

# HTTP
iptables -t nat -A OUTPUT -p tcp --dport 80 -j DNAT --to <relay>:80

# HTTPS
iptables -t nat -A OUTPUT -p tcp --dport 443 -j DNAT --to <relay>:443
```

PREROUTING rules mirror these for forwarded traffic from buildkit's internal network.

### Layer 2: iptables Filter (DROP)

After DNAT, only relay and loopback traffic is permitted:

```
iptables -A OUTPUT -d <relay_ip> -j ACCEPT
iptables -A OUTPUT -d 127.0.0.0/8 -j ACCEPT
iptables -A OUTPUT -p udp --dport 53 -j ACCEPT  # buildkit DNS forwarding
iptables -A OUTPUT -j DROP
```

### Layer 3: Route Replacement

As defense-in-depth, the default route is replaced to point at the relay:

```
ip route replace default via <relay_ip>
```

Even if iptables were somehow flushed, traffic would still route to the relay.

### Layer 4: FORWARD Chain

IP forwarding is enabled (for buildkit's internal network to reach the relay), but only to the relay:

```
iptables -A FORWARD -d <relay_ip> -j ACCEPT
iptables -A FORWARD -j DROP
```

## Why Rootless Docker Matters

The build container runs `docker:dind-rootless`. The inner Docker daemon and all build processes run as an unprivileged user (`rootless`). This user:

- Cannot modify iptables (no `CAP_NET_ADMIN`)
- Cannot change routes
- Cannot escape the network namespace

The iptables rules are applied by the orchestrator as root (via `--privileged` exec from outside), before any build code runs.

## DNS Configuration

The build container's Docker daemon is pre-configured (via `daemon.json`) to use the relay as its DNS server. This ensures that buildkit's internal DNS resolution flows through the relay, enabling full DNS auditing.

## Concurrent Builds

Each build gets a unique /29 subnet allocated from `100.64.87.0/24` (CGNAT range, unlikely to collide with user networks). The allocator probes for unused subnets, enabling multiple simultaneous builds on the same host.
