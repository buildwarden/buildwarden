package qemu

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// resolveImage maps a FROM image reference to a local QCOW2 path.
// Downloads cloud images on first use and caches them.
func resolveImage(imageRef string) (string, error) {
	if imageRef == "" || imageRef == "scratch" {
		return "", nil
	}

	// Already a local path
	if strings.HasPrefix(imageRef, "/") || strings.HasPrefix(imageRef, "./") {
		if _, err := os.Stat(imageRef); err != nil {
			return "", fmt.Errorf("image not found: %s", imageRef)
		}
		return imageRef, nil
	}

	url := cloudImageURL(imageRef)
	if url == "" {
		return "", fmt.Errorf(
			"cannot resolve %q to a cloud image; "+
				"provide a local path with --image", imageRef)
	}

	cached := cachedImagePath(imageRef)
	if _, err := os.Stat(cached); err == nil {
		if verifyImageIntegrity(cached) {
			return cached, nil
		}
		fmt.Fprintf(os.Stderr,
			"[warden] cached image failed integrity check, re-downloading\n")
		_ = os.Remove(cached)
		_ = os.Remove(cached + ".sha256")
	}

	if err := downloadImage(url, cached); err != nil {
		return "", fmt.Errorf("downloading %s: %w", imageRef, err)
	}
	return cached, nil
}

func cloudImageURL(ref string) string {
	arch := runtime.GOARCH
	qemuArch := hostQEMUArch()

	name, tag := splitImageRef(ref)

	switch {
	case name == "alpine" || strings.HasPrefix(name, "alpine:"):
		ver := tag
		if ver == "" || ver == "latest" {
			ver = "3.21"
		}
		return fmt.Sprintf(
			"https://dl-cdn.alpinelinux.org/alpine/v%s/releases/cloud/"+
				"generic_alpine-%s.0-%s-uefi-cloudinit-r0.qcow2",
			ver, ver, qemuArch)

	case name == "ubuntu" || strings.HasPrefix(name, "ubuntu:"):
		ver := tag
		if ver == "" || ver == "latest" {
			ver = "24.04"
		}
		series := ubuntuSeries(ver)
		return fmt.Sprintf(
			"https://cloud-images.ubuntu.com/%s/current/"+
				"%s-server-cloudimg-%s.img",
			series, series, ubuntuArch(arch))

	case name == "debian" || strings.HasPrefix(name, "debian:"):
		ver := tag
		if ver == "" || ver == "latest" {
			ver = "bookworm"
		}
		return fmt.Sprintf(
			"https://cloud.debian.org/images/cloud/%s/latest/"+
				"debian-12-generic-%s.qcow2",
			ver, debianArch(arch))
	}

	return ""
}

func splitImageRef(ref string) (name, tag string) {
	parts := strings.SplitN(ref, ":", 2)
	name = parts[0]
	if len(parts) > 1 {
		tag = parts[1]
	}
	return
}

func ubuntuSeries(ver string) string {
	switch ver {
	case "24.04":
		return "noble"
	case "22.04":
		return "jammy"
	case "20.04":
		return "focal"
	default:
		return ver
	}
}

func ubuntuArch(goarch string) string {
	switch goarch {
	case "arm64":
		return "arm64"
	case "amd64":
		return "amd64"
	default:
		return goarch
	}
}

func debianArch(goarch string) string {
	switch goarch {
	case "arm64":
		return "arm64"
	case "amd64":
		return "amd64"
	default:
		return goarch
	}
}

func cachedImagePath(ref string) string {
	safe := strings.NewReplacer(
		"/", "_", ":", "_", ".", "_").Replace(ref)
	dir := filepath.Join(cacheBaseDir(), "warden", "images")
	_ = os.MkdirAll(dir, 0755)
	return filepath.Join(dir, safe+".qcow2")
}

func downloadImage(url, dest string) error {
	fmt.Fprintf(os.Stderr, "[warden] downloading image...\n")
	tmp := dest + ".tmp"
	cmd := exec.Command("curl", "-fSL", "--progress-bar", "-o", tmp, url)
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		_ = os.Remove(tmp)
		return err
	}

	hash, err := hashFile(tmp)
	if err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("hashing image: %w", err)
	}
	if err := os.WriteFile(dest+".sha256", []byte(hash+"\n"), 0644); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("writing hash: %w", err)
	}

	if err := os.Rename(tmp, dest); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "[warden] image cached (sha256:%s)\n", hash[:12])
	return nil
}

func hashFile(path string) (string, error) {
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

func verifyImageIntegrity(path string) bool {
	expected, err := os.ReadFile(path + ".sha256")
	if err != nil {
		return true // no hash file = skip verification
	}
	actual, err := hashFile(path)
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(expected)) == actual
}
