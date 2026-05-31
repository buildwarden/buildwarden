package qemu

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"syscall"
	"time"
)

// vmProcess wraps a running QEMU subprocess.
type vmProcess struct {
	cmd  *exec.Cmd
	name string
}

func (d *Driver) startRelayVM(
	kernelPath, initrdPath, sharedDir, socketPath string,
) (*vmProcess, error) {
	arch := hostQEMUArch()
	accel := d.detectAccel()
	binary := d.qemuBinary(arch)

	virtfs := fmt.Sprintf(
		"local,path=%s,mount_tag=shared,security_model=none,id=shared0",
		sharedDir)
	buildNet := fmt.Sprintf(
		"stream,id=buildnet,server=on,addr.type=unix,addr.path=%s",
		socketPath)

	args := []string{
		"-machine", fmt.Sprintf("virt,accel=%s", accel),
		"-cpu", cpuForAccel(accel, arch),
		"-m", "512",
		"-smp", "2",
		"-nodefaults",
		"-display", "none",
		"-chardev", "stdio,id=ser0",
		"-serial", "chardev:ser0",
		"-kernel", kernelPath,
		"-initrd", initrdPath,
		"-append", consoleArg(arch),
		"-virtfs", virtfs,
		"-device", "virtio-net-pci,netdev=buildnet",
		"-netdev", buildNet,
		"-device", "virtio-net-pci,netdev=usernet",
		"-netdev", "user,id=usernet,restrict=off",
		"-device", "virtio-rng-pci",
	}

	cmd := exec.Command(binary, args...)
	cmd.Stdout = d.vmOutput()
	cmd.Stderr = d.vmOutput()
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("starting relay QEMU: %w", err)
	}

	return &vmProcess{cmd: cmd, name: "relay"}, nil
}

// buildVMConfig holds parameters for the build VM.
type buildVMConfig struct {
	Arch     string
	CPUs     int
	MemoryMB int

	// Disk image boot (production path)
	DiskImage string
	SeedISO   string // cloud-init NoCloud seed ISO

	// Direct kernel boot (lightweight/test path)
	Kernel string
	Initrd string
}

func (d *Driver) startBuildVM(
	cfg *buildVMConfig, socketPath string,
) (*vmProcess, error) {
	accel := d.detectAccel()
	binary := d.qemuBinary(cfg.Arch)

	relayNet := fmt.Sprintf(
		"stream,id=relaynet,addr.type=unix,addr.path=%s", socketPath)

	args := []string{
		"-machine", fmt.Sprintf("virt,accel=%s", accel),
		"-cpu", cpuForAccel(accel, cfg.Arch),
		"-m", fmt.Sprintf("%d", cfg.MemoryMB),
		"-smp", fmt.Sprintf("%d", cfg.CPUs),
		"-nodefaults",
		"-display", "none",
		"-chardev", "stdio,id=ser0",
		"-serial", "chardev:ser0",
	}

	if cfg.DiskImage != "" {
		drive := fmt.Sprintf(
			"file=%s,format=qcow2,if=virtio", cfg.DiskImage)
		args = append(args,
			"-drive", drive,
			"-bios", efiCodePath(cfg.Arch),
		)
		if cfg.SeedISO != "" {
			args = append(args,
				"-drive", fmt.Sprintf(
					"file=%s,format=raw,if=none,id=cidata,"+
						"media=cdrom,readonly=on", cfg.SeedISO),
				"-device", "virtio-blk-pci,drive=cidata",
			)
		}
	} else {
		args = append(args,
			"-kernel", cfg.Kernel,
			"-initrd", cfg.Initrd,
			"-append", consoleArg(cfg.Arch),
		)
	}

	args = append(args,
		"-device", "virtio-net-pci,netdev=relaynet",
		"-netdev", relayNet,
		"-device", "virtio-rng-pci",
	)

	cmd := exec.Command(binary, args...)
	cmd.Stdout = d.vmOutput()
	cmd.Stderr = d.vmOutput()
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("starting build QEMU: %w", err)
	}

	return &vmProcess{cmd: cmd, name: "build"}, nil
}

func (p *vmProcess) stop() {
	if p.cmd == nil || p.cmd.Process == nil {
		return
	}
	_ = p.cmd.Process.Signal(syscall.SIGTERM)
	done := make(chan struct{})
	go func() {
		_ = p.cmd.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		_ = p.cmd.Process.Kill()
		<-done
	}
}

func (p *vmProcess) wait() error {
	return p.cmd.Wait()
}

func (d *Driver) vmOutput() io.Writer {
	if d.Verbose {
		return os.Stderr
	}
	return io.Discard
}

func consoleArg(arch string) string {
	if arch == "x86_64" {
		return "console=ttyS0"
	}
	return "console=ttyAMA0"
}

func cpuForAccel(accel, arch string) string {
	switch {
	case accel == "hvf" && arch == "aarch64":
		return "host"
	case accel == "kvm":
		return "host"
	case arch == "aarch64":
		return "cortex-a72"
	case arch == "x86_64":
		return "qemu64"
	default:
		return "max"
	}
}

func efiCodePath(arch string) string {
	switch arch {
	case "aarch64":
		candidates := []string{
			"/opt/homebrew/share/qemu/edk2-aarch64-code.fd",
			"/usr/share/qemu/edk2-aarch64-code.fd",
			"/usr/share/OVMF/AAVMF_CODE.fd",
		}
		for _, p := range candidates {
			if _, err := os.Stat(p); err == nil {
				return p
			}
		}
		return "edk2-aarch64-code.fd"
	case "x86_64":
		candidates := []string{
			"/opt/homebrew/share/qemu/edk2-x86_64-code.fd",
			"/usr/share/qemu/edk2-x86_64-code.fd",
			"/usr/share/OVMF/OVMF_CODE.fd",
		}
		for _, p := range candidates {
			if _, err := os.Stat(p); err == nil {
				return p
			}
		}
		return "edk2-x86_64-code.fd"
	default:
		return ""
	}
}
