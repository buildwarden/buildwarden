package main

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// runInitialize orchestrates the full build environment initialization:
//  1. Configure network (platform-specific DNS/IP)
//  2. Wait for relay health
//  3. Fetch + install CA
//  4. Set environment variables
//  5. Fetch build script from relay
//  6. Exec build script
func runInitialize(args []string) int {
	gateway, selfIP, script := parseInitArgs(args)
	if gateway == "" {
		gateway = os.Getenv("WARDEN_GATEWAY")
	}
	if selfIP == "" {
		selfIP = os.Getenv("WARDEN_IP")
	}
	if script == "" {
		script = defaultScriptName()
	}
	_ = selfIP

	// Step 1: Platform-specific network configuration
	logStep("configuring network")
	if err := configureNetwork(gateway, selfIP); err != nil {
		fmt.Fprintf(os.Stderr, "warden-io: network config: %s\n", err)
		return 1
	}

	// Step 2: Wait for relay health
	logStep("waiting for relay")
	if err := waitForRelay(gateway); err != nil {
		fmt.Fprintf(os.Stderr, "warden-io: relay not ready: %s\n", err)
		return 1
	}

	// Step 3: Fetch and install CA
	logStep("installing CA")
	if err := fetchAndInstallCA(); err != nil {
		fmt.Fprintf(os.Stderr, "warden-io: CA install: %s\n", err)
		return 1
	}

	// Step 4: Set environment variables for tools that need explicit CA paths
	setCAEnvironment()

	// Step 5: Fetch build script from the canonical /build-script endpoint. The
	// local filename (and therefore the interpreter picked by scriptCommand)
	// still comes from the platform default or --script; only the source is
	// standardized, so the guest no longer needs to know where the script
	// physically lives (shared context vs. streamed from the collector).
	logStep("fetching build script")
	scriptPath := scriptDestPath(script)
	if err := fetchBuildScript(scriptPath); err != nil {
		fmt.Fprintf(os.Stderr, "warden-io: fetch script: %s\n", err)
		return 1
	}
	os.Chmod(scriptPath, 0755) //nolint:errcheck

	// Step 6: Run build script, report exit code to relay
	logStep("running " + script)
	code := execScript(scriptPath, gateway)
	reportComplete(code)
	return code
}

func waitForRelay(gateway string) error {
	healthURL := "http://artifacts/health"
	if gateway != "" {
		healthURL = fmt.Sprintf("http://%s:8300/health", gateway)
	}

	client := &http.Client{Timeout: 2 * time.Second}
	deadline := time.Now().Add(60 * time.Second)

	for time.Now().Before(deadline) {
		resp, err := client.Get(healthURL)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("relay did not become healthy within 60s")
}

// fetchBuildScript downloads the build script from the relay's canonical
// build-script endpoint and writes it to dest. Unlike the generic context fetch
// (http://cwd/<file>), this is a well-known resource the relay resolves itself
// (from the shared context, or on a diskless relay by streaming from the
// collector), so the guest never needs to know where the script physically
// lives.
func fetchBuildScript(dest string) error {
	resp, err := http.Get("http://artifacts/build-script")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	if dir := filepath.Dir(dest); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, resp.Body)
	return err
}

func fetchAndInstallCA() error {
	resp, err := http.Get("http://artifacts/ca.pem")
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	pem, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading CA: %w", err)
	}

	return installCA(pem)
}

func setCAEnvironment() {
	bundlePath := findCABundle()
	if bundlePath == "" {
		return
	}

	// Set environment variables used by common tools
	os.Setenv("SSL_CERT_FILE", bundlePath)
	os.Setenv("CARGO_HTTP_CAINFO", bundlePath)
	os.Setenv("REQUESTS_CA_BUNDLE", bundlePath)
	os.Setenv("NODE_EXTRA_CA_CERTS", bundlePath)
	os.Setenv("CURL_CA_BUNDLE", bundlePath)
}

// scriptDestPath returns where the fetched build script is written on the
// guest. It preserves the script's base name (and thus its extension, which
// scriptCommand uses to pick an interpreter) under the OS temp directory.
func scriptDestPath(script string) string {
	return filepath.Join(os.TempDir(), filepath.Base(script))
}

func execScript(path, gateway string) int {
	cmd := scriptCommand(path)
	cmd.Stdin = os.Stdin

	// Capture build output while echoing it to the local console (serial), and
	// STREAM it to the collector as it is produced rather than buffering the
	// whole log in memory. The build's stdout/stderr fan out to os.Stdout and an
	// io.Pipe; a reader goroutine (below) POSTs each chunk to /v1/output, so a
	// chatty build never costs more than one ~32KB chunk of RAM. Chunk boundaries
	// don't matter: the collector appends raw bytes, so build-output.log is
	// byte-identical regardless of how the stream is split.
	pr, pw := io.Pipe()
	w := io.MultiWriter(os.Stdout, pw)
	cmd.Stdout = w
	cmd.Stderr = w

	// Ensure warden-io is on PATH for the build script (e.g. `warden-io post`).
	// Match the PATH key case-INSENSITIVELY (Windows exposes it as "Path=") and
	// join with the OS path-list separator (';' on Windows, ':' elsewhere); a
	// hardcoded ":" + "PATH=" silently no-ops on Windows. Append a PATH if none
	// exists.
	env := os.Environ()
	if self, err := os.Executable(); err == nil {
		selfDir := filepath.Dir(self)
		sep := string(os.PathListSeparator)
		found := false
		for i, e := range env {
			if len(e) >= 5 && strings.EqualFold(e[:5], "PATH=") {
				env[i] = e[:5] + selfDir + sep + e[5:]
				found = true
				break
			}
		}
		if !found {
			env = append(env, "PATH="+selfDir)
		}
	}
	cmd.Env = env

	if err := cmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "warden-io: exec: %s\n", err)
		return 1
	}

	// Stream build output to the collector as it is produced. Reading pr keeps
	// the build's stdout/stderr flowing: io.Pipe is unbuffered, so the build
	// blocks only for as long as one chunk POST takes against the local relay
	// link. If that link fails mid-build the reader keeps draining (and
	// discarding) so the build never hangs on a dead relay; the tail is then
	// only in the guest console, which is logged.
	streamDone := make(chan struct{})
	go func() {
		defer close(streamDone)
		client := &http.Client{Timeout: 30 * time.Second}
		buf := make([]byte, 32*1024)
		deliver := gateway != ""
		for {
			n, rerr := pr.Read(buf)
			if n > 0 && deliver {
				if !postBuildOutputChunk(client, gateway, buf[:n]) {
					deliver = false
					fmt.Fprintln(os.Stderr, "warden-io: output streaming disabled after a relay-link error; remaining build output stays in the guest console only")
				}
			}
			if rerr != nil {
				return
			}
		}
	}()

	// Send heartbeats while build runs
	done := make(chan struct{})
	go func() {
		client := &http.Client{Timeout: 2 * time.Second}
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				resp, err := client.Get("http://artifacts/heartbeat")
				if err == nil {
					resp.Body.Close()
				}
			}
		}
	}()

	err := cmd.Wait()
	close(done)

	// cmd.Wait has copied all stdout/stderr into the pipe; close the writer so
	// the streamer sees EOF, then wait for it to deliver the final chunk before
	// returning. The caller reports completion right after, which triggers relay
	// teardown, so the tail must already be on its way.
	_ = pw.Close()
	<-streamDone

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return exitErr.ExitCode()
		}
		fmt.Fprintf(os.Stderr, "warden-io: exec: %s\n", err)
		return 1
	}
	return 0
}

// postBuildOutputChunk streams one chunk of build output to the relay's
// control-plane /v1/output endpoint (gateway:8300), which forwards it to the
// collector's build-output.log. Returns false on failure so the caller can stop
// delivering (and just drain) rather than stalling the build on a dead link.
// Best-effort by design: a dropped chunk weakens the convenience log but never
// fails the build.
func postBuildOutputChunk(client *http.Client, gateway string, data []byte) bool {
	if gateway == "" || len(data) == 0 {
		return false
	}
	url := fmt.Sprintf("http://%s:8300/v1/output", gateway)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		fmt.Fprintf(os.Stderr, "warden-io: post build output: %s\n", err)
		return false
	}
	req.Header.Set("Content-Type", "text/plain")
	resp, err := client.Do(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warden-io: post build output: %s\n", err)
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "warden-io: post build output: HTTP %d\n", resp.StatusCode)
		return false
	}
	return true
}

func logStep(msg string) {
	fmt.Fprintf(os.Stderr, "warden-io: %s\n", msg)
}

// reportComplete notifies the relay that the build is finished.
func reportComplete(code int) {
	url := fmt.Sprintf("http://artifacts/exit?code=%d", code)
	resp, err := http.Post(url, "", strings.NewReader(""))
	if err == nil {
		resp.Body.Close()
	}
}

func parseInitArgs(args []string) (gateway, selfIP, script string) {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if val, ok := strings.CutPrefix(arg, "--gateway="); ok {
			gateway = val
		} else if arg == "--gateway" {
			i++
			if i < len(args) {
				gateway = args[i]
			}
		} else if val, ok := strings.CutPrefix(arg, "--ip="); ok {
			selfIP = val
		} else if arg == "--ip" {
			i++
			if i < len(args) {
				selfIP = args[i]
			}
		} else if val, ok := strings.CutPrefix(arg, "--script="); ok {
			script = val
		} else if arg == "--script" {
			i++
			if i < len(args) {
				script = args[i]
			}
		}
	}
	return
}
