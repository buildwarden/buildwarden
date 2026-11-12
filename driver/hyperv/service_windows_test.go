//go:build windows

package hyperv

import (
	"errors"
	"fmt"
	"testing"

	"golang.org/x/sys/windows"
)

// TestSCMServiceState maps each answer the SCM can give for the privileged
// service: only "does not exist" is not installed, access denied is installed
// with its state unread, any other error is unread, and an opened service
// reads its state.
func TestSCMServiceState(t *testing.T) {
	other := windows.RPC_S_SERVER_UNAVAILABLE
	for _, c := range []struct {
		name          string
		openErr       error
		queryErr      error
		current       uint32
		wantInstalled bool
		wantState     string
		wantErr       error
	}{
		{"does not exist", windows.ERROR_SERVICE_DOES_NOT_EXIST, nil, 0,
			false, "", nil},
		{"access denied", windows.ERROR_ACCESS_DENIED, nil, 0,
			true, "", windows.ERROR_ACCESS_DENIED},
		{"open failed otherwise", other, nil, 0, false, "", other},
		{"query failed", nil, windows.ERROR_ACCESS_DENIED, 0,
			true, "", windows.ERROR_ACCESS_DENIED},
		{"stopped", nil, nil, windows.SERVICE_STOPPED, true, serviceStopped, nil},
		{"running", nil, nil, windows.SERVICE_RUNNING, true, serviceRunning, nil},
	} {
		t.Run(c.name, func(t *testing.T) {
			installed, state, err := scmServiceState(c.openErr, c.queryErr, c.current)
			if installed != c.wantInstalled || state != c.wantState ||
				!errors.Is(err, c.wantErr) || (err == nil) != (c.wantErr == nil) {
				t.Errorf("got (%v, %q, %v), want (%v, %q, %v)", installed, state, err,
					c.wantInstalled, c.wantState, c.wantErr)
			}
		})
	}
}

// TestServiceStateName names every SCM state code doctor can print.
func TestServiceStateName(t *testing.T) {
	for code, want := range map[uint32]string{
		windows.SERVICE_STOPPED:          serviceStopped,
		windows.SERVICE_START_PENDING:    "starting",
		windows.SERVICE_STOP_PENDING:     "stopping",
		windows.SERVICE_RUNNING:          serviceRunning,
		windows.SERVICE_CONTINUE_PENDING: "resuming",
		windows.SERVICE_PAUSE_PENDING:    "paused",
		windows.SERVICE_PAUSED:           "paused",
		42:                               fmt.Sprintf("in state %d", 42),
	} {
		if got := serviceStateName(code); got != want {
			t.Errorf("serviceStateName(%d) = %q, want %q", code, got, want)
		}
	}
}
