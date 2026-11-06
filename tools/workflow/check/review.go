package check

import (
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strings"
)

// approvedRe finds an approval in the design's Design review section: each
// approval names the commit the approved design was read at (FLOW-19).
// The phrase may wrap across lines.
var approvedRe = regexp.MustCompile("(?i)approved\\s+at\\s+`?([0-9a-f]{7,40})")

// approvals returns the commits of the design's recorded approvals, oldest
// first. A rule change's own approval (`rule change <ID> approved at <sha>`)
// is not one.
func approvals(d *Design) []string {
	var out []string
	text := ruleApprovedRe.ReplaceAllString(d.Sections["Design review"], "")
	for _, m := range approvedRe.FindAllStringSubmatch(text, -1) {
		out = append(out, m[1])
	}
	return out
}

// reviewDecision is Design's part of FLOW-19: the design records
// design_review, the queue matches it, and a skip meets the criteria.
func (c *checker) reviewDecision(d *Design) {
	dr := d.Front.DesignReview
	c.add("design_review decided (FLOW-19)", dr == "required" || dr == "skip",
		"design_review %q", dr)
	c.add("design_review recorded in queue", dr != "" && dr == c.f.DesignReview,
		"design %q, queue %q", dr, c.f.DesignReview)
	if dr == "skip" {
		c.skipValid(d)
	}
}

// reviewed is FLOW-19's approval check, run when a design-review step is
// marked done (Review) and again at the Plan gate: every design review seeded
// so far has an approval recorded, or the skip is still valid against the
// design as planned.
func (c *checker) reviewed() {
	d, err := c.loadDesign(c.f.Branch)
	if err != nil {
		c.add("design reviewed (FLOW-19)", false, "%v", err)
		return
	}
	if c.f.Round != "process" && c.f.DesignReview == "skip" {
		c.skipValid(d)
		return
	}
	want := 1
	if c.f.Round != "process" {
		want = 0
		for _, st := range c.f.Steps {
			if strings.Contains(st.ID, "-design-review") {
				want++
			}
		}
	}
	got := len(approvals(d))
	c.add("design reviewed (FLOW-19)", want > 0 && got >= want,
		"%d approvals recorded in Design review, %d reviews seeded", got, want)
}

// approvalDelta is the Plan gate's guard (FLOW-19): planning leaves the
// design's impact fields and rule_changes as its last approval left them, so
// Plan cannot widen the design past its review. A rule_changes entry that
// differs from the approved design passes when its own rule change approval
// names a commit whose front-matter has the entry as it is now: the human
// approved it on its own line. A skipped review has no approval to compare;
// skipValid rechecks it, and reviewed reports a missing approval.
func (c *checker) approvalDelta() {
	const name = "design unchanged since its approval (FLOW-19)"
	if c.f.DesignReview == "skip" {
		return
	}
	d, err := c.loadDesign(c.f.Branch)
	if err != nil {
		c.add(name, false, "%v", err)
		return
	}
	a := approvals(d)
	if len(a) == 0 {
		return
	}
	base := a[len(a)-1]
	prev, err := c.loadDesign(base)
	if err != nil {
		c.add(name, false, "approved design at %s: %v", base, err)
		return
	}
	var diffs []string
	if diff := impactDelta(&prev.Front, &d.Front); diff != "" {
		diffs = append(diffs, diff)
	}
	diffs = append(diffs, c.ruleChangesDelta(prev, d)...)
	if len(diffs) > 0 {
		c.add(name, false, "changed since the approval at %s, a design change for Design "+
			"(FLOW-06): %s", base, strings.Join(diffs, "; "))
		return
	}
	c.add(name, true, "impact fields and rule_changes as approved at %s", base)
}

// ruleChangesDelta lists how d's rule_changes differ from the approved
// design's, leaving out each entry its own approval line covers.
func (c *checker) ruleChangesDelta(approved, d *Design) []string {
	then := map[string]RuleChange{}
	for _, rc := range approved.Front.RuleChanges {
		then[rc.Rule] = rc
	}
	var out []string
	now := map[string]bool{}
	for _, rc := range d.Front.RuleChanges {
		now[rc.Rule] = true
		if was, ok := then[rc.Rule]; ok && sameRuleChange(was, rc) {
			continue
		}
		if !c.ownApproval(d, rc) {
			out = append(out, fmt.Sprintf("rule_changes %s added or changed, and no "+
				"rule change %s approved at a commit that has it", rc.Rule, rc.Rule))
		}
	}
	for _, rc := range approved.Front.RuleChanges {
		if !now[rc.Rule] {
			out = append(out, fmt.Sprintf("rule_changes %s dropped", rc.Rule))
		}
	}
	return out
}

// ownApproval reports whether a `rule change <ID> approved at <sha>` line
// names a commit whose design has rc as it is now.
func (c *checker) ownApproval(d *Design, rc RuleChange) bool {
	for _, m := range ruleApprovedRe.FindAllStringSubmatch(d.Sections["Design review"], -1) {
		if m[1] != rc.Rule {
			continue
		}
		then, err := c.loadDesign(m[2])
		if err != nil {
			continue
		}
		for _, was := range then.Front.RuleChanges {
			if sameRuleChange(was, rc) {
				return true
			}
		}
	}
	return false
}

func sameRuleChange(a, b RuleChange) bool {
	return a.Rule == b.Rule && a.Change == b.Change && sameSet(a.Paths, b.Paths)
}

// skipValid applies the FLOW-19 criteria for skipping a design review. A
// design never approved is judged on its own; a design already approved is
// judged on its delta from the design at its last approval, so a minimal
// change after a return to Design needs no re-approval.
func (c *checker) skipValid(d *Design) {
	const name = "design review skip is valid (FLOW-19)"
	if n := len(d.Front.RuleChanges); n > 0 {
		c.add(name, false, "the design declares %d rule_changes, each approved in review",
			n)
		return
	}
	for _, q := range c.f.Questions {
		if (q.Kind == "assumed" && q.Status != "confirmed") ||
			(q.Kind == "blocking" && q.Status == "open") {
			c.add(name, false, "question %s is %s %s", q.ID, q.Status, q.Kind)
			return
		}
	}
	base := "origin/main"
	if a := approvals(d); len(a) > 0 {
		base = a[len(a)-1]
		prev, err := c.loadDesign(base)
		if err != nil {
			c.add(name, false, "approved design at %s: %v", base, err)
			return
		}
		if diff := impactDelta(&prev.Front, &d.Front); diff != "" {
			c.add(name, false, "changed since the approval at %s: %s", base, diff)
			return
		}
	} else {
		for _, q := range c.f.Questions {
			if q.Kind == "assumed" {
				c.add(name, false, "assumed question %s needs a review to confirm",
					q.ID)
				return
			}
		}
		if why := impactOf(&d.Front); why != "" {
			c.add(name, false, "%s", why)
			return
		}
	}
	files, err := c.git("diff", "--name-only", base+"..."+c.f.Branch, "--",
		"docs/development/")
	if err != nil {
		c.add(name, false, "%v", err)
		return
	}
	c.add(name, files == "", "rules changed since %s: %q", base, files)
}

// impactOf returns why a design, judged on its own, needs a review.
func impactOf(fm *Front) string {
	switch {
	case fm.Alters == nil || *fm.Alters:
		return "alters_relationships is not false"
	case len(fm.Seams) > 0:
		return fmt.Sprintf("seams %v", fm.Seams)
	case fm.SecurityBoundary != "none":
		return "security_boundary " + fm.SecurityBoundary
	case len(blockingHosts(fm)) > 0:
		return fmt.Sprintf("blocking native validation on %v", blockingHosts(fm))
	}
	return ""
}

// impactDelta returns how the impact fields changed between two versions of
// a design, or "".
func impactDelta(was, now *Front) string {
	var out []string
	if ptr(was.Alters) != ptr(now.Alters) {
		out = append(out, fmt.Sprintf("alters_relationships %s -> %s", ptr(was.Alters),
			ptr(now.Alters)))
	}
	if !sameSet(was.Seams, now.Seams) {
		out = append(out, fmt.Sprintf("seams %v -> %v", was.Seams, now.Seams))
	}
	if was.SecurityBoundary != now.SecurityBoundary {
		out = append(out, fmt.Sprintf("security_boundary %s -> %s", was.SecurityBoundary,
			now.SecurityBoundary))
	}
	if !sameSet(blockingHosts(was), blockingHosts(now)) {
		out = append(out, fmt.Sprintf("blocking hosts %v -> %v", blockingHosts(was),
			blockingHosts(now)))
	}
	return strings.Join(out, "; ")
}

func blockingHosts(fm *Front) []string {
	var out []string
	for _, nv := range fm.NativeValidation {
		if nv.Blocking != nil && *nv.Blocking {
			out = append(out, nv.Host)
		}
	}
	return out
}

func sameSet(a, b []string) bool {
	a, b = append([]string(nil), a...), append([]string(nil), b...)
	sort.Strings(a)
	sort.Strings(b)
	return len(a) == len(b) && (len(a) == 0 || reflect.DeepEqual(a, b))
}
