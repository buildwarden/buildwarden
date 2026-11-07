package vz

import (
	"crypto/sha512"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/buildwarden/buildwarden/driver"
	"github.com/buildwarden/buildwarden/driver/buildlink"
)

// plistHeader is the standard Apple plist XML header.
const plistHeader = "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n" +
	"<!DOCTYPE plist PUBLIC \"-//Apple//DTD PLIST 1.0//EN\"" +
	" \"http://www.apple.com/DTDs/PropertyList-1.0.dtd\">\n" +
	"<plist version=\"1.0\">\n"

// ImageCache manages cached VM base images.
type ImageCache struct {
	CacheDir string

	// The host steps preparing an image runs, nil meaning the real one, so
	// the prepare decisions are tier-0 testable on every OS (GLOBAL-07).
	latest      func() (string, error)          // latest supported IPSW URL
	download    func(url, dst string) error     // fetch an IPSW to dst
	restoreDisk func(ipsw, imgDir string) error // IPSW to a booted, headless disk
	refresh     func(imgDir string) error       // re-prepare in place
	requireRoot func() error
	log         io.Writer // progress and warnings; nil is stderr
}

// activeFile names, in the cache, the file holding the active image's
// directory name: the image every successful prepare ends on.
const activeFile = "active"

// Image is one prepared macOS image in the cache.
type Image struct {
	// Dir is the image directory (macos-<key>).
	Dir string
	// Prepared is when its .prepared marker was last written.
	Prepared time.Time
	// Active reports that builds without --image use it (ImageDir).
	Active bool
	// IPSW is the file name of the IPSW it was restored from, from its
	// recorded provenance; empty when none was recorded.
	IPSW string
}

// ImageDir returns the directory of the prepared macOS image builds use
// without --image: the active image when it is valid, else the valid image
// whose .prepared marker is newest (the name breaking ties), so a cache
// prepared before the active pointer existed still picks the image prepared
// last. Returns "" if no prepared image exists.
func (c *ImageCache) ImageDir() (string, error) {
	if b, err := os.ReadFile(filepath.Join(c.CacheDir, activeFile)); err == nil {
		if dir := c.imageNamed(strings.TrimSpace(string(b))); dir != "" {
			return dir, nil
		}
	}
	best, bestTime := "", time.Time{}
	for _, img := range c.validImages() {
		if best == "" || img.Prepared.After(bestTime) {
			best, bestTime = img.Dir, img.Prepared
		}
	}
	return best, nil
}

// imageNamed returns the valid image directory called name in the cache, or
// "" when name is not a plain directory name or not a valid image.
func (c *ImageCache) imageNamed(name string) string {
	if name == "" || name == "." || name == ".." || filepath.Base(name) != name {
		return ""
	}
	dir := filepath.Join(c.CacheDir, name)
	if !isValidImageDir(dir) {
		return ""
	}
	return dir
}

// validImages lists the cache's valid image directories in name order, with
// their .prepared times.
func (c *ImageCache) validImages() []Image {
	entries, _ := os.ReadDir(c.CacheDir)
	var imgs []Image
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(c.CacheDir, e.Name())
		if !isValidImageDir(dir) {
			continue
		}
		fi, err := os.Stat(filepath.Join(dir, ".prepared"))
		if err != nil {
			continue
		}
		imgs = append(imgs, Image{Dir: dir, Prepared: fi.ModTime()})
	}
	return imgs
}

// Images lists every valid prepared image in the cache, in name order, the
// one ImageDir returns marked active, each with its recorded IPSW file name.
func (c *ImageCache) Images() []Image {
	active, _ := c.ImageDir()
	imgs := c.validImages()
	for i := range imgs {
		imgs[i].Active = imgs[i].Dir == active
		imgs[i].IPSW = ipswFileName(c.recordedIPSW(imgs[i].Dir))
	}
	return imgs
}

// SetActive makes dir, an image directory in the cache, the one builds use
// without --image. The pointer is replaced atomically and handed back to the
// sudo user like the rest of the cache.
func (c *ImageCache) SetActive(dir string) error {
	if filepath.Clean(filepath.Dir(dir)) != filepath.Clean(c.CacheDir) {
		return fmt.Errorf("%s is not an image directory in %s", dir, c.CacheDir)
	}
	tmp, err := os.CreateTemp(c.CacheDir, activeFile+".*")
	if err != nil {
		return fmt.Errorf("writing the active image: %w", err)
	}
	_, werr := tmp.WriteString(filepath.Base(dir) + "\n")
	cerr := tmp.Close()
	if werr == nil {
		werr = cerr
	}
	if werr == nil {
		werr = os.Chmod(tmp.Name(), 0o644)
	}
	if werr == nil {
		werr = os.Rename(tmp.Name(), filepath.Join(c.CacheDir, activeFile))
	}
	if werr != nil {
		_ = os.Remove(tmp.Name())
		return fmt.Errorf("writing the active image: %w", werr)
	}
	giveBackToSudoUser(filepath.Join(c.CacheDir, activeFile))
	return nil
}

// recordedIPSW returns the URL of the IPSW dir's disk was recorded as
// prepared from (file://<path> for a local file), or "" when none was.
func (c *ImageCache) recordedIPSW(dir string) string {
	src, ok := c.LayerCache().Source(filepath.Join(dir, "disk.img"))
	if !ok {
		return ""
	}
	for _, in := range src.Inputs {
		if in.Name == "ipsw" {
			return in.URL
		}
	}
	return ""
}

// ipswFileName is the last path element of an IPSW URL, "" for none.
func ipswFileName(ipswURL string) string {
	if ipswURL == "" {
		return ""
	}
	if u, err := url.Parse(ipswURL); err == nil && u.Path != "" {
		return path.Base(u.Path)
	}
	return path.Base(ipswURL)
}

func isValidImageDir(dir string) bool {
	required := []string{
		"disk.img", "aux-storage", "hardware-model",
		"machine-id", ".prepared",
	}
	for _, f := range required {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			return false
		}
	}
	return true
}

// RestoreIPSW restores the IPSW at source (a URL, a local IPSW file, or ""
// for the latest supported one) to a bootable disk image and prepares it for headless use,
// returning the disk. An image already restored from source is returned as
// is. It writes no active pointer: only Prepare does.
func (c *ImageCache) RestoreIPSW(source string) (string, error) {
	if source == "" {
		u, err := c.latestIPSW()
		if err != nil {
			return "", fmt.Errorf("fetching latest IPSW URL: %w", err)
		}
		fmt.Fprintf(c.out(), "Latest macOS IPSW: %s\n", u)
		source = u
	}
	if !isIPSWURL(source) {
		abs, err := filepath.Abs(source)
		if err != nil {
			return "", err
		}
		source = abs
	}
	dir, err := c.restore(source)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "disk.img"), nil
}

// restore acquires the IPSW at source (a URL, or a local file's absolute
// path) for the image directory it keys to, macos-<shortHash(source)>, runs the restore step on it, and records the
// image's provenance, stamp and .prepared marker. Readying the image needs
// root, so that is checked before the download rather than after it.
func (c *ImageCache) restore(source string) (string, error) {
	if err := os.MkdirAll(c.CacheDir, 0755); err != nil {
		return "", fmt.Errorf("creating cache dir: %w", err)
	}
	key := shortHash(source)
	imgDir := filepath.Join(c.CacheDir, "macos-"+key)
	if err := os.MkdirAll(imgDir, 0755); err != nil {
		return "", fmt.Errorf("creating image dir: %w", err)
	}
	if isValidImageDir(imgDir) {
		return imgDir, nil
	}
	if err := c.checkRoot(); err != nil {
		return "", err
	}
	defer giveBackToSudoUser(filepath.Dir(c.CacheDir))

	// A local IPSW file is read where it is: no download, no copy into the
	// cache, and never deleted (it is the user's).
	ipswLocal := source
	if isIPSWURL(source) {
		var err error
		if ipswLocal, err = c.ensureIPSWDownloaded(source, key); err != nil {
			return "", err
		}
	}
	if err := c.restoreDiskStep(ipswLocal, imgDir); err != nil {
		return "", err
	}
	if err := c.recordPreparedImage(imgDir, ipswInput(source, ipswLocal)); err != nil {
		return "", fmt.Errorf("recording prepared image provenance: %w", err)
	}
	if err := writeLinkStamp(imgDir); err != nil {
		return "", fmt.Errorf("writing the link-channel stamp: %w", err)
	}
	_ = os.WriteFile(filepath.Join(imgDir, ".prepared"), []byte("ok\n"), 0644)

	fmt.Fprintf(c.out(), "macOS image ready at %s\n", imgDir)
	return imgDir, nil
}

// restoreDiskReal turns an IPSW into imgDir's disk and platform files (64 GB
// disk), installs the warden agent, and boots it once so macOS completes its
// initial setup. A disk that already has content (restore succeeded but a
// later step failed) is not restored again.
func (c *ImageCache) restoreDiskReal(ipswLocal, imgDir string) error {
	diskPath := filepath.Join(imgDir, "disk.img")
	diskInfo, _ := os.Stat(diskPath)
	if diskInfo == nil || diskInfo.Size() == 0 {
		fmt.Fprintf(c.out(), "Restoring macOS from IPSW (this takes 10-20 minutes)...\n")
		if err := restoreIPSW(ipswLocal, diskPath, 64,
			filepath.Join(imgDir, "aux-storage"), filepath.Join(imgDir, "hardware-model"),
			filepath.Join(imgDir, "machine-id")); err != nil {
			return fmt.Errorf("restoring IPSW: %w", err)
		}
	} else {
		fmt.Fprintf(c.out(), "Disk image exists, skipping restore...\n")
	}
	if err := PrepareHeadlessImage(diskPath); err != nil {
		return fmt.Errorf("headless setup: %w", err)
	}
	// Give hdiutil detach time to release the disk before the VM opens it.
	time.Sleep(2 * time.Second)
	fmt.Fprintf(c.out(), "Running first boot (macOS initial setup)...\n")
	if err := c.firstBoot(diskPath, imgDir); err != nil {
		return fmt.Errorf("first boot: %w", err)
	}
	return nil
}

// ensureIPSWDownloaded makes sure the IPSW identified by urlHash is present
// locally and returns its path. An already-cached file is reused as-is; a
// pre-existing ipsw-latest.ipsw (manual or previous download) is preferred
// over downloading again; otherwise it is fetched into a .part file renamed
// on success, so an interrupted download is never mistaken for a complete one.
func (c *ImageCache) ensureIPSWDownloaded(ipswURL, urlHash string) (string, error) {
	ipswLocal := filepath.Join(c.CacheDir, "ipsw-"+urlHash+".ipsw")
	if _, err := os.Stat(ipswLocal); err == nil {
		return ipswLocal, nil
	}
	latestPath := filepath.Join(c.CacheDir, "ipsw-latest.ipsw")
	if info, errLat := os.Stat(latestPath); errLat == nil && info.Size() > 0 {
		return latestPath, nil
	}
	fmt.Fprintf(c.out(), "Downloading IPSW...\n")
	part := ipswLocal + ".part"
	if err := c.downloadFile(ipswURL, part); err != nil {
		return "", fmt.Errorf("downloading IPSW (rerun to resume): %w", err)
	}
	if err := os.Rename(part, ipswLocal); err != nil {
		return "", err
	}
	return ipswLocal, nil
}

// isIPSWURL reports whether an IPSW source is a URL (it has a scheme,
// file:// included) rather than a local file path.
func isIPSWURL(source string) bool { return strings.Contains(source, "://") }

// curlDownload fetches url to dst with curl, resuming a partial dst.
func curlDownload(url, dst string) error {
	cmd := exec.Command("curl", "-fL", "-C", "-", "-o", dst, "--progress-bar", url)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (c *ImageCache) out() io.Writer {
	if c.log != nil {
		return c.log
	}
	return os.Stderr
}

func (c *ImageCache) latestIPSW() (string, error) {
	if c.latest != nil {
		return c.latest()
	}
	return latestSupportedIPSW()
}

func (c *ImageCache) downloadFile(url, dst string) error {
	if c.download != nil {
		return c.download(url, dst)
	}
	return curlDownload(url, dst)
}

func (c *ImageCache) restoreDiskStep(ipswLocal, imgDir string) error {
	if c.restoreDisk != nil {
		return c.restoreDisk(ipswLocal, imgDir)
	}
	return c.restoreDiskReal(ipswLocal, imgDir)
}

func (c *ImageCache) refreshStep(imgDir string) error {
	if c.refresh != nil {
		return c.refresh(imgDir)
	}
	return ReprepareImage(imgDir)
}

func (c *ImageCache) checkRoot() error {
	if c.requireRoot != nil {
		return c.requireRoot()
	}
	return RequireRoot()
}

// PrepareHeadlessImage installs the warden build agent into a macOS disk image.
// Writes a LaunchDaemon, boot script, and the warden-io binary directly onto
// the disk so no virtio-fs shared directory is needed at runtime.
func PrepareHeadlessImage(diskPath string) error {
	dm, err := mountDiskImage(diskPath)
	if err != nil {
		return fmt.Errorf("mounting disk: %w", err)
	}
	defer unmountDiskImage(dm)
	mountPoint := dm.DataVolumePath

	if err := installAgentLaunchd(mountPoint); err != nil {
		return fmt.Errorf("installing agent launchd: %w", err)
	}

	if err := installWardenIO(mountPoint); err != nil {
		return fmt.Errorf("installing warden-io: %w", err)
	}

	return nil
}

// installWardenIO cross-compiles warden-io for darwin/arm64 and installs it
// into the disk image at /usr/local/bin/warden-io.
func installWardenIO(mountPoint string) error {
	binDir := filepath.Join(mountPoint, "usr", "local", "bin")
	agentBin := filepath.Join(binDir, "warden-io")

	built, err := driver.GuestBinary("warden-io", "darwin", "arm64")
	if err != nil {
		return err
	}
	if err := driver.CopyFile(built, agentBin); err != nil {
		return err
	}
	_ = os.Chmod(agentBin, 0o755)
	os.Chown(agentBin, 0, 0) //nolint:errcheck
	return nil
}

// ReprepareImage updates the boot script and LaunchDaemon on an existing
// prepared image. Use this after upgrading warden to install the latest
// boot script without needing a full IPSW restore.
func ReprepareImage(imgDir string) error {
	diskPath := filepath.Join(imgDir, "disk.img")
	if _, err := os.Stat(diskPath); err != nil {
		return fmt.Errorf("disk image not found: %w", err)
	}
	if err := RequireRoot(); err != nil {
		return err
	}
	defer giveBackToSudoUser(imgDir)
	cache := &ImageCache{CacheDir: filepath.Dir(imgDir)}
	defer giveBackToSudoUser(cache.LayerCache().Path)

	if err := PrepareHeadlessImage(diskPath); err != nil {
		return err
	}
	if err := cache.recordPreparedImage(imgDir, nil); err != nil {
		return fmt.Errorf("recording prepared image provenance: %w", err)
	}
	if err := writeLinkStamp(imgDir); err != nil {
		return fmt.Errorf("writing the link-channel stamp: %w", err)
	}

	// Update the .prepared marker
	os.WriteFile(filepath.Join(imgDir, ".prepared"), //nolint:errcheck
		[]byte("ok\n"), 0644)

	return nil
}

// RequireRoot fails fast when readying a macOS image cannot work: it mounts
// the disk with ownership enabled and installs root-owned LaunchDaemons, so
// it needs root, and finding that out after a 25 GB download is too late.
func RequireRoot() error {
	if os.Geteuid() == 0 {
		return nil
	}
	return driver.Fixable(PrepareCommand,
		"preparing a macOS base image needs root (it installs "+
			"root-owned system files into the image)")
}

// giveBackToSudoUser hands what a sudo'd prepare created under dir back to
// the user who ran sudo, so their unprivileged builds can read and clone it
// (aux-storage is mode 0600). A no-op unless running as root via sudo.
func giveBackToSudoUser(dir string) {
	u := sudoUser()
	if u == nil {
		return
	}
	uid, err1 := strconv.Atoi(u.Uid)
	gid, err2 := strconv.Atoi(u.Gid)
	if err1 != nil || err2 != nil {
		return
	}
	_ = filepath.WalkDir(dir, func(p string, _ fs.DirEntry, err error) error {
		if err == nil {
			_ = os.Lchown(p, uid, gid)
		}
		return nil
	})
}

// suppressSetupAssistant writes the markers that tell macOS to skip the
// first-run experience entirely.
func suppressSetupAssistant(mountPoint string) error {
	varDB := filepath.Join(mountPoint, "private", "var", "db")
	if err := os.MkdirAll(varDB, 0755); err != nil {
		return err
	}

	// Primary marker — existence of this file suppresses Setup Assistant
	doneFile := filepath.Join(varDB, ".AppleSetupDone")
	if err := os.WriteFile(doneFile, []byte{}, 0644); err != nil {
		return fmt.Errorf("writing .AppleSetupDone: %w", err)
	}

	// Setup Assistant plist with skip list for all known screens.
	// This covers macOS 13-15. New screens in future versions may need
	// additions but the .AppleSetupDone marker alone handles most cases.
	skipItems := []string{
		"Accessibility",
		"AccessibilityOptions",
		"AppStore",
		"AppleID",
		"Appearance",
		"CloudStorage",
		"CrashReporterSoftware",
		"DataMigration",
		"Diagnostics",
		"FindMy",
		"HomeButtonSensitivity",
		"iCloudDocuments",
		"iCloudStorage",
		"Intro",
		"Location",
		"NanoPreview",
		"Privacy",
		"PrivacyBriefing",
		"RemoteManagement",
		"ScreenTime",
		"Selection",
		"Siri",
		"SiriDatabase",
		"TrueTone",
		"User",
		"WatchKit",
	}

	plist := buildSkipPlist(skipItems)
	plistPath := filepath.Join(varDB, "com.apple.SetupAssistant.plist")
	if err := os.WriteFile(plistPath, []byte(plist), 0644); err != nil {
		return fmt.Errorf("writing SetupAssistant plist: %w", err)
	}

	return nil
}

func buildSkipPlist(items []string) string {
	var sb strings.Builder
	sb.WriteString(plistHeader)
	sb.WriteString(`<dict>
	<key>DidSeeCloudSetup</key>
	<true/>
	<key>DidSeePrivacy</key>
	<true/>
	<key>DidSeeSiriSetup</key>
	<true/>
	<key>DidSeeAccessibility</key>
	<true/>
	<key>DidSeeAppearanceSetup</key>
	<true/>
	<key>DidSeeScreenTime</key>
	<true/>
	<key>DidSeeTrueTonePrivacy</key>
	<true/>
	<key>DidSeeiCloudLoginForStorageServices</key>
	<true/>
	<key>LastSeenBuddyBuildVersion</key>
	<string>99Z999</string>
	<key>LastSeenCloudProductVersion</key>
	<string>99.9</string>
	<key>SkipFirstLoginOptimization</key>
	<true/>
`)
	sb.WriteString("\t<key>SkipSetupItems</key>\n\t<array>\n")
	for _, item := range items {
		fmt.Fprintf(&sb, "\t\t<string>%s</string>\n", item)
	}
	sb.WriteString("\t</array>\n</dict>\n</plist>\n")
	return sb.String()
}

// createWardenUser pre-creates a local user account in the macOS disk image.
// Uses dslocal plist format (the same store Open Directory uses offline).
const wardenUser = "warden"
const wardenUID = "501"
const wardenGID = "20" // staff group

func createWardenUser(mountPoint string) error {
	usersDir := filepath.Join(mountPoint, "private", "var", "db",
		"dslocal", "nodes", "Default", "users")
	if err := os.MkdirAll(usersDir, 0755); err != nil {
		return err
	}

	// Generate password hash. macOS uses PBKDF2-SHA512 in ShadowHashData
	// but for VM-only ephemeral use, we use a simple salted SHA-512.
	// The VM is torn down after each build so password strength is irrelevant.
	passHash := hashPassword(wardenUser)

	userPlist := plistHeader + fmt.Sprintf(`<dict>
	<key>authentication_authority</key>
	<array>
		<string>;ShadowHash;HASHLIST:&lt;SALTED-SHA512-PBKDF2&gt;</string>
	</array>
	<key>generateduid</key>
	<array>
		<string>FFFFEEEE-DDDD-CCCC-BBBB-AAAA00000001</string>
	</array>
	<key>gid</key>
	<array>
		<string>%s</string>
	</array>
	<key>home</key>
	<array>
		<string>/Users/%s</string>
	</array>
	<key>name</key>
	<array>
		<string>%s</string>
	</array>
	<key>passwd</key>
	<array>
		<string>********</string>
	</array>
	<key>realname</key>
	<array>
		<string>Warden Build User</string>
	</array>
	<key>shell</key>
	<array>
		<string>/bin/zsh</string>
	</array>
	<key>uid</key>
	<array>
		<string>%s</string>
	</array>
	<key>ShadowHashData</key>
	<array>
		<data>%s</data>
	</array>
</dict>
</plist>
`, wardenGID, wardenUser, wardenUser, wardenUID, passHash)

	userPath := filepath.Join(usersDir, wardenUser+".plist")
	if err := os.WriteFile(userPath, []byte(userPlist), 0600); err != nil {
		return fmt.Errorf("writing user plist: %w", err)
	}

	// Create home directory
	homeDir := filepath.Join(mountPoint, "Users", wardenUser)
	if err := os.MkdirAll(homeDir, 0755); err != nil {
		return fmt.Errorf("creating home dir: %w", err)
	}

	return nil
}

// hashPassword generates a placeholder ShadowHashData value.
// For ephemeral VMs this doesn't need to be cryptographically correct —
// auto-login bypasses password verification entirely.
func hashPassword(password string) string {
	h := sha512.Sum512([]byte(password))
	return hex.EncodeToString(h[:])
}

// enableAutoLogin configures the system to log in as the warden user
// without prompting for a password.
func enableAutoLogin(mountPoint string) error {
	kcDir := filepath.Join(mountPoint, "private", "etc")
	if err := os.MkdirAll(kcDir, 0755); err != nil {
		return err
	}

	// /etc/kcpassword stores the auto-login password XOR-encoded.
	// The encoding is a simple repeating XOR with a fixed key.
	encoded := encodeKCPassword(wardenUser)
	kcPath := filepath.Join(kcDir, "kcpassword")
	if err := os.WriteFile(kcPath, encoded, 0600); err != nil {
		return fmt.Errorf("writing kcpassword: %w", err)
	}

	// loginwindow plist to enable auto-login
	lwPlist := plistHeader + fmt.Sprintf(`<dict>
	<key>autoLoginUser</key>
	<string>%s</string>
</dict>
</plist>
`, wardenUser)

	prefDir := filepath.Join(mountPoint, "Library", "Preferences")
	if err := os.MkdirAll(prefDir, 0755); err != nil {
		return err
	}
	lwPath := filepath.Join(prefDir, "com.apple.loginwindow.plist")
	return os.WriteFile(lwPath, []byte(lwPlist), 0644)
}

// encodeKCPassword XOR-encodes a password for /etc/kcpassword.
// macOS uses a fixed 11-byte repeating key.
var kcKey = []byte{0x7D, 0x89, 0x52, 0x23, 0xD2, 0xBC, 0xDD, 0xEA, 0xA3, 0xB9, 0x1F}

func encodeKCPassword(password string) []byte {
	pass := []byte(password)
	// Pad to multiple of 12 bytes
	padLen := 12 - (len(pass) % 12)
	if padLen < 12 {
		pass = append(pass, make([]byte, padLen)...)
	}

	encoded := make([]byte, len(pass))
	for i, b := range pass {
		encoded[i] = b ^ kcKey[i%len(kcKey)]
	}
	return encoded
}

// installAgentLaunchd writes a LaunchDaemon plist that watches the virtio-fs
// shared directory for the warden-io agent binary and executes it on arrival.
func installAgentLaunchd(mountPoint string) error {
	// Use the Apple system daemon path which loads during Installer Progress
	// (before user login / full boot). The regular /Library/LaunchDaemons/
	// only loads after the login window, which is too late.
	daemonDir := filepath.Join(mountPoint, "Library", "Apple", "System",
		"Library", "LaunchDaemons")
	if err := os.MkdirAll(daemonDir, 0755); err != nil {
		return err
	}

	// The agent watches for /Volumes/My Shared Files/agent/warden-io.
	// On macOS guests with Virtualization.framework, virtio-fs shares with
	// the automount tag appear at /Volumes/My Shared Files/<tag>.
	// We use the tag "shared" so the mount is /Volumes/My Shared Files/shared/
	plist := plistHeader + `<dict>
	<key>Label</key>
	<string>com.buildwarden.agent</string>
	<key>ProgramArguments</key>
	<array>
		<string>/usr/local/bin/warden-boot</string>
	</array>
	<key>RunAtLoad</key>
	<true/>
	<key>StandardOutPath</key>
	<string>/var/log/warden-agent.log</string>
	<key>StandardErrorPath</key>
	<string>/var/log/warden-agent.log</string>
</dict>
</plist>
`
	plistPath := filepath.Join(daemonDir, "com.buildwarden.agent.plist")
	if err := os.WriteFile(plistPath, []byte(plist), 0644); err != nil {
		return err
	}
	os.Chown(plistPath, 0, 0) //nolint:errcheck

	// Write the boot script that waits for virtio-fs mount then runs
	// warden-io initialize. This lives on the local disk so it's always
	// available at boot. warden-io initialize handles all subsequent setup:
	// network config, relay health wait, CA install, build script fetch+exec.
	binDir := filepath.Join(mountPoint, "usr", "local", "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		return err
	}
	bootScript := wardenBootScript
	bootScriptPath := filepath.Join(binDir, "warden-boot")
	if err := os.WriteFile(bootScriptPath, []byte(bootScript), 0755); err != nil {
		return err
	}
	os.Chown(bootScriptPath, 0, 0) //nolint:errcheck
	return nil
}

// wardenBootScript is the prepared image's launcher, run by launchd at boot.
// It carries no address: it takes the build link from the DHCP lease the host
// relay serves from the claim (the guest, mask, and the relay as router and
// DNS), so a change of [vm] build_link needs no re-prepare.
const wardenBootScript = `#!/bin/sh
# Warden boot: take the build link's lease, run warden-io initialize.
# Runs as LaunchDaemon (root, no user session needed).
# warden-io is baked into the image at /usr/local/bin/warden-io.
LOG="/var/log/warden-boot.log"
AGENT="/usr/local/bin/warden-io"

exec >>"$LOG" 2>&1
echo "$(date): warden-boot starting"

if [ ! -x "$AGENT" ]; then
    echo "$(date): FATAL agent not found at $AGENT"
    exit 1
fi

# The FileHandle virtio-net device is the only ethernet interface. Wait for it
# to become active (macOS takes a moment to bring the driver online), then
# take the relay's DHCP lease on it, all within 60 seconds.
echo "$(date): configuring network..."
IFACE=""
i=0
while [ -z "$IFACE" ] && [ $i -lt 30 ]; do
    for iface in $(ifconfig -l); do
        case "$iface" in lo0|anpi*|bridge*|awdl*|llw*|utun*|ap*) continue ;; esac
        hasether=$(ifconfig "$iface" 2>/dev/null | grep "ether")
        status=$(ifconfig "$iface" 2>/dev/null | grep "status:" | awk '{print $2}')
        if [ -n "$hasether" ] && [ "$status" = "active" ]; then
            IFACE=$iface
            break
        fi
    done
    if [ -z "$IFACE" ]; then
        sleep 2
        i=$((i + 1))
    fi
done

if [ -z "$IFACE" ]; then
    echo "$(date): FATAL no active ethernet interface after 60s"
    ifconfig -a 2>&1
    exit 1
fi

ipconfig set "$IFACE" DHCP
SELF=""
while [ -z "$SELF" ] && [ $i -lt 30 ]; do
    SELF=$(ipconfig getifaddr "$IFACE" 2>/dev/null)
    if [ -z "$SELF" ]; then
        sleep 2
        i=$((i + 1))
    fi
done
MASK=$(ipconfig getoption "$IFACE" subnet_mask 2>/dev/null)
GW=$(ipconfig getoption "$IFACE" router 2>/dev/null)
if [ -z "$SELF" ] || [ -z "$MASK" ] || [ -z "$GW" ]; then
    echo "$(date): FATAL no DHCP lease from the relay on $IFACE after 60s"
    ipconfig getpacket "$IFACE" 2>&1
    exit 1
fi

echo "$(date): $IFACE -> $SELF/$MASK gw $GW (dhcp)"
route add default "$GW" 2>/dev/null || true

# DNS (multiple mechanisms for macOS compatibility)
mkdir -p /etc/resolver
echo "nameserver $GW" > /etc/resolv.conf
echo "nameserver $GW" > /etc/resolver/default
scutil 2>/dev/null <<SCUTIL || true
d.init
d.add ServerAddresses * $GW
set State:/Network/Service/warden/DNS
d.init
d.add Addresses * $SELF
d.add SubnetMasks * $MASK
d.add Router $GW
d.add InterfaceName $IFACE
set State:/Network/Service/warden/IPv4
quit
SCUTIL

echo "$(date): launching warden-io initialize (gateway=$GW)"
exec "$AGENT" initialize --gateway="$GW" --ip="$SELF"
`

// linkStampName is the prepared image's link-channel stamp, in its directory.
const linkStampName = "link-channel"

// writeLinkStamp records that the image's warden-boot takes the build link
// per build (buildlink.Channel), after either prepare path installed it.
func writeLinkStamp(imgDir string) error {
	return os.WriteFile(filepath.Join(imgDir, linkStampName),
		[]byte(fmt.Sprintf("%s=%d\n", buildlink.StampKey, buildlink.Channel)), 0o644)
}

// diskMount holds the result of mounting a macOS disk image.
type diskMount struct {
	DataVolumePath string // e.g., "/Volumes/Data"
	BaseDevice     string // e.g., "/dev/disk4" — used for detach
}

// mountDiskImage mounts a macOS APFS disk image and returns the Data volume
// mount point. Uses `diskutil` to identify the Data role volume precisely.
func mountDiskImage(diskPath string) (*diskMount, error) {
	// Attach the disk image with ownership enabled so that files we write
	// retain root ownership (required for LaunchDaemons and system binaries).
	// This requires running as root (sudo).
	cmd := exec.Command("hdiutil", "attach", diskPath,
		"-nobrowse", "-noverify", "-noautoopen", "-owners", "on")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf(
			"hdiutil attach: %s: %w", string(out), err)
	}

	baseDevice, mountPoints := parseHdiutilOutput(string(out))

	dataPath := findDataVolume(mountPoints)
	if dataPath == "" {
		exec.Command("hdiutil", "detach", baseDevice, "-force").Run() //nolint:errcheck
		return nil, fmt.Errorf(
			"data volume not found in mounted disk image")
	}

	return &diskMount{
		DataVolumePath: dataPath,
		BaseDevice:     baseDevice,
	}, nil
}

// parseHdiutilOutput extracts the base device and mount points from
// hdiutil attach output.
func parseHdiutilOutput(output string) (string, []string) {
	var baseDevice string
	var mountPoints []string
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 &&
			strings.HasPrefix(fields[0], "/dev/disk") &&
			baseDevice == "" {
			baseDevice = fields[0]
		}
		tabIdx := strings.LastIndex(line, "\t")
		if tabIdx < 0 {
			continue
		}
		mp := strings.TrimSpace(line[tabIdx+1:])
		if mp != "" && mp[0] == '/' {
			mountPoints = append(mountPoints, mp)
		}
	}
	return baseDevice, mountPoints
}

// findDataVolume identifies the APFS Data volume from a list of mount
// points by checking `diskutil info` for "Volume Name: Data".
func findDataVolume(mountPoints []string) string {
	for _, mp := range mountPoints {
		infoOut, err := exec.Command("diskutil", "info", mp).Output()
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(infoOut), "\n") {
			trimmed := strings.TrimSpace(line)
			if !strings.HasPrefix(trimmed, "Volume Name:") {
				continue
			}
			suffix := strings.TrimPrefix(trimmed, "Volume Name:")
			if strings.TrimSpace(suffix) == "Data" {
				return mp
			}
		}
	}
	return ""
}

// unmountDiskImage detaches all volumes for a previously mounted disk image.
func unmountDiskImage(dm *diskMount) {
	exec.Command("hdiutil", "detach", dm.BaseDevice, "-force").Run() //nolint:errcheck
}

// CloneDisk creates an APFS copy-on-write clone of a base disk image.
func CloneDisk(src, dst string) error {
	cmd := exec.Command("cp", "-c", src, dst)
	if err := cmd.Run(); err != nil {
		cmd = exec.Command("cp", src, dst)
		return cmd.Run()
	}
	return nil
}

// firstBoot boots the macOS VM once so it completes initial setup.
// Places a probe agent on the shared volume and waits for it to signal.
// This validates the full chain: boot → auto-login → launchd → mount → agent.
func (c *ImageCache) firstBoot(diskPath, platformDir string) error {
	// First boot validates the boot chain: launchd → warden-boot → warden-io.
	// warden-io is baked into the image. The boot script takes the relay's
	// DHCP lease and runs "warden-io initialize". For validation we
	// just boot with NAT and verify the VM starts successfully. A full relay
	// test is done at e2e time rather than during image prep.
	vm, err := NewMacOSVM(macOSVMConfig{
		CPUs:               2,
		MemoryMB:           4096,
		DiskImagePath:      diskPath,
		AuxStoragePath:     filepath.Join(platformDir, "aux-storage"),
		HardwareModelPath:  filepath.Join(platformDir, "hardware-model"),
		MachineIDPath:      filepath.Join(platformDir, "machine-id"),
		FileHandleSocketFD: -1,
		AttachNAT:          true,
	})
	if err != nil {
		return fmt.Errorf("creating first-boot VM: %w", err)
	}
	if err := vm.Start(); err != nil {
		return fmt.Errorf("starting first-boot VM: %w", err)
	}
	defer func() { _ = vm.Stop() }()

	// Wait 90 seconds for macOS to complete initial setup.
	// On first boot after IPSW restore, macOS does one-time tasks
	// (APFS seal verification, preference migration, etc.).
	fmt.Fprintf(os.Stderr,
		"Waiting for macOS first boot (90s)...\n")
	time.Sleep(90 * time.Second)

	fmt.Fprintf(os.Stderr, "First boot complete.\n")
	return nil
}

func shortHash(s string) string {
	h := sha512.Sum512([]byte(s))
	return hex.EncodeToString(h[:8])
}
