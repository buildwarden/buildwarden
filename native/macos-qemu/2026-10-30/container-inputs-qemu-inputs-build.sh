#!/bin/sh
set -e
apk add --no-cache jq && jq --version > /tmp/jq.txt && warden-io post /tmp/jq.txt
