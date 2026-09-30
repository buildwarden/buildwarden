# numpy source build (Windows, MSVC) — Phase 2 example

Where `examples/conda-forge-win` validates a package **install** (fetch +
hash-verify through the relay), this example validates a full **source build**:
numpy is compiled from source with the MSVC toolchain, and BuildWarden witnesses
the entire build — *including acquiring the compiler itself*.

## What `build.ps1` does

1. **Installs Visual C++ Build Tools (VCTools)** unattended. The stock Windows
   Server eval guest has no compiler, and conda-forge's `vs2022_win-64` package
   is only an activation shim (it points at an existing VS install, it does not
   ship MSVC). So the `vs_BuildTools.exe` bootstrapper and its several GB of
   components are downloaded **through the relay** and recorded in the ledger.
2. Installs Miniforge and creates a `python` + `pip` env (witnessed).
3. `pip install --no-binary numpy numpy==2.1.3` — build isolation pulls
   `meson-python` / `meson` / `Cython` / `ninja` (witnessed from PyPI), then
   meson drives an **MSVC** compile of the numpy sources (`-Db_vscrt=md`),
   producing a wheel that is *built, not downloaded*.
4. Imports numpy and runs `linalg.norm` to confirm the compiled result works.

## Run it

```sh
warden build --driver hyperv --guest-os windows examples/numpy-source-win
```

The build VM needs more than the default 4 GB / 4 vCPU for the VS install +
parallel MSVC compile:

```
WARDEN_HYPERV_BUILD_MEMORY_MB=8192 WARDEN_HYPERV_BUILD_CPUS=6
```

## Measured result (2026-09-30, Hyper-V driver)

End-to-end in ~9 minutes on a native x64 host:

```
numpy 2.1.3 from-source OK, linalg 5.0
warden inspect: 3272 records, 811 requests, 2.2 GB audited
  SIGNATURES: ✅ All 3273 signatures valid
  COMPLETENESS: ✅ All channels closed
```

The 2.2 GB / 811 requests span the VS Build Tools install, the conda-forge
python env, the PyPI build backend, and the numpy source — every byte witnessed
and signed.

## Note: this compiles via `pip --no-binary`, not `conda build`

This is the fast, faithful validation of *witnessed MSVC source compilation*. A
strict conda-forge `conda build` of the numpy **feedstock** (recipe-driven,
activation packages, test suite) is a further step on top of the same
witnessed-toolchain foundation.

## Future: ledger'd derived images

Installing VS Build Tools on every build is witnessed but slow. The natural next
step is a **derived base image** built from a blank VM with its own composition
ledger, whose entries are prepended to every build that starts from it — so a
VS-preinstalled image stays fully provenance-tracked without re-installing the
toolchain each run. That mechanism does not exist yet (no driver has it today).
