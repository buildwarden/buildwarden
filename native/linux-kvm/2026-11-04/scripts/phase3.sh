#!/bin/bash
# linux-kvm phase 3: a DHCP DISCOVER from the host on the slot's upstream TAP
# during a build gets no offer (control: one on the relay-side TAP does), the
# build fetching the link's relay address is refused, and direct egress from
# the guest is dropped (host chain counters). Then cleanup.
set -u
export PATH="$HOME/.local/bin:$PATH"
D=$HOME/fcval; L=$D/logs; mkdir -p "$L"; cd "$D"
W=$D/bin/warden
FILTER='init: (link|no DHCP)|relay: (build-if|DHCP|claimed)|SUCCEEDED|FAILED|ERROR|To fix|inet |default via|relay=|code=|curl|egress|udp-|blocked|six'
cp /mnt/c/Users/jeff/buildwarden-native/.scratch/dhcpprobe/dhcpprobe-linux "$D/dhcpprobe"; chmod +x "$D/dhcpprobe"
echo "== setup --slots 2 (idempotent; the distro may have restarted)"
sudo -n env PATH="$PATH" "$W" setup --driver firecracker --slots 2 >"$L/29-setup.log" 2>&1; echo "rc=$?"; tail -4 "$L/29-setup.log"

rm -rf sec; mkdir -p sec
cat >sec/Dockerfile <<'S'
FROM ubuntu:24.04
RUN ip -4 -o addr show; ip route
RUN gw=$(ip -4 route | awk '/default/{print $3}'); echo "relay=$gw"; \
    echo "== http to the link's relay address through the proxy (expected refused)"; \
    curl -sS -m 15 -o /dev/null -w "code=%{http_code}\n" "http://$gw/" || echo "curl exit=$?"; \
    echo "== http to the guest's own link address through the proxy (expected refused)"; \
    me=$(ip -4 -o addr show | awk '!/ lo /{print $4}' | cut -d/ -f1 | head -1); \
    curl -sS -m 15 -o /dev/null -w "code=%{http_code}\n" "http://$me/" || echo "curl exit=$?"
RUN echo "== direct egress (expected blocked)"; \
    (timeout 5 bash -c '</dev/tcp/1.1.1.1/22' && echo EGRESS-OPEN-tcp22) || echo egress-blocked-tcp22; \
    (timeout 5 bash -c 'echo canary > /dev/udp/1.1.1.1/9' && echo udp-send-accepted-locally) || echo udp-send-failed
RUN echo "== pause 30 s for the host-side DHCP probes"; sleep 30
RUN echo "== next fetch through the relay"; curl -fsS -o /tmp/six.html https://pypi.org/simple/six/ && wc -c /tmp/six.html
S

echo "== host chain counters before"; sudo -n iptables -L WARDEN-FC-FWD -v -n -x 2>/dev/null | head -20 >"$L/30-counters-before.log"; cat "$L/30-counters-before.log"
echo "== start the build (background)"
"$W" build --driver firecracker --verbose --timeout 15m -o out-sec sec >"$L/31-sec-build.log" 2>&1 &
BPID=$!
for i in $(seq 1 120); do [ "$(pgrep -c firecracker)" -ge 2 ] && break; sleep 1; done
echo "firecracker processes: $(pgrep -c firecracker) after ${i}s; waiting 20 s for the guest to be up"
sleep 20
echo "== DISCOVER from the host on the slot's upstream TAP wfc0u (expect no offer)"
sudo -n "$D/dhcpprobe" -if wfc0u -wait 8s >"$L/32-probe-upstream.log" 2>&1; echo "rc=$?"; cat "$L/32-probe-upstream.log"
echo "== control: DISCOVER from the host on the relay-side TAP wfc0r (the responder's interface)"
sudo -n "$D/dhcpprobe" -if wfc0r -wait 8s >"$L/33-probe-relayside.log" 2>&1; echo "rc=$?"; cat "$L/33-probe-relayside.log"
echo "== slot interfaces"; ip -br link | grep wfc
wait $BPID; echo "build rc=$?"
grep -E "$FILTER" "$L/31-sec-build.log" | head -60
echo "== host chain counters after"; sudo -n iptables -L WARDEN-FC-FWD -v -n -x 2>/dev/null | head -20 | tee "$L/34-counters-after.log"
echo "== inspect out-sec"; "$W" inspect out-sec >"$L/35-inspect-sec.log" 2>&1; grep -iE "signature|channel|GET|CONNECT|complete" "$L/35-inspect-sec.log" | head -20

echo "== cleanup"
"$W" clean --driver firecracker 2>&1 | tail -3
sudo -n env PATH="$PATH" sh "$HOME/bwb/tools/firecracker-vm/slots.sh" down 2>&1 | tail -3
ip -br link | grep -c wfc || echo "0 wfc interfaces"
sudo -n iptables -S FORWARD | head -1
ls ~/.cache/warden/firecracker/work 2>&1 | head -3; pgrep -a firecracker || echo "no firecracker processes"
df -h / | tail -1
