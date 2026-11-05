#!/bin/bash
# linux-kvm phase 1: slots, main baseline on the old assets, the branch on the
# old assets (launcher refused; disk build on the old relay initramfs), then the
# new launcher with the old relay initramfs (direct boot takes its lease), then
# the new relay initramfs installed.
set -u
export PATH="$HOME/.local/bin:$PATH"
D=$HOME/fcval; L=$D/logs; mkdir -p "$L"; cd "$D"
W=$D/bin/warden; WM=$HOME/fcval-main/bin/warden
FILTER='init: |relay: |\[relay\]|\[build\]|SUCCEEDED|FAILED|ERROR|To fix|hello-from|inet |default via|six|link-channel|build link'

echo "== host"; date -u +%Y-%m-%dT%H:%M:%SZ; uname -r; firecracker --version | head -1; cat /etc/resolv.conf; ip -4 -br addr
sudo -n iptables -S FORWARD | head -1; systemctl is-active warden-fc-posture.service
echo "== assets (old) beside the binaries"; sha256sum "$D"/bin/firecracker-* "$HOME"/fcval-main/bin/firecracker-*

echo "== setup --slots 2 (branch binary)"
sudo -n env PATH="$PATH" "$W" setup --driver firecracker --slots 2 >"$L/01-setup.log" 2>&1; echo "rc=$?"; tail -4 "$L/01-setup.log"

rm -rf direct ubuntu out-*; mkdir -p direct ubuntu
cat >direct/build.sh <<'S'
echo hello-from-direct-boot
uname -r
ip -4 -o addr show | grep -v ' lo '
ip route
wget -q -O - http://example.com/ | head -c 40; echo
S
cat >ubuntu/Dockerfile <<'S'
FROM ubuntu:24.04
RUN ip -4 -o addr show; ip route
RUN curl -fsS -o /tmp/six.html https://pypi.org/simple/six/ && wc -c /tmp/six.html
S

echo "== main $(cat /mnt/c/Users/jeff/buildwarden-native/.scratch/wsl/main-rev 2>/dev/null): direct-boot build on the old assets"
"$WM" build --driver firecracker --verbose --script direct/build.sh --timeout 5m -o out-main-direct direct >"$L/02-main-direct.log" 2>&1; echo "rc=$?"
grep -E "$FILTER" "$L/02-main-direct.log" | head -30

echo "== branch: doctor on the old assets"
"$W" doctor --driver firecracker >"$L/03-doctor-old.log" 2>&1; echo "rc=$?"; cat "$L/03-doctor-old.log"

echo "== branch: direct-boot build with the old launcher (expect refused before any VM)"
"$W" build --driver firecracker --script direct/build.sh --timeout 5m -o out-old-launcher direct >"$L/04-old-launcher-direct.log" 2>&1; echo "rc=$?"; cat "$L/04-old-launcher-direct.log"
pgrep -a firecracker || echo "no firecracker processes"

echo "== branch: ubuntu disk build on the OLD relay initramfs"
"$W" build --driver firecracker --verbose --timeout 15m -o out-oldrelay-ubuntu ubuntu >"$L/05-oldrelay-ubuntu.log" 2>&1; echo "rc=$?"
grep -E "$FILTER" "$L/05-oldrelay-ubuntu.log" | head -40

echo "== install the new launcher (keep the old relay initramfs)"
cp "$HOME/bwb/tools/firecracker-vm/output/initramfs.cpio.gz" "$D/bin/firecracker-build-initramfs.cpio.gz"
cp "$HOME/bwb/tools/firecracker-vm/output/link-channel" "$D/bin/firecracker-build-initramfs.cpio.gz.link-channel"
sha256sum "$D"/bin/firecracker-build-initramfs.cpio.gz; cat "$D"/bin/firecracker-build-initramfs.cpio.gz.link-channel
"$W" doctor --driver firecracker >"$L/06-doctor-newlauncher.log" 2>&1; echo "rc=$?"; grep -E "build link|link-channel|stamp|resolver|DNS" "$L/06-doctor-newlauncher.log"

echo "== branch: direct-boot build on the OLD relay initramfs with the new launcher"
"$W" build --driver firecracker --verbose --script direct/build.sh --timeout 5m -o out-oldrelay-direct direct >"$L/07-oldrelay-direct.log" 2>&1; echo "rc=$?"
grep -E "$FILTER" "$L/07-oldrelay-direct.log" | head -40

echo "== install the new relay initramfs"
cp "$D/bin/firecracker-relay-initramfs.cpio.gz" "$D/old-relay-initramfs.cpio.gz"
cp "$HOME/bwb/tools/relay-vm/output/x86_64/initramfs.cpio.gz" "$D/bin/firecracker-relay-initramfs.cpio.gz"
sha256sum "$D"/bin/firecracker-*
echo "== leftovers"; ls ~/.cache/warden/firecracker/work 2>&1 | head; pgrep -a firecracker || echo "no firecracker processes"
