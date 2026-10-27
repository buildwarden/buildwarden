// Package container implements the container-based build driver.
// It orchestrates an unprivileged build container with network
// isolation enforced by iptables rules applied via a sidecar
// container sharing the build container's network namespace
// (Kubernetes init container pattern).
package container

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/lesiw/ctrctl"

	"github.com/buildwarden/buildwarden/driver"
	"github.com/buildwarden/buildwarden/relaycfg"
)

const (
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
	// workDir is this build's private scratch directory (driver.NewWorkDir).
	// It holds the staged /.warden tree and the relay image's build context;
	// nothing is written into the user's build context.
	workDir string
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
	stdin, stdout, stderr := stdio(req)

	tail := newTailWriter(stderr)
	execCmd := &exec.Cmd{
		Stdin:  stdin,
		Stdout: stdout,
		Stderr: tail,
	}
	execDone := make(chan error, 1)
	go func() {
		_, execErr := ctrctl.ContainerExec(
			&ctrctl.ContainerExecOpts{
				Cmd:         execCmd,
				Interactive: true,
				Workdir:     buildWorkdir,
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
		<-execDone //nolint:errcheck // cancelled: the exec error is moot
		return nil, ctx.Err()
	}
	if err != nil {
		return nil, b.execFailure(ctx, err, tail.String())
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

// completionGrace is how long a failed exec waits for the guest's completion
// report, which warden-io sends just before it exits.
const completionGrace = 5 * time.Second

// execFailure explains a non-zero `exec` of the build. The script's exit code
// as the guest reported it to the collector is authoritative (nerdctl's own
// exit status is always 1). With no report, warden-io failed before the
// script ran (no relay, say): that is an environment error, not a build
// result.
func (b *build) execFailure(ctx context.Context, execErr error, stderrTail string) error {
	wctx, cancel := context.WithTimeout(ctx, completionGrace)
	defer cancel()
	var ee *driver.ExitError
	if err := b.sess.Wait(wctx); errors.As(err, &ee) {
		return err
	}
	if code, ok := execExitCode(execErr, stderrTail); ok {
		return fmt.Errorf(
			"build environment failed before the build ran "+
				"(warden-io exited with code %d; see its output above)", code)
	}
	return fmt.Errorf("build error: %w", execErr)
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
	stdin, stdout, stderr := stdio(req)

	// The shell runs under a wrapper that records its exit status: with a
	// TTY, finch delivers nerdctl's "exit code N" line on the terminal
	// stream, not stderr, and its own exit status is always 1.
	_, err = ctrctl.ContainerExec(
		&ctrctl.ContainerExecOpts{
			Cmd: &exec.Cmd{
				Stdin:  stdin,
				Stdout: stdout,
				Stderr: stderr,
			},
			Interactive: true,
			Tty:         true,
			Workdir:     buildWorkdir,
		},
		b.buildContainer,
		"sh", "-c", "sh; s=$?; echo $s > "+shellExitFile+"; exit $s",
	)
	if err != nil {
		// The shell's own exit status is the user's, not a warden failure.
		if code, ok := b.shellExitCode(err); ok {
			return &driver.ExitError{Code: code}
		}
	}
	return err
}

// shellExitFile is where the shell wrapper records the shell's exit status.
const shellExitFile = "/.warden/shell-exit"

// shellExitCode returns the exit status of the interactive shell, preferring
// the one its wrapper recorded over the runtime CLI's own exit status.
func (b *build) shellExitCode(execErr error) (int, bool) {
	out, err := ctrctl.ContainerExec(nil, b.buildContainer, "cat", shellExitFile)
	if err == nil {
		if n, err := strconv.Atoi(strings.TrimSpace(out)); err == nil {
			return n, true
		}
	}
	return execExitCode(execErr, "")
}

func (d *Driver) Close() error { return nil }

// stdio resolves the request's stdin, stdout and stderr, defaulting each to
// the process's own stream when the request leaves it unset.
func stdio(req *driver.BuildRequest) (stdin io.Reader, stdout, stderr io.Writer) {
	stdin, stdout, stderr = req.Stdin, req.Stdout, req.Stderr
	if stdin == nil {
		stdin = os.Stdin
	}
	if stdout == nil {
		stdout = os.Stdout
	}
	if stderr == nil {
		stderr = os.Stderr
	}
	return stdin, stdout, stderr
}

// --- build lifecycle ---

func (b *build) setup(ctx context.Context) error {
	b.buildID = driver.RandAlphaNum(8)
	logInfo(b.req,
		fmt.Sprintf("Build ID: %s", b.buildID))

	var err error
	if b.workDir, err = driver.NewWorkDir(b.req, "container-"+b.buildID); err != nil {
		return err
	}
	if err := os.MkdirAll(
		filepath.Join(b.wardenDirPath(), "ext.d"), 0755); err != nil {
		return fmt.Errorf(
			"error creating %s: %w",
			b.wardenDirPath(), err)
	}

	if err := b.buildWardenIO(); err != nil {
		return err
	}

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
	sub, err := driver.AllocateNetwork("warden-" + b.buildID)
	if err != nil {
		return err
	}
	b.subnet = sub
	b.isolatedNetwork = "warden-" + b.buildID

	if err := b.startSessionAndRelay(ctx, env); err != nil {
		return err
	}

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
		filepath.Join(b.wardenDirPath(), "Containerfile"),
		b.d.Extensions,
	); err != nil {
		return err
	}

	return nil
}

// startSessionAndRelay starts the collector session, boots the relay
// container, claims it with the build's configuration, and records the
// relay's CA certificate.
func (b *build) startSessionAndRelay(ctx context.Context, env *relaycfg.Environment) error {
	var err error
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

// buildWorkdir is where build commands start. It is created by injectWarden
// rather than with run --workdir: podman refuses a workdir the image lacks.
const buildWorkdir = "/work"

func (b *build) startBuildContainer(image string) error {
	args := append(ctrctl.Cli, "container", "run",
		"--detach",
		"--name", "warden-build-"+b.buildID,
		"--network", "warden-"+b.buildID,
		"--dns", b.subnet.RelayIP,
		"--add-host", "artifacts:"+b.subnet.RelayIP,
		"--add-host", "cwd:"+b.subnet.RelayIP,
		image,
		"sleep", "infinity",
	)
	cmd := exec.Command(args[0], args[1:]...)
	out, err := cmd.Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) && len(ee.Stderr) > 0 {
			err = fmt.Errorf("%w: %s", err, strings.TrimSpace(string(ee.Stderr)))
		}
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

// ensureNetnsImage builds the iptables sidecar image once per runtime,
// using dir (inside the build's work dir) as the image build context.
func ensureNetnsImage(dir string) error {
	args := append(ctrctl.Cli,
		"image", "inspect", netnsImageTag)
	cmd := exec.Command(args[0], args[1:]...)
	if cmd.Run() == nil {
		return nil
	}

	buildCtx := dir
	if err := os.MkdirAll(buildCtx, 0o755); err != nil {
		return err
	}

	if err := os.WriteFile(
		filepath.Join(buildCtx, "Containerfile"),
		[]byte(netnsDockerfile), 0644); err != nil {
		return err
	}

	_, err := ctrctl.ImageBuild(
		&ctrctl.ImageBuildOpts{
			Tag:  netnsImageTag,
			File: filepath.Join(buildCtx, "Containerfile"),
		},
		buildCtx, "")
	return err
}

func (b *build) isolateBuildContainer() error {
	if err := ensureNetnsImage(filepath.Join(b.workDir, "netns-image")); err != nil {
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
		{"mkdir", "-p", buildWorkdir},
		// cp from a Windows host carries no exec bit.
		{"chmod", "0755", "/.warden/warden-io"},
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

// relayPublishAttempts bounds how many host ports startRelayContainer tries
// before giving up on publishing the claim port.
const relayPublishAttempts = 8

// startRelayContainer starts the relay unconfigured, with its claim port
// published on host loopback only, and returns that host address. Nothing is
// mounted: the relay reaches its collector over the session's tunnel.
//
// The host port is chosen here rather than left to the runtime: nerdctl
// (finch) hands the SAME ephemeral port to containers started concurrently,
// so two builds would dial one relay. An explicit host port is reserved
// atomically by the runtime (a second bind is refused), so a collision with
// another build or process surfaces as a start error and is retried.
func (b *build) startRelayContainer(claimToken string) (string, error) {
	name := "warden-relay-" + b.buildID
	var lastErr error
	for range relayPublishAttempts {
		hostPort, err := driver.FreeLoopbackPort()
		if err != nil {
			return "", fmt.Errorf("finding a free loopback port: %w", err)
		}
		publish := fmt.Sprintf("127.0.0.1:%d:%d", hostPort, relaycfg.DefaultPort)
		args := append(ctrctl.Cli, "container", "run",
			"--detach",
			"--name", name,
			"--network", "warden-"+b.buildID,
			"--ip", b.subnet.RelayIP,
			"--publish", publish,
			"--env", relaycfg.TokenEnv+"="+claimToken,
			b.relayImage,
		)
		var stderr strings.Builder
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Stderr = &stderr
		out, err := cmd.Output()
		if err == nil {
			b.relayContainer = strings.TrimSpace(string(out))
			return net.JoinHostPort("127.0.0.1", strconv.Itoa(hostPort)), nil
		}
		lastErr = fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
		if !isPortConflict(stderr.String()) {
			break
		}
		// docker/podman leave a Created container behind on a failed
		// publish; clear the name before retrying with another port.
		_, _ = ctrctl.ContainerRm(&ctrctl.ContainerRmOpts{Force: true}, name)
	}
	return "", fmt.Errorf("error starting relay container: %w", lastErr)
}

// isPortConflict reports whether a runtime's start error means the chosen
// host port was already taken (wording differs across docker, nerdctl and
// podman).
func isPortConflict(stderr string) bool {
	s := strings.ToLower(stderr)
	for _, m := range []string{
		"port is already allocated",
		"address already in use",
		"bind for",
	} {
		if strings.Contains(s, m) {
			return true
		}
	}
	return false
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
	// The image build context lives in the work dir, so it must be a path
	// the runtime's VM can see (finch, Docker Desktop and podman machine
	// all share the macOS temp dir; see --tmpdir otherwise).
	tmpBase := filepath.Join(b.workDir, "relay-image")
	if err := os.MkdirAll(tmpBase, 0o755); err != nil {
		return fmt.Errorf(
			"error creating relay image dir: %w", err)
	}

	relayBin := filepath.Join(tmpBase, "relay")
	cmd := exec.Command(
		"go", "build", "-o", relayBin, "./cmd/relay")
	cmd.Env = append(os.Environ(),
		"GOOS=linux", "CGO_ENABLED=0")
	root, err := driver.ModuleRoot()
	if err != nil {
		return fmt.Errorf("building the relay from source: %w", err)
	}
	cmd.Dir = root
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf(
			"error cross-compiling relay: %w", err)
	}

	// chmod in the image: a context on a Windows host carries no exec bit.
	dockerfile := `FROM alpine:latest
RUN apk add --no-cache ca-certificates
COPY relay /usr/local/bin/relay
RUN chmod 0755 /usr/local/bin/relay
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
	root, err := driver.ModuleRoot()
	if err != nil {
		return fmt.Errorf("building warden-io from source: %w", err)
	}
	cmd.Dir = root
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf(
			"error cross-compiling warden-io: %w", err)
	}
	return nil
}

// wardenDirPath is the staged tree copied into the build container as
// /.warden (warden-io, the edited Containerfile, build.sh, extension files).
func (b *build) wardenDirPath() string {
	return filepath.Join(b.workDir, "warden")
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

	if b.workDir != "" {
		_ = os.RemoveAll(b.workDir)
	}

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
