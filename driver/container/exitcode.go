package container

import (
	"errors"
	"regexp"
	"strconv"

	"github.com/lesiw/ctrctl"
)

// nerdctlExecExit is the line nerdctl (finch) prints when an exec'd command
// fails; nerdctl itself then exits 1 whatever the command's code was.
var nerdctlExecExit = regexp.MustCompile(`exec failed with exit code (\d+)`)

// execExitCode recovers the exit code of the command a `<runtime> exec` ran.
// docker and podman exit with that code; nerdctl always exits 1 and reports
// the real one on stderr, whose tail is stderrTail. ok is false when err is
// not a non-zero exit of the runtime CLI (it failed to start, say).
func execExitCode(err error, stderrTail string) (code int, ok bool) {
	var ce *ctrctl.CliError
	if !errors.As(err, &ce) || ce.ProcessState == nil {
		return 0, false
	}
	if m := nerdctlExecExit.FindAllStringSubmatch(stderrTail, -1); len(m) > 0 {
		if n, err := strconv.Atoi(m[len(m)-1][1]); err == nil {
			return n, true
		}
	}
	return ce.ExitCode(), true
}
