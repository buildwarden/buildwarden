package queue

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"
)

// RecordsFile is the program records on the workflow branch: every decision
// FLOW-20 lets the loop take in the human's place, appended by wf record
// (Decision 13 of docs/design/program-north-star.md).
const RecordsFile = "program/records.yaml"

// Record is one entry of the program records.
type Record struct {
	At time.Time `yaml:"at"`
	By string    `yaml:"by"`
	// Kind is one of RecordKinds.
	Kind    string `yaml:"kind"`
	Feature string `yaml:"feature,omitempty"`
	// Repo is the repository the decision is in, owner/name.
	Repo string `yaml:"repo,omitempty"`
	// Head is the commit the decision was taken on: a merged PR's head, the
	// head a finding was found or deferred on, a tagged commit.
	Head string `yaml:"head,omitempty"`
	// Ref names what the decision is about: a PR (#N), a finding's id, a
	// tag, a question, an item filed.
	Ref string `yaml:"ref,omitempty"`
	// Note is the decision and its reason, in a sentence.
	Note string `yaml:"note"`
}

// RecordKinds are the decisions FLOW-20 lets the loop take, and
// security-ack, the human's acknowledgment of a security note at a program
// trunk's merge (FLOW-08's merge gate, point 7). The first ones listed
// under Decided for you are firstKinds.
var RecordKinds = []string{
	"merge", "deferred", "protected", "umbrella", "process-test", "security-note",
	"assumed", "departure", "return", "re-estimate", "sync", "sync-resolution",
	"filed", "relation", "area-map", "tag", "resumed", "escalation", "security-ack",
}

// The refs of a security-note record, which FLOW-08's merge gate reads at
// point 7 (a14 (a)): the design's, written when the umbrella test (U4) or the
// design review accepted that the only trip is GLOBAL-06's relay clause with
// no change to a boundary's posture, and the diff's, written by Review on the
// pull request's head when the diff stays within it.
const (
	SecurityNoteDesign = "design"
	SecurityNoteDiff   = "diff"
)

// firstKinds are listed first under Decided for you: a departure from a
// decision of the human's, a return to Design past the second, and a
// re-estimate (FLOW-20).
var firstKinds = []string{"departure", "return", "re-estimate"}

// LoadRecords reads the program records in the workflow checkout dir; a
// missing file is no record.
func LoadRecords(dir string) ([]Record, error) {
	data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(RecordsFile)))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var rs []Record
	if err := yaml.Unmarshal(data, &rs); err != nil {
		return nil, fmt.Errorf("%s: %w", RecordsFile, err)
	}
	return rs, nil
}

// AppendRecord validates r and appends it to the program records in dir,
// creating the file when there is none.
func AppendRecord(dir string, r Record) error {
	switch {
	case !slices.Contains(RecordKinds, r.Kind):
		return fmt.Errorf("record kind %q is not one of %s", r.Kind,
			strings.Join(RecordKinds, ", "))
	case strings.TrimSpace(r.Note) == "":
		return errors.New("a record says what was decided and why (-note)")
	case r.By == "":
		return errors.New("a record names who decided (-worker)")
	case r.Kind == "security-ack" && Kind(r.By) != "human":
		return errors.New("only the human acknowledges a security note (FLOW-08)")
	case r.Kind == "security-note" && r.Ref != SecurityNoteDesign &&
		r.Ref != SecurityNoteDiff:
		return fmt.Errorf("a security-note record says what was read: -ref %s (U4 or "+
			"the design review) or -ref %s (Review, on the head)", SecurityNoteDesign,
			SecurityNoteDiff)
	case r.Kind == "security-note" && r.Ref == SecurityNoteDiff && len(r.Head) < 7:
		return errors.New("a security-note -ref diff record names the head Review read " +
			"(-head)")
	}
	rs, err := LoadRecords(dir)
	if err != nil {
		return err
	}
	r.At = r.At.UTC().Truncate(time.Second)
	rs = append(rs, r)
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(rs); err != nil {
		return err
	}
	p := filepath.Join(dir, filepath.FromSlash(RecordsFile))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	return os.WriteFile(p, buf.Bytes(), 0o644)
}

// DecidedForYou orders records for the digest's and QUEUE.md's Decided for
// you: a departure, a return past the second and a re-estimate first, then
// the rest, each group in the order recorded.
func DecidedForYou(rs []Record) []Record {
	out := slices.Clone(rs)
	slices.SortStableFunc(out, func(a, b Record) int {
		fa, fb := slices.Contains(firstKinds, a.Kind), slices.Contains(firstKinds, b.Kind)
		switch {
		case fa && !fb:
			return -1
		case fb && !fa:
			return 1
		}
		return 0
	})
	return out
}

// Line renders a record as one Markdown list line.
func (r Record) Line() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s", r.Kind)
	if r.Feature != "" {
		fmt.Fprintf(&b, " **%s**", r.Feature)
	}
	for _, v := range []string{r.Repo, r.Ref} {
		if v != "" {
			fmt.Fprintf(&b, " %s", v)
		}
	}
	if r.Head != "" {
		fmt.Fprintf(&b, " at `%s`", short(r.Head))
	}
	fmt.Fprintf(&b, ": %s (%s, %s)", r.Note, r.By, r.At.Format("2006-01-02"))
	return b.String()
}

// bodyOrder is the order of the landing pull request's sections: what the
// human's merge acknowledges first (deferred findings, security notes read
// by the loop), then the decisions listed first under Decided for you, then
// the rest in RecordKinds order.
var bodyOrder = append([]string{"deferred", "security-note", "protected", "departure",
	"return", "re-estimate"}, RecordKinds...)

// ProgramBody renders the program records for repo as the body of the pull
// request that lands its program trunk (FLOW-20, Decision 12): every
// record of that repository, or of none, one section per kind, so the human's
// squash merge is the sign-off for each.
func ProgramBody(rs []Record, repo string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Program records (FLOW-20)\n\nEvery decision the loop took in "+
		"the human's place for %s, from %s. Merging this pull request acknowledges "+
		"each deferred finding and each security note below; dismiss one with a "+
		"recorded reason in a comment (FLOW-08, LINT-03).\n", repo, RecordsFile)
	seen, found := map[string]bool{}, false
	for _, kind := range bodyOrder {
		if seen[kind] {
			continue
		}
		seen[kind] = true
		var lines []string
		for _, r := range rs {
			if r.Kind == kind && (r.Repo == "" || r.Repo == repo) {
				lines = append(lines, "- "+r.Line())
			}
		}
		if len(lines) > 0 {
			found = true
			fmt.Fprintf(&b, "\n### %s (%d)\n\n%s\n", kind, len(lines),
				strings.Join(lines, "\n"))
		}
	}
	if !found {
		b.WriteString("\nNo record.\n")
	}
	return b.String()
}
