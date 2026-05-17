package qemu

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

// vmProcess wraps a running QEMU subprocess.
type vmProcess struct {
	cmd  *exec.Cmd
	name string
}

// startRelayVM boots the relay VM using direct kernel boot (no disk image).
// Two network interfaces: a socket for the isolated build link, and user-net
// for upstream internet access.
func (d *Driver) startRelayVM(kernelPath, initrdPath, sharedDir, socketPath string) (*vmProcess, error) {
	accel := d.detectAccel()
	binary := d.qemuBinary("aarch64")

	args := []string{
		"-machine", fmt.Sprintf("virt,accel=%s", accel),
		"-cpu", cpuForAccel(accel, "aarch64"),
		"-m", "512",
		"-smp", "2",
		"-nographic",

		// Direct kernel boot
		"-kernel", kernelPath,
		"-initrd", initrdPath,
		"-append", "console=ttyAMA0",

		// Shared directory via virtio-9p (works without virtiofsd daemon)
		"-virtfs", fmt.Sprintf("local,path=%s,mount_tag=shared,security_model=mapped-xattr,id=shared0", sharedDir),

		// Network 1: isolated link to build VM via socket
		"-device", "virtio-net-pci,netdev=buildnet",
		"-netdev", fmt.Sprintf("socket,id=buildnet,listen=%s", socketPath),

		// Network 2: user-mode networking for internet (upstream requests)
		"-device", "virtio-net-pci,netdev=usernet",
		"-netdev", "user,id=usernet,restrict=off",

		// No display, no audio
		"-display", "none",
		"-nodefaults",

		// virtio-rng for fast boot entropy
		"-device", "virtio-rng-pci",
	}

	cmd := exec.Command(binary, args...)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("starting relay QEMU: %w", err)
	}

	return &vmProcess{cmd: cmd, name: "relay"}, nil
}

// startBuildVM boots the build VM from a disk image with a single network
// interface connected to the relay VM's socket (isolated, relay is sole gateway).
func (d *Driver) startBuildVM(req *buildVMConfig, sharedDir, socketPath string) (*vmProcess, error) {
	accel := d.detectAccel()
	binary := d.qemuBinary(req.Arch)

	args := []string{
		"-machine", fmt.Sprintf("virt,accel=%s", accel),
		"-cpu", cpuForAccel(accel, req.Arch),
		"-m", fmt.Sprintf("%d", req.MemoryMB),
		"-smp", fmt.Sprintf("%d", req.CPUs),
		"-nographic",

		// Boot from disk image
		"-drive", fmt.Sprintf("file=%s,format=qcow2,if=virtio", req.DiskImage),

		// EFI firmware (required for arm64 guests)
		"-bios", efiCodePath(req.Arch),

		// Shared directory via virtio-9p
		"-virtfs", fmt.Sprintf("local,path=%s,mount_tag=shared,security_model=mapped-xattr,id=shared0", sharedDir),

		// Network: connect to relay VM's socket (isolated — sole path out)
		"-device", "virtio-net-pci,netdev=relaynet",
		"-netdev", fmt.Sprintf("socket,id=relaynet,connect=%s", socketPath),

		// No display
		"-display", "none",
		"-nodefaults",

		"-device", "virtio-rng-pci",
	}

	cmd := exec.Command(binary, args...)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
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
	// Send SIGTERM first for graceful shutdown
	p.cmd.Process.Signal(syscall.SIGTERM)
	// Give it 5 seconds, then force kill
	done := make(chan struct{})
	go func() {
		p.cmd.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-timeAfter(5):
		p.cmd.Process.Kill()
		<-done
	}
}

func (p *vmProcess) wait() error {
	return p.cmd.Wait()
}

// buildVMConfig holds parameters specific to the build VM.
type buildVMConfig struct {
	Arch      string // "aarch64" or "x86_64"
	CPUs      int
	MemoryMB  int
	DiskImage string
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
		// Standard homebrew location
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

func timeAfter(seconds int) <-chan struct{} {
	ch := make(chan struct{})
	go func() {
		select {
		case <-make(chan struct{}): // never fires, sleep via select
		}
	}()
	_ = seconds
	// Simple implementation using time.After would need the time import
	// but we already avoid it for the stub. Use a goroutine with sleep.
	go func() {
		for i := 0; i < seconds*10; i++ {
			// busy-ish wait at 100ms intervals
			syscall.Select(0, nil, nil, nil, &syscall.Timeval{Usec: 100000})
		}
		close(ch)
	}()
	return ch
}
