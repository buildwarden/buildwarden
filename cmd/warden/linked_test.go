package main

import (
	"encoding/json"
	"testing"
)

// DRV-36, DRV-38, DRV-C1, DRV-C4, DRV-C11: each linked driver's describe,
// built by the kit from its package's declarations, is one warden accepts
// from a driver binary: it matches the schema, uses no flag of warden's on
// the same command and puts every key in its place. A clash is found here,
// by make test, and not by a user.
func TestLinkedDescriptions(t *testing.T) {
	for name, l := range linkedDrivers {
		t.Run(name, func(t *testing.T) {
			d, err := l.description(name)
			if err != nil {
				t.Fatal(err)
			}
			out, err := json.Marshal(d)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := parseDescription(out, name); err != nil {
				t.Fatalf("warden refuses %s's describe: %v\n%s", name, err, out)
			}
		})
	}
}
