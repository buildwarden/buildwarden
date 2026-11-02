package firecracker

import (
	"os"
	"testing"
)

// statLine is a /proc/<pid>/stat line whose command name holds spaces and a
// parenthesis, with starttime (field 22) 987654.
const statLine = "4242 (fire cracker)) S 1 4242 4242 0 -1 4194560 100 0 0 0 1 2 0 0 20 0 3 0 " +
	"987654 1000 10\n"

func TestProcStartTime(t *testing.T) {
	got, err := procStartTime(statLine)
	if err != nil || got != 987654 {
		t.Fatalf("procStartTime = %d, %v", got, err)
	}
	if _, err := procStartTime("4242 (x) S 1"); err == nil {
		t.Fatal("short stat line accepted")
	}
}

func TestPidFileRoundTrip(t *testing.T) {
	recs := []vmmRecord{
		{pid: 4242, start: 987654, exe: "/usr/local/bin/firecracker"},
		{pid: 4243, start: 987700, exe: "/usr/local/bin/firecracker", keep: true},
	}
	got := parsePidFile(formatPidFile(recs) + "garbage\n1 2\n")
	if len(got) != 2 || got[0] != recs[0] || got[1] != recs[1] {
		t.Fatalf("round trip = %+v", got)
	}
}

// TestLiveVMMs is stale-VMM detection: only a pid that still runs the
// recorded executable from the recorded start time counts.
func TestLiveVMMs(t *testing.T) {
	files := map[string]string{
		"/proc/4242/stat": statLine,
		"/proc/5000/stat": statLine, // pid reused: same stat, other exe
		"/proc/6000/stat": statLine, // started at another time than recorded
	}
	links := map[string]string{
		"/proc/4242/exe": "/usr/local/bin/firecracker (deleted)",
		"/proc/5000/exe": "/usr/bin/bash",
		"/proc/6000/exe": "/usr/local/bin/firecracker",
	}
	read := func(p string) ([]byte, error) {
		if s, ok := files[p]; ok {
			return []byte(s), nil
		}
		return nil, os.ErrNotExist
	}
	link := func(p string) (string, error) {
		if s, ok := links[p]; ok {
			return s, nil
		}
		return "", os.ErrNotExist
	}
	fc := "/usr/local/bin/firecracker"
	live := liveVMMs([]vmmRecord{
		{pid: 4242, start: 987654, exe: fc},
		{pid: 5000, start: 987654, exe: fc},
		{pid: 6000, start: 1, exe: fc},
		{pid: 7000, start: 987654, exe: fc}, // gone
	}, read, link)
	if len(live) != 1 || live[0].pid != 4242 {
		t.Fatalf("live = %+v, want only 4242", live)
	}
}
