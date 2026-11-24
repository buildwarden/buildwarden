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
	"strconv"
	"strings"

	"github.com/buildwarden/buildwarden/tools/workflow/queue"
)

// GitHub is what the merge gate reads from GitHub: a pull request's base and
// head, the check runs on a commit, and the checks a branch's ruleset
// requires. Env.GitHub nil reads them with the gh command.
type GitHub interface {
	PR(repo string, number int) (PRView, error)
	CheckRuns(repo, sha string) ([]CheckRun, error)
	Required(repo, branch string) ([]string, error)
}

// PRView is a pull request as GitHub has it: its base branch, its head
// commit and its head branch.
type PRView struct {
	Base, Head, Branch string
}

// CheckRun is one check run on a commit.
type CheckRun struct {
	Name, Status, Conclusion string
}

// ghCLI reads GitHub with the gh command.
type ghCLI struct{}

func (ghCLI) run(args ...string) (string, error) {
	out, err := exec.Command("gh", args...).Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return "", fmt.Errorf("gh %s: %v: %s", strings.Join(args, " "), err,
				strings.TrimSpace(string(ee.Stderr)))
		}
		return "", fmt.Errorf("gh %s: %w", strings.Join(args, " "), err)
	}
	return strings.TrimSpace(string(out)), nil
}

func (g ghCLI) PR(repo string, number int) (PRView, error) {
	out, err := g.run("pr", "view", strconv.Itoa(number), "-R", repo, "--json",
		"baseRefName,headRefOid,headRefName", "--jq",
		`.baseRefName + " " + .headRefOid + " " + .headRefName`)
	if err != nil {
		return PRView{}, err
	}
	f := strings.Fields(out)
	if len(f) != 3 {
		return PRView{}, fmt.Errorf("gh pr view %d -R %s: %q", number, repo, out)
	}
	return PRView{Base: f[0], Head: f[1], Branch: f[2]}, nil
}

func (g ghCLI) CheckRuns(repo, sha string) ([]CheckRun, error) {
	out, err := g.run("api", "--paginate", "repos/"+repo+"/commits/"+sha+"/check-runs",
		"--jq", `.check_runs[] | [.name, .status, (.conclusion // "")] | @tsv`)
	if err != nil {
		return nil, err
	}
	var rs []CheckRun
	for _, l := range strings.Split(out, "\n") {
		if f := strings.Split(l, "\t"); len(f) == 3 {
			rs = append(rs, CheckRun{Name: f[0], Status: f[1], Conclusion: f[2]})
		}
	}
	return rs, nil
}

func (g ghCLI) Required(repo, branch string) ([]string, error) {
	out, err := g.run("api", "repos/"+repo+"/rules/branches/"+branch, "--jq",
		`.[] | select(.type == "required_status_checks") | `+
			`.parameters.required_status_checks[].context`)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, l := range strings.Split(out, "\n") {
		if l = strings.TrimSpace(l); l != "" && !slices.Contains(names, l) {
			names = append(names, l)
		}
	}
	return names, nil
}

func (e Env) github() GitHub {
	if e.GitHub != nil {
		return e.GitHub
	}
	return ghCLI{}
}

// protectedRe matches the rules a tier-2 MUST finding is never deferred on
// and never merged over (Decision 10 of the program-north-star design):
// GLOBAL-06, the SEC rules, RELAY-01 to RELAY-03 and RELAY-05, and FLOW-20,
// whose never-break list (the ledger wire format v1, the reader's support for
// every released ledger, a written schema $id, every security boundary) a
// finding cites by FLOW-20 where RELAY-05 or GLOBAL-06 does not cover it.
var protectedRe = regexp.MustCompile(`^(GLOBAL-06|SEC-[0-9]+|RELAY-0[1235]|FLOW-20)$`)

// MergeReady is the merge gate of a pull request into a program trunk
// (FLOW-08, FLOW-20): it checks the item's pull request in repo against
// TrunkFor, and each failure names its point. The loop merges only while
// every point holds, on the head point 2 read, with --match-head-commit, at
// once after the gate passes under its claim: point 4 fetches the program
// trunk and reads its head as origin has it then.
func MergeReady(env Env, f *queue.Feature, repo string) []Result {
	c := newChecker(env, f)
	if !c.ready() {
		return c.out
	}
	r := c.repoNamed(repo)
	if r == nil {
		c.add("merge gate (FLOW-08)", false, "%s has no branch in %s", f.Feature, repo)
		return c.out
	}
	if r.err != nil {
		c.add("repository (LOOP-01)", false, "%v", r.err)
		return c.out
	}
	c.at = r
	view, ok := c.gateHead()
	if !ok {
		return c.out
	}
	c.gateChecks(view.Head)
	c.gateContains(view.Head)
	c.gateReport(view.Head)
	c.gateHuman()
	c.gateSecurity(view.Head)
	c.gateMergedFirst()
	c.ruleClearance()
	c.at = c.home
	return c.out
}

// MergeWaitsOnHuman reports whether the item's loop merge in repo is held on
// the human, read on its recorded pr.head: at point 7 on a posture change he
// has not acknowledged (no security-ack, and not the relay clause's two
// security-note records), or at point 5 on a protected finding recorded on
// that head, which only he may dismiss (FLOW-05, FLOW-20, Decision 10). Such a
// merge counts toward the waiting limit and shows under Needs Jeff.
func MergeWaitsOnHuman(env Env, f *queue.Feature, repo string) bool {
	c := newChecker(env, f)
	if !c.ready() {
		return false
	}
	r := c.repoNamed(repo)
	if r == nil || r.err != nil {
		return false
	}
	c.at = r
	head, ok := c.recordedHead()
	if !ok {
		return false
	}
	records, err := queue.LoadRecords(env.Workflow)
	if err != nil {
		return false
	}
	atHead := func(r queue.Record) bool {
		return r.Feature == f.Feature && len(r.Head) >= 7 &&
			strings.HasPrefix(head, r.Head)
	}
	if slices.ContainsFunc(records, func(r queue.Record) bool {
		return r.Kind == "protected" && atHead(r)
	}) {
		return true
	}
	return c.postureWaits(records, atHead, head)
}

// recordedHead is the PR's recorded pr.head in the repository being
// checked, resolved to the full commit where the checkout has it.
func (c *checker) recordedHead() (string, bool) {
	pr := c.pr()
	if pr == nil || len(pr.Head) < 7 {
		return "", false
	}
	if full, err := c.git("rev-parse", "--verify", "-q", pr.Head+"^{commit}"); err == nil {
		return full, true
	}
	return pr.Head, true
}

// postureWaits reports whether point 7 holds the merge on the human: the
// security boundary is not none, he has not acknowledged its note on the
// head, and the relay clause's two security-note records are not both there.
func (c *checker) postureWaits(records []queue.Record, atHead func(queue.Record) bool,
	head string) bool {
	at := c.at
	c.at = c.home
	d, err := c.loadDesign(c.f.Branch)
	c.at = at
	if err != nil || d.Front.SecurityBoundary == "none" {
		return false
	}
	if slices.ContainsFunc(records, func(rec queue.Record) bool {
		return securityAck(rec, c.at.name) && atHead(rec)
	}) {
		return false
	}
	return len(c.relayClauseOnly(records, atHead, head)) > 0
}

// repoNamed is the item's repository of that name, the home for "".
func (c *checker) repoNamed(repo string) *repoAt {
	for _, r := range c.repos() {
		if repo == "" || r.name == repo {
			return r
		}
	}
	return nil
}

// gateHead checks points 1 and 2: the PR's base is the item's program trunk,
// and its head is the recorded pr.head and the remote branch's head.
func (c *checker) gateHead() (PRView, bool) {
	pr := c.pr()
	pt := c.state().Config.ProgramTrunkOf(c.at.name)
	if pr == nil || pr.Number == 0 {
		c.add("merge gate 1: base is the program trunk (FLOW-08)", false,
			"no PR recorded in %s", c.at.name)
		return PRView{}, false
	}
	view, err := c.env.github().PR(c.at.name, pr.Number)
	if err != nil {
		c.add("merge gate 1: base is the program trunk (FLOW-08)", false, "%v", err)
		return view, false
	}
	base := pt != "" && view.Base == pt && c.at.against == pt
	c.add("merge gate 1: base is the program trunk (FLOW-08)", base,
		"PR #%d's base %s; the item's trunk %s, program trunk %s", pr.Number, view.Base,
		c.at.against, orNone(pt))
	remote := c.remoteHead(c.f.Branch)
	head := pr.HeadIs(view.Head) && remote == view.Head
	c.add("merge gate 2: head is the recorded and the remote head (FLOW-08)", head,
		"PR head %s, pr.head %s, origin/%s %s", short(view.Head), orNone(pr.Head),
		c.f.Branch, orNone(short(remote)))
	return view, base && head
}

// passed are the conclusions a check run may end with for the gate.
var passed = []string{"success", "skipped", "neutral"}

// gateChecks checks point 3: every check run on the head completed with
// success, skipped or neutral, and each check the program trunk's ruleset
// requires is among them and succeeded. The required list is read from the
// ruleset and never assumed: a ruleset that requires none refuses the merge.
func (c *checker) gateChecks(head string) {
	const name = "merge gate 3: checks passed (FLOW-08)"
	gh := c.env.github()
	runs, err := gh.CheckRuns(c.at.name, head)
	if err != nil {
		c.add(name, false, "%v", err)
		return
	}
	required, err := gh.Required(c.at.name, c.at.against)
	if err != nil {
		c.add(name, false, "%v", err)
		return
	}
	if len(required) == 0 {
		c.add(name, false, "the ruleset of %s requires no check: no loop merge until "+
			"it does (setup, kind 1)", c.at.against)
		return
	}
	var bad []string
	for _, r := range runs {
		if r.Status != "completed" || !slices.Contains(passed, r.Conclusion) {
			bad = append(bad, fmt.Sprintf("%s %s/%s", r.Name, r.Status,
				orNone(r.Conclusion)))
		}
	}
	for _, want := range required {
		i := slices.IndexFunc(runs, func(r CheckRun) bool {
			return r.Name == want && r.Status == "completed" &&
				r.Conclusion == "success"
		})
		if i < 0 {
			bad = append(bad, want+" required and not succeeded")
		}
	}
	c.add(name, len(bad) == 0, "%s", orText(strings.Join(bad, "; "),
		fmt.Sprintf("%d runs on %s; required %s", len(runs), short(head),
			strings.Join(required, ", "))))
}

// gateContains checks point 4: the head contains the program trunk's head,
// fetched first, so that under the loop merge's claim the gate reads the
// program trunk as it is when the loop merges (FLOW-08). Another loop merge
// in the repository is not taken meanwhile (wf next's serialization), and a
// head that moved anyway fails the merge pinned to the head.
func (c *checker) gateContains(head string) {
	const name = "merge gate 4: the branch contains the program trunk (FLOW-08)"
	ref := c.at.against
	if _, err := c.git("fetch", "-q", "origin",
		"+refs/heads/"+ref+":refs/remotes/origin/"+ref); err != nil {
		c.add(name, false, "the program trunk not fetched: %v", err)
		return
	}
	_, err := c.git("merge-base", "--is-ancestor", c.trunk(), head)
	c.add(name, err == nil, "%s is an ancestor of %s", c.trunk(), short(head))
}

// gateReport checks point 5: the tier-2 report is for the head, and no MUST
// finding is open but one the records defer; a protected one, by its rule or
// by a protected record, is never deferred, and sends the item back to
// Design (Decision 10).
func (c *checker) gateReport(head string) {
	const name = "merge gate 5: tier-2 report, no open MUST (FLOW-08, LINT-03)"
	at := c.at
	c.at = c.home
	home, err := c.git("rev-parse", "--short=12", "refs/remotes/origin/"+c.f.Branch)
	c.at = at
	if err != nil {
		c.add(name, false, "%v", err)
		return
	}
	p := filepath.Join(c.env.Workflow, "reports", c.f.Feature, home+".json")
	rep, err := readReport(p)
	if err != nil {
		c.add(name, false, "%v", err)
		return
	}
	got := rep.Heads[c.at.name]
	if got == "" && c.at == c.home {
		got = rep.Head
	}
	if len(got) < 7 || !strings.HasPrefix(head, got) {
		c.add(name, false, "%s read %s, not the PR head %s", p, orNone(got), short(head))
		return
	}
	records, err := queue.LoadRecords(c.env.Workflow)
	if err != nil {
		c.add(name, false, "%v", err)
		return
	}
	open := rep.openMust(records, c.f.Feature, head)
	c.add(name, len(open) == 0, "%s", orText(strings.Join(open, "; "),
		"no open MUST finding but those deferred, in "+p))
}

// tier2Report is what the merge gate reads of a tier-2 report.
type tier2Report struct {
	Head     string            `json:"head"`
	Heads    map[string]string `json:"heads"`
	Findings []struct {
		ID       any    `json:"id"`
		Rule     string `json:"rule"`
		Severity string `json:"severity"`
		Status   string `json:"status"`
	} `json:"findings"`
}

// readReport reads the tier-2 report at p.
func readReport(p string) (tier2Report, error) {
	var rep tier2Report
	data, err := os.ReadFile(p)
	if err == nil {
		err = json.Unmarshal(data, &rep)
	}
	if err != nil {
		return rep, fmt.Errorf("%s: %w", p, err)
	}
	return rep, nil
}

// resolved are the statuses that close a finding; any other, or none, leaves
// it open: point 5 fails closed.
var resolved = []string{"fixed", "resolved", "suppressed"}

// openMust lists the report's MUST findings (a severity starting MUST, in
// any case, so MUST-review too) that are open, by any status but a resolved
// one, and that the records do not defer on the head. One under a protected
// rule, or with a protected record for the item, is never deferred.
func (rep tier2Report) openMust(records []queue.Record, feature, head string) []string {
	var open []string
	for _, f := range rep.Findings {
		if !strings.HasPrefix(strings.ToUpper(strings.TrimSpace(f.Severity)), "MUST") {
			continue
		}
		word, _, _ := strings.Cut(strings.ToLower(strings.TrimSpace(f.Status)), " ")
		if slices.Contains(resolved, strings.TrimRight(word, ".,;:")) {
			continue
		}
		id := fmt.Sprint(f.ID)
		switch {
		case protectedRe.MatchString(f.Rule) || isProtected(records, feature, id):
			open = append(open, fmt.Sprintf("%s (%s) is protected: back to Design "+
				"(FLOW-06)", id, f.Rule))
		case !deferred(records, feature, id, head):
			open = append(open, fmt.Sprintf("%s (%s) open and not deferred", id,
				f.Rule))
		}
	}
	return open
}

// isProtected reports whether a protected record names the item's finding:
// no deferred record lets it merge then (Decision 10).
func isProtected(rs []queue.Record, feature, id string) bool {
	return slices.ContainsFunc(rs, func(r queue.Record) bool {
		return r.Kind == "protected" && r.Feature == feature && r.Ref == id
	})
}

// deferred reports whether the records defer the finding on the head.
func deferred(rs []queue.Record, feature, id, head string) bool {
	return slices.ContainsFunc(rs, func(r queue.Record) bool {
		return r.Kind == "deferred" && r.Feature == feature && r.Ref == id &&
			len(r.Head) >= 7 && strings.HasPrefix(head, r.Head)
	})
}

// gateHuman checks point 6: no human step of the item is pending: no open
// blocking question, no human step and no host step (blocking native
// validation, FLOW-09) of a phase the item has reached.
func (c *checker) gateHuman() {
	const name = "merge gate 6: no human step pending (FLOW-08)"
	var waits []string
	for _, q := range c.f.OpenBlocking() {
		waits = append(waits, "blocking question "+q.ID)
	}
	for _, st := range c.f.Steps {
		if st.Status == "pending" && st.Phase <= c.f.Phase &&
			(st.Affinity == "human" || strings.HasPrefix(st.Affinity, "host:")) {
			waits = append(waits, st.Affinity+" step "+st.ID)
		}
	}
	c.add(name, len(waits) == 0, "%s", orText(strings.Join(waits, "; "),
		"none pending"))
}

// gateSecurity checks point 7: security_boundary is none; or its only trip is
// GLOBAL-06's relay clause with no posture change, which its review accepted
// (a security-note record with ref design, from U4 or the design review) and
// Review found the diff at this head stays within (a security-note record
// with ref diff on the head) (a14 (a)); or the human has acknowledged the
// security note for this head (a security-ack record): a posture change is
// component design and waits on the human (kind 2).
func (c *checker) gateSecurity(head string) {
	const name = "merge gate 7: security boundary (FLOW-08, GLOBAL-06)"
	at := c.at
	c.at = c.home
	d, err := c.loadDesign(c.f.Branch)
	c.at = at
	if err != nil {
		c.add(name, false, "%v", err)
		return
	}
	b := d.Front.SecurityBoundary
	if b == "none" {
		c.add(name, true, "security_boundary: none")
		return
	}
	records, err := queue.LoadRecords(c.env.Workflow)
	if err != nil {
		c.add(name, false, "%v", err)
		return
	}
	atHead := func(r queue.Record) bool {
		return r.Feature == c.f.Feature && len(r.Head) >= 7 &&
			strings.HasPrefix(head, r.Head)
	}
	if slices.ContainsFunc(records, func(r queue.Record) bool {
		return securityAck(r, c.at.name) && atHead(r)
	}) {
		c.add(name, true, "security_boundary: %s: the human acknowledged the security "+
			"note at %s", b, short(head))
		return
	}
	missing := c.relayClauseOnly(records, atHead, head)
	if len(missing) == 0 {
		c.add(name, true, "security_boundary: %s: only GLOBAL-06's relay clause, no "+
			"posture change: accepted at the design's review, and the diff at %s "+
			"stays within it (a14); acknowledged by the landing merge", b, short(head))
		return
	}
	c.add(name, false, "security_boundary: %s: %s; a posture change waits on the "+
		"human's acknowledgment of the security note at %s (kind 2)", orNone(b),
		strings.Join(missing, "; "), short(head))
}

// securityAck reports whether r is the human's acknowledgment of a security
// note in repo: written by a human worker, for the repository or none named.
// The records are a plain file, so who wrote one is checked here too.
func securityAck(r queue.Record, repo string) bool {
	return r.Kind == "security-ack" && queue.Kind(r.By) == "human" &&
		(r.Repo == "" || r.Repo == repo)
}

// relayClauseOnly lists what is missing for point 7's relay-clause case
// (a14 (a)): the security-note record of the design's review, and Review's
// on the head.
func (c *checker) relayClauseOnly(records []queue.Record, atHead func(queue.Record) bool,
	head string) []string {
	var missing []string
	if !slices.ContainsFunc(records, func(r queue.Record) bool {
		return r.Kind == "security-note" && r.Feature == c.f.Feature &&
			r.Ref == queue.SecurityNoteDesign
	}) {
		missing = append(missing, "no security-note -ref design record (U4 or the "+
			"design review)")
	}
	if !slices.ContainsFunc(records, func(r queue.Record) bool {
		return r.Kind == "security-note" && r.Ref == queue.SecurityNoteDiff && atHead(r)
	}) {
		missing = append(missing, fmt.Sprintf("no security-note -ref diff record on %s "+
			"(Review)", short(head)))
	}
	return missing
}

// gateMergedFirst checks point 8: every pull request the design names to
// merge first has merged.
func (c *checker) gateMergedFirst() {
	const name = "merge gate 8: merged-first PRs merged (FLOW-08)"
	var open []string
	for _, w := range c.f.Work {
		if w.First && w.Merged == "" && (c.at.work == nil || c.at.work.Repo != w.Repo) {
			open = append(open, w.Repo)
		}
	}
	c.add(name, len(open) == 0, "%s", orText(strings.Join(open, ", "),
		"none outstanding"))
}

// syncBranchRe is the program sync's branch, sync/program-<main sha>, as wf
// sync-program names it.
var syncBranchRe = regexp.MustCompile(`^sync/program-([0-9a-f]{7,40})$`)

// SyncReportDir is where the tier-2 report of a program sync's conflict
// resolution is written on the workflow branch: reports/sync/<head12>.json,
// <head12> the sync head's 12-character short hash.
const SyncReportDir = "reports/sync"

// SyncHead is the program sync's head as wf sync-program -head checks it
// before recording it: a merge commit whose first parent is
// origin/<program trunk>, whose second is a commit of the trunk, and that
// carries nothing else. It returns the branch the head names,
// sync/program-<second parent, 12>, and whether the merge's tree is the clean
// merge's, or why the head is not a sync head.
func SyncHead(env Env, repo, head string) (branch string, clean bool, err error) {
	c := newChecker(env, &queue.Feature{Repos: []string{repo}})
	if !c.ready() {
		return "", false, errors.New(c.out[0].Info)
	}
	main, clean, err := c.syncHead(head)
	if err != nil {
		return "", false, err
	}
	return "sync/program-" + main[:12], clean, nil
}

// syncHead returns the trunk commit the sync head merges, its second
// parent, and whether the merge is clean, or why it is not a sync head. It
// reads the shape as SyncShape does (syncParents), with origin/<program
// trunk> as the base, and is stricter: the first parent is that head itself.
func (c *checker) syncHead(head string) (string, bool, error) {
	pt := c.state().Config.ProgramTrunkOf(c.at.name)
	if pt == "" {
		return "", false, fmt.Errorf("config.yaml records no program trunk for %s",
			c.at.name)
	}
	ptHead, err := c.git("rev-parse", "origin/"+pt)
	if err != nil {
		return "", false, err
	}
	ps, why, err := syncParents(c.dir(), "origin/"+pt, head, "origin/"+c.at.trunk)
	if err != nil {
		return "", false, err
	}
	if ps == nil {
		return "", false, errors.New(why[0])
	}
	if ps[0] != ptHead {
		return "", false, fmt.Errorf("%s's first parent %s is not origin/%s %s: the "+
			"head carries more than the sync's merge, or the program trunk moved",
			short(head), short(ps[0]), pt, short(ptHead))
	}
	if len(why) > 0 {
		return "", false, errors.New(why[0])
	}
	return ps[1], c.cleanMerge(head, ps[0], ps[1]), nil
}

// cleanMerge reports whether head's tree is the one git's own merge of the
// two parents writes, with no conflict: a merge with no resolution in it. A
// git that cannot say is read as a resolution, which needs its review.
func (c *checker) cleanMerge(head, a, b string) bool {
	res, err := resolution(c.dir(), head, a, b)
	return err == nil && len(res) == 0
}

// NotSyncError says why a head is not a program sync's: each condition of
// SyncShape it fails.
type NotSyncError struct {
	Why []string
}

func (e *NotSyncError) Error() string { return strings.Join(e.Why, "; ") }

// SyncShape is a program sync's pull request as CI recognizes it (FLOW-01,
// FLOW-20): head, on branch sync/program-<hex>, is a merge of exactly two
// parents, the first an ancestor of base (the pull request's base commit on
// the program trunk), the second an ancestor of trunk whose hash starts with
// <hex>. repo is the checkout. It returns the resolution: each path where
// head's tree differs from git's own merge of the two parents, and each
// path that merge leaves conflicted, sorted. A head of another shape is a
// *NotSyncError naming every condition it fails; any other error is git's,
// and says nothing about the shape.
func SyncShape(repo, base, branch, head, trunk string) ([]string, error) {
	m := syncBranchRe.FindStringSubmatch(branch)
	var why []string
	if m == nil {
		why = append(why, fmt.Sprintf("the branch %s is not sync/program-<sha>", branch))
	}
	ps, more, err := syncParents(repo, base, head, trunk)
	if err != nil {
		return nil, err
	}
	why = append(why, more...)
	if ps != nil && m != nil && !strings.HasPrefix(ps[1], m[1]) {
		why = append(why, fmt.Sprintf("the second parent %s does not start with the "+
			"branch's %s", short(ps[1]), m[1]))
	}
	if len(why) > 0 {
		return nil, &NotSyncError{Why: why}
	}
	return resolution(repo, head, ps[0], ps[1])
}

// syncParents reads the shape SyncShape and syncHead share: head's two
// parents, the first an ancestor of base and the second of trunk. It returns
// the parents, nil when head is not a merge of two, and each condition that
// fails; an error is git's.
func syncParents(dir, base, head, trunk string) ([]string, []string, error) {
	out, err := gitAt(dir, "rev-list", "--parents", "-n", "1", head)
	if err != nil {
		return nil, nil, err
	}
	ps := strings.Fields(out)[1:]
	if len(ps) != 2 {
		return nil, []string{fmt.Sprintf("%s is not a merge of two commits", short(head))},
			nil
	}
	var why []string
	for _, p := range []struct{ commit, ref, which string }{
		{ps[0], base, "first"}, {ps[1], trunk, "second"},
	} {
		in, err := ancestorAt(dir, p.commit, p.ref)
		if err != nil {
			return nil, nil, err
		}
		if !in {
			why = append(why, fmt.Sprintf("%s's %s parent %s is not in %s", short(head),
				p.which, short(p.commit), p.ref))
		}
	}
	return ps, why, nil
}

// resolution returns the paths where head's tree differs from git's own
// merge of a and b, with each path that merge leaves conflicted, sorted.
func resolution(dir, head, a, b string) ([]string, error) {
	tree, conflicted, err := mergeTree(dir, a, b)
	if err != nil {
		return nil, err
	}
	diff, err := gitAt(dir, "diff", "--name-only", "--no-renames", "-z", tree, head)
	if err != nil {
		return nil, err
	}
	paths := conflicted
	for _, p := range strings.Split(diff, "\x00") {
		if p != "" && !slices.Contains(paths, p) {
			paths = append(paths, p)
		}
	}
	slices.Sort(paths)
	return paths, nil
}

// mergeTree is git's own merge of a and b: the tree it writes, conflict
// markers and all, and the paths it leaves conflicted.
func mergeTree(dir, a, b string) (string, []string, error) {
	cmd := exec.Command("git", "merge-tree", "--write-tree", "--name-only",
		"--no-messages", a, b)
	cmd.Dir = dir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	// Exit status 1 is a merge with conflicts: the tree, then the files.
	var exit *exec.ExitError
	if err != nil && (!errors.As(err, &exit) || exit.ExitCode() != 1) || lines[0] == "" {
		return "", nil, fmt.Errorf("git merge-tree --write-tree %s %s: %v: %s", short(a),
			short(b), err, strings.TrimSpace(stderr.String()))
	}
	var conflicted []string
	for _, p := range lines[1:] {
		if p != "" && !slices.Contains(conflicted, p) {
			conflicted = append(conflicted, p)
		}
	}
	return lines[0], conflicted, nil
}

// GuardedResolution returns the paths of a sync's resolution that FLOW-14
// guards, which a sync resolution may not edit (FLOW-01, FLOW-20): a rule or
// machinery path of the classification built from the profile file at
// either of head's parents, or a workflow file whose change from git's own
// merge of the parents to head touches a rule-running job
// (workflowGuardChanged). repo is the checkout and profile the profile
// file's path in it; empty, the repository has not adopted and guards
// nothing. A profile at neither parent, or a workflow that does not parse
// on either side, is an error: the caller cannot say, and fails closed.
func GuardedResolution(repo, profile, head string, paths []string) ([]string, error) {
	if profile == "" || len(paths) == 0 {
		return nil, nil
	}
	out, err := gitAt(repo, "rev-list", "--parents", "-n", "1", head)
	if err != nil {
		return nil, err
	}
	ps := strings.Fields(out)[1:]
	if len(ps) != 2 {
		return nil, fmt.Errorf("%s is not a merge of two commits", short(head))
	}
	var ks []classifier
	for _, p := range ps {
		text, err := blobAt(repo, p, profile)
		if err != nil {
			return nil, err
		}
		if text == nil {
			continue
		}
		name := profile + " at " + short(p)
		prof, err := ParseProfile(name, text)
		if err != nil {
			return nil, err
		}
		k, err := newClassifier(name, profile, prof.Rules)
		if err != nil {
			return nil, err
		}
		ks = append(ks, k)
	}
	if len(ks) == 0 {
		return nil, fmt.Errorf("the profile file %s is at neither parent of %s", profile,
			short(head))
	}
	tree, conflicted, err := mergeTree(repo, ps[0], ps[1])
	if err != nil {
		return nil, err
	}
	var guarded []string
	for _, p := range paths {
		hit, err := guardedPath(repo, ks, tree, head, p, slices.Contains(conflicted, p))
		if err != nil {
			return nil, err
		}
		if hit {
			guarded = append(guarded, p)
		}
	}
	return guarded, nil
}

// guardedPath reports whether one resolution path is guarded by any of the
// classifications ks; a workflow file is compared from tree to head. A
// conflicted workflow file is guarded: git's own merge left it with conflict
// markers, so there is no clean version to compare with, and FLOW-20 keeps
// the program trunk off .github/, so it should not conflict there at all.
func guardedPath(repo string, ks []classifier, tree, head, p string, conflicted bool,
) (bool, error) {
	for _, k := range ks {
		if k.kindOf(p) != ordinary {
			return true, nil
		}
	}
	if !strings.HasPrefix(p, workflowsDir) ||
		!strings.HasSuffix(p, ".yml") && !strings.HasSuffix(p, ".yaml") {
		return false, nil
	}
	if conflicted {
		return true, nil
	}
	before, err := blobAt(repo, tree, p)
	if err != nil {
		return false, err
	}
	after, err := blobAt(repo, head, p)
	if err != nil {
		return false, err
	}
	for _, k := range ks {
		if k.runs == nil {
			continue
		}
		changed, err := workflowGuardChanged(k.runs, before, after)
		if err != nil {
			return false, fmt.Errorf("%s: %w", p, err)
		}
		if changed {
			return true, nil
		}
	}
	return false, nil
}

// blobAt is the file at path in rev, nil when rev has none there.
func blobAt(dir, rev, path string) ([]byte, error) {
	ls, err := gitAt(dir, "ls-tree", rev, "--", path)
	if err != nil || ls == "" {
		return nil, err
	}
	cmd := exec.Command("git", "cat-file", "blob", rev+":"+path)
	cmd.Dir = dir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git cat-file blob %s:%s: %v: %s", short(rev), path, err,
			strings.TrimSpace(stderr.String()))
	}
	return out, nil
}

// gitAt runs git in dir and returns its trimmed output.
func gitAt(dir string, args ...string) (string, error) {
	return (&checker{env: Env{Repo: dir}, f: &queue.Feature{}}).git(args...)
}

// ancestorAt reports whether a is an ancestor of b in dir. Exit status 1 is
// "not an ancestor"; any other failure is an error, never read as one.
func ancestorAt(dir, a, b string) (bool, error) {
	cmd := exec.Command("git", "merge-base", "--is-ancestor", a, b)
	cmd.Dir = dir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	err := cmd.Run()
	var exit *exec.ExitError
	switch {
	case err == nil:
		return true, nil
	case errors.As(err, &exit) && exit.ExitCode() == 1:
		return false, nil
	}
	return false, fmt.Errorf("git merge-base --is-ancestor %s %s: %v: %s", a, b, err,
		strings.TrimSpace(stderr.String()))
}

// SyncReady is the program sync's gate (FLOW-08, FLOW-20, Decision 14): the
// pull request's base is repo's program trunk; its head branch is
// sync/program-<sha> and its head the branch's on the remote and the one wf
// sync-program -head recorded for that branch (a sync record); the head is
// a merge of origin/<program trunk> and the trunk's commit <sha>, carrying
// nothing else; every check on it passed or was skipped and the ruleset's
// required ones passed; the head contains both the trunk's and the program
// trunk's heads; and, when the merge's tree is not git's clean merge (a
// conflict resolution), the tier-2 report of the head, under SyncReportDir,
// has no open MUST finding (point 5, read as the merge gate reads it).
func SyncReady(env Env, repo string, number int) []Result {
	f := &queue.Feature{Repos: []string{repo}}
	c := newChecker(env, f)
	if !c.ready() {
		return c.out
	}
	pt := c.state().Config.ProgramTrunkOf(repo)
	c.at.against = pt
	view, err := env.github().PR(repo, number)
	if err != nil {
		c.add("sync gate 1: base is the program trunk (FLOW-20)", false, "%v", err)
		return c.out
	}
	c.add("sync gate 1: base is the program trunk (FLOW-20)", pt != "" && view.Base == pt,
		"PR #%d's base %s, program trunk %s", number, view.Base, orNone(pt))
	c.syncRecorded(view)
	clean := c.syncMerge(view)
	c.gateChecks(view.Head)
	for _, ref := range []string{"origin/" + c.at.trunk, "origin/" + pt} {
		_, err := c.git("merge-base", "--is-ancestor", ref, view.Head)
		c.add("sync gate 4: the head contains "+ref+" (FLOW-20)", err == nil,
			"%s is an ancestor of %s", ref, short(view.Head))
	}
	if !clean {
		c.syncReport(view.Head)
	}
	return c.out
}

// syncRecorded checks point 2 for the sync: the head branch is a sync
// branch, its head is the remote's, and it is the head wf sync-program
// recorded for that branch, the last sync record naming it.
func (c *checker) syncRecorded(view PRView) {
	const name = "sync gate 2: the recorded sync head, at its remote head (FLOW-20)"
	remote := c.remoteHead(view.Branch)
	if !syncBranchRe.MatchString(view.Branch) || remote != view.Head {
		c.add(name, false, "PR head %s on %s, origin/%s %s", short(view.Head),
			view.Branch, view.Branch, orNone(short(remote)))
		return
	}
	records, err := queue.LoadRecords(c.env.Workflow)
	if err != nil {
		c.add(name, false, "%v", err)
		return
	}
	recorded := ""
	for _, r := range records {
		if r.Kind == "sync" && r.Ref == view.Branch &&
			(r.Repo == "" || r.Repo == c.at.name) {
			recorded = r.Head
		}
	}
	c.add(name, len(recorded) >= 7 && strings.HasPrefix(view.Head, recorded),
		"PR head %s on %s; recorded %s (wf sync-program -head)", short(view.Head),
		view.Branch, orNone(recorded))
}

// syncMerge checks that the head is the sync's merge and nothing more, and
// that its branch names the trunk commit it merges; it returns whether the
// merge is git's clean one.
func (c *checker) syncMerge(view PRView) bool {
	const name = "sync gate 2: a merge of the program trunk and the named trunk " +
		"commit, nothing else (FLOW-20)"
	main, clean, err := c.syncHead(view.Head)
	if err != nil {
		c.add(name, false, "%v", err)
		return false
	}
	m := syncBranchRe.FindStringSubmatch(view.Branch)
	ok := m != nil && strings.HasPrefix(main, m[1])
	c.add(name, ok, "%s merges %s; the PR's branch %s", short(view.Head), short(main),
		view.Branch)
	return clean
}

// syncReport checks point 5 on a sync whose merge resolved a conflict: the
// tier-2 report of the head has no open MUST finding.
func (c *checker) syncReport(head string) {
	const name = "sync gate 5: the conflict resolution's tier-2 report, no open MUST " +
		"(FLOW-08, FLOW-20)"
	p := filepath.Join(c.env.Workflow, filepath.FromSlash(SyncReportDir),
		short12(head)+".json")
	rep, err := readReport(p)
	if err != nil {
		c.add(name, false, "%v", err)
		return
	}
	if len(rep.Head) < 7 || !strings.HasPrefix(head, rep.Head) {
		c.add(name, false, "%s read %s, not the PR head %s", p, orNone(rep.Head),
			short(head))
		return
	}
	records, err := queue.LoadRecords(c.env.Workflow)
	if err != nil {
		c.add(name, false, "%v", err)
		return
	}
	open := rep.openMust(records, "", head)
	c.add(name, len(open) == 0, "%s", orText(strings.Join(open, "; "),
		"no open MUST finding, in "+p))
}

// short12 is a commit's 12-character short hash, as reports are named.
func short12(sha string) string {
	if len(sha) > 12 {
		return sha[:12]
	}
	return sha
}
