package main

import (
	"crypto/sha1"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"strings"
	"unicode"
)

// caCertPEM holds the relay's ephemeral CA cert, read from the ledger directory
// by ExtTrustStore and consumed by other extensions (e.g., ExtBazel).
var caCertPEM []byte

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
	for _, rune := range s {
		if rune > 128 {
			inSpace = false
			newStr.WriteRune(rune)
		} else if unicode.IsSpace(rune) {
			if !inSpace {
				inSpace = true
				newStr.WriteRune(' ')
			}
		} else {
			inSpace = false
			newStr.WriteRune(unicode.ToLower(rune))
		}
	}
	return newStr.String()
}
