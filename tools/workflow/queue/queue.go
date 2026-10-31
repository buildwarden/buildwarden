// Package queue reads and writes the feature queue kept on the workflow orphan
// branch (FLOW-03, FLOW-04, FLOW-05). The schema is documented in that branch's
// README.md.
package queue

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"
)

// Config is config.yaml at the root of the workflow branch.
type Config struct {
	// PriorityConfirmed gates unattended runs: the loop does no work until the
	// human has confirmed the priority order.
	PriorityConfirmed bool       `yaml:"priority_confirmed"`
	RunBudget         StepBudget `yaml:"run_budget"`
	// NativeBudget bounds one scheduled native session's batch, so native
	// work never competes with the cloud loop (FLOW-13). Nil falls back to
	// RunBudget.
	NativeBudget *StepBudget `yaml:"native_budget,omitempty"`
	Limits       struct {
		InProgress          int `yaml:"in_progress"`
		AltersRelationships int `yaml:"alters_relationships"`
	} `yaml:"limits"`
	LeaseMinutes  int     `yaml:"lease_minutes"`
	RunawayFactor float64 `yaml:"runaway_factor"`
}

// StepBudget is how many steps one run may take.
type StepBudget struct {
	Steps int `yaml:"steps"`
}

// Budget returns the steps one run may take: the native budget for a native
// session, falling back to the run budget when config.yaml sets none.
func (c Config) Budget(native bool) int {
	if native && c.NativeBudget != nil {
		return c.NativeBudget.Steps
	}
	return c.RunBudget.Steps
}

// Feature is one queue/<feature>.yaml file.
type Feature struct {
	Feature string `yaml:"feature"`
	Title   string `yaml:"title"`
	// Priority orders features; 1 is the highest. Unique across the queue.
	Priority int `yaml:"priority"`
	// Kind is feature (the default, empty) or process: a change to the rules
	// the loops run under, which sorts ahead of every feature (FLOW-05).
	Kind string `yaml:"kind,omitempty"`
	// Holds lists the affinity patterns (cloud, host:*, host:macos-vz) whose
	// workers take no new step while this process item is in progress.
	Holds  []string `yaml:"holds,omitempty"`
	Status string   `yaml:"status"` // queued | in_progress | done
	// Round is feature or process. parallel and merge remain only on
	// finished items from before the single round (wf migrate).
	Round  string   `yaml:"round"`
	Phase  int      `yaml:"phase"`  // FLOW-03 phase, 0..8
	Design string   `yaml:"design"`
	Branch string   `yaml:"branch"`
	// AltersRelationships is nil until Design decides it (FLOW-02).
	AltersRelationships *bool `yaml:"alters_relationships"`
	// DesignReview is empty until Design decides it: required, or skip when
	// the design meets the FLOW-19 criteria. It decides whether Plan seeds
	// a design-review step.
	DesignReview        string     `yaml:"design_review,omitempty"`
	Source              string     `yaml:"source,omitempty"`
	PR                  *PR        `yaml:"pr,omitempty"`
	Steps               []Step     `yaml:"steps"`
	Budget              Budget     `yaml:"budget"`
	Questions           []Question `yaml:"questions"`
	History             []Event    `yaml:"history"`
}

// PR records the feature's pull request, opened in Publish (FLOW-08, FLOW-11).
type PR struct {
	Number int    `yaml:"number"`
	CI     string `yaml:"ci"` // pending | green | red
}

// Step is one unit of work a worker claims (FLOW-04).
type Step struct {
	ID       string   `yaml:"id"`
	Phase    int      `yaml:"phase"`
	Affinity string   `yaml:"affinity"` // cloud | human | host:<name>
	Status   string   `yaml:"status"`   // pending | done
	Needs    []string `yaml:"needs,omitempty"`
	Lease    Lease    `yaml:"lease"`
	Note     string   `yaml:"note,omitempty"`
}

// Lease is a claim on a step: who holds it and until when.
type Lease struct {
	ClaimedBy string     `yaml:"claimed_by"`
	Expires   *time.Time `yaml:"expires"`
}

// Held reports whether the lease is live at now.
func (l Lease) Held(now time.Time) bool {
	return l.ClaimedBy != "" && l.Expires != nil && now.Before(*l.Expires)
}

// Budget is the per-feature runaway check (FLOW-13), counted in runs and steps.
type Budget struct {
	Estimate Count `yaml:"estimate"`
	Spent    Count `yaml:"spent"`
}

// Count is a quantity of loop runs and steps.
type Count struct {
	Runs  int `yaml:"runs"`
	Steps int `yaml:"steps"`
}

// Question is a FLOW-07 question.
type Question struct {
	ID          string   `yaml:"id"`
	Kind        string   `yaml:"kind"`   // blocking | assumed
	Status      string   `yaml:"status"` // open | answered (blocking) / confirmed (assumed)
	Question    string   `yaml:"question"`
	Options     []string `yaml:"options"`
	Recommended string   `yaml:"recommended"`
	Answer      string   `yaml:"answer,omitempty"`
}

// Event is one history entry.
type Event struct {
	At    time.Time `yaml:"at"`
	By    string    `yaml:"by"`
	Event string    `yaml:"event"`
}

// State is the whole workflow branch as read from a checkout of it.
type State struct {
	Dir      string
	Config   Config
	Features []*Feature
	// Machines and Gaps are hosts.yaml and backlog/native-gaps.yaml; both
	// files are optional.
	Machines []Machine
	Gaps     []Gap
	// Statuses are native/status/*.yaml, one per machine (FLOW-09).
	Statuses []MachineStatus
}

// Load reads config.yaml and queue/*.yaml under dir.
func Load(dir string) (*State, error) {
	s := &State{Dir: dir}
	if err := readYAML(filepath.Join(dir, "config.yaml"), &s.Config); err != nil {
		return nil, err
	}
	paths, err := filepath.Glob(filepath.Join(dir, "queue", "*.yaml"))
	if err != nil {
		return nil, err
	}
	for _, p := range paths {
		f := &Feature{}
		if err := readYAML(p, f); err != nil {
			return nil, err
		}
		if want := strings.TrimSuffix(filepath.Base(p), ".yaml"); f.Feature != want {
			return nil, fmt.Errorf("%s: feature %q does not match its file name", p,
				f.Feature)
		}
		s.Features = append(s.Features, f)
	}
	s.sortFeatures()
	if err := s.loadNative(); err != nil {
		return nil, err
	}
	return s, nil
}

// sortFeatures orders process items ahead of every feature, then by priority
// (FLOW-05).
func (s *State) sortFeatures() {
	sort.SliceStable(s.Features, func(i, j int) bool {
		a, b := s.Features[i], s.Features[j]
		if a.IsProcess() != b.IsProcess() {
			return a.IsProcess()
		}
		return a.Priority < b.Priority
	})
}

// IsProcess reports whether the item is a process change (kind: process).
func (f *Feature) IsProcess() bool { return f.Kind == "process" }

// Add creates a queued feature at the given priority, moving the features at
// or below it down one, and returns every feature it changed.
func (s *State) Add(f *Feature, now time.Time, by string) ([]*Feature, error) {
	if _, err := s.Find(f.Feature); err == nil {
		return nil, fmt.Errorf("feature %q already queued", f.Feature)
	}
	changed := []*Feature{f}
	for _, o := range s.Features {
		if o.Priority >= f.Priority {
			o.Priority++
			changed = append(changed, o)
		}
	}
	f.Status, f.Round, f.Phase = "queued", "feature", 0
	if f.IsProcess() {
		f.Round, f.Phase = "process", 1
	}
	Seed(f)
	f.Log(now, by, "queued at priority %d (intake)", f.Priority)
	s.Features = append(s.Features, f)
	s.sortFeatures()
	return changed, nil
}

// Find returns the named feature.
func (s *State) Find(name string) (*Feature, error) {
	for _, f := range s.Features {
		if f.Feature == name {
			return f, nil
		}
	}
	return nil, fmt.Errorf("no feature %q in %s/queue", name, s.Dir)
}

// Save writes one feature back to queue/<feature>.yaml.
func (s *State) Save(f *Feature) error {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(f); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(s.Dir, "queue", f.Feature+".yaml"), buf.Bytes(), 0o644)
}

func readYAML(path string, v any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := yaml.Unmarshal(data, v); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}

var (
	affinityRe = regexp.MustCompile(`^(cloud|human|host:[a-z0-9-]+)$`)
	holdRe     = regexp.MustCompile(`^(cloud|human|host:[a-z0-9*?-]+)$`)
)

// Validate checks the schema of every feature and the FLOW-05 limits.
func (s *State) Validate() []error {
	var errs []error
	if s.Config.RunBudget.Steps < 1 {
		errs = append(errs, fmt.Errorf("config: run_budget.steps %d is below 1",
			s.Config.RunBudget.Steps))
	}
	if nb := s.Config.NativeBudget; nb != nil && nb.Steps < 1 {
		errs = append(errs, fmt.Errorf("config: native_budget.steps %d is below 1",
			nb.Steps))
	}
	prio := map[int]string{}
	inProgress, alters := 0, 0
	for _, f := range s.Features {
		bad := func(format string, a ...any) {
			errs = append(errs, fmt.Errorf("%s: %s", f.Feature,
				fmt.Sprintf(format, a...)))
		}
		if other, ok := prio[f.Priority]; ok {
			bad("priority %d also held by %s", f.Priority, other)
		}
		prio[f.Priority] = f.Feature
		if f.Status == "in_progress" {
			inProgress++
			if f.alters() && f.Phase >= 2 {
				alters++
			}
		}
		f.validate(bad)
	}
	if n := s.Config.Limits.InProgress; n > 0 && inProgress > n {
		errs = append(errs, fmt.Errorf("%d features in progress, limit %d (FLOW-05)",
			inProgress, n))
	}
	if n := s.Config.Limits.AltersRelationships; n > 0 && alters > n {
		errs = append(errs, fmt.Errorf(
			"%d features past Design alter relationships, limit %d (FLOW-05)",
			alters, n))
	}
	for _, e := range s.validateNative() {
		errs = append(errs, errors.New(e))
	}
	return errs
}

func (f *Feature) validate(bad func(string, ...any)) {
	if !oneOf(f.Status, "queued", "in_progress", "done") {
		bad("status %q is not queued, in_progress or done", f.Status)
	}
	f.validateKind(bad)
	if !oneOf(f.DesignReview, "", "required", "skip") {
		bad("design_review %q is not required or skip", f.DesignReview)
	}
	if f.Phase < 0 || f.Phase > 8 {
		bad("phase %d is outside 0..8", f.Phase)
	}
	f.validateSteps(bad)
	for _, q := range f.Questions {
		if !oneOf(q.Kind, "blocking", "assumed") {
			bad("question %s kind %q is not blocking or assumed", q.ID, q.Kind)
		}
		if q.Recommended == "" || len(q.Options) == 0 {
			bad("question %s needs options and a recommended answer (FLOW-07)", q.ID)
		}
	}
}

func (f *Feature) validateKind(bad func(string, ...any)) {
	switch {
	case !oneOf(f.Kind, "", "feature", "process"):
		bad("kind %q is not feature or process", f.Kind)
	case f.IsProcess():
		if f.Round != "process" {
			bad("a process item's round is process, not %q", f.Round)
		}
		if f.Phase < 1 || f.Phase > 4 {
			bad("process phase %d is outside 1..4", f.Phase)
		}
		for _, h := range f.Holds {
			if _, err := path.Match(h, ""); err != nil || !holdRe.MatchString(h) {
				bad("hold %q is not cloud, human or host:<pattern>", h)
			}
		}
	default:
		legacy := f.Status == "done" && oneOf(f.Round, "parallel", "merge")
		if f.Round != "feature" && !legacy {
			bad("round %q is not feature (run wf migrate)", f.Round)
		}
		if len(f.Holds) > 0 {
			bad("only a process item holds workers")
		}
	}
}

func (f *Feature) validateSteps(bad func(string, ...any)) {
	ids := map[string]bool{}
	for _, st := range f.Steps {
		if ids[st.ID] {
			bad("step id %q repeats", st.ID)
		}
		ids[st.ID] = true
		if !stepIDRe.MatchString(st.ID) {
			bad("step id %q is not <f|x><phase>-<name>", st.ID)
		}
		if !affinityRe.MatchString(st.Affinity) {
			bad("step %s affinity %q is not cloud, human or host:<name>",
				st.ID, st.Affinity)
		}
		if !oneOf(st.Status, "pending", "done") {
			bad("step %s status %q is not pending or done", st.ID, st.Status)
		}
	}
	for _, st := range f.Steps {
		for _, n := range st.Needs {
			if !ids[n] {
				bad("step %s needs unknown step %q", st.ID, n)
			}
		}
	}
}

func oneOf(s string, opts ...string) bool {
	for _, o := range opts {
		if s == o {
			return true
		}
	}
	return false
}

func (f *Feature) alters() bool { return f.AltersRelationships != nil && *f.AltersRelationships }

// OpenBlocking returns the feature's unanswered blocking questions.
func (f *Feature) OpenBlocking() []Question {
	var out []Question
	for _, q := range f.Questions {
		if q.Kind == "blocking" && q.Status == "open" {
			out = append(out, q)
		}
	}
	return out
}

// Runaway reports whether the feature has spent its runaway budget: the
// runaway factor times its step estimate (FLOW-13).
func (s *State) Runaway(f *Feature) bool {
	factor := s.Config.RunawayFactor
	if factor == 0 {
		factor = 2
	}
	est := f.Budget.Estimate.Steps
	return est > 0 && float64(f.Budget.Spent.Steps) >= factor*float64(est)
}

// Step returns the feature's step with the given id.
func (f *Feature) Step(id string) (*Step, error) {
	for i := range f.Steps {
		if f.Steps[i].ID == id {
			return &f.Steps[i], nil
		}
	}
	return nil, fmt.Errorf("%s has no step %q", f.Feature, id)
}

// Log appends a history event.
func (f *Feature) Log(now time.Time, by, format string, a ...any) {
	f.History = append(f.History, Event{At: now.UTC().Truncate(time.Second), By: by,
		Event: fmt.Sprintf(format,
			a...)})
}
