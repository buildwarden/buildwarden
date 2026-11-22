# driver-contract f6-validate-macos-vz-r2 — host:macos-vz/macbook.attended-20261122T190734Z (attended)

Re-run after Implement r2. macbook, macOS arm64, vz active image
macos-0181f7e0b146845c (27.2.1). Branch feat/driver-contract 190b092d, main
d67da9dd; `make build sign` in .scratch worktrees. vz.sh ran every case for
each build (vz-main-summary.txt, vz-dc-summary.txt).

**Result: PASS, all five cases** (as on a0934b53, 2026-11-20).

- **1:** `doctor --driver vz` and `image list` identical to main's.
- **2:** vz build SUCCEEDED, 11 signatures valid, all channels closed;
  relay.log written; driver.json `{"name":"vz","contract":1,"found":"linked",
  "shadowed":[]…}` (main writes none). With a Containerfile in the context the
  build SUCCEEDED and inputs/ is absent, as on main.
- **3:** a script exiting 3 → warden 3; `--timeout 30s` → 124 "timed out after
  30s", as on main.
- **4:** mid-build (one $TMPDIR/warden-vz-* work dir, one `--mode=host` relay):
  SIGINT → exit 130, SIGTERM → exit 1 "build terminated (SIGTERM)"; after each,
  no relay process and no work dir. Main: SIGTERM exit 143 with its relay and
  work dir left (removed by hand after the baseline).
- **5:** `--runtime docker` and `--network x` → unknown flag for driver "vz"
  (main accepted --runtime; --network "only used by --driver hyperv");
  `[runtime] cli = 5` does not stop a vz build (SUCCEEDED; main fails parsing);
  `clean --driver vz` with no container runtime on PATH → "Nothing to clean.",
  exit 0 (main: "no container runtime found", exit 1).

Follow-ups filed: none.
