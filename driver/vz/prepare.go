package vz

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/buildwarden/buildwarden/driver"
)

// srcKind is what an `image prepare --driver vz` source names.
type srcKind int

const (
	srcEmpty srcKind = iota // no source
	srcURL                  // an IPSW URL (file:// included)
	srcFile                 // a local IPSW file, given as a plain path
	srcImage                // a prepared image directory, or its disk.img
)

// prepSource is a classified source. path is the URL, the absolute IPSW
// path, or the image directory (inside the cache: CacheDir/<name>).
type prepSource struct {
	kind    srcKind
	path    string
	outside bool // a prepared image that is not a directory of the cache
}

// classify decides what source names: nothing, a URL (it has a scheme), a
// prepared image (a valid image directory or the disk.img inside one) or a
// local IPSW file (any other regular file). A plain path that names none of
// these is refused naming the path.
func classify(source, cacheDir string) (prepSource, error) {
	if source == "" {
		return prepSource{kind: srcEmpty}, nil
	}
	if strings.Contains(source, "://") {
		return prepSource{kind: srcURL, path: source}, nil
	}
	refuse := driver.Fixable(
		"give an IPSW URL, a local .ipsw file, or a prepared image directory",
		"no IPSW file or prepared image at %s", source)
	abs, err := filepath.Abs(source)
	if err != nil {
		return prepSource{}, refuse
	}
	fi, err := os.Stat(abs)
	if err != nil {
		return prepSource{}, refuse
	}
	dir := abs
	if !fi.IsDir() {
		if filepath.Base(abs) != "disk.img" || !isValidImageDir(filepath.Dir(abs)) {
			if !fi.Mode().IsRegular() {
				return prepSource{}, refuse
			}
			return prepSource{kind: srcFile, path: abs}, nil
		}
		dir = filepath.Dir(abs)
	}
	if !isValidImageDir(dir) {
		return prepSource{}, refuse
	}
	if sameDir(filepath.Dir(dir), cacheDir) {
		return prepSource{kind: srcImage, path: filepath.Join(cacheDir, filepath.Base(dir))}, nil
	}
	return prepSource{kind: srcImage, path: dir, outside: true}, nil
}

// sameDir reports whether a and b name the same directory, through symlinks
// (a cache under /var is /private/var on macOS).
func sameDir(a, b string) bool {
	if filepath.Clean(a) == filepath.Clean(b) {
		return true
	}
	ia, err1 := os.Stat(a)
	ib, err2 := os.Stat(b)
	return err1 == nil && err2 == nil && os.SameFile(ia, ib)
}

// Prepare is `warden image prepare --driver vz [source | --update]`, after
// the platform and root checks: it restores or refreshes one image, makes it
// the active image and returns its directory.
//
//   - no source: refresh the active image in place, or restore the latest
//     supported IPSW when no image is prepared;
//   - an IPSW URL or file: refresh its image in place when it is prepared,
//     else restore it;
//   - a prepared image in the cache: refresh it in place;
//   - update (no source): restore the latest supported IPSW, or refresh its
//     image in place when it is prepared.
func (c *ImageCache) Prepare(source string, update bool) (string, error) {
	if update && source != "" {
		return "", driver.Fixable("drop the source, or drop --update to prepare that source",
			"--update picks the newest IPSW itself")
	}
	src, err := classify(source, c.CacheDir)
	if err != nil {
		return "", err
	}
	var dir string
	switch {
	case update:
		u, lerr := c.latestIPSW()
		if lerr != nil {
			return "", fmt.Errorf("fetching latest IPSW URL: %w", lerr)
		}
		fmt.Fprintf(c.out(), "Latest macOS IPSW: %s\n", u)
		dir, err = c.restoreOrRefresh(u)
	case src.kind == srcEmpty:
		active, _ := c.ImageDir()
		if active != "" {
			dir, err = active, c.refreshImage(active)
			break
		}
		u, lerr := c.latestIPSW()
		if lerr != nil {
			return "", fmt.Errorf("fetching latest IPSW URL: %w", lerr)
		}
		fmt.Fprintf(c.out(), "Latest macOS IPSW: %s\n", u)
		dir, err = c.restoreOrRefresh(u)
	case src.kind == srcURL, src.kind == srcFile:
		dir, err = c.restoreOrRefresh(src.path)
	case src.kind == srcImage:
		if src.outside {
			return "", driver.Fixable(customImageFix(c.CacheDir),
				"%s is a prepared image outside the image cache %s", src.path, c.CacheDir)
		}
		dir, err = src.path, c.refreshImage(src.path)
	}
	if err != nil {
		return "", err
	}
	if err := c.SetActive(dir); err != nil {
		return "", err
	}
	// The image's provenance (IPSW URL, digest) is recorded, and a later
	// prepare of the same source refreshes this image in place, so its
	// downloaded IPSW (about 25 GB) is never read again. A local IPSW file
	// is the user's and is never in the cache under this name.
	key := strings.TrimPrefix(filepath.Base(dir), "macos-")
	if err := os.Remove(filepath.Join(c.CacheDir, "ipsw-"+key+".ipsw")); err != nil &&
		!errors.Is(err, os.ErrNotExist) {
		fmt.Fprintf(c.out(), "warning: could not delete the image's cached IPSW: %v\n", err)
	}
	return dir, nil
}

// restoreOrRefresh refreshes in place the image an IPSW source keys to when
// it is prepared, wherever it sorts in the cache, and restores it otherwise.
func (c *ImageCache) restoreOrRefresh(source string) (string, error) {
	dir := filepath.Join(c.CacheDir, "macos-"+shortHash(source))
	if isValidImageDir(dir) {
		return dir, c.refreshImage(dir)
	}
	return c.restore(source)
}

func (c *ImageCache) refreshImage(dir string) error {
	fmt.Fprintf(c.out(), "Refreshing boot agent on %s\n", dir)
	return c.refreshStep(dir)
}
