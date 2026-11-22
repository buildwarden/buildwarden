package drivercmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// MaxStateFile is the most a state file may hold (DRV-61).
const MaxStateFile = 64 << 10

// SaveState writes v as the build's state file, atomically: a reader sees
// the old file or the new one, never part of one. The driver saves it
// before it creates what it describes (DRV-61).
func SaveState(stateFile string, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("state file %s: %w", stateFile, err)
	}
	if len(data) > MaxStateFile {
		return fmt.Errorf("state file %s: %d bytes, more than the contract's %d",
			stateFile, len(data), MaxStateFile)
	}
	tmp, err := os.CreateTemp(filepath.Dir(stateFile), filepath.Base(stateFile)+".*")
	if err != nil {
		return fmt.Errorf("state file: %w", err)
	}
	_, werr := tmp.Write(data)
	cerr := tmp.Close()
	if err := errors.Join(werr, cerr); err != nil {
		_ = os.Remove(tmp.Name())
		return fmt.Errorf("state file %s: %w", stateFile, err)
	}
	if err := os.Rename(tmp.Name(), stateFile); err != nil {
		_ = os.Remove(tmp.Name())
		return fmt.Errorf("state file: %w", err)
	}
	return nil
}

// LoadState reads the build's state file into v, in attach and reap, and
// reports whether one exists.
func LoadState(stateFile string, v any) (found bool, err error) {
	data, err := os.ReadFile(stateFile)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("state file: %w", err)
	}
	if err := json.Unmarshal(data, v); err != nil {
		return true, fmt.Errorf("state file %s: %w", stateFile, err)
	}
	return true, nil
}
