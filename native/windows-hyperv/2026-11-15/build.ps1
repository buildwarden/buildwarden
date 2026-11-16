$ErrorActionPreference = 'Stop'
"guest: " + [Environment]::OSVersion.VersionString
$r = Invoke-WebRequest -UseBasicParsing https://example.com/
"example.com: " + $r.StatusCode + " " + $r.RawContentLength + " bytes"
