package warden

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"warden/relay"

	"github.com/google/uuid"
	"github.com/lesiw/ctrctl"
)

var wardenDir = ".warden"

var exts = []Extension{
	&ExtTrustStore{},
	&ExtPip{},
	&ExtBazel{},
}

type CtrEnv struct {
	isolatedNetwork string
	buildConfig     *BuildConfig
	buildContainer  string
	relayContainer  string
}

type relayPorts struct {
	http  int
	https int
	dns   int
}

func NewCtrEnv() BuildEnv {
	return &CtrEnv{}
}

func (d *CtrEnv) inBuildEnv(config *BuildConfig, fn func() error) error {
	ctrctl.Verbose = true
	d.buildConfig = config
	proxyErr := make(chan error)
	fnErr := make(chan error)

	defer d.teardownBuildEnv()
	if err := d.createBuildEnv(proxyErr); err != nil {
		return err
	}

	go func() { fnErr <- fn() }()

	// TODO: loop necessary?
	for i := 0; i < 2; i++ {
		select {
		case err := <-proxyErr:
			if err != nil {
				return fmt.Errorf("proxy error: %w", err)
			}
		case err := <-fnErr:
			if err != nil {
				return fmt.Errorf("build error: %w", err)
			}
			return nil
		}
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
				Tty:         true,
			},
			d.buildContainer,
			"docker", "build", "--network=host", "-f", config.Containerfile, ".",
		)
		return err
	})
}

func (d *CtrEnv) Shell(config *BuildConfig) error {
	shell := func() error {
		_, err := ctrctl.ContainerExec(
			&ctrctl.ContainerExecOpts{
				Interactive: true,
				Tty:         true,
			},
			d.buildContainer,
			"sh",
		)
		return err
	}
	return d.inBuildEnv(config, shell)
}

func (d *CtrEnv) createBuildEnv(proxyErr chan<- error) error {
	err := os.MkdirAll(filepath.Join(d.wardenDirPath(), "ext.d"), 0755)
	if err != nil {
		return fmt.Errorf("error creating %s: %w", d.wardenDirPath(), err)
	}

	for _, ext := range exts {
		if err := ext.BeforeBuild(d); err != nil {
			return err
		}
	}

	if err := d.createNetwork(); err != nil {
		return err
	}
	if err := d.editContainerfile(); err != nil {
		return err
	}
	if err := d.startBuildContainer(); err != nil {
		return err
	}

	listenIp := net.IPv4zero
	hostIp := net.IPv4(172, 24, 0, 1)

	ports, err := getRelayPorts()
	if err != nil {
		return err
	}

	output, err := os.OpenFile("out.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}

	err = relay.NewLedger(output)
	if err != nil {
		return err
	}
	startRelays(listenIp, ports, proxyErr)

	if err := d.configureBuildContainer(hostIp, ports); err != nil {
		return err
	}

	return nil
}

func getRelayPorts() (ports relayPorts, err error) {
	ports.http, err = relay.EphemeralPort(net.IPv4zero, "tcp")
	if err != nil {
		err = fmt.Errorf("error getting http proxy port: %w", err)
		return
	}
	ports.https, err = relay.EphemeralPort(net.IPv4zero, "tcp")
	if err != nil {
		err = fmt.Errorf("error getting https proxy port: %w", err)
		return
	}
	ports.dns, err = relay.EphemeralPort(net.IPv4zero, "udp")
	if err != nil {
		err = fmt.Errorf("error getting dns proxy port: %w", err)
		return
	}
	return
}

func startRelays(ip net.IP, ports relayPorts, err chan<- error) {
	go func() {
		err <- relay.RunHttp(
			net.TCPAddr{
				IP:   ip,
				Port: ports.http,
			},
		)
	}()
	go func() {
		err <- relay.RunHttps(
			net.TCPAddr{
				IP:   ip,
				Port: ports.https,
			},
		)
	}()
	fmt.Printf("starting DNS server on port %d\n", ports.dns)
	go func() {
		err <- relay.RunDns(
			net.TCPAddr{
				IP:   ip,
				Port: ports.dns,
			},
		)
	}()
}

func (d *CtrEnv) teardownBuildEnv() {
	relay.FinishLedger()
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
	if d.isolatedNetwork != "" {
		_, err := ctrctl.NetworkRm(nil, d.isolatedNetwork)
		if err != nil {
			fmt.Fprintf(os.Stderr, "network cleanup failure: %s\n", err)
		}
	}
}

func (d *CtrEnv) createNetwork() error {
	id, err := ctrctl.NetworkCreate(
		&ctrctl.NetworkCreateOpts{
			Driver:   "bridge",
			Gateway:  "172.24.0.1",
			Internal: true,
			Subnet:   "172.24.0.0/29", // TODO: randomize network subnet.
		},
		"warden-net-"+uuid.New().String(),
	)
	if err != nil {
		return err
	}
	d.isolatedNetwork = id
	return nil
}

func (d *CtrEnv) startBuildContainer() error {
	id, err := ctrctl.ContainerRun(
		&ctrctl.ContainerRunOpts{
			Detach:      true,
			Interactive: true,
			Name:        "warden-build-" + uuid.New().String(),
			Network:     "warden",
			Privileged:  true, // TODO: investigate nonprivileged alternatives.
			Tty:         true,
			Workdir:     "/work",
		},
		"docker:dind",
		"cat",
	)
	if err != nil {
		return err
	}
	d.buildContainer = id

	return nil
}

func (d *CtrEnv) configureBuildContainer(host net.IP, ports relayPorts) error {
	_, err := ctrctl.ContainerCp(
		nil,
		d.buildConfig.Context+"/.",
		fmt.Sprintf("%s:/work", d.buildContainer),
	)
	if err != nil {
		return err
	}

	cmds := [][]string{
		{"iptables", "-t", "nat", "-A", "OUTPUT", "-p", "tcp", "--dport", "80",
			"-j", "DNAT", "--to-destination",
			fmt.Sprintf("%s:%d", host.String(), ports.http)},
		{"iptables", "-t", "nat", "-A", "OUTPUT", "-p", "tcp", "--dport", "443",
			"-j", "DNAT", "--to-destination",
			fmt.Sprintf("%s:%d", host.String(), ports.https)},
		{"iptables",
			"-t", "nat",
			"-A", "OUTPUT",
			"-d", host.String(),
			"-p", "udp", "--dport", "53",
			"-j", "DNAT", "--to-destination",
			fmt.Sprintf("%s:%d", host.String(), ports.dns)},
		{"iptables",
			"-t", "nat",
			"-A", "POSTROUTING",
			"-s", host.String(),
			"-p", "udp", "--sport", fmt.Sprintf("%d", ports.dns),
			"-j", "SNAT", "--to", ":53"},
		{"sh", "-c", fmt.Sprintf(`echo "nameserver %s" > /etc/resolv.conf`,
			host.String())},
		{"ln", "-s", "/work/.warden", "/.warden"},
		{"find", "/.warden/ext.d/", "-exec", "sh", "{}", ";"},
	}
	for _, cmd := range cmds {
		_, err := ctrctl.ContainerExec(
			&ctrctl.ContainerExecOpts{Privileged: true},
			d.buildContainer,
			cmd[0],
			cmd[1:]...,
		)
		if err != nil {
			return err
		}
	}

	_, err = ctrctl.ContainerExec(
		&ctrctl.ContainerExecOpts{Detach: true},
		d.buildContainer,
		"dockerd",
	)
	if err != nil {
		return err
	}

	return nil
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
