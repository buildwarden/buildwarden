// Command wf reads and moves the feature queue on the workflow orphan branch
// (FLOW-03, FLOW-04, FLOW-05, FLOW-13). Workers call it instead of editing
// queue state by hand, so that step selection, leases and phase transitions
// are mechanical.
//
//	wf validate -state DIR [-areas CANDIDATE [-repo DIR]]
//	wf status   -state DIR [-all | -md]
//	wf next     -state DIR -worker cloud [-feature F]
//	wf claim    -state DIR -worker W -feature F -step S
//	wf release  -state DIR -worker W -feature F -step S -why TEXT
//	wf release  -state DIR -session MACHINE.RUN -why TEXT
//	wf release  -state DIR -machine M -stale -current M.RUN -why TEXT
//	wf renew    -state DIR -worker W -feature F -step S
//	wf check    -state DIR -repo DIR -feature F [-run]
//	wf done     -state DIR -repo DIR -worker W -feature F -step S [-run]
//	wf answer   -state DIR -worker W -feature F -question Q -answer TEXT
//	wf back     -state DIR -worker W -feature F -why FINDING [-to 1|3]
//	wf new      -state DIR -worker W -feature F -title T -priority P
//	            -estimate-steps N -estimate-runs N [-source S] [-follows ITEM]
//	            [-kind process -holds PATTERN,...] [-hold WHY]
//	            [-repos OWNER/NAME,...] [-areas AREA,...] [-seams SEAM,...]
//	            [-adopts CONSUMABLE@VERSION] [-urgent] [-lead host:NAME]
//	wf move     -state DIR -worker human/NAME -feature F -to N [-why TEXT]
//	wf urgent   -state DIR -worker human/NAME -feature F [-clear]
//	wf lead     -state DIR -worker W -feature F (-host NAME | -clear)
//	wf declare  -state DIR -repo DIR -worker W -feature F
//	wf relate   -state DIR -worker human/NAME -feature A -with B [-clear]
//	wf relations -state DIR [-feature F]
//	wf stale    -state DIR -repo DIR -worker W
//	wf commit   -state DIR -worker W -m MESSAGE [-claimed FEATURE/STEP]
//	            [-renewed FEATURE/STEP] [PATH...]
//	wf main     -state DIR [-sha SHA -ci green|red|pending [-gates] [-run URL]]
//	            [-followup ITEM] [-up-to-date required|not-required]
//	wf digest   -state DIR
//	wf areas    -state DIR -list
//	wf areas    -state DIR -check -repo DIR [-rev REV] [-edges FILE|-]
//	wf gate     -state DIR -repo DIR -machine M -stamp FILE [-sweep-days N] [-record]
//	wf budget   -state DIR [-native]
//	wf migrate  -state DIR -worker W [-done]
//	wf drops    -repo DIR -base REV -main FILE -branch FILE [-head REV]
//
// An open item's priority is its position in FLOW-05's order, 1 for the
// first; a done item has none once queue/done/ exists (FLOW-05). Every
// command that changes state renumbers the open items when it saves, and
// commit renumbers what no command did. new -priority P places the item at
// position P as nearly as its class and status allow, prints "queued F
// position K" and logs that line on the item alone, with "(asked P)" when K
// is not P. move, a human's only (LOOP-04), places an open item the same way
// and logs "moved to position K from M (asked N)" with -why's reason. status
// prints each item's position, and -all a dash for a done item with no
// number, done items in the order they closed. migrate -done, run once by a
// human after queue-positions merges, creates queue/done/, moves every done
// item into it without its number and renumbers the open items; until then
// a done item keeps a number past every open item's.
//
// new -follows ITEM queues a fix or follow-up of ITEM: FLOW-05 classes it
// with the fixes, and wf next starts it only once ITEM is done. new -hold WHY
// queues the item on hold (FLOW-20's freeze): WHY is the text of a blocking
// question q1, and the item's first step waits for the human's answer to it
// (wf answer -question q1). Without -hold the loop may take the item at once.
//
// new -repos names the repositories the item works in, its home first; an
// item homed in another repository than config.yaml's is tracked: wf seeds
// it two human steps, t1-work and t2-merge, runs no gate for it and counts
// it in no limit (FLOW-03). new -areas names the areas of the area map the
// item changes; without it the item is undeclared and collides with every
// item in its repositories, and given empty it declares that the item
// changes no area (FLOW-05). With -follows ITEM and no -areas the new item
// takes ITEM's repositories and areas, and its history says so; -repos and
// -areas override that. new -seams records a tracked item's seams, which wf
// cannot read from its design; for an item homed here wf declare copies
// them. new -adopts marks a reconciliation item, which FLOW-05 classes with
// the fixes (LOOP-07).
//
// new -lead, and lead for an item already queued or in progress, name the
// host name whose machines take a feature's cloud steps of Branch setup
// through Guide (FLOW-04, FLOW-09). Any worker may set or clear a lead; lead
// re-labels the item's pending template steps of phases 0 to 5 that no lease
// holds and writes one history line, which the run's digest lists under
// Moved. A process item and a tracked item are never led.
//
// claim leases a step for lease_minutes (120 when unset), or, for a host
// worker host:<name>/<machine>.<run>, for host_lease_minutes (1200 when
// unset). renew sets a lease the worker holds to now plus that length; it
// counts nothing against the item's budget, and commit -renewed pushes it,
// renewing again on the new state after a rejected push, never claiming
// (FLOW-04). release -session releases every live lease of the session, as a
// machine's wrapper does when its session ends and an attended session at its
// Close; release -machine M -stale releases every live lease of a host worker
// whose session is M.<run>, other than -current and any attended-<UTC> run,
// as the wrapper does before the gate. Each release writes a history line.
//
// new -urgent, and urgent for an item already queued, mark a feature so that
// the waiting limit does not refuse its start (FLOW-05). Only a human worker
// sets the mark, never on a process item, a tracked item or an item that
// follows one, which that limit never refuses; urgent -clear takes it from
// any item. wf does not check what the item fixes: FLOW-05 says what the
// mark is for. claim prints the history line of a start past the waiting
// limit.
//
// claim and commit -claimed count a run on the first claim of the item by the
// worker's run, worked out from the worker and the item's history. -new-run
// is accepted and ignored, for callers that still pass it.
//
// next and claim apply FLOW-05's limits to every worker: no step is offered
// or claimed whose claim would put an item's set over the in-progress limit,
// the alters_relationships limit or the PR slot, and no queued feature starts
// while the items waiting on the human number limits.waiting_on_human or
// more. next prints a blocked line with the reason and goes on to the next
// item.
//
// declare copies what the design on the item's branch declares into its
// queue file: repos, areas, seams, the rule IDs of rule_changes,
// alters_relationships and design_review, with the branch's commit as
// declared (FLOW-02). Until Design is done any worker may declare any value,
// except that a design with design_review: skip may not narrow: remove a
// repository or an area the queue has, or declare areas where it has none
// (FLOW-19). In Plan an agent's declare is refused. From Implement on an
// agent may only add areas, each one the area map has and the design implies
// as it stood at the commit its last approval names: the area of a path of a
// rule change, of a surface, or of a driver when the design declares no
// shared area. A design whose review was skipped is read on the branch.
// Every other difference is a design change, for a return to Design
// (FLOW-06). A human may declare any value after Design. No declaration is
// accepted that would put a limit over. For a process item only repos are
// recorded. relate records the human's word that two items collide whatever
// the areas say, and -clear removes it; relations prints each pair of open
// items that collide or are near, with the reason.
//
// The done-when checks compare the queue with the design from Plan on, for
// an item that has declared, and from Implement on require every path the
// branch changes to lie in a declared area (FLOW-03). check at Review and in
// Publish also reports a trial merge with the branch of each in-progress
// feature the item is disjoint from; the report never fails the check.
//
// stale seeds a refresh step on every item main has moved past (FLOW-10): an
// open pull request whose branch does not contain origin/main, or whose head
// is not the commit pr.head records, and a feature that merged main in
// Publish and has not finished it. It sets such a PR's pr.ci to pending, so
// it is not listed as ready to merge until its refresh has passed (FLOW-08).
// For an item with a PR it runs the merge check first: a PR the human merged
// gets no refresh, and prints "merged: ITEM" at its merge step; one merged
// while the item was sent back moves the item to Close with a human step
// that carries the finding. It refreshes nothing itself, and run again it
// adds nothing. done on a refresh step runs the refresh's done-when: main
// merged, the branch pushed, the consolidation entry naming the main commit
// and, with a PR recorded, the archived design and CI green on the pushed
// head. The first claim of a refresh step is not counted in the item's
// budget; next offers none while main.yaml records main red, except to the
// item it names as the fix.
//
// DIR is a checkout of the workflow branch; -repo is a checkout of the code
// with origin fetched. A worker is cloud, human or host:<name>, optionally
// with a /<session> suffix that tells its leases apart. Commands that change
// state write the queue file; the caller pushes it with wf commit.
//
// commit regenerates QUEUE.md and the digest pages (digest), commits them
// with the PATHs given and nothing else, and pushes the workflow branch. A
// PATH queue/F.yaml also names queue/done/F.yaml, and the reverse, when that
// exists, so a close commits the move. It renumbers the queue, and commits
// besides each queue file whose only uncommitted change is its priority line
// and the move of a done item an old wf left in queue/ with no other change.
// With nothing to commit it exits 0. On
// a rejected push it fetches and, when the checkout holds no uncommitted
// change that a reset would discard, resets to origin/workflow: a change is
// replayed with cherry-pick and checked with Validate; a claim (-claimed) is
// not replayed but asked again on the new state through what claim and, for
// a cloud worker, next check, and claimed once more when nothing refuses it.
// A conflict in a generated file, or in main.yaml, keeps the remote's copy;
// one in a queue file confined to priority lines is merged again with the
// remote's numbers (the session's, for an item it placed) and renumbered; one
// in any other file stops it. It tries three times. Exit 4: the claim is
// refused on the new state, and the worker asks next again. Exit 5: settle it
// by hand (a conflict, a new Validate error, another session's uncommitted
// files, three rejections). A change to areas.yaml is committed for a human
// worker only (FLOW-05). It never force-pushes and pushes only workflow.
//
// main records in main.yaml what the session read about main: the head it
// saw and its check runs' result (green only when every run succeeded), a
// failing run's URL, the item filed to fix a red main, and whether GitHub
// requires an up-to-date branch on main (FLOW-08, FLOW-09). It reads no CI
// itself. A red keeps red_since, its first red commit, while main stays red;
// -gates marks a red the session's own gates found on the commit, which a
// green reading of the same commit does not clear. The follow-up is recorded
// once, while main is red. While main is red no refresh is offered but the
// follow-up's.
//
// digest builds digest/<date>.md from the run files in digest/<date>/, in
// file-name order; a page written by hand is first moved into the directory.
// commit runs it.
//
// areas -list prints the area map. areas -check compares it with the tree of
// REV (origin/main by default) and, with -edges, with the dependency lines
// the profile's command prints (tools/tier1/area-edges.sh): each path in the
// catch-all, each pattern that matches no file (a pattern whose area an open
// item declares is expected until that item merges), and each dependency
// between exclusive areas that reaches lacks or does not support. For
// origin/main it records the result in main.yaml. It is a report: it exits 0
// whatever it finds (FLOW-05, PROFILE-01).
//
// validate -areas checks a candidate map in place of areas.yaml, without
// saving it, and prints what it does to the open items: each relation that
// changes, the limit findings under it, and, with -repo, each declared item in
// progress whose branch has a path the candidate puts outside its areas, and
// the candidate's comparison with the paths of origin/main. It fails when the
// candidate fails validation or puts an item outside its areas.
//
// gate is the scheduled native session's go/no-go (FLOW-09): it exits 0 when
// machine M (hosts.yaml) has a ready host step, or open backlog gaps and main
// or a waiting branch moved since the stamp; 3 when idle. -record writes the
// stamp after a finished session.
//
// budget prints the steps one run may take: run_budget, or with -native the
// native_budget a scheduled native session spends (FLOW-13), falling back to
// run_budget.
//
// drops reads no queue: it is FLOW-14's check of an enforcement change by
// main's enforcement, run by tools/workflow/enforcement-check.sh. -main is
// the tier1 -json report of tier1 built from the merge base, -branch the
// branch's own, both over the diff from REV to -head (default HEAD). It
// fails when the branch's report drops a finding on a rule no approved
// rule_changes entry names.
package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"
	"time"

	"github.com/buildwarden/buildwarden/tools/workflow/check"
	"github.com/buildwarden/buildwarden/tools/workflow/queue"
)

const usage = "usage: wf " +
	"validate|status|next|claim|renew|release|check|done|answer|back|new|move|urgent|lead|" +
	"declare|" +
	"relate|relations|stale|commit|main|digest|areas|gate|budget|migrate|drops [flags]"

// errIdle is gate's "nothing to do", exit status 3.
var errIdle = errors.New("idle")

type opts struct {
	state, repo, checkouts, worker, feature, step, why        string
	question, answer                                          string
	title, source, machine, stamp, kind, holds, follows, hold string
	lead, host, session, current, renewed                     string
	repos, areas, seams, adopts, with, message, claimed       string
	rev, edges, repository                                    string
	urgent, clear, check, list, clones, prepare               bool
	// given holds the names of the flags the command line set, for a flag
	// whose empty value means something.
	given                                      map[string]bool
	base, head, mainReport, branchReport       string
	priority, estSteps, estRuns, sweepDays, to int
	run, record, native, md, all, held, done   bool
	stale                                      bool
	now                                        time.Time
	// trunkRev is the trunk the command compares with, origin/<trunk>; empty,
	// the queue repository's origin/main.
	trunkRev string
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
		var ce *queue.CommitError
		if errors.As(err, &ce) {
			os.Exit(ce.Code)
		}
		os.Exit(1)
	}
}

func run(cmd string, args []string, now time.Time) error {
	if cmd == "main" {
		return mainCmd(args)
	}
	o := opts{now: now}
	fs := flag.NewFlagSet(cmd, flag.ExitOnError)
	fs.StringVar(&o.state, "state", ".", "checkout of the workflow branch")
	fs.StringVar(&o.repo, "repo", ".", "checkout of the code, origin fetched")
	fs.StringVar(&o.checkouts, "checkouts", "",
		"a directory of clones of the other repositories, DIR/<owner>/<name>, "+
			"origin fetched")
	fs.StringVar(&o.worker, "worker", "", "cloud, human or host:<name>, optionally /<session>")
	fs.StringVar(&o.feature, "feature", "", "feature name")
	fs.StringVar(&o.step, "step", "", "step id")
	fs.Bool("new-run", false, "ignored: runs are inferred from the worker and the history")
	fs.StringVar(&o.why, "why", "", "reason for a release, or back's finding")
	fs.IntVar(&o.to, "to", 1, "back: the phase to return to, 1 (Design) or 3 (Implement); "+
		"move: the position asked for")
	fs.BoolVar(&o.run, "run", false, "run make lint / make test where a check needs them")
	fs.StringVar(&o.question, "question", "", "question id")
	fs.StringVar(&o.answer, "answer", "", "the human's answer")
	fs.StringVar(&o.title, "title", "", "new: the feature's title")
	fs.StringVar(&o.source, "source", "", "new: where the item came from")
	fs.StringVar(&o.kind, "kind", "", "new: feature (default) or process")
	fs.StringVar(&o.follows, "follows", "", "new: the item this one fixes or finishes")
	fs.StringVar(&o.holds, "holds", "",
		"new: process only, comma-separated affinity patterns it holds")
	fs.StringVar(&o.hold, "hold", "",
		"new: queue the item on hold; the reason is its blocking question q1 (FLOW-20)")
	fs.StringVar(&o.repos, "repos", "",
		"new: comma-separated repositories the item works in, its home first")
	fs.StringVar(&o.areas, "areas", "",
		"new: comma-separated areas the item changes; given empty, it changes none; "+
			"validate: a candidate area map")
	fs.BoolVar(&o.check, "check", false, "areas: compare the map with a tree of -repo; "+
		"native: the install check of the machine's clones and profiles")
	fs.BoolVar(&o.clones, "clones", false,
		"native: print each second repository the machine lists, with its trunk")
	fs.BoolVar(&o.prepare, "prepare", false,
		"native: run the profiles' native.prepare for the machine's hosts")
	fs.StringVar(&o.repository, "repository", "",
		"areas: owner/name, the repository whose map under repos/ to read; its tree is "+
			"its clone under -checkouts")
	fs.BoolVar(&o.list, "list", false, "areas: print the map's areas")
	fs.StringVar(&o.rev, "rev", "origin/main", "areas -check: the tree to compare")
	fs.StringVar(&o.edges, "edges", "",
		"areas -check: the dependency lines, a file or - for standard input")
	fs.StringVar(&o.seams, "seams", "", "new: a tracked item's comma-separated seams")
	fs.StringVar(&o.adopts, "adopts", "", "new: the release adopted, <consumable>@<version>")
	fs.BoolVar(&o.urgent, "urgent", false,
		"new: mark the feature urgent, a human worker only (FLOW-05)")
	fs.BoolVar(&o.clear, "clear", false,
		"urgent: remove the mark; relate: remove the collision; lead: remove the lead")
	fs.StringVar(&o.lead, "lead", "", "new: host:<name>, the feature's lead (FLOW-04)")
	fs.StringVar(&o.host, "host", "", "lead: the host name that leads the feature")
	fs.StringVar(&o.message, "m", "", "commit: the commit message")
	fs.StringVar(&o.claimed, "claimed", "",
		"commit: FEATURE/STEP the change claims, asked again on a rejected push")
	fs.StringVar(&o.renewed, "renewed", "",
		"commit: FEATURE/STEP whose lease the change renews, renewed again if rejected")
	fs.StringVar(&o.session, "session", "",
		"release: release every live lease of this session, <machine>.<run>")
	fs.BoolVar(&o.stale, "stale", false,
		"release: with -machine, release the leases earlier sessions of it left")
	fs.StringVar(&o.current, "current", "", "release -stale: the session to spare")
	fs.StringVar(&o.with, "with", "", "relate: the item the feature collides with")
	fs.IntVar(&o.priority, "priority", 0,
		"new: the position asked for, 1 first, kept as nearly as the item's class allows")
	fs.IntVar(&o.estSteps, "estimate-steps", 0, "new: estimated steps")
	fs.IntVar(&o.estRuns, "estimate-runs", 0, "new: estimated runs")
	fs.StringVar(&o.machine, "machine", "", "gate: the machine's name in hosts.yaml")
	fs.StringVar(&o.stamp, "stamp", "", "gate: file recording the last finished session")
	fs.IntVar(&o.sweepDays, "sweep-days", 7, "gate: re-sweep open gaps after this many days")
	fs.BoolVar(&o.record, "record", false, "gate: write the stamp instead of deciding")
	fs.BoolVar(&o.native, "native", false, "budget: the native session's budget")
	fs.BoolVar(&o.md, "md", false, "status: print the QUEUE.md summary instead")
	fs.BoolVar(&o.all, "all", false, "status: add done items, for a retrospective (FLOW-13)")
	fs.BoolVar(&o.done, "done", false,
		"migrate: move the done items to queue/done/, once (queue-positions)")
	fs.StringVar(&o.base, "base", "origin/main", "drops: the diff's base")
	fs.StringVar(&o.head, "head", "HEAD", "drops: the diff's head")
	fs.StringVar(&o.mainReport, "main", "", "drops: the merge base's tier1 -json report")
	fs.StringVar(&o.branchReport, "branch", "", "drops: the branch's tier1 -json report")
	_ = fs.Parse(args)
	o.given = map[string]bool{}
	fs.Visit(func(fl *flag.Flag) { o.given[fl.Name] = true })
	// -hold given with an empty reason is refused by new, never read as no hold.
	o.held = o.given["hold"]
	switch cmd {
	case "drops":
		return drops(o)
	case "commit":
		return commitCmd(o, fs.Args())
	}

	s, err := queue.Load(o.state)
	if err != nil {
		return err
	}
	cmds := map[string]func(*queue.State, opts) error{
		"validate": validate, "status": status, "next": next, "check": checkPhase,
		"answer": answer, "back": back, "new": newFeature, "move": move, "urgent": urgent,
		"lead": lead,
		"gate": gate, "budget": budget, "migrate": migrate, "stale": stale,
		"declare": declareItem, "relate": relate, "relations": relations,
		"digest": digest, "areas": areasCmd, "native": native,
	}
	for _, c := range []string{"claim", "release", "renew", "done"} {
		cmds[c] = func(s *queue.State, o opts) error {
			if c == "release" && (o.session != "" || o.stale) {
				return releaseLeases(s, o)
			}
			return stepCmd(c, s, o)
		}
	}
	f, ok := cmds[cmd]
	if !ok {
		return fmt.Errorf("unknown command %q; %s", cmd, usage)
	}
	return f(s, o)
}

// commitCmd is wf commit: it pushes the session's paths with the generated
// files, and on a rejected push replays the change or asks the claim again.
func commitCmd(o opts, paths []string) error {
	lines, err := queue.Commit(o.state, queue.CommitRequest{
		Worker: o.worker, Message: o.message, Claimed: o.claimed, Renewed: o.renewed,
		Paths: paths, Now: o.now,
	})
	for _, l := range lines {
		fmt.Println(l)
	}
	return err
}

// mainCmd is wf main: it records what the session read about main in
// main.yaml, or with -repository about that repository's trunk in its own
// main.yaml under repos/. Its flags are its own, since its -run is a URL.
func mainCmd(args []string) error {
	var r queue.MainReading
	var state, repository string
	fs := flag.NewFlagSet("main", flag.ExitOnError)
	fs.StringVar(&state, "state", ".", "checkout of the workflow branch")
	fs.StringVar(&repository, "repository", "",
		"owner/name: record that repository's trunk in repos/<owner>/<name>/main.yaml; "+
			"absent, the queue's own in main.yaml")
	fs.StringVar(&r.SHA, "sha", "", "the head of main the session saw")
	fs.StringVar(&r.CI, "ci", "", "the result of its check runs: green, red or pending")
	fs.BoolVar(&r.Gates, "gates", false, "the red is the session's own gates' on the commit")
	fs.StringVar(&r.Run, "run", "", "a failing run's URL")
	fs.StringVar(&r.Followup, "followup", "", "the item filed to fix a red main")
	fs.StringVar(&r.UpToDate, "up-to-date", "",
		"whether GitHub requires an up-to-date branch on main: required or not-required")
	_ = fs.Parse(args)
	if r == (queue.MainReading{}) {
		return errors.New("main needs -sha and -ci, -followup or -up-to-date")
	}
	s, err := queue.Load(state)
	if err != nil {
		return err
	}
	v, err := s.ForRepository(repository)
	if err != nil {
		return err
	}
	return v.RecordMain(r)
}

// digest is wf digest: it builds each day's page from the day's run files.
func digest(s *queue.State, _ opts) error {
	paths, err := s.Digest()
	for _, p := range paths {
		fmt.Println("wrote", p)
	}
	return err
}

func validate(s *queue.State, o opts) error {
	if o.given["areas"] {
		return validateCandidate(s, o)
	}
	errs := s.Validate()
	for _, e := range errs {
		fmt.Println(e)
	}
	if len(errs) > 0 {
		return fmt.Errorf("%d problems", len(errs))
	}
	fmt.Printf("ok: %d features\n", len(s.Features))
	return s.WriteSummary()
}

func budget(s *queue.State, o opts) error {
	fmt.Println(s.Config.Budget(o.native))
	return nil
}

func status(s *queue.State, o opts) error {
	if o.md {
		fmt.Print(queue.Summary(s))
		return nil
	}
	// The positions as the order has them now, whatever a hand edit wrote.
	s.Renumber()
	for _, f := range s.Listed(o.all) {
		b := f.Budget
		pos := "-"
		if f.Priority > 0 {
			pos = fmt.Sprint(f.Priority)
		}
		fmt.Printf("%s\t%s\t%s\t%s/%d %s\tsteps %d/%d\truns %d/%d\n",
			pos, f.Feature, f.Status, f.Round, f.Phase, queue.PhaseName(f),
			b.Spent.Steps, b.Estimate.Steps, b.Spent.Runs, b.Estimate.Runs)
		if f.IsProcess() {
			fmt.Printf("\t  process item, holds: %s\n", holdList(f.Holds))
		}
		if f.Follows != "" {
			fmt.Printf("\t  follows: %s\n", f.Follows)
		}
		if len(f.Repos) > 0 {
			fmt.Printf("\t  repos: %s\n", strings.Join(f.Repos, ", "))
		}
		if f.Areas != nil {
			fmt.Printf("\t  areas: %s\n", holdList(*f.Areas))
		}
		if f.Declared != "" {
			fmt.Printf("\t  declared: %s\n", f.Declared)
		}
		if f.Adopts != nil {
			fmt.Printf("\t  adopts: %s %s\n", f.Adopts.Consumable, f.Adopts.Version)
		}
		if f.Urgent != "" {
			fmt.Printf("\t  urgent: %s\n", f.Urgent)
		}
		if f.Lead != "" {
			fmt.Printf("\t  lead: %s\n", f.Lead)
		}
		for _, q := range f.Questions {
			if q.Status == "open" {
				fmt.Printf("\t  %s question %s: %s (recommended: %s)\n",
					q.Kind, q.ID, q.Question, q.Recommended)
			}
		}
	}
	return nil
}

func holdList(hs []string) string {
	if len(hs) == 0 {
		return "nothing"
	}
	return strings.Join(hs, ", ")
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
	if !runChecks(s, o, f) {
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
		if err := s.MayClaim(f, st, o.worker, o.now); err != nil {
			return err
		}
		if line := s.Claim(f, st, o.worker, o.now); line != "" {
			fmt.Println(line)
		}
	case "release":
		s.Release(f, st, o.worker, o.now, o.why)
	case "renew":
		if err := s.Renew(f, st, o.worker, o.now); err != nil {
			return err
		}
	case "done":
		if err := finish(s, f, st, o); err != nil {
			return err
		}
	}
	return s.Save(f)
}

// releaseLeases is release -session and release -machine -stale: it releases
// every live lease of the session, or those earlier sessions of the machine
// left, saves each item it changed, and prints a line for each (FLOW-04).
func releaseLeases(s *queue.State, o opts) error {
	if o.why == "" {
		return errors.New("release needs -why")
	}
	var rs []queue.Released
	switch {
	case o.stale && o.session == "" && o.machine != "" && o.current != "":
		rs = s.ReleaseStale(o.machine, o.current, o.why, o.now)
	case !o.stale && o.machine == "" && o.session != "":
		rs = s.ReleaseSession(o.session, o.why, o.now)
	default:
		return errors.New("release takes -session MACHINE.RUN, or -machine M -stale " +
			"-current M.RUN")
	}
	saved := map[*queue.Feature]bool{}
	for _, r := range rs {
		fmt.Printf("released\t%s\t%s\t%s\n", r.Feature.Feature, r.Step, r.ClaimedBy)
		if !saved[r.Feature] {
			saved[r.Feature] = true
			if err := s.Save(r.Feature); err != nil {
				return err
			}
		}
	}
	return nil
}

// finish is wf done: it completes the step once its done-when passes. A gate
// has its phase's, and three steps that are not gates have their own: the
// design review, which precedes Plan, its approval (FLOW-19), a refresh,
// which no phase seeds, the refresh's (FLOW-10), and an f6-merged-<n>, the
// merge of its work entry's pull request (FLOW-08).
func finish(s *queue.State, f *queue.Feature, st *queue.Step, o opts) error {
	if err := queue.Early(f, st); err != nil {
		return err
	}
	if queue.IsDesignReview(st) && !printResults(check.Review(env(s, o), f)) {
		return fmt.Errorf("%s has no recorded approval (FLOW-19)", st.ID)
	}
	if queue.IsGate(f, st) && !runChecks(s, o, f) {
		return fmt.Errorf("%s done-when not met; phase stays %d (FLOW-03)", st.ID,
			f.Phase)
	}
	if queue.IsRefresh(st) && !printResults(check.Refresh(env(s, o), f)) {
		return fmt.Errorf("%s done-when not met (FLOW-10)", st.ID)
	}
	if n, ok := queue.MergedFirst(st); ok && !printResults(check.MergedFirst(env(s, o), f,
		n)) {
		return fmt.Errorf("%s done-when not met: the work entry's PR is not merged "+
			"(FLOW-08)", st.ID)
	}
	queue.Complete(f, st, o.worker, o.now)
	return nil
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

// back returns the feature to Design (-to 1, FLOW-06) or Implement (-to 3,
// FLOW-10) with the finding.
func back(s *queue.State, o opts) error {
	f, err := s.Find(o.feature)
	if err != nil {
		return err
	}
	if o.worker == "" || o.why == "" {
		return fmt.Errorf("back needs -worker and -why (the finding, FLOW-06)")
	}
	if o.to != 1 && o.to != 3 {
		return fmt.Errorf("back -to is 1 (Design) or 3 (Implement), not %d", o.to)
	}
	queue.Back(f, o.worker, o.now, o.why, o.to)
	return s.Save(f)
}

// migrate moves every unfinished item onto the single feature round and the
// new process phases, once, after workflow-streamline merges. With -done it
// moves the done items to queue/done/, once, after queue-positions merges.
func migrate(s *queue.State, o opts) error {
	if o.worker == "" {
		return fmt.Errorf("migrate needs -worker")
	}
	if o.done {
		moved, err := s.MigrateDone(o.worker)
		for _, f := range moved {
			fmt.Printf("moved\t%s\tqueue/done/%s.yaml\n", f.Feature, f.Feature)
		}
		return err
	}
	changed, err := s.Migrate(o.worker, o.now)
	if err != nil {
		return err
	}
	for _, f := range changed {
		fmt.Printf("migrated\t%s\t%s/%d %s\n", f.Feature, f.Round, f.Phase,
			queue.PhaseName(f))
		if err := s.Save(f); err != nil {
			return err
		}
	}
	return nil
}

// newFeature queues a feature (the workflow-intake entry point).
func newFeature(s *queue.State, o opts) error {
	if o.feature == "" || o.title == "" || o.priority < 1 || o.estSteps < 1 {
		return fmt.Errorf("new needs -feature, -title, -priority and -estimate-steps")
	}
	kind, holds, err := kindFlags(o)
	if err != nil {
		return err
	}
	f := &queue.Feature{
		Feature: o.feature, Title: o.title, Priority: o.priority, Source: o.source,
		Kind: kind, Holds: holds, Follows: o.follows, Lead: o.lead,
		Design: "docs/design/" + o.feature + ".md", Branch: "feat/" + o.feature,
		Budget: queue.Budget{Estimate: queue.Count{Runs: o.estRuns, Steps: o.estSteps}},
	}
	taken, err := declare(s, f, o)
	if err != nil {
		return err
	}
	if err := s.Add(f, o.now, o.worker); err != nil {
		return err
	}
	if taken != "" {
		f.Log(o.now, o.worker, "%s", taken)
	}
	if err := accept(s, f, o); err != nil {
		return err
	}
	if o.held {
		if err := f.PutOnHold(o.hold, o.now, o.worker); err != nil {
			return fmt.Errorf("-hold: %w", err)
		}
	}
	if err := s.Save(f); err != nil {
		return err
	}
	fmt.Printf("queued\t%s\tposition %d\n", f.Feature, f.Priority)
	return nil
}

// move is wf move: the human places an open item (FLOW-05, LOOP-04).
func move(s *queue.State, o opts) error {
	f, err := s.Find(o.feature)
	if err != nil {
		return err
	}
	if !o.given["to"] {
		return errors.New("move needs -to, the position asked for")
	}
	if err := s.Move(f, o.to, o.why, o.worker, o.now); err != nil {
		return err
	}
	if err := s.Save(f); err != nil {
		return err
	}
	fmt.Printf("moved\t%s\tposition %d\n", f.Feature, f.Priority)
	return nil
}

// declare sets what new's -repos, -areas, -seams and -adopts say on the item.
// With -follows and no -areas the item takes the repositories and areas of
// the item it follows, and declare returns the history line that says so.
func declare(s *queue.State, f *queue.Feature, o opts) (string, error) {
	f.Repos, f.Seams = list(o.repos), list(o.seams)
	if o.given["areas"] {
		// Given empty, the item declares that it changes no area.
		areas := append([]string{}, list(o.areas)...)
		f.Areas = &areas
	}
	if o.given["adopts"] {
		consumable, version, ok := strings.Cut(o.adopts, "@")
		if !ok || consumable == "" || version == "" {
			return "", fmt.Errorf("-adopts is <consumable>@<version>, not %q",
				o.adopts)
		}
		f.Adopts = &queue.Adopts{Consumable: consumable, Version: version}
	}
	if o.follows == "" {
		return "", nil
	}
	parent, err := s.Find(o.follows)
	if err != nil {
		return "", fmt.Errorf("-follows: %w", err)
	}
	if o.given["areas"] {
		return "", nil
	}
	return f.Inherit(parent, o.given["repos"]), nil
}

// accept marks the queued item urgent when -urgent asks, and refuses an item
// wf validate would fail, before anything is written.
func accept(s *queue.State, f *queue.Feature, o opts) error {
	if o.urgent {
		if err := s.MarkUrgent(f, o.worker, o.now); err != nil {
			return err
		}
	}
	if f.Lead != "" && !s.Serves(f.Lead) {
		return fmt.Errorf("-lead: no machine in hosts.yaml serves %s", f.Lead)
	}
	if f.Lead != "" {
		if err := s.MayLeadOn(f, f.Lead); err != nil {
			return fmt.Errorf("-lead: %w", err)
		}
	}
	if len(f.Seams) > 0 && !s.Tracked(f) {
		return fmt.Errorf("-seams is for a tracked item: wf declare copies the seams of "+
			"%s, which is homed here, from its design (FLOW-02)", f.Feature)
	}
	return errors.Join(s.Problems(f)...)
}

// areasFor returns the state and options wf areas works with: the queue
// repository's map and trunk, or with -repository that repository's map
// under repos/, its clone and its trunk. -rev absent is the trunk.
func areasFor(s *queue.State, o opts) (*queue.State, opts, error) {
	t, err := queueTrunk(s)
	if err != nil {
		return nil, o, err
	}
	o.trunkRev = t
	v, err := s.ForRepository(o.repository)
	if err != nil {
		return nil, o, err
	}
	if v != s {
		if o.repo, err = check.Clone(env(s, o), o.repository); err != nil {
			return nil, o, err
		}
		trunk, err := s.Config.Trunk(o.repository)
		if err != nil {
			return nil, o, err
		}
		o.trunkRev = "origin/" + trunk
	}
	if !o.given["rev"] {
		o.rev = o.trunkRev
	}
	return v, o, nil
}

// queueTrunk is the queue repository's trunk ref, origin/<trunk> from its
// config.yaml entry. With no queue repository set, as in a state before
// config.yaml named one, it is origin/main, as before.
func queueTrunk(s *queue.State) (string, error) {
	if s.Config.Repository == "" {
		return "origin/main", nil
	}
	t, err := s.Config.Trunk(s.Config.Repository)
	if err != nil {
		return "", err
	}
	return "origin/" + t, nil
}

// trunk is the trunk ref the command compares with.
func (o opts) trunk() string {
	if o.trunkRev != "" {
		return o.trunkRev
	}
	return "origin/main"
}

// list splits a comma-separated flag value.
func list(value string) []string {
	var out []string
	for _, v := range strings.Split(value, ",") {
		if v = strings.TrimSpace(v); v != "" {
			out = append(out, v)
		}
	}
	return out
}

// urgent marks a queued feature urgent, or with -clear removes the mark, for
// a human worker (FLOW-05).
func urgent(s *queue.State, o opts) error {
	f, err := s.Find(o.feature)
	if err != nil {
		return err
	}
	if o.clear {
		err = s.ClearUrgent(f, o.worker, o.now)
	} else {
		err = s.MarkUrgent(f, o.worker, o.now)
	}
	if err != nil {
		return err
	}
	return s.Save(f)
}

// lead sets the feature's lead, or with -clear removes it, for any worker
// (FLOW-04, FLOW-09).
func lead(s *queue.State, o opts) error {
	if o.worker == "" {
		return errors.New("lead needs -worker")
	}
	if o.clear == (o.host != "") {
		return errors.New("lead needs -host NAME or -clear")
	}
	f, err := s.Find(o.feature)
	if err != nil {
		return err
	}
	to := ""
	if o.host != "" {
		to = "host:" + strings.TrimPrefix(o.host, "host:")
	}
	if err := s.SetLead(f, o.worker, to, o.now); err != nil {
		return err
	}
	return s.Save(f)
}

// kindFlags parses new's -kind and -holds; a plain feature stores no kind.
func kindFlags(o opts) (string, []string, error) {
	if o.kind != "" && o.kind != "feature" && o.kind != "process" {
		return "", nil, fmt.Errorf("-kind is feature or process, not %q", o.kind)
	}
	holds := list(o.holds)
	if len(holds) > 0 && o.kind != "process" {
		return "", nil, fmt.Errorf("-holds needs -kind process")
	}
	if o.kind == "process" {
		return "process", holds, nil
	}
	return "", nil, nil
}

// env is where the done-when checks look for this command line.
func env(s *queue.State, o opts) check.Env {
	return check.Env{Repo: o.repo, Checkouts: o.checkouts, Workflow: o.state, Run: o.run,
		State: s}
}

// declareItem is wf declare: it copies what the item's design declares on
// its branch into the queue file, by the rule for where the item is and who
// asks (FLOW-02), and prints the history line.
func declareItem(s *queue.State, o opts) error {
	f, err := s.Find(o.feature)
	if err != nil {
		return err
	}
	if err := s.MayDeclare(f, o.worker); err != nil {
		return err
	}
	d, err := check.Declarations(env(s, o), f)
	if err != nil {
		return err
	}
	if err := s.Declare(f, d, o.worker, o.now); err != nil {
		return err
	}
	fmt.Printf("%s\t%s\n", f.Feature, f.History[len(f.History)-1].Event)
	return s.Save(f)
}

// relate is wf relate: the human's word that two items collide, or with
// -clear that they no longer do by that word (FLOW-05).
func relate(s *queue.State, o opts) error {
	f, err := s.Find(o.feature)
	if err != nil {
		return err
	}
	if o.with == "" {
		return errors.New("relate needs -with, the item it collides with")
	}
	if err := s.Relate(f, o.with, o.clear, o.worker, o.now); err != nil {
		return err
	}
	return s.Save(f)
}

// relations is wf relations: each pair of open items that collide or are
// near, with the reason, in FLOW-05's order; with -feature only that item's
// pairs. Disjoint pairs are not printed, and a process item is outside the
// relations.
func relations(s *queue.State, o opts) error {
	var open []*queue.Feature
	for _, f := range s.Listed(false) {
		if !f.IsProcess() {
			open = append(open, f)
		}
	}
	if o.feature != "" {
		f, err := s.Find(o.feature)
		if err != nil {
			return err
		}
		if f.IsProcess() {
			fmt.Printf("%s is a process item: outside the relations (FLOW-05)\n",
				f.Feature)
			return nil
		}
	}
	printed := false
	for i, a := range open {
		for _, b := range open[i+1:] {
			if o.feature != "" && a.Feature != o.feature && b.Feature != o.feature {
				continue
			}
			r := s.Relation(a, b)
			if r.Kind != queue.Colliding && r.Kind != queue.Near {
				continue
			}
			first, second := a.Feature, b.Feature
			if second == o.feature {
				first, second = second, first
			}
			fmt.Printf("%s, %s: %s\n", first, second, r)
			printed = true
		}
	}
	if !printed {
		fmt.Println("none: every pair is disjoint")
	}
	return nil
}

func runChecks(s *queue.State, o opts, f *queue.Feature) bool {
	return printResults(check.Phase(env(s, o), f))
}

// git runs git in the code checkout and returns what it printed.
func git(repo string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = repo
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err,
			strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(string(out)), nil
}

// stale seeds a refresh on each item main has moved past, after the merge
// check (FLOW-08, FLOW-10). It prints one line for each item it changed or
// found merged or gone, and saves each item it changed.
func stale(s *queue.State, o opts) error {
	if o.worker == "" {
		return errors.New("stale needs -worker")
	}
	for _, f := range s.Ordered() {
		line, changed, err := staleItem(s, f, o)
		if err != nil {
			return fmt.Errorf("%s: %w", f.Feature, err)
		}
		if line != "" {
			fmt.Println(line)
		}
		if changed {
			if err := s.Save(f); err != nil {
				return err
			}
		}
	}
	return nil
}

// staleItem is wf stale for one item: the line to print, and whether the
// item changed. It looks at an item in progress and homed here that has a PR
// recorded and has not reached Close, or is a feature that merged main in
// Publish.
func staleItem(s *queue.State, f *queue.Feature, o opts) (string, bool, error) {
	hasPR := f.PR != nil && f.PR.Number > 0
	open := hasPR && f.Phase <= f.PublishPhase()+1
	if f.Status != "in_progress" || s.Tracked(f) || (!open && !f.MergedMainInPublish()) {
		return "", false, nil
	}
	if hasPR && check.Merged(env(s, o), f) {
		line, changed := staleMerged(f, o)
		return line, changed, nil
	}
	head, err := remoteHead(o.repo, f.Branch)
	if err != nil {
		return "", false, err
	}
	if head == "" {
		what := ""
		if hasPR {
			what = fmt.Sprintf("PR #%d is not merged and ", f.PR.Number)
		}
		return fmt.Sprintf("gone\t%s\t%sorigin/%s is gone: no refresh", f.Feature, what,
			f.Branch), false, nil
	}
	return staleSeed(s, f, head, o)
}

// staleSeed seeds the refresh of an item whose branch is on origin at head
// and whose PR, if it has one, is not merged, when it needs one and has none
// pending. A PR that waits for the merge is then not ready: its pr.ci is set
// to pending. An item sent back with its PR open gets none: its next Publish
// merges main at its start.
func staleSeed(s *queue.State, f *queue.Feature, head string, o opts) (string, bool,
	error,
) {
	waits := f.PR != nil && f.PR.Number > 0 && f.AtMerge()
	if f.PendingRefresh() != nil || (!waits && !f.MergedMainInPublish()) {
		return "", false, nil
	}
	trunk, err := queueTrunk(s)
	if err != nil {
		return "", false, err
	}
	var pr *queue.PR
	if waits {
		pr = f.PR
	}
	note, err := staleNote(o.repo, trunk, f.Branch, head, pr)
	if err != nil {
		return "", false, err
	}
	notes := []string{}
	if note != "" {
		notes = append(notes, note)
	}
	more, err := staleWork(s, f, o, waits)
	if err != nil {
		return "", false, err
	}
	if note = strings.Join(append(notes, more...), "; "); note == "" {
		return "", false, nil
	}
	st := queue.SeedRefresh(f, note, o.worker, o.now)
	if waits {
		f.PR.CI = "pending"
	}
	return fmt.Sprintf("refresh\t%s\t%s\t%s", f.Feature, st.ID, note), true, nil
}

// staleMerged is wf stale for an item whose PR the human has merged: no
// refresh is seeded, since a push would bring the deleted branch back
// (FLOW-16). At its merge step the item is only named: that step is the
// cloud's, which the tick's merge confirmation completes and an attended
// session cannot. In Publish nothing is done: its gate passes on the merge.
// Before Publish, the item was sent back with its PR open, and the merge
// supersedes its phases (FLOW-08).
func staleMerged(f *queue.Feature, o opts) (line string, changed bool) {
	switch {
	case f.AtMerge():
		return "merged: " + f.Feature, false
	case f.Phase < f.PublishPhase():
		queue.Landed(f, o.worker, o.now)
		return fmt.Sprintf("landed\t%s\tPR #%d merged while it was sent back: "+
			"moved to Close with %s (FLOW-08)", f.Feature, f.PR.Number,
			queue.StepID(f.Round, f.Phase, "escalate-returned")), true
	}
	return "", false
}

// remoteHead is the commit origin has for the branch, "" when it has none.
func remoteHead(repo, branch string) (string, error) {
	out, err := git(repo, "ls-remote", "--heads", "origin", "refs/heads/"+branch)
	if err != nil || out == "" {
		return "", err
	}
	return strings.Fields(out)[0], nil
}

// staleWork is the refresh notes of the item's branches in its other
// repositories, each against its own trunk (FLOW-10), naming the repository.
// A repository where the branch is not on the remote has nothing to
// refresh.
func staleWork(s *queue.State, f *queue.Feature, o opts, waits bool) ([]string, error) {
	var out []string
	for _, w := range f.Work {
		dir, err := check.Clone(env(s, o), w.Repo)
		if err != nil {
			return nil, err
		}
		trunk, err := s.Config.Trunk(w.Repo)
		if err != nil {
			return nil, err
		}
		head, err := remoteHead(dir, w.Branch)
		if err != nil {
			return nil, err
		}
		if head == "" {
			continue
		}
		var pr *queue.PR
		if waits && w.PR != nil && w.PR.Number > 0 {
			pr = w.PR
		}
		note, err := staleNote(dir, "origin/"+trunk, w.Branch, head, pr)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", w.Repo, err)
		}
		if note != "" {
			out = append(out, w.Repo+": "+note)
		}
	}
	return out, nil
}

// staleNote says why the branch needs a refresh, or "" when it needs none:
// its trunk has moved past it, or, for a PR that waits for the merge, the
// branch's head is not the commit pr.ci was read for. head is the branch's
// commit on origin, which the checkout must have fetched; pr is nil when no
// PR waits.
func staleNote(repo, trunk, branch, head string, pr *queue.PR) (string, error) {
	if _, err := git(repo, "cat-file", "-e", head+"^{commit}"); err != nil {
		return "", fmt.Errorf("origin/%s is at %s, which %s does not have: "+
			"git fetch origin, then run wf stale again", branch, short(head), repo)
	}
	if _, err := git(repo, "merge-base", "--is-ancestor", trunk, head); err != nil {
		return movedNote(repo, trunk, head)
	}
	if pr != nil && !pr.HeadIs(head) {
		was := "no commit"
		if pr.Head != "" {
			was = short(pr.Head)
		}
		return fmt.Sprintf("origin/%s is at %s, and pr.head records %s: pr.ci was read "+
			"for another commit", branch, short(head), was), nil
	}
	return "", nil
}

// movedNote is a refresh step's note when main has moved past the branch's
// head: where main is, how many commits the branch lacks, and the files
// those commits and the branch both change, which the consolidation pass
// reads first.
func movedNote(repo, trunk, head string) (string, error) {
	main, err := git(repo, "rev-parse", trunk)
	if err != nil {
		return "", err
	}
	count, err := git(repo, "rev-list", "--count", head+".."+trunk)
	if err != nil {
		return "", err
	}
	base, err := git(repo, "merge-base", trunk, head)
	if err != nil {
		return "", err
	}
	var changed [2][]string
	for i, rev := range []string{trunk, head} {
		out, err := git(repo, "diff", "--name-only", "--no-renames", base, rev)
		if err != nil {
			return "", err
		}
		changed[i] = strings.Fields(out)
	}
	var both []string
	for _, p := range changed[0] {
		if slices.Contains(changed[1], p) {
			both = append(both, p)
		}
	}
	commits := "commits"
	if count == "1" {
		commits = "commit"
	}
	overlap := "no file changed by both"
	if len(both) > 0 {
		overlap = "both change " + strings.Join(both, ", ")
	}
	return fmt.Sprintf("%s at %s: %s %s since the branch merged it; %s",
		strings.TrimPrefix(trunk, "origin/"), short(main), count, commits, overlap), nil
}

// short abbreviates a commit id to 7 digits.
func short(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}

// printResults prints each check's outcome and reports whether all passed.
func printResults(rs []check.Result) bool {
	for _, r := range rs {
		mark := "ok  "
		if !r.OK {
			mark = "FAIL"
		}
		fmt.Printf("%s %s: %s\n", mark, r.Name, lastLines(r.Info, 6))
	}
	return check.OK(rs)
}

// drops compares main's tier 1 report with the branch's (FLOW-14).
func drops(o opts) error {
	if o.mainReport == "" || o.branchReport == "" {
		return fmt.Errorf("drops needs -main and -branch")
	}
	was, err := os.ReadFile(o.mainReport)
	if err != nil {
		return err
	}
	now, err := os.ReadFile(o.branchReport)
	if err != nil {
		return err
	}
	if !printResults(check.Drops(o.repo, o.base, o.head, was, now)) {
		return fmt.Errorf(
			"the branch drops main's tier 1 findings without an approved rule change")
	}
	return nil
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

// validateCandidate is wf validate -areas: the candidate map's checks and
// what it does to the open items (FLOW-05).
func validateCandidate(s *queue.State, o opts) error {
	m, err := queue.LoadAreas(o.areas)
	if err != nil {
		return err
	}
	c := s.WithAreas(m)
	errs := c.Validate()
	for _, e := range errs {
		fmt.Println(e)
	}
	changed := false
	var open []*queue.Feature
	for _, f := range s.Listed(false) {
		if !f.IsProcess() {
			open = append(open, f)
		}
	}
	for i, a := range open {
		for _, b := range open[i+1:] {
			was, is := s.Relation(a, b), c.Relation(a, b)
			if was.Kind != is.Kind || was.Reason != is.Reason {
				fmt.Printf("%s, %s: %s -> %s\n", a.Feature, b.Feature, was, is)
				changed = true
			}
		}
	}
	if !changed {
		fmt.Println("relations: no change")
	}
	if err := candidateInFlight(s, c, o); err != nil {
		return err
	}
	if len(errs) > 0 {
		return fmt.Errorf("%d problems", len(errs))
	}
	return nil
}

// candidateInFlight is wf validate -areas's check of the branches in
// flight and of the trunk's tree with the candidate map c, run with -repo.
func candidateInFlight(s, c *queue.State, o opts) error {
	if !o.given["repo"] {
		fmt.Println("in-flight branches: not checked")
		return nil
	}
	trunk, err := queueTrunk(s)
	if err != nil {
		return err
	}
	outside, err := inFlight(c, o.repo, trunk)
	if err != nil {
		return err
	}
	if err := tracedTree(c, o.repo, trunk); err != nil {
		return err
	}
	if outside > 0 {
		return fmt.Errorf("%d items would be outside their areas: settle them "+
			"before the map is saved (FLOW-05)", outside)
	}
	return nil
}

// inFlight prints each path of a declared item in progress that the state's
// map puts outside the item's areas, which is the gates' diff check run with
// that map, and returns how many items it named.
func inFlight(s *queue.State, repo, trunk string) (int, error) {
	named := 0
	for _, f := range s.Ordered() {
		if f.Status != "in_progress" || f.IsProcess() || s.Tracked(f) || f.Areas == nil ||
			f.Branch == "" {
			continue
		}
		if _, err := git(repo, "rev-parse", "--verify", "--quiet",
			"refs/remotes/origin/"+f.Branch); err != nil {
			fmt.Printf("%s: origin/%s is not on the remote: not checked\n", f.Feature,
				f.Branch)
			continue
		}
		out, err := git(repo, "diff", "--name-only", "--no-renames",
			trunk+"...origin/"+f.Branch)
		if err != nil {
			return 0, err
		}
		was := named
		for _, p := range strings.Fields(out) {
			area, err := s.Areas.AreaOf(p)
			if err != nil {
				return 0, err
			}
			if p != f.Design && !slices.Contains(*f.Areas, area) {
				fmt.Printf("%s: %s would be in %s, outside its areas %v\n",
					f.Feature, p, area, *f.Areas)
				if named == was {
					named++
				}
			}
		}
	}
	if named == 0 {
		fmt.Println("in-flight branches: every declared item stays inside its areas")
	}
	return named, nil
}

// tracedTree prints the state's map compared with the paths of origin/main:
// a pattern added ahead of its tree is printed as expected.
func tracedTree(s *queue.State, repo, trunk string) error {
	files, err := treeFiles(repo, trunk)
	if err != nil {
		return err
	}
	c, err := s.CheckAreas(files, nil, false)
	if err != nil {
		return err
	}
	for _, l := range c.Lines {
		fmt.Println(trunk + ": " + l)
	}
	return nil
}

// treeFiles lists the files of the revision's tree.
func treeFiles(repo, rev string) ([]string, error) {
	out, err := git(repo, "ls-tree", "-r", "--name-only", rev)
	if err != nil {
		return nil, err
	}
	return strings.Split(out, "\n"), nil
}

// areasCmd is wf areas -list and -check, of the queue repository's map or,
// with -repository, of that repository's under repos/.
func areasCmd(s *queue.State, o opts) error {
	s, o, err := areasFor(s, o)
	if err != nil {
		return err
	}
	if s.Areas == nil {
		return fmt.Errorf("%s has no areas.yaml: each repository is one exclusive area",
			o.state)
	}
	switch {
	case o.list:
		m := s.Areas
		for _, name := range m.Names() {
			a := m.Areas[name]
			line := name + "\t" + a.Mode
			if name == m.CatchAll {
				line += "\tcatch-all"
			}
			if len(a.Paths) > 0 {
				line += "\tpaths: " + strings.Join(a.Paths, ", ")
			}
			if len(a.Reaches) > 0 {
				line += "\treaches: " + strings.Join(a.Reaches, ", ")
			}
			fmt.Println(line)
		}
		return nil
	case o.check:
		return checkAreas(s, o)
	}
	return errors.New("areas needs -check or -list")
}

// checkAreas is wf areas -check. It is a report: what it finds does not fail
// it, and only a failure to run does.
func checkAreas(s *queue.State, o opts) error {
	sha, err := git(o.repo, "rev-parse", o.rev+"^{commit}")
	if err != nil {
		return err
	}
	files, err := treeFiles(o.repo, sha)
	if err != nil {
		return err
	}
	var edges []queue.Edge
	if o.edges != "" {
		r := os.Stdin
		if o.edges != "-" {
			if r, err = os.Open(o.edges); err != nil {
				return err
			}
			defer func() { _ = r.Close() }()
		}
		if edges, err = queue.ParseEdges(r); err != nil {
			return err
		}
	}
	c, err := s.CheckAreas(files, edges, o.edges != "")
	if err != nil {
		return err
	}
	for _, l := range c.Lines {
		fmt.Println(l)
	}
	if c.Differences == 0 {
		fmt.Printf("no difference from %s at %s\n", o.rev, short(sha))
	} else {
		fmt.Printf("%d differences from %s at %s\n", c.Differences, o.rev, short(sha))
	}
	if o.rev != o.trunk() {
		return nil
	}
	return s.RecordAreas(sha, c)
}
