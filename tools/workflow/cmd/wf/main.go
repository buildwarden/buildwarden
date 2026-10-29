// Command wf reads and moves the feature queue on the workflow orphan branch
// (FLOW-03, FLOW-04, FLOW-05, FLOW-13). Workers call it instead of editing
// queue state by hand, so that step selection, leases and phase transitions
// are mechanical.
//
//	wf validate -state DIR
//	wf status   -state DIR
//	wf next     -state DIR -worker cloud [-feature F]
//	wf claim    -state DIR -worker W -feature F -step S [-new-run]
//	wf release  -state DIR -worker W -feature F -step S -why TEXT
//	wf check    -state DIR -repo DIR -feature F [-run]
//	wf done     -state DIR -repo DIR -worker W -feature F -step S [-run]
//	wf answer   -state DIR -worker W -feature F -question Q -answer TEXT
//	wf back     -state DIR -worker W -feature F -why FINDING
//	wf new      -state DIR -worker W -feature F -title T -priority N
//	            -estimate-steps N -estimate-runs N [-source S]
//	wf gate     -state DIR -repo DIR -machine M -stamp FILE [-sweep-days N] [-record]
//
// DIR is a checkout of the workflow branch; -repo is a checkout of the code
// with origin fetched. A worker is cloud, human or host:<name>, optionally
// with a /<session> suffix that tells its leases apart. Commands that change
// state write the queue file; the caller commits and pushes it, and re-reads
// state if the push is rejected.
//
// gate is the scheduled native session's go/no-go (FLOW-09): it exits 0 when
// machine M (hosts.yaml) has a ready host step, or open backlog gaps and main
// or a waiting branch moved since the stamp; 3 when idle. -record writes the
// stamp after a finished session.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/buildwarden/buildwarden/tools/workflow/check"
	"github.com/buildwarden/buildwarden/tools/workflow/queue"
)

const usage = "usage: wf " +
	"validate|status|next|claim|release|check|done|answer|back|new|gate [flags]"

// errIdle is gate's "nothing to do", exit status 3.
var errIdle = errors.New("idle")

type opts struct {
	state, repo, worker, feature, step, why, question, answer string
	title, source, machine, stamp                             string
	priority, estSteps, estRuns, sweepDays                    int
	newRun, run, record                                       bool
	now                                                       time.Time
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, usage)
		os.Exit(2)
	}
	if err := run(os.Args[1], os.Args[2:], time.Now().UTC()); err != nil {
		if errors.Is(err, errIdle) {
			os.Exit(3)
		}
		fmt.Fprintln(os.Stderr, "wf:", err)
		os.Exit(1)
	}
}

func run(cmd string, args []string, now time.Time) error {
	o := opts{now: now}
	fs := flag.NewFlagSet(cmd, flag.ExitOnError)
	fs.StringVar(&o.state, "state", ".", "checkout of the workflow branch")
	fs.StringVar(&o.repo, "repo", ".", "checkout of the code, origin fetched")
	fs.StringVar(&o.worker, "worker", "", "cloud, human or host:<name>, optionally /<session>")
	fs.StringVar(&o.feature, "feature", "", "feature name")
	fs.StringVar(&o.step, "step", "", "step id")
	fs.BoolVar(&o.newRun, "new-run", false, "first claim of this feature in this run")
	fs.StringVar(&o.why, "why", "", "reason for a release")
	fs.BoolVar(&o.run, "run", false, "run make lint / make test where a check needs them")
	fs.StringVar(&o.question, "question", "", "question id")
	fs.StringVar(&o.answer, "answer", "", "the human's answer")
	fs.StringVar(&o.title, "title", "", "new: the feature's title")
	fs.StringVar(&o.source, "source", "", "new: where the item came from")
	fs.IntVar(&o.priority, "priority", 0, "new: priority, 1 highest")
	fs.IntVar(&o.estSteps, "estimate-steps", 0, "new: estimated steps")
	fs.IntVar(&o.estRuns, "estimate-runs", 0, "new: estimated runs")
	fs.StringVar(&o.machine, "machine", "", "gate: the machine's name in hosts.yaml")
	fs.StringVar(&o.stamp, "stamp", "", "gate: file recording the last finished session")
	fs.IntVar(&o.sweepDays, "sweep-days", 7, "gate: re-sweep open gaps after this many days")
	fs.BoolVar(&o.record, "record", false, "gate: write the stamp instead of deciding")
	_ = fs.Parse(args)

	s, err := queue.Load(o.state)
	if err != nil {
		return err
	}
	cmds := map[string]func(*queue.State, opts) error{
		"validate": validate, "status": status, "next": next, "check": checkPhase,
		"answer": answer, "back": back, "new": newFeature, "gate": gate,
	}
	for _, c := range []string{"claim", "release", "done"} {
		cmds[c] = func(s *queue.State, o opts) error { return stepCmd(c, s, o) }
	}
	f, ok := cmds[cmd]
	if !ok {
		return fmt.Errorf("unknown command %q; %s", cmd, usage)
	}
	return f(s, o)
}

func validate(s *queue.State, _ opts) error {
	errs := s.Validate()
	for _, e := range errs {
		fmt.Println(e)
	}
	if len(errs) > 0 {
		return fmt.Errorf("%d problems", len(errs))
	}
	fmt.Printf("ok: %d features\n", len(s.Features))
	return nil
}

func status(s *queue.State, _ opts) error {
	for _, f := range s.Features {
		b := f.Budget
		fmt.Printf("%d\t%s\t%s\t%s/%d %s\tsteps %d/%d\truns %d/%d\n",
			f.Priority, f.Feature, f.Status, f.Round, f.Phase,
			queue.PhaseNames[f.Phase],
			b.Spent.Steps, b.Estimate.Steps, b.Spent.Runs, b.Estimate.Runs)
		for _, q := range f.Questions {
			if q.Status == "open" {
				fmt.Printf("\t  %s question %s: %s (recommended: %s)\n",
					q.Kind, q.ID, q.Question, q.Recommended)
			}
		}
	}
	return nil
}

func next(s *queue.State, o opts) error {
	pick, blocked := s.Next(o.worker, o.now)
	if o.feature != "" {
		pick, blocked = s.NextFor(o.feature, o.worker, o.now)
	}
	for _, b := range blocked {
		fmt.Printf("blocked\t%s\t%s\n", b.Feature, b.Reason)
	}
	if pick == nil {
		fmt.Println("none")
		return nil
	}
	start := ""
	if pick.Starts {
		start = "\tstarts-feature"
	}
	fmt.Printf("next\t%s\t%s\t%s%s\n",
		pick.Feature.Feature, pick.Step.ID, pick.Step.Affinity, start)
	return nil
}

func checkPhase(s *queue.State, o opts) error {
	f, err := s.Find(o.feature)
	if err != nil {
		return err
	}
	if !runChecks(o, f) {
		return fmt.Errorf("phase %d done-when not met", f.Phase)
	}
	return nil
}

// stepCmd is claim, release and done.
func stepCmd(cmd string, s *queue.State, o opts) error {
	f, err := s.Find(o.feature)
	if err != nil {
		return err
	}
	st, err := f.Step(o.step)
	if err != nil {
		return err
	}
	if o.worker == "" || !queue.Satisfies(o.worker, st.Affinity) {
		return fmt.Errorf("worker %q may not take %s step %s (FLOW-04)", o.worker,
			st.Affinity, st.ID)
	}
	switch cmd {
	case "claim":
		if st.Status != "pending" {
			return fmt.Errorf("%s is %s", st.ID, st.Status)
		}
		if st.Lease.Held(o.now) && st.Lease.ClaimedBy != o.worker {
			return fmt.Errorf("%s is leased to %s until %s",
				st.ID, st.Lease.ClaimedBy, st.Lease.Expires)
		}
		s.Claim(f, st, o.worker, o.now, o.newRun)
	case "release":
		s.Release(f, st, o.worker, o.now, o.why)
	case "done":
		if queue.IsGate(st) && !runChecks(o, f) {
			return fmt.Errorf("%s done-when not met; phase stays %d (FLOW-03)", st.ID,
				f.Phase)
		}
		queue.Complete(f, st, o.worker, o.now)
	}
	return s.Save(f)
}

func answer(s *queue.State, o opts) error {
	f, err := s.Find(o.feature)
	if err != nil {
		return err
	}
	if queue.Kind(o.worker) != "human" {
		return fmt.Errorf("only a human answers questions (FLOW-07)")
	}
	for i := range f.Questions {
		q := &f.Questions[i]
		if q.ID != o.question {
			continue
		}
		q.Answer = o.answer
		q.Status = map[string]string{"blocking": "answered",
			"assumed": "confirmed"}[q.Kind]
		f.Log(o.now, o.worker, "%s question %s %s: %s", q.Kind, q.ID, q.Status, o.answer)
		for j := range f.Steps {
			st := &f.Steps[j]
			if st.Status == "pending" && strings.HasSuffix(st.ID, "-answer-"+q.ID) {
				queue.Complete(f, st, o.worker, o.now)
			}
		}
		return s.Save(f)
	}
	return fmt.Errorf("%s has no question %q", f.Feature, o.question)
}

// back returns the feature to Design with the finding (FLOW-06).
func back(s *queue.State, o opts) error {
	f, err := s.Find(o.feature)
	if err != nil {
		return err
	}
	if o.worker == "" || o.why == "" {
		return fmt.Errorf("back needs -worker and -why (the finding, FLOW-06)")
	}
	queue.Back(f, o.worker, o.now, o.why)
	return s.Save(f)
}

// newFeature queues a feature (the workflow-intake entry point).
func newFeature(s *queue.State, o opts) error {
	if o.feature == "" || o.title == "" || o.priority < 1 || o.estSteps < 1 {
		return fmt.Errorf("new needs -feature, -title, -priority and -estimate-steps")
	}
	f := &queue.Feature{
		Feature: o.feature, Title: o.title, Priority: o.priority, Source: o.source,
		Design: "docs/design/" + o.feature + ".md", Branch: "feat/" + o.feature,
		Budget: queue.Budget{Estimate: queue.Count{Runs: o.estRuns, Steps: o.estSteps}},
	}
	changed, err := s.Add(f, o.now, o.worker)
	if err != nil {
		return err
	}
	for _, c := range changed {
		if err := s.Save(c); err != nil {
			return err
		}
	}
	return nil
}

func runChecks(o opts, f *queue.Feature) bool {
	rs := check.Phase(check.Env{Repo: o.repo, Workflow: o.state, Run: o.run}, f)
	for _, r := range rs {
		mark := "ok  "
		if !r.OK {
			mark = "FAIL"
		}
		fmt.Printf("%s %s: %s\n", mark, r.Name, lastLines(r.Info, 6))
	}
	return check.OK(rs)
}

func lastLines(s string, n int) string {
	lines := strings.Split(s, "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n     ")
}

// gate decides whether a machine's scheduled native session should start an
// agent, or (-record) stamps what a finished session saw.
func gate(s *queue.State, o opts) error {
	if o.machine == "" || o.stamp == "" {
		return fmt.Errorf("gate needs -machine and -stamp")
	}
	m, err := s.Machine(o.machine)
	if err != nil {
		return err
	}
	last, err := queue.ReadStamp(o.stamp)
	if err != nil {
		return err
	}
	g, err := s.Gate(m, o.repo, last, o.sweepDays, o.now)
	if err != nil {
		return err
	}
	if o.record {
		fmt.Printf("recorded\t%s\n", g.Print)
		return queue.WriteStamp(o.stamp, queue.GateStamp{Print: g.Print, At: o.now})
	}
	for _, r := range g.Reasons {
		fmt.Println(r)
	}
	if !g.Run {
		return errIdle
	}
	return nil
}
