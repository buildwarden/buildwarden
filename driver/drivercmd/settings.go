package drivercmd

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/netip"
	"os"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/BurntSushi/toml"

	"github.com/buildwarden/buildwarden/driver"
)

// Type is the type of a setting's value, which fixes how the kit reads and
// checks it and the TOML type describe reports for its key.
type Type int

// The types of a setting. A Bool with a flag is a switch: --name, on when
// present. Every other type's flag takes a value.
const (
	// String is a TOML string, or the flag's value as typed.
	String Type = iota + 1
	// Int is a TOML integer, or a flag value that parses as one.
	Int
	// Bool is a TOML boolean, or a switch.
	Bool
	// Age is a string holding an age, "7d" or "36h" (driver.ParseAge).
	Age
	// CIDR is a string holding a prefix, "10.9.0.0/30".
	CIDR
	// IPList is a TOML array of strings, or a flag value of addresses
	// separated by commas, each an IP address.
	IPList
	// Path is a string holding a path.
	Path
	// Raw is a key of any shape, read with Settings.Decode.
	Raw
)

// tomlType is the type describe reports for a key of this Type.
func (t Type) tomlType() string {
	switch t {
	case Int:
		return "integer"
	case Bool:
		return "boolean"
	case IPList:
		return "array"
	case Raw:
		return "any"
	default:
		return "string"
	}
}

func (t Type) valid() bool { return t >= String && t <= Raw }

// Setting is one value a driver takes: a flag, a config key, or both. A
// driver declares its settings as a literal with constant strings; the kit
// builds describe's flags and keys from it, and tier 1's surface check reads
// the literal from source (UX-01, UX-03, ORCH-03).
type Setting struct {
	// Name is what the driver reads it by: cfg.String("runtime").
	Name string
	// Type is how the value is read and checked.
	Type Type
	// Flag is "runtime" for --runtime; "" for no flag.
	Flag string
	// Key is a key of the driver's own, under its table:
	// "driver.nomad.region"; "" for none.
	Key string
	// Common is a common key the driver honours, by the name the contract
	// gives it: "image_refresh_after". It is read in [driver.<name>], then
	// in [driver]. Not with Key.
	Common string
	// Aliases are the key's deprecated old names, each one the contract
	// lists for this driver: "runtime.cli".
	Aliases []string
	// Env is WARDEN_CTR_CLI or WARDEN_IMAGE, the container driver's two
	// variables, and "" for every other setting (DRV-C9, UX-07).
	Env string
	// Commands are the commands that take it; EveryCommand is every command
	// the driver lists.
	Commands []string
	// Usage is the flag's help line, and the key's.
	Usage string
}

// EveryCommand is a Setting's Commands for a setting every command the driver
// lists takes. For a flag, that is each of those commands that takes flags.
var EveryCommand = []string{everyCommand}

const everyCommand = "*"

// key is the setting's place in the file as a dotted path: its own Key, or
// driver.<Common>; "" when it has neither.
func (s *Setting) key() string {
	if s.Common != "" {
		return "driver." + s.Common
	}
	return s.Key
}

// places are where the file may write s for the driver called name: its key,
// the override driver.<name>.<key> of a common key, and its old names. None
// for a setting with no key.
func (s *Setting) places(name string) []string {
	if s.key() == "" {
		return nil
	}
	p := []string{s.key()}
	if s.Common != "" {
		p = append(p, "driver."+name+"."+s.Common)
	}
	return append(p, s.Aliases...)
}

// appliesTo reports whether command takes s.
func (s *Setting) appliesTo(command string) bool {
	return slices.Contains(s.Commands, everyCommand) || slices.Contains(s.Commands, command)
}

// expand is s's commands, with EveryCommand replaced by listed, keeping only
// the commands keep allows.
func (s *Setting) expand(listed []string, keep func(string) bool) []string {
	var out []string
	add := func(c string) {
		if keep(c) && !slices.Contains(out, c) {
			out = append(out, c)
		}
	}
	for _, c := range s.Commands {
		if c == everyCommand {
			for _, l := range listed {
				add(l)
			}
			continue
		}
		add(c)
	}
	return out
}

// The common keys of contract version 1 (v1.md, The common keys of version
// 1). Both hold an age.
var commonKeys = []string{"image_refresh_after", "image_min_grace"}

// oldNames are the deprecated old names of contract version 1 by driver
// name, each mapped to its key (v1.md, Old names). A driver of no other name
// has any.
var oldNames = map[string]map[string]string{
	"container": {
		"runtime.cli":         "driver.container.cli",
		"runtime.relay_image": "driver.container.relay_image",
	},
	"qemu": {
		"vm.image_refresh_after": "driver.image_refresh_after",
		"vm.image_min_grace":     "driver.image_min_grace",
	},
	"hyperv": {
		"vm.image_refresh_after":  "driver.image_refresh_after",
		"vm.image_min_grace":      "driver.image_min_grace",
		"vm.network":              "driver.hyperv.network",
		"vm.windows_image":        "driver.hyperv.windows_image",
		"vm.windows_image_update": "driver.hyperv.windows_image_update",
	},
}

// envKeys are the two environment variables of version 1 and the key each
// sets (v1.md, Old names; DRV-C9).
var envKeys = map[string]string{
	"WARDEN_CTR_CLI": "driver.container.cli",
	"WARDEN_IMAGE":   "driver.container.relay_image",
}

var (
	driverNameRE = regexp.MustCompile(`^[a-z][a-z0-9]{0,31}$`)
	flagNameRE   = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
	keyPartRE    = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)
)

// checkSettings refuses a declaration that breaks the contract's placement
// rules (DRV-C1, DRV-C4, DRV-C9, DRV-C11). Its errors are the driver's
// defects, not the user's, so none is fixable.
func checkSettings(name string, settings []Setting) error {
	if !driverNameRE.MatchString(name) {
		return fmt.Errorf("driver name %q is not of the form [a-z][a-z0-9]{0,31}", name)
	}
	names, flags, keys := map[string]bool{}, map[string]bool{}, map[string]string{}
	for i := range settings {
		s := &settings[i]
		if err := checkSetting(name, s); err != nil {
			return fmt.Errorf("setting %q: %w", s.Name, err)
		}
		if names[s.Name] {
			return fmt.Errorf("setting %q is declared twice", s.Name)
		}
		names[s.Name] = true
		if s.Flag != "" {
			if flags[s.Flag] {
				return fmt.Errorf("flag --%s is declared twice", s.Flag)
			}
			flags[s.Flag] = true
		}
		// No two places are equal with ASCII letters folded, so a key
		// spelt as declared is never another key in another case (DRV-C4).
		for _, k := range s.places(name) {
			if prev, ok := keys[foldASCII(k)]; ok {
				if prev == k {
					return fmt.Errorf("key %s is declared twice", k)
				}
				return fmt.Errorf("keys %s and %s are one key in another "+
					"letter case", prev, k)
			}
			keys[foldASCII(k)] = k
		}
	}
	return nil
}

// checkSetting refuses a setting of the driver called name that is not
// whole, or not placed where the contract puts it.
func checkSetting(name string, s *Setting) error {
	if err := checkWhole(s); err != nil {
		return err
	}
	if err := checkPlace(name, s); err != nil {
		return err
	}
	for _, a := range s.Aliases {
		if oldNames[name][a] != s.key() || s.key() == "" {
			return fmt.Errorf("the contract lists no old name %s for %q of driver %q",
				a, s.key(), name)
		}
	}
	if s.Env != "" && (envKeys[s.Env] == "" || envKeys[s.Env] != s.Key) {
		return fmt.Errorf("variable %s is not the contract's for %s", s.Env, s.Key)
	}
	return nil
}

// checkWhole refuses a setting with a field missing or malformed.
func checkWhole(s *Setting) error {
	switch {
	case s.Name == "":
		return errors.New("no name")
	case !s.Type.valid():
		return errors.New("no type")
	case len(s.Commands) == 0:
		return errors.New("no commands")
	case s.Usage == "":
		return errors.New("no usage")
	case s.Flag == "" && s.Key == "" && s.Common == "":
		return errors.New("neither a flag nor a key")
	case s.Flag != "" && !flagNameRE.MatchString(s.Flag):
		return fmt.Errorf("flag %q is not of the form [a-z][a-z0-9-]*", s.Flag)
	case s.Key != "" && s.Common != "":
		return errors.New("both Key and Common")
	}
	for _, c := range s.Commands {
		if c != everyCommand && !slices.Contains(Commands, c) {
			return fmt.Errorf("unknown command %q", c)
		}
	}
	return nil
}

// checkPlace refuses a common key the contract does not name, and a key of
// the driver's own outside its table or with a common key's name (DRV-C4).
func checkPlace(name string, s *Setting) error {
	if s.Common != "" {
		if !slices.Contains(commonKeys, s.Common) {
			return fmt.Errorf("%q is not a common key of contract version 1", s.Common)
		}
		if s.Type != Age {
			return fmt.Errorf("common key %s holds an age", s.Common)
		}
	}
	if s.Key == "" {
		return nil
	}
	own, ok := strings.CutPrefix(s.Key, "driver."+name+".")
	if !ok || own == "" {
		return fmt.Errorf("key %s is not under driver.%s", s.Key, name)
	}
	parts := strings.Split(own, ".")
	for _, p := range parts {
		if !keyPartRE.MatchString(p) {
			return fmt.Errorf("key %s has an element that is not [A-Za-z0-9_-]+",
				s.Key)
		}
	}
	if slices.Contains(commonKeys, parts[0]) {
		return fmt.Errorf("key %s has a common key's name, which in "+
			"[driver.%s] is that key's override", s.Key, name)
	}
	return nil
}

// Resolve reads the settings of the driver called name that apply to
// command: the flag in args, else the setting's environment variable, else
// its key in configFile, the request's config_file (ORCH-03); configFile is
// empty when the request names none. A common key is read in
// [driver.<name>] and then in [driver], and an old name counts as the key
// itself. environ is the environment, as os.Environ gives it.
//
// It refuses an element of args that settings does not declare, a value of
// the wrong type and a file that sets a key under two names, with a
// FixableError that names where the value came from, and, before it reads
// any value from the file, a key the file spells in another ASCII letter
// case than one of the settings' places (DRV-C12). When a setting of
// command has a key, it refuses a configFile that does not exist, cannot be
// read or is not TOML, naming the file (DRV-C6). Only for reap and attach is
// that different: it reads nothing until a setting is asked for, and reads
// a file that no longer exists as if none had been named (DRV-C10); an error
// then is the Settings' Err. A declaration that breaks the contract's rules
// is refused with an error that is not fixable: it is the driver's defect.
func Resolve(name string, settings []Setting, command string, args []string,
	configFile string, environ []string) (driver.Settings, error) {
	r := &resolved{driverName: name, file: configFile, values: map[string]*value{},
		notices: os.Stderr}
	if err := checkSettings(name, settings); err != nil {
		return nil, fmt.Errorf("driver %q declares its settings wrongly: %w", name, err)
	}
	for _, s := range settings {
		if s.appliesTo(command) {
			r.settings = append(r.settings, s)
		}
	}
	if command == CmdReap || command == CmdAttach {
		r.lazy = func() error { return r.resolve(command, args, environ, true) }
		return r, nil
	}
	if err := r.resolve(command, args, environ, false); err != nil {
		return nil, err
	}
	return r, nil
}

// resolved is the kit's driver.Settings.
type resolved struct {
	driverName string
	file       string
	settings   []Setting
	values     map[string]*value
	warnings   []string
	// lazy, when set, resolves on the first read (reap and attach); err is
	// its outcome.
	lazy    func() error
	once    sync.Once
	err     error
	notices io.Writer
}

// value is one setting's value and where it came from.
type value struct {
	setting Setting
	v       any // a string from a flag or variable, a bool switch, or a TOML value
	source  string
}

func (r *resolved) resolve(command string, args, environ []string, lazy bool) error {
	if err := r.fromArgs(command, args); err != nil {
		return err
	}
	for _, s := range r.settings {
		if _, ok := r.values[s.Name]; ok || s.Env == "" {
			continue
		}
		if v := lookupEnv(environ, s.Env); v != "" {
			if err := r.set(s, v, s.Env); err != nil {
				return err
			}
		}
	}
	tree, err := r.load(lazy)
	if err != nil || tree == nil {
		return err
	}
	for _, s := range r.settings {
		for _, place := range s.places(r.driverName) {
			if err := r.checkCase(tree, s, place); err != nil {
				return err
			}
		}
	}
	return r.fromFile(tree, lazy)
}

// fromArgs takes each element of args as a declared flag of command
// (DRV-C2). For a flag that holds one value the last occurrence wins; an
// empty value is not set (DRV-C8).
func (r *resolved) fromArgs(command string, args []string) error {
	for _, a := range args {
		flag, val, hasVal := strings.Cut(strings.TrimPrefix(a, "--"), "=")
		var s *Setting
		for i := range r.settings {
			if strings.HasPrefix(a, "--") && r.settings[i].Flag == flag && flag != "" {
				s = &r.settings[i]
			}
		}
		switch {
		case s == nil:
			return &driver.FixableError{
				Msg: fmt.Sprintf("%s is not a flag of driver %q for %s",
					driver.EscapeOutside(a), r.driverName, command),
				Fix: "remove it; the flags this driver takes are in its describe"}
		case s.Type == Bool && hasVal:
			return driver.Fixable("write --"+flag+" to turn it on, and leave "+
				"it out to turn it off",
				"--%s is a switch and takes no value, not %q", flag, val)
		case s.Type != Bool && !hasVal:
			return driver.Fixable("write --"+flag+"=<value>", "--%s needs a value",
				flag)
		case s.Type == Bool:
			r.values[s.Name] = &value{setting: *s, v: true, source: "--" + flag}
		case val == "":
			delete(r.values, s.Name)
		default:
			if err := r.set(*s, val, "--"+flag); err != nil {
				return err
			}
		}
	}
	return nil
}

// set records v for s from source, once it converts.
func (r *resolved) set(s Setting, v any, source string) error {
	if _, err := convert(s.Type, v); err != nil {
		return &driver.FixableError{Msg: fmt.Sprintf("%s: %v", source, err),
			Fix: fixFor(s.Type)}
	}
	r.values[s.Name] = &value{setting: s, v: v, source: source}
	return nil
}

// load reads and parses the config file when a setting of the command has a
// key (DRV-C6). In reap and attach (lazy), a file that no longer exists is
// read as if none had been named, said on stderr (DRV-C10).
func (r *resolved) load(lazy bool) (map[string]any, error) {
	if r.file == "" || !slices.ContainsFunc(r.settings, func(s Setting) bool {
		return s.key() != ""
	}) {
		return nil, nil
	}
	path := driver.EscapeOutside(r.file)
	data, err := os.ReadFile(r.file)
	switch {
	case lazy && errors.Is(err, fs.ErrNotExist):
		fmt.Fprintf(r.notices, "[warden] config file %s no longer exists; "+
			"reading no config file\n", path)
		return nil, nil
	case errors.Is(err, fs.ErrNotExist):
		return nil, driver.Fixable("create it, or name a config file that exists",
			"config file %s does not exist", path)
	case err != nil:
		return nil, driver.Fixable("make it readable, or name another config file",
			"reading config file %s: %v", path, err)
	}
	var tree map[string]any
	if _, err := toml.Decode(string(data), &tree); err != nil {
		return nil, driver.Fixable("fix the TOML syntax in "+path, "parsing %s: %v",
			path, err)
	}
	return tree, nil
}

// fromFile reads each setting not already set from the file, by the
// contract's precedence: for a common key [driver.<name>], then [driver]
// (DRV-C8); an old name stands where its key stands (DRV-C11).
func (r *resolved) fromFile(tree map[string]any, lazy bool) error {
	path := driver.EscapeOutside(r.file)
	for _, s := range r.settings {
		key := s.key()
		if key == "" {
			continue
		}
		// The names the file may give this key at its own step, the new
		// name first, and which of them it sets.
		var set []string
		for _, n := range append([]string{key}, s.Aliases...) {
			if _, ok := lookup(tree, n); ok {
				set = append(set, n)
			}
		}
		if len(set) > 1 {
			return driver.Fixable("remove "+label(set[1])+" from "+path,
				"%s sets %s twice: as %s and as its old name %s", path, label(key),
				label(set[0]), label(set[1]))
		}
		for _, n := range set {
			if n != key && !lazy {
				r.warnings = append(r.warnings, fmt.Sprintf(
					"[warden] warning: %s: %s is deprecated; use %s",
					path, label(n), label(key)))
			}
		}
		if _, done := r.values[s.Name]; done {
			continue
		}
		places := set
		if s.Common != "" {
			override := "driver." + r.driverName + "." + s.Common
			places = append([]string{override}, set...)
		}
		for _, n := range places {
			v, ok := lookup(tree, n)
			if !ok {
				continue
			}
			if err := r.set(s, v, path+" "+label(n)); err != nil {
				return err
			}
			break
		}
	}
	return nil
}

// checkCase refuses a file that writes place, a place of s, in another ASCII
// letter case, at the same path as the place or beside it in one table: the
// key is read as spelt and only so, and a value the user wrote must not be
// dropped without a word (DRV-C12). It runs before any value is read and
// before the refusal of both names (DRV-C11).
func (r *resolved) checkCase(tree map[string]any, s Setting, place string) error {
	var found []string
	walkFolded(tree, strings.Split(place, "."), nil, &found)
	found = slices.DeleteFunc(found, func(f string) bool { return f == place })
	if len(found) == 0 {
		return nil
	}
	slices.Sort(found)
	path := driver.EscapeOutside(r.file)
	// For an old name the key to write is the new one, so following the fix
	// gives no deprecation warning.
	write, reads := place, label(place)
	if slices.Contains(s.Aliases, place) {
		write = s.key()
		reads = label(write) + " (old name " + label(place) + ")"
	}
	labels := make([]string, len(found))
	for i, f := range found {
		labels[i] = label(f)
	}
	what := joinAnd(labels)
	table := place[:strings.LastIndex(place, ".")]
	_, exact := lookup(tree, place)
	_, written := lookup(tree, write)
	fix := "write it as " + label(write)
	switch {
	case exact && allIn(found, table):
		names := []string{place[len(table)+1:]}
		for _, f := range found {
			names = append(names, f[len(table)+1:])
		}
		what = "[" + table + "] holds " + joinAnd(names)
		fix = "keep one, written as " + label(write)
	case written:
		fix = "remove " + what + "; " + label(write) + " is already set"
	case exact:
		fix = "remove " + what + "; " + label(place) + " is already set"
	case len(found) > 1:
		fix = "keep one, written as " + label(write)
	}
	return driver.Fixable(fix, "%s: %s: keys are case-sensitive, and the %s driver "+
		"reads %s", path, what, r.driverName, reads)
}

// walkFolded adds to found the dotted path of every value in tree at a path
// equal to parts with ASCII letters folded; at is the path so far.
func walkFolded(tree map[string]any, parts, at []string, found *[]string) {
	keys := make([]string, 0, len(tree))
	for k := range tree {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	for _, k := range keys {
		if foldASCII(k) != foldASCII(parts[0]) {
			continue
		}
		here := append(slices.Clip(at), k)
		if len(parts) == 1 {
			*found = append(*found, strings.Join(here, "."))
			continue
		}
		if sub, ok := tree[k].(map[string]any); ok {
			walkFolded(sub, parts[1:], here, found)
		}
	}
}

// foldASCII is s with its ASCII upper-case letters in lower case, and every
// other byte as it is: the contract folds ASCII letters only (DRV-C12).
func foldASCII(s string) string {
	return strings.Map(func(r rune) rune {
		if r >= 'A' && r <= 'Z' {
			return r + 'a' - 'A'
		}
		return r
	}, s)
}

// allIn reports whether every dotted path in keys is a key of table.
func allIn(keys []string, table string) bool {
	for _, k := range keys {
		if k[:strings.LastIndex(k, ".")] != table {
			return false
		}
	}
	return true
}

// joinAnd is "a", "a and b", or "a, b and c".
func joinAnd(items []string) string {
	if len(items) == 1 {
		return items[0]
	}
	return strings.Join(items[:len(items)-1], ", ") + " and " + items[len(items)-1]
}

// lookup finds the dotted path key in tree; an element that is not a table
// holds nothing beneath it.
func lookup(tree map[string]any, key string) (any, bool) {
	parts := strings.Split(key, ".")
	var cur any = tree
	for _, p := range parts {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		if cur, ok = m[p]; !ok {
			return nil, false
		}
	}
	return cur, true
}

// label is a dotted key as the file writes it: "[driver.hyperv] network".
func label(key string) string {
	i := strings.LastIndex(key, ".")
	return "[" + key[:i] + "] " + key[i+1:]
}

func lookupEnv(environ []string, name string) string {
	for i := len(environ) - 1; i >= 0; i-- {
		if v, ok := strings.CutPrefix(environ[i], name+"="); ok {
			return v
		}
	}
	return ""
}

// convert reads v, a flag's or variable's string or a TOML value, as t.
func convert(t Type, v any) (any, error) {
	switch t {
	case Int:
		switch x := v.(type) {
		case int64:
			return int(x), nil
		case string:
			n, err := strconv.Atoi(x)
			if err != nil {
				return nil, fmt.Errorf("%q is not an integer", x)
			}
			return n, nil
		}
	case Bool:
		if b, ok := v.(bool); ok {
			return b, nil
		}
	case IPList:
		return convertIPList(v)
	case Raw:
		return v, nil
	default:
		if s, ok := v.(string); ok {
			return convertString(t, s)
		}
	}
	return nil, fmt.Errorf("want %s, not %s", typeName(t), valueType(v))
}

// convertString reads s as t, one of the types a string holds; an empty
// string is the zero value.
func convertString(t Type, s string) (any, error) {
	switch {
	case t == Age && s == "":
		return time.Duration(0), nil
	case t == Age:
		return driver.ParseAge(s)
	case t == CIDR && s == "":
		return netip.Prefix{}, nil
	case t == CIDR:
		p, err := netip.ParsePrefix(s)
		if err != nil {
			return nil, fmt.Errorf("%q is not a CIDR prefix", s)
		}
		return p, nil
	}
	return s, nil
}

func convertIPList(v any) (any, error) {
	var items []any
	switch x := v.(type) {
	case string:
		for _, s := range strings.Split(x, ",") {
			items = append(items, strings.TrimSpace(s))
		}
	case []any:
		items = x
	case []map[string]any:
		return nil, errors.New("want an array of IP addresses, not an array of tables")
	default:
		return nil, fmt.Errorf("want an array of IP addresses, not %s", valueType(v))
	}
	out := []netip.Addr{}
	for _, it := range items {
		s, ok := it.(string)
		if !ok {
			return nil, fmt.Errorf("want an array of IP addresses, not one holding %s",
				valueType(it))
		}
		a, err := netip.ParseAddr(s)
		if err != nil {
			return nil, fmt.Errorf("%q is not an IP address", s)
		}
		out = append(out, a)
	}
	return out, nil
}

func typeName(t Type) string {
	switch t {
	case Int:
		return "an integer"
	case Bool:
		return "a boolean"
	case Age:
		return `a string holding an age, such as "7d" or "36h"`
	case CIDR:
		return "a string holding a CIDR prefix"
	case IPList:
		return "an array of IP addresses"
	default:
		return "a string"
	}
}

func valueType(v any) string {
	switch v.(type) {
	case string:
		return "a string"
	case int64:
		return "an integer"
	case float64:
		return "a float"
	case bool:
		return "a boolean"
	case []any, []map[string]any:
		return "an array"
	case map[string]any:
		return "a table"
	default:
		return "a date or time"
	}
}

func fixFor(t Type) string {
	if t == Age {
		return `set it to an age such as "7d" or "36h"`
	}
	return "set it to " + typeName(t)
}

// get is the value of the setting called name, resolving first when that
// was put off.
func (r *resolved) get(name string) (any, bool) {
	if r.lazy != nil {
		r.once.Do(func() { r.err = r.lazy() })
		if r.err != nil {
			return nil, false
		}
	}
	v, ok := r.values[name]
	if !ok {
		return nil, false
	}
	c, err := convert(v.setting.Type, v.v)
	if err != nil {
		return nil, false
	}
	return c, true
}

// String implements driver.Settings.
func (r *resolved) String(name string) (string, bool) {
	v, ok := r.get(name)
	s, _ := v.(string)
	return s, ok
}

// Int implements driver.Settings.
func (r *resolved) Int(name string) (int, bool) {
	v, ok := r.get(name)
	n, _ := v.(int)
	return n, ok
}

// Bool implements driver.Settings.
func (r *resolved) Bool(name string) (value, ok bool) {
	v, ok := r.get(name)
	b, _ := v.(bool)
	return b, ok
}

// Age implements driver.Settings.
func (r *resolved) Age(name string) (time.Duration, bool) {
	v, ok := r.get(name)
	d, _ := v.(time.Duration)
	return d, ok
}

// CIDR implements driver.Settings.
func (r *resolved) CIDR(name string) (netip.Prefix, bool) {
	v, ok := r.get(name)
	p, _ := v.(netip.Prefix)
	return p, ok
}

// IPList implements driver.Settings.
func (r *resolved) IPList(name string) ([]netip.Addr, bool) {
	v, ok := r.get(name)
	l, _ := v.([]netip.Addr)
	return l, ok
}

// Path implements driver.Settings.
func (r *resolved) Path(name string) (string, bool) { return r.String(name) }

// Decode implements driver.Settings: the value, of any shape, is decoded
// into v, a pointer, as the TOML package decodes a key of that Go type.
func (r *resolved) Decode(name string, v any) (bool, error) {
	val, ok := r.get(name)
	if !ok {
		return false, nil
	}
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Pointer || rv.IsNil() {
		return true, fmt.Errorf("decode %s: want a non-nil pointer, not %T", name, v)
	}
	// TOML decodes tables only, so the value is wrapped in one and decoded
	// into a struct with a single field of v's type.
	var buf strings.Builder
	if err := toml.NewEncoder(&buf).Encode(map[string]any{"v": val}); err != nil {
		return true, fmt.Errorf("decode %s: %w", name, err)
	}
	wrap := reflect.New(reflect.StructOf([]reflect.StructField{{
		Name: "V", Type: rv.Elem().Type(), Tag: `toml:"v"`}}))
	if _, err := toml.Decode(buf.String(), wrap.Interface()); err != nil {
		return true, &driver.FixableError{
			Msg: fmt.Sprintf("%s: %v", r.values[name].source, err),
			Fix: "see the driver's documentation for the shape of this key"}
	}
	rv.Elem().Set(wrap.Elem().Field(0))
	return true, nil
}

// Source implements driver.Settings; it is "" for a setting nobody set.
func (r *resolved) Source(name string) string {
	if _, ok := r.get(name); !ok {
		return ""
	}
	return r.values[name].source
}

// Warnings implements driver.Settings.
func (r *resolved) Warnings() []string { return r.warnings }

// Err implements driver.Settings.
func (r *resolved) Err() error { return r.err }
