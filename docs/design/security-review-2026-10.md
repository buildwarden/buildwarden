# BuildWarden Security Review — October 2026

Scope: a vulnerability and safety review of BuildWarden as a secure build
substrate that runs **customer-defined build scripts** inside an isolated
environment, under something like AWS CodeBuild or GitHub Actions. No server is
exposed to the internet; the attack surface under review is the untrusted build
script and the build environment it controls.

The review covers, in order: (1) the architectural design, (2) the relay
implementation, and (3) each per-driver build environment. Findings are ranked
by relative risk = probability × severity. Theoretical issues that cannot be
truly eliminated (kernel 0-days, DoS against an ephemeral single-tenant relay —
already out of scope per `SECURITY.md`) are noted only where they change how a
real deployment should be configured.

Threat model used throughout is BuildWarden's own (`security-suite.md`, SEC-01):
- **Class 1 — escape / influence beyond the boundary** (host OS, relay keys, the
  network past the relay).
- **Class 2 — measurement evasion** (bytes move in or out without the ledger
  faithfully recording them).

---

## Summary of findings

| # | Finding | Class | Probability | Severity | Risk |
|---|---------|-------|-------------|----------|------|
| A | DNS queries/answers are proxied upstream but **never written to the ledger** | 2 | High | High | **High** |
| B | Relay has **unrestricted egress to the host's own network** (cloud IMDS / RFC1918) in container and QEMU modes | 1 | High on cloud CI | High | **High** |
| C | Hyper-V claims its relay with an **empty claim token** (container/QEMU use a per-build token) | 1 | Low–Med | High | **Medium** |
| D | Ledger **metadata (URL, method, headers) is unsigned**, so the hash↔source binding is not tamper-evident | 2 | Med | Med | **Medium** |
| E | `warden inspect --extract` **follows capture symlinks** out of `payloads/` → arbitrary host-file read on an attacker-supplied output dir | 1 | Low–Med | Med | **Medium** |
| F | Guest-controlled **build-output.log and exit code** via the unauthenticated control plane | 2 | High | Low | **Low–Med** |
| G | Build container runs **root + default caps**, `no-new-privileges`/cap-drop applied only to the holder | 1 | Low | Med | **Low** |
| H | `/tmp/warden-ca.pem` predictable path; exclusive-trust silently downgraded on macOS/Windows | 1/2 | Low | Low | **Low** |
| I | Ed25519-over-SHA512 prehash and MD5/SHA-1 in the hash block — benign, document them | — | — | — | **Note** |

The two High findings both **contradict statements in the shipped docs**
(`network-isolation.md`), so they are also documentation-accuracy bugs.

---

## 1. Architectural design

The core shape is sound and the strongest parts of the design are real
guarantees, not marketing:

- **Push-claim with a host-initiated tunnel** (`relaycfg`): the relay never dials
  the host and the host never listens on the network. The claim listener closes
  after exactly one claim, before the guest boots. This is a genuinely good
  topology and removes a whole class of host-exposure bugs.
- **Deliver-one-binary bootstrap** (`bootstrap.md`): the only thing the host puts
  in the environment is `warden-io`; everything else arrives through the relay
  and is recorded. Isolation precedes execution in every driver.
- **Diskless relay, keys memory-only** (RELAY-03): the Ed25519 ledger key and the
  MITM CA key are generated per relay, never written to disk, and the ledger key
  is zeroed on `Finish()`. Core dumps are disabled at process start.
- **Streaming ledger**: a relay that dies mid-build still leaves a
  truncated-but-verifiable ledger because every record is individually chained.

### Finding A (High) — DNS is not recorded in the ledger

`relay/dns.go` resolves the reserved names to the relay and forwards every other
query upstream (`resolveViaSystem`, then a raw `dns.Client.Exchange`). **No code
path in the DNS handler writes to the ledger.** Compare the HTTP path, which
opens/checkpoints/closes a ledger channel for every request.

Both shipped docs claim the opposite:
- `concepts/overview.md`: "anything else -> upstream, recorded".
- `concepts/network-isolation.md`: "All other queries are forwarded upstream and
  logged in the ledger."

Impact: DNS is a fully unmonitored, bidirectional channel. A build script can
exfiltrate data by encoding it in query labels to an attacker's authoritative
name server, and pull data back in the answer, with **nothing in the ledger** —
not the payload, not even the destination. This is exactly the Class-2
"DNS-tunnel through the relay resolver" cell of SEC-01, and it defeats the
central completeness promise (RELAY-02) for any build that uses DNS as a carrier.
The egress surface is otherwise tightly limited to HTTP/HTTPS/DNS (all other
TCP/UDP is dropped by topology or iptables), which makes the one unrecorded
protocol disproportionately important.

Recommendation: record each forwarded DNS query and its answer as a ledger
channel (question name/type as metadata, answer bytes hashed like an HTTP body),
or, if the ledger should stay HTTP-shaped, add a dedicated DNS record schema.
Until then, correct the two docs — claiming DNS is recorded when it is not is
worse than silence. Then crystallize it as an Arm-A regression test (SEC-03).

### Finding B (High) — relay egress reaches the host's own network / cloud metadata

The SSRF filter (`relay/proxy.go` `isBlockedIP`: loopback, unspecified, private,
link-local, `169.254.169.254`, `100.100.100.200`, self) is only wired in when
`cfg.SSRF` is true, and that is set **only in host mode (VZ)** (`cmd/relay/mode_host.go`).
In `vm` and `container` modes `buildTransport` installs no `DialContext` filter
at all:

- **Container**: the relay container sits on a bridge network with NAT to the
  host; it can reach `169.254.169.254` and any RFC1918 host-side service.
- **QEMU**: the relay VM's upstream is `-netdev user,...,restrict=off` (SLIRP).
  `restrict=off` lets the guest reach the host and everything the host can route,
  including the link-local metadata address.

`network-isolation.md` states metadata access is "Blocked by SSRF filter in host
mode; **unreachable in VM modes**". That is inaccurate for container and QEMU.

Impact on the stated deployment (CodeBuild / GitHub Actions) is the headline
risk: those runners expose an instance metadata service (IMDS) that vends the
runner's IAM / workload credentials. A malicious build script simply asks the
relay to fetch `http://169.254.169.254/...`; the relay proxies it (and faithfully
records it — this is Class 1, not evasion), handing the host's cloud credentials
to the untrusted build. The build can then use or forward them. The ledger
records the theft, but the boundary ("influence beyond the relay", SEC-01
build-env ↔ host OS / ↔ beyond relay) is broken.

Recommendation: apply the `isBlockedIP` filter to the upstream transport in
**all** modes, not just host mode, and treat the filter as the egress policy
rather than a VZ-only backstop. The topology stops the build from bypassing the
relay; it does nothing to stop the relay from being a confused deputy to the
host's own metadata endpoint. Add IMDS (v1 and v2, both IPv4 and the IPv6
`fd00:ec2::254`) and the GCP/Azure metadata hostnames to the blfrom list, and
make reaching them a hard Arm-A test on every driver.

### Design note D (Medium) — provenance is not part of the integrity guarantee

The ledger signs `type ‖ prev_sig ‖ open_sig ‖ payload_size ‖ hash_block`
(`ledger/ledger.go`, `buildRecordSigInput`). The CBOR metadata — which holds the
request **URL, method, and headers** — is deliberately outside the signature so
it can be redacted (`ledger-spec.md`). The consequence is under-stated in the
spec: the binding between a signed content hash and the **source it came from**
is not tamper-evident. A ledger holder can rewrite "this blake2b hash came from
`evil.example`" to "...came from `pypi.org`" without breaking the chain.

For a "completeness of inputs" document this is defensible (the bytes the build
consumed are what is signed), but anyone treating the ledger as a provenance
attestation — "this artifact was built only from these sources" — is relying on
unsigned data. Recommendation: state this limit plainly in `ledger-spec.md`, and
consider a signed-metadata mode (or signing a hash of the metadata) for
deployments that use the ledger as a supply-chain attestation rather than only an
I/O record. This is the SEC-01 "smuggle via unsigned CBOR metadata" cell.

### Note I — signature construction and hash block

- `sign()` computes `ed25519.Sign(key, SHA512(input))`. Ed25519 already hashes
  internally; signing a pre-hash is a non-standard construction (not RFC 8032
  Ed25519ph, which uses a distinct domain-separation context). It is not a
  vulnerability — forgery still requires breaking Ed25519 or SHA-512 — but it
  should be documented in the spec as "Ed25519 over SHA-512(message)" so
  third-party verifiers reproduce it exactly, and so it is not mistaken for
  Ed25519ph.
- The default hash block is `blake2b_256 + sha256 + sha1 + md5`. Integrity rests
  on the full signed block (so the weak members do not weaken tamper-evidence),
  and MD5/SHA-1 are there only for registry cross-referencing. `SECURITY.md`
  already frames them that way; keep that framing visible wherever the hash block
  is described, so MD5/SHA-1 are never read as the security anchor.

---

## 2. Relay implementation

The relay is careful: constant-memory streaming with size caps everywhere
(`maxOutputBytes` 256 MiB, artifact 2–4 GiB, environment 1 MiB), single-writer
ledger via a channel (RELAY-01), auth headers redacted in recorded metadata
(`isAuthHeader`), bodies hashed as the identical stream that is served
(RELAY-02), upstream TLS actually verified (no `InsecureSkipVerify` anywhere in
the tree), and the collector bearer token compared in constant time
(`crypto/subtle`). The claim body is bounded (`io.LimitReader` 16 MiB) and
strict (`DisallowUnknownFields`). Good.

### Finding F (Low–Med) — the guest controls the console log and the exit code

The relay's control plane on `:8300` (`relay/controlplane.go`) is guest-facing
and unauthenticated by design. A build can therefore:
- POST arbitrary bytes to `/v1/output` (bounded, but it pollutes
  `build-output.log`);
- POST `/v1/complete?code=0` (or hit `http://artifacts/exit?code=0`) to report
  success and trigger relay teardown regardless of what the script did.

This is inherent to the model (the build *is* the exit code's author), but it
means `build-output.log` and the reported exit code are attacker-influenced and
must not be used as security signals — only the signed ledger is trustworthy.
Recommendation: document this explicitly (the console log is a convenience
record, not evidence), and have the orchestrator treat an exit arriving before
the guest's own heartbeat/run phase as suspicious rather than authoritative.

### Observations that are correctly handled

- **Re-claim is closed off**: the claim listener is one-shot and closes before
  the guest boots; by the time a build runs, `:8301` is gone. VM init drops the
  build interface from `:8301` even earlier.
- **`safeDialContext` resolves then dials by the resolved IP**, so there is no
  DNS-rebinding TOCTOU window in the SSRF check. (Finding B is that this filter
  is simply not installed in two of three modes, not that the filter is wrong.)
- **Reserved-host handling** (`artifacts`, `cwd`) is served locally and never
  proxied upstream; artifact and context names are validated by shared rules
  (`collector/paths.go`) on both the relay and collector side, and the collector
  confines all disk access to `os.Root` handles so a symlink in the build context
  cannot escape to `~/.ssh` etc.
- **Cert cache growth** and **connection fan-out** are unbounded, i.e. a build can
  make the relay spend memory/CPU. This is DoS against an ephemeral single-tenant
  relay and is explicitly out of scope; the fairness scheduler (`fair.go`) already
  blunts bandwidth starvation. No action beyond keeping it out of scope.

---

## 3. Per-driver build environments

### 3.1 Container driver (`driver/container`)

Isolation model is the Kubernetes-pod pattern and is implemented correctly:
holder owns the netns with `--cap-drop ALL --read-only --security-opt
no-new-privileges`; a `--privileged` sidecar applies the iptables rules and
exits; the build container is **created, not started**, inside the holder's netns
and only then started, so the image's own `ENTRYPOINT` never runs on an open
network. Release builds pin the holder and sidecar images by digest
(`holderImageRef`/`netnsImageRef` via `-ldflags`), and the sidecar base is pinned
by `sha256` in its Containerfile. The DNAT/DROP ruleset forces 53/80/443 to the
relay and drops everything else, and the legacy-iptables Docker DNS rules are
flushed first so DNS cannot bypass the relay.

- **Finding B applies here** (relay egress to host IMDS).
- **Finding G (Low)**: the build container itself runs `--user 0` with Docker's
  **default capability set** (includes `NET_RAW`, `SETUID`, etc.) and neither
  `no-new-privileges` nor a cap-drop. The hardening is all on the holder. Within
  an `OUTPUT -j DROP` netns, `NET_RAW` does not buy an egress bypass (the drop is
  on the build's own namespace), so this is not an isolation break on its own —
  but it widens the container-escape surface that the project already
  acknowledges is weaker than a VM. Recommendation: drop `NET_RAW` and set
  `no-new-privileges` on the build container too, unless a real build need is
  identified; it costs nothing for the common case.

### 3.2 QEMU driver (`driver/qemu`)

Two-VM topology; the build VM has one NIC to the relay VM over a Unix socket
pair, IPv6 disabled in guest init, FORWARD DROP + REDIRECT on the relay VM, and
the build VM dropped from the claim port. Cloud images are cached with a recorded
SHA-256 and re-verified on reuse. The relay binary is appended fresh to the
prebuilt initramfs each build. This is the strongest isolation of the Linux-guest
drivers.

- **Finding B applies here and is the main concern**: `-netdev user,…,restrict=off`
  lets the relay VM reach the host's networks and IMDS. Either set `restrict=on`
  and give the relay an explicit upstream, or (preferred) install the
  `isBlockedIP` filter in `vm` mode as well. `restrict=on` alone would also close
  the gap but is coarser than the IP filter.
- Downloaded cloud images are fetched over HTTPS with `curl -fSL`, but the
  recorded `.sha256` is **computed from the download itself**, not checked against
  a published digest (`verifyImageIntegrity` only detects later corruption, not a
  MITM at download time). Since downloads go through the host's normal TLS, this
  is low risk, but pinning expected digests for the built-in Alpine/Ubuntu/Debian
  URLs would make the base image trust-on-first-use explicit rather than
  implicit. Low priority.

### 3.3 VZ driver (`driver/vz`, macOS/arm64)

This is the best-isolated egress path: the relay is a host process using a gvisor
netstack over a socketpair, and **the SSRF filter is active** (`SSRF: true`,
`BlockedSelfIP` set). The CGo layer explicitly refuses a macOS VM configured with
both a socketpair and NAT ("NAT would bypass network isolation") — a nice
belt-and-suspenders assertion. The guest cannot add interfaces
(Virtualization.framework constraint).

Notes, all Low:
- The netstack runs with promiscuous mode and spoofing enabled (required for
  transparent proxying). That is correct for a single-tenant socketpair, but it
  means the relay stack will answer for any destination IP; the SSRF filter is
  what keeps that from reaching host-local addresses, so Finding B's filter must
  never be made conditional in host mode.
- `isBlockedIP` does not block the CGNAT range `100.64.0.0/10` (the warden base
  net) beyond the relay's own IP. Not reachable in this topology today; worth a
  line of defense-in-depth if host-mode is ever used on a host that routes CGNAT.

### 3.4 Hyper-V driver (`driver/hyperv`)

Two VMs on a Private build switch; relay egress via an Internal NAT switch with a
static host-only address; the claim endpoint lives only on the host-only NAT
switch and the build VM is dropped from it. The privileged network-standup helper
is a Windows service behind an ACL'd named pipe (`pipeSDDL` grants SYSTEM,
Administrators, and the Hyper-V Administrators SID) that exposes only three typed
operations (ping / ensure-network / teardown) and rejects version mismatches.
PowerShell arguments are single-quote-escaped (`psEscapeSingle`), and the values
passed are host-controlled (build IDs from `crypto/rand`, internal paths), not
build-script input, so there is no host-side PowerShell injection from untrusted
data. Good.

- **Finding C (Medium)**: `StartRelay` calls `sess.ClaimRelay(..., "", ...)` — an
  **empty claim token** — and the relay VM boots with `warden.claim_token=` empty.
  Container and QEMU both generate a per-build `driver.RandomToken()` and the docs
  state the token's purpose is to "stop another local process from claiming a warm
  relay first." Hyper-V drops that protection. The claim endpoint is only on the
  host-only NAT switch, so this is a local-only exposure, but any other process or
  user on the host that can reach `192.168.240.2:8301` during the claim window can
  claim the warm relay first, set `CollectorURL` to its own endpoint, and receive
  the build's inputs/outputs while supplying its own MITM CA. Recommendation:
  generate and pass a claim token on Hyper-V exactly as the other drivers do; it
  is a one-line consistency fix and removes a real (if local) hijack race.
- **Finding B** applies to the NAT switch path as well (host-routed IMDS); the
  same upstream-filter fix covers it.

### 3.5 In-VM executor and host-side inspect

- **Finding E (Medium)** — `cmd/warden/inspect_impl.go` `extractPayloads` does
  `os.Readlink` on each `captures/<name>` and reads `filepath.Join(capturesDir,
  target)` with no check that the resolved path stays under `payloads/`. `warden
  inspect --extract` run against a **downloaded / attacker-supplied** output
  directory will therefore follow `captures/x -> ../../../../etc/passwd` and copy
  host files into the extract dir. `SECURITY.md` lists "crafted ledger files …
  arbitrary code execution in `warden inspect`" as in-scope, so this read-only
  traversal is in-scope too and is cleanly fixable (resolve the link and require
  it to live inside `payloads/`, or reject absolute/`..` targets). The ledger
  *parser* itself is bounds-checked and did not show an obvious panic or
  unbounded allocation (lengths are validated before `make`).
- **Finding H (Low)** — macOS/Windows `installCA` write the CA to a predictable
  path (`/tmp/warden-ca.pem` on macOS) and append to shared bundles. The guest is
  single-tenant, so the symlink/TOCTOU risk is low, but a fixed `/tmp` name is
  worth replacing with an `os.CreateTemp` file. Also, **exclusive trust is
  silently downgraded to append on macOS and Windows** (logged, not enforced).
  That does not create a bypass — traffic is still topologically forced through
  the relay MITM — but it means "exclusive" is a Linux-only guarantee today;
  state that in the bootstrap docs so operators do not assume Windows/macOS guests
  get the same trust-store lockdown.
- The executor correctly uses `crypto/rand` for tokens and IDs, streams build
  output in constant memory, and keeps draining the build's stdout even when the
  relay link dies so a dead relay never hangs the build.

---

## Recommendations, in priority order

1. **Record DNS in the ledger** (Finding A) and fix the two docs that claim it
   already is. Highest-value fix: it closes the one unmonitored egress protocol.
2. **Apply the `isBlockedIP` SSRF filter to the upstream transport in every relay
   mode** (Finding B), add the cloud metadata endpoints explicitly, and correct
   `network-isolation.md`. This is the fix that matters most for CodeBuild /
   GitHub Actions, where the host IMDS vends real credentials.
3. **Give the Hyper-V driver a per-build claim token** (Finding C) to match
   container and QEMU.
4. **Confine `warden inspect --extract` symlinks to `payloads/`** (Finding E).
5. **Document the integrity scope** (Finding D): the ledger signs content and
   sizes, not source URLs; the console log and exit code are guest-controlled
   (Finding F). Consider a signed-metadata mode for supply-chain use.
6. **Harden the build container** (Finding G: drop `NET_RAW`, set
   `no-new-privileges`) and **tidy CA install** (Finding H). Both are cheap.
7. Crystallize each confirmed item as an Arm-A regression test per SEC-03, so the
   hole stays shut.

### Residual risks left deliberately unaddressed
- DoS against the ephemeral, single-tenant relay (cert cache, connection
  fan-out, bandwidth) — out of scope by design and genuinely hard to eliminate
  for an untrusted in-environment workload.
- Container kernel / runtime escape — inherent to the shared-kernel model, which
  the project already rates lower-severity than the VM drivers; the mitigations
  above reduce but cannot eliminate it.
