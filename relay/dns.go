package relay

import (
	"context"
	"log"
	"net"
	"strings"
	"time"

	"github.com/miekg/dns"
)

func (r *Relay) runDNS() error {
	handler := r.newDNSHandler()

	addr := net.TCPAddr{IP: net.IPv4zero, Port: 53}

	if r.cfg.DNSPacketConn != nil {
		// Injected PacketConn (host mode): serve UDP only via netstack.
		// No TCP DNS needed since the netstack handles all traffic.
		server := &dns.Server{
			PacketConn: r.cfg.DNSPacketConn, Handler: handler,
		}
		return server.ActivateAndServe()
	}

	// Interface mode: bind UDP and TCP on port 53.
	udpServer := &dns.Server{
		Addr: addr.String(), Net: "udp", Handler: handler,
	}
	go udpServer.ListenAndServe() //nolint:errcheck

	tcpServer := &dns.Server{
		Addr: addr.String(), Net: "tcp", Handler: handler,
	}
	return tcpServer.ListenAndServe()
}

func (r *Relay) newDNSHandler() dns.HandlerFunc {
	dnsClient := &dns.Client{Timeout: 5 * time.Second}
	resolver := &net.Resolver{}

	return func(w dns.ResponseWriter, req *dns.Msg) {
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

		resp := r.resolveViaSystem(resolver, req)
		if resp == nil && r.cfg.DNSPacketConn == nil {
			// Raw UDP forwarding only works when we have a real network
			// path to the upstream (container/VM mode). In host mode,
			// the netstack can't route to external DNS servers.
			var err error
			resp, _, err = dnsClient.Exchange(req, r.upstreamDNS)
			if err != nil {
				log.Printf("DNS forward error (%s): %v (qtype=%d name=%s)",
					r.upstreamDNS, err, req.Question[0].Qtype,
					req.Question[0].Name)
				m.Rcode = dns.RcodeServerFailure
				w.WriteMsg(m) //nolint:errcheck
				return
			}
		}
		if resp == nil {
			m.Rcode = dns.RcodeServerFailure
			w.WriteMsg(m) //nolint:errcheck
			return
		}
		resp.Id = req.Id
		if err := w.WriteMsg(resp); err != nil {
			log.Printf("DNS proxy error: %s\n", err)
		}
	}
}

func (r *Relay) resolveViaSystem(resolver *net.Resolver, req *dns.Msg) *dns.Msg {
	if len(req.Question) == 0 {
		return nil
	}
	q := req.Question[0]
	name := strings.TrimSuffix(q.Name, ".")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp := new(dns.Msg)
	resp.SetReply(req)

	switch q.Qtype {
	case dns.TypeA, dns.TypeAAAA:
		ips, err := resolver.LookupIPAddr(ctx, name)
		if err != nil {
			return nil
		}
		for _, ip := range ips {
			if v4 := ip.IP.To4(); v4 != nil && q.Qtype == dns.TypeA {
				resp.Answer = append(resp.Answer, &dns.A{
					Hdr: dns.RR_Header{
						Name:   q.Name,
						Rrtype: dns.TypeA,
						Class:  dns.ClassINET,
						Ttl:    60,
					},
					A: v4,
				})
			} else if v4 == nil && q.Qtype == dns.TypeAAAA {
				resp.Answer = append(resp.Answer, &dns.AAAA{
					Hdr: dns.RR_Header{
						Name:   q.Name,
						Rrtype: dns.TypeAAAA,
						Class:  dns.ClassINET,
						Ttl:    60,
					},
					AAAA: ip.IP,
				})
			}
		}
	case dns.TypeCNAME:
		cname, err := resolver.LookupCNAME(ctx, name)
		if err != nil {
			return nil
		}
		resp.Answer = append(resp.Answer, &dns.CNAME{
			Hdr: dns.RR_Header{
				Name:   q.Name,
				Rrtype: dns.TypeCNAME,
				Class:  dns.ClassINET,
				Ttl:    60,
			},
			Target: dns.Fqdn(cname),
		})
	default:
		return nil
	}
	return resp
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
		go r.ServeHTTPConn(conn)
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
		go r.ServeTLSConn(conn)
	}
}
