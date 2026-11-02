package firecracker

import (
	"errors"
	"fmt"
	"io"
	"net"
	"strings"

	"github.com/buildwarden/buildwarden/driver"
)

// RunSetup is `warden setup --driver firecracker`: as root, once per boot,
// it makes opts.Slots network slots owned by the user who ran sudo, with the
// bridge table and WARDEN-FC rules that isolate them (Decision 4). It is
// idempotent: existing interfaces are verified, warden's own rules reloaded.
func RunSetup(out io.Writer, opts SetupOptions) error {
	if err := hostSupported(); err != nil {
		return err
	}
	return setup(realHost{}, out, opts)
}

// Doctor is `warden doctor --driver firecracker`: every preflight item plus
// the slots' sysctls and rules, host posture, reflink support and orphaned
// VMMs, one line each. It changes nothing and returns true when a build
// cannot run.
func Doctor(out io.Writer) (blocked bool) {
	return doctor(realHost{}, out, hostLocal)
}

// report prints doctor's lines and tracks whether anything blocks a build.
type report struct {
	w       io.Writer
	blocked bool
}

func (r *report) line(status, what string, err error) {
	fmt.Fprintf(r.w, "%-5s %s\n", status, what)
	if err == nil {
		return
	}
	fmt.Fprintf(r.w, "      %v\n", err)
	var fe *driver.FixableError
	if errors.As(err, &fe) && fe.Fix != "" {
		fmt.Fprintf(r.w, "      to fix: %s\n", fe.Fix)
	}
}

// check reports err as a failure that blocks builds.
func (r *report) check(what string, err error) {
	if err != nil {
		r.blocked = true
		r.line("FAIL", what, err)
		return
	}
	r.line("ok", what, nil)
}

// warn reports err without blocking builds.
func (r *report) warn(what string, err error) {
	if err != nil {
		r.line("warn", what, err)
		return
	}
	r.line("ok", what, nil)
}

func doctor(h host, out io.Writer, local func(net.IP) bool) bool {
	r := &report{w: out}
	if err := hostCheck(); err != nil {
		r.check("host", err)
		return true
	}
	r.check("/dev/kvm", checkKVM(h))
	r.check("firecracker", checkFirecracker(h))
	_, err := findAssets()
	r.check("boot assets", err)
	r.warn("disk-image builds (qemu-img, genisoimage)", checkDiskTools(h))
	slots, err := checkSlots(h)
	r.check("network slots owned by this user: "+fmtSlots(slots), err)
	if len(slots) > 0 {
		r.check("slot sysctls", checkSysctls(h, slots))
	}
	if h.euid() == 0 {
		r.check("bridge table and WARDEN-FC rules", checkRules(h))
		r.check("host firewall posture", checkPosture(h))
	} else {
		r.check("firewalld", checkFirewalld(h))
		fmt.Fprintln(out, "skip  rules and FORWARD policy (reading them needs root: "+
			"sudo warden doctor --driver firecracker)")
	}
	dns, err := upstreamResolvers(h.readFile, local)
	r.check("upstream DNS for the relay: "+strings.Join(dns, ", "), err)
	r.warn("reflink copies on the cache filesystem", checkReflink(h))
	r.warn("orphaned VMMs", checkOrphans(h, slots))
	return r.blocked
}

func checkFirewalld(h host) error {
	if h.run([]string{"systemctl", "is-active", "-q", "firewalld"}) == nil {
		return driver.Fixable(firewalldFix, "firewalld is running")
	}
	return nil
}

// sysctlWant is one per-slot sysctl as setup sets it.
func sysctlWants(slots []Slot) map[string]string {
	want := map[string]string{"net/ipv4/ip_forward": "1"}
	for _, s := range slots {
		b, u := s.Bridge(), s.UpstreamTap()
		want["net/ipv4/conf/"+b+"/rp_filter"] = "1"
		want["net/ipv4/conf/"+u+"/rp_filter"] = "1"
		want["net/ipv4/conf/"+b+"/arp_ignore"] = "8"
		for _, l := range []string{b, s.RelayTap(), s.GuestTap(), u} {
			want["net/ipv6/conf/"+l+"/disable_ipv6"] = "1"
		}
	}
	return want
}

func checkSysctls(h host, slots []Slot) error {
	var bad []string
	for key, v := range sysctlWants(slots) {
		b, err := h.readFile("/proc/sys/" + key)
		if err != nil || strings.TrimSpace(string(b)) != v {
			bad = append(bad, strings.ReplaceAll(key, "/", "."))
		}
	}
	if len(bad) > 0 {
		return driver.Fixable(setupFix, "%d sysctl(s) not as setup sets them, e.g. %s",
			len(bad), minString(bad))
	}
	return nil
}

func minString(ss []string) string {
	m := ss[0]
	for _, s := range ss[1:] {
		if s < m {
			m = s
		}
	}
	return m
}

// ruleJumps are the jumps setup inserts first in each hooked chain.
var ruleJumps = []struct {
	cmd   func(...string) []string
	table string
	hook  string
	chain string
}{
	{ipt, "filter", "INPUT", chainIn}, {ipt, "filter", "FORWARD", chainFwd},
	{ipt, "filter", "OUTPUT", chainOut}, {ipt, "raw", "PREROUTING", chainRaw},
	{ipt, "nat", "POSTROUTING", chainNAT},
	{ip6t, "filter", "INPUT", chainIPv6}, {ip6t, "filter", "FORWARD", chainIPv6},
	{ip6t, "filter", "OUTPUT", chainIPv6},
}

// checkRules checks (as root) that the bridge table exists and each WARDEN-FC
// jump is the first rule of its chain.
func checkRules(h host) error {
	if err := h.run([]string{"nft", "list", "table", "bridge", nftTable}); err != nil {
		return driver.Fixable(setupFix, "the nft bridge table %s is missing", nftTable)
	}
	for _, j := range ruleJumps {
		out, err := h.output(j.cmd("-t", j.table, "-S", j.hook))
		if err != nil || firstRule(out) != "-A "+j.hook+" -j "+j.chain {
			return driver.Fixable(setupFix, "%s is not the first rule of %s %s",
				j.chain, j.table, j.hook)
		}
	}
	return nil
}

// firstRule is the first -A line of `iptables -S <chain>`.
func firstRule(out string) string {
	for _, l := range strings.Split(out, "\n") {
		if l = strings.TrimSpace(l); strings.HasPrefix(l, "-A ") {
			return l
		}
	}
	return ""
}

func checkReflink(h host) error {
	if err := h.reflink(WorkRoot()); err != nil {
		return fmt.Errorf("each disk-image build copies its whole rootfs (%v); "+
			"a cache on btrfs or XFS (reflink=1) makes it instant", err)
	}
	return nil
}

// checkOrphans reports VMMs recorded on a slot no build holds.
func checkOrphans(h host, slots []Slot) error {
	var found []string
	for _, s := range slots {
		unlock, ok, err := driver.TryHostLock(s.LockName())
		if err != nil || !ok {
			continue
		}
		b, err := h.readFile(pidFilePath(s))
		unlock()
		if err != nil {
			continue
		}
		for _, v := range liveVMMs(parsePidFile(string(b)), h.readFile, h.readlink) {
			what := fmt.Sprintf("slot %d pid %d", s, v.pid)
			if v.keep {
				what += " (--keep-vms)"
			}
			found = append(found, what)
		}
	}
	if len(found) > 0 {
		return driver.Fixable("warden clean --driver firecracker (the next build on the "+
			"slot "+
			"also kills them, unless --keep-vms left them)",
			"VMMs with no warden: %s", strings.Join(found, ", "))
	}
	return nil
}
