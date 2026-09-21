#requires -Version 5.1
<#
  build-boot-vhdx-native.ps1  (RUN ELEVATED — Administrator required)

  Builds a Hyper-V Gen2-bootable VHDX the Windows-native way: create VHDX,
  Mount-VHD, GPT-init, one ESP (FAT32) partition, copy the UKI to
  \EFI\BOOT\BOOTX64.EFI, dismount. This is the reliable path Mount-VHD needs
  full Administrator for, and it is the exact tooling an admin CI runner uses.
  It exists to isolate the qemu-img VHDX (which Hyper-V would not boot) from the
  boot IMAGE itself: if THIS VHDX boots, qemu-img was the sole culprit.
#>
[CmdletBinding()]
param(
  [string]$UkiPath = "C:\Users\jeff\buildwarden\tools\relay-vm\hyperv\output\warden-relay-boot.efi",
  [string]$OutVhdx = "C:\Users\jeff\buildwarden\tools\relay-vm\hyperv\output\warden-relay-boot-native.vhdx",
  [int]$SizeMB     = 256
)
$ErrorActionPreference = "Stop"

# --- elevation gate ---
$isAdmin = ([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()
           ).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
if (-not $isAdmin) {
  throw "This script must be run from an ELEVATED PowerShell (Run as Administrator). Mount-VHD needs full Administrator."
}
if (-not (Test-Path -LiteralPath $UkiPath)) { throw "UKI not found: $UkiPath" }

# ESP partition type GUID (EFI System Partition)
$ESP_GUID = '{c12a7328-f81f-11d2-ba4b-00a0c93ec93b}'

if (Test-Path -LiteralPath $OutVhdx) {
  Write-Host "Removing existing $OutVhdx"
  # ensure it's not still attached
  try { Dismount-VHD -Path $OutVhdx -ErrorAction SilentlyContinue } catch {}
  Remove-Item -LiteralPath $OutVhdx -Force
}

Write-Host "Creating VHDX ($SizeMB MB, dynamic, GPT-ready)..."
New-VHD -Path $OutVhdx -SizeBytes ($SizeMB * 1MB) -Dynamic | Out-Null

$diskNumber = $null
try {
  $disk = Mount-VHD -Path $OutVhdx -Passthru | Get-Disk
  $diskNumber = $disk.Number
  Write-Host "Mounted as disk number $diskNumber"

  # Fresh VHDX is RAW; initialize GPT.
  if ($disk.PartitionStyle -eq 'RAW') {
    Initialize-Disk -Number $diskNumber -PartitionStyle GPT -Confirm:$false | Out-Null
  } else {
    Clear-Disk -Number $diskNumber -RemoveData -RemoveOEM -Confirm:$false
    Initialize-Disk -Number $diskNumber -PartitionStyle GPT -Confirm:$false | Out-Null
  }

  Write-Host "Creating ESP partition..."
  $part = New-Partition -DiskNumber $diskNumber -UseMaximumSize -GptType $ESP_GUID
  Format-Volume -Partition $part -FileSystem FAT32 -NewFileSystemLabel "ESP" -Confirm:$false | Out-Null

  # ESP partitions do not auto-mount a letter — assign one to populate it.
  $part | Add-PartitionAccessPath -AssignDriveLetter
  $drv = (Get-Partition -DiskNumber $diskNumber -PartitionNumber $part.PartitionNumber).DriveLetter
  if (-not $drv) { throw "Failed to assign a drive letter to the ESP." }
  Write-Host "ESP mounted at ${drv}:"

  Write-Host "Copying UKI -> \EFI\BOOT\BOOTX64.EFI ..."
  New-Item -ItemType Directory -Path "${drv}:\EFI\BOOT" -Force | Out-Null
  Copy-Item -LiteralPath $UkiPath -Destination "${drv}:\EFI\BOOT\BOOTX64.EFI" -Force

  # verify
  $dst = "${drv}:\EFI\BOOT\BOOTX64.EFI"
  if (-not (Test-Path -LiteralPath $dst)) { throw "Copy verification failed: $dst missing." }
  $sz = (Get-Item -LiteralPath $dst).Length
  Write-Host ("Verified BOOTX64.EFI on ESP: {0} bytes" -f $sz)
}
finally {
  if ($diskNumber -ne $null) {
    Write-Host "Dismounting VHDX..."
    Dismount-VHD -Path $OutVhdx -ErrorAction SilentlyContinue
  }
}

Write-Host "=== DONE ==="
Write-Host ("Native boot VHDX: {0}" -f $OutVhdx)
