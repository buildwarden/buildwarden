package ledger

import (
	"encoding/hex"
	"strings"
	"testing"
)

const (
	dockerList = `{"schemaVersion":2,"mediaType":` +
		`"application/vnd.docker.distribution.manifest.list.v2+json","manifests":[]}`
	ociIndex = `{"schemaVersion":2,"mediaType":"application/vnd.oci.image.index.v1+json"}`
)

func TestEnvironmentType(t *testing.T) {
	tests := []struct {
		name, payload, want, wantErr string
	}{
		{name: "descriptor", payload: `{"type":"vm-disk-chain/v1"}`,
			want: EnvTypeVMDiskChain},
		{name: "future type", payload: `{"type":"firecracker-rootfs/v1"}`,
			want: "firecracker-rootfs/v1"},
		{name: "oci manifest", want: EnvTypeOCIImage,
			payload: `{"schemaVersion":2,"config":{},"layers":[]}`},
		{name: "docker manifest list", want: EnvTypeOCIImage, payload: dockerList},
		{name: "oci index by media type", want: EnvTypeOCIImage, payload: ociIndex},
		{name: "inspect fallback", payload: `[{"Id":"x"}]`, wantErr: "not a JSON object"},
		{name: "empty object", payload: `{}`, wantErr: "names no type"},
		{name: "schemaVersion alone", payload: `{"schemaVersion":2}`,
			wantErr: "names no type"},
		{name: "non-string type", payload: `{"type":3}`, wantErr: "not a string"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := EnvironmentType([]byte(tt.payload))
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Fatalf("got %q, %v; want %q", got, err, tt.want)
			}
		})
	}
}

func hexOf(c string) string { return strings.Repeat(c, 64) }

func validChain() VMDiskChain {
	eval := DiskLayer{
		Role: LayerRoleDisk, Name: "eval.vhd", Format: "vhd", SHA256: hexOf("a"), Size: 10,
		Source: LayerSource{Kind: SourceURL, URL: "https://x/eval.vhd", Pinned: true},
	}
	prepared := DiskLayer{
		Role: LayerRoleDisk, Name: "prepared.vhd", Format: "vhd",
		SHA256: hexOf("b"), Size: 5,
		Source: LayerSource{
			Kind: SourcePrepared, PreparedBy: "warden 1.0",
			Inputs: []PreparationInput{{Name: "prepare.ps1", SHA256: hexOf("c")}},
		},
	}
	return VMDiskChain{
		Type: EnvTypeVMDiskChain, Driver: "hyperv", GuestOS: "windows", Arch: "amd64",
		Generation: 1,
		Layers:     []DiskLayer{eval, prepared},
		Seed:       []SeedFile{{Name: "warden-io.exe", SHA256: hexOf("d"), Size: 1}},
	}
}

func TestVMDiskChainRoundTrip(t *testing.T) {
	d := validChain()
	payload, err := d.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if typ, _ := EnvironmentType(payload); typ != EnvTypeVMDiskChain {
		t.Fatalf("payload names %q", typ)
	}
	got, err := DecodeVMDiskChain(payload)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Layers) != 2 || got.Layers[1].Source.Inputs[0].Name != "prepare.ps1" {
		t.Fatalf("round trip = %+v", got)
	}
	again, _ := got.Marshal()
	if string(again) != string(payload) {
		t.Fatal("encoding is not deterministic")
	}
}

func TestVMDiskChainValidate(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*VMDiskChain)
		wantErr string
	}{
		{"wrong type", func(d *VMDiskChain) { d.Type = "oci-image/v1" }, "want"},
		{"no driver", func(d *VMDiskChain) { d.Driver = "" }, "driver"},
		{"no layers", func(d *VMDiskChain) { d.Layers = nil }, "no layers"},
		{"bad role", func(d *VMDiskChain) { d.Layers[0].Role = "swap" }, "role"},
		{"bad sha", func(d *VMDiskChain) { d.Layers[0].SHA256 = "ABC" }, "sha256"},
		{"url without url", func(d *VMDiskChain) {
			d.Layers[0].Source.URL = ""
		}, "no url"},
		{"prepared without tool", func(d *VMDiskChain) {
			d.Layers[1].Source.PreparedBy = ""
		}, "prepared_by"},
		{"bad source", func(d *VMDiskChain) {
			d.Layers[0].Source.Kind = "magic"
		}, "source kind"},
		{"bad seed", func(d *VMDiskChain) { d.Seed[0].SHA256 = "" }, "seed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := validChain()
			tt.mutate(&d)
			err := d.Validate()
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("err = %v, want %q", err, tt.wantErr)
			}
		})
	}
	extra := []byte(`{"type":"vm-disk-chain/v1","extra":1}`)
	if _, err := DecodeVMDiskChain(extra); err == nil {
		t.Fatal("unknown fields accepted")
	}
}

func TestComputeHashBlock(t *testing.T) {
	block, err := ComputeHashBlock([]string{"sha256", "md5"}, []byte("abc"))
	if err != nil {
		t.Fatal(err)
	}
	want := "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad" +
		"900150983cd24fb0d6963f7d28e17f72"
	if hex.EncodeToString(block) != want {
		t.Fatalf("block = %x", block)
	}
	if _, err := ComputeHashBlock([]string{"crc32"}, nil); err == nil {
		t.Fatal("unknown algorithm accepted")
	}
}
