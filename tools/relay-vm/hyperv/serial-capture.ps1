#requires -Version 5.1
<#
  serial-capture.ps1 — capture a VM's COM1 serial console to a file WITHOUT
  deadlocking the unattended shell. The blocking pipe read runs inside a
  background Job that is force-killed by an outer Wait-Job -Timeout + Stop-Job,
  so the main shell always returns. Our UKI cmdline sets console=ttyS0, so the
  kernel prints here even when the video framebuffer is absent — this is the
  video-independent boot signal (and it captures panics).
#>
[CmdletBinding()]
param(
  [string]$VmName   = "warden-boot-test",
  [string]$PipeName = "wardenboot",
  [string]$OutFile  = "C:\Users\jeff\buildwarden\tools\relay-vm\hyperv\output\serial.log",
  [int]$Seconds     = 30
)
$ErrorActionPreference = "Stop"
if (Test-Path $OutFile)        { Remove-Item $OutFile -Force }
if (Test-Path "$OutFile.err")  { Remove-Item "$OutFile.err" -Force }

# Ensure VM is off, wire COM1 to a named pipe, then boot.
Stop-VM -Name $VmName -TurnOff -Force -ErrorAction SilentlyContinue
Set-VMComPort -VMName $VmName -Number 1 -Path "\\.\pipe\$PipeName"
Start-VM -Name $VmName

$job = Start-Job -ScriptBlock {
  param($pipeName, $outFile, $seconds)
  $fs = $null; $pipe = $null
  try {
    $pipe = New-Object System.IO.Pipes.NamedPipeClientStream('.', $pipeName, [System.IO.Pipes.PipeDirection]::In)
    $pipe.Connect(15000)
    $fs = [System.IO.File]::Open($outFile, 'Create', 'Write')
    $buf = New-Object byte[] 4096
    $sw = [System.Diagnostics.Stopwatch]::StartNew()
    while ($sw.Elapsed.TotalSeconds -lt $seconds) {
      $n = $pipe.Read($buf, 0, $buf.Length)   # may block; outer Stop-Job kills us
      if ($n -gt 0) { $fs.Write($buf, 0, $n); $fs.Flush() }
      elseif ($n -eq 0) { Start-Sleep -Milliseconds 100 }
    }
  } catch {
    $_ | Out-String | Set-Content "$outFile.err"
  } finally {
    if ($fs)   { $fs.Close() }
    if ($pipe) { $pipe.Dispose() }
  }
} -ArgumentList $PipeName, $OutFile, $Seconds

# Hard bound: the main shell NEVER blocks longer than this.
Wait-Job $job -Timeout ($Seconds + 12) | Out-Null
Stop-Job $job -ErrorAction SilentlyContinue
Remove-Job $job -Force -ErrorAction SilentlyContinue

Stop-VM -Name $VmName -TurnOff -Force -ErrorAction SilentlyContinue

$len = 0
if (Test-Path $OutFile) { $len = (Get-Item $OutFile).Length }
Write-Host ("serial bytes captured: {0}" -f $len)
if (Test-Path "$OutFile.err") { Write-Host "reader error:"; Get-Content "$OutFile.err" }
