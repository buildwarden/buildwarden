# Windows-Native Build Driver — Planning

Status: **Planning**. Supersedes the dev-environment assumptions in
`hyperv-driver-plan.md` (see "Relationship to the Hyper-V plan"). Windows is a
**P1** build target per `vm-driver-release-plan.md`.

## Goal

Let BuildWarden run a build inside an **ephemeral Windows VM** with the same
guarantee every other driver gives: the build environment has no network path
except through the relay, and every network input is recorded in the signed
ledger. Development and evaluation happen **locally** on an Apple Silicon Mac
using **Parallels Desktop** (valid Windows 11 Pro license), not on cloud
bare-metal.

**Proving use-case:** conda-forge producing its `win-64` packages, such that
the conda-forge project could use BuildWarden to witness its own Windows builds.

## The reframe: separate the layers, don't build "one Windows driver"

Windows support is **not** a single driver. It is three layers, only one of
which is driver-specific. Scoping the work by layer is what lets it intermingle
cleanly with the existing container/qemu/vz drivers.

| Layer | What it is | Driver-specific? | Reused by |
|-------|-----------|------------------|-----------|
| **L1 — Guest agent** | `warden-io` port for Windows guests: network config, CA trust, fetch, exec, heartbeat, exit | **No** | Every driver that boots a Windows guest (qemu, hyperv, parallels) |
| **L2 — Build-input model** | How a Windows build is described and driven (not a `/bin/sh` script) | **No** | Same |
| **L3 — Host driver** | Boots the Windows guest and enforces topological isolation on a given host | **Yes** | One per (host, hypervisor) |

L1 and L2 are the high-leverage, low-risk foundation: pure Go, unit-testable,
and they unblock *any* host driver. L3 is where the local-dev-vehicle decision
lives.

## Current runtime contract (what a new driver must satisfy)

From reading the tree (`driver/driver.go`, `driver/vz/driver.go`,
`cmd/warden-io/`, `cmd/warden/main.go`):

- **`driver.Driver`** interface: `Name()`, `StartBuild(ctx, *BuildRequest)`,
  `Exec(ctx, *BuildRequest)` (may return `ErrExecNotSupported`), `Close()`.
- **Registration**: a `switch cfg.Runtime.Driver` in
  `cmd/warden/main.go:runBuild`/`runShell`, plus `validateDriver` /
  `validateFlagsForDriver` in `cmd/warden/config.go`. Adding a driver = new
  `case` + import.
- **VM pattern (from `vz`)**: host runs the relay as a process
  (`relay --mode=host --fd=3 --subnet=...`) reading raw Ethernet frames off a
  socketpair via gvisor netstack; the build VM's *only* NIC is that link, so
  isolation is **topological** (no firewall inside the guest). Host waits on a
  signal dir (heartbeat + exit code), then collects the ledger.
- **Guest agent (`warden-io initialize`)**, `cmd/warden-io/initialize.go`,
  runs six steps; the OS-specific ones live in `platform_<os>.go`:
  1. `configureNetwork(gateway, selfIP)` — Linux writes `/etc/resolv.conf`;
     darwin uses `scutil`/`/etc/resolver`.
  2. `waitForRelay` — HTTP `GET http://artifacts/health` (**portable**).
  3. `fetchAndInstallCA` → `installCA(pem)` — Linux `update-ca-certificates`;
     darwin `security add-trusted-cert` + `/etc/ssl/cert.pem`.
  4. `setCAEnvironment` — sets `SSL_CERT_FILE`, `REQUESTS_CA_BUNDLE`, etc.
     (**portable**, values differ).
  5. `fetchFile("build.sh")` over HTTP (**portable**).
  6. `execScript` — hardcodes `/bin/sh`|`/bin/bash`; heartbeats via
     `GET http://artifacts/heartbeat`; `reportComplete` → `POST
     http://artifacts/exit?code=N` (**HTTP portable, shell not**).
- **Build translation**: `driver/script/translate.go` turns a Dockerfile into a
  `#!/bin/sh` script and rewrites `COPY` into `warden-io fetch`. Emits POSIX
  shell only.

## Layer 1 — Windows guest agent (`warden-io` for Windows)

`warden-io` is pure Go and should cross-compile `GOOS=windows` with
`CGO_ENABLED=0`. New file `cmd/warden-io/platform_windows.go` (build tag
`//go:build windows`) implementing:

- **`configureNetwork`** — prefer DHCP served by the relay (matches the
  topological model; the guest's sole NIC faces the relay). If static is needed:
  `netsh interface ip set address` + `set dns`, or the `Set-DnsClientServerAddress`
  PowerShell cmdlet. Reserved hosts `artifacts`/`cwd` resolve via relay DNS.
- **`installCA`** — import the relay's per-build CA into the machine root store:
  `certutil -addstore -f Root <cert.pem>` or PowerShell
  `Import-Certificate -FilePath cert.pem -CertStoreLocation Cert:\LocalMachine\Root`.
  This is what makes Schannel-based tools (and thus most Windows tooling) trust
  the MITM CA.
- **`setCAEnvironment`** — still set `SSL_CERT_FILE` / `REQUESTS_CA_BUNDLE` /
  `CARGO_HTTP_CAINFO` / `NODE_EXTRA_CA_CERTS` to a written-out PEM, because
  conda/pip/openssl-based tools bypass the system store.
- **`execScript`** — branch on extension/OS: run `.ps1` via
  `powershell.exe -ExecutionPolicy Bypass -File`, `.cmd`/`.bat` via `cmd /c`.
  Keep the same heartbeat goroutine and `reportComplete`.

`execScript` in `initialize.go` needs a small refactor so the shell selection is
platform-provided rather than hardcoded to `/bin/sh`.

**This layer is the recommended first chunk** — it is the shared dependency of
every Windows-capable driver and is fully testable without a working L3 driver
(unit tests for arg parsing / cert-install command shaping; a manual smoke test
inside any Windows VM).

## Layer 2 — Windows build-input model (the conda-forge-shaped work)

The Dockerfile→`/bin/sh` translator does not fit Windows, and conda-forge builds
are **not** Dockerfiles. conda-forge drives `conda-build` over a recipe
(`meta.yaml` + `bld.bat`/`build.sh`) inside an MSVC environment activated by
`vcvarsall.bat` (today on Azure Pipelines with VS 2019/2022). So the Windows
build model should be:

- **Primary path**: run a user-supplied `build.ps1` / `bld.bat` (or a direct
  `conda-build` invocation) fetched from the relay, rather than translating a
  Dockerfile. The build recipe already exists in the conda-forge feedstock.
- **Optional later**: a Dockerfile→PowerShell emitter for parity with the other
  drivers (a `driver/script` sibling that targets PowerShell), but this is not
  needed for the conda-forge case and can wait.

**Decided (2026-09-09): minimal image, toolchain bootstrapped through the
relay.** The Windows base image does not bake in Miniforge / VS Build Tools;
the build recipe pulls the toolchain through the relay so its download is
witnessed in the ledger (faithful to BuildWarden's thesis), at the cost of a
longer first build. See the image-preparation decisions below.

**conda-forge `win-64` reproduction target** (from the pipeline research): a
`windows-2022`/`windows-2025`-equivalent image with **VS 2022 (toolset vc143,
`cl` 19.3x/19.4x)** discoverable via `vswhere`; run **`bld.bat` in a `cmd`
shell** under a **sealed conda-build env** (no ambient env inherited) with MSVC
activated (`vcvarsall`-equivalent, both build and host prefixes), **Ninja/JOM**
as the CMake generator (MSBuild isn't wired for the VS generators on these
images), exposing `PREFIX` / `LIBRARY_PREFIX` / `BUILD_PREFIX` / `SRC_DIR`;
honor the global `vc: 14` + `vs2022` pin.

**Relay/ledger constraint the conda pipeline imposes**: conda-build verifies
**sha256/md5/sha1** on every fetched source *before* extraction, and the
dominant traffic is `conda.anaconda.org` channel downloads plus hash-checked
source URLs/git clones and occasional pip. The relay's MITM must be **byte-exact**
end-to-end or these hash checks fail — worth an explicit test in the conda-forge
milestone. `HTTP(S)_PROXY` is honored by conda-build, which is the natural hook.

## Layer 3 — Host driver for the Windows guest

Three candidate vehicles; they are not mutually exclusive:

### (a) Parallels (`driver/parallels/`) — recommended **local dev harness**
Wrap `prlctl`/`prlsrvctl` to clone, configure, boot and destroy a Windows 11 VM.
Microsoft **authorizes** Parallels Desktop 18–20 for Windows 11 ARM on Apple
Silicon, so this is a sanctioned, turnkey Windows boot on Jeff's Mac. Risk to
resolve before it could ever ship: Parallels' network isolation. We need the
guest's only reachable peer to be the host relay. Parallels offers host-only /
"isolated" networking, but proving "no egress except through relay" is less
obviously airtight than the vz/qemu socketpair topology. Treat Parallels as the
**iteration harness for L1/L2**, and gate any "shipped driver" status on a
security-validated isolation story (per the release plan's adversarial gate).

### (b) qemu + Windows guest — recommended **shippable cross-platform path**
The `qemu` driver already exists and runs on macOS. Windows 11 **ARM** as a qemu
guest under **HVF** on Apple Silicon boots at native speed and reuses the exact
socketpair-topological isolation the other VM drivers already prove. This needs
no proprietary dependency and no new driver package — just Windows-guest support
(L1/L2 + unattend/provisioning) inside `driver/qemu`. x64 Windows under qemu on
Apple Silicon is TCG emulation (very slow) — fine for correctness smoke tests,
not for real builds.

### (c) hyperv (`driver/hyperv/`) — production Windows-host path (later)
For operators whose host is Windows x64. This is the existing
`hyperv-driver-plan.md`, and it is the natural home for **native `win-64`**
builds. Not a local-dev vehicle for Jeff (nested virt is x64-only, so it cannot
run nested inside a Parallels Windows-ARM guest). Research refined the design:
- **Programmatic surface**: prefer Microsoft's **HCS API via `microsoft/hcsshim`**
  (MIT, Go, used by containerd/Moby) over shelling PowerShell — HCS is the
  sanctioned local single-machine VM/container API. hcsshim's *documented* path
  is container-oriented; the standalone-VM path uses its lower-level HCS layer,
  so a **Hyper-V PowerShell** v1 is the pragmatic first cut, HCS the v2.
- **VM generation**: **Gen2** (UEFI, Secure Boot, vTPM) for modern Windows guests.
- **Relay-only isolation recipe (all supported)**: attach the guest's single NIC
  to a **private/internal vSwitch** with no external uplink, put the relay on that
  same switch, and apply **port ACLs** (`Add-VMNetworkAdapterAcl` / Extended Port
  ACLs) whitelisting only the relay's MAC/IP — a deny-all-except-relay posture.
- **Windows Sandbox** now has a scriptable `wsb` CLI but only on/off networking —
  too coarse to force traffic through a relay. Useful for quick prototyping, not
  the isolation guarantee.

## The two pivotal decisions

### Decision 1 — local host vehicle: Parallels driver vs. extend qemu
- **Parallels driver**: fastest, most reliable Windows-11-ARM boot on Jeff's
  Mac; Microsoft-authorized; but adds a proprietary dependency to an Apache-2.0
  project and needs an isolation-proof before it's more than a harness.
- **Extend qemu**: no new dependency, reuses proven topological isolation,
  becomes a genuinely shippable cross-platform Windows path; but Windows-on-qemu
  provisioning (UEFI, virtio drivers on Windows, unattend) is more finicky to
  get booting than Parallels.

Recommendation: build **L1 first** (it's needed either way), iterate it on
**Parallels** because it boots today, and design the *shippable* Windows-guest
support inside **qemu**. Promote a Parallels driver only if users want it and
its isolation is validated.

### Decision 2 — ARM vs x64 / the conda-forge reality
This is the biggest risk to the stated use-case. Local Parallels/HVF gives you
**Windows 11 ARM (`win-arm64`)**. conda-forge's eponymous channel builds
**`win-64` (x86-64)**. The L1/L2 plumbing is **architecture-agnostic** and can be
fully developed on `win-arm64` locally. But a real `win-64` conda-forge build
needs an **x64 Windows environment**:
- an x64 Windows box with Hyper-V (driver c), or
- a Linux/amd64 host running qemu-KVM booting Windows x64 (driver b, fast), or
- x64 emulation as a slow last resort (see below).

Research confirmed how sharp this edge is:
- **Raw MSVC/CMake cross-compile works natively and correctly.** On Arm64
  Windows, `vcvarsall.bat arm64_amd64` runs a native Arm64 compiler that emits
  x86-64 machine code (VS 2022 17.4+; install the ARM64→x64 build-tools
  component). The output is identical machine code to an x64 host —
  [MSVC command-line build](https://learn.microsoft.com/en-us/cpp/build/building-on-the-command-line?view=msvc-170).
- **But conda-build does not.** conda-forge has only partial/experimental
  `win-arm64` support and cross-compiles `win-arm64` *from* `win-64` — the
  *opposite* direction. There is no supported "build `win-64` from arm64" conda
  workflow; Miniforge for Windows arm64 doesn't ship, so you'd run **x86-64
  Miniforge + conda-build under x64 emulation** inside the ARM VM — slow,
  unsupported, and any freshly-built x64 test binary also executes emulated
  ([conda-forge win-arm64, Feb 2026](https://conda-forge.org/blog/2026/02/09/win-arm64/)).
- **Nested Hyper-V is a dead end on Apple Silicon.** Nested virtualization is
  x64-only, so you cannot run the `hyperv` driver nested inside a Parallels
  Windows-ARM guest
  ([nested virt](https://learn.microsoft.com/en-us/windows-server/virtualization/hyper-v/nested-virtualization)).

**Decouple**: develop and demo the plumbing on ARM locally; validate the
`win-64` conda-forge build on a genuine x64 host as a separate milestone — that
is the only fully-supported win-64 conda story.

## Image preparation (decided 2026-09-09)

Windows image prep matches the vz driver's automated lifecycle (`warden image
restore` / `prepare` / `list` + per-build COW clones). Decisions:

- **Acquisition — both.** Operator-supplied licensed media, and auto-download of
  a Microsoft evaluation ISO. (The clean eval ISO is x64-only; `win-arm64` has no
  equally clean auto-download, so local ARM dev supplies ARM64 media.)
- **Prep — full unattended install from ISO** via a generated `Autounattend.xml`
  (`driver/qemu/autounattend.go`): LabConfig bypass, virtio-win driver load in
  WinPE, headless OOBE, ephemeral autologon admin, and a startup Scheduled Task
  running the WARDEN seed's `warden-run.ps1`.
- **Firmware — LabConfig bypass** now. **swtpm + Secure Boot is a documented
  opt-in future feature to prioritize on explicit request** (a customer needing a
  "supported" Win11 config, a future measured-boot feature, or an ARM64 installer
  that rejects the bypass). Firmware is built as a pluggable policy so this needs
  no driver rework. Rationale: the guest is untrusted by design, so an emulated
  TPM adds nothing to the ledger and does not change compiler output.
- **Toolchain — minimal image**, bootstrapped through the relay (see Layer 2).

Details and the operator contract live in `tools/win-image-prep/README.md`.

## Relationship to the Hyper-V plan (reuse vs. discard)

`hyperv-driver-plan.md` is kept for its **reusable** research; its **dev/CI
pattern is discarded**.

**Discard** (cloud/bare-metal dev assumptions):
- "Can we test in CI without bare-metal? … Azure VMs with nested virtualization
  … self-hosted runner on Azure Ev3." We develop locally on Parallels instead.
- The assumption that warden runs *on a Windows host*. That is one production
  target (driver c), not the local-dev topology.

**Reuse** (host-agnostic, valuable):
- Windows guest provisioning via **`unattend.xml`** + `SetupComplete.cmd` to skip
  OOBE, set an ephemeral random admin password, and launch the agent.
- Getting the agent in: **seed disk** (attach a small VHDX/ISO with
  `warden-io.exe` + build recipe) vs. **HTTP fetch from relay** once networking
  is up. Seed disk is more robust; relay-fetch keeps the download in the ledger.
- CA trust via `certutil -addstore Root` (feeds directly into L1).
- Network isolation as a **host-enforced** property (the guest's sole NIC faces
  the relay), matching vz/qemu — not in-guest firewall rules.
- `warden-io` is pure Go → `GOOS=windows CGO_ENABLED=0` (feeds L1).
- Build-tag layout: `driver_windows.go` (impl) + `driver_stub.go`
  (`//go:build !windows`) so non-Windows hosts still compile.

## Legal / licensing findings (verified 2026-09-09)

- **Parallels + Windows 11 ARM is Microsoft-authorized.** Parallels Desktop
  18/19/20 are named authorized solutions for Arm Windows 11 Pro/Enterprise on
  Apple M-series. Source:
  [Microsoft support — options for Windows 11 on M1/M2/M3](https://support.microsoft.com/en-us/windows/experience/platform-variants/options-for-using-windows-11-with-mac-computers-with-apple-m1-m2-and-m3-chips).
- **VS Build Tools is free to install on build machines with no per-machine
  license.** The standalone VSBT EULA's **"Build Server"** clause lets you install
  it onto build machines (incl. VMs/containers and ephemeral cloud agents)
  "solely for the purpose of compiling, building, verifying and archiving your
  applications or running quality or performance tests." A 2022 update further
  allows compiling **open-source** C++ deps from source **without any VS license,
  even for enterprise/commercial/closed-source projects** — which is exactly the
  conda-forge case. A paid VS (or Community, where eligible) is still required
  only for *interactive proprietary development* in the full IDE. Sources:
  [VSBT EULA mt644918](https://visualstudio.microsoft.com/license-terms/mt644918/),
  [cppblog 2022 OSS-build update](https://devblogs.microsoft.com/cppblog/updates-to-visual-studio-build-tools-license-for-c-and-cpp-open-source-projects/),
  [MS Q&A — ephemeral CI agents](https://learn.microsoft.com/en-us/answers/questions/5685362/clarification-on-visual-studio-licensing-for-ephem).
- **Windows images**: BuildWarden ships none. Operators supply their own
  licensed Windows media (retail/OEM/Volume) or Microsoft's time-limited
  evaluation ISOs/VHDXs. Matches the existing `platform-compatibility.md` legal
  note.
- **Two engineering caveats surfaced by the licensing research** (not blockers,
  but track them): (1) redistributing the **MSVC C++ runtime** with shipped
  binaries must follow the Distributable Code list / `vc_redist`; (2) **vcpkg**
  itself is MIT, but each package it builds carries its **own** upstream license
  — anything shipped must have that license tracked/surfaced.

## Phasing

Development splits at **what can be validated locally**, not by architecture
alone:

- **Phase 1 — local, on Apple Silicon + Parallels (`win-arm64`). This session
  owns it.** Everything architecture-agnostic and provable on this laptop: the
  guest agent (CA trust via `certutil`, `powershell`/`cmd` exec, network config
  — all arch-identical), the Windows build-input model, a local host driver, and
  isolation validation, all exercised end-to-end against a `win-arm64` guest.
  Phase 1 also **cross-compiles the `warden-io` `windows/amd64` artifact** (Go
  cross-build is trivial) even though it is only *run* in Phase 2. This de-risks
  the large majority of the driver.
- **Phase 2 — on an x86-64 Windows desktop (`win-64`), planned in a separate
  (cloned) session.** Only the parts that genuinely need native x64: the real
  conda-forge `win-64` build, the `hyperv` driver (nested virt is x64-only, so it
  cannot run under Parallels on Apple Silicon), byte-exact MITM validation
  against live anaconda.org channel traffic, and native `win-64` performance
  baselines. Jeff runs Phase 2 on his x86-64 Windows desktop; that planning
  happens in a cloned chat while Phase 1 executes here.

**Explicitly out of scope for this laptop / deferred to Phase 2:** native
`win-64` compilation and testing, and Hyper-V-in-Hyper-V (nested virtualization).
These are accepted limitations, not gaps to work around.

## Proposed chunking (issues, roughly ordered)

Each is independently mergeable and sized to intermingle with the existing
drivers.

**Phase 1 (local, `win-arm64`):**
1. **`warden-io` Windows port (L1)** — `platform_windows.go`; refactor
   `execScript` shell selection; `installCA` via `certutil`; CA env vars; unit
   tests + a manual smoke test inside a Windows VM. Also emit the
   `windows/amd64` cross-build for Phase 2. *(Foundation — do first.)*
2. **Windows build-input model (L2)** — run a fetched `.ps1`/`.bat` /
   `conda-build` recipe through the relay; decide baked-toolchain vs.
   relay-bootstrapped toolchain; add a `conda-forge win` example.
3. **Local host driver (Decision 1)** — either `driver/parallels/` (`prlctl`
   wrapper, host relay + isolated net) or Windows-guest support in
   `driver/qemu/` (unattend + provisioning). Boot → agent → trivial build →
   valid ledger, end to end, on `win-arm64`.
4. **Isolation validation** — adversarial "escape the build env" agent for the
   Windows guest (release-plan security gate), incl. Windows-specific vectors
   (SMB/NetBIOS discovery, stack fallbacks).
5. **Docs + matrix** — update `platform-compatibility.md` and the book; document
   image sourcing per the release plan's graduation criteria.

**Phase 2 (x86-64 Windows desktop, planned in a separate session):**
6. **conda-forge `win-64` milestone (Decision 2)** — reproduce a real conda-forge
   `win-64` build on native x64 and produce a verifiable ledger of every input,
   including a byte-exact-MITM test against conda-build's sha256/md5/sha1 source
   checks on live anaconda.org traffic.
7. **`hyperv` driver** — the production x64 Windows-host driver (HCS/`hcsshim`,
   Gen2, private-vSwitch + port-ACL isolation) and native `win-64` performance
   baselines.

Legal/licensing is resolved inline above; only MSVC-runtime redistribution and
vcpkg per-package licenses remain as engineering caveats, tracked in Phase 2
where distribution happens.

## Graduation gates (from `vm-driver-release-plan.md`)

A Windows host×driver×target combo ships only when: implementation complete;
integration-tested end to end (valid ledger); **security-validated by an
independent adversarial agent** (zero unrecorded egress); performance baselined
(boot, build latency, throughput); image sourcing documented; error handling
graceful.
