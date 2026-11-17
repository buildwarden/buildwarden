package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/buildwarden/buildwarden/driver"
)

// resolveUpstreamCAs reads the upstream CA paths in effect and returns the
// certificates a build's claim carries, printing through print every line
// driver.LoadUpstreamCAs reports: each skip and each path's count always,
// the per-file list with --verbose. It runs before anything is created, so a
// path that a build cannot use stops it there. With no configured path it
// reads and prints nothing.
func resolveUpstreamCAs(cfg *Config, print func(string)) ([]byte, error) {
	paths, source, err := upstreamCAPaths(cfg)
	if err != nil {
		return nil, err
	}
	return readUpstreamCAs(cfg, paths, source, print)
}

// upstreamCAPaths returns the configured upstream CA paths and where they
// were configured: WARDEN_UPSTREAM_CA_CERTS's entries when the variable is
// set, split on the operating system's path-list separator, else [relay]
// upstream_ca_certs. The variable replaces the key; it does not add to it.
func upstreamCAPaths(cfg *Config) ([]string, string, error) {
	v := os.Getenv(driver.CAFromEnv)
	if v == "" {
		return cfg.Relay.UpstreamCACerts, driver.CAFromConfig, nil
	}
	// TRANSITIONAL(one-config-file)
	if err := formerCAVariableAdded(cfg); err != nil {
		return nil, "", err
	}
	return filepath.SplitList(v), driver.CAFromEnv, nil
}

// readUpstreamCAs reads paths and prints the report's lines, those before an
// error included.
func readUpstreamCAs(cfg *Config, paths []string, source string,
	print func(string)) ([]byte, error) {
	if len(paths) == 0 {
		return nil, nil
	}
	certs, report, err := driver.LoadUpstreamCAs(paths, source)
	for _, line := range report.Lines(cfg.Output.Verbose) {
		print(line)
	}
	return certs, err
}

// doctorUpstreamCAs is resolveUpstreamCAs in doctor's report: the same lines
// on out, without the prefix, and a path a build would refuse reported as
// BLOCKED, which fails doctor. The transitional error is returned as it is,
// as a build returns it.
func doctorUpstreamCAs(out io.Writer, cfg *Config) (blocked bool, err error) {
	paths, source, err := upstreamCAPaths(cfg)
	if err != nil {
		return false, err
	}
	_, err = readUpstreamCAs(cfg, paths, source, func(line string) {
		fmt.Fprintln(out, line)
	})
	if err == nil {
		return false, nil
	}
	fmt.Fprintf(out, "upstream CA: BLOCKED: %v\n", err)
	var fe *driver.FixableError
	if errors.As(err, &fe) && fe.Fix != "" {
		fmt.Fprintf(out, "upstream CA: to fix: %s\n", fe.Fix)
	}
	return true, nil
}
