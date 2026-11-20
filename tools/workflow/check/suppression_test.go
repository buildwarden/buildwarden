package check

import "testing"

// TestTrailers: each Tier1-Allow trailer of a commit is an allow, or, with
// no reason or not in the form, a suppression finding (LINT-05).
func TestTrailers(t *testing.T) {
	allows, bad := Trailers("0123456789abcdef",
		"subject\n\nTier1-Allow: GLOBAL-01 a/*.go: why\n"+
			"  Tier1-Allow: GLOBAL-02 b/:   \nTier1-Allow: nonsense\nnot a trailer\n")
	if len(allows) != 1 || allows[0] != (Allow{Rule: "GLOBAL-01", Glob: "a/*.go",
		Reason: "why", Commit: "0123456789abcdef"}) {
		t.Errorf("allows %+v", allows)
	}
	if len(bad) != 2 || bad[0].Rule != "LINT-05" || bad[0].Check != "suppression" ||
		bad[0].Severity != Must || bad[1].Message != `commit 0123456789ab: Tier1-Allow `+
		`"nonsense" needs the form "<RULE-ID> <path-glob>: <reason>" with a reason` {
		t.Errorf("bad %+v", bad)
	}
}
