package driver

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"github.com/buildwarden/buildwarden/ledger"
)

// LayerCache remembers the SHA-256 and provenance of VM base-image files, so a
// build describes multi-GB disks without re-hashing them. Fetch and prepare
// commands record the hash they already computed (with where the file came
// from); a build reuses an entry while the file's size and mtime, and its
// parent layer's hash, are unchanged. Anything else is re-hashed, and a file
// that changed since it was recorded loses its provenance (it is no longer the
// file that was fetched).
//
// The cache is a single JSON file in a driver's image cache directory. Writes
// are atomic (temp file + rename); concurrent builds may race, in which case
// the loser's entry is simply re-hashed on a later build.
type LayerCache struct {
	// Path is the cache file, e.g. <images dir>/layers.json.
	Path string
	// Progress, if set, is told when a layer has to be hashed in full.
	Progress io.Writer

	mu sync.Mutex
}

// LayerCacheFile is the cache's file name inside a driver's image directory.
const LayerCacheFile = "layers.json"

type layerCacheEntry struct {
	SHA256       string             `json:"sha256"`
	Size         int64              `json:"size"`
	ModTimeNanos int64              `json:"mtime_ns"`
	ParentSHA256 string             `json:"parent_sha256,omitempty"`
	Source       ledger.LayerSource `json:"source"`
}

type layerCacheFile struct {
	Layers map[string]layerCacheEntry `json:"layers"`
}

// LayerInfo is a hashed layer file and its provenance.
type LayerInfo struct {
	Path   string
	SHA256 string
	Size   int64
	Source ledger.LayerSource
}

func layerKey(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return filepath.Clean(abs), nil
}

func (c *LayerCache) load() (*layerCacheFile, error) {
	f := &layerCacheFile{Layers: map[string]layerCacheEntry{}}
	b, err := os.ReadFile(c.Path)
	if os.IsNotExist(err) {
		return f, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, f); err != nil {
		// A corrupt cache only costs a re-hash; start over rather than fail.
		return &layerCacheFile{Layers: map[string]layerCacheEntry{}}, nil
	}
	if f.Layers == nil {
		f.Layers = map[string]layerCacheEntry{}
	}
	return f, nil
}

func (c *LayerCache) save(f *layerCacheFile) error {
	if err := os.MkdirAll(filepath.Dir(c.Path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(c.Path), ".layers-*.tmp")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(append(b, '\n')); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), c.Path)
}

// Record stores a hash the caller already computed (on fetch or prepare)
// together with the layer's provenance. parentSHA256 is the hash of the layer
// this one is a child of, or "".
func (c *LayerCache) Record(path, sha256Hex, parentSHA256 string, src ledger.LayerSource) error {
	key, err := layerKey(path)
	if err != nil {
		return err
	}
	fi, err := os.Stat(key)
	if err != nil {
		return fmt.Errorf("layer cache: %w", err)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	f, err := c.load()
	if err != nil {
		return fmt.Errorf("layer cache: %w", err)
	}
	f.Layers[key] = layerCacheEntry{
		SHA256: sha256Hex, Size: fi.Size(), ModTimeNanos: fi.ModTime().UnixNano(),
		ParentSHA256: parentSHA256, Source: src,
	}
	if err := c.save(f); err != nil {
		return fmt.Errorf("layer cache: %w", err)
	}
	return nil
}

// Lookup returns the layer at path, from the cache when its entry is still
// valid, otherwise by hashing it (and caching the result). A layer hashed here
// has no recorded provenance and is reported as a local file.
func (c *LayerCache) Lookup(path, parentSHA256 string) (LayerInfo, error) {
	key, err := layerKey(path)
	if err != nil {
		return LayerInfo{}, err
	}
	fi, err := os.Stat(key)
	if err != nil {
		return LayerInfo{}, fmt.Errorf("layer %s: %w", path, err)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	f, err := c.load()
	if err != nil {
		return LayerInfo{}, fmt.Errorf("layer cache: %w", err)
	}
	if e, ok := f.Layers[key]; ok && e.Size == fi.Size() &&
		e.ModTimeNanos == fi.ModTime().UnixNano() && e.ParentSHA256 == parentSHA256 {
		return LayerInfo{Path: key, SHA256: e.SHA256, Size: e.Size, Source: e.Source}, nil
	}
	if c.Progress != nil {
		fmt.Fprintf(c.Progress, "[warden] hashing %s (%s, first use or changed)...\n",
			filepath.Base(key), humanSize(fi.Size()))
	}
	sum, err := HashFileSHA256(key)
	if err != nil {
		return LayerInfo{}, fmt.Errorf("hashing layer %s: %w", path, err)
	}
	e := layerCacheEntry{
		SHA256: sum, Size: fi.Size(), ModTimeNanos: fi.ModTime().UnixNano(),
		ParentSHA256: parentSHA256, Source: ledger.LayerSource{Kind: ledger.SourceLocal},
	}
	f.Layers[key] = e
	if err := c.save(f); err != nil && c.Progress != nil {
		fmt.Fprintf(c.Progress, "[warden] warning: layer cache not saved: %v\n", err)
	}
	return LayerInfo{Path: key, SHA256: e.SHA256, Size: e.Size, Source: e.Source}, nil
}

// LayerVerifyResult is one entry checked by Verify.
type LayerVerifyResult struct {
	Path   string
	Status string // "ok", "changed", "missing"
	Was    string // cached sha256
	Now    string // recomputed sha256 ("" when missing)
}

// Verify re-hashes every cached layer in full. Changed files are re-recorded
// as local (provenance dropped) and missing files are removed from the cache.
func (c *LayerCache) Verify() ([]LayerVerifyResult, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	f, err := c.load()
	if err != nil {
		return nil, fmt.Errorf("layer cache: %w", err)
	}
	keys := make([]string, 0, len(f.Layers))
	for k := range f.Layers {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	results := make([]LayerVerifyResult, 0, len(keys))
	for _, k := range keys {
		e := f.Layers[k]
		r := LayerVerifyResult{Path: k, Was: e.SHA256}
		fi, statErr := os.Stat(k)
		if statErr != nil {
			r.Status = "missing"
			delete(f.Layers, k)
			results = append(results, r)
			continue
		}
		if c.Progress != nil {
			fmt.Fprintf(c.Progress, "[warden] hashing %s (%s)...\n",
				filepath.Base(k), humanSize(fi.Size()))
		}
		sum, err := HashFileSHA256(k)
		if err != nil {
			return results, fmt.Errorf("hashing %s: %w", k, err)
		}
		r.Now = sum
		r.Status = "ok"
		if sum != e.SHA256 {
			r.Status = "changed"
			e.SHA256 = sum
			e.Source = ledger.LayerSource{Kind: ledger.SourceLocal}
		}
		e.Size, e.ModTimeNanos = fi.Size(), fi.ModTime().UnixNano()
		f.Layers[k] = e
		results = append(results, r)
	}
	if err := c.save(f); err != nil {
		return results, fmt.Errorf("layer cache: %w", err)
	}
	return results, nil
}

// HashFileSHA256 returns the lowercase hex SHA-256 of a file.
func HashFileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func humanSize(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}

// Source returns the provenance recorded for path, whether or not the file
// still matches its entry, so a re-prepare can carry it forward.
func (c *LayerCache) Source(path string) (ledger.LayerSource, bool) {
	key, err := layerKey(path)
	if err != nil {
		return ledger.LayerSource{}, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	f, err := c.load()
	if err != nil {
		return ledger.LayerSource{}, false
	}
	e, ok := f.Layers[key]
	return e.Source, ok
}

// Forget drops a layer's cached entry (for a layer that was deleted).
func (c *LayerCache) Forget(path string) error {
	key, err := layerKey(path)
	if err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	f, err := c.load()
	if err != nil {
		return err
	}
	if _, ok := f.Layers[key]; !ok {
		return nil
	}
	delete(f.Layers, key)
	return c.save(f)
}
