package main

import (
	"crypto/ed25519"
	"crypto/sha512"
	"flag"
	"fmt"
	"os"
	"strings"

	"warden/relay"

	"github.com/fxamacker/cbor/v2"
)

// channel tracks a complete request lifecycle.
type channel struct {
	open        relay.Record
	checkpoints []relay.Record
	close       *relay.Record
}

var verbosity int

func main() {
	flag.IntVar(&verbosity, "v", 0, "verbosity: 0=compact, 1=tree, 2=full")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: ledger-inspect [-v N] <ledger-file>\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	if flag.NArg() < 1 {
		flag.Usage()
		os.Exit(1)
	}

	data, err := os.ReadFile(flag.Arg(0))
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if !relay.IsValidLedger(data) {
		fmt.Fprintf(os.Stderr, "error: not a valid BuildWarden ledger (invalid magic bytes)\n")
		os.Exit(1)
	}

	inspect(data)
}

func inspect(data []byte) {
	result, err := relay.Verify(data)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	h := result.Header
	headerValid := verifyHeader(h)

	// Print header
	fmt.Println(strings.Repeat("═", 80))
	fmt.Printf("  LEDGER  scheme=%s  hashes=%v  %s\n",
		h.SigScheme, h.Meta.Hashes, sigStatus(headerValid))
	if verbosity >= 1 {
		fmt.Printf("  sig_size=%d  hash_block_size=%d  pub_key_len=%d\n",
			h.SigSize, h.HashBlockSize, h.PubKeyLen)
		fmt.Printf("  schemas=%v\n", h.Meta.Schemas)
		fmt.Printf("  environment: %v\n", h.Meta.Environment)
	}
	fmt.Println(strings.Repeat("═", 80))

	// Track channels for compact output
	channels := make(map[string]*channel)
	var ordered []*channel

	for i, rec := range result.Records {
		if verbosity >= 1 {
			printEntryTree(i, rec, h)
		}

		sigKey := string(rec.Signature)
		switch rec.Type {
		case relay.RecordOpen:
			ch := &channel{open: rec}
			channels[sigKey] = ch
			ordered = append(ordered, ch)
		case relay.RecordCheckpoint:
			if ch, ok := channels[string(rec.OpenSig)]; ok {
				ch.checkpoints = append(ch.checkpoints, rec)
			}
		case relay.RecordClose, relay.RecordArtifact:
			if ch, ok := channels[string(rec.OpenSig)]; ok {
				ch.close = &rec
			}
		}
	}

	// Compact output
	if verbosity == 0 {
		for _, ch := range ordered {
			printCompact(ch, h)
		}
	}

	// Summary
	fmt.Println()
	fmt.Println(strings.Repeat("═", 80))
	fmt.Printf("  SUMMARY: %d entries (%d requests)\n",
		result.TotalRecords, len(ordered))
	if result.SigErrors == 0 {
		fmt.Printf("  SIGNATURES: ✅ All %d signatures valid\n",
			result.TotalRecords+1) // +1 for header
	} else {
		fmt.Printf("  SIGNATURES: ❌ %d signature error(s)\n", result.SigErrors)
	}
	if result.Unclosed > 0 {
		fmt.Printf("  COMPLETENESS: ❌ %d unclosed channels\n", result.Unclosed)
	} else {
		fmt.Printf("  COMPLETENESS: ✅ All channels closed\n")
	}
	fmt.Println(strings.Repeat("═", 80))
}

func verifyHeader(h relay.Header) bool {
	digest := sha512.Sum512(h.PrefixBytes)
	return ed25519.Verify(h.PubKey, digest[:], h.Signature)
}

func printEntryTree(seq int, r relay.Record, h relay.Header) {
	check := "✅"
	switch r.Type {
	case relay.RecordOpen:
		meta := metaSummary(r, h)
		fmt.Printf("[%3d] %s OPEN %s\n", seq+1, check, meta)
	case relay.RecordCheckpoint:
		dir := dirArrow(r.Direction())
		fmt.Printf("[%3d] %s  ├─ CHECKPOINT %s %s (%d bytes)\n",
			seq+1, check, dir, r.Direction(), r.AbsPayloadSize())
	case relay.RecordClose:
		dir := dirArrow(r.Direction())
		fmt.Printf("[%3d] %s  └─ CLOSE %s %s (%d bytes)\n",
			seq+1, check, dir, r.Direction(), r.AbsPayloadSize())
	case relay.RecordArtifact:
		meta := metaSummary(r, h)
		fmt.Printf("[%3d] %s  └─ ARTIFACT %s (%d bytes)\n",
			seq+1, check, meta, r.AbsPayloadSize())
	}
}

func printCompact(ch *channel, h relay.Header) {
	meta := metaSummary(ch.open, h)
	status := ""
	var size int64
	if ch.close != nil {
		size = ch.close.AbsPayloadSize()
		if ch.close.Type == relay.RecordArtifact {
			status = " ARTIFACT"
		}
	} else {
		status = " UNCLOSED"
	}
	fmt.Printf("✅%s %s (%d bytes)\n", status, meta, size)
}

func metaSummary(r relay.Record, h relay.Header) string {
	if r.Metadata == nil {
		return ""
	}
	var m map[string]any
	if err := cbor.Unmarshal(r.Metadata, &m); err != nil {
		return ""
	}
	method, _ := m["method"].(string)
	url, _ := m["url"].(string)
	name, _ := m["name"].(string)
	if method != "" && url != "" {
		return fmt.Sprintf("%s %s", method, url)
	}
	if name != "" {
		return fmt.Sprintf("→ %s", name)
	}
	return ""
}

func dirArrow(dir string) string {
	if dir == "in" {
		return "◀"
	}
	return "▶"
}

func sigStatus(valid bool) string {
	if valid {
		return "✅"
	}
	return "❌"
}
