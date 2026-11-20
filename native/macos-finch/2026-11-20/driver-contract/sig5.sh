#!/bin/bash
# case 5 helper: start a long container build, signal something mid-build, report what is left.
set -u; cd "$(dirname "$0")"; S=$PWD; B=$S/dc/dist/warden; E=$S/ev11/dc; TD=${TMPDIR:-/tmp}
state(){ echo "containers=$(finch ps -a -q 2>/dev/null | wc -l | tr -d ' ') warden-nets=$(finch network ls 2>/dev/null | grep -c warden) workdirs=$(ls -d $TD/warden-* 2>/dev/null | wc -l | tr -d ' ') drivers=$(pgrep -f 'warden-driver-container' | wc -l | tr -d ' ')"; }
for mode in INT TERM KILLDRIVER KILLWARDEN; do
  echo "== $mode; before: $(state)"
  rm -rf out-sig$mode; (cd fp/long && exec $B build -o ../../out-sig$mode . > $E/sig$mode.log 2>&1) & pid=$!
  for i in $(seq 1 120); do grep -q build-started $E/sig$mode.log out-sig$mode/build-output.log 2>/dev/null && break; sleep 2; done
  echo "   mid-build: $(state)"
  case $mode in INT) kill -INT $pid;; TERM) kill -TERM $pid;; KILLDRIVER) pkill -KILL -f 'warden-driver-container';; KILLWARDEN) kill -KILL $pid;; esac
  wait $pid; rc=$?; sleep 20
  echo "   warden rc=$rc; after 20s: $(state)"; grep -vE 'TLS handshake|hosts file|^\s*$|elapsed|config-sha|layer-sha|manifest-sha|index-sha' $E/sig$mode.log | tail -3 | sed 's/^/   | /'
done
