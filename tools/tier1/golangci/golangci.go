// Package golangci runs tier 1's golangci-lint config over the diff and maps
// each issue to the rule it enforces (GLOBAL-01, GLOBAL-02, GLOBAL-03).
package golangci

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/buildwarden/buildwarden/tools/tier1"
	"github.com/buildwarden/buildwarden/tools/tier1/report"
)

// Install is the command that installs the golangci-lint tier 1 expects.
const Install = "go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.5.0"

// linters maps a golangci-lint linter to its rule and check ID.
var linters = map[string][2]string{
	"revive":    {"GLOBAL-01", "exported-doc"},
	"gofmt":     {"GLOBAL-02", "gofmt-touched"},
	"errorlint": {"GLOBAL-03", "error-wrap"},
	"forbidigo": {"GLOBAL-03", "error-wrap"},
}

// MissingError is golangci-lint not on PATH.
type MissingError struct{}

func (MissingError) Error() string { return "golangci-lint is not on PATH" }

// Fix is the install command.
func (MissingError) Fix() string { return Install }

// Run lints the root module in repo against base and returns the mapped
// issues.
func Run(repo, base string, sev func(string) report.Severity) ([]report.Finding, error) {
	bin, err := exec.LookPath("golangci-lint")
	if err != nil {
		return nil, MissingError{}
	}
	cfg, err := os.CreateTemp("", "tier1-golangci-*.yml")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.Remove(cfg.Name()) }()
	if _, err := cfg.Write(tier1.GolangciYAML); err != nil {
		return nil, err
	}
	if err := cfg.Close(); err != nil {
		return nil, err
	}
	cmd := exec.Command(bin, "run", "-c", cfg.Name(), "--new-from-merge-base="+base,
		"--output.json.path=stdout", "--output.text.path=", "--show-stats=false", "./...")
	cmd.Dir = repo
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err = cmd.Run()
	var exit *exec.ExitError
	// Exit 1 means issues were found; anything else is a failed run.
	if err != nil && (!errors.As(err, &exit) || exit.ExitCode() != 1) {
		return nil, fmt.Errorf("golangci-lint: %w: %s", err, stderr.String())
	}
	return Parse(stdout.Bytes(), sev)
}

// Parse maps golangci-lint's JSON output to findings.
func Parse(data []byte, sev func(string) report.Severity) ([]report.Finding, error) {
	var out struct {
		Issues []struct {
			FromLinter string
			Text       string
			Pos        struct {
				Filename string
				Line     int
			}
		}
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("golangci-lint output: %w", err)
	}
	var fs []report.Finding
	for _, is := range out.Issues {
		m, ok := linters[is.FromLinter]
		if !ok {
			return nil, fmt.Errorf("golangci-lint: linter %q is not in tier 1's map",
				is.FromLinter)
		}
		fs = append(fs, report.Finding{Rule: m[0], Severity: sev(m[0]),
			File: filepath.ToSlash(is.Pos.Filename), Line: is.Pos.Line, Check: m[1],
			Message: is.FromLinter + ": " + is.Text})
	}
	return fs, nil
}
