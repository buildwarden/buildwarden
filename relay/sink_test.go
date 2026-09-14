package relay

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

// capturedReq records what the emulated collector received.
type capturedReq struct {
	method string
	path   string
	auth   string
	body   []byte
}

// emulatedCollector is a bare net/http/httptest.Server that stands in for the
// real collector. It does NOT import the collector package (a sibling change is
// editing it); it only records what the relay's httpSink SENT so the tests can
// assert method, path, Authorization header, and body.
type emulatedCollector struct {
	mu   sync.Mutex
	reqs []capturedReq
	srv  *httptest.Server
}

func newEmulatedCollector() *emulatedCollector {
	c := &emulatedCollector{}
	mux := http.NewServeMux()
	record := func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		c.mu.Lock()
		c.reqs = append(c.reqs, capturedReq{
			method: r.Method,
			path:   r.URL.Path,
			auth:   r.Header.Get("Authorization"),
			body:   body,
		})
		c.mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}
	mux.HandleFunc("/v1/artifacts/", record)
	mux.HandleFunc("/v1/ledger", record)
	mux.HandleFunc("/v1/output", record)
	mux.HandleFunc("/v1/ready", record)
	mux.HandleFunc("/v1/complete", record)
	c.srv = httptest.NewServer(mux)
	return c
}

func (c *emulatedCollector) close() { c.srv.Close() }

func (c *emulatedCollector) find(path string) (capturedReq, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, r := range c.reqs {
		if r.path == path {
			return r, true
		}
	}
	return capturedReq{}, false
}

const testToken = "test-bearer-token-123"

func TestHTTPSink_ArtifactRoundTrip(t *testing.T) {
	col := newEmulatedCollector()
	defer col.close()

	sink := newHTTPSink(col.srv.URL, testToken)
	payload := []byte("hello artifact body")
	hasher := NewStreamingHasher(defaultHashes)

	hashBlock, size, err := sink.PutArtifact("dist/out.bin", bytes.NewReader(payload), hasher)
	if err != nil {
		t.Fatalf("PutArtifact: %v", err)
	}
	if size != int64(len(payload)) {
		t.Fatalf("size = %d, want %d", size, len(payload))
	}

	// The hash block must equal a one-shot hash of the identical bytes.
	want := NewStreamingHasher(defaultHashes)
	want.Write(payload) //nolint:errcheck
	wantBlock, _ := want.Finish()
	if !bytes.Equal(hashBlock, wantBlock) {
		t.Fatal("hash block does not match one-shot hash of the sent body")
	}

	got, ok := col.find("/v1/artifacts/dist/out.bin")
	if !ok {
		t.Fatalf("collector never received the artifact; saw %+v", col.reqs)
	}
	if got.method != http.MethodPut {
		t.Fatalf("artifact method = %s, want PUT", got.method)
	}
	if got.auth != "Bearer "+testToken {
		t.Fatalf("artifact auth = %q, want Bearer %s", got.auth, testToken)
	}
	if !bytes.Equal(got.body, payload) {
		t.Fatalf("artifact body = %q, want %q", got.body, payload)
	}
}

func TestHTTPSink_LedgerFlush(t *testing.T) {
	col := newEmulatedCollector()
	defer col.close()

	sink := newHTTPSink(col.srv.URL, testToken)

	// Serialize a real ledger into the sink's buffer, then flush.
	w, err := sink.LedgerWriter()
	if err != nil {
		t.Fatalf("LedgerWriter: %v", err)
	}
	l, err := NewLedger(LedgerConfig{Writer: w, Environment: map[string]any{"type": "test"}})
	if err != nil {
		t.Fatalf("NewLedger: %v", err)
	}
	openSig := l.Open(SchemaNoMetadata, nil)
	body := []byte("payload")
	l.Close(openSig, int64(len(body)), l.ComputeHashBlock(body), SchemaNoMetadata, nil)
	l.Finish()

	if _, ok := col.find("/v1/ledger"); ok {
		t.Fatal("ledger PUT fired before FlushLedger")
	}
	if err := sink.FlushLedger(); err != nil {
		t.Fatalf("FlushLedger: %v", err)
	}

	got, ok := col.find("/v1/ledger")
	if !ok {
		t.Fatalf("collector never received the ledger; saw %+v", col.reqs)
	}
	if got.method != http.MethodPut {
		t.Fatalf("ledger method = %s, want PUT", got.method)
	}
	if got.auth != "Bearer "+testToken {
		t.Fatalf("ledger auth = %q, want Bearer %s", got.auth, testToken)
	}
	// Body must be a valid signed ledger (BLDL magic).
	if len(got.body) < 4 || string(got.body[:4]) != "BLDL" {
		t.Fatalf("ledger body does not start with BLDL magic")
	}
}

func TestHTTPSink_LargeArtifactStreams(t *testing.T) {
	col := newEmulatedCollector()
	defer col.close()

	sink := newHTTPSink(col.srv.URL, testToken)
	const sz = 10 * 1024 * 1024 // ~10MB
	payload := make([]byte, sz)
	if _, err := rand.Read(payload); err != nil {
		t.Fatalf("rand: %v", err)
	}
	hasher := NewStreamingHasher(defaultHashes)

	hashBlock, size, err := sink.PutArtifact("big.bin", bytes.NewReader(payload), hasher)
	if err != nil {
		t.Fatalf("PutArtifact: %v", err)
	}
	if size != sz {
		t.Fatalf("size = %d, want %d", size, sz)
	}

	want := NewStreamingHasher(defaultHashes)
	want.Write(payload) //nolint:errcheck
	wantBlock, _ := want.Finish()
	if !bytes.Equal(hashBlock, wantBlock) {
		t.Fatal("large artifact hash block mismatch")
	}

	got, ok := col.find("/v1/artifacts/big.bin")
	if !ok {
		t.Fatalf("collector never received the large artifact")
	}
	if len(got.body) != sz {
		t.Fatalf("received %d bytes, want %d", len(got.body), sz)
	}
	if !bytes.Equal(got.body, payload) {
		t.Fatal("large artifact body corrupted in transit")
	}
}

func TestHTTPSink_Output(t *testing.T) {
	col := newEmulatedCollector()
	defer col.close()

	sink := newHTTPSink(col.srv.URL, testToken)
	w, finish, err := sink.OutputWriter()
	if err != nil {
		t.Fatalf("OutputWriter: %v", err)
	}
	line := []byte("build log line 1\nbuild log line 2\n")
	if _, err := w.Write(line); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := finish(); err != nil {
		t.Fatalf("finish: %v", err)
	}

	got, ok := col.find("/v1/output")
	if !ok {
		t.Fatalf("collector never received output")
	}
	if got.method != http.MethodPost {
		t.Fatalf("output method = %s, want POST", got.method)
	}
	if got.auth != "Bearer "+testToken {
		t.Fatalf("output auth = %q, want Bearer %s", got.auth, testToken)
	}
	if !bytes.Equal(got.body, line) {
		t.Fatalf("output body = %q, want %q", got.body, line)
	}
}

func TestHTTPSink_ReadyAndComplete(t *testing.T) {
	col := newEmulatedCollector()
	defer col.close()

	sink := newHTTPSink(col.srv.URL, testToken)

	if err := sink.PostReady(); err != nil {
		t.Fatalf("PostReady: %v", err)
	}
	ready, ok := col.find("/v1/ready")
	if !ok {
		t.Fatalf("collector never received ready")
	}
	if ready.method != http.MethodPost {
		t.Fatalf("ready method = %s, want POST", ready.method)
	}
	if ready.auth != "Bearer "+testToken {
		t.Fatalf("ready auth = %q, want Bearer %s", ready.auth, testToken)
	}
	if len(ready.body) != 0 {
		t.Fatalf("ready body = %q, want empty", ready.body)
	}

	if err := sink.PostComplete(7, "done", "boom"); err != nil {
		t.Fatalf("PostComplete: %v", err)
	}
	comp, ok := col.find("/v1/complete")
	if !ok {
		t.Fatalf("collector never received complete")
	}
	if comp.method != http.MethodPost {
		t.Fatalf("complete method = %s, want POST", comp.method)
	}
	if comp.auth != "Bearer "+testToken {
		t.Fatalf("complete auth = %q, want Bearer %s", comp.auth, testToken)
	}
	var payload struct {
		ExitCode int    `json:"exit_code"`
		Message  string `json:"message"`
		Error    string `json:"error"`
	}
	if err := json.Unmarshal(comp.body, &payload); err != nil {
		t.Fatalf("complete body not JSON: %v (%q)", err, comp.body)
	}
	if payload.ExitCode != 7 || payload.Message != "done" || payload.Error != "boom" {
		t.Fatalf("complete payload = %+v, want {7 done boom}", payload)
	}
}

// TestLocalSink_NoOpSignals confirms the local sink's signal methods are
// no-ops (return nil) and never touch the network.
func TestLocalSink_NoOpSignals(t *testing.T) {
	s := &localSink{r: &Relay{}}
	if err := s.PostReady(); err != nil {
		t.Fatalf("localSink.PostReady = %v, want nil", err)
	}
	if err := s.PostComplete(0, "", ""); err != nil {
		t.Fatalf("localSink.PostComplete = %v, want nil", err)
	}
}
