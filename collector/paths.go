package collector

import "strings"

// Path rules shared by the relay (which enforces them on guest requests) and
// the collector (which enforces them again before touching the host disk), so
// a name is accepted or refused identically on both sides of the wire.

// MaxPathLen bounds artifact and context paths.
const MaxPathLen = 255

// IsSafeArtifactPath reports whether p is an acceptable artifact name: a
// relative, slash-separated path of segments that start and end with an
// alphanumeric and contain only [A-Za-z0-9._-].
func IsSafeArtifactPath(p string) bool {
	if p == "" || len(p) > MaxPathLen || p[0] == '/' || p[len(p)-1] == '/' {
		return false
	}
	for _, seg := range strings.Split(p, "/") {
		if seg == "" || seg == "." || seg == ".." {
			return false
		}
		if seg[0] == '.' || seg[0] == '-' || seg[0] == '_' || !safeTail(seg) {
			return false
		}
	}
	return true
}

// IsSafeContextPath reports whether p may be served from the build context.
// Like IsSafeArtifactPath but segments may start with '.', so staged
// directories such as .warden/ remain reachable.
func IsSafeContextPath(p string) bool {
	if p == "" || len(p) > MaxPathLen || p[0] == '/' || p[len(p)-1] == '/' {
		return false
	}
	for _, seg := range strings.Split(p, "/") {
		if seg == "" || seg == "." || seg == ".." || !safeTail(seg) {
			return false
		}
	}
	return true
}

func safeTail(seg string) bool {
	last := seg[len(seg)-1]
	if last == '.' || last == '-' || last == '_' {
		return false
	}
	for _, c := range seg {
		if !isSafeChar(c) {
			return false
		}
	}
	return true
}

func isSafeChar(c rune) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
		(c >= '0' && c <= '9') || c == '-' || c == '_' || c == '.'
}

// sanitizeCaptureName maps a capture name (derived from an arbitrary request
// URL) to a single portable file name: every character outside
// [A-Za-z0-9._-] becomes '_'. Returns "" for names that cannot be made safe.
func sanitizeCaptureName(name string) string {
	if name == "" || len(name) > MaxPathLen {
		return ""
	}
	b := []byte(name)
	for i, c := range b {
		if !isSafeChar(rune(c)) {
			b[i] = '_'
		}
	}
	out := string(b)
	if strings.Trim(out, ".") == "" {
		return ""
	}
	return out
}
