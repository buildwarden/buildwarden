package container

import (
	"slices"
	"testing"
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
