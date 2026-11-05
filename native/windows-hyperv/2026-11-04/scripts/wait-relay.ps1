# Wait until a warden-relay-* VM has been running for at least -Uptime seconds
# (the Windows guest is then booted and in its script), list the relay VM's
# adapters and the host's vEthernet addresses, then return so the DHCP probe
# runs while the relay is live.
param([int]$Uptime = 75, [int]$MaxSecs = 400)
Import-Module Hyper-V -ErrorAction SilentlyContinue
$d = (Get-Date).AddSeconds($MaxSecs)
$relay = $null
while ((Get-Date) -lt $d) {
  $relay = Get-VM -ErrorAction SilentlyContinue | Where-Object { $_.Name -like 'warden-relay-*' -and $_.State -eq 'Running' -and $_.Uptime.TotalSeconds -ge $Uptime } | Select-Object -First 1
  if ($relay) { break }
  Start-Sleep -Seconds 2
}
"now $(Get-Date -Format o)"
if (-not $relay) { throw "no warden-relay-* VM reached ${Uptime}s uptime within ${MaxSecs}s" }
Get-VM | Where-Object { $_.Name -like 'warden-*' } | Select-Object Name,State,Uptime | Format-Table -AutoSize | Out-String
Get-VMNetworkAdapter -VMName $relay.Name | Select-Object VMName,SwitchName,MacAddress,IPAddresses | Format-Table -AutoSize | Out-String
Get-VMSwitch | Select-Object Name,SwitchType | Format-Table -AutoSize | Out-String
Get-NetIPAddress -AddressFamily IPv4 | Where-Object { $_.InterfaceAlias -like 'vEthernet*' } | Select-Object InterfaceAlias,IPAddress,PrefixLength | Format-Table -AutoSize | Out-String
