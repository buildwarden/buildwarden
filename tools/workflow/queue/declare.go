package queue

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
)

// Declaration is what an item's design declares, as wf declare copies it into
// the queue file (FLOW-02): the front matter's repos, areas, seams, the rule
// IDs of its rule_changes, alters_relationships and design_review.
type Declaration struct {
	// Commit is the commit of the item's branch the design was read at,
	// which the queue file records as declared.
	Commit       string
	Repos        []string
	Areas        *[]string
	Seams        []string
	RulesChanged []string
	Alters       *bool
	DesignReview string
	// MergeFirst are the design's merge_first: the repositories after the
	// home whose pull request merges before the home's is ready (FLOW-08).
	MergeFirst []string
	// Live are the repositories of the item's work entries whose branch
	// exists on that repository's remote: an entry the declaration would
	// drop is refused while its branch exists.
	Live []string
	// Within is the design a widening after Design is judged in.
	Within Within
}

// Within is the design an agent's widening is judged in, once Design is
// done: the item's design at the commit its last approval names, not on the
// branch, which the same agent is editing.
type Within struct {
	// Commit is the commit the design's last approval names. It is empty
	// with OnBranch, and when the design has neither an approval nor a
	// skipped review.
	Commit string
	// OnBranch says the design's review was skipped: it has no approved
	// commit, so it was read on the branch, and the history line says so.
	OnBranch bool
	// Areas are the areas that design declares.
	Areas *[]string
	// Drivers, Surfaces and RulePaths are that design's drivers, surfaces
	// and the paths of its rule_changes: what an added area must follow
	// from.
	Drivers, Surfaces, RulePaths []string
}

const designChange = "a design change: back to Design (FLOW-06)"

// Declare copies what the item's design declares into its queue file and
// records the commit as declared (FLOW-02). What it accepts depends on where
// the item is and who asks:
//
//   - Until Design is done, any value from any worker; but with
//     design_review: skip a change that narrows is refused, since the review
//     is where the human agrees to more room (FLOW-19).
//   - In Plan, nothing from an agent: the Plan gate compares the design with
//     its approval.
//   - From Implement on, from an agent only added areas, each within the
//     approved design (Within); every other difference is a design change
//     (FLOW-06).
//   - After Design, any value from a human: this is how the review narrows,
//     and how an item that finished Design before wf declare is declared.
//
// No declaration is accepted that adds a limit finding to Validate: the
// change is applied, the limits' predicate is run, and the item is restored
// when a set is newly over. A process item is outside the relations, so only
// its repos are recorded.
func (s *State) Declare(f *Feature, d Declaration, worker string, now time.Time) error {
	if err := s.MayDeclare(f, worker); err != nil {
		return err
	}
	if !commitRe.MatchString(d.Commit) {
		return fmt.Errorf("the design's commit %q is not a commit id", d.Commit)
	}
	to := s.target(f, d)
	within, err := s.declarable(f, to, d, worker)
	if err != nil {
		return err
	}
	changes := s.changes(f, to, within)
	if d.Within.OnBranch && len(within) > 0 {
		changes += " (design on the branch: review skipped)"
	}
	work, err := declaredWork(f, to.Repos, d.MergeFirst, d.Live)
	if err != nil {
		return err
	}
	if w := workChange(f.Work, work); w != "" {
		if changes == "nothing changed" {
			changes = ""
		} else {
			changes += "; "
		}
		changes += "work " + w
	}
	before, was := *f, s.limitFindings()
	f.Repos, f.Areas, f.Seams, f.RulesChanged = to.Repos, to.Areas, to.Seams, to.RulesChanged
	f.Work = work
	f.AltersRelationships, f.DesignReview = to.Alters, to.DesignReview
	if over := s.newLimitFindings(was); len(over) > 0 {
		*f = before
		then := "back to Design (FLOW-06)"
		if f.Phase <= 1 {
			then = "raise a blocking question (FLOW-07)"
		}
		return fmt.Errorf("%s; wf declare changed nothing: %s", strings.Join(over, "; "),
			then)
	}
	f.Declared = d.Commit
	f.Log(now, worker, "declared at %s: %s", d.Commit, changes)
	return nil
}

// declaredWork is the item's work as the design declares it: one entry per
// repository after the home, on the home's branch name, first when
// merge_first names it. An entry it keeps keeps its pull request and merge.
// It refuses to drop an entry whose branch is live on its remote: the
// branch would then be one no gate sees.
func declaredWork(f *Feature, repos, mergeFirst, live []string) ([]Work, error) {
	var out []Work
	for i, r := range repos {
		if i == 0 {
			continue
		}
		w := Work{Repo: r, Branch: f.Branch}
		for _, old := range f.Work {
			if old.Repo == r {
				w.PR, w.Merged = old.PR, old.Merged
			}
		}
		w.First = slices.Contains(mergeFirst, r)
		out = append(out, w)
	}
	for _, old := range f.Work {
		if !slices.ContainsFunc(out, func(w Work) bool { return w.Repo == old.Repo }) &&
			slices.Contains(live, old.Repo) {
			return nil, fmt.Errorf("%s: the design no longer names %s, but the "+
				"item's branch %s exists there; wf declare changed nothing "+
				"(FLOW-16)",
				f.Feature, old.Repo, old.Branch)
		}
	}
	return out, nil
}

// mergeFirstOf is the declaration's merge_first, limited to the repositories
// it names after the home, as declaredWork reads it.
func mergeFirstOf(d Declaration) []string {
	var out []string
	for i, r := range d.Repos {
		if i > 0 && slices.Contains(d.MergeFirst, r) {
			out = append(out, r)
		}
	}
	return out
}

// mergeFirstChange describes how the declaration changes which work entries
// are first, or "". An item declared before work existed gains its entries
// at its next declaration, which changes no repository, so it has none.
func mergeFirstChange(f *Feature, to Declaration) string {
	if len(f.Work) == 0 {
		return ""
	}
	var was []string
	for _, w := range f.Work {
		if w.First {
			was = append(was, w.Repo)
		}
	}
	return delta(was, mergeFirstOf(to))
}

// workChange describes the change of the work entries' repositories and
// their first marks, or "".
func workChange(was, now []Work) string {
	names := func(ws []Work) []string {
		var out []string
		for _, w := range ws {
			n := w.Repo
			if w.First {
				n += " (first)"
			}
			out = append(out, n)
		}
		return out
	}
	return delta(names(was), names(now))
}

// MayDeclare says why the item has nothing for wf declare to copy, before its
// design is read: no worker is named, the item is tracked, whose design is in
// a repository wf cannot read, or it is done.
func (s *State) MayDeclare(f *Feature, worker string) error {
	switch {
	case worker == "":
		return errors.New("wf declare needs -worker")
	case s.Tracked(f):
		return fmt.Errorf("%s is tracked: wf cannot read its design in %s; wf new "+
			"records what it declares", f.Feature, f.Repos[0])
	case f.Status == "done":
		return fmt.Errorf("%s is done", f.Feature)
	}
	return nil
}

// target is what the declaration makes of the item's declared fields. A
// process item keeps all but its repos and their work: the relations do not
// read the rest.
func (s *State) target(f *Feature, d Declaration) Declaration {
	if !f.IsProcess() {
		return d
	}
	return Declaration{Repos: d.Repos, Areas: f.Areas, Seams: f.Seams,
		RulesChanged: f.RulesChanged, Alters: f.AltersRelationships,
		DesignReview: f.DesignReview, MergeFirst: d.MergeFirst, Live: d.Live}
}

// declarable applies the rule for where the item is and who asks. For an
// agent's widening it returns why each added area is within the approved
// design.
func (s *State) declarable(
	f *Feature, to, d Declaration, worker string,
) (map[string]string, error) {
	switch {
	case f.Phase <= 1:
		how := s.Narrows(f, to.Repos, to.Areas)
		if how != "" && to.DesignReview == "skip" {
			return nil, fmt.Errorf("%s: %s: narrowing needs the design review "+
				"(FLOW-19)", f.Feature, how)
		}
	case Kind(worker) == "human":
	case !f.IsProcess() && f.Phase == 2:
		return nil, fmt.Errorf("%s is in Plan: the Plan gate compares the design with "+
			"its approval, and wf declare changes nothing there (FLOW-19)", f.Feature)
	default:
		return s.widening(f, to, d.Within)
	}
	return nil, nil
}

// reposOr returns the repositories as ReposOf reads a written list: a design
// or a queue file that names none works only in the queue's repository.
func (s *State) reposOr(repos []string) []string {
	if len(repos) > 0 {
		return repos
	}
	return []string{s.Config.Repository}
}

// Narrows says how the repositories and areas narrow what the queue records
// for the item, or "": a repository or an area the queue has is removed, or
// areas are declared where the queue has none, which ends the item's
// collision with every item in its repositories. Narrowing claims more room
// to run beside other items than the human agreed to.
func (s *State) Narrows(f *Feature, repos []string, areas *[]string) string {
	if gone := minus(s.ReposOf(f), s.reposOr(repos)); len(gone) > 0 {
		return "it removes the repository " + strings.Join(gone, ", ")
	}
	switch {
	case areas == nil:
		return ""
	case f.Areas == nil:
		return "it declares areas where the queue records none"
	}
	if gone := minus(*f.Areas, *areas); len(gone) > 0 {
		return "it removes the area " + strings.Join(gone, ", ")
	}
	return ""
}

// minus returns the names of a that b lacks, in a's order.
func minus(a, b []string) []string {
	var out []string
	for _, n := range a {
		if !slices.Contains(b, n) {
			out = append(out, n)
		}
	}
	return out
}

// delta is "+x -y" for the names added to and removed from was.
func delta(was, now []string) string {
	var out []string
	for _, n := range minus(now, was) {
		out = append(out, "+"+n)
	}
	for _, n := range minus(was, now) {
		out = append(out, "-"+n)
	}
	return strings.Join(out, " ")
}

func sameAreas(a, b *[]string) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return delta(*a, *b) == ""
}

func boolOrUnset(b *bool) string {
	if b == nil {
		return "unset"
	}
	return fmt.Sprint(*b)
}

func orUnset(v string) string {
	if v == "" {
		return "unset"
	}
	return v
}

// changes describes what the declaration changes in the item's queue file,
// for its history. within gives, for an added area, why it is within the
// approved design.
func (s *State) changes(f *Feature, to Declaration, within map[string]string) string {
	var out []string
	add := func(field, change string) {
		if change != "" {
			out = append(out, field+" "+change)
		}
	}
	add("repos", delta(s.ReposOf(f), s.reposOr(to.Repos)))
	add("areas", areasChange(f.Areas, to.Areas, within))
	add("seams", delta(f.Seams, to.Seams))
	add("rules_changed", delta(f.RulesChanged, to.RulesChanged))
	if was, now := boolOrUnset(f.AltersRelationships), boolOrUnset(to.Alters); was != now {
		add("alters_relationships", was+" -> "+now)
	}
	if f.DesignReview != to.DesignReview {
		add("design_review", orUnset(f.DesignReview)+" -> "+orUnset(to.DesignReview))
	}
	if len(out) == 0 {
		return "nothing changed"
	}
	return strings.Join(out, "; ")
}

// areasChange describes a change of the areas: the names added and removed,
// or the step between undeclared and declared.
func areasChange(was, now *[]string, within map[string]string) string {
	switch {
	case was == nil && now == nil:
		return ""
	case now == nil:
		return "undeclared, were " + listOrNone(*was)
	case was == nil:
		return "declared: " + listOrNone(*now)
	}
	var out []string
	for _, n := range minus(*now, *was) {
		if why := within[n]; why != "" {
			n += " (" + why + ")"
		}
		out = append(out, "+"+n)
	}
	for _, n := range minus(*was, *now) {
		out = append(out, "-"+n)
	}
	return strings.Join(out, " ")
}

func listOrNone(names []string) string {
	if len(names) == 0 {
		return "none"
	}
	return strings.Join(names, ", ")
}

// widening is the rule for an agent once Design is done: the declaration may
// differ from the queue file only by added areas, each of which the map has
// and the approved design implies. It returns why each added area is within
// that design.
func (s *State) widening(f *Feature, to Declaration, w Within) (map[string]string, error) {
	var differs []string
	for field, change := range map[string]string{
		"repos":         delta(s.ReposOf(f), s.reposOr(to.Repos)),
		"seams":         delta(f.Seams, to.Seams),
		"rules_changed": delta(f.RulesChanged, to.RulesChanged),
	} {
		if change != "" {
			differs = append(differs, field+" "+change)
		}
	}
	if was, now := boolOrUnset(f.AltersRelationships), boolOrUnset(to.Alters); was != now {
		differs = append(differs, "alters_relationships "+was+" -> "+now)
	}
	if change := mergeFirstChange(f, to); change != "" {
		differs = append(differs, "merge_first "+change)
	}
	if f.DesignReview != to.DesignReview {
		differs = append(differs, "design_review "+orUnset(f.DesignReview)+" -> "+
			orUnset(to.DesignReview))
	}
	var added []string
	switch {
	case f.Areas == nil || to.Areas == nil:
		if change := areasChange(f.Areas, to.Areas, nil); change != "" {
			differs = append(differs, "areas "+change)
		}
	default:
		if gone := minus(*f.Areas, *to.Areas); len(gone) > 0 {
			differs = append(differs, "areas -"+strings.Join(gone, " -"))
		}
		added = minus(*to.Areas, *f.Areas)
	}
	if len(differs) > 0 {
		slices.Sort(differs)
		return nil, fmt.Errorf("%s: %s: %s", f.Feature, strings.Join(differs, "; "),
			designChange)
	}
	within := map[string]string{}
	for _, area := range added {
		why, err := s.within(f, area, w)
		if err != nil {
			return nil, fmt.Errorf("%s: %w: %s", f.Feature, err, designChange)
		}
		within[area] = why
	}
	return within, nil
}

// within says why the added area follows from the approved design: it holds
// a path of one of the design's rule_changes, the map gives it to one of the
// design's surfaces, or it is the area of one of the design's drivers. The
// last holds only for a design that declares no shared area: a change to a
// shared package lists every driver and edits none of their trees, so for it
// a driver's tree is a design change.
func (s *State) within(f *Feature, area string, w Within) (string, error) {
	if w.Commit == "" && !w.OnBranch {
		return "", fmt.Errorf("area %s is added, and there is no approved design to "+
			"judge a widening in: the design has no approval and its review was not "+
			"skipped", area)
	}
	m := s.Areas
	if m == nil || !m.Has(area) {
		return "", fmt.Errorf("area %s is not in the area map (wf areas -list)", area)
	}
	for _, p := range w.RulePaths {
		if a, err := m.AreaOf(p); err == nil && a == area {
			return "within rule_changes: " + p, nil
		}
	}
	for _, surface := range w.Surfaces {
		if slices.Contains(m.Implies.Surfaces[surface], area) {
			return "within surfaces: " + surface, nil
		}
	}
	design := "the approved design (at " + w.Commit + ")"
	if w.OnBranch {
		design = "the design on the branch, whose review was skipped"
	}
	for _, driver := range w.Drivers {
		if m.Implies.Drivers[driver] != area {
			continue
		}
		if shared := s.declaresShared(f, w); shared != "" {
			return "", fmt.Errorf("area %s is outside %s: it declares the shared "+
				"area %s, so it lists the driver %s without editing its tree",
				area, design, shared, driver)
		}
		return "within drivers: " + driver, nil
	}
	return "", fmt.Errorf("area %s is outside %s: no rule_changes path, surface or "+
		"driver of it implies the area", area, design)
}

// declaresShared returns a shared area (the map's implies.shared) that the
// approved design or the queue file declares, or "". The queue file counts
// too: a human's declaration after the approval is part of the design the
// approval names.
func (s *State) declaresShared(f *Feature, w Within) string {
	for _, areas := range []*[]string{w.Areas, f.Areas} {
		if areas == nil {
			continue
		}
		for _, a := range *areas {
			if slices.Contains(s.Areas.Implies.Shared, a) {
				return a
			}
		}
	}
	return ""
}

// limitFindings are Validate's limit findings as the state stands, by their
// text.
func (s *State) limitFindings() map[string]bool {
	out := map[string]bool{}
	for _, err := range s.validateLimits() {
		out[err.Error()] = true
	}
	return out
}

// newLimitFindings returns the limit findings the state has that was lacks.
func (s *State) newLimitFindings(was map[string]bool) []string {
	var out []string
	for _, err := range s.validateLimits() {
		if !was[err.Error()] {
			out = append(out, err.Error())
		}
	}
	return out
}

// Relate records, or with clear removes, the human's word that the item
// collides with the named item, whatever the areas say (FLOW-05). Only a
// human worker may: it changes what may run side by side. It is refused, as
// a declaration is, when the collision would put a limit over.
func (s *State) Relate(f *Feature, with string, clear bool, worker string, now time.Time,
) error {
	if Kind(worker) != "human" {
		return errors.New("only a human names a collision (FLOW-05)")
	}
	if _, err := s.Find(with); err != nil {
		return err
	}
	has := slices.Contains(f.CollidesWith, with)
	switch {
	case with == f.Feature:
		return fmt.Errorf("%s cannot collide with itself", f.Feature)
	case clear && !has:
		return fmt.Errorf("%s does not name %s in collides_with", f.Feature, with)
	case clear:
		f.CollidesWith = slices.DeleteFunc(slices.Clone(f.CollidesWith),
			func(n string) bool { return n == with })
		f.Log(now, worker, "no longer collides with %s by the human's word (FLOW-05)",
			with)
		return nil
	case has:
		return fmt.Errorf("%s already names %s in collides_with", f.Feature, with)
	}
	before, was := f.CollidesWith, s.limitFindings()
	f.CollidesWith = append(slices.Clone(f.CollidesWith), with)
	if over := s.newLimitFindings(was); len(over) > 0 {
		f.CollidesWith = before
		return fmt.Errorf("%s; wf relate changed nothing", strings.Join(over, "; "))
	}
	f.Log(now, worker, "collides with %s: named by the human (FLOW-05)", with)
	return nil
}
