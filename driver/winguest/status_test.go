package winguest

import (
	"strings"
	"testing"
)

func TestParseSeedStatusPicksNewestCompleteBlock(t *testing.T) {
	disk := make([]byte, 4096) // FAT padding around the file data
	raw := string(disk) +
		"WARDEN-PREPARE-STATUS seq=200\nwarden-prepare-state=running:ngen\n" +
		"END-WARDEN-PREPARE-STATUS\n" + string(disk[:512]) +
		"WARDEN-PREPARE-STATUS seq=300\r\nwarden-prepare-state=done\r\n" +
		"warden-grace-minutes=14400\r\nEND-WARDEN-PREPARE-STATUS\n" +
		"WARDEN-PREPARE-STATUS seq=100\nwarden-prepare-state=running:services\n" +
		"END-WARDEN-PREPARE-STATUS\n" +
		"WARDEN-PREPARE-STATUS seq=999\nwarden-prepare-state=torn" // no end
	items, ok := ParseSeedStatus([]byte(raw))
	if !ok {
		t.Fatal("no block found")
	}
	if items[KVPState] != StateDone || items[KVPGraceMinutes] != "14400" {
		t.Errorf("items = %v, want the seq=300 block", items)
	}
}

func TestParseSeedStatusNone(t *testing.T) {
	if _, ok := ParseSeedStatus(make([]byte, 1024)); ok {
		t.Error("found a block in an empty image")
	}
}

func TestPrepareScriptWritesSeedStatus(t *testing.T) {
	ps := PrepareScript()
	for _, want := range []string{
		"-FileSystemLabel '" + PrepareSeedLabel + "'",
		strings.TrimSuffix(statusBegin, "seq=") + "seq=",
		statusEnd,
		"Write-SeedStatus\n}",
	} {
		if !strings.Contains(ps, want) {
			t.Errorf("prepare script missing %q", want)
		}
	}
}
