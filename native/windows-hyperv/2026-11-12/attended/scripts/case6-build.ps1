$ErrorActionPreference = 'Continue'
$me = (Get-NetIPAddress -AddressFamily IPv4 -InterfaceAlias Ethernet).IPAddress
$mac = (Get-NetAdapter -Name Ethernet).MacAddress
$gw = (Get-NetRoute -AddressFamily IPv4 -DestinationPrefix 0.0.0.0/0 | Select-Object -First 1).NextHop
"ISO me=$me mac=$mac gw=$gw"
Invoke-WebRequest -UseBasicParsing http://cwd/dhcpprobe.exe -OutFile C:\warden\dhcpprobe.exe
Start-Sleep -Seconds 30
"ISO ping:"; ping -n 2 $gw | Select-String 'Reply|timed out|unreachable'
"ISO neighbors:"; Get-NetNeighbor -AddressFamily IPv4 -InterfaceAlias Ethernet | ? State -ne 'Permanent' | % { "ISO neigh $($_.IPAddress) $($_.LinkLayerAddress) $($_.State)" }
"ISO dhcp probe:"; & C:\warden\dhcpprobe.exe -src $me -wait 8s 2>&1 | % { "ISO $_" }
Start-Sleep -Seconds 30
"ISO done"
