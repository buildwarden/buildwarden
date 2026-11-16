#!/bin/bash
# project-config-trust-vz backlog cases on feat/project-config-trust. Run as yourself (not with sudo):
#   ./pct-vz.sh    -- it calls sudo itself; enter your password once.
set -u
cd "$(dirname "$0")"; S=$PWD; B=$S/pct/dist/warden; EV=$S/ev9; LOG=$EV/run.txt; : > $LOG
UC=$HOME/.config/warden/config.toml; NAMED=$S/named.toml
step(){ echo; echo "=== $*" | tee -a $LOG; }
run(){ echo "\$ $*" >> $LOG; "$@" > $EV/out.txt 2>&1; rc=$?; grep -vE 'Hashing|TLS handshake' $EV/out.txt | tee -a $LOG; echo "rc=$rc" | tee -a $LOG; }
[ -e "$UC" ] && { echo "refusing: $UC already exists"; exit 1; }
sudo -v || exit 1
step "A: user build with ~/.config/warden/config.toml present"
mkdir -p "$(dirname "$UC")"; printf '[vm]\nbuild_link = "100.127.255.252/30"\n' > "$UC"
(cd pv && run "$B" build --driver vz --script ./build.sh -o "$S/out-a" .)
step "B: sudo image prepare with ~/.config/warden/config.toml present"
run sudo "$B" image prepare --driver vz
rm -f "$UC"; rmdir "$(dirname "$UC")" 2>/dev/null; rmdir "$HOME/.config" 2>/dev/null; ls -la "$HOME/.config/warden" >> $LOG 2>&1
printf '[vm]\nbuild_link = "100.127.255.252/30"\n' > "$NAMED"
step "C: WARDEN_CONFIG=$NAMED set; plain sudo image prepare (sudo drops it)"
export WARDEN_CONFIG=$NAMED
run sudo "$B" image prepare --driver vz
step "D: sudo --preserve-env=WARDEN_CONFIG image prepare"
run sudo --preserve-env=WARDEN_CONFIG "$B" image prepare --driver vz
unset WARDEN_CONFIG
step "E: sudo image prepare --config $NAMED"
run sudo "$B" --config "$NAMED" image prepare --driver vz
step "F: development warden from its tree, vz build"
(cd pv && run "$B" build --driver vz -q --script ./build.sh -o "$S/out-f" .)
"$B" inspect --verify-disks "$S/out-f" 2>&1 | grep -E 'SIGNATURES|COMPLETENESS' | tee -a $LOG
rm -f "$NAMED"; echo; echo "DONE. Evidence: $LOG"
