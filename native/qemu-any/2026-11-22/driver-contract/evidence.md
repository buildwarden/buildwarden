# driver-contract f6-validate-qemu-any — host:qemu-any/macbook.attended-20261122T190734Z (attended)

macbook, macOS arm64, QEMU emulator version 11.1.1. Branch feat/driver-contract 190b092d, main d67da9dd,
each in a .scratch worktree with `make build sign` and both initramfs scripts.
q.sh ran cases 1–4 for each build (qemu-main-summary.txt, qemu-dc-summary.txt).
Direct-boot builds fetch over HTTP (the busybox guest's wget has no TLS through
the relay, as before); the cloud image is the cached alpine_3_21.qcow2.

**Result: PASS, all six cases.**

- **1:** `doctor --driver qemu` identical to main's.
- **2:** direct-boot (`--script`) and cloud-image (`--image … --script`) builds
  SUCCEEDED, 11 signatures each; driver.json `{"name":"qemu","contract":1,
  "found":"linked"…}`. A Containerfile `FROM alpine:3.21` (no --script)
  boots alpine_3_21.qcow2 (environment.json; guest prints 3.21.0 and the RUN
  line), 7 signatures, inputs/ Containerfile and build.sh, as on main; with
  `--script` it still boots alpine_3_21.qcow2, 11 signatures, as on main
  (inputs/ is empty there on both main and the branch).
- **3:** mid-build (2 qemu-system processes, 1 $TMPDIR/warden-qemu-* work
  dir): SIGINT → exit 130, SIGTERM → exit 1 "build terminated (SIGTERM)"
  (main: exit 1 with a "context canceled" bug report), each leaving no
  qemu-system process and no work dir; SIGINT with --keep-vms → exit 130, no
  qemu process, the work dir left ("--keep-vms: left …").
- **4:** `--timeout 30s` on a sleeping script → 124 "timed out after 30s", as
  on main.
- **5:** with Finch's VM stopped, `clean --driver qemu` → "Nothing to clean.",
  exit 0 (main: "container runtime finch is installed but not running", exit
  1). VM restarted after.
- **6:** a second SIGINT sent back to back with the first, while warden was
  tearing down (both delivered, 3 tries): warden ends, exit 130 ("Interrupted,
  cleaning up… (press ctrl+c again to force)" / "build interrupted"), leaving
  no qemu-system process and no work dir, then and 20 s later. qemu's teardown
  takes under 50 ms here, so a forced exit cannot be told apart from a normal
  teardown that absorbed the second signal; a 50 ms gap already found warden
  gone.

Follow-ups filed: none.
