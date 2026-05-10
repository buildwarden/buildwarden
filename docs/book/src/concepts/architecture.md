# Architecture

BuildWarden has three components: the **orchestrator** (runs on your machine), the **relay** (runs in a container), and the **build container** (rootless Docker-in-Docker).

```
┌──────────────────────────────────────────────────────────────┐
│  Host                                                        │
│                                                              │
│  ┌──────────────┐           ┌──────────────────────────┐    │
│  │    Relay     │◄──────────│     Build Container      │    │
│  │              │           │                          │    │
│  │  • DNS       │   only    │  • Rootless DinD         │    │
│  │  • HTTP/S    │◄──conn───►│  • iptables isolated     │    │
│  │  • Ledger    │           │  • Source via relay      │    │
│  │  • Context   │           │                          │    │
│  └──────┬───────┘           └──────────────────────────┘    │
│         │                                                    │
│         ▼ external                                           │
│    ┌──────────┐                                              │
│    │ Internet │                                              │
│    └──────────┘                                              │
└──────────────────────────────────────────────────────────────┘
```

## Orchestrator

The orchestrator (`cmd/warden/`) runs on the host and manages the full lifecycle:

1. Builds or pulls the relay container image
2. Allocates a unique /29 subnet for this build
3. Creates an isolated Docker network
4. Starts the relay and build containers
5. Configures iptables rules for network isolation
6. Rewrites COPY directives for provenance tracking
7. Runs the Docker build
8. Collects output (ledger, logs, Dockerfiles, artifacts)
9. Tears down all containers and networks

## Relay

The relay (`cmd/relay/`, `relay/`) runs inside a container and serves as:

- **MITM HTTPS proxy** — intercepts all TLS connections using an ephemeral CA
- **DNS server** — resolves reserved hostnames (`artifacts`, `cwd`) to itself, forwards everything else upstream
- **Context file server** — serves build context files via `http://cwd/<path>`
- **Artifact store** — accepts POST requests to `http://artifacts/<name>`
- **Ledger writer** — records every request/response with chained Ed25519 signatures

The relay auto-detects its own IP at startup and reads upstream DNS from `/etc/resolv.conf`.

## Build Container

A customized `docker:dind-rootless` image with:

- Pre-configured `daemon.json` pointing Docker's DNS at the relay
- Rootless Docker — build processes cannot modify iptables or escape isolation
- `--network=host` for the inner Docker build — traffic traverses the outer container's iptables rules

## Network Isolation

See [Network Isolation](./network-isolation.md) for the detailed iptables rule set.

## Lifecycle

```
warden build .
  │
  ├─ buildRelayImage() or pullRelayImage()
  ├─ allocateSubnet()
  ├─ createNetwork()
  ├─ startRelayContainer()    ← mounts context read-only
  ├─ editContainerfile()      ← rewrites COPY → curl from relay
  ├─ buildBuildImage()        ← injects daemon.json with relay DNS
  ├─ startBuildContainer()
  ├─ configureBuildContainer() ← iptables, extensions
  ├─ docker build ...          ← the actual build
  ├─ collectOutput()          ← ledger, logs, Dockerfiles, artifacts
  └─ teardown()              ← rm containers, network, images
```
