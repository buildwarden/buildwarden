#requires -Version 5.1
<#
  read-guest-log.ps1 — read the Windows build guest's C:\warden\warden-run.log
  off a KEPT build overlay (from a `WARDEN_HYPERV_KEEP_VMS=1` run), for
  post-mortem when the build did not complete.

  MUST RUN ELEVATED: Mount-VHD requires full Administrator. It mounts the overlay
  READ-ONLY (so the disk chain is never mutated), locates the guest's Windows
  volume, prints the warden log(s), then dismounts.

  Pass -Vhdx with the overlay path (the .avhdx checkpoint layer if present, else
  build.vhdx), or let it auto-discover the newest warden-hyperv-* overlay under
  -ScratchRoot.
#>
[CmdletBinding()]
param(
  [string]$Vhdx        = "",
  [string]$ScratchRoot = "$env:KIROCREW_SCRATCH",
  [string]$GuestLog    = "warden\warden-run.log"
)
$ErrorActionPreference = "Stop"

# Require elevation up front with a clear message (Mount-VHD fails cryptically otherwise).
$admin = ([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
if (-not $admin) { throw "This script must run in an ELEVATED PowerShell (Mount-VHD needs Administrator)." }

if (-not $Vhdx) {
  if (-not $ScratchRoot -or -not (Test-Path $ScratchRoot)) {
    throw "No -Vhdx given and ScratchRoot '$ScratchRoot' not found; pass -Vhdx <path to build overlay>."
  }
  # Newest build overlay: prefer the .avhdx checkpoint layer (guest writes land there) if one exists.
  $wd = Get-ChildItem "$ScratchRoot\warden-hyperv-*" -Directory | Sort-Object LastWriteTime -Descending | Select-Object -First 1
  if (-not $wd) { throw "no warden-hyperv-* work dir under $ScratchRoot" }
  $Vhdx = (Get-ChildItem $wd.FullName -Filter '*.avhdx' | Sort-Object LastWriteTime -Descending | Select-Object -First 1).FullName
  if (-not $Vhdx) { $Vhdx = Join-Path $wd.FullName 'build.vhdx' }
  Write-Host "auto-discovered overlay: $Vhdx"
}
if (-not (Test-Path $Vhdx)) { throw "overlay not found: $Vhdx" }

$mounted = $null
try {
  $mounted = Mount-VHD -Path $Vhdx -ReadOnly -PassThru
  Start-Sleep -Seconds 1
  $vols = $mounted | Get-Disk | Get-Partition | Get-Volume | Where-Object { $_.DriveLetter }
  Write-Host ("mounted; volumes: " + (($vols | ForEach-Object { "$($_.DriveLetter): ($($_.FileSystemLabel))" }) -join ', '))
  $found = $false
  foreach ($v in $vols) {
    $p = "$($v.DriveLetter):\$GuestLog"
    if (Test-Path $p) {
      $found = $true
      Write-Host "`n===== $p =====`n"
      Get-Content $p
    }
  }
  if (-not $found) { Write-Host "`n(no $GuestLog on any mounted volume — guest may not have reached first logon)" }
} finally {
  if ($mounted) { Dismount-VHD -Path $Vhdx -ErrorAction SilentlyContinue; Write-Host "`ndismounted." }
}
