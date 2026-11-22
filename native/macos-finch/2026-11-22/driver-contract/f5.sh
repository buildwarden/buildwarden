#!/bin/bash
set -u; cd "$(dirname "$0")"; S=$PWD; B=$S/dc/dist/warden; E=$S/ev/finch-dc; TD=${TMPDIR:-/tmp}
state(){ echo "containers=$(finch ps -a -q 2>/dev/null | wc -l | tr -d ' ') nets=$(finch network ls 2>/dev/null | grep -c warden) workdirs=$(ls -d $TD/warden-* 2>/dev/null | wc -l | tr -d ' ') drivers=$(pgrep -f 'warden-driver-container' | wc -l | tr -d ' ') locks=$(ls $HOME/Library/Caches/buildwarden/locks 2>/dev/null | grep -c build-)"; }
for mode in INT TERM KILLDRIVER KILLWARDEN; do echo "== $mode; before: $(state)"; rm -rf out-sig$mode; (cd fp/long && exec $B build -o ../../out-sig$mode . > $E/sig$mode.log 2>&1) & pid=$!
  for i in $(seq 1 150); do grep -q build-started $E/sig$mode.log out-sig$mode/build-output.log 2>/dev/null && break; sleep 2; done; echo "   mid: $(state)"
  case $mode in INT) kill -INT $pid;; TERM) kill -TERM $pid;; KILLDRIVER) pkill -KILL -f 'warden-driver-container run';; KILLWARDEN) kill -KILL $pid;; esac
  wait $pid; rc=$?; sleep 20; echo "   rc=$rc; after: $(state)"; grep -vE 'TLS handshake|hosts file|^\s*$|elapsed|sha256:' $E/sig$mode.log | grep -E 'Interrupted|terminated|ERROR|interrupted|Tearing' | tail -2 | sed 's/^/   | /'; done
