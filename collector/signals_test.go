package collector

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"
)

// postSignal issues a POST with an optional bearer token and body.
func postSignal(t *testing.T, url, token string, body []byte) *http.Response {
	t.Helper()
	var r io.Reader
	if body != nil {
		r = bytes.NewReader(body)
	}
	req, err := http.NewRequest(http.MethodPost, url, r)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	return resp
}

func TestReadyFiresCallback(t *testing.T) {
	var fired bool
	srv, _ := newTestServer(t, Config{Token: "t", OnReady: func() { fired = true }})
	resp := postSignal(t, srv.URL+"/v1/ready", "t", nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if !fired {
		t.Fatal("OnReady was not invoked")
	}
}

func TestCompleteFiresCallbackAndStatus(t *testing.T) {
	var got CompletionSignal
	srv, _ := newTestServer(t, Config{Token: "t", OnComplete: func(s CompletionSignal) { got = s }})

	body := []byte(`{"exit_code":42,"message":"done","error":""}`)
	resp := postSignal(t, srv.URL+"/v1/complete", "t", body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("complete status = %d, want 200", resp.StatusCode)
	}
	if got.ExitCode != 42 || got.Message != "done" {
		t.Fatalf("OnComplete signal = %+v, want ExitCode 42 / Message \"done\"", got)
	}
	if got.ReceivedAt.IsZero() {
		t.Fatal("OnComplete signal has zero ReceivedAt")
	}

	// A subsequent GET /v1/status returns the stored signal.
	sresp, err := http.Get(srv.URL + "/v1/status")
	if err != nil {
		t.Fatalf("get status: %v", err)
	}
	defer sresp.Body.Close()
	if sresp.StatusCode != http.StatusOK {
		t.Fatalf("status endpoint = %d, want 200", sresp.StatusCode)
	}
	var landed CompletionSignal
	if err := json.NewDecoder(sresp.Body).Decode(&landed); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	if landed.ExitCode != 42 || landed.Message != "done" {
		t.Fatalf("status signal = %+v, want ExitCode 42 / Message \"done\"", landed)
	}
}

func TestStatusRunningBeforeCompletion(t *testing.T) {
	srv, _ := newTestServer(t, Config{Token: "t"})
	resp, err := http.Get(srv.URL + "/v1/status")
	if err != nil {
		t.Fatalf("get status: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	b, _ := io.ReadAll(resp.Body)
	var m map[string]string
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("decode: %v (body %q)", err, b)
	}
	if m["state"] != "running" {
		t.Fatalf("status body = %q, want state=running", b)
	}
}

func TestCompleteMalformedBody(t *testing.T) {
	srv, _ := newTestServer(t, Config{Token: "t"})
	resp := postSignal(t, srv.URL+"/v1/complete", "t", []byte(`{not json`))
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("malformed body: status = %d, want 400", resp.StatusCode)
	}
}

func TestSignalAuthEnforced(t *testing.T) {
	srv, _ := newTestServer(t, Config{Token: "sekret"})
	body := []byte(`{"exit_code":0}`)

	for _, tc := range []struct {
		name, token string
	}{
		{"wrong token", "nope"},
		{"no token", ""},
	} {
		r1 := postSignal(t, srv.URL+"/v1/ready", tc.token, nil)
		r1.Body.Close()
		if r1.StatusCode != http.StatusUnauthorized {
			t.Errorf("ready %s: status = %d, want 401", tc.name, r1.StatusCode)
		}
		r2 := postSignal(t, srv.URL+"/v1/complete", tc.token, body)
		r2.Body.Close()
		if r2.StatusCode != http.StatusUnauthorized {
			t.Errorf("complete %s: status = %d, want 401", tc.name, r2.StatusCode)
		}
	}
}

func TestDoneDeliversSignal(t *testing.T) {
	c, err := New(Config{OutputDir: t.TempDir()})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	// Drive completion directly through the recording path used by the handler.
	c.recordCompletion(CompletionSignal{ExitCode: 7, Message: "ok", ReceivedAt: time.Now()})

	select {
	case sig := <-c.Done():
		if sig.ExitCode != 7 || sig.Message != "ok" {
			t.Fatalf("Done signal = %+v, want ExitCode 7 / Message \"ok\"", sig)
		}
	case <-time.After(time.Second):
		t.Fatal("Done did not deliver the completion signal")
	}
	// Status re-reads the recorded signal after Done was consumed.
	if got, ok := c.Status(); !ok || got.ExitCode != 7 {
		t.Fatalf("Status after Done = %+v ok=%v, want ExitCode 7", got, ok)
	}
}
