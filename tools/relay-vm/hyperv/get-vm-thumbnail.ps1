#requires -Version 5.1
<#
  get-vm-thumbnail.ps1 — capture the live console frame of a running Hyper-V VM
  as a PNG, via the WMI GetVirtualSystemThumbnailImage API. This is the
  non-blocking replacement for serial-console capture (which deadlocks the
  unattended shell): it reads the emulated video framebuffer, so it shows
  blank-logo vs kernel-text without VMConnect and without a pipe read loop.
#>
[CmdletBinding()]
param(
  [string]$VmName = "warden-boot-test",
  [int]$Width  = 640,
  [int]$Height = 480,
  [string]$OutPng = "C:\Users\jeff\buildwarden\tools\relay-vm\hyperv\output\console.png"
)
$ErrorActionPreference = "Stop"
$ns = "root\virtualization\v2"

$vmms = Get-CimInstance -Namespace $ns -ClassName Msvm_VirtualSystemManagementService
$vm   = Get-CimInstance -Namespace $ns -ClassName Msvm_ComputerSystem -Filter "ElementName='$VmName'"
if (-not $vm) { throw "VM not found: $VmName" }

# Realized settings instance is the target for a live thumbnail.
$settings = Get-CimAssociatedInstance -InputObject $vm -Namespace $ns -ResultClassName Msvm_VirtualSystemSettingData
$sd = $settings | Where-Object { $_.VirtualSystemType -like '*Realized*' } | Select-Object -First 1
if (-not $sd) { $sd = $settings | Select-Object -First 1 }

$res = Invoke-CimMethod -InputObject $vmms -MethodName GetVirtualSystemThumbnailImage -Arguments @{
  HeightPixels = [uint16]$Height
  WidthPixels  = [uint16]$Width
  TargetSystem = $sd
}
if ($res.ReturnValue -ne 0) { throw "GetVirtualSystemThumbnailImage failed: ReturnValue=$($res.ReturnValue)" }
$data = $res.ImageData
if (-not $data -or $data.Length -lt ($Width * $Height * 2)) {
  throw "Thumbnail data too small: got $($data.Length) bytes, expected $($Width*$Height*2)"
}

# RGB565 (2 bytes/pixel, little-endian) -> 24bpp PNG via LockBits (fast).
Add-Type -AssemblyName System.Drawing
$bmp = New-Object System.Drawing.Bitmap($Width, $Height, [System.Drawing.Imaging.PixelFormat]::Format24bppRgb)
$rect = New-Object System.Drawing.Rectangle(0, 0, $Width, $Height)
$bd = $bmp.LockBits($rect, [System.Drawing.Imaging.ImageLockMode]::WriteOnly, [System.Drawing.Imaging.PixelFormat]::Format24bppRgb)
$stride = $bd.Stride
$buf = New-Object byte[] ($stride * $Height)
$nonBlack = 0
for ($y = 0; $y -lt $Height; $y++) {
  $row = $y * $stride
  $src = $y * $Width * 2
  for ($x = 0; $x -lt $Width; $x++) {
    $i = $src + $x * 2
    $p = ([int]$data[$i]) -bor ([int]$data[$i + 1] -shl 8)
    $r = (($p -shr 11) -band 0x1F) -shl 3
    $g = (($p -shr 5)  -band 0x3F) -shl 2
    $b = ( $p          -band 0x1F) -shl 3
    $o = $row + $x * 3
    $buf[$o]     = [byte]$b
    $buf[$o + 1] = [byte]$g
    $buf[$o + 2] = [byte]$r
    if (($r + $g + $b) -gt 40) { $nonBlack++ }
  }
}
[System.Runtime.InteropServices.Marshal]::Copy($buf, 0, $bd.Scan0, $buf.Length)
$bmp.UnlockBits($bd)
$bmp.Save($OutPng, [System.Drawing.Imaging.ImageFormat]::Png)
$bmp.Dispose()

$pct = [math]::Round(100.0 * $nonBlack / ($Width * $Height), 1)
Write-Host ("Saved {0} ({1}x{2}); non-black pixels: {3}%" -f $OutPng, $Width, $Height, $pct)
