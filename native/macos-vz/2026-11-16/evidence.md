# vz-nocgo-build f6-validate-macos-vz — host:macos-vz/20261116T170552Z (attended)

macbook, macOS 27.2.1 arm64, go1.26.5, Apple clang version 21.0.0 (clang-2100.4.1.1). Branch
feat/vz-nocgo-build ad1fc70 and main 8c41eaa, each in its own .scratch
worktree. vz guest: active image macos-0181f7e0b146845c (27.2.1).

**main first:** `CGO_ENABLED=0 go build ./cmd/warden` FAILS
(`driver/vz/vm.go:25:22: undefined: linuxVMConfig`, … macOSVMConfig,
createLinuxVM); `GOARCH=amd64 CGO_ENABLED=1 go build ./cmd/warden` FAILS
(`Objective-C source files not allowed when not using cgo or SWIG:
vz_darwin.m`); main has no `release-build` target. `make build sign` and
`doctor --driver vz` work (main-doctor-vz.txt).

- **Case 1 PASS:** `make build sign` rc 0. `go list -f '{{.CgoFiles}}
  {{.MFiles}} {{.IgnoredGoFiles}}' ./driver/vz/` →
  `[vz_darwin_arm64.go] [vz_darwin_arm64.m] [network_windows.go vz_stub.go]`.
  `make test` rc 0 (34 packages ok, none failing). `doctor --driver vz`
  output identical to main's. A vz build SUCCEEDED (fetch 200), ledger
  verifies (11 signatures, all channels closed).
- **Case 2 PASS:** `CGO_ENABLED=0 go build -o … ./cmd/warden` rc 0. With it,
  `doctor --driver vz`, `build --driver vz`, `shell --driver vz` and `image
  prepare --driver vz` (no sudo) each exit 1 within 0.2 s with
  "ERROR: this warden was built without cgo, so it has no vz driver (released
  binaries are built this way)" / "To fix: build warden from source on this Mac
  (xcode-select --install, then make build), or use --driver container or
  --driver qemu"; no output dir, no change to ~/.cache/warden/images, no relay
  or VM. (doctor first prints its pure-Go cache checks: build link, image
  stamp.) `image list` output identical to the cgo build's. `doctor --driver
  container` works: `container runtime: finch` / `ok`, same as the cgo build
  (a first run during Finch's VM start reported "finch is installed but not
  running"; both builds agree on reruns, nocgo-doctor-container-1.txt).
- **Case 3 PASS:** the same binary signed `codesign --force --sign -
  --entitlements entitlements.plist` (com.apple.security.virtualization
  present) gives the same ERROR/To fix on all four commands, rc 1; no
  entitlement or VM error.
- **Case 4 PASS:** `make release-build` rc 0 for linux/amd64, linux/arm64,
  darwin/amd64, darwin/arm64 (CGO_ENABLED=0).
- **Case 5 PASS:** `GOARCH=amd64 CGO_ENABLED=1 go build ./cmd/warden` rc 0,
  Mach-O 64-bit x86_64; go list for that target selects no cgo or M file and
  ignores vz_darwin_arm64.go.

Follow-ups filed: none.
