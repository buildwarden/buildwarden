package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/buildwarden/buildwarden/tools/workflow/check"
)

// rulesCmd is wf rules: the loop's generic checks on a component's checkout,
// read through its profile file (a13 of docs/design/loop-repository.md):
// rule-index (LINT-01, LINT-02) over the profile's rules.dir with its
// rules.index map when it names one; design-front-matter (FLOW-02, FLOW-12)
// over the profile's designs, every design or, with -base, those the change
// touches; and, with -base, suppression (LINT-05) over the Tier1-Allow
// trailers of the change's commits. It needs no queue state, so a
// component's CI runs it. It fails when a finding is MUST.
func rulesCmd(args []string) error {
	var repo, profile, base string
	fs := flag.NewFlagSet("rules", flag.ExitOnError)
	fs.StringVar(&repo, "repo", ".", "the component's checkout")
	fs.String("state", ".", "ignored: wf rules reads no queue state")
	fs.StringVar(&profile, "profile", "", "its profile file, a path in the checkout")
	fs.StringVar(&base, "base", "",
		"check the change from the merge base of REV and HEAD; absent, every design")
	_ = fs.Parse(args)
	if profile == "" {
		return errors.New("rules needs -profile, the component's profile file")
	}
	p, err := check.ReadProfile(filepath.Join(repo, filepath.FromSlash(profile)))
	if err != nil {
		return err
	}
	findings, err := genericChecks(repo, p, base)
	if err != nil {
		return err
	}
	must := 0
	for _, f := range findings {
		where := f.File
		if f.Line > 0 {
			where = fmt.Sprintf("%s:%d", f.File, f.Line)
		}
		fmt.Printf("%s\t%s\t%s\t%s\t%s\n", f.Severity, f.Rule, f.Check, where, f.Message)
		if f.Severity == check.Must {
			must++
		}
	}
	fmt.Printf("%d findings, %d MUST\n", len(findings), must)
	if must > 0 {
		return fmt.Errorf("%d MUST findings", must)
	}
	return nil
}

// genericChecks runs the three checks and returns their findings.
func genericChecks(repo string, p *check.Profile, base string) ([]check.Finding, error) {
	ix := &check.RuleIndex{}
	var out []check.Finding
	if p.Rules.Dir != "" {
		var err error
		if ix, err = check.LoadRuleIndex(repo, p.Rules.Dir); err != nil {
			return nil, err
		}
	}
	m := &check.RuleMap{}
	if p.Rules.Index != "" {
		data, err := os.ReadFile(filepath.Join(repo, filepath.FromSlash(p.Rules.Index)))
		if err != nil {
			return nil, fmt.Errorf("rules.index: %v", err)
		}
		if m, err = check.ParseRuleMap(p.Rules.Index, data); err != nil {
			return nil, err
		}
		out = append(out, check.SelfCheck(ix, m, p.Rules.Index, p.Rules.Dir, nil)...)
	} else {
		out = append(out, ix.Problems...)
	}
	t := gitTree{repo}
	ds := check.DesignsOf(p)
	if base == "" {
		vs := check.AllDesigns(t, "HEAD", ds, m.DesignTolerate)
		return append(out, check.DesignFindings(vs, ix.Severity("FLOW-02"))...), nil
	}
	mb, err := git(repo, "merge-base", base, "HEAD")
	if err != nil {
		return nil, err
	}
	files, err := changedFiles(repo, mb)
	if err != nil {
		return nil, err
	}
	vs := check.TouchedDesigns(t, mb, "HEAD", files, ds, m.DesignTolerate)
	out = append(out, check.DesignFindings(vs, ix.Severity("FLOW-02"))...)
	log, err := git(repo, "log", "--format=%H%x00%B%x1e", mb+"..HEAD")
	if err != nil {
		return nil, err
	}
	for _, rec := range strings.Split(log, "\x1e") {
		sha, body, ok := strings.Cut(strings.TrimLeft(rec, "\n"), "\x00")
		if ok {
			_, bad := check.Trailers(sha, body)
			out = append(out, bad...)
		}
	}
	return out, nil
}

// changedFiles lists the paths changed from base to HEAD, renames found.
func changedFiles(repo, base string) ([]check.Changed, error) {
	out, err := git(repo, "diff", "--name-status", "-z", "-M", base, "HEAD")
	if err != nil {
		return nil, err
	}
	var files []check.Changed
	f := strings.Split(strings.TrimSuffix(out, "\x00"), "\x00")
	for i := 0; i < len(f); i++ {
		st := f[i]
		if st == "" || i+1 >= len(f) {
			break
		}
		c := check.Changed{Status: st[0], Path: f[i+1]}
		i++
		if st[0] == 'R' || st[0] == 'C' {
			if i+1 >= len(f) {
				break
			}
			c.Old, c.Path = c.Path, f[i+1]
			i++
		}
		files = append(files, c)
	}
	return files, nil
}

// gitTree reads files and directories of a checkout's revisions.
type gitTree struct{ dir string }

func (g gitTree) Show(rev, path string) (string, bool) {
	out, err := git(g.dir, "show", rev+":"+path)
	return out, err == nil
}

func (g gitTree) List(rev, dir string) []string {
	out, err := git(g.dir, "ls-tree", "-r", "--name-only", "-z", rev, "--", dir)
	if err != nil || out == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(out, "\x00"), "\x00")
}
