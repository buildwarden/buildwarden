package buildlink

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The default link is written once, in this package; the old link is gone.
// This test walks the module tree (no git needed) and fails on either
// written anywhere else, so a copy cannot creep back in.
var (
	// newLink matches the default's three link addresses, whole.
	newLink = regexp.MustCompile(`(^|[^0-9.])100\.127\.255\.25[234]([^0-9]|$)`)
	// oldLink matches 10.0.0.0/30, 10.0.0.1, 10.0.0.2 and the Go form
	// net.IPv4(10, 0, 0, <n>), whole.
	oldLink = regexp.MustCompile(
		`(^|[^0-9.])(10\.0\.0\.0/30|10\.0\.0\.[12])([^0-9]|$)|IPv4\(10, 0, 0, [0-9]+\)`)
)

// strayAllowed reports whether rel (slash-separated, from the module root) may
// hold a match of the given kind ("new" or "old").
func strayAllowed(rel, kind string) bool {
	switch {
	case strings.HasPrefix(rel, "driver/buildlink/"),
		strings.HasPrefix(rel, "docs/design/"),
		strings.HasPrefix(rel, "driver/linuxguest/testdata/seed/"):
		return true
	case kind == "new":
		// Reference docs name the default, and tests pin what renders from it.
		return strings.HasPrefix(rel, "docs/book/") ||
			regexp.MustCompile(`^tools/[^/]+/README\.md$`).MatchString(rel) ||
			strings.HasSuffix(rel, "_test.go")
	}
	// The old link, only in the named refusal and parsing cases: an AWS VPC
	// resolver at 10.0.0.2, a relay VM init's old address being flushed, and
	// a host route's gateway being parsed.
	return map[string]bool{
		"driver/firecracker/posture_test.go": true,
		"cmd/relay/vmlink_test.go":           true,
		"driver/firecracker/slots_test.go":   true,
	}[rel]
}

// straySkip are directories that hold no source: VCS and agent state, build
// outputs and caches.
var straySkip = map[string]bool{
	".git": true, ".dev": true, ".entire": true, "dist": true, "output": true,
	".cache": true, ".rootfs": true, "warden-output": true, ".tmp": true,
	".scratch": true, "worktrees": true, "site": true, "node_modules": true,
}

func TestNoStrayLinkLiterals(t *testing.T) {
	root := filepath.Join("..", "..")
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != root && straySkip[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if bytes.IndexByte(b[:min(len(b), 8000)], 0) >= 0 {
			return nil // binary
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		for i, line := range strings.Split(string(b), "\n") {
			for kind, re := range map[string]*regexp.Regexp{"new": newLink, "old": oldLink} {
				if re.MatchString(line) && !strayAllowed(rel, kind) {
					t.Errorf("%s:%d: %s build link literal outside driver/buildlink: %s",
						rel, i+1, kind, strings.TrimSpace(line))
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestStrayPatterns(t *testing.T) {
	for s, want := range map[string]bool{
		"10.0.0.1":                true,
		"via 10.0.0.2 dev":        true,
		"10.0.0.0/30":             true,
		"net.IPv4(10, 0, 0, 1)":   true,
		"10.0.0.20":               false,
		"110.0.0.1":               false,
		"10.0.0.0/16":             false,
		"10.0.0.3":                false,
		"addr 10.0.0.10":          false,
		"1.10.0.0.1":              false,
		"100.127.255.254":         true,
		"100.127.255.2540":        false,
		"x100.127.255.253":        true, // a letter is a word boundary here
		"1100.127.255.253":        false,
		"100.127.255.252/30 link": true,
	} {
		got := oldLink.MatchString(s) || newLink.MatchString(s)
		if got != want {
			t.Errorf("%q matched = %v, want %v", s, got, want)
		}
	}
}
