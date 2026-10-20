package collector

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

// Every host-disk access goes through an os.Root: outputs land under
// OutputDir (c.out) and context reads stay inside ContextDir (c.context).
// The names come from the build (artifact names, capture names, context
// paths). They are validated before use (paths.go), but only a root also
// refuses a symlink that leads out of its tree. Without that, a build context
// holding `key -> ~/.ssh/id_ed25519` would hand that file to the guest, which
// `docker build` never does.

// localPath converts a validated slash path to a path local to a root,
// refusing anything that could name a location outside it.
func localPath(slashPath string) (string, error) {
	p := filepath.FromSlash(slashPath)
	if !filepath.IsLocal(p) {
		return "", fmt.Errorf("collector: path %q is not local", slashPath)
	}
	return p, nil
}

// createOutput creates (or truncates) rel under the output dir, with any
// missing parent directories.
func (c *Collector) createOutput(rel string) (*os.File, error) {
	if !filepath.IsLocal(rel) {
		return nil, fmt.Errorf("collector: output path %q is not local", rel)
	}
	if dir := filepath.Dir(rel); dir != "." {
		if err := c.out.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
	}
	return c.out.Create(rel)
}

// createOutputTemp creates a new, uniquely named file in the output
// subdirectory dir, for landing a payload before it is renamed into place.
func (c *Collector) createOutputTemp(dir, prefix string) (*os.File, string, error) {
	if err := c.out.MkdirAll(dir, 0o755); err != nil {
		return nil, "", err
	}
	for range 8 {
		b := make([]byte, 8)
		if _, err := rand.Read(b); err != nil {
			return nil, "", err
		}
		rel := filepath.Join(dir, prefix+hex.EncodeToString(b))
		f, err := c.out.OpenFile(rel, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o644)
		if err == nil {
			return f, rel, nil
		}
		if !os.IsExist(err) {
			return nil, "", err
		}
	}
	return nil, "", fmt.Errorf("collector: no free temporary name in %s", dir)
}

// linkCapture points captures/<name> at ../payloads/<hash>. Where symlinks
// are unavailable (unprivileged Windows) it falls back to a copy, which
// `warden inspect` reads as a regular file.
func (c *Collector) linkCapture(name, hash string) error {
	if err := c.out.MkdirAll("captures", 0o755); err != nil {
		return err
	}
	link := filepath.Join("captures", name)
	payload := filepath.Join("payloads", hash)
	_ = c.out.Remove(link)
	if c.out.Symlink(filepath.Join("..", payload), link) == nil {
		return nil
	}
	src, err := c.out.Open(payload)
	if err != nil {
		return err
	}
	defer src.Close()
	dst, err := c.out.Create(link)
	if err != nil {
		return err
	}
	if _, err := io.Copy(dst, src); err != nil {
		dst.Close()
		return err
	}
	return dst.Close()
}

// contextFiles lists the safe file paths under dir (a path local to the
// context root, "." for all of it), skipping symlinks that resolve outside
// the context, since those could never be fetched.
func (c *Collector) contextFiles(dir string) []string {
	var files []string
	_ = fs.WalkDir(c.context.FS(), filepath.ToSlash(dir),
		func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			if d.Type()&fs.ModeSymlink != 0 {
				fi, err := c.context.Stat(filepath.FromSlash(p))
				if err != nil || fi.IsDir() {
					return nil
				}
			}
			if IsSafeContextPath(p) {
				files = append(files, p)
			}
			return nil
		})
	return files
}
