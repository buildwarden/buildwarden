# driver-contract f6-validate-macos-finch — host:macos-finch/macbook.attended-20261120T230945Z (attended)

macbook, macOS arm64, Finch (VM running), branch feat/driver-contract a0934b53,
main be138478, `make build sign` in .scratch worktrees. Builds use a
Containerfile (`FROM alpine:3.21`, COPY and RUN build.sh) unless noted.

**Result: FAIL** (cases 6, 7 and 9); cases 1–5 and 8 pass.

- **Case 1 PASS:** `driver list`: container `*` at dc/dist/warden-driver-container,
  found "beside warden", and qemu, vz, hyperv, firecracker linked; `doctor`:
  `container runtime: finch` / `ok` (as main).
- **Case 2 PASS:** default build SUCCEEDED, ledger verifies (13 signatures);
  inputs/ holds Containerfile and build.sh (as main); driver.json names
  warden-driver-container with its sha256 (matches `shasum -a 256`), found
  beside-warden, shadowed [] (dc/driver-default.json).
- **Case 3 PASS:** `warden shell` (under `script` for a pty) opens an aarch64
  shell; `exit 7` → warden exits 7, `exit 0` → 0 (main also 7).
- **Case 4 PASS:** `[driver.container] relay_image = "dev"` → "Building relay
  from source…", SUCCEEDED; `[runtime] relay_image = "dev"` → one warning
  "[runtime] relay_image is deprecated; use [driver.container] relay_image",
  SUCCEEDED; both set → refused naming the file and the fix, no output dir;
  `WARDEN_IMAGE=ghcr.io/buildwarden/relay:v0.2.0` → "Using relay image: …v0.2.0"
  (pulled; that old relay then never accepts the tunnel, on main too);
  `WARDEN_IMAGE=…:no-such-tag-xyz` → 404 on pull, fails, no fallback to the
  default.
- **Case 5 PASS:** mid-build (3 containers, 1 network, 2 work dirs, 1 driver):
  SIGINT → exit 130, all torn down; SIGTERM → exit 1 "build terminated
  (SIGTERM)", all torn down; driver SIGKILLed → warden "build failed: lost the
  connection to the relay", exit 1, all torn down; warden SIGKILLed → the
  driver tears down by itself, no container/network/driver left (its stage dir,
  lock and record stay; case 6's clean removed them) (case5.txt).
- **Case 6 FAIL:** X (`--tmpdir tmpA`, warden.toml) with warden and driver
  SIGKILLed keeps its 3 containers running. With Y running (default HOME) and Z
  running with HOME=<scratch>/home2 (its lock there), X's warden.toml deleted,
  `warden clean` (clean takes no --tmpdir: its tmpdir is the OS default, not
  X's tmpA) removed X's containers, network, relay image, work dir in tmpA,
  stage dir, lock and record, and the case-5 leftover's lock/record/stage dir,
  and left Y. **It also removed Z's three containers and network** (under
  "Cleaning up after build tpbll3sr"), and Z failed "lost the connection to the
  relay", exit 1 (case6b.txt, c6z2.log). Cause: driver/container/clean.go
  liveProcessBuilds looks in `ps aux` for `warden-build-<id>`, but on finch the
  driver's child is `finch container wait <64-hex container id>` (psprobe:
  `finch container wait 040889b2…`), so the process scan sees no live build
  on macOS with finch and only keep_ids protects a build.
- **Case 7 FAIL (partly):** WARDEN_CTR_CLI=finch and --runtime finch select
  finch (SUCCEEDED); `--network x` → "unknown flag --network for driver
  "container"", no output dir; `[runtime] cli = 5` and `[driver.container] cli
  = 5` each refused naming the file ("want a string, not an integer"), no
  output dir. But a missing runtime (WARDEN_CTR_CLI=podman, --runtime docker,
  neither installed) fails late: "error building relay image: exec: "podman":
  executable file not found in $PATH", exit 1, not a fixable error, and leaves
  an output directory. main does the same, and `doctor` with
  WARDEN_CTR_CLI=podman prints "container runtime: podman / ok" on both.
- **Case 8 PASS:** a copy in a PATH dir ahead of warden's runs: `driver list`
  shows it (found PATH) and the bundled one it shadows; the build's
  driver.json has found PATH, its sha256 and the bundled copy under shadowed;
  a copy behind warden's on PATH does not run (found beside-warden) and is
  listed as shadowed; both builds SUCCEEDED, ledgers verify;
  `[runtime] driver = "./x"` refused ("a config file names a driver, not a
  file"), no output dir.
- **Case 9 FAIL:** `make driver-conformance DRIVER=dist/warden-driver-container`
  with no flags: every lifecycle build fails "names no base image" (the suite
  needs --base-image; CI passes one). With
  `DRIVER_CONFORMANCE_FLAGS="--base-image alpine:3.21"`, twice (CI's
  `--skip shell` on the second): 27 ok, static tier all ok; lifecycle FAIL,
  reproducibly: **inputs** (DRV-45) "the ledger does not record
  conformance-input.txt, read from the context"; **stop** (DRV-24, DRV-49)
  "waiting for stopped, the driver failed: code internal, the build was stopped
  before the relay was claimed (interrupted)"; **end of input** (DRV-50,
  DRV-51) "the orchestrator's input ended before the relay was claimed";
  **clean** (DRV-27, DRV-55, DRV-58) "clean left the killed build: its reap
  removed 1". shell (DRV-28…) cannot get a console in the suite's own attach,
  even under `script` (CI skips it; case 3 covers warden shell).
  Logs: dc/c9-conformance*.log.

No leftover containers, networks, locks, work dirs or driver processes after
the run. Follow-ups filed: none (the missing-runtime behavior that main shares
is part of this item's case 7, returned with it).
