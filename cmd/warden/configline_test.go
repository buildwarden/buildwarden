package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fatih/color"

	"github.com/buildwarden/buildwarden/driver"
)

// captureLog collects what warden prints through its logger, which is its
// own lines on stderr, until the test ends. Colors are off, so the prefix is
// the plain "[warden]".
func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	out, noColor := log.out, color.NoColor
	prefix := prefixWarden
	log.out, color.NoColor, prefixWarden = &buf, true, "[warden]"
	t.Cleanup(func() { log.out, color.NoColor, prefixWarden = out, noColor, prefix })
	return &buf
}

// captureStdout runs fn with the process's stdout going to a file, and
// returns what was written there.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	f, err := os.Create(filepath.Join(t.TempDir(), "stdout"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	orig := os.Stdout
	os.Stdout = f
	defer func() { os.Stdout = orig }()
	fn()
	os.Stdout = orig
	data, err := os.ReadFile(f.Name())
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// The config: line has four forms, the path absolute.
func TestConfigLineForms(t *testing.T) {
	const none = "config: none (no --config, no WARDEN_CONFIG, no ./warden.toml): " +
		"defaults apply"
	for _, tc := range []struct {
		name  string
		setup func(t *testing.T) (want string)
	}{
		{"--config", func(t *testing.T) string {
			inConfigFiles(t, "")
			path := write(t, t.TempDir(), "ci.toml", "")
			giveConfigFlag(t, path)
			return "config: " + path + " (--config)"
		}},
		{"WARDEN_CONFIG", func(t *testing.T) string {
			inConfigFiles(t, "")
			path := write(t, t.TempDir(), "ci.toml", "")
			t.Setenv("WARDEN_CONFIG", path)
			return "config: " + path + " (WARDEN_CONFIG)"
		}},
		{"a relative --config is printed absolute", func(t *testing.T) string {
			wd := inConfigFiles(t, "")
			write(t, wd, "ci.toml", "")
			giveConfigFlag(t, "ci.toml")
			return "config: " + filepath.Join(filepath.Dir(configPath(t)), "ci.toml") +
				" (--config)"
		}},
		{"./warden.toml", func(t *testing.T) string {
			inConfigFiles(t, "[output]\nverbose = false\n")
			return "config: " + configPath(t) + " (found in the working directory)"
		}},
		{"none", func(t *testing.T) string {
			inConfigFiles(t, "")
			return none
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			want := tc.setup(t)
			cfg, err := LoadConfig()
			if err != nil {
				t.Fatalf("LoadConfig: %v", err)
			}
			if got := configLine(cfg); got != want {
				t.Errorf("got  %q\nwant %q", got, want)
			}
		})
	}
}

// Every command that reads configuration says which file it read, without
// --verbose: build, shell, clean, setup and image prepare on stderr with the
// [warden] prefix, as their first line and with nothing of it on stdout;
// doctor as the first line of its report, on stdout.
func TestConfigLineOnEveryCommandThatReadsConfiguration(t *testing.T) {
	// Each command is stopped soon after its configuration has loaded: build
	// and shell find no Dockerfile, clean and doctor find no container
	// runtime, image prepare has no image for the container driver.
	onStderr := map[string]func() error{
		"build":         func() error { return runBuild(buildCmd, nil) },
		"shell":         func() error { return runShell(shellCmd, nil) },
		"clean":         func() error { return runClean(cleanCmd, nil) },
		"setup":         func() error { return setupCmd.RunE(setupCmd, nil) },
		"image prepare": func() error { return runImagePrepare(imagePrepareCmd, nil) },
	}
	for name, run := range onStderr {
		t.Run(name, func(t *testing.T) {
			inConfigFiles(t, "[output]\nverbose = false\n")
			t.Setenv("PATH", t.TempDir())
			want := "[warden] config: " + configPath(t) +
				" (found in the working directory)\n"
			stderr := captureLog(t)
			stdout := captureStdout(t, func() { _ = run() })
			if !strings.HasPrefix(stderr.String(), want) {
				t.Errorf("stderr starts %q, want %q", stderr.String(), want)
			}
			if strings.Contains(stdout, "config:") {
				t.Errorf("the config line is on stdout: %q", stdout)
			}
		})
	}
	t.Run("doctor", func(t *testing.T) {
		inConfigFiles(t, "[output]\nverbose = false\n")
		t.Setenv("PATH", t.TempDir())
		want := "config: " + configPath(t) + " (found in the working directory)\n"
		stderr := captureLog(t)
		var report bytes.Buffer
		doctorCmd.SetOut(&report)
		t.Cleanup(func() { doctorCmd.SetOut(nil) })
		_ = doctorCmd.RunE(doctorCmd, nil)
		if !strings.HasPrefix(report.String(), want) {
			t.Errorf("doctor's report starts %q, want %q", report.String(), want)
		}
		if strings.Contains(stderr.String(), "config:") {
			t.Errorf("doctor's config line is on stderr: %q", stderr.String())
		}
	})
}

// A load that fails prints its error, which names the file, and no config:
// line.
func TestNoConfigLineWhenTheLoadFails(t *testing.T) {
	inConfigFiles(t, "")
	giveConfigFlag(t, filepath.Join(t.TempDir(), "missing.toml"))
	stderr := captureLog(t)
	var report bytes.Buffer
	doctorCmd.SetOut(&report)
	t.Cleanup(func() { doctorCmd.SetOut(nil) })
	for name, run := range map[string]func() error{
		"build":  func() error { return runBuild(buildCmd, nil) },
		"doctor": func() error { return doctorCmd.RunE(doctorCmd, nil) },
	} {
		if err := run(); err == nil {
			t.Errorf("%s: no error for a named file that does not exist", name)
		}
	}
	if got := stderr.String() + report.String(); got != "" {
		t.Errorf("a failed load printed %q", got)
	}
}

// The file a command read is recorded for the background image prepare,
// absolute, and none is recorded when none was read.
func TestConfigFileRecordedForTheDetachedPrepare(t *testing.T) {
	t.Cleanup(func() { driver.ConfigFile = "" })
	t.Run("a file", func(t *testing.T) {
		inConfigFiles(t, "")
		path := write(t, t.TempDir(), "ci.toml", "")
		giveConfigFlag(t, path)
		if _, err := resolveConfig(); err != nil || driver.ConfigFile != path {
			t.Errorf("recorded %q, %v; want %s", driver.ConfigFile, err, path)
		}
	})
	t.Run("none", func(t *testing.T) {
		inConfigFiles(t, "")
		if _, err := resolveConfig(); err != nil || driver.ConfigFile != "" {
			t.Errorf("recorded %q, %v; want none", driver.ConfigFile, err)
		}
	})
}
