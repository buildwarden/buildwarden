package winguest

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/buildwarden/buildwarden/driver"
)

// A prepared layer built from an evaluation image runs on the guest's
// wall-clock initial grace period (10 days; prepare has no network, so it is
// never activated). It is re-prepared in the background once RefreshAfter
// old, which leaves a margin for long builds, and a build refuses to start
// with less than MinGrace left. See docs/design/hyperv-windows-prepare.md.
const (
	DefaultRefreshAfter = 7 * 24 * time.Hour
	DefaultMinGrace     = 24 * time.Hour
)

// Freshness is what the rules need to know about a prepared layer: when it
// was prepared and, for an evaluation edition, when its grace ends (RFC 3339;
// empty otherwise).
type Freshness struct {
	PreparedAt string
	GraceEnd   string
}

// CheckFreshness decides whether a build may start on the layer at now, and
// whether a background refresh is due. A layer with no grace end only ages
// into a refresh. fix is the command that prepares a fresh layer.
func CheckFreshness(
	f Freshness, now time.Time, refreshAfter, minGrace time.Duration, fix string,
) (refresh bool, err error) {
	if refreshAfter <= 0 {
		refreshAfter = DefaultRefreshAfter
	}
	if minGrace <= 0 {
		minGrace = DefaultMinGrace
	}
	if f.GraceEnd != "" {
		end, perr := time.Parse(time.RFC3339, f.GraceEnd)
		if perr == nil && end.Sub(now) < minGrace {
			return true, driver.Fixable(fix,
				"the prepared Windows image's evaluation grace ends %s (%s left, "+
					"less than the %s minimum); prepare a fresh one first",
				end.Format(time.RFC3339), RoundAge(end.Sub(now)),
				RoundAge(minGrace))
		}
	}
	at, perr := time.Parse(time.RFC3339, f.PreparedAt)
	if perr != nil {
		return true, nil
	}
	return now.Sub(at) >= refreshAfter, nil
}

// Age is how long ago the layer was prepared (zero if unknown).
func (f Freshness) Age(now time.Time) time.Duration {
	at, err := time.Parse(time.RFC3339, f.PreparedAt)
	if err != nil {
		return 0
	}
	return now.Sub(at)
}

// RoundAge formats an age for people: days past two days, else h/m.
func RoundAge(d time.Duration) string {
	if d >= 48*time.Hour {
		return fmt.Sprintf("%.1fd", d.Hours()/24)
	}
	return d.Round(time.Minute).String()
}

// GraceEnd turns the guest's license report (minutes left at a UTC instant)
// into an absolute end, for evaluation editions only; "" otherwise.
func GraceEnd(report map[string]string) string {
	if !strings.Contains(report[KVPLicenseChannel], "TIMEBASED_EVAL") {
		return ""
	}
	at, err := time.Parse(time.RFC3339Nano, report[KVPReportedAt])
	if err != nil {
		return ""
	}
	minutes, err := strconv.Atoi(report[KVPGraceMinutes])
	if err != nil {
		return ""
	}
	return at.Add(time.Duration(minutes) * time.Minute).UTC().Format(time.RFC3339)
}
