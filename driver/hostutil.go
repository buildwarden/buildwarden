package driver

import (
	"crypto/sha256"
	"encoding/hex"
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
//  2. a cached cross-build of this source tree, under CacheDir()/bin/<tree>
//     (guestCachePath), that is newer than both the module's go.sum and the
//     warden executable;
//  3. a fresh `go build` into that cache (needs the source tree).
//
// The cache is keyed by the source tree because every checkout on a host
// shares CacheDir(): one flat cache let a newer warden from another checkout
// rebuild a guest binary from its own source, which this checkout's builds
// then staged.
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

	root, rootErr := ModuleRoot()
	if rootErr != nil {
		return "", fmt.Errorf(
			"%s not found next to warden, and no source tree to build it "+
				"from: %w", name, rootErr)
	}
	cached := guestCachePath(CacheDir(), root, name)
	if !isStale(cached, root) {
		return cached, nil
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

// guestCachePath is where GuestBinary caches name built from the source tree
// at root: a directory per tree, named by a hash of its absolute path, so two
// checkouts never stage each other's builds.
func guestCachePath(cacheDir, root, name string) string {
	if abs, err := filepath.Abs(root); err == nil {
		root = abs
	}
	sum := sha256.Sum256([]byte(filepath.Clean(root)))
	return filepath.Join(cacheDir, "bin", hex.EncodeToString(sum[:6]), name)
}

// isStale reports whether cached is missing or older than the module's go.sum
// or the running warden binary (a dev rebuild).
func isStale(cached, root string) bool {
	info, err := os.Stat(cached)
	if err != nil {
		return true
	}
	if root != "" {
		gosum := filepath.Join(root, "go.sum")
		if s, err := os.Stat(gosum); err == nil && s.ModTime().After(info.ModTime()) {
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

// moduleDecl is the line of go.mod that declares this module.
const moduleDecl = "module github.com/buildwarden/buildwarden"

// testModuleRoot is the tree SetModuleRootForTests names; empty outside
// tests.
var testModuleRoot string

// SetModuleRootForTests makes ModuleRoot return root. It is for test
// binaries only, and is the only way other than the executable's location
// that the tree is named: a go test binary is built in a temporary directory
// outside any tree, so a package whose tests reach ModuleRoot (through
// GuestBinary) calls it once, from its TestMain, with the tree found from
// the test's own directory. There is no environment variable for it.
func SetModuleRootForTests(root string) { testModuleRoot = root }

// ModuleRoot finds the buildwarden source tree that warden itself is in: the
// first directory, walking up from the warden executable's, whose go.mod
// holds the line "module github.com/buildwarden/buildwarden" whole. The
// working directory is not looked at: a repository being built that declares
// the module is not warden's source, and go build on it would run its code
// on the host. A nested module of the tree (tools/tier1) or a module whose
// path only starts the same does not match.
func ModuleRoot() (string, error) {
	if testModuleRoot != "" {
		return testModuleRoot, nil
	}
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("finding the warden executable: %w", err)
	}
	if root, ok := moduleRootAbove(filepath.Dir(exe)); ok {
		return root, nil
	}
	return "", Fixable("run 'make build' in warden's source tree and use its "+
		"dist/warden", "%s (version %s) is not inside a buildwarden source tree",
		exe, WardenVersion)
}

// moduleRootAbove walks up from dir to the first directory whose go.mod
// declares this module.
func moduleRootAbove(dir string) (string, bool) {
	for {
		if declaresModule(filepath.Join(dir, "go.mod")) {
			return dir, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}

// declaresModule reports whether the go.mod at path has the module line of
// this module, matched whole.
func declaresModule(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) == moduleDecl {
			return true
		}
	}
	return false
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

// NewWorkDir creates a fresh, private work directory for one build, like
// `mktemp -d`: <req.TempDir or the OS temp dir>/warden-<name>-XXXXXXXX, mode
// 0700. Every build gets its own, so concurrent builds never share staging
// state, and nothing is ever written into the build context.
func NewWorkDir(req *BuildRequest, name string) (string, error) {
	if req.TempDir != "" {
		if err := os.MkdirAll(req.TempDir, 0o755); err != nil {
			return "", fmt.Errorf("creating temp dir %s: %w", req.TempDir, err)
		}
	}
	dir, err := os.MkdirTemp(req.TempDir, "warden-"+name+"-")
	if err != nil {
		return "", fmt.Errorf("creating work dir: %w", err)
	}
	return dir, nil
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
