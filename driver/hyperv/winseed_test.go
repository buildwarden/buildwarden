package hyperv

import (
	"encoding/xml"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/buildwarden/buildwarden/driver"
	"github.com/buildwarden/buildwarden/driver/buildlink"
	"github.com/buildwarden/buildwarden/driver/winguest"
)

func assertWellFormedXML(t *testing.T, s string) {
	t.Helper()
	dec := xml.NewDecoder(strings.NewReader(s))
	for {
		_, err := dec.Token()
		if err == io.EOF {
			return
		}
		if err != nil {
			t.Fatalf("XML is not well-formed: %v", err)
		}
	}
}

func TestStageSeedDir(t *testing.T) {
	seedDir, err := stageSeedDir(
		t.TempDir(), "amd64", &driver.BuildRequest{GuestOS: "windows"},
	)
	if err != nil {
		t.Fatalf("stageSeedDir: %v", err)
	}
	entries, err := os.ReadDir(seedDir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	// The prepared image already went through OOBE: a build seed carries only
	// the agent and the bootstrap the warden-run task runs.
	if got := strings.Join(names, ","); got != "warden-io.exe,warden-run.ps1" {
		t.Errorf("build seed = %s, want warden-io.exe,warden-run.ps1", got)
	}
	ps, _ := os.ReadFile(filepath.Join(seedDir, "warden-run.ps1"))
	if string(ps) != winguest.WardenRunPS1(buildlink.Default()) {
		t.Error("staged warden-run.ps1 is not the shared winguest bootstrap")
	}
}

// TestStageSeedDirConfiguredLink checks the seed carries the shared launcher
// for the build's own link, which passes initialize no --ip.
func TestStageSeedDirConfiguredLink(t *testing.T) {
	link, err := buildlink.Parse("100.126.255.252/30")
	if err != nil {
		t.Fatal(err)
	}
	seedDir, err := stageSeedDir(t.TempDir(), "amd64",
		&driver.BuildRequest{GuestOS: "windows", BuildLink: link})
	if err != nil {
		t.Fatalf("stageSeedDir: %v", err)
	}
	ps, err := os.ReadFile(filepath.Join(seedDir, "warden-run.ps1"))
	if err != nil {
		t.Fatal(err)
	}
	if string(ps) != winguest.WardenRunPS1(link) {
		t.Error("staged warden-run.ps1 is not the shared bootstrap for the build's link")
	}
	if !strings.Contains(string(ps), "initialize --gateway=100.126.255.253 ") ||
		strings.Contains(string(ps), "--ip") {
		t.Errorf("staged warden-run.ps1 does not run initialize --gateway alone:\n%s", ps)
	}
}

func TestStagePrepareSeedDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "prepare-seed")
	if err := winguest.StagePrepareSeed(dir, "amd64"); err != nil {
		t.Fatalf("StagePrepareSeed: %v", err)
	}
	for _, name := range []string{"unattend.xml", "Autounattend.xml"} {
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("missing %s: %v", name, err)
		}
		assertWellFormedXML(t, string(b))
		if !strings.Contains(string(b), "FirstLogonCommands") {
			t.Errorf("%s has no FirstLogonCommands", name)
		}
	}
	ps, err := os.ReadFile(filepath.Join(dir, winguest.PrepareScriptName))
	if err != nil {
		t.Fatal(err)
	}
	if string(ps) != winguest.PrepareScript() {
		t.Error("staged prepare script is not winguest.PrepareScript")
	}
}
