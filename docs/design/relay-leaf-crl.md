---
feature: relay-leaf-crl
status: archived
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
- RELAY-02: every byte served to the build is recorded. In practice the
  relay's own trust material is the exception: `/ca.pem` is served unrecorded,
  and the CA is written to the output as `ca.cert.pem` instead. The CRL is
  trust material of the same kind and follows `/ca.pem`, not `/bootstrap` and
  `/build-script`, which are build inputs and go through `recordServed`.
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
5. **Served unrecorded, like `/ca.pem` (a1, Jeff, 2026-10-30).** The empty
   CRL is a relay-internal artifact: it lets the guest's TLS stack accept
   certificates it already trusts, and no build output depends on it. A build
   input is something that can change what the build produces. The CRL can't,
   so recording it would add records with no information. Leaving it out also
   keeps ledgers from Windows guests, where Schannel fetches it, from differing
   from other guests' ledgers for the same build. Windows caches a CRL until
   `NextUpdate`, so a build fetches it about once per process that checks
   revocation.
6. **No guest-side workaround.** A `_curlrc` with `ssl-revoke-best-effort` in
   the Windows baseline would fix `curl.exe` only, and would weaken the check
   for the upstream CRL fetches a build makes itself. Fixing the certificates
   fixes every Schannel client and leaves revocation on.

## Questions

### Blocking

None.

### Assumed

- **a1 (confirmed by Jeff, 2026-10-30):** serve the CRL **unrecorded**, like
  `/ca.pem`. Jeff's words: serving the blank revocation list "should be
  considered a relay internal artifact, not something relevant to the actual
  inputs". Its signature bytes differ on every build but carry nothing a build
  could depend on. RELAY-02's wording ("every byte") doesn't yet name this
  exception for relay trust material. Sign-off may want the rule to say so
  (a FLOW-14 rule change, not made here).
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
    /ca.crl -> 200, Content-Type: application/pkix-crl, the DER bytes
               (not recorded, as /ca.pem; Decision 5)
```

Nothing changes for the guest agent, the collector, the claim protocol or the
ledger format, and the ledger has no `/ca.crl` record.

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
  nothing to the collector or the ledger, so a hostile guest gains
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
  so an empty CRL is the truth, not a bypass. It is served unrecorded, like
  `/ca.pem`, because no build output can depend on it (a1). Anyone checking a
  ledger can rebuild the same "nothing revoked" answer from `ca.cert.pem`.
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
  - A test that the fetch adds no ledger record, as `/ca.pem` doesn't
    (Decision 5).
- **Tier 1:** `make lint`.
- **Tier 2, blocking: windows-hyperv.** A Windows guest build whose script runs
  `curl.exe` over HTTPS with **no** `--ssl-no-revoke` succeeds, and
  `inspect --verify-disks` shows all signatures valid and no `/ca.crl`
  record. The step is `m6-validate-windows-hyperv` in the queue.
- No other host blocks: Linux and macOS clients ignore the extension by
  default, and tier 0 covers the endpoint on every platform.

## Implementation plan

1. `generateCA` (`relay/relay.go`) also signs the empty CRL with the CA key
   (`x509.CreateRevocationList`: number 1, `ThisUpdate`/`NextUpdate` = the CA's
   `NotBefore`/`NotAfter`) and keeps its DER in a new `caCRL` field next to
   `caCert`. Nothing is written to disk or to the ledger. RELAY-03, GLOBAL-06.
2. `signHost` (`relay/proxy.go`) sets `CRLDistributionPoints` to
   `http://artifacts/ca.crl` through one shared constant. GLOBAL-06, RELAY-04
   (one code path for every mode).
3. `handleArtifacts` (`relay/relay.go`) serves `/ca.crl` with
   `application/pkix-crl` from a `handleCACRLGet` beside `handleCACertGet`,
   matched before the POST upload fallback and not recorded (Decision 5).
   RELAY-02 (the exception named in Decision 5), GLOBAL-01.
4. Tier-0 tests in `relay/` (TEST-01, TEST-02 tier 0, GLOBAL-04):
   - a leaf from `signHost` names `http://artifacts/ca.crl`;
   - a GET through the relay's HTTP listener returns a CRL that parses, has
     no entries, verifies with `CheckSignatureFrom` against the served CA,
     and has a `NextUpdate` no earlier than the CA's `NotAfter`;
   - two GETs return identical bytes, and the fetch adds no ledger record;
   - a POST to `/ca.crl` gets the CRL and uploads nothing.
5. `docs/book/src/concepts/overview.md`: add `/ca.crl` to the relay's
   `http://artifacts/` list (FLOW-15). No user-facing reference doc changes:
   no flag, config or output changes (UX-01).
6. `make lint` (and `GOOS=linux`/`GOOS=darwin` lint of `relay/`) and
   `make test` (FLOW-17).
7. Native validation on windows-hyperv (`m6-validate-windows-hyperv`, FLOW-09,
   TEST-02 tier 2): refresh the cached relay boot VHDX
   (`tools/relay-vm/hyperv/refresh-cache.sh`), then a Windows guest build runs
   `curl.exe` over HTTPS with no `--ssl-no-revoke` and
   `inspect --verify-disks` passes with no `/ca.crl` record.

No rework of implemented code (FLOW-06): nothing of this feature exists yet.

Merge round (m2): the consolidation pass adds no tasks. Tasks 1-7 are done,
and the merged head re-runs tier 1 and the review (FLOW-03, FLOW-10).

## Consolidation (merge round)

Forked from `main` at `35a9ad1`; `main` merged at `d43e5a0` (m0).

- **Landed since the fork:** only #66, which archives
  `docs/design/workflow-bootstrap.md`. It touches no relay code, no seam this
  design declares and none of its concepts, so there is nothing to reconcile.
- **Terminology:** "relay trust material" (Decision 5, a1) is the name this
  design gives `/ca.pem` and `/ca.crl`. No other design uses a different term
  for them.
- **Abstractions:** `handleCACRLGet` uses the shared `newBytesResponse`.
  `handleCACertGet` still builds its response by hand. Moving it onto the same
  helper would be a cleanup with no behavior change, outside this feature, so
  it isn't done here.
- **FLOW-15 sweep:** `make deadcode` (linux, darwin, windows) reports nothing
  this feature added. `Relay.CAFingerprint` and `Relay.Ledger` were already
  unreachable on `main`. `overview.md` has `/ca.crl` in the relay pseudocode.
- **Rule proposed for sign-off (FLOW-14):** RELAY-02 to name the exception
  for relay trust material that `/ca.pem` and now `/ca.crl` rely on, for
  example: "The relay's own trust material (its CA certificate and CRL) is
  served unrecorded. It is not a build input, and the CA is in the output as
  `ca.cert.pem`." Not made here; `docs/development/` changes only at human
  sign-off.

## Rollback notes

## Archive summary

Merged 2026-10-30 in buildwarden/buildwarden#70 (`98b0ee1`), taken through
every loop phase in one attended session at priority 1.

- **Built.** `generateCA` (`relay/relay.go`) also signs an empty CRL with the
  CA key, valid for the CA's lifetime, and keeps it in memory (`caCRL`).
  `signHost` (`relay/proxy.go`) puts `CRLDistributionPoints:
  ["http://artifacts/ca.crl"]` (`caCRLURL`) on every leaf, and
  `handleArtifacts` serves the DER from `handleCACRLGet` as
  `application/pkix-crl`. Tests are in `relay/crl_test.go`. `overview.md` lists
  the endpoint.
- **Decisions taken:**
  - a CRL, not OCSP;
  - the URL `http://artifacts/ca.crl` on the existing reserved host (a2);
  - only the leaves change, since the CA is a trusted root and already has
    `KeyUsageCRLSign`;
  - `NextUpdate` set to the CA's `NotAfter`;
  - **served unrecorded**, as relay trust material like `/ca.pem` (a1, Jeff:
    a relay-internal artifact that can't affect build outputs).
- **Rejected:**
  - recording each fetch in the ledger;
  - a new reserved hostname;
  - a guest-side `_curlrc` with `ssl-revoke-best-effort` or `ssl-no-revoke`,
    which would fix only `curl.exe` and weaken checks the build makes upstream.
- **Validated.** windows-hyperv (blocking): `curl.exe` with no revocation flag
  fetched two HTTPS hosts, `certutil -urlcache` showed Schannel fetching
  `http://artifacts/ca.crl`, and `inspect --verify-disks` had 15 valid
  signatures and no `/ca.crl` record.
- **For future designs touching `mitm-ca`, `leaf-certificates` or
  `revocation`:**
  - any new leaf template must keep the distribution point, or Windows
    clients break again;
  - the CRL must outlive every leaf, so a longer-lived CA needs a matching
    `NextUpdate` or a reissued CRL;
  - anything else the relay serves as trust material follows Decision 5 (not
    recorded). RELAY-02's text doesn't name that exception yet; the
    Consolidation section has the proposed wording.
