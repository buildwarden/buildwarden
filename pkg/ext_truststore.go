package warden

import (
	"fmt"
	"os"
	"path/filepath"
	"warden/relay"
)

type ExtTrustStore struct{}

func (e *ExtTrustStore) BeforeBuild(env *CtrEnv) error {
	err := os.WriteFile(filepath.Join(env.wardenDirPath(), "ca.crt"), relay.CA_CERT, 0644)
	if err != nil {
		return fmt.Errorf("error writing certificate file: %w", err)
	}

	certhash, err := relay.CertSubjectHash()
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
