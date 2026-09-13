# In-VM Executor Standards

Standards for the in-VM executor: `cmd/warden-io/`. The executor is the
build-environment guest agent, with subcommands `initialize` (full lifecycle),
`fetch`, `post`, and `trust`. It is the build-environment entry point and part of
the trust boundary that makes measurement possible.

See [`README.md`](./README.md) for the rule format.

---

### EXEC-01 — Pure Go, no cgo, all guest targets

**Severity:** MUST
**Check:** automated + agent-judgment

`warden-io` builds with `CGO_ENABLED=0` for every supported guest target: linux,
darwin, and windows on amd64 and arm64. Platform specifics live in
`platform_<os>.go` files with stubs so all targets compile. This is the executor's
instance of GLOBAL-07.

The automated part cross-builds every executor target in CI.

---

### EXEC-02 — Minimal-environment robustness

**Severity:** MUST
**Check:** agent-judgment

The executor runs in a stripped guest and assumes nothing about which tools,
interpreters, or paths exist beyond what it carries itself. Failures produce a
structured exit reported back through the relay, never a silent death.

---

### EXEC-03 — Trust-install parity across guests

**Severity:** MUST
**Check:** automated + agent-judgment

CA trust installation behaves equivalently across supported guest OSes
(`certutil -addstore Root` on Windows, the system store on Linux/darwin). A new
platform's trust path is not "done" until it is implemented for all supported
guests, or the gap is explicitly deferred and tracked.

This is security-boundary work, so GLOBAL-06 fires. The parity logic is unit-tested
via stubs on every platform; the actual CA install is validated per-guest in the
tier-2 tests (TEST-03), which is how contribution stays un-gated on owning every
platform.

The automated part checks `platform_*.go` coverage for the trust path.

---

### EXEC-04 — Versioned public contract

**Severity:** MUST
**Check:** automated + agent-judgment

The executor exposes a versioned public interface under GLOBAL-08:

- the `initialize` invocation contract: how an orchestrator launches it, its env,
  args, and exit reporting; and
- its side of the relay protocol: CA install, artifact `POST`, heartbeat, and exit
  report.

Changes follow the pre/post-1.0 seam posture and update the contract document. The
automated part requires a change to the contract surface to also change its contract
doc.

---

### EXEC-05 — Embed large assets

**Severity:** SHOULD
**Check:** automated + agent-judgment

Literal blobs (embedded scripts, templates) go in `//go:embed` files, not Go string
literals. The `lll` linter scans string literals, and inline blobs are
unmaintainable. The automated part greps for large string literals; the agent part
judges borderline cases.
