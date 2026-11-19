"now (UTC): " + (Get-Date).ToUniversalTime().ToString('o')
"warden processes:"
Get-CimInstance Win32_Process -Filter "Name LIKE 'warden%'" | Select-Object ProcessId, CreationDate, ExecutablePath, CommandLine | Format-List
$d = Join-Path $env:LOCALAPPDATA 'warden\hyperv'
"prepare.log:"
Get-Item (Join-Path $d 'prepare.log') -ErrorAction SilentlyContinue | Select-Object Length, LastWriteTimeUtc | Format-List
Get-Content (Join-Path $d 'prepare.log') -ErrorAction SilentlyContinue
"images:"
Get-ChildItem (Join-Path $d 'images') -ErrorAction SilentlyContinue | Select-Object Name, Length, LastWriteTimeUtc | Format-Table -AutoSize
