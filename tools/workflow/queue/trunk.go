package queue

import (
	"fmt"
	"slices"
	"strings"
	"time"
)

// machineryProbes are paths that stand for the loop's machinery and the
// repository's CI and release configuration: an item whose declared areas
// hold any of them keeps the trunk (FLOW-20). Each is looked up in the
// repository's area map, so the map decides which area holds it.
var machineryProbes = []string{".claude/settings.json", "tools/workflow/go.mod",
	".github/workflows/main.yml", ".goreleaser.yaml"}

// ProgramTrunkOf returns the program trunk config.yaml records for repo, or
// "" when its entry has none (FLOW-20).
func (c Config) ProgramTrunkOf(repo string) string {
	if e := c.Entry(repo); e != nil {
		return e.ProgramTrunk
	}
	return ""
}

// HasProgramTrunk reports whether any repository entry records a program
// trunk: FLOW-20 stands, and a process item's design takes the process test
// or the human's review.
func (c Config) HasProgramTrunk() bool {
	return slices.ContainsFunc(c.Repositories, func(e RepositoryEntry) bool {
		return e.ProgramTrunk != ""
	})
}

// recordedTrunk returns the trunk the item recorded for repo, at Branch setup
// or at its first wf declare, or "". A recorded program trunk that
// config.yaml no longer names is not read: with program_trunk taken off an
// entry, every item there is back on the trunk (the design's rollback).
func (s *State) recordedTrunk(repo string, f *Feature) string {
	t := f.Trunk
	if repo != s.ReposOf(f)[0] {
		t = ""
		if w := s.workEntry(f, repo); w != nil {
			t = w.Trunk
		}
	}
	if t != "" && t == s.Config.ProgramTrunkOf(repo) {
		return t
	}
	if trunk, err := s.Config.Trunk(repo); err == nil && t == trunk {
		return t
	}
	return ""
}

// TrunkFor is the branch the item works against in repo: the trunk the item
// recorded there, once it has; else the repository's program trunk for a
// program item that is not a process item and whose declared areas reach no
// loop machinery, .github/ or .goreleaser.yaml path, in a repository whose
// entry has one; else the repository's trunk (FLOW-20). Config.Trunk stays
// the only reader of trunk, so the profile is still compared with it
// (LOOP-01).
func (s *State) TrunkFor(repo string, f *Feature) (string, error) {
	if t := s.recordedTrunk(repo, f); t != "" {
		return t, nil
	}
	return s.chooseTrunk(repo, f)
}

// chooseTrunk is TrunkFor before anything is recorded.
func (s *State) chooseTrunk(repo string, f *Feature) (string, error) {
	trunk, err := s.Config.Trunk(repo)
	if err != nil {
		return "", err
	}
	pt := s.Config.ProgramTrunkOf(repo)
	if pt == "" || !f.Program || f.IsProcess() || s.reachesMachinery(f) {
		return trunk, nil
	}
	return pt, nil
}

// reachesMachinery reports whether the item's declared areas hold a path of
// the loop's machinery or of CI or release configuration, as the map of the
// item's home, whose names its areas are, places them. An undeclared item
// reaches none it has declared; one that declares areas where its home has
// no map is taken to reach them, so the program trunk is never chosen on a
// guess.
func (s *State) reachesMachinery(f *Feature) bool {
	if f.Areas == nil || len(*f.Areas) == 0 {
		return false
	}
	v, err := s.ForRepository(s.ReposOf(f)[0])
	if err != nil || v.Areas == nil {
		return true
	}
	for _, p := range machineryProbes {
		if a, err := v.Areas.AreaOf(p); err == nil && slices.Contains(*f.Areas, a) {
			return true
		}
	}
	return false
}

// OnProgramTrunk reports whether the item works against a program trunk in
// repo.
func (s *State) OnProgramTrunk(repo string, f *Feature) bool {
	t, err := s.TrunkFor(repo, f)
	return err == nil && t != "" && t == s.Config.ProgramTrunkOf(repo)
}

// PinTrunks records the item's trunk in each repository it has a branch in
// whose entry has a program trunk and that has none recorded yet, as
// TrunkFor chooses it, and logs each it records. Branch setup and the first
// wf declare call it, so a branch forked from one trunk is never checked
// against another (FLOW-20). Where there is no program trunk there is no
// choice to pin.
func (s *State) PinTrunks(f *Feature, by string, now time.Time) error {
	if f.Round != "feature" {
		return nil
	}
	for i, repo := range s.ReposOf(f) {
		if s.Config.ProgramTrunkOf(repo) == "" || s.recordedTrunk(repo, f) != "" ||
			(i > 0 && s.workEntry(f, repo) == nil) {
			continue
		}
		t, err := s.chooseTrunk(repo, f)
		if err != nil {
			return err
		}
		if i == 0 {
			f.Trunk = t
		} else {
			s.workEntry(f, repo).Trunk = t
		}
		f.Log(now, by, "trunk in %s: %s (FLOW-20)", repo, t)
	}
	return nil
}

func (s *State) workEntry(f *Feature, repo string) *Work {
	for i := range f.Work {
		if f.Work[i].Repo == repo {
			return &f.Work[i]
		}
	}
	return nil
}

// trunkFlips returns, for each repository where the item has recorded a
// trunk, why its current declaration would choose another, or "". The item
// goes back to Design to be split instead (FLOW-06, FLOW-20).
func (s *State) trunkFlips(f *Feature) string {
	var out []string
	for _, repo := range s.ReposOf(f) {
		was := s.recordedTrunk(repo, f)
		if was == "" {
			continue
		}
		now, err := s.chooseTrunk(repo, f)
		if err == nil && now != was {
			out = append(out, fmt.Sprintf("%s: recorded %s, would now be %s", repo,
				was, now))
		}
	}
	return strings.Join(out, "; ")
}
