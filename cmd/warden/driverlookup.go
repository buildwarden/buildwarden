package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"

	"github.com/buildwarden/buildwarden/driver"
)

// Finding a driver binary (specs/driver-contract/v1.md, Finding a driver;
// the design's Discovery and trust). A selection is a name or a path. A path
// is used as given. A name is looked up on PATH in order, then in warden's
// own directory, then among the drivers linked into warden; the first match
// wins. Nothing compares owners or modes: what is on PATH is the user's to
// arrange. The one input that may not be the user's, the config file, may
// name a driver but not a file.

// driverPrefix is the start of a driver binary's file name (DRV-2).
const driverPrefix = "warden-driver-"

// Where a driver was found, as driver list and driver.json say it.
const (
	foundPath    = "path"
	foundPATH    = "PATH"
	foundBeside  = "beside-warden"
	foundLinked  = "linked"
	sourceFlag   = "--driver"
	sourceEnv    = "WARDEN_DRIVER"
	sourceConfig = "config"
	sourceDflt   = "default"
)

// bundledDrivers are the drivers BuildWarden ships, in the order driver list
// prints them.
var bundledDrivers = []string{"container", "qemu", "vz", "hyperv", "firecracker"}

// wardenCommands are warden's commands, the names no driver may take (DRV-1).
// hyperv is also a driver: it is linked, and `warden hyperv service` stays
// warden's command in K1.
var wardenCommands = []string{"build", "shell", "doctor", "setup", "image", "clean",
	"inspect", "driver", "help", "completion", "hyperv"}

var driverNameRE = regexp.MustCompile(`^[a-z][a-z0-9]{0,31}$`)

// selection is the driver a command line selects, and where it came from:
// sourceFlag, sourceEnv, sourceConfig or sourceDflt.
type selection struct {
	value  string
	source string
	// file is the config file that set it, for sourceConfig.
	file string
}

// lookupEnv is what a lookup reads from the process: PATH, warden's own
// directory, whether this is Windows, and which names are linked.
type lookupEnv struct {
	path    string
	selfDir string
	self    string
	windows bool
	linked  func(name string) bool
}

// hostLookupEnv is the lookup environment of this process.
func hostLookupEnv() lookupEnv {
	self, _ := os.Executable()
	if r, err := filepath.EvalSymlinks(self); err == nil {
		self = r
	}
	return lookupEnv{path: os.Getenv("PATH"), selfDir: filepath.Dir(self), self: self,
		windows: isWindows, linked: func(n string) bool { return linkedDrivers[n] != nil }}
}

// driverCopy is a file of a driver's name the lookup met, and where.
type driverCopy struct {
	Path  string `json:"path"`
	Found string `json:"found"`
}

// skippedCopy is a file of a driver's name the lookup passed over because it
// cannot be run, with the reason (DRV-8).
type skippedCopy struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

// foundDriver is the outcome of a lookup: the file that runs for a name, or
// the linked driver, and every other copy.
type foundDriver struct {
	Name string
	// Path is the driver's absolute path; for a linked driver, warden's own.
	Path   string
	Found  string
	Linked bool
	// Source names the flag or variable behind a path.
	Source   string
	Shadowed []driverCopy
	Skipped  []skippedCopy
}

// isPathSelection reports whether a selection names a file (DRV-4).
func isPathSelection(v string, windows bool) bool {
	return strings.Contains(v, "/") || (windows && strings.Contains(v, `\`))
}

// driverFileName is the file name of the driver name.
func driverFileName(name string, windows bool) string {
	if windows {
		return driverPrefix + name + ".exe"
	}
	return driverPrefix + name
}

// checkDriverName refuses a name a driver cannot have (DRV-1).
func checkDriverName(name string) error {
	if !driverNameRE.MatchString(name) {
		return driver.Fixable("a driver's name is a lower-case letter followed by "+
			"up to 31 lower-case letters and digits; `warden driver list` shows "+
			"the drivers warden can find",
			"%q is not a driver name", name)
	}
	if name != "hyperv" && contains(wardenCommands, name) {
		return driver.Fixable("choose a driver by another name",
			"%q is a warden command, not a driver", name)
	}
	return nil
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// findDriver resolves a selection to the driver that runs for it (DRV-6).
func findDriver(sel selection, env lookupEnv) (*foundDriver, error) {
	if isPathSelection(sel.value, env.windows) {
		return findByPath(sel, env)
	}
	if err := checkDriverName(sel.value); err != nil {
		return nil, err
	}
	fd, err := findByName(sel.value, env)
	if err != nil {
		return nil, err
	}
	return fd, nil
}

// isWindows is whether warden runs on Windows, where a driver's file ends in
// .exe and a backslash separates a path.
const isWindows = runtime.GOOS == "windows"

// findByPath checks a file named by path (DRV-4, DRV-7): it must come from
// the flag or the variable, exist, be a regular file once links are followed,
// be executable, and have a driver's file name.
func findByPath(sel selection, env lookupEnv) (*foundDriver, error) {
	if sel.source == sourceConfig {
		return nil, driver.Fixable("name the driver by its name in the config file, "+
			"or give its file with --driver or WARDEN_DRIVER",
			"[runtime] driver in %s is %q: a config file names a driver, not a file",
			driver.EscapeOutside(sel.file), sel.value)
	}
	abs, err := filepath.Abs(sel.value)
	if err != nil {
		return nil, err
	}
	shown := driver.EscapeOutside(abs)
	base := filepath.Base(abs)
	name := strings.TrimPrefix(base, driverPrefix)
	if env.windows {
		name = strings.TrimSuffix(name, ".exe")
	}
	fix := fmt.Sprintf("name a driver's file, warden-driver-<name>%s, with %s",
		map[bool]string{true: ".exe"}[env.windows], sel.source)
	if !strings.HasPrefix(base, driverPrefix) || base != driverFileName(name, env.windows) ||
		checkDriverName(name) != nil {
		return nil, driver.Fixable(fix, "%s (%s) is not a driver's file: its name "+
			"is not warden-driver-<name>", shown, sel.source)
	}
	if reason := unrunnable(abs, env.windows); reason != "" {
		return nil, driver.Fixable(fix, "driver %s (%s) %s", shown, sel.source, reason)
	}
	fd := &foundDriver{Name: name, Path: abs, Found: foundPath, Source: sel.source}
	if byName, err := findByName(name, env); err == nil {
		fd.Shadowed = append(fd.Shadowed, driverCopy{Path: byName.Path,
			Found: byName.Found})
		fd.Shadowed = append(fd.Shadowed, byName.Shadowed...)
		fd.Skipped = byName.Skipped
	}
	return fd, nil
}

// unrunnable says why the file at path cannot be run as a driver, or "".
func unrunnable(path string, windows bool) string {
	info, err := os.Stat(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return "does not exist"
	case err != nil:
		return "cannot be examined: " + osReason(err)
	case info.IsDir():
		return "is a directory"
	case !info.Mode().IsRegular():
		return "is not a regular file"
	case !windows && info.Mode().Perm()&0o111 == 0:
		return "is not executable"
	}
	return ""
}

// candidates collects the files a lookup meets, compared by identity after
// links are resolved: two that are one file are one candidate (DRV-10).
type candidates struct {
	paths []string
	infos []os.FileInfo
}

func (c *candidates) add(path string) {
	info, err := os.Stat(path)
	if err != nil {
		return
	}
	for _, mi := range c.infos {
		if os.SameFile(mi, info) {
			return
		}
	}
	c.paths, c.infos = append(c.paths, path), append(c.infos, info)
}

// findByName looks a name up (DRV-6, DRV-8, DRV-9, DRV-10). When the name is
// not found, it returns the error with the lookup's skipped files.
func findByName(name string, env lookupEnv) (*foundDriver, error) {
	file := driverFileName(name, env.windows)
	fd := &foundDriver{Name: name}
	var met candidates
	if err := searchPATH(name, env, fd, &met); err != nil {
		return nil, err
	}
	var bundled os.FileInfo
	if env.selfDir != "" {
		cand := filepath.Join(env.selfDir, file)
		bundled, _ = os.Stat(cand)
		switch reason := unrunnable(cand, env.windows); reason {
		case "":
			met.add(cand)
		case "does not exist":
		default:
			fd.Skipped = appendSkipped(fd.Skipped, cand, reason)
		}
	}
	var copies []driverCopy
	for i, p := range met.paths {
		found := foundPATH
		if bundled != nil && os.SameFile(met.infos[i], bundled) {
			found = foundBeside
		}
		copies = append(copies, driverCopy{Path: p, Found: found})
	}
	if env.linked != nil && env.linked(name) {
		copies = append(copies, driverCopy{Path: env.self, Found: foundLinked})
	}
	if len(copies) == 0 {
		return fd, notFound(name, env, fd.Skipped)
	}
	fd.Path, fd.Found = copies[0].Path, copies[0].Found
	fd.Linked = fd.Found == foundLinked
	fd.Shadowed = copies[1:]
	return fd, nil
}

// searchPATH meets the name's candidates on PATH, in order, and records the
// files it passes over. A candidate through a relative entry, or on Unix an
// empty one, stops the lookup with an error (DRV-9); on Windows an empty
// entry is skipped.
func searchPATH(name string, env lookupEnv, fd *foundDriver, met *candidates) error {
	file := driverFileName(name, env.windows)
	for _, entry := range filepath.SplitList(env.path) {
		if entry == "" && env.windows {
			continue
		}
		cand := filepath.Join(entry, file)
		if entry == "" {
			cand = "." + string(filepath.Separator) + file
		}
		if reason := unrunnable(cand, env.windows); reason != "" {
			if reason != "does not exist" {
				fd.Skipped = appendSkipped(fd.Skipped, cand, reason)
			}
			continue
		}
		if entry != "" && filepath.IsAbs(entry) {
			met.add(cand)
			continue
		}
		shownEntry := entry
		if entry == "" {
			shownEntry = "an empty entry (the current directory)"
		}
		return driver.Fixable(fmt.Sprintf("make the PATH entry absolute, or name the "+
			"file: --driver %s", driver.EscapeOutside(cand)),
			"driver %q was found through the relative PATH entry %s, as %s, which "+
				"warden does not run", name, driver.EscapeOutside(shownEntry),
			driver.EscapeOutside(cand))
	}
	return nil
}

func appendSkipped(s []skippedCopy, path, reason string) []skippedCopy {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	for _, k := range s {
		if k.Path == abs {
			return s
		}
	}
	return append(s, skippedCopy{Path: abs, Reason: reason})
}

// notFound is the error for a name that is not found: it says where warden
// looked, and names each file it passed over (DRV-5, DRV-8).
func notFound(name string, env lookupEnv, skipped []skippedCopy) error {
	where := "PATH"
	if env.selfDir != "" {
		where += ", then " + driver.EscapeOutside(env.selfDir) +
			", where warden is installed"
	}
	msg := fmt.Sprintf("driver %q not found (searched %s)", name, where)
	for _, s := range skipped {
		msg += fmt.Sprintf("; passed over %s (%s)", driver.EscapeOutside(s.Path), s.Reason)
	}
	return driver.Fixable(fmt.Sprintf("put %s on PATH or beside warden, or name its "+
		"file: --driver /path/to/%s. `warden driver list` shows the drivers warden "+
		"can find", driverFileName(name, env.windows), driverFileName(name, env.windows)),
		"%s", msg)
}

// listedDriver is one entry of driver list.
type listedDriver struct {
	Name     string        `json:"name"`
	Path     string        `json:"path"`
	Found    string        `json:"found"`
	Source   string        `json:"source,omitempty"`
	Selected bool          `json:"selected"`
	Shadowed []driverCopy  `json:"shadowed"`
	Skipped  []skippedCopy `json:"skipped"`
	// err is why the name cannot be used, for the line under the table.
	err error
}

// listDrivers is driver list's entries: the bundled names, then every
// warden-driver-* on PATH or beside warden, by name. It reads directories
// only and runs no driver. sel, when set, is the selected driver, marked; a
// path selection is listed under its name with the file named. The error is
// why the selection cannot be used.
func listDrivers(env lookupEnv, sel *selection) ([]listedDriver, error) {
	names := append(append([]string{}, bundledDrivers...), foundNames(env)...)
	var selFound *foundDriver
	var selErr error
	selName := ""
	if sel != nil && isPathSelection(sel.value, env.windows) {
		if selFound, selErr = findByPath(*sel, env); selFound != nil {
			selName = selFound.Name
		}
	} else if sel != nil {
		selName = sel.value
	}
	var out []listedDriver
	listed := map[string]bool{}
	for _, n := range names {
		fd, err := selFound, error(nil)
		if selFound == nil || n != selFound.Name {
			fd, err = findByName(n, env)
		}
		unlisted := !contains(bundledDrivers, n) && (fd == nil || len(fd.Skipped) == 0)
		if err != nil && unlisted {
			continue
		}
		listed[n] = true
		out = append(out, newListed(n, fd, err, n == selName && selErr == nil))
	}
	if selFound != nil && !listed[selFound.Name] {
		out = append(out, newListed(selFound.Name, selFound, nil, true))
	}
	return out, selErr
}

// newListed is the entry of name, which the lookup found as fd, or not, with
// err.
func newListed(name string, fd *foundDriver, err error, selected bool) listedDriver {
	ld := listedDriver{Name: name, Selected: selected, Shadowed: []driverCopy{},
		Skipped: []skippedCopy{}, err: err}
	if fd != nil {
		ld.Skipped = append(ld.Skipped, fd.Skipped...)
	}
	if err == nil {
		ld.Path, ld.Found, ld.Source = fd.Path, fd.Found, fd.Source
		ld.Shadowed = append(ld.Shadowed, fd.Shadowed...)
	}
	return ld
}

// foundNames are the names of the warden-driver-* files on PATH's absolute
// entries and beside warden that are not bundled, sorted.
func foundNames(env lookupEnv) []string {
	seen := map[string]bool{}
	for _, n := range bundledDrivers {
		seen[n] = true
	}
	var names []string
	dirs := filepath.SplitList(env.path)
	if env.selfDir != "" {
		dirs = append(dirs, env.selfDir)
	}
	for _, dir := range dirs {
		if dir == "" || !filepath.IsAbs(dir) {
			continue
		}
		entries, _ := os.ReadDir(dir)
		for _, e := range entries {
			n, ok := strings.CutPrefix(e.Name(), driverPrefix)
			if env.windows {
				n, ok = strings.CutSuffix(n, ".exe")
				ok = ok && strings.HasPrefix(e.Name(), driverPrefix)
			}
			if !ok || seen[n] || checkDriverName(n) != nil {
				continue
			}
			seen[n] = true
			names = append(names, n)
		}
	}
	sort.Strings(names)
	return names
}
