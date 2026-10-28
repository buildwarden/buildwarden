//go:build linux

package main

import (
	"crypto/sha1" //nolint:gosec // OpenSSL's subject hash is SHA-1 by definition
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

// wardenCAPath is where the relay CA is kept on Linux guests; the exclusive
// trust hook re-applies it from here.
const wardenCAPath = "/usr/local/share/ca-certificates/warden-ca.crt"

// otherBundles are system CA bundles outside /etc/ssl/certs that some
// distributions (RHEL, Fedora, Alpine's cert.pem) read directly.
var otherBundles = []string{
	"/etc/pki/tls/certs/ca-bundle.crt",
	"/etc/pki/ca-trust/extracted/pem/tls-ca-bundle.pem",
	"/etc/ssl/cert.pem",
}

// trustHook is installed as a ca-certificates update hook, so a build that
// reinstalls ca-certificates (apt, apk) gets exclusive trust back. It runs
// last (99) and touches only what installExclusiveCA does.
const trustHook = `#!/bin/sh
# Installed by warden-io: the warden CA is the only trusted root.
rm -rf /etc/ssl/certs
mkdir -p /etc/ssl/certs
cp %[1]s /etc/ssl/certs/warden.crt
cp %[1]s /etc/ssl/certs/ca-certificates.crt
ln -s warden.crt /etc/ssl/certs/%[2]s.0
`

// installExclusiveCA makes the warden CA the only system root: every TLS
// connection goes through the relay's MITM, so no other root can validate a
// real connection. pemData has already been written to wardenCAPath.
func installExclusiveCA(pemData []byte, cert *x509.Certificate) error {
	hash, err := subjectHash(cert)
	if err != nil {
		return err
	}
	if err := os.RemoveAll("/etc/ssl/certs"); err != nil {
		return fmt.Errorf("clearing /etc/ssl/certs: %w", err)
	}
	if err := os.MkdirAll("/etc/ssl/certs", 0o755); err != nil {
		return err
	}
	for _, name := range []string{"warden.crt", "ca-certificates.crt"} {
		if err := os.WriteFile(filepath.Join("/etc/ssl/certs", name), pemData, 0o644); err != nil {
			return err
		}
	}
	if err := os.Symlink("warden.crt", filepath.Join("/etc/ssl/certs", hash+".0")); err != nil {
		return err
	}
	for _, b := range otherBundles {
		if _, err := os.Lstat(b); err == nil {
			_ = os.Remove(b) // may be a symlink into the old /etc/ssl/certs
			if err := os.WriteFile(b, pemData, 0o644); err != nil {
				return fmt.Errorf("writing %s: %w", b, err)
			}
		}
	}
	hookDir := "/etc/ca-certificates/update.d"
	if err := os.MkdirAll(hookDir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(hookDir, "99warden"),
		fmt.Appendf(nil, trustHook, wardenCAPath, hash), 0o755)
}

type asn1UTF8Value struct {
	Type  asn1.ObjectIdentifier
	Value string `asn1:"utf8"`
}

// asn1UTF8ValueSET encodes as an ASN.1 SET: encoding/asn1 picks SET for a
// slice type whose name ends in "SET".
type asn1UTF8ValueSET []asn1UTF8Value

// subjectHash is OpenSSL's X509_NAME_hash (`openssl x509 -subject_hash`): the
// first four bytes, little-endian, of the SHA-1 of the canonical subject. It
// names the c_rehash link OpenSSL looks the CA up by.
func subjectHash(cert *x509.Certificate) (string, error) {
	var seq pkix.RDNSequence
	if _, err := asn1.Unmarshal(cert.RawSubject, &seq); err != nil {
		return "", fmt.Errorf("parsing certificate subject: %w", err)
	}
	h := sha1.New() //nolint:gosec // see above
	for _, set := range seq {
		var canon asn1UTF8ValueSET
		for _, attr := range set {
			val, ok := attr.Value.(string)
			if !ok {
				continue
			}
			canon = append(canon, asn1UTF8Value{Type: attr.Type, Value: canonicalString(val)})
		}
		enc, err := asn1.Marshal(canon)
		if err != nil {
			return "", fmt.Errorf("encoding certificate subject: %w", err)
		}
		h.Write(enc)
	}
	s := hex.EncodeToString(h.Sum(nil))
	return s[6:8] + s[4:6] + s[2:4] + s[0:2], nil
}

// canonicalString applies OpenSSL's name canonicalization: leading and
// trailing whitespace removed, ASCII lowercased, runs of whitespace folded to
// one space.
func canonicalString(s string) string {
	s = strings.TrimSpace(s)
	var b strings.Builder
	var inSpace bool
	for _, r := range s {
		switch {
		case r > 128:
			inSpace = false
			b.WriteRune(r)
		case unicode.IsSpace(r):
			if !inSpace {
				inSpace = true
				b.WriteRune(' ')
			}
		default:
			inSpace = false
			b.WriteRune(unicode.ToLower(r))
		}
	}
	return b.String()
}
