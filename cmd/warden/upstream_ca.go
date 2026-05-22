package main

import (
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var certExtensions = map[string]bool{
	".pem": true,
	".crt": true,
	".cer": true,
}

func prepareUpstreamCACerts(
	ledgerDir string, paths []string, includeSystem bool,
) error {
	if len(paths) == 0 && includeSystem {
		return nil
	}

	var bundle []byte
	var certCount, sourceCount int

	for _, p := range paths {
		expanded := expandHome(p)
		info, err := os.Stat(expanded)
		if err != nil {
			return fmt.Errorf("upstream CA path %q: %w", p, err)
		}

		if info.IsDir() {
			n, data, err := loadCertDir(expanded, p)
			if err != nil {
				return err
			}
			if n > 0 {
				bundle = append(bundle, data...)
				certCount += n
				sourceCount++
			}
		} else {
			n, data, err := loadCertFile(expanded, p)
			if err != nil {
				return err
			}
			bundle = append(bundle, data...)
			certCount += n
			sourceCount++
		}
	}

	if len(bundle) == 0 {
		return nil
	}

	bundlePath := filepath.Join(ledgerDir, "upstream-ca-bundle.pem")
	if err := os.WriteFile(bundlePath, bundle, 0644); err != nil {
		return fmt.Errorf("writing upstream CA bundle: %w", err)
	}

	log.Info(fmt.Sprintf(
		"Prepared upstream CA bundle: %d certificates from %d sources",
		certCount, sourceCount))
	return nil
}

func loadCertDir(dir, label string) (int, []byte, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, nil, fmt.Errorf("reading CA directory %q: %w", label, err)
	}
	var bundle []byte
	total := 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if !certExtensions[ext] {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			return 0, nil, fmt.Errorf(
				"reading %s/%s: %w", label, entry.Name(), err)
		}
		n := countPEMCerts(data)
		if n == 0 {
			return 0, nil, fmt.Errorf(
				"%s/%s: no valid PEM certificates found",
				label, entry.Name())
		}
		bundle = appendPEM(bundle, data)
		total += n
	}
	return total, bundle, nil
}

func loadCertFile(path, label string) (int, []byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, nil, fmt.Errorf("reading CA cert %q: %w", label, err)
	}
	n := countPEMCerts(data)
	if n == 0 {
		return 0, nil, fmt.Errorf("%q: no valid PEM certificates found", label)
	}
	return n, appendPEM(nil, data), nil
}

func appendPEM(bundle, data []byte) []byte {
	bundle = append(bundle, data...)
	if len(data) > 0 && data[len(data)-1] != '\n' {
		bundle = append(bundle, '\n')
	}
	return bundle
}

func countPEMCerts(data []byte) int {
	count := 0
	rest := data
	for {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}
		if block.Type == "CERTIFICATE" {
			count++
		}
	}
	return count
}

func expandHome(path string) string {
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}
