#!/bin/sh
set -e
sw_vers -productVersion
curl -sS -o /dev/null -w "fetch %{http_code}\n" https://example.com/
