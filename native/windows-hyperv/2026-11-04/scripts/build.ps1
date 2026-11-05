$ErrorActionPreference = 'Continue'
$ProgressPreference = 'SilentlyContinue'
"== guest"
[Environment]::OSVersion.VersionString
"== ipconfig /all"
ipconfig /all
"== route print -4"
route print -4
"== Get-NetIPAddress (IPv4)"
Get-NetIPAddress -AddressFamily IPv4 | Select-Object InterfaceAlias,IPAddress,PrefixLength,PrefixOrigin,SuffixOrigin,AddressState | Format-Table -AutoSize | Out-String -Width 200
"== default route"
Get-NetRoute -AddressFamily IPv4 -DestinationPrefix 0.0.0.0/0 | Select-Object InterfaceAlias,NextHop,RouteMetric | Format-Table -AutoSize | Out-String
"== Dhcp-Client admin log (last 10)"
Get-WinEvent -LogName 'Microsoft-Windows-Dhcp-Client/Admin' -MaxEvents 10 -ErrorAction SilentlyContinue | Select-Object TimeCreated,Id,Message | Format-List | Out-String -Width 200
"== warden-run.log"
Get-Content C:\warden\warden-run.log
$gw = (Get-NetRoute -AddressFamily IPv4 -DestinationPrefix 0.0.0.0/0 | Select-Object -First 1).NextHop
"gateway=$gw"
"== https via Invoke-WebRequest"
try { "status=$((Invoke-WebRequest -UseBasicParsing https://example.com/).StatusCode)" } catch { "failed: $($_.Exception.Message)" }
"== https via curl.exe (no --ssl-no-revoke)"
curl.exe -sS -o NUL -w "code=%{http_code} bytes=%{size_download}`n" https://pypi.org/simple/six/
"curl exit=$LASTEXITCODE"
"== http to the link's relay address through the proxy (expected refused)"
curl.exe -sS -m 15 -o NUL -w "code=%{http_code}`n" "http://$gw/"
"curl exit=$LASTEXITCODE"
"== direct egress to 1.1.1.1:8080 (expected blocked)"
try {
    $c = New-Object Net.Sockets.TcpClient
    $ok = $c.ConnectAsync('1.1.1.1', 8080).Wait(5000)
    "completed=$ok connected=$($c.Connected)"
} catch { "blocked: $($_.Exception.InnerException.Message)" }
"== relay CA in root store"
"warden roots: $((Get-ChildItem Cert:\LocalMachine\Root | Where-Object { $_.Subject -match 'warden|relay' }).Count)"
"== pause 45 s for the host-side DHCP probe"
Start-Sleep -Seconds 45
"hello from hyperv" | Set-Content hello.txt
"done"
