package queue

import (
	"fmt"
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
