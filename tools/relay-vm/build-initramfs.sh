#!/bin/sh
# Builds the relay VM initramfs from Alpine minirootfs.
#
# The relay VM is the trust boundary. Its configuration is entirely outside
# the influence of the build VM. Using Alpine minirootfs provides:
# - Known provenance (official Alpine release)
# - Complete musl libc (supports dynamically linked tools like iptables)
# - Simple auditing ("Alpine 3.21 + iptables + linux-virt modules + init")
#
# Output: tools/relay-vm/output/initramfs.cpio.gz
#         tools/relay-vm/output/vmlinuz
#
# Dependencies: curl, cpio, gzip, tar
# Usage: ./tools/relay-vm/build-initramfs.sh

set -eu

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
OUTPUT_DIR="$SCRIPT_DIR/output"
CACHE_DIR="$SCRIPT_DIR/.cache"
ROOTFS_DIR="$SCRIPT_DIR/.rootfs"

ALPINE_VERSION="3.21"
ALPINE_ARCH="aarch64"
ALPINE_MIRROR="https://dl-cdn.alpinelinux.org/alpine/v${ALPINE_VERSION}/main/${ALPINE_ARCH}"
MINIROOTFS_URL="https://dl-cdn.alpinelinux.org/alpine/v${ALPINE_VERSION}/releases/${ALPINE_ARCH}"
MINIROOTFS_FILE="alpine-minirootfs-3.21.7-aarch64.tar.gz"

mkdir -p "$OUTPUT_DIR" "$CACHE_DIR"

# --- Fetch Alpine minirootfs ---

fetch_minirootfs() {
    if [ -f "$CACHE_DIR/alpine-minirootfs.tar.gz" ]; then
        echo "Using cached minirootfs"
        return
    fi
    echo "Fetching Alpine minirootfs..."
    curl -sSL "$MINIROOTFS_URL/$MINIROOTFS_FILE" \
        -o "$CACHE_DIR/alpine-minirootfs.tar.gz"
}

# --- Fetch Alpine linux-virt kernel ---

fetch_kernel() {
    if [ -f "$CACHE_DIR/vmlinuz" ]; then
        echo "Using cached kernel"
        return
    fi

    echo "Fetching package index..."
    curl -sSL "${ALPINE_MIRROR}/APKINDEX.tar.gz" \
        -o "$CACHE_DIR/APKINDEX.tar.gz"

    PKG_NAME=$(tar -xzf "$CACHE_DIR/APKINDEX.tar.gz" -O APKINDEX 2>/dev/null | \
        awk '/^P:linux-virt$/{found=1} found && /^V:/{print "linux-virt-"substr($0,3)".apk"; exit}')

    if [ -z "$PKG_NAME" ]; then
        echo "Error: Could not find linux-virt package" >&2
        exit 1
    fi

    echo "Downloading $PKG_NAME..."
    curl -sSL "${ALPINE_MIRROR}/${PKG_NAME}" -o "$CACHE_DIR/linux-virt.apk"

    mkdir -p "$CACHE_DIR/kernel-extract"
    tar -xzf "$CACHE_DIR/linux-virt.apk" -C "$CACHE_DIR/kernel-extract" 2>/dev/null || true
    VMLINUZ=$(find "$CACHE_DIR/kernel-extract" -name "vmlinuz-*" -type f | head -1)
    if [ -z "$VMLINUZ" ]; then
        echo "Error: vmlinuz not found" >&2
        exit 1
    fi
    cp "$VMLINUZ" "$CACHE_DIR/vmlinuz"
    rm -rf "$CACHE_DIR/kernel-extract"
    echo "Kernel cached: $(ls -lh "$CACHE_DIR/vmlinuz" | awk '{print $5}')"
}

# --- Fetch iptables package ---

fetch_iptables() {
    if [ -f "$CACHE_DIR/iptables.apk" ]; then
        echo "Using cached iptables"
        return
    fi

    if [ ! -f "$CACHE_DIR/APKINDEX.tar.gz" ]; then
        curl -sSL "${ALPINE_MIRROR}/APKINDEX.tar.gz" \
            -o "$CACHE_DIR/APKINDEX.tar.gz"
    fi

    # Fetch iptables-legacy + extensions (iptables package has the .so files)
    for pkg in iptables iptables-legacy libxtables libip4tc libip6tc libmnl libnftnl; do
        PKG_NAME=$(tar -xzf "$CACHE_DIR/APKINDEX.tar.gz" -O APKINDEX 2>/dev/null | \
            awk "/^P:${pkg}\$/{found=1} found && /^V:/{print \"${pkg}-\"substr(\$0,3)\".apk\"; exit}")
        if [ -n "$PKG_NAME" ]; then
            echo "Downloading $PKG_NAME..."
            curl -sSL "${ALPINE_MIRROR}/${PKG_NAME}" \
                -o "$CACHE_DIR/${pkg}.apk"
        fi
    done
}

# --- Build initramfs ---

build_initramfs() {
    echo "Building initramfs..."
    rm -rf "$ROOTFS_DIR"
    mkdir -p "$ROOTFS_DIR"

    # Unpack Alpine minirootfs as base
    tar -xzf "$CACHE_DIR/alpine-minirootfs.tar.gz" -C "$ROOTFS_DIR"
    mkdir -p "$ROOTFS_DIR/shared"

    # Layer iptables + deps (order: libs first, binary last)
    for pkg in libmnl libnftnl libxtables libip4tc libip6tc iptables iptables-legacy; do
        if [ -f "$CACHE_DIR/${pkg}.apk" ]; then
            tar -xzf "$CACHE_DIR/${pkg}.apk" -C "$ROOTFS_DIR" 2>/dev/null || true
        fi
    done
    # Force iptables → legacy binary (not nft)
    ln -sf xtables-legacy-multi "$ROOTFS_DIR/usr/sbin/iptables" 2>/dev/null || true

    # Extract kernel modules
    echo "Extracting kernel modules..."
    mkdir -p "$CACHE_DIR/kernel-extract"
    tar -xzf "$CACHE_DIR/linux-virt.apk" \
        -C "$CACHE_DIR/kernel-extract" 2>/dev/null || true
    KVER=$(ls "$CACHE_DIR/kernel-extract/lib/modules/" | head -1)
    mkdir -p "$ROOTFS_DIR/lib/modules"

    for mod in \
        drivers/char/hw_random/rng-core.ko.gz \
        drivers/char/hw_random/virtio-rng.ko.gz \
        fs/netfs/netfs.ko.gz \
        fs/fuse/fuse.ko.gz \
        fs/fuse/virtiofs.ko.gz \
        net/9p/9pnet.ko.gz \
        net/9p/9pnet_virtio.ko.gz \
        fs/9p/9p.ko.gz \
        net/core/failover.ko.gz \
        drivers/net/net_failover.ko.gz \
        drivers/net/virtio_net.ko.gz \
        crypto/crc32c_generic.ko.gz \
        lib/libcrc32c.ko.gz \
        net/ipv4/netfilter/nf_defrag_ipv4.ko.gz \
        net/ipv6/netfilter/nf_defrag_ipv6.ko.gz \
        net/netfilter/x_tables.ko.gz \
        net/netfilter/nf_conntrack.ko.gz \
        net/netfilter/nf_nat.ko.gz \
        net/ipv4/netfilter/ip_tables.ko.gz \
        net/ipv4/netfilter/iptable_filter.ko.gz \
        net/ipv4/netfilter/iptable_nat.ko.gz \
        net/netfilter/xt_tcpudp.ko.gz \
        net/netfilter/xt_REDIRECT.ko.gz \
        net/netfilter/xt_MASQUERADE.ko.gz \
        net/netfilter/xt_conntrack.ko.gz \
        net/netfilter/xt_state.ko.gz \
        net/netfilter/nf_tables.ko.gz \
        net/netfilter/nfnetlink.ko.gz; do
        src="$CACHE_DIR/kernel-extract/lib/modules/$KVER/kernel/$mod"
        if [ -f "$src" ]; then
            dst="$ROOTFS_DIR/lib/modules/$(basename "$mod" .gz)"
            gzip -dc "$src" > "$dst"
        fi
    done
    rm -rf "$CACHE_DIR/kernel-extract"

    # Install our init script
    cp "$SCRIPT_DIR/init" "$ROOTFS_DIR/init"
    chmod 755 "$ROOTFS_DIR/init"

    # Remove unnecessary files to reduce size
    rm -rf "$ROOTFS_DIR/var/cache" \
           "$ROOTFS_DIR/usr/share/man" \
           "$ROOTFS_DIR/usr/share/doc" \
           "$ROOTFS_DIR/usr/include"

    # Pack as cpio archive
    (cd "$ROOTFS_DIR" && find . | cpio -o -H newc 2>/dev/null | gzip -9) \
        > "$OUTPUT_DIR/initramfs.cpio.gz"

    rm -rf "$ROOTFS_DIR"
    echo "Initramfs built: $(ls -lh "$OUTPUT_DIR/initramfs.cpio.gz" | awk '{print $5}')"
}

# --- Main ---

fetch_minirootfs
fetch_kernel
fetch_iptables
build_initramfs

cp "$CACHE_DIR/vmlinuz" "$OUTPUT_DIR/vmlinuz"

# Extract uncompressed Image for VZLinuxBootLoader (requires raw ARM64 Image format)
decompress_kernel() {
    if [ -f "$OUTPUT_DIR/Image" ]; then
        echo "Using existing uncompressed Image"
        return
    fi
    echo "Decompressing kernel for VZ (vmlinuz -> Image)..."
    python3 -c "
import sys, zlib
data = open('$OUTPUT_DIR/vmlinuz', 'rb').read()
idx = data.find(b'\x1f\x8b\x08')
if idx < 0:
    print('Error: no gzip payload in vmlinuz', file=sys.stderr)
    sys.exit(1)
raw = zlib.decompress(data[idx:], 16 + zlib.MAX_WBITS)
magic = int.from_bytes(raw[56:60], 'little')
if magic != 0x644d5241:
    print(f'Warning: ARM64 magic not found (got 0x{magic:08x})', file=sys.stderr)
open('$OUTPUT_DIR/Image', 'wb').write(raw)
print(f'  Image: {len(raw)} bytes')
"
}

decompress_kernel

echo ""
echo "Relay VM artifacts:"
echo "  Kernel (QEMU): $OUTPUT_DIR/vmlinuz"
echo "  Kernel (VZ):   $OUTPUT_DIR/Image"
echo "  Initramfs:     $OUTPUT_DIR/initramfs.cpio.gz"
echo "  Base:          Alpine $ALPINE_VERSION minirootfs + iptables + linux-virt modules"
