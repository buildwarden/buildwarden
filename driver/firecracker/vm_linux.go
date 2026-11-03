//go:build linux

package firecracker

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
)

// vmm is one running firecracker process.
type vmm struct {
	name    string
	cmd     *exec.Cmd
	done    chan struct{}
	console string
	rec     vmmRecord
}

// vmmCommand is the firecracker invocation for a config file: no API socket,
// stdin /dev/null (the serial console is output only, so no host bytes reach
// the guest through it), its own process group, and SIGKILL when warden dies,
// unless the VM is kept (--keep-vms) to outlive it.
func vmmCommand(bin, configPath string, keep bool) *exec.Cmd {
	c := exec.Command(bin, "--no-api", "--config-file", configPath)
	c.Stdin = nil
	attr := &syscall.SysProcAttr{Setpgid: true}
	if !keep {
		attr.Pdeathsig = syscall.SIGKILL
	}
	c.SysProcAttr = attr
	return c
}

// startVMM writes cfg to <work>/<name>.json and starts its VMM, its console
// captured to <work>/<name>.console (and shown on stderr when verbose).
func startVMM(work, name string, cfg vmConfig, verbose, keep bool) (*vmm, error) {
	configPath := filepath.Join(work, name+".json")
	if err := cfg.write(configPath); err != nil {
		return nil, err
	}
	bin, err := exec.LookPath("firecracker")
	if err != nil {
		return nil, err
	}
	console := filepath.Join(work, name+".console")
	f, err := os.OpenFile(console, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return nil, err
	}
	c := vmmCommand(bin, configPath, keep)
	var w io.Writer = f
	if verbose {
		w = io.MultiWriter(f, &prefixWriter{prefix: "[" + name + "] ", w: os.Stderr})
	}
	c.Stdout, c.Stderr = w, w

	v := &vmm{name: name, cmd: c, done: make(chan struct{}), console: console}
	started := make(chan error, 1)
	go func() {
		// Pdeathsig fires when the thread that started the child exits, not
		// the process: hold this thread for the child's whole life.
		runtime.LockOSThread()
		if err := c.Start(); err != nil {
			f.Close()
			started <- err
			return
		}
		started <- nil
		_ = c.Wait()
		f.Close()
		close(v.done)
	}()
	if err := <-started; err != nil {
		return nil, fmt.Errorf("starting the %s VM: %w", name, err)
	}
	if v.rec, err = processRecord(c.Process.Pid, keep); err != nil {
		// A VMM its pid file cannot name would never be found and reaped.
		v.stop()
		err = fmt.Errorf("starting the %s VM: recording its process: %w", name, err)
		if tail := consoleTail(console, 20); tail != "" {
			err = fmt.Errorf("%w; last console lines:\n%s", err, indent(tail))
		}
		return nil, err
	}
	return v, nil
}

// processRecord is a started VMM's pid-file entry, read from /proc. It fails
// rather than record an entry liveVMMs could never match.
func processRecord(pid int, keep bool) (vmmRecord, error) {
	r := vmmRecord{pid: pid, keep: keep}
	proc := "/proc/" + strconv.Itoa(pid)
	stat, err := os.ReadFile(proc + "/stat")
	if err != nil {
		return r, err
	}
	if r.start, err = procStartTime(string(stat)); err != nil {
		return r, fmt.Errorf("%s/stat: %w", proc, err)
	}
	if r.exe, err = os.Readlink(proc + "/exe"); err != nil {
		return r, err
	}
	if strings.ContainsAny(r.exe, "\n") {
		return r, fmt.Errorf("the firecracker path %q holds a newline", r.exe)
	}
	return r, nil
}

// stop kills the VMM and waits for it to exit.
func (v *vmm) stop() {
	select {
	case <-v.done:
		return
	default:
	}
	_ = v.cmd.Process.Kill()
	<-v.done
}

// exited reports whether the VMM has exited.
func (v *vmm) exited() bool {
	select {
	case <-v.done:
		return true
	default:
		return false
	}
}
