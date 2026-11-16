module github.com/buildwarden/buildwarden/tools/conformance/collector-outputs

go 1.26.3

tool github.com/buildwarden/buildwarden-specs/collector-outputs/cmd/bw-conformance-collector-outputs

require (
	github.com/BurntSushi/toml v1.6.0 // indirect
	github.com/buildwarden/buildwarden-specs/collector-outputs v0.0.0-20261116000050-bc03f6fdc96e // indirect
	github.com/buildwarden/buildwarden-specs/conformance v0.1.0 // indirect
)

replace github.com/buildwarden/buildwarden-specs/conformance v0.1.0 => github.com/buildwarden/buildwarden-specs/conformance v0.0.0-20261116000050-bc03f6fdc96e
