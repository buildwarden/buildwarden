#!/bin/bash
set -u; cd "$(dirname "$0")"; S=$PWD; B=$S/dc/dist/warden; E=$S/ev/finch-dc; TD=${TMPDIR:-/tmp}; L="$HOME/Library/Caches/buildwarden/locks"
snap(){ echo "  containers: $(finch ps -a --format '{{.Names}}' 2>/dev/null | sort | tr '\n' ' ')"; echo "  nets: $(finch network ls --format '{{.Name}}' 2>/dev/null | grep warden | tr '\n' ' ')"; echo "  locks: $(ls $L 2>/dev/null | grep build- | tr '\n' ' ')"; echo "  home2 locks: $(ls $S/home2/Library/Caches/buildwarden/locks 2>/dev/null | grep build- | tr '\n' ' ')"; echo "  tmpA: $(ls $S/tmpA | tr '\n' ' ') | TMPDIR: $(ls -d $TD/warden-* 2>/dev/null | xargs -n1 basename 2>/dev/null | tr '\n' ' ')"; }
waitfor(){ for i in $(seq 1 150); do grep -q build-started "$1" 2>/dev/null && return; sleep 2; done; echo "  (timeout $1)"; }
echo "== X: --tmpdir tmpA, warden.toml; then SIGKILL warden and its driver"; (cd fp/c6 && exec $B build --tmpdir $S/tmpA -o $S/out-c6x . > $E/c6x.log 2>&1) & X=$!; waitfor $E/c6x.log
XD=$(pgrep -P $X -f warden-driver-container | head -1); kill -KILL $X ${XD:-}; sleep 3; snap
echo "== Y running (default HOME)"; (cd fp/long && exec $B build -o $S/out-c6y . > $E/c6y.log 2>&1) & Y=$!; waitfor $E/c6y.log
echo "== Z running (HOME=home2)"; (cd fp/long && HOME=$S/home2 GOPATH=$HOME/go GOMODCACHE=$HOME/go/pkg/mod GOCACHE=$HOME/Library/Caches/go-build exec $B build -o $S/out-c6z . > $E/c6z.log 2>&1) & Z=$!; waitfor $E/c6z.log
echo "== before clean"; snap
rm -f fp/c6/warden.toml; echo "== warden clean (from fp/c6, warden.toml deleted)"; (cd fp/c6 && $B clean > $E/c6-clean.txt 2>&1); echo "  clean rc=$?"; grep -v 'config:' $E/c6-clean.txt | sed 's/^/  | /'
echo "== after clean"; snap; echo "  Y alive=$(kill -0 $Y 2>/dev/null && echo yes || echo no) Z alive=$(kill -0 $Z 2>/dev/null && echo yes || echo no)"
sleep 15; echo "  15s later: Y alive=$(kill -0 $Y 2>/dev/null && echo yes || echo no) Z alive=$(kill -0 $Z 2>/dev/null && echo yes || echo no)"
kill -INT $Y $Z 2>/dev/null; wait $Y; echo "  Y rc=$?"; wait $Z; echo "  Z rc=$?"; sleep 15; echo "== after stopping Y and Z"; snap
