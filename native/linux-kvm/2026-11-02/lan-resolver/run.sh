#!/bin/sh
# firecracker-distro-lan-resolver (backlog, host:linux-kvm), run in WSL2 Ubuntu
# by the windows-desktop native session. The host's /etc/resolv.conf is never
# changed: each case runs warden in a private mount namespace (unshare -m,
# propagation private) with a temporary resolv.conf bind-mounted over
# /etc/resolv.conf, as the unprivileged user jeff (runuser), so the preflight
# sees a host whose only resolver is one of its own LAN addresses.
#
# Usage: sh lan-resolver.sh <checkout-in-wsl> <log-dir-in-wsl>
set -u
CO="$1"
LOG="$2"
BIN="$CO/.scratch/main/dist/warden"
mkdir -p "$LOG"
USERPATH="/home/jeff/.local/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
export PATH="$USERPATH"

log() { echo "== $*"; }

# in_ns runs "$@" as jeff in a private mount namespace where /etc/resolv.conf is
# the file $RESOLV. Nothing outside the namespace sees the bind mount.
in_ns() {
	sudo -n unshare -m --propagation private sh -c '
		R="$0"; P="$1"; shift
		mount --bind "$R" /etc/resolv.conf || exit 97
		exec runuser -u jeff -- env -i HOME=/home/jeff USER=jeff LOGNAME=jeff \
			PATH="$P" TERM=dumb "$@"
	' "$RESOLV" "$USERPATH" "$@"
}

LAN=$(ip -4 -o addr show eth0 | awk '{print $4}' | cut -d/ -f1)
{
	log "date"; date -u +%Y-%m-%dT%H:%M:%SZ
	log "host"; uname -r; head -1 /etc/os-release; id
	log "firecracker"; firecracker --version | head -1
	log "warden"; "$BIN" version 2>&1 || "$BIN" --version 2>&1
	log "assets beside the binary"; ls -la "$(dirname "$BIN")"; sha256sum "$(dirname "$BIN")"/firecracker-*
	log "host addresses"; ip -4 -br addr
	log "host /etc/resolv.conf (unchanged throughout)"; cat /etc/resolv.conf
	log "LAN address used as the only resolver: $LAN"
	log "FORWARD policy"; sudo -n iptables -S FORWARD | head -1
	log "firewalld"; systemctl is-active firewalld
} >"$LOG/00-host.log" 2>&1
cat "$LOG/00-host.log"

log "setup --slots 1"
sudo -n "$BIN" setup --driver firecracker --slots 1 >"$LOG/01-setup.log" 2>&1
echo "exit $?" >>"$LOG/01-setup.log"
cat "$LOG/01-setup.log"

log "doctor, real resolv.conf (baseline)"
"$BIN" doctor --driver firecracker >"$LOG/02-doctor-baseline.log" 2>&1
echo "exit $?" >>"$LOG/02-doctor-baseline.log"
cat "$LOG/02-doctor-baseline.log"

# Case 1: the only resolver is one of the host's own LAN addresses.
RESOLV=/tmp/warden-lan-only.conf
printf 'nameserver %s\n' "$LAN" >"$RESOLV"
log "case 1: doctor with resolv.conf = only $LAN"
in_ns "$BIN" doctor --driver firecracker >"$LOG/03-doctor-lan-only.log" 2>&1
echo "exit $?" >>"$LOG/03-doctor-lan-only.log"
cat "$LOG/03-doctor-lan-only.log"

CTX=/tmp/warden-lan-ctx
rm -rf "$CTX"; mkdir -p "$CTX"
printf '#!/bin/sh\necho hello from the build VM\nuname -r\n' >"$CTX/build.sh"
log "case 1: build with resolv.conf = only $LAN (expect a preflight refusal)"
rm -f "$CTX/.resolv-seen"
before=$(pgrep -c firecracker || true)
rm -rf /tmp/warden-lan-out-1
in_ns "$BIN" build --driver firecracker --script "$CTX/build.sh" -o /tmp/warden-lan-out-1 "$CTX" >"$LOG/04-build-lan-only.log" 2>&1
echo "exit $?" >>"$LOG/04-build-lan-only.log"
after=$(pgrep -c firecracker || true)
echo "firecracker processes before=$before after=$after" >>"$LOG/04-build-lan-only.log"
cat "$LOG/04-build-lan-only.log"

# Case 2: the LAN address first, an off-host resolver second: the LAN one is
# excluded and the build proceeds on 8.8.8.8.
RESOLV=/tmp/warden-lan-mixed.conf
printf 'nameserver %s\nnameserver 8.8.8.8\n' "$LAN" >"$RESOLV"
log "case 2: doctor with resolv.conf = $LAN then 8.8.8.8"
in_ns "$BIN" doctor --driver firecracker >"$LOG/05-doctor-lan-mixed.log" 2>&1
echo "exit $?" >>"$LOG/05-doctor-lan-mixed.log"
cat "$LOG/05-doctor-lan-mixed.log"

# Control: the same namespace mechanism with the host's real resolvers, so a
# refusal above is the resolver check and not the namespace.
RESOLV=/tmp/warden-real.conf
cp /etc/resolv.conf "$RESOLV"
log "control: direct-boot build in the namespace with the real resolvers"
out=/tmp/warden-lan-out-ctl
rm -rf "$out"
in_ns "$BIN" build --driver firecracker --script "$CTX/build.sh" -o "$out" "$CTX" >"$LOG/06-build-control.log" 2>&1
echo "exit $?" >>"$LOG/06-build-control.log"
cat "$LOG/06-build-control.log"
if [ -d "$out" ]; then
	log "control: inspect $out"
	"$BIN" inspect "$out" >"$LOG/07-inspect-control.log" 2>&1
	echo "exit $?" >>"$LOG/07-inspect-control.log"
	tail -15 "$LOG/07-inspect-control.log"
	log "control: outputs"; ls -la "$out" >>"$LOG/07-inspect-control.log" 2>&1
fi

{
	log "host /etc/resolv.conf after the cases"; cat /etc/resolv.conf
	log "mounts on /etc/resolv.conf in the host namespace"; grep -c ' /etc/resolv.conf ' /proc/self/mountinfo || true
	log "firecracker processes"; pgrep -a firecracker || echo none
} >"$LOG/08-after.log" 2>&1
cat "$LOG/08-after.log"

log "cleanup"
"$BIN" clean --driver firecracker >"$LOG/09-cleanup.log" 2>&1
sudo -n sh "$CO/tools/firecracker-vm/slots.sh" down >>"$LOG/09-cleanup.log" 2>&1
echo "slots down exit $?" >>"$LOG/09-cleanup.log"
{
	echo "wfc interfaces left: $(ls /sys/class/net | grep -c '^wfc')"
	sudo -n iptables -S FORWARD | head -1
	rm -rf "$CTX" /tmp/warden-lan-only.conf /tmp/warden-lan-mixed.conf /tmp/warden-real.conf \
		/tmp/warden-lan-out-1 /tmp/warden-lan-out-ctl
	echo "disk:"; df -h / | tail -1
} >>"$LOG/09-cleanup.log" 2>&1
cat "$LOG/09-cleanup.log"
