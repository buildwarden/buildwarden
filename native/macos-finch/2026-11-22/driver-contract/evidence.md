# driver-contract f6-validate-macos-finch — host:macos-finch/macbook.attended-20261122T190734Z (attended)

Re-run after Implement r2 (the 2026-11-20 run on a0934b53 failed cases 6, 7,
9), against the step's r4 note. macbook, macOS arm64, Finch. Branch
feat/driver-contract 190b092d (c477f43 consolidated, plus a Hyper-V hint
commit), main d67da9dd; `make build sign` in .scratch worktrees. f1.sh ran
cases 1, 2, 4, 7 (and 8 on the branch) for main and the branch
(finch-main-summary.txt, finch-dc-summary.txt); f5.sh case 5; f6.sh and f6b.sh
case 6.

**Result: PASS, all nine cases, with the r4 additions to 6 and 7.**

- **1:** `driver list`: container `*` at dc/dist/warden-driver-container, beside
  warden, and qemu/vz/hyperv/firecracker linked (main has no `driver`
  command); `doctor`: `container runtime: finch` / `ok`.
- **2:** default build SUCCEEDED, 13 signatures valid; inputs/ Containerfile and
  build.sh (as main); driver.json names warden-driver-container, sha256
  e45e0460… (= `shasum -a 256`), found beside-warden, shadowed [].
- **3:** `warden shell` under `script`: exit 7 → 7, exit 0 → 0.
- **4:** `[driver.container] relay_image = "dev"` builds the relay from source;
  `[runtime] relay_image = "dev"` the same with one warning naming
  `[driver.container] relay_image`; both → refused naming the file, no output
  dir (main built with both); `WARDEN_IMAGE=…relay:v0.2.0` → "Using relay image:
  …v0.2.0" (that old relay never accepts the tunnel, on main too);
  `…:no-such-tag-xyz` → pull fails, no fallback.
- **5:** mid-build (3 containers, 1 network, 2 work dirs, 1 driver, 3 lock
  files): SIGINT exit 130, SIGTERM exit 1 "build terminated (SIGTERM)", driver
  SIGKILLed → "lost the connection to the relay" exit 1: each leaves nothing;
  warden SIGKILLed → the driver tears down its containers and network itself
  (stage dir, lock and record stay for clean).
- **6 (failed before):** X (`--tmpdir tmpA`, warden.toml) with warden and driver
  SIGKILLed keeps its containers; Y running; Z running with HOME=<scratch>/home2
  (its lock there). After deleting X's warden.toml, `warden clean` removed X's
  three containers, network, relay image, tmpA work dir and stage dir, lock and
  record, and case 5's SIGKILL leftover, and left Y and **Z** running (both
  alive 15 s later; each then stopped by SIGINT, exit 130, leaving nothing)
  (case6.txt). r4 (Decision 25), f6b.sh: `ps -A -ww -o pid,ppid,args` before
  clean shows X's waiter `finch container wait warden-build-w605xnp9` alive
  with PPID 1 (its driver gone), and Z's `finch container wait
  warden-build-ocz1ggsu` under its live `warden-driver-container run` (PID
  27257) (c6b-ps-before-clean.txt); clean removed X and kept Z; no waiter left
  afterwards.
- **7 (failed before):** WARDEN_CTR_CLI=finch and --runtime finch build;
  WARDEN_CTR_CLI=podman → "WARDEN_CTR_CLI names the container runtime podman,
  which is not installed (not found on PATH)" / "To fix: install Podman, or set
  WARDEN_CTR_CLI to another runtime, or unset it", exit 1, **no output dir**
  (main: late "error building relay image", output dir left); --runtime docker
  likewise ("--runtime names …"); `[driver.container] cli = "podman"` likewise,
  naming the file; `--runtime finch` with `finch vm stop` → "--runtime names the
  container runtime finch, which is installed but not running (`finch info`
  failed)" / "To fix: start its VM: finch vm start …", no output dir (VM
  restarted after); `doctor` with WARDEN_CTR_CLI=podman blocked with the same
  error (main: "ok"); --network x → unknown flag for driver "container";
  `[runtime] cli = 5` and `[driver.container] cli = 5` refused naming the file,
  no output dir.
- **8:** copy ahead on PATH runs (found PATH, its sha256, bundled copy under
  shadowed); copy behind does not (found beside-warden, PATH copy shadowed);
  both builds SUCCEEDED; `[runtime] driver = "./x"` refused.
- **9 (failed before):** `make driver-conformance DRIVER=dist/warden-driver-container
  DRIVER_CONFORMANCE_FLAGS="--base-image alpine:3.21 --skip shell"`, twice: 31
  ok, 0 FAIL; skipped: ext (not listed), keep (capabilities.keep none), shell
  (the suite's attach has no console here; CI skips it; case 3 covers warden
  shell). Logs: dc/c9-run1.log, dc/c9-run2.log.

Nothing left afterwards: no containers, networks, build locks, waiters or
driver processes; Finch's VM running. Follow-ups filed: none.
