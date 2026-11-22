package main

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/buildwarden/buildwarden/driver"
)

func outputDirFile(dir string) string { return "[build]\noutput_dir = \"" + dir + "\"\n" }

// notTOML is a config file body no decoder takes. A test that loads with it
// in a file passes only when that file is never decoded.
const notTOML = "this is = = not TOML\n"

// selfLink makes path a symbolic link to itself, a file that no user, root
// included, can examine or open: the operating system refuses it with "too
// many levels of symbolic links".
func selfLink(t *testing.T, path string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("needs a symbolic link, which Windows restricts")
	}
	if err := os.Symlink(filepath.Base(path), path); err != nil {
		t.Fatal(err)
	}
}

// unreadable takes every permission from path. It skips the test where that
// denies nothing: for root, and on Windows.
func unreadable(t *testing.T, path string) {
	t.Helper()
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("mode 0 denies nothing to root or on Windows")
	}
	if err := os.Chmod(path, 0); err != nil {
		t.Fatal(err)
	}
}

// giveConfigEnv names the config file with WARDEN_CONFIG.
func giveConfigEnv(t *testing.T, path string) { t.Setenv("WARDEN_CONFIG", path) }

// modeZero writes a config file at path that its owner cannot read.
func modeZero(t *testing.T, path string) {
	t.Helper()
	write(t, filepath.Dir(path), filepath.Base(path), outputDirFile("x"))
	unreadable(t, path)
}

// wantFixable checks that err is a FixableError, which main exits 1 on
// (UX-09), with exactly this message and fix.
func wantFixable(t *testing.T, err error, msg, fix string) {
	t.Helper()
	var fe *driver.FixableError
	if !errors.As(err, &fe) {
		t.Fatalf("got %v, want a FixableError saying %q", err, msg)
	}
	var es exitStatusError
	if errors.As(err, &es) {
		t.Errorf("the error carries exit status %d, want main's exit 1", int(es))
	}
	if fe.Msg != msg || fe.Fix != fix {
		t.Errorf("got  %q\n     %q\nwant %q\n     %q", fe.Msg, fe.Fix, msg, fix)
	}
}

// The one config file is the one --config names, else the one WARDEN_CONFIG
// names, else ./warden.toml, else none. Each level is asked for a different
// output directory, so the one in effect shows which file was decoded.
func TestConfigFileLookupOrder(t *testing.T) {
	for _, tc := range []struct {
		name           string
		flag, env, dir bool // which levels name a file
		empty          bool // WARDEN_CONFIG is set and empty
		wantOutput     string
		wantSource     string
	}{
		{name: "the flag over the variable and ./warden.toml",
			flag: true, env: true, dir: true,
			wantOutput: "from-flag", wantSource: "--config"},
		{name: "the variable over ./warden.toml", env: true, dir: true,
			wantOutput: "from-env", wantSource: "WARDEN_CONFIG"},
		{name: "./warden.toml when nothing names a file", dir: true,
			wantOutput: "from-dir", wantSource: "found in the working directory"},
		{name: "an empty variable is not set", dir: true, empty: true,
			wantOutput: "from-dir", wantSource: "found in the working directory"},
		{name: "no file at all", wantOutput: ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := ""
			if tc.dir {
				body = outputDirFile("from-dir")
			}
			wd := inConfigFiles(t, body)
			elsewhere := t.TempDir()
			want := ""
			if tc.dir {
				want = filepath.Join(wd, "warden.toml")
			}
			if tc.env {
				want = write(t, elsewhere, "env.toml", outputDirFile("from-env"))
				t.Setenv("WARDEN_CONFIG", want)
			}
			if tc.empty {
				t.Setenv("WARDEN_CONFIG", "")
			}
			if tc.flag {
				want = write(t, elsewhere, "flag.toml", outputDirFile("from-flag"))
				giveConfigFlag(t, want)
			}
			cfg, err := LoadConfig()
			if err != nil {
				t.Fatalf("LoadConfig: %v", err)
			}
			if cfg.Build.OutputDir != tc.wantOutput {
				t.Errorf("output_dir = %q, want %q: another file was decoded",
					cfg.Build.OutputDir, tc.wantOutput)
			}
			if cfg.File != want || cfg.FileSource != tc.wantSource {
				t.Errorf("config file = %q (%q), want %q (%q)", cfg.File,
					cfg.FileSource, want, tc.wantSource)
			}
		})
	}
}

// ./warden.toml is named by the working directory as the user entered it,
// through a symbolic link as a shell's PWD gives it, not as the kernel
// resolves it: macOS's $TMPDIR is under /var, a link to /private/var.
func TestConfigLineNamesTheDirectoryAsEntered(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("needs a symbolic link, which Windows restricts")
	}
	inConfigFiles(t, "")
	realDir := t.TempDir()
	write(t, realDir, projectConfig, outputDirFile("from-dir"))
	link := filepath.Join(t.TempDir(), "linked")
	if err := os.Symlink(realDir, link); err != nil {
		t.Fatal(err)
	}
	t.Chdir(link)
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	want := filepath.Join(link, projectConfig)
	if cfg.File != want || cfg.FileSource != configFromDir {
		t.Errorf("config file = %q (%q), want %q (%q)", cfg.File, cfg.FileSource,
			want, configFromDir)
	}
	if cfg.Build.OutputDir != "from-dir" {
		t.Errorf("output_dir = %q, want %q: another file was decoded",
			cfg.Build.OutputDir, "from-dir")
	}
	wantLine := "config: " + want + " (" + configFromDir + ")"
	if line := configLine(cfg); line != wantLine {
		t.Errorf("configLine = %q, want %q", line, wantLine)
	}
}

// A file that is named is the only one: the sources below it are not
// opened, so a file there that no decoder takes is not an error.
func TestNamedConfigFileIsTheOnlyOneRead(t *testing.T) {
	t.Run("--config, with ./warden.toml and the variable's file not TOML", func(t *testing.T) {
		inConfigFiles(t, notTOML)
		t.Setenv("WARDEN_CONFIG", write(t, t.TempDir(), "env.toml", notTOML))
		giveConfigFlag(t, write(t, t.TempDir(), "ci.toml", outputDirFile("named")))
		cfg, err := LoadConfig()
		if err != nil || cfg.Build.OutputDir != "named" {
			t.Fatalf("LoadConfig = %+v, %v; want the named file alone", cfg, err)
		}
	})
	t.Run("WARDEN_CONFIG, with ./warden.toml not TOML", func(t *testing.T) {
		inConfigFiles(t, notTOML)
		t.Setenv("WARDEN_CONFIG", write(t, t.TempDir(), "ci.toml", outputDirFile("named")))
		cfg, err := LoadConfig()
		if err != nil || cfg.Build.OutputDir != "named" {
			t.Fatalf("LoadConfig = %+v, %v; want the named file alone", cfg, err)
		}
	})
	t.Run("--config, with the variable naming a file that does not exist", func(t *testing.T) {
		inConfigFiles(t, "")
		t.Setenv("WARDEN_CONFIG", filepath.Join(t.TempDir(), "missing.toml"))
		giveConfigFlag(t, write(t, t.TempDir(), "ci.toml", outputDirFile("named")))
		cfg, err := LoadConfig()
		if err != nil || cfg.Build.OutputDir != "named" {
			t.Fatalf("LoadConfig = %+v, %v; want the named file alone", cfg, err)
		}
	})
}

// A named file that cannot be read is an error that names it and its source,
// and nothing else is tried: the ./warden.toml beside it, which sets an
// output directory, is not used (GLOBAL-10).
func TestNamedConfigFileThatCannotBeRead(t *testing.T) {
	const (
		flagFix = "pass --config an existing file, or drop it to use " +
			"WARDEN_CONFIG or ./warden.toml"
		envFix = "set WARDEN_CONFIG to an existing file, or unset it to use " +
			"./warden.toml"
		readFix = "make it readable, or name another file with --config or " +
			"WARDEN_CONFIG"
	)
	type source struct {
		name, from, fix string
		give            func(t *testing.T, path string)
	}
	sources := []source{
		{"--config", "from --config", flagFix, giveConfigFlag},
		{"WARDEN_CONFIG", "from WARDEN_CONFIG", envFix, giveConfigEnv},
	}
	for _, src := range sources {
		for _, tc := range []struct {
			name string
			make func(t *testing.T, path string)
			msg  string // after the path and its source
			fix  string
		}{
			{name: "does not exist", make: func(*testing.T, string) {},
				msg: "does not exist", fix: src.fix},
			{name: "is a directory",
				make: func(t *testing.T, path string) {
					if err := os.Mkdir(path, 0o755); err != nil {
						t.Fatal(err)
					}
				},
				msg: "is a directory", fix: src.fix},
			{name: "cannot be examined (a link to itself)", make: selfLink,
				msg: "too many levels of symbolic links", fix: readFix},
			{name: "cannot be opened (mode 0)",
				make: modeZero, msg: "permission denied", fix: readFix},
		} {
			t.Run(src.name+" "+tc.name, func(t *testing.T) {
				inConfigFiles(t, outputDirFile("from-dir"))
				path := filepath.Join(t.TempDir(), "ci.toml")
				tc.make(t, path)
				src.give(t, path)
				cfg, err := LoadConfig()
				if cfg != nil {
					t.Errorf("LoadConfig returned a configuration with its "+
						"error (output_dir %q)", cfg.Build.OutputDir)
				}
				named := path + " (" + src.from + ")"
				want := "config file " + named + " " + tc.msg
				if tc.fix == readFix {
					want = "cannot read config file " + named + ": " + tc.msg
				}
				wantFixable(t, err, want, tc.fix)
			})
		}
	}
}

// --config given with an empty value is an error, not "not set": the next
// source down is ./warden.toml, which can be the file the caller meant not
// to use (a8).
func TestEmptyConfigFlagIsAnError(t *testing.T) {
	inConfigFiles(t, outputDirFile("from-dir"))
	t.Setenv("WARDEN_CONFIG", write(t, t.TempDir(), "env.toml", outputDirFile("from-env")))
	giveConfigFlag(t, "")
	cfg, err := LoadConfig()
	if cfg != nil {
		t.Errorf("LoadConfig returned a configuration (output_dir %q) with its error",
			cfg.Build.OutputDir)
	}
	wantFixable(t, err, "--config was given an empty path",
		"pass --config the path of a config file, or drop the flag to use "+
			"WARDEN_CONFIG or ./warden.toml")
}

// A ./warden.toml that exists and cannot be read is an error, not "no file"
// (GLOBAL-11).
func TestWorkingDirectoryConfigThatCannotBeRead(t *testing.T) {
	const fix = "make it readable, or name another file with --config or WARDEN_CONFIG"
	for name, tc := range map[string]struct {
		make   func(t *testing.T, path string)
		reason string
	}{
		"a link to itself": {selfLink, "too many levels of symbolic links"},
		"mode 0":           {modeZero, "permission denied"},
		"a directory": {func(t *testing.T, path string) {
			if err := os.Mkdir(path, 0o755); err != nil {
				t.Fatal(err)
			}
		}, "is a directory"},
		"a link to a device": {linkToDevice, "not a regular file"},
	} {
		t.Run(name, func(t *testing.T) {
			inConfigFiles(t, "")
			path := configPath(t)
			tc.make(t, path)
			cfg, err := LoadConfig()
			if cfg != nil {
				t.Error("LoadConfig returned the defaults with its error")
			}
			wantFixable(t, err, "cannot read config file "+path+
				" (found in the working directory): "+tc.reason, fix)
		})
	}
}

// linkToDevice makes path a symbolic link to /dev/null, a device that a
// repository's warden.toml can point at as well as /dev/zero or a terminal.
func linkToDevice(t *testing.T, path string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("needs /dev/null and a symbolic link")
	}
	if err := os.Symlink(os.DevNull, path); err != nil {
		t.Fatal(err)
	}
}

// A file named with --config or WARDEN_CONFIG is read whatever kind of file
// it is: only the working directory's, which nobody named, must be a regular
// file.
func TestNamedConfigFileThatIsADevice(t *testing.T) {
	for name, give := range map[string]func(*testing.T, string){
		"--config": giveConfigFlag, "WARDEN_CONFIG": giveConfigEnv,
	} {
		t.Run(name, func(t *testing.T) {
			inConfigFiles(t, "")
			path := filepath.Join(t.TempDir(), "warden.toml")
			linkToDevice(t, path)
			give(t, path)
			cfg, err := LoadConfig()
			if err != nil {
				t.Fatalf("LoadConfig: %v", err)
			}
			if cfg.File != path {
				t.Errorf("read %q, want %s", cfg.File, path)
			}
		})
	}
}

// To read no settings at all, name an empty file.
func TestEmptyConfigFileGivesTheDefaults(t *testing.T) {
	empty := map[string]func(t *testing.T) string{
		"an empty file": func(t *testing.T) string {
			return write(t, t.TempDir(), "empty.toml", "")
		},
	}
	if runtime.GOOS != "windows" {
		empty["/dev/null"] = func(*testing.T) string { return os.DevNull }
	}
	for name, path := range empty {
		t.Run(name, func(t *testing.T) {
			inConfigFiles(t, outputDirFile("from-dir"))
			named := path(t)
			giveConfigFlag(t, named)
			cfg, err := LoadConfig()
			if err != nil {
				t.Fatalf("LoadConfig: %v", err)
			}
			if cfg.Build.OutputDir != "" || cfg.File != named {
				t.Errorf("output_dir = %q from %q, want the default from %q",
					cfg.Build.OutputDir, cfg.File, named)
			}
		})
	}
}

// A relative --config or WARDEN_CONFIG is relative to the working directory,
// and the path a message prints is absolute.
func TestRelativeConfigFile(t *testing.T) {
	for name, give := range map[string]func(t *testing.T, path string){
		"--config":      giveConfigFlag,
		"WARDEN_CONFIG": giveConfigEnv,
	} {
		t.Run(name, func(t *testing.T) {
			wd := inConfigFiles(t, outputDirFile("from-dir"))
			if err := os.Mkdir(filepath.Join(wd, "ci"), 0o755); err != nil {
				t.Fatal(err)
			}
			write(t, filepath.Join(wd, "ci"), "warden.toml", outputDirFile("named"))
			give(t, filepath.Join("ci", "warden.toml"))
			cfg, err := LoadConfig()
			if err != nil {
				t.Fatalf("LoadConfig: %v", err)
			}
			want := filepath.Join(filepath.Dir(configPath(t)), "ci", "warden.toml")
			if cfg.Build.OutputDir != "named" || cfg.File != want {
				t.Errorf("output_dir = %q from %q, want named from %q",
					cfg.Build.OutputDir, cfg.File, want)
			}
		})
	}
}

// The default is the working directory's file, not the build context's: a
// [path] argument naming a directory that holds a warden.toml does not make
// that file the one read.
func TestBuildContextConfigIsNotRead(t *testing.T) {
	inConfigFiles(t, "")
	context := t.TempDir()
	write(t, context, "Dockerfile", "FROM alpine\n")
	write(t, context, "warden.toml", outputDirFile("from-context"))
	cfg, err := resolveConfig()
	if err != nil {
		t.Fatalf("resolveConfig: %v", err)
	}
	req, err := newRequest(cfg, []string{context})
	if err != nil {
		t.Fatalf("newRequest: %v", err)
	}
	if cfg.File != "" || req.OutputDir != "warden-output" {
		t.Errorf("config file %q, output directory %q; want none and the default",
			cfg.File, req.OutputDir)
	}
}

// The syntax error keeps its text and names the one file by its path.
func TestConfigFileThatIsNotTOML(t *testing.T) {
	inConfigFiles(t, notTOML)
	path := configPath(t)
	_, err := LoadConfig()
	var fe *driver.FixableError
	if !errors.As(err, &fe) || fe.Fix != "fix the TOML syntax in "+path {
		t.Fatalf("LoadConfig = %v, want the syntax error naming %s", err, path)
	}
	if want := "parsing " + path + ": "; !strings.HasPrefix(fe.Msg, want) {
		t.Errorf("message %q does not start with %q", fe.Msg, want)
	}
}

// --config is a global flag. A command that reads no configuration accepts
// it and opens nothing, so a file that does not exist is not an error there.
func TestConfigFlagOnACommandThatReadsNoConfiguration(t *testing.T) {
	inConfigFiles(t, notTOML)
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("LocalAppData", t.TempDir())
	missing := filepath.Join(t.TempDir(), "missing.toml")
	if f := rootCmd.PersistentFlags().Lookup("config"); f == nil || f.Shorthand != "" {
		t.Fatalf("--config = %+v, want a global flag with no short form", f)
	}
	for _, args := range [][]string{
		{"--config", missing, "image", "list"},
		{"image", "list", "--config", missing},
		{"--config", missing, "--version"},
	} {
		rootCmd.SetArgs(args)
		t.Cleanup(func() {
			rootCmd.SetArgs(nil)
			_ = rootCmd.Flags().Set("version", "false")
		})
		if err := rootCmd.Execute(); err != nil {
			t.Errorf("warden %v: %v", args, err)
		}
	}
}

// capFix is the fix the error for a ./warden.toml over the cap gives.
const capFix = "make it 1 MiB or smaller, or name it with --config or " +
	"WARDEN_CONFIG, which reads it whatever its size"

// sizedConfig is a config file of exactly size bytes that sets output_dir to
// "sized", padded with a comment.
func sizedConfig(t *testing.T, size int) string {
	t.Helper()
	body := outputDirFile("sized") + "#"
	if size < len(body) {
		t.Fatalf("a config file of %d bytes cannot hold %q", size, body)
	}
	return body + strings.Repeat("x", size-len(body))
}

// linkToProc makes path a symbolic link to the procfs file proc, which stat
// reports as an empty regular file, and skips the test where it is absent.
func linkToProc(t *testing.T, path, proc string) {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skip("needs " + proc + ", which only Linux has")
	}
	if _, err := os.Stat(proc); err != nil {
		t.Skipf("needs %s: %v", proc, err)
	}
	if err := os.Symlink(proc, path); err != nil {
		t.Fatal(err)
	}
}

// linkToPagemap links path to /proc/self/pagemap, which gives far more than
// 1 MiB and refuses a read whose length is not a multiple of 8.
func linkToPagemap(t *testing.T, path string) {
	linkToProc(t, path, "/proc/self/pagemap")
}

// linkToMem links path to /proc/self/mem, whose first read, at an address
// nothing is mapped at, fails with EIO. It skips where that read succeeds.
func linkToMem(t *testing.T, path string) {
	t.Helper()
	const mem = "/proc/self/mem"
	linkToProc(t, path, mem)
	f, err := os.Open(mem)
	if err != nil {
		t.Skipf("needs %s open: %v", mem, err)
	}
	defer func() { _ = f.Close() }()
	if _, err := f.Read(make([]byte, 1<<16)); err == nil {
		t.Skipf("needs the first read of %s to fail, and it read", mem)
	}
}

// The working directory's warden.toml is read up to 1 MiB, counted as read
// and not as stat reports it: a file that gives more is an error and nothing
// is decoded, and a read that fails before the cap is the OS's error
// (Decision 14, a14).
func TestWorkingDirectoryConfigIsCapped(t *testing.T) {
	const unreadable = "make it readable, or name another file with --config or " +
		"WARDEN_CONFIG"
	for name, tc := range map[string]struct {
		make        func(t *testing.T, path string)
		reason, fix string
	}{
		"exactly 1 MiB": {make: func(t *testing.T, path string) {
			write(t, filepath.Dir(path), projectConfig, sizedConfig(t, 1<<20))
		}},
		"1 MiB and one byte": {reason: "larger than 1 MiB", fix: capFix,
			make: func(t *testing.T, path string) {
				body := sizedConfig(t, 1<<20+1)
				write(t, filepath.Dir(path), projectConfig, body)
			}},
		"a link to /proc/self/pagemap": {make: linkToPagemap,
			reason: "larger than 1 MiB", fix: capFix},
		"a link to /proc/self/mem": {make: linkToMem,
			reason: "input/output error", fix: unreadable},
	} {
		t.Run(name, func(t *testing.T) {
			inConfigFiles(t, "")
			path := configPath(t)
			tc.make(t, path)
			cfg, err := LoadConfig()
			if tc.reason == "" {
				if err != nil {
					t.Fatalf("LoadConfig: %v", err)
				}
				if got := cfg.Build.OutputDir; got != "sized" {
					t.Errorf("output_dir = %q, want %q", got, "sized")
				}
				return
			}
			if cfg != nil {
				t.Error("LoadConfig returned a configuration with its error")
			}
			wantFixable(t, err, "cannot read config file "+path+
				" (found in the working directory): "+tc.reason, tc.fix)
		})
	}
}

// endless is a reader that never ends and counts the bytes it gives. With
// whole set, it refuses a read whose length is not a multiple of 8, as
// /proc/self/pagemap does.
type endless struct {
	given int64
	whole bool
}

func (e *endless) Read(p []byte) (int, error) {
	if e.whole && len(p)%8 != 0 {
		return 0, errors.New("invalid argument")
	}
	for i := range p {
		p[i] = '#'
	}
	e.given += int64(len(p))
	return len(p), nil
}

// failing is a reader that gives n bytes and then fails with err.
type failing struct {
	n   int
	err error
}

func (f *failing) Read(p []byte) (int, error) {
	if f.n == 0 {
		return 0, f.err
	}
	k := min(len(p), f.n)
	f.n -= k
	return k, nil
}

// The capped read reads in blocks of 64 KiB and stops once more than the cap
// has arrived, so it reads no further than the cap and one block whatever the
// reader would give; it returns a reader's error from before the cap, and all
// of a reader that gives exactly the cap. It needs no procfs, so it runs on
// every OS.
func TestCappedReadStopsPastTheCap(t *testing.T) {
	const capAndBlock = 1<<20 + 1<<16
	for name, r := range map[string]*endless{
		"an endless reader":                        {},
		"an endless reader of multiples of 8 only": {whole: true},
	} {
		t.Run(name, func(t *testing.T) {
			data, err := readCapped(r)
			if !errors.Is(err, errTooLarge) || data != nil {
				t.Errorf("readCapped = %d bytes, %v; want no bytes and %v",
					len(data), err, errTooLarge)
			}
			if r.given > capAndBlock {
				t.Errorf("read %d bytes, want at most %d", r.given, capAndBlock)
			}
		})
	}
	t.Run("a reader that fails before the cap", func(t *testing.T) {
		boom := errors.New("boom")
		data, err := readCapped(&failing{n: 1000, err: boom})
		if !errors.Is(err, boom) || data != nil {
			t.Errorf("readCapped = %d bytes, %v; want no bytes and %v",
				len(data), err, boom)
		}
	})
	t.Run("exactly the cap", func(t *testing.T) {
		data, err := readCapped(strings.NewReader(strings.Repeat("#", 1<<20)))
		if err != nil || len(data) != 1<<20 {
			t.Errorf("readCapped = %d bytes, %v; want %d bytes", len(data), err, 1<<20)
		}
	})
}

// A file named with --config or WARDEN_CONFIG is read whole whatever its
// size: the cap is for the file nobody named.
func TestNamedConfigFileIsNotCapped(t *testing.T) {
	for name, give := range map[string]func(*testing.T, string){
		"--config": giveConfigFlag, "WARDEN_CONFIG": giveConfigEnv,
	} {
		t.Run(name, func(t *testing.T) {
			inConfigFiles(t, "")
			path := write(t, t.TempDir(), "big.toml", sizedConfig(t, 2<<20))
			give(t, path)
			cfg, err := LoadConfig()
			if err != nil {
				t.Fatalf("LoadConfig: %v", err)
			}
			if cfg.Build.OutputDir != "sized" {
				t.Errorf("output_dir = %q, want %q", cfg.Build.OutputDir, "sized")
			}
		})
	}
}
