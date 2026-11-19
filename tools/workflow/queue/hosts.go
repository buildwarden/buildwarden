package queue

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"
)

// Machine is one native machine in hosts.yaml: the host names (FLOW-04) its
// scheduled native session serves. A host name may be listed on several
// machines, and a step of it goes to whichever serving worker next takes
// steps; routing a capability (say, qemu-any) to a machine is a state edit.
type Machine struct {
	Name  string   `yaml:"name"`
	Hosts []string `yaml:"hosts"`
	// Repositories are those whose host steps the machine takes, each with a
	// clone its owner made (LOOP-05). Absent, the queue's own only.
	Repositories []string `yaml:"repositories,omitempty"`
	Note         string   `yaml:"note,omitempty"`
}

// lists returns the repositories the machine takes steps in: its
// repositories, or the queue's own when it lists none (FLOW-04).
func (s *State) lists(m *Machine) []string {
	if len(m.Repositories) > 0 {
		return m.Repositories
	}
	return []string{s.Config.Repository}
}

// stepRepos are the repositories a step runs its work in: every repository
// the item works in for a led step, which runs the gates in each, and
// otherwise the step's repo or the item's home (FLOW-04).
func (s *State) stepRepos(f *Feature, st *Step) []string {
	if aff, ok := templateAffinity(st); ok && aff == "cloud" && f.Lead != "" &&
		st.Affinity == f.Lead {
		return s.ReposOf(f)
	}
	if st.Repo != "" {
		return []string{st.Repo}
	}
	return []string{s.Home(f)}
}

// missingOn returns the repositories of repos the machine does not list.
func (s *State) missingOn(m *Machine, repos []string) []string {
	return minus(repos, s.lists(m))
}

// takers returns the machines that serve the host name and list every
// repository, and the names of all machines that serve it.
func (s *State) takers(host string, repos []string) (ok, serving []string) {
	for i := range s.Machines {
		m := &s.Machines[i]
		if !slices.Contains(m.Hosts, host) {
			continue
		}
		serving = append(serving, m.Name)
		if len(s.missingOn(m, repos)) == 0 {
			ok = append(ok, m.Name)
		}
	}
	return ok, serving
}

// unlisted is the finding for a host name whose serving machines none list
// every repository: each machine with the repositories missing from it.
func (s *State) unlisted(host string, repos []string) string {
	var each []string
	for i := range s.Machines {
		m := &s.Machines[i]
		if slices.Contains(m.Hosts, host) {
			each = append(each, fmt.Sprintf("%s lacks %s", m.Name,
				strings.Join(s.missingOn(m, repos), ", ")))
		}
	}
	return fmt.Sprintf("no machine serving host %s lists %s in hosts.yaml (%s): Jeff "+
		"adds them to one machine's repositories (FLOW-04)", host,
		strings.Join(repos, ", "), strings.Join(each, "; "))
}

// MayLeadOn returns why no machine can take the lead's steps for the item:
// none serving it lists every repository in the item's repos (FLOW-04). A
// state with no hosts.yaml routes nothing and refuses nothing.
func (s *State) MayLeadOn(f *Feature, lead string) error {
	h, ok := strings.CutPrefix(lead, "host:")
	if !ok || len(s.Machines) == 0 {
		return nil
	}
	if took, serving := s.takers(h, s.ReposOf(f)); len(serving) > 0 && len(took) == 0 {
		return fmt.Errorf("%s: lead %s: %s, or a worker clears the lead with wf lead "+
			"-clear", f.Feature, lead, s.unlisted(h, s.ReposOf(f)))
	}
	return nil
}

// machineRefuses says why the machine a host worker's session names may not
// take the step, which runs in a repository the machine does not list, or
// "". A session that names no machine of hosts.yaml is not routed by it.
func (s *State) machineRefuses(f *Feature, st *Step, worker string) string {
	name, _, _ := strings.Cut(Session(worker), ".")
	if !strings.HasPrefix(Kind(worker), "host:") || name == "" {
		return ""
	}
	m, err := s.Machine(name)
	if err != nil {
		return ""
	}
	if gone := s.missingOn(m, s.stepRepos(f, st)); len(gone) > 0 {
		return fmt.Sprintf("step %s runs in %s, which %s does not list in hosts.yaml "+
			"(FLOW-04)", st.ID, strings.Join(gone, ", "), m.Name)
	}
	return ""
}

// Gap is one entry of backlog/native-gaps.yaml (FLOW-09, TEST-03).
type Gap struct {
	ID     string `yaml:"id"`
	Host   string `yaml:"host"`
	What   string `yaml:"what"`
	Design string `yaml:"design"`
	Note   string `yaml:"note,omitempty"`
	Status string `yaml:"status"` // open | deferred | done
}

// MachineStatus is native/status/<machine>.yaml, written by native-tick.sh
// on every scheduled run so a failure before the agent starts is still seen.
type MachineStatus struct {
	File    string    `yaml:"-"`
	Machine string    `yaml:"machine"`
	LastRun time.Time `yaml:"last_run"`
	Outcome string    `yaml:"outcome"` // worked | idle | skipped | failed
	Reason  string    `yaml:"reason"`  // one line, never log contents (FLOW-18)
	Since   time.Time `yaml:"since"`
}

// loadNative reads the optional hosts.yaml, backlog/native-gaps.yaml and
// native/status/*.yaml.
func (s *State) loadNative() error {
	var hosts struct {
		Machines []Machine `yaml:"machines"`
	}
	if err := readOptional(filepath.Join(s.Dir, "hosts.yaml"), &hosts); err != nil {
		return err
	}
	s.Machines = hosts.Machines
	var gaps struct {
		Gaps []Gap `yaml:"gaps"`
	}
	gapsPath := filepath.Join(s.Dir, "backlog", "native-gaps.yaml")
	if err := readOptional(gapsPath, &gaps); err != nil {
		return err
	}
	s.Gaps = gaps.Gaps
	paths, err := filepath.Glob(filepath.Join(s.Dir, "native", "status", "*.yaml"))
	if err != nil {
		return err
	}
	for _, p := range paths {
		st := MachineStatus{File: filepath.Base(p)}
		if err := readYAML(p, &st); err != nil {
			return err
		}
		s.Statuses = append(s.Statuses, st)
	}
	return nil
}

// validateStatus checks the shape of the machine status files.
func (s *State) validateStatus() []string {
	var errs []string
	for _, st := range s.Statuses {
		bad := func(format string, a ...any) {
			errs = append(errs, fmt.Sprintf("native/status/%s: ", st.File)+
				fmt.Sprintf(format, a...))
		}
		if st.Machine+".yaml" != st.File {
			bad("machine %q does not match the file name", st.Machine)
		}
		if len(s.Machines) > 0 {
			if _, err := s.Machine(st.Machine); err != nil {
				bad("machine %q is not in hosts.yaml", st.Machine)
			}
		}
		if !oneOf(st.Outcome, "worked", "idle", "skipped", "failed") {
			bad("outcome %q is not worked, idle, skipped or failed", st.Outcome)
		}
		if r := st.Reason; r == "" || strings.ContainsAny(r, "\r\n") || len(r) > 120 {
			bad("reason must be one line of 1 to 120 characters")
		}
		if st.LastRun.IsZero() || st.Since.IsZero() || st.Since.After(st.LastRun) {
			bad("last_run and since must be set, and since not after last_run")
		}
	}
	return errs
}

func readOptional(path string, v any) error {
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return readYAML(path, v)
}

// served returns the host names some machine in hosts.yaml serves.
func (s *State) served() map[string]bool {
	out := map[string]bool{}
	for _, m := range s.Machines {
		for _, h := range m.Hosts {
			out[h] = true
		}
	}
	return out
}

// Machine returns the named machine from hosts.yaml.
func (s *State) Machine(name string) (*Machine, error) {
	for i := range s.Machines {
		if s.Machines[i].Name == name {
			return &s.Machines[i], nil
		}
	}
	return nil, fmt.Errorf("no machine %q in %s/hosts.yaml", name, s.Dir)
}

// validateGaps checks each backlog gap's status, and that a deferred gap has
// a note saying what it waits on: nothing else wakes a session for it.
func (s *State) validateGaps() []string {
	var errs []string
	for _, g := range s.Gaps {
		bad := func(format string, a ...any) {
			errs = append(errs, "backlog: gap "+g.ID+": "+fmt.Sprintf(format, a...))
		}
		if !oneOf(g.Status, "open", "deferred", "done") {
			bad("status %q is not open, deferred or done", g.Status)
		}
		if g.Status == "deferred" && strings.TrimSpace(g.Note) == "" {
			bad("deferred needs a note saying what it waits on")
		}
	}
	return errs
}

// validateNative checks the gaps, the host names, and that every pending host
// step, open gap and open item's lead names a host some machine serves. A
// name several machines list is valid (FLOW-04).
func (s *State) validateNative() []string {
	errs := append(s.validateStatus(), s.validateGaps()...)
	if len(s.Machines) == 0 {
		return errs
	}
	bad := func(format string, a ...any) { errs = append(errs, fmt.Sprintf(format, a...)) }
	for _, m := range s.Machines {
		for _, h := range m.Hosts {
			if !affinityRe.MatchString("host:" + h) {
				bad("hosts.yaml: %s: bad host name %q", m.Name, h)
			}
		}
	}
	served := s.served()
	for _, f := range s.Features {
		f.validateServed(served, bad)
		s.validateListed(f, served, bad)
	}
	for _, g := range s.Gaps {
		if g.Status == "open" && !served[g.Host] {
			bad("backlog: gap %s: no machine serves host %s", g.ID, g.Host)
		}
	}
	return errs
}

// validateServed checks that each pending host step of the item, and its
// lead while it is open, names a host some machine serves.
func (f *Feature) validateServed(served map[string]bool, bad func(string, ...any)) {
	for _, st := range f.Steps {
		h, ok := strings.CutPrefix(st.Affinity, "host:")
		if ok && st.Status == "pending" && !served[h] {
			bad("%s: step %s: no machine serves host %s", f.Feature, st.ID, h)
		}
	}
	if h, ok := strings.CutPrefix(f.Lead, "host:"); ok && f.Status != "done" && !served[h] {
		bad("%s: lead: no machine serves host %s", f.Feature, h)
	}
}

// validateListed checks that each pending host step of the item, and its
// lead while it is open, has a machine that serves its host name and lists
// the repositories it runs in (FLOW-04). A name no machine serves is
// validateServed's finding.
func (s *State) validateListed(f *Feature, served map[string]bool,
	bad func(string, ...any),
) {
	for i := range f.Steps {
		st := &f.Steps[i]
		h, ok := strings.CutPrefix(st.Affinity, "host:")
		aff, _ := templateAffinity(st)
		// A led step's finding is the lead's, below.
		if !ok || st.Status != "pending" || !served[h] ||
			(aff == "cloud" && st.Affinity == f.Lead) {
			continue
		}
		if took, _ := s.takers(h, s.stepRepos(f, st)); len(took) == 0 {
			bad("%s: step %s: %s", f.Feature, st.ID, s.unlisted(h, s.stepRepos(f, st)))
		}
	}
	if h, ok := strings.CutPrefix(f.Lead, "host:"); ok && f.Status != "done" && served[h] {
		if took, _ := s.takers(h, s.ReposOf(f)); len(took) == 0 {
			bad("%s: lead: %s", f.Feature, s.unlisted(h, s.ReposOf(f)))
		}
	}
}

// Gate is the scheduled native session's go/no-go, decided without an agent so
// an idle night costs nothing.
type Gate struct {
	Run     bool
	Reasons []string // why the session should run, or why it is idle
	Print   string   // fingerprint of main and the waiting branches
}

// GateStamp is what the last full native session on a machine saw.
type GateStamp struct {
	Print string    `json:"fingerprint"`
	At    time.Time `json:"at"`
}

// Gate decides whether machine m has native work: a ready step for one of its
// hosts, or open backlog gaps on its hosts when main or a waiting branch has
// moved since the last session (stamp), or when sweepDays have passed since it.
// While the human has not confirmed the priority order it is idle, with that
// reason, as the cloud is (FLOW-13).
func (s *State) Gate(m *Machine, repo string, last *GateStamp, sweepDays int,
	now time.Time) (*Gate, error) {
	if !s.Config.PriorityConfirmed {
		return &Gate{Reasons: []string{
			"idle\tthe priority order is not confirmed (FLOW-13)"}}, nil
	}
	g := &Gate{}
	for _, h := range m.Hosts {
		if p, _ := s.Next("host:"+h+"/"+m.Name+".gate", now); p != nil {
			g.Run = true
			g.Reasons = append(g.Reasons, fmt.Sprintf("step\thost:%s\t%s\t%s", h,
				p.Feature.Feature, p.Step.ID))
		}
	}
	serves := s.unheld(m, g)
	var open []string
	for _, gap := range s.Gaps {
		if gap.Status == "open" && serves[gap.Host] {
			open = append(open, gap.ID)
		}
	}
	print, err := s.fingerprint(m, repo)
	if err != nil {
		return nil, err
	}
	g.Print = print
	switch {
	case len(open) == 0:
	case last == nil:
		g.Run = true
		g.Reasons = append(g.Reasons, "gaps\tfirst sweep\t"+strings.Join(open, ","))
	case last.Print != print:
		g.Run = true
		g.Reasons = append(g.Reasons, "gaps\tmain or a waiting branch moved\t"+
			strings.Join(open, ","))
	case sweepDays > 0 && now.Sub(last.At) >= time.Duration(sweepDays)*24*time.Hour:
		g.Run = true
		g.Reasons = append(g.Reasons, fmt.Sprintf("gaps\tno sweep in %d days\t%s",
			sweepDays, strings.Join(open, ",")))
	default:
		g.Reasons = append(g.Reasons, fmt.Sprintf(
			"idle\t%d open gaps, nothing moved since %s", len(open),
			last.At.Format(time.RFC3339)))
	}
	if !g.Run && len(g.Reasons) == 0 {
		g.Reasons = append(g.Reasons, "idle\tno host step ready and no open gaps")
	}
	return g, nil
}

// unheld returns the machine's hosts that no in-progress process item holds;
// a held host starts no backlog sweep, and the gate says why.
func (s *State) unheld(m *Machine, g *Gate) map[string]bool {
	serves := map[string]bool{}
	for _, h := range m.Hosts {
		if by := s.Hold(nil, "host:"+h); by != "" {
			g.Reasons = append(g.Reasons, fmt.Sprintf(
				"held\thost:%s\tby process item %s", h, by))
			continue
		}
		serves[h] = true
	}
	return serves
}

// fingerprint hashes, for each repository the machine lists, its trunk and
// the branch there of every in-progress feature that works in it, as last
// fetched: the queue's own in repo, the machine's dedicated checkout, and
// each other in its clone at repo/.dev/checkouts/<owner>/<name> (a14 of
// docs/design/loop-repository.md). A repository with no entry is an error.
func (s *State) fingerprint(m *Machine, repo string) (string, error) {
	h := sha256.New()
	for _, name := range s.lists(m) {
		dir, label := repo, ""
		trunk := "main"
		if name != "" {
			t, err := s.Config.Trunk(name)
			if err != nil && s.Config.Repository != "" {
				return "", err
			}
			if t != "" {
				trunk = t
			}
		}
		if name != s.Config.Repository {
			dir = filepath.Join(repo, ".dev", "checkouts", filepath.FromSlash(name))
			label = name + " "
		}
		refs := []string{"refs/remotes/origin/" + trunk}
		for _, f := range s.Features {
			if f.Status == "in_progress" && f.Branch != "" &&
				slices.Contains(s.ReposOf(f), name) {
				refs = append(refs, "refs/remotes/origin/"+f.Branch)
			}
		}
		sort.Strings(refs[1:])
		for _, r := range refs {
			out, err := exec.Command("git", "-C", dir, "rev-parse", "-q", "--verify",
				r+"^{commit}").Output()
			sha := strings.TrimSpace(string(out))
			if err != nil {
				sha = "absent"
			}
			fmt.Fprintf(h, "%s%s %s\n", label, r, sha)
		}
	}
	return hex.EncodeToString(h.Sum(nil))[:16], nil
}

// ReadStamp reads a gate stamp; a missing file is no stamp.
func ReadStamp(path string) (*GateStamp, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	st := &GateStamp{}
	if err := json.Unmarshal(data, st); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return st, nil
}

// WriteStamp records what a finished native session saw.
func WriteStamp(path string, st GateStamp) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, _ := json.Marshal(st)
	return os.WriteFile(path, append(data, '\n'), 0o644)
}
