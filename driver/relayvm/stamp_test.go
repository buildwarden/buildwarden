package relayvm

import (
	"path/filepath"
	"testing"
)

// TestStampPath checks the stamp comes from the same place as its asset:
// link-channel in the source tree's output directory, or the release file
// name plus .link-channel beside the warden binary.
func TestStampPath(t *testing.T) {
	root := filepath.FromSlash("/src/buildwarden")
	for _, a := range []Asset{QEMUBuild[1], Firecracker[2]} {
		src := filepath.Join(root, filepath.FromSlash(a.Source))
		if got, want := StampPath(a, src),
			filepath.Join(filepath.Dir(src), "link-channel"); got != want {
			t.Errorf("source tree: %s, want %s", got, want)
		}
		rel := filepath.Join(filepath.FromSlash("/opt/warden/bin"), a.Release)
		if got, want := StampPath(a, rel), rel+".link-channel"; got != want {
			t.Errorf("release: %s, want %s", got, want)
		}
	}
}
