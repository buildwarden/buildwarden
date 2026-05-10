package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type ExtTrustStore struct{}

func (e *ExtTrustStore) BeforeBuild(env *CtrEnv) error {
	caPath := filepath.Join(env.ledgerDir, "ca.cert.pem")
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
		filepath.Join(env.wardenDirPath(), "ca.crt"), caCert, 0644,
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
	err = os.WriteFile(filepath.Join(env.wardenDirPath(), "certhook.sh"),
		[]byte(certhook), 0755)
	if err != nil {
		return fmt.Errorf("failed to write certhook.sh: %w", err)
	}

	script := `#!/usr/bin/env sh

mkdir -p /etc/ca-certificates/update.d
cp /.warden/certhook.sh /etc/ca-certificates/update.d/99warden
sh /etc/ca-certificates/update.d/99warden
`
	err = os.WriteFile(filepath.Join(env.wardenScriptPath(), "truststore.sh"),
		[]byte(script), 0755)
	if err != nil {
		return fmt.Errorf("error writing truststore.sh: %w", err)
	}

	return nil
}

func (e *ExtTrustStore) Env() map[string]string {
	return nil
}
