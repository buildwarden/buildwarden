package main

import (
	"bufio"
	"crypto/rand"
	"fmt"
	"io"
	"math/big"
	"os"
	"path/filepath"
	"strings"

	"github.com/klauspost/compress/zstd"
	"github.com/lesiw/ctrctl"
)

// allocateSubnet finds an available /29 within the warden base /24.
// It tries each of the 32 possible /29 blocks and returns the first
// one that doesn't conflict with existing container networks.
func allocateSubnet() (buildSubnet, error) {
	for block := 0; block < 32; block++ {
		base := block * 8
		sub := buildSubnet{
			cidr:    fmt.Sprintf("%s.%d/29", wardenBaseNet, base),
			relayIP: fmt.Sprintf("%s.%d", wardenBaseNet, base+2),
			buildIP: fmt.Sprintf("%s.%d", wardenBaseNet, base+3),
		}
		_, err := ctrctl.NetworkCreate(
			&ctrctl.NetworkCreateOpts{
				Driver: "bridge",
				Subnet: sub.cidr,
			},
			"warden-probe",
		)
		if err != nil {
			continue
		}
		// Probe succeeded — remove it and return this subnet.
		_, _ = ctrctl.NetworkRm(nil, "warden-probe")
		return sub, nil
	}
	return buildSubnet{}, fmt.Errorf(
		"no available /29 subnet in %s.0/24", wardenBaseNet)
}

var wardenDir = ".warden"

var exts = []Extension{
	&ExtTrustStore{},
	&ExtBazel{},
	&ExtCACerts{},
	&ExtEpoch{},
}

// wardenBaseNet is a /24 in the CGNAT range (100.64.0.0/10) which is
// reserved for carrier-grade NAT and unlikely to collide with user networks.
// Each build allocates a /29 within this range.
const wardenBaseNet = "100.64.87"

type buildSubnet struct {
	cidr    string
	relayIP string
	buildIP string
}

type CtrEnv struct {
	buildConfig *BuildConfig
	buildId     string
	ledgerDir   string
	subnet      buildSubnet
}


const relayImageRepo = "ghcr.io/buildwarden/relay"


func (d *CtrEnv) moveDir(src, dst string) {
	entries, err := os.ReadDir(src)
	if err != nil {
		return
	}
	for _, entry := range entries {
		s := filepath.Join(src, entry.Name())
		if d.buildConfig.Compress {
			d.compressFile(s, filepath.Join(dst, entry.Name()+".zst"))
		} else {
			os.Rename(s, filepath.Join(dst, entry.Name())) //nolint:errcheck
		}
	}
}

// compressedMagic identifies already-compressed formats by their first bytes.
var compressedMagic = [][]byte{
	{0x1f, 0x8b},             // gzip
	{0x28, 0xb5, 0x2f, 0xfd}, // zstd
	{0xfd, 0x37, 0x7a, 0x58}, // xz
	{0x50, 0x4b, 0x03, 0x04}, // zip/jar/whl
	{0x42, 0x5a, 0x68},       // bzip2
}

func isCompressedFile(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	buf := make([]byte, 4)
	n, _ := f.Read(buf)
	for _, magic := range compressedMagic {
		if n >= len(magic) {
			match := true
			for i, b := range magic {
				if buf[i] != b {
					match = false
					break
				}
			}
			if match {
				return true
			}
		}
	}
	return false
}

func (d *CtrEnv) compressFile(src, dst string) {
	if isCompressedFile(src) {
		uncompDst := strings.TrimSuffix(dst, ".zst")
		os.Rename(src, uncompDst) //nolint:errcheck
		return
	}

	in, err := os.Open(src)
	if err != nil {
		return
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return
	}

	enc, err := zstd.NewWriter(out)
	if err != nil {
		out.Close()
		os.Remove(dst) //nolint:errcheck
		return
	}
	io.Copy(enc, in) //nolint:errcheck
	enc.Close()
	out.Close()
}


func (d *CtrEnv) editContainerfile() error {
	// Create an edited copy in .warden/Containerfile — never modify the original.
	origfile, err := os.Open(d.buildConfig.Containerfile)
	if err != nil {
		return fmt.Errorf("error opening containerfile: %w", err)
	}
	defer origfile.Close()

	ctrfilePath := filepath.Join(d.wardenDirPath(), "Containerfile")
	ctrfile, err := os.Create(ctrfilePath)
	if err != nil {
		return fmt.Errorf("error creating edited containerfile: %w", err)
	}
	defer ctrfile.Close()

	env := make(map[string]string)
	for _, ext := range exts {
		extenv := ext.Env()
		if extenv == nil {
			continue
		}
		for k, v := range extenv {
			if _, ok := env[k]; ok {
				return fmt.Errorf("env collision: %s", k)
			}
			env[k] = v
		}
	}
	var enventries []string
	for k, v := range env {
		enventries = append(enventries, fmt.Sprintf("%s=%s", k, v))
	}

	scanner := bufio.NewScanner(origfile)
	for scanner.Scan() {
		line := scanner.Text()

		if strings.HasPrefix(line, "FROM ") {
			_, _ = ctrfile.WriteString(line + "\n")
			if len(enventries) > 0 {
				_, _ = ctrfile.WriteString("ENV " +
					strings.Join(enventries, " \\\n") + "\n")
			}
			_, _ = ctrfile.WriteString("COPY .warden /.warden\n")
			_, _ = ctrfile.WriteString(
				"RUN ln -sf /.warden/warden-io /usr/local/bin/warden-io" +
					" && find /.warden/ext.d/ -exec sh {} \\;\n")
			continue
		}

		if isCopyDirective(line) {
			rewritten, err := d.rewriteCopy(line)
			if err != nil {
				return fmt.Errorf("error rewriting COPY: %w", err)
			}
			_, _ = ctrfile.WriteString(rewritten + "\n")
			continue
		}

		_, _ = ctrfile.WriteString(line + "\n")
	}

	return nil
}

func isCopyDirective(line string) bool {
	trimmed := strings.TrimSpace(line)
	return strings.HasPrefix(trimmed, "COPY ") ||
		strings.HasPrefix(trimmed, "COPY\t")
}

func (d *CtrEnv) rewriteCopy(line string) (string, error) {
	trimmed := strings.TrimSpace(line)
	args := trimmed[5:] // strip "COPY "

	if strings.Contains(args, "--from=") {
		return line, nil
	}

	chown, chmod, positional := parseCopyFlags(args)
	if len(positional) < 2 {
		return line, nil
	}

	dest := positional[len(positional)-1]
	sources := positional[:len(positional)-1]

	var files []string
	for _, src := range sources {
		expanded, err := d.expandContextPath(src)
		if err != nil {
			return "", err
		}
		files = append(files, expanded...)
	}

	if len(files) == 0 {
		return "# (empty COPY: no matching files)", nil
	}

	run := buildFetchRun(files, dest)
	if chown != "" {
		run += fmt.Sprintf(" && \\\n    chown -R %s %s", chown, dest)
	}
	if chmod != "" {
		run += fmt.Sprintf(" && \\\n    chmod -R %s %s", chmod, dest)
	}
	return run, nil
}

func parseCopyFlags(args string) (chown, chmod string, positional []string) {
	for _, p := range strings.Fields(args) {
		switch {
		case strings.HasPrefix(p, "--chown="):
			chown = strings.TrimPrefix(p, "--chown=")
		case strings.HasPrefix(p, "--chmod="):
			chmod = strings.TrimPrefix(p, "--chmod=")
		default:
			positional = append(positional, p)
		}
	}
	return
}

func buildFetchRun(files []string, dest string) string {
	if len(files) == 1 && !strings.HasSuffix(dest, "/") {
		dir := dest[:strings.LastIndex(dest, "/")+1]
		if dir != "" {
			return fmt.Sprintf(
				"RUN mkdir -p %s && warden-io fetch %s -o %s",
				dir, files[0], dest)
		}
		return fmt.Sprintf("RUN warden-io fetch %s -o %s", files[0], dest)
	}

	d := dest
	if !strings.HasSuffix(d, "/") {
		d += "/"
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "RUN mkdir -p %s && \\\n    printf '%%s\\n'", dest)
	for _, f := range files {
		fmt.Fprintf(&sb, " '%s'", f)
	}
	fmt.Fprintf(&sb,
		" | \\\n    xargs -P8 -I{} sh -c "+
			"'mkdir -p \"%s$(dirname \"{}\")\" && "+
			"warden-io fetch \"{}\" -o \"%s{}\"'",
		d, d)
	return sb.String()
}

// expandContextPath resolves a source path or glob against the context.
func (d *CtrEnv) expandContextPath(src string) ([]string, error) {
	ctxDir := d.buildConfig.Context
	fullPattern := filepath.Join(ctxDir, src)

	info, err := os.Stat(fullPattern)
	if err == nil && info.IsDir() {
		// Directory: walk and collect all files.
		var files []string
		err = filepath.Walk(fullPattern, func(
			path string, fi os.FileInfo, walkErr error,
		) error {
			if walkErr != nil {
				return walkErr
			}
			if fi.IsDir() {
				switch fi.Name() {
				case wardenDir, ".git":
					return filepath.SkipDir
				}
				return nil
			}
			rel, _ := filepath.Rel(ctxDir, path)
			files = append(files, rel)
			return nil
		})
		return files, err
	}

	// Try as glob pattern.
	matches, err := filepath.Glob(fullPattern)
	if err != nil {
		return nil, err
	}
	var files []string
	for _, m := range matches {
		info, err := os.Stat(m)
		if err != nil || info.IsDir() {
			continue
		}
		rel, _ := filepath.Rel(ctxDir, m)
		if strings.HasPrefix(rel, wardenDir+"/") ||
			strings.HasPrefix(rel, ".git/") {
			continue
		}
		files = append(files, rel)
	}
	return files, nil
}

func (d *CtrEnv) wardenDirPath() string {
	return filepath.Join(d.buildConfig.Context, wardenDir)
}

func (d *CtrEnv) wardenScriptPath() string {
	return filepath.Join(d.wardenDirPath(), "ext.d")
}

var anumRunes = []rune("abcdefghijklmnopqrstuvwxyz0123456789")

// randanum produces a cryptographically-random alphanumeric string.
func randanum(n int) string {
	b := make([]rune, n)
	for i := range b {
		j, err := rand.Int(rand.Reader, big.NewInt(int64(len(anumRunes))))
		if err != nil {
			panic(err)
		}
		b[i] = anumRunes[j.Uint64()]
	}
	return string(b)
}
