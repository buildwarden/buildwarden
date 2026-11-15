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
	"slices"
	"sort"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"
)

// Config is config.yaml at the root of the workflow branch.
type Config struct {
	// PriorityConfirmed gates unattended runs: the loop does no work until the
	// human has confirmed the priority order.
	PriorityConfirmed bool `yaml:"priority_confirmed"`
	// Repository is the repository whose gates wf runs and whose area map an
	// item's areas name, as owner/name. It has no default: unset, an item
	// that names repos fails Validate.
	Repository string     `yaml:"repository,omitempty"`
	RunBudget  StepBudget `yaml:"run_budget"`
	// NativeBudget bounds one scheduled native session's batch, so native
	// work never competes with the cloud loop (FLOW-13). Nil falls back to
	// RunBudget.
	NativeBudget *StepBudget `yaml:"native_budget,omitempty"`
	Limits       struct {
		// InProgress is how many features may be in progress at once
		// (FLOW-05). Nil, the key absent, is 5 (inProgressLimit); a written
		// value below 1 is an error.
		InProgress *int `yaml:"in_progress,omitempty"`
		// AltersRelationships is how many features may hold an
		// alters_relationships slot at once (FLOW-05, FLOW-20). Nil, the key
		// absent, is 1 (altersLimit); a written value below 1 is an error.
		AltersRelationships *int `yaml:"alters_relationships,omitempty"`
		// WaitingOnHuman is the number of items waiting on the human at
		// which no queued feature starts (FLOW-05). Nil, the key absent, is
		// 8 (waitingLimit); a written value below 1 is an error, and no value
		// means "no cap".
		WaitingOnHuman *int `yaml:"waiting_on_human,omitempty"`
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
	// Priority orders items within a FLOW-05 class; 1 is the highest. Unique
	// across the queue.
	Priority int `yaml:"priority"`
	// Kind is feature (the default, empty) or process: a change to the rules
	// the loops run under, which FLOW-05 classes ahead of every feature not
	// in implementation.
	Kind string `yaml:"kind,omitempty"`
	// Follows names the item this one fixes or finishes. A feature that
	// follows an item is a fix (FLOW-05 class 3) and starts only once that
	// item is done; on a process item it is only a record.
	Follows string `yaml:"follows,omitempty"`
	// Urgent is the human worker who marked the item urgent, as in
	// human/jeff: the claim that starts it is not refused by the waiting
	// limit (FLOW-05). Empty, the item is not marked. Only a human sets or
	// clears it.
	Urgent string `yaml:"urgent,omitempty"`
	// Holds lists the affinity patterns (cloud, host:*, host:macos-vz) whose
	// workers take no new step while this process item is in progress.
	Holds  []string `yaml:"holds,omitempty"`
	Status string   `yaml:"status"` // queued | in_progress | done
	// Round is feature or process. parallel and merge remain only on
	// finished items from before the single round (wf migrate).
	Round  string `yaml:"round"`
	Phase  int    `yaml:"phase"` // FLOW-03 phase, 0..8
	Design string `yaml:"design"`
	Branch string `yaml:"branch"`
	// Repos lists the repositories the item works in, as owner/name, its
	// home first: the home holds its design, its branch and its PR
	// (LOOP-04). Empty, the item works only in Config.Repository.
	Repos []string `yaml:"repos,omitempty"`
	// Areas names the areas of the area map whose files the item changes
	// (FLOW-02, FLOW-05). Nil, the key absent, is undeclared: the item
	// collides with every item it shares a repository with. An empty list is
	// a declaration that it changes nothing but its own design, which is why
	// this is a pointer: the two must stay apart through a save and a load.
	Areas *[]string `yaml:"areas,omitempty"`
	// Seams are the seams the item changes, the design's seams (FLOW-02).
	Seams []string `yaml:"seams,omitempty"`
	// RulesChanged are the rule IDs of the design's rule_changes (FLOW-14).
	RulesChanged []string `yaml:"rules_changed,omitempty"`
	// Declared is the commit of the item's branch whose design wf declare
	// last copied into this file. Empty, the item was never declared, and no
	// gate compares this file with its design.
	Declared string `yaml:"declared,omitempty"`
	// Adopts marks a reconciliation item (LOOP-07).
	Adopts *Adopts `yaml:"adopts,omitempty"`
	// CollidesWith names items the human has said this one collides with,
	// whatever the areas say.
	CollidesWith []string `yaml:"collides_with,omitempty"`
	// AltersRelationships is nil until Design decides it (FLOW-02).
	AltersRelationships *bool `yaml:"alters_relationships"`
	// DesignReview is empty until Design decides it: required, or skip when
	// the design meets the FLOW-19 criteria. It decides whether phase 2
	// seeds a design-review step before Plan.
	DesignReview string     `yaml:"design_review,omitempty"`
	Source       string     `yaml:"source,omitempty"`
	PR           *PR        `yaml:"pr,omitempty"`
	Steps        []Step     `yaml:"steps"`
	Budget       Budget     `yaml:"budget"`
	Questions    []Question `yaml:"questions"`
	History      []Event    `yaml:"history"`
}

// Adopts is what a reconciliation item adopts: a release of a consumable
// (LOOP-07).
type Adopts struct {
	Consumable string `yaml:"consumable"`
	Version    string `yaml:"version"`
}

// PR records the feature's pull request, opened in Publish (FLOW-08, FLOW-11).
type PR struct {
	Number int    `yaml:"number"`
	CI     string `yaml:"ci"` // pending | green | red
	// Head is the commit CI was read for. Empty, on a PR recorded before
	// the field existed, that commit is not known.
	Head string `yaml:"head,omitempty"`
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

// Budget is the per-feature runaway check (FLOW-13). Runs and steps are both
// counted; only steps trip the check, and only for an open item.
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
	// Areas is areas.yaml, the area map of Config.Repository; the file is
	// optional. Nil, the repository is one exclusive area (FLOW-05).
	Areas *AreaMap
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
	if err := s.loadAreas(); err != nil {
		return nil, err
	}
	return s, nil
}

// sortFeatures puts s.Features in FLOW-05 order, a stable starting order for
// code that walks them; readers that need the current order use Ordered.
func (s *State) sortFeatures() { sortFeatures(s.Features) }

// sortFeatures sorts by class, then in-progress before queued, then priority;
// done items have class 0 and go last, by priority.
func sortFeatures(fs []*Feature) {
	sort.SliceStable(fs, func(i, j int) bool {
		a, b := fs[i], fs[j]
		if ca, cb := a.Class(), b.Class(); ca != cb {
			return cb == 0 || (ca != 0 && ca < cb)
		}
		if ia, ib := a.Status == "in_progress", b.Status == "in_progress"; ia != ib {
			return ia
		}
		return a.Priority < b.Priority
	})
}

// Ordered returns every item in FLOW-05 order. Class changes with status and
// phase, which a command changes as it runs, so the order is computed at each
// call rather than stored.
func (s *State) Ordered() []*Feature {
	out := slices.Clone(s.Features)
	sortFeatures(out)
	return out
}

// Listed returns the items wf status prints, in FLOW-05 order: the open
// (queued and in-progress) ones, plus the done ones after them when all is
// set, for a retrospective (FLOW-13).
func (s *State) Listed(all bool) []*Feature {
	var out []*Feature
	for _, f := range s.Ordered() {
		if all || f.Status != "done" {
			out = append(out, f)
		}
	}
	return out
}

// IsProcess reports whether the item is a process change (kind: process).
func (f *Feature) IsProcess() bool { return f.Kind == "process" }

// Class returns the item's FLOW-05 class: 1 in implementation, 2 a process
// item, 3 a fix (a feature that follows another item), 4 new functionality.
// A done item has no class, 0.
func (f *Feature) Class() int {
	switch {
	case f.Status == "done":
		return 0
	case f.inImplementation():
		return 1
	case f.IsProcess():
		return 2
	case f.Follows != "":
		return 3
	}
	return 4
}

// inImplementation reports whether the item is in progress past Plan and not
// waiting for the human's merge: a merge check must not go ahead of process
// and fix work, and the merge is confirmed before wf next anyway.
func (f *Feature) inImplementation() bool {
	if f.Status != "in_progress" {
		return false
	}
	if f.IsProcess() {
		return f.Phase == 2 || f.Phase == 4
	}
	return f.Phase >= 3 && f.Phase != 7
}

// Add creates a queued feature at the given priority, moving the features at
// or below it down one with a history line each, and returns every feature it
// changed.
func (s *State) Add(f *Feature, now time.Time, by string) ([]*Feature, error) {
	if _, err := s.Find(f.Feature); err == nil {
		return nil, fmt.Errorf("feature %q already queued", f.Feature)
	}
	changed := []*Feature{f}
	for _, o := range s.Features {
		if o.Priority >= f.Priority {
			o.Priority++
			o.Log(now, by, "priority %d -> %d (renumbered by wf new: %s queued at %d)",
				o.Priority-1, o.Priority, f.Feature, f.Priority)
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

// PutOnHold files a queued item on hold (FLOW-20's freeze): an open blocking
// question q1 whose text is why, and a human step <f0|x1>-answer-q1 that the
// item's first step needs, so no worker is offered the item's work until the
// human answers q1 (FLOW-07). wf answer completes that step. It is for an
// item Add has just queued: holding one already in progress stays a
// hand-edit, and one that already has a question is refused, never merged.
func (f *Feature) PutOnHold(why string, now time.Time, by string) error {
	if strings.TrimSpace(why) == "" {
		return fmt.Errorf(
			"a hold needs its reason, the blocking question's text (FLOW-07)")
	}
	if f.Status != "queued" || len(f.Steps) == 0 || len(f.Questions) > 0 {
		return fmt.Errorf("%s: only a newly queued item with no question is put on hold",
			f.Feature)
	}
	const resume = "resume it"
	f.Questions = append(f.Questions, Question{ID: "q1", Kind: "blocking", Status: "open",
		Question: why, Options: []string{resume}, Recommended: resume})
	answer := Step{ID: StepID(f.Round, f.Phase, "answer-q1"), Phase: f.Phase,
		Affinity: "human", Status: "pending"}
	f.Steps[0].Needs = addNeeds(f.Steps[0].Needs, answer.ID)
	f.Steps = append([]Step{answer}, f.Steps...)
	f.Log(now, by, "on hold (q1): %s", why)
	return nil
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

// Save writes one feature back to queue/<feature>.yaml and regenerates
// QUEUE.md, so the summary is committed with the change that produced it.
func (s *State) Save(f *Feature) error {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(f); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(s.Dir, "queue", f.Feature+".yaml"), buf.Bytes(),
		0o644); err != nil {
		return err
	}
	return s.WriteSummary()
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

// altersLimit is how many features may hold an alters_relationships slot: the
// value config.yaml writes (FLOW-20), or FLOW-05's 1 when it writes none. A
// written value is returned as it is, one below 1 included, which Validate
// reports and under which Next gives no slot: no default stands in for it
// (GLOBAL-10).
func (s *State) altersLimit() int {
	if n := s.Config.Limits.AltersRelationships; n != nil {
		return *n
	}
	return 1
}

// inProgressLimit is how many features may be in progress: the value
// config.yaml writes, or FLOW-05's 5 when it writes none. A written value is
// returned as it is, one below 1 included, which Validate reports and under
// which nothing starts (GLOBAL-10).
func (s *State) inProgressLimit() int {
	if n := s.Config.Limits.InProgress; n != nil {
		return *n
	}
	return 5
}

// waitingLimit is the number of items waiting on the human at which no
// queued feature starts: the value config.yaml writes, or FLOW-05's 8 when it
// writes none. A written value is returned as it is; Validate reports one
// below 1.
func (s *State) waitingLimit() int {
	if n := s.Config.Limits.WaitingOnHuman; n != nil {
		return *n
	}
	return 8
}

// Tracked reports whether the item's home, its first repository, is not the
// queue's own: wf sees no branch, design or PR there, so it runs no gate for
// the item and counts it in no limit (FLOW-03, FLOW-05).
func (s *State) Tracked(f *Feature) bool {
	return len(f.Repos) > 0 && f.Repos[0] != s.Config.Repository
}

// validateConfig checks config.yaml's budgets and limits. A limit is checked
// as inProgressLimit, altersLimit and waitingLimit return it, so an absent
// key is its default and a written value below 1 is the finding.
func (s *State) validateConfig() []error {
	var errs []error
	if s.Config.RunBudget.Steps < 1 {
		errs = append(errs, fmt.Errorf("config: run_budget.steps %d is below 1",
			s.Config.RunBudget.Steps))
	}
	if nb := s.Config.NativeBudget; nb != nil && nb.Steps < 1 {
		errs = append(errs, fmt.Errorf("config: native_budget.steps %d is below 1",
			nb.Steps))
	}
	for _, l := range []struct {
		key   string
		value int
	}{{"in_progress", s.inProgressLimit()}, {"alters_relationships", s.altersLimit()},
		{"waiting_on_human", s.waitingLimit()}} {
		if l.value < 1 {
			errs = append(errs, fmt.Errorf("config: limits.%s %d is below 1", l.key,
				l.value))
		}
	}
	return errs
}

// Validate checks the schema of every feature and the FLOW-05 limits. The
// alters_relationships limit is compared with the features that hold a slot
// (altersHolders), the count Next withholds a Plan step by, so a state Next
// produces never fails here.
func (s *State) Validate() []error {
	errs := s.validateConfig()
	limit := s.altersLimit()
	prio := map[int]string{}
	inProgress := 0
	for _, f := range s.Features {
		bad := func(format string, a ...any) {
			errs = append(errs, fmt.Errorf("%s: %s", f.Feature,
				fmt.Sprintf(format, a...)))
		}
		if other, ok := prio[f.Priority]; ok {
			bad("priority %d also held by %s", f.Priority, other)
		}
		prio[f.Priority] = f.Feature
		s.validateFollows(f, bad)
		if f.Status == "in_progress" && !f.IsProcess() {
			inProgress++
		}
		tracked := s.Tracked(f)
		if len(f.Repos) > 0 && s.Config.Repository == "" {
			errs = append(errs, fmt.Errorf(
				"config: repository is not set, and %s names repos", f.Feature))
			// Which repository is the queue's own is not known, so neither is
			// whether the item is tracked: its round is checked as written.
			tracked = f.Round == "tracked"
		}
		f.validate(bad, tracked)
		s.validateDeclared(f, bad)
		s.validateUrgent(f, bad)
	}
	// No count means anything against a limit below 1.
	if n := s.inProgressLimit(); n >= 1 && inProgress > n {
		errs = append(errs, fmt.Errorf("%d features in progress, limit %d (FLOW-05)",
			inProgress, n))
	}
	// No count of holders means anything against a limit below 1.
	if holders := s.altersHolders(); limit >= 1 && len(holders) > limit {
		errs = append(errs, fmt.Errorf(
			"%d features hold an alters_relationships slot, limit %d (FLOW-05): %s",
			len(holders), limit, strings.Join(holders, ", ")))
	}
	for _, e := range append(s.validateNative(), s.validateAreas()...) {
		errs = append(errs, errors.New(e))
	}
	return errs
}

// validateFollows checks that follows, when set, names another item, done or
// open.
func (s *State) validateFollows(f *Feature, bad func(string, ...any)) {
	if f.Follows == "" {
		return
	}
	if _, err := s.Find(f.Follows); err != nil || f.Follows == f.Feature {
		bad("follows %q, which is not another item in the queue", f.Follows)
	}
}

var (
	repoRe   = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)
	commitRe = regexp.MustCompile(`^[0-9a-f]{7,40}$`)
)

// validateDeclared checks what the item declares (FLOW-02, FLOW-03): repos,
// areas, declared, adopts and collides_with. An area the map lacks is not an
// error: the item is undeclared until an attended session adds the area.
func (s *State) validateDeclared(f *Feature, bad func(string, ...any)) {
	s.validateRepos(f, bad)
	if f.Areas != nil {
		if dup := repeated(*f.Areas); dup != "" {
			bad("areas names %s twice", dup)
		}
	}
	if f.Declared != "" && !commitRe.MatchString(f.Declared) {
		bad("declared %q is not a commit id", f.Declared)
	}
	// A tracked item's seams are what intake records: wf cannot read its
	// design, so nothing was declared from one.
	if f.Declared == "" && !s.Tracked(f) && len(f.Seams)+len(f.RulesChanged) > 0 {
		bad("seams and rules_changed are what wf declare copies from the design, " +
			"and declared is not set")
	}
	if a := f.Adopts; a != nil {
		if a.Consumable == "" || a.Version == "" {
			bad("adopts needs both its consumable and its version")
		}
		if f.IsProcess() {
			bad("only a feature adopts a release (LOOP-07)")
		}
	}
	for _, name := range f.CollidesWith {
		if _, err := s.Find(name); err != nil || name == f.Feature {
			bad("collides_with %q, which is not another item in the queue", name)
		}
	}
}

// validateRepos checks the form of the item's repos.
func (s *State) validateRepos(f *Feature, bad func(string, ...any)) {
	for _, r := range f.Repos {
		if !repoRe.MatchString(r) {
			bad("repos entry %q is not owner/name", r)
		}
	}
	if dup := repeated(f.Repos); dup != "" {
		bad("repos names %s twice", dup)
	}
}

// repeated returns the first name the list has twice, or "".
func repeated(names []string) string {
	seen := map[string]bool{}
	for _, n := range names {
		if seen[n] {
			return n
		}
		seen[n] = true
	}
	return ""
}

// validateUrgent checks the urgent mark: its value is the human worker who
// set it, and the item is one whose start the waiting limit could refuse.
// Any other value is honoured by nothing, so it is an error and is read
// neither as marked nor as unmarked (GLOBAL-10). What the item fixes is not
// checked: FLOW-05's words and intake hold the mark to a security fix.
func (s *State) validateUrgent(f *Feature, bad func(string, ...any)) {
	const never = "the waiting limit never refuses"
	switch {
	case f.Urgent == "":
	case Kind(f.Urgent) != "human":
		bad("urgent names %s: only a human marks an item urgent (FLOW-05)", f.Urgent)
	case f.IsProcess():
		bad("urgent on a process item: %s one (FLOW-05)", never)
	case s.Tracked(f):
		bad("urgent on a tracked item: %s one (FLOW-05)", never)
	case f.Follows != "":
		bad("urgent, and it follows %s: %s a fix (FLOW-05)", f.Follows, never)
	}
}

func (f *Feature) validate(bad func(string, ...any), tracked bool) {
	if !oneOf(f.Status, "queued", "in_progress", "done") {
		bad("status %q is not queued, in_progress or done", f.Status)
	}
	f.validateKind(bad, tracked)
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

// validateKind checks the item's kind against its round. tracked says the
// item is homed in another repository.
func (f *Feature) validateKind(bad func(string, ...any), tracked bool) {
	switch {
	case !oneOf(f.Kind, "", "feature", "process"):
		bad("kind %q is not feature or process", f.Kind)
	case f.IsProcess() && tracked:
		bad("a process item is homed in the queue's repository, not %s "+
			"(an item homed in another repository is tracked)", f.Repos[0])
	case f.IsProcess():
		f.validateProcess(bad)
	default:
		legacy := f.Status == "done" && oneOf(f.Round, "parallel", "merge")
		switch {
		case tracked && f.Round != "tracked":
			bad("a tracked item's round is tracked, not %q", f.Round)
		case !tracked && f.Round == "tracked":
			bad("round tracked is for an item homed in another repository, and this " +
				"one is homed in the queue's")
		case !tracked && f.Round != "feature" && !legacy:
			bad("round %q is not feature (run wf migrate)", f.Round)
		}
		if len(f.Holds) > 0 {
			bad("only a process item holds workers")
		}
	}
}

func (f *Feature) validateProcess(bad func(string, ...any)) {
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

// Runaway reports whether an open item has spent its runaway budget: the
// runaway factor times its step estimate (FLOW-13). A done item never is.
func (s *State) Runaway(f *Feature) bool {
	if f.Status == "done" {
		return false
	}
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
