package hyperv

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

// TestLiveDefaultImageResolves checks the registry's default link against
// Microsoft's real endpoint with a HEAD request only (nothing is downloaded):
// it must land on a Microsoft host and name a VHDX. Gated on
// WARDEN_HYPERV_LIVE_FETCH=1 (needs the network); never runs in normal CI.
func TestLiveDefaultImageResolves(t *testing.T) {
	if os.Getenv("WARDEN_HYPERV_LIVE_FETCH") != "1" {
		t.Skip("set WARDEN_HYPERV_LIVE_FETCH=1 to resolve the real default image link")
	}
	spec, _ := LookupEvalImage(DefaultEvalImage)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	r, err := resolveRemote(ctx, spec.URL)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%s -> %s (%d bytes, ETag %s, Last-Modified %s)",
		spec.URL, r.URL, r.Size, r.ETag, r.LastModified)
	if !strings.HasSuffix(strings.ToLower(r.Host), officialHostSuffix) {
		t.Errorf("resolved host %q is not a Microsoft host", r.Host)
	}
	if gen, err := diskGeneration(r.Name); err != nil || gen != 2 || r.Size <= 0 {
		t.Errorf("resolved %q (gen %d, %d bytes, %v), want a sized Gen2 VHDX",
			r.Name, gen, r.Size, err)
	}
}
