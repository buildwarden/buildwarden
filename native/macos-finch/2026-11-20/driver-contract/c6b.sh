#!/bin/bash
set -u; cd "$(dirname "$0")"; S=$PWD; B=$S/dc/dist/warden; E=$S/ev11/dc; TD=${TMPDIR:-/tmp}; L="$HOME/Library/Caches/buildwarden/locks"
snap(){ echo "  containers: $(finch ps -a --format '{{.Names}}' 2>/dev/null | sort | tr '\n' ' ')"; echo "  nets: $(finch network ls --format '{{.Name}}' 2>/dev/null | grep warden | tr '\n' ' ')"; echo "  locks: $(ls $L 2>/dev/null | grep build-container | tr '\n' ' ')"; echo "  home2 locks: $(ls $S/home2/Library/Caches/buildwarden/locks 2>/dev/null | tr '\n' ' ')"; echo "  tmpA: $(ls $S/tmpA | tr '\n' ' ')  TMPDIR warden-*: $(ls -d $TD/warden-* 2>/dev/null | xargs -n1 basename 2>/dev/null | tr '\n' ' ')"; }
waitfor(){ for i in $(seq 1 120); do grep -q build-started "$1" 2>/dev/null && return; sleep 2; done; echo "  (timeout waiting for $1)"; }
echo "== start Y (running, default tmpdir)"; (cd fp/long && exec $B build -o $S/out-c6y2 . > $E/c6y2.log 2>&1) & Y=$!; waitfor $E/c6y2.log
echo "== start Z (running, HOME=home2)"; (cd fp/long && HOME=$S/home2 exec $B build -o $S/out-c6z2 . > $E/c6z2.log 2>&1) & Z=$!; waitfor $E/c6z2.log
echo "== before clean (X=g32wr9rn dead, its warden.toml deleted; tpbll3sr = case 5's SIGKILL leftover)"; ls fp/c6; snap
echo "== warden clean (from fp/c6; clean's tmpdir is the OS default, not X's tmpA)"; (cd fp/c6 && $B clean > $E/c6-clean2.txt 2>&1); echo "  clean rc=$?"; sed 's/^/  | /' $E/c6-clean2.txt
echo "== after clean"; snap; echo "  Y alive=$(kill -0 $Y 2>/dev/null && echo yes || echo no) Z alive=$(kill -0 $Z 2>/dev/null && echo yes || echo no)"
kill -INT $Y $Z; wait $Y; echo "  Y rc=$?"; wait $Z; echo "  Z rc=$?"; sleep 10; echo "== after stopping Y and Z"; snap
