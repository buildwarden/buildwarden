package inspect

import (
	"crypto/ed25519"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"warden/relay"

	"github.com/fxamacker/cbor/v2"
)

type Options struct {
	JSON      bool
	Verbosity int
	Writer    io.Writer
	Extract   string
}

func Run(path string, opts Options) error {
	if opts.Writer == nil {
		opts.Writer = os.Stdout
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	if !relay.IsValidLedger(data) {
		return fmt.Errorf("not a valid BuildWarden ledger (invalid magic bytes)")
	}

	result, err := relay.Verify(data)
	if err != nil {
		return err
	}

	// Detect sibling captures/ directory for payload enrichment.
	ledgerDir := filepath.Dir(path)
	capturesDir := filepath.Join(ledgerDir, "captures")
	hasCaps := dirExists(capturesDir)

	if opts.Extract != "" && hasCaps {
		if err := extractPayloads(capturesDir, opts.Extract, opts.Writer); err != nil {
			return err
		}
	}

	if opts.JSON {
		return printJSON(opts.Writer, result, ledgerDir, hasCaps)
	}

	printHuman(opts.Writer, result, opts.Verbosity, ledgerDir, hasCaps)
	if !result.Valid {
		return fmt.Errorf("ledger verification failed: %d signature error(s)",
			result.SigErrors)
	}
	return nil
}

type channel struct {
	open        relay.Record
	checkpoints []relay.Record
	close       *relay.Record
}

func printHuman(
	w io.Writer, result *relay.VerifyResult, verbosity int,
	ledgerDir string, hasCaps bool,
) {
	h := result.Header
	headerValid := verifyHeader(h)

	fmt.Fprintln(w, strings.Repeat("═", 80))
	fmt.Fprintf(w, "  LEDGER  scheme=%s  hashes=%v  %s\n",
		h.SigScheme, h.Meta.Hashes, sigStatus(headerValid))
	if verbosity >= 1 {
		fmt.Fprintf(w, "  sig_size=%d  hash_block_size=%d  pub_key_len=%d\n",
			h.SigSize, h.HashBlockSize, h.PubKeyLen)
		fmt.Fprintf(w, "  schemas=%v\n", h.Meta.Schemas)
		fmt.Fprintf(w, "  environment: %v\n", h.Meta.Environment)
	}
	fmt.Fprintln(w, strings.Repeat("═", 80))

	channels := make(map[string]*channel)
	var ordered []*channel

	for i, rec := range result.Records {
		if verbosity >= 1 {
			printEntryTree(w, i, rec)
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

	if verbosity == 0 {
		for _, ch := range ordered {
			printCompact(w, ch)
		}
	}

	printSummary(w, result, len(ordered), ledgerDir, hasCaps)
}

func printSummary(
	w io.Writer, result *relay.VerifyResult, requestCount int,
	ledgerDir string, hasCaps bool,
) {
	var totalBytes int64
	var artifactCount int
	for _, rec := range result.Records {
		totalBytes += rec.AbsPayloadSize()
		if rec.Type == relay.RecordArtifact {
			artifactCount++
		}
	}

	fmt.Fprintln(w)
	fmt.Fprintln(w, strings.Repeat("═", 80))
	fmt.Fprintf(w, "  SUMMARY: %d records, %d requests, %s audited",
		result.TotalRecords, requestCount, humanBytes(totalBytes))
	if artifactCount > 0 {
		fmt.Fprintf(w, ", %d artifact(s)", artifactCount)
	}
	fmt.Fprintln(w)
	if result.SigErrors == 0 {
		fmt.Fprintf(w, "  SIGNATURES: ✅ All %d signatures valid\n",
			result.TotalRecords+1)
	} else {
		fmt.Fprintf(w, "  SIGNATURES: ❌ %d signature error(s)\n", result.SigErrors)
	}
	if result.Unclosed > 0 {
		fmt.Fprintf(w, "  COMPLETENESS: ❌ %d unclosed channels\n", result.Unclosed)
	} else {
		fmt.Fprintf(w, "  COMPLETENESS: ✅ All channels closed\n")
	}
	if hasCaps {
		capsDir := filepath.Join(ledgerDir, "captures")
		n := countCaptures(capsDir)
		fmt.Fprintf(w, "  CAPTURES: %d payload(s) saved in %s\n", n, capsDir)
	}
	fmt.Fprintln(w, strings.Repeat("═", 80))
}

type jsonReport struct {
	Header  jsonHeader   `json:"header"`
	Records []jsonRecord `json:"records"`
	Summary jsonSummary  `json:"summary"`
}

type jsonHeader struct {
	Version       int            `json:"version"`
	SigScheme     string         `json:"sig_scheme"`
	SigSize       int            `json:"sig_size"`
	HashBlockSize int            `json:"hash_block_size"`
	Hashes        []string       `json:"hashes"`
	Schemas       []string       `json:"schemas"`
	Environment   map[string]any `json:"environment"`
	Valid         bool           `json:"valid"`
}

type jsonRecord struct {
	Seq       int            `json:"seq"`
	Type      string         `json:"type"`
	Direction string         `json:"direction,omitempty"`
	Bytes     int64          `json:"bytes"`
	Signature string         `json:"signature"`
	OpenSig   string         `json:"open_sig,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

type jsonSummary struct {
	TotalRecords int   `json:"total_records"`
	Requests     int   `json:"requests"`
	BytesAudited int64 `json:"bytes_audited"`
	Artifacts    int   `json:"artifacts"`
	Captures     int   `json:"captures,omitempty"`
	SigErrors    int   `json:"sig_errors"`
	Unclosed     int   `json:"unclosed"`
	Valid        bool  `json:"valid"`
}

func printJSON(
	w io.Writer, result *relay.VerifyResult,
	ledgerDir string, hasCaps bool,
) error {
	h := result.Header
	headerValid := verifyHeader(h)

	report := jsonReport{
		Header: jsonHeader{
			Version:       int(h.Version),
			SigScheme:     h.SigScheme,
			SigSize:       h.SigSize,
			HashBlockSize: h.HashBlockSize,
			Hashes:        h.Meta.Hashes,
			Schemas:       h.Meta.Schemas,
			Environment:   h.Meta.Environment,
			Valid:         headerValid,
		},
	}

	var totalBytes int64
	var artifactCount int
	openCount := 0

	for i, rec := range result.Records {
		jr := jsonRecord{
			Seq:       i + 1,
			Type:      rec.TypeName(),
			Direction: rec.Direction(),
			Bytes:     rec.AbsPayloadSize(),
			Signature: hex.EncodeToString(rec.Signature[:8]),
		}
		if rec.OpenSig != nil {
			jr.OpenSig = hex.EncodeToString(rec.OpenSig[:8])
		}
		if rec.Metadata != nil {
			var m map[string]any
			if err := cbor.Unmarshal(rec.Metadata, &m); err == nil {
				jr.Metadata = m
			}
		}
		report.Records = append(report.Records, jr)

		totalBytes += rec.AbsPayloadSize()
		if rec.Type == relay.RecordArtifact {
			artifactCount++
		}
		if rec.Type == relay.RecordOpen {
			openCount++
		}
	}

	var capsCount int
	if hasCaps {
		capsCount = countCaptures(filepath.Join(ledgerDir, "captures"))
	}

	report.Summary = jsonSummary{
		TotalRecords: result.TotalRecords,
		Requests:     openCount,
		BytesAudited: totalBytes,
		Artifacts:    artifactCount,
		Captures:     capsCount,
		SigErrors:    result.SigErrors,
		Unclosed:     result.Unclosed,
		Valid:        result.Valid,
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(report); err != nil {
		return fmt.Errorf("encoding JSON: %w", err)
	}

	if !result.Valid {
		return fmt.Errorf("ledger verification failed: %d signature error(s)",
			result.SigErrors)
	}
	return nil
}

func verifyHeader(h relay.Header) bool {
	digest := sha512.Sum512(h.PrefixBytes)
	return ed25519.Verify(h.PubKey, digest[:], h.Signature)
}

func printEntryTree(w io.Writer, seq int, r relay.Record) {
	check := "✅"
	switch r.Type {
	case relay.RecordOpen:
		meta := metaSummary(r)
		fmt.Fprintf(w, "[%3d] %s OPEN %s\n", seq+1, check, meta)
	case relay.RecordCheckpoint:
		dir := dirArrow(r.Direction())
		fmt.Fprintf(w, "[%3d] %s  ├─ CHECKPOINT %s %s (%d bytes)\n",
			seq+1, check, dir, r.Direction(), r.AbsPayloadSize())
	case relay.RecordClose:
		dir := dirArrow(r.Direction())
		fmt.Fprintf(w, "[%3d] %s  └─ CLOSE %s %s (%d bytes)\n",
			seq+1, check, dir, r.Direction(), r.AbsPayloadSize())
	case relay.RecordArtifact:
		meta := metaSummary(r)
		fmt.Fprintf(w, "[%3d] %s  └─ ARTIFACT %s (%d bytes)\n",
			seq+1, check, meta, r.AbsPayloadSize())
	}
}

func printCompact(w io.Writer, ch *channel) {
	meta := metaSummary(ch.open)
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
	fmt.Fprintf(w, "✅%s %s (%d bytes)\n", status, meta, size)
}

func metaSummary(r relay.Record) string {
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

func humanBytes(b int64) string {
	switch {
	case b >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(b)/float64(1<<30))
	case b >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(b)/float64(1<<20))
	case b >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(b)/float64(1<<10))
	default:
		return fmt.Sprintf("%d B", b)
	}
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func extractPayloads(capturesDir, destDir string, w io.Writer) error {
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("creating extract dir: %w", err)
	}

	entries, err := os.ReadDir(capturesDir)
	if err != nil {
		return fmt.Errorf("reading captures dir: %w", err)
	}

	var count int
	var totalSize int64
	for _, entry := range entries {
		symPath := filepath.Join(capturesDir, entry.Name())
		target, err := os.Readlink(symPath)
		if err != nil {
			continue
		}
		srcPath := filepath.Join(capturesDir, target)
		info, err := os.Stat(srcPath)
		if err != nil {
			continue
		}

		data, err := os.ReadFile(srcPath)
		if err != nil {
			continue
		}
		destPath := filepath.Join(destDir, entry.Name())
		if err := os.WriteFile(destPath, data, 0644); err != nil {
			continue
		}
		count++
		totalSize += info.Size()
	}

	fmt.Fprintf(w, "Extracted %d payloads (%s) to %s\n",
		count, humanBytes(totalSize), destDir)
	return nil
}

func countCaptures(capturesDir string) int {
	entries, err := os.ReadDir(capturesDir)
	if err != nil {
		return 0
	}
	return len(entries)
}
