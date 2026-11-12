$ErrorActionPreference = 'Stop'
Resolve-DnsName example.com | Select-Object -First 1 | Out-String | Write-Output
(Invoke-WebRequest -UseBasicParsing https://example.com).StatusCode
Get-NetIPConfiguration | Out-String | Write-Output
Get-NetIPAddress -AddressFamily IPv4 | Format-Table IPAddress,PrefixOrigin,InterfaceAlias | Out-String | Write-Output
Write-Output '--- warden-run.log'
Get-Content C:\warden\warden-run.log | Write-Output
