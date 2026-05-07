package main

import (
	"bufio"
	"crypto"
	"crypto/ed25519"
	"crypto/rsa"
	"crypto/sha512"
	"crypto/x509"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"os"
	"strings"
)

type HeaderEntry struct {
	EntryType       string         `json:"entry_type"`
	Version         string         `json:"version"`
	Format          string         `json:"format"`
	SignatureScheme string         `json:"signature_scheme"`
	Hashes          []string       `json:"hashes"`
	Environment     map[string]any `json:"environment"`
	Payload         *PayloadRecord `json:"payload"`
	Signature       string         `json:"signature"`
}

type LedgerEntry struct {
	EntryType     string         `json:"entry_type"`
	OpenSignature string         `json:"open_signature,omitempty"`
	Direction     string         `json:"direction,omitempty"`
	Payload       *PayloadRecord `json:"payload,omitempty"`
	Signature     string         `json:"signature"`
	Seq           int64          `json:"seq"`
	Timestamp     string         `json:"timestamp"`
	Metadata      map[string]any `json:"metadata,omitempty"`
}

type PayloadRecord struct {
	Size   int64             `json:"size"`
	Hashes map[string]string `json:"hashes"`
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "usage: ledger-inspect <ledger-file>\n")
		os.Exit(1)
	}

	f, err := os.Open(os.Args[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 4*1024*1024), 4*1024*1024)

	// Parse header
	if !scanner.Scan() {
		fmt.Fprintf(os.Stderr, "error: empty ledger\n")
		os.Exit(1)
	}

	var header HeaderEntry
	if err := json.Unmarshal(scanner.Bytes(), &header); err != nil {
		fmt.Fprintf(os.Stderr, "error parsing header: %v\n", err)
		os.Exit(1)
	}

	// Extract public key from payload
	verifier := reconstructVerifier(header)

	// Verify header signature
	headerValid := false
	if verifier != nil {
		headerValid = verifyHeader(header, verifier)
	}

	printHeader(header, headerValid)

	// Process entries
	prevSig := header.Signature
	openChannels := make(map[string]map[string]any) // sig -> metadata
	var totalEntries, opens, checkpoints, closes int
	var sigErrors int

	for scanner.Scan() {
		var entry LedgerEntry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			fmt.Fprintf(os.Stderr, "error parsing entry: %v\n", err)
			continue
		}
		totalEntries++

		valid := false
		if verifier != nil {
			valid = verifyEntry(entry, prevSig, header.Hashes, verifier)
			if !valid {
				sigErrors++
			}
		}

		switch entry.EntryType {
		case "open":
			opens++
			openChannels[entry.Signature] = entry.Metadata
		case "checkpoint":
			checkpoints++
		case "close":
			closes++
			delete(openChannels, entry.OpenSignature)
		}

		printEntry(entry, valid)
		prevSig = entry.Signature
	}

	// Summary
	fmt.Println()
	fmt.Println(strings.Repeat("═", 80))
	fmt.Printf("  SUMMARY: %d entries (%d open, %d checkpoint, %d close)\n",
		totalEntries, opens, checkpoints, closes)
	if verifier != nil {
		if sigErrors == 0 {
			fmt.Printf("  SIGNATURES: ✅ All %d signatures valid\n", totalEntries+1)
		} else {
			fmt.Printf("  SIGNATURES: ❌ %d/%d failed verification\n", sigErrors, totalEntries)
		}
	} else {
		fmt.Printf("  SIGNATURES: ⚠️  Could not verify (no public key)\n")
	}
	if len(openChannels) > 0 {
		fmt.Printf("  UNCLOSED CHANNELS: %d\n", len(openChannels))
		for sig, meta := range openChannels {
			fmt.Printf("    • %s... %v\n", sig[:16], metaSummary(meta))
		}
	} else {
		fmt.Printf("  COMPLETENESS: ✅ All channels closed\n")
	}
	fmt.Println(strings.Repeat("═", 80))
}

func printHeader(h HeaderEntry, valid bool) {
	check := sigStatus(valid)
	fmt.Println(strings.Repeat("═", 80))
	fmt.Printf("  LEDGER v%s  format=%s  hashes=%v\n", h.Version, h.Format, h.Hashes)
	fmt.Printf("  environment: %v\n", h.Environment)
	fmt.Printf("  cert: %d bytes  %s\n", h.Payload.Size, check)
	fmt.Println(strings.Repeat("═", 80))
}

func printEntry(e LedgerEntry, valid bool) {
	check := sigStatus(valid)

	switch e.EntryType {
	case "open":
		fmt.Printf("[%3d] %s OPEN %s  %s\n", e.Seq, check, metaSummary(e.Metadata), e.Timestamp)
	case "checkpoint":
		dir := dirArrow(e.Direction)
		fmt.Printf("[%3d] %s  ├─ CHECKPOINT %s %s (%d bytes)\n",
			e.Seq, check, dir, e.Direction, payloadSize(e.Payload))
	case "close":
		dir := dirArrow(e.Direction)
		fmt.Printf("[%3d] %s  └─ CLOSE %s %s (%d bytes)\n",
			e.Seq, check, dir, e.Direction, payloadSize(e.Payload))
	}
}

func dirArrow(dir string) string {
	if dir == "in" {
		return "◀"
	}
	return "▶"
}

func payloadSize(p *PayloadRecord) int64 {
	if p == nil {
		return 0
	}
	return p.Size
}

func metaSummary(m map[string]any) string {
	if m == nil {
		return ""
	}
	method, _ := m["method"].(string)
	url, _ := m["url"].(string)
	host, _ := m["host"].(string)
	if method != "" && url != "" {
		return fmt.Sprintf("%s %s", method, url)
	}
	if host != "" {
		return host
	}
	b, _ := json.Marshal(m)
	return string(b)
}

func sigStatus(valid bool) string {
	if valid {
		return "✅"
	}
	return "❌"
}

// sigVerifier abstracts signature verification for different schemes.
type sigVerifier interface {
	verify(input []byte, sigB64 string) bool
}

type ed25519Verifier struct {
	key ed25519.PublicKey
}

func (v *ed25519Verifier) verify(input []byte, sigB64 string) bool {
	sigBytes, err := base64.StdEncoding.DecodeString(sigB64)
	if err != nil {
		return false
	}
	digest := sha512.Sum512(input)
	return ed25519.Verify(v.key, digest[:], sigBytes)
}

type rsaVerifier struct {
	key *rsa.PublicKey
}

func (v *rsaVerifier) verify(input []byte, sigB64 string) bool {
	sigBytes, err := base64.StdEncoding.DecodeString(sigB64)
	if err != nil {
		return false
	}
	digest := sha512.Sum512(input)
	return rsa.VerifyPKCS1v15(v.key, crypto.SHA512, digest[:], sigBytes) == nil
}

func reconstructVerifier(h HeaderEntry) sigVerifier {
	if len(os.Args) < 2 {
		return nil
	}
	path := strings.TrimSuffix(os.Args[1], "ledger") + "ledger.cert.pem"
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return nil
	}

	switch {
	case h.SignatureScheme == "ed25519-sha512" || block.Type == "ED25519 PUBLIC KEY":
		if len(block.Bytes) == ed25519.PublicKeySize {
			return &ed25519Verifier{key: ed25519.PublicKey(block.Bytes)}
		}
	default:
		// Legacy RSA
		key, err := x509.ParsePKCS1PublicKey(block.Bytes)
		if err == nil {
			return &rsaVerifier{key: key}
		}
	}
	return nil
}

func verifyHeader(h HeaderEntry, v sigVerifier) bool {
	var sigInput []byte
	sigInput = append(sigInput, []byte("header")...)
	sigInput = append(sigInput, sizeBytes(h.Payload.Size)...)
	sigInput = append(sigInput, rawHashBytes(h.Payload.Hashes, h.Hashes)...)
	return v.verify(sigInput, h.Signature)
}

func verifyEntry(e LedgerEntry, prevSig string, hashes []string, v sigVerifier) bool {
	var sigInput []byte
	prevSigBytes, _ := base64.StdEncoding.DecodeString(prevSig)
	sigInput = append(sigInput, prevSigBytes...)

	switch e.EntryType {
	case "open":
		sigInput = append(sigInput, []byte("open")...)
	case "checkpoint", "close":
		openSigBytes, _ := base64.StdEncoding.DecodeString(e.OpenSignature)
		sigInput = append(sigInput, openSigBytes...)
		sigInput = append(sigInput, []byte(e.EntryType)...)
		sigInput = append(sigInput, []byte(e.Direction)...)
		if e.Payload != nil {
			sigInput = append(sigInput, sizeBytes(e.Payload.Size)...)
			sigInput = append(sigInput, rawHashBytes(e.Payload.Hashes, hashes)...)
		}
	}

	return v.verify(sigInput, e.Signature)
}

func sizeBytes(size int64) []byte {
	buf := make([]byte, 8)
	binary.LittleEndian.PutUint64(buf, uint64(size))
	return buf
}

func rawHashBytes(hashes map[string]string, order []string) []byte {
	var out []byte
	for _, name := range order {
		b, _ := hex.DecodeString(hashes[name])
		out = append(out, b...)
	}
	return out
}
