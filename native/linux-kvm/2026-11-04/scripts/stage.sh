#!/bin/bash
# Stage the branch and main binaries with the OLD (pre-change) boot assets in WSL.
set -e
S=/mnt/c/Users/jeff/buildwarden-native/.scratch
rm -rf ~/fcval ~/fcval-main ~/bwb
mkdir -p ~/fcval/bin ~/fcval/logs ~/fcval-main/bin ~/fcval-main/logs ~/bwb/tools
cp "$S"/build-link-subnet/dist/linux/warden "$S"/build-link-subnet/dist/linux/warden-io-linux-amd64 \
	"$S"/build-link-subnet/dist/linux/warden-relay-linux-amd64 ~/fcval/bin/
cp "$S"/linux-main/warden "$S"/linux-main/warden-io-linux-amd64 "$S"/linux-main/warden-relay-linux-amd64 ~/fcval-main/bin/
chmod +x ~/fcval/bin/* ~/fcval-main/bin/*
for d in ~/fcval/bin ~/fcval-main/bin; do
	cp ~/bw/tools/firecracker-vm/output/vmlinux "$d"/firecracker-vmlinux
	cp ~/bw/tools/relay-vm/output/x86_64/initramfs.cpio.gz "$d"/firecracker-relay-initramfs.cpio.gz
	cp ~/bw/tools/firecracker-vm/output/initramfs.cpio.gz "$d"/firecracker-build-initramfs.cpio.gz
done
# The branch's tool scripts, for building the new assets natively in WSL.
cp -r "$S"/build-link-subnet/tools/firecracker-vm ~/bwb/tools/
cp -r "$S"/build-link-subnet/tools/relay-vm ~/bwb/tools/
rm -rf ~/bwb/tools/firecracker-vm/output ~/bwb/tools/relay-vm/output ~/bwb/tools/relay-vm/hyperv
mkdir -p ~/bwb/tools/firecracker-vm/.cache
cp ~/bw/tools/firecracker-vm/.cache/busybox-static.apk ~/bwb/tools/firecracker-vm/.cache/busybox-static-1.37.0-r14.apk
echo "== staged"
ls -la ~/fcval/bin ~/fcval-main/bin
sha256sum ~/fcval/bin/firecracker-*
~/fcval/bin/warden version 2>&1 | head -2
~/fcval-main/bin/warden version 2>&1 | head -2
