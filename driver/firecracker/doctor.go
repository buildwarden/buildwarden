package firecracker

import (
	"errors"
	"fmt"
	"io"
	"net"
	"path/filepath"
	"strings"

	"github.com/buildwarden/buildwarden/driver"
	"github.com/buildwarden/buildwarden/driver/buildlink"
)

// RunSetup is `warden setup --driver firecracker`: as root, once per boot,
// it makes opts.Slots network slots owned by the user who ran sudo, with the
// bridge table and WARDEN-FC rules that isolate them (Decision 4). It is
// idempotent: existing interfaces are verified, warden's own rules reloaded.
// It refuses to run while any slot is in use, since the reload would leave
// that slot's VMs unfiltered.
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
func Doctor(out io.Writer, link buildlink.Link) (blocked bool) {
	return doctor(realHost{}, out, hostLocal, link)
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

func doctor(h host, out io.Writer, local func(net.IP) bool, link buildlink.Link) bool {
	r := &report{w: out}
	if err := hostCheck(); err != nil {
		r.check("host", err)
		return true
	}
	r.check("/dev/kvm", checkBuildKVM(h))
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
	dns, err := upstreamResolvers(h.readFile, local, link)
	r.check("upstream DNS for the relay: "+strings.Join(dns, ", "), err)
	if name, ok := sudoUser(h); ok {
		// The probe would write as root into the user's cache.
		fmt.Fprintf(out, "skip  reflink copies on the cache filesystem (probing them "+
			"as root would write into %s's cache: run doctor without sudo)\n", name)
	} else {
		r.warn("reflink copies on the cache filesystem", checkReflink(h))
	}
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

// sudoUser is the user who ran sudo when warden runs as root under it: the
// user builds run as, whose slots, locks and cache doctor checks.
func sudoUser(h host) (string, bool) {
	name := h.getenv("SUDO_USER")
	return name, h.euid() == 0 && name != "" && name != "root"
}

// checkBuildKVM is checkKVM for the user builds run as. Root opens /dev/kvm
// whatever its group, so under sudo the user's kvm membership is checked.
func checkBuildKVM(h host) error {
	name, ok := sudoUser(h)
	if !ok {
		return checkKVM(h)
	}
	if !h.exists("/dev/kvm") {
		return driver.Fixable(kvmFix, "/dev/kvm is not present")
	}
	_, inKVM, err := h.lookupOwner(name)
	if err != nil {
		return err
	}
	if !inKVM {
		return driver.Fixable(fmt.Sprintf("sudo usermod -aG kvm %s, then log in again",
			name), "%s is not in the kvm group, so builds cannot open /dev/kvm", name)
	}
	return nil
}

// buildLockDir is the lock dir of the user builds run as (driver.LockDir, or
// under sudo the user's own).
func buildLockDir(h host) (string, error) {
	if name, ok := sudoUser(h); ok {
		return ownerLockDir(h, name)
	}
	return driver.LockDir(), nil
}

// checkOrphans reports VMMs recorded on a slot no build holds. It reads the
// build user's pid files and creates no lock file, so under sudo it leaves
// nothing root-owned in the user's lock dir.
func checkOrphans(h host, slots []Slot) error {
	dir, err := buildLockDir(h)
	if err != nil {
		return err
	}
	var found []string
	for _, s := range slots {
		unlock, held, err := h.tryLock(filepath.Join(dir, s.LockName()+".lock"))
		if err != nil || held {
			continue
		}
		b, err := h.readFile(filepath.Join(dir, s.LockName()+".pids"))
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
