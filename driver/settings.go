package driver

import (
	"net/netip"
	"time"
)

// Settings is a driver's resolved configuration for one command
// (specs/driver-contract/v1.md, Configuration). A driver reads a value by the
// name it declared; ok is false when nobody set it, and the driver then
// applies its default. driver/drivercmd implements it.
type Settings interface {
	String(name string) (value string, ok bool)
	Int(name string) (value int, ok bool)
	Bool(name string) (value, ok bool)
	// Age is a Go duration or a number of days ("7d", "36h"); an empty
	// string in the file is the zero age.
	Age(name string) (value time.Duration, ok bool)
	CIDR(name string) (value netip.Prefix, ok bool)
	IPList(name string) (value []netip.Addr, ok bool)
	Path(name string) (value string, ok bool)
	// Decode reads a Raw setting, a key of any shape, into v.
	Decode(name string, v any) (ok bool, err error)
	// Source is the label to print for a value: --runtime, WARDEN_CTR_CLI,
	// or the file it was read from with the table and key as written there.
	Source(name string) string
	// Warnings are the lines to print on stderr: one for each deprecated
	// old name a config file sets. drivercmd.Call prints them, in a probe
	// and in a command warden does not probe first; a driver's function
	// does not.
	Warnings() []string
	// Err is the error of a resolution put off until a setting was first
	// read, as in reap and attach (DRV-C10); nil otherwise, and nil before
	// a setting was read. A value read while Err is not nil is not set.
	Err() error
}
