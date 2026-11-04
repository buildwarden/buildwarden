#!/bin/sh
set -e
apk add --no-cache jq
jq --version
wget -qO- https://dl-cdn.alpinelinux.org/alpine/v3.21/main/aarch64/APKINDEX.tar.gz | wc -c
