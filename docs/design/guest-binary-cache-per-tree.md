---
feature: guest-binary-cache-per-tree
status: active
drivers: [qemu, vz, hyperv, firecracker]
seams: []
surfaces: []
security_boundary: none
alters_relationships: false
design_review: skip
native_validation: []
tier2_gaps: []
rules: [TEST-01, FLOW-01]
rule_changes: []
concepts: [guest-binary, cache, dev-build]
related: []
---

# Guest binary cache per source tree

## Problem

`driver.GuestBinary` finds the relay and `warden-io` binaries a VM build puts in
the guest. With no prebuilt binary next to `warden`, it cross-builds one from the
source tree into `CacheDir()/bin/<name>` and reuses it while that file is newer
than the module's `go.sum` and the running `warden`.

Every checkout on a host shares `CacheDir()`. A newer `warden`, or a test binary,
from another checkout finds the cached file stale, rebuilds it from its own
source, and leaves it newer than the first checkout's `warden`, which then stages
the other checkout's binary without noticing. The 2026-11-12 attended Hyper-V
validation of `hyperv-relay-per-build-channel` hit this: its builds staged main's
`warden-io-windows-amd64.exe` (evidence:
`native/windows-hyperv/2026-11-12/attended/` on the `workflow` branch, finding F5).
A branch's validation then tests code it does not contain.

## Prior art

No archived design covers the guest-binary cache. `GuestBinary`'s lookup order
(a prebuilt binary next to `warden`, then the cache, then a fresh build) is
unchanged here.

## Decisions

1. **Key the cache by the source tree.** The cached binary lives under
   `CacheDir()/bin/<tree>/<name>`, where `<tree>` is the first 12 hex digits of
   the SHA-256 of the tree's absolute, cleaned path. Each checkout gets its own
   cache, so no checkout's rebuild can reach another's builds.
   Rejected: keying by a hash of the source contents. That catches edits that
   neither `go.sum` nor the `warden` binary reflects, but hashing the tree on
   every build costs more than the bug warrants, and rebuilding `warden` after an
   edit is already the dev loop. Rejected: a lock around the flat cache. It
   orders the rebuilds but still lets the last checkout's binary win.
2. **No source tree, no cache.** Without a tree there is no key, so
   `GuestBinary` returns its "not found next to warden, and no source tree"
   error, as it does today when no cached binary exists. A release ships its
   guest binaries next to `warden` (lookup 1), so it never relied on the cache.
3. **Old flat cache files are left alone.** Nothing reads `bin/<name>` any more,
   and `warden clean --cache` removes them with the rest of the cache.

## Questions

### Blocking

None.

### Assumed

None.

## Design

`GuestBinary(pkg, goos, goarch)`:

1. A prebuilt binary next to the `warden` executable: use it (unchanged).
2. Find the source tree (`ModuleRoot`). None: refuse, naming the binary.
3. `cached = guestCachePath(CacheDir(), root, name)`. Not stale (newer than
   `go.sum` and `warden`): use it.
4. Otherwise `go build` into `cached` from that tree.

## Defaults and overrides

None. The cache location is internal and has no override.

## Cross-driver impact

Every driver that stages a guest binary from a dev build (qemu and vz: relay and
`warden-io`; hyperv: `warden-io` and, through `driver/relayvm`, the relay;
firecracker: `warden-io`) gets its own per-tree cache. The container driver
builds images, not guest binaries, and is unaffected.

## Validation plan

- **Tier 0** (TEST-01): `guestCachePath` gives one path per tree (a trailing
  separator does not change it), different paths for two trees, and keeps the
  binary's name under `CacheDir()/bin`.
- **Lint**: `make lint` (windows here; the change is untagged Go).
- **Tier 2**: none blocking. The attended Hyper-V run's builds after this change
  stage the branch's own `warden-io` with no binary placed beside `dist/warden`.

## Implementation plan

1. `driver/hostutil.go`: `guestCachePath`, and `GuestBinary` uses it after
   finding the tree (TEST-01).
2. `driver/hostutil_test.go`: the tier-0 cases above.

## Design review

Skipped (FLOW-19): no seams, no relationship change, no security boundary, no
blocking native validation, no assumed questions, no rule changes.
