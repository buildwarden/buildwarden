$ErrorActionPreference = 'Stop'
$net = 'warden-dev'
$n = 2
function Attached($sw) {
    @(Get-VMNetworkAdapter -All | Where-Object { $_.SwitchName -eq $sw } |
        ForEach-Object { $_.VMName }) -join ', '
}
for ($i = 0; $i -lt $n; $i++) {
    $name = "$net-$i"
    $s = Get-VMSwitch -Name $name -ErrorAction SilentlyContinue
    if (-not $s) {
        New-VMSwitch -Name $name -SwitchType Private | Out-Null
        "SLOT=$name	created"
    } elseif ($s.SwitchType -ne 'Private') {
        "WRONGTYPE=$name	$($s.SwitchType)"
    } else {
        "SLOT=$name	present"
    }
}
$re = '^' + [regex]::Escape($net) + '-([0-9]+)$'
foreach ($s in @(Get-VMSwitch -ErrorAction SilentlyContinue)) {
    if ($s.Name -match $re -and [long]$Matches[1] -ge $n) {
        $vms = Attached $s.Name
        if ($vms) { "BUSY=$($s.Name)	$vms" }
        else { Remove-VMSwitch -Name $s.Name -Force; "REMOVED=$($s.Name)" }
    }
}
$old = Get-VMSwitch -Name $net -ErrorAction SilentlyContinue
if ($old -and $old.SwitchType -eq 'Private') {
    $vms = Attached $net
    if ($vms) { "OLDBUSY=$net	$vms" }
    else { Remove-VMSwitch -Name $net -Force; "RETIRED=$net" }
}
