#!/bin/sh
set -e
sw_vers
uname -m
curl -sS -o /dev/null -w "%{http_code} %{size_download}\n" https://example.com/
curl -sS -m 20 -o /dev/null -w "port8080 %{http_code} %{size_download}\n" http://1.1.1.1:8080/ || echo "port8080 curl failed rc=$?"
