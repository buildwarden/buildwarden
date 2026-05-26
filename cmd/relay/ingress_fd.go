package main

import (
	"context"
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"os"
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

// FDIngressConfig configures the file-descriptor-based network ingress.
type FDIngressConfig struct {
	FD         int    // Inherited socketpair FD carrying raw Ethernet frames
	GatewayIP  net.IP // Relay's IP inside the virtual subnet (e.g. 10.100.0.1)
	GuestIP    net.IP // IP assigned to the VM via DHCP (e.g. 10.100.0.2)
	SubnetMask net.IPMask
	GatewayMAC net.HardwareAddr // MAC address for the relay's virtual NIC

	// ConnHandler is called for each intercepted TCP connection from the VM.
	// The dstPort indicates the original destination port the VM was connecting
	// to (80 for HTTP, 443 for HTTPS, etc.). The conn is a full net.Conn.
	ConnHandler func(conn net.Conn, dstPort uint16)
}

// FDIngress manages a userspace network stack reading Ethernet frames from a
// socketpair FD. It provides net.Listener (TCP) and net.PacketConn (UDP)
// interfaces that plug directly into the relay's existing proxy/DNS handlers.
type FDIngress struct {
	cfg  FDIngressConfig
	file *os.File
	ep   *channel.Endpoint
	s    *stack.Stack

	// guestMAC is learned from the first frame the VM sends (or DHCP discover).
	guestMAC   net.HardwareAddr
	guestMACMu sync.Mutex

	ctx    context.Context
	cancel context.CancelFunc
}

// NewFDIngress creates a userspace network stack attached to the given FD.
// The FD must be one end of a socketpair whose other end is connected to a
// VZFileHandleNetworkDeviceAttachment (or equivalent).
func NewFDIngress(cfg FDIngressConfig) (*FDIngress, error) {
	if cfg.GatewayMAC == nil {
		cfg.GatewayMAC = net.HardwareAddr{0x02, 0xBD, 0x00, 0x00, 0x00, 0x01}
	}

	// Increase socket buffer sizes before setting non-blocking.
	unix.SetsockoptInt(cfg.FD, unix.SOL_SOCKET, unix.SO_SNDBUF, 4*1024*1024)
	unix.SetsockoptInt(cfg.FD, unix.SOL_SOCKET, unix.SO_RCVBUF, 4*1024*1024)

	// Set non-blocking so Go's runtime poller can manage the FD.
	// Inherited FDs from exec may be in blocking mode.
	if err := unix.SetNonblock(cfg.FD, true); err != nil {
		return nil, fmt.Errorf("setting FD %d non-blocking: %w", cfg.FD, err)
	}

	f := os.NewFile(uintptr(cfg.FD), "vmnet-socket")
	if f == nil {
		return nil, fmt.Errorf("invalid FD %d", cfg.FD)
	}

	ep := channel.New(chanSize, mtu, tcpip.LinkAddress(cfg.GatewayMAC))

	s := stack.New(stack.Options{
		NetworkProtocols:   []stack.NetworkProtocolFactory{ipv4.NewProtocol},
		TransportProtocols: []stack.TransportProtocolFactory{
			tcp.NewProtocol, udp.NewProtocol,
		},
	})

	if err := s.CreateNIC(nicID, ep); err != nil {
		return nil, fmt.Errorf("creating NIC: %v", err)
	}

	// Enable promiscuous mode so the stack accepts packets addressed to
	// any IP (not just the gateway IP). This is essential for transparent
	// proxying: the build VM sends SYNs to external IPs (e.g. 108.x.x.x)
	// and we need the TCP forwarder to intercept them.
	if err := s.SetPromiscuousMode(nicID, true); err != nil {
		return nil, fmt.Errorf("enabling promiscuous mode: %v", err)
	}

	// Enable spoofing so the stack can send responses from IPs it doesn't
	// own (replying to connections addressed to external IPs).
	if err := s.SetSpoofing(nicID, true); err != nil {
		return nil, fmt.Errorf("enabling spoofing: %v", err)
	}

	gwAddr := tcpip.AddrFrom4([4]byte(cfg.GatewayIP.To4()))
	prefixLen := maskToPrefixLen(cfg.SubnetMask)
	protoAddr := tcpip.ProtocolAddress{
		Protocol: ipv4.ProtocolNumber,
		AddressWithPrefix: tcpip.AddressWithPrefix{
			Address:   gwAddr,
			PrefixLen: prefixLen,
		},
	}
	if err := s.AddProtocolAddress(nicID, protoAddr, stack.AddressProperties{}); err != nil {
		return nil, fmt.Errorf("adding address: %v", err)
	}

	// Route all traffic through this NIC (we are the only interface).
	s.SetRouteTable([]tcpip.Route{{
		Destination: header.IPv4EmptySubnet,
		NIC:         nicID,
	}})

	// Set up TCP forwarder to intercept all inbound TCP from the VM.
	// This replaces the iptables REDIRECT that the container driver uses.
	if cfg.ConnHandler != nil {
		fwd := tcp.NewForwarder(s, 0, 256, func(r *tcp.ForwarderRequest) {
			id := r.ID()
			var wq waiter.Queue
			ep, err := r.CreateEndpoint(&wq)
			if err != nil {
				log.Printf("netstack: TCP handshake failed: %v", err)
				r.Complete(true) // send RST
				return
			}
			r.Complete(false)
			conn := gonet.NewTCPConn(&wq, ep)
			go cfg.ConnHandler(conn, id.LocalPort)
		})
		s.SetTransportProtocolHandler(
			tcp.ProtocolNumber, fwd.HandlePacket,
		)
	}

	ctx, cancel := context.WithCancel(context.Background())

	ing := &FDIngress{
		cfg:    cfg,
		file:   f,
		ep:     ep,
		s:      s,
		ctx:    ctx,
		cancel: cancel,
	}

	go ing.readLoop()
	go ing.writeLoop()
	go ing.linkKickstart()

	return ing, nil
}

// linkKickstart sends periodic gratuitous ARP announcements to the VM.
// macOS requires receiving frames before it considers the link active.
// Without this, the guest reports "status: inactive" / "media: none".
func (ing *FDIngress) linkKickstart() {
	gwIP := ing.cfg.GatewayIP.To4()
	gwMAC := ing.cfg.GatewayMAC

	// Gratuitous ARP: sender announces its own IP→MAC binding.
	// Broadcast to ff:ff:ff:ff:ff:ff so macOS sees it regardless of state.
	arp := make([]byte, 28)
	binary.BigEndian.PutUint16(arp[0:2], 1)      // hardware: Ethernet
	binary.BigEndian.PutUint16(arp[2:4], 0x0800)  // protocol: IPv4
	arp[4] = 6                                     // hw addr len
	arp[5] = 4                                     // proto addr len
	binary.BigEndian.PutUint16(arp[6:8], 2)       // opcode: reply (gratuitous)
	copy(arp[8:14], gwMAC)                         // sender MAC
	copy(arp[14:18], gwIP)                         // sender IP
	copy(arp[18:24], gwMAC)                        // target MAC (self)
	copy(arp[24:28], gwIP)                         // target IP (self)

	frame := make([]byte, 14+28)
	copy(frame[0:6], []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff}) // dst: broadcast
	copy(frame[6:12], gwMAC)
	binary.BigEndian.PutUint16(frame[12:14], 0x0806) // ARP
	copy(frame[14:], arp)

	// Send every 500ms for 60 seconds (covers macOS boot time).
	for i := 0; i < 120; i++ {
		select {
		case <-ing.ctx.Done():
			return
		default:
		}
		ing.file.Write(frame) //nolint:errcheck
		time.Sleep(500 * time.Millisecond)
	}
}

// ListenTCP returns a net.Listener on the given port within the virtual stack.
func (ing *FDIngress) ListenTCP(port uint16) (net.Listener, error) {
	gwAddr := tcpip.AddrFrom4([4]byte(ing.cfg.GatewayIP.To4()))
	fullAddr := tcpip.FullAddress{
		NIC:  nicID,
		Addr: gwAddr,
		Port: port,
	}
	return gonet.ListenTCP(ing.s, fullAddr, ipv4.ProtocolNumber)
}

// ListenUDP returns a net.PacketConn on the given port within the virtual stack.
func (ing *FDIngress) ListenUDP(port uint16) (net.PacketConn, error) {
	gwAddr := tcpip.AddrFrom4([4]byte(ing.cfg.GatewayIP.To4()))
	fullAddr := tcpip.FullAddress{
		NIC:  nicID,
		Addr: gwAddr,
		Port: port,
	}
	return gonet.DialUDP(ing.s, &fullAddr, nil, ipv4.ProtocolNumber)
}

// Close shuts down the ingress.
func (ing *FDIngress) Close() {
	ing.cancel()
	time.Sleep(10 * time.Millisecond) // let goroutines observe cancellation
	ing.s.Close()
	ing.file.Close()
}

// readLoop reads raw Ethernet frames from the socketpair FD and dispatches
// them into the netstack or handles them at L2 (ARP, DHCP).
func (ing *FDIngress) readLoop() {
	buf := make([]byte, mtu+14+4) // ethernet header + possible padding
	for {
		select {
		case <-ing.ctx.Done():
			return
		default:
		}

		n, err := ing.file.Read(buf)
		if err != nil {
			select {
			case <-ing.ctx.Done():
				return
			default:
				log.Printf("netstack: read error: %v", err)
				return
			}
		}
		if n < 14 {
			continue // too short for Ethernet header
		}

		frame := buf[:n]
		etherType := binary.BigEndian.Uint16(frame[12:14])
		srcMAC := net.HardwareAddr(append([]byte(nil), frame[6:12]...))

		// Learn guest MAC from first frame
		ing.guestMACMu.Lock()
		if ing.guestMAC == nil {
			ing.guestMAC = srcMAC
			log.Printf("netstack: learned guest MAC %s", srcMAC)
		}
		ing.guestMACMu.Unlock()

		switch etherType {
		case 0x0806: // ARP
			ing.handleARP(frame)
		case 0x0800: // IPv4
			// Check for DHCP (UDP src port 68, dst port 67)
			if ing.isDHCP(frame[14:]) {
				ing.handleDHCP(frame)
				continue
			}
			// Inject IP packet into netstack
			pkt := stack.NewPacketBuffer(stack.PacketBufferOptions{
				Payload: buffer.MakeWithData(frame[14:]),
			})
			ing.ep.InjectInbound(ipv4.ProtocolNumber, pkt)
			pkt.DecRef()
		}
	}
}

// writeLoop reads packets from netstack and writes them as Ethernet frames
// to the socketpair FD (toward the VM).
func (ing *FDIngress) writeLoop() {
	for {
		pkt := ing.ep.ReadContext(ing.ctx)
		if pkt == nil {
			return
		}

		// Build Ethernet frame: dst MAC | src MAC | ethertype | payload
		ing.guestMACMu.Lock()
		dstMAC := ing.guestMAC
		ing.guestMACMu.Unlock()

		if dstMAC == nil {
			dstMAC = net.HardwareAddr{0xff, 0xff, 0xff, 0xff, 0xff, 0xff}
		}

		payload := pkt.ToView().AsSlice()
		frame := make([]byte, 14+len(payload))
		copy(frame[0:6], dstMAC)
		copy(frame[6:12], ing.cfg.GatewayMAC)
		binary.BigEndian.PutUint16(frame[12:14], 0x0800) // IPv4
		copy(frame[14:], payload)

		if _, err := ing.file.Write(frame); err != nil {
			select {
			case <-ing.ctx.Done():
				return
			default:
				// Drop frames when buffer is full (non-blocking socket).
				// This is normal under load — the VM will retransmit.
				continue
			}
		}
		pkt.DecRef()
	}
}

// isDHCP checks if an IPv4 packet is a DHCP request (UDP, dst port 67).
func (ing *FDIngress) isDHCP(ipPayload []byte) bool {
	if len(ipPayload) < 28 {
		return false
	}
	protocol := ipPayload[9]
	if protocol != 17 {
		return false
	}
	ihl := int(ipPayload[0]&0x0f) * 4
	if len(ipPayload) < ihl+4 {
		return false
	}
	dstPort := binary.BigEndian.Uint16(ipPayload[ihl+2 : ihl+4])
	return dstPort == 67
}

// isDHCPFrame checks an entire Ethernet frame (including eth header).
func isDHCPFrame(frame []byte) bool {
	if len(frame) < 14+28 {
		return false
	}
	if binary.BigEndian.Uint16(frame[12:14]) != 0x0800 {
		return false
	}
	ipPayload := frame[14:]
	if ipPayload[9] != 17 {
		return false
	}
	ihl := int(ipPayload[0]&0x0f) * 4
	if len(ipPayload) < ihl+4 {
		return false
	}
	dstPort := binary.BigEndian.Uint16(ipPayload[ihl+2 : ihl+4])
	return dstPort == 67
}

func maskToPrefixLen(mask net.IPMask) int {
	ones, _ := mask.Size()
	return ones
}
