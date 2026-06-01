# Remote Execution and Measured Hermeticity

## Overview

This document describes a model where BuildWarden serves as a remote execution backend that provides **measured hermeticity** — cryptographic proof of whether an action was hermetic, rather than relying on declarations. Each execution worker is relay-bound and produces a complete ledger. Cached results carry their provenance with them, and parent builds incorporate that provenance directly into their own ledgers.

The system is compatible with existing remote execution clients (Bazel, Buck2, Pants) via the standard Remote Execution API (RE API), while providing guarantees no existing backend offers: per-action provenance, empirical hermeticity measurement, and targeted cache invalidation by environment.

## Motivation

### The hermeticity gap in existing build systems

Bazel and similar build systems declare hermeticity as a convention: actions should not access the network, and the RE API spec is silent on enforcement. In practice:

- Workers may or may not enforce network isolation
- Actions tagged `requires-network` explicitly violate hermeticity
- Toolchain leakage (pre-installed tools not captured in the input root) can silently affect outputs
- Cache poisoning is possible if the action cache is compromised

The result is that "hermetic" is a social contract, not a measured property. A cached result claims to be deterministic from its inputs, but there is no proof.

### What measured hermeticity provides

When every action executes inside a relay-bound sandbox:

- **Hermeticity becomes observable**: an action that makes zero network calls across many executions is empirically hermetic. An action that occasionally fetches from the network is empirically non-hermetic, regardless of its tags.
- **Cache entries carry proof**: each cached result has an associated ledger proving how it was produced — what environment it ran in, what (if any) network inputs it consumed.
- **Environment compromise is recoverable**: when a worker image is found to be compromised, all cache entries produced on that environment can be identified and selectively invalidated.
- **Non-determinism is distinguishable from non-hermeticity**: if two runs with identical inputs and zero network access produce different outputs, that's a non-determinism bug — a different class of problem from "it fetched something different."

## Architecture

### Worker model

Workers are ephemeral relay-bound sandboxes. Each worker:

1. Starts with a fresh relay instance
2. Executes a single action in network isolation (all traffic routes through the relay)
3. Produces a complete ledger for that action
4. Is torn down after execution

New action = new worker + new relay. Workers may be containers, VMs, or cloud instances — the isolation mechanism varies, but the relay-bound guarantee is uniform.

A pool of ready workers serves incoming action requests. The pool is sized and scaled like any remote execution worker fleet. The relay overhead per action is comparable to existing sandbox setup costs (filesystem namespace creation, cgroup allocation).

### Ledger structure

#### Environment as first record

The ledger header contains only structural parameters (signature scheme, hash algorithms, schema list). Environment identity moves from header metadata to a mandatory first record — a signed, content-addressed channel:

```
Record 1: open  (schema=environment-*, payload_size=0)
Record 2: close (schema=environment-*, payload_size=manifest_bytes, hash_block=H(manifest))
           metadata: {reference, digest, os, arch, ...}
```

The payload is verifiable content: an OCI manifest for containers, an image hash for VMs, or other attestation material appropriate to the environment type. Being part of the signature chain, it cannot be modified without invalidating the ledger.

#### Environment schema types

Each environment type has its own schema describing what is being measured:

| Schema | Payload | Identifying metadata |
|--------|---------|---------------------|
| `environment-container` | OCI image manifest bytes | reference, digest, mediaType |
| `environment-vm-image` | Disk image content hash | os, arch, image_path |
| `environment-vm-ipsw` | IPSW restore content hash | os, arch, build_version |
| `environment-filesystem` | Merkle root of measured FS | method (e.g., dm-verity) |
| `environment-inputs` | Hash of traced file tree | method (e.g., syscall-trace), paths |

The last two are future extensions supporting richer measurement — verifying not just the base image but the actual filesystem state or file-level inputs during execution.

#### Multiple environment records

A ledger may contain multiple environment records:

- The **first** environment record is always the relay's own execution environment ("where I am observing").
- **Subsequent** environment records represent environments that produced incorporated cached results ("some of my inputs were produced there").

### Action execution lifecycle

```
1. Client submits action (command, input root, platform properties)
2. Scheduler matches platform to an available worker
3. Worker relay starts, writes environment record as first ledger entry
4. Inputs are staged from content-addressed storage into the sandbox
5. Command executes; relay observes all network traffic
6. Outputs are collected; relay writes artifact records
7. Relay finalizes ledger (all channels closed)
8. Worker is torn down
9. Results (outputs + ledger) stored in cache, keyed by action identity
```

### Cache model

#### Cache key

```
hash(command + sorted_environment + input_root_digest + platform_properties)
```

This is structurally identical to RE API's Action digest — the same inputs produce the same key.

#### Cache value

```
{
  output_root_digest   // content-addressed outputs
  ledger_digest        // content hash of the complete sub-ledger
  environment_digest   // hash block from the environment record
  hermetic             // derived: ledger contains zero network records
}
```

The `hermetic` flag is derived, not declared. It is recomputed from the ledger whenever the cache entry is validated.

#### Cache hit behavior

When a parent build uses a cached action result, the ledger for that action is not referenced — it is **incorporated**. The parent ledger absorbs the novel information:

1. **Environment records** from the sub-ledger that are not already present in the parent (by hash block identity) are added as new environment records in the parent.
2. **Network records** (HTTP channels with novel hash blocks) from non-hermetic cached actions are added to the parent ledger.
3. **Records with hash blocks already present** in the parent ledger are not duplicated.

After incorporation, the parent ledger is self-contained. An auditor reading only the parent ledger knows every unique input (by content) and every environment that contributed to the build. No external references need to be chased.

The sub-ledgers continue to exist in the system for:
- Cache validity checking
- Environment compromise response
- Deep audit (exact execution traces)

But they are an implementation detail of the caching layer, not a structural dependency of any parent ledger.

### Deduplication

Deduplication is by hash block identity. When incorporating cached results:

- If the parent already recorded a network fetch with hash block H (e.g., a particular version of numpy), and the sub-ledger also records an input with hash block H, no new record is added. The input is already accounted for.
- If the sub-ledger records a network fetch with a hash block not present in the parent, that record is added. It represents a novel input the parent hasn't otherwise observed.
- Environment records deduplicate the same way: same environment (same manifest hash) = already recorded.

This ensures ledger size grows with the number of *unique* inputs, not with the number of actions executed.

### Measured hermeticity signal

Over time, the system accumulates empirical data about each action's hermeticity:

| Signal | Meaning |
|--------|---------|
| Action X executed N times, zero network records in all ledgers | High-confidence hermetic; safe to cache aggressively |
| Action Y executed N times, network records on K occasions | Non-hermetic; cache only within same-input runs, flag for user |
| Action Z was previously hermetic, now shows network activity | Alert: something changed (dependency escaped, supply chain drift) |
| Action W is hermetic but produces different outputs from same inputs | Non-determinism bug (timestamps, random seeds, etc.) |

This signal is unavailable in any system that relies on declared hermeticity.

### Environment compromise and targeted invalidation

When a worker environment is discovered to be compromised (e.g., a base image contained a backdoored library):

1. **Identify**: Query all cache entries where `environment_digest` matches the compromised image.
2. **Invalidate**: Remove those entries from the action cache.
3. **Rebuild**: Re-execute only the affected actions on patched worker environments.
4. **Verify**: New sub-ledgers prove execution occurred on clean infrastructure.

No blanket cache flush. No guessing which builds were affected. The environment dimension in the cache key provides surgical precision.

Existing parent ledgers that incorporated results from the compromised environment remain historically accurate — they correctly describe what happened at that time. They are superseded by new builds, not rewritten.

## RE API compatibility

### As a backend

The system exposes a standard RE API gRPC interface:

- **Execute**: accepts an Action digest, dispatches to a relay-bound worker, returns ActionResult
- **ContentAddressableStorage**: stores/retrieves input and output blobs
- **ActionCache**: returns cached results (with ledger-backed provenance)
- **Capabilities**: advertises supported platforms and features

To Bazel, it looks like Buildbarn or any other RE API server. The measured-hermeticity layer is invisible to clients — they get standard semantics with stronger guarantees.

### Handling `requires-network` actions

Actions that declare network requirements can be handled in two ways:

1. **Execute locally within the relay-observed environment** (preferred) — force `requires-network` actions to run inside the parent build's relay boundary via `--modify_execution_info=requires-network=+no-remote`. Network access is available but observed. Provenance is complete.
2. **Execute remotely with full observation** — the relay-bound worker allows network access (it's relay-bound, so all traffic is recorded regardless). The resulting ledger shows the action was non-hermetic. The cache stores it with `hermetic: false`, and consumers are aware.

In both cases, provenance is never lost. The relay's observation is unconditional.

### Client configuration

For Bazel clients building inside a BuildWarden-observed environment and dispatching to this backend:

```
build --remote_executor=grpc://warden-farm:50051
build --sandbox_default_allow_network=false
build --modify_execution_info=requires-network=+no-remote
```

This ensures:
- Hermetic actions dispatch to relay-bound workers (measured, cached, deduplicated)
- Network-requiring actions run locally inside the parent's relay boundary (observed, not cached remotely)
- The analysis/fetch phase runs through the parent relay (full traffic observation)

## Composition with parent builds

A top-level BuildWarden build that uses this remote execution system has a ledger containing:

1. **Its own environment record** — the build environment where Bazel (or equivalent) is running
2. **Network records from the analysis/fetch phase** — repository rule downloads, dependency resolution (all observed through the parent relay)
3. **Incorporated environment records** — one per unique worker environment that produced cached action results
4. **Incorporated network records** — only from non-hermetic actions, only those with novel content hashes

The resulting ledger is a complete provenance document: every input, every environment, every transformation — without redundancy.

## Future extensions

### Filesystem-level measurement

Environment schemas like `environment-inputs` or `environment-filesystem` would record file-level inputs via syscall tracing (ptrace, eBPF, or similar). This extends provenance to local inputs: source files read, tools invoked, shared libraries loaded. Combined with network observation, this produces complete input enumeration for any action — not just "what came from the network" but "what came from anywhere."

### Cross-build deduplication

When multiple independent top-level builds incorporate the same cached action (same action digest, same sub-ledger), they all add the same environment record (by hash block). A fleet-wide view of "which environments contributed to which builds" falls out naturally from ledger analysis.

### Confidence-based caching policies

The measured hermeticity signal enables policies like:
- Cache indefinitely if hermetic across >100 executions
- Cache with short TTL if occasionally non-hermetic
- Never cache if non-deterministic (same inputs, different outputs, no network)
- Alert on hermeticity regression (previously stable action starts making network calls)

## Build system compatibility

The RE API is the standard protocol for remote build execution. Because this system exposes a compliant RE API interface, it is immediately usable by any build system that speaks the protocol — without modification to those build systems. The measured hermeticity layer is invisible to clients; they get standard semantics with stronger guarantees.

### Bazel

Bazel is the most mature RE API client and the most natural first target. Key characteristics:

- **Repository rules fetch during analysis** — `http_archive`, `pip.parse`, `npm_translate_lock`, and similar rules download external dependencies on the client during the analysis phase. When running inside a BuildWarden-observed environment, this entire fetch phase goes through the parent relay with full traffic recording. This is where most non-hermetic network activity lives in a well-structured Bazel build.
- **Actions are designed to be hermetic** — compilation, linking, and code generation actions receive pre-staged inputs and should not access the network. These dispatch to relay-bound workers, producing sub-ledgers that empirically confirm (or refute) that hermeticity claim.
- **`requires-network` is rare and discouraged** — well-structured projects do not use it on compilation actions. Where it appears, it is almost always dependency fetching that should have been moved to repository rules. Forcing these actions local (via `--modify_execution_info=requires-network=+no-remote`) keeps them inside the parent relay's observation boundary.
- **`--sandbox_default_allow_network=false`** ensures the hermeticity contract is enforced locally, complementing the relay-bound enforcement on remote workers.

Bazel users benefit most immediately because the system fills a gap they already feel: declared hermeticity has no proof, and cache poisoning has no detection mechanism. Measured hermeticity provides both.

### Buck2

Meta's Buck2 also uses the RE API protobufs and has been validated against Buildbarn and EngFlow backends. Its architectural differences make it a complementary fit:

- **RE-first design** — Buck2 treats local execution as a special case of remote execution. It pre-computes Merkle tree digests as a natural part of graph evaluation. This means Buck2 already assumes "the execution environment provides isolation" — if that environment is a relay-bound worker, Buck2 requires no conceptual adaptation.
- **No local sandboxing** — Buck2 deliberately provides no local hermeticity enforcement. The assumption is that RE backends handle isolation. Today this is a trust assumption; with relay-bound workers, it becomes a measured guarantee. Buck2 users gain hermeticity proof where they currently have only faith in their RE backend.
- **Static dependency model** — Buck2 has no equivalent to Bazel's repository rules. External dependencies are pre-vendored using offline tools (`reindeer` for Rust, etc.) before the build system runs. This means the parent relay sees less "interesting" fetch traffic — most provenance value comes from the action sub-ledgers rather than a dynamic fetch phase.
- **Deferred materialization** — Buck2 avoids downloading intermediate outputs to the local machine; artifacts stay in CAS until explicitly needed. This is compatible with the incorporation model: the parent ledger absorbs input identities (hash blocks) from sub-ledgers, not the bytes themselves. Intermediate outputs that never transit the parent relay are still provenance-tracked via their worker sub-ledgers.
- **DICE engine** — Buck2's unified computation graph (no phase separation) means action dispatch happens as a natural part of graph evaluation. Actions flow to the RE API endpoint without a distinct "execution phase," making the integration transparent.

Buck2's open-source RE story is less mature than Bazel's today, but architecturally it is the more natural client: it already expects a trusted remote executor, and this system simply makes that trust verifiable.

### Pants, Goma, Reclient, and others

Any build system that speaks RE API v2 can dispatch to this backend without modification. The measured hermeticity layer is protocol-transparent — clients send Execute requests and receive ActionResults. The sub-ledger production, incorporation, and cache management happen entirely server-side.

### What clients get without modification

By pointing any RE API client at this backend, builds immediately gain:

- **Per-action provenance** — cryptographic proof of what environment and inputs produced each cached result
- **Empirical hermeticity measurement** — automatic detection of actions that access the network when they shouldn't
- **Targeted cache invalidation** — surgical response to compromised environments without blanket flushes
- **Non-determinism detection** — distinguishing "fetched something different" from "produced different output from same inputs with no network"
- **Audit trail** — every cached result traceable to its execution environment, inputs, and (absence of) network activity

None of this requires changes to build rules, action definitions, or client configuration beyond pointing `--remote_executor` at the BuildWarden backend.
