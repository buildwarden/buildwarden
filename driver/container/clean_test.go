package container

import (
	"slices"
	"strings"
	"testing"

	"github.com/lesiw/ctrctl"
)

// A build's containers are removed build first and holder last: podman
// refuses to remove a holder whose namespace the build container still
// joins, and the network stays while the holder does.
func TestRemovalOrder(t *testing.T) {
	listed := []string{"warden-relay-a", "warden-holder-a", "warden-build-a",
		"warden-holder-b", "warden-relay-b", "warden-build-b"}
	want := []string{"warden-build-a", "warden-build-b", "warden-relay-a",
		"warden-relay-b", "warden-holder-a", "warden-holder-b"}
	if got := removalOrder(listed); !slices.Equal(got, want) {
		t.Errorf("removalOrder(%q) = %q, want %q", listed, got, want)
	}
	if listed[0] != "warden-relay-a" {
		t.Error("removalOrder changed its argument")
	}
}

// The driver's waiter under a live driver keeps its build; nothing else
// does (Decision 25): the same waiter orphaned (parent PID 1, or a parent
// gone from the listing) or under a parent that is not the driver, a waiter
// on the container's ID, podman's conmon, and any line that names the
// relay container.
func TestLiveBuilds(t *testing.T) {
	const id = "k3v9x2ab"
	for _, tc := range []struct {
		name    string
		listing string
		live    bool
	}{
		{"docker's waiter under the driver by path", `
    1     0 /sbin/init
  200     1 /usr/local/bin/warden-driver-container run
  201   200 docker container wait warden-build-k3v9x2ab
`, true},
		{"the driver by bare name", `
  200    90 warden-driver-container run
  201   200 docker container wait warden-build-k3v9x2ab
`, true},
		{"finch's waiter by path, its helpers beneath it", `
    1     0 /sbin/launchd
  200     1 /usr/local/bin/warden-driver-container run
  201   200 /usr/local/bin/finch container wait warden-build-k3v9x2ab
  202   201 limactl shell finch sudo nerdctl container wait warden-build-k3v9x2ab
  203   202 ssh -F /dev/null 127.0.0.1 -- sudo nerdctl container wait warden-build-k3v9x2ab
`, true},
		{"podman's waiter, and its conmon", `
  200     1 /usr/bin/warden-driver-container run
  201   200 podman container wait warden-build-k3v9x2ab
  300     1 /usr/bin/conmon --api-version 1 -c 0123abcd -n warden-build-k3v9x2ab -r /usr/bin/crun
`, true},
		{"orphaned: parent PID 1", `
    1     0 /sbin/init
  201     1 docker container wait warden-build-k3v9x2ab
`, false},
		{"orphaned: the parent gone from the listing", `
  201   200 docker container wait warden-build-k3v9x2ab
`, false},
		{"under a shell", `
  200     1 -bash
  201   200 docker container wait warden-build-k3v9x2ab
`, false},
		{"under finch itself", `
  200     1 /usr/local/bin/finch container wait warden-build-k3v9x2ab
  201   200 /usr/local/bin/finch container wait warden-build-k3v9x2ab
`, false},
		{"a waiter on the 64-hex ID", `
  200     1 /usr/local/bin/warden-driver-container run
  201   200 docker container wait ` + strings.Repeat("0123abcd", 8) + `
`, false},
		{"conmon alone", `
  300     1 /usr/bin/conmon --api-version 1 -c 0123abcd -n warden-build-k3v9x2ab -r /usr/bin/crun
`, false},
		{"the relay named under the driver", `
  200     1 /usr/local/bin/warden-driver-container run
  201   200 docker logs -f warden-relay-k3v9x2ab
  202   200 docker run --name warden-relay-k3v9x2ab ghcr.io/buildwarden/relay
`, false},
		{"a waiter with more arguments", `
  200     1 /usr/local/bin/warden-driver-container run
  201   200 docker container wait warden-build-k3v9x2ab warden-build-other
`, false},
		{"another program's wait", `
  200     1 /usr/local/bin/warden-driver-container run
  201   200 nerdctl container wait warden-build-k3v9x2ab
`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := liveBuilds(tc.listing)
			if got[id] != tc.live || len(got) > 1 {
				t.Errorf("liveBuilds = %v, want %s live: %v", got, id, tc.live)
			}
		})
	}
}

// run's waiter names the build container, which is what clean's scan looks
// for, not the ID the runtime's create returned.
func TestWaiterNamesTheContainer(t *testing.T) {
	old := ctrctl.Cli
	ctrctl.Cli = []string{"finch"}
	t.Cleanup(func() { ctrctl.Cli = old })
	b := &build{buildID: "k3v9x2ab", buildContainer: strings.Repeat("0123abcd", 8)}
	want := []string{"finch", "container", "wait", "warden-build-k3v9x2ab"}
	if got := b.waitArgs(); !slices.Equal(got, want) {
		t.Errorf("waitArgs = %q, want %q", got, want)
	}
	if id, ok := waiterBuild(b.waitArgs()); !ok || id != "k3v9x2ab" {
		t.Errorf("the scan does not take the waiter for build k3v9x2ab's: %q, %v",
			id, ok)
	}
}

// With no ps, as on Windows, the scan finds nothing.
func TestLiveProcessBuildsWithoutPs(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	if got := liveProcessBuilds(); len(got) != 0 {
		t.Errorf("liveProcessBuilds = %v with no ps", got)
	}
}
