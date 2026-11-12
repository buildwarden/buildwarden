package hyperv

import (
	"fmt"
	"strings"
)

// psDone is the line a script prints last. `powershell -Command` exits with
// the success of the script's last statement, so a script ending on an
// -ErrorAction SilentlyContinue lookup that finds nothing exits 1 with an
// empty stderr; a final string literal makes a script that ran to its end
// exit 0, and its absence says the script stopped early.
const psDone = "DONE"

// psTailLines bounds how much of a script's stdout an error repeats.
const psTailLines = 10

// psTail is the last psTailLines non-empty lines of out, joined by "; ", or
// "" when out has none.
func psTail(out string) string {
	var lines []string
	for _, l := range strings.Split(out, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			lines = append(lines, l)
		}
	}
	if len(lines) > psTailLines {
		lines = lines[len(lines)-psTailLines:]
	}
	return strings.Join(lines, "; ")
}

// psDoneCheck refuses the output of a script that did not print psDone as
// its last line, naming what it did print.
func psDoneCheck(out string) error {
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if strings.TrimSpace(lines[len(lines)-1]) == psDone {
		return nil
	}
	if tail := psTail(out); tail != "" {
		return fmt.Errorf("the script stopped before its end; it printed: %s", tail)
	}
	return fmt.Errorf("the script stopped before its end, printing nothing")
}
