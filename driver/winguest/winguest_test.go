package winguest

import (
	"encoding/base64"
	"encoding/binary"
	"strings"
	"testing"
	"unicode/utf16"

	"github.com/buildwarden/buildwarden/driver/buildlink"
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

func TestWardenRunPS1ConfiguredLink(t *testing.T) {
	link, err := buildlink.Parse("100.126.255.252/30")
	if err != nil {
		t.Fatal(err)
	}
	ps := WardenRunPS1(link)
	if !strings.Contains(ps, "initialize --gateway=100.126.255.253 2>&1") ||
		strings.Contains(ps, BuildGatewayIP) {
		t.Errorf("warden-run.ps1 does not follow the configured link:\n%s", ps)
	}
}

// TestWardenRunPS1NoIP checks the launcher passes initialize only the relay
// as its gateway, never --ip, so the guest keeps the relay's lease.
func TestWardenRunPS1NoIP(t *testing.T) {
	configured, err := buildlink.Parse("100.126.255.252/30")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		link buildlink.Link
	}{
		{"default", buildlink.Default()},
		{"configured", configured},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ps := WardenRunPS1(tc.link)
			if strings.Contains(ps, "--ip") || strings.Contains(ps, tc.link.GuestCIDR()) {
				t.Errorf("warden-run.ps1 passes a static address:\n%s", ps)
			}
			want := "initialize --gateway=" + tc.link.Relay.String()
			if n := strings.Count(ps, want); n != 2 {
				t.Errorf("%q appears %d times, want 2 (the run and its Emit line)", want, n)
			}
		})
	}
}

func TestWardenRunPS1(t *testing.T) {
	ps := WardenRunPS1(buildlink.Default())
	for _, want := range []string{
		"warden-io.exe",
		"initialize --gateway=" + BuildGatewayIP + " 2>&1",
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
