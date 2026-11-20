package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
)

// start is the fields of the start message the fake reads.
type start struct {
	Type           string   `json:"type"`
	Contract       int      `json:"contract"`
	Probe          bool     `json:"probe"`
	BuildID        string   `json:"build_id"`
	Mode           string   `json:"mode"`
	ClaimToken     string   `json:"claim_token"`
	TempDir        string   `json:"tmpdir"`
	DiagnosticsDir string   `json:"diagnostics_dir"`
	StateFile      string   `json:"state_file"`
	Keep           bool     `json:"keep"`
	ConfigFile     string   `json:"config_file"`
	Args           []string `json:"args"`
}

// out writes run's messages; a write that fails is dropped, so that the
// teardown goes on when the orchestrator is gone (DRV-51).
var out = struct {
	sync.Mutex
	w *os.File
}{w: os.Stdout}

func send(m map[string]any) {
	data, _ := json.Marshal(m)
	out.Lock()
	defer out.Unlock()
	_, _ = out.w.Write(append(data, '\n'))
}

func failed(err error) int {
	obj, exit := errorObject(err)
	fmt.Fprintln(os.Stderr, "warden-driver-fake:", err)
	send(map[string]any{"type": "failed", "error": obj})
	return exit
}

// run is the run command: the build's messages on stdin and stdout.
func run() int {
	if fault != faultDieClosedStderr {
		signal.Ignore(syscall.SIGPIPE)
	}
	if fault == faultStrayStdout {
		fmt.Println("a stray line on stdout")
	}
	in := lines(os.Stdin)
	first, ok := <-in
	if !ok {
		if fault == faultIgnoreEOF {
			for {
				time.Sleep(time.Hour)
			}
		}
		return 0
	}
	data, _ := json.Marshal(first)
	var st start
	if err := json.Unmarshal(data, &st); err != nil || st.Type != "start" {
		return failed(invalid("the first message is not start"))
	}
	if st.Contract != 1 {
		return failed(&codeError{code: "unsupported-contract", exit: 4,
			msg: fmt.Sprintf("the fake speaks contract 1, not %d", st.Contract)})
	}
	cfg, err := readConfig("run", st.ConfigFile, st.Args, st.Probe)
	if err != nil {
		return failed(err)
	}
	relay, err := stubRelay()
	if err != nil {
		return failed(err)
	}
	if st.Probe {
		cfg.echo()
		send(map[string]any{"type": "stopped"})
		return 0
	}
	return build(&st, cfg, relay, in)
}

// stubRelay is the stubrelay program beside the fake.
func stubRelay() (string, error) {
	self, err := os.Executable()
	if err != nil {
		return "", err
	}
	name := "stubrelay"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	path := filepath.Join(filepath.Dir(self), name)
	if _, err := os.Stat(path); err != nil {
		return "", &fixable{msg: fmt.Sprintf("the fake's relay %s is missing", path),
			fix: "build conformance/driver/stubrelay beside warden-driver-fake"}
	}
	return path, nil
}

// build runs one build: the stub relay, then the environment, which is a
// line on stderr, until stop or the end of input.
func build(st *start, cfg *config, relayPath string, in <-chan map[string]any) int {
	dir := buildDir(st.TempDir, st.BuildID)
	stData, _ := json.Marshal(state{WorkDir: dir, Shell: st.Mode == "shell"})
	if err := os.WriteFile(st.StateFile, stData, 0o600); err != nil {
		return failed(err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return failed(err)
	}
	relay, addr, err := startRelay(relayPath, st)
	if err != nil {
		_ = os.RemoveAll(dir)
		return failed(err)
	}
	stopping := make(chan struct{})
	go func() {
		_ = relay.Wait()
		select {
		case <-stopping:
		default:
			send(map[string]any{"type": "exited", "which": "relay",
				"detail": "the stub relay exited"})
		}
	}()
	send(map[string]any{"type": "relay-started",
		"claim":        map[string]any{"network": "tcp", "address": addr},
		"claim_values": map[string]any{}, "guest_os": "linux",
		"old_relay_fix": "rebuild stubrelay beside warden-driver-fake"})
	stopped := false
messages:
	for m := range in {
		switch m["type"] {
		case "claimed":
			g := cfg.greeting
			if cfg.shout {
				g = strings.ToUpper(g)
			}
			fmt.Fprintf(os.Stderr, "fake: %s, build %s\n", g, st.BuildID)
			send(map[string]any{"type": "environment-started"})
		case "stop":
			stopped = true
			break messages
		}
	}
	for !stopped && fault == faultIgnoreEOF {
		time.Sleep(time.Hour)
	}
	close(stopping)
	return teardown(st, relay, dir)
}

// teardown stops the relay and removes the build's work, writing its log
// whether or not anyone still reads it.
func teardown(st *start, relay *exec.Cmd, dir string) int {
	if fault == faultNoTeardown {
		send(map[string]any{"type": "stopped"})
		return 0
	}
	_ = relay.Process.Kill()
	if d, err := time.ParseDuration(os.Getenv("WARDEN_FAKE_TEARDOWN")); err == nil {
		time.Sleep(d)
	}
	for i := range 20 {
		if _, err := fmt.Fprintf(os.Stderr, "fake: tearing down (%d)\n", i); err != nil &&
			fault == faultDieClosedStderr {
			os.Exit(1)
		}
	}
	msg := map[string]any{"type": "stopped"}
	if st.Keep {
		msg["kept"] = []string{dir}
	} else if err := os.RemoveAll(dir); err != nil {
		return failed(err)
	}
	send(msg)
	return 0
}

// startRelay starts the stub relay with the claim token and returns the
// address it accepts its claim on.
func startRelay(path string, st *start) (*exec.Cmd, string, error) {
	cmd := exec.Command(path, "--listen", "127.0.0.1:0")
	cmd.Env = append(os.Environ(), "WARDEN_CLAIM_TOKEN="+st.ClaimToken)
	if st.DiagnosticsDir != "" {
		if log, err := os.Create(filepath.Join(st.DiagnosticsDir,
			"relay.log")); err == nil {
			cmd.Stderr = log
		}
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, "", err
	}
	if err := cmd.Start(); err != nil {
		return nil, "", err
	}
	line, err := bufio.NewReader(stdout).ReadString('\n')
	addr, ok := strings.CutPrefix(strings.TrimSpace(line), "listening ")
	if err != nil || !ok {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return nil, "", fmt.Errorf("the stub relay did not report its address: %q", line)
	}
	return cmd, addr, nil
}
