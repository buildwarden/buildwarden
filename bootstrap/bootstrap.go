// Package bootstrap is the declarative configuration a build environment's
// executor fetches from the relay at GET /bootstrap (BOOT-05). It carries
// policy only: no paths, file contents, commands or environment variables.
// Everything the executor installs is derived from this config and the
// relay's CA by warden-io itself. See docs/development/bootstrap.md.
package bootstrap

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// Version is the config format version this package reads and writes.
const Version = 1

// Trust modes for the guest's system trust store.
const (
	// TrustAppend adds the warden CA to the system's existing roots.
	TrustAppend = "append"
	// TrustExclusive makes the warden CA the only system root. Every TLS
	// connection from the guest goes through the relay's MITM, so no other
	// root can validate a real connection.
	TrustExclusive = "exclusive"
)

// Config is the bootstrap document. The zero value is the empty config: append
// trust, which is also what an executor uses against a relay that predates
// the endpoint.
type Config struct {
	Version int   `json:"version"`
	Trust   Trust `json:"trust"`
}

// Trust is the system trust policy.
type Trust struct {
	Mode string `json:"mode,omitempty"`
}

// ForGuest returns the default config for a guest OS family ("linux",
// "windows", "darwin"; empty means linux). Linux guests get exclusive trust;
// the others append until their trust stores get the same treatment.
func ForGuest(guestOS string) Config {
	mode := TrustExclusive
	switch strings.ToLower(guestOS) {
	case "windows", "darwin", "macos":
		mode = TrustAppend
	}
	return Config{Version: Version, Trust: Trust{Mode: mode}}
}

// TrustMode returns the effective trust mode (append when unset).
func (c Config) TrustMode() string {
	if c.Trust.Mode == "" {
		return TrustAppend
	}
	return c.Trust.Mode
}

// Marshal encodes the config as JSON.
func (c Config) Marshal() []byte {
	data, _ := json.Marshal(c) //nolint:errchkjson // plain struct, cannot fail
	return data
}

// Parse decodes and validates a config strictly: an unknown field, version or
// trust mode is an error, so an executor never guesses at a newer config. An
// empty body is the empty config.
func Parse(data []byte) (Config, error) {
	var c Config
	if len(bytes.TrimSpace(data)) == 0 {
		return c, nil
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&c); err != nil {
		return Config{}, fmt.Errorf("bootstrap config: %w", err)
	}
	if dec.More() {
		return Config{}, fmt.Errorf("bootstrap config: trailing data")
	}
	if c.Version != Version {
		return Config{}, fmt.Errorf("bootstrap config: unsupported version %d", c.Version)
	}
	switch c.Trust.Mode {
	case "", TrustAppend, TrustExclusive:
	default:
		return Config{}, fmt.Errorf("bootstrap config: unknown trust mode %q",
			c.Trust.Mode)
	}
	return c, nil
}
