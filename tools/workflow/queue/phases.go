package queue

import (
	"fmt"
	"regexp"
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
}

// templates are the steps each phase starts with, per round. Design adds
// others (answer-<question>, validate-<host>) as it finds them.
var templates = map[string][][]template{
	"feature": {
		{{"branch-setup", "cloud", nil}},
		{{"design", "cloud", nil}},
		// The design review is seeded unless Design recorded a valid skip
		// (FLOW-19); see seeded.
		{{"plan", "cloud", nil}, {"design-review", "human", []string{"plan"}}},
		{{"implement", "cloud", nil}},
		{{"review", "cloud", nil}},
		{{"guide", "cloud", nil}},
		// merge-main waits for the PR slot (FLOW-05); open-pr archives the
		// design on the branch and opens the PR (FLOW-08, FLOW-12).
		{{"merge-main", "cloud", nil}, {"consolidate", "cloud", []string{"merge-main"}},
			{"open-pr", "cloud", []string{"consolidate"}}},
		// Completes by its check once a human has merged the PR (FLOW-08).
		{{"merge", "cloud", nil}},
		{{"close", "cloud", nil}},
	},
	// A process item changes the rules the loops run under. Its design is
	// the human's (the design review, FLOW-19); after that the loop works on
	// the branch, and the human's merge is the sign-off (FLOW-14).
	"process": {
		nil,
		{{"design", "human", nil}},
		{{"implement", "cloud", nil}},
		{{"merge", "cloud", nil}},
		{{"close", "cloud", nil}},
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

// IsGate reports whether the step is its phase's last seeded template step,
// whose completion the FLOW-03 done-when check must confirm.
func IsGate(f *Feature, st *Step) bool {
	m := stepIDRe.FindStringSubmatch(st.ID)
	if m == nil {
		return false
	}
	round := letterRound(m[1])
	phase := int(m[2][0] - '0')
	ts := templates[round]
	if phase >= len(ts) || len(ts[phase]) == 0 {
		return false
	}
	var last string
	for _, t := range ts[phase] {
		if seeded(f, t) {
			last = t.name
		}
	}
	return last == retryRe.ReplaceAllString(m[3], "")
}

// Seed adds the template steps for the feature's current round and phase.
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
	for _, t := range ts[f.Phase] {
		if !seeded(f, t) {
			continue
		}
		st := Step{ID: StepID(f.Round, f.Phase, t.name) + suffix, Phase: f.Phase,
			Affinity: t.affinity, Status: "pending"}
		for _, n := range t.needs {
			st.Needs = append(st.Needs, StepID(f.Round, f.Phase, n)+suffix)
		}
		f.Steps = append(f.Steps, st)
	}
}

func (f *Feature) hasStep(id string) bool {
	_, err := f.Step(id)
	return err == nil
}

// Back returns the feature to an earlier phase with the finding recorded:
// Design (FLOW-06), or Implement when Publish's consolidation finds work the
// plan already covers (FLOW-10). Pending steps of later phases are dropped and
// the phase is seeded again with fresh ids, so a return to Design also seeds
// a fresh design review after Plan (FLOW-19). After the second return to
// Design the redesign waits on a human step instead of looping again.
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
	kept := f.Steps[:0]
	for _, st := range f.Steps {
		if st.Status == "done" || st.Phase < to {
			kept = append(kept, st)
		}
	}
	f.Steps = kept
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
