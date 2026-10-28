package container

import (
	"strings"
	"testing"

	"github.com/fxamacker/cbor/v2"

	"github.com/buildwarden/buildwarden/ledger"
)

func TestOCIEnvironment(t *testing.T) {
	const mt = "application/vnd.oci.image.manifest.v1+json"
	manifest := []byte(`{"schemaVersion":2,"mediaType":"` + mt + `",` +
		`"config":{},"layers":[]}`)
	digest := "sha256:" + strings.Repeat("ab", 32)

	env, err := ociEnvironment("alpine:3.21", digest, mt, manifest)
	if err != nil {
		t.Fatal(err)
	}
	if env.Type != ledger.EnvTypeOCIImage || string(env.Payload) != string(manifest) {
		t.Fatalf("env = %+v; payload must stay the manifest bytes", env)
	}
	var meta map[string]any
	if err := cbor.Unmarshal(env.Metadata, &meta); err != nil {
		t.Fatal(err)
	}
	if meta["type"] != ledger.EnvTypeOCIImage || meta["digest"] != digest ||
		meta["reference"] != "alpine:3.21" {
		t.Fatalf("metadata = %v", meta)
	}

	// Runtime-inspect fallbacks are not manifests and name no type.
	for _, bad := range []string{`{}`, `[{"Id":"sha256:x"}]`, `{"Id":"x","Config":{}}`} {
		if _, err := ociEnvironment("x", digest, "", []byte(bad)); err == nil {
			t.Errorf("payload %s accepted as an image manifest", bad)
		}
	}
}
