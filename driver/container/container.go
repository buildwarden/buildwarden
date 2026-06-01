// Package container implements the container-based build driver.
// It orchestrates an unprivileged build container with network
// isolation enforced by iptables rules applied via a sidecar
// container sharing the build container's network namespace
// (Kubernetes init container pattern).
package container

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/lesiw/ctrctl"

	"warden/driver"
)

const (
	wardenDir      = ".warden"
	relayImageRepo = "ghcr.io/buildwarden/relay"
)

// Compile-time interface check.
var _ driver.Driver = (*Driver)(nil)

// Driver orchestrates builds inside containers with network
// isolation enforced by iptables.
type Driver struct {
	Runtime    string // CLI name: "finch", "docker", "podman"
	Verbose    bool
	Version    string
	Extensions []driver.Extension
}

// build holds the mutable state for a single build invocation.
type build struct {
	d               *Driver
	req             *driver.BuildRequest
	buildContainer  string
	relayContainer  string
	relayImage      string
	relayBuiltLocal bool
	buildID         string
	ledgerDir       string
	outputDir       string
	subnet          driver.Subnet
	isolatedNetwork string
}

func (d *Driver) Name() string { return "container" }

func (d *Driver) StartBuild(
	ctx context.Context, req *driver.BuildRequest,
) (*driver.BuildResult, error) {
	ctrctl.Cli = []string{d.Runtime}

	b := &build{d: d, req: req}
	defer b.teardown()

	if err := b.setup(); err != nil {
		return nil, err
	}

	ctrfilePath := filepath.Join(
		b.wardenDirPath(), "Containerfile")
	result, err := DockerfileToScript(ctrfilePath)
	if err != nil {
		return nil, fmt.Errorf(
			"translating dockerfile: %w", err)
	}

	// Write build script to context dir — the relay serves it
	// via HTTP and warden-io initialize fetches it at runtime.
	scriptPath := filepath.Join(req.ContextDir, "build.sh")
	if err := os.WriteFile(
		scriptPath, []byte(result.Script), 0755); err != nil {
		return nil, fmt.Errorf(
			"writing build script: %w", err)
	}
	defer os.Remove(scriptPath)

	logInfo(req, "Starting build container...")
	if err := b.startBuildContainer(result.Image); err != nil {
		return nil, err
	}
	if err := b.isolateBuildContainer(); err != nil {
		return nil, err
	}
	if err := b.injectWarden(); err != nil {
		return nil, err
	}

	logInfo(req, "Executing build...")
	stdout := req.Stdout
	if stdout == nil {
		stdout = os.Stdout
	}
	stderr := req.Stderr
	if stderr == nil {
		stderr = os.Stderr
	}
	stdin := req.Stdin
	if stdin == nil {
		stdin = os.Stdin
	}

	_, err = ctrctl.ContainerExec(
		&ctrctl.ContainerExecOpts{
			Cmd: &exec.Cmd{
				Stdin:  stdin,
				Stdout: stdout,
				Stderr: stderr,
			},
			Interactive: true,
		},
		b.buildContainer,
		"warden-io", "initialize",
		"--gateway="+b.subnet.RelayIP,
	)
	if err != nil {
		return nil, fmt.Errorf("build error: %w", err)
	}

	return &driver.BuildResult{
		OutputDir: b.outputDir,
	}, nil
}

func (d *Driver) Exec(
	ctx context.Context, req *driver.BuildRequest,
) error {
	ctrctl.Cli = []string{d.Runtime}

	b := &build{d: d, req: req}
	defer b.teardown()

	if err := b.setup(); err != nil {
		return err
	}

	ctrfilePath := filepath.Join(
		b.wardenDirPath(), "Containerfile")
	result, err := DockerfileToScript(ctrfilePath)
	if err != nil {
		return fmt.Errorf(
			"translating dockerfile: %w", err)
	}

	if err := b.startBuildContainer(result.Image); err != nil {
		return err
	}
	if err := b.isolateBuildContainer(); err != nil {
		return err
	}
	if err := b.injectWarden(); err != nil {
		return err
	}

	logInfo(req, "Dropping into shell (exit to tear down)...")
	stdout := req.Stdout
	if stdout == nil {
		stdout = os.Stdout
	}
	stderr := req.Stderr
	if stderr == nil {
		stderr = os.Stderr
	}
	stdin := req.Stdin
	if stdin == nil {
		stdin = os.Stdin
	}

	_, err = ctrctl.ContainerExec(
		&ctrctl.ContainerExecOpts{
			Cmd: &exec.Cmd{
				Stdin:  stdin,
				Stdout: stdout,
				Stderr: stderr,
			},
			Interactive: true,
			Tty:         true,
		},
		b.buildContainer,
		"sh",
	)
	return err
}

func (d *Driver) Close() error { return nil }

// --- build lifecycle ---

func (b *build) setup() error {
	err := os.MkdirAll(
		filepath.Join(b.wardenDirPath(), "ext.d"), 0755)
	if err != nil {
		return fmt.Errorf(
			"error creating %s: %w",
			b.wardenDirPath(), err)
	}

	if err := b.buildWardenIO(); err != nil {
		return err
	}

	b.buildID = driver.RandAlphaNum(8)
	logInfo(b.req,
		fmt.Sprintf("Build ID: %s", b.buildID))

	b.outputDir = b.req.OutputDir
	if b.outputDir == "" {
		b.outputDir = "warden-output"
	}
	if err = os.MkdirAll(b.outputDir, 0755); err != nil {
		return fmt.Errorf(
			"error creating output dir: %w", err)
	}

	home, _ := os.UserHomeDir()
	b.ledgerDir, err = os.MkdirTemp(home,
		".warden-ledger-"+b.buildID+"-")
	if err != nil {
		return fmt.Errorf(
			"error creating ledger temp dir: %w", err)
	}

	if err := b.resolveRelayImage(); err != nil {
		return err
	}

	if err := b.prepareEnvironment(); err != nil {
		return err
	}

	logInfo(b.req, "Allocating network...")
	sub, err := driver.AllocateSubnet(ctrctl.Cli)
	if err != nil {
		return err
	}
	b.subnet = sub

	if err := b.createNetwork(); err != nil {
		return err
	}

	logInfo(b.req, "Starting relay...")
	if err := b.startRelayContainer(); err != nil {
		return err
	}

	extCtx := &driver.ExtensionContext{
		WardenDir: b.wardenDirPath(),
		ScriptDir: filepath.Join(
			b.wardenDirPath(), "ext.d"),
		LedgerDir: b.ledgerDir,
		RelayIP:   b.subnet.RelayIP,
		BuildID:   b.buildID,
	}
	for _, ext := range b.d.Extensions {
		if err := ext.BeforeBuild(extCtx); err != nil {
			return err
		}
	}

	if err := editContainerfile(
		b.req.Containerfile,
		b.req.ContextDir,
		b.d.Extensions,
	); err != nil {
		return err
	}

	return nil
}

func (b *build) prepareEnvironment() error {
	image, err := driver.ExtractFromImage(
		b.req.Containerfile)
	if err != nil {
		return fmt.Errorf("reading FROM image: %w", err)
	}

	logInfo(b.req,
		fmt.Sprintf("Pulling image: %s", image))
	if err := b.pullBuildBaseImage(image); err != nil {
		return err
	}

	if err := writeEnvironment(
		image, b.ledgerDir); err != nil {
		logWarn(b.req,
			fmt.Sprintf(
				"failed to write environment: %s", err))
	}
	return nil
}

func (b *build) pullBuildBaseImage(image string) error {
	args := append(ctrctl.Cli, "pull", image)
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf(
			"error pulling base image %s: %w",
			image, err)
	}
	return nil
}

func (b *build) startBuildContainer(image string) error {
	args := append(ctrctl.Cli, "container", "run",
		"--detach",
		"--name", "warden-build-"+b.buildID,
		"--network", "warden-"+b.buildID,
		"--dns", b.subnet.RelayIP,
		"--add-host", "artifacts:"+b.subnet.RelayIP,
		"--add-host", "cwd:"+b.subnet.RelayIP,
		"--workdir", "/work",
		image,
		"sleep", "infinity",
	)
	cmd := exec.Command(args[0], args[1:]...)
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf(
			"error starting build container: %w", err)
	}
	b.buildContainer = strings.TrimSpace(string(out))
	return nil
}

const netnsDockerfile = `FROM alpine:latest
RUN apk add --no-cache iptables
ENTRYPOINT ["sh", "-c"]
`

const netnsImageTag = "warden-netns:latest"

func ensureNetnsImage() error {
	args := append(ctrctl.Cli,
		"image", "inspect", netnsImageTag)
	cmd := exec.Command(args[0], args[1:]...)
	if cmd.Run() == nil {
		return nil
	}

	home, _ := os.UserHomeDir()
	buildCtx, err := os.MkdirTemp(home, ".warden-netns-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(buildCtx)

	if err := os.WriteFile(
		filepath.Join(buildCtx, "Containerfile"),
		[]byte(netnsDockerfile), 0644); err != nil {
		return err
	}

	_, err = ctrctl.ImageBuild(
		&ctrctl.ImageBuildOpts{
			Tag:  netnsImageTag,
			File: filepath.Join(buildCtx, "Containerfile"),
		},
		buildCtx, "")
	return err
}

func (b *build) isolateBuildContainer() error {
	if err := ensureNetnsImage(); err != nil {
		return fmt.Errorf(
			"building netns image: %w", err)
	}

	script := fmt.Sprintf(`set -e
iptables -t nat -A OUTPUT -p udp --dport 53 `+
		`-j DNAT --to-destination %[1]s:53
iptables -t nat -A OUTPUT -p tcp --dport 53 `+
		`-j DNAT --to-destination %[1]s:53
iptables -t nat -A OUTPUT -p tcp --dport 80 `+
		`-j DNAT --to-destination %[1]s:80
iptables -t nat -A OUTPUT -p tcp --dport 443 `+
		`-j DNAT --to-destination %[1]s:443
iptables -A OUTPUT -d %[1]s -j ACCEPT
iptables -A OUTPUT -d 127.0.0.0/8 -j ACCEPT
iptables -A OUTPUT -j DROP
`, b.subnet.RelayIP)

	args := append(ctrctl.Cli, "container", "run",
		"--rm",
		"--network", "container:"+b.buildContainer,
		"--privileged",
		netnsImageTag,
		script,
	)
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf(
			"network isolation failed: %w", err)
	}
	return nil
}

func (b *build) injectWarden() error {
	_, err := ctrctl.ContainerCp(
		nil,
		b.wardenDirPath()+"/.",
		fmt.Sprintf(
			"%s:/.warden", b.buildContainer),
	)
	if err != nil {
		return fmt.Errorf(
			"error copying .warden into container: %w",
			err)
	}

	cmds := [][]string{
		{"ln", "-sf",
			"/.warden/warden-io",
			"/usr/local/bin/warden-io"},
		{"sh", "-c",
			"for f in /.warden/ext.d/*.sh; " +
				"do [ -f \"$f\" ] && sh \"$f\"; done"},
	}
	for _, c := range cmds {
		_, err := ctrctl.ContainerExec(
			nil, b.buildContainer, c[0], c[1:]...)
		if err != nil {
			return fmt.Errorf(
				"error running setup command %v: %w",
				c, err)
		}
	}
	return nil
}

func (b *build) createNetwork() error {
	id, err := ctrctl.NetworkCreate(
		&ctrctl.NetworkCreateOpts{
			Driver: "bridge",
			Subnet: b.subnet.CIDR,
		},
		"warden-"+b.buildID,
	)
	if err != nil {
		return err
	}
	b.isolatedNetwork = id
	return nil
}

func (b *build) startRelayContainer() error {
	args := append(ctrctl.Cli, "container", "run",
		"--detach",
		"--name", "warden-relay-"+b.buildID,
		"--network", "warden-"+b.buildID,
		"--ip", b.subnet.RelayIP,
		"--volume", b.ledgerDir+":/ledger",
		"--volume", b.req.ContextDir+":/context:ro",
	)

	capture := b.req.CaptureMode
	if capture != "" && capture != "none" {
		args = append(args,
			"--env", "CAPTURE_MODE="+capture)
	}
	args = append(args, b.relayImage)

	cmd := exec.Command(args[0], args[1:]...)
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf(
			"error starting relay container: %w", err)
	}
	b.relayContainer = strings.TrimSpace(string(out))
	return nil
}

func (b *build) resolveRelayImage() error {
	img := b.req.RelayImage
	ver := b.d.Version
	switch {
	case img == "dev" || (img == "" && ver == "dev"):
		logInfo(b.req, "Building relay from source...")
		b.relayBuiltLocal = true
		return b.buildRelayFromSource()
	case img != "":
		logInfo(b.req,
			fmt.Sprintf("Using relay image: %s", img))
		b.relayImage = img
		return b.pullImage(img)
	default:
		b.relayImage = relayImageRepo + ":latest"
		logInfo(b.req, fmt.Sprintf(
			"Pulling relay image: %s", b.relayImage))
		return b.pullImage(b.relayImage)
	}
}

func (b *build) pullImage(image string) error {
	args := append(ctrctl.Cli, "pull", image)
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf(
			"error pulling image %s: %w", image, err)
	}
	return nil
}

func (b *build) buildRelayFromSource() error {
	// Use a temp dir under the context dir so finch/Lima VM
	// can access it (Lima only mounts $HOME by default).
	tmpBase, err := os.MkdirTemp(
		b.req.ContextDir, ".warden-relay-")
	if err != nil {
		return fmt.Errorf(
			"error creating relay temp dir: %w", err)
	}
	defer os.RemoveAll(tmpBase)

	relayBin := filepath.Join(tmpBase, "relay")
	cmd := exec.Command(
		"go", "build", "-o", relayBin, "./cmd/relay")
	cmd.Env = append(os.Environ(),
		"GOOS=linux", "CGO_ENABLED=0")
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf(
			"error cross-compiling relay: %w", err)
	}

	dockerfile := `FROM alpine:latest
RUN apk add --no-cache ca-certificates
COPY relay /usr/local/bin/relay
EXPOSE 53/udp 80 443
ENTRYPOINT ["relay"]
`
	if err := os.WriteFile(
		filepath.Join(tmpBase, "Containerfile"),
		[]byte(dockerfile), 0644); err != nil {
		return fmt.Errorf(
			"error writing relay Containerfile: %w", err)
	}

	b.relayImage = "warden-relay:" + b.buildID
	_, err = ctrctl.ImageBuild(
		&ctrctl.ImageBuildOpts{
			Tag:  b.relayImage,
			File: filepath.Join(tmpBase, "Containerfile"),
		},
		tmpBase,
		"",
	)
	if err != nil {
		return fmt.Errorf(
			"error building relay image: %w", err)
	}
	return nil
}

func (b *build) buildWardenIO() error {
	dest := filepath.Join(
		b.wardenDirPath(), "warden-io")
	if b.d.Version != "dev" {
		exe, err := os.Executable()
		if err == nil {
			candidate := filepath.Join(
				filepath.Dir(exe), "warden-io")
			if data, err := os.ReadFile(
				candidate); err == nil {
				return os.WriteFile(dest, data, 0755)
			}
		}
	}
	cmd := exec.Command(
		"go", "build", "-o", dest, "./cmd/warden-io")
	cmd.Env = append(os.Environ(),
		"GOOS=linux", "CGO_ENABLED=0")
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf(
			"error cross-compiling warden-io: %w", err)
	}
	return nil
}

func (b *build) wardenDirPath() string {
	return filepath.Join(b.req.ContextDir, wardenDir)
}

func (b *build) teardown() {
	logInfo(b.req, "Tearing down environment...")

	if b.relayContainer != "" && b.outputDir != "" {
		b.collectRelayLogs()
	}
	if b.ledgerDir != "" && b.outputDir != "" {
		b.collectOutput()
	}

	_ = os.RemoveAll(b.wardenDirPath())

	if b.relayContainer != "" {
		_, err := ctrctl.ContainerRm(
			&ctrctl.ContainerRmOpts{Force: true},
			b.relayContainer)
		if err != nil {
			logWarn(b.req, fmt.Sprintf(
				"container cleanup: %s", err))
		}
	}
	if b.buildContainer != "" {
		_, err := ctrctl.ContainerRm(
			&ctrctl.ContainerRmOpts{
				Force:   true,
				Volumes: true,
			},
			b.buildContainer)
		if err != nil {
			logWarn(b.req, fmt.Sprintf(
				"container cleanup: %s", err))
		}
	}
	if b.relayBuiltLocal && b.relayImage != "" {
		_, _ = ctrctl.ImageRm(nil, b.relayImage)
	}
	if b.isolatedNetwork != "" {
		_, err := ctrctl.NetworkRm(
			nil, b.isolatedNetwork)
		if err != nil {
			logWarn(b.req, fmt.Sprintf(
				"network cleanup: %s", err))
		}
	}
}

func (b *build) collectRelayLogs() {
	logsArgs := append(ctrctl.Cli,
		"logs", b.relayContainer)
	cmd := exec.Command(logsArgs[0], logsArgs[1:]...)
	out, err := cmd.CombinedOutput()
	if err != nil || len(out) == 0 {
		return
	}
	logPath := filepath.Join(b.outputDir, "relay.log")
	os.WriteFile(logPath, out, 0644) //nolint:errcheck
}

func (b *build) collectOutput() {
	compress := b.req.Compress

	ledgerSrc := filepath.Join(b.ledgerDir, "ledger")
	if compress {
		compressFile(ledgerSrc,
			filepath.Join(b.outputDir, "ledger.zst"))
	} else {
		os.Rename(ledgerSrc, //nolint:errcheck
			filepath.Join(b.outputDir, "ledger"))
	}

	caSrc := filepath.Join(b.ledgerDir, "ca.cert.pem")
	if compress {
		compressFile(caSrc,
			filepath.Join(
				b.outputDir, "ca.cert.pem.zst"))
	} else {
		os.Rename(caSrc, //nolint:errcheck
			filepath.Join(b.outputDir, "ca.cert.pem"))
	}

	artDir := filepath.Join(b.ledgerDir, "artifacts")
	if entries, err := os.ReadDir(artDir); err == nil {
		outArt := filepath.Join(b.outputDir, "artifacts")
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

	os.RemoveAll(b.ledgerDir) //nolint:errcheck

	if b.req.Containerfile != "" {
		submitted, err := os.ReadFile(
			b.req.Containerfile)
		if err == nil {
			os.WriteFile( //nolint:errcheck
				filepath.Join(b.outputDir,
					"Dockerfile.submitted"),
				submitted, 0644)
		}
	}

	actual := filepath.Join(
		b.wardenDirPath(), "Containerfile")
	if data, err := os.ReadFile(actual); err == nil {
		os.WriteFile( //nolint:errcheck
			filepath.Join(b.outputDir,
				"Dockerfile.actual"),
			data, 0644)
	}

	scriptPath := filepath.Join(
		b.wardenDirPath(), "build.sh")
	if data, err := os.ReadFile(scriptPath); err == nil {
		os.WriteFile( //nolint:errcheck
			filepath.Join(b.outputDir, "build.sh"),
			data, 0644)
	}
}

// logInfo writes an informational message to stderr via the
// request's Stderr writer, or os.Stderr if not set.
func logInfo(req *driver.BuildRequest, msg string) {
	w := req.Stderr
	if w == nil {
		w = os.Stderr
	}
	fmt.Fprintf(w, "[warden] %s\n", msg)
}

// logWarn writes a warning message to stderr.
func logWarn(req *driver.BuildRequest, msg string) {
	w := req.Stderr
	if w == nil {
		w = os.Stderr
	}
	fmt.Fprintf(w, "[warden] WARNING: %s\n", msg)
}
