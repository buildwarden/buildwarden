param([string]$OutFile, [int]$WaitSecs = 240)
$ErrorActionPreference = 'Continue'
$deadline = (Get-Date).AddSeconds($WaitSecs); $ip = $null
while (-not $ip -and (Get-Date) -lt $deadline) {
  $vm = Get-VM -ErrorAction SilentlyContinue | ? { $_.Name -like 'warden-relay-*' } | select -First 1
  if ($vm -and $vm.Notes -match 'warden-relay-ip=(\S+)') { $ip = $Matches[1] } else { Start-Sleep -Milliseconds 200 }
}
"relay $($vm.Name) at $ip" | Out-File $OutFile
Add-Type -AssemblyName System.Net.Http
$h = New-Object System.Net.Http.HttpClient; $h.Timeout = [TimeSpan]::FromSeconds(2)
$done = @{}
while ((Get-Date) -lt $deadline -and $done.Count -lt 2) {
  foreach ($c in @(@{n='claim-no-token';p='/v1/claim';a=$null}, @{n='tunnel-wrong-token';p='/v1/tunnel';a='Bearer 0000wrong'})) {
    if ($done[$c.n]) { continue }
    $req = New-Object System.Net.Http.HttpRequestMessage('POST', "http://${ip}:8301$($c.p)")
    $req.Content = New-Object System.Net.Http.StringContent('{}')
    if ($c.a) { $req.Headers.TryAddWithoutValidation('Authorization', $c.a) | Out-Null }
    try { $r = $h.SendAsync($req).Result; $done[$c.n]=1; "$(Get-Date -f o) $($c.n): $([int]$r.StatusCode) $($r.Content.ReadAsStringAsync().Result.Trim())" | Out-File -Append $OutFile } catch { }
  }
  Start-Sleep -Milliseconds 50
}
