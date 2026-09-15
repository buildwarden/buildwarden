#!/bin/sh
# Build the relay boot VHDX from the UKI, entirely in userspace: no elevation,
# no Hyper-V role, no mkfs, no loop devices. This is the exact step CI runs on a
# plain Linux runner to publish relay-boot.vhdx as a release artifact, so users'
# `warden hyperv setup` downloads + verifies it instead of building locally.
#
# Pipeline:
#   UKI (EFI-stub kernel + embedded initramfs/cmdline, from build.sh)
#     -> FAT32 EFI System Partition image   (mtools: mformat/mmd/mcopy)
#     -> GPT disk image with that ESP        (sgdisk + dd)
#     -> fixed VHDX                          (qemu-img convert)
#
# Hyper-V Gen2 boots the VHDX over UEFI and loads \EFI\BOOT\BOOTX64.EFI (the
# UKI) with no bootloader -- the kernel's own EFI stub consumes the embedded
# initramfs and cmdline. (Gen2 requires VHDX; the legacy VHD format is not
# supported. go-diskfs only emits raw images, which is why it is not used.)
set -eu

HERE=$(cd "$(dirname "$0")" && pwd)
OUT="$HERE/output"
UKI="$OUT/warden-relay-boot.efi"
VHDX="$OUT/warden-relay-boot.vhdx"
ESP_MB=64

for t in mformat mmd mcopy mdir sgdisk qemu-img; do
	command -v "$t" >/dev/null 2>&1 || { echo "ERROR: missing tool: $t" >&2; exit 1; }
done
[ -f "$UKI" ] || { echo "ERROR: UKI not found: $UKI (run build.sh first)" >&2; exit 1; }

WORK=$(mktemp -d)
trap 'rm -rf "$WORK"' EXIT
ESP="$WORK/esp.img"
DISK="$WORK/disk.raw"

# 1. FAT32 ESP image with the UKI as the default EFI boot application.
truncate -s "${ESP_MB}M" "$ESP"
mformat -i "$ESP" -F -v WARDENESP ::
mmd -i "$ESP" ::/EFI
mmd -i "$ESP" ::/EFI/BOOT
mcopy -i "$ESP" "$UKI" ::/EFI/BOOT/BOOTX64.EFI
echo "--- ESP contents ---"
mdir -i "$ESP" ::/EFI/BOOT

# 2. GPT disk with a single EFI System Partition (type ef00) holding the ESP.
#    Partition starts at sector 2048 (1 MiB align); disk = ESP + 2 MiB for the
#    primary and backup GPT headers.
truncate -s "$((ESP_MB + 2))M" "$DISK"
sgdisk -o "$DISK" >/dev/null
sgdisk -n "1:2048:+${ESP_MB}M" -t 1:ef00 -c 1:"EFI System Partition" "$DISK" >/dev/null
dd if="$ESP" of="$DISK" bs=512 seek=2048 conv=notrunc status=none
echo "--- GPT layout ---"
sgdisk -p "$DISK"

# 3. Fixed VHDX for Hyper-V Gen2.
qemu-img convert -f raw -O vhdx -o subformat=fixed "$DISK" "$VHDX"
echo "--- VHDX ---"
qemu-img info "$VHDX"
ls -lh "$VHDX"
echo "OK: $VHDX"
