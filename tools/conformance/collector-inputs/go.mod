module github.com/buildwarden/buildwarden/tools/conformance/collector-inputs

go 1.26.3

tool github.com/buildwarden/buildwarden-specs/collector-inputs/cmd/bw-conformance-collector-inputs

require (
	github.com/BurntSushi/toml v1.6.0 // indirect
	github.com/buildwarden/buildwarden-specs/collector-inputs v0.0.0-20261116042704-bfd8dabf3d7c // indirect
	github.com/buildwarden/buildwarden-specs/conformance v0.1.0 // indirect
	github.com/miekg/dns v1.1.73 // indirect
	github.com/santhosh-tekuri/jsonschema/v6 v6.0.2 // indirect
	golang.org/x/net v0.57.0 // indirect
	golang.org/x/sys v0.47.0 // indirect
	golang.org/x/text v0.40.0 // indirect
)

replace github.com/buildwarden/buildwarden-specs/conformance v0.1.0 => github.com/buildwarden/buildwarden-specs/conformance v0.0.0-20261116042704-bfd8dabf3d7c
