#!/bin/sh
# Builds the Firecracker direct-boot base initramfs (Decision 6 of
# docs/design/firecracker-driver.md): Alpine's static busybox and the launcher
# (tools/firecracker-vm/init). No warden-io, no modules: the driver appends
# warden-io per build as a second cpio at /bin/warden-io, and the guest
# kernel (build-kernel.sh) has everything built in.
#
# Output: tools/firecracker-vm/output/initramfs.cpio.gz and its link-channel
# stamp
#
# Dependencies: curl, cpio, gzip, tar, sha256sum
# Usage: ./tools/firecracker-vm/build-initramfs.sh

set -eu

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
OUTPUT_DIR="$SCRIPT_DIR/output"
CACHE_DIR="$SCRIPT_DIR/.cache"
ROOTFS_DIR="$SCRIPT_DIR/.rootfs"

ALPINE_VERSION="3.21"
ALPINE_MIRROR="https://dl-cdn.alpinelinux.org/alpine/v${ALPINE_VERSION}/main/x86_64"
# Pinned for reproducible assets. Alpine keeps only a branch's current
# package, so when it moves the download fails: set both from the new one.
BUSYBOX_VERSION="1.37.0-r14"
BUSYBOX_SHA256="feef45f02f31ee30b6b47eed9b643a0f99e63a21a08a90e7c680066a712405bd"
BUSYBOX_APK="busybox-static-${BUSYBOX_VERSION}.apk"

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
    (cd "$CACHE_DIR" && echo "$BUSYBOX_SHA256  $BUSYBOX_APK" | sha256sum -c -)
}

build_initramfs() {
    echo "Building initramfs..."
    rm -rf "$ROOTFS_DIR"
    mkdir -p "$ROOTFS_DIR/bin" "$ROOTFS_DIR/sbin" "$ROOTFS_DIR/usr/bin" \
        "$ROOTFS_DIR/usr/sbin" "$ROOTFS_DIR/dev" "$ROOTFS_DIR/proc" \
        "$ROOTFS_DIR/sys" "$ROOTFS_DIR/tmp" "$ROOTFS_DIR/etc" "$ROOTFS_DIR/root"

    mkdir -p "$CACHE_DIR/busybox-extract"
    tar -xzf "$CACHE_DIR/$BUSYBOX_APK" -C "$CACHE_DIR/busybox-extract" 2>/dev/null || true
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

    (cd "$ROOTFS_DIR" && find . | cpio -o -H newc 2>/dev/null | gzip -9) \
        > "$OUTPUT_DIR/initramfs.cpio.gz"
    rm -rf "$ROOTFS_DIR"
    # The link-channel stamp (buildlink.Channel): this init takes the build link
    # per build, so warden accepts the initramfs.
    echo "WARDEN_LINK_CHANNEL=1" > "$OUTPUT_DIR/link-channel"
    echo "Initramfs built: $(ls -lh "$OUTPUT_DIR/initramfs.cpio.gz" | awk '{print $5}')"
}

fetch_busybox
build_initramfs

echo ""
echo "Firecracker direct-boot artifacts (x86_64):"
echo "  Initramfs: $OUTPUT_DIR/initramfs.cpio.gz (append warden-io per build)"
echo "  Kernel:    $OUTPUT_DIR/vmlinux (build-kernel.sh)"
