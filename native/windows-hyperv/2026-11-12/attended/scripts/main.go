// dhcpprobe sends one DHCPDISCOVER on a network and reports any BOOTREPLY
// that arrives within -wait. Validation tool for build-link-subnet's "a
// DISCOVER on the relay VM's upstream network gets no offer" case.
//
//	linux:   sudo dhcpprobe -if wfc0u [-wait 5s]        (raw AF_PACKET frame on the interface)
//	windows: dhcpprobe -src 192.168.240.1 [-wait 5s]    (UDP :68 bound to the host address)
package main

import (
	"crypto/rand"
	"encoding/binary"
	"flag"
	"fmt"
	"net"
	"os"
	"time"
)

var (
	ifName = flag.String("if", "", "linux: interface to send on")
	srcIP  = flag.String("src", "", "windows: local IPv4 address to bind :68 on")
	wait   = flag.Duration("wait", 5*time.Second, "how long to wait for a reply")
)

func main() {
	flag.Parse()
	mac := make([]byte, 6)
	if _, err := rand.Read(mac); err != nil {
		fatal(err)
	}
	mac[0] = (mac[0] | 0x02) &^ 0x01 // locally administered, unicast
	xid := make([]byte, 4)
	if _, err := rand.Read(xid); err != nil {
		fatal(err)
	}
	pkt := discover(mac, xid)
	fmt.Printf("sending DHCPDISCOVER xid=%x chaddr=%s\n", xid, net.HardwareAddr(mac))
	replies, err := probe(pkt, mac, xid, *wait)
	if err != nil {
		fatal(err)
	}
	if len(replies) == 0 {
		fmt.Printf("no BOOTREPLY within %s\n", *wait)
		return
	}
	for _, r := range replies {
		fmt.Println(describe(r))
	}
	os.Exit(2)
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "dhcpprobe:", err)
	os.Exit(1)
}

// discover builds a BOOTREQUEST with DHCPDISCOVER, broadcast flag set.
func discover(mac, xid []byte) []byte {
	p := make([]byte, 240, 300)
	p[0] = 1 // op BOOTREQUEST
	p[1] = 1 // htype ethernet
	p[2] = 6 // hlen
	copy(p[4:8], xid)
	binary.BigEndian.PutUint16(p[10:12], 0x8000) // broadcast
	copy(p[28:34], mac)
	copy(p[236:240], []byte{99, 130, 83, 99})
	p = append(p, 53, 1, 1)         // DHCPDISCOVER
	p = append(p, 55, 3, 1, 3, 6)   // request mask, router, dns
	p = append(p, 255)              // end
	for len(p) < 300 {
		p = append(p, 0)
	}
	return p
}

// isReplyFor reports whether payload is a BOOTREPLY to xid.
func isReplyFor(payload, xid []byte) bool {
	return len(payload) >= 240 && payload[0] == 2 && string(payload[4:8]) == string(xid)
}

// describe summarises a BOOTREPLY.
func describe(p []byte) string {
	yi := net.IP(p[16:20]).String()
	si := net.IP(p[20:24]).String()
	msg, sid, router, mask := "?", "", "", ""
	opts := p[240:]
	for i := 0; i+1 < len(opts); {
		t := opts[i]
		if t == 255 {
			break
		}
		if t == 0 {
			i++
			continue
		}
		l := int(opts[i+1])
		if i+2+l > len(opts) {
			break
		}
		v := opts[i+2 : i+2+l]
		switch t {
		case 53:
			if l == 1 {
				msg = map[byte]string{2: "OFFER", 5: "ACK", 6: "NAK"}[v[0]]
			}
		case 54:
			sid = net.IP(v).String()
		case 3:
			router = net.IP(v[:4]).String()
		case 1:
			mask = net.IP(v).String()
		}
		i += 2 + l
	}
	return fmt.Sprintf("BOOTREPLY %s yiaddr=%s siaddr=%s server-id=%s router=%s mask=%s",
		msg, yi, si, sid, router, mask)
}
