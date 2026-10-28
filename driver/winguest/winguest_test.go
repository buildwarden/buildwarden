package winguest

import (
	"encoding/base64"
	"encoding/binary"
	"strings"
	"testing"
	"unicode/utf16"
)

func decodeEncoded(t *testing.T, b64 string) string {
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

func TestEncodedCommandRoundTrip(t *testing.T) {
	const script = "Write-Host 'hi & bye' \u00e9"
	if got := decodeEncoded(t, EncodedCommand(script)); got != script {
		t.Errorf("round-trip = %q, want %q", got, script)
	}
}

func TestXMLText(t *testing.T) {
	if got := XMLText(`p&w"<x>`); got != "p&amp;w&#34;&lt;x&gt;" {
		t.Errorf("XMLText = %q", got)
	}
}

func TestWardenRunPS1(t *testing.T) {
	ps := WardenRunPS1()
	for _, want := range []string{
		"warden-io.exe",
		"initialize --gateway=" + BuildGatewayIP + " --ip=" + BuildGuestCIDR,
		"Stop-Computer -Force",
		"SerialPort 'COM1'",
		`C:\warden\warden-run.log`,
	} {
		if !strings.Contains(ps, want) {
			t.Errorf("warden-run.ps1 missing %q", want)
		}
	}
	// COM1 must be best effort: opening it is inside a try.
	if i, j := strings.Index(ps, "try {"), strings.Index(ps, "$com.Open()"); i < 0 || j < i {
		t.Error("COM1 open is not guarded by try")
	}
}

func TestStartupTaskCommand(t *testing.T) {
	cmd := StartupTaskCommand(SeedLabel)
	for _, want := range []string{
		"Register-ScheduledTask -TaskName warden-run",
		"-User SYSTEM -RunLevel Highest",
		"-AtStartup",
		"AllowStartIfOnBatteries",
		"DontStopIfGoingOnBatteries",
		"-EncodedCommand ",
	} {
		if !strings.Contains(cmd, want) {
			t.Errorf("task registration missing %q", want)
		}
	}
	// XML/quote safety: no double quotes, no raw ampersand.
	if strings.ContainsAny(cmd, `"&`) {
		t.Error("task registration must contain no double quotes or '&'")
	}
	if len(cmd) >= FirstLogonCommandLimit {
		t.Errorf("task registration is %d chars; must stay under %d",
			len(cmd), FirstLogonCommandLimit)
	}

	rest := cmd[strings.Index(cmd, "-EncodedCommand ")+len("-EncodedCommand "):]
	b64 := rest[:strings.IndexByte(rest, '\'')]
	wait := decodeEncoded(t, b64)
	for _, want := range []string{
		"Get-Volume -FileSystemLabel '" + SeedLabel + "'",
		"warden-run.ps1",
		"Start-Sleep",
		"DriveLetter",
	} {
		if !strings.Contains(wait, want) {
			t.Errorf("decoded wait loop missing %q; got: %s", want, wait)
		}
	}
}
