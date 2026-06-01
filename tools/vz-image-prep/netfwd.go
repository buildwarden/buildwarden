//go:build ignore

// vz-netfwd: Transparent TCP/UDP forwarder for macOS VZ VMs.
// Provides internet access via socketpair WITHOUT TLS interception.
// Used during image preparation (CLT install, system updates) where
// Apple's certificate pinning prevents MITM.
//
// This is NOT the relay — it produces no ledger, no logs, no interception.
// It exists solely for image prep where direct internet is needed.
//
// Build:
//   go build -o .dev/vz-netfwd .dev/vz-netfwd.go
//
// Usage (called by vz-personalize, not directly):
//   ./.dev/vz-netfwd --fd=3 --subnet=10.0.0.0/30

package main

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/sys/unix"

	"gvisor.dev/gvisor/pkg/buffer"
	"gvisor.dev/gvisor/pkg/tcpip"
	"gvisor.dev/gvisor/pkg/tcpip/adapters/gonet"
	"gvisor.dev/gvisor/pkg/tcpip/header"
	"gvisor.dev/gvisor/pkg/tcpip/link/channel"
	"gvisor.dev/gvisor/pkg/tcpip/network/ipv4"
	"gvisor.dev/gvisor/pkg/tcpip/stack"
	"gvisor.dev/gvisor/pkg/tcpip/transport/tcp"
	"gvisor.dev/gvisor/pkg/tcpip/transport/udp"
	"gvisor.dev/gvisor/pkg/waiter"
)

const (
	nicID    = 1
	mtu      = 1500
	chanSize = 256
)

func main() {
	fd := 3
	subnet := "10.0.0.0/30"

	for _, arg := range os.Args[1:] {
		if strings.HasPrefix(arg, "--fd=") {
			fd, _ = strconv.Atoi(arg[5:])
		}
		if strings.HasPrefix(arg, "--subnet=") {
			subnet = arg[9:]
		}
	}

	gwIP, guestIP, mask, err := parseSubnet(subnet)
	if err != nil {
		log.Fatalf("invalid subnet: %v", err)
	}

	unix.SetsockoptInt(fd, unix.SOL_SOCKET, unix.SO_SNDBUF, 4*1024*1024)
	unix.SetsockoptInt(fd, unix.SOL_SOCKET, unix.SO_RCVBUF, 4*1024*1024)
	unix.SetNonblock(fd, true)

	f := os.NewFile(uintptr(fd), "vmnet-socket")
	gwMAC := net.HardwareAddr{0x02, 0xBD, 0x00, 0x00, 0x00, 0x01}

	ep := channel.New(chanSize, mtu, tcpip.LinkAddress(gwMAC))
	s := stack.New(stack.Options{
		NetworkProtocols:   []stack.NetworkProtocolFactory{ipv4.NewProtocol},
		TransportProtocols: []stack.TransportProtocolFactory{tcp.NewProtocol, udp.NewProtocol},
	})

	s.CreateNIC(nicID, ep)
	s.SetPromiscuousMode(nicID, true)
	s.SetSpoofing(nicID, true)

	gwAddr := tcpip.AddrFrom4([4]byte(gwIP.To4()))
	prefixLen, _ := mask.Size()
	s.AddProtocolAddress(nicID, tcpip.ProtocolAddress{
		Protocol: ipv4.ProtocolNumber,
		AddressWithPrefix: tcpip.AddressWithPrefix{
			Address:   gwAddr,
			PrefixLen: prefixLen,
		},
	}, stack.AddressProperties{})

	s.SetRouteTable([]tcpip.Route{{
		Destination: header.IPv4EmptySubnet,
		NIC:         nicID,
	}})

	// TCP forwarder: transparently forward all TCP to the real internet
	fwd := tcp.NewForwarder(s, 0, 4096, func(r *tcp.ForwarderRequest) {
		id := r.ID()
		dstIP := id.LocalAddress
		dstPort := id.LocalPort

		var wq waiter.Queue
		ep, tcpErr := r.CreateEndpoint(&wq)
		if tcpErr != nil {
			r.Complete(true)
			return
		}
		r.Complete(false)
		vmConn := gonet.NewTCPConn(&wq, ep)

		// Connect to the real destination on the host network
		dst := net.JoinHostPort(dstIP.String(), fmt.Sprintf("%d", dstPort))
		realConn, err := net.DialTimeout("tcp", dst, 10*time.Second)
		if err != nil {
			vmConn.Close()
			return
		}

		// Bidirectional pipe
		go func() {
			io.Copy(realConn, vmConn)
			realConn.Close()
		}()
		go func() {
			io.Copy(vmConn, realConn)
			vmConn.Close()
		}()
	})
	s.SetTransportProtocolHandler(tcp.ProtocolNumber, fwd.HandlePacket)

	// DNS: forward UDP/53 to the host's resolver
	go serveDNS(s, gwIP)

	// DHCP: respond with guestIP
	var guestMAC net.HardwareAddr
	var guestMACMu sync.Mutex

	// Read loop: frames from VM → netstack
	go func() {
		buf := make([]byte, mtu+18)
		for {
			n, err := f.Read(buf)
			if err != nil {
				return
			}
			if n < 14 {
				continue
			}
			frame := buf[:n]
			etherType := binary.BigEndian.Uint16(frame[12:14])
			srcMAC := net.HardwareAddr(append([]byte(nil), frame[6:12]...))

			guestMACMu.Lock()
			if guestMAC == nil {
				guestMAC = srcMAC
				log.Printf("netfwd: learned guest MAC %s", srcMAC)
			}
			guestMACMu.Unlock()

			switch etherType {
			case 0x0806:
				handleARP(f, frame, gwIP, gwMAC)
			case 0x0800:
				if isDHCP(frame[14:]) {
					handleDHCP(f, frame, gwIP, guestIP, mask, gwMAC)
					continue
				}
				pkt := stack.NewPacketBuffer(stack.PacketBufferOptions{
					Payload: buffer.MakeWithData(frame[14:]),
				})
				ep.InjectInbound(ipv4.ProtocolNumber, pkt)
				pkt.DecRef()
			}
		}
	}()

	// Write loop: netstack → VM
	go func() {
		for {
			pkt := ep.ReadContext(context.Background())
			if pkt == nil {
				return
			}
			guestMACMu.Lock()
			dstMAC := guestMAC
			guestMACMu.Unlock()
			if dstMAC == nil {
				dstMAC = net.HardwareAddr{0xff, 0xff, 0xff, 0xff, 0xff, 0xff}
			}

			payload := pkt.ToView().AsSlice()
			frame := make([]byte, 14+len(payload))
			copy(frame[0:6], dstMAC)
			copy(frame[6:12], gwMAC)
			binary.BigEndian.PutUint16(frame[12:14], 0x0800)
			copy(frame[14:], payload)
			f.Write(frame)
			pkt.DecRef()
		}
	}()

	// ARP kickstart (macOS needs to see frames before link goes active)
	go func() {
		arp := makeGratuitousARP(gwIP, gwMAC)
		for i := 0; i < 120; i++ {
			f.Write(arp)
			time.Sleep(500 * time.Millisecond)
		}
	}()

	fmt.Fprintf(os.Stderr, "netfwd: ready (gateway %s, guest %s)\n", gwIP, guestIP)

	// Block forever (parent kills us)
	select {}
}

func serveDNS(s *stack.Stack, gwIP net.IP) {
	gwAddr := tcpip.AddrFrom4([4]byte(gwIP.To4()))
	fullAddr := tcpip.FullAddress{NIC: nicID, Addr: gwAddr, Port: 53}
	conn, err := gonet.DialUDP(s, &fullAddr, nil, ipv4.ProtocolNumber)
	if err != nil {
		log.Printf("netfwd: DNS listen error: %v", err)
		return
	}

	buf := make([]byte, 4096)
	for {
		n, addr, err := conn.ReadFrom(buf)
		if err != nil {
			return
		}
		go func(data []byte, from net.Addr) {
			// Forward to system resolver
			resp, err := forwardDNS(data)
			if err != nil {
				return
			}
			conn.WriteTo(resp, from)
		}(append([]byte(nil), buf[:n]...), addr)
	}
}

func forwardDNS(query []byte) ([]byte, error) {
	// Parse the DNS query to extract the question name and type,
	// then resolve using Go's system resolver (which uses cgo/mDNSResponder
	// on macOS and isn't blocked by endpoint security).
	if len(query) < 12 {
		return nil, fmt.Errorf("query too short")
	}

	// Parse question section
	id := query[0:2]
	qdcount := binary.BigEndian.Uint16(query[4:6])
	if qdcount == 0 {
		return nil, fmt.Errorf("no questions")
	}

	// Parse domain name from question
	offset := 12
	var nameParts []string
	for offset < len(query) {
		l := int(query[offset])
		if l == 0 {
			offset++
			break
		}
		offset++
		if offset+l > len(query) {
			return nil, fmt.Errorf("name overflow")
		}
		nameParts = append(nameParts, string(query[offset:offset+l]))
		offset += l
	}
	name := strings.Join(nameParts, ".")

	if offset+4 > len(query) {
		return nil, fmt.Errorf("query truncated")
	}
	qtype := binary.BigEndian.Uint16(query[offset : offset+2])

	// Resolve using system resolver
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var answers []byte
	ancount := uint16(0)

	switch qtype {
	case 1: // A
		ips, err := net.DefaultResolver.LookupIPAddr(ctx, name)
		if err != nil {
			return buildDNSError(id, query), nil
		}
		for _, ip := range ips {
			if v4 := ip.IP.To4(); v4 != nil {
				answers = append(answers, buildDNSAnswer(name, 1, v4)...)
				ancount++
			}
		}
	case 28: // AAAA
		ips, err := net.DefaultResolver.LookupIPAddr(ctx, name)
		if err != nil {
			return buildDNSError(id, query), nil
		}
		for _, ip := range ips {
			if ip.IP.To4() == nil {
				answers = append(answers, buildDNSAnswer(name, 28, ip.IP.To16())...)
				ancount++
			}
		}
	default:
		// Unsupported type — return empty response (no error, just no answers)
		return buildDNSEmpty(id, query), nil
	}

	// Build response
	resp := make([]byte, len(query)+len(answers))
	copy(resp, query)
	resp[2] = 0x81 // QR=1, RD=1
	resp[3] = 0x80 // RA=1
	binary.BigEndian.PutUint16(resp[6:8], ancount)
	resp = append(query, answers...)
	// Fix header
	resp[2] = 0x81
	resp[3] = 0x80
	binary.BigEndian.PutUint16(resp[6:8], ancount)
	return resp, nil
}

func buildDNSAnswer(name string, qtype uint16, rdata []byte) []byte {
	// Use pointer to question name (offset 12)
	var ans []byte
	ans = append(ans, 0xc0, 0x0c) // name pointer
	ans = append(ans, byte(qtype>>8), byte(qtype))
	ans = append(ans, 0, 1) // class IN
	ans = append(ans, 0, 0, 0, 60) // TTL 60s
	ans = append(ans, byte(len(rdata)>>8), byte(len(rdata)))
	ans = append(ans, rdata...)
	return ans
}

func buildDNSError(id []byte, query []byte) []byte {
	resp := make([]byte, len(query))
	copy(resp, query)
	resp[2] = 0x81
	resp[3] = 0x83 // NXDOMAIN
	return resp
}

func buildDNSEmpty(id []byte, query []byte) []byte {
	resp := make([]byte, len(query))
	copy(resp, query)
	resp[2] = 0x81
	resp[3] = 0x80 // no error, no answers
	return resp
}

func handleARP(f *os.File, frame []byte, gwIP net.IP, gwMAC net.HardwareAddr) {
	if len(frame) < 42 {
		return
	}
	arp := frame[14:]
	if binary.BigEndian.Uint16(arp[6:8]) != 1 {
		return
	}
	targetIP := net.IP(arp[24:28])
	if !targetIP.Equal(gwIP) {
		return
	}
	senderMAC := arp[8:14]
	senderIP := arp[14:18]

	reply := make([]byte, 42)
	copy(reply[0:6], senderMAC)
	copy(reply[6:12], gwMAC)
	binary.BigEndian.PutUint16(reply[12:14], 0x0806)
	r := reply[14:]
	binary.BigEndian.PutUint16(r[0:2], 1)
	binary.BigEndian.PutUint16(r[2:4], 0x0800)
	r[4] = 6
	r[5] = 4
	binary.BigEndian.PutUint16(r[6:8], 2)
	copy(r[8:14], gwMAC)
	copy(r[14:18], gwIP.To4())
	copy(r[18:24], senderMAC)
	copy(r[24:28], senderIP)
	f.Write(reply)
}

func isDHCP(ipPayload []byte) bool {
	if len(ipPayload) < 28 {
		return false
	}
	if ipPayload[9] != 17 {
		return false
	}
	ihl := int(ipPayload[0]&0x0f) * 4
	if len(ipPayload) < ihl+4 {
		return false
	}
	return binary.BigEndian.Uint16(ipPayload[ihl+2:ihl+4]) == 67
}

func handleDHCP(f *os.File, frame []byte, gwIP, guestIP net.IP, mask net.IPMask, gwMAC net.HardwareAddr) {
	if len(frame) < 14+28+236 {
		return
	}
	ipHeader := frame[14:]
	ihl := int(ipHeader[0]&0x0f) * 4
	bootp := ipHeader[ihl+8:]
	if bootp[0] != 1 {
		return
	}

	xid := bootp[4:8]
	clientMAC := net.HardwareAddr(bootp[28:34])

	msgType := byte(0)
	opts := bootp[236:]
	if len(opts) >= 4 && binary.BigEndian.Uint32(opts[:4]) == 0x63825363 {
		opts = opts[4:]
		for len(opts) > 0 {
			if opts[0] == 255 {
				break
			}
			if opts[0] == 0 {
				opts = opts[1:]
				continue
			}
			if len(opts) < 2 {
				break
			}
			if opts[0] == 53 && opts[1] >= 1 {
				msgType = opts[2]
			}
			opts = opts[2+int(opts[1]):]
		}
	}

	var replyType byte
	switch msgType {
	case 1:
		replyType = 2
	case 3:
		replyType = 5
	default:
		return
	}

	resp := buildDHCPReply(replyType, xid, clientMAC, gwIP, guestIP, mask)
	ethFrame := make([]byte, 14+len(resp))
	copy(ethFrame[0:6], clientMAC)
	copy(ethFrame[6:12], gwMAC)
	binary.BigEndian.PutUint16(ethFrame[12:14], 0x0800)
	copy(ethFrame[14:], resp)
	f.Write(ethFrame)
}

func buildDHCPReply(msgType byte, xid []byte, clientMAC net.HardwareAddr, gwIP, guestIP net.IP, mask net.IPMask) []byte {
	bootp := make([]byte, 300)
	bootp[0] = 2
	bootp[1] = 1
	bootp[2] = 6
	copy(bootp[4:8], xid)
	copy(bootp[16:20], guestIP.To4())
	copy(bootp[20:24], gwIP.To4())
	copy(bootp[28:34], clientMAC)

	i := 236
	copy(bootp[i:], []byte{0x63, 0x82, 0x53, 0x63})
	i += 4
	bootp[i], bootp[i+1], bootp[i+2] = 53, 1, msgType
	i += 3
	bootp[i], bootp[i+1] = 54, 4
	i += 2
	copy(bootp[i:], gwIP.To4())
	i += 4
	bootp[i], bootp[i+1] = 51, 4
	i += 2
	bootp[i], bootp[i+1], bootp[i+2], bootp[i+3] = 0, 0, 0x0e, 0x10
	i += 4
	bootp[i], bootp[i+1] = 1, 4
	i += 2
	copy(bootp[i:], mask)
	i += 4
	bootp[i], bootp[i+1] = 3, 4
	i += 2
	copy(bootp[i:], gwIP.To4())
	i += 4
	bootp[i], bootp[i+1] = 6, 4
	i += 2
	copy(bootp[i:], gwIP.To4())
	i += 4
	bootp[i] = 255

	// UDP + IP header
	udpLen := 8 + len(bootp)
	udpHdr := make([]byte, 8)
	binary.BigEndian.PutUint16(udpHdr[0:2], 67)
	binary.BigEndian.PutUint16(udpHdr[2:4], 68)
	binary.BigEndian.PutUint16(udpHdr[4:6], uint16(udpLen))

	ipLen := 20 + udpLen
	ipHdr := make([]byte, 20)
	ipHdr[0] = 0x45
	binary.BigEndian.PutUint16(ipHdr[2:4], uint16(ipLen))
	ipHdr[8] = 64
	ipHdr[9] = 17
	copy(ipHdr[12:16], gwIP.To4())
	copy(ipHdr[16:20], net.IPv4bcast.To4())
	binary.BigEndian.PutUint16(ipHdr[10:12], ipChecksum(ipHdr))

	var pkt []byte
	pkt = append(pkt, ipHdr...)
	pkt = append(pkt, udpHdr...)
	pkt = append(pkt, bootp...)
	return pkt
}

func makeGratuitousARP(ip net.IP, mac net.HardwareAddr) []byte {
	frame := make([]byte, 42)
	copy(frame[0:6], []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff})
	copy(frame[6:12], mac)
	binary.BigEndian.PutUint16(frame[12:14], 0x0806)
	arp := frame[14:]
	binary.BigEndian.PutUint16(arp[0:2], 1)
	binary.BigEndian.PutUint16(arp[2:4], 0x0800)
	arp[4] = 6
	arp[5] = 4
	binary.BigEndian.PutUint16(arp[6:8], 2)
	copy(arp[8:14], mac)
	copy(arp[14:18], ip.To4())
	copy(arp[18:24], mac)
	copy(arp[24:28], ip.To4())
	return frame
}

func ipChecksum(hdr []byte) uint16 {
	var sum uint32
	for i := 0; i < len(hdr)-1; i += 2 {
		sum += uint32(binary.BigEndian.Uint16(hdr[i : i+2]))
	}
	for sum > 0xffff {
		sum = (sum >> 16) + (sum & 0xffff)
	}
	return ^uint16(sum)
}

func parseSubnet(cidr string) (gateway, guest net.IP, mask net.IPMask, err error) {
	ip, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, nil, nil, err
	}
	base := ip.Mask(ipNet.Mask).To4()
	gw := make(net.IP, 4)
	copy(gw, base)
	gw[3]++
	g := make(net.IP, 4)
	copy(g, base)
	g[3] += 2
	return gw, g, ipNet.Mask, nil
}
