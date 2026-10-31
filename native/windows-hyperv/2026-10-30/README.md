# windows-hyperv 2026-10-30: guest HTTPS through the relay

host:windows-hyperv/20261030T173857Z, attended, main 384b6ce, Hyper-V on
Windows 11 Pro 26300, prepared Server 2025 eval image
windows-server-2025-20261030T175627Z. `diag-build.ps1` is the build script;
`diag-build-output.log` is its console.

## 1. MITM HTTPS responses never end (all drivers)

`curl.exe --ssl-no-revoke https://pypi.org/simple/six/` receives all 16337
bytes, then waits: curl reports "no chunk, no close, no size. Assume close to
signal end" and times out. `Invoke-WebRequest` (with or without -OutFile)
hangs for 300 s. The ledger records each GET complete and closed. Plain HTTP
through the relay is fine.

Cause (relay/proxy.go): the upstream http.Transport leaves
DisableCompression false. For a client that sends no Accept-Encoding
(curl.exe, PowerShell 5.1), Go adds `gzip`, decompresses, and returns
ContentLength -1 with Uncompressed true, so `resp.Write` emits no
Content-Length, no chunked framing and no `Connection: close`, while
ServeTLSConn keeps the connection alive. Binary downloads (no gzip) and
clients that send Accept-Encoding themselves (Maven, pip) are unaffected,
which is why earlier runs passed. Likely fix: DisableCompression: true, so
the guest gets the upstream bytes as sent, or chunk when ContentLength < 0.
A relay change: GLOBAL-06.

## 2. Windows curl.exe rejects relay leaf certificates

Schannel: `CRYPT_E_NO_REVOCATION_CHECK (0x80092012)`; works with
`--ssl-no-revoke`. The relay CA is in LocalMachine\Root. Leaf certs carry no
revocation info, and curl's Schannel backend fails closed on that. Options:
the trust baseline writes a curlrc with `ssl-no-revoke` for Windows guests,
or the relay serves a CRL for its ephemeral CA.

## Resolution

1 is fixed by `DisableCompression: true` (#67, merged as c93ea05). Verified
on this host with the cached relay boot VHDX, rebuilt from that commit by
`tools/relay-vm/hyperv/refresh-cache.sh` (#69): `Invoke-WebRequest` and
`curl.exe` fetch the page in under 1 s, the gzip request gets 5613 gzip
bytes, and there are 23 valid signatures. 2 is fixed in #70 (merged as
98b0ee1): leaf certificates name `http://artifacts/ca.crl`, an empty
CRL the relay serves, and `curl.exe` works with no `--ssl-no-revoke`.
