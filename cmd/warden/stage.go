package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/buildwarden/buildwarden/driver"
	"github.com/buildwarden/buildwarden/driver/script"
)

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

// stageBuild creates the output directory and writes dj there as
// driver.json, then stages the build script for a driver with caps in the
// stage directory of the build id, <tmpdir>/warden-stage-<id>/, which is
// warden's, and records an altered input in the output directory (ORCH-02),
// before any driver work. A driver whose guest cannot run a Containerfile
// (capabilities.containerfile: ignore) has it dropped first, so nothing is
// translated or recorded. It sets the request's BuildScript, the staged
// script's absolute path, and BaseImage. The returned function removes the
// stage directory.
func stageBuild(req *driver.BuildRequest, caps capabilities, id string,
	dj *driverJSON) (func(), error) {
	if caps.Containerfile == containerfileIgnore {
		req.Containerfile = ""
	}
	req.BaseImage = baseImage(req)
	if err := os.MkdirAll(req.OutputDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating output dir: %w", err)
	}
	if dj != nil {
		if err := dj.write(req.OutputDir); err != nil {
			return nil, fmt.Errorf("writing driver.json: %w", err)
		}
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
