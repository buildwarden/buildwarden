package main

import (
	"strings"
	"testing"

	"github.com/buildwarden/buildwarden/driver"
)

// On Windows the variable is split on ';', so a drive letter's ':' stays in
// its path.
func TestUpstreamCAPathsWindowsSeparator(t *testing.T) {
	inConfigFiles(t, "")
	t.Setenv(driver.CAFromEnv, `C:\certs\corp.pem;D:\roots`)
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	paths, _, err := upstreamCAPaths(cfg)
	if err != nil || strings.Join(paths, "|") != `C:\certs\corp.pem|D:\roots` {
		t.Errorf("got %q, %v", paths, err)
	}
}
