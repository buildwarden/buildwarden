package main

import (
	"crypto/x509"
	"fmt"
	"os"
	"path/filepath"
)

func configureUpstreamTLS(ledgerDir string) error {
	pool, err := buildUpstreamCertPool(ledgerDir)
	if err != nil {
		return err
	}
	if pool != nil {
		transport.TLSClientConfig.RootCAs = pool
	}
	return nil
}

func buildUpstreamCertPool(ledgerDir string) (*x509.CertPool, error) {
	includeSystem := os.Getenv("RELAY_SYSTEM_CA") != "false"

	bundlePath := filepath.Join(ledgerDir, "upstream-ca-bundle.pem")
	data, err := os.ReadFile(bundlePath)
	if os.IsNotExist(err) {
		if includeSystem {
			return nil, nil
		}
		fmt.Fprintf(os.Stderr,
			"relay: WARNING: system CAs disabled and no custom bundle provided"+
				" — all upstream HTTPS will fail\n")
		return x509.NewCertPool(), nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading upstream CA bundle: %w", err)
	}

	var pool *x509.CertPool
	if includeSystem {
		pool, err = x509.SystemCertPool()
		if err != nil {
			pool = x509.NewCertPool()
		}
	} else {
		pool = x509.NewCertPool()
	}

	if !pool.AppendCertsFromPEM(data) {
		return nil, fmt.Errorf(
			"upstream-ca-bundle.pem: no valid certificates found")
	}

	fmt.Fprintf(os.Stderr, "relay: loaded custom upstream CA bundle (%d bytes)\n",
		len(data))
	return pool, nil
}
