#!/bin/sh
set -e
apk add --no-cache jq
jq --version
wget -q -O /tmp/APKINDEX.tar.gz https://dl-cdn.alpinelinux.org/alpine/v3.21/main/aarch64/APKINDEX.tar.gz
wc -c < /tmp/APKINDEX.tar.gz
