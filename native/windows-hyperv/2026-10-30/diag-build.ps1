$ErrorActionPreference = 'Continue'
$ProgressPreference = 'SilentlyContinue'
"== env"
Get-ChildItem env: | ? { $_.Name -match 'proxy|ssl|cert|warden' } | % { "$($_.Name)=$($_.Value)" }
netsh winhttp show proxy
Get-NetIPConfiguration | % { "$($_.InterfaceAlias) $($_.IPv4Address.IPAddress) gw=$($_.IPv4DefaultGateway.NextHop) dns=$($_.DNSServer.ServerAddresses)" }
Get-NetIPInterface -AddressFamily IPv4 | % { "mtu $($_.InterfaceAlias)=$($_.NlMtu)" }
"== root store"
Get-ChildItem Cert:\LocalMachine\Root | ? { $_.Subject -match 'warden|relay' -or $_.NotBefore -gt (Get-Date).AddDays(-1) } | % { "$($_.Subject) $($_.Thumbprint)" }
"root count: $((Get-ChildItem Cert:\LocalMachine\Root).Count)"
foreach ($a in @('-sS', '-sS --ssl-no-revoke')) {
    "== curl $a https"
    $t = Get-Date
    Invoke-Expression "curl.exe $a -v --max-time 30 -o c.out https://pypi.org/simple/six/ 2>&1" | Select-Object -Last 25
    "exit=$LASTEXITCODE secs=$([int]((Get-Date)-$t).TotalSeconds)"
}
"== curl http"
curl.exe -sS -v --max-time 30 -o h.out http://pypi.org/simple/six/ 2>&1 | Select-Object -Last 20
"exit=$LASTEXITCODE"
"hello" | Set-Content hello.txt
