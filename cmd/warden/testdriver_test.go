package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The test binary doubles as a driver binary for the exec transport's
// tests: copied as warden-driver-tst and run with WARDEN_TEST_DRIVER set, it
// plays the variant the WARDEN_TEST_* variables name.
//
//	WARDEN_TEST_DESCRIBE  ok, hang, wrong-name, no-reap, clash, bad-key,
//	                      contract-9
//	WARDEN_TEST_CALL      ok, fixable, busy, exit-2, exit-3, exit-4, exit-7,
//	                      no-result, not-json, hang
//	WARDEN_TEST_RUN       ok, failed, noise, early, no-stop, probe-hang
//	WARDEN_TEST_REAP      ok, fail, hang
//	WARDEN_TEST_LOG       a file each request is appended to, as one line

func TestMain(m *testing.M) {
	if os.Getenv("WARDEN_TEST_DRIVER") != "" {
		os.Exit(testDriver(os.Args[1:]))
	}
	dir, err := os.MkdirTemp("", "warden-test-driver-")
	if err != nil {
		panic(err)
	}
	testDriverPath = filepath.Join(dir, driverFileName("tst", isWindows))
	if err := copyFile(os.Args[0], testDriverPath, 0o755); err != nil {
		panic(err)
	}
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

// testDriverPath is the test binary's copy as warden-driver-tst.
var testDriverPath string

func copyFile(from, to string, mode os.FileMode) error {
	data, err := os.ReadFile(from)
	if err != nil {
		return err
	}
	return os.WriteFile(to, data, mode)
}

// useTestDriver makes the test binary act as a driver in the processes the
// test starts, with the variants vars name.
func useTestDriver(t *testing.T, vars ...string) {
	t.Helper()
	t.Setenv("WARDEN_TEST_DRIVER", "1")
	for _, k := range []string{"WARDEN_TEST_DESCRIBE", "WARDEN_TEST_CALL", "WARDEN_TEST_RUN",
		"WARDEN_TEST_REAP"} {
		t.Setenv(k, "ok")
	}
	for i := 0; i+1 < len(vars); i += 2 {
		t.Setenv(vars[i], vars[i+1])
	}
}

// testDescription is the test driver's describe, less what a variant
// changes.
func testDescription(variant string) map[string]any {
	commands := []string{"run", "reap", "doctor", "setup", "clean", "attach"}
	if variant == "no-reap" {
		commands = []string{"run"}
	}
	flags := []map[string]any{{"name": "greeting", "value": true,
		"commands": []string{"run"}, "usage": "a greeting"}}
	if variant == "clash" {
		flags[0]["name"] = "timeout"
	}
	keys := []map[string]any{{"key": "driver.tst.greeting", "type": "string",
		"commands": []string{"run"}, "usage": "a greeting"}}
	if variant == "bad-key" {
		keys[0]["key"] = "vm.greeting"
	}
	name, contract := "tst", []int{1}
	switch variant {
	case "wrong-name":
		name = "other"
	case "contract-9":
		contract = []int{9}
	}
	return map[string]any{"contract": contract, "name": name, "version": "0.1.0",
		"vendor": "test", "bug_url": "https://example.com/tst/bugs",
		"commands": commands,
		"capabilities": map[string]any{"vm": false, "shell": true, "keep": "none",
			"exited": true, "containerfile": "translate",
			"guest_os": []string{"linux"}},
		"flags": flags, "keys": keys, "ext": []any{}, "help": map[string]string{}}
}

// testDriver is the test binary acting as a driver.
func testDriver(args []string) int {
	if len(args) == 0 {
		return 2
	}
	switch args[0] {
	case "describe":
		v := os.Getenv("WARDEN_TEST_DESCRIBE")
		if v == "hang" {
			time.Sleep(time.Hour)
		}
		data, _ := json.Marshal(testDescription(v))
		fmt.Println(string(data))
		return 0
	case "run":
		return testRun(os.Getenv("WARDEN_TEST_RUN"))
	case "reap":
		return testCall(os.Getenv("WARDEN_TEST_REAP"), `{"removed": 1}`)
	case "attach":
		data, _ := os.ReadFile(os.Getenv(envDriverRequest))
		logRequest("attach", data)
		_ = os.WriteFile(os.Getenv(envDriverResult), []byte(`{}`), 0o600)
		return 5
	}
	return testCall(os.Getenv("WARDEN_TEST_CALL"), `{"blocked": false, "removed": 0}`)
}

func logRequest(command string, data []byte) {
	if path := os.Getenv("WARDEN_TEST_LOG"); path != "" {
		f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
		if err == nil {
			_, _ = fmt.Fprintf(f, "%s %s\n", command, strings.TrimSpace(string(data)))
			_ = f.Close()
		}
	}
}

// testCall answers a command with a result file, as variant says.
func testCall(variant, ok string) int {
	data, _ := io.ReadAll(os.Stdin)
	logRequest(os.Args[1], data)
	result := func(s string) { _ = os.WriteFile(os.Getenv(envDriverResult), []byte(s), 0o600) }
	errObj := func(code string) string {
		return `{"error": {"code": "` + code + `", "msg": "the test driver refused",
			"fix": "do what the test says"}}`
	}
	switch variant {
	case "fixable", "fail":
		result(errObj("fixable"))
		return 1
	case "busy":
		result(errObj("busy"))
		return 1
	case "exit-2":
		result(errObj("invalid-request"))
		return 2
	case "exit-3":
		result(errObj("unsupported-command"))
		return 3
	case "exit-4":
		result(errObj("unsupported-contract"))
		return 4
	case "exit-7":
		result(ok)
		return 7
	case "no-result":
		return 0
	case "not-json":
		result("[1, 2]")
		return 0
	case "hang":
		time.Sleep(time.Hour)
	}
	result(ok)
	return 0
}

// testRun is the test driver's run.
func testRun(variant string) int {
	in := bufio.NewScanner(os.Stdin)
	if !in.Scan() {
		return 0
	}
	var start map[string]any
	_ = json.Unmarshal(in.Bytes(), &start)
	logRequest("run", in.Bytes())
	say := func(s string) { fmt.Println(s) }
	if start["probe"] == true {
		if variant == "probe-hang" {
			time.Sleep(time.Hour)
		}
		say(`{"type": "stopped"}`)
		return 0
	}
	switch variant {
	case "early":
		return 0
	case "failed":
		say(`{"type": "failed", "error": {"code": "fixable", "msg": "no runtime", ` +
			`"fix": "install one"}}`)
		return 1
	}
	say(`{"type": "relay-started", "claim": {"network": "tcp", ` +
		`"address": "127.0.0.1:41233"}, "claim_values": {}, "guest_os": "linux"}`)
	if variant == "noise" {
		say("hello from the driver")
	}
	for in.Scan() {
		var m map[string]any
		_ = json.Unmarshal(in.Bytes(), &m)
		switch m["type"] {
		case "claimed":
			say(`{"type": "environment-started"}`)
		case "stop":
			if variant == "no-stop" {
				time.Sleep(time.Hour)
			}
			say(`{"type": "stopped"}`)
			return 0
		}
	}
	if variant == "no-stop" {
		time.Sleep(time.Hour)
	}
	say(`{"type": "stopped"}`)
	return 0
}
