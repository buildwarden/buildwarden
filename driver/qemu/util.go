package qemu

import (
	"io"
	"os"
	"path/filepath"
	"runtime"
)

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}

	info, err := in.Stat()
	if err == nil {
		_ = out.Chmod(info.Mode())
	}
	return out.Close()
}

func cachedBinaryPath(name, goarch string) string {
	cacheDir := filepath.Join(cacheBaseDir(), "warden", "bin")
	_ = os.MkdirAll(cacheDir, 0755)
	return filepath.Join(cacheDir, name+"-linux-"+goarch)
}

// isCacheStale returns true if the cached binary is older than the
// newest source file in the module. Compares against go.sum mtime
// as a lightweight proxy for "source changed."
func isCacheStale(cached string) bool {
	info, err := os.Stat(cached)
	if err != nil {
		return true
	}

	root := findModuleRoot()
	gosum := filepath.Join(root, "go.sum")
	sumInfo, err := os.Stat(gosum)
	if err != nil {
		return false
	}
	if sumInfo.ModTime().After(info.ModTime()) {
		return true
	}

	// Also check if the warden binary itself is newer (dev rebuild)
	if exe, err := os.Executable(); err == nil {
		if exeInfo, err := os.Stat(exe); err == nil {
			if exeInfo.ModTime().After(info.ModTime()) {
				return true
			}
		}
	}
	return false
}

func cacheBaseDir() string {
	if dir, err := os.UserCacheDir(); err == nil {
		return dir
	}
	if runtime.GOOS == "linux" {
		if home := os.Getenv("HOME"); home != "" {
			return filepath.Join(home, ".cache")
		}
	}
	return os.TempDir()
}

func findModuleRoot() string {
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		for {
			if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
				return dir
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}

	dir, _ := os.Getwd()
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "."
}
