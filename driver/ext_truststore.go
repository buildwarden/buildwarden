package driver

import (
	"crypto/sha1"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"
)

// caCertPEM holds the relay's ephemeral CA cert, read from the ledger directory.
// Shared between ExtTrustStore and ExtBazel.
var caCertPEM []byte

type ExtTrustStore struct{}

func (e *ExtTrustStore) BeforeBuild(ctx *ExtensionContext) error {
	caPath := filepath.Join(ctx.LedgerDir, "ca.cert.pem")
	var caCert []byte
	for i := 0; i < 50; i++ {
		var err error
		caCert, err = os.ReadFile(caPath)
		if err == nil && len(caCert) > 0 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if len(caCert) == 0 {
		return fmt.Errorf("timed out waiting for CA cert at %s", caPath)
	}

	caCertPEM = caCert

	err := os.WriteFile(
		filepath.Join(ctx.WardenDir, "ca.crt"), caCert, 0644,
	)
	if err != nil {
		return fmt.Errorf("error writing certificate file: %w", err)
	}

	certhash, err := certSubjectHash()
	if err != nil {
		return err
	}

	certhook := fmt.Sprintf(`#!/usr/bin/env sh

rm -rf /etc/ssl/certs
mkdir -p /etc/ssl/certs
cp /.warden/ca.crt /etc/ssl/certs/warden.crt
cp /.warden/ca.crt /etc/ssl/certs/ca-certificates.crt
ln -s warden.crt "/etc/ssl/certs/%s.0"
`, certhash)
	err = os.WriteFile(filepath.Join(ctx.WardenDir, "certhook.sh"),
		[]byte(certhook), 0755)
	if err != nil {
		return fmt.Errorf("failed to write certhook.sh: %w", err)
	}

	script := `#!/usr/bin/env sh

mkdir -p /etc/ca-certificates/update.d
cp /.warden/certhook.sh /etc/ca-certificates/update.d/99warden
sh /etc/ca-certificates/update.d/99warden
`
	err = os.WriteFile(filepath.Join(ctx.ScriptDir, "truststore.sh"),
		[]byte(script), 0755)
	if err != nil {
		return fmt.Errorf("error writing truststore.sh: %w", err)
	}

	return nil
}

func (e *ExtTrustStore) Env() map[string]string { return nil }

type asn1Utf8Value struct {
	Type  asn1.ObjectIdentifier
	Value string `asn1:"utf8"`
}
type asn1Utf8ValueSET []asn1Utf8Value

func certSubjectHash() (string, error) {
	block, _ := pem.Decode(caCertPEM)
	if block == nil {
		return "", fmt.Errorf("failed to parse certificate PEM")
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return "", fmt.Errorf("error parsing relay certificate: %w", err)
	}

	hasher := sha1.New()

	var seq pkix.RDNSequence
	_, err = asn1.Unmarshal(cert.RawSubject, &seq)
	if err != nil {
		return "", fmt.Errorf("error unmarshalling certificate subject: %w", err)
	}

	for _, set := range seq {
		var newSet asn1Utf8ValueSET
		for _, attr := range set {
			val, ok := attr.Value.(string)
			if !ok {
				continue
			}
			newSet = append(newSet, asn1Utf8Value{
				Type:  attr.Type,
				Value: canonicalString(val),
			})
		}
		encoded, err := asn1.Marshal(newSet)
		if err != nil {
			return "", fmt.Errorf("error marshalling certificate subject: %w", err)
		}
		hasher.Write(encoded)
	}
	hash := hex.EncodeToString(hasher.Sum(nil))

	return hash[6:8] + hash[4:6] + hash[2:4] + hash[0:2], nil
}

func canonicalString(s string) string {
	var newStr strings.Builder
	var inSpace bool
	for _, r := range s {
		if r > 128 {
			inSpace = false
			newStr.WriteRune(r)
		} else if unicode.IsSpace(r) {
			if !inSpace {
				inSpace = true
				newStr.WriteRune(' ')
			}
		} else {
			inSpace = false
			newStr.WriteRune(unicode.ToLower(r))
		}
	}
	return newStr.String()
}
