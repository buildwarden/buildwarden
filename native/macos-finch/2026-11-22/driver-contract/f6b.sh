#!/bin/bash
set -u; cd "$(dirname "$0")"; S=$PWD; B=$S/dc/dist/warden; E=$S/ev/finch-dc
waitfor(){ for i in $(seq 1 150); do grep -q build-started "$1" 2>/dev/null && return; sleep 2; done; echo "  (timeout $1)"; }
(cd fp/c6 && exec $B build --tmpdir $S/tmpA -o $S/out-c6bx . > $E/c6bx.log 2>&1) & X=$!; waitfor $E/c6bx.log; XD=$(pgrep -P $X -f warden-driver-container | head -1); XID=$(grep -o 'Build ID: [a-z0-9]*' $E/c6bx.log | head -1 | awk '{print $3}')
(cd fp/long && HOME=$S/home2 GOPATH=$HOME/go GOMODCACHE=$HOME/go/pkg/mod GOCACHE=$HOME/Library/Caches/go-build exec $B build -o $S/out-c6bz . > $E/c6bz.log 2>&1) & Z=$!; waitfor $E/c6bz.log; ZD=$(pgrep -P $Z -f warden-driver-container | head -1)
echo "X warden=$X driver=$XD; Z warden=$Z driver=$ZD"
echo "== ps before the kill (waiters and drivers)"; ps -A -ww -o pid,ppid,args | grep -E 'container wait warden-build|warden-driver-container' | grep -v grep
kill -KILL $X $XD; sleep 3
echo "== ps after killing X's warden and driver, before warden clean"; ps -A -ww -o pid,ppid,args | grep -E 'container wait warden-build|warden-driver-container' | grep -v grep | tee $E/c6b-ps-before-clean.txt
rm -f fp/c6/warden.toml; (cd fp/c6 && $B clean > $E/c6b-clean.txt 2>&1); echo "== clean rc=$?"; grep -v config: $E/c6b-clean.txt | sed 's/^/  | /'
echo "  containers now: $(finch ps -a --format '{{.Names}}' | sort | tr '\n' ' ')"; echo "  Z alive=$(kill -0 $Z 2>/dev/null && echo yes || echo no)"
kill -INT $Z; wait $Z; echo "  Z rc=$?"; sleep 10; echo "  after: containers=$(finch ps -a -q | wc -l | tr -d ' ') locks=$(ls $HOME/Library/Caches/buildwarden/locks | grep -c build-) tmpA=$(ls tmpA | wc -l | tr -d ' ')"
