package main

// ExtCACerts configures CA certificate trust for package managers that
// don't use the system trust store by default. Each env var is harmless
// when its corresponding tool is not installed.
type ExtCACerts struct{}

func (e *ExtCACerts) BeforeBuild(env *CtrEnv) error { return nil }

func (e *ExtCACerts) Env() map[string]string {
	cert := "/etc/ssl/certs/warden.crt"
	bundle := "/etc/ssl/certs/ca-certificates.crt"

	return map[string]string{
		// Python: pip
		"PIP_CERT": cert,
		// Python: uv
		"UV_NATIVE_TLS": "1",
		// Python: poetry, conda, conan, httpx-based tools
		"REQUESTS_CA_BUNDLE": bundle,
		// Node.js: npm, yarn, pnpm, bun
		"NODE_EXTRA_CA_CERTS": cert,
		// Ruby: gem, bundler
		// Erlang/Elixir: hex, rebar3
		// Go: net/http (additive to system store)
		// General OpenSSL consumers
		"SSL_CERT_FILE": bundle,
		// PHP: composer (libcurl)
		"CURL_CA_BUNDLE": bundle,
		// Nix: nix-env, nix-build, nix-shell
		"NIX_SSL_CERT_FILE": bundle,
		// Elixir: hex
		"HEX_CACERTS_PATH": bundle,
	}
}
