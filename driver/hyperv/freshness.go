package hyperv

import (
	"fmt"
	"io"
	"path/filepath"
	"time"

	"github.com/buildwarden/buildwarden/driver"
	"github.com/buildwarden/buildwarden/driver/winguest"
)

const prepareFix = "warden image prepare --driver hyperv"

func (pb PreparedBase) freshness() winguest.Freshness {
	return winguest.Freshness{PreparedAt: pb.PreparedAt, GraceEnd: pb.GraceEnd}
}

// ensureFreshBase runs the freshness rules (winguest.CheckFreshness) for the
// active prepared base and, when a refresh is due, starts one in the
// background; the build continues on the current base either way.
func ensureFreshBase(req *driver.BuildRequest, progress io.Writer) error {
	pb, _, ok := activePrepared()
	if !ok {
		return nil
	}
	now := time.Now()
	refresh, err := winguest.CheckFreshness(
		pb.freshness(), now, req.ImageRefreshAfter, req.ImageMinGrace, prepareFix)
	if err != nil || !refresh {
		return err
	}
	if !prepareRunning() {
		logPath := filepath.Join(hypervCacheDir(), "prepare.log")
		if err := driver.StartDetached(logPath,
			"image", "prepare", "--driver", "hyperv"); err == nil {
			fmt.Fprintf(progress, "[warden] prepared base is %s old; refreshing it "+
				"in the background (log: %s)\n",
				winguest.RoundAge(pb.freshness().Age(now)), logPath)
		}
	}
	return nil
}

// prepareRunning reports whether a prepare holds the prepare lock.
func prepareRunning() bool {
	unlock, free, err := driver.TryHostLock(prepareLockName)
	if err != nil {
		return false
	}
	if free {
		unlock()
	}
	return !free
}
