package main

import (
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"

	"github.com/pavlo-v-chernykh/keystore-go/v4"
)

type ExtBazel struct{}

func (e *ExtBazel) BeforeBuild(env *CtrEnv) error {
	bazelrc := `startup --host_jvm_args=-Djavax.net.ssl.trustStore=/.warden/certs.jks \
--host_jvm_args=-Djavax.net.ssl.trustStorePassword=changeit
`
	err := os.WriteFile(filepath.Join(env.wardenDirPath(), "bazel.bazelrc"),
		[]byte(bazelrc), 0644)
	if err != nil {
		return fmt.Errorf("failed to write bazel.bazelrc: %w", err)
	}

	block, _ := pem.Decode(caCertPEM)
	if block == nil {
		return fmt.Errorf("failed to parse relay certificate PEM")
	}
	jks := keystore.New()
	err = jks.SetTrustedCertificateEntry("warden",
		keystore.TrustedCertificateEntry{
			Certificate: keystore.Certificate{
				Content: block.Bytes,
				Type:    "X509",
			},
		})
	if err != nil {
		return fmt.Errorf("error adding certificate to JKS: %w", err)
	}

	f, err := os.Create(filepath.Join(env.wardenDirPath(), "certs.jks"))
	if err != nil {
		return fmt.Errorf("failed to create JKS file: %w", err)
	}
	defer f.Close()

	err = jks.Store(f, []byte("changeit"))
	if err != nil {
		return fmt.Errorf("failed to write JKS file: %w", err)
	}

	script := `#!/usr/bin/env sh

cp /.warden/bazel.bazelrc /etc/bazel.bazelrc
`
	err = os.WriteFile(filepath.Join(env.wardenScriptPath(), "bazel.sh"),
		[]byte(script), 0755)
	if err != nil {
		return fmt.Errorf("error writing bazel.sh: %w", err)
	}

	return nil
}

func (e *ExtBazel) Env() map[string]string {
	// JVM truststore flags — references /.warden/certs.jks created by BeforeBuild.
	// Covers: bazel (via bazelrc), maven (MAVEN_OPTS), gradle (GRADLE_OPTS)
	jvmFlags := "-Djavax.net.ssl.trustStore=/.warden/certs.jks " +
		"-Djavax.net.ssl.trustStorePassword=changeit"
	return map[string]string{
		"MAVEN_OPTS":  jvmFlags,
		"GRADLE_OPTS": jvmFlags,
	}
}
