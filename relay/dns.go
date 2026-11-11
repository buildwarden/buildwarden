package relay

import (
	"fmt"
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

// dnsQueryTimeout is how long the relay waits for one upstream resolver to
// answer a query before it asks the next one in its list.
var dnsQueryTimeout = 5 * time.Second

func (r *Relay) newDNSHandler() dns.HandlerFunc {
	dnsClient := &dns.Client{Timeout: dnsQueryTimeout}

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

		resp := r.forwardDNS(dnsClient, req)
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

// forwardDNS asks the relay's upstream resolvers for req in list order and
// returns the first answer, or nil when none answers. A server that gives no
// answer within the per-query timeout is logged and the next one is asked:
// the list's own order is the one named fallback (relaycfg's package doc), and
// it never leaves the list.
func (r *Relay) forwardDNS(client *dns.Client, req *dns.Msg) *dns.Msg {
	q := "(no question)"
	if len(req.Question) > 0 {
		q = fmt.Sprintf("qtype=%d name=%s", req.Question[0].Qtype, req.Question[0].Name)
	}
	for _, server := range r.upstreamDNS {
		resp, _, err := client.Exchange(req, server)
		if err == nil {
			return resp
		}
		log.Printf("relay: upstream resolver %s did not answer: %v (%s)", server, err, q)
	}
	return nil
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
