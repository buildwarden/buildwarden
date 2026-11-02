# Pre-fetch the Alpine x86_64 artifacts the offline Hyper-V relay build consumes.
#
# Run this on the HOST (Windows PowerShell) because the WSL environment used to
# assemble the initramfs has no network. Then run the build from WSL:
#     wsl -d Ubuntu bash -lc 'cd tools/relay-vm/hyperv && sh build.sh'
#
# The Alpine release and minirootfs are pinned; bump them together when moving to
# a new release. Package versions are resolved from the branch's APKINDEX, because
# main/ keeps only each package's latest build and a pinned .apk soon 404s.
# Usage:  pwsh tools/relay-vm/hyperv/fetch-cache.ps1

$ErrorActionPreference = 'Stop'
$cache = Join-Path $PSScriptRoot '.cache'
New-Item -ItemType Directory -Force -Path $cache | Out-Null

$alpine   = '3.21'
$rootfsV  = '3.21.7'       # alpine-minirootfs

$main = "https://dl-cdn.alpinelinux.org/alpine/v$alpine/main/x86_64"
$rel  = "https://dl-cdn.alpinelinux.org/alpine/v$alpine/releases/x86_64"

$index = Join-Path $cache 'APKINDEX.tar.gz'
Invoke-WebRequest "$main/APKINDEX.tar.gz" -OutFile $index -UseBasicParsing
$versions = @{}
$pkg = $null
# System32 bsdtar by path: a Git Bash PATH puts GNU tar first, which reads C: as a host.
$tar = Join-Path $env:SystemRoot 'System32\tar.exe'
foreach ($line in (& $tar -xzf $index -O APKINDEX)) {
    if ($line.StartsWith('P:')) { $pkg = $line.Substring(2) }
    elseif ($line.StartsWith('V:') -and $pkg) { $versions[$pkg] = $line.Substring(2) }
}
if ($LASTEXITCODE -ne 0) { throw "could not read APKINDEX from $index" }

$dl = [ordered]@{ 'alpine-minirootfs.tar.gz' = "$rel/alpine-minirootfs-$rootfsV-x86_64.tar.gz" }
# iptables + iptables-legacy + libxtables + libip4tc + libip6tc, libmnl, libnftnl
foreach ($p in 'linux-virt', 'iptables', 'iptables-legacy', 'libxtables', 'libip4tc',
               'libip6tc', 'libmnl', 'libnftnl') {
    if (-not $versions.ContainsKey($p)) { throw "$p is not in the Alpine $alpine APKINDEX" }
    $dl["$p.apk"] = "$main/$p-$($versions[$p]).apk"
}

foreach ($name in $dl.Keys) {
    $out = Join-Path $cache $name
    Invoke-WebRequest $dl[$name] -OutFile $out -UseBasicParsing
    '{0,-24} {1,12:N0} bytes  {2}' -f $name, (Get-Item $out).Length, ($dl[$name] -split '/')[-1]
}
Write-Host "`nCache ready: $cache"
Write-Host "Next: wsl -d Ubuntu bash -lc 'cd tools/relay-vm/hyperv && sh build.sh'"
