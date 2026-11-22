package main

import (
	"fmt"
	"io"
	"os"
	"runtime"
	"time"

	"github.com/spf13/cobra"

	"github.com/buildwarden/buildwarden/driver"
	"github.com/buildwarden/buildwarden/driver/firecracker"
	"github.com/buildwarden/buildwarden/driver/hyperv"
	"github.com/buildwarden/buildwarden/driver/qemu"
	"github.com/buildwarden/buildwarden/driver/vz"
)

var imageVerifyCmd = &cobra.Command{
	Use:   "verify",
	Args:  cobra.NoArgs,
	Short: "Re-hash cached base-image layers and report drift",
	Long: `Re-hash, in full, every base-image file a VM driver has recorded in its
layer cache (the hashes builds put in the ledger's environment record).

Builds reuse a cached hash while the file's size and modification time are
unchanged; verify reads every byte instead. A file whose contents changed is
re-recorded as a local file (its download/prepare provenance no longer
applies) and a missing file is dropped. Exits 1 if anything drifted.

With --driver, only that driver's cache is checked; otherwise every VM
driver's cache on this host.`,
	Example: `  warden image verify
  warden image verify --driver hyperv`,
	RunE: runImageVerify,
}

func init() {
	imageCmd.AddCommand(imageVerifyCmd)
}

func layerCaches(drv string) (map[string]*driver.LayerCache, error) {
	all := map[string]*driver.LayerCache{
		"qemu":        qemu.LayerCache(),
		"hyperv":      hyperv.LayerCache(),
		"firecracker": firecracker.LayerCache(),
	}
	if runtime.GOOS == "darwin" {
		all["vz"] = (&vz.ImageCache{CacheDir: vz.DefaultCacheDir()}).LayerCache()
	}
	if drv == "" {
		return all, nil
	}
	c, ok := all[drv]
	if !ok {
		return nil, driver.Fixable("warden image verify --driver "+
			"qemu|hyperv|vz|firecracker",
			"driver %q has no base-image layer cache on this host", drv)
	}
	return map[string]*driver.LayerCache{drv: c}, nil
}

func runImageVerify(_ *cobra.Command, _ []string) error {
	bins, err := verifyBinaries()
	if err != nil {
		return err
	}
	drift := 0
	for _, name := range append(append([]string{}, bundledDrivers...), flagDriver) {
		bin := bins[name]
		if bin == nil || !bin.desc.lists("image-verify") {
			continue
		}
		res, err := bin.call("image-verify", withConfig(map[string]any{
			"contract": bin.contract}, unparsedConfigFile()), callOpts{})
		if err != nil {
			return err
		}
		drift += resultInt(res, "drift")
	}
	// A driver binary that keeps no layer cache, the container driver's, gets
	// layerCaches' refusal.
	if bin := bins[flagDriver]; flagDriver != "" && bin != nil &&
		bin.desc.lists("image-verify") {
		return finishVerify(drift)
	}
	caches, err := layerCaches(flagDriver)
	if err != nil {
		return err
	}
	for _, name := range []string{"qemu", "vz", "hyperv", "firecracker"} {
		c, ok := caches[name]
		if !ok || bins[name] != nil {
			continue
		}
		n, err := verifyLayerCache(os.Stdout, name, c)
		if err != nil {
			return err
		}
		drift += n
	}
	return finishVerify(drift)
}

// finishVerify is image verify's exit: 1 when anything drifted.
func finishVerify(drift int) error {
	if drift > 0 {
		return fmt.Errorf("%d cached layer(s) changed or missing", drift)
	}
	return nil
}

// verifyBinaries are the driver binaries image verify asks: the one --driver
// names, or the bundled names the lookup resolves to a binary.
func verifyBinaries() (map[string]*execDriver, error) {
	if flagDriver == "" {
		return bundledBinaries()
	}
	fd, err := findDriver(selection{value: flagDriver, source: sourceFlag}, hostLookupEnv())
	if err != nil {
		return nil, err
	}
	if fd.Linked {
		return map[string]*execDriver{}, nil
	}
	bin, err := describeDriver(fd, time.After)
	if err != nil {
		return nil, err
	}
	return map[string]*execDriver{flagDriver: bin}, nil
}

// verifyLayerCache re-hashes one driver's cache, printing a line per layer,
// and returns how many drifted.
func verifyLayerCache(w io.Writer, name string, c *driver.LayerCache) (int, error) {
	if _, err := os.Stat(c.Path); err != nil {
		return 0, nil
	}
	results, err := c.Verify()
	if err != nil {
		return 0, fmt.Errorf("%s: %w", name, err)
	}
	drift := 0
	for _, r := range results {
		fmt.Fprintf(w, "%-7s %-8s %s\n", name, r.Status, r.Path)
		if r.Status != "ok" {
			drift++
		}
	}
	return drift, nil
}
