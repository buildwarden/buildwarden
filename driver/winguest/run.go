package winguest

import "fmt"

// WardenRunPS1 is the bootstrap the warden-run startup task runs off the seed
// volume. It copies warden-io.exe local, runs `warden-io initialize` against
// the build link, and powers off so the host sees completion.
//
// Progress and all warden-io output go to C:\warden\warden-run.log and, best
// effort, to COM1. Hyper-V wires COM1 to a host named pipe, and qemu to its
// serial chardev, so a failed boot is observable from the host without
// mounting the guest disk. A missing or busy COM1 never breaks the build.
func WardenRunPS1() string {
	return fmt.Sprintf(`# warden-run.ps1 - Windows build-guest bootstrap.
# Run as SYSTEM by the warden-run startup task from the WARDEN seed volume.
$ErrorActionPreference = 'Continue'
$seed = $PSScriptRoot
$log = 'C:\warden\warden-run.log'
New-Item -ItemType Directory -Force -Path 'C:\warden' | Out-Null

$com = $null
try {
  $com = New-Object System.IO.Ports.SerialPort 'COM1',115200
  $com.Open()
} catch { $com = $null }
function Out-Line($t) {
  Add-Content -Path $log -Value $t
  if ($com) { try { $com.WriteLine($t) } catch {} }
}
function Emit($m) { Out-Line "[warden-run] $(Get-Date -Format o) $m" }

Emit 'start'
try {
  Copy-Item -Force (Join-Path $seed 'warden-io.exe') 'C:\warden\warden-io.exe'
  Emit 'warden-io.exe staged; running initialize --gateway=%[1]s --ip=%[2]s'
  & 'C:\warden\warden-io.exe' initialize --gateway=%[1]s --ip=%[2]s 2>&1 |
    ForEach-Object { Out-Line ([string]$_) }
  Emit "warden-io exit=$LASTEXITCODE"
} catch {
  Emit "bootstrap error: $_"
}
Emit 'powering off'
if ($com) { try { $com.Close() } catch {} }
Stop-Computer -Force
`, BuildGatewayIP, BuildGuestCIDR)
}
