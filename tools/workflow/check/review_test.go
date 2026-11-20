package check

import (
	"slices"
	"strings"
	"testing"

	"github.com/buildwarden/buildwarden/tools/workflow/queue"
)

// TestApprovalsAreEntryScoped: a design's approvals are read one per review
// entry, the first design approval in a top-level bullet led by r<N>; a later
// mention of an older approval, in the entry or elsewhere, is not one
// (FLOW-19; merge-confirm-attended, a1).
func TestApprovalsAreEntryScoped(t *testing.T) {
	for _, tc := range []struct {
		name, review string
		want         []string
	}{
		{"an entry citing an older approval",
			"- **r1, 2026-11-17: approved at `b7619b5`**\n" +
				"- **r2, 2026-11-18: approved at `0c1cad5`**, read again; the\n" +
				"  design" +
				" approved at `b7619b5` declared drivers: [qemu].\n",
			[]string{"b7619b5", "0c1cad5"}},
		{"a bullet that is not an entry",
			"- r1: approved at `aaa1111`.\n" +
				"- **Decisions.** Kept as approved at `bbb2222`.\n",
			[]string{"aaa1111"}},
		{"a nested citation bullet",
			"- r1: approved at `aaa1111`.\n- r2: approved at `bbb2222`.\n" +
				"  - The design approved at `aaa1111` had no seams.\n" +
				"  - r3: approved at `ccc3333` in a draft.\n",
			[]string{"aaa1111", "bbb2222"}},
		{"the legacy head, wrapped",
			"- **r1, 2026-11-01: approved** after the sitting. Approved\n" +
				"  at `ddd4444`.\n",
			[]string{"ddd4444"}},
		{"a rule change's approval",
			"- r1 addendum: rule change TEST-09 approved at `eee5555`.\n", nil},
		{"a heading ends an entry",
			"- r1: changes requested.\n### Notes\n  approved at `fff6666`\n", nil},
		{"an unindented line ends an entry",
			"- r1: changes requested.\n" +
				"The draft was approved at `fff6666` once.\n", nil},
		{"a re-review",
			"- r1: approved at `aaa1111`.\n- Re-review r2: approved at `bbb2222`.\n",
			[]string{"aaa1111", "bbb2222"}},
	} {
		d, err := ParseDesign(design+"\n## Design review\n\n"+tc.review, tmpl)
		if err != nil {
			t.Fatal(err)
		}
		if got := approvals(d); !slices.Equal(got, tc.want) {
			t.Errorf("%s: approvals = %v, want %v", tc.name, got, tc.want)
		}
		last, ok := lastApproval(d)
		if want := len(tc.want) > 0; ok != want ||
			(ok && last != tc.want[len(tc.want)-1]) {
			t.Errorf("%s: lastApproval = %q, %v", tc.name, last, ok)
		}
	}
}

// TestReReviewCitingAnOlderApprovalIsNotApproved: a second review whose entry
// records no approval fails, though the section cites the first approval
// outside its entry (FLOW-19).
func TestReReviewCitingAnOlderApprovalIsNotApproved(t *testing.T) {
	req := strings.Replace(design, "design_review: skip", "design_review: required", 1)
	work := repo(t, req)
	env := here(work)
	f := demo(2)
	f.DesignReview = "required"
	f.Steps = []queue.Step{{ID: "f2-design-review", Phase: 2, Status: "done"},
		{ID: "f2-design-review-r2", Phase: 2}}
	approved := commit(t, work, req+"\n<!-- as approved -->\n")
	commit(t, work, req+"\n## Design review\n\n- r1: approved at `"+approved+"`.\n"+
		"- **Decisions.** The design approved at `"+approved+"` stands.\n"+
		"- r2: changes requested.\n")
	if got := failures(Review(env, f)); !strings.Contains(got, "1 approvals") {
		t.Fatalf("r2 citing r1's approval: %q", got)
	}
}

// TestLatestApprovalIsTheLastEntrys: the Plan gate and the declaration take
// the newest entry's approval as the approved design, when that entry also
// cites the older one (FLOW-19).
func TestLatestApprovalIsTheLastEntrys(t *testing.T) {
	work := repo(t, ruled)
	env := here(work)
	f := demo(2)
	f.DesignReview = "required"
	f.Steps = []queue.Step{{ID: "f2-design-review", Phase: 2, Status: "done"},
		{ID: "f2-design-review-r2", Phase: 2, Status: "done"},
		{ID: "f2-plan", Phase: 2, Needs: []string{"f2-design-review-r2"}}}
	first := commit(t, work, ruled+"\n<!-- as approved -->\n")
	review := "\n## Design review\n\n- r1: approved at `" + first + "`.\n"
	seams := strings.Replace(ruled, "seams: []", "seams: [relay-claim]", 1)
	second := commit(t, work, seams+review)
	commit(t, work, seams+review+"- r2: approved at `"+second+"`; the design approved at `"+
		first+"` had no seams.\n")
	f.RulesChanged, f.Seams = []string{"TEST-09"}, []string{"relay-claim"}
	if rs := Phase(env, f); !OK(rs) {
		t.Errorf("the Plan gate against r2: %s", failures(rs))
	}
	d, err := Declarations(env, f)
	if err != nil {
		t.Fatal(err)
	}
	if d.Within.Commit != second {
		t.Errorf("the declaration's approved design: %q, want r2's %q", d.Within.Commit,
			second)
	}
}
