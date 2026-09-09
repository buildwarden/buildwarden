# build.ps1 - Windows build script run by warden-io inside the build VM.
#
# warden-io fetches this script from the relay (http://cwd/build.ps1), installs
# the per-build CA into the trust store, then runs it via:
#   powershell.exe -NoProfile -ExecutionPolicy Bypass -File build.ps1
# and reports the exit code back to the relay automatically.
#
# This mirrors how conda-forge builds win-64 packages: conda-build drives the
# recipe, and the MSVC toolchain is activated by conda-forge's own
# `vs2022_win-64` compiler package (selected via {{ compiler('cxx') }} in the
# recipe), so we do NOT call vcvarsall.bat by hand here.
#
# Every network input (anaconda.org channel fetches, hash-checked source
# downloads) flows through the relay and lands in the ledger. conda-build
# verifies sha256/md5/sha1 on sources before extraction, so the relay's MITM
# must be byte-exact end to end.

$ErrorActionPreference = 'Stop'

# conda / conda-build come from Miniforge, which is either baked into the base
# image or bootstrapped through the relay (see README - this is the toolchain
# provisioning decision still open in the plan).
conda build recipe --output-folder output

# For a plain (non-conda) MSVC C++ build on an ARM64 Windows host you would
# instead activate the native cross toolset that emits x86-64:
#   & "$vsPath\VC\Auxiliary\Build\vcvarsall.bat" arm64_amd64
