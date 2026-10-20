package relay

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fxamacker/cbor/v2"

	"github.com/buildwarden/buildwarden/collector"
	"github.com/buildwarden/buildwarden/ledger"
	"github.com/buildwarden/buildwarden/relaycfg"
)

const testDiskChain = `{"type":"vm-disk-chain/v1","driver":"qemu","guest_os":"linux",` +
	`"arch":"amd64","layers":[{"role":"disk","name":"a.qcow2","sha256":` +
	`"0000000000000000000000000000000000000000000000000000000000000000",` +
	`"size":1,"source":{"kind":"local"}}]}`

func TestEnvironmentLabel(t *testing.T) {
	manifest := []byte(`{"schemaVersion":2,"mediaType":` +
		`"application/vnd.oci.image.manifest.v1+json","layers":[]}`)
	tests := []struct {
		name    string
		env     *relaycfg.Environment
		want    string
		wantErr string
	}{
		{name: "none", env: nil},
		{name: "oci undeclared", env: &relaycfg.Environment{Payload: manifest},
			want: ledger.EnvTypeOCIImage},
		{name: "oci declared", want: ledger.EnvTypeOCIImage, env: &relaycfg.Environment{
			Type: ledger.EnvTypeOCIImage, Payload: manifest}},
		{name: "disk chain", want: ledger.EnvTypeVMDiskChain, env: &relaycfg.Environment{
			Type: ledger.EnvTypeVMDiskChain, Payload: []byte(testDiskChain)}},
		{name: "mismatch", wantErr: "declared as", env: &relaycfg.Environment{
			Type: ledger.EnvTypeVMDiskChain, Payload: manifest}},
		{name: "empty", wantErr: "empty", env: &relaycfg.Environment{}},
		{name: "untyped", wantErr: "names no type",
			env: &relaycfg.Environment{Payload: []byte(`{"a":1}`)}},
		{name: "oversize", wantErr: "limit", env: &relaycfg.Environment{
			Payload: bytes.Repeat([]byte(" "), maxEnvironmentBytes+1)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := environmentLabel(tt.env)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if tt.want == "" {
				if got != nil {
					t.Fatalf("label = %v, want none", got)
				}
				return
			}
			if got["type"] != tt.want {
				t.Fatalf("label = %v, want type %q", got, tt.want)
			}
		})
	}
}

// TestRelayRecordsEnvironment checks the header label, that the payload lands
// with the collector, and that it hashes to the signed environment record.
func TestRelayRecordsEnvironment(t *testing.T) {
	meta, _ := cbor.Marshal(map[string]any{"type": ledger.EnvTypeVMDiskChain})
	payload := []byte(testDiskChain)
	r, _, _, out := startTestRelay(t, collector.Config{}, "", func(c *Config) {
		c.Environment = &relaycfg.Environment{
			Type: ledger.EnvTypeVMDiskChain, Payload: payload, Metadata: meta}
	})
	r.Stop()

	landed, err := os.ReadFile(filepath.Join(out, collector.EnvironmentFile))
	if err != nil || !bytes.Equal(landed, payload) {
		t.Fatalf("environment.json = %q, %v", landed, err)
	}
	data, err := os.ReadFile(filepath.Join(out, "ledger"))
	if err != nil {
		t.Fatal(err)
	}
	res, err := ledger.Verify(data)
	if err != nil || !res.Valid {
		t.Fatalf("verify: %v (%+v)", err, res)
	}
	if got := res.Header.Meta.Environment["type"]; got != ledger.EnvTypeVMDiskChain {
		t.Fatalf("header environment = %v", res.Header.Meta.Environment)
	}
	rec, ok := ledger.EnvironmentRecord(res.Header, res.Records)
	if !ok {
		t.Fatal("no environment record")
	}
	want, _ := ledger.ComputeHashBlock(res.Header.Meta.Hashes, landed)
	if !bytes.Equal(rec.HashBlock, want) || rec.AbsPayloadSize() != int64(len(payload)) {
		t.Fatal("landed payload does not match the signed environment record")
	}
}

func TestRelayNoEnvironmentNoLabel(t *testing.T) {
	r, _, _, out := startTestRelay(t, collector.Config{}, "")
	r.Stop()
	data, err := os.ReadFile(filepath.Join(out, "ledger"))
	if err != nil {
		t.Fatal(err)
	}
	res, err := ledger.Verify(data)
	if err != nil {
		t.Fatal(err)
	}
	if res.Header.Meta.Environment != nil {
		t.Fatalf("header environment = %v, want none", res.Header.Meta.Environment)
	}
	if _, err := os.Stat(filepath.Join(out, collector.EnvironmentFile)); !os.IsNotExist(err) {
		t.Fatalf("environment.json written without an environment (err=%v)", err)
	}
}
