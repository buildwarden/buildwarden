package check

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"go.yaml.in/yaml/v3"
)

// workflowRunsRe matches a step's run: or uses: value, or a job's uses:,
// that runs the rule enforcement (FLOW-14): tier 1, golangci-lint or the
// workflow tools, directly or through the make targets that run them without
// naming them (Makefile: lint, enforcement).
var workflowRunsRe = regexp.MustCompile(
	`tier1|golangci-lint|tools/workflow|\bmake\s+(lint|enforcement)\b`)

// shellCommentRe matches a whole-line shell comment in a run: script. It is
// removed before matching so that a script's comment naming a tool does not
// make its job rule-running. A trailing comment is left in: telling it from
// a # inside quotes or ${#x} needs a shell parser, and a false positive
// there fails closed.
var shellCommentRe = regexp.MustCompile(`(?m)^\s*#.*$`)

// guarded is what FLOW-14 guards in one workflow file.
type guarded struct {
	// bodies holds each rule-running job's canonical body. The job key is
	// not part of it: the key runs nothing, so a rename alone is no change.
	bodies []string
	// edges holds (downstream job key, needed rule-running job's body) for
	// every needs: entry on a rule-running job. The downstream key is kept
	// because a gate's name is what branch protection requires (ci-ok).
	edges map[[2]string]bool
}

// workflowGuardChanged reports whether a workflow file's change, from before to
// after, changes the rule enforcement: a rule-running job's body is added,
// removed or changed, or a job stops needing a rule-running job. Adding a
// needs: entry is not a change, so a new job can join the required check.
// A nil side is a missing file and has no jobs, so adding or deleting a
// workflow with a rule-running job is guarded. A side that does not parse is
// an error: fail closed and say why.
func workflowGuardChanged(before, after []byte) (bool, error) {
	a, err := guardedParts(before)
	if err != nil {
		return false, fmt.Errorf("merge base: %w", err)
	}
	b, err := guardedParts(after)
	if err != nil {
		return false, fmt.Errorf("branch: %w", err)
	}
	if strings.Join(a.bodies, "\x00") != strings.Join(b.bodies, "\x00") {
		return true, nil
	}
	for e := range a.edges {
		if !b.edges[e] {
			return true, nil
		}
	}
	return false, nil
}

// guardedParts decodes a workflow and returns its guarded parts. Decoding
// resolves anchors and aliases, and the bodies are compared as canonical
// JSON, so YAML comments and formatting do not count and an alias cannot
// hide a change.
func guardedParts(text []byte) (guarded, error) {
	g := guarded{edges: map[[2]string]bool{}}
	var doc map[string]any
	if err := yaml.Unmarshal(text, &doc); err != nil {
		return g, err
	}
	raw, ok := doc["jobs"]
	if !ok || raw == nil {
		return g, nil
	}
	jobs, ok := plain(raw).(map[string]any)
	if !ok {
		return g, fmt.Errorf("jobs is not a mapping")
	}
	all := map[string]map[string]any{}
	for k, j := range jobs {
		job, ok := j.(map[string]any)
		if !ok {
			return g, fmt.Errorf("job %s is not a mapping", k)
		}
		all[k] = job
	}
	body := map[string]string{} // rule-running jobs only
	for k, job := range all {
		if !runsRules(job) {
			continue
		}
		b, err := json.Marshal(job)
		if err != nil {
			return g, fmt.Errorf("job %s: %w", k, err)
		}
		body[k] = string(b)
		g.bodies = append(g.bodies, string(b))
	}
	sort.Strings(g.bodies)
	for k, job := range all {
		for _, n := range needs(job) {
			if b, ok := body[n]; ok {
				g.edges[[2]string{k, b}] = true
			}
		}
	}
	return g, nil
}

// runsRules reports whether a job runs the rule enforcement: its uses:, or a
// step's uses: or run: with whole-line comments removed, matches
// workflowRunsRe. Keys, names, needs:, if:, env:, with: and
// working-directory: are not searched; they only name things (FLOW-14,
// rule-running-line-match Decision 1).
func runsRules(job map[string]any) bool {
	if workflowRunsRe.MatchString(scalar(job["uses"])) {
		return true
	}
	steps, _ := job["steps"].([]any)
	for _, s := range steps {
		step, _ := s.(map[string]any)
		if workflowRunsRe.MatchString(scalar(step["uses"])) ||
			workflowRunsRe.MatchString(shellCommentRe.ReplaceAllString(scalar(step["run"]), "")) {
			return true
		}
	}
	return false
}

// needs returns a job's needs:, written as one name or a list.
func needs(job map[string]any) []string {
	switch n := job["needs"].(type) {
	case string:
		return []string{n}
	case []any:
		out := make([]string, 0, len(n))
		for _, v := range n {
			out = append(out, scalar(v))
		}
		return out
	}
	return nil
}

func scalar(v any) string {
	if v == nil {
		return ""
	}
	return fmt.Sprint(v)
}

// plain turns the mappings yaml decodes with non-string keys into
// map[string]any, which encoding/json can encode.
func plain(v any) any {
	switch x := v.(type) {
	case map[string]any:
		for k, e := range x {
			x[k] = plain(e)
		}
		return x
	case map[any]any:
		out := make(map[string]any, len(x))
		for k, e := range x {
			out[fmt.Sprint(k)] = plain(e)
		}
		return out
	case []any:
		for i, e := range x {
			x[i] = plain(e)
		}
		return x
	}
	return v
}
