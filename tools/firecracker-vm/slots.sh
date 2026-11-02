#!/bin/sh
# Creates or removes the Firecracker network slots of Decision 4 in
# docs/design/firecracker-driver.md. Spike stand-in for
# `warden setup --driver firecracker`; it goes when that lands.
#
# Root only. Installed root-owned (never run from a user-writable path under
# sudo) as /usr/local/sbin/warden-fc-slots, so a scoped NOPASSWD sudoers entry
# lets the warden user recreate slots after a host or WSL restart:
#     sudo install -o root -g root -m 0755 tools/firecracker-vm/slots.sh \
#         /usr/local/sbin/warden-fc-slots
#
# The TAPs belong to the user who ran sudo (SUDO_USER); run directly as root,
# pass --user. Host posture is the operator's: a filter FORWARD policy of
# ACCEPT or a running firewalld makes `up` refuse and print the fix (a11).
#
# Usage: warden-fc-slots up [N] [--user U]   (N slots, 1-256, default 16)
#        warden-fc-slots down
#        warden-fc-slots status

set -eu

NET=100.64.88.0/22
PATH=/usr/sbin:/usr/bin:/sbin:/bin
export PATH

die() {
	echo "warden-fc-slots: $*" >&2
	exit 1
}

usage() {
	die "usage: warden-fc-slots up [N] [--user U] | down | status"
}

[ "$(id -u)" = 0 ] || die "must run as root"

cmd="${1:-}"
[ $# -gt 0 ] && shift
slots=16
user="${SUDO_USER:-}"
case "$cmd" in
up)
	while [ $# -gt 0 ]; do
		case "$1" in
		--user)
			# Only root itself may name another owner; under sudo it is SUDO_USER.
			[ -z "${SUDO_USER:-}" ] || die "--user is not allowed under sudo"
			[ $# -ge 2 ] || usage
			user="$2"
			shift 2
			;;
		*)
			case "$1" in '' | *[!0-9]*) usage ;; esac
			slots="$1"
			shift
			;;
		esac
	done
	[ "$slots" -ge 1 ] && [ "$slots" -le 256 ] || die "N must be 1-256"
	[ -n "$user" ] || die "no owner: run under sudo, or pass --user"
	uid="$(id -u "$user")" || die "no such user: $user"
	;;
down | status) [ $# -eq 0 ] || usage ;;
*) usage ;;
esac

# addr I OFFSET: 100.64.88.0 + 4I + OFFSET, carried into the third octet.
addr() {
	n=$((4 * $1 + $2))
	echo "100.64.$((88 + n / 256)).$((n % 256))"
}

uplink() {
	ip route show default | awk '{ for (i = 1; i < NF; i++) if ($i == "dev") { print $(i + 1); exit } }'
}

ipt() { iptables -w "$@"; }
ip6t() { ip6tables -w "$@"; }

# Removes every WARDEN-FC chain and the jumps into it.
flush_rules() {
	for t in "filter INPUT WARDEN-FC-IN" "filter FORWARD WARDEN-FC-FWD" \
		"filter OUTPUT WARDEN-FC-OUT" "raw PREROUTING WARDEN-FC-RAW" \
		"nat POSTROUTING WARDEN-FC-NAT"; do
		set -- $t
		while ipt -t "$1" -D "$2" -j "$3" 2>/dev/null; do :; done
		ipt -t "$1" -F "$3" 2>/dev/null || true
		ipt -t "$1" -X "$3" 2>/dev/null || true
	done
	for c in INPUT FORWARD OUTPUT; do
		while ip6t -D "$c" -j WARDEN-FC6 2>/dev/null; do :; done
	done
	ip6t -F WARDEN-FC6 2>/dev/null || true
	ip6t -X WARDEN-FC6 2>/dev/null || true
	nft delete table bridge warden_fc 2>/dev/null || true
}

down() {
	flush_rules
	for l in $(ip -o link show | awk -F': ' '{ sub(/@.*/, "", $2); print $2 }' | grep -E '^wfc[0-9]+[rgub]$' || true); do
		ip link del "$l" 2>/dev/null || true
	done
}

posture() {
	for c in ip nft iptables ip6tables; do
		command -v "$c" >/dev/null || die "$c is missing"
	done
	if command -v systemctl >/dev/null && systemctl is-active -q firewalld 2>/dev/null; then
		die "firewalld is running; put the upstream TAPs in the trusted zone first:
    firewall-cmd --permanent --zone=trusted --add-interface=wfc+u && firewall-cmd --reload"
	fi
	if ipt -S FORWARD | grep -qx -- '-P FORWARD ACCEPT'; then
		die "the filter FORWARD policy is ACCEPT, so enabling ip_forward would route
between this host's other interfaces. Set it to DROP (and keep it across reboots):
    iptables -P FORWARD DROP && ip6tables -P FORWARD DROP"
	fi
}

tap() {
	ip link show "$1" >/dev/null 2>&1 || ip tuntap add dev "$1" mode tap user "$user"
	sysctl -qw "net.ipv6.conf.$1.disable_ipv6=1"
}

up() {
	posture
	up_if="$(uplink)"
	[ -n "$up_if" ] || die "no default route"
	up_mtu="$(cat "/sys/class/net/$up_if/mtu")"
	flush_rules

	nft -f - <<-EOF
		table bridge warden_fc {
			chain input {
				type filter hook input priority 0; policy accept;
				iifname "wfc*" drop
			}
			chain forward {
				type filter hook forward priority 0; policy accept;
			}
		}
	EOF

	ipt -N WARDEN-FC-IN
	ipt -N WARDEN-FC-FWD
	ipt -N WARDEN-FC-OUT
	ipt -t raw -N WARDEN-FC-RAW
	ipt -t nat -N WARDEN-FC-NAT
	ip6t -N WARDEN-FC6

	i=0
	while [ "$i" -lt "$slots" ]; do
		b="wfc${i}b" r="wfc${i}r" g="wfc${i}g" u="wfc${i}u"
		host="$(addr "$i" 1)" relay="$(addr "$i" 2)"

		ip link show "$b" >/dev/null 2>&1 || ip link add "$b" type bridge
		sysctl -qw "net.ipv6.conf.$b.disable_ipv6=1"
		tap "$r"
		tap "$g"
		tap "$u"
		ip link set "$r" master "$b"
		ip link set "$g" master "$b"
		ip addr flush dev "$u"
		ip addr add "$host/30" dev "$u"
		sysctl -qw "net.ipv4.conf.$b.rp_filter=1" "net.ipv4.conf.$u.rp_filter=1" \
			"net.ipv4.conf.$b.arp_ignore=8"
		for l in "$b" "$r" "$g" "$u"; do ip link set "$l" up; done

		nft add rule bridge warden_fc forward iifname "$r" oifname "$g" accept
		nft add rule bridge warden_fc forward iifname "$g" oifname "$r" accept

		ipt -A WARDEN-FC-IN -i "$b" -j DROP
		ipt -A WARDEN-FC-IN -i "$u" -s "$relay" -m conntrack --ctstate ESTABLISHED -j ACCEPT

		# The uplink's MTU can be below the TAPs' 1500 (a VPN, WSL's 1280):
		# clamp the MSS both ways, since the relay sizes its segments from the
		# server's SYN-ACK, and let ICMP errors (RELATED) back for PMTU discovery.
		ipt -A WARDEN-FC-FWD -i "$u" -s "$relay" -o "$up_if" -p tcp \
			--tcp-flags SYN,RST SYN -j TCPMSS --clamp-mss-to-pmtu
		ipt -A WARDEN-FC-FWD -i "$up_if" -o "$u" -d "$relay" -p tcp \
			--tcp-flags SYN,RST SYN -m tcpmss --mss "$((up_mtu - 39)):65535" \
			-j TCPMSS --set-mss "$((up_mtu - 40))"
		ipt -A WARDEN-FC-FWD -i "$u" -s "$relay" -o "$up_if" -j ACCEPT
		ipt -A WARDEN-FC-FWD -i "$up_if" -o "$u" -d "$relay" -m conntrack --ctstate ESTABLISHED,RELATED -j ACCEPT
		ipt -A WARDEN-FC-FWD -i "$u" -j DROP
		ipt -A WARDEN-FC-FWD -i "$b" -o "$b" -j ACCEPT

		ipt -A WARDEN-FC-OUT -o "$u" -d "$relay" -p tcp --dport 8301 \
			-m conntrack --ctstate NEW -m owner --uid-owner "$uid" -j ACCEPT

		ipt -t raw -A WARDEN-FC-RAW -i "$b" -j CT --zone "$((i + 1))"
		i=$((i + 1))
	done

	nft add rule bridge warden_fc forward iifname "wfc*" drop
	nft add rule bridge warden_fc forward oifname "wfc*" drop

	ipt -A WARDEN-FC-IN -i wfc+ -j DROP
	ipt -A WARDEN-FC-FWD -i wfc+ -j DROP
	ipt -A WARDEN-FC-FWD -o wfc+ -j DROP
	ipt -A WARDEN-FC-OUT -o wfc+ -d "$NET" -m conntrack --ctstate ESTABLISHED -j ACCEPT
	# The host's ICMP errors (fragmentation needed) for a relay's own flows.
	ipt -A WARDEN-FC-OUT -o wfc+ -d "$NET" -p icmp -m conntrack --ctstate RELATED -j ACCEPT
	ipt -A WARDEN-FC-OUT -o wfc+ -j DROP
	ipt -t nat -A WARDEN-FC-NAT -s "$NET" -o "$up_if" -j MASQUERADE
	ip6t -A WARDEN-FC6 -i wfc+ -j DROP
	ip6t -A WARDEN-FC6 -o wfc+ -j DROP

	ipt -I INPUT 1 -j WARDEN-FC-IN
	ipt -I FORWARD 1 -j WARDEN-FC-FWD
	ipt -I OUTPUT 1 -j WARDEN-FC-OUT
	ipt -t raw -I PREROUTING 1 -j WARDEN-FC-RAW
	ipt -t nat -I POSTROUTING 1 -j WARDEN-FC-NAT
	for c in INPUT FORWARD OUTPUT; do ip6t -I "$c" 1 -j WARDEN-FC6; done

	# Only once the drops are in place.
	sysctl -qw net.ipv4.ip_forward=1
	echo "warden-fc-slots: $slots slot(s) for $user (uid $uid), uplink $up_if (mtu $up_mtu)"
}

status() {
	ip -br link show | grep -E '^wfc[0-9]+[rgub][ @]' || echo "no slots"
	nft list table bridge warden_fc 2>/dev/null || echo "no bridge table"
	ipt -S WARDEN-FC-IN 2>/dev/null | head -3 || true
}

"$cmd"
