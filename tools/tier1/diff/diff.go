// Package diff resolves tier 1's base and reads what the diff from that base
// to HEAD changed: paths, added and changed lines, and Tier1-Allow trailers
// (LINT-04, LINT-05). Paths stay /-separated as git prints them.
package diff

import (
	"bufio"
	"bytes"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"github.com/buildwarden/buildwarden/tools/tier1/report"
)

// File is one changed path. Status is git's letter: A, M, D, R, C or T. Old
// is the source path of a rename or copy.
type File struct {
	Status byte
	Path   string
	Old    string
}

// Range is an inclusive span of HEAD line numbers the diff added or changed.
type Range struct{ Start, End int }

// Allow is one Tier1-Allow trailer: findings against Rule on paths matching
// Glob are suppressed, and Reason is printed with them.
type Allow struct {
	Rule, Glob, Reason, Commit string
}

// Diff is the change from Base (the merge base) to Head.
type Diff struct {
	Dir    string
	Base   string
	Head   string
	Files  []File
	Allows []Allow
	// Bad lists trailers that cannot be applied: no reason, or not in the
	// "<RULE-ID> <path-glob>: <reason>" form.
	Bad   []report.Finding
	lines map[string][]Range
	paths map[string]bool
}

// BaseError is an unresolvable base. Fix is the command that resolves it
// (UX-04); tier 1 never falls back to a whole-repo run.
type BaseError struct {
	Rev   string
	Cause string
	fix   string
}

func (e *BaseError) Error() string {
	return fmt.Sprintf("cannot resolve base %q: %s", e.Rev, e.Cause)
}

// Fix is the command that makes the base resolvable.
func (e *BaseError) Fix() string { return e.fix }

// Load resolves the merge base of rev and HEAD in dir and reads the diff.
func Load(dir, rev string) (*Diff, error) {
	d := &Diff{Dir: dir, lines: map[string][]Range{}, paths: map[string]bool{}}
	head, err := d.git("rev-parse", "--verify", "HEAD^{commit}")
	if err != nil {
		return nil, fmt.Errorf("%s is not a git checkout with a commit: %w", dir, err)
	}
	d.Head = strings.TrimSpace(head)
	if _, err := d.git("rev-parse", "--verify", "--quiet", rev+"^{commit}"); err != nil {
		return nil, &BaseError{Rev: rev, Cause: "no such commit in this checkout",
			fix: fetchFix(rev)}
	}
	base, err := d.git("merge-base", rev, "HEAD")
	if err != nil {
		return nil, &BaseError{Rev: rev, Cause: "no merge base with HEAD (shallow clone?)",
			fix: "git fetch --unshallow origin, or check out with fetch-depth: 0"}
	}
	d.Base = strings.TrimSpace(base)
	if err := d.readFiles(); err != nil {
		return nil, err
	}
	if err := d.readLines(); err != nil {
		return nil, err
	}
	return d, d.readTrailers()
}

func fetchFix(rev string) string {
	if b, ok := strings.CutPrefix(rev, "origin/"); ok {
		return "git fetch origin " + b
	}
	return "git fetch origin, or pass -base with a commit this checkout has"
}

func (d *Diff) git(args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-c", "core.quotePath=false"}, args...)...)
	cmd.Dir = d.Dir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		return "", fmt.Errorf("git %s: %w: %s", args[0], err, msg)
	}
	return string(out), nil
}

func (d *Diff) readFiles() error {
	out, err := d.git("diff", "--name-status", "-z", "-M", "--no-ext-diff", d.Base, d.Head)
	if err != nil {
		return err
	}
	tok := strings.Split(strings.TrimSuffix(out, "\x00"), "\x00")
	for i := 0; i+1 < len(tok); i += 2 {
		f := File{Status: tok[i][0], Path: tok[i+1]}
		if f.Status == 'R' || f.Status == 'C' {
			if i+2 >= len(tok) {
				return fmt.Errorf("git diff: truncated %s entry", tok[i])
			}
			f.Old, f.Path = tok[i+1], tok[i+2]
			i++
		}
		d.Files = append(d.Files, f)
		d.paths[f.Path] = true
		if f.Old != "" {
			d.paths[f.Old] = true
		}
	}
	return nil
}

var hunkRe = regexp.MustCompile(`^@@ -\d+(?:,\d+)? \+(\d+)(?:,(\d+))? @@`)

func (d *Diff) readLines() error {
	out, err := d.git("diff", "--unified=0", "--no-color", "--no-ext-diff", "-M",
		d.Base, d.Head)
	if err != nil {
		return err
	}
	var path string
	sc := bufio.NewScanner(strings.NewReader(out))
	sc.Buffer(make([]byte, 1<<20), 1<<26)
	for sc.Scan() {
		line := sc.Text()
		if p, ok := strings.CutPrefix(line, "+++ "); ok {
			path = strings.TrimSuffix(strings.TrimPrefix(p, "b/"), "\t")
			continue
		}
		m := hunkRe.FindStringSubmatch(line)
		if m == nil || path == "/dev/null" {
			continue
		}
		start, _ := strconv.Atoi(m[1])
		n := 1
		if m[2] != "" {
			n, _ = strconv.Atoi(m[2])
		}
		if n > 0 {
			d.lines[path] = append(d.lines[path], Range{start, start + n - 1})
		}
	}
	return sc.Err()
}

var (
	trailerRe = regexp.MustCompile(`^Tier1-Allow:\s*(.*)$`)
	allowRe   = regexp.MustCompile(`^([A-Z]+-\d{2})\s+(\S+):(.*)$`)
)

func (d *Diff) readTrailers() error {
	out, err := d.git("log", "--format=%H%x00%B%x1e", d.Base+".."+d.Head)
	if err != nil {
		return err
	}
	for _, rec := range strings.Split(out, "\x1e") {
		sha, body, ok := strings.Cut(strings.TrimLeft(rec, "\n"), "\x00")
		if !ok {
			continue
		}
		for _, line := range strings.Split(body, "\n") {
			m := trailerRe.FindStringSubmatch(strings.TrimSpace(line))
			if m != nil {
				d.trailer(sha, m[1])
			}
		}
	}
	return nil
}

func (d *Diff) trailer(sha, value string) {
	m := allowRe.FindStringSubmatch(strings.TrimSpace(value))
	if m == nil || strings.TrimSpace(m[3]) == "" {
		d.Bad = append(d.Bad, report.Finding{Rule: "LINT-05", Severity: report.Must,
			Check: "suppression",
			Message: fmt.Sprintf("commit %.12s: Tier1-Allow %q needs the form "+
				"\"<RULE-ID> <path-glob>: <reason>\" with a reason", sha, value)})
		return
	}
	d.Allows = append(d.Allows, Allow{Rule: m[1], Glob: m[2],
		Reason: strings.TrimSpace(m[3]), Commit: sha})
}

// Touched reports whether the diff changed path, as either side of a rename.
func (d *Diff) Touched(path string) bool { return d.paths[path] }

// LineChanged reports whether the diff added or changed line in path at HEAD.
func (d *Diff) LineChanged(path string, line int) bool {
	for _, r := range d.lines[path] {
		if line >= r.Start && line <= r.End {
			return true
		}
	}
	return false
}

// Show returns path's content at rev, and ok false when it does not exist
// there.
func (d *Diff) Show(rev, path string) (string, bool) {
	out, err := d.git("show", rev+":"+path)
	return out, err == nil
}

// List returns the paths under dir at rev, or none when dir does not exist
// there.
func (d *Diff) List(rev, dir string) []string {
	out, err := d.git("ls-tree", "-r", "--name-only", "-z", rev, "--", dir)
	if err != nil || out == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(out, "\x00"), "\x00")
}

// OnChangedLines keeps the findings that are file-level or fall on a line the
// diff added or changed (LINT-04).
func (d *Diff) OnChangedLines(fs []report.Finding) []report.Finding {
	var out []report.Finding
	for _, f := range fs {
		if f.Line == 0 || d.LineChanged(f.File, f.Line) {
			out = append(out, f)
		}
	}
	return out
}

// Suppress drops each finding a trailer allows and records the trailers that
// were applied.
func (d *Diff) Suppress(r *report.Report) {
	var kept []report.Finding
	used := map[int]bool{}
	for _, f := range r.Findings {
		i := d.allowFor(f)
		if i < 0 {
			kept = append(kept, f)
			continue
		}
		if !used[i] {
			used[i] = true
			a := d.Allows[i]
			r.Suppressed = append(r.Suppressed, report.Suppression{Rule: a.Rule,
				Path: a.Glob, Reason: a.Reason, Commit: a.Commit})
		}
	}
	r.Findings = kept
}

func (d *Diff) allowFor(f report.Finding) int {
	for i, a := range d.Allows {
		if a.Rule == f.Rule && Match(a.Glob, f.File) {
			return i
		}
	}
	return -1
}

// Match reports whether a /-separated path matches glob. "*" and "?" stay
// within one path element, "**" crosses elements, and a glob ending in "/"
// matches everything under that directory.
func Match(glob, path string) bool {
	if strings.HasSuffix(glob, "/") {
		return strings.HasPrefix(path, glob)
	}
	return globRe(glob).MatchString(path)
}

func globRe(glob string) *regexp.Regexp {
	var b strings.Builder
	b.WriteString("^")
	for i := 0; i < len(glob); i++ {
		switch c := glob[i]; {
		case c == '*' && i+1 < len(glob) && glob[i+1] == '*':
			b.WriteString(".*")
			i++
		case c == '*':
			b.WriteString("[^/]*")
		case c == '?':
			b.WriteString("[^/]")
		default:
			b.WriteString(regexp.QuoteMeta(string(c)))
		}
	}
	b.WriteString("$")
	return regexp.MustCompile(b.String())
}
