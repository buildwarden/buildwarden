$ErrorActionPreference = 'Continue'
$ProgressPreference = 'SilentlyContinue'
"== guest"
[Environment]::OSVersion.VersionString
"== https via Invoke-WebRequest"
try { "status=$((Invoke-WebRequest -UseBasicParsing https://example.com/).StatusCode)" } catch { "failed: $($_.Exception.Message)" }
"== https via curl.exe (no --ssl-no-revoke)"
curl.exe -sS -o NUL -w "code=%{http_code} bytes=%{size_download}`n" https://pypi.org/simple/six/
"curl exit=$LASTEXITCODE"
"== direct egress to 1.1.1.1:8080 (expected blocked)"
try {
    $c = New-Object Net.Sockets.TcpClient
    $ok = $c.ConnectAsync('1.1.1.1', 8080).Wait(5000)
    "completed=$ok connected=$($c.Connected)"
} catch { "blocked: $($_.Exception.InnerException.Message)" }
"== relay CA in root store"
"warden roots: $((Get-ChildItem Cert:\LocalMachine\Root | Where-Object { $_.Subject -match 'warden|relay' }).Count)"
"hello from hyperv" | Set-Content hello.txt
"done"
