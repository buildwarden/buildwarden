package check

import (
	"fmt"
	"regexp"
	"strings"
)

// Allow is one Tier1-Allow trailer: findings against Rule on paths matching
// Glob are suppressed, and Reason is printed with them (LINT-05).
type Allow struct {
	Rule, Glob, Reason, Commit string
}

var (
	trailerRe = regexp.MustCompile(`^Tier1-Allow:\s*(.*)$`)
	allowRe   = regexp.MustCompile(`^([A-Z]+-\d{2})\s+(\S+):(.*)$`)
)

// Trailers parses the Tier1-Allow trailers of one commit's message: the
// allows, and a suppression finding for each trailer that has no reason or
// is not in the "<RULE-ID> <path-glob>: <reason>" form (LINT-05).
func Trailers(sha, body string) ([]Allow, []Finding) {
	var allows []Allow
	var bad []Finding
	for _, line := range strings.Split(body, "\n") {
		m := trailerRe.FindStringSubmatch(strings.TrimSpace(line))
		if m == nil {
			continue
		}
		a, err := ParseAllow(sha, m[1])
		if err != nil {
			bad = append(bad, *err)
			continue
		}
		allows = append(allows, a)
	}
	return allows, bad
}

// ParseAllow parses one Tier1-Allow trailer's value from commit sha.
func ParseAllow(sha, value string) (Allow, *Finding) {
	m := allowRe.FindStringSubmatch(strings.TrimSpace(value))
	if m == nil || strings.TrimSpace(m[3]) == "" {
		return Allow{}, &Finding{Rule: "LINT-05", Severity: Must, Check: "suppression",
			Message: fmt.Sprintf("commit %.12s: Tier1-Allow %q needs the form "+
				"\"<RULE-ID> <path-glob>: <reason>\" with a reason", sha, value)}
	}
	return Allow{Rule: m[1], Glob: m[2], Reason: strings.TrimSpace(m[3]), Commit: sha}, nil
}
