package queue

import (
	"slices"
)

// The ways two items relate (FLOW-05).
const (
	// Outside: one of the two is a process item, which stops other work only
	// through its holds.
	Outside = "outside"
	// Colliding: the two change the same thing, or wf cannot tell that they
	// do not.
	Colliding = "colliding"
	// Near: the two change different things that are linked. They are built
	// side by side and take turns at the PR.
	Near = "near"
	// Disjoint: neither counts against the other in any limit.
	Disjoint = "disjoint"
)

// Relation is how two items relate, the row of FLOW-05's relation table that
// decided it, and the reason wf prints: empty for Outside and Disjoint.
type Relation struct {
	Kind   string
	Row    int
	Reason string
}

// String is the relation as wf relations prints it: the kind, and the reason
// in parentheses when there is one.
func (r Relation) String() string {
	if r.Reason == "" {
		return r.Kind
	}
	return r.Kind + " (" + r.Reason + ")"
}

// pair is two items and what several rows of the relation table read.
type pair struct {
	s    *State
	a, b *Feature
	// repos are the repositories the two have in common, sorted.
	repos []string
}

// relationRows is FLOW-05's relation table, in its order: the first row that
// applies decides. A row returns its reason, and whether it applies. Where a
// row could name several seams, rules, areas or items it names the first in
// sorted order, so the reason is the same in both orders of the pair.
var relationRows = []struct {
	kind    string
	applies func(pair) (string, bool)
}{
	{Outside, func(p pair) (string, bool) { return "", p.a.IsProcess() || p.b.IsProcess() }},
	{Colliding, pair.named},
	{Colliding, pair.unwrittenSeam},
	{Colliding, pair.noMap},
	{Colliding, pair.undeclared},
	{Colliding, pair.sharedArea},
	{Near, pair.writtenSeam},
	{Near, pair.sameRule},
	{Near, pair.reaches},
	{Disjoint, func(pair) (string, bool) { return "", true }},
}

// Relation returns how items a and b relate (FLOW-05). It is computed from
// the two queue files, the area map and config.yaml's repository, and from
// nothing else: no clock and no git, so every worker computes the same
// relation from the same pushed state. It is total, since the last row takes
// whatever is left, and symmetric. A tracked item's relations are computed by
// the same rows; the limits skip it.
func (s *State) Relation(a, b *Feature) Relation {
	p := pair{s: s, a: a, b: b, repos: common(s.ReposOf(a), s.ReposOf(b))}
	for i, row := range relationRows {
		if reason, ok := row.applies(p); ok {
			return Relation{Kind: row.kind, Row: i + 1, Reason: reason}
		}
	}
	// Not reached: the last row applies to every pair.
	return Relation{Kind: Disjoint, Row: len(relationRows)}
}

// ReposOf returns the repositories the item works in, its home first. A
// written list is used as written; an item that names none works only in the
// queue's own repository, config.yaml's repository (LOOP-04).
func (s *State) ReposOf(f *Feature) []string {
	if len(f.Repos) > 0 {
		return f.Repos
	}
	return []string{s.Config.Repository}
}

// Undeclared reports whether the item has no areas the map can read: its
// areas are absent, or name an area the map lacks, or there is no map of
// its home repository, whose areas they name. An undeclared item
// collides with every item it shares a repository with. An empty list is a
// declaration.
func (s *State) Undeclared(f *Feature) bool {
	m := s.Areas
	if f.Areas == nil || !s.hasMap(s.ReposOf(f)[0]) {
		return true
	}
	return slices.ContainsFunc(*f.Areas, func(a string) bool { return !m.Has(a) })
}

// hasMap reports whether the queue state has the repository's area map. It
// has at most one, areas.yaml, for the repository that file names.
func (s *State) hasMap(repo string) bool {
	return repo != "" && s.Areas != nil && s.Areas.Repository == repo
}

// written reports whether the seam has a specification (the map's
// written_seams). With no map none has.
func (s *State) written(seam string) bool {
	return s.Areas != nil && slices.Contains(s.Areas.WrittenSeams, seam)
}

// common returns the names both lists have, sorted.
func common(x, y []string) []string {
	var out []string
	for _, n := range x {
		if slices.Contains(y, n) && !slices.Contains(out, n) {
			out = append(out, n)
		}
	}
	slices.Sort(out)
	return out
}

// named is row 2: one names the other in collides_with.
func (p pair) named() (string, bool) {
	return "named by the human", slices.Contains(p.a.CollidesWith, p.b.Feature) ||
		slices.Contains(p.b.CollidesWith, p.a.Feature)
}

// unwrittenSeam is row 3: both list a seam that has no specification. Seams
// count across repositories.
func (p pair) unwrittenSeam() (string, bool) {
	for _, seam := range common(p.a.Seams, p.b.Seams) {
		if !p.s.written(seam) {
			return "seam " + seam + ", which has no specification", true
		}
	}
	return "", false
}

// noMap is row 4: the two have a repository in common that has no area map,
// which is then one exclusive area.
func (p pair) noMap() (string, bool) {
	for _, repo := range p.repos {
		if p.s.hasMap(repo) {
			continue
		}
		if repo == "" {
			// An item that names no repos, under a config.yaml that names
			// none either.
			repo = "the queue's repository"
		}
		return repo + " has no area map", true
	}
	return "", false
}

// undeclared is row 5: the two have a repository in common and either is
// undeclared.
func (p pair) undeclared() (string, bool) {
	if len(p.repos) == 0 {
		return "", false
	}
	var names []string
	for _, f := range []*Feature{p.a, p.b} {
		if p.s.Undeclared(f) {
			names = append(names, f.Feature)
		}
	}
	if len(names) == 0 {
		return "", false
	}
	return slices.Min(names) + " is undeclared", true
}

// areas returns the two items' areas when the map relates them: they have a
// repository in common, so by rows 4 and 5 it has the map and both items are
// declared. ok is false for two items with no repository in common, whose
// areas name different maps.
func (p pair) areas() (a, b []string, ok bool) {
	if len(p.repos) == 0 || p.s.Undeclared(p.a) || p.s.Undeclared(p.b) {
		return nil, nil, false
	}
	return *p.a.Areas, *p.b.Areas, true
}

// sharedArea is row 6: the two share an exclusive area. Sharing an ordered
// area relates nothing.
func (p pair) sharedArea() (string, bool) {
	a, b, ok := p.areas()
	if !ok {
		return "", false
	}
	for _, area := range common(a, b) {
		if p.s.Areas.Exclusive(area) {
			return "area " + area, true
		}
	}
	return "", false
}

// writtenSeam is row 7: both list a seam that has a specification.
func (p pair) writtenSeam() (string, bool) {
	for _, seam := range common(p.a.Seams, p.b.Seams) {
		if p.s.written(seam) {
			return "seam " + seam, true
		}
	}
	return "", false
}

// sameRule is row 8: both change the same rule. Rules count across
// repositories.
func (p pair) sameRule() (string, bool) {
	if rules := common(p.a.RulesChanged, p.b.RulesChanged); len(rules) > 0 {
		return "rule " + rules[0], true
	}
	return "", false
}

// reaches is row 9: an area of one reaches an area of the other, in either
// direction. Only an exclusive area reaches or is reached: an ordered area
// has no reaches, whatever its code imports.
func (p pair) reaches() (string, bool) {
	a, b, ok := p.areas()
	if !ok {
		return "", false
	}
	m := p.s.Areas
	var found []string
	note := func(from, to string) {
		if m.Exclusive(from) && m.Exclusive(to) && m.Reaches(from, to) {
			found = append(found, from+" reaches "+to)
		}
	}
	for _, x := range a {
		for _, y := range b {
			note(x, y)
			note(y, x)
		}
	}
	if len(found) == 0 {
		return "", false
	}
	return slices.Min(found), true
}
