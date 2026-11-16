"user config present: " + (Test-Path (Join-Path $env:USERPROFILE '.config\warden\config.toml'))
"WARDEN_* variables: " + ((Get-ChildItem env:WARDEN* -ErrorAction SilentlyContinue | ForEach-Object { $_.Name }) -join ',')
"OS: " + [Environment]::OSVersion.VersionString
"docker on PATH: " + [bool](Get-Command docker -ErrorAction SilentlyContinue)
"podman: " + (Get-Command podman -ErrorAction SilentlyContinue).Source
