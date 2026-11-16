module github.com/buildwarden/buildwarden/tools/conformance/bootstrap-config

go 1.26.3

tool github.com/buildwarden/buildwarden-specs/bootstrap-config/cmd/bw-conformance-bootstrap-config

require (
	github.com/BurntSushi/toml v1.6.0 // indirect
	github.com/buildwarden/buildwarden-specs/bootstrap-config v0.0.0-20261116043704-f201fd4ec04d // indirect
	github.com/buildwarden/buildwarden-specs/conformance v0.1.0 // indirect
	github.com/miekg/dns v1.1.73 // indirect
	github.com/santhosh-tekuri/jsonschema/v6 v6.0.2 // indirect
	golang.org/x/net v0.57.0 // indirect
	golang.org/x/sys v0.47.0 // indirect
	golang.org/x/text v0.40.0 // indirect
)

replace github.com/buildwarden/buildwarden-specs/conformance v0.1.0 => github.com/buildwarden/buildwarden-specs/conformance v0.0.0-20261116043704-f201fd4ec04d
