package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/fxamacker/cbor/v2"

	"github.com/buildwarden/buildwarden/collector"
	"github.com/buildwarden/buildwarden/driver"
	"github.com/buildwarden/buildwarden/ledger"
)

// Payload verification states of an environment record.
const (
	envPayloadVerified = "verified" // environment.json hashes to the signed record
	envPayloadMismatch = "MISMATCH" // it does not: the file was altered
	envPayloadMissing  = "missing"  // no environment.json beside the ledger
	envPayloadNone     = "none"     // the ledger has no environment record
)

// envReport is what inspect knows and could check about the build
// environment.
type envReport struct {
	// Type is the descriptor type the payload names (falling back to the
	// header label when the payload is unavailable).
	Type string `json:"type"`
	// Label is the header's unsigned environment label.
	Label   string `json:"label,omitempty"`
	Payload string `json:"payload"`
	// Descriptor is the decoded payload (vm-disk-chain/v1 only).
	Descriptor *ledger.VMDiskChain `json:"descriptor,omitempty"`
	// Reference and Digest are oci-image/v1 record metadata (unsigned);
	// DigestMatch reports whether Digest is the payload's own sha256.
	Reference   string `json:"reference,omitempty"`
	Digest      string `json:"digest,omitempty"`
	DigestMatch *bool  `json:"digest_matches_payload,omitempty"`
	// Layers are the --verify-disks results, in descriptor order.
	Layers   []layerCheck `json:"layers,omitempty"`
	Warnings []string     `json:"warnings,omitempty"`
	Failures []string     `json:"failures,omitempty"`
}

type layerCheck struct {
	Name   string `json:"name"`
	Path   string `json:"path"`
	Status string `json:"status"` // ok, MISMATCH, missing, unreadable
}

// checkEnvironment decodes and verifies the ledger's environment record
// against <ledgerDir>/environment.json, and with verifyDisks re-hashes each
// layer the descriptor names. Nil when the ledger says nothing about its
// environment.
func checkEnvironment(
	result *ledger.VerifyResult, ledgerDir string, verifyDisks bool,
) *envReport {
	h := result.Header
	label, _ := h.Meta.Environment["type"].(string)
	rec, hasRec := ledger.EnvironmentRecord(h, result.Records)
	if !hasRec {
		if label == "" {
			return nil
		}
		return &envReport{
			Type: ledger.NormalizeEnvType(label), Label: label,
			Payload: envPayloadNone,
			Warnings: []string{"header names an environment but the ledger records " +
				"none (written before typed environments)"},
		}
	}
	e := &envReport{Type: ledger.NormalizeEnvType(label), Label: label}
	payload, err := os.ReadFile(filepath.Join(ledgerDir, collector.EnvironmentFile))
	if err != nil {
		e.Payload = envPayloadMissing
		return e
	}
	want, err := ledger.ComputeHashBlock(h.Meta.Hashes, payload)
	if err != nil || !bytes.Equal(want, rec.HashBlock) ||
		rec.AbsPayloadSize() != int64(len(payload)) {
		e.Payload = envPayloadMismatch
		e.Failures = append(e.Failures, collector.EnvironmentFile+
			" does not match the signed environment record")
		return e
	}
	e.Payload = envPayloadVerified
	e.decodePayload(payload, rec)
	if verifyDisks && e.Descriptor != nil {
		e.verifyLayers()
	}
	return e
}

// decodePayload takes the type from the verified payload and decodes it.
func (e *envReport) decodePayload(payload []byte, rec ledger.Record) {
	t, err := ledger.EnvironmentType(payload)
	if err != nil {
		e.Failures = append(e.Failures, "environment payload: "+err.Error())
		return
	}
	if e.Label != "" && ledger.NormalizeEnvType(e.Label) != t {
		e.Warnings = append(e.Warnings, fmt.Sprintf(
			"unsigned header label %q disagrees with the signed payload's type %q",
			e.Label, t))
	}
	e.Type = t
	switch t {
	case ledger.EnvTypeVMDiskChain:
		d, err := ledger.DecodeVMDiskChain(payload)
		if err != nil {
			e.Failures = append(e.Failures, err.Error())
			return
		}
		e.Descriptor = d
	case ledger.EnvTypeOCIImage:
		var meta map[string]any
		_ = cbor.Unmarshal(rec.Metadata, &meta)
		e.Reference, _ = meta["reference"].(string)
		e.Digest, _ = meta["digest"].(string)
		if e.Digest != "" {
			sum := sha256.Sum256(payload)
			match := e.Digest == "sha256:"+hex.EncodeToString(sum[:])
			e.DigestMatch = &match
		}
	default:
		e.Warnings = append(e.Warnings, "unknown environment type "+t+
			"; only the payload hash was checked")
	}
}

func (e *envReport) verifyLayers() {
	for _, l := range e.Descriptor.Layers {
		c := layerCheck{Name: l.Name, Path: l.Path}
		switch sum, err := driver.HashFileSHA256(l.Path); {
		case l.Path == "":
			c.Status = "unreadable"
		case os.IsNotExist(err):
			c.Status = "missing"
		case err != nil:
			c.Status = "unreadable"
		case sum != l.SHA256:
			c.Status = "MISMATCH"
			e.Failures = append(e.Failures, fmt.Sprintf(
				"layer %s at %s no longer matches the ledger", l.Name, l.Path))
		default:
			c.Status = "ok"
		}
		e.Layers = append(e.Layers, c)
	}
}

func (e *envReport) failed() bool { return e != nil && len(e.Failures) > 0 }

func printEnvironment(w io.Writer, e *envReport) {
	if e == nil {
		return
	}
	fmt.Fprintf(w, "  ENVIRONMENT  %s  payload %s\n", e.Type, e.Payload)
	if d := e.Descriptor; d != nil {
		gen := ""
		if d.Generation != 0 {
			gen = fmt.Sprintf(" gen%d", d.Generation)
		}
		fmt.Fprintf(w, "    %s %s/%s%s\n", d.Driver, d.GuestOS, d.Arch, gen)
		for i, l := range d.Layers {
			status := ""
			if i < len(e.Layers) {
				status = "  [" + e.Layers[i].Status + "]"
			}
			fmt.Fprintf(w, "    %-8s %s  %s  sha256:%s  %s%s\n",
				l.Role, l.Name, humanBytes(l.Size), shortHex(l.SHA256),
				describeSource(l.Source), status)
		}
		if len(d.Seed) > 0 {
			fmt.Fprintf(w, "    seed     %d per-build files\n", len(d.Seed))
		}
	}
	if e.Reference != "" || e.Digest != "" {
		fmt.Fprintf(w, "    %s %s", e.Reference, e.Digest)
		if e.DigestMatch != nil && !*e.DigestMatch {
			fmt.Fprint(w, " (not the payload's own hash: the runtime re-serialized "+
				"the manifest, or the digest names an index)")
		}
		fmt.Fprintln(w)
	}
	for _, s := range e.Warnings {
		fmt.Fprintf(w, "    warning: %s\n", s)
	}
	for _, s := range e.Failures {
		fmt.Fprintf(w, "    FAIL: %s\n", s)
	}
}

func describeSource(s ledger.LayerSource) string {
	switch s.Kind {
	case ledger.SourceURL:
		if s.Pinned {
			return "url (pinned) " + s.URL
		}
		return "url " + s.URL
	case ledger.SourcePrepared:
		return fmt.Sprintf("prepared by %s from %d input(s)", s.PreparedBy, len(s.Inputs))
	}
	return s.Kind
}

func shortHex(s string) string {
	if len(s) > 12 {
		return s[:12]
	}
	return s
}
