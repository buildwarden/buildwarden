module github.com/buildwarden/buildwarden/tools/conformance/relay-guest-api

go 1.26.3

tool github.com/buildwarden/buildwarden-specs/relay-guest-api/cmd/bw-conformance-relay-guest-api

require (
	github.com/BurntSushi/toml v1.6.0 // indirect
	github.com/buildwarden/buildwarden-specs/conformance v0.1.0 // indirect
	github.com/buildwarden/buildwarden-specs/relay-guest-api v0.0.0-20261116171557-b808a9192ccb // indirect
	github.com/google/btree v1.1.2 // indirect
	github.com/miekg/dns v1.1.73 // indirect
	github.com/santhosh-tekuri/jsonschema/v6 v6.0.2 // indirect
	golang.org/x/crypto v0.54.0 // indirect
	golang.org/x/exp v0.0.0-20250711185948-6ae5c78190dc // indirect
	golang.org/x/net v0.57.0 // indirect
	golang.org/x/sys v0.47.0 // indirect
	golang.org/x/text v0.40.0 // indirect
	golang.org/x/time v0.15.0 // indirect
	gvisor.dev/gvisor v0.0.0-20260907062237-d1e35511e5a4 // indirect
)

replace github.com/buildwarden/buildwarden-specs/conformance v0.1.0 => github.com/buildwarden/buildwarden-specs/conformance v0.0.0-20261116171557-b808a9192ccb
