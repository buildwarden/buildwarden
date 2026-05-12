package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/fxamacker/cbor/v2"
	"github.com/lesiw/ctrctl"
)

// ScriptEnv implements BuildEnv using direct container exec instead of DinD.
// The Dockerfile is translated to a shell script and executed inside an
// unprivileged container. Network isolation is enforced by container runtime
// topology: the build container is on an isolated bridge with the relay as
// its sole gateway. No iptables, no CAP_NET_ADMIN, no privileged mode.
type ScriptEnv struct {
	buildConfig    *BuildConfig
	buildContainer string
	relayContainer string
	relayImage     string
	relayBuiltLocal bool
	buildId        string
	ledgerDir      string
	outputDir      string
	subnet         buildSubnet
	isolatedNetwork string
}

func NewScriptEnv() BuildEnv {
	return &ScriptEnv{}
}

func (s *ScriptEnv) Build(config *BuildConfig) error {
	return s.inBuildEnv(config, func() error {
		ctrfilePath := filepath.Join(s.wardenDirPath(), "Containerfile")
		result, err := dockerfileToScript(ctrfilePath)
		if err != nil {
			return fmt.Errorf("translating dockerfile: %w", err)
		}

		scriptPath := filepath.Join(s.wardenDirPath(), "build.sh")
		if err := os.WriteFile(scriptPath, []byte(result.Script), 0755); err != nil {
			return fmt.Errorf("writing build script: %w", err)
		}

		log.Build("Starting build container...")
		if err := s.startBuildContainer(result.Image); err != nil {
			return err
		}
		if err := s.isolateBuildContainer(); err != nil {
			return err
		}
		if err := s.injectWarden(); err != nil {
			return err
		}

		log.Build("Executing build...")
		_, err = ctrctl.ContainerExec(
			&ctrctl.ContainerExecOpts{
				Cmd: &exec.Cmd{
					Stdin:  os.Stdin,
					Stdout: os.Stdout,
					Stderr: os.Stderr,
				},
				Interactive: true,
			},
			s.buildContainer,
			"sh", "/.warden/build.sh",
		)
		if err == nil {
			log.Success("Build complete")
		}
		return err
	})
}

func (s *ScriptEnv) Shell(config *BuildConfig) error {
	return s.inBuildEnv(config, func() error {
		ctrfilePath := filepath.Join(s.wardenDirPath(), "Containerfile")
		result, err := dockerfileToScript(ctrfilePath)
		if err != nil {
			return fmt.Errorf("translating dockerfile: %w", err)
		}

		if err := s.startBuildContainer(result.Image); err != nil {
			return err
		}
		if err := s.isolateBuildContainer(); err != nil {
			return err
		}
		if err := s.injectWarden(); err != nil {
			return err
		}

		log.Info("Dropping into shell (exit to tear down)...")
		_, err = ctrctl.ContainerExec(
			&ctrctl.ContainerExecOpts{
				Cmd: &exec.Cmd{
					Stdin:  os.Stdin,
					Stdout: os.Stdout,
					Stderr: os.Stderr,
				},
				Interactive: true,
				Tty:         true,
			},
			s.buildContainer,
			"sh",
		)
		return err
	})
}

func (s *ScriptEnv) inBuildEnv(config *BuildConfig, fn func() error) error {
	s.buildConfig = config
	defer s.teardown()

	if err := s.setup(); err != nil {
		return err
	}
	if err := fn(); err != nil {
		return fmt.Errorf("build error: %w", err)
	}
	return nil
}

func (s *ScriptEnv) setup() error {
	err := os.MkdirAll(filepath.Join(s.wardenDirPath(), "ext.d"), 0755)
	if err != nil {
		return fmt.Errorf("error creating %s: %w", s.wardenDirPath(), err)
	}

	if err := s.buildWardenIO(); err != nil {
		return err
	}

	s.buildId = randanum(8)
	log.Info(fmt.Sprintf("Build ID: %s", s.buildId))

	s.outputDir = s.buildConfig.OutputDir
	if s.outputDir == "" {
		s.outputDir = "warden-output"
	}
	if err = os.MkdirAll(s.outputDir, 0755); err != nil {
		return fmt.Errorf("error creating output dir: %w", err)
	}

	s.ledgerDir, err = os.MkdirTemp("", "warden-ledger-"+s.buildId+"-")
	if err != nil {
		return fmt.Errorf("error creating ledger temp dir: %w", err)
	}

	if err := s.resolveRelayImage(); err != nil {
		return err
	}

	// Pull the base image and write environment record files BEFORE
	// starting the relay. The relay reads these at startup and records
	// the environment as the first ledger entry.
	if err := s.prepareEnvironment(); err != nil {
		return err
	}

	log.Info("Allocating network...")
	sub, err := allocateSubnet()
	if err != nil {
		return err
	}
	s.subnet = sub

	if err := s.createNetwork(); err != nil {
		return err
	}

	log.Info("Starting relay...")
	if err := s.startRelayContainer(); err != nil {
		return err
	}

	for _, ext := range exts {
		if err := ext.BeforeBuild(s.ctrEnvCompat()); err != nil {
			return err
		}
	}

	if err := s.editContainerfile(); err != nil {
		return err
	}

	return nil
}

func (s *ScriptEnv) prepareEnvironment() error {
	image, err := extractFromImage(s.buildConfig.Containerfile)
	if err != nil {
		return fmt.Errorf("reading FROM image: %w", err)
	}

	log.Info(fmt.Sprintf("Pulling image: %s", image))
	if err := s.pullBuildBaseImage(image); err != nil {
		return err
	}

	if err := s.writeEnvironment(image); err != nil {
		log.Warn(fmt.Sprintf("failed to write environment: %s", err))
	}
	return nil
}

func (s *ScriptEnv) pullBuildBaseImage(image string) error {
	args := append(ctrctl.Cli, "pull", image)
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("error pulling base image %s: %w", image, err)
	}
	return nil
}

// writeEnvironment writes environment payload + metadata to the ledger
// volume BEFORE the relay starts. The relay reads these at startup and
// writes the environment record as the first ledger entry.
func (s *ScriptEnv) writeEnvironment(image string) error {
	inspectArgs := append(ctrctl.Cli, "image", "inspect",
		"--format", "{{json .RepoDigests}}", image)
	cmd := exec.Command(inspectArgs[0], inspectArgs[1:]...)
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("inspecting image: %w", err)
	}

	digest := parseDigestFromInspect(strings.TrimSpace(string(out)))
	if digest == "" {
		return fmt.Errorf("could not resolve digest for %s", image)
	}

	payload, mediaType := s.getImageManifest(image)

	envDir := filepath.Join(s.ledgerDir, "environment")
	_ = os.MkdirAll(envDir, 0755)

	if err := os.WriteFile(
		filepath.Join(envDir, "payload"), payload, 0644); err != nil {
		return err
	}

	// CBOR-encode the metadata.
	meta := map[string]any{
		"reference": image,
		"digest":    digest,
		"mediaType": mediaType,
	}
	metaBytes, err := cbor.Marshal(meta)
	if err != nil {
		return fmt.Errorf("encoding environment metadata: %w", err)
	}
	if err := os.WriteFile(
		filepath.Join(envDir, "metadata"), metaBytes, 0644); err != nil {
		return err
	}

	return nil
}

// getImageManifest retrieves the raw OCI manifest from the container
// runtime. The runtime already pulled it and handles private registry
// auth, so this needs no extra network access. The raw manifest bytes
// are externally verifiable: sha256(manifest) == the image digest.
//
// Tries multiple extraction methods for runtime compatibility:
//  1. `manifest inspect` (docker, podman)
//  2. `image inspect --mode=native` .Manifest (nerdctl, finch)
//  3. Falls back to image config (deterministic but not registry-fetchable)
func (s *ScriptEnv) getImageManifest(image string) ([]byte, string) {
	mediaType := "application/vnd.oci.image.manifest.v1+json"

	// docker/podman: manifest inspect
	args := append(ctrctl.Cli, "manifest", "inspect", image)
	cmd := exec.Command(args[0], args[1:]...)
	if out, err := cmd.Output(); err == nil && len(out) > 0 {
		return out, mediaType
	}

	// nerdctl/finch: native mode inspect has .Manifest field
	args = append(ctrctl.Cli[:len(ctrctl.Cli):len(ctrctl.Cli)],
		"image", "inspect", "--mode=native", image)
	cmd = exec.Command(args[0], args[1:]...)
	if out, err := cmd.Output(); err == nil {
		if m := extractNativeManifest(out); m != nil {
			return m, mediaType
		}
	}

	// Fallback: full image inspect (runtime-specific).
	args = append(ctrctl.Cli[:len(ctrctl.Cli):len(ctrctl.Cli)],
		"image", "inspect", "--format", "{{json .}}", image)
	cmd = exec.Command(args[0], args[1:]...)
	if out, err := cmd.Output(); err == nil {
		return out, "application/vnd.buildwarden.image-inspect.v1+json"
	}

	return []byte("{}"), ""
}

func extractNativeManifest(inspectOutput []byte) []byte {
	var images []map[string]json.RawMessage
	if err := json.Unmarshal(inspectOutput, &images); err != nil {
		return nil
	}
	if len(images) == 0 {
		return nil
	}
	raw, ok := images[0]["Manifest"]
	if !ok || len(raw) == 0 {
		return nil
	}
	// Re-marshal compactly to get canonical JSON.
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
	// Format varies: "[repo@sha256:abc...]" or "repo@sha256:abc..."
	output = strings.Trim(output, "[]")
	if idx := strings.Index(output, "sha256:"); idx >= 0 {
		digest := output[idx:]
		// Trim anything after the hash (spaces, quotes, etc.)
		if end := strings.IndexAny(digest, " \t\n\r]\"'"); end >= 0 {
			digest = digest[:end]
		}
		if len(digest) == 71 { // "sha256:" + 64 hex chars
			return digest
		}
	}
	return ""
}

func (s *ScriptEnv) startBuildContainer(image string) error {
	// Start unprivileged — no CAP_NET_ADMIN. Network isolation is
	// applied externally via a sidecar container sharing the network
	// namespace (see isolateBuildContainer). The build container cannot
	// modify or flush the iptables rules.
	args := append(ctrctl.Cli, "container", "run",
		"--detach",
		"--name", "warden-build-"+s.buildId,
		"--network", "warden-"+s.buildId,
		"--dns", s.subnet.relayIP,
		"--add-host", "artifacts:"+s.subnet.relayIP,
		"--add-host", "cwd:"+s.subnet.relayIP,
		"--workdir", "/work",
		image,
		"sleep", "infinity",
	)
	cmd := exec.Command(args[0], args[1:]...)
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("error starting build container: %w", err)
	}
	s.buildContainer = strings.TrimSpace(string(out))
	return nil
}

// Network namespace isolation uses a pattern based on Kubernetes init
// containers (stable since Kubernetes 1.6): a short-lived privileged
// container shares the pod/build container's network namespace to apply
// iptables rules, then exits. The rules persist in the namespace because
// they belong to the namespace, not to any specific container process.
// The build container has no CAP_NET_ADMIN so it cannot modify the rules.
//
// Reference: https://kubernetes.io/docs/concepts/workloads/pods/init-containers/
// The --network=container:<name> flag is the container-runtime equivalent
// of Kubernetes pod-level network namespace sharing.

const netnsDockerfile = `FROM alpine:3.20
RUN apk add --no-cache iptables
ENTRYPOINT ["sh", "-c"]
`

const netnsImageTag = "warden-netns:alpine3.20"

// ensureNetnsImage builds the warden-netns image if it doesn't already
// exist. The Dockerfile is deterministic, so the container runtime
// deduplicates by instruction hash across all builds on this host.
func ensureNetnsImage() error {
	args := append(ctrctl.Cli, "image", "inspect", netnsImageTag)
	cmd := exec.Command(args[0], args[1:]...)
	if cmd.Run() == nil {
		return nil
	}

	buildCtx, err := os.MkdirTemp("", "warden-netns-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(buildCtx)

	if err := os.WriteFile(
		filepath.Join(buildCtx, "Dockerfile"),
		[]byte(netnsDockerfile), 0644); err != nil {
		return err
	}

	_, err = ctrctl.ImageBuild(
		&ctrctl.ImageBuildOpts{Tag: netnsImageTag},
		buildCtx, "")
	return err
}

// isolateBuildContainer applies iptables rules to force all traffic
// through the relay. Uses the warden-netns image sharing the build
// container's network namespace. After this returns, the build
// container's network is locked down — it has no CAP_NET_ADMIN to
// modify or flush the rules.
func (s *ScriptEnv) isolateBuildContainer() error {
	if err := ensureNetnsImage(); err != nil {
		return fmt.Errorf("building netns image: %w", err)
	}

	script := fmt.Sprintf(`set -e
iptables -t nat -A OUTPUT -p udp --dport 53 -j DNAT --to-destination %[1]s:53
iptables -t nat -A OUTPUT -p tcp --dport 53 -j DNAT --to-destination %[1]s:53
iptables -t nat -A OUTPUT -p tcp --dport 80 -j DNAT --to-destination %[1]s:80
iptables -t nat -A OUTPUT -p tcp --dport 443 -j DNAT --to-destination %[1]s:443
iptables -A OUTPUT -d %[1]s -j ACCEPT
iptables -A OUTPUT -d 127.0.0.0/8 -j ACCEPT
iptables -A OUTPUT -j DROP
`, s.subnet.relayIP)

	args := append(ctrctl.Cli, "container", "run",
		"--rm",
		"--network", "container:"+s.buildContainer,
		"--privileged",
		netnsImageTag,
		script,
	)
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("network isolation failed: %w", err)
	}
	return nil
}

func (s *ScriptEnv) injectWarden() error {
	_, err := ctrctl.ContainerCp(
		nil,
		s.wardenDirPath()+"/.",
		fmt.Sprintf("%s:/.warden", s.buildContainer),
	)
	if err != nil {
		return fmt.Errorf("error copying .warden into container: %w", err)
	}

	// Symlink warden-io to PATH and run extension scripts.
	cmds := [][]string{
		{"ln", "-sf", "/.warden/warden-io", "/usr/local/bin/warden-io"},
		{"sh", "-c", "for f in /.warden/ext.d/*.sh; do [ -f \"$f\" ] && sh \"$f\"; done"},
	}
	for _, cmd := range cmds {
		_, err := ctrctl.ContainerExec(nil, s.buildContainer, cmd[0], cmd[1:]...)
		if err != nil {
			return fmt.Errorf("error running setup command %v: %w", cmd, err)
		}
	}
	return nil
}

func (s *ScriptEnv) createNetwork() error {
	// Create a bridge network for the build. The build container is
	// unprivileged (no CAP_NET_ADMIN) and uses the relay as its DNS
	// server, so all name resolution goes through the relay.
	//
	// NOTE: True network isolation (--internal) requires runtime support
	// for multi-homing (so the relay can reach the internet). Finch/nerdctl
	// doesn't support `network connect`. For runtimes that do, this should
	// be upgraded to an internal network.
	id, err := ctrctl.NetworkCreate(
		&ctrctl.NetworkCreateOpts{
			Driver: "bridge",
			Subnet: s.subnet.cidr,
		},
		"warden-"+s.buildId,
	)
	if err != nil {
		return err
	}
	s.isolatedNetwork = id
	return nil
}

func (s *ScriptEnv) startRelayContainer() error {
	args := append(ctrctl.Cli, "container", "run",
		"--detach",
		"--name", "warden-relay-"+s.buildId,
		"--network", "warden-"+s.buildId,
		"--ip", s.subnet.relayIP,
		"--volume", s.ledgerDir+":/ledger",
		"--volume", s.buildConfig.Context+":/context:ro",
	)
	if s.buildConfig.Capture != "" && s.buildConfig.Capture != "none" {
		args = append(args, "--env", "CAPTURE_MODE="+s.buildConfig.Capture)
	}
	args = append(args, s.relayImage)
	cmd := exec.Command(args[0], args[1:]...)
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("error starting relay container: %w", err)
	}
	s.relayContainer = strings.TrimSpace(string(out))
	return nil
}

func (s *ScriptEnv) resolveRelayImage() error {
	img := s.buildConfig.RelayImage
	switch {
	case img == "dev" || (img == "" && version == "dev"):
		log.Info("Building relay from source...")
		s.relayBuiltLocal = true
		return s.buildRelayFromSource()
	case img != "":
		log.Info(fmt.Sprintf("Using relay image: %s", img))
		s.relayImage = img
		return s.pullImage(img)
	default:
		s.relayImage = relayImageRepo + ":latest"
		log.Info(fmt.Sprintf("Pulling relay image: %s", s.relayImage))
		return s.pullImage(s.relayImage)
	}
}

func (s *ScriptEnv) pullImage(image string) error {
	args := append(ctrctl.Cli, "pull", image)
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("error pulling image %s: %w", image, err)
	}
	return nil
}

func (s *ScriptEnv) buildRelayFromSource() error {
	relayBin := filepath.Join(os.TempDir(), "warden-relay-"+s.buildId)
	cmd := exec.Command("go", "build", "-o", relayBin, "./cmd/relay")
	cmd.Env = append(os.Environ(), "GOOS=linux", "CGO_ENABLED=0")
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("error cross-compiling relay: %w", err)
	}
	defer os.Remove(relayBin)

	buildCtx, err := os.MkdirTemp("", "warden-relay-ctx-")
	if err != nil {
		return fmt.Errorf("error creating relay build context: %w", err)
	}
	defer os.RemoveAll(buildCtx)

	binData, err := os.ReadFile(relayBin)
	if err != nil {
		return fmt.Errorf("error reading relay binary: %w", err)
	}
	if err := os.WriteFile(
		filepath.Join(buildCtx, "relay"), binData, 0755); err != nil {
		return fmt.Errorf("error writing relay to context: %w", err)
	}

	dockerfile := `FROM alpine:3.20
RUN apk add --no-cache ca-certificates
COPY relay /usr/local/bin/relay
EXPOSE 53/udp 80 443
ENTRYPOINT ["relay"]
`
	if err := os.WriteFile(
		filepath.Join(buildCtx, "Dockerfile"),
		[]byte(dockerfile), 0644); err != nil {
		return fmt.Errorf("error writing relay Dockerfile: %w", err)
	}

	s.relayImage = "warden-relay:" + s.buildId
	_, err = ctrctl.ImageBuild(
		&ctrctl.ImageBuildOpts{Tag: s.relayImage},
		buildCtx,
		"",
	)
	if err != nil {
		return fmt.Errorf("error building relay image: %w", err)
	}
	return nil
}

func (s *ScriptEnv) buildWardenIO() error {
	dest := filepath.Join(s.wardenDirPath(), "warden-io")
	if version != "dev" {
		exe, err := os.Executable()
		if err == nil {
			candidate := filepath.Join(filepath.Dir(exe), "warden-io")
			if data, err := os.ReadFile(candidate); err == nil {
				return os.WriteFile(dest, data, 0755)
			}
		}
	}
	cmd := exec.Command("go", "build", "-o", dest, "./cmd/warden-io")
	cmd.Env = append(os.Environ(), "GOOS=linux", "CGO_ENABLED=0")
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("error cross-compiling warden-io: %w", err)
	}
	return nil
}

func (s *ScriptEnv) editContainerfile() error {
	// Reuse the existing CtrEnv's editContainerfile logic via a compat shim.
	return s.ctrEnvCompat().editContainerfile()
}

// ctrEnvCompat creates a CtrEnv with enough fields set to reuse its
// editContainerfile and extension methods.
func (s *ScriptEnv) ctrEnvCompat() *CtrEnv {
	return &CtrEnv{
		buildConfig: s.buildConfig,
		buildId:     s.buildId,
		ledgerDir:   s.ledgerDir,
		subnet:      s.subnet,
	}
}

func (s *ScriptEnv) wardenDirPath() string {
	return filepath.Join(s.buildConfig.Context, wardenDir)
}

func (s *ScriptEnv) teardown() {
	log.Info("Tearing down environment...")

	if s.relayContainer != "" && s.outputDir != "" {
		s.collectRelayLogs()
	}
	if s.ledgerDir != "" && s.outputDir != "" {
		s.collectOutput()
	}

	_ = os.RemoveAll(s.wardenDirPath())

	if s.relayContainer != "" {
		_, err := ctrctl.ContainerRm(
			&ctrctl.ContainerRmOpts{Force: true}, s.relayContainer)
		if err != nil {
			log.Warn(fmt.Sprintf("container cleanup: %s", err))
		}
	}
	if s.buildContainer != "" {
		_, err := ctrctl.ContainerRm(
			&ctrctl.ContainerRmOpts{Force: true, Volumes: true},
			s.buildContainer)
		if err != nil {
			log.Warn(fmt.Sprintf("container cleanup: %s", err))
		}
	}
	if s.relayBuiltLocal && s.relayImage != "" {
		_, _ = ctrctl.ImageRm(nil, s.relayImage)
	}
	if s.isolatedNetwork != "" {
		_, err := ctrctl.NetworkRm(nil, s.isolatedNetwork)
		if err != nil {
			log.Warn(fmt.Sprintf("network cleanup: %s", err))
		}
	}
	if s.outputDir != "" {
		log.Result(fmt.Sprintf("Output: %s", s.outputDir))
	}
}

func (s *ScriptEnv) collectRelayLogs() {
	logsArgs := append(ctrctl.Cli, "logs", s.relayContainer)
	cmd := exec.Command(logsArgs[0], logsArgs[1:]...)
	out, err := cmd.CombinedOutput()
	if err != nil || len(out) == 0 {
		return
	}
	logPath := filepath.Join(s.outputDir, "relay.log")
	os.WriteFile(logPath, out, 0644) //nolint:errcheck
}

func (s *ScriptEnv) collectOutput() {
	compat := s.ctrEnvCompat()

	ledgerSrc := filepath.Join(s.ledgerDir, "ledger")
	if s.buildConfig.Compress {
		compat.compressFile(ledgerSrc,
			filepath.Join(s.outputDir, "ledger.zst"))
	} else {
		os.Rename(ledgerSrc, //nolint:errcheck
			filepath.Join(s.outputDir, "ledger"))
	}

	caSrc := filepath.Join(s.ledgerDir, "ca.cert.pem")
	if s.buildConfig.Compress {
		compat.compressFile(caSrc,
			filepath.Join(s.outputDir, "ca.cert.pem.zst"))
	} else {
		os.Rename(caSrc, //nolint:errcheck
			filepath.Join(s.outputDir, "ca.cert.pem"))
	}

	artDir := filepath.Join(s.ledgerDir, "artifacts")
	if entries, err := os.ReadDir(artDir); err == nil {
		outArt := filepath.Join(s.outputDir, "artifacts")
		os.MkdirAll(outArt, 0755) //nolint:errcheck
		for _, entry := range entries {
			src := filepath.Join(artDir, entry.Name())
			real, err := filepath.EvalSymlinks(src)
			if err != nil {
				real = src
			}
			os.Rename(real, //nolint:errcheck
				filepath.Join(outArt, entry.Name()))
		}
	}

	os.RemoveAll(s.ledgerDir) //nolint:errcheck

	if s.buildConfig.Containerfile != "" {
		submitted, err := os.ReadFile(s.buildConfig.Containerfile)
		if err == nil {
			os.WriteFile( //nolint:errcheck
				filepath.Join(s.outputDir, "Dockerfile.submitted"),
				submitted, 0644)
		}
	}

	actual := filepath.Join(s.wardenDirPath(), "Containerfile")
	if data, err := os.ReadFile(actual); err == nil {
		os.WriteFile( //nolint:errcheck
			filepath.Join(s.outputDir, "Dockerfile.actual"),
			data, 0644)
	}

	scriptPath := filepath.Join(s.wardenDirPath(), "build.sh")
	if data, err := os.ReadFile(scriptPath); err == nil {
		os.WriteFile( //nolint:errcheck
			filepath.Join(s.outputDir, "build.sh"),
			data, 0644)
	}
}

// waitForRelay polls the relay until it's responsive.
func (s *ScriptEnv) waitForRelay() error {
	for i := 0; i < 50; i++ {
		_, err := ctrctl.ContainerExec(
			nil, s.relayContainer, "true",
		)
		if err == nil {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("timed out waiting for relay")
}
