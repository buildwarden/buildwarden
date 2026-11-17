package main

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/buildwarden/buildwarden/driver"
)

// The script is staged in warden's own stage directory under the build's
// tmpdir, named for the build, and the returned function removes it; the
// request carries the staged path and the Containerfile's base image, the
// latter with --script too (ORCH-02, BOOT-04).
func TestStageBuildDirectory(t *testing.T) {
	ctx, tmp := t.TempDir(), t.TempDir()
	df := write(t, ctx, "Dockerfile", "FROM alpine:3.21\nRUN echo hi\n")
	own := write(t, ctx, "own.sh", "echo own\n")
	req := &driver.BuildRequest{ContextDir: ctx, Containerfile: df, Script: own,
		OutputDir: filepath.Join(t.TempDir(), "out"), TempDir: tmp}
	unstage, err := stageBuild(req, "qemu")
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Dir(req.BuildScript)
	if filepath.Dir(dir) != tmp ||
		!regexp.MustCompile(`^warden-stage-[a-z0-9]{8}$`).MatchString(filepath.Base(dir)) {
		t.Errorf("staged at %s, want %s/warden-stage-<id>/", req.BuildScript, tmp)
	}
	if got, _ := os.ReadFile(req.BuildScript); string(got) != "echo own\n" {
		t.Errorf("staged script = %q, want the --script's", got)
	}
	if req.BaseImage != "alpine:3.21" {
		t.Errorf("BaseImage = %q, want the FROM image with --script", req.BaseImage)
	}
	if _, err := os.Stat(filepath.Join(req.OutputDir, "inputs")); !os.IsNotExist(err) {
		t.Errorf("a script run as given was recorded: %v", err)
	}
	unstage()
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("stage directory left behind: %v", err)
	}
}

// The base image is empty for FROM scratch and for a Containerfile that does
// not parse, and vz is sent no Containerfile and no base image.
func TestStageBuildBaseImage(t *testing.T) {
	for _, tc := range []struct {
		name, driver, from string
		want               string
	}{
		{"scratch", "qemu", "FROM scratch\n", ""},
		{"unparsable", "qemu", "RUN true\n", ""},
		{"vz drops it", "vz", "FROM alpine\n", ""},
		{"translated", "firecracker", "FROM debian:12\n", "debian:12"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := t.TempDir()
			req := &driver.BuildRequest{ContextDir: ctx,
				Containerfile: write(t, ctx, "Dockerfile", tc.from),
				Script:        write(t, ctx, "own.sh", "true\n"),
				OutputDir:     filepath.Join(t.TempDir(), "out"),
				TempDir:       t.TempDir()}
			unstage, err := stageBuild(req, tc.driver)
			if err != nil {
				t.Fatal(err)
			}
			defer unstage()
			if req.BaseImage != tc.want {
				t.Errorf("BaseImage = %q, want %q", req.BaseImage, tc.want)
			}
			if tc.driver == "vz" && req.Containerfile != "" {
				t.Errorf("vz was sent the Containerfile %s", req.Containerfile)
			}
		})
	}
}
