package container

import (
	"errors"
	"os/exec"
	"testing"

	"github.com/lesiw/ctrctl"
)

// cliError produces a real *ctrctl.CliError for a process exiting with code.
func cliError(t *testing.T, code string) error {
	t.Helper()
	err := exec.Command("sh", "-c", "exit "+code).Run()
	var ee *exec.ExitError
	if !errors.As(err, &ee) {
		t.Fatalf("sh exit %s: %v", code, err)
	}
	return &ctrctl.CliError{ProcessState: ee.ProcessState}
}

func TestExecExitCode(t *testing.T) {
	nerdctl := "build output\ntime=\"...\" level=fatal msg=\"exec failed with exit code 7\"\n"
	for _, tc := range []struct {
		name   string
		err    error
		tail   string
		code   int
		wantOK bool
	}{
		{"docker/podman pass the code through", cliError(t, "7"), "", 7, true},
		{"nerdctl reports it on stderr", cliError(t, "1"), nerdctl, 7, true},
		{"last nerdctl line wins", cliError(t, "1"),
			"exec failed with exit code 3\n" + nerdctl, 7, true},
		{"not a runtime exit", errors.New("exec: not found"), nerdctl, 0, false},
	} {
		code, ok := execExitCode(tc.err, tc.tail)
		if code != tc.code || ok != tc.wantOK {
			t.Errorf("%s: execExitCode = %d, %v; want %d, %v",
				tc.name, code, ok, tc.code, tc.wantOK)
		}
	}
}
