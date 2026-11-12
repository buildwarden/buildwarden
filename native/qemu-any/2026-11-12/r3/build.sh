#!/bin/sh
set -e
wget -q -O /tmp/idx http://dl-cdn.alpinelinux.org/alpine/v3.21/main/aarch64/APKINDEX.tar.gz
echo "fetched $(wc -c < /tmp/idx) bytes"
