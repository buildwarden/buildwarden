# Security Policy

## Reporting a Vulnerability

If you discover a security vulnerability in BuildWarden, please report it responsibly.

**Do not open a public GitHub issue for security vulnerabilities.**

Instead, please email: **security@buildwarden.dev**

Include:
- Description of the vulnerability
- Steps to reproduce
- Impact assessment (what an attacker could achieve)
- Any suggested fix (optional)

## Response Timeline

- **Acknowledgment**: within 48 hours
- **Initial assessment**: within 7 days
- **Fix or mitigation**: targeting 30 days for critical issues

## Scope

The following are in scope:

- Signature forgery or chain bypass in the ledger format
- Escaping network isolation (traffic that bypasses the relay)
- MITM CA private key leakage outside the relay container
- Crafted ledger files that cause crashes or arbitrary code execution in `warden inspect`
- Privilege escalation from the build container to the host

The following are out of scope:

- Denial of service against the relay (it's ephemeral and single-tenant)
- Issues requiring physical access to the build host
- Vulnerabilities in upstream dependencies (report those upstream)

## Cryptographic Design

BuildWarden uses:
- **Ed25519-SHA512** for ledger signature chains
- **RSA-2048** for ephemeral MITM CA certificates (24-hour lifetime, never persisted)
- **BLAKE2b-256 + SHA-256 + SHA-1 + MD5** as the default hash block (multiple algorithms for cross-referencing against package registries)

The Ed25519 private key exists only in relay container memory and is never written to disk. The MITM CA private key is similarly memory-only within the relay container.
