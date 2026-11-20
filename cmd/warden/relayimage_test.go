package main

import (
	"bytes"
	"context"
	"strconv"
	"strings"
	"testing"

	"github.com/buildwarden/buildwarden/driver"
)

const relayImageFix = "set it to an image reference such as " +
	"ghcr.io/buildwarden/relay:v0.2.0 or to \"dev\", or remove it"

// Empty, dev and references pass; a value shaped like an option, or holding
// a space or a control character, is refused from the file and from
// WARDEN_IMAGE, by a build and by doctor, under the container driver only.
func TestRelayImageShape(t *testing.T) {
	passes := []string{"", "dev", "ghcr.io/buildwarden/relay:v0.2.0",
		"localhost:5000/relay@sha256:0123", "relay"}
	refused := map[string]string{
		"an option":                 "--volume=/:/host",
		"a short option":            "-v",
		"a space":                   "relay --privileged",
		"a tab":                     "relay\t--privileged",
		"a line break":              "relay\nx",
		"an escape character":       "relay\x1b[2J",
		"a right-to-left override":  "a\u202eb",
		"a zero-width space":        "a\u200bb",
		"a leading byte order mark": "\ufeff--x",
		"a no-break space":          "relay\u00a0x",
	}
	// A build checks it in the container driver's probe, before the runtime
	// is resolved; a runtime that cannot be found is not this test's.
	newBuild := func(t *testing.T) error {
		cfg, err := resolveConfig()
		if err != nil {
			t.Fatalf("resolveConfig: %v", err)
		}
		probe := &driver.Start{Contract: 1, Probe: true, Mode: driver.ModeBuild,
			GuestOS: "linux", ConfigFile: absConfigFile(cfg), Args: []string{}}
		_, err = callLinkedRun(context.Background(), linkedDrivers["container"],
			"container", cfg, &driver.BuildRequest{}, probe, nil)
		if err != nil && !strings.Contains(err.Error(), "relay_image") &&
			!strings.Contains(err.Error(), "WARDEN_IMAGE") {
			return nil
		}
		return err
	}
	doctor := func(t *testing.T) error {
		doctorCmd.SetOut(&bytes.Buffer{})
		t.Cleanup(func() { doctorCmd.SetOut(nil) })
		return doctorCmd.RunE(doctorCmd, nil)
	}
	for _, v := range passes {
		t.Run("passes "+v, func(t *testing.T) {
			inConfigFiles(t, "[runtime]\nrelay_image = "+tomlString(v)+"\n")
			captureLog(t)
			if err := newBuild(t); err != nil {
				t.Errorf("newRequest: %v", err)
			}
		})
	}
	for name, v := range refused {
		for cmd, run := range map[string]func(*testing.T) error{
			"build": newBuild, "doctor": doctor,
		} {
			t.Run(cmd+" refuses "+name+" from the file", func(t *testing.T) {
				inConfigFiles(t, "[runtime]\nrelay_image = "+tomlString(v)+"\n")
				captureLog(t)
				msg := "[runtime] relay_image in " + configPath(t) + " is " +
					strconv.Quote(v) + ": not an image reference"
				wantFixable(t, run(t), msg, relayImageFix)
			})
			t.Run(cmd+" refuses "+name+" from WARDEN_IMAGE", func(t *testing.T) {
				inConfigFiles(t, "[runtime]\nrelay_image = \"dev\"\n")
				t.Setenv("WARDEN_IMAGE", v)
				captureLog(t)
				wantFixable(t, run(t), "WARDEN_IMAGE is "+strconv.Quote(v)+
					": not an image reference", relayImageFix)
			})
		}
	}
	t.Run("not checked under a VM driver", func(t *testing.T) {
		inConfigFiles(t, "[runtime]\nrelay_image = \"--volume=/:/host\"\n")
		flagDriver = "qemu"
		captureLog(t)
		cfg, err := resolveConfig()
		if err != nil {
			t.Fatal(err)
		}
		_, err = newRequest(cfg, linkedSel(cfg), []string{buildProject(t)})
		if err != nil && strings.Contains(err.Error(), "relay_image") {
			t.Errorf("a VM driver's build checked the relay image: %v", err)
		}
	})
}
