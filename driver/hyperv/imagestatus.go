package hyperv

import (
	"fmt"
	"io"
	"path/filepath"
	"time"

	"github.com/buildwarden/buildwarden/driver"
)

// ImageStatus describes the Hyper-V build images for `image list` and
// `doctor`.
type ImageStatus struct {
	// Base is the active generalized base path ("" when none is pinned).
	Base   string
	Pinned bool
	// Active is the prepared layer builds boot from (zero when none).
	Active PreparedBase
	// Older are superseded prepared layers not yet retired.
	Older []PreparedBase
	// RefreshRunning reports that a prepare holds the prepare lock.
	RefreshRunning bool
}

// Status reads the image manifest and the prepare lock.
func Status() ImageStatus {
	var st ImageStatus
	m, err := loadEvalManifest()
	if err != nil {
		return st
	}
	if b, ok := m.Images[m.Active]; ok {
		st.Base, st.Pinned = b.Path, b.Pinned
	}
	for _, p := range m.Prepared {
		if p.Path == m.ActivePrepared {
			st.Active = p
		} else {
			st.Older = append(st.Older, p)
		}
	}
	if unlock, free, err := driver.TryHostLock(prepareLockName); err == nil {
		if free {
			unlock()
		} else {
			st.RefreshRunning = true
		}
	}
	return st
}

// Summary is a one-line state of the active prepared base at now.
func (s ImageStatus) Summary(now time.Time) string {
	if s.Active.Path == "" {
		if s.Base == "" {
			return "none (warden image prepare --driver hyperv --source <image>)"
		}
		return "not prepared (warden image prepare --driver hyperv)"
	}
	out := fmt.Sprintf("%s, %s old", filepath.Base(s.Active.Path), roundAge(age(s.Active)))
	if end, err := time.Parse(time.RFC3339, s.Active.GraceEnd); err == nil {
		out += fmt.Sprintf(", evaluation grace ends %s (%s left)",
			end.Format("2006-01-02 15:04Z"), roundAge(end.Sub(now)))
	}
	if s.RefreshRunning {
		out += ", refresh running"
	}
	return out
}

// writeImageStatus is doctor's prepared-base row. It reports blocked when the
// active base is too close to its grace end for a build to start.
func writeImageStatus(w io.Writer, s ImageStatus, now time.Time) (blocked bool) {
	fmt.Fprintf(w, "%s%s\n", dotted("Prepared base"), s.Summary(now))
	if s.Active.Path == "" {
		return false
	}
	refresh, err := checkPreparedFreshness(s.Active, now, 0, 0)
	switch {
	case err != nil:
		fmt.Fprintf(w, "  %v\n", err)
		return true
	case refresh && !s.RefreshRunning:
		fmt.Fprintf(w, "  due for refresh: the next build starts one, or run "+
			"`warden image prepare --driver hyperv`\n")
	}
	return false
}
