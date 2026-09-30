# build.ps1 - Phase 2 numpy SOURCE build with MSVC, for the Windows Hyper-V guest.
#
# Unlike examples/conda-forge-win (which INSTALLS a prebuilt numpy package and
# validates the package-fetch + hash-verify path), this COMPILES numpy from
# source with the MSVC toolchain. It proves BuildWarden witnesses a full source
# build end to end, including toolchain ACQUISITION:
#
#   1. Visual C++ Build Tools (VCTools) - the bootstrapper and its several GB of
#      components download THROUGH the relay and are recorded in the ledger.
#      (conda-forge's vs2022_win-64 is only an activation shim; the actual MSVC
#      compiler must be installed on the guest.)
#   2. Miniforge + a python env (witnessed conda-forge fetches).
#   3. `pip install --no-binary numpy numpy` - build isolation pulls
#      meson-python / meson / Cython / ninja (witnessed from PyPI), then meson
#      drives an MSVC compile of the numpy sources (-Db_vscrt=md), producing a
#      wheel that is built, not downloaded.
#
# Every network input above flows through the relay's MITM and is signed into
# the ledger. On the Hyper-V driver this needs more than the default build-VM
# resources; run with WARDEN_HYPERV_BUILD_MEMORY_MB=8192 WARDEN_HYPERV_BUILD_CPUS=6.
#
#   warden build --driver hyperv --guest-os windows examples/numpy-source-win

$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'
Write-Host '=== Phase 2: numpy SOURCE build with MSVC (witnessed) ==='

# 1) Visual C++ Build Tools (VCTools) - witnessed download + install.
$vs = Join-Path $env:TEMP 'vs_BuildTools.exe'
Write-Host 'downloading VS Build Tools bootstrapper...'
Invoke-WebRequest -UseBasicParsing -Uri 'https://aka.ms/vs/17/release/vs_BuildTools.exe' -OutFile $vs
Write-Host 'installing VC++ Build Tools (downloads several GB via the relay)...'
$p = Start-Process -FilePath $vs -Wait -PassThru -ArgumentList @(
  '--quiet','--wait','--norestart','--nocache',
  '--add','Microsoft.VisualStudio.Workload.VCTools','--includeRecommended')
if ($p.ExitCode -ne 0 -and $p.ExitCode -ne 3010) { throw "VS Build Tools install failed ($($p.ExitCode))" }
Write-Host "VS Build Tools installed (exit $($p.ExitCode))"

# 2) Miniforge (witnessed).
$installer = Join-Path $env:TEMP 'Miniforge3.exe'
Write-Host 'downloading Miniforge...'
Invoke-WebRequest -UseBasicParsing -Uri 'https://github.com/conda-forge/miniforge/releases/latest/download/Miniforge3-Windows-x86_64.exe' -OutFile $installer
$prefix = 'C:\Miniforge3'
Start-Process -FilePath $installer -Wait -ArgumentList '/InstallationType=JustMe','/RegisterPython=0','/S',"/D=$prefix"
$conda = Join-Path $prefix 'Scripts\conda.exe'
if (-not (Test-Path $conda)) { throw "conda not found at $conda" }
if ($env:REQUESTS_CA_BUNDLE) { $env:CONDA_SSL_VERIFY = $env:REQUESTS_CA_BUNDLE; Write-Host "CONDA_SSL_VERIFY=$env:CONDA_SSL_VERIFY" }
$env:PIP_CERT = $env:REQUESTS_CA_BUNDLE

# 3) Python env (witnessed conda-forge fetches).
Write-Host 'creating build env (python + pip)...'
& $conda create -y -n src --override-channels -c conda-forge python=3.12 pip
if ($LASTEXITCODE -ne 0) { throw "conda create failed ($LASTEXITCODE)" }

# 4) Compile numpy FROM SOURCE with MSVC. --no-binary forces building the sdist;
#    pip build isolation pulls meson-python/cython/ninja (all witnessed) and
#    meson locates MSVC via vswhere. This is the real source compile.
Write-Host 'building numpy from source with MSVC (pip --no-binary)...'
& $conda run -n src pip install --no-binary numpy numpy==2.1.3 -v
if ($LASTEXITCODE -ne 0) { throw "numpy source build failed ($LASTEXITCODE)" }

# 5) Verify it imports and works.
& $conda run -n src python -c "import numpy; print('numpy', numpy.__version__, 'from-source OK'); import numpy.linalg as l; print('linalg', l.norm([3,4]))"
if ($LASTEXITCODE -ne 0) { throw "numpy verify failed ($LASTEXITCODE)" }
Write-Host '=== Phase 2 source build: SUCCESS ==='
exit 0
