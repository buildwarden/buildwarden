package firecracker

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/buildwarden/buildwarden/driver"
	"github.com/buildwarden/buildwarden/relaycfg"
)

// Network slots (Decision 4 of docs/design/firecracker-driver.md). `warden
// setup` makes them once as root; a build takes a free one under a host lock
// and needs no privilege beyond /dev/kvm and owning the slot's TAPs.
const (
	// MaxSlots is the most slots the upstream range holds (a /30 each).
	MaxSlots = 256
	// DefaultSlots is how many `warden setup --driver firecracker` makes.
	DefaultSlots = 16

	upstreamNet = "100.64.88.0/22"
	// The chains setup owns. It flushes and refills only these, so it never
	// touches another rule on the host.
	chainIn   = "WARDEN-FC-IN"
	chainFwd  = "WARDEN-FC-FWD"
	chainOut  = "WARDEN-FC-OUT"
	chainRaw  = "WARDEN-FC-RAW"
	chainNAT  = "WARDEN-FC-NAT"
	chainIPv6 = "WARDEN-FC6"
	nftTable  = "warden_fc"
)

// Slot is one durable network slot: a bridge joining the relay's build NIC to
// the build VM's only NIC, and the relay's upstream TAP.
type Slot int

// Bridge is the slot's address-less bridge.
func (s Slot) Bridge() string { return fmt.Sprintf("wfc%db", s) }

// RelayTap is the relay VM's build NIC (eth0), a bridge port.
func (s Slot) RelayTap() string { return fmt.Sprintf("wfc%dr", s) }

// GuestTap is the build VM's only NIC, the bridge's other port.
func (s Slot) GuestTap() string { return fmt.Sprintf("wfc%dg", s) }

// UpstreamTap is the relay VM's upstream NIC (eth1), routed and NATed by the
// host.
func (s Slot) UpstreamTap() string { return fmt.Sprintf("wfc%du", s) }

// HostAddr is the host's address on the upstream TAP's /30.
func (s Slot) HostAddr() string { return slotAddr(int(s), 1) }

// RelayAddr is the relay's upstream address, where the host claims it.
func (s Slot) RelayAddr() string { return slotAddr(int(s), 2) }

// ClaimAddr is the relay's claim endpoint, host:port.
func (s Slot) ClaimAddr() string {
	return s.RelayAddr() + ":" + strconv.Itoa(relaycfg.DefaultPort)
}

// LockName is the host lock a build holds on the slot.
func (s Slot) LockName() string { return fmt.Sprintf("firecracker-slot-%d", s) }

// slotAddr is 100.64.88.0 + 4i + off, carried into the third octet.
func slotAddr(i, off int) string {
	n := 4*i + off
	return fmt.Sprintf("100.64.%d.%d", 88+n/256, n%256)
}

// ValidateSlotCount checks a --slots value.
func ValidateSlotCount(n int) error {
	if n < 1 || n > MaxSlots {
		return driver.Fixable(fmt.Sprintf("pass --slots between 1 and %d", MaxSlots),
			"--slots %d is out of range (1-%d)", n, MaxSlots)
	}
	return nil
}

// step is one host command of setup. A step with probe is skipped when the
// probe succeeds (the resource exists and is verified, not recreated); ignore
// tolerates failure (removing what may not exist); untilFail repeats the
// command until it fails (deleting every copy of a jump).
type step struct {
	args      []string
	probe     []string
	ignore    bool
	untilFail bool
}

func cmd(args ...string) step { return step{args: args} }

func tolerant(args ...string) step { return step{args: args, ignore: true} }

// setupParams is what setup's command list depends on.
type setupParams struct {
	slots     int
	user      string // owner of the TAPs
	uid       int
	uplink    string // the default route's interface
	uplinkMTU int
}

// flushSteps remove every WARDEN-FC chain, the jumps into them and the bridge
// table, leaving all other rules as they were.
func flushSteps() []step {
	var out []step
	for _, c := range []struct{ table, hook, chain string }{
		{"filter", "INPUT", chainIn}, {"filter", "FORWARD", chainFwd},
		{"filter", "OUTPUT", chainOut}, {"raw", "PREROUTING", chainRaw},
		{"nat", "POSTROUTING", chainNAT},
	} {
		out = append(out,
			step{args: ipt("-t", c.table, "-D", c.hook, "-j", c.chain),
				untilFail: true},
			tolerant(ipt("-t", c.table, "-F", c.chain)...),
			tolerant(ipt("-t", c.table, "-X", c.chain)...))
	}
	for _, hook := range []string{"INPUT", "FORWARD", "OUTPUT"} {
		out = append(out, step{args: ip6t("-D", hook, "-j", chainIPv6), untilFail: true})
	}
	out = append(out,
		tolerant(ip6t("-F", chainIPv6)...),
		tolerant(ip6t("-X", chainIPv6)...),
		tolerant("nft", "delete", "table", "bridge", nftTable))
	return out
}

func ipt(args ...string) []string  { return append([]string{"iptables", "-w"}, args...) }
func ip6t(args ...string) []string { return append([]string{"ip6tables", "-w"}, args...) }

func sysctl(kv ...string) step { return cmd(append([]string{"sysctl", "-qw"}, kv...)...) }

// setupSteps is the whole of setup's host change, in order: its own rules
// flushed, the bridge table, the chains, each slot's interfaces, sysctls and
// rules, the catch-all drops, the jumps, and last ip_forward, so forwarding
// is on only once every drop is in place.
func setupSteps(p setupParams) []step {
	out := flushSteps()
	out = append(out,
		cmd("nft", "add", "table", "bridge", nftTable),
		cmd("nft", "add", "chain", "bridge", nftTable, "input",
			"{", "type", "filter", "hook", "input", "priority", "0", ";", "policy",
			"accept", ";", "}"),
		// No frame from a build segment reaches the host stack.
		cmd("nft", "add", "rule", "bridge", nftTable, "input", "iifname", "wfc*", "drop"),
		cmd("nft", "add", "chain", "bridge", nftTable, "forward",
			"{", "type", "filter", "hook", "forward", "priority", "0", ";",
			"policy", "accept", ";", "}"),
		cmd(ipt("-N", chainIn)...),
		cmd(ipt("-N", chainFwd)...),
		cmd(ipt("-N", chainOut)...),
		cmd(ipt("-t", "raw", "-N", chainRaw)...),
		cmd(ipt("-t", "nat", "-N", chainNAT)...),
		cmd(ip6t("-N", chainIPv6)...),
	)
	uid := strconv.Itoa(p.uid)
	for i := 0; i < p.slots; i++ {
		out = append(out, slotSteps(Slot(i), p, uid)...)
	}
	out = append(out,
		cmd("nft", "add", "rule", "bridge", nftTable, "forward", "iifname", "wfc*",
			"drop"),
		cmd("nft", "add", "rule", "bridge", nftTable, "forward", "oifname", "wfc*",
			"drop"),
		cmd(ipt("-A", chainIn, "-i", "wfc+", "-j", "DROP")...),
		cmd(ipt("-A", chainFwd, "-i", "wfc+", "-j", "DROP")...),
		cmd(ipt("-A", chainFwd, "-o", "wfc+", "-j", "DROP")...),
		cmd(ipt("-A", chainOut, "-o", "wfc+", "-d", upstreamNet,
			"-m", "conntrack", "--ctstate", "ESTABLISHED", "-j", "ACCEPT")...),
		// The host's ICMP errors (fragmentation needed) for a relay's own
		// flows, so path-MTU discovery works (a12).
		cmd(ipt("-A", chainOut, "-o", "wfc+", "-d", upstreamNet, "-p", "icmp",
			"-m", "conntrack", "--ctstate", "RELATED", "-j", "ACCEPT")...),
		cmd(ipt("-A", chainOut, "-o", "wfc+", "-j", "DROP")...),
		cmd(ipt("-t", "nat", "-A", chainNAT, "-s", upstreamNet, "-o", p.uplink, "-j",
			"MASQUERADE")...),
		cmd(ip6t("-A", chainIPv6, "-i", "wfc+", "-j", "DROP")...),
		cmd(ip6t("-A", chainIPv6, "-o", "wfc+", "-j", "DROP")...),
		cmd(ipt("-I", "INPUT", "1", "-j", chainIn)...),
		cmd(ipt("-I", "FORWARD", "1", "-j", chainFwd)...),
		cmd(ipt("-I", "OUTPUT", "1", "-j", chainOut)...),
		cmd(ipt("-t", "raw", "-I", "PREROUTING", "1", "-j", chainRaw)...),
		cmd(ipt("-t", "nat", "-I", "POSTROUTING", "1", "-j", chainNAT)...),
	)
	for _, hook := range []string{"INPUT", "FORWARD", "OUTPUT"} {
		out = append(out, cmd(ip6t("-I", hook, "1", "-j", chainIPv6)...))
	}
	return append(out, sysctl("net.ipv4.ip_forward=1"))
}

func slotSteps(s Slot, p setupParams, uid string) []step {
	b, r, g, u := s.Bridge(), s.RelayTap(), s.GuestTap(), s.UpstreamTap()
	host, relay := s.HostAddr(), s.RelayAddr()
	out := []step{
		{args: []string{"ip", "link", "add", b, "type", "bridge"},
			probe: []string{"ip", "link", "show", b}},
		sysctl("net.ipv6.conf." + b + ".disable_ipv6=1"),
	}
	for _, tap := range []string{r, g, u} {
		out = append(out,
			step{args: []string{"ip", "tuntap", "add", "dev", tap, "mode", "tap",
				"user", p.user},
				probe: []string{"ip", "link", "show", tap}},
			sysctl("net.ipv6.conf."+tap+".disable_ipv6=1"))
	}
	out = append(out,
		cmd("ip", "link", "set", r, "master", b),
		cmd("ip", "link", "set", g, "master", b),
		cmd("ip", "addr", "flush", "dev", u),
		cmd("ip", "addr", "add", host+"/30", "dev", u),
		sysctl("net.ipv4.conf."+b+".rp_filter=1", "net.ipv4.conf."+u+".rp_filter=1",
			"net.ipv4.conf."+b+".arp_ignore=8"),
	)
	for _, l := range []string{b, r, g, u} {
		out = append(out, cmd("ip", "link", "set", l, "up"))
	}
	mss := strconv.Itoa(p.uplinkMTU - 40)
	out = append(out,
		// Frames cross the bridge only between this slot's two ports.
		cmd("nft", "add", "rule", "bridge", nftTable, "forward", "iifname", r, "oifname",
			g, "accept"),
		cmd("nft", "add", "rule", "bridge", nftTable, "forward", "iifname", g, "oifname",
			r, "accept"),
		cmd(ipt("-A", chainIn, "-i", b, "-j", "DROP")...),
		cmd(ipt("-A", chainIn, "-i", u, "-s", relay,
			"-m", "conntrack", "--ctstate", "ESTABLISHED", "-j", "ACCEPT")...),
		// The uplink's MTU can be below the TAPs' 1500 (a VPN, WSL's 1280):
		// clamp the MSS both ways, since the relay sizes its segments from
		// the server's SYN-ACK (a12).
		cmd(ipt("-A", chainFwd, "-i", u, "-s", relay, "-o", p.uplink, "-p", "tcp",
			"--tcp-flags", "SYN,RST", "SYN", "-j", "TCPMSS",
			"--clamp-mss-to-pmtu")...),
		cmd(ipt("-A", chainFwd, "-i", p.uplink, "-o", u, "-d", relay, "-p", "tcp",
			"--tcp-flags", "SYN,RST", "SYN", "-m", "tcpmss", "--mss",
			strconv.Itoa(p.uplinkMTU-39)+":65535", "-j", "TCPMSS", "--set-mss",
			mss)...),
		cmd(ipt("-A", chainFwd, "-i", u, "-s", relay, "-o", p.uplink, "-j", "ACCEPT")...),
		cmd(ipt("-A", chainFwd, "-i", p.uplink, "-o", u, "-d", relay,
			"-m", "conntrack", "--ctstate", "ESTABLISHED,RELATED", "-j", "ACCEPT")...),
		// Anti-spoof: anything else from the relay's upstream is dropped, so
		// un-NATed build traffic never leaves even if the relay's rules fail.
		cmd(ipt("-A", chainFwd, "-i", u, "-j", "DROP")...),
		cmd(ipt("-A", chainFwd, "-i", b, "-o", b, "-j", "ACCEPT")...),
		// Only the slot owner reaches the relay, and only its claim port
		// (a10); the relay's other listeners are closed to the host.
		cmd(ipt("-A", chainOut, "-o", u, "-d", relay, "-p", "tcp",
			"--dport", strconv.Itoa(relaycfg.DefaultPort),
			"-m", "conntrack", "--ctstate", "NEW", "-m", "owner", "--uid-owner", uid,
			"-j", "ACCEPT")...),
		// Every slot reuses 10.0.0.1/10.0.0.2; a zone per bridge keeps their
		// flows apart in conntrack when br_netfilter is loaded.
		cmd(ipt("-t", "raw", "-A", chainRaw, "-i", b, "-j", "CT", "--zone",
			strconv.Itoa(int(s)+1))...),
	)
	return out
}

// uplinkFromRoute picks the interface of `ip route show default`.
func uplinkFromRoute(out string) string {
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		for i := 0; i+1 < len(f); i++ {
			if f[i] == "dev" {
				return f[i+1]
			}
		}
	}
	return ""
}

// forwardPolicyAccept reports whether `iptables -S FORWARD` shows an ACCEPT
// policy, under which enabling ip_forward would route between the host's
// other interfaces (a11).
func forwardPolicyAccept(iptablesS string) bool {
	for _, line := range strings.Split(iptablesS, "\n") {
		if strings.TrimSpace(line) == "-P FORWARD ACCEPT" {
			return true
		}
	}
	return false
}
