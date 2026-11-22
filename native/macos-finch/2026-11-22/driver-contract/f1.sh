#!/bin/bash
# driver-contract macos-finch cases 1, 2, 4, 7, 8 for one build. Usage: f1.sh <main|dc>
set -u; cd "$(dirname "$0")"; S=$PWD; w=$1; WD=$S/$w/dist; B=$WD/warden; E=$S/ev/finch-$w; mkdir -p $E
GD=$(dirname $(mise which go)); BP=$GD:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin:/opt/homebrew/bin
r(){ n=$1; p=$2; shift 2; rm -rf $S/out-$w-$n; (cd $S/fp/$p && env "$@" > $E/$n.log 2>&1); rc=$?; echo "== $n rc=$rc out=$([ -d $S/out-$w-$n ] && echo yes || echo no) :: $(grep -hiE 'relay image|building relay|deprecated|ERROR|To fix|SUCCEEDED|FAILED|fetched' $E/$n.log $S/out-$w-$n/build-output.log 2>/dev/null | grep -vE 'TLS|hosts file|config:' | sort -u | head -4 | tr '\n' ' ' | cut -c1-360)"; }
echo "### $w $(git -C $S/$w log --oneline -1 | cut -c1-50)"
echo "-- 1"; $B driver list > $E/driver-list.txt 2>&1; cat $E/driver-list.txt | sed 's/^/   /'; $B doctor 2>&1 | grep -v 'config:' | sed 's/^/   doctor: /'
echo "-- 2"; r basic basic $B build -q -o $S/out-$w-basic .; $B inspect --verify-disks $S/out-$w-basic 2>&1 | grep -E SIGNATURES | tr -s ' '; echo "   inputs: $(ls $S/out-$w-basic/inputs 2>/dev/null | tr '\n' ' ')"; [ -f $S/out-$w-basic/driver.json ] && tr -d '\n ' < $S/out-$w-basic/driver.json | cut -c1-260 && echo
echo "-- 4"; r ri-new ri-new $B build -o $S/out-$w-ri-new .; r ri-old ri-old $B build -o $S/out-$w-ri-old .; r ri-both ri-both $B build -o $S/out-$w-ri-both .; r env basic WARDEN_IMAGE=ghcr.io/buildwarden/relay:v0.2.0 $B build -o $S/out-$w-env .; r bad basic WARDEN_IMAGE=ghcr.io/buildwarden/relay:no-such-tag-xyz $B build -o $S/out-$w-bad .
echo "-- 7"; r c7-env c7 WARDEN_CTR_CLI=finch $B build -q -o $S/out-$w-c7-env .; r c7-flag c7 $B build --runtime finch -q -o $S/out-$w-c7-flag .; r c7-miss-env c7 WARDEN_CTR_CLI=podman $B build -o $S/out-$w-c7-miss-env .; r c7-miss-flag c7 $B build --runtime docker -o $S/out-$w-c7-miss-flag .; r c7-network c7 $B build --network x -o $S/out-$w-c7-network .; r c7-cli5 c7cli5 $B build -o $S/out-$w-c7-cli5 .; r c7-dcli5 c7dcli5 $B build -o $S/out-$w-c7-dcli5 .; echo "   doctor podman: $(WARDEN_CTR_CLI=podman $B doctor 2>&1 | grep -v config: | tr '\n' '|' | cut -c1-250)"
if [ $w = dc ]; then echo "-- 8"; rm -rf $S/pathA $S/pathB; mkdir -p $S/pathA $S/pathB; cp $WD/warden-driver-container $S/pathA/; cp $WD/warden-driver-container $S/pathB/
  PATH=$S/pathA:$WD:$BP $B driver list 2>&1 | head -3 | sed 's/^/   ahead: /'; r c8a basic PATH=$S/pathA:$WD:$BP $B build -q -o $S/out-$w-c8a .; tr -d '\n ' < $S/out-$w-c8a/driver.json | cut -c1-400; echo
  PATH=$WD:$S/pathB:$BP $B driver list 2>&1 | head -3 | sed 's/^/   behind: /'; r c8b basic PATH=$WD:$S/pathB:$BP $B build -q -o $S/out-$w-c8b .; tr -d '\n ' < $S/out-$w-c8b/driver.json | cut -c1-400; echo
  r c8x c8x $B build -o $S/out-$w-c8x .; fi
