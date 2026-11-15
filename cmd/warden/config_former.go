// TRANSITIONAL(one-config-file): removed in the change that prepares 1.0,
// together with the two calls (LoadConfig in config.go, resolveUpstreamCAs
// in upstreamca.go), config_former_test.go and the "Moving from
// ~/.config/warden/config.toml" subsection of the configuration reference
// (guide/configuration.md). A search for the marker finds all five. It also
// finds docs/design/project-config-trust.md, the design that records them,
// which is kept.
//
// Until warden read one config file, two things held that no longer do:
// ~/.config/warden/config.toml was read under ./warden.toml, and
// WARDEN_UPSTREAM_CA_CERTS added its paths to those of [relay]
// upstream_ca_certs. Neither is true now, and without this file both would
// stop without a word: the old file's settings would no longer apply, and
// the key's paths would be dropped when the variable is set. This file makes
// each an error that says what to change. It only produces errors: it reads
// no setting and never changes which file is read or which paths are used.

package main

import (
	"os"
	"path/filepath"

	"github.com/buildwarden/buildwarden/driver"
)

// formerUserConfig returns the error for a ~/.config/warden/config.toml that
// exists, or nil. The file is not opened: it is no longer a source of
// settings. configFile is the config file in use, empty when there is none;
// when it is that same file, the caller named it with --config or
// WARDEN_CONFIG, it is the config file, and nothing has stopped applying.
func formerUserConfig(configFile string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	former := filepath.Join(home, ".config", "warden", "config.toml")
	info, err := os.Stat(former)
	if err != nil {
		return nil
	}
	if configFile != "" {
		if inUse, err := os.Stat(configFile); err == nil && os.SameFile(info, inUse) {
			return nil
		}
	}
	fix := "move its settings into the warden.toml you build with, or into " +
		"the environment (WARDEN_DRIVER, WARDEN_CTR_CLI, WARDEN_IMAGE, " +
		"WARDEN_UPSTREAM_CA_CERTS), and delete it; or name it as the config " +
		"file with --config or WARDEN_CONFIG (it is then the only one read: " +
		"./warden.toml is not)"
	// sudo resets the environment by default, so someone who took the second
	// way out with the variable meets this error again under sudo.
	if os.Getenv("SUDO_USER") != "" {
		fix += ". Under sudo, WARDEN_CONFIG is dropped unless sudo is told " +
			"to keep it: pass --config, or run sudo --preserve-env=WARDEN_CONFIG"
	}
	return driver.Fixable(fix, "%s is no longer read: warden reads one config "+
		"file, ./warden.toml or the one --config or WARDEN_CONFIG names", former)
}

// formerCAVariableAdded returns the error for WARDEN_UPSTREAM_CA_CERTS set
// together with [relay] upstream_ca_certs, or nil. The variable used to add
// its paths to the key's and now replaces them, so with both set the relay
// would trust fewer roots than before; the caller has found the variable
// set.
func formerCAVariableAdded(cfg *Config) error {
	if len(cfg.Relay.UpstreamCACerts) == 0 {
		return nil
	}
	return driver.Fixable("put every path in one of the two, and unset the "+
		"variable or remove the key",
		"WARDEN_UPSTREAM_CA_CERTS and [relay] upstream_ca_certs in %s are both "+
			"set: the variable now replaces the key, and no longer adds its "+
			"paths to the key's", cfg.File)
}
