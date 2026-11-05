//go:build linux

package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"net/netip"
	"reflect"
	"syscall"
	"testing"

	"golang.org/x/sys/unix"
)

// The canned netlink messages below are laid out field by field from the
// kernel's structs (nlmsghdr, ifaddrmsg, rtmsg, rtattr), in host byte order
// as rtnetlink uses, so the tests pin the layout without opening a socket.

// nlMsg is one netlink message: a 16-byte nlmsghdr, then payload, padded to
// four bytes.
func nlMsg(typ uint16, flags uint16, payload []byte) []byte {
	b := make([]byte, 16, 16+len(payload)+3)
	binary.NativeEndian.PutUint32(b[0:4], uint32(16+len(payload)))
	binary.NativeEndian.PutUint16(b[4:6], typ)
	binary.NativeEndian.PutUint16(b[6:8], flags)
	binary.NativeEndian.PutUint32(b[8:12], 1)
	b = append(b, payload...)
	for len(b)%4 != 0 {
		b = append(b, 0)
	}
	return b
}

// rtAttr is one route attribute: length, type, value, padded to four bytes.
func rtAttr(typ uint16, v []byte) []byte {
	b := make([]byte, 4, 4+len(v)+3)
	binary.NativeEndian.PutUint16(b[0:2], uint16(4+len(v)))
	binary.NativeEndian.PutUint16(b[2:4], typ)
	b = append(b, v...)
	for len(b)%4 != 0 {
		b = append(b, 0)
	}
	return b
}

// ifAddrMsg is an RTM_NEWADDR body: an 8-byte ifaddrmsg, then attributes.
func ifAddrMsg(prefixLen byte, index uint32, attrs ...[]byte) []byte {
	b := []byte{unix.AF_INET, prefixLen, 0, 0, 0, 0, 0, 0}
	binary.NativeEndian.PutUint32(b[4:8], index)
	for _, a := range attrs {
		b = append(b, a...)
	}
	return nlMsg(unix.RTM_NEWADDR, unix.NLM_F_MULTI, b)
}

// rtMsg is an RTM_NEWROUTE body: a 12-byte rtmsg, then attributes.
func rtMsg(family, dstLen byte, attrs ...[]byte) []byte {
	b := []byte{family, dstLen, 0, 0, unix.RT_TABLE_MAIN, 0, 0, unix.RTN_UNICAST,
		0, 0, 0, 0}
	for _, a := range attrs {
		b = append(b, a...)
	}
	return nlMsg(unix.RTM_NEWROUTE, unix.NLM_F_MULTI, b)
}

func u32(v uint32) []byte {
	b := make([]byte, 4)
	binary.NativeEndian.PutUint32(b, v)
	return b
}

func ip4(s string) []byte {
	a := netip.MustParseAddr(s).As4()
	return a[:]
}

func done() []byte { return nlMsg(unix.NLMSG_DONE, unix.NLM_F_MULTI, u32(0)) }

func cat(parts ...[]byte) []byte { return bytes.Join(parts, nil) }

func TestParseAddrs(t *testing.T) {
	rib := cat(
		// lo: 127.0.0.1/8.
		ifAddrMsg(8, 1, rtAttr(unix.IFA_ADDRESS, ip4("127.0.0.1")),
			rtAttr(unix.IFA_LOCAL, ip4("127.0.0.1")),
			rtAttr(unix.IFA_LABEL, []byte("lo\x00"))),
		// eth0 with two addresses, the second only IFA_ADDRESS.
		ifAddrMsg(30, 2, rtAttr(unix.IFA_ADDRESS, ip4("100.127.255.253")),
			rtAttr(unix.IFA_LOCAL, ip4("100.127.255.253"))),
		ifAddrMsg(24, 2, rtAttr(unix.IFA_ADDRESS, ip4("10.0.0.1"))),
		// A point-to-point address: IFA_ADDRESS is the peer, IFA_LOCAL ours.
		ifAddrMsg(32, 3, rtAttr(unix.IFA_ADDRESS, ip4("192.0.2.9")),
			rtAttr(unix.IFA_LOCAL, ip4("192.0.2.1"))),
		// An attribute too short to be an address is skipped.
		ifAddrMsg(24, 4, rtAttr(unix.IFA_ADDRESS, []byte{1, 2})),
		done(),
	)
	got, err := parseAddrs(rib)
	if err != nil {
		t.Fatal(err)
	}
	want := []ifAddr{
		{1, netip.MustParsePrefix("127.0.0.1/8")},
		{2, netip.MustParsePrefix("100.127.255.253/30")},
		{2, netip.MustParsePrefix("10.0.0.1/24")},
		{3, netip.MustParsePrefix("192.0.2.1/32")},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseAddrs = %v, want %v", got, want)
	}
}

func TestParseRoutes(t *testing.T) {
	rib := cat(
		// default via 192.168.1.1 dev 3 (no RTA_DST).
		rtMsg(unix.AF_INET, 0, rtAttr(unix.RTA_GATEWAY, ip4("192.168.1.1")),
			rtAttr(unix.RTA_OIF, u32(3))),
		// 10.1.2.0/30 dev 4.
		rtMsg(unix.AF_INET, 30, rtAttr(unix.RTA_DST, ip4("10.1.2.0")),
			rtAttr(unix.RTA_OIF, u32(4))),
		// 100.127.255.252/30 dev 2, the build interface: left out.
		rtMsg(unix.AF_INET, 30, rtAttr(unix.RTA_DST, ip4("100.127.255.252")),
			rtAttr(unix.RTA_OIF, u32(2))),
		// A multipath route names no RTA_OIF: kept.
		rtMsg(unix.AF_INET, 16, rtAttr(unix.RTA_DST, ip4("172.16.0.0"))),
		// An IPv6 destination is not an IPv4 route.
		rtMsg(unix.AF_INET6, 64, rtAttr(unix.RTA_DST,
			netip.MustParseAddr("fd00::").AsSlice()), rtAttr(unix.RTA_OIF, u32(3))),
		done(),
	)
	got, err := parseRoutes(rib, 2)
	if err != nil {
		t.Fatal(err)
	}
	want := []netip.Prefix{
		netip.MustParsePrefix("0.0.0.0/0"),
		netip.MustParsePrefix("10.1.2.0/30"),
		netip.MustParsePrefix("172.16.0.0/16"),
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseRoutes = %v, want %v", got, want)
	}
}

func TestEncodeAddrRequest(t *testing.T) {
	addr := netip.MustParsePrefix("100.127.255.253/30")
	got := encodeAddrRequest(unix.RTM_NEWADDR, unix.NLM_F_CREATE|unix.NLM_F_EXCL, 7, addr)

	body := []byte{unix.AF_INET, 30, 0, 0, 0, 0, 0, 0}
	binary.NativeEndian.PutUint32(body[4:8], 7)
	body = cat(body, rtAttr(unix.IFA_LOCAL, ip4("100.127.255.253")),
		rtAttr(unix.IFA_ADDRESS, ip4("100.127.255.253")))
	want := nlMsg(unix.RTM_NEWADDR,
		unix.NLM_F_REQUEST|unix.NLM_F_ACK|unix.NLM_F_CREATE|unix.NLM_F_EXCL, body)
	if !bytes.Equal(got, want) {
		t.Fatalf("encodeAddrRequest =\n%x\nwant\n%x", got, want)
	}
	if len(got) != 16+8+2*8 {
		t.Fatalf("request is %d bytes", len(got))
	}
}

func TestParseAck(t *testing.T) {
	ack := func(code int32) []byte {
		return nlMsg(unix.NLMSG_ERROR, 0, cat(u32(uint32(code)), make([]byte, 16)))
	}
	if err := parseAck(ack(0)); err != nil {
		t.Fatalf("acknowledgement: %v", err)
	}
	if err := parseAck(ack(-int32(unix.EEXIST))); !errors.Is(err, unix.EEXIST) {
		t.Fatalf("EEXIST: %v", err)
	}
	if err := parseAck(done()); err == nil {
		t.Fatal("a reply with no NLMSG_ERROR was taken as an acknowledgement")
	}
	if err := parseAck([]byte{1, 2, 3}); err == nil {
		t.Fatal("a truncated reply was accepted")
	}
}

func TestFlushAddrs(t *testing.T) {
	a := netip.MustParsePrefix("100.127.255.253/30")
	b := netip.MustParsePrefix("100.127.255.254/30")
	c := netip.MustParsePrefix("10.0.0.1/24")
	var tried []netip.Prefix
	del := func(p netip.Prefix) error {
		tried = append(tried, p)
		if p == b {
			// Gone with its primary a.
			return syscall.Errno(unix.EADDRNOTAVAIL)
		}
		return nil
	}
	if err := flushAddrs([]netip.Prefix{a, b, c}, del); err != nil {
		t.Fatalf("a secondary already removed failed the flush: %v", err)
	}
	if !reflect.DeepEqual(tried, []netip.Prefix{a, b, c}) {
		t.Fatalf("tried %v", tried)
	}
	err := flushAddrs([]netip.Prefix{a}, func(netip.Prefix) error { return unix.EPERM })
	if !errors.Is(err, unix.EPERM) {
		t.Fatalf("EPERM: %v", err)
	}
}
