module github.com/buildwarden/buildwarden/tools/conformance/collector-inputs

go 1.26.3

tool github.com/buildwarden/buildwarden-specs/collector-inputs/cmd/bw-conformance-collector-inputs

require (
	github.com/BurntSushi/toml v1.6.0 // indirect
	github.com/buildwarden/buildwarden-specs/collector-inputs v0.0.0-20261116000050-bc03f6fdc96e // indirect
	github.com/buildwarden/buildwarden-specs/conformance v0.1.0 // indirect
)

replace github.com/buildwarden/buildwarden-specs/conformance v0.1.0 => github.com/buildwarden/buildwarden-specs/conformance v0.0.0-20261116000050-bc03f6fdc96e
