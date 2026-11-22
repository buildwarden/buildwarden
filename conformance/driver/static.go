package driverconformance

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// Suite runs the conformance checks against one driver binary.
type Suite struct {
	path string
	// name is the driver's name, from its file's name.
	name string
	// tmp is the temporary directory the driver is given; scratch holds
	// the suite's own files.
	tmp     string
	scratch string
	desc    *description
	// env are variables set for every call, such as a fault of the fake's
	// in the suite's self-test.
	env []string
}

// Result is one check's outcome.
type Result struct {
	// Check names the check.
	Check string
	// Requirements are the requirements of v1.md it holds the driver to.
	Requirements []string
	// Skipped says why the check did not apply, or is "".
	Skipped string
	// Failure says what was wrong, or is "" when the check passed.
	Failure string
}

// New prepares a suite against the driver binary at path.
func New(path string) (*Suite, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	base := strings.TrimSuffix(filepath.Base(abs), ".exe")
	name, ok := strings.CutPrefix(base, "warden-driver-")
	if !ok {
		return nil, fmt.Errorf("%s is not named warden-driver-<name> (DRV-2)", abs)
	}
	scratch, err := os.MkdirTemp("", "driver-conformance-")
	if err != nil {
		return nil, err
	}
	tmp := filepath.Join(scratch, "tmp")
	if err := os.Mkdir(tmp, 0o700); err != nil {
		return nil, err
	}
	return &Suite{path: abs, name: name, tmp: tmp, scratch: scratch}, nil
}

// Close removes the suite's scratch files.
func (s *Suite) Close() error { return os.RemoveAll(s.scratch) }

// check is one check of the static tier.
type check struct {
	name string
	reqs []string
	run  func(s *Suite) (skipped, failure string)
}

// staticChecks are the static tier's checks, in order. The first reads
// describe, which every later check needs.
var staticChecks = []check{
	{"describe", []string{"DRV-36"}, (*Suite).checkDescribe},
	{"declarations", []string{"DRV-1", "DRV-3", "DRV-23", "DRV-C1", "DRV-C4", "DRV-C11"},
		(*Suite).checkDeclarations},
	{"capabilities", []string{"DRV-37"}, (*Suite).checkCapabilities},
	{"undeclared argument", []string{"DRV-C2", "DRV-30", "DRV-68"},
		(*Suite).checkUndeclaredArg},
	{"only the named file", []string{"DRV-C3"}, (*Suite).checkOnlyNamedFile},
	{"tables it does not own", []string{"DRV-C4", "DRV-C5"}, (*Suite).checkUnowned},
	{"config file and key refused", []string{"DRV-C6", "DRV-C7", "DRV-68"},
		(*Suite).checkKeysRefused},
	{"a listed key in another letter case", []string{"DRV-C12", "DRV-C7"},
		(*Suite).checkAnotherCase},
	{"reap with its config file gone", []string{"DRV-C10"}, (*Suite).checkReapFileGone},
	{"probes", []string{"DRV-30", "DRV-31"}, (*Suite).checkProbes},
	{"unlisted commands", []string{"DRV-21", "DRV-23"}, (*Suite).checkUnlisted},
	{"contract version", []string{"DRV-69"}, (*Suite).checkContractVersion},
	{"invalid request", []string{"DRV-70"}, (*Suite).checkInvalidRequest},
	{"reap of an unknown build", []string{"DRV-24"}, (*Suite).checkReapUnknown},
	{"run with no start", []string{"DRV-50"}, (*Suite).checkRunNoStart},
	{"image-list and doctor", []string{"DRV-25", "DRV-26"}, (*Suite).checkReadOnly},
	{"ext with no request", []string{"DRV-34"}, (*Suite).checkExtNoRequest},
}

// Static runs the static tier.
func (s *Suite) Static() []Result {
	var out []Result
	for _, c := range staticChecks {
		r := Result{Check: c.name, Requirements: c.reqs}
		if c.name != "describe" && s.desc == nil {
			r.Skipped = "describe failed"
		} else {
			r.Skipped, r.Failure = c.run(s)
		}
		out = append(out, r)
	}
	return out
}

func (s *Suite) lists(command string) bool { return slices.Contains(s.desc.Commands, command) }

// checkDescribe: exit 0 within 10 s, one JSON object matching the schema and
// nothing else, the same on a second call.
func (s *Suite) checkDescribe() (string, string) {
	first := s.run(call{command: "describe", timeout: 10 * time.Second})
	second := s.run(call{command: "describe", timeout: 10 * time.Second})
	switch {
	case first.timedOut:
		return "", "describe did not exit within 10 s"
	case first.code != 0:
		return "", fmt.Sprintf("describe exited %d: %s", first.code, first.stderr)
	case !bytes.Equal(first.stdout, second.stdout):
		return "", "describe printed something else on a second call"
	}
	out := bytes.TrimSuffix(first.stdout, []byte("\n"))
	if bytes.Contains(out, []byte("\n")) {
		return "", "describe printed more than one line"
	}
	if err := validate("describe", out); err != nil {
		return "", err.Error()
	}
	var d description
	if err := json.Unmarshal(out, &d); err != nil {
		return "", err.Error()
	}
	s.desc = &d
	return "", ""
}

// validate checks data against the schema's definition def.
func validate(def string, data []byte) error {
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(schemaJSON))
	if err != nil {
		return err
	}
	m, _ := doc.(map[string]any)
	id, _ := m["$id"].(string)
	c := jsonschema.NewCompiler()
	if err := c.AddResource(id, doc); err != nil {
		return err
	}
	sch, err := c.Compile(id + "#/$defs/" + def)
	if err != nil {
		return err
	}
	v, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("%s is not JSON: %w", def, err)
	}
	if err := sch.Validate(v); err != nil {
		return fmt.Errorf("%s does not match the schema: %w", def, err)
	}
	return nil
}

// checkDeclarations: the name, the required commands, and the placement of
// every flag and key.
func (s *Suite) checkDeclarations() (string, string) {
	d := s.desc
	switch {
	case d.Name != s.name:
		return "", fmt.Sprintf("describe reports name %q; the file is warden-driver-%s",
			d.Name, s.name)
	case !driverNameRE.MatchString(d.Name):
		return "", fmt.Sprintf("name %q is not of the form [a-z][a-z0-9]{0,31}", d.Name)
	case !s.lists("run") || !s.lists("reap"):
		return "", "run and reap are not both listed"
	}
	if msg := s.checkFlags(); msg != "" {
		return "", msg
	}
	for _, k := range d.Keys {
		if msg := s.checkKey(k.Key, k.Type, k.Aliases, k.Commands); msg != "" {
			return "", msg
		}
	}
	return "", s.checkPlacesFolded()
}

// checkPlacesFolded: no two places where the driver's listed keys may be
// written are equal when ASCII letters are folded (DRV-C4).
func (s *Suite) checkPlacesFolded() string {
	seen := map[string]string{}
	for _, k := range s.desc.Keys {
		for _, p := range s.places(k) {
			f := strings.Map(func(r rune) rune {
				if r >= 'A' && r <= 'Z' {
					return r + 'a' - 'A'
				}
				return r
			}, p)
			if prev, ok := seen[f]; ok && prev != p {
				return fmt.Sprintf("places %s and %s are equal when ASCII "+
					"letters are folded", prev, p)
			}
			seen[f] = p
		}
	}
	return ""
}

// places are where the file may write k: its key, the override of a common
// key, and its old names.
func (s *Suite) places(k describeKey) []string {
	places := append([]string{k.Key}, k.Aliases...)
	if common, ok := strings.CutPrefix(k.Key, "driver."); ok &&
		slices.Contains(commonKeys, common) {
		places = append(places, "driver."+s.desc.Name+"."+common)
	}
	return places
}

// checkFlags checks each flag's name, usage and commands (DRV-C1).
func (s *Suite) checkFlags() string {
	for _, f := range s.desc.Flags {
		if !flagNameRE.MatchString(f.Name) || f.Usage == "" || len(f.Commands) == 0 {
			return fmt.Sprintf("flag %q has a bad name, no usage or no commands",
				f.Name)
		}
		for _, c := range f.Commands {
			if !slices.Contains(flagCommands, c) || !s.lists(c) {
				return fmt.Sprintf("flag --%s names %s, which takes no flag or "+
					"is not listed", f.Name, c)
			}
			if slices.Contains(reservedFlags[""], f.Name) ||
				slices.Contains(reservedFlags[c], f.Name) {
				return fmt.Sprintf("flag --%s is warden's on %s", f.Name, c)
			}
		}
	}
	return ""
}

// checkKey checks one key's place, type, old names and commands (DRV-C4,
// DRV-C11).
func (s *Suite) checkKey(key, typ string, aliases, commands []string) string {
	own, isOwn := strings.CutPrefix(key, "driver."+s.desc.Name+".")
	common, isCommon := strings.CutPrefix(key, "driver.")
	switch {
	case isOwn && slices.Contains(commonKeys, strings.Split(own, ".")[0]):
		return fmt.Sprintf("key %s has a common key's name", key)
	case isOwn:
	case isCommon && slices.Contains(commonKeys, common):
		if typ != "string" {
			return fmt.Sprintf("common key %s is listed with type %s, not string", key,
				typ)
		}
	default:
		return fmt.Sprintf("key %s is neither a common key of version 1 nor under "+
			"driver.%s.", key, s.desc.Name)
	}
	for _, a := range aliases {
		if oldNames[s.desc.Name][a] != key {
			return fmt.Sprintf("old name %s of %s is not the specification's for "+
				"driver %q", a, key, s.desc.Name)
		}
	}
	for _, c := range commands {
		if !s.lists(c) {
			return fmt.Sprintf("key %s names %s, which is not listed", key, c)
		}
	}
	return ""
}

// checkCapabilities: attach exactly when shell, ext exactly when the ext
// list is not empty, image_os exactly when image-prepare is listed.
func (s *Suite) checkCapabilities() (string, string) {
	d := s.desc
	switch {
	case d.Capabilities.Shell != s.lists("attach"):
		return "", "attach is listed when and only when capabilities.shell is not"
	case (len(d.Ext) > 0) != s.lists("ext"):
		return "", "ext is listed when and only when the ext list is not empty is not"
	case (len(d.Capabilities.ImageOS) > 0) != s.lists("image-prepare"):
		return "", "capabilities.image_os is present when and only when image-prepare " +
			"is listed is not"
	}
	return "", ""
}
