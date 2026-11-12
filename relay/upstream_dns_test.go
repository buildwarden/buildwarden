package relay

import (
	"bytes"
	"context"
	"log"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/miekg/dns"

	"github.com/buildwarden/buildwarden/collector"
)

// lockedBuffer is a bytes.Buffer safe for the logger's writes and the test's
// reads from different goroutines.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// captureLog sends the standard logger to a buffer until the test ends. The
// logger is global, so a test that calls it must not run in parallel. Call it
// before starting anything that logs, so its cleanup runs last.
func captureLog(t *testing.T) *lockedBuffer {
	t.Helper()
	buf := &lockedBuffer{}
	flags := log.Flags()
	log.SetOutput(buf)
	log.SetFlags(0)
	t.Cleanup(func() {
		log.SetOutput(os.Stderr)
		log.SetFlags(flags)
	})
	return buf
}

// fakeResolver is a loopback UDP DNS server that answers every A query with
// answer, or never answers when silent. It counts the queries it receives.
type fakeResolver struct {
	addr string
	hits atomic.Int32
}

func startFakeResolver(t *testing.T, answer string, silent bool) *fakeResolver {
	t.Helper()
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeResolver{addr: pc.LocalAddr().String()}
	srv := &dns.Server{PacketConn: pc, Handler: dns.HandlerFunc(
		func(w dns.ResponseWriter, req *dns.Msg) {
			f.hits.Add(1)
			if silent {
				return
			}
			m := new(dns.Msg)
			m.SetReply(req)
			if q := req.Question[0]; q.Qtype == dns.TypeA {
				m.Answer = append(m.Answer, &dns.A{
					Hdr: dns.RR_Header{Name: q.Name, Rrtype: dns.TypeA,
						Class: dns.ClassINET, Ttl: 60},
					A: net.ParseIP(answer),
				})
			}
			_ = w.WriteMsg(m)
		})}
	go func() { _ = srv.ActivateAndServe() }()
	t.Cleanup(func() { _ = srv.Shutdown() })
	return f
}

// firstA returns the address of resp's single A answer.
func firstA(resp *dns.Msg) (string, bool) {
	if len(resp.Answer) != 1 {
		return "", false
	}
	a, ok := resp.Answer[0].(*dns.A)
	if !ok {
		return "", false
	}
	return a.A.String(), true
}

// shortQueryTimeout makes a silent resolver time out quickly.
func shortQueryTimeout(t *testing.T) {
	t.Helper()
	old := dnsQueryTimeout
	dnsQueryTimeout = 300 * time.Millisecond
	t.Cleanup(func() { dnsQueryTimeout = old })
}

// TestDNSHandlerForwardsToListInOrder checks the guest's queries go to the
// list only: the first entry is asked, and the second answers when the first
// does not. A server named by UPSTREAM_DNS is never asked.
func TestDNSHandlerForwardsToListInOrder(t *testing.T) {
	logs := captureLog(t)
	shortQueryTimeout(t)
	silent := startFakeResolver(t, "", true)
	second := startFakeResolver(t, "192.0.2.10", false)
	env := startFakeResolver(t, "192.0.2.99", false)
	t.Setenv("UPSTREAM_DNS", env.addr)

	r := &Relay{upstreamDNS: []string{silent.addr, second.addr}}
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := &dns.Server{PacketConn: pc, Handler: r.newDNSHandler()}
	go func() { _ = srv.ActivateAndServe() }()
	defer func() { _ = srv.Shutdown() }()

	q := new(dns.Msg)
	q.SetQuestion("pkg.example.", dns.TypeA)
	client := &dns.Client{Timeout: 5 * time.Second}
	resp, _, err := client.Exchange(q, pc.LocalAddr().String())
	if err != nil {
		t.Fatalf("query through the relay: %v", err)
	}
	a, ok := firstA(resp)
	if !ok || a != "192.0.2.10" {
		t.Fatalf("answer = %v, want the second resolver's 192.0.2.10", resp.Answer)
	}
	if silent.hits.Load() != 1 || second.hits.Load() != 1 {
		t.Fatalf("hits: first %d, second %d; want 1 each, in order",
			silent.hits.Load(), second.hits.Load())
	}
	if env.hits.Load() != 0 {
		t.Fatalf("the UPSTREAM_DNS server was asked %d times", env.hits.Load())
	}
	var timeouts int
	for _, line := range strings.Split(logs.String(), "\n") {
		if strings.Contains(line, "upstream resolver "+silent.addr+" did not answer") {
			timeouts++
		}
		if strings.Contains(line, second.addr) {
			t.Errorf("a log line names the answering resolver: %q", line)
		}
	}
	if timeouts != 1 {
		t.Fatalf("logged %d timeouts for the silent resolver, want 1:\n%s",
			timeouts, logs.String())
	}
}

// TestDNSHandlerNoAnswerIsServfail checks no resolver outside the list stands
// in when every entry is silent.
func TestDNSHandlerNoAnswerIsServfail(t *testing.T) {
	shortQueryTimeout(t)
	silent := startFakeResolver(t, "", true)
	r := &Relay{upstreamDNS: []string{silent.addr}}
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := &dns.Server{PacketConn: pc, Handler: r.newDNSHandler()}
	go func() { _ = srv.ActivateAndServe() }()
	defer func() { _ = srv.Shutdown() }()
	q := new(dns.Msg)
	q.SetQuestion("pkg.example.", dns.TypeA)
	client := &dns.Client{Timeout: 5 * time.Second}
	resp, _, err := client.Exchange(q, pc.LocalAddr().String())
	if err != nil {
		t.Fatalf("query through the relay: %v", err)
	}
	if resp.Rcode != dns.RcodeServerFailure {
		t.Fatalf("rcode = %s, want SERVFAIL", dns.RcodeToString[resp.Rcode])
	}
}

// TestUpstreamDialResolvesThroughList checks the relay's own dials (the plain
// transport's and the filtering one's) resolve names through the list, the
// second entry answering when the first does not, and never through the
// server UPSTREAM_DNS names.
func TestUpstreamDialResolvesThroughList(t *testing.T) {
	shortQueryTimeout(t)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()
	_, port, _ := net.SplitHostPort(ln.Addr().String())

	tests := []struct {
		name string
		cfg  Config
	}{
		{"plain", Config{}},
		{"filtering", Config{BuildLink: netip.MustParsePrefix("100.127.255.252/30")}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			silent := startFakeResolver(t, "", true)
			second := startFakeResolver(t, "127.0.0.1", false)
			env := startFakeResolver(t, "192.0.2.99", false)
			t.Setenv("UPSTREAM_DNS", env.addr)
			r := &Relay{cfg: tt.cfg, upstreamDNS: []string{silent.addr, second.addr}}
			r.buildTransport()
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			conn, err := r.transport.DialContext(ctx, "tcp",
				net.JoinHostPort("upstream.example", port))
			if err != nil {
				t.Fatalf("dial by name: %v", err)
			}
			conn.Close()
			if silent.hits.Load() == 0 || second.hits.Load() == 0 {
				t.Fatalf("hits: first %d, second %d; want both asked",
					silent.hits.Load(), second.hits.Load())
			}
			if env.hits.Load() != 0 {
				t.Fatalf("the UPSTREAM_DNS server was asked %d times",
					env.hits.Load())
			}
		})
	}
}

func TestUpstreamLogLine(t *testing.T) {
	list := []netip.Addr{netip.MustParseAddr("10.20.0.53"), netip.MustParseAddr("10.20.0.54")}
	for _, src := range []string{SourceClaim, SourceResolvConf} {
		want := "relay: upstream resolvers: 10.20.0.53, 10.20.0.54 (" + src + ")"
		if got := upstreamLogLine(list, src); got != want {
			t.Errorf("upstreamLogLine = %q, want %q", got, want)
		}
	}
}

func TestStartRequiresUpstreamDNS(t *testing.T) {
	list := []netip.Addr{netip.MustParseAddr("127.0.0.1")}
	tests := []struct {
		name string
		cfg  Config
		want string
	}{
		{"no list", Config{CollectorURL: "http://x", UpstreamDNSSource: SourceClaim},
			"UpstreamDNS is required"},
		{"no source", Config{CollectorURL: "http://x", UpstreamDNS: list},
			"UpstreamDNSSource is required"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Start(tt.cfg)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Start err = %v, want %q", err, tt.want)
			}
		})
	}
}

// TestNoBuiltInResolver scans the relay's sources for the public resolver
// and the environment variable that used to stand in for the claim's list.
func TestNoBuiltInResolver(t *testing.T) {
	banned := []string{"8.8.8" + ".8", "UPSTREAM" + "_DNS"}
	for _, dir := range []string{".", filepath.Join("..", "cmd", "relay")} {
		files, err := filepath.Glob(filepath.Join(dir, "*.go"))
		if err != nil {
			t.Fatal(err)
		}
		for _, f := range files {
			if strings.HasSuffix(f, "_test.go") {
				continue
			}
			b, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			for _, s := range banned {
				if strings.Contains(string(b), s) {
					t.Errorf("%s contains %q", f, s)
				}
			}
		}
	}
}

// TestStartLogsUpstreamResolvers checks a started relay logs its resolver
// list and where the list came from.
func TestStartLogsUpstreamResolvers(t *testing.T) {
	for _, src := range []string{SourceClaim, SourceResolvConf} {
		t.Run(src, func(t *testing.T) {
			logs := captureLog(t)
			startTestRelay(t, collector.Config{}, "", func(c *Config) {
				c.UpstreamDNS = []netip.Addr{netip.MustParseAddr("127.0.0.1")}
				c.UpstreamDNSSource = src
			})
			want := "relay: upstream resolvers: 127.0.0.1 (" + src + ")"
			if !strings.Contains(logs.String(), want) {
				t.Fatalf("log lacks %q:\n%s", want, logs.String())
			}
		})
	}
}

// TestDirectCollectorURLResolvesThroughList checks a direct CollectorURL that
// names a host, with no CollectorTransport, is dialed through the relay's
// resolver list: collector.example resolves only through the fake resolver
// the list names, and the collector receives the relay's ledger and ready.
func TestDirectCollectorURLResolvesThroughList(t *testing.T) {
	if os.Getenv("HTTP_PROXY") != "" || os.Getenv("http_proxy") != "" {
		// The cloned default transport would send the request to the proxy.
		t.Skip("HTTP_PROXY is set; the collector request would go to the proxy")
	}
	res := startFakeResolver(t, "127.0.0.1", false)
	_, port, err := net.SplitHostPort(res.addr)
	if err != nil {
		t.Fatal(err)
	}
	p, err := strconv.ParseUint(port, 10, 16)
	if err != nil {
		t.Fatal(err)
	}
	old := upstreamDNSPort
	upstreamDNSPort = uint16(p)
	t.Cleanup(func() { upstreamDNSPort = old })

	r, _, _, _ := startTestRelay(t, collector.Config{}, "", func(c *Config) {
		c.CollectorURL = strings.Replace(c.CollectorURL, "127.0.0.1",
			"collector.example", 1)
		c.CollectorTransport = nil
	})
	if !strings.HasPrefix(r.cfg.CollectorURL, "http://collector.example:") {
		t.Fatalf("CollectorURL = %s, want a host name", r.cfg.CollectorURL)
	}
	if res.hits.Load() == 0 {
		t.Fatal("the collector reached the relay without asking the resolver list")
	}
}
