package queue

import (
	"bytes"
	"errors"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"
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
	// Renewed names the step whose lease the change renews, as
	// FEATURE/STEP. A rejected renewal is renewed again on the state that
	// won, and never claims (FLOW-04).
	Renewed string
	// Paths are the files of the session's change, from the checkout's root.
	// Nothing else but the generated files is staged.
	Paths []string
	Now   time.Time
	// Wire sets, on each state wf commit loads, what wf sets on its own: the
	// merge gate a claim asked again checks (MergeGate) and whether a loop
	// merge waits on the human (MergeWaitsOnHuman), which QUEUE.md shows.
	// Nil, they stay unset: no loop merge is claimed again.
	Wire func(*State)
}

// wire sets on s what r.Wire sets.
func (r CommitRequest) wire(s *State) {
	if r.Wire != nil {
		r.Wire(s)
	}
}

// MayClaim says why the worker may not claim the step: it is not pending, its
// item has not reached its phase (FLOW-04), someone else holds it, a process
// item holds the worker, or a limit refuses the claim, whoever the worker is
// (FLOW-05), or, for a loop merge, another loop merge in its repository is
// claimed or the merge gate does not pass (FLOW-08). A step the worker
// already holds stays claimable. wf claim and a claim wf commit asks again
// share it.
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
	if IsMerge(st) || isMergedFirst(st) {
		return s.mayClaimMerge(f, st, now, mine)
	}
	return nil
}

// mayClaimMerge is MayClaim's test of a merge step: only a loop merge is
// claimed, and only while no other loop merge in its repository is and the
// merge gate passes, unless the worker already holds it (FLOW-08).
func (s *State) mayClaimMerge(f *Feature, st *Step, now time.Time, mine bool) error {
	if !s.LoopMerges(f, st) {
		return fmt.Errorf("%s waits for the human's merge and is never claimed "+
			"(FLOW-08)", st.ID)
	}
	if mine {
		return nil
	}
	why := s.mergeBusy(f, st, now)
	if why == "" {
		why = s.gateWait(f, st)
	}
	if why != "" {
		return fmt.Errorf("%s may not be claimed: it waits on %s", st.ID, why)
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
	claim, err := g.requestPaths(&r)
	if err != nil {
		return nil, err
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

// requestPaths adds to the session's paths the claimed or renewed item's
// file and the done-folder counterparts (pairDone), and returns the claim.
func (g stateGit) requestPaths(r *CommitRequest) (*claimRef, error) {
	claim, err := r.ref()
	if err != nil {
		return nil, err
	}
	if claim != nil {
		r.Paths = append(r.Paths, "queue/"+claim.feature+".yaml")
	}
	paths, err := g.pairDone(r.Paths)
	r.Paths = paths
	return claim, err
}

// claimRef is the step a change claims or, with renew, whose lease it renews.
type claimRef struct {
	feature, step string
	renew         bool
}

// what is the claim or renewal as wf commit's messages name it.
func (c *claimRef) what() string {
	if c.renew {
		return "renewal"
	}
	return "claim"
}

// ref parses -claimed or -renewed, which exclude each other.
func (r CommitRequest) ref() (*claimRef, error) {
	flag, v := "-claimed", r.Claimed
	switch {
	case r.Claimed != "" && r.Renewed != "":
		return nil, errors.New("commit takes -claimed or -renewed, not both")
	case r.Renewed != "":
		flag, v = "-renewed", r.Renewed
	case v == "":
		return nil, nil
	}
	feature, step, ok := strings.Cut(v, "/")
	if !ok || feature == "" || step == "" {
		return nil, fmt.Errorf("%s is FEATURE/STEP, not %q", flag, v)
	}
	return &claimRef{feature, step, flag == "-renewed"}, nil
}

// refuseMapChange refuses a change to areas.yaml from a worker that is not
// human: the map is the human's word (FLOW-05). While a program trunk is
// configured (FLOW-20) the loop may change it as a reviewed or
// umbrella-passed program design names, or to bring reaches in line with the
// dependency edges, and records each change: the commit then passes when
// the records it carries to origin add at least one area-map record for each
// map change it carries. It looks at the session's paths and at the
// unpushed commits the push would carry with them. A hand push is not
// stopped.
func (g stateGit) refuseMapChange(r CommitRequest) error {
	if Kind(r.Worker) == "human" {
		return nil
	}
	session := false
	if len(r.Paths) > 0 {
		changed, err := g.lines(append([]string{"status", "--porcelain", "--"},
			r.Paths...)...)
		if err != nil {
			return err
		}
		for _, l := range changed {
			if len(l) > 3 && l[3:] == "areas.yaml" {
				session = true
			}
		}
	}
	if _, err := g.run("fetch", "-q", "origin", workflowBranch); err != nil {
		return err
	}
	origin, err := g.configAt("origin/" + workflowBranch + ":config.yaml")
	if err != nil {
		return err
	}
	if err := g.refuseProgramTrunkChange(r, origin); err != nil {
		return err
	}
	unpushed, err := g.lines("log", "--format=%h %s", "origin/"+workflowBranch+"..HEAD",
		"--", "areas.yaml")
	if err != nil {
		return err
	}
	changes := len(unpushed)
	if session {
		changes++
	}
	if changes == 0 {
		return nil
	}
	// Whether the loop may change the map is read from the pushed
	// config.yaml, never the session's copy (FLOW-20).
	if origin.HasProgramTrunk() {
		return g.mapChangesRecorded(r, changes)
	}
	refused := fmt.Sprintf("%s may not commit a change to areas.yaml: the area "+
		"map is changed on the human's word, by a human worker (FLOW-05)", r.Worker)
	if !session {
		return fmt.Errorf("%s, and the push would carry the unpushed commit %s, "+
			"which changes it", refused, strings.Join(unpushed, "; "))
	}
	return errors.New(refused)
}

// configAt reads config.yaml at a revision, REV:PATH, or in the working
// tree for "". A revision with none is an empty config.
func (g stateGit) configAt(ref string) (Config, error) {
	var cfg Config
	if ref == "" {
		return cfg, readYAML(filepath.Join(g.dir, "config.yaml"), &cfg)
	}
	data, err := g.run("show", ref)
	if err != nil {
		return cfg, nil
	}
	if err := yaml.Unmarshal([]byte(data), &cfg); err != nil {
		return cfg, fmt.Errorf("%s: %w", ref, err)
	}
	return cfg, nil
}

// programTrunks maps each repository entry to its program trunk.
func programTrunks(c Config) map[string]string {
	out := map[string]string{}
	for _, r := range c.Repositories {
		if r.ProgramTrunk != "" {
			out[r.Name] = r.ProgramTrunk
		}
	}
	return out
}

// refuseProgramTrunkChange refuses, from a worker that is not human, a push
// that adds, changes or removes a program_trunk in config.yaml: the session's
// copy when it commits config.yaml, else HEAD's, against origin's. A program
// trunk is what turns on the loop's merges, map changes and tests, and is
// configured by the human's state step (FLOW-20).
func (g stateGit) refuseProgramTrunkChange(r CommitRequest, origin Config) error {
	ref := "HEAD:config.yaml"
	if slices.Contains(r.Paths, "config.yaml") {
		ref = ""
	}
	carried, err := g.configAt(ref)
	if err != nil {
		return err
	}
	if maps.Equal(programTrunks(carried), programTrunks(origin)) {
		return nil
	}
	return fmt.Errorf("%s may not commit a change to a program_trunk in config.yaml: "+
		"a program trunk is configured by the human's state step, by a human worker "+
		"(FLOW-20)", r.Worker)
}

// mapChangesRecorded passes a loop's area-map changes while a program trunk
// is configured (FLOW-05, FLOW-20, a16 (a)) when the records the push would
// carry add an area-map record for each of them: the working tree's
// records when the session commits program/records.yaml, else HEAD's,
// against origin's.
func (g stateGit) mapChangesRecorded(r CommitRequest, changes int) error {
	ref := "HEAD:" + RecordsFile
	if slices.Contains(r.Paths, RecordsFile) {
		ref = ""
	}
	carried, err := g.areaMapRecords(ref)
	if err != nil {
		return err
	}
	pushed, err := g.areaMapRecords("origin/" + workflowBranch + ":" + RecordsFile)
	if err != nil {
		return err
	}
	if carried-pushed < changes {
		return fmt.Errorf("%s may commit a change to areas.yaml only as a reviewed or "+
			"umbrella-passed program design names it, or to bring reaches in line "+
			"with the dependency edges, each recorded: the push carries %d map "+
			"change(s) and %d new area-map record(s) (wf record -kind area-map, "+
			"committed with %s; FLOW-05, FLOW-20)", r.Worker, changes, carried-pushed,
			RecordsFile)
	}
	return nil
}

// areaMapRecords counts the area-map records in the records file at a
// revision, REV:PATH, or in the working tree for "". A missing file has
// none.
func (g stateGit) areaMapRecords(ref string) (int, error) {
	var rs []Record
	if ref == "" {
		var err error
		if rs, err = LoadRecords(g.dir); err != nil {
			return 0, err
		}
	} else {
		data, err := g.run("show", ref)
		if err != nil {
			return 0, nil
		}
		if err := yaml.Unmarshal([]byte(data), &rs); err != nil {
			return 0, fmt.Errorf("%s: %w", ref, err)
		}
	}
	n := 0
	for _, r := range rs {
		if r.Kind == "area-map" {
			n++
		}
	}
	return n, nil
}

// pairDone adds to each named queue/F.yaml its counterpart queue/done/F.yaml,
// and the reverse, so a close commits the move. A counterpart is added only
// when the worktree, the index or HEAD has it: git add and git commit --only
// fail on a path that matches nothing, and before wf migrate -done there is
// no queue/done/ at all.
func (g stateGit) pairDone(paths []string) ([]string, error) {
	out := slices.Clone(paths)
	for _, p := range paths {
		var other string
		switch name, ok := strings.CutPrefix(p, doneFolder+"/"); {
		case ok:
			other = "queue/" + name
		case strings.HasPrefix(p, "queue/") && strings.Count(p, "/") == 1:
			other = doneFolder + "/" + strings.TrimPrefix(p, "queue/")
		default:
			continue
		}
		if !strings.HasSuffix(other, ".yaml") || slices.Contains(out, other) {
			continue
		}
		known, err := g.known(other)
		if err != nil {
			return nil, err
		}
		if known {
			out = append(out, other)
		}
	}
	return out, nil
}

// known reports whether the worktree, the index or HEAD has the path.
func (g stateGit) known(p string) (bool, error) {
	if _, err := os.Stat(filepath.Join(g.dir, filepath.FromSlash(p))); err == nil {
		return true, nil
	}
	indexed, err := g.lines("ls-files", "--", p)
	if err != nil {
		return false, err
	}
	if len(indexed) > 0 {
		return true, nil
	}
	_, err = g.run("cat-file", "-e", "HEAD:"+p)
	return err == nil, nil
}

// commitOnce regenerates the generated files and commits them with the
// session's paths. made is false when there was nothing to commit.
func (g stateGit) commitOnce(r CommitRequest) (lines []string, made bool, err error) {
	generated, err := g.generate(r)
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
// digest's day pages, and returns their paths with the queue files it
// commits beside the session's. It renumbers what no command did: a hand
// edit, an old wf's write, a replay (queue-positions, Decision 1). A queue
// file outside the session's paths is committed when its only uncommitted
// change is its priority line, and a done item an old wf left in queue/ is
// moved to queue/done/ when its file has no other uncommitted change; a file
// with other uncommitted changes is left alone, as --only leaves it.
func (g stateGit) generate(r CommitRequest) ([]string, error) {
	paths := r.Paths
	s, err := Load(g.dir)
	if err != nil {
		return nil, err
	}
	r.wire(s)
	s.Renumber()
	var extra []string
	for _, f := range s.Ordered() {
		more, err := g.settle(s, f, paths)
		if err != nil {
			return nil, err
		}
		extra = append(extra, more...)
	}
	pages, err := s.Digest()
	if err != nil {
		return nil, err
	}
	return slices.Concat([]string{SummaryFile}, pages, extra), s.WriteSummary()
}

// settle writes what renumbering changed in one item's file, or moves a done
// item an old wf left in queue/, as generate says, and returns the paths to
// commit beside the session's.
func (g stateGit) settle(s *State, f *Feature, paths []string) ([]string, error) {
	if f.file == "" {
		return nil, nil
	}
	named := slices.ContainsFunc(paths, func(p string) bool {
		return p == f.file || strings.HasPrefix(f.file, strings.TrimSuffix(p, "/")+"/")
	})
	change := "named"
	if !named {
		var err error
		if change, err = g.change(f.file); err != nil || change == "other" {
			return nil, err
		}
	}
	switch {
	case f.Status == "done" && s.doneDir && !strings.HasPrefix(f.file, doneFolder+"/"):
		left := f.file
		return []string{left, s.fileOf(f)}, s.put(f)
	case f.Priority != f.stored:
		return []string{f.file}, s.writeNumber(f)
	case change == "priority":
		return []string{f.file}, nil
	}
	return nil, nil
}

// change says how a queue file differs from HEAD: none, priority when only
// its priority line differs, other otherwise, a file HEAD lacks included.
func (g stateGit) change(p string) (string, error) {
	now, err := os.ReadFile(filepath.Join(g.dir, filepath.FromSlash(p)))
	if err != nil {
		return "", err
	}
	was, err := g.run("show", "HEAD:"+p)
	if err != nil {
		return "other", nil
	}
	switch {
	case string(now) == was:
		return "none", nil
	case withoutPriority(string(now)) == withoutPriority(was):
		return "priority", nil
	}
	return "other", nil
}

// withoutPriority is an item file's text without its priority line.
func withoutPriority(text string) string {
	return priorityLine.ReplaceAllString(text, "")
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
	return &CommitError{5, msg + ": the " + claim.what() + " is dropped; ask wf next again"}
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
	switch {
	case claim != nil && claim.renew:
		return g.renewAgain(r, claim)
	case claim != nil:
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
// checks and, for any worker but human, the refusals wf next adds, and claims the
// step once more when nothing refuses it. The first claim's commit is gone,
// so the step is counted once.
func (g stateGit) claimAgain(r CommitRequest, c *claimRef) ([]string, error) {
	s, err := Load(g.dir)
	if err != nil {
		return nil, err
	}
	r.wire(s)
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
	if Kind(r.Worker) != "human" {
		if why, _ := s.unattendedHold(f, st); why != "" {
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

// renewAgain renews the lease again on the remote's state through Renew, the
// check wf renew makes: refused unless the step is still pending and its live
// lease the worker's. It never claims, so the item's budget is unchanged.
func (g stateGit) renewAgain(r CommitRequest, c *claimRef) ([]string, error) {
	s, err := Load(g.dir)
	if err != nil {
		return nil, err
	}
	refuse := func(why string) error {
		return &CommitError{4, fmt.Sprintf("the renewal of %s/%s is refused on the new "+
			"state: %s; stop working on the step", c.feature, c.step, why)}
	}
	f, err := s.Find(c.feature)
	if err != nil {
		return nil, refuse(err.Error())
	}
	st, err := f.Step(c.step)
	if err != nil {
		return nil, refuse(err.Error())
	}
	if err := s.Renew(f, st, r.Worker, r.Now); err != nil {
		return nil, refuse(err.Error())
	}
	return []string{fmt.Sprintf("renewed again on the new state: %s/%s", c.feature,
		c.step)}, s.Save(f)
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
	lines, err := g.cherryPick(own)
	if err != nil {
		return nil, err
	}
	_, _ = g.run("cherry-pick", "--quit")
	if err := renumberFiles(g.dir); err != nil {
		return nil, g.restore(own, "the replayed state does not load: "+err.Error())
	}
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

// cherryPick applies the session's commit to the checkout without committing
// it, and settles the conflicts replay may settle. It returns replay's lines.
func (g stateGit) cherryPick(own string) ([]string, error) {
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
			case strings.HasPrefix(p, "queue/"):
				if ok, err := g.remerge(own, p); err != nil {
					return nil, err
				} else if !ok {
					hard = append(hard, p)
				}
				continue
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
	return lines, nil
}

// placedLine matches a history line that places an item.
var placedLine = regexp.MustCompile(`(?m)^\s*event: (queued|moved) to position .*$`)

// remerge settles a replay's conflict in a queue file when it is confined to
// priority lines, which both sides may have renumbered (queue-positions,
// Decision 11). It merges the file again with each side's priority line set
// to the remote's value, or to the session's when the session's change
// placed the item, and takes the result when that merges cleanly. It reports
// whether it did.
func (g stateGit) remerge(own, p string) (bool, error) {
	ours, err := g.run("show", "HEAD:"+p)
	if err != nil {
		return false, nil
	}
	theirs, err := g.run("show", own+":"+p)
	if err != nil {
		return false, nil
	}
	base, _ := g.run("show", own+"^:"+p)
	keep := priorityLine.FindString(ours)
	for _, l := range placedLine.FindAllString(theirs, -1) {
		if !strings.Contains(base, l+"\n") {
			keep = priorityLine.FindString(theirs)
			break
		}
	}
	if keep == "" {
		return false, nil
	}
	tmp, err := os.MkdirTemp("", "wf-remerge-")
	if err != nil {
		return false, err
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	var files []string
	for i, text := range []string{ours, base, theirs} {
		f := filepath.Join(tmp, strconv.Itoa(i))
		text = priorityLine.ReplaceAllLiteralString(text, keep)
		if err := os.WriteFile(f, []byte(text), 0o644); err != nil {
			return false, err
		}
		files = append(files, f)
	}
	merged, err := g.run(append([]string{"merge-file", "-p"}, files...)...)
	if err != nil {
		return false, nil
	}
	if err := os.WriteFile(filepath.Join(g.dir, filepath.FromSlash(p)), []byte(merged),
		0o644); err != nil {
		return false, err
	}
	_, err = g.run("add", "--", p)
	return err == nil, err
}

// renumberFiles renumbers the state in dir and writes each number that moved,
// so a replayed state is validated in positions.
func renumberFiles(dir string) error {
	s, err := Load(dir)
	if err != nil {
		return err
	}
	s.Renumber()
	for _, f := range s.Features {
		if f.file == "" || f.Priority == f.stored || (f.Status == "done" && s.doneDir) {
			continue
		}
		if err := s.writeNumber(f); err != nil {
			return err
		}
	}
	return nil
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
