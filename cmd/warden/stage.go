package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/buildwarden/buildwarden/driver"
	"github.com/buildwarden/buildwarden/driver/script"
)

// containerfileIgnored reports whether driver d's guest cannot run a
// Containerfile, so that warden drops it before staging: nothing is
// translated or recorded (describe's capabilities.containerfile: ignore).
func containerfileIgnored(d string) bool { return d == "vz" }

// baseImage is the FROM image of the build's Containerfile, as written there,
// whether or not the script is translated from it (with --script, or a
// build.sh named by path, the image is still the Containerfile's). It is
// empty when there is no Containerfile, when it cannot be parsed, or when its
// FROM is scratch: a parse error is the translation's to report, when there
// is one.
func baseImage(req *driver.BuildRequest) string {
	if req.Containerfile == "" {
		return ""
	}
	res, err := script.Translate(req.Containerfile, req.ContextDir)
	if err != nil || res.Image == "scratch" {
		return ""
	}
	return res.Image
}

// stageBuild stages the build script for driver d in the stage directory of
// the build id, <tmpdir>/warden-stage-<id>/, which is warden's, and records
// an altered input in the output directory (ORCH-02), before any driver work.
// It sets the request's BuildScript, the staged script's absolute path, and
// BaseImage. The returned function removes the stage directory.
func stageBuild(req *driver.BuildRequest, d, id string) (func(), error) {
	if containerfileIgnored(d) {
		req.Containerfile = ""
	}
	req.BaseImage = baseImage(req)
	if err := os.MkdirAll(req.OutputDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating output dir: %w", err)
	}
	dir, err := makeStageDir(req.TempDir, id)
	if err != nil {
		return nil, err
	}
	remove := func() { _ = os.RemoveAll(dir) }
	staged, err := driver.StageBuildScript(req, dir)
	if err != nil {
		remove()
		return nil, fmt.Errorf("staging build script: %w", err)
	}
	if err := driver.RecordInputs(req.OutputDir, req, staged); err != nil {
		remove()
		return nil, fmt.Errorf("recording inputs: %w", err)
	}
	req.BuildScript = staged
	return remove, nil
}

// stageDirName is the base name of the stage directory of the build id.
func stageDirName(id string) string { return "warden-stage-" + id }

// stageDir is the stage directory of the build id under tmpdir, the system's
// temporary directory when tmpdir is empty.
func stageDir(tmpdir, id string) string {
	if tmpdir == "" {
		tmpdir = os.TempDir()
	}
	return filepath.Join(tmpdir, stageDirName(id))
}

// makeStageDir creates the stage directory of the build id under tmpdir.
func makeStageDir(tmpdir, id string) (string, error) {
	if tmpdir != "" {
		if err := os.MkdirAll(tmpdir, 0o755); err != nil {
			return "", fmt.Errorf("creating temp dir %s: %w", tmpdir, err)
		}
	}
	dir := stageDir(tmpdir, id)
	if err := os.Mkdir(dir, 0o700); err != nil {
		return "", fmt.Errorf("creating stage dir: %w", err)
	}
	return dir, nil
}
