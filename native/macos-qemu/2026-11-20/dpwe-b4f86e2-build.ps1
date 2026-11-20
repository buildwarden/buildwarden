whoami | Out-File -Encoding ascii C:\whoami.txt
$r = Invoke-WebRequest -UseBasicParsing https://www.python.org/ftp/python/3.13.0/python-3.13.0-embed-arm64.zip -OutFile C:\py.zip -PassThru
(Get-Item C:\py.zip).Length | Out-File -Encoding ascii -Append C:\whoami.txt
warden-io post C:\whoami.txt
