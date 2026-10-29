package queue

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// PhaseNames are the FLOW-03 phases, by number.
var PhaseNames = []string{"branch-setup", "design", "plan", "implement", "review", "guide",
	"publish", "sign-off", "archive"}

// processPhaseNames are a process item's phases, numbered from 1.
var processPhaseNames = []string{"", "design", "implement", "sign-off", "archive"}

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
	"parallel": {
		{{"branch-setup", "cloud", nil}},
		{{"design", "cloud", nil}},
		{{"plan", "cloud", nil}},
		{{"implement", "cloud", nil}},
		{{"review", "cloud", nil}},
		{{"guide", "cloud", nil}},
		{{"publish", "cloud", nil}},
	},
	"merge": {
		{{"merge-main", "cloud", nil}},
		{{"consolidate", "cloud", nil}},
		{{"plan", "cloud", nil}},
		{{"implement", "cloud", nil}},
		{{"review", "cloud", nil}},
		{{"guide", "cloud", nil}},
		{{"confirm-pr", "human", nil}, {"publish", "cloud", []string{"confirm-pr"}}},
		{{"sign-off", "human", nil}},
		{{"archive", "cloud", nil}},
	},
	// A process item changes the rules the loops run under, which the loop
	// may not do unattended (FLOW-14): human throughout, phases 1 to 4.
	"process": {
		nil,
		{{"design", "human", nil}},
		{{"implement", "human", nil}},
		{{"sign-off", "human", nil}},
		{{"archive", "human", nil}},
	},
}

// roundLetters prefix step ids: p1-design, m6-publish, x3-sign-off.
var roundLetters = map[string]string{"parallel": "p", "merge": "m", "process": "x"}

func letterRound(l string) string {
	for r, x := range roundLetters {
		if x == l {
			return r
		}
	}
	return ""
}

var (
	stepIDRe = regexp.MustCompile(`^([pmx])([0-8])-([a-z0-9-]+)$`)
	retryRe  = regexp.MustCompile(`-r[0-9]+$`)
)

// StepID builds a step id: round letter, phase digit, name.
func StepID(round string, phase int, name string) string {
	return fmt.Sprintf("%s%d-%s", roundLetters[round], phase, name)
}

// IsGate reports whether the step is its phase's last template step, whose
// completion the FLOW-03 done-when check must confirm.
func IsGate(st *Step) bool {
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
	last := ts[phase][len(ts[phase])-1]
	return last.name == retryRe.ReplaceAllString(m[3], "")
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

// Back returns the feature to Design with the finding recorded (FLOW-06).
// Pending steps of later phases are dropped. After the second return the
// redesign waits on a human step instead of looping again.
func Back(f *Feature, by string, now time.Time, finding string) {
	returns := 0
	for _, e := range f.History {
		if strings.HasPrefix(e.Event, "back to design") {
			returns++
		}
	}
	kept := f.Steps[:0]
	for _, st := range f.Steps {
		if st.Status == "done" || st.Phase <= 1 {
			kept = append(kept, st)
		}
	}
	f.Steps = kept
	f.Phase = 1
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
// done, moves the feature on: the next phase, the merge queue after the
// parallel round's Publish, the merge round when it is taken, or done after
// Archive. The caller has already run the done-when check for gate steps.
func Complete(f *Feature, st *Step, by string, now time.Time) {
	st.Status = "done"
	st.Lease = Lease{}
	f.Log(now, by, "done %s", st.ID)

	for _, o := range f.Steps {
		if o.Status != "done" && o.Phase == f.Phase && stepRound(o.ID) == f.Round {
			return
		}
	}
	switch {
	case st.ID == StepID("parallel", 6, "merge-round"):
		f.Round, f.Phase = "merge", 0
		f.Log(now, by, "entered the merge round")
		Seed(f)
	case f.Round == "parallel" && f.Phase == 6:
		f.Steps = append(f.Steps, Step{
			ID:    StepID("parallel", 6, "merge-round"),
			Phase: 6, Affinity: "cloud", Status: "pending",
			Note: "Waits for the serial merge queue (FLOW-05).",
		})
		f.Log(now, by, "parallel round done; waiting on the merge queue")
	case f.Round == "merge" && f.Phase == 8, f.Round == "process" && f.Phase == 4:
		f.Status = "done"
		f.Log(now, by, "archived; feature done")
	default:
		f.Phase++
		f.Log(now, by, "entered phase %d (%s)", f.Phase, PhaseName(f))
		Seed(f)
	}
}

func stepRound(id string) string {
	m := stepIDRe.FindStringSubmatch(id)
	if m == nil {
		return ""
	}
	return letterRound(m[1])
}
