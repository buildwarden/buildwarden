// Package driverconformance is the driver contract's conformance suite
// (specs/driver-contract/v1.md): it plays the orchestrator against a driver
// binary and reports, per check, the requirements of the specification the
// check holds the driver to. It is written from the specification and
// imports nothing under driver/ or cmd/warden, so a driver in any language
// is held to the same checks.
//
// This package holds the static tier: checks that need no hypervisor, no
// container runtime and no network. The command driver-conformance runs it
// against a binary.
package driverconformance

import (
	_ "embed"
	"regexp"
)

// schemaJSON is v1.schema.json, a copy of specs/driver-contract's, which a
// test keeps identical to it.
//
//go:embed v1.schema.json
var schemaJSON []byte

// The contract's commands, describe aside, in the specification's order.
var contractCommands = []string{"run", "reap", "doctor", "setup", "image-prepare",
	"image-list", "image-verify", "clean", "attach", "ext"}

// flagCommands are the commands a driver's flag may name.
var flagCommands = []string{"run", "doctor", "setup", "image-prepare", "clean"}

// commonKeys are the common keys of contract version 1, each a string
// holding an age.
var commonKeys = []string{"image_refresh_after", "image_min_grace"}

// oldNames are the deprecated old names of version 1 by driver name, each
// with its key.
var oldNames = map[string]map[string]string{
	"container": {
		"runtime.cli":         "driver.container.cli",
		"runtime.relay_image": "driver.container.relay_image",
	},
	"qemu": {
		"vm.image_refresh_after": "driver.image_refresh_after",
		"vm.image_min_grace":     "driver.image_min_grace",
	},
	"hyperv": {
		"vm.image_refresh_after":  "driver.image_refresh_after",
		"vm.image_min_grace":      "driver.image_min_grace",
		"vm.network":              "driver.hyperv.network",
		"vm.windows_image":        "driver.hyperv.windows_image",
		"vm.windows_image_update": "driver.hyperv.windows_image_update",
	},
}

// reservedFlags are warden's flag names by command; those under "" are
// reserved on every command.
var reservedFlags = map[string][]string{
	"": {"driver", "config", "verbose", "color", "help", "version"},
	"run": {"capture", "output", "no-compress", "timeout", "tmpdir", "quiet", "guest-os",
		"script", "image", "cpus", "memory", "keep-vms", "build-link"},
	"doctor":        {"build-link"},
	"image-prepare": {"os", "source"},
	"clean":         {"cache"},
}

var (
	driverNameRE = regexp.MustCompile(`^[a-z][a-z0-9]{0,31}$`)
	flagNameRE   = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
)

// describeKey is one entry of describe's keys.
type describeKey struct {
	Key      string   `json:"key"`
	Type     string   `json:"type"`
	Aliases  []string `json:"aliases"`
	Commands []string `json:"commands"`
}

// description is the part of describe the suite reads.
type description struct {
	Contract     []int    `json:"contract"`
	Name         string   `json:"name"`
	Commands     []string `json:"commands"`
	Capabilities struct {
		VM             bool     `json:"vm"`
		Shell          bool     `json:"shell"`
		Keep           string   `json:"keep"`
		Exited         bool     `json:"exited"`
		GuestOS        []string `json:"guest_os"`
		ImageOS        []string `json:"image_os"`
		DefaultImageOS string   `json:"default_image_os"`
	} `json:"capabilities"`
	Flags []struct {
		Name     string   `json:"name"`
		Value    bool     `json:"value"`
		Commands []string `json:"commands"`
		Usage    string   `json:"usage"`
	} `json:"flags"`
	Keys []describeKey `json:"keys"`
	Ext  []struct {
		Path []string `json:"path"`
	} `json:"ext"`
}
