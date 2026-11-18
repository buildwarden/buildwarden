package queue

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"slices"
	"strings"
	"time"
)

// The branch wf commit pushes, and the only one.
const workflowBranch = "workflow"

// commitTries is how many pushes wf commit makes before it stops: enough for a
// busy minute, after which the session decides.
const commitTries = 3

// CommitError is a wf commit that stopped short of a push, with its exit
// status: 4 when a claim asked again on the new state is refused, 5 when the
// session must settle something by hand.
type CommitError struct {
	Code int
	Msg  string
}

func (e *CommitError) Error() string { return e.Msg }

// CommitRequest is what wf commit is asked to push.
type CommitRequest struct {
	Worker, Message string
	// Claimed names the step the change claims, as FEATURE/STEP. A rejected
	// claim is never replayed: it is asked again on the state that won.
	Claimed string
	// Paths are the files of the session's change, from the checkout's root.
	// Nothing else but the generated files is staged.
	Paths []string
	Now   time.Time
}

// MayClaim says why the worker may not claim the step: it is not pending, its
// item has not reached its phase (FLOW-04), someone else holds it, a process
// item holds the worker, or a limit refuses the claim, whoever the worker is
// (FLOW-05). A step the worker already holds stays claimable. wf claim and a
// claim wf commit asks again share it.
func (s *State) MayClaim(f *Feature, st *Step, worker string, now time.Time) error {
	if st.Status != "pending" {
		return fmt.Errorf("%s is %s", st.ID, st.Status)
	}
	if err := Early(f, st); err != nil {
		return err
	}
	mine := st.Lease.Held(now) && st.Lease.ClaimedBy == worker
	if st.Lease.Held(now) && !mine {
		return fmt.Errorf("%s is leased to %s until %s",
			st.ID, st.Lease.ClaimedBy, st.Lease.Expires)
	}
	if h := s.Hold(f, worker); h != "" && !mine {
		return fmt.Errorf("%s is held by process item %s (FLOW-05)", worker, h)
	}
	if why := s.Guard(f, st, worker); why != "" && !mine {
		return fmt.Errorf("%s may not be claimed: %s", st.ID, why)
	}
	return nil
}

// stateGit runs git in a checkout of the workflow branch.
type stateGit struct{ dir string }

func (g stateGit) run(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = g.dir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return string(out), fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err,
			strings.TrimSpace(stderr.String()))
	}
	return string(out), nil
}

// lines runs git and returns its output's non-empty lines.
func (g stateGit) lines(args ...string) ([]string, error) {
	out, err := g.run(args...)
	if err != nil {
		return nil, err
	}
	var ls []string
	for _, l := range strings.Split(out, "\n") {
		if l != "" {
			ls = append(ls, l)
		}
	}
	return ls, nil
}

// Commit is wf commit: it regenerates the generated files, commits the
// session's paths with them, and pushes the workflow branch (FLOW-04,
// FLOW-13). On a rejected push it fetches, resets the checkout to the
// remote's state and brings the change onto it: a change by replaying its
// commit, a claim by asking it again. It tries commitTries times, rewrites
// only its own unpushed commit, and never force-pushes. It returns the lines
// to print.
func Commit(dir string, r CommitRequest) ([]string, error) {
	g := stateGit{dir}
	if r.Worker == "" || r.Message == "" {
		return nil, errors.New("commit needs -worker and -m")
	}
	branch, err := g.run("symbolic-ref", "--short", "HEAD")
	if err != nil || strings.TrimSpace(branch) != workflowBranch {
		return nil, fmt.Errorf("%s is not a checkout of the %s branch", dir,
			workflowBranch)
	}
	var claim *claimRef
	if r.Claimed != "" {
		if claim, err = parseClaimed(r.Claimed); err != nil {
			return nil, err
		}
		r.Paths = append(r.Paths, "queue/"+claim.feature+".yaml")
	}
	if err := g.refuseMapChange(r); err != nil {
		return nil, err
	}
	var out []string
	for try := 1; ; try++ {
		lines, made, err := g.commitOnce(r)
		out = append(out, lines...)
		if err != nil || !made {
			return out, err
		}
		rejected, err := g.push()
		if err != nil || !rejected {
			return out, err
		}
		if try == commitTries {
			return out, g.giveUp(claim)
		}
		lines, err = g.bringOnto(r, claim)
		out = append(out, lines...)
		if err != nil {
			return out, err
		}
	}
}

type claimRef struct{ feature, step string }

func parseClaimed(v string) (*claimRef, error) {
	feature, step, ok := strings.Cut(v, "/")
	if !ok || feature == "" || step == "" {
		return nil, fmt.Errorf("-claimed is FEATURE/STEP, not %q", v)
	}
	return &claimRef{feature, step}, nil
}

// refuseMapChange refuses a change to areas.yaml from a worker that is not
// human: the map is the human's word (FLOW-05). It looks at the session's
// paths and at the unpushed commits the push would carry with them. A hand
// push is not stopped.
func (g stateGit) refuseMapChange(r CommitRequest) error {
	if Kind(r.Worker) == "human" {
		return nil
	}
	refused := fmt.Sprintf("%s may not commit a change to areas.yaml: the area "+
		"map is changed on the human's word, by a human worker (FLOW-05)", r.Worker)
	if len(r.Paths) > 0 {
		changed, err := g.lines(append([]string{"status", "--porcelain", "--"},
			r.Paths...)...)
		if err != nil {
			return err
		}
		for _, l := range changed {
			if len(l) > 3 && l[3:] == "areas.yaml" {
				return errors.New(refused)
			}
		}
	}
	if _, err := g.run("fetch", "-q", "origin", workflowBranch); err != nil {
		return err
	}
	unpushed, err := g.lines("log", "--format=%h %s", "origin/"+workflowBranch+"..HEAD",
		"--", "areas.yaml")
	if err != nil {
		return err
	}
	if len(unpushed) > 0 {
		return fmt.Errorf("%s, and the push would carry the unpushed commit %s, "+
			"which changes it", refused, strings.Join(unpushed, "; "))
	}
	return nil
}

// commitOnce regenerates the generated files and commits them with the
// session's paths. made is false when there was nothing to commit.
func (g stateGit) commitOnce(r CommitRequest) (lines []string, made bool, err error) {
	generated, err := generate(g.dir)
	if err != nil {
		return nil, false, err
	}
	paths := slices.Concat(r.Paths, generated)
	if _, err := g.run(append([]string{"add", "-A", "--"}, paths...)...); err != nil {
		return nil, false, err
	}
	staged, err := g.lines(append([]string{"diff", "--cached", "--name-only", "--"},
		paths...)...)
	if err != nil {
		return nil, false, err
	}
	if len(staged) == 0 {
		return []string{"nothing to commit"}, false, nil
	}
	// --only commits these paths and leaves whatever else is staged alone:
	// another session's work in the same checkout is never swept in.
	_, err = g.run(append([]string{"commit", "-q", "--only", "-m", r.Message, "--"},
		paths...)...)
	return nil, err == nil, err
}

// generate writes the files wf builds from the state, QUEUE.md and the
// digest's day pages, and returns their paths.
func generate(dir string) ([]string, error) {
	s, err := Load(dir)
	if err != nil {
		return nil, err
	}
	pages, err := s.Digest()
	if err != nil {
		return nil, err
	}
	return append([]string{SummaryFile}, pages...), s.WriteSummary()
}

// push pushes the checkout's head to the workflow branch. rejected reports a
// push refused because the remote moved, before git sent the push or while
// the remote updated its ref; any other failure is an error.
func (g stateGit) push() (rejected bool, err error) {
	_, err = g.run("push", "-q", "origin", "HEAD:refs/heads/"+workflowBranch)
	if err == nil {
		return false, nil
	}
	msg := err.Error()
	raced := strings.Contains(msg, "cannot lock ref") && strings.Contains(msg, "but expected")
	if raced || strings.Contains(msg, "[rejected]") || strings.Contains(msg, "fetch first") ||
		strings.Contains(msg, "non-fast-forward") {
		return true, nil
	}
	return false, err
}

// giveUp ends wf commit after the last rejected push. A claim is dropped and
// the checkout left at the remote's state; a change keeps its commit locally.
func (g stateGit) giveUp(claim *claimRef) error {
	msg := fmt.Sprintf("the push was rejected %d times", commitTries)
	if claim == nil {
		return &CommitError{5, msg + ": the commit is kept locally"}
	}
	if err := g.resetToRemote(); err != nil {
		return err
	}
	return &CommitError{5, msg + ": the claim is dropped; ask wf next again"}
}

func (g stateGit) resetToRemote() error {
	if _, err := g.run("fetch", "-q", "origin", workflowBranch); err != nil {
		return err
	}
	_, err := g.run("reset", "-q", "--hard", "origin/"+workflowBranch)
	return err
}

// bringOnto puts the session's change on the remote's state after a rejected
// push: it fetches, stops when a reset would discard work that is not its
// own, resets, and replays the change or asks the claim again.
func (g stateGit) bringOnto(r CommitRequest, claim *claimRef) ([]string, error) {
	if _, err := g.run("fetch", "-q", "origin", workflowBranch); err != nil {
		return nil, err
	}
	dirty, err := g.lines("status", "--porcelain")
	if err != nil {
		return nil, err
	}
	if len(dirty) > 0 {
		return nil, &CommitError{5, "the push was rejected, and the checkout has " +
			"changes that are not this commit's, which a reset would discard: " +
			strings.Join(porcelainPaths(dirty), ", ")}
	}
	own, err := g.run("rev-parse", "HEAD")
	if err != nil {
		return nil, err
	}
	own = strings.TrimSpace(own)
	if ahead, err := g.lines("rev-list", "origin/"+workflowBranch+".."+own+"^"); err != nil {
		return nil, err
	} else if len(ahead) > 0 {
		return nil, &CommitError{5, fmt.Sprintf("the push was rejected, and the "+
			"checkout has %d unpushed commits besides this one, which wf commit does "+
			"not rewrite", len(ahead))}
	}
	if _, err := g.run("reset", "-q", "--hard", "origin/"+workflowBranch); err != nil {
		return nil, err
	}
	if claim != nil {
		return g.claimAgain(r, claim)
	}
	return g.replay(own, r.Worker)
}

func porcelainPaths(ls []string) []string {
	out := make([]string, len(ls))
	for i, l := range ls {
		out[i] = strings.TrimSpace(l[min(3, len(l)):])
	}
	return out
}

// claimAgain asks the claim again on the remote's state, through what wf claim
// checks and, for a cloud worker, the refusals wf next adds, and claims the
// step once more when nothing refuses it. The first claim's commit is gone,
// so the step is counted once.
func (g stateGit) claimAgain(r CommitRequest, c *claimRef) ([]string, error) {
	s, err := Load(g.dir)
	if err != nil {
		return nil, err
	}
	refuse := func(why string) error {
		return &CommitError{4, fmt.Sprintf("the claim of %s/%s is refused on the new "+
			"state: %s; ask wf next again", c.feature, c.step, why)}
	}
	f, err := s.Find(c.feature)
	if err != nil {
		return nil, refuse(err.Error())
	}
	st, err := f.Step(c.step)
	if err != nil {
		return nil, refuse(err.Error())
	}
	if err := s.MayClaim(f, st, r.Worker, r.Now); err != nil {
		return nil, refuse(err.Error())
	}
	if Kind(r.Worker) == "cloud" {
		if why, _ := s.cloudHold(f, st); why != "" {
			return nil, refuse(why)
		}
	}
	lines := []string{fmt.Sprintf("claimed again on the new state: %s/%s", c.feature,
		c.step)}
	if past := s.Claim(f, st, r.Worker, r.Now); past != "" {
		lines = append(lines, past)
	}
	return lines, s.Save(f)
}

// mainKept is wf commit's line for a conflict in main.yaml: what it holds is a
// reading, which the session takes again with wf main.
const mainKept = "main.yaml: kept the remote's; record main again"

// generatedFile reports whether wf writes the file, so that a conflict in it
// is settled by the remote's copy: the next commit writes it again.
func generatedFile(p string) bool {
	return p == SummaryFile || (strings.HasPrefix(p, "digest/") &&
		strings.Count(p, "/") == 1 && strings.HasSuffix(p, ".md"))
}

// replay brings the session's commit onto the remote's state as a change and
// checks the result. A conflict in a generated file, or in main.yaml, which
// holds a reading that can be taken again, keeps the remote's copy. Any
// other conflict, or a Validate error the remote's state did not have,
// restores the session's commit and stops.
func (g stateGit) replay(own, worker string) ([]string, error) {
	before, err := validated(g.dir)
	if err != nil {
		return nil, err
	}
	var lines []string
	if _, err := g.run("cherry-pick", "--no-commit", own); err != nil {
		conflicts, lerr := g.lines("diff", "--name-only", "--diff-filter=U")
		if lerr != nil || len(conflicts) == 0 {
			return nil, errors.Join(err, lerr)
		}
		var hard []string
		for _, p := range conflicts {
			switch {
			case generatedFile(p):
			case p == "main.yaml":
				lines = append(lines, mainKept)
			default:
				hard = append(hard, p)
				continue
			}
			if _, err := g.run("checkout", "--ours", "--", p); err != nil {
				hard = append(hard, p)
				continue
			}
			if _, err := g.run("add", "--", p); err != nil {
				return nil, err
			}
		}
		if len(hard) > 0 {
			return nil, g.restore(own, "the replay conflicts in "+
				strings.Join(hard, ", ")+": re-apply the change by hand")
		}
	}
	_, _ = g.run("cherry-pick", "--quit")
	after, err := validated(g.dir)
	if err != nil {
		return nil, g.restore(own, "the replayed state does not load: "+err.Error())
	}
	var fresh []string
	for _, e := range after {
		if !slices.Contains(before, e) {
			fresh = append(fresh, e)
		}
	}
	if len(fresh) > 0 {
		return nil, g.restore(own, "wf validate fails on the replayed state:\n"+
			strings.Join(fresh, "\n"))
	}
	return append(lines, "replayed onto origin/"+workflowBranch+" for "+worker), nil
}

// restore puts the checkout back on the session's own commit, ahead of the
// remote, and returns the exit-5 error with why.
func (g stateGit) restore(own, why string) error {
	_, _ = g.run("cherry-pick", "--quit")
	if _, err := g.run("reset", "-q", "--hard", own); err != nil {
		return err
	}
	return &CommitError{5, why + "; the session's commit is restored"}
}

// validated loads the state in dir and returns Validate's lines.
func validated(dir string) ([]string, error) {
	s, err := Load(dir)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range s.Validate() {
		out = append(out, e.Error())
	}
	return out, nil
}
