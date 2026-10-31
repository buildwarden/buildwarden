#!/usr/bin/env bash
# Keeps warden's cached relay boot VHDX built from the current relay source, so a
# Windows host never runs Hyper-V builds behind a relay that predates a fix.
#
#   tools/relay-vm/hyperv/refresh-cache.sh           # rebuild if stale
#   tools/relay-vm/hyperv/refresh-cache.sh --check   # exit 1 if stale; changes nothing
#   tools/relay-vm/hyperv/refresh-cache.sh --force   # rebuild regardless
#
# Run from Git Bash on the Windows host. The source stamp is the last commit to
# touch the relay's packages (go list -deps ./cmd/relay), go.mod/go.sum or
# tools/relay-vm; it is written next to the cached disk as relay-boot.source.
# Building needs a WSL distro (WARDEN_WSL_DISTRO, default Ubuntu) with mtools,
# gdisk, qemu-utils and systemd-boot-efi; the Alpine inputs are fetched on the
# host by fetch-cache.ps1, since that WSL build runs offline.
set -euo pipefail

mode="${1:-refresh}"
here="$(cd "$(dirname "$0")" && pwd)"
repo="$(cd "$here/../../.." && pwd)"
distro="${WARDEN_WSL_DISTRO:-Ubuntu}"
cache="${WARDEN_HYPERV_CACHE_DIR:-$LOCALAPPDATA/warden/hyperv}"
cache="$(cygpath -u "$cache")"
disk="$cache/relay-boot.vhdx"
stampf="$cache/relay-boot.source"

cd "$repo"
mod="$(go list -m)"
paths=$(go list -deps -f '{{if .Module}}{{if .Module.Main}}{{.ImportPath}}{{end}}{{end}}' ./cmd/relay |
	sed "s|^$mod/||")
# This script is not an input to the disk, so editing it does not make it stale.
self=":!tools/relay-vm/hyperv/refresh-cache.sh"
# shellcheck disable=SC2086 # paths is a list of package directories
src="$(git log -1 --format=%H -- $paths go.mod go.sum tools/relay-vm "$self")"
# shellcheck disable=SC2086
[ -z "$(git status --porcelain -- $paths go.mod go.sum tools/relay-vm "$self")" ] || src="$src-dirty"
have="$(cat "$stampf" 2>/dev/null || true)"

if [ "$mode" != --force ] && [ -f "$disk" ] && [ "$have" = "$src" ] && [ "${src%-dirty}" = "$src" ]; then
	echo "relay boot VHDX is current (relay source ${src:0:12})"
	exit 0
fi
if [ "$mode" = --check ]; then
	echo "relay boot VHDX in $cache is stale: built from '${have:-unknown}', relay source is ${src:0:12}"
	echo "  fix: tools/relay-vm/hyperv/refresh-cache.sh"
	exit 1
fi

echo "rebuilding the relay boot VHDX (relay source ${src:0:12}, cached '${have:-none}')"
for f in alpine-minirootfs.tar.gz linux-virt.apk iptables.apk iptables-legacy.apk \
	libxtables.apk libip4tc.apk libip6tc.apk libmnl.apk libnftnl.apk; do
	if [ ! -f "$here/.cache/$f" ]; then
		powershell.exe -NoProfile -ExecutionPolicy Bypass -File "$(cygpath -w "$here/fetch-cache.ps1")"
		break
	fi
done
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o "$here/.cache/warden-relay" ./cmd/relay
wslhere="$(wsl.exe -d "$distro" wslpath -a "$(cygpath -m "$here")" | tr -d '\r')"
wsl.exe -d "$distro" sh -c "cd '$wslhere' && sh build.sh && sh build-boot-vhdx.sh"

mkdir -p "$cache"
[ -f "$disk" ] && cp -f "$disk" "$disk.prev"
cp -f "$here/output/warden-relay-boot.vhdx" "$disk"
echo "$src" >"$stampf"
echo "installed $disk (relay source ${src:0:12}); the previous disk is $disk.prev"
