package check

import (
	"fmt"
	"slices"
	"strings"

	"github.com/buildwarden/buildwarden/tools/workflow/queue"
)

// state is the queue state the checks read the area map and the other items
// from. With none given there is no map, and every item is undeclared.
func (c *checker) state() *queue.State {
	if c.env.State != nil {
		return c.env.State
	}
	return &queue.State{}
}

// ruleIDs are the rule IDs of the design's rule_changes, which the queue
// records as rules_changed.
func (fm *Front) ruleIDs() []string {
	var out []string
	for _, rc := range fm.RuleChanges {
		out = append(out, rc.Rule)
	}
	return out
}

// rulePaths are the paths of the design's rule_changes.
func (fm *Front) rulePaths() []string {
	var out []string
	for _, rc := range fm.RuleChanges {
		out = append(out, rc.Paths...)
	}
	return out
}

// Declarations reads what the item's design declares on its branch, for wf
// declare to copy into the queue (FLOW-02), with the commit it was read at.
// It also reads what an agent's widening is judged in: the design at the
// commit its last approval names, since the branch is what the agent edits.
// A design whose review was skipped has no approved commit and is read on the
// branch. With neither an approval nor a skip, Within is empty.
func Declarations(env Env, f *queue.Feature) (queue.Declaration, error) {
	c := &checker{env: env, f: f}
	commit, err := c.git("rev-parse", "--short", "refs/heads/"+f.Branch)
	if err != nil {
		return queue.Declaration{}, err
	}
	d, err := c.loadDesign(f.Branch)
	if err != nil {
		return queue.Declaration{}, err
	}
	fm := &d.Front
	out := queue.Declaration{Commit: commit, Repos: fm.Repos, Areas: fm.Areas,
		Seams: fm.Seams, RulesChanged: fm.ruleIDs(), Alters: fm.Alters,
		DesignReview: fm.DesignReview, MergeFirst: fm.MergeFirst}
	for _, w := range f.Work {
		if slices.Contains(fm.Repos[min(1, len(fm.Repos)):], w.Repo) {
			continue
		}
		// The design drops the repository: wf declare refuses while the
		// branch is there.
		head, err := c.remoteHeadIn(w.Repo, w.Branch)
		if err != nil {
			return queue.Declaration{}, err
		}
		if head != "" {
			out.Live = append(out.Live, w.Repo)
		}
	}
	within := func(fm *Front) queue.Within {
		return queue.Within{Areas: fm.Areas, Drivers: fm.Drivers, Surfaces: fm.Surfaces,
			RulePaths: fm.rulePaths()}
	}
	switch a := approvals(d); {
	case len(a) > 0:
		base := a[len(a)-1]
		prev, err := c.loadDesign(base)
		if err != nil {
			return out, fmt.Errorf("approved design at %s: %w", base, err)
		}
		out.Within = within(&prev.Front)
		out.Within.Commit = base
	case fm.DesignReview == "skip":
		out.Within = within(fm)
		out.Within.OnBranch = true
	}
	return out, nil
}

// queueDiffers lists how the item's queue file differs from what its design
// declares. With all unset it leaves out alters_relationships and
// design_review, which Design's done-when reports under their own names.
func (c *checker) queueDiffers(fm *Front, all bool) []string {
	f, s := c.f, c.state()
	var out []string
	differ := func(field string, design, queue any) {
		out = append(out, fmt.Sprintf("%s: design %v, queue %v", field, design, queue))
	}
	if d, q := s.ReposOf(&queue.Feature{Repos: fm.Repos}), s.ReposOf(f); !slices.Equal(d, q) {
		differ("repos", d, q)
	}
	if d, q := areasText(fm.Areas), areasText(f.Areas); !sameAreas(fm.Areas, f.Areas) {
		differ("areas", d, q)
	}
	if !sameSet(fm.Seams, f.Seams) {
		differ("seams", fm.Seams, f.Seams)
	}
	if !sameSet(fm.ruleIDs(), f.RulesChanged) {
		differ("rules_changed", fm.ruleIDs(), f.RulesChanged)
	}
	if !all {
		return out
	}
	if d, q := ptr(fm.Alters), ptr(f.AltersRelationships); d != q {
		differ("alters_relationships", d, q)
	}
	if fm.DesignReview != f.DesignReview {
		differ("design_review", fmt.Sprintf("%q", fm.DesignReview),
			fmt.Sprintf("%q", f.DesignReview))
	}
	return out
}

func sameAreas(a, b *[]string) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return sameSet(*a, *b)
}

// areasText prints declared areas, or "undeclared" for an absent key, which
// an empty list is not.
func areasText(a *[]string) string {
	if a == nil {
		return "undeclared"
	}
	return fmt.Sprint(*a)
}

const queueMatches = "queue records what the design declares (FLOW-03)"

// declaredAtDesign is Design's done-when for what the item declares
// (FLOW-02, FLOW-03): wf declare has copied the design into the queue; each
// declared area is in the map, where one it lacks is reported and does not
// fail the step, since the item is undeclared until an attended session adds
// the area; and the areas cover what the design's other fields imply, which
// an undeclared design skips.
func (c *checker) declaredAtDesign(d *Design) {
	c.add("declared recorded in queue (FLOW-02)", c.f.Declared != "",
		"declared %q: wf declare copies the design's declarations into the queue",
		c.f.Declared)
	diffs := c.queueDiffers(&d.Front, false)
	c.add(queueMatches, len(diffs) == 0, "%s", orText(strings.Join(diffs, "; "),
		"repos, areas, seams and rules_changed as the design declares them"))
	const implied = "areas cover what the design implies (FLOW-02)"
	m := c.state().Areas
	switch {
	case d.Front.Areas == nil:
		c.add(implied, true, "undeclared: the design leaves areas out, and the item "+
			"collides with every item in its repositories (FLOW-05)")
		return
	case m == nil:
		c.add(implied, true, "the queue state has no area map: not checked, and the "+
			"repository is one exclusive area (FLOW-05)")
		return
	}
	var missing []string
	for _, a := range *d.Front.Areas {
		if !m.Has(a) {
			missing = append(missing, a)
		}
	}
	if len(missing) > 0 {
		c.add("declared areas are in the area map (FLOW-05)", true, "%s is not in the "+
			"area map: the item is undeclared until an attended session adds it",
			strings.Join(missing, ", "))
	}
	problems := impliedAreas(&d.Front, m)
	c.add(implied, len(problems) == 0, "%s", orText(strings.Join(problems, "; "),
		"drivers, surfaces and rule_changes each have a declared area"))
}

func orText(s, empty string) string {
	if s == "" {
		return empty
	}
	return s
}

// impliedAreas lists the declarations of the front matter that have no
// declared area to land in. Areas name what the change edits, so each driver
// needs its own area or a shared one: a change to a shared package declares
// every driver and no driver's tree. `interface` needs a shared area or
// every driver's. Each surface needs its areas, and each path of a rule
// change the area that holds it. The map says which areas those are.
func impliedAreas(fm *Front, m *queue.AreaMap) []string {
	declared := func(area string) bool { return slices.Contains(*fm.Areas, area) }
	shared := slices.ContainsFunc(m.Implies.Shared, declared)
	var out []string
	miss := func(format string, a ...any) { out = append(out, fmt.Sprintf(format, a...)) }
	for _, driver := range fm.Drivers {
		area, known := m.Implies.Drivers[driver]
		switch {
		case shared || (known && declared(area)):
		case driver == "interface":
			if !everyDriver(m, declared) {
				miss("drivers interface: neither a shared area nor every " +
					"driver's area is declared")
			}
		case !known:
			miss("driver %s: the area map gives it no area (implies.drivers), and no "+
				"shared area is declared", driver)
		default:
			miss("driver %s: neither its area %s nor a shared area is declared",
				driver, area)
		}
	}
	for _, surface := range fm.Surfaces {
		for _, area := range m.Implies.Surfaces[surface] {
			if !declared(area) {
				miss("surface %s: its area %s is not declared", surface, area)
			}
		}
	}
	for _, rc := range fm.RuleChanges {
		for _, p := range rc.Paths {
			area, err := m.AreaOf(p)
			switch {
			case err != nil:
				miss("rule_changes %s: %v", rc.Rule, err)
			case !declared(area):
				miss("rule_changes %s: %s is in area %s, which is not declared",
					rc.Rule, p, area)
			}
		}
	}
	return out
}

// everyDriver reports whether the area of every driver the map knows is
// declared.
func everyDriver(m *queue.AreaMap, declared func(string) bool) bool {
	for _, area := range m.Implies.Drivers {
		if !declared(area) {
			return false
		}
	}
	return len(m.Implies.Drivers) > 0
}

// declaredStill checks, from Plan on, that the queue still records what the
// design declares (FLOW-03): the limits and the relations are computed from
// the queue, so a design changed on the branch alone would run beside items
// it collides with. An item with no declared finished Design before wf
// declare existed: it is not compared, and the result says so.
func (c *checker) declaredStill() {
	if c.f.Declared == "" {
		c.add(queueMatches, true, "not declared: finished Design before wf declare")
		return
	}
	d, err := c.loadDesign(c.f.Branch)
	if err != nil {
		c.add(queueMatches, false, "%v", err)
		return
	}
	diffs := c.queueDiffers(&d.Front, true)
	if len(diffs) > 0 {
		c.add(queueMatches, false, "%s: widen with wf declare inside the approved "+
			"design, or back to Design (FLOW-06)", strings.Join(diffs, "; "))
		return
	}
	c.add(queueMatches, true, "as declared at %s", c.f.Declared)
}
