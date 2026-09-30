#requires -Version 5.1
<#
  read-serial-pipe.ps1 — attach to a VM's COM1 host named pipe and tail it to a
  file, WITHOUT touching the VM lifecycle. Unlike serial-capture.ps1 (which
  wires + boots the VM itself), this is a pure reader for a VM whose lifecycle
  is owned by something else (e.g. `warden build`, which creates/starts/stops
  the build VM). Hyper-V hosts the pipe SERVER when a running VM has COM1 set to
  \\.\pipe\<name>, so we connect as a client.

  The pipe only exists once the VM is running, and the VM may not exist yet when
  this starts, so we poll for the pipe (or a warden-build-* pipe) before
  connecting. The blocking read runs inside a Job bounded by Wait-Job -Timeout
  so the shell never deadlocks. Live bytes are also echoed to the host console.
#>
[CmdletBinding()]
param(
  # Exact pipe name (without the \\.\pipe\ prefix), e.g. warden-build-abc123.
  # If empty, the script discovers the first pipe matching -DiscoverPrefix.
  [string]$PipeName       = "",
  [string]$DiscoverPrefix = "warden-build-",
  [string]$OutFile        = "$PSScriptRoot\output\build-serial.log",
  [int]$Seconds           = 600,
  [int]$WaitForPipeSecs   = 180,
  # VM to gate on: the reader waits for this VM's integration-services heartbeat
  # before connecting. IMPORTANT: connecting a client to a Gen2 VM's COM1 named
  # pipe DURING the UEFI firmware phase prevents the guest from booting (console
  # redirection hang), so we must not connect until the guest OS is up. Leave
  # empty to skip gating (only safe when the VM is already known to be booted).
  [string]$VMName         = ""
)
$ErrorActionPreference = "Stop"
$outDir = Split-Path -Parent $OutFile
if (-not (Test-Path $outDir)) { New-Item -ItemType Directory -Force -Path $outDir | Out-Null }
if (Test-Path $OutFile)       { Remove-Item $OutFile -Force }
if (Test-Path "$OutFile.err") { Remove-Item "$OutFile.err" -Force }

# Gate on the guest OS being up before connecting (see -VMName note above).
if ($VMName) {
  Import-Module Hyper-V -ErrorAction SilentlyContinue
  $hbDeadline = (Get-Date).AddSeconds($WaitForPipeSecs)
  while ((Get-Date) -lt $hbDeadline) {
    $hb = (Get-VMIntegrationService -VMName $VMName -Name Heartbeat -ErrorAction SilentlyContinue).PrimaryStatusDescription
    if ($hb -like 'OK*') { break }
    Start-Sleep -Seconds 3
  }
  Write-Host "heartbeat gate: VM '$VMName' -> $((Get-VMIntegrationService -VMName $VMName -Name Heartbeat -ErrorAction SilentlyContinue).PrimaryStatusDescription); connecting"
  # Small extra settle so the guest is well past UEFI before the COM client attaches.
  Start-Sleep -Seconds 5
}

function Get-PipeList { [System.IO.Directory]::GetFiles('\\.\pipe\') | ForEach-Object { Split-Path $_ -Leaf } }

# Resolve the pipe name: explicit, else discover by prefix (polling until it appears).
$deadline = (Get-Date).AddSeconds($WaitForPipeSecs)
while (-not $PipeName -and (Get-Date) -lt $deadline) {
  $match = Get-PipeList | Where-Object { $_ -like "$DiscoverPrefix*" } | Select-Object -First 1
  if ($match) { $PipeName = $match; break }
  Start-Sleep -Milliseconds 500
}
if (-not $PipeName) { throw "no pipe matching '$DiscoverPrefix*' appeared within ${WaitForPipeSecs}s" }
Write-Host "attaching to \\.\pipe\$PipeName -> $OutFile (${Seconds}s)"

$job = Start-Job -ScriptBlock {
  param($pipeName, $outFile, $seconds, $waitForPipeSecs)
  $fs = $null; $pipe = $null
  try {
    # ONE long blocking Connect (matches the proven serial-capture.ps1). Do NOT
    # dispose/reconnect in a tight loop: flapping a Hyper-V COM named pipe during
    # early guest boot destabilises the VM. Connect() waits up to the timeout for
    # Hyper-V's pipe server to appear (the VM may still be powering on).
    $pipe = New-Object System.IO.Pipes.NamedPipeClientStream('.', $pipeName, [System.IO.Pipes.PipeDirection]::In)
    $pipe.Connect($waitForPipeSecs * 1000)
    $fs = [System.IO.File]::Open($outFile, 'Create', 'Write')
    $buf = New-Object byte[] 4096
    $sw = [System.Diagnostics.Stopwatch]::StartNew()
    while ($sw.Elapsed.TotalSeconds -lt $seconds) {
      $n = $pipe.Read($buf, 0, $buf.Length)   # may block; outer Stop-Job kills us
      if ($n -gt 0) {
        $fs.Write($buf, 0, $n); $fs.Flush()
        [Console]::Out.Write([System.Text.Encoding]::UTF8.GetString($buf, 0, $n))
      } elseif ($n -eq 0) { Start-Sleep -Milliseconds 200 }  # server gone (VM off) or idle
    }
  } catch {
    $_ | Out-String | Set-Content "$outFile.err"
  } finally {
    if ($fs)   { $fs.Close() }
    if ($pipe) { $pipe.Dispose() }
  }
} -ArgumentList $PipeName, $OutFile, $Seconds, $WaitForPipeSecs

# Hard bound: the main shell NEVER blocks longer than this.
Wait-Job $job -Timeout ($Seconds + 12) | Out-Null
Receive-Job $job -ErrorAction SilentlyContinue | Out-Host
Stop-Job $job -ErrorAction SilentlyContinue
Remove-Job $job -Force -ErrorAction SilentlyContinue

$len = 0
if (Test-Path $OutFile) { $len = (Get-Item $OutFile).Length }
Write-Host ("serial bytes captured: {0}" -f $len)
if (Test-Path "$OutFile.err") { Write-Host "reader error:"; Get-Content "$OutFile.err" }
