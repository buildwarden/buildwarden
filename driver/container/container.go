// Package container implements the container-based build driver.
// It orchestrates an unprivileged build container with network
// isolation enforced by iptables rules applied via a sidecar
// container sharing the build container's network namespace
// (Kubernetes init container pattern).
package container

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/lesiw/ctrctl"

	"github.com/buildwarden/buildwarden/driver"
	"github.com/buildwarden/buildwarden/relaycfg"
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
	sess            *driver.Session
	caCert          []byte
	outputDir       string
	subnet          driver.Subnet
	isolatedNetwork string
}

func (d *Driver) Name() string { return "container" }

func (d *Driver) StartBuild(
	ctx context.Context, req *driver.BuildRequest,
) (*driver.BuildResult, error) {
	ctrctl.Cli = []string{d.Runtime}
	if req.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, req.Timeout)
		defer cancel()
	}

	b := &build{d: d, req: req}
	defer b.teardown()

	if err := b.setup(ctx); err != nil {
		return nil, err
	}

	ctrfilePath := filepath.Join(
		b.wardenDirPath(), "Containerfile")
	result, err := DockerfileToScript(ctrfilePath)
	if err != nil {
		return nil, fmt.Errorf(
			"translating dockerfile: %w", err)
	}

	scriptPath := filepath.Join(b.wardenDirPath(), "build.sh")
	if err := os.WriteFile(
		scriptPath, []byte(result.Script), 0755); err != nil {
		return nil, fmt.Errorf(
			"writing build script: %w", err)
	}

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

	execCmd := &exec.Cmd{
		Stdin:  stdin,
		Stdout: stdout,
		Stderr: stderr,
	}
	execDone := make(chan error, 1)
	go func() {
		_, execErr := ctrctl.ContainerExec(
			&ctrctl.ContainerExecOpts{
				Cmd:         execCmd,
				Interactive: true,
			},
			b.buildContainer,
			"warden-io", "initialize",
			"--gateway="+b.subnet.RelayIP,
			"--script=.warden/build.sh",
		)
		execDone <- execErr
	}()

	select {
	case err = <-execDone:
	case <-ctx.Done():
		if execCmd.Process != nil {
			_ = execCmd.Process.Kill()
		}
		_, _ = ctrctl.ContainerStop(nil, b.buildContainer)
		<-execDone
		return nil, ctx.Err()
	}
	if err != nil {
		return nil, fmt.Errorf("build error: %w", err)
	}
	// The relay flushes the ledger before it reports completion, so this
	// returns as soon as the collector has everything.
	if err := b.sess.Wait(ctx); err != nil {
		return nil, err
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

	if err := b.setup(ctx); err != nil {
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

func (b *build) setup(ctx context.Context) error {
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

	if err := b.resolveRelayImage(); err != nil {
		return err
	}

	env, err := b.prepareEnvironment()
	if err != nil {
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

	// The build console reaches the terminal through `exec`, so the
	// collector does not tee it as well.
	b.sess, err = driver.StartSession(driver.SessionConfig{
		OutputDir:   b.outputDir,
		ContextDir:  b.req.ContextDir,
		BuildScript: filepath.Join(b.wardenDirPath(), "build.sh"),
		Verbose:     b.d.Verbose,
	})
	if err != nil {
		return err
	}

	logInfo(b.req, "Starting relay...")
	claimToken, err := driver.RandomToken()
	if err != nil {
		return err
	}
	claimAddr, err := b.startRelayContainer(claimToken)
	if err != nil {
		return err
	}
	id, err := b.sess.ClaimRelay(ctx, claimAddr, claimToken, b.req, env)
	if err != nil {
		return err
	}
	b.caCert = []byte(id.CACert)

	extCtx := &driver.ExtensionContext{
		WardenDir: b.wardenDirPath(),
		ScriptDir: filepath.Join(
			b.wardenDirPath(), "ext.d"),
		CACert:  b.caCert,
		RelayIP: b.subnet.RelayIP,
		BuildID: b.buildID,
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

func (b *build) prepareEnvironment() (*relaycfg.Environment, error) {
	image, err := driver.ExtractFromImage(
		b.req.Containerfile)
	if err != nil {
		return nil, fmt.Errorf("reading FROM image: %w", err)
	}

	logInfo(b.req,
		fmt.Sprintf("Pulling image: %s", image))
	if err := b.pullBuildBaseImage(image); err != nil {
		return nil, err
	}

	env, err := imageEnvironment(image)
	if err != nil {
		logWarn(b.req,
			fmt.Sprintf(
				"failed to record environment: %s", err))
		return nil, nil
	}
	return env, nil
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
RUN apk add --no-cache iptables iptables-legacy
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

	// Flush Docker's embedded DNS legacy iptables rules before applying ours.
	// Docker on Linux injects legacy-iptables DNAT rules that redirect
	// 127.0.0.11:53 to its embedded DNS resolver. If these remain, DNS
	// bypasses the relay (legacy and nftables are separate kernel tables).
	script := fmt.Sprintf(`set -e
iptables-legacy -t nat -F DOCKER_OUTPUT 2>/dev/null || true
iptables-legacy -t nat -F DOCKER_POSTROUTING 2>/dev/null || true
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

	// Overwrite resolv.conf to point directly at the relay. Docker on
	// user-defined networks ignores --dns and always sets nameserver to
	// 127.0.0.11. Writing the relay IP ensures DNS queries reach the relay
	// regardless of iptables backend mismatches.
	_, err := ctrctl.ContainerExec(nil, b.buildContainer,
		"sh", "-c",
		fmt.Sprintf("echo 'nameserver %s' > /etc/resolv.conf",
			b.subnet.RelayIP))
	if err != nil {
		return fmt.Errorf("overwriting resolv.conf: %w", err)
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

// startRelayContainer starts the relay unconfigured, with its claim port
// published on host loopback only, and returns that host address. Nothing is
// mounted: the relay reaches its collector over the session's tunnel.
func (b *build) startRelayContainer(claimToken string) (string, error) {
	port := strconv.Itoa(relaycfg.DefaultPort)
	args := append(ctrctl.Cli, "container", "run",
		"--detach",
		"--name", "warden-relay-"+b.buildID,
		"--network", "warden-"+b.buildID,
		"--ip", b.subnet.RelayIP,
		"--publish", "127.0.0.1::"+port,
		"--env", relaycfg.TokenEnv+"="+claimToken,
		b.relayImage,
	)
	out, err := exec.Command(args[0], args[1:]...).Output()
	if err != nil {
		return "", fmt.Errorf(
			"error starting relay container: %w", err)
	}
	b.relayContainer = strings.TrimSpace(string(out))

	portArgs := append(ctrctl.Cli, "container", "port", b.relayContainer, port+"/tcp")
	out, err = exec.Command(portArgs[0], portArgs[1:]...).Output()
	if err != nil {
		return "", fmt.Errorf("finding the relay's published claim port: %w", err)
	}
	return parsePublishedAddr(string(out))
}

// parsePublishedAddr picks the IPv4 loopback mapping from `container port`
// output (one "host:port" per line, possibly also an IPv6 one).
func parsePublishedAddr(out string) (string, error) {
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		line = strings.TrimSpace(line)
		if host, p, err := net.SplitHostPort(line); err == nil && p != "" &&
			(host == "127.0.0.1" || host == "0.0.0.0") {
			return net.JoinHostPort("127.0.0.1", p), nil
		}
	}
	return "", fmt.Errorf("no IPv4 loopback mapping in %q", strings.TrimSpace(out))
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
EXPOSE 53/udp 80 443 8301
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

	if b.sess != nil {
		b.sess.Close()
	}
	if b.relayContainer != "" && b.outputDir != "" {
		b.collectRelayLogs()
	}
	if b.sess != nil {
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

// collectOutput finalizes the collector's output (the same as every driver)
// and adds the container driver's provenance files.
func (b *build) collectOutput() {
	driver.FinalizeOutput(b.outputDir, b.req.Compress)

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
