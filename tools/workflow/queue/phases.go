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
		// (FLOW-19); see seeded.
		{{"plan", "cloud", nil, false}, {"design-review", "human", []string{"plan"}, false}},
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

// stepTemplates parses a step id and returns its phase's templates and its
// name without any -r<n> suffix.
func stepTemplates(id string) ([]template, string) {
	m := stepIDRe.FindStringSubmatch(id)
	if m == nil {
		return nil, ""
	}
	ts := templates[letterRound(m[1])]
	phase := int(m[2][0] - '0')
	if phase >= len(ts) {
		return nil, ""
	}
	return ts[phase], retryRe.ReplaceAllString(m[3], "")
}

// isTemplate reports whether the step is one its phase seeds, in any of its
// -r<n> forms, rather than one a session added.
func isTemplate(st *Step) bool {
	ts, name := stepTemplates(st.ID)
	for _, t := range ts {
		if t.name == name {
			return true
		}
	}
	return false
}

// gateTemplate is the phase's last seeded template step.
func gateTemplate(f *Feature, ts []template) (template, bool) {
	var last template
	ok := false
	for _, t := range ts {
		if seeded(f, t) {
			last, ok = t, true
		}
	}
	return last, ok
}

// IsGate reports whether the step is its phase's last seeded template step,
// whose completion the FLOW-03 done-when check must confirm.
func IsGate(f *Feature, st *Step) bool {
	ts, name := stepTemplates(st.ID)
	gate, ok := gateTemplate(f, ts)
	return ok && gate.name == name
}

// Seed adds the template steps for the feature's current round and phase.
// When the phase's gate has waitsAdded (Publish), the gate also needs every
// pending step already added to the phase, and each added host step needs
// what the gate needs (consolidate), so validation runs on the branch the PR
// opens on (FLOW-03, FLOW-09). A step added after seeding is not linked.
func Seed(f *Feature) {
	ts := templates[f.Round]
	if f.Phase >= len(ts) || len(ts[f.Phase]) == 0 {
		return
	}
	// A phase entered again (FLOW-06) gets fresh ids: <id>-r2, <id>-r3, ...
	suffix := ""
	for n := 2; f.hasStep(StepID(f.Round, f.Phase, ts[f.Phase][0].name) + suffix); n++ {
		suffix = fmt.Sprintf("-r%d", n)
	}
	gate, _ := gateTemplate(f, ts[f.Phase])
	var added []string
	for i := range f.Steps {
		st := &f.Steps[i]
		if st.Status != "pending" || st.Phase != f.Phase || stepRound(st.ID) != f.Round ||
			isTemplate(st) {
			continue
		}
		added = append(added, st.ID)
		if gate.waitsAdded && strings.HasPrefix(st.Affinity, "host:") {
			for _, n := range gate.needs {
				st.Needs = addNeeds(st.Needs, StepID(f.Round, f.Phase, n)+suffix)
			}
		}
	}
	for _, t := range ts[f.Phase] {
		if !seeded(f, t) {
			continue
		}
		st := Step{ID: StepID(f.Round, f.Phase, t.name) + suffix, Phase: f.Phase,
			Affinity: t.affinity, Status: "pending"}
		for _, n := range t.needs {
			st.Needs = append(st.Needs, StepID(f.Round, f.Phase, n)+suffix)
		}
		if t.waitsAdded {
			st.Needs = addNeeds(st.Needs, added...)
		}
		f.Steps = append(f.Steps, st)
	}
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
// review after Plan (FLOW-19). Pending steps added for later phases stay,
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
		if gate, ok := gateTemplate(f, ts); ok && gate.waitsAdded {
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
