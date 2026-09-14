#!/bin/sh
# Builds the x86_64 Hyper-V relay VM initramfs + kernel from a PRE-POPULATED
# cache. Unlike tools/relay-vm/build-initramfs.sh (aarch64, virtio, fetches over
# the network), this runs fully OFFLINE: the .cache/ directory must already
# contain the Alpine artifacts, fetched on the host (the WSL environment used to
# assemble this has no network). Populate .cache/ with:
#
#   alpine-minirootfs.tar.gz  linux-virt.apk
#   iptables.apk  iptables-legacy.apk  libxtables.apk
#   libip4tc.apk  libip6tc.apk  libmnl.apk  libnftnl.apk
#
# Output: output/vmlinuz + output/initramfs.cpio.gz (both gitignored).
# Boot on Hyper-V Gen2 via Set-VMFirmware -LinuxKernelImagePath/-LinuxInitrdImagePath.
#
# Usage (from WSL): cd tools/relay-vm/hyperv && sh build.sh

set -eu

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
CACHE_DIR="$SCRIPT_DIR/.cache"
OUTPUT_DIR="$SCRIPT_DIR/output"
# Assemble the rootfs in a native filesystem (not a Windows DrvFs mount under
# /mnt/c), so Linux symlinks and mode bits survive. Only final blobs are
# written back to OUTPUT_DIR.
WORK_DIR="$(mktemp -d)"
ROOTFS_DIR="$WORK_DIR/rootfs"

mkdir -p "$OUTPUT_DIR"

for f in alpine-minirootfs.tar.gz linux-virt.apk iptables.apk \
         iptables-legacy.apk libxtables.apk libip4tc.apk libip6tc.apk \
         libmnl.apk libnftnl.apk; do
    if [ ! -f "$CACHE_DIR/$f" ]; then
        echo "ERROR: missing cache file $CACHE_DIR/$f" >&2
        echo "Pre-fetch the Alpine artifacts on the host first." >&2
        exit 1
    fi
done

echo "Assembling rootfs (offline)..."
rm -rf "$ROOTFS_DIR"
mkdir -p "$ROOTFS_DIR"
tar -xzf "$CACHE_DIR/alpine-minirootfs.tar.gz" -C "$ROOTFS_DIR"
mkdir -p "$ROOTFS_DIR/shared"

# Layer iptables + deps (libs first, binary last).
for pkg in libmnl libnftnl libxtables libip4tc libip6tc iptables iptables-legacy; do
    tar -xzf "$CACHE_DIR/${pkg}.apk" -C "$ROOTFS_DIR" 2>/dev/null || true
done
# apk metadata files are not needed in the initramfs.
rm -f "$ROOTFS_DIR/.PKGINFO" "$ROOTFS_DIR/.SIGN."* 2>/dev/null || true
# Force iptables -> legacy backend (not nft).
ln -sf xtables-legacy-multi "$ROOTFS_DIR/usr/sbin/iptables" 2>/dev/null || true

echo "Extracting kernel + modules..."
KX="$WORK_DIR/kx"; mkdir -p "$KX"
tar -xzf "$CACHE_DIR/linux-virt.apk" -C "$KX" 2>/dev/null || true
KVER=$(ls "$KX/lib/modules/" | head -1)
echo "  kernel: $KVER"
mkdir -p "$ROOTFS_DIR/lib/modules"

# Hyper-V + FAT + netfilter modules. hv_vmbus/hv_netvsc are built-in (=y), so
# they are intentionally NOT listed. Missing modules are skipped, not fatal.
for mod in \
    drivers/scsi/hv_storvsc.ko.gz \
    fs/fat/fat.ko.gz fs/fat/vfat.ko.gz \
    net/packet/af_packet.ko.gz \
    crypto/crc32c_generic.ko.gz lib/libcrc32c.ko.gz \
    net/ipv4/netfilter/nf_defrag_ipv4.ko.gz \
    net/ipv6/netfilter/nf_defrag_ipv6.ko.gz \
    net/netfilter/x_tables.ko.gz net/netfilter/nf_conntrack.ko.gz \
    net/netfilter/nf_nat.ko.gz \
    net/ipv4/netfilter/ip_tables.ko.gz \
    net/ipv4/netfilter/iptable_filter.ko.gz \
    net/ipv4/netfilter/iptable_nat.ko.gz \
    net/netfilter/xt_tcpudp.ko.gz net/netfilter/xt_REDIRECT.ko.gz \
    net/netfilter/xt_MASQUERADE.ko.gz net/netfilter/xt_conntrack.ko.gz \
    net/netfilter/xt_state.ko.gz \
    net/netfilter/nf_tables.ko.gz net/netfilter/nfnetlink.ko.gz; do
    src="$KX/lib/modules/$KVER/kernel/$mod"
    if [ -f "$src" ]; then
        gzip -dc "$src" > "$ROOTFS_DIR/lib/modules/$(basename "$mod" .gz)"
    else
        echo "  (skip missing $mod)"
    fi
done

# Install init.
cp "$SCRIPT_DIR/init" "$ROOTFS_DIR/init"
chmod 755 "$ROOTFS_DIR/init"

# Trim.
rm -rf "$ROOTFS_DIR/var/cache" "$ROOTFS_DIR/usr/share/man" \
       "$ROOTFS_DIR/usr/share/doc" "$ROOTFS_DIR/usr/include"

# Pack cpio. Prefer a system cpio; fall back to the rootfs's own (static musl)
# busybox cpio so no extra package is needed in the offline environment.
if command -v cpio >/dev/null 2>&1; then
    CPIO="cpio"
elif [ -x "$ROOTFS_DIR/bin/busybox" ]; then
    CPIO="$ROOTFS_DIR/bin/busybox cpio"
else
    echo "ERROR: no cpio available (system or busybox)" >&2
    exit 1
fi
echo "Packing initramfs with: $CPIO"
( cd "$ROOTFS_DIR" && find . | $CPIO -o -H newc 2>/dev/null | gzip -9 ) \
    > "$OUTPUT_DIR/initramfs.cpio.gz"

# Copy the kernel (bzImage). Hyper-V Gen2 direct boot loads it as-is; no
# decompression (that step is VZ/ARM-only in the sibling script).
VMLINUZ=$(find "$KX" -name "vmlinuz-*" -type f | head -1)
cp "$VMLINUZ" "$OUTPUT_DIR/vmlinuz"

rm -rf "$WORK_DIR"

echo ""
echo "Hyper-V relay VM artifacts (x86_64, kernel $KVER):"
echo "  Kernel:    $OUTPUT_DIR/vmlinuz    ($(ls -lh "$OUTPUT_DIR/vmlinuz" | awk '{print $5}'))"
echo "  Initramfs: $OUTPUT_DIR/initramfs.cpio.gz  ($(ls -lh "$OUTPUT_DIR/initramfs.cpio.gz" | awk '{print $5}'))"
