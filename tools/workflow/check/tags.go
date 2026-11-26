package check

import (
	"cmp"
	"fmt"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/buildwarden/buildwarden/tools/workflow/queue"
)

// specsRepo is the specifications repository, whose tags are per
// specification directory, <directory>/vX.Y.Z.
const specsRepo = "buildwarden/buildwarden-specs"

// specsTagsItem is the item in the specifications repository whose merge
// lets its release workflow publish a program trunk's pre-release tags;
// until it is done no tag is proposed there (FLOW-20).
const specsTagsItem = "specs-prerelease-tags"

// harness is the specifications repository's conformance harness, tagged
// first.
const harness = "conformance"

var releaseRe = regexp.MustCompile(`^v(\d+)\.(\d+)\.(\d+)$`)

// TagNext returns the pre-release tags the loop may create on repo's
// program trunk head, and that head (FLOW-20, Decision 15): vX.Y.Z-dev.N,
// where X.Y.Z is the next minor after the latest release without a suffix
// (v0.1.0 when there is none) and N counts from 1; in the specifications
// repository one <directory>/vX.Y.Z-dev.N per specification directory the
// last merge changed, the harness first. A tag the records hold as due
// counts as taken (FLOW-17). It returns none, with why, when the last merge
// changed no consumable (a path outside docs/ and Markdown files), the head
// already carries a pre-release or has one due, the head's checks are not
// green, or the repository is the specifications one and
// specs-prerelease-tags is not done.
func TagNext(env Env, repo string) (tags []string, head, why string, err error) {
	c, pt, head, why, err := programHead(env, repo)
	if c == nil {
		return nil, "", why, err
	}
	changed, err := c.mergeChanges(head)
	if err != nil {
		return nil, "", "", err
	}
	dirs := consumables(repo, changed, func(dir string) bool {
		_, err := c.git("cat-file", "-e", head+":"+dir+"/go.mod")
		return err == nil
	})
	if len(dirs) == 0 {
		return nil, "", "the last merge on " + pt + " changed no consumable", nil
	}
	if why := c.headGreen(pt, head); why != "" {
		return nil, "", why, nil
	}
	all, err := c.taken(repo)
	if err != nil {
		return nil, "", "", err
	}
	for _, d := range dirs {
		tags = append(tags, nextPre(all, d, "dev"))
	}
	return tags, head, "", nil
}

// taken are the tags of repo's clone and the tags the records hold as due
// there: a due tag is taken, so no later name repeats it (FLOW-17).
func (c *checker) taken(repo string) ([]string, error) {
	all, err := c.git("tag", "-l")
	if err != nil {
		return nil, err
	}
	tags := strings.Fields(all)
	for _, r := range queue.Due(c.state().Records, repo) {
		tags = append(tags, r.Ref)
	}
	return tags, nil
}

// TagNextRC returns the release candidates the loop may create on repo's
// program trunk head, and that head (FLOW-20, a17 (a)): vX.Y.Z-rc.N, X.Y.Z as for -dev and N
// counting from 1, once the component's last phase has merged there and its
// CI and conformance suites pass at the latest pre-releases of what it
// consumes. feature names the program item of the component's last phase in
// repo; the tags are proposed only when its pull request's head is in the
// program trunk's head, no other program item that works in repo is open,
// the head carries no pre-release, its checks are green, and every module of
// another configured repository that a go.mod at the head requires is
// pinned at that repository's latest tag (a pre-release included), read in
// its clone. In the specifications repository it is one
// <directory>/vX.Y.Z-rc.N per directory with a -dev tag of its next
// version, the harness first; until specs-prerelease-tags is done, none. A
// tag the records hold as due counts as taken, as for TagNext; a pin is read
// against the tags that exist.
func TagNextRC(env Env, repo, feature string) (tags []string, head, why string,
	err error) {
	f, err := lastPhase(env.State, repo, feature)
	if err != nil {
		return nil, "", "", err
	}
	c, pt, head, why, err := programHead(env, repo)
	if c == nil {
		return nil, "", why, err
	}
	if why, err := lastPhaseMerged(c, repo, f, head); why != "" || err != nil {
		return nil, "", why, err
	}
	if why := c.headGreen(pt, head); why != "" {
		return nil, "", why, nil
	}
	if why, err := pinsLatest(c, repo, head); why != "" || err != nil {
		return nil, "", why, err
	}
	all, err := c.taken(repo)
	if err != nil {
		return nil, "", "", err
	}
	dirs := []string{""}
	if repo == specsRepo {
		dirs = devDirs(all)
		if len(dirs) == 0 {
			return nil, "", "no specification has a -dev tag of its next version", nil
		}
	}
	for _, d := range dirs {
		tags = append(tags, nextPre(all, d, "rc"))
	}
	return tags, head, "", nil
}

// programHead is the checker on repo's program trunk and its head; a nil
// checker comes with why no tag is proposed, or the error.
func programHead(env Env, repo string) (c *checker, pt, head, why string, err error) {
	s := env.State
	pt = s.Config.ProgramTrunkOf(repo)
	if pt == "" {
		return nil, "", "", "", fmt.Errorf("config.yaml records no program trunk for %s",
			repo)
	}
	if repo == specsRepo {
		if f, err := s.Find(specsTagsItem); err != nil || f.Status != "done" {
			return nil, "", "", specsTagsItem + " has not merged: the " +
				"specifications repository's release workflow does not take " +
				"pre-release tags yet", nil
		}
	}
	c = newChecker(env, &queue.Feature{Repos: []string{repo}})
	if !c.ready() {
		return nil, "", "", "", fmt.Errorf("%s", failed(c.out))
	}
	head, err = c.git("rev-parse", "origin/"+pt)
	if err != nil {
		return nil, "", "", "", err
	}
	if at, _ := c.git("tag", "--points-at", head); strings.Contains(at, "-dev.") ||
		strings.Contains(at, "-rc.") {
		return nil, "", "", "the program trunk's head " + short(head) + " is tagged: " +
			at, nil
	}
	for _, r := range queue.Due(s.Records, repo) {
		if r.Head == head && (strings.Contains(r.Ref, "-dev.") ||
			strings.Contains(r.Ref, "-rc.")) {
			return nil, "", "", "the program trunk's head " + short(head) +
				" has a pre-release due: " + r.Ref, nil
		}
	}
	return c, pt, head, "", nil
}

// headGreen returns why the head's checks are not green, or "".
func (c *checker) headGreen(pt, head string) string {
	c.at.against = pt
	c.gateChecks(head)
	if !OK(c.out) {
		return "the head's checks are not green: " + failed(c.out)
	}
	return ""
}

// worksIn reports whether the item works in repo.
func worksIn(s *queue.State, f *queue.Feature, repo string) bool {
	if len(f.Repos) == 0 {
		return repo == s.Config.Repository
	}
	return slices.Contains(f.Repos, repo)
}

// lastPhase is the program item that works in repo named as the
// component's last phase.
func lastPhase(s *queue.State, repo, feature string) (*queue.Feature, error) {
	if feature == "" {
		return nil, fmt.Errorf("an -rc names the program item of the component's last " +
			"phase (-feature)")
	}
	f, err := s.Find(feature)
	if err != nil {
		return nil, err
	}
	if !f.Program || !worksIn(s, f, repo) {
		return nil, fmt.Errorf("%s is not a program item that works in %s", feature, repo)
	}
	return f, nil
}

// lastPhaseMerged returns why the component's last phase has not merged on
// repo's program trunk head, or "": f's pull request head there is in the
// program trunk's head, and no other program item that works in repo is
// open.
func lastPhaseMerged(c *checker, repo string, f *queue.Feature, head string) (string,
	error) {
	s := c.state()
	feature := f.Feature
	pr := f.PR
	for i := range f.Work {
		if f.Work[i].Repo == repo {
			pr = f.Work[i].PR
		}
	}
	if pr == nil || pr.Head == "" {
		return feature + " has no pull request recorded in " + repo, nil
	}
	if _, err := c.git("merge-base", "--is-ancestor", pr.Head, head); err != nil {
		return feature + "'s pull request head " + short(pr.Head) + " has not merged " +
			"into " + short(head), nil
	}
	var open []string
	for _, o := range s.Features {
		if o.Feature != feature && o.Program && o.Status != "done" && worksIn(s, o, repo) {
			open = append(open, o.Feature)
		}
	}
	if len(open) > 0 {
		return "program items still open in " + repo + ": " + strings.Join(open, ", "), nil
	}
	return "", nil
}

// requireRe matches a go.mod require of a module of a buildwarden
// repository: its path and version.
var requireRe = regexp.MustCompile(
	`(?m)^\s*(?:require\s+)?(github\.com/(buildwarden/[^/\s]+)(?:/(\S+))?)\s+(v\S+)`)

// pinsLatest returns why a module repo consumes is not pinned at its latest
// tag, or "": each go.mod at the head that requires a module of another
// configured repository pins the newest tag of that module's prefix in the
// repository's clone, pre-releases included.
func pinsLatest(c *checker, repo, head string) (string, error) {
	files, err := c.git("ls-tree", "-r", "--name-only", head)
	if err != nil {
		return "", err
	}
	s := c.state()
	var stale []string
	for _, p := range strings.Split(files, "\n") {
		if path.Base(p) != "go.mod" {
			continue
		}
		mod, err := c.git("show", head+":"+p)
		if err != nil {
			return "", err
		}
		for _, m := range requireRe.FindAllStringSubmatch(mod, -1) {
			other, sub, pinned := m[2], m[3], m[4]
			if other == repo || s.Config.Entry(other) == nil {
				continue
			}
			prefix := ""
			if sub != "" {
				prefix = sub + "/"
			}
			latest, err := c.latestTag(other, prefix)
			if err != nil {
				return "", err
			}
			if latest != "" && latest != prefix+pinned {
				stale = append(stale, fmt.Sprintf("%s pins %s %s, latest %s", p,
					m[1], pinned, latest))
			}
		}
	}
	if len(stale) > 0 {
		return "not at the latest pre-releases of what it consumes: " +
			strings.Join(stale, "; "), nil
	}
	return "", nil
}

// latestTag is the newest tag under prefix in repo's clone, pre-releases
// included, or "" when it has none.
func (c *checker) latestTag(repo, prefix string) (string, error) {
	out, err := c.gitIn(repo, "tag", "-l", prefix+"v*")
	if err != nil {
		return "", err
	}
	best, bestV := "", version{}
	for _, t := range strings.Fields(out) {
		v, ok := parseVersion(strings.TrimPrefix(t, prefix))
		if ok && (best == "" || v.compare(bestV) > 0) {
			best, bestV = t, v
		}
	}
	return best, nil
}

// version is a tag's vX.Y.Z with its -dev.N or -rc.N, if any.
type version struct {
	n   [3]int
	pre string // "", "rc" or "dev"
	pn  int
}

var versionRe = regexp.MustCompile(`^v(\d+)\.(\d+)\.(\d+)(?:-(dev|rc)\.(\d+))?$`)

func parseVersion(t string) (version, bool) {
	m := versionRe.FindStringSubmatch(t)
	if m == nil {
		return version{}, false
	}
	var v version
	for i := range v.n {
		v.n[i], _ = strconv.Atoi(m[i+1])
	}
	v.pre = m[4]
	v.pn, _ = strconv.Atoi(m[5])
	return v, true
}

// compare orders by semver: a release after its -rc, an -rc after its -dev.
func (v version) compare(w version) int {
	if c := slices.Compare(v.n[:], w.n[:]); c != 0 {
		return c
	}
	rank := map[string]int{"dev": 0, "rc": 1, "": 2}
	if c := cmp.Compare(rank[v.pre], rank[w.pre]); c != 0 {
		return c
	}
	return cmp.Compare(v.pn, w.pn)
}

// devDirs are the specification directories with a -dev tag of their next
// version, the harness first.
func devDirs(tags []string) []string {
	var out []string
	for _, t := range tags {
		dir, _, ok := strings.Cut(t, "/")
		if !ok || slices.Contains(out, dir+"/") {
			continue
		}
		next := dir + "/" + nextVersion(tags, dir+"/") + "-dev."
		if slices.ContainsFunc(tags, func(x string) bool {
			return strings.HasPrefix(x, next)
		}) {
			out = append(out, dir+"/")
		}
	}
	harnessFirst(out)
	return out
}

// mergeChanges lists the paths the head's merge changed against its first
// parent.
func (c *checker) mergeChanges(head string) ([]string, error) {
	out, err := c.git("diff", "--name-only", head+"^1", head)
	if err != nil {
		return nil, err
	}
	if out == "" {
		return nil, nil
	}
	return strings.Split(out, "\n"), nil
}

// consumables returns the tag prefixes the changed paths make consumable:
// "" for a repository released as a whole, or each specification directory
// changed in the specifications repository, the harness first. Docs and
// Markdown files change no consumable. In the specifications repository a
// directory is a specification, or the harness, only when it is a module
// (isModule: a go.mod at <dir>/go.mod at the head) and not a dot-directory,
// so a CI or tooling change (.github/, tools/, scripts/) proposes no tag.
func consumables(repo string, changed []string, isModule func(dir string) bool) []string {
	var out []string
	for _, p := range changed {
		if strings.HasPrefix(p, "docs/") || path.Ext(p) == ".md" {
			continue
		}
		prefix := ""
		if repo == specsRepo {
			dir, _, ok := strings.Cut(p, "/")
			if !ok || strings.HasPrefix(dir, ".") || !isModule(dir) {
				continue
			}
			prefix = dir + "/"
		}
		if !slices.Contains(out, prefix) {
			out = append(out, prefix)
		}
	}
	harnessFirst(out)
	return out
}

// harnessFirst sorts tag prefixes by name, the harness first.
func harnessFirst(out []string) {
	slices.SortFunc(out, func(a, b string) int {
		ha, hb := a == harness+"/", b == harness+"/"
		switch {
		case ha && !hb:
			return -1
		case hb && !ha:
			return 1
		}
		return cmp.Compare(a, b)
	})
}

// nextVersion is the version a pre-release under the prefix carries: the
// next minor after the latest release without a suffix, v0.1.0 with none.
func nextVersion(tags []string, prefix string) string {
	var latest [3]int
	found := false
	for _, t := range tags {
		v, ok := strings.CutPrefix(t, prefix)
		m := releaseRe.FindStringSubmatch(v)
		if !ok || m == nil {
			continue
		}
		var n [3]int
		for i := range n {
			n[i], _ = strconv.Atoi(m[i+1])
		}
		if !found || slices.Compare(n[:], latest[:]) > 0 {
			latest, found = n, true
		}
	}
	if !found {
		return "v0.1.0"
	}
	return fmt.Sprintf("v%d.%d.0", latest[0], latest[1]+1)
}

// nextPre is the next pre-release tag of the kind, dev or rc, under the
// prefix: nextVersion, and the number after the highest one of that kind
// and version.
func nextPre(tags []string, prefix, kind string) string {
	version := nextVersion(tags, prefix)
	n := 0
	for _, t := range tags {
		rest, ok := strings.CutPrefix(t, prefix+version+"-"+kind+".")
		if k, err := strconv.Atoi(rest); ok && err == nil && k > n {
			n = k
		}
	}
	return fmt.Sprintf("%s%s-%s.%d", prefix, version, kind, n+1)
}

// failed joins the failed results' names and reasons.
func failed(rs []Result) string {
	var out []string
	for _, r := range rs {
		if !r.OK {
			out = append(out, r.Name+": "+r.Info)
		}
	}
	return strings.Join(out, "; ")
}
