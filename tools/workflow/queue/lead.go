package queue

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// leadLastPhase is the last phase whose cloud template steps a lead takes:
// Guide. Publish needs gh, which a machine's settings deny, so Publish, Merge
// and Close stay cloud (FLOW-04).
const leadLastPhase = 5

var leadRe = regexp.MustCompile(`^host:[a-z0-9-]+$`)

// leads returns the affinity a template step of the given phase and template
// affinity is seeded with on the feature: its lead in place of cloud in
// phases 0 to 5 of a led feature, and the template's otherwise.
func (f *Feature) leads(phase int, affinity string) string {
	if f.Lead != "" && f.Round == "feature" && phase <= leadLastPhase &&
		affinity == "cloud" {
		return f.Lead
	}
	return affinity
}

// templateAffinity returns the affinity the step's template gives it, and
// false for a step no template seeds.
func templateAffinity(st *Step) (string, bool) {
	r, ok := stepTemplates(st.ID)
	if !ok {
		return "", false
	}
	for _, t := range r.ts {
		if t.name == r.name {
			return t.affinity, true
		}
	}
	return "", false
}

// validateLead checks the lead's form and that the item may be led: only a
// feature homed here, since a process item's work is the loop's machinery and
// a tracked item's steps are all human (FLOW-09). Whether a machine serves it
// is validateNative's.
func (s *State) validateLead(f *Feature, bad func(string, ...any)) {
	switch {
	case f.Lead == "":
	case !leadRe.MatchString(f.Lead):
		bad("lead %q is not host:<name> (FLOW-04)", f.Lead)
	case f.IsProcess():
		bad("lead on a process item: a process item is never led (FLOW-09)")
	case s.Tracked(f):
		bad("lead on a tracked item: its steps are all human (FLOW-09)")
	}
}

// SetLead sets the feature's lead, or with lead "" clears it, for any worker
// (FLOW-04, FLOW-09). It re-labels each pending template step of phases 0 to
// 5 whose template affinity is cloud and whose lease is not held, between
// cloud and the lead; a held step keeps its affinity, so no session loses a
// step it is working. It writes one history line naming the old and new lead
// and the steps changed and left. A lead is routing, not placement: no
// priority, order or limit changes.
func (s *State) SetLead(f *Feature, worker, lead string, now time.Time) error {
	if err := s.mayLead(f, lead); err != nil {
		return err
	}
	old := f.Lead
	f.Lead = lead
	changed, left := f.relabel(now)
	line := fmt.Sprintf("lead %s -> %s (FLOW-04)", orNone(old), orNone(lead))
	if len(changed) > 0 {
		line += "; re-labelled " + strings.Join(changed, ", ")
	}
	if len(left) > 0 {
		line += "; left " + strings.Join(left, ", ")
	}
	f.Log(now, worker, "%s", line)
	return nil
}

// mayLead returns why the feature may not take the lead, or nil.
func (s *State) mayLead(f *Feature, lead string) error {
	switch {
	case f.IsProcess():
		return fmt.Errorf("%s is a process item: it is never led (FLOW-09)", f.Feature)
	case s.Tracked(f):
		return fmt.Errorf("%s is tracked: its steps are all human (FLOW-09)", f.Feature)
	case f.Status == "done":
		return fmt.Errorf("%s is done", f.Feature)
	case lead != "" && !leadRe.MatchString(lead):
		return fmt.Errorf("lead %q is not host:<name> (FLOW-04)", lead)
	case lead == f.Lead:
		return fmt.Errorf("%s's lead is already %s", f.Feature, orNone(lead))
	case lead != "" && !s.Serves(lead):
		return fmt.Errorf("no machine in hosts.yaml serves %s", lead)
	}
	return s.MayLeadOn(f, lead)
}

// relabel gives each pending cloud template step of phases 0 to 5 the
// affinity the feature's lead now gives it, unless a live lease holds it. It
// returns the steps it changed and those it left.
func (f *Feature) relabel(now time.Time) (changed, left []string) {
	for i := range f.Steps {
		st := &f.Steps[i]
		aff, ok := templateAffinity(st)
		if !ok || aff != "cloud" || st.Status != "pending" || st.Phase > leadLastPhase ||
			stepRound(st.ID) != "feature" {
			continue
		}
		if st.Lease.Held(now) {
			left = append(left, st.ID+" (leased to "+st.Lease.ClaimedBy+")")
			continue
		}
		if want := f.leads(st.Phase, aff); st.Affinity != want {
			st.Affinity = want
			changed = append(changed, st.ID)
		}
	}
	return changed, left
}

// Serves reports whether some machine in hosts.yaml serves the host name of
// the affinity host:<name>. Without hosts.yaml every host name is served: the
// table is optional, and nothing is routed by it then.
func (s *State) Serves(affinity string) bool {
	h, ok := strings.CutPrefix(affinity, "host:")
	return ok && (len(s.Machines) == 0 || s.served()[h])
}

// orNone is the lead as a history line names it.
func orNone(lead string) string {
	if lead == "" {
		return "none"
	}
	return lead
}
