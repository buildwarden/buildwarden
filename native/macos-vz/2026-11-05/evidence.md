# build-link-subnet f6-validate-macos-vz — host:macos-vz/20261105T124003Z

Unattended, macOS 27.0.1 (26A434), arm64. Branch feat/build-link-subnet at
a68ac2c. warden ad-hoc signed with the virtualization entitlement
(`make -C .scratch/build-link-subnet build`). Cached prepared image
~/.cache/warden/images/macos-90a1b1f94f64b00a (prepared before this feature; no
link-channel stamp).

## NOT COMPLETED — needs an attended sudo re-prepare

The vz validation's core cases (a default-link build, a configured-link build,
the lease taken within the boot window, the link block on a live build) all
need a prepared image that carries the link-channel stamp and the new
DHCP `warden-boot`. The stamp and the rewritten warden-boot are written only by
`sudo warden image prepare --driver vz` (vz.ReprepareImage calls RequireRoot;
geteuid must be 0). This unattended session has no passwordless sudo
(`sudo -n true` is denied), so it cannot re-prepare the cached image, and every
vz build is refused by the required stamp. The step is released, not done.

## What did run (no re-prepare needed)

- `doctor --driver vz` on the un-re-prepared image: "build link
  100.127.255.252/30 (default)", "no host interface overlaps it", then
  "BLOCKED: vz prepared macOS image (warden-boot): link-channel stamp missing;
  rebuild it once: sudo warden image prepare --driver vz" (exit 1).
- A vz build on that image is refused before any VM boots: "vz prepared macOS
  image (warden-boot) predates the configurable build link and must be rebuilt
  once (no link-channel stamp at .../macos-90a1b1f94f64b00a/link-channel) To
  fix: sudo warden image prepare --driver vz".
- Config source: doctor from a dir with [vm] build_link = 100.126.255.252/30
  reports "build link 100.126.255.252/30 (warden.toml)".
- Overlap / malformed --build-link refused (same resolver as every driver):
  10.0.2.0/30 (qemu user-mode 10.0.2.0/24), 192.168.240.0/30 (hyperv NAT
  192.168.240.0/20), 100.64.88.0/30 (firecracker upstream 100.64.88.0/22),
  100.126.0.1/30 (not a network address; did you mean 100.126.0.0/30).

## To finish (attended)

Run `sudo warden image prepare --driver vz` (re-prepares the cached image in
place, no IPSW restore; writes warden-boot's DHCP script and the link-channel
stamp). Then: a default-link build (guest 100.127.255.254, ledger verifies via
inspect --verify-disks), a --build-link 100.126.255.252/30 build with no
re-prepare (guest 100.126.255.254), the lease taken within the boot window
(time from boot to initialize against main), and the link block on the
configured link. The in-guest forged-DHCP/ARP TEST-08 case stays with
security-suite, as on the other hosts.

# Attended run — host:macos-vz/20261105T234633Z (Jeff present)

macOS host 27.0.1 (26A434), arm64, Go 1.26.5. Branch feat/build-link-subnet
a68ac2c, main 4c8f9dc, both `make build sign` in `.scratch/` worktrees.

## Re-prepare

`sudo .scratch/build-link-subnet/dist/warden image prepare --driver vz` (Jeff)
did not refresh the cached image in place: the default source is the latest
IPSW, now macOS 27.2 (26B123), so it restored a new image
`macos-a139c218a2010753` (25.6 GB IPSW, restore and first boot ~13 min), with
`link-channel` = `WARDEN_LINK_CHANNEL=1`. All runs below pass
`--image ~/.cache/warden/images/macos-a139c218a2010753/disk.img`.

Side finding (pre-existing, main too): with two prepared images,
`ImageCache.ImageDir` returns the first valid directory in name order, so a
build with no `--image` keeps using the old `macos-90a1…` (27.0.1) image after
a newer one is prepared; the branch then refuses it for the missing stamp.

## Results

- **Main baseline on 27.2:** 2/2 SUCCEEDED, guest 10.0.0.2, lease ACK 5–6 s
  after the claim (same on the 27.0.1 image); ledger verifies (14 signatures,
  all channels closed).
- **(2) Default link:** 2/2 builds SUCCEEDED, guest `100.127.255.254/30`
  (ifconfig in the guest), DHCP OFFER/ACK 4–6 s after the claim, so the lease
  is taken within the boot window, no slower than main.
- **(3) Configured link `--build-link 100.126.255.252/30`, no re-prepare:**
  2/2 SUCCEEDED, guest `100.126.255.254/30`.
- **Link block on the configured link:** guest fetch of
  `http://100.126.255.253/` through the relay refused (relay "blocked:
  connection to 100.126.255.253 (100.126.255.253) denied"); the next fetch
  200. Ledger verifies (15 signatures).
- **FAIL — empty ledger with compression (default).** Every branch vz build
  above writes a **0-byte `ledger.zst`** while reporting `build SUCCEEDED`;
  `warden inspect` (main's and the branch's): "not a valid BuildWarden ledger
  (invalid magic bytes)". The same build with `--no-compress` writes a 4286-byte
  `ledger` that verifies (14 signatures, all channels closed). Main on the
  same image writes a valid `ledger.zst`. Cause (reading the diff): the branch
  moved vz's stops into `runVMs` (driver/vz/driver.go), which stops the build
  VM and the relay and closes the virtual network *before* `StartBuild` calls
  `driver.FinalizeOutput`; on main those were deferred in `StartBuild`, so they
  ran after it. Likely FinalizeOutput now compresses a ledger that is not yet on
  disk, then removes the source. Not seen on qemu-any, whose ordering did not
  change.
- Not run: the build longer than an hour (left until the ledger fix, as it
  would have to be repeated); the in-guest forged-DHCP/ARP case (TEST-08,
  security-suite, as on the other hosts).

Logs: `attended/` beside this file.
