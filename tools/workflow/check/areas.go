package check

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"slices"
	"strings"

	"github.com/buildwarden/buildwarden/tools/workflow/queue"
)

// insideAreas checks that every path the branch changes lies in an area the
// item declares, or is the item's own design (FLOW-03): the relations that
// let items run side by side are computed from those areas. An undeclared
// item collides with every item in its repositories, so any path is its own.
// The diff has no rename detection: a renamed file is a change to the area
// it left and to the area it entered.
func (c *checker) insideAreas() {
	const name = "diff inside the declared areas (FLOW-03)"
	s := c.state()
	if s.Undeclared(c.f) {
		c.add(name, true, "undeclared: the item collides with every item in its "+
			"repositories, and no path is outside it (FLOW-05)")
		return
	}
	files, err := c.git("diff", "--name-only", "--no-renames", "origin/main..."+c.f.Branch)
	if err != nil {
		c.add(name, false, "%v", err)
		return
	}
	var outside []string
	for _, p := range strings.Fields(files) {
		if p == c.f.Design {
			continue
		}
		area, err := s.Areas.AreaOf(p)
		if err != nil {
			c.add(name, false, "%v", err)
			return
		}
		if !slices.Contains(*c.f.Areas, area) {
			outside = append(outside, fmt.Sprintf("%s (area %s)", p, area))
		}
	}
	if len(outside) > 0 {
		c.add(name, false, "outside the declared areas %v: %s: widen with wf "+
			"declare when the approved design covers the area, otherwise back to "+
			"Design (FLOW-06)", *c.f.Areas, strings.Join(outside, ", "))
		return
	}
	c.add(name, true, "every changed path is in %v", *c.f.Areas)
}

// trialMerge tries a merge of the branch with the remote branch of each
// in-progress feature the item is disjoint from, and reports what it finds
// (FLOW-10). It never fails the step: disjoint items may have open PRs side
// by side, and this names the other item early. Conflicts in ordered files
// are settled by the later of the two at its refresh; a conflict in an
// exclusive area means one of the two is outside its areas, which that
// item's own diff check fails.
func (c *checker) trialMerge() {
	s := c.state()
	for _, o := range s.Ordered() {
		if o == c.f || o.Feature == c.f.Feature || o.Status != "in_progress" ||
			o.IsProcess() || s.Tracked(o) || o.Branch == "" ||
			s.Relation(c.f, o).Kind != queue.Disjoint {
			continue
		}
		c.add("trial merge with "+o.Feature+" (a report, FLOW-10)", true, "%s",
			c.tryMerge(o))
	}
}

// tryMerge is the report of one trial merge.
func (c *checker) tryMerge(o *queue.Feature) string {
	ref := "refs/remotes/origin/" + o.Branch
	if _, err := c.git("rev-parse", "--verify", "--quiet", ref); err != nil {
		return fmt.Sprintf("origin/%s is not on the remote: not tried", o.Branch)
	}
	cmd := exec.Command("git", "merge-tree", "--write-tree", "--name-only", "--no-messages",
		"refs/heads/"+c.f.Branch, ref)
	cmd.Dir = c.env.Repo
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err == nil {
		return "clean"
	}
	// Exit status 1 is a merge with conflicts: the tree, then the files.
	var exit *exec.ExitError
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if !errors.As(err, &exit) || exit.ExitCode() != 1 || len(lines) < 2 {
		return fmt.Sprintf("not tried: git merge-tree: %v: %s", err,
			strings.TrimSpace(stderr.String()))
	}
	files := lines[1:]
	m := c.state().Areas
	for _, p := range files {
		if area, err := m.AreaOf(p); err == nil && m.Exclusive(area) {
			return fmt.Sprintf("a conflict in the exclusive area %s, so one of the "+
				"two is outside its areas: %s", area, strings.Join(files, ", "))
		}
	}
	return "conflicts in ordered files, which the later of the two settles at its " +
		"refresh: " + strings.Join(files, ", ")
}
