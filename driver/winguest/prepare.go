package winguest

import "fmt"

// Prepare-time contract. A prepare boots a generalized image with no network
// adapter and a seed labelled PrepareSeedLabel carrying the OOBE answer file
// and PrepareScriptName. OOBE's one FirstLogonCommand runs the script, which
// tunes the image, registers the warden-run startup task, reports the eval
// grace clock, and powers off. The image it leaves is the prepared layer every
// build boots from.
//
// Progress goes to C:\warden\warden-prepare.log and, as guest key-value items,
// to the host: on Hyper-V under HKLM\SOFTWARE\Microsoft\Virtual Machine\Guest,
// which the Data Exchange integration service publishes (readable by Hyper-V
// Administrators over WMI, and unaffected by guest reboots). Other drivers
// read the log or their own channel.
const (
	// PrepareSeedLabel differs from SeedLabel so the warden-run task, once
	// registered, never mistakes the prepare seed for a build seed.
	PrepareSeedLabel  = "WPREP"
	PrepareScriptName = "warden-prepare.ps1"

	// KVPState is "running:<step>", then "done", or "error:<message>".
	KVPState = "warden-prepare-state"
	// KVPGraceMinutes, KVPLicenseStatus and KVPLicenseChannel describe the
	// Windows license at the end of prepare; KVPReportedAt is the guest's UTC
	// time of that reading (RFC 3339), so the host can turn the minutes into
	// an absolute grace end.
	KVPGraceMinutes   = "warden-grace-minutes"
	KVPLicenseStatus  = "warden-license-status"
	KVPLicenseChannel = "warden-license-channel"
	KVPReportedAt     = "warden-reported-at"

	StateDone        = "done"
	StateRunningPref = "running:"
	StateErrorPref   = "error:"
)

// PrepareScript is warden-prepare.ps1. Tuning steps are best effort: a step
// that fails is logged and prepare continues, because each only removes
// background work from build boots. Registering the warden-run task is not
// best effort: if it fails the prepare reports an error.
//
// It runs in two stages. "logon" (the FirstLogonCommand, as Administrator)
// applies the tuning; if that leaves a reboot pending (removing Defender does)
// it registers a one-shot SYSTEM startup task for "finish" and restarts.
// "finish" runs the .NET ngen queue, registers warden-run, reports the license
// clock, signals done and powers off.
func PrepareScript() string {
	return fmt.Sprintf(prepareScriptTemplate, StartupTaskCommand(SeedLabel))
}

const prepareScriptTemplate = `# warden-prepare.ps1 - prepare a generalized Windows image once.
param([string]$Stage = 'logon')
$ErrorActionPreference = 'Continue'
$dir = 'C:\warden'
New-Item -ItemType Directory -Force -Path $dir | Out-Null
$log = Join-Path $dir 'warden-prepare.log'
$kvp = 'HKLM:\SOFTWARE\Microsoft\Virtual Machine\Guest'

function Report($name, $value) {
  Add-Content -Path $log -Value "$(Get-Date -Format o) $name=$value"
  try {
    # Not New-Item -Force on an existing key: that recreates it, dropping
    # every value reported so far.
    if (-not (Test-Path $kvp)) { New-Item -Path $kvp -ErrorAction Stop | Out-Null }
    New-ItemProperty -Path $kvp -Name $name -Value ([string]$value) ` + "`" + `
      -PropertyType String -Force -ErrorAction Stop | Out-Null
  } catch { Add-Content -Path $log -Value "kvp write failed: $_" }
  Write-SeedStatus
}
# Mirror every reported item onto a writable prepare seed (qemu's FAT disk):
# the host has no KVP there and reads the newest block out of the raw image
# after the VM powers off. A read-only seed (Hyper-V's ISO) is skipped.
function Write-SeedStatus {
  try {
    $v = Get-Volume -FileSystemLabel 'WPREP' -ErrorAction Stop |
      Where-Object DriveLetter | Select-Object -First 1
    if (-not $v) { return }
    $p = Get-ItemProperty -Path $kvp -ErrorAction Stop
    $lines = @("WARDEN-PREPARE-STATUS seq=$([DateTime]::UtcNow.Ticks)")
    foreach ($n in ($p.PSObject.Properties.Name | Where-Object { $_ -like 'warden-*' })) {
      $lines += "$n=$($p.$n)"
    }
    $lines += 'END-WARDEN-PREPARE-STATUS'
    $text = ($lines -join [char]10) + [char]10
    [IO.File]::WriteAllText("$($v.DriveLetter):\warden-prepare-status.txt", $text)
  } catch {}
}
function Step($name, [scriptblock]$body) {
  Report 'warden-prepare-state' "running:$name"
  try { & $body *>> $log } catch { Add-Content -Path $log -Value "step $name failed: $_" }
}
function Fail($msg) {
  Report 'warden-prepare-state' "error:$msg"
  Start-Sleep 20
  Stop-Computer -Force
  exit 1
}

function Finish {
  Unregister-ScheduledTask -TaskName 'warden-prepare-continue' -Confirm:$false ` + "`" + `
    -ErrorAction SilentlyContinue
  Step 'ngen' {
    foreach ($fw in 'Framework64', 'Framework') {
      $ngen = Join-Path $env:windir "Microsoft.NET\$fw\v4.0.30319\ngen.exe"
      if (Test-Path $ngen) { & $ngen executeQueuedItems /nologo }
    }
  }
  Report 'warden-prepare-state' 'running:register-warden-run'
  try {
    %s
  } catch { Fail "register warden-run: $_" }
  if (-not (Get-ScheduledTask -TaskName 'warden-run' -ErrorAction SilentlyContinue)) {
    Fail 'warden-run task missing after registration'
  }
  $lic = Get-CimInstance SoftwareLicensingProduct ` + "`" + `
    -Filter ("PartialProductKey IS NOT NULL AND " +
             "ApplicationID='55c92734-d682-4d71-983e-d6ec3f16059f'") |
    Select-Object -First 1
  if ($lic) {
    Report 'warden-grace-minutes' $lic.GracePeriodRemaining
    Report 'warden-license-status' $lic.LicenseStatus
    Report 'warden-license-channel' $lic.Description
  }
  Report 'warden-reported-at' ((Get-Date).ToUniversalTime().ToString('o'))
  Report 'warden-prepare-state' 'done'
  # Give the host time to read the final state, then leave no warden items in
  # the image (every build VM would otherwise publish them).
  Start-Sleep 30
  Remove-ItemProperty -Path $kvp -Name 'warden-*' -ErrorAction SilentlyContinue
  Stop-Computer -Force
  exit 0
}

if ($Stage -eq 'finish') { Finish }

Copy-Item -Force $PSCommandPath (Join-Path $dir 'warden-prepare.ps1')

Step 'services' {
  foreach ($s in 'wuauserv', 'UsoSvc', 'DiagTrack', 'SysMain', 'dmwappushservice',
                 'MapsBroker', 'WSearch') {
    Stop-Service -Name $s -Force -ErrorAction SilentlyContinue
    Set-Service -Name $s -StartupType Disabled -ErrorAction SilentlyContinue
  }
}
Step 'scheduled-tasks' {
  foreach ($p in '\Microsoft\Windows\Defrag\', '\Microsoft\Windows\Server Manager\',
                 '\Microsoft\Windows\WindowsUpdate\', '\Microsoft\Windows\UpdateOrchestrator\',
                 '\Microsoft\Windows\Application Experience\',
                 '\Microsoft\Windows\Customer Experience Improvement Program\',
                 '\Microsoft\Windows\Diagnosis\',
                 '\Microsoft\Windows\SoftwareProtectionPlatform\') {
    Get-ScheduledTask -TaskPath $p -ErrorAction SilentlyContinue |
      Disable-ScheduledTask -ErrorAction SilentlyContinue | Out-Null
  }
}
Step 'policy' {
  $nt = 'HKLM\SOFTWARE\Microsoft\Windows NT\CurrentVersion'
  reg add "$nt\Schedule\Maintenance" /v MaintenanceDisabled /t REG_DWORD /d 1 /f
  reg add "$nt\SoftwareProtectionPlatform\Activation" /v Manual /t REG_DWORD /d 1 /f
  reg add 'HKLM\SOFTWARE\Microsoft\ServerManager' /v DoNotOpenServerManagerAtLogon ` + "`" + `
    /t REG_DWORD /d 1 /f
  reg add 'HKLM\SOFTWARE\Policies\Microsoft\Windows\WindowsUpdate\AU' ` + "`" + `
    /v NoAutoUpdate /t REG_DWORD /d 1 /f
}
# Background traffic every build would otherwise send through the relay (and
# record in its ledger): the connectivity probe, root/disallowed certificate
# list updates, and Store-style license sync.
Step 'background-network' {
  reg add 'HKLM\SOFTWARE\Policies\Microsoft\Windows\NetworkConnectivityStatusIndicator' ` + "`" + `
    /v NoActiveProbe /t REG_DWORD /d 1 /f
  $certs = 'HKLM\SOFTWARE\Policies\Microsoft\SystemCertificates\AuthRoot'
  reg add $certs /v DisableRootAutoUpdate /t REG_DWORD /d 1 /f
  reg add $certs /v EnableDisallowedCertAutoUpdate /t REG_DWORD /d 0 /f
  foreach ($s in 'ClipSVC', 'LicenseManager') {
    reg add "HKLM\SYSTEM\CurrentControlSet\Services\$s" /v Start /t REG_DWORD /d 4 /f
  }
}
Step 'power' {
  powercfg /setactive 8c5e7fda-e8bf-4a96-9a85-a6e23a8c635c
  powercfg /hibernate off
}
$restart = $false
Step 'defender' {
  if (Get-Command Uninstall-WindowsFeature -ErrorAction SilentlyContinue) {
    $r = Uninstall-WindowsFeature -Name Windows-Defender
    if ("$($r.RestartNeeded)" -ne 'No') { $script:restart = $true }
  }
}

if ($restart) {
  try {
    $a = New-ScheduledTaskAction -Execute 'powershell.exe' ` + "`" + `
      -Argument "-NoProfile -ExecutionPolicy Bypass -File $dir\warden-prepare.ps1 -Stage finish"
    $t = New-ScheduledTaskTrigger -AtStartup
    $s = New-ScheduledTaskSettingsSet -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries
    Register-ScheduledTask -TaskName 'warden-prepare-continue' -Action $a -Trigger $t ` + "`" + `
      -Settings $s -User SYSTEM -RunLevel Highest -Force -ErrorAction Stop | Out-Null
  } catch { Fail "register continue task: $_" }
  Report 'warden-prepare-state' 'running:reboot'
  Restart-Computer -Force
  exit 0
}
Finish
`
