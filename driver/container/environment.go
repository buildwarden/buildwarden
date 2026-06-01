package container

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/fxamacker/cbor/v2"
	"github.com/lesiw/ctrctl"
)

// writeEnvironment writes environment payload and metadata to the
// ledger volume BEFORE the relay starts. The relay reads these at
// startup and writes the environment record as the first ledger
// entry.
func writeEnvironment(
	image string, ledgerDir string,
) error {
	inspectArgs := append(ctrctl.Cli, "image", "inspect",
		"--format", "{{json .RepoDigests}}", image)
	cmd := exec.Command(inspectArgs[0], inspectArgs[1:]...)
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("inspecting image: %w", err)
	}

	digest := parseDigestFromInspect(
		strings.TrimSpace(string(out)))
	if digest == "" {
		return fmt.Errorf(
			"could not resolve digest for %s", image)
	}

	payload, mediaType := getImageManifest(image)

	envDir := filepath.Join(ledgerDir, "environment")
	_ = os.MkdirAll(envDir, 0755)

	if err := os.WriteFile(
		filepath.Join(envDir, "payload"),
		payload, 0644); err != nil {
		return err
	}

	meta := map[string]any{
		"reference": image,
		"digest":    digest,
		"mediaType": mediaType,
	}
	metaBytes, err := cbor.Marshal(meta)
	if err != nil {
		return fmt.Errorf(
			"encoding environment metadata: %w", err)
	}
	if err := os.WriteFile(
		filepath.Join(envDir, "metadata"),
		metaBytes, 0644); err != nil {
		return err
	}

	return nil
}

// getImageManifest retrieves the raw OCI manifest from the
// container runtime. Tries multiple extraction methods for
// runtime compatibility:
//  1. `manifest inspect` (docker, podman)
//  2. `image inspect --mode=native` .Manifest (nerdctl, finch)
//  3. Falls back to image config (deterministic but not
//     registry-fetchable)
func getImageManifest(
	image string,
) ([]byte, string) {
	mediaType :=
		"application/vnd.oci.image.manifest.v1+json"

	// docker/podman: manifest inspect
	args := append(ctrctl.Cli,
		"manifest", "inspect", image)
	cmd := exec.Command(args[0], args[1:]...)
	if out, err := cmd.Output(); err == nil &&
		len(out) > 0 {
		return out, mediaType
	}

	// nerdctl/finch: native mode inspect has .Manifest field
	args = append(
		ctrctl.Cli[:len(ctrctl.Cli):len(ctrctl.Cli)],
		"image", "inspect", "--mode=native", image)
	cmd = exec.Command(args[0], args[1:]...)
	if out, err := cmd.Output(); err == nil {
		if m := extractNativeManifest(out); m != nil {
			return m, mediaType
		}
	}

	// Fallback: full image inspect (runtime-specific).
	args = append(
		ctrctl.Cli[:len(ctrctl.Cli):len(ctrctl.Cli)],
		"image", "inspect", "--format", "{{json .}}",
		image)
	cmd = exec.Command(args[0], args[1:]...)
	if out, err := cmd.Output(); err == nil {
		return out,
			"application/vnd.buildwarden." +
				"image-inspect.v1+json"
	}

	return []byte("{}"), ""
}

func extractNativeManifest(inspectOutput []byte) []byte {
	var images []map[string]json.RawMessage
	if err := json.Unmarshal(
		inspectOutput, &images); err != nil {
		return nil
	}
	if len(images) == 0 {
		return nil
	}
	raw, ok := images[0]["Manifest"]
	if !ok || len(raw) == 0 {
		return nil
	}
	var manifest any
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return nil
	}
	compact, err := json.Marshal(manifest)
	if err != nil {
		return nil
	}
	return compact
}

func parseDigestFromInspect(output string) string {
	output = strings.Trim(output, "[]")
	if idx := strings.Index(output, "sha256:"); idx >= 0 {
		digest := output[idx:]
		if end := strings.IndexAny(
			digest, " \t\n\r]\"'"); end >= 0 {
			digest = digest[:end]
		}
		// "sha256:" + 64 hex chars
		if len(digest) == 71 {
			return digest
		}
	}
	return ""
}
