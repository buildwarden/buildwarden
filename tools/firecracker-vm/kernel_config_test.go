// Package firecrackervm holds the Firecracker guest boot-asset scripts; its
// test checks the pinned kernel config without building a kernel.
package firecrackervm

import (
	"bufio"
	"os"
	"strings"
	"testing"
)

// TestKernelConfigHasRequiredOptions fails when kernel.config, the config
// build-kernel.sh resolved for the pinned kernel, lacks an option
// kernel-required.config builds in, or enables modules (Decision 10).
func TestKernelConfigHasRequiredOptions(t *testing.T) {
	have := readConfig(t, "kernel.config")
	required := readConfig(t, "kernel-required.config")
	if len(required) == 0 {
		t.Fatal("kernel-required.config lists no options")
	}
	for opt, want := range required {
		if got := have[opt]; got != want {
			t.Errorf("%s=%q in kernel.config, want %q", opt, got, want)
		}
	}
	if m := have["CONFIG_MODULES"]; m != "" {
		t.Errorf("CONFIG_MODULES=%s; the guest kernel loads no modules", m)
	}
}

func readConfig(t *testing.T, path string) map[string]string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	opts := map[string]string{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		k, v, ok := strings.Cut(strings.TrimSpace(sc.Text()), "=")
		if ok && strings.HasPrefix(k, "CONFIG_") {
			opts[k] = v
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	return opts
}
