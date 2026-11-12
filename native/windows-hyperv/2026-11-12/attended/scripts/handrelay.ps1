param([string]$Name, [string]$SeedISO = '', [string]$BuildSwitch, [string]$Notes, [string]$WorkDir)
$ErrorActionPreference = 'Stop'
$boot = "$env:LOCALAPPDATA\warden\hyperv\relay-boot.vhdx"
New-Item -ItemType Directory -Force $WorkDir | Out-Null
$ov = Join-Path $WorkDir "$Name.vhdx"
New-VHD -Path $ov -ParentPath $boot -Differencing | Out-Null
New-VM -Name $Name -Generation 2 -MemoryStartupBytes 512MB -VHDPath $ov -SwitchName $BuildSwitch | Out-Null
Set-VM -Name $Name -AutomaticCheckpointsEnabled $false -CheckpointType Disabled
Set-VMProcessor -VMName $Name -Count 2
Add-VMNetworkAdapter -VMName $Name -SwitchName 'warden-dev-nat'
$drive = Get-VMHardDiskDrive -VMName $Name
Set-VMFirmware -VMName $Name -EnableSecureBoot Off -FirstBootDevice $drive
Set-VMComPort -VMName $Name -Number 1 -Path ("\\.\pipe\$Name")
if ($SeedISO) { Add-VMDvdDrive -VMName $Name -Path $SeedISO }
Set-VM -Name $Name -Notes $Notes
'OK'
