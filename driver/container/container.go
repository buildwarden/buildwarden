// Package container implements the container-based build driver.
//
// It follows the shared bootstrap model (docs/development/bootstrap.md):
//
//  1. isolate: a holder container owns the build's network namespace, and a
//     privileged sidecar applies the iptables rules to it, before anything
//     from the build image exists;
//  2. deliver the executor: the build container is created (not started) in
//     the holder's namespace with warden-io initialize as its entrypoint, and
//     warden-io is copied in;
//  3. start: the container is started and warden-io initialize bootstraps
//     itself from the relay. The host runs nothing inside it afterwards.
package container

import (
	"archive/tar"
	"bytes"
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
	"github.com/buildwarden/buildwarden/driver/script"
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
	Runtime string // CLI name: "finch", "docker", "podman"
	Verbose bool
	Version string
}

// build holds the mutable state for a single build invocation.
type build struct {
	d               *Driver
	req             *driver.BuildRequest
	buildContainer  string
	holderContainer string
	relayContainer  string
	relayImage      string
	relayBuiltLocal bool
	holderImage     string
	netnsImage      string
	baseImage       string
	buildID         string
	sess            *driver.Session
	outputDir       string
	subnet          driver.Subnet
	isolatedNetwork string
	// workDir is this build's private scratch directory (driver.NewWorkDir).
	// It holds the staged build script, the executor and the support
	// images' build contexts; nothing is written into the user's build
	// context.
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
	logInfo(req, "Starting build container...")
	if err := b.startEnvironment(false); err != nil {
		return nil, err
	}
	logInfo(req, "Executing build...")
	if err := b.waitBuild(ctx); err != nil {
		return nil, err
	}
	return &driver.BuildResult{
		OutputDir: b.outputDir,
	}, nil
}

// completionGrace is how long a build container that has exited waits for the
// guest's completion report, which warden-io sends just before it exits.
const completionGrace = 5 * time.Second

// waitBuild waits for the build's completion report, or for the build
// container to exit without one. The relay flushes the ledger before it
// reports completion, so this returns as soon as the collector has everything.
func (b *build) waitBuild(ctx context.Context) error {
	sctx, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- b.sess.Wait(sctx) }()
	exited := make(chan int, 1)
	go func() { exited <- b.containerWait() }()

	select {
	case err := <-done:
		return err
	case code := <-exited:
		select {
		case err := <-done:
			var ee *driver.ExitError
			if err == nil || errors.As(err, &ee) {
				return err
			}
		case <-time.After(completionGrace):
		}
		return fmt.Errorf(
			"build environment failed before the build ran "+
				"(warden-io exited with code %d):\n%s", code, b.consoleTail())
	case <-ctx.Done():
		_, _ = ctrctl.ContainerStop(nil, b.buildContainer)
		return ctx.Err()
	}
}

// containerWait blocks until the build container exits and returns its exit
// code (-1 if the runtime does not report one).
func (b *build) containerWait() int {
	args := append(ctrctl.Cli, "container", "wait", b.buildContainer)
	out, err := exec.Command(args[0], args[1:]...).Output()
	if err != nil {
		return -1
	}
	code, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil {
		return -1
	}
	return code
}

// consoleTail returns the last lines of the build container's console, which
// holds warden-io's own messages when it fails before the relay is reachable.
func (b *build) consoleTail() string {
	lines := strings.Split(strings.TrimRight(b.containerLogs(), "\n"), "\n")
	if len(lines) > 20 {
		lines = lines[len(lines)-20:]
	}
	return strings.Join(lines, "\n")
}

func (b *build) containerLogs() string {
	args := append(ctrctl.Cli, "logs", b.buildContainer)
	out, _ := exec.Command(args[0], args[1:]...).CombinedOutput()
	return string(out)
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
	if err := b.startEnvironment(true); err != nil {
		return err
	}
	if err := b.waitHeld(ctx); err != nil {
		return err
	}

	logInfo(req, "Dropping into shell (exit to tear down)...")
	stdin, stdout, stderr := stdio(req)

	// The shell sources the environment initialize --hold recorded, and runs
	// under a wrapper that records its exit status: with a TTY, finch
	// delivers nerdctl's "exit code N" line on the terminal stream, not
	// stderr, and its own exit status is always 1.
	_, err := ctrctl.ContainerExec(
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
		"sh", "-c", ". "+shellEnvFile+"; sh; s=$?; echo $s > "+shellExitFile+"; exit $s",
	)
	if err != nil {
		// The shell's own exit status is the user's, not a warden failure.
		if code, ok := b.shellExitCode(err); ok {
			return &driver.ExitError{Code: code}
		}
	}
	return err
}

// shellEnvFile is where initialize --hold records the build environment once
// setup is done (warden-io's shellEnvPath on Linux).
const shellEnvFile = "/etc/warden/shell.env"

// shellExitFile is where the shell wrapper records the shell's exit status.
const shellExitFile = "/.warden/shell-exit"

// holdTimeout bounds how long initialize --hold may take to set up.
const holdTimeout = 2 * time.Minute

// waitHeld waits until initialize --hold has finished setup. Interactive
// debugging is exempt from BOOT-03 once setup is done; these probes only look.
func (b *build) waitHeld(ctx context.Context) error {
	deadline := time.Now().Add(holdTimeout)
	for time.Now().Before(deadline) {
		if _, err := ctrctl.ContainerExec(nil, b.buildContainer,
			"test", "-f", shellEnvFile); err == nil {
			return nil
		}
		if !b.containerRunning() {
			return fmt.Errorf("build environment setup failed:\n%s", b.consoleTail())
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(250 * time.Millisecond):
		}
	}
	return fmt.Errorf("build environment setup timed out:\n%s", b.consoleTail())
}

func (b *build) containerRunning() bool {
	args := append(ctrctl.Cli, "container", "inspect",
		"--format", "{{.State.Running}}", b.buildContainer)
	out, err := exec.Command(args[0], args[1:]...).Output()
	return err == nil && strings.TrimSpace(string(out)) == "true"
}

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
	if err := os.MkdirAll(b.stagePath(), 0755); err != nil {
		return fmt.Errorf(
			"error creating %s: %w",
			b.stagePath(), err)
	}
	// The Containerfile is translated on the host: COPY sources are expanded
	// against the build context. The script is a build input the guest
	// fetches from the relay (BOOT-04).
	if err := b.stageBuildScript(); err != nil {
		return err
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
	if err := b.resolveSupportImages(); err != nil {
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

	return b.startSessionAndRelay(ctx, env)
}

// stageBuildScript writes the build script translated from the Containerfile
// (the same translation every driver's Linux guest runs).
func (b *build) stageBuildScript() error {
	result, err := script.Translate(b.req.Containerfile, b.req.ContextDir)
	if err != nil {
		return fmt.Errorf(
			"translating dockerfile: %w", err)
	}
	b.baseImage = result.Image
	if err := os.WriteFile(
		b.scriptPath(), []byte(result.Script), 0755); err != nil {
		return fmt.Errorf(
			"writing build script: %w", err)
	}
	return nil
}

// startSessionAndRelay starts the collector session, boots the relay
// container and claims it with the build's configuration.
func (b *build) startSessionAndRelay(ctx context.Context, env *relaycfg.Environment) error {
	var err error
	b.sess, err = driver.StartSession(driver.SessionConfig{
		OutputDir:   b.outputDir,
		ContextDir:  b.req.ContextDir,
		BuildScript: b.scriptPath(),
		OutputTee:   driver.ConsoleTee(b.req),
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
	_, err = b.sess.ClaimRelay(ctx, claimAddr, claimToken, b.req, env)
	return err
}

func (b *build) prepareEnvironment() (*relaycfg.Environment, error) {
	logInfo(b.req,
		fmt.Sprintf("Pulling image: %s", b.baseImage))
	if err := b.pullImage(b.baseImage); err != nil {
		return nil, err
	}

	env, err := imageEnvironment(b.baseImage)
	if err != nil {
		logWarn(b.req,
			fmt.Sprintf(
				"failed to record environment: %s", err))
		return nil, nil
	}
	return env, nil
}

// startEnvironment runs the bootstrap: isolate, deliver the executor, start.
// With hold, initialize stops after setup for an interactive shell.
func (b *build) startEnvironment(hold bool) error {
	if err := b.startHolder(); err != nil {
		return err
	}
	if err := b.isolate(); err != nil {
		return err
	}
	if err := b.createBuildContainer(hold); err != nil {
		return err
	}
	if err := b.copyExecutor(); err != nil {
		return err
	}
	if _, err := ctrctl.ContainerStart(nil, b.buildContainer); err != nil {
		return fmt.Errorf("error starting build container: %w", err)
	}
	return nil
}

// buildWorkdir is where build commands start; initialize creates it.
const buildWorkdir = "/work"

// startHolder starts the namespace holder on the build network. DNS and the
// relay's reserved names are set here: a container sharing the holder's
// namespace cannot set its own.
func (b *build) startHolder() error {
	out, err := runCLI("container", "run",
		"--detach",
		"--name", "warden-holder-"+b.buildID,
		"--network", b.isolatedNetwork,
		"--dns", b.subnet.RelayIP,
		"--add-host", "artifacts:"+b.subnet.RelayIP,
		"--add-host", "cwd:"+b.subnet.RelayIP,
		"--cap-drop", "ALL",
		"--read-only",
		"--security-opt", "no-new-privileges",
		b.holderImage,
	)
	if err != nil {
		return fmt.Errorf("error starting namespace holder: %w", err)
	}
	b.holderContainer = out
	return nil
}

// isolationRules makes the relay the build namespace's only network path.
// Docker on Linux injects legacy-iptables DNAT rules that redirect
// 127.0.0.11:53 to its embedded DNS resolver; if these remain, DNS bypasses
// the relay (legacy and nftables are separate kernel tables), so they are
// flushed first.
const isolationRules = `set -e
iptables-legacy -t nat -F DOCKER_OUTPUT 2>/dev/null || true
iptables-legacy -t nat -F DOCKER_POSTROUTING 2>/dev/null || true
iptables -t nat -A OUTPUT -p udp --dport 53 -j DNAT --to-destination %[1]s:53
iptables -t nat -A OUTPUT -p tcp --dport 53 -j DNAT --to-destination %[1]s:53
iptables -t nat -A OUTPUT -p tcp --dport 80 -j DNAT --to-destination %[1]s:80
iptables -t nat -A OUTPUT -p tcp --dport 443 -j DNAT --to-destination %[1]s:443
iptables -A OUTPUT -d %[1]s -j ACCEPT
iptables -A OUTPUT -d 127.0.0.0/8 -j ACCEPT
iptables -A OUTPUT -j DROP
`

// isolate applies the isolation rules to the holder's namespace from the
// privileged sidecar, before the build container exists (BOOT-01). The build
// container never holds CAP_NET_ADMIN.
func (b *build) isolate() error {
	args := append(ctrctl.Cli, "container", "run",
		"--rm",
		"--network", "container:"+b.holderContainer,
		"--privileged",
		b.netnsImage,
		fmt.Sprintf(isolationRules, b.subnet.RelayIP),
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

// createBuildContainer creates, but does not start, the build container in
// the holder's isolated namespace, with warden-io initialize as its
// entrypoint. Nothing from the image runs until the executor is in place.
func (b *build) createBuildContainer(hold bool) error {
	args := []string{"container", "create",
		"--name", "warden-build-" + b.buildID,
		"--network", "container:" + b.holderContainer,
		"--user", "0",
		"--entrypoint", executorPath,
		b.baseImage,
		"initialize",
		"--gateway=" + b.subnet.RelayIP,
		"--workdir=" + buildWorkdir,
	}
	if hold {
		args = append(args, "--hold")
	}
	out, err := runCLI(args...)
	if err != nil {
		return fmt.Errorf("error creating build container: %w", err)
	}
	b.buildContainer = out
	return nil
}

// executorPath is where warden-io lands in the build container.
const executorPath = "/.warden/warden-io"

// copyExecutor copies warden-io into the created container (BOOT-02). docker
// and podman take a tar stream built here, so the file mode is warden's and
// not the host filesystem's (a Windows host has no exec bit). nerdctl (finch)
// takes the staged directory, from a macOS or Linux host that keeps the mode.
func (b *build) copyExecutor() error {
	switch filepath.Base(b.d.Runtime) {
	case "finch", "nerdctl":
		_, err := ctrctl.ContainerCp(nil,
			filepath.Dir(b.executorStagePath())+"/.",
			b.buildContainer+":"+filepath.ToSlash(filepath.Dir(executorPath)))
		if err != nil {
			return fmt.Errorf("error copying warden-io into the build container: %w",
				err)
		}
		return nil
	}
	bin, err := os.ReadFile(b.executorStagePath())
	if err != nil {
		return err
	}
	stream, err := executorTar(bin)
	if err != nil {
		return err
	}
	args := append(ctrctl.Cli, "container", "cp", "-", b.buildContainer+":/")
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdin = bytes.NewReader(stream)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("error copying warden-io into the build container: %w: %s",
			err, strings.TrimSpace(string(out)))
	}
	return nil
}

// executorTar is a tar stream holding /.warden/warden-io, mode 0755, owned by
// root.
func executorTar(bin []byte) ([]byte, error) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	now := time.Now()
	dir := strings.TrimPrefix(filepath.ToSlash(filepath.Dir(executorPath)), "/") + "/"
	if err := tw.WriteHeader(&tar.Header{Typeflag: tar.TypeDir, Name: dir, Mode: 0o755,
		ModTime: now}); err != nil {
		return nil, err
	}
	if err := tw.WriteHeader(&tar.Header{Typeflag: tar.TypeReg,
		Name: strings.TrimPrefix(executorPath, "/"), Mode: 0o755, Size: int64(len(bin)),
		ModTime: now}); err != nil {
		return nil, err
	}
	if _, err := tw.Write(bin); err != nil {
		return nil, err
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// runCLI runs the container runtime and returns its trimmed stdout, with its
// stderr in the error.
func runCLI(args ...string) (string, error) {
	full := append(append([]string{}, ctrctl.Cli...), args...)
	out, err := exec.Command(full[0], full[1:]...).Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) && len(ee.Stderr) > 0 {
			err = fmt.Errorf("%w: %s", err, strings.TrimSpace(string(ee.Stderr)))
		}
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
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
	if err := goBuildLinux("./cmd/relay", filepath.Join(tmpBase, "relay")); err != nil {
		return err
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
	_, err := ctrctl.ImageBuild(
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

// buildWardenIO stages the executor: the release's warden-io next to the
// warden binary, or cross-compiled from source for a development build.
func (b *build) buildWardenIO() error {
	dest := b.executorStagePath()
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
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
	return goBuildLinux("./cmd/warden-io", dest)
}

// stagePath holds the build's staged input, build.sh, served by the collector
// and kept as provenance.
func (b *build) stagePath() string {
	return filepath.Join(b.workDir, "stage")
}

func (b *build) scriptPath() string {
	return filepath.Join(b.stagePath(), "build.sh")
}

// executorStagePath is warden-io, the one file the build container receives
// from the host (BOOT-02).
func (b *build) executorStagePath() string {
	return filepath.Join(b.workDir, "executor", filepath.Base(executorPath))
}

func (b *build) teardown() {
	logInfo(b.req, "Tearing down environment...")

	if b.sess != nil {
		b.sess.Close()
	}
	if b.relayContainer != "" && b.outputDir != "" {
		b.collectLogs()
	}
	if b.sess != nil {
		b.collectOutput()
	}

	if b.workDir != "" {
		_ = os.RemoveAll(b.workDir)
	}

	for _, c := range []string{b.buildContainer, b.relayContainer, b.holderContainer} {
		if c == "" {
			continue
		}
		_, err := ctrctl.ContainerRm(
			&ctrctl.ContainerRmOpts{Force: true, Volumes: true}, c)
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

// collectLogs keeps the relay's log and the build container's console
// (warden-io's own messages and the build's output as the guest printed it).
func (b *build) collectLogs() {
	logsArgs := append(ctrctl.Cli,
		"logs", b.relayContainer)
	cmd := exec.Command(logsArgs[0], logsArgs[1:]...)
	if out, err := cmd.CombinedOutput(); err == nil && len(out) > 0 {
		os.WriteFile(filepath.Join(b.outputDir, "relay.log"), out, 0644) //nolint:errcheck
	}
	if b.buildContainer != "" {
		if out := b.containerLogs(); out != "" {
			os.WriteFile( //nolint:errcheck
				filepath.Join(b.outputDir, "console.log"), []byte(out), 0644)
		}
	}
}

// collectOutput finalizes the collector's output and records the translated
// Containerfile under inputs/, the same as every driver.
func (b *build) collectOutput() {
	driver.FinalizeOutput(b.outputDir, b.req.Compress)
	if err := driver.RecordInputs(b.outputDir, b.req, b.scriptPath()); err != nil {
		logWarn(b.req, fmt.Sprintf("recording inputs: %s", err))
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
