package queue

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// processRenames maps a process item's old step names to the new ones.
var processRenames = map[string]string{"sign-off": "merge", "archive": "close"}

// Migrate moves every unfinished item onto the single feature round and the
// new process phases (workflow-streamline). A parallel-round feature keeps
// its phase number; its pending steps are renamed to f<phase>-<name>, and a
// feature waiting on the old merge queue is reseeded at Publish. A feature
// in Implement whose design was never reviewed gets a design review before
// Implement continues (FLOW-19). Done steps keep their ids as history. It
// returns the items it changed, and refuses an in-progress merge-round item,
// whose steps have no single-round equivalent.
func (s *State) Migrate(by string, now time.Time) ([]*Feature, error) {
	var changed []*Feature
	for _, f := range s.Features {
		if f.Status == "done" {
			continue
		}
		switch f.Round {
		case "parallel":
			f.migrateParallel()
		case "process":
			if !f.migrateProcess() {
				continue
			}
		case "merge":
			return nil, fmt.Errorf("%s is in the merge round (phase %d): finish it "+
				"under the old steps before migrating", f.Feature, f.Phase)
		default:
			continue
		}
		f.Log(now, by, "migrated to the %s round at phase %d (workflow-streamline)",
			f.Round, f.Phase)
		changed = append(changed, f)
	}
	return changed, nil
}

func (f *Feature) migrateParallel() {
	f.Round = "feature"
	renames := map[string]string{}
	kept := f.Steps[:0]
	for _, st := range f.Steps {
		if st.Status == "pending" && f.Phase == 6 && st.Phase == 6 &&
			strings.HasPrefix(st.ID, "p6-") {
			continue // the old publish and merge-round steps; reseeded below
		}
		if st.Status == "pending" && strings.HasPrefix(st.ID, "p") {
			renames[st.ID] = "f" + st.ID[1:]
			st.ID = renames[st.ID]
		}
		kept = append(kept, st)
	}
	f.Steps = kept
	for i := range f.Steps {
		for j, n := range f.Steps[i].Needs {
			if r, ok := renames[n]; ok {
				f.Steps[i].Needs[j] = r
			}
		}
	}
	switch {
	case f.Phase == 6:
		Seed(f)
	case f.Phase == 3 && f.DesignReview == "":
		f.addReviewBeforeImplement()
	}
}

// addReviewBeforeImplement gives a feature already in Implement the design
// review it never had: a human step every pending implement step needs.
func (f *Feature) addReviewBeforeImplement() {
	id := StepID(f.Round, 3, "design-review")
	for i := range f.Steps {
		st := &f.Steps[i]
		if st.Status == "pending" && st.Phase == 3 {
			st.Needs = append(st.Needs, id)
		}
	}
	f.Steps = append(f.Steps, Step{ID: id, Phase: 3, Affinity: "human", Status: "pending",
		Note: "Design review (FLOW-19) for a design that predates it; the human may " +
			"waive it by completing this step with a recorded reason."})
}

// migrateProcess renames sign-off and archive to merge and close and gives
// Implement and later to the cloud. It reports whether anything changed.
func (f *Feature) migrateProcess() bool {
	moved := false
	for i := range f.Steps {
		st := &f.Steps[i]
		if st.Status != "pending" || st.Phase < 2 {
			continue
		}
		parts := strings.SplitN(st.ID, "-", 2)
		if r, ok := processRenames[parts[1]]; ok {
			st.ID = parts[0] + "-" + r
		}
		st.Affinity = "cloud"
		moved = true
	}
	return moved
}

// MigrateDone is wf migrate -done, run once at queue-positions' x4-state: it
// creates queue/done/, moves every done item into it without its number, and
// renumbers the open items (queue-positions, Decision 7). It writes the files
// and returns the items it moved. The move is the layout's, so it logs no
// history line. It refuses a worker that is not a named human, and a second
// run.
func (s *State) MigrateDone(by string) ([]*Feature, error) {
	if !namedHuman(by) {
		return nil, fmt.Errorf("migrate -done is run by a human worker, human/<name>, "+
			"not %s", by)
	}
	dir := filepath.Join(s.Dir, filepath.FromSlash(doneFolder))
	if _, err := os.Stat(dir); err == nil {
		return nil, fmt.Errorf("%s already exists: the done items were moved", doneFolder)
	}
	if err := os.Mkdir(dir, 0o755); err != nil {
		return nil, err
	}
	s.doneDir = true
	s.Renumber()
	var moved []*Feature
	for _, f := range s.Ordered() {
		switch {
		case f.Status == "done":
			if err := s.put(f); err != nil {
				return nil, err
			}
			moved = append(moved, f)
		case f.file != "" && f.Priority != f.stored:
			if err := s.writeNumber(f); err != nil {
				return nil, err
			}
		}
	}
	return moved, s.WriteSummary()
}
