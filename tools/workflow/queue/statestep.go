package queue

import (
	"errors"
	"fmt"
	"slices"
	"time"
)

// SetStateStep records a process item's state step, or with clear removes
// it, for a human worker: wf state-step (FLOW-03). Before Close it records
// the field alone, which Seed reads when the item enters Close. In Close it
// adds x4-state at once and makes x4-close wait for it, or gives a pending
// x4-state the new note; clear removes a pending x4-state and x4-close's
// need on it. A done x4-state is the human's finished work: it is refused
// either way.
func SetStateStep(f *Feature, note string, clear bool, worker string, now time.Time) error {
	switch {
	case Kind(worker) != "human":
		return errors.New("only a human records a state step (FLOW-03)")
	case !f.IsProcess():
		return fmt.Errorf("%s is not a process item: only a process item has a state "+
			"step (FLOW-03)", f.Feature)
	case f.Status == "done":
		return fmt.Errorf("%s is done: its Close has passed", f.Feature)
	case clear && note != "":
		return errors.New("state-step takes -note or -clear, not both")
	case !clear && note == "":
		return errors.New("state-step needs -note, the checklist, or -clear")
	}
	if err := f.closeStateStep(note, clear); err != nil {
		return err
	}
	f.StateStep = note
	if clear {
		f.Log(now, worker, "state step cleared")
	} else {
		f.Log(now, worker, "state step recorded: %s", note)
	}
	return nil
}

// pendingClose is the index of a process item's pending x4-close while it is
// in Close, or -1.
func (f *Feature) pendingClose() int {
	if f.Phase != 4 {
		return -1
	}
	for i := range f.Steps {
		r, ok := stepTemplates(f.Steps[i].ID)
		if ok && r.round == "process" && r.phase == 4 && r.name == "close" &&
			f.Steps[i].Status == "pending" {
			return i
		}
	}
	return -1
}

// closeStateStep applies a state step's change to Close's steps when the
// item is in Close with its x4-close pending; before Close there is nothing
// to change.
func (f *Feature) closeStateStep(note string, clear bool) error {
	gate := f.pendingClose()
	if gate < 0 {
		return nil
	}
	suffix := f.Steps[gate].ID[len(StepID(f.Round, 4, "close")):]
	id := StepID(f.Round, 4, "state") + suffix
	st, err := f.Step(id)
	switch {
	case err == nil && st.Status == "done":
		return fmt.Errorf("%s is done: the state step is finished", id)
	case err == nil && clear:
		f.Steps = slices.DeleteFunc(f.Steps, func(o Step) bool { return o.ID == id })
		for i := range f.Steps {
			f.Steps[i].Needs = slices.DeleteFunc(f.Steps[i].Needs,
				func(n string) bool { return n == id })
		}
	case err == nil:
		st.Note = note
	case !clear:
		f.Steps[gate].Needs = addNeeds(f.Steps[gate].Needs, id)
		f.Steps = append(f.Steps, Step{ID: id, Phase: 4, Affinity: "human",
			Status: "pending", Note: note})
	}
	return nil
}
