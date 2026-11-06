package main

import (
	"encoding/binary"
	"errors"
	"net"
	"net/netip"
	"strings"
	"sync"
	"testing"
)

// bootpRequest builds a BOOTP payload with the given op, xid, chaddr and
// options (after the magic cookie, without the end option).
func bootpRequest(op byte, xid uint32, mac net.HardwareAddr, opts ...byte) []byte {
	b := make([]byte, bootpFixedLen, bootpFixedLen+4+len(opts)+1)
	b[0], b[1], b[2] = op, 1, 6
	binary.BigEndian.PutUint32(b[4:8], xid)
	copy(b[28:34], mac)
	b = binary.BigEndian.AppendUint32(b, dhcpCookie)
	b = append(b, opts...)
	return append(b, optEnd)
}

func msg(t byte) []byte { return []byte{optMessageType, 1, t} }

// replyOptions parses a reply's options into a map.
func replyOptions(t *testing.T, rep []byte) map[byte][]byte {
	t.Helper()
	if binary.BigEndian.Uint32(rep[bootpFixedLen:]) != dhcpCookie {
		t.Fatal("reply has no cookie")
	}
	out := map[byte][]byte{}
	o := rep[bootpFixedLen+4:]
	for len(o) > 0 && o[0] != optEnd {
		out[o[0]] = o[2 : 2+o[1]]
		o = o[2+o[1]:]
	}
	return out
}

func lease(cidr string) dhcpLease {
	p := netip.MustParsePrefix(cidr)
	r := p.Addr().Next()
	return newDHCPLease(p, r, r.Next())
}

func TestDHCPReplyOfferAndAck(t *testing.T) {
	mac := net.HardwareAddr{2, 0xaa, 0xbb, 0xcc, 0xdd, 0xee}
	for _, cidr := range []string{"100.127.255.252/30", "100.126.255.252/30", "192.0.2.0/24"} {
		p := netip.MustParsePrefix(cidr)
		relay := p.Addr().Next()
		guest := relay.Next()
		l := lease(cidr)
		for _, tc := range []struct{ in, out byte }{{dhcpDiscover, dhcpOffer},
			{dhcpRequest, dhcpAck}} {
			rep, ok := dhcpReply(bootpRequest(1, 0x1234abcd, mac, msg(tc.in)...), l)
			if !ok {
				t.Fatalf("%s: no reply to message %d", cidr, tc.in)
			}
			if rep[0] != 2 || binary.BigEndian.Uint32(rep[4:8]) != 0x1234abcd ||
				net.HardwareAddr(rep[28:34]).String() != mac.String() {
				t.Fatalf("%s: header op %d xid %x chaddr %s", cidr, rep[0],
					rep[4:8], rep[28:34])
			}
			if got := net.IP(rep[16:20]).String(); got != guest.String() {
				t.Errorf("%s: yiaddr %s, want %s", cidr, got, guest)
			}
			o := replyOptions(t, rep)
			mask := net.IP(net.CIDRMask(p.Bits(), 32)).String()
			checks := map[byte]string{
				optServerID: relay.String(), optRouter: relay.String(),
				optDNS:        relay.String(),
				optSubnetMask: mask,
			}
			for code, want := range checks {
				if got := net.IP(o[code]).String(); got != want {
					t.Errorf("%s: option %d = %s, want %s", cidr, code, got,
						want)
				}
			}
			if o[optMessageType][0] != tc.out {
				t.Errorf("%s: type %d, want %d", cidr, o[optMessageType][0],
					tc.out)
			}
			if binary.BigEndian.Uint32(o[optLeaseTime]) != dhcpInfinite {
				t.Errorf("%s: lease %x, want infinite", cidr, o[optLeaseTime])
			}
			want := "OFFER"
			if tc.out == dhcpAck {
				want = "ACK"
			}
			if line := dhcpLogLine(rep); !strings.Contains(line,
				want+" "+guest.String()) {
				t.Errorf("log line %q", line)
			}
		}
	}
}

func TestDHCPReplyIgnores(t *testing.T) {
	mac := net.HardwareAddr{2, 1, 2, 3, 4, 5}
	l := lease("100.127.255.252/30")
	noCookie := bootpRequest(1, 1, mac, msg(dhcpDiscover)...)
	binary.BigEndian.PutUint32(noCookie[bootpFixedLen:], 0)
	truncated := bootpRequest(1, 1, mac, msg(dhcpDiscover)...)
	truncated = append(truncated[:len(truncated)-1], 12, 40, 'x') // hostname claims 40 bytes
	tests := map[string][]byte{
		"bootreply (a forged server)": bootpRequest(2, 1, mac, msg(dhcpOffer)...),
		"release":                     bootpRequest(1, 1, mac, msg(7)...),
		"decline":                     bootpRequest(1, 1, mac, msg(4)...),
		"inform":                      bootpRequest(1, 1, mac, msg(8)...),
		"no message type":             bootpRequest(1, 1, mac),
		"no cookie":                   noCookie,
		"short":                       make([]byte, 100),
		"empty":                       nil,
		"truncated option":            truncated,
	}
	for name, req := range tests {
		if rep, ok := dhcpReply(req, l); ok {
			t.Errorf("%s: replied %x", name, rep[:8])
		}
	}
}

// TestDHCPReplyStateless is the starvation and spoofing case: any number of
// clients, transactions and requested addresses all get the one guest
// address, and nothing is remembered between them.
func TestDHCPReplyStateless(t *testing.T) {
	l := lease("100.127.255.252/30")
	for i := 0; i < 2000; i++ {
		mac := net.HardwareAddr{2, byte(i >> 8), byte(i), 9, 9, 9}
		typ := byte(dhcpDiscover)
		opts := msg(typ)
		if i%2 == 1 {
			// A REQUEST naming another address (option 50).
			opts = append(msg(dhcpRequest), 50, 4, 100, 127, 255, byte(i))
		}
		rep, ok := dhcpReply(bootpRequest(1, uint32(i), mac, opts...), l)
		if !ok {
			t.Fatalf("request %d unanswered", i)
		}
		if got := net.IP(rep[16:20]).String(); got != "100.127.255.254" {
			t.Fatalf("request %d offered %s", i, got)
		}
		if binary.BigEndian.Uint32(rep[4:8]) != uint32(i) {
			t.Fatalf("request %d: xid not echoed", i)
		}
	}
}

func FuzzDHCPReply(f *testing.F) {
	mac := net.HardwareAddr{2, 1, 2, 3, 4, 5}
	f.Add(bootpRequest(1, 7, mac, msg(dhcpDiscover)...))
	f.Add(bootpRequest(1, 7, mac, append(msg(dhcpRequest), 50, 4, 1, 2, 3, 4)...))
	f.Add(bootpRequest(2, 7, mac, msg(dhcpOffer)...))
	f.Add(bootpRequest(1, 7, mac, 0, 0, 12, 200))
	f.Add([]byte{1})
	l := lease("100.127.255.252/30")
	f.Fuzz(func(t *testing.T, req []byte) {
		rep, ok := dhcpReply(req, l)
		if !ok {
			return
		}
		if len(rep) != 300 || net.IP(rep[16:20]).String() != "100.127.255.254" {
			t.Fatalf("reply %d bytes offering %s", len(rep), net.IP(rep[16:20]))
		}
		_ = dhcpLogLine(rep)
	})
}

// fakeConn is a packetConn fed from a channel, recording what it is sent.
type fakeConn struct {
	in     chan []byte
	mu     sync.Mutex
	sent   []net.Addr
	closed chan struct{}
	once   sync.Once
}

func newFakeConn() *fakeConn {
	return &fakeConn{in: make(chan []byte, 8), closed: make(chan struct{})}
}

func (c *fakeConn) ReadFrom(b []byte) (int, net.Addr, error) {
	from := &net.UDPAddr{IP: net.IPv4zero, Port: 68}
	select { // drain what was queued before a close
	case p := <-c.in:
		return copy(b, p), from, nil
	default:
	}
	select {
	case p := <-c.in:
		return copy(b, p), from, nil
	case <-c.closed:
		return 0, nil, net.ErrClosed
	}
}

func (c *fakeConn) WriteTo(b []byte, a net.Addr) (int, error) {
	c.mu.Lock()
	c.sent = append(c.sent, a)
	c.mu.Unlock()
	return len(b), nil
}

func (c *fakeConn) Close() error {
	c.once.Do(func() { close(c.closed) })
	return nil
}

func TestVMDHCPResponder(t *testing.T) {
	conn := newFakeConn()
	var bound []string
	open := func(name string) (packetConn, error) {
		bound = append(bound, name)
		return conn, nil
	}
	stop, err := startVMDHCP(open, "eth0", lease("100.127.255.252/30"))
	if err != nil {
		t.Fatal(err)
	}
	mac := net.HardwareAddr{2, 1, 2, 3, 4, 5}
	conn.in <- bootpRequest(2, 1, mac, msg(dhcpOffer)...) // ignored
	conn.in <- bootpRequest(1, 1, mac, msg(dhcpDiscover)...)
	conn.in <- bootpRequest(1, 2, mac, msg(dhcpRequest)...)
	conn.in <- bootpRequest(1, 3, mac, msg(7)...) // ignored
	conn.in <- bootpRequest(1, 4, mac, msg(dhcpDiscover)...)
	stop()

	if len(bound) != 1 || bound[0] != "eth0" {
		t.Fatalf("bound %v, want the build interface only", bound)
	}
	conn.mu.Lock()
	defer conn.mu.Unlock()
	if len(conn.sent) != 3 {
		t.Fatalf("sent %d replies, want 3", len(conn.sent))
	}
	for _, a := range conn.sent {
		if a.String() != "255.255.255.255:68" {
			t.Fatalf("reply sent to %s, want the broadcast address", a)
		}
	}
	select {
	case <-conn.closed:
	default:
		t.Fatal("stop did not close the socket")
	}
}

func TestVMDHCPOpenError(t *testing.T) {
	open := func(string) (packetConn, error) { return nil, errors.New("bind failed") }
	if _, err := startVMDHCP(open, "eth0", lease("100.127.255.252/30")); err == nil {
		t.Fatal("a bind failure did not fail the start")
	}
}
