#!/bin/sh
# Builds the pinned x86_64 guest kernel both Firecracker VMs boot (Decision 10
# of docs/design/firecracker-driver.md): Linux 6.1 LTS from kernel.org with
# Firecracker's published microVM guest config, plus kernel-required.config
# built in. The kernel loads no modules.
#
# Output: tools/firecracker-vm/output/vmlinux (uncompressed, for Firecracker)
#         tools/firecracker-vm/kernel.config (the resolved config; check it in
#         when a pin moves, since kernel_config_test.go reads it)
#
# Dependencies: curl, tar, xz, sha256sum, make, gcc, flex, bison, bc,
#               libelf-dev, libssl-dev
# Usage: ./tools/firecracker-vm/build-kernel.sh

set -eu

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
OUTPUT_DIR="$SCRIPT_DIR/output"
CACHE_DIR="$SCRIPT_DIR/.cache"

KERNEL_VERSION="6.1.191"
KERNEL_SHA256="02f15b77d213e15607923832ef9382e2414ea90df6c007f15a910e139645c35d" # kernel.org sha256sums.asc
FIRECRACKER_VERSION="v1.17.0"
GUEST_CONFIG="microvm-kernel-ci-x86_64-6.1.config"

KERNEL_TARBALL="linux-$KERNEL_VERSION.tar.xz"
KERNEL_URL="https://cdn.kernel.org/pub/linux/kernel/v6.x/$KERNEL_TARBALL"
GUEST_CONFIG_URL="https://raw.githubusercontent.com/firecracker-microvm/firecracker/$FIRECRACKER_VERSION/resources/guest_configs/$GUEST_CONFIG"

mkdir -p "$OUTPUT_DIR" "$CACHE_DIR"
cd "$CACHE_DIR"

if [ ! -f "$KERNEL_TARBALL" ]; then
	echo "Downloading $KERNEL_TARBALL..."
	curl -fsSL -o "$KERNEL_TARBALL.part" "$KERNEL_URL"
	mv "$KERNEL_TARBALL.part" "$KERNEL_TARBALL"
fi
echo "$KERNEL_SHA256  $KERNEL_TARBALL" | sha256sum -c -
if [ ! -f "$GUEST_CONFIG" ]; then
	curl -fsSL -o "$GUEST_CONFIG" "$GUEST_CONFIG_URL"
fi

SRC="$CACHE_DIR/linux-$KERNEL_VERSION"
if [ ! -d "$SRC" ]; then
	echo "Extracting..."
	tar xJf "$KERNEL_TARBALL"
fi

cd "$SRC"
cp "$CACHE_DIR/$GUEST_CONFIG" .config
./scripts/kconfig/merge_config.sh -m .config "$SCRIPT_DIR/kernel-required.config" >/dev/null
make -s olddefconfig

# Every required option must have survived olddefconfig as =y.
missing=$(grep -E '^CONFIG_[A-Z0-9_]+=y$' "$SCRIPT_DIR/kernel-required.config" | while read -r opt; do
	grep -qx "$opt" .config || echo "$opt"
done)
if [ -n "$missing" ]; then
	echo "error: required options not =y after olddefconfig:" >&2
	echo "$missing" >&2
	exit 1
fi
if grep -q '^CONFIG_MODULES=y' .config; then
	echo "error: CONFIG_MODULES is set; the guest kernel loads no modules" >&2
	exit 1
fi

echo "Building vmlinux ($KERNEL_VERSION)..."
make -s -j"$(nproc)" vmlinux
cp vmlinux "$OUTPUT_DIR/vmlinux"
cp .config "$SCRIPT_DIR/kernel.config"

echo ""
echo "Firecracker guest kernel $KERNEL_VERSION (x86_64):"
echo "  vmlinux: $OUTPUT_DIR/vmlinux ($(du -h "$OUTPUT_DIR/vmlinux" | cut -f1))"
echo "  config:  $SCRIPT_DIR/kernel.config"
