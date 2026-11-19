// Package queue reads and writes the feature queue kept on the workflow orphan
// branch (FLOW-03, FLOW-04, FLOW-05). The schema is documented in that branch's
// README.md.
package queue

import (
	"bytes"
	"cmp"
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
	Repository string `yaml:"repository,omitempty"`
	// Repositories has one entry for every repository an item may name, the
	// queue's own included: its trunk, which is the only place wf reads a
	// trunk from, and, once the change that adopts it has merged, its
	// profile file (LOOP-01, LOOP-08). An absent entry is never read as a
	// default.
	Repositories []RepositoryEntry `yaml:"repositories,omitempty"`
	RunBudget    StepBudget        `yaml:"run_budget"`
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
	// LeaseMinutes is a cloud or human claim's lease; 0 is 120 (FLOW-04).
	LeaseMinutes int `yaml:"lease_minutes"`
	// HostLeaseMinutes is a machine's claim's lease, which its session
	// renews while it works (wf renew); 0 or unset is 1200, Jeff's 20 hours
	// (FLOW-04). A value below 0 is an error.
	HostLeaseMinutes int     `yaml:"host_lease_minutes,omitempty"`
	RunawayFactor    float64 `yaml:"runaway_factor"`
}

// RepositoryEntry is one config.yaml repositories entry.
type RepositoryEntry struct {
	// Name is owner/name.
	Name string `yaml:"name"`
	// Trunk is the branch the repository's pull requests merge to.
	Trunk string `yaml:"trunk"`
	// Profile is the path of the repository's profile file in it. Empty,
	// the repository has not adopted the contract (LOOP-08).
	Profile string `yaml:"profile,omitempty"`
}

// Entry returns repo's repositories entry, or nil when it has none.
func (c Config) Entry(repo string) *RepositoryEntry {
	for i := range c.Repositories {
		if c.Repositories[i].Name == repo {
			return &c.Repositories[i]
		}
	}
	return nil
}

// Trunk returns repo's trunk from its repositories entry, the only place wf
// reads one (LOOP-01). A repository with no entry, or an entry with no
// trunk, is an error naming the fix.
func (c Config) Trunk(repo string) (string, error) {
	e := c.Entry(repo)
	switch {
	case e == nil:
		return "", fmt.Errorf("config.yaml's repositories has no entry for %s: an "+
			"attended session adds it with its trunk", repo)
	case e.Trunk == "":
		return "", fmt.Errorf("config.yaml's repositories entry %s has no trunk", repo)
	}
	return e.Trunk, nil
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
	// Priority is an open item's position in FLOW-05's order, 1 for the
	// first, which wf keeps (Renumber). A done item has none once queue/done/
	// exists; until then it holds a number past every open item's.
	Priority int `yaml:"priority,omitempty"`
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
	Holds []string `yaml:"holds,omitempty"`
	// Lead is the host name, host:<name>, whose machines take a feature's
	// cloud steps of phases 0 to 5 (FLOW-04, FLOW-09). Empty, the item is
	// not led. Any worker may set or clear it (SetLead); each change is a
	// history line.
	Lead   string `yaml:"lead,omitempty"`
	Status string `yaml:"status"` // queued | in_progress | done
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
	// Work has one entry per repository in Repos after the home: the item's
	// branch and pull request there (FLOW-03, FLOW-08). wf declare writes it
	// from the design. Empty on an item declared before it existed, which
	// keeps its gates in its home only.
	Work []Work `yaml:"work,omitempty"`
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

	// stored is the number the item's file holds and file is where that file
	// is, from the checkout's root; both are empty for an item never read or
	// written. Save writes the items whose number moved from stored.
	stored int
	file   string
}

// Work is the item's branch and pull request in one repository other than
// its home.
type Work struct {
	Repo string `yaml:"repo"`
	// Branch is the home's branch name: an item has one name in every
	// repository (FLOW-17).
	Branch string `yaml:"branch"`
	// First says the design names the repository in merge_first: its pull
	// request merges before the home's is listed ready, confirmed in Publish
	// (FLOW-08).
	First bool `yaml:"first,omitempty"`
	PR    *PR  `yaml:"pr,omitempty"`
	// Merged is the merge commit the loop confirmed on the repository's
	// trunk.
	Merged string `yaml:"merged,omitempty"`
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

// HeadIs reports whether pr.head records the commit: its full id, or an
// abbreviation of it of 7 digits or more. A PR with no head recorded has a
// result for no commit that is known.
func (p *PR) HeadIs(sha string) bool {
	return p != nil && len(p.Head) >= 7 && strings.HasPrefix(sha, p.Head)
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
	// Repo is the repository a host step runs in, one of the item's work
	// entries; empty, the item's home (FLOW-04).
	Repo string `yaml:"repo,omitempty"`
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
	// Main is main.yaml, what a session last recorded about main; the file
	// is optional. Nil, main's CI is not known, which is not read as green.
	Main *Main
	// doneDir records that queue/done/ exists: done items are written there
	// and carry no number (queue-positions, Decision 8).
	doneDir bool
	// repos are the views ForRepository has made, by repository.
	repos map[string]*State
	// summaryDir is where a view's writes regenerate QUEUE.md: the queue
	// state's own directory. Empty, it is Dir.
	summaryDir string
}

// doneFolder is where done items live once wf migrate -done has created it.
const doneFolder = "queue/done"

// Main is main.yaml: the head of main a session saw and the result of its
// check runs, whether GitHub requires an up-to-date branch on main, and the
// last comparison of the area map with main (FLOW-08, FLOW-09, FLOW-10). wf
// reads no CI itself: wf main records what the session read.
type Main struct {
	SHA string `yaml:"sha,omitempty"`
	CI  string `yaml:"ci,omitempty"` // green | red | pending
	// Run is a failing run's URL, kept while main is red.
	Run string `yaml:"run,omitempty"`
	// RedSince is the first red commit after the last green one, kept while
	// main is not green.
	RedSince string `yaml:"red_since,omitempty"`
	// Gates marks a red that a session's own gates found on SHA: a later
	// green reading of the check runs for the same commit does not clear it.
	Gates bool `yaml:"gates,omitempty"`
	// Followup is the item filed to fix a red main.
	Followup string `yaml:"followup,omitempty"`
	// UpToDate is whether GitHub requires an up-to-date branch on main:
	// required or not-required. Empty, no session has read it.
	UpToDate string `yaml:"up_to_date,omitempty"`
	// Areas is the last comparison of the area map with origin/main.
	Areas *AreasResult `yaml:"areas,omitempty"`
}

// AreasResult is what wf areas -check printed for a commit of main.
type AreasResult struct {
	SHA         string   `yaml:"sha"`
	Differences int      `yaml:"differences"`
	Lines       []string `yaml:"lines"`
}

// MainReading is what wf main is asked to record. A field left empty
// changes nothing.
type MainReading struct {
	SHA, CI, Run, Followup, UpToDate string
	Gates                            bool
}

var shaRe = regexp.MustCompile(`^[0-9a-f]{7,40}$`)

// RecordMain records a reading of main's CI, the follow-up filed for a red
// main, and whether GitHub requires an up-to-date branch on main, in
// main.yaml (FLOW-09, FLOW-10). A red keeps red_since while main stays red,
// and a green clears it with the run and the follow-up. A red the gates found
// (-gates) stays when a later reading for the same commit is green, and goes
// with another head. The follow-up is recorded once while main is red.
func (s *State) RecordMain(r MainReading) error {
	m := Main{}
	if s.Main != nil {
		m = *s.Main
	}
	if r.CI != "" || r.SHA != "" {
		if !shaRe.MatchString(r.SHA) {
			return fmt.Errorf("main: -sha %q is not a commit id", r.SHA)
		}
		if !slices.Contains([]string{"green", "red", "pending"}, r.CI) {
			return fmt.Errorf("main: -ci is green, red or pending, not %q", r.CI)
		}
		if r.Gates && r.CI != "red" {
			return errors.New("main: -gates marks a red the gates found; give -ci red")
		}
		m.reading(r)
	}
	if err := s.followup(&m, r.Followup); err != nil {
		return err
	}
	if r.UpToDate != "" {
		if r.UpToDate != "required" && r.UpToDate != "not-required" {
			return fmt.Errorf("main: -up-to-date is required or not-required, not %q",
				r.UpToDate)
		}
		m.UpToDate = r.UpToDate
	}
	s.Main = &m
	return s.SaveMain()
}

// followup records the item filed to fix a red main, once.
func (s *State) followup(m *Main, item string) error {
	if item == "" {
		return nil
	}
	if m.CI != "red" {
		return fmt.Errorf("main: -followup %s: main is not recorded red", item)
	}
	if m.Followup != "" && m.Followup != item {
		return fmt.Errorf("main: the follow-up %s is already recorded for main red "+
			"since %s", m.Followup, m.RedSince)
	}
	if _, err := s.Find(item); err != nil {
		return fmt.Errorf("main: -followup: %w", err)
	}
	m.Followup = item
	return nil
}

// reading applies a reading of main's check runs.
func (m *Main) reading(r MainReading) {
	if m.Gates && m.SHA == r.SHA && r.CI != "red" {
		// The gates fail on this commit whatever CI says.
		return
	}
	m.SHA, m.CI, m.Gates = r.SHA, r.CI, r.Gates
	switch r.CI {
	case "green":
		m.RedSince, m.Run, m.Followup = "", "", ""
	case "red":
		if m.RedSince == "" {
			m.RedSince = r.SHA
		}
		m.Run = r.Run
	default:
		m.Run = ""
	}
}

// SaveMain writes main.yaml.
func (s *State) SaveMain() error {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(s.Main); err != nil {
		return err
	}
	if err := s.mkdir(); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(s.Dir, "main.yaml"), buf.Bytes(), 0o644); err != nil {
		return err
	}
	if s.summaryDir != "" {
		// A view's main.yaml is a second repository's; QUEUE.md is the
		// queue state's, which the next wf validate or status writes.
		return nil
	}
	return s.WriteSummary()
}

// validateMain checks main.yaml's values.
func (s *State) validateMain() []string {
	m := s.Main
	if m == nil {
		return nil
	}
	var out []string
	if m.CI != "" && !slices.Contains([]string{"green", "red", "pending"}, m.CI) {
		out = append(out, fmt.Sprintf("main.yaml: ci %q is not green, red or pending",
			m.CI))
	}
	if m.SHA != "" && !shaRe.MatchString(m.SHA) {
		out = append(out, fmt.Sprintf("main.yaml: sha %q is not a commit id", m.SHA))
	}
	if m.UpToDate != "" && m.UpToDate != "required" && m.UpToDate != "not-required" {
		out = append(out, fmt.Sprintf("main.yaml: up_to_date %q is not required or "+
			"not-required", m.UpToDate))
	}
	return out
}

// Load reads config.yaml, queue/*.yaml and, when it exists, queue/done/*.yaml
// under dir, into one list of items. An item in both places is an error.
func Load(dir string) (*State, error) {
	s := &State{Dir: dir}
	if err := readYAML(filepath.Join(dir, "config.yaml"), &s.Config); err != nil {
		return nil, err
	}
	if fi, err := os.Stat(filepath.Join(dir, doneFolder)); err == nil && fi.IsDir() {
		s.doneDir = true
	}
	if err := s.loadItems(); err != nil {
		return nil, err
	}
	s.sortFeatures()
	if err := s.loadNative(); err != nil {
		return nil, err
	}
	if err := s.loadAreas(); err != nil {
		return nil, err
	}
	if err := s.loadMain(); err != nil {
		return nil, err
	}
	return s, nil
}

// loadItems reads queue/*.yaml and queue/done/*.yaml.
func (s *State) loadItems() error {
	seen := map[string]bool{}
	for _, folder := range []string{"queue", doneFolder} {
		paths, err := filepath.Glob(filepath.Join(s.Dir, filepath.FromSlash(folder),
			"*.yaml"))
		if err != nil {
			return err
		}
		for _, p := range paths {
			f := &Feature{}
			if err := readYAML(p, f); err != nil {
				return err
			}
			want := strings.TrimSuffix(filepath.Base(p), ".yaml")
			if f.Feature != want {
				return fmt.Errorf("%s: feature %q does not match its file name",
					p, f.Feature)
			}
			if seen[f.Feature] {
				return fmt.Errorf("%s is in both queue/ and %s/: keep one",
					f.Feature, doneFolder)
			}
			seen[f.Feature] = true
			f.stored, f.file = f.Priority, folder+"/"+f.Feature+".yaml"
			s.Features = append(s.Features, f)
		}
	}
	return nil
}

// loadMain reads the optional main.yaml.
func (s *State) loadMain() error {
	p := filepath.Join(s.Dir, "main.yaml")
	if _, err := os.Stat(p); errors.Is(err, os.ErrNotExist) {
		return nil
	}
	s.Main = &Main{}
	return readYAML(p, s.Main)
}

// sortFeatures puts s.Features in FLOW-05 order, a stable starting order for
// code that walks them; readers that need the current order use Ordered.
func (s *State) sortFeatures() { sortFeatures(s.Features) }

// sortFeatures sorts by class, then in-progress before queued, then priority,
// then the later placement, then name; done items have class 0 and go last,
// in the order they closed, then by name. The key is total, so the order is
// the same however the items were listed (queue-positions, Decision 10).
func sortFeatures(fs []*Feature) {
	sort.SliceStable(fs, func(i, j int) bool {
		a, b := fs[i], fs[j]
		if c := compareGroup(a, b); c != 0 {
			return c < 0
		}
		if a.Status == "done" {
			if ta, tb := a.closedAt(), b.closedAt(); !ta.Equal(tb) {
				return ta.Before(tb)
			}
			return a.Feature < b.Feature
		}
		if a.Priority != b.Priority {
			return a.Priority < b.Priority
		}
		if ta, tb := a.placedAt(), b.placedAt(); !ta.Equal(tb) {
			return ta.After(tb)
		}
		return a.Feature < b.Feature
	})
}

// compareGroup compares the parts of FLOW-05's order that priority does not
// decide: the class, done last, then in progress before queued. It returns
// -1, 0 or 1 as a sorts before, with or after b.
func compareGroup(a, b *Feature) int {
	rank := func(f *Feature) int {
		c := 2 * f.Class()
		if c == 0 {
			c = 10
		}
		if f.Status != "in_progress" {
			c++
		}
		return c
	}
	return cmp.Compare(rank(a), rank(b))
}

// placedAt is when the item was last placed: its newest "queued at
// position", "moved to position" or, from before positions, "queued at
// priority" line. It is zero for an item never placed.
func (f *Feature) placedAt() time.Time {
	for i := len(f.History) - 1; i >= 0; i-- {
		e := f.History[i].Event
		if strings.HasPrefix(e, "queued at position") ||
			strings.HasPrefix(e, "moved to position") ||
			strings.HasPrefix(e, "queued at priority") {
			return f.History[i].At
		}
	}
	return time.Time{}
}

// closedAt is when a done item closed: the time of its last "done <step>"
// line, which Complete writes with "closed; feature done", or of its last
// line when it has none.
func (f *Feature) closedAt() time.Time {
	for i := len(f.History) - 1; i >= 0; i-- {
		if strings.HasPrefix(f.History[i].Event, "done ") {
			return f.History[i].At
		}
	}
	if len(f.History) > 0 {
		return f.History[len(f.History)-1].At
	}
	return time.Time{}
}

// Renumber sets each open item's priority to its position in FLOW-05's order,
// 1 for the first (FLOW-05). It never changes the order: after it the
// priorities follow the order, so sorting again gives the same list. Once
// queue/done/ exists a done item has no number. Until then a done item keeps
// a number clear of the open ones, the next after the highest held, so the
// numbers stay unique across every item as an old wf's Validate requires
// (queue-positions, Decision 8).
func (s *State) Renumber() {
	n := 0
	var done []*Feature
	for _, f := range s.Ordered() {
		if f.Status == "done" {
			done = append(done, f)
			continue
		}
		n++
		f.Priority = n
	}
	if s.doneDir {
		for _, f := range done {
			f.Priority = 0
		}
		return
	}
	high, held := n, map[int]bool{}
	var clash []*Feature
	for _, f := range done {
		if f.Priority <= n || held[f.Priority] {
			clash = append(clash, f)
			continue
		}
		held[f.Priority] = true
		high = max(high, f.Priority)
	}
	for _, f := range clash {
		high++
		f.Priority = high
	}
}

// place puts the item at the asked position as nearly as FLOW-05's order
// allows: after every open item whose class, or status within the class,
// sorts before the item's, and before every one whose sorts after it. It
// renumbers and returns the item's position.
func (s *State) place(f *Feature, asked int) int {
	var open []*Feature
	lo, hi := 0, 0
	for _, o := range s.Ordered() {
		if o == f || o.Status == "done" {
			continue
		}
		open = append(open, o)
		switch compareGroup(o, f) {
		case -1:
			lo++
			hi++
		case 0:
			hi++
		}
	}
	at := min(max(asked-1, lo), hi)
	open = slices.Insert(open, at, f)
	for i, o := range open {
		o.Priority = i + 1
	}
	s.Renumber()
	return at + 1
}

// askedNote is the history line's note of the position asked for, when the order
// placed the item elsewhere.
func askedNote(want, at int) string {
	if want == at {
		return ""
	}
	return fmt.Sprintf(" (asked %d)", want)
}

// Move is wf move: the human places an open item at a position, as nearly as
// its class and status allow, and the others are renumbered (FLOW-05,
// LOOP-04). The moved item gets the one history line.
func (s *State) Move(f *Feature, to int, why, by string, now time.Time) error {
	if !namedHuman(by) {
		return fmt.Errorf("%s may not move %s: only the human places an item, as "+
			"human/<name> (FLOW-05, LOOP-04)", by, f.Feature)
	}
	if f.Status == "done" {
		return fmt.Errorf("%s is done: a done item has no position", f.Feature)
	}
	if to < 1 {
		return fmt.Errorf("move -to %d: a position is 1 or more", to)
	}
	from := slices.Index(s.Listed(false), f) + 1
	at := s.place(f, to)
	line := fmt.Sprintf("moved to position %d from %d (asked %d)", at, from, to)
	if why != "" {
		line += ": " + why
	}
	f.Log(now, by, "%s", line)
	s.sortFeatures()
	return nil
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
// item, 3 a fix (a feature that follows another item) or an adoption (one
// that adopts a release, LOOP-07), 4 new functionality. A done item has no
// class, 0. A tracked item is classed as the feature it is.
func (f *Feature) Class() int {
	switch {
	case f.Status == "done":
		return 0
	case f.inImplementation():
		return 1
	case f.IsProcess():
		return 2
	case f.Follows != "", f.Adopts != nil:
		return 3
	}
	return 4
}

// inImplementation reports whether the item is in progress past Plan and not
// waiting for the human's merge: a merge check must not go ahead of process
// and fix work, and the merge is confirmed before wf next anyway. An item
// with a pending refresh step is, at its merge step too, so a run refreshes
// open PRs before it starts anything new (FLOW-05, FLOW-10). A tracked item
// never is: the loop takes none of its steps.
func (f *Feature) inImplementation() bool {
	if f.Status != "in_progress" || f.Round == "tracked" {
		return false
	}
	if f.PendingRefresh() != nil {
		return true
	}
	if f.IsProcess() {
		return f.Phase == 2 || f.Phase == 4
	}
	return f.Phase >= 3 && f.Phase != 7
}

// Add creates a queued feature, placed at the position its priority asks for
// as nearly as its class allows (place), and renumbers the others with no
// history line on them. An item homed in another repository gets the tracked
// round.
func (s *State) Add(f *Feature, now time.Time, by string) error {
	if _, err := s.Find(f.Feature); err == nil {
		return fmt.Errorf("feature %q already queued", f.Feature)
	}
	f.Status, f.Round, f.Phase = "queued", "feature", 0
	switch {
	case f.IsProcess():
		f.Round, f.Phase = "process", 1
	case s.Tracked(f):
		f.Round, f.Phase = "tracked", 1
	}
	Seed(f)
	want := f.Priority
	s.Features = append(s.Features, f)
	at := s.place(f, want)
	led := ""
	if f.Lead != "" {
		led = ", led by " + f.Lead
	}
	f.Log(now, by, "queued at position %d%s%s", at, askedNote(want, at), led)
	s.sortFeatures()
	return nil
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

// Save renumbers the open items (Renumber), writes the feature back to its
// queue file and the number of every other item whose number moved, and
// regenerates QUEUE.md, so the summary is committed with the change that
// produced it. Every command that changes state saves, so the state is in
// positions when it returns (queue-positions, Decision 1). A done item an old
// wf left in queue/ is not moved here: wf commit moves it.
func (s *State) Save(f *Feature) error {
	s.Renumber()
	if err := s.put(f); err != nil {
		return err
	}
	for _, o := range s.Features {
		if o == f || o.file == "" || o.Priority == o.stored ||
			(o.Status == "done" && s.doneDir) {
			continue
		}
		if err := s.writeNumber(o); err != nil {
			return err
		}
	}
	return s.WriteSummary()
}

// fileOf is where the item's file belongs: queue/done/ for a done item once
// that folder exists, queue/ otherwise.
func (s *State) fileOf(f *Feature) string {
	if f.Status == "done" && s.doneDir {
		return doneFolder + "/" + f.Feature + ".yaml"
	}
	return "queue/" + f.Feature + ".yaml"
}

// put writes the whole item where it belongs and removes its file from the
// other place.
func (s *State) put(f *Feature) error {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(f); err != nil {
		return err
	}
	rel := s.fileOf(f)
	if err := os.WriteFile(filepath.Join(s.Dir, filepath.FromSlash(rel)), buf.Bytes(),
		0o644); err != nil {
		return err
	}
	other := doneFolder + "/" + f.Feature + ".yaml"
	if rel == other {
		other = "queue/" + f.Feature + ".yaml"
	}
	err := os.Remove(filepath.Join(s.Dir, filepath.FromSlash(other)))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	f.file, f.stored = rel, f.Priority
	return nil
}

// priorityLine is an item file's top-level priority line.
var priorityLine = regexp.MustCompile(`(?m)^priority:.*\n?`)

// writeNumber rewrites only the priority line of the item's file, so that a
// renumber is a one-line change and a file another session or a hand edit
// wrote keeps the rest of its text. A file with no priority line is written
// whole.
func (s *State) writeNumber(f *Feature) error {
	p := filepath.Join(s.Dir, filepath.FromSlash(f.file))
	data, err := os.ReadFile(p)
	if err != nil {
		return err
	}
	loc := priorityLine.FindIndex(data)
	if loc == nil {
		return s.put(f)
	}
	line := ""
	if f.Priority != 0 {
		line = fmt.Sprintf("priority: %d\n", f.Priority)
	}
	data = slices.Concat(data[:loc[0]], []byte(line), data[loc[1]:])
	if err := os.WriteFile(p, data, 0o644); err != nil {
		return err
	}
	f.stored = f.Priority
	return nil
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

// inProgressLimit is how many features may be in progress: the value
// config.yaml writes, or FLOW-05's 5 when it writes none. A written value is
// returned as it is, one below 1 included, which Validate reports and under
// which Guard starts nothing (GLOBAL-10).
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

// Home is the item's home repository: its first repository, or the queue's
// own when it names none (LOOP-04).
func (s *State) Home(f *Feature) string {
	if len(f.Repos) > 0 {
		return f.Repos[0]
	}
	return s.Config.Repository
}

// Tracked reports whether the item's home has not adopted the contract: its
// config.yaml entry names no profile, so wf has no gate to run there and
// counts the item in no limit (FLOW-03, FLOW-05). An adopted home, whichever
// repository it is, gets the feature round (a6 of
// docs/design/loop-repository.md). With no queue repository set, the
// fallback Problems names: an item that names repos is tracked.
func (s *State) Tracked(f *Feature) bool {
	if s.Config.Repository == "" {
		return len(f.Repos) > 0
	}
	e := s.Config.Entry(s.Home(f))
	return e == nil || e.Profile == ""
}

// homedHere reports whether the item's home is the queue's own repository,
// where a process item is homed (a6).
func (s *State) homedHere(f *Feature) bool {
	return s.Config.Repository == "" || s.Home(f) == s.Config.Repository
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
	if m := s.Config.HostLeaseMinutes; m < 0 {
		errs = append(errs, fmt.Errorf("config: host_lease_minutes %d is below 0", m))
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
	return append(errs, s.validateRepositories()...)
}

// validateRepositories checks config.yaml's repositories: each entry names
// a repository once and gives its trunk, and the queue's own repository has
// one like any other (GLOBAL-11).
func (s *State) validateRepositories() []error {
	var errs []error
	seen := map[string]bool{}
	for i, e := range s.Config.Repositories {
		who := e.Name
		if !repoRe.MatchString(e.Name) {
			who = fmt.Sprintf("entry %d", i+1)
			errs = append(errs, fmt.Errorf("config: repositories %s: name %q is not "+
				"owner/name", who, e.Name))
		}
		if seen[e.Name] {
			errs = append(errs, fmt.Errorf("config: repositories names %s twice",
				e.Name))
		}
		seen[e.Name] = true
		if e.Trunk == "" {
			errs = append(errs, fmt.Errorf("config: repositories %s has no trunk",
				who))
		}
	}
	if r := s.Config.Repository; r != "" && !seen[r] {
		errs = append(errs, fmt.Errorf("config: repositories has no entry for %s, the "+
			"queue's own repository: an attended session adds it with its trunk", r))
	}
	return errs
}

// Validate checks the schema of every feature and the FLOW-05 limits. Each
// limit is the predicate Guard evaluates before a claim (validateLimits), so
// a state Guard allows has no limit finding here.
func (s *State) Validate() []error {
	errs := s.validateConfig()
	for _, f := range s.Features {
		if err := f.validatePosition(); err != nil {
			errs = append(errs, err)
		}
		errs = append(errs, s.Problems(f)...)
	}
	errs = append(errs, s.validateLimits()...)
	for _, e := range slices.Concat(s.validateNative(), s.validateAreas(), s.validateMain()) {
		errs = append(errs, errors.New(e))
	}
	return errs
}

// validatePosition checks the item's number against where its file is. A
// number that is not yet the item's position, or that another item holds, is
// not an error: Save and wf commit renumber it.
func (f *Feature) validatePosition() error {
	inDone := strings.HasPrefix(f.file, doneFolder+"/")
	switch {
	case inDone && f.Status != "done":
		return fmt.Errorf("%s: status %s in %s/, which holds done items only", f.Feature,
			f.Status, doneFolder)
	case inDone && f.Priority != 0:
		return fmt.Errorf("%s: a done item in %s/ has no priority, not %d", f.Feature,
			doneFolder, f.Priority)
	case f.Status != "done" && f.Priority < 1:
		return fmt.Errorf("%s: priority %d is below 1", f.Feature, f.Priority)
	}
	return nil
}

// Problems returns Validate's findings about one item's own fields, which wf
// new refuses to write.
func (s *State) Problems(f *Feature) []error {
	var errs []error
	bad := func(format string, a ...any) {
		errs = append(errs, fmt.Errorf("%s: %s", f.Feature, fmt.Sprintf(format, a...)))
	}
	s.validateFollows(f, bad)
	tracked := s.Tracked(f)
	if len(f.Repos) > 0 && s.Config.Repository == "" {
		errs = append(errs, fmt.Errorf(
			"config: repository is not set, and %s names repos", f.Feature))
		// Which repository is the queue's own is not known, so neither is
		// whether the item is tracked: its round is checked as written.
		tracked = f.Round == "tracked"
	}
	f.validate(bad, tracked, s.homedHere(f))
	s.validateEntered(f, bad)
	s.validateDeclared(f, bad)
	s.validateUrgent(f, bad)
	s.validateLead(f, bad)
	if f.PR != nil && f.PR.Head != "" && !commitRe.MatchString(f.PR.Head) {
		bad("pr.head %q is not a commit id", f.PR.Head)
	}
	validateWork(f, bad)
	return errs
}

// validateEntered checks that each repository the item names has a
// repositories entry. With no queue repository set, the fallback of
// Problems, entries are not read either.
func (s *State) validateEntered(f *Feature, bad func(string, ...any)) {
	if s.Config.Repository == "" {
		return
	}
	for i, r := range f.Repos {
		if repoRe.MatchString(r) && s.Config.Entry(r) == nil &&
			!slices.Contains(f.Repos[:i], r) {
			bad("repos names %s, which has no entry in config.yaml's repositories: "+
				"an attended session adds it with its trunk", r)
		}
	}
}

// validateWork checks that work, once written, has one entry for each
// repository in repos after the home, in its order, on the home's branch
// name (FLOW-03, FLOW-17).
func validateWork(f *Feature, bad func(string, ...any)) {
	if len(f.Work) == 0 {
		return
	}
	var want, got []string
	if len(f.Repos) > 1 {
		want = f.Repos[1:]
	}
	for _, w := range f.Work {
		got = append(got, w.Repo)
		if w.Branch != f.Branch {
			bad("work %s: branch %q is not the item's branch %q", w.Repo, w.Branch,
				f.Branch)
		}
		if w.PR != nil && w.PR.Head != "" && !commitRe.MatchString(w.PR.Head) {
			bad("work %s: pr.head %q is not a commit id", w.Repo, w.PR.Head)
		}
		if w.Merged != "" && !commitRe.MatchString(w.Merged) {
			bad("work %s: merged %q is not a commit id", w.Repo, w.Merged)
		}
	}
	if !slices.Equal(got, want) {
		bad("work names %v, but repos after the home are %v: wf declare writes work "+
			"from the design", got, want)
	}
}

// Inherit gives a new item that follows parent the parent's repositories and
// areas: a fix or finish of an item is in that item's areas to a first
// approximation, and the human agreed those (FLOW-09). It is for an item
// filed with no areas of its own; ownRepos says it was given its
// repositories, which are then kept. It returns the history line, which says
// what was taken.
func (f *Feature) Inherit(parent *Feature, ownRepos bool) string {
	what, none := "repos and areas", parent.Areas == nil
	if ownRepos {
		what = "areas"
	} else {
		f.Repos = slices.Clone(parent.Repos)
		none = none && len(parent.Repos) == 0
	}
	if parent.Areas != nil {
		areas := slices.Clone(*parent.Areas)
		f.Areas = &areas
	}
	line := what + " taken from " + parent.Feature
	if none {
		line += ": it has none"
	}
	return line
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
	case !namedHuman(f.Urgent):
		bad("urgent names %q: %s", f.Urgent, urgentUnnamed)
	case f.IsProcess():
		bad("urgent on a process item: %s one (FLOW-05)", never)
	case s.Tracked(f):
		bad("urgent on a tracked item: %s one (FLOW-05)", never)
	case f.Follows != "":
		bad("urgent, and it follows %s: %s a fix (FLOW-05)", f.Follows, never)
	}
}

func (f *Feature) validate(bad func(string, ...any), tracked, homedHere bool) {
	if !oneOf(f.Status, "queued", "in_progress", "done") {
		bad("status %q is not queued, in_progress or done", f.Status)
	}
	f.validateKind(bad, tracked, homedHere)
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
func (f *Feature) validateKind(bad func(string, ...any), tracked, homedHere bool) {
	switch {
	case !oneOf(f.Kind, "", "feature", "process"):
		bad("kind %q is not feature or process", f.Kind)
	case f.IsProcess() && !homedHere:
		bad("a process item is homed in the queue's repository, not %s", f.Repos[0])
	case f.IsProcess():
		f.validateProcess(bad)
	default:
		legacy := f.Status == "done" && oneOf(f.Round, "parallel", "merge")
		switch {
		case tracked && f.Round != "tracked":
			bad("a tracked item's round is tracked, not %q", f.Round)
		case !tracked && f.Round == "tracked":
			bad("round tracked is for an item whose home has not adopted the " +
				"contract (no profile in config.yaml), and this one's has")
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
