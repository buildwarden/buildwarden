#!/bin/bash
# driver-contract f6-validate-qemu-any cases for one build. Usage: q.sh <main|dc>
set -u; cd "$(dirname "$0")"; S=$PWD; w=$1; B=$S/$w/dist/warden; E=$S/ev/qemu-$w; mkdir -p $E; TD=${TMPDIR:-/tmp}; IMG=$HOME/Library/Caches/warden/images/alpine_3_21.qcow2
wd(){ ls -d $TD/warden-qemu-* 2>/dev/null | wc -l | tr -d ' '; }
qp(){ pgrep -f qemu-system | wc -l | tr -d ' '; }
b(){ n=$1; p=$2; shift 2; rm -rf $S/out-q-$w-$n; (cd $S/qp/$p && "$B" build --driver qemu -q "$@" -o $S/out-q-$w-$n . > $E/$n.log 2>&1); rc=$?; echo "$n rc=$rc $(grep -hE '^fetched|from-dockerfile|^3\.21|SUCCEEDED|FAILED|ERROR' $E/$n.log $S/out-q-$w-$n/build-output.log 2>/dev/null | sort -u | tr '\n' ' ' | cut -c1-200)"; [ -d $S/out-q-$w-$n ] && "$B" inspect --verify-disks $S/out-q-$w-$n 2>&1 | grep -E 'SIGNATURES' | tr -s ' '; }
echo "### $w ($(git -C $S/$w log --oneline -1 | cut -c1-60))"
"$B" doctor --driver qemu > $E/doctor.txt 2>&1; echo "1 doctor rc=$?"
echo "-- 2"; b direct direct --script ./build.sh; b disk direct --image $IMG --script ./build.sh; b df df; echo "   df inputs: $(ls $S/out-q-$w-df/inputs 2>/dev/null | tr '\n' ' ') disk: $(grep -o 'alpine_3_21[^"]*' $S/out-q-$w-df/environment.json 2>/dev/null | head -1)"; b df-script df --script ./build.sh; echo "   df+script inputs: $(ls $S/out-q-$w-df-script/inputs 2>/dev/null | tr '\n' ' ') disk: $(grep -o 'alpine_3_21[^"]*' $S/out-q-$w-df-script/environment.json 2>/dev/null | head -1)"
echo "-- 4"; b to sleep --script ./build.sh --timeout 30s; grep -h 'timed out' $E/to.log | head -1
echo "-- 3"; for mode in INT TERM KEEP; do extra=""; sig=INT; [ $mode = TERM ] && sig=TERM; [ $mode = KEEP ] && extra=--keep-vms; w0=$(wd); rm -rf $S/out-q-$w-s$mode; (cd $S/qp/sleep && exec "$B" build --driver qemu -q $extra --script ./build.sh -o $S/out-q-$w-s$mode . > $E/s$mode.log 2>&1) & pid=$!; for i in $(seq 1 90); do grep -q sleeping $S/out-q-$w-s$mode/build-output.log 2>/dev/null && break; sleep 2; done; dq=$(qp); dw=$(wd); kill -$sig $pid; wait $pid; rc=$?; sleep 5; echo "   $mode (SIG$sig $extra): rc=$rc during: qemu=$dq workdirs=$dw; after: qemu=$(qp) workdirs=$(wd) (before $w0)"; tail -2 $E/s$mode.log | tr '\n' '|'; echo; done
