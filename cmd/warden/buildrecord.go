package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/buildwarden/buildwarden/driver"
)

// contractVersion is the driver contract version warden speaks
// (specs/driver-contract/v1.md).
const contractVersion = 1

// buildRecord is a build's record, build-<driver>-<id>.json in the lock
// directory, written before anything else of the build is created: what a
// later warden needs to reap the build, and to attach to it, whoever runs
// it and from wherever (DRV-59, DRV-60). It holds no secret: no claim token,
// and no value read from a config file, which it names.
type buildRecord struct {
	Driver string `json:"driver"`
	ID     string `json:"build_id"`
	// Path and SHA256 are the driver binary's; a linked driver is Linked, with
	// warden's own path and no digest.
	Path     string `json:"path"`
	SHA256   string `json:"sha256,omitempty"`
	Linked   bool   `json:"linked"`
	Contract int    `json:"contract"`
	TempDir  string `json:"tmpdir"`
	StageDir string `json:"stage_dir"`
	// ConfigFile and Args are the config_file and args run was started with.
	ConfigFile string   `json:"config_file,omitempty"`
	Args       []string `json:"args"`
}

// buildIDPattern is the form of a build ID (DRV-40).
var buildIDPattern = regexp.MustCompile(`^[a-z0-9]{8}$`)

// liveBuild is warden's hold on one build: the build lock, and beside it the
// build's record and the driver's state file.
type liveBuild struct {
	lock   *driver.BuildLock
	record buildRecord
}

// buildFiles is the path, without its extension, of a build's three files.
func buildFiles(driverName, id string) string {
	return filepath.Join(driver.LockDir(), driver.BuildLockName(driverName, id))
}

// startBuildRecord takes the build's lock and then writes its record, before
// anything else of the build exists (DRV-56). A record that cannot be written
// removes the lock again.
func startBuildRecord(rec buildRecord) (*liveBuild, error) {
	lock, err := driver.TakeBuildLock(rec.Driver, rec.ID)
	if err != nil {
		return nil, err
	}
	if err := writeRecord(buildFiles(rec.Driver, rec.ID)+".json", rec); err != nil {
		_ = lock.Remove()
		return nil, fmt.Errorf("writing the build record: %w", err)
	}
	return &liveBuild{lock: lock, record: rec}, nil
}

// writeRecord writes rec to path through a temporary file, so a reader never
// sees half a record.
func writeRecord(path string, rec buildRecord) error {
	data, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// statePath is the driver's state file of the build (DRV-61).
func (b *liveBuild) statePath() string {
	return buildFiles(b.record.Driver, b.record.ID) + ".state"
}

// end removes the build's state file, record and lock, the build having
// ended cleanly: its driver tore down what it made.
func (b *liveBuild) end() error {
	return removeBuildFiles(buildFiles(b.record.Driver, b.record.ID), b.lock)
}

// keep releases the lock and leaves the three files for warden clean: the
// build's leftovers remain.
func (b *liveBuild) keep() { b.lock.Release() }

// removeBuildFiles removes the state file and the record at base, then the
// lock file, which l holds.
func removeBuildFiles(base string, l *driver.BuildLock) error {
	var errs []error
	for _, p := range []string{base + ".state", base + ".json"} {
		if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		l.Release()
		return errors.Join(errs...)
	}
	return l.Remove()
}

// reapFunc runs the driver's reap for a dead build, with the record's values
// and the state file's path (DRV-60).
type reapFunc func(rec buildRecord, stateFile string) error

// buildLocks lists the lock files of the driver's builds, with each build's
// ID.
func buildLocks(driverName string) (map[string]string, error) {
	pattern := filepath.Join(driver.LockDir(), driver.BuildLockName(driverName, "*")+".lock")
	paths, err := filepath.Glob(pattern)
	if err != nil {
		return nil, err
	}
	locks := map[string]string{}
	prefix := driver.BuildLockName(driverName, "")
	for _, p := range paths {
		id := strings.TrimSuffix(strings.TrimPrefix(filepath.Base(p), prefix), ".lock")
		if buildIDPattern.MatchString(id) {
			locks[id] = p
		}
	}
	return locks, nil
}

// heldBuildIDs are the IDs of the driver's builds whose locks are held: the
// live builds this user's warden can see, which clean is sent as keep_ids
// (DRV-58).
func heldBuildIDs(driverName string) ([]string, error) {
	locks, err := buildLocks(driverName)
	if err != nil {
		return nil, err
	}
	var ids []string
	for id, p := range locks {
		l, held, err := driver.TryBuildLock(p)
		switch {
		case err != nil:
			return nil, err
		case held:
			ids = append(ids, id)
		case l != nil:
			l.Release()
		}
	}
	return ids, nil
}

// cleanDeadBuilds cleans up after every build of the driver whose lock nobody
// holds. It runs reap, when the driver has one, with the record's values;
// then it removes the build's stage directory, which is warden's own, a
// linked driver's included; and when reap succeeded, or there is none, it
// removes the state file, the record and the lock. A lock with no record is a
// build that died before it created anything, and its file is removed. A
// failed reap keeps the three files for the next clean, and its error is
// returned at the end, after every other build was cleaned. It reports how
// many builds it cleaned up after.
func cleanDeadBuilds(driverName string, reap reapFunc, log io.Writer) (int, error) {
	locks, err := buildLocks(driverName)
	if err != nil {
		return 0, err
	}
	cleaned := 0
	var errs []error
	for id, p := range locks {
		l, held, err := driver.TryBuildLock(p)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if held || l == nil {
			continue
		}
		base := strings.TrimSuffix(p, ".lock")
		rec, err := readRecord(base+".json", driverName, id)
		if errors.Is(err, os.ErrNotExist) {
			if err := l.Remove(); err != nil {
				errs = append(errs, err)
			}
			continue
		}
		if err != nil {
			l.Release()
			errs = append(errs, err)
			continue
		}
		fmt.Fprintf(log, "Cleaning up after build %s (%s)\n", id, driverName)
		var reapErr error
		if reap != nil && !rec.Linked {
			reapErr = reap(rec, base+".state")
		}
		if err := removeStageDir(rec); err != nil {
			errs = append(errs, err)
		}
		if reapErr != nil {
			l.Release()
			errs = append(errs, fmt.Errorf("build %s: %w", id, reapErr))
			continue
		}
		if err := removeBuildFiles(base, l); err != nil {
			errs = append(errs, err)
			continue
		}
		cleaned++
	}
	return cleaned, errors.Join(errs...)
}

// readRecord reads the record at path, and checks that it is the record of
// the driver's build id.
func readRecord(path, driverName, id string) (buildRecord, error) {
	var rec buildRecord
	data, err := os.ReadFile(path)
	if err != nil {
		return rec, err
	}
	if err := json.Unmarshal(data, &rec); err != nil {
		return rec, fmt.Errorf("build record %s: %w", path, err)
	}
	if rec.Driver != driverName || rec.ID != id {
		return rec, fmt.Errorf("build record %s is of build %s (%s)", path, rec.ID,
			rec.Driver)
	}
	return rec, nil
}

// removeStageDir removes the build's stage directory, and only a directory
// of that build's name: a record is a file in the user's cache, and what it
// names is not removed unless it is the stage directory warden made.
func removeStageDir(rec buildRecord) error {
	if rec.StageDir == "" {
		return nil
	}
	if filepath.Base(rec.StageDir) != stageDirName(rec.ID) || !filepath.IsAbs(rec.StageDir) {
		return fmt.Errorf("build record of %s names %s as its stage directory; "+
			"not removed", rec.ID, rec.StageDir)
	}
	return os.RemoveAll(rec.StageDir)
}
