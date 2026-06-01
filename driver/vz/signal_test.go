package vz

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCheckBuildStatus_NotStarted(t *testing.T) {
	dir := t.TempDir()
	heartbeat := filepath.Join(dir, "heartbeat")
	exitCode := filepath.Join(dir, "exit_code")

	status, code := checkBuildStatus(heartbeat, exitCode)
	if status != BuildNotStarted {
		t.Errorf("expected BuildNotStarted, got %d", status)
	}
	if code != 0 {
		t.Errorf("expected code 0, got %d", code)
	}
}

func TestCheckBuildStatus_Running(t *testing.T) {
	dir := t.TempDir()
	heartbeat := filepath.Join(dir, "heartbeat")
	exitCode := filepath.Join(dir, "exit_code")

	_ = os.WriteFile(heartbeat, []byte("1234567890"), 0644)

	status, _ := checkBuildStatus(heartbeat, exitCode)
	if status != BuildRunning {
		t.Errorf("expected BuildRunning, got %d", status)
	}
}

func TestCheckBuildStatus_Completed(t *testing.T) {
	dir := t.TempDir()
	heartbeat := filepath.Join(dir, "heartbeat")
	exitCode := filepath.Join(dir, "exit_code")

	_ = os.WriteFile(exitCode, []byte("0\n"), 0644)

	status, code := checkBuildStatus(heartbeat, exitCode)
	if status != BuildCompleted {
		t.Errorf("expected BuildCompleted, got %d", status)
	}
	if code != 0 {
		t.Errorf("expected code 0, got %d", code)
	}
}

func TestCheckBuildStatus_CompletedWithError(t *testing.T) {
	dir := t.TempDir()
	heartbeat := filepath.Join(dir, "heartbeat")
	exitCode := filepath.Join(dir, "exit_code")

	_ = os.WriteFile(exitCode, []byte("1\n"), 0644)

	status, code := checkBuildStatus(heartbeat, exitCode)
	if status != BuildCompleted {
		t.Errorf("expected BuildCompleted, got %d", status)
	}
	if code != 1 {
		t.Errorf("expected code 1, got %d", code)
	}
}

func TestCheckBuildStatus_Unresponsive(t *testing.T) {
	dir := t.TempDir()
	heartbeat := filepath.Join(dir, "heartbeat")
	exitCode := filepath.Join(dir, "exit_code")

	// Write heartbeat file then backdate it
	_ = os.WriteFile(heartbeat, []byte("1234567890"), 0644)
	staleTime := time.Now().Add(-heartbeatTimeout - time.Second)
	_ = os.Chtimes(heartbeat, staleTime, staleTime)

	status, _ := checkBuildStatus(heartbeat, exitCode)
	if status != BuildUnresponsive {
		t.Errorf("expected BuildUnresponsive, got %d", status)
	}
}

func TestWaitForBuild_Completed(t *testing.T) {
	dir := t.TempDir()
	signalDir := dir

	// Simulate build completion after a short delay
	go func() {
		time.Sleep(100 * time.Millisecond)
		_ = os.WriteFile(filepath.Join(signalDir, "heartbeat"), []byte("1"), 0644)
		time.Sleep(100 * time.Millisecond)
		os.Remove(filepath.Join(signalDir, "heartbeat"))
		_ = os.WriteFile(filepath.Join(signalDir, "exit_code"), []byte("0"), 0644)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	code, err := WaitForBuild(ctx, signalDir, false)
	if err != nil {
		t.Fatalf("WaitForBuild error: %v", err)
	}
	if code != 0 {
		t.Errorf("expected exit code 0, got %d", code)
	}
}

func TestWaitForBuild_ContextCancelled(t *testing.T) {
	dir := t.TempDir()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := WaitForBuild(ctx, dir, false)
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
}

