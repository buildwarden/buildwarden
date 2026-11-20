package check

import (
	"fmt"
	"regexp"
	"strings"
)

// classifier is FLOW-14's classification of one repository's paths: its
// profile's rules keys, as the profile file is at the branch's merge base
// with the repository's trunk, never as the branch has them (Decision 2 of
// docs/design/loop-repository.md).
type classifier struct {
	// paths and machinery are path prefixes; a prefix that names a file
	// matches that file only.
	paths, machinery []string
	// runs matches a CI step that runs the rule enforcement; nil, none does.
	runs *regexp.Regexp
	// profile is the profile file's path, which is always a rule path.
	profile string
}

// newClassifier builds the classification from a profile's rules keys. A
// rule_jobs that does not compile is an error naming the file.
func newClassifier(name, profilePath string, r ProfileRules) (classifier, error) {
	k := classifier{paths: r.Paths, machinery: r.Machinery, profile: profilePath}
	if r.RuleJobs != "" {
		re, err := regexp.Compile(r.RuleJobs)
		if err != nil {
			return k, fmt.Errorf("profile file %s: rules.rule_jobs: %v", name, err)
		}
		k.runs = re
	}
	return k, nil
}

// kindOf classifies a path. A workflow file is ordinary here; whether its
// change touches a rule-running job decides (workflowGuardChanged).
func (k classifier) kindOf(p string) pathKind {
	switch {
	case k.profile != "" && p == k.profile, underAny(p, k.paths):
		return rulePath
	case underAny(p, k.machinery):
		return machinery
	}
	return ordinary
}

// underAny reports whether p is one of the prefixes or under one that ends
// in a slash.
func underAny(p string, prefixes []string) bool {
	for _, pre := range prefixes {
		if p == pre || (strings.HasSuffix(pre, "/") && strings.HasPrefix(p, pre)) {
			return true
		}
	}
	return false
}

// classifierAt is the classification of the repository being checked, read
// at the branch's merge base with its trunk (Decision 2). A repository whose
// entry names no profile has not adopted and guards nothing (Decision 3).
// Where the merge base has no profile file: a branch that has none either
// forked before the adoption merged and must merge its trunk; a branch that
// adds one has every path under the new file's rule and machinery paths
// classified as a rule path, since it cannot classify its own edits.
func (c *checker) classifierAt() (classifier, error) {
	r := c.at
	if r.profilePath == "" {
		return classifier{}, nil
	}
	name := r.name + ":" + r.profilePath
	mb, err := c.git("merge-base", c.trunk(), c.f.Branch)
	if err != nil {
		return classifier{}, err
	}
	text, baseErr := c.git("show", mb+":"+r.profilePath)
	if baseErr == nil {
		p, err := ParseProfile(name+" at the merge base "+short(mb), []byte(text))
		if err != nil {
			return classifier{}, err
		}
		return newClassifier(name, r.profilePath, p.Rules)
	}
	text, err = c.git("show", "refs/heads/"+c.f.Branch+":"+r.profilePath)
	if err != nil {
		return classifier{}, fmt.Errorf("%s adopted with the profile file %s, which %s "+
			"has at neither its merge base %s nor its head: merge %s into it "+
			"(FLOW-14)",
			r.name, r.profilePath, c.f.Branch, short(mb), c.trunk())
	}
	p, err := ParseProfile(name+" on "+c.f.Branch, []byte(text))
	if err != nil {
		return classifier{}, err
	}
	all := append(append([]string{}, p.Rules.Paths...), p.Rules.Machinery...)
	k, err := newClassifier(name, r.profilePath, ProfileRules{RuleJobs: p.Rules.RuleJobs})
	k.paths = all
	return k, err
}
