#!/bin/bash
# linux-kvm phase 2: the new assets on the default link (direct boot and disk,
# ledgers verified), the configured link 100.126.255.252/30 by flag and by
# [vm] build_link, the overlap refusal, and a host resolver inside the link.
set -u
export PATH="$HOME/.local/bin:$PATH"
D=$HOME/fcval; L=$D/logs; mkdir -p "$L"; cd "$D"
W=$D/bin/warden
FILTER='init: (link|no DHCP|ready|malformed)|relay: (build-if|DHCP|claimed)|SUCCEEDED|FAILED|ERROR|To fix|hello-from|inet |default via|six|link-channel|build link'
USERPATH="/home/jeff/.local/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
ALT=100.126.255.252/30

# WSL stops an idle distro between scripts and the slots go with it: re-run
# setup (idempotent) first, as the drivers guide says after a reboot.
echo "== setup --slots 2 (idempotent; the distro restarted since phase 1)"
sudo -n env PATH="$PATH" "$W" setup --driver firecracker --slots 2 >"$L/09-setup.log" 2>&1; echo "rc=$?"; tail -4 "$L/09-setup.log"
echo "== assets (new) beside the binary"; sha256sum "$D"/bin/firecracker-*; cat "$D"/bin/firecracker-build-initramfs.cpio.gz.link-channel
echo "== doctor (default link)"
"$W" doctor --driver firecracker >"$L/10-doctor-default.log" 2>&1; echo "rc=$?"; cat "$L/10-doctor-default.log"

echo "== default link: direct-boot build"
"$W" build --driver firecracker --verbose --script direct/build.sh --timeout 5m -o out-def-direct direct >"$L/11-def-direct.log" 2>&1; echo "rc=$?"
grep -E "$FILTER" "$L/11-def-direct.log" | head -40
echo "== default link: ubuntu disk build"
"$W" build --driver firecracker --verbose --timeout 15m -o out-def-ubuntu ubuntu >"$L/12-def-ubuntu.log" 2>&1; echo "rc=$?"
grep -E "$FILTER" "$L/12-def-ubuntu.log" | head -40
for o in out-def-direct out-def-ubuntu; do
	echo "== inspect --verify-disks $o"
	"$W" inspect --verify-disks "$o" >"$L/13-inspect-$o.log" 2>&1; echo "rc=$?"
	grep -iE "signature|verif|mismatch|channel|environment|complete" "$L/13-inspect-$o.log" | head -12
done

echo "== configured link by flag: doctor --build-link $ALT"
"$W" doctor --driver firecracker --build-link "$ALT" >"$L/14-doctor-alt.log" 2>&1; echo "rc=$?"; grep -E "build link|link-channel|DNS" "$L/14-doctor-alt.log"
echo "== configured link: direct-boot build (no rebuild)"
"$W" build --driver firecracker --build-link "$ALT" --verbose --script direct/build.sh --timeout 5m -o out-alt-direct direct >"$L/15-alt-direct.log" 2>&1; echo "rc=$?"
grep -E "$FILTER" "$L/15-alt-direct.log" | head -40
echo "== configured link: ubuntu disk build"
"$W" build --driver firecracker --build-link "$ALT" --verbose --timeout 15m -o out-alt-ubuntu ubuntu >"$L/16-alt-ubuntu.log" 2>&1; echo "rc=$?"
grep -E "$FILTER" "$L/16-alt-ubuntu.log" | head -40
for o in out-alt-direct out-alt-ubuntu; do
	echo "== inspect --verify-disks $o"
	"$W" inspect --verify-disks "$o" >"$L/17-inspect-$o.log" 2>&1; echo "rc=$?"
	grep -iE "signature|verif|mismatch|channel|environment|complete" "$L/17-inspect-$o.log" | head -12
done

echo "== configured link by [vm] build_link in ./warden.toml"
rm -rf alt; mkdir -p alt; printf '[vm]\nbuild_link = "%s"\n' "$ALT" >alt/warden.toml; cp direct/build.sh alt/build.sh
(cd alt && "$W" doctor --driver firecracker >"$L/18-doctor-toml.log" 2>&1; echo "rc=$?"; grep -E "build link" "$L/18-doctor-toml.log")
(cd alt && "$W" build --driver firecracker --verbose --script build.sh --timeout 5m -o out-toml-direct . >"$L/19-toml-direct.log" 2>&1; echo "rc=$?"; grep -E "$FILTER" "$L/19-toml-direct.log" | head -20)
echo "== flag over key: --build-link 100.125.255.252/30 with warden.toml present"
(cd alt && "$W" doctor --driver firecracker --build-link 100.125.255.252/30 2>&1 | grep -E "build link")
echo "== a bad [vm] build_link fails at load"
mkdir -p bad; printf '[vm]\nbuild_link = "100.126.255.253/30"\n' >bad/warden.toml
(cd bad && "$W" doctor --driver firecracker >"$L/20-doctor-badtoml.log" 2>&1; echo "rc=$?"; cat "$L/20-doctor-badtoml.log")

echo "== overlap refusal: build --build-link 100.64.88.0/30 (expect refused at startup)"
"$W" build --driver firecracker --build-link 100.64.88.0/30 --script direct/build.sh --timeout 5m -o out-overlap direct >"$L/21-overlap.log" 2>&1; echo "rc=$?"; cat "$L/21-overlap.log"
echo "== host interface overlap: --build-link 172.19.128.44/30 (holds eth0's 172.19.128.47)"
"$W" doctor --driver firecracker --build-link 172.19.128.44/30 >"$L/22-hostoverlap.log" 2>&1; echo "rc=$?"; grep -E "build link" "$L/22-hostoverlap.log"
"$W" build --driver firecracker --build-link 172.19.128.44/30 --script direct/build.sh --timeout 5m -o out-hostoverlap direct >>"$L/22-hostoverlap.log" 2>&1; echo "rc=$?"; tail -3 "$L/22-hostoverlap.log"
pgrep -a firecracker || echo "no firecracker processes"

# A host resolver inside the link: warden runs as jeff in a private mount
# namespace with a temporary resolv.conf bind-mounted over /etc/resolv.conf
# (the host file is never changed), as native/linux-kvm/2026-11-02/lan-resolver did.
in_ns() {
	sudo -n unshare -m --propagation private sh -c '
		R="$0"; P="$1"; shift
		mount --bind "$R" /etc/resolv.conf || exit 97
		exec runuser -u jeff -- env -i HOME=/home/jeff USER=jeff LOGNAME=jeff PATH="$P" TERM=dumb "$@"
	' "$RESOLV" "$USERPATH" "$@"
}
echo "== host resolv.conf (unchanged throughout)"; cat /etc/resolv.conf
RESOLV=/tmp/warden-inlink.conf; printf 'nameserver 100.126.255.253\n' >"$RESOLV"
echo "== resolver inside the configured link only: doctor --build-link $ALT"
in_ns "$W" doctor --driver firecracker --build-link "$ALT" >"$L/23-doctor-inlink.log" 2>&1; echo "rc=$?"; grep -iE "DNS|resolver|build link" "$L/23-doctor-inlink.log"
echo "== resolver inside the configured link only: build refuses in preflight"
in_ns "$W" build --driver firecracker --build-link "$ALT" --script direct/build.sh --timeout 5m -o out-inlink direct >"$L/24-build-inlink.log" 2>&1; echo "rc=$?"; cat "$L/24-build-inlink.log"
pgrep -a firecracker || echo "no firecracker processes"
RESOLV=/tmp/warden-inlink-mixed.conf; printf 'nameserver 100.126.255.253\nnameserver 8.8.8.8\n' >"$RESOLV"
echo "== resolver inside the link plus 8.8.8.8: the in-link one is excluded"
in_ns "$W" doctor --driver firecracker --build-link "$ALT" >"$L/25-doctor-inlink-mixed.log" 2>&1; echo "rc=$?"; grep -iE "DNS|resolver" "$L/25-doctor-inlink-mixed.log"
RESOLV=/tmp/warden-inlink-default.conf; printf 'nameserver 100.127.255.253\n' >"$RESOLV"
echo "== resolver inside the default link only: doctor"
in_ns "$W" doctor --driver firecracker >"$L/26-doctor-inlink-default.log" 2>&1; echo "rc=$?"; grep -iE "DNS|resolver" "$L/26-doctor-inlink-default.log"
echo "== resolver 10.0.0.2 (the old link) now passes: doctor"
RESOLV=/tmp/warden-oldlink.conf; printf 'nameserver 10.0.0.2\n' >"$RESOLV"
in_ns "$W" doctor --driver firecracker >"$L/27-doctor-oldlink.log" 2>&1; echo "rc=$?"; grep -iE "DNS|resolver" "$L/27-doctor-oldlink.log"
echo "== host resolv.conf after"; cat /etc/resolv.conf; mount | grep resolv.conf || echo "no resolv.conf bind mount left"
echo "== leftovers"; ls ~/.cache/warden/firecracker/work 2>&1 | head; pgrep -a firecracker || echo "no firecracker processes"
