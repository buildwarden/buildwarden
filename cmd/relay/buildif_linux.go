//go:build linux

package main

import (
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os"
	"sort"
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
)

// netlinkIface is buildIface over rtnetlink. It reads and changes IPv4
// addresses on one interface and only reads everything else.
type netlinkIface struct {
	name  string
	index int
}

// openBuildIface picks the build interface (want, or the init's rule) and
// opens it for netlink address changes.
func openBuildIface(want string) (buildIface, error) {
	entries, err := os.ReadDir("/sys/class/net")
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)
	name, err := pickBuildIf(names, want)
	if err != nil {
		return nil, err
	}
	ifi, err := net.InterfaceByName(name)
	if err != nil {
		return nil, err
	}
	return &netlinkIface{name: name, index: ifi.Index}, nil
}

func (n *netlinkIface) Name() string { return n.name }

// ifAddr is one IPv4 address from an RTM_GETADDR dump.
type ifAddr struct {
	index  int
	prefix netip.Prefix
}

func dumpAddrs() ([]ifAddr, error) {
	rib, err := syscall.NetlinkRIB(unix.RTM_GETADDR, unix.AF_INET)
	if err != nil {
		return nil, fmt.Errorf("RTM_GETADDR: %w", err)
	}
	return parseAddrs(rib)
}

// parseAddrs reads the IPv4 addresses from an RTM_GETADDR dump. An address
// takes IFA_LOCAL when present (a point-to-point peer puts the peer in
// IFA_ADDRESS), else IFA_ADDRESS.
func parseAddrs(rib []byte) ([]ifAddr, error) {
	msgs, err := syscall.ParseNetlinkMessage(rib)
	if err != nil {
		return nil, err
	}
	var out []ifAddr
	for _, m := range msgs {
		if m.Header.Type != unix.RTM_NEWADDR || len(m.Data) < unix.SizeofIfAddrmsg {
			continue
		}
		ifa := (*unix.IfAddrmsg)(unsafe.Pointer(&m.Data[0]))
		attrs, err := syscall.ParseNetlinkRouteAttr(&m)
		if err != nil {
			return nil, err
		}
		var addr, local netip.Addr
		for _, a := range attrs {
			ip, ok := netip.AddrFromSlice(a.Value)
			if !ok || !ip.Is4() {
				continue
			}
			switch a.Attr.Type {
			case unix.IFA_ADDRESS:
				addr = ip
			case unix.IFA_LOCAL:
				local = ip
			}
		}
		if local.IsValid() {
			addr = local
		}
		if addr.IsValid() {
			p := netip.PrefixFrom(addr, int(ifa.Prefixlen))
			out = append(out, ifAddr{int(ifa.Index), p})
		}
	}
	return out, nil
}

func (n *netlinkIface) Addrs() ([]netip.Prefix, error) {
	all, err := dumpAddrs()
	if err != nil {
		return nil, err
	}
	var out []netip.Prefix
	for _, a := range all {
		if a.index == n.index {
			out = append(out, a.prefix)
		}
	}
	return out, nil
}

func (n *netlinkIface) Others() (addrs, routes []netip.Prefix, err error) {
	all, err := dumpAddrs()
	if err != nil {
		return nil, nil, err
	}
	for _, a := range all {
		if a.index != n.index {
			addrs = append(addrs, a.prefix)
		}
	}
	rib, err := syscall.NetlinkRIB(unix.RTM_GETROUTE, unix.AF_INET)
	if err != nil {
		return nil, nil, fmt.Errorf("RTM_GETROUTE: %w", err)
	}
	if routes, err = parseRoutes(rib, n.index); err != nil {
		return nil, nil, err
	}
	return addrs, routes, nil
}

// parseRoutes reads the IPv4 route destinations from an RTM_GETROUTE dump,
// leaving out routes whose output interface is index.
func parseRoutes(rib []byte, index int) ([]netip.Prefix, error) {
	msgs, err := syscall.ParseNetlinkMessage(rib)
	if err != nil {
		return nil, err
	}
	var routes []netip.Prefix
	for _, m := range msgs {
		if m.Header.Type != unix.RTM_NEWROUTE || len(m.Data) < unix.SizeofRtMsg {
			continue
		}
		dst, oif, err := parseRoute(&m)
		if err != nil {
			return nil, err
		}
		if oif != index && dst.Addr().Is4() {
			routes = append(routes, dst)
		}
	}
	return routes, nil
}

// parseRoute returns an RTM_NEWROUTE message's destination (0.0.0.0/0 for a
// default route) and output interface (-1 when it names none).
func parseRoute(m *syscall.NetlinkMessage) (netip.Prefix, int, error) {
	rt := (*unix.RtMsg)(unsafe.Pointer(&m.Data[0]))
	attrs, err := syscall.ParseNetlinkRouteAttr(m)
	if err != nil {
		return netip.Prefix{}, 0, err
	}
	dst, oif := netip.IPv4Unspecified(), -1
	for _, a := range attrs {
		switch a.Attr.Type {
		case unix.RTA_DST:
			if ip, ok := netip.AddrFromSlice(a.Value); ok {
				dst = ip
			}
		case unix.RTA_OIF:
			if len(a.Value) == 4 {
				oif = int(binary.NativeEndian.Uint32(a.Value))
			}
		}
	}
	return netip.PrefixFrom(dst, int(rt.Dst_len)), oif, nil
}

func (n *netlinkIface) Flush() error {
	addrs, err := n.Addrs()
	if err != nil {
		return err
	}
	return flushAddrs(addrs, func(a netip.Prefix) error {
		return n.addrRequest(unix.RTM_DELADDR, 0, a)
	})
}

// flushAddrs removes each address with del. Removing a primary address also
// removes its secondaries in the same subnet, so one that a later del finds
// gone (EADDRNOTAVAIL) counts as removed.
func flushAddrs(addrs []netip.Prefix, del func(netip.Prefix) error) error {
	for _, a := range addrs {
		if err := del(a); err != nil && !errors.Is(err, unix.EADDRNOTAVAIL) {
			return fmt.Errorf("removing %s: %w", a, err)
		}
	}
	return nil
}

func (n *netlinkIface) Add(addr netip.Prefix) error {
	return n.addrRequest(unix.RTM_NEWADDR, unix.NLM_F_CREATE|unix.NLM_F_EXCL, addr)
}

// addrRequest sends one RTM_NEWADDR or RTM_DELADDR for addr on the interface
// and waits for the kernel's acknowledgement.
func (n *netlinkIface) addrRequest(typ uint16, flags int, addr netip.Prefix) error {
	msg := encodeAddrRequest(typ, flags, n.index, addr)
	fd, err := unix.Socket(unix.AF_NETLINK, unix.SOCK_RAW|unix.SOCK_CLOEXEC,
		unix.NETLINK_ROUTE)
	if err != nil {
		return err
	}
	defer unix.Close(fd)
	if err := unix.Bind(fd, &unix.SockaddrNetlink{Family: unix.AF_NETLINK}); err != nil {
		return err
	}
	kernel := &unix.SockaddrNetlink{Family: unix.AF_NETLINK}
	if err := unix.Sendto(fd, msg, 0, kernel); err != nil {
		return err
	}
	buf := make([]byte, 4096)
	nr, _, err := unix.Recvfrom(fd, buf, 0)
	if err != nil {
		return err
	}
	return parseAck(buf[:nr])
}

// encodeAddrRequest builds one RTM_NEWADDR or RTM_DELADDR request, asking
// for an acknowledgement, for addr on interface index: an nlmsghdr, an
// ifaddrmsg and IFA_LOCAL and IFA_ADDRESS both holding the address.
func encodeAddrRequest(typ uint16, flags, index int, addr netip.Prefix) []byte {
	ip := addr.Addr().As4()
	attr := func(t uint16, v []byte) []byte {
		b := make([]byte, unix.SizeofRtAttr+len(v))
		binary.NativeEndian.PutUint16(b[0:2], uint16(len(b)))
		binary.NativeEndian.PutUint16(b[2:4], t)
		copy(b[unix.SizeofRtAttr:], v)
		return b
	}
	body := make([]byte, unix.SizeofIfAddrmsg, unix.SizeofIfAddrmsg+2*(unix.SizeofRtAttr+4))
	body[0] = unix.AF_INET
	body[1] = byte(addr.Bits())
	binary.NativeEndian.PutUint32(body[4:8], uint32(index))
	body = append(body, attr(unix.IFA_LOCAL, ip[:])...)
	body = append(body, attr(unix.IFA_ADDRESS, ip[:])...)

	msg := make([]byte, unix.SizeofNlMsghdr, unix.SizeofNlMsghdr+len(body))
	binary.NativeEndian.PutUint32(msg[0:4], uint32(unix.SizeofNlMsghdr+len(body)))
	binary.NativeEndian.PutUint16(msg[4:6], typ)
	binary.NativeEndian.PutUint16(msg[6:8], uint16(unix.NLM_F_REQUEST|unix.NLM_F_ACK|flags))
	binary.NativeEndian.PutUint32(msg[8:12], 1)
	return append(msg, body...)
}

// parseAck reads the kernel's reply to one request: nil for an
// acknowledgement, the errno for an NLMSG_ERROR that carries one.
func parseAck(buf []byte) error {
	msgs, err := syscall.ParseNetlinkMessage(buf)
	if err != nil {
		return err
	}
	for _, m := range msgs {
		if m.Header.Type == unix.NLMSG_ERROR && len(m.Data) >= 4 {
			if code := int32(binary.NativeEndian.Uint32(m.Data[0:4])); code != 0 {
				return syscall.Errno(-code)
			}
			return nil
		}
	}
	return fmt.Errorf("no netlink acknowledgement")
}
