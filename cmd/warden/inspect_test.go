package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha512"
	"encoding/binary"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"warden/ledger"

	"github.com/fxamacker/cbor/v2"
	"github.com/klauspost/compress/zstd"
)

// buildLedger constructs a minimal valid ledger binary (header only, no
// records) using the specification:
//
//	magic "BLDL" + version 0x01 + null-terminated sig scheme +
//	uint16 BE sig_size + uint16 BE hash_block_size + uint16 BE pub_key_len +
//	pub_key + signature + uint32 BE meta_len + CBOR meta
func buildLedger(t *testing.T) ([]byte, ed25519.PrivateKey) {
	t.Helper()

	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}

	var prefix bytes.Buffer
	// Magic + version
	prefix.WriteString("BLDL")
	prefix.WriteByte(0x01)
	// Signature scheme (null-terminated)
	prefix.WriteString("ed25519-sha512")
	prefix.WriteByte(0x00)
	// sig_size = 64, hash_block_size = 32, pub_key_len = 32
	binary.Write(&prefix, binary.BigEndian, uint16(64)) //nolint:errcheck
	binary.Write(&prefix, binary.BigEndian, uint16(32)) //nolint:errcheck
	binary.Write(&prefix, binary.BigEndian, uint16(32)) //nolint:errcheck
	// Public key
	prefix.Write(pub)

	prefixBytes := prefix.Bytes()

	// Sign: sha512(prefix) then ed25519.Sign
	digest := sha512.Sum512(prefixBytes)
	sig := ed25519.Sign(priv, digest[:])

	// CBOR-encode header metadata
	meta := ledger.HeaderMeta{
		Hashes:      []string{"sha256"},
		Schemas:     []string{},
		Environment: map[string]any{},
	}
	metaBytes, err := cbor.Marshal(meta)
	if err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	buf.Write(prefixBytes)
	buf.Write(sig)
	binary.Write(&buf, binary.BigEndian, uint32(len(metaBytes))) //nolint:errcheck
	buf.Write(metaBytes)

	return buf.Bytes(), priv
}

// buildLedgerWithRecord constructs a valid ledger with a single open+close
// record pair so we get a non-empty record list.
func buildLedgerWithRecord(t *testing.T) []byte {
	t.Helper()

	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}

	var prefix bytes.Buffer
	prefix.WriteString("BLDL")
	prefix.WriteByte(0x01)
	prefix.WriteString("ed25519-sha512")
	prefix.WriteByte(0x00)
	binary.Write(&prefix, binary.BigEndian, uint16(64)) //nolint:errcheck
	binary.Write(&prefix, binary.BigEndian, uint16(32)) //nolint:errcheck
	binary.Write(&prefix, binary.BigEndian, uint16(32)) //nolint:errcheck
	prefix.Write(pub)

	prefixBytes := prefix.Bytes()
	digest := sha512.Sum512(prefixBytes)
	headerSig := ed25519.Sign(priv, digest[:])

	meta := ledger.HeaderMeta{
		Hashes:      []string{"sha256"},
		Schemas:     []string{},
		Environment: map[string]any{},
	}
	metaBytes, err := cbor.Marshal(meta)
	if err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	buf.Write(prefixBytes)
	buf.Write(headerSig)
	binary.Write(&buf, binary.BigEndian, uint32(len(metaBytes))) //nolint:errcheck
	buf.Write(metaBytes)

	// Build an open record (type=0x01, prev_sig=headerSig,
	// payload_size=0, no hash_block, signature, schema=0xFF no metadata)
	openInput := buildRecordInput(ledger.RecordOpen, headerSig, nil, 0, nil)
	openDigest := sha512.Sum512(openInput)
	openSig := ed25519.Sign(priv, openDigest[:])

	buf.WriteByte(ledger.RecordOpen)
	buf.Write(headerSig) // prev_sig
	binary.Write(&buf, binary.BigEndian, int64(0)) //nolint:errcheck
	buf.Write(openSig)
	buf.WriteByte(0xFF) // no metadata

	// Build a close record (type=0x03, prev_sig=openSig, open_sig=openSig,
	// payload_size=0, signature, schema=0xFF)
	closeInput := buildRecordInput(
		ledger.RecordClose, openSig, openSig, 0, nil,
	)
	closeDigest := sha512.Sum512(closeInput)
	closeSig := ed25519.Sign(priv, closeDigest[:])

	buf.WriteByte(ledger.RecordClose)
	buf.Write(openSig)                             // prev_sig
	buf.Write(openSig)                             // open_sig
	binary.Write(&buf, binary.BigEndian, int64(0)) //nolint:errcheck
	buf.Write(closeSig)
	buf.WriteByte(0xFF) // no metadata

	return buf.Bytes()
}

// buildRecordInput reconstructs the bytes that are signed for a record,
// matching the ledger package's buildRecordSigInput.
func buildRecordInput(
	recType byte, prevSig, openSig []byte,
	payloadSize int64, hashBlock []byte,
) []byte {
	var b []byte
	b = append(b, recType)
	b = append(b, prevSig...)
	if recType != ledger.RecordOpen {
		b = append(b, openSig...)
	}
	b = binary.BigEndian.AppendUint64(b, uint64(payloadSize))
	if payloadSize != 0 {
		b = append(b, hashBlock...)
	}
	return b
}

func writeTempLedger(t *testing.T, name string, data []byte) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, data, 0644); err != nil {
		t.Fatal(err)
	}
	return p
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestRunInspectImpl_ValidEmptyLedger(t *testing.T) {
	data, _ := buildLedger(t)
	path := writeTempLedger(t, "ledger", data)

	var out bytes.Buffer
	err := runInspectImpl(path, inspectOptions{Writer: &out})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "LEDGER") {
		t.Error("output should contain LEDGER header")
	}
	if !strings.Contains(output, "ed25519-sha512") {
		t.Error("output should contain signature scheme")
	}
	if !strings.Contains(output, "All channels closed") {
		t.Error("output should report all channels closed")
	}
}

func TestRunInspectImpl_ValidLedgerWithRecords(t *testing.T) {
	data := buildLedgerWithRecord(t)
	path := writeTempLedger(t, "ledger", data)

	var out bytes.Buffer
	err := runInspectImpl(path, inspectOptions{Writer: &out})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "2 records") {
		t.Errorf("expected summary to mention 2 records, got:\n%s", output)
	}
	if !strings.Contains(output, "All channels closed") {
		t.Error("channels should all be closed")
	}
}

func TestRunInspectImpl_JSONOutput(t *testing.T) {
	data, _ := buildLedger(t)
	path := writeTempLedger(t, "ledger", data)

	var out bytes.Buffer
	err := runInspectImpl(path, inspectOptions{
		JSON:   true,
		Writer: &out,
	})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	var report jsonReport
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatalf("failed to parse JSON output: %v\nraw: %s", err, out.String())
	}

	if report.Header.SigScheme != "ed25519-sha512" {
		t.Errorf("unexpected sig_scheme: %s", report.Header.SigScheme)
	}
	if report.Header.Version != 1 {
		t.Errorf("unexpected version: %d", report.Header.Version)
	}
	if report.Header.SigSize != 64 {
		t.Errorf("unexpected sig_size: %d", report.Header.SigSize)
	}
	if !report.Header.Valid {
		t.Error("header should be valid")
	}
	if report.Header.PubKey == "" {
		t.Error("header pub_key should be non-empty base64")
	}
	if report.Header.Signature == "" {
		t.Error("header signature should be non-empty base64")
	}
	if !report.Summary.Valid {
		t.Error("summary should be valid")
	}
	if report.Summary.SigErrors != 0 {
		t.Errorf("expected 0 sig errors, got %d", report.Summary.SigErrors)
	}
}

func TestRunInspectImpl_JSONOutputWithRecords(t *testing.T) {
	data := buildLedgerWithRecord(t)
	path := writeTempLedger(t, "ledger", data)

	var out bytes.Buffer
	err := runInspectImpl(path, inspectOptions{
		JSON:   true,
		Writer: &out,
	})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	var report jsonReport
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}

	if len(report.Records) != 2 {
		t.Fatalf("expected 2 records, got %d", len(report.Records))
	}
	if report.Records[0].Type != "open" {
		t.Errorf("first record should be open, got %s", report.Records[0].Type)
	}
	if report.Records[1].Type != "close" {
		t.Errorf("second record should be close, got %s", report.Records[1].Type)
	}
	// Full base64 signatures should be present
	if report.Records[0].Signature == "" {
		t.Error("open record should have base64 signature")
	}
	if report.Records[0].PrevSig == "" {
		t.Error("open record should have base64 prev_sig")
	}
	if report.Records[1].OpenSig == "" {
		t.Error("close record should have base64 open_sig")
	}
	if report.Summary.TotalRecords != 2 {
		t.Errorf("expected 2 total records, got %d", report.Summary.TotalRecords)
	}
	if report.Summary.Requests != 1 {
		t.Errorf("expected 1 request, got %d", report.Summary.Requests)
	}
}

func TestRunInspectImpl_InvalidFile(t *testing.T) {
	path := writeTempLedger(t, "bad", []byte("not a ledger"))

	var out bytes.Buffer
	err := runInspectImpl(path, inspectOptions{Writer: &out})
	if err == nil {
		t.Fatal("expected error for invalid ledger")
	}
	if !strings.Contains(err.Error(), "invalid magic") {
		t.Errorf("expected magic-bytes error, got: %v", err)
	}
}

func TestRunInspectImpl_EmptyFile(t *testing.T) {
	path := writeTempLedger(t, "empty", []byte{})

	var out bytes.Buffer
	err := runInspectImpl(path, inspectOptions{Writer: &out})
	if err == nil {
		t.Fatal("expected error for empty file")
	}
}

func TestRunInspectImpl_NonexistentFile(t *testing.T) {
	var out bytes.Buffer
	err := runInspectImpl("/no/such/file", inspectOptions{Writer: &out})
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestRunInspectImpl_Verbosity1(t *testing.T) {
	data := buildLedgerWithRecord(t)
	path := writeTempLedger(t, "ledger", data)

	var out bytes.Buffer
	err := runInspectImpl(path, inspectOptions{
		Verbosity: 1,
		Writer:    &out,
	})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	output := out.String()
	// Verbosity >= 1 prints sig_size, hash_block_size, pub_key_len
	if !strings.Contains(output, "sig_size=64") {
		t.Error("verbosity 1 should show sig_size")
	}
	if !strings.Contains(output, "hash_block_size=32") {
		t.Error("verbosity 1 should show hash_block_size")
	}
	// Verbosity >= 1 prints entry tree with sequence numbers
	if !strings.Contains(output, "[  1]") {
		t.Error("verbosity 1 should print entry tree with seq numbers")
	}
}

func TestRunInspectImpl_Verbosity2(t *testing.T) {
	data := buildLedgerWithRecord(t)
	path := writeTempLedger(t, "ledger", data)

	var out bytes.Buffer
	err := runInspectImpl(path, inspectOptions{
		Verbosity: 2,
		Writer:    &out,
		NoColor:   true,
	})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	output := out.String()
	// Verbosity 2 includes signature hex prefixes
	if !strings.Contains(output, "sig=") {
		t.Error("verbosity 2 should show sig= labels")
	}
}

func TestRunInspectImpl_NoColor(t *testing.T) {
	data := buildLedgerWithRecord(t)
	path := writeTempLedger(t, "ledger", data)

	var out bytes.Buffer
	err := runInspectImpl(path, inspectOptions{
		Verbosity: 1,
		Writer:    &out,
		NoColor:   true,
	})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	output := out.String()
	if strings.Contains(output, "\033[") {
		t.Error("no-color output should not contain ANSI escape codes")
	}
}

func TestRunInspectImpl_ColorOutput(t *testing.T) {
	data := buildLedgerWithRecord(t)
	path := writeTempLedger(t, "ledger", data)

	var out bytes.Buffer
	err := runInspectImpl(path, inspectOptions{
		Verbosity: 1,
		Writer:    &out,
		NoColor:   false,
	})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "\033[") {
		t.Error("colored output should contain ANSI escape codes")
	}
	if !strings.Contains(output, colorReset) {
		t.Error("colored output should contain reset sequences")
	}
}

// ---------------------------------------------------------------------------
// readLedger tests
// ---------------------------------------------------------------------------

func TestReadLedger_PlainFile(t *testing.T) {
	data, _ := buildLedger(t)
	path := writeTempLedger(t, "ledger", data)

	got, err := readLedger(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, data) {
		t.Error("readLedger should return raw bytes for non-.zst files")
	}
}

func TestReadLedger_ZstdCompressed(t *testing.T) {
	data, _ := buildLedger(t)

	enc, err := zstd.NewWriter(nil)
	if err != nil {
		t.Fatal(err)
	}
	compressed := enc.EncodeAll(data, nil)
	enc.Close()

	path := writeTempLedger(t, "ledger.zst", compressed)

	got, err := readLedger(path)
	if err != nil {
		t.Fatalf("readLedger .zst failed: %v", err)
	}
	if !bytes.Equal(got, data) {
		t.Error("readLedger should decompress .zst files")
	}
}

func TestReadLedger_ZstdInvalidData(t *testing.T) {
	path := writeTempLedger(t, "corrupt.zst", []byte("not zstd"))

	_, err := readLedger(path)
	if err == nil {
		t.Fatal("expected error for corrupt .zst file")
	}
}

// ---------------------------------------------------------------------------
// resolveLedgerPath tests
// ---------------------------------------------------------------------------

func TestResolveLedgerPath_DirectFile(t *testing.T) {
	data, _ := buildLedger(t)
	path := writeTempLedger(t, "ledger", data)

	got := resolveLedgerPath(path)
	if got != path {
		t.Errorf("expected %s, got %s", path, got)
	}
}

func TestResolveLedgerPath_DirWithZst(t *testing.T) {
	dir := t.TempDir()
	zstPath := filepath.Join(dir, "ledger.zst")
	if err := os.WriteFile(zstPath, []byte("placeholder"), 0644); err != nil {
		t.Fatal(err)
	}

	got := resolveLedgerPath(dir)
	if got != zstPath {
		t.Errorf("expected %s, got %s", zstPath, got)
	}
}

func TestResolveLedgerPath_DirWithPlainLedger(t *testing.T) {
	dir := t.TempDir()
	ledgerPath := filepath.Join(dir, "ledger")
	if err := os.WriteFile(ledgerPath, []byte("placeholder"), 0644); err != nil {
		t.Fatal(err)
	}

	got := resolveLedgerPath(dir)
	if got != ledgerPath {
		t.Errorf("expected %s, got %s", ledgerPath, got)
	}
}

func TestResolveLedgerPath_DirPreferZst(t *testing.T) {
	dir := t.TempDir()
	// Create both files; .zst should be preferred.
	for _, name := range []string{"ledger", "ledger.zst"} {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	got := resolveLedgerPath(dir)
	want := filepath.Join(dir, "ledger.zst")
	if got != want {
		t.Errorf("expected %s, got %s", want, got)
	}
}

func TestResolveLedgerPath_DirWithNeither(t *testing.T) {
	dir := t.TempDir()

	got := resolveLedgerPath(dir)
	if got != dir {
		t.Errorf("expected dir path back, got %s", got)
	}
}

func TestResolveLedgerPath_NonexistentPath(t *testing.T) {
	path := "/no/such/path"
	got := resolveLedgerPath(path)
	if got != path {
		t.Errorf("expected %s, got %s", path, got)
	}
}

// ---------------------------------------------------------------------------
// Helper function tests
// ---------------------------------------------------------------------------

func TestHumanBytes(t *testing.T) {
	tests := []struct {
		input int64
		want  string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1023, "1023 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1048576, "1.0 MB"},
		{1572864, "1.5 MB"},
		{1073741824, "1.0 GB"},
		{1610612736, "1.5 GB"},
	}
	for _, tc := range tests {
		got := humanBytes(tc.input)
		if got != tc.want {
			t.Errorf("humanBytes(%d) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestDirArrow(t *testing.T) {
	if got := dirArrow("in"); got != "▶" {
		t.Errorf("dirArrow(in) = %q, want ▶", got)
	}
	if got := dirArrow("out"); got != "◀" {
		t.Errorf("dirArrow(out) = %q, want ◀", got)
	}
	if got := dirArrow(""); got != "◀" {
		t.Errorf("dirArrow(\"\") = %q, want ◀", got)
	}
}

func TestSigStatus(t *testing.T) {
	valid := sigStatus(true)
	invalid := sigStatus(false)

	if valid == invalid {
		t.Error("sigStatus(true) and sigStatus(false) should differ")
	}
	// The actual strings contain emoji, just verify they are non-empty.
	if len(valid) == 0 || len(invalid) == 0 {
		t.Error("sigStatus should return non-empty strings")
	}
}

// ---------------------------------------------------------------------------
// End-to-end: zst file through runInspectImpl
// ---------------------------------------------------------------------------

func TestRunInspectImpl_ZstFile(t *testing.T) {
	data, _ := buildLedger(t)

	enc, err := zstd.NewWriter(nil)
	if err != nil {
		t.Fatal(err)
	}
	compressed := enc.EncodeAll(data, nil)
	enc.Close()

	path := writeTempLedger(t, "ledger.zst", compressed)

	var out bytes.Buffer
	err = runInspectImpl(path, inspectOptions{Writer: &out})
	if err != nil {
		t.Fatalf("runInspectImpl with .zst file failed: %v", err)
	}
	if !strings.Contains(out.String(), "LEDGER") {
		t.Error("output should contain LEDGER header")
	}
}
