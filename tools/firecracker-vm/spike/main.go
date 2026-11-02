//go:build linux && spike

// Command spike is the Phase 0 Firecracker spike (implementation plan step 1d
// of docs/design/firecracker-driver.md): one build on one network slot made by
// tools/firecracker-vm/slots.sh, with both VMs started from hand-written
// Firecracker configs. Throwaway; driver/firecracker replaces it.
//
//	go run -tags spike ./tools/firecracker-vm/spike -context DIR -out DIR
package main

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/buildwarden/buildwarden/driver"
)

type opts struct {
	slot                      int
	kernel, relayBase         string
	buildBase                 string
	relayBin, ioBin           string
	contextDir, containerfile string
	outDir, dns               string
	image, seed, partUUID     string
	cpus, memMB, relayMemMB   int
	verbose                   bool
}

func main() {
	var o opts
	flag.IntVar(&o.slot, "slot", 0, "network slot")
	flag.StringVar(&o.kernel, "kernel", "tools/firecracker-vm/output/vmlinux", "guest kernel")
	flag.StringVar(&o.relayBase, "relay-initramfs", "tools/relay-vm/output/x86_64/initramfs.cpio.gz", "relay base initramfs")
	flag.StringVar(&o.buildBase, "build-initramfs", "tools/firecracker-vm/output/initramfs.cpio.gz", "direct-boot base initramfs")
	flag.StringVar(&o.relayBin, "relay", "", "relay binary (linux/amd64)")
	flag.StringVar(&o.ioBin, "warden-io", "", "warden-io binary (linux/amd64)")
	flag.StringVar(&o.contextDir, "context", "", "build context")
	flag.StringVar(&o.containerfile, "containerfile", "", "Containerfile (default <context>/Containerfile)")
	flag.StringVar(&o.outDir, "out", "warden-output", "output directory")
	flag.StringVar(&o.dns, "dns", "", "upstream DNS for the relay, comma-separated")
	flag.StringVar(&o.image, "image", "", "raw build disk (disk path; per-build copy)")
	flag.StringVar(&o.seed, "seed", "", "CIDATA seed ISO for the disk path")
	flag.StringVar(&o.partUUID, "partuuid", "", "root partition PARTUUID for the disk path")
	flag.IntVar(&o.cpus, "cpus", 2, "build VM vCPUs")
	flag.IntVar(&o.memMB, "memory", 2048, "build VM memory (MiB)")
	flag.IntVar(&o.relayMemMB, "relay-memory", 512, "relay VM memory (MiB)")
	flag.BoolVar(&o.verbose, "verbose", false, "show consoles and collector traffic")
	flag.Parse()
	if err := run(o); err != nil {
		fmt.Fprintln(os.Stderr, "spike:", err)
		os.Exit(1)
	}
}

func slotAddr(slot, off int) string {
	n := 4*slot + off
	return fmt.Sprintf("100.64.%d.%d", 88+n/256, n%256)
}

func run(o opts) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if o.containerfile == "" && o.contextDir != "" {
		o.containerfile = filepath.Join(o.contextDir, "Containerfile")
	}
	if err := os.MkdirAll(o.outDir, 0o755); err != nil {
		return err
	}
	work, err := os.MkdirTemp("", "fc-spike-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(work)

	req := &driver.BuildRequest{
		ContextDir: o.contextDir, Containerfile: o.containerfile, OutputDir: o.outDir,
	}
	script, err := driver.StageBuildScript(req, work)
	if err != nil {
		return err
	}
	if err := driver.RecordInputs(o.outDir, req, script); err != nil {
		return err
	}

	sess, err := driver.StartSession(driver.SessionConfig{
		OutputDir: o.outDir, ContextDir: o.contextDir, BuildScript: script,
		OutputTee: os.Stdout, Verbose: o.verbose,
	})
	if err != nil {
		return err
	}
	defer sess.Close()

	relayInitrd := filepath.Join(work, "relay-initrd")
	if err := appendFile(o.relayBase, relayInitrd, o.relayBin, "sbin/relay"); err != nil {
		return err
	}
	token, err := driver.RandomToken()
	if err != nil {
		return err
	}
	host, relay := slotAddr(o.slot, 1), slotAddr(o.slot, 2)
	relayArgs := "console=ttyS0 reboot=k panic=1 pci=off warden.net=static" +
		" warden.nat_ip=" + relay + " warden.nat_prefix=30 warden.nat_gw=" + host +
		" warden.dns=" + o.dns + " warden.claim_token=" + token
	relayCfg := vmConfig(o.kernel, relayInitrd, relayArgs, 2, o.relayMemMB,
		[]string{fmt.Sprintf("wfc%dr", o.slot), fmt.Sprintf("wfc%du", o.slot)}, nil, true)

	t0 := time.Now()
	relayVM, err := startVM(work, "relay", relayCfg, o.verbose)
	if err != nil {
		return err
	}
	defer relayVM.stop()

	if _, err := sess.ClaimRelay(ctx, net.JoinHostPort(relay, "8301"), token, req, nil); err != nil {
		return fmt.Errorf("claim: %w (relay console: %s)", err, relayVM.console)
	}
	fmt.Fprintf(os.Stderr, "spike: relay claimed in %s\n", time.Since(t0).Round(time.Millisecond))

	var buildCfg map[string]any
	buildIf := []string{fmt.Sprintf("wfc%dg", o.slot)}
	if o.image == "" {
		buildInitrd := filepath.Join(work, "build-initrd")
		if err := appendFile(o.buildBase, buildInitrd, o.ioBin, "bin/warden-io"); err != nil {
			return err
		}
		buildCfg = vmConfig(o.kernel, buildInitrd, "console=ttyS0 reboot=k panic=1 pci=off",
			o.cpus, o.memMB, buildIf, nil, false)
	} else {
		disk := filepath.Join(work, "rootfs.raw")
		if out, err := exec.Command("cp", "--reflink=auto", o.image, disk).CombinedOutput(); err != nil {
			return fmt.Errorf("copy rootfs: %s: %w", out, err)
		}
		drives := []map[string]any{
			{"drive_id": "rootfs", "path_on_host": disk, "is_root_device": true,
				"partuuid": o.partUUID, "is_read_only": false},
			{"drive_id": "seed", "path_on_host": o.seed, "is_root_device": false, "is_read_only": true},
		}
		buildCfg = vmConfig(o.kernel, "", "console=ttyS0 reboot=k panic=1 pci=off",
			o.cpus, o.memMB, buildIf, drives, false)
	}
	t1 := time.Now()
	buildVM, err := startVM(work, "build", buildCfg, o.verbose)
	if err != nil {
		return err
	}
	defer buildVM.stop()

	// Exit ordering (Decision 8): the build VM's exit allows a bounded grace
	// for the completion; the relay's exit fails at once.
	wctx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		select {
		case <-buildVM.done:
			select {
			case <-time.After(10 * time.Second):
				fmt.Fprintln(os.Stderr, "spike: build VM exited with no completion")
				cancel()
			case <-wctx.Done():
			}
		case <-relayVM.done:
			fmt.Fprintln(os.Stderr, "spike: relay VM exited before completion")
			cancel()
		case <-wctx.Done():
		}
	}()
	err = sess.Wait(wctx)
	fmt.Fprintf(os.Stderr, "spike: build finished in %s: %v\n", time.Since(t1).Round(time.Millisecond), err)
	driver.FinalizeOutput(o.outDir, false)
	return err
}

func vmConfig(kernel, initrd, args string, cpus, memMB int, taps []string,
	drives []map[string]any, entropy bool) map[string]any {
	boot := map[string]any{"kernel_image_path": kernel, "boot_args": args}
	if initrd != "" {
		boot["initrd_path"] = initrd
	}
	var nics []map[string]any
	for i, t := range taps {
		nics = append(nics, map[string]any{"iface_id": fmt.Sprintf("eth%d", i), "host_dev_name": t})
	}
	if drives == nil {
		drives = []map[string]any{}
	}
	cfg := map[string]any{
		"boot-source":        boot,
		"drives":             drives,
		"machine-config":     map[string]any{"vcpu_count": cpus, "mem_size_mib": memMB},
		"network-interfaces": nics,
	}
	if entropy {
		cfg["entropy"] = map[string]any{}
	}
	return cfg
}

type vm struct {
	cmd     *exec.Cmd
	done    chan struct{}
	console string
}

func startVM(work, name string, cfg map[string]any, verbose bool) (*vm, error) {
	path := filepath.Join(work, name+".json")
	b, _ := json.MarshalIndent(cfg, "", "  ")
	if err := os.WriteFile(path, b, 0o600); err != nil {
		return nil, err
	}
	console := filepath.Join(work, name+".console")
	f, err := os.Create(console)
	if err != nil {
		return nil, err
	}
	cmd := exec.Command("firecracker", "--no-api", "--config-file", path)
	cmd.Stdin = nil // /dev/null: the console is output only
	var w io.Writer = f
	if verbose {
		w = io.MultiWriter(f, prefixWriter{name})
	}
	cmd.Stdout, cmd.Stderr = w, w
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true, Pdeathsig: syscall.SIGKILL}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start %s VM: %w", name, err)
	}
	v := &vm{cmd: cmd, done: make(chan struct{}), console: console}
	go func() {
		_ = cmd.Wait()
		f.Close()
		// Keep the console beside the outputs for the spike's evidence.
		_ = copyFile(console, filepath.Join(filepath.Dir(work), "fc-spike-"+name+".console"))
		close(v.done)
	}()
	return v, nil
}

func (v *vm) stop() {
	select {
	case <-v.done:
	default:
		_ = v.cmd.Process.Kill()
		<-v.done
	}
}

type prefixWriter struct{ name string }

func (p prefixWriter) Write(b []byte) (int, error) {
	for _, l := range strings.SplitAfter(string(b), "\n") {
		if l != "" {
			fmt.Fprintf(os.Stderr, "[%s] %s", p.name, l)
		}
	}
	return len(b), nil
}

func copyFile(src, dst string) error {
	b, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, b, 0o644)
}

// appendFile writes base followed by a gzip'd newc cpio holding src at path.
func appendFile(base, out, src, path string) error {
	b, err := os.ReadFile(base)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	f, err := os.Create(out)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.Write(b); err != nil {
		return err
	}
	zw := gzip.NewWriter(f)
	if err := newc(zw, path, 0o100755, data); err != nil {
		return err
	}
	if err := newc(zw, "TRAILER!!!", 0, nil); err != nil {
		return err
	}
	if err := zw.Close(); err != nil {
		return err
	}
	return f.Close()
}

func newc(w io.Writer, name string, mode uint32, data []byte) error {
	nlink := 1
	if name == "TRAILER!!!" {
		nlink = 0
	}
	hdr := fmt.Sprintf("070701%08X%08X%08X%08X%08X%08X%08X%08X%08X%08X%08X%08X%08X",
		0, mode, 0, 0, nlink, 0, len(data), 0, 0, 0, 0, len(name)+1, 0)
	buf := append([]byte(hdr), name...)
	buf = append(buf, 0)
	for len(buf)%4 != 0 {
		buf = append(buf, 0)
	}
	if _, err := w.Write(buf); err != nil {
		return err
	}
	if _, err := w.Write(data); err != nil {
		return err
	}
	_, err := w.Write(make([]byte, (4-len(data)%4)%4))
	return err
}
