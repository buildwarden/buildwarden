package container

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/buildwarden/buildwarden/driver"
	"github.com/buildwarden/buildwarden/driver/drivercmd"
)

// dockerOnPath installs a docker CLI, a POSIX shell script on a PATH of its
// own, whose body is cases, a case statement over "$1 $2"; every
// invocation's arguments are appended to the returned log.
func dockerOnPath(t *testing.T, cases string) (log string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the fake runtime is a POSIX shell script")
	}
	dir := t.TempDir()
	log = filepath.Join(t.TempDir(), "calls")
	writeScript(t, filepath.Join(dir, "docker"), "#!/bin/sh\necho \"$*\" >> '"+log+
		"'\ncase \"$1 $2\" in\n"+cases+"\nesac\n")
	t.Setenv("PATH", dir)
	t.Setenv("WARDEN_CTR_CLI", "")
	t.Setenv("WARDEN_IMAGE", "")
	return log
}

// dockerAnswers is the case that answers the version query, as a docker
// whose daemon is up does.
const dockerAnswers = `"version --format") echo 27.3.1 ;;`

// runtimeWithBuilds answers the version query, and the listings with two
// builds' containers, networks and dev relay images: k3v9x2ab's and
// live1234's.
const runtimeWithBuilds = dockerAnswers + `
"container ls") printf 'warden-build-k3v9x2ab\nwarden-relay-k3v9x2ab\n` +
	`warden-holder-k3v9x2ab\nwarden-build-live1234\nother\n' ;;
"network ls") printf 'bridge\nwarden-k3v9x2ab\nwarden-live1234\n' ;;
"image ls") printf 'warden-relay:k3v9x2ab\nwarden-relay:live1234\nalpine:latest\n' ;;`

// calls are the log's lines that remove something.
func removals(t *testing.T, log string) string {
	t.Helper()
	data, _ := os.ReadFile(log)
	var out []string
	for _, l := range strings.Split(string(data), "\n") {
		if strings.Contains(l, " rm ") || strings.HasPrefix(l, "rm ") {
			out = append(out, l)
		}
	}
	return strings.Join(out, "\n")
}

func callContainer(t *testing.T, command string, req *drivercmd.Request) (
	*drivercmd.Result, string, error) {
	t.Helper()
	var out bytes.Buffer
	req.Streams = drivercmd.Streams{Stdin: strings.NewReader(""), Stdout: &out,
		Stderr: &out}
	res, err := drivercmd.Call(context.Background(), Driver(), command, req, nil)
	return res, out.String(), err
}

// reap removes one build's containers, network, dev relay image and work
// directories on the runtime its state file names, and nothing of another
// build; its own settings are not read (DRV-24, DRV-C10).
func TestReapFromTheStateFile(t *testing.T) {
	log := dockerOnPath(t, runtimeWithBuilds)
	tmp := t.TempDir()
	work := filepath.Join(tmp, "warden-container-k3v9x2ab-123")
	other := filepath.Join(tmp, "warden-container-live1234-456")
	for _, d := range []string{work, other} {
		if err := os.Mkdir(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	stateFile := filepath.Join(t.TempDir(), "build-container-k3v9x2ab.state")
	if err := drivercmd.SaveState(stateFile, state{Runtime: "docker"}); err != nil {
		t.Fatal(err)
	}
	// A config file that is gone, and a flag that would be refused: neither
	// is read with a state file.
	res, out, err := callContainer(t, drivercmd.CmdReap, &drivercmd.Request{Contract: 1,
		BuildID: "k3v9x2ab", TempDir: tmp, StateFile: stateFile,
		ConfigFile: filepath.Join(t.TempDir(), "gone.toml"),
		Args:       []string{"--runtime=./rt"}})
	if err != nil {
		t.Fatalf("reap: %v\n%s", err, out)
	}
	if res.Removed != 6 {
		t.Errorf("removed %d, want 6:\n%s", res.Removed, out)
	}
	got := removals(t, log)
	for _, want := range []string{"warden-build-k3v9x2ab", "warden-relay-k3v9x2ab",
		"warden-holder-k3v9x2ab", "network rm warden-k3v9x2ab",
		"image rm warden-relay:k3v9x2ab"} {
		if !strings.Contains(got, want) {
			t.Errorf("reap did not remove %s:\n%s", want, got)
		}
	}
	if strings.Contains(got, "live1234") {
		t.Errorf("reap removed another build's:\n%s", got)
	}
	if _, err := os.Stat(work); !os.IsNotExist(err) {
		t.Errorf("the work directory is left: %v", err)
	}
	if _, err := os.Stat(other); err != nil {
		t.Errorf("another build's work directory: %v", err)
	}
}

// A runtime that does not answer is reap's error, exit 1 with the fix,
// never nothing to remove (DRV-24).
func TestReapRuntimeNotAnswering(t *testing.T) {
	dockerOnPath(t, `*) exit 1 ;;`)
	stateFile := filepath.Join(t.TempDir(), "x.state")
	if err := drivercmd.SaveState(stateFile, state{Runtime: "docker"}); err != nil {
		t.Fatal(err)
	}
	_, _, err := callContainer(t, drivercmd.CmdReap, &drivercmd.Request{Contract: 1,
		BuildID: "k3v9x2ab", StateFile: stateFile})
	var fe *driver.FixableError
	if !errors.As(err, &fe) || !strings.Contains(fe.Msg, "docker") ||
		!strings.Contains(fe.Fix, "start Docker") {
		t.Errorf("reap = %v, want the runtime's fixable error", err)
	}
}

// A state file's runtime is checked again as it is read back: one that is
// not supported is refused, and nothing is run (DRV-61).
func TestStateFileRuntimeChecked(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "ran")
	dir := t.TempDir()
	shipPrograms(t, dir, marker, "rt")
	t.Setenv("PATH", dir)
	stateFile := filepath.Join(t.TempDir(), "x.state")
	if err := drivercmd.SaveState(stateFile, state{Runtime: filepath.Join(dir,
		"rt")}); err != nil {
		t.Fatal(err)
	}
	for _, command := range []string{drivercmd.CmdReap, drivercmd.CmdAttach} {
		_, _, err := callContainer(t, command, &drivercmd.Request{Contract: 1,
			BuildID: "k3v9x2ab", StateFile: stateFile})
		var fe *driver.FixableError
		if !errors.As(err, &fe) || !strings.Contains(fe.Msg, "the state file") {
			t.Errorf("%s = %v, want the state file's runtime refused", command, err)
		}
	}
	wantNotRun(t, marker)
}

// clean removes the leftovers of every build that is neither in keep_ids
// nor waited on by a live driver's waiter, and keeps the others (DRV-27, DRV-58).
func TestCleanKeepsTheLiveBuilds(t *testing.T) {
	log := dockerOnPath(t, runtimeWithBuilds)
	res, out, err := callContainer(t, drivercmd.CmdClean, &drivercmd.Request{Contract: 1,
		KeepIDs: []string{"live1234"}, Args: []string{"--runtime=docker"}})
	if err != nil {
		t.Fatalf("clean: %v\n%s", err, out)
	}
	got := removals(t, log)
	if res.Removed != 5 || strings.Contains(got, "live1234") ||
		!strings.Contains(got, "warden-build-k3v9x2ab") {
		t.Errorf("removed %d:\n%s\n%s", res.Removed, got, out)
	}
	if !strings.Contains(out, "Removing container: warden-build-k3v9x2ab") {
		t.Errorf("no Removing line on stderr:\n%s", out)
	}
}

// doctor names the runtime it resolved, and leaves ok to warden (DRV-25).
func TestDoctorNamesTheRuntime(t *testing.T) {
	dockerOnPath(t, dockerAnswers)
	res, out, err := callContainer(t, drivercmd.CmdDoctor, &drivercmd.Request{Contract: 1,
		Args: []string{}})
	if err != nil || res.Blocked || out != "container runtime: docker\n" {
		t.Errorf("doctor = %+v, %v:\n%q", res, err, out)
	}
}

// The build writes the runtime it resolved to its state file before it
// creates anything: here the first runtime call fails, and the file holds
// the runtime (DRV-61).
func TestRunWritesTheState(t *testing.T) {
	dockerOnPath(t, `*) exit 1 ;;`)
	stateFile := filepath.Join(t.TempDir(), "x.state")
	start := &driver.Start{Contract: 1, BuildID: "k3v9x2ab", Mode: driver.ModeBuild,
		GuestOS: "linux", StateFile: stateFile, TempDir: t.TempDir(),
		Args: []string{"--runtime=docker"}}
	var out bytes.Buffer
	_, err := drivercmd.Call(context.Background(), Driver(), drivercmd.CmdRun,
		&drivercmd.Request{Start: start, Streams: drivercmd.Streams{Stderr: &out}},
		&exitHost{exits: make(chan string, 1)})
	if err == nil {
		t.Fatal("a build on a runtime that fails every call succeeded")
	}
	var st state
	if found, err := drivercmd.LoadState(stateFile, &st); !found || err != nil ||
		st.Runtime != "docker" {
		t.Errorf("state %+v, %v, %v; want the runtime docker", st, found, err)
	}
}

// describe is valid for the contract and lists what the driver implements.
func TestDescribe(t *testing.T) {
	d, err := Driver().Description()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"run", "reap", "doctor", "clean", "attach"}
	if strings.Join(d.Commands, " ") != strings.Join(want, " ") || !d.Capabilities.Shell ||
		d.Capabilities.VM || d.Name != "container" {
		t.Errorf("describe %+v", d)
	}
}
