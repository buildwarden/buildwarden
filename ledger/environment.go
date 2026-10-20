package ledger

import (
	"bytes"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"regexp"
	"strings"

	"golang.org/x/crypto/blake2b"
)

// Environment descriptor types (see "Environment Record" in the ledger spec).
// A descriptor names its own type inside the signed payload; the header label
// and record metadata only echo it and are unsigned.
const (
	// EnvTypeOCIImage is a container image; the payload is its OCI/Docker
	// manifest, identified by the manifest's own schemaVersion/mediaType.
	EnvTypeOCIImage = "oci-image/v1"
	// EnvTypeVMDiskChain is a VM booted from an ordered chain of disk layers
	// plus per-build seed files; the payload is a VMDiskChain JSON object.
	EnvTypeVMDiskChain = "vm-disk-chain/v1"
	// EnvTypeLegacyContainer is the header label every ledger carried before
	// environment types were specified. Readers treat it as EnvTypeOCIImage.
	EnvTypeLegacyContainer = "container"
)

// Layer roles in a VMDiskChain.
const (
	LayerRoleDisk     = "disk"
	LayerRoleKernel   = "kernel"
	LayerRoleInitrd   = "initrd"
	LayerRoleNVRAM    = "nvram"
	LayerRolePlatform = "platform"
)

// Layer source kinds in a VMDiskChain.
const (
	SourceURL      = "url"
	SourceLocal    = "local"
	SourcePrepared = "prepared"
)

// VMDiskChain is the vm-disk-chain/v1 environment descriptor.
type VMDiskChain struct {
	Type       string      `json:"type"`
	Driver     string      `json:"driver"`
	GuestOS    string      `json:"guest_os"`
	Arch       string      `json:"arch"`
	Generation int         `json:"generation,omitempty"`
	Layers     []DiskLayer `json:"layers"`
	Seed       []SeedFile  `json:"seed,omitempty"`
}

// DiskLayer is one file the build VM boots from. Layers with role "disk" form
// the disk chain base-first; the per-build writable child is not a layer.
type DiskLayer struct {
	Role   string      `json:"role"`
	Name   string      `json:"name"`
	Format string      `json:"format,omitempty"`
	Path   string      `json:"path,omitempty"`
	SHA256 string      `json:"sha256"`
	Size   int64       `json:"size"`
	Source LayerSource `json:"source"`
}

// LayerSource is a layer's provenance.
type LayerSource struct {
	Kind string `json:"kind"`
	URL  string `json:"url,omitempty"`
	// Pinned reports that a downloaded layer verified against an expected
	// hash supplied independently of the download.
	Pinned bool `json:"pinned,omitempty"`
	// PreparedBy is "warden <version>" for a layer warden produced.
	PreparedBy string `json:"prepared_by,omitempty"`
	// Inputs are the hashed inputs a prepared layer was produced from.
	Inputs []PreparationInput `json:"inputs,omitempty"`
}

// PreparationInput is one input to a prepared layer.
type PreparationInput struct {
	Name   string `json:"name"`
	SHA256 string `json:"sha256,omitempty"`
	Size   int64  `json:"size,omitempty"`
	URL    string `json:"url,omitempty"`
}

// SeedFile is a per-build file handed to the guest (agent, bootstrap, answer
// file, build script).
type SeedFile struct {
	Name   string `json:"name"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

var sha256Hex = regexp.MustCompile(`^[0-9a-f]{64}$`)

// EnvironmentType returns the descriptor type a payload names in its own
// bytes: the "type" member of a typed descriptor, or EnvTypeOCIImage for an
// OCI/Docker image manifest or index.
func EnvironmentType(payload []byte) (string, error) {
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(payload, &probe); err != nil {
		return "", fmt.Errorf("environment payload is not a JSON object: %w", err)
	}
	if raw, ok := probe["type"]; ok {
		var t string
		if err := json.Unmarshal(raw, &t); err != nil || t == "" {
			return "", errors.New(`environment payload "type" is not a string`)
		}
		return t, nil
	}
	if isImageManifest(probe) {
		return EnvTypeOCIImage, nil
	}
	return "", errors.New("environment payload names no type")
}

// isImageManifest reports whether probe looks like an OCI/Docker manifest or
// index: schemaVersion plus layers, manifests or an image media type.
func isImageManifest(probe map[string]json.RawMessage) bool {
	if _, ok := probe["schemaVersion"]; !ok {
		return false
	}
	if _, ok := probe["layers"]; ok {
		return true
	}
	if _, ok := probe["manifests"]; ok {
		return true
	}
	var mt string
	if raw, ok := probe["mediaType"]; ok && json.Unmarshal(raw, &mt) == nil {
		return strings.Contains(mt, "manifest") || strings.Contains(mt, "image.index")
	}
	return false
}

// NormalizeEnvType maps a header label to its descriptor type, so ledgers
// written before typed environments read as oci-image/v1.
func NormalizeEnvType(label string) string {
	if label == EnvTypeLegacyContainer {
		return EnvTypeOCIImage
	}
	return label
}

// DecodeVMDiskChain parses and validates a vm-disk-chain/v1 payload.
func DecodeVMDiskChain(payload []byte) (*VMDiskChain, error) {
	dec := json.NewDecoder(bytes.NewReader(payload))
	dec.DisallowUnknownFields()
	var d VMDiskChain
	if err := dec.Decode(&d); err != nil {
		return nil, fmt.Errorf("decoding %s: %w", EnvTypeVMDiskChain, err)
	}
	if err := d.Validate(); err != nil {
		return nil, err
	}
	return &d, nil
}

// Validate checks the fields the vm-disk-chain/v1 schema requires.
func (d *VMDiskChain) Validate() error {
	switch {
	case d.Type != EnvTypeVMDiskChain:
		return fmt.Errorf("descriptor type %q, want %q", d.Type, EnvTypeVMDiskChain)
	case d.Driver == "":
		return errors.New("descriptor: empty driver")
	case d.GuestOS == "":
		return errors.New("descriptor: empty guest_os")
	case d.Arch == "":
		return errors.New("descriptor: empty arch")
	case len(d.Layers) == 0:
		return errors.New("descriptor: no layers")
	}
	for i, l := range d.Layers {
		if err := l.validate(); err != nil {
			return fmt.Errorf("descriptor layer %d (%s): %w", i, l.Name, err)
		}
	}
	for _, s := range d.Seed {
		if s.Name == "" || !sha256Hex.MatchString(s.SHA256) || s.Size < 0 {
			return fmt.Errorf(
				"descriptor seed %q: needs name, sha256 and size", s.Name)
		}
	}
	return nil
}

func (l DiskLayer) validate() error {
	switch l.Role {
	case LayerRoleDisk, LayerRoleKernel, LayerRoleInitrd, LayerRoleNVRAM, LayerRolePlatform:
	default:
		return fmt.Errorf("unknown role %q", l.Role)
	}
	if l.Name == "" {
		return errors.New("empty name")
	}
	if !sha256Hex.MatchString(l.SHA256) {
		return errors.New("sha256 must be 64 lowercase hex digits")
	}
	if l.Size < 0 {
		return errors.New("negative size")
	}
	switch l.Source.Kind {
	case SourceURL:
		if l.Source.URL == "" {
			return errors.New("url source with no url")
		}
	case SourceLocal:
	case SourcePrepared:
		if l.Source.PreparedBy == "" {
			return errors.New("prepared source with no prepared_by")
		}
	default:
		return fmt.Errorf("unknown source kind %q", l.Source.Kind)
	}
	return nil
}

// Marshal encodes the descriptor as its signed payload. The encoding is
// deterministic for a given descriptor (fixed field order, no maps).
func (d *VMDiskChain) Marshal() ([]byte, error) {
	if err := d.Validate(); err != nil {
		return nil, err
	}
	return json.Marshal(d)
}

// NewHash returns a hash for a ledger hash-algorithm name.
func NewHash(name string) (hash.Hash, error) {
	switch name {
	case "blake2b_256":
		return blake2b.New256(nil)
	case "sha256":
		return sha256.New(), nil
	case "sha1":
		return sha1.New(), nil
	case "md5":
		return md5.New(), nil
	}
	return nil, fmt.Errorf("unsupported hash algorithm %q", name)
}

// ComputeHashBlock concatenates the named hashes of data, as a record's hash
// block does.
func ComputeHashBlock(names []string, data []byte) ([]byte, error) {
	var block []byte
	for _, name := range names {
		h, err := NewHash(name)
		if err != nil {
			return nil, err
		}
		h.Write(data)
		block = h.Sum(block)
	}
	return block, nil
}

// EnvironmentRecord returns the close record of the ledger's environment
// record: the first close record on a schema whose URL names "environment".
func EnvironmentRecord(h Header, records []Record) (Record, bool) {
	for _, r := range records {
		if r.Type != RecordClose || int(r.SchemaIndex) >= len(h.Meta.Schemas) {
			continue
		}
		if strings.Contains(h.Meta.Schemas[r.SchemaIndex], "environment") {
			return r, true
		}
	}
	return Record{}, false
}
