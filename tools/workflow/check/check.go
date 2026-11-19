// Package check runs the FLOW-03 done-when checks: a phase transition happens
// only when these pass, never on an agent's report that it is done.
package check

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/buildwarden/buildwarden/tools/workflow/queue"
)

// Env is where the checks look.
type Env struct {
	Repo     string // a checkout of the queue's own repository with origin fetched
	Workflow string // a checkout of the workflow branch
	// Checkouts holds a clone of each other repository an item works in,
	// at Checkouts/<owner>/<name>, origin fetched (-checkouts). Empty, wf
	// reads no second repository, and a check that needs one fails.
	Checkouts string
	// Run enables the checks that run make targets; off, they are reported
	// as skipped and fail.
	Run bool
	// State is the queue state, for the area map and the other items. Nil,
	// there is no map: every item is undeclared.
	State *queue.State
}

// Result is one check's outcome.
type Result struct {
	Name string
	OK   bool
	Info string
}

// Phase runs the done-when checks for the feature's current phase.
func Phase(env Env, f *queue.Feature) []Result {
	c := newChecker(env, f)
	if f.Round == "tracked" {
		// The item's branch, design and PR are in its home repository,
		// where wf sees none of them.
		c.add("tracked", true, "wf runs no gate in %s (FLOW-03)",
			strings.Join(f.Repos[:min(1, len(f.Repos))], ""))
		return c.out
	}
	if !c.ready() {
		return c.out
	}
	if f.Round == "process" {
		c.process()
		return c.out
	}
	switch f.Phase {
	case 0:
		c.each(c.branchOnRemote)
	case 1:
		c.design()
	case 2:
		c.plan()
		c.reviewed()
		c.approvalDelta()
		c.declaredStill()
	case 3:
		c.each(c.onBranchHead)
		c.each(c.fastGate)
		c.each(func() { c.referenceDocs("UX-01 reference docs updated", false) })
		c.each(c.ruleClearance)
		c.declaredStill()
		c.each(c.insideAreas)
	case 4:
		c.each(c.onBranchHead)
		c.each(c.fastGate)
		c.tier2Report()
		c.each(c.ruleClearance)
		c.declaredStill()
		c.each(c.insideAreas)
		c.each(c.trialMerge)
	case 5:
		c.each(func() { c.referenceDocs("guides or examples updated", true) })
		c.declaredStill()
		c.each(c.insideAreas)
	case 6:
		if c.mergedEarly() {
			break
		}
		c.each(c.mainMerged)
		if d, err := c.loadDesign(c.f.Branch); err == nil {
			c.section(d, "Consolidation")
		}
		c.eachOther(c.consolidationNamesMain)
		c.each(c.ruleClearance)
		c.declaredStill()
		c.each(c.insideAreas)
		c.each(c.trialMerge)
		c.archivedOn(c.f.Branch)
		c.each(c.prGreen)
	case 7:
		c.each(c.merged)
	case 8:
		c.closed()
	}
	return c.out
}

// Review runs the done-when check of a feature's design-review step
// (FLOW-19): every review seeded so far, this one included, has its approval
// recorded. The review is not its phase's gate, so Phase does not run it.
func Review(env Env, f *queue.Feature) []Result {
	c := newChecker(env, f)
	if !c.ready() {
		return c.out
	}
	c.reviewed()
	return c.out
}

// Merged reports whether the human has merged the item's pull request, by
// the merge step's own check (FLOW-08). wf stale asks it before it seeds a
// refresh: the human may have merged since the last confirmation, and a
// refresh pushed after that would bring a deleted branch back (FLOW-16).
func Merged(env Env, f *queue.Feature) bool {
	c := newChecker(env, f)
	if !c.ready() {
		return false
	}
	c.merged()
	return OK(c.out)
}

// MergedFirst runs the done-when check of an f6-merged-<n>: the pull
// request of the item's n-th work entry, counted from 1, is merged into that
// repository's trunk, by the merge step's proofs (FLOW-08).
func MergedFirst(env Env, f *queue.Feature, n int) []Result {
	c := newChecker(env, f)
	if !c.ready() {
		return c.out
	}
	if n < 1 || n > len(f.Work) {
		c.add("merged by a human (FLOW-08)", false, "work has no entry %d", n)
		return c.out
	}
	c.repos()
	c.at = c.others[n-1]
	if c.at.err != nil {
		c.add("repository (LOOP-01)", false, "%v", c.at.err)
		return c.out
	}
	c.merged()
	return c.out
}

// Refresh runs the done-when check of a refresh step (FLOW-10): the branch
// contains current main and is pushed, and what Publish's gate checks holds
// again on the branch with main merged: the consolidation entry, the
// rule-change clearance, the queue equal to the design and the diff inside
// the areas. The archive and the PR's CI are
// checked only when a PR is recorded: a feature refreshed in Publish opens
// its PR afterwards, at f6-open-pr. A process item has no consolidation pass
// to record. It does not pass on a merge the human made meanwhile: the
// refresh then pushes nothing, and the merge confirmation drops the step.
func Refresh(env Env, f *queue.Feature) []Result {
	c := newChecker(env, f)
	if !c.ready() {
		return c.out
	}
	c.each(c.mainMerged)
	c.each(c.pushed)
	hasPR := f.PR != nil && f.PR.Number > 0
	if f.Round != "process" {
		c.each(c.consolidationNamesMain)
		c.each(c.ruleClearance)
		c.declaredStill()
		c.each(c.insideAreas)
	}
	if hasPR || f.Round == "process" {
		c.archivedOn(f.Branch)
		c.each(c.prGreen)
	}
	return c.out
}

var commitIDRe = regexp.MustCompile(`\b[0-9a-f]{7,40}\b`)

// consolidationNamesMain checks that the design's Consolidation section
// names the commit of the trunk the branch merged: the refresh's pass over
// what landed is recorded against that commit (FLOW-10). The design is the
// home's, whichever repository's trunk is checked.
func (c *checker) consolidationNamesMain() {
	const name = "consolidation names the main commit merged (FLOW-10)"
	at := c.at
	c.at = c.home
	d, err := c.loadDesign(c.f.Branch)
	c.at = at
	if err != nil {
		c.add(name, false, "%v", err)
		return
	}
	main, err := c.git("rev-parse", c.trunk())
	if err != nil {
		c.add(name, false, "%v", err)
		return
	}
	for _, id := range commitIDRe.FindAllString(d.Sections["Consolidation"], -1) {
		if strings.HasPrefix(main, id) {
			c.add(name, true, "%s %s", c.trunk(), short(main))
			return
		}
	}
	c.add(name, false, "the Consolidation section on %s does not name %s %s",
		c.f.Branch, c.trunk(), short(main))
}

// process runs a process item's done-when checks: x1 design written and
// reviewed, x2 archived on the pushed branch with its PR green (the FLOW-14
// clearance reports it exempt), x3 merged by a human, x4 closed.
func (c *checker) process() {
	switch c.f.Phase {
	case 1:
		_, err := c.git("cat-file", "-e", "refs/heads/"+c.f.Branch+":"+c.f.Design)
		c.add("design present (FLOW-01)", err == nil, "%s on %s", c.f.Design, c.f.Branch)
		c.reviewed()
	case 2:
		if c.mergedEarly() {
			break
		}
		c.pushed()
		c.each(c.ruleClearance)
		c.archivedOn(c.f.Branch)
		c.prGreen()
	case 3:
		c.merged()
	case 4:
		c.closed()
	}
}

// OK reports whether every result passed.
func OK(rs []Result) bool {
	for _, r := range rs {
		if !r.OK {
			return false
		}
	}
	return len(rs) > 0
}

type checker struct {
	env Env
	f   *queue.Feature
	out []Result
	// home is the item's home repository, others its work entries' (repos);
	// at is the one the check being run concerns.
	home, at *repoAt
	others   []*repoAt
}

// add records a result. One about a repository other than the home names
// it.
func (c *checker) add(name string, ok bool, format string, a ...any) {
	if c.at != nil && c.at != c.home {
		name += " in " + c.at.name
	}
	c.out = append(c.out, Result{name, ok, fmt.Sprintf(format, a...)})
}

// eachOther is each without the home.
func (c *checker) eachOther(check func()) {
	c.each(func() {
		if c.at != c.home {
			check()
		}
	})
}

func (c *checker) git(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = c.dir()
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git %s: %v: %s", strings.Join(args, " "), err,
			strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(string(out)), nil
}

// clone returns the checkout of repo: Env.Repo for the queue's own
// repository (or for an item that names none), else its clone under
// Env.Checkouts. A clone that is not there is an error naming -checkouts and
// the path it was looked for at (GLOBAL-11).
func (c *checker) clone(repo string) (string, error) {
	if repo == "" || repo == c.state().Config.Repository {
		return c.env.Repo, nil
	}
	if c.env.Checkouts == "" {
		return "", fmt.Errorf("%s is not the queue's own repository, and no -checkouts "+
			"is given: wf reads it in DIR/%s, given -checkouts DIR", repo, repo)
	}
	dir := filepath.Join(c.env.Checkouts, filepath.FromSlash(repo))
	if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
		return "", fmt.Errorf("no clone of %s at %s (-checkouts %s): clone it there",
			repo, dir, c.env.Checkouts)
	}
	return dir, nil
}

// gitIn runs git in repo's checkout.
func (c *checker) gitIn(repo string, args ...string) (string, error) {
	dir, err := c.clone(repo)
	if err != nil {
		return "", err
	}
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git %s in %s: %v: %s", strings.Join(args, " "), repo, err,
			strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(string(out)), nil
}

// remoteHeadIn returns the commit of branch on repo's remote, "" when the
// branch is not there.
func (c *checker) remoteHeadIn(repo, branch string) (string, error) {
	out, err := c.gitIn(repo, "ls-remote", "--heads", "origin", "refs/heads/"+branch)
	if err != nil || out == "" {
		return "", err
	}
	return strings.Fields(out)[0], nil
}

func (c *checker) remoteHead(branch string) string {
	out, err := c.git("ls-remote", "--heads", "origin", "refs/heads/"+branch)
	if err != nil || out == "" {
		return ""
	}
	return strings.Fields(out)[0]
}

func (c *checker) branchOnRemote() {
	sha := c.remoteHead(c.f.Branch)
	c.add("branch on remote", sha != "", "origin/%s %s", c.f.Branch, sha)
}

func (c *checker) mainMerged() {
	_, err := c.git("merge-base", "--is-ancestor", c.trunk(), c.f.Branch)
	c.add("current main merged", err == nil, "%s is an ancestor of %s", c.trunk(),
		c.f.Branch)
}

func (c *checker) pushed() {
	local, err := c.git("rev-parse", "refs/heads/"+c.f.Branch)
	remote := c.remoteHead(c.f.Branch)
	c.add("branch pushed (FLOW-17)", err == nil && local == remote, "local %s, origin %s",
		local, remote)
}

func (c *checker) onBranchHead() {
	head, _ := c.git("rev-parse", "HEAD")
	branch, _ := c.git("rev-parse", "refs/heads/"+c.f.Branch)
	status, _ := c.git("status", "--porcelain")
	c.add("checkout is the clean branch head", head != "" && head == branch && status == "",
		"HEAD %s, %s %s", head, c.f.Branch, branch)
}

// docsTouched passes when the design declares no surfaces, or the branch
// changes a file under one of the prefixes.
func (c *checker) docsTouched(name string, prefixes ...string) {
	at := c.at
	c.at = c.home
	d, err := c.loadDesign(c.f.Branch)
	c.at = at
	if err != nil {
		c.add(name, false, "%v", err)
		return
	}
	if len(d.Front.Surfaces) == 0 {
		c.add(name, true, "design declares no surfaces")
		return
	}
	files, err := c.git("diff", "--name-only", c.trunk()+"..."+c.f.Branch)
	if err != nil {
		c.add(name, false, "%v", err)
		return
	}
	for _, p := range strings.Fields(files) {
		for _, pre := range prefixes {
			if strings.HasPrefix(p, pre) {
				c.add(name, true, "surfaces %v; %s changed", d.Front.Surfaces, p)
				return
			}
		}
	}
	c.add(name, false, "surfaces %v but nothing under %v changed", d.Front.Surfaces, prefixes)
}

// tier2Report checks that the tier-2 report for the home's branch head is
// written, and that the heads its JSON records are the item's current
// branch heads in every repository it has a branch in: one report covers
// one state of all the item's branches (LINT-04). An item with no work entry
// may have a report without heads, as every report before them.
func (c *checker) tier2Report() {
	sha, err := c.git("rev-parse", "--short=12", "refs/heads/"+c.f.Branch)
	if err != nil {
		c.add("tier-2 report (LINT-04)", false, "%v", err)
		return
	}
	var jsonPath string
	for _, ext := range []string{".md", ".json"} {
		p := filepath.Join(c.env.Workflow, "reports", c.f.Feature, sha+ext)
		_, err := os.Stat(p)
		c.add("tier-2 report "+ext+" (LINT-04, LINT-05)", err == nil, "%s", p)
		if err == nil && ext == ".json" {
			jsonPath = p
		}
	}
	if jsonPath != "" {
		c.reportHeads(jsonPath)
	}
}

// reportHeads compares the report's heads with each repository's branch
// head.
func (c *checker) reportHeads(path string) {
	const name = "tier-2 report heads (LINT-04)"
	var rep struct {
		Heads map[string]string `json:"heads"`
	}
	data, err := os.ReadFile(path)
	if err == nil {
		err = json.Unmarshal(data, &rep)
	}
	if err != nil {
		c.add(name, false, "%s: %v", path, err)
		return
	}
	if rep.Heads == nil && len(c.f.Work) == 0 {
		return
	}
	var stale []string
	c.each(func() {
		head, err := c.git("rev-parse", "refs/heads/"+c.f.Branch)
		got := rep.Heads[c.at.name]
		if err != nil || len(got) < 7 || !strings.HasPrefix(head, got) {
			stale = append(stale, fmt.Sprintf("%s: report %s, branch %s", c.at.name,
				orNone(got), orNone(short(head))))
		}
	})
	if len(stale) > 0 {
		c.add(name, false, "the report's heads are not the item's branch heads: %s",
			strings.Join(stale, "; "))
		return
	}
	c.add(name, true, "every branch head is the one the report read")
}

// prGreen checks that the PR is open with its checks green on the branch's
// pushed head: pr.ci is a result for the commit pr.head records, and says
// nothing about another (FLOW-08).
func (c *checker) prGreen() {
	const name = "PR open with CI green (FLOW-11)"
	pr := c.pr()
	if c.at != c.home && pr == nil {
		// A repository whose branch adds nothing to its trunk gets no pull
		// request; its entry closes with the home's.
		if n, err := c.git("rev-list", "--count", c.trunk()+".."+c.f.Branch); err == nil &&
			n == "0" {
			c.add(name, true, "%s adds no commit to %s: no PR", c.f.Branch, c.trunk())
			return
		}
	}
	if pr == nil || pr.Number == 0 || pr.CI != "green" {
		c.add(name, false, "pr %+v", pr)
		return
	}
	head := c.remoteHead(c.f.Branch)
	c.add(name, pr.HeadIs(head), "pr %+v; origin/%s is at %s, and pr.ci counts only for "+
		"the commit pr.head records (FLOW-08)", *pr, c.f.Branch, orNone(head))
}

// merged confirms the human's merge (FLOW-08). Each branch head still
// around, local or remote, must be contained in the trunk, or, for a
// squash merge, the trunk must carry a commit naming the PR whose tree is
// the head's tree, or which merging the head onto its parent writes
// (FLOW-16; see refMerged). Once GitHub has deleted the branch, the PR's
// commit on the trunk alone is the proof. Each repository the item has a
// branch in is checked against its own trunk (FLOW-08).
func (c *checker) merged() {
	const name = "merged by a human (FLOW-08)"
	var prCommits []string
	pr := c.pr()
	if c.at != c.home && pr == nil {
		// A repository whose branch added nothing got no pull request.
		if n, err := c.git("rev-list", "--count", c.trunk()+".."+c.f.Branch); err == nil &&
			n == "0" {
			c.add(name, true, "%s adds no commit to %s: no PR", c.f.Branch, c.trunk())
			return
		}
	}
	if pr != nil && pr.Number > 0 {
		out, _ := c.git("log", c.trunk(), "--format=%h %s", "-E",
			fmt.Sprintf("--grep=(Merge pull request |\\()#%d([^0-9]|$)", pr.Number))
		if out != "" {
			prCommits = strings.Split(out, "\n")
		}
	}
	var infos []string
	for _, ref := range []string{"refs/heads/" + c.f.Branch,
		"refs/remotes/origin/" + c.f.Branch} {
		if _, err := c.git("rev-parse", "--verify", "--quiet", ref); err != nil {
			continue
		}
		info, ok := c.refMerged(ref, prCommits)
		if !ok {
			c.add(name, false, "%s", info)
			return
		}
		infos = append(infos, info)
	}
	if len(infos) > 0 {
		c.add(name, true, "%s", strings.Join(infos, "; "))
		c.at.merge = c.mergeCommit(prCommits)
		return
	}
	if pr == nil || pr.Number == 0 {
		c.add(name, false, "no %s branch and no PR recorded", c.f.Branch)
		return
	}
	c.add(name, len(prCommits) > 0, "PR #%d on %s: %s", pr.Number, c.trunk(),
		firstLine(strings.Join(prCommits, "\n")))
	if len(prCommits) > 0 {
		c.at.merge = c.mergeCommit(prCommits)
	}
}

// mergeCommit is the commit a confirmed merge is recorded by: the PR's
// commit on the trunk, or else the trunk's head, which contains the branch.
func (c *checker) mergeCommit(prCommits []string) string {
	rev := c.trunk()
	if len(prCommits) > 0 {
		rev = strings.Fields(prCommits[0])[0]
	}
	sha, _ := c.git("rev-parse", rev+"^{commit}")
	return sha
}

// WorkMerges returns, for each of the item's work entries in order, the
// merge commit the merge step's proofs confirm in that repository, or ""
// where they do not hold: what the merge confirmation records as the
// entry's merged (FLOW-08).
func WorkMerges(env Env, f *queue.Feature) []string {
	c := newChecker(env, f)
	out := make([]string, len(f.Work))
	if !c.ready() {
		return out
	}
	for i, r := range c.repos()[1:] {
		if r.err != nil {
			continue
		}
		c.at = r
		c.merged()
		out[i] = r.merge
	}
	return out
}

// refMerged reports whether ref is on the trunk, by the first of three
// proofs that holds: ref is an ancestor of it; a commit in prCommits has
// ref's tree; or merging ref onto the parent of a one-parent commit C in
// prCommits writes C's tree (mergeTreeProof). The last proof covers a squash
// made after main moved, whose tree also holds main's new commits (the
// GLOBAL-11 fallback named in docs/design/wf-squash-after-base-moved.md), and
// a ref older than the head that merged (Decision 23 of
// docs/design/loop-parallel.md). Each proof is what makes deleting a squashed
// branch safe: a head pushed past the squash, or a ref that puts a line
// elsewhere, writes a different tree.
func (c *checker) refMerged(ref string, prCommits []string) (string, bool) {
	if _, err := c.git("merge-base", "--is-ancestor", ref, c.trunk()); err == nil {
		return ref + " is contained in " + c.trunk(), true
	}
	if len(prCommits) == 0 {
		return ref + " is not contained in " + c.trunk() + " and no PR commit is on it",
			false
	}
	tree, _ := c.git("rev-parse", ref+"^{tree}")
	for _, line := range prCommits {
		sha := strings.Fields(line)[0]
		pt, _ := c.git("rev-parse", sha+"^{tree}")
		if pt != "" && pt == tree {
			return fmt.Sprintf("%s squash-merged as %s (tree %s)", ref, line,
				short(tree)), true
		}
	}
	var seen []string
	for _, line := range prCommits {
		sha := strings.Fields(line)[0]
		info, ok := c.mergeTreeProof(ref, sha)
		if ok {
			return fmt.Sprintf("%s squash-merged as %s after main moved (%s)",
				ref, sha, info), true
		}
		seen = append(seen, info)
	}
	drop := "git branch -D " + c.f.Branch
	if c.remoteHead(c.f.Branch) == "" {
		drop += "; git update-ref -d refs/remotes/origin/" + c.f.Branch
	}
	return fmt.Sprintf("%s (tree %s) is not contained in %s, and no PR commit "+
		"has its tree or is its merge: %s; after comparing, a human may delete: %s",
		ref, short(tree), c.trunk(), strings.Join(seen, ", "), drop), false
}

// mergeTreeProof reports whether PR commit sha has one parent and merging ref
// onto that parent, from ref's single merge base B with it, writes exactly
// sha's tree: `git merge-tree --write-tree --merge-base=B sha^ ref`. The merge
// replays what ref adds where it adds it, so a ref holding anything sha does
// not, or a line somewhere else, writes another tree. With no merge base or
// several, the proof does not hold: a chosen base would be a guess
// (GLOBAL-10). Its text names every value compared, or why sha was not
// compared, and the command, so that a human can repeat it at the escalation.
func (c *checker) mergeTreeProof(ref, sha string) (string, bool) {
	pt, _ := c.git("rev-parse", sha+"^{tree}")
	desc := fmt.Sprintf("%s tree %s", sha, short(pt))
	parents, _ := c.git("rev-list", "--parents", "-n", "1", sha)
	if n := len(strings.Fields(parents)) - 1; n != 1 {
		return fmt.Sprintf("%s (%d parents, not compared)", desc, n), false
	}
	bases, _ := c.git("merge-base", "--all", ref, sha+"^")
	switch b := strings.Fields(bases); len(b) {
	case 0:
		return desc + ", no merge base with " + sha + "^", false
	case 1:
	default:
		for i := range b {
			b[i] = short(b[i])
		}
		return fmt.Sprintf("%s, merge bases %s", desc, strings.Join(b, ", ")), false
	}
	base := strings.TrimSpace(bases)
	repeat := fmt.Sprintf("git merge-tree --write-tree --merge-base=%s %s^ %s",
		short(base), sha, ref)
	args := append(c.textMergeOnly(), "merge-tree", "--write-tree", "--no-messages",
		"--merge-base="+base, sha+"^", ref)
	cmd := exec.Command("git", args...)
	cmd.Dir = c.dir()
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	got := firstLine(strings.TrimSpace(string(out)))
	var exit *exec.ExitError
	switch {
	case err == nil && got != "" && got == pt:
		return fmt.Sprintf("merged onto %s^ from base %s writes tree %s", sha,
			short(base), short(pt)), true
	case err == nil:
		return fmt.Sprintf("%s, base %s, the merge writes tree %s (repeat: %s)", desc,
			short(base), short(orNone(got)), repeat), false
	case errors.As(err, &exit) && exit.ExitCode() == 1:
		return fmt.Sprintf("%s, base %s, the merge conflicts (repeat: %s)", desc,
			short(base), repeat), false
	}
	version, _ := c.git("version")
	return fmt.Sprintf("%s, base %s, not compared: %s could not run (%v: %s); "+
		"--merge-base needs git 2.40 or later, and this is %s", desc, short(base),
		repeat, err, firstLine(strings.TrimSpace(stderr.String())), orNone(version)), false
}

// textMerge is the merge driver the trial merge gives every driver name: git
// merge-file is git's own three-way text merge, writes the result to %A and
// exits non-zero on a conflict, as a driver must.
const textMerge = "git merge-file -q %A %O %B"

// textMergeOnly returns the -c options that make the trial merge git's text
// merge of every file, whatever an attribute or the config says. A merge
// attribute can come from the checkout's .gitattributes, from the ref's or
// C^'s tree, from info/attributes or from the global attributes file, and a
// driver such as union resolves a conflict by keeping both sides' or one
// side's lines: the trial merge would then write C's tree although the ref
// changed a line C does not have (F-P1 of the implementation's review). So
// the built-in union, every driver the config defines, and merge.default are
// all made the text merge, and no file is renormalized. A file whose
// attribute is binary, or -merge, still conflicts, which refuses the proof:
// the safe side. Overriding the driver works on every git the proof runs on,
// where reading no attributes (--attr-source) needs git 2.41 and leaves
// info/attributes read.
func (c *checker) textMergeOnly() []string {
	opts := []string{"-c", "merge.renormalize=false", "-c", "merge.default=text",
		"-c", "merge.union.driver=" + textMerge}
	defined, _ := c.git("config", "--name-only", "--get-regexp", `^merge\..*\.driver$`)
	for _, key := range strings.Fields(defined) {
		if strings.EqualFold(key, "merge.union.driver") {
			continue
		}
		opts = append(opts, "-c", key+"="+textMerge)
	}
	return opts
}

func orNone(id string) string {
	if id == "" {
		return "none"
	}
	return id
}

func short(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}

// mergedEarly handles a PR the human merged before Publish's step was
// recorded: GitHub has deleted the branch and main has moved past it, so the
// push, main-merged and PR checks cannot pass any more. The merge supersedes
// them (FLOW-08); what remains is that the design landed archived (FLOW-12).
// It reports nothing and returns false while the PR is not merged.
func (c *checker) mergedEarly() bool {
	probe := &checker{env: c.env, f: c.f, home: c.home, at: c.home, others: c.others}
	probe.merged()
	if !OK(probe.out) {
		return false
	}
	c.out = append(c.out, probe.out...)
	c.archivedOn(c.trunk())
	return true
}

// archivedOn checks that the design is archived with its summary at rev: on
// the branch in Publish, so the archive lands in the feature PR, and on main
// at Close (FLOW-12).
func (c *checker) archivedOn(rev string) {
	d, err := c.loadDesign(rev)
	if err != nil {
		c.add("design archived (FLOW-12)", false, "%v", err)
		return
	}
	c.add("design archived (FLOW-12)", d.Front.Status == "archived",
		"status %q on %s", d.Front.Status, rev)
	c.section(d, "Archive summary")
}

// closed is Close's done-when: merged, archived on the home's trunk, and the
// branch gone from the remote (the host deletes it on merge) and from the
// checkout, in each repository the item has a branch in (FLOW-16).
func (c *checker) closed() {
	c.merged()
	c.archivedOn(c.trunk())
	c.each(c.branchGone)
}

// branchGone checks that the item's branch is deleted on the remote and in
// the checkout (FLOW-16).
func (c *checker) branchGone() {
	c.add("remote branch deleted (FLOW-16)", c.remoteHead(c.f.Branch) == "",
		"origin/%s", c.f.Branch)
	_, err := c.git("rev-parse", "--verify", "--quiet", "refs/heads/"+c.f.Branch)
	c.add("local branch deleted (FLOW-16)", err != nil, "refs/heads/%s", c.f.Branch)
}

func (c *checker) design() {
	d, err := c.loadDesign(c.f.Branch)
	if err != nil {
		c.add("design present (FLOW-01)", false, "%v", err)
		return
	}
	c.add("design present (FLOW-01)", true, "%s on %s", c.f.Design, c.f.Branch)
	p, err := c.profile()
	if err != nil {
		c.add("front-matter (FLOW-02)", false, "%v", err)
	}
	for _, e := range d.Front.Problems(c.f.Feature, p) {
		c.add("front-matter (FLOW-02)", false, "%s", e)
	}
	if d.Front.Alters == nil || c.f.AltersRelationships == nil ||
		*d.Front.Alters != *c.f.AltersRelationships {
		c.add("alters_relationships recorded in queue", false, "design %v, queue %v",
			ptr(d.Front.Alters), ptr(c.f.AltersRelationships))
	}
	c.declaredAtDesign(d)
	for _, s := range []string{"Problem", "Prior art", "Decisions", "Design",
		"Defaults and overrides", "Cross-driver impact", "Validation plan"} {
		c.section(d, s)
	}
	if d.Front.SecurityBoundary != "none" {
		c.section(d, "Security note")
	}
	for _, s := range d.Front.Seams {
		if s == "bootstrap-model" {
			c.section(d, "Bootstrap comparison")
		}
	}
	if len(d.Front.RuleChanges) > 0 {
		c.section(d, "Rule changes")
	}
	c.reviewDecision(d)
	open := c.f.OpenBlocking()
	c.add("no open blocking question (FLOW-07)", len(open) == 0, "%d open", len(open))
}

var ruleRe = regexp.MustCompile(`\b[A-Z]+-\d{2}\b`)

func (c *checker) plan() {
	d, err := c.loadDesign(c.f.Branch)
	if err != nil {
		c.add("implementation plan", false, "%v", err)
		return
	}
	items := listItems(d.Sections["Implementation plan"])
	c.add("implementation plan has tasks", len(items) > 0, "%d tasks", len(items))
	for i, it := range items {
		if !ruleRe.MatchString(it) {
			c.add("task cites a rule", false, "task %d: %q", i+1, firstLine(it))
		}
	}
}

func (c *checker) section(d *Design, name string) {
	body := clean(d.Sections[name])
	ok := body != "" && body != clean(d.template[name])
	c.add("section filled: "+name, ok, "%d chars", len(body))
}

// Design is a parsed design document.
type Design struct {
	Front    Front
	Sections map[string]string
	template map[string]string
}

// Front is the FLOW-02 front-matter.
type Front struct {
	Feature      string `yaml:"feature"`
	Status       string `yaml:"status"`
	DesignReview string `yaml:"design_review"`
	// Repos are the repositories the item works in, its home first; empty
	// or absent, only the one that holds the design (LOOP-04).
	Repos []string `yaml:"repos"`
	// MergeFirst are the repositories in Repos, not the home, whose pull
	// request merges before the home's is listed ready (FLOW-08).
	MergeFirst []string `yaml:"merge_first"`
	// Areas are the areas of the area map the change edits (FLOW-02). Nil,
	// the key absent, the design is undeclared, which an empty list is not.
	Areas            *[]string `yaml:"areas"`
	Drivers          []string  `yaml:"drivers"`
	Seams            []string  `yaml:"seams"`
	Surfaces         []string  `yaml:"surfaces"`
	SecurityBoundary string    `yaml:"security_boundary"`
	Alters           *bool     `yaml:"alters_relationships"`
	NativeValidation []struct {
		Host     string `yaml:"host"`
		Blocking *bool  `yaml:"blocking"`
		Reason   string `yaml:"reason"`
	} `yaml:"native_validation"`
	Tier2Gaps []any    `yaml:"tier2_gaps"`
	Rules     []string `yaml:"rules"`
	// RuleChanges is optional: empty or absent means the feature changes no
	// rule (FLOW-14).
	RuleChanges []RuleChange `yaml:"rule_changes"`
	Concepts    []string     `yaml:"concepts"`
	present     map[string]bool
	// raw is the front-matter as parsed, for the values a profile checks.
	raw map[string]any
}

// loopKeys are the front-matter keys every design carries, whatever its
// component (FLOW-02, LOOP-08).
var loopKeys = []string{"feature", "status", "seams", "alters_relationships",
	"design_review", "native_validation", "rules", "concepts"}

// keyOrder is the order Problems reports missing keys in, so that a
// component's added keys fall where they always have here.
var keyOrder = []string{"feature", "status", "drivers", "seams", "surfaces",
	"security_boundary", "alters_relationships", "design_review", "native_validation",
	"tier2_gaps", "rules", "concepts"}

// Problems lists the ways the front-matter breaks FLOW-02 for the named
// feature: a missing key, or a value outside the allowed set. The keys are
// the loop's and those the home's profile file adds, with the values it
// gives (a17 of docs/design/loop-repository.md); a nil profile adds none. It
// is empty when the front-matter is valid.
func (fm *Front) Problems(feature string, p *Profile) []string {
	var impact []string
	var values map[string][]string
	var k classifier
	if p != nil {
		impact, values = p.Impact.Keys, p.Impact.Values
		k = classifier{paths: p.Rules.Paths, machinery: p.Rules.Machinery}
	}
	required := func(k string) bool {
		return slices.Contains(loopKeys, k) || slices.Contains(impact, k)
	}
	out := fm.missing(required, impact)
	if fm.Feature != feature {
		out = append(out, fmt.Sprintf("feature %q, queue says %q", fm.Feature, feature))
	}
	if !oneOf(fm.Status, "draft", "active", "archived") {
		out = append(out, fmt.Sprintf("status %q", fm.Status))
	}
	for _, k := range sortedKeys(values) {
		if v := fm.scalar(k); required(k) && !slices.Contains(values[k], v) {
			out = append(out, fmt.Sprintf("%s %q", k, v))
		}
	}
	if len(fm.Rules) == 0 {
		out = append(out, "rules is empty")
	}
	if len(fm.Concepts) == 0 {
		out = append(out, "concepts is empty")
	}
	for _, nv := range fm.NativeValidation {
		if nv.Host == "" || nv.Blocking == nil || nv.Reason == "" {
			out = append(out, fmt.Sprintf(
				"native_validation %+v needs host, blocking and reason", nv))
		}
	}
	out = append(out, fm.repoProblems()...)
	return append(out, ruleChangeProblems(fm.RuleChanges, k)...)
}

// missing lists the required keys the front-matter lacks, in keyOrder and
// then the order the profile adds them.
func (fm *Front) missing(required func(string) bool, impact []string) []string {
	keys := slices.DeleteFunc(slices.Clone(keyOrder), func(k string) bool {
		return !required(k)
	})
	for _, k := range impact {
		if !slices.Contains(keys, k) {
			keys = append(keys, k)
		}
	}
	var out []string
	for _, k := range keys {
		// design_review decides a gate before implementation (FLOW-19); an
		// archived design is past it, and designs archived before FLOW-19
		// never had the key.
		if !fm.present[k] && (k != "design_review" || fm.Status != "archived") {
			out = append(out, "missing "+k)
		}
	}
	return out
}

// scalar is the front-matter value of key as text, "" when it is absent.
func (fm *Front) scalar(key string) string {
	if v, ok := fm.raw[key]; ok && v != nil {
		return fmt.Sprint(v)
	}
	return ""
}

// repoProblems checks merge_first and the prefixed areas against repos: each
// names a repository the item works in other than its home (FLOW-02).
func (fm *Front) repoProblems() []string {
	var out []string
	others := fm.Repos[min(1, len(fm.Repos)):]
	for _, r := range fm.MergeFirst {
		if !slices.Contains(others, r) {
			out = append(out, fmt.Sprintf(
				"merge_first %s is not in repos after the home", r))
		}
	}
	if fm.Areas == nil {
		return out
	}
	for _, a := range *fm.Areas {
		if repo, _, ok := strings.Cut(a, ":"); ok && !slices.Contains(others, repo) {
			out = append(out, fmt.Sprintf(
				"area %s names %s, which is not in repos after the home", a, repo))
		}
	}
	return out
}

func (c *checker) loadDesign(rev string) (*Design, error) {
	if c.f.Design == "" {
		return nil, fmt.Errorf("queue names no design path")
	}
	text, err := c.git("show", rev+":"+c.f.Design)
	if err != nil {
		return nil, err
	}
	tmpl, err := c.git("show", rev+":docs/design/TEMPLATE.md")
	if err != nil {
		return nil, err
	}
	return ParseDesign(text, tmpl)
}

// ParseDesign parses a design document and the template it was written from.
func ParseDesign(text, tmpl string) (*Design, error) {
	front, body, err := splitFront(text)
	if err != nil {
		return nil, err
	}
	d := &Design{Sections: sections(body)}
	if err := yaml.Unmarshal([]byte(front), &d.Front); err != nil {
		return nil, fmt.Errorf("front-matter: %w", err)
	}
	keys := map[string]any{}
	_ = yaml.Unmarshal([]byte(front), &keys)
	d.Front.present, d.Front.raw = map[string]bool{}, keys
	for k := range keys {
		d.Front.present[k] = true
	}
	if _, tbody, err := splitFront(tmpl); err == nil {
		d.template = sections(tbody)
	}
	return d, nil
}

func splitFront(text string) (string, string, error) {
	if !strings.HasPrefix(text, "---\n") {
		return "", "", fmt.Errorf("no front-matter")
	}
	rest := text[4:]
	end := strings.Index(rest, "\n---\n")
	if end < 0 {
		return "", "", fmt.Errorf("front-matter not closed")
	}
	return rest[:end], rest[end+5:], nil
}

func sections(body string) map[string]string {
	out := map[string]string{}
	var name string
	var buf []string
	flush := func() {
		if name != "" {
			out[name] = strings.Join(buf, "\n")
		}
	}
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, "## ") {
			flush()
			name, buf = strings.TrimSpace(line[3:]), nil
			continue
		}
		buf = append(buf, line)
	}
	flush()
	return out
}

var commentRe = regexp.MustCompile(`(?s)<!--.*?-->`)

func clean(s string) string {
	return strings.Join(strings.Fields(commentRe.ReplaceAllString(s, "")), " ")
}

var itemRe = regexp.MustCompile(`^(\d+\.|-|\*) `)

// listItems returns the top-level list items of a section, each with its
// continuation lines.
func listItems(s string) []string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		switch {
		case itemRe.MatchString(line):
			out = append(out, line)
		case len(out) > 0 && strings.HasPrefix(line, " "):
			out[len(out)-1] += "\n" + line
		}
	}
	return out
}

func firstLine(s string) string { return strings.SplitN(s, "\n", 2)[0] }

func oneOf(s string, opts ...string) bool {
	for _, o := range opts {
		if s == o {
			return true
		}
	}
	return false
}

func ptr(b *bool) string {
	if b == nil {
		return "unset"
	}
	return fmt.Sprint(*b)
}
