package main

import (
	"bytes"
	"crypto/sha1" //nolint:gosec // the JKS integrity digest is SHA-1 by format
	"crypto/x509"
	"encoding/binary"
	"fmt"
	"unicode/utf16"
)

// jksPassword is the Java truststore password. A truststore holds only public
// certificates; the password is an integrity key, not a secret, and changeit
// is the JDK's own default.
const jksPassword = "changeit"

// writeJKS returns a Java KeyStore (JKS, version 2) holding cert as one
// trusted-certificate entry. This is the stable subset of the format that
// `keytool -importcert -storetype JKS` writes:
//
//	magic 0xFEEDFEED, version 2, entry count
//	per entry: tag 2, alias (UTF), creation time (ms), cert type "X.509" (UTF),
//	           DER length, DER bytes
//	SHA-1(password as UTF-16BE || "Mighty Aphrodite" || everything above)
//
// The creation time is the certificate's NotBefore, so the same certificate
// always yields the same bytes.
func writeJKS(alias string, cert *x509.Certificate, password string) ([]byte, error) {
	var b bytes.Buffer
	put32 := func(v uint32) { _ = binary.Write(&b, binary.BigEndian, v) }
	putUTF := func(s string) error {
		// Java's modified UTF-8 equals UTF-8 for the ASCII aliases and type
		// names written here.
		for _, r := range s {
			if r == 0 || r > 0x7f {
				return fmt.Errorf("jks: non-ASCII string %q", s)
			}
		}
		_ = binary.Write(&b, binary.BigEndian, uint16(len(s)))
		b.WriteString(s)
		return nil
	}

	put32(0xFEEDFEED)
	put32(2)
	put32(1)
	put32(2) // trusted certificate entry
	if err := putUTF(alias); err != nil {
		return nil, err
	}
	_ = binary.Write(&b, binary.BigEndian, uint64(cert.NotBefore.UnixMilli()))
	if err := putUTF("X.509"); err != nil {
		return nil, err
	}
	put32(uint32(len(cert.Raw)))
	b.Write(cert.Raw)

	h := sha1.New() //nolint:gosec // format-mandated
	for _, u := range utf16.Encode([]rune(password)) {
		_ = binary.Write(h, binary.BigEndian, u)
	}
	h.Write([]byte("Mighty Aphrodite"))
	h.Write(b.Bytes())
	b.Write(h.Sum(nil))
	return b.Bytes(), nil
}
