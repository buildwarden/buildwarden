package firecracker

import (
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/buildwarden/buildwarden/driver"
)

// host is what setup and doctor need from the machine, so their command lists
// and checks run in tier-0 tests against a fake (no root, no Linux).
type host interface {
	// run runs a command, its output folded into the error on failure.
	run(args []string) error
	// output runs a command and returns its stdout.
	output(args []string) (string, error)
	readFile(path string) ([]byte, error)
	readlink(path string) (string, error)
	exists(path string) bool
	// reflink reports whether a file in dir can be copied as a reflink.
	reflink(dir string) error
	// openRW reports whether path opens for reading and writing (/dev/kvm).
	openRW(path string) error
	lookPath(name string) error
	getenv(key string) string
	// lookupOwner returns a user's uid and whether it is in the kvm group.
	lookupOwner(name string) (uid int, inKVM bool, err error)
	// userHome returns a user's home directory.
	userHome(name string) (string, error)
	// tryLock takes the host lock held in the existing file path without
	// waiting. It never creates the file, so a root warden leaves nothing in a
	// user's lock dir: a missing file is free (a no-op unlock). held is true
	// when another process holds the lock.
	tryLock(path string) (unlock func(), held bool, err error)
	euid() int
}

// SetupOptions configures RunSetup.
type SetupOptions struct {
	// Slots is how many network slots to make (1 to MaxSlots; 0 means
	// DefaultSlots).
	Slots int
}

// owner is the user builds run as, who owns the slots' TAPs.
type owner struct {
	name string
	uid  int
}

// setupOwner is the user who ran `sudo warden setup`, checked to be in the kvm
// group, since builds then need /dev/kvm and nothing else.
func setupOwner(h host) (owner, error) {
	name := h.getenv("SUDO_USER")
	if name == "" || name == "root" {
		return owner{}, driver.Fixable(
			"run it with sudo as the user builds run as: sudo warden setup --driver "+
				"firecracker",
			"warden setup --driver firecracker gives the slots to the user who ran "+
				"sudo, "+
				"and there is none")
	}
	uid, inKVM, err := h.lookupOwner(name)
	if err != nil {
		return owner{}, err
	}
	if !inKVM {
		return owner{}, driver.Fixable(
			fmt.Sprintf("sudo usermod -aG kvm %s, then log in again", name),
			"%s is not in the kvm group, so builds cannot open /dev/kvm", name)
	}
	return owner{name: name, uid: uid}, nil
}

const kvmFix = "enable KVM: load kvm_intel or kvm_amd (under WSL2, set " +
	"nestedVirtualization=true in .wslconfig)"

// requiredTools are the commands setup runs.
var requiredTools = []string{"ip", "nft", "iptables", "ip6tables", "sysctl"}

// checkPosture refuses the host firewall states setup cannot make safe
// (a11): a running firewalld, an ACCEPT FORWARD policy, or an nftables
// forward chain that would drop the relays' traffic. Each names its fix.
func checkPosture(h host) error {
	for _, t := range requiredTools {
		if err := h.lookPath(t); err != nil {
			return driver.Fixable("install it (iproute2, nftables, iptables, procps)",
				"%s is missing", t)
		}
	}
	if h.run([]string{"systemctl", "is-active", "-q", "firewalld"}) == nil {
		return driver.Fixable(firewalldFix, "firewalld is running")
	}
	fwd, err := h.output(ipt("-S", "FORWARD"))
	if err != nil {
		return fmt.Errorf("reading the FORWARD policy: %w", err)
	}
	if forwardPolicyAccept(fwd) {
		return driver.Fixable(forwardPolicyFix,
			"the filter FORWARD policy is ACCEPT, so enabling ip_forward would route "+
				"between this host's other interfaces")
	}
	if out, err := h.output([]string{"nft", "-j", "list", "chains"}); err == nil {
		drops, err := nftForwardDrops([]byte(out))
		if err != nil {
			return err
		}
		if len(drops) > 0 {
			return driver.Fixable(nftDropFix(drops),
				"an nftables forward chain drops by default and would drop the "+
					"relays' "+
					"traffic: %s %s %s", drops[0].Family, drops[0].Table,
				drops[0].Name)
		}
	}
	return nil
}

// uplinkParams finds the default route's interface and its MTU.
func uplinkParams(h host) (string, int, error) {
	route, err := h.output([]string{"ip", "route", "show", "default"})
	if err != nil {
		return "", 0, fmt.Errorf("reading the default route: %w", err)
	}
	up := uplinkFromRoute(route)
	if up == "" {
		return "", 0, driver.Fixable("give the host a default route to the network "+
			"builds fetch from",
			"the host has no default route")
	}
	b, err := h.readFile("/sys/class/net/" + up + "/mtu")
	if err != nil {
		return "", 0, fmt.Errorf("reading %s's MTU: %w", up, err)
	}
	mtu, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil {
		return "", 0, fmt.Errorf("parsing %s's MTU: %w", up, err)
	}
	return up, mtu, nil
}

// setup is RunSetup against h: checks, then the command list.
func setup(h host, out io.Writer, opts SetupOptions) error {
	n := opts.Slots
	if n == 0 {
		n = DefaultSlots
	}
	if err := ValidateSlotCount(n); err != nil {
		return err
	}
	if h.euid() != 0 {
		return driver.Fixable("sudo warden setup --driver firecracker",
			"warden setup --driver firecracker changes host networking and must run "+
				"as root")
	}
	if !h.exists("/dev/kvm") {
		return driver.Fixable(kvmFix, "/dev/kvm is not present")
	}
	o, err := setupOwner(h)
	if err != nil {
		return err
	}
	if err := checkPosture(h); err != nil {
		return err
	}
	up, mtu, err := uplinkParams(h)
	if err != nil {
		return err
	}
	release, err := quiesceSlots(h, o)
	if err != nil {
		return err
	}
	defer release()
	created, verified, err := runSteps(h, setupSteps(setupParams{
		slots: n, user: o.name, uid: o.uid, uplink: up, uplinkMTU: mtu,
		addrOK: upstreamAddrsOK(h, n),
	}))
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "firecracker: %d slot(s) for %s (uid %d), uplink %s (mtu %d)\n",
		n, o.name, o.uid, up, mtu)
	fmt.Fprintf(out, "  interfaces created: %d, verified: %d\n", len(created), len(verified))
	fmt.Fprintf(out, "  bridge table %s and the %s chains (re)loaded; net.ipv4.ip_forward=1\n",
		nftTable, "WARDEN-FC")
	fmt.Fprintln(out,
		"  this is runtime state: re-run setup after a reboot (see the drivers guide)")
	return nil
}

const slotsBusyFix = "wait for the builds on them to finish, or stop VMs left by " +
	"--keep-vms (warden clean --driver firecracker, as the build user), then " +
	"re-run sudo warden setup --driver firecracker"

// quiesceSlots refuses setup while any slot is in use, and otherwise holds the
// owner's slot locks until setup is done so no build takes a slot meanwhile.
// Setup reloads every slot's rules, so a slot in use would run unfiltered for
// the reload (GLOBAL-06). A slot is in use when a process has one of its TAPs
// attached (the kernel reports carrier on a TAP only then), which finds
// running builds, --keep-vms VMs and orphans whoever started them; or when a
// build holds its lock before its VMMs start.
func quiesceSlots(h host, o owner) (release func(), err error) {
	dir, err := ownerLockDir(h, o.name)
	if err != nil {
		return nil, err
	}
	var unlocks []func()
	release = func() {
		for _, u := range unlocks {
			u()
		}
	}
	var busy []string
	for i := 0; i < MaxSlots; i++ {
		s := Slot(i)
		if !h.exists("/sys/class/net/" + s.UpstreamTap()) {
			continue
		}
		if tapAttached(h, s) {
			busy = append(busy, strconv.Itoa(i))
			continue
		}
		unlock, held, err := h.tryLock(filepath.Join(dir, s.LockName()+".lock"))
		if err != nil {
			release()
			return nil, err
		}
		if held {
			busy = append(busy, strconv.Itoa(i))
			continue
		}
		unlocks = append(unlocks, unlock)
	}
	if len(busy) > 0 {
		release()
		return nil, driver.Fixable(slotsBusyFix,
			"firecracker slot(s) %s are in use, and re-running setup now would drop "+
				"their isolation rules while it reloads them",
			strings.Join(busy, ", "))
	}
	return release, nil
}

// tapAttached reports whether a VMM has one of s's TAPs open.
func tapAttached(h host, s Slot) bool {
	for _, tap := range []string{s.RelayTap(), s.GuestTap(), s.UpstreamTap()} {
		b, err := h.readFile("/sys/class/net/" + tap + "/carrier")
		if err == nil && strings.TrimSpace(string(b)) == "1" {
			return true
		}
	}
	return false
}

// ownerLockDir is driver.LockDir as the user name sees it. Under sudo HOME is
// usually root's, so it is found from the user's home; XDG_CACHE_HOME counts
// only when HOME is still the user's (sudo -E).
func ownerLockDir(h host, name string) (string, error) {
	home, err := h.userHome(name)
	if err != nil {
		return "", err
	}
	cache := filepath.Join(home, ".cache")
	if x := h.getenv("XDG_CACHE_HOME"); x != "" && h.getenv("HOME") == home {
		cache = x
	}
	return filepath.Join(cache, "buildwarden", "locks"), nil
}

// upstreamAddrsOK finds the slots whose upstream TAP already has exactly its
// host address, so re-running setup does not flush it.
func upstreamAddrsOK(h host, n int) map[Slot]bool {
	ok := map[Slot]bool{}
	for i := 0; i < n; i++ {
		s := Slot(i)
		out, err := h.output([]string{"ip", "-o", "-4", "addr", "show", "dev",
			s.UpstreamTap()})
		if err == nil && onlyAddr(out, s.HostAddr()+"/30") {
			ok[s] = true
		}
	}
	return ok
}

// runSteps runs a command list, returning the probed resources it created and
// the ones it found present.
func runSteps(h host, steps []step) (created, verified []string, err error) {
	for _, s := range steps {
		if s.probe != nil {
			if h.run(s.probe) == nil {
				verified = append(verified, s.probe[len(s.probe)-1])
				continue
			}
			created = append(created, s.probe[len(s.probe)-1])
		}
		if s.untilFail {
			for i := 0; i < 64 && h.run(s.args) == nil; i++ {
			}
			continue
		}
		if err := h.run(s.args); err != nil && !s.ignore {
			return nil, nil, fmt.Errorf("setup: %s: %w", strings.Join(s.args, " "),
				err)
		}
	}
	return created, verified, nil
}
