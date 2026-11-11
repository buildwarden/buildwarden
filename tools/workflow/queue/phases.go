package queue

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

// PhaseNames are the FLOW-03 phases, by number.
var PhaseNames = []string{"branch-setup", "design", "plan", "implement", "review", "guide",
	"publish", "merge", "close"}

// processPhaseNames are a process item's phases, numbered from 1.
var processPhaseNames = []string{"", "design", "implement", "merge", "close"}

// PhaseName names the feature's current phase.
func PhaseName(f *Feature) string {
	names := PhaseNames
	if f.Round == "process" {
		names = processPhaseNames
	}
	if f.Phase < 0 || f.Phase >= len(names) {
		return "?"
	}
	return names[f.Phase]
}

type template struct {
	name     string
	affinity string
	needs    []string // template names in the same phase
	// waitsAdded makes this gate need the phase's pending added steps when
	// the phase is seeded, and their host steps need what the gate needs.
	waitsAdded bool
}

// templates are the steps each phase starts with, per round. Design adds
// others (answer-<question>, validate-<host>) as it finds them; a step of a
// later phase waits for its phase (Offered) and survives Back. Publish's
// gate waits for the steps added to Publish, so the PR opens only after
// blocking native validation (FLOW-03, FLOW-09).
var templates = map[string][][]template{
	"feature": {
		{{"branch-setup", "cloud", nil, false}},
		{{"design", "cloud", nil, false}},
		// The design review is seeded unless Design recorded a valid skip
		// (FLOW-19); see seeded. Plan waits for its approval.
		{{"design-review", "human", nil, false},
			{"plan", "cloud", []string{"design-review"}, false}},
		{{"implement", "cloud", nil, false}},
		{{"review", "cloud", nil, false}},
		{{"guide", "cloud", nil, false}},
		// merge-main waits for the PR slot (FLOW-05); open-pr archives the
		// design on the branch and opens the PR (FLOW-08, FLOW-12).
		{{"merge-main", "cloud", nil, false},
			{"consolidate", "cloud", []string{"merge-main"}, false},
			{"open-pr", "cloud", []string{"consolidate"}, true}},
		// Completes by its check once a human has merged the PR (FLOW-08).
		{{"merge", "cloud", nil, false}},
		{{"close", "cloud", nil, false}},
	},
	// A process item changes the rules the loops run under. Its design is
	// the human's (the design review, FLOW-19); after that the loop works on
	// the branch, and the human's merge is the sign-off (FLOW-14).
	"process": {
		nil,
		{{"design", "human", nil, false}},
		{{"implement", "cloud", nil, false}},
		{{"merge", "cloud", nil, false}},
		{{"close", "cloud", nil, false}},
	},
}

// seeded reports whether the template step belongs in the feature's phase:
// every step but a design review that Design recorded as skipped.
func seeded(f *Feature, t template) bool {
	return t.name != "design-review" || f.DesignReview != "skip"
}

// roundLetters prefix step ids: f1-design, f6-open-pr, x3-merge. The
// parallel and merge rounds' p and m ids remain only in finished items'
// history (wf migrate moved the rest).
var roundLetters = map[string]string{"feature": "f", "process": "x", "parallel": "p",
	"merge": "m"}

func letterRound(l string) string {
	for r, x := range roundLetters {
		if x == l {
			return r
		}
	}
	return ""
}

var (
	stepIDRe = regexp.MustCompile(`^([fxpm])([0-8])-([a-z0-9-]+)$`)
	retryRe  = regexp.MustCompile(`-r[0-9]+$`)
)

// StepID builds a step id: round letter, phase digit, name.
func StepID(round string, phase int, name string) string {
	return fmt.Sprintf("%s%d-%s", roundLetters[round], phase, name)
}

// stepRef is a step id parsed against the templates: its round, phase, name
// without any -r<n> suffix, that suffix, and its phase's templates.
type stepRef struct {
	round        string
	phase        int
	name, suffix string
	ts           []template
}

// stepTemplates parses a step id against the templates, the one parse that
// IsGate, IsDesignReview, IsMerge and isTemplate share. ok is false for an id that is
// not a step id or names a phase its round does not have.
func stepTemplates(id string) (stepRef, bool) {
	m := stepIDRe.FindStringSubmatch(id)
	if m == nil {
		return stepRef{}, false
	}
	r := stepRef{round: letterRound(m[1]), phase: int(m[2][0] - '0'),
		name: retryRe.ReplaceAllString(m[3], "")}
	r.suffix = m[3][len(r.name):]
	ts := templates[r.round]
	if r.phase >= len(ts) {
		return stepRef{}, false
	}
	r.ts = ts[r.phase]
	return r, true
}

// isTemplate reports whether the parsed step is one its phase seeds, in any
// of its -r<n> forms, rather than one a session added.
func (r stepRef) isTemplate() bool {
	for _, t := range r.ts {
		if t.name == r.name {
			return true
		}
	}
	return false
}

// isTemplate reports whether the step is one its phase seeds, in any of its
// -r<n> forms, rather than one a session added.
func isTemplate(st *Step) bool {
	r, ok := stepTemplates(st.ID)
	return ok && r.isTemplate()
}

// waiter is the phase's template step marked waitsAdded: Publish's gate.
func waiter(ts []template) (template, bool) {
	for _, t := range ts {
		if t.waitsAdded {
			return t, true
		}
	}
	return template{}, false
}

// IsGate reports whether the step is its phase's gate, whose completion the
// FLOW-03 done-when check must confirm: a template step of the phase that no
// other template step seeded with it (the same retry suffix) needs. The
// steps' own needs decide, not the template's order, so a phase seeded under
// an older template keeps the gate it was seeded with: phase 2 seeded before
// the design review moved ahead of Plan has the review as its gate. Needs on
// added steps (Publish's open-pr on its host steps) do not count.
func IsGate(f *Feature, st *Step) bool {
	r, ok := stepTemplates(st.ID)
	if !ok || !r.isTemplate() {
		return false
	}
	for _, t := range r.ts {
		o, err := f.Step(StepID(r.round, r.phase, t.name) + r.suffix)
		if t.name == r.name || err != nil {
			continue
		}
		if slices.Contains(o.Needs, st.ID) {
			return false
		}
	}
	return true
}

// IsDesignReview reports whether the step is a feature's design review
// (FLOW-19), seeded in phase 2 from the template.
func IsDesignReview(st *Step) bool {
	r, ok := stepTemplates(st.ID)
	return ok && r.round == "feature" && r.phase == 2 && r.name == "design-review"
}

// IsMerge reports whether the step is its round's merge step, in any of its
// -r<n> forms: f7-merge or x3-merge, which completes only once the human has
// merged the PR (FLOW-08). wf next never offers it (FLOW-05).
func IsMerge(st *Step) bool {
	r, ok := stepTemplates(st.ID)
	return ok && r.name == "merge" &&
		((r.round == "feature" && r.phase == 7) || (r.round == "process" && r.phase == 3))
}

// Seed adds the template steps for the feature's current round and phase.
// A template step's need on a step not seeded (a skipped design review) is
// dropped. When the phase's gate has waitsAdded (Publish), the gate also
// needs every pending step already added to the phase, and each added host
// step needs what the gate needs (consolidate), so validation runs on the
// branch the PR opens on (FLOW-03, FLOW-09). A step added after seeding is
// not linked.
func Seed(f *Feature) {
	ts := templates[f.Round]
	if f.Phase >= len(ts) || len(ts[f.Phase]) == 0 {
		return
	}
	// A phase entered again (FLOW-06) gets fresh ids: <id>-r2, <id>-r3, ...
	// The suffix must be free for every template step, since an earlier
	// entry may have left only some of them (a skipped review, or one
	// dropped by Back).
	suffix := ""
	for n := 2; f.anySeeded(ts[f.Phase], suffix); n++ {
		suffix = fmt.Sprintf("-r%d", n)
	}
	added := linkAdded(f, ts[f.Phase], suffix)
	in := map[string]bool{}
	for _, t := range ts[f.Phase] {
		in[t.name] = seeded(f, t)
	}
	for _, t := range ts[f.Phase] {
		if !in[t.name] {
			continue
		}
		st := Step{ID: StepID(f.Round, f.Phase, t.name) + suffix, Phase: f.Phase,
			Affinity: t.affinity, Status: "pending"}
		for _, n := range t.needs {
			if in[n] {
				st.Needs = append(st.Needs, StepID(f.Round, f.Phase, n)+suffix)
			}
		}
		if t.waitsAdded {
			st.Needs = addNeeds(st.Needs, added...)
		}
		f.Steps = append(f.Steps, st)
	}
}

// anySeeded reports whether the feature has any of the phase's template
// steps with the retry suffix.
func (f *Feature) anySeeded(ts []template, suffix string) bool {
	for _, t := range ts {
		if f.hasStep(StepID(f.Round, f.Phase, t.name) + suffix) {
			return true
		}
	}
	return false
}

// linkAdded returns the phase's pending added steps and, when the phase has a
// waitsAdded gate, makes each added host step need what that gate needs.
func linkAdded(f *Feature, ts []template, suffix string) []string {
	gate, waits := waiter(ts)
	var added []string
	for i := range f.Steps {
		st := &f.Steps[i]
		if st.Status != "pending" || st.Phase != f.Phase || stepRound(st.ID) != f.Round ||
			isTemplate(st) {
			continue
		}
		added = append(added, st.ID)
		if waits && strings.HasPrefix(st.Affinity, "host:") {
			for _, n := range gate.needs {
				st.Needs = addNeeds(st.Needs, StepID(f.Round, f.Phase, n)+suffix)
			}
		}
	}
	return added
}

// addNeeds appends the ids needs does not already hold.
func addNeeds(needs []string, ids ...string) []string {
	for _, id := range ids {
		if !slices.Contains(needs, id) {
			needs = append(needs, id)
		}
	}
	return needs
}

func (f *Feature) hasStep(id string) bool {
	_, err := f.Step(id)
	return err == nil
}

// Back returns the feature to an earlier phase with the finding recorded:
// Design (FLOW-06), or Implement when Publish's consolidation finds work the
// plan already covers (FLOW-10). It drops the pending steps of the target
// phase and the pending template steps of later phases, and seeds the phase
// again with fresh ids, so a return to Design also seeds a fresh design
// review before Plan (FLOW-19). Pending steps added for later phases stay,
// without their leases and without needs on dropped steps, for Design to
// reconcile; a host step already done in Publish is queued again, since the
// branch it validated will change. After the second return to Design the
// redesign waits on a human step instead of looping again.
func Back(f *Feature, by string, now time.Time, finding string, to int) {
	if to != 3 {
		to = 1
	}
	returns := 0
	for _, e := range f.History {
		if strings.HasPrefix(e.Event, "back to design") {
			returns++
		}
	}
	dropped := map[string]bool{}
	var kept []Step
	for _, st := range f.Steps {
		if st.Status == "done" || st.Phase < to || (st.Phase > to && !isTemplate(&st)) {
			kept = append(kept, st)
		} else {
			dropped[st.ID] = true
		}
	}
	for i := range kept {
		st := &kept[i]
		st.Needs = slices.DeleteFunc(st.Needs, func(id string) bool { return dropped[id] })
		if st.Status == "pending" && st.Phase > to {
			st.Lease = Lease{}
			f.Log(now, by, "kept %s pending for phase %d", st.ID, st.Phase)
		}
	}
	f.Steps = append(kept, rerunPublishHostSteps(f, kept, now, by)...)
	f.Phase = to
	if to == 3 {
		f.Log(now, by, "back to implement: %s", finding)
		Seed(f)
		return
	}
	f.Log(now, by, "back to design: %s", finding)
	Seed(f)
	if returns >= 2 {
		id := StepID(f.Round, 1, fmt.Sprintf("escalate-%d", returns+1))
		gate := &f.Steps[len(f.Steps)-1]
		gate.Needs = append(gate.Needs, id)
		f.Steps = append(f.Steps, Step{
			ID: id, Phase: 1, Affinity: "human", Status: "pending",
			Note: "Returned to Design more than twice (FLOW-06): the human decides."})
	}
}

// rerunPublishHostSteps returns a fresh pending copy of each Publish host
// step that is done with no pending retry, so a validation of the branch
// before the return cannot satisfy the next round's gate. Steps are grouped
// by id without its -r<n> suffix; the copy takes the next suffix and the
// latest step's affinity and note.
func rerunPublishHostSteps(f *Feature, steps []Step, now time.Time, by string) []Step {
	publish := -1
	for phase, ts := range templates[f.Round] {
		if _, ok := waiter(ts); ok {
			publish = phase
		}
	}
	type group struct {
		latest        Step
		max           int
		done, pending bool
	}
	groups := map[string]*group{}
	var order []string
	for _, st := range steps {
		if st.Phase != publish || stepRound(st.ID) != f.Round ||
			!strings.HasPrefix(st.Affinity, "host:") {
			continue
		}
		base := retryRe.ReplaceAllString(st.ID, "")
		n := 1
		if r := strings.TrimPrefix(st.ID, base); r != "" {
			n, _ = strconv.Atoi(strings.TrimPrefix(r, "-r"))
		}
		g := groups[base]
		if g == nil {
			g = &group{}
			groups[base] = g
			order = append(order, base)
		}
		if n >= g.max {
			g.latest, g.max = st, n
		}
		g.done = g.done || st.Status == "done"
		g.pending = g.pending || st.Status == "pending"
	}
	var out []Step
	for _, base := range order {
		g := groups[base]
		if !g.done || g.pending {
			continue
		}
		st := Step{ID: fmt.Sprintf("%s-r%d", base, g.max+1), Phase: publish,
			Affinity: g.latest.Affinity, Status: "pending", Note: g.latest.Note}
		f.Log(now, by, "queued %s: %s validated the branch before the return",
			st.ID, g.latest.ID)
		out = append(out, st)
	}
	return out
}

// Complete marks a step done and, once every step of the current phase is
// done, moves the feature on: the next phase, or done after Close. The caller
// has already run the done-when check for gate steps.
func Complete(f *Feature, st *Step, by string, now time.Time) {
	st.Status = "done"
	st.Lease = Lease{}
	f.Log(now, by, "done %s", st.ID)

	for _, o := range f.Steps {
		if o.Status != "done" && o.Phase == f.Phase && stepRound(o.ID) == f.Round {
			return
		}
	}
	last := len(templates[f.Round]) - 1
	if f.Phase >= last {
		f.Status = "done"
		f.Log(now, by, "closed; feature done")
		return
	}
	f.Phase++
	f.Log(now, by, "entered phase %d (%s)", f.Phase, PhaseName(f))
	Seed(f)
}

func stepRound(id string) string {
	m := stepIDRe.FindStringSubmatch(id)
	if m == nil {
		return ""
	}
	return letterRound(m[1])
}
