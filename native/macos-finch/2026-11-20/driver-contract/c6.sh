#!/bin/bash
set -u; cd "$(dirname "$0")"; S=$PWD; B=$S/dc/dist/warden; E=$S/ev11/dc; TD=${TMPDIR:-/tmp}; L="$HOME/Library/Caches/buildwarden/locks"
snap(){ echo "  containers: $(finch ps -a --format '{{.Names}}' 2>/dev/null | sort | tr '\n' ' ')"; echo "  nets: $(finch network ls --format '{{.Name}}' 2>/dev/null | grep warden | tr '\n' ' ')"; echo "  locks: $(ls $L 2>/dev/null | grep build-container | tr '\n' ' ')"; echo "  home2 locks: $(ls $S/home2/Library/Caches/buildwarden/locks 2>/dev/null | tr '\n' ' ')"; echo "  tmpA: $(ls $S/tmpA | tr '\n' ' ')  TMPDIR warden-*: $(ls -d $TD/warden-* 2>/dev/null | xargs -n1 basename 2>/dev/null | tr '\n' ' ')"; }
waitfor(){ for i in $(seq 1 120); do grep -q build-started "$1" 2>/dev/null && return; sleep 2; done; echo "  (timeout waiting for $1)"; }
echo "== start X (--tmpdir tmpA, with warden.toml)"; (cd fp/c6 && exec $B --tmpdir $S/tmpA build -o $S/out-c6x . > $E/c6x.log 2>&1) & X=$!; waitfor $E/c6x.log
XD=$(pgrep -P $X -f warden-driver-container | head -1); echo "  X warden=$X driver=${XD:-?}"; kill -KILL $X ${XD:-}; pkill -KILL -f "warden-driver-container.*" -P $X 2>/dev/null; sleep 3; echo "== X killed (warden and driver)"; snap
echo "== start Y (running, default tmpdir)"; (cd fp/long && exec $B build -o $S/out-c6y . > $E/c6y.log 2>&1) & Y=$!; waitfor $E/c6y.log
echo "== start Z (running, HOME=home2)"; (cd fp/long && HOME=$S/home2 exec $B build -o $S/out-c6z . > $E/c6z.log 2>&1) & Z=$!; waitfor $E/c6z.log
pgrep -fl warden-driver-container | cut -c1-120; echo "== before clean"; snap
rm -f fp/c6/warden.toml; echo "== warden clean --tmpdir tmpB (from fp/c6, warden.toml deleted)"; (cd fp/c6 && $B --tmpdir $S/tmpB clean > $E/c6-clean.txt 2>&1); echo "  clean rc=$?"; sed 's/^/  | /' $E/c6-clean.txt
echo "== after clean"; snap; echo "  Y alive=$(kill -0 $Y 2>/dev/null && echo yes || echo no) Z alive=$(kill -0 $Z 2>/dev/null && echo yes || echo no)"
kill -INT $Y $Z; wait $Y; echo "  Y rc=$?"; wait $Z; echo "  Z rc=$?"; sleep 10; echo "== after stopping Y and Z"; snap
