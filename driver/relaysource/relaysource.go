// Package relaysource names the relay a driver runs for a build: its release
// form and the roles of its files. It is a kit library and not part of the
// driver contract: a driver from outside finds the project's relay however
// it likes, and warden never sees one.
//
// The form and role names are provisional. They are this repository's own
// until seam 8, the relay release forms, is written, and may change with it.
package relaysource

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/buildwarden/buildwarden/driver"
)

// RelayForm names a relay release form (seam 8, provisional).
type RelayForm string

// The relay release forms.
const (
	// RelayHostBinary is a relay executable run on the host in host mode.
	RelayHostBinary RelayForm = "host-binary"
	// RelayOCIImage is a container image run in container mode.
	RelayOCIImage RelayForm = "oci-image"
	// RelayAppliance is a Linux relay VM on the kernel network path: a
	// kernel and an initramfs, or one boot disk.
	RelayAppliance RelayForm = "appliance"
	// RelayUnikernel is a single bootable unikernel image. No driver runs
	// one yet; the name is reserved for M6.
	RelayUnikernel RelayForm = "unikernel"
)

// RelayRole names a file's part in a form (provisional).
type RelayRole string

// The roles of a relay's files.
const (
	// RelayBinary is the host-binary's executable; or, in an appliance, a
	// relay to append to the initramfs at /sbin/relay.
	RelayBinary RelayRole = "binary"
	// RelayKernel is an appliance's or a unikernel's kernel.
	RelayKernel RelayRole = "kernel"
	// RelayInitramfs is an appliance's initramfs.
	RelayInitramfs RelayRole = "initramfs"
	// RelayDisk is an appliance as one boot disk.
	RelayDisk RelayRole = "disk"
)

// Relay is the relay one build runs, resolved to local files or an image.
type Relay struct {
	Form RelayForm
	// Version is the release it comes from, "dev" for a source-tree build,
	// or "" when unknown (an override).
	Version string
	// Origin says where it came from, for messages: "bundled", "source
	// tree", or the option or variable that named it.
	Origin string
	// Image is the reference to run, for RelayOCIImage.
	Image string
	// Files are the form's files by role.
	Files map[RelayRole]RelayFile
}

// RelayFile is one file of a relay.
type RelayFile struct {
	Path string
	// SHA256 is the published hash the file must have, or "" when none is
	// published for it. Verify checks every file that has one.
	SHA256 string
}

// Verify re-hashes each file that carries a published hash and refuses a
// mismatch with a FixableError naming the file and its origin.
func (r *Relay) Verify() error {
	roles := make([]string, 0, len(r.Files))
	for role := range r.Files {
		roles = append(roles, string(role))
	}
	sort.Strings(roles)
	for _, role := range roles {
		f := r.Files[RelayRole(role)]
		if f.SHA256 == "" {
			continue
		}
		got, err := fileSHA256(f.Path)
		if err != nil {
			return fmt.Errorf("verifying the relay's %s %s: %w", role, f.Path, err)
		}
		if !strings.EqualFold(got, f.SHA256) {
			return driver.Fixable("replace it with the published file, or remove "+
				"what names it",
				"the relay's %s %s (from %s) has SHA-256 %s, not the published %s",
				role, f.Path, r.Origin, got, f.SHA256)
		}
	}
	return nil
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// RelaySource resolves the relay for a build.
type RelaySource interface {
	Relay(ctx context.Context) (*Relay, error)
}

// ErrNotFound is what Bundled's Relay wraps when neither place has every
// file; the driver adds its own fix.
var ErrNotFound = errors.New("relay files not found")

// GoBuild is a cmd/ package to find beside the running binary, or to build
// from the source tree, for an OS and architecture (driver.GuestBinary).
type GoBuild struct {
	Package, GOOS, GOARCH string
}

// File is one bundled file: its release name beside the binary and its path
// in the source tree (slash-separated, from the module root), or a cmd/
// package to build.
type File struct {
	Role            RelayRole
	Release, Source string
	Build           *GoBuild
}

// Bundled finds a relay's files beside the running binary, else in the
// source tree, all from one place, and builds or finds a Build file as
// driver.GuestBinary does. Relay returns an error wrapping ErrNotFound when
// neither place has every file.
func Bundled(form RelayForm, files ...File) RelaySource {
	return bundled{form: form, files: files}
}

type bundled struct {
	form  RelayForm
	files []File
}

// The two places Bundled looks in; tests replace them.
var (
	executable = os.Executable
	moduleRoot = driver.ModuleRoot
)

// place is where Bundled looks for the files: beside the running binary, or
// in the source tree.
type place struct {
	origin string
	at     func(File) string
}

func (b bundled) Relay(context.Context) (*Relay, error) {
	var places []place
	if exe, err := executable(); err == nil {
		dir := filepath.Dir(exe)
		places = append(places, place{"bundled",
			func(f File) string { return filepath.Join(dir, f.Release) }})
	}
	if root, err := moduleRoot(); err == nil {
		places = append(places, place{"source tree", func(f File) string {
			return filepath.Join(root, filepath.FromSlash(f.Source))
		}})
	}
	for _, p := range places {
		if r, ok := b.findIn(p); ok {
			return r, b.build(r)
		}
	}
	return nil, fmt.Errorf("%w: %s", ErrNotFound, b.names())
}

// findIn is the relay with every file that is not built found at p, or
// false when one is missing there.
func (b bundled) findIn(p place) (*Relay, bool) {
	r := &Relay{Form: b.form, Origin: p.origin, Files: map[RelayRole]RelayFile{}}
	if p.origin == "source tree" {
		r.Version = "dev"
	}
	for _, f := range b.files {
		if f.Build != nil {
			continue
		}
		path := p.at(f)
		if _, err := os.Stat(path); err != nil {
			return nil, false
		}
		r.Files[f.Role] = RelayFile{Path: path}
	}
	return r, true
}

// build adds the files built from a cmd/ package.
func (b bundled) build(r *Relay) error {
	for _, f := range b.files {
		if f.Build == nil {
			continue
		}
		path, err := driver.GuestBinary(f.Build.Package, f.Build.GOOS, f.Build.GOARCH)
		if err != nil {
			return err
		}
		r.Files[f.Role] = RelayFile{Path: path}
	}
	return nil
}

func (b bundled) names() string {
	var names []string
	for _, f := range b.files {
		if f.Build == nil {
			names = append(names, f.Release)
		}
	}
	return strings.Join(names, ", ")
}

// Path is a relay file the user named: used as given, never looked up
// elsewhere. origin is the option or variable that named it.
func Path(form RelayForm, role RelayRole, path, origin string) RelaySource {
	return pathSource{form: form, role: role, path: path, origin: origin}
}

type pathSource struct {
	form   RelayForm
	role   RelayRole
	path   string
	origin string
}

func (p pathSource) Relay(context.Context) (*Relay, error) {
	if _, err := os.Stat(p.path); err != nil {
		return nil, driver.Fixable("name an existing file, or remove "+p.origin,
			"the relay %s %s named by %s: %v", p.role, p.path, p.origin, err)
	}
	return &Relay{Form: p.form, Origin: p.origin,
		Files: map[RelayRole]RelayFile{p.role: {Path: p.path}}}, nil
}

// Image is an OCI image reference the user named, pulled as given.
func Image(ref, origin string) RelaySource {
	return imageSource{ref: ref, origin: origin}
}

type imageSource struct{ ref, origin string }

func (i imageSource) Relay(context.Context) (*Relay, error) {
	return &Relay{Form: RelayOCIImage, Origin: i.origin, Image: i.ref}, nil
}
