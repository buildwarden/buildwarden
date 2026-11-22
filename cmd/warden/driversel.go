package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/buildwarden/buildwarden/driver"
)

// selectedDriver is the driver an invocation runs: linked into warden, or a
// binary reached through the exec transport. Exactly one of linked and bin
// is set.
type selectedDriver struct {
	found  *foundDriver
	linked *linkedDriver
	bin    *execDriver
}

func (s *selectedDriver) name() string { return s.found.Name }

// lists reports whether the driver lists command.
func (s *selectedDriver) lists(command string) bool {
	if s.bin != nil {
		return s.bin.desc.lists(command)
	}
	return slices.Contains(s.linked.Commands, command)
}

// caps is what the driver's describe reports under capabilities.
func (s *selectedDriver) caps() capabilities {
	if s.bin != nil {
		return s.bin.desc.caps()
	}
	return s.linked.Capabilities
}

// driverSelection is the selection cfg's sources make: --driver, else
// WARDEN_DRIVER, else [runtime] driver, else container (DRV-11).
func driverSelection(cfg *Config) selection {
	if cfg.Runtime.Driver == "" {
		return selection{value: "container", source: sourceDflt}
	}
	src := cfg.Runtime.DriverSource
	if src == "" {
		src = sourceConfig
	}
	return selection{value: cfg.Runtime.Driver, source: src, file: cfg.File}
}

// selectDriver finds the driver cfg selects and, for a binary, runs its
// describe once (DRV-6, DRV-38).
func selectDriver(cfg *Config) (*selectedDriver, error) {
	if st := driverFlags; st.sel != nil && st.key == driverSelection(cfg) {
		if st.sel.bin != nil {
			st.sel.bin.teardown = cfg.Build.teardown()
		}
		return st.sel, nil
	}
	return findSelected(cfg)
}

// findSelected is selectDriver without the driver the scan selected.
func findSelected(cfg *Config) (*selectedDriver, error) {
	fd, err := findDriver(driverSelection(cfg), hostLookupEnv())
	if err != nil {
		return nil, err
	}
	if fd.Linked {
		return &selectedDriver{found: fd, linked: linkedDrivers[fd.Name]}, nil
	}
	bin, err := describeDriver(fd, time.After)
	if err != nil {
		return nil, err
	}
	bin.teardown = cfg.Build.teardown()
	return &selectedDriver{found: fd, bin: bin}, nil
}

// driverJSON is <output>/driver.json: which driver ran a build, where it was
// found, and the copies its lookup passed over. It is unsigned and not part
// of the ledger.
type driverJSON struct {
	Name     string       `json:"name"`
	Version  string       `json:"version"`
	Contract int          `json:"contract"`
	Path     string       `json:"path"`
	SHA256   string       `json:"sha256"`
	Found    string       `json:"found"`
	Shadowed []driverCopy `json:"shadowed"`
}

// newDriverJSON is the driver.json of a build of s; for a linked driver the
// path and digest are warden's own.
func newDriverJSON(s *selectedDriver) (*driverJSON, error) {
	dj := &driverJSON{Name: s.name(), Version: version, Contract: contractVersion,
		Path: s.found.Path, Found: s.found.Found,
		Shadowed: append([]driverCopy{}, s.found.Shadowed...)}
	if s.bin != nil {
		dj.Version, dj.Contract = s.bin.desc.Version, s.bin.contract
	}
	sum, err := fileSHA256(s.found.Path)
	if err != nil {
		return nil, fmt.Errorf("the digest of driver %s: %w",
			driver.EscapeOutside(s.found.Path), err)
	}
	dj.SHA256 = sum
	return dj, nil
}

// write writes driver.json into the output directory dir.
func (dj *driverJSON) write(dir string) error {
	data, err := json.MarshalIndent(dj, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "driver.json"), append(data, '\n'), 0o644)
}

// fileSHA256 is the hex SHA-256 digest of the file at path.
func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
