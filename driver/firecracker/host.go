package firecracker

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"slices"
	"strconv"
	"strings"
)

// realHost is host on the machine warden runs on.
type realHost struct{}

func (realHost) run(args []string) error {
	out, err := exec.Command(args[0], args[1:]...).CombinedOutput()
	if err != nil {
		if msg := strings.TrimSpace(string(out)); msg != "" {
			return fmt.Errorf("%w: %s", err, msg)
		}
		return err
	}
	return nil
}

func (realHost) output(args []string) (string, error) {
	var stderr bytes.Buffer
	c := exec.Command(args[0], args[1:]...)
	c.Stderr = &stderr
	out, err := c.Output()
	if err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return string(out), fmt.Errorf("%w: %s", err, msg)
		}
	}
	return string(out), err
}

func (realHost) readFile(path string) ([]byte, error) { return os.ReadFile(path) }

func (realHost) readlink(path string) (string, error) { return os.Readlink(path) }

func (realHost) exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func (realHost) lookPath(name string) error {
	_, err := exec.LookPath(name)
	return err
}

func (realHost) getenv(key string) string { return os.Getenv(key) }

func (realHost) euid() int { return os.Geteuid() }

func (realHost) openRW(path string) error {
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	return f.Close()
}

func (realHost) reflink(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	src, err := os.CreateTemp(dir, "reflink-probe-")
	if err != nil {
		return err
	}
	src.Close()
	defer os.Remove(src.Name())
	dst := src.Name() + ".copy"
	defer os.Remove(dst)
	out, err := exec.Command("cp", "--reflink=always", src.Name(), dst).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s", strings.TrimSpace(string(out)))
	}
	return nil
}

func (realHost) lookupOwner(name string) (int, bool, error) {
	u, err := user.Lookup(name)
	if err != nil {
		return 0, false, fmt.Errorf("looking up %s: %w", name, err)
	}
	uid, err := strconv.Atoi(u.Uid)
	if err != nil {
		return 0, false, fmt.Errorf("uid of %s: %w", name, err)
	}
	g, err := user.LookupGroup("kvm")
	if err != nil {
		return uid, false, nil
	}
	ids, err := u.GroupIds()
	return uid, err == nil && slices.Contains(ids, g.Gid), nil
}
