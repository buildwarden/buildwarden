package check

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"

	"go.yaml.in/yaml/v3"
)

// ProfileVersion is the profile file format this wf reads (LOOP-01).
const ProfileVersion = 1

// Profile is a component's profile file: the entries of its profile under
// the Loop Contract that the loop's tools read (LOOP-01). A key the file
// leaves out is the term none: its field is the zero value, never this
// repository's entry.
type Profile struct {
	Version   int    `yaml:"version"`
	Component string `yaml:"component"`
	// Trunk repeats the profile's Trunk entry. wf never uses it: the trunk
	// is the repository's config.yaml entry's, and CheckTrunk fails a file
	// that differs from it.
	Trunk   string `yaml:"trunk"`
	Branch  string `yaml:"branch"`
	Designs struct {
		Dir      string `yaml:"dir"`
		Template string `yaml:"template"`
	} `yaml:"designs"`
	// FastGate is the commands run in the checkout, each an argv that must
	// exit 0 (FLOW-03, FLOW-17).
	FastGate [][]string `yaml:"fast_gate"`
	Impact   struct {
		// Keys are the front-matter keys the component adds to the loop's.
		Keys []string `yaml:"keys"`
		// Values restricts a key to the values listed for it.
		Values map[string][]string `yaml:"values"`
	} `yaml:"impact"`
	ReferenceDocs struct {
		Implement []string `yaml:"implement"`
		Guides    []string `yaml:"guides"`
	} `yaml:"reference_docs"`
	Rules ProfileRules `yaml:"rules"`
	Areas struct {
		Edges []string `yaml:"edges"`
	} `yaml:"areas"`
	ToolchainCheck []string `yaml:"toolchain_check"`
	Native         struct {
		// Prepare and Check map a host name to the commands run for it
		// before a session (the wrapper) and at install (LOOP-05).
		Prepare map[string][][]string `yaml:"prepare"`
		Check   map[string][][]string `yaml:"check"`
	} `yaml:"native"`
}

// ProfileRules is the profile file's rules key: where the component keeps
// its rules and their enforcement (FLOW-14, LINT-01).
type ProfileRules struct {
	Dir       string   `yaml:"dir"`
	Paths     []string `yaml:"paths"`
	Machinery []string `yaml:"machinery"`
	// RuleJobs is a regular expression matching a CI step that runs the
	// rule enforcement (FLOW-14).
	RuleJobs string `yaml:"rule_jobs"`
	// Index is LINT-01's rule-to-check map; empty is none.
	Index string `yaml:"index"`
}

// ParseProfile reads a profile file. name is what an error names: the
// file's path, and the revision it was read at when it came from git. A
// file that does not parse, has a key this format does not have, or a
// version this wf does not know is an error, never read as none.
func ParseProfile(name string, data []byte) (*Profile, error) {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	var p Profile
	if err := dec.Decode(&p); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, fmt.Errorf("profile file %s is empty", name)
		}
		return nil, fmt.Errorf("profile file %s: %v", name, err)
	}
	if p.Version != ProfileVersion {
		return nil, fmt.Errorf("profile file %s: version %d is not one this wf reads "+
			"(it reads %d)", name, p.Version, ProfileVersion)
	}
	return &p, nil
}

// ReadProfile parses the profile file at path.
func ReadProfile(path string) (*Profile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("profile file: %v", err)
	}
	return ParseProfile(path, data)
}

// CheckTrunk fails a profile file whose trunk differs from the trunk the
// repository's config.yaml entry gives, which is the one wf uses (LOOP-01).
// A file that leaves trunk out repeats nothing and passes.
func (p *Profile) CheckTrunk(name, entryTrunk string) error {
	if p.Trunk != "" && p.Trunk != entryTrunk {
		return fmt.Errorf("profile file %s gives trunk %q, but config.yaml's entry gives %q, "+
			"which wf uses: make them equal (LOOP-01)", name, p.Trunk, entryTrunk)
	}
	return nil
}
