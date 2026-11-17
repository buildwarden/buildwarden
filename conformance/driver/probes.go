package driverconformance

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// probed are the commands the suite probes, in order: every command but
// attach and ext, which inherit the terminal.
var probed = []string{"run", "reap", "doctor", "setup", "image-prepare", "image-list",
	"image-verify", "clean"}

// unknownID is a build ID no build of the suite has.
const unknownID = "nonexist"

// request is the JSON of a probe of command, with fields set over the
// command's required ones; a field set to nil is left out. For run it is
// the start message, on one line.
func (s *Suite) request(command string, fields map[string]any) []byte {
	req := map[string]any{"contract": 1, "probe": true}
	state := filepath.Join(s.scratch, "locks", "build-"+s.name+"-"+unknownID+".state")
	switch command {
	case "run":
		req["type"] = "start"
		req["mode"] = "build"
		req["guest_os"] = s.desc.Capabilities.GuestOS[0]
		req["context_dir"] = s.scratch
		req["tmpdir"] = s.tmp
		req["image"], req["base_image"] = "", ""
		req["keep"], req["verbose"] = false, false
		req["args"] = []string{}
		if s.desc.Capabilities.VM {
			req["vm"] = map[string]any{}
		}
	case "reap":
		req["build_id"], req["state_file"], req["tmpdir"] = unknownID, state, s.tmp
		req["args"] = []string{}
	case "attach":
		req["build_id"], req["state_file"], req["args"] = unknownID, state, []string{}
	case "doctor", "setup":
		req["args"] = []string{}
		if s.desc.Capabilities.VM {
			req["vm"] = map[string]any{}
		}
	case "image-prepare":
		req["os"], req["source"], req["args"] = s.desc.Capabilities.DefaultImageOS, "",
			[]string{}
	case "clean":
		req["keep_ids"], req["cache"], req["tmpdir"], req["args"] = []string{}, false,
			s.tmp, []string{}
	}
	for k, v := range fields {
		if v == nil {
			delete(req, k)
			continue
		}
		req[k] = v
	}
	data, _ := json.Marshal(req)
	if command == "run" {
		data = append(data, '\n')
	}
	return data
}

// probe runs a probe of command with fields over its request.
func (s *Suite) probe(command string, fields map[string]any, env ...string) *outcome {
	argv := []string{}
	if command == "reap" || command == "attach" {
		argv = []string{unknownID}
	}
	return s.run(call{command: command, argv: argv, request: s.request(command, fields),
		env: env})
}

// created reports what is under the driver's temporary directory, and
// empties it for the next call.
func (s *Suite) created() string {
	entries, _ := os.ReadDir(s.tmp)
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
		_ = os.RemoveAll(filepath.Join(s.tmp, e.Name()))
	}
	return strings.Join(names, ", ")
}

// flagged are the listed commands that take flags.
func (s *Suite) flagged() []string {
	var out []string
	for _, c := range flagCommands {
		if s.lists(c) {
			out = append(out, c)
		}
	}
	return out
}

// configured are the listed commands the suite probes with a config file.
func (s *Suite) configured() []string {
	var out []string
	for _, c := range probed {
		if s.lists(c) && c != "reap" {
			out = append(out, c)
		}
	}
	return out
}

// checkUndeclaredArg: a probe of each listed command that takes flags, with
// an element of args the driver did not declare, exits 1 with a fixable
// error naming it, and creates nothing.
func (s *Suite) checkUndeclaredArg() (string, string) {
	const bogus = "--conformance-undeclared=1"
	for _, c := range s.flagged() {
		o := s.probe(c, map[string]any{"args": []string{bogus}})
		named := strings.Contains(o.errText(), bogus)
		if o.code != 1 || o.errCode() != "fixable" || !named {
			return "", fmt.Sprintf("a probe of %s with %s: exit %d, error %v; want "+
				"exit 1 and a fixable error naming it", c, bogus, o.code, o.result)
		}
		if made := s.created(); made != "" {
			return "", fmt.Sprintf("a probe of %s created %s", c, made)
		}
	}
	return "", ""
}

// checkOnlyNamedFile: an invalid warden.toml in the working directory and
// WARDEN_CONFIG naming another make no difference to a probe that names no
// config file.
func (s *Suite) checkOnlyNamedFile() (string, string) {
	dir, err := os.MkdirTemp(s.scratch, "cwd-")
	if err != nil {
		return "", err.Error()
	}
	other := filepath.Join(s.scratch, "other.toml")
	for _, f := range []string{filepath.Join(dir, "warden.toml"), other} {
		if err := os.WriteFile(f, []byte("[driver.\nthis is = not toml"),
			0o600); err != nil {
			return "", err.Error()
		}
	}
	for _, c := range s.configured() {
		plain := s.probe(c, nil)
		o := s.run(call{command: c, request: s.request(c, nil), dir: dir,
			env: []string{"WARDEN_CONFIG=" + other}})
		if !o.same(plain) {
			return "", fmt.Sprintf("a probe of %s beside an invalid warden.toml "+
				"and with WARDEN_CONFIG set ended with exit %d, error %v; with "+
				"neither, exit %d", c, o.code, o.result, plain.code)
		}
		s.created()
	}
	return "", ""
}

// unownedFile is a config file holding warden's tables, another driver's,
// an unknown one written with syntax only TOML 1.1 has, and every common key
// the driver does not list, in [driver] and in its own table, each of a
// type the specification does not give it.
func (s *Suite) unownedFile() string {
	var b strings.Builder
	b.WriteString("[runtime]\ndriver = \"" + s.desc.Name + "\"\n" +
		"[build]\noutput_dir = \"out\"\n[vm]\ncpus = 2\n" +
		"[relay]\nsystem_ca_bundle = true\n" +
		"[unknownconformance]\nx = { a = 1,\n  b = [2, 3],\n}\n")
	other := "conformanceother"
	if s.desc.Name == other {
		other += "x"
	}
	b.WriteString("[driver." + other + "]\nimage_refresh_after = 5\nanything = [1]\n")
	var unlisted []string
	for _, k := range commonKeys {
		if !slices.ContainsFunc(s.desc.Keys, func(dk describeKey) bool {
			return dk.Key == "driver."+k
		}) {
			unlisted = append(unlisted, k)
		}
	}
	b.WriteString("[driver]\n")
	for _, k := range unlisted {
		b.WriteString(k + " = 5\n")
	}
	b.WriteString("[driver." + s.desc.Name + "]\n")
	for _, k := range unlisted {
		b.WriteString(k + " = [true]\n")
	}
	return b.String()
}

// checkUnowned: with a file that holds only what the driver does not own, a
// probe ends as it does with no file.
func (s *Suite) checkUnowned() (string, string) {
	file := filepath.Join(s.scratch, "unowned.toml")
	if err := os.WriteFile(file, []byte(s.unownedFile()), 0o600); err != nil {
		return "", err.Error()
	}
	for _, c := range s.configured() {
		plain := s.probe(c, nil)
		o := s.probe(c, map[string]any{"config_file": file})
		if !o.same(plain) {
			return "", fmt.Sprintf("a probe of %s with a file of tables and keys "+
				"it does not own ended with exit %d, error %v; with no file, "+
				"exit %d",
				c, o.code, o.result, plain.code)
		}
		s.created()
	}
	return "", ""
}

// probeable is the first command of commands the suite probes with a
// config file, or "".
func (s *Suite) probeable(commands []string) string {
	for _, c := range s.configured() {
		if slices.Contains(commands, c) {
			return c
		}
	}
	return ""
}

// wrongValue is a TOML value of another type than typ.
func wrongValue(typ string) string {
	if typ == "integer" {
		return `"five"`
	}
	return "5"
}

// checkKeysRefused: for each listed key, a missing and a non-TOML config
// file, and the key given a value of the wrong type at each place it may be
// written, each make a probe exit 1 with a fixable error naming the file
// (and the key), and create nothing.
func (s *Suite) checkKeysRefused() (string, string) {
	if len(s.desc.Keys) == 0 {
		return "the driver lists no key", ""
	}
	missing := filepath.Join(s.scratch, "missing.toml")
	notTOML := filepath.Join(s.scratch, "nottoml.toml")
	if err := os.WriteFile(notTOML, []byte("[driver.\nnot toml"), 0o600); err != nil {
		return "", err.Error()
	}
	for _, k := range s.desc.Keys {
		c := s.probeable(k.Commands)
		if c == "" {
			continue
		}
		for _, f := range []string{missing, notTOML} {
			if msg := s.refused(c, f, ""); msg != "" {
				return "", msg
			}
		}
		if k.Type == "any" {
			continue
		}
		places := append([]string{k.Key}, k.Aliases...)
		if common, ok := strings.CutPrefix(k.Key, "driver."); ok &&
			slices.Contains(commonKeys, common) {
			places = append(places, "driver."+s.desc.Name+"."+common)
		}
		for i, p := range places {
			file := filepath.Join(s.scratch, fmt.Sprintf("wrongtype-%d.toml", i))
			table, key := p[:strings.LastIndex(p, ".")], p[strings.LastIndex(p,
				".")+1:]
			body := "[" + table + "]\n" + key + " = " + wrongValue(k.Type) + "\n"
			if err := os.WriteFile(file, []byte(body), 0o600); err != nil {
				return "", err.Error()
			}
			if msg := s.refused(c, file, key); msg != "" {
				return "", msg
			}
		}
	}
	return "", ""
}

// refused checks that a probe of c with file exits 1 with a fixable error
// naming the file and key, and creates nothing; it says what was wrong, or
// is "".
func (s *Suite) refused(c, file, key string) string {
	o := s.probe(c, map[string]any{"config_file": file})
	text := o.errText()
	if o.code != 1 || o.errCode() != "fixable" || !strings.Contains(text, file) ||
		!strings.Contains(text, key) {
		return fmt.Sprintf("a probe of %s with config file %s (key %q): exit %d, "+
			"error %v; want exit 1 and a fixable error naming them", c, file, key,
			o.code, o.result)
	}
	if made := s.created(); made != "" {
		return fmt.Sprintf("a probe of %s with a bad config file created %s", c, made)
	}
	return ""
}

// checkReapFileGone: reap of an unknown build, sent a config file that does
// not exist, ends as it does with none named.
func (s *Suite) checkReapFileGone() (string, string) {
	gone := filepath.Join(s.scratch, "gone.toml")
	plain := s.reap(nil)
	o := s.reap(map[string]any{"config_file": gone})
	if !o.same(plain) {
		return "", fmt.Sprintf("reap with a config file that is gone: exit %d, error %v; "+
			"with none named, exit %d", o.code, o.result, plain.code)
	}
	return "", ""
}

// reap runs reap of the unknown build, not as a probe.
func (s *Suite) reap(fields map[string]any) *outcome {
	f := map[string]any{"probe": nil}
	for k, v := range fields {
		f[k] = v
	}
	o := s.probe("reap", f)
	s.created()
	return o
}

// checkProbes: each listed command answers a probe within 30 s with exit 0,
// or 1 and a fixable error, and creates nothing.
func (s *Suite) checkProbes() (string, string) {
	for _, c := range probed {
		if !s.lists(c) {
			continue
		}
		o := s.probe(c, nil)
		switch {
		case o.timedOut:
			return "", fmt.Sprintf("a probe of %s did not end within 30 s", c)
		case o.code == 1 && o.errCode() == "fixable":
		case o.code != 0:
			return "", fmt.Sprintf("a probe of %s: exit %d, error %v; want 0, or 1 "+
				"and a fixable error", c, o.code, o.result)
		case c == "run" && o.result["type"] != "stopped":
			return "", fmt.Sprintf("a probe of run answered %v, want stopped",
				o.result)
		}
		if made := s.created(); made != "" {
			return "", fmt.Sprintf("a probe of %s created %s", c, made)
		}
	}
	return "", ""
}

// checkUnlisted: each command of the contract the driver does not list
// exits 3.
func (s *Suite) checkUnlisted() (string, string) {
	for _, c := range contractCommands {
		if s.lists(c) {
			continue
		}
		o := s.run(call{command: c, request: []byte(`{"contract":1}`)})
		if o.code != 3 {
			return "", fmt.Sprintf("%s, which the driver does not list, exited %d, "+
				"not 3", c, o.code)
		}
	}
	return "", ""
}

// checkContractVersion: a version the driver does not list exits 4.
func (s *Suite) checkContractVersion() (string, string) {
	v := 2
	for slices.Contains(s.desc.Contract, v) {
		v++
	}
	o := s.reap(map[string]any{"contract": v})
	if o.code != 4 {
		return "", fmt.Sprintf("reap with contract %d exited %d, not 4", v, o.code)
	}
	return "", ""
}

// checkInvalidRequest: a request that is not JSON exits 2.
func (s *Suite) checkInvalidRequest() (string, string) {
	o := s.run(call{command: "reap", argv: []string{unknownID}, request: []byte("not json")})
	if o.code != 2 {
		return "", fmt.Sprintf("reap with a request that is not JSON exited %d, not 2",
			o.code)
	}
	return "", ""
}

// checkReapUnknown: reap of an unknown build exits 0 with removed 0, or 1
// with a fixable error, the same twice.
func (s *Suite) checkReapUnknown() (string, string) {
	first, second := s.reap(nil), s.reap(nil)
	for _, o := range []*outcome{first, second} {
		removed, _ := o.result["removed"].(float64)
		ok := (o.code == 0 && removed == 0 && o.result != nil) ||
			(o.code == 1 && o.errCode() == "fixable")
		if !ok {
			return "", fmt.Sprintf("reap of an unknown build: exit %d, result %v",
				o.code,
				o.result)
		}
	}
	if !first.same(second) {
		return "", "reap of an unknown build ended differently the second time"
	}
	return "", ""
}

// checkRunNoStart: run whose input ends before start exits with nothing on
// stdout.
func (s *Suite) checkRunNoStart() (string, string) {
	o := s.run(call{command: "run", request: []byte{}})
	if o.timedOut || len(o.stdout) != 0 {
		return "", fmt.Sprintf("run with no start: timed out %v, stdout %q", o.timedOut,
			o.stdout)
	}
	return "", ""
}

// checkReadOnly: image-list exits 0 with a count equal to the lines it
// printed; it and doctor change nothing under the temporary directory.
func (s *Suite) checkReadOnly() (string, string) {
	if s.lists("image-list") {
		o := s.run(call{command: "image-list", request: []byte(`{"contract":1}`)})
		count, _ := o.result["count"].(float64)
		lines := strings.Count(string(o.stdout), "\n")
		if o.code != 0 || int(count) != lines {
			return "", fmt.Sprintf("image-list: exit %d, count %v, %d lines printed",
				o.code, o.result["count"], lines)
		}
		if made := s.created(); made != "" {
			return "", "image-list created " + made
		}
	}
	if s.lists("doctor") {
		s.run(call{command: "doctor", request: s.request("doctor",
			map[string]any{"probe": nil})})
		if made := s.created(); made != "" {
			return "", "doctor created " + made
		}
	}
	return "", ""
}

// checkExtNoRequest: ext with no arguments and no request file does not exit
// 2.
func (s *Suite) checkExtNoRequest() (string, string) {
	if !s.lists("ext") {
		return "ext is not listed", ""
	}
	if o := s.run(call{command: "ext"}); o.code == 2 {
		return "", "ext with no request file exited 2"
	}
	return "", ""
}
