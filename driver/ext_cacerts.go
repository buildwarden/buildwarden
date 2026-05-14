package driver

type ExtCACerts struct{}

func (e *ExtCACerts) BeforeBuild(_ *ExtensionContext) error { return nil }

func (e *ExtCACerts) Env() map[string]string {
	cert := "/etc/ssl/certs/warden.crt"
	bundle := "/etc/ssl/certs/ca-certificates.crt"

	return map[string]string{
		"PIP_CERT":           cert,
		"UV_NATIVE_TLS":     "1",
		"REQUESTS_CA_BUNDLE": bundle,
		"NODE_EXTRA_CA_CERTS": cert,
		"SSL_CERT_FILE":      bundle,
		"CURL_CA_BUNDLE":     bundle,
		"NIX_SSL_CERT_FILE":  bundle,
		"HEX_CACERTS_PATH":   bundle,
	}
}
