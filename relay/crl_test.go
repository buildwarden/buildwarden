package relay

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/pem"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/buildwarden/buildwarden/collector"
	"github.com/buildwarden/buildwarden/ledger"
)

// Every leaf names the CRL on the reserved host, so Windows Schannel has
// somewhere to check revocation.
func TestLeafNamesCRL(t *testing.T) {
	r, _, _, _ := startTestRelay(t, collector.Config{}, "")
	cert, err := r.signHost("example.com")
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	got := leaf.CRLDistributionPoints
	if len(got) != 1 || got[0] != "http://artifacts/ca.crl" {
		t.Fatalf("CRLDistributionPoints = %v, want [http://artifacts/ca.crl]", got)
	}
}

// The CRL the leaves name is signed by the served CA, revokes nothing, and
// lasts as long as the CA. It is the same bytes on every fetch and for any
// method, and is relay trust material: serving it adds no ledger record and
// a POST to its path uploads nothing.
func TestRelayServesCACRL(t *testing.T) {
	baseline := ledgerRecords(t, func(*http.Client) {})

	var crlDER []byte
	var ca *x509.Certificate
	got := ledgerRecords(t, func(client *http.Client) {
		crlDER = fetch(t, client, http.MethodGet, "http://artifacts/ca.crl?x=1")
		again := fetch(t, client, http.MethodGet, "http://artifacts/ca.crl")
		if !bytes.Equal(again, crlDER) {
			t.Error("second GET of /ca.crl returned different bytes")
		}
		posted := fetch(t, client, http.MethodPost, "http://artifacts/ca.crl")
		if !bytes.Equal(posted, crlDER) {
			t.Error("POST to /ca.crl did not return the CRL")
		}
		block, _ := pem.Decode(fetch(t, client, http.MethodGet, "http://artifacts/ca.pem"))
		var err error
		if ca, err = x509.ParseCertificate(block.Bytes); err != nil {
			t.Fatal(err)
		}
	})

	crl, err := x509.ParseRevocationList(crlDER)
	if err != nil {
		t.Fatalf("parsing CRL: %v", err)
	}
	if err := crl.CheckSignatureFrom(ca); err != nil {
		t.Errorf("CRL not signed by the served CA: %v", err)
	}
	if n := len(crl.RevokedCertificateEntries); n != 0 {
		t.Errorf("CRL revokes %d certificates, want 0", n)
	}
	if crl.NextUpdate.Before(ca.NotAfter) {
		t.Errorf("CRL NextUpdate %v is before the CA's NotAfter %v",
			crl.NextUpdate, ca.NotAfter)
	}
	if got != baseline {
		t.Errorf("ledger has %d records, want %d: serving the CRL must not record",
			got, baseline)
	}
}

// ledgerRecords runs a build that does use, then exits, and returns how many
// records its verified ledger holds. It also checks no artifact landed.
func ledgerRecords(t *testing.T, use func(*http.Client)) int {
	t.Helper()
	_, col, client, out := startTestRelay(t, collector.Config{}, "")
	use(client)
	getExpectCode(t, client, "http://artifacts/exit?code=0", 200)
	if _, err := col.Wait(context.Background(), 0); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(out, "artifacts", "ca.crl")); err == nil {
		t.Error("a POST to /ca.crl landed as an artifact")
	}
	data, err := os.ReadFile(filepath.Join(out, "ledger"))
	if err != nil {
		t.Fatal(err)
	}
	res, err := ledger.Verify(data)
	if err != nil || !res.Valid {
		t.Fatalf("ledger does not verify: %v", err)
	}
	return res.TotalRecords
}

// fetch sends one request through the relay and returns the 200 body.
func fetch(t *testing.T, c *http.Client, method, url string) []byte {
	t.Helper()
	req, _ := http.NewRequest(method, url, strings.NewReader(""))
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("%s %s = %d", method, url, resp.StatusCode)
	}
	return body
}
