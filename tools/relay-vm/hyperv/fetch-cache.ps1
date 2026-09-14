# Pre-fetch the Alpine x86_64 artifacts the offline Hyper-V relay build consumes.
#
# Run this on the HOST (Windows PowerShell) because the WSL environment used to
# assemble the initramfs has no network. Then run the build from WSL:
#     wsl -d Ubuntu bash -lc 'cd tools/relay-vm/hyperv && sh build.sh'
#
# Versions are pinned; bump them together when moving to a new Alpine release.
# Usage:  pwsh tools/relay-vm/hyperv/fetch-cache.ps1

$ErrorActionPreference = 'Stop'
$cache = Join-Path $PSScriptRoot '.cache'
New-Item -ItemType Directory -Force -Path $cache | Out-Null

$alpine   = '3.21'
$rootfsV  = '3.21.7'       # alpine-minirootfs
$kernelV  = '6.12.109-r0'  # linux-virt
$iptV     = '1.8.11-r1'    # iptables + iptables-legacy + libxtables + libip4tc + libip6tc
$mnlV     = '1.0.5-r2'     # libmnl
$nftnlV   = '1.2.8-r0'     # libnftnl

$main = "https://dl-cdn.alpinelinux.org/alpine/v$alpine/main/x86_64"
$rel  = "https://dl-cdn.alpinelinux.org/alpine/v$alpine/releases/x86_64"

$dl = [ordered]@{
    'alpine-minirootfs.tar.gz' = "$rel/alpine-minirootfs-$rootfsV-x86_64.tar.gz"
    'linux-virt.apk'           = "$main/linux-virt-$kernelV.apk"
    'iptables.apk'             = "$main/iptables-$iptV.apk"
    'iptables-legacy.apk'      = "$main/iptables-legacy-$iptV.apk"
    'libxtables.apk'           = "$main/libxtables-$iptV.apk"
    'libip4tc.apk'             = "$main/libip4tc-$iptV.apk"
    'libip6tc.apk'             = "$main/libip6tc-$iptV.apk"
    'libmnl.apk'               = "$main/libmnl-$mnlV.apk"
    'libnftnl.apk'             = "$main/libnftnl-$nftnlV.apk"
}

foreach ($name in $dl.Keys) {
    $out = Join-Path $cache $name
    Invoke-WebRequest $dl[$name] -OutFile $out -UseBasicParsing
    '{0,-24} {1,12:N0} bytes' -f $name, (Get-Item $out).Length
}
Write-Host "`nCache ready: $cache"
Write-Host "Next: wsl -d Ubuntu bash -lc 'cd tools/relay-vm/hyperv && sh build.sh'"
