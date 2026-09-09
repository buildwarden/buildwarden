# conda-forge Windows build (example)

BuildWarden's Windows target does not translate a Dockerfile. Windows builds,
and conda-forge builds in particular, are driven by a native build script
(`build.ps1` / `bld.bat`) or a `conda-build` recipe, not by `RUN` layers. So the
Windows build-input model is **bring your own script**:

- The driver places your `build.ps1` (or `.bat`) into the build context.
- Inside the guest, `warden-io initialize` fetches it from the relay, installs
  the per-build CA (`certutil -addstore -f Root`), sets `SSL_CERT_FILE` /
  `REQUESTS_CA_BUNDLE` / ... for tools that bypass the system store, then runs
  the script (`.ps1` via PowerShell, `.cmd`/`.bat` via cmd) and reports its exit
  code to the relay.
- The default script name on Windows guests is `build.ps1` (vs `build.sh`
  elsewhere); override with `warden build --script <name>`.

`build.ps1` here shows the shape of a conda-forge-style build: `conda build`
over a recipe, with MSVC activated by conda-forge's `vs2022_win-64` compiler
package rather than a manual `vcvarsall.bat` call.

## Open decision: toolchain provisioning

Two ways to get Miniforge + the VS Build Tools into the guest:

1. **Baked into the base image** - faster first build, but the toolchain
   download is not witnessed in the ledger.
2. **Bootstrapped through the relay** - the toolchain download itself is
   recorded in the ledger (more faithful to BuildWarden's thesis), at the cost
   of a longer first build.

This is still open in `docs/design/windows-driver-plan.md` and is settled as
part of the host-driver / image-prep work (chunk 3).

## Phase boundary

The guest-agent plumbing this exercises is architecture-agnostic and is
developed/validated locally on `win-arm64` (Parallels on Apple Silicon). A real,
verified **win-64** conda-forge build runs on a native x86-64 Windows host and
is a **Phase 2** milestone - see the plan. Nothing here has been executed
end-to-end yet; it documents the intended contract.
