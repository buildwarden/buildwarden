---
feature: unified-bootstrap
status: draft
round: parallel
drivers: [container]
seams: [executor-initialize, relay-guest-api, collector]
surfaces: []
security_boundary: env-beyond
native_validation:
  - {host: windows-podman, blocking: true, reason: "exec-bit and workdir differences found 2026-10-27"}
  - {host: linux-docker, blocking: true, reason: "default runtime; --network container: rules"}
  - {host: macos-finch, blocking: false, reason: "path sharing and nerdctl cp behaviour"}
tier2_gaps: []
rules: [BOOT-01, BOOT-02, BOOT-03, BOOT-04, BOOT-05, GLOBAL-06, GLOBAL-08, EXEC-04, RELAY-02, ORCH-01]
concepts: [bootstrap, executor-delivery, extensions, network-isolation, container-driver]
related: [hyperv-windows-prepare.md]
---

# Unified bootstrap: bring the container driver into the model

## Problem

The VM drivers bootstrap the build environment the same way: isolate it, deliver
the executor, start it, and let `warden-io initialize` pull everything else from
the relay ([`bootstrap.md`](../development/bootstrap.md)). The container driver
does not (deviation D1). It starts the user's image as `sleep infinity` and then
drives it from the host:

```
  host                                          build container
  ----                                          ---------------
  run <FROM image> sleep infinity  ---------->  running, network open to the relay net
  run --privileged netns sidecar   ---------->  iptables rules applied
  exec: write resolv.conf          ---------->
  cp /.warden (warden-io, ext.d,   ---------->  /.warden/...
      truststore, certhook, ...)
  exec: ln -sf warden-io           ---------->
  exec: sh each ext.d/*.sh         ---------->
  exec: warden-io initialize       ---------->  GET /ca.pem, /build-script, cwd/...
```

That costs us in four ways:

- **Platform bugs.** Every host step is a place where runtimes differ. Validating
  podman on Windows found three in a day (no exec bit after `cp` or `COPY`, podman
  refusing `--workdir` for a missing directory, lost stderr).
- **Extension material bypasses the relay.** The Java truststore, `certhook.sh`,
  `bazel.bazelrc` and the `ext.d` scripts are copied in by `cp`. They are not in
  the relay's record of what the build received, and the VM drivers cannot get
  them at all (D3).
- **Two lifecycles to reason about.** A fix in one bootstrap path does not apply
  to the other, and the container path is the default driver.
- **The relay is not the only setup channel**, which is the property that makes
  the record trustworthy.

## Prior art

- The VM drivers' delivery (cloud-init ISO; the Windows `WARDEN` seed with the
  `warden-run` hook baked at prepare, [`hyperv-windows-prepare.md`](./hyperv-windows-prepare.md)).
- The current netns sidecar ([`network-isolation.md`](../book/src/concepts/network-isolation.md)):
  a privileged one-shot container joins the build container's network namespace,
  applies iptables rules and exits. The build container never has `CAP_NET_ADMIN`.
  This design keeps that and only moves *when* it happens.
- Kubernetes pods: a pause container owns the network namespace and application
  containers join it.

## Decisions

1. **A namespace holder owns the network, and is isolated before the build
   container exists.** warden starts a small holder container (the existing
   `warden-netns` image running `sleep infinity`, unprivileged) on the per-build
   network, applies the iptables rules to it with the privileged sidecar as
   today, and only then creates the build container with `--network
   container:<holder>`. Nothing from the user's image runs before isolation
   (BOOT-01). *Rejected:* starting the build container with an entrypoint and
   racing the sidecar; making `initialize` wait for a signal (the build image's
   own entrypoint and the executor itself would still run with an open network).
2. **The executor goes in before the container starts.** `create` the build
   container with entrypoint `/.warden/warden-io initialize`, copy the executor
   into the created (not running) container, then `start` it. The copy is a tar
   stream warden builds itself, so the file mode is set by warden and not by the
   host filesystem (the Windows exec-bit bug cannot recur). *Rejected:* a bind
   mount (needs the host path visible to the runtime; breaks remote
   `DOCKER_HOST` and depends on per-runtime path sharing); a derived image built
   per base image (an image build and cache per base, and a second image digest
   to explain in the environment record).
3. **The start is the hand-off.** The container's exit is the build's exit; the
   driver observes the relay and the container state, and runs no `exec` (BOOT-03).
   Output reaches the console through the relay as on the VM drivers, and the
   container's own stdout is kept as `build-output.log` for failures before the
   relay is reachable.
4. **Extension material moves to the relay.** Extensions keep running on the host
   (`BeforeBuild`), but their files land in a per-build bootstrap bundle the
   collector serves and the relay passes through as `GET /bootstrap/<name>`, with
   a manifest at `GET /bootstrap/`. `initialize` fetches the manifest after
   installing the CA, places each file and runs each setup script before fetching
   the build script. Bundle bytes are recorded like the build script (RELAY-02).
   *Rejected:* keeping a `cp` of extension files (BOOT-05; and it keeps D3).
5. **`resolv.conf` moves into `initialize`.** The Linux network step points DNS at
   the gateway. This is already what it does on the VM drivers; on a user-defined
   container network the runtime's embedded resolver is otherwise used.
6. **Root by default.** The build container runs as user 0, as the current `exec`
   effectively does on most images; `initialize` needs it to install the CA. A
   `USER` in the Dockerfile still applies to the translated build steps as today.

## Questions

### Blocking

1. **Remote-runtime support for the copy.** Decision 2 relies on `cp` into a
   created container, which docker, podman and nerdctl all support, remotely
   included. Is remote `DOCKER_HOST` a supported configuration we must keep
   working? *Recommended:* yes; it is why Decision 2 rejects the bind mount.

### Assumed

1. **Proceeding with `warden shell` keeping one `exec`**, after `initialize` has
   finished setup and holds instead of running a script (`initialize --hold`).
   BOOT-03 exempts interactive debugging. Confirm.
2. **Proceeding with extension *transport* only.** The VM guests start receiving
   the bundle, but the existing extension scripts are POSIX shell, so Windows
   guests skip `*.sh` entries. Real VM extension support stays tracked under D3.
   Confirm.
3. **Proceeding without fixing D2 (VZ executor baked at prepare)** in this design.
   Confirm.
4. **Proceeding with one holder container per build** rather than reusing a warm
   holder pool. Confirm.

## Design

```
  host                                          build environment
  ----                                          -----------------
  run holder (warden-netns, sleep)  -------->  holder: owns the netns on the build net
  run --privileged sidecar  ---------------->  iptables applied to the holder netns
  create <FROM image>                           build container, not running,
      --network container:<holder>              sharing the isolated netns
      --user 0
      --entrypoint /.warden/warden-io  initialize --gateway=<relay>
  cp (tar stream, mode 0755) warden-io ----->  /.warden/warden-io
  start  ----------------------------------->  initialize
                                                 |-- resolv.conf -> relay
                                                 |-- GET /ca.pem
                                                 |-- GET /bootstrap/  (+ each file, run setup)
                                                 |-- GET /build-script, run it
                                                 |     '-- GET cwd/<path> per COPY
                                                 '-- POST heartbeat, artifacts, exit
  wait for container exit / relay completion
```

This matches the model's diagram step for step, so it is not a model change
(BOOT-07). The bootstrap model gains the `/bootstrap/` endpoint, which
[`bootstrap.md`](../development/bootstrap.md) already shows as pending this design.

Contract changes (GLOBAL-08, EXEC-04):

- Relay guest API: `GET /bootstrap/` (manifest) and `GET /bootstrap/<name>`.
- Collector protocol: `GET /v1/bootstrap/` and `GET /v1/bootstrap/<name>`.
- Executor: `initialize` fetches and applies the bundle; `--hold` for shell mode;
  writes `resolv.conf` on Linux.

## Cross-driver impact

- **container:** the whole bootstrap path above; D1 removed.
- **qemu, hyperv, vz:** no driver code changes. Their `initialize` gains the
  bundle fetch, which is empty unless extensions are configured, and a relay
  without the endpoint (an older relay) is treated as an empty bundle.
- **Shared:** the collector gains the bundle; `driver.Session` serves it.

## Security note

Boundary: build environment to beyond (`env-beyond`), the container driver's
isolation.

- **Posture improves; today's gap is real.** The per-build network is an ordinary
  bridge with outbound routing (`driver.AllocateNetwork` does not create it
  `--internal`), and the build container is started as `run <image> sleep
  infinity`. That does not override an image's `ENTRYPOINT`: `sleep infinity`
  becomes its arguments, so an image with an `ENTRYPOINT` runs it with open egress
  until the sidecar applies the rules. Confirmed 2026-10-27 with podman 5.8 on
  Windows: an image whose `ENTRYPOINT` fetches `https://example.com` succeeds
  when started this way on a fresh bridge network. With the holder, no process from the
  user's image exists before the rules are in place. The build container still
  never holds `CAP_NET_ADMIN`.
- **Unchanged:** the rule set itself, the privileged sidecar's scope (it joins
  the holder, not the build container), and the relay as the sole path.
- **New input path:** extension material now comes over the relay, where it is
  recorded; previously it was unrecorded.
- **TEST-08 should exercise:** an image whose `ENTRYPOINT`/first instruction
  attempts egress (must fail); the build container attempting to change iptables
  (must fail without `NET_ADMIN`); a malicious bootstrap manifest entry name
  (`../`, absolute paths) rejected by `initialize`.

## Validation plan

- Tier 0: unit tests for the bundle manifest, the tar stream's modes, the
  holder/build container argument construction, and `initialize`'s bundle
  handling with a fake relay.
- Tier 1: `test-integration.sh` container cases on Linux docker in CI.
- Tier 2 / native: podman on Windows (blocking; this host), docker on Linux
  (blocking), finch on macOS (tracked). Each runs the COPY case, an extension
  case (truststore), and the egress-at-first-instruction case.

## Implementation plan

1. Executor: `resolv.conf` step and bundle fetch in `initialize`, with an empty
   bundle when the endpoint is missing; `--hold` (EXEC-02, EXEC-04, BOOT-04, BOOT-05).
2. Collector and relay: `/v1/bootstrap/` and `/bootstrap/`, recorded per RELAY-02;
   update the relay and collector contract docs (GLOBAL-08).
3. Extensions write into the bundle instead of `/.warden` (BOOT-05).
4. Container driver: holder plus sidecar, create, tar copy, start, wait; delete
   the `exec` setup path (BOOT-01, BOOT-02, BOOT-03, ORCH-04 for holder teardown).
5. `warden shell` on `--hold`.
6. Docs: `bootstrap.md` (remove D1, update the mapping row),
   `architecture.md` container section, `network-isolation.md`, `extensions.md`.
7. Native validation per the plan above.

## Consolidation (merge round)

## Rollback notes

## Archive summary
