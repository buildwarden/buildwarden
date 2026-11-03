set -u
export PATH="$HOME/.local/bin:$PATH"
D=$HOME/fcval; W=$D/bin/warden; L=$D/logs; cd $D
t() { date +%s.%N; }
el() { echo "$2 - $1" | bc; }
echo "== sudo doctor"; sudo env PATH="$PATH" $W doctor --driver firecracker 2>&1 | tee $L/doctor-sudo.log | grep -vE "^      "
echo "== direct-boot build"
s=$(t); $W build --driver firecracker --script direct/build.sh --timeout 5m -o out-direct direct > $L/direct.log 2>&1; rc=$?; e=$(t)
echo "rc=$rc secs=$(el $s $e)"; grep -E "hello|6\.1|Example|egress|EGRESS|SUCCEEDED|FAILED|ERROR" $L/direct.log | head
$W inspect out-direct 2>&1 | grep -E "SUMMARY|SIGNATURES|COMPLETENESS|GET " | head -8
mkdir -p slow; printf 'echo slow-start\nsleep 90\necho slow-end\n' > slow/build.sh
echo "== concurrent builds with br_netfilter"
had=$(lsmod | grep -c '^br_netfilter'); sudo modprobe br_netfilter; echo "br_netfilter loaded: $(lsmod | grep -c '^br_netfilter') (before: $had)"
( $W build --driver firecracker --script direct/build.sh --timeout 5m -o out-c1 direct > $L/conc1.log 2>&1; echo "c1 rc=$?" ) &
( $W build --driver firecracker --script direct/build.sh --timeout 5m -o out-c2 direct > $L/conc2.log 2>&1; echo "c2 rc=$?" ) &
wait
grep -h "slot\|SUCCEEDED\|FAILED" $L/conc1.log $L/conc2.log | head -6
[ "$had" = 0 ] && sudo modprobe -r br_netfilter; echo "br_netfilter loaded after: $(lsmod | grep -c '^br_netfilter')"
echo "== build VM killed mid-build"
$W build --driver firecracker --script slow/build.sh --timeout 5m -o out-kill slow > $L/kill-vm.log 2>&1 & wp=$!
sleep 10; pgrep -af firecracker | tee $L/kill-vm-procs.log | cut -c1-200
echo "-- other local user (nobody) to relay :8300 and :8301"
for p in 8300 8301; do if sudo -u nobody timeout 4 bash -c "</dev/tcp/100.64.88.2/$p" 2>/dev/null; then echo "nobody:$p REACHABLE"; else echo "nobody:$p blocked"; fi; done
if timeout 4 bash -c "</dev/tcp/100.64.88.2/8300" 2>/dev/null; then echo "jeff:8300 reachable"; else echo "jeff:8300 blocked"; fi
bp=$(pgrep -af firecracker | grep -v relay | grep -i build | awk '{print $1}' | tail -1)
[ -z "$bp" ] && bp=$(pgrep -n firecracker)
echo "killing build VMM pid $bp"; s=$(t); kill -9 $bp; wait $wp; rc=$?; e=$(t)
echo "warden rc=$rc secs-after-kill=$(el $s $e)"; tail -4 $L/kill-vm.log
sleep 1; pgrep -a firecracker || echo "no firecracker processes"
echo "== warden kill -9 mid-build"
$W build --driver firecracker --script slow/build.sh --timeout 5m -o out-k9 slow > $L/kill-warden.log 2>&1 & wp=$!
sleep 10; echo "firecracker during: $(pgrep -c firecracker)"; kill -9 $wp; wait $wp 2>/dev/null; sleep 3
echo "firecracker after kill -9: $(pgrep -c firecracker)"
s=$(t); $W build --driver firecracker --script direct/build.sh --timeout 5m -o out-after direct > $L/after-k9.log 2>&1; rc=$?; e=$(t)
echo "next build rc=$rc secs=$(el $s $e)"; grep -E "slot|SUCCEEDED|FAILED" $L/after-k9.log | head -3
echo "== teardown"; ls -A ~/.cache/warden/firecracker/work 2>&1; pgrep -a firecracker || echo "no firecracker processes"
$W doctor --driver firecracker 2>&1 | grep -E "orphan|slots"
echo "== distro: firewalld"; command -v firewall-cmd || echo "firewalld not installed"
