package hyperv

import (
	"strings"
	"testing"
	"time"
)

func TestCheckPreparedFreshness(t *testing.T) {
	now := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	prepared := func(daysAgo int) PreparedBase {
		at := now.Add(-time.Duration(daysAgo) * 24 * time.Hour)
		return PreparedBase{
			PreparedAt: at.Format(time.RFC3339),
			GraceEnd:   at.Add(10 * 24 * time.Hour).Format(time.RFC3339),
		}
	}
	cases := []struct {
		name         string
		pb           PreparedBase
		refreshAfter time.Duration
		wantRefresh  bool
		wantErr      string
	}{
		{name: "fresh", pb: prepared(2)},
		{name: "seven days old refreshes", pb: prepared(7), wantRefresh: true},
		{name: "configured refresh age", pb: prepared(3), refreshAfter: 72 * time.Hour,
			wantRefresh: true},
		{name: "under a day of grace refuses", pb: prepared(9), /* 24h left, not < */
			wantRefresh: true},
		{name: "expired refuses", pb: prepared(10), wantRefresh: true,
			wantErr: "evaluation grace ends"},
		{name: "non-eval only ages", pb: PreparedBase{
			PreparedAt: now.Add(-30 * 24 * time.Hour).Format(time.RFC3339),
		}, wantRefresh: true},
		{name: "unreadable age refreshes", pb: PreparedBase{PreparedAt: "?"},
			wantRefresh: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			refresh, err := checkPreparedFreshness(c.pb, now, c.refreshAfter, 0)
			if refresh != c.wantRefresh {
				t.Errorf("refresh = %v, want %v", refresh, c.wantRefresh)
			}
			if c.wantErr == "" && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if c.wantErr != "" &&
				(err == nil || !strings.Contains(err.Error(), c.wantErr)) {
				t.Errorf("err = %v, want %q", err, c.wantErr)
			}
		})
	}
}

func TestCheckPreparedFreshnessMinGrace(t *testing.T) {
	now := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	pb := PreparedBase{
		PreparedAt: now.Add(-6 * 24 * time.Hour).Format(time.RFC3339),
		GraceEnd:   now.Add(4 * 24 * time.Hour).Format(time.RFC3339),
	}
	if _, err := checkPreparedFreshness(pb, now, 0, 5*24*time.Hour); err == nil {
		t.Fatal("4 days left with a 5-day minimum should refuse the build")
	}
	if _, err := checkPreparedFreshness(pb, now, 0, 3*24*time.Hour); err != nil {
		t.Fatalf("4 days left with a 3-day minimum: %v", err)
	}
}
