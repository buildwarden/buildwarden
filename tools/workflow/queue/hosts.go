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
	"sort"
	"strings"
	"time"
)

// Machine is one native machine in hosts.yaml: the host names (FLOW-04) its
// scheduled native session serves. Each host name is served by one machine,
// so routing a capability (say, qemu-any) to a machine is a state edit.
type Machine struct {
	Name  string   `yaml:"name"`
	Hosts []string `yaml:"hosts"`
	Note  string   `yaml:"note,omitempty"`
}

// Gap is one entry of backlog/native-gaps.yaml (FLOW-09, TEST-03).
type Gap struct {
	ID     string `yaml:"id"`
	Host   string `yaml:"host"`
	What   string `yaml:"what"`
	Design string `yaml:"design"`
	Note   string `yaml:"note,omitempty"`
	Status string `yaml:"status"` // open | done
}

// loadNative reads the optional hosts.yaml and backlog/native-gaps.yaml.
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
	return nil
}

func readOptional(path string, v any) error {
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return readYAML(path, v)
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

// validateNative checks that each host name has one machine and that every
// host step and open gap names a host some machine serves.
func (s *State) validateNative() []string {
	if len(s.Machines) == 0 {
		return nil
	}
	var errs []string
	bad := func(format string, a ...any) { errs = append(errs, fmt.Sprintf(format, a...)) }
	servedBy := map[string]string{}
	for _, m := range s.Machines {
		for _, h := range m.Hosts {
			if !affinityRe.MatchString("host:" + h) {
				bad("hosts.yaml: %s: bad host name %q", m.Name, h)
			}
			if o, dup := servedBy[h]; dup {
				bad("hosts.yaml: host %s served by both %s and %s", h, o, m.Name)
			}
			servedBy[h] = m.Name
		}
	}
	for _, f := range s.Features {
		for _, st := range f.Steps {
			h, ok := strings.CutPrefix(st.Affinity, "host:")
			if ok && st.Status == "pending" && servedBy[h] == "" {
				bad("%s: step %s: no machine serves host %s", f.Feature, st.ID, h)
			}
		}
	}
	for _, g := range s.Gaps {
		if g.Status == "open" && servedBy[g.Host] == "" {
			bad("backlog: gap %s: no machine serves host %s", g.ID, g.Host)
		}
	}
	return errs
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
func (s *State) Gate(m *Machine, repo string, last *GateStamp, sweepDays int,
	now time.Time) (*Gate, error) {
	g := &Gate{}
	for _, h := range m.Hosts {
		if p, _ := s.Next("host:"+h+"/gate", now); p != nil {
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
	print, err := s.fingerprint(repo)
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

// fingerprint hashes origin/main and the branch of every in-progress feature,
// as last fetched into repo.
func (s *State) fingerprint(repo string) (string, error) {
	refs := []string{"refs/remotes/origin/main"}
	for _, f := range s.Features {
		if f.Status == "in_progress" && f.Branch != "" {
			refs = append(refs, "refs/remotes/origin/"+f.Branch)
		}
	}
	sort.Strings(refs[1:])
	h := sha256.New()
	for _, r := range refs {
		out, err := exec.Command("git", "-C", repo, "rev-parse", "-q", "--verify",
			r+"^{commit}").Output()
		sha := strings.TrimSpace(string(out))
		if err != nil {
			sha = "absent"
		}
		fmt.Fprintf(h, "%s %s\n", r, sha)
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
