package firecracker

import (
	"encoding/json"
	"fmt"
	"os"
)

// Each VM is one `firecracker --no-api --config-file <json>` process (Decision
// 8): no API socket, no vsock, no MMDS, stdin /dev/null.

// Resource defaults. The relay ran its builds at 128 to 512 MiB in the Phase 0
// spike; 256 leaves room for large captured bodies.
const (
	relayVCPUs    = 2
	relayMemoryMB = 256
	buildVCPUs    = 4
	buildMemoryMB = 4096
)

// vmConfig is a Firecracker --config-file document.
type vmConfig struct {
	BootSource bootSource `json:"boot-source"`
	Drives     []drive    `json:"drives"`
	Machine    machine    `json:"machine-config"`
	NICs       []nic      `json:"network-interfaces"`
	Entropy    *struct{}  `json:"entropy,omitempty"`
}

type bootSource struct {
	Kernel   string `json:"kernel_image_path"`
	Initrd   string `json:"initrd_path,omitempty"`
	BootArgs string `json:"boot_args"`
}

type drive struct {
	ID       string `json:"drive_id"`
	Path     string `json:"path_on_host"`
	Root     bool   `json:"is_root_device"`
	PartUUID string `json:"partuuid,omitempty"`
	ReadOnly bool   `json:"is_read_only"`
}

type machine struct {
	VCPUs    int `json:"vcpu_count"`
	MemoryMB int `json:"mem_size_mib"`
}

type nic struct {
	ID     string `json:"iface_id"`
	HostIf string `json:"host_dev_name"`
}

// baseArgs is every VM's kernel command line: the serial console, a reset on
// reboot, and a reset one second after a panic, so the VMM exits either way.
const baseArgs = "console=ttyS0 reboot=k panic=1 pci=off"

// relayArgs is the relay VM's command line (Decision 5): a static upstream on
// the slot's /30 and the per-build claim token. Its resolvers are not a boot
// value: the claim carries them (upstream_dns, RELAY-08).
func relayArgs(s Slot, token string) string {
	return baseArgs + " warden.net=static warden.nat_ip=" + s.RelayAddr() +
		" warden.nat_prefix=30 warden.nat_gw=" + s.HostAddr() +
		" warden.claim_token=" + token
}

// relayConfig is the relay VM: diskless, eth0 on the slot's bridge, eth1 its
// upstream TAP, and the entropy device so the CRNG seeds before the relay
// makes its keys (RELAY-03).
func relayConfig(s Slot, kernel, initrd, args string) vmConfig {
	return vmConfig{
		BootSource: bootSource{Kernel: kernel, Initrd: initrd, BootArgs: args},
		Drives:     []drive{},
		Machine:    machine{VCPUs: relayVCPUs, MemoryMB: relayMemoryMB},
		NICs: []nic{
			{ID: "eth0", HostIf: s.RelayTap()},
			{ID: "eth1", HostIf: s.UpstreamTap()},
		},
		Entropy: &struct{}{},
	}
}

// buildBoot is how the build VM boots: from a per-build rootfs copy and the
// CIDATA seed (the disk path), or from the direct-boot initramfs.
type buildBoot struct {
	Kernel string
	// Disk path.
	Rootfs   string
	PartUUID string
	Seed     string
	// Direct boot. LinkArgs are the build link's kernel command-line keys
	// the direct-boot init reads.
	Initrd   string
	LinkArgs string
}

// buildConfig is the build VM: its only NIC on the slot's bridge.
func buildConfig(s Slot, b buildBoot, cpus, memMB int) vmConfig {
	args := baseArgs
	if b.LinkArgs != "" {
		args += " " + b.LinkArgs
	}
	c := vmConfig{
		BootSource: bootSource{Kernel: b.Kernel, Initrd: b.Initrd, BootArgs: args},
		Drives:     []drive{},
		Machine:    machine{VCPUs: cpus, MemoryMB: memMB},
		NICs:       []nic{{ID: "eth0", HostIf: s.GuestTap()}},
	}
	if b.Rootfs != "" {
		c.Drives = []drive{
			{ID: "rootfs", Path: b.Rootfs, Root: true, PartUUID: b.PartUUID},
			{ID: "seed", Path: b.Seed, ReadOnly: true},
		}
	}
	return c
}

// effectiveArgs is the command line the guest kernel sees: Firecracker
// appends the root device from the root drive's partuuid, read-write.
func (c vmConfig) effectiveArgs() string {
	args := c.BootSource.BootArgs
	for _, d := range c.Drives {
		if d.Root {
			if d.PartUUID != "" {
				args += " root=PARTUUID=" + d.PartUUID
			} else {
				args += " root=/dev/vda"
			}
			if d.ReadOnly {
				args += " ro"
			} else {
				args += " rw"
			}
		}
	}
	return args
}

// write saves c as a private JSON file.
func (c vmConfig) write(path string) error {
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, append(b, '\n'), 0o600); err != nil {
		return fmt.Errorf("writing VM config: %w", err)
	}
	return nil
}
