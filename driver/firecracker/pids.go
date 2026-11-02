package firecracker

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/buildwarden/buildwarden/driver"
)

// A slot's pid file records the VMMs a build started on it, so a VMM that
// outlives its warden (SIGKILL, OOM) is found and killed before the slot is
// reused, and doctor and clean can report and reap it (Decision 8).

// vmmRecord is one VMM in a slot's pid file. start is its start time in clock
// ticks since boot (/proc/<pid>/stat field 22), which with exe tells the
// recorded process from a later one that reused its pid. keep marks a VMM
// left running by --keep-vms, which holds the slot until it exits.
type vmmRecord struct {
	pid   int
	start uint64
	exe   string
	keep  bool
}

// pidFilePath is slot s's pid file, beside the host locks.
func pidFilePath(s Slot) string {
	return filepath.Join(driver.LockDir(), s.LockName()+".pids")
}

// formatPidFile writes one "<pid> <start> keep|- <exe>" line per record. The
// exe is last and runs to the end of the line, so a path with spaces round
// trips; processRecord refuses one with a newline.
func formatPidFile(recs []vmmRecord) string {
	var b strings.Builder
	for _, r := range recs {
		flag := "-"
		if r.keep {
			flag = "keep"
		}
		fmt.Fprintf(&b, "%d %d %s %s\n", r.pid, r.start, flag, r.exe)
	}
	return b.String()
}

// parsePidFile reads a pid file, skipping lines it cannot parse (a torn write
// leaves nothing to kill).
func parsePidFile(s string) []vmmRecord {
	var recs []vmmRecord
	for _, line := range strings.Split(s, "\n") {
		f := strings.SplitN(line, " ", 4)
		if len(f) < 4 || f[3] == "" || (f[2] != "keep" && f[2] != "-") {
			continue
		}
		pid, err1 := strconv.Atoi(f[0])
		start, err2 := strconv.ParseUint(f[1], 10, 64)
		if err1 != nil || err2 != nil || pid <= 1 {
			continue
		}
		recs = append(recs, vmmRecord{pid: pid, start: start, exe: f[3],
			keep: f[2] == "keep"})
	}
	return recs
}

// procStartTime returns field 22 (starttime) of a /proc/<pid>/stat line. The
// command name (field 2) may hold spaces and parentheses, so fields are
// counted from the last ')'.
func procStartTime(stat string) (uint64, error) {
	i := strings.LastIndexByte(stat, ')')
	if i < 0 {
		return 0, fmt.Errorf("malformed stat line")
	}
	f := strings.Fields(stat[i+1:])
	// f[0] is field 3 (state), so field 22 is f[19].
	if len(f) < 20 {
		return 0, fmt.Errorf("stat line has %d fields after the name", len(f))
	}
	return strconv.ParseUint(f[19], 10, 64)
}

// liveVMMs returns the recorded VMMs still running: the pid exists, started
// at the recorded time, and runs the recorded executable.
func liveVMMs(recs []vmmRecord, readFile func(string) ([]byte, error),
	readlink func(string) (string, error)) []vmmRecord {
	var live []vmmRecord
	for _, r := range recs {
		proc := "/proc/" + strconv.Itoa(r.pid)
		stat, err := readFile(proc + "/stat")
		if err != nil {
			continue
		}
		start, err := procStartTime(string(stat))
		if err != nil || start != r.start {
			continue
		}
		exe, err := readlink(proc + "/exe")
		if err != nil || strings.TrimSuffix(exe, " (deleted)") != r.exe {
			continue
		}
		live = append(live, r)
	}
	return live
}
