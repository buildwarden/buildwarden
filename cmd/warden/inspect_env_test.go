package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fxamacker/cbor/v2"

	"github.com/buildwarden/buildwarden/collector"
	"github.com/buildwarden/buildwarden/driver"
	"github.com/buildwarden/buildwarden/ledger"
	"github.com/buildwarden/buildwarden/relay"
)

// writeEnvLedger writes an output dir holding a ledger whose environment
// record signs payload (header label = label) and, unless landed is false,
// payload itself as environment.json.
func writeEnvLedger(
	t *testing.T, label string, payload, meta []byte, landed bool,
) string {
	t.Helper()
	dir := t.TempDir()
	var buf bytes.Buffer
	var headerEnv map[string]any
	if label != "" {
		headerEnv = map[string]any{"type": label}
	}
	l, err := relay.NewLedger(relay.LedgerConfig{Writer: &buf, Environment: headerEnv})
	if err != nil {
		t.Fatal(err)
	}
	if payload != nil {
		open, _ := cbor.Marshal(map[string]any{"type": "environment"})
		sig := l.Open(5, open)
		l.Close(sig, -int64(len(payload)), l.ComputeHashBlock(payload), 5, meta)
	}
	l.Finish()
	if err := os.WriteFile(filepath.Join(dir, "ledger"), buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	if landed && payload != nil {
		p := filepath.Join(dir, collector.EnvironmentFile)
		if err := os.WriteFile(p, payload, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// diskChainPayload describes one real disk file, so --verify-disks can check it.
func diskChainPayload(t *testing.T) ([]byte, string) {
	t.Helper()
	disk := filepath.Join(t.TempDir(), "base.vhd")
	if err := os.WriteFile(disk, []byte("disk bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	c := &driver.LayerCache{Path: filepath.Join(t.TempDir(), "layers.json")}
	b := driver.NewDiskChain(c, "hyperv", "windows", "amd64", 1)
	if err := b.AddLayer(ledger.LayerRoleDisk, disk, "vhd"); err != nil {
		t.Fatal(err)
	}
	env, err := b.Environment()
	if err != nil {
		t.Fatal(err)
	}
	return env.Payload, disk
}

func inspectDir(t *testing.T, dir string, opts inspectOptions) (string, error) {
	t.Helper()
	var out bytes.Buffer
	opts.Writer = &out
	opts.NoColor = true
	err := runInspectImpl(resolveLedgerPath(dir), opts)
	return out.String(), err
}

func TestInspectEnvironmentDiskChain(t *testing.T) {
	payload, disk := diskChainPayload(t)
	meta, _ := cbor.Marshal(map[string]any{"type": ledger.EnvTypeVMDiskChain})
	dir := writeEnvLedger(t, ledger.EnvTypeVMDiskChain, payload, meta, true)

	out, err := inspectDir(t, dir, inspectOptions{VerifyDisks: true})
	if err != nil {
		t.Fatalf("inspect: %v\n%s", err, out)
	}
	for _, want := range []string{"ENVIRONMENT  vm-disk-chain/v1  payload verified",
		"hyperv windows/amd64 gen1", "base.vhd", "[ok]"} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}

	// The disk changed after the build: --verify-disks fails, plain inspect
	// (which never reads disks) still passes.
	if err := os.WriteFile(disk, []byte("swapped"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err = inspectDir(t, dir, inspectOptions{VerifyDisks: true})
	if err == nil || !strings.Contains(out, "[MISMATCH]") {
		t.Fatalf("swapped disk: err=%v\n%s", err, out)
	}
	if _, err := inspectDir(t, dir, inspectOptions{}); err != nil {
		t.Fatalf("without --verify-disks: %v", err)
	}
}

func TestInspectEnvironmentPayloadStates(t *testing.T) {
	payload, _ := diskChainPayload(t)
	meta, _ := cbor.Marshal(map[string]any{"type": ledger.EnvTypeVMDiskChain})

	t.Run("tampered", func(t *testing.T) {
		dir := writeEnvLedger(t, ledger.EnvTypeVMDiskChain, payload, meta, true)
		tampered := bytes.Replace(payload, []byte(`"hyperv"`), []byte(`"qemu"`), 1)
		p := filepath.Join(dir, collector.EnvironmentFile)
		if err := os.WriteFile(p, tampered, 0o644); err != nil {
			t.Fatal(err)
		}
		out, err := inspectDir(t, dir, inspectOptions{})
		if err == nil || !strings.Contains(out, "payload MISMATCH") {
			t.Fatalf("err=%v\n%s", err, out)
		}
	})
	t.Run("missing", func(t *testing.T) {
		dir := writeEnvLedger(t, ledger.EnvTypeVMDiskChain, payload, meta, false)
		out, err := inspectDir(t, dir, inspectOptions{})
		if err != nil || !strings.Contains(out, "payload missing") {
			t.Fatalf("err=%v\n%s", err, out)
		}
	})
	t.Run("lying label", func(t *testing.T) {
		dir := writeEnvLedger(t, ledger.EnvTypeOCIImage, payload, meta, true)
		out, err := inspectDir(t, dir, inspectOptions{})
		if err != nil || !strings.Contains(out, "disagrees with the signed payload") ||
			!strings.Contains(out, "ENVIRONMENT  vm-disk-chain/v1") {
			t.Fatalf("err=%v\n%s", err, out)
		}
	})
	t.Run("legacy label without record", func(t *testing.T) {
		dir := writeEnvLedger(t, ledger.EnvTypeLegacyContainer, nil, nil, false)
		out, err := inspectDir(t, dir, inspectOptions{})
		want := "ENVIRONMENT  oci-image/v1  payload none"
		if err != nil || !strings.Contains(out, want) {
			t.Fatalf("err=%v\n%s", err, out)
		}
	})
	t.Run("no environment", func(t *testing.T) {
		dir := writeEnvLedger(t, "", nil, nil, false)
		out, err := inspectDir(t, dir, inspectOptions{})
		if err != nil || strings.Contains(out, "ENVIRONMENT") {
			t.Fatalf("err=%v\n%s", err, out)
		}
	})
}

func TestInspectEnvironmentOCIJSON(t *testing.T) {
	manifest := []byte(`{"schemaVersion":2,"mediaType":` +
		`"application/vnd.oci.image.manifest.v1+json","config":{},"layers":[]}`)
	digest := "sha256:" + strings.Repeat("0", 64)
	meta, _ := cbor.Marshal(map[string]any{
		"type": ledger.EnvTypeOCIImage, "reference": "alpine:3.21", "digest": digest,
	})
	dir := writeEnvLedger(t, ledger.EnvTypeOCIImage, manifest, meta, true)

	out, err := inspectDir(t, dir, inspectOptions{JSON: true})
	if err != nil {
		t.Fatalf("inspect: %v\n%s", err, out)
	}
	var report struct {
		Environment envReport `json:"environment"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatal(err)
	}
	e := report.Environment
	if e.Type != ledger.EnvTypeOCIImage || e.Payload != envPayloadVerified ||
		e.Reference != "alpine:3.21" || e.DigestMatch == nil || *e.DigestMatch {
		t.Fatalf("environment = %+v", e)
	}
}
