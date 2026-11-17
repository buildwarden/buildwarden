// TRANSITIONAL(one-config-file): removed with config_former.go.

package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

const (
	formerMsg = " is no longer read: warden reads one config file, " +
		"./warden.toml or the one --config or WARDEN_CONFIG names"
	formerFix = "move its settings into the warden.toml you build with, or " +
		"into the environment (WARDEN_DRIVER, WARDEN_CTR_CLI, WARDEN_IMAGE, " +
		"WARDEN_UPSTREAM_CA_CERTS), and delete it; or name it as the config " +
		"file with --config or WARDEN_CONFIG (it is then the only one read: " +
		"./warden.toml is not)"
	formerSudoFix = formerFix + ". Under sudo, WARDEN_CONFIG is dropped unless " +
		"sudo is told to keep it: pass --config, or run sudo " +
		"--preserve-env=WARDEN_CONFIG"
)

// formerFile writes ~/.config/warden/config.toml under the test's home and
// returns its path. Its body is not TOML, so a load that decoded it would
// fail with the syntax error, not the error under test; where the process is
// not root it cannot be opened at all.
func formerFile(t *testing.T) string {
	t.Helper()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(home, ".config", "warden")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := write(t, dir, "config.toml", notTOML)
	if runtime.GOOS != "windows" && os.Geteuid() != 0 {
		if err := os.Chmod(path, 0); err != nil {
			t.Fatal(err)
		}
	}
	return path
}

// configCommands are the six commands that read configuration, each stopped
// by the first error of its configuration load.
var configCommands = map[string]func() error{
	"build":         func() error { return runBuild(buildCmd, nil) },
	"shell":         func() error { return runShell(shellCmd, nil) },
	"clean":         func() error { return runClean(cleanCmd, nil) },
	"doctor":        func() error { return doctorCmd.RunE(doctorCmd, nil) },
	"setup":         func() error { return setupCmd.RunE(setupCmd, nil) },
	"image prepare": func() error { return runImagePrepare(imagePrepareCmd, nil) },
}

// An existing ~/.config/warden/config.toml is an error on every command that
// reads configuration, whichever way the config file was chosen: its
// settings applied yesterday and apply to nothing now. The file is never
// opened.
func TestFormerUserConfigIsAnError(t *testing.T) {
	chosen := map[string]func(t *testing.T){
		"no config file": func(*testing.T) {},
		"./warden.toml":  func(t *testing.T) { write(t, ".", projectConfig, "") },
		"--config naming another file": func(t *testing.T) {
			giveConfigFlag(t, write(t, t.TempDir(), "ci.toml", ""))
		},
		"WARDEN_CONFIG naming another file": func(t *testing.T) {
			giveConfigEnv(t, write(t, t.TempDir(), "ci.toml", ""))
		},
	}
	for how, choose := range chosen {
		for name, run := range configCommands {
			t.Run(name+" with "+how, func(t *testing.T) {
				inConfigFiles(t, "")
				former := formerFile(t)
				choose(t)
				captureLog(t)
				wantFixable(t, run(), former+formerMsg, formerFix)
			})
		}
	}
}

// Naming that file as the config file is the message's second way out: it is
// then the config file, and nothing has stopped applying (a10).
func TestFormerUserConfigMayBeNamed(t *testing.T) {
	for name, give := range map[string]func(t *testing.T, path string){
		"--config":      giveConfigFlag,
		"WARDEN_CONFIG": giveConfigEnv,
	} {
		t.Run(name, func(t *testing.T) {
			inConfigFiles(t, notTOML)
			home, _ := os.UserHomeDir()
			dir := filepath.Join(home, ".config", "warden")
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			former := write(t, dir, "config.toml", outputDirFile("former"))
			give(t, former)
			cfg, err := LoadConfig()
			if err != nil || cfg.Build.OutputDir != "former" || cfg.File != former {
				t.Fatalf("LoadConfig = %+v, %v; want %s read as the config file",
					cfg, err, former)
			}
		})
	}
	t.Run("through a link to it", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("needs a symbolic link, which Windows restricts")
		}
		inConfigFiles(t, "")
		home, _ := os.UserHomeDir()
		dir := filepath.Join(home, ".config", "warden")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		former := write(t, dir, "config.toml", outputDirFile("former"))
		link := filepath.Join(t.TempDir(), "mine.toml")
		if err := os.Symlink(former, link); err != nil {
			t.Fatal(err)
		}
		giveConfigFlag(t, link)
		if _, err := LoadConfig(); err != nil {
			t.Fatalf("LoadConfig: %v", err)
		}
	})
}

func TestFormerUserConfigEdges(t *testing.T) {
	t.Run("no home directory, nothing to look for", func(t *testing.T) {
		inConfigFiles(t, "")
		formerFile(t)
		t.Setenv("HOME", "")
		t.Setenv("USERPROFILE", "")
		if _, err := LoadConfig(); err != nil {
			t.Fatalf("LoadConfig: %v", err)
		}
	})
	t.Run("no such file", func(t *testing.T) {
		inConfigFiles(t, "")
		if _, err := LoadConfig(); err != nil {
			t.Fatalf("LoadConfig: %v", err)
		}
	})
	t.Run("a named file that does not exist is reported first", func(t *testing.T) {
		inConfigFiles(t, "")
		formerFile(t)
		missing := filepath.Join(t.TempDir(), "missing.toml")
		giveConfigFlag(t, missing)
		_, err := LoadConfig()
		wantFixable(t, err, "config file "+missing+" (from --config) does not exist",
			"pass --config an existing file, or drop it to use WARDEN_CONFIG "+
				"or ./warden.toml")
	})
	t.Run("it is reported before the config file's syntax", func(t *testing.T) {
		inConfigFiles(t, notTOML)
		former := formerFile(t)
		_, err := LoadConfig()
		wantFixable(t, err, former+formerMsg, formerFix)
	})
	t.Run("under sudo the fix says the variable is dropped", func(t *testing.T) {
		inConfigFiles(t, "")
		former := formerFile(t)
		t.Setenv("SUDO_USER", "you")
		_, err := LoadConfig()
		wantFixable(t, err, former+formerMsg, formerSudoFix)
	})
	t.Run("without sudo it does not", func(t *testing.T) {
		inConfigFiles(t, "")
		former := formerFile(t)
		t.Setenv("SUDO_USER", "")
		_, err := LoadConfig()
		wantFixable(t, err, former+formerMsg, formerFix)
	})
}

// The variable replaces the key. While this detection stands, both set is an
// error for the commands that use the CA paths (a5).
func TestFormerCAVariableAdded(t *testing.T) {
	const file = "/src/repo/warden.toml"
	cfg := &Config{File: file}
	if err := formerCAVariableAdded(cfg); err != nil {
		t.Errorf("the key unset: %v", err)
	}
	cfg.Relay.UpstreamCACerts = []string{"/etc/warden/corp.pem"}
	wantFixable(t, formerCAVariableAdded(cfg),
		"WARDEN_UPSTREAM_CA_CERTS and [relay] upstream_ca_certs in "+file+
			" are both set: the variable now replaces the key, and no longer "+
			"adds its paths to the key's",
		"put every path in one of the two, and unset the variable or remove the key")
}
