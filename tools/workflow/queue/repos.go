package queue

import (
	"fmt"
	"os"
	"path/filepath"
)

// reposDir holds the state of each repository other than the queue's own:
// repos/<owner>/<name>/areas.yaml and main.yaml, beside the root's two
// files, which stay the queue repository's (Decision 8 of
// docs/design/loop-repository.md).
const reposDir = "repos"

// ForRepository returns the state as wf main, wf areas and the refresh's
// red-trunk hold read it for one repository: the queue state itself for the
// queue's own repository, and for another a view whose area map and main.yaml
// are that repository's, under repos/<owner>/<name>/, and whose
// Config.Repository is that repository. Writing the view's main.yaml writes
// that repository's. A repository with no config.yaml entry is an error.
func (s *State) ForRepository(repo string) (*State, error) {
	if repo == "" || repo == s.Config.Repository {
		return s, nil
	}
	if s.Config.Entry(repo) == nil {
		return nil, fmt.Errorf("config.yaml's repositories has no entry for %s: an "+
			"attended session adds it with its trunk", repo)
	}
	if s.repos == nil {
		s.repos = map[string]*State{}
	}
	if v := s.repos[repo]; v != nil {
		return v, nil
	}
	v := &State{Dir: filepath.Join(s.Dir, reposDir, filepath.FromSlash(repo)),
		Config: s.Config, Features: s.Features, Machines: s.Machines, doneDir: s.doneDir,
		summaryDir: s.Dir}
	v.Config.Repository, v.queueRepo = repo, s.Config.Repository
	if err := v.loadAreas(); err != nil {
		return nil, err
	}
	if err := v.loadMain(); err != nil {
		return nil, err
	}
	s.repos[repo] = v
	return v, nil
}

// MainOf is what main.yaml records for the repository's trunk, nil when
// nothing is recorded or the repository has no entry.
func (s *State) MainOf(repo string) *Main {
	v, err := s.ForRepository(repo)
	if err != nil {
		return nil
	}
	return v.Main
}

// mkdir makes the view's directory before a file is written there.
func (s *State) mkdir() error {
	return os.MkdirAll(s.Dir, 0o755)
}
