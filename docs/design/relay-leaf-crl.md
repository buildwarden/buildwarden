---
feature: relay-leaf-crl
status: draft
round: parallel
drivers: [container, qemu, vz, hyperv]   # relay/ is shared: every driver's guest gets the new certificates
seams: [relay-guest-api]
surfaces: []
security_boundary: env-relay
alters_relationships: false
native_validation:
  - {host: windows-hyperv, blocking: true, reason: "the failure is Windows Schannel's revocation check; only a Windows guest exercises it"}
tier2_gaps: []
rules: [GLOBAL-06, RELAY-02, RELAY-03, RELAY-04, ORCH-01, FLOW-15]
concepts: [relay, mitm-ca, leaf-certificates, revocation, crl, windows-guest, schannel]
related: [unified-bootstrap.md, hyperv-windows-prepare.md]
---

# Relay leaf certificates name a CRL the relay serves

## Problem

On a Windows guest, `curl.exe` fails every HTTPS request through the relay:

```
curl: (35) schannel: next InitializeSecurityContext failed:
CRYPT_E_NO_REVOCATION_CHECK (0x80092012) - The revocation function was unable
to check revocation for the certificate.
```

Found in the first windows-hyperv native session
(`native/windows-hyperv/2026-10-30/` on the `workflow` branch, finding 2).
Schannel checks revocation for every certificate below a trusted root. The
relay's per-host leaf certificates (`signHost`, `relay/proxy.go`) carry no CRL
distribution point and no OCSP responder, so there is nowhere to check, and
Schannel treats "unknown" as a failure. Anything that uses Schannel with its
default revocation policy fails the same way, for example Git for Windows with
`http.sslBackend=schannel`. PowerShell 5.1's `Invoke-WebRequest` works only
because .NET Framework skips revocation by default. The workaround,
`--ssl-no-revoke`, would have to be added to every build script that runs
`curl.exe`, so builds that work on a normal Windows host fail under warden.

## Prior art

- `unified-bootstrap.md` gives the trust model: the relay generates a per-build
  CA, the guest fetches it from `http://artifacts/ca.pem` and installs it as a
  root. This design keeps that and adds one more relay-generated document next
  to it, on the same reserved host.
- `hyperv-windows-prepare.md` leaves revocation checks a build makes itself
  (CRL/AIA fetches for code-signing certificates) alone. Those go upstream
  through the relay and are recorded. This design doesn't touch them: its URL
  is on `artifacts`, which never goes upstream.
- RELAY-02: every byte served to the build is recorded. `/bootstrap` and
  `/build-script` already go through `recordServed`, and the CRL does too.
- RELAY-03: the CA key never leaves memory. Signing the CRL with it at CA
  creation keeps that true.

## Decisions

1. **A CRL, not OCSP.** The relay signs one empty CRL with its CA key when it
   generates the CA, and serves those same bytes on every fetch. OCSP would
   mean a signed answer per request, a responder certificate or CA-signed
   responses, and nonce handling, all to say "not revoked" about certificates
   nothing can revoke. A CRL is a static document from code the relay already
   has (`x509.CreateRevocationList`), and Schannel, OpenSSL, Java and macOS all
   accept it.
2. **The URL is `http://artifacts/ca.crl`.** Revocation data is fetched over
   plain HTTP by convention, because an HTTPS fetch would need its own
   revocation check. `artifacts` already resolves to the relay in every
   driver's guest (container, qemu, vz, hyperv), and `handleArtifacts` already
   serves the relay's guest API there, so the URL works everywhere with no DNS
   or routing change. The alternative, a new reserved hostname, adds a name
   for no gain.
3. **Only the leaf certificates change.** The CA is a self-signed root that the
   guest installs into its root store, and no verifier checks a trusted root's
   revocation. The CA already has `KeyUsageCRLSign`, so its certificate and
   `ca.pem` are unchanged.
4. **The CRL is valid for the CA's lifetime.** `ThisUpdate` matches the CA's
   `NotBefore`, `NextUpdate` matches its `NotAfter` (24 h), and the CRL number
   is 1. A `NextUpdate` in the past would make Schannel report
   `CRYPT_E_REVOCATION_OFFLINE`, so the CRL must not expire before the
   certificates it covers. Leaves already expire within the CA's 24 h.
5. **Recorded like `/bootstrap`.** Each fetch goes through `recordServed`
   (RELAY-02). The bytes are fixed per build, so every record of it has the
   same hash. Windows caches a CRL until `NextUpdate`, so a build fetches it
   about once per process that does its own revocation checks.
6. **No guest-side workaround.** A `_curlrc` with `ssl-revoke-best-effort` in
   the Windows baseline would fix `curl.exe` only, and would weaken the check
   for the upstream CRL fetches a build makes itself. Fixing the certificates
   fixes every Schannel client and leaves revocation on.

## Questions

### Blocking

None.

### Assumed

- **a1:** Record each CRL fetch in the ledger (RELAY-02), as `/bootstrap` is,
  rather than serving it unrecorded as `/ca.pem` is?
  Options: record it; serve it unrecorded like `/ca.pem`.
  Recommended: **record it**. `/ca.pem`'s content is already in the output
  (`ca.cert.pem`), but the CRL's signature bytes are new on every build.
  Recording costs one small record per fetch.
- **a2:** `http://artifacts/ca.crl` as the distribution point, rather than a
  new reserved hostname or a path under `cwd`?
  Recommended: **`http://artifacts/ca.crl`**.
- **a3:** Leave the existing `--ssl-no-revoke` in the native diagnostic scripts
  on the `workflow` branch, and validate this feature with a build script that
  uses no revocation flag?
  Recommended: **yes**. Those logs record what was run then.

## Design

Relay (`relay/relay.go`, `relay/proxy.go`):

```
at CA generation (the claim):
    ca  := self-signed, IsCA, KeyUsageCertSign|CRLSign, 24 h   # unchanged
    crl := x509.CreateRevocationList(rand, {
               Number: 1, ThisUpdate: ca.NotBefore, NextUpdate: ca.NotAfter,
               RevokedCertificateEntries: none,
           }, ca, caKey)
    keep crl (DER) in memory with the CA

signHost(hostname):                                   # per-host leaf
    template.CRLDistributionPoints = ["http://artifacts/ca.crl"]
    ... as today

handleArtifacts:
    /ca.crl -> recordServed(req, crl, "ca.crl");
               200, Content-Type: application/pkix-crl, the DER bytes
```

Nothing changes for the guest agent, the collector, the claim protocol or the
ledger format. `inspect` lists the record like `/bootstrap`
(`GET /ca.crl (n bytes)`).

`docs/book/src/concepts/overview.md` adds `/ca.crl` to the relay's
`http://artifacts/` list (FLOW-15).

## Cross-driver impact

The relay is the same binary in all three modes (RELAY-04), so every driver's
guests get leaf certificates with the distribution point:

- **hyperv (Windows guests):** the fix. Schannel fetches
  `http://artifacts/ca.crl`, finds the leaf unrevoked, and the handshake
  succeeds with no flags.
- **qemu (Linux and Windows guests):** Windows guests get the same fix. Linux
  clients (OpenSSL, curl, Go, Python) don't check revocation by default and
  ignore the extension.
- **container (Linux):** the same as Linux on qemu. A client that turns CRL
  checks on (for example Java with `com.sun.security.enableCRLDP=true`) now
  gets a valid answer where before it had nowhere to look.
- **vz (macOS guests):** Security.framework's TLS revocation checking is
  best-effort by default. A fetch, if made, succeeds.

## Security note

Boundary: the relay (GLOBAL-06 covers every relay change) and the guest-relay
API on `artifacts`.

- **New guest-facing endpoint:** `GET http://artifacts/ca.crl` returns fixed,
  relay-generated bytes. It takes no input, reaches no upstream and writes
  nothing to the collector except the ledger record, so a hostile guest gains
  nothing it can't already do with `GET /ca.pem`.
- **Key handling (RELAY-03):** the CA key signs one CRL in memory, at the same
  moment it signs the CA. Nothing is persisted, and the key never leaves the
  relay.
- **Trust posture:** the guest's trust doesn't widen. The guest already trusts
  the CA, and the CRL is signed by that CA, so it can only say "nothing
  revoked" about certificates the guest already accepts. Revocation checking
  stays on in the guest, where the alternative (`ssl-no-revoke`) would turn it
  off.
- **Honesty of the record:** the leaves are ephemeral and nothing revokes them,
  so an empty CRL is the truth, not a bypass. Every fetch is in the ledger
  (a1).
- **What TEST-08 should exercise:** the response doesn't depend on the query
  string, headers or method; a POST to `/ca.crl` can't upload an artifact
  under that name, since the path is matched before the upload fallback, as
  `/ca.pem` is; the CRL verifies against the served CA and no other key; and
  no request makes the relay sign anything else with the CA key.

## Validation plan

- **Tier 0 (`relay/`):**
  - A test that a leaf from `signHost` names `http://artifacts/ca.crl`.
  - A test that a GET of that URL through the relay returns a CRL that
    `CheckSignatureFrom` the served CA accepts, with `NextUpdate` no earlier
    than the CA's `NotAfter` and no entries.
  - A test that the fetch is recorded, with bytes equal to the served bytes
    (RELAY-02's round-trip).
- **Tier 1:** `make lint`.
- **Tier 2, blocking: windows-hyperv.** A Windows guest build whose script runs
  `curl.exe` over HTTPS with **no** `--ssl-no-revoke` succeeds, and
  `inspect --verify-disks` shows the `/ca.crl` record with all signatures
  valid. The step is `m6-validate-windows-hyperv` in the queue.
- No other host blocks: Linux and macOS clients ignore the extension by
  default, and tier 0 covers the endpoint on every platform.

## Implementation plan

1. Generate and keep the CRL with the CA (`relay/relay.go`); RELAY-03.
2. Add the distribution point to `signHost` (`relay/proxy.go`).
3. Serve and record `/ca.crl` in `handleArtifacts`; RELAY-02.
4. Tier-0 tests from the validation plan.
5. `overview.md`: add `/ca.crl` to the relay pseudocode (FLOW-15).
6. Native validation on windows-hyperv, with the cached relay boot VHDX
   refreshed by `tools/relay-vm/hyperv/refresh-cache.sh`.

## Consolidation (merge round)

## Rollback notes

## Archive summary
