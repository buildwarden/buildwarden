package orchestrator

import (
	"bufio"
	"crypto/rand"
	"fmt"
	"io"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/klauspost/compress/zstd"
	"github.com/lesiw/ctrctl"
)

// allocateSubnet finds an available /29 within the warden base /24.
// It tries each of the 32 possible /29 blocks and returns the first
// one that doesn't conflict with existing container networks.
func allocateSubnet() (buildSubnet, error) {
	for block := 0; block < 32; block++ {
		base := block * 8
		sub := buildSubnet{
			cidr:    fmt.Sprintf("%s.%d/29", wardenBaseNet, base),
			relayIP: fmt.Sprintf("%s.%d", wardenBaseNet, base+2),
			buildIP: fmt.Sprintf("%s.%d", wardenBaseNet, base+3),
		}
		_, err := ctrctl.NetworkCreate(
			&ctrctl.NetworkCreateOpts{
				Driver: "bridge",
				Subnet: sub.cidr,
			},
			"warden-probe",
		)
		if err != nil {
			continue
		}
		// Probe succeeded — remove it and return this subnet.
		_, _ = ctrctl.NetworkRm(nil, "warden-probe")
		return sub, nil
	}
	return buildSubnet{}, fmt.Errorf(
		"no available /29 subnet in %s.0/24", wardenBaseNet)
}

var wardenDir = ".warden"

var exts = []Extension{
	&ExtTrustStore{},
	&ExtPip{},
	&ExtBazel{},
	&ExtEpoch{},
}

const rootlessDockerSock = "unix:///run/user/1000/docker.sock"

// wardenBaseNet is a /24 in the CGNAT range (100.64.0.0/10) which is
// reserved for carrier-grade NAT and unlikely to collide with user networks.
// Each build allocates a /29 within this range.
const wardenBaseNet = "100.64.87"

type buildSubnet struct {
	cidr    string
	relayIP string
	buildIP string
}

type CtrEnv struct {
	isolatedNetwork string
	defaultNetwork  string
	buildConfig     *BuildConfig
	buildContainer  string
	buildImage      string
	relayContainer  string
	relayImage      string
	relayBuiltLocal bool
	buildId         string
	ledgerDir       string
	outputDir       string
	subnet          buildSubnet
}

func NewCtrEnv() BuildEnv {
	return &CtrEnv{}
}

func (d *CtrEnv) inBuildEnv(config *BuildConfig, fn func() error) error {
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
		log.Build("Starting build...")
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
			"docker", "build", "--network=host",
				"--add-host", "artifacts:"+d.subnet.relayIP,
				"--add-host", "cwd:"+d.subnet.relayIP,
				"-f", ".warden/Containerfile", ".",
		)
		if err == nil {
			log.Success("Build complete")
		}
		return err
	})
}

func (d *CtrEnv) Shell(config *BuildConfig) error {
	return d.inBuildEnv(config, func() error {
		log.Info("Dropping into shell (exit to tear down)...")
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
	log.Info(fmt.Sprintf("Build ID: %s", d.buildId))

	d.outputDir = d.buildConfig.OutputDir
	if d.outputDir == "" {
		d.outputDir = "warden-output"
	}
	if err = os.MkdirAll(d.outputDir, 0755); err != nil {
		return fmt.Errorf("error creating output dir: %w", err)
	}

	d.ledgerDir, err = os.MkdirTemp("", "warden-ledger-"+d.buildId+"-")
	if err != nil {
		return fmt.Errorf("error creating ledger temp dir: %w", err)
	}

	if err := d.resolveRelayImage(); err != nil {
		return err
	}
	log.Info("Allocating network...")
	sub, err := allocateSubnet()
	if err != nil {
		return err
	}
	d.subnet = sub
	if err := d.createNetwork(); err != nil {
		return err
	}
	log.Info("Starting relay...")
	if err := d.startRelayContainer(); err != nil {
		return err
	}

	for _, ext := range exts {
		if err := ext.BeforeBuild(d); err != nil {
			return err
		}
	}

	if err := d.editContainerfile(); err != nil {
		return err
	}
	log.Info("Starting build container...")
	if err := d.startBuildContainer(); err != nil {
		return err
	}
	log.Info("Configuring network isolation...")
	if err := d.configureBuildContainer(); err != nil {
		return err
	}

	log.Success("Environment ready")
	return nil
}

const relayImageRepo = "ghcr.io/buildwarden/relay"

func (d *CtrEnv) resolveRelayImage() error {
	img := d.buildConfig.RelayImage

	switch {
	case img == "dev" || (img == "" && version == "dev"):
		log.Info("Building relay from source...")
		d.relayBuiltLocal = true
		return d.buildRelayFromSource()
	case img != "":
		log.Info(fmt.Sprintf("Using relay image: %s", img))
		d.relayImage = img
		return d.pullRelayImage()
	default:
		d.relayImage = relayImageRepo + ":latest"
		log.Info(fmt.Sprintf("Pulling relay image: %s", d.relayImage))
		return d.pullRelayImage()
	}
}

func (d *CtrEnv) pullRelayImage() error {
	args := append(ctrctl.Cli, "pull", d.relayImage)
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("error pulling relay image %s: %w",
			d.relayImage, err)
	}
	return nil
}

func (d *CtrEnv) buildRelayFromSource() error {
	relayBin := filepath.Join(os.TempDir(), "warden-relay-"+d.buildId)
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
	id, err := ctrctl.NetworkCreate(
		&ctrctl.NetworkCreateOpts{
			Driver: "bridge",
			Subnet: d.subnet.cidr,
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
	args := append(ctrctl.Cli, "container", "run",
		"--detach",
		"--name", "warden-relay-"+d.buildId,
		"--network", "warden-"+d.buildId,
		"--ip", d.subnet.relayIP,
		"--volume", d.ledgerDir+":/ledger",
		"--volume", d.buildConfig.Context+":/context:ro",
	)
	if d.buildConfig.Capture != "" && d.buildConfig.Capture != "none" {
		args = append(args, "--env", "CAPTURE_MODE="+d.buildConfig.Capture)
	}
	args = append(args, d.relayImage)
	cmd := exec.Command(args[0], args[1:]...)
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("error starting relay container: %w", err)
	}
	d.relayContainer = strings.TrimSpace(string(out))
	return nil
}

func (d *CtrEnv) startBuildContainer() error {
	img, err := d.buildBuildImage()
	if err != nil {
		return err
	}

	id, err := ctrctl.ContainerRun(
		&ctrctl.ContainerRunOpts{
			Detach:     true,
			Name:       "warden-build-" + d.buildId,
			Network:    "warden-" + d.buildId,
			Ip:         d.subnet.buildIP,
			Privileged: true,
			Workdir:    "/work",
		},
		img,
		"",
	)
	if err != nil {
		return err
	}
	d.buildContainer = id
	return nil
}

func (d *CtrEnv) buildBuildImage() (string, error) {
	buildCtx, err := os.MkdirTemp("", "warden-build-ctx-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(buildCtx)

	// Pre-configure dockerd with relay DNS so buildkit resolves through it.
	daemonJSON := fmt.Sprintf(`{"dns": ["%s"]}`, d.subnet.relayIP)
	os.WriteFile( //nolint:errcheck
		filepath.Join(buildCtx, "daemon.json"), []byte(daemonJSON), 0644)

	dockerfile := `FROM docker:dind-rootless
COPY daemon.json /home/rootless/.config/docker/daemon.json
`
	os.WriteFile( //nolint:errcheck
		filepath.Join(buildCtx, "Dockerfile"), []byte(dockerfile), 0644)

	d.buildImage = "warden-build:" + d.buildId
	_, err = ctrctl.ImageBuild(
		&ctrctl.ImageBuildOpts{Tag: d.buildImage},
		buildCtx,
		"",
	)
	if err != nil {
		return "", fmt.Errorf("error building build image: %w", err)
	}
	return d.buildImage, nil
}

func (d *CtrEnv) configureBuildContainer() error {
	// Only copy .warden/ — needed for CA cert trust before relay is usable.
	// All other context files are fetched through the relay for provenance.
	_, err := ctrctl.ContainerCp(
		nil,
		filepath.Join(d.buildConfig.Context, wardenDir)+"/.",
		fmt.Sprintf("%s:/work/%s", d.buildContainer, wardenDir),
	)
	if err != nil {
		return err
	}

	// Network isolation rules applied as privileged exec from the
	// orchestrator. The rootless build container cannot override these
	// because it lacks CAP_NET_ADMIN.
	cmds := [][]string{
		// Enable forwarding for buildkit's internal network to reach relay.
		{"sysctl", "-w", "net.ipv4.ip_forward=1"},
		// NAT: redirect all DNS/HTTP/HTTPS to relay (catches both
		// locally-originated and forwarded traffic from buildkit).
		{"iptables", "-t", "nat", "-A", "OUTPUT", "-p", "udp", "--dport", "53",
			"-j", "DNAT", "--to-destination", d.subnet.relayIP + ":53"},
		{"iptables", "-t", "nat", "-A", "OUTPUT", "-p", "tcp", "--dport", "53",
			"-j", "DNAT", "--to-destination", d.subnet.relayIP + ":53"},
		{"iptables", "-t", "nat", "-A", "PREROUTING", "-p", "udp", "--dport", "53",
			"-j", "DNAT", "--to-destination", d.subnet.relayIP + ":53"},
		{"iptables", "-t", "nat", "-A", "PREROUTING", "-p", "tcp", "--dport", "53",
			"-j", "DNAT", "--to-destination", d.subnet.relayIP + ":53"},
		{"iptables", "-t", "nat", "-A", "OUTPUT", "-p", "tcp", "--dport", "80",
			"-j", "DNAT", "--to-destination", d.subnet.relayIP + ":80"},
		{"iptables", "-t", "nat", "-A", "PREROUTING", "-p", "tcp", "--dport", "80",
			"-j", "DNAT", "--to-destination", d.subnet.relayIP + ":80"},
		{"iptables", "-t", "nat", "-A", "OUTPUT", "-p", "tcp", "--dport", "443",
			"-j", "DNAT", "--to-destination", d.subnet.relayIP + ":443"},
		{"iptables", "-t", "nat", "-A", "PREROUTING", "-p", "tcp", "--dport", "443",
			"-j", "DNAT", "--to-destination", d.subnet.relayIP + ":443"},
		// MASQUERADE forwarded traffic so relay can reply.
		{"iptables", "-t", "nat", "-A", "POSTROUTING",
			"-d", d.subnet.relayIP, "-j", "MASQUERADE"},
		// Isolate: allow relay, loopback, and DNS (needed for buildkit).
		{"iptables", "-A", "OUTPUT", "-d", d.subnet.relayIP, "-j", "ACCEPT"},
		{"iptables", "-A", "OUTPUT", "-d", "127.0.0.0/8", "-j", "ACCEPT"},
		{"iptables", "-A", "OUTPUT", "-p", "udp", "--dport", "53", "-j", "ACCEPT"},
		{"iptables", "-A", "OUTPUT", "-j", "DROP"},
		// Allow forwarding to relay (for buildkit's internal traffic).
		{"iptables", "-A", "FORWARD", "-d", d.subnet.relayIP, "-j", "ACCEPT"},
		{"iptables", "-A", "FORWARD", "-j", "DROP"},
		// Defense-in-depth: replace the default route with one pointing at
		// the relay. Even if iptables are flushed, all traffic still goes
		// to the relay (which won't forward unrecognized traffic).
		{"ip", "route", "replace", "default", "via", d.subnet.relayIP},
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
	log.Info("Tearing down environment...")

	// Collect relay logs before removing the container.
	if d.relayContainer != "" && d.outputDir != "" {
		d.collectRelayLogs()
	}

	// Copy ledger output to the final output directory.
	if d.ledgerDir != "" && d.outputDir != "" {
		d.collectOutput()
	}

	_ = os.RemoveAll(d.wardenDirPath())
	if d.relayContainer != "" {
		_, err := ctrctl.ContainerRm(
			&ctrctl.ContainerRmOpts{Force: true},
			d.relayContainer,
		)
		if err != nil {
			log.Warn(fmt.Sprintf("container cleanup: %s", err))
		}
	}
	if d.buildContainer != "" {
		_, err := ctrctl.ContainerRm(
			&ctrctl.ContainerRmOpts{Force: true},
			d.buildContainer,
		)
		if err != nil {
			log.Warn(fmt.Sprintf("container cleanup: %s", err))
		}
	}
	if d.relayBuiltLocal && d.relayImage != "" {
		_, _ = ctrctl.ImageRm(nil, d.relayImage)
	}
	if d.buildImage != "" {
		_, _ = ctrctl.ImageRm(nil, d.buildImage)
	}
	if d.isolatedNetwork != "" {
		_, err := ctrctl.NetworkRm(nil, d.isolatedNetwork)
		if err != nil {
			log.Warn(fmt.Sprintf("network cleanup: %s", err))
		}
	}
	if d.outputDir != "" {
		log.Result(fmt.Sprintf("Output: %s", d.outputDir))
	}
}

func (d *CtrEnv) collectRelayLogs() {
	logsArgs := append(ctrctl.Cli, "logs", d.relayContainer)
	cmd := exec.Command(logsArgs[0], logsArgs[1:]...)
	out, err := cmd.CombinedOutput()
	if err != nil || len(out) == 0 {
		return
	}
	logPath := filepath.Join(d.outputDir, "relay.log")
	os.WriteFile(logPath, out, 0644) //nolint:errcheck
}

func (d *CtrEnv) collectOutput() {
	// Collect ledger (compress if enabled).
	ledgerSrc := filepath.Join(d.ledgerDir, "ledger")
	if d.buildConfig.Compress {
		d.compressFile(ledgerSrc,
			filepath.Join(d.outputDir, "ledger.zst"))
	} else {
		os.Rename(ledgerSrc, //nolint:errcheck
			filepath.Join(d.outputDir, "ledger"))
	}

	// Collect CA cert.
	caSrc := filepath.Join(d.ledgerDir, "ca.cert.pem")
	if d.buildConfig.Compress {
		d.compressFile(caSrc,
			filepath.Join(d.outputDir, "ca.cert.pem.zst"))
	} else {
		os.Rename(caSrc, //nolint:errcheck
			filepath.Join(d.outputDir, "ca.cert.pem"))
	}

	// Collect artifacts by name (resolve symlinks to get real content).
	artDir := filepath.Join(d.ledgerDir, "artifacts")
	if entries, err := os.ReadDir(artDir); err == nil {
		outArt := filepath.Join(d.outputDir, "artifacts")
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

	os.RemoveAll(d.ledgerDir) //nolint:errcheck

	// Write submitted Dockerfile.
	if d.buildConfig.Containerfile != "" {
		submitted, err := os.ReadFile(d.buildConfig.Containerfile)
		if err == nil {
			os.WriteFile( //nolint:errcheck
				filepath.Join(d.outputDir, "Dockerfile.submitted"),
				submitted, 0644)
		}
	}

	// The actual (rewritten) Containerfile was in .warden/ which we're
	// about to delete. Save it if it still exists.
	actual := filepath.Join(d.wardenDirPath(), "Containerfile")
	if data, err := os.ReadFile(actual); err == nil {
		os.WriteFile( //nolint:errcheck
			filepath.Join(d.outputDir, "Dockerfile.actual"),
			data, 0644)
	}
}

func (d *CtrEnv) moveDir(src, dst string) {
	entries, err := os.ReadDir(src)
	if err != nil {
		return
	}
	for _, entry := range entries {
		s := filepath.Join(src, entry.Name())
		if d.buildConfig.Compress {
			d.compressFile(s, filepath.Join(dst, entry.Name()+".zst"))
		} else {
			os.Rename(s, filepath.Join(dst, entry.Name())) //nolint:errcheck
		}
	}
}

// compressedMagic identifies already-compressed formats by their first bytes.
var compressedMagic = [][]byte{
	{0x1f, 0x8b},             // gzip
	{0x28, 0xb5, 0x2f, 0xfd}, // zstd
	{0xfd, 0x37, 0x7a, 0x58}, // xz
	{0x50, 0x4b, 0x03, 0x04}, // zip/jar/whl
	{0x42, 0x5a, 0x68},       // bzip2
}

func isCompressedFile(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	buf := make([]byte, 4)
	n, _ := f.Read(buf)
	for _, magic := range compressedMagic {
		if n >= len(magic) {
			match := true
			for i, b := range magic {
				if buf[i] != b {
					match = false
					break
				}
			}
			if match {
				return true
			}
		}
	}
	return false
}

func (d *CtrEnv) compressFile(src, dst string) {
	if isCompressedFile(src) {
		uncompDst := strings.TrimSuffix(dst, ".zst")
		os.Rename(src, uncompDst) //nolint:errcheck
		return
	}

	in, err := os.Open(src)
	if err != nil {
		return
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return
	}

	enc, err := zstd.NewWriter(out)
	if err != nil {
		out.Close()
		os.Remove(dst) //nolint:errcheck
		return
	}
	io.Copy(enc, in) //nolint:errcheck
	enc.Close()
	out.Close()
}

func (d *CtrEnv) doBuild() error {
	return nil
}

func (d *CtrEnv) editContainerfile() error {
	// Create an edited copy in .warden/Containerfile — never modify the original.
	origfile, err := os.Open(d.buildConfig.Containerfile)
	if err != nil {
		return fmt.Errorf("error opening containerfile: %w", err)
	}
	defer origfile.Close()

	ctrfilePath := filepath.Join(d.wardenDirPath(), "Containerfile")
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

		if strings.HasPrefix(line, "FROM ") {
			_, _ = ctrfile.WriteString(line + "\n")
			if len(enventries) > 0 {
				_, _ = ctrfile.WriteString("ENV " +
					strings.Join(enventries, " \\\n") + "\n")
			}
			_, _ = ctrfile.WriteString("COPY .warden /.warden\n")
			_, _ = ctrfile.WriteString(
				"RUN find /.warden/ext.d/ -exec sh {} \\;\n")
			continue
		}

		if isCopyDirective(line) {
			rewritten, err := d.rewriteCopy(line)
			if err != nil {
				return fmt.Errorf("error rewriting COPY: %w", err)
			}
			_, _ = ctrfile.WriteString(rewritten + "\n")
			continue
		}

		_, _ = ctrfile.WriteString(line + "\n")
	}

	return nil
}

func isCopyDirective(line string) bool {
	trimmed := strings.TrimSpace(line)
	return strings.HasPrefix(trimmed, "COPY ") ||
		strings.HasPrefix(trimmed, "COPY\t")
}

func (d *CtrEnv) rewriteCopy(line string) (string, error) {
	trimmed := strings.TrimSpace(line)
	args := trimmed[5:] // strip "COPY "

	if strings.Contains(args, "--from=") {
		return line, nil
	}

	chown, chmod, positional := parseCopyFlags(args)
	if len(positional) < 2 {
		return line, nil
	}

	dest := positional[len(positional)-1]
	sources := positional[:len(positional)-1]

	var files []string
	for _, src := range sources {
		expanded, err := d.expandContextPath(src)
		if err != nil {
			return "", err
		}
		files = append(files, expanded...)
	}

	if len(files) == 0 {
		return "# (empty COPY: no matching files)", nil
	}

	run := buildFetchRun(files, dest)
	if chown != "" {
		run += fmt.Sprintf(" && \\\n    chown -R %s %s", chown, dest)
	}
	if chmod != "" {
		run += fmt.Sprintf(" && \\\n    chmod -R %s %s", chmod, dest)
	}
	return run, nil
}

func parseCopyFlags(args string) (chown, chmod string, positional []string) {
	for _, p := range strings.Fields(args) {
		switch {
		case strings.HasPrefix(p, "--chown="):
			chown = strings.TrimPrefix(p, "--chown=")
		case strings.HasPrefix(p, "--chmod="):
			chmod = strings.TrimPrefix(p, "--chmod=")
		default:
			positional = append(positional, p)
		}
	}
	return
}

func buildFetchRun(files []string, dest string) string {
	if len(files) == 1 && !strings.HasSuffix(dest, "/") {
		dir := dest[:strings.LastIndex(dest, "/")+1]
		if dir != "" {
			return fmt.Sprintf(
				"RUN mkdir -p %s && curl -fsSL -o %s \"http://cwd/%s\"",
				dir, dest, files[0])
		}
		return fmt.Sprintf(
			"RUN curl -fsSL -o %s \"http://cwd/%s\"", dest, files[0])
	}

	d := dest
	if !strings.HasSuffix(d, "/") {
		d += "/"
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "RUN mkdir -p %s && \\\n    printf '%%s\\n'", dest)
	for _, f := range files {
		fmt.Fprintf(&sb, " '%s'", f)
	}
	fmt.Fprintf(&sb,
		" | \\\n    xargs -P8 -I{} sh -c "+
			"'mkdir -p \"%s$(dirname \"{}\")\" && "+
			"curl -fsSL -o \"%s{}\" \"http://cwd/{}\"'",
		d, d)
	return sb.String()
}

// expandContextPath resolves a source path or glob against the context.
func (d *CtrEnv) expandContextPath(src string) ([]string, error) {
	ctxDir := d.buildConfig.Context
	fullPattern := filepath.Join(ctxDir, src)

	info, err := os.Stat(fullPattern)
	if err == nil && info.IsDir() {
		// Directory: walk and collect all files.
		var files []string
		err = filepath.Walk(fullPattern, func(
			path string, fi os.FileInfo, walkErr error,
		) error {
			if walkErr != nil {
				return walkErr
			}
			if fi.IsDir() {
				switch fi.Name() {
				case wardenDir, ".git":
					return filepath.SkipDir
				}
				return nil
			}
			rel, _ := filepath.Rel(ctxDir, path)
			files = append(files, rel)
			return nil
		})
		return files, err
	}

	// Try as glob pattern.
	matches, err := filepath.Glob(fullPattern)
	if err != nil {
		return nil, err
	}
	var files []string
	for _, m := range matches {
		info, err := os.Stat(m)
		if err != nil || info.IsDir() {
			continue
		}
		rel, _ := filepath.Rel(ctxDir, m)
		if strings.HasPrefix(rel, wardenDir+"/") ||
			strings.HasPrefix(rel, ".git/") {
			continue
		}
		files = append(files, rel)
	}
	return files, nil
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
