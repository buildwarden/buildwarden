package hyperv

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/buildwarden/buildwarden/driver/buildlink"
)

// TestRefreshCacheWritesRelayBootChannel: refresh-cache.sh names the stamp
// once, in a variable it both writes beside the disk and requires of an
// existing disk's stamp before calling the disk current, and that stamp is
// relayBootChannel, not buildlink.Channel.
func TestRefreshCacheWritesRelayBootChannel(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", "tools", "relay-vm", "hyperv",
		"refresh-cache.sh"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	want := fmt.Sprintf("%s=%d", buildlink.StampKey, relayBootChannel)
	for _, w := range []string{`channel="` + want + `"`, `echo "$channel" >"$linkf"`,
		`[ "$link" = "$channel" ]`} {
		if !strings.Contains(s, w) {
			t.Errorf("refresh-cache.sh lacks %q", w)
		}
	}
	if n := strings.Count(s, buildlink.StampKey+"="); n != 1 {
		t.Errorf("refresh-cache.sh names %s %d times, want once", buildlink.StampKey, n)
	}
	if relayBootChannel == buildlink.Channel {
		t.Errorf("relayBootChannel = buildlink.Channel = %d: the relay boot VHDX's "+
			"stamp no longer tells this disk from one built before the relay seed",
			relayBootChannel)
	}
}
