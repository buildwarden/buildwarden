set -u
export PATH="$HOME/.local/bin:$PATH"
D=$HOME/fcval; W=$D/bin/warden; L=$D/logs; mkdir -p $L
cd $D
echo "== versions"; uname -r; firecracker --version | head -1; $W version 2>&1 | head -2
echo "== setup"; sudo env PATH="$PATH" $W setup --driver firecracker --slots 4 2>&1 | tee $L/setup.log | tail -4
echo "== doctor (as jeff)"; $W doctor --driver firecracker 2>&1 | tee $L/doctor.log
rm -rf direct ubuntu out-*
mkdir -p direct ubuntu
cat > direct/build.sh <<'S'
echo hello-from-direct-boot
uname -r
wget -q -O - http://example.com/ | head -c 60; echo
if nc -w 3 1.1.1.1 22 </dev/null; then echo EGRESS-OPEN-tcp22; else echo egress-blocked-tcp22; fi
S
echo "== direct-boot build"
s=$(date +%s.%N); $W build --driver firecracker --script build.sh --timeout 5m -o out-direct direct > $L/direct.log 2>&1; rc=$?; e=$(date +%s.%N)
echo "rc=$rc secs=$(echo "$e - $s" | bc)"; tail -12 $L/direct.log
echo hello-from-copy > ubuntu/hello.txt
cat > ubuntu/Dockerfile <<'S'
FROM ubuntu:24.04
COPY hello.txt /tmp/hello.txt
RUN cat /tmp/hello.txt
RUN curl -fsS -o /tmp/six.html https://pypi.org/simple/six/ && wc -c /tmp/six.html
RUN echo "trust-store certs: $(grep -c 'BEGIN CERTIFICATE' /etc/ssl/certs/ca-certificates.crt)"
RUN if timeout 5 bash -c '</dev/tcp/1.1.1.1/22'; then echo EGRESS-OPEN-tcp22; else echo egress-blocked-tcp22; fi
RUN if timeout 5 bash -c '</dev/tcp/8.8.8.8/853'; then echo EGRESS-OPEN-dot; else echo egress-blocked-dot; fi
S
echo "== ubuntu build"
s=$(date +%s.%N); $W build --driver firecracker --timeout 15m -o out-ubuntu ubuntu > $L/ubuntu.log 2>&1; rc=$?; e=$(date +%s.%N)
echo "rc=$rc secs=$(echo "$e - $s" | bc)"; grep -E "hello-from|six|trust-store|egress|EGRESS|SUCCEEDED|FAILED|ERROR" $L/ubuntu.log | head -20
for o in out-direct out-ubuntu; do echo "== inspect $o"; $W inspect $o 2>&1 | grep -E "SUMMARY|SIGNATURES|COMPLETENESS|GET|POST|CONNECT" | head -14; done
echo "== leftovers"; ls ~/.cache/warden/firecracker/work 2>&1 | head; pgrep -a firecracker || echo "no firecracker processes"
