package main

type ExtCACerts struct{}

func (e *ExtCACerts) BeforeBuild(env *CtrEnv) error { return nil }

func (e *ExtCACerts) Env() map[string]string {
	return map[string]string{
		"NODE_EXTRA_CA_CERTS": "/etc/ssl/certs/warden.crt",
		"REQUESTS_CA_BUNDLE":  "/etc/ssl/certs/ca-certificates.crt",
		"SSL_CERT_FILE":       "/etc/ssl/certs/ca-certificates.crt",
		"HEX_CACERTS_PATH":    "/etc/ssl/certs/ca-certificates.crt",
	}
}
