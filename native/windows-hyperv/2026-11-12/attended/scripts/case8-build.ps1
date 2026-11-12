foreach ($n in 'myip.opendns.com','example.com') {
  try { $r = Resolve-DnsName $n -Type A -DnsOnly -ErrorAction Stop | ? { $_.Type -eq 'A' } | select -First 1; "DNS $n -> $($r.IPAddress)" } catch { "DNS $n FAILED: $($_.Exception.Message)" }
}
