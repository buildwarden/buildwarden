#!/bin/bash
# Build the branch's direct-boot launcher initramfs (with its link-channel stamp)
# and the x86_64 relay initramfs, natively in WSL from the copied tool scripts.
set -e
cd ~/bwb
echo "== build-initramfs.sh (firecracker launcher)"
sh tools/firecracker-vm/build-initramfs.sh
echo "== relay initramfs (x86_64, no modules)"
ARCH=x86_64 NO_MODULES=1 sh tools/relay-vm/build-initramfs.sh
echo "== outputs"
ls -la tools/firecracker-vm/output tools/relay-vm/output/x86_64
cat tools/firecracker-vm/output/link-channel
sha256sum tools/firecracker-vm/output/initramfs.cpio.gz tools/relay-vm/output/x86_64/initramfs.cpio.gz
