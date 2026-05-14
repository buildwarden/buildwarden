package relay

import (
	"log"
	"net"
	"strings"
	"time"

	"github.com/miekg/dns"
)

func (r *Relay) runDNS() error {
	dnsClient := &dns.Client{Timeout: 5 * time.Second}

	handler := dns.HandlerFunc(func(w dns.ResponseWriter, req *dns.Msg) {
		m := new(dns.Msg)
		m.SetReply(req)
		for _, q := range m.Question {
			name := strings.TrimSuffix(q.Name, ".")

			if reservedHosts[name] {
				if q.Qtype == dns.TypeA {
					m.Answer = append(m.Answer, &dns.A{
						Hdr: dns.RR_Header{
							Name:   q.Name,
							Rrtype: dns.TypeA,
							Class:  dns.ClassINET,
							Ttl:    0,
						},
						A: r.selfIP,
					})
				}
				if err := w.WriteMsg(m); err != nil {
					log.Printf("DNS proxy error: %s\n", err)
				}
				return
			}
		}

		resp, _, err := dnsClient.Exchange(req, r.upstreamDNS)
		if err != nil {
			log.Printf("DNS forward error (%s): %v", r.upstreamDNS, err)
			m.Rcode = dns.RcodeServerFailure
			w.WriteMsg(m) //nolint:errcheck
			return
		}
		resp.Id = req.Id
		if err := w.WriteMsg(resp); err != nil {
			log.Printf("DNS proxy error: %s\n", err)
		}
	})

	addr := net.TCPAddr{IP: net.IPv4zero, Port: 53}

	// UDP listener
	if r.cfg.DNSPacketConn != nil {
		server := &dns.Server{PacketConn: r.cfg.DNSPacketConn, Handler: handler}
		go server.ActivateAndServe() //nolint:errcheck
	} else {
		server := &dns.Server{Addr: addr.String(), Net: "udp", Handler: handler}
		go server.ListenAndServe() //nolint:errcheck
	}

	// TCP listener (fallback)
	tcpServer := &dns.Server{Addr: addr.String(), Net: "tcp", Handler: handler}
	return tcpServer.ListenAndServe()
}

func (r *Relay) runHTTP() error {
	var listener net.Listener
	var err error

	if r.cfg.HTTPListener != nil {
		listener = r.cfg.HTTPListener
	} else {
		listener, err = net.Listen("tcp", ":80")
		if err != nil {
			return err
		}
	}

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("HTTP listener.Accept error: %v\n", err)
			continue
		}
		go r.serveHTTPConn(conn)
	}
}

func (r *Relay) runHTTPS() error {
	var listener net.Listener
	var err error

	if r.cfg.HTTPSListener != nil {
		listener = r.cfg.HTTPSListener
	} else {
		listener, err = net.Listen("tcp", ":443")
		if err != nil {
			return err
		}
	}

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("HTTPS listener.Accept error: %v\n", err)
			continue
		}
		go r.serveTLSConn(conn)
	}
}
