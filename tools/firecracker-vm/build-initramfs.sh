#!/bin/sh
# Builds the Firecracker direct-boot base initramfs (Decision 6 of
# docs/design/firecracker-driver.md): Alpine's static busybox and the launcher
# (tools/firecracker-vm/init). No warden-io, no modules: the driver appends
# warden-io per build as a second cpio at /bin/warden-io, and the guest
# kernel (build-kernel.sh) has everything built in.
#
# Output: tools/firecracker-vm/output/initramfs.cpio.gz
#
# Dependencies: curl, cpio, gzip, tar
# Usage: ./tools/firecracker-vm/build-initramfs.sh

set -eu

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
OUTPUT_DIR="$SCRIPT_DIR/output"
CACHE_DIR="$SCRIPT_DIR/.cache"
ROOTFS_DIR="$SCRIPT_DIR/.rootfs"

ALPINE_VERSION="3.21"
ALPINE_MIRROR="https://dl-cdn.alpinelinux.org/alpine/v${ALPINE_VERSION}/main/x86_64"

mkdir -p "$OUTPUT_DIR" "$CACHE_DIR"

fetch_busybox() {
    if [ -f "$CACHE_DIR/busybox-static.apk" ]; then
        echo "Using cached busybox-static"
        return
    fi
    curl -fsSL "${ALPINE_MIRROR}/APKINDEX.tar.gz" -o "$CACHE_DIR/APKINDEX.tar.gz"
    PKG_NAME=$(tar -xzf "$CACHE_DIR/APKINDEX.tar.gz" -O APKINDEX 2>/dev/null | \
        awk '/^P:busybox-static$/{found=1} found && /^V:/{print "busybox-static-"substr($0,3)".apk"; exit}')
    if [ -z "$PKG_NAME" ]; then
        echo "Error: Could not find busybox-static package" >&2
        exit 1
    fi
    echo "Downloading $PKG_NAME..."
    curl -fsSL "${ALPINE_MIRROR}/${PKG_NAME}" -o "$CACHE_DIR/busybox-static.apk"
}

build_initramfs() {
    echo "Building initramfs..."
    rm -rf "$ROOTFS_DIR"
    mkdir -p "$ROOTFS_DIR/bin" "$ROOTFS_DIR/sbin" "$ROOTFS_DIR/usr/bin" \
        "$ROOTFS_DIR/usr/sbin" "$ROOTFS_DIR/dev" "$ROOTFS_DIR/proc" \
        "$ROOTFS_DIR/sys" "$ROOTFS_DIR/tmp" "$ROOTFS_DIR/etc" "$ROOTFS_DIR/root"

    mkdir -p "$CACHE_DIR/busybox-extract"
    tar -xzf "$CACHE_DIR/busybox-static.apk" -C "$CACHE_DIR/busybox-extract" 2>/dev/null || true
    cp "$CACHE_DIR/busybox-extract/bin/busybox.static" "$ROOTFS_DIR/bin/busybox"
    rm -rf "$CACHE_DIR/busybox-extract"
    chmod 755 "$ROOTFS_DIR/bin/busybox"

    # One link per applet, at the path busybox itself reports.
    for applet in $("$ROOTFS_DIR/bin/busybox" --list-full); do
        [ "$applet" = bin/busybox ] && continue
        mkdir -p "$ROOTFS_DIR/$(dirname "$applet")"
        ln -sf /bin/busybox "$ROOTFS_DIR/$applet"
    done

    cp "$SCRIPT_DIR/init" "$ROOTFS_DIR/init"
    chmod 755 "$ROOTFS_DIR/init"

    (cd "$ROOTFS_DIR" && find . | cpio -o -H newc 2>/dev/null | gzip -9) \
        > "$OUTPUT_DIR/initramfs.cpio.gz"
    rm -rf "$ROOTFS_DIR"
    echo "Initramfs built: $(ls -lh "$OUTPUT_DIR/initramfs.cpio.gz" | awk '{print $5}')"
}

fetch_busybox
build_initramfs

echo ""
echo "Firecracker direct-boot artifacts (x86_64):"
echo "  Initramfs: $OUTPUT_DIR/initramfs.cpio.gz (append warden-io per build)"
echo "  Kernel:    $OUTPUT_DIR/vmlinux (build-kernel.sh)"
