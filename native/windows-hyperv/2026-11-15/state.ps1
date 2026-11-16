"now (UTC): " + (Get-Date).ToUniversalTime().ToString('o')
"warden processes:"
Get-Process warden* -ErrorAction SilentlyContinue | Select-Object Id, StartTime, Path | Format-List
$d = Join-Path $env:LOCALAPPDATA 'warden\hyperv'
"prepare.log:"
Get-Item (Join-Path $d 'prepare.log') -ErrorAction SilentlyContinue | Select-Object Length, LastWriteTimeUtc, CreationTimeUtc | Format-List
"cache:"
Get-ChildItem $d | Select-Object Name, Length, LastWriteTimeUtc | Format-Table -AutoSize
