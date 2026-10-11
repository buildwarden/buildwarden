package driver

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Host-side helpers shared by every driver: staging the guest binaries
// (relay, warden-io) and plain file copies.

// CacheDir is warden's per-user cache root (<UserCacheDir>/warden).
func CacheDir() string {
	if dir, err := os.UserCacheDir(); err == nil {
		return filepath.Join(dir, "warden")
	}
	return filepath.Join(os.TempDir(), "warden-cache")
}

// GuestBinary returns a path to cmd/<pkg> ("relay" or "warden-io") built for
// goos/goarch. Lookup order:
//
//  1. a prebuilt release binary next to the warden executable, named as
//     `make build` names it (warden-relay-linux-arm64, warden-io-windows-amd64.exe, ...);
//  2. a cached cross-build under CacheDir()/bin that is newer than both the
//     module's go.sum and the warden executable;
//  3. a fresh `go build` into that cache (needs the source tree).
func GuestBinary(pkg, goos, goarch string) (string, error) {
	name := pkg + "-" + goos + "-" + goarch
	if pkg == "relay" {
		name = "warden-" + name
	}
	if goos == "windows" {
		name += ".exe"
	}

	if exe, err := os.Executable(); err == nil {
		cand := filepath.Join(filepath.Dir(exe), name)
		if _, err := os.Stat(cand); err == nil {
			return cand, nil
		}
	}

	cached := filepath.Join(CacheDir(), "bin", name)
	root, rootErr := ModuleRoot()
	if !isStale(cached, root) {
		return cached, nil
	}
	if rootErr != nil {
		return "", fmt.Errorf("%s not found next to warden or in %s, and no source tree to build it from: %w",
			name, filepath.Dir(cached), rootErr)
	}
	if err := os.MkdirAll(filepath.Dir(cached), 0o755); err != nil {
		return "", err
	}
	cmd := exec.Command("go", "build", "-ldflags=-s -w", "-o", cached, "./cmd/"+pkg)
	cmd.Env = append(os.Environ(), "GOOS="+goos, "GOARCH="+goarch, "CGO_ENABLED=0")
	cmd.Dir = root
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("building %s: %w", name, err)
	}
	return cached, nil
}

// isStale reports whether cached is missing or older than the module's go.sum
// or the running warden binary (a dev rebuild).
func isStale(cached, root string) bool {
	info, err := os.Stat(cached)
	if err != nil {
		return true
	}
	if root != "" {
		if s, err := os.Stat(filepath.Join(root, "go.sum")); err == nil && s.ModTime().After(info.ModTime()) {
			return true
		}
	}
	if exe, err := os.Executable(); err == nil {
		if e, err := os.Stat(exe); err == nil && e.ModTime().After(info.ModTime()) {
			return true
		}
	}
	return false
}

// ModuleRoot finds the buildwarden source tree (the directory holding go.mod),
// walking up from the warden executable and then from the working directory.
func ModuleRoot() (string, error) {
	var starts []string
	if exe, err := os.Executable(); err == nil {
		starts = append(starts, filepath.Dir(exe))
	}
	if wd, err := os.Getwd(); err == nil {
		starts = append(starts, wd)
	}
	for _, dir := range starts {
		for {
			if data, err := os.ReadFile(filepath.Join(dir, "go.mod")); err == nil &&
				strings.Contains(string(data), "module github.com/buildwarden/buildwarden") {
				return dir, nil
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}
	return "", fmt.Errorf("buildwarden source tree (go.mod) not found")
}

// CopyFile copies src to dst, preserving the permission bits.
func CopyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if info, err := in.Stat(); err == nil {
		_ = out.Chmod(info.Mode())
	}
	return out.Close()
}

// OrDefault returns v, or def when v is zero.
func OrDefault(v, def int) int {
	if v == 0 {
		return def
	}
	return v
}

// RemoveWorkDir deletes a build's work directory (overlay disks, seeds,
// staged inputs), unless --keep-vms asked to leave it for inspection.
func RemoveWorkDir(req *BuildRequest, dir string) {
	if req.KeepVMs {
		fmt.Fprintf(os.Stderr, "[warden] --keep-vms: leaving work dir %s\n", dir)
		return
	}
	_ = os.RemoveAll(dir)
}
