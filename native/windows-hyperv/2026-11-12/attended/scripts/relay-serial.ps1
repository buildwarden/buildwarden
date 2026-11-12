#requires -Version 5.1
# Attach to the next warden-relay-* VM's COM1 pipe once it is past firmware
# (uptime gate; the Alpine relay has no integration services to gate on) and
# tail the relay's console to a file. Pure reader; the VM lifecycle is warden's.
param(
  [string]$OutFile,
  [int]$Seconds = 300,
  [int]$WaitForVMSecs = 180,
  [double]$UptimeGate = 5,
  [string]$VMName = ''
)
$ErrorActionPreference = 'Stop'
Import-Module Hyper-V -ErrorAction SilentlyContinue
if (Test-Path $OutFile) { Remove-Item $OutFile -Force }
if (Test-Path "$OutFile.err") { Remove-Item "$OutFile.err" -Force }
$deadline = (Get-Date).AddSeconds($WaitForVMSecs)
$vm = $null
while (-not $vm -and (Get-Date) -lt $deadline) {
  $vm = Get-VM -ErrorAction SilentlyContinue | Where-Object { ($(if ($VMName) { $_.Name -eq $VMName } else { $_.Name -like 'warden-relay-*' })) -and $_.State -eq 'Running' } | Select-Object -First 1
  if (-not $vm) { Start-Sleep -Milliseconds 300 }
}
if (-not $vm) { throw "no running warden-relay-* VM within ${WaitForVMSecs}s" }
while (((Get-VM -Name $vm.Name).Uptime.TotalSeconds) -lt $UptimeGate) { Start-Sleep -Milliseconds 200 }
$pipeName = $vm.Name
Write-Host "attaching to \\.\pipe\$pipeName at uptime $([math]::Round((Get-VM -Name $vm.Name).Uptime.TotalSeconds,1))s -> $OutFile"
$job = Start-Job -ScriptBlock {
  param($pipeName, $outFile, $seconds)
  $fs = $null; $pipe = $null
  try {
    $pipe = New-Object System.IO.Pipes.NamedPipeClientStream('.', $pipeName, [System.IO.Pipes.PipeDirection]::In)
    $pipe.Connect(30000)
    $fs = [System.IO.File]::Open($outFile, 'Create', 'Write')
    $buf = New-Object byte[] 4096
    $sw = [System.Diagnostics.Stopwatch]::StartNew()
    while ($sw.Elapsed.TotalSeconds -lt $seconds) {
      $n = $pipe.Read($buf, 0, $buf.Length)
      if ($n -gt 0) { $fs.Write($buf, 0, $n); $fs.Flush() }
      elseif ($n -eq 0) { Start-Sleep -Milliseconds 200 }
    }
  } catch {
    $_ | Out-String | Set-Content "$outFile.err"
  } finally {
    if ($fs) { $fs.Close() }
    if ($pipe) { $pipe.Dispose() }
  }
} -ArgumentList $pipeName, $OutFile, $Seconds
Wait-Job $job -Timeout ($Seconds + 12) | Out-Null
Stop-Job $job -ErrorAction SilentlyContinue
Remove-Job $job -Force -ErrorAction SilentlyContinue
$len = 0
if (Test-Path $OutFile) { $len = (Get-Item $OutFile).Length }
Write-Host ("relay serial bytes captured: {0}" -f $len)
if (Test-Path "$OutFile.err") { Write-Host "reader error:"; Get-Content "$OutFile.err" }
