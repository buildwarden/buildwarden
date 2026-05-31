#!/bin/sh
# Builds a minimal initramfs for the build VM (test/integration use).
# Reuses cached artifacts from tools/relay-vm/.cache/.
#
# Output: tools/build-vm/output/initramfs.cpio.gz
# (Uses the same vmlinuz as relay-vm)

set -eu

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
OUTPUT_DIR="$SCRIPT_DIR/output"
RELAY_CACHE="$SCRIPT_DIR/../relay-vm/.cache"
ROOTFS_DIR="$SCRIPT_DIR/.rootfs"

if [ ! -f "$RELAY_CACHE/busybox" ] || [ ! -f "$RELAY_CACHE/linux-virt.apk" ]; then
    echo "Error: run tools/relay-vm/build-initramfs.sh first" >&2
    exit 1
fi

mkdir -p "$OUTPUT_DIR"

echo "Building build-vm initramfs..."
rm -rf "$ROOTFS_DIR"
mkdir -p "$ROOTFS_DIR"

for dir in bin sbin dev proc sys etc tmp shared etc/ssl/certs; do
    mkdir -p "$ROOTFS_DIR/$dir"
done

# Empty CA bundle for warden-io to append the relay's ephemeral CA to
touch "$ROOTFS_DIR/etc/ssl/certs/ca-certificates.crt"

# Busybox (static, from relay cache)
cp "$RELAY_CACHE/busybox" "$ROOTFS_DIR/bin/busybox"
chmod 755 "$ROOTFS_DIR/bin/busybox"

for cmd in sh mount mkdir cat echo ip ln ls sleep date rm set grep \
           awk sed head insmod modprobe wget ping kill wait \
           hostname env wc tr; do
    ln -s busybox "$ROOTFS_DIR/bin/$cmd"
done
ln -s ../bin/busybox "$ROOTFS_DIR/sbin/ip"
ln -s ../bin/busybox "$ROOTFS_DIR/sbin/insmod"

# Kernel modules (same set as relay)
mkdir -p "$ROOTFS_DIR/lib/modules"
mkdir -p "$RELAY_CACHE/kernel-extract"
tar -xzf "$RELAY_CACHE/linux-virt.apk" \
    -C "$RELAY_CACHE/kernel-extract" 2>/dev/null || true
KVER=$(ls "$RELAY_CACHE/kernel-extract/lib/modules/" | head -1)
for mod in \
    fs/netfs/netfs.ko.gz \
    net/9p/9pnet.ko.gz \
    net/9p/9pnet_virtio.ko.gz \
    fs/9p/9p.ko.gz \
    net/core/failover.ko.gz \
    drivers/net/net_failover.ko.gz \
    drivers/net/virtio_net.ko.gz; do
    src="$RELAY_CACHE/kernel-extract/lib/modules/$KVER/kernel/$mod"
    if [ -f "$src" ]; then
        dst="$ROOTFS_DIR/lib/modules/$(basename "$mod" .gz)"
        gzip -dc "$src" > "$dst"
    fi
done
rm -rf "$RELAY_CACHE/kernel-extract"

# Cross-compile warden-io for the build VM
MODULE_ROOT="$SCRIPT_DIR/../.."
echo "  Cross-compiling warden-io (linux/arm64)..."
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 \
    go build -ldflags="-s -w" -o "$ROOTFS_DIR/bin/warden-io" \
    "$MODULE_ROOT/cmd/warden-io"

# Init script
cp "$SCRIPT_DIR/init" "$ROOTFS_DIR/init"
chmod 755 "$ROOTFS_DIR/init"

# Pack
(cd "$ROOTFS_DIR" && find . | cpio -o -H newc 2>/dev/null | gzip -9) \
    > "$OUTPUT_DIR/initramfs.cpio.gz"

rm -rf "$ROOTFS_DIR"
echo "Build VM initramfs: $(ls -lh "$OUTPUT_DIR/initramfs.cpio.gz" | awk '{print $5}')"
