package check

import (
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
// GLOBAL-06, the SEC rules, RELAY-01 to RELAY-03 and RELAY-05.
var protectedRe = regexp.MustCompile(`^(GLOBAL-06|SEC-[0-9]+|RELAY-0[1235])$`)

// MergeReady is the merge gate of a pull request into a program trunk
// (FLOW-08, FLOW-20): it checks the item's pull request in repo against
// TrunkFor, and each failure names its point. The loop merges only while
// every point holds, on the head point 2 read, with --match-head-commit.
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

// gateContains checks point 4: the head contains the program trunk's head.
func (c *checker) gateContains(head string) {
	const name = "merge gate 4: the branch contains the program trunk (FLOW-08)"
	_, err := c.git("merge-base", "--is-ancestor", c.trunk(), head)
	c.add(name, err == nil, "%s is an ancestor of %s", c.trunk(), short(head))
}

// gateReport checks point 5: the tier-2 report is for the head, and no MUST
// finding is open but one the records defer; a protected one is never
// deferred, and sends the item back to Design (Decision 10).
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
	var rep struct {
		Head     string            `json:"head"`
		Heads    map[string]string `json:"heads"`
		Findings []struct {
			ID       any    `json:"id"`
			Rule     string `json:"rule"`
			Severity string `json:"severity"`
			Status   string `json:"status"`
		} `json:"findings"`
	}
	data, err := os.ReadFile(p)
	if err == nil {
		err = json.Unmarshal(data, &rep)
	}
	if err != nil {
		c.add(name, false, "%s: %v", p, err)
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
	var open []string
	for _, f := range rep.Findings {
		if f.Severity != "MUST" || !strings.HasPrefix(f.Status, "open") {
			continue
		}
		id := fmt.Sprint(f.ID)
		switch {
		case protectedRe.MatchString(f.Rule):
			open = append(open, fmt.Sprintf("%s (%s) is protected: back to Design "+
				"(FLOW-06)", id, f.Rule))
		case !deferred(records, c.f.Feature, id, head):
			open = append(open, fmt.Sprintf("%s (%s) open and not deferred", id,
				f.Rule))
		}
	}
	c.add(name, len(open) == 0, "%s", orText(strings.Join(open, "; "),
		"no open MUST finding but those deferred, in "+p))
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
		return r.Kind == "security-ack" && atHead(r)
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
var syncBranchRe = regexp.MustCompile(`^sync/program-[0-9a-f]{7,40}$`)

// SyncReady is the program sync's gate (FLOW-20, Decision 14): the pull
// request's base is repo's program trunk; its head branch is the one wf
// sync-program names and its head is that branch's on the remote; every
// check on it passed or was skipped and the ruleset's required ones passed;
// and the head contains both the trunk's and the program trunk's heads.
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
	remote := c.remoteHead(view.Branch)
	c.add("sync gate 2: a sync branch, at its remote head (FLOW-20)",
		syncBranchRe.MatchString(view.Branch) && remote == view.Head,
		"PR head %s on %s, origin/%s %s", short(view.Head), view.Branch, view.Branch,
		orNone(short(remote)))
	c.gateChecks(view.Head)
	for _, ref := range []string{"origin/" + c.at.trunk, "origin/" + pt} {
		_, err := c.git("merge-base", "--is-ancestor", ref, view.Head)
		c.add("sync gate 4: the head contains "+ref+" (FLOW-20)", err == nil,
			"%s is an ancestor of %s", ref, short(view.Head))
	}
	return c.out
}
