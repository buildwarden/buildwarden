---
feature: unified-bootstrap
status: active
round: parallel
drivers: [container, qemu, hyperv, vz]
seams: [executor-initialize, relay-guest-api, collector, bootstrap-model]
surfaces: []
security_boundary: env-beyond
native_validation:
  - {host: windows-podman, blocking: true, reason: "exec-bit and workdir differences found 2026-10-27"}
  - {host: linux-docker, blocking: true, reason: "default runtime; --network container: rules"}
  - {host: macos-finch, blocking: false, reason: "path sharing and nerdctl cp behaviour"}
tier2_gaps: []
rules: [BOOT-01, BOOT-02, BOOT-03, BOOT-04, BOOT-05, BOOT-07, GLOBAL-06, GLOBAL-08, EXEC-04, RELAY-02, ORCH-01]
concepts: [bootstrap, executor-delivery, extensions, network-isolation, container-driver, java-truststore]
related: [hyperv-windows-prepare.md]
---

# Unified bootstrap: one start, and `initialize` sets itself up

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
      ca.crt, certhook.sh, certs.jks,
      bazel.bazelrc)
  exec: ln -sf warden-io           ---------->
  exec: sh each ext.d/*.sh         ---------->
  exec: warden-io initialize       ---------->  GET /ca.pem, /build-script, cwd/...
```

That costs us in five ways:

- **An isolation gap.** The per-build network is an ordinary routable bridge and
  `run <image> sleep infinity` does not override an image's `ENTRYPOINT`, so that
  entrypoint runs with open egress before the sidecar applies the rules
  (confirmed; see the security note).
- **Platform bugs.** Every host step is a place where runtimes differ. Validating
  podman on Windows found three in a day (no exec bit after `cp` or `COPY`, podman
  refusing `--workdir` for a missing directory, lost stderr).
- **Extensions are host-generated, unrecorded, and container-only.** The
  extensions write files and POSIX shell scripts on the host; `cp` carries them
  in, so they are not in the relay's record, and the VM drivers get none of them
  (D3).
- **Extensions duplicate `initialize`.** `initialize` already writes
  `resolv.conf`, installs the CA (`installCA`) and sets the CA env vars
  (`setCAEnvironment`). The truststore and cacerts extensions do the same jobs a
  second way, with different results (the extension replaces the system trust,
  `initialize` appends to it).
- **The sidecar image is trusted by tag.** `warden-netns:latest` is built on
  first use from `alpine:latest` plus `apk add`, and afterwards any image with
  that tag on the runtime is run `--privileged`.

## Prior art

- The VM drivers' delivery (cloud-init ISO; the Windows `WARDEN` seed with the
  `warden-run` hook baked at prepare, [`hyperv-windows-prepare.md`](./hyperv-windows-prepare.md)).
- The current netns sidecar ([`network-isolation.md`](../book/src/concepts/network-isolation.md)):
  a privileged one-shot container joins the build container's network namespace,
  applies iptables rules and exits. The build container never has `CAP_NET_ADMIN`.
  This design keeps that and only moves *when* it happens.
- Kubernetes pods: a pause container owns the network namespace and application
  containers join it.
- The relay image, already published to `ghcr.io/buildwarden/relay` by
  `.github/workflows/relay-image.yml`.
- JKS is already written in pure Go on the host (`driver/ext_bazel.go`, through
  `github.com/pavlo-v-chernykh/keystore-go`); no `keytool` is involved today.

## Decisions

1. **A namespace holder owns the network, and is isolated before the build
   container exists.** warden starts a holder container on the per-build network,
   applies the iptables rules to its namespace with the privileged sidecar, and
   only then creates the build container with `--network container:<holder>`.
   Nothing from the user's image runs before isolation (BOOT-01). *Rejected:*
   starting the build container with an entrypoint and racing the sidecar; making
   `initialize` wait for a signal (the image's own entrypoint and the executor
   would still run with an open network).
2. **The holder and the sidecar are published release images, run by digest.**
   The holder runs *before* the rules exist, so whatever it runs has open egress;
   it must be trusted code, not merely harmless code.
   - **Holder:** `FROM scratch`, one static `pause` binary that blocks until
     signalled. No shell, no network code, nothing listening (the build container
     shares its loopback). Run `--cap-drop ALL --read-only --user 65534`. It is
     deliberately *not* `warden-io` or the relay: no networked code before
     isolation.
   - **Sidecar:** the iptables image, its base pinned by digest.
   - Both are built and pushed by CI alongside the relay image and each release
     records their digests in the `warden` binary (ldflags). warden runs
     `image@sha256:...`, never a tag. A `dev` build builds them from the embedded
     Containerfiles (base pinned by digest) and tags them with the content hash.
   - *Rejected:* keeping the local `alpine:latest` build (unpinned, network at
     first use, tag-trusted); reusing a warm holder across builds (one per build;
     see assumed question 4).
3. **The executor goes in before the container starts.** `create` the build
   container from the user's FROM image unchanged, with entrypoint
   `/.warden/warden-io initialize --gateway=<relay>`, `--user 0` and `--network
   container:<holder>`; copy the executor in as a tar stream warden builds itself
   (mode 0755 set in the header, so no `chmod` step and the Windows exec-bit bug
   cannot recur); then `start`. No image is built. *Rejected:* a bind mount
   (needs the host path visible to the runtime; breaks remote `DOCKER_HOST` and
   depends on per-runtime path sharing); a derived image per base image (an image
   build and cache per base, and a second image digest to explain in the record).
4. **The start is the hand-off.** The container's exit is the build's exit; the
   driver observes the relay and the container state, and runs no `exec` (BOOT-03).
   Output reaches the console through the relay as on the VM drivers, and the
   container's own stdout is kept as `build-output.log` for failures before the
   relay is reachable.
5. **Extensions become `initialize`'s own baseline; the bootstrap config carries
   only trust policy.** This is the model change; see the Bootstrap comparison.
   - Trust-store and build-system integrations, and other best-practice defaults
     for every build, are `warden-io`'s domain. `initialize` always applies them,
     per platform, from the CA alone: they are not configurable and not
     conditional. Each is a few small file writes, so there is no cost worth
     avoiding when a build does not use the tool. Each is logged as a step, so it
     shows in the build output.
   - Build-specific environment variables are not part of bootstrap at all: they
     come from the build's own `ENV` lines, in the translated build script.
   - The one orchestrator decision left is trust policy. The collector serves it
     as a small JSON document at `GET /v1/bootstrap`, passed through by the relay
     as `GET /bootstrap` and recorded like the build script (RELAY-02):

     ```json
     { "version": 1, "trust": { "mode": "exclusive" } }
     ```

   - Today's extensions map onto it as follows:

     | Extension today | `initialize` |
     |---|---|
     | truststore: `ca.crt`, `certhook.sh`, `truststore.sh` | `trust.mode` from the config: `exclusive` (the warden CA is the only system root, re-applied by a `ca-certificates` update hook, as the extension does today) or `append` (what `installCA` does today) |
     | bazel: `certs.jks`, `bazel.bazelrc`, `bazel.sh` | always: the JKS (Decision 6), `MAVEN_OPTS`/`GRADLE_OPTS` pointing at it, and `/etc/bazel.bazelrc` |
     | cacerts: env vars | always: merged into `setCAEnvironment` with the existing CA env vars |
     | epoch: `SOURCE_DATE_EPOCH=0` | always; a build's own `ENV` overrides it |

   - The baseline env is set on the build script's process, so the script's own
     `ENV` lines win. A build that sets `MAVEN_OPTS` itself replaces the
     truststore flags; that is the build's choice, and the same as today.
   - A relay without the endpoint (an older relay) gives an empty config, which
     means `append` trust; the baseline still applies.
   - *Rejected:* serving the generated files and scripts through the relay (the
     previous draft of this design). It records them, but keeps host-generated
     root scripts and a binary keystore as inputs, keeps them POSIX-only, and
     keeps two implementations of CA installation.
6. **`initialize` writes the Java truststore itself, with a minimal JKS writer.**
   The JVM tools need a keystore file; `keytool` is not in most images, and the
   current host-side library would otherwise move into the guest binary.
   - The writer handles one case: a keystore of trusted-certificate entries,
     written once. That is the stable part of the format: magic `0xFEEDFEED`,
     version 2, the entry count, then per entry the tag 2, the alias, a
     timestamp, the type `X.509`, the DER length and bytes, and a trailing SHA-1
     over the password (UTF-16BE), the string `Mighty Aphrodite` and everything
     before it. It is about 60 lines and has no reader in production code.
   - Output is deterministic: the entry timestamp is the CA's `NotBefore`, so the
     same CA always gives the same bytes and the file can be checked against the
     record.
   - The current code writes the type as `X509` where `keytool` writes `X.509`.
     Java accepts both, and a byte comparison against `keytool` is exactly the
     test that surfaces differences like this one.
   - Validation: a test that runs wherever a JDK is present (always in CI) builds
     the same keystore with `keytool -importcert -storetype JKS` and compares the
     bytes with the timestamp field normalized; checks that `keytool -list
     -storetype JKS -storepass changeit` accepts ours (which verifies the
     integrity digest); and loads it through a `TrustManagerFactory` in a one-line
     Java program to prove a JVM trusts the CA through it.
   - *Rejected:* PKCS#12 (Java 9+ default, but much more format to write, and
     Java 8 still reads JKS by default); running `keytool` when present (most
     images lack it, and the result would depend on the image's JDK); adding to
     the JDK's own `cacerts` (needs finding every JDK in the image).
7. **`resolv.conf` stays in `initialize`, and the driver's rewrite goes.**
   `configureNetwork` already writes it on Linux; the container driver's `exec`
   rewrite and its iptables `DOCKER_OUTPUT` flush stay as they are only until the
   holder applies them.

## Questions

### Blocking

1. **Remote-runtime support for the copy.** Decision 3 relies on `cp` into a
   created container, which docker, podman and nerdctl all support, remotely
   included. Is remote `DOCKER_HOST` a supported configuration we must keep
   working? *Resolved 2026-10-28:* yes; it is why Decision 3 rejects the bind
   mount.
2. **Sign-off on the Bootstrap comparison** below (BOOT-07), which rewrites
   BOOT-05. *Accepted 2026-10-28,* with two refinements now in the design: the
   integrations are an always-on `initialize` baseline and the config carries
   only trust policy (Decision 5); and the BOOT-05 text states that the
   orchestrator still generates the build script, which is translation of the
   user's build definition, not setup material.

### Assumed

1. **Proceeding with `warden shell` keeping one `exec`**, after `initialize` has
   finished setup and holds instead of running a script (`initialize --hold`).
   BOOT-03 exempts interactive debugging. Confirm.
2. **Proceeding with `trust.mode: exclusive` as the default on Linux guests of
   every driver**, as the container driver does today. Every TLS connection goes
   through the relay's MITM, so no other root can validate a real connection.
   Windows and macOS guests keep `append` until their trust stores get the same
   treatment. Confirm.
3. **Proceeding without fixing D2 (VZ executor baked at prepare)** in this design.
   It now matters more: a stale baked `warden-io` lacks features the config asks
   for: it would silently skip the baseline it does not know, and it reports
   unknown config keys as a step failure rather than ignoring them. Confirm, or pull D2 in.
4. **Proceeding with one holder container per build** rather than reusing a warm
   holder pool. Confirm.
5. **Proceeding with the relay image's `:latest` pull left as it is here.** It has
   the same tag-trust and version-skew problem as the sidecar, but is not part of
   the bootstrap path. Tracked as a follow-up. Confirm. *Confirmed 2026-10-28:*
   deferred to a later feature on component image publishing, which may come
   with splitting the components into their own repositories.

## Design

```
  host (orchestrator)                           build environment
  -------------------                           -----------------
  run holder@sha256 (pause)  --------------->  holder: owns the netns on the build net
  run --privileged sidecar@sha256  --------->  iptables applied to the holder netns
  create <FROM image>                           build container, not running,
      --network container:<holder>              sharing the isolated netns
      --user 0
      --entrypoint /.warden/warden-io  initialize --gateway=<relay>
  cp (tar stream, mode 0755) warden-io ----->  /.warden/warden-io
  start  ----------------------------------->  initialize
                                                 |-- resolv.conf -> relay
                                                 |-- GET /ca.pem      system trust
                                                 |-- GET /bootstrap   trust mode
                                                 |-- baseline: JKS, bazelrc,
                                                 |     CA env, SOURCE_DATE_EPOCH
                                                 |-- GET /build-script, run it
                                                 |     '-- GET cwd/<path> per COPY
                                                 '-- POST heartbeat, artifacts, exit
  wait for container exit / relay completion
```

Scope after this design:

| Party | Owns |
|---|---|
| Orchestrator | Allocating the network; isolation (holder, sidecar, or VM topology); claiming the relay and pointing it at the collector; delivering the executor; one start; observing. It translates the user's build definition into the build script (host-side, because that needs the context: glob and directory expansion, explicit modes where the host filesystem has none, `ARG`/`ENV` expansion, rejecting unsupported directives before anything boots) and sets the trust policy. It generates no setup material. |
| Relay | The only path in or out: serves `/ca.pem`, `/bootstrap`, `/build-script`, `cwd/`, and records all of it. |
| `initialize` | Everything inside the environment: network and DNS, system trust, the always-on baseline (Java truststore, tool config, default env), the workdir, running the script, reporting. Its only start-up input is `--gateway` (and a static address on VMs). |

Contract changes (GLOBAL-08, EXEC-04):

- Relay guest API: `GET /bootstrap` (the config, `{"version":1,...}`); an absent
  endpoint means an empty config.
- Collector protocol: `GET /v1/bootstrap`.
- Executor: `initialize` fetches and applies the trust policy, applies the
  baseline, creates the workdir; `--hold` for shell mode; unknown config keys fail the step.
- Extension interface: removed. `driver.Session` builds the trust policy; the
  Containerfile rewrite no longer injects extension `ENV` lines. `ExtensionContext`, `caCertPEM` and the
  `keystore-go` dependency go (it stays only if the test oracle needs it).

## Cross-driver impact

- **container:** the whole bootstrap path above; D1 removed.
- **qemu / Linux:** no driver code changes. Their `initialize` now gets
  exclusive trust and the baseline (JKS, bazelrc, default env); before, they got
  none. This
  is a behavior change for those builds, intended.
- **qemu, hyperv / Windows, vz:** the baseline applies (JKS at a platform path); system
  trust stays `append` (assumed question 2). VZ only once its baked `warden-io`
  is re-prepared (D2).
- **Shared:** the collector serves the config; `driver.Session` builds it.
- D3 is removed: the transport and the features both reach every driver.

## Bootstrap comparison

### Current model

```
  (1) isolate --> (2) deliver executor --> (3) start
                                              initialize
                                                |-- GET /ca.pem
                                                |-- GET /bootstrap/...  extension
                                                |     material: host-generated
                                                |     files and setup scripts,
                                                |     installed and run as given
                                                |-- GET /build-script, run it
                                                '-- POST heartbeat, artifacts, exit
```

### Proposed model

```
  (1) isolate --> (2) deliver executor --> (3) start
                                              initialize
                                                |-- GET /ca.pem
                                                |-- GET /bootstrap      trust policy
                                                |     only (declarative)
                                                |-- baseline, always: every setup
                                                |     file derived from the CA by
                                                |     initialize itself
                                                |-- GET /build-script, run it
                                                '-- POST heartbeat, artifacts, exit
```

### Per invariant

| Invariant | Current | Proposed | Effect per driver |
|---|---|---|---|
| BOOT-01 Isolation first | Unchanged | Unchanged | container: now actually met (holder) |
| BOOT-02 Executor only out-of-band | Unchanged | Unchanged | container: `cp` carries only `warden-io` |
| BOOT-03 One start | Unchanged | Unchanged | container: `exec` steps removed |
| BOOT-04 Build inputs from relay | Unchanged | Unchanged | none |
| BOOT-05 Setup material | "Anything the orchestrator generates to set up the environment is served by the relay and installed by `initialize`." | "Environment setup is the executor's domain. `initialize` applies a fixed baseline (trust-store and build-system integrations, best-practice defaults) derived from the CA, plus the trust policy the orchestrator sends as a declarative config over the relay. The orchestrator generates no setup material; the build script it translates from the user's build definition is a build input (BOOT-04), not setup. The guest runs no setup code it did not ship with." | all: features reach every driver; no host-written scripts run as root in any guest |
| BOOT-06 Hooks, not content | Unchanged | Unchanged | none; but a stale baked executor (D2) now also lacks features |

### Gains

- **Smaller, readable record.** What set up the environment is one JSON document
  plus the CA, not a set of files and scripts; an auditor reads intent, and every
  derived file (the JKS included) is reproducible from those two inputs.
- **No setup code from the host runs in the guest.** The only code that runs
  before the build script is the version-matched executor.
- **One implementation per job.** CA installation, CA env vars and DNS each have
  one implementation in `initialize`, written per platform in Go, so features
  work on Windows and macOS guests instead of being POSIX shell.
- **The orchestrator's job is placement and security**, and the guest side
  bootstraps itself from the relay, which is the scoping that makes a third-party
  orchestrator (`cmd/collector`) able to drive a guest with no extension code.

### Costs

- **A new setup capability needs an executor change**, not just an orchestrator
  change. Since the executor ships with the orchestrator (BOOT-02), that is the
  same release, except under D2.
- **No arbitrary setup scripts.** There is no user extension mechanism today
  (`DefaultExtensions` is fixed), so nothing is lost now; adding one later would
  be its own model change.
- **The executor grows** by the feature code and the JKS writer (small; it
  already carries an HTTP client and CA installation).
- **Security posture:** improves (no host-written root scripts; one CA path).
  **Record:** the config replaces the generated files; the files are reproducible
  from it, so nothing becomes unrecorded.

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
  when started this way on a fresh bridge network. With the holder, no process
  from the user's image exists before the rules are in place. The build container
  still never holds `CAP_NET_ADMIN`.
- **The holder is pre-isolation code.** It runs before the rules, so it is a
  digest-pinned, shell-less, capability-free `pause` (Decision 2). The build
  container cannot signal it (separate PID namespace), and if it dies the
  namespace and its rules persist while the build container holds them (fails
  closed).
- **The privileged sidecar is no longer trusted by tag.** Today any image tagged
  `warden-netns:latest` would run privileged; after this, only the release digest.
- **New input path:** the bootstrap config comes over the relay, where it is
  recorded. `initialize` validates it strictly (known keys, typed values; no
  paths or commands are taken from it).
- **TEST-08 should exercise:** an image whose `ENTRYPOINT` attempts egress (must
  fail); the build container attempting to change iptables (must fail without
  `NET_ADMIN`); a substituted image under the old sidecar tag (must not be run);
  a bootstrap config with unknown keys or path-like values (rejected).

## Validation plan

- Tier 0: unit tests for the bootstrap config (schema, unknown keys, empty when
  absent), the tar stream's modes, the holder/build container argument
  construction, each `initialize` feature against a temp root, and the JKS writer
  (golden bytes; determinism).
- Tier 1: `test-integration.sh` container cases on Linux docker in CI; the JKS
  `keytool` comparison and JVM trust check in CI with a JDK.
- Tier 2 / native: podman on Windows (blocking; this host), docker on Linux
  (blocking), finch on macOS (tracked). Each runs the COPY case, a Java/Maven
  fetch through the truststore, and the egress-at-first-instruction case. Hyper-V
  and qemu Linux guests run the Java case once, as the first builds there to get
  features.

## Implementation plan

1. Holder and sidecar images: Containerfiles, a static `pause`, CI publishing next
   to the relay image, digests recorded in the binary, content-hash tags for
   `dev` (GLOBAL-06, ORCH-01).
2. Executor: the bootstrap config fetch and validation; `trust.mode`; the
   baseline (JKS writer, bazelrc, `MAVEN_OPTS`/`GRADLE_OPTS`, CA env,
   `SOURCE_DATE_EPOCH`); workdir creation; `--hold` (EXEC-02, EXEC-04,
   BOOT-04, BOOT-05). The JKS writer lands with its `keytool` comparison test.
3. Collector and relay: `/v1/bootstrap` and `/bootstrap`, recorded per RELAY-02;
   update the relay and collector contract docs (GLOBAL-08).
4. Orchestrator: remove the `Extension` interface in favor of the trust policy;
   delete the extension file writers, `ext.d`, `caCertPEM` and the `/.warden`
   extension staging (BOOT-05).
5. Container driver: holder plus sidecar, create, tar copy, start, wait; delete
   the `exec` setup path and the Containerfile's `COPY .warden` / `ext.d` lines
   (BOOT-01, BOOT-02, BOOT-03, ORCH-04 for holder teardown).
6. `warden shell` on `--hold`.
7. Docs: `bootstrap.md` (the BOOT-05 text from the comparison, the diagram, remove
   D1 and D3, update the mapping row), `architecture.md` container section,
   `network-isolation.md`, `extensions.md`, `CLAUDE.md`'s extension convention.
8. Native validation per the plan above.

## Implementation notes

Steps 1 to 7 are implemented on `feat/unified-bootstrap`. Small adaptations made
during implementation:

- `initialize` takes `--workdir` (the container driver passes `/work`) rather
  than hard-coding a workdir, so the VM drivers keep their current one.
- The container's own stdout is kept as `console.log`, not `build-output.log`,
  which the collector already owns.
- docker and podman take the executor as a tar stream on `cp -`. nerdctl (finch)
  gets a staged-directory `cp` instead, from macOS or Linux hosts that keep the
  exec bit; whether nerdctl copies into a created, not yet started, container is
  unvalidated (finch validation is non-blocking).
- The holder and sidecar are published by the release workflow (job
  `support-images`) and their digests compiled in through ldflags. Development
  builds build them locally from `driver/container/images/*.Containerfile`,
  tagged by content hash.
- The JKS lives at `/etc/warden/certs.jks` (`%ProgramData%\warden` on Windows), not
  under `/.warden`, which now holds only the executor.
- Porting the OpenSSL subject hash broke its ASN.1 SET encoding; fixed, and the
  canonicalization now also trims whitespace as OpenSSL does (a pre-existing
  gap). Pinned by a test against `openssl x509 -hash` values.

Validation (2026-10-28, podman 5.8 on Windows):

- The JKS writer is byte-identical to JDK 21 `keytool -importcert -storetype JKS`
  (timestamp field normalized), and `keytool -list` accepts it.
- A build on `alpine:3.22`: COPY of a file and a directory, exclusive trust (one
  root, correct hash link), the baseline files and env, HTTPS through the relay,
  `/.warden` holding only `warden-io`, `/bootstrap` and `/build-script` recorded in
  the ledger.
- Security: an image `ENTRYPOINT` is replaced (it never runs); raw egress
  (`1.1.1.1:8080`) is blocked; `iptables` in the build container fails without
  `NET_ADMIN` and egress stays blocked.
- A build on `eclipse-temurin:21-jdk` (Ubuntu): `java $MAVEN_OPTS` fetches over
  HTTPS through the JKS (200); the JDK's own store rejects the MITM; Ubuntu's
  `update-ca-certificates` runs the `99warden` hook and restores exclusive trust.
  Alpine's `update-ca-certificates` does not run `update.d` hooks.
- `warden shell`: setup completes before the prompt, the shell starts in `/work`
  with the baseline env, and its exit code propagates.
- Podman: a failing build exits with the script's code (7); a minimal `busybox`
  image works; two concurrent builds stay separate and leave nothing behind.
- Hyper-V, Windows Server 2025 guest (relay boot disk rebuilt with the current
  relay): `/bootstrap` served and recorded (append trust); the baseline env, the
  JKS and bazelrc under `%ProgramData%`, the CA in `LocalMachine\Root`, a context
  fetch and HTTPS through the relay all pass (34 s).
- Not yet run: Linux docker (blocking), macOS finch (tracked), and the VM drivers
  with the new baseline (qemu Linux guests now get exclusive trust). Hyper-V has
  no Linux guest support yet (the seed is not implemented), so qemu on macOS is
  the only VM-driver Linux check.

## Consolidation (merge round)

Sweep (FLOW-15), 2026-10-28:

- **Duplication.** The container driver had its own Dockerfile translator
  (Containerfile rewrite plus `DockerfileToScript`) next to `driver/script`'s,
  and they had drifted: `driver/script` single-quoted ENV values (so `ENV
  PATH=$PATH:...` did not expand on VM guests) and sent COPY globs to the guest
  as directory fetches, which cannot expand a glob. `driver/script` now does what
  the container path did, for every driver: COPY expanded on the host against
  the context, ENV with Dockerfile `$VAR` semantics. `Dockerfile.actual` is
  retired; `build.sh` is the record of what ran. The extensions duplicated
  `initialize`'s CA installation and CA env vars; both now have one
  implementation in `warden-io`.
- **Dead code** (`deadcode -test`, unreachable on linux, darwin and windows).
  Removed: `driver.ExtractFromImage` (shelled out to `head`, so it never worked
  on a Windows host without Git), the unused `Logger` methods in `cmd/warden`,
  hyperv `pinState`, qemu `vmProcess.wait`, the container driver's
  `tailWriter`. Kept: exported API of library packages third-party orchestrators
  embed (`collector.Collector.BytesWritten`, `relay.Relay.CAFingerprint`,
  `relay.Relay.Ledger`). Not judged here: the `driver/vz` results
  (`createWardenUser`, `enableAutoLogin`, `NewLinuxVM`, ...), because a
  cross-platform run cannot see its cgo files; to be checked on a macOS host.
- **Overview.** `docs/book/src/concepts/overview.md` added with pseudocode for
  the orchestrator, session and collector, translator, container and VM
  drivers, `warden-io initialize`, the relay and the holder.
- Also noted, not changed: `warden clean` detects live builds by container
  names in `ps` output, but the driver passes container IDs to the runtime, so
  the check rarely sees a live build.

## Rollback notes

Revert the `feat/unified-bootstrap` commits. The relay and collector additions
(`/bootstrap`, `/v1/bootstrap`) are additive: an older executor never requests
them, and a newer executor treats a missing endpoint as the empty config
(append trust plus the baseline).

## Archive summary
