package winguest

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/buildwarden/buildwarden/driver"
	"github.com/buildwarden/buildwarden/ledger"
)

// StagePrepareSeed writes the prepare seed's files into dir: the OOBE answer
// file under both implicit-search names, and warden-prepare.ps1. Each driver
// packages dir as media labelled PrepareSeedLabel (Hyper-V: an ISO; qemu: a
// writable FAT disk).
func StagePrepareSeed(dir, goarch string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("prepare seed dir: %w", err)
	}
	unattend := OOBEUnattend(OOBEConfig{
		ProcArch:          UnattendArch(goarch),
		AdminPassword:     driver.RandAlphaNum(20),
		FirstLogonCommand: SeedScriptCommand(PrepareScriptName),
		Description:       "warden image prepare",
	})
	files := map[string]string{
		"unattend.xml":     unattend,
		"Autounattend.xml": unattend,
		PrepareScriptName:  PrepareScript(),
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			return fmt.Errorf("write %s: %w", name, err)
		}
	}
	return nil
}

// UnattendArch maps a Go arch to the unattend processorArchitecture token.
func UnattendArch(goarch string) string {
	if goarch == "arm64" {
		return "arm64"
	}
	return "amd64"
}

// SeedInputs hashes a prepare seed's files, sorted by name, as the prepared
// layer's recorded inputs.
func SeedInputs(dir string) ([]ledger.PreparationInput, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var inputs []ledger.PreparationInput
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		sum, err := driver.HashFileSHA256(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		fi, err := e.Info()
		if err != nil {
			return nil, err
		}
		inputs = append(inputs, ledger.PreparationInput{
			Name: e.Name(), SHA256: sum, Size: fi.Size(),
		})
	}
	sort.Slice(inputs, func(i, j int) bool { return inputs[i].Name < inputs[j].Name })
	return inputs, nil
}
