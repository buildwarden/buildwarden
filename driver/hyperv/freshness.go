package hyperv

import (
	"fmt"
	"io"
	"time"

	"github.com/buildwarden/buildwarden/driver"
)

// Prepared bases built from an evaluation image run on the guest's wall-clock
// grace period (10 days, never activated: prepare has no network). A base is
// re-prepared in the background once it is refreshAfter old, which leaves a
// margin for long builds, and a build refuses to start with less than
// minGrace left. See docs/design/hyperv-windows-prepare.md.
const (
	defaultRefreshAfter = 7 * 24 * time.Hour
	defaultMinGrace     = 24 * time.Hour
)

// checkPreparedFreshness decides whether a build may use pb at now and
// whether a background refresh should start. Bases with no grace end (non-
// evaluation editions) only age into a refresh.
func checkPreparedFreshness(
	pb PreparedBase, now time.Time, refreshAfter, minGrace time.Duration,
) (refresh bool, err error) {
	if refreshAfter <= 0 {
		refreshAfter = defaultRefreshAfter
	}
	if minGrace <= 0 {
		minGrace = defaultMinGrace
	}
	if pb.GraceEnd != "" {
		end, perr := time.Parse(time.RFC3339, pb.GraceEnd)
		if perr == nil && end.Sub(now) < minGrace {
			return true, driver.Fixable("warden image prepare --driver hyperv",
				"the prepared Hyper-V base's evaluation grace ends %s (%s left, "+
					"less than the %s minimum); prepare a fresh base first",
				end.Format(time.RFC3339), roundAge(end.Sub(now)),
				roundAge(minGrace))
		}
	}
	at, perr := time.Parse(time.RFC3339, pb.PreparedAt)
	if perr != nil {
		return true, nil
	}
	return now.Sub(at) >= refreshAfter, nil
}

// ensureFreshBase runs the freshness check for the active prepared base and,
// when it is due, starts a background refresh; the build continues on the
// current base either way.
func ensureFreshBase(req *driver.BuildRequest, progress io.Writer) error {
	pb, _, ok := activePrepared()
	if !ok {
		return nil
	}
	refresh, err := checkPreparedFreshness(
		pb, time.Now(), req.ImageRefreshAfter, req.ImageMinGrace)
	if err != nil || !refresh {
		return err
	}
	if log, started := startBackgroundRefresh(); started {
		fmt.Fprintf(progress, "[warden] prepared base is %s old; refreshing it in "+
			"the background (log: %s)\n", roundAge(age(pb)), log)
	}
	return nil
}

func age(pb PreparedBase) time.Duration {
	at, err := time.Parse(time.RFC3339, pb.PreparedAt)
	if err != nil {
		return 0
	}
	return time.Since(at)
}

func roundAge(d time.Duration) string {
	if d >= 48*time.Hour {
		return fmt.Sprintf("%.1fd", d.Hours()/24)
	}
	return d.Round(time.Minute).String()
}
