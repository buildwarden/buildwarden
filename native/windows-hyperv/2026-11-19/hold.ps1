param([int]$Seconds = 900)
$p = Join-Path $env:LOCALAPPDATA 'warden\hyperv\prepare.log'
$f = [IO.File]::Open($p, 'OpenOrCreate', 'ReadWrite', 'None')
"holding $p with FileShare None for $Seconds s (pid $PID)"
Start-Sleep -Seconds $Seconds
$f.Close()
"released"
