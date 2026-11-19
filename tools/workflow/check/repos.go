package check

import (
	"fmt"
	"os/exec"
	"slices"
	"strings"

	"github.com/buildwarden/buildwarden/tools/workflow/queue"
)

// repoAt is one repository an item has a branch in, as a check reads it:
// its checkout, the trunk its config.yaml entry gives and, once it has
// adopted, its profile file (Decision 5 of docs/design/loop-repository.md).
type repoAt struct {
	name string
	// work is the item's entry for the repository; nil for the home.
	work  *queue.Work
	dir   string
	trunk string
	// profilePath is the entry's profile; empty, the repository has not
	// adopted the contract and has no fast gate, reference documents or
	// rule paths (LOOP-08).
	profilePath string
	// err says why the repository cannot be checked at all: no entry, no
	// trunk, or no clone.
	err error

	profile       *Profile
	profileErr    error
	profileLoaded bool
}

// newChecker is a checker on the item's home.
func newChecker(env Env, f *queue.Feature) *checker {
	c := &checker{env: env, f: f}
	c.home = c.resolve(c.homeName(), nil)
	c.at = c.home
	return c
}

// homeName is the item's first repository, or the queue's own when it names
// none.
func (c *checker) homeName() string {
	if len(c.f.Repos) > 0 {
		return c.f.Repos[0]
	}
	return c.state().Config.Repository
}

// resolve finds the repository's trunk and checkout. Neither has a default:
// the trunk is the config.yaml entry's only (LOOP-01, GLOBAL-11).
func (c *checker) resolve(name string, w *queue.Work) *repoAt {
	r := &repoAt{name: name, work: w}
	if name == "" {
		r.err = fmt.Errorf("config.yaml names no repository, and %s names no repos: wf "+
			"reads a trunk only from config.yaml's repositories", c.f.Feature)
		return r
	}
	s := c.state()
	if r.trunk, r.err = s.Config.Trunk(name); r.err != nil {
		return r
	}
	r.profilePath = s.Config.Entry(name).Profile
	r.dir, r.err = c.clone(name)
	return r
}

// repos are the repositories the item has a branch in: the home, then each
// work entry in its order.
func (c *checker) repos() []*repoAt {
	if c.others == nil {
		c.others = []*repoAt{}
		for i := range c.f.Work {
			w := &c.f.Work[i]
			c.others = append(c.others, c.resolve(w.Repo, w))
		}
	}
	return append([]*repoAt{c.home}, c.others...)
}

// ready reports whether the home can be checked, and adds the reason as a
// failed result when it cannot.
func (c *checker) ready() bool {
	if c.home.err != nil {
		c.add("repository (LOOP-01)", false, "%v", c.home.err)
		return false
	}
	return true
}

// each runs check once in each repository the item has a branch in, the
// home first. A repository that cannot be checked fails, naming why.
func (c *checker) each(check func()) {
	for _, r := range c.repos() {
		c.at = r
		if r.err != nil {
			c.add("repository (LOOP-01)", false, "%v", r.err)
			continue
		}
		check()
	}
	c.at = c.home
}

// trunk is the remote-tracking ref of the trunk of the repository being
// checked.
func (c *checker) trunk() string { return "origin/" + c.at.trunk }

// dir is the checkout of the repository being checked.
func (c *checker) dir() string {
	if c.at == nil || c.at.dir == "" {
		return c.env.Repo
	}
	return c.at.dir
}

// pr is the item's pull request in the repository being checked.
func (c *checker) pr() *queue.PR {
	if c.at != nil && c.at.work != nil {
		return c.at.work.PR
	}
	return c.f.PR
}

// profile is the profile file of the repository being checked, read from
// the item's branch there, as wf reads the design (Decision 2). Nil with no
// error, the repository has not adopted. A file that does not parse, or
// whose trunk differs from the entry's, is an error naming it.
func (c *checker) profile() (*Profile, error) {
	r := c.at
	if r.profilePath == "" || r.profileLoaded {
		return r.profile, r.profileErr
	}
	r.profileLoaded = true
	rev := "refs/heads/" + c.f.Branch
	name := r.name + ":" + r.profilePath + " on " + c.f.Branch
	text, err := c.git("show", rev+":"+r.profilePath)
	if err != nil {
		r.profileErr = fmt.Errorf("profile file %s: %v", name, err)
		return nil, r.profileErr
	}
	r.profile, r.profileErr = ParseProfile(name, []byte(text))
	if r.profileErr == nil {
		r.profileErr = r.profile.CheckTrunk(name, r.trunk)
	}
	if r.profileErr != nil {
		r.profile = nil
	}
	return r.profile, r.profileErr
}

// fastGate runs the profile's fast gate in the repository being checked,
// each command a result named by its words (FLOW-03 phase 3, LOOP-01). A
// repository that has not adopted has none. The home must have adopted: an
// item homed elsewhere is tracked.
func (c *checker) fastGate() {
	const name = "fast gate (LOOP-01)"
	p, err := c.profile()
	switch {
	case err != nil:
		c.add(name, false, "%v", err)
		return
	case p == nil && c.at == c.home:
		c.add(name, false, "%s has no profile in config.yaml's repositories: an item "+
			"homed there is tracked (FLOW-03)", c.at.name)
		return
	case p == nil:
		return
	}
	for _, argv := range p.FastGate {
		c.command(argv)
	}
}

// command runs one fast-gate command in the checkout.
func (c *checker) command(argv []string) {
	name := strings.Join(argv, " ")
	if len(argv) == 0 {
		c.add("fast gate (LOOP-01)", false, "an empty command in fast_gate")
		return
	}
	if !c.env.Run {
		c.add(name, false, "skipped (run with -run)")
		return
	}
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Dir = c.dir()
	out, err := cmd.CombinedOutput()
	tail := string(out)
	if len(tail) > 2000 {
		tail = tail[len(tail)-2000:]
	}
	c.add(name, err == nil, "%s", strings.TrimSpace(tail))
}

// referenceDocs runs docsTouched with the profile's reference documents of
// the kind: implement (Implement, UX-01) or guides (Guide). A repository
// with no profile, or whose profile names none, has none to check.
func (c *checker) referenceDocs(name string, guides bool) {
	p, err := c.profile()
	if err != nil {
		c.add(name, false, "%v", err)
		return
	}
	if p == nil {
		return
	}
	prefixes := p.ReferenceDocs.Implement
	if guides {
		prefixes = p.ReferenceDocs.Guides
	}
	if len(prefixes) > 0 {
		c.docsTouched(name, prefixes...)
	}
}

// worksIn reports whether the item works in the repository.
func (c *checker) worksIn(s *queue.State, o *queue.Feature, repo string) bool {
	return slices.Contains(s.ReposOf(o), repo)
}
