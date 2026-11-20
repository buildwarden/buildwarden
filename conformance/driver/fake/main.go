// Command warden-driver-fake is the driver contract's reference fake
// (specs/driver-contract/v1.md). It implements all eleven commands with the
// standard library and a TOML parser, and none of the kit, to show what a
// driver must do by hand. Its "relay" is stubrelay, found beside it, and its
// work is files under the temporary directory it is given.
//
// It declares one flag with a value (--greeting), one switch (--shout), one
// key of its own ([driver.fake] greeting) and one common key
// (image_refresh_after), which it only echoes: in a probe it prints each
// value on stderr with the place it came from.
//
// WARDEN_FAKE_FAULT names one fault for the suite's self-test (faults.go).
// WARDEN_FAKE_TEARDOWN, a Go duration, makes run's teardown take that long
// after it has stopped the relay, so that a test can signal warden again
// while the driver tears down.
package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strconv"
	"strings"
)

const name = "fake"

// describe is what the fake's describe prints, less what a fault changes.
func describe() map[string]any {
	commands := []string{"run", "reap", "doctor", "setup", "image-prepare", "image-list",
		"image-verify", "clean", "attach", "ext"}
	if fault == faultUndeclaredCommand {
		commands = slices.DeleteFunc(commands, func(c string) bool { return c == "setup" })
	}
	return map[string]any{
		"contract": []int{1}, "name": name, "version": "1.0.0", "vendor": "buildwarden",
		"bug_url":  "https://github.com/buildwarden/buildwarden/issues/new",
		"commands": commands,
		"capabilities": map[string]any{"vm": false, "shell": true, "keep": "stopped",
			"exited": true, "containerfile": "translate",
			"guest_os": []string{"linux"},
			"image_os": []string{"linux"}, "default_image_os": "linux"},
		"flags": []map[string]any{
			{"name": "greeting", "value": true, "commands": greetingCommands,
				"usage": "the greeting to echo " +
					"(default: [driver.fake] greeting, then hello)"},
			{"name": "shout", "value": false, "commands": []string{"run"},
				"usage": "echo the greeting in capitals"},
		},
		"keys": []map[string]any{
			{"key": "driver.fake.greeting", "type": "string",
				"commands": greetingCommands,
				"usage":    "the greeting to echo"},
			{"key": "driver.image_refresh_after", "type": "string",
				"commands": []string{"run"},
				"usage":    "an age, which the fake only echoes"},
		},
		"ext": []map[string]any{{"path": []string{"echo"},
			"usage": "print its arguments"}},
		"help": map[string]string{
			"doctor": "Checks nothing; a fake host is always ready."},
	}
}

// greetingCommands are the commands that take --greeting and read
// [driver.fake] greeting.
var greetingCommands = []string{"run", "doctor", "setup", "image-prepare", "clean"}

// fixable is a failure with a known remedy: the contract's fixable error.
type fixable struct{ msg, fix string }

func (e *fixable) Error() string { return e.msg }

// codeError is an error object with its own code and exit code.
type codeError struct {
	code string
	exit int
	msg  string
}

func (e *codeError) Error() string { return e.msg }

func invalid(format string, args ...any) error {
	exit := 2
	if fault == faultWrongExit {
		exit = 1
	}
	return &codeError{code: "invalid-request", exit: exit, msg: fmt.Sprintf(format, args...)}
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "warden-driver-fake: no command")
		os.Exit(2)
	}
	command := os.Args[1]
	switch command {
	case "describe":
		data, _ := json.Marshal(describe())
		fmt.Println(string(data))
		return
	case "run":
		os.Exit(run())
	}
	os.Exit(oneShot(command, os.Args[2:]))
}

// request is the fields of every request but start that the fake reads.
type request struct {
	Contract   int      `json:"contract"`
	Probe      bool     `json:"probe"`
	BuildID    string   `json:"build_id"`
	TempDir    string   `json:"tmpdir"`
	StateFile  string   `json:"state_file"`
	ConfigFile string   `json:"config_file"`
	Args       []string `json:"args"`
	OS         string   `json:"os"`
	KeepIDs    []string `json:"keep_ids"`
	Cache      bool     `json:"cache"`
	fields     map[string]json.RawMessage
}

// oneShot runs a command that answers with a result file.
func oneShot(command string, argv []string) int {
	res, err := answer(command, argv)
	if command == "ext" {
		if err != nil {
			fmt.Fprintln(os.Stderr, "warden-driver-fake:", err)
			return 1
		}
		return 0
	}
	code := 0
	out := res
	if command == "attach" && err == nil {
		code, _ = res["exit_code"].(int)
		out = map[string]any{}
	}
	if err != nil {
		obj, exit := errorObject(err)
		out, code = map[string]any{"error": obj}, exit
		fmt.Fprintln(os.Stderr, "warden-driver-fake:", err)
	}
	if path := os.Getenv("WARDEN_DRIVER_RESULT"); path != "" {
		data, _ := json.Marshal(out)
		_ = os.WriteFile(path, data, 0o600)
	}
	return code
}

func errorObject(err error) (map[string]any, int) {
	var f *fixable
	var c *codeError
	switch {
	case errors.As(err, &f):
		obj := map[string]any{"code": "fixable", "msg": f.msg}
		if f.fix != "" {
			obj["fix"] = f.fix
		}
		return obj, 1
	case errors.As(err, &c):
		return map[string]any{"code": c.code, "msg": c.msg}, c.exit
	}
	return map[string]any{"code": "internal", "msg": err.Error()}, 1
}

var known = []string{"reap", "doctor", "setup", "image-prepare", "image-list",
	"image-verify", "clean", "attach", "ext"}

// answer reads and checks command's request, then runs it.
func answer(command string, argv []string) (map[string]any, error) {
	listed, _ := describe()["commands"].([]string)
	if !slices.Contains(known, command) ||
		(!slices.Contains(listed, command) && fault != faultUndeclaredCommand) {
		return nil, &codeError{code: "unsupported-command", exit: 3,
			msg: fmt.Sprintf("the fake does not implement %q", command)}
	}
	req, err := readRequest(command)
	if err != nil {
		return nil, err
	}
	if req == nil {
		// ext started by the system: no config file, the defaults.
		req = &request{}
	}
	cfg, err := readConfig(command, req.ConfigFile, req.Args, req.Probe)
	if err != nil {
		return nil, err
	}
	if req.Probe {
		cfg.echo()
		return map[string]any{}, nil
	}
	return commands[command](req, cfg, argv)
}

// readRequest reads command's request from stdin, or from the request file
// for attach and ext; it is nil for an ext with no request file.
func readRequest(command string) (*request, error) {
	var data []byte
	var err error
	if command == "attach" || command == "ext" {
		path := os.Getenv("WARDEN_DRIVER_REQUEST")
		if path == "" && command == "ext" {
			return nil, nil
		}
		data, err = os.ReadFile(path)
	} else {
		data, err = io.ReadAll(os.Stdin)
	}
	if err != nil {
		return nil, invalid("reading the request: %v", err)
	}
	req := &request{}
	if err := json.Unmarshal(data, &req.fields); err != nil || req.fields == nil {
		return nil, invalid("the request is not a JSON object")
	}
	if err := json.Unmarshal(data, req); err != nil {
		return nil, invalid("the request: %v", err)
	}
	if req.Contract == 0 {
		return nil, invalid("the request has no contract version")
	}
	if req.Contract != 1 {
		return nil, &codeError{code: "unsupported-contract", exit: 4,
			msg: fmt.Sprintf("the fake speaks contract 1, not %d", req.Contract)}
	}
	for _, f := range map[string][]string{"reap": {"build_id", "state_file"},
		"attach": {"build_id", "state_file"}, "clean": {"keep_ids"},
		"image-prepare": {"os"}}[command] {
		if _, ok := req.fields[f]; !ok {
			return nil, invalid("the %s request has no %s", command, f)
		}
	}
	return req, nil
}

// lines reads run's input one message at a time.
func lines(r io.Reader) <-chan map[string]any {
	ch := make(chan map[string]any)
	go func() {
		defer close(ch)
		sc := bufio.NewScanner(r)
		sc.Buffer(make([]byte, 64<<10), 1<<20+1)
		for sc.Scan() {
			var m map[string]any
			if json.Unmarshal(sc.Bytes(), &m) == nil {
				ch <- m
			}
		}
	}()
	return ch
}

// atoi is strconv.Atoi with 0 for anything that is not a number.
func atoi(s string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(s))
	return n
}
