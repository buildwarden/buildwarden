package firecracker

import (
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/buildwarden/buildwarden/driver"
	"github.com/buildwarden/buildwarden/driver/relayvm"
)

// minVersion is the pinned Firecracker release (Phase 0 spike): it has the
// entropy device the relay needs and is the version the driver is validated
// on.
var minVersion = [3]int{1, 17, 0}

const firecrackerFix = "install Firecracker v1.17.0 or later from " +
	"https://github.com/firecracker-microvm/firecracker/releases and put " +
	"`firecracker` on PATH"

const assetsFix = "build them from the source tree: tools/firecracker-vm/build-kernel.sh, " +
	"ARCH=x86_64 NO_MODULES=1 tools/relay-vm/build-initramfs.sh and " +
	"tools/firecracker-vm/build-initramfs.sh"

const setupFix = "sudo warden setup --driver firecracker (re-run it after a reboot)"

// assets are the driver's boot files (relayvm.Firecracker).
type assets struct {
	Kernel         string
	RelayInitramfs string
	BuildInitramfs string
}

func findAssets() (assets, error) {
	p, ok := relayvm.Find(relayvm.Firecracker)
	if !ok {
		return assets{}, driver.Fixable(assetsFix,
			"firecracker boot assets (vmlinux, relay and direct-boot initramfs) not "+
				"found")
	}
	return assets{Kernel: p[0], RelayInitramfs: p[1], BuildInitramfs: p[2]}, nil
}

// parseVersion reads `firecracker --version` ("Firecracker v1.17.0").
func parseVersion(out string) ([3]int, bool) {
	var v [3]int
	for _, f := range strings.Fields(out) {
		if !strings.HasPrefix(f, "v") {
			continue
		}
		parts := strings.SplitN(strings.TrimPrefix(f, "v"), ".", 3)
		if len(parts) != 3 {
			continue
		}
		ok := true
		for i, p := range parts {
			n, err := strconv.Atoi(strings.SplitN(p, "-", 2)[0])
			if err != nil {
				ok = false
				break
			}
			v[i] = n
		}
		if ok {
			return v, true
		}
	}
	return v, false
}

func versionAtLeast(v, min [3]int) bool {
	for i := range v {
		if v[i] != min[i] {
			return v[i] > min[i]
		}
	}
	return true
}

func checkKVM(h host) error {
	if !h.exists("/dev/kvm") {
		return driver.Fixable(kvmFix, "/dev/kvm is not present")
	}
	if err := h.openRW("/dev/kvm"); err != nil {
		return driver.Fixable("sudo usermod -aG kvm $USER, then log in again",
			"cannot open /dev/kvm for reading and writing: %v", err)
	}
	return nil
}

func checkFirecracker(h host) error {
	if err := h.lookPath("firecracker"); err != nil {
		return driver.Fixable(firecrackerFix, "firecracker is not on PATH")
	}
	out, err := h.output([]string{"firecracker", "--version"})
	if err != nil {
		return driver.Fixable(firecrackerFix, "firecracker --version failed: %v", err)
	}
	v, ok := parseVersion(out)
	if !ok {
		return driver.Fixable(firecrackerFix, "cannot read the version from %q",
			strings.TrimSpace(out))
	}
	if !versionAtLeast(v, minVersion) {
		return driver.Fixable(firecrackerFix,
			"firecracker v%d.%d.%d is older than v%d.%d.%d",
			v[0], v[1], v[2], minVersion[0], minVersion[1], minVersion[2])
	}
	return nil
}

// ownedSlots returns the slots setup made for uid, in order: every slot from 0
// up to the first missing one whose three TAPs belong to uid.
func ownedSlots(h host, uid int) []Slot {
	var out []Slot
	want := strconv.Itoa(uid)
	for i := 0; i < MaxSlots; i++ {
		s := Slot(i)
		if !h.exists("/sys/class/net/" + s.UpstreamTap()) {
			break
		}
		owned := true
		for _, tap := range []string{s.RelayTap(), s.GuestTap(), s.UpstreamTap()} {
			b, err := h.readFile("/sys/class/net/" + tap + "/owner")
			if err != nil || strings.TrimSpace(string(b)) != want {
				owned = false
			}
		}
		if owned {
			out = append(out, s)
		}
	}
	return out
}

func checkSlots(h host) ([]Slot, error) {
	uid := slotUID(h)
	slots := ownedSlots(h, uid)
	if len(slots) == 0 {
		return nil, driver.Fixable(setupFix,
			"no firecracker network slots owned by this user (uid %d)", uid)
	}
	return slots, nil
}

// slotUID is the user whose slots count: this user, or under sudo the user
// who ran it, so `sudo warden doctor` checks the slots builds will use.
func slotUID(h host) int {
	if name := h.getenv("SUDO_USER"); h.euid() == 0 && name != "" {
		if uid, _, err := h.lookupOwner(name); err == nil {
			return uid
		}
	}
	return h.euid()
}

// hostLocal reports whether ip is assigned to one of this host's interfaces.
func hostLocal(ip net.IP) bool {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return false
	}
	for _, a := range addrs {
		if n, ok := a.(*net.IPNet); ok && n.IP.Equal(ip) {
			return true
		}
	}
	return false
}

// preflighted is what a build learns from its preflight.
type preflighted struct {
	assets    assets
	slots     []Slot
	resolvers []string
}

// preflight is the build's cheap up-front checks (UX-04), in the order a user
// would fix them: the host, KVM, the VMM, the boot assets, a slot, an
// off-host resolver. The disk path adds checkDiskTools once the build's image
// is known.
func preflight(h host, local func(net.IP) bool) (*preflighted, error) {
	if err := hostCheck(); err != nil {
		return nil, err
	}
	if err := checkKVM(h); err != nil {
		return nil, err
	}
	if err := checkFirecracker(h); err != nil {
		return nil, err
	}
	a, err := findAssets()
	if err != nil {
		return nil, err
	}
	slots, err := checkSlots(h)
	if err != nil {
		return nil, err
	}
	dns, err := upstreamResolvers(h.readFile, local)
	if err != nil {
		return nil, err
	}
	return &preflighted{assets: a, slots: slots, resolvers: dns}, nil
}

// hostCheck is hostSupported, replaced in tests so the host-independent
// checks run on every platform.
var hostCheck = hostSupported

// unsupportedHost is the error a non-Linux or non-x86_64 host gets.
func unsupportedHost(goos, goarch string) error {
	return driver.Fixable("run warden on an x86_64 Linux host with KVM, or pick "+
		"another driver (--driver qemu works on every host)",
		"--driver firecracker needs an x86_64 Linux host with /dev/kvm (this is %s/%s)",
		goos, goarch)
}

func fmtSlots(slots []Slot) string {
	if len(slots) == 0 {
		return "none"
	}
	return fmt.Sprintf("%d (wfc0 to wfc%d)", len(slots), slots[len(slots)-1])
}

// checkDiskTools is the disk path's own preflight: qemu-img converts the
// image, and an ISO tool writes a CIDATA seed cloud-init reads by its exact
// file names (the built-in level-1 ISO writer cannot).
func checkDiskTools(h host) error {
	if err := h.lookPath("qemu-img"); err != nil {
		return driver.Fixable(qemuImgFix,
			"a disk-image build converts its image with qemu-img, which is not on "+
				"PATH")
	}
	if h.lookPath("genisoimage") != nil && h.lookPath("mkisofs") != nil {
		return driver.Fixable("install it: apt install genisoimage (or your distro's "+
			"equivalent)",
			"a disk-image build writes its cloud-init seed with genisoimage or "+
				"mkisofs, "+
				"and neither is on PATH")
	}
	return nil
}
