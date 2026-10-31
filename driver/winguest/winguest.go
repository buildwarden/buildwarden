// Package winguest holds the Windows build-guest contract shared by the VM
// drivers that run Windows guests (qemu, hyperv): the seed volume label, the
// build-link addressing, the warden-run.ps1 bootstrap, and the startup task
// that runs it.
//
// The contract: a prepared Windows image carries a SYSTEM startup task named
// warden-run. On every boot the task waits for the WARDEN seed volume and runs
// its warden-run.ps1, which stages warden-io.exe, runs `warden-io initialize`
// (static network, per-build CA, fetch + run build.ps1, report exit code) and
// powers the guest off. Each driver supplies the seed media and the image; the
// guest-side behaviour is defined once here.
//
// Everything in this package is pure string generation, so it is tested on any
// platform.
package winguest

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"encoding/xml"
	"unicode/utf16"
)

const (
	// SeedLabel is the volume label of the per-build seed the startup task
	// looks for.
	SeedLabel = "WARDEN"
	// BuildGatewayIP is the relay's static address on the build link; it must
	// match the relay VM's build NIC. There is no DHCP on that link.
	BuildGatewayIP = "10.0.0.1"
	// BuildGuestCIDR is the build VM's static address and prefix on the build
	// link, the other host of BuildGatewayIP's /30.
	BuildGuestCIDR = "10.0.0.2/30"
	// FirstLogonCommandLimit is Windows Setup's cap on a FirstLogonCommands
	// <CommandLine>. An over-long entry fails the oobeSystem pass with
	// "Windows could not complete the installation" before any command runs.
	FirstLogonCommandLimit = 1024
)

// XMLText escapes s for use as XML element text. Answer files are rendered
// without auto-escaping, and a raw '&', '<' or quote in a password or command
// makes Windows Setup reject the whole file. Windows decodes the entities
// before using the value.
func XMLText(s string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

// EncodedCommand encodes a PowerShell script for powershell.exe
// -EncodedCommand (UTF-16LE, base64). The result is pure [A-Za-z0-9+/=], so it
// survives XML, cmd and PowerShell quoting unchanged.
func EncodedCommand(script string) string {
	u := utf16.Encode([]rune(script))
	b := make([]byte, len(u)*2)
	for i, r := range u {
		binary.LittleEndian.PutUint16(b[i*2:], r)
	}
	return base64.StdEncoding.EncodeToString(b)
}
