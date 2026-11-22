package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/buildwarden/buildwarden/driver"
	"github.com/buildwarden/buildwarden/driver/firecracker"
	"github.com/buildwarden/buildwarden/driver/hyperv"
	"github.com/buildwarden/buildwarden/driver/linuxguest"
	"github.com/buildwarden/buildwarden/driver/qemu"
	"github.com/buildwarden/buildwarden/driver/vz"
)

// `warden image` prepares and lists build-guest base images. One verb,
// `prepare`, covers every driver: it acquires the base (download, restore or
// install), verifies it, readies it for headless builds, and records it where
// `warden build --driver <d>` finds it. Every input a driver needs is a flag;
// nothing is prompted for. Re-running prepare is safe: cached work is reused.

var (
	flagImgOS     string
	flagImgSource string
)

// prepareOptions are the settings of a linked driver's image prepare, its
// flags and keys as the kit resolves them; a driver that does not declare
// one leaves it at its zero value.
type prepareOptions struct {
	force, update, keepVMs      bool
	sha256, name, edition       string
	virtio, arch                string
	windowsImage, windowsUpdate string
	// windowsUpdateSource is where windowsUpdate was set, for its error.
	windowsUpdateSource string
}

// prepareOptionsFrom reads image prepare's settings from set.
func prepareOptionsFrom(set driver.Settings) prepareOptions {
	var o prepareOptions
	o.force, _ = set.Bool("force")
	o.update, _ = set.Bool("update")
	o.keepVMs, _ = set.Bool("keep-vms")
	o.sha256, _ = set.String("sha256")
	o.name, _ = set.String("name")
	o.edition, _ = set.String("edition")
	o.virtio, _ = set.String("virtio")
	o.arch, _ = set.String("arch")
	o.windowsImage, _ = set.String("windows-image")
	o.windowsUpdate, _ = set.String("windows-image-update")
	o.windowsUpdateSource = set.Source("windows-image-update")
	return o
}

var imageCmd = &cobra.Command{
	Use:   "image",
	Short: "Prepare and list build-guest base images (VM drivers)",
}

var imagePrepareCmd = &cobra.Command{
	Use:   "prepare [source]",
	Args:  cobra.MaximumNArgs(1),
	Short: "Acquire, verify and ready a build-guest base image",
	Long: `Acquire, verify and ready a build-guest base image for a VM driver.

The driver comes from --driver (or warden.toml); --os defaults per driver
(vz: macos, qemu and firecracker: linux, hyperv: windows). [source] (or --source) is a local
path or URL:

  --driver vz     --os macos    macOS IPSW URL, a local .ipsw file (read
                                in place, kept), or a prepared image
                                directory. Run under sudo. No source
                                refreshes the active image (the one builds
                                use) in place, no download, and warns when
                                a newer macOS exists; with none prepared it
                                restores the latest supported IPSW. An IPSW
                                already prepared is refreshed in place, a
                                downloaded IPSW is deleted once prepared,
                                and the result becomes the active image.
                                --update moves to the latest supported IPSW.
  --driver qemu   --os linux    cloud image reference (alpine:3.21,
                                ubuntu:24.04, debian:bookworm) or a qcow2.
  --driver firecracker          cloud image reference (default ubuntu:24.04)
                --os linux    or a local qcow2 or raw image, downloaded if
                              needed and converted once to the raw image
                              builds copy. Images with a backing file or an
                              external data file are refused.
  --driver qemu   --os windows  Windows install ISO (required), installed
                                unattended (--edition, --virtio, --arch).
  --driver hyperv --os windows  Windows Server evaluation VHDX: a URL, or a
                                local file registered in place. Default:
                                [vm] windows_image, else Microsoft's current
                                Server 2025 evaluation download. Then
                                prepares it: one network-free boot through
                                OOBE and tuning, producing the layer builds
                                boot from. With no source it is the refresh:
                                re-checks a downloaded base for new media
                                ([vm] windows_image_update) and re-prepares.

--sha256 verifies the download against an expected digest (optional: the
digest of what was used is always recorded); --force re-acquires even when a
valid cached copy exists; --update (vz only) restores the newest supported OS
release, or refreshes it in place when already prepared, and makes it active.`,
	Example: `  sudo warden image prepare --driver vz
  sudo warden image prepare --driver vz --update
  sudo warden image prepare --driver vz ~/Downloads/UniversalMac_27.2_26B123_Restore.ipsw
  sudo warden image prepare --driver vz ~/.cache/warden/images/macos-<key>
  warden image prepare --driver qemu ubuntu:24.04
  warden image prepare --driver firecracker ubuntu:24.04
  warden image prepare --driver qemu --os windows --source Win11_Arm64.iso
  warden image prepare --driver hyperv --source https://.../server2025.vhd
  warden image prepare --driver hyperv --sha256 <hash-printed-on-first-prepare>
  warden image prepare --driver hyperv --source D:\images\server2025.vhdx
  warden image prepare --driver hyperv`,
	RunE: runImagePrepare,
}

var imageListCmd = &cobra.Command{
	Use:   "list",
	Short: "List prepared build-guest base images for every driver",
	Args:  cobra.NoArgs,
	RunE:  runImageList,
}

func init() {
	f := imagePrepareCmd.Flags()
	f.StringVar(&flagImgOS, "os", "", "guest OS: macos, linux or windows (default per driver)")
	f.StringVar(&flagImgSource, "source", "",
		"image source: local path or URL (same as [source])")

	imageCmd.AddCommand(imagePrepareCmd)
	imageCmd.AddCommand(imageListCmd)
	rootCmd.AddCommand(imageCmd)
}

// noImageFor is the refusal of an image prepare for a guest os the bundled
// driver drv has no image for, the container driver's every one.
func noImageFor(osName, drv string) error {
	return driver.Fixable("",
		"no %s image for --driver %q "+
			"(vz: macos; qemu: linux, windows; hyperv: windows; "+
			"firecracker: linux)",
		osName, drv)
}

func defaultImageOS(drv string) string {
	switch drv {
	case "vz":
		return "macos"
	case "hyperv":
		return "windows"
	default:
		return "linux"
	}
}

func runImagePrepare(cmd *cobra.Command, args []string) error {
	cfg, err := announcedConfig()
	if err != nil {
		return err
	}
	sd, err := selectDriver(cfg)
	if err != nil {
		return err
	}
	if sd.bin != nil {
		return prepareBinary(cfg, sd, args)
	}
	set, err := linkedSettings(cfg, sd, "image-prepare")
	if err != nil {
		return err
	}
	opts := prepareOptionsFrom(set)
	drv := sd.name()
	osName := flagImgOS
	if osName == "" {
		osName = defaultImageOS(drv)
	}
	source, err := imageSource(args)
	if err != nil {
		return err
	}

	var path string
	switch drv + "/" + osName {
	case "vz/macos":
		path, err = prepareVZMacos(source, opts.update)
	case "qemu/linux":
		path, err = linuxguest.PrepareImage(source)
	case "firecracker/linux":
		path, err = firecracker.PrepareImage(source)
	case "qemu/windows":
		path, err = prepareQemuWindows(source, opts)
	case "hyperv/windows":
		path, err = prepareHyperVWindows(cmd.Context(), source, opts)
	default:
		return noImageFor(osName, drv)
	}
	if err != nil {
		return err
	}
	// An empty path is the "already prepared" short-circuit: the helper has
	// printed its own message, so there is nothing more to report.
	if path != "" {
		fmt.Fprintf(os.Stderr, "Base image ready: %s\n", path)
	}
	return nil
}

// imageSource resolves the image source from --source and the positional [source]
// argument, rejecting a conflicting pair.
func imageSource(args []string) (string, error) {
	source := flagImgSource
	if len(args) > 0 {
		if source != "" && source != args[0] {
			return "", driver.Fixable("", "give the source once: [source] or --source")
		}
		source = args[0]
	}
	return source, nil
}

// prepareVZMacos restores or refreshes a macOS base image for the vz driver
// and makes it the active image (vz.ImageCache.Prepare).
func prepareVZMacos(source string, update bool) (string, error) {
	// Every vz prepare path writes root-owned files into the image, and
	// restoring needs the virtualization entitlement: check both before
	// anything is downloaded.
	if err := vz.Preflight(); err != nil {
		return "", err
	}
	if err := vz.RequireRoot(); err != nil {
		return "", err
	}
	cache := &vz.ImageCache{CacheDir: vz.DefaultCacheDir()}
	return cache.Prepare(source, update)
}

// prepareQemuWindows readies the qemu Windows image in its two stages: the
// install from ISO (ending in sysprep /generalize) when --source asks for it
// or there is no base yet, then the prepared layer builds boot from. With no
// source it only re-prepares the layer, which is what the periodic refresh
// runs.
func prepareQemuWindows(source string, o prepareOptions) (string, error) {
	arch := o.arch
	if arch == "" {
		arch = runtime.GOARCH
	}
	_, statErr := os.Stat(qemu.WindowsImagePath(arch))
	installed := statErr == nil
	switch {
	case source == "" && !installed:
		return "", driver.Fixable(
			"warden image prepare --driver qemu --os windows --source <path-or-url>",
			"--driver qemu --os windows needs the Windows install ISO")
	case source != "" && installed && !o.force:
		fmt.Fprintf(os.Stderr, "Windows base already installed: %s "+
			"(--force reinstalls it); preparing it\n", qemu.WindowsImagePath(arch))
	case source != "":
		opts := qemu.WindowsPrepOptions{
			ISO: source, VirtioISO: o.virtio, Edition: o.edition, Arch: arch,
		}
		media, err := qemu.AcquireWindowsMedia(opts)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(os.Stderr, "Running unattended Windows install (headless)...\n")
		if _, err := qemu.InstallWindowsImage(media, opts); err != nil {
			return "", err
		}
	}
	layer, err := qemu.PrepareWindowsLayer(arch, o.keepVMs, os.Stderr)
	if errors.Is(err, qemu.ErrPrepareRunning) {
		fmt.Fprintln(os.Stderr, "A qemu Windows image prepare is already running; "+
			"its result will become the active image when it finishes.")
		return "", nil
	}
	return layer, err
}

// prepareHyperVWindows readies the hyperv Windows base, then prepares the
// layer builds boot from. The base comes from --source, else [driver.hyperv]
// windows_image (or its old name), else (first time only) the registry's
// default URL. Without an explicit --source this is also the periodic
// refresh: a base that came from a URL is re-checked for new media unless
// windows_image_update is "reuse", a failed check only warns, and a base
// registered from a local path is never replaced unless the configuration
// names another.
func prepareHyperVWindows(ctx context.Context, source string, o prepareOptions) (string,
	error) {
	name := strings.TrimSpace(o.name)
	if name == "" {
		name = hyperv.DefaultEvalImage
	}
	switch o.windowsUpdate {
	case "", "auto", "reuse":
	default:
		return "", driver.Fixable(`set it to "auto" or "reuse"`,
			"%s = %q", o.windowsUpdateSource, o.windowsUpdate)
	}
	if src, fetch, tolerant := hypervImageSource(source, o, hyperv.Status()); fetch {
		_, err := hyperv.FetchEvalImage(ctx, hyperv.FetchOptions{
			Name: name, URL: src, SHA256: o.sha256, Force: o.force,
			Progress: os.Stderr,
		})
		if err != nil && !tolerant {
			return "", err
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: could not check the base image for new "+
				"media (%v); preparing the current base\n", err)
		}
	}
	pb, err := hyperv.PrepareBase(ctx, hyperv.PrepareOptions{
		KeepVM: o.keepVMs, Progress: os.Stderr,
	})
	if errors.Is(err, hyperv.ErrPrepareRunning) {
		fmt.Fprintln(os.Stderr, "A Hyper-V image prepare is already running; "+
			"its result will become the active base when it finishes.")
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return pb.Path, nil
}

// hypervImageSource decides whether image prepare fetches a base first, from
// where ("" = the registry default), and whether a failed fetch may be
// skipped because a usable base is already active.
func hypervImageSource(
	source string, o prepareOptions, st hyperv.ImageStatus,
) (src string, fetch, tolerant bool) {
	if source != "" || o.sha256 != "" || o.force || st.Base == "" {
		return firstNonEmptyStr(source, o.windowsImage), true, false
	}
	reuse := o.windowsUpdate == "reuse"
	if c := o.windowsImage; c != "" {
		if reuse && (c == st.Base || c == st.BaseURL) {
			return "", false, false
		}
		return c, true, true
	}
	if st.BaseURL != "" && !reuse {
		return st.BaseURL, true, true
	}
	return "", false, false
}

func firstNonEmptyStr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func runImageList(_ *cobra.Command, _ []string) error {
	bins, err := bundledBinaries()
	if err != nil {
		return err
	}
	found := false
	for _, name := range bundledDrivers {
		bin := bins[name]
		if bin == nil || !bin.desc.lists("image-list") {
			continue
		}
		res, err := bin.call("image-list", withConfig(map[string]any{
			"contract": bin.contract}, unparsedConfigFile()), callOpts{})
		if err != nil {
			return err
		}
		found = found || resultInt(res, "count") > 0
	}
	if listLinkedImages(bins) {
		found = true
	}
	if !found {
		fmt.Println("No prepared images. Run 'warden image prepare --driver <driver>'.")
	}
	return nil
}

// formatVZImages renders image list's vz lines: the active image first, then
// the others newest prepared first, each with the IPSW it was restored from
// when that was recorded.
func formatVZImages(imgs []vz.Image) []string {
	sorted := append([]vz.Image(nil), imgs...)
	sort.SliceStable(sorted, func(i, j int) bool {
		a, b := sorted[i], sorted[j]
		if a.Active != b.Active {
			return a.Active
		}
		if !a.Prepared.Equal(b.Prepared) {
			return a.Prepared.After(b.Prepared)
		}
		return a.Dir < b.Dir
	})
	lines := make([]string, 0, len(sorted))
	for _, img := range sorted {
		state := "older"
		if img.Active {
			state = "active"
		}
		line := fmt.Sprintf("vz      macos    %s  %-6s  %s", img.Dir, state, img.IPSW)
		lines = append(lines, strings.TrimRight(line, " "))
	}
	return lines
}

// prepareBinary is image prepare for a driver binary: --os defaults to the
// driver's default_image_os.
func prepareBinary(cfg *Config, sd *selectedDriver, args []string) error {
	bin := sd.bin
	if !bin.desc.lists("image-prepare") && contains(bundledDrivers, sd.name()) {
		osName := flagImgOS
		if osName == "" {
			osName = defaultImageOS(sd.name())
		}
		return noImageFor(osName, sd.name())
	}
	if !bin.desc.lists("image-prepare") {
		return driver.Fixable("", "driver %q prepares no images", sd.name())
	}
	osName := flagImgOS
	if osName == "" {
		osName = bin.desc.Capabilities.DefaultImageOS
	}
	source, err := imageSource(args)
	if err != nil {
		return err
	}
	req := withConfig(map[string]any{"contract": bin.contract, "os": osName,
		"source": source, "args": driverFlags.argsFor("image-prepare"),
		"verbose": cfg.Output.Verbose},
		absConfigFile(cfg))
	res, err := bin.call("image-prepare", req, callOpts{})
	if err != nil {
		return err
	}
	var path string
	_ = json.Unmarshal(res["path"], &path)
	if path != "" {
		fmt.Fprintf(os.Stderr, "Base image ready: %s\n", path)
	}
	return nil
}

// bundledBinaries are the bundled names the lookup resolves to a driver
// binary, each described, for the commands that select no driver (image list,
// and image verify with no --driver): a copy on PATH answers for its name
// there, as it would in a build. A name that is not found is left out.
func bundledBinaries() (map[string]*execDriver, error) {
	env := hostLookupEnv()
	out := map[string]*execDriver{}
	for _, name := range bundledDrivers {
		fd, err := findByName(name, env)
		var fe *driver.FixableError
		switch {
		case err != nil && fd != nil && errors.As(err, &fe):
			continue // not found
		case err != nil:
			return nil, err
		case fd.Linked:
			continue
		}
		bin, err := describeDriver(fd, time.After)
		if err != nil {
			return nil, err
		}
		out[name] = bin
	}
	return out, nil
}

// unparsedConfigFile is the config file of an invocation that loads no
// configuration, named without being parsed, or "" for none.
func unparsedConfigFile() string {
	path, _, err := configFile()
	if err != nil {
		return ""
	}
	return path
}

// listLinkedImages prints the images of the linked drivers that no driver
// binary answers for, and reports whether there were any.
func listLinkedImages(bins map[string]*execDriver) bool {
	found := false
	if bins["vz"] == nil && runtime.GOOS == "darwin" {
		cache := &vz.ImageCache{CacheDir: vz.DefaultCacheDir()}
		for _, line := range formatVZImages(cache.Images()) {
			fmt.Println(line)
			found = true
		}
	}
	qemuImages, _ := filepath.Glob(filepath.Join(driver.CacheDir(), "images", "*.qcow2"))
	if bins["qemu"] != nil {
		qemuImages = nil
	}
	for _, p := range qemuImages {
		osName := "linux"
		if strings.HasPrefix(filepath.Base(p), "windows-") {
			osName = "windows"
		}
		fmt.Printf("qemu    %-8s %s\n", osName, p)
		found = true
	}
	for _, p := range firecracker.ListImages() {
		if bins["firecracker"] != nil {
			break
		}
		fmt.Printf("firecracker linux %s\n", p)
		found = true
	}
	if st := hyperv.Status(); st.Base != "" && bins["hyperv"] == nil {
		pin := "pinned"
		if !st.Pinned {
			pin = "UNPINNED"
		}
		fmt.Printf("hyperv  windows  base     %s (%s)\n", st.Base, pin)
		fmt.Printf("hyperv  windows  prepared %s\n", st.Summary(time.Now()))
		for _, p := range st.Older {
			fmt.Printf("hyperv  windows  older    %s (deleted by the next prepare)\n",
				p.Path)
		}
		found = true
	}
	return found
}
