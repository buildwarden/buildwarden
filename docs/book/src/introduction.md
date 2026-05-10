# BuildWarden

BuildWarden produces a cryptographically-signed, tamper-evident ledger of **every input** to a software build. It answers the question: *what exactly went into this binary?*

## The Problem

When you run `docker build`, hundreds of network requests happen silently — base images pulled, packages downloaded, dependencies fetched. If any of those sources are compromised, your build is compromised. Traditional approaches (pinning versions, using lockfiles) help but don't prove what actually happened at build time.

## The Solution

BuildWarden interposes a relay between your build and the network. Every byte that enters (or leaves) the build flows through this relay and is recorded into a cryptographic ledger:

- **Source files** — individually hashed as they enter via the relay
- **Network fetches** — every HTTP/HTTPS request and response recorded
- **Container images** — registry pulls captured at the layer level
- **Build artifacts** — outputs posted back to the ledger for verification

The ledger uses chained Ed25519 signatures — altering, reordering, or removing any entry invalidates all subsequent signatures.

## Who Is This For?

- **Build infrastructure providers** (CI/CD platforms, cloud build services) who want to offer verified builds
- **Security teams** auditing supply chain integrity
- **Open source maintainers** proving their releases match their source
- **Anyone** who wants to answer "what went into this build?" with cryptographic proof
