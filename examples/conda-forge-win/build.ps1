# build.ps1 - conda-forge validation build for the Windows (qemu) guest.
#
# warden-io fetches this from the relay (http://cwd/build.ps1), installs the
# per-build CA into the trust store, sets REQUESTS_CA_BUNDLE / SSL_CERT_FILE,
# then runs it via:
#   powershell.exe -NoProfile -ExecutionPolicy Bypass -File build.ps1
# and reports the exit code back to the relay automatically. Every network
# input below flows through the relay and is recorded, signed, into the ledger.
#
# Phase 1 (this script): prove BuildWarden audits a conda-forge PACKAGE INSTALL
# end to end. On win-arm64 there is no native Miniforge and conda-forge's arm64
# channel is only partial, so - following conda-forge's own guidance for
# Windows/ARM - we install the x86_64 Miniforge and let Windows' built-in x64
# emulation run it, pulling the fully-populated win-64 channel. A full source
# `conda build` (compiling numpy with the MSVC toolchain) is deferred to Phase 2
# on a native x86-64 Windows host, where no emulation is involved.

$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'  # no progress UI -> faster Invoke-WebRequest

Write-Host '=== conda-forge validation build (numpy) ==='

# 1) Bootstrap Miniforge (x86_64) THROUGH the relay, so the toolchain download
#    is itself witnessed in the ledger. Invoke-WebRequest uses Schannel + the
#    machine Root store, into which warden-io imported the per-build CA.
$installer = Join-Path $env:TEMP 'Miniforge3.exe'
$url = 'https://github.com/conda-forge/miniforge/releases/latest/download/Miniforge3-Windows-x86_64.exe'
Write-Host "downloading Miniforge: $url"
Invoke-WebRequest -UseBasicParsing -Uri $url -OutFile $installer

$prefix = 'C:\Miniforge3'
Write-Host "installing Miniforge -> $prefix"
# NSIS silent install: JustMe, no PATH/registry pollution.
Start-Process -FilePath $installer -Wait -ArgumentList `
    '/InstallationType=JustMe', '/RegisterPython=0', '/S', "/D=$prefix"
$conda = Join-Path $prefix 'Scripts\conda.exe'
if (-not (Test-Path $conda)) { throw "conda not found at $conda after install" }

# 2) Point conda's TLS verification at the relay CA. conda ships its own
#    (vendored) requests and does NOT read REQUESTS_CA_BUNDLE; it honors
#    CONDA_SSL_VERIFY / ssl_verify. warden-io wrote the per-build CA bundle and
#    set REQUESTS_CA_BUNDLE to its path, so reuse that.
if ($env:REQUESTS_CA_BUNDLE) {
    $env:CONDA_SSL_VERIFY = $env:REQUESTS_CA_BUNDLE
    Write-Host "CONDA_SSL_VERIFY = $env:CONDA_SSL_VERIFY"
}

# 3) Create an environment with numpy from conda-forge ONLY. Every repodata and
#    .conda package fetch (numpy + python + libblas/openblas + deps) flows
#    through the relay; conda hash-verifies each package against the exact bytes
#    the relay ledgered, so the MITM must be byte-exact end to end.
Write-Host 'creating env "bw" with numpy from conda-forge...'
& $conda create -y -n bw --override-channels -c conda-forge numpy
if ($LASTEXITCODE -ne 0) { throw "conda create failed ($LASTEXITCODE)" }

# 4) Prove it actually works: import numpy in the new environment.
Write-Host 'verifying numpy import...'
& $conda run -n bw python -c "import numpy; print('numpy', numpy.__version__, 'OK')"
if ($LASTEXITCODE -ne 0) { throw "numpy import failed ($LASTEXITCODE)" }

Write-Host '=== conda-forge validation build: SUCCESS ==='
exit 0
