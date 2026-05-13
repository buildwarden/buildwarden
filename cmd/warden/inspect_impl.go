package main

import (
	"crypto/ed25519"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"warden/ledger"

	"github.com/fxamacker/cbor/v2"
	"github.com/klauspost/compress/zstd"
)

type inspectOptions struct {
	JSON      bool
	Verbosity int
	Writer    io.Writer
	Extract   string
	NoColor   bool
}

func runInspectImpl(path string, opts inspectOptions) error {
	if opts.Writer == nil {
		opts.Writer = os.Stdout
	}

	data, err := readLedger(path)
	if err != nil {
		return err
	}

	if !ledger.IsValidLedger(data) {
		return fmt.Errorf(
			"not a valid BuildWarden ledger (invalid magic bytes)")
	}

	result, err := ledger.Verify(data)
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

	printHuman(opts.Writer, result, opts.Verbosity, ledgerDir, hasCaps, opts.NoColor)
	if !result.Valid {
		return fmt.Errorf("ledger verification failed: %d signature error(s)",
			result.SigErrors)
	}
	return nil
}

type channel struct {
	open        ledger.Record
	checkpoints []ledger.Record
	close       *ledger.Record
}

// channelColors are ANSI colors assigned to concurrent channels to
// visually distinguish interleaved requests in tree output.
var channelColors = []string{
	"\033[36m",  // cyan
	"\033[33m",  // yellow
	"\033[35m",  // magenta
	"\033[32m",  // green
	"\033[34m",  // blue
	"\033[91m",  // bright red
	"\033[96m",  // bright cyan
	"\033[93m",  // bright yellow
}

const colorReset = "\033[0m"

// Reserved hostname colors — visually distinct from channel rotation.
const (
	colorContext  = "\033[2m"    // dim (context fetches are routine plumbing)
	colorArtifact = "\033[1;32m" // bold green (artifacts are the valuable output)
	colorEnv      = "\033[1;35m" // bold magenta (environment identity)
)

type colorTracker struct {
	openSet      map[string]int
	reservedSet  map[string]string // sigKey -> fixed color for reserved hosts
	nextColor    int
	useColor     bool
}

func newColorTracker(useColor bool) *colorTracker {
	return &colorTracker{
		openSet:     make(map[string]int),
		reservedSet: make(map[string]string),
		useColor:    useColor,
	}
}

func (ct *colorTracker) assignOpen(
	sigKey string, rec ledger.Record, schemas []string,
) {
	if isEnvSchema(rec.SchemaIndex, schemas) {
		ct.reservedSet[sigKey] = colorEnv
		return
	}
	if host := extractHost(rec); host != "" {
		switch host {
		case "cwd":
			ct.reservedSet[sigKey] = colorContext
			return
		case "artifacts":
			ct.reservedSet[sigKey] = colorArtifact
			return
		}
	}
	ct.openSet[sigKey] = ct.nextColor % len(channelColors)
	ct.nextColor++
}

func isEnvSchema(idx byte, schemas []string) bool {
	if int(idx) >= len(schemas) {
		return false
	}
	return strings.Contains(schemas[idx], "environment")
}

func (ct *colorTracker) colorFor(rec ledger.Record) string {
	if !ct.useColor {
		return ""
	}
	key := string(rec.Signature)
	if rec.Type != ledger.RecordOpen {
		key = string(rec.OpenSig)
	}
	if c, ok := ct.reservedSet[key]; ok {
		return c
	}
	if rec.Type == ledger.RecordOpen {
		return channelColors[ct.openSet[string(rec.Signature)]]
	}
	if rec.OpenSig != nil {
		if idx, ok := ct.openSet[string(rec.OpenSig)]; ok {
			return channelColors[idx]
		}
	}
	return channelColors[0]
}

func (ct *colorTracker) isReserved(rec ledger.Record) bool {
	key := string(rec.Signature)
	if rec.Type != ledger.RecordOpen {
		key = string(rec.OpenSig)
	}
	_, ok := ct.reservedSet[key]
	return ok
}

func (ct *colorTracker) closeChannel(rec ledger.Record) {
	if rec.OpenSig != nil {
		delete(ct.openSet, string(rec.OpenSig))
		delete(ct.reservedSet, string(rec.OpenSig))
	}
}

func extractHost(rec ledger.Record) string {
	if rec.Metadata == nil {
		return ""
	}
	var m map[string]any
	if err := cbor.Unmarshal(rec.Metadata, &m); err != nil {
		return ""
	}
	url, _ := m["url"].(string)
	if url == "" {
		return ""
	}
	url = strings.TrimPrefix(url, "http://")
	url = strings.TrimPrefix(url, "https://")
	if i := strings.IndexAny(url, ":/"); i >= 0 {
		return url[:i]
	}
	return url
}

func printHuman(
	w io.Writer, result *ledger.VerifyResult, verbosity int,
	ledgerDir string, hasCaps bool, noColor bool,
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
	ct := newColorTracker(verbosity >= 1 && !noColor)
	schemas := h.Meta.Schemas

	for i, rec := range result.Records {
		sigKey := string(rec.Signature)
		ordered = groupRecord(rec, sigKey, channels, ordered, ct, schemas)

		if verbosity >= 1 {
			printEntryTree(
				w, i, rec, ct.colorFor(rec), ct.useColor, verbosity, schemas, ct,
			)
			if rec.Type == ledger.RecordClose || rec.Type == ledger.RecordArtifact {
				ct.closeChannel(rec)
			}
		}
	}

	if verbosity == 0 {
		for _, ch := range ordered {
			printCompact(w, ch, noColor, schemas)
		}
	}

	printSummary(w, result, len(ordered), ledgerDir, hasCaps)
}

func groupRecord(
	rec ledger.Record, sigKey string,
	channels map[string]*channel, ordered []*channel,
	ct *colorTracker, schemas []string,
) []*channel {
	switch rec.Type {
	case ledger.RecordOpen:
		ch := &channel{open: rec}
		channels[sigKey] = ch
		ordered = append(ordered, ch)
		ct.assignOpen(sigKey, rec, schemas)
	case ledger.RecordCheckpoint:
		if ch, ok := channels[string(rec.OpenSig)]; ok {
			ch.checkpoints = append(ch.checkpoints, rec)
		}
	case ledger.RecordClose, ledger.RecordArtifact:
		if ch, ok := channels[string(rec.OpenSig)]; ok {
			ch.close = &rec
		}
	}
	return ordered
}

func printSummary(
	w io.Writer, result *ledger.VerifyResult, requestCount int,
	ledgerDir string, hasCaps bool,
) {
	var totalBytes int64
	var artifactCount int
	for _, rec := range result.Records {
		totalBytes += rec.AbsPayloadSize()
		if rec.Type == ledger.RecordArtifact {
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
	PubKey        string         `json:"pub_key"`
	Signature     string         `json:"signature"`
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
	Schema    string         `json:"schema,omitempty"`
	PrevSig   string         `json:"prev_sig"`
	Signature string         `json:"signature"`
	OpenSig   string         `json:"open_sig,omitempty"`
	HashBlock string         `json:"hash_block,omitempty"`
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
	w io.Writer, result *ledger.VerifyResult,
	ledgerDir string, hasCaps bool,
) error {
	h := result.Header
	headerValid := verifyHeader(h)

	var env map[string]any
	if h.Meta.Environment != nil {
		if normalized, ok := normalizeForJSON(h.Meta.Environment).(map[string]any); ok {
			env = normalized
		}
	}
	report := jsonReport{
		Header: jsonHeader{
			Version:       int(h.Version),
			SigScheme:     h.SigScheme,
			SigSize:       h.SigSize,
			HashBlockSize: h.HashBlockSize,
			PubKey:        base64.StdEncoding.EncodeToString(h.PubKey),
			Signature:     base64.StdEncoding.EncodeToString(h.Signature),
			Hashes:        h.Meta.Hashes,
			Schemas:       h.Meta.Schemas,
			Environment:   env,
			Valid:         headerValid,
		},
	}

	var totalBytes int64
	var artifactCount int
	openCount := 0

	schemas := h.Meta.Schemas
	for i, rec := range result.Records {
		jr := jsonRecord{
			Seq:       i + 1,
			Type:      rec.TypeName(),
			Direction: rec.Direction(),
			Bytes:     rec.AbsPayloadSize(),
			Schema:    resolveSchema(rec.SchemaIndex, schemas),
			PrevSig:   base64.StdEncoding.EncodeToString(rec.PrevSig),
			Signature: base64.StdEncoding.EncodeToString(rec.Signature),
		}
		if rec.OpenSig != nil {
			jr.OpenSig = base64.StdEncoding.EncodeToString(rec.OpenSig)
		}
		if rec.HashBlock != nil {
			jr.HashBlock = hex.EncodeToString(rec.HashBlock)
		}
		if rec.Metadata != nil {
			var m map[string]any
			if err := cbor.Unmarshal(rec.Metadata, &m); err == nil {
				if normalized, ok := normalizeForJSON(m).(map[string]any); ok {
					jr.Metadata = normalized
				}
			}
		}
		report.Records = append(report.Records, jr)

		totalBytes += rec.AbsPayloadSize()
		if rec.Type == ledger.RecordArtifact {
			artifactCount++
		}
		if rec.Type == ledger.RecordOpen {
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

func verifyHeader(h ledger.Header) bool {
	digest := sha512.Sum512(h.PrefixBytes)
	return ed25519.Verify(h.PubKey, digest[:], h.Signature)
}

func printEntryTree(
	w io.Writer, seq int, r ledger.Record,
	color string, useColor bool, verbosity int, schemas []string,
	ct *colorTracker,
) {
	check := "✅"
	reset := ""
	if useColor && color != "" {
		reset = colorReset
	} else {
		color = ""
	}

	sigLabel := ""
	if verbosity >= 2 {
		sigLabel = fmt.Sprintf(" sig=%s", hex.EncodeToString(r.Signature[:8]))
		if r.HashBlock != nil {
			sigLabel += fmt.Sprintf(
				" hash=%s", hex.EncodeToString(r.HashBlock[:8]))
		}
	}

	schemaLabel := ""
	if s := resolveSchema(r.SchemaIndex, schemas); s != "" {
		schemaLabel = fmt.Sprintf(" [%s]", schemaShortName(s))
	}

	reserved := ct.isReserved(r)

	switch r.Type {
	case ledger.RecordOpen:
		meta := metaSummary(r)
		typeLabel := "OPEN"
		if reserved {
			typeLabel = reservedTypeLabel(r, schemas)
		}
		fmt.Fprintf(w, "[%3d] %s %s%s %s%s%s%s\n",
			seq+1, check, color, typeLabel, meta, schemaLabel, sigLabel, reset)
	case ledger.RecordCheckpoint:
		dir := dirArrow(r.Direction())
		openLabel := channelLabel(r.OpenSig, useColor, color)
		fmt.Fprintf(w, "[%3d] %s %s ├─ CHECKPOINT %s %s (%d bytes)%s%s%s%s\n",
			seq+1, check, color, dir, r.Direction(),
			r.AbsPayloadSize(), schemaLabel, openLabel, sigLabel, reset)
	case ledger.RecordClose:
		dir := dirArrow(r.Direction())
		openLabel := channelLabel(r.OpenSig, useColor, color)
		fmt.Fprintf(w, "[%3d] %s %s └─ CLOSE %s %s (%d bytes)%s%s%s%s\n",
			seq+1, check, color, dir, r.Direction(),
			r.AbsPayloadSize(), schemaLabel, openLabel, sigLabel, reset)
	case ledger.RecordArtifact:
		meta := metaSummary(r)
		openLabel := channelLabel(r.OpenSig, useColor, color)
		fmt.Fprintf(w, "[%3d] %s %s └─ ARTIFACT %s (%d bytes)%s%s%s%s\n",
			seq+1, check, color, meta,
			r.AbsPayloadSize(), schemaLabel, openLabel, sigLabel, reset)
	}
}

func reservedTypeLabel(r ledger.Record, schemas []string) string {
	if isEnvSchema(r.SchemaIndex, schemas) {
		return "ENVIRONMENT"
	}
	host := extractHost(r)
	switch host {
	case "cwd":
		return "CONTEXT"
	case "artifacts":
		return "ARTIFACT"
	default:
		return "OPEN"
	}
}

func resolveSchema(idx byte, schemas []string) string {
	if idx == ledger.SchemaNoMetadata {
		return ""
	}
	if int(idx) < len(schemas) {
		return schemas[idx]
	}
	return fmt.Sprintf("unknown(%d)", idx)
}

func schemaShortName(url string) string {
	if i := strings.LastIndex(url, "/"); i >= 0 {
		name := url[i+1:]
		if strings.HasSuffix(name, ".json") {
			return name[:len(name)-5]
		}
		return name
	}
	return url
}

func channelLabel(openSig []byte, useColor bool, _ string) string {
	if openSig == nil {
		return ""
	}
	if !useColor {
		return fmt.Sprintf(" [%s]", hex.EncodeToString(openSig[:4]))
	}
	return ""
}

func printCompact(
	w io.Writer, ch *channel, noColor bool, schemas []string,
) {
	meta := metaSummary(ch.open)
	if meta == "" && ch.close != nil {
		meta = metaSummary(*ch.close)
	}
	status := ""
	var size int64
	if ch.close != nil {
		size = ch.close.AbsPayloadSize()
		if ch.close.Type == ledger.RecordArtifact {
			status = " ARTIFACT"
		}
	} else {
		status = " UNCLOSED"
	}

	host := extractHost(ch.open)
	isEnv := isEnvSchema(ch.open.SchemaIndex, schemas)
	prefix := ""
	suffix := ""
	if !noColor {
		switch {
		case isEnv:
			prefix = colorEnv
			suffix = colorReset
			if status == "" {
				status = " ENVIRONMENT"
			}
		case host == "cwd":
			prefix = colorContext
			suffix = colorReset
			if status == "" {
				status = " CONTEXT"
			}
		case host == "artifacts":
			prefix = colorArtifact
			suffix = colorReset
		}
	} else {
		switch {
		case isEnv:
			if status == "" {
				status = " ENVIRONMENT"
			}
		case host == "cwd":
			if status == "" {
				status = " CONTEXT"
			}
		}
	}

	fmt.Fprintf(w, "%s✅%s %s (%d bytes)%s\n", prefix, status, meta, size, suffix)
}

func metaSummary(r ledger.Record) string {
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
	reference, _ := m["reference"].(string)
	if method != "" && url != "" {
		return fmt.Sprintf("%s %s", method, friendlyURL(url))
	}
	if reference != "" {
		return reference
	}
	if name != "" {
		return fmt.Sprintf("→ %s", name)
	}
	return ""
}

func friendlyURL(url string) string {
	for _, prefix := range []string{"http://cwd", "http://artifacts"} {
		if strings.HasPrefix(url, prefix+"/") {
			return strings.TrimPrefix(url, prefix)
		}
	}
	return url
}

func dirArrow(dir string) string {
	if dir == "in" {
		return "▶"
	}
	return "◀"
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

func normalizeForJSON(v any) any {
	switch val := v.(type) {
	case map[any]any:
		out := make(map[string]any, len(val))
		for k, v2 := range val {
			out[fmt.Sprint(k)] = normalizeForJSON(v2)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(val))
		for k, v2 := range val {
			out[k] = normalizeForJSON(v2)
		}
		return out
	case []any:
		out := make([]any, len(val))
		for i, v2 := range val {
			out[i] = normalizeForJSON(v2)
		}
		return out
	default:
		return v
	}
}

func readLedger(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if strings.HasSuffix(path, ".zst") {
		dec, err := zstd.NewReader(nil)
		if err != nil {
			return nil, fmt.Errorf("zstd init: %w", err)
		}
		defer dec.Close()
		return dec.DecodeAll(data, nil)
	}
	return data, nil
}
