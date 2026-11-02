package firecracker

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/buildwarden/buildwarden/driver"
	"github.com/buildwarden/buildwarden/driver/linuxguest"
	"github.com/buildwarden/buildwarden/ledger"
)

// Build rootfs images (Decision 7): Firecracker needs raw block devices, so a
// cloud image is converted to raw once into <cache>/firecracker/images,
// recorded there as a warden-prepared layer whose input is the cloud image's
// hash, and copied per build.

// ImageDir is where converted raw images are cached.
func ImageDir() string { return filepath.Join(driver.CacheDir(), "firecracker", "images") }

// WorkRoot holds each build's rootfs copy, on the cache's filesystem so the
// copy can be a reflink and a tmpfs temp dir never holds a multi-GB disk.
func WorkRoot() string { return filepath.Join(driver.CacheDir(), "firecracker", "work") }

// LayerCache is the firecracker driver's layer-hash cache, beside its raw
// images.
func LayerCache() *driver.LayerCache {
	return &driver.LayerCache{
		Path:     filepath.Join(ImageDir(), driver.LayerCacheFile),
		Progress: os.Stderr,
	}
}

// qemuImgInfo is the part of `qemu-img info --output=json` the gate reads.
type qemuImgInfo struct {
	Format          string `json:"format"`
	BackingFilename string `json:"backing-filename"`
	FormatSpecific  struct {
		Data struct {
			DataFile string `json:"data-file"`
		} `json:"data"`
	} `json:"format-specific"`
}

// checkImageInfo is the conversion gate: only a qcow2 or raw image that
// refers to no other file may be converted, so a crafted qcow2 cannot pull a
// host file (a backing file or an external data file, say ~/.ssh/id_rsa) into
// the build's rootfs. It returns the format to pass to qemu-img's -f.
func checkImageInfo(image string, infoJSON []byte) (string, error) {
	var info qemuImgInfo
	if err := json.Unmarshal(infoJSON, &info); err != nil {
		return "", fmt.Errorf("parsing qemu-img info for %s: %w", image, err)
	}
	const fix = "use a self-contained qcow2 or raw image: " +
		"qemu-img convert -O qcow2 <image> <flattened.qcow2>, then pass that with --image"
	switch {
	case info.BackingFilename != "":
		return "", driver.Fixable(fix,
			"%s has a backing file (%s); refusing to convert it",
			image, info.BackingFilename)
	case info.FormatSpecific.Data.DataFile != "":
		return "", driver.Fixable(fix,
			"%s has an external data file (%s); refusing to convert it",
			image, info.FormatSpecific.Data.DataFile)
	case info.Format != "qcow2" && info.Format != "raw":
		return "", driver.Fixable(fix,
			"%s is a %q image; only qcow2 and raw are converted",
			image, info.Format)
	}
	return info.Format, nil
}

// rawName is the cache file for a source image: its name and the start of its
// hash, so a changed source converts to a new file and two sources with the
// same name never collide.
func rawName(source, sha256Hex string) string {
	stem := strings.TrimSuffix(filepath.Base(source), filepath.Ext(source))
	stem = strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9',
			r == '-', r == '_':
			return r
		}
		return '_'
	}, stem)
	return stem + "-" + sha256Hex[:12] + ".raw"
}

// rawSource is the provenance of a raw image converted from a source whose
// hash is sourceSHA.
func rawSource(sourceSHA string) ledger.LayerSource {
	return ledger.LayerSource{
		Kind: ledger.SourcePrepared, PreparedBy: driver.PreparedBy(),
		Inputs: []ledger.PreparationInput{{Name: "cloud-image", SHA256: sourceSHA}},
	}
}

// sourceHash is a source image's sha256, from the cloud image cache for a
// downloaded image (where its download is recorded) and from this driver's
// cache for any other path.
func sourceHash(source string) (string, error) {
	cache := LayerCache()
	if strings.HasPrefix(source, linuxguest.ImageDir()+string(filepath.Separator)) {
		cache = linuxguest.LayerCache()
	}
	info, err := cache.Lookup(source, "")
	if err != nil {
		return "", err
	}
	return info.SHA256, nil
}

// EnsureRaw returns the cached raw image for source, converting it first if
// it has not been: `qemu-img info` gates it, `qemu-img convert -f <format>`
// writes it, and it is recorded as a prepared layer. Concurrent builds of one
// source convert it once.
func EnsureRaw(source string) (string, error) {
	source, err := filepath.Abs(source)
	if err != nil {
		return "", err
	}
	sum, err := sourceHash(source)
	if err != nil {
		return "", fmt.Errorf("hashing %s: %w", source, err)
	}
	raw := filepath.Join(ImageDir(), rawName(source, sum))
	if converted(raw, sum) {
		return raw, nil
	}
	unlock, err := driver.HostLock(convertLockName(sum[:12]))
	if err != nil {
		return "", err
	}
	defer unlock()
	if converted(raw, sum) {
		return raw, nil
	}
	return raw, convert(source, raw, sum)
}

// converted reports whether raw exists and is recorded as made from sum.
func converted(raw, sum string) bool {
	if _, err := os.Stat(raw); err != nil {
		return false
	}
	src, ok := LayerCache().Source(raw)
	return ok && src.Kind == ledger.SourcePrepared && len(src.Inputs) == 1 &&
		src.Inputs[0].SHA256 == sum
}

func convert(source, raw, sum string) error {
	if _, err := exec.LookPath("qemu-img"); err != nil {
		return driver.Fixable(qemuImgFix,
			"converting %s to raw needs qemu-img, which is not on PATH", source)
	}
	info, err := exec.Command("qemu-img", "info", "--output=json", source).Output()
	if err != nil {
		return fmt.Errorf("qemu-img info %s: %w", source, err)
	}
	format, err := checkImageInfo(source, info)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(ImageDir(), 0o755); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "[warden] converting %s to raw (once per image)...\n",
		filepath.Base(source))
	tmp := raw + ".tmp"
	out, err := exec.Command("qemu-img", "convert", "-f", format, "-O", "raw", source, tmp).
		CombinedOutput()
	if err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("qemu-img convert %s: %s: %w", source,
			strings.TrimSpace(string(out)), err)
	}
	rawSum, err := driver.HashFileSHA256(tmp)
	if err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("hashing %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, raw); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return LayerCache().Record(raw, rawSum, "", rawSource(sum))
}

const qemuImgFix = "install it: apt install qemu-utils (or your distro's equivalent)"

// ListImages returns the cached raw images.
func ListImages() []string {
	raws, _ := filepath.Glob(filepath.Join(ImageDir(), "*.raw"))
	return raws
}
