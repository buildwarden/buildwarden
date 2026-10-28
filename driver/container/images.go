package container

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/lesiw/ctrctl"

	"github.com/buildwarden/buildwarden/driver"
)

// The holder and the iptables sidecar are trusted code: the holder runs before
// the build's network is isolated, and the sidecar runs --privileged. A release
// build sets these (-ldflags -X) to the published images by digest
// (ghcr.io/buildwarden/holder@sha256:...), and warden runs exactly those. When
// unset (a development build), warden builds them from the embedded
// Containerfiles and tags them by content hash, never by a reusable tag.
var (
	holderImageRef string
	netnsImageRef  string
)

//go:embed images/holder.Containerfile
var holderContainerfile []byte

//go:embed images/netns.Containerfile
var netnsContainerfile []byte

// resolveSupportImages sets the holder and sidecar images for this build.
func (b *build) resolveSupportImages() error {
	var err error
	if b.holderImage, err = supportImage(holderImageRef, b.buildHolderImage); err != nil {
		return fmt.Errorf("holder image: %w", err)
	}
	if b.netnsImage, err = supportImage(netnsImageRef, b.buildNetnsImage); err != nil {
		return fmt.Errorf("network isolation image: %w", err)
	}
	return nil
}

func supportImage(ref string, build func() (string, error)) (string, error) {
	if ref == "" {
		return build()
	}
	if imageExists(ref) {
		return ref, nil
	}
	args := append(ctrctl.Cli, "pull", ref)
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("pulling %s: %w", ref, err)
	}
	return ref, nil
}

func imageExists(ref string) bool {
	args := append(ctrctl.Cli, "image", "inspect", ref)
	return exec.Command(args[0], args[1:]...).Run() == nil
}

// buildHolderImage cross-compiles cmd/warden-pause and builds the holder image
// from it, tagged by the hash of the Containerfile and the binary.
func (b *build) buildHolderImage() (string, error) {
	dir := filepath.Join(b.workDir, "holder-image")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	bin := filepath.Join(dir, "warden-pause-"+runtime.GOARCH)
	if err := goBuildLinux("./cmd/warden-pause", bin); err != nil {
		return "", err
	}
	data, err := os.ReadFile(bin)
	if err != nil {
		return "", err
	}
	tag := "warden-holder:" + contentHash(holderContainerfile, data)
	return tag, buildSupportImage(tag, dir, holderContainerfile, "TARGETARCH="+runtime.GOARCH)
}

func (b *build) buildNetnsImage() (string, error) {
	dir := filepath.Join(b.workDir, "netns-image")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	tag := "warden-netns:" + contentHash(netnsContainerfile)
	return tag, buildSupportImage(tag, dir, netnsContainerfile, "")
}

func buildSupportImage(tag, dir string, containerfile []byte, buildArg string) error {
	if imageExists(tag) {
		return nil
	}
	file := filepath.Join(dir, "Containerfile")
	if err := os.WriteFile(file, containerfile, 0o644); err != nil {
		return err
	}
	opts := &ctrctl.ImageBuildOpts{Tag: tag, File: file, BuildArg: buildArg}
	_, err := ctrctl.ImageBuild(opts, dir, "")
	return err
}

func contentHash(parts ...[]byte) string {
	h := sha256.New()
	for _, p := range parts {
		h.Write(p)
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// goBuildLinux cross-compiles a package of this module for Linux on the host's
// architecture (development builds).
func goBuildLinux(pkg, out string) error {
	root, err := driver.ModuleRoot()
	if err != nil {
		return fmt.Errorf("building %s from source: %w", pkg, err)
	}
	cmd := exec.Command("go", "build", "-o", out, pkg)
	cmd.Env = append(os.Environ(), "GOOS=linux", "CGO_ENABLED=0")
	cmd.Dir = root
	cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("cross-compiling %s: %w", pkg, err)
	}
	return nil
}
