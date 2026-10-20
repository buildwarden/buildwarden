# conda-forge Windows build (example)

BuildWarden's Windows target does not translate a Dockerfile. Windows builds -
and conda-forge builds in particular - are driven by a native build script
(`build.ps1` / `bld.bat`) or a `conda-build` recipe, not by `RUN` layers. So the
Windows build-input model is **bring your own script**:

- The driver places your `build.ps1` (or `.bat`) into the build context.
- Inside the guest, `warden-io initialize` fetches it from the relay
  (`http://cwd/build.ps1`), installs the per-build CA (`certutil -addstore -f
  Root`), sets `SSL_CERT_FILE` / `REQUESTS_CA_BUNDLE` / ... for tools that bypass
  the system store, runs the script (`.ps1` via PowerShell, `.cmd`/`.bat` via
  cmd), and reports its exit code to the relay.
- The default script name on Windows guests is `build.ps1` (vs `build.sh`
  elsewhere); override with `warden build --script <name>`.

Run it:

```sh
warden build --driver qemu --guest-os windows examples/conda-forge-win
```

## What `build.ps1` does (Phase 1 validation)

It proves BuildWarden audits a conda-forge **package install** end to end:

1. Downloads the x86_64 Miniforge installer **through the relay** (the toolchain
   download is itself witnessed in the ledger).
2. Installs Miniforge silently, points `CONDA_SSL_VERIFY` at the per-build CA
   (conda ships its own vendored `requests` and ignores `REQUESTS_CA_BUNDLE`).
3. `conda create -n bw --override-channels -c conda-forge numpy` - every
   repodata + `.conda` package fetch (numpy + python + libblas/openblas + deps)
   flows through the relay and is hash-verified by conda against the exact bytes
   the relay ledgered, so the MITM must be byte-exact.
4. `import numpy` in the new env to confirm it actually works.

## Toolchain provisioning (decided)

The toolchain is **bootstrapped through the relay**, not baked into the base
image: the Miniforge download and all channel/package traffic are recorded in
the ledger, which is more faithful to BuildWarden's thesis (witness every
network input) at the cost of a slightly longer first build. Baking Miniforge
into the base image would be faster but would leave the toolchain download
unwitnessed.

## Architecture note: x64 emulation on win-arm64

Phase 1 is developed/validated locally on **win-arm64** (qemu on Apple Silicon).
There is no native arm64 Miniforge yet and conda-forge's arm64 channel is only
partial, so - per conda-forge's own Windows/ARM guidance - we install the
**x86_64** Miniforge and run it under Windows' built-in x64 emulation, pulling
the fully-populated **win-64** channel. This validates the guest-agent + relay
plumbing (which is architecture-agnostic).

## Phase boundary

This example validates a conda-forge **install** (package fetch + verify)
through the relay. A rigorous **source `conda build`** (compiling numpy with the
conda-forge `vs2022_win-64` MSVC compiler package) is a **Phase 2** milestone: it
runs on a native x86-64 Windows host, needs no emulation, and drives the recipe
via `conda build`. See `docs/design/windows-driver-plan.md`.
