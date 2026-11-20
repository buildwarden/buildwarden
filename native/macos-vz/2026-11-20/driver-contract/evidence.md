# driver-contract f6-validate-macos-vz — host:macos-vz/macbook.attended-20261120T230040Z (attended)

macbook, macOS 27.2.1 arm64, go1.26.5, vz active image
macos-0181f7e0b146845c (27.2.1). Branch feat/driver-contract a0934b53 and main
be138478, `make build sign` in .scratch worktrees. Script dc-vz.sh runs every
case for one build; summary-main.txt and summary-dc.txt are its output, the
per-case logs are under main/ and dc/.

- **Case 1 PASS:** `doctor --driver vz` and `image list` outputs identical to
  main's.
- **Case 2 PASS:** vz build SUCCEEDED, ledger verifies (11 signatures, all
  channels closed), relay.log written, driver.json `{"name":"vz",
  "contract":1, "found":"linked", "shadowed":[] …}` (main writes none). With a
  Containerfile in the context the build SUCCEEDED and no inputs/ directory
  exists, as on main.
- **Case 3 PASS:** a script exiting 3 makes warden exit 3; `--timeout 30s` on
  a sleeping script exits 124 ("timed out after 30s"), both as on main.
- **Case 4 PASS:** mid-build (the guest's "sleeping" in build-output.log; the
  relay `warden-relay-<id> --mode=host` running as warden's child and one
  $TMPDIR/warden-vz-* work dir present): SIGINT → "build interrupted", exit
  130; SIGTERM → "build terminated (SIGTERM)", exit 1; after each, no relay
  process and no work dir. On main SIGTERM killed warden (exit 143) and left
  its relay process and work dir (both removed by hand after the baseline).
- **Case 5 PASS:** `--runtime docker` → "unknown flag --runtime for driver
  "vz"", exit 1 (main accepted it); `--network x` → "unknown flag --network
  for driver "vz"", exit 1 (main: "only used by --driver hyperv");
  `[runtime] cli = 5` in warden.toml does not stop a vz build (SUCCEEDED; main
  fails parsing it); `clean --driver vz` with no container runtime on PATH
  reports "Nothing to clean.", exit 0 (main: "no container runtime found", exit
  1).

Follow-ups filed: none.
