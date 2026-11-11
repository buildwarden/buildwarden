#!/bin/sh
# Builds a minimal initramfs for the build VM (qemu direct boot, aarch64).
# Takes the kernel modules from tools/relay-vm/.cache/linux-virt.apk, so they
# match the relay's vmlinuz, and fetches Alpine's static busybox into
# tools/build-vm/.cache/.
#
# Output: tools/build-vm/output/initramfs.cpio.gz and its link-channel stamp
# (Uses the same vmlinuz as relay-vm)
#
# Dependencies: curl, cpio, gzip, tar, go, sha256sum or shasum
# Usage: ./tools/build-vm/build-initramfs.sh

set -eu

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
OUTPUT_DIR="$SCRIPT_DIR/output"
CACHE_DIR="$SCRIPT_DIR/.cache"
RELAY_CACHE="$SCRIPT_DIR/../relay-vm/.cache"
ROOTFS_DIR="$SCRIPT_DIR/.rootfs"

ALPINE_VERSION="3.21"
ALPINE_MIRROR="https://dl-cdn.alpinelinux.org/alpine/v${ALPINE_VERSION}/main/aarch64"
# Pinned for reproducible assets. Alpine keeps only a branch's current
# package, so when it moves the download fails: set both from the new one.
# ALPINE_VERSION and BUSYBOX_VERSION match tools/firecracker-vm/build-initramfs.sh
# (a tier-0 test checks); the checksum is aarch64's.
BUSYBOX_VERSION="1.37.0-r14"
BUSYBOX_SHA256="6fd7ea97062beb51fa785ba858f823e1dfe4daf6bfa91ff4d5359b1061988c69"
BUSYBOX_APK="busybox-static-${BUSYBOX_VERSION}.apk"

if [ ! -f "$RELAY_CACHE/linux-virt.apk" ]; then
    echo "Error: $RELAY_CACHE/linux-virt.apk is missing; run tools/relay-vm/build-initramfs.sh first" >&2
    exit 1
fi

mkdir -p "$OUTPUT_DIR" "$CACHE_DIR"

fetch_busybox() {
    if [ ! -f "$CACHE_DIR/$BUSYBOX_APK" ]; then
        echo "Downloading $BUSYBOX_APK..."
        if ! curl -fsSL "${ALPINE_MIRROR}/${BUSYBOX_APK}" -o "$CACHE_DIR/$BUSYBOX_APK.part"; then
            echo "Error: Alpine $ALPINE_VERSION no longer has $BUSYBOX_APK; set" >&2
            echo "BUSYBOX_VERSION and BUSYBOX_SHA256 in $0 to the current" >&2
            echo "busybox-static (its APKINDEX) and rebuild." >&2
            exit 1
        fi
        mv "$CACHE_DIR/$BUSYBOX_APK.part" "$CACHE_DIR/$BUSYBOX_APK"
    fi
    # macOS, the qemu driver's usual host, ships shasum but not sha256sum.
    if command -v sha256sum >/dev/null 2>&1; then
        (cd "$CACHE_DIR" && echo "$BUSYBOX_SHA256  $BUSYBOX_APK" | sha256sum -c -)
    else
        (cd "$CACHE_DIR" && echo "$BUSYBOX_SHA256  $BUSYBOX_APK" | shasum -a 256 -c -)
    fi
}

fetch_busybox

echo "Building build-vm initramfs..."
rm -rf "$ROOTFS_DIR"
mkdir -p "$ROOTFS_DIR"

for dir in bin sbin dev proc sys etc tmp shared etc/ssl/certs; do
    mkdir -p "$ROOTFS_DIR/$dir"
done

# Empty CA bundle for warden-io to append the relay's ephemeral CA to
touch "$ROOTFS_DIR/etc/ssl/certs/ca-certificates.crt"

# Busybox (static). tar stops after the apk's data segment, hence || true;
# the check after it reports a failed extraction.
rm -rf "$CACHE_DIR/busybox-extract"
mkdir -p "$CACHE_DIR/busybox-extract"
tar -xzf "$CACHE_DIR/$BUSYBOX_APK" -C "$CACHE_DIR/busybox-extract" 2>/dev/null || true
if [ ! -f "$CACHE_DIR/busybox-extract/bin/busybox.static" ]; then
    echo "Error: $CACHE_DIR/$BUSYBOX_APK has no bin/busybox.static; delete it and rerun" >&2
    exit 1
fi
cp "$CACHE_DIR/busybox-extract/bin/busybox.static" "$ROOTFS_DIR/bin/busybox"
rm -rf "$CACHE_DIR/busybox-extract"
chmod 755 "$ROOTFS_DIR/bin/busybox"

for cmd in sh mount mkdir cat echo ip ln ls sleep date rm set grep \
           awk sed head insmod modprobe wget ping kill wait \
           hostname env wc tr udhcpc printf; do
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
    drivers/net/virtio_net.ko.gz \
    net/packet/af_packet.ko.gz; do
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

# udhcpc's script for the init's one DHCP request: it records the lease
# and configures nothing. Kept identical in tools/build-vm/build-initramfs.sh
# and tools/firecracker-vm/build-initramfs.sh (a tier-0 test checks).
cat > "$ROOTFS_DIR/etc/warden-lease.sh" <<'LEASE'
#!/bin/sh
# udhcpc script: records the build link's lease for init and configures
# nothing (no address, route or resolver; init applies them). A value that
# is not a dotted quad is not recorded.
case "$1" in
    bound | renew)
        gw="${router%% *}"
        for v in "$ip" "$subnet" "$gw"; do
            echo "$v" | grep -q '^[0-9]\{1,3\}\.[0-9]\{1,3\}\.[0-9]\{1,3\}\.[0-9]\{1,3\}$' || exit 0
        done
        printf 'ip=%s\nmask=%s\nrouter=%s\n' "$ip" "$subnet" "$gw" > /tmp/warden-lease
        ;;
esac
exit 0
LEASE
chmod 755 "$ROOTFS_DIR/etc/warden-lease.sh"

# Pack
(cd "$ROOTFS_DIR" && find . | cpio -o -H newc 2>/dev/null | gzip -9) \
    > "$OUTPUT_DIR/initramfs.cpio.gz"

rm -rf "$ROOTFS_DIR"
# The link-channel stamp (buildlink.Channel): this init takes the build link
# per build, so warden accepts the initramfs.
echo "WARDEN_LINK_CHANNEL=1" > "$OUTPUT_DIR/link-channel"
echo "Build VM initramfs: $(ls -lh "$OUTPUT_DIR/initramfs.cpio.gz" | awk '{print $5}')"
