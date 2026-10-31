# BuildWarden Security Review — October 2026

Ad-hoc security review. Scope: BuildWarden as a secure build substrate that runs
**customer-defined build scripts** inside an isolated environment, under
something like AWS CodeBuild or GitHub Actions. No server is exposed to the
internet; the attack surface under review is the untrusted build script and the
build environment it controls.

The review covers, in order: (1) the architectural design, (2) the relay
implementation, and (3) each per-driver build environment. Findings are ranked
by relative risk = probability × severity. Theoretical issues that cannot be
truly eliminated (kernel 0-days; DoS against an ephemeral single-tenant relay —
already out of scope per `SECURITY.md`) are noted only where they change how a
real deployment should be configured.

The threat model is BuildWarden's own (`docs/development/security-suite.md`,
SEC-01):
- **Class 1 — escape / influence beyond the boundary** (host OS, relay keys, the
  network past the relay).
- **Class 2 — measurement evasion** (bytes move in or out without the ledger
  faithfully recording them).

> **Verification.** Every finding below was independently checked against the
> source by an adversarial pass (each claim handed to a skeptic told to refute
> it from the code; the two High findings to two skeptics with different
> lenses). Findings A, B, C, D, F, G, H were confirmed verbatim; E and I were
> corrected (noted inline); J and K were added by a completeness critic that the
> first-pass review had missed. Line numbers are as of the reviewed tree and are
> meant as starting points for follow-up, not fixed addresses.

This document is intended to be triaged through the project's normal workflow
(intake → design → implement), one item at a time.

---

## Summary of findings

| # | Finding | Class | Probability | Severity | Risk |
|---|---------|-------|-------------|----------|------|
| A | DNS queries/answers are proxied upstream but **never written to the ledger** | 2 | High | High | **High** |
| B | Relay has **unrestricted egress to the host's own network** (cloud IMDS / RFC1918) in container and QEMU modes | 1 | High on cloud CI | High | **High** |
| J | Container driver isolates **IPv4 only**; with daemon IPv6 enabled the build egresses directly past the relay, unrecorded | 1+2 | Med–High (daemon-dependent) | High | **High (conditional)** |
| C | Hyper-V claims its relay with an **empty claim token** (container/QEMU use a per-build token) | 1 | Low–Med | High | **Medium** |
| D | Ledger **metadata (URL, method, headers) is unsigned**, so the hash↔source binding is not tamper-evident | 2 | Med | Med | **Medium** |
| E | `warden inspect --extract` **follows relative `../` capture symlinks** → arbitrary host-file read on an attacker-supplied output dir | 1 | Low–Med | Med | **Medium** |
| F | Guest-controlled **build-output.log and exit code** via the unauthenticated control plane | 2 | High | Low | **Low–Med** |
| K | Guest `/v1/output` bytes are **teed verbatim to the operator's live terminal** → terminal-escape injection | 1 | Med | Low | **Low** |
| G | Build container runs **root + default caps**; `no-new-privileges`/cap-drop applied only to the holder | 1 | Low | Med | **Low** |
| H | `/tmp/warden-ca.pem` predictable path; exclusive-trust silently downgraded on macOS/Windows | 1/2 | Low | Low | **Low** |
| I | Ed25519-over-SHA-512 prehash and MD5/SHA-1 in the hash block — benign, document them | — | — | — | **Note** |

Three findings (A, B, J) are unrecorded-channel or host-egress breaks; A, B also
**contradict statements in the shipped docs** (`docs/book/src/concepts/network-isolation.md`),
so they are documentation-accuracy bugs as well.

---

## 1. Architectural design

The core shape is sound, and the strongest parts are real guarantees:

- **Push-claim over a host-initiated tunnel** (`relaycfg`): the relay never dials
  the host and the host never listens on the network. The claim listener closes
  after exactly one claim, before the guest boots.
- **Deliver-one-binary bootstrap** (`docs/development/bootstrap.md`): the only
  thing the host puts in the environment is `warden-io`; everything else arrives
  through the relay and is recorded. Isolation precedes execution in every driver.
- **Diskless relay, keys memory-only** (RELAY-03): the Ed25519 ledger key and the
  MITM CA key are generated per relay, never written to disk, and the ledger key
  is zeroed on `Finish()`. Core dumps are disabled at process start.
- **Streaming chained ledger**: a relay that dies mid-build still leaves a
  truncated-but-verifiable ledger, since every record is individually chained.

### Finding A (High) — DNS is not recorded in the ledger

`relay/dns.go` `newDNSHandler` (≈lines 39–94) resolves the reserved names to the
relay and forwards every other query upstream (`resolveViaSystem` ≈lines 96–156,
then `dnsClient.Exchange` ≈line 74). **No code path in the DNS handler writes to
the ledger.** Every ledger write in the package is in `relay/relay.go` (the HTTP,
artifact, environment and served-input paths); the DNS goroutine, started
separately at `relay/relay.go:213`, reaches none of them, and there is no DNS
record schema.

Both shipped docs claim the opposite:
- `docs/book/src/concepts/overview.md`: "anything else -> upstream, recorded".
- `docs/book/src/concepts/network-isolation.md`: "logs DNS queries, and writes
  network events to the signed ledger" / "All other queries are forwarded
  upstream and logged in the ledger."

Impact: DNS is a fully unmonitored, bidirectional channel. A build can exfiltrate
by encoding data in query labels to an attacker's authoritative name server and
pull data back in the answer, with **nothing in the ledger** — not the payload,
not even the destination. This is the Class-2 "DNS-tunnel through the relay
resolver" cell of SEC-01, and it defeats the completeness promise (RELAY-02) for
any build that tunnels over DNS. The egress surface is otherwise tightly limited
to HTTP/HTTPS/DNS, which makes the one unrecorded protocol disproportionate.

Recommendation: record each forwarded DNS query and its answer as a ledger
channel (question name/type as metadata, answer bytes hashed like a body), or add
a dedicated DNS record schema. Until then, correct the two docs — claiming DNS is
recorded when it is not is worse than silence. Crystallize as an Arm-A regression
test (SEC-03).

### Finding B (High) — relay egress reaches the host's own network / cloud metadata

The egress IP filter (`relay/proxy.go` `isBlockedIP` ≈lines 91–114, wired via
`safeDialContext`) is attached to the upstream transport **only when `cfg.SSRF`
is true** (`relay/proxy.go:55-57`), and SSRF is set true **only in host mode**
(`cmd/relay/mode_host.go:67`). In `vm` and `container` modes, `cmd/relay/main.go`
(≈lines 52–55) runs `serveClaim(..., relay.Config{}, nil)` with SSRF unset
(false), so the transport uses Go's default unrestricted dialer. The QEMU relay
VM's upstream is `-netdev user,...,restrict=off` (`driver/qemu/vm.go` ≈lines
32–34), and the relay VM's iptables (`tools/relay-vm/init` ≈lines 132–150) has
`FORWARD DROP` + `POSTROUTING MASQUERADE` but **no OUTPUT-chain filter**, so the
relay process's own locally-generated connections egress unrestricted.

`network-isolation.md` states metadata access is "Blocked by SSRF filter in host
mode; **unreachable in VM modes**". That is inaccurate for container and QEMU.

Impact (the headline risk for the stated deployment): CodeBuild / GitHub Actions
runners expose an instance metadata service (IMDS) that vends the runner's IAM /
workload credentials. A build asks the relay to fetch `http://169.254.169.254/...`;
because the guest-side redirect matches on destination port, not IP, the relay
proxies it (and records it — this is Class 1, not evasion) and hands the host's
cloud credentials to the untrusted build. The ledger records the theft, but the
"influence beyond the relay" boundary is broken.

Recommendation: apply `isBlockedIP` to the upstream transport in **all** relay
modes, not just host mode; treat the filter as the egress policy rather than a
VZ-only backstop. Add IMDS (v1/v2, IPv4 and the IPv6 `fd00:ec2::254`) and the
GCP/Azure metadata hostnames explicitly, and make reaching them a hard Arm-A test
on every driver. Correct `network-isolation.md`.

### Finding J (High, conditional) — container isolation is IPv4-only; IPv6 egresses unrecorded

`driver/container/container.go` `isolationRules` (≈lines 475–485) installs only
`iptables` (IPv4) rules — DNAT of 53/80/443 to the relay, ACCEPT to the relay and
loopback, then `iptables -A OUTPUT -j DROP`. There is **no `ip6tables`
counterpart**, and the holder that owns the netns is started (≈lines 450–461)
without `--sysctl net.ipv6.conf.all.disable_ipv6=1`. The bridge network
(`driver/subnet.go` ≈lines 44–51) is created with an IPv4 subnet only, so warden
neither enables nor suppresses IPv6 — it inherits the daemon default.

Impact: if the Docker/nerdctl daemon has IPv6 enabled (`daemon.json "ipv6": true`
/ `--ip6tables`, increasingly the default), the build netns gets a v6 address and
default route, and the IPv4 catch-all `OUTPUT -j DROP` the whole isolation story
rests on simply does not apply to v6. The untrusted build can then open arbitrary
IPv6 TCP/UDP/ICMPv6 to the internet — alternate DNS, raw egress, QUIC, IMDS at
`fd00:ec2::254` — with the relay never seeing a byte. This is SEC-01's
build-env ↔ beyond-relay "direct egress bypassing the relay (IPv6)" cell, and
because v6 build networks can share space with no DROP, potentially the "reaching
other builds" cell too. It is both a Class-1 escape and a Class-2 evasion.

The VM drivers do not share this gap — their guest init explicitly disables IPv6
(`tools/relay-vm/init`, `tools/build-vm/init`: `disable_ipv6=1`). Only the
container driver is exposed.

Recommendation: disable IPv6 on the holder netns (`--sysctl
net.ipv6.conf.all.disable_ipv6=1` and `.default.disable_ipv6=1`) **and/or** add a
matching `ip6tables` ruleset (DNAT the three ports to the relay, else DROP) so the
drop is enforced regardless of daemon IPv6 state. Belt-and-suspenders is right
here: the invariant must not depend on a daemon config the operator may flip.
Add an Arm-A test that boots with daemon IPv6 on and asserts no v6 egress.

### Design note D (Medium) — provenance is not part of the integrity guarantee

The ledger writer's signing loop (`relay/ledger.go` ≈lines 257–307) signs only
`type ‖ prev_sig ‖ open_sig ‖ payload_size ‖ hash_block`; the verifier
(`ledger/ledger.go` `buildRecordSigInput`) reconstructs the same fields. The CBOR
metadata — which holds the request **URL, method, and headers** (built in
`relay/relay.go` `onRequest` ≈lines 496–501) — is appended to the serialized
record **after** signing and is never in the signature input. This is deliberate
(the ledger spec calls metadata redactable), but the security consequence is
under-stated: the binding between a signed content hash and the **source it came
from** is not tamper-evident. A ledger holder can rewrite "this hash came from
`evil.example`" to "...from `pypi.org`" without breaking the chain.

For "completeness of inputs" this is defensible (the bytes the build consumed are
signed), but anyone treating the ledger as a provenance attestation is relying on
unsigned data. Recommendation: state this limit plainly in `ledger-spec.md`, and
consider a signed-metadata mode (or signing a hash of the metadata) for
supply-chain use. SEC-01 "smuggle via unsigned CBOR metadata" cell.

### Note I — signature construction and hash block

- `relay/ledger.go` `sign()` (≈lines 309–312) computes
  `ed25519.Sign(key, SHA512(input))`. Ed25519 already hashes internally; signing
  a pre-hash is a non-standard construction — **not** RFC 8032 Ed25519ph (no `ph`
  domain-separation context). It is not a vulnerability (forgery still requires
  breaking Ed25519 or SHA-512) but should be documented as "Ed25519 over
  SHA-512(message)" so third-party verifiers reproduce it exactly and do not
  mistake it for Ed25519ph. (`ledger/ledger.go` holds only the verify side,
  `verifyEd25519Sig`.)
- The default hash block is `blake2b_256 + sha256 + sha1 + md5`
  (`relay/ledger.go` `defaultHashes` ≈line 71). Integrity rests on the full signed
  block, and MD5/SHA-1 are present only for registry cross-referencing. Keep that
  framing visible wherever the hash block is described so MD5/SHA-1 are never read
  as the security anchor.

> Correction from the verification pass: an earlier draft cited these in
> `ledger/ledger.go`. The production signing code and `defaultHashes` are in
> `relay/ledger.go`; `ledger/ledger.go` is the reader/verifier.

---

## 2. Relay implementation

The relay is careful: constant-memory streaming with size caps everywhere
(`maxOutputBytes` 256 MiB, artifact 2–4 GiB, environment 1 MiB), single-writer
ledger via a channel (RELAY-01), auth headers redacted in recorded metadata
(`isAuthHeader`), bodies hashed as the identical stream that is served (RELAY-02),
upstream TLS actually verified (no `InsecureSkipVerify` anywhere in the tree), and
the collector bearer token compared in constant time (`crypto/subtle`). The claim
body is bounded (`io.LimitReader` 16 MiB) and strict (`DisallowUnknownFields`).

### Finding F (Low–Med) — the guest controls the console log and the exit code

The control plane on `:8300` (`relay/controlplane.go` `runControlPlane`) is
guest-facing and unauthenticated by design (plain `http.ServeMux`, no token, no
TLS). The in-guest agent posts to it at `gateway:8300` (`cmd/warden-io/initialize.go`).
A build can therefore POST arbitrary bytes to `/v1/output` (bounded, but it
pollutes `build-output.log`), and POST `/v1/complete?code=0` (or hit
`http://artifacts/exit?code=0`) to report success and trigger teardown regardless
of what the script did.

This is inherent to the model (the build is the exit code's author), but it means
`build-output.log` and the reported exit code are attacker-influenced and must not
be used as security signals — only the signed ledger is trustworthy.
Recommendation: document it, and have the orchestrator treat an exit that arrives
before the guest's run phase as suspicious rather than authoritative.

### Finding K (Low) — the guest console is teed to the operator's live terminal

The same `/v1/output` bytes are teed **verbatim** to the host operator's terminal:
`collector/collector.go` `handleOutput` (≈lines 335–350) does
`io.MultiWriter(f, ignoreErrWriter{OutputTee})` with no sanitization, and
`driver.ConsoleTee` returns `os.Stdout` unless `--quiet`
(`cmd/warden/main.go`; wired by the container and qemu drivers). A malicious build
can therefore emit ANSI/OSC control sequences — screen rewrites, title/clipboard
OSC, prompt spoofing, or payloads targeting terminal-emulator parser bugs — that
execute in the operator's terminal while they watch the build. Finding F noted the
log file is untrustworthy; it missed that the same bytes are a live-terminal
injection surface.

Recommendation: strip or escape C0/C1 control bytes (keep `\n`/`\t`) before teeing
to a TTY, or sanitize at the collector. Low severity, cheap fix.

### Correctly handled

- **Re-claim is closed off**: the claim listener is one-shot and closes before the
  guest boots; by then `:8301` is gone. VM init drops the build interface from
  `:8301` even earlier.
- **`safeDialContext` resolves then dials by the resolved IP**, so there is no
  DNS-rebinding TOCTOU in the SSRF check (Finding B is that the filter is simply
  not installed in two of three modes, not that it is wrong).
- **Reserved hosts** (`artifacts`, `cwd`) are served locally, never proxied;
  artifact/context names are validated by shared rules (`collector/paths.go`), and
  the collector confines disk access to `os.Root` handles so a build-context
  symlink cannot escape to `~/.ssh`.
- **Cert-cache growth** and **connection fan-out** are unbounded (DoS against an
  ephemeral single-tenant relay) — explicitly out of scope; the fairness scheduler
  blunts bandwidth starvation. No action.

---

## 3. Per-driver build environments

### 3.1 Container driver (`driver/container`)

Isolation is the Kubernetes-pod pattern, implemented correctly: holder owns the
netns with `--cap-drop ALL --read-only --security-opt no-new-privileges`; a
`--privileged` sidecar applies iptables and exits; the build container is
**created, not started**, inside the holder's netns and only then started, so the
image's own `ENTRYPOINT` never runs on an open network. Release builds pin the
holder and sidecar images by digest, and the sidecar base is pinned by `sha256`.
The DNAT/DROP ruleset forces 53/80/443 to the relay and the legacy Docker DNS
rules are flushed first.

- **Finding J (High, conditional)** — IPv4-only isolation; see §1.
- **Finding B** applies here (relay egress to host IMDS).
- **Finding G (Low)** — the build container runs `--user 0` with the runtime's
  **default capability set** (`NET_RAW`, etc.) and neither `no-new-privileges` nor
  a cap-drop; the hardening is all on the holder (`createBuildContainer` ≈lines
  511–531 vs `startHolder` ≈lines 450–461). Within an `OUTPUT -j DROP` netns,
  `NET_RAW` does not buy an egress bypass, so this is not an isolation break on its
  own, but it widens the container-escape surface the project already rates weaker
  than a VM. Recommendation: drop `NET_RAW` and set `no-new-privileges` on the
  build container unless a build need is identified.

### 3.2 QEMU driver (`driver/qemu`)

Two-VM topology; the build VM has one NIC to the relay VM over a Unix socket pair,
IPv6 disabled in guest init, `FORWARD DROP` + `REDIRECT` on the relay VM, the
build VM dropped from the claim port. Cloud images are cached with a recorded
SHA-256 and re-verified on reuse. The relay binary is appended fresh to the
prebuilt initramfs each build. Strongest isolation of the Linux-guest drivers.

- **Finding B applies and is the main concern**: `-netdev user,...,restrict=off`
  lets the relay VM reach the host's networks and IMDS. Set `restrict=on` with an
  explicit upstream, or (preferred) install the `isBlockedIP` filter in `vm` mode.
- Cloud-image `.sha256` is computed from the download itself, not checked against a
  published digest (`verifyImageIntegrity` detects later corruption, not a
  download-time MITM). Low risk over HTTPS, but pinning expected digests for the
  built-in distro URLs would make the base-image trust explicit. Low priority.

### 3.3 VZ driver (`driver/vz`, macOS/arm64)

Best-isolated egress path: the relay is a host process using a gvisor netstack over
a socketpair, and **the SSRF filter is active** (`SSRF: true`, `BlockedSelfIP`
set). The CGo layer explicitly refuses a macOS VM configured with both a socketpair
and NAT ("NAT would bypass network isolation"). The guest cannot add interfaces.

Notes, all Low:
- The netstack runs promiscuous + spoofing (required for transparent proxying);
  correct for a single-tenant socketpair, but it means the SSRF filter is the only
  thing keeping the stack from answering for host-local addresses — so Finding B's
  filter must never be made conditional in host mode.
- `isBlockedIP` does not block CGNAT `100.64.0.0/10` beyond the relay's own IP; not
  reachable in this topology today, worth a line of defense-in-depth.

### 3.4 Hyper-V driver (`driver/hyperv`)

Two VMs on a Private build switch; relay egress via an Internal NAT switch with a
static host-only address; the claim endpoint lives only on the host-only NAT
switch and the build VM is dropped from it. The privileged network-standup helper
is a Windows service behind an ACL'd named pipe (SYSTEM, Administrators, Hyper-V
Administrators) exposing only three typed operations and rejecting version
mismatches. Host-side PowerShell args are single-quote-escaped and the values are
host-controlled (build IDs from `crypto/rand`, internal paths), not build input,
so there is no host-side PowerShell injection from untrusted data.

- **Finding C (Medium)** — `StartRelay` claims with an **empty claim token**
  (`driver/hyperv/relayhost.go` ≈lines 130–131: `ClaimRelay(..., "", ...)`), with
  a comment acknowledging it as deliberate. Container and QEMU both generate a
  per-build `driver.RandomToken()`. The claim endpoint is only on the host-only NAT
  switch, so this is a local-only exposure, but any other process/user on the host
  that can reach `192.168.240.2:8301` during the claim window can claim the warm
  relay first, point `CollectorURL` at its own endpoint, and receive the build's
  I/O while supplying its own MITM CA. Recommendation: generate and pass a claim
  token on Hyper-V as the other drivers do — a one-line consistency fix that closes
  a real (if local) hijack race.
- **Finding B** applies to the NAT path (host-routed IMDS); the same upstream-filter
  fix covers it.

### 3.5 In-VM executor and host-side inspect

- **Finding E (Medium)** — `cmd/warden/inspect_impl.go` `extractPayloads` (≈lines
  693–733) does `os.Readlink` on each `captures/<name>` then reads
  `filepath.Join(capturesDir, target)` with no check that the resolved path stays
  under `payloads/`. Running `warden inspect --extract` against a **downloaded /
  attacker-supplied** output directory therefore follows a capture symlink out of
  the tree and copies host files into the extract dir. `SECURITY.md` lists crafted
  inputs to `warden inspect` as in-scope, so this read-only traversal is in-scope
  and cleanly fixable (require the resolved target to live inside `payloads/`).
  **Correction from the verification pass:** the vector is a **relative** `../`
  symlink target only. An absolute target (`/etc/passwd`) is re-rooted by
  `filepath.Join` under `capturesDir` and does not escape, so the earlier
  "absolute path" phrasing was wrong. The ledger parser itself is bounds-checked;
  no obvious panic or unbounded allocation was found.
- **Finding H (Low)** — macOS `installCA` writes the CA to a predictable
  `/tmp/warden-ca.pem` (`cmd/warden-io/platform_darwin.go` ≈lines 94–95, removed at
  ≈124) and both macOS and Windows `installCA` **silently downgrade exclusive trust
  to append** (logged, not enforced). No bypass results — traffic is still
  topologically forced through the relay MITM — but "exclusive" is a Linux-only
  guarantee today; state that in the bootstrap docs, and replace the fixed `/tmp`
  name with `os.CreateTemp`.
- The executor correctly uses `crypto/rand` for tokens/IDs, streams build output in
  constant memory, and keeps draining the build's stdout when the relay link dies
  so a dead relay never hangs the build.

---

## Recommendations, in priority order

1. **Record DNS in the ledger** (A) and fix the two docs that claim it already is.
   Closes the one unmonitored egress protocol.
2. **Apply the `isBlockedIP` SSRF filter to the upstream transport in every relay
   mode** (B), add the cloud metadata endpoints explicitly, and correct
   `network-isolation.md`. Highest-impact for CodeBuild / GitHub Actions, where the
   host IMDS vends real credentials.
3. **Enforce the egress drop for IPv6 in the container driver** (J) — disable IPv6
   on the holder netns and/or add a mirrored `ip6tables` ruleset, so the invariant
   does not depend on daemon IPv6 config.
4. **Give the Hyper-V driver a per-build claim token** (C), matching container and
   QEMU.
5. **Confine `warden inspect --extract` symlinks to `payloads/`** (E).
6. **Document the integrity scope** (D) and the guest-controlled log/exit (F);
   **sanitize control bytes before teeing the guest console to a TTY** (K).
7. **Harden the build container** (G: drop `NET_RAW`, set `no-new-privileges`) and
   **tidy CA install** (H). Both cheap.
8. Document the Ed25519-over-SHA-512 construction and the role of MD5/SHA-1 (I).
9. Crystallize each confirmed item as an Arm-A regression test per SEC-03.

### Residual risks left deliberately unaddressed
- DoS against the ephemeral, single-tenant relay (cert cache, connection fan-out,
  bandwidth) — out of scope by design and genuinely hard to eliminate for an
  untrusted in-environment workload.
- Container kernel / runtime escape — inherent to the shared-kernel model, which
  the project already rates lower-severity than the VM drivers; the mitigations
  above reduce but cannot eliminate it.
