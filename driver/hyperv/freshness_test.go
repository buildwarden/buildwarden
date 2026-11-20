package hyperv

import (
	"bytes"
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/buildwarden/buildwarden/driver"
)

// withAgedPrepared makes the active prepared base two hours old, past the
// one-minute refresh age the tests ask for, with no evaluation grace end.
func withAgedPrepared(t *testing.T) {
	t.Helper()
	base := withBase(t)
	m, err := loadEvalManifest()
	if err != nil {
		t.Fatal(err)
	}
	pb := PreparedBase{
		Path: filepath.Join(t.TempDir(), "prepared.vhdx"), Base: base.Name,
		PreparedAt: time.Now().Add(-2 * time.Hour).UTC().Format(time.RFC3339),
	}
	m.Prepared = append(m.Prepared, pb)
	m.ActivePrepared = pb.Path
	if err := saveEvalManifest(m); err != nil {
		t.Fatal(err)
	}
}

// stubStart replaces the background start with one that records its args and
// returns err.
func stubStart(t *testing.T, err error) *[]string {
	t.Helper()
	var got []string
	orig := startDetached
	t.Cleanup(func() { startDetached = orig })
	startDetached = func(_ string, args ...string) error {
		got = args
		return err
	}
	return &got
}

var refreshDue = &driver.BuildRequest{ImageRefreshAfter: time.Minute}

// A due refresh that does not start is reported with the error and the
// command that refreshes by hand, and the build goes on.
func TestEnsureFreshBaseReportsFailedStart(t *testing.T) {
	withAgedPrepared(t)
	stubStart(t, errors.New("access denied"))

	var out bytes.Buffer
	if err := ensureFreshBase(refreshDue, &out); err != nil {
		t.Fatalf("ensureFreshBase: %v", err)
	}
	want := "[warden] prepared base is 2h old, and its background refresh did not " +
		"start: access denied; run `warden image prepare --driver hyperv` to refresh it\n"
	if out.String() != want {
		t.Errorf("output = %q, want %q", out.String(), want)
	}
}

// A due refresh that starts says so, and runs the hyperv prepare.
func TestEnsureFreshBaseStartsRefresh(t *testing.T) {
	withAgedPrepared(t)
	args := stubStart(t, nil)

	var out bytes.Buffer
	if err := ensureFreshBase(refreshDue, &out); err != nil {
		t.Fatalf("ensureFreshBase: %v", err)
	}
	if want := []string{"image", "prepare", "--driver", "hyperv"}; !slices.Equal(*args, want) {
		t.Errorf("args = %q, want %q", *args, want)
	}
	if !strings.HasPrefix(out.String(),
		"[warden] prepared base is 2h old; refreshing it in the background (log: ") {
		t.Errorf("output = %q, want the refreshing line", out.String())
	}
}
