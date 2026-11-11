# Relay VM

`init` is the init of every vm-mode relay VM: the qemu and Firecracker relay
initramfs (`build-initramfs.sh`) and the Hyper-V relay boot VHDX
(`hyperv/build.sh`, `hyperv/refresh-cache.sh`). It brings the relay VM's two
interfaces up, installs the iptables rules that force the build VM through
the relay, and starts `/sbin/relay -mode vm` unconfigured, waiting for one
claim on port 8301 (`relaycfg`). This page is the boot channel for an
orchestrator other than warden that boots these assets itself.

## What the boot channel carries

Only the relay's pre-claim values: what it needs to reach the network the
claim arrives on, and the token that authenticates the claim (RELAY-08).
Everything else, among them the build link (`build_link`), the collector and
the relay's upstream DNS servers (`upstream_dns`), goes in the claim.

Each value is a `warden.<key>=<value>` word. The init reads only these keys,
never evaluates the words, and stops on a malformed value with a console
line (on Hyper-V also on COM1): no relay starts, so the claim times out with
that line as the evidence.

| Key | Value | Checked |
|---|---|---|
| `warden.net` | `dhcp` (default) or `static`: how the upstream interface is addressed | one of the two |
| `warden.nat_ip` | the relay's static upstream address, where it accepts the claim | a dotted quad |
| `warden.nat_prefix` | its prefix length | 1 to 30 |
| `warden.nat_gw` | its default gateway | a dotted quad |
| `warden.claim_token` | the claim token, passed to the relay as `WARDEN_CLAIM_TOKEN` and never printed | 64 lowercase hex digits |
| `warden.seed` | the relay seed's format version (Hyper-V) | `1` |

`warden.net=static` needs all three of `warden.nat_ip`, `warden.nat_prefix`
and `warden.nat_gw`; there are no defaults. With `warden.net=dhcp` the init
runs `udhcpc` on the upstream interface and stops when it gets no lease.

## Where the keys come from

- **qemu and Firecracker:** the kernel command line.
- **Hyper-V:** a Gen2 VM gives the host no per-boot kernel command line, so
  the keys come from the **relay seed**, a small ISO 9660 image with volume
  label `WARDENRELAY`, attached to the relay VM as a DVD before it starts. It
  holds one file, `warden-relay.conf`, one `warden.<key>=<value>` per line,
  among them `warden.seed=1`:

  ```
  warden.seed=1
  warden.net=static
  warden.nat_ip=192.168.240.7
  warden.nat_prefix=20
  warden.nat_gw=192.168.240.1
  warden.claim_token=<64 lowercase hex digits>
  ```

  The init waits up to 10 s for the CD, mounts it read-only, copies the file
  and unmounts it. No CD, no `warden-relay.conf`, or no `warden.seed=1` stops
  the init, so a relay boot VHDX booted with no seed starts no relay. warden
  writes a fresh seed per build (`driver/hyperv/relayseed.go`) with the
  relay's own address on the NAT and a fresh token. Hyper-V's NAT gives no
  DHCP lease, so a seed names a static upstream.

## The relay's resolvers

`warden.dns` is retired. An init that finds it stops, naming `upstream_dns`
in the claim, rather than ignore a value someone set. The init writes no
`/etc/resolv.conf` of its own: on a DHCP upstream `udhcpc`'s script writes
the lease's servers, and on a static upstream there is none.

The relay takes its resolvers from the claim's `upstream_dns`, a non-empty
list of IPv4 addresses, asked in order and used exclusively. A claim without
it takes the relay's named default, the usable nameservers of the relay VM's
`/etc/resolv.conf` (not the relay's own address, `127.0.0.1` or the build
link), so a relay on a **static upstream must be claimed with
`upstream_dns`**: without it the claim is refused with 400 naming the field,
and the relay stays claimable.
A relay built before the field refuses a claim carrying it as an unknown
field; rebuild the asset.
