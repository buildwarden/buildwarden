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
var approvedRe = regexp.MustCompile("(?i)approved at `?([0-9a-f]{7,40})")

// approvals returns the commits of the design's recorded approvals, oldest
// first.
func approvals(d *Design) []string {
	var out []string
	for _, m := range approvedRe.FindAllStringSubmatch(d.Sections["Design review"], -1) {
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

// reviewed is Plan's done-when for FLOW-19: every design review seeded so
// far has an approval recorded, or the skip is still valid against the
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

// skipValid applies the FLOW-19 criteria for skipping a design review. A
// design never approved is judged on its own; a design already approved is
// judged on its delta from the design at its last approval, so a minimal
// change after a return to Design needs no re-approval.
func (c *checker) skipValid(d *Design) {
	const name = "design review skip is valid (FLOW-19)"
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
