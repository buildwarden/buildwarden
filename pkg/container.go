package warden

import (
	"bufio"
	"crypto/rand"
	"fmt"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/lesiw/ctrctl"
)

var wardenDir = ".warden"

var exts = []Extension{
	&ExtTrustStore{},
	&ExtPip{},
	&ExtBazel{},
}

const (
	relayIP          = "10.0.87.2"
	buildIP          = "10.0.87.3"
	subnet           = "10.0.87.0/29"
	rootlessDockerSock = "unix:///run/user/1000/docker.sock"
)

type CtrEnv struct {
	isolatedNetwork string
	defaultNetwork  string
	buildConfig     *BuildConfig
	buildContainer  string
	relayContainer  string
	relayImage      string
	buildId         string
	ledgerDir       string
}

func NewCtrEnv() BuildEnv {
	return &CtrEnv{}
}

func (d *CtrEnv) inBuildEnv(config *BuildConfig, fn func() error) error {
	ctrctl.Verbose = true
	if cli := os.Getenv("WARDEN_CTR_CLI"); cli != "" {
		ctrctl.Cli = []string{cli}
	}
	d.buildConfig = config

	defer d.teardownBuildEnv()
	if err := d.createBuildEnv(); err != nil {
		return err
	}

	if err := fn(); err != nil {
		return fmt.Errorf("build error: %w", err)
	}

	return nil
}

func (d *CtrEnv) Build(config *BuildConfig) error {
	return d.inBuildEnv(config, func() error {
		_, err := ctrctl.ContainerExec(
			&ctrctl.ContainerExecOpts{
				Cmd: &exec.Cmd{
					Stdin:  os.Stdin,
					Stdout: os.Stdout,
					Stderr: os.Stderr,
				},
				Interactive: true,
				User:        "rootless",
				Env:         "DOCKER_HOST=" + rootlessDockerSock,
			},
			d.buildContainer,
			"docker", "build", "--network=host", "-f", config.Containerfile, ".",
		)
		return err
	})
}

func (d *CtrEnv) Shell(config *BuildConfig) error {
	return d.inBuildEnv(config, func() error {
		_, err := ctrctl.ContainerExec(
			&ctrctl.ContainerExecOpts{
				Cmd: &exec.Cmd{
					Stdin:  os.Stdin,
					Stdout: os.Stdout,
					Stderr: os.Stderr,
				},
				Interactive: true,
				Tty:         true,
			},
			d.buildContainer,
			"sh",
		)
		return err
	})
}

func (d *CtrEnv) createBuildEnv() error {
	err := os.MkdirAll(filepath.Join(d.wardenDirPath(), "ext.d"), 0755)
	if err != nil {
		return fmt.Errorf("error creating %s: %w", d.wardenDirPath(), err)
	}

	d.buildId = randanum(8)

	// Create a host directory for ledger output.
	d.ledgerDir, err = os.MkdirTemp("", "warden-ledger-"+d.buildId+"-")
	if err != nil {
		return fmt.Errorf("error creating ledger temp dir: %w", err)
	}

	if err := d.buildRelayImage(); err != nil {
		return err
	}
	if err := d.createNetwork(); err != nil {
		return err
	}
	if err := d.startRelayContainer(); err != nil {
		return err
	}

	// Extensions run after the relay is up — they need its CA cert.
	for _, ext := range exts {
		if err := ext.BeforeBuild(d); err != nil {
			return err
		}
	}

	if err := d.editContainerfile(); err != nil {
		return err
	}
	if err := d.startBuildContainer(); err != nil {
		return err
	}
	if err := d.configureBuildContainer(); err != nil {
		return err
	}

	return nil
}

func (d *CtrEnv) buildRelayImage() error {
	// Cross-compile the relay binary for linux.
	relayBin := filepath.Join(os.TempDir(), "warden-relay-"+d.buildId)
	cmd := exec.Command("go", "build", "-o", relayBin, "./cmd/relay")
	cmd.Env = append(os.Environ(), "GOOS=linux", "CGO_ENABLED=0")
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("error cross-compiling relay: %w", err)
	}
	defer os.Remove(relayBin)

	// Build the relay container image.
	// We need a temp build context with the binary and Dockerfile.
	buildCtx, err := os.MkdirTemp("", "warden-relay-ctx-")
	if err != nil {
		return fmt.Errorf("error creating relay build context: %w", err)
	}
	defer os.RemoveAll(buildCtx)

	// Copy binary into build context.
	binData, err := os.ReadFile(relayBin)
	if err != nil {
		return fmt.Errorf("error reading relay binary: %w", err)
	}
	if err := os.WriteFile(filepath.Join(buildCtx, "relay"), binData, 0755); err != nil {
		return fmt.Errorf("error writing relay binary to context: %w", err)
	}

	// Write Dockerfile into build context.
	dockerfile := `FROM alpine:3.20
RUN apk add --no-cache ca-certificates
COPY relay /usr/local/bin/relay
EXPOSE 53/udp 80 443
ENTRYPOINT ["relay"]
`
	dfPath := filepath.Join(buildCtx, "Dockerfile")
	if err := os.WriteFile(dfPath, []byte(dockerfile), 0644); err != nil {
		return fmt.Errorf("error writing relay Dockerfile: %w", err)
	}

	d.relayImage = "warden-relay:" + d.buildId
	_, err = ctrctl.ImageBuild(
		&ctrctl.ImageBuildOpts{Tag: d.relayImage},
		buildCtx,
		"",
	)
	if err != nil {
		return fmt.Errorf("error building relay image: %w", err)
	}

	return nil
}

func (d *CtrEnv) createNetwork() error {
	// Isolated network for relay and build container.
	// The relay needs external access to forward build traffic upstream.
	// Build container isolation is enforced by:
	//   1. iptables OUTPUT rules (only relay + loopback allowed)
	//   2. Deleting the default route (no path to gateway even if iptables flushed)
	id, err := ctrctl.NetworkCreate(
		&ctrctl.NetworkCreateOpts{
			Driver: "bridge",
			Subnet: subnet,
		},
		"warden-"+d.buildId,
	)
	if err != nil {
		return err
	}
	d.isolatedNetwork = id
	return nil
}

func (d *CtrEnv) startRelayContainer() error {
	id, err := ctrctl.ContainerRun(
		&ctrctl.ContainerRunOpts{
			Detach:  true,
			Name:    "warden-relay-" + d.buildId,
			Network: "warden-" + d.buildId,
			Ip:      relayIP,
			Volume:  d.ledgerDir + ":/ledger",
		},
		d.relayImage,
		"",
	)
	if err != nil {
		return fmt.Errorf("error starting relay container: %w", err)
	}
	d.relayContainer = id
	return nil
}

func (d *CtrEnv) startBuildContainer() error {
	// The container needs --privileged for rootlesskit to create user
	// namespaces. However, the inner dockerd and all build processes run
	// as the unprivileged "rootless" user, which cannot modify iptables
	// or routes in the real network namespace.
	id, err := ctrctl.ContainerRun(
		&ctrctl.ContainerRunOpts{
			Detach:     true,
			Name:       "warden-build-" + d.buildId,
			Network:    "warden-" + d.buildId,
			Ip:         buildIP,
			Privileged: true,
			Workdir:    "/work",
		},
		"docker:dind-rootless",
		"",
	)
	if err != nil {
		return err
	}
	d.buildContainer = id
	return nil
}

func (d *CtrEnv) configureBuildContainer() error {
	_, err := ctrctl.ContainerCp(
		nil,
		d.buildConfig.Context+"/.",
		fmt.Sprintf("%s:/work", d.buildContainer),
	)
	if err != nil {
		return err
	}

	// Network isolation rules applied as privileged exec from the
	// orchestrator. The rootless build container cannot override these
	// because it lacks CAP_NET_ADMIN.
	cmds := [][]string{
		// Point DNS at the relay.
		{"sh", "-c", fmt.Sprintf(`echo "nameserver %s" > /etc/resolv.conf`, relayIP)},
		// Redirect HTTP/HTTPS to the relay.
		{"iptables", "-t", "nat", "-A", "OUTPUT", "-p", "tcp", "--dport", "80",
			"-j", "DNAT", "--to-destination", relayIP + ":80"},
		{"iptables", "-t", "nat", "-A", "OUTPUT", "-p", "tcp", "--dport", "443",
			"-j", "DNAT", "--to-destination", relayIP + ":443"},
		// Isolate: only allow traffic to the relay, drop everything else.
		{"iptables", "-A", "OUTPUT", "-d", relayIP, "-j", "ACCEPT"},
		{"iptables", "-A", "OUTPUT", "-d", "127.0.0.0/8", "-j", "ACCEPT"},
		{"iptables", "-A", "OUTPUT", "-j", "DROP"},
		// Defense-in-depth: replace the default route with one pointing at
		// the relay. Even if iptables are flushed, all traffic still goes
		// to the relay (which won't forward unrecognized traffic).
		{"ip", "route", "replace", "default", "via", relayIP},
		// Set up warden extensions.
		{"ln", "-s", "/work/.warden", "/.warden"},
		{"find", "/.warden/ext.d/", "-exec", "sh", "{}", ";"},
	}
	for _, cmd := range cmds {
		_, err := ctrctl.ContainerExec(
			&ctrctl.ContainerExecOpts{Privileged: true, User: "0"},
			d.buildContainer,
			cmd[0],
			cmd[1:]...,
		)
		if err != nil {
			return err
		}
	}

	// Wait for the rootless docker daemon (started by the image entrypoint).
	for i := 0; i < 50; i++ {
		_, err = ctrctl.ContainerExec(
			&ctrctl.ContainerExecOpts{
				User: "rootless",
				Env:  "DOCKER_HOST=" + rootlessDockerSock,
			},
			d.buildContainer,
			"docker", "info",
		)
		if err == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err != nil {
		return fmt.Errorf("timed out waiting for rootless dockerd: %w", err)
	}

	return nil
}

func (d *CtrEnv) teardownBuildEnv() {
	d.revertContainerfileEdits() // TODO: remove this when moved to wardenDir
	_ = os.RemoveAll(d.wardenDirPath())
	if d.relayContainer != "" {
		_, err := ctrctl.ContainerRm(
			&ctrctl.ContainerRmOpts{Force: true},
			d.relayContainer,
		)
		if err != nil {
			fmt.Fprintf(os.Stderr, "container cleanup failure: %s\n", err)
		}
	}
	if d.buildContainer != "" {
		_, err := ctrctl.ContainerRm(
			&ctrctl.ContainerRmOpts{Force: true},
			d.buildContainer,
		)
		if err != nil {
			fmt.Fprintf(os.Stderr, "container cleanup failure: %s\n", err)
		}
	}
	if d.relayImage != "" {
		_, _ = ctrctl.ImageRm(nil, d.relayImage)
	}
	if d.isolatedNetwork != "" {
		_, err := ctrctl.NetworkRm(nil, d.isolatedNetwork)
		if err != nil {
			fmt.Fprintf(os.Stderr, "network cleanup failure: %s\n", err)
		}
	}
	if d.ledgerDir != "" {
		fmt.Fprintf(os.Stderr, "ledger output: %s\n", d.ledgerDir)
	}
}

func (d *CtrEnv) doBuild() error {
	return nil
}

func (d *CtrEnv) editContainerfile() error {
	// FIXME: This should produce a copy in .warden/Containerfile.
	// Don't move/edit the original Containerfile.

	ctrfilePath := filepath.Join(d.buildConfig.Context, d.buildConfig.Containerfile)
	origfilePath := filepath.Join(d.buildConfig.Context, d.buildConfig.Containerfile+".orig")
	err := os.Rename(ctrfilePath, origfilePath)
	if err != nil {
		return fmt.Errorf("error moving %s: %w", ctrfilePath, err)
	}
	origfile, err := os.Open(origfilePath)
	if err != nil {
		return fmt.Errorf("error opening containerfile: %w", err)
	}
	defer origfile.Close()
	ctrfile, err := os.Create(ctrfilePath)
	if err != nil {
		return fmt.Errorf("error creating edited containerfile: %w", err)
	}
	defer ctrfile.Close()

	env := make(map[string]string)
	for _, ext := range exts {
		extenv := ext.Env()
		if extenv == nil {
			continue
		}
		for k, v := range extenv {
			if _, ok := env[k]; ok {
				return fmt.Errorf("env collision: %s", k)
			}
			env[k] = v
		}
	}
	var enventries []string
	for k, v := range env {
		enventries = append(enventries, fmt.Sprintf("%s=%s", k, v))
	}

	scanner := bufio.NewScanner(origfile)
	for scanner.Scan() {
		line := scanner.Text()
		_, _ = ctrfile.WriteString(line + "\n")

		if strings.HasPrefix(line, "FROM ") {
			if len(enventries) > 0 {
				_, _ = ctrfile.WriteString("ENV " +
					strings.Join(enventries, " \\\n") + "\n")
			}
			_, _ = ctrfile.WriteString("COPY .warden /.warden\n")
			_, _ = ctrfile.WriteString("RUN find /.warden/ext.d/ -exec sh {} \\;\n")
		}
	}

	return nil
}

func (d *CtrEnv) revertContainerfileEdits() {
	ctrfilePath := filepath.Join(d.buildConfig.Context, d.buildConfig.Containerfile)
	origfilePath := filepath.Join(d.buildConfig.Context, d.buildConfig.Containerfile+".orig")
	_, err := os.Stat(origfilePath)
	if err != nil {
		return // No original file to restore.
	}
	_ = os.Remove(ctrfilePath)
	_ = os.Rename(origfilePath, ctrfilePath)
}

func (d *CtrEnv) wardenDirPath() string {
	return filepath.Join(d.buildConfig.Context, wardenDir)
}

func (d *CtrEnv) wardenScriptPath() string {
	return filepath.Join(d.wardenDirPath(), "ext.d")
}

var anumRunes = []rune("abcdefghijklmnopqrstuvwxyz0123456789")

// randanum produces a cryptographically-random alphanumeric string.
func randanum(n int) string {
	b := make([]rune, n)
	for i := range b {
		j, err := rand.Int(rand.Reader, big.NewInt(int64(len(anumRunes))))
		if err != nil {
			panic(err)
		}
		b[i] = anumRunes[j.Uint64()]
	}
	return string(b)
}
