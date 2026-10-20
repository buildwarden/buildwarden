#requires -Version 5.1
# Create-only Gen2 boot VM for manual VMConnect verification.
# Deliberately does NOT read the serial console (that read loop deadlocked the
# non-interactive harness twice). It creates + configures the VM and leaves it
# OFF so a human starts it in Hyper-V Manager and watches POST live.
[CmdletBinding()]
param(
  [string]$VmName   = "warden-boot-test",
  [string]$VhdxPath = "C:\Users\jeff\buildwarden\tools\relay-vm\hyperv\output\warden-relay-boot.vhdx",
  [int]$MemoryMB    = 512
)
$ErrorActionPreference = "Stop"

if (-not (Test-Path -LiteralPath $VhdxPath)) {
  throw "Boot VHDX not found: $VhdxPath"
}

# Remove any leftover VM of the same name (Off it first if running).
$existing = Get-VM -Name $VmName -ErrorAction SilentlyContinue
if ($existing) {
  if ($existing.State -ne 'Off') { Stop-VM -Name $VmName -TurnOff -Force -ErrorAction SilentlyContinue }
  Remove-VM -Name $VmName -Force
  Write-Host "Removed pre-existing VM '$VmName'."
}

# Gen2, no network (isolation), boot straight off the VHDX.
$vm = New-VM -Name $VmName -Generation 2 -MemoryStartupBytes ($MemoryMB * 1MB) -NoVHD
Set-VM -Name $VmName -AutomaticCheckpointsEnabled $false -CheckpointType Disabled
Set-VMProcessor -VMName $VmName -Count 2

# Attach the boot VHDX read-only-safe (it's the static UKI disk).
Add-VMHardDiskDrive -VMName $VmName -Path $VhdxPath
$drive = Get-VMHardDiskDrive -VMName $VmName

# Secure Boot OFF (our UKI is not MS-signed) and boot from the drive.
Set-VMFirmware -VMName $VmName -EnableSecureBoot Off -FirstBootDevice $drive

# No NIC — remove the default one for a clean isolated boot test.
Get-VMNetworkAdapter -VMName $VmName | Remove-VMNetworkAdapter -ErrorAction SilentlyContinue

# Summary
$fw = Get-VMFirmware -VMName $VmName
Write-Host "=== VM READY (Off) ==="
Write-Host ("Name        : {0}" -f $vm.Name)
Write-Host ("Generation  : {0}" -f $vm.Generation)
Write-Host ("Memory      : {0} MB" -f $MemoryMB)
Write-Host ("SecureBoot  : {0}" -f $fw.SecureBoot)
Write-Host ("BootDisk    : {0}" -f $drive.Path)
Write-Host ("State       : {0}" -f (Get-VM -Name $VmName).State)
