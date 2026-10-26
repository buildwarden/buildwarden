package winguest

import (
	"bytes"
	"strconv"
	"strings"
)

const (
	statusBegin = "WARDEN-PREPARE-STATUS seq="
	statusEnd   = "END-WARDEN-PREPARE-STATUS"
)

// ParseSeedStatus finds the newest complete status block warden-prepare.ps1
// wrote to a writable prepare seed and returns its items (the same names as
// the KVP items). It scans the raw disk image rather than reading the
// filesystem: the file is small, FAT stores its bytes verbatim, and stale
// copies from earlier writes carry lower sequence numbers. ok is false when
// no complete block is present.
func ParseSeedStatus(raw []byte) (items map[string]string, ok bool) {
	var bestSeq int64 = -1
	for off := 0; ; {
		i := bytes.Index(raw[off:], []byte(statusBegin))
		if i < 0 {
			break
		}
		start := off + i
		off = start + len(statusBegin)
		end := bytes.Index(raw[start:], []byte(statusEnd))
		if end < 0 {
			break
		}
		block := string(raw[start : start+end])
		header, body, _ := strings.Cut(block, "\n")
		seq, err := strconv.ParseInt(strings.TrimSpace(
			strings.TrimPrefix(header, statusBegin)), 10, 64)
		if err != nil || seq <= bestSeq {
			continue
		}
		kv := map[string]string{}
		for _, line := range strings.Split(body, "\n") {
			if k, v, found := strings.Cut(strings.TrimRight(line, "\r"), "="); found {
				kv[k] = v
			}
		}
		items, bestSeq = kv, seq
	}
	return items, bestSeq >= 0
}
