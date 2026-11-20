#!/bin/bash
# driver-contract f6-validate-macos-vz cases, for one build of warden. Usage: dc-vz.sh <main|dc>
set -u; cd "$(dirname "$0")"; S=$PWD; w=$1; B=$S/$w/dist/warden; E=$S/ev10/$w; mkdir -p $E; TD=${TMPDIR:-/tmp}
wd(){ ls -d $TD/warden-vz-* 2>/dev/null | wc -l | tr -d ' '; }
relays(){ pgrep -f "$S/.*relay.*--mode=host|/relay --mode=host" | wc -l | tr -d ' '; }
b(){ n=$1; p=$2; shift 2; rm -rf $S/out-$w-$n; (cd $S/dcp/$p && "$B" build --driver vz -q "$@" --script ./build.sh -o $S/out-$w-$n . > $E/$n.log 2>&1); echo $?; }
echo "### $w ($(git -C $S/$w log --oneline -1 | cut -c1-60))"
"$B" doctor --driver vz > $E/doctor.txt 2>&1; echo "1 doctor rc=$?"; "$B" image list > $E/image-list.txt 2>&1; echo "1 image list rc=$?"
rc=$(b plain plain); echo "2 plain build rc=$rc relay.log=$([ -s $S/out-$w-plain/relay.log ] && echo yes || echo no) driver.json=$(cat $S/out-$w-plain/driver.json 2>/dev/null | tr -d '\n ' | cut -c1-160)"; "$B" inspect --verify-disks $S/out-$w-plain 2>&1 | grep -E 'SIGNATURES|COMPLETENESS' | tr -s ' '
rc=$(b cf cf); echo "2 containerfile build rc=$rc inputs/=$([ -d $S/out-$w-cf/inputs ] && echo present || echo absent) $(grep -hE 'SUCCEEDED|FAILED|ERROR' $E/cf.log | head -2 | tr '\n' ' ')"
rc=$(b x3 x3); echo "3 exit-3 script: warden rc=$rc"
rc=$(b to sleep --timeout 30s); echo "3 timeout 30s: warden rc=$rc $(grep -hE 'timed out' $E/to.log | head -1)"
for sig in INT TERM; do w0=$(wd); (cd $S/dcp/sleep && exec "$B" build --driver vz -q --script ./build.sh -o $S/out-$w-sig$sig . > $E/sig$sig.log 2>&1) & pid=$!; for i in $(seq 1 90); do grep -q 'sleeping' $S/out-$w-sig$sig/build-output.log 2>/dev/null && break; sleep 2; done; during_wd=$(wd); during_r=$(relays); kill -$sig $pid; wait $pid; rc=$?; sleep 3; echo "4 SIG$sig mid-build: warden rc=$rc; during: workdirs=$during_wd relays=$during_r; after: workdirs=$(wd) (before $w0) relays=$(relays) vms=$(pgrep -f 'Virtualization.VirtualMachine' | wc -l | tr -d ' ')"; tail -3 $E/sig$sig.log | tr '\n' '|'; echo; done
(cd $S/dcp/plain && "$B" build --driver vz --runtime docker --script ./build.sh -o $S/out-$w-rt . > $E/runtime-flag.log 2>&1); echo "5 --runtime docker rc=$? $(grep -hiE 'unknown|error' $E/runtime-flag.log | head -1)"
(cd $S/dcp/plain && "$B" build --driver vz --network x --script ./build.sh -o $S/out-$w-nw . > $E/network-flag.log 2>&1); echo "5 --network x rc=$? $(grep -hiE 'unknown|error' $E/network-flag.log | head -1)"
rc=$(b cfg5 cfg5); echo "5 [runtime] cli = 5 build rc=$rc $(grep -hE 'SUCCEEDED|FAILED|ERROR' $E/cfg5.log | head -2 | tr '\n' ' ')"
PATH=/usr/bin:/bin:/usr/sbin:/sbin "$B" clean --driver vz > $E/clean.txt 2>&1; echo "5 clean --driver vz (no runtime on PATH) rc=$?: $(head -3 $E/clean.txt | tr '\n' '|')"
