package queue

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"
)

// The two modes of an area (FLOW-05). Two items that both change an exclusive
// area collide; any number may change an ordered one side by side.
const (
	ModeExclusive = "exclusive"
	ModeOrdered   = "ordered"
)

// AreaMap is areas.yaml at the root of the workflow branch: the areas of the
// queue's repository, by which two items are related (FLOW-05, PROFILE-01).
// It is queue state, changed in an attended session on the human's word.
type AreaMap struct {
	Version int `yaml:"version"`
	// Repository is the repository the map describes, as owner/name.
	Repository string `yaml:"repository"`
	// CatchAll names the exclusive area every path that no pattern matches
	// belongs to.
	CatchAll string          `yaml:"catch_all"`
	Areas    map[string]Area `yaml:"areas"`
	Implies  Implies         `yaml:"implies"`
	// WrittenSeams lists the seams that have a specification: two items that
	// both change a seam not listed here collide.
	WrittenSeams []string `yaml:"written_seams"`
}

// Area is one area of the map.
type Area struct {
	Mode string `yaml:"mode"`
	// Paths are patterns from the repository root: a directory ending in /,
	// which matches everything under it; a file; or a file name with * in
	// it, which matches within one directory.
	Paths []string `yaml:"paths"`
	// Reaches lists the areas this one depends on, which is the import
	// graph. Only an exclusive area has it, and it names only exclusive
	// areas.
	Reaches []string `yaml:"reaches"`
}

// Implies says which areas a value in a design's front matter stands for, so
// wf holds none of the repository's vocabulary (LOOP-08).
type Implies struct {
	// Drivers gives each driver's own area.
	Drivers map[string]string `yaml:"drivers"`
	// Shared lists the areas that hold a path every driver depends on.
	Shared []string `yaml:"shared"`
	// Surfaces gives each surface's areas.
	Surfaces map[string][]string `yaml:"surfaces"`
}

// ParseAreas reads an area map. A key the format does not have is an error.
func ParseAreas(data []byte) (*AreaMap, error) {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	m := &AreaMap{}
	if err := dec.Decode(m); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, errors.New("the file is empty")
		}
		return nil, err
	}
	return m, nil
}

// LoadAreas reads the area map at path.
func LoadAreas(path string) (*AreaMap, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	m, err := ParseAreas(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return m, nil
}

// loadAreas reads the optional areas.yaml. With none, Areas stays nil and
// the queue's repository is one exclusive area.
func (s *State) loadAreas() error {
	p := filepath.Join(s.Dir, "areas.yaml")
	if _, err := os.Stat(p); errors.Is(err, os.ErrNotExist) {
		return nil
	}
	m, err := LoadAreas(p)
	if err != nil {
		return err
	}
	s.Areas = m
	return nil
}

// Names returns the map's area names, sorted.
func (m *AreaMap) Names() []string { return slices.Sorted(maps.Keys(m.Areas)) }

// Has reports whether the map has the named area.
func (m *AreaMap) Has(name string) bool {
	_, ok := m.Areas[name]
	return ok
}

// Exclusive reports whether the named area is in the map and exclusive. A
// mode that is missing or neither exclusive nor ordered counts as exclusive:
// only an explicit ordered area relaxes anything, and wf validate reports the
// mode (FLOW-05).
func (m *AreaMap) Exclusive(name string) bool {
	a, ok := m.Areas[name]
	return ok && a.Mode != ModeOrdered
}

// Reaches reports whether area from lists area to in its reaches.
func (m *AreaMap) Reaches(from, to string) bool {
	return slices.Contains(m.Areas[from].Reaches, to)
}

// AreaOf returns the area the path belongs to: the one whose matching pattern
// has the longest text before any *, or the catch-all when no pattern
// matches. Two areas matching equally is an error, which Validate reports for
// the map before any path is asked.
func (m *AreaMap) AreaOf(p string) (string, error) {
	best, length, tied := m.CatchAll, -1, ""
	for _, name := range m.Names() {
		for _, pat := range m.Areas[name].Paths {
			n, ok := matchPattern(pat, p)
			switch {
			case !ok || n < length:
			case n > length:
				best, length, tied = name, n, ""
			case name != best:
				tied = name
			}
		}
	}
	if tied != "" {
		return "", fmt.Errorf("areas.yaml: %s matches areas %s and %s equally", p, best,
			tied)
	}
	return best, nil
}

// patternText is the pattern's text before any *, by whose length AreaOf
// ranks the patterns that match a path.
func patternText(pat string) string {
	text, _, _ := strings.Cut(pat, "*")
	return text
}

// matchPattern reports whether the pattern matches the path, and the length
// of the pattern's text before any *.
func matchPattern(pat, p string) (int, bool) {
	n := len(patternText(pat))
	switch {
	case strings.Contains(pat, "*"):
		return n, globMatch(pat, p)
	case strings.HasSuffix(pat, "/"):
		return n, strings.HasPrefix(p, pat)
	}
	return n, p == pat
}

// globMatch matches s against a pattern in which * stands for any run of
// characters other than /, so a * never leaves its directory. path.Match is
// not used: it gives ?, [ and \ meanings the map's format does not have.
func globMatch(pat, s string) bool {
	head, rest, star := strings.Cut(pat, "*")
	if !star {
		return s == pat
	}
	if !strings.HasPrefix(s, head) {
		return false
	}
	s = s[len(head):]
	for i := 0; ; i++ {
		if globMatch(rest, s[i:]) {
			return true
		}
		if i == len(s) || s[i] == '/' {
			return false
		}
	}
}

// validateAreas checks areas.yaml and its fit with config.yaml. It has no
// tree to check the map against: CheckAreas compares it with one.
func (s *State) validateAreas() []string {
	m := s.Areas
	if m == nil {
		return nil
	}
	var errs []string
	bad := func(format string, a ...any) {
		errs = append(errs, "areas.yaml: "+fmt.Sprintf(format, a...))
	}
	switch repo := s.Config.Repository; {
	case repo == "":
		bad("repository %s is not config.yaml's repository, which is not set",
			m.Repository)
	case m.Repository != repo:
		bad("repository %s is not config.yaml's repository %s", m.Repository, repo)
	}
	if m.Version != 1 {
		bad("version %d is not 1", m.Version)
	}
	m.checkCatchAll(bad)
	for _, name := range m.Names() {
		m.checkArea(name, bad)
	}
	m.checkPatterns(bad)
	m.checkImplies(bad)
	return errs
}

func (m *AreaMap) checkCatchAll(bad func(string, ...any)) {
	if m.CatchAll == "" {
		bad("catch_all is not set (it names an exclusive area with no paths)")
		return
	}
	if a, ok := m.Areas[m.CatchAll]; !ok || a.Mode != ModeExclusive || len(a.Paths) > 0 {
		bad("catch_all %s does not name an exclusive area with no paths", m.CatchAll)
	}
}

func (m *AreaMap) checkArea(name string, bad func(string, ...any)) {
	a := m.Areas[name]
	switch {
	case a.Mode == "":
		bad("area %s has no mode", name)
	case !oneOf(a.Mode, ModeExclusive, ModeOrdered):
		bad("area %s mode %q is not exclusive or ordered", name, a.Mode)
	}
	if len(a.Paths) == 0 && name != m.CatchAll {
		bad("area %s has no paths (only catch_all may have none)", name)
	}
	if a.Mode == ModeOrdered && len(a.Reaches) > 0 {
		bad("area %s is ordered and has reaches (only an exclusive area may)", name)
	}
	for _, r := range a.Reaches {
		switch {
		case !m.Has(r):
			bad("area %s reaches %s, which the map lacks", name, r)
		case m.Areas[r].Mode == ModeOrdered:
			bad("area %s reaches %s, which is ordered "+
				"(reaches names only exclusive areas)", name, r)
		}
	}
}

// checkPatterns reports two areas whose patterns have the same text before
// any *: the one way two areas can match a path equally, since each matching
// pattern's text is a prefix of the path.
func (m *AreaMap) checkPatterns(bad func(string, ...any)) {
	first := map[string]string{}
	said := map[string]bool{}
	for _, name := range m.Names() {
		for _, pat := range m.Areas[name].Paths {
			text := patternText(pat)
			other, seen := first[text]
			if !seen {
				first[text] = name
				continue
			}
			key := other + "\x00" + name + "\x00" + text
			if other == name || said[key] {
				continue
			}
			said[key] = true
			bad("areas %s and %s both list a pattern with the text %s before any *",
				other, name, text)
		}
	}
}

func (m *AreaMap) checkImplies(bad func(string, ...any)) {
	lacks := func(key, area string) {
		if !m.Has(area) {
			bad("implies.%s names %s, which the map lacks", key, area)
		}
	}
	for _, d := range slices.Sorted(maps.Keys(m.Implies.Drivers)) {
		lacks("drivers."+d, m.Implies.Drivers[d])
	}
	for _, a := range m.Implies.Shared {
		lacks("shared", a)
	}
	for _, sf := range slices.Sorted(maps.Keys(m.Implies.Surfaces)) {
		for _, a := range m.Implies.Surfaces[sf] {
			lacks("surfaces."+sf, a)
		}
	}
}

// Edge is one dependency of the component's code: a package's directory, and
// the directory of a package it or its tests depend on, directly or not, both
// from the repository root.
type Edge struct{ From, To string }

// ParseEdges reads the lines "<dir> <dir>" that the component's own command
// prints (the profile's Areas entry names it). wf runs no toolchain itself
// (LOOP-08).
func ParseEdges(r io.Reader) ([]Edge, error) {
	var out []Edge
	sc := bufio.NewScanner(r)
	for n := 1; sc.Scan(); n++ {
		fields := strings.Fields(sc.Text())
		if len(fields) == 0 {
			continue
		}
		if len(fields) != 2 {
			return nil, fmt.Errorf("edges: line %d: %q is not two directories", n,
				sc.Text())
		}
		out = append(out, Edge{fields[0], fields[1]})
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("edges: %w", err)
	}
	return out, nil
}

// AreasCheck is what CheckAreas found. It is a report, not a gate.
type AreasCheck struct {
	// Lines is every line of the report, in a stable order.
	Lines []string
	// Differences counts the lines that say the map and the tree differ. A
	// pattern that is expected to match nothing until an open item merges,
	// and the line that says reaches was not compared, are not among them.
	Differences int
}

func (c *AreasCheck) differs(format string, a ...any) {
	c.Lines = append(c.Lines, fmt.Sprintf(format, a...))
	c.Differences++
}

// expected adds the line for a pattern that matches no file because the open
// item's design adds its tree. It is not a difference.
func (c *AreasCheck) expected(pat, area, item string) {
	c.Lines = append(c.Lines, fmt.Sprintf(
		"pattern: %s of area %s matches no file: expected until %s merges", pat, area,
		item))
}

// CheckAreas compares the area map with a tree, as its file paths from the
// repository root, and with the component's dependency edges when compared is
// set. It reports each path that falls to the catch-all, each pattern that
// matches no file, and, between exclusive areas, each dependency reaches
// lacks and each reaches entry no dependency supports.
func (s *State) CheckAreas(files []string, edges []Edge, compared bool) (AreasCheck, error) {
	var c AreasCheck
	m := s.Areas
	if m == nil {
		return c, fmt.Errorf("%s has no areas.yaml: there is no map to compare", s.Dir)
	}
	for _, p := range slices.Sorted(slices.Values(files)) {
		area, err := m.AreaOf(p)
		if err != nil {
			return c, err
		}
		if area == m.CatchAll {
			c.differs("catch-all: %s is in no area's paths (it falls to %s)", p, area)
		}
	}
	s.checkUnmatched(&c, files)
	if !compared {
		c.Lines = append(c.Lines, "reaches: not compared")
		return c, nil
	}
	return c, m.checkReaches(&c, edges)
}

// checkUnmatched reports each pattern that matches no file. A pattern whose
// area an open item declares is expected to match nothing until that item
// merges: its design adds the tree, and the area was put in the map ahead of
// it.
func (s *State) checkUnmatched(c *AreasCheck, files []string) {
	m := s.Areas
	for _, name := range m.Names() {
		for _, pat := range m.Areas[name].Paths {
			if slices.ContainsFunc(files, func(p string) bool {
				_, ok := matchPattern(pat, p)
				return ok
			}) {
				continue
			}
			if item := s.declares(name); item != "" {
				c.expected(pat, name, item)
				continue
			}
			c.differs("pattern: %s of area %s matches no file", pat, name)
		}
	}
}

// declares returns the open item that declares the area, the first by name,
// or "".
func (s *State) declares(area string) string {
	var names []string
	for _, f := range s.Features {
		// An item's areas in this map are its bare names when it is homed in
		// this map's repository, and its prefixed ones otherwise.
		if f.Status == "done" || f.Areas == nil {
			continue
		}
		if slices.Contains(s.AreasIn(f, s.Config.Repository), area) {
			names = append(names, f.Feature)
		}
	}
	if len(names) == 0 {
		return ""
	}
	return slices.Min(names)
}

// checkReaches compares reaches with the edges, between exclusive areas: an
// ordered area reaches nothing and is reached by nothing, whatever it
// imports.
func (m *AreaMap) checkReaches(c *AreasCheck, edges []Edge) error {
	// made is, for each dependency between two exclusive areas, the first
	// edge in sorted order that makes it.
	made := map[Edge]Edge{}
	sorted := slices.SortedFunc(slices.Values(edges), func(a, b Edge) int {
		return strings.Compare(a.From+"\x00"+a.To, b.From+"\x00"+b.To)
	})
	for _, e := range sorted {
		from, err := m.dirArea(e.From)
		if err != nil {
			return err
		}
		to, err := m.dirArea(e.To)
		if err != nil {
			return err
		}
		dep := Edge{from, to}
		if _, seen := made[dep]; seen || from == to {
			continue
		}
		if m.Exclusive(from) && m.Exclusive(to) {
			made[dep] = e
		}
	}
	deps := slices.SortedFunc(maps.Keys(made), func(a, b Edge) int {
		return strings.Compare(a.From+"\x00"+a.To, b.From+"\x00"+b.To)
	})
	for _, name := range m.Names() {
		for _, dep := range deps {
			if dep.From == name && !m.Reaches(name, dep.To) {
				c.differs("reaches: %s lacks %s (%s depends on %s)", name, dep.To,
					made[dep].From, made[dep].To)
			}
		}
		for _, r := range slices.Sorted(slices.Values(m.Areas[name].Reaches)) {
			if _, ok := made[Edge{name, r}]; ok {
				continue
			}
			c.differs("reaches: %s lists %s, which no dependency supports", name, r)
		}
	}
	return nil
}

// dirArea returns the area of a package's directory: the area a file in it
// would have by the directory patterns. A file pattern never places a
// directory.
func (m *AreaMap) dirArea(dir string) (string, error) {
	return m.AreaOf(path.Clean(dir) + "/")
}

// WithAreas returns the state with a candidate map in place of areas.yaml,
// for wf validate -areas to judge the candidate by. The state is not changed.
func (s *State) WithAreas(m *AreaMap) *State {
	c := *s
	c.Areas = m
	return &c
}

// RecordAreas records in main.yaml what wf areas -check found for the commit
// of origin/main, so QUEUE.md shows it.
func (s *State) RecordAreas(sha string, c AreasCheck) error {
	m := Main{}
	if s.Main != nil {
		m = *s.Main
	}
	m.Areas = &AreasResult{SHA: sha, Differences: c.Differences, Lines: c.Lines}
	s.Main = &m
	return s.SaveMain()
}
