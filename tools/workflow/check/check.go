// Package check runs the FLOW-03 done-when checks: a phase transition happens
// only when these pass, never on an agent's report that it is done.
package check

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/buildwarden/buildwarden/tools/workflow/queue"
)

// Env is where the checks look.
type Env struct {
	Repo     string // a checkout of buildwarden/buildwarden with origin fetched
	Workflow string // a checkout of the workflow branch
	// Run enables the checks that run make targets; off, they are reported
	// as skipped and fail.
	Run bool
}

// Result is one check's outcome.
type Result struct {
	Name string
	OK   bool
	Info string
}

// Phase runs the done-when checks for the feature's current phase.
func Phase(env Env, f *queue.Feature) []Result {
	c := &checker{env: env, f: f}
	if f.Round == "process" {
		c.process()
		return c.out
	}
	merge := f.Round == "merge"
	switch f.Phase {
	case 0:
		if merge {
			c.mainMerged()
		} else {
			c.branchOnRemote()
		}
	case 1:
		c.design(merge)
	case 2:
		c.plan()
	case 3:
		c.onBranchHead()
		c.make("lint")
		c.make("test")
		c.docsTouched("UX-01 reference docs updated", "docs/")
	case 4:
		c.onBranchHead()
		c.make("lint")
		c.make("test")
		c.tier2Report()
	case 5:
		c.docsTouched("guides or examples updated", "docs/book/src/guide/",
			"docs/book/src/getting-started/", "examples/", "README.md")
	case 6:
		if merge {
			c.prGreen()
		} else {
			c.pushed()
		}
	case 7:
		c.merged()
	case 8:
		c.archived()
	}
	return c.out
}

// process runs a process item's done-when checks: x1 design written, x2
// branch pushed, x3 merged by a human, x4 archived.
func (c *checker) process() {
	switch c.f.Phase {
	case 1:
		_, err := c.git("cat-file", "-e", "refs/heads/"+c.f.Branch+":"+c.f.Design)
		c.add("design present (FLOW-01)", err == nil, "%s on %s", c.f.Design, c.f.Branch)
	case 2:
		c.pushed()
	case 3:
		c.merged()
	case 4:
		c.archived()
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
}

func (c *checker) add(name string, ok bool, format string, a ...any) {
	c.out = append(c.out, Result{name, ok, fmt.Sprintf(format, a...)})
}

func (c *checker) git(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = c.env.Repo
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git %s: %v: %s", strings.Join(args, " "), err,
			strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(string(out)), nil
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
	_, err := c.git("merge-base", "--is-ancestor", "origin/main", c.f.Branch)
	c.add("current main merged", err == nil, "origin/main is an ancestor of %s", c.f.Branch)
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

func (c *checker) make(target string) {
	name := "make " + target
	if !c.env.Run {
		c.add(name, false, "skipped (run with -run)")
		return
	}
	cmd := exec.Command("make", target)
	cmd.Dir = c.env.Repo
	out, err := cmd.CombinedOutput()
	tail := string(out)
	if len(tail) > 2000 {
		tail = tail[len(tail)-2000:]
	}
	c.add(name, err == nil, "%s", strings.TrimSpace(tail))
}

// docsTouched passes when the design declares no surfaces, or the branch
// changes a file under one of the prefixes.
func (c *checker) docsTouched(name string, prefixes ...string) {
	d, err := c.loadDesign(c.f.Branch)
	if err != nil {
		c.add(name, false, "%v", err)
		return
	}
	if len(d.Front.Surfaces) == 0 {
		c.add(name, true, "design declares no surfaces")
		return
	}
	files, err := c.git("diff", "--name-only", "origin/main..."+c.f.Branch)
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

func (c *checker) tier2Report() {
	sha, err := c.git("rev-parse", "--short=12", "refs/heads/"+c.f.Branch)
	if err != nil {
		c.add("tier-2 report (LINT-04)", false, "%v", err)
		return
	}
	for _, ext := range []string{".md", ".json"} {
		p := filepath.Join(c.env.Workflow, "reports", c.f.Feature, sha+ext)
		_, err := os.Stat(p)
		c.add("tier-2 report "+ext+" (LINT-04, LINT-05)", err == nil, "%s", p)
	}
}

func (c *checker) prGreen() {
	ok := c.f.PR != nil && c.f.PR.Number > 0 && c.f.PR.CI == "green"
	c.add("PR open with CI green (FLOW-11)", ok, "pr %+v", c.f.PR)
}

func (c *checker) merged() {
	_, err := c.git("merge-base", "--is-ancestor", "refs/heads/"+c.f.Branch, "origin/main")
	c.add("merged by a human (FLOW-08)", err == nil, "%s is contained in origin/main",
		c.f.Branch)
}

func (c *checker) archived() {
	d, err := c.loadDesign("origin/main")
	if err != nil {
		c.add("design archived (FLOW-12)", false, "%v", err)
	} else {
		c.add("design archived (FLOW-12)", d.Front.Status == "archived",
			"status %q on main", d.Front.Status)
		c.section(d, "Archive summary")
	}
	c.add("branch deleted (FLOW-16)", c.remoteHead(c.f.Branch) == "", "origin/%s", c.f.Branch)
}

func (c *checker) design(merge bool) {
	d, err := c.loadDesign(c.f.Branch)
	if err != nil {
		c.add("design present (FLOW-01)", false, "%v", err)
		return
	}
	c.add("design present (FLOW-01)", true, "%s on %s", c.f.Design, c.f.Branch)
	for _, e := range d.Front.problems(c.f.Feature) {
		c.add("front-matter (FLOW-02)", false, "%s", e)
	}
	if d.Front.Alters == nil || c.f.AltersRelationships == nil ||
		*d.Front.Alters != *c.f.AltersRelationships {
		c.add("alters_relationships recorded in queue", false, "design %v, queue %v",
			ptr(d.Front.Alters), ptr(c.f.AltersRelationships))
	}
	for _, s := range []string{"Problem", "Prior art", "Decisions", "Design",
		"Cross-driver impact", "Validation plan"} {
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
	if merge {
		c.section(d, "Consolidation (merge round)")
	}
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
	Feature          string   `yaml:"feature"`
	Status           string   `yaml:"status"`
	Round            string   `yaml:"round"`
	Drivers          []string `yaml:"drivers"`
	Seams            []string `yaml:"seams"`
	Surfaces         []string `yaml:"surfaces"`
	SecurityBoundary string   `yaml:"security_boundary"`
	Alters           *bool    `yaml:"alters_relationships"`
	NativeValidation []struct {
		Host     string `yaml:"host"`
		Blocking *bool  `yaml:"blocking"`
		Reason   string `yaml:"reason"`
	} `yaml:"native_validation"`
	Tier2Gaps []any    `yaml:"tier2_gaps"`
	Rules     []string `yaml:"rules"`
	Concepts  []string `yaml:"concepts"`
	present   map[string]bool
}

func (fm *Front) problems(feature string) []string {
	var out []string
	for _, k := range []string{"feature", "status", "round", "drivers", "seams", "surfaces",
		"security_boundary", "alters_relationships", "native_validation", "tier2_gaps",
		"rules", "concepts"} {
		if !fm.present[k] {
			out = append(out, "missing "+k)
		}
	}
	if fm.Feature != feature {
		out = append(out, fmt.Sprintf("feature %q, queue says %q", fm.Feature, feature))
	}
	if !oneOf(fm.Status, "draft", "active", "archived") {
		out = append(out, fmt.Sprintf("status %q", fm.Status))
	}
	if !oneOf(fm.SecurityBoundary, "none", "host-env", "env-relay", "env-beyond") {
		out = append(out, fmt.Sprintf("security_boundary %q", fm.SecurityBoundary))
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
	d.Front.present = map[string]bool{}
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
