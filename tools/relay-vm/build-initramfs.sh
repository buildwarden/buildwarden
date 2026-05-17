#!/bin/sh
# Builds a minimal initramfs for the relay VM.
#
# Output: tools/relay-vm/output/initramfs.cpio.gz
#         tools/relay-vm/output/vmlinuz
#
# Dependencies: curl, cpio, gzip
# The Alpine linux-virt kernel is downloaded and cached.
#
# Usage: ./tools/relay-vm/build-initramfs.sh

set -eu

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
OUTPUT_DIR="$SCRIPT_DIR/output"
CACHE_DIR="$SCRIPT_DIR/.cache"
ROOTFS_DIR="$SCRIPT_DIR/.rootfs"

# Alpine version and architecture for the relay VM kernel
ALPINE_VERSION="3.21"
ALPINE_ARCH="aarch64"
ALPINE_MIRROR="https://dl-cdn.alpinelinux.org/alpine/v${ALPINE_VERSION}/main/${ALPINE_ARCH}"

mkdir -p "$OUTPUT_DIR" "$CACHE_DIR"

# --- Fetch Alpine linux-virt kernel ---

fetch_kernel() {
    if [ -f "$CACHE_DIR/vmlinuz" ]; then
        echo "Using cached kernel"
        return
    fi

    echo "Fetching Alpine linux-virt kernel index..."
    APKINDEX_URL="${ALPINE_MIRROR}/APKINDEX.tar.gz"
    curl -sSL "$APKINDEX_URL" -o "$CACHE_DIR/APKINDEX.tar.gz"

    # Find the linux-virt package filename
    PKG_NAME=$(tar -xzf "$CACHE_DIR/APKINDEX.tar.gz" -O APKINDEX 2>/dev/null | \
        awk '/^P:linux-virt$/{found=1} found && /^V:/{print "linux-virt-"substr($0,3)".apk"; exit}')

    if [ -z "$PKG_NAME" ]; then
        echo "Error: Could not find linux-virt package in index" >&2
        exit 1
    fi

    echo "Downloading $PKG_NAME..."
    curl -sSL "${ALPINE_MIRROR}/${PKG_NAME}" -o "$CACHE_DIR/linux-virt.apk"

    # Extract kernel from the APK (it's a tar.gz with a nested tar)
    mkdir -p "$CACHE_DIR/kernel-extract"
    tar -xzf "$CACHE_DIR/linux-virt.apk" -C "$CACHE_DIR/kernel-extract" 2>/dev/null || true

    # The kernel is at boot/vmlinuz-virt
    VMLINUZ=$(find "$CACHE_DIR/kernel-extract" -name "vmlinuz-*" -type f | head -1)
    if [ -z "$VMLINUZ" ]; then
        echo "Error: vmlinuz not found in kernel package" >&2
        exit 1
    fi
    cp "$VMLINUZ" "$CACHE_DIR/vmlinuz"
    rm -rf "$CACHE_DIR/kernel-extract"
    echo "Kernel cached: $(ls -lh "$CACHE_DIR/vmlinuz" | awk '{print $5}')"
}

# --- Build initramfs ---

build_initramfs() {
    echo "Building initramfs..."
    rm -rf "$ROOTFS_DIR"
    mkdir -p "$ROOTFS_DIR"

    # Create directory structure
    for dir in bin sbin dev proc sys etc tmp shared; do
        mkdir -p "$ROOTFS_DIR/$dir"
    done

    # Install busybox (static, from Alpine)
    if [ ! -f "$CACHE_DIR/busybox" ]; then
        echo "Fetching busybox-static..."
        # Find busybox-static package
        BUSYBOX_PKG=$(tar -xzf "$CACHE_DIR/APKINDEX.tar.gz" -O APKINDEX 2>/dev/null | \
            awk '/^P:busybox-binsh$/{found=1} found && /^V:/{print "busybox-binsh-"substr($0,3)".apk"; exit}')

        # Actually we just need the static busybox binary
        BUSYBOX_PKG=$(tar -xzf "$CACHE_DIR/APKINDEX.tar.gz" -O APKINDEX 2>/dev/null | \
            awk '/^P:busybox$/{found=1} found && /^V:/{print "busybox-"substr($0,3)".apk"; exit}')

        if [ -z "$BUSYBOX_PKG" ]; then
            echo "Error: Could not find busybox package" >&2
            exit 1
        fi

        curl -sSL "${ALPINE_MIRROR}/${BUSYBOX_PKG}" -o "$CACHE_DIR/busybox.apk"
        mkdir -p "$CACHE_DIR/bb-extract"
        tar -xzf "$CACHE_DIR/busybox.apk" -C "$CACHE_DIR/bb-extract" 2>/dev/null || true
        cp "$CACHE_DIR/bb-extract/bin/busybox" "$CACHE_DIR/busybox"
        rm -rf "$CACHE_DIR/bb-extract"
    fi

    cp "$CACHE_DIR/busybox" "$ROOTFS_DIR/bin/busybox"
    chmod 755 "$ROOTFS_DIR/bin/busybox"

    # Create busybox symlinks for essential commands
    for cmd in sh mount mkdir cat echo ip ln ls sleep date rm set grep \
               awk sed head udhcpc; do
        ln -s busybox "$ROOTFS_DIR/bin/$cmd"
    done
    ln -s ../bin/busybox "$ROOTFS_DIR/sbin/ip"
    ln -s ../bin/busybox "$ROOTFS_DIR/sbin/udhcpc"

    # Install init script
    cp "$SCRIPT_DIR/init" "$ROOTFS_DIR/init"
    chmod 755 "$ROOTFS_DIR/init"

    # Pack as cpio archive
    (cd "$ROOTFS_DIR" && find . | cpio -o -H newc 2>/dev/null | gzip -9) \
        > "$OUTPUT_DIR/initramfs.cpio.gz"

    rm -rf "$ROOTFS_DIR"
    echo "Initramfs built: $(ls -lh "$OUTPUT_DIR/initramfs.cpio.gz" | awk '{print $5}')"
}

# --- Main ---

fetch_kernel
build_initramfs

# Copy kernel to output
cp "$CACHE_DIR/vmlinuz" "$OUTPUT_DIR/vmlinuz"

echo ""
echo "Relay VM artifacts:"
echo "  Kernel:   $OUTPUT_DIR/vmlinuz"
echo "  Initramfs: $OUTPUT_DIR/initramfs.cpio.gz"
echo ""
echo "These are used by the vz driver to boot the relay VM."
echo "The relay binary itself is placed on the shared volume at runtime."
