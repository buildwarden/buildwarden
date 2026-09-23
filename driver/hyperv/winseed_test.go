package hyperv

import (
	"encoding/base64"
	"encoding/binary"
	"encoding/xml"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf16"

	"github.com/buildwarden/buildwarden/driver"
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
			t.Fatalf("unattend XML is not well-formed: %v", err)
		}
	}
}

func TestGenerateWindowsUnattend(t *testing.T) {
	xmlStr := generateWindowsUnattend(unattendConfig{AdminPassword: `p&w"<x>`, ProcArch: "amd64"})
	assertWellFormedXML(t, xmlStr)

	for _, want := range []string{
		`pass="oobeSystem"`,
		"<AutoLogon>",
		"<Username>Administrator</Username>",
		"<FirstLogonCommands>",
		`processorArchitecture="amd64"`,
	} {
		if !strings.Contains(xmlStr, want) {
			t.Errorf("unattend missing %q", want)
		}
	}
	// The '&' in the password must be escaped, never raw.
	if strings.Contains(xmlStr, `p&w`) {
		t.Error("password '&' was not XML-escaped")
	}
	if !strings.Contains(xmlStr, "p&amp;w") {
		t.Error("expected escaped password entity p&amp;w")
	}
}

func TestFirstLogonCommand_LengthAndPayload(t *testing.T) {
	cmd := firstLogonCommand()
	if len(cmd) >= 1024 {
		t.Errorf("FirstLogonCommand length %d must stay under the 1024 limit", len(cmd))
	}
	if !strings.HasPrefix(cmd, "powershell.exe -NoProfile -ExecutionPolicy Bypass -EncodedCommand ") {
		t.Fatalf("unexpected command prefix: %q", cmd)
	}
	b64 := strings.TrimPrefix(cmd, "powershell.exe -NoProfile -ExecutionPolicy Bypass -EncodedCommand ")
	decoded := decodePSEncoded(t, b64)
	if !strings.Contains(decoded, "warden-run.ps1") {
		t.Errorf("encoded command should reference warden-run.ps1, got %q", decoded)
	}
}

func TestGenerateWardenRunPS1(t *testing.T) {
	ps := generateWardenRunPS1()
	for _, want := range []string{
		"warden-io.exe",
		"initialize",
		"--gateway=10.0.0.1",
		"--ip=10.0.0.2/30",
		"Stop-Computer",
	} {
		if !strings.Contains(ps, want) {
			t.Errorf("warden-run.ps1 missing %q", want)
		}
	}
}

func TestPSEncodedCommand_RoundTrip(t *testing.T) {
	const script = "Write-Host 'hi & bye'"
	got := decodePSEncoded(t, psEncodedCommand(script))
	if got != script {
		t.Errorf("round-trip = %q, want %q", got, script)
	}
}

func decodePSEncoded(t *testing.T, b64 string) string {
	t.Helper()
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		t.Fatalf("base64 decode: %v", err)
	}
	if len(raw)%2 != 0 {
		t.Fatalf("UTF-16LE payload has odd length %d", len(raw))
	}
	u := make([]uint16, len(raw)/2)
	for i := range u {
		u[i] = binary.LittleEndian.Uint16(raw[i*2:])
	}
	return string(utf16.Decode(u))
}

func TestStageSeedDir(t *testing.T) {
	// Point at a dummy warden-io.exe so ensureWardenIOExe resolves without a build.
	dummy := filepath.Join(t.TempDir(), "warden-io.exe")
	if err := os.WriteFile(dummy, []byte("MZ-stub"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WARDEN_IO_WINDOWS_EXE", dummy)
	t.Setenv("WARDEN_HYPERV_CACHE_DIR", t.TempDir())

	seedDir, err := stageSeedDir(t.TempDir(), "amd64", &driver.BuildRequest{GuestOS: "windows"})
	if err != nil {
		t.Fatalf("stageSeedDir: %v", err)
	}
	for _, name := range []string{"warden-io.exe", "warden-run.ps1", "unattend.xml", "Autounattend.xml"} {
		if _, err := os.Stat(filepath.Join(seedDir, name)); err != nil {
			t.Errorf("seed dir missing %s: %v", name, err)
		}
	}
	// The staged answer file is well-formed and carries the OOBE bootstrap.
	data, err := os.ReadFile(filepath.Join(seedDir, "unattend.xml"))
	if err != nil {
		t.Fatal(err)
	}
	assertWellFormedXML(t, string(data))
	if !strings.Contains(string(data), "FirstLogonCommands") {
		t.Error("staged unattend.xml missing FirstLogonCommands")
	}
}
