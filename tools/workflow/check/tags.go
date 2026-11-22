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

// TagNext returns the pre-release tags the loop may push on repo's program
// trunk head (FLOW-20, Decision 15): vX.Y.Z-dev.N, where X.Y.Z is the next
// minor after the latest release without a suffix (v0.1.0 when there is
// none) and N counts from 1; in the specifications repository one
// <directory>/vX.Y.Z-dev.N per specification directory the last merge
// changed, the harness first. It returns none, with why, when the last
// merge changed no consumable (a path outside docs/ and Markdown files), the
// head already carries a pre-release, the head's checks are not green, or
// the repository is the specifications one and specs-prerelease-tags is not
// done.
func TagNext(env Env, repo string) (tags []string, why string, err error) {
	s := env.State
	pt := s.Config.ProgramTrunkOf(repo)
	if pt == "" {
		return nil, "", fmt.Errorf("config.yaml records no program trunk for %s", repo)
	}
	if repo == specsRepo {
		if f, err := s.Find(specsTagsItem); err != nil || f.Status != "done" {
			return nil, specsTagsItem + " has not merged: the specifications " +
				"repository's release workflow does not take pre-release tags " +
				"yet", nil
		}
	}
	c := newChecker(env, &queue.Feature{Repos: []string{repo}})
	if !c.ready() {
		return nil, "", fmt.Errorf("%s", failed(c.out))
	}
	head, err := c.git("rev-parse", "origin/"+pt)
	if err != nil {
		return nil, "", err
	}
	if at, _ := c.git("tag", "--points-at", head); strings.Contains(at, "-dev.") ||
		strings.Contains(at, "-rc.") {
		return nil, "the program trunk's head " + short(head) + " is tagged: " + at, nil
	}
	changed, err := c.mergeChanges(head)
	if err != nil {
		return nil, "", err
	}
	dirs := consumables(repo, changed)
	if len(dirs) == 0 {
		return nil, "the last merge on " + pt + " changed no consumable", nil
	}
	c.at.against = pt
	c.gateChecks(head)
	if !OK(c.out) {
		return nil, "the head's checks are not green: " + failed(c.out), nil
	}
	all, err := c.git("tag", "-l")
	if err != nil {
		return nil, "", err
	}
	for _, d := range dirs {
		tags = append(tags, nextDev(strings.Fields(all), d))
	}
	return tags, "", nil
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
// Markdown files change no consumable.
func consumables(repo string, changed []string) []string {
	var out []string
	for _, p := range changed {
		if strings.HasPrefix(p, "docs/") || path.Ext(p) == ".md" {
			continue
		}
		prefix := ""
		if repo == specsRepo {
			dir, _, ok := strings.Cut(p, "/")
			if !ok {
				continue
			}
			prefix = dir + "/"
		}
		if !slices.Contains(out, prefix) {
			out = append(out, prefix)
		}
	}
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
	return out
}

// nextDev is the next -dev tag under the prefix: the next minor after the
// latest release without a suffix, v0.1.0 with none, and the dev number
// after the highest one of that version.
func nextDev(tags []string, prefix string) string {
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
	version := "v0.1.0"
	if found {
		version = fmt.Sprintf("v%d.%d.0", latest[0], latest[1]+1)
	}
	dev := 0
	for _, t := range tags {
		rest, ok := strings.CutPrefix(t, prefix+version+"-dev.")
		if n, err := strconv.Atoi(rest); ok && err == nil && n > dev {
			dev = n
		}
	}
	return fmt.Sprintf("%s%s-dev.%d", prefix, version, dev+1)
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
