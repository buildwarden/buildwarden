#!/bin/sh
set -e
sw_vers
uname -m
curl -sS -o /dev/null -w 'example.com %{http_code}\n' https://example.com/
