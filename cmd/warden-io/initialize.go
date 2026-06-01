package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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
		script = "build.sh"
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

	// Step 5: Fetch build script
	logStep("fetching build script")
	scriptPath := "/tmp/build.sh"
	if err := fetchFile(script, scriptPath); err != nil {
		fmt.Fprintf(os.Stderr, "warden-io: fetch script: %s\n", err)
		return 1
	}
	os.Chmod(scriptPath, 0755) //nolint:errcheck

	// Step 6: Run build script, report exit code to relay
	logStep("running " + script)
	code := execScript(scriptPath)
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

func findCABundle() string {
	if runtime.GOOS == "darwin" {
		return "/etc/ssl/cert.pem"
	}
	for _, path := range []string{
		"/etc/ssl/certs/ca-certificates.crt",
		"/etc/pki/tls/certs/ca-bundle.crt",
		"/etc/ssl/cert.pem",
	} {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return ""
}

func execScript(path string) int {
	shell := "/bin/sh"
	if _, err := os.Stat("/bin/bash"); err == nil {
		shell = "/bin/bash"
	}

	cmd := exec.Command(shell, path)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Ensure warden-io is available in PATH for the build script
	env := os.Environ()
	if self, err := os.Executable(); err == nil {
		selfDir := filepath.Dir(self)
		for i, e := range env {
			if strings.HasPrefix(e, "PATH=") {
				env[i] = "PATH=" + selfDir + ":" + e[5:]
				break
			}
		}
	}
	cmd.Env = env

	if err := cmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "warden-io: exec: %s\n", err)
		return 1
	}

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

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return exitErr.ExitCode()
		}
		fmt.Fprintf(os.Stderr, "warden-io: exec: %s\n", err)
		return 1
	}
	return 0
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
