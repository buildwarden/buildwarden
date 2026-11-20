package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
)

// commands are the fake's commands but describe and run.
var commands = map[string]func(*request, *config, []string) (map[string]any, error){
	"reap": reap, "doctor": doctor, "setup": setup, "image-prepare": prepare,
	"image-list": list, "image-verify": verify, "clean": clean, "attach": attach,
	"ext": ext,
}

// root is where the fake keeps its work: the request's temporary directory,
// else the system's.
func root(tmpdir string) string {
	if tmpdir != "" {
		return tmpdir
	}
	return os.TempDir()
}

// buildDir is a build's work directory, named from its ID.
func buildDir(tmpdir, id string) string {
	return filepath.Join(root(tmpdir), "warden-fake-"+id)
}

func imagesDir() string { return filepath.Join(root(""), "warden-fake", "images") }

// state is the fake's state file: what reap and attach need of a build.
type state struct {
	WorkDir string `json:"work_dir"`
	Shell   bool   `json:"shell"`
}

func loadState(path string) (*state, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	var st state
	if json.Unmarshal(data, &st) != nil {
		return nil, false
	}
	return &st, true
}

func reap(req *request, _ *config, _ []string) (map[string]any, error) {
	dir := buildDir(req.TempDir, req.BuildID)
	if st, ok := loadState(req.StateFile); ok {
		dir = st.WorkDir
	}
	removed := 0
	if _, err := os.Stat(dir); err == nil && fault != faultReapNoLook {
		stopRelay(dir)
		if err := os.RemoveAll(dir); err != nil {
			return nil, err
		}
		removed = 1
	}
	return map[string]any{"removed": removed}, nil
}

// relayPIDFile, in a build's work directory, holds its relay's process ID.
const relayPIDFile = "relay.pid"

// stopRelay kills the relay of the build whose work directory is dir, left
// running when its driver was killed.
func stopRelay(dir string) {
	data, err := os.ReadFile(filepath.Join(dir, relayPIDFile))
	if err != nil {
		return
	}
	if p, err := os.FindProcess(atoi(string(data))); err == nil && atoi(string(data)) > 0 {
		_ = p.Kill()
	}
}

func doctor(_ *request, cfg *config, _ []string) (map[string]any, error) {
	fmt.Printf("fake: %s\n", cfg.greeting)
	return map[string]any{"blocked": false}, nil
}

func setup(_ *request, _ *config, _ []string) (map[string]any, error) {
	dir := filepath.Join(root(""), "warden-fake")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	return map[string]any{}, os.WriteFile(filepath.Join(dir, "setup.done"), nil, 0o600)
}

func prepare(req *request, _ *config, _ []string) (map[string]any, error) {
	if err := os.MkdirAll(imagesDir(), 0o700); err != nil {
		return nil, err
	}
	path := filepath.Join(imagesDir(), req.OS+".img")
	fmt.Fprintf(os.Stderr, "fake: preparing %s\n", path)
	if err := os.WriteFile(path, []byte(req.OS), 0o600); err != nil {
		return nil, err
	}
	return map[string]any{"path": path}, nil
}

func images() []string {
	entries, _ := os.ReadDir(imagesDir())
	var out []string
	for _, e := range entries {
		out = append(out, e.Name())
	}
	return out
}

func list(_ *request, _ *config, _ []string) (map[string]any, error) {
	imgs := images()
	for _, i := range imgs {
		fmt.Println(filepath.Join(imagesDir(), i))
	}
	return map[string]any{"count": len(imgs)}, nil
}

func verify(_ *request, _ *config, _ []string) (map[string]any, error) {
	imgs := images()
	for _, i := range imgs {
		fmt.Printf("ok %s\n", i)
	}
	return map[string]any{"checked": len(imgs), "drift": 0}, nil
}

func clean(req *request, _ *config, _ []string) (map[string]any, error) {
	entries, _ := os.ReadDir(root(req.TempDir))
	removed := 0
	for _, e := range entries {
		id, ok := strings.CutPrefix(e.Name(), "warden-fake-")
		if !ok || len(id) != 8 || slices.Contains(req.KeepIDs, id) {
			continue
		}
		path := filepath.Join(root(req.TempDir), e.Name())
		fmt.Fprintf(os.Stderr, "Removing %s\n", path)
		stopRelay(path)
		if err := os.RemoveAll(path); err != nil {
			return nil, err
		}
		removed++
	}
	if req.Cache {
		if err := os.RemoveAll(imagesDir()); err != nil {
			return nil, err
		}
	}
	return map[string]any{"removed": removed}, nil
}

// attach runs a host shell on the inherited terminal for a build held in
// mode shell, and exits with its code.
func attach(req *request, _ *config, _ []string) (map[string]any, error) {
	st, ok := loadState(req.StateFile)
	if !ok || !st.Shell {
		return nil, &fixable{msg: fmt.Sprintf("no build %s is held for a shell",
			req.BuildID),
			fix: "start one with warden shell"}
	}
	shell := exec.Command("sh")
	if runtime.GOOS == "windows" {
		shell = exec.Command("cmd.exe", "/Q")
	}
	shell.Dir = st.WorkDir
	shell.Stdin, shell.Stdout, shell.Stderr = os.Stdin, os.Stdout, os.Stderr
	err := shell.Run()
	if ee, isExit := err.(*exec.ExitError); isExit {
		return map[string]any{"exit_code": ee.ExitCode()}, nil
	}
	if err != nil {
		return nil, err
	}
	return map[string]any{"exit_code": 0}, nil
}

func ext(_ *request, _ *config, argv []string) (map[string]any, error) {
	if len(argv) == 0 || argv[0] != "echo" {
		return nil, fmt.Errorf("the fake's only subcommand is echo")
	}
	fmt.Println(strings.Join(argv[1:], " "))
	return nil, nil
}
