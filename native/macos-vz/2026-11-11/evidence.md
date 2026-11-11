# vz-image-prepare-refresh f6-validate-macos-vz — host:macos-vz/20261111T174419Z (attended, Jeff)

Host macOS 27.0.1 (26A434), arm64. Branch feat/vz-image-prepare-refresh at
2168fe5 (`make build sign` in `.scratch/vz`), main beb51ba. Starting cache:
macos-90a1b1f94f64b00a (27.0.1, unstamped, no recorded IPSW),
macos-a139c218a2010753 (27.2 26B123, stamped), both ipsw-*.ipsw, no `active`.
Case 7 staged by `ln ~/.cache/warden/images/ipsw-a139….ipsw
~/UniversalMac_27.2_26B123_Restore.ipsw`. Cases 2–7 ran from one `sudo`
script (prepares as root, builds as jeff); full log `run.txt` (restore
progress and `ls` lines trimmed).

**Apple released macOS 27.2.1 (26B131) after the plan was written**, so the
latest supported IPSW was not 27.2 during this run. Where that changes an
expected output it is noted; each is the designed behavior (Decision 3).

- **main baseline:** a vz build with no `--image` FAILED: first-by-name picks
  macos-90a1…, refused for the missing link-channel stamp (the defect).
- **Case 1 PASS** (no sudo): `image list` marks macos-a139… active with its
  IPSW name, macos-90a1… older with none (no IPSW recorded); `doctor`
  "link-channel stamp current"; build booted 27.2 26B123, 10 signatures valid,
  all channels closed, environment `macos-a139…/disk.img`; no `active` written.
- **Case 2 PASS:** no-source prepare refreshed macos-a139… in place ("Refreshing
  boot agent", no download); `active` = macos-a139…, owned by jeff;
  ipsw-a139… deleted, ipsw-90a1… kept, hard link intact; build 27.2, ledger
  verifies. Ended with the out-of-date warning naming 27.2.1 and `--update`
  (the plan expected none: 27.2 was latest when written).
- **Case 3 PASS:** `prepare <macos-90a1… dir>` refreshed it in place, made it
  active, deleted ipsw-90a1…; build booted 27.0.1 26A434, ledger verifies;
  no-source refresh warned "the active macOS image is macos-90a1b1f94f64b00a
  (IPSW not recorded); macOS 27.2.1 (26B131) is available. To update …
  --update" (the "not prepared" form, since 27.2.1 was not yet prepared).
- **Case 4 PASS:** with macos-a139…/link-channel removed, `build --image
  …/macos-a139…/disk.img` refused: "To fix: sudo warden image prepare --driver
  vz /Users/jeff/.cache/warden/images/macos-a139c218a2010753 (refreshes it and
  makes it active), or omit --image to build from the active image"; running it
  restamped and made it active; build 27.2, ledger verifies.
- **Case 5 PASS (other path):** `prepare --update` with 27.2.1 now latest
  downloaded and restored it into macos-0181f7e0b146845c and made it active
  (the "not prepared" path; the planned "already prepared, no download" path is
  covered natively by case 7's warning and at tier 0). `--update <path>`
  refused ("--update picks the newest IPSW itself"); `--driver qemu --update`
  refused ("not supported for --driver qemu yet"). No ipsw-*.ipsw left in the
  cache afterwards.
- **Case 6 PASS:** offline (Jeff turned the network off), no-source prepare
  refreshed macos-0181… in place, rc 0, no warning or error.
- **Case 7 PASS:** `prepare ~/UniversalMac_27.2_26B123_Restore.ipsw` (plain
  path) restored with no "Downloading IPSW" and no new ipsw-*.ipsw, into
  macos-f6022b9f5a7099f7, made active; the file is still present (same inode);
  provenance `file:///Users/jeff/UniversalMac_27.2_26B123_Restore.ipsw`;
  `image list` shows it active with that file name; build 27.2, ledger
  verifies. Back online, no-source refresh warned "the active macOS image is
  macOS 27.2 (26B123); macOS 27.2.1 (26B131) is already prepared. To use it (no
  download): … --update" (the plan expected none; 27.2.1 is latest and
  prepared). The same path again refreshed in place.
- Cleanup in the script made macos-a139… active again. Note: its "case 7
  image dir" line names macos-0181… by a script bug; case 7's image is
  macos-f6022b9f5a7099f7.

TEST-08 forged-DHCP/ARP is not part of this design. Follow-ups filed: none.
