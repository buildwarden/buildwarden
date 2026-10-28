# Build-Environment Bootstrap Model

How every driver gets a build environment from "started" to "running the build
script". This is the model the drivers share; the rules below are its
invariants, and BOOT-07 is how the model itself is allowed to change.

See [`README.md`](./README.md) for the rule format.

## The model

```
  HOST (warden + collector)                    BUILD ENVIRONMENT
  ==========================                   ==================================

  (1) isolate ------------------------------>  sole network path: the relay
                                               (exists before any guest code runs)

  (2) deliver the executor ----------------->  warden-io + a launcher
      per build, version-matched               (seed disk, cloud-init ISO,
                                               copied-in file, ...)

  (3) start -------------------------------->  launcher runs `warden-io initialize`
      then hands off; no further host            |
      steps inside the environment               |   all of it over the relay:
                                                 |-- GET  /health
                                                 |-- GET  /ca.pem         trust
                                                 |-- GET  /bootstrap/...  extension material
                                                 |-- GET  /build-script
                                                 |-- run the script
                                                 |     '-- GET cwd/<path>  (each COPY)
                                                 '-- POST heartbeat, artifacts, exit
                                                          |
  collector <==== tunnel ==== relay <=====================+
  (serves every input, lands every output)
```

`/bootstrap/...` does not exist yet; [`unified-bootstrap`](../design/unified-bootstrap.md)
adds it (D1, D3). Every other endpoint is the relay's current guest API.

Only the executor crosses into the environment outside the relay. Everything
the build consumes or produces passes through the relay, where it is recorded.

### Per-driver mapping

| Driver / guest | (1) Isolation | (2) Executor delivery | (3) Start trigger |
|---|---|---|---|
| qemu, hyperv / Linux | Topology: the only NIC reaches the relay VM | cloud-init `CIDATA` ISO per build | cloud-init `runcmd` |
| qemu, hyperv / Windows | Topology | `WARDEN` seed disk per build (`warden-io.exe`, `warden-run.ps1`) | `warden-run` startup task baked at prepare |
| vz / macOS | Topology: one socketpair NIC to the relay | **Baked into the image at prepare** (D2) | launchd daemon baked at prepare |
| container | iptables applied from a privileged sidecar | **`cp` of `/.warden` into a running container** (D1) | **host `exec` steps, then `exec warden-io initialize`** (D1) |

### Deviation register

Known places where a driver does not yet follow the model. Removing an entry
needs only the change that removes it. Adding one is a model change (BOOT-07).

| ID | Driver | Invariants | Deviation | Plan |
|---|---|---|---|---|
| D1 | container | BOOT-01, BOOT-02, BOOT-03, BOOT-05 | The container is started as `<image> sleep infinity` on a routable bridge, so an image `ENTRYPOINT` runs before isolation; the host then applies isolation, rewrites `resolv.conf`, copies `/.warden` (executor plus extension material) and runs setup through `exec` before starting `initialize`. | [`unified-bootstrap`](../design/unified-bootstrap.md) |
| D2 | vz | BOOT-02, BOOT-06 | `warden-io` and its launchd daemon are baked into the prepared image, so the executor version is the one from the last prepare. | Tracked; not in the current design |
| D3 | qemu, vz, hyperv | BOOT-05 | Extensions (truststore, bazel, ...) run only on the container driver; VM guests get none of their material. | Transport fixed by `unified-bootstrap`; VM guest support tracked |

---

### BOOT-01 — Isolation precedes execution

**Severity:** MUST
**Check:** agent-judgment

The build environment's only network path is the relay before any code from the
build environment runs: the launcher, the executor and the build script alike.
Topological isolation satisfies this by construction. Isolation applied to an
environment that is already running does not, unless nothing in it can run until
the isolation is in place.

This is a security boundary (GLOBAL-06).

---

### BOOT-02 — The executor is the only out-of-band input

**Severity:** MUST
**Check:** agent-judgment

The host delivers exactly one thing to the build environment outside the relay:
the executor (`warden-io`) and the minimal launcher that starts it. It is
delivered per build and matches the orchestrator's version. The build script,
context files, the CA and extension material never travel this way.

---

### BOOT-03 — One start, then hands off

**Severity:** MUST
**Check:** agent-judgment

The environment starts `warden-io initialize` itself, through its platform's
boot mechanism or entrypoint. After starting the environment the host performs no
steps inside it (no `exec`, no file copy, no guest-side commands); it only
observes, through the relay and the environment's lifecycle state.

The launcher locates the executor and starts it. Static network addressing
applied by the platform's own boot configuration (cloud-init `network-config`, a
fixed address on a single NIC) is allowed in the launcher; trust, DNS and every
other setup step belong to `initialize`.

Interactive debugging (`warden shell`) is not a build and is exempt once
`initialize` has finished setup.

---

### BOOT-04 — Build inputs come from the relay

**Severity:** MUST
**Check:** automated + agent-judgment

The CA, the build script and every context file reach the build environment by
the executor fetching them from the relay (`/ca.pem`, `/build-script`,
`http://cwd/<path>`), which pulls them from the collector. This keeps every byte
the build consumes inside the relay's record (RELAY-02).

The automated part checks that the executor's fetch paths are the only readers of
build inputs in `cmd/warden-io/`.

---

### BOOT-05 — Setup material comes from the relay too

**Severity:** MUST
**Check:** agent-judgment

Anything the orchestrator generates to set up the environment, such as extension
output (a Java truststore, a `bazelrc`, setup scripts), is served by the relay and
installed by `initialize`, the same way as the CA. It is not copied in by the
driver.

---

### BOOT-06 — Images carry hooks, not content

**Severity:** MUST
**Check:** agent-judgment

A static or prepared image may carry a generic hook that finds and starts the
per-build executor (the Windows `warden-run` task, cloud-init itself). It carries
nothing per-build and nothing tied to a warden version, so a warden upgrade never
needs an image re-prepare to take effect.

---

### BOOT-07 — The model changes only by a signed comparison

**Severity:** MUST-review
**Check:** automated + agent-judgment

A change that alters the model above (the diagram, an invariant, the per-driver
mapping beyond correcting it to match the code, or the deviation register by
adding an entry) is a model change. A model change needs a design document with a
**Bootstrap comparison** section, and the human sign-off step (FLOW-08) must
explicitly accept that comparison before implementation starts. The comparison
contains:

1. The current model diagram and the proposed one, side by side or one after the
   other.
2. A table with one row per invariant (BOOT-01 to BOOT-06): current, proposed,
   and what changes for each driver.
3. What the change gains, and what it costs in security posture and in the
   record of what the build consumed.

Changes that bring a driver into line with the model, or that leave it untouched,
are not model changes. They still name the invariants they rely on in their
design's `rules`.

The automated part flags a diff that touches the bootstrap surface without
touching this document, or that changes this document without a design whose
`rules` cites BOOT-07. The bootstrap surface is `cmd/warden-io/initialize*.go`, the
relay's guest-facing endpoints, and each driver's executor delivery and start
code (`driver/container/container.go`, `driver/qemu/cloudinit.go`,
`driver/winguest/`, `driver/hyperv/winseed*.go`, `driver/vz/image.go`). The agent
part judges whether a change counts as a model change.
